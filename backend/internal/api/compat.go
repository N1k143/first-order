package api

// This adapter keeps the versioned API stable while serving the currently shipped
// frontend contract (same-origin /api proxy, camelCase DTOs and legacy paths).
import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"unicode"
)

func (s *Server) compatibility(canonical http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, "/companies") && !strings.HasPrefix(path, "/orders") && !strings.HasPrefix(path, "/me") && !strings.HasPrefix(path, "/profiles") && !strings.HasPrefix(path, "/categories") && !strings.HasPrefix(path, "/applications") && !strings.HasPrefix(path, "/imports") && !strings.HasPrefix(path, "/moderation") {
			writeError(w, r, problem(404, "NOT_FOUND", "Маршрут не найден"))
			return
		}
		req := r.Clone(r.Context())
		req.URL.Path = strings.ReplaceAll(strings.ReplaceAll(path, "/corrections", "/correction-requests"), "/me/contacts", "/me/company-contacts")
		if strings.HasPrefix(path, "/me/favorites/") && req.Method == "POST" {
			req.Method = "PUT"
		}
		if strings.HasPrefix(path, "/me/contacts/") && req.Method == "PATCH" {
			req.Method = "PUT"
		}
		q := req.URL.Query()
		for old, new := range map[string]string{"pageSize": "limit", "min": "budget_from", "max": "budget_to", "companyId": "company_id"} {
			if q.Has(old) {
				q.Set(new, q.Get(old))
				q.Del(old)
			}
		}
		if path == "/companies" {
			if q.Has("status") {
				q.Set("website_status", q.Get("status"))
				q.Del("status")
			}
			if !q.Has("limit") {
				q.Set("limit", "3")
			}
		}
		if q.Get("status") == "in_progress" {
			q.Set("status", "contractor_selected")
		}
		req.URL.RawQuery = q.Encode()
		if r.Body != nil && r.ContentLength != 0 {
			raw, e := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
			if e != nil || len(raw) > 1<<20 {
				writeError(w, r, problem(400, "BAD_REQUEST", "Некорректное тело запроса"))
				return
			}
			if len(bytes.TrimSpace(raw)) > 0 {
				var m map[string]any
				if json.Unmarshal(raw, &m) != nil || m == nil {
					writeError(w, r, problem(400, "BAD_REQUEST", "Ожидается JSON-объект"))
					return
				}
				converted := map[string]any{}
				for k, v := range m {
					converted[snake(k)] = v
				}
				if strings.HasSuffix(path, "/applications") && r.Method == "POST" {
					// Never use browser-generated actor, status or record IDs, even on the legacy endpoint.
					for _, k := range []string{"id", "profile_id", "developer_id", "order_id", "name", "status", "actor_id"} {
						delete(converted, k)
					}
					renameKey(converted, "price", "proposed_price")
					renameKey(converted, "deadline", "proposed_deadline")
					renameKey(converted, "letter", "message")
				}
				if strings.HasPrefix(path, "/me/contacts/") {
					delete(converted, "company_id")
					renameKey(converted, "note", "notes")
					renameKey(converted, "date", "contacted_at")
					if v, ok := converted["status"].(string); ok {
						if mapped, yes := map[string]string{"Планирую написать": "planned", "Написал": "contacted", "Ответили": "replied", "Договорились": "replied", "Не актуально": "declined"}[v]; yes {
							converted["status"] = mapped
						}
					}
					if v, ok := converted["contacted_at"].(string); ok && len(v) == 10 {
						converted["contacted_at"] = v + "T00:00:00Z"
					}
				}
				if strings.Contains(path, "/portfolio") {
					renameKey(converted, "url", "project_url")
				}
				raw, _ = json.Marshal(converted)
				req.Body = io.NopCloser(bytes.NewReader(raw))
				req.ContentLength = int64(len(raw))
			}
		}
		rec := httptest.NewRecorder()
		canonical.ServeHTTP(rec, req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chi.NewRouteContext())))
		for k, vs := range rec.Header() {
			w.Header()[k] = vs
		}
		if rec.Code >= 400 || rec.Code == 204 || !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		var data any
		if json.Unmarshal(rec.Body.Bytes(), &data) != nil {
			writeError(w, r, problem(500, "INTERNAL_ERROR", "Не удалось подготовить ответ"))
			return
		}
		data = frontendValue(data)
		if path == "/categories" || path == "/orders" || path == "/me/applications" || path == "/me/portfolio" || strings.HasSuffix(path, "/applications") || strings.HasSuffix(path, "/members") {
			if p, ok := data.(map[string]any); ok {
				if items, exists := p["items"]; exists {
					data = items
				}
			}
		}
		_ = respond(w, rec.Code, data)
	})
}
func renameKey(m map[string]any, old, new string) {
	if v, ok := m[old]; ok {
		if _, exists := m[new]; !exists {
			m[new] = v
		}
		delete(m, old)
	}
}
func snake(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsUpper(r) {
			b.WriteByte('_')
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func camel(s string) string {
	p := strings.Split(s, "_")
	for i := 1; i < len(p); i++ {
		if p[i] != "" {
			p[i] = strings.ToUpper(p[i][:1]) + p[i][1:]
		}
	}
	return strings.Join(p, "")
}
func frontendValue(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = frontendValue(v)
		}
		return out
	case map[string]any:
		m := map[string]any{}
		for k, v := range x {
			m[camel(k)] = frontendValue(v)
		}
		if _, ok := x["website_status"]; ok {
			m["initials"] = initials(stringValue(x, "name"))
			if _, ok := m["category"]; !ok {
				m["category"] = stringValue(x, "category_name")
			}
			m["website"] = x["website_url"]
			m["checkedAt"] = x["website_checked_at"]
			if m["checkedAt"] == nil {
				m["checkedAt"] = x["updated_at"]
			}
			if t, ok := m["checkedAt"].(string); ok && len(t) >= 10 {
				m["checkedAt"] = t[:10]
			}
			if m["description"] == nil {
				m["description"] = ""
			}
			if m["address"] == nil {
				m["address"] = ""
			}
		}
		if _, ok := x["budget_min"]; ok {
			// Keep optional budget/deadline values unchanged so reading and
			// re-saving a draft does not turn unset terms into zero/empty text.
			m["publishedAt"] = x["created_at"]
			if m["applicationsCount"] == nil {
				m["applicationsCount"] = 0
			}
			if m["status"] == "contractor_selected" {
				m["status"] = "in_progress"
			}
		}
		if _, ok := x["developer_id"]; ok {
			if _, app := x["order_id"]; app {
				m["profileId"] = x["developer_id"]
				m["name"] = x["developer_name"]
				m["price"] = x["proposed_price"]
				m["deadline"] = x["proposed_deadline"]
				m["letter"] = x["message"]
				switch x["status"] {
				case "submitted":
					m["status"] = "pending"
				case "accepted":
					m["status"] = "selected"
				}
			}
		}
		if v, ok := x["project_url"]; ok {
			m["url"] = v
		}
		if v, ok := x["notes"]; ok {
			m["note"] = v
			m["date"] = x["contacted_at"]
			if t, ok := m["date"].(string); ok && len(t) >= 10 {
				m["date"] = t[:10]
			}
			if v, ok := x["status"].(string); ok {
				if label, yes := map[string]string{"planned": "Планирую написать", "contacted": "Написал", "replied": "Ответили", "declined": "Не актуально"}[v]; yes {
					m["status"] = label
				}
			}
		}
		if v, ok := x["member_role"]; ok {
			m["id"] = x["user_id"]
			m["role"] = v
			if v == "representative" {
				m["role"] = "member"
			}
		}
		if v, ok := x["limit"]; ok {
			m["pageSize"] = v
		}
		if v, ok := x["user_id"]; ok {
			if _, profile := x["bio"]; profile {
				m["id"] = v
				m["github"] = x["github_url"]
				m["skills"] = []string{}
				if skills, yes := x["skills"].(string); yes && skills != "" {
					m["skills"] = strings.Split(skills, ",")
				}
				if comps, yes := x["companies"].([]any); yes && len(comps) > 0 && m["role"] != "moderator" {
					m["role"] = "company"
				}
				if m["email"] == nil {
					m["email"] = ""
				}
				m["city"] = ""
				m["headline"] = ""
			}
		}
		return m
	default:
		return v
	}
}
func initials(name string) string {
	words := strings.Fields(name)
	var out []rune
	for _, w := range words {
		for _, r := range w {
			out = append(out, unicode.ToUpper(r))
			break
		}
		if len(out) == 2 {
			break
		}
	}
	return string(out)
}
