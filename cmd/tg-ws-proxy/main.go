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
	"tg-ws-proxy/internal/mtprotoproxy"
	"tg-ws-proxy/internal/socks5"
	"tg-ws-proxy/internal/wsbridge"
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
	fs.BoolVar(&cfg.SOCKS5Enabled, "socks5", cfg.SOCKS5Enabled, "enable SOCKS5 proxy")
	fs.StringVar(&cfg.Username, "username", cfg.Username, "SOCKS5 username auth")
	fs.StringVar(&cfg.Password, "password", cfg.Password, "SOCKS5 password auth")
	fs.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "enable verbose logging")
	fs.IntVar(&cfg.BufferKB, "buf-kb", cfg.BufferKB, "socket buffer size in KB")
	fs.IntVar(&cfg.PoolSize, "pool-size", cfg.PoolSize, "number of pre-opened idle WebSocket connections per active DC bucket")
	fs.DurationVar(&cfg.DialTimeout, "dial-timeout", cfg.DialTimeout, "TCP dial timeout")
	fs.DurationVar(&cfg.InitTimeout, "init-timeout", cfg.InitTimeout, "client MTProto init timeout")
	fs.Var(&dcIPs, "dc-ip", "Target IP for a DC, for example --dc-ip 2:149.154.167.220")
	fs.BoolVar(&cfg.MTProtoEnabled, "mtproto", cfg.MTProtoEnabled, "enable MTProto proxy (fake-TLS ee mode)")
	fs.StringVar(&cfg.MTProtoHost, "mtproto-host", cfg.MTProtoHost, "MTProto proxy listen host")
	fs.IntVar(&cfg.MTProtoPort, "mtproto-port", cfg.MTProtoPort, "MTProto proxy listen port")
	fs.StringVar(&cfg.MTProtoSecret, "mtproto-secret", cfg.MTProtoSecret, "MTProto proxy secret (ee + 32 hex chars); auto-generated if empty")
	fs.StringVar(&cfg.MTProtoDomain, "mtproto-domain", cfg.MTProtoDomain, "MTProto fake-TLS domain used in tg://proxy link")
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
	if !cfg.SOCKS5Enabled && !cfg.MTProtoEnabled {
		return config.Config{}, fmt.Errorf("at least one proxy mode must be enabled")
	}
	if cfg.MTProtoEnabled {
		cfg.MTProtoDomain = mtprotoproxy.NormalizeFakeTLSDomain(cfg.MTProtoDomain)
		if cfg.MTProtoPort == 0 {
			return config.Config{}, fmt.Errorf("--mtproto-port is required when --mtproto is enabled")
		}
		if cfg.MTProtoSecret == "" {
			sec, err := mtprotoproxy.GenerateSecret()
			if err != nil {
				return config.Config{}, fmt.Errorf("generate mtproto secret: %w", err)
			}
			cfg.MTProtoSecret = sec
		}
		if _, err := mtprotoproxy.ParseSecret(cfg.MTProtoSecret); err != nil {
			return config.Config{}, fmt.Errorf("invalid --mtproto-secret: %w", err)
		}
	}
	return cfg, nil
}

func main() {
	if handled, err := handleUtilityMode(os.Args[1:]); handled {
		if err != nil {
			log.Fatal(err)
		}
		return
	}

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

	errCh := make(chan error, 2)

	if cfg.SOCKS5Enabled {
		srv := socks5.NewServer(cfg, logger)
		go func() {
			errCh <- srv.Run(ctx)
		}()
	}

	if cfg.MTProtoEnabled {
		secret, _ := mtprotoproxy.ParseSecret(cfg.MTProtoSecret)
		pool := wsbridge.NewPool(cfg)
		mtSrv := mtprotoproxy.NewServer(cfg, logger, pool, secret)

		linkHost := cfg.MTProtoHost
		if linkHost == "" {
			linkHost = cfg.Host
		}
		link := mtprotoproxy.FormatLink(linkHost, cfg.MTProtoPort, cfg.MTProtoSecret, cfg.MTProtoDomain)
		logger.Printf("mtproto proxy link: %s", link)

		go func() {
			errCh <- mtSrv.Run(ctx)
		}()
	}

	if err := <-errCh; err != nil {
		logger.Fatalf("server stopped with error: %v", err)
	}
}

func handleUtilityMode(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}

	switch args[0] {
	case "qr":
		if len(args) != 2 {
			return true, fmt.Errorf("usage: %s qr <text>", os.Args[0])
		}
		qr, err := mtprotoproxy.RenderTerminalQR(args[1])
		if err != nil {
			return true, err
		}
		_, err = fmt.Fprint(os.Stdout, qr)
		return true, err
	default:
		return false, nil
	}
}
