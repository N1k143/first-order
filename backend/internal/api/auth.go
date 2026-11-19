package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const accessCookie = "firstorder_access"
const refreshCookie = "firstorder_refresh"

type Actor struct{ ID, Role, Email, Name string }
type identity struct {
	ID       string         `json:"id"`
	Email    string         `json:"email"`
	Metadata map[string]any `json:"user_metadata"`
}

func (s *Server) actor(r *http.Request) *Actor {
	a, _ := r.Context().Value(actorKey).(*Actor)
	return a
}
func (s *Server) requireActor(r *http.Request) (*Actor, error) {
	a := s.actor(r)
	if a == nil {
		return nil, problem(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	if s.DB == nil {
		return nil, dbUnavailable()
	}
	if !validUUID(a.ID) {
		return nil, problem(403, "USER_REQUIRED", "Требуется пользовательская сессия")
	}
	return a, nil
}
func (s *Server) requireRole(r *http.Request, roles ...string) (*Actor, error) {
	a := s.actor(r)
	if a == nil {
		return nil, problem(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	if s.DB == nil {
		return nil, dbUnavailable()
	}
	if !oneOf(a.Role, roles...) {
		return nil, problem(403, "FORBIDDEN", "Недостаточно прав")
	}
	return a, nil
}
func secureToken(header, want string) bool {
	got := strings.TrimPrefix(header, "Bearer ")
	return want != "" && got != header && len(got) == len(want) && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Auth operations verify their own credentials; an expired session must not prevent login/refresh.
		if strings.Contains(r.URL.Path, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		if s.Config.ImporterToken != "" && secureToken(header, s.Config.ImporterToken) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey, &Actor{Role: "system_importer"})))
			return
		}
		token := ""
		if header != "" {
			if !strings.HasPrefix(header, "Bearer ") {
				writeError(w, r, problem(401, "UNAUTHORIZED", "Ожидается Bearer JWT"))
				return
			}
			token = strings.TrimPrefix(header, "Bearer ")
		} else if c, e := r.Cookie(accessCookie); e == nil {
			token = c.Value
		}
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		who, err := s.verifyToken(r.Context(), token)
		if err != nil {
			writeError(w, r, err)
			return
		}
		actor, err := s.ensureActor(r.Context(), who)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if info := requestInfo(r); info != nil {
			info.userID = actor.ID
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey, actor)))
	})
}
func (s *Server) verifyToken(ctx context.Context, token string) (identity, error) {
	deny := problem(401, "INVALID_TOKEN", "Сессия недействительна или истекла")
	if len(token) > 16384 {
		return identity{}, deny
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return identity{}, deny
	}
	var header struct {
		Alg string `json:"alg"`
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil || json.Unmarshal(raw, &header) != nil || !oneOf(header.Alg, "HS256", "RS256", "ES256") {
		return identity{}, deny
	}
	if header.Alg == "HS256" && s.Config.JWTSecret != "" {
		sig, e := base64.RawURLEncoding.DecodeString(parts[2])
		if e != nil {
			return identity{}, deny
		}
		h := hmac.New(sha256.New, []byte(s.Config.JWTSecret))
		_, _ = h.Write([]byte(parts[0] + "." + parts[1]))
		if !hmac.Equal(sig, h.Sum(nil)) {
			return identity{}, deny
		}
		raw, e = base64.RawURLEncoding.DecodeString(parts[1])
		if e != nil {
			return identity{}, deny
		}
		var claims struct {
			Sub      string          `json:"sub"`
			Email    string          `json:"email"`
			Exp      int64           `json:"exp"`
			Nbf      int64           `json:"nbf"`
			Iss      string          `json:"iss"`
			Aud      json.RawMessage `json:"aud"`
			Role     string          `json:"role"`
			Metadata map[string]any  `json:"user_metadata"`
		}
		if json.Unmarshal(raw, &claims) != nil {
			return identity{}, deny
		}
		issuer := s.Config.JWTIssuer
		if issuer == "" {
			issuer = strings.TrimRight(s.Config.SupabaseURL, "/") + "/auth/v1"
		}
		var aud string
		var audiences []string
		_ = json.Unmarshal(claims.Aud, &aud)
		_ = json.Unmarshal(claims.Aud, &audiences)
		if !validUUID(claims.Sub) || claims.Exp <= time.Now().Unix() || claims.Nbf > time.Now().Unix()+10 || claims.Iss != issuer || claims.Role != "authenticated" || (aud != "authenticated" && !oneOf("authenticated", audiences...)) {
			return identity{}, deny
		}
		return identity{claims.Sub, claims.Email, claims.Metadata}, nil
	}
	// Supabase verifies signatures (including rotating asymmetric signing keys) and session status.
	data, status, err := s.authCall(ctx, http.MethodGet, "/user", nil, token)
	if err != nil {
		return identity{}, err
	}
	if status != 200 {
		return identity{}, deny
	}
	var who identity
	if json.Unmarshal(data, &who) != nil || !validUUID(who.ID) {
		return identity{}, deny
	}
	return who, nil
}
func (s *Server) ensureActor(ctx context.Context, who identity) (*Actor, error) {
	if s.DB == nil {
		return nil, dbUnavailable()
	}
	name := strings.TrimSpace(stringValue(who.Metadata, "name"))
	if name == "" {
		name = strings.Split(who.Email, "@")[0]
	}
	if name == "" {
		name = "Пользователь"
	}
	if len(name) > 200 {
		name = "Пользователь"
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, "INSERT INTO profiles.profiles(user_id,name) VALUES($1,$2) ON CONFLICT(user_id) DO NOTHING", who.ID, name)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() > 0 && stringValue(who.Metadata, "registration_role") == "company" {
		company := strings.TrimSpace(stringValue(who.Metadata, "company"))
		city := strings.TrimSpace(stringValue(who.Metadata, "city"))
		if city == "" {
			city = "Астана"
		}
		// Registration may create a NEW hidden company. It never grants control of an existing catalog company.
		if company != "" && len(company) <= 300 && len(city) <= 100 {
			var exists bool
			err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM catalog.companies WHERE lower(name)=lower($1))", company).Scan(&exists)
			if err != nil {
				return nil, err
			}
			if !exists {
				var id string
				err = tx.QueryRow(ctx, "INSERT INTO catalog.companies(name,city,visibility) VALUES($1,$2,'hidden') RETURNING id::text", company, city).Scan(&id)
				if err != nil {
					return nil, err
				}
				_, err = tx.Exec(ctx, "INSERT INTO profiles.company_members(company_id,user_id,member_role) VALUES($1,$2,'owner')", id, who.ID)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	a := &Actor{ID: who.ID, Email: who.Email}
	err = tx.QueryRow(ctx, "SELECT role,name FROM profiles.profiles WHERE user_id=$1", who.ID).Scan(&a.Role, &a.Name)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}
func (s *Server) authCall(ctx context.Context, method, path string, body any, bearer string) ([]byte, int, error) {
	if s.Config.SupabaseURL == "" || s.Config.SupabaseAnonKey == "" {
		return nil, 0, problem(503, "AUTH_NOT_CONFIGURED", "Supabase Auth не настроен")
	}
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.Config.SupabaseURL, "/")+"/auth/v1"+path, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("apikey", s.Config.SupabaseAnonKey)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := *s.httpClient
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, problem(503, "AUTH_UNAVAILABLE", "Сервис авторизации временно недоступен")
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, 0, problem(503, "AUTH_UNAVAILABLE", "Сервис авторизации временно недоступен")
	}
	return data, res.StatusCode, nil
}
func (s *Server) registerAuth(r chi.Router) {
	r.Post("/auth/register", handle(s.authRegister))
	r.Post("/auth/login", handle(s.authLogin))
	r.Post("/auth/logout", handle(s.authLogout))
	r.Post("/auth/refresh", handle(s.authRefresh))
	r.Post("/auth/session", handle(s.authSession))
	r.Post("/auth/forgot-password", handle(s.authForgot))
	r.Post("/auth/reset-password", handle(s.authReset))
	r.Post("/auth/change-email", handle(s.authChangeEmail))
	r.Patch("/auth/change-email", handle(s.authChangeEmail))
	r.Post("/auth/resend-verification", handle(s.authResend))
	r.Post("/auth/verify", handle(s.authVerify))
}

type authForm struct {
	Email    string  `json:"email"`
	Password string  `json:"password"`
	Confirm  string  `json:"confirm"`
	Name     string  `json:"name"`
	Role     string  `json:"role"`
	Company  string  `json:"company"`
	City     string  `json:"city"`
	Token    *string `json:"token"`
}

func emailValid(v string) bool {
	a, e := mail.ParseAddress(v)
	return e == nil && a.Address == v && len(v) <= 254
}
func (s *Server) authRegister(w http.ResponseWriter, r *http.Request) error {
	var in authForm
	if e := readJSON(r, &in); e != nil {
		return e
	}
	if !emailValid(in.Email) {
		return invalid("email", "Укажите email")
	}
	if len(in.Password) < 8 || len(in.Password) > 256 {
		return invalid("password", "Пароль должен содержать от 8 до 256 символов")
	}
	if in.Confirm != "" && in.Confirm != in.Password {
		return invalid("confirm", "Пароли не совпадают")
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 {
		return invalid("name", "Укажите имя до 200 символов")
	}
	if in.Role == "" {
		in.Role = "developer"
	}
	if !oneOf(in.Role, "developer", "company") {
		return invalid("role", "Допустимы developer или company")
	}
	if in.Role == "company" {
		if strings.TrimSpace(in.Company) == "" || len(in.Company) > 300 || len(in.City) > 100 {
			return invalid("company", "Укажите название и город компании")
		}
		if s.DB == nil {
			return dbUnavailable()
		}
		var exists bool
		if err := s.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM catalog.companies WHERE lower(name)=lower($1))", strings.TrimSpace(in.Company)).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return problem(409, "COMPANY_CLAIM_REQUIRES_REVIEW", "Для доступа к существующей компании обратитесь к модератору")
		}
	}
	body := map[string]any{"email": in.Email, "password": in.Password, "data": map[string]string{"name": strings.TrimSpace(in.Name), "registration_role": in.Role, "company": in.Company, "city": in.City}}
	path := "/signup"
	if s.Config.AuthRedirectURL != "" {
		path += "?redirect_to=" + url.QueryEscape(s.Config.AuthRedirectURL)
	}
	data, status, err := s.authCall(r.Context(), "POST", path, body, "")
	if err != nil {
		return err
	}
	if status >= 400 {
		return authUpstreamError(status, false)
	}
	var session authTokens
	_ = json.Unmarshal(data, &session)
	if session.AccessToken != "" {
		if _, err = s.acceptSession(w, r, session); err != nil {
			return err
		}
	}
	return respond(w, 201, map[string]any{"status": "verification_required", "email": in.Email})
}

type authTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func authUpstreamError(status int, login bool) error {
	if status == 429 {
		return problem(429, "RATE_LIMITED", "Слишком много запросов")
	}
	if status >= 500 {
		return problem(503, "AUTH_UNAVAILABLE", "Сервис авторизации недоступен")
	}
	if login {
		return problem(401, "INVALID_CREDENTIALS", "Неверный email, пароль или email не подтверждён")
	}
	return problem(422, "AUTH_VALIDATION_ERROR", "Не удалось выполнить операцию авторизации. Проверьте данные")
}
func (s *Server) acceptSession(w http.ResponseWriter, r *http.Request, t authTokens) (*Actor, error) {
	who, e := s.verifyToken(r.Context(), t.AccessToken)
	if e != nil {
		return nil, e
	}
	actor, e := s.ensureActor(r.Context(), who)
	if e != nil {
		return nil, e
	}
	age := t.ExpiresIn
	if age <= 0 || age > 86400 {
		age = 3600
	}
	http.SetCookie(w, &http.Cookie{Name: accessCookie, Value: t.AccessToken, Path: "/", HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: age})
	if t.RefreshToken != "" {
		http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: t.RefreshToken, Path: "/", HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 86400})
	}
	return actor, nil
}
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) error {
	var in authForm
	if e := readJSON(r, &in); e != nil {
		return e
	}
	if !emailValid(in.Email) || in.Password == "" || len(in.Password) > 256 {
		return invalid("credentials", "Укажите email и пароль")
	}
	data, status, e := s.authCall(r.Context(), "POST", "/token?grant_type=password", map[string]string{"email": in.Email, "password": in.Password}, "")
	if e != nil {
		return e
	}
	if status != 200 {
		return authUpstreamError(status, true)
	}
	var t authTokens
	if e = json.Unmarshal(data, &t); e != nil {
		return problem(503, "AUTH_UNAVAILABLE", "Некорректный ответ Auth")
	}
	a, e := s.acceptSession(w, r, t)
	if e != nil {
		return e
	}
	return respond(w, 200, map[string]any{"user_id": a.ID, "name": a.Name, "role": a.Role, "email": a.Email})
}
func (s *Server) authRefresh(w http.ResponseWriter, r *http.Request) error {
	c, e := r.Cookie(refreshCookie)
	if e != nil {
		return problem(401, "UNAUTHORIZED", "Нет сессии для обновления")
	}
	data, status, e := s.authCall(r.Context(), "POST", "/token?grant_type=refresh_token", map[string]string{"refresh_token": c.Value}, "")
	if e != nil {
		return e
	}
	if status != 200 {
		return authUpstreamError(status, true)
	}
	var t authTokens
	if json.Unmarshal(data, &t) != nil {
		return problem(503, "AUTH_UNAVAILABLE", "Некорректный ответ Auth")
	}
	if _, e = s.acceptSession(w, r, t); e != nil {
		return e
	}
	return respond(w, 200, map[string]string{"status": "refreshed"})
}
func (s *Server) authSession(w http.ResponseWriter, r *http.Request) error {
	var t authTokens
	if e := readJSON(r, &t); e != nil {
		return e
	}
	a, e := s.acceptSession(w, r, t)
	if e != nil {
		return e
	}
	return respond(w, 200, map[string]any{"user_id": a.ID, "role": a.Role})
}
func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) error {
	if c, e := r.Cookie(accessCookie); e == nil {
		_, _, _ = s.authCall(r.Context(), "POST", "/logout", nil, c.Value)
	}
	for _, n := range []string{accessCookie, refreshCookie} {
		http.SetCookie(w, &http.Cookie{Name: n, Path: "/", Value: "", MaxAge: -1, HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode})
	}
	return respond(w, 204, nil)
}
func (s *Server) authForgot(w http.ResponseWriter, r *http.Request) error {
	var in authForm
	if e := readJSON(r, &in); e != nil {
		return e
	}
	if !emailValid(in.Email) {
		return invalid("email", "Укажите email")
	}
	path := "/recover"
	if s.Config.AuthRedirectURL != "" {
		path += "?redirect_to=" + url.QueryEscape(s.Config.AuthRedirectURL)
	}
	_, status, e := s.authCall(r.Context(), "POST", path, map[string]string{"email": in.Email}, "")
	if e != nil {
		return e
	}
	if status == 429 || status >= 500 {
		return authUpstreamError(status, false)
	}
	return respond(w, 202, map[string]string{"status": "accepted"})
}
func (s *Server) authReset(w http.ResponseWriter, r *http.Request) error {
	var in authForm
	if e := readJSON(r, &in); e != nil {
		return e
	}
	if len(in.Password) < 8 || len(in.Password) > 256 || in.Confirm != "" && in.Confirm != in.Password {
		return invalid("password", "Проверьте новый пароль и подтверждение")
	}
	token := ""
	if c, e := r.Cookie(accessCookie); e == nil {
		token = c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimPrefix(h, "Bearer ")
	}
	if _, e := s.verifyToken(r.Context(), token); e != nil {
		return e
	}
	_, status, e := s.authCall(r.Context(), "PUT", "/user", map[string]string{"password": in.Password}, token)
	if e != nil {
		return e
	}
	if status >= 400 {
		return authUpstreamError(status, false)
	}
	return respond(w, 200, map[string]string{"status": "updated"})
}

