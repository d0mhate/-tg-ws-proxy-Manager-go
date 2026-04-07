package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/mtproto"
	"tg-ws-proxy/internal/telegram"
	"tg-ws-proxy/internal/wsbridge"
)

var socksReplies = map[byte][]byte{
	0x00: {0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
	0x05: {0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
	0x07: {0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
	0x08: {0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
}

var errWSBlacklisted = errors.New("websocket route blacklisted")
var errUDPFragmentUnsupported = errors.New("udp fragmentation is not supported")

const (
	socksCmdConnect      = 0x01
	socksCmdUDPAssociate = 0x03
	socksAuthNoAuth      = 0x00
	socksAuthUserPass    = 0x02
	wsFailCooldown       = 30 * time.Second
	wsFailFastDial       = 2 * time.Second
	statsLogEvery        = 5 * time.Second
)

const errInvalidUsernamePassword = "invalid username/password"

var wsEnabledDCs = map[int]struct{}{
	2: {},
	4: {},
}

type Server struct {
	cfg    config.Config
	logger *log.Logger
	pool   *wsbridge.Pool

	stateMu      sync.Mutex
	wsBlacklist  map[routeKey]struct{}
	wsFailUntil  map[routeKey]time.Time
	stats        *runtimeStats
	authFails    *authFailureTracker
	hsFails      *handshakeFailureTracker
	verboseFails *connFailureTracker
	verboseDebug *debugEventTracker
	wsDialFunc   wsbridge.DialFunc

	proxyTCPFunc         func(ctx context.Context, conn net.Conn, host string, port int) error
	proxyTCPWithInitFunc func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error
	connectWSFunc        func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error)
}

type request struct {
	Cmd     byte
	DstHost string
	DstPort int
}

type udpPacket struct {
	Host    string
	Port    int
	Payload []byte
}

type routeKey struct {
	dc      int
	isMedia bool
}

type runtimeStats struct {
	mu               sync.Mutex
	handshakeWait    int
	handshakeEOF     int
	handshakeBadVer  int
	handshakeOther   int
	connections      int
	wsConnections    int
	wsMedia          int
	tcpFallbacks     int
	tcpFallbackMedia int
	httpRejected     int
	passthrough      int
	wsErrors         int
	poolHits         int
	poolMisses       int
	blacklistHits    int
	cooldownActivs   int
	wsByDC           map[int]int
	tcpFallbackByDC  map[int]int
	errorCounts      map[string]int
}

type authFailureTracker struct {
	mu       sync.Mutex
	count    int
	lastAddr string
}

type handshakeFailureTracker struct {
	mu       sync.Mutex
	counts   map[string]int
	lastAddr map[string]string
}

type connFailureTracker struct {
	mu       sync.Mutex
	counts   map[string]int
	lastAddr map[string]string
}

type debugEventTracker struct {
	mu      sync.Mutex
	counts  map[string]int
	samples map[string]string
}

type handshakeError struct {
	stage string
	err   error
}

func (e *handshakeError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *handshakeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func NewServer(cfg config.Config, logger *log.Logger) *Server {
	srv := &Server{
		cfg:         cfg,
		logger:      logger,
		pool:        wsbridge.NewPool(cfg),
		wsBlacklist: make(map[routeKey]struct{}),
		wsFailUntil: make(map[routeKey]time.Time),
		stats: &runtimeStats{
			wsByDC:          make(map[int]int),
			tcpFallbackByDC: make(map[int]int),
			errorCounts:     make(map[string]int),
		},
		authFails: &authFailureTracker{},
		hsFails: &handshakeFailureTracker{
			counts:   make(map[string]int),
			lastAddr: make(map[string]string),
		},
		verboseFails: &connFailureTracker{
			counts:   make(map[string]int),
			lastAddr: make(map[string]string),
		},
		verboseDebug: &debugEventTracker{
			counts:  make(map[string]int),
			samples: make(map[string]string),
		},
		wsDialFunc: wsbridge.Dial,
	}
	if srv.pool != nil {
		srv.pool.SetDialFunc(srv.wsDialFunc)
	}
	return srv
}

func (s *Server) Run(ctx context.Context) error {
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	defer func() {
		if s.pool != nil {
			s.pool.Close()
		}
	}()
	s.startStatsLogger(ctx)

	s.logger.Printf("listening on %s", addr)
	if s.cfg.Verbose {
		s.logger.Printf("verbose aggregation window: %s", statsLogEvery)
		s.logger.Printf("verbose mode: repeated events and errors will be summarized, waiting for traffic...")
		s.logger.Printf("stats legend:\n  hs_wait       waiting for SOCKS5 handshake\n  hs_eof        client closed before SOCKS5 greeting\n  hs_badver     invalid SOCKS version byte\n  hs_other      other handshake failures\n  conn          successful SOCKS5 handshakes\n  ws            websocket routes\n  tcp_fb        tcp fallbacks\n  passthrough   direct passthrough routes\n  http_reject   telegram http transport fallbacks\n  ws_err        websocket dial/bridge errors\n  pool_hit      websocket pool hits\n  pool_miss     websocket pool misses\n  blacklist_hit websocket blacklist hits\n  cooldown_set  websocket cooldown activations")
	}

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
		s.tuneConn(conn)
		s.debugf("accepted connection from %s", conn.RemoteAddr())
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	clientAddr := remoteAddr(conn)

	s.stats.incHandshakeWait()
	req, err := handshake(conn, s.cfg)
	s.stats.decHandshakeWait()
	if err != nil {
		s.logHandshakeFailure(clientAddr, err)
		return
	}
	s.stats.incConnections()
	switch req.Cmd {
	case socksCmdConnect:
		s.debugf("[%s] socks connect request to %s:%d", clientAddr, req.DstHost, req.DstPort)
	case socksCmdUDPAssociate:
		s.debugf("[%s] socks udp associate request for %s:%d", clientAddr, req.DstHost, req.DstPort)
		s.handleUDPAssociate(ctx, conn, req)
		return
	default:
		s.logger.Printf("[%s] unsupported socks command: %d", clientAddr, req.Cmd)
		_ = writeReply(conn, 0x07)
		return
	}

	if req.DstHost == "" || req.DstPort <= 0 {
		_ = writeReply(conn, 0x05)
		return
	}

	dstIP := net.ParseIP(req.DstHost)
	isIPv6 := dstIP != nil && dstIP.To4() == nil
	isTelegramCandidate := telegram.IsTelegramIP(req.DstHost) || isLikelyTelegramIPv6(req, isIPv6)
	shouldProbeMTProto := !isTelegramCandidate && shouldProbeTelegramByPort(req)
	routeByInitOnly := false

	if !isTelegramCandidate && !shouldProbeMTProto {
		s.stats.incPassthrough()
		s.debugf("[%s] route=passthrough destination=%s:%d", clientAddr, req.DstHost, req.DstPort)
		if err := writeReply(conn, 0x00); err != nil {
			return
		}
		if err := s.proxyTCP(ctx, conn, req.DstHost, req.DstPort); err != nil && !errors.Is(err, io.EOF) {
			s.stats.recordError("passthrough", err)
			s.recordVerboseConnFailure(clientAddr, "passthrough", err)
		}
		return
	}
	s.debugf("[%s] telegram destination detected: %s:%d", clientAddr, req.DstHost, req.DstPort)

	if err := writeReply(conn, 0x00); err != nil {
		return
	}

	init := make([]byte, 64)
	n, err := readWithContext(ctx, conn, init, s.cfg.InitTimeout)
	if err != nil {
		if !isTelegramCandidate {
			s.stats.incPassthrough()
			s.debugf("[%s] route=passthrough reason=probe-read-failed destination=%s:%d err=%v", clientAddr, req.DstHost, req.DstPort, err)
			if n == 0 {
				if ptErr := s.proxyTCP(ctx, conn, req.DstHost, req.DstPort); ptErr != nil && !errors.Is(ptErr, io.EOF) {
					s.stats.recordError("passthrough", ptErr)
					s.recordVerboseConnFailure(clientAddr, "passthrough", ptErr)
				}
				return
			}
			if ptErr := s.proxyTCPWithInit(ctx, conn, req.DstHost, req.DstPort, init[:n]); ptErr != nil && !errors.Is(ptErr, io.EOF) {
				s.stats.recordError("passthrough", ptErr)
				s.recordVerboseConnFailure(clientAddr, "passthrough", ptErr)
			}
			return
		}
		s.stats.recordError("mtproto_init", err)
		s.recordVerboseConnFailure(clientAddr, "mtproto_init", err)
		return
	}

	if !isTelegramCandidate {
		info, parseErr := mtproto.ParseInit(init)
		if parseErr != nil {
			s.stats.incPassthrough()
			s.debugf("[%s] route=passthrough reason=mtproto-probe-miss destination=%s:%d", clientAddr, req.DstHost, req.DstPort)
			if ptErr := s.proxyTCPWithInit(ctx, conn, req.DstHost, req.DstPort, init); ptErr != nil && !errors.Is(ptErr, io.EOF) {
				s.stats.recordError("passthrough", ptErr)
				s.recordVerboseConnFailure(clientAddr, "passthrough", ptErr)
			}
			return
		}
		isTelegramCandidate = true
		routeByInitOnly = true
		s.debugf("[%s] telegram route inferred from mtproto init on destination %s:%d dc=%d media=%v", clientAddr, req.DstHost, req.DstPort, info.DC, info.IsMedia)
	}

	if mtproto.IsHTTPTransport(init) {
		if routeByInitOnly {
			s.stats.incPassthrough()
			s.debugf("[%s] route=passthrough reason=http-probe destination=%s:%d", clientAddr, req.DstHost, req.DstPort)
			if ptErr := s.proxyTCPWithInit(ctx, conn, req.DstHost, req.DstPort, init); ptErr != nil && !errors.Is(ptErr, io.EOF) {
				s.stats.recordError("passthrough", ptErr)
				s.recordVerboseConnFailure(clientAddr, "passthrough", ptErr)
			}
			return
		}
		s.stats.incTCPFallback()
		s.stats.recordTCPFallbackRoute(0, false)
		s.debugf("[%s] route=tcp-fallback reason=http-transport destination=%s:%d", clientAddr, req.DstHost, req.DstPort)
		if err := s.proxyTCPWithInit(ctx, conn, req.DstHost, req.DstPort, init); err != nil && !errors.Is(err, io.EOF) {
			s.stats.recordError("tcp_fb", err)
			s.recordVerboseConnFailure(clientAddr, "tcp_fb", err)
		}
		return
	}

	info, err := mtproto.ParseInit(init)
	if err != nil && !errors.Is(err, mtproto.ErrInvalidProto) {
		s.recordVerboseConnFailure(clientAddr, "mtproto_parse", err)
	}

	dc := info.DC
	isMedia := info.IsMedia
	proto := info.Proto
	initPatched := false
	inferredFromDestination := false
	s.debugf("[%s] mtproto init parsed: dc=%d media=%v proto=0x%08x", clientAddr, dc, isMedia, proto)

	if dc == 0 {
		if endpoint, ok := telegram.LookupEndpoint(req.DstHost); ok {
			dc = endpoint.DC
			isMedia = endpoint.IsMedia
			inferredFromDestination = true
			s.debugf("[%s] dc inferred from destination ip: dc=%d media=%v", clientAddr, dc, isMedia)
			if _, ok := s.cfg.DCIPs[dc]; ok {
				patched, patchErr := mtproto.PatchInitDC(init, choosePatchedDC(dc, isMedia))
				if patchErr == nil {
					init = patched
					initPatched = true
					s.debugf("[%s] patched mtproto init with dc=%d", clientAddr, choosePatchedDC(dc, isMedia))
				}
			}
		}
	}

	effectiveDC := s.effectiveDC(dc)
	if effectiveDC != 0 && effectiveDC != dc {
		patched, patchErr := mtproto.PatchInitDC(init, choosePatchedDC(effectiveDC, isMedia))
		if patchErr == nil {
			init = patched
			initPatched = true
			s.debugf("[%s] normalized dc=%d -> %d and patched mtproto init", clientAddr, dc, effectiveDC)
		}
	}

	targetIP, ok := s.cfg.DCIPs[effectiveDC]
	if !ok || targetIP == "" {
		s.stats.incTCPFallback()
		s.stats.recordTCPFallbackRoute(effectiveDC, isMedia)
		s.debugf("[%s] route=tcp-fallback reason=no-dc-override dc=%d effective_dc=%d destination=%s:%d", clientAddr, dc, effectiveDC, req.DstHost, req.DstPort)
		if err := s.proxyTCPWithInit(ctx, conn, req.DstHost, req.DstPort, init); err != nil && !errors.Is(err, io.EOF) {
			s.stats.recordError("tcp_fb", err)
			s.recordVerboseConnFailure(clientAddr, "tcp_fb", err)
		}
		return
	}

	fallbackHost := req.DstHost
	if isIPv6 || effectiveDC != dc || routeByInitOnly || inferredFromDestination {
		fallbackHost = targetIP
		s.debugf("[%s] telegram route will fallback via dc target %s", clientAddr, targetIP)
	}

	wsDomainDC := s.wsDomainDC(effectiveDC)
	if !isWSEnabledDC(wsDomainDC) {
		s.stats.incTCPFallback()
		s.stats.recordTCPFallbackRoute(effectiveDC, isMedia)
		s.debugf("[%s] route=tcp-fallback reason=ws-disabled-dc dc=%d effective_dc=%d ws_dc=%d target=%s", clientAddr, dc, effectiveDC, wsDomainDC, targetIP)
		if err := s.proxyTCPWithInit(ctx, conn, fallbackHost, req.DstPort, init); err != nil && !errors.Is(err, io.EOF) {
			s.stats.recordError("tcp_fb", err)
			s.recordVerboseConnFailure(clientAddr, "tcp_fb", err)
		}
		return
	}

	ws, err := s.connectWS(ctx, targetIP, effectiveDC, isMedia)
	if err != nil {
		s.stats.recordError("ws_connect", err)
		s.recordVerboseConnFailure(clientAddr, "ws_connect", err)
		s.stats.incTCPFallback()
		s.stats.recordTCPFallbackRoute(effectiveDC, isMedia)
		s.debugf("[%s] route=tcp-fallback reason=%s dc=%d effective_dc=%d target=%s", clientAddr, fallbackReason(err), dc, effectiveDC, targetIP)
		if fbErr := s.proxyTCPWithInit(ctx, conn, fallbackHost, req.DstPort, init); fbErr != nil && !errors.Is(fbErr, io.EOF) {
			s.stats.recordError("tcp_fb", fbErr)
			s.recordVerboseConnFailure(clientAddr, "tcp_fb", fbErr)
		}
		return
	}
	defer ws.Close()
	s.stats.incWSConnections()
	s.stats.recordWSRoute(effectiveDC, isMedia)
	s.debugf("[%s] route=websocket dc=%d effective_dc=%d ws_dc=%d media=%v target=%s", clientAddr, dc, effectiveDC, wsDomainDC, isMedia, targetIP)

	var splitter *mtproto.Splitter
	if proto != 0 && (initPatched || isMedia || proto != mtproto.ProtoIntermediate) {
		splitter, _ = mtproto.NewSplitter(init, proto)
		if splitter != nil {
			s.debugf("[%s] websocket splitter enabled for proto=0x%08x", clientAddr, proto)
		}
	}

	if err := wsbridge.Bridge(ctx, conn, ws, init, splitter); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		s.stats.recordError("ws_bridge", err)
		s.recordVerboseConnFailure(clientAddr, "ws_bridge", err)
		return
	}
	s.debugf("[%s] connection finished", clientAddr)
}

func (s *Server) connectWS(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
	if s.connectWSFunc != nil {
		return s.connectWSFunc(ctx, targetIP, dc, isMedia)
	}

	key := routeKey{dc: dc, isMedia: isMedia}
	if s.isBlacklisted(key) {
		s.stats.incBlacklistHits()
		return nil, errWSBlacklisted
	}

	domains := telegram.WSDomains(s.wsDomainDC(dc), isMedia)
	if s.pool != nil {
		s.pool.SetDialFunc(s.wsDialFunc)
		if ws, ok := s.pool.Get(dc, isMedia, targetIP, domains); ok {
			s.stats.incPoolHit()
			s.debugf("ws pool hit: dc=%d media=%v target=%s", dc, isMedia, targetIP)
			return ws, nil
		}
		s.stats.incPoolMiss()
	}

	dialCfg := s.cfg
	if dialCfg.DialTimeout <= 0 || dialCfg.DialTimeout > wsFailFastDial {
		dialCfg.DialTimeout = wsFailFastDial
		s.debugf("ws fail-fast timeout: dc=%d media=%v timeout=%s", dc, isMedia, dialCfg.DialTimeout)
	}
	if s.isCooldownActive(key) {
		s.debugf("ws cooldown active: dc=%d media=%v timeout=%s", dc, isMedia, dialCfg.DialTimeout)
	}

	var lastErr error
	allRedirects := true
	sawRedirect := false
	for _, domain := range domains {
		s.debugf("ws dial attempt: dc=%d media=%v target=%s domain=%s", dc, isMedia, targetIP, domain)
		ws, err := s.wsDialFunc(ctx, dialCfg, targetIP, domain)
		if err == nil {
			s.clearFailureState(key)
			s.debugf("ws dial success: dc=%d media=%v target=%s domain=%s", dc, isMedia, targetIP, domain)
			return ws, nil
		}
		s.debugf("ws dial failed: dc=%d media=%v target=%s domain=%s err=%v", dc, isMedia, targetIP, domain, err)
		s.stats.incWSErrors()
		var hErr *wsbridge.HandshakeError
		if errors.As(err, &hErr) && hErr.IsRedirect() {
			sawRedirect = true
		} else {
			allRedirects = false
		}
		lastErr = err
	}

	if sawRedirect && allRedirects {
		s.markBlacklisted(key)
		return nil, fmt.Errorf("all websocket routes redirected: %w", errWSBlacklisted)
	}
	s.markFailureCooldown(key)
	return nil, lastErr
}

func (s *Server) proxyTCP(ctx context.Context, conn net.Conn, host string, port int) error {
	if s.proxyTCPFunc != nil {
		return s.proxyTCPFunc(ctx, conn, host, port)
	}

	upstream, err := s.dialTCP(ctx, host, port)
	if err != nil {
		return err
	}
	defer upstream.Close()
	return bridgeTCP(ctx, conn, upstream)
}

func (s *Server) proxyTCPWithInit(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
	if s.proxyTCPWithInitFunc != nil {
		return s.proxyTCPWithInitFunc(ctx, conn, host, port, init)
	}

	upstream, err := s.dialTCP(ctx, host, port)
	if err != nil {
		return err
	}
	defer upstream.Close()

	if _, err := upstream.Write(init); err != nil {
		return err
	}
	return bridgeTCP(ctx, conn, upstream)
}

func (s *Server) dialTCP(ctx context.Context, host string, port int) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: s.cfg.DialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	s.tuneConn(conn)
	return conn, nil
}

func (s *Server) tuneConn(conn net.Conn) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	_ = tcpConn.SetNoDelay(true)
	if s.cfg.BufferKB > 0 {
		size := s.cfg.BufferKB * 1024
		_ = tcpConn.SetReadBuffer(size)
		_ = tcpConn.SetWriteBuffer(size)
	}
}

func bridgeTCP(ctx context.Context, a net.Conn, b net.Conn) error {
	errCh := make(chan error, 2)

	go func() {
		_, err := io.Copy(b, a)
		errCh <- normalizeEOF(err)
	}()
	go func() {
		_, err := io.Copy(a, b)
		errCh <- normalizeEOF(err)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func handshake(conn net.Conn, cfg config.Config) (request, error) {
	var req request
	buf := make([]byte, 262)

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return req, &handshakeError{stage: "greeting", err: err}
	}
	if buf[0] != 0x05 {
		return req, &handshakeError{stage: "greeting", err: errors.New("unsupported socks version")}
	}

	nMethods := int(buf[1])
	if nMethods == 0 {
		return req, &handshakeError{stage: "greeting", err: errors.New("no auth methods provided")}
	}
	if _, err := io.ReadFull(conn, buf[:nMethods]); err != nil {
		return req, &handshakeError{stage: "greeting", err: err}
	}
	method, err := negotiateAuthMethod(buf[:nMethods], cfg)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0xff})
		return req, &handshakeError{stage: "greeting", err: err}
	}
	if _, err := conn.Write([]byte{0x05, method}); err != nil {
		return req, &handshakeError{stage: "greeting", err: err}
	}
	if method == socksAuthUserPass {
		if err := authenticateUserPass(conn, cfg.Username, cfg.Password); err != nil {
			return req, &handshakeError{stage: "auth", err: err}
		}
	}

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return req, &handshakeError{stage: "request", err: err}
	}
	if buf[0] != 0x05 {
		return req, &handshakeError{stage: "request", err: errors.New("unsupported socks version")}
	}
	req.Cmd = buf[1]
	if req.Cmd != socksCmdConnect && req.Cmd != socksCmdUDPAssociate {
		return req, &handshakeError{stage: "request", err: errors.New("only connect and udp associate are supported")}
	}

	switch buf[3] {
	case 0x01:
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return req, &handshakeError{stage: "request", err: err}
		}
		req.DstHost = net.IP(buf[:4]).String()
	case 0x03:
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return req, &handshakeError{stage: "request", err: err}
		}
		size := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:size]); err != nil {
			return req, &handshakeError{stage: "request", err: err}
		}
		req.DstHost = string(buf[:size])
	case 0x04:
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return req, &handshakeError{stage: "request", err: err}
		}
		req.DstHost = net.IP(buf[:16]).String()
	default:
		return req, &handshakeError{stage: "request", err: errors.New("address type not supported")}
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return req, &handshakeError{stage: "request", err: err}
	}
	req.DstPort = int(binary.BigEndian.Uint16(buf[:2]))
	return req, nil
}

