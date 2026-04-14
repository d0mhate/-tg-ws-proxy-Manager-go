package socks5

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/mtproto"
	"tg-ws-proxy/internal/wsbridge"
)

func TestHandshakeDomainConnect(t *testing.T) {
	req, err := runHandshakeClient(t, []byte{
		0x05, 0x01, 0x00,
		0x05, 0x01, 0x00, 0x03,
		0x0c,
		't', 'e', 'l', 'e', 'g', 'r', 'a', 'm', '.', 'o', 'r', 'g',
		0x01, 0xbb,
	})
	if err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}

	if req.DstHost != "telegram.org" {
		t.Fatalf("unexpected destination host: %q", req.DstHost)
	}
	if req.DstPort != 443 {
		t.Fatalf("unexpected destination port: %d", req.DstPort)
	}
}

func TestHandshakeIPv4Connect(t *testing.T) {
	req, err := runHandshakeClient(t, []byte{
		0x05, 0x01, 0x00,
		0x05, 0x01, 0x00, 0x01,
		149, 154, 167, 220,
		0x01, 0xbb,
	})
	if err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}

	if req.DstHost != "149.154.167.220" {
		t.Fatalf("unexpected destination host: %q", req.DstHost)
	}
	if req.DstPort != 443 {
		t.Fatalf("unexpected destination port: %d", req.DstPort)
	}
}

func TestHandshakeUDPAssociate(t *testing.T) {
	req, err := runHandshakeClient(t, []byte{
		0x05, 0x01, 0x00,
		0x05, 0x03, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}

	if req.Cmd != socksCmdUDPAssociate {
		t.Fatalf("unexpected command: %d", req.Cmd)
	}
	if req.DstHost != "0.0.0.0" {
		t.Fatalf("unexpected destination host: %q", req.DstHost)
	}
	if req.DstPort != 0 {
		t.Fatalf("unexpected destination port: %d", req.DstPort)
	}
}

func TestHandshakeRejectsUnsupportedCommand(t *testing.T) {
	_, err := runHandshakeClient(t, []byte{
		0x05, 0x01, 0x00,
		0x05, 0x02, 0x00, 0x01,
	})
	if err == nil {
		t.Fatal("expected handshake to reject unsupported command")
	}
}

func TestHandshakeUsernamePasswordConnect(t *testing.T) {
	cfg := config.Default()
	cfg.Username = "alice"
	cfg.Password = "secret"

	req, err := runHandshakeClientWithConfig(t, cfg,
		[]byte{0x05, 0x02, socksAuthNoAuth, socksAuthUserPass},
		buildUserPassAuthPayload("alice", "secret"),
		ipv4ConnectRequest("149.154.167.220", 443),
	)
	if err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}
	if req.DstHost != "149.154.167.220" || req.DstPort != 443 {
		t.Fatalf("unexpected request parsed after auth: %s:%d", req.DstHost, req.DstPort)
	}
}

func TestHandshakeAcceptsNoAuthWhenServerDoesNotRequireCredentials(t *testing.T) {
	req, err := runHandshakeClientWithConfig(t, config.Default(),
		[]byte{0x05, 0x02, socksAuthNoAuth, socksAuthUserPass},
		nil,
		ipv4ConnectRequest("149.154.167.220", 443),
	)
	if err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}
	if req.DstHost != "149.154.167.220" || req.DstPort != 443 {
		t.Fatalf("unexpected request parsed without auth: %s:%d", req.DstHost, req.DstPort)
	}
}

func TestHandshakeRejectsMissingUsernamePasswordMethod(t *testing.T) {
	cfg := config.Default()
	cfg.Username = "alice"
	cfg.Password = "secret"

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		if _, err := clientConn.Write([]byte{0x05, 0x01, socksAuthNoAuth}); err != nil {
			errCh <- err
			return
		}
		reply := make([]byte, 2)
		if _, err := io.ReadFull(clientConn, reply); err != nil {
			errCh <- err
			return
		}
		if !bytes.Equal(reply, []byte{0x05, 0xff}) {
			errCh <- errors.New("unexpected auth method rejection reply")
			return
		}
	}()

	_, err := handshake(serverConn, cfg)
	if err == nil {
		t.Fatal("expected handshake to reject missing username/password method")
	}
	if clientErr := <-errCh; clientErr != nil {
		t.Fatalf("client side of handshake failed: %v", clientErr)
	}
}

