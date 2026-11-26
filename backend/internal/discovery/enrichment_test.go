package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestCorporateEmailDomains(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"sales@company.kz", []string{"company.kz"}},
		{"SALES@Company.KZ;info@company.kz", []string{"company.kz"}},
		{"team@gmail.com;person@mail.ru;user@list.ru;user@sub.outlook.com", []string{}},
		{"invalid;name@localhost;name@127.0.0.1;name@bad-.kz;user@instagram.com", []string{}},
		{"Contact <info@branch.company.kz>", []string{"branch.company.kz"}},
	} {
		if got := corporateEmailDomains(tc.input); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("domains for %q: %v", tc.input, got)
		}
	}
}
func TestWikidataClaimsExcludeHistoricalAndUnsafeLinks(t *testing.T) {
	w := NewWikidata()
	raw := []byte(`{"entities":{"Q42":{"id":"Q42","claims":{"P856":[
 {"rank":"deprecated","mainsnak":{"snaktype":"value","datavalue":{"value":"https://old.kz"}}},
 {"rank":"preferred","qualifiers":{"P582":[{}]},"mainsnak":{"snaktype":"value","datavalue":{"value":"https://ended.kz"}}},
 {"rank":"preferred","mainsnak":{"snaktype":"value","datavalue":{"value":"https://company.kz"}}},
 {"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"https://other.kz"}}},
 {"rank":"preferred","mainsnak":{"snaktype":"value","datavalue":{"value":"http://127.0.0.1"}}},
 {"rank":"preferred","mainsnak":{"snaktype":"value","datavalue":{"value":"https://instagram.com/company"}}}
 ],"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q123"}}}}]}}}}`)
	links, err := w.decode("Q42", raw)
	if err != nil || !reflect.DeepEqual(links, []string{"https://company.kz"}) {
		t.Fatal("wrong live claims", links, err)
	}
	if _, err = w.decode("Q43", raw); err == nil {
		t.Fatal("wrong entity accepted")
	}
}
func TestWikidataBoundedRequestsAndEnrichmentProvenance(t *testing.T) {
	w := NewWikidata()
	calls := 0
	w.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "www.wikidata.org" || r.Header.Get("User-Agent") == "" {
			t.Fatal("invalid Wikidata request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"entities":{"Q42":{"id":"Q42","claims":{"P856":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"https://brand.kz"}}}]}}}}`))}, nil
	})
	company := Candidate{WebsiteURL: "company.kz;https://company.kz/", Email: "hi@company.kz", SourceURL: "https://www.openstreetmap.org/node/1", Tags: map[string]string{"brand:wikidata": "Q42", "operator:wikidata": "Q42"}}
	Enrich(context.Background(), &company, w)
	if calls != 1 || len(company.WebsiteCandidates) != 4 {
		t.Fatalf("cache/provenance: calls=%d candidates=%+v", calls, company.WebsiteCandidates)
	}
	for _, entry := range company.WebsiteCandidates {
		if entry.SourceURL == "" || entry.Scope == "" {
			t.Fatal("missing source")
		}
	}
	if _, err := w.websites(context.Background(), "../../private"); err == nil || calls != 1 {
		t.Fatal("unsafe ID reached network")
	}
	w.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("limited"))}, nil
	})
	company.Tags = map[string]string{"wikidata": "Q43"}
	Enrich(context.Background(), &company, w)
	if len(company.DiscoveryWarnings) != 1 {
		t.Fatal("provider error silently lost")
	}
}
func TestDiscoverTriesAlternativeAndRequiresIdentity(t *testing.T) {
	pages := 0
	checker := &Checker{Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		code := 200
		body := "Company contact"
		if r.URL.Path == "/robots.txt" {
			code = 404
			body = ""
		} else {
			pages++
		}
		return &http.Response{StatusCode: code, Request: r, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	company := Candidate{Name: "Company", Email: "info@company.kz", WebsiteCandidates: []WebsiteCandidate{
		{URL: "https://wrong.kz", Source: "osm", Scope: "company"},
		{URL: "https://company.kz", Source: "email_domain", Scope: "inferred"},
	}}
	result := checker.Discover(context.Background(), company)
	if result.Status != "found" || result.CandidateSource != "email_domain" || len(result.Attempts) != 2 || pages != 2 || result.SearchPerformed {
		t.Fatalf("wrong discovery %+v", result)
	}
	if !strings.Contains(strings.Join(result.Evidence, ","), "email_domain_match") {
		t.Fatal("missing email evidence")
	}
	company.Email = "other@gmail.com"
	result = checker.Discover(context.Background(), company)
	if result.Status != "needs_review" {
		t.Fatal("name alone accepted")
	}
}
func TestRobotsPathRulesAndRedirectRestrictions(t *testing.T) {
	for _, tc := range []struct {
		robots, path string
		allowed      bool
	}{
		{"User-agent: *\nDisallow: /admin", "/", true},
		{"User-agent: *\nDisallow: /", "/", false},
		{"User-agent: *\nDisallow: /\nAllow: /public", "/public/info", true},
		{"User-agent: *\nDisallow: /*?", "/?q=a", false},
		{"User-agent: OtherBot\nDisallow: /\nUser-agent: *\nDisallow: /admin", "/", true},
		{"User-agent: *\nDisallow: /\nUser-agent: FirstOrder-CompanyDiscovery\nAllow: /", "/", true},
	} {
		t.Run(fmt.Sprintf("%s_%v", tc.path, tc.allowed), func(t *testing.T) {
			calls := 0
			checker := &Checker{Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.robots)), Header: http.Header{"Content-Type": {"text/plain"}}, Request: r}, nil
			})}}
			req, _ := http.NewRequest("GET", "https://company.kz"+tc.path, nil)
			if checker.robotsAllowed(context.Background(), req.URL) != tc.allowed {
				t.Fatal("incorrect path rule")
			}
			checker.robotsAllowed(context.Background(), req.URL)
			if calls != 1 {
				t.Fatal("robots not cached")
			}
		})
	}
	checker := &Checker{Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/robots.txt" {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Request: r, Body: io.NopCloser(strings.NewReader("User-agent: *\nDisallow: /private"))}, nil
		}
		if r.URL.Path == "/private" {
			t.Fatal("disallowed redirect path fetched")
		}
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"/private"}}, Request: r, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	if _, err := checker.getPage(context.Background(), "https://company.kz/"); err == nil {
		t.Fatal("redirect ignored robots")
	}
}
