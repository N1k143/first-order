package config

import (
	"fmt"
	"github.com/jackc/pgx/v5"
	"ingeneriya/backend/internal/api"
	"net/netip"
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
		if e != nil || u.Host == "" || !oneScheme(u.Scheme) || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return cfg, fmt.Errorf("ALLOWED_ORIGINS must contain exact http(s) origins")
		}
		cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
	}
	cfg.Environment = env("APP_ENV", "development")
	if cfg.Environment != "development" && cfg.Environment != "production" {
		return cfg, fmt.Errorf("APP_ENV must be development or production")
	}
	for _, value := range strings.Split(os.Getenv("TRUSTED_PROXY_CIDRS"), ",") {
		if strings.TrimSpace(value) == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil || prefix.Bits() == 0 {
			return cfg, fmt.Errorf("TRUSTED_PROXY_CIDRS must contain explicit CIDRs, never a default route")
		}
		cfg.TrustedProxies = append(cfg.TrustedProxies, prefix.Masked())
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
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if _, err := pgx.ParseConfig(cfg.DatabaseURL); err != nil {
		return cfg, fmt.Errorf("invalid DATABASE_URL")
	}
	if cfg.Environment == "production" {
		if !cfg.CookieSecure || len(cfg.ReadyToken) < 32 || cfg.SupabaseAnonKey == "" {
			return cfg, fmt.Errorf("production requires secure cookies, READY_TOKEN of at least 32 characters, SUPABASE_ANON_KEY")
		}
		for _, origin := range cfg.AllowedOrigins {
			if !strings.HasPrefix(origin, "https://") {
				return cfg, fmt.Errorf("production origins must use HTTPS")
			}
		}
		u, e := url.Parse(cfg.AuthRedirectURL)
		if e != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return cfg, fmt.Errorf("production AUTH_REDIRECT_URL must be an HTTPS callback")
		}
		allowed := false
		for _, origin := range cfg.AllowedOrigins {
			if origin == u.Scheme+"://"+u.Host {
				allowed = true
			}
		}
		if !allowed || u.Path != "/auth/callback" {
			return cfg, fmt.Errorf("production callback must be /auth/callback on an allowed origin")
		}
		authURL, _ := url.Parse(cfg.SupabaseURL)
		localAuth := false
		if authURL != nil && authURL.Scheme == "http" {
			ip, e := netip.ParseAddr(authURL.Hostname())
			localAuth = e == nil && ip.IsLoopback()
		}
		if !strings.HasPrefix(cfg.SupabaseURL, "https://") && !localAuth {
			return cfg, fmt.Errorf("production SUPABASE_URL requires HTTPS or a literal loopback HTTP address")
		}
		if cfg.JWTSecret != "" && len(cfg.JWTSecret) < 32 {
			return cfg, fmt.Errorf("production JWT_SECRET is too short")
		}
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
