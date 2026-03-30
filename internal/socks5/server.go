package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
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
	statsLogEvery        = 30 * time.Second
)

var wsEnabledDCs = map[int]struct{}{
	2: {},
	4: {},
}

type Server struct {
	cfg    config.Config
	logger *log.Logger
	pool   *wsbridge.Pool

	stateMu     sync.Mutex
	wsBlacklist map[routeKey]struct{}
	wsFailUntil map[routeKey]time.Time
	stats       *runtimeStats
	wsDialFunc  wsbridge.DialFunc

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
	mu             sync.Mutex
	connections    int
	wsConnections  int
	tcpFallbacks   int
	httpRejected   int
	passthrough    int
	wsErrors       int
	poolHits       int
	poolMisses     int
	blacklistHits  int
	cooldownActivs int
}

func NewServer(cfg config.Config, logger *log.Logger) *Server {
	srv := &Server{
		cfg:         cfg,
		logger:      logger,
		pool:        wsbridge.NewPool(cfg),
		wsBlacklist: make(map[routeKey]struct{}),
		wsFailUntil: make(map[routeKey]time.Time),
		stats:       &runtimeStats{},
		wsDialFunc:  wsbridge.Dial,
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

	req, err := handshake(conn, s.cfg)
	if err != nil {
		s.logger.Printf("[%s] handshake failed: %v", clientAddr, err)
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
			s.logger.Printf("[%s] passthrough failed: %v", clientAddr, err)
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
					s.logger.Printf("[%s] passthrough failed: %v", clientAddr, ptErr)
				}
				return
			}
			if ptErr := s.proxyTCPWithInit(ctx, conn, req.DstHost, req.DstPort, init[:n]); ptErr != nil && !errors.Is(ptErr, io.EOF) {
				s.logger.Printf("[%s] passthrough failed: %v", clientAddr, ptErr)
			}
			return
		}
		s.logger.Printf("[%s] failed to read mtproto init: %v", clientAddr, err)
		return
	}

	if !isTelegramCandidate {
		info, parseErr := mtproto.ParseInit(init)
		if parseErr != nil {
			s.stats.incPassthrough()
			s.debugf("[%s] route=passthrough reason=mtproto-probe-miss destination=%s:%d", clientAddr, req.DstHost, req.DstPort)
			if ptErr := s.proxyTCPWithInit(ctx, conn, req.DstHost, req.DstPort, init); ptErr != nil && !errors.Is(ptErr, io.EOF) {
				s.logger.Printf("[%s] passthrough failed: %v", clientAddr, ptErr)
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
				s.logger.Printf("[%s] passthrough failed: %v", clientAddr, ptErr)
			}
			return
		}
		s.stats.incTCPFallback()
		s.debugf("[%s] route=tcp-fallback reason=http-transport destination=%s:%d", clientAddr, req.DstHost, req.DstPort)
		if err := s.proxyTCPWithInit(ctx, conn, req.DstHost, req.DstPort, init); err != nil && !errors.Is(err, io.EOF) {
			s.logger.Printf("[%s] tcp fallback failed: %v", clientAddr, err)
		}
		return
	}

	info, err := mtproto.ParseInit(init)
	if err != nil && !errors.Is(err, mtproto.ErrInvalidProto) {
		s.logger.Printf("[%s] mtproto init parse failed: %v", clientAddr, err)
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

	effectiveDC := telegram.NormalizeDC(dc)
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
		s.debugf("[%s] route=tcp-fallback reason=no-dc-override dc=%d effective_dc=%d destination=%s:%d", clientAddr, dc, effectiveDC, req.DstHost, req.DstPort)
		if err := s.proxyTCPWithInit(ctx, conn, req.DstHost, req.DstPort, init); err != nil && !errors.Is(err, io.EOF) {
			s.logger.Printf("[%s] tcp fallback failed: %v", clientAddr, err)
		}
		return
	}

	fallbackHost := req.DstHost
	if isIPv6 || effectiveDC != dc || routeByInitOnly || inferredFromDestination {
		fallbackHost = targetIP
		s.debugf("[%s] telegram route will fallback via dc target %s", clientAddr, targetIP)
	}

	if !isWSEnabledDC(effectiveDC) {
		s.stats.incTCPFallback()
		s.debugf("[%s] route=tcp-fallback reason=ws-disabled-dc dc=%d effective_dc=%d target=%s", clientAddr, dc, effectiveDC, targetIP)
		if err := s.proxyTCPWithInit(ctx, conn, fallbackHost, req.DstPort, init); err != nil && !errors.Is(err, io.EOF) {
			s.logger.Printf("[%s] tcp fallback failed: %v", clientAddr, err)
		}
		return
	}

	ws, err := s.connectWS(ctx, targetIP, effectiveDC, isMedia)
	if err != nil {
		s.logger.Printf("[%s] ws connect failed for DC%d via %s: %v", clientAddr, effectiveDC, targetIP, err)
		s.stats.incTCPFallback()
		s.debugf("[%s] route=tcp-fallback reason=%s dc=%d effective_dc=%d target=%s", clientAddr, fallbackReason(err), dc, effectiveDC, targetIP)
		if fbErr := s.proxyTCPWithInit(ctx, conn, fallbackHost, req.DstPort, init); fbErr != nil && !errors.Is(fbErr, io.EOF) {
			s.logger.Printf("[%s] tcp fallback failed: %v", clientAddr, fbErr)
		}
		return
	}
	defer ws.Close()
	s.stats.incWSConnections()
	s.debugf("[%s] route=websocket dc=%d effective_dc=%d media=%v target=%s", clientAddr, dc, effectiveDC, isMedia, targetIP)

	var splitter *mtproto.Splitter
	if proto != 0 && (initPatched || isMedia || proto != mtproto.ProtoIntermediate) {
		splitter, _ = mtproto.NewSplitter(init, proto)
		if splitter != nil {
			s.debugf("[%s] websocket splitter enabled for proto=0x%08x", clientAddr, proto)
		}
	}

	if err := wsbridge.Bridge(ctx, conn, ws, init, splitter); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		s.logger.Printf("[%s] ws bridge ended with error: %v", clientAddr, err)
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

	domains := telegram.WSDomains(dc, isMedia)
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
		return req, err
	}
	if buf[0] != 0x05 {
		return req, errors.New("unsupported socks version")
	}

	nMethods := int(buf[1])
	if nMethods == 0 {
		return req, errors.New("no auth methods provided")
	}
	if _, err := io.ReadFull(conn, buf[:nMethods]); err != nil {
		return req, err
	}
	method, err := negotiateAuthMethod(buf[:nMethods], cfg)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0xff})
		return req, err
	}
	if _, err := conn.Write([]byte{0x05, method}); err != nil {
		return req, err
	}
	if method == socksAuthUserPass {
		if err := authenticateUserPass(conn, cfg.Username, cfg.Password); err != nil {
			return req, err
		}
	}

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return req, err
	}
	if buf[0] != 0x05 {
		return req, errors.New("unsupported socks version")
	}
	req.Cmd = buf[1]
	if req.Cmd != socksCmdConnect && req.Cmd != socksCmdUDPAssociate {
		return req, errors.New("only connect and udp associate are supported")
	}

	switch buf[3] {
	case 0x01:
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return req, err
		}
		req.DstHost = net.IP(buf[:4]).String()
	case 0x03:
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return req, err
		}
		size := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:size]); err != nil {
			return req, err
		}
		req.DstHost = string(buf[:size])
	case 0x04:
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return req, err
		}
		req.DstHost = net.IP(buf[:16]).String()
	default:
		return req, errors.New("address type not supported")
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return req, err
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
	s.logger.Printf(format, args...)
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
				s.logger.Printf("stats: %s blacklist=%d cooldown=%d", s.stats.summary(), s.blacklistSize(), s.cooldownSize())
				return
			case <-ticker.C:
				s.logger.Printf("stats: %s blacklist=%d cooldown=%d", s.stats.summary(), s.blacklistSize(), s.cooldownSize())
			}
		}
	}()
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
		"conn=%d ws=%d tcp_fb=%d passthrough=%d http_reject=%d ws_err=%d pool_hit=%d pool_miss=%d blacklist_hit=%d cooldown_set=%d",
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