// authChangeEmail delegates ownership and confirmation to the current Auth
// session. It never updates auth.users directly or accepts a target user ID.
func (s *Server) authChangeEmail(w http.ResponseWriter, r *http.Request) error {
	token := ""
	if c, err := r.Cookie(accessCookie); err == nil {
		token = c.Value
	}
	if h := r.Header.Get("Authorization"); h != "" {
		if !strings.HasPrefix(h, "Bearer ") {
			return problem(401, "INVALID_TOKEN", "Войдите в аккаунт")
		}
		token = strings.TrimPrefix(h, "Bearer ")
	}
	who, err := s.verifyToken(r.Context(), token)
	if err != nil {
		return err
	}
	if info := requestInfo(r); info != nil {
		info.userID = who.ID
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	in.Email = strings.TrimSpace(in.Email)
	if !emailValid(in.Email) {
		return invalid("email", "Укажите корректный email")
	}
	path := "/user"
	if s.Config.AuthRedirectURL != "" {
		path += "?redirect_to=" + url.QueryEscape(s.Config.AuthRedirectURL)
	}
	data, status, err := s.authCall(r.Context(), "PUT", path, map[string]string{"email": in.Email}, token)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return authUpstreamError(status, false)
	}
	var user struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		NewEmail string `json:"new_email"`
	}
	if json.Unmarshal(data, &user) != nil || user.ID != who.ID {
		return problem(503, "AUTH_UNAVAILABLE", "Некорректный ответ Auth")
	}
	if strings.EqualFold(user.NewEmail, in.Email) {
		return respond(w, http.StatusAccepted, map[string]string{"status": "confirmation_required", "email": in.Email})
	}
	if strings.EqualFold(user.Email, in.Email) {
		return respond(w, http.StatusOK, map[string]string{"status": "updated", "email": user.Email})
	}
	return problem(503, "AUTH_UNAVAILABLE", "Auth не подтвердил запрос смены email")
}

