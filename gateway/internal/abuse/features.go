package abuse

import (
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Feature indices and constants matching features/spec.py (Spec v1.0.0).
const (
	NumFeatures = 6

	FeatureRequestsLast60s        = 0
	FeatureAvgIntervalMs          = 1
	FeatureIntervalStddevMs       = 2
	FeatureDistinctEndpointsLast5m = 3
	FeatureFailedAuthCountLast5m  = 4
	FeatureHeaderAnomalyScore     = 5

	Window60s = 60 * time.Second
	Window5m  = 5 * time.Minute
)

// FeatureNames lists canonical feature identifiers in order.
var FeatureNames = [NumFeatures]string{
	"requests_last_60s",
	"avg_interval_ms",
	"interval_stddev_ms",
	"distinct_endpoints_last_5m",
	"failed_auth_count_last_5m",
	"header_anomaly_score",
}

// RequestEvent holds past event metadata in the in-memory sliding tracker.
type RequestEvent struct {
	Timestamp  time.Time
	Path       string
	StatusCode int
}

// FeatureExtractor extracts the 6-dimensional ML feature vector from request telemetry.
type FeatureExtractor struct {
	mu      sync.RWMutex
	history map[string][]RequestEvent
}

// NewFeatureExtractor creates a new thread-safe in-memory FeatureExtractor.
func NewFeatureExtractor() *FeatureExtractor {
	return &FeatureExtractor{
		history: make(map[string][]RequestEvent),
	}
}

// ComputeHeaderAnomaly calculates heuristic header penalty [0.0, 1.0].
func ComputeHeaderAnomaly(header http.Header, userAgent string) float32 {
	var score float32

	ua := strings.TrimSpace(userAgent)
	if ua == "" && header != nil {
		ua = strings.TrimSpace(header.Get("User-Agent"))
	}

	if ua == "" {
		score += 0.4
	} else {
		uaLower := strings.ToLower(ua)
		toolKeywords := []string{
			"python-requests",
			"sqlmap",
			"nikto",
			"curl",
			"go-http-client",
			"headlesschrome",
			"wpscan",
			"dirbuster",
			"postman",
		}

		matchedTool := false
		for _, kw := range toolKeywords {
			if strings.Contains(uaLower, kw) {
				score += 0.5
				matchedTool = true
				break
			}
		}

		if !matchedTool && strings.Contains(uaLower, "mozilla") {
			browsers := []string{"chrome/", "firefox/", "safari/", "edg/", "version/"}
			hasBrowser := false
			for _, b := range browsers {
				if strings.Contains(uaLower, b) {
					hasBrowser = true
					break
				}
			}
			if !hasBrowser {
				score += 0.3
			}
		}
	}

	if header == nil || header.Get("Accept-Language") == "" {
		score += 0.2
	}

	if header != nil && strings.TrimSpace(header.Get("Accept")) == "*/*" {
		score += 0.1
	}

	if score > 1.0 {
		return 1.0
	}
	if score < 0.0 {
		return 0.0
	}
	return float32(math.Round(float64(score)*1000) / 1000)
}

// Extract computes the 6-element feature vector for an incoming request.
// It also updates the client's trailing history window.
func (fe *FeatureExtractor) Extract(req *RequestContext, statusCode int) []float32 {
	now := req.Timestamp
	if now.IsZero() {
		now = time.Now().UTC()
	}

	key := req.ClientID
	if key == "" {
		key = req.IP
	}

	fe.mu.Lock()
	defer fe.mu.Unlock()

	// Append current event
	events := append(fe.history[key], RequestEvent{
		Timestamp:  now,
		Path:       req.Path,
		StatusCode: statusCode,
	})

	// Prune events older than 5 minutes
	cutoff5m := now.Add(-Window5m)
	idx5m := 0
	for idx5m < len(events) && events[idx5m].Timestamp.Before(cutoff5m) {
		idx5m++
	}
	events = events[idx5m:]
	fe.history[key] = events

	// Filter for 60s window
	cutoff60s := now.Add(-Window60s)
	var events60s []RequestEvent
	for _, ev := range events {
		if !ev.Timestamp.Before(cutoff60s) {
			events60s = append(events60s, ev)
		}
	}

	// 0: requests_last_60s
	requestsLast60s := float32(len(events60s))

	// 1 & 2: avg_interval_ms and interval_stddev_ms
	var avgIntervalMs, intervalStddevMs float32
	if len(events60s) >= 2 {
		deltas := make([]float64, len(events60s)-1)
		var sumDeltas float64
		for i := 1; i < len(events60s); i++ {
			delta := float64(events60s[i].Timestamp.Sub(events60s[i-1].Timestamp).Microseconds()) / 1000.0
			deltas[i-1] = delta
			sumDeltas += delta
		}
		meanDelta := sumDeltas / float64(len(deltas))
		avgIntervalMs = float32(meanDelta)

		if len(deltas) >= 2 {
			var sumSqDiff float64
			for _, d := range deltas {
				diff := d - meanDelta
				sumSqDiff += diff * diff
			}
			variance := sumSqDiff / float64(len(deltas)-1)
			intervalStddevMs = float32(math.Sqrt(math.Max(0.0, variance)))
		}
	}

	// 3: distinct_endpoints_last_5m
	uniquePaths := make(map[string]struct{}, len(events))
	var failedAuthCount float32
	for _, ev := range events {
		uniquePaths[ev.Path] = struct{}{}
		if ev.StatusCode == 401 || ev.StatusCode == 403 {
			failedAuthCount++
		}
	}
	distinctEndpointsLast5m := float32(len(uniquePaths))

	// 5: header_anomaly_score
	anomalyScore := ComputeHeaderAnomaly(req.Header, req.UserAgent)

	return []float32{
		requestsLast60s,
		avgIntervalMs,
		intervalStddevMs,
		distinctEndpointsLast5m,
		failedAuthCount,
		anomalyScore,
	}
}

// ColdStartVector returns the default baseline feature vector [1.0, 0.0, 0.0, 1.0, 0.0, 0.0].
func ColdStartVector() []float32 {
	return []float32{1.0, 0.0, 0.0, 1.0, 0.0, 0.0}
}
