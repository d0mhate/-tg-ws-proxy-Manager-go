package telegram

import "testing"

func TestIsTelegramIP(t *testing.T) {
	if !IsTelegramIP("149.154.167.220") {
		t.Fatal("expected telegram ip to be recognized")
	}
	if IsTelegramIP("8.8.8.8") {
		t.Fatal("did not expect google dns to be recognized as telegram ip")
	}
}

func TestLookupEndpoint(t *testing.T) {
	ep, ok := LookupEndpoint("149.154.167.220")
	if !ok {
		t.Fatal("expected endpoint lookup to succeed")
	}
	if ep.DC != 2 || ep.IsMedia {
		t.Fatalf("unexpected endpoint: %+v", ep)
	}
}

func TestLookupEndpointAdditionalDC2Host(t *testing.T) {
	ep, ok := LookupEndpoint("149.154.167.35")
	if !ok {
		t.Fatal("expected additional dc2 endpoint lookup to succeed")
	}
	if ep.DC != 2 || ep.IsMedia {
		t.Fatalf("unexpected additional endpoint: %+v", ep)
	}
}

func TestLookupEndpointAdditionalTelegramCallHosts(t *testing.T) {
	testCases := []struct {
		ip   string
		dc   int
		name string
	}{
		{ip: "149.154.167.255", dc: 2, name: "dc2 call host"},
		{ip: "149.154.175.211", dc: 1, name: "dc1 call host"},
	}

	for _, tc := range testCases {
		ep, ok := LookupEndpoint(tc.ip)
		if !ok {
			t.Fatalf("expected lookup to succeed for %s", tc.name)
		}
		if ep.DC != tc.dc || ep.IsMedia {
			t.Fatalf("unexpected endpoint for %s: %+v", tc.name, ep)
		}
	}
}

func TestLookupEndpointIPv6(t *testing.T) {
	ep, ok := LookupEndpoint("2001:67c:4e8:f002::7")
	if !ok {
		t.Fatal("expected ipv6 endpoint lookup to succeed")
	}
	if ep.DC != 2 || ep.IsMedia {
		t.Fatalf("unexpected ipv6 endpoint: %+v", ep)
	}
}

func TestWSDomains(t *testing.T) {
	got := WSDomains(203, false)
	if len(got) != 2 {
		t.Fatalf("unexpected ws domains len: %d", len(got))
	}
	if got[0] != "kws2.web.telegram.org" {
		t.Fatalf("unexpected first ws domain: %q", got[0])
	}
}

func TestCFWSDomain(t *testing.T) {
	cases := []struct {
		dc   int
		want string
	}{
		{1, "kws1.example.com"},
		{2, "kws2.example.com"},
		{203, "kws203.example.com"},
		{-2, "kws2.example.com"},
	}
	for _, tc := range cases {
		if got := CFWSDomain("example.com", tc.dc); got != tc.want {
			t.Errorf("CFWSDomain(example.com, %d) = %q, want %q", tc.dc, got, tc.want)
		}
	}
}

func TestCFWSDomains(t *testing.T) {
	got := CFWSDomains("example.com", 2)
	if len(got) != 1 || got[0] != "kws2.example.com" {
		t.Fatalf("unexpected CF domains: %v", got)
	}
}

func TestNormalizeDC(t *testing.T) {
	if got := NormalizeDC(203); got != 2 {
		t.Fatalf("unexpected normalized dc: %d", got)
	}
	if got := NormalizeDC(4); got != 4 {
		t.Fatalf("unexpected normalized dc without override: %d", got)
	}
}
