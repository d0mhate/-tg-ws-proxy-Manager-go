package socks5

import (
	"io"
	"log"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
)

func TestWSRouteStateBlacklistLifecycle(t *testing.T) {
	state := newWSRouteState(wsFailCooldown)
	key := routeKey{dc: 2, isMedia: false}

	if state.isBlacklisted(key) {
		t.Fatal("expected route to start unblacklisted")
	}

	state.markBlacklisted(key)

	if !state.isBlacklisted(key) {
		t.Fatal("expected route to be blacklisted")
	}
	if got := state.blacklistSize(); got != 1 {
		t.Fatalf("expected blacklist size 1, got %d", got)
	}
}

func TestWSRouteStateCooldownLifecycle(t *testing.T) {
	base := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	now := base
	state := newWSRouteState(30 * time.Second)
	state.now = func() time.Time { return now }
	key := routeKey{dc: 4, isMedia: true}

	if state.isCooldownActive(key) {
		t.Fatal("expected route to start without cooldown")
	}

	state.markWSFailure(key)

	if !state.isCooldownActive(key) {
		t.Fatal("expected cooldown after websocket failure")
	}
	if got := state.cooldownSize(); got != 1 {
		t.Fatalf("expected cooldown size 1, got %d", got)
	}

	state.clearWSFailure(key)
	if state.isCooldownActive(key) {
		t.Fatal("expected clearWSFailure to remove cooldown")
	}

	state.markWSFailure(key)
	now = now.Add(31 * time.Second)

	if state.isCooldownActive(key) {
		t.Fatal("expected cooldown to expire after deadline")
	}
	if got := state.cooldownSize(); got != 0 {
		t.Fatalf("expected expired cooldown to be pruned, got %d", got)
	}
}

func TestServerMarkWSFailureRecordsCooldownActivation(t *testing.T) {
	srv := NewServer(config.Config{PoolSize: 0}, log.New(io.Discard, "", 0))
	key := routeKey{dc: 2, isMedia: false}

	srv.markWSFailure(key)

	if !srv.isCooldownActive(key) {
		t.Fatal("expected cooldown after markWSFailure")
	}
	if got := srv.stats.cooldownActivs.Load(); got != 1 {
		t.Fatalf("expected cooldown activation stat to be recorded, got %d", got)
	}
}
