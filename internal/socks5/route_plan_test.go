package socks5

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

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
	cfg.DCIPs = map[int]string{2: "149.154.167.40"}
	srv := NewServer(cfg, log.New(io.Discard, "", 0))

	init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 0)

	plan := srv.buildTelegramRoutePlan(request{
		DstHost: "149.154.167.220",
		DstPort: 443,
	}, init, false, false, "client")

	if plan.dc != 2 {
		t.Fatalf("expected destination lookup to infer dc=2, got %d", plan.dc)
	}
	if plan.effectiveDC != 2 {
		t.Fatalf("expected effective dc=2, got %d", plan.effectiveDC)
	}
	if plan.targetIP != "149.154.167.40" {
		t.Fatalf("unexpected target ip: %q", plan.targetIP)
	}
	if !plan.inferredFromDestination {
		t.Fatal("expected plan to note destination-based inference")
	}
	if !plan.initPatched {
		t.Fatal("expected init to be patched after destination-based inference")
	}
}
