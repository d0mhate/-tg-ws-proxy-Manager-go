package socks5

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"tg-ws-proxy/internal/cfbalance"
	"tg-ws-proxy/internal/config"
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
	maxConcurrentConns   = 4096
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
	cfBalancer           *cfbalance.Balancer
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
		cfBalancer: &cfbalance.Balancer{},
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

	sem := make(chan struct{}, maxConcurrentConns)
	for {
		select {
		case <-ctx.Done():
			return nil
		case sem <- struct{}{}:
		}

		conn, err := ln.Accept()
		if err != nil {
			<-sem
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		s.tuneConn(conn)
		s.debugf("accepted connection from %s", conn.RemoteAddr())
		go func(conn net.Conn) {
			defer func() { <-sem }()
			s.handleConn(ctx, conn)
		}(conn)
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

	initResult, handled := s.readAndClassifyInit(ctx, conn, req, clientAddr, isTelegramCandidate)
	if handled {
		return
	}
	init := initResult.init
	routeByInitOnly = initResult.routeByInitOnly

	plan := s.buildTelegramRoutePlan(req, init, isIPv6, routeByInitOnly, clientAddr)
	init = plan.init

	ws, handled := s.tryTelegramWebsocketRoute(ctx, conn, req, plan, init, clientAddr)
	if handled {
		return
	}
	s.bridgeWebsocketRoute(ctx, conn, ws, plan, init, clientAddr)
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

	domains := websocketDomainsForDC(dc, isMedia)
	if ws, ok := s.tryPooledWebsocket(dc, isMedia, targetIP, domains); ok {
		return ws, nil
	}

	dialCfg := s.websocketDialConfig(dc, isMedia, key)
	result := s.dialWebsocketDomains(ctx, dialCfg, key, targetIP, dc, isMedia, domains)
	if result.ws != nil {
		return result.ws, nil
	}
	return nil, s.finalizeWebsocketDialFailure(key, result)
}

func (s *Server) connectTelegramThenCloudflareWS(ctx context.Context, clientAddr string, dc int, effectiveDC int, isMedia bool, targetIP string, allowTelegramWS bool) (*wsbridge.Client, error) {
	tryCloudflare := s.cfg.UseCFProxy && len(s.cfg.CFDomains) > 0
	cfDomains := s.cfDomainsForConn()
	var lastErr error

	tryBridgeCF := func() (*wsbridge.Client, error) {
		s.debugf("[%s] cloudflare websocket attempt: dc=%d effective_dc=%d media=%v domains=%v", clientAddr, dc, effectiveDC, isMedia, cfDomains)
		cfWS, cfErr := s.connectWSCF(ctx, dc, isMedia, cfDomains)
		if cfErr != nil {
			s.stats.recordError("ws_cf_connect", cfErr)
			s.recordVerboseConnFailure(clientAddr, "ws_cf_connect", cfErr)
			s.recordCFEvent(clientAddr, effectiveDC, isMedia, cfErr)
			s.debugf("[%s] cloudflare websocket failed: %v", clientAddr, cfErr)
			lastErr = cfErr
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
			lastErr = err
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

	if allowTelegramWS {
		ws, err := tryTelegram()
		if err == nil {
			return ws, nil
		}
	}

	if tryCloudflare && !s.cfg.UseCFProxyFirst {
		if cfWS, cfErr := tryBridgeCF(); cfErr == nil {
			s.debugf("[%s] route=websocket fallback=cloudflare dc=%d effective_dc=%d media=%v", clientAddr, dc, effectiveDC, isMedia)
			return cfWS, nil
		}
	}

	if tryCloudflare && s.cfg.UseCFProxyFirst && allowTelegramWS {
		s.debugf("[%s] cloudflare-first fallback exhausted, telegram websocket failed: %v", clientAddr, lastErr)
	}
	return nil, lastErr
}

func (s *Server) cfDomainsForConn() []string {
	return s.cfBalancer.Domains(s.cfg.CFDomains, s.cfg.UseCFBalance)
}

func (s *Server) connectWSCF(ctx context.Context, dc int, isMedia bool, domains []string) (*wsbridge.Client, error) {
	var lastErr error
	for _, domain := range domains {
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
	bridgeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, gctx := errgroup.WithContext(bridgeCtx)
	go func() {
		<-gctx.Done()
		_ = a.Close()
		_ = b.Close()
	}()

	g.Go(func() error {
		defer cancel()
		_, err := io.Copy(b, a)
		return normalizeBridgeCopyError(gctx, normalizeEOF(err))
	})
	g.Go(func() error {
		defer cancel()
		_, err := io.Copy(a, b)
		return normalizeBridgeCopyError(gctx, normalizeEOF(err))
	})

	if err := g.Wait(); err != nil {
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return ctx.Err()
}

func normalizeBridgeCopyError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil && (errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled)) {
		return nil
	}
	return err
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
