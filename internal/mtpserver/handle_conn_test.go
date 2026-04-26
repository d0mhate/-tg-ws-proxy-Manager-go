package mtpserver

import (
	"bytes"
	"io"
	"net"
	"slices"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/mtproto"
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
	cfg.DCIPs[5] = "149.154.171.5"
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
	if session.tcpFallbackTarget != "149.154.171.5" {
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
	if session.tcpFallbackTarget != "149.154.167.220" {
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
