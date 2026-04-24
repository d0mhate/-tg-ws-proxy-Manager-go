package mtpserver

import (
	"context"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"time"

	"tg-ws-proxy/internal/cfbalance"
	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/faketls"
	"tg-ws-proxy/internal/mtproto"
	"tg-ws-proxy/internal/wsbridge"
)

// mtproto listener that accepts obfuscated client connections and bridges them
// to tg over websocket, re-encrypting both directions.
type MTServer struct {
	cfg            config.Config
	secret         []byte
	logger         *log.Logger
	agg            *aggLogger // aggregates repeated verbose lines within 2s
	pool           *wsbridge.Pool
	stats          *statsCollector
	routeCooldowns *routeCooldowns
	wsDialFunc     wsbridge.DialFunc
	cfBalancer     *cfbalance.Balancer
}

func NewMTServer(cfg config.Config, secret []byte, logger *log.Logger) *MTServer {
	srv := &MTServer{
		cfg:            cfg,
		secret:         secret,
		logger:         logger,
		agg:            newAggLogger(logger, 2*time.Second),
		stats:          newStatsCollector(),
		routeCooldowns: newRouteCooldowns(30*time.Second, 60*time.Second, 5*time.Minute),
		pool:           wsbridge.NewPool(cfg),
		wsDialFunc:     wsbridge.Dial,
		cfBalancer:     &cfbalance.Balancer{},
	}
	if srv.pool != nil {
		srv.pool.SetDialFunc(srv.wsDialFunc)
	}
	return srv
}

// get the 16-byte key from the configured secret.
// plain secrets are used as-is, dd/ee secrets use bytes[1:17].
func (s *MTServer) secretKey() []byte {
	if len(s.secret) >= 17 && (s.secret[0] == 0xdd || s.secret[0] == 0xee) {
		return s.secret[1:17]
	}
	return s.secret
}

// true for 0xee faketls secrets.
func (s *MTServer) isFakeTLS() bool {
	return len(s.secret) >= 17 && s.secret[0] == 0xee
}

func (s *MTServer) cfDomainsForConn() []string {
	return s.cfBalancer.Domains(s.cfg.CFDomains, s.cfg.UseCFBalance)
}

func (s *MTServer) Run(ctx context.Context) error {
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

	s.logger.Printf("mtproto proxy listening on %s", addr)
	if s.pool != nil {
		s.pool.Warmup(s.cfg.DCIPs)
	}
	if s.cfg.Verbose && s.stats != nil {
		go s.stats.run(ctx.Done(), s.routeCooldowns, 15*time.Second, s.agg.Printf)
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
		go s.handleConn(ctx, conn)
	}
}

