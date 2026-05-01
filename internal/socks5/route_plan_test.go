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
	"tg-ws-proxy/internal/mtproto"
)

func TestChoosePatchedDC(t *testing.T) {
	if got := choosePatchedDC(5, true); got != -5 {
		t.Fatalf("unexpected media patched dc: %d", got)
	}
	if got := choosePatchedDC(2, false); got != 2 {
		t.Fatalf("unexpected non-media patched dc: %d", got)
	}
}

func TestClassifyRuntimeError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: "probe_other"},
		{name: "canceled", err: context.Canceled, want: "probe_canceled"},
		{name: "timeout text", err: errors.New("dial tcp: i/o timeout"), want: "probe_timeout"},
		{name: "no route", err: errors.New("connect: no route to host"), want: "probe_no_route"},
		{name: "reset", err: errors.New("read: connection reset by peer"), want: "probe_reset"},
		{name: "eof", err: io.EOF, want: "probe_eof"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyRuntimeError("probe", tc.err); got != tc.want {
				t.Fatalf("unexpected classification: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestClassifyInitialRoute(t *testing.T) {
	isTelegramCandidate, shouldProbeMTProto, isIPv6 := classifyInitialRoute(request{
		DstHost: "2001:4860:4860::8888",
		DstPort: 443,
	})
	if !isTelegramCandidate {
		t.Fatal("expected IPv6 port 443 destination to be treated as a telegram candidate")
	}
	if shouldProbeMTProto {
		t.Fatal("did not expect MTProto probe for telegram candidate")
	}
	if !isIPv6 {
		t.Fatal("expected IPv6 destination to be detected")
	}
}

func TestBuildTelegramRoutePlanInfersDCFromDestination(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs = map[int]string{2: testIPv4DC2AltRoute}
	srv := NewServer(cfg, log.New(io.Discard, "", 0))

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 0)

	plan := srv.buildTelegramRoutePlan(request{
		DstHost: testIPv4DC2,
		DstPort: 443,
	}, init, false, false, "client")

	if plan.dc != 2 {
		t.Fatalf("expected destination lookup to infer dc=2, got %d", plan.dc)
	}
	if plan.effectiveDC != 2 {
		t.Fatalf("expected effective dc=2, got %d", plan.effectiveDC)
	}
	if plan.targetIP != testIPv4DC2AltRoute {
		t.Fatalf("unexpected target ip: %q", plan.targetIP)
	}
	if !plan.inferredFromDestination {
		t.Fatal("expected plan to note destination-based inference")
	}
	if !plan.initPatched {
		t.Fatal("expected init to be patched after destination-based inference")
	}
}

func TestBuildTelegramRoutePlanUsesTargetAsFallbackForInitOnlyRoute(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs = map[int]string{2: testIPv4DC2}
	srv := NewServer(cfg, log.New(io.Discard, "", 0))

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 2)
	plan := srv.buildTelegramRoutePlan(request{
		DstHost: "203.0.113.10",
		DstPort: 443,
	}, init, false, true, "client")

	if !plan.routeByInitOnly {
		t.Fatal("expected routeByInitOnly to be preserved")
	}
	if plan.fallbackHost != testIPv4DC2 {
		t.Fatalf("expected fallback host to use dc target, got %q", plan.fallbackHost)
	}
}

func TestBuildTelegramRoutePlanKeepsOriginalFallbackHostForDirectTelegramRoute(t *testing.T) {
	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 2)
	plan := srv.buildTelegramRoutePlan(request{
		DstHost: testIPv4DC2,
		DstPort: 443,
	}, init, false, false, "client")

	if plan.fallbackHost != testIPv4DC2 {
		t.Fatalf("expected original destination as fallback host, got %q", plan.fallbackHost)
	}
}

