package mtproto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

func TestDCFromInit(t *testing.T) {
	init := makeInitPacket(t, ProtoIntermediate, -4)

	dc, isMedia, proto, err := DCFromInit(init)
	if err != nil {
		t.Fatalf("DCFromInit returned error: %v", err)
	}
	if dc != 4 {
		t.Fatalf("unexpected dc: %d", dc)
	}
	if !isMedia {
		t.Fatal("expected media flag")
	}
	if proto != ProtoIntermediate {
		t.Fatalf("unexpected proto: 0x%08x", proto)
	}
}

func TestPatchInitDC(t *testing.T) {
	init := makeInitPacket(t, ProtoAbridged, 2)

	patched, err := PatchInitDC(init, -5)
	if err != nil {
		t.Fatalf("PatchInitDC returned error: %v", err)
	}

	dc, isMedia, proto, err := DCFromInit(patched)
	if err != nil {
		t.Fatalf("DCFromInit returned error after patch: %v", err)
	}
	if dc != 5 || !isMedia || proto != ProtoAbridged {
		t.Fatalf("unexpected parsed values after patch: dc=%d media=%v proto=0x%08x", dc, isMedia, proto)
	}
}

func TestParseInitUnknownDCStillReturnsProto(t *testing.T) {
	init := makeInitPacket(t, ProtoPaddedIntermediate, 100)

	info, err := ParseInit(init)
	if err != nil {
		t.Fatalf("ParseInit returned error: %v", err)
	}
	if info.DC != 0 {
		t.Fatalf("expected unknown dc to map to 0, got %d", info.DC)
	}
	if info.Proto != ProtoPaddedIntermediate {
		t.Fatalf("unexpected proto: 0x%08x", info.Proto)
	}
}

func TestIsHTTPTransport(t *testing.T) {
	if !IsHTTPTransport([]byte("POST / HTTP/1.1")) {
		t.Fatal("expected POST transport to be detected")
	}
	if IsHTTPTransport([]byte{0xef, 0xef, 0xef, 0xef}) {
		t.Fatal("did not expect mtproto bytes to look like http")
	}
}

func TestSplitterIntermediate(t *testing.T) {
	init := makeInitPacket(t, ProtoIntermediate, 2)
	splitter, err := NewSplitter(init, ProtoIntermediate)
	if err != nil {
		t.Fatalf("NewSplitter returned error: %v", err)
	}

	plain1 := makeIntermediatePacket([]byte("hello"))
	plain2 := makeIntermediatePacket([]byte("world!"))
	cipherText := encryptAfterInit(t, init, append(plain1, plain2...))

	parts := splitter.Split(cipherText[:7])
	if len(parts) != 0 {
		t.Fatalf("expected no packet after partial split, got %d", len(parts))
	}

	parts = splitter.Split(cipherText[7:11])
	if len(parts) != 1 {
		t.Fatalf("expected first packet after completing split, got %d", len(parts))
	}
	if !bytes.Equal(parts[0], cipherText[:len(plain1)]) {
		t.Fatal("first packet ciphertext boundary mismatch")
	}

	parts = splitter.Split(cipherText[11:])
	if len(parts) != 1 {
		t.Fatalf("expected second packet after tail split, got %d", len(parts))
	}
	if !bytes.Equal(parts[0], cipherText[len(plain1):]) {
		t.Fatal("second packet ciphertext boundary mismatch")
	}
}

