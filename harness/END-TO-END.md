# End-to-end journeys

Plain-English journeys the running app must satisfy. They are encoded as
machine-verified scenarios for `archon-verify-runtime`, maintained **outside
this repository** (factory host path, alongside the private holdout set) so the
implementing agent can never tune code against the verification — same
discipline as the holdout: journeys and scenarios describe *what must be true*,
not how to pass them.

1. **Health** — the app answers `GET /healthz` with 200 and a readiness body.
2. **First todo** — I open the app and see the UI; I add a todo; it appears in
   the list.
3. **API round-trip** — I `POST /api/todos`; the response echoes the todo with
   an id; `GET /api/todos` contains it.
4. **Persistence** — I add a todo, restart the app, and the todo is still
   there (SQLite file via `DB_PATH`).

Additional edge-case journeys (malformed input, boundary values, restart
abuse) exist as **private holdout scenarios** — deliberately outside this
repository so the implementing agent cannot tune against them.
