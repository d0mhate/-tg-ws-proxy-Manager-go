package mtprotoproxy

import (
	"strings"
	"testing"
)

func TestRenderTerminalQR(t *testing.T) {
	out, err := RenderTerminalQR("tg://proxy?server=192.168.1.1&port=8443&secret=ee0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("RenderTerminalQR returned error: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected non-empty QR output")
	}
	if !strings.ContainsAny(out, "▀▄█") {
		t.Fatalf("expected QR block characters, got:\n%s", out)
	}
}
