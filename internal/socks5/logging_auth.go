package socks5

import "sync"

type authFailureTracker struct {
	mu       sync.Mutex
	count    int
	lastAddr string
}

func (s *Server) recordAuthFailure(clientAddr string) {
	if s.authFails == nil {
		s.logger.Printf("[%s] handshake failed: %s", clientAddr, errInvalidUsernamePassword)
		return
	}
	s.authFails.mu.Lock()
	s.authFails.count++
	s.authFails.lastAddr = clientAddr
	s.authFails.mu.Unlock()
}

func (s *Server) flushAuthFailureSummary() {
	if s.cfg.Verbose || s.authFails == nil {
		return
	}

	s.authFails.mu.Lock()
	count := s.authFails.count
	lastAddr := s.authFails.lastAddr
	s.authFails.count = 0
	s.authFails.lastAddr = ""
	s.authFails.mu.Unlock()

	if count == 0 {
		return
	}

	s.logger.Printf("auth failures summary: %s x%d in %s last_source=%s", errInvalidUsernamePassword, count, statsLogEvery, lastAddr)
}
