package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const testJWTSecret = "firstorder-integration-secret-not-production"
const testJWTIssuer = "http://test.local/auth/v1"

type testFixture struct {
	Moderator      string
	Developer      string
	OtherDeveloper string
	Owner          string
	Company        string
	HiddenCompany  string
	Category       string
}

// dbTest requires an explicit dedicated test database; it never truncates tables
// or touches IDs that were not generated for this test.
func dbTest(t *testing.T) (*Server, *pgxpool.Pool, testFixture) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset; integration tests require a dedicated migrated database")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_URL")
	}
	if !strings.Contains(strings.ToLower(cfg.ConnConfig.Database), "test") {
		t.Fatal("TEST_DATABASE_URL must name a dedicated database containing 'test'")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect dedicated database: %v", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping dedicated database: %v", err)
	}
	f := testFixture{newID(), newID(), newID(), newID(), newID(), newID(), newID()}
	users := []string{f.Moderator, f.Developer, f.OtherDeveloper, f.Owner}
	companies := []string{f.Company, f.HiddenCompany}
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanCancel()
		defer pool.Close()
		tx, cleanErr := pool.Begin(cleanCtx)
		if cleanErr != nil {
			t.Errorf("fixture cleanup begin: %v", cleanErr)
			return
		}
		defer tx.Rollback(cleanCtx)
		// Moderation CHECK requires a surviving target, so remove fixture audit rows
		// before deleting their companies/orders. Never delete unrelated audit rows.
		statements := []struct {
			sql  string
			args []any
		}{
			{`DELETE FROM catalog.moderation_actions WHERE moderator_id=ANY($1::uuid[]) OR company_id=ANY($2::uuid[]) OR order_id IN(SELECT id FROM orders.orders WHERE company_id=ANY($2::uuid[]) OR created_by=ANY($1::uuid[]))`, []any{users, companies}},
			{`UPDATE orders.orders SET selected_application_id=NULL WHERE company_id=ANY($2::uuid[]) OR created_by=ANY($1::uuid[])`, []any{users, companies}},
			{`DELETE FROM orders.orders WHERE company_id=ANY($2::uuid[]) OR created_by=ANY($1::uuid[])`, []any{users, companies}},
			{`DELETE FROM catalog.companies WHERE id=ANY($1::uuid[])`, []any{companies}},
			{`DELETE FROM catalog.categories WHERE id=$1`, []any{f.Category}},
			{`DELETE FROM auth.users WHERE id=ANY($1::uuid[])`, []any{users}},
		}
		for _, st := range statements {
			if _, cleanErr = tx.Exec(cleanCtx, st.sql, st.args...); cleanErr != nil {
				t.Errorf("fixture cleanup failed: %v", cleanErr)
				return
			}
		}
		if cleanErr = tx.Commit(cleanCtx); cleanErr != nil {
			t.Errorf("fixture cleanup commit: %v", cleanErr)
		}
	})
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for i, id := range users {
		if _, err = tx.Exec(ctx, `INSERT INTO auth.users(id,email) VALUES($1,$2)`, id, id+"@integration.invalid"); err != nil {
			t.Fatal(err)
		}
		role := "developer"
		if id == f.Moderator {
			role = "moderator"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO profiles.profiles(user_id,name,role) VALUES($1,$2,$3)`, id, fmt.Sprintf("Integration user %d", i), role); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO catalog.categories(id,name) VALUES($1,$2)`, f.Category, "Integration category "+f.Category); err != nil {
		t.Fatal(err)
	}
	for _, id := range companies {
		visibility := "published"
		if id == f.HiddenCompany {
			visibility = "hidden"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO catalog.companies(id,name,category_id,city,phone,visibility) VALUES($1,$2,$3,'Test City','old-phone',$4)`, id, "Integration company "+id, f.Category, visibility); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO profiles.company_members(company_id,user_id,member_role) VALUES($1,$2,'owner')`, id, f.Owner); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	server := New(pool, Config{JWTSecret: testJWTSecret, JWTIssuer: testJWTIssuer, SupabaseURL: "http://test.local", AllowedOrigins: []string{"http://test.local"}, RateLimit: 10000})
	return server, pool, f
}

func testToken(id string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{"sub": id, "email": id + "@integration.invalid", "role": "authenticated", "aud": "authenticated", "iss": testJWTIssuer, "exp": time.Now().Add(time.Hour).Unix()})
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(testJWTSecret))
	_, _ = mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func testRequest(t *testing.T, s *Server, method, path, actor string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if actor != "" {
		req.Header.Set("Authorization", "Bearer "+testToken(actor))
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}
func testObject(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not a JSON object (status %d): %s", rec.Code, rec.Body.String())
	}
	return body
}
func testStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("HTTP status %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
}
