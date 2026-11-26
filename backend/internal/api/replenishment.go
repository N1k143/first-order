package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// Availability describes a catalog card, not proof that a business lacks a site
// or has agreed to an order. Saving a favorite does not consume a card.
const replenishmentCounts = `SELECT
 (SELECT count(*) FROM catalog.companies WHERE visibility='published' AND city='Астана' AND website_status IN('not_found','needs_review')) AS available,
 (SELECT count(*) FROM importer.company_candidates WHERE source='osm_astana' AND review_status='pending') AS pending`

type replenishmentPolicy struct {
	Enabled       bool  `json:"enabled"`
	Minimum       int   `json:"minimum"`
	Target        int   `json:"target"`
	BatchSize     int   `json:"batch_size"`
	PendingLimit  int   `json:"pending_limit"`
	CooldownHours int   `json:"cooldown_hours"`
	Revision      int64 `json:"revision"`
}

func loadReplenishment(ctx context.Context, tx pgx.Tx) (replenishmentPolicy, error) {
	var p replenishmentPolicy
	err := tx.QueryRow(ctx, `SELECT enabled,minimum,target,batch_size,pending_limit,cooldown_hours,revision FROM importer.replenishment_policy WHERE id FOR UPDATE`).Scan(&p.Enabled, &p.Minimum, &p.Target, &p.BatchSize, &p.PendingLimit, &p.CooldownHours, &p.Revision)
	return p, err
}

func (s *Server) getReplenishment(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	item, err := oneJSON(r.Context(), s.DB, `SELECT p.*, 'osm_astana'::text AS source, 'Астана'::text AS city, false AS automatic_publication, 300 AS check_interval_seconds FROM importer.replenishment_policy p WHERE id`)
	if err != nil {
		return err
	}
	counts, err := oneJSON(r.Context(), s.DB, replenishmentCounts)
	if err != nil {
		return err
	}
	item["counts"] = counts
	if item["last_job_id"] != nil {
		job, err := oneJSON(r.Context(), s.DB, `SELECT `+collectionColumns+` FROM importer.collection_jobs WHERE id=$1`, item["last_job_id"])
		if err != nil {
			return err
		}
		item["last_job"] = job
	}
	return respond(w, 200, item)
}

func (s *Server) patchReplenishment(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireRole(r, "moderator")
	if err != nil {
		return err
	}
	var body struct {
		ExpectedRevision int64 `json:"expected_revision"`
		Enabled          *bool `json:"enabled"`
		Minimum          *int  `json:"minimum"`
		Target           *int  `json:"target"`
		BatchSize        *int  `json:"batch_size"`
		PendingLimit     *int  `json:"pending_limit"`
		CooldownHours    *int  `json:"cooldown_hours"`
	}
	if err = readJSON(r, &body); err != nil {
		return err
	}
	if body.Enabled == nil && body.Minimum == nil && body.Target == nil && body.BatchSize == nil && body.PendingLimit == nil && body.CooldownHours == nil {
		return invalid("body", "Укажите настройки пополнения")
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer rollbackImport(tx)
	// Same lock/order as scheduling, manual queueing and cancellation.
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(1730191921)`); err != nil {
		return err
	}
	p, err := loadReplenishment(r.Context(), tx)
	if err != nil {
		return err
	}
	if body.ExpectedRevision != p.Revision {
		return problem(409, "REVISION_CONFLICT", "Настройки уже изменены; перечитайте их")
	}
	before, _ := json.Marshal(p)
	if body.Enabled != nil {
		p.Enabled = *body.Enabled
	}
	for _, pair := range []struct {
		src *int
		dst *int
	}{{body.Minimum, &p.Minimum}, {body.Target, &p.Target}, {body.BatchSize, &p.BatchSize}, {body.PendingLimit, &p.PendingLimit}, {body.CooldownHours, &p.CooldownHours}} {
		if pair.src != nil {
			*pair.dst = *pair.src
		}
	}
	if p.Minimum < 1 || p.Minimum > 10000 || p.Target <= p.Minimum || p.Target > 20000 {
		return invalid("target", "Порог 1–10000, цель больше порога и не более 20000")
	}
	if p.BatchSize < 1 || p.BatchSize > 100 {
		return invalid("batch_size", "Размер партии от 1 до 100")
	}
	if p.PendingLimit < 1 || p.PendingLimit > 1000 {
		return invalid("pending_limit", "В очереди от 1 до 1000 карточек")
	}
	if p.CooldownHours < 1 || p.CooldownHours > 168 {
		return invalid("cooldown_hours", "Пауза от 1 до 168 часов")
	}
	p.Revision++
	after, _ := json.Marshal(p)
	if _, err = tx.Exec(r.Context(), `UPDATE importer.replenishment_policy SET enabled=$1,minimum=$2,target=$3,batch_size=$4,pending_limit=$5,cooldown_hours=$6,revision=revision+1,next_check_at=now(),updated_at=now() WHERE id`, p.Enabled, p.Minimum, p.Target, p.BatchSize, p.PendingLimit, p.CooldownHours); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO importer.replenishment_actions(actor_id,before_policy,after_policy) VALUES($1,$2,$3)`, actor.ID, before, after); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	// Disabling stops future scheduling; an already queued job has a separate
	// explicit cancellation endpoint and its own audit trail.
	return respond(w, 200, p)
}

