package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
	"time"
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
	pa, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if pa.cfg.Host != "127.0.0.1" {
		t.Fatalf("unexpected default host: %q", pa.cfg.Host)
	}
	if pa.cfg.Port != 1080 {
		t.Fatalf("unexpected default port: %d", pa.cfg.Port)
	}
	if pa.mode != "socks5" {
		t.Fatalf("unexpected default mode: %q", pa.mode)
	}
	if pa.cfg.PoolMaxAge != 55*time.Second {
		t.Fatalf("unexpected default pool max age: %s", pa.cfg.PoolMaxAge)
	}
	if pa.cfg.PoolRefillDelay != 250*time.Millisecond {
		t.Fatalf("unexpected default pool refill delay: %s", pa.cfg.PoolRefillDelay)
	}
}

func TestParseArgsOverridesHostAndPort(t *testing.T) {
	pa, err := parseArgs([]string{"--host", "0.0.0.0", "--port", "19080"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if pa.cfg.Host != "0.0.0.0" {
		t.Fatalf("unexpected overridden host: %q", pa.cfg.Host)
	}
	if pa.cfg.Port != 19080 {
		t.Fatalf("unexpected overridden port: %d", pa.cfg.Port)
	}
}

func TestParseArgsPprofAddr(t *testing.T) {
	pa, err := parseArgs([]string{"--pprof-addr", "127.0.0.1:6060"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if pa.cfg.PprofAddr != "127.0.0.1:6060" {
		t.Fatalf("unexpected pprof addr: %q", pa.cfg.PprofAddr)
	}
}

func TestParseArgsPoolTuning(t *testing.T) {
	pa, err := parseArgs([]string{"--pool-max-age", "90s", "--pool-refill-delay", "500ms"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if pa.cfg.PoolMaxAge != 90*time.Second {
		t.Fatalf("unexpected pool max age: %s", pa.cfg.PoolMaxAge)
	}
	if pa.cfg.PoolRefillDelay != 500*time.Millisecond {
		t.Fatalf("unexpected pool refill delay: %s", pa.cfg.PoolRefillDelay)
	}
}

func TestParseArgsRejectsNegativePoolTuning(t *testing.T) {
	if _, err := parseArgs([]string{"--pool-max-age", "-1s"}); err == nil {
		t.Fatal("expected negative pool max age to be rejected")
	}
	if _, err := parseArgs([]string{"--pool-refill-delay", "-1ms"}); err == nil {
		t.Fatal("expected negative pool refill delay to be rejected")
	}
}

func TestParseArgsUsernamePassword(t *testing.T) {
	pa, err := parseArgs([]string{"--username", "alice", "--password", "secret"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if pa.cfg.Username != "alice" {
		t.Fatalf("unexpected username: %q", pa.cfg.Username)
	}
	if pa.cfg.Password != "secret" {
		t.Fatalf("unexpected password: %q", pa.cfg.Password)
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
	pa, err := parseArgs([]string{"--cf-proxy", "--cf-domain", "example.com"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !pa.cfg.UseCFProxy {
		t.Fatal("expected UseCFProxy to be true")
	}
	if pa.cfg.CFDomain != "example.com" {
		t.Fatalf("unexpected CFDomain: %q", pa.cfg.CFDomain)
	}
	if len(pa.cfg.CFDomains) != 1 || pa.cfg.CFDomains[0] != "example.com" {
		t.Fatalf("expected CFDomains to be [%q], got %v", "example.com", pa.cfg.CFDomains)
	}
	if pa.cfg.UseCFProxyFirst {
		t.Fatal("expected CF proxy first to stay disabled unless requested")
	}
}

func TestParseArgsCFProxyFirst(t *testing.T) {
	pa, err := parseArgs([]string{"--cf-proxy", "--cf-proxy-first", "--cf-domain", "example.com"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !pa.cfg.UseCFProxyFirst {
		t.Fatal("expected UseCFProxyFirst to be true")
	}
}

func TestParseArgsCFBalance(t *testing.T) {
	pa, err := parseArgs([]string{"--cf-proxy", "--cf-balance", "--cf-domain", "a.example.com,b.example.com"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !pa.cfg.UseCFBalance {
		t.Fatal("expected UseCFBalance to be true")
	}
	if got, want := pa.cfg.CFDomains, []string{"a.example.com", "b.example.com"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("unexpected CFDomains: got %v want %v", got, want)
	}
}

func TestParseArgsCFProxyDefaultsDisabled(t *testing.T) {
	pa, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if pa.cfg.UseCFProxy {
		t.Fatal("expected CF proxy to be disabled by default")
	}
	if pa.cfg.CFDomain != "" {
		t.Fatalf("expected empty CFDomain when no --cf-domain given, got %q", pa.cfg.CFDomain)
	}
	if len(pa.cfg.CFDomains) != 0 {
		t.Fatalf("expected empty CFDomains by default, got %v", pa.cfg.CFDomains)
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

func TestParseArgsMTProtoMode(t *testing.T) {
	pa, err := parseArgs([]string{"--mode", "mtproto", "--secret", "deadbeefdeadbeefdeadbeefdeadbeef"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if pa.mode != "mtproto" {
		t.Fatalf("unexpected mode: %q", pa.mode)
	}
	if len(pa.secret) != 16 {
		t.Fatalf("expected 16-byte secret, got %d", len(pa.secret))
	}
}

func TestParseArgsMTProtoRequiresSecret(t *testing.T) {
	if _, err := parseArgs([]string{"--mode", "mtproto"}); err == nil {
		t.Fatal("expected error when --mode mtproto but no --secret")
	}
}

func TestParseArgsMTProtoRejectsBadSecret(t *testing.T) {
	// too short
	if _, err := parseArgs([]string{"--mode", "mtproto", "--secret", "deadbeef"}); err == nil {
		t.Fatal("expected error for short secret")
	}
	// not hex
	if _, err := parseArgs([]string{"--mode", "mtproto", "--secret", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}); err == nil {
		t.Fatal("expected error for non-hex secret")
	}
}

func TestParseArgsRejectsUnknownMode(t *testing.T) {
	if _, err := parseArgs([]string{"--mode", "foobar"}); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestStartupSummaryIncludesCFBalanceForSocks5(t *testing.T) {
	pa, err := parseArgs([]string{
		"--verbose",
		"--cf-proxy",
		"--cf-proxy-first",
		"--cf-balance",
		"--cf-domain", "a.example.com,b.example.com",
		"--username", "alice",
		"--password", "secret",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	got := startupSummary(pa)
	for _, want := range []string{
		"mode=socks5",
		"socks5_auth=userpass",
		"cf_proxy=true",
		"cf_order=first",
		"cf_mode=balance",
		"cf_domains=2",
		"cf_domain_list=a.example.com,b.example.com",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("startup summary missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("did not expect startup summary to expose a password or secret: %s", got)
	}
}

func TestStartupSummaryIncludesMTProtoSecretKind(t *testing.T) {
	pa, err := parseArgs([]string{
		"--mode", "mtproto",
		"--secret", "ee0123456789abcdef0123456789abcdef676f6f676c652e636f6d",
		"--link-ip", "127.0.0.1",
		"--cf-proxy",
		"--cf-balance",
		"--cf-domain", "cf.example.com",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	got := startupSummary(pa)
	for _, want := range []string{
		"mode=mtproto",
		"mtproto_secret=ee-faketls",
		"link_ip=127.0.0.1",
		"cf_mode=balance",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("startup summary missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "ee0123456789abcdef") {
		t.Fatalf("did not expect startup summary to expose the secret value: %s", got)
	}
}

func TestStartupSummaryCanBeLogged(t *testing.T) {
	pa, err := parseArgs([]string{"--cf-proxy", "--cf-domain", "example.com", "--pprof-addr", "127.0.0.1:6060"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	logger.Printf("%s", startupSummary(pa))

	if !strings.Contains(buf.String(), "mode=socks5") {
		t.Fatalf("expected startup summary to be loggable, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "pprof_addr=127.0.0.1:6060") {
		t.Fatalf("expected startup summary to include pprof addr, got %q", buf.String())
	}
}

func TestCurrentBinaryPathFallsBackToArgv0(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"./tg-ws-proxy-test"}

	got := currentBinaryPath()
	if got == "" || got == "<unknown>" {
		t.Fatalf("expected currentBinaryPath to return a non-empty path, got %q", got)
	}
}