func TestHandshakeRejectsInvalidUsernamePassword(t *testing.T) {
	cfg := config.Default()
	cfg.Username = "alice"
	cfg.Password = "secret"

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		if _, err := clientConn.Write([]byte{0x05, 0x01, socksAuthUserPass}); err != nil {
			errCh <- err
			return
		}
		reply := make([]byte, 2)
		if _, err := io.ReadFull(clientConn, reply); err != nil {
			errCh <- err
			return
		}
		if !bytes.Equal(reply, []byte{0x05, socksAuthUserPass}) {
			errCh <- errors.New("unexpected auth method selection reply")
			return
		}
		if _, err := clientConn.Write(buildUserPassAuthPayload("alice", "wrong")); err != nil {
			errCh <- err
			return
		}
		authReply := make([]byte, 2)
		if _, err := io.ReadFull(clientConn, authReply); err != nil {
			errCh <- err
			return
		}
		if !bytes.Equal(authReply, []byte{0x01, 0x01}) {
			errCh <- errors.New("unexpected auth rejection reply")
			return
		}
	}()

	_, err := handshake(serverConn, cfg)
	if err == nil {
		t.Fatal("expected handshake to reject invalid username/password")
	}
	if clientErr := <-errCh; clientErr != nil {
		t.Fatalf("client side of handshake failed: %v", clientErr)
	}
}

func TestHandleConnPassthroughRoute(t *testing.T) {
	var called struct {
		host string
		port int
	}

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	srv.proxyTCPFunc = func(ctx context.Context, conn net.Conn, host string, port int) error {
		called.host = host
		called.port = port
		return nil
	}

	runHandleConnFlow(t, srv, ipv4ConnectRequest("8.8.8.8", 443), nil, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if called.host != "8.8.8.8" || called.port != 443 {
		t.Fatalf("unexpected passthrough target: %s:%d", called.host, called.port)
	}
}

func TestHandleConnPassthroughRouteWithAuth(t *testing.T) {
	var called struct {
		host string
		port int
	}

	cfg := config.Default()
	cfg.Username = "alice"
	cfg.Password = "secret"
	srv := NewServer(cfg, log.New(io.Discard, "", 0))
	srv.proxyTCPFunc = func(ctx context.Context, conn net.Conn, host string, port int) error {
		called.host = host
		called.port = port
		return nil
	}

	runHandleConnFlowWithAuth(t, srv, ipv4ConnectRequest("8.8.8.8", 443), nil, "alice", "secret", func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if called.host != "8.8.8.8" || called.port != 443 {
		t.Fatalf("unexpected passthrough target: %s:%d", called.host, called.port)
	}
}

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

func TestHandleConnTelegramFallbackWithoutOverride(t *testing.T) {
	var got struct {
		host string
		port int
		init []byte
	}

	srv := NewServer(config.Config{
		Host:        "127.0.0.1",
		Port:        1080,
		DialTimeout: time.Second,
		InitTimeout: time.Second,
		DCIPs:       map[int]string{},
	}, log.New(io.Discard, "", 0))
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		got.host = host
		got.port = port
		got.init = append([]byte(nil), init...)
		return nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 5)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.171.5", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if got.host != "149.154.171.5" || got.port != 443 {
		t.Fatalf("unexpected tcp fallback target: %s:%d", got.host, got.port)
	}
	if !bytes.Equal(got.init, init) {
		t.Fatal("expected original init packet to be forwarded to tcp fallback")
	}
}

