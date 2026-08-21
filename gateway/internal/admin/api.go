package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// API handles internal administrative REST requests.
type API struct {
	store  Store
	logger *slog.Logger
}

// NewRouter mounts all admin JSON API endpoints under a chi router.
func NewRouter(store Store, logger *slog.Logger) http.Handler {
	api := &API{
		store:  store,
		logger: logger,
	}

	r := chi.NewRouter()

	r.Get("/stats", api.handleGetStats)
	r.Get("/clients", api.handleListClients)
	r.Post("/clients", api.handleCreateClient)
	r.Get("/clients/{id}", api.handleGetClient)
	r.Put("/clients/{id}", api.handleUpdateClient)
	r.Delete("/clients/{id}", api.handleDeleteClient)
	r.Get("/logs", api.handleGetLogs)
	r.Get("/system", api.handleGetSystem)

	return r
}

func (a *API) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.GetSystemStats(r.Context())
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.respondJSON(w, http.StatusOK, stats)
}

func (a *API) handleListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := a.store.ListClients(r.Context())
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if clients == nil {
		clients = []ClientSummary{}
	}
	a.respondJSON(w, http.StatusOK, map[string]any{"clients": clients})
}

// CreateClientRequest defines the JSON payload for creating a tenant.
type CreateClientRequest struct {
	Name      string `json:"name"`
	TargetURL string `json:"target_url"`
	RateLimit int    `json:"rate_limit"`
	PlanTier  string `json:"plan_tier"`
}

// CreateClientResponse returns the created tenant and its live plaintext API key.
type CreateClientResponse struct {
	Client *ClientSummary `json:"client"`
	APIKey string         `json:"api_key"`
}

func (a *API) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	var req CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Name == "" {
		a.respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TargetURL == "" {
		a.respondError(w, http.StatusBadRequest, "target_url is required")
		return
	}

	clientSummary, rawKey, err := a.store.CreateClient(r.Context(), NewClientParams{
		Name:      req.Name,
		TargetURL: req.TargetURL,
		RateLimit: req.RateLimit,
		PlanTier:  req.PlanTier,
	})
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.respondJSON(w, http.StatusCreated, CreateClientResponse{
		Client: clientSummary,
		APIKey: rawKey,
	})
}

func (a *API) handleGetClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	clientSummary, err := a.store.GetClient(r.Context(), id)
	if err != nil {
		a.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	a.respondJSON(w, http.StatusOK, clientSummary)
}

func (a *API) handleUpdateClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	clientSummary, err := a.store.UpdateClient(r.Context(), id, NewClientParams{
		Name:      req.Name,
		TargetURL: req.TargetURL,
		RateLimit: req.RateLimit,
		PlanTier:  req.PlanTier,
	})
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.respondJSON(w, http.StatusOK, clientSummary)
}

func (a *API) handleDeleteClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.store.DeleteClient(r.Context(), id); err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

func (a *API) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	filter := LogFilter{
		ClientID: q.Get("client_id"),
		Outcome:  q.Get("outcome"),
		Limit:    limit,
		Offset:   offset,
	}

	logs, err := a.store.GetRecentLogs(r.Context(), filter)
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if logs == nil {
		logs = []LogEntry{}
	}

	a.respondJSON(w, http.StatusOK, map[string]any{
		"logs":  logs,
		"count": len(logs),
	})
}

func (a *API) handleGetSystem(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.GetSystemStats(r.Context())
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.respondJSON(w, http.StatusOK, map[string]any{
		"status":           "healthy",
		"ml_model_active":  stats.MLModelActive,
		"ml_model_path":    stats.MLModelPath,
		"active_clients":   stats.ActiveClientsCount,
		"threat_velocity":  stats.ThreatVelocity,
		"total_requests":   stats.TotalRequests,
	})
}

func (a *API) respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (a *API) respondError(w http.ResponseWriter, status int, message string) {
	a.respondJSON(w, status, map[string]string{"error": message})
}