func (s *Server) replenishmentHistory(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM importer.replenishment_actions`).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, `SELECT id,actor_id,before_policy,after_policy,created_at FROM importer.replenishment_actions ORDER BY id DESC LIMIT $1 OFFSET $2`, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, page)
}

// RunReplenishment only queues ordinary bounded collection jobs. It never fetches
// HTTP inside a transaction, publishes cards or modifies a moderator's decisions.
func (s *Server) RunReplenishment(ctx context.Context) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		check, stop := context.WithTimeout(ctx, 20*time.Second)
		err := s.scheduleReplenishment(check)
		stop()
		if err != nil && ctx.Err() == nil {
			s.logger.Error("catalog replenishment database operation failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Server) scheduleReplenishment(ctx context.Context) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackImport(tx)
	// try-lock avoids delaying manual controls or other API instances.
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(1730191921)`).Scan(&locked); err != nil || !locked {
		return err
	}
	p, err := loadReplenishment(ctx, tx)
	if err != nil {
		return err
	}
	var due, cooled, refilling bool
	if err = tx.QueryRow(ctx, `SELECT next_check_at<=now(),next_run_at<=now(),refill_active FROM importer.replenishment_policy WHERE id`).Scan(&due, &cooled, &refilling); err != nil {
		return err
	}
	if !due {
		return tx.Commit(ctx)
	}
	// Repeated failures or an exhausted source must not cause a daily scrape
	// forever. A productive run resets the consecutive-failure window.
	if _, err = tx.Exec(ctx, `WITH recent AS (
 SELECT status,saved,finished_at,row_number() OVER(ORDER BY created_at DESC,id DESC) AS n
 FROM importer.collection_jobs WHERE request_key LIKE 'replenishment/%'
 ORDER BY created_at DESC,id DESC LIMIT 3
 ), streak AS (
 SELECT count(*)::int AS n,max(finished_at) AS finished FROM recent
 WHERE n<COALESCE((SELECT min(n) FROM recent WHERE status NOT IN('failed','completed') OR (status='completed' AND saved>0)),4)
 ) UPDATE importer.replenishment_policy SET next_run_at=GREATEST(next_run_at,
 (SELECT finished+make_interval(hours=>LEAST(168,$1*(1<<n))) FROM streak WHERE n>0))
 WHERE id AND EXISTS(SELECT 1 FROM recent WHERE n=1 AND (status='failed' OR (status='completed' AND saved=0)))`, p.CooldownHours); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT next_run_at<=now() FROM importer.replenishment_policy WHERE id`).Scan(&cooled); err != nil {
		return err
	}
	var available, pending int
	if err = tx.QueryRow(ctx, replenishmentCounts).Scan(&available, &pending); err != nil {
		return err
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM importer.collection_jobs WHERE status IN('pending','running'))`).Scan(&active); err != nil {
		return err
	}
	if available >= p.Target {
		refilling = false
	} else if available <= p.Minimum {
		refilling = true
	}
	reason := "stock_sufficient"
	var jobID any
	switch {
	case !p.Enabled:
		reason = "disabled"
	case !refilling:
	case active:
		reason = "collection_active"
	case pending >= p.PendingLimit:
		reason = "pending_limit"
	case available+pending >= p.Target:
		reason = "awaiting_moderation"
	case !cooled:
		reason = "cooldown"
	default:
		limit := min(p.BatchSize, p.Target-available-pending, p.PendingLimit-pending)
		id := newID()
		if _, err = tx.Exec(ctx, `INSERT INTO importer.collection_jobs(id,kind,requested_limit,request_key,available_at) VALUES($1,'collect',$2,$3,(SELECT GREATEST(now(),next_fetch_at) FROM importer.collection_sources WHERE source='osm_astana'))`, id, limit, "replenishment/"+id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO importer.collection_events(job_id,event) VALUES($1,'automatic_replenishment')`, id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE importer.replenishment_policy SET next_run_at=now()+make_interval(hours=>$1),last_job_id=$2 WHERE id`, p.CooldownHours, id); err != nil {
			return err
		}
		reason = "queued"
		jobID = id
	}
	if _, err = tx.Exec(ctx, `UPDATE importer.replenishment_policy SET refill_active=$1,last_reason=$2,last_checked_at=now(),next_check_at=now()+interval '5 minutes' WHERE id`, refilling, reason); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if jobID != nil {
		s.logger.Info("catalog replenishment queued", "job_id", jobID, "available", available, "pending", pending)
	}
	return nil
}