func TestHandleConnTelegramFallbackAfterWSFailure(t *testing.T) {
	var got struct {
		host string
		port int
		init []byte
	}

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		return nil, io.EOF
	}
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		got.host = host
		got.port = port
		got.init = append([]byte(nil), init...)
		return nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 2)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.167.41", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if got.host != "149.154.167.41" || got.port != 443 {
		t.Fatalf("unexpected fallback target after ws failure: %s:%d", got.host, got.port)
	}
	if !bytes.Equal(got.init, init) {
		t.Fatal("expected init packet to be forwarded after ws failure")
	}
}

func TestHandleConnUnknownIPWithMTProtoInitRoutesAsTelegram(t *testing.T) {
	var got struct {
		host string
		port int
		init []byte
	}

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		return nil, io.EOF
	}
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		got.host = host
		got.port = port
		got.init = append([]byte(nil), init...)
		return nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 2)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("203.0.113.10", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if got.host != "149.154.167.220" || got.port != 443 {
		t.Fatalf("unexpected tcp fallback target for mtproto probe route: %s:%d", got.host, got.port)
	}
	if !bytes.Equal(got.init, init) {
		t.Fatal("expected mtproto init to be forwarded on telegram route inferred by probe")
	}
}

func TestHandleConnFallsBackForTelegramHTTPTransport(t *testing.T) {
	var got struct {
		host string
		port int
		init []byte
	}

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		got.host = host
		got.port = port
		got.init = append([]byte(nil), init...)
		return nil
	}

	init := append([]byte("GET / HTTP/1.1"), bytes.Repeat([]byte{0}, 64-len("GET / HTTP/1.1"))...)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.167.41", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if got.host != "149.154.167.41" || got.port != 443 {
		t.Fatalf("unexpected tcp fallback target for telegram http transport: %s:%d", got.host, got.port)
	}
	if !bytes.Equal(got.init, init) {
		t.Fatal("expected http transport bytes to be forwarded to tcp fallback")
	}
}

func TestHandleConnUnknownIPHTTPProbeFallsBackToPassthrough(t *testing.T) {
	var got struct {
		host string
		port int
		init []byte
	}

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		got.host = host
		got.port = port
		got.init = append([]byte(nil), init...)
		return nil
	}

	httpProbe := append([]byte("GET / HTTP/1.1"), bytes.Repeat([]byte{0}, 64-len("GET / HTTP/1.1"))...)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("203.0.113.10", 443), httpProbe, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if got.host != "203.0.113.10" || got.port != 443 {
		t.Fatalf("unexpected passthrough target after http probe: %s:%d", got.host, got.port)
	}
	if !bytes.Equal(got.init, httpProbe) {
		t.Fatal("expected probe bytes to be forwarded to passthrough target")
	}
}

func TestHandleConnPassesThroughNonTelegramIPv6Destination(t *testing.T) {
	var called struct {
		host string
		port int
	}

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	srv.proxyTCPFunc = func(ctx context.Context, conn net.Conn, host string, port int) error {
		called.host = host
		called.port = port
		return nil
	}

	runHandleConnFlow(t, srv, ipv6ConnectRequestWithPort(net.ParseIP("2001:db8::1"), 8443), nil, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if called.host != "2001:db8::1" || called.port != 8443 {
		t.Fatalf("unexpected ipv6 passthrough target: %s:%d", called.host, called.port)
	}
}

func TestHandleConnTelegramIPv6FallbackUsesIPv4DCTarget(t *testing.T) {
	var got struct {
		host string
		port int
		init []byte
	}

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		return nil, io.EOF
	}
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		got.host = host
		got.port = port
		got.init = append([]byte(nil), init...)
		return nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 2)
	runHandleConnFlow(t, srv, ipv6ConnectRequest(net.ParseIP("2001:db8::1")), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if got.host != "149.154.167.220" || got.port != 443 {
		t.Fatalf("unexpected tcp fallback target for telegram ipv6: %s:%d", got.host, got.port)
	}
	if !bytes.Equal(got.init, init) {
		t.Fatal("expected init packet to be forwarded to ipv4 dc target")
	}
}

