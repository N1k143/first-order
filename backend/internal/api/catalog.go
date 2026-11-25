package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) registerCatalog(r chi.Router) {
	r.MethodFunc("GET", "/categories", handle(s.catalogListCategories))
	r.MethodFunc("POST", "/categories", handle(s.catalogCreateCategory))
	r.MethodFunc("PATCH", "/categories/{category_id}", handle(s.catalogPatchCategory))
	r.MethodFunc("DELETE", "/categories/{category_id}", handle(s.catalogDeleteCategory))
	r.MethodFunc("GET", "/companies", handle(s.catalogListCompanies))
	r.MethodFunc("GET", "/companies/{company_id}", handle(s.catalogGetCompany))
	r.MethodFunc("POST", "/companies", handle(s.catalogCreateCompany))
	r.MethodFunc("PATCH", "/companies/{company_id}", handle(s.catalogPatchCompany))
	r.MethodFunc("PATCH", "/companies/{company_id}/visibility", handle(s.catalogSetVisibility))
	r.MethodFunc("GET", "/me/favorites", handle(s.catalogListFavorites))
	r.MethodFunc("PUT", "/me/favorites/{company_id}", handle(s.catalogPutFavorite))
	r.MethodFunc("DELETE", "/me/favorites/{company_id}", handle(s.catalogDeleteFavorite))
	r.MethodFunc("GET", "/me/company-contacts", handle(s.catalogListContacts))
	r.MethodFunc("GET", "/me/company-contacts/{company_id}", handle(s.catalogGetContact))
	r.MethodFunc("PUT", "/me/company-contacts/{company_id}", handle(s.catalogPutContact))
	r.MethodFunc("DELETE", "/me/company-contacts/{company_id}", handle(s.catalogDeleteContact))
	r.MethodFunc("POST", "/companies/{company_id}/correction-requests", handle(s.catalogCreateCorrection))
	r.MethodFunc("GET", "/me/correction-requests", handle(s.catalogOwnCorrections))
	r.MethodFunc("GET", "/moderation/correction-requests", handle(s.catalogModerationCorrections))
	r.MethodFunc("GET", "/moderation/correction-requests/{request_id}", handle(s.catalogModerationCorrection))
	r.MethodFunc("PATCH", "/moderation/correction-requests/{request_id}", handle(s.catalogResolveCorrection))
	r.MethodFunc("GET", "/moderation/actions", handle(s.catalogModerationActions))
}

func catalogRequiredText(field, value string, limit int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalid(field, "Поле обязательно")
	}
	if utf8.RuneCountInString(value) > limit {
		return "", invalid(field, fmt.Sprintf("Не более %d символов", limit))
	}
	return value, nil
}

// A raw-field DTO deliberately distinguishes omission from explicit JSON null.
// Only keys validated here can become SQL identifiers below.
func catalogCompanyFields(raw map[string]json.RawMessage, create bool) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, invalid("body", "Укажите изменяемые поля")
	}
	fields := make(map[string]any, len(raw))
	for key, value := range raw {
		switch key {
		case "name", "category_id", "city", "address", "phone", "email", "website_url", "website_status", "website_checked_at", "source_url":
		default:
			return nil, invalid(key, "Поле недоступно для изменения")
		}
		nullable := !oneOf(key, "name", "city", "website_status")
		if string(value) == "null" {
			if !nullable {
				return nil, invalid(key, "Значение не может быть null")
			}
			fields[key] = nil
			continue
		}
		var v string
		if err := json.Unmarshal(value, &v); err != nil {
			return nil, invalid(key, "Ожидается строка или null для необязательного поля")
		}
		v = strings.TrimSpace(v)
		if utf8.RuneCountInString(v) > 4000 {
			return nil, invalid(key, "Не более 4000 символов")
		}
		switch key {
		case "name", "city":
			var err error
			v, err = catalogRequiredText(key, v, 240)
			if err != nil {
				return nil, err
			}
		case "category_id":
			if !validUUID(v) {
				return nil, invalid(key, "Ожидается UUID или null")
			}
		case "website_status":
			if !oneOf(v, "not_found", "found", "needs_review") {
				return nil, invalid(key, "Неизвестный статус проверки сайта")
			}
		case "website_url", "source_url":
			if err := validateURL(key, v); err != nil {
				return nil, err
			}
		case "email":
			if v != "" {
				addr, err := mail.ParseAddress(v)
				if err != nil || addr.Address != v {
					return nil, invalid(key, "Некорректный email")
				}
			}
		case "website_checked_at":
			parsed, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				return nil, invalid(key, "Ожидается дата и время ISO 8601 с часовым поясом")
			}
			fields[key] = parsed.UTC()
			continue
		}
		fields[key] = v
	}
	if create {
		if _, ok := fields["name"]; !ok {
			return nil, invalid("name", "Поле обязательно")
		}
		if _, ok := fields["city"]; !ok {
			fields["city"] = "Астана"
		}
		if _, ok := fields["website_status"]; !ok {
			fields["website_status"] = "needs_review"
		}
	}
	return fields, nil
}

