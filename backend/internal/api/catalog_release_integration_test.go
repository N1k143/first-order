package api

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func releaseFixture(t *testing.T, pool *pgxpool.Pool, f testFixture, category, phone string) string {
	t.Helper()
	id := moderationFixture(t, pool, f)
	raw, _ := json.Marshal(map[string]string{"category": category, "phone": phone})
	if _, err := pool.Exec(context.Background(), `UPDATE importer.company_candidates SET payload=payload || $2::jsonb WHERE id=$1`, id, raw); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestIntegrationCatalogReleaseProvenanceAndRepeat(t *testing.T) {
	s, pool, f := dbTest(t)
	ctx := context.Background()
	id := releaseFixture(t, pool, f, "office:it", "8 (701) 111-22-33")
	releaseFixture(t, pool, f, "office:government", "+77012223344")
	releaseFixture(t, pool, f, "shop:florist", "")
	manual := releaseFixture(t, pool, f, "office:it", "+77013334455")
	if _, err := pool.Exec(ctx, `UPDATE importer.company_candidates SET draft='{"name":"Human reviewed draft"}' WHERE id=$1`, manual); err != nil {
		t.Fatal(err)
	}
	preview, err := s.PrepareCatalog(ctx, false, false)
	if err != nil {
		t.Fatal(err)
	}
	eligible := 0
	for _, item := range preview.Items {
		if item.Ready {
			eligible++
		}
	}
	if eligible != 1 || preview.Created != 0 {
		t.Fatalf("wrong preview %+v", preview)
	}
	var status string
	pool.QueryRow(ctx, `SELECT review_status FROM importer.company_candidates WHERE id=$1`, id).Scan(&status)
	if status != "pending" {
		t.Fatal("preview mutated data")
	}
	result, err := s.PrepareCatalog(ctx, true, true)
	if err != nil || result.Created != 1 || result.Published != 1 {
		t.Fatalf("release: %+v %v", result, err)
	}
	var cid string
	pool.QueryRow(ctx, `SELECT company_id::text FROM importer.company_candidates WHERE id=$1`, id).Scan(&cid)
	response := testRequest(t, s, "GET", "/api/v1/companies/"+cid, "", nil)
	testStatus(t, response, 200)
	item := testObject(t, response)
	if item["phone"] != "+77011112233" || item["category_name"] != "Компьютеры, электроника и сервис" || item["has_contacts"] != true {
		t.Fatal("normalization missing")
	}
	if len(item["sources"].([]any)) != 1 || len(item["phones"].([]any)) != 1 || !strings.Contains(response.Header().Get("Link"), "rel=\"license\"") {
		t.Fatal("source attribution missing")
	}
	// No private evidence or reviewer identity may be exposed by public provenance.
	source := item["sources"].([]any)[0].(map[string]any)
	if source["license"] != "ODbL-1.0" || source["source_url"] == nil || source["payload"] != nil || source["draft"] != nil || source["reviewed_by"] != nil {
		t.Fatal("bad provenance")
	}
	response = testRequest(t, s, "GET", "/api/v1/companies?q="+url.QueryEscape("+77011112233"), "", nil)
	testStatus(t, response, 200)
	if testObject(t, response)["total"] != float64(1) {
		t.Fatal("phone search not working")
	}
	response = testRequest(t, s, "GET", "/api/v1/catalog/filters", "", nil)
	testStatus(t, response, 200)
	if testObject(t, response)["attribution"] == nil {
		t.Fatal("facet attribution missing")
	}
	categories := testObject(t, response)["categories"].([]any)
	found := false
	for _, row := range categories {
		v := row.(map[string]any)
		if v["name"] == "Компьютеры, электроника и сервис" {
			found = v["total"] == float64(1)
		}
	}
	if !found {
		t.Fatal("public category counts wrong")
	}
	var count int
	pool.QueryRow(ctx, `SELECT count(*) FROM catalog.moderation_actions WHERE company_id=$1 AND actor_type='operator' AND moderator_id IS NULL`, cid).Scan(&count)
	if count != 2 {
		t.Fatal("operator decisions not auditable")
	}
	result, err = s.PrepareCatalog(ctx, true, true)
	if err != nil || result.Created != 0 || result.Published != 0 {
		t.Fatal("repeat created duplicates", err)
	}
	// Approval preserved reviewed source and manual drafts were not overridden.
	pool.QueryRow(ctx, `SELECT review_status FROM importer.company_candidates WHERE id=$1`, manual).Scan(&status)
	if status != "pending" {
		t.Fatal("manual draft auto-approved")
	}
}
func TestIntegrationCatalogDuplicatePhonesAndHiddenFacets(t *testing.T) {
	s, pool, f := dbTest(t)
	id := releaseFixture(t, pool, f, "office:it", "+77011112233")
	if _, err := pool.Exec(context.Background(), `UPDATE catalog.companies SET phone='+7 700 999 88 77; 8 (701) 111-22-33' WHERE id=$1`, f.HiddenCompany); err != nil {
		t.Fatal(err)
	}
	response := testRequest(t, s, "GET", "/api/v1/imports/candidates/"+id+"/duplicates", f.Moderator, nil)
	testStatus(t, response, 200)
	if testObject(t, response)["total"] != float64(1) {
		t.Fatal("shared phone in a list not matched")
	}
	result, err := s.PrepareCatalog(context.Background(), true, true)
	if err != nil || result.Created != 0 {
		t.Fatal("duplicate published", err)
	}
	response = testRequest(t, s, "GET", "/api/v1/catalog/filters", "", nil)
	testStatus(t, response, 200)
	categories := testObject(t, response)["categories"].([]any)
	for _, item := range categories {
		if item.(map[string]any)["total"] != float64(1) {
			t.Fatal("hidden company leaked into category counts")
		}
	}
}