func TestHandleConnSkipsWSForDisabledDCAndUsesTCPFallback(t *testing.T) {
	var got struct {
		host string
		port int
		init []byte
	}

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		t.Fatal("did not expect websocket dial for disabled dc")
		return nil, nil
	}
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		got.host = host
		got.port = port
		got.init = append([]byte(nil), init...)
		return nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, -1)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.175.211", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if got.host != "149.154.175.211" || got.port != 443 {
		t.Fatalf("unexpected tcp fallback target for disabled dc: %s:%d", got.host, got.port)
	}
	if !bytes.Equal(got.init, init) {
		t.Fatal("expected init packet to be forwarded when ws is disabled for dc")
	}
}

func TestHandleConnDC203UsesDC2OverrideTargetAndPatchedInit(t *testing.T) {
	var got struct {
		host string
		port int
		init []byte
	}

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		return nil, io.EOF
	}
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		got.host = host
		got.port = port
		got.init = append([]byte(nil), init...)
		return nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 203)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("91.105.192.100", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if got.host != "149.154.167.220" || got.port != 443 {
		t.Fatalf("unexpected tcp fallback target for dc203: %s:%d", got.host, got.port)
	}

	info, err := mtproto.ParseInit(got.init)
	if err != nil {
		t.Fatalf("expected patched init to parse, got %v", err)
	}
	if info.DC != 2 || info.IsMedia {
		t.Fatalf("expected patched init to use dc2 non-media, got %+v", info)
	}
}

func TestHandleConnDC203UsesExplicitTargetAndKeepsPatchedDCWhenConfigured(t *testing.T) {
	var got struct {
		host string
		port int
		init []byte
	}

	cfg := config.Default()
	cfg.DCIPs[203] = "91.105.192.100"

	srv := NewServer(cfg, log.New(io.Discard, "", 0))
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		return nil, io.EOF
	}
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		got.host = host
		got.port = port
		got.init = append([]byte(nil), init...)
		return nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 203)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("91.105.192.100", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if got.host != "91.105.192.100" || got.port != 443 {
		t.Fatalf("unexpected tcp fallback target for explicit dc203: %s:%d", got.host, got.port)
	}

	info, err := mtproto.ParseInit(got.init)
	if err != nil {
		t.Fatalf("expected patched init to parse, got %v", err)
	}
	if info.DC != 203 || info.IsMedia {
		t.Fatalf("expected patched init to keep dc203 non-media, got %+v", info)
	}
}

func TestHandleConnDC203UsesExplicitTargetForWebSocketRoute(t *testing.T) {
	var got struct {
		targetIP string
		dc       int
		isMedia  bool
	}

	cfg := config.Default()
	cfg.DCIPs[203] = "91.105.192.100"

	srv := NewServer(cfg, log.New(io.Discard, "", 0))
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		t.Fatal("did not expect tcp fallback for explicit dc203 websocket route")
		return nil
	}
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		got.targetIP = targetIP
		got.dc = dc
		got.isMedia = isMedia
		clientConn, peerConn := net.Pipe()
		go func() { _ = peerConn.Close() }()
		return wsbridge.NewClient(clientConn), nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 203)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("91.105.192.100", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if got.targetIP != "91.105.192.100" || got.dc != 203 || got.isMedia {
		t.Fatalf("unexpected websocket route for explicit dc203: %+v", got)
	}
}

