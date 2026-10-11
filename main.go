package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	port := envOr("PORT", "8080")
	dbPath := envOr("DB_PATH", "./todos.db")

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

func newMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("POST /api/todos", h.createTodo)
	mux.HandleFunc("GET /api/todos", h.listTodos)
	mux.HandleFunc("PUT /api/todos/{id}", h.updateTodo)
	mux.HandleFunc("PATCH /api/todos/{id}/toggle", h.toggleTodo)
	mux.HandleFunc("DELETE /api/todos/{id}", h.deleteTodo)
	mux.HandleFunc("GET /{$}", h.index)
	return mux
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/index.html")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
