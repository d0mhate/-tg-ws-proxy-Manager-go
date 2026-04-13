package main

import (
	"testing"

	"tg-ws-proxy/internal/config"
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

func TestParseArgsCFProxyWithDomain(t *testing.T) {
	cfg, err := parseArgs([]string{"--cf-proxy", "--cf-domain", "example.com"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !cfg.UseCFProxy {
		t.Fatal("expected UseCFProxy to be true")
	}
	if cfg.CFDomain != "example.com" {
		t.Fatalf("unexpected CFDomain: %q", cfg.CFDomain)
	}
	if cfg.UseCFProxyFirst {
		t.Fatal("expected CF proxy first to stay disabled unless requested")
	}
}

func TestParseArgsCFProxyFirst(t *testing.T) {
	cfg, err := parseArgs([]string{"--cf-proxy", "--cf-proxy-first", "--cf-domain", "example.com"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !cfg.UseCFProxyFirst {
		t.Fatal("expected UseCFProxyFirst to be true")
	}
}

func TestParseArgsCFProxyDefaultsDisabled(t *testing.T) {
	cfg, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if cfg.UseCFProxy {
		t.Fatal("expected CF proxy to be disabled by default")
	}
	if cfg.CFDomain != config.DefaultCFDomain {
		t.Fatalf("expected CF domain %q, got %q", config.DefaultCFDomain, cfg.CFDomain)
	}
}

func TestParseArgsCFDomainValidation(t *testing.T) {
	invalid := []string{"not-a-domain", "example", "has space.com"}
	for _, d := range invalid {
		if _, err := parseArgs([]string{"--cf-domain", d}); err == nil {
			t.Errorf("expected invalid domain %q to be rejected", d)
		}
	}

	valid := []string{"example.com", "sub.example.com", "my-domain.org"}
	for _, d := range valid {
		if _, err := parseArgs([]string{"--cf-domain", d}); err != nil {
			t.Errorf("expected valid domain %q, got %v", d, err)
		}
	}
}
