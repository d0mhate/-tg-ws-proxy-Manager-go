package mtpserver

import (
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"

	"golang.org/x/sync/errgroup"

	"tg-ws-proxy/internal/config"
)

const telegramTCPPort = 443

func (s *MTServer) tcpFallbackTargetIP(dc int, routes []directRouteCandidate) string {
	if ip := s.cfg.DCIPs[dc]; ip != "" {
		return ip
	}

	defaultDCIPs := config.Default().DCIPs
	if ip := defaultDCIPs[dc]; ip != "" {
		return ip
	}

	if len(routes) > 0 {
		return routes[0].targetIP
	}

	effectiveDC := s.effectiveDC(dc)
	if ip := s.cfg.DCIPs[effectiveDC]; ip != "" {
		return ip
	}
	if ip := defaultDCIPs[effectiveDC]; ip != "" {
		return ip
	}

	return ""
}

func (s *MTServer) dialTCPFallback(ctx context.Context, targetIP string, relayInit [64]byte) (net.Conn, error) {
	addr := net.JoinHostPort(targetIP, strconv.Itoa(telegramTCPPort))
	dialCtx, cancel := context.WithTimeout(ctx, s.cfg.DialTimeout)
	defer cancel()

	tcpConn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tcp fallback dial %s: %w", addr, err)
	}
	if _, err := tcpConn.Write(relayInit[:]); err != nil {
		_ = tcpConn.Close()
		return nil, fmt.Errorf("tcp fallback send init %s: %w", addr, err)
	}

	return tcpConn, nil
}

func (s *MTServer) bridgeRelay(
	ctx context.Context,
	client net.Conn,
	remote net.Conn,
	clientDec, clientEnc cipher.Stream,
	remoteEnc, remoteDec cipher.Stream,
) {
	bridgeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, gctx := errgroup.WithContext(bridgeCtx)
	go func() {
		<-gctx.Done()
		_ = client.Close()
		_ = remote.Close()
	}()

	buf := s.cfg.BufferKB * 1024

	g.Go(func() error {
		defer cancel()
		b := make([]byte, buf)
		for {
			n, readErr := client.Read(b)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, b[:n])
				clientDec.XORKeyStream(chunk, chunk)
				remoteEnc.XORKeyStream(chunk, chunk)
				if _, writeErr := remote.Write(chunk); writeErr != nil {
					return normalizeBridgeError(gctx, writeErr)
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					readErr = nil
				}
				return normalizeBridgeError(gctx, readErr)
			}
		}
	})

	g.Go(func() error {
		defer cancel()
		b := make([]byte, buf)
		for {
			n, readErr := remote.Read(b)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, b[:n])
				remoteDec.XORKeyStream(chunk, chunk)
				clientEnc.XORKeyStream(chunk, chunk)
				if _, writeErr := client.Write(chunk); writeErr != nil {
					return normalizeBridgeError(gctx, writeErr)
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					readErr = nil
				}
				return normalizeBridgeError(gctx, readErr)
			}
		}
	})

	_ = g.Wait()
}

func (s *MTServer) bridgeTCPFallback(
	ctx context.Context,
	client net.Conn,
	targetIP string,
	relayInit [64]byte,
	clientDec, clientEnc cipher.Stream,
	relayEnc, relayDec cipher.Stream,
) error {
	remote, err := s.dialTCPFallback(ctx, targetIP, relayInit)
	if err != nil {
		return err
	}
	defer remote.Close()

	s.bridgeRelay(ctx, client, remote, clientDec, clientEnc, relayEnc, relayDec)
	return nil
}

func normalizeBridgeError(ctx context.Context, err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	if ctx.Err() != nil && (errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled)) {
		return nil
	}
	return err
}
