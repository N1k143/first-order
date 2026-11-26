package config

import (
	"strings"
	"testing"
)

func production(t *testing.T) {
	t.Helper()
	for _, key := range []string{"JWT_SECRET", "JWT_ISSUER", "RATE_LIMIT_PER_MINUTE", "TRUSTED_PROXY_CIDRS"} {
		t.Setenv(key, "")
	}
	for key, value := range map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgres://test:test@127.0.0.1/test", "SUPABASE_URL": "https://auth.example.test", "SUPABASE_ANON_KEY": "test-public-key", "READY_TOKEN": strings.Repeat("x", 32), "IMPORTER_TOKEN": "", "COOKIE_SECURE": "true", "ALLOWED_ORIGINS": "https://example.test", "AUTH_REDIRECT_URL": "https://example.test/auth/callback"} {
		t.Setenv(key, value)
	}
}
func TestProductionConfiguration(t *testing.T) {
	production(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32,::1/128")
	cfg, err := Load()
	if err != nil || len(cfg.TrustedProxies) != 2 {
		t.Fatalf("valid configuration rejected: %v", err)
	}
}
func TestUnsafeProductionConfigurationRejected(t *testing.T) {
	for _, test := range []struct{ key, value string }{
		{"DATABASE_URL", ""}, {"DATABASE_URL", "postgres://user:%ZZ@invalid"}, {"COOKIE_SECURE", "false"}, {"READY_TOKEN", "short"}, {"SUPABASE_ANON_KEY", ""},
		{"ALLOWED_ORIGINS", "http://localhost:3000"}, {"ALLOWED_ORIGINS", "https://user:password@example.test"},
		{"ALLOWED_ORIGINS", "https://example.test?unsafe=true"}, {"ALLOWED_ORIGINS", "https://example.test#fragment"},
		{"AUTH_REDIRECT_URL", "https://other.test/auth/callback"}, {"AUTH_REDIRECT_URL", "https://example.test/wrong"},
		{"SUPABASE_URL", "http://auth.example.test"}, {"TRUSTED_PROXY_CIDRS", "0.0.0.0/0"}, {"TRUSTED_PROXY_CIDRS", "::/0"}, {"TRUSTED_PROXY_CIDRS", "invalid"},
		{"JWT_SECRET", "short"}, {"APP_ENV", "prodction"},
	} {
		t.Run(test.key+"_"+test.value, func(t *testing.T) {
			production(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestProductionAllowsOnlyLiteralLoopbackHTTPAuth(t *testing.T) {
	for _, address := range []string{"http://127.0.0.1:18000", "http://[::1]:18000"} {
		production(t)
		t.Setenv("SUPABASE_URL", address)
		if _, err := Load(); err != nil {
			t.Fatalf("local gateway rejected: %v", err)
		}
	}
	for _, address := range []string{"http://localhost:18000", "http://10.0.0.1:18000", "http://auth.example.test"} {
		production(t)
		t.Setenv("SUPABASE_URL", address)
		if _, err := Load(); err == nil {
			t.Fatal("non-loopback plaintext Auth allowed")
		}
	}
}
