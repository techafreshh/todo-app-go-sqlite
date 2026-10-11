package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := openDB(dbPath)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	srv := httptest.NewServer(newMux(&Handler{DB: db}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

func TestCreateAndListTodos(t *testing.T) {
	srv := newTestServer(t)

	payload := `{"title":"Pay rent","priority":"high","due_date":"2026-10-15"}`
	resp, err := http.Post(srv.URL+"/api/todos", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/todos: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var created Todo
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID != 1 {
		t.Errorf("id = %d, want 1", created.ID)
	}
	if created.Title != "Pay rent" {
		t.Errorf("title = %q, want Pay rent", created.Title)
	}
	if created.Done {
		t.Errorf("done = true, want false")
	}
	if created.Priority != "high" {
		t.Errorf("priority = %q, want high", created.Priority)
	}
	if created.DueDate == nil || *created.DueDate != "2026-10-15" {
		t.Errorf("due_date = %v, want 2026-10-15", created.DueDate)
	}

	listResp, err := http.Get(srv.URL + "/api/todos")
	if err != nil {
		t.Fatalf("GET /api/todos: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listResp.StatusCode)
	}
	var todos []Todo
	if err := json.NewDecoder(listResp.Body).Decode(&todos); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(todos) != 1 {
		t.Fatalf("len(todos) = %d, want 1", len(todos))
	}
	got := todos[0]
	if got.ID != 1 {
		t.Errorf("id = %d, want 1", got.ID)
	}
	if got.Title != "Pay rent" {
		t.Errorf("title = %q, want Pay rent", got.Title)
	}
	if got.Done {
		t.Errorf("done = true, want false")
	}
	if got.Priority != "high" {
		t.Errorf("priority = %q, want high", got.Priority)
	}
	if got.DueDate == nil || *got.DueDate != "2026-10-15" {
		t.Errorf("due_date = %v, want 2026-10-15", got.DueDate)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Errorf("timestamps = %q/%q, want non-empty", got.CreatedAt, got.UpdatedAt)
	}
}

func TestCreateTodoDefaults(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Post(srv.URL+"/api/todos", "application/json", strings.NewReader(`{"title":"Milk"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var created Todo
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Priority != "medium" {
		t.Errorf("priority = %q, want medium", created.Priority)
	}
	if created.DueDate != nil {
		t.Errorf("due_date = %v, want nil", created.DueDate)
	}

	listResp, err := http.Get(srv.URL + "/api/todos")
	if err != nil {
		t.Fatalf("GET /api/todos: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listResp.StatusCode)
	}
	var todos []Todo
	if err := json.NewDecoder(listResp.Body).Decode(&todos); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(todos) != 1 {
		t.Fatalf("len(todos) = %d, want 1", len(todos))
	}
	if todos[0].Priority != "medium" {
		t.Errorf("listed priority = %q, want medium", todos[0].Priority)
	}
	if todos[0].DueDate != nil {
		t.Errorf("listed due_date = %v, want null", todos[0].DueDate)
	}
}

func TestCreateTodoValidation(t *testing.T) {
	srv := newTestServer(t)
	cases := []struct {
		name string
		body string
	}{
		{"empty title", `{"title":"   "}`},
		{"long title", `{"title":"` + strings.Repeat("x", 256) + `"}`},
		{"bad priority", `{"title":"x","priority":"urgent"}`},
		{"bad date", `{"title":"x","due_date":"10/15/2026"}`},
		{"invalid date", `{"title":"x","due_date":"2026-13-40"}`},
		{"malformed json", `{"title":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/api/todos", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if body.Error == "" {
				t.Errorf("error message is empty")
			}
		})
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist.db")

	db, err := openDB(dbPath)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	if _, err := createTodo(db, "Persist me", "low", nil); err != nil {
		t.Fatalf("createTodo: %v", err)
	}
	db.Close()

	reopened, err := openDB(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	todos, err := listTodos(reopened)
	if err != nil {
		t.Fatalf("listTodos: %v", err)
	}
	if len(todos) != 1 || todos[0].Title != "Persist me" {
		t.Fatalf("todos = %+v, want one row 'Persist me'", todos)
	}
}

func TestIndexServesHTML(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}
