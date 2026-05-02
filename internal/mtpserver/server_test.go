package mtpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/wsbridge"
)

func newTestServer(t *testing.T, cfg config.Config) *MTServer {
	t.Helper()

	srv := NewMTServer(cfg, make([]byte, 16), log.New(io.Discard, "", 0))
	t.Cleanup(func() {
		if srv.pool != nil {
			srv.pool.Close()
		}
	})
	return srv
}

func TestEffectiveDCUsesExplicitOverride(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs[203] = testIPv4DC203

	srv := newTestServer(t, cfg)

	if got := srv.effectiveDC(203); got != 203 {
		t.Fatalf("expected explicit dc203 target to be preserved, got %d", got)
	}
}

func TestEffectiveDCFallsBackToNormalizedDC(t *testing.T) {
	cfg := config.Default()
	delete(cfg.DCIPs, 203)

	srv := newTestServer(t, cfg)

	if got := srv.effectiveDC(203); got != 2 {
		t.Fatalf("expected dc203 to normalize to dc2 without explicit override, got %d", got)
	}
}

func TestWSDomainDCNormalizesDC203(t *testing.T) {
	srv := newTestServer(t, config.Default())

	if got := srv.wsDomainDC(203); got != 2 {
		t.Fatalf("expected dc203 websocket domains to use dc2, got %d", got)
	}
}

func TestDialDirectWSIncludesDCInMissingTargetIPErrors(t *testing.T) {
	srv := newTestServer(t, config.Default())

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
	cfg.DCIPs[203] = testIPv4DC203

	srv := newTestServer(t, cfg)
	routes := srv.directRouteCandidates(203)

	if len(routes) != 2 {
		t.Fatalf("expected explicit and normalized direct routes for dc203, got %d", len(routes))
	}
	if routes[0].targetDC != 203 || routes[0].targetIP != testIPv4DC203 || routes[0].wsDomainDC != 2 {
		t.Fatalf("unexpected explicit direct route: %+v", routes[0])
	}
	if routes[1].targetDC != 2 || routes[1].targetIP != testIPv4DC2 || routes[1].wsDomainDC != 2 {
		t.Fatalf("unexpected normalized fallback route: %+v", routes[1])
	}
}

func TestDirectRouteCandidatesDedupSameEndpointForDC203(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs[203] = testIPv4DC2

	srv := newTestServer(t, cfg)
	routes := srv.directRouteCandidates(203)

	if len(routes) != 1 {
		t.Fatalf("expected identical dc203 and normalized routes to be deduplicated, got %d", len(routes))
	}
	if routes[0].targetDC != 203 || routes[0].targetIP != testIPv4DC2 || routes[0].wsDomainDC != 2 {
		t.Fatalf("unexpected deduplicated route: %+v", routes[0])
	}
}

func TestDirectRouteCandidatesFallbackToNormalizedTargetWithoutExplicitDC203(t *testing.T) {
	cfg := config.Default()
	delete(cfg.DCIPs, 203)

	srv := newTestServer(t, cfg)
	routes := srv.directRouteCandidates(203)

	if len(routes) != 1 {
		t.Fatalf("expected one normalized route for dc203 without explicit override, got %d", len(routes))
	}
	if routes[0].targetDC != 2 || routes[0].targetIP != testIPv4DC2 || routes[0].wsDomainDC != 2 {
		t.Fatalf("unexpected normalized route: %+v", routes[0])
	}
}

func TestBridgeRelayClosesBothSidesOnContextCancel(t *testing.T) {
	srv := newTestServer(t, config.Default())
	client := newBlockingConn()
	remote := newBlockingConn()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		srv.bridgeRelay(ctx, client, remote, nopStream{}, nopStream{}, nopStream{}, nopStream{})
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("bridgeRelay did not return after context cancellation")
	}

	if !client.isClosed() {
		t.Fatal("expected client conn to be closed")
	}
	if !remote.isClosed() {
		t.Fatal("expected remote conn to be closed")
	}
}

type nopStream struct{}

func (nopStream) XORKeyStream(dst, src []byte) {
	copy(dst, src)
}