func catalogSortedFields(fields map[string]any) []string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func catalogUpdateCompany(ctx context.Context, q Querier, id string, fields map[string]any) (map[string]any, error) {
	keys := catalogSortedFields(fields)
	set := make([]string, 0, len(keys))
	args := []any{id}
	for _, k := range keys {
		args = append(args, fields[k])
		set = append(set, fmt.Sprintf("%s=$%d", k, len(args)))
	}
	return changeJSON(ctx, q, "UPDATE catalog.companies SET "+strings.Join(set, ",")+" WHERE id=$1 RETURNING *", args...)
}

func catalogAudit(ctx context.Context, q Querier, actorID, companyID, action string, reason *string) error {
	_, err := q.Exec(ctx, `INSERT INTO catalog.moderation_actions(moderator_id,company_id,action,reason) VALUES($1,$2,$3,$4)`, actorID, companyID, action, reason)
	return err
}

func (s *Server) catalogListCategories(w http.ResponseWriter, r *http.Request) error {
	p, err := pagination(r)
	if err != nil {
		return err
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM catalog.categories`).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, `SELECT id,name FROM catalog.categories ORDER BY name,id LIMIT $1 OFFSET $2`, p.Limit, p.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, p)
}
func (s *Server) catalogCreateCategory(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	name, err := catalogRequiredText("name", body.Name, 240)
	if err != nil {
		return err
	}
	item, err := changeJSON(r.Context(), s.DB, `INSERT INTO catalog.categories(name) VALUES($1) RETURNING *`, name)
	if err != nil {
		return err
	}
	return respond(w, http.StatusCreated, item)
}
func (s *Server) catalogPatchCategory(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	id, err := pathID(r, "category_id")
	if err != nil {
		return err
	}
	var body struct {
		Name string `json:"name"`
	}
	if err = readJSON(r, &body); err != nil {
		return err
	}
	name, err := catalogRequiredText("name", body.Name, 240)
	if err != nil {
		return err
	}
	item, err := changeJSON(r.Context(), s.DB, `UPDATE catalog.categories SET name=$2 WHERE id=$1 RETURNING *`, id, name)
	if err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}
func (s *Server) catalogDeleteCategory(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	id, err := pathID(r, "category_id")
	if err != nil {
		return err
	}
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM catalog.categories WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return respond(w, http.StatusNoContent, nil)
}

func catalogCompanyOrder(value string) (string, error) {
	switch value {
	case "", "updated":
		return "c.website_checked_at DESC NULLS LAST,c.updated_at DESC,c.id", nil
	case "name", "name_asc":
		return "c.name ASC,c.id", nil
	case "name_desc":
		return "c.name DESC,c.id", nil
	case "newest", "created_at_desc":
		return "c.created_at DESC,c.id", nil
	case "oldest", "created_at_asc":
		return "c.created_at ASC,c.id", nil
	default:
		return "", invalid("sort", "Неизвестный способ сортировки")
	}
}
func (s *Server) catalogListCompanies(w http.ResponseWriter, r *http.Request) error {
	p, err := pagination(r)
	if err != nil {
		return err
	}
	query := r.URL.Query()
	order, err := catalogCompanyOrder(query.Get("sort"))
	if err != nil {
		return err
	}
	clauses := []string{}
	args := []any{}
	add := func(column string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s=$%d", column, len(args)))
	}
	visibility := query.Get("visibility")
	if visibility != "" && visibility != "published" {
		if _, err = s.requireRole(r, "moderator"); err != nil {
			return err
		}
		if !oneOf(visibility, "hidden", "all") {
			return invalid("visibility", "Допустимы published, hidden, all")
		}
	}
	if visibility != "all" {
		if visibility == "" {
			visibility = "published"
		}
		add("c.visibility", visibility)
	}
	if q := strings.TrimSpace(query.Get("q")); q != "" {
		if utf8.RuneCountInString(q) > 240 {
			return invalid("q", "Слишком длинный поисковый запрос")
		}
		args = append(args, q)
		clauses = append(clauses, fmt.Sprintf("c.name ILIKE '%%' || $%d || '%%'", len(args)))
	}
	if id := query.Get("category_id"); id != "" {
		if !validUUID(id) {
			return invalid("category_id", "Некорректный UUID")
		}
		add("c.category_id", id)
	}
	if category := strings.TrimSpace(query.Get("category")); category != "" {
		if utf8.RuneCountInString(category) > 240 {
			return invalid("category", "Не более 240 символов")
		}
		args = append(args, category)
		clauses = append(clauses, fmt.Sprintf("EXISTS (SELECT 1 FROM catalog.categories category_filter WHERE category_filter.id=c.category_id AND category_filter.name=$%d)", len(args)))
	}
	if city := strings.TrimSpace(query.Get("city")); city != "" {
		add("c.city", city)
	}
	if status := query.Get("website_status"); status != "" {
		if !oneOf(status, "not_found", "found", "needs_review") {
			return invalid("website_status", "Неизвестный статус")
		}
		add("c.website_status", status)
	}
	where := "TRUE"
	if len(clauses) > 0 {
		where = strings.Join(clauses, " AND ")
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), "SELECT count(*) FROM catalog.companies c WHERE "+where, args...).Scan(&total); err != nil {
		return err
	}
	args = append(args, p.Limit, p.Offset)
	sql := fmt.Sprintf(`SELECT c.*,cat.name AS category_name FROM catalog.companies c LEFT JOIN catalog.categories cat ON cat.id=c.category_id WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`, where, order, len(args)-1, len(args))
	items, err := listJSON(r.Context(), s.DB, sql, args...)
	if err != nil {
		return err
	}
	return paged(w, items, total, p)
}
func (s *Server) catalogGetCompany(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	actorID := ""
	moderator := false
	if a := s.actor(r); a != nil {
		actorID = a.ID
		moderator = a.Role == "moderator"
	}
	item, err := oneJSON(r.Context(), s.DB, `SELECT c.*,cat.name AS category_name FROM catalog.companies c LEFT JOIN catalog.categories cat ON cat.id=c.category_id WHERE c.id=$1 AND (c.visibility='published' OR $3::boolean OR EXISTS(SELECT 1 FROM profiles.company_members m WHERE m.company_id=c.id AND m.user_id=NULLIF($2,'')::uuid))`, id, actorID, moderator)
	if err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}
func (s *Server) catalogCreateCompany(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator", "system_importer"); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := readJSON(r, &raw); err != nil {
		return err
	}
	fields, err := catalogCompanyFields(raw, true)
	if err != nil {
		return err
	}
	keys := catalogSortedFields(fields)
	params := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, k := range keys {
		args = append(args, fields[k])
		params = append(params, fmt.Sprintf("$%d", len(args)))
	}
	item, err := changeJSON(r.Context(), s.DB, "INSERT INTO catalog.companies ("+strings.Join(keys, ",")+") VALUES ("+strings.Join(params, ",")+") RETURNING *", args...)
	if err != nil {
		return err
	}
	return respond(w, http.StatusCreated, item)
}
func (s *Server) catalogPatchCompany(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "developer", "moderator")
	if err != nil {
		return err
	}
	id, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err = readJSON(r, &raw); err != nil {
		return err
	}
	fields, err := catalogCompanyFields(raw, false)
	if err != nil {
		return err
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM catalog.companies WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return err
	}
	if a.Role != "moderator" {
		if err = requireMember(ctx, tx, a.ID, id, false); err != nil {
			return err
		}
	}
	item, err := catalogUpdateCompany(ctx, tx, id, fields)
	if err != nil {
		return err
	}
	if a.Role == "moderator" {
		if err = catalogAudit(ctx, tx, a.ID, id, "correct", nil); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}
func (s *Server) catalogSetVisibility(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "moderator")
	if err != nil {
		return err
	}
	id, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	var body struct {
		Visibility string  `json:"visibility"`
		Reason     *string `json:"reason"`
	}
	if err = readJSON(r, &body); err != nil {
		return err
	}
	if !oneOf(body.Visibility, "published", "hidden") {
		return invalid("visibility", "Допустимы published или hidden")
	}
	if body.Reason != nil && utf8.RuneCountInString(*body.Reason) > 4000 {
		return invalid("reason", "Не более 4000 символов")
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current string
	if err = tx.QueryRow(ctx, `SELECT visibility FROM catalog.companies WHERE id=$1 FOR UPDATE`, id).Scan(&current); err != nil {
		return err
	}
	item, err := changeJSON(ctx, tx, `UPDATE catalog.companies SET visibility=$2 WHERE id=$1 RETURNING *`, id, body.Visibility)
	if err != nil {
		return err
	}
	if current != body.Visibility {
		action := "publish"
		if body.Visibility == "hidden" {
			action = "hide"
		}
		if err = catalogAudit(ctx, tx, a.ID, id, action, body.Reason); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}

// A shared row lock prevents the company from becoming hidden mid-mutation.
func catalogLockPublished(ctx context.Context, q Querier, companyID string) error {
	var id string
	return q.QueryRow(ctx, `SELECT id FROM catalog.companies WHERE id=$1 AND visibility='published' FOR SHARE`, companyID).Scan(&id)
}
func (s *Server) catalogListFavorites(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	p, err := pagination(r)
	if err != nil {
		return err
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM catalog.favorites f JOIN catalog.companies c ON c.id=f.company_id WHERE f.developer_id=$1 AND c.visibility='published'`, a.ID).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, `SELECT c.*,cat.name AS category_name,f.created_at AS favorited_at FROM catalog.favorites f JOIN catalog.companies c ON c.id=f.company_id LEFT JOIN catalog.categories cat ON cat.id=c.category_id WHERE f.developer_id=$1 AND c.visibility='published' ORDER BY f.created_at DESC,c.id LIMIT $2 OFFSET $3`, a.ID, p.Limit, p.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, p)
}
func (s *Server) catalogPutFavorite(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	id, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = catalogLockPublished(ctx, tx, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO catalog.favorites(developer_id,company_id) VALUES($1,$2) ON CONFLICT(developer_id,company_id) DO NOTHING`, a.ID, id); err != nil {
		return err
	}
	item, err := oneJSON(ctx, tx, `SELECT * FROM catalog.favorites WHERE developer_id=$1 AND company_id=$2`, a.ID, id)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}
func (s *Server) catalogDeleteFavorite(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	id, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(r.Context(), `DELETE FROM catalog.favorites WHERE developer_id=$1 AND company_id=$2`, a.ID, id)
	if err != nil {
		return err
	}
	return respond(w, http.StatusNoContent, nil)
}
func (s *Server) catalogListContacts(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	p, err := pagination(r)
	if err != nil {
		return err
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM catalog.company_contacts WHERE developer_id=$1`, a.ID).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, `SELECT * FROM catalog.company_contacts WHERE developer_id=$1 ORDER BY updated_at DESC,company_id LIMIT $2 OFFSET $3`, a.ID, p.Limit, p.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, p)
}
func (s *Server) catalogGetContact(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	id, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	item, err := oneJSON(r.Context(), s.DB, `SELECT * FROM catalog.company_contacts WHERE developer_id=$1 AND company_id=$2`, a.ID, id)
	if err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}
