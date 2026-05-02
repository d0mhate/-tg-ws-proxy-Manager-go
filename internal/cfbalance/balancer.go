package cfbalance

import "sync"

// Balancer keeps a sticky Cloudflare domain per DC when balancing is enabled.
type Balancer struct {
	mu         sync.Mutex
	nextAssign int
	lastKey    string
	dcToDomain map[int]string
}

func (b *Balancer) DomainsForDC(dc int, domains []string, enabled bool) []string {
	if len(domains) <= 1 || !enabled {
		return append([]string(nil), domains...)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.dcToDomain == nil {
		b.dcToDomain = make(map[int]string)
	}

	key := domainsKey(domains)
	if b.lastKey != key {
		b.lastKey = key
		b.nextAssign = 0
		b.dcToDomain = make(map[int]string)
	}

	current, ok := b.dcToDomain[dc]
	if !ok || !contains(domains, current) {
		current = domains[b.nextAssign%len(domains)]
		b.nextAssign++
		b.dcToDomain[dc] = current
	}

	ordered := make([]string, 0, len(domains))
	ordered = append(ordered, current)
	for _, domain := range domains {
		if domain != current {
			ordered = append(ordered, domain)
		}
	}
	return ordered
}

func domainsKey(domains []string) string {
	if len(domains) == 0 {
		return ""
	}
	n := 0
	for _, domain := range domains {
		n += len(domain) + 1
	}
	buf := make([]byte, 0, n)
	for _, domain := range domains {
		buf = append(buf, domain...)
		buf = append(buf, 0)
	}
	return string(buf)
}

func contains(domains []string, want string) bool {
	for _, domain := range domains {
		if domain == want {
			return true
		}
	}
	return false
}