func (s *MTServer) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(s.cfg.InitTimeout))

	if s.cfg.Verbose {
		s.agg.Printf("mtproto: new connection from %s", conn.RemoteAddr())
	}

	key := s.secretKey()

	// ee secrets start with faketls. after the fake handshake, traffic is
	// wrapped in tls appdata records.
	var dataConn net.Conn = conn
	if s.isFakeTLS() {
		clientRandom := faketls.AcceptClientHello(conn, key)
		if clientRandom == nil {
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: FakeTLS ClientHello invalid from %s", conn.RemoteAddr())
			}
			go io.Copy(io.Discard, conn)
			return
		}
		if err := faketls.SendFakeServerHello(conn, key, clientRandom); err != nil {
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: FakeTLS ServerHello send error: %v", err)
			}
			return
		}
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: FakeTLS handshake complete from %s", conn.RemoteAddr())
		}
		dataConn = faketls.NewConn(conn)
	}

	var handshake [64]byte
	if _, err := io.ReadFull(dataConn, handshake[:]); err != nil {
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: handshake read error: %v", err)
		}
		return
	}

	_ = conn.SetDeadline(time.Time{})

	info, err := mtproto.ParseInitWithSecret(handshake[:], key)
	if err != nil {
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: bad handshake: %v", err)
		}
		// wrong secret: drain silently so scanners get less signal.
		go io.Copy(io.Discard, conn)
		return
	}

	if s.cfg.Verbose {
		s.agg.Printf("mtproto: handshake ok dc=%d media=%v proto=%08x", info.DC, info.IsMedia, info.Proto)
	}

	clientDec, clientEnc, err := mtproto.BuildConnectionCiphers(handshake, key)
	if err != nil {
		s.logger.Printf("mtproto: cipher build: %v", err)
		return
	}

	dc := info.DC
	if dc == 0 {
		dc = 2
	}
	directRoutes := s.directRouteCandidates(dc)
	hasCF := s.cfg.UseCFProxy && len(s.cfg.CFDomains) > 0

	relayInit, relayEnc, relayDec, err := mtproto.GenerateRelayInit(info.Proto, dc)
	if err != nil {
		s.logger.Printf("mtproto: generate relay init: %v", err)
		return
	}

	tcpFallbackTarget := s.tcpFallbackTargetIP(dc, directRoutes)

	dialCtx, cancel := context.WithTimeout(ctx, s.cfg.DialTimeout)
	defer cancel()

	var selectedDirectRoute directRouteCandidate

	dialCF := func() (*wsbridge.Client, error) {
		for _, cfDomain := range s.cfDomainsForConn() {
			cfHost := cfWSHost(cfDomain, dc)
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: CF dial dc=%d → %s", dc, cfHost)
			}
			ws, err := wsbridge.Dial(dialCtx, s.cfg, cfHost, cfHost)
			if err == nil {
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: CF connected dc=%d → %s", dc, cfHost)
				}
				return ws, nil
			}
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: CF dial failed dc=%d → %s: %v", dc, cfHost, err)
			}
		}
		return nil, fmt.Errorf("all CF domains failed for dc=%d", dc)
	}

	dialDirect := func() (*wsbridge.Client, error) {
		ws, route, err := s.dialDirectWSWithFallback(dialCtx, dc, info.IsMedia, directRoutes)
		selectedDirectRoute = route
		return ws, err
	}

	var ws *wsbridge.Client
	var wsErr error

	if hasCF && s.cfg.UseCFProxyFirst {
		ws, wsErr = dialCF()
		if wsErr != nil {
			if len(directRoutes) > 0 {
				ws, wsErr = dialDirect()
			}
		}
	} else {
		if len(directRoutes) > 0 {
			ws, wsErr = dialDirect()
		} else {
			wsErr = fmt.Errorf("no direct route configured for dc=%d", dc)
		}
		if wsErr != nil && hasCF {
			ws, wsErr = dialCF()
		}
	}

	if wsErr != nil {
		// websocket failed, try upstream mtproto proxies.
		for _, up := range s.cfg.UpstreamProxies {
			upConn, upEnc, upDec, upErr := s.dialUpstream(ctx, up, dc, info.Proto)
			if upErr != nil {
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: upstream %s:%d failed: %v", up.Host, up.Port, upErr)
				}
				continue
			}
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: upstream %s:%d connected dc=%d", up.Host, up.Port, dc)
			}
			defer upConn.Close()
			s.bridgeUpstream(ctx, dataConn, upConn, clientDec, clientEnc, upEnc, upDec)
			return
		}
		if tcpFallbackTarget != "" {
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: tcp fallback dc=%d → %s:%d", dc, tcpFallbackTarget, telegramTCPPort)
			}
			if err := s.bridgeTCPFallback(ctx, dataConn, tcpFallbackTarget, relayInit, clientDec, clientEnc, relayEnc, relayDec); err != nil {
				s.agg.Printf("mtproto: tcp fallback dc=%d: %v", dc, err)
				return
			}
			return
		}
		s.agg.Printf("mtproto: ws dial dc=%d: %v", dc, wsErr)
		return
	}
	defer ws.Close()

	if err := ws.Send(relayInit[:]); err != nil {
		s.markDirectRouteBridgeFailure(dc, info.IsMedia, selectedDirectRoute)
		s.agg.Printf("mtproto: send relay init dc=%d: %v", dc, err)
		return
	}

	// The splitter shadows relayEnc so it can detect MTProto packet boundaries
	// in the re-encrypted byte stream and forward exactly one complete packet
	// per WebSocket frame.  Without this, a partial TCP read produces a truncated
	// WebSocket frame that Telegram rejects, causing the client to reconnect.
	splitter, err := mtproto.NewSplitter(relayInit[:], info.Proto)
	if err != nil {
		s.logger.Printf("mtproto: splitter init dc=%d: %v", dc, err)
		return
	}

	if s.cfg.Verbose {
		s.agg.Printf("mtproto: bridge started dc=%d", dc)
	}

	errCh := make(chan error, 2)

	// Client → Telegram: decrypt from client, re-encrypt for relay, then split
	// into complete MTProto packets before sending each as one WebSocket frame.
	go func() {
		buf := make([]byte, s.cfg.BufferKB*1024)
		for {
			n, readErr := dataConn.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				clientDec.XORKeyStream(chunk, chunk)
				relayEnc.XORKeyStream(chunk, chunk)
				for _, part := range splitter.Split(chunk) {
					if sendErr := ws.Send(part); sendErr != nil {
						s.markDirectRouteBridgeFailure(dc, info.IsMedia, selectedDirectRoute)
						if s.cfg.Verbose {
							s.agg.Printf("mtproto: client→tg send error dc=%d: %v", dc, sendErr)
						}
						errCh <- sendErr
						return
					}
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					readErr = nil
				} else if s.cfg.Verbose {
					s.agg.Printf("mtproto: client read error dc=%d: %v", dc, readErr)
				}
				for _, part := range splitter.Flush() {
					_ = ws.Send(part)
				}
				errCh <- readErr
				return
			}
		}
	}()

	// Telegram → Client: decrypt from relay, re-encrypt for client.
	go func() {
		for {
			data, recvErr := ws.Recv()
			if recvErr != nil {
				s.markDirectRouteBridgeFailure(dc, info.IsMedia, selectedDirectRoute)
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: tg→client recv error dc=%d: %v", dc, recvErr)
				}
				s.stats.recordRecvError(dc)
				errCh <- recvErr
				return
			}
			if data == nil {
				s.markDirectRouteBridgeFailure(dc, info.IsMedia, selectedDirectRoute)
				s.stats.recordClosedWS(dc)
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: tg closed ws dc=%d", dc)
				}
				errCh <- nil
				return
			}
			relayDec.XORKeyStream(data, data)
			clientEnc.XORKeyStream(data, data)
			if _, writeErr := dataConn.Write(data); writeErr != nil {
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: tg→client write error dc=%d: %v", dc, writeErr)
				}
				errCh <- writeErr
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-errCh:
	}
}

