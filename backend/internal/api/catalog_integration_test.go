package api

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestIntegrationCatalogVisibilityAndFilters(t *testing.T) {
	s, pool, f := dbTest(t)
	for _, actor := range []string{"", f.Developer} {
		testStatus(t, testRequest(t, s, "GET", "/api/v1/companies/"+f.HiddenCompany, actor, nil), 404)
	}
	for _, actor := range []string{f.Owner, f.Moderator} {
		testStatus(t, testRequest(t, s, "GET", "/api/v1/companies/"+f.HiddenCompany, actor, nil), 200)
	}
	result := testRequest(t, s, "GET", "/api/v1/companies?category_id="+f.Category, "", nil)
	testStatus(t, result, 200)
	obj := testObject(t, result)
	if obj["total"] != float64(1) {
		t.Fatalf("public list leaked hidden company: %s", result.Body.String())
	}
	items := obj["items"].([]any)
	if items[0].(map[string]any)["id"] != f.Company {
		t.Fatal("wrong public company")
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/companies?visibility=hidden", f.Developer, nil), 403)
	result = testRequest(t, s, "GET", "/api/v1/companies?visibility=hidden&category_id="+f.Category, f.Moderator, nil)
	testStatus(t, result, 200)
	if testObject(t, result)["total"] != float64(1) {
		t.Fatal("moderator cannot list hidden fixture company")
	}
	category := "Integration category " + f.Category
	result = testRequest(t, s, "GET", "/api/v1/companies?category="+url.QueryEscape(category), "", nil)
	testStatus(t, result, 200)
	if testObject(t, result)["total"] != float64(1) {
		t.Fatal("category name filter ignored")
	}
	result = testRequest(t, s, "GET", "/api/v1/companies?category="+url.QueryEscape("unknown-"+f.Category), "", nil)
	testStatus(t, result, 200)
	if testObject(t, result)["total"] != float64(0) {
		t.Fatal("nonexistent category name must not return unfiltered catalog")
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/companies?sort="+url.QueryEscape("name;DROP TABLE catalog.companies"), "", nil), 422)
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM catalog.companies WHERE id=$1`, f.Company).Scan(&count); err != nil || count != 1 {
		t.Fatalf("catalog changed during filter validation: %d %v", count, err)
	}
}

func TestIntegrationCompanyOwnershipNullPatchAndAudit(t *testing.T) {
	s, pool, f := dbTest(t)
	path := "/api/v1/companies/" + f.Company
	testStatus(t, testRequest(t, s, "PATCH", path, f.OtherDeveloper, map[string]any{"name": "stolen"}), 403)
	testStatus(t, testRequest(t, s, "PATCH", path, f.Owner, map[string]any{"visibility": "hidden"}), 422)
	result := testRequest(t, s, "PATCH", path, f.Owner, map[string]any{"phone": nil})
	testStatus(t, result, 200)
	company := testObject(t, result)
	if company["phone"] != nil || company["city"] != "Test City" {
		t.Fatalf("PATCH lost null/omission semantics: %s", result.Body.String())
	}
	testStatus(t, testRequest(t, s, "PATCH", path, f.Moderator, map[string]any{"phone": "verified-phone"}), 200)
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM catalog.moderation_actions WHERE company_id=$1 AND action='correct'`, f.Company).Scan(&count); err != nil || count != 1 {
		t.Fatalf("moderation update not audited: %d %v", count, err)
	}
	testStatus(t, testRequest(t, s, "PATCH", path+"/visibility", f.Owner, map[string]any{"visibility": "hidden"}), 403)
	testStatus(t, testRequest(t, s, "PATCH", path+"/visibility", f.Moderator, map[string]any{"visibility": "hidden", "reason": "integration test"}), 200)
	testStatus(t, testRequest(t, s, "GET", path, "", nil), 404)
	testStatus(t, testRequest(t, s, "GET", path, f.Owner, nil), 200)
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM catalog.moderation_actions WHERE company_id=$1 AND action='hide'`, f.Company).Scan(&count); err != nil || count != 1 {
		t.Fatalf("visibility change not audited: %d %v", count, err)
	}
	testStatus(t, testRequest(t, s, "PATCH", path+"/visibility", f.Moderator, map[string]any{"visibility": "published"}), 200)
	testStatus(t, testRequest(t, s, "GET", path, "", nil), 200)
}

func TestIntegrationFavoritesIdempotencyAndIsolation(t *testing.T) {
	s, pool, f := dbTest(t)
	path := "/api/v1/me/favorites/" + f.Company
	testStatus(t, testRequest(t, s, "PUT", path, "", nil), 401)
	testStatus(t, testRequest(t, s, "PUT", path, f.Moderator, nil), 403)
	for i := 0; i < 2; i++ {
		testStatus(t, testRequest(t, s, "PUT", path, f.Developer, nil), 200)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM catalog.favorites WHERE developer_id=$1 AND company_id=$2`, f.Developer, f.Company).Scan(&count); err != nil || count != 1 {
		t.Fatalf("favorite PUT duplicated rows: %d %v", count, err)
	}
	own := testRequest(t, s, "GET", "/api/v1/me/favorites", f.Developer, nil)
	testStatus(t, own, 200)
	if testObject(t, own)["total"] != float64(1) {
		t.Fatal("favorite missing")
	}
	other := testRequest(t, s, "GET", "/api/v1/me/favorites", f.OtherDeveloper, nil)
	testStatus(t, other, 200)
	if testObject(t, other)["total"] != float64(0) {
		t.Fatal("favorites leaked to another developer")
	}
	testStatus(t, testRequest(t, s, "PUT", "/api/v1/me/favorites/"+f.HiddenCompany, f.Developer, nil), 404)
	for i := 0; i < 2; i++ {
		testStatus(t, testRequest(t, s, "DELETE", path, f.Developer, nil), 204)
	}
}