func TestClassifyInitPacket(t *testing.T) {
	mtprotoInit := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 2)
	httpInit := append([]byte("GET / HTTP/1.1"), make([]byte, 64-len("GET / HTTP/1.1"))...)

	tests := []struct {
		name                string
		init                []byte
		isTelegramCandidate bool
		wantAction          initPacketAction
		wantInitOnly        bool
		wantReason          string
	}{
		{
			name:                "non telegram invalid probe becomes passthrough",
			init:                bytesOfLen(64, 0x01),
			isTelegramCandidate: false,
			wantAction:          initPacketPassthrough,
			wantReason:          "mtproto-probe-miss",
		},
		{
			name:                "non telegram mtproto init continues as inferred route",
			init:                mtprotoInit,
			isTelegramCandidate: false,
			wantAction:          initPacketContinue,
			wantInitOnly:        true,
		},
		{
			name:                "telegram http transport uses tcp fallback",
			init:                httpInit,
			isTelegramCandidate: true,
			wantAction:          initPacketTCPFallback,
			wantReason:          "http-transport",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyInitPacket(tc.init, tc.isTelegramCandidate)
			if got.action != tc.wantAction {
				t.Fatalf("unexpected action: got %d want %d", got.action, tc.wantAction)
			}
			if got.routeByInitOnly != tc.wantInitOnly {
				t.Fatalf("unexpected routeByInitOnly: got %v want %v", got.routeByInitOnly, tc.wantInitOnly)
			}
			if got.reason != tc.wantReason {
				t.Fatalf("unexpected reason: got %q want %q", got.reason, tc.wantReason)
			}
		})
	}
}

func TestDecideTelegramWSRoute(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		plan telegramRoutePlan
		want telegramWSRouteDecision
	}{
		{
			name: "no override falls back immediately",
			cfg:  config.Default(),
			plan: telegramRoutePlan{},
			want: telegramWSRouteDecision{action: telegramWSRouteTCPFallbackNoOverride},
		},
		{
			name: "disabled dc falls back to target host",
			cfg:  config.Default(),
			plan: telegramRoutePlan{targetIP: testIPv4DC1Call, fallbackHost: testIPv4DC1Call, wsDomainDC: 1},
			want: telegramWSRouteDecision{action: telegramWSRouteTCPFallbackWSDisabled, fallbackHost: testIPv4DC1Call},
		},
		{
			name: "cloudflare only still attempts websocket route",
			cfg: config.Config{
				UseCFProxy: true,
				CFDomains:  []string{"cf.example.com"},
			},
			plan: telegramRoutePlan{wsDomainDC: 2},
			want: telegramWSRouteDecision{action: telegramWSRouteConnect, allowCloudflareWS: true},
		},
		{
			name: "telegram websocket allowed for enabled dc",
			cfg:  config.Default(),
			plan: telegramRoutePlan{targetIP: testIPv4DC2, fallbackHost: testIPv4DC2, wsDomainDC: 2},
			want: telegramWSRouteDecision{action: telegramWSRouteConnect, allowTelegramWS: true, fallbackHost: testIPv4DC2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decideTelegramWSRoute(tc.cfg, tc.plan)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected websocket route decision: got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestReadAndClassifyInitNonTelegramProbeMissFallsBackToPassthrough(t *testing.T) {
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

	conn := newScriptedReadConn(bytesOfLen(64, 0x01))
	_, handled := srv.readAndClassifyInit(context.Background(), conn, request{DstHost: "203.0.113.10", DstPort: 443}, "client", false)
	if !handled {
		t.Fatal("expected probe miss to be handled")
	}
	if got.host != "203.0.113.10" || got.port != 443 {
		t.Fatalf("unexpected passthrough target: %s:%d", got.host, got.port)
	}
}

func bytesOfLen(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

type scriptedReadConn struct {
	data []byte
	off  int
}

func newScriptedReadConn(data []byte) *scriptedReadConn {
	return &scriptedReadConn{data: append([]byte(nil), data...)}
}

func (c *scriptedReadConn) Read(p []byte) (int, error) {
	if c.off >= len(c.data) {
		return 0, io.EOF
	}
	n := copy(p, c.data[c.off:])
	c.off += n
	return n, nil
}

func (c *scriptedReadConn) Write(p []byte) (int, error)        { return len(p), nil }
func (c *scriptedReadConn) Close() error                       { return nil }
func (c *scriptedReadConn) LocalAddr() net.Addr                { return routePlanDummyAddr("local") }
func (c *scriptedReadConn) RemoteAddr() net.Addr               { return routePlanDummyAddr("remote") }
func (c *scriptedReadConn) SetDeadline(_ time.Time) error      { return nil }
func (c *scriptedReadConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *scriptedReadConn) SetWriteDeadline(_ time.Time) error { return nil }

type routePlanDummyAddr string

func (a routePlanDummyAddr) Network() string { return "tcp" }
func (a routePlanDummyAddr) String() string  { return string(a) }