func negotiateAuthMethod(methods []byte, cfg config.Config) (byte, error) {
	required := byte(socksAuthNoAuth)
	if cfg.Username != "" || cfg.Password != "" {
		required = byte(socksAuthUserPass)
	}
	for _, method := range methods {
		if method == required {
			return required, nil
		}
	}
	if required == socksAuthUserPass {
		return 0xff, errors.New("username/password auth is required")
	}
	return 0xff, errors.New("no supported auth methods provided")
}

func authenticateUserPass(conn net.Conn, username, password string) error {
	buf := make([]byte, 513)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return err
	}
	if buf[0] != 0x01 {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return errors.New("unsupported username/password auth version")
	}

	userLen := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:userLen]); err != nil {
		return err
	}
	gotUser := string(buf[:userLen])

	if _, err := io.ReadFull(conn, buf[:1]); err != nil {
		return err
	}
	passLen := int(buf[0])
	if _, err := io.ReadFull(conn, buf[:passLen]); err != nil {
		return err
	}
	gotPass := string(buf[:passLen])

	if gotUser != username || gotPass != password {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return errors.New("invalid username/password")
	}
	if _, err := conn.Write([]byte{0x01, 0x00}); err != nil {
		return err
	}
	return nil
}

func writeReply(conn net.Conn, status byte) error {
	return writeReplyAddr(conn, status, net.IPv4zero.String(), 0)
}

