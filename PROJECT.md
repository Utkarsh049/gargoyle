# PROJECT.md — Gargoyle Go Core

This document covers the Go application only — `gateway/` in the repo. It's the heart of Gargoyle: the process that actually receives traffic, makes allow/block decisions, and forwards requests.

---

## 1. Responsibilities of the Go core

The Go core has exactly four jobs. Everything else (dashboard, ML training, simulator) is a separate process that talks *to* the Go core, not part of it.

1. **Identify the client** — figure out which registered customer this request belongs to
2. **Decide** — check rate limits and abuse signals, decide allow / block / flag
3. **Forward** — reverse-proxy the request to the client's real backend if allowed
4. **Record** — emit metrics (Prometheus) and log detail (Postgres) for every decision

---

## 2. Request lifecycle

```
1. Request arrives at Gargoyle
2. Extract client identifier (API key header, or subdomain)
3. Look up client config (target URL, rate limit, plan tier)
      -> cache in-memory with short TTL to avoid hitting Postgres on every request
4. Check rate limit for this client (Redis)
      -> over limit? return 429, log, stop here
5. Run abuse checks:
      a. Rule-based heuristics (always run)
      b. ML classifier score via ONNX runtime, IF a model is loaded (optional — see §6)
      -> score above threshold? block or flag depending on config, log, stop here
6. Forward request to client's target_url via reverse proxy
7. Record outcome:
      -> Prometheus counter/histogram (aggregate)
      -> Postgres row (per-client detail) if blocked/flagged, sampled if allowed
8. Return response to original caller
```

---

## 3. Package layout

```
gateway/
  cmd/
    gargoyle/          main.go — wiring, startup, graceful shutdown
  internal/
    proxy/              reverse proxy logic, request forwarding
    client/              client registry: lookup, in-memory cache, Postgres access
    ratelimit/          token bucket / sliding window implementation over Redis
    abuse/
      rules/              heuristic checks (timing, sequence, header anomalies)
      model/              ONNX model loading + inference wrapper
    metrics/             Prometheus metric definitions and /metrics handler
    logstore/            per-client detail logging to Postgres
    config/               environment/config file parsing
  api/
    admin/               internal API used by the dashboard (client CRUD, stats queries)
  web/
    templates/            html/template pages — dashboard, client list, logs view
    static/                 htmx script, minimal CSS
    handlers.go          renders templates, serves htmx partial responses
```

---

## 4. Core data models

**Client config** (Postgres — `clients` table)
```
id              uuid
name            text
api_key_hash    text
target_url      text
rate_limit      int      -- requests per minute
plan_tier       text     -- free / pro / enterprise
created_at      timestamp
```

**Decision log** (Postgres — `request_logs` table, written for blocked/flagged requests)
```
id              uuid
client_id       uuid
timestamp       timestamp
ip              text
path            text
outcome         text     -- allowed / rate_limited / blocked_abuse / flagged
abuse_score     float
reason          text     -- which rule or model fired
```

**Prometheus metrics** (aggregate, not per-client to avoid cardinality blowup)
```
gargoyle_requests_total{outcome="allowed|rate_limited|blocked_abuse"}
gargoyle_request_duration_seconds (histogram)
gargoyle_active_clients
gargoyle_abuse_score_distribution (histogram)
```

---

## 5. Rate limiting design

- Sliding window counter per `client_id`, stored in Redis with a TTL matching the window
- On each request: increment counter, check against `rate_limit` from client config
- Chosen over a fixed window to avoid the classic "burst at window boundary" problem
- Single-node first (Phase 3); shared Redis state means this design already supports multiple Gargoyle instances later without changes to the limiting logic itself — only the *deployment* changes (a stretch goal after Phase 10, see TIMELINE.md)

---

## 6. Abuse detection design

Abuse detection is deliberately modular: rule-based scoring always works standalone, and ML scoring is an optional layer bolted on top. This is implemented as a small interface, not a hard dependency:

```go
type AbuseScorer interface {
    Score(features []float64) (float64, error)
}

type RuleBasedScorer struct{}          // always available
type MLScorer struct{ session *onnxruntime_go.Session }  // only if a model loaded
```

At startup, Gargoyle looks for `abuse_model.onnx`. If it's present and loads successfully, `MLScorer` is added alongside the rules; if it's missing or fails to load, Gargoyle logs "ML scoring disabled, running rules-only" and continues with `RuleBasedScorer` alone. Nothing else in the codebase — proxying, rate limiting, logging, the dashboard — knows or cares which scorers are active. **Gargoyle is a complete, fully functional product without the ONNX file ever existing.**

