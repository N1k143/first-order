package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"ingeneriya/backend/internal/discovery"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func collectionRequest(t *testing.T, s *Server, actor, key, kind string, limit int) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"kind": kind, "limit": limit})
	req := httptest.NewRequest("POST", "/api/v1/collection/jobs", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	if actor != "" {
		req.Header.Set("Authorization", "Bearer "+testToken(actor))
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}
func collectionCleanup(t *testing.T, pool *pgxpool.Pool, f testFixture) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM importer.collection_jobs WHERE created_by=$1`, f.Moderator); err != nil {
			t.Error(err)
		}
		pool.Exec(context.Background(), `UPDATE importer.collection_sources SET next_fetch_at=now()`)
	})
}
func collectionStart(t *testing.T, s *Server, f testFixture, kind string, limit int) string {
	t.Helper()
	rec := collectionRequest(t, s, f.Moderator, newID(), kind, limit)
	testStatus(t, rec, 202)
	return testObject(t, rec)["id"].(string)
}
func TestIntegrationCollectionAccessAndIdempotency(t *testing.T) {
	s, pool, f := dbTest(t)
	collectionCleanup(t, pool, f)
	for _, a := range []string{"", f.Developer, f.Owner} {
		want := 403
		if a == "" {
			want = 401
		}
		testStatus(t, collectionRequest(t, s, a, newID(), "collect", 2), want)
		for _, route := range []struct{ method, path string }{{"GET", "/api/v1/collection/jobs"}, {"GET", "/api/v1/collection/jobs/" + newID()}, {"POST", "/api/v1/collection/jobs/" + newID() + "/cancel"}, {"POST", "/api/v1/collection/jobs/" + newID() + "/retry"}} {
			testStatus(t, testRequest(t, s, route.method, route.path, a, nil), want)
		}
	}
	s.Config.ImporterToken = "collection-internal-at-least-32-characters"
	testStatus(t, testRequestWithImporter(t, s, "GET", "/api/v1/collection/jobs", nil), 403)
	testStatus(t, collectionRequest(t, s, f.Moderator, "", "collect", 2), 422)
	testStatus(t, collectionRequest(t, s, f.Moderator, newID(), "collect", 101), 422)
	testStatus(t, collectionRequest(t, s, f.Moderator, newID(), "arbitrary-provider", 2), 422)
	key := newID()
	first := collectionRequest(t, s, f.Moderator, key, "collect", 2)
	testStatus(t, first, 202)
	id := testObject(t, first)["id"].(string)
	repeat := collectionRequest(t, s, f.Moderator, key, "collect", 2)
	testStatus(t, repeat, 200)
	if testObject(t, repeat)["id"] != id {
		t.Fatal("idempotency lost")
	}
	testStatus(t, collectionRequest(t, s, f.Moderator, key, "collect", 3), 409)
	testStatus(t, collectionRequest(t, s, f.Moderator, newID(), "collect", 2), 409)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/collection/jobs/"+id+"/cancel", f.Moderator, nil), 202)
	rec := testRequest(t, s, "GET", "/api/v1/collection/jobs/"+id, f.Moderator, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["status"] != "canceled" {
		t.Fatal("queued cancellation failed")
	}
	if strings.Contains(rec.Body.String(), "lease_token") || strings.Contains(rec.Body.String(), "request_key") || strings.Contains(rec.Body.String(), `"items"`) {
		t.Fatal("private worker state exposed")
	}
}
func TestIntegrationCollectionProgressProtectionAndResume(t *testing.T) {
	s, pool, f := dbTest(t)
	collectionCleanup(t, pool, f)
	ids := []string{moderationFixture(t, pool, f), moderationFixture(t, pool, f), moderationFixture(t, pool, f)}
	var candidates []discovery.Candidate
	for _, id := range ids {
		var raw []byte
		if err := pool.QueryRow(context.Background(), `SELECT payload FROM importer.company_candidates WHERE id=$1`, id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var c discovery.Candidate
		json.Unmarshal(raw, &c)
		candidates = append(candidates, c)
	}
	// Both a manual draft and an approved/public-linked candidate must survive collection.
	pool.Exec(context.Background(), `UPDATE importer.company_candidates SET draft='{"name":"Manual"}' WHERE id=$1`, ids[1])
	pool.Exec(context.Background(), `UPDATE importer.company_candidates SET review_status='approved',company_id=$2 WHERE id=$1`, ids[2], f.Company)
	s.collectionFetch = func(context.Context, int) ([]discovery.Candidate, error) { return candidates, nil }
	jobID := collectionStart(t, s, f, "collect", 3)
	job, err := s.claimCollection(context.Background())
	if err != nil || job == nil {
		t.Fatalf("claim: %v", err)
	}
	// A second worker cannot claim an active unexpired lease.
	other, err := s.claimCollection(context.Background())
	if err != nil || other != nil {
		t.Fatalf("double claim: %v", err)
	}
	s.collectionCheck = func(_ context.Context, c discovery.Candidate) discovery.Candidate {
		c.Check.Reason = "Checked again"
		return c
	}
	// Simulate a committed first checkpoint and a crash before the second.
	job.Items = make([]collectionItem, len(candidates))
	for i, c := range candidates {
		job.Items[i] = collectionItem{Candidate: c}
	}
	raw, _ := json.Marshal(job.Items)
	pool.Exec(context.Background(), `UPDATE importer.collection_jobs SET items=$2,total=3 WHERE id=$1`, jobID, raw)
	if err = s.saveCollectionItem(context.Background(), job, 0, candidates[0]); err != nil {
		t.Fatal(err)
	}
	rec := testRequest(t, s, "GET", "/api/v1/collection/jobs/"+jobID, f.Moderator, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["processed"] != float64(1) || testObject(t, rec)["status"] != "running" {
		t.Fatal("progress not visible before completion")
	}
	pool.Exec(context.Background(), `UPDATE importer.collection_jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, jobID)
	resumed, err := s.claimCollection(context.Background())
	if err != nil || resumed == nil || resumed.Processed != 1 {
		t.Fatalf("recovery failed %v", err)
	}
	if err = s.saveCollectionItem(context.Background(), job, 1, candidates[1]); !errors.Is(err, errCollectionLease) {
		t.Fatalf("old worker was not fenced: %v", err)
	}
	if code := s.executeCollection(context.Background(), resumed); code != "" {
		t.Fatal(code)
	}
	var processed, saved, skipped int
	if err = pool.QueryRow(context.Background(), `SELECT processed,saved,skipped FROM importer.collection_jobs WHERE id=$1`, jobID).Scan(&processed, &saved, &skipped); err != nil {
		t.Fatal(err)
	}
	if processed != 3 || saved != 1 || skipped != 2 {
		t.Fatalf("checkpoint/protection: %d %d %d", processed, saved, skipped)
	}
	var manual, approved bool
	pool.QueryRow(context.Background(), `SELECT draft->>'name'='Manual' FROM importer.company_candidates WHERE id=$1`, ids[1]).Scan(&manual)
	pool.QueryRow(context.Background(), `SELECT review_status='approved' AND company_id=$2 FROM importer.company_candidates WHERE id=$1`, ids[2], f.Company).Scan(&approved)
	if !manual || !approved {
		t.Fatal("reviewer changes overwritten")
	}
}
func TestIntegrationCollectionRunningCancellation(t *testing.T) {
	s, pool, f := dbTest(t)
	collectionCleanup(t, pool, f)
	id := moderationFixture(t, pool, f)
	testStatus(t, collectionRequest(t, s, f.Moderator, newID(), "recheck", 1), 409)
	pool.Exec(context.Background(), `UPDATE importer.company_candidates SET collected_at=now()-interval '2 days' WHERE id=$1`, id)
	jobID := collectionStart(t, s, f, "recheck", 1)
	started := make(chan struct{})
	var once sync.Once
	s.collectionCheck = func(ctx context.Context, c discovery.Candidate) discovery.Candidate {
		once.Do(func() { close(started) })
		<-ctx.Done()
		return c
	}
	done := make(chan error, 1)
	go func() { done <- s.processCollection(context.Background()) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	testStatus(t, testRequest(t, s, "POST", "/api/v1/collection/jobs/"+jobID+"/cancel", f.Moderator, nil), 202)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("network operation not canceled")
	}
	rec := testRequest(t, s, "GET", "/api/v1/collection/jobs/"+jobID, f.Moderator, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["status"] != "canceled" || testObject(t, rec)["processed"] != float64(0) {
		t.Fatal("cancellation saved partial check")
	}
	var revision int64
	pool.QueryRow(context.Background(), `SELECT revision FROM importer.company_candidates WHERE id=$1`, id).Scan(&revision)
	if revision != 2 {
		t.Fatal("canceled candidate changed")
	}
}
func TestIntegrationCollectionFailureRetryAndCooldown(t *testing.T) {
	s, pool, f := dbTest(t)
	collectionCleanup(t, pool, f)
	s.collectionFetch = func(context.Context, int) ([]discovery.Candidate, error) {
		return nil, errors.New("private upstream details")
	}
	id := collectionStart(t, s, f, "collect", 1)
	if err := s.processCollection(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec := testRequest(t, s, "GET", "/api/v1/collection/jobs/"+id, f.Moderator, nil)
	testStatus(t, rec, 200)
	obj := testObject(t, rec)
	if obj["status"] != "failed" || obj["error_code"] != "SOURCE_UNAVAILABLE" || strings.Contains(rec.Body.String(), "private upstream") {
		t.Fatal("unsafe/missing failure")
	}
	testStatus(t, testRequest(t, s, "POST", "/api/v1/collection/jobs/"+id+"/retry", f.Moderator, nil), 202)
	job, err := s.claimCollection(context.Background())
	if err != nil || job != nil {
		t.Fatal("source cooldown ignored")
	}
	for attempt := 2; attempt <= 3; attempt++ {
		pool.Exec(context.Background(), `UPDATE importer.collection_sources SET next_fetch_at=now()`)
		pool.Exec(context.Background(), `UPDATE importer.collection_jobs SET available_at=now() WHERE id=$1`, id)
		if err = s.processCollection(context.Background()); err != nil {
			t.Fatal(err)
		}
		want := 202
		if attempt == 3 {
			want = 409
		}
		testStatus(t, testRequest(t, s, "POST", "/api/v1/collection/jobs/"+id+"/retry", f.Moderator, nil), want)
	}
}
func TestIntegrationCollectionRecheckRevision(t *testing.T) {
	s, pool, f := dbTest(t)
	collectionCleanup(t, pool, f)
	id := moderationFixture(t, pool, f)
	testStatus(t, collectionRequest(t, s, f.Moderator, newID(), "recheck", 1), 409)
	pool.Exec(context.Background(), `UPDATE importer.company_candidates SET collected_at=now()-interval '2 days' WHERE id=$1`, id)
	jobID := collectionStart(t, s, f, "recheck", 1)
	// Even an edit that clears the draft increments revision; snapshot cannot overwrite it.
	pool.Exec(context.Background(), `UPDATE importer.company_candidates SET draft='{}' WHERE id=$1`, id)
	s.collectionCheck = func(_ context.Context, c discovery.Candidate) discovery.Candidate { return c }
	if err := s.processCollection(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec := testRequest(t, s, "GET", "/api/v1/collection/jobs/"+jobID, f.Moderator, nil)
	testStatus(t, rec, 200)
	obj := testObject(t, rec)
	if obj["status"] != "completed" || obj["saved"] != float64(0) || obj["skipped"] != float64(1) {
		t.Fatal("revision conflict not protected")
	}
	if len(obj["results"].([]any)) != 1 {
		t.Fatal("per-company evidence missing")
	}
}

func TestIntegrationCollectionStagesNewWithoutPublishing(t *testing.T) {
	s, pool, f := dbTest(t)
	collectionCleanup(t, pool, f)
	ext := "node/" + newID()
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM importer.company_candidates WHERE external_id=$1`, ext)
	})
	c := discovery.Candidate{Source: "osm_astana", ExternalID: ext, Name: "New source company", City: "Астана", Check: discovery.WebsiteCheck{Status: "needs_review", Reason: "website_not_provided"}}
	s.collectionFetch = func(context.Context, int) ([]discovery.Candidate, error) { return []discovery.Candidate{c}, nil }
	s.collectionCheck = func(_ context.Context, c discovery.Candidate) discovery.Candidate { return c }
	var before, after int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM catalog.companies`).Scan(&before)
	id := collectionStart(t, s, f, "collect", 1)
	if err := s.processCollection(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec := testRequest(t, s, "GET", "/api/v1/collection/jobs/"+id, f.Moderator, nil)
	testStatus(t, rec, 200)
	obj := testObject(t, rec)
	if obj["status"] != "completed" || obj["saved"] != float64(1) || obj["total"] != float64(1) {
		t.Fatal("new candidate not saved")
	}
	var pending bool
	pool.QueryRow(context.Background(), `SELECT review_status='pending' AND company_id IS NULL FROM importer.company_candidates WHERE external_id=$1`, ext).Scan(&pending)
	pool.QueryRow(context.Background(), `SELECT count(*) FROM catalog.companies`).Scan(&after)
	if !pending || before != after {
		t.Fatal("automatic publication occurred")
	}
}

func TestIntegrationCollectionRuntimePrivilegesAndOperator(t *testing.T) {
	s, pool, f := dbTest(t)
	cfg := pool.Config().Copy()
	cfg.ConnConfig.RuntimeParams["role"] = "firstorder_backend"
	runtime, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	worker := New(runtime, s.Config)
	key := newID()
	item, status, err := worker.QueueCollection(context.Background(), "collect", 1, key, "")
	if err != nil || status != 202 {
		t.Fatalf("runtime enqueue %d %v", status, err)
	}
	id := item["id"].(string)
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM importer.collection_jobs WHERE id=$1`, id)
		pool.Exec(context.Background(), `UPDATE importer.collection_sources SET next_fetch_at=now()`)
	})
	if item["created_by"] != nil {
		t.Fatal("operator got fake profile")
	}
	_, status, err = worker.QueueCollection(context.Background(), "collect", 1, key, "")
	if err != nil || status != 200 {
		t.Fatal("operator replay failed")
	}
	worker.collectionFetch = func(context.Context, int) ([]discovery.Candidate, error) { return []discovery.Candidate{}, nil }
	if err = worker.processCollection(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec := testRequest(t, worker, "GET", "/api/v1/collection/jobs/"+id, f.Moderator, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["status"] != "completed" {
		t.Fatal("runtime worker failed")
	}
	// Browser roles have neither table access nor an RLS policy.
	for _, role := range []string{"anon", "authenticated"} {
		var allowed bool
		if err = pool.QueryRow(context.Background(), `SELECT has_table_privilege($1,'importer.collection_jobs','SELECT') OR has_table_privilege($1,'importer.collection_results','INSERT')`, role).Scan(&allowed); err != nil || allowed {
			t.Fatalf("browser access %s: %v", role, err)
		}
	}
}