func catalogContactFields(raw map[string]json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, invalid("body", "Укажите статус или данные отметки")
	}
	out := map[string]any{}
	for k, v := range raw {
		if !oneOf(k, "status", "notes", "contacted_at") {
			return nil, invalid(k, "Поле недоступно для изменения")
		}
		if string(v) == "null" {
			if k == "status" {
				return nil, invalid(k, "Статус не может быть null")
			}
			out[k] = nil
			continue
		}
		var str string
		if err := json.Unmarshal(v, &str); err != nil {
			return nil, invalid(k, "Ожидается строка")
		}
		switch k {
		case "status":
			if !oneOf(str, "planned", "contacted", "replied", "declined") {
				return nil, invalid(k, "Неизвестный статус контакта")
			}
			out[k] = str
		case "notes":
			if utf8.RuneCountInString(str) > 10000 {
				return nil, invalid(k, "Не более 10000 символов")
			}
			out[k] = str
		case "contacted_at":
			value, err := time.Parse(time.RFC3339Nano, str)
			if err != nil {
				return nil, invalid(k, "Ожидается ISO 8601 с часовым поясом")
			}
			out[k] = value.UTC()
		}
	}
	return out, nil
}
func (s *Server) catalogPutContact(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	id, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err = readJSON(r, &raw); err != nil {
		return err
	}
	fields, err := catalogContactFields(raw)
	if err != nil {
		return err
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = catalogLockPublished(ctx, tx, id); err != nil {
		return err
	}
	_, hasStatus := fields["status"]
	_, hasNotes := fields["notes"]
	_, hasAt := fields["contacted_at"]
	item, err := changeJSON(ctx, tx, `INSERT INTO catalog.company_contacts AS cc(developer_id,company_id,status,notes,contacted_at) VALUES($1,$2,COALESCE($3::text,'planned'),$4,$5) ON CONFLICT(developer_id,company_id) DO UPDATE SET status=CASE WHEN $6::boolean THEN EXCLUDED.status ELSE cc.status END,notes=CASE WHEN $7::boolean THEN EXCLUDED.notes ELSE cc.notes END,contacted_at=CASE WHEN $8::boolean THEN EXCLUDED.contacted_at ELSE cc.contacted_at END RETURNING *`, a.ID, id, fields["status"], fields["notes"], fields["contacted_at"], hasStatus, hasNotes, hasAt)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}
func (s *Server) catalogDeleteContact(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	id, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(r.Context(), `DELETE FROM catalog.company_contacts WHERE developer_id=$1 AND company_id=$2`, a.ID, id)
	if err != nil {
		return err
	}
	return respond(w, http.StatusNoContent, nil)
}

func (s *Server) catalogCreateCorrection(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "developer", "moderator")
	if err != nil {
		return err
	}
	id, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	var body struct {
		Message string `json:"message"`
	}
	if err = readJSON(r, &body); err != nil {
		return err
	}
	message, err := catalogRequiredText("message", body.Message, 10000)
	if err != nil {
		return err
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = catalogLockPublished(ctx, tx, id); err != nil {
		return err
	}
	item, err := changeJSON(ctx, tx, `INSERT INTO catalog.correction_requests(company_id,requested_by,message) VALUES($1,$2,$3) RETURNING *`, id, a.ID, message)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return respond(w, http.StatusCreated, item)
}
func (s *Server) catalogOwnCorrections(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "developer", "moderator")
	if err != nil {
		return err
	}
	return s.catalogCorrectionsPage(w, r, a.ID)
}
func (s *Server) catalogModerationCorrections(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	return s.catalogCorrectionsPage(w, r, "")
}
func (s *Server) catalogModerationCorrection(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	id, err := pathID(r, "request_id")
	if err != nil {
		return err
	}
	item, err := oneJSON(r.Context(), s.DB, `SELECT cr.*,c.name AS company_name,p.name AS requested_by_name,to_jsonb(c) AS current_company FROM catalog.correction_requests cr JOIN catalog.companies c ON c.id=cr.company_id JOIN profiles.profiles p ON p.user_id=cr.requested_by WHERE cr.id=$1`, id)
	if err != nil {
		return err
	}
	return respond(w, 200, item)
}
func (s *Server) catalogCorrectionsPage(w http.ResponseWriter, r *http.Request, ownerID string) error {
	p, err := pagination(r)
	if err != nil {
		return err
	}
	status := r.URL.Query().Get("status")
	if status != "" && !oneOf(status, "pending", "approved", "rejected") {
		return invalid("status", "Неизвестный статус исправления")
	}
	args := []any{ownerID, status}
	where := `($1::text='' OR cr.requested_by=NULLIF($1,'')::uuid) AND ($2::text='' OR cr.status=$2)`
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM catalog.correction_requests cr WHERE `+where, args...).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, `SELECT cr.*,c.name AS company_name,p.name AS requested_by_name FROM catalog.correction_requests cr JOIN catalog.companies c ON c.id=cr.company_id JOIN profiles.profiles p ON p.user_id=cr.requested_by WHERE `+where+` ORDER BY cr.created_at DESC,cr.id LIMIT $3 OFFSET $4`, ownerID, status, p.Limit, p.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, p)
}

type catalogResolution struct {
	Status         string          `json:"status"`
	ResolutionNote *string         `json:"resolution_note"`
	Changes        json.RawMessage `json:"changes"`
}

func catalogResolutionFields(body catalogResolution) (map[string]any, error) {
	if !oneOf(body.Status, "approved", "rejected") {
		return nil, invalid("status", "Допустимы approved или rejected")
	}
	if body.ResolutionNote != nil && utf8.RuneCountInString(*body.ResolutionNote) > 10000 {
		return nil, invalid("resolution_note", "Не более 10000 символов")
	}
	if len(body.Changes) == 0 {
		return nil, nil
	}
	if body.Status != "approved" {
		return nil, invalid("changes", "Исправления применяются только при approved")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body.Changes, &raw); err != nil {
		return nil, invalid("changes", "Ожидается объект изменяемых полей компании")
	}
	return catalogCompanyFields(raw, false)
}
func (s *Server) catalogResolveCorrection(w http.ResponseWriter, r *http.Request) error {
	a, err := s.requireRole(r, "moderator")
	if err != nil {
		return err
	}
	id, err := pathID(r, "request_id")
	if err != nil {
		return err
	}
	var body catalogResolution
	if err = readJSON(r, &body); err != nil {
		return err
	}
	changes, err := catalogResolutionFields(body)
	if err != nil {
		return err
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var companyID, status string
	if err = tx.QueryRow(ctx, `SELECT company_id,status FROM catalog.correction_requests WHERE id=$1 FOR UPDATE`, id).Scan(&companyID, &status); err != nil {
		return err
	}
	if status != "pending" {
		return problem(http.StatusConflict, "CORRECTION_ALREADY_RESOLVED", "Заявка уже обработана")
	}
	if len(changes) > 0 {
		var locked string
		if err = tx.QueryRow(ctx, `SELECT id FROM catalog.companies WHERE id=$1 FOR UPDATE`, companyID).Scan(&locked); err != nil {
			return err
		}
		if _, err = catalogUpdateCompany(ctx, tx, companyID, changes); err != nil {
			return err
		}
	}
	item, err := changeJSON(ctx, tx, `UPDATE catalog.correction_requests SET status=$2,resolved_by=$3,resolution_note=$4,resolved_at=now() WHERE id=$1 RETURNING *`, id, body.Status, a.ID, body.ResolutionNote)
	if err != nil {
		return err
	}
	action := "approve"
	if body.Status == "rejected" {
		action = "reject"
	}
	if err = catalogAudit(ctx, tx, a.ID, companyID, action, body.ResolutionNote); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}

func catalogDateFilter(field, value string, end bool) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", value); err == nil {
		if end {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, invalid(field, "Ожидается YYYY-MM-DD или дата-время ISO 8601")
}
func (s *Server) catalogModerationActions(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	p, err := pagination(r)
	if err != nil {
		return err
	}
	query := r.URL.Query()
	args := []any{}
	clauses := []string{"TRUE"}
	add := func(column, op string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s %s $%d", column, op, len(args)))
	}
	for _, key := range []string{"company_id", "order_id", "moderator_id"} {
		if value := query.Get(key); value != "" {
			if !validUUID(value) {
				return invalid(key, "Некорректный UUID")
			}
			add(key, "=", value)
		}
	}
	if action := query.Get("action"); action != "" {
		if !oneOf(action, "publish", "hide", "correct", "approve", "reject") {
			return invalid("action", "Неизвестное действие")
		}
		add("action", "=", action)
	}
	objectType, objectID := query.Get("object_type"), query.Get("object_id")
	if objectType != "" {
		if !oneOf(objectType, "company", "order") {
			return invalid("object_type", "Допустимы company или order")
		}
		if objectID == "" {
			clauses = append(clauses, objectType+"_id IS NOT NULL")
		} else {
			if !validUUID(objectID) {
				return invalid("object_id", "Некорректный UUID")
			}
			add(objectType+"_id", "=", objectID)
		}
	} else if objectID != "" {
		return invalid("object_type", "Нужен тип объекта для object_id")
	}
	var from, to time.Time
	if value := query.Get("date_from"); value != "" {
		from, err = catalogDateFilter("date_from", value, false)
		if err != nil {
			return err
		}
		add("created_at", ">=", from)
	}
	if value := query.Get("date_to"); value != "" {
		to, err = catalogDateFilter("date_to", value, true)
		if err != nil {
			return err
		}
		add("created_at", "<=", to)
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return invalid("date_to", "Конец периода раньше начала")
	}
	where := strings.Join(clauses, " AND ")
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM catalog.moderation_actions WHERE `+where, args...).Scan(&total); err != nil {
		return err
	}
	args = append(args, p.Limit, p.Offset)
	sql := fmt.Sprintf(`SELECT * FROM catalog.moderation_actions WHERE %s ORDER BY created_at DESC,id LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))
	items, err := listJSON(r.Context(), s.DB, sql, args...)
	if err != nil {
		return err
	}
	return paged(w, items, total, p)
}