func writeReplyAddr(conn net.Conn, status byte, host string, port int) error {
	reply, err := buildReply(status, host, port)
	if err != nil {
		reply = socksReplies[0x05]
	}
	_, err = conn.Write(reply)
	return err
}

func buildReply(status byte, host string, port int) ([]byte, error) {
	replyStatus := status
	reply, ok := socksReplies[replyStatus]
	if !ok {
		replyStatus = 0x05
		reply = socksReplies[replyStatus]
	}
	if host == "" && port == 0 {
		return append([]byte(nil), reply...), nil
	}

	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		out := []byte{0x05, replyStatus, 0x00, 0x01}
		out = append(out, ip4...)
		var portBuf [2]byte
		binary.BigEndian.PutUint16(portBuf[:], uint16(port))
		out = append(out, portBuf[:]...)
		return out, nil
	}
	if ip16 := ip.To16(); ip16 != nil {
		out := []byte{0x05, replyStatus, 0x00, 0x04}
		out = append(out, ip16...)
		var portBuf [2]byte
		binary.BigEndian.PutUint16(portBuf[:], uint16(port))
		out = append(out, portBuf[:]...)
		return out, nil
	}
	if len(host) > 255 {
		return nil, errors.New("domain name too long")
	}
	out := []byte{0x05, replyStatus, 0x00, 0x03, byte(len(host))}
	out = append(out, []byte(host)...)
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	out = append(out, portBuf[:]...)
	return out, nil
}