func TestHandleConnAdditionalTelegramCallHostsUseKnownDCMappings(t *testing.T) {
	t.Run("dc2 host routes through websocket target", func(t *testing.T) {
		var got struct {
			targetIP string
			dc       int
			isMedia  bool
		}

		srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
		srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
			got.targetIP = targetIP
			got.dc = dc
			got.isMedia = isMedia
			clientConn, peerConn := net.Pipe()
			go func() { _ = peerConn.Close() }()
			return wsbridge.NewClient(clientConn), nil
		}

		init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 0)
		runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.167.255", 443), init, func(reply []byte) {
			if reply[1] != 0x00 {
				t.Fatalf("unexpected socks reply status: %d", reply[1])
			}
		})

		if got.targetIP != "149.154.167.220" || got.dc != 2 || got.isMedia {
			t.Fatalf("unexpected websocket route: %+v", got)
		}
	})

	t.Run("dc1 host routes through dc target fallback", func(t *testing.T) {
		var got struct {
			host string
			port int
			init []byte
		}

		srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
		srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
			t.Fatal("did not expect websocket dial for dc1 host")
			return nil, nil
		}
		srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
			got.host = host
			got.port = port
			got.init = append([]byte(nil), init...)
			return nil
		}

		init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 0)
		runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.175.211", 443), init, func(reply []byte) {
			if reply[1] != 0x00 {
				t.Fatalf("unexpected socks reply status: %d", reply[1])
			}
		})

		if got.host != "149.154.175.205" || got.port != 443 {
			t.Fatalf("unexpected tcp fallback target: %s:%d", got.host, got.port)
		}
		info, err := mtproto.ParseInit(got.init)
		if err != nil {
			t.Fatalf("expected patched init to parse, got %v", err)
		}
		if info.DC != 1 || info.IsMedia {
			t.Fatalf("expected patched init to use dc1 non-media, got %+v", info)
		}
	})
}

func TestChoosePatchedDC(t *testing.T) {
	if got := choosePatchedDC(5, true); got != -5 {
		t.Fatalf("unexpected media patched dc: %d", got)
	}
	if got := choosePatchedDC(2, false); got != 2 {
		t.Fatalf("unexpected non-media patched dc: %d", got)
	}
}

func TestWriteReplyUsesGeneralFailureForUnknownStatus(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := writeReply(serverConn, 0xff); err != nil {
			t.Errorf("writeReply returned error: %v", err)
		}
	}()

	reply := make([]byte, 10)
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("failed to read reply: %v", err)
	}
	wg.Wait()

	if reply[1] != 0x05 {
		t.Fatalf("unexpected fallback reply status: %d", reply[1])
	}
}

func TestHandleConnUDPAssociateRelaysDatagrams(t *testing.T) {
	echoPC, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start udp echo server: %v", err)
	}
	defer echoPC.Close()

	echoDone := make(chan struct{})
	go func() {
		defer close(echoDone)
		buf := make([]byte, 2048)
		n, src, readErr := echoPC.ReadFrom(buf)
		if readErr != nil {
			return
		}
		_, _ = echoPC.WriteTo(buf[:n], src)
	}()

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for tcp control channel: %v", err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		srv.handleConn(ctx, conn)
	}()

	controlConn, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect tcp control channel: %v", err)
	}

	if _, err := controlConn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("failed to send auth greeting: %v", err)
	}

	authReply := make([]byte, 2)
	if _, err := io.ReadFull(controlConn, authReply); err != nil {
		t.Fatalf("failed to read auth reply: %v", err)
	}
	if !bytes.Equal(authReply, []byte{0x05, 0x00}) {
		t.Fatalf("unexpected auth reply: %v", authReply)
	}

	if _, err := controlConn.Write([]byte{
		0x05, 0x03, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
	}); err != nil {
		t.Fatalf("failed to send udp associate request: %v", err)
	}

	bindHost, bindPort := readSocksReplyAddr(t, controlConn)
	if bindPort == 0 {
		t.Fatal("expected udp associate bind port")
	}

	clientPC, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open udp client socket: %v", err)
	}
	defer clientPC.Close()

	payload := []byte("hello over udp associate")
	echoAddr := echoPC.LocalAddr().(*net.UDPAddr)
	packet, err := buildUDPAssociatePacket(echoAddr.IP.String(), echoAddr.Port, payload)
	if err != nil {
		t.Fatalf("failed to build udp associate packet: %v", err)
	}

	udpBindAddr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(bindHost, strconv.Itoa(bindPort)))
	if err != nil {
		t.Fatalf("failed to resolve udp bind addr: %v", err)
	}
	if _, err := clientPC.WriteTo(packet, udpBindAddr); err != nil {
		t.Fatalf("failed to send udp associate payload: %v", err)
	}

	buf := make([]byte, 2048)
	_ = clientPC.SetDeadline(time.Now().Add(2 * time.Second))
	n, _, err := clientPC.ReadFrom(buf)
	if err != nil {
		t.Fatalf("failed to read udp associate response: %v", err)
	}

	resp, err := parseUDPAssociatePacket(buf[:n])
	if err != nil {
		t.Fatalf("failed to parse udp associate response: %v", err)
	}
	if resp.Host != echoAddr.IP.String() || resp.Port != echoAddr.Port {
		t.Fatalf("unexpected udp response source: %s:%d", resp.Host, resp.Port)
	}
	if !bytes.Equal(resp.Payload, payload) {
		t.Fatalf("unexpected udp response payload: %q", resp.Payload)
	}

	_ = controlConn.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("udp associate handler did not complete after control channel close")
	}
	select {
	case <-echoDone:
	case <-time.After(2 * time.Second):
		t.Fatal("udp echo server did not complete")
	}
}

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

