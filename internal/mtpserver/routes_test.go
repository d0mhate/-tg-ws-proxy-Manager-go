package mtpserver

import (
	"strings"
	"testing"

	"tg-ws-proxy/internal/config"
)

func TestPreferredFallbackRoutePrefersDCMatchingWSDomain(t *testing.T) {
	srv := newTestServer(t, config.Default())

	routes := []directRouteCandidate{
		{targetDC: 203, wsDomainDC: 2, targetIP: testIPv4DC203},
		{targetDC: 2, wsDomainDC: 2, targetIP: testIPv4DC2},
	}

	preferred := srv.preferredFallbackRoute(routes)
	if preferred.targetDC != 2 {
		t.Fatalf("expected route where targetDC==wsDomainDC to be preferred, got targetDC=%d", preferred.targetDC)
	}
}

func TestPreferredFallbackRouteFallsBackToFirstWhenNoneMatch(t *testing.T) {
	srv := newTestServer(t, config.Default())

	routes := []directRouteCandidate{
		{targetDC: 203, wsDomainDC: 2, targetIP: testIPv4DC203},
		{targetDC: 5, wsDomainDC: 3, targetIP: "1.2.3.4"},
	}

	preferred := srv.preferredFallbackRoute(routes)
	if preferred.targetDC != 203 {
		t.Fatalf("expected first route when none have matching targetDC/wsDomainDC, got targetDC=%d", preferred.targetDC)
	}
}

func TestPreferredFallbackRouteReturnsEmptyForEmptySlice(t *testing.T) {
	srv := newTestServer(t, config.Default())

	preferred := srv.preferredFallbackRoute(nil)
	if preferred.targetIP != "" || preferred.targetDC != 0 {
		t.Fatalf("expected zero-value candidate for empty routes, got %+v", preferred)
	}
}

func TestCFWSHostProducesDifferentOutputForDifferentDCs(t *testing.T) {
	hostDC2 := cfWSHost("cf.example.com", 2)
	hostDC4 := cfWSHost("cf.example.com", 4)

	if hostDC2 == "" || hostDC4 == "" {
		t.Fatal("expected non-empty CF WS host for both DCs")
	}
	if hostDC2 == hostDC4 {
		t.Fatalf("expected different hosts for dc=2 and dc=4, got %q for both", hostDC2)
	}
	if !strings.Contains(hostDC2, "cf.example.com") {
		t.Fatalf("expected CF domain to appear in host, got %q", hostDC2)
	}
}
