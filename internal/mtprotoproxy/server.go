package mtprotoproxy

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/mtproto"
	"tg-ws-proxy/internal/telegram"
	"tg-ws-proxy/internal/wsbridge"
)

const (
	wsFailFastDial   = 2 * time.Second
	wsFailCooldown   = 30 * time.Second
	initReadTimeout  = 10 * time.Second
	statsLogInterval = 30 * time.Second
)

type Server struct {
	cfg    config.Config
	logger *log.Logger
	pool   *wsbridge.Pool
	secret []byte

	stateMu     sync.Mutex
	wsBlacklist map[routeKey]struct{}
	wsFailUntil map[routeKey]time.Time
	stats       *stats
	wsDialFunc  wsbridge.DialFunc

	// for testing
	connectWSFunc func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error)
}

type routeKey struct {
	dc      int
	isMedia bool
}

type stats struct {
	mu          sync.Mutex
	connections int
	wsRouted    int
	tcpRouted   int
	errors      int
}

func NewServer(cfg config.Config, logger *log.Logger, pool *wsbridge.Pool, secret []byte) *Server {
	srv := &Server{
		cfg:         cfg,
		logger:      logger,
		pool:        pool,
		secret:      secret,
		wsBlacklist: make(map[routeKey]struct{}),
		wsFailUntil: make(map[routeKey]time.Time),
		stats:       &stats{},
		wsDialFunc:  wsbridge.Dial,
	}
	return srv
}

