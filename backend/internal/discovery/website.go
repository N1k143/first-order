package discovery

import (
	"context"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/temoto/robotstxt"
)

type WebsiteCheck struct {
	Status          string           `json:"status"`
	Reason          string           `json:"reason"`
	CheckedAt       time.Time        `json:"checked_at"`
	FinalURL        string           `json:"final_url,omitempty"`
	HTTPStatus      int              `json:"http_status,omitempty"`
	Evidence        []string         `json:"evidence"`
	SearchPerformed bool             `json:"search_performed"`
	Attempts        []WebsiteAttempt `json:"attempts,omitempty"`
	CandidateSource string           `json:"candidate_source,omitempty"`
	CandidateScope  string           `json:"candidate_scope,omitempty"`
}
type Checker struct {
	Client *http.Client
	robots map[string]robotsPolicy
}
type robotsPolicy struct {
	data    *robotstxt.RobotsData
	allowed bool
}

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96", "2002::/16"} {
		if netip.MustParsePrefix(raw).Contains(ip) {
			return false
		}
	}
	return true
}
func validTarget(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || !(u.Scheme == "http" || u.Scheme == "https") || len(raw) > 2048 {
		return nil, fmt.Errorf("invalid website URL")
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" {
		return nil, fmt.Errorf("unsupported port")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !publicAddress(ip) {
		return nil, fmt.Errorf("nonpublic destination")
	}
	return u, nil
}

// Resolve and dial the validated IP directly on every connection, including redirects.
// Proxy and automatic DNS re-resolution cannot bypass the address check.
func NewChecker() *Checker {
	transport := &http.Transport{TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second, MaxIdleConns: 4, MaxConnsPerHost: 1}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("DNS unavailable")
		}
		for _, ip := range ips {
			if !publicAddress(ip) {
				return nil, fmt.Errorf("nonpublic destination")
			}
		}
		var last error
		for _, ip := range ips {
			conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
	return &Checker{Client: &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return fmt.Errorf("redirect limit")
		}
		_, err := validTarget(req.URL.String())
		return err
	}}}
}
func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, s)), " ")
}
func externalPlatform(host string) bool {
	host = strings.ToLower(host)
	for _, domain := range []string{"instagram.com", "facebook.com", "vk.com", "t.me", "wa.me", "whatsapp.com", "2gis.kz", "2gis.ru", "yandex.ru", "yandex.kz", "google.com", "maps.google.com", "kaspi.kz", "taplink.cc", "linktr.ee"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
func (c *Checker) get(ctx context.Context, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "FirstOrder-CompanyDiscovery/1.0 (+https://e1.iq-home.kz)")
	return c.Client.Do(req)
}

// Evaluate rules for the requested path, not the mere presence of Disallow.
// Unknown/failed robots checks never imply permission or absence of a website.
func (c *Checker) robotsAllowed(ctx context.Context, u *url.URL) bool {
	target := u.Scheme + "://" + u.Host + "/robots.txt"
	if c.robots == nil {
		c.robots = map[string]robotsPolicy{}
	}
	policy, exists := c.robots[target]
	if exists {
		return policy.permits(ctx, u)
	}
	// Cache failures too: one bounded run should not repeatedly hit an unavailable host.
	c.robots[target] = robotsPolicy{}
	response, err := c.get(ctx, target)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode == 404 || response.StatusCode == 410 {
		c.robots[target] = robotsPolicy{allowed: true}
		return true
	}
	if response.StatusCode != 200 {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return false
	}
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/html") && !strings.Contains(strings.ToLower(string(raw)), "user-agent:") && len(strings.TrimSpace(string(raw))) > 0 {
		return false
	}
	data, err := robotstxt.FromBytes(raw)
	if err != nil {
		return false
	}
	policy = robotsPolicy{data: data}
	c.robots[target] = policy
	return policy.permits(ctx, u)
}

func (p robotsPolicy) permits(ctx context.Context, u *url.URL) bool {
	if p.data == nil {
		return p.allowed
	}
	group := p.data.FindGroup("FirstOrder-CompanyDiscovery")
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	if !group.Test(path) {
		return false
	}
	// Long Crawl-delay requests are deferred to manual review for this bounded pilot.
	if group.CrawlDelay > 10*time.Second {
		return false
	}
	if group.CrawlDelay > 0 {
		timer := time.NewTimer(group.CrawlDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
		}
	}
	return true
}

