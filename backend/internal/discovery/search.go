package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Search is optional. Its credential is only sent to the fixed provider endpoint.
// HTTP failures never become "not_found".
type Searcher struct {
	Key    string
	Client *http.Client
}

func NewSearcher(key string) *Searcher {
	return &Searcher{Key: key, Client: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("search redirects disabled") }}}
}
func (s *Searcher) Find(ctx context.Context, company Candidate) ([]string, error) {
	query := company.Name + " " + company.City
	if phone := strings.TrimSpace(strings.Split(company.Phone, ";")[0]); phone != "" {
		query += " " + phone
	}
	words := strings.Fields(query)
	if len(words) > 75 {
		words = words[:75]
	}
	query = strings.Join(words, " ")
	runes := []rune(query)
	if len(runes) > 600 {
		query = string(runes[:600])
	}
	params := url.Values{"q": {query}, "count": {"5"}, "country": {"ALL"}, "search_lang": {"ru"}}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.search.brave.com/res/v1/web/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Subscription-Token", s.Key)
	req.Header.Set("Accept", "application/json")
	response, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("search HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, fmt.Errorf("search response too large")
	}
	var data struct {
		Type string `json:"type"`
		Web  struct {
			Results []struct {
				URL string `json:"url"`
			} `json:"results"`
		} `json:"web"`
	}
	if json.Unmarshal(raw, &data) != nil || data.Type != "search" {
		return nil, fmt.Errorf("invalid search response")
	}
	result := []string{}
	seen := map[string]bool{}
	for _, item := range data.Web.Results {
		u, err := validTarget(item.URL)
		if err != nil || externalPlatform(u.Hostname()) || seen[u.Hostname()] {
			continue
		}
		seen[u.Hostname()] = true
		result = append(result, item.URL)
		if len(result) == 3 {
			break
		}
	}
	return result, nil
}
func (c *Checker) Discover(ctx context.Context, company Candidate, search *Searcher) WebsiteCheck {
	initial := c.Check(ctx, company)
	if initial.Status == "found" || search == nil || search.Key == "" {
		return initial
	}
	links, err := search.Find(ctx, company)
	if err != nil {
		initial.Reason = "search_unavailable"
		return initial
	}
	initial.SearchPerformed = true
	if len(links) == 0 {
		initial.Reason = "search_no_independent_website_candidates"
		return initial
	}
	for _, link := range links {
		proposed := company
		proposed.WebsiteURL = link
		check := c.Check(ctx, proposed)
		check.SearchPerformed = true
		if check.Status == "found" {
			return check
		}
	}
	initial.Reason = "search_candidates_unconfirmed"
	return initial
}
