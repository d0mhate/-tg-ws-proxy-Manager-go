package mtpserver

import (
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"tg-ws-proxy/internal/faketls"
	"tg-ws-proxy/internal/mtproto"
	"tg-ws-proxy/internal/wsbridge"
)

type clientSession struct {
	dataConn          net.Conn
	info              mtproto.InitInfo
	clientDec         cipher.Stream
	clientEnc         cipher.Stream
	relayInit         [64]byte
	relayEnc          cipher.Stream
	relayDec          cipher.Stream
	dc                int
	directRoutes      []directRouteCandidate
	tcpFallbackTarget string
}

type websocketDialPath uint8

const (
	websocketDialDirect websocketDialPath = iota
	websocketDialCloudflare
)

func (s *MTServer) prepareClientSession(conn net.Conn) (*clientSession, bool) {
	_ = conn.SetDeadline(time.Now().Add(s.cfg.InitTimeout))

	if s.cfg.Verbose {
		s.agg.Printf("mtproto: new connection from %s", conn.RemoteAddr())
	}

	key := s.secretKey()
	dataConn, ok := s.acceptClientDataConn(conn, key)
	if !ok {
		return nil, false
	}

	handshake, ok := s.readClientHandshake(dataConn)
	if !ok {
		return nil, false
	}

	_ = conn.SetDeadline(time.Time{})

	info, err := mtproto.ParseInitWithSecret(handshake[:], key)
	if err != nil {
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: bad handshake: %v", err)
		}
		return nil, false
	}

	if s.cfg.Verbose {
		s.agg.Printf("mtproto: handshake ok dc=%d media=%v proto=%08x", info.DC, info.IsMedia, info.Proto)
	}

	clientDec, clientEnc, err := mtproto.BuildConnectionCiphers(handshake, key)
	if err != nil {
		s.logger.Printf("mtproto: cipher build: %v", err)
		return nil, false
	}

	dc := info.DC
	if dc == 0 {
		dc = 2
	}

	relayInit, relayEnc, relayDec, err := mtproto.GenerateRelayInit(info.Proto, dc)
	if err != nil {
		s.logger.Printf("mtproto: generate relay init: %v", err)
		return nil, false
	}

	directRoutes := s.directRouteCandidates(dc)
	return &clientSession{
		dataConn:          dataConn,
		info:              info,
		clientDec:         clientDec,
		clientEnc:         clientEnc,
		relayInit:         relayInit,
		relayEnc:          relayEnc,
		relayDec:          relayDec,
		dc:                dc,
		directRoutes:      directRoutes,
		tcpFallbackTarget: s.tcpFallbackTargetIP(dc, directRoutes),
	}, true
}

func (s *MTServer) acceptClientDataConn(conn net.Conn, key []byte) (net.Conn, bool) {
	if !s.isFakeTLS() {
		return conn, true
	}

	clientRandom := faketls.AcceptClientHello(conn, key)
	if clientRandom == nil {
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: FakeTLS ClientHello invalid from %s", conn.RemoteAddr())
		}
		return nil, false
	}
	if err := faketls.SendFakeServerHello(conn, key, clientRandom); err != nil {
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: FakeTLS ServerHello send error: %v", err)
		}
		return nil, false
	}
	if s.cfg.Verbose {
		s.agg.Printf("mtproto: FakeTLS handshake complete from %s", conn.RemoteAddr())
	}
	return faketls.NewConn(conn), true
}

func (s *MTServer) readClientHandshake(dataConn net.Conn) ([64]byte, bool) {
	var handshake [64]byte
	if _, err := io.ReadFull(dataConn, handshake[:]); err != nil {
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: handshake read error: %v", err)
		}
		return [64]byte{}, false
	}
	return handshake, true
}

func (s *MTServer) connectRelay(
	ctx context.Context,
	session *clientSession,
) (*wsbridge.Client, directRouteCandidate, bool) {
	ws, selectedDirectRoute, wsErr := s.connectWebsocketRelay(ctx, session)
	if wsErr == nil {
		return ws, selectedDirectRoute, true
	}

	if s.bridgeFallbackRelay(ctx, session, wsErr) {
		return nil, directRouteCandidate{}, false
	}

	return nil, directRouteCandidate{}, false
}

