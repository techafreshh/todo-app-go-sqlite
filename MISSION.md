# Mission

Give an individual a fast, reliable, locally-run todo list that just works
anywhere Go or Docker runs.

## What this project is

- Single-user, local-first todo web app: Go stdlib backend, SQLite persistence,
  single-file vanilla-JS frontend.
- Zero-ops: one binary + one SQLite file + `docker compose up --build` →
  http://localhost:8080.

## Invariants (never break these)

1. Simplicity over features: stdlib `net/http`, no frameworks, no frontend
   build toolchain.
2. Local-first persistence: the SQLite file is the source of truth; data
   survives restarts.
3. Portability: same binary via `go run` and via the distroless Docker image;
   `CGO_ENABLED=0` throughout.
4. Clarity: single `static/index.html`, thin HTTP handlers, explicit schema,
   parameterized SQL.

## Out of scope (do not build)

Multi-user / auth / accounts; tags, subtasks, recurring tasks, reminders;
mobile apps or PWA sync; Postgres/MySQL or Kubernetes; full-text ranking;
collaboration or sharing.

## Sourcing

Derived from `.agents/PRDs/PRD.md`, which is authoritative. If this file and
the PRD disagree, the PRD wins — propose an amendment instead of drifting.
