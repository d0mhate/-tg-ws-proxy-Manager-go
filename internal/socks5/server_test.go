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

func TestHandshakeRejectsUnsupportedCommand(t *testing.T) {
	_, err := runHandshakeClient(t, []byte{
		0x05, 0x01, 0x00,
		0x05, 0x02, 0x00, 0x01,
	})
	if err == nil {
		t.Fatal("expected handshake to reject unsupported command")
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

func TestHandleConnRejectsHTTPTransport(t *testing.T) {
	var called bool

	srv := NewServer(config.Default(), log.New(io.Discard, "", 0))
	srv.proxyTCPFunc = func(ctx context.Context, conn net.Conn, host string, port int) error {
		called = true
		return nil
	}
	srv.proxyTCPWithInitFunc = func(ctx context.Context, conn net.Conn, host string, port int, init []byte) error {
		called = true
		return nil
	}

	init := append([]byte("GET / HTTP/1.1"), bytes.Repeat([]byte{0}, 64-len("GET / HTTP/1.1"))...)
	runHandleConnFlow(t, srv, ipv4ConnectRequest("149.154.167.41", 443), init, func(reply []byte) {
		if reply[1] != 0x00 {
			t.Fatalf("unexpected socks reply status: %d", reply[1])
		}
	})

	if called {
		t.Fatal("did not expect proxying paths to be called for http transport")
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

func runHandshakeClient(t *testing.T, request []byte) (requestOut request, err error) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientErrCh := make(chan error, 1)
	go func() {
		defer close(clientErrCh)

		if _, writeErr := clientConn.Write(request[:3]); writeErr != nil {
			clientErrCh <- writeErr
			return
		}

		reply := make([]byte, 2)
		if _, readErr := io.ReadFull(clientConn, reply); readErr != nil {
			clientErrCh <- readErr
			return
		}
		if !bytes.Equal(reply, []byte{0x05, 0x00}) {
			clientErrCh <- io.ErrUnexpectedEOF
			return
		}

		if _, writeErr := clientConn.Write(request[3:]); writeErr != nil {
			clientErrCh <- writeErr
			return
		}
	}()

	requestOut, err = handshake(serverConn)
	if clientErr := <-clientErrCh; clientErr != nil {
		t.Fatalf("client side of handshake failed: %v", clientErr)
	}
	return requestOut, err
}

func runHandleConnFlow(t *testing.T, srv *Server, connectReq []byte, init []byte, assertReply func([]byte)) {
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

	if _, err := clientConn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("failed to send auth greeting: %v", err)
	}

	authReply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, authReply); err != nil {
		t.Fatalf("failed to read auth reply: %v", err)
	}
	if !bytes.Equal(authReply, []byte{0x05, 0x00}) {
		t.Fatalf("unexpected auth reply: %v", authReply)
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
