package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/socks5"
)

type dcIPFlags []string

func (f *dcIPFlags) String() string {
	return fmt.Sprintf("%v", []string(*f))
}

func (f *dcIPFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func parseArgs(args []string) (config.Config, error) {
	cfg := config.Default()
	var dcIPs dcIPFlags

	fs := flag.NewFlagSet("tg-ws-proxy", flag.ContinueOnError)
	fs.StringVar(&cfg.Host, "host", cfg.Host, "SOCKS5 listen host")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "SOCKS5 listen port")
	fs.StringVar(&cfg.Username, "username", cfg.Username, "SOCKS5 username auth")
	fs.StringVar(&cfg.Password, "password", cfg.Password, "SOCKS5 password auth")
	fs.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "enable verbose logging")
	fs.IntVar(&cfg.BufferKB, "buf-kb", cfg.BufferKB, "socket buffer size in KB")
	fs.IntVar(&cfg.PoolSize, "pool-size", cfg.PoolSize, "number of pre-opened idle WebSocket connections per active DC bucket")
	fs.DurationVar(&cfg.DialTimeout, "dial-timeout", cfg.DialTimeout, "TCP dial timeout")
	fs.DurationVar(&cfg.InitTimeout, "init-timeout", cfg.InitTimeout, "client MTProto init timeout")
	fs.Var(&dcIPs, "dc-ip", "Target IP for a DC, for example --dc-ip 2:149.154.167.220")
	if err := fs.Parse(args); err != nil {
		return config.Config{}, err
	}

	if len(dcIPs) > 0 {
		parsed, err := config.ParseDCIPList(dcIPs)
		if err != nil {
			return config.Config{}, fmt.Errorf("invalid --dc-ip: %w", err)
		}
		cfg.DCIPs = parsed
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return config.Config{}, fmt.Errorf("--username and --password must be used together")
	}
	return cfg, nil
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	logger := log.New(os.Stdout, "tg-ws-proxy ", log.LstdFlags)
	if cfg.Verbose {
		logger.Printf("starting with verbose logging enabled")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv := socks5.NewServer(cfg, logger)
	if err := srv.Run(ctx); err != nil {
		logger.Fatalf("server stopped with error: %v", err)
	}
}
