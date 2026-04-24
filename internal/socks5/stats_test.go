package socks5

import (
	"bytes"
	"context"
	"log"
	"net"
	"strings"
	"testing"

	"tg-ws-proxy/internal/config"
)

func TestHandleConnInvalidAuthAggregatesSummaryWhenNotVerbose(t *testing.T) {
	var logs bytes.Buffer
	cfg := config.Default()
	cfg.Username = "alice"
	cfg.Password = "secret"
	srv := NewServer(cfg, log.New(&logs, "", 0))

	for i := 0; i < 3; i++ {
		runHandleConnInvalidAuthAttempt(t, srv, "alice", "wrong")
	}

	if strings.Contains(logs.String(), "handshake failed: invalid username/password") {
		t.Fatalf("expected non-verbose mode to suppress per-attempt invalid auth logs, got:\n%s", logs.String())
	}

	srv.flushAuthFailureSummary()
	out := logs.String()
	if !strings.Contains(out, "auth failures summary: invalid username/password x3 in 5s") {
		t.Fatalf("expected aggregated auth failure summary, got:\n%s", out)
	}
}

func TestHandleConnInvalidAuthLogsEachAttemptInVerboseMode(t *testing.T) {
	var logs bytes.Buffer
	cfg := config.Default()
	cfg.Username = "alice"
	cfg.Password = "secret"
	cfg.Verbose = true
	srv := NewServer(cfg, log.New(&logs, "", 0))

	for i := 0; i < 2; i++ {
		runHandleConnInvalidAuthAttempt(t, srv, "alice", "wrong")
	}
	srv.flushAuthFailureSummary()

	out := logs.String()
	if strings.Count(out, "handshake failed: invalid username/password") != 2 {
		t.Fatalf("expected verbose mode to log each invalid auth attempt, got:\n%s", out)
	}
	if strings.Contains(out, "auth failures summary: invalid username/password") {
		t.Fatalf("expected verbose mode to skip aggregated auth summary, got:\n%s", out)
	}
}

func TestHandleConnHandshakeFailuresAggregateSummaryWhenNotVerbose(t *testing.T) {
	var logs bytes.Buffer
	srv := NewServer(config.Default(), log.New(&logs, "", 0))

	runHandleConnRawHandshakeAttempt(t, srv, nil)
	runHandleConnRawHandshakeAttempt(t, srv, []byte{0x04, 0x01})

	if strings.Contains(logs.String(), "handshake failed:") {
		t.Fatalf("expected non-verbose mode to suppress per-attempt handshake logs, got:\n%s", logs.String())
	}

	srv.flushHandshakeFailureSummary()
	out := logs.String()
	if !strings.Contains(out, "handshake failures summary: closed_before_greeting x1, invalid_version x1 in 5s") {
		t.Fatalf("expected aggregated handshake failure summary, got:\n%s", out)
	}
}

func TestHandleConnHandshakeFailuresLogEachAttemptInVerboseMode(t *testing.T) {
	var logs bytes.Buffer
	cfg := config.Default()
	cfg.Verbose = true
	srv := NewServer(cfg, log.New(&logs, "", 0))

	runHandleConnRawHandshakeAttempt(t, srv, []byte{0x04, 0x01})
	srv.flushHandshakeFailureSummary()

	out := logs.String()
	if !strings.Contains(out, "handshake failed: unsupported socks version") {
		t.Fatalf("expected verbose mode to log each handshake failure, got:\n%s", out)
	}
	if strings.Contains(out, "handshake failures summary:") {
		t.Fatalf("expected verbose mode to skip aggregated handshake summary, got:\n%s", out)
	}
}

func TestRuntimeStatsSummaryIncludesHandshakeCounters(t *testing.T) {
	stats := &runtimeStats{
		handshakeWait:   3,
		handshakeEOF:    2,
		handshakeBadVer: 1,
		handshakeOther:  4,
		connections:     5,
	}

	out := stats.summary()
	if !strings.Contains(out, "hs_wait=3") ||
		!strings.Contains(out, "hs_eof=2") ||
		!strings.Contains(out, "hs_badver=1") ||
		!strings.Contains(out, "hs_other=4") ||
		!strings.Contains(out, "conn=5") {
		t.Fatalf("expected handshake counters in stats summary, got:\n%s", out)
	}
}

