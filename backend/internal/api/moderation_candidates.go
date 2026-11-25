package api

import (
	"context"
	"encoding/json"
	"fmt"
	"ingeneriya/backend/internal/discovery"
	"net/http"
	"strings"
	"unicode/utf8"
)

type candidateRecord struct {
	ID        string                     `json:"id"`
	Payload   discovery.Candidate        `json:"payload"`
	Draft     map[string]json.RawMessage `json:"draft"`
	Revision  int64                      `json:"revision"`
	Status    string                     `json:"review_status"`
	CompanyID *string                    `json:"company_id"`
}

func loadCandidate(ctx context.Context, q Querier, id string, lock bool) (candidateRecord, error) {
	var row candidateRecord
	var payload, draft []byte
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	err := q.QueryRow(ctx, `SELECT id::text,payload,draft,revision,review_status,company_id::text FROM importer.company_candidates WHERE id=$1`+suffix, id).Scan(&row.ID, &payload, &draft, &row.Revision, &row.Status, &row.CompanyID)
	if err != nil {
		return row, err
	}
	if json.Unmarshal(payload, &row.Payload) != nil || json.Unmarshal(draft, &row.Draft) != nil {
		return row, problem(422, "INVALID_CANDIDATE", "Некорректная исходная карточка")
	}
	return row, nil
}
func proposedCompany(row candidateRecord) (map[string]any, error) {
	p := row.Payload
	website := ""
	if p.Check.Status == "found" {
		website = p.Check.FinalURL
		if website == "" {
			website = p.WebsiteURL
		}
	}
	nullable := func(v string) any {
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return v
	}
	status := p.Check.Status
	if !oneOf(status, "found", "not_found", "needs_review") {
		status = "needs_review"
	}
	fields := map[string]any{"name": p.Name, "city": p.City, "category_id": nil, "category_name": nullable(p.Category), "address": nullable(p.Address), "phone": nullable(p.Phone), "email": nullable(p.Email), "website_url": nullable(website), "website_status": status}
	for k, raw := range row.Draft {
		var value any
		if json.Unmarshal(raw, &value) != nil {
			return nil, invalid("draft", "Некорректные поля черновика")
		}
		fields[k] = value
	}
	return fields, nil
}
func candidateResponse(ctx context.Context, q Querier, id string) (map[string]any, error) {
	item, err := oneJSON(ctx, q, `SELECT c.*,p.name AS reviewer_name FROM importer.company_candidates c LEFT JOIN profiles.profiles p ON p.user_id=c.reviewed_by WHERE c.id=$1`, id)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(item)
	var row candidateRecord
	if json.Unmarshal(raw, &row) != nil {
		return nil, problem(422, "INVALID_CANDIDATE", "Некорректная карточка")
	}
	fields, err := proposedCompany(row)
	if err != nil {
		return nil, err
	}
	item["proposed_company"] = fields
	return item, nil
}
func (s *Server) getCompanyCandidate(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	id, err := pathID(r, "candidate_id")
	if err != nil {
		return err
	}
	item, err := candidateResponse(r.Context(), s.DB, id)
	if err != nil {
		return err
	}
	return respond(w, 200, item)
}
func (s *Server) listCompanyCandidates(w http.ResponseWriter, r *http.Request) error {
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
	add := func(expression string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(expression, len(args)))
	}
	status := query.Get("status")
	if status == "" {
		status = "pending"
	}
	if !oneOf(status, "pending", "approved", "rejected", "all") {
		return invalid("status", "Неизвестный статус")
	}
	if status != "all" {
		add("c.review_status=$%d", status)
	}
	for _, field := range []string{"q", "city", "category"} {
		value := strings.TrimSpace(query.Get(field))
		if value == "" {
			continue
		}
		if utf8.RuneCountInString(value) > 240 {
			return invalid(field, "Не более 240 символов")
		}
		switch field {
		case "q":
			add("coalesce(c.draft->>'name',c.payload->>'name','') ILIKE '%%' || $%d || '%%'", value)
		case "city":
			add("coalesce(c.draft->>'city',c.payload->>'city','')=$%d", value)
		case "category":
			add("(CASE WHEN c.draft->>'category_id' IS NOT NULL THEN (SELECT cat.name FROM catalog.categories cat WHERE cat.id=(c.draft->>'category_id')::uuid) WHEN c.draft ? 'category_name' THEN c.draft->>'category_name' ELSE c.payload->>'category' END)=$%d", value)
		}
	}
	if value := query.Get("category_id"); value != "" {
		if !validUUID(value) {
			return invalid("category_id", "Некорректный UUID")
		}
		add("c.draft->>'category_id'=$%d", value)
	}
	if value := query.Get("website_status"); value != "" {
		if !oneOf(value, "found", "not_found", "needs_review") {
			return invalid("website_status", "Неизвестный статус сайта")
		}
		add("coalesce(c.draft->>'website_status',c.payload->'website_check'->>'status','needs_review')=$%d", value)
	}
	if value := query.Get("source"); value != "" {
		if value != "osm_astana" {
			return invalid("source", "Неизвестный источник")
		}
		add("c.source=$%d", value)
	}
	if value := query.Get("company_id"); value != "" {
		if !validUUID(value) {
			return invalid("company_id", "Некорректный UUID")
		}
		add("c.company_id=$%d", value)
	}
	where := strings.Join(clauses, " AND ")
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM importer.company_candidates c WHERE `+where, args...).Scan(&total); err != nil {
		return err
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := listJSON(r.Context(), s.DB, fmt.Sprintf(`SELECT c.*,p.name AS reviewer_name FROM importer.company_candidates c LEFT JOIN profiles.profiles p ON p.user_id=c.reviewed_by WHERE %s ORDER BY c.collected_at DESC,c.id LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return err
	}
	for _, item := range rows {
		raw, _ := json.Marshal(item)
		var row candidateRecord
		if json.Unmarshal(raw, &row) != nil {
			return problem(422, "INVALID_CANDIDATE", "Некорректная карточка")
		}
		fields, err := proposedCompany(row)
		if err != nil {
			return err
		}
		item["proposed_company"] = fields
	}
	return paged(w, rows, total, page)
}
func candidateConflict(row candidateRecord, expected *int64) error {
	if row.Status != "pending" {
		return problem(409, "ALREADY_REVIEWED", "Запись уже рассмотрена")
	}
	if expected != nil && *expected != row.Revision {
		e := problem(409, "STALE_CANDIDATE", "Карточка изменилась. Перечитайте данные")
		e.Details = map[string]any{"current_revision": row.Revision}
		return e
	}
	return nil
}
func candidateNote(value *string) error {
	if value != nil {
		if utf8.RuneCountInString(*value) > 4000 {
			return invalid("reason", "Не более 4000 символов")
		}
		*value = strings.TrimSpace(*value)
	}
	return nil
}
func candidateDraftFields(raw map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, invalid("changes", "Укажите изменяемые поля")
	}
	if id, ok := raw["category_id"]; ok && string(id) != "null" {
		if name, ok := raw["category_name"]; ok && string(name) != "null" {
			return nil, invalid("category_id", "Передайте category_id или category_name")
		}
	}
	base := map[string]json.RawMessage{}
	for key, value := range raw {
		if !oneOf(key, "name", "city", "category_id", "category_name", "address", "phone", "email", "website_url", "website_status") {
			return nil, invalid(key, "Поле недоступно для изменения")
		}
		if key == "category_name" {
			var name *string
			if json.Unmarshal(value, &name) != nil {
				return nil, invalid(key, "Ожидается строка или null")
			}
			if name != nil {
				normalized, err := catalogRequiredText(key, *name, 240)
				if err != nil {
					return nil, err
				}
				name = &normalized
			}
			raw[key], _ = json.Marshal(name)
		} else {
			base[key] = value
		}
	}
	if len(base) > 0 {
		normalized, err := catalogCompanyFields(base, false)
		if err != nil {
			return nil, err
		}
		for k, v := range normalized {
			raw[k], _ = json.Marshal(v)
		}
	}
	return raw, nil
}
func validateProposed(fields map[string]any) error {
	raw := map[string]json.RawMessage{}
	for key, value := range fields {
		if key != "category_name" {
			raw[key], _ = json.Marshal(value)
		}
	}
	normalized, err := catalogCompanyFields(raw, false)
	if err != nil {
		return err
	}
	for key, value := range normalized {
		fields[key] = value
	}
	if fields["category_id"] != nil && fields["category_name"] != nil {
		return invalid("category_id", "Выберите category_id или category_name")
	}
	if fields["website_status"] == "found" && (fields["website_url"] == nil || fields["website_url"] == "") {
		return invalid("website_url", "Для found укажите сайт")
	}
	if fields["website_status"] == "not_found" && fields["website_url"] != nil && fields["website_url"] != "" {
		return invalid("website_url", "Для not_found очистите ссылку")
	}
	return nil
}
func candidateEvent(ctx context.Context, q Querier, actor, id, action string, reason *string, before, after any) error {
	a, err := json.Marshal(before)
	if err != nil {
		return err
	}
	b, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO importer.candidate_actions(candidate_id,moderator_id,action,reason,before_state,after_state) VALUES($1,$2,$3,$4,$5,$6)`, id, actor, action, reason, a, b)
	return err
}
func (s *Server) patchCompanyCandidate(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireRole(r, "moderator")
	if err != nil {
		return err
	}
	id, err := pathID(r, "candidate_id")
	if err != nil {
		return err
	}
	var body struct {
		Changes  map[string]json.RawMessage `json:"changes"`
		Expected *int64                     `json:"expected_revision"`
		Reason   *string                    `json:"reason"`
	}
	if err = readJSON(r, &body); err != nil {
		return err
	}
	if body.Expected == nil || *body.Expected < 1 {
		return invalid("expected_revision", "Укажите текущую версию карточки")
	}
	if err = candidateNote(body.Reason); err != nil {
		return err
	}
	fields, err := candidateDraftFields(body.Changes)
	if err != nil {
		return err
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
	merged := map[string]json.RawMessage{}
	for k, v := range row.Draft {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}
	if v, ok := fields["category_id"]; ok && string(v) != "null" {
		merged["category_name"] = json.RawMessage("null")
	}
	if v, ok := fields["category_name"]; ok && string(v) != "null" {
		merged["category_id"] = json.RawMessage("null")
	}
	row.Draft = merged
	proposed, err := proposedCompany(row)
	if err != nil {
		return err
	}
	if err = validateProposed(proposed); err != nil {
		return err
	}
	if category, ok := proposed["category_id"].(string); ok {
		var exists bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM catalog.categories WHERE id=$1)`, category).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return invalid("category_id", "Категория не найдена")
		}
	}
	raw, _ := json.Marshal(merged)
	if _, err = tx.Exec(r.Context(), `UPDATE importer.company_candidates SET draft=$2 WHERE id=$1`, id, raw); err != nil {
		return err
	}
	row, err = loadCandidate(r.Context(), tx, id, false)
	if err != nil {
		return err
	}
	if err = candidateEvent(r.Context(), tx, actor.ID, id, "edit", body.Reason, before, row); err != nil {
		return err
	}
	item, err := candidateResponse(r.Context(), tx, id)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, 200, item)
}