func makeInitPacket(t *testing.T, proto uint32, dc int16) []byte {
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

func makeIntermediatePacket(payload []byte) []byte {
	out := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(out[:4], uint32(len(payload)))
	copy(out[4:], payload)
	return out
}

func encryptAfterInit(t *testing.T, init []byte, plain []byte) []byte {
	t.Helper()

	block, err := aes.NewCipher(init[8:40])
	if err != nil {
		t.Fatalf("aes.NewCipher failed: %v", err)
	}
	stream := cipher.NewCTR(block, init[40:56])
	zero := make([]byte, 64)
	stream.XORKeyStream(zero, zero)

	out := append([]byte(nil), plain...)
	stream.XORKeyStream(out, out)
	return out
}

func decryptAfterInit(t *testing.T, init []byte, cipherText []byte) []byte {
	t.Helper()
	return encryptAfterInit(t, init, cipherText)
}

func initKeystreamForTest(t *testing.T, init []byte) []byte {
	t.Helper()
	ks, err := initKeystream(init)
	if err != nil {
		t.Fatalf("initKeystream failed: %v", err)
	}
	return ks
}

// makeSecretInitPacket builds a 64-byte handshake encrypted with secret,
// encoding the given proto and dc values.
func makeSecretInitPacket(t *testing.T, proto uint32, dc int16, secret []byte) []byte {
	t.Helper()

	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i + 1)
	}

	// Derive key with secret: SHA-256(data[8:40] ∥ secret)
	h := sha256.New()
	h.Write(data[8:40])
	h.Write(secret)
	key := h.Sum(nil)

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	stream := cipher.NewCTR(block, data[40:56])
	zero := make([]byte, 64)
	ks := make([]byte, 64)
	stream.XORKeyStream(ks, zero)

	var plain [8]byte
	binary.LittleEndian.PutUint32(plain[:4], proto)
	binary.LittleEndian.PutUint16(plain[4:6], uint16(dc))
	for i := range plain {
		data[56+i] = plain[i] ^ ks[56+i]
	}
	return data
}

func TestParseInitWithSecret(t *testing.T) {
	secret := []byte("testsecret123456")
	data := makeSecretInitPacket(t, ProtoIntermediate, -3, secret)

	info, err := ParseInitWithSecret(data, secret)
	if err != nil {
		t.Fatalf("ParseInitWithSecret returned error: %v", err)
	}
	if info.DC != 3 {
		t.Fatalf("expected dc=3, got %d", info.DC)
	}
	if !info.IsMedia {
		t.Fatal("expected isMedia=true")
	}
	if info.Proto != ProtoIntermediate {
		t.Fatalf("expected ProtoIntermediate, got 0x%08x", info.Proto)
	}
}

