package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

func (s *Server) registerProfiles(r chi.Router) {
	r.MethodFunc("GET", "/me", handle(s.getMe))
	r.MethodFunc("PATCH", "/me/profile", handle(s.patchMyProfile))
	r.MethodFunc("GET", "/profiles/{user_id}", handle(s.getPublicProfile))
	r.MethodFunc("GET", "/me/portfolio", handle(s.listMyPortfolio))
	r.MethodFunc("POST", "/me/portfolio", handle(s.createPortfolioItem))
	r.MethodFunc("PATCH", "/me/portfolio/{item_id}", handle(s.patchPortfolioItem))
	r.MethodFunc("DELETE", "/me/portfolio/{item_id}", handle(s.deletePortfolioItem))
	r.MethodFunc("GET", "/companies/{company_id}/members", handle(s.listCompanyMembers))
	r.MethodFunc("POST", "/companies/{company_id}/members", handle(s.addCompanyMember))
	r.MethodFunc("DELETE", "/companies/{company_id}/members/{user_id}", handle(s.deleteCompanyMember))
}

const publicProfileColumns = `user_id, name, avatar_url, bio, skills, github_url, role, created_at, updated_at`

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	profile, err := oneJSON(r.Context(), s.DB, `SELECT `+publicProfileColumns+` FROM profiles.profiles WHERE user_id=$1`, actor.ID)
	if err != nil {
		return err
	}
	companies, err := listJSON(r.Context(), s.DB, `SELECT m.company_id,m.member_role,c.name FROM profiles.company_members m JOIN catalog.companies c ON c.id=m.company_id WHERE m.user_id=$1 ORDER BY c.name,m.company_id LIMIT 100`, actor.ID)
	if err != nil {
		return err
	}
	profile["companies"] = companies
	profile["email"] = actor.Email
	return respond(w, http.StatusOK, profile)
}

type profilePatch struct {
	present   map[string]bool
	Name      *string `json:"name"`
	Bio       *string `json:"bio"`
	Skills    *string `json:"skills"`
	GithubURL *string `json:"github_url"`
	AvatarURL *string `json:"avatar_url"`
}

// profileStringObject preserves explicit null separately from omission while
// retaining strict rejection of unknown and read-only fields.
func profileStringObject(data []byte, allowed ...string) (map[string]*string, map[string]bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	if raw == nil {
		return nil, nil, fmt.Errorf("expected JSON object")
	}
	values := map[string]*string{}
	present := map[string]bool{}
	for key, value := range raw {
		if !oneOf(key, allowed...) {
			return nil, nil, fmt.Errorf("unknown field %s", key)
		}
		var text *string
		if err := json.Unmarshal(value, &text); err != nil {
			return nil, nil, err
		}
		values[key], present[key] = text, true
	}
	return values, present, nil
}
func profileHas(present map[string]bool, key string, value *string) bool {
	return present[key] || value != nil
}
func (p *profilePatch) UnmarshalJSON(data []byte) error {
	values, present, err := profileStringObject(data, "name", "bio", "skills", "github_url", "avatar_url")
	if err != nil {
		return err
	}
	p.Name, p.Bio, p.Skills, p.GithubURL = values["name"], values["bio"], values["skills"], values["github_url"]
	p.AvatarURL = values["avatar_url"]
	p.present = present
	return nil
}

func profileText(field string, value *string, max int, required bool) error {
	if value == nil {
		return nil
	}
	*value = strings.TrimSpace(*value)
	if required && *value == "" {
		return invalid(field, "Поле обязательно")
	}
	if utf8.RuneCountInString(*value) > max {
		return invalid(field, "Значение слишком длинное")
	}
	return nil
}

func (p *profilePatch) validate() error {
	if len(p.present) == 0 && p.Name == nil && p.Bio == nil && p.Skills == nil && p.GithubURL == nil && p.AvatarURL == nil {
		return invalid("body", "Укажите хотя бы одно изменяемое поле")
	}
	if profileHas(p.present, "name", p.Name) && p.Name == nil {
		return invalid("name", "Имя не может быть null")
	}
	if err := profileText("name", p.Name, 200, true); err != nil {
		return err
	}
	if err := profileText("bio", p.Bio, 5000, false); err != nil {
		return err
	}
	if err := profileText("skills", p.Skills, 2000, false); err != nil {
		return err
	}
	if err := profileText("github_url", p.GithubURL, 2048, false); err != nil {
		return err
	}
	if p.GithubURL != nil {
		if err := validateURL("github_url", *p.GithubURL); err != nil {
			return err
		}
	}
	return validateAvatar(p.AvatarURL)
}