func TestHandleConnCFFallbackTriedBeforeTCP(t *testing.T) {
	cfg := config.Default()
	cfg.UseCFProxy = true
	cfg.CFDomains = []string{"example.com"}

	var order []string
	srv := NewServer(cfg, log.New(io.Discard, "", 0))
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		order = append(order, "ws")
		return nil, io.EOF
	}
	srv.wsDialFunc = func(ctx context.Context, dialCfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		order = append(order, "cf")
		return nil, io.EOF
	}
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		order = append(order, "tcp")
		return nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 2)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.167.220", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	want := []string{"ws", "cf", "tcp"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("unexpected fallback order: got %v want %v", order, want)
	}
}

func TestHandleConnTCPFallbackWhenCFDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.UseCFProxy = false

	var order []string
	srv := NewServer(cfg, log.New(io.Discard, "", 0))
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		order = append(order, "ws")
		return nil, io.EOF
	}
	srv.wsDialFunc = func(ctx context.Context, dialCfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		order = append(order, "cf")
		return nil, io.EOF
	}
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		order = append(order, "tcp")
		return nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 2)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.167.220", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	want := []string{"ws", "tcp"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("unexpected order when CF disabled: got %v want %v", order, want)
	}
}

func TestHandleConnCFPreferredBeforeTelegramWS(t *testing.T) {
	cfg := config.Default()
	cfg.UseCFProxy = true
	cfg.UseCFProxyFirst = true
	cfg.CFDomains = []string{"example.com"}

	var order []string
	srv := NewServer(cfg, log.New(io.Discard, "", 0))
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		order = append(order, "ws")
		return nil, io.EOF
	}
	srv.wsDialFunc = func(ctx context.Context, dialCfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
		order = append(order, "cf")
		return nil, io.EOF
	}
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		order = append(order, "tcp")
		return nil
	}

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 2)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.167.220", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	want := []string{"cf", "ws", "tcp"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("unexpected fallback order with CF preferred: got %v want %v", order, want)
	}
}

func runHandshakeClient(t *testing.T, request []byte) (requestOut request, err error) {
	t.Helper()
	return runHandshakeClientWithConfig(t, config.Default(), request[:3], nil, request[3:])
}

