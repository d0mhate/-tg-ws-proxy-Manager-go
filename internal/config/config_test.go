package config

import "testing"

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
	if cfg.CFDomain != DefaultCFDomain {
		t.Fatalf("expected CFDomain %q, got %q", DefaultCFDomain, cfg.CFDomain)
	}
}

func TestDefaultIncludesCommonWSDCs(t *testing.T) {
	cfg := Default()

	if cfg.PoolSize != 1 {
		t.Fatalf("unexpected default pool size: %d", cfg.PoolSize)
	}

	want := map[int]string{
		1: "149.154.175.205",
		2: "149.154.167.220",
		4: "149.154.167.220",
		5: "91.108.56.100",
	}

	for dc, ip := range want {
		if got := cfg.DCIPs[dc]; got != ip {
			t.Fatalf("unexpected default dc %d ip: got %q want %q", dc, got, ip)
		}
	}
}