func readWithContext(ctx context.Context, conn net.Conn, buf []byte, timeout time.Duration) (int, error) {
	if timeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		defer conn.SetReadDeadline(time.Time{})
	}

	type readResult struct {
		n   int
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		n, err := io.ReadFull(conn, buf)
		done <- readResult{n: n, err: err}
	}()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case result := <-done:
		return result.n, result.err
	}
}

func choosePatchedDC(dc int, isMedia bool) int {
	if isMedia {
		return -dc
	}
	return dc
}

func (s *Server) effectiveDC(dc int) int {
	if dc == 0 {
		return 0
	}
	if _, ok := s.cfg.DCIPs[dc]; ok {
		return dc
	}
	return telegram.NormalizeDC(dc)
}

func (s *Server) wsDomainDC(dc int) int {
	if dc == 0 {
		return 0
	}
	return telegram.NormalizeDC(dc)
}

func normalizeEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func isLikelyTelegramIPv6(req request, isIPv6 bool) bool {
	if !isIPv6 {
		return false
	}
	switch req.DstPort {
	case 80, 443, 5222:
		return true
	default:
		return false
	}
}

func shouldProbeTelegramByPort(req request) bool {
	switch req.DstPort {
	case 80, 443, 5222:
		return true
	default:
		return false
	}
}