const duplicateWhere = `c.source_url=$1 OR (lower(c.name)=lower($2) AND lower(c.city)=lower($3) AND (NULLIF($4,'') IS NULL OR lower(coalesce(c.address,''))=lower($4))) OR (NULLIF(regexp_replace($5,'[^0-9]','','g'),'') IS NOT NULL AND regexp_replace(coalesce(c.phone,''),'[^0-9]','','g')=regexp_replace($5,'[^0-9]','','g'))`

func duplicateArgs(row candidateRecord, fields map[string]any) []any {
	return []any{row.Payload.SourceURL, fields["name"], fields["city"], fields["address"], fields["phone"]}
}
func (s *Server) candidateDuplicates(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	id, err := pathID(r, "candidate_id")
	if err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	row, err := loadCandidate(r.Context(), s.DB, id, false)
	if err != nil {
		return err
	}
	fields, err := proposedCompany(row)
	if err != nil {
		return err
	}
	args := duplicateArgs(row, fields)
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM catalog.companies c WHERE `+duplicateWhere, args...).Scan(&total); err != nil {
		return err
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := listJSON(r.Context(), s.DB, `SELECT c.*,cat.name AS category_name FROM catalog.companies c LEFT JOIN catalog.categories cat ON cat.id=c.category_id WHERE `+duplicateWhere+` ORDER BY c.name,c.id LIMIT $6 OFFSET $7`, args...)
	if err != nil {
		return err
	}
	return paged(w, rows, total, page)
}
