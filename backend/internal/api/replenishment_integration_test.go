package api

import (
	"context"
	"encoding/json"
	"ingeneriya/backend/internal/discovery"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func replenishmentTest(t *testing.T) (*Server, *pgxpool.Pool, testFixture) {
	t.Helper()
	s, p, f := dbTest(t)
	ctx := context.Background()
	started := time.Now().UTC()
	var original []byte
	if err := p.QueryRow(ctx, `SELECT to_jsonb(p) FROM importer.replenishment_policy p WHERE id`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := p.Exec(ctx, `UPDATE importer.replenishment_policy SET
  (enabled,minimum,target,batch_size,pending_limit,cooldown_hours,revision,refill_active,next_check_at,next_run_at,last_checked_at,last_reason,last_job_id,updated_at)=
  (SELECT enabled,minimum,target,batch_size,pending_limit,cooldown_hours,revision,refill_active,next_check_at,next_run_at,last_checked_at,last_reason,last_job_id,updated_at FROM jsonb_populate_record(NULL::importer.replenishment_policy,$1)) WHERE id`, original)
		if err != nil {
			t.Error(err)
		}
		if _, err = p.Exec(ctx, `DELETE FROM importer.replenishment_actions WHERE actor_id=$1`, f.Moderator); err != nil {
			t.Error(err)
		}
		if _, err = p.Exec(ctx, `DELETE FROM importer.collection_jobs WHERE request_key LIKE 'replenishment/%' AND created_at>=$1`, started); err != nil {
			t.Error(err)
		}
	})
	if _, err := p.Exec(ctx, `UPDATE importer.replenishment_policy SET enabled=true,minimum=1,target=4,batch_size=2,pending_limit=10,cooldown_hours=24,revision=1,refill_active=false,next_check_at=now(),next_run_at=now(),last_job_id=NULL WHERE id`); err != nil {
		t.Fatal(err)
	}
	collectionCleanup(t, p, f)
	return s, p, f
}

func replenishCheck(t *testing.T, s *Server, p *pgxpool.Pool, want string) {
	t.Helper()
	if _, err := p.Exec(context.Background(), `UPDATE importer.replenishment_policy SET next_check_at=now() WHERE id`); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduleReplenishment(context.Background()); err != nil {
		t.Fatal(err)
	}
	var reason string
	if err := p.QueryRow(context.Background(), `SELECT last_reason FROM importer.replenishment_policy WHERE id`).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != want {
		t.Fatalf("reason=%q want=%q", reason, want)
	}
}

func TestIntegrationReplenishmentThresholdHysteresisAndCounts(t *testing.T) {
	s, p, f := replenishmentTest(t)
	ctx := context.Background()
	// Other cities, hidden cards and known websites do not count as stock.
	p.Exec(ctx, `UPDATE catalog.companies SET city='Астана',website_status='found' WHERE id=ANY($1::uuid[])`, []string{f.Company, f.HiddenCompany})
	var available, pending int
	if err := p.QueryRow(ctx, replenishmentCounts).Scan(&available, &pending); err != nil || available != 0 || pending != 0 {
		t.Fatalf("counts %d/%d: %v", available, pending, err)
	}
	p.Exec(ctx, `UPDATE catalog.companies SET website_status='needs_review' WHERE id=$1`, f.Company)
	replenishCheck(t, s, p, "queued") // Exact minimum triggers.
	var id string
	var limit int
	if err := p.QueryRow(ctx, `SELECT id::text,requested_limit FROM importer.collection_jobs WHERE status='pending'`).Scan(&id, &limit); err != nil || limit != 2 {
		t.Fatalf("queue %d: %v", limit, err)
	}
	replenishCheck(t, s, p, "collection_active")
	p.Exec(ctx, `UPDATE importer.collection_jobs SET status='completed',saved=2,finished_at=now() WHERE id=$1`, id)
	replenishCheck(t, s, p, "cooldown")
	// Above minimum but below target: refill latch survives server recreation.
	p.Exec(ctx, `UPDATE catalog.companies SET visibility='published',website_status='needs_review' WHERE id=$1`, f.HiddenCompany)
	p.Exec(ctx, `UPDATE importer.replenishment_policy SET next_run_at=now() WHERE id`)
	other := New(p, s.Config)
	replenishCheck(t, other, p, "queued")
	p.Exec(ctx, `UPDATE importer.collection_jobs SET status='completed',saved=2,finished_at=now() WHERE status='pending'`)
	p.Exec(ctx, `UPDATE importer.replenishment_policy SET target=2,minimum=1,next_run_at=now() WHERE id`)
	replenishCheck(t, s, p, "stock_sufficient")
	var latch bool
	p.QueryRow(ctx, `SELECT refill_active FROM importer.replenishment_policy WHERE id`).Scan(&latch)
	if latch {
		t.Fatal("target did not reset hysteresis")
	}
	// Counted cards remain unchanged by the scheduler.
	p.QueryRow(ctx, replenishmentCounts).Scan(&available, &pending)
	if available != 2 {
		t.Fatal("scheduler changed public catalog")
	}
}

