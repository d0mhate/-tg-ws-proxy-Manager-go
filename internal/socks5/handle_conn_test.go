package socks5

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"reflect"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/mtproto"
	"tg-ws-proxy/internal/wsbridge"
)

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

		if got.host != "149.154.175.211" || got.port != 443 {
			t.Fatalf("unexpected tcp fallback target: %s:%d", got.host, got.port)
		}
		info, err := mtproto.ParseInit(got.init)
		if err != nil {
			t.Fatalf("expected patched init to parse, got %v", err)
		}
		if info.DC != 0 || info.IsMedia {
			t.Fatalf("expected original init to stay unchanged without direct dc mapping, got %+v", info)
		}
	})

	t.Run("dc1 host routes through cloudflare websocket when enabled", func(t *testing.T) {
		var got struct {
			targetIP string
			domain   string
		}

		cfg := config.Default()
		cfg.UseCFProxy = true
		cfg.CFDomains = []string{"example.com"}

		srv := NewServer(cfg, log.New(io.Discard, "", 0))
		srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
			t.Fatal("did not expect direct telegram websocket dial for dc1 cloudflare route")
			return nil, nil
		}
		srv.wsDialFunc = func(ctx context.Context, dialCfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
			got.targetIP = targetIP
			got.domain = domain
			clientConn, peerConn := net.Pipe()
			go func() { _ = peerConn.Close() }()
			return wsbridge.NewClient(clientConn), nil
		}
		srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
			t.Fatal("did not expect tcp fallback for dc1 cloudflare route")
			return nil
		}

		init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, 0)
		runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.175.211", 443), init, func(reply []byte) {
			if reply[1] != 0x00 {
				t.Fatalf("unexpected socks reply status: %d", reply[1])
			}
		})

		if got.targetIP != "kws1.example.com" || got.domain != "kws1.example.com" {
			t.Fatalf("unexpected cloudflare websocket route: %+v", got)
		}
	})
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

func TestHandleConnCFRouteForNonWhitelistDCs(t *testing.T) {
	cases := []struct {
		name       string
		initDC     int16
		dst        string
		dcIPs      map[int]string
		wantCFHost string
	}{
		{
			name:       "dc1 raw init routes through cloudflare websocket",
			initDC:     1,
			dst:        "149.154.175.50",
			dcIPs:      nil,
			wantCFHost: "kws1.example.com",
		},
		{
			name:       "dc5 raw init routes through cloudflare websocket",
			initDC:     5,
			dst:        "91.108.56.100",
			dcIPs:      nil,
			wantCFHost: "kws5.example.com",
		},
		{
			name:       "dc1 with DCIPs mapping still uses cloudflare websocket",
			initDC:     1,
			dst:        "149.154.175.50",
			dcIPs:      map[int]string{1: "149.154.175.50"},
			wantCFHost: "kws1.example.com",
		},
		{
			name:       "dc5 with DCIPs mapping still uses cloudflare websocket",
			initDC:     5,
			dst:        "91.108.56.100",
			dcIPs:      map[int]string{5: "91.108.56.100"},
			wantCFHost: "kws5.example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.UseCFProxy = true
			cfg.CFDomains = []string{"example.com"}
			if tc.dcIPs != nil {
				cfg.DCIPs = tc.dcIPs
			} else {
				cfg.DCIPs = map[int]string{}
			}

			var got struct {
				targetIP string
				domain   string
			}

			srv := NewServer(cfg, log.New(io.Discard, "", 0))
			srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
				t.Fatalf("did not expect direct telegram websocket dial for dc%d cloudflare route", tc.initDC)
				return nil, nil
			}
			srv.wsDialFunc = func(ctx context.Context, dialCfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
				got.targetIP = targetIP
				got.domain = domain
				clientConn, peerConn := net.Pipe()
				go func() { _ = peerConn.Close() }()
				return wsbridge.NewClient(clientConn), nil
			}
			srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
				t.Fatalf("did not expect tcp fallback for dc%d cloudflare route", tc.initDC)
				return nil
			}

			init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, tc.initDC)
			runHandleConnFlow(t, srv, ipv4ConnectRequest(tc.dst, 443), init, func(reply []byte) {
				if reply[1] != 0x00 {
					t.Fatalf("unexpected socks reply status: %d", reply[1])
				}
			})

			if got.targetIP != tc.wantCFHost || got.domain != tc.wantCFHost {
				t.Fatalf("unexpected cloudflare websocket route: got target=%q domain=%q want %q", got.targetIP, got.domain, tc.wantCFHost)
			}
		})
	}
}

func TestHandleConnCFFallbackUsesDCIPMappingTargetForTCPWhenCFFails(t *testing.T) {
	cases := []struct {
		name        string
		initDC      int16
		dst         string
		dcIPs       map[int]string
		wantTCPHost string
	}{
		{
			name:        "dc1 raw init falls back to dst host when cf fails",
			initDC:      1,
			dst:         "149.154.175.50",
			dcIPs:       nil,
			wantTCPHost: "149.154.175.50",
		},
		{
			name:        "dc5 raw init falls back to dst host when cf fails",
			initDC:      5,
			dst:         "91.108.56.100",
			dcIPs:       nil,
			wantTCPHost: "91.108.56.100",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.UseCFProxy = true
			cfg.CFDomains = []string{"example.com"}
			if tc.dcIPs != nil {
				cfg.DCIPs = tc.dcIPs
			} else {
				cfg.DCIPs = map[int]string{}
			}

			var order []string
			var tcpHost string

			srv := NewServer(cfg, log.New(io.Discard, "", 0))
			srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
				t.Fatalf("did not expect direct telegram websocket dial for dc%d", tc.initDC)
				return nil, nil
			}
			srv.wsDialFunc = func(ctx context.Context, dialCfg config.Config, targetIP string, domain string) (*wsbridge.Client, error) {
				order = append(order, "cf:"+domain)
				return nil, io.EOF
			}
			srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
				order = append(order, "tcp")
				tcpHost = host
				return nil
			}

			init := makeMTProtoInitPacket(t, mtproto.ProtoIntermediate, tc.initDC)
			runHandleConnFlow(t, srv, ipv4ConnectRequest(tc.dst, 443), init, func(reply []byte) {
				if reply[1] != 0x00 {
					t.Fatalf("unexpected socks reply status: %d", reply[1])
				}
			})

			if len(order) < 2 || order[0] == "tcp" {
				t.Fatalf("expected CF attempt before tcp fallback, got order=%v", order)
			}
			if order[len(order)-1] != "tcp" {
				t.Fatalf("expected tcp fallback as final step, got order=%v", order)
			}
			if tcpHost != tc.wantTCPHost {
				t.Fatalf("unexpected tcp fallback host: got %q want %q", tcpHost, tc.wantTCPHost)
			}
		})
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
