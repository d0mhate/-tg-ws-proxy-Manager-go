package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestDCIPFlagsSetAndString(t *testing.T) {
	var flags dcIPFlags

	if err := flags.Set("2:149.154.167.220"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if err := flags.Set("4:149.154.167.220"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	if got, want := len(flags), 2; got != want {
		t.Fatalf("unexpected number of flag values: got %d want %d", got, want)
	}

	if got := flags.String(); got != "[2:149.154.167.220 4:149.154.167.220]" {
		t.Fatalf("unexpected String output: %q", got)
	}
}

func TestParseArgsDefaults(t *testing.T) {
	cfg, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if cfg.Host != "127.0.0.1" {
		t.Fatalf("unexpected default host: %q", cfg.Host)
	}
	if cfg.Port != 1080 {
		t.Fatalf("unexpected default port: %d", cfg.Port)
	}
}

func TestParseArgsOverridesHostAndPort(t *testing.T) {
	cfg, err := parseArgs([]string{"--host", "0.0.0.0", "--port", "19080"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if cfg.Host != "0.0.0.0" {
		t.Fatalf("unexpected overridden host: %q", cfg.Host)
	}
	if cfg.Port != 19080 {
		t.Fatalf("unexpected overridden port: %d", cfg.Port)
	}
}

func TestParseArgsUsernamePassword(t *testing.T) {
	cfg, err := parseArgs([]string{"--username", "alice", "--password", "secret"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if cfg.Username != "alice" {
		t.Fatalf("unexpected username: %q", cfg.Username)
	}
	if cfg.Password != "secret" {
		t.Fatalf("unexpected password: %q", cfg.Password)
	}
}

func TestParseArgsRejectsPartialUsernamePassword(t *testing.T) {
	if _, err := parseArgs([]string{"--username", "alice"}); err == nil {
		t.Fatal("expected parseArgs to reject username without password")
	}
	if _, err := parseArgs([]string{"--password", "secret"}); err == nil {
		t.Fatal("expected parseArgs to reject password without username")
	}
}

func TestParseArgsMTProtoEnabled(t *testing.T) {
	cfg, err := parseArgs([]string{"--mtproto", "--mtproto-port", "8443"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !cfg.MTProtoEnabled {
		t.Fatal("expected MTProtoEnabled to be true")
	}
	if cfg.MTProtoPort != 8443 {
		t.Fatalf("unexpected mtproto port: %d", cfg.MTProtoPort)
	}
	// Secret should be auto-generated.
	if cfg.MTProtoSecret == "" {
		t.Fatal("expected auto-generated secret")
	}
	if !strings.HasPrefix(cfg.MTProtoSecret, "ee") {
		t.Fatalf("expected ee prefix, got %q", cfg.MTProtoSecret)
	}
}

func TestParseArgsMTProtoCustomSecret(t *testing.T) {
	secret := "ee0123456789abcdef0123456789abcdef"
	cfg, err := parseArgs([]string{"--mtproto", "--mtproto-port", "8443", "--mtproto-secret", secret})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if cfg.MTProtoSecret != secret {
		t.Fatalf("unexpected secret: %q", cfg.MTProtoSecret)
	}
}

func TestParseArgsMTProtoRequiresPort(t *testing.T) {
	_, err := parseArgs([]string{"--mtproto"})
	if err == nil {
		t.Fatal("expected error when --mtproto-port is not set")
	}
}

func TestParseArgsMTProtoInvalidSecret(t *testing.T) {
	_, err := parseArgs([]string{"--mtproto", "--mtproto-port", "8443", "--mtproto-secret", "badvalue"})
	if err == nil {
		t.Fatal("expected error for invalid secret")
	}
}

func TestParseArgsMTProtoOnlyMode(t *testing.T) {
	cfg, err := parseArgs([]string{"--socks5=false", "--mtproto", "--mtproto-port", "8443"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if cfg.SOCKS5Enabled {
		t.Fatal("expected SOCKS5Enabled to be false")
	}
	if !cfg.MTProtoEnabled {
		t.Fatal("expected MTProtoEnabled to be true")
	}
}

func TestParseArgsRejectsAllModesDisabled(t *testing.T) {
	_, err := parseArgs([]string{"--socks5=false"})
	if err == nil {
		t.Fatal("expected error when all proxy modes are disabled")
	}
}

func TestHandleUtilityModeQR(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	handled, err := handleUtilityMode([]string{"qr", "tg://proxy?server=127.0.0.1&port=8443&secret=ee0123456789abcdef0123456789abcdef"})
	_ = w.Close()
	if err != nil {
		t.Fatalf("handleUtilityMode returned error: %v", err)
	}
	if !handled {
		t.Fatal("expected QR utility mode to be handled")
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(bytes.TrimSpace(out)) == 0 {
		t.Fatal("expected QR output")
	}
	if !strings.ContainsAny(string(out), "▀▄█") {
		t.Fatalf("expected QR block characters, got:\n%s", string(out))
	}
}
