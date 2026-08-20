package abuse

import (
	"math"
	"net/http"
	"testing"
	"time"
)

func floatEquals(a, b float32, eps float64) bool {
	return math.Abs(float64(a-b)) <= eps
}

func TestColdStartVector(t *testing.T) {
	v := ColdStartVector()
	if len(v) != NumFeatures {
		t.Fatalf("expected vector len %d, got %d", NumFeatures, len(v))
	}
	expected := []float32{1.0, 0.0, 0.0, 1.0, 0.0, 0.0}
	for i, exp := range expected {
		if v[i] != exp {
			t.Errorf("feature %d (%s): expected %f, got %f", i, FeatureNames[i], exp, v[i])
		}
	}
}

func TestComputeHeaderAnomaly(t *testing.T) {
	tests := []struct {
		name      string
		header    http.Header
		userAgent string
		expected  float32
	}{
		{
			name: "clean browser headers",
			header: http.Header{
				"User-Agent":      []string{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"},
				"Accept":          []string{"text/html,application/json"},
				"Accept-Language": []string{"en-US,en;q=0.9"},
			},
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
			expected:  0.0,
		},
		{
			name: "python-requests tool without accept-language",
			header: http.Header{
				"User-Agent": []string{"python-requests/2.31.0"},
				"Accept":     []string{"application/json"},
			},
			userAgent: "python-requests/2.31.0",
			expected:  0.7, // 0.5 (tool UA) + 0.2 (missing Accept-Language)
		},
		{
			name: "sqlmap scanner with wildcard accept and no accept-language",
			header: http.Header{
				"User-Agent": []string{"sqlmap/1.7#stable"},
				"Accept":     []string{"*/*"},
			},
			userAgent: "sqlmap/1.7#stable",
			expected:  0.8, // 0.5 (tool UA) + 0.2 (no Accept-Language) + 0.1 (wildcard accept)
		},
		{
			name:      "missing user agent",
			header:    http.Header{},
			userAgent: "",
			expected:  0.6, // 0.4 (missing UA) + 0.2 (missing Accept-Language)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ComputeHeaderAnomaly(tt.header, tt.userAgent)
			if !floatEquals(score, tt.expected, 0.001) {
				t.Errorf("expected anomaly score %f, got %f", tt.expected, score)
			}
		})
	}
}

func TestFeatureExtractor_ParityFixtures(t *testing.T) {
	// Test case 1: Cold start single request
	t.Run("cold_start_single_request", func(t *testing.T) {
		fe := NewFeatureExtractor()
		req := &RequestContext{
			ClientID:  "192.168.1.10",
			Path:      "/api/v1/products",
			Timestamp: time.Unix(1700000000, 0).UTC(),
			Header: http.Header{
				"User-Agent":      []string{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"},
				"Accept":          []string{"text/html,application/json"},
				"Accept-Language": []string{"en-US,en;q=0.9"},
			},
			UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
		}
		vec := fe.Extract(req, 200)
		expected := []float32{1.0, 0.0, 0.0, 1.0, 0.0, 0.0}
		for i, exp := range expected {
			if !floatEquals(vec[i], exp, 0.01) {
				t.Errorf("feature %d (%s): expected %f, got %f", i, FeatureNames[i], exp, vec[i])
			}
		}
	})

	// Test case 2: Normal human browsing (4 requests)
	t.Run("normal_human_browsing", func(t *testing.T) {
		fe := NewFeatureExtractor()
		clientIP := "192.168.1.20"
		hdr := http.Header{
			"User-Agent":      []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"},
			"Accept":          []string{"application/json"},
			"Accept-Language": []string{"en-US,en;q=0.9"},
		}

		events := []struct {
			tOffsetMs int64
			path      string
		}{
			{0, "/api/v1/search?q=laptop"},
			{600, "/api/v1/products/101"},
			{1000, "/api/v1/cart"},
			{1800, "/api/v1/products/101"},
		}

		baseTime := time.Unix(1700000000, 0).UTC()
		var lastVec []float32

		for _, ev := range events {
			req := &RequestContext{
				ClientID:  clientIP,
				Path:      ev.path,
				Timestamp: baseTime.Add(time.Duration(ev.tOffsetMs) * time.Millisecond),
				Header:    hdr,
				UserAgent: hdr.Get("User-Agent"),
			}
			lastVec = fe.Extract(req, 200)
		}

		expected := []float32{4.0, 600.0, 200.0, 3.0, 0.0, 0.0}
		for i, exp := range expected {
			if !floatEquals(lastVec[i], exp, 0.01) {
				t.Errorf("feature %d (%s): expected %f, got %f", i, FeatureNames[i], exp, lastVec[i])
			}
		}
	})

	// Test case 3: Brute force login burst
	t.Run("brute_force_login_burst", func(t *testing.T) {
		fe := NewFeatureExtractor()
		clientIP := "10.0.0.50"
		hdr := http.Header{
			"User-Agent": []string{"python-requests/2.31.0"},
			"Accept":     []string{"application/json"},
		}

		baseTime := time.Unix(1700000010, 0).UTC()
		var lastVec []float32

		for i := 0; i < 3; i++ {
			req := &RequestContext{
				ClientID:  clientIP,
				Path:      "/api/v1/auth/login",
				Timestamp: baseTime.Add(time.Duration(i*50) * time.Millisecond),
				Header:    hdr,
				UserAgent: hdr.Get("User-Agent"),
			}
			lastVec = fe.Extract(req, 401)
		}

		expected := []float32{3.0, 50.0, 0.0, 1.0, 3.0, 0.7}
		for i, exp := range expected {
			if !floatEquals(lastVec[i], exp, 0.01) {
				t.Errorf("feature %d (%s): expected %f, got %f", i, FeatureNames[i], exp, lastVec[i])
			}
		}
	})

	// Test case 4: Endpoint scanner dirbust
	t.Run("endpoint_scanner_dirbust", func(t *testing.T) {
		fe := NewFeatureExtractor()
		clientIP := "10.0.0.60"
		hdr := http.Header{
			"User-Agent": []string{"sqlmap/1.7#stable"},
			"Accept":     []string{"*/*"},
		}

		paths := []string{"/admin", "/wp-admin", "/api/v1/debug", "/api/v1/metrics"}
		baseTime := time.Unix(1700000020, 0).UTC()
		var lastVec []float32

		for i, p := range paths {
			req := &RequestContext{
				ClientID:  clientIP,
				Path:      p,
				Timestamp: baseTime.Add(time.Duration(i*100) * time.Millisecond),
				Header:    hdr,
				UserAgent: hdr.Get("User-Agent"),
			}
			lastVec = fe.Extract(req, 403)
		}

		expected := []float32{4.0, 100.0, 0.0, 4.0, 4.0, 0.8}
		for i, exp := range expected {
			if !floatEquals(lastVec[i], exp, 0.01) {
				t.Errorf("feature %d (%s): expected %f, got %f", i, FeatureNames[i], exp, lastVec[i])
			}
		}
	})
}
