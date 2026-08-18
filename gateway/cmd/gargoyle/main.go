// Command gargoyle is the Gargoyle gateway process. As of Phase 3, it
// resolves each request's API key to a registered client (Postgres,
// cached in memory), enforces per-client rate limits (Redis sliding window),
// and forwards allowed requests to the client's own target_url.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"gargoyle/internal/client"
	"gargoyle/internal/config"
	"gargoyle/internal/db"
	"gargoyle/internal/httpserver"
	"gargoyle/internal/proxy"
	"gargoyle/internal/ratelimit"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(context.Background(), logger); err != nil {
		logger.Error("gargoyle: fatal error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("redis: parsing url: %w", err)
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("redis: connecting: %w", err)
	}

	registry := client.NewRegistry(client.NewPostgresStore(pool), cfg.ClientCacheTTL)
	limiter := ratelimit.NewRedisLimiter(rdb, cfg.RateLimitWindow)
	rp := proxy.New(logger)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Get("/healthz", handleHealthz)

	// Authenticated & rate-limited routes
	r.Group(func(r chi.Router) {
		r.Use(client.Middleware(registry, logger))
		r.Use(ratelimit.Middleware(limiter, logger))
		r.Handle("/*", rp)
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	logger.Info("gargoyle: starting", "listen_addr", cfg.ListenAddr)

	return httpserver.Run(ctx, srv, cfg.ShutdownTimeout, logger)
}

// handleHealthz is a liveness endpoint that never touches Postgres or any
// client's upstream, so it stays healthy independently of both. It's
// deliberately excluded from client resolution and proxying.
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// requestLogger is a small structured-logging middleware built on slog,
// used instead of Chi's default text logger so that request logs are
// machine-parseable from day one (they'll sit alongside Prometheus metrics
// and Postgres request logs added in later phases).
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			attrs := []any{
				"request_id", middleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"remote_addr", r.RemoteAddr,
			}
			if c, ok := client.FromContext(r.Context()); ok {
				attrs = append(attrs, "client_id", c.ID, "client_name", c.Name)
			}

			logger.InfoContext(r.Context(), "request", attrs...)
		})
	}
}
