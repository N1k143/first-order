package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type settingsTransport func(*http.Request) (*http.Response, error)

func (f settingsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func settingsResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func settingsServer() *Server {
	return New(nil, Config{JWTSecret: testJWTSecret, JWTIssuer: testJWTIssuer, SupabaseURL: "https://auth.example.test", SupabaseAnonKey: "public-test-key", AuthRedirectURL: "https://app.example.test/auth/callback", AllowedOrigins: []string{"https://app.example.test"}, CookieSecure: true})
}

const settingsUser = "11111111-1111-4111-8111-111111111111"

func TestEmailChangeAuthenticationAndFields(t *testing.T) {
	valid := testToken(settingsUser)
	forgedRole := signedClaims(testJWTSecret, map[string]any{"sub": settingsUser, "aud": "authenticated", "role": "service_role", "iss": testJWTIssuer, "exp": time.Now().Add(time.Hour).Unix()})
	for _, tc := range []struct {
		name, token, body, origin string
		cookie                    bool
		status                    int
	}{
		{name: "signed out", body: `{"email":"next@example.test"}`, status: 401},
		{name: "invalid token", token: "invalid", body: `{"email":"next@example.test"}`, status: 401},
		{name: "service token", token: forgedRole, body: `{"email":"next@example.test"}`, status: 401},
		{name: "invalid email", token: valid, body: `{"email":"wrong"}`, status: 422},
		{name: "platform role injection", token: valid, body: `{"email":"next@example.test","role":"moderator"}`, status: 400},
		{name: "target ID injection", token: valid, body: `{"email":"next@example.test","user_id":"another"}`, status: 400},
		{name: "redirect injection", token: valid, body: `{"email":"next@example.test","redirect_to":"https://evil.test"}`, status: 400},
		{name: "cookie missing Origin", token: valid, body: `{"email":"next@example.test"}`, cookie: true, status: 403},
		{name: "cookie foreign Origin", token: valid, body: `{"email":"next@example.test"}`, cookie: true, origin: "https://evil.test", status: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := settingsServer()
			s.httpClient.Transport = settingsTransport(func(*http.Request) (*http.Response, error) { t.Fatal("rejected request reached Auth"); return nil, nil })
			req := httptest.NewRequest("POST", "/auth/change-email", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.token != "" {
				if tc.cookie {
					req.AddCookie(&http.Cookie{Name: accessCookie, Value: tc.token})
				} else {
					req.Header.Set("Authorization", "Bearer "+tc.token)
				}
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			testStatus(t, rec, tc.status)
		})
	}
}
func TestEmailChangeDelegatesCurrentSessionAndReportsPending(t *testing.T) {
	for _, tc := range []struct {
		name, path, method string
		cookie             bool
	}{
		{"cookie session", "/auth/change-email", "POST", true},
		{"canonical bearer", "/api/v1/auth/change-email", "PATCH", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := settingsServer()
			token := testToken(settingsUser)
			calls := 0
			s.httpClient.Transport = settingsTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != "PUT" || req.URL.Path != "/auth/v1/user" || req.URL.Query().Get("redirect_to") != s.Config.AuthRedirectURL {
					t.Fatal("wrong upstream operation or callback")
				}
				if req.Header.Get("Authorization") != "Bearer "+token || req.Header.Get("apikey") != "public-test-key" {
					t.Fatal("current session or public API key missing")
				}
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || len(payload) != 1 || payload["email"] != "next@example.test" {
					t.Fatal("untrusted fields forwarded to Auth")
				}
				return settingsResponse(200, `{"id":"`+settingsUser+`","email":"original@example.test","new_email":"next@example.test","access_token":"must-not-leak"}`), nil
			})
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"email":"next@example.test"}`))
			req.Header.Set("Content-Type", "application/json")
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: accessCookie, Value: token})
				req.Header.Set("Origin", "https://app.example.test")
			} else {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			testStatus(t, rec, 202)
			if calls != 1 || testObject(t, rec)["status"] != "confirmation_required" || strings.Contains(rec.Body.String(), "must-not-leak") || len(rec.Result().Cookies()) != 0 {
				t.Fatal("pending request reported completion, leaked provider data or replaced session")
			}
		})
	}
}
func TestEmailChangeDoesNotInventSuccess(t *testing.T) {
	for _, tc := range []struct {
		name     string
		upstream int
		response string
		status   int
		state    string
	}{
		{"already confirmed", 200, `{"id":"` + settingsUser + `","email":"next@example.test"}`, 200, "updated"},
		{"upstream rejection", 422, `{"error":"private-provider-detail"}`, 422, ""},
		{"upstream unavailable", 500, `{"error":"private-provider-detail"}`, 503, ""},
		{"rate limited", 429, `{}`, 429, ""},
		{"unrelated identity", 200, `{"id":"22222222-2222-4222-8222-222222222222","new_email":"next@example.test"}`, 503, ""},
		{"unexpected response", 200, `{}`, 503, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := settingsServer()
			s.httpClient.Transport = settingsTransport(func(*http.Request) (*http.Response, error) { return settingsResponse(tc.upstream, tc.response), nil })
			req := httptest.NewRequest("POST", "/auth/change-email", strings.NewReader(`{"email":"next@example.test"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+testToken(settingsUser))
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			testStatus(t, rec, tc.status)
			if tc.state != "" && testObject(t, rec)["status"] != tc.state {
				t.Fatal("incorrect completion status")
			}
			if strings.Contains(rec.Body.String(), "private-provider-detail") {
				t.Fatal("upstream error exposed")
			}
		})
	}
}
func TestSecureEmailChangeFirstConfirmationKeepsSessionPending(t *testing.T) {
	s := settingsServer()
	s.httpClient.Transport = settingsTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method != "POST" || req.URL.Path != "/auth/v1/verify" {
			t.Fatal("wrong verification operation")
		}
		return settingsResponse(200, `{"msg":"Confirmation link accepted. Please proceed to confirm link sent to the other email","code":"200"}`), nil
	})
	req := httptest.NewRequest("POST", "/auth/verify", strings.NewReader(`{"type":"email_change","token_hash":"test-one-time-hash"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	testStatus(t, rec, 202)
	if testObject(t, rec)["status"] != "second_confirmation_required" || len(rec.Result().Cookies()) != 0 {
		t.Fatal("first confirmation must not establish a new session")
	}
}
func TestPasswordSettingsUseExistingSessionWithoutRecovery(t *testing.T) {
	s := settingsServer()
	token := testToken(settingsUser)
	s.httpClient.Transport = settingsTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method != "PUT" || req.URL.Path != "/auth/v1/user" || req.Header.Get("Authorization") != "Bearer "+token {
			t.Fatal("password update did not use current session")
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || len(payload) != 1 || payload["password"] != "new-test-password" {
			t.Fatal("unexpected password update fields")
		}
		return settingsResponse(200, `{"id":"`+settingsUser+`"}`), nil
	})
	req := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader(`{"password":"new-test-password","confirm":"new-test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://app.example.test")
	req.AddCookie(&http.Cookie{Name: accessCookie, Value: token})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	testStatus(t, rec, 200)
	if testObject(t, rec)["status"] != "updated" {
		t.Fatal("password not updated")
	}
}