func (s *Server) authResend(w http.ResponseWriter, r *http.Request) error {
	var in authForm
	if e := readJSON(r, &in); e != nil {
		return e
	}
	if !emailValid(in.Email) {
		return invalid("email", "Укажите email для повторной отправки")
	}
	_, status, e := s.authCall(r.Context(), "POST", "/resend?redirect_to="+url.QueryEscape(s.Config.AuthRedirectURL), map[string]string{"type": "signup", "email": in.Email}, "")
	if e != nil {
		return e
	}
	if status == 429 || status >= 500 {
		return authUpstreamError(status, false)
	}
	return respond(w, 202, map[string]string{"status": "accepted"})
}
func (s *Server) authVerify(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		TokenHash string `json:"token_hash"`
		Type      string `json:"type"`
	}
	if e := readJSON(r, &in); e != nil {
		return e
	}
	if in.TokenHash == "" || !oneOf(in.Type, "signup", "email", "recovery", "invite", "magiclink", "email_change") {
		return invalid("token_hash", "Некорректная ссылка подтверждения")
	}
	data, status, e := s.authCall(r.Context(), "POST", "/verify", in, "")
	if e != nil {
		return e
	}
	if status != 200 {
		return authUpstreamError(status, false)
	}
	var t authTokens
	if json.Unmarshal(data, &t) != nil {
		return problem(503, "AUTH_UNAVAILABLE", "Некорректный ответ Auth")
	}
	// Secure email change returns a successful non-session response after
	// the first confirmation. Never create cookies or claim completion yet.
	if in.Type == "email_change" && t.AccessToken == "" {
		var pending struct {
			Message string `json:"msg"`
			Code    string `json:"code"`
		}
		if json.Unmarshal(data, &pending) == nil && pending.Code == "200" && pending.Message == "Confirmation link accepted. Please proceed to confirm link sent to the other email" {
			return respond(w, http.StatusAccepted, map[string]string{"status": "second_confirmation_required"})
		}
		return problem(503, "AUTH_UNAVAILABLE", "Некорректный ответ Auth")
	}
	if _, e = s.acceptSession(w, r, t); e != nil {
		return e
	}
	return respond(w, 200, map[string]string{"status": "verified"})
}
