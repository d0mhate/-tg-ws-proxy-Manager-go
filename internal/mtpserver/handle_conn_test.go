package mtpserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/mtproto"
	"tg-ws-proxy/internal/wsbridge"
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

func TestHasCloudflareRoutesRequiresFlagAndDomains(t *testing.T) {
	cfg := config.Default()
	srv := newTestServer(t, cfg)
	if srv.hasCloudflareRoutes() {
		t.Fatal("expected cloudflare routes to be disabled by default")
	}

	cfg = config.Default()
	cfg.UseCFProxy = true
	cfg.CFDomains = []string{"cf.example.com"}
	srv = newTestServer(t, cfg)
	if !srv.hasCloudflareRoutes() {
		t.Fatal("expected cloudflare routes when flag and domains are set")
	}
}

func TestPrepareClientSessionBuildsSessionFromHandshake(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs[5] = testIPv4DC5Alt
	srv := newTestServer(t, cfg)

	handshake, _, _, err := mtproto.GenerateClientHandshake(srv.secretKey(), 5, mtproto.ProtoIntermediate)
	if err != nil {
		t.Fatalf("GenerateClientHandshake: %v", err)
	}

	conn := newScriptedConn(handshake[:])
	session, ok := srv.prepareClientSession(conn)
	if !ok {
		t.Fatal("expected session to be prepared")
	}
	if session.dataConn != conn {
		t.Fatal("expected plain connection to be used as data conn")
	}
	if session.info.DC != 5 || session.dc != 5 {
		t.Fatalf("unexpected dc values: info=%d session=%d", session.info.DC, session.dc)
	}
	if session.info.IsMedia {
		t.Fatal("expected non-media session")
	}
	if session.info.Proto != mtproto.ProtoIntermediate {
		t.Fatalf("unexpected proto: %#x", session.info.Proto)
	}
	if len(session.directRoutes) != 1 || session.directRoutes[0].targetDC != 5 {
		t.Fatalf("unexpected direct routes: %+v", session.directRoutes)
	}
	if session.tcpFallbackTarget != testIPv4DC5Alt {
		t.Fatalf("unexpected tcp fallback target: %q", session.tcpFallbackTarget)
	}
	if session.clientDec == nil || session.clientEnc == nil || session.relayEnc == nil || session.relayDec == nil {
		t.Fatal("expected all cipher streams to be initialized")
	}
	if len(conn.deadlines) < 2 {
		t.Fatalf("expected init deadline and release, got %d deadline calls", len(conn.deadlines))
	}
	if conn.deadlines[len(conn.deadlines)-1] != (time.Time{}) {
		t.Fatal("expected connection deadline to be cleared after handshake")
	}
}

func TestPrepareClientSessionDefaultsUnknownDCTo2(t *testing.T) {
	srv := newTestServer(t, config.Default())

	handshake, _, _, err := mtproto.GenerateClientHandshake(srv.secretKey(), 0, mtproto.ProtoAbridged)
	if err != nil {
		t.Fatalf("GenerateClientHandshake: %v", err)
	}

	session, ok := srv.prepareClientSession(newScriptedConn(handshake[:]))
	if !ok {
		t.Fatal("expected session to be prepared")
	}
	if session.info.DC != 0 {
		t.Fatalf("expected parsed init dc=0, got %d", session.info.DC)
	}
	if session.dc != 2 {
		t.Fatalf("expected session dc fallback to 2, got %d", session.dc)
	}
	if session.tcpFallbackTarget != testIPv4DC2 {
		t.Fatalf("unexpected tcp fallback target: %q", session.tcpFallbackTarget)
	}
}

func TestReadClientHandshakeRejectsShortRead(t *testing.T) {
	srv := newTestServer(t, config.Default())

	_, ok := srv.readClientHandshake(newScriptedConn([]byte("short")))
	if ok {
		t.Fatal("expected short handshake read to fail")
	}
}

type scriptedConn struct {
	reader    *bytes.Reader
	deadlines []time.Time
}

func newScriptedConn(data []byte) *scriptedConn {
	return &scriptedConn{reader: bytes.NewReader(data)}
}

func (c *scriptedConn) Read(p []byte) (int, error)  { return c.reader.Read(p) }
func (c *scriptedConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *scriptedConn) Close() error                { return nil }
func (c *scriptedConn) LocalAddr() net.Addr         { return dummyAddr("local") }
func (c *scriptedConn) RemoteAddr() net.Addr        { return dummyAddr("remote") }
func (c *scriptedConn) SetReadDeadline(time.Time) error {
	return nil
}
func (c *scriptedConn) SetWriteDeadline(time.Time) error {
	return nil
}