func (s *MTServer) connectWebsocketRelay(
	ctx context.Context,
	session *clientSession,
) (*wsbridge.Client, directRouteCandidate, error) {
	dialCtx, cancel := context.WithTimeout(ctx, s.cfg.DialTimeout)
	defer cancel()

	var (
		lastErr             error
		selectedDirectRoute directRouteCandidate
	)

	for _, path := range s.webSocketDialOrder(len(session.directRoutes) > 0, s.hasCloudflareRoutes()) {
		switch path {
		case websocketDialDirect:
			ws, route, err := s.dialDirectWSWithFallback(dialCtx, session.dc, session.info.IsMedia, session.directRoutes)
			if err == nil {
				return ws, route, nil
			}
			selectedDirectRoute = route
			lastErr = err
		case websocketDialCloudflare:
			ws, err := s.dialCloudflareWS(dialCtx, session.dc)
			if err == nil {
				return ws, selectedDirectRoute, nil
			}
			lastErr = err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no websocket relay path configured for dc=%d", session.dc)
	}
	return nil, directRouteCandidate{}, lastErr
}

func (s *MTServer) bridgeFallbackRelay(
	ctx context.Context,
	session *clientSession,
	wsErr error,
) bool {
	for _, up := range s.cfg.UpstreamProxies {
		upConn, upEnc, upDec, upErr := s.dialUpstream(ctx, up, session.dc, session.info.Proto)
		if upErr != nil {
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: upstream %s:%d failed: %v", up.Host, up.Port, upErr)
			}
			continue
		}
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: upstream %s:%d connected dc=%d", up.Host, up.Port, session.dc)
		}
		defer upConn.Close()
		s.bridgeUpstream(ctx, session.dataConn, upConn, session.clientDec, session.clientEnc, upEnc, upDec)
		return true
	}

	if session.tcpFallbackTarget != "" {
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: tcp fallback dc=%d → %s:%d", session.dc, session.tcpFallbackTarget, telegramTCPPort)
		}
		if err := s.bridgeTCPFallback(ctx, session.dataConn, session.tcpFallbackTarget, session.relayInit, session.clientDec, session.clientEnc, session.relayEnc, session.relayDec); err != nil {
			s.agg.Printf("mtproto: tcp fallback dc=%d: %v", session.dc, err)
		}
		return true
	}

	s.agg.Printf("mtproto: ws dial dc=%d: %v", session.dc, wsErr)
	return true
}