func isWSEnabledDC(dc int) bool {
	_, ok := wsEnabledDCs[dc]
	return ok
}

func (s *Server) debugf(format string, args ...any) {
	if !s.cfg.Verbose {
		return
	}
	s.recordVerboseDebug(fmt.Sprintf(format, args...))
}

func remoteAddr(conn net.Conn) string {
	if conn == nil || conn.RemoteAddr() == nil {
		return "unknown"
	}
	return conn.RemoteAddr().String()
}

func (s *Server) handleUDPAssociate(ctx context.Context, conn net.Conn, req request) {
	clientAddr := remoteAddr(conn)
	pc, bindHost, bindPort, err := s.listenUDPAssociate(conn)
	if err != nil {
		s.logger.Printf("[%s] udp associate setup failed: %v", clientAddr, err)
		_ = writeReply(conn, 0x05)
		return
	}
	defer pc.Close()

	if err := writeReplyAddr(conn, 0x00, bindHost, bindPort); err != nil {
		s.logger.Printf("[%s] udp associate reply failed: %v", clientAddr, err)
		return
	}
	s.debugf("[%s] route=udp-associate bind=%s:%d expected=%s:%d", clientAddr, bindHost, bindPort, req.DstHost, req.DstPort)

	if err := s.serveUDPAssociation(ctx, conn, pc, req); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		s.logger.Printf("[%s] udp associate ended with error: %v", clientAddr, err)
		return
	}
	s.debugf("[%s] udp association finished", clientAddr)
}

func (s *Server) listenUDPAssociate(conn net.Conn) (net.PacketConn, string, int, error) {
	tcpLocal, _ := conn.LocalAddr().(*net.TCPAddr)

	network := "udp4"
	bindHost := net.IPv4zero.String()
	replyHost := bindHost

	if tcpLocal != nil && tcpLocal.IP != nil && tcpLocal.IP.To4() == nil {
		network = "udp6"
		bindHost = "::"
		replyHost = "::"
	}
	if tcpLocal != nil && tcpLocal.IP != nil && !tcpLocal.IP.IsUnspecified() {
		bindHost = tcpLocal.IP.String()
		replyHost = bindHost
	}

	pc, err := net.ListenPacket(network, net.JoinHostPort(bindHost, "0"))
	if err != nil {
		return nil, "", 0, err
	}

	udpAddr, ok := pc.LocalAddr().(*net.UDPAddr)
	if !ok {
		_ = pc.Close()
		return nil, "", 0, errors.New("unexpected udp listener address")
	}
	if replyHost == "" || replyHost == "::" || replyHost == "0.0.0.0" {
		if udpAddr.IP != nil && !udpAddr.IP.IsUnspecified() {
			replyHost = udpAddr.IP.String()
		}
	}
	if replyHost == "" {
		replyHost = net.IPv4zero.String()
	}
	return pc, replyHost, udpAddr.Port, nil
}

func (s *Server) serveUDPAssociation(ctx context.Context, conn net.Conn, pc net.PacketConn, req request) error {
	associated := s.expectedUDPClientAddr(conn, req)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(io.Discard, conn)
	}()

	buf := make([]byte, 64*1024)
	for {
		_ = pc.SetReadDeadline(time.Now().Add(time.Second))
		n, src, err := pc.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-done:
					return nil
				default:
					continue
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				return nil
			default:
				return err
			}
		}

		srcAddr, ok := src.(*net.UDPAddr)
		if !ok {
			continue
		}

		if isAssociatedUDPClient(srcAddr, associated) {
			packet, perr := parseUDPAssociatePacket(buf[:n])
			if perr != nil {
				s.debugf("[%s] udp client packet ignored: %v", remoteAddr(conn), perr)
				continue
			}
			if associated.Port == 0 {
				associated = &net.UDPAddr{IP: append(net.IP(nil), srcAddr.IP...), Port: srcAddr.Port}
			}
			dstAddr, derr := net.ResolveUDPAddr("udp", net.JoinHostPort(packet.Host, strconv.Itoa(packet.Port)))
			if derr != nil {
				s.debugf("[%s] udp destination resolve failed for %s:%d: %v", remoteAddr(conn), packet.Host, packet.Port, derr)
				continue
			}
			if _, werr := pc.WriteTo(packet.Payload, dstAddr); werr != nil {
				return werr
			}
			continue
		}

		if associated == nil || associated.Port == 0 {
			continue
		}
		payload, perr := buildUDPAssociatePacket(srcAddr.IP.String(), srcAddr.Port, buf[:n])
		if perr != nil {
			continue
		}
		if _, werr := pc.WriteTo(payload, associated); werr != nil {
			return werr
		}
	}
}

func (s *Server) expectedUDPClientAddr(conn net.Conn, req request) *net.UDPAddr {
	tcpRemote, _ := conn.RemoteAddr().(*net.TCPAddr)
	if tcpRemote == nil {
		return nil
	}

	ip := append(net.IP(nil), tcpRemote.IP...)
	if parsed := net.ParseIP(req.DstHost); parsed != nil && !parsed.IsUnspecified() {
		ip = append(net.IP(nil), parsed...)
	}
	return &net.UDPAddr{IP: ip, Port: req.DstPort}
}

func isAssociatedUDPClient(src *net.UDPAddr, expected *net.UDPAddr) bool {
	if src == nil || expected == nil {
		return false
	}
	if expected.IP != nil && len(expected.IP) > 0 && !src.IP.Equal(expected.IP) {
		return false
	}
	if expected.Port != 0 && src.Port != expected.Port {
		return false
	}
	return true
}

