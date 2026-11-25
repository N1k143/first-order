package discovery

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSearchCredentialsFilteringAndFailureSemantics(t *testing.T) {
	search := NewSearcher("private-test-key")
	for _, code := range []int{200, 429, 503} {
		search.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "api.search.brave.com" || r.Header.Get("X-Subscription-Token") != "private-test-key" || !strings.Contains(r.URL.Query().Get("q"), "Астана") {
				t.Fatal("wrong provider request")
			}
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(`{"type":"search","web":{"results":[{"url":"http://127.0.0.1/"},{"url":"https://instagram.com/x"},{"url":"https://company.kz"},{"url":"https://company.kz/about"}]}}`))}, nil
		})
		links, err := search.Find(context.Background(), Candidate{Name: "Company", City: "Астана"})
		if code == 200 {
			if err != nil || len(links) != 1 || links[0] != "https://company.kz" {
				t.Fatal("invalid candidates")
			}
		} else if err == nil {
			t.Fatal("provider failure treated as absence")
		}
	}
	req, _ := http.NewRequest("GET", "https://untrusted.example/", nil)
	if search.Client.CheckRedirect(req, nil) == nil {
		t.Fatal("credential redirect allowed")
	}
}
func TestSearchNoResultsNeverClaimsWebsiteAbsent(t *testing.T) {
	search := NewSearcher("private-test-key")
	search.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"type":"search"}`))}, nil
	})
	checker := NewChecker()
	defer checker.Client.CloseIdleConnections()
	result := checker.Discover(context.Background(), Candidate{Name: "Company", City: "Астана"}, search)
	if result.Status != "needs_review" || !result.SearchPerformed || result.Reason != "search_no_independent_website_candidates" {
		t.Fatal("wrong empty search result")
	}
}
