package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type mockStore struct {
	clients []ClientSummary
	logs    []LogEntry
	stats   *SystemStats
}

func (m *mockStore) ListClients(ctx context.Context) ([]ClientSummary, error) {
	return m.clients, nil
}

func (m *mockStore) GetClient(ctx context.Context, id string) (*ClientSummary, error) {
	for _, c := range m.clients {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, fmt.Errorf("client not found")
}

func (m *mockStore) CreateClient(ctx context.Context, params NewClientParams) (*ClientSummary, string, error) {
	u, _ := url.Parse(params.TargetURL)
	c := ClientSummary{
		ID:              "client-mock-1",
		Name:            params.Name,
		TargetURL:       u,
		TargetURLString: params.TargetURL,
		RateLimit:       params.RateLimit,
		CreatedAt:       time.Now().UTC(),
	}
	m.clients = append(m.clients, c)
	return &c, "gk_live_mocksecretkey12345", nil
}

func (m *mockStore) UpdateClient(ctx context.Context, id string, params NewClientParams) (*ClientSummary, error) {
	for i, c := range m.clients {
		if c.ID == id {
			if params.Name != "" {
				m.clients[i].Name = params.Name
			}
			return &m.clients[i], nil
		}
	}
	return nil, fmt.Errorf("client not found")
}

func (m *mockStore) DeleteClient(ctx context.Context, id string) error {
	for i, c := range m.clients {
		if c.ID == id {
			m.clients = append(m.clients[:i], m.clients[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("client not found")
}

func (m *mockStore) GetSystemStats(ctx context.Context) (*SystemStats, error) {
	if m.stats != nil {
		return m.stats, nil
	}
	return &SystemStats{
		TotalRequests:       100,
		AllowedRequests:     80,
		CleanTrafficPercent: 80.0,
		ThreatVelocity:      2.5,
		ActiveClientsCount:  len(m.clients),
		MLModelActive:       true,
		MLModelPath:         "abuse_model.onnx",
	}, nil
}

func (m *mockStore) GetRecentLogs(ctx context.Context, filter LogFilter) ([]LogEntry, error) {
	return m.logs, nil
}

func setupTestAPI() (http.Handler, *mockStore) {
	u, _ := url.Parse("http://localhost:9000")
	mock := &mockStore{
		clients: []ClientSummary{
			{
				ID:              "client-1",
				Name:            "Test Client",
				TargetURL:       u,
				TargetURLString: "http://localhost:9000",
				RateLimit:       100,
				CreatedAt:       time.Now().UTC(),
			},
		},
		logs: []LogEntry{
			{
				ID:         "log-1",
				ClientID:   "client-1",
				Timestamp:  time.Now().UTC(),
				IP:         "127.0.0.1",
				Path:       "/api/v1/products",
				Outcome:    "allowed",
				AbuseScore: 0.1,
			},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(mock, logger)
	return router, mock
}

func TestAPIGetStats(t *testing.T) {
	router, _ := setupTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var stats SystemStats
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if stats.TotalRequests != 100 || !stats.MLModelActive {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestAPIListClients(t *testing.T) {
	router, _ := setupTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/clients", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		Clients []ClientSummary `json:"clients"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Clients) != 1 || resp.Clients[0].Name != "Test Client" {
		t.Errorf("unexpected clients list: %+v", resp.Clients)
	}
}

func TestAPICreateClient(t *testing.T) {
	router, _ := setupTestAPI()

	body, _ := json.Marshal(CreateClientRequest{
		Name:      "New Tenant",
		TargetURL: "http://localhost:9001",
		RateLimit: 500,
	})

	req := httptest.NewRequest(http.MethodPost, "/clients", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp CreateClientResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Client.Name != "New Tenant" || resp.APIKey != "gk_live_mocksecretkey12345" {
		t.Errorf("unexpected create response: %+v", resp)
	}
}

func TestAPIGetLogs(t *testing.T) {
	router, _ := setupTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/logs?limit=10", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		Logs  []LogEntry `json:"logs"`
		Count int        `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Count != 1 || resp.Logs[0].Path != "/api/v1/products" {
		t.Errorf("unexpected logs response: %+v", resp)
	}
}
