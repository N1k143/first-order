package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) moderationSummary(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	row, err := oneJSON(r.Context(), s.DB, `SELECT
 (SELECT count(*) FROM importer.company_candidates WHERE review_status='pending') AS pending_candidates,
 (SELECT count(*) FROM importer.company_candidates WHERE review_status='approved') AS approved_candidates,
 (SELECT count(*) FROM importer.company_candidates WHERE review_status='rejected') AS rejected_candidates,
 (SELECT count(*) FROM importer.company_candidates WHERE review_status='pending' AND coalesce(draft->>'website_status',payload->'website_check'->>'status')='found') AS pending_candidates_with_website,
 (SELECT count(*) FROM catalog.companies WHERE visibility='hidden') AS hidden_companies,
 (SELECT count(*) FROM catalog.companies WHERE visibility='published') AS published_companies,
 (SELECT count(*) FROM catalog.correction_requests WHERE status='pending') AS pending_corrections,
 (SELECT count(*) FROM orders.orders WHERE visibility=false) AS hidden_orders`)
	if err != nil {
		return err
	}
	return respond(w, 200, row)
}
func (s *Server) candidateActions(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	query := r.URL.Query()
	args := []any{}
	clauses := []string{"TRUE"}
	add := func(column, op string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("a.%s %s $%d", column, op, len(args)))
	}
	for _, key := range []string{"candidate_id", "moderator_id"} {
		if value := query.Get(key); value != "" {
			if !validUUID(value) {
				return invalid(key, "Некорректный UUID")
			}
			add(key, "=", value)
		}
	}
	if action := query.Get("action"); action != "" {
		if !oneOf(action, "edit", "approve", "reject", "link") {
			return invalid("action", "Неизвестное действие")
		}
		add("action", "=", action)
	}
	var from, to time.Time
	for _, field := range []string{"date_from", "date_to"} {
		if value := query.Get(field); value != "" {
			date, err := catalogDateFilter(field, value, field == "date_to")
			if err != nil {
				return err
			}
			if field == "date_from" {
				from = date
				add("created_at", ">=", date)
			} else {
				to = date
				add("created_at", "<=", date)
			}
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return invalid("date_to", "Конец периода раньше начала")
	}
	where := strings.Join(clauses, " AND ")
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM importer.candidate_actions a WHERE `+where, args...).Scan(&total); err != nil {
		return err
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := listJSON(r.Context(), s.DB, fmt.Sprintf(`SELECT a.*,p.name AS moderator_name FROM importer.candidate_actions a LEFT JOIN profiles.profiles p ON p.user_id=a.moderator_id WHERE %s ORDER BY a.created_at DESC,a.id LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return err
	}
	return paged(w, rows, total, page)
}
