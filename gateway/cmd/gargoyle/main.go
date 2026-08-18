// Command gargoyle is the Gargoyle gateway process. Phase 1 wires up the
// bare skeleton: a Chi router that forwards every request to a single
// hardcoded upstream via net/http/httputil.ReverseProxy.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"gargoyle/internal/config"
	"gargoyle/internal/httpserver"
	"gargoyle/internal/proxy"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("gargoyle: fatal error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	rp := proxy.New(cfg.TargetURL, logger)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Get("/healthz", handleHealthz)
	r.Handle("/*", rp)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	logger.Info("gargoyle: starting",
		"listen_addr", cfg.ListenAddr,
		"target_url", cfg.TargetURL.String(),
	)

	return httpserver.Run(context.Background(), srv, cfg.ShutdownTimeout, logger)
}

// handleHealthz is a liveness endpoint that never touches the upstream, so
// it stays healthy independently of whether the backend is reachable. It's
// deliberately excluded from proxying.
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

			logger.InfoContext(r.Context(), "request",
				"request_id", middleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}
