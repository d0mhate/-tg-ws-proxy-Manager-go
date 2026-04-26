package socks5

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

type handshakeFailureTracker struct {
	mu       sync.Mutex
	counts   map[string]int
	lastAddr map[string]string
}

func classifyHandshakeFailure(err error) string {
	var hsErr *handshakeError
	if errors.As(err, &hsErr) {
		switch {
		case errors.Is(hsErr.err, io.EOF), errors.Is(hsErr.err, io.ErrUnexpectedEOF):
			switch hsErr.stage {
			case "greeting":
				return "closed_before_greeting"
			case "request":
				return "closed_before_request"
			default:
				return "closed_during_auth"
			}
		case hsErr.err != nil && hsErr.err.Error() == "unsupported socks version":
			return "invalid_version"
		}
	}
	return "other"
}

func (s *Server) recordHandshakeFailure(clientAddr string, err error) {
	if s.hsFails == nil {
		s.logger.Printf("[%s] handshake failed: %v", clientAddr, err)
		return
	}
	reason := classifyHandshakeFailure(err)
	s.hsFails.mu.Lock()
	s.hsFails.counts[reason]++
	s.hsFails.lastAddr[reason] = clientAddr
	s.hsFails.mu.Unlock()

	switch reason {
	case "closed_before_greeting":
		s.stats.incHandshakeEOF()
	case "invalid_version":
		s.stats.incHandshakeBadVersion()
	default:
		s.stats.incHandshakeOther()
	}
}

func (s *Server) flushHandshakeFailureSummary() {
	if s.cfg.Verbose || s.hsFails == nil {
		return
	}

	s.hsFails.mu.Lock()
	counts := make(map[string]int, len(s.hsFails.counts))
	lastAddrs := make(map[string]string, len(s.hsFails.lastAddr))
	for reason, count := range s.hsFails.counts {
		counts[reason] = count
	}
	for reason, addr := range s.hsFails.lastAddr {
		lastAddrs[reason] = addr
	}
	s.hsFails.counts = make(map[string]int)
	s.hsFails.lastAddr = make(map[string]string)
	s.hsFails.mu.Unlock()

	order := []string{"closed_before_greeting", "closed_before_request", "closed_during_auth", "invalid_version", "other"}
	parts := make([]string, 0, len(order))
	lastSource := ""
	for _, reason := range order {
		count := counts[reason]
		if count == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s x%d", reason, count))
		if lastSource == "" {
			lastSource = lastAddrs[reason]
		}
	}
	if len(parts) == 0 {
		return
	}
	s.logger.Printf("handshake failures summary: %s in %s last_source=%s", strings.Join(parts, ", "), statsLogEvery, lastSource)
}
