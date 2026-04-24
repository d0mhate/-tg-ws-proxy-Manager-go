package mtpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/wsbridge"
)

func TestEffectiveDCUsesExplicitOverride(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs[203] = "91.105.192.100"

	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	if got := srv.effectiveDC(203); got != 203 {
		t.Fatalf("expected explicit dc203 target to be preserved, got %d", got)
	}
}

func TestEffectiveDCFallsBackToNormalizedDC(t *testing.T) {
	cfg := config.Default()
	delete(cfg.DCIPs, 203)

	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	if got := srv.effectiveDC(203); got != 2 {
		t.Fatalf("expected dc203 to normalize to dc2 without explicit override, got %d", got)
	}
}

func TestWSDomainDCNormalizesDC203(t *testing.T) {
	srv := NewMTServer(config.Default(), make([]byte, 16), log.New(io.Discard, "", 0))

	if got := srv.wsDomainDC(203); got != 2 {
		t.Fatalf("expected dc203 websocket domains to use dc2, got %d", got)
	}
}

func TestDialDirectWSIncludesDCInMissingTargetIPErrors(t *testing.T) {
	srv := NewMTServer(config.Default(), make([]byte, 16), log.New(io.Discard, "", 0))

	_, _, err := srv.dialDirectWS(context.Background(), 203, false, directRouteCandidate{
		targetDC:   203,
		wsDomainDC: 2,
	})
	if err == nil {
		t.Fatal("expected missing target IP error")
	}
	if !strings.Contains(err.Error(), "dc=203") {
		t.Fatalf("expected error to include dc number, got %q", err)
	}
}

func TestDirectRouteCandidatesUseExplicitTargetWithNormalizedWSDomainForDC203(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs[203] = "91.105.192.100"

	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))
	routes := srv.directRouteCandidates(203)

	if len(routes) != 2 {
		t.Fatalf("expected explicit and normalized direct routes for dc203, got %d", len(routes))
	}
	if routes[0].targetDC != 203 || routes[0].targetIP != "91.105.192.100" || routes[0].wsDomainDC != 2 {
		t.Fatalf("unexpected explicit direct route: %+v", routes[0])
	}
	if routes[1].targetDC != 2 || routes[1].targetIP != "149.154.167.220" || routes[1].wsDomainDC != 2 {
		t.Fatalf("unexpected normalized fallback route: %+v", routes[1])
	}
}

func TestDirectRouteCandidatesDedupSameEndpointForDC203(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs[203] = "149.154.167.220"

	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))
	routes := srv.directRouteCandidates(203)

	if len(routes) != 1 {
		t.Fatalf("expected identical dc203 and normalized routes to be deduplicated, got %d", len(routes))
	}
	if routes[0].targetDC != 203 || routes[0].targetIP != "149.154.167.220" || routes[0].wsDomainDC != 2 {
		t.Fatalf("unexpected deduplicated route: %+v", routes[0])
	}
}

func TestDirectRouteCandidatesFallbackToNormalizedTargetWithoutExplicitDC203(t *testing.T) {
	cfg := config.Default()
	delete(cfg.DCIPs, 203)

	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))
	routes := srv.directRouteCandidates(203)

	if len(routes) != 1 {
		t.Fatalf("expected one normalized route for dc203 without explicit override, got %d", len(routes))
	}
	if routes[0].targetDC != 2 || routes[0].targetIP != "149.154.167.220" || routes[0].wsDomainDC != 2 {
		t.Fatalf("unexpected normalized route: %+v", routes[0])
	}
}

func TestOrderedDirectRoutesReturnSingleRouteUnchanged(t *testing.T) {
	cfg := config.Default()

	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))
	routes := srv.directRouteCandidates(2)

	ordered := srv.orderedDirectRoutes(203, false, routes)
	if len(ordered) != 1 {
		t.Fatalf("expected single direct route to be preserved, got %d", len(ordered))
	}
	if ordered[0] != routes[0] {
		t.Fatalf("expected single route ordering to stay unchanged, got %+v want %+v", ordered[0], routes[0])
	}
}

