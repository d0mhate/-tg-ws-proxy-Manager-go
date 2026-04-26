package mtpserver

import (
	"testing"
	"time"
)

func TestMarkBridgeFailureSetsLongerCooldownThanMarkFailure(t *testing.T) {
	now := time.Now()
	r := &routeCooldowns{
		until:            make(map[routeCooldownKey]time.Time),
		now:              func() time.Time { return now },
		failDuration:     30 * time.Second,
		bridgeDuration:   60 * time.Second,
		redirectDuration: 5 * time.Minute,
	}
	key := routeCooldownKey{requestDC: 2, targetDC: 2}

	r.markFailure(key)
	failUntil := r.until[key]

	r.markBridgeFailure(key)
	bridgeUntil := r.until[key]

	if !bridgeUntil.After(failUntil) {
		t.Fatalf("bridge failure cooldown (%v) should exceed plain failure cooldown (%v)",
			bridgeUntil.Sub(now), failUntil.Sub(now))
	}
}

func TestMarkRedirectSetsLongerCooldownThanMarkBridgeFailure(t *testing.T) {
	now := time.Now()
	r := &routeCooldowns{
		until:            make(map[routeCooldownKey]time.Time),
		now:              func() time.Time { return now },
		failDuration:     30 * time.Second,
		bridgeDuration:   60 * time.Second,
		redirectDuration: 5 * time.Minute,
	}
	key := routeCooldownKey{requestDC: 2, targetDC: 2}

	r.markBridgeFailure(key)
	bridgeUntil := r.until[key]

	r.markRedirect(key)
	redirectUntil := r.until[key]

	if !redirectUntil.After(bridgeUntil) {
		t.Fatalf("redirect cooldown (%v) should exceed bridge failure cooldown (%v)",
			redirectUntil.Sub(now), bridgeUntil.Sub(now))
	}
}

func TestSnapshotCountsGroupsActiveEntriesByRequestDC(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Minute)
	r := &routeCooldowns{
		until: map[routeCooldownKey]time.Time{
			{requestDC: 2, targetDC: 2}:   future,
			{requestDC: 2, targetDC: 203}: future,
			{requestDC: 4, targetDC: 4}:   future,
		},
		now: func() time.Time { return now },
	}

	counts := r.snapshotCounts()

	if counts[2] != 2 {
		t.Fatalf("expected dc2 count=2, got %d", counts[2])
	}
	if counts[4] != 1 {
		t.Fatalf("expected dc4 count=1, got %d", counts[4])
	}
}

func TestSnapshotCountsSkipsAndPrunesExpiredEntries(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	future := time.Now().Add(time.Minute)
	r := &routeCooldowns{
		until: map[routeCooldownKey]time.Time{
			{requestDC: 2, targetDC: 2}: past,
			{requestDC: 4, targetDC: 4}: future,
		},
		now: time.Now,
	}

	counts := r.snapshotCounts()

	if counts[2] != 0 {
		t.Fatalf("expected expired dc2 entry to be excluded, got %d", counts[2])
	}
	if counts[4] != 1 {
		t.Fatalf("expected active dc4 entry to be counted, got %d", counts[4])
	}
	expiredKey := routeCooldownKey{requestDC: 2, targetDC: 2}
	if _, ok := r.until[expiredKey]; ok {
		t.Fatal("expected expired entry to be pruned from the map")
	}
}

func TestSnapshotCountsNilReceiverReturnsNil(t *testing.T) {
	var r *routeCooldowns
	if counts := r.snapshotCounts(); counts != nil {
		t.Fatalf("expected nil for nil receiver, got %v", counts)
	}
}
