package socks5

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
)

type runtimeStats struct {
	mu               sync.Mutex
	handshakeWait    int
	handshakeEOF     int
	handshakeBadVer  int
	handshakeOther   int
	connections      int
	wsConnections    int
	wsMedia          int
	tcpFallbacks     int
	tcpFallbackMedia int
	httpRejected     int
	passthrough      int
	wsErrors         int
	poolHits         int
	poolMisses       int
	blacklistHits    int
	cooldownActivs   int
	wsByDC           map[int]int
	tcpFallbackByDC  map[int]int
	errorCounts      map[string]int
}

func (s *runtimeStats) incHandshakeWait() { s.add(func() { s.handshakeWait++ }) }
func (s *runtimeStats) decHandshakeWait() {
	s.add(func() {
		if s.handshakeWait > 0 {
			s.handshakeWait--
		}
	})
}
func (s *runtimeStats) incHandshakeEOF()        { s.add(func() { s.handshakeEOF++ }) }
func (s *runtimeStats) incHandshakeBadVersion() { s.add(func() { s.handshakeBadVer++ }) }
func (s *runtimeStats) incHandshakeOther()      { s.add(func() { s.handshakeOther++ }) }
func (s *runtimeStats) incConnections()         { s.add(func() { s.connections++ }) }
func (s *runtimeStats) incWSConnections()       { s.add(func() { s.wsConnections++ }) }
func (s *runtimeStats) incTCPFallback()         { s.add(func() { s.tcpFallbacks++ }) }
func (s *runtimeStats) incHTTPRejected()        { s.add(func() { s.httpRejected++ }) }
func (s *runtimeStats) incPassthrough()         { s.add(func() { s.passthrough++ }) }
func (s *runtimeStats) incWSErrors()            { s.add(func() { s.wsErrors++ }) }
func (s *runtimeStats) incPoolHit()             { s.add(func() { s.poolHits++ }) }
func (s *runtimeStats) incPoolMiss()            { s.add(func() { s.poolMisses++ }) }
func (s *runtimeStats) incBlacklistHits()       { s.add(func() { s.blacklistHits++ }) }
func (s *runtimeStats) incCooldownActivations() { s.add(func() { s.cooldownActivs++ }) }

func (s *runtimeStats) recordWSRoute(dc int, isMedia bool) {
	s.add(func() {
		if isMedia {
			s.wsMedia++
		}
		if s.wsByDC == nil {
			s.wsByDC = make(map[int]int)
		}
		s.wsByDC[dc]++
	})
}

func (s *runtimeStats) recordTCPFallbackRoute(dc int, isMedia bool) {
	s.add(func() {
		if isMedia {
			s.tcpFallbackMedia++
		}
		if s.tcpFallbackByDC == nil {
			s.tcpFallbackByDC = make(map[int]int)
		}
		s.tcpFallbackByDC[dc]++
	})
}

func classifyRuntimeError(prefix string, err error) string {
	if err == nil {
		return prefix + "_other"
	}
	if errors.Is(err, context.Canceled) {
		return prefix + "_canceled"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return prefix + "_timeout"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "i/o timeout"):
		return prefix + "_timeout"
	case strings.Contains(msg, "no route to host"):
		return prefix + "_no_route"
	case strings.Contains(msg, "connection reset by peer"):
		return prefix + "_reset"
	case strings.Contains(msg, "EOF"):
		return prefix + "_eof"
	default:
		return prefix + "_other"
	}
}

func (s *runtimeStats) recordError(prefix string, err error) {
	s.add(func() {
		if s.errorCounts == nil {
			s.errorCounts = make(map[string]int)
		}
		s.errorCounts[classifyRuntimeError(prefix, err)]++
	})
}

func (s *runtimeStats) add(fn func()) {
	if s == nil {
		return
	}
	s.mu.Lock()
	fn()
	s.mu.Unlock()
}

