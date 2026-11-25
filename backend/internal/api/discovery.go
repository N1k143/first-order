package api

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

func (s *Server) registerDiscovery(r chi.Router) {
	r.Get("/imports/candidates", handle(s.listCompanyCandidates))
	r.Get("/imports/candidates/{candidate_id}", handle(s.getCompanyCandidate))
	r.Patch("/imports/candidates/{candidate_id}", handle(s.patchCompanyCandidate))
	r.Get("/imports/candidates/{candidate_id}/duplicates", handle(s.candidateDuplicates))
	r.Post("/imports/candidates/{candidate_id}/review", handle(s.reviewCompanyCandidate))
	r.Get("/moderation/summary", handle(s.moderationSummary))
	r.Get("/moderation/candidate-actions", handle(s.candidateActions))
}
func (s *Server) reviewCompanyCandidate(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireRole(r, "moderator")
	if err != nil {
		return err
	}
	id, err := pathID(r, "candidate_id")
	if err != nil {
		return err
	}
	var body struct {
		Decision  string  `json:"decision"`
		Expected  *int64  `json:"expected_revision"`
		Reason    *string `json:"reason"`
		CompanyID *string `json:"company_id"`
	}
	if err = readJSON(r, &body); err != nil {
		return err
	}
	if !oneOf(body.Decision, "approve", "reject", "link") {
		return invalid("decision", "Допустимы approve, reject, link")
	}
	if body.Expected != nil && *body.Expected < 1 {
		return invalid("expected_revision", "Версия должна быть положительной")
	}
	if err = candidateNote(body.Reason); err != nil {
		return err
	}
	if body.Decision == "link" {
		if body.CompanyID == nil || !validUUID(*body.CompanyID) {
			return invalid("company_id", "Укажите существующую компанию")
		}
		if body.Expected == nil {
			return invalid("expected_revision", "Укажите текущую версию карточки")
		}
	}
	if body.Decision != "link" && body.CompanyID != nil {
		return invalid("company_id", "Поле допустимо только для link")
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer rollbackImport(tx)
	row, err := loadCandidate(r.Context(), tx, id, true)
	if err != nil {
		return err
	}
	if err = candidateConflict(row, body.Expected); err != nil {
		return err
	}
	before := row
	var companyID *string
	if body.Decision != "reject" {
		if _, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(1730191920)"); err != nil {
			return err
		}
		if body.Decision == "link" {
			var locked string
			if err = tx.QueryRow(r.Context(), `SELECT id::text FROM catalog.companies WHERE id=$1 FOR UPDATE`, *body.CompanyID).Scan(&locked); err != nil {
				return err
			}
			companyID = &locked
		} else {
			fields, err := proposedCompany(row)
			if err != nil {
				return err
			}
			if err = validateProposed(fields); err != nil {
				return err
			}
			if fields["website_status"] != "found" {
				fields["website_url"] = nil
			}
			var duplicate bool
			if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM catalog.companies c WHERE `+duplicateWhere+`)`, duplicateArgs(row, fields)...).Scan(&duplicate); err != nil {
				return err
			}
			if duplicate {
				e := problem(409, "POSSIBLE_DUPLICATE", "Найдена похожая компания. Проверьте совпадения и используйте link")
				e.Details = map[string]string{"duplicates_path": "/api/v1/imports/candidates/" + id + "/duplicates"}
				return e
			}
			category := fields["category_id"]
			if category == nil && fields["category_name"] != nil {
				var categoryID string
				if err = tx.QueryRow(r.Context(), `INSERT INTO catalog.categories(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, fields["category_name"]).Scan(&categoryID); err != nil {
					return err
				}
				category = categoryID
			}
			var cid string
			checked := row.Payload.Check.CheckedAt
			if _, ok := row.Draft["website_status"]; ok {
				checked = time.Now().UTC()
			}
			if _, ok := row.Draft["website_url"]; ok {
				checked = time.Now().UTC()
			}
			if err = tx.QueryRow(r.Context(), `INSERT INTO catalog.companies(name,category_id,city,address,phone,email,website_url,website_status,website_checked_at,source_url,visibility) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),'hidden') RETURNING id::text`, fields["name"], category, fields["city"], fields["address"], fields["phone"], fields["email"], fields["website_url"], fields["website_status"], checked, row.Payload.SourceURL).Scan(&cid); err != nil {
				return err
			}
			companyID = &cid
			reason := "Подтверждена импортированная карточка; публикация выполняется отдельно"
			if body.Reason != nil {
				reason = *body.Reason
			}
			if err = catalogAudit(r.Context(), tx, actor.ID, cid, "approve", &reason); err != nil {
				return err
			}
		}
	}
	status := "approved"
	if body.Decision == "reject" {
		status = "rejected"
	}
	if _, err = tx.Exec(r.Context(), `UPDATE importer.company_candidates SET review_status=$2,company_id=$3,reviewed_by=$4,reviewed_at=now(),review_note=$5 WHERE id=$1`, id, status, companyID, actor.ID, body.Reason); err != nil {
		return err
	}
	after, err := loadCandidate(r.Context(), tx, id, false)
	if err != nil {
		return err
	}
	if err = candidateEvent(r.Context(), tx, actor.ID, id, body.Decision, body.Reason, before, after); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, 200, map[string]any{"status": status, "company_id": companyID, "revision": after.Revision})
}
