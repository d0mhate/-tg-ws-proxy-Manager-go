package socks5

import (
	"cmp"
	"slices"
	"strings"
	"sync"
)

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
