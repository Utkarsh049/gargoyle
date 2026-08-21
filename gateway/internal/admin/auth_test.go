package admin

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adminKey := "secret-admin-token-123"

	handler := AuthMiddleware(adminKey, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	tests := []struct {
		name         string
		headerKey    string
		headerVal    string
		queryKey     string
		expectedCode int
	}{
		{
			name:         "missing key",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "wrong key",
			headerKey:    "X-Admin-Key",
			headerVal:    "wrong-key",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "valid X-Admin-Key",
			headerKey:    "X-Admin-Key",
			headerVal:    adminKey,
			expectedCode: http.StatusOK,
		},
		{
			name:         "valid Bearer token",
			headerKey:    "Authorization",
			headerVal:    "Bearer " + adminKey,
			expectedCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
			if tt.headerKey != "" {
				req.Header.Set(tt.headerKey, tt.headerVal)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedCode {
				t.Errorf("expected status %d, got %d", tt.expectedCode, rec.Code)
			}
		})
	}
}
