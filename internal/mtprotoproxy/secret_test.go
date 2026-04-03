package mtprotoproxy

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestGenerateSecret(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}

	if !strings.HasPrefix(secret, "ee") {
		t.Fatalf("expected ee prefix, got %q", secret)
	}

	if len(secret) != 2+32 { // "ee" + 32 hex chars
		t.Fatalf("expected length 34, got %d", len(secret))
	}

	// Verify hex part is valid.
	_, err = hex.DecodeString(secret[2:])
	if err != nil {
		t.Fatalf("invalid hex in secret: %v", err)
	}
}

func TestGenerateSecretUniqueness(t *testing.T) {
	s1, _ := GenerateSecret()
	s2, _ := GenerateSecret()
	if s1 == s2 {
		t.Fatal("two generated secrets should not be equal")
	}
}

func TestParseSecretValid(t *testing.T) {
	input := "ee" + strings.Repeat("ab", 16) // ee + 32 hex chars
	raw, err := ParseSecret(input)
	if err != nil {
		t.Fatalf("ParseSecret failed: %v", err)
	}
	if len(raw) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(raw))
	}
	for _, b := range raw {
		if b != 0xab {
			t.Fatalf("unexpected byte: 0x%02x", b)
		}
	}
}

func TestParseSecretUpperCase(t *testing.T) {
	input := "EE" + strings.Repeat("AB", 16)
	raw, err := ParseSecret(input)
	if err != nil {
		t.Fatalf("ParseSecret failed: %v", err)
	}
	if len(raw) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(raw))
	}
}

func TestParseSecretWithWhitespace(t *testing.T) {
	input := "  ee" + strings.Repeat("cd", 16) + "  "
	raw, err := ParseSecret(input)
	if err != nil {
		t.Fatalf("ParseSecret failed: %v", err)
	}
	if len(raw) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(raw))
	}
}

func TestParseSecretNoPrefix(t *testing.T) {
	input := strings.Repeat("ab", 16) // 32 hex chars, no ee prefix
	_, err := ParseSecret(input)
	if err != ErrSecretTooShort {
		t.Fatalf("expected ErrSecretTooShort, got %v", err)
	}
}

func TestParseSecretTooShort(t *testing.T) {
	_, err := ParseSecret("ee1234")
	if err != ErrSecretTooShort {
		t.Fatalf("expected ErrSecretTooShort, got %v", err)
	}
}

func TestParseSecretTooLong(t *testing.T) {
	input := "ee" + strings.Repeat("ab", 17) // 34 hex chars
	_, err := ParseSecret(input)
	if err != ErrSecretTooShort {
		t.Fatalf("expected ErrSecretTooShort, got %v", err)
	}
}

func TestParseSecretInvalidHex(t *testing.T) {
	input := "ee" + strings.Repeat("zz", 16)
	_, err := ParseSecret(input)
	if err != ErrSecretInvalid {
		t.Fatalf("expected ErrSecretInvalid, got %v", err)
	}
}

func TestParseSecretDDPrefix(t *testing.T) {
	input := "dd" + strings.Repeat("ab", 16)
	_, err := ParseSecret(input)
	if err != ErrSecretTooShort {
		t.Fatalf("expected ErrSecretTooShort for dd prefix, got %v", err)
	}
}

func TestParseSecretRoundTrip(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}

	raw, err := ParseSecret(secret)
	if err != nil {
		t.Fatalf("ParseSecret failed: %v", err)
	}

	reconstructed := "ee" + hex.EncodeToString(raw)
	if reconstructed != secret {
		t.Fatalf("round-trip mismatch: %q vs %q", reconstructed, secret)
	}
}

func TestFormatLink(t *testing.T) {
	link := FormatLink("192.168.1.1", 8443, "ee0123456789abcdef0123456789abcdef")
	want := "tg://proxy?server=192.168.1.1&port=8443&secret=ee0123456789abcdef0123456789abcdef"
	if link != want {
		t.Fatalf("unexpected link:\ngot:  %s\nwant: %s", link, want)
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		v    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{8443, "8443"},
		{65535, "65535"},
	}
	for _, tt := range tests {
		if got := itoa(tt.v); got != tt.want {
			t.Fatalf("itoa(%d) = %q, want %q", tt.v, got, tt.want)
		}
	}
}
