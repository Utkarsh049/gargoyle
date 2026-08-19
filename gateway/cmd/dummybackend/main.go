// Command dummybackend is a throwaway HTTP server used to prove Gargoyle's
// reverse proxy forwards requests untouched. It is a development/demo
// utility only — it is not one of the four responsibilities of the Go
//
// It echoes back everything it received (method, path, headers, body) as
// JSON, which makes it easy to visually confirm that what arrives here
// matches what was sent to Gargoyle.
package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type echoResponse struct {
	Backend   string            `json:"backend"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Query     string            `json:"query"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := ":9000"
	if v, ok := os.LookupEnv("DUMMY_BACKEND_ADDR"); ok && v != "" {
		addr = v
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleEcho(logger))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("dummybackend: listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("dummybackend: server failed", "error", err)
		os.Exit(1)
	}
}

func handleEcho(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		headers := make(map[string]string, len(r.Header))
		for k := range r.Header {
			headers[k] = r.Header.Get(k)
		}

		resp := echoResponse{
			Backend:   "dummybackend",
			Method:    r.Method,
			Path:      r.URL.Path,
			Query:     r.URL.RawQuery,
			Headers:   headers,
			Body:      string(body),
			Timestamp: time.Now().UTC(),
		}

		logger.Info("dummybackend: received request", "method", r.Method, "path", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
