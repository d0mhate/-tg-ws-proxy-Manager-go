package mtprotoproxy

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	secretLen      = 16 // 16 bytes = 32 hex chars
	fakeTLSPrefix  = "ee"
	defaultDomain  = "www.google.com"
)

var (
	ErrSecretTooShort = errors.New("secret must be 32 hex characters (with optional ee prefix)")
	ErrSecretInvalid  = errors.New("secret contains invalid hex characters")
)

// GenerateSecret creates a new random 16-byte secret and returns it
// as an ee-prefixed hex string for fake-TLS mode.
func GenerateSecret() (string, error) {
	raw := make([]byte, secretLen)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return fakeTLSPrefix + hex.EncodeToString(raw), nil
}

func DefaultFakeTLSDomain() string {
	return defaultDomain
}

func NormalizeFakeTLSDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return defaultDomain
	}
	return domain
}

// ParseSecret parses an ee-prefixed hex secret string and returns
// the raw 16-byte secret. Only fake-TLS (ee) secrets are supported.
func ParseSecret(s string) ([]byte, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	if !strings.HasPrefix(s, fakeTLSPrefix) {
		return nil, ErrSecretTooShort
	}
	s = s[len(fakeTLSPrefix):]

	if len(s) < secretLen*2 || len(s)%2 != 0 {
		return nil, ErrSecretTooShort
	}

	if _, err := hex.DecodeString(s); err != nil {
		return nil, ErrSecretInvalid
	}

	raw, err := hex.DecodeString(s[:secretLen*2])
	if err != nil {
		return nil, ErrSecretInvalid
	}
	return raw, nil
}

// FormatLink returns a tg://proxy link for the given server, port and secret.
func FormatLink(host string, port int, secret, domain string) string {
	return "tg://proxy?server=" + host + "&port=" + itoa(port) + "&secret=" + LinkSecret(secret, domain)
}

func LinkSecret(secret, domain string) string {
	secret = strings.ToLower(strings.TrimSpace(secret))
	if !strings.HasPrefix(secret, fakeTLSPrefix) || len(secret) != 2+secretLen*2 {
		return secret
	}
	return secret + hex.EncodeToString([]byte(NormalizeFakeTLSDomain(domain)))
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
