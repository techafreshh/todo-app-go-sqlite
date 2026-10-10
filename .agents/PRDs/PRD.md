# PRD: Simple Todo Web App (Go + SQLite + Docker)

## 1. Executive Summary

A single-user, local-first todo web app with a Go backend, SQLite persistence, and a single-page vanilla-JS frontend. The product lets one user add, edit, delete, complete, filter, search, and prioritize todos with optional due dates.

Core value proposition: **zero-ops simplicity** — one binary + one SQLite file + one Docker command (`docker compose up --build` → `http://localhost:8080`). No auth, no frontend build toolchain, no external database.

MVP goal: working local app + Docker image with persistent storage, covering full CRUD + due dates/priority + filters/search, verified by manual browser test and API `curl` checks.

## 2. Mission

**Mission:** Give an individual a fast, reliable, locally-run todo list that just works anywhere Go or Docker runs.

Core principles:

1. **Simplicity over features** — stdlib only, no SPA framework.
2. **Local-first persistence** — SQLite file is source of truth, survives restarts.
3. **Portability** — same binary runs via `go run` and via distroless Docker image.
4. **Clarity** — single `index.html`, readable handlers, explicit schema.
5. **Reproducibility** — one-command Docker run with named volume.

## 3. Target Users

Primary persona: **Solo developer / power user** running the app locally for personal task tracking.

- Technical comfort: comfortable with `go run` or `docker compose up`, editing env vars.
- Key needs: quick capture, mark done, find by text, see what's overdue, prioritize.
- Pain points: heavyweight SaaS todo tools requiring accounts; apps that lose data on restart; complex JS build setups.

No secondary personas for MVP. Multi-user is explicitly out of scope.

## 4. MVP Scope

### In Scope

**Core Functionality:**

- [ ] Add todo with title (required)
- [ ] Edit title, priority, due date
- [ ] Delete todo
- [ ] Toggle done / active
- [ ] Priority `low|medium|high` with default `medium`
- [ ] Due date `YYYY-MM-DD` optional + overdue highlight
- [ ] Filters: All / Active / Done + priority filter + text search
- [ ] Sort: by due date / created / priority

**Technical:**

- [ ] Go `net/http` server, `database/sql` + pure-Go SQLite
- [ ] Auto-migration on startup
- [ ] `GET /` serves `static/index.html`, `GET /healthz`
- [ ] JSON API under `/api/todos`

**Deployment:**

- [ ] Multi-stage `Dockerfile` (builder `golang:1.26-bookworm` → `gcr.io/distroless/static-debian12`)
- [ ] `compose.yaml` with `8080:8080`, `DB_PATH=/data/todos.db`, named volume `todo-data`
- [ ] `PORT` + `DB_PATH` env config
- [ ] `.dockerignore`, `.gitignore` (`todos.db`, `data/`)

### Out of Scope

- [ ] Multi-user accounts / auth
- [ ] Tags, subtasks, recurring tasks, reminders/notifications
- [ ] Mobile native apps, offline PWA sync
- [ ] Postgres/MySQL support, Kubernetes manifests
- [ ] Full-text ranking, collaboration/sharing

## 5. User Stories

1. **As a user, I want to add a todo with title, priority and optional due date, so that I capture work quickly.**
   Example: `POST /api/todos {"title":"Pay rent","priority":"high","due_date":"2026-10-15"}` → appears at top of list.

2. **As a user, I want to mark a todo done/undone with one click, so that I track progress.**
   Example: checkbox → `PATCH /api/todos/3/toggle` → strikethrough + moves to Done filter.

3. **As a user, I want to edit title/priority/due date inline, so that I fix mistakes without deleting.**
   Example: edit `Buy milk` → `Buy oat milk`, change `low` → `medium`.

4. **As a user, I want to delete a todo, so that I remove clutter.**
   Example: delete button → `DELETE /api/todos/5` → row disappears.

5. **As a user, I want to filter by All/Active/Done and priority + search by text, so that I find tasks.**
   Example: `GET /api/todos?status=active&priority=high&q=rent&sort=due`.

6. **As a user, I want to see overdue todos highlighted and sorted by due date, so that I focus on what's urgent.**
   Example: due `2026-10-01` past today + not done → red badge.

7. **As a user, I want my todos to persist across restarts, so that I don't lose data.**
   Example: add 3 todos → `docker compose down` → `up` → same 3 present via volume `todo-data:/data`.

