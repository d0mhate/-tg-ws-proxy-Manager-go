package config

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host            string
	Port            int
	Username        string
	Password        string
	Verbose         bool
	BufferKB        int
	PoolSize        int
	DialTimeout     time.Duration
	InitTimeout     time.Duration
	ConnectWSPath   string
	DCIPs           map[int]string
	UseCFProxy      bool
	UseCFProxyFirst bool
	CFDomain        string
}

const defaultCFDomainEnc = "virkgj.iu.aq"

var DefaultCFDomain = decodeDomain(defaultCFDomainEnc)

func decodeDomain(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c = (c-'a'+20)%26 + 'a'
		}
		b[i] = c
	}
	return string(b)
}

func Default() Config {
	return Config{
		Host:          "127.0.0.1",
		Port:          1080,
		Verbose:       false,
		BufferKB:      256,
		PoolSize:      1,
		DialTimeout:   10 * time.Second,
		InitTimeout:   15 * time.Second,
		ConnectWSPath: "/apiws",
		CFDomain:      DefaultCFDomain,
		DCIPs: map[int]string{
			1: "149.154.175.205",
			2: "149.154.167.220",
			4: "149.154.167.220",
			5: "91.108.56.100",
		},
	}
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