func TestRuntimeStatsSummaryBlockIncludesReadableBreakdown(t *testing.T) {
	stats := &runtimeStats{
		handshakeWait:    1,
		handshakeEOF:     2,
		handshakeBadVer:  3,
		handshakeOther:   4,
		connections:      10,
		wsConnections:    7,
		wsMedia:          5,
		tcpFallbacks:     3,
		tcpFallbackMedia: 2,
		passthrough:      1,
		httpRejected:     0,
		wsErrors:         2,
		poolHits:         8,
		poolMisses:       9,
		blacklistHits:    1,
		cooldownActivs:   2,
		wsByDC:           map[int]int{2: 6, 203: 1},
		tcpFallbackByDC:  map[int]int{1: 2, 5: 1},
		errorCounts:      map[string]int{"tcp_fb_timeout": 4, "ws_connect_reset": 1, "mtproto_init_eof": 2},
	}

	out := stats.summaryBlock(3, 4)
	for _, want := range []string{
		"stats:",
		"handshake  wait=1 eof=2 badver=3 other=4",
		"routes     conn=10 ws=7 tcp_fb=3 passthrough=1 http_reject=0",
		"media      ws=5 tcp_fb=2",
		"dc         ws{2=6, 203=1} tcp_fb{1=2, 5=1}",
		"probe      init{eof=2}",
		"errors     tcp_fb_timeout=4, mtproto_init_eof=2, ws_connect_reset=1",
		"state      ws_err=2 pool_hit=8 pool_miss=9 blacklist_hit=1 cooldown_set=2 blacklist=3 cooldown=4",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in summary block, got:\n%s", want, out)
		}
	}
}

func TestHandleConnPassthroughErrorStaysOutOfNormalLog(t *testing.T) {
	var logs bytes.Buffer
	srv := NewServer(config.Default(), log.New(&logs, "", 0))
	srv.proxyTCPFunc = func(ctx context.Context, conn net.Conn, host string, port int) error {
		return &net.DNSError{IsTimeout: true}
	}

	runHandleConnFlow(t, srv, ipv4ConnectRequest("8.8.8.8", 443), nil, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	out := logs.String()
	if strings.Contains(out, "passthrough failed:") {
		t.Fatalf("expected non-verbose mode to suppress per-connection passthrough errors, got:\n%s", out)
	}

	stats := srv.stats.summaryBlock(0, 0)
	if !strings.Contains(stats, "errors     passthrough_timeout=1") {
		t.Fatalf("expected summary block to include aggregated passthrough timeout, got:\n%s", stats)
	}
	if !strings.Contains(stats, "probe      passthrough{timeout=1}") {
		t.Fatalf("expected summary block to include passthrough probe breakdown, got:\n%s", stats)
	}
}

func TestHandleConnPassthroughErrorAggregatesInVerboseMode(t *testing.T) {
	var logs bytes.Buffer
	cfg := config.Default()
	cfg.Verbose = true
	srv := NewServer(cfg, log.New(&logs, "", 0))
	srv.proxyTCPFunc = func(ctx context.Context, conn net.Conn, host string, port int) error {
		return &net.DNSError{IsTimeout: true}
	}

	runHandleConnFlow(t, srv, ipv4ConnectRequest("8.8.8.8", 443), nil, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})
	srv.flushVerboseConnFailureSummary()

	out := logs.String()
	if strings.Contains(out, "passthrough failed:") {
		t.Fatalf("expected verbose mode to aggregate passthrough errors, got:\n%s", out)
	}
	if !strings.Contains(out, "passthrough_timeout x1 in 5s") {
		t.Fatalf("expected verbose summary for passthrough timeout, got:\n%s", out)
	}
}

func TestVerboseDebugEventsAggregateByNormalizedMessage(t *testing.T) {
	var logs bytes.Buffer
	cfg := config.Default()
	cfg.Verbose = true
	srv := NewServer(cfg, log.New(&logs, "", 0))

	srv.debugf("[%s] socks connect request to %s:%d", "192.168.1.10:12345", "91.105.192.100", 443)
	srv.debugf("[%s] socks connect request to %s:%d", "192.168.1.11:23456", "91.105.192.100", 443)
	srv.debugf("accepted connection from %s", "192.168.1.10:12345")
	srv.debugf("accepted connection from %s", "192.168.1.11:23456")
	srv.flushVerboseDebugSummary()

	out := logs.String()
	if !strings.Contains(out, "socks connect request to 91.105.192.100:443 x2 in 5s") {
		t.Fatalf("expected normalized socks request aggregation, got:\n%s", out)
	}
	if !strings.Contains(out, "accepted connection from <client> x2 in 5s") {
		t.Fatalf("expected accepted connection aggregation, got:\n%s", out)
	}
}