func parseUDPAssociatePacket(data []byte) (udpPacket, error) {
	var packet udpPacket
	if len(data) < 4 {
		return packet, io.ErrUnexpectedEOF
	}
	if data[0] != 0x00 || data[1] != 0x00 {
		return packet, errors.New("invalid udp associate reserved bytes")
	}
	if data[2] != 0x00 {
		return packet, errUDPFragmentUnsupported
	}

	offset := 4
	switch data[3] {
	case 0x01:
		if len(data) < offset+4+2 {
			return packet, io.ErrUnexpectedEOF
		}
		packet.Host = net.IP(data[offset : offset+4]).String()
		offset += 4
	case 0x03:
		if len(data) < offset+1 {
			return packet, io.ErrUnexpectedEOF
		}
		size := int(data[offset])
		offset++
		if len(data) < offset+size+2 {
			return packet, io.ErrUnexpectedEOF
		}
		packet.Host = string(data[offset : offset+size])
		offset += size
	case 0x04:
		if len(data) < offset+16+2 {
			return packet, io.ErrUnexpectedEOF
		}
		packet.Host = net.IP(data[offset : offset+16]).String()
		offset += 16
	default:
		return packet, errors.New("unsupported udp associate address type")
	}

	packet.Port = int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	packet.Payload = append([]byte(nil), data[offset:]...)
	return packet, nil
}

func buildUDPAssociatePacket(host string, port int, payload []byte) ([]byte, error) {
	packet := []byte{0x00, 0x00, 0x00}

	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		packet = append(packet, 0x01)
		packet = append(packet, ip4...)
	} else if ip16 := ip.To16(); ip16 != nil {
		packet = append(packet, 0x04)
		packet = append(packet, ip16...)
	} else {
		if len(host) > 255 {
			return nil, errors.New("domain name too long")
		}
		packet = append(packet, 0x03, byte(len(host)))
		packet = append(packet, []byte(host)...)
	}

	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	packet = append(packet, portBuf[:]...)
	packet = append(packet, payload...)
	return packet, nil
}

func (s *Server) startStatsLogger(ctx context.Context) {
	if s.stats == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(statsLogEvery)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.flushAuthFailureSummary()
				s.flushHandshakeFailureSummary()
				s.flushVerboseConnFailureSummary()
				s.flushVerboseDebugSummary()
				s.logger.Printf("%s", s.stats.summaryBlock(s.blacklistSize(), s.cooldownSize()))
				return
			case <-ticker.C:
				s.flushAuthFailureSummary()
				s.flushHandshakeFailureSummary()
				s.flushVerboseConnFailureSummary()
				s.flushVerboseDebugSummary()
				s.logger.Printf("%s", s.stats.summaryBlock(s.blacklistSize(), s.cooldownSize()))
			}
		}
	}()
}

func (s *Server) logHandshakeFailure(clientAddr string, err error) {
	if err == nil {
		return
	}
	if err.Error() == errInvalidUsernamePassword && !s.cfg.Verbose {
		s.recordAuthFailure(clientAddr)
		return
	}
	if !s.cfg.Verbose {
		s.recordHandshakeFailure(clientAddr, err)
		return
	}
	s.logger.Printf("[%s] handshake failed: %v", clientAddr, err)
}

func (s *Server) recordAuthFailure(clientAddr string) {
	if s.authFails == nil {
		s.logger.Printf("[%s] handshake failed: %s", clientAddr, errInvalidUsernamePassword)
		return
	}
	s.authFails.mu.Lock()
	s.authFails.count++
	s.authFails.lastAddr = clientAddr
	s.authFails.mu.Unlock()
}

func (s *Server) flushAuthFailureSummary() {
	if s.cfg.Verbose || s.authFails == nil {
		return
	}

	s.authFails.mu.Lock()
	count := s.authFails.count
	lastAddr := s.authFails.lastAddr
	s.authFails.count = 0
	s.authFails.lastAddr = ""
	s.authFails.mu.Unlock()

	if count == 0 {
		return
	}

	s.logger.Printf("auth failures summary: %s x%d in %s last_source=%s", errInvalidUsernamePassword, count, statsLogEvery, lastAddr)
}

func classifyHandshakeFailure(err error) string {
	var hsErr *handshakeError
	if errors.As(err, &hsErr) {
		switch {
		case errors.Is(hsErr.err, io.EOF), errors.Is(hsErr.err, io.ErrUnexpectedEOF):
			switch hsErr.stage {
			case "greeting":
				return "closed_before_greeting"
			case "request":
				return "closed_before_request"
			default:
				return "closed_during_auth"
			}
		case hsErr.err != nil && hsErr.err.Error() == "unsupported socks version":
			return "invalid_version"
		}
	}
	return "other"
}

func (s *Server) recordHandshakeFailure(clientAddr string, err error) {
	if s.hsFails == nil {
		s.logger.Printf("[%s] handshake failed: %v", clientAddr, err)
		return
	}
	reason := classifyHandshakeFailure(err)
	s.hsFails.mu.Lock()
	s.hsFails.counts[reason]++
	s.hsFails.lastAddr[reason] = clientAddr
	s.hsFails.mu.Unlock()

	switch reason {
	case "closed_before_greeting":
		s.stats.incHandshakeEOF()
	case "invalid_version":
		s.stats.incHandshakeBadVersion()
	default:
		s.stats.incHandshakeOther()
	}
}

func (s *Server) flushHandshakeFailureSummary() {
	if s.cfg.Verbose || s.hsFails == nil {
		return
	}

	s.hsFails.mu.Lock()
	counts := make(map[string]int, len(s.hsFails.counts))
	lastAddrs := make(map[string]string, len(s.hsFails.lastAddr))
	for reason, count := range s.hsFails.counts {
		counts[reason] = count
	}
	for reason, addr := range s.hsFails.lastAddr {
		lastAddrs[reason] = addr
	}
	s.hsFails.counts = make(map[string]int)
	s.hsFails.lastAddr = make(map[string]string)
	s.hsFails.mu.Unlock()

	order := []string{"closed_before_greeting", "closed_before_request", "closed_during_auth", "invalid_version", "other"}
	parts := make([]string, 0, len(order))
	lastSource := ""
	for _, reason := range order {
		count := counts[reason]
		if count == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s x%d", reason, count))
		if lastSource == "" {
			lastSource = lastAddrs[reason]
		}
	}
	if len(parts) == 0 {
		return
	}
	s.logger.Printf("handshake failures summary: %s in %s last_source=%s", strings.Join(parts, ", "), statsLogEvery, lastSource)
}

func (s *Server) recordVerboseConnFailure(clientAddr, prefix string, err error) {
	if !s.cfg.Verbose || s.verboseFails == nil {
		return
	}
	key := classifyRuntimeError(prefix, err)
	s.verboseFails.mu.Lock()
	s.verboseFails.counts[key]++
	s.verboseFails.lastAddr[key] = clientAddr
	s.verboseFails.mu.Unlock()
}

