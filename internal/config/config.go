package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host          string
	Port          int
	Username      string
	Password      string
	Verbose       bool
	BufferKB      int
	PoolSize      int
	DialTimeout   time.Duration
	InitTimeout   time.Duration
	ConnectWSPath  string
	DCIPs          map[int]string
	MTProtoEnabled bool
	MTProtoHost    string
	MTProtoPort    int
	MTProtoSecret  string
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
