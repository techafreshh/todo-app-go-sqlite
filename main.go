package main

import (
	"log"
	"net/http"
	"os"
)

// The container image re-declares the process's runtime interface as its own
// literals; docker_test.go binds the Dockerfile to these owners so a rename on
// either side fails the build instead of silently falling back to a default.
const (
	envPort   = "PORT"
	envDBPath = "DB_PATH"

	// defaultPort is the listen port when PORT is unset or empty; the compose
	// test derives the effective port from it rather than assuming a literal.
	defaultPort = "8080"

	// defaultDBPath is where the database lands when DB_PATH is unset; the
	// README guard test binds its documented value to this constant.
	defaultDBPath = "./todos.db"
)

// indexFile is served relative to the process working directory, which the
// image sets and populates.
const indexFile = "static/index.html"

func main() {
	port := envOr(envPort, defaultPort)
	dbPath := envOr(envDBPath, defaultDBPath)

	db, err := openDB(dbPath)
	if err != nil {
		log.Fatalf("open database %s: %v", dbPath, err)
	}
	defer db.Close()

	h := &Handler{DB: db}
	mux := newMux(h)

	addr := ":" + port
	log.Printf("listening on %s (db %s)", addr, dbPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// The declared HTTP surface, each pattern "METHOD /path". newMux registers from
// these and the README guard test binds the documented route table to them, so a
// route renamed in one place and not the other fails a check.
const (
	routeHealthz    = "GET /healthz"
	routeCreateTodo = "POST /api/todos"
	routeListTodos  = "GET /api/todos"

	// routeUpdateTodo owns the edit wire contract. The served page re-declares
	// the same verb and path in its client fetch, and the conformance test binds
	// the page to this constant.
	routeUpdateTodo = "PUT /api/todos/{id}"

	routeToggleTodo = "PATCH /api/todos/{id}/toggle"
	routeDeleteTodo = "DELETE /api/todos/{id}"
	routeIndex      = "GET /{$}"
)

func newMux(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(routeHealthz, h.healthz)
	mux.HandleFunc(routeCreateTodo, h.createTodo)
	mux.HandleFunc(routeListTodos, h.listTodos)
	mux.HandleFunc(routeUpdateTodo, h.updateTodo)
	mux.HandleFunc(routeToggleTodo, h.toggleTodo)
	mux.HandleFunc(routeDeleteTodo, h.deleteTodo)
	mux.HandleFunc(routeIndex, h.index)
	return jsonErrorResponses(mux)
}

// jsonErrorResponses rewrites the plain-text 404 and 405 bodies ServeMux writes
// for an unmatched route or a wrong method into the documented JSON error shape,
// preserving the status and the Allow header it sets. ServeMux emits those
// responses itself, before any handler runs, so the fallback has to sit outside
// it; handler responses set their own Content-Type and pass through untouched.
func jsonErrorResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&jsonErrorWriter{ResponseWriter: w}, r)
	})
}

type jsonErrorWriter struct {
	http.ResponseWriter
	replaced bool
}

func (w *jsonErrorWriter) WriteHeader(status int) {
	if !w.replaced && w.Header().Get("Content-Type") == "text/plain; charset=utf-8" &&
		(status == http.StatusNotFound || status == http.StatusMethodNotAllowed) {
		w.replaced = true
		writeError(w.ResponseWriter, status, http.StatusText(status))
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *jsonErrorWriter) Write(b []byte) (int, error) {
	if w.replaced {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, indexFile)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
