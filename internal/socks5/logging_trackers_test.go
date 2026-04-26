package socks5

import (
	"bytes"
	"errors"
	"io"
	"log"
	"strings"
	"testing"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/wsbridge"
)

func TestFlushVerboseConnFailureSummarySortsByCountAndLimitsOutput(t *testing.T) {
	var logs bytes.Buffer
	cfg := config.Default()
	cfg.Verbose = true
	srv := NewServer(cfg, log.New(&logs, "", 0))

	record := func(prefix string, n int) {
		for i := 0; i < n; i++ {
			srv.recordVerboseConnFailure("10.0.0.1:1111", prefix, io.EOF)
		}
	}

	record("gamma", 1)
	record("alpha", 4)
	record("epsilon", 2)
	record("beta", 4)
	record("delta", 3)
	record("zeta", 1)
	record("eta", 1)

	srv.flushVerboseConnFailureSummary()

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected top 6 verbose failure summaries, got %d: %q", len(lines), logs.String())
	}
	if !strings.Contains(lines[0], "alpha_eof x4") {
		t.Fatalf("expected alpha_eof first, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "beta_eof x4") {
		t.Fatalf("expected beta_eof second, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "delta_eof x3") {
		t.Fatalf("expected delta_eof third, got %q", lines[2])
	}
	if strings.Contains(logs.String(), "zeta_eof") && strings.Contains(logs.String(), "eta_eof") {
		t.Fatalf("expected one low-frequency entry to be trimmed, got:\n%s", logs.String())
	}
}

func TestFlushCFEventSummaryAggregatesConnectedAndFailed(t *testing.T) {
	var logs bytes.Buffer
	srv := NewServer(config.Default(), log.New(&logs, "", 0))

	srv.recordCFEvent("10.0.0.1:1000", 2, false, nil)
	srv.recordCFEvent("10.0.0.2:2000", 2, false, nil)
	srv.recordCFEvent("10.0.0.3:3000", 4, true, &wsbridge.HandshakeError{
		StatusCode: 302,
		StatusLine: "HTTP/1.1 302 Found",
	})
	srv.recordCFEvent("10.0.0.4:4000", 4, true, errors.New("dial tcp: i/o timeout"))
	srv.flushCFEventSummary()

	out := logs.String()
	if !strings.Contains(out, "cloudflare summary in 5s:") {
		t.Fatalf("expected cloudflare summary, got:\n%s", out)
	}
	if !strings.Contains(out, "dc=2 media=false connected=2") {
		t.Fatalf("expected connected count, got:\n%s", out)
	}
	if !strings.Contains(out, "dc=4 media=true err=302 failed=1") {
		t.Fatalf("expected redirect status aggregation, got:\n%s", out)
	}
	if !strings.Contains(out, "dc=4 media=true err=i/o timeout failed=1") {
		t.Fatalf("expected dial error aggregation, got:\n%s", out)
	}
	if !strings.Contains(out, "last_source=10.0.0.4:4000") {
		t.Fatalf("expected last source to be preserved, got:\n%s", out)
	}

	logs.Reset()
	srv.flushCFEventSummary()
	if logs.Len() != 0 {
		t.Fatalf("expected flush to reset cloudflare tracker, got:\n%s", logs.String())
	}
}
