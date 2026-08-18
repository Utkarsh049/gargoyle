// Package httpserver provides a small helper for running an *http.Server
// with graceful shutdown on SIGINT/SIGTERM. It exists so that "how Gargoyle
// starts and stops" is written once and reused by every binary in cmd/,
// rather than each main.go reimplementing signal handling.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// Run starts srv, blocks until the process receives SIGINT or SIGTERM (or
// the given context is canceled), then attempts a graceful shutdown bounded
// by shutdownTimeout. It returns a non-nil error only for failures that
// aren't the expected "server closed" outcome of a graceful shutdown.
func Run(ctx context.Context, srv *http.Server, shutdownTimeout time.Duration, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http server: listening", "addr", srv.Addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: listen and serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("http server: shutdown signal received, draining connections",
			"timeout", shutdownTimeout,
		)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http server: graceful shutdown failed: %w", err)
	}

	logger.Info("http server: shutdown complete")
	return nil
}
