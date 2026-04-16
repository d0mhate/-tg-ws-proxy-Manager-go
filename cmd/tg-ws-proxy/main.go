package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"unicode"

	"tg-ws-proxy/internal/config"
	"tg-ws-proxy/internal/mtpserver"
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

type parsedArgs struct {
	cfg    config.Config
	mode   string
	secret []byte
	linkIP string
}

func parseArgs(args []string) (parsedArgs, error) {
	cfg := config.Default()
	var dcIPs dcIPFlags

	fs := flag.NewFlagSet("tg-ws-proxy", flag.ContinueOnError)
	fs.StringVar(&cfg.Host, "host", cfg.Host, "listen host")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "listen port")
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

	var mode string
	var secretHex string
	var linkIP string
	fs.StringVar(&mode, "mode", "socks5", "proxy mode: socks5 or mtproto")
	fs.StringVar(&secretHex, "secret", "", "MTProto proxy secret (32 hex chars, required for --mode mtproto)")
	fs.StringVar(&linkIP, "link-ip", "", "public IP to include in the tg:// proxy link (mtproto mode)")

	if err := fs.Parse(args); err != nil {
		return parsedArgs{}, err
	}

	if len(dcIPs) > 0 {
		parsed, err := config.ParseDCIPList(dcIPs)
		if err != nil {
			return parsedArgs{}, fmt.Errorf("invalid --dc-ip: %w", err)
		}
		cfg.DCIPs = parsed
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return parsedArgs{}, fmt.Errorf("--username and --password must be used together")
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
				return parsedArgs{}, fmt.Errorf("invalid --cf-domain value: %q", p)
			}
			domains = append(domains, p)
		}
		if len(domains) == 0 {
			return parsedArgs{}, fmt.Errorf("--cf-domain: no valid domains provided")
		}
		cfg.CFDomain = domains[0]
		cfg.CFDomains = domains
	}

	if mode != "socks5" && mode != "mtproto" {
		return parsedArgs{}, fmt.Errorf("--mode must be socks5 or mtproto, got %q", mode)
	}

	var secret []byte
	if mode == "mtproto" {
		if secretHex == "" {
			return parsedArgs{}, fmt.Errorf("--secret is required for --mode mtproto")
		}
		decoded, err := hex.DecodeString(secretHex)
		if err != nil || len(decoded) != 16 {
			return parsedArgs{}, fmt.Errorf("--secret must be exactly 32 hex characters (16 bytes), got %q", secretHex)
		}
		secret = decoded
	}

	return parsedArgs{cfg: cfg, mode: mode, secret: secret, linkIP: linkIP}, nil
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
	pa, err := parseArgs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	logger := log.New(os.Stdout, "tg-ws-proxy ", log.LstdFlags)
	if pa.cfg.Verbose {
		logger.Printf("starting with verbose logging enabled")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if pa.mode == "mtproto" {
		if pa.linkIP != "" {
			logger.Printf("tg://proxy?server=%s&port=%d&secret=dd%s",
				pa.linkIP, pa.cfg.Port, hex.EncodeToString(pa.secret))
		}
		srv := mtpserver.NewMTServer(pa.cfg, pa.secret, logger)
		if err := srv.Run(ctx); err != nil {
			logger.Fatalf("mtproto server stopped with error: %v", err)
		}
		return
	}

	srv := socks5.NewServer(pa.cfg, logger)
	if err := srv.Run(ctx); err != nil {
		logger.Fatalf("server stopped with error: %v", err)
	}
}
