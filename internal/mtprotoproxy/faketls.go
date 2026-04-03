package mtprotoproxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"net"
)

const (
	tlsRecordHandshake  = 0x16
	tlsRecordChangeCipher = 0x14
	tlsRecordAppData    = 0x17
	tlsVersion12        = 0x0303

	handshakeClientHello = 0x01
	handshakeServerHello = 0x02

	clientHelloRandomOffset = 6  // offset within the handshake message body
	clientHelloRandomLen    = 32
)

var (
	ErrNotTLS       = errors.New("faketls: not a TLS handshake")
	ErrBadHMAC      = errors.New("faketls: client HMAC verification failed")
	ErrTooShort     = errors.New("faketls: record too short")
)

// FakeTLSHandshake performs the fake-TLS handshake with the client.
// It reads the TLS Client Hello, verifies the HMAC using the proxy secret,
// sends back a Server Hello, and returns the underlying connection ready
// for obfuscated2 data (wrapped in TLS application data records).
func FakeTLSHandshake(conn net.Conn, secret []byte) error {
	// Read TLS record header (5 bytes): type(1) + version(2) + length(2)
	var recHdr [5]byte
	if _, err := io.ReadFull(conn, recHdr[:]); err != nil {
		return err
	}

	if recHdr[0] != tlsRecordHandshake {
		return ErrNotTLS
	}

	recLen := int(binary.BigEndian.Uint16(recHdr[3:5]))
	if recLen < 40 { // minimum for Client Hello with random
		return ErrTooShort
	}

	recBody := make([]byte, recLen)
	if _, err := io.ReadFull(conn, recBody); err != nil {
		return err
	}

	// recBody[0] = handshake type
	if recBody[0] != handshakeClientHello {
		return ErrNotTLS
	}

	// The full Client Hello record for HMAC verification.
	fullRecord := append(recHdr[:], recBody...)

	// Extract the client random (32 bytes at offset within handshake body).
	// Handshake header: type(1) + length(3) + version(2) = 6 bytes before random.
	randomStart := 1 + 3 + 2 // within recBody
	if len(recBody) < randomStart+clientHelloRandomLen {
		return ErrTooShort
	}
	clientRandom := make([]byte, clientHelloRandomLen)
	copy(clientRandom, recBody[randomStart:randomStart+clientHelloRandomLen])

	// Verify HMAC: zero the random field, compute HMAC-SHA256(secret, record), compare.
	recordCopy := make([]byte, len(fullRecord))
	copy(recordCopy, fullRecord)
	// Zero the random in the copy (offset: 5 bytes header + randomStart)
	for i := 0; i < clientHelloRandomLen; i++ {
		recordCopy[5+randomStart+i] = 0
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(recordCopy)
	expected := mac.Sum(nil)

	if !hmac.Equal(clientRandom, expected[:clientHelloRandomLen]) {
		return ErrBadHMAC
	}

	// Extract session ID for echo.
	sessionIDOffset := randomStart + clientHelloRandomLen
	if len(recBody) < sessionIDOffset+1 {
		return ErrTooShort
	}
	sessionIDLen := int(recBody[sessionIDOffset])
	if len(recBody) < sessionIDOffset+1+sessionIDLen {
		return ErrTooShort
	}
	sessionID := recBody[sessionIDOffset+1 : sessionIDOffset+1+sessionIDLen]

	// Compute server random: HMAC-SHA256(secret, clientRandom)
	serverMac := hmac.New(sha256.New, secret)
	serverMac.Write(clientRandom)
	serverRandom := serverMac.Sum(nil)[:32]

	// Build and send Server Hello + Change Cipher Spec + dummy encrypted extensions.
	response := buildServerResponse(serverRandom, sessionID)
	_, err := conn.Write(response)
	return err
}

// ReadTLSRecord reads one TLS application data record and returns the payload.
func ReadTLSRecord(r io.Reader) ([]byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	if hdr[0] != tlsRecordAppData {
		return nil, ErrNotTLS
	}
	length := int(binary.BigEndian.Uint16(hdr[3:5]))
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// WriteTLSRecord writes data as a TLS application data record.
func WriteTLSRecord(w io.Writer, data []byte) error {
	var hdr [5]byte
	hdr[0] = tlsRecordAppData
	binary.BigEndian.PutUint16(hdr[1:3], tlsVersion12)
	binary.BigEndian.PutUint16(hdr[3:5], uint16(len(data)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func buildServerResponse(serverRandom, sessionID []byte) []byte {
	// Server Hello handshake message.
	var sh []byte
	sh = append(sh, 0x03, 0x03)            // server version TLS 1.2
	sh = append(sh, serverRandom...)        // 32 bytes
	sh = append(sh, byte(len(sessionID)))   // session ID length
	sh = append(sh, sessionID...)           // session ID
	sh = append(sh, 0x13, 0x01)            // cipher suite TLS_AES_128_GCM_SHA256
	sh = append(sh, 0x00)                  // compression method: null

	// Extensions (minimal).
	extensions := []byte{
		0x00, 0x2b, 0x00, 0x02, 0x03, 0x04, // supported_versions: TLS 1.3
	}
	sh = append(sh, byte(len(extensions)>>8), byte(len(extensions)))
	sh = append(sh, extensions...)

	// Wrap in handshake message.
	var hsMsg []byte
	hsMsg = append(hsMsg, handshakeServerHello) // type
	hsMsg = append(hsMsg, byte(len(sh)>>16), byte(len(sh)>>8), byte(len(sh)))
	hsMsg = append(hsMsg, sh...)

	// Wrap in TLS record.
	var result []byte
	result = append(result, tlsRecordHandshake)
	result = append(result, 0x03, 0x03) // TLS 1.2
	result = append(result, byte(len(hsMsg)>>8), byte(len(hsMsg)))
	result = append(result, hsMsg...)

	// Change Cipher Spec record.
	result = append(result, tlsRecordChangeCipher, 0x03, 0x03, 0x00, 0x01, 0x01)

	// Dummy encrypted extensions (application data record with random-ish content).
	dummyPayload := make([]byte, 64)
	for i := range dummyPayload {
		dummyPayload[i] = byte(i * 7)
	}
	result = append(result, tlsRecordAppData, 0x03, 0x03)
	result = append(result, byte(len(dummyPayload)>>8), byte(len(dummyPayload)))
	result = append(result, dummyPayload...)

	return result
}