func TestIntegrationContactUpsertPreservesOtherFields(t *testing.T) {
	s, _, f := dbTest(t)
	path := "/api/v1/me/company-contacts/" + f.Company
	first := testRequest(t, s, "PUT", path, f.Developer, map[string]any{"status": "contacted", "notes": "original", "contacted_at": "2026-11-19T10:00:00Z"})
	testStatus(t, first, 200)
	second := testRequest(t, s, "PUT", path, f.Developer, map[string]any{"notes": nil})
	testStatus(t, second, 200)
	body := testObject(t, second)
	if body["status"] != "contacted" || body["notes"] != nil || body["contacted_at"] == nil {
		t.Fatalf("contact upsert reset omitted fields: %s", second.Body.String())
	}
	testStatus(t, testRequest(t, s, "GET", path, f.OtherDeveloper, nil), 404)
	testStatus(t, testRequest(t, s, "PUT", path, f.Developer, map[string]any{"developer_id": f.OtherDeveloper, "status": "replied"}), 422)
	testStatus(t, testRequest(t, s, "PUT", path, f.Developer, map[string]any{"status": "accepted"}), 422)
	result := testRequest(t, s, "GET", "/api/v1/me/company-contacts", f.Developer, nil)
	testStatus(t, result, 200)
	if testObject(t, result)["total"] != float64(1) {
		t.Fatal("contact upsert duplicated rows")
	}
	for i := 0; i < 2; i++ {
		testStatus(t, testRequest(t, s, "DELETE", path, f.Developer, nil), 204)
	}
}

