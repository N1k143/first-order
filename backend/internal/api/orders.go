package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) registerOrders(r chi.Router) {
	r.MethodFunc("GET", "/orders", handle(s.listOrders))
	r.MethodFunc("POST", "/orders", handle(s.createOrder))
	r.MethodFunc("GET", "/orders/{order_id}", handle(s.getOrder))
	r.MethodFunc("PATCH", "/orders/{order_id}", handle(s.updateOrder))
	r.MethodFunc("POST", "/orders/{order_id}/publish", handle(s.publishOrder))
	r.MethodFunc("POST", "/orders/{order_id}/close", handle(s.closeOrder))
	r.MethodFunc("POST", "/orders/{order_id}/complete", handle(s.completeOrder))
	// Visibility is independent of the order lifecycle: restoring a card must
	// never reopen a closed order or discard a selected contractor.
	r.MethodFunc("PATCH", "/orders/{order_id}/visibility", handle(s.setOrderVisibility))
	r.MethodFunc("POST", "/orders/{order_id}/applications", handle(s.createApplication))
	r.MethodFunc("GET", "/orders/{order_id}/applications", handle(s.listOrderApplications))
	r.MethodFunc("GET", "/me/applications", handle(s.listMyApplications))
	r.MethodFunc("GET", "/applications/{application_id}", handle(s.getApplication))
	r.MethodFunc("PATCH", "/applications/{application_id}", handle(s.updateApplication))
	r.MethodFunc("POST", "/applications/{application_id}/withdraw", handle(s.withdrawApplication))
	r.MethodFunc("POST", "/orders/{order_id}/select-application", handle(s.selectApplication))
}

const orderSelect = `SELECT o.*, c.name AS company_name, c.city,
 (SELECT count(*) FROM applications.applications ac WHERE ac.order_id=o.id) AS applications_count,
 (SELECT sa.developer_id FROM applications.applications sa WHERE sa.id=o.selected_application_id) AS contractor_id
 FROM orders.orders o JOIN catalog.companies c ON c.id=o.company_id`

const orderApplicationSelect = `SELECT a.*, p.name AS developer_name,
 o.title AS order_title, o.company_id, c.name AS company_name,
 o.status AS order_status,o.visibility AS order_visibility,
 (o.selected_application_id=a.id) IS TRUE AS is_selected
 FROM applications.applications a
 JOIN profiles.profiles p ON p.user_id=a.developer_id
 JOIN orders.orders o ON o.id=a.order_id
 JOIN catalog.companies c ON c.id=o.company_id`

// An actor can see publicly open orders, their company's orders, or an order
// assigned to them. Moderators may inspect every state. This expression is
// shared by list and detail so query filters cannot bypass visibility.
const orderReadACL = `( (o.status='open' AND o.visibility=true) OR $2::boolean
 OR EXISTS (SELECT 1 FROM profiles.company_members cm WHERE cm.company_id=o.company_id AND cm.user_id=$1::uuid)
 OR EXISTS (SELECT 1 FROM applications.applications sa WHERE sa.id=o.selected_application_id AND sa.developer_id=$1::uuid) )`

func orderActorID(a *Actor) *string {
	if a == nil || !validUUID(a.ID) {
		return nil
	}
	return &a.ID
}

func orderModerator(a *Actor) bool { return a != nil && a.Role == "moderator" }

