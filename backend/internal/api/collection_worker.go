package api

import (
	"context"
	"encoding/json"
	"errors"
	"ingeneriya/backend/internal/discovery"
	"time"

	"github.com/jackc/pgx/v5"
)

type collectionItem struct {
	Candidate discovery.Candidate `json:"candidate"`
	Revision  int64               `json:"revision,omitempty"`
}
type collectionJob struct {
	ID, Token, Kind  string
	Limit, Processed int
	Items            []collectionItem
}

var errCollectionLease = errors.New("collection lease lost")

// RunCollection uses short transactions for claims/checkpoints. HTTP requests never
// hold a database transaction. Every write is fenced by the current lease token.
func (s *Server) RunCollection(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.processCollection(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("collection worker database operation failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (s *Server) claimCollection(ctx context.Context) (*collectionJob, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollbackImport(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1730191921)`); err != nil {
		return nil, err
	}
	// A stopped worker's token becomes unusable before another worker resumes.
	if _, err = tx.Exec(ctx, `WITH recovered AS (
 UPDATE importer.collection_jobs SET status=CASE WHEN cancel_requested THEN 'canceled' WHEN attempt>=3 THEN 'failed' ELSE 'pending' END,
 error_code=CASE WHEN NOT cancel_requested AND attempt>=3 THEN 'WORKER_INTERRUPTED' ELSE NULL END,
 finished_at=CASE WHEN cancel_requested OR attempt>=3 THEN now() ELSE NULL END,
 lease_token=NULL,lease_until=NULL,updated_at=now()
 WHERE status='running' AND lease_until<now() RETURNING id,status)
 INSERT INTO importer.collection_events(job_id,event) SELECT id,CASE WHEN status='pending' THEN 'worker_recovered' ELSE status END FROM recovered`); err != nil {
		return nil, err
	}
	var job collectionJob
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT id::text,kind,requested_limit,processed,items FROM importer.collection_jobs WHERE status='pending' AND available_at<=now() ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&job.ID, &job.Kind, &job.Limit, &job.Processed, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	if raw != nil {
		if err = json.Unmarshal(raw, &job.Items); err != nil {
			return nil, err
		}
	}
	if job.Kind == "collect" && raw == nil {
		var ready bool
		if err = tx.QueryRow(ctx, `SELECT next_fetch_at<=now() FROM importer.collection_sources WHERE source='osm_astana' FOR UPDATE`).Scan(&ready); err != nil {
			return nil, err
		}
		if !ready {
			if _, err = tx.Exec(ctx, `UPDATE importer.collection_jobs SET available_at=(SELECT next_fetch_at FROM importer.collection_sources WHERE source='osm_astana'),updated_at=now() WHERE id=$1`, job.ID); err != nil {
				return nil, err
			}
			return nil, tx.Commit(ctx)
		}
		if _, err = tx.Exec(ctx, `UPDATE importer.collection_sources SET next_fetch_at=now()+interval '1 hour' WHERE source='osm_astana'`); err != nil {
			return nil, err
		}
	}
	job.Token = newID()
	if _, err = tx.Exec(ctx, `UPDATE importer.collection_jobs SET status='running',attempt=attempt+1,lease_token=$2,lease_until=now()+interval '2 minutes',started_at=COALESCE(started_at,now()),updated_at=now() WHERE id=$1`, job.ID, job.Token); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO importer.collection_events(job_id,event) VALUES($1,'started')`, job.ID); err != nil {
		return nil, err
	}
	return &job, tx.Commit(ctx)
}
func (s *Server) processCollection(parent context.Context) error {
	job, err := s.claimCollection(parent)
	if err != nil || job == nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				beat, stop := context.WithTimeout(ctx, 5*time.Second)
				tag, e := s.DB.Exec(beat, `UPDATE importer.collection_jobs SET lease_until=now()+interval '2 minutes',updated_at=now() WHERE id=$1 AND lease_token=$2 AND status='running' AND NOT cancel_requested AND lease_until>now()`, job.ID, job.Token)
				stop()
				if e != nil || tag.RowsAffected() != 1 {
					cancel()
					return
				}
			}
		}
	}()
	code := s.executeCollection(ctx, job)
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	cancel()
	<-done
	// Shutdown leaves a recoverable lease; never convert it to a completed job.
	if parent.Err() != nil {
		return nil
	}
	if timedOut {
		code = "JOB_TIMEOUT"
	}
	finish, stop := context.WithTimeout(parent, 10*time.Second)
	defer stop()
	tx, err := s.DB.Begin(finish)
	if err != nil {
		return err
	}
	defer rollbackImport(tx)
	// Lease loss, including heartbeat failure, must not finalize another worker's job.
	status := "completed"
	if code != "" {
		status = "failed"
	}
	var actual string
	err = tx.QueryRow(finish, `UPDATE importer.collection_jobs SET status=CASE WHEN cancel_requested THEN 'canceled' ELSE $3 END,error_code=CASE WHEN cancel_requested THEN NULL ELSE NULLIF($4,'') END,finished_at=now(),updated_at=now(),lease_until=NULL,lease_token=NULL WHERE id=$1 AND lease_token=$2 AND status='running' AND lease_until>now() RETURNING status`, job.ID, job.Token, status, code).Scan(&actual)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(finish, `INSERT INTO importer.collection_events(job_id,event) VALUES($1,$2)`, job.ID, actual); err != nil {
		return err
	}
	return tx.Commit(finish)
}
func (s *Server) executeCollection(ctx context.Context, job *collectionJob) string {
	if job.Items == nil {
		fetch := s.collectionFetch
		if fetch == nil {
			fetch = s.fetchNewCompanies
		}
		candidates, err := fetch(ctx, job.Limit)
		if err != nil {
			return "SOURCE_UNAVAILABLE"
		}
		job.Items = make([]collectionItem, len(candidates))
		for i, c := range candidates {
			job.Items[i] = collectionItem{Candidate: c}
		}
		raw, err := json.Marshal(job.Items)
		if err != nil {
			return "SOURCE_INVALID"
		}
		tag, err := s.DB.Exec(ctx, `UPDATE importer.collection_jobs SET items=$3,total=$4,updated_at=now() WHERE id=$1 AND lease_token=$2 AND status='running' AND NOT cancel_requested AND lease_until>now()`, job.ID, job.Token, raw, len(job.Items))
		if err != nil || tag.RowsAffected() != 1 {
			return "CHECKPOINT_FAILED"
		}
	}
	checker := discovery.NewChecker()
	defer checker.Client.CloseIdleConnections()
	wikidata := discovery.NewWikidata()
	defer wikidata.Client.CloseIdleConnections()
	check := s.collectionCheck
	if check == nil {
		check = func(ctx context.Context, c discovery.Candidate) discovery.Candidate {
			// Rebuild inferred URLs and warnings so repeated checks do not accumulate stale evidence.
			c.WebsiteCandidates = nil
			c.DiscoveryWarnings = nil
			discovery.Enrich(ctx, &c, wikidata)
			c.Check = checker.Discover(ctx, c)
			return c
		}
	}
	for i := job.Processed; i < len(job.Items); i++ {
		if ctx.Err() != nil {
			return "WORKER_INTERRUPTED"
		}
		c := check(ctx, job.Items[i].Candidate)
		if ctx.Err() != nil {
			return "WORKER_INTERRUPTED"
		}
		if err := s.saveCollectionItem(ctx, job, i, c); err != nil {
			return "CHECKPOINT_FAILED"
		}
		select {
		case <-ctx.Done():
			return "WORKER_INTERRUPTED"
		case <-time.After(300 * time.Millisecond):
		}
	}
	return ""
}

func (s *Server) fetchNewCompanies(ctx context.Context, limit int) ([]discovery.Candidate, error) {
	snapshot, err := discovery.Fetch(ctx, "")
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT external_id FROM importer.company_candidates WHERE source='osm_astana'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		known[id] = true
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return discovery.SelectUnseen(snapshot, limit, known), nil
}
func (s *Server) saveCollectionItem(ctx context.Context, job *collectionJob, index int, c discovery.Candidate) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackImport(tx)
	// Lock/fence the job before touching a candidate. Cancellation and recovery
	// cannot race a checkpoint into a later attempt.
	var current int
	err = tx.QueryRow(ctx, `SELECT processed FROM importer.collection_jobs WHERE id=$1 AND lease_token=$2 AND status='running' AND NOT cancel_requested AND lease_until>now() FOR UPDATE`, job.ID, job.Token).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return errCollectionLease
	}
	if err != nil {
		return err
	}
	if current != index {
		return errCollectionLease
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	saved := int64(0)
	if job.Kind == "recheck" {
		tag, e := tx.Exec(ctx, `UPDATE importer.company_candidates SET payload=$3,collected_at=now() WHERE source=$1 AND external_id=$2 AND review_status='pending' AND draft='{}'::jsonb AND revision=$4`, c.Source, c.ExternalID, raw, job.Items[index].Revision)
		if e != nil {
			return e
		}
		saved = tag.RowsAffected()
	} else {
		tag, e := tx.Exec(ctx, `INSERT INTO importer.company_candidates(source,external_id,payload) VALUES($1,$2,$3) ON CONFLICT(source,external_id) DO UPDATE SET payload=EXCLUDED.payload,collected_at=now() WHERE company_candidates.review_status='pending' AND company_candidates.draft='{}'::jsonb`, c.Source, c.ExternalID, raw)
		if e != nil {
			return e
		}
		saved = tag.RowsAffected()
	}
	found, review := 0, 0
	if c.Check.Status == "found" {
		found = 1
	} else {
		review = 1
	}
	if _, err = tx.Exec(ctx, `INSERT INTO importer.collection_results(job_id,item_index,external_id,name,website_status,reason,saved) VALUES($1,$2,$3,$4,$5,$6,$7)`, job.ID, index, c.ExternalID, c.Name, c.Check.Status, c.Check.Reason, saved == 1); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE importer.collection_jobs SET processed=processed+1,saved=saved+$3,skipped=skipped+1-$3,found=found+$4,needs_review=needs_review+$5,updated_at=now() WHERE id=$1 AND lease_token=$2`, job.ID, job.Token, saved, found, review); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
