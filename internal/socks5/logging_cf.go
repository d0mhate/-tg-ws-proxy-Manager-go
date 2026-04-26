package socks5

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"tg-ws-proxy/internal/wsbridge"
)

type cfEventTracker struct {
	mu        sync.Mutex
	failed    map[string]int
	connected map[string]int
	lastAddr  string
}

func cfErrSummary(err error) string {
	var hsErr *wsbridge.HandshakeError
	if errors.As(err, &hsErr) {
		return fmt.Sprintf("%d", hsErr.StatusCode)
	}
	msg := err.Error()
	if idx := strings.LastIndex(msg, ": "); idx >= 0 {
		return msg[idx+2:]
	}
	return msg
}

func (s *Server) recordCFEvent(clientAddr string, dc int, isMedia bool, err error) {
	if s.cfEvents == nil {
		return
	}
	dcKey := fmt.Sprintf("dc=%d media=%v", dc, isMedia)
	s.cfEvents.mu.Lock()
	s.cfEvents.lastAddr = clientAddr
	if err != nil {
		key := dcKey + " err=" + cfErrSummary(err)
		s.cfEvents.failed[key]++
	} else {
		s.cfEvents.connected[dcKey]++
	}
	s.cfEvents.mu.Unlock()
}

func (s *Server) flushCFEventSummary() {
	if s.cfEvents == nil {
		return
	}
	s.cfEvents.mu.Lock()
	failed := s.cfEvents.failed
	connected := s.cfEvents.connected
	lastAddr := s.cfEvents.lastAddr
	s.cfEvents.failed = make(map[string]int)
	s.cfEvents.connected = make(map[string]int)
	s.cfEvents.mu.Unlock()

	var parts []string
	for key, n := range connected {
		parts = append(parts, fmt.Sprintf("%s connected=%d", key, n))
	}
	for key, n := range failed {
		parts = append(parts, fmt.Sprintf("%s failed=%d", key, n))
	}
	if len(parts) == 0 {
		return
	}
	slices.Sort(parts)
	s.logger.Printf("cloudflare summary in %s: %s last_source=%s", statsLogEvery, strings.Join(parts, "; "), lastAddr)
}
