package mtpserver

import (
	"context"
	"errors"
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

	if len(routes) != 1 {
		t.Fatalf("expected one direct route for dc203 with explicit override, got %d", len(routes))
	}
	if routes[0].targetDC != 203 || routes[0].targetIP != "91.105.192.100" {
		t.Fatalf("unexpected direct route: %+v", routes[0])
	}
	if routes[0].wsDomainDC != 2 {
		t.Fatalf("expected dc203 websocket domains to stay normalized to dc2, got %+v", routes[0])
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
	cfg.DCIPs[203] = "91.105.192.100"

	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))
	routes := srv.directRouteCandidates(203)

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
	cfg.DCIPs[203] = "91.105.192.100"
	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		return nil, &wsbridge.HandshakeError{StatusCode: 302, StatusLine: "HTTP/1.1 302 Found"}
	}

	route := directRouteCandidate{targetDC: 203, wsDomainDC: 2, targetIP: "91.105.192.100"}
	_, _, err := srv.dialDirectWSWithFallback(context.Background(), 203, false, []directRouteCandidate{route})
	if err == nil {
		t.Fatal("expected redirect error")
	}

	key := routeCooldownKey{requestDC: 203, targetDC: 203, isMedia: false}
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

func TestDialDirectWSSingleRouteKeepsNormalTimeout(t *testing.T) {
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
		if timeout != cfg.DialTimeout {
			t.Fatalf("expected normal timeout %s for single-route dc203, got %s", cfg.DialTimeout, timeout)
		}
	}
}

func TestDialDirectWSKeepsNormalTimeoutForDefaultSingleRoute(t *testing.T) {
	cfg := config.Default()
	cfg.DialTimeout = 10 * time.Second
	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))

	route := directRouteCandidate{targetDC: 2, wsDomainDC: 2, targetIP: "149.154.167.220"}
	srv.routeCooldowns.markFailure(routeCooldownKey{requestDC: 2, targetDC: 2, isMedia: false})

	var seen []time.Duration
	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		seen = append(seen, cfg.DialTimeout)
		return nil, io.EOF
	}

	_, _, err := srv.dialDirectWS(context.Background(), 2, false, route)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected dial error, got %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("expected ws dial attempts")
	}
	for _, timeout := range seen {
		if timeout != cfg.DialTimeout {
			t.Fatalf("expected normal timeout %s for single-route dc, got %s", cfg.DialTimeout, timeout)
		}
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
