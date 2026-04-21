package wsbridge

import (
	"context"
	"sync"
	"time"

	"tg-ws-proxy/internal/config"
)

const defaultPoolMaxAge = 120 * time.Second

type DialFunc func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*Client, error)

type poolKey struct {
	dc      int
	isMedia bool
}

type pooledClient struct {
	client  *Client
	created time.Time
}

type Pool struct {
	cfg       config.Config
	maxIdle   int
	maxAge    time.Duration
	dial      DialFunc
	now       func() time.Time
	mu        sync.Mutex
	idle      map[poolKey][]pooledClient
	refilling map[poolKey]bool
	closed    bool
}

func NewPool(cfg config.Config) *Pool {
	if cfg.PoolSize <= 0 {
		return nil
	}

	return &Pool{
		cfg:       cfg,
		maxIdle:   cfg.PoolSize,
		maxAge:    defaultPoolMaxAge,
		dial:      Dial,
		now:       time.Now,
		idle:      make(map[poolKey][]pooledClient),
		refilling: make(map[poolKey]bool),
	}
}

func (p *Pool) SetDialFunc(dial DialFunc) {
	if p == nil || dial == nil {
		return
	}
	p.mu.Lock()
	p.dial = dial
	p.mu.Unlock()
}

func (p *Pool) Get(dc int, isMedia bool, targetIP string, domains []string) (*Client, bool) {
	if p == nil || p.maxIdle <= 0 {
		return nil, false
	}

	key := poolKey{dc: dc, isMedia: isMedia}
	now := p.now()

	var stale []*Client
	var hit *Client

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, false
	}

	bucket := p.idle[key]
	kept := bucket[:0]
	for _, entry := range bucket {
		if entry.client == nil || now.Sub(entry.created) > p.maxAge {
			if entry.client != nil {
				stale = append(stale, entry.client)
			}
			continue
		}
		if hit == nil {
			hit = entry.client
			continue
		}
		kept = append(kept, entry)
	}
	if len(kept) == 0 {
		delete(p.idle, key)
	} else {
		p.idle[key] = kept
	}
	p.mu.Unlock()

	for _, client := range stale {
		_ = client.Close()
	}

	p.scheduleRefill(key, targetIP, domains)
	if hit != nil {
		return hit, true
	}
	return nil, false
}

func (p *Pool) Warmup(dcIPs map[int]string) {
	if p == nil || p.maxIdle <= 0 {
		return
	}

	for dc, targetIP := range dcIPs {
		if targetIP == "" {
			continue
		}
		for _, isMedia := range []bool{false, true} {
			p.scheduleRefill(poolKey{dc: dc, isMedia: isMedia}, targetIP, warmupDomains(dc, isMedia))
		}
	}
}

func (p *Pool) Close() {
	if p == nil {
		return
	}

	var toClose []*Client

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	for _, bucket := range p.idle {
		for _, entry := range bucket {
			if entry.client != nil {
				toClose = append(toClose, entry.client)
			}
		}
	}
	p.idle = make(map[poolKey][]pooledClient)
	p.refilling = make(map[poolKey]bool)
	p.mu.Unlock()

	for _, client := range toClose {
		_ = client.Close()
	}
}

func (p *Pool) scheduleRefill(key poolKey, targetIP string, domains []string) {
	if p == nil || p.maxIdle <= 0 {
		return
	}

	p.mu.Lock()
	if p.closed || p.refilling[key] {
		p.mu.Unlock()
		return
	}
	p.refilling[key] = true
	p.mu.Unlock()

	go p.refill(key, targetIP, append([]string(nil), domains...))
}

func (p *Pool) refill(key poolKey, targetIP string, domains []string) {
	defer func() {
		p.mu.Lock()
		delete(p.refilling, key)
		p.mu.Unlock()
	}()

	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}

		now := p.now()
		bucket := p.idle[key]
		kept := bucket[:0]
		for _, entry := range bucket {
			if entry.client == nil || now.Sub(entry.created) > p.maxAge {
				if entry.client != nil {
					go entry.client.Close()
				}
				continue
			}
			kept = append(kept, entry)
		}
		if len(kept) == 0 {
			delete(p.idle, key)
		} else {
			p.idle[key] = kept
		}

		needed := p.maxIdle - len(kept)
		p.mu.Unlock()

		if needed <= 0 {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		client := p.connectOne(ctx, targetIP, domains)
		cancel()
		if client == nil {
			return
		}

		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			_ = client.Close()
			return
		}
		p.idle[key] = append(p.idle[key], pooledClient{
			client:  client,
			created: p.now(),
		})
		p.mu.Unlock()
	}
}

func (p *Pool) connectOne(ctx context.Context, targetIP string, domains []string) *Client {
	p.mu.Lock()
	dial := p.dial
	cfg := p.cfg
	p.mu.Unlock()

	for _, domain := range domains {
		client, err := dial(ctx, cfg, targetIP, domain)
		if err == nil {
			return client
		}
	}
	return nil
}

func warmupDomains(dc int, isMedia bool) []string {
	if isMedia {
		return []string{"kws" + itoa(dc) + "-1.web.telegram.org", "kws" + itoa(dc) + ".web.telegram.org"}
	}
	return []string{"kws" + itoa(dc) + ".web.telegram.org", "kws" + itoa(dc) + "-1.web.telegram.org"}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	if v < 0 {
		return "-" + itoa(-v)
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
