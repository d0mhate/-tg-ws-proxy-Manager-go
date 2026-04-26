package socks5

import (
	"errors"
	"sync"
	"time"
)

var errWSBlacklisted = errors.New("websocket route blacklisted")

type routeKey struct {
	dc      int
	isMedia bool
}

type wsRouteState struct {
	mu        sync.Mutex
	blacklist map[routeKey]struct{}
	failUntil map[routeKey]time.Time
	now       func() time.Time
	cooldown  time.Duration
}

func newWSRouteState(cooldown time.Duration) *wsRouteState {
	return &wsRouteState{
		blacklist: make(map[routeKey]struct{}),
		failUntil: make(map[routeKey]time.Time),
		now:       time.Now,
		cooldown:  cooldown,
	}
}

func (s *wsRouteState) isBlacklisted(key routeKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.blacklist[key]
	return ok
}

func (s *wsRouteState) isCooldownActive(key routeKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	until, ok := s.failUntil[key]
	if !ok {
		return false
	}
	if s.now().After(until) {
		delete(s.failUntil, key)
		return false
	}
	return true
}

func (s *wsRouteState) markBlacklisted(key routeKey) {
	s.mu.Lock()
	s.blacklist[key] = struct{}{}
	s.mu.Unlock()
}

func (s *wsRouteState) markWSFailure(key routeKey) {
	s.mu.Lock()
	s.failUntil[key] = s.now().Add(s.cooldown)
	s.mu.Unlock()
}

func (s *wsRouteState) clearWSFailure(key routeKey) {
	s.mu.Lock()
	delete(s.failUntil, key)
	s.mu.Unlock()
}

func (s *wsRouteState) blacklistSize() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.blacklist)
}

func (s *wsRouteState) cooldownSize() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	total := 0
	for key, until := range s.failUntil {
		if now.After(until) {
			delete(s.failUntil, key)
			continue
		}
		total++
	}
	return total
}

func (s *Server) isBlacklisted(key routeKey) bool {
	if s.routeState == nil {
		return false
	}
	return s.routeState.isBlacklisted(key)
}

func (s *Server) isCooldownActive(key routeKey) bool {
	if s.routeState == nil {
		return false
	}
	return s.routeState.isCooldownActive(key)
}

func (s *Server) markBlacklisted(key routeKey) {
	if s.routeState == nil {
		return
	}
	s.routeState.markBlacklisted(key)
}

func (s *Server) markWSFailure(key routeKey) {
	if s.routeState == nil {
		return
	}
	s.routeState.markWSFailure(key)
	s.stats.incCooldownActivations()
}

func (s *Server) clearWSFailure(key routeKey) {
	if s.routeState == nil {
		return
	}
	s.routeState.clearWSFailure(key)
}

func (s *Server) blacklistSize() int {
	if s.routeState == nil {
		return 0
	}
	return s.routeState.blacklistSize()
}

func (s *Server) cooldownSize() int {
	if s.routeState == nil {
		return 0
	}
	return s.routeState.cooldownSize()
}

func fallbackReason(err error) string {
	if errors.Is(err, errWSBlacklisted) {
		return "ws-blacklisted"
	}
	return "ws-connect-failed"
}
