//go:build !pprof

package main

import "testing"

func TestParseArgsRejectsPprofAddrInDefaultBuild(t *testing.T) {
	if _, err := parseArgs([]string{"--pprof-addr", "127.0.0.1:6060"}); err == nil {
		t.Fatal("expected default build to reject --pprof-addr")
	}
}
