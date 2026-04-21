package wsbridge

import (
	"context"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
)

func TestPoolRefillAfterMissThenHit(t *testing.T) {
	pool := NewPool(config.Config{PoolSize: 1})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	dialCalls := 0
	pool.dial = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*Client, error) {
		dialCalls++
		return &Client{conn: newMockConn(nil)}, nil
	}
	defer pool.Close()

	if ws, hit := pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"}); ws != nil || hit {
		t.Fatal("expected first get to miss and trigger background refill")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ws, hit := pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"}); ws != nil && hit {
			if dialCalls == 0 {
				t.Fatal("expected dial to be called during refill")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected pool hit after refill")
}

func TestPoolCloseClosesIdleClients(t *testing.T) {
	pool := NewPool(config.Config{PoolSize: 1})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	conn := newMockConn(nil)
	key := poolKey{dc: 2, isMedia: false}
	pool.idle[key] = []pooledClient{{
		client:  &Client{conn: conn},
		created: time.Now(),
	}}

	pool.Close()

	if !conn.closed {
		t.Fatal("expected idle pooled connection to be closed")
	}
}

func TestPoolWarmupPreloadsBuckets(t *testing.T) {
	pool := NewPool(config.Config{PoolSize: 1})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	dialCalls := 0
	pool.dial = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*Client, error) {
		dialCalls++
		return &Client{conn: newMockConn(nil)}, nil
	}
	defer pool.Close()

	pool.Warmup(map[int]string{2: "149.154.167.220"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ws, hit := pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"}); ws != nil && hit {
			if dialCalls == 0 {
				t.Fatal("expected warmup to dial at least once")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected warmup to preload a pooled client")
}