func (s *Server) patchMyProfile(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	var body profilePatch
	if err = readJSON(r, &body); err != nil {
		return err
	}
	if err = body.validate(); err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var id string
	if err = tx.QueryRow(r.Context(), `SELECT user_id::text FROM profiles.profiles WHERE user_id=$1 FOR UPDATE`, actor.ID).Scan(&id); err != nil {
		return err
	}
	item, err := changeJSON(r.Context(), tx, `UPDATE profiles.profiles SET name=CASE WHEN $6::boolean THEN $2::text ELSE name END,bio=CASE WHEN $7::boolean THEN $3::text ELSE bio END,skills=CASE WHEN $8::boolean THEN $4::text ELSE skills END,github_url=CASE WHEN $9::boolean THEN $5::text ELSE github_url END,avatar_url=CASE WHEN $11::boolean THEN $10::text ELSE avatar_url END,updated_at=now() WHERE user_id=$1 RETURNING `+publicProfileColumns, actor.ID, body.Name, body.Bio, body.Skills, body.GithubURL, profileHas(body.present, "name", body.Name), profileHas(body.present, "bio", body.Bio), profileHas(body.present, "skills", body.Skills), profileHas(body.present, "github_url", body.GithubURL), body.AvatarURL, profileHas(body.present, "avatar_url", body.AvatarURL))
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}

func (s *Server) getPublicProfile(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireActor(r); err != nil {
		return err
	}
	id, err := pathID(r, "user_id")
	if err != nil {
		return err
	}
	profile, err := oneJSON(r.Context(), s.DB, `SELECT `+publicProfileColumns+` FROM profiles.profiles WHERE user_id=$1`, id)
	if err != nil {
		return err
	}
	portfolio, err := listJSON(r.Context(), s.DB, `SELECT * FROM profiles.portfolio_items WHERE developer_id=$1 ORDER BY created_at DESC,id LIMIT 100`, id)
	if err != nil {
		return err
	}
	profile["portfolio"] = portfolio
	return respond(w, http.StatusOK, profile)
}

func (s *Server) listMyPortfolio(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM profiles.portfolio_items WHERE developer_id=$1`, actor.ID).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, `SELECT * FROM profiles.portfolio_items WHERE developer_id=$1 ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, actor.ID, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, page)
}

type portfolioInput struct {
	present     map[string]bool
	Title       *string `json:"title"`
	Description *string `json:"description"`
	ProjectURL  *string `json:"project_url"`
}

func (p *portfolioInput) UnmarshalJSON(data []byte) error {
	values, present, err := profileStringObject(data, "title", "description", "project_url")
	if err != nil {
		return err
	}
	p.Title, p.Description, p.ProjectURL = values["title"], values["description"], values["project_url"]
	p.present = present
	return nil
}

