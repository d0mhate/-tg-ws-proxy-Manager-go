//go:build !pprof

package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"tg-ws-proxy/internal/config"
)

func registerPprofFlag(_ *flag.FlagSet, _ *config.Config) {}

func startPprofServer(_ context.Context, addr string, _ *log.Logger) error {
	if addr == "" {
		return nil
	}
	return fmt.Errorf("pprof support is not included in this build")
}