func TestSingleRouteDoesNotEnterRedirectCooldown(t *testing.T) {
	cfg := config.Default()
	cfg.DialTimeout = 10 * time.Second
	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		return nil, &wsbridge.HandshakeError{StatusCode: 302, StatusLine: "HTTP/1.1 302 Found"}
	}

	route := directRouteCandidate{targetDC: 2, wsDomainDC: 2, targetIP: "149.154.167.220"}
	_, _, err := srv.dialDirectWSWithFallback(context.Background(), 2, false, []directRouteCandidate{route})
	if err == nil {
		t.Fatal("expected redirect error")
	}

	key := routeCooldownKey{requestDC: 2, targetDC: 2, isMedia: false}
	if srv.routeCooldowns.active(key) {
		t.Fatal("expected single-route redirect to skip route cooldown policy")
	}
}

func TestSingleRouteDoesNotEnterCooldownOnBridgeFailure(t *testing.T) {
	cfg := config.Default()
	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	route := directRouteCandidate{targetDC: 2, wsDomainDC: 2, targetIP: "149.154.167.220"}
	key := routeCooldownKey{requestDC: 2, targetDC: 2, isMedia: false}
	srv.markDirectRouteBridgeFailure(2, false, route)

	if srv.routeCooldowns.active(key) {
		t.Fatal("expected single-route dc to skip bridge cooldown policy")
	}
}

func TestDC203FallbackRouteSkipsFailureCooldown(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs[203] = "91.105.192.100"
	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	fallbackRoute := directRouteCandidate{targetDC: 2, wsDomainDC: 2, targetIP: "149.154.167.220"}
	explicitRoute := directRouteCandidate{targetDC: 203, wsDomainDC: 2, targetIP: "91.105.192.100"}

	srv.markDirectRouteFailure(203, false, fallbackRoute)
	if srv.routeCooldowns.active(routeCooldownKey{requestDC: 203, targetDC: 2, isMedia: false}) {
		t.Fatal("expected dc203 fallback route to skip ordinary failure cooldown")
	}

	srv.markDirectRouteFailure(203, false, explicitRoute)
	if !srv.routeCooldowns.active(routeCooldownKey{requestDC: 203, targetDC: 203, isMedia: false}) {
		t.Fatal("expected explicit dc203 route to keep ordinary failure cooldown")
	}
}

func TestDialDirectWSKeepsNormalTimeoutForInactiveMultiRouteCooldown(t *testing.T) {
	cfg := config.Default()
	cfg.DialTimeout = 10 * time.Second
	cfg.DCIPs[203] = "91.105.192.100"
	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	route := directRouteCandidate{targetDC: 2, wsDomainDC: 2, targetIP: "149.154.167.220"}
	srv.routeCooldowns.markFailure(routeCooldownKey{requestDC: 203, targetDC: 203, isMedia: false})

	var seen []time.Duration
	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		seen = append(seen, cfg.DialTimeout)
		return nil, io.EOF
	}

	_, _, err := srv.dialDirectWS(context.Background(), 203, false, route)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected dial error, got %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("expected ws dial attempts")
	}
	for _, timeout := range seen {
		if timeout != cfg.DialTimeout {
			t.Fatalf("expected normal timeout %s for dc203 fallback route without active cooldown, got %s", cfg.DialTimeout, timeout)
		}
	}
}

func TestDialDirectWSUsesFailFastTimeoutForMultiRouteDC203(t *testing.T) {
	cfg := config.Default()
	cfg.DialTimeout = 10 * time.Second
	cfg.DCIPs[203] = "91.105.192.100"
	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	route := directRouteCandidate{targetDC: 203, wsDomainDC: 2, targetIP: "91.105.192.100"}
	srv.routeCooldowns.markFailure(routeCooldownKey{requestDC: 203, targetDC: 203, isMedia: false})

	var seen []time.Duration
	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		seen = append(seen, cfg.DialTimeout)
		return nil, io.EOF
	}

	_, _, err := srv.dialDirectWS(context.Background(), 203, false, route)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected dial error, got %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("expected ws dial attempts")
	}
	for _, timeout := range seen {
		if timeout != wsFailFastDial {
			t.Fatalf("expected fail-fast timeout %s for multi-route dc203, got %s", wsFailFastDial, timeout)
		}
	}
}

