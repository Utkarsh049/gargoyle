# Gargoyle

**Gargoyle** is a self-hostable API gateway that sits in front of any backend and gives it rate limiting and behavioral abuse detection out of the box — the kind of protection API teams normally have to buy from Cloudflare, Kong, or AWS WAF, but open, self-hosted, and inspectable.

A gargoyle sits at the edge of a building and watches over it. That's the job here: Gargoyle stands between the internet and your application, deciding what gets in.

---

## The problem

Any API that gets real traffic eventually needs two things:

1. **Rate limiting** — stop any single client from overwhelming the backend
2. **Abuse detection** — catch traffic that's *within* the rate limit but still malicious: credential stuffing, scraping, bot traffic that paces itself to avoid tripping simple thresholds

Most teams either bolt on a basic rate limiter and call it done, or pay for a commercial gateway that does both but is a black box. Gargoyle aims to do both, properly, as something you can run yourself and actually understand.

---

## How it works, in one paragraph

A client's backend registers with Gargoyle and gets an API key. All traffic to that backend is routed through Gargoyle first. Gargoyle checks the request against that client's rate limit (Redis-backed) and scores it for abuse patterns using a rule-based heuristics layer that always runs, plus an optional ML classifier that adds a second score when a trained model is present. Allowed requests are forwarded cleanly to the real backend; blocked ones are logged. Every decision is recorded — aggregate stats go to Prometheus, per-client detail goes to Postgres — and surfaced on a dashboard the client can check anytime.

---

## Two ways to use it

**1. Standalone gateway (zero code changes)**
Run Gargoyle as a Docker container in front of your API. Point your DNS/traffic at Gargoyle instead of your backend directly. Works regardless of what language or framework your backend is written in.

**2. Thin SDK / middleware**
A small client library (e.g. for Express) that calls Gargoyle's decision API directly from inside your app, for teams who don't want a separate proxy hop in their infrastructure.

---

## Dashboard: single repo, no separate frontend

Gargoyle ships as one self-contained Go binary — the dashboard is server-rendered directly from the same process using Go's `html/template`, with `htmx` for live-updating tables and charts (polling, no SPA, no build step, no second process to run). A user gets rate limits, blocked-request logs, and abuse scores in the browser the moment they run the container — nothing extra to deploy.

Grafana remains available as an optional deeper observability layer for anyone who wants to point it at Gargoyle's Prometheus metrics, but it is not required to use the product day to day.

A separate, more polished frontend (e.g. Next.js) is a possible *future* extension once the core is stable — see TIMELINE.md — but is explicitly out of scope for the core build.

---

## Architecture overview

```
Client traffic
      |
      v
+-----------------+        +--------------+
|  Gargoyle (Go)  | -----> | Redis        |  (rate limit state)
|  reverse proxy  | -----> | Postgres     |  (client config + per-client logs)
|  + abuse check  | -----> | Prometheus   |  (aggregate metrics)
+-----------------+
      |
      v
User's real backend
```

```
Prometheus  ---\
                 +---> api/admin (JSON) ---> web/ (html/template + htmx) ---> browser
Postgres    ---/
```

The embedded dashboard is served by the same Go process — no separate frontend service sits between the data stores and the browser. Grafana can optionally be pointed at Gargoyle's Prometheus metrics directly for a deeper, system-wide view.

---

## Tech stack

| Layer | Technology | Why |
|---|---|---|
| Gateway core | Go + Chi | Thin, predictable, minimal overhead — matches how real infra tools (Traefik, Caddy) are built |
| Rate limit state | Redis | Fast in-memory counters, natural fit for token bucket / sliding window limits |
| Config + per-client logs | Postgres | Relational data (clients, API keys, targets), high-cardinality detail that doesn't belong in Prometheus |
| Aggregate metrics | Prometheus | Industry-standard time-series store for system-wide numbers |
| Dashboard UI | Go `html/template` + htmx | Server-rendered, ships inside the same binary — no separate frontend repo, process, or build step |
| Deep metrics (optional) | Grafana | Points at Gargoyle's Prometheus metrics for anyone who wants a richer internal view |
| Abuse detection model (optional) | Python (scikit-learn) trained, exported to ONNX, run from Go | A separate, independently-buildable project — see PYTHON.md. Handoff is a single `.onnx` file; Go runs inference in-process with no live Python service. Gargoyle works fully on rule-based detection alone if this file isn't present |
| Packaging | Docker / Docker Compose | The distribution model *is* the product — users run the image directly |

---

## Project structure (top level)

```
gargoyle/
  gateway/        Go core — proxy, rate limiter, abuse checker, metrics, admin API
    web/            html/template pages + htmx endpoints (embedded dashboard UI)
  ml/              Python training pipeline + exported ONNX model
  simulator/       Traffic generator (normal + attack patterns) for testing/demo
  deploy/          Docker Compose, Prometheus config, optional Grafana dashboards
```

Everything a user needs — gateway, rate limiting, abuse detection, and dashboard — lives inside `gateway/` and ships as a single Docker image.

---

## Quickstart (Docker Compose)

The easiest way to run the entire Gargoyle stack (Gateway, PostgreSQL, Redis, Prometheus, Mock Upstream Backend) is with Docker Compose:

```bash
# 1. Clone the repository
git clone https://github.com/Utkarsh049/gargoyle.git
cd gargoyle

# 2. Start the full stack
docker compose up -d
```

### Accessing Services

| Service | URL | Description |
|---|---|---|
| **Web Dashboard** | [http://localhost:8080/dashboard](http://localhost:8080/dashboard) | Live security telemetry, client manager, and audit logs |
| **API Ingress** | `http://localhost:8080/*` | Protected gateway routing to upstream backends |
| **Prometheus** | [http://localhost:9090](http://localhost:9090) | Scrapes gateway metrics at `/metrics` |
| **Mock Backend** | `http://localhost:9000` | Upstream target service |

### Generating Test Traffic

Run the built-in simulator to generate mixed clean, burst, and attack traffic against the pre-seeded demo key:

```bash
python3 tools/simulator/simulate.py \
  --target-url http://localhost:8080 \
  --api-key gk_live_mRLoRus8nmTOakGwOaEm5d99f7WwbD9t \
  -n 100 \
  --mode mixed
```

Watch the live decision audit logs and sparkline charts update in real-time on your dashboard at `http://localhost:8080/dashboard`!

