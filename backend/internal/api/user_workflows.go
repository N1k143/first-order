package api

import (
	"net/http"
)

func (s *Server) rejectApplication(w http.ResponseWriter, r *http.Request) error {
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
	defer rollbackImport(tx)
	order, err := orderLockForApplication(r.Context(), tx, id)
	if err != nil {
		return err
	}
	if err = orderMember(r.Context(), tx, actor, order.CompanyID); err != nil {
		return err
	}
	var status string
	if err = tx.QueryRow(r.Context(), `SELECT status FROM applications.applications WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return err
	}
	if status == "rejected" {
		return orderApplicationCommit(w, r, tx, id, 200)
	}
	if err = orderState(order.Status, "open"); err != nil {
		return err
	}
	if status != "submitted" {
		return problem(409, "APPLICATION_STATE_CONFLICT", "Отклонить можно только отправленный отклик")
	}
	if err = notifyApplicant(r.Context(), tx, actor.ID, order.ID, id, "application_rejected"); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `UPDATE applications.applications SET status='rejected',updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return orderApplicationCommit(w, r, tx, id, 200)
}

func (s *Server) myWorkspace(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	item, err := oneJSON(r.Context(), s.DB, `WITH
 memberships AS(SELECT company_id FROM profiles.company_members WHERE user_id=$1),
 own_orders AS(SELECT status FROM orders.orders WHERE company_id IN(SELECT company_id FROM memberships)),
 own_applications AS(SELECT status FROM applications.applications WHERE developer_id=$1),
 work AS(SELECT o.status FROM orders.orders o JOIN applications.applications a ON a.id=o.selected_application_id WHERE a.developer_id=$1)
 SELECT (SELECT count(*) FROM memberships) AS companies,
 (SELECT jsonb_build_object('total',count(*),'draft',count(*) FILTER(WHERE status='draft'),'open',count(*) FILTER(WHERE status='open'),'contractor_selected',count(*) FILTER(WHERE status='contractor_selected'),'completed',count(*) FILTER(WHERE status='completed'),'closed',count(*) FILTER(WHERE status='closed')) FROM own_orders) AS orders,
 (SELECT jsonb_build_object('total',count(*),'submitted',count(*) FILTER(WHERE status='submitted'),'accepted',count(*) FILTER(WHERE status='accepted'),'rejected',count(*) FILTER(WHERE status='rejected'),'withdrawn',count(*) FILTER(WHERE status='withdrawn')) FROM own_applications) AS applications,
 (SELECT jsonb_build_object('total',count(*),'in_progress',count(*) FILTER(WHERE status='contractor_selected'),'completed',count(*) FILTER(WHERE status='completed'),'closed',count(*) FILTER(WHERE status='closed')) FROM work) AS work,
 (SELECT count(*) FROM catalog.favorites f JOIN catalog.companies c ON c.id=f.company_id WHERE f.developer_id=$1 AND c.visibility='published') AS favorites,
 (SELECT count(*) FROM catalog.company_contacts WHERE developer_id=$1) AS contacts,
 (SELECT count(*) FROM profiles.portfolio_items WHERE developer_id=$1) AS portfolio,
 (SELECT count(*) FROM profiles.notifications WHERE user_id=$1 AND read_at IS NULL) AS unread_notifications`, actor.ID)
	if err != nil {
		return err
	}
	return respond(w, 200, item)
}

func (s *Server) myWork(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	status := r.URL.Query().Get("status")
	if status != "" && !oneOf(status, "contractor_selected", "completed", "closed") {
		return invalid("status", "Допустимы contractor_selected, completed, closed")
	}
	const where = ` EXISTS(SELECT 1 FROM applications.applications a WHERE a.id=o.selected_application_id AND a.developer_id=$1) AND ($2='' OR o.status=$2)`
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM orders.orders o WHERE `+where, actor.ID, status).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, orderSelect+` WHERE `+where+` ORDER BY o.updated_at DESC,o.id DESC LIMIT $3 OFFSET $4`, actor.ID, status, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, page)
}

func (s *Server) myCompanyApplications(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
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
	company := r.URL.Query().Get("company_id")
	if company != "" {
		if !validUUID(company) {
			return invalid("company_id", "Ожидается UUID компании")
		}
		if err = requireMember(r.Context(), s.DB, actor.ID, company, false); err != nil {
			return err
		}
	}
	const where = ` EXISTS(SELECT 1 FROM profiles.company_members m WHERE m.company_id=o.company_id AND m.user_id=$1) AND ($2='' OR a.status=$2) AND ($3::uuid IS NULL OR o.company_id=$3::uuid)`
	var companyID *string
	if company != "" {
		companyID = &company
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM applications.applications a JOIN orders.orders o ON o.id=a.order_id WHERE `+where, actor.ID, status, companyID).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, orderApplicationSelect+` WHERE `+where+` ORDER BY a.created_at DESC,a.id DESC LIMIT $4 OFFSET $5`, actor.ID, status, companyID, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, page)
}

// Contact exchange is limited to the actual engagement, including when the
// order/company is hidden publicly. Other applicants never receive account emails.
func (s *Server) orderParticipants(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "order_id")
	if err != nil {
		return err
	}
	item, err := oneJSON(r.Context(), s.DB, `SELECT o.id AS order_id,o.status,
 jsonb_build_object('id',c.id,'name',c.name,'email',c.email,'phone',c.phone) AS company,
 jsonb_build_object('user_id',customer.user_id,'name',customer.name,'email',cu.email) AS customer,
 jsonb_build_object('user_id',contractor.user_id,'name',contractor.name,'email',du.email,'github_url',contractor.github_url) AS contractor
 FROM orders.orders o JOIN catalog.companies c ON c.id=o.company_id
 JOIN applications.applications a ON a.id=o.selected_application_id
 JOIN profiles.profiles contractor ON contractor.user_id=a.developer_id JOIN auth.users du ON du.id=contractor.user_id
 JOIN profiles.profiles customer ON customer.user_id=o.created_by JOIN auth.users cu ON cu.id=customer.user_id
 WHERE o.id=$1 AND o.status IN('contractor_selected','completed') AND
 (a.developer_id=$2 OR EXISTS(SELECT 1 FROM profiles.company_members m WHERE m.company_id=o.company_id AND m.user_id=$2))`, id, actor.ID)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "private, no-store")
	return respond(w, 200, item)
}