func orderFilter(r *http.Request, actor *Actor) (string, []any, error) {
	q := r.URL.Query()
	clauses := []string{orderReadACL}
	args := []any{orderActorID(actor), orderModerator(actor)}
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if value := strings.TrimSpace(q.Get("q")); value != "" {
		if utf8.RuneCountInString(value) > 200 {
			return "", nil, invalid("q", "Поисковая строка должна быть не длиннее 200 символов")
		}
		add("(o.title ILIKE '%%' || $%d || '%%' OR o.description ILIKE '%%' || $%[1]d || '%%')", value)
	}
	if value := q.Get("company_id"); value != "" {
		if !validUUID(value) {
			return "", nil, invalid("company_id", "Ожидается UUID компании")
		}
		add("o.company_id=$%d::uuid", value)
	}
	if value := strings.TrimSpace(q.Get("city")); value != "" {
		if utf8.RuneCountInString(value) > 100 {
			return "", nil, invalid("city", "Название города слишком длинное")
		}
		add("c.city=$%d", value)
	}
	if value := q.Get("status"); value != "" {
		if !oneOf(value, "draft", "open", "contractor_selected", "completed", "closed", "hidden") {
			return "", nil, invalid("status", "Неизвестный статус заказа")
		}
		add("o.status=$%d", value)
	}
	var lower, upper *float64
	for _, field := range []string{"budget_from", "budget_to"} {
		if value := q.Get(field); value != "" {
			number, err := strconv.ParseFloat(value, 64)
			if err != nil || orderMoney(field, &number, false) != nil {
				return "", nil, invalid(field, "Укажите конечное неотрицательное число")
			}
			if field == "budget_from" {
				lower = &number
				add("COALESCE(o.budget_max,o.budget_min)>=$%d::numeric", number)
			} else {
				upper = &number
				add("COALESCE(o.budget_min,o.budget_max)<=$%d::numeric", number)
			}
		}
	}
	if lower != nil && upper != nil && *upper < *lower {
		return "", nil, invalid("budget_to", "Верхняя граница меньше нижней")
	}
	if value := q.Get("deadline_before"); value != "" {
		if err := orderDate("deadline_before", &value, false); err != nil {
			return "", nil, err
		}
		add("o.deadline<=$%d::date", value)
	}
	if value := q.Get("mine"); value != "" {
		mine, err := strconv.ParseBool(value)
		if err != nil {
			return "", nil, invalid("mine", "Ожидается true или false")
		}
		if mine {
			if orderActorID(actor) == nil {
				return "", nil, problem(http.StatusUnauthorized, "UNAUTHORIZED", "Войдите в аккаунт")
			}
			clauses = append(clauses, `EXISTS (SELECT 1 FROM profiles.company_members own WHERE own.company_id=o.company_id AND own.user_id=$1::uuid)`)
		}
	}
	return strings.Join(clauses, " AND "), args, nil
}

