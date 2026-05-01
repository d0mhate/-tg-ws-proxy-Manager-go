//go:build pprof

package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestParseArgsPprofAddr(t *testing.T) {
	pa, err := parseArgs([]string{"--pprof-addr", "127.0.0.1:6060"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if pa.cfg.PprofAddr != "127.0.0.1:6060" {
		t.Fatalf("unexpected pprof addr: %q", pa.cfg.PprofAddr)
	}
}

func TestStartupSummaryIncludesPprofAddr(t *testing.T) {
	pa, err := parseArgs([]string{"--cf-proxy", "--cf-domain", "example.com", "--pprof-addr", "127.0.0.1:6060"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	logger.Printf("%s", startupSummary(pa))

	if !strings.Contains(buf.String(), "pprof_addr=127.0.0.1:6060") {
		t.Fatalf("expected startup summary to include pprof addr, got %q", buf.String())
	}
}
