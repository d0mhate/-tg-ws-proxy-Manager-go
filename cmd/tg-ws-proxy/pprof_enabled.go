//go:build pprof

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"tg-ws-proxy/internal/config"
)

func registerPprofFlag(fs *flag.FlagSet, cfg *config.Config) {
	fs.StringVar(&cfg.PprofAddr, "pprof-addr", cfg.PprofAddr, "enable pprof HTTP server on this address, for example 127.0.0.1:6060")
}

func startPprofServer(ctx context.Context, addr string, logger *log.Logger) error {
	if addr == "" {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen pprof on %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Printf("pprof shutdown error: %v", err)
		}
	}()

	go func() {
		logger.Printf("pprof listening on http://%s/debug/pprof/", addr)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("pprof server stopped with error: %v", err)
		}
	}()

	return nil
}
