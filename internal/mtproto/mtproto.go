package mtproto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

const (
	ProtoAbridged           uint32 = 0xEFEFEFEF
	ProtoIntermediate       uint32 = 0xEEEEEEEE
	ProtoPaddedIntermediate uint32 = 0xDDDDDDDD
	initPacketSize                 = 64
)

var (
	ErrInitTooShort = errors.New("mtproto init packet is too short")
	ErrInvalidProto = errors.New("invalid mtproto transport protocol")
)

type InitInfo struct {
	DC      int
	IsMedia bool
	Proto   uint32
}

type Splitter struct {
	stream    cipher.Stream
	proto     uint32
	cipherBuf []byte
	plainBuf  []byte
	disabled  bool
}

func IsHTTPTransport(data []byte) bool {
	return len(data) >= 4 && (hasPrefix(data, []byte("POST ")) ||
		hasPrefix(data, []byte("GET ")) ||
		hasPrefix(data, []byte("HEAD ")) ||
		hasPrefix(data, []byte("OPTIONS ")))
}

func ParseInit(data []byte) (InitInfo, error) {
	dc, isMedia, proto, err := DCFromInit(data)
	if err != nil {
		return InitInfo{}, err
	}
	return InitInfo{DC: dc, IsMedia: isMedia, Proto: proto}, nil
}

func DCFromInit(data []byte) (dc int, isMedia bool, proto uint32, err error) {
	if len(data) < initPacketSize {
		return 0, false, 0, ErrInitTooShort
	}

	keystream, err := initKeystream(data)
	if err != nil {
		return 0, false, 0, err
	}

	var plain [8]byte
	for i := 0; i < len(plain); i++ {
		plain[i] = data[56+i] ^ keystream[56+i]
	}

	proto = binary.LittleEndian.Uint32(plain[:4])
	if !validProto(proto) {
		return 0, false, 0, ErrInvalidProto
	}

	dcRaw := int(int16(binary.LittleEndian.Uint16(plain[4:6])))
	dc = abs(dcRaw)
	isMedia = dcRaw < 0

	if dc < 1 || (dc > 5 && dc != 203) {
		return 0, false, proto, nil
	}

	return dc, isMedia, proto, nil
}

func PatchInitDC(data []byte, dc int) ([]byte, error) {
	if len(data) < initPacketSize {
		return nil, ErrInitTooShort
	}

	keystream, err := initKeystream(data)
	if err != nil {
		return nil, err
	}

	out := append([]byte(nil), data...)
	dcBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(dcBytes, uint16(int16(dc)))
	out[60] = keystream[60] ^ dcBytes[0]
	out[61] = keystream[61] ^ dcBytes[1]
	return out, nil
}

func NewSplitter(initData []byte, proto uint32) (*Splitter, error) {
	if len(initData) < initPacketSize {
		return nil, ErrInitTooShort
	}
	if !validProto(proto) {
		return nil, ErrInvalidProto
	}

	block, err := aes.NewCipher(initData[8:40])
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, initData[40:56])

	zero := make([]byte, initPacketSize)
	stream.XORKeyStream(zero, zero)

	return &Splitter{
		stream: stream,
		proto:  proto,
	}, nil
}

func (s *Splitter) Split(chunk []byte) [][]byte {
	if len(chunk) == 0 {
		return nil
	}
	if s.disabled {
		return [][]byte{append([]byte(nil), chunk...)}
	}

	s.cipherBuf = append(s.cipherBuf, chunk...)
	plain := append([]byte(nil), chunk...)
	s.stream.XORKeyStream(plain, plain)
	s.plainBuf = append(s.plainBuf, plain...)

	var parts [][]byte
	for len(s.cipherBuf) > 0 {
		packetLen, ok := s.nextPacketLen()
		if !ok {
			break
		}
		if packetLen <= 0 {
			parts = append(parts, append([]byte(nil), s.cipherBuf...))
			s.cipherBuf = s.cipherBuf[:0]
			s.plainBuf = s.plainBuf[:0]
			s.disabled = true
			break
		}

		parts = append(parts, append([]byte(nil), s.cipherBuf[:packetLen]...))
		s.cipherBuf = append([]byte(nil), s.cipherBuf[packetLen:]...)
		s.plainBuf = append([]byte(nil), s.plainBuf[packetLen:]...)
	}

	return parts
}

func (s *Splitter) Flush() [][]byte {
	if len(s.cipherBuf) == 0 {
		return nil
	}
	tail := append([]byte(nil), s.cipherBuf...)
	s.cipherBuf = s.cipherBuf[:0]
	s.plainBuf = s.plainBuf[:0]
	return [][]byte{tail}
}

