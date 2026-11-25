package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"
)

func TestSelectBalancesCategoriesDeduplicatesBranches(t *testing.T) {
	data := Snapshot{Elements: []Element{
		{Type: "node", ID: 1, Tags: map[string]string{"name": "Shop", "shop": "bakery", "addr:street": "Main", "addr:housenumber": "1", "phone": "+7 (777) 1234567"}},
		{Type: "way", ID: 2, Tags: map[string]string{"name": "Shop", "shop": "bakery", "addr:street": "Main", "addr:housenumber": "1", "phone": "77771234567"}},
		{Type: "node", ID: 3, Tags: map[string]string{"name": "Shop", "shop": "bakery", "addr:street": "Main", "addr:housenumber": "2", "phone": "77771234567"}},
		{Type: "node", ID: 4, Tags: map[string]string{"name": "Cafe", "amenity": "cafe"}},
		{Type: "node", ID: 5, Tags: map[string]string{"name": "Abandoned", "shop": "vacant"}},
		{Type: "node", ID: 6, Tags: map[string]string{"name": "Bench", "amenity": "bench"}},
	}}
	// Production query excludes benches; Select should enforce business scope for replay too.
	result := Select(data, 3)
	if len(result) != 3 || result[0].Category == result[1].Category {
		t.Fatal("categories not balanced")
	}
	result = Select(data, 100)
	if len(result) != 3 {
		t.Fatalf("unexpected candidates: %d", len(result))
	}
	for _, row := range result {
		if row.City != "Астана" || row.Attribution == "" || !strings.HasPrefix(row.SourceURL, "https://www.openstreetmap.org/") {
			t.Fatal("missing provenance")
		}
	}
}
func TestDecodeRejectsPartialSource(t *testing.T) {
	for _, body := range []string{`{}`, `{"elements":[],"remark":"runtime error"}`, `<html>error</html>`} {
		if _, err := Decode(strings.NewReader(body)); err == nil {
			t.Fatal("accepted incomplete source")
		}
	}
}
func TestWebsiteDestinationSafety(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1", "http://[::1]", "http://169.254.169.254/latest", "file:///etc/passwd", "https://user:pass@example.com", "https://example.com:8080", "http://[::ffff:127.0.0.1]"} {
		if _, err := validTarget(raw); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
	for _, raw := range []string{"10.0.0.1", "100.100.100.200", "192.168.1.1", "198.18.0.1", "fc00::1", "64:ff9b::7f00:1"} {
		if publicAddress(netip.MustParseAddr(raw)) {
			t.Fatal("unsafe destination accepted")
		}
	}
	for _, raw := range []string{"1.1.1.1", "2606:4700:4700::1111"} {
		if !publicAddress(netip.MustParseAddr(raw)) {
			t.Fatal("public IP blocked")
		}
	}
	c := NewChecker()
	defer c.Client.CloseIdleConnections()
	if _, err := c.Client.Transport.(*http.Transport).DialContext(context.Background(), "tcp", "127.0.0.1:80"); err == nil {
		t.Fatal("private dial allowed")
	}
	req, _ := http.NewRequest("GET", "http://127.0.0.1/", nil)
	if c.Client.CheckRedirect(req, []*http.Request{req}) == nil {
		t.Fatal("private redirect allowed")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestWebsiteChecksRequireEvidenceAndRespectRobots(t *testing.T) {
	tests := []struct {
		name, url, robots, body, status, reason string
		code                                    int
	}{
		{name: "no link", status: "needs_review", reason: "no_website_in_source_search_required"},
		{name: "social", url: "https://www.instagram.com/company", status: "needs_review", reason: "social_or_directory_link"},
		{name: "blocked", url: "https://company.kz", robots: "User-agent: *\nDisallow: /", status: "needs_review", reason: "robots_or_access_restriction"},
		{name: "phone", url: "https://company.kz", body: "<p>Телефон +7 (777) 123-45-67</p>", status: "found", reason: "ownership_evidence_matched", code: 200},
		{name: "wrong company", url: "https://company.kz", body: "Other unrelated business", status: "needs_review", reason: "ownership_not_confirmed", code: 200},
		{name: "name alone", url: "https://company.kz", body: "Company", status: "needs_review", reason: "ownership_not_confirmed", code: 200},
		{name: "address", url: "https://company.kz", body: "Company Main street 15", status: "found", reason: "ownership_evidence_matched", code: 200},
		{name: "parked", url: "https://company.kz", body: "Company Main street 15 domain for sale", status: "needs_review", reason: "parked_domain", code: 200},
		{name: "failure", url: "https://company.kz", status: "needs_review", reason: "website_http_error", code: 503},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pages := 0
			c := &Checker{Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				body := tc.body
				code := tc.code
				if r.URL.Path == "/robots.txt" {
					body = tc.robots
					code = 200
				} else {
					pages++
				}
				if code == 0 {
					return nil, fmt.Errorf("network failure")
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}}
			result := c.Check(context.Background(), Candidate{Name: "Company", Address: "Main street 15", Phone: "+77771234567", WebsiteURL: tc.url})
			if result.Status != tc.status || result.Reason != tc.reason || result.SearchPerformed {
				t.Fatalf("wrong result: %+v", result)
			}
			if tc.name == "blocked" && pages != 0 {
				t.Fatal("robots ignored")
			}
		})
	}
}