func (s *Server) listOrders(w http.ResponseWriter, r *http.Request) error {
	if s.DB == nil {
		return dbUnavailable()
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	where, args, err := orderFilter(r, s.actor(r))
	if err != nil {
		return err
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM orders.orders o JOIN catalog.companies c ON c.id=o.company_id WHERE `+where, args...).Scan(&total); err != nil {
		return err
	}
	args = append(args, page.Limit, page.Offset)
	items, err := listJSON(r.Context(), s.DB, orderSelect+` WHERE `+where+fmt.Sprintf(` ORDER BY o.created_at DESC,o.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return err
	}
	return paged(w, items, total, page)
}

func (s *Server) getOrder(w http.ResponseWriter, r *http.Request) error {
	if s.DB == nil {
		return dbUnavailable()
	}
	id, err := pathID(r, "order_id")
	if err != nil {
		return err
	}
	actor := s.actor(r)
	item, err := oneJSON(r.Context(), s.DB, orderSelect+` WHERE `+orderReadACL+` AND o.id=$3::uuid`, orderActorID(actor), orderModerator(actor), id)
	if err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}

// Set distinguishes an omitted PATCH property from an explicit null. Nullable
// budgets and dates can be cleared, while title/description cannot be null.
type orderField[T any] struct {
	Set   bool
	Value *T
}

func (f *orderField[T]) UnmarshalJSON(data []byte) error {
	f.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		f.Value = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	f.Value = &value
	return nil
}

type orderCreateDTO struct {
	CompanyID   string   `json:"company_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	BudgetMin   *float64 `json:"budget_min"`
	BudgetMax   *float64 `json:"budget_max"`
	Deadline    *string  `json:"deadline"`
}

type orderPatchDTO struct {
	Title       orderField[string]  `json:"title"`
	Description orderField[string]  `json:"description"`
	BudgetMin   orderField[float64] `json:"budget_min"`
	BudgetMax   orderField[float64] `json:"budget_max"`
	Deadline    orderField[string]  `json:"deadline"`
}

func orderText(field string, value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalid(field, "Поле обязательно")
	}
	if utf8.RuneCountInString(value) > max {
		return "", invalid(field, fmt.Sprintf("Не более %d символов", max))
	}
	return value, nil
}

func orderMoney(field string, value *float64, applicationPrice bool) error {
	if value == nil {
		return nil
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 {
		return invalid(field, "Укажите конечное неотрицательное число")
	}
	// The actual application column is numeric(12,2). Reject out-of-range
	// prices before PostgreSQL raises an otherwise opaque numeric overflow.
	if applicationPrice && *value > 9999999999.99 {
		return invalid(field, "Стоимость превышает 9 999 999 999,99 ₸")
	}
	if applicationPrice && math.Abs(*value*100-math.Round(*value*100)) > 0.0001 {
		return invalid(field, "Допускается не более двух знаков после запятой")
	}
	return nil
}

func orderBudgets(min, max *float64) error {
	if err := orderMoney("budget_min", min, false); err != nil {
		return err
	}
	if err := orderMoney("budget_max", max, false); err != nil {
		return err
	}
	if min != nil && max != nil && *max < *min {
		return invalid("budget_max", "Максимальный бюджет не может быть меньше минимального")
	}
	return nil
}

func orderDate(field string, value *string, future bool) error {
	if value == nil {
		return nil
	}
	date, err := time.Parse("2006-01-02", *value)
	if err != nil || date.Format("2006-01-02") != *value || date.Year() < 1 {
		return invalid(field, "Ожидается существующая дата YYYY-MM-DD или null")
	}
	if future && date.Before(time.Now().UTC().Truncate(24*time.Hour)) {
		return invalid(field, "Дата не может быть в прошлом")
	}
	return nil
}

func (d *orderCreateDTO) validate() error {
	if !validUUID(d.CompanyID) {
		return invalid("company_id", "Ожидается UUID компании")
	}
	var err error
	if d.Title, err = orderText("title", d.Title, 200); err != nil {
		return err
	}
	if d.Description, err = orderText("description", d.Description, 20000); err != nil {
		return err
	}
	if err = orderBudgets(d.BudgetMin, d.BudgetMax); err != nil {
		return err
	}
	return orderDate("deadline", d.Deadline, true)
}

// Every existing-order mutation acquires this lock first, including application
// submissions, updates and withdrawals. It is the shared serialization point
// for selection vs withdrawal and for selection vs a newly submitted reply.
const orderLockSelect = `SELECT o.id::text,o.company_id::text,o.created_by::text,o.status,o.visibility,
 o.selected_application_id::text,o.title,o.description,o.budget_min::float8,o.budget_max::float8,o.deadline::text
 FROM orders.orders o`

type orderLocked struct {
	Deadline                         *string
	ID, CompanyID, CreatedBy, Status string
	Visibility                       bool
	SelectedApplicationID            *string
	Title, Description               string
	BudgetMin, BudgetMax             *float64
}

func orderScanLock(row pgx.Row) (*orderLocked, error) {
	var o orderLocked
	err := row.Scan(&o.ID, &o.CompanyID, &o.CreatedBy, &o.Status, &o.Visibility, &o.SelectedApplicationID, &o.Title, &o.Description, &o.BudgetMin, &o.BudgetMax, &o.Deadline)
	return &o, err
}

func orderLock(ctx context.Context, tx pgx.Tx, id string) (*orderLocked, error) {
	return orderScanLock(tx.QueryRow(ctx, orderLockSelect+` WHERE o.id=$1::uuid FOR UPDATE OF o`, id))
}

func orderLockForApplication(ctx context.Context, tx pgx.Tx, applicationID string) (*orderLocked, error) {
	return orderScanLock(tx.QueryRow(ctx, orderLockSelect+` JOIN applications.applications a ON a.order_id=o.id WHERE a.id=$1::uuid FOR UPDATE OF o`, applicationID))
}

func orderState(status string, allowed ...string) error {
	if !oneOf(status, allowed...) {
		return problem(http.StatusConflict, "ORDER_STATE_CONFLICT", "Действие недоступно в текущем состоянии заказа")
	}
	return nil
}

func orderMember(ctx context.Context, q Querier, actor *Actor, companyID string) error {
	if orderActorID(actor) == nil {
		return problem(http.StatusForbidden, "FORBIDDEN", "Нет права управлять этой компанией")
	}
	if _, inTransaction := q.(pgx.Tx); inTransaction {
		// Hold membership until commit so concurrent revocation cannot race
		// the authorization check. Existing orders are always locked first.
		var memberID string
		err := q.QueryRow(ctx, `SELECT user_id::text FROM profiles.company_members WHERE company_id=$1::uuid AND user_id=$2::uuid FOR SHARE`, companyID, actor.ID).Scan(&memberID)
		if errors.Is(err, pgx.ErrNoRows) {
			return problem(http.StatusForbidden, "FORBIDDEN", "Нет права управлять этой компанией")
		}
		return err
	}
	return requireMember(ctx, q, actor.ID, companyID, false)
}

func orderCommit(w http.ResponseWriter, r *http.Request, tx pgx.Tx, id string, status int) error {
	item, err := oneJSON(r.Context(), tx, orderSelect+` WHERE o.id=$1::uuid`, id)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, status, item)
}

func orderApplicationCommit(w http.ResponseWriter, r *http.Request, tx pgx.Tx, id string, status int) error {
	item, err := oneJSON(r.Context(), tx, orderApplicationSelect+` WHERE a.id=$1::uuid`, id)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, status, item)
}

func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	var d orderCreateDTO
	if err = readJSON(r, &d); err != nil {
		return err
	}
	if err = d.validate(); err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	// Membership management locks company before members. Match that order
	// for creation, where there is no order row to lock yet.
	var companyID string
	if err = tx.QueryRow(r.Context(), `SELECT id::text FROM catalog.companies WHERE id=$1::uuid FOR KEY SHARE`, d.CompanyID).Scan(&companyID); err != nil {
		return err
	}
	if err = orderMember(r.Context(), tx, actor, d.CompanyID); err != nil {
		return err
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO orders.orders(company_id,created_by,title,description,budget_min,budget_max,deadline,status)
 VALUES($1::uuid,$2::uuid,$3,$4,$5::numeric,$6::numeric,$7::date,'draft') RETURNING id::text`, d.CompanyID, actor.ID, d.Title, d.Description, d.BudgetMin, d.BudgetMax, d.Deadline).Scan(&id)
	if err != nil {
		return err
	}
	return orderCommit(w, r, tx, id, http.StatusCreated)
}