func (p *portfolioInput) validate(create bool) error {
	if create && p.Title == nil {
		return invalid("title", "Поле обязательно")
	}
	if !create && len(p.present) == 0 && p.Title == nil && p.Description == nil && p.ProjectURL == nil {
		return invalid("body", "Укажите хотя бы одно изменяемое поле")
	}
	if profileHas(p.present, "title", p.Title) && p.Title == nil {
		return invalid("title", "Название не может быть null")
	}
	if err := profileText("title", p.Title, 300, true); err != nil {
		return err
	}
	if err := profileText("description", p.Description, 10000, false); err != nil {
		return err
	}
	if err := profileText("project_url", p.ProjectURL, 2048, false); err != nil {
		return err
	}
	if p.ProjectURL != nil {
		return validateURL("project_url", *p.ProjectURL)
	}
	return nil
}
func (s *Server) createPortfolioItem(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireRole(r, "developer")
	if err != nil {
		return err
	}
	var body portfolioInput
	if err = readJSON(r, &body); err != nil {
		return err
	}
	if err = body.validate(true); err != nil {
		return err
	}
	item, err := changeJSON(r.Context(), s.DB, `INSERT INTO profiles.portfolio_items(developer_id,title,description,project_url) VALUES($1,$2,$3,$4) RETURNING *`, actor.ID, body.Title, body.Description, body.ProjectURL)
	if err != nil {
		return err
	}
	return respond(w, http.StatusCreated, item)
}
func portfolioOwner(ctx context.Context, q Querier, itemID, actorID string) error {
	var owner string
	if err := q.QueryRow(ctx, `SELECT developer_id::text FROM profiles.portfolio_items WHERE id=$1 FOR UPDATE`, itemID).Scan(&owner); err != nil {
		return err
	}
	if owner != actorID {
		return problem(http.StatusForbidden, "FORBIDDEN", "Можно изменять только свои работы")
	}
	return nil
}
func (s *Server) patchPortfolioItem(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "item_id")
	if err != nil {
		return err
	}
	var body portfolioInput
	if err = readJSON(r, &body); err != nil {
		return err
	}
	if err = body.validate(false); err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if err = portfolioOwner(r.Context(), tx, id, actor.ID); err != nil {
		return err
	}
	item, err := changeJSON(r.Context(), tx, `UPDATE profiles.portfolio_items SET title=CASE WHEN $5::boolean THEN $2::text ELSE title END,description=CASE WHEN $6::boolean THEN $3::text ELSE description END,project_url=CASE WHEN $7::boolean THEN $4::text ELSE project_url END,updated_at=now() WHERE id=$1 RETURNING *`, id, body.Title, body.Description, body.ProjectURL, profileHas(body.present, "title", body.Title), profileHas(body.present, "description", body.Description), profileHas(body.present, "project_url", body.ProjectURL))
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}
func (s *Server) deletePortfolioItem(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "item_id")
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if err = portfolioOwner(r.Context(), tx, id, actor.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM profiles.portfolio_items WHERE id=$1`, id); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, http.StatusNoContent, nil)
}

func (s *Server) listCompanyMembers(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	company, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	if actor.Role != "moderator" {
		if err = requireMember(r.Context(), s.DB, actor.ID, company, false); err != nil {
			return err
		}
	} else {
		var exists bool
		if err = s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM catalog.companies WHERE id=$1)`, company).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return problem(http.StatusNotFound, "NOT_FOUND", "Компания не найдена")
		}
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM profiles.company_members WHERE company_id=$1`, company).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, `SELECT m.company_id,m.user_id,m.member_role,m.created_at,p.name FROM profiles.company_members m JOIN profiles.profiles p ON p.user_id=m.user_id WHERE m.company_id=$1 ORDER BY m.created_at,m.user_id LIMIT $2 OFFSET $3`, company, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, page)
}

type memberInput struct {
	UserID     string `json:"user_id"`
	MemberRole string `json:"member_role"`
}

func (m *memberInput) validate() error {
	if !validUUID(m.UserID) {
		return invalid("user_id", "Ожидается UUID пользователя")
	}
	if m.MemberRole == "" {
		m.MemberRole = "representative"
	}
	if !oneOf(m.MemberRole, "owner", "representative") {
		return invalid("member_role", "Допустимы owner и representative")
	}
	return nil
}
func lockCompanyForMembers(ctx context.Context, q Querier, company string) error {
	var id string
	return q.QueryRow(ctx, `SELECT id::text FROM catalog.companies WHERE id=$1 FOR UPDATE`, company).Scan(&id)
}
func (s *Server) addCompanyMember(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	company, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	var body memberInput
	if err = readJSON(r, &body); err != nil {
		return err
	}
	if err = body.validate(); err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if err = lockCompanyForMembers(r.Context(), tx, company); err != nil {
		return err
	}
	if err = requireMember(r.Context(), tx, actor.ID, company, true); err != nil {
		return err
	}
	// The target user is an object identifier. The acting owner always comes from JWT.
	var target string
	if err = tx.QueryRow(r.Context(), `SELECT user_id::text FROM profiles.profiles WHERE user_id=$1`, body.UserID).Scan(&target); err != nil {
		return err
	}
	item, err := changeJSON(r.Context(), tx, `INSERT INTO profiles.company_members(company_id,user_id,member_role) VALUES($1,$2,$3) RETURNING *`, company, body.UserID, body.MemberRole)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, http.StatusCreated, item)
}
func (s *Server) deleteCompanyMember(w http.ResponseWriter, r *http.Request) error {
	actor, err := s.requireActor(r)
	if err != nil {
		return err
	}
	company, err := pathID(r, "company_id")
	if err != nil {
		return err
	}
	user, err := pathID(r, "user_id")
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if err = lockCompanyForMembers(r.Context(), tx, company); err != nil {
		return err
	}
	if err = requireMember(r.Context(), tx, actor.ID, company, true); err != nil {
		return err
	}
	var role string
	if err = tx.QueryRow(r.Context(), `SELECT member_role FROM profiles.company_members WHERE company_id=$1 AND user_id=$2 FOR UPDATE`, company, user).Scan(&role); err != nil {
		return err
	}
	if role == "owner" {
		var owners int64
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM profiles.company_members WHERE company_id=$1 AND member_role='owner'`, company).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return problem(http.StatusConflict, "LAST_COMPANY_OWNER", "Нельзя удалить последнего владельца компании")
		}
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM profiles.company_members WHERE company_id=$1 AND user_id=$2`, company, user); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, http.StatusNoContent, nil)
}
