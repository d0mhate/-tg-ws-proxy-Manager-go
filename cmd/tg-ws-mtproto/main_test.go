package main

import (
	"strings"
	"testing"
)

func TestExpandSecretAppendsDomainHex(t *testing.T) {
	secret, err := expandSecret("ee0123456789abcdef0123456789abcdef", "www.google.com")
	if err != nil {
		t.Fatalf("expandSecret failed: %v", err)
	}

	if got, want := secret, "ee0123456789abcdef0123456789abcdef7777772e676f6f676c652e636f6d"; got != want {
		t.Fatalf("unexpected secret: %q", got)
	}
}

func TestExpandSecretKeepsExpandedValue(t *testing.T) {
	secret := "ee0123456789abcdef0123456789abcdef7777772e676f6f676c652e636f6d"
	got, err := expandSecret(secret, "example.com")
	if err != nil {
		t.Fatalf("expandSecret failed: %v", err)
	}
	if got != secret {
		t.Fatalf("unexpected secret: %q", got)
	}
}

func TestParseArgsDefaultsPublicHost(t *testing.T) {
	opts, err := parseArgs([]string{"--secret", "ee0123456789abcdef0123456789abcdef"})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}

	if opts.PublicHost != "127.0.0.1" {
		t.Fatalf("unexpected public host: %q", opts.PublicHost)
	}
	if !strings.HasSuffix(opts.Secret, "7777772e676f6f676c652e636f6d") {
		t.Fatalf("expected domain hex suffix, got %q", opts.Secret)
	}
}

func TestGenerateSecretProducesExpandedFakeTLSSecret(t *testing.T) {
	secret, err := generateSecret("www.google.com")
	if err != nil {
		t.Fatalf("generateSecret failed: %v", err)
	}
	if !strings.HasPrefix(secret, "ee") {
		t.Fatalf("expected ee-prefixed secret, got %q", secret)
	}
	if !strings.HasSuffix(secret, "7777772e676f6f676c652e636f6d") {
		t.Fatalf("expected domain hex suffix, got %q", secret)
	}
}
