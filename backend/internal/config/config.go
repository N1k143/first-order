package config

import (
	"fmt"
	"ingeneriya/backend/internal/api"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func Load() (api.Config, error) {
	cfg := api.Config{Addr: env("API_ADDR", "127.0.0.1:18080"), DatabaseURL: os.Getenv("DATABASE_URL"), SupabaseURL: strings.TrimRight(os.Getenv("SUPABASE_URL"), "/"), SupabaseAnonKey: os.Getenv("SUPABASE_ANON_KEY"), JWTSecret: os.Getenv("JWT_SECRET"), JWTIssuer: os.Getenv("JWT_ISSUER"), CookieSecure: env("COOKIE_SECURE", "true") == "true", ImporterToken: os.Getenv("IMPORTER_TOKEN"), ReadyToken: os.Getenv("READY_TOKEN"), ImportCSVPath: env("IMPORT_CSV_PATH", "testdata/companies.csv"), AuthRedirectURL: env("AUTH_REDIRECT_URL", "http://localhost:3000/auth/callback"), RateLimit: 120}
	for _, o := range strings.Split(env("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), ",") {
		o = strings.TrimSpace(o)
		u, e := url.Parse(o)
		if e != nil || u.Host == "" || !oneScheme(u.Scheme) || u.Path != "" {
			return cfg, fmt.Errorf("ALLOWED_ORIGINS must contain exact http(s) origins")
		}
		cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
	}
	if v := os.Getenv("RATE_LIMIT_PER_MINUTE"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 10000 {
			return cfg, fmt.Errorf("invalid RATE_LIMIT_PER_MINUTE")
		}
		cfg.RateLimit = n
	}
	if cfg.SupabaseURL != "" {
		u, e := url.Parse(cfg.SupabaseURL)
		if e != nil || u.Host == "" || !oneScheme(u.Scheme) || u.User != nil || u.RawQuery != "" {
			return cfg, fmt.Errorf("invalid SUPABASE_URL")
		}
	}
	if cfg.JWTSecret != "" && cfg.JWTIssuer == "" && cfg.SupabaseURL == "" {
		return cfg, fmt.Errorf("JWT_SECRET requires JWT_ISSUER or SUPABASE_URL")
	}
	if cfg.ImporterToken != "" && len(cfg.ImporterToken) < 32 {
		return cfg, fmt.Errorf("IMPORTER_TOKEN must contain at least 32 characters")
	}
	return cfg, nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func oneScheme(s string) bool { return s == "http" || s == "https" }
