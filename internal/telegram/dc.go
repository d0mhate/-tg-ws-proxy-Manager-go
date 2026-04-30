package telegram

import (
	"encoding/binary"
	"net"
	"strings"
)

type Endpoint struct {
	DC      int
	IsMedia bool
}

var ipRanges = [][2]uint32{
	{mustIPv4ToUint32("185.76.151.0"), mustIPv4ToUint32("185.76.151.255")},
	{mustIPv4ToUint32("149.154.160.0"), mustIPv4ToUint32("149.154.175.255")},
	{mustIPv4ToUint32("91.105.192.0"), mustIPv4ToUint32("91.105.193.255")},
	{mustIPv4ToUint32("91.108.0.0"), mustIPv4ToUint32("91.108.255.255")},
}

var IPToDC = map[string]Endpoint{
	IPv4DC1:           {DC: 1, IsMedia: false},
	"149.154.175.51":  {DC: 1, IsMedia: false},
	"149.154.175.53":  {DC: 1, IsMedia: false},
	"149.154.175.54":  {DC: 1, IsMedia: false},
	IPv4DC1Call:       {DC: 1, IsMedia: false},
	"149.154.175.52":  {DC: 1, IsMedia: true},
	IPv4DC2Alt:        {DC: 2, IsMedia: false},
	IPv4DC2AltHTTP:    {DC: 2, IsMedia: false},
	"149.154.167.50":  {DC: 2, IsMedia: false},
	"149.154.167.51":  {DC: 2, IsMedia: false},
	IPv4DC2Call:       {DC: 2, IsMedia: false},
	IPv4DC2:           {DC: 2, IsMedia: false},
	"95.161.76.100":   {DC: 2, IsMedia: false},
	"149.154.167.151": {DC: 2, IsMedia: true},
	"149.154.167.222": {DC: 2, IsMedia: true},
	"149.154.167.223": {DC: 2, IsMedia: true},
	"149.154.162.123": {DC: 2, IsMedia: true},
	"149.154.175.100": {DC: 3, IsMedia: false},
	"149.154.175.101": {DC: 3, IsMedia: false},
	"149.154.175.102": {DC: 3, IsMedia: true},
	"149.154.167.91":  {DC: 4, IsMedia: false},
	"149.154.167.92":  {DC: 4, IsMedia: false},
	"149.154.164.250": {DC: 4, IsMedia: true},
	"149.154.166.120": {DC: 4, IsMedia: true},
	"149.154.166.121": {DC: 4, IsMedia: true},
	"149.154.167.118": {DC: 4, IsMedia: true},
	"149.154.165.111": {DC: 4, IsMedia: true},
	IPv4DC5:           {DC: 5, IsMedia: false},
	"91.108.56.101":   {DC: 5, IsMedia: false},
	"91.108.56.116":   {DC: 5, IsMedia: false},
	"91.108.56.126":   {DC: 5, IsMedia: false},
	IPv4DC5Alt:        {DC: 5, IsMedia: false},
	"91.108.56.102":   {DC: 5, IsMedia: true},
	"91.108.56.128":   {DC: 5, IsMedia: true},
	"91.108.56.151":   {DC: 5, IsMedia: true},
	IPv4DC203:         {DC: 203, IsMedia: false},
}

var DCOverrides = map[int]int{
	203: 2,
}

var IPv6Prefixes = []struct {
	Prefix   string
	Endpoint Endpoint
}{
	{Prefix: "2001:b28:f23d:f001:", Endpoint: Endpoint{DC: 1, IsMedia: false}},
	{Prefix: "2001:67c:4e8:f002:", Endpoint: Endpoint{DC: 2, IsMedia: false}},
	{Prefix: "2001:b28:f23d:f003:", Endpoint: Endpoint{DC: 3, IsMedia: false}},
	{Prefix: "2001:67c:4e8:f004:", Endpoint: Endpoint{DC: 4, IsMedia: false}},
	{Prefix: "2001:b28:f23f:f005:", Endpoint: Endpoint{DC: 5, IsMedia: false}},
}

func IsTelegramIP(ip string) bool {
	n, ok := ipv4ToUint32(ip)
	if !ok {
		return false
	}
	for _, r := range ipRanges {
		if r[0] <= n && n <= r[1] {
			return true
		}
	}
	return false
}

func LookupEndpoint(ip string) (Endpoint, bool) {
	ep, ok := IPToDC[ip]
	if ok {
		return ep, true
	}

	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() != nil {
		return Endpoint{}, false
	}

	normalized := strings.ToLower(parsed.String())
	for _, item := range IPv6Prefixes {
		if strings.HasPrefix(normalized, item.Prefix) {
			return item.Endpoint, true
		}
	}

	return Endpoint{}, false
}

func WSDomains(dc int, isMedia bool) []string {
	dc = overrideDC(dc)
	if isMedia {
		return []string{"kws" + itoa(dc) + "-1.web.telegram.org", "kws" + itoa(dc) + ".web.telegram.org"}
	}
	return []string{"kws" + itoa(dc) + ".web.telegram.org", "kws" + itoa(dc) + "-1.web.telegram.org"}
}

func CFWSDomain(baseDomain string, dc int) string {
	if dc < 0 {
		dc = -dc
	}
	return "kws" + itoa(dc) + "." + baseDomain
}

func CFWSDomains(baseDomain string, dc int) []string {
	return []string{CFWSDomain(baseDomain, dc)}
}

func NormalizeDC(dc int) int {
	return overrideDC(dc)
}

func overrideDC(dc int) int {
	if mapped, ok := DCOverrides[dc]; ok {
		return mapped
	}
	return dc
}

func mustIPv4ToUint32(value string) uint32 {
	n, ok := ipv4ToUint32(value)
	if !ok {
		panic("invalid IPv4 literal: " + value)
	}
	return n
}

func ipv4ToUint32(value string) (uint32, bool) {
	ip := net.ParseIP(value)
	if ip == nil {
		return 0, false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return 0, false
	}
	return binary.BigEndian.Uint32(ip4), true
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	if v < 0 {
		return "-" + itoa(-v)
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
