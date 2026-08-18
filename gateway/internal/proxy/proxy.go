// Package proxy builds the reverse proxy that forwards allowed requests to
// a client's backend. In Phase 1 there is exactly one, hardcoded target;
// later phases swap the static target for a per-request lookup against the
// client registry, without changing how the proxy itself is built.
package proxy

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// New builds a reverse proxy that forwards every request to target.
//
// The returned proxy:
//   - preserves the original request path and query string
//   - sets the Host header to the target's host, so name-based virtual
//     hosting on the upstream works as expected
//   - logs and returns 502 Bad Gateway on upstream failures, instead of
//     letting the default ReverseProxy behavior leak a bare connection
//     error to the client
func New(target *url.URL, logger *slog.Logger) *httputil.ReverseProxy {
	rp := httputil.NewSingleHostReverseProxy(target)

	director := rp.Director
	rp.Director = func(r *http.Request) {
		director(r)
		r.Host = target.Host
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if errors.Is(err, context.Canceled) {
			// The client disconnected before the upstream responded; there's
			// no meaningful response to write, so just log it quietly.
			logger.WarnContext(r.Context(), "proxy: client canceled request",
				"path", r.URL.Path,
			)
			return
		}

		logger.ErrorContext(r.Context(), "proxy: upstream request failed",
			"path", r.URL.Path,
			"target", target.String(),
			"error", err,
		)
		w.WriteHeader(http.StatusBadGateway)
	}

	return rp
}