// connect to one upstream mtproto proxy and do its handshake.
// returns the data conn plus upstream enc/dec ciphers.
//
// secret format:
// - 32 chars: plain
// - 34 chars with 0xdd: padded intermediate, key = bytes[1:17]
// - 34+ chars with 0xee: faketls, key = bytes[1:17], hostname = bytes[17:]
func (s *MTServer) dialUpstream(
	ctx context.Context,
	up config.UpstreamProxy,
	dc int,
	proto uint32,
) (net.Conn, cipher.Stream, cipher.Stream, error) {
	raw, err := hex.DecodeString(up.Secret)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("upstream secret hex: %w", err)
	}

	isFakeTLS := len(raw) > 17 && raw[0] == 0xee
	var key []byte
	if len(raw) >= 17 && (raw[0] == 0xdd || raw[0] == 0xee) {
		key = raw[1:17]
	} else {
		key = raw
	}

	addr := net.JoinHostPort(up.Host, strconv.Itoa(up.Port))
	dialCtx, cancel := context.WithTimeout(ctx, s.cfg.DialTimeout)
	defer cancel()

	tcpConn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("upstream dial %s: %w", addr, err)
	}

	handshake, upEnc, upDec, err := mtproto.GenerateClientHandshake(key, dc, proto)
	if err != nil {
		tcpConn.Close()
		return nil, nil, nil, fmt.Errorf("upstream handshake gen: %w", err)
	}

	if isFakeTLS {
		hostname := string(raw[17:])
		hello := faketls.BuildClientHello(hostname)
		faketls.SignClientHello(hello, key)

		if _, err := tcpConn.Write(hello); err != nil {
			tcpConn.Close()
			return nil, nil, nil, fmt.Errorf("upstream FakeTLS ClientHello: %w", err)
		}
		if !faketls.DrainServerHello(tcpConn) {
			tcpConn.Close()
			return nil, nil, nil, fmt.Errorf("upstream FakeTLS server handshake failed")
		}

		ftConn := faketls.NewConn(tcpConn)
		if _, err := ftConn.Write(handshake[:]); err != nil {
			tcpConn.Close()
			return nil, nil, nil, fmt.Errorf("upstream FakeTLS send init: %w", err)
		}
		return ftConn, upEnc, upDec, nil
	}

	// plain and dd send the handshake as raw bytes.
	if _, err := tcpConn.Write(handshake[:]); err != nil {
		tcpConn.Close()
		return nil, nil, nil, fmt.Errorf("upstream plain send init: %w", err)
	}
	return tcpConn, upEnc, upDec, nil
}

// bridge client <-> upstream with re-encryption on both sides.
func (s *MTServer) bridgeUpstream(
	ctx context.Context,
	client net.Conn,
	upstream net.Conn,
	clientDec, clientEnc cipher.Stream,
	upEnc, upDec cipher.Stream,
) {
	s.bridgeRelay(ctx, client, upstream, clientDec, clientEnc, upEnc, upDec)
}
