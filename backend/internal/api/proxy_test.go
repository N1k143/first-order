package api

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestRateLimitUsesOnlyTrustedProxyAddresses(t *testing.T) {
	s := New(nil, Config{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("::1/128")}})
	for _, test := range []struct{ peer, forwarded, want string }{
		{"198.51.100.1:20", "1.2.3.4", "198.51.100.1"},
		{"127.0.0.1:20", "198.51.100.2", "198.51.100.2"},
		{"127.0.0.1:20", "1.2.3.4, 198.51.100.2, 127.0.0.1", "198.51.100.2"},
		{"[::1]:20", "2001:db8::123, ::1", "2001:db8::123"},
		{"127.0.0.1:20", "malformed", "127.0.0.1"},
		{"127.0.0.1:20", "", "127.0.0.1"},
	} {
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.RemoteAddr = test.peer
		req.Header.Set("X-Forwarded-For", test.forwarded)
		if got := s.clientIP(req); got != test.want {
			t.Errorf("peer=%s forwarded=%s got=%s want=%s", test.peer, test.forwarded, got, test.want)
		}
	}
	s.Config.TrustedProxies = append(s.Config.TrustedProxies, netip.MustParsePrefix("173.245.48.0/20"))
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:20"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.9, 173.245.48.1, 127.0.0.1")
	if got := s.clientIP(req); got != "203.0.113.9" {
		t.Fatalf("Cloudflare chain trusted forged prefix: %s", got)
	}

}
