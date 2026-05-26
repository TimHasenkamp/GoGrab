// Command server is the GoGrab HTTP service: a single Go binary that embeds
// the built SvelteKit frontend and exposes the JSON API.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"github.com/timhasenkamp/gograb/internal/audit"
	"github.com/timhasenkamp/gograb/internal/auth"
	"github.com/timhasenkamp/gograb/internal/config"
	"github.com/timhasenkamp/gograb/internal/db"
	"github.com/timhasenkamp/gograb/internal/handlers"
	"github.com/timhasenkamp/gograb/internal/notify"
	gogwebauthn "github.com/timhasenkamp/gograb/internal/webauthn"
	"github.com/timhasenkamp/gograb/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}
	queries := db.New(pool)

	var notifier notify.Notifier = notify.Nop{}
	if cfg.NotifyWebhookURL != "" {
		notifier = notify.NewWebhook(cfg.NotifyWebhookURL, cfg.NotifyWebhookTimeout, log)
		log.Info("notify webhook enabled")
	}

	auditLog := audit.New(queries, log)
	deps := handlers.New(queries, notifier, auditLog, log, cfg.DefaultTTL, cfg.MaxCiphertextBytes)

	waSvc, err := gogwebauthn.New(gogwebauthn.Config{
		RPDisplayName: cfg.RPDisplayName,
		RPID:          cfg.RPID,
		RPOrigins:     cfg.RPOrigins,
		SessionSecret: cfg.SessionSecret,
	})
	if err != nil {
		return fmt.Errorf("init webauthn: %w", err)
	}
	deps.WithAuth(&handlers.AuthDeps{WebAuthn: waSvc})
	log.Info("webauthn ready", "rp_id", cfg.RPID, "origins", cfg.RPOrigins)
	mux := buildRouter(cfg, deps, log)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           withRequestLog(log, mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// expiry sweeper
	go expirySweeper(ctx, queries, log)

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func buildRouter(cfg config.Config, deps *handlers.Deps, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// --- public API (rate-limited, unauthenticated) ---
	publicRL := newRateLimiter(rate.Limit(float64(cfg.RatePerMin)/60.0), cfg.RateBurst)
	publicMW := chain(publicRL.Middleware)

	mux.Handle("GET /api/requests/{token}/meta", publicMW(http.HandlerFunc(deps.PublicMeta)))
	mux.Handle("POST /api/requests/{token}/submit", publicMW(http.HandlerFunc(deps.PublicSubmit)))

	// --- admin API (forward-auth) ---
	var adminMW func(http.Handler) http.Handler
	if cfg.DevUser != "" {
		log.Warn("DEV MODE: admin endpoints use fixed dev user", "user", cfg.DevUser)
		adminMW = auth.DevMiddleware(cfg.DevUser, cfg.DevUser+"@local")
	} else {
		adminMW = auth.Middleware(cfg.TrustedProxy)
	}
	admin := chain(adminMW)

	mux.Handle("GET /api/admin/requests", admin(http.HandlerFunc(deps.AdminList)))
	mux.Handle("POST /api/admin/requests", admin(http.HandlerFunc(deps.AdminCreate)))
	mux.Handle("GET /api/admin/requests/{id}", admin(http.HandlerFunc(deps.AdminGet)))
	mux.Handle("POST /api/admin/requests/{id}/retrieve", admin(http.HandlerFunc(deps.AdminRetrieve)))
	mux.Handle("DELETE /api/admin/requests/{id}", admin(http.HandlerFunc(deps.AdminDelete)))

	// WebAuthn / unlock ceremony endpoints (forward-auth still required)
	mux.Handle("GET /api/admin/auth/status", admin(http.HandlerFunc(deps.AuthStatus)))
	mux.Handle("POST /api/admin/auth/register/begin", admin(http.HandlerFunc(deps.AuthRegisterBegin)))
	mux.Handle("POST /api/admin/auth/register/finish", admin(http.HandlerFunc(deps.AuthRegisterFinish)))
	mux.Handle("POST /api/admin/auth/login/begin", admin(http.HandlerFunc(deps.AuthLoginBegin)))
	mux.Handle("POST /api/admin/auth/login/finish", admin(http.HandlerFunc(deps.AuthLoginFinish)))
	mux.Handle("GET /api/admin/auth/credentials", admin(http.HandlerFunc(deps.AuthListCredentials)))
	mux.Handle("DELETE /api/admin/auth/credentials/{id}", admin(http.HandlerFunc(deps.AuthDeleteCredential)))

	// --- healthcheck ---
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// --- static frontend (everything else) ---
	mux.Handle("/", spaHandler(log))
	return mux
}

// spaHandler serves the embedded SvelteKit output with an index.html fallback
// for client-side routes (/admin/*, /r/*). API and /healthz are handled by
// more-specific patterns above.
func spaHandler(log *slog.Logger) http.Handler {
	sub, err := fs.Sub(web.FS, "build")
	if err != nil {
		log.Error("embed sub", "err", err)
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend not embedded", http.StatusInternalServerError)
		})
	}

	fileServer := http.FileServer(http.FS(sub))
	indexBytes, indexErr := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean == "" {
			clean = "index.html"
		}
		if f, err := sub.Open(clean); err == nil {
			info, statErr := f.Stat()
			_ = f.Close()
			if statErr == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// fallback to index.html for SPA routes
		if indexErr != nil {
			http.Error(w, "frontend not built", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(indexBytes)
	})
}

// --- middleware -----------------------------------------------------------

func chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			h = mws[i](h)
		}
		return h
	}
}

func withRequestLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(sr, r)
		log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sr.status,
			"dur_ms", time.Since(start).Milliseconds(),
			"ip", clientIP(r),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if comma := strings.Index(v, ","); comma >= 0 {
			return strings.TrimSpace(v[:comma])
		}
		return v
	}
	return r.RemoteAddr
}

// --- rate limiter ---------------------------------------------------------

type rateLimiter struct {
	limit rate.Limit
	burst int
	mu    sync.Mutex
	bkts  map[string]*rate.Limiter
}

func newRateLimiter(r rate.Limit, b int) *rateLimiter {
	return &rateLimiter{limit: r, burst: b, bkts: make(map[string]*rate.Limiter)}
}

func (rl *rateLimiter) get(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if l, ok := rl.bkts[key]; ok {
		return l
	}
	l := rate.NewLimiter(rl.limit, rl.burst)
	rl.bkts[key] = l
	return l
}

func (rl *rateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.get(clientIP(r)).Allow() {
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":"rate_limited","message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- background jobs ------------------------------------------------------

func expirySweeper(ctx context.Context, q db.Querier, log *slog.Logger) {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			n, err := q.ExpirePendingRequests(ctx)
			if err != nil {
				log.Warn("expire sweep failed", "err", err)
				continue
			}
			if n > 0 {
				log.Info("expired pending requests", "count", n)
			}
		}
	}
}

// --- logging --------------------------------------------------------------

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
