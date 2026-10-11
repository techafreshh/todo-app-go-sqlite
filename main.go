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
)

// indexFile is served relative to the process working directory, which the
// image sets and populates.
const indexFile = "static/index.html"

func main() {
	port := envOr(envPort, "8080")
	dbPath := envOr(envDBPath, "./todos.db")

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

// routeUpdateTodo owns the edit wire contract. The served page re-declares the
// same verb and path in its client fetch, and the conformance test binds the
// page to this constant.
const routeUpdateTodo = "PUT /api/todos/{id}"

func newMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("POST /api/todos", h.createTodo)
	mux.HandleFunc("GET /api/todos", h.listTodos)
	mux.HandleFunc(routeUpdateTodo, h.updateTodo)
	mux.HandleFunc("PATCH /api/todos/{id}/toggle", h.toggleTodo)
	mux.HandleFunc("DELETE /api/todos/{id}", h.deleteTodo)
	mux.HandleFunc("GET /{$}", h.index)
	return mux
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
