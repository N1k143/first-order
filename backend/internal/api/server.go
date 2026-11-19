package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type Config struct {
	Addr            string
	DatabaseURL     string
	SupabaseURL     string
	SupabaseAnonKey string
	JWTSecret       string
	JWTIssuer       string
	AllowedOrigins  []string
	CookieSecure    bool
	ImporterToken   string
	ReadyToken      string
	ImportCSVPath   string
	AuthRedirectURL string
	RateLimit       int
}
type Server struct {
	DB         DB
	Config     Config
	logger     *slog.Logger
	httpClient *http.Client
	limiter    *rateLimiter
}

func New(db DB, cfg Config) *Server {
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 120
	}
	return &Server{DB: db, Config: cfg, logger: slog.Default(), httpClient: &http.Client{Timeout: 10 * time.Second}, limiter: &rateLimiter{entries: map[string]rateEntry{}, max: cfg.RateLimit}}
}
func (s *Server) Handler() http.Handler {
	canonical := chi.NewRouter()
	canonical.Use(s.authenticate)
	s.registerAuth(canonical)
	s.registerProfiles(canonical)
	s.registerCatalog(canonical)
	s.registerOrders(canonical)
	s.registerImports(canonical)
	canonical.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, problem(404, "NOT_FOUND", "Маршрут не найден"))
	})
	canonical.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, problem(405, "METHOD_NOT_ALLOWED", "Метод не поддерживается"))
	})
	root := chi.NewRouter()
	root.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("OK"))
	})
	root.Get("/ready", handle(s.ready))
	root.Mount("/api/v1", canonical)
	s.registerAuth(root)
	root.Handle("/*", s.compatibility(canonical))
	return s.middleware(root)
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) error {
	if s.Config.ReadyToken != "" && !secureToken(r.Header.Get("Authorization"), s.Config.ReadyToken) {
		return problem(403, "FORBIDDEN", "Мониторинг требует отдельный токен")
	}
	if s.DB == nil {
		return dbUnavailable()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.DB.Ping(ctx); err != nil {
		return problem(503, "DATABASE_UNAVAILABLE", "База временно недоступна")
	}
	return respond(w, 200, map[string]string{"status": "ready", "database": "up"})
}

type contextKey string

const (
	requestIDKey   contextKey = "request_id"
	requestInfoKey contextKey = "request_info"
	actorKey       contextKey = "actor"
)

type requestLogInfo struct{ userID, errorCode string }

func requestInfo(r *http.Request) *requestLogInfo {
	v, _ := r.Context().Value(requestInfoKey).(*requestLogInfo)
	return v
}

func requestID(r *http.Request) string { v, _ := r.Context().Value(requestIDKey).(string); return v }
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

type responseStatus struct {
	http.ResponseWriter
	status int
}

func (w *responseStatus) WriteHeader(n int) {
	if w.status == 0 {
		w.status = n
		w.ResponseWriter.WriteHeader(n)
	}
}
func (w *responseStatus) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := newID()
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
		info := &requestLogInfo{}
		r = r.WithContext(context.WithValue(r.Context(), requestInfoKey, info))
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		rec := &responseStatus{ResponseWriter: w}
		defer func() {
			if v := recover(); v != nil {
				s.logger.Error("request panic", "request_id", id, "stack", string(debug.Stack()))
				if rec.status == 0 {
					writeError(rec, r, problem(500, "INTERNAL_ERROR", "Не удалось выполнить запрос"))
				}
			}
			s.logger.Info("http", "request_id", id, "user_id", info.userID, "method", r.Method, "path", r.URL.Path, "status", rec.status, "error_code", info.errorCode, "duration_ms", time.Since(start).Milliseconds())
		}()
		origin := r.Header.Get("Origin")
		allowed := false
		for _, o := range s.Config.AllowedOrigins {
			if origin == o {
				allowed = true
				break
			}
		}
		if origin != "" && !allowed {
			writeError(rec, r, problem(403, "ORIGIN_DENIED", "Источник запроса не разрешён"))
			return
		}
		if origin != "" {
			rec.Header().Set("Access-Control-Allow-Origin", origin)
			rec.Header().Set("Access-Control-Allow-Credentials", "true")
			rec.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			rec.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			rec.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			rec.Header().Set("Access-Control-Max-Age", "600")
			rec.WriteHeader(204)
			return
		}
		unsafe := !oneOf(r.Method, "GET", "HEAD", "OPTIONS")
		if unsafe {
			if _, err := r.Cookie(accessCookie); err == nil && r.Header.Get("Authorization") == "" && origin == "" {
				writeError(rec, r, problem(403, "CSRF_ORIGIN_REQUIRED", "Для запроса с сессией обязателен Origin"))
				return
			}
			if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") && r.ContentLength > 0 {
				writeError(rec, r, problem(415, "UNSUPPORTED_MEDIA_TYPE", "Ожидается application/json"))
				return
			}
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			if host == "" {
				host = r.RemoteAddr
			}
			if !s.limiter.allow(host, time.Now()) {
				rec.Header().Set("Retry-After", "60")
				writeError(rec, r, problem(429, "RATE_LIMITED", "Слишком много запросов. Попробуйте через минуту"))
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		next.ServeHTTP(rec, r.WithContext(ctx))
	})
}

type rateEntry struct {
	count int
	start time.Time
}
type rateLimiter struct {
	sync.Mutex
	entries map[string]rateEntry
	max     int
}

func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.Lock()
	defer l.Unlock()
	e := l.entries[key]
	if now.Sub(e.start) >= time.Minute {
		e = rateEntry{start: now}
	}
	e.count++
	l.entries[key] = e
	if len(l.entries) > 10000 {
		for k, v := range l.entries {
			if now.Sub(v.start) >= time.Minute {
				delete(l.entries, k)
			}
		}
	}
	return e.count <= l.max
}