func (s *Server) updateOrder(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "order_id")
	if err != nil {
		return err
	}
	var d orderPatchDTO
	if err = readJSON(r, &d); err != nil {
		return err
	}
	if !d.Title.Set && !d.Description.Set && !d.BudgetMin.Set && !d.BudgetMax.Set && !d.Deadline.Set {
		return invalid("body", "Укажите хотя бы одно изменяемое поле")
	}
	for _, field := range []struct {
		name string
		val  *orderField[string]
		max  int
	}{{"title", &d.Title, 200}, {"description", &d.Description, 20000}} {
		if field.val.Set {
			if field.val.Value == nil {
				return invalid(field.name, "Поле не может быть null")
			}
			text, textErr := orderText(field.name, *field.val.Value, field.max)
			if textErr != nil {
				return textErr
			}
			field.val.Value = &text
		}
	}
	if d.Deadline.Set {
		if err = orderDate("deadline", d.Deadline.Value, true); err != nil {
			return err
		}
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	order, err := orderLock(r.Context(), tx, id)
	if err != nil {
		return err
	}
	if err = orderMember(r.Context(), tx, actor, order.CompanyID); err != nil {
		return err
	}
	if err = orderState(order.Status, "draft", "open"); err != nil {
		return err
	}
	min, max := order.BudgetMin, order.BudgetMax
	if d.BudgetMin.Set {
		min = d.BudgetMin.Value
	}
	if d.BudgetMax.Set {
		max = d.BudgetMax.Value
	}
	if err = orderBudgets(min, max); err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `UPDATE orders.orders SET
 title=CASE WHEN $2 THEN $3::text ELSE title END,
 description=CASE WHEN $4 THEN $5::text ELSE description END,
 budget_min=CASE WHEN $6 THEN $7::numeric ELSE budget_min END,
 budget_max=CASE WHEN $8 THEN $9::numeric ELSE budget_max END,
 deadline=CASE WHEN $10 THEN $11::date ELSE deadline END, updated_at=now()
 WHERE id=$1::uuid`, id, d.Title.Set, d.Title.Value, d.Description.Set, d.Description.Value,
		d.BudgetMin.Set, d.BudgetMin.Value, d.BudgetMax.Set, d.BudgetMax.Value, d.Deadline.Set, d.Deadline.Value)
	if err != nil {
		return err
	}
	return orderCommit(w, r, tx, id, http.StatusOK)
}

