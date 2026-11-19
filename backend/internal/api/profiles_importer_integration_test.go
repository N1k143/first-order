package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationPortfolioForeignOwnership(t *testing.T) {
	s, pool, f := dbTest(t)
	created := testRequest(t, s, "POST", "/api/v1/me/portfolio", f.Developer, map[string]any{"title": "Private edit ownership", "description": "keep description", "project_url": "https://example.test/portfolio"})
	testStatus(t, created, 201)
	id := testObject(t, created)["id"].(string)
	path := "/api/v1/me/portfolio/" + id
	testStatus(t, testRequest(t, s, "PATCH", path, f.OtherDeveloper, map[string]any{"title": "stolen"}), 403)
	testStatus(t, testRequest(t, s, "DELETE", path, f.OtherDeveloper, nil), 403)
	var title string
	if err := pool.QueryRow(context.Background(), `SELECT title FROM profiles.portfolio_items WHERE id=$1`, id).Scan(&title); err != nil || title != "Private edit ownership" {
		t.Fatalf("foreign mutation changed portfolio: %q %v", title, err)
	}
	testStatus(t, testRequest(t, s, "PATCH", path, f.Developer, map[string]any{"title": "Owner changed"}), 200)
	testStatus(t, testRequest(t, s, "GET", "/api/v1/profiles/"+f.Developer, f.OtherDeveloper, nil), 200)
	testStatus(t, testRequest(t, s, "DELETE", path, f.Developer, nil), 204)
}

