# TIMELINE.md — Gargoyle Go Core Build Plan

Ten phases, ordered so that every phase produces something runnable and demoable — no phase leaves you with a half-working black box. Each phase lists what you're building, why it's positioned there, and what "done" looks like.

---

## Progress

- [x] Phase 1 — Bare reverse proxy
- [x] Phase 2 — Client registry (multi-tenancy)
- [ ] Phase 3 — Rate limiting
- [ ] Phase 4 — Metrics exposition
- [ ] Phase 5 — Per-client logging
- [ ] Phase 6 — Rule-based abuse detection
- [ ] Phase 7 — Traffic simulator
- [ ] Phase 8 — ML abuse scoring (optional layer)
- [ ] Phase 9 — Admin API + embedded dashboard UI
- [ ] Phase 10 — Hardening & packaging

---

## Phase 1 — Bare reverse proxy

**Build:** A Go service using Chi that accepts any request and forwards it to a single hardcoded target URL using `net/http/httputil.ReverseProxy`.

**Why first:** This is the skeleton everything else attaches to. Nothing else in the project works without a functioning forward path.

**Done when:** You can run a dummy backend locally, point Gargoyle at it, and see requests pass through untouched.

---

## Phase 2 — Client registry (multi-tenancy)

**Build:** Postgres `clients` table. API key extraction from incoming requests. Lookup logic that maps a key to a specific `target_url`. In-memory cache with TTL to avoid hitting Postgres on every request.

**Why here:** Nothing else (rate limits, logging, abuse detection) means anything without first knowing *which client* a request belongs to.

**Done when:** Two different dummy backends, two different API keys, and Gargoyle correctly routes each key's traffic to the right one.

---

## Phase 3 — Rate limiting

**Build:** Redis-backed sliding window rate limiter, per `client_id`, using each client's configured limit from Postgres. Return `429` when exceeded.

**Why here:** The simplest, most self-contained "real feature" — good checkpoint before layering in the harder abuse-detection logic.

**Done when:** You can script a burst of requests against one client's key and watch it get throttled, while a second client's key is unaffected.

---

## Phase 4 — Metrics exposition

**Build:** `/metrics` endpoint exposing Prometheus-format counters and histograms — total requests by outcome, request duration, active clients.

**Why here:** You want observability wired in early so every subsequent phase's behavior is visible, not just inferred from logs.

**Done when:** A local Prometheus instance can scrape Gargoyle and you can query `gargoyle_requests_total` and see it moving.

---

## Phase 5 — Per-client logging

**Build:** Postgres `request_logs` table. Write a row for every rate-limited request (full logging of "allowed" traffic comes later or is sampled, to avoid write overload).

**Why here:** This is the data source the future dashboard needs — better to have it flowing before you build anything that reads it.

**Done when:** Rate-limited requests show up as rows in Postgres with correct `client_id`, timestamp, and reason.

---

## Phase 6 — Rule-based abuse detection

**Build:** The heuristics layer — request sequencing/timing analysis, endpoint sweep detection, basic header/UA anomaly checks. Wire into the decision pipeline after the rate limit check.

**Why here:** This is the part that makes Gargoyle more than a rate limiter, and it's pure Go logic — no external ML dependency yet, so it's a good next step in complexity.

**Done when:** A scripted "scraping" pattern (many endpoints, uniform timing) against a test client gets flagged/blocked while normal randomized traffic doesn't.

---

## Phase 7 — Traffic simulator

**Build:** A separate small tool (can live outside the Go core, e.g. a script) that generates both normal traffic and labeled attack patterns against a running Gargoyle instance.

**Why here:** You need labeled traffic *before* you can train a model — this phase exists specifically to produce that dataset, and it doubles as your demo tool going forward.

**Done when:** Running the simulator produces a mix of clearly-labeled normal and attack traffic, and you can visually confirm Phase 6's rules react correctly to the attack portion.

---

## Phase 8 — ML abuse scoring (optional layer)

**Build:** (Python side happens in a separate track — see PYTHON.md, which can run in parallel with Phases 1–7) Once a trained model is exported to `abuse_model.onnx`, this phase is the Go-side integration: implement the `MLScorer` behind the same `AbuseScorer` interface as the rule layer (see PROJECT.md §6), load the ONNX file at startup, run inference in-process on each request, and combine its score with Phase 6's rule outcomes.

**Why here:** Deliberately placed after the rule-based layer and the simulator exist — you need both a baseline to compare against and labeled data to train on. This phase is additive, not a rework: Gargoyle already works fully on rules alone (Phase 6) before this point.

**If the `.onnx` file isn't ready yet:** skip this phase and move on — Gargoyle logs "ML scoring disabled, running rules-only" and continues working exactly as it did after Phase 6. Nothing downstream (dashboard, packaging) depends on this phase being done.

**Done when:** Gargoyle produces an `abuse_score` per request when a model is loaded, and correctly falls back to rules-only when it isn't.

---

## Phase 9 — Admin API + embedded dashboard UI

**Build:** A small internal API (`api/admin`) exposing client CRUD, usage stats, and recent blocked-request lists as JSON — reading from the data Phases 3–8 already produce. Then, in the same phase, add `web/` on top of it: `html/template` pages plus htmx-driven live updates, served directly from the Gargoyle binary. No separate frontend process or repo.

**Why here:** By this point there's enough real data flowing that building both the read layer and its UI on top of it is straightforward, rather than designing against data that doesn't exist yet. Doing the API and the UI in the same phase (instead of two) is only possible because the UI is a thin rendering layer over the same queries — there's no separate frontend build to coordinate.

**Done when:** You can hit the admin endpoints directly (curl/Postman) and get correct, client-scoped JSON — and separately, load `/dashboard` in a browser and watch it auto-refresh as the simulator (Phase 7) generates traffic.

---

## Phase 10 — Hardening & packaging

**Build:** Dockerfile for the Go core, Docker Compose wiring Gargoyle + Redis + Postgres + Prometheus together, basic test coverage on the rate limiter and abuse rules, config validation and graceful shutdown handling.

**Why last:** Packaging and hardening only matter once the behavior underneath is stable — polishing distribution before the logic works is wasted effort.

**Done when:** `docker compose up` brings up the entire stack from a clean checkout, and the simulator run from Phase 7 still produces correct results against the containerized version.

---

## Notes on scope

- Phases 1–6 are the non-negotiable core — this alone is already a legitimate, demoable project even without Phases 7–10.
- Phase 8 (ML) is optional and additive — it's what turns "a rate limiter with some if-statements" into something with a real machine learning component worth discussing in an interview, but Gargoyle is fully functional without it. Don't start the Go-side integration before Phase 7 gives you real data; the Python-side training work itself can start much earlier in parallel.
- Distributed/multi-node Gargoyle (running several instances behind a load balancer, sharing rate-limit state correctly) is intentionally not a numbered phase here — it's a stretch goal *after* Phase 10, only worth attempting once the single-node version is fully solid.
- Gargoyle ships as a single repo and a single binary — the embedded `html/template` + htmx dashboard from Phase 9 is the intended, final UI, not a placeholder. A separate polished frontend (e.g. Next.js) is an optional future project on top of the existing admin API, not part of this timeline, and should only be attempted after Phase 10 is complete and stable.