func (s *runtimeStats) summary() string {
	if s == nil {
		return "disabled"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	return fmt.Sprintf(
		"hs_wait=%d hs_eof=%d hs_badver=%d hs_other=%d conn=%d ws=%d tcp_fb=%d passthrough=%d http_reject=%d ws_err=%d pool_hit=%d pool_miss=%d blacklist_hit=%d cooldown_set=%d",
		s.handshakeWait,
		s.handshakeEOF,
		s.handshakeBadVer,
		s.handshakeOther,
		s.connections,
		s.wsConnections,
		s.tcpFallbacks,
		s.passthrough,
		s.httpRejected,
		s.wsErrors,
		s.poolHits,
		s.poolMisses,
		s.blacklistHits,
		s.cooldownActivs,
	)
}

func (s *runtimeStats) summaryBlock(blacklist, cooldown int) string {
	if s == nil {
		return "stats: disabled"
	}

	s.mu.Lock()
	hsWait := s.handshakeWait
	hsEOF := s.handshakeEOF
	hsBadVer := s.handshakeBadVer
	hsOther := s.handshakeOther
	connections := s.connections
	wsConnections := s.wsConnections
	wsMedia := s.wsMedia
	tcpFallbacks := s.tcpFallbacks
	tcpFallbackMedia := s.tcpFallbackMedia
	passthrough := s.passthrough
	httpRejected := s.httpRejected
	wsErrors := s.wsErrors
	poolHits := s.poolHits
	poolMisses := s.poolMisses
	blacklistHits := s.blacklistHits
	cooldownActivs := s.cooldownActivs
	wsByDC := make(map[int]int, len(s.wsByDC))
	for dc, count := range s.wsByDC {
		wsByDC[dc] = count
	}
	fbByDC := make(map[int]int, len(s.tcpFallbackByDC))
	for dc, count := range s.tcpFallbackByDC {
		fbByDC[dc] = count
	}
	errorCounts := make(map[string]int, len(s.errorCounts))
	for key, count := range s.errorCounts {
		errorCounts[key] = count
	}
	s.mu.Unlock()

	lines := []string{
		"stats:",
		fmt.Sprintf("  handshake  wait=%d eof=%d badver=%d other=%d", hsWait, hsEOF, hsBadVer, hsOther),
		fmt.Sprintf("  routes     conn=%d ws=%d tcp_fb=%d passthrough=%d http_reject=%d", connections, wsConnections, tcpFallbacks, passthrough, httpRejected),
		fmt.Sprintf("  media      ws=%d tcp_fb=%d", wsMedia, tcpFallbackMedia),
	}

	if dcLine := summarizeDCBreakdown(wsByDC, fbByDC); dcLine != "" {
		lines = append(lines, "  dc         "+dcLine)
	}
	if probeLine := summarizeProbeCounts(errorCounts); probeLine != "" {
		lines = append(lines, "  probe      "+probeLine)
	}
	if errorLine := summarizeErrorCounts(errorCounts); errorLine != "" {
		lines = append(lines, "  errors     "+errorLine)
	}
	lines = append(lines, fmt.Sprintf("  state      ws_err=%d pool_hit=%d pool_miss=%d blacklist_hit=%d cooldown_set=%d blacklist=%d cooldown=%d", wsErrors, poolHits, poolMisses, blacklistHits, cooldownActivs, blacklist, cooldown))
	return strings.Join(lines, "\n")
}

func summarizeDCBreakdown(wsByDC, fbByDC map[int]int) string {
	parts := make([]string, 0, 2)
	if s := summarizeDCMap("ws", wsByDC); s != "" {
		parts = append(parts, s)
	}
	if s := summarizeDCMap("tcp_fb", fbByDC); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func summarizeDCMap(label string, counts map[int]int) string {
	if len(counts) == 0 {
		return ""
	}
	dcs := make([]int, 0, len(counts))
	for dc, count := range counts {
		if count > 0 {
			dcs = append(dcs, dc)
		}
	}
	if len(dcs) == 0 {
		return ""
	}
	sort.Ints(dcs)
	parts := make([]string, 0, len(dcs))
	for _, dc := range dcs {
		parts = append(parts, fmt.Sprintf("%d=%d", dc, counts[dc]))
	}
	return fmt.Sprintf("%s{%s}", label, strings.Join(parts, ", "))
}

func summarizeErrorCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
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
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].key < items[j].key
	})
	if len(items) > 4 {
		items = items[:4]
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s=%d", item.key, item.count))
	}
	return strings.Join(parts, ", ")
}

func summarizeProbeCounts(counts map[string]int) string {
	parts := make([]string, 0, 2)
	if s := summarizePrefixedCounts("mtproto_init", "init", counts); s != "" {
		parts = append(parts, s)
	}
	if s := summarizePrefixedCounts("passthrough", "passthrough", counts); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func summarizePrefixedCounts(prefix, label string, counts map[string]int) string {
	filtered := make(map[string]int)
	for key, count := range counts {
		if count <= 0 || !strings.HasPrefix(key, prefix+"_") {
			continue
		}
		filtered[strings.TrimPrefix(key, prefix+"_")] = count
	}
	if len(filtered) == 0 {
		return ""
	}
	type item struct {
		key   string
		count int
	}
	items := make([]item, 0, len(filtered))
	for key, count := range filtered {
		items = append(items, item{key: key, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].key < items[j].key
	})
	if len(items) > 3 {
		items = items[:3]
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s=%d", item.key, item.count))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s{%s}", label, strings.Join(parts, ", "))
}
