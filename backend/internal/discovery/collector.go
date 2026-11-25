// Package discovery collects source evidence without modifying public company cards.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const Attribution = "© OpenStreetMap contributors · ODbL 1.0 · https://www.openstreetmap.org/copyright"
const AstanaQuery = `[out:json][timeout:60][maxsize:67108864];area["boundary"="administrative"]["name:en"="Astana"]->.city;(nwr(area.city)["shop"]["name"];nwr(area.city)["office"]["name"];nwr(area.city)["craft"]["name"];nwr(area.city)["amenity"~"^(restaurant|cafe|fast_food|bar|pub|pharmacy|clinic|dentist|veterinary|bank|car_rental|car_wash|fuel|cinema|theatre|language_school|driving_school)$"]["name"];);out center tags;`

type Element struct {
	Type string            `json:"type"`
	ID   int64             `json:"id"`
	Tags map[string]string `json:"tags"`
}
type Snapshot struct {
	Elements []Element `json:"elements"`
	Remark   string    `json:"remark"`
}
type Candidate struct {
	Source            string             `json:"source"`
	ExternalID        string             `json:"external_id"`
	Name              string             `json:"name"`
	City              string             `json:"city"`
	Category          string             `json:"category"`
	Address           string             `json:"address"`
	Phone             string             `json:"phone"`
	Email             string             `json:"email"`
	WebsiteURL        string             `json:"website_url"`
	SourceURL         string             `json:"source_url"`
	Attribution       string             `json:"attribution"`
	Tags              map[string]string  `json:"tags"`
	Check             WebsiteCheck       `json:"website_check"`
	WebsiteCandidates []WebsiteCandidate `json:"website_candidates,omitempty"`
	DiscoveryWarnings []string           `json:"discovery_warnings,omitempty"`
}

func Fetch(ctx context.Context, endpoint string) (Snapshot, error) {
	var result Snapshot
	if endpoint == "" {
		endpoint = "https://overpass-api.de/api/interpreter"
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return result, fmt.Errorf("invalid HTTPS Overpass endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(url.Values{"data": {AstanaQuery}}.Encode()))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "FirstOrder-CompanyDiscovery/1.0 (+https://e1.iq-home.kz)")
	response, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return result, fmt.Errorf("Overpass request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return result, fmt.Errorf("Overpass HTTP %d; retry later", response.StatusCode)
	}
	return Decode(response.Body)
}
func Decode(r io.Reader) (Snapshot, error) {
	var data Snapshot
	raw, err := io.ReadAll(io.LimitReader(r, (16<<20)+1))
	if err != nil || len(raw) > 16<<20 {
		return data, fmt.Errorf("source response exceeds limit or is unreadable")
	}
	if json.Unmarshal(raw, &data) != nil || data.Elements == nil || data.Remark != "" {
		return Snapshot{}, fmt.Errorf("incomplete or invalid Overpass response")
	}
	return data, nil
}
func first(tags map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(tags[key]); value != "" {
			return value
		}
	}
	return ""
}
func category(tags map[string]string) string {
	for _, key := range []string{"shop", "office", "craft", "amenity"} {
		if value := tags[key]; value != "" && value != "no" && value != "vacant" {
			if key == "amenity" && !strings.Contains("|restaurant|cafe|fast_food|bar|pub|pharmacy|clinic|dentist|veterinary|bank|car_rental|car_wash|fuel|cinema|theatre|language_school|driving_school|", "|"+value+"|") {
				continue
			}
			return key + ":" + value
		}
	}
	return ""
}

// Select round-robins categories instead of selecting only the first map objects.
// Equal names alone never merge branches; same phone+address or name+address can.
func Select(snapshot Snapshot, limit int) []Candidate {
	if limit < 1 || limit > 100 {
		return nil
	}
	elements := append([]Element(nil), snapshot.Elements...)
	sort.Slice(elements, func(i, j int) bool {
		if elements[i].Type != elements[j].Type {
			return elements[i].Type < elements[j].Type
		}
		return elements[i].ID < elements[j].ID
	})
	groups := map[string][]Candidate{}
	seen := map[string]bool{}
	for _, e := range elements {
		if e.ID <= 0 || !(e.Type == "node" || e.Type == "way" || e.Type == "relation") {
			continue
		}
		t := e.Tags
		name := first(t, "name:ru", "name", "name:en")
		cat := category(t)
		if name == "" || cat == "" || t["disused"] == "yes" || t["abandoned"] == "yes" {
			continue
		}
		address := first(t, "addr:full")
		if address == "" {
			address = strings.TrimSpace(first(t, "addr:street") + " " + first(t, "addr:housenumber"))
		}
		phone := first(t, "contact:phone", "phone")
		id := e.Type + "/" + strconv.FormatInt(e.ID, 10)
		key := "id:" + id
		if address != "" {
			key = "address:" + strings.ToLower(address) + ":" + strings.ToLower(name)
			if digits(phone) != "" {
				key = "phone:" + digits(phone) + ":" + strings.ToLower(address)
			}
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		groups[cat] = append(groups[cat], Candidate{Source: "osm_astana", ExternalID: id, Name: name, City: "Астана", Category: cat, Address: address, Phone: phone, Email: first(t, "contact:email", "email"), WebsiteURL: first(t, "contact:website", "website", "url"), SourceURL: "https://www.openstreetmap.org/" + id, Attribution: Attribution, Tags: t})
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := []Candidate{}
	for offset := 0; len(result) < limit; offset++ {
		added := false
		for _, k := range keys {
			if offset < len(groups[k]) {
				result = append(result, groups[k][offset])
				added = true
				if len(result) == limit {
					break
				}
			}
		}
		if !added {
			break
		}
	}
	return result
}
