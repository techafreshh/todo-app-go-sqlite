# Todo App (Go + SQLite)

A single-user, local-first todo web app: a Go standard-library HTTP server, a
pure-Go SQLite database, and a single-page vanilla-JS frontend. No accounts, no
external database, no frontend build step.

Once running, open <http://localhost:8080> to add, edit, complete, filter,
search, and prioritize todos.

## Requirements

Either one is enough:

- **Go** 1.26 or newer (`go version`), or
- **Docker** with Compose v2 (`docker compose version`).

## Run with Go

From the repository root:

```sh
go run .
```

The server listens on `:8080` by default and stores data in `./todos.db`. Open
<http://localhost:8080>.

Configuration is read from the environment:

```sh
PORT=9090 DB_PATH=/tmp/todos.db go run .
```

## Run with Docker

From the repository root:

```sh
docker compose up --build
```

Open <http://localhost:8080>. The database lives in the named `todo-data`
volume at `/data/todos.db`, so your todos survive `docker compose down` and
come back on the next `up`.

To stop the service:

```sh
docker compose down
```

## Configuration

| Variable  | Default (`go run`) | Default (Docker)   | Purpose          |
|-----------|--------------------|--------------------|------------------|
| `PORT`    | `8080`             | `8080`             | listen port      |
| `DB_PATH` | `./todos.db`       | `/data/todos.db`   | SQLite file path |

The server binds to `0.0.0.0:$PORT` for Docker. It has no authentication, so do
not expose it publicly without a reverse proxy.

## API

All responses are JSON. Errors use `{"error":"..."}` with `400` for validation
failures and `404` for unknown ids.

| Method   | Path                      | Purpose                          |
|----------|---------------------------|----------------------------------|
| `GET`    | `/`                       | single-page UI                   |
| `GET`    | `/healthz`                | readiness check                  |
| `GET`    | `/api/todos`              | list, with `status`, `priority`, `q`, `sort` filters |
| `POST`   | `/api/todos`              | create a todo                    |
| `PUT`    | `/api/todos/{id}`         | update a todo                    |
| `PATCH`  | `/api/todos/{id}/toggle`  | toggle done                      |
| `DELETE` | `/api/todos/{id}`         | delete a todo                    |

## Development checks

```sh
go vet ./...
go test ./...
```