func (s *Server) Run(ctx context.Context) error {
	host := s.cfg.MTProtoHost
	if host == "" {
		host = s.cfg.Host
	}
	addr := net.JoinHostPort(host, strconv.Itoa(s.cfg.MTProtoPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	s.logger.Printf("mtproto proxy listening on %s", addr)
	s.startStatsLogger(ctx)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		tuneConn(conn)
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	clientAddr := conn.RemoteAddr().String()
	s.stats.inc(&s.stats.connections)
	s.debugf("[%s] accepted mtproto proxy connection", clientAddr)

	// Phase 1: Fake-TLS handshake.
	if err := FakeTLSHandshake(conn, s.secret); err != nil {
		s.debugf("[%s] fake-tls handshake failed: %v", clientAddr, err)
		s.stats.inc(&s.stats.errors)
		return
	}
	s.debugf("[%s] fake-tls handshake complete", clientAddr)

	// Phase 2: Read the obfuscated2 init packet (64 bytes inside a TLS app data record).
	_ = conn.SetReadDeadline(time.Now().Add(initReadTimeout))
	initRecord, err := ReadTLSRecord(conn)
	if err != nil {
		s.debugf("[%s] failed to read init record: %v", clientAddr, err)
		s.stats.inc(&s.stats.errors)
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	if len(initRecord) < initPacketSize {
		s.debugf("[%s] init record too short: %d bytes", clientAddr, len(initRecord))
		s.stats.inc(&s.stats.errors)
		return
	}

	// Phase 3: Decrypt init to get DC and cipher streams.
	pi, err := DecryptInit(initRecord[:initPacketSize], s.secret)
	if err != nil {
		s.debugf("[%s] init decryption failed: %v", clientAddr, err)
		s.stats.inc(&s.stats.errors)
		return
	}
	s.debugf("[%s] dc=%d media=%v proto=0x%08x", clientAddr, pi.DC, pi.IsMedia, pi.Proto)

	// Wrap the connection with crypto + TLS record framing.
	cryptoConn := NewTLSCryptoConn(conn, pi.Decryptor, pi.Encryptor)

	// Phase 4: Route to upstream DC.
	effectiveDC := telegram.NormalizeDC(pi.DC)
	targetIP, ok := s.cfg.DCIPs[effectiveDC]
	if !ok || targetIP == "" {
		s.debugf("[%s] no DC override for dc=%d, using tcp fallback to dc target", clientAddr, effectiveDC)
		s.stats.inc(&s.stats.errors)
		return
	}

	if isWSEnabledDC(effectiveDC) {
		s.routeWebSocket(ctx, cryptoConn, clientAddr, targetIP, effectiveDC, pi)
	} else {
		s.routeTCPFallback(ctx, cryptoConn, clientAddr, targetIP, effectiveDC, pi)
	}
}

func (s *Server) routeWebSocket(ctx context.Context, conn net.Conn, clientAddr, targetIP string, dc int, pi *ProxyInit) {
	ws, err := s.connectWS(ctx, targetIP, dc, pi.IsMedia)
	if err != nil {
		s.debugf("[%s] ws connect failed for DC%d: %v, falling back to tcp", clientAddr, dc, err)
		s.routeTCPFallback(ctx, conn, clientAddr, targetIP, dc, pi)
		return
	}
	defer ws.Close()
	s.stats.inc(&s.stats.wsRouted)
	s.debugf("[%s] route=websocket dc=%d media=%v target=%s", clientAddr, dc, pi.IsMedia, targetIP)

	var splitter *mtproto.Splitter
	splitter, _ = mtproto.NewSplitter(make([]byte, 64), pi.Proto)

	if err := wsbridge.Bridge(ctx, conn, ws, nil, splitter); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		s.debugf("[%s] ws bridge error: %v", clientAddr, err)
	}
	s.debugf("[%s] connection finished", clientAddr)
}

func (s *Server) routeTCPFallback(ctx context.Context, conn net.Conn, clientAddr, targetIP string, dc int, pi *ProxyInit) {
	s.stats.inc(&s.stats.tcpRouted)
	s.debugf("[%s] route=tcp dc=%d target=%s:443", clientAddr, dc, targetIP)

	dialer := &net.Dialer{Timeout: s.cfg.DialTimeout}
	upstream, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(targetIP, "443"))
	if err != nil {
		s.debugf("[%s] tcp dial failed: %v", clientAddr, err)
		s.stats.inc(&s.stats.errors)
		return
	}
	defer upstream.Close()
	tuneConn(upstream)

	// Relay between decrypted client and upstream DC.
	bridgeTCP(ctx, conn, upstream)
	s.debugf("[%s] connection finished", clientAddr)
}

func (s *Server) connectWS(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
	if s.connectWSFunc != nil {
		return s.connectWSFunc(ctx, targetIP, dc, isMedia)
	}

	key := routeKey{dc: dc, isMedia: isMedia}
	if s.isBlacklisted(key) {
		return nil, errors.New("ws blacklisted")
	}

	domains := telegram.WSDomains(dc, isMedia)
	if s.pool != nil {
		s.pool.SetDialFunc(s.wsDialFunc)
		if ws, ok := s.pool.Get(dc, isMedia, targetIP, domains); ok {
			s.debugf("ws pool hit: dc=%d media=%v", dc, isMedia)
			return ws, nil
		}
	}

	dialCfg := s.cfg
	if dialCfg.DialTimeout <= 0 || dialCfg.DialTimeout > wsFailFastDial {
		dialCfg.DialTimeout = wsFailFastDial
	}

	var lastErr error
	for _, domain := range domains {
		s.debugf("ws dial attempt: dc=%d media=%v target=%s domain=%s", dc, isMedia, targetIP, domain)
		ws, err := s.wsDialFunc(ctx, dialCfg, targetIP, domain)
		if err == nil {
			s.clearFailureState(key)
			s.debugf("ws dial success: dc=%d media=%v target=%s domain=%s", dc, isMedia, targetIP, domain)
			return ws, nil
		}
		s.debugf("ws dial failed: dc=%d media=%v domain=%s err=%v", dc, isMedia, domain, err)
		lastErr = err
	}

	s.markFailureCooldown(key)
	return nil, lastErr
}

// --- TLS-framed CryptoConn ---

// TLSCryptoConn wraps a connection with AES-CTR crypto and TLS record framing.
type TLSCryptoConn struct {
	inner     net.Conn
	decryptor *CryptoConn // for decrypt/encrypt
	readBuf   []byte      // buffered decrypted data from partial TLS records
}

func NewTLSCryptoConn(conn net.Conn, decryptor, encryptor interface{ XORKeyStream(dst, src []byte) }) *TLSCryptoConn {
	cc := NewCryptoConn(conn, decryptor.(interface {
		XORKeyStream(dst, src []byte)
	}), encryptor.(interface {
		XORKeyStream(dst, src []byte)
	}))
	return &TLSCryptoConn{
		inner:     conn,
		decryptor: cc,
	}
}

func (c *TLSCryptoConn) Read(b []byte) (int, error) {
	// If we have buffered data, return that first.
	if len(c.readBuf) > 0 {
		n := copy(b, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	// Read a TLS record from the underlying connection.
	payload, err := ReadTLSRecord(c.inner)
	if err != nil {
		return 0, err
	}

	// Decrypt the payload.
	c.decryptor.decryptor.XORKeyStream(payload, payload)

	n := copy(b, payload)
	if n < len(payload) {
		c.readBuf = payload[n:]
	}
	return n, nil
}

func (c *TLSCryptoConn) Write(b []byte) (int, error) {
	// Encrypt the data.
	encrypted := make([]byte, len(b))
	c.decryptor.encryptor.XORKeyStream(encrypted, b)

	// Write as TLS record.
	if err := WriteTLSRecord(c.inner, encrypted); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *TLSCryptoConn) Close() error                       { return c.inner.Close() }
func (c *TLSCryptoConn) LocalAddr() net.Addr                { return c.inner.LocalAddr() }
func (c *TLSCryptoConn) RemoteAddr() net.Addr               { return c.inner.RemoteAddr() }
func (c *TLSCryptoConn) SetDeadline(t time.Time) error      { return c.inner.SetDeadline(t) }
func (c *TLSCryptoConn) SetReadDeadline(t time.Time) error  { return c.inner.SetReadDeadline(t) }
func (c *TLSCryptoConn) SetWriteDeadline(t time.Time) error { return c.inner.SetWriteDeadline(t) }

// --- helpers ---

var wsEnabledDCs = map[int]struct{}{
	2: {},
	4: {},
}

func isWSEnabledDC(dc int) bool {
	_, ok := wsEnabledDCs[dc]
	return ok
}

func (s *Server) isBlacklisted(key routeKey) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	_, ok := s.wsBlacklist[key]
	return ok
}

func (s *Server) isCooldownActive(key routeKey) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	until, ok := s.wsFailUntil[key]
	return ok && time.Now().Before(until)
}

func (s *Server) clearFailureState(key routeKey) {
	s.stateMu.Lock()
	delete(s.wsBlacklist, key)
	delete(s.wsFailUntil, key)
	s.stateMu.Unlock()
}

func (s *Server) markFailureCooldown(key routeKey) {
	s.stateMu.Lock()
	s.wsFailUntil[key] = time.Now().Add(wsFailCooldown)
	s.stateMu.Unlock()
}

func (s *stats) inc(field *int) {
	s.mu.Lock()
	*field++
	s.mu.Unlock()
}

func (s *stats) snapshot() (int, int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections, s.wsRouted, s.tcpRouted, s.errors
}

func (s *Server) startStatsLogger(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(statsLogInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				conn, ws, tcp, errs := s.stats.snapshot()
				s.logger.Printf("mtproto stats: conn=%d ws=%d tcp=%d err=%d", conn, ws, tcp, errs)
				return
			case <-ticker.C:
				conn, ws, tcp, errs := s.stats.snapshot()
				s.logger.Printf("mtproto stats: conn=%d ws=%d tcp=%d err=%d", conn, ws, tcp, errs)
			}
		}
	}()
}

func (s *Server) debugf(format string, args ...any) {
	if !s.cfg.Verbose {
		return
	}
	s.logger.Printf(format, args...)
}

func tuneConn(conn net.Conn) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	_ = tcpConn.SetNoDelay(true)
}

func bridgeTCP(ctx context.Context, a, b net.Conn) {
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(b, a)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(a, b)
		errCh <- err
	}()
	select {
	case <-ctx.Done():
	case <-errCh:
	}
}
