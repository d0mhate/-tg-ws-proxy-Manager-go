package cfbalance

import "sync/atomic"

// Balancer rotates Cloudflare domains per server instance when balancing is enabled.
type Balancer struct {
	counter atomic.Uint64
}

func (b *Balancer) Domains(domains []string, enabled bool) []string {
	if len(domains) <= 1 || !enabled {
		return append([]string(nil), domains...)
	}

	start := int(b.counter.Add(1)-1) % len(domains)
	ordered := make([]string, 0, len(domains))
	for i := range domains {
		ordered = append(ordered, domains[(start+i)%len(domains)])
	}
	return ordered
}