8. **Technical: As an operator, I want `PORT`/`DB_PATH` env + `/healthz`, so that Docker HEALTHCHECK and port mapping work.**
   Example: `PORT=8080 DB_PATH=/data/todos.db ./server`, `curl localhost:8080/healthz → 200 OK`.

## 6. Core Architecture & Patterns

High-level: monolith, server-served static file + JSON API. `main.go` wires DB + routes. `db.go` owns SQL. `handlers.go` is thin HTTP → DB. No ORM.

Directory structure:

```text
todo-app/
├── go.mod
├── main.go           # env, listen, routes /, /healthz, /api/
├── db.go             # open, migrate, CRUD queries
├── handlers.go       # JSON handlers + validation
├── static/
│   └── index.html    # single-page UI
├── Dockerfile        # multi-stage build
├── compose.yaml      # one-command run + persistent volume
├── .dockerignore
└── .gitignore        # todos.db, data/
```

Key design patterns and principles:

- Standard-library `net/http` mux, no framework.
- Dependency injection (`Handler{DB *sql.DB}`).
- Migration-on-boot (`CREATE TABLE IF NOT EXISTS`).
- Env-based twelve-factor config (`PORT`, `DB_PATH`).
- Volume-for-state in Docker (`/data`).
- Parameterized SQL, thin handlers, validation at boundary.

## 7. Tools/Features

Detailed feature specifications:

- **Todo form:** title input (required, maxlength 255), priority select (`low|medium|high`), date input (`<input type=date>`), Add button, client-side validation.
- **Filter bar:** segmented All/Active/Done, search input (debounced ~200ms), priority dropdown, sort dropdown (`due|created|priority`).
- **List rows:** checkbox toggle, title (strikethrough if done), priority badge (color-coded), due label (`Overdue` red, `Today` amber), edit affordance, delete button.
- **Empty/error states:** "No todos match" empty state + fetch error banner.
- **Persistence:** SQLite file, `updated_at` auto-touched on write, `busy_timeout` for safety.

## 8. Technology Stack

- **Backend:** Go `1.26.0`, stdlib `net/http`, `database/sql`, `encoding/json`
- **DB driver:** `modernc.org/sqlite` (pure Go, `CGO_ENABLED=0`) — avoids C toolchain; alternative `mattn/go-sqlite3` rejected for MVP (requires CGO + larger image)
- **Frontend:** single `static/index.html`, vanilla JS `fetch`, inline CSS, no npm/build
- **Docker builder:** `golang:1.26-bookworm`
- **Docker runtime:** `gcr.io/distroless/static-debian12` (confirmed), `CGO_ENABLED=0`, `PORT=8080`, `DB_PATH=/data/todos.db`, `EXPOSE 8080`, `VOLUME /data`
- **Compose:** Compose v2 `compose.yaml`, service `todo`, `ports: ["8080:8080"]`, volume `todo-data:/data`, `restart: unless-stopped`

Optional dependencies: none for MVP.

Third-party integrations: none.

## 9. Security & Configuration

Authentication/authorization: none (single-user local, explicitly out of scope). Binds to `0.0.0.0:$PORT` for Docker; document not to expose publicly without reverse proxy.

Configuration management:

| Var | Default (local) | Default (Docker) | Purpose |
|-----|-----------------|------------------|---------|
| `PORT` | `8080` | `8080` | listen port |
| `DB_PATH` | `./todos.db` | `/data/todos.db` | SQLite file path |

Security in-scope: input validation (empty title → 400, bad priority/date → 400), `Content-Type: application/json`, parameterized SQL queries, `.dockerignore` hygiene.

Security out-of-scope: authn/z, rate-limiting, TLS, encryption at rest.

Deployment considerations: Docker Desktop with WSL integration required on Windows/WSL hosts (docker CLI not present in current dev distro); local `go run` path must work without Docker.

## 10. API Specification

Base `http://localhost:8080`. All JSON, no auth.

### `GET /healthz`

Response `200`:

```json
{"status":"ok"}
```

### `GET /api/todos?status=all|active|done&q=&priority=low|medium|high&sort=due|created|priority`

Response `200`:

```json
[
  {
    "id": 1,
    "title": "Pay rent",
    "done": false,
    "priority": "high",
    "due_date": "2026-10-15",
    "created_at": "2026-10-10T19:00:00Z",
    "updated_at": "2026-10-10T19:00:00Z"
  }
]
```

### `POST /api/todos`

Request:

```json
{"title":"Pay rent","priority":"high","due_date":"2026-10-15"}
```

Response `201` with created object. `due_date` may be `null`/omitted.

