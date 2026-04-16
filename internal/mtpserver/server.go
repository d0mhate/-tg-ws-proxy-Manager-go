package mtpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/mtproto"
	"tg-ws-proxy/internal/telegram"
	"tg-ws-proxy/internal/wsbridge"
)

// MTServer listens for raw MTProto obfuscated connections and proxies them to
// Telegram via WebSocket, re-encrypting traffic on both sides.
type MTServer struct {
	cfg    config.Config
	secret []byte
	logger *log.Logger
	agg    *aggLogger // aggregates repeated verbose lines within 2s
}

func NewMTServer(cfg config.Config, secret []byte, logger *log.Logger) *MTServer {
	return &MTServer{
		cfg:    cfg,
		secret: secret,
		logger: logger,
		agg:    newAggLogger(logger, 2*time.Second),
	}
}

func (s *MTServer) Run(ctx context.Context) error {
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	s.logger.Printf("mtproto proxy listening on %s", addr)

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

	var handshake [64]byte
	if _, err := io.ReadFull(conn, handshake[:]); err != nil {
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: handshake read error: %v", err)
		}
		return
	}

	_ = conn.SetDeadline(time.Time{})

	info, err := mtproto.ParseInitWithSecret(handshake[:], s.secret)
	if err != nil {
		if s.cfg.Verbose {
			s.agg.Printf("mtproto: bad handshake: %v", err)
		}
		// Wrong secret - drain silently to defeat port scanners.
		go io.Copy(io.Discard, conn)
		return
	}

	if s.cfg.Verbose {
		s.agg.Printf("mtproto: handshake ok dc=%d media=%v proto=%08x", info.DC, info.IsMedia, info.Proto)
	}

	clientDec, clientEnc, err := mtproto.BuildConnectionCiphers(handshake, s.secret)
	if err != nil {
		s.logger.Printf("mtproto: cipher build: %v", err)
		return
	}

	dc := info.DC
	if dc == 0 {
		dc = 2
	}
	effectiveDC := telegram.NormalizeDC(dc)

	targetIP := s.cfg.DCIPs[effectiveDC]
	hasCF := s.cfg.UseCFProxy && len(s.cfg.CFDomains) > 0

	if targetIP == "" && !hasCF {
		s.logger.Printf("mtproto: no IP configured for dc=%d and no CF proxy", effectiveDC)
		return
	}

	relayInit, relayEnc, relayDec, err := mtproto.GenerateRelayInit(info.Proto, dc)
	if err != nil {
		s.logger.Printf("mtproto: generate relay init: %v", err)
		return
	}

	dialCtx, cancel := context.WithTimeout(ctx, s.cfg.DialTimeout)
	defer cancel()

	dialCF := func() (*wsbridge.Client, error) {
		for _, cfDomain := range s.cfg.CFDomains {
			cfHost := telegram.CFWSDomain(cfDomain, dc)
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
		if targetIP == "" {
			return nil, fmt.Errorf("no IP configured for dc=%d", effectiveDC)
		}
		var lastErr error
		for _, domain := range telegram.WSDomains(effectiveDC, info.IsMedia) {
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: direct dial dc=%d → %s via %s", dc, domain, targetIP)
			}
			// Connect TCP to the DC IP; use domain only for TLS SNI and HTTP Host.
			// Telegram's DC IPs accept WebSocket on port 443 - no DNS needed.
			ws, err := wsbridge.Dial(dialCtx, s.cfg, targetIP, domain)
			if err == nil {
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: direct connected dc=%d → %s", dc, targetIP)
				}
				return ws, nil
			}
			if s.cfg.Verbose {
				s.agg.Printf("mtproto: direct dial failed dc=%d → %s: %v", dc, targetIP, err)
			}
			lastErr = err
		}
		return nil, lastErr
	}

	var ws *wsbridge.Client
	var wsErr error

	if hasCF && s.cfg.UseCFProxyFirst {
		ws, wsErr = dialCF()
		if wsErr != nil {
			ws, wsErr = dialDirect()
		}
	} else {
		ws, wsErr = dialDirect()
		if wsErr != nil && hasCF {
			ws, wsErr = dialCF()
		}
	}

	if wsErr != nil {
		s.agg.Printf("mtproto: ws dial dc=%d: %v", dc, wsErr)
		return
	}
	defer ws.Close()

	if err := ws.Send(relayInit[:]); err != nil {
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
			n, readErr := conn.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				clientDec.XORKeyStream(chunk, chunk)
				relayEnc.XORKeyStream(chunk, chunk)
				for _, part := range splitter.Split(chunk) {
					if sendErr := ws.Send(part); sendErr != nil {
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
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: tg→client recv error dc=%d: %v", dc, recvErr)
				}
				errCh <- recvErr
				return
			}
			if data == nil {
				if s.cfg.Verbose {
					s.agg.Printf("mtproto: tg closed ws dc=%d", dc)
				}
				errCh <- nil
				return
			}
			relayDec.XORKeyStream(data, data)
			clientEnc.XORKeyStream(data, data)
			if _, writeErr := conn.Write(data); writeErr != nil {
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