func (c *Checker) getPage(ctx context.Context, target string) (*http.Response, error) {
	client := *c.Client
	original := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return fmt.Errorf("redirect limit")
		}
		if _, err := validTarget(req.URL.String()); err != nil {
			return err
		}
		if original != nil {
			if err := original(req, via); err != nil {
				return err
			}
		}
		if !c.robotsAllowed(req.Context(), req.URL) {
			return fmt.Errorf("redirect path disallowed")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "FirstOrder-CompanyDiscovery/1.0 (+https://e1.iq-home.kz)")
	return client.Do(req)
}

var scripts = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(?:script|style)>`)
var markup = regexp.MustCompile(`(?s)<[^>]*>`)
var phones = regexp.MustCompile(`\+?[0-9][0-9 ()\-\.]{8,24}[0-9]`)

func (c *Checker) Check(ctx context.Context, company Candidate) WebsiteCheck {
	result := WebsiteCheck{Status: "needs_review", Reason: "no_website_in_source_search_required", CheckedAt: time.Now().UTC(), Evidence: []string{}}
	target := strings.TrimSpace(strings.Split(company.WebsiteURL, ";")[0])
	if target == "" {
		return result
	}
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}
	u, err := validTarget(target)
	if err != nil {
		result.Reason = "invalid_or_unsafe_url"
		return result
	}
	if externalPlatform(u.Hostname()) {
		result.Reason = "social_or_directory_link"
		return result
	}
	if !c.robotsAllowed(ctx, u) {
		result.Reason = "robots_or_access_restriction"
		return result
	}
	response, err := c.getPage(ctx, u.String())
	if err != nil {
		result.Reason = "website_unreachable"
		return result
	}
	defer response.Body.Close()
	result.HTTPStatus = response.StatusCode
	result.FinalURL = response.Request.URL.String()
	if externalPlatform(response.Request.URL.Hostname()) {
		result.Reason = "redirect_to_external_platform"
		return result
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Reason = "website_http_error"
		return result
	}
	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/html") {
		result.Reason = "not_html"
		return result
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (768<<10)+1))
	if err != nil || len(raw) > 768<<10 {
		result.Reason = "page_unreadable_or_too_large"
		return result
	}
	text := html.UnescapeString(markup.ReplaceAllString(scripts.ReplaceAllString(string(raw), " "), " "))
	normalized := normalize(text)
	for _, term := range []string{"domain for sale", "buy this domain", "домен продается", "домен продаётся"} {
		if strings.Contains(strings.ToLower(text), term) {
			result.Reason = "parked_domain"
			return result
		}
	}
	wanted := digits(strings.Split(company.Phone, ";")[0])
	phoneMatch := false
	if len(wanted) >= 10 {
		for _, value := range phones.FindAllString(text, -1) {
			got := digits(value)
			if len(got) >= 10 && got[len(got)-10:] == wanted[len(wanted)-10:] {
				phoneMatch = true
				break
			}
		}
	}
	name := normalize(company.Name)
	nameMatch := len([]rune(name)) >= 4 && strings.Contains(normalized, name)
	address := normalize(company.Address)
	addressMatch := len([]rune(address)) >= 6 && strings.Contains(normalized, address)
	emailMatch := false
	finalHost := strings.TrimPrefix(strings.ToLower(response.Request.URL.Hostname()), "www.")
	for _, host := range corporateEmailDomains(company.Email) {
		if finalHost == strings.TrimPrefix(host, "www.") {
			emailMatch = true
		}
	}
	if phoneMatch {
		result.Evidence = append(result.Evidence, "phone_match")
	}
	if nameMatch {
		result.Evidence = append(result.Evidence, "name_match")
	}
	if addressMatch {
		result.Evidence = append(result.Evidence, "address_match")
	}
	if emailMatch {
		result.Evidence = append(result.Evidence, "email_domain_match")
	}
	result.Reason = "ownership_not_confirmed"
	if phoneMatch || (nameMatch && (addressMatch || emailMatch)) {
		result.Status = "found"
		result.Reason = "ownership_evidence_matched"
	}
	return result
}