func TestIntegrationCompanyLastOwnerAndConcurrentRevocation(t *testing.T) {
	s, pool, f := dbTest(t)
	base := "/api/v1/companies/" + f.Company + "/members"
	testStatus(t, testRequest(t, s, "DELETE", base+"/"+f.Owner, f.Owner, nil), 409)
	testStatus(t, testRequest(t, s, "POST", base, f.Owner, map[string]any{"user_id": f.OtherDeveloper, "member_role": "owner"}), 201)
	start := make(chan struct{})
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for _, actor := range []string{f.Owner, f.OtherDeveloper} {
		actor := actor
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := testRequest(t, s, "DELETE", base+"/"+actor, actor, nil)
			codes <- rec.Code
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	success, conflict := 0, 0
	for status := range codes {
		switch status {
		case 204:
			success++
		case 409:
			conflict++
		default:
			t.Fatalf("unexpected concurrent owner result: %d", status)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("two self-removals must retain one owner: success=%d conflict=%d", success, conflict)
	}
	var remaining string
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM profiles.company_members WHERE company_id=$1 AND member_role='owner'`, f.Company).Scan(&count); err != nil || count != 1 {
		t.Fatalf("last owner invariant lost: %d %v", count, err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT user_id::text FROM profiles.company_members WHERE company_id=$1 AND member_role='owner'`, f.Company).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	testStatus(t, testRequest(t, s, "POST", base, remaining, map[string]any{"user_id": f.Developer, "member_role": "representative"}), 201)
	start = make(chan struct{})
	results := make(chan struct {
		kind   string
		status int
	}, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		rec := testRequest(t, s, "DELETE", base+"/"+f.Developer, remaining, nil)
		results <- struct {
			kind   string
			status int
		}{"revoke", rec.Code}
	}()
	go func() {
		defer wg.Done()
		<-start
		rec := testRequest(t, s, "PATCH", "/api/v1/companies/"+f.Company, f.Developer, map[string]any{"phone": "authorized-before-revocation"})
		results <- struct {
			kind   string
			status int
		}{"edit", rec.Code}
	}()
	close(start)
	wg.Wait()
	close(results)
	for result := range results {
		if result.kind == "revoke" && result.status != 204 {
			t.Fatalf("revocation failed: %d", result.status)
		}
		if result.kind == "edit" && result.status != 200 && result.status != 403 {
			t.Fatalf("edit should serialize before revoke or be forbidden: %d", result.status)
		}
	}
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/companies/"+f.Company, f.Developer, map[string]any{"phone": "must-never-apply"}), 403)
	var phone string
	if err := pool.QueryRow(context.Background(), `SELECT phone FROM catalog.companies WHERE id=$1`, f.Company).Scan(&phone); err != nil || phone == "must-never-apply" {
		t.Fatalf("revoked actor changed company: %q %v", phone, err)
	}
}

// Import workers claim a shared queue. Serialize these test cases across test
// processes as well as goroutines without truncating or taking unrelated runs.
func testImportQueue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(492019761)`); err != nil {
		conn.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := conn.Exec(ctx, `SELECT pg_advisory_unlock(492019761)`)
		conn.Release()
		if err != nil {
			t.Errorf("release test queue lock: %v", err)
		}
	})
	var pending int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM importer.import_runs WHERE status='pending'`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatal("dedicated test import queue contains unrelated pending runs; refusing to consume them")
	}
}
func testImportCleanup(t *testing.T, pool *pgxpool.Pool, runIDs *[]string, externalIDs []string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Errorf("import cleanup begin: %v", err)
			return
		}
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `WITH removed AS (DELETE FROM importer.source_records WHERE source='demo_csv' AND external_id=ANY($1::text[]) RETURNING company_id) DELETE FROM catalog.companies WHERE id IN(SELECT company_id FROM removed WHERE company_id IS NOT NULL)`, externalIDs)
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM importer.import_runs WHERE id=ANY($1::uuid[])`, *runIDs)
		}
		if err != nil {
			t.Errorf("import fixture cleanup: %v", err)
			return
		}
		if err = tx.Commit(ctx); err != nil {
			t.Errorf("import cleanup commit: %v", err)
		}
	})
}
func testWriteCSV(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}
func testRunImport(t *testing.T, s *Server, actor string, runIDs *[]string) string {
	t.Helper()
	res := testRequest(t, s, "POST", "/api/v1/imports", actor, map[string]any{"source": "demo_csv"})
	testStatus(t, res, 202)
	id := testObject(t, res)["run_id"].(string)
	*runIDs = append(*runIDs, id)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	found, err := s.processNextImport(ctx)
	if err != nil || !found {
		t.Fatalf("worker did not finish queued run: found=%t error=%v", found, err)
	}
	return id
}
func TestIntegrationImporterRepeatCSVIsDeduplicated(t *testing.T) {
	s, pool, f := dbTest(t)
	testImportQueue(t, pool)
	prefix := newID()
	extIDs := []string{prefix + "-one", prefix + "-two"}
	runs := []string{}
	testImportCleanup(t, pool, &runs, extIDs)
	path := filepath.Join(t.TempDir(), "companies.csv")
	s.Config.ImportCSVPath = path
	csv := fmt.Sprintf("external_id,name,category_id,phone\n%s,First imported,%s,+77000000001\n%s,Second imported,%s,+77000000002\n", extIDs[0], f.Category, extIDs[1], f.Category)
	testWriteCSV(t, path, csv)
	first := testRunImport(t, s, f.Moderator, &runs)
	var count int
	var company string
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM importer.source_records WHERE source='demo_csv' AND external_id=ANY($1::text[])`, extIDs).Scan(&count); err != nil || count != 2 {
		t.Fatalf("first run records=%d error=%v", count, err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT company_id::text FROM importer.source_records WHERE source='demo_csv' AND external_id=$1`, extIDs[0]).Scan(&company); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE catalog.companies SET visibility='hidden' WHERE id=$1`, company); err != nil {
		t.Fatal(err)
	}
	csv = fmt.Sprintf("external_id,name,category_id,phone\n%s,First refreshed,%s,+77000000003\n%s,Second refreshed,%s,+77000000004\n", extIDs[0], f.Category, extIDs[1], f.Category)
	testWriteCSV(t, path, csv)
	second := testRunImport(t, s, f.Moderator, &runs)
	if second == first {
		t.Fatal("each import must have a new run ID")
	}
	var latest, linked, name, visibility, phone string
	if err := pool.QueryRow(context.Background(), `SELECT s.last_run_id::text,s.company_id::text,c.name,c.visibility,c.phone FROM importer.source_records s JOIN catalog.companies c ON c.id=s.company_id WHERE s.source='demo_csv' AND s.external_id=$1`, extIDs[0]).Scan(&latest, &linked, &name, &visibility, &phone); err != nil {
		t.Fatal(err)
	}
	if latest != second || linked != company || name != "First refreshed" || visibility != "hidden" || phone != "+77000000003" {
		t.Fatalf("repeat import duplicates/re-publishes/loses update: %s %s %s %s %s", latest, linked, name, visibility, phone)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM importer.source_records WHERE source='demo_csv' AND external_id=ANY($1::text[])`, extIDs).Scan(&count); err != nil || count != 2 {
		t.Fatalf("repeat run duplicates source records: %d %v", count, err)
	}
	status := testRequest(t, s, "GET", "/api/v1/imports/"+second, f.Moderator, nil)
	testStatus(t, status, 200)
	body := testObject(t, status)
	if body["status"] != "completed" || body["processed_count"] != float64(2) {
		t.Fatalf("wrong completed run: %s", status.Body.String())
	}
}
func TestIntegrationImporterFailureRollsBackAllRecords(t *testing.T) {
	s, pool, f := dbTest(t)
	testImportQueue(t, pool)
	prefix := newID()
	extIDs := []string{prefix + "-1", prefix + "-2"}
	runs := []string{}
	testImportCleanup(t, pool, &runs, extIDs)
	path := filepath.Join(t.TempDir(), "fail.csv")
	s.Config.ImportCSVPath = path
	// CSV passes field validation. The second row's missing category fails at
	// the database after the first row has already been written in the savepoint.
	testWriteCSV(t, path, fmt.Sprintf("external_id,name,category_id\n%s,Rollback first,%s\n%s,Rollback second,%s\n", extIDs[0], f.Category, extIDs[1], newID()))
	id := testRunImport(t, s, f.Moderator, &runs)
	var status string
	var count, processed int
	var errorMessage *string
	if err := pool.QueryRow(context.Background(), `SELECT status,processed_count,error_message FROM importer.import_runs WHERE id=$1`, id).Scan(&status, &processed, &errorMessage); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || processed != 0 || errorMessage == nil {
		t.Fatalf("failed run not recorded safely: %s %d %v", status, processed, errorMessage)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM importer.source_records WHERE source='demo_csv' AND external_id=ANY($1::text[])`, extIDs).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed import retained partial source records: %d %v", count, err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM catalog.companies WHERE name IN ('Rollback first','Rollback second')`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed import retained partial companies: %d %v", count, err)
	}
}
func TestIntegrationImporterServiceIdentityIsNotHuman(t *testing.T) {
	s, pool, _ := dbTest(t)
	testImportQueue(t, pool)
	runs := []string{}
	testImportCleanup(t, pool, &runs, nil)
	s.Config.ImporterToken = "integration-internal-token"
	s.Config.ImportCSVPath = filepath.Join(t.TempDir(), "empty.csv")
	testWriteCSV(t, s.Config.ImportCSVPath, "external_id,name\n")
	call := func(method, path string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer integration-internal-token")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}
	rec := call("POST", "/api/v1/imports", []byte(`{"source":"demo_csv"}`))
	testStatus(t, rec, 202)
	runs = append(runs, testObject(t, rec)["run_id"].(string))
	testStatus(t, call("GET", "/api/v1/me", nil), 403)
	testStatus(t, call("GET", "/api/v1/imports", nil), 403)
	testStatus(t, call("PATCH", "/api/v1/me/profile", []byte(`{"name":"service must not edit user"}`)), 403)
	if found, err := s.processNextImport(context.Background()); err != nil || !found {
		t.Fatalf("service queued run not processed: %t %v", found, err)
	}
}

func TestIntegrationProfileAndPortfolioExplicitNull(t *testing.T) {
	s, _, f := dbTest(t)
	initial := testRequest(t, s, "PATCH", "/api/v1/me/profile", f.Developer, map[string]any{"name": "Nullable profile", "bio": "old bio", "skills": "Go", "github_url": "https://github.com/example"})
	testStatus(t, initial, 200)
	cleared := testRequest(t, s, "PATCH", "/api/v1/me/profile", f.Developer, map[string]any{"bio": nil, "github_url": nil})
	testStatus(t, cleared, 200)
	p := testObject(t, cleared)
	if p["bio"] != nil || p["github_url"] != nil || p["name"] != "Nullable profile" || p["skills"] != "Go" {
		t.Fatalf("profile null/omission lost: %s", cleared.Body.String())
	}
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/me/profile", f.Developer, map[string]any{"name": nil}), 422)
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/me/profile", f.Developer, map[string]any{"role": "moderator"}), 400)
	created := testRequest(t, s, "POST", "/api/v1/me/portfolio", f.Developer, map[string]any{"title": "Preserve title", "description": "old", "project_url": "https://example.test/work"})
	testStatus(t, created, 201)
	id := testObject(t, created)["id"].(string)
	cleared = testRequest(t, s, "PATCH", "/api/v1/me/portfolio/"+id, f.Developer, map[string]any{"description": nil, "project_url": nil})
	testStatus(t, cleared, 200)
	p = testObject(t, cleared)
	if p["title"] != "Preserve title" || p["description"] != nil || p["project_url"] != nil {
		t.Fatalf("portfolio null/omission lost: %s", cleared.Body.String())
	}
}