// Action routes accept an empty body or {}. Unknown data (including actor or
// status overrides) is rejected even though these operations need no payload.
func orderEmptyBody(r *http.Request) error {
	if r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 {
		return nil
	}
	var empty struct{}
	return readJSON(r, &empty)
}

func (s *Server) publishOrder(w http.ResponseWriter, r *http.Request) error {
	return s.orderTransition(w, r, "open")
}

func (s *Server) closeOrder(w http.ResponseWriter, r *http.Request) error {
	return s.orderTransition(w, r, "closed")
}

func (s *Server) completeOrder(w http.ResponseWriter, r *http.Request) error {
	return s.orderTransition(w, r, "completed")
}

func (s *Server) orderTransition(w http.ResponseWriter, r *http.Request, next string) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "order_id")
	if err != nil {
		return err
	}
	if err = orderEmptyBody(r); err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	order, err := orderLock(r.Context(), tx, id)
	if err != nil {
		return err
	}
	if next != "closed" || !orderModerator(actor) {
		if err = orderMember(r.Context(), tx, actor, order.CompanyID); err != nil {
			return err
		}
	}
	if order.Status == next {
		return orderCommit(w, r, tx, id, http.StatusOK)
	}
	switch next {
	case "open":
		err = orderState(order.Status, "draft")
		if err == nil {
			err = orderDate("deadline", order.Deadline, true)
		}
	case "closed":
		err = orderState(order.Status, "draft", "open", "contractor_selected")
	case "completed":
		err = orderState(order.Status, "contractor_selected")
	default:
		return problem(http.StatusInternalServerError, "INTERNAL_ERROR", "Неизвестный переход заказа")
	}
	if err != nil {
		return err
	}
	if next == "closed" {
		if err = notifyDecision(r.Context(), tx, actor.ID, id, nil); err != nil {
			return err
		}
		if order.SelectedApplicationID != nil {
			if err = notifyApplicant(r.Context(), tx, actor.ID, id, *order.SelectedApplicationID, "order_closed"); err != nil {
				return err
			}
		}
		// Accepted replies and selected_application_id remain as history of the
		// cancelled engagement. Pending replies can no longer be selected.
		_, err = tx.Exec(r.Context(), `UPDATE applications.applications SET status='rejected',updated_at=now() WHERE order_id=$1::uuid AND status='submitted'`, id)
		if err != nil {
			return err
		}
	}
	if next == "completed" && order.SelectedApplicationID != nil {
		if err = notifyApplicant(r.Context(), tx, actor.ID, id, *order.SelectedApplicationID, "order_completed"); err != nil {
			return err
		}
	}
	_, err = tx.Exec(r.Context(), `UPDATE orders.orders SET status=$2,updated_at=now() WHERE id=$1::uuid`, id, next)
	if err != nil {
		return err
	}
	return orderCommit(w, r, tx, id, http.StatusOK)
}

