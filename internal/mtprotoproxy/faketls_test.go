package mtprotoproxy

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestFakeTLSHandshakeSuccess(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientHello := buildFakeClientHello(t, secret)

	errCh := make(chan error, 1)
	go func() {
		errCh <- FakeTLSHandshake(serverConn, secret)
	}()

	// Client sends Client Hello.
	if _, err := clientConn.Write(clientHello); err != nil {
		t.Fatalf("write client hello: %v", err)
	}

	// Read server response.
	resp := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(resp)
	if err != nil {
		t.Fatalf("read server response: %v", err)
	}
	resp = resp[:n]

	if err := <-errCh; err != nil {
		t.Fatalf("FakeTLSHandshake failed: %v", err)
	}

	// Verify response starts with a TLS handshake record (Server Hello).
	if resp[0] != tlsRecordHandshake {
		t.Fatalf("expected handshake record, got 0x%02x", resp[0])
	}
}

func TestFakeTLSHandshakeBadHMAC(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	wrongSecret := make([]byte, 16)
	rand.Read(wrongSecret)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// Build with wrong secret.
	clientHello := buildFakeClientHello(t, wrongSecret)

	errCh := make(chan error, 1)
	go func() {
		errCh <- FakeTLSHandshake(serverConn, secret)
	}()

	clientConn.Write(clientHello)

	err := <-errCh
	if err != ErrBadHMAC {
		t.Fatalf("expected ErrBadHMAC, got %v", err)
	}
}

func TestFakeTLSHandshakeNotTLS(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- FakeTLSHandshake(serverConn, secret)
	}()

	// Send garbage with first byte != 0x16 (not a TLS record), then close.
	clientConn.Write([]byte{0x47}) // 'G'
	clientConn.Write([]byte{0x03, 0x01, 0x00, 0x05}) // fake header
	clientConn.Close()

	err := <-errCh
	if err != ErrNotTLS {
		t.Fatalf("expected ErrNotTLS, got %v", err)
	}
}

func TestTLSRecordReadWrite(t *testing.T) {
	payload := []byte("hello tls record")

	var buf bytes.Buffer
	if err := WriteTLSRecord(&buf, payload); err != nil {
		t.Fatalf("WriteTLSRecord: %v", err)
	}

	got, err := ReadTLSRecord(&buf)
	if err != nil {
		t.Fatalf("ReadTLSRecord: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q, want %q", got, payload)
	}
}

func TestReadTLSRecordBadType(t *testing.T) {
	// Write a handshake record type instead of app data.
	var buf bytes.Buffer
	buf.Write([]byte{tlsRecordHandshake, 0x03, 0x03, 0x00, 0x05})
	buf.Write([]byte("hello"))

	_, err := ReadTLSRecord(&buf)
	if err != ErrNotTLS {
		t.Fatalf("expected ErrNotTLS, got %v", err)
	}
}

// --- helpers ---

// buildFakeClientHello constructs a minimal TLS Client Hello with correct HMAC.
func buildFakeClientHello(t *testing.T, secret []byte) []byte {
	t.Helper()

	// Build a minimal Client Hello handshake message.
	var hsBody []byte

	// Client version.
	hsBody = append(hsBody, 0x03, 0x03) // TLS 1.2

	// Placeholder for random (32 bytes of zeros — will be filled with HMAC).
	randomOffset := len(hsBody)
	hsBody = append(hsBody, make([]byte, 32)...)

	// Session ID (32 bytes).
	sessionID := make([]byte, 32)
	rand.Read(sessionID)
	hsBody = append(hsBody, byte(len(sessionID)))
	hsBody = append(hsBody, sessionID...)

	// Cipher suites.
	hsBody = append(hsBody, 0x00, 0x02) // length
	hsBody = append(hsBody, 0x13, 0x01) // TLS_AES_128_GCM_SHA256

	// Compression methods.
	hsBody = append(hsBody, 0x01, 0x00)

	// Wrap in handshake message: type(1) + length(3)
	var hsMsg []byte
	hsMsg = append(hsMsg, handshakeClientHello)
	hsMsg = append(hsMsg, byte(len(hsBody)>>16), byte(len(hsBody)>>8), byte(len(hsBody)))
	hsMsg = append(hsMsg, hsBody...)

	// Wrap in TLS record: type(1) + version(2) + length(2)
	var record []byte
	record = append(record, tlsRecordHandshake)
	record = append(record, 0x03, 0x01) // TLS 1.0 in record layer (standard practice)
	record = append(record, byte(len(hsMsg)>>8), byte(len(hsMsg)))
	record = append(record, hsMsg...)

	// Now compute HMAC with zeroed random.
	mac := hmac.New(sha256.New, secret)
	mac.Write(record)
	computedHMAC := mac.Sum(nil)

	// Fill in the random field: 5 (record header) + 1 (handshake type) + 3 (length) + 2 (version) = 11
	randomAbsOffset := 5 + 1 + 3 + randomOffset
	copy(record[randomAbsOffset:randomAbsOffset+32], computedHMAC[:32])

	return record
}

func init() {
	// Verify helper compiles.
	_ = io.Reader(nil)
	_ = binary.BigEndian
}
