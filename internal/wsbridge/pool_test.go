package wsbridge

import (
	"context"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
)

func newReusableClient() *Client {
	conn := newMockConn(nil)
	conn.timeoutOnEmpty = true
	return NewClient(conn)
}

func TestPoolRefillAfterMissThenHit(t *testing.T) {
	pool := NewPool(config.Config{PoolSize: 1})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	dialCalls := 0
	pool.dial = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*Client, error) {
		dialCalls++
		return newReusableClient(), nil
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

	client := newReusableClient()
	conn := client.conn.(*mockConn)
	key := poolKey{dc: 2, isMedia: false, targetIP: "149.154.167.220"}
	pool.idle[key] = []pooledClient{{
		client:  client,
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
		return newReusableClient(), nil
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

func TestPoolWarmupPreloadsVariantMediaBucket(t *testing.T) {
	pool := NewPool(config.Config{PoolSize: 1})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	dialCalls := 0
	pool.dial = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*Client, error) {
		dialCalls++
		return newReusableClient(), nil
	}
	defer pool.Close()

	pool.Warmup(map[int]string{2: "149.154.167.220"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ws, hit := pool.Get(2, true, "149.154.167.220", []string{"kws2-1.web.telegram.org", "kws2.web.telegram.org"}); ws != nil && hit {
			if dialCalls == 0 {
				t.Fatal("expected warmup to dial at least once")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected warmup to preload a media pooled client")
}

func TestPoolSeparatesBucketsByTargetIP(t *testing.T) {
	pool := NewPool(config.Config{PoolSize: 1})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	client := newReusableClient()
	key := poolKey{dc: 2, isMedia: false, targetIP: "149.154.167.220"}
	pool.idle[key] = []pooledClient{{
		client:  client,
		created: time.Now(),
	}}
	defer pool.Close()

	if ws, hit := pool.Get(2, false, "91.105.192.100", []string{"kws2.web.telegram.org"}); ws != nil || hit {
		t.Fatal("expected different target IP to use a separate pool bucket")
	}
	if ws, hit := pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"}); ws == nil || !hit {
		t.Fatal("expected original target IP bucket to remain reusable")
	}
}

func TestPoolDiscardsClosedClientBeforeHit(t *testing.T) {
	pool := NewPool(config.Config{PoolSize: 1})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	client := newReusableClient()
	key := poolKey{dc: 2, isMedia: false, targetIP: "149.154.167.220"}
	pool.idle[key] = []pooledClient{{
		client:  client,
		created: time.Now(),
	}}
	client.conn.(*mockConn).closed = true
	defer pool.Close()

	if ws, hit := pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"}); ws != nil || hit {
		t.Fatal("expected closed pooled client to be discarded")
	}
}