func (s *Splitter) nextPacketLen() (int, bool) {
	if len(s.plainBuf) == 0 {
		return 0, false
	}
	switch s.proto {
	case ProtoAbridged:
		return s.nextAbridgedLen()
	case ProtoIntermediate, ProtoPaddedIntermediate:
		return s.nextIntermediateLen()
	default:
		return 0, true
	}
}

func (s *Splitter) nextAbridgedLen() (int, bool) {
	first := s.plainBuf[0]
	var payloadLen int
	headerLen := 1

	if first == 0x7F || first == 0xFF {
		if len(s.plainBuf) < 4 {
			return 0, false
		}
		payloadLen = int(uint32(s.plainBuf[1])|uint32(s.plainBuf[2])<<8|uint32(s.plainBuf[3])<<16) * 4
		headerLen = 4
	} else {
		payloadLen = int(first&0x7F) * 4
	}

	if payloadLen <= 0 {
		return 0, true
	}

	packetLen := headerLen + payloadLen
	if len(s.plainBuf) < packetLen {
		return 0, false
	}
	return packetLen, true
}

func (s *Splitter) nextIntermediateLen() (int, bool) {
	if len(s.plainBuf) < 4 {
		return 0, false
	}
	payloadLen := int(binary.LittleEndian.Uint32(s.plainBuf[:4]) & 0x7FFFFFFF)
	if payloadLen <= 0 {
		return 0, true
	}
	packetLen := 4 + payloadLen
	if len(s.plainBuf) < packetLen {
		return 0, false
	}
	return packetLen, true
}

func initKeystream(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(data[8:40])
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, data[40:56])
	zero := make([]byte, initPacketSize)
	keystream := make([]byte, initPacketSize)
	stream.XORKeyStream(keystream, zero)
	return keystream, nil
}

func validProto(proto uint32) bool {
	switch proto {
	case ProtoAbridged, ProtoIntermediate, ProtoPaddedIntermediate:
		return true
	default:
		return false
	}
}

func hasPrefix(data []byte, prefix []byte) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i := range prefix {
		if data[i] != prefix[i] {
			return false
		}
	}
	return true
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// ErrInvalidSecret is returned when the handshake does not match the expected secret.
var ErrInvalidSecret = errors.New("mtproto handshake: invalid secret")

// ErrInitGenFailed is returned when GenerateRelayInit exhausts retries.
var ErrInitGenFailed = errors.New("mtproto: failed to generate valid relay init")

// ParseInitWithSecret validates a 64-byte MTProto obfuscated handshake against
// secret, then extracts the DC, media flag, and transport protocol.
func ParseInitWithSecret(data []byte, secret []byte) (InitInfo, error) {
	if len(data) < initPacketSize {
		return InitInfo{}, ErrInitTooShort
	}

	h := sha256.New()
	h.Write(data[8:40])
	h.Write(secret)
	key := h.Sum(nil)

	block, err := aes.NewCipher(key)
	if err != nil {
		return InitInfo{}, err
	}
	stream := cipher.NewCTR(block, data[40:56])

	zero := make([]byte, initPacketSize)
	keystream := make([]byte, initPacketSize)
	stream.XORKeyStream(keystream, zero)

	var plain [8]byte
	for i := range plain {
		plain[i] = data[56+i] ^ keystream[56+i]
	}

	proto := binary.LittleEndian.Uint32(plain[:4])
	if !validProto(proto) {
		return InitInfo{}, ErrInvalidSecret
	}

	dcRaw := int(int16(binary.LittleEndian.Uint16(plain[4:6])))
	dc := abs(dcRaw)
	isMedia := dcRaw < 0
	if dc < 1 || (dc > 5 && dc != 203) {
		dc = 0
	}

	return InitInfo{DC: dc, IsMedia: isMedia, Proto: proto}, nil
}

// BuildConnectionCiphers returns the two AES-256-CTR streams for the client
// connection: clientDec for bytes read from the client, clientEnc for bytes
// written to the client.
//
// clientDec skips 64 bytes (the client already consumed them in the init packet).
// clientEnc starts at position 0 - the client's inbound stream has no skip.
func BuildConnectionCiphers(handshake [64]byte, secret []byte) (clientDec, clientEnc cipher.Stream, err error) {
	clientDec, err = newSecretCTR(handshake[8:40], handshake[40:56], secret, false, initPacketSize)
	if err != nil {
		return nil, nil, err
	}
	clientEnc, err = newSecretCTR(handshake[8:40], handshake[40:56], secret, true, 0)
	if err != nil {
		return nil, nil, err
	}
	return clientDec, clientEnc, nil
}

