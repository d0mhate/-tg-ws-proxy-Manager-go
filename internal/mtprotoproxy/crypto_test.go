package mtprotoproxy

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestDecryptInitRoundTrip(t *testing.T) {
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}

	// Build an init packet as a client would.
	initPacket := buildClientInit(t, secret, 0xEFEFEFEF, 2, false)

	pi, err := DecryptInit(initPacket, secret)
	if err != nil {
		t.Fatalf("DecryptInit failed: %v", err)
	}
	if pi.DC != 2 {
		t.Fatalf("expected DC 2, got %d", pi.DC)
	}
	if pi.IsMedia {
		t.Fatal("expected non-media")
	}
	if pi.Proto != 0xEFEFEFEF {
		t.Fatalf("expected proto 0xEFEFEFEF, got 0x%08X", pi.Proto)
	}
}

func TestDecryptInitMedia(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	initPacket := buildClientInit(t, secret, 0xEEEEEEEE, 4, true)

	pi, err := DecryptInit(initPacket, secret)
	if err != nil {
		t.Fatalf("DecryptInit failed: %v", err)
	}
	if pi.DC != 4 {
		t.Fatalf("expected DC 4, got %d", pi.DC)
	}
	if !pi.IsMedia {
		t.Fatal("expected media")
	}
}

func TestDecryptInitBadProto(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	initPacket := buildClientInit(t, secret, 0x12345678, 2, false)

	_, err := DecryptInit(initPacket, secret)
	if err != ErrBadProtocol {
		t.Fatalf("expected ErrBadProtocol, got %v", err)
	}
}

func TestDecryptInitWrongSecret(t *testing.T) {
	secret1 := make([]byte, 16)
	secret2 := make([]byte, 16)
	rand.Read(secret1)
	rand.Read(secret2)

	initPacket := buildClientInit(t, secret1, 0xEFEFEFEF, 2, false)

	// Decrypting with wrong secret should yield bad protocol.
	_, err := DecryptInit(initPacket, secret2)
	if err != ErrBadProtocol {
		t.Fatalf("expected ErrBadProtocol with wrong secret, got %v", err)
	}
}

func TestDecryptInitTooShort(t *testing.T) {
	_, err := DecryptInit(make([]byte, 32), make([]byte, 16))
	if err != ErrInitTooShort {
		t.Fatalf("expected ErrInitTooShort, got %v", err)
	}
}

func TestCryptoConnReadWrite(t *testing.T) {
	secret := make([]byte, 16)
	rand.Read(secret)

	initPacket := buildClientInit(t, secret, 0xEFEFEFEF, 2, false)

	pi, err := DecryptInit(initPacket, secret)
	if err != nil {
		t.Fatalf("DecryptInit failed: %v", err)
	}

	// Simulate: client sends encrypted data, proxy reads decrypted data.
	// Then proxy sends plaintext, client reads encrypted.
	clientEnc, clientDec := clientStreams(t, initPacket, secret)

	clientConn, proxyRaw := net.Pipe()
	defer clientConn.Close()
	defer proxyRaw.Close()

	proxyConn := NewCryptoConn(proxyRaw, pi.Decryptor, pi.Encryptor)

	// Client writes encrypted data.
	plaintext := []byte("hello from client")
	encrypted := make([]byte, len(plaintext))
	clientEnc.XORKeyStream(encrypted, plaintext)

	go func() {
		clientConn.Write(encrypted)
	}()

	// Proxy reads decrypted data via CryptoConn.
	buf := make([]byte, 256)
	n, err := proxyConn.Read(buf)
	if err != nil {
		t.Fatalf("proxyConn.Read failed: %v", err)
	}
	if !bytes.Equal(buf[:n], plaintext) {
		t.Fatalf("decrypted mismatch: got %q, want %q", buf[:n], plaintext)
	}

	// Proxy writes plaintext via CryptoConn, client decrypts.
	response := []byte("hello from proxy")
	go func() {
		proxyConn.Write(response)
	}()

	buf2 := make([]byte, 256)
	n2, err := clientConn.Read(buf2)
	if err != nil {
		t.Fatalf("clientConn.Read failed: %v", err)
	}
	decrypted := make([]byte, n2)
	clientDec.XORKeyStream(decrypted, buf2[:n2])
	if !bytes.Equal(decrypted, response) {
		t.Fatalf("response mismatch: got %q, want %q", decrypted, response)
	}
}