func (c *scriptedConn) SetDeadline(t time.Time) error {
	c.deadlines = append(c.deadlines, t)
	return nil
}

var _ net.Conn = (*scriptedConn)(nil)
var _ io.Reader = (*scriptedConn)(nil)

// --- dialCloudflareWS ---

func TestDialCloudflareWSSucceedsOnFirstAvailableDomain(t *testing.T) {
	cfg := config.Default()
	cfg.UseCFProxy = true
	cfg.CFDomains = []string{"fail.example.com", "ok.example.com"}
	srv := newTestServer(t, cfg)

	attempts := 0
	srv.wsDialFunc = func(ctx context.Context, c config.Config, targetIP, domain string) (*wsbridge.Client, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("domain unavailable")
		}
		clientConn, peerConn := net.Pipe()
		go func() { _ = peerConn.Close() }()
		return wsbridge.NewClient(clientConn), nil
	}

	ws, err := srv.dialCloudflareWS(context.Background(), 2)
	if err != nil {
		t.Fatalf("expected successful CF dial on second domain, got %v", err)
	}
	_ = ws.Close()
	if attempts != 2 {
		t.Fatalf("expected 2 dial attempts, got %d", attempts)
	}
}

func TestDialCloudflareWSReturnsErrorWhenAllDomainsFail(t *testing.T) {
	cfg := config.Default()
	cfg.UseCFProxy = true
	cfg.CFDomains = []string{"a.example.com", "b.example.com"}
	srv := newTestServer(t, cfg)

	srv.wsDialFunc = func(ctx context.Context, c config.Config, targetIP, domain string) (*wsbridge.Client, error) {
		return nil, errors.New("dial failed")
	}

	_, err := srv.dialCloudflareWS(context.Background(), 2)
	if err == nil {
		t.Fatal("expected error when all CF domains fail")
	}
	if !strings.Contains(err.Error(), "kws2.a.example.com") || !strings.Contains(err.Error(), "kws2.b.example.com") {
		t.Fatalf("expected error to list attempted CF hosts, got %v", err)
	}
	if !strings.Contains(err.Error(), "dial failed") {
		t.Fatalf("expected error to include dial failure details, got %v", err)
	}
}

// --- connectWebsocketRelay ---

func TestConnectWebsocketRelayDirectSucceeds(t *testing.T) {
	cfg := config.Default()
	srv := newTestServer(t, cfg)

	srv.wsDialFunc = func(ctx context.Context, c config.Config, targetIP, domain string) (*wsbridge.Client, error) {
		clientConn, peerConn := net.Pipe()
		go func() { _ = peerConn.Close() }()
		return wsbridge.NewClient(clientConn), nil
	}

	handshake, _, _, err := mtproto.GenerateClientHandshake(srv.secretKey(), 2, mtproto.ProtoIntermediate)
	if err != nil {
		t.Fatalf("GenerateClientHandshake: %v", err)
	}
	session, ok := srv.prepareClientSession(newScriptedConn(handshake[:]))
	if !ok {
		t.Fatal("expected session to be prepared")
	}

	ws, _, wsErr := srv.connectWebsocketRelay(context.Background(), session)
	if wsErr != nil {
		t.Fatalf("expected successful websocket relay, got %v", wsErr)
	}
	if ws == nil {
		t.Fatal("expected non-nil websocket client")
	}
	_ = ws.Close()
}

func TestConnectWebsocketRelayFallsBackToCloudflare(t *testing.T) {
	cfg := config.Default()
	cfg.UseCFProxy = true
	cfg.CFDomains = []string{"cf.example.com"}
	srv := newTestServer(t, cfg)

	callOrder := make([]string, 0, 2)
	srv.wsDialFunc = func(ctx context.Context, c config.Config, targetIP, domain string) (*wsbridge.Client, error) {
		if domain == targetIP {
			callOrder = append(callOrder, "cf")
			clientConn, peerConn := net.Pipe()
			go func() { _ = peerConn.Close() }()
			return wsbridge.NewClient(clientConn), nil
		}
		callOrder = append(callOrder, "direct")
		return nil, errors.New("direct WS unavailable")
	}

	handshake, _, _, err := mtproto.GenerateClientHandshake(srv.secretKey(), 2, mtproto.ProtoIntermediate)
	if err != nil {
		t.Fatalf("GenerateClientHandshake: %v", err)
	}
	session, ok := srv.prepareClientSession(newScriptedConn(handshake[:]))
	if !ok {
		t.Fatal("expected session to be prepared")
	}

	ws, _, wsErr := srv.connectWebsocketRelay(context.Background(), session)
	if wsErr != nil {
		t.Fatalf("expected CF fallback to succeed, got %v", wsErr)
	}
	if ws == nil {
		t.Fatal("expected non-nil websocket client from CF")
	}
	_ = ws.Close()

	if len(callOrder) < 2 {
		t.Fatalf("expected at least 2 dial attempts, got %v", callOrder)
	}
	for i, call := range callOrder[:len(callOrder)-1] {
		if call != "direct" {
			t.Fatalf("expected all pre-CF calls to be direct, got %q at index %d in %v", call, i, callOrder)
		}
	}
	if last := callOrder[len(callOrder)-1]; last != "cf" {
		t.Fatalf("expected last dial attempt to be cf, got %q in %v", last, callOrder)
	}
}

