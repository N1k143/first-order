package api

import (
	"bytes"
	"context"
	"encoding/json"
	"ingeneriya/backend/internal/discovery"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testRequestWithImporter(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://test.local")
	req.Header.Set("Authorization", "Bearer "+s.Config.ImporterToken)
	result := httptest.NewRecorder()
	s.Handler().ServeHTTP(result, req)
	return result
}

func moderationFixture(t *testing.T, pool *pgxpool.Pool, f testFixture) string {
	t.Helper()
	id := newID()
	candidate := discovery.Candidate{Source: "osm_astana", ExternalID: "node/" + id, Name: "Candidate " + id, City: "Астана", Category: "Candidate category " + id, Phone: "+70001112233", SourceURL: "https://www.openstreetmap.org/node/" + id, Check: discovery.WebsiteCheck{Status: "needs_review", CheckedAt: time.Now().UTC()}}
	raw, _ := json.Marshal(candidate)
	if _, err := pool.Exec(context.Background(), `INSERT INTO importer.company_candidates(id,source,external_id,payload) VALUES($1,'osm_astana',$2,$3)`, id, candidate.ExternalID, raw); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		var cid *string
		pool.QueryRow(ctx, `SELECT company_id::text FROM importer.company_candidates WHERE id=$1`, id).Scan(&cid)
		pool.Exec(ctx, `DELETE FROM importer.company_candidates WHERE id=$1`, id)
		if cid != nil && *cid != f.Company && *cid != f.HiddenCompany {
			pool.Exec(ctx, `DELETE FROM catalog.moderation_actions WHERE company_id=$1`, *cid)
			pool.Exec(ctx, `DELETE FROM catalog.companies WHERE id=$1`, *cid)
		}
		pool.Exec(ctx, `DELETE FROM catalog.categories WHERE name=$1`, candidate.Category)
	})
	return id
}
func TestIntegrationModerationRoles(t *testing.T) {
	s, _, f := dbTest(t)
	routes := []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/v1/moderation/summary", nil},
		{"GET", "/api/v1/moderation/candidate-actions", nil},
		{"GET", "/api/v1/imports/candidates/" + f.Company, nil},
		{"GET", "/api/v1/imports/candidates/" + f.Company + "/duplicates", nil},
		{"PATCH", "/api/v1/imports/candidates/" + f.Company, map[string]any{"expected_revision": 1, "changes": map[string]any{"name": "x"}}},
		{"GET", "/api/v1/moderation/correction-requests/" + f.Company, nil},
	}
	for _, route := range routes {
		for _, actor := range []string{"", f.Developer, f.Owner} {
			want := 403
			if actor == "" {
				want = 401
			}
			testStatus(t, testRequest(t, s, route.method, route.path, actor, route.body), want)
		}
	}
	s.Config.ImporterToken = "test-internal-token-at-least-32-characters"
	// Internal importer credentials must not grant moderation privileges.
	for _, route := range routes {
		req := testRequestWithImporter(t, s, route.method, route.path, route.body)
		testStatus(t, req, 403)
	}
}
func TestIntegrationModerationDraftRevisionAndAudit(t *testing.T) {
	s, pool, f := dbTest(t)
	id := moderationFixture(t, pool, f)
	path := "/api/v1/imports/candidates/" + id
	before := testRequest(t, s, "GET", path, f.Moderator, nil)
	testStatus(t, before, 200)
	initial := testObject(t, before)
	if initial["revision"] != float64(1) {
		t.Fatal("wrong initial revision")
	}
	update := map[string]any{"expected_revision": 1, "reason": "Checked contacts", "changes": map[string]any{"name": "Reviewed " + id, "category_id": f.Category, "phone": nil, "website_url": "https://company.example", "website_status": "found"}}
	response := testRequest(t, s, "PATCH", path, f.Moderator, update)
	testStatus(t, response, 200)
	edited := testObject(t, response)
	if edited["revision"] != float64(2) {
		t.Fatal("revision not incremented")
	}
	proposed := edited["proposed_company"].(map[string]any)
	if proposed["phone"] != nil || proposed["category_id"] != f.Category || proposed["category_name"] != nil {
		t.Fatal("draft null/category semantics lost")
	}
	if edited["payload"].(map[string]any)["name"] != initial["payload"].(map[string]any)["name"] {
		t.Fatal("source overwritten")
	}
	testStatus(t, testRequest(t, s, "PATCH", path, f.Moderator, update), 409)
	query := "/api/v1/imports/candidates?q=" + url.QueryEscape("Reviewed "+id) + "&city=" + url.QueryEscape("Астана") + "&website_status=found&limit=1"
	response = testRequest(t, s, "GET", query, f.Moderator, nil)
	testStatus(t, response, 200)
	if testObject(t, response)["total"] != float64(1) {
		t.Fatal("effective draft filters ignored")
	}
	for _, filter := range []string{"category_id=" + f.Category, "category=" + url.QueryEscape("Integration category "+f.Category)} {
		result := testRequest(t, s, "GET", "/api/v1/imports/candidates?q="+url.QueryEscape("Reviewed "+id)+"&"+filter, f.Moderator, nil)
		testStatus(t, result, 200)
		if testObject(t, result)["total"] != float64(1) {
			t.Fatal("edited category filter ignored")
		}
	}
	// A collector refresh changes the revision but never resets moderator edits.
	if _, err := pool.Exec(context.Background(), `UPDATE importer.company_candidates SET payload=jsonb_set(payload,'{name}','"New source name"') WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	testStatus(t, testRequest(t, s, "POST", path+"/review", f.Moderator, map[string]any{"decision": "approve", "expected_revision": 2}), 409)
	response = testRequest(t, s, "POST", path+"/review", f.Moderator, map[string]any{"decision": "approve", "expected_revision": 3, "reason": "Verified manually"})
	testStatus(t, response, 200)
	cid := testObject(t, response)["company_id"].(string)
	var name, visibility, status string
	var phone *string
	if err := pool.QueryRow(context.Background(), `SELECT name,visibility,phone,website_status FROM catalog.companies WHERE id=$1`, cid).Scan(&name, &visibility, &phone, &status); err != nil || name != "Reviewed "+id || visibility != "hidden" || phone != nil || status != "found" {
		t.Fatal("approved company lost draft", err)
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/companies/"+cid, "", nil), 404)
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/companies/"+cid+"/visibility", f.Moderator, map[string]any{"visibility": "published", "reason": "Ready for catalog"}), 200)
	testStatus(t, testRequest(t, s, "GET", "/api/v1/companies/"+cid, "", nil), 200)
	response = testRequest(t, s, "GET", "/api/v1/moderation/candidate-actions?candidate_id="+id, f.Moderator, nil)
	testStatus(t, response, 200)
	events := testObject(t, response)
	if events["total"] != float64(2) {
		t.Fatal("missing edit/approval events")
	}
	testStatus(t, testRequest(t, s, "PATCH", path, f.Moderator, map[string]any{"expected_revision": 4, "changes": map[string]any{"name": "Too late"}}), 409)
}
func TestIntegrationModerationRejectAndLink(t *testing.T) {
	s, pool, f := dbTest(t)
	id := moderationFixture(t, pool, f)
	path := "/api/v1/imports/candidates/" + id
	response := testRequest(t, s, "POST", path+"/review", f.Moderator, map[string]any{"decision": "reject", "expected_revision": 1, "reason": "Not a business"})
	testStatus(t, response, 200)
	if testObject(t, response)["company_id"] != nil {
		t.Fatal("rejection created company")
	}
	response = testRequest(t, s, "GET", "/api/v1/moderation/candidate-actions?candidate_id="+id+"&action=reject", f.Moderator, nil)
	testStatus(t, response, 200)
	if testObject(t, response)["total"] != float64(1) {
		t.Fatal("rejection not audited")
	}
	id = moderationFixture(t, pool, f)
	path = "/api/v1/imports/candidates/" + id
	var original []byte
	if err := pool.QueryRow(context.Background(), `SELECT to_jsonb(c) FROM catalog.companies c WHERE id=$1`, f.Company).Scan(&original); err != nil {
		t.Fatal(err)
	}
	response = testRequest(t, s, "POST", path+"/review", f.Moderator, map[string]any{"decision": "link", "expected_revision": 1, "company_id": f.Company, "reason": "Same branch"})
	testStatus(t, response, 200)
	var after []byte
	if err := pool.QueryRow(context.Background(), `SELECT to_jsonb(c) FROM catalog.companies c WHERE id=$1`, f.Company).Scan(&after); err != nil || string(original) != string(after) {
		t.Fatal("link overwrote existing company")
	}
	response = testRequest(t, s, "GET", "/api/v1/moderation/summary", f.Moderator, nil)
	testStatus(t, response, 200)
	if testObject(t, response)["rejected_candidates"].(float64) < 1 || testObject(t, response)["approved_candidates"].(float64) < 1 {
		t.Fatal("summary counts missing")
	}
}
func TestIntegrationModerationConcurrentReview(t *testing.T) {
	s, pool, f := dbTest(t)
	id := moderationFixture(t, pool, f)
	path := "/api/v1/imports/candidates/" + id + "/review"
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			codes <- testRequest(t, s, "POST", path, f.Moderator, map[string]any{"decision": "approve", "expected_revision": 1}).Code
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	got := map[int]int{}
	for code := range codes {
		got[code]++
	}
	if got[200] != 1 || got[409] != 1 {
		t.Fatalf("concurrent decisions: %v", got)
	}
}

func TestIntegrationModerationConcurrentDuplicatesAndHistoryACL(t *testing.T) {
	s, pool, f := dbTest(t)
	ids := []string{moderationFixture(t, pool, f), moderationFixture(t, pool, f)}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	start := make(chan struct{})
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			codes <- testRequest(t, s, "POST", "/api/v1/imports/candidates/"+id+"/review", f.Moderator, map[string]any{"decision": "approve", "expected_revision": 1}).Code
		}(id)
	}
	close(start)
	wg.Wait()
	close(codes)
	results := map[int]int{}
	for code := range codes {
		results[code]++
	}
	if results[200] != 1 || results[409] != 1 {
		t.Fatalf("duplicate approvals raced: %v", results)
	}
	var allowed bool
	for _, role := range []string{"anon", "authenticated"} {
		if err := pool.QueryRow(context.Background(), `SELECT has_table_privilege($1,'importer.candidate_actions','SELECT')`, role).Scan(&allowed); err != nil || allowed {
			t.Fatal("public history access", err)
		}
	}
	if err := pool.QueryRow(context.Background(), `SELECT has_table_privilege('firstorder_backend','importer.candidate_actions','UPDATE') OR has_table_privilege('firstorder_backend','importer.candidate_actions','DELETE')`).Scan(&allowed); err != nil || allowed {
		t.Fatal("runtime can rewrite audit history", err)
	}
}
func TestIntegrationModerationValidationAndCorrections(t *testing.T) {
	s, pool, f := dbTest(t)
	id := moderationFixture(t, pool, f)
	path := "/api/v1/imports/candidates/" + id
	for _, body := range []map[string]any{
		{"changes": map[string]any{"name": "No revision"}},
		{"expected_revision": 1, "changes": map[string]any{"payload": map[string]string{"role": "moderator"}}},
		{"expected_revision": 1, "changes": map[string]any{"website_status": "found"}},
		{"expected_revision": 1, "changes": map[string]any{"category_id": f.Category, "category_name": "Ambiguous"}},
		{"expected_revision": 1, "changes": map[string]any{"category_id": newID()}},
	} {
		testStatus(t, testRequest(t, s, "PATCH", path, f.Moderator, body), 422)
	}
	for _, query := range []string{"status=bad", "website_status=bad", "source=bad", "company_id=bad", "limit=101"} {
		r := testRequest(t, s, "GET", "/api/v1/imports/candidates?"+query, f.Moderator, nil)
		if r.Code != 400 && r.Code != 422 {
			t.Fatalf("invalid query accepted: %s %d", query, r.Code)
		}
	}
	response := testRequest(t, s, "POST", "/api/v1/companies/"+f.Company+"/correction-requests", f.Developer, map[string]any{"message": "Please correct the company phone"})
	testStatus(t, response, 201)
	request := testObject(t, response)["id"].(string)
	response = testRequest(t, s, "GET", "/api/v1/moderation/correction-requests/"+request, f.Moderator, nil)
	testStatus(t, response, 200)
	if testObject(t, response)["current_company"].(map[string]any)["id"] != f.Company {
		t.Fatal("correction context missing")
	}
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/moderation/correction-requests/"+request, f.Moderator, map[string]any{"status": "approved", "changes": map[string]any{"phone": "+70000000000"}, "resolution_note": "Checked company contacts"}), 200)
}
