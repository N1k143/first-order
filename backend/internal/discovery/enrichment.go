package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"
)

// A source link is a candidate, not proof of ownership of a local branch's site.
type WebsiteCandidate struct {
	URL       string `json:"url"`
	Source    string `json:"source"`
	SourceURL string `json:"source_url,omitempty"`
	Scope     string `json:"scope"`
}
type WebsiteAttempt struct {
	Candidate  WebsiteCandidate `json:"candidate"`
	Status     string           `json:"status"`
	Reason     string           `json:"reason"`
	Evidence   []string         `json:"evidence"`
	FinalURL   string           `json:"final_url,omitempty"`
	HTTPStatus int              `json:"http_status,omitempty"`
}

var domainLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var qid = regexp.MustCompile(`^Q[1-9][0-9]{0,11}$`)

func corporateEmailDomains(value string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, raw := range strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == ',' }) {
		address, err := mail.ParseAddress(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		_, host, ok := strings.Cut(address.Address, "@")
		host = strings.ToLower(host)
		if !ok || len(host) > 253 || !strings.Contains(host, ".") || externalPlatform(host) {
			continue
		}
		public := false
		for _, provider := range []string{"gmail.com", "googlemail.com", "mail.ru", "bk.ru", "list.ru", "inbox.ru", "internet.ru", "yandex.ru", "yandex.kz", "ya.ru", "yahoo.com", "outlook.com", "hotmail.com", "live.com", "icloud.com", "me.com", "proton.me", "protonmail.com", "aol.com", "rambler.ru", "mail.kz", "email.com", "duck.com", "fastmail.com", "gmx.com", "gmx.de"} {
			if host == provider || strings.HasSuffix(host, "."+provider) {
				public = true
				break
			}
		}
		if public {
			continue
		}
		labels := strings.Split(host, ".")
		valid := true
		for _, label := range labels {
			if !domainLabel.MatchString(label) {
				valid = false
			}
		}
		if !valid || seen[host] {
			continue
		}
		if _, err := validTarget("https://" + host); err != nil {
			continue
		}
		seen[host] = true
		result = append(result, host)
		if len(result) == 3 {
			break
		}
	}
	return result
}

type Wikidata struct {
	Client *http.Client
	cache  map[string][]string
}

func NewWikidata() *Wikidata {
	return &Wikidata{Client: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("Wikidata redirects disabled") }}, cache: map[string][]string{}}
}
func (w *Wikidata) websites(ctx context.Context, id string) ([]string, error) {
	if !qid.MatchString(id) {
		return nil, fmt.Errorf("invalid Wikidata ID")
	}
	if links, ok := w.cache[id]; ok {
		return links, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.wikidata.org/wiki/Special:EntityData/"+id+".json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "FirstOrder-CompanyDiscovery/1.0 (+https://e1.iq-home.kz)")
	req.Header.Set("Accept", "application/json")
	response, err := w.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Wikidata unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("Wikidata HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return nil, fmt.Errorf("Wikidata response exceeds limit")
	}
	return w.decode(id, raw)
}

type websiteClaim struct {
	Rank       string                     `json:"rank"`
	Qualifiers map[string]json.RawMessage `json:"qualifiers"`
	MainSnak   struct {
		SnakType  string `json:"snaktype"`
		DataValue struct {
			Value string `json:"value"`
		} `json:"datavalue"`
	} `json:"mainsnak"`
}

