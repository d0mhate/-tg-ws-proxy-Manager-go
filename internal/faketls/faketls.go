// faketls support for 0xee mtproto secrets.
//
// flow:
// 1. client sends a tls-like ClientHello with hmac in random and sni from secret
// 2. server replies with fake ServerHello + CCS + AppData
// 3. after that, traffic is wrapped in tls AppData records and still uses mtproto ctr inside
package faketls

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	RecordHandshake        = 0x16
	RecordChangeCipherSpec = 0x14
	RecordApplicationData  = 0x17
	RecordAlert            = 0x15

	// max tls record payload size.
	MaxRecordPayload = 16384

	maxHandshakeRecords = 20

	// offset of the random field in the full tls record:
	// record header(5) + handshake header(4) + ClientHello version(2) = 11.
	digestPos = 11
	digestLen = 32
)

var version12 = [2]byte{0x03, 0x03}
var recordVersion = [2]byte{0x03, 0x01} // TLS 1.0 in record layer for compat

// build a tls 1.2-style ClientHello with zeroed random.
// call SignClientHello before sending it.
func BuildClientHello(hostname string) []byte {
	var exts []byte

	// server_name (SNI)
	host := []byte(hostname)
	hostLen := uint16(len(host))
	sniEntryLen := 1 + 2 + hostLen
	sniListLen := sniEntryLen
	sniDataLen := 2 + sniListLen
	exts = appendU16(exts, 0x0000) // ext type: server_name
	exts = appendU16(exts, sniDataLen)
	exts = appendU16(exts, sniListLen)
	exts = append(exts, 0x00) // name_type: host_name
	exts = appendU16(exts, hostLen)
	exts = append(exts, host...)

	// extended_master_secret (empty)
	exts = append(exts, 0x00, 0x17, 0x00, 0x00)

	// renegotiation_info (empty)
	exts = append(exts, 0xff, 0x01, 0x00, 0x01, 0x00)

	// supported_groups: x25519, secp256r1, secp384r1, secp521r1
	exts = append(exts,
		0x00, 0x0a, 0x00, 0x0a, 0x00, 0x08,
		0x00, 0x1d, 0x00, 0x17, 0x00, 0x18, 0x00, 0x19,
	)

	// ec_point_formats: uncompressed
	exts = append(exts, 0x00, 0x0b, 0x00, 0x02, 0x01, 0x00)

	// session_ticket (empty)
	exts = append(exts, 0x00, 0x23, 0x00, 0x00)

	// signature_algorithms
	exts = append(exts,
		0x00, 0x0d, 0x00, 0x14, 0x00, 0x12,
		0x04, 0x03, 0x08, 0x04, 0x04, 0x01,
		0x05, 0x03, 0x08, 0x05, 0x05, 0x01,
		0x08, 0x06, 0x06, 0x01, 0x02, 0x01,
	)

	cipherSuites := []byte{
		0x13, 0x01, // TLS_AES_128_GCM_SHA256
		0x13, 0x02, // TLS_AES_256_GCM_SHA384
		0x13, 0x03, // TLS_CHACHA20_POLY1305_SHA256
		0xc0, 0x2b, // ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
		0xc0, 0x2f, // ECDHE_RSA_WITH_AES_128_GCM_SHA256
		0xc0, 0x2c, // ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
		0xc0, 0x30, // ECDHE_RSA_WITH_AES_256_GCM_SHA384
		0xcc, 0xa9, // ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
		0xcc, 0xa8, // ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
		0xc0, 0x13, // ECDHE_RSA_WITH_AES_128_CBC_SHA
		0xc0, 0x14, // ECDHE_RSA_WITH_AES_256_CBC_SHA
		0x00, 0x9c, // RSA_WITH_AES_128_GCM_SHA256
		0x00, 0x9d, // RSA_WITH_AES_256_GCM_SHA384
		0x00, 0x2f, // RSA_WITH_AES_128_CBC_SHA
		0x00, 0x35, // RSA_WITH_AES_256_CBC_SHA
		0x00, 0x0a, // RSA_WITH_3DES_EDE_CBC_SHA
	}

	// 32 random-ish bytes for session id. upstream ignores it for auth.
	sessionID := make([]byte, 32)
	ts := uint64(time.Now().UnixNano())
	for i := range sessionID {
		ts = ts*6364136223846793005 + 1442695040888963407
		sessionID[i] = byte(ts >> 56)
	}

	// ClientHello body
	var hello []byte
	hello = append(hello, version12[:]...)
	hello = append(hello, make([]byte, 32)...) // random: zeroed (filled by Sign)
	hello = append(hello, 0x20)                // session_id length = 32
	hello = append(hello, sessionID...)
	hello = appendU16(hello, uint16(len(cipherSuites)))
	hello = append(hello, cipherSuites...)
	hello = append(hello, 0x01, 0x00) // 1 compression method: null

	hello = appendU16(hello, uint16(len(exts)))
	hello = append(hello, exts...)

	// Handshake message
	hLen := len(hello)
	var hs []byte
	hs = append(hs, 0x01) // ClientHello
	hs = append(hs, byte(hLen>>16), byte(hLen>>8), byte(hLen))
	hs = append(hs, hello...)

	// TLS record
	var rec []byte
	rec = append(rec, RecordHandshake)
	rec = append(rec, recordVersion[:]...)
	rec = appendU16(rec, uint16(len(hs)))
	rec = append(rec, hs...)

	return rec
}

