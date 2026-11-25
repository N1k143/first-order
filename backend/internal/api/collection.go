package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

const collectionColumns = `id,kind,source,city,requested_limit,status,created_by,attempt,processed,total,saved,skipped,found,needs_review,error_code,cancel_requested,available_at,created_at,started_at,finished_at,updated_at`

func (s *Server) registerCollection(r chi.Router) {
	r.Post("/collection/jobs", handle(s.createCollection))
	r.Get("/collection/jobs", handle(s.listCollection))
	r.Get("/collection/jobs/{job_id}", handle(s.getCollection))
	r.Post("/collection/jobs/{job_id}/cancel", handle(s.cancelCollection))
	r.Post("/collection/jobs/{job_id}/retry", handle(s.retryCollection))
}

func (s *Server) createCollection(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireRole(r, "moderator")
	if err != nil {
		return err
	}
	var body struct {
		Kind  string `json:"kind"`
		Limit int    `json:"limit"`
	}
	if err = readJSON(r, &body); err != nil {
		return err
	}
	item, status, err := s.QueueCollection(r.Context(), body.Kind, body.Limit, r.Header.Get("Idempotency-Key"), actor.ID)
	if err != nil {
		return err
	}
	return respond(w, status, item)
}

// QueueCollection is also used by the trusted operator command. An empty actorID
// records an operator run without inventing an Auth profile or granting a role.
func (s *Server) QueueCollection(ctx context.Context, kind string, limit int, key, actorID string) (map[string]any, int, error) {
	var creator any
	if actorID != "" {
		creator = actorID
	}
	if !oneOf(kind, "collect", "recheck") {
		return nil, 0, invalid("kind", "Допустимы collect и recheck")
	}
	if limit < 1 || limit > 100 {
		return nil, 0, invalid("limit", "Укажите от 1 до 100 компаний")
	}
	if !validUUID(key) {
		return nil, 0, invalid("Idempotency-Key", "Укажите уникальный UUID запроса")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer rollbackImport(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1730191921)`); err != nil {
		return nil, 0, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM importer.collection_jobs WHERE request_key=$1)`, key).Scan(&exists); err != nil {
		return nil, 0, err
	}
	if exists {
		item, e := oneJSON(ctx, tx, `SELECT `+collectionColumns+` FROM importer.collection_jobs WHERE request_key=$1`, key)
		if e != nil {
			return nil, 0, e
		}
		if item["kind"] != kind || item["requested_limit"] != float64(limit) || item["created_by"] != creator {
			return nil, 0, problem(409, "IDEMPOTENCY_CONFLICT", "Этот ключ уже использован для другого запроса")
		}
		return item, 200, nil
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM importer.collection_jobs WHERE status IN('pending','running'))`).Scan(&exists); err != nil {
		return nil, 0, err
	}
	if exists {
		return nil, 0, problem(409, "COLLECTION_ACTIVE", "Дождитесь завершения или отмените текущее задание")
	}
	var items any
	total := 0
	if kind == "recheck" {
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('candidate',payload,'revision',revision) ORDER BY collected_at,id),'[]'::jsonb) FROM (SELECT * FROM importer.company_candidates WHERE review_status='pending' AND draft='{}'::jsonb AND collected_at<now()-interval '24 hours' ORDER BY collected_at,id LIMIT $1) c`, limit).Scan(&raw); err != nil {
			return nil, 0, err
		}
		var entries []collectionItem
		if err = json.Unmarshal(raw, &entries); err != nil {
			return nil, 0, err
		}
		total = len(entries)
		if total == 0 {
			return nil, 0, problem(409, "NO_CANDIDATES", "Нет карточек без ручных правок, проверенных более суток назад")
		}
		items = raw
	}
	item, err := changeJSON(ctx, tx, `INSERT INTO importer.collection_jobs(kind,requested_limit,created_by,request_key,items,total,available_at) VALUES($1,$2,$3,$4,$5,$6,CASE WHEN $1='collect' THEN (SELECT GREATEST(now(),next_fetch_at) FROM importer.collection_sources WHERE source='osm_astana') ELSE now() END) RETURNING `+collectionColumns, kind, limit, creator, key, items, total)
	if err != nil {
		return nil, 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO importer.collection_events(job_id,event,actor_id) VALUES($1,'created',$2)`, item["id"], creator); err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return item, 202, nil
}
func (s *Server) listCollection(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	status := r.URL.Query().Get("status")
	if status != "" && !oneOf(status, "pending", "running", "completed", "failed", "canceled") {
		return invalid("status", "Неизвестный статус задания")
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM importer.collection_jobs WHERE ($1='' OR status=$1)`, status).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, `SELECT `+collectionColumns+` FROM importer.collection_jobs WHERE ($1='' OR status=$1) ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, status, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, page)
}
func (s *Server) getCollection(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	id, err := pathID(r, "job_id")
	if err != nil {
		return err
	}
	item, err := oneJSON(r.Context(), s.DB, `SELECT `+collectionColumns+` FROM importer.collection_jobs WHERE id=$1`, id)
	if err != nil {
		return err
	}
	events, err := listJSON(r.Context(), s.DB, `SELECT id,event,actor_id,created_at FROM importer.collection_events WHERE job_id=$1 ORDER BY id`, id)
	if err != nil {
		return err
	}
	item["events"] = events
	results, err := listJSON(r.Context(), s.DB, `SELECT item_index,external_id,name,website_status,reason,saved,checked_at FROM importer.collection_results WHERE job_id=$1 ORDER BY item_index`, id)
	if err != nil {
		return err
	}
	item["results"] = results
	return respond(w, 200, item)
}
func (s *Server) cancelCollection(w http.ResponseWriter, r *http.Request) error {
	return s.controlCollection(w, r, false)
}
func (s *Server) retryCollection(w http.ResponseWriter, r *http.Request) error {
	return s.controlCollection(w, r, true)
}
func (s *Server) controlCollection(w http.ResponseWriter, r *http.Request, retry bool) error {
	actor, err := s.requireRole(r, "moderator")
	if err != nil {
		return err
	}
	id, err := pathID(r, "job_id")
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer rollbackImport(tx)
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(1730191921)`); err != nil {
		return err
	}
	job, err := oneJSON(r.Context(), tx, `SELECT `+collectionColumns+` FROM importer.collection_jobs WHERE id=$1 FOR UPDATE`, id)
	if err != nil {
		return err
	}
	event := "cancel_requested"
	if retry {
		if job["status"] != "failed" || job["attempt"].(float64) >= 3 {
			return problem(409, "RETRY_UNAVAILABLE", "Повтор доступен только для ошибки, не более трёх попыток")
		}
		var active bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM importer.collection_jobs WHERE status IN('pending','running'))`).Scan(&active); err != nil {
			return err
		}
		if active {
			return problem(409, "COLLECTION_ACTIVE", "Уже выполняется другое задание")
		}
		_, err = tx.Exec(r.Context(), `UPDATE importer.collection_jobs SET status='pending',cancel_requested=false,error_code=NULL,finished_at=NULL,lease_token=NULL,lease_until=NULL,available_at=CASE WHEN kind='collect' AND items IS NULL THEN (SELECT GREATEST(now(),next_fetch_at) FROM importer.collection_sources WHERE source='osm_astana') ELSE now() END,updated_at=now() WHERE id=$1`, id)
		event = "retry_requested"
	} else {
		if !oneOf(job["status"].(string), "pending", "running") {
			return respond(w, 200, job)
		}
		_, err = tx.Exec(r.Context(), `UPDATE importer.collection_jobs SET cancel_requested=true,status=CASE WHEN status='pending' THEN 'canceled' ELSE status END,finished_at=CASE WHEN status='pending' THEN now() ELSE finished_at END,updated_at=now() WHERE id=$1`, id)
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO importer.collection_events(job_id,event,actor_id) VALUES($1,$2,$3)`, id, event, actor.ID); err != nil {
		return err
	}
	job, err = oneJSON(r.Context(), tx, `SELECT `+collectionColumns+` FROM importer.collection_jobs WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, 202, job)
}