func TestReverseBytes(t *testing.T) {
	input := []byte{1, 2, 3, 4, 5}
	got := reverseBytes(input)
	want := []byte{5, 4, 3, 2, 1}
	if !bytes.Equal(got, want) {
		t.Fatalf("reverseBytes: got %v, want %v", got, want)
	}
	// Verify input not modified.
	if !bytes.Equal(input, []byte{1, 2, 3, 4, 5}) {
		t.Fatal("reverseBytes modified input")
	}
}

func TestValidProto(t *testing.T) {
	if !validProto(0xEFEFEFEF) {
		t.Fatal("expected abridged to be valid")
	}
	if !validProto(0xEEEEEEEE) {
		t.Fatal("expected intermediate to be valid")
	}
	if !validProto(0xDDDDDDDD) {
		t.Fatal("expected padded intermediate to be valid")
	}
	if validProto(0x12345678) {
		t.Fatal("expected 0x12345678 to be invalid")
	}
}

// --- helpers ---

// buildClientInit creates an obfuscated2 init packet as a real Telegram client would.
func buildClientInit(t *testing.T, secret []byte, proto uint32, dc int, isMedia bool) []byte {
	t.Helper()

	init := make([]byte, 64)
	// Fill with random data.
	if _, err := io.ReadFull(rand.Reader, init); err != nil {
		t.Fatal(err)
	}

	// Ensure the first byte doesn't match forbidden values (0xef, etc.)
	// and first 4 bytes aren't common protocol headers.
	init[0] = 0x01

	// Build the plaintext payload at bytes 56-63.
	var plain [8]byte
	binary.LittleEndian.PutUint32(plain[:4], proto)
	dcVal := int16(dc)
	if isMedia {
		dcVal = -dcVal
	}
	binary.LittleEndian.PutUint16(plain[4:6], uint16(dcVal))

	// Encrypt with the proxy-mode key derivation.
	decKey := sha256sum(init[8:40], secret)
	decIV := make([]byte, 16)
	copy(decIV, init[40:56])

	block, err := aes.NewCipher(decKey)
	if err != nil {
		t.Fatal(err)
	}
	stream := cipher.NewCTR(block, decIV)

	// Generate keystream for first 64 bytes.
	keystream := make([]byte, 64)
	stream.XORKeyStream(keystream, keystream)

	// XOR plaintext into the init packet at bytes 56-63.
	for i := 0; i < 8; i++ {
		init[56+i] = plain[i] ^ keystream[56+i]
	}

	return init
}

// clientStreams creates the AES-CTR streams as the client would use them
// (encrypt = client->proxy, decrypt = proxy->client).
func clientStreams(t *testing.T, initPacket, secret []byte) (encrypt, decrypt cipher.Stream) {
	t.Helper()

	// Client encrypts with: SHA256(init[8:40] + secret), IV = init[40:56]
	encKey := sha256sum(initPacket[8:40], secret)
	encIV := make([]byte, 16)
	copy(encIV, initPacket[40:56])

	block, err := aes.NewCipher(encKey)
	if err != nil {
		t.Fatal(err)
	}
	encStream := cipher.NewCTR(block, encIV)
	// Skip the first 64 bytes of keystream (used for init packet).
	skip := make([]byte, 64)
	encStream.XORKeyStream(skip, skip)

	// Client decrypts with: SHA256(reverse(init[8:40]) + secret), IV = reverse(init[40:56])
	decKey := sha256sum(reverseBytes(initPacket[8:40]), secret)
	decIV := make([]byte, 16)
	copy(decIV, reverseBytes(initPacket[40:56]))

	block2, err := aes.NewCipher(decKey)
	if err != nil {
		t.Fatal(err)
	}
	decStream := cipher.NewCTR(block2, decIV)

	return encStream, decStream
}

// dummy time helper to satisfy interface checks
func init() {
	var _ net.Conn = (*CryptoConn)(nil)
}

// mockConn for simple tests
type mockConn struct {
	readBuf  *bytes.Reader
	writeBuf bytes.Buffer
}

func (c *mockConn) Read(p []byte) (int, error)         { return c.readBuf.Read(p) }
func (c *mockConn) Write(p []byte) (int, error)        { return c.writeBuf.Write(p) }
func (c *mockConn) Close() error                       { return nil }
func (c *mockConn) LocalAddr() net.Addr                { return dummyAddr("local") }
func (c *mockConn) RemoteAddr() net.Addr               { return dummyAddr("remote") }
func (c *mockConn) SetDeadline(time.Time) error        { return nil }
func (c *mockConn) SetReadDeadline(time.Time) error    { return nil }
func (c *mockConn) SetWriteDeadline(time.Time) error   { return nil }

type dummyAddr string

func (a dummyAddr) Network() string { return "tcp" }
func (a dummyAddr) String() string  { return string(a) }