func TestIntegrationCorrectionAtomicApplyRollbackAndConflict(t *testing.T) {
	s, pool, f := dbTest(t)
	created := testRequest(t, s, "POST", "/api/v1/companies/"+f.Company+"/correction-requests", f.Developer, map[string]any{"message": "The phone needs changing; do not parse this message into data."})
	testStatus(t, created, 201)
	id := testObject(t, created)["id"].(string)
	path := "/api/v1/moderation/correction-requests/" + id
	testStatus(t, testRequest(t, s, "PATCH", path, f.Developer, map[string]any{"status": "approved"}), 403)
	// An invalid FK aborts the entire transaction: request and card stay unchanged.
	testStatus(t, testRequest(t, s, "PATCH", path, f.Moderator, map[string]any{"status": "approved", "changes": map[string]any{"phone": "must-rollback", "category_id": newID()}}), 409)
	var status, phone string
	if err := pool.QueryRow(context.Background(), `SELECT r.status,c.phone FROM catalog.correction_requests r JOIN catalog.companies c ON c.id=r.company_id WHERE r.id=$1`, id).Scan(&status, &phone); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || phone != "old-phone" {
		t.Fatalf("failed correction partially committed: %s %s", status, phone)
	}
	accepted := testRequest(t, s, "PATCH", path, f.Moderator, map[string]any{"status": "approved", "resolution_note": "checked", "changes": map[string]any{"phone": "new-phone"}})
	testStatus(t, accepted, 200)
	if err := pool.QueryRow(context.Background(), `SELECT r.status,c.phone FROM catalog.correction_requests r JOIN catalog.companies c ON c.id=r.company_id WHERE r.id=$1`, id).Scan(&status, &phone); err != nil {
		t.Fatal(err)
	}
	if status != "approved" || phone != "new-phone" {
		t.Fatalf("correction not atomically applied: %s %s", status, phone)
	}
	repeat := testRequest(t, s, "PATCH", path, f.Moderator, map[string]any{"status": "rejected"})
	testStatus(t, repeat, 409)
	if !strings.Contains(repeat.Body.String(), "CORRECTION_ALREADY_RESOLVED") {
		t.Fatalf("missing stable conflict code: %s", repeat.Body.String())
	}
	var actor string
	if err := pool.QueryRow(context.Background(), `SELECT moderator_id::text FROM catalog.moderation_actions WHERE company_id=$1 AND action='approve'`, f.Company).Scan(&actor); err != nil || actor != f.Moderator {
		t.Fatalf("approval audit actor incorrect: %s %v", actor, err)
	}
	own := testRequest(t, s, "GET", "/api/v1/me/correction-requests", f.Developer, nil)
	testStatus(t, own, 200)
	if testObject(t, own)["total"] != float64(1) {
		t.Fatal("own correction missing")
	}
	other := testRequest(t, s, "GET", "/api/v1/me/correction-requests", f.OtherDeveloper, nil)
	testStatus(t, other, 200)
	if testObject(t, other)["total"] != float64(0) {
		t.Fatal("corrections leaked")
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/moderation/correction-requests?status=approved", f.Moderator, nil), 200)
	testStatus(t, testRequest(t, s, "GET", "/api/v1/moderation/actions", f.Developer, nil), 403)
	audit := testRequest(t, s, "GET", "/api/v1/moderation/actions?company_id="+f.Company+"&action=approve", f.Moderator, nil)
	testStatus(t, audit, 200)
	if testObject(t, audit)["total"] != float64(1) {
		t.Fatal("approval audit missing/duplicated")
	}
}

func TestIntegrationCategoryDeleteSetsCompanyReferenceNull(t *testing.T) {
	s, pool, f := dbTest(t)
	testStatus(t, testRequest(t, s, "GET", "/api/v1/categories", "", nil), 200)
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/categories/"+f.Category, f.Moderator, map[string]any{"name": "Renamed " + f.Category}), 200)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/categories", f.Moderator, map[string]any{"name": "Renamed " + f.Category}), 409)
	testStatus(t, testRequest(t, s, "DELETE", "/api/v1/categories/"+f.Category, f.Owner, nil), 403)
	testStatus(t, testRequest(t, s, "DELETE", "/api/v1/categories/"+f.Category, f.Moderator, nil), 204)
	var cleared bool
	if err := pool.QueryRow(context.Background(), `SELECT category_id IS NULL FROM catalog.companies WHERE id=$1`, f.Company).Scan(&cleared); err != nil || !cleared {
		t.Fatalf("category DELETE did not SET NULL: %t %v", cleared, err)
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/companies/"+f.Company, "", nil), 200)
}

func TestIntegrationLegacyCatalogAliasesActuallyRoute(t *testing.T) {
	s, _, f := dbTest(t)
	testStatus(t, testRequest(t, s, "POST", "/me/favorites/"+f.Company, f.Developer, nil), 200)
	result := testRequest(t, s, "PATCH", "/me/contacts/"+f.Company, f.Developer, map[string]any{"companyId": f.Company, "status": "Написал", "note": "legacy note", "date": "2026-11-19"})
	testStatus(t, result, 200)
	detail := testRequest(t, s, "GET", "/companies/"+f.Company, "", nil)
	testStatus(t, detail, 200)
	checkedAt, ok := testObject(t, detail)["checkedAt"].(string)
	if !ok || len(checkedAt) != 10 {
		t.Fatalf("legacy date must be YYYY-MM-DD for frontend: %v", checkedAt)
	}
}
