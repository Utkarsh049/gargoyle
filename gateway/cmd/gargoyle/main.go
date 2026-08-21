// Command gargoyle is the API gateway process. It resolves API keys,
// enforces rate limits and abuse rules, exposes Prometheus metrics,
// logs blocked decisions, and reverse-proxies allowed traffic.
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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"gargoyle/internal/abuse"
	"gargoyle/internal/abuse/rules"
	"gargoyle/internal/admin"
	"gargoyle/internal/client"
	"gargoyle/internal/config"
	"gargoyle/internal/db"
	"gargoyle/internal/httpserver"
	"gargoyle/internal/logstore"
	"gargoyle/internal/metrics"
	"gargoyle/internal/proxy"
	"gargoyle/internal/ratelimit"
	"gargoyle/web"
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
	logStore := logstore.NewPostgresStore(pool)
	promMetrics := metrics.New(prometheus.DefaultRegisterer)
	rp := proxy.New(logger)

	// Configure abuse detection rules
	sweepTracker := rules.NewRedisSweepTracker(rdb)
	timingTracker := rules.NewRedisTimingTracker(rdb)
	abuseEngine := abuse.NewEngine(
		cfg.AbuseBlockThreshold,
		rules.NewHeaderAnomalyRule(),
		rules.NewEndpointSweepRule(sweepTracker, cfg.AbuseSweepThreshold, cfg.AbuseSweepWindow),
		rules.NewRequestSequencingRule(timingTracker),
	)

	// ML Abuse Scorer (looks for abuse_model.onnx)
	mlActive := false
	if mlScorer, err := abuse.NewMLScorer(cfg.ONNXModelPath, cfg.MLScoreThreshold); err != nil {
		logger.InfoContext(ctx, "gargoyle: ML scoring disabled, running rules-only", "reason", err.Error())
	} else {
		mlActive = true
		abuseEngine.AddRule(mlScorer)
		logger.InfoContext(ctx, "gargoyle: ML scoring enabled", "model_path", cfg.ONNXModelPath, "threshold", cfg.MLScoreThreshold)
	}

	adminStore := admin.NewPostgresStore(pool, cfg.ONNXModelPath, mlActive)
	adminRouter := admin.NewRouter(adminStore, logger)
	webHandler, err := web.NewHandler(adminStore, logger)
	if err != nil {
		return fmt.Errorf("web: initializing dashboard: %w", err)
	}

	if clientCount, err := registry.CountClients(ctx); err != nil {
		logger.WarnContext(ctx, "metrics: failed to query initial active client count", "error", err)
	} else {
		promMetrics.ActiveClients.Set(float64(clientCount))
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(metrics.Middleware(promMetrics))
	r.Use(requestLogger(logger))

	r.Get("/healthz", handleHealthz)
	r.Handle("/metrics", promhttp.Handler())

	// Admin JSON REST API (protected when GARGOYLE_ADMIN_KEY is configured)
	r.Group(func(r chi.Router) {
		r.Use(admin.AuthMiddleware(cfg.AdminKey, logger))
		r.Mount("/api/admin", adminRouter)
	})

	// Embedded Web Dashboard UI
	webHandler.MountRoutes(r)

	// Authenticated and protected ingress routes
	r.Group(func(r chi.Router) {
		r.Use(ratelimit.PreAuthMiddleware(limiter, cfg.PreAuthRateLimit, logger))
		r.Use(client.Middleware(registry, logger))
		r.Use(ratelimit.Middleware(limiter, logStore, logger))
		r.Use(abuse.Middleware(abuseEngine, logStore, promMetrics, logger))
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

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

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
