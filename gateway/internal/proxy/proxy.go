// Package proxy builds the reverse proxy that forwards allowed requests to
// a client's backend. Since Phase 2, there is no single fixed target: the
// destination is resolved per request from the Client attached to the
// request context by client.Middleware (see PROJECT.md §2, §7).
package proxy

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"

	"gargoyle/internal/client"
)

// New builds a reverse proxy whose upstream target is resolved per request
// from client.FromContext.
//
// The returned proxy:
//   - forwards to the resolved client's TargetURL, preserving path and
//     query string
//   - sets the Host header to the target's host, so name-based virtual
//     hosting on the upstream works as expected
//   - logs and returns 502 Bad Gateway on upstream failures, instead of
//     letting the default ReverseProxy behavior leak a bare connection
//     error to the client
func New(logger *slog.Logger) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			c, ok := client.FromContext(pr.In.Context())
			if !ok {
				// Unreachable in normal operation: client.Middleware runs
				// on every route this proxy is mounted behind, and it
				// rejects requests with 401 before they ever get here. If
				// this fires, it's a wiring bug, not a client error — log
				// loudly and leave the outgoing request untouched so it
				// fails fast (empty scheme/host) rather than silently
				// going somewhere unintended.
				logger.ErrorContext(pr.In.Context(), "proxy: no client resolved in request context")
				return
			}

			pr.SetURL(c.TargetURL)
			pr.SetXForwarded()
			pr.Out.Host = c.TargetURL.Host
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				// The client disconnected before the upstream responded;
				// there's no meaningful response to write, so just log it
				// quietly.
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
