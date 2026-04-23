package socks5

import (
	"errors"
	"time"
)

var errWSBlacklisted = errors.New("websocket route blacklisted")

type routeKey struct {
	dc      int
	isMedia bool
}

func (s *Server) isBlacklisted(key routeKey) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	_, ok := s.wsBlacklist[key]
	return ok
}

func (s *Server) isCooldownActive(key routeKey) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	until, ok := s.wsFailUntil[key]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(s.wsFailUntil, key)
		return false
	}
	return true
}

func (s *Server) markBlacklisted(key routeKey) {
	s.stateMu.Lock()
	s.wsBlacklist[key] = struct{}{}
	s.stateMu.Unlock()
}

func (s *Server) markFailureCooldown(key routeKey) {
	s.stateMu.Lock()
	s.wsFailUntil[key] = time.Now().Add(wsFailCooldown)
	s.stateMu.Unlock()
	s.stats.incCooldownActivations()
}

func (s *Server) clearFailureState(key routeKey) {
	s.stateMu.Lock()
	delete(s.wsFailUntil, key)
	s.stateMu.Unlock()
}

func (s *Server) blacklistSize() int {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return len(s.wsBlacklist)
}

func (s *Server) cooldownSize() int {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	now := time.Now()
	total := 0
	for key, until := range s.wsFailUntil {
		if now.After(until) {
			delete(s.wsFailUntil, key)
			continue
		}
		total++
	}
	return total
}

func fallbackReason(err error) string {
	if errors.Is(err, errWSBlacklisted) {
		return "ws-blacklisted"
	}
	return "ws-connect-failed"
}