// fill ClientHello random with the faketls hmac scheme:
//
//	random[0:28]  = HMAC-SHA256(secret, record_with_random_zeroed)[0:28]
//	random[28:32] = hmac[28:32] XOR timestamp_le
func SignClientHello(record, secret []byte) {
	// Zero the random field (should already be zero from BuildClientHello).
	for i := digestPos; i < digestPos+digestLen; i++ {
		record[i] = 0
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(record)
	digest := mac.Sum(nil)

	copy(record[digestPos:digestPos+28], digest[:28])

	ts := uint32(time.Now().Unix())
	var tsBytes [4]byte
	binary.LittleEndian.PutUint32(tsBytes[:], ts)
	for i := 0; i < 4; i++ {
		record[digestPos+28+i] = digest[28+i] ^ tsBytes[i]
	}
}

// drain tls records until faketls handshake is done.
// success means we saw the first AppData record.
func DrainServerHello(r io.Reader) bool {
	hdr := make([]byte, 5)

	for i := 0; i < maxHandshakeRecords; i++ {
		if _, err := io.ReadFull(r, hdr); err != nil {
			return false
		}

		recType := hdr[0]
		ver := [2]byte{hdr[1], hdr[2]}
		payLen := int(binary.BigEndian.Uint16(hdr[3:5]))

		if ver != version12 {
			return false
		}
		if payLen > MaxRecordPayload+256 {
			return false
		}

		payload := make([]byte, payLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return false
		}

		switch recType {
		case RecordHandshake, RecordChangeCipherSpec:
			// discard and keep reading
		case RecordApplicationData:
			return true // fake handshake complete
		default:
			return false
		}
	}
	return false
}

// net.Conn wrapper for tls AppData framing.
type Conn struct {
	net.Conn
	readBuf []byte
}

// wrap inner conn with tls AppData framing.
func NewConn(inner net.Conn) *Conn {
	return &Conn{Conn: inner}
}

// unwrap tls AppData records and return payload.
// ccs is skipped, anything else ends the stream.
func (c *Conn) Read(b []byte) (int, error) {
	// return buffered leftover from a previous large record first.
	if len(c.readBuf) > 0 {
		n := copy(b, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	hdr := make([]byte, 5)
	for {
		if _, err := io.ReadFull(c.Conn, hdr); err != nil {
			return 0, err
		}

		recType := hdr[0]
		payLen := int(binary.BigEndian.Uint16(hdr[3:5]))

		payload := make([]byte, payLen)
		if _, err := io.ReadFull(c.Conn, payload); err != nil {
			return 0, err
		}

		switch recType {
		case RecordApplicationData:
			n := copy(b, payload)
			if n < len(payload) {
				// keep the part that did not fit.
				c.readBuf = make([]byte, len(payload)-n)
				copy(c.readBuf, payload[n:])
			}
			return n, nil
		case RecordChangeCipherSpec:
			// stray ccs, skip it.
			continue
		default:
			return 0, fmt.Errorf("faketls: unexpected record type 0x%02x", recType)
		}
	}
}

// wrap data into tls AppData records.
func (c *Conn) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		chunk := b
		if len(chunk) > MaxRecordPayload {
			chunk = b[:MaxRecordPayload]
		}

		hdr := [5]byte{
			RecordApplicationData,
			version12[0], version12[1],
			byte(len(chunk) >> 8), byte(len(chunk)),
		}

		buf := make([]byte, 5+len(chunk))
		copy(buf[:5], hdr[:])
		copy(buf[5:], chunk)
		if _, err := c.Conn.Write(buf); err != nil {
			return total, err
		}

		total += len(chunk)
		b = b[len(chunk):]
	}
	return total, nil
}

func appendU16(b []byte, v uint16) []byte {
	return append(b, byte(v>>8), byte(v))
}

// read a ClientHello, validate its hmac, and return client random on success.
// random[28:32] contains a timestamp and must be within 300 seconds.
func AcceptClientHello(r io.Reader, secret []byte) []byte {
	hdr := make([]byte, 5)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil
	}
	if hdr[0] != RecordHandshake {
		return nil
	}
	payLen := int(binary.BigEndian.Uint16(hdr[3:5]))
	if payLen > MaxRecordPayload+256 {
		return nil
	}
	payload := make([]byte, payLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil
	}

	// rebuild the full record so hmac covers the same bytes as SignClientHello.
	record := make([]byte, 5+payLen)
	copy(record, hdr)
	copy(record[5:], payload)

	if len(record) < digestPos+digestLen {
		return nil
	}

	// save random, then zero it before recomputing hmac.
	var saved [digestLen]byte
	copy(saved[:], record[digestPos:digestPos+digestLen])
	for i := digestPos; i < digestPos+digestLen; i++ {
		record[i] = 0
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(record)
	expected := mac.Sum(nil)

	// first 28 bytes must match exactly.
	if !hmac.Equal(saved[:28], expected[:28]) {
		return nil
	}

	// recover timestamp: random[28:32] = hmac[28:32] XOR ts_le.
	var tsBytes [4]byte
	for i := 0; i < 4; i++ {
		tsBytes[i] = saved[28+i] ^ expected[28+i]
	}
	ts := binary.LittleEndian.Uint32(tsBytes[:])
	now := uint32(time.Now().Unix())
	diff := int64(now) - int64(ts)
	if diff < -300 || diff > 300 {
		return nil
	}

	return append([]byte(nil), saved[:]...)
}

// send a minimal fake tls 1.2 server handshake:
//
//	ServerHello (signed random) → ChangeCipherSpec → ApplicationData (32 pseudo-random bytes)
//
// tg clients validate server_random as:
// hmac-sha256(secret, clientRandom || serverHello_record_with_zeroed_random)
// this is different from ClientHello, which uses the timestamp-masked form.
//
// tg drains Handshake and CCS until the first non-empty AppData record.
//
// clientRandom is the 32-byte random returned by AcceptClientHello.
func SendFakeServerHello(w io.Writer, secret, clientRandom []byte) error {
	var out []byte

	// 1. ServerHello signed with clientRandom-bound hmac
	var body []byte
	body = append(body, version12[:]...)     // server_version = TLS 1.2
	body = append(body, make([]byte, 32)...) // random: zeroed, filled by sign below
	body = append(body, 0x00)               // session_id_length = 0
	body = append(body, 0xc0, 0x2f)         // ECDHE_RSA_WITH_AES_128_GCM_SHA256
	body = append(body, 0x00)               // compression: null
	body = appendU16(body, 0)               // no extensions

	var hs []byte
	hs = append(hs, 0x02) // HandshakeType: ServerHello
	hs = append(hs, byte(len(body)>>16), byte(len(body)>>8), byte(len(body)))
	hs = append(hs, body...)

	// build ServerHello first, then compute digest over the whole server flight.
	shRec := make([]byte, 0, 5+len(hs))
	shRec = append(shRec, RecordHandshake, version12[0], version12[1])
	shRec = appendU16(shRec, uint16(len(hs)))
	shRec = append(shRec, hs...)
	out = append(out, shRec...)

	// 2. ChangeCipherSpec
	out = append(out, RecordChangeCipherSpec, version12[0], version12[1], 0x00, 0x01, 0x01)

	// 3. AppData with fake Finished payload. must be non-empty.
	fakeFinished := fakePseudoRandBytes(32)
	out = append(out, RecordApplicationData, version12[0], version12[1])
	out = appendU16(out, uint16(len(fakeFinished)))
	out = append(out, fakeFinished...)

	mac := hmac.New(sha256.New, secret)
	mac.Write(clientRandom)
	mac.Write(out)
	digest := mac.Sum(nil)
	copy(out[digestPos:digestPos+digestLen], digest)

	_, err := w.Write(out)
	return err
}

// deterministic pseudo-random bytes for fake tls padding.
func fakePseudoRandBytes(n int) []byte {
	b := make([]byte, n)
	ts := uint64(time.Now().UnixNano())
	for i := range b {
		ts = ts*6364136223846793005 + 1442695040888963407
		b[i] = byte(ts >> 56)
	}
	return b
}
