package mtprotoproxy

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/wsbridge"
)

// --- TLSCryptoConn tests ---

func TestTLSCryptoConnReadWrite(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	initPacket := buildClientInit(t, secret, 0xEFEFEFEF, 2, false)
	pi, err := DecryptInit(initPacket, secret)
	if err != nil {
		t.Fatalf("DecryptInit: %v", err)
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	tlsConn := NewTLSCryptoConn(serverConn, pi.Decryptor, pi.Encryptor)

	// Client sends encrypted data inside a TLS record.
	clientEnc, clientDec := clientStreams(t, initPacket, secret)

	plaintext := []byte("hello from client via tls record")
	encrypted := make([]byte, len(plaintext))
	clientEnc.XORKeyStream(encrypted, plaintext)

	go func() {
		WriteTLSRecord(clientConn, encrypted)
	}()

	buf := make([]byte, 256)
	n, err := tlsConn.Read(buf)
	if err != nil {
		t.Fatalf("TLSCryptoConn.Read: %v", err)
	}
	if !bytes.Equal(buf[:n], plaintext) {
		t.Fatalf("decrypted mismatch: got %q, want %q", buf[:n], plaintext)
	}

	// Server writes plaintext via TLSCryptoConn, client reads TLS record and decrypts.
	response := []byte("hello from server")
	go func() {
		tlsConn.Write(response)
	}()

	payload, err := ReadTLSRecord(clientConn)
	if err != nil {
		t.Fatalf("ReadTLSRecord: %v", err)
	}
	decrypted := make([]byte, len(payload))
	clientDec.XORKeyStream(decrypted, payload)
	if !bytes.Equal(decrypted, response) {
		t.Fatalf("response mismatch: got %q, want %q", decrypted, response)
	}
}

func TestTLSCryptoConnBuffering(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	initPacket := buildClientInit(t, secret, 0xEFEFEFEF, 2, false)
	pi, err := DecryptInit(initPacket, secret)
	if err != nil {
		t.Fatalf("DecryptInit: %v", err)
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	tlsConn := NewTLSCryptoConn(serverConn, pi.Decryptor, pi.Encryptor)

	clientEnc, _ := clientStreams(t, initPacket, secret)

	// Send a large message that won't fit in a small read buffer.
	plaintext := make([]byte, 100)
	for i := range plaintext {
		plaintext[i] = byte(i)
	}
	encrypted := make([]byte, len(plaintext))
	clientEnc.XORKeyStream(encrypted, plaintext)

	go func() {
		WriteTLSRecord(clientConn, encrypted)
	}()

	// Read in small chunks to test buffering.
	var got []byte
	buf := make([]byte, 10)
	for len(got) < len(plaintext) {
		n, err := tlsConn.Read(buf)
		if err != nil {
			t.Fatalf("TLSCryptoConn.Read: %v", err)
		}
		got = append(got, buf[:n]...)
	}

	if !bytes.Equal(got, plaintext) {
		t.Fatalf("buffered read mismatch")
	}
}

func TestTLSCryptoConnNetConn(t *testing.T) {
	// Verify TLSCryptoConn implements net.Conn.
	var _ net.Conn = (*TLSCryptoConn)(nil)
}

// --- Server integration tests ---

func TestServerHandleConnWSRoute(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	cfg := config.Default()
	cfg.Verbose = true
	cfg.DialTimeout = 2 * time.Second

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	srv := NewServer(cfg, logger, nil, secret)

	var wsMu sync.Mutex
	var wsConnected bool
	var wsTargetIP string
	var wsDC int

	// Mock WebSocket connection.
	mockWS := newMockWSClient()
	srv.connectWSFunc = func(ctx context.Context, targetIP string, dc int, isMedia bool) (*wsbridge.Client, error) {
		wsMu.Lock()
		wsConnected = true
		wsTargetIP = targetIP
		wsDC = dc
		wsMu.Unlock()
		return mockWS, nil
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go srv.handleConn(ctx, serverConn)

	// Phase 1: Send fake-TLS Client Hello.
	clientHello := buildFakeClientHello(t, secret)
	if _, err := clientConn.Write(clientHello); err != nil {
		t.Fatalf("write client hello: %v", err)
	}

	// Read server response (Server Hello + Change Cipher Spec + dummy).
	resp := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(resp)
	if err != nil {
		t.Fatalf("read server response: %v", err)
	}
	if n == 0 {
		t.Fatal("empty server response")
	}
	clientConn.SetReadDeadline(time.Time{})

	// Phase 2: Send init packet as TLS app data record.
	// DC=2 (WS-enabled)
	initPacket := buildClientInit(t, secret, 0xEFEFEFEF, 2, false)

	// We need to encrypt the init packet with the same AES-CTR key stream
	// that DecryptInit expects: key = SHA256(init[8:40] + secret), IV = init[40:56]
	// But the TLSCryptoConn decrypts using those keys, so we just send it raw inside a TLS record.
	if err := WriteTLSRecord(clientConn, initPacket); err != nil {
		t.Fatalf("write init TLS record: %v", err)
	}

	// Give server time to process and route.
	time.Sleep(200 * time.Millisecond)

	wsMu.Lock()
	if !wsConnected {
		t.Fatal("expected WS connection for DC2")
	}
	if wsTargetIP != cfg.DCIPs[2] {
		t.Fatalf("expected target IP %s, got %s", cfg.DCIPs[2], wsTargetIP)
	}
	if wsDC != 2 {
		t.Fatalf("expected DC 2, got %d", wsDC)
	}
	wsMu.Unlock()
}

func TestServerHandleConnTCPRoute(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	cfg := config.Default()
	cfg.Verbose = true

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	srv := NewServer(cfg, logger, nil, secret)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go srv.handleConn(ctx, serverConn)

	// Send fake-TLS handshake.
	clientHello := buildFakeClientHello(t, secret)
	clientConn.Write(clientHello)

	resp := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	clientConn.Read(resp)
	clientConn.SetReadDeadline(time.Time{})

	// Send init for DC=5 (not WS-enabled).
	initPacket := buildClientInit(t, secret, 0xEFEFEFEF, 5, false)
	WriteTLSRecord(clientConn, initPacket)

	// Give server time to process.
	time.Sleep(200 * time.Millisecond)

	// The TCP fallback will fail (no real upstream), but we can verify
	// from logs that it attempted TCP routing.
	logs := logBuf.String()
	if !containsStr(logs, "route=tcp") {
		t.Fatalf("expected TCP route in logs, got: %s", logs)
	}
}

func TestServerHandleConnBadHandshake(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	cfg := config.Default()
	cfg.Verbose = true

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	srv := NewServer(cfg, logger, nil, secret)

	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		srv.handleConn(ctx, serverConn)
		close(done)
	}()

	// Send garbage (not a TLS record).
	clientConn.Write([]byte{0x47, 0x03, 0x01, 0x00, 0x05})
	clientConn.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handleConn did not return after bad handshake")
	}

	conn, _, _, errs := srv.stats.snapshot()
	if conn != 1 {
		t.Fatalf("expected 1 connection, got %d", conn)
	}
	if errs != 1 {
		t.Fatalf("expected 1 error, got %d", errs)
	}
}

func TestServerRunAndAccept(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	cfg := config.Default()
	cfg.Host = "127.0.0.1"
	cfg.MTProtoHost = "127.0.0.1"
	cfg.MTProtoPort = 0 // will use a random port

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	srv := NewServer(cfg, logger, nil, secret)

	// Use a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	cfg.MTProtoPort = port
	srv.cfg = cfg

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	// Wait for server to start.
	time.Sleep(100 * time.Millisecond)

	// Connect to the server.
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestIsWSEnabledDC(t *testing.T) {
	tests := []struct {
		dc   int
		want bool
	}{
		{1, false},
		{2, true},
		{3, false},
		{4, true},
		{5, false},
	}
	for _, tc := range tests {
		if got := isWSEnabledDC(tc.dc); got != tc.want {
			t.Errorf("isWSEnabledDC(%d) = %v, want %v", tc.dc, got, tc.want)
		}
	}
}

func TestConnectWSBlacklisted(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	cfg := config.Default()
	logger := log.New(io.Discard, "", 0)

	srv := NewServer(cfg, logger, nil, secret)

	key := routeKey{dc: 2, isMedia: false}
	srv.stateMu.Lock()
	srv.wsBlacklist[key] = struct{}{}
	srv.stateMu.Unlock()

	_, err := srv.connectWS(context.Background(), "149.154.167.220", 2, false)
	if err == nil || err.Error() != "ws blacklisted" {
		t.Fatalf("expected blacklist error, got: %v", err)
	}
}

func TestConnectWSCooldown(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	cfg := config.Default()
	cfg.Verbose = true
	logger := log.New(io.Discard, "", 0)

	srv := NewServer(cfg, logger, nil, secret)

	// Make dial always fail.
	srv.wsDialFunc = func(ctx context.Context, cfg config.Config, targetIP, domain string) (*wsbridge.Client, error) {
		return nil, errors.New("dial failed")
	}

	_, err := srv.connectWS(context.Background(), "149.154.167.220", 2, false)
	if err == nil {
		t.Fatal("expected error")
	}

	// After failure, cooldown should be active.
	key := routeKey{dc: 2, isMedia: false}
	if !srv.isCooldownActive(key) {
		t.Fatal("expected cooldown to be active after failure")
	}
}

func TestClearFailureState(t *testing.T) {
	cfg := config.Default()
	logger := log.New(io.Discard, "", 0)

	srv := NewServer(cfg, logger, nil, nil)

	key := routeKey{dc: 2, isMedia: false}
	srv.stateMu.Lock()
	srv.wsBlacklist[key] = struct{}{}
	srv.wsFailUntil[key] = time.Now().Add(time.Hour)
	srv.stateMu.Unlock()

	srv.clearFailureState(key)

	if srv.isBlacklisted(key) {
		t.Fatal("expected blacklist cleared")
	}
	if srv.isCooldownActive(key) {
		t.Fatal("expected cooldown cleared")
	}
}

func TestStatsIncAndSnapshot(t *testing.T) {
	s := &stats{}
	s.inc(&s.connections)
	s.inc(&s.connections)
	s.inc(&s.wsRouted)
	s.inc(&s.errors)

	conn, ws, tcp, errs := s.snapshot()
	if conn != 2 {
		t.Fatalf("connections: got %d, want 2", conn)
	}
	if ws != 1 {
		t.Fatalf("wsRouted: got %d, want 1", ws)
	}
	if tcp != 0 {
		t.Fatalf("tcpRouted: got %d, want 0", tcp)
	}
	if errs != 1 {
		t.Fatalf("errors: got %d, want 1", errs)
	}
}

func TestBridgeTCP(t *testing.T) {
	a1, a2 := net.Pipe()
	b1, b2 := net.Pipe()
	defer a1.Close()
	defer a2.Close()
	defer b1.Close()
	defer b2.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go bridgeTCP(ctx, a2, b2)

	// Write from a-side, read from b-side.
	msg := []byte("test message")
	go func() {
		a1.Write(msg)
	}()

	buf := make([]byte, 256)
	b1.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := b1.Read(buf)
	if err != nil {
		t.Fatalf("b1.Read: %v", err)
	}
	if !bytes.Equal(buf[:n], msg) {
		t.Fatalf("bridge mismatch: got %q, want %q", buf[:n], msg)
	}
}

// --- helpers ---

// newMockWSClient creates a mock wsbridge.Client backed by a net.Pipe.
func newMockWSClient() *wsbridge.Client {
	c1, c2 := net.Pipe()
	// Drain reads on c2 to prevent blocking.
	go func() {
		io.Copy(io.Discard, c2)
		c2.Close()
	}()
	return wsbridge.NewClient(c1)
}

func containsStr(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

// suppress unused import warnings
func init() {
	_ = hmac.New
	_ = sha256.New
	_ = binary.BigEndian
	_ = aes.NewCipher
	_ = cipher.NewCTR
}
