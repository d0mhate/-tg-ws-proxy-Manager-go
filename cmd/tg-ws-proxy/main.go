package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"unicode"

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
	var cfDomainFlag string
	fs.BoolVar(&cfg.UseCFProxy, "cf-proxy", cfg.UseCFProxy, "enable Cloudflare proxy mode for websocket routing")
	fs.BoolVar(&cfg.UseCFProxyFirst, "cf-proxy-first", cfg.UseCFProxyFirst, "try Cloudflare websocket routing before direct Telegram websocket routing")
	fs.StringVar(&cfDomainFlag, "cf-domain", "", "Cloudflare domain(s) for websocket routing, e.g. example.com or domain1.com,domain2.com (required for CF proxy mode)")
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
	if cfDomainFlag != "" {
		parts := strings.Split(cfDomainFlag, ",")
		domains := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if !isValidDomain(p) {
				return config.Config{}, fmt.Errorf("invalid --cf-domain value: %q", p)
			}
			domains = append(domains, p)
		}
		if len(domains) == 0 {
			return config.Config{}, fmt.Errorf("--cf-domain: no valid domains provided")
		}
		cfg.CFDomain = domains[0]
		cfg.CFDomains = domains
	}
	return cfg, nil
}

func isValidDomain(domain string) bool {
	domain = strings.TrimSpace(domain)
	if domain == "" || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}

	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
				continue
			}
			return false
		}
	}

	last := labels[len(labels)-1]
	if len(last) < 2 {
		return false
	}
	for _, r := range last {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
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
