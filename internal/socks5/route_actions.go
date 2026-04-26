package socks5

import (
	"context"
	"errors"
	"io"
	"net"
)

func (s *Server) runPassthrough(ctx context.Context, conn net.Conn, host string, port int, clientAddr string) {
	s.stats.incPassthrough()
	s.debugf("[%s] route=passthrough destination=%s:%d", clientAddr, host, port)
	if err := writeReply(conn, 0x00); err != nil {
		return
	}
	if err := s.proxyTCP(ctx, conn, host, port); err != nil && !errors.Is(err, io.EOF) {
		s.stats.recordError("passthrough", err)
		s.recordVerboseConnFailure(clientAddr, "passthrough", err)
	}
}

func (s *Server) runPassthroughWithInit(ctx context.Context, conn net.Conn, host string, port int, init []byte, clientAddr string, reason string) {
	s.stats.incPassthrough()
	s.debugf("[%s] route=passthrough reason=%s destination=%s:%d", clientAddr, reason, host, port)
	if err := s.proxyTCPWithInit(ctx, conn, host, port, init); err != nil && !errors.Is(err, io.EOF) {
		s.stats.recordError("passthrough", err)
		s.recordVerboseConnFailure(clientAddr, "passthrough", err)
	}
}

func (s *Server) runProbeReadPassthrough(ctx context.Context, conn net.Conn, host string, port int, init []byte, n int, readErr error, clientAddr string) {
	s.stats.incPassthrough()
	s.debugf("[%s] route=passthrough reason=probe-read-failed destination=%s:%d err=%v", clientAddr, host, port, readErr)
	if n == 0 {
		if err := s.proxyTCP(ctx, conn, host, port); err != nil && !errors.Is(err, io.EOF) {
			s.stats.recordError("passthrough", err)
			s.recordVerboseConnFailure(clientAddr, "passthrough", err)
		}
		return
	}
	if err := s.proxyTCPWithInit(ctx, conn, host, port, init[:n]); err != nil && !errors.Is(err, io.EOF) {
		s.stats.recordError("passthrough", err)
		s.recordVerboseConnFailure(clientAddr, "passthrough", err)
	}
}

func (s *Server) runTCPFallbackWithInit(ctx context.Context, conn net.Conn, host string, port int, init []byte, dc int, isMedia bool, clientAddr string, debugf func()) {
	s.stats.incTCPFallback()
	s.stats.recordTCPFallbackRoute(dc, isMedia)
	debugf()
	if err := s.proxyTCPWithInit(ctx, conn, host, port, init); err != nil && !errors.Is(err, io.EOF) {
		s.stats.recordError("tcp_fb", err)
		s.recordVerboseConnFailure(clientAddr, "tcp_fb", err)
	}
}