**Layer A — rule-based heuristics** (always present, built in Go Phase 6)
- Request sequencing: near-identical time intervals between requests from the same client fingerprint
- Endpoint sweep detection: hitting many distinct endpoints in rapid, ordered succession
- Auth anomaly: repeated failed logins across different usernames from one source in a short window
- Header/UA inconsistency: mismatched header sets vs known browser signatures

**Layer B — ML classifier** (optional, added in Go Phase 8 only)
- Trained offline in Python — see PYTHON.md for the full pipeline; that document is a separate, independently-buildable project
- The only thing that crosses from Python to Go is the exported `abuse_model.onnx` file — a portable, language-neutral description of the model's math, not code. Go never runs Python at any point, in development or in production
- Loaded once at Gargoyle startup, inference run in-process via an ONNX Runtime Go binding, no network call per request

Final decision combines whichever scorers are active: a hard rule match blocks immediately; an elevated ML score (when present) contributes to a "flagged" state when no hard rule fired, useful for cases you want to log and review rather than auto-block.

---

## 7. Client identification

Two supported patterns, both resolve to the same internal client lookup:

- **Header-based:** `X-Gargoyle-Key: gk_live_...` — works with any DNS setup, minimal config on the client's end
- **Subdomain-based:** `acme-corp.gargoyle.dev` routes to the client registered under `acme-corp` — cleaner for teams that want a dedicated ingress point

---

## 8. Config caching

Client config is read from Postgres but cached in-memory (short TTL, e.g. 30s) inside the Go process. This avoids a database round-trip on every single request — the hot path only touches Redis (rate limit) and the in-memory cache (client config); Postgres is only touched on cache miss or when logging blocked requests.

---

## 9. Multi-tenancy guarantee

Every metric, log row, and rate-limit key is scoped by `client_id`. No shared state crosses tenant boundaries except the physical Redis/Postgres instances themselves — this is the property that makes "many projects behind one Gargoyle deployment" safe rather than accidentally sharing limits or leaking one client's stats into another's dashboard.

---

## 10. Embedded dashboard UI

The dashboard is not a separate application — it's server-rendered directly by the Go core using `html/template`, and lives in `web/`.

**How it works:**
- `api/admin` already exposes the data (client stats, blocked-request logs, abuse scores) as JSON for programmatic use
- `web/handlers.go` reuses the same underlying queries and renders them as HTML pages instead of JSON, using Go's `html/template`
- **htmx** (a single `<script>` tag, no build step) is used for live updates: a table or stat card is given `hx-get="/web/stats" hx-trigger="every 5s"`, and htmx quietly re-fetches and swaps in just that fragment — this is what produces the "auto-refreshing" feel discussed earlier, without writing any client-side JavaScript by hand

**What pages exist:**
- `/dashboard` — overview: requests allowed/blocked today, active clients, current abuse score distribution
- `/clients` — list of registered clients, add/edit a client (target URL, rate limit)
- `/clients/{id}/logs` — recent blocked/flagged requests for one client, with reason and score

**Why this works well enough not to need a separate frontend:**
- No CORS configuration, no separate deploy target, no API versioning concerns between two codebases
- htmx's polling model is a legitimate, production-used pattern (not a hack) for dashboards that don't need sub-second live updates
- The moment `api/admin` exists (Phase 9), the UI layer on top of it is a small addition, not a new subsystem

**Where Grafana still fits:** Grafana remains available as an optional, deeper view for anyone who wants to point it at Gargoyle's `/metrics` Prometheus endpoint directly — useful for system-wide trends across all clients, which isn't the embedded UI's job (the embedded UI is intentionally per-client and operational, not a full observability suite).

---

## 11. What's explicitly out of scope for the Go core

- Model training — lives in `ml/` (Python), a separate project — see PYTHON.md. The Go core only ever consumes the resulting `.onnx` file
- Attack traffic generation — lives in `simulator/`
- Billing/tiers UI — deferred, not part of the core engineering problem
- A separate, more polished frontend (e.g. Next.js) — possible future project built against the existing `api/admin` endpoints, not part of this repo's core

Keeping these separate is deliberate: the Go core should be small, fast, and easy to reason about on its own. Note that the embedded dashboard UI (§10) is *not* out of scope — it ships inside `gateway/` as part of the core.