### `PUT /api/todos/{id}`

Request:

```json
{"title":"Pay rent early","priority":"medium","due_date":"2026-10-14","done":true}
```

Response `200` with updated object.

### `PATCH /api/todos/{id}/toggle`

Response `200` with toggled object.

### `DELETE /api/todos/{id}`

Response `204` empty.

Errors:

```json
{"error":"title is required"}
```

- `400` validation error
- `404` not found

`GET /` serves `static/index.html`.

## 11. Success Criteria

MVP success definition: `docker compose up --build` serves UI on `:8080`, CRUD + filters work, data survives `down/up`.

Functional requirements:

- [ ] Add/edit/toggle/delete round-trip via UI + curl
- [ ] Filters + search + sort correct
- [ ] Overdue highlight correct
- [ ] Restart persistence (local file + Docker volume)
- [ ] `/healthz` returns 200, Dockerfile builds, compose starts

Quality indicators: `go vet` clean, startup <2s, p95 list <100ms local with <1000 rows, image <30MB (distroless goal).

User experience goals: no JS build step, works with JS enabled, responsive ≥360px width, Enter key adds todo, one-click toggle.

## 12. Implementation Phases

### Phase 1 — Backend + DB (0.5 day)

Goal: runnable JSON API with persistence.

Deliverables:

- [ ] `go mod init` + add `modernc.org/sqlite`
- [ ] `db.go` open/migrate + CRUD SQL
- [ ] `handlers.go` + routes + validation + `/healthz`
- [ ] `PORT`/`DB_PATH` env handling

Validation: `go run .` + `curl POST/GET/PUT/DELETE` passes, restart retains rows.

### Phase 2 — Single-page UI (0.5 day)

Goal: usable `index.html`.

Deliverables:

- [ ] Form + list + filter/search/sort in vanilla JS
- [ ] Toggle/edit/delete wiring, overdue styling
- [ ] Empty + error states

Validation: manual browser add/edit/filter/search/toggle/delete works.

### Phase 3 — Docker packaging (0.25 day)

Goal: portable image + compose.

Deliverables:

- [ ] Multi-stage `Dockerfile`, `.dockerignore`, `.gitignore`
- [ ] `compose.yaml` with `todo-data:/data`

Validation (on Docker host): `docker build -t todo-app .`, `docker compose up`, add → down/up → data persists, `/healthz` healthy.

### Phase 4 — Hardening + docs (0.25 day)

Goal: shippable.

Deliverables:

- [ ] `go vet`, error handling polish, README run instructions

Validation: clean `vet`, README steps reproduce from empty clone.

## 13. Future Considerations

- Multi-user + SQLite per-user or `user_id` column + session auth
- Tags, full-text search (`FTS5`), subtasks, recurring tasks
- Due reminders, dark mode, drag-reorder, keyboard shortcuts
- Export/import JSON/CSV
- Postgres option, Kubernetes manifests, CI publishing to GHCR

## 14. Risks & Mitigations

1. **modernc.org/sqlite version drift / heavy build** — Mitigation: pin version in `go.mod`, use `go mod tidy`, cache `go mod download` in builder; fallback to `mattn/go-sqlite3` + `debian-slim` if needed.
2. **Docker unavailable in dev (current WSL)** — Mitigation: develop/verify via `go run` here; document Docker verify for host with Docker Desktop + WSL integration enabled.
3. **SQLite locking on concurrent writes** — Mitigation: single-user scope + `database/sql` pool=1, `busy_timeout=5000`, short transactions, WAL mode consideration.
4. **Date handling bugs (empty vs null, timezone)** — Mitigation: store `YYYY-MM-DD` text only, validate `^\d{4}-\d{2}-\d{2}$`, allow empty → NULL; client uses `<input type=date>`.
5. **Volume data loss (bind vs named confusion)** — Mitigation: default named volume `todo-data`, document `docker volume ls/inspect`, never store DB in image layer.

## 15. Appendix

- Repo: `/home/techafresh/projects/todo-app` (empty at PRD time)
- Env verified: `go1.26.0 linux/amd64`, no `sqlite3` CLI, no `docker` CLI in WSL distro
- Key deps: `modernc.org/sqlite` docs, `golang:1.26-bookworm` + `gcr.io/distroless/static-debian12` on Docker Hub
- Related docs: README (to be added Phase 4), `compose.yaml` spec
- Decisions confirmed: distroless/static base OK, keep `8080:8080`, keep `todo-data` volume and other defaults