func (s *Server) flushVerboseConnFailureSummary() {
	if !s.cfg.Verbose || s.verboseFails == nil {
		return
	}

	s.verboseFails.mu.Lock()
	counts := make(map[string]int, len(s.verboseFails.counts))
	lastAddrs := make(map[string]string, len(s.verboseFails.lastAddr))
	for key, count := range s.verboseFails.counts {
		counts[key] = count
	}
	for key, addr := range s.verboseFails.lastAddr {
		lastAddrs[key] = addr
	}
	s.verboseFails.counts = make(map[string]int)
	s.verboseFails.lastAddr = make(map[string]string)
	s.verboseFails.mu.Unlock()

	type item struct {
		key   string
		count int
	}
	items := make([]item, 0, len(counts))
	for key, count := range counts {
		if count > 0 {
			items = append(items, item{key: key, count: count})
		}
	}
	if len(items) == 0 {
		return
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].key < items[j].key
	})
	if len(items) > 6 {
		items = items[:6]
	}

	for _, item := range items {
		s.logger.Printf("%s x%d in %s last_source=%s", item.key, item.count, statsLogEvery, lastAddrs[item.key])
	}
}

func normalizeVerboseMessage(msg string) string {
	if strings.HasPrefix(msg, "[") {
		if idx := strings.Index(msg, "] "); idx >= 0 {
			msg = msg[idx+2:]
		}
	}
	if strings.HasPrefix(msg, "accepted connection from ") {
		return "accepted connection from <client>"
	}
	return msg
}

func (s *Server) recordVerboseDebug(msg string) {
	if !s.cfg.Verbose || s.verboseDebug == nil || msg == "" {
		return
	}
	key := normalizeVerboseMessage(msg)
	s.verboseDebug.mu.Lock()
	s.verboseDebug.counts[key]++
	if _, ok := s.verboseDebug.samples[key]; !ok {
		s.verboseDebug.samples[key] = msg
	}
	s.verboseDebug.mu.Unlock()
}

func (s *Server) flushVerboseDebugSummary() {
	if !s.cfg.Verbose || s.verboseDebug == nil {
		return
	}

	s.verboseDebug.mu.Lock()
	counts := make(map[string]int, len(s.verboseDebug.counts))
	samples := make(map[string]string, len(s.verboseDebug.samples))
	for key, count := range s.verboseDebug.counts {
		counts[key] = count
	}
	for key, sample := range s.verboseDebug.samples {
		samples[key] = sample
	}
	s.verboseDebug.counts = make(map[string]int)
	s.verboseDebug.samples = make(map[string]string)
	s.verboseDebug.mu.Unlock()

	type item struct {
		key    string
		count  int
		sample string
	}
	items := make([]item, 0, len(counts))
	for key, count := range counts {
		if count > 0 {
			items = append(items, item{key: key, count: count, sample: samples[key]})
		}
	}
	if len(items) == 0 {
		return
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].key < items[j].key
	})
	if len(items) > 12 {
		items = items[:12]
	}

	for _, item := range items {
		s.logger.Printf("%s x%d in %s", item.key, item.count, statsLogEvery)
	}
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
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(s.wsFailUntil, key)
		return false
	}
	return true
}

func (s *Server) markBlacklisted(key routeKey) {
	s.stateMu.Lock()
	s.wsBlacklist[key] = struct{}{}
	s.stateMu.Unlock()
}

func (s *Server) markFailureCooldown(key routeKey) {
	s.stateMu.Lock()
	s.wsFailUntil[key] = time.Now().Add(wsFailCooldown)
	s.stateMu.Unlock()
	s.stats.incCooldownActivations()
}

func (s *Server) clearFailureState(key routeKey) {
	s.stateMu.Lock()
	delete(s.wsFailUntil, key)
	s.stateMu.Unlock()
}

func (s *Server) blacklistSize() int {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return len(s.wsBlacklist)
}

func (s *Server) cooldownSize() int {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	now := time.Now()
	total := 0
	for key, until := range s.wsFailUntil {
		if now.After(until) {
			delete(s.wsFailUntil, key)
			continue
		}
		total++
	}
	return total
}

func fallbackReason(err error) string {
	if errors.Is(err, errWSBlacklisted) {
		return "ws-blacklisted"
	}
	return "ws-connect-failed"
}

func (s *runtimeStats) incHandshakeWait() { s.add(func() { s.handshakeWait++ }) }
func (s *runtimeStats) decHandshakeWait() {
	s.add(func() {
		if s.handshakeWait > 0 {
			s.handshakeWait--
		}
	})
}
func (s *runtimeStats) incHandshakeEOF()        { s.add(func() { s.handshakeEOF++ }) }
func (s *runtimeStats) incHandshakeBadVersion() { s.add(func() { s.handshakeBadVer++ }) }
func (s *runtimeStats) incHandshakeOther()      { s.add(func() { s.handshakeOther++ }) }
func (s *runtimeStats) incConnections()         { s.add(func() { s.connections++ }) }
func (s *runtimeStats) incWSConnections()       { s.add(func() { s.wsConnections++ }) }
func (s *runtimeStats) incTCPFallback()         { s.add(func() { s.tcpFallbacks++ }) }
func (s *runtimeStats) incHTTPRejected()        { s.add(func() { s.httpRejected++ }) }
func (s *runtimeStats) incPassthrough()         { s.add(func() { s.passthrough++ }) }
func (s *runtimeStats) incWSErrors()            { s.add(func() { s.wsErrors++ }) }
func (s *runtimeStats) incPoolHit()             { s.add(func() { s.poolHits++ }) }
func (s *runtimeStats) incPoolMiss()            { s.add(func() { s.poolMisses++ }) }
func (s *runtimeStats) incBlacklistHits()       { s.add(func() { s.blacklistHits++ }) }
func (s *runtimeStats) incCooldownActivations() { s.add(func() { s.cooldownActivs++ }) }

func (s *runtimeStats) recordWSRoute(dc int, isMedia bool) {
	s.add(func() {
		if isMedia {
			s.wsMedia++
		}
		if s.wsByDC == nil {
			s.wsByDC = make(map[int]int)
		}
		s.wsByDC[dc]++
	})
}

func (s *runtimeStats) recordTCPFallbackRoute(dc int, isMedia bool) {
	s.add(func() {
		if isMedia {
			s.tcpFallbackMedia++
		}
		if s.tcpFallbackByDC == nil {
			s.tcpFallbackByDC = make(map[int]int)
		}
		s.tcpFallbackByDC[dc]++
	})
}

