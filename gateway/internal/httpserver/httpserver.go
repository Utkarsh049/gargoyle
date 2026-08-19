// Package httpserver provides lifecycle management for HTTP servers with graceful shutdown.
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

// Run starts the server and gracefully shuts down on SIGINT/SIGTERM or context cancellation.
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
