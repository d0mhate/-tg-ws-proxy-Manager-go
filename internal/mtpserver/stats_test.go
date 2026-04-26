package mtpserver

import (
	"strings"
	"testing"
	"time"
)

func TestStatsCollectorRecordsAllCounters(t *testing.T) {
	s := newStatsCollector()

	s.recordPoolHit(2)
	s.recordPoolHit(2)
	s.recordPoolMiss(2)
	s.recordDirectConnected(2)
	s.recordDirectDialFailed(2)
	s.recordRecvError(2)
	s.recordClosedWS(2)

	snap := s.snapshot(nil)
	dc2 := snap.perDC[2]

	if dc2.poolHits != 2 {
		t.Fatalf("expected poolHits=2, got %d", dc2.poolHits)
	}
	if dc2.poolMisses != 1 {
		t.Fatalf("expected poolMisses=1, got %d", dc2.poolMisses)
	}
	if dc2.directConnected != 1 {
		t.Fatalf("expected directConnected=1, got %d", dc2.directConnected)
	}
	if dc2.directDialFailed != 1 {
		t.Fatalf("expected directDialFailed=1, got %d", dc2.directDialFailed)
	}
	if dc2.recvErrors != 1 {
		t.Fatalf("expected recvErrors=1, got %d", dc2.recvErrors)
	}
	if dc2.closedWS != 1 {
		t.Fatalf("expected closedWS=1, got %d", dc2.closedWS)
	}
}

func TestStatsCollectorNilReceiverIsNoop(t *testing.T) {
	var s *statsCollector

	// none of these should panic
	s.recordPoolHit(2)
	s.recordPoolMiss(2)
	s.recordDirectConnected(2)
	s.recordDirectDialFailed(2)
	s.recordRecvError(2)
	s.recordClosedWS(2)

	snap := s.snapshot(nil)
	if len(snap.perDC) != 0 {
		t.Fatalf("expected empty snapshot for nil collector, got %v", snap.perDC)
	}
}

func TestStatsSnapshotIncludesCooldownCounts(t *testing.T) {
	s := newStatsCollector()
	s.recordPoolHit(2)

	now := time.Now()
	cooldowns := &routeCooldowns{
		until: map[routeCooldownKey]time.Time{
			{requestDC: 2, targetDC: 2}: now.Add(time.Minute),
		},
		now: func() time.Time { return now },
	}

	snap := s.snapshot(cooldowns)
	if snap.activeRoutes[2] != 1 {
		t.Fatalf("expected 1 active cooldown for dc2, got %d", snap.activeRoutes[2])
	}
}

func TestStatsSnapshotFormatIncludesAllFields(t *testing.T) {
	s := newStatsCollector()
	s.recordPoolHit(2)
	s.recordPoolMiss(2)
	s.recordDirectConnected(2)
	s.recordDirectDialFailed(2)
	s.recordRecvError(2)
	s.recordClosedWS(2)

	line := s.snapshot(nil).format()

	for _, want := range []string{"dc=2", "hits=1", "miss=1", "connected=1", "dial_fail=1", "recv_err=1", "closed=1"} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected %q in stats format output, got: %s", want, line)
		}
	}
}

func TestStatsSnapshotFormatReturnsEmptyWhenNoData(t *testing.T) {
	s := newStatsCollector()
	if line := s.snapshot(nil).format(); line != "" {
		t.Fatalf("expected empty format for zero stats, got %q", line)
	}
}

func TestStatsSnapshotFormatSortsDCsNumerically(t *testing.T) {
	s := newStatsCollector()
	s.recordPoolHit(4)
	s.recordPoolHit(2)

	line := s.snapshot(nil).format()

	dc2pos := strings.Index(line, "dc=2")
	dc4pos := strings.Index(line, "dc=4")
	if dc2pos < 0 || dc4pos < 0 {
		t.Fatalf("expected both dc=2 and dc=4 in output, got: %s", line)
	}
	if dc2pos > dc4pos {
		t.Fatalf("expected dc=2 to appear before dc=4, got: %s", line)
	}
}

func TestStatsRunStopsOnChannelClose(t *testing.T) {
	s := newStatsCollector()
	s.recordPoolHit(2)

	stop := make(chan struct{})
	done := make(chan struct{})

	logLines := 0
	go func() {
		s.run(stop, nil, 10*time.Millisecond, func(string, ...any) { logLines++ })
		close(done)
	}()

	time.Sleep(35 * time.Millisecond)
	close(stop)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stats.run did not stop after channel close")
	}
	if logLines == 0 {
		t.Fatal("expected at least one stats log line before stop")
	}
}