func classifyRuntimeError(prefix string, err error) string {
	if err == nil {
		return prefix + "_other"
	}
	if errors.Is(err, context.Canceled) {
		return prefix + "_canceled"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return prefix + "_timeout"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "i/o timeout"):
		return prefix + "_timeout"
	case strings.Contains(msg, "no route to host"):
		return prefix + "_no_route"
	case strings.Contains(msg, "connection reset by peer"):
		return prefix + "_reset"
	case strings.Contains(msg, "EOF"):
		return prefix + "_eof"
	default:
		return prefix + "_other"
	}
}

func (s *runtimeStats) recordError(prefix string, err error) {
	s.add(func() {
		if s.errorCounts == nil {
			s.errorCounts = make(map[string]int)
		}
		s.errorCounts[classifyRuntimeError(prefix, err)]++
	})
}

func (s *runtimeStats) add(fn func()) {
	if s == nil {
		return
	}
	s.mu.Lock()
	fn()
	s.mu.Unlock()
}

func (s *runtimeStats) summary() string {
	if s == nil {
		return "disabled"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	return fmt.Sprintf(
		"hs_wait=%d hs_eof=%d hs_badver=%d hs_other=%d conn=%d ws=%d tcp_fb=%d passthrough=%d http_reject=%d ws_err=%d pool_hit=%d pool_miss=%d blacklist_hit=%d cooldown_set=%d",
		s.handshakeWait,
		s.handshakeEOF,
		s.handshakeBadVer,
		s.handshakeOther,
		s.connections,
		s.wsConnections,
		s.tcpFallbacks,
		s.passthrough,
		s.httpRejected,
		s.wsErrors,
		s.poolHits,
		s.poolMisses,
		s.blacklistHits,
		s.cooldownActivs,
	)
}

func (s *runtimeStats) summaryBlock(blacklist, cooldown int) string {
	if s == nil {
		return "stats: disabled"
	}

	s.mu.Lock()
	hsWait := s.handshakeWait
	hsEOF := s.handshakeEOF
	hsBadVer := s.handshakeBadVer
	hsOther := s.handshakeOther
	connections := s.connections
	wsConnections := s.wsConnections
	wsMedia := s.wsMedia
	tcpFallbacks := s.tcpFallbacks
	tcpFallbackMedia := s.tcpFallbackMedia
	passthrough := s.passthrough
	httpRejected := s.httpRejected
	wsErrors := s.wsErrors
	poolHits := s.poolHits
	poolMisses := s.poolMisses
	blacklistHits := s.blacklistHits
	cooldownActivs := s.cooldownActivs
	wsByDC := make(map[int]int, len(s.wsByDC))
	for dc, count := range s.wsByDC {
		wsByDC[dc] = count
	}
	fbByDC := make(map[int]int, len(s.tcpFallbackByDC))
	for dc, count := range s.tcpFallbackByDC {
		fbByDC[dc] = count
	}
	errorCounts := make(map[string]int, len(s.errorCounts))
	for key, count := range s.errorCounts {
		errorCounts[key] = count
	}
	s.mu.Unlock()

	lines := []string{
		"stats:",
		fmt.Sprintf("  handshake  wait=%d eof=%d badver=%d other=%d", hsWait, hsEOF, hsBadVer, hsOther),
		fmt.Sprintf("  routes     conn=%d ws=%d tcp_fb=%d passthrough=%d http_reject=%d", connections, wsConnections, tcpFallbacks, passthrough, httpRejected),
		fmt.Sprintf("  media      ws=%d tcp_fb=%d", wsMedia, tcpFallbackMedia),
	}

	if dcLine := summarizeDCBreakdown(wsByDC, fbByDC); dcLine != "" {
		lines = append(lines, "  dc         "+dcLine)
	}
	if probeLine := summarizeProbeCounts(errorCounts); probeLine != "" {
		lines = append(lines, "  probe      "+probeLine)
	}
	if errorLine := summarizeErrorCounts(errorCounts); errorLine != "" {
		lines = append(lines, "  errors     "+errorLine)
	}
	lines = append(lines, fmt.Sprintf("  state      ws_err=%d pool_hit=%d pool_miss=%d blacklist_hit=%d cooldown_set=%d blacklist=%d cooldown=%d", wsErrors, poolHits, poolMisses, blacklistHits, cooldownActivs, blacklist, cooldown))
	return strings.Join(lines, "\n")
}

func summarizeDCBreakdown(wsByDC, fbByDC map[int]int) string {
	parts := make([]string, 0, 2)
	if s := summarizeDCMap("ws", wsByDC); s != "" {
		parts = append(parts, s)
	}
	if s := summarizeDCMap("tcp_fb", fbByDC); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func summarizeDCMap(label string, counts map[int]int) string {
	if len(counts) == 0 {
		return ""
	}
	dcs := make([]int, 0, len(counts))
	for dc, count := range counts {
		if count > 0 {
			dcs = append(dcs, dc)
		}
	}
	if len(dcs) == 0 {
		return ""
	}
	sort.Ints(dcs)
	parts := make([]string, 0, len(dcs))
	for _, dc := range dcs {
		parts = append(parts, fmt.Sprintf("%d=%d", dc, counts[dc]))
	}
	return fmt.Sprintf("%s{%s}", label, strings.Join(parts, ", "))
}

func summarizeErrorCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	type item struct {
		key   string
		count int
	}
	items := make([]item, 0, len(counts))
	for key, count := range counts {
		if count > 0 {
			items = append(items, item{key: key, count: count})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].key < items[j].key
	})
	if len(items) > 4 {
		items = items[:4]
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s=%d", item.key, item.count))
	}
	return strings.Join(parts, ", ")
}

func summarizeProbeCounts(counts map[string]int) string {
	parts := make([]string, 0, 2)
	if s := summarizePrefixedCounts("mtproto_init", "init", counts); s != "" {
		parts = append(parts, s)
	}
	if s := summarizePrefixedCounts("passthrough", "passthrough", counts); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func summarizePrefixedCounts(prefix, label string, counts map[string]int) string {
	filtered := make(map[string]int)
	for key, count := range counts {
		if count <= 0 || !strings.HasPrefix(key, prefix+"_") {
			continue
		}
		filtered[strings.TrimPrefix(key, prefix+"_")] = count
	}
	if len(filtered) == 0 {
		return ""
	}

	type item struct {
		key   string
		count int
	}
	items := make([]item, 0, len(filtered))
	for key, count := range filtered {
		items = append(items, item{key: key, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].key < items[j].key
	})
	if len(items) > 3 {
		items = items[:3]
	}

	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s=%d", item.key, item.count))
	}
	return fmt.Sprintf("%s{%s}", label, strings.Join(parts, ", "))
}
