package mtprotoproxy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"net"
	"time"
)

const initPacketSize = 64

var (
	ErrInitTooShort  = errors.New("mtproto proxy: init packet too short")
	ErrBadProtocol   = errors.New("mtproto proxy: invalid protocol in init")
)

// ProxyInit holds the result of decrypting the client's obfuscated2 init packet.
type ProxyInit struct {
	DC          int
	IsMedia     bool
	Proto       uint32
	Decryptor   cipher.Stream // client -> proxy direction
	Encryptor   cipher.Stream // proxy -> client direction
}

// DecryptInit decrypts the 64-byte obfuscated2 init packet using the proxy secret.
// Returns the parsed DC/protocol info and cipher streams for the connection.
func DecryptInit(initPacket []byte, secret []byte) (*ProxyInit, error) {
	if len(initPacket) < initPacketSize {
		return nil, ErrInitTooShort
	}

	// Client -> Proxy decrypt key: SHA256(init[8:40] + secret)
	decKey := sha256sum(initPacket[8:40], secret)
	decIV := make([]byte, 16)
	copy(decIV, initPacket[40:56])

	// Proxy -> Client encrypt key: SHA256(reverse(init[8:40]) + secret)
	reversed := reverseBytes(initPacket[8:40])
	encKey := sha256sum(reversed, secret)
	encIVSrc := reverseBytes(initPacket[40:56])
	encIV := make([]byte, 16)
	copy(encIV, encIVSrc)

	decryptor, err := newCTR(decKey, decIV)
	if err != nil {
		return nil, err
	}

	encryptor, err := newCTR(encKey, encIV)
	if err != nil {
		return nil, err
	}

	// Decrypt the init packet to read protocol and DC.
	decrypted := make([]byte, initPacketSize)
	decryptor.XORKeyStream(decrypted, initPacket)

	proto := binary.LittleEndian.Uint32(decrypted[56:60])
	if !validProto(proto) {
		return nil, ErrBadProtocol
	}

	dcRaw := int(int16(binary.LittleEndian.Uint16(decrypted[60:62])))
	dc := dcRaw
	isMedia := false
	if dc < 0 {
		dc = -dc
		isMedia = true
	}

	return &ProxyInit{
		DC:        dc,
		IsMedia:   isMedia,
		Proto:     proto,
		Decryptor: decryptor,
		Encryptor: encryptor,
	}, nil
}

// CryptoConn wraps a net.Conn with AES-CTR encryption/decryption.
type CryptoConn struct {
	inner     net.Conn
	decryptor cipher.Stream
	encryptor cipher.Stream
}

// NewCryptoConn wraps conn with proxy obfuscation streams.
func NewCryptoConn(conn net.Conn, decryptor, encryptor cipher.Stream) *CryptoConn {
	return &CryptoConn{
		inner:     conn,
		decryptor: decryptor,
		encryptor: encryptor,
	}
}

func (c *CryptoConn) Read(b []byte) (int, error) {
	n, err := c.inner.Read(b)
	if n > 0 {
		c.decryptor.XORKeyStream(b[:n], b[:n])
	}
	return n, err
}

func (c *CryptoConn) Write(b []byte) (int, error) {
	encrypted := make([]byte, len(b))
	c.encryptor.XORKeyStream(encrypted, b)
	return c.inner.Write(encrypted)
}

func (c *CryptoConn) Close() error                       { return c.inner.Close() }
func (c *CryptoConn) LocalAddr() net.Addr                { return c.inner.LocalAddr() }
func (c *CryptoConn) RemoteAddr() net.Addr               { return c.inner.RemoteAddr() }
func (c *CryptoConn) SetDeadline(t time.Time) error      { return c.inner.SetDeadline(t) }
func (c *CryptoConn) SetReadDeadline(t time.Time) error  { return c.inner.SetReadDeadline(t) }
func (c *CryptoConn) SetWriteDeadline(t time.Time) error { return c.inner.SetWriteDeadline(t) }

func sha256sum(parts ...[]byte) []byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

func reverseBytes(src []byte) []byte {
	out := make([]byte, len(src))
	for i, b := range src {
		out[len(src)-1-i] = b
	}
	return out
}

func newCTR(key, iv []byte) (cipher.Stream, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewCTR(block, iv), nil
}

func validProto(proto uint32) bool {
	switch proto {
	case 0xEFEFEFEF, 0xEEEEEEEE, 0xDDDDDDDD:
		return true
	default:
		return false
	}
}
