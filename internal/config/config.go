package config

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"tg-ws-proxy/internal/telegram"
)

// upstream proxy for fallback when websocket to tg fails.
// secret is hex from tg://proxy:
// - 32 hex chars (16 bytes): plain
// - 34 hex chars (17 bytes, 0xdd prefix): padded intermediate
// - 34+ hex chars (17+ bytes, 0xee prefix): faketls, remaining bytes are hostname
type UpstreamProxy struct {
	Host   string
	Port   int
	Secret string
}

type Config struct {
	Host            string
	Port            int
	PprofAddr       string
	Username        string
	Password        string
	Verbose         bool
	BufferKB        int
	PoolSize        int
	PoolMaxAge      time.Duration
	PoolRefillDelay time.Duration
	DialTimeout     time.Duration
	InitTimeout     time.Duration
	ConnectWSPath   string
	DCIPs           map[int]string
	UseCFProxy      bool
	UseCFProxyFirst bool
	UseCFBalance    bool
	CFDomainSource  string
	CFDomain        string
	CFDomains       []string
	UpstreamProxies []UpstreamProxy
}

const (
	CFDomainSourceBuiltin = "built-in"
	CFDomainSourceCustom  = "custom"
)

const builtinCFDomainsObf = `\160\143\154\145\141\144\056\143\157\056\165\153\054\157\146\146\163\150\157\162\056\143\157\056\165\153\054\143\141\153\145\151\163\141\154\151\145\056\143\157\056\165\153\054\156\157\163\153\157\155\156\141\144\172\157\162\056\143\157\056\165\153\054\154\157\166\145\164\162\165\145\056\143\157\056\165\153\054\163\157\162\157\153\144\166\141\056\143\157\056\165\153\054\160\171\141\164\144\145\163\171\141\164\144\166\141\056\143\157\056\165\153\054\153\141\162\164\157\163\150\153\141\056\143\157\056\165\153`

func Default() Config {
	return Config{
		Host:            "127.0.0.1",
		Port:            1080,
		PprofAddr:       "",
		Verbose:         false,
		BufferKB:        256,
		PoolSize:        4,
		PoolMaxAge:      55 * time.Second,
		PoolRefillDelay: 250 * time.Millisecond,
		DialTimeout:     10 * time.Second,
		InitTimeout:     15 * time.Second,
		ConnectWSPath:   "/apiws",
		CFDomains:       nil,
		DCIPs: map[int]string{
			2: telegram.IPv4DC2,
			4: telegram.IPv4DC2,
		},
	}
}

func BuiltinCFDomains() []string {
	decoded := make([]byte, 0, len(builtinCFDomainsObf))
	for i := 0; i < len(builtinCFDomainsObf); {
		if builtinCFDomainsObf[i] != '\\' || i+3 >= len(builtinCFDomainsObf) {
			return nil
		}
		v, err := strconv.ParseUint(builtinCFDomainsObf[i+1:i+4], 8, 8)
		if err != nil {
			return nil
		}
		decoded = append(decoded, byte(v))
		i += 4
	}
	return strings.Split(string(decoded), ",")
}

func ParseDCIPList(values []string) (map[int]string, error) {
	out := make(map[int]string, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("expected DC:IP, got %q", value)
		}

		dc, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid dc in %q", value)
		}
		if ip := net.ParseIP(parts[1]); ip == nil || ip.To4() == nil {
			return nil, fmt.Errorf("invalid IPv4 in %q", value)
		}

		out[dc] = parts[1]
	}
	return out, nil
}

func ParseDCIPString(value string) (map[int]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return map[int]string{}, nil
	}

	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			return nil, fmt.Errorf("empty DC:IP entry")
		}
		values = append(values, trimmed)
	}

	return ParseDCIPList(values)
}

func FormatDCIPMap(dcIPs map[int]string) string {
	if len(dcIPs) == 0 {
		return ""
	}

	keys := make([]int, 0, len(dcIPs))
	for dc := range dcIPs {
		keys = append(keys, dc)
	}
	sort.Ints(keys)

	parts := make([]string, 0, len(keys))
	for _, dc := range keys {
		parts = append(parts, fmt.Sprintf("%d:%s", dc, dcIPs[dc]))
	}
	return strings.Join(parts, ", ")
}

func CFDomainSource() string {
	switch strings.TrimSpace(os.Getenv("TG_WS_PROXY_CF_DOMAIN_SOURCE")) {
	case "builtin", "built-in":
		return CFDomainSourceBuiltin
	case "custom":
		return CFDomainSourceCustom
	default:
		return ""
	}
}

func BuiltinCFDomainsInUse() bool {
	return CFDomainSource() == CFDomainSourceBuiltin
}

func MaskCFDomainForLog(domain string) string {
	if BuiltinCFDomainsInUse() {
		return "built-in"
	}
	return domain
}

func MaskCFDomainsForLog(domains []string) string {
	if BuiltinCFDomainsInUse() {
		return "built-in"
	}
	return strings.Join(domains, ",")
}
