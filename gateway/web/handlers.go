package web

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"

	"gargoyle/internal/admin"

	"github.com/go-chi/chi/v5"
)

// Handler serves server-rendered HTML dashboard pages and HTMX fragments.
type Handler struct {
	store     admin.Store
	logger    *slog.Logger
	templates map[string]*template.Template
}

// NewHandler initializes the dashboard web handler and compiles embedded templates.
func NewHandler(store admin.Store, logger *slog.Logger) (*Handler, error) {
	templates := make(map[string]*template.Template)

	pages := []struct {
		name  string
		files []string
	}{
		{"dashboard", []string{"templates/layout.html", "templates/dashboard.html"}},
		{"clients", []string{"templates/layout.html", "templates/clients.html"}},
		{"logs", []string{"templates/layout.html", "templates/logs.html"}},
		{"settings", []string{"templates/layout.html", "templates/settings.html"}},
		{"stats_fragment", []string{"templates/fragments/stats_fragment.html"}},
		{"logs_fragment", []string{"templates/fragments/logs_fragment.html"}},
	}

	for _, p := range pages {
		t, err := template.ParseFS(embeddedFiles, p.files...)
		if err != nil {
			return nil, fmt.Errorf("web: compiling template %q: %w", p.name, err)
		}
		templates[p.name] = t
	}

	return &Handler{
		store:     store,
		logger:    logger,
		templates: templates,
	}, nil
}

// MountRoutes registers all UI, static assets, and HTMX fragment routes directly onto a router.
func (h *Handler) MountRoutes(r chi.Router) {
	// Static assets
	r.Handle("/static/*", http.StripPrefix("/static/", StaticFS()))

	// Full Page Views
	r.Get("/dashboard", h.handleDashboard)
	r.Get("/clients", h.handleClients)
	r.Get("/logs", h.handleLogs)
	r.Get("/settings", h.handleSettings)

	// HTMX Live Polling Fragments
	r.Get("/web/fragments/stats", h.handleFragmentStats)
	r.Get("/web/fragments/logs", h.handleFragmentLogs)

	// Form actions
	r.Post("/web/clients/create", h.handleCreateClientForm)
	r.Delete("/web/clients/{id}", h.handleDeleteClient)

	// Root redirect
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusMovedPermanently)
	})
}

// Routes returns a standalone sub-router (for testing).
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	h.MountRoutes(r)
	return r
}

type PageData struct {
	Title           string
	SectionTitle    string
	ActiveNav       string
	Stats           *admin.SystemStats
	Clients         []admin.ClientSummary
	Logs            []admin.LogEntry
	SelectedOutcome string
	NewlyCreatedKey string
	Error           string
}

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetSystemStats(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: failed to load system stats", "error", err)
		stats = &admin.SystemStats{} // fallback to avoid nil dereference
	}

	data := PageData{
		Title:        "Overview",
		SectionTitle: "Overview",
		ActiveNav:    "dashboard",
		Stats:        stats,
	}

	h.renderTemplate(w, "dashboard", data)
}

func (h *Handler) handleClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.store.ListClients(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: failed to list clients", "error", err)
		clients = []admin.ClientSummary{}
	}

	data := PageData{
		Title:        "Clients",
		SectionTitle: "Client Ingress Management",
		ActiveNav:    "clients",
		Clients:      clients,
	}

	h.renderTemplate(w, "clients", data)
}

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	outcome := r.URL.Query().Get("outcome")
	logs, err := h.store.GetRecentLogs(r.Context(), admin.LogFilter{
		Outcome: outcome,
		Limit:   50,
	})
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: failed to get recent logs", "error", err)
		logs = []admin.LogEntry{}
	}

	data := PageData{
		Title:           "Security Logs",
		SectionTitle:    "Decision Audit Trail",
		ActiveNav:       "logs",
		Logs:            logs,
		SelectedOutcome: outcome,
	}

	h.renderTemplate(w, "logs", data)
}

func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetSystemStats(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: failed to get system stats", "error", err)
		stats = &admin.SystemStats{} // fallback to avoid nil dereference
	}

	data := PageData{
		Title:        "Settings",
		SectionTitle: "System Configuration",
		ActiveNav:    "settings",
		Stats:        stats,
	}

	h.renderTemplate(w, "settings", data)
}

func (h *Handler) handleFragmentStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetSystemStats(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: fragment stats error", "error", err)
		stats = &admin.SystemStats{}
	}

	data := PageData{
		Stats: stats,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := h.templates["stats_fragment"]
	if !ok {
		h.logger.Error("web: stats_fragment template not found")
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	if err := t.Execute(w, data); err != nil {
		h.logger.ErrorContext(r.Context(), "web: executing stats_fragment template", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) handleFragmentLogs(w http.ResponseWriter, r *http.Request) {
	outcome := r.URL.Query().Get("outcome")
	logs, err := h.store.GetRecentLogs(r.Context(), admin.LogFilter{
		Outcome: outcome,
		Limit:   50,
	})
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: fragment logs error", "error", err)
		logs = []admin.LogEntry{}
	}

	data := PageData{
		Logs:            logs,
		SelectedOutcome: outcome,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := h.templates["logs_fragment"]
	if !ok {
		h.logger.Error("web: logs_fragment template not found")
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	if err := t.Execute(w, data); err != nil {
		h.logger.ErrorContext(r.Context(), "web: executing logs_fragment template", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) handleCreateClientForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	targetURL := r.FormValue("target_url")
	if targetURL == "" {
		http.Error(w, "target_url is required", http.StatusBadRequest)
		return
	}

	rateLimitStr := r.FormValue("rate_limit")
	rateLimit, convErr := strconv.Atoi(rateLimitStr)
	if convErr != nil || rateLimit <= 0 {
		http.Error(w, "invalid rate_limit: must be a positive integer", http.StatusBadRequest)
		return
	}

	planTier := r.FormValue("plan_tier")
	if planTier == "" {
		planTier = "free"
	}

	_, _, err := h.store.CreateClient(r.Context(), admin.NewClientParams{
		Name:      name,
		TargetURL: targetURL,
		RateLimit: rateLimit,
		PlanTier:  planTier,
	})
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: failed to create client from form", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

func (h *Handler) handleDeleteClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteClient(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) renderTemplate(w http.ResponseWriter, name string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := h.templates[name]
	if !ok {
		h.logger.Error("web: template not found", "name", name)
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}

	if err := t.Execute(w, data); err != nil {
		h.logger.Error("web: template execution error", "name", name, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
