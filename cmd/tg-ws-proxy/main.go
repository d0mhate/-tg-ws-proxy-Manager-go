package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"rsc.io/qr"
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

type mtproxyFlags []string

func (f *mtproxyFlags) String() string { return fmt.Sprintf("%v", []string(*f)) }
func (f *mtproxyFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}

// parse HOST:PORT:SECRET into an UpstreamProxy.
// splitn is enough here because host is expected to be ipv4 or hostname.
func parseMtproxyFlag(s string) (config.UpstreamProxy, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return config.UpstreamProxy{}, fmt.Errorf("expected HOST:PORT:SECRET, got %q", s)
	}
	host, portStr, secret := parts[0], parts[1], parts[2]
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return config.UpstreamProxy{}, fmt.Errorf("invalid port %q in --mtproto-proxy", portStr)
	}
	if _, err := hex.DecodeString(secret); err != nil {
		return config.UpstreamProxy{}, fmt.Errorf("invalid hex secret in --mtproto-proxy: %w", err)
	}
	return config.UpstreamProxy{Host: host, Port: port, Secret: secret}, nil
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
	var mtProxies mtproxyFlags

	fs := flag.NewFlagSet("tg-ws-proxy", flag.ContinueOnError)
	fs.StringVar(&cfg.Host, "host", cfg.Host, "listen host")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "listen port")
	fs.StringVar(&cfg.PprofAddr, "pprof-addr", cfg.PprofAddr, "enable pprof HTTP server on this address, for example 127.0.0.1:6060")
	fs.StringVar(&cfg.Username, "username", cfg.Username, "SOCKS5 username auth")
	fs.StringVar(&cfg.Password, "password", cfg.Password, "SOCKS5 password auth")
	fs.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "enable verbose logging")
	fs.IntVar(&cfg.BufferKB, "buf-kb", cfg.BufferKB, "socket buffer size in KB")
	fs.IntVar(&cfg.PoolSize, "pool-size", cfg.PoolSize, "number of pre-opened idle WebSocket connections per active DC bucket")
	fs.DurationVar(&cfg.PoolMaxAge, "pool-max-age", cfg.PoolMaxAge, "maximum age of an idle pooled WebSocket connection before it is discarded")
	fs.DurationVar(&cfg.PoolRefillDelay, "pool-refill-delay", cfg.PoolRefillDelay, "delay between opening pooled WebSocket connections while refilling a bucket")
	fs.DurationVar(&cfg.DialTimeout, "dial-timeout", cfg.DialTimeout, "TCP dial timeout")
	fs.DurationVar(&cfg.InitTimeout, "init-timeout", cfg.InitTimeout, "client MTProto init timeout")
	fs.Var(&dcIPs, "dc-ip", "Target IP for a DC, for example --dc-ip 2:149.154.167.220")
	var cfDomainFlag string
	fs.BoolVar(&cfg.UseCFProxy, "cf-proxy", cfg.UseCFProxy, "enable Cloudflare proxy mode for websocket routing")
	fs.BoolVar(&cfg.UseCFProxyFirst, "cf-proxy-first", cfg.UseCFProxyFirst, "try Cloudflare websocket routing before direct Telegram websocket routing")
	fs.BoolVar(&cfg.UseCFBalance, "cf-balance", cfg.UseCFBalance, "round-robin across multiple Cloudflare websocket domains while preserving fallback order")
	fs.StringVar(&cfDomainFlag, "cf-domain", "", "Cloudflare domain(s) for websocket routing, e.g. example.com or domain1.com,domain2.com (required for CF proxy mode)")

	var mode string
	var secretHex string
	var linkIP string
	fs.StringVar(&mode, "mode", "socks5", "proxy mode: socks5 or mtproto")
	fs.StringVar(&secretHex, "secret", "", "MTProto proxy secret (32 hex chars, required for --mode mtproto)")
	fs.StringVar(&linkIP, "link-ip", "", "public IP to include in the tg:// proxy link (mtproto mode)")
	fs.Var(&mtProxies, "mtproto-proxy", "upstream MTProto proxy HOST:PORT:SECRET (may be repeated)")

	if err := fs.Parse(args); err != nil {
		return parsedArgs{}, err
	}

	for _, raw := range mtProxies {
		up, err := parseMtproxyFlag(raw)
		if err != nil {
			return parsedArgs{}, fmt.Errorf("invalid --mtproto-proxy: %w", err)
		}
		cfg.UpstreamProxies = append(cfg.UpstreamProxies, up)
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
	if cfg.PoolMaxAge < 0 {
		return parsedArgs{}, fmt.Errorf("--pool-max-age must be >= 0")
	}
	if cfg.PoolRefillDelay < 0 {
		return parsedArgs{}, fmt.Errorf("--pool-refill-delay must be >= 0")
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
		if err != nil {
			return parsedArgs{}, fmt.Errorf("--secret: invalid hex: %w", err)
		}
		switch {
		case len(decoded) == 16:
			// Plain 16-byte secret.
			secret = decoded
		case len(decoded) >= 17 && (decoded[0] == 0xdd || decoded[0] == 0xee):
			// dd-prefix (padded intermediate) or ee-prefix (FakeTLS).
			// For ee, bytes[17:] are the UTF-8 hostname.
			secret = decoded
		default:
			return parsedArgs{}, fmt.Errorf(
				"--secret must be 32 hex chars (plain), or 34+ hex chars starting with dd or ee, got %d hex chars",
				len(secretHex),
			)
		}
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

func mtSecretKind(secret []byte) string {
	switch {
	case len(secret) >= 17 && secret[0] == 0xee:
		return "ee-faketls"
	case len(secret) >= 17 && secret[0] == 0xdd:
		return "dd-intermediate"
	case len(secret) == 16:
		return "hex"
	default:
		return "unknown"
	}
}

func startupSummary(pa parsedArgs) string {
	var b bytes.Buffer

	fmt.Fprintf(&b,
		"startup mode=%s listen=%s:%d verbose=%t buf_kb=%d pool_size=%d pool_max_age=%s pool_refill_delay=%s dial_timeout=%s init_timeout=%s",
		pa.mode, pa.cfg.Host, pa.cfg.Port, pa.cfg.Verbose, pa.cfg.BufferKB, pa.cfg.PoolSize,
		pa.cfg.PoolMaxAge, pa.cfg.PoolRefillDelay, pa.cfg.DialTimeout, pa.cfg.InitTimeout,
	)
	if pa.cfg.PprofAddr != "" {
		fmt.Fprintf(&b, " pprof_addr=%s", pa.cfg.PprofAddr)
	}

	if pa.mode == "socks5" {
		authMode := "off"
		if pa.cfg.Username != "" {
			authMode = "userpass"
		}
		fmt.Fprintf(&b, " socks5_auth=%s", authMode)
	}

	if pa.mode == "mtproto" {
		fmt.Fprintf(&b, " mtproto_secret=%s", mtSecretKind(pa.secret))
		if pa.linkIP != "" {
			fmt.Fprintf(&b, " link_ip=%s", pa.linkIP)
		}
	}

	cfOrder := "fallback"
	if pa.cfg.UseCFProxyFirst {
		cfOrder = "first"
	}
	cfMode := "ordered"
	if pa.cfg.UseCFBalance {
		cfMode = "balance"
	}
	fmt.Fprintf(&b,
		" cf_proxy=%t cf_order=%s cf_mode=%s cf_domains=%d",
		pa.cfg.UseCFProxy, cfOrder, cfMode, len(pa.cfg.CFDomains),
	)
	if len(pa.cfg.CFDomains) > 0 {
		fmt.Fprintf(&b, " cf_domain_list=%s", strings.Join(pa.cfg.CFDomains, ","))
	}

	fmt.Fprintf(&b, " dc_overrides=%d upstream_mtproto=%d", len(pa.cfg.DCIPs), len(pa.cfg.UpstreamProxies))
	return b.String()
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

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "qr" {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: tg-ws-proxy qr <link>")
			os.Exit(1)
		}
		code, err := qr.Encode(os.Args[2], qr.L)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		size := code.Size
		blk := func(x, y int) bool {
			return x >= 0 && x < size && y >= 0 && y < size && code.Black(x, y)
		}
		for y := -2; y < size+2; y += 2 {
			for x := -2; x < size+2; x++ {
				t, b := blk(x, y), blk(x, y+1)
				switch {
				case t && b:
					fmt.Fprint(os.Stdout, "█")
				case t:
					fmt.Fprint(os.Stdout, "▀")
				case b:
					fmt.Fprint(os.Stdout, "▄")
				default:
					fmt.Fprint(os.Stdout, " ")
				}
			}
			fmt.Fprintln(os.Stdout)
		}
		return
	}

	pa, err := parseArgs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	logger := log.New(os.Stdout, "tg-ws-proxy ", log.LstdFlags)
	logger.Printf("%s", startupSummary(pa))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := startPprofServer(ctx, pa.cfg.PprofAddr, logger); err != nil {
		logger.Fatalf("pprof init failed: %v", err)
	}

	if pa.mode == "mtproto" {
		if pa.linkIP != "" {
			var secretDisplay string
			if len(pa.secret) == 16 {
				// Plain secret - advertise with dd prefix (padded intermediate mode).
				secretDisplay = "dd" + hex.EncodeToString(pa.secret)
			} else {
				// dd or ee prefix already present - use as-is.
				secretDisplay = hex.EncodeToString(pa.secret)
			}
			logger.Printf("tg://proxy?server=%s&port=%d&secret=%s",
				pa.linkIP, pa.cfg.Port, secretDisplay)
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