func TestDialDirectWSKeepsNormalTimeoutForDefaultSingleRouteDCs(t *testing.T) {
	for _, dc := range []int{2, 4} {
		t.Run(fmt.Sprintf("dc%d", dc), func(t *testing.T) {
			cfg := config.Default()
			cfg.DialTimeout = 10 * time.Second
			srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

			route := directRouteCandidate{targetDC: dc, wsDomainDC: dc, targetIP: cfg.DCIPs[dc]}
			srv.routeCooldowns.markFailure(routeCooldownKey{requestDC: dc, targetDC: dc, isMedia: false})

			var seen []time.Duration
			srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
				seen = append(seen, cfg.DialTimeout)
				return nil, io.EOF
			}

			_, _, err := srv.dialDirectWS(context.Background(), dc, false, route)
			if !errors.Is(err, io.EOF) {
				t.Fatalf("expected dial error, got %v", err)
			}
			if len(seen) == 0 {
				t.Fatal("expected ws dial attempts")
			}
			for _, timeout := range seen {
				if timeout != cfg.DialTimeout {
					t.Fatalf("expected normal timeout %s for single-route dc=%d, got %s", cfg.DialTimeout, dc, timeout)
				}
			}
		})
	}
}

func TestCFDomainsForConnBalancesRoundRobin(t *testing.T) {
	srv := NewMTServer(config.Config{
		UseCFProxy:   true,
		UseCFBalance: true,
		CFDomains:    []string{"d1.example.com", "d2.example.com", "d3.example.com"},
	}, make([]byte, 16), log.New(io.Discard, "", 0))

	got1 := srv.cfDomainsForConn()
	got2 := srv.cfDomainsForConn()
	got3 := srv.cfDomainsForConn()

	if want := []string{"d1.example.com", "d2.example.com", "d3.example.com"}; !equalStrings(got1, want) {
		t.Fatalf("unexpected first CF domain order: got %v want %v", got1, want)
	}
	if want := []string{"d2.example.com", "d3.example.com", "d1.example.com"}; !equalStrings(got2, want) {
		t.Fatalf("unexpected second CF domain order: got %v want %v", got2, want)
	}
	if want := []string{"d3.example.com", "d1.example.com", "d2.example.com"}; !equalStrings(got3, want) {
		t.Fatalf("unexpected third CF domain order: got %v want %v", got3, want)
	}
}

func TestCFDomainsForConnUsesPerServerState(t *testing.T) {
	cfg := config.Config{
		UseCFProxy:   true,
		UseCFBalance: true,
		CFDomains:    []string{"d1.example.com", "d2.example.com", "d3.example.com"},
	}
	first := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))
	second := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	_ = first.cfDomainsForConn()
	got := second.cfDomainsForConn()

	if want := []string{"d1.example.com", "d2.example.com", "d3.example.com"}; !equalStrings(got, want) {
		t.Fatalf("unexpected independent CF domain order: got %v want %v", got, want)
	}
}

func equalStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestTCPFallbackTargetIPReturnsEmptyWithoutConfiguredRoute(t *testing.T) {
	cfg := config.Default()
	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	if got := srv.tcpFallbackTargetIP(1, nil); got != "" {
		t.Fatalf("expected empty tcp fallback ip for dc1 without direct route, got %q", got)
	}
}

func TestTCPFallbackTargetIPPrefersExplicitDC203Override(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs[203] = "91.105.192.100"
	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	routes := srv.directRouteCandidates(203)
	if got := srv.tcpFallbackTargetIP(203, routes); got != "91.105.192.100" {
		t.Fatalf("expected explicit dc203 tcp fallback ip, got %q", got)
	}
}

func TestDialDirectWSSuccessClearsRouteCooldown(t *testing.T) {
	cfg := config.Default()
	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	route := directRouteCandidate{targetDC: 203, wsDomainDC: 2, targetIP: "91.105.192.100"}
	key := routeCooldownKey{requestDC: 203, targetDC: 203, isMedia: false}
	srv.routeCooldowns.markFailure(key)

	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		clientConn, peerConn := net.Pipe()
		go func() { _ = peerConn.Close() }()
		return wsbridge.NewClient(clientConn), nil
	}

	ws, _, err := srv.dialDirectWSWithFallback(context.Background(), 203, false, []directRouteCandidate{route})
	if err != nil {
		t.Fatalf("expected successful dial, got %v", err)
	}
	if ws == nil {
		t.Fatal("expected websocket client")
	}
	_ = ws.Close()

	if srv.routeCooldowns.active(key) {
		t.Fatal("expected cooldown to be cleared after successful dial")
	}
}
