package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gargoyle/internal/admin"
)

type mockAdminStore struct{}

func (m *mockAdminStore) ListClients(ctx context.Context) ([]admin.ClientSummary, error) {
	u, _ := url.Parse("http://localhost:9000")
	return []admin.ClientSummary{
		{
			ID:              "client-1",
			Name:            "Demo App",
			TargetURL:       u,
			TargetURLString: "http://localhost:9000",
			RateLimit:       1000,
			PlanTier:        "enterprise",
			CreatedAt:       time.Now(),
		},
	}, nil
}

func (m *mockAdminStore) GetClient(ctx context.Context, id string) (*admin.ClientSummary, error) {
	return nil, nil
}

func (m *mockAdminStore) CreateClient(ctx context.Context, params admin.NewClientParams) (*admin.ClientSummary, string, error) {
	return nil, "gk_live_mock", nil
}

func (m *mockAdminStore) UpdateClient(ctx context.Context, id string, params admin.NewClientParams) (*admin.ClientSummary, error) {
	return nil, nil
}

func (m *mockAdminStore) DeleteClient(ctx context.Context, id string) error {
	return nil
}

func (m *mockAdminStore) GetSystemStats(ctx context.Context) (*admin.SystemStats, error) {
	return &admin.SystemStats{
		TotalRequests:       1500,
		AllowedRequests:     1400,
		CleanTrafficPercent: 93.3,
		ThreatVelocity:      5.7,
		AvailableCapacity:   83,
		MLModelActive:       true,
		Equalizers: []admin.EqualizerColumn{
			{
				Title:      "Allowed Traffic",
				Direction:  "up",
				ValueRange: "52–71",
				Unit:       "req/s",
				Bars:       []admin.EqualizerBar{{Height: 50, IsActive: true}},
			},
		},
		WeeklyDays: []admin.DayStat{
			{Day: "Mon", Direction: "up", Value: 276, IsActive: false},
			{Day: "Wed", Direction: "up", Value: 297, IsActive: true},
		},
		TimelineStops: []admin.TimelineStop{
			{Time: "11AM", IsActive: true, IsSolid: true},
		},
	}, nil
}

func (m *mockAdminStore) GetRecentLogs(ctx context.Context, filter admin.LogFilter) ([]admin.LogEntry, error) {
	return []admin.LogEntry{
		{
			ID:         "log-1",
			ClientID:   "client-1",
			ClientName: "Demo App",
			Timestamp:  time.Now(),
			IP:         "127.0.0.1",
			Path:       "/api/v1/products",
			Outcome:    "allowed",
			AbuseScore: 0.1,
			Reason:     "Clean",
		},
	}, nil
}

func TestWebHandler_Dashboard(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h, err := NewHandler(&mockAdminStore{}, logger)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	routes := h.Routes()

	tests := []struct {
		path         string
		expectedCode int
	}{
		{"/dashboard", http.StatusOK},
		{"/clients", http.StatusOK},
		{"/logs", http.StatusOK},
		{"/settings", http.StatusOK},
		{"/web/fragments/stats", http.StatusOK},
		{"/web/fragments/logs", http.StatusOK},
		{"/static/styles.css", http.StatusOK},
		{"/static/dashboard.js", http.StatusOK},
		{"/", http.StatusMovedPermanently},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, req)

			if rec.Code != tt.expectedCode {
				t.Errorf("path %s: expected status %d, got %d", tt.path, tt.expectedCode, rec.Code)
			}
		})
	}
}
