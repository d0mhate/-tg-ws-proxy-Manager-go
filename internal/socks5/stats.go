package socks5

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

type runtimeStats struct {
	handshakeWait    atomic.Int64
	handshakeEOF     atomic.Int64
	handshakeBadVer  atomic.Int64
	handshakeOther   atomic.Int64
	connections      atomic.Int64
	wsConnections    atomic.Int64
	wsMedia          atomic.Int64
	tcpFallbacks     atomic.Int64
	tcpFallbackMedia atomic.Int64
	httpRejected     atomic.Int64
	passthrough      atomic.Int64
	wsErrors         atomic.Int64
	poolHits         atomic.Int64
	poolMisses       atomic.Int64
	blacklistHits    atomic.Int64
	cooldownActivs   atomic.Int64

	mapMu           sync.Mutex
	wsByDC          map[int]int
	tcpFallbackByDC map[int]int
	errorCounts     map[string]int
}

func (s *runtimeStats) incHandshakeWait() {
	if s == nil {
		return
	}
	s.handshakeWait.Add(1)
}

func (s *runtimeStats) decHandshakeWait() {
	if s == nil {
		return
	}
	for {
		v := s.handshakeWait.Load()
		if v <= 0 {
			return
		}
		if s.handshakeWait.CompareAndSwap(v, v-1) {
			return
		}
	}
}

func (s *runtimeStats) incHandshakeEOF()        { if s != nil { s.handshakeEOF.Add(1) } }
func (s *runtimeStats) incHandshakeBadVersion() { if s != nil { s.handshakeBadVer.Add(1) } }
func (s *runtimeStats) incHandshakeOther()      { if s != nil { s.handshakeOther.Add(1) } }
func (s *runtimeStats) incConnections()         { if s != nil { s.connections.Add(1) } }
func (s *runtimeStats) incWSConnections()       { if s != nil { s.wsConnections.Add(1) } }
func (s *runtimeStats) incTCPFallback()         { if s != nil { s.tcpFallbacks.Add(1) } }
func (s *runtimeStats) incHTTPRejected()        { if s != nil { s.httpRejected.Add(1) } }
func (s *runtimeStats) incPassthrough()         { if s != nil { s.passthrough.Add(1) } }
func (s *runtimeStats) incWSErrors()            { if s != nil { s.wsErrors.Add(1) } }
func (s *runtimeStats) incPoolHit()             { if s != nil { s.poolHits.Add(1) } }
func (s *runtimeStats) incPoolMiss()            { if s != nil { s.poolMisses.Add(1) } }
func (s *runtimeStats) incBlacklistHits()       { if s != nil { s.blacklistHits.Add(1) } }
func (s *runtimeStats) incCooldownActivations() { if s != nil { s.cooldownActivs.Add(1) } }

func (s *runtimeStats) recordWSRoute(dc int, isMedia bool) {
	if s == nil {
		return
	}
	if isMedia {
		s.wsMedia.Add(1)
	}
	s.mapMu.Lock()
	if s.wsByDC == nil {
		s.wsByDC = make(map[int]int)
	}
	s.wsByDC[dc]++
	s.mapMu.Unlock()
}

func (s *runtimeStats) recordTCPFallbackRoute(dc int, isMedia bool) {
	if s == nil {
		return
	}
	if isMedia {
		s.tcpFallbackMedia.Add(1)
	}
	s.mapMu.Lock()
	if s.tcpFallbackByDC == nil {
		s.tcpFallbackByDC = make(map[int]int)
	}
	s.tcpFallbackByDC[dc]++
	s.mapMu.Unlock()
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
	if s == nil {
		return
	}
	key := classifyRuntimeError(prefix, err)
	s.mapMu.Lock()
	if s.errorCounts == nil {
		s.errorCounts = make(map[string]int)
	}
	s.errorCounts[key]++
	s.mapMu.Unlock()
}

func (s *runtimeStats) summary() string {
	if s == nil {
		return "disabled"
	}
	return fmt.Sprintf(
		"hs_wait=%d hs_eof=%d hs_badver=%d hs_other=%d conn=%d ws=%d tcp_fb=%d passthrough=%d http_reject=%d ws_err=%d pool_hit=%d pool_miss=%d blacklist_hit=%d cooldown_set=%d",
		s.handshakeWait.Load(),
		s.handshakeEOF.Load(),
		s.handshakeBadVer.Load(),
		s.handshakeOther.Load(),
		s.connections.Load(),
		s.wsConnections.Load(),
		s.tcpFallbacks.Load(),
		s.passthrough.Load(),
		s.httpRejected.Load(),
		s.wsErrors.Load(),
		s.poolHits.Load(),
		s.poolMisses.Load(),
		s.blacklistHits.Load(),
		s.cooldownActivs.Load(),
	)
}

func (s *runtimeStats) summaryBlock(blacklist, cooldown int) string {
	if s == nil {
		return "stats: disabled"
	}

	hsWait := s.handshakeWait.Load()
	hsEOF := s.handshakeEOF.Load()
	hsBadVer := s.handshakeBadVer.Load()
	hsOther := s.handshakeOther.Load()
	connections := s.connections.Load()
	wsConnections := s.wsConnections.Load()
	wsMedia := s.wsMedia.Load()
	tcpFallbacks := s.tcpFallbacks.Load()
	tcpFallbackMedia := s.tcpFallbackMedia.Load()
	passthrough := s.passthrough.Load()
	httpRejected := s.httpRejected.Load()
	wsErrors := s.wsErrors.Load()
	poolHits := s.poolHits.Load()
	poolMisses := s.poolMisses.Load()
	blacklistHits := s.blacklistHits.Load()
	cooldownActivs := s.cooldownActivs.Load()

	s.mapMu.Lock()
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
	s.mapMu.Unlock()

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
	slices.Sort(dcs)
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
	slices.SortFunc(items, func(a, b item) int {
		if a.count != b.count {
			return cmp.Compare(b.count, a.count)
		}
		return cmp.Compare(a.key, b.key)
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
	slices.SortFunc(items, func(a, b item) int {
		if a.count != b.count {
			return cmp.Compare(b.count, a.count)
		}
		return cmp.Compare(a.key, b.key)
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
