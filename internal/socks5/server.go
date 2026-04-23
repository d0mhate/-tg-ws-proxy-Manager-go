package socks5

import (
	"context"
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
	cfEvents     *cfEventTracker
	wsDialFunc   wsbridge.DialFunc

	proxyTCPFunc         func(ctx context.Context, conn net.Conn, host string, port int) error
	proxyTCPWithInitFunc func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error
	connectWSFunc        func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error)
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
		cfEvents: &cfEventTracker{
			failed:    make(map[string]int),
			connected: make(map[string]int),
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
		s.handleConnectRequest(ctx, conn, req, clientAddr)
	case socksCmdUDPAssociate:
		s.debugf("[%s] socks udp associate request for %s:%d", clientAddr, req.DstHost, req.DstPort)
		s.handleUDPAssociate(ctx, conn, req)
	default:
		s.logger.Printf("[%s] unsupported socks command: %d", clientAddr, req.Cmd)
		_ = writeReply(conn, 0x07)
	}
}

func (s *Server) handleConnectRequest(ctx context.Context, conn net.Conn, req request, clientAddr string) {
	if req.DstHost == "" || req.DstPort <= 0 {
		_ = writeReply(conn, 0x05)
		return
	}

	isTelegramCandidate, shouldProbeMTProto, isIPv6 := classifyInitialRoute(req)
	routeByInitOnly := false

	if !isTelegramCandidate && !shouldProbeMTProto {
		s.runPassthrough(ctx, conn, req.DstHost, req.DstPort, clientAddr)
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
			s.runProbeReadPassthrough(ctx, conn, req.DstHost, req.DstPort, init, n, err, clientAddr)
			return
		}
		s.stats.recordError("mtproto_init", err)
		s.recordVerboseConnFailure(clientAddr, "mtproto_init", err)
		return
	}

	if !isTelegramCandidate {
		_, inferred, info, parseErr := inferTelegramCandidateFromInit(req, init)
		if parseErr != nil {
			s.runPassthroughWithInit(ctx, conn, req.DstHost, req.DstPort, init, clientAddr, "mtproto-probe-miss")
			return
		}
		isTelegramCandidate = inferred
		routeByInitOnly = inferred
		s.debugf("[%s] telegram route inferred from mtproto init on destination %s:%d dc=%d media=%v", clientAddr, req.DstHost, req.DstPort, info.DC, info.IsMedia)
	}

	if mtproto.IsHTTPTransport(init) {
		if routeByInitOnly {
			s.runPassthroughWithInit(ctx, conn, req.DstHost, req.DstPort, init, clientAddr, "http-probe")
			return
		}
		s.runTCPFallbackWithInit(ctx, conn, req.DstHost, req.DstPort, init, 0, false, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=http-transport destination=%s:%d", clientAddr, req.DstHost, req.DstPort)
		})
		return
	}

	plan := s.buildTelegramRoutePlan(req, init, isIPv6, routeByInitOnly, clientAddr)
	init = plan.init

	if plan.targetIP == "" {
		s.runTCPFallbackWithInit(ctx, conn, req.DstHost, req.DstPort, init, plan.effectiveDC, plan.isMedia, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=no-dc-override dc=%d effective_dc=%d destination=%s:%d", clientAddr, plan.dc, plan.effectiveDC, req.DstHost, req.DstPort)
		})
		return
	}

	if !isWSEnabledDC(plan.wsDomainDC) {
		s.runTCPFallbackWithInit(ctx, conn, plan.fallbackHost, req.DstPort, init, plan.effectiveDC, plan.isMedia, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=ws-disabled-dc dc=%d effective_dc=%d ws_dc=%d target=%s", clientAddr, plan.dc, plan.effectiveDC, plan.wsDomainDC, plan.targetIP)
		})
		return
	}

	ws, err := s.connectTelegramThenCloudflareWS(ctx, clientAddr, plan.dc, plan.effectiveDC, plan.isMedia, plan.targetIP)
	if err != nil {
		s.runTCPFallbackWithInit(ctx, conn, plan.fallbackHost, req.DstPort, init, plan.effectiveDC, plan.isMedia, clientAddr, func() {
			s.debugf("[%s] route=tcp-fallback reason=%s dc=%d effective_dc=%d target=%s", clientAddr, fallbackReason(err), plan.dc, plan.effectiveDC, plan.targetIP)
		})
		return
	}
	defer ws.Close()
	s.stats.incWSConnections()
	s.stats.recordWSRoute(plan.effectiveDC, plan.isMedia)
	s.debugf("[%s] route=websocket dc=%d effective_dc=%d ws_dc=%d media=%v target=%s", clientAddr, plan.dc, plan.effectiveDC, plan.wsDomainDC, plan.isMedia, plan.targetIP)

	var splitter *mtproto.Splitter
	if plan.proto != 0 && (plan.initPatched || plan.isMedia || plan.proto != mtproto.ProtoIntermediate) {
		splitter, _ = mtproto.NewSplitter(init, plan.proto)
		if splitter != nil {
			s.debugf("[%s] websocket splitter enabled for proto=0x%08x", clientAddr, plan.proto)
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

func (s *Server) connectTelegramThenCloudflareWS(ctx context.Context, clientAddr string, dc int, effectiveDC int, isMedia bool, targetIP string) (*wsbridge.Client, error) {
	tryCloudflare := s.cfg.UseCFProxy && len(s.cfg.CFDomains) > 0

	tryBridgeCF := func() (*wsbridge.Client, error) {
		s.debugf("[%s] cloudflare websocket attempt: dc=%d effective_dc=%d media=%v domains=%v", clientAddr, dc, effectiveDC, isMedia, s.cfg.CFDomains)
		cfWS, cfErr := s.connectWSCF(ctx, dc, isMedia)
		if cfErr != nil {
			s.stats.recordError("ws_cf_connect", cfErr)
			s.recordVerboseConnFailure(clientAddr, "ws_cf_connect", cfErr)
			s.recordCFEvent(clientAddr, effectiveDC, isMedia, cfErr)
			s.debugf("[%s] cloudflare websocket failed: %v", clientAddr, cfErr)
			return nil, cfErr
		}
		s.recordCFEvent(clientAddr, effectiveDC, isMedia, nil)
		return cfWS, nil
	}

	tryTelegram := func() (*wsbridge.Client, error) {
		ws, err := s.connectWS(ctx, targetIP, effectiveDC, isMedia)
		if err != nil {
			s.stats.recordError("ws_connect", err)
			s.recordVerboseConnFailure(clientAddr, "ws_connect", err)
			return nil, err
		}
		return ws, nil
	}

	if tryCloudflare && s.cfg.UseCFProxyFirst {
		if ws, err := tryBridgeCF(); err == nil {
			s.debugf("[%s] route=websocket primary=cloudflare dc=%d effective_dc=%d media=%v", clientAddr, dc, effectiveDC, isMedia)
			return ws, nil
		}
	}

	ws, err := tryTelegram()
	if err == nil {
		return ws, nil
	}

	if tryCloudflare && !s.cfg.UseCFProxyFirst {
		if cfWS, cfErr := tryBridgeCF(); cfErr == nil {
			s.debugf("[%s] route=websocket fallback=cloudflare dc=%d effective_dc=%d media=%v", clientAddr, dc, effectiveDC, isMedia)
			return cfWS, nil
		}
	}

	if tryCloudflare && s.cfg.UseCFProxyFirst {
		s.debugf("[%s] cloudflare-first fallback exhausted, telegram websocket failed: %v", clientAddr, err)
	}
	return nil, err
}

func (s *Server) connectWSCF(ctx context.Context, dc int, isMedia bool) (*wsbridge.Client, error) {
	var lastErr error
	for _, domain := range s.cfg.CFDomains {
		cfHost := telegram.CFWSDomain(domain, dc)
		s.debugf("ws cf attempt: dc=%d media=%v cf_host=%s", dc, isMedia, cfHost)
		dialCfg := s.cfg
		ws, err := s.wsDialFunc(ctx, dialCfg, cfHost, cfHost)
		if err == nil {
			return ws, nil
		}
		s.stats.incWSErrors()
		lastErr = err
	}
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
