package config

import (
	"testing"
	"time"
)

func TestParseDCIPList(t *testing.T) {
	got, err := ParseDCIPList([]string{"2:149.154.167.220", "4:149.154.167.220"})
	if err != nil {
		t.Fatalf("ParseDCIPList returned error: %v", err)
	}

	if got[2] != "149.154.167.220" {
		t.Fatalf("unexpected dc 2 ip: %q", got[2])
	}
	if got[4] != "149.154.167.220" {
		t.Fatalf("unexpected dc 4 ip: %q", got[4])
	}
}

func TestParseDCIPListRejectsInvalidInput(t *testing.T) {
	cases := [][]string{
		{"2"},
		{"x:149.154.167.220"},
		{"2:not-an-ip"},
	}

	for _, tc := range cases {
		if _, err := ParseDCIPList(tc); err == nil {
			t.Fatalf("expected error for %v", tc)
		}
	}
}

func TestParseDCIPString(t *testing.T) {
	got, err := ParseDCIPString("2:149.154.167.220, 4:149.154.167.220")
	if err != nil {
		t.Fatalf("ParseDCIPString returned error: %v", err)
	}

	if got[2] != "149.154.167.220" {
		t.Fatalf("unexpected dc 2 ip: %q", got[2])
	}
	if got[4] != "149.154.167.220" {
		t.Fatalf("unexpected dc 4 ip: %q", got[4])
	}
}

func TestParseDCIPStringRejectsEmptyEntry(t *testing.T) {
	if _, err := ParseDCIPString("2:149.154.167.220, "); err == nil {
		t.Fatal("expected ParseDCIPString to reject empty entry")
	}
}

func TestFormatDCIPMap(t *testing.T) {
	got := FormatDCIPMap(map[int]string{
		4: "149.154.167.220",
		2: "149.154.167.220",
		1: "149.154.175.205",
	})
	if got != "1:149.154.175.205, 2:149.154.167.220, 4:149.154.167.220" {
		t.Fatalf("unexpected formatted dc map: %q", got)
	}
}

func TestDefaultCFProxyDisabled(t *testing.T) {
	cfg := Default()
	if cfg.UseCFProxy {
		t.Fatal("expected UseCFProxy to default to false")
	}
	if cfg.UseCFProxyFirst {
		t.Fatal("expected UseCFProxyFirst to default to false")
	}
	if len(cfg.CFDomains) != 0 {
		t.Fatalf("expected empty CFDomains by default, got %v", cfg.CFDomains)
	}
}

func TestDefaultIncludesCommonWSDCs(t *testing.T) {
	cfg := Default()

	if cfg.PoolSize != 4 {
		t.Fatalf("unexpected default pool size: %d", cfg.PoolSize)
	}
	if cfg.PoolMaxAge != 55*time.Second {
		t.Fatalf("unexpected default pool max age: %s", cfg.PoolMaxAge)
	}
	if cfg.PoolRefillDelay != 250*time.Millisecond {
		t.Fatalf("unexpected default pool refill delay: %s", cfg.PoolRefillDelay)
	}

	want := map[int]string{
		2: "149.154.167.220",
		4: "149.154.167.220",
	}

	for dc, ip := range want {
		if got := cfg.DCIPs[dc]; got != ip {
			t.Fatalf("unexpected default dc %d ip: got %q want %q", dc, got, ip)
		}
	}

	for _, dc := range []int{1, 3, 5} {
		if _, ok := cfg.DCIPs[dc]; ok {
			t.Fatalf("did not expect default direct dc %d override", dc)
		}
	}
}