func runHandshakeClientWithConfig(t *testing.T, cfg config.Config, greeting []byte, authPayload []byte, requestPayload []byte) (requestOut request, err error) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientErrCh := make(chan error, 1)
	go func() {
		defer close(clientErrCh)

		if _, writeErr := clientConn.Write(greeting); writeErr != nil {
			clientErrCh <- writeErr
			return
		}

		reply := make([]byte, 2)
		if _, readErr := io.ReadFull(clientConn, reply); readErr != nil {
			clientErrCh <- readErr
			return
		}
		expectedMethod := byte(socksAuthNoAuth)
		if cfg.Username != "" || cfg.Password != "" {
			expectedMethod = socksAuthUserPass
		}
		if !bytes.Equal(reply, []byte{0x05, expectedMethod}) {
			clientErrCh <- io.ErrUnexpectedEOF
			return
		}

		if expectedMethod == socksAuthUserPass {
			if _, writeErr := clientConn.Write(authPayload); writeErr != nil {
				clientErrCh <- writeErr
				return
			}
			authReply := make([]byte, 2)
			if _, readErr := io.ReadFull(clientConn, authReply); readErr != nil {
				clientErrCh <- readErr
				return
			}
			if !bytes.Equal(authReply, []byte{0x01, 0x00}) {
				clientErrCh <- io.ErrUnexpectedEOF
				return
			}
		}

		if _, writeErr := clientConn.Write(requestPayload); writeErr != nil {
			clientErrCh <- writeErr
			return
		}
	}()

	requestOut, err = handshake(serverConn, cfg)
	if clientErr := <-clientErrCh; clientErr != nil {
		t.Fatalf("client side of handshake failed: %v", clientErr)
	}
	return requestOut, err
}

func runHandleConnFlow(t *testing.T, srv *Server, connectReq []byte, init []byte, assertReply func([]byte)) {
	t.Helper()
	runHandleConnFlowWithAuth(t, srv, connectReq, init, "", "", assertReply)
}

func runHandleConnFlowWithAuth(t *testing.T, srv *Server, connectReq []byte, init []byte, username string, password string, assertReply func([]byte)) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(ctx, serverConn)
	}()

	greeting := []byte{0x05, 0x01, 0x00}
	expectedMethod := byte(socksAuthNoAuth)
	if username != "" || password != "" {
		greeting = []byte{0x05, 0x02, socksAuthNoAuth, socksAuthUserPass}
		expectedMethod = socksAuthUserPass
	}
	if _, err := clientConn.Write(greeting); err != nil {
		t.Fatalf("failed to send auth greeting: %v", err)
	}

	authReply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, authReply); err != nil {
		t.Fatalf("failed to read auth reply: %v", err)
	}
	if !bytes.Equal(authReply, []byte{0x05, expectedMethod}) {
		t.Fatalf("unexpected auth reply: %v", authReply)
	}
	if expectedMethod == socksAuthUserPass {
		if _, err := clientConn.Write(buildUserPassAuthPayload(username, password)); err != nil {
			t.Fatalf("failed to send username/password auth payload: %v", err)
		}
		userPassReply := make([]byte, 2)
		if _, err := io.ReadFull(clientConn, userPassReply); err != nil {
			t.Fatalf("failed to read username/password auth reply: %v", err)
		}
		if !bytes.Equal(userPassReply, []byte{0x01, 0x00}) {
			t.Fatalf("unexpected username/password auth reply: %v", userPassReply)
		}
	}

	if _, err := clientConn.Write(connectReq); err != nil {
		t.Fatalf("failed to send connect request: %v", err)
	}

	reply := make([]byte, 10)
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("failed to read connect reply: %v", err)
	}
	if assertReply != nil {
		assertReply(reply)
	}

	if len(init) > 0 {
		if _, err := clientConn.Write(init); err != nil {
			t.Fatalf("failed to send init packet: %v", err)
		}
	}

	_ = clientConn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not complete")
	}
}

func buildUserPassAuthPayload(username, password string) []byte {
	out := []byte{0x01, byte(len(username))}
	out = append(out, []byte(username)...)
	out = append(out, byte(len(password)))
	out = append(out, []byte(password)...)
	return out
}

