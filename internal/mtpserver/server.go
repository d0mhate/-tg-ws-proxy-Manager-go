package mtpserver

import (
	"context"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
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

const maxConns = 4096

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

func (s *MTServer) cfDomainsForConn(dc int) []string {
	return s.cfBalancer.DomainsForDC(dc, s.cfg.CFDomains, s.cfg.UseCFBalance)
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
	defer func() {
		if s.agg != nil {
			s.agg.Flush()
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

	sem := make(chan struct{}, maxConns)
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
		go func(conn net.Conn) {
			defer func() { <-sem }()
			s.handleConn(ctx, conn)
		}(conn)
	}
}

func (s *MTServer) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	session, ok := s.prepareClientSession(conn)
	if !ok {
		return
	}

	ws, selectedDirectRoute, ok := s.connectRelay(ctx, session)
	if !ok {
		return
	}
	defer ws.Close()

	s.bridgeWebsocketRelay(ctx, session, ws, selectedDirectRoute)
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
