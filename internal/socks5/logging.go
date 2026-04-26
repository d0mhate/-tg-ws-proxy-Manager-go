package socks5

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

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
				s.flushCFEventSummary()
				s.logger.Printf("%s", s.stats.summaryBlock(s.blacklistSize(), s.cooldownSize()))
				return
			case <-ticker.C:
				s.flushAuthFailureSummary()
				s.flushHandshakeFailureSummary()
				s.flushVerboseConnFailureSummary()
				s.flushVerboseDebugSummary()
				s.flushCFEventSummary()
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
	var hsErr *handshakeError
	if errors.As(err, &hsErr) && len(hsErr.firstBytes) > 0 {
		s.logger.Printf("[%s] handshake failed: %v first_bytes=%s", clientAddr, err, hex.EncodeToString(hsErr.firstBytes))
		return
	}
	s.logger.Printf("[%s] handshake failed: %v", clientAddr, err)
}

func (s *Server) debugf(format string, args ...any) {
	if !s.cfg.Verbose {
		return
	}
	s.recordVerboseDebug(fmt.Sprintf(format, args...))
}