func runHandleConnInvalidAuthAttempt(t *testing.T, srv *Server, username string, password string) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(ctx, serverConn)
	}()

	if _, err := clientConn.Write([]byte{0x05, 0x02, socksAuthNoAuth, socksAuthUserPass}); err != nil {
		t.Fatalf("failed to send auth greeting: %v", err)
	}

	authReply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, authReply); err != nil {
		t.Fatalf("failed to read auth reply: %v", err)
	}
	if !bytes.Equal(authReply, []byte{0x05, socksAuthUserPass}) {
		t.Fatalf("unexpected auth reply: %v", authReply)
	}

	if _, err := clientConn.Write(buildUserPassAuthPayload(username, password)); err != nil {
		t.Fatalf("failed to send invalid username/password auth payload: %v", err)
	}

	userPassReply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, userPassReply); err != nil {
		t.Fatalf("failed to read invalid username/password auth reply: %v", err)
	}
	if !bytes.Equal(userPassReply, []byte{0x01, 0x01}) {
		t.Fatalf("unexpected invalid username/password auth reply: %v", userPassReply)
	}

	_ = clientConn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not complete after invalid auth attempt")
	}
}

func runHandleConnRawHandshakeAttempt(t *testing.T, srv *Server, payload []byte) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(ctx, serverConn)
	}()

	if len(payload) > 0 {
		if _, err := clientConn.Write(payload); err != nil {
			t.Fatalf("failed to send raw handshake payload: %v", err)
		}
	}

	_ = clientConn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not complete after raw handshake attempt")
	}
}

func ipv4ConnectRequest(ip string, port int) []byte {
	out := []byte{0x05, 0x01, 0x00, 0x01}
	out = append(out, net.ParseIP(ip).To4()...)
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	out = append(out, portBuf[:]...)
	return out
}

func ipv6ConnectRequest(ip net.IP) []byte {
	return ipv6ConnectRequestWithPort(ip, 443)
}

func ipv6ConnectRequestWithPort(ip net.IP, port int) []byte {
	out := []byte{0x05, 0x01, 0x00, 0x04}
	out = append(out, ip.To16()...)
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	out = append(out, portBuf[:]...)
	return out
}

func makeMTProtoInitPacket(t *testing.T, proto uint32, dc int16) []byte {
	t.Helper()

	init := make([]byte, 64)
	for i := range init {
		init[i] = byte(i + 1)
	}

	var plain [8]byte
	binary.LittleEndian.PutUint32(plain[:4], proto)
	binary.LittleEndian.PutUint16(plain[4:6], uint16(dc))

	keystream := initKeystreamForTest(t, init)
	for i := 0; i < len(plain); i++ {
		init[56+i] = plain[i] ^ keystream[56+i]
	}
	return init
}

func initKeystreamForTest(t *testing.T, init []byte) []byte {
	t.Helper()

	block, err := aes.NewCipher(init[8:40])
	if err != nil {
		t.Fatalf("aes.NewCipher failed: %v", err)
	}
	stream := cipher.NewCTR(block, init[40:56])
	zero := make([]byte, 64)
	keystream := make([]byte, 64)
	stream.XORKeyStream(keystream, zero)
	return keystream
}

func readSocksReplyAddr(t *testing.T, conn net.Conn) (string, int) {
	t.Helper()

	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		t.Fatalf("failed to read socks reply head: %v", err)
	}
	if head[0] != 0x05 {
		t.Fatalf("unexpected socks version in reply: %d", head[0])
	}
	if head[1] != 0x00 {
		t.Fatalf("unexpected socks reply status: %d", head[1])
	}

	var host string
	switch head[3] {
	case 0x01:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			t.Fatalf("failed to read ipv4 reply addr: %v", err)
		}
		host = net.IP(addr).String()
	case 0x04:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			t.Fatalf("failed to read ipv6 reply addr: %v", err)
		}
		host = net.IP(addr).String()
	case 0x03:
		size := make([]byte, 1)
		if _, err := io.ReadFull(conn, size); err != nil {
			t.Fatalf("failed to read domain size: %v", err)
		}
		addr := make([]byte, int(size[0]))
		if _, err := io.ReadFull(conn, addr); err != nil {
			t.Fatalf("failed to read domain reply addr: %v", err)
		}
		host = string(addr)
	default:
		t.Fatalf("unexpected socks reply atyp: %d", head[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		t.Fatalf("failed to read reply port: %v", err)
	}
	return host, int(binary.BigEndian.Uint16(portBuf))
}
