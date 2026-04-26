package wsbridge

import (
	"context"
	"errors"
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

func TestPoolDiscardsExpiredClient(t *testing.T) {
	pool := NewPool(config.Config{PoolSize: 1, PoolMaxAge: time.Second})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	base := time.Unix(100, 0)
	pool.now = func() time.Time { return base.Add(2 * time.Second) }
	pool.dial = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*Client, error) {
		return nil, errors.New("test dial blocked")
	}
	conn := newMockConn(nil)
	key := poolKey{dc: 2, isMedia: false, targetIP: "149.154.167.220"}
	pool.idle[key] = []pooledClient{{
		client:  &Client{conn: conn},
		created: base,
	}}

	if ws, hit := pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"}); ws != nil || hit {
		t.Fatal("expected expired pooled client to miss")
	}

	if !waitForCondition(2*time.Second, func() bool { return conn.closed }) {
		t.Fatal("expected expired pooled client to be closed")
	}
}

func TestPoolZeroTuningIsRespected(t *testing.T) {
	pool := NewPool(config.Config{
		PoolSize:        1,
		PoolMaxAge:      0,
		PoolRefillDelay: 0,
	})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
	defer pool.Close()

	if pool.maxAge != 0 {
		t.Fatalf("expected pool max age 0 to be preserved, got %s", pool.maxAge)
	}
	if pool.refillDelay != 0 {
		t.Fatalf("expected refill delay 0 to be preserved, got %s", pool.refillDelay)
	}
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
	pool.dial = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*Client, error) {
		return nil, errors.New("test dial blocked")
	}
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

func TestPoolGetDoesNotBlockOnStaleClose(t *testing.T) {
	pool := NewPool(config.Config{PoolSize: 1})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
	defer pool.Close()

	pool.dial = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*Client, error) {
		return nil, errors.New("test dial blocked")
	}

	conn := newBlockingWriteConn()
	client := NewClient(conn)
	key := poolKey{dc: 2, isMedia: false, targetIP: "149.154.167.220"}
	pool.idle[key] = []pooledClient{{
		client:  client,
		created: time.Now(),
	}}

	start := time.Now()
	if ws, hit := pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"}); ws != nil || hit {
		t.Fatal("expected stale pooled client to miss")
	}
	if elapsed := time.Since(start); elapsed >= closeWriteTimeout/2 {
		t.Fatalf("expected stale close to be asynchronous, Get took %s", elapsed)
	}

	deadline := time.Now().Add(2 * time.Second)
	if waitForCondition(time.Until(deadline), func() bool {
		conn.mu.Lock()
		defer conn.mu.Unlock()
		return conn.closed
	}) {
		return
	}

	t.Fatal("expected stale pooled connection to close in background")
}

func TestPoolRefillDelayAppliedBetweenConnections(t *testing.T) {
	pool := NewPool(config.Config{
		PoolSize:        2,
		PoolRefillDelay: 20 * time.Millisecond,
	})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
	defer pool.Close()

	dialTimes := make(chan time.Time, 4)
	pool.dial = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*Client, error) {
		dialTimes <- time.Now()
		return newReusableClient(), nil
	}

	if ws, hit := pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"}); ws != nil || hit {
		t.Fatal("expected initial get to miss and trigger refill")
	}

	first := waitForTime(t, dialTimes)
	second := waitForTime(t, dialTimes)
	if second.Sub(first) < 15*time.Millisecond {
		t.Fatalf("expected refill delay between pooled dials, got %s", second.Sub(first))
	}
}

func TestPoolGetReturnsFreshestClientFirst(t *testing.T) {
	pool := NewPool(config.Config{PoolSize: 3, PoolMaxAge: time.Minute})
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
	defer pool.Close()

	pool.dial = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*Client, error) {
		return nil, errors.New("test dial blocked")
	}

	c1 := newReusableClient()
	c2 := newReusableClient()
	c3 := newReusableClient()
	base := time.Now()
	key := poolKey{dc: 2, isMedia: false, targetIP: "149.154.167.220"}
	pool.idle[key] = []pooledClient{
		{client: c1, created: base.Add(-3 * time.Second)},
		{client: c2, created: base.Add(-2 * time.Second)},
		{client: c3, created: base.Add(-1 * time.Second)},
	}

	ws, hit := pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"})
	if !hit || ws != c3 {
		t.Fatalf("expected freshest client first, got hit=%v client=%p", hit, ws)
	}
	ws, hit = pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"})
	if !hit || ws != c2 {
		t.Fatalf("expected second freshest client next, got hit=%v client=%p", hit, ws)
	}
	ws, hit = pool.Get(2, false, "149.154.167.220", []string{"kws2.web.telegram.org"})
	if !hit || ws != c1 {
		t.Fatalf("expected oldest client last, got hit=%v client=%p", hit, ws)
	}
}

func waitForTime(t *testing.T, ch <-chan time.Time) time.Time {
	t.Helper()

	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pooled dial")
		return time.Time{}
	}
}

func waitForCondition(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return cond()
}
