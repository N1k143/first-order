package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func signedClaims(secret string, claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, _ := json.Marshal(claims)
	data := header + "." + base64.RawURLEncoding.EncodeToString(body)
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(data))
	return data + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func TestJWTVerificationRejectsUntrustedClaims(t *testing.T) {
	const secret = "auth-test-secret-only"
	cfg := Config{JWTSecret: secret, JWTIssuer: "https://auth.test/auth/v1"}
	s := New(nil, cfg)
	valid := func() map[string]any {
		return map[string]any{"sub": "11111111-1111-4111-8111-111111111111", "email": "dev@example.test", "role": "authenticated", "aud": "authenticated", "iss": cfg.JWTIssuer, "exp": time.Now().Add(time.Hour).Unix()}
	}
	for _, test := range []struct {
		name   string
		change func(map[string]any)
		sign   string
		ok     bool
	}{
		{"valid", func(map[string]any) {}, secret, true}, {"forged signature", func(map[string]any) {}, "attacker", false}, {"expired", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }, secret, false}, {"missing expiration", func(c map[string]any) { delete(c, "exp") }, secret, false}, {"wrong issuer", func(c map[string]any) { c["iss"] = "https://evil.test" }, secret, false}, {"wrong audience", func(c map[string]any) { c["aud"] = "service_role" }, secret, false}, {"service token", func(c map[string]any) { c["role"] = "service_role" }, secret, false}, {"invalid subject", func(c map[string]any) { c["sub"] = "not-a-uuid" }, secret, false}, {"future token", func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() }, secret, false}, {"array audience", func(c map[string]any) { c["aud"] = []string{"authenticated"} }, secret, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims := valid()
			test.change(claims)
			who, e := s.verifyToken(context.Background(), signedClaims(test.sign, claims))
			if (e == nil) != test.ok {
				t.Fatalf("ok=%v err=%v", test.ok, e)
			}
			if test.ok && who.ID != claims["sub"] {
				t.Fatal("subject changed")
			}
		})
	}
	if _, e := s.verifyToken(context.Background(), "eyJhbGciOiJub25lIn0.e30."); e == nil {
		t.Fatal("accepted unsigned token")
	}
}
func TestTransportSecurityAndErrors(t *testing.T) {
	s := New(nil, Config{AllowedOrigins: []string{"https://app.test"}, RateLimit: 2})
	h := s.Handler()
	cases := []struct {
		method, path, origin, body string
		cookie                     bool
		want                       int
	}{
		{"GET", "/health", "", "", false, 200}, {"GET", "/ready", "", "", false, 503}, {"GET", "/api/v1/me", "", "", false, 401}, {"POST", "/auth/logout", "https://evil.test", "", false, 403}, {"POST", "/auth/logout", "", "", true, 403}, {"POST", "/auth/login", "https://app.test", `{"email":"x@y.test","password":"x","admin":true}`, false, 400},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if c.cookie {
			r.AddCookie(&http.Cookie{Name: accessCookie, Value: "test"})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Fatalf("%s: %d %s", c.path, w.Code, w.Body)
		}
		if !validUUID(w.Header().Get("X-Request-ID")) {
			t.Fatal("missing request id")
		}
		if w.Code >= 400 {
			var e map[string]any
			if json.Unmarshal(w.Body.Bytes(), &e) != nil || e["request_id"] == nil {
				t.Fatal("error envelope")
			}
		}
	}
}
func TestFrontendDTOCompatibility(t *testing.T) {
	got := frontendValue(map[string]any{"id": "x", "name": "Arman Coffee", "category_name": "Кафе", "website_status": "not_found", "website_checked_at": "2026-11-19T10:00:00Z"}).(map[string]any)
	if got["checkedAt"] != "2026-11-19" || got["websiteStatus"] != "not_found" || got["category"] != "Кафе" || got["initials"] != "AC" {
		t.Fatalf("%#v", got)
	}
	a := frontendValue(map[string]any{"developer_id": "server-user", "order_id": "order", "status": "accepted", "proposed_price": float64(100), "message": "message"}).(map[string]any)
	if a["profileId"] != "server-user" || a["status"] != "selected" || a["letter"] != "message" {
		t.Fatalf("%#v", a)
	}
}
func TestAuthIntegrationCookiesAndDatabaseRole(t *testing.T) {
	s, pool, f := dbTest(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "test-public-key" {
			t.Error("missing provider key")
		}
		if r.URL.Path != "/auth/v1/token" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(authTokens{AccessToken: testToken(f.Developer), RefreshToken: "test-refresh-only", ExpiresIn: 3600})
	}))
	defer provider.Close()
	s.Config.SupabaseURL = provider.URL
	s.Config.SupabaseAnonKey = "test-public-key"
	s.Config.CookieSecure = true
	w := testRequest(t, s, "POST", "/auth/login", "", map[string]any{"email": "dev@example.test", "password": "test-password"})
	testStatus(t, w, 200)
	if strings.Contains(w.Body.String(), "access_token") || strings.Contains(w.Body.String(), "refresh_token") {
		t.Fatal("tokens leaked into browser JSON")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookies %v", cookies)
	}
	for _, c := range cookies {
		if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
			t.Fatal("unsafe cookie")
		}
	}
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.AddCookie(cookies[0])
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	testStatus(t, rec, 200)
	if testObject(t, rec)["user_id"] != f.Developer {
		t.Fatal("wrong cookie actor")
	}
	// Roles are read from PostgreSQL, not elevated by editable JWT metadata.
	var role string
	if err := pool.QueryRow(context.Background(), "SELECT role FROM profiles.profiles WHERE user_id=$1", f.Developer).Scan(&role); err != nil || role != "developer" {
		t.Fatalf("role=%s error=%v", role, err)
	}
	bad := testRequest(t, s, "POST", "/auth/register", "", map[string]any{"name": "Test", "email": "new@example.test", "password": "test-password", "role": "moderator"})
	testStatus(t, bad, 422)
}