func (s *Server) setOrderVisibility(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireRole(r, "moderator")
	if err != nil {
		return err
	}
	id, err := pathID(r, "order_id")
	if err != nil {
		return err
	}
	var d struct {
		Visibility *bool  `json:"visibility"`
		Reason     string `json:"reason"`
	}
	if err = readJSON(r, &d); err != nil {
		return err
	}
	if d.Visibility == nil {
		return invalid("visibility", "Укажите true или false")
	}
	d.Reason = strings.TrimSpace(d.Reason)
	if utf8.RuneCountInString(d.Reason) > 2000 {
		return invalid("reason", "Причина должна быть не длиннее 2000 символов")
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	order, err := orderLock(r.Context(), tx, id)
	if err != nil {
		return err
	}
	if order.Visibility != *d.Visibility {
		if _, err = tx.Exec(r.Context(), `UPDATE orders.orders SET visibility=$2,updated_at=now() WHERE id=$1::uuid`, id, *d.Visibility); err != nil {
			return err
		}
		action := "hide"
		if *d.Visibility {
			action = "publish"
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO catalog.moderation_actions(moderator_id,order_id,action,reason) VALUES($1::uuid,$2::uuid,$3,NULLIF($4,''))`, actor.ID, id, action, d.Reason)
		if err != nil {
			return err
		}
	}
	return orderCommit(w, r, tx, id, http.StatusOK)
}

type orderApplicationCreateDTO struct {
	Message          string   `json:"message"`
	ProposedPrice    *float64 `json:"proposed_price"`
	ProposedDeadline *string  `json:"proposed_deadline"`
}

type orderApplicationPatchDTO struct {
	Message          orderField[string]  `json:"message"`
	ProposedPrice    orderField[float64] `json:"proposed_price"`
	ProposedDeadline orderField[string]  `json:"proposed_deadline"`
}

func (s *Server) createApplication(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	orderID, err := pathID(r, "order_id")
	if err != nil {
		return err
	}
	var d orderApplicationCreateDTO
	if err = readJSON(r, &d); err != nil {
		return err
	}
	if d.Message, err = orderText("message", d.Message, 10000); err != nil {
		return err
	}
	if err = orderMoney("proposed_price", d.ProposedPrice, true); err != nil {
		return err
	}
	if err = orderDate("proposed_deadline", d.ProposedDeadline, true); err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	order, err := orderLock(r.Context(), tx, orderID)
	if err != nil {
		return err
	}
	if err = orderState(order.Status, "open"); err != nil {
		return err
	}
	if !order.Visibility {
		return problem(http.StatusNotFound, "NOT_FOUND", "Заказ недоступен")
	}
	var member, duplicate bool
	err = tx.QueryRow(r.Context(), `SELECT
 EXISTS(SELECT 1 FROM profiles.company_members WHERE company_id=$1::uuid AND user_id=$2::uuid),
 EXISTS(SELECT 1 FROM applications.applications WHERE order_id=$3::uuid AND developer_id=$2::uuid)`, order.CompanyID, actor.ID, orderID).Scan(&member, &duplicate)
	if err != nil {
		return err
	}
	if member || strings.EqualFold(order.CreatedBy, actor.ID) {
		return problem(http.StatusForbidden, "SELF_APPLICATION_FORBIDDEN", "Нельзя откликнуться на заказ своей компании")
	}
	if duplicate {
		return problem(http.StatusConflict, "APPLICATION_ALREADY_EXISTS", "На этот заказ уже отправлен отклик, в том числе отозванный")
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO applications.applications(order_id,developer_id,message,proposed_price,proposed_deadline,status)
 VALUES($1::uuid,$2::uuid,$3,$4::numeric,$5::date,'submitted') RETURNING id::text`, orderID, actor.ID, d.Message, d.ProposedPrice, d.ProposedDeadline).Scan(&id)
	if err != nil {
		return err
	}
	if err = notifyCompany(r.Context(), tx, actor.ID, orderID, id, "application_submitted"); err != nil {
		return err
	}
	return orderApplicationCommit(w, r, tx, id, http.StatusCreated)
}

func orderApplicationStatusFilter(r *http.Request) (string, error) {
	status := r.URL.Query().Get("status")
	if status != "" && !oneOf(status, "submitted", "accepted", "rejected", "withdrawn") {
		return "", invalid("status", "Неизвестный статус отклика")
	}
	return status, nil
}

func (s *Server) listOrderApplications(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "order_id")
	if err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	status, err := orderApplicationStatusFilter(r)
	if err != nil {
		return err
	}
	var companyID string
	if err = s.DB.QueryRow(r.Context(), `SELECT company_id::text FROM orders.orders WHERE id=$1::uuid`, id).Scan(&companyID); err != nil {
		return err
	}
	if !orderModerator(actor) {
		if err = orderMember(r.Context(), s.DB, actor, companyID); err != nil {
			return err
		}
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM applications.applications WHERE order_id=$1::uuid AND ($2::text='' OR status=$2)`, id, status).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, orderApplicationSelect+` WHERE a.order_id=$1::uuid AND ($2::text='' OR a.status=$2) ORDER BY a.created_at DESC,a.id DESC LIMIT $3 OFFSET $4`, id, status, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, page)
}

func (s *Server) listMyApplications(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	status, err := orderApplicationStatusFilter(r)
	if err != nil {
		return err
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM applications.applications WHERE developer_id=$1::uuid AND ($2::text='' OR status=$2)`, actor.ID, status).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, orderApplicationSelect+` WHERE a.developer_id=$1::uuid AND ($2::text='' OR a.status=$2) ORDER BY a.created_at DESC,a.id DESC LIMIT $3 OFFSET $4`, actor.ID, status, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, page)
}

