package mtpserver

import (
	"slices"
	"testing"

	"tg-ws-proxy/internal/config"
)

func TestWebSocketDialOrderPrefersDirectByDefault(t *testing.T) {
	srv := newTestServer(t, config.Default())

	got := srv.webSocketDialOrder(true, true)
	want := []websocketDialPath{websocketDialDirect, websocketDialCloudflare}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected dial order: got %v want %v", got, want)
	}
}

func TestWebSocketDialOrderPrefersCloudflareWhenConfigured(t *testing.T) {
	cfg := config.Default()
	cfg.UseCFProxyFirst = true
	srv := newTestServer(t, cfg)

	got := srv.webSocketDialOrder(true, true)
	want := []websocketDialPath{websocketDialCloudflare, websocketDialDirect}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected dial order: got %v want %v", got, want)
	}
}

func TestWebSocketDialOrderOmitsUnavailablePaths(t *testing.T) {
	srv := newTestServer(t, config.Default())

	if got := srv.webSocketDialOrder(false, true); !slices.Equal(got, []websocketDialPath{websocketDialCloudflare}) {
		t.Fatalf("unexpected cloudflare-only order: %v", got)
	}
	if got := srv.webSocketDialOrder(true, false); !slices.Equal(got, []websocketDialPath{websocketDialDirect}) {
		t.Fatalf("unexpected direct-only order: %v", got)
	}
	if got := srv.webSocketDialOrder(false, false); len(got) != 0 {
		t.Fatalf("expected empty order without relay paths, got %v", got)
	}
}
