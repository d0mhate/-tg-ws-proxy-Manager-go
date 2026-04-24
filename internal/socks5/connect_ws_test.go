package socks5

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"reflect"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/wsbridge"
)

func TestConnectWSBlacklistsAllRedirects(t *testing.T) {
	srv := NewServer(config.Config{PoolSize: 0}, log.New(io.Discard, "", 0))
	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		return nil, &wsbridge.HandshakeError{
			StatusCode: 302,
			StatusLine: "HTTP/1.1 302 Found",
			Location:   "https://example.invalid",
		}
	}

	_, err := srv.connectWS(context.Background(), "149.154.167.220", 2, false)
	if !errors.Is(err, errWSBlacklisted) {
		t.Fatalf("expected blacklist error, got %v", err)
	}

	if !srv.isBlacklisted(routeKey{dc: 2, isMedia: false}) {
		t.Fatal("expected route to be blacklisted")
	}
}

func TestConnectWSFailureSetsCooldownAndSuccessClearsIt(t *testing.T) {
	srv := NewServer(config.Config{PoolSize: 0}, log.New(io.Discard, "", 0))
	fail := true
	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		if fail {
			return nil, io.EOF
		}
		clientConn, peerConn := net.Pipe()
		go func() { _ = peerConn.Close() }()
		return wsbridge.NewClient(clientConn), nil
	}

	_, err := srv.connectWS(context.Background(), "149.154.167.220", 2, false)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected initial dial error, got %v", err)
	}

	key := routeKey{dc: 2, isMedia: false}
	if !srv.isCooldownActive(key) {
		t.Fatal("expected cooldown after failed websocket dial")
	}

	fail = false
	ws, err := srv.connectWS(context.Background(), "149.154.167.220", 2, false)
	if err != nil {
		t.Fatalf("expected successful dial after cooldown test, got %v", err)
	}
	if ws == nil {
		t.Fatal("expected websocket client on successful dial")
	}
	_ = ws.Close()

	if srv.isCooldownActive(key) {
		t.Fatal("expected cooldown to be cleared after successful websocket dial")
	}
}

func TestConnectWSUsesFailFastDialTimeoutForAllDCs(t *testing.T) {
	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	var seen []time.Duration

	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		seen = append(seen, cfg.DialTimeout)
		return nil, io.EOF
	}

	_, err := srv.connectWS(context.Background(), "149.154.175.205", 1, true)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected dial error, got %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("expected websocket dial attempts")
	}
	for _, timeout := range seen {
		if timeout != wsFailFastDial {
			t.Fatalf("expected fail-fast dial timeout %s, got %s", wsFailFastDial, timeout)
		}
	}
}

func TestConnectWSUsesNormalizedDomainsForExplicitDC203(t *testing.T) {
	srv := NewServer(config.Config{PoolSize: 0}, log.New(io.Discard, "", 0))
	var seenDomains []string

	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		seenDomains = append(seenDomains, domain)
		return nil, io.EOF
	}

	_, err := srv.connectWS(context.Background(), "91.105.192.100", 203, false)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected dial error, got %v", err)
	}

	want := []string{"kws2.web.telegram.org", "kws2-1.web.telegram.org"}
	if !reflect.DeepEqual(seenDomains, want) {
		t.Fatalf("unexpected domains for explicit dc203: got %v want %v", seenDomains, want)
	}
}

func TestConnectWSCFDialsHostname(t *testing.T) {
	srv := NewServer(config.Config{PoolSize: 0, UseCFProxy: true, CFDomains: []string{"cf.example.com"}}, log.New(io.Discard, "", 0))
	var seenTarget, seenDomain string

	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		seenTarget = targetIP
		seenDomain = domain
		return nil, io.EOF
	}

	_, _ = srv.connectWSCF(context.Background(), 2, false)
	if seenTarget != "kws2.cf.example.com" {
		t.Fatalf("unexpected CF dial target: %q", seenTarget)
	}
	if seenDomain != "kws2.cf.example.com" {
		t.Fatalf("unexpected CF dial domain: %q", seenDomain)
	}
}
