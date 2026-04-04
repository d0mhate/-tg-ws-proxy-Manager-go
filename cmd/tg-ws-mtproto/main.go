package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/alecthomas/units"
	"github.com/gotd/mtg/antireplay"
	mtgconfig "github.com/gotd/mtg/config"
	"github.com/gotd/mtg/faketls"
	"github.com/gotd/mtg/hub"
	"github.com/gotd/mtg/obfuscated2"
	"github.com/gotd/mtg/proxy"
	"github.com/gotd/mtg/stats"
	"github.com/gotd/mtg/telegram"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const defaultFakeTLSDomain = "www.google.com"

type options struct {
	Host       string
	Port       int
	PublicHost string
	Secret     string
	Domain     string
	Verbose    bool
	Debug      bool
}

func normalizeDomain(domain string) string {
	trimmed := strings.TrimSpace(domain)
	if trimmed == "" {
		return defaultFakeTLSDomain
	}
	return trimmed
}

func generateSecret(domain string) (string, error) {
	raw := make([]byte, mtgconfig.SimpleSecretLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return "ee" + hex.EncodeToString(raw) + hex.EncodeToString([]byte(normalizeDomain(domain))), nil
}

func expandSecret(secret, domain string) (string, error) {
	normalizedDomain := normalizeDomain(domain)
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return generateSecret(normalizedDomain)
	}

	if strings.HasPrefix(trimmed, "ee") {
		switch len(trimmed) {
		case 2 + mtgconfig.SimpleSecretLength*2:
			return trimmed + hex.EncodeToString([]byte(normalizedDomain)), nil
		default:
			if len(trimmed) > 2+mtgconfig.SimpleSecretLength*2 {
				return trimmed, nil
			}
		}
	}

	return "", fmt.Errorf("expected fake-tls secret in ee... format")
}

func parseArgs(args []string) (options, error) {
	opts := options{
		Host:   "0.0.0.0",
		Port:   8443,
		Domain: defaultFakeTLSDomain,
	}

	fs := flag.NewFlagSet("tg-ws-mtproto", flag.ContinueOnError)
	fs.StringVar(&opts.Host, "host", opts.Host, "listen host")
	fs.IntVar(&opts.Port, "port", opts.Port, "listen port")
	fs.StringVar(&opts.PublicHost, "public-host", opts.PublicHost, "public IPv4 used in tg://proxy links")
	fs.StringVar(&opts.Secret, "secret", opts.Secret, "MTProto fake-TLS secret (bare ee+32hex or expanded ee+32hex+domainhex)")
	fs.StringVar(&opts.Domain, "domain", opts.Domain, "fake-TLS cloak domain used when expanding bare ee secret")
	fs.BoolVar(&opts.Verbose, "verbose", opts.Verbose, "enable verbose logging")
	fs.BoolVar(&opts.Debug, "debug", opts.Debug, "enable debug logging")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}

	expanded, err := expandSecret(opts.Secret, opts.Domain)
	if err != nil {
		return options{}, err
	}
	opts.Secret = expanded
	opts.Domain = normalizeDomain(opts.Domain)

	if opts.PublicHost == "" {
		if opts.Host == "" || opts.Host == "0.0.0.0" {
			opts.PublicHost = "127.0.0.1"
		} else {
			opts.PublicHost = opts.Host
		}
	}

	return opts, nil
}

func handleUtilityMode(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}

	switch args[0] {
	case "generate-secret":
		domain := defaultFakeTLSDomain
		if len(args) > 1 {
			domain = args[1]
		}
		secret, err := generateSecret(domain)
		if err != nil {
			return true, err
		}
		_, err = fmt.Fprintln(os.Stdout, secret)
		return true, err
	default:
		return false, nil
	}
}

func tcpAddr(host string, port int) (*net.TCPAddr, error) {
	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	return addr, nil
}

func setupLogger(debug, verbose bool) {
	atom := zap.NewAtomicLevel()
	switch {
	case debug:
		atom.SetLevel(zapcore.DebugLevel)
	case verbose:
		atom.SetLevel(zapcore.InfoLevel)
	default:
		atom.SetLevel(zapcore.ErrorLevel)
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = ""
	logger := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		atom,
	))
	zap.ReplaceGlobals(logger)
}

func run(opts options) error {
	bindAddr, err := tcpAddr(opts.Host, opts.Port)
	if err != nil {
		return fmt.Errorf("resolve bind address: %w", err)
	}

	publicIPv4, err := tcpAddr(opts.PublicHost, opts.Port)
	if err != nil {
		return fmt.Errorf("resolve public address: %w", err)
	}

	publicIPv6 := &net.TCPAddr{IP: net.IPv6loopback, Port: opts.Port}
	statsBind := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}

	secret, err := hex.DecodeString(opts.Secret)
	if err != nil {
		return fmt.Errorf("decode secret: %w", err)
	}

	if err := mtgconfig.Init(
		mtgconfig.Opt{Option: mtgconfig.OptionTypeDebug, Value: opts.Debug},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeVerbose, Value: opts.Verbose},
		mtgconfig.Opt{Option: mtgconfig.OptionTypePreferIP, Value: "ipv4"},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeBind, Value: bindAddr},
		mtgconfig.Opt{Option: mtgconfig.OptionTypePublicIPv4, Value: publicIPv4},
		mtgconfig.Opt{Option: mtgconfig.OptionTypePublicIPv6, Value: publicIPv6},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeStatsBind, Value: statsBind},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeStatsNamespace, Value: "mtg"},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeWriteBufferSize, Value: flagBytes("32KB")},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeReadBufferSize, Value: flagBytes("32KB")},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeCloakPort, Value: uint16(443)},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeAntiReplayMaxSize, Value: flagBytes("128MB")},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeMultiplexPerConnection, Value: uint(50)},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeNTPServers, Value: []string{"0.pool.ntp.org", "1.pool.ntp.org", "2.pool.ntp.org", "3.pool.ntp.org"}},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeTestDC, Value: false},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeSecret, Value: secret},
		mtgconfig.Opt{Option: mtgconfig.OptionTypeAdtag, Value: []byte(nil)},
	); err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	setupLogger(opts.Debug, opts.Verbose)
	defer zap.L().Sync() // nolint: errcheck

	zap.S().Infow("MTProto proxy link", "link", mtgconfig.GetURLs().IPv4.TG)

	if err := stats.Init(ctx); err != nil {
		return err
	}

	antireplay.Init()
	telegram.Init()
	hub.Init(ctx)

	listener, err := net.Listen("tcp", bindAddr.String())
	if err != nil {
		return err
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	app := &proxy.Proxy{
		Logger:              zap.S().Named("proxy"),
		Context:             ctx,
		ClientProtocolMaker: obfuscated2.MakeClientProtocol,
	}
	if mtgconfig.C.SecretMode == mtgconfig.SecretModeTLS {
		app.ClientProtocolMaker = faketls.MakeClientProtocol
	}

	app.Serve(listener)
	return nil
}

func flagBytes(v string) interface{} {
	value, err := units.ParseBase2Bytes(v)
	if err != nil {
		panic(err)
	}
	return value
}

func main() {
	if handled, err := handleUtilityMode(os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
