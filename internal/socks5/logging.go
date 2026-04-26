package socks5

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"tg-ws-proxy/internal/wsbridge"
)

type authFailureTracker struct {
	mu       sync.Mutex
	count    int
	lastAddr string
}

type handshakeFailureTracker struct {
	mu       sync.Mutex
	counts   map[string]int
	lastAddr map[string]string
}

type connFailureTracker struct {
	mu       sync.Mutex
	counts   map[string]int
	lastAddr map[string]string
}

type debugEventTracker struct {
	mu      sync.Mutex
	counts  map[string]int
	samples map[string]string
}

type cfEventTracker struct {
	mu        sync.Mutex
	failed    map[string]int
	connected map[string]int
	lastAddr  string
}

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

func (s *Server) recordVerboseConnFailure(clientAddr, prefix string, err error) {
	if !s.cfg.Verbose || s.verboseFails == nil {
		return
	}
	key := classifyRuntimeError(prefix, err)
	s.verboseFails.mu.Lock()
	s.verboseFails.counts[key]++
	s.verboseFails.lastAddr[key] = clientAddr
	s.verboseFails.mu.Unlock()
}

func (s *Server) flushVerboseConnFailureSummary() {
	if !s.cfg.Verbose || s.verboseFails == nil {
		return
	}

	s.verboseFails.mu.Lock()
	counts := make(map[string]int, len(s.verboseFails.counts))
	lastAddrs := make(map[string]string, len(s.verboseFails.lastAddr))
	for key, count := range s.verboseFails.counts {
		counts[key] = count
	}
	for key, addr := range s.verboseFails.lastAddr {
		lastAddrs[key] = addr
	}
	s.verboseFails.counts = make(map[string]int)
	s.verboseFails.lastAddr = make(map[string]string)
	s.verboseFails.mu.Unlock()

	type item struct {
		key   string
		count int
	}
	items := make([]item, 0, len(counts))
	for key, count := range counts {
		if count > 0 {
			items = append(items, item{key: key, count: count})
		}
	}
	if len(items) == 0 {
		return
	}
	slices.SortFunc(items, func(a, b item) int {
		if a.count != b.count {
			return cmp.Compare(b.count, a.count)
		}
		return cmp.Compare(a.key, b.key)
	})
	if len(items) > 6 {
		items = items[:6]
	}

	for _, item := range items {
		s.logger.Printf("%s x%d in %s last_source=%s", item.key, item.count, statsLogEvery, lastAddrs[item.key])
	}
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

func normalizeVerboseMessage(msg string) string {
	if strings.HasPrefix(msg, "[") {
		if idx := strings.Index(msg, "] "); idx >= 0 {
			msg = msg[idx+2:]
		}
	}
	if strings.HasPrefix(msg, "accepted connection from ") {
		return "accepted connection from <client>"
	}
	return msg
}

func (s *Server) recordVerboseDebug(msg string) {
	if !s.cfg.Verbose || s.verboseDebug == nil || msg == "" {
		return
	}
	key := normalizeVerboseMessage(msg)
	s.verboseDebug.mu.Lock()
	s.verboseDebug.counts[key]++
	if _, ok := s.verboseDebug.samples[key]; !ok {
		s.verboseDebug.samples[key] = msg
	}
	s.verboseDebug.mu.Unlock()
}

func (s *Server) flushVerboseDebugSummary() {
	if !s.cfg.Verbose || s.verboseDebug == nil {
		return
	}

	s.verboseDebug.mu.Lock()
	counts := make(map[string]int, len(s.verboseDebug.counts))
	samples := make(map[string]string, len(s.verboseDebug.samples))
	for key, count := range s.verboseDebug.counts {
		counts[key] = count
	}
	for key, sample := range s.verboseDebug.samples {
		samples[key] = sample
	}
	s.verboseDebug.counts = make(map[string]int)
	s.verboseDebug.samples = make(map[string]string)
	s.verboseDebug.mu.Unlock()

	type item struct {
		key    string
		count  int
		sample string
	}
	items := make([]item, 0, len(counts))
	for key, count := range counts {
		if count > 0 {
			items = append(items, item{key: key, count: count, sample: samples[key]})
		}
	}
	if len(items) == 0 {
		return
	}
	slices.SortFunc(items, func(a, b item) int {
		if a.count != b.count {
			return cmp.Compare(b.count, a.count)
		}
		return cmp.Compare(a.key, b.key)
	})
	if len(items) > 12 {
		items = items[:12]
	}

	for _, item := range items {
		s.logger.Printf("%s x%d in %s", item.key, item.count, statsLogEvery)
	}
}

func (s *Server) debugf(format string, args ...any) {
	if !s.cfg.Verbose {
		return
	}
	s.recordVerboseDebug(fmt.Sprintf(format, args...))
}
