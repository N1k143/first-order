package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) registerUserWorkflows(r chi.Router) {
	r.Get("/me/notifications", handle(s.listNotifications))
	r.Post("/me/notifications/read-all", handle(s.readAllNotifications))
	r.Post("/me/notifications/{notification_id}/read", handle(s.readNotification))
	r.Get("/me/workspace", handle(s.myWorkspace))
	r.Get("/me/work", handle(s.myWork))
	r.Get("/me/company-applications", handle(s.myCompanyApplications))
	r.Get("/orders/{order_id}/participants", handle(s.orderParticipants))
	r.Post("/applications/{application_id}/reject", handle(s.rejectApplication))
}

const notificationSelect = `SELECT n.id,n.event,n.order_id,n.application_id,n.actor_id,n.created_at,n.read_at,n.order_title,n.company_name FROM profiles.notifications n`

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	unread := false
	if v := r.URL.Query().Get("unread"); v != "" {
		unread, err = strconv.ParseBool(v)
		if err != nil {
			return invalid("unread", "Ожидается true или false")
		}
	}
	var total, count int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FILTER(WHERE NOT $2 OR read_at IS NULL),count(*) FILTER(WHERE read_at IS NULL) FROM profiles.notifications WHERE user_id=$1`, actor.ID, unread).Scan(&total, &count); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, notificationSelect+` WHERE n.user_id=$1 AND (NOT $2 OR n.read_at IS NULL) ORDER BY n.created_at DESC,n.id DESC LIMIT $3 OFFSET $4`, actor.ID, unread, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return respond(w, 200, map[string]any{"items": items, "total": total, "unread_count": count, "page": page.Number, "limit": page.Limit})
}
func (s *Server) readNotification(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "notification_id")
	if err != nil {
		return err
	}
	if err = orderEmptyBody(r); err != nil {
		return err
	}
	item, err := changeJSON(r.Context(), s.DB, `UPDATE profiles.notifications SET read_at=COALESCE(read_at,now()) WHERE id=$1 AND user_id=$2 RETURNING id,read_at`, id, actor.ID)
	if err != nil {
		return err
	}
	return respond(w, 200, item)
}
func (s *Server) readAllNotifications(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	if err = orderEmptyBody(r); err != nil {
		return err
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE profiles.notifications SET read_at=now() WHERE user_id=$1 AND read_at IS NULL`, actor.ID)
	if err != nil {
		return err
	}
	return respond(w, 200, map[string]int64{"updated": tag.RowsAffected()})
}

// Business state and notifications commit together. A repeated action cannot
// create another notification for the same recipient, event and application.
func notifyCompany(ctx context.Context, tx pgx.Tx, actorID, orderID, applicationID, event string) error {
	_, err := tx.Exec(ctx, `INSERT INTO profiles.notifications(user_id,actor_id,order_id,application_id,event,order_title,company_name) SELECT m.user_id,$1,$2,$3,$4,o.title,c.name FROM profiles.company_members m JOIN orders.orders o ON o.company_id=m.company_id JOIN catalog.companies c ON c.id=o.company_id WHERE o.id=$2 AND m.user_id<>$1::uuid ON CONFLICT(user_id,event,application_id) DO NOTHING`, actorID, orderID, applicationID, event)
	return err
}
func notifyApplicant(ctx context.Context, tx pgx.Tx, actorID, orderID, applicationID, event string) error {
	_, err := tx.Exec(ctx, `INSERT INTO profiles.notifications(user_id,actor_id,order_id,application_id,event,order_title,company_name) SELECT a.developer_id,$1,$2,a.id,$4,o.title,c.name FROM applications.applications a JOIN orders.orders o ON o.id=a.order_id JOIN catalog.companies c ON c.id=o.company_id WHERE a.id=$3 AND a.order_id=$2 AND a.developer_id<>$1::uuid ON CONFLICT(user_id,event,application_id) DO NOTHING`, actorID, orderID, applicationID, event)
	return err
}
func notifyDecision(ctx context.Context, tx pgx.Tx, actorID, orderID string, selected *string) error {
	_, err := tx.Exec(ctx, `INSERT INTO profiles.notifications(user_id,actor_id,order_id,application_id,event,order_title,company_name) SELECT a.developer_id,$1,a.order_id,a.id,CASE WHEN a.id=$3::uuid THEN 'application_accepted' ELSE 'application_rejected' END,o.title,c.name FROM applications.applications a JOIN orders.orders o ON o.id=a.order_id JOIN catalog.companies c ON c.id=o.company_id WHERE a.order_id=$2 AND a.status='submitted' AND a.developer_id<>$1::uuid ON CONFLICT(user_id,event,application_id) DO NOTHING`, actorID, orderID, selected)
	return err
}