func (w *Wikidata) decode(id string, raw []byte) ([]string, error) {
	var data struct {
		Entities map[string]struct {
			ID     string                     `json:"id"`
			Claims map[string]json.RawMessage `json:"claims"`
		} `json:"entities"`
	}
	if json.Unmarshal(raw, &data) != nil || data.Entities[id].ID != id {
		return nil, fmt.Errorf("invalid Wikidata entity")
	}
	var claims []websiteClaim
	if rawClaims := data.Entities[id].Claims["P856"]; len(rawClaims) > 0 {
		if json.Unmarshal(rawClaims, &claims) != nil {
			return nil, fmt.Errorf("invalid Wikidata website claims")
		}
	}
	preferred := false
	usable := func(claim websiteClaim) bool {
		return (claim.Rank == "normal" || claim.Rank == "preferred") && claim.MainSnak.SnakType == "value" && len(claim.Qualifiers["P582"]) == 0
	}
	for _, claim := range claims {
		if usable(claim) && claim.Rank == "preferred" {
			preferred = true
		}
	}
	links := []string{}
	seen := map[string]bool{}
	for _, claim := range claims {
		if !usable(claim) || (preferred && claim.Rank != "preferred") {
			continue
		}
		link := claim.MainSnak.DataValue.Value
		u, err := validTarget(link)
		if err != nil || externalPlatform(u.Hostname()) || seen[link] {
			continue
		}
		seen[link] = true
		links = append(links, link)
	}
	sort.Strings(links)
	if len(links) > 3 {
		links = links[:3]
	}
	if w.cache == nil {
		w.cache = map[string][]string{}
	}
	w.cache[id] = links
	return links, nil
}

func Enrich(ctx context.Context, company *Candidate, w *Wikidata) {
	company.WebsiteCandidates = nil
	company.DiscoveryWarnings = nil
	seen := map[string]bool{}
	add := func(link, source, sourceURL, scope string) {
		link = strings.TrimSpace(link)
		if link == "" {
			return
		}
		if !strings.Contains(link, "://") {
			link = "https://" + link
		}
		u, err := validTarget(link)
		if err != nil || externalPlatform(u.Hostname()) {
			return
		}
		key := strings.TrimSuffix(link, "/") + "|" + source + "|" + sourceURL + "|" + scope
		if seen[key] {
			return
		}
		seen[key] = true
		company.WebsiteCandidates = append(company.WebsiteCandidates, WebsiteCandidate{link, source, sourceURL, scope})
	}
	for _, link := range strings.Split(company.WebsiteURL, ";") {
		add(link, "osm", company.SourceURL, "company")
	}
	for _, host := range corporateEmailDomains(company.Email) {
		add("https://"+host, "email_domain", company.SourceURL, "inferred")
	}
	if w == nil {
		return
	}
	for _, entry := range []struct{ Tag, Scope string }{{"wikidata", "company"}, {"brand:wikidata", "brand"}, {"operator:wikidata", "operator"}} {
		id := strings.TrimSpace(company.Tags[entry.Tag])
		if id == "" {
			continue
		}
		links, err := w.websites(ctx, id)
		if err != nil {
			company.DiscoveryWarnings = append(company.DiscoveryWarnings, entry.Tag+":lookup_unavailable")
			continue
		}
		for _, link := range links {
			add(link, "wikidata", "https://www.wikidata.org/wiki/"+id, entry.Scope)
		}
	}
}

func (c *Checker) Discover(ctx context.Context, company Candidate) WebsiteCheck {
	if len(company.WebsiteCandidates) == 0 && strings.TrimSpace(company.WebsiteURL) != "" {
		return c.Check(ctx, company)
	}
	initial := WebsiteCheck{Status: "needs_review", Reason: "no_website_candidates", CheckedAt: time.Now().UTC(), Evidence: []string{}}
	attempts := []WebsiteAttempt{}
	seen := map[string]WebsiteCheck{}
	for _, candidate := range company.WebsiteCandidates {
		if ctx.Err() != nil {
			initial.Reason = "check_canceled"
			break
		}
		key := strings.TrimSuffix(candidate.URL, "/")
		check, ok := seen[key]
		if !ok {
			proposed := company
			proposed.WebsiteURL = candidate.URL
			check = c.Check(ctx, proposed)
			seen[key] = check
		}
		attempts = append(attempts, WebsiteAttempt{candidate, check.Status, check.Reason, check.Evidence, check.FinalURL, check.HTTPStatus})
		if len(attempts) == 1 {
			initial = check
		}
		if check.Status == "found" {
			check.Attempts = attempts
			check.CandidateSource = candidate.Source
			check.CandidateScope = candidate.Scope
			return check
		}
	}
	initial.Attempts = attempts
	return initial
}
