package rules

import (
	"context"
	"net/http"
	"testing"
	"time"

	"gargoyle/internal/abuse"
)

func TestHeaderAnomalyRule(t *testing.T) {
	rule := NewHeaderAnomalyRule()
	ctx := context.Background()

	tests := []struct {
		name           string
		req            *abuse.RequestContext
		expectedAction abuse.Action
		minScore       float64
	}{
		{
			name: "Clean legitimate browser request",
			req: &abuse.RequestContext{
				ClientID:  "client-1",
				IP:        "192.0.2.1",
				Path:      "/api/v1/data",
				Method:    "GET",
				UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
				Header: http.Header{
					"Accept": []string{"application/json, text/plain, */*"},
				},
				Timestamp: time.Now(),
			},
			expectedAction: abuse.ActionAllow,
			minScore:       0.0,
		},
		{
			name: "Missing User-Agent header",
			req: &abuse.RequestContext{
				ClientID:  "client-1",
				IP:        "192.0.2.1",
				Path:      "/api/v1/data",
				Method:    "GET",
				UserAgent: "",
				Header:    http.Header{},
				Timestamp: time.Now(),
			},
			expectedAction: abuse.ActionBlock,
			minScore:       0.85,
		},
		{
			name: "Curl automated scraper",
			req: &abuse.RequestContext{
				ClientID:  "client-1",
				IP:        "192.0.2.1",
				Path:      "/api/v1/data",
				Method:    "GET",
				UserAgent: "curl/7.88.1",
				Header:    http.Header{},
				Timestamp: time.Now(),
			},
			expectedAction: abuse.ActionBlock,
			minScore:       0.90,
		},
		{
			name: "Python requests scraper",
			req: &abuse.RequestContext{
				ClientID:  "client-1",
				IP:        "192.0.2.1",
				Path:      "/api/v1/data",
				Method:    "GET",
				UserAgent: "python-requests/2.31.0",
				Header:    http.Header{},
				Timestamp: time.Now(),
			},
			expectedAction: abuse.ActionBlock,
			minScore:       0.90,
		},
		{
			name: "Sqlmap attack tool",
			req: &abuse.RequestContext{
				ClientID:  "client-1",
				IP:        "192.0.2.1",
				Path:      "/api/v1/data",
				Method:    "GET",
				UserAgent: "sqlmap/1.7.2#stable",
				Header:    http.Header{},
				Timestamp: time.Now(),
			},
			expectedAction: abuse.ActionBlock,
			minScore:       0.90,
		},
		{
			name: "Headless Chrome automation",
			req: &abuse.RequestContext{
				ClientID:  "client-1",
				IP:        "192.0.2.1",
				Path:      "/api/v1/data",
				Method:    "GET",
				UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/119.0.6045.105 Safari/537.36",
				Header: http.Header{
					"Accept": []string{"*/*"},
				},
				Timestamp: time.Now(),
			},
			expectedAction: abuse.ActionBlock,
			minScore:       0.85,
		},
		{
			name: "Spoofed browser User-Agent without Accept header",
			req: &abuse.RequestContext{
				ClientID:  "client-1",
				IP:        "192.0.2.1",
				Path:      "/api/v1/data",
				Method:    "GET",
				UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
				Header:    http.Header{}, // Missing Accept header
				Timestamp: time.Now(),
			},
			expectedAction: abuse.ActionBlock,
			minScore:       0.80,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dec, err := rule.Evaluate(ctx, tc.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if dec.Action != tc.expectedAction {
				t.Fatalf("expected action %q, got %q (reason: %s)", tc.expectedAction, dec.Action, dec.Reason)
			}
			if dec.Score < tc.minScore {
				t.Fatalf("expected score >= %v, got %v", tc.minScore, dec.Score)
			}
		})
	}
}
