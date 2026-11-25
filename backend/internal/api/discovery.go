package api

import (
	"encoding/json"
	"ingeneriya/backend/internal/discovery"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) registerDiscovery(r chi.Router) {
	r.Get("/imports/candidates", handle(s.listCompanyCandidates))
	r.Post("/imports/candidates/{candidate_id}/review", handle(s.reviewCompanyCandidate))
}
func (s *Server) listCompanyCandidates(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	if !oneOf(status, "pending", "approved", "rejected") {
		return invalid("status", "Неизвестный статус")
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM importer.company_candidates WHERE review_status=$1`, status).Scan(&total); err != nil {
		return err
	}
	rows, err := listJSON(r.Context(), s.DB, `SELECT * FROM importer.company_candidates WHERE review_status=$1 ORDER BY collected_at DESC,id LIMIT $2 OFFSET $3`, status, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return paged(w, rows, total, page)
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
		Decision string `json:"decision"`
	}
	if err = readJSON(r, &body); err != nil {
		return err
	}
	if !oneOf(body.Decision, "approve", "reject") {
		return invalid("decision", "Допустимы approve и reject")
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer rollbackImport(tx)
	var payload []byte
	var status string
	if err = tx.QueryRow(r.Context(), `SELECT payload,review_status FROM importer.company_candidates WHERE id=$1 FOR UPDATE`, id).Scan(&payload, &status); err != nil {
		return err
	}
	if status != "pending" {
		return problem(409, "ALREADY_REVIEWED", "Запись уже рассмотрена")
	}
	var companyID *string
	if body.Decision == "approve" {
		if _, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(1730191920)"); err != nil {
			return err
		}
		var candidate discovery.Candidate
		if json.Unmarshal(payload, &candidate) != nil {
			return problem(422, "INVALID_CANDIDATE", "Некорректная карточка")
		}
		// Only new cards are approved here. Matches require a deliberate manual merge.
		var duplicate bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM catalog.companies WHERE source_url=$1 OR (lower(name)=lower($2) AND lower(city)=lower($3) AND (NULLIF($4,'') IS NULL OR lower(coalesce(address,''))=lower($4))) OR (NULLIF($5,'') IS NOT NULL AND regexp_replace(coalesce(phone,''),'[^0-9]','','g')=regexp_replace($5,'[^0-9]','','g')))`, candidate.SourceURL, candidate.Name, candidate.City, candidate.Address, candidate.Phone).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate {
			return problem(409, "POSSIBLE_DUPLICATE", "Найдена похожая компания. Проверьте объединение вручную")
		}
		var categoryID string
		if err = tx.QueryRow(r.Context(), `INSERT INTO catalog.categories(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, candidate.Category).Scan(&categoryID); err != nil {
			return err
		}
		website := candidate.WebsiteURL
		if candidate.Check.FinalURL != "" {
			website = candidate.Check.FinalURL
		}
		// Unconfirmed source links remain evidence in the candidate, never official websites.
		if candidate.Check.Status != "found" {
			website = ""
		}
		var cid string
		if err = tx.QueryRow(r.Context(), `INSERT INTO catalog.companies(name,category_id,city,address,phone,email,website_url,website_status,website_checked_at,source_url,visibility) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9,$10,'hidden') RETURNING id::text`, candidate.Name, categoryID, candidate.City, candidate.Address, candidate.Phone, candidate.Email, website, candidate.Check.Status, candidate.Check.CheckedAt, candidate.SourceURL).Scan(&cid); err != nil {
			return err
		}
		companyID = &cid
		reason := "Подтверждена карточка из OpenStreetMap; публикация выполняется отдельно"
		if err = catalogAudit(r.Context(), tx, actor.ID, cid, "approve", &reason); err != nil {
			return err
		}
	}
	status = "rejected"
	if body.Decision == "approve" {
		status = "approved"
	}
	if _, err = tx.Exec(r.Context(), `UPDATE importer.company_candidates SET review_status=$2,company_id=$3,reviewed_by=$4,reviewed_at=now() WHERE id=$1`, id, status, companyID, actor.ID); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, 200, map[string]any{"status": status, "company_id": companyID})
}