func (s *MTServer) bridgeWebsocketRelay(
	ctx context.Context,
	session *clientSession,
	ws *wsbridge.Client,
	selectedDirectRoute directRouteCandidate,
) {
	if err := ws.Send(session.relayInit[:]); err != nil {
		s.markDirectRouteBridgeFailure(session.dc, session.info.IsMedia, selectedDirectRoute)
		s.agg.Printf("mtproto: send relay init dc=%d: %v", session.dc, err)
		return
	}

	// The splitter shadows relayEnc so it can detect MTProto packet boundaries
	// in the re-encrypted byte stream and forward exactly one complete packet
	// per WebSocket frame. Without this, a partial TCP read produces a truncated
	// WebSocket frame that Telegram rejects, causing the client to reconnect.
	splitter, err := mtproto.NewSplitter(session.relayInit[:], session.info.Proto)
	if err != nil {
		s.logger.Printf("mtproto: splitter init dc=%d: %v", session.dc, err)
		return
	}

	if s.cfg.Verbose {
		s.agg.Printf("mtproto: bridge started dc=%d", session.dc)
	}

	bridgeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, gctx := errgroup.WithContext(bridgeCtx)
	go func() {
		<-gctx.Done()
		_ = ws.Close()
		_ = session.dataConn.Close()
	}()

	// Client -> Telegram: decrypt from client, re-encrypt for relay, then split
	// into complete MTProto packets before sending each as one WebSocket frame.
	g.Go(func() error {
		defer cancel()
		buf := make([]byte, s.cfg.BufferKB*1024)
		for {
			n, readErr := session.dataConn.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				session.clientDec.XORKeyStream(chunk, chunk)
				session.relayEnc.XORKeyStream(chunk, chunk)
				for _, part := range splitter.Split(chunk) {
					if sendErr := ws.Send(part); sendErr != nil {
						s.markDirectRouteBridgeFailure(session.dc, session.info.IsMedia, selectedDirectRoute)
						if s.cfg.Verbose {
							s.agg.Printf("mtproto: client→tg send error dc=%d: %v", session.dc, sendErr)
						}
						return normalizeBridgeError(gctx, sendErr)
					}
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					readErr = nil
				} else if s.cfg.Verbose {
					s.agg.Printf("mtproto: client read error dc=%d: %v", session.dc, readErr)
				}
				for _, part := range splitter.Flush() {
					_ = ws.Send(part)
				}
				return normalizeBridgeError(gctx, readErr)
			}
		}
	})

	// Telegram -> Client: decrypt from relay, re-encrypt for client.
	g.Go(func() error {
		defer cancel()
		for {
			data, recvErr := ws.Recv()
			if recvErr != nil {
				s.markDirectRouteBridgeFailure(session.dc, session.info.IsMedia, selectedDirectRoute)
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: tg→client recv error dc=%d: %v", session.dc, recvErr)
				}
				s.stats.recordRecvError(session.dc)
				return normalizeBridgeError(gctx, recvErr)
			}
			if data == nil {
				s.markDirectRouteBridgeFailure(session.dc, session.info.IsMedia, selectedDirectRoute)
				s.stats.recordClosedWS(session.dc)
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: tg closed ws dc=%d", session.dc)
				}
				return nil
			}
			session.relayDec.XORKeyStream(data, data)
			session.clientEnc.XORKeyStream(data, data)
			if _, writeErr := session.dataConn.Write(data); writeErr != nil {
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: tg→client write error dc=%d: %v", session.dc, writeErr)
				}
				return normalizeBridgeError(gctx, writeErr)
			}
		}
	})

	_ = g.Wait()
}

func (s *MTServer) hasCloudflareRoutes() bool {
	return s.cfg.UseCFProxy && len(s.cfg.CFDomains) > 0
}

func (s *MTServer) webSocketDialOrder(hasDirect, hasCloudflare bool) []websocketDialPath {
	order := make([]websocketDialPath, 0, 2)
	if hasCloudflare && s.cfg.UseCFProxyFirst {
		order = append(order, websocketDialCloudflare)
	}
	if hasDirect {
		order = append(order, websocketDialDirect)
	}
	if hasCloudflare && !s.cfg.UseCFProxyFirst {
		order = append(order, websocketDialCloudflare)
	}
	return order
}

func (s *MTServer) dialCloudflareWS(ctx context.Context, dc int) (*wsbridge.Client, error) {
	attempts := make([]string, 0, len(s.cfg.CFDomains))
	var lastErr error
	for _, cfDomain := range s.cfDomainsForConn() {
		cfHost := cfWSHost(cfDomain, dc)
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: CF dial dc=%d → %s", dc, cfHost)
		}
		ws, err := s.wsDialFunc(ctx, s.cfg, cfHost, cfHost)
		if err == nil {
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: CF connected dc=%d → %s", dc, cfHost)
			}
			return ws, nil
		}
		lastErr = err
		attempts = append(attempts, fmt.Sprintf("%s (%v)", cfHost, err))
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: CF dial failed dc=%d → %s: %v", dc, cfHost, err)
		}
	}
	if len(attempts) == 0 {
		return nil, fmt.Errorf("all CF domains failed for dc=%d", dc)
	}
	if len(attempts) == 1 {
		return nil, fmt.Errorf("all CF domains failed for dc=%d: %s", dc, attempts[0])
	}
	return nil, fmt.Errorf("all CF domains failed for dc=%d: tried %s; last error: %v", dc, strings.Join(attempts, ", "), lastErr)
}
