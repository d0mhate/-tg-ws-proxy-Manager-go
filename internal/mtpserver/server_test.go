package mtpserver

import (
	"io"
	"log"
	"testing"

	"tg-ws-proxy/internal/config"
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