func TestConnectWebsocketRelayReturnsErrorWhenNoPathsConfigured(t *testing.T) {
	cfg := config.Default()
	// remove all DC IPs so there are no direct routes
	cfg.DCIPs = map[int]string{}
	srv := newTestServer(t, cfg)

	session := &clientSession{dc: 99}

	_, _, err := srv.connectWebsocketRelay(context.Background(), session)
	if err == nil {
		t.Fatal("expected error when no websocket paths are configured")
	}
}

// --- connectRelay ---

func TestConnectRelayReturnsTrueOnWebsocketSuccess(t *testing.T) {
	cfg := config.Default()
	srv := newTestServer(t, cfg)

	srv.wsDialFunc = func(ctx context.Context, c config.Config, targetIP, domain string) (*wsbridge.Client, error) {
		clientConn, peerConn := net.Pipe()
		go func() { _ = peerConn.Close() }()
		return wsbridge.NewClient(clientConn), nil
	}

	handshake, _, _, err := mtproto.GenerateClientHandshake(srv.secretKey(), 2, mtproto.ProtoIntermediate)
	if err != nil {
		t.Fatalf("GenerateClientHandshake: %v", err)
	}
	session, ok := srv.prepareClientSession(newScriptedConn(handshake[:]))
	if !ok {
		t.Fatal("expected session to be prepared")
	}

	ws, _, connected := srv.connectRelay(context.Background(), session)
	if !connected {
		t.Fatal("expected connectRelay to return true on WS success")
	}
	if ws == nil {
		t.Fatal("expected non-nil websocket client")
	}
	_ = ws.Close()
}

func TestConnectRelayReturnsFalseAfterFallbackHandled(t *testing.T) {
	cfg := config.Default()
	cfg.DCIPs = map[int]string{}
	srv := newTestServer(t, cfg)

	session := &clientSession{
		dc:                99,
		dataConn:          newBlockingConn(),
		tcpFallbackTarget: "",
	}

	ws, _, connected := srv.connectRelay(context.Background(), session)
	if connected {
		t.Fatal("expected connectRelay to return false when WS fails and fallback handles it")
	}
	if ws != nil {
		t.Fatal("expected nil websocket client when falling back")
	}
}

// --- bridgeFallbackRelay ---

func TestBridgeFallbackRelayReturnsTrueWithNoFallbackConfigured(t *testing.T) {
	cfg := config.Default()
	cfg.UpstreamProxies = nil
	srv := newTestServer(t, cfg)

	session := &clientSession{
		dc:                2,
		dataConn:          newBlockingConn(),
		tcpFallbackTarget: "",
	}

	handled := srv.bridgeFallbackRelay(context.Background(), session, errors.New("ws failed"))
	if !handled {
		t.Fatal("expected bridgeFallbackRelay to return true even with no fallback configured")
	}
}

func TestBridgeFallbackRelayReturnsTrueWhenTCPFallbackDialFails(t *testing.T) {
	cfg := config.Default()
	cfg.UpstreamProxies = nil
	srv := newTestServer(t, cfg)

	session := &clientSession{
		dc:                2,
		dataConn:          newBlockingConn(),
		tcpFallbackTarget: "127.0.0.1", // port 443 will be refused in test
		clientDec:         nopStream{},
		clientEnc:         nopStream{},
		relayEnc:          nopStream{},
		relayDec:          nopStream{},
	}

	handled := srv.bridgeFallbackRelay(context.Background(), session, errors.New("ws failed"))
	if !handled {
		t.Fatal("expected bridgeFallbackRelay to return true even when TCP fallback dial fails")
	}
}
