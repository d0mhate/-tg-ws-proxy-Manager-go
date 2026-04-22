package mtpserver

import (
	"sync"
	"time"
)

type routeCooldownKey struct {
	requestDC int
	targetDC  int
	isMedia   bool
}

type routeCooldowns struct {
	mu               sync.Mutex
	until            map[routeCooldownKey]time.Time
	now              func() time.Time
	failDuration     time.Duration
	bridgeDuration   time.Duration
	redirectDuration time.Duration
}

func newRouteCooldowns(failDuration time.Duration, bridgeDuration time.Duration, redirectDuration time.Duration) *routeCooldowns {
	return &routeCooldowns{
		until:            make(map[routeCooldownKey]time.Time),
		now:              time.Now,
		failDuration:     failDuration,
		bridgeDuration:   bridgeDuration,
		redirectDuration: redirectDuration,
	}
}

func (r *routeCooldowns) active(key routeCooldownKey) bool {
	if r == nil {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	until, ok := r.until[key]
	if !ok {
		return false
	}
	if !r.now().Before(until) {
		delete(r.until, key)
		return false
	}
	return true
}

func (r *routeCooldowns) markFailure(key routeCooldownKey) {
	r.mark(key, r.failDuration)
}

func (r *routeCooldowns) markBridgeFailure(key routeCooldownKey) {
	r.mark(key, r.bridgeDuration)
}

func (r *routeCooldowns) markRedirect(key routeCooldownKey) {
	r.mark(key, r.redirectDuration)
}

func (r *routeCooldowns) mark(key routeCooldownKey, duration time.Duration) {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.until[key] = r.now().Add(duration)
	r.mu.Unlock()
}

func (r *routeCooldowns) clear(key routeCooldownKey) {
	if r == nil {
		return
	}

	r.mu.Lock()
	delete(r.until, key)
	r.mu.Unlock()
}

func (r *routeCooldowns) snapshotCounts() map[int]int {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	counts := make(map[int]int)
	for key, until := range r.until {
		if !now.Before(until) {
			delete(r.until, key)
			continue
		}
		counts[key.requestDC]++
	}
	return counts
}

func (r *routeCooldowns) timeoutFor(key routeCooldownKey, normalTimeout time.Duration, cooldownTimeout time.Duration) time.Duration {
	if r == nil {
		return normalTimeout
	}
	if normalTimeout <= 0 || normalTimeout <= cooldownTimeout {
		return normalTimeout
	}
	if r.active(key) {
		return cooldownTimeout
	}
	return normalTimeout
}