func TestParseInitWithSecretWrongSecret(t *testing.T) {
	data := makeSecretInitPacket(t, ProtoIntermediate, 2, []byte("correctsecret123"))
	_, err := ParseInitWithSecret(data, []byte("wrongsecret12345"))
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestParseInitWithSecretTooShort(t *testing.T) {
	_, err := ParseInitWithSecret(make([]byte, 32), []byte("secret"))
	if err != ErrInitTooShort {
		t.Fatalf("expected ErrInitTooShort, got %v", err)
	}
}

func TestBuildConnectionCiphers(t *testing.T) {
	secret := []byte("testsecret123456")
	raw := makeSecretInitPacket(t, ProtoAbridged, 1, secret)

	var handshake [64]byte
	copy(handshake[:], raw)

	// Simulate proxy → client direction: encrypt with enc, decrypt by rebuilding enc.
	_, clientEnc, err := BuildConnectionCiphers(handshake, secret)
	if err != nil {
		t.Fatalf("BuildConnectionCiphers returned error: %v", err)
	}

	plaintext := []byte("hello mtproto world")
	ciphertext := make([]byte, len(plaintext))
	copy(ciphertext, plaintext)
	clientEnc.XORKeyStream(ciphertext, ciphertext)

	// Rebuild enc on the "client" side to decrypt - same key/iv, same CTR stream.
	_, clientEnc2, err := BuildConnectionCiphers(handshake, secret)
	if err != nil {
		t.Fatalf("BuildConnectionCiphers (2nd) returned error: %v", err)
	}
	recovered := make([]byte, len(ciphertext))
	copy(recovered, ciphertext)
	clientEnc2.XORKeyStream(recovered, recovered)

	if !bytes.Equal(recovered, plaintext) {
		t.Fatalf("enc round-trip mismatch: got %q, want %q", recovered, plaintext)
	}

	// Simulate client → proxy direction: encrypt with dec, decrypt by rebuilding dec.
	var hs3 [64]byte
	copy(hs3[:], raw)
	clientDec, _, err := BuildConnectionCiphers(hs3, secret)
	if err != nil {
		t.Fatalf("BuildConnectionCiphers (3rd) returned error: %v", err)
	}
	ct2 := append([]byte(nil), plaintext...)
	clientDec.XORKeyStream(ct2, ct2)

	var hs4 [64]byte
	copy(hs4[:], raw)
	clientDec2, _, err := BuildConnectionCiphers(hs4, secret)
	if err != nil {
		t.Fatalf("BuildConnectionCiphers (4th) returned error: %v", err)
	}
	clientDec2.XORKeyStream(ct2, ct2)
	if !bytes.Equal(ct2, plaintext) {
		t.Fatalf("dec round-trip mismatch: got %q, want %q", ct2, plaintext)
	}
}

func TestBuildConnectionCiphersEncDecAreDifferent(t *testing.T) {
	secret := []byte("testsecret123456")
	raw := makeSecretInitPacket(t, ProtoAbridged, 2, secret)

	var handshake [64]byte
	copy(handshake[:], raw)

	_, _, err := BuildConnectionCiphers(handshake, secret)
	if err != nil {
		t.Fatalf("BuildConnectionCiphers returned error: %v", err)
	}

	// Verify enc and dec produce different keystreams by encrypting zero bytes.
	var hs2 [64]byte
	copy(hs2[:], raw)
	dec, enc, _ := BuildConnectionCiphers(hs2, secret)

	decKS := make([]byte, 32)
	encKS := make([]byte, 32)
	dec.XORKeyStream(decKS, decKS)
	enc.XORKeyStream(encKS, encKS)

	if bytes.Equal(decKS, encKS) {
		t.Fatal("expected dec and enc keystreams to differ")
	}
}

// TestCipherReverseIsWholeBlock verifies that the "reverse" cipher mode reverses
// the entire prekey∥IV block as one unit, not each field independently.
// This matches the MTProto obfuscation spec (and the Rust reference impl).
//
// If this test fails it means newSecretCTR / newDirectCTR were changed to
// reverse the prekey and IV separately - which would break client decryption.
func TestCipherReverseIsWholeBlock(t *testing.T) {
	// Fixed 48-byte block: first 32 are the "prekey", last 16 are the IV.
	prekey := make([]byte, 32)
	iv := make([]byte, 16)
	for i := range prekey {
		prekey[i] = byte(i + 1) // 1..32
	}
	for i := range iv {
		iv[i] = byte(i + 33) // 33..48
	}
	secret := []byte("testsecret123456")

	// Expected: reverse the entire 48-byte prekey∥iv, then split.
	combined := append(append([]byte(nil), prekey...), iv...)
	rev := reverseSlice(combined)
	wantKey := rev[:32]
	wantIV := rev[32:]

	// Derive the expected AES-CTR keystream manually.
	h := sha256.New()
	h.Write(wantKey)
	h.Write(secret)
	aesKey := h.Sum(nil)
	block, _ := aes.NewCipher(aesKey)
	wantStream := cipher.NewCTR(block, wantIV)
	wantKS := make([]byte, 32)
	wantStream.XORKeyStream(wantKS, wantKS)

	// Obtain the keystream from newSecretCTR with reverse=true.
	got, err := newSecretCTR(prekey, iv, secret, true, 0)
	if err != nil {
		t.Fatalf("newSecretCTR: %v", err)
	}
	gotKS := make([]byte, 32)
	got.XORKeyStream(gotKS, gotKS)

	if !bytes.Equal(gotKS, wantKS) {
		t.Fatalf("newSecretCTR reverse mode produces wrong keystream\n"+
			"got:  %x\nwant: %x\n"+
			"This means the prekey and IV were reversed separately instead of as one 48-byte block.",
			gotKS, wantKS)
	}
}

func TestGenerateRelayInit(t *testing.T) {
	buf, relayEnc, relayDec, err := GenerateRelayInit(ProtoIntermediate, 2)
	if err != nil {
		t.Fatalf("GenerateRelayInit returned error: %v", err)
	}

	// Packet must pass reserved-byte checks.
	if !validRelayInitBytes(buf[:]) {
		t.Fatal("generated relay init failed reserved-byte check")
	}

	// Proto and DC must round-trip through DCFromInit.
	dc, isMedia, proto, err := DCFromInit(buf[:])
	if err != nil {
		t.Fatalf("DCFromInit on relay init: %v", err)
	}
	if proto != ProtoIntermediate {
		t.Fatalf("expected ProtoIntermediate, got 0x%08x", proto)
	}
	if dc != 2 {
		t.Fatalf("expected dc=2, got %d", dc)
	}
	if isMedia {
		t.Fatal("expected isMedia=false")
	}

	// relayEnc and relayDec must produce different keystreams.
	encKS := make([]byte, 32)
	decKS := make([]byte, 32)
	relayEnc.XORKeyStream(encKS, encKS)
	relayDec.XORKeyStream(decKS, decKS)
	if bytes.Equal(encKS, decKS) {
		t.Fatal("expected relayEnc and relayDec keystreams to differ")
	}
}
