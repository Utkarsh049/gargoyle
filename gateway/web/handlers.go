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
	store  admin.Store
	logger *slog.Logger
	tmpl   *template.Template
}

// NewHandler initializes the dashboard web handler and compiles embedded templates.
func NewHandler(store admin.Store, logger *slog.Logger) (*Handler, error) {
	tmpl, err := template.ParseFS(embeddedFiles, "templates/*.html", "templates/fragments/*.html")
	if err != nil {
		return nil, fmt.Errorf("web: parsing embedded templates: %w", err)
	}

	return &Handler{
		store:  store,
		logger: logger,
		tmpl:   tmpl,
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
	}

	data := PageData{
		Title:        "Overview",
		SectionTitle: "Overview",
		ActiveNav:    "dashboard",
		Stats:        stats,
	}

	h.renderTemplate(w, "dashboard.html", data)
}

func (h *Handler) handleClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.store.ListClients(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: failed to list clients", "error", err)
	}

	data := PageData{
		Title:        "Clients",
		SectionTitle: "Client Ingress Management",
		ActiveNav:    "clients",
		Clients:      clients,
	}

	h.renderTemplate(w, "clients.html", data)
}

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	outcome := r.URL.Query().Get("outcome")
	logs, err := h.store.GetRecentLogs(r.Context(), admin.LogFilter{
		Outcome: outcome,
		Limit:   50,
	})
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: failed to get recent logs", "error", err)
	}

	data := PageData{
		Title:           "Security Logs",
		SectionTitle:    "Decision Audit Trail",
		ActiveNav:       "logs",
		Logs:            logs,
		SelectedOutcome: outcome,
	}

	h.renderTemplate(w, "logs.html", data)
}

func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetSystemStats(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: failed to get system stats", "error", err)
	}

	data := PageData{
		Title:        "Settings",
		SectionTitle: "System Configuration",
		ActiveNav:    "settings",
		Stats:        stats,
	}

	h.renderTemplate(w, "settings.html", data)
}

func (h *Handler) handleFragmentStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetSystemStats(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: fragment stats error", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := PageData{
		Stats: stats,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tmpl.ExecuteTemplate(w, "stats.html", data)
}

func (h *Handler) handleFragmentLogs(w http.ResponseWriter, r *http.Request) {
	outcome := r.URL.Query().Get("outcome")
	logs, err := h.store.GetRecentLogs(r.Context(), admin.LogFilter{
		Outcome: outcome,
		Limit:   50,
	})
	if err != nil {
		h.logger.ErrorContext(r.Context(), "web: fragment logs error", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := PageData{
		Logs:            logs,
		SelectedOutcome: outcome,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tmpl.ExecuteTemplate(w, "logs.html", data)
}

func (h *Handler) handleCreateClientForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	targetURL := r.FormValue("target_url")
	rateLimit, _ := strconv.Atoi(r.FormValue("rate_limit"))
	planTier := r.FormValue("plan_tier")

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
	// Clone template so execution is isolated
	t, err := template.ParseFS(embeddedFiles, "templates/layout.html", "templates/"+name)
	if err != nil {
		h.logger.Error("web: failed to parse layout and template", "name", name, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := t.Execute(w, data); err != nil {
		h.logger.Error("web: template execution error", "name", name, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
