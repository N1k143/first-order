package api

import (
	"context"
	"encoding/json"
	"ingeneriya/backend/internal/discovery"
	"testing"
	"time"
)

func TestIntegrationDiscoveryReview(t *testing.T) {
	s, pool, f := dbTest(t)
	ctx := context.Background()
	candidate := discovery.Candidate{Source: "osm_astana", ExternalID: "node/" + f.Company, Name: "Discovery " + f.Company, City: "Астана", Category: "Discovery category " + f.Category, Address: "Test street 12", WebsiteURL: "https://example.com/unconfirmed", SourceURL: "https://www.openstreetmap.org/node/" + f.Company, Attribution: discovery.Attribution, Check: discovery.WebsiteCheck{Status: "needs_review", Reason: "ownership_not_confirmed", CheckedAt: time.Now().UTC()}}
	raw, _ := json.Marshal(candidate)
	id := newID()
	if _, err := pool.Exec(ctx, `INSERT INTO importer.company_candidates(id,source,external_id,payload) VALUES($1,$2,$3,$4)`, id, candidate.Source, candidate.ExternalID, raw); err != nil {
		t.Fatal(err)
	}
	var company string
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM importer.company_candidates WHERE id=$1`, id)
		if company != "" {
			pool.Exec(ctx, `DELETE FROM catalog.moderation_actions WHERE company_id=$1`, company)
			pool.Exec(ctx, `DELETE FROM catalog.companies WHERE id=$1`, company)
		}
		pool.Exec(ctx, `DELETE FROM catalog.categories WHERE name=$1`, candidate.Category)
	})
	path := "/api/v1/imports/candidates/" + id + "/review"
	testStatus(t, testRequest(t, s, "GET", "/api/v1/imports/candidates", "", nil), 401)
	testStatus(t, testRequest(t, s, "GET", "/api/v1/imports/candidates", f.Developer, nil), 403)
	testStatus(t, testRequest(t, s, "GET", "/api/v1/imports/candidates", f.Moderator, nil), 200)
	testStatus(t, testRequest(t, s, "POST", path, f.Developer, map[string]string{"decision": "approve"}), 403)
	testStatus(t, testRequest(t, s, "POST", path, f.Moderator, map[string]string{"decision": "publish"}), 422)
	response := testRequest(t, s, "POST", path, f.Moderator, map[string]string{"decision": "approve"})
	testStatus(t, response, 200)
	company = testObject(t, response)["company_id"].(string)
	var visibility string
	var website *string
	if err := pool.QueryRow(ctx, `SELECT visibility,website_url FROM catalog.companies WHERE id=$1`, company).Scan(&visibility, &website); err != nil || visibility != "hidden" || website != nil {
		t.Fatal("unsafe or published company created", err)
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/companies/"+company, "", nil), 404)
	testStatus(t, testRequest(t, s, "POST", path, f.Moderator, map[string]string{"decision": "approve"}), 409)
	// Duplicate candidates from another source object must not create another card.
	other := newID()
	candidate.ExternalID = "way/" + f.Company
	raw, _ = json.Marshal(candidate)
	if _, err := pool.Exec(ctx, `INSERT INTO importer.company_candidates(id,source,external_id,payload) VALUES($1,$2,$3,$4)`, other, candidate.Source, candidate.ExternalID, raw); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM importer.company_candidates WHERE id=$1`, other)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/imports/candidates/"+other+"/review", f.Moderator, map[string]string{"decision": "approve"}), 409)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/imports/candidates/"+other+"/review", f.Moderator, map[string]string{"decision": "reject"}), 200)
}
