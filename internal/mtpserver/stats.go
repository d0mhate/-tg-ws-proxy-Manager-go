package mtpserver

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type dcStats struct {
	poolHits         uint64
	poolMisses       uint64
	directConnected  uint64
	directDialFailed uint64
	recvErrors       uint64
	closedWS         uint64
}

type statsSnapshot struct {
	perDC        map[int]dcStats
	activeRoutes map[int]int
}

type statsCollector struct {
	mu    sync.Mutex
	perDC map[int]dcStats
}

func newStatsCollector() *statsCollector {
	return &statsCollector{
		perDC: make(map[int]dcStats),
	}
}

func (s *statsCollector) recordPoolHit(dc int) {
	s.update(dc, func(stats *dcStats) {
		stats.poolHits++
	})
}

func (s *statsCollector) recordPoolMiss(dc int) {
	s.update(dc, func(stats *dcStats) {
		stats.poolMisses++
	})
}

func (s *statsCollector) recordDirectConnected(dc int) {
	s.update(dc, func(stats *dcStats) {
		stats.directConnected++
	})
}

func (s *statsCollector) recordDirectDialFailed(dc int) {
	s.update(dc, func(stats *dcStats) {
		stats.directDialFailed++
	})
}

func (s *statsCollector) recordRecvError(dc int) {
	s.update(dc, func(stats *dcStats) {
		stats.recvErrors++
	})
}

func (s *statsCollector) recordClosedWS(dc int) {
	s.update(dc, func(stats *dcStats) {
		stats.closedWS++
	})
}

func (s *statsCollector) update(dc int, fn func(*dcStats)) {
	if s == nil {
		return
	}

	s.mu.Lock()
	stats := s.perDC[dc]
	fn(&stats)
	s.perDC[dc] = stats
	s.mu.Unlock()
}

func (s *statsCollector) snapshot(cooldowns *routeCooldowns) statsSnapshot {
	result := statsSnapshot{
		perDC:        make(map[int]dcStats),
		activeRoutes: make(map[int]int),
	}
	if s == nil {
		return result
	}

	s.mu.Lock()
	for dc, stats := range s.perDC {
		result.perDC[dc] = stats
	}
	s.mu.Unlock()

	if cooldowns != nil {
		result.activeRoutes = cooldowns.snapshotCounts()
	}

	return result
}

func (s *statsCollector) run(stop <-chan struct{}, cooldowns *routeCooldowns, interval time.Duration, logf func(string, ...any)) {
	if s == nil || logf == nil || interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			line := s.snapshot(cooldowns).format()
			if line == "" {
				continue
			}
			logf("mtproto: stats %s", line)
		}
	}
}

func (s statsSnapshot) format() string {
	if len(s.perDC) == 0 {
		return ""
	}

	dcs := make([]int, 0, len(s.perDC))
	for dc := range s.perDC {
		dcs = append(dcs, dc)
	}
	sort.Ints(dcs)

	parts := make([]string, 0, len(dcs))
	for _, dc := range dcs {
		stats := s.perDC[dc]
		activeCooldowns := s.activeRoutes[dc]
		parts = append(parts, fmt.Sprintf(
			"dc=%d hits=%d miss=%d connected=%d dial_fail=%d recv_err=%d closed=%d cooldown=%d",
			dc,
			stats.poolHits,
			stats.poolMisses,
			stats.directConnected,
			stats.directDialFailed,
			stats.recvErrors,
			stats.closedWS,
			activeCooldowns,
		))
	}

	return strings.Join(parts, " | ")
}