type blockingConn struct {
	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

func newBlockingConn() *blockingConn {
	return &blockingConn{done: make(chan struct{})}
}

func (c *blockingConn) Read([]byte) (int, error) {
	<-c.done
	return 0, net.ErrClosed
}

func (c *blockingConn) Write([]byte) (int, error) {
	<-c.done
	return 0, net.ErrClosed
}

func (c *blockingConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	close(c.done)
	return nil
}

func (c *blockingConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *blockingConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (c *blockingConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (c *blockingConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr string

func (a dummyAddr) Network() string { return "tcp" }
func (a dummyAddr) String() string  { return string(a) }

func TestOrderedDirectRoutesReturnSingleRouteUnchanged(t *testing.T) {
	cfg := config.Default()

	srv := newTestServer(t, cfg)
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
	srv := newTestServer(t, cfg)

	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		return nil, &wsbridge.HandshakeError{StatusCode: 302, StatusLine: "HTTP/1.1 302 Found"}
	}

	route := directRouteCandidate{targetDC: 2, wsDomainDC: 2, targetIP: testIPv4DC2}
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
	srv := newTestServer(t, cfg)

	route := directRouteCandidate{targetDC: 2, wsDomainDC: 2, targetIP: testIPv4DC2}
	key := routeCooldownKey{requestDC: 2, targetDC: 2, isMedia: false}
	srv.markDirectRouteBridgeFailure(2, false, route)

	if srv.routeCooldowns.active(key) {
		t.Fatal("expected single-route dc to skip bridge cooldown policy")
	}
}

func TestDC203FallbackRouteSkipsFailureCooldown(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs[203] = testIPv4DC203
	srv := newTestServer(t, cfg)

	fallbackRoute := directRouteCandidate{targetDC: 2, wsDomainDC: 2, targetIP: testIPv4DC2}
	explicitRoute := directRouteCandidate{targetDC: 203, wsDomainDC: 2, targetIP: testIPv4DC203}

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
	cfg.DCIPs[203] = testIPv4DC203
	srv := newTestServer(t, cfg)

	route := directRouteCandidate{targetDC: 2, wsDomainDC: 2, targetIP: testIPv4DC2}
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
	cfg.DCIPs[203] = testIPv4DC203
	srv := newTestServer(t, cfg)

	route := directRouteCandidate{targetDC: 203, wsDomainDC: 2, targetIP: testIPv4DC203}
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
			srv := newTestServer(t, cfg)

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

func TestCFDomainsForConnIsStickyPerDC(t *testing.T) {
	srv := newTestServer(t, config.Config{
		UseCFProxy:   true,
		UseCFBalance: true,
		CFDomains:    []string{"d1.example.com", "d2.example.com", "d3.example.com"},
	})

	got1 := srv.cfDomainsForConn(2)
	got2 := srv.cfDomainsForConn(2)
	got3 := srv.cfDomainsForConn(2)

	if want := []string{"d1.example.com", "d2.example.com", "d3.example.com"}; !equalStrings(got1, want) {
		t.Fatalf("unexpected first CF domain order: got %v want %v", got1, want)
	}
	if !equalStrings(got2, got1) {
		t.Fatalf("expected sticky CF domain order for same dc, got %v then %v", got1, got2)
	}
	if !equalStrings(got3, got1) {
		t.Fatalf("expected sticky CF domain order for same dc, got %v then %v", got1, got3)
	}
}

func TestCFDomainsForConnAssignsDifferentDCsIndependently(t *testing.T) {
	srv := newTestServer(t, config.Config{
		UseCFProxy:   true,
		UseCFBalance: true,
		CFDomains:    []string{"d1.example.com", "d2.example.com", "d3.example.com"},
	})

	got2 := srv.cfDomainsForConn(2)
	got4 := srv.cfDomainsForConn(4)

	if want := []string{"d1.example.com", "d2.example.com", "d3.example.com"}; !equalStrings(got2, want) {
		t.Fatalf("unexpected dc2 CF domain order: got %v want %v", got2, want)
	}
	if want := []string{"d2.example.com", "d1.example.com", "d3.example.com"}; !equalStrings(got4, want) {
		t.Fatalf("unexpected dc4 CF domain order: got %v want %v", got4, want)
	}
}

func TestCFDomainsForConnUsesPerServerState(t *testing.T) {
	cfg := config.Config{
		UseCFProxy:   true,
		UseCFBalance: true,
		CFDomains:    []string{"d1.example.com", "d2.example.com", "d3.example.com"},
	}
	first := newTestServer(t, cfg)
	second := newTestServer(t, cfg)

	_ = first.cfDomainsForConn(2)
	got := second.cfDomainsForConn(2)

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
	srv := newTestServer(t, cfg)

	if got := srv.tcpFallbackTargetIP(1, nil); got != "" {
		t.Fatalf("expected empty tcp fallback ip for dc1 without direct route, got %q", got)
	}
}

func TestTCPFallbackTargetIPPrefersExplicitDC203Override(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs[203] = testIPv4DC203
	srv := newTestServer(t, cfg)

	routes := srv.directRouteCandidates(203)
	if got := srv.tcpFallbackTargetIP(203, routes); got != testIPv4DC203 {
		t.Fatalf("expected explicit dc203 tcp fallback ip, got %q", got)
	}
}

func TestNormalizeBridgeErrorReturnsNilForNilError(t *testing.T) {
	ctx := context.Background()
	if err := normalizeBridgeError(ctx, nil); err != nil {
		t.Fatalf("expected nil for nil error, got %v", err)
	}
}

func TestNormalizeBridgeErrorReturnsNilForEOF(t *testing.T) {
	ctx := context.Background()
	if err := normalizeBridgeError(ctx, io.EOF); err != nil {
		t.Fatalf("expected nil for EOF, got %v", err)
	}
}

func TestNormalizeBridgeErrorReturnsNilForCancelledContextWithClosedConn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := normalizeBridgeError(ctx, net.ErrClosed); err != nil {
		t.Fatalf("expected nil for ErrClosed on cancelled context, got %v", err)
	}
}

func TestNormalizeBridgeErrorPreservesUnrelatedErrors(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("unexpected protocol error")
	if err := normalizeBridgeError(ctx, sentinel); !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error to be preserved, got %v", err)
	}
}

func TestDialDirectWSSuccessClearsRouteCooldown(t *testing.T) {
	cfg := config.Default()
	srv := newTestServer(t, cfg)

	route := directRouteCandidate{targetDC: 203, wsDomainDC: 2, targetIP: testIPv4DC203}
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