func (s *Server) getApplication(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "application_id")
	if err != nil {
		return err
	}
	item, err := oneJSON(r.Context(), s.DB, orderApplicationSelect+` WHERE a.id=$1::uuid AND
 (a.developer_id=$2::uuid OR $3::boolean OR EXISTS (SELECT 1 FROM profiles.company_members cm WHERE cm.company_id=o.company_id AND cm.user_id=$2::uuid))`, id, orderActorID(actor), orderModerator(actor))
	if err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}

func orderLockApplication(ctx context.Context, tx pgx.Tx, id, orderID, actorID string) error {
	var owner, status string
	err := tx.QueryRow(ctx, `SELECT developer_id::text,status FROM applications.applications WHERE id=$1::uuid AND order_id=$2::uuid FOR UPDATE`, id, orderID).Scan(&owner, &status)
	if err != nil {
		return err
	}
	if !strings.EqualFold(owner, actorID) {
		return problem(http.StatusForbidden, "FORBIDDEN", "Изменять отклик может только его автор")
	}
	if status != "submitted" {
		return problem(http.StatusConflict, "APPLICATION_STATE_CONFLICT", "Изменить или отозвать можно только отправленный отклик")
	}
	return nil
}

func (s *Server) updateApplication(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "application_id")
	if err != nil {
		return err
	}
	var d orderApplicationPatchDTO
	if err = readJSON(r, &d); err != nil {
		return err
	}
	if !d.Message.Set && !d.ProposedPrice.Set && !d.ProposedDeadline.Set {
		return invalid("body", "Укажите хотя бы одно изменяемое поле")
	}
	if d.Message.Set {
		if d.Message.Value == nil {
			return invalid("message", "Сообщение не может быть null")
		}
		text, textErr := orderText("message", *d.Message.Value, 10000)
		if textErr != nil {
			return textErr
		}
		d.Message.Value = &text
	}
	if d.ProposedPrice.Set {
		if err = orderMoney("proposed_price", d.ProposedPrice.Value, true); err != nil {
			return err
		}
	}
	if d.ProposedDeadline.Set {
		if err = orderDate("proposed_deadline", d.ProposedDeadline.Value, true); err != nil {
			return err
		}
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	order, err := orderLockForApplication(r.Context(), tx, id)
	if err != nil {
		return err
	}
	if err = orderLockApplication(r.Context(), tx, id, order.ID, actor.ID); err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `UPDATE applications.applications SET
 message=CASE WHEN $2 THEN $3::text ELSE message END,
 proposed_price=CASE WHEN $4 THEN $5::numeric ELSE proposed_price END,
 proposed_deadline=CASE WHEN $6 THEN $7::date ELSE proposed_deadline END,updated_at=now()
 WHERE id=$1::uuid`, id, d.Message.Set, d.Message.Value, d.ProposedPrice.Set, d.ProposedPrice.Value, d.ProposedDeadline.Set, d.ProposedDeadline.Value)
	if err != nil {
		return err
	}
	return orderApplicationCommit(w, r, tx, id, http.StatusOK)
}