// GenerateRelayInit produces a fresh 64-byte MTProto obfuscated init packet to
// send to Telegram, together with the two AES-256-CTR streams for the relay
// side: relayEnc for bytes sent to Telegram, relayDec for bytes received from
// Telegram.
func GenerateRelayInit(proto uint32, dc int) ([64]byte, cipher.Stream, cipher.Stream, error) {
	for range 100 {
		var buf [64]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return [64]byte{}, nil, nil, err
		}
		if !validRelayInitBytes(buf[:]) {
			continue
		}

		ks, err := initKeystream(buf[:])
		if err != nil {
			return [64]byte{}, nil, nil, err
		}
		var plain [8]byte
		binary.LittleEndian.PutUint32(plain[:4], proto)
		binary.LittleEndian.PutUint16(plain[4:6], uint16(int16(dc)))
		for i := range plain {
			buf[56+i] = plain[i] ^ ks[56+i]
		}

		// relayEnc skips 64 bytes - we already sent them as the relay init.
		// relayDec starts at position 0 - Telegram's response stream has no skip.
		relayEnc, err := newDirectCTR(buf[8:40], buf[40:56], false, initPacketSize)
		if err != nil {
			return [64]byte{}, nil, nil, err
		}
		relayDec, err := newDirectCTR(buf[8:40], buf[40:56], true, 0)
		if err != nil {
			return [64]byte{}, nil, nil, err
		}
		return buf, relayEnc, relayDec, nil
	}
	return [64]byte{}, nil, nil, ErrInitGenFailed
}

// newSecretCTR creates an AES-256-CTR stream keyed with SHA-256(rawKey ∥ secret).
// If reverse is true, the entire rawKey∥iv block is reversed as one unit before
// hashing/use - matching the MTProto obfuscation spec where the server->client
// cipher uses reverse(prekey ∥ IV)[0:32] as the key and reverse(prekey ∥ IV)[32:48]
// as the IV.  Reversing each field independently produces different results.
// skip is the number of keystream bytes to fast-forward past.
func newSecretCTR(rawKey, iv, secret []byte, reverse bool, skip int) (cipher.Stream, error) {
	k := rawKey
	v := iv
	if reverse {
		combined := make([]byte, len(rawKey)+len(iv))
		copy(combined, rawKey)
		copy(combined[len(rawKey):], iv)
		rev := reverseSlice(combined)
		k = rev[:len(rawKey)]
		v = rev[len(rawKey):]
	}
	h := sha256.New()
	h.Write(k)
	h.Write(secret)
	key := h.Sum(nil)
	return newCTRFastForward(key, v, skip)
}

// newDirectCTR creates an AES-256-CTR stream using rawKey directly (no secret).
// If reverse is true, the entire rawKey∥iv block is reversed as one unit -
// same convention as newSecretCTR.
// skip is the number of keystream bytes to fast-forward past.
func newDirectCTR(rawKey, iv []byte, reverse bool, skip int) (cipher.Stream, error) {
	k := rawKey
	v := iv
	if reverse {
		combined := make([]byte, len(rawKey)+len(iv))
		copy(combined, rawKey)
		copy(combined[len(rawKey):], iv)
		rev := reverseSlice(combined)
		k = rev[:len(rawKey)]
		v = rev[len(rawKey):]
	}
	return newCTRFastForward(k, v, skip)
}

func newCTRFastForward(key, iv []byte, skip int) (cipher.Stream, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, iv)
	if skip > 0 {
		discard := make([]byte, skip)
		stream.XORKeyStream(discard, discard)
	}
	return stream, nil
}

func reverseSlice(b []byte) []byte {
	r := make([]byte, len(b))
	for i, v := range b {
		r[len(b)-1-i] = v
	}
	return r
}

// validRelayInitBytes returns false if data begins with any reserved sequence
// that Telegram (or DPI) would misidentify as a different protocol.
func validRelayInitBytes(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	// Abridged tag
	if data[0] == 0xef {
		return false
	}
	// Intermediate / padded-intermediate markers
	if data[0] == 0xee && data[1] == 0xee && data[2] == 0xee && data[3] == 0xee {
		return false
	}
	if data[0] == 0xdd && data[1] == 0xdd && data[2] == 0xdd && data[3] == 0xdd {
		return false
	}
	// HTTP methods
	if hasPrefix(data, []byte("GET ")) || hasPrefix(data, []byte("POST")) || hasPrefix(data, []byte("HEAD")) {
		return false
	}
	// TLS ClientHello
	if data[0] == 0x16 && data[1] == 0x03 && data[2] == 0x01 && data[3] == 0x02 {
		return false
	}
	// All-zero bytes at [4:8]
	if data[4] == 0x00 && data[5] == 0x00 && data[6] == 0x00 && data[7] == 0x00 {
		return false
	}
	return true
}
