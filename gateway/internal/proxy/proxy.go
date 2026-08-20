// Package proxy provides reverse proxying to client upstream backends.
package proxy

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"

	"gargoyle/internal/client"
	"gargoyle/internal/metrics"
)

// New constructs a reverse proxy that dynamically forwards requests
// to the target URL of the authenticated client in the request context.
func New(logger *slog.Logger) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			c, ok := client.FromContext(pr.In.Context())
			if !ok {
				logger.ErrorContext(pr.In.Context(), "proxy: no client in request context")
				return
			}

			metrics.SetDecision(pr.In.Context(), metrics.OutcomeAllowed)
			pr.SetURL(c.TargetURL)
			pr.SetXForwarded()
			pr.Out.Host = c.TargetURL.Host
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			metrics.SetDecision(r.Context(), metrics.OutcomeError)
			if errors.Is(err, context.Canceled) {
				logger.WarnContext(r.Context(), "proxy: client canceled request", "path", r.URL.Path)
				return
			}

			logAttrs := []any{"path", r.URL.Path, "error", err}
			if c, ok := client.FromContext(r.Context()); ok {
				logAttrs = append(logAttrs, "client_id", c.ID, "target", c.TargetURL.String())
			}
			logger.ErrorContext(r.Context(), "proxy: upstream request failed", logAttrs...)

			w.WriteHeader(http.StatusBadGateway)
		},
	}
}