func (s *Server) withdrawApplication(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "application_id")
	if err != nil {
		return err
	}
	if err = orderEmptyBody(r); err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	order, err := orderLockForApplication(r.Context(), tx, id)
	if err != nil {
		return err
	}
	var owner, status string
	if err = tx.QueryRow(r.Context(), `SELECT developer_id::text,status FROM applications.applications WHERE id=$1 FOR UPDATE`, id).Scan(&owner, &status); err != nil {
		return err
	}
	if !strings.EqualFold(owner, actor.ID) {
		return problem(403, "FORBIDDEN", "Отозвать отклик может только его автор")
	}
	if status == "withdrawn" {
		return orderApplicationCommit(w, r, tx, id, http.StatusOK)
	}
	if err = orderLockApplication(r.Context(), tx, id, order.ID, actor.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `UPDATE applications.applications SET status='withdrawn',updated_at=now() WHERE id=$1::uuid`, id); err != nil {
		return err
	}
	if err = notifyCompany(r.Context(), tx, actor.ID, order.ID, id, "application_withdrawn"); err != nil {
		return err
	}
	return orderApplicationCommit(w, r, tx, id, http.StatusOK)
}

func (s *Server) selectApplication(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	orderID, err := pathID(r, "order_id")
	if err != nil {
		return err
	}
	var d struct {
		ApplicationID string `json:"application_id"`
	}
	if err = readJSON(r, &d); err != nil {
		return err
	}
	if !validUUID(d.ApplicationID) {
		return invalid("application_id", "Ожидается UUID отклика")
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	order, err := orderLock(r.Context(), tx, orderID)
	if err != nil {
		return err
	}
	if err = orderMember(r.Context(), tx, actor, order.CompanyID); err != nil {
		return err
	}
	if order.SelectedApplicationID != nil && *order.SelectedApplicationID == d.ApplicationID && oneOf(order.Status, "contractor_selected", "completed") {
		return orderCommit(w, r, tx, orderID, http.StatusOK)
	}
	if err = orderState(order.Status, "open"); err != nil {
		return err
	}
	if order.SelectedApplicationID != nil {
		return problem(http.StatusConflict, "CONTRACTOR_ALREADY_SELECTED", "Исполнитель уже выбран")
	}
	var status string
	err = tx.QueryRow(r.Context(), `SELECT status FROM applications.applications WHERE id=$1::uuid AND order_id=$2::uuid FOR UPDATE`, d.ApplicationID, orderID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return problem(http.StatusNotFound, "APPLICATION_NOT_FOUND", "Отклик не найден для этого заказа")
	}
	if err != nil {
		return err
	}
	if status != "submitted" {
		return problem(http.StatusConflict, "APPLICATION_STATE_CONFLICT", "Выбрать можно только отправленный отклик")
	}
	if err = notifyDecision(r.Context(), tx, actor.ID, orderID, &d.ApplicationID); err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `UPDATE applications.applications SET status=CASE WHEN id=$2::uuid THEN 'accepted' ELSE 'rejected' END,updated_at=now()
 WHERE order_id=$1::uuid AND status='submitted'`, orderID, d.ApplicationID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `UPDATE orders.orders SET selected_application_id=$2::uuid,status='contractor_selected',updated_at=now() WHERE id=$1::uuid`, orderID, d.ApplicationID)
	if err != nil {
		return err
	}
	return orderCommit(w, r, tx, orderID, http.StatusOK)
}