func TestIntegrationReplenishmentBacklogAndLimits(t *testing.T) {
	s, p, f := replenishmentTest(t)
	ids := []string{}
	for range 4 {
		ids = append(ids, moderationFixture(t, p, f))
	}
	replenishCheck(t, s, p, "awaiting_moderation")
	p.Exec(context.Background(), `UPDATE importer.replenishment_policy SET pending_limit=4 WHERE id`)
	replenishCheck(t, s, p, "pending_limit")
	// Reviewed/rejected cards no longer count toward the pending buffer.
	p.Exec(context.Background(), `UPDATE importer.company_candidates SET review_status='rejected' WHERE id=ANY($1::uuid[])`, ids[:3])
	p.Exec(context.Background(), `UPDATE importer.replenishment_policy SET batch_size=100,pending_limit=2 WHERE id`)
	replenishCheck(t, s, p, "queued")
	var limit int
	if err := p.QueryRow(context.Background(), `SELECT requested_limit FROM importer.collection_jobs WHERE status='pending'`).Scan(&limit); err != nil || limit != 1 {
		t.Fatalf("pending cap exceeded: %d %v", limit, err)
	}
}

func TestIntegrationReplenishmentConcurrencyAndSourceDelay(t *testing.T) {
	s, p, _ := replenishmentTest(t)
	p.Exec(context.Background(), `UPDATE importer.collection_sources SET next_fetch_at=now()+interval '1 hour'`)
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 12 {
		wg.Go(func() { errs <- s.scheduleReplenishment(context.Background()) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	var delayed bool
	if err := p.QueryRow(context.Background(), `SELECT count(*),bool_and(available_at>now()+interval '59 minutes') FROM importer.collection_jobs WHERE status='pending'`).Scan(&count, &delayed); err != nil || count != 1 || !delayed {
		t.Fatalf("concurrent queue: %d delay=%v %v", count, delayed, err)
	}
	// Ordinary operator queue uses the same lock and cannot race an auto job.
	if _, status, err := s.QueueCollection(context.Background(), "collect", 2, newID(), ""); err == nil || status != 0 {
		t.Fatalf("manual queue bypassed active job: %v", err)
	}
}

func TestIntegrationReplenishmentFailureBackoff(t *testing.T) {
	s, p, _ := replenishmentTest(t)
	ctx := context.Background()
	for n, wantHours := range []int{48, 96, 168} {
		id := newID()
		if _, err := p.Exec(ctx, `INSERT INTO importer.collection_jobs(id,kind,requested_limit,request_key,status,finished_at,created_at,saved) VALUES($1,'collect',2,$2,$3,now(),now()+make_interval(secs=>$4),$5)`, id, "replenishment/"+id, []string{"failed", "completed", "failed"}[n], float64(n), []int{1, 0, 1}[n]); err != nil {
			t.Fatal(err)
		}
		replenishCheck(t, s, p, "cooldown")
		var hours float64
		if err := p.QueryRow(ctx, `SELECT extract(epoch FROM next_run_at-now())/3600 FROM importer.replenishment_policy WHERE id`).Scan(&hours); err != nil || hours < float64(wantHours)-0.01 || hours > float64(wantHours)+0.01 {
			t.Fatalf("backoff %d got %.2f: %v", wantHours, hours, err)
		}
	}
}

func TestIntegrationReplenishmentCollectsOnlyToModeration(t *testing.T) {
	s, p, f := replenishmentTest(t)
	ctx := context.Background()
	id := moderationFixture(t, p, f)
	var raw []byte
	if err := p.QueryRow(ctx, `SELECT payload FROM importer.company_candidates WHERE id=$1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var candidate discovery.Candidate
	if err := json.Unmarshal(raw, &candidate); err != nil {
		t.Fatal(err)
	}
	candidate.ExternalID = "node/" + newID()
	t.Cleanup(func() {
		p.Exec(ctx, `DELETE FROM importer.company_candidates WHERE source='osm_astana' AND external_id=$1`, candidate.ExternalID)
	})
	s.collectionFetch = func(context.Context, int) ([]discovery.Candidate, error) {
		return []discovery.Candidate{candidate}, nil
	}
	s.collectionCheck = func(_ context.Context, c discovery.Candidate) discovery.Candidate { return c }
	replenishCheck(t, s, p, "queued")
	if err := s.processCollection(ctx); err != nil {
		t.Fatal(err)
	}
	var status, review string
	var saved int
	var linked *string
	if err := p.QueryRow(ctx, `SELECT status,saved FROM importer.collection_jobs WHERE request_key LIKE 'replenishment/%' ORDER BY created_at DESC LIMIT 1`).Scan(&status, &saved); err != nil || status != "completed" || saved != 1 {
		t.Fatalf("worker result %s %d %v", status, saved, err)
	}
	if err := p.QueryRow(ctx, `SELECT review_status,company_id::text FROM importer.company_candidates WHERE external_id=$1`, candidate.ExternalID).Scan(&review, &linked); err != nil || review != "pending" || linked != nil {
		t.Fatalf("unexpected automatic publication: %s %v %v", review, linked, err)
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/collection/replenishment", f.Moderator, nil), 200)
	// A fresh API process retains the daily budget after successful collection.
	replenishCheck(t, New(p, s.Config), p, "cooldown")
}

func TestIntegrationReplenishmentSettingsAuthorizationAndAudit(t *testing.T) {
	s, p, f := replenishmentTest(t)
	base := "/api/v1/collection/replenishment"
	for _, actor := range []string{"", f.Owner, f.Developer} {
		want := 403
		if actor == "" {
			want = 401
		}
		for _, route := range []struct{ method, path string }{{"GET", base}, {"PATCH", base}, {"GET", base + "/history"}} {
			testStatus(t, testRequest(t, s, route.method, route.path, actor, map[string]any{"expected_revision": 1, "enabled": false}), want)
		}
	}
	s.Config.ImporterToken = "replenishment-importer-long-token-32-characters"
	testStatus(t, testRequestWithImporter(t, s, "PATCH", base, map[string]any{"expected_revision": 1, "enabled": false}), 403)
	rec := testRequest(t, s, "GET", base, f.Moderator, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["automatic_publication"] != false {
		t.Fatal("unexpected publication policy")
	}
	for _, body := range []map[string]any{{"expected_revision": 1, "minimum": 0}, {"expected_revision": 1, "target": 1}, {"expected_revision": 1, "batch_size": 101}, {"expected_revision": 1, "pending_limit": 0}, {"expected_revision": 1, "cooldown_hours": 0}, {"expected_revision": 1}} {
		testStatus(t, testRequest(t, s, "PATCH", base, f.Moderator, body), 422)
	}
	testStatus(t, testRequest(t, s, "PATCH", base, f.Moderator, map[string]any{"expected_revision": 0, "enabled": false}), 409)
	testStatus(t, testRequest(t, s, "PATCH", base, f.Moderator, map[string]any{"expected_revision": 1, "enabled": false}), 200)
	replenishCheck(t, s, p, "disabled")
	testStatus(t, testRequest(t, s, "PATCH", base, f.Moderator, map[string]any{"expected_revision": 1, "enabled": true}), 409)
	rec = testRequest(t, s, "GET", base+"/history", f.Moderator, nil)
	testStatus(t, rec, 200)
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatal("invalid audit JSON")
	}
	var count int
	if err := p.QueryRow(context.Background(), `SELECT count(*) FROM importer.replenishment_actions WHERE actor_id=$1 AND (before_policy->>'enabled')::bool AND NOT (after_policy->>'enabled')::bool`, f.Moderator).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit count %d: %v", count, err)
	}
}

func TestIntegrationReplenishmentRuntimeGrants(t *testing.T) {
	_, p, _ := replenishmentTest(t)
	tx, err := p.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), `SET LOCAL ROLE firstorder_backend`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`SELECT enabled FROM importer.replenishment_policy`, `UPDATE importer.replenishment_policy SET last_reason='test' WHERE id`, `INSERT INTO importer.replenishment_actions(before_policy,after_policy) VALUES('{}','{}') RETURNING id`} {
		if _, err = tx.Exec(context.Background(), query); err != nil {
			t.Fatalf("runtime grant: %v", err)
		}
	}
	if err = tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	cfg := p.Config().Copy()
	cfg.ConnConfig.RuntimeParams["role"] = "firstorder_backend"
	runtime, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	worker := New(runtime, Config{})
	if err = worker.scheduleReplenishment(context.Background()); err != nil {
		t.Fatalf("runtime scheduler: %v", err)
	}
}
