package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type DB interface {
	Querier
	Begin(context.Context) (pgx.Tx, error)
	Ping(context.Context) error
}
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details"`
}

func (e *APIError) Error() string { return e.Message }
func problem(status int, code, message string) *APIError {
	return &APIError{status, code, message, map[string]any{}}
}
func invalid(field, message string) *APIError {
	e := problem(422, "VALIDATION_ERROR", "Проверьте поля запроса")
	e.Details = map[string]string{field: message}
	return e
}
func handle(h func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			writeError(w, r, err)
		}
	}
}
func respond(w http.ResponseWriter, status int, data any) error {
	if status == 204 {
		w.WriteHeader(status)
		return nil
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var e *APIError
	if !errors.As(err, &e) {
		e = problem(500, "INTERNAL_ERROR", "Не удалось выполнить запрос")
		var pg *pgconn.PgError
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			e = problem(404, "NOT_FOUND", "Объект не найден")
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			e = problem(503, "SERVICE_UNAVAILABLE", "Сервис временно недоступен")
		case errors.As(err, &pg):
			switch pg.Code {
			case "23505":
				e = problem(409, "ALREADY_EXISTS", "Такая запись уже существует")
			case "23503":
				e = problem(409, "REFERENCE_CONFLICT", "Связанная запись не найдена или используется")
			case "23514", "23502", "22007", "22008", "22P02", "22003":
				e = problem(422, "VALIDATION_ERROR", "Данные нарушают ограничения")
			case "40001", "40P01":
				e = problem(409, "CONCURRENT_CHANGE", "Данные изменились. Повторите запрос")
			case "08006", "08003", "57P01", "53300":
				e = problem(503, "SERVICE_UNAVAILABLE", "База временно недоступна")
			}
		default:
			var connect *pgconn.ConnectError
			if errors.As(err, &connect) {
				e = problem(503, "SERVICE_UNAVAILABLE", "База временно недоступна")
			}
		}
	}
	if info := requestInfo(r); info != nil {
		info.errorCode = e.Code
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": e, "request_id": requestID(r)})
}
func readJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return problem(400, "BAD_REQUEST", "Тело запроса обязательно")
	}
	// A bounded reader is used even when this helper is called directly in tests.
	raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil {
		return problem(400, "BAD_REQUEST", "Не удалось прочитать запрос")
	}
	if len(raw) > 1<<20 {
		return problem(413, "PAYLOAD_TOO_LARGE", "Запрос слишком большой")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err = dec.Decode(dst); err != nil {
		return problem(400, "BAD_REQUEST", "Некорректный JSON или неизвестные поля")
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return problem(400, "BAD_REQUEST", "Ожидался один JSON-объект")
	}
	return nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validUUID(v string) bool { return uuidPattern.MatchString(v) }
func pathID(r *http.Request, name string) (string, error) {
	v := chi.URLParam(r, name)
	if !validUUID(v) {
		return "", problem(400, "INVALID_ID", "Некорректный UUID: "+name)
	}
	return v, nil
}
func oneOf(v string, choices ...string) bool {
	for _, s := range choices {
		if v == s {
			return true
		}
	}
	return false
}
func validateURL(field, value string) error {
	if value == "" {
		return nil
	}
	u, e := url.Parse(value)
	if e != nil || !oneOf(u.Scheme, "https", "http") || u.Hostname() == "" || u.User != nil {
		return invalid(field, "Укажите корректный http(s) URL")
	}
	return nil
}

type Page struct{ Number, Limit, Offset int }

func pagination(r *http.Request) (Page, error) {
	p := Page{Number: 1, Limit: 20}
	var err error
	for key, dst := range map[string]*int{"page": &p.Number, "limit": &p.Limit} {
		if v := r.URL.Query().Get(key); v != "" {
			*dst, err = strconv.Atoi(v)
			if err != nil || *dst < 1 {
				return p, problem(400, "INVALID_PAGINATION", "page и limit должны быть положительными числами")
			}
		}
	}
	if p.Limit > 100 || p.Number > 1000000 {
		return p, problem(400, "INVALID_PAGINATION", "limit ≤ 100, page ≤ 1000000")
	}
	p.Offset = (p.Number - 1) * p.Limit
	return p, nil
}
func paged(w http.ResponseWriter, items any, total int64, p Page) error {
	return respond(w, 200, map[string]any{"items": items, "total": total, "page": p.Number, "limit": p.Limit})
}
func listJSON(ctx context.Context, q Querier, sql string, args ...any) ([]map[string]any, error) {
	rows, err := q.Query(ctx, "SELECT to_jsonb(_row) FROM ("+strings.TrimSuffix(strings.TrimSpace(sql), ";")+") AS _row", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m map[string]any
		if err = json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func oneJSON(ctx context.Context, q Querier, sql string, args ...any) (map[string]any, error) {
	rows, err := listJSON(ctx, q, sql, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, pgx.ErrNoRows
	}
	return rows[0], nil
}
func changeJSON(ctx context.Context, q Querier, sql string, args ...any) (map[string]any, error) {
	var raw []byte
	err := q.QueryRow(ctx, "WITH changed AS ("+strings.TrimSuffix(strings.TrimSpace(sql), ";")+") SELECT to_jsonb(changed) FROM changed", args...).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(raw, &out)
	return out, err
}
func requireMember(ctx context.Context, q Querier, actorID, companyID string, ownerOnly bool) error {
	var role string
	err := q.QueryRow(ctx, "SELECT member_role FROM profiles.company_members WHERE company_id=$1 AND user_id=$2", companyID, actorID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return problem(403, "FORBIDDEN", "Нет прав на эту компанию")
	}
	if err != nil {
		return err
	}
	if ownerOnly && role != "owner" {
		return problem(403, "OWNER_REQUIRED", "Нужны права владельца компании")
	}
	return nil
}
func stringValue(m map[string]any, key string) string { v, _ := m[key].(string); return v }
func dbUnavailable() error {
	return problem(503, "SERVICE_UNAVAILABLE", "Подключение к базе не настроено")
}
func quoteColumn(name string) string { return pgx.Identifier{name}.Sanitize() }
