package admin

import (
	"context"
	"net/url"
	"time"
)

// ClientSummary represents a registered tenant with summary telemetry.
type ClientSummary struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	APIKeyHash      string     `json:"api_key_hash,omitempty"`
	TargetURL       *url.URL   `json:"target_url"`
	TargetURLString string     `json:"target_url_str"`
	RateLimit       int        `json:"rate_limit"`
	PlanTier        string     `json:"plan_tier"`
	CreatedAt       time.Time  `json:"created_at"`
	TotalRequests   int64      `json:"total_requests"`
	BlockedRequests int64      `json:"blocked_requests"`
	LastSeen        *time.Time `json:"last_seen,omitempty"`
}

// NewClientParams contains attributes required to create a new client.
type NewClientParams struct {
	Name      string `json:"name"`
	TargetURL string `json:"target_url"`
	RateLimit int    `json:"rate_limit"`
	PlanTier  string `json:"plan_tier"`
}

// LogEntry describes an individual security decision logged in the database.
type LogEntry struct {
	ID         string    `json:"id"`
	ClientID   string    `json:"client_id"`
	ClientName string    `json:"client_name,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	IP         string    `json:"ip"`
	Path       string    `json:"path"`
	Outcome    string    `json:"outcome"`
	AbuseScore float64   `json:"abuse_score"`
	Reason     string    `json:"reason"`
}

// LogFilter specifies search and filtering criteria for request logs.
type LogFilter struct {
	ClientID string `json:"client_id,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

// EqualizerBar represents an individual bar in a sparkline equalizer histogram.
type EqualizerBar struct {
	Height   int  `json:"height"`   // Percentage 10-100
	IsActive bool `json:"is_active"` // High-contrast highlight
}

// EqualizerColumn represents a monitored traffic category in Card 1.
type EqualizerColumn struct {
	Title      string         `json:"title"`
	Direction  string         `json:"direction"` // "up" or "down"
	ValueRange string         `json:"value_range"`
	Unit       string         `json:"unit"`
	Bars       []EqualizerBar `json:"bars"`
}

// DayStat represents a single day in the weekly calendar matrix (Card 5).
type DayStat struct {
	Day       string `json:"day"`       // "Mon", "Tue", etc.
	Direction string `json:"direction"` // "up" or "down"
	Value     int    `json:"value"`     // Consumption / request volume
	IsActive  bool   `json:"is_active"` // Elevated highlight pill
}

// TimelineStop represents a time node in the connected pipeline slider (Card 6).
type TimelineStop struct {
	Time     string `json:"time"`
	IsActive bool   `json:"is_active"`
	IsSolid  bool   `json:"is_solid"`
}

// SystemStats contains aggregated KPIs and visual metrics for the dashboard.
type SystemStats struct {
	TotalRequests          int64             `json:"total_requests"`
	AllowedRequests        int64             `json:"allowed_requests"`
	BlockedAbuseRequests   int64             `json:"blocked_abuse_requests"`
	RateLimitedRequests    int64             `json:"rate_limited_requests"`
	CleanTrafficPercent    float64           `json:"clean_traffic_percent"`
	ThreatVelocity         float64           `json:"threat_velocity"` // Blocked reqs/sec
	ActiveClientsCount     int               `json:"active_clients_count"`
	AvailableCapacity      int               `json:"available_capacity"` // Percentage (e.g. 83)
	MLModelActive          bool              `json:"ml_model_active"`
	MLModelPath            string            `json:"ml_model_path"`
	Equalizers             []EqualizerColumn `json:"equalizers"`
	WeeklyDays             []DayStat         `json:"weekly_days"`
	TimelineStops          []TimelineStop    `json:"timeline_stops"`
	RecentRecommendations  []Recommendation  `json:"recommendations"`
}

// Recommendation holds security insights displayed in Card 3.
type Recommendation struct {
	Type        string `json:"type"` // "primary" or "secondary"
	Title       string `json:"title"`
	Description string `json:"description"`
	Tag         string `json:"tag"`
	Timestamp   string `json:"timestamp"`
}

// Store defines database operations for admin management and statistics.
type Store interface {
	ListClients(ctx context.Context) ([]ClientSummary, error)
	GetClient(ctx context.Context, id string) (*ClientSummary, error)
	CreateClient(ctx context.Context, params NewClientParams) (*ClientSummary, string, error)
	UpdateClient(ctx context.Context, id string, params NewClientParams) (*ClientSummary, error)
	DeleteClient(ctx context.Context, id string) error
	GetSystemStats(ctx context.Context) (*SystemStats, error)
	GetRecentLogs(ctx context.Context, filter LogFilter) ([]LogEntry, error)
}
