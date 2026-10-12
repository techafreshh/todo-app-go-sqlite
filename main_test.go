package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
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

// The mux emits its own plain-text 404/405 for a route or method it does not
// declare; the server must rewrite those into the documented JSON error shape
// without disturbing the declared routes (issue #10: all API error responses are
// JSON with the correct status).
func TestUnmatchedRouteAndMethodReturnJSONErrors(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantAllow  []string
	}{
		{"unknown route", http.MethodGet, "/api/nope", http.StatusNotFound, nil},
		{"wrong method on collection", http.MethodDelete, "/api/todos", http.StatusMethodNotAllowed, []string{"GET", "POST"}},
		{"wrong method on item", http.MethodGet, "/api/todos/1", http.StatusMethodNotAllowed, []string{"PUT", "DELETE"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doRequest(t, tc.method, srv.URL+tc.path, "")
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			for _, method := range tc.wantAllow {
				if allow := resp.Header.Get("Allow"); !strings.Contains(allow, method) {
					t.Errorf("Allow = %q, want it to include %s", allow, method)
				}
			}
			decodeError(t, resp)
		})
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
	todos, err := listTodos(reopened, listFilter{})
	if err != nil {
		t.Fatalf("listTodos: %v", err)
	}
	if len(todos) != 1 || todos[0].Title != "Persist me" {
		t.Fatalf("todos = %+v, want one row 'Persist me'", todos)
	}
}

func doRequest(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func createTestTodo(t *testing.T, srv *httptest.Server, payload string) Todo {
	t.Helper()
	resp := doRequest(t, http.MethodPost, srv.URL+"/api/todos", payload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created Todo
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	return created
}

func decodeError(t *testing.T, resp *http.Response) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error == "" {
		t.Errorf("error message is empty")
	}
	return body.Error
}

func assertLater(t *testing.T, earlier, later string) {
	t.Helper()
	e, err := time.Parse(time.RFC3339, earlier)
	if err != nil {
		t.Fatalf("parse earlier timestamp %q: %v", earlier, err)
	}
	l, err := time.Parse(time.RFC3339, later)
	if err != nil {
		t.Fatalf("parse later timestamp %q: %v", later, err)
	}
	if !l.After(e) {
		t.Errorf("updated_at %q is not later than created_at %q", later, earlier)
	}
}

func TestUpdateTodo(t *testing.T) {
	srv := newTestServer(t)
	created := createTestTodo(t, srv, `{"title":"Pay rent","priority":"high","due_date":"2026-10-15"}`)

	resp := doRequest(t, http.MethodPut, srv.URL+"/api/todos/1",
		`{"title":"Pay rent early","priority":"medium","due_date":"2026-10-14","done":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", resp.StatusCode)
	}
	var updated Todo
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated: %v", err)
	}
	if updated.ID != 1 {
		t.Errorf("id = %d, want 1", updated.ID)
	}
	if updated.Title != "Pay rent early" {
		t.Errorf("title = %q, want Pay rent early", updated.Title)
	}
	if updated.Priority != "medium" {
		t.Errorf("priority = %q, want medium", updated.Priority)
	}
	if updated.DueDate == nil || *updated.DueDate != "2026-10-14" {
		t.Errorf("due_date = %v, want 2026-10-14", updated.DueDate)
	}
	if !updated.Done {
		t.Errorf("done = false, want true")
	}
	if updated.CreatedAt != created.CreatedAt {
		t.Errorf("created_at = %q, want unchanged %q", updated.CreatedAt, created.CreatedAt)
	}
	assertLater(t, created.CreatedAt, updated.UpdatedAt)

	listResp := doRequest(t, http.MethodGet, srv.URL+"/api/todos", "")
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listResp.StatusCode)
	}
	var todos []Todo
	if err := json.NewDecoder(listResp.Body).Decode(&todos); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(todos) != 1 || todos[0].Title != "Pay rent early" || !todos[0].Done {
		t.Fatalf("todos = %+v, want one updated done row", todos)
	}
}

func TestUpdateTodoValidation(t *testing.T) {
	srv := newTestServer(t)
	createTestTodo(t, srv, `{"title":"Pay rent"}`)
	cases := []struct {
		name string
		body string
	}{
		{"empty title", `{"title":"   ","priority":"medium","done":true}`},
		{"long title", `{"title":"` + strings.Repeat("x", 256) + `"}`},
		{"bad priority", `{"title":"x","priority":"urgent"}`},
		{"bad date", `{"title":"x","due_date":"10/15/2026"}`},
		{"malformed json", `{"title":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doRequest(t, http.MethodPut, srv.URL+"/api/todos/1", tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			decodeError(t, resp)
		})
	}
}

func TestUpdateTodoNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/api/todos/999",
		`{"title":"x","priority":"low","done":true}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	decodeError(t, resp)
}

func TestToggleTodo(t *testing.T) {
	srv := newTestServer(t)
	created := createTestTodo(t, srv, `{"title":"Pay rent","priority":"high"}`)

	resp := doRequest(t, http.MethodPatch, srv.URL+"/api/todos/1/toggle", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("toggle status = %d, want 200", resp.StatusCode)
	}
	var toggled Todo
	if err := json.NewDecoder(resp.Body).Decode(&toggled); err != nil {
		t.Fatalf("decode toggled: %v", err)
	}
	if !toggled.Done {
		t.Errorf("done = false, want true")
	}
	assertLater(t, created.CreatedAt, toggled.UpdatedAt)

	back := doRequest(t, http.MethodPatch, srv.URL+"/api/todos/1/toggle", "")
	if back.StatusCode != http.StatusOK {
		t.Fatalf("second toggle status = %d, want 200", back.StatusCode)
	}
	var untoggled Todo
	if err := json.NewDecoder(back.Body).Decode(&untoggled); err != nil {
		t.Fatalf("decode untoggled: %v", err)
	}
	if untoggled.Done {
		t.Errorf("done = true, want false")
	}
	assertLater(t, toggled.UpdatedAt, untoggled.UpdatedAt)
}

func TestToggleTodoNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doRequest(t, http.MethodPatch, srv.URL+"/api/todos/999/toggle", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	decodeError(t, resp)
}

func TestDeleteTodo(t *testing.T) {
	srv := newTestServer(t)
	createTestTodo(t, srv, `{"title":"Pay rent"}`)

	resp := doRequest(t, http.MethodDelete, srv.URL+"/api/todos/1", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("delete body = %q, want empty", body)
	}

	listResp := doRequest(t, http.MethodGet, srv.URL+"/api/todos", "")
	var todos []Todo
	if err := json.NewDecoder(listResp.Body).Decode(&todos); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(todos) != 0 {
		t.Fatalf("todos = %+v, want empty", todos)
	}

	again := doRequest(t, http.MethodDelete, srv.URL+"/api/todos/1", "")
	if again.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", again.StatusCode)
	}
	decodeError(t, again)
}

func TestDeleteTodoNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := doRequest(t, http.MethodDelete, srv.URL+"/api/todos/999", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	decodeError(t, resp)
}

func TestWriteRowIsolation(t *testing.T) {
	verbs := []struct {
		name    string
		request func(t *testing.T, srv *httptest.Server)
	}{
		{
			name: "update",
			request: func(t *testing.T, srv *httptest.Server) {
				resp := doRequest(t, http.MethodPut, srv.URL+"/api/todos/1",
					`{"title":"changed","priority":"high","done":true}`)
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("PUT status = %d, want 200", resp.StatusCode)
				}
			},
		},
		{
			name: "toggle",
			request: func(t *testing.T, srv *httptest.Server) {
				resp := doRequest(t, http.MethodPatch, srv.URL+"/api/todos/1/toggle", "")
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("PATCH status = %d, want 200", resp.StatusCode)
				}
			},
		},
		{
			name: "delete",
			request: func(t *testing.T, srv *httptest.Server) {
				resp := doRequest(t, http.MethodDelete, srv.URL+"/api/todos/1", "")
				if resp.StatusCode != http.StatusNoContent {
					t.Fatalf("DELETE status = %d, want 204", resp.StatusCode)
				}
			},
		},
	}

	for _, v := range verbs {
		t.Run(v.name, func(t *testing.T) {
			srv := newTestServer(t)
			createTestTodo(t, srv, `{"title":"first","priority":"high","due_date":"2026-10-15"}`)
			createTestTodo(t, srv, `{"title":"second","priority":"low","due_date":"2026-10-20"}`)

			v.request(t, srv)

			listResp := doRequest(t, http.MethodGet, srv.URL+"/api/todos", "")
			var todos []Todo
			if err := json.NewDecoder(listResp.Body).Decode(&todos); err != nil {
				t.Fatalf("decode list: %v", err)
			}
			var survivor *Todo
			for i := range todos {
				if todos[i].ID == 2 {
					survivor = &todos[i]
				}
			}
			if survivor == nil {
				t.Fatalf("todos = %+v, want id 2 to survive", todos)
			}
			if survivor.Title != "second" || survivor.Priority != "low" || survivor.Done {
				t.Errorf("row 2 = %+v, want original fields", survivor)
			}
			if survivor.DueDate == nil || *survivor.DueDate != "2026-10-20" {
				t.Errorf("row 2 due_date = %v, want 2026-10-20", survivor.DueDate)
			}
		})
	}
}

func TestToggleTodoConcurrent(t *testing.T) {
	srv := newTestServer(t)
	createTestTodo(t, srv, `{"title":"race"}`)

	const toggles = 8
	statuses := make(chan int, toggles)
	var wg sync.WaitGroup
	for i := 0; i < toggles; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodPatch, srv.URL+"/api/todos/1/toggle", nil)
			if err != nil {
				statuses <- -1
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				statuses <- -1
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			statuses <- resp.StatusCode
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("toggle status = %d, want 200", status)
		}
	}

	listResp := doRequest(t, http.MethodGet, srv.URL+"/api/todos", "")
	var todos []Todo
	if err := json.NewDecoder(listResp.Body).Decode(&todos); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(todos) != 1 {
		t.Fatalf("len(todos) = %d, want 1", len(todos))
	}
	if todos[0].Done {
		t.Errorf("done = true after %d concurrent toggles, want false", toggles)
	}
}

func listQuery(t *testing.T, srv *httptest.Server, query string) []Todo {
	t.Helper()
	resp := doRequest(t, http.MethodGet, srv.URL+"/api/todos"+query, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/todos%s status = %d, want 200", query, resp.StatusCode)
	}
	var todos []Todo
	if err := json.NewDecoder(resp.Body).Decode(&todos); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return todos
}

func todoTitles(todos []Todo) []string {
	titles := make([]string, len(todos))
	for i, todo := range todos {
		titles[i] = todo.Title
	}
	return titles
}

func seedListFixture(t *testing.T, srv *httptest.Server) {
	t.Helper()
	createTestTodo(t, srv, `{"title":"Pay rent","priority":"high","due_date":"2026-10-15"}`)
	createTestTodo(t, srv, `{"title":"Buy milk","priority":"low"}`)
	createTestTodo(t, srv, `{"title":"Old task","priority":"medium","due_date":"2026-10-01"}`)
	resp := doRequest(t, http.MethodPatch, srv.URL+"/api/todos/3/toggle", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("toggle status = %d, want 200", resp.StatusCode)
	}
}

func TestListTodosFilterAndSearch(t *testing.T) {
	srv := newTestServer(t)
	seedListFixture(t, srv)

	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"default returns all in created order", "", []string{"Pay rent", "Buy milk", "Old task"}},
		{"status all", "?status=all", []string{"Pay rent", "Buy milk", "Old task"}},
		{"status active", "?status=active", []string{"Pay rent", "Buy milk"}},
		{"status done", "?status=done", []string{"Old task"}},
		{"priority high", "?priority=high", []string{"Pay rent"}},
		{"search is case-insensitive", "?q=RENT", []string{"Pay rent"}},
		{"search has no match", "?q=zzz", []string{}},
		{"filters combine with AND", "?status=active&priority=high&q=rent&sort=due", []string{"Pay rent"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := todoTitles(listQuery(t, srv, tc.query))
			if !slices.Equal(got, tc.want) {
				t.Errorf("titles = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestListTodosSort(t *testing.T) {
	srv := newTestServer(t)
	seedListFixture(t, srv)

	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"due ascending with null last", "?sort=due", []string{"Old task", "Pay rent", "Buy milk"}},
		{"created ascending", "?sort=created", []string{"Pay rent", "Buy milk", "Old task"}},
		{"priority high to low", "?sort=priority", []string{"Pay rent", "Old task", "Buy milk"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := todoTitles(listQuery(t, srv, tc.query))
			if !slices.Equal(got, tc.want) {
				t.Errorf("titles = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPriorityDomainConformance(t *testing.T) {
	t.Run("default is a member of the set", func(t *testing.T) {
		if !validPriorities[defaultPriority] {
			t.Errorf("defaultPriority %q is not in validPriorities", defaultPriority)
		}
	})

	t.Run("sqlite schema default conforms", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "schema.db")
		db, err := openDB(dbPath)
		if err != nil {
			t.Fatalf("openDB: %v", err)
		}
		defer db.Close()

		now := nowTimestamp()
		if _, err := db.Exec(
			`INSERT INTO todos (title, created_at, updated_at) VALUES (?, ?, ?)`,
			"schema default", now, now,
		); err != nil {
			t.Fatalf("insert relying on schema default: %v", err)
		}
		var priority string
		if err := db.QueryRow(`SELECT priority FROM todos WHERE title = ?`, "schema default").Scan(&priority); err != nil {
			t.Fatalf("read back priority: %v", err)
		}
		if priority != defaultPriority {
			t.Errorf("schema default = %q, want defaultPriority %q", priority, defaultPriority)
		}
	})

	t.Run("priority sort conforms to the owner order", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "sort.db")
		db, err := openDB(dbPath)
		if err != nil {
			t.Fatalf("openDB: %v", err)
		}
		defer db.Close()

		for _, p := range priorities {
			if _, err := createTodo(db, "todo-"+p, p, nil); err != nil {
				t.Fatalf("createTodo(%q): %v", p, err)
			}
		}
		todos, err := listTodos(db, listFilter{Sort: "priority"})
		if err != nil {
			t.Fatalf("listTodos: %v", err)
		}
		got := make([]string, len(todos))
		for i, todo := range todos {
			got[i] = todo.Priority
		}
		want := slices.Clone(priorities)
		slices.Reverse(want)
		if !slices.Equal(got, want) {
			t.Errorf("priority sort = %v, want most urgent first %v", got, want)
		}
	})

	t.Run("validation message names the set in owner order", func(t *testing.T) {
		want := "priority must be one of " + strings.Join(priorities, ", ")
		srv := newTestServer(t)

		create := doRequest(t, http.MethodPost, srv.URL+"/api/todos", `{"title":"x","priority":"urgent"}`)
		if create.StatusCode != http.StatusBadRequest {
			t.Fatalf("create status = %d, want 400", create.StatusCode)
		}
		if got := decodeError(t, create); got != want {
			t.Errorf("create priority error = %q, want %q", got, want)
		}

		createTestTodo(t, srv, `{"title":"valid"}`)
		update := doRequest(t, http.MethodPut, srv.URL+"/api/todos/1", `{"title":"x","priority":"urgent"}`)
		if update.StatusCode != http.StatusBadRequest {
			t.Fatalf("update status = %d, want 400", update.StatusCode)
		}
		if got := decodeError(t, update); got != want {
			t.Errorf("update priority error = %q, want %q", got, want)
		}

		list := doRequest(t, http.MethodGet, srv.URL+"/api/todos?priority=urgent", "")
		if list.StatusCode != http.StatusBadRequest {
			t.Fatalf("list status = %d, want 400", list.StatusCode)
		}
		if got := decodeError(t, list); got != want {
			t.Errorf("list priority error = %q, want %q", got, want)
		}
	})
}

func TestListTodosRejectsUnknownValues(t *testing.T) {
	srv := newTestServer(t)
	for _, query := range []string{"?status=bogus", "?priority=urgent", "?sort=alpha"} {
		t.Run(query, func(t *testing.T) {
			resp := doRequest(t, http.MethodGet, srv.URL+"/api/todos"+query, "")
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			decodeError(t, resp)
		})
	}
}

func TestListTodosSearchTreatsWildcardsLiterally(t *testing.T) {
	srv := newTestServer(t)
	createTestTodo(t, srv, `{"title":"50% off"}`)
	createTestTodo(t, srv, `{"title":"a_b"}`)
	createTestTodo(t, srv, `{"title":"axb"}`)
	createTestTodo(t, srv, `{"title":"Plain"}`)

	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"percent is literal", "?q=%25", []string{"50% off"}},
		{"underscore is literal", "?q=a_b", []string{"a_b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := todoTitles(listQuery(t, srv, tc.query))
			if !slices.Equal(got, tc.want) {
				t.Errorf("titles = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestListTodosSearchBindsQueryText(t *testing.T) {
	srv := newTestServer(t)
	createTestTodo(t, srv, `{"title":"O'Brien"}`)
	createTestTodo(t, srv, `{"title":"Plain"}`)

	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"quote is matched literally", "?q=O%27Brien", []string{"O'Brien"}},
		{"injection attempt matches nothing", "?q=%27%20OR%201%3D1%20--%20", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := todoTitles(listQuery(t, srv, tc.query))
			if !slices.Equal(got, tc.want) {
				t.Errorf("titles = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIndexServesHTML(t *testing.T) {
	srv := newTestServer(t)
	resp, page := getIndex(t, srv)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	for _, want := range []string{
		"<html",
		"<form",
		`id="title"`,
		`maxlength="255"`,
		`id="priority"`,
		`type="date"`,
		`/api/todos`,
		`todo-toggle`,
		`todo-delete`,
		`/toggle`,
		`method: "PATCH"`,
		`method: "DELETE"`,
		`todo-edit`,
		`Overdue`,
		`Today`,
		`id="search"`,
		`id="filter-priority"`,
		`id="sort"`,
		"No todos match",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html body missing %q", want)
		}
	}

	// The served form and the server validator are separate hand-written
	// declarations across a language boundary (the file is served verbatim, no
	// build step), so bind them here: the values and parameter names the page
	// uses must match the vocabulary and names the server accepts, and every
	// priority the page can render must have a badge style.
	assertOptionsMatch(t, parseDataStatus(t, page), validStatuses, "status")
	assertOptionsMatch(t, parseSelectOptions(t, page, "priority"), validPriorities, "priority")
	assertOptionsMatch(t, parseSelectOptions(t, page, "filter-priority"), validPriorities, "filter-priority")
	assertOptionsMatch(t, parseSelectOptions(t, page, "sort"), validSorts, "sort")

	// The owner declares a display order, so the page must present each priority
	// in that order rather than merely offering the same set.
	for _, id := range []string{"priority", "filter-priority"} {
		if got := selectOptionSequence(t, page, id); !slices.Equal(got, priorities) {
			t.Errorf("index.html select %q option order = %v, want owner order %v", id, got, priorities)
		}
	}

	for _, param := range []string{paramStatus, paramQuery, paramPriority, paramSort} {
		if !strings.Contains(page, "params.set(\""+param+"\"") {
			t.Errorf("index.html body missing params.set(%q)", param)
		}
	}

	for priority := range validPriorities {
		if !strings.Contains(page, ".badge-"+priority) {
			t.Errorf("index.html missing badge style for priority %q", priority)
		}
	}

	for _, cls := range []string{".due-overdue", ".due-today"} {
		if !strings.Contains(page, cls) {
			t.Errorf("index.html missing due-label style %q", cls)
		}
	}

	// Each control's initial selection must be the server's default for that
	// parameter, so an untouched page requests the documented default list.
	assertSingleSelected(t, parseSelectOptions(t, page, "priority"), "selected", defaultPriority, "priority")
	assertSingleSelected(t, parseSelectOptions(t, page, "sort"), "selected", defaultSort, "sort")
	assertSingleSelected(t, parseDataStatus(t, page), `aria-pressed="true"`, defaultStatus, "status")
}

// The UI is embedded into the binary, so GET / must serve it even when the
// process runs from a directory with no static/ tree. Regression for the
// cwd-relative ServeFile that 404'd the UI outside the repo root (issue #22).
// This is the fast in-process guard; TestBinaryServesHTMLFromAnyWorkingDirectory
// proves the same outcome against the built artifact, which is the only way to
// catch a read that resolves before this test's t.Chdir can take effect.
func TestIndexServesHTMLFromAnyWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := newTestServer(t)
	resp, page := getIndex(t, srv)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(page, "<html") {
		t.Error("served body is not the UI")
	}
}

// TestBinaryServesHTMLFromAnyWorkingDirectory builds the server and runs the
// built artifact from an unrelated cwd, then asserts GET / serves the UI. It
// exists because an in-process test cannot observe a cwd-dependent asset read
// that resolves before the test changes cwd (at package init, or a cached first
// request populated by an earlier test), which is exactly how the issue-#22
// regression would pass the suite while the shipped binary serves no UI.
func TestBinaryServesHTMLFromAnyWorkingDirectory(t *testing.T) {
	bin := buildServerBinary(t)
	port := freePort(t)

	cmd := exec.Command(bin)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		envPort+"="+port,
		envDBPath+"="+filepath.Join(t.TempDir(), "todos.db"),
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	base := "http://127.0.0.1:" + port
	waitForHealthz(t, base, &stderr)

	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (server stderr: %s)", resp.StatusCode, stderr.String())
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "<html") {
		t.Errorf("served body is not the UI (%d bytes)", len(body))
	}
}

// buildServerBinary compiles the package under test into a temp dir and returns
// the binary path. go test runs in the package source dir, so "." is the module.
func buildServerBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "server")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// freePort asks the kernel for an unused port and releases it for the server to
// bind. The gap between release and bind is small; a lost race surfaces as a
// healthz timeout with the server's stderr rather than a silent pass.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("split listen address: %v", err)
	}
	return port
}

// waitForHealthz polls until the server answers /healthz or the deadline passes,
// reporting the server's stderr on failure so a crash is distinguishable from a
// timeout.
func waitForHealthz(t *testing.T, base string, stderr *strings.Builder) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not become healthy at %s (server stderr: %s)", base, stderr.String())
}

// getIndex fetches the served page once for the conformance tests that read it
// as text.
func getIndex(t *testing.T, srv *httptest.Server) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(body)
}

// TestIndexBindsClientRoutes binds every fetch the served page makes to the
// mux's route constants. The page is served verbatim (no build step), so it
// re-declares the routes by hand; comparing the normalized fetch set to the
// owner set fails on a rename on either side instead of silently calling a
// route the server no longer registers.
func TestIndexBindsClientRoutes(t *testing.T) {
	srv := newTestServer(t)
	_, page := getIndex(t, srv)

	want := map[string]bool{}
	for _, route := range []string{routeCreateTodo, routeListTodos, routeUpdateTodo, routeToggleTodo, routeDeleteTodo} {
		method, path := splitRoute(t, route)
		want[method+" "+path] = true
	}

	got := map[string]bool{}
	for _, r := range clientFetchRoutes(t, page) {
		key := r.method + " " + r.path
		if !want[key] {
			t.Errorf("index.html fetch %s does not match any server route", key)
		}
		got[key] = true
	}
	for key := range want {
		if !got[key] {
			t.Errorf("server route %s is not fetched by index.html", key)
		}
	}
}

// TestIndexBindsRequestPayloads binds each request flow's body keys to its own
// request struct in both directions: a key dropped from one flow cannot hide
// behind another flow that still sends it, and a key the client declares that
// the server does not own is caught instead of being silently dropped by the
// decoder.
func TestIndexBindsRequestPayloads(t *testing.T) {
	srv := newTestServer(t)
	_, page := getIndex(t, srv)

	cases := []struct {
		name   string
		anchor string
		sample any
	}{
		{"create", `form.addEventListener("submit"`, createTodoRequest{}},
		{"update", "function saveEdit", updateTodoRequest{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keys := payloadKeysBefore(t, page, tc.anchor)
			serverTags := map[string]bool{}
			for _, tag := range jsonTags(t, tc.sample) {
				serverTags[tag] = true
				if !keys[tag] {
					t.Errorf("index.html %s payload missing %T key %q", tc.name, tc.sample, tag)
				}
			}
			for key := range keys {
				if !serverTags[key] {
					t.Errorf("index.html %s payload declares %q, not a %T json tag", tc.name, key, tc.sample)
				}
			}
		})
	}
}

// TestIndexBindsTodoReadKeys checks that every field the page reads from a todo
// object is a Todo json tag, so a renamed response field fails here instead of
// rendering blank.
func TestIndexBindsTodoReadKeys(t *testing.T) {
	srv := newTestServer(t)
	_, page := getIndex(t, srv)

	todoTags := map[string]bool{}
	for _, tag := range jsonTags(t, Todo{}) {
		todoTags[tag] = true
	}
	for _, key := range todoReadKeys(page) {
		if !todoTags[key] {
			t.Errorf("index.html reads todo.%s, which is not a Todo json tag", key)
		}
	}
}

// TestIndexBindsErrorEnvelope triggers a real server error, decodes the
// envelope it emits, and checks every key the page reads from an error body
// exists there. The behavior is the contract, so probing it needs no owner
// constant and fails on drift in either direction.
func TestIndexBindsErrorEnvelope(t *testing.T) {
	srv := newTestServer(t)
	_, page := getIndex(t, srv)

	resp, err := http.Post(srv.URL+"/api/todos", "application/json", strings.NewReader("{"))
	if err != nil {
		t.Fatalf("POST /api/todos: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var envelope map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	for _, key := range clientErrorReadKeys(t, page) {
		if _, ok := envelope[key]; !ok {
			t.Errorf("index.html reads error key %q, absent from server envelope %v", key, envelope)
		}
	}
}

// clientRoute is a fetch's verb and normalized path, in the same
// "METHOD /path" shape the mux registers.
type clientRoute struct {
	method string
	path   string
}

// clientFetchRoutes scans the served page for every fetch(...) call and
// normalizes it to the "METHOD /path" shape the mux registers, so the client's
// declared routes can be compared to the route constants rather than merely
// substring-matched. It fails loudly on a fetch whose shape it cannot classify
// rather than skipping it, so a new client call cannot pass unnoticed.
func clientFetchRoutes(t *testing.T, page string) []clientRoute {
	t.Helper()
	fetchRe := regexp.MustCompile(`fetch\(([^,)]+)`)
	verbRe := regexp.MustCompile(`^\s*,\s*\{\s*method:\s*"([A-Z]+)"`)
	literalRe := regexp.MustCompile(`"([^"]*)"|todo\.id`)
	listRe := regexp.MustCompile(`"(/api/[^"?]*)`)

	var routes []clientRoute
	for _, loc := range fetchRe.FindAllStringSubmatchIndex(page, -1) {
		expr := page[loc[2]:loc[3]]
		method := http.MethodGet
		if m := verbRe.FindStringSubmatch(page[loc[1]:]); m != nil {
			method = m[1]
		}

		var path string
		if strings.Contains(expr, "listURL") {
			// The list route is built in listURL(), not written inline, so it
			// cannot be read from the fetch call alone; bind its base path
			// (the query string is already bound by TestIndexServesHTML).
			listAt := strings.Index(page, "function listURL")
			if listAt < 0 {
				t.Fatal("page fetches listURL() but defines no function listURL")
			}
			m := listRe.FindStringSubmatch(page[listAt:])
			if m == nil {
				t.Fatal("listURL() has no /api/ literal to bind")
			}
			path = m[1]
		} else {
			for _, m := range literalRe.FindAllStringSubmatch(expr, -1) {
				if m[0] == "todo.id" {
					path += "{id}"
				} else {
					path += m[1]
				}
			}
		}
		if path == "" {
			t.Fatalf("fetch expression %q has no recognizable path", expr)
		}
		routes = append(routes, clientRoute{method: method, path: path})
	}
	if len(routes) == 0 {
		t.Fatal("page contains no fetch calls")
	}
	return routes
}

// payloadKeysBefore returns the request-body keys the page sets on the payload
// object of the flow beginning at anchor, so each flow's copy can be bound to
// its own request struct rather than to a merge of every flow's payload.
func payloadKeysBefore(t *testing.T, page, anchor string) map[string]bool {
	t.Helper()
	start := strings.Index(page, anchor)
	if start < 0 {
		t.Fatalf("index.html has no anchor %q", anchor)
	}
	rest := page[start:]
	payloadAt := strings.Index(rest, "var payload")
	if payloadAt < 0 {
		t.Fatalf("index.html flow %q sets no payload", anchor)
	}
	slice := rest[payloadAt:]
	fetchAt := strings.Index(slice, "fetch(")
	if fetchAt < 0 {
		t.Fatalf("index.html flow %q has no fetch after its payload", anchor)
	}
	return payloadKeys(slice[:fetchAt])
}

// clientErrorReadKeys returns the distinct keys the page reads from an error
// envelope body, so the client's copy can be bound to the envelope the server
// actually emits.
func clientErrorReadKeys(t *testing.T, page string) []string {
	t.Helper()
	seen := map[string]bool{}
	var keys []string
	for _, m := range regexp.MustCompile(`body\.([A-Za-z_]+)`).FindAllStringSubmatch(page, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			keys = append(keys, m[1])
		}
	}
	if len(keys) == 0 {
		t.Fatal("index.html reads no error-envelope key")
	}
	return keys
}

// jsonTags returns the wire keys a request or response struct declares, read
// from its struct tags so the served page's client copy can be compared to its
// owner.
func jsonTags(t *testing.T, sample any) []string {
	t.Helper()
	typ := reflect.TypeOf(sample)
	tags := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			t.Fatalf("%s field %s has no usable json tag", typ.Name(), typ.Field(i).Name)
		}
		tags = append(tags, name)
	}
	return tags
}

// payloadKeys returns every key the page sets on a request `payload` object,
// whether in an object literal or a later property assignment.
func payloadKeys(page string) map[string]bool {
	keys := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?s)var payload = \{(.*?)\}`).FindAllStringSubmatch(page, -1) {
		for _, k := range regexp.MustCompile(`([A-Za-z_]+)\s*:`).FindAllStringSubmatch(m[1], -1) {
			keys[k[1]] = true
		}
	}
	for _, m := range regexp.MustCompile(`payload\.([A-Za-z_]+)\s*=`).FindAllStringSubmatch(page, -1) {
		keys[m[1]] = true
	}
	return keys
}

// todoReadKeys returns the distinct fields the page reads from a todo object,
// whether it came from the list or a write response.
func todoReadKeys(page string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range regexp.MustCompile(`todo\.([A-Za-z_]+)`).FindAllStringSubmatch(page, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			keys = append(keys, m[1])
		}
	}
	return keys
}

// splitRoute splits a mux pattern like "PUT /api/todos/{id}" into its verb and
// path.
func splitRoute(t *testing.T, route string) (method, path string) {
	t.Helper()
	parts := strings.SplitN(route, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("route %q is not \"METHOD /path\"", route)
	}
	return parts[0], parts[1]
}

// parseDataStatus maps each status button's data-status value to the rest of
// its attributes, so the segmented control can be compared to the server's
// status vocabulary and its selected default read back.
func parseDataStatus(t *testing.T, page string) map[string]string {
	t.Helper()
	re := regexp.MustCompile(`<button[^>]*data-status="([^"]+)"([^>]*)>`)
	matches := re.FindAllStringSubmatch(page, -1)
	if len(matches) == 0 {
		t.Fatal("index.html contains no data-status buttons")
	}
	statuses := map[string]string{}
	for _, m := range matches {
		statuses[m[1]] = m[2]
	}
	return statuses
}

// assertSingleSelected checks that exactly one option carries the selection
// marker and that it is the wanted value.
func assertSingleSelected(t *testing.T, options map[string]string, marker, want, label string) {
	t.Helper()
	var selected []string
	for value, attrs := range options {
		if strings.Contains(attrs, marker) {
			selected = append(selected, value)
		}
	}
	if len(selected) != 1 || selected[0] != want {
		t.Errorf("selected %s = %v, want exactly [%q]", label, selected, want)
	}
}

// parseSelectOptions maps each <option value="..."> inside the select with the
// given id to its remaining attributes. The empty value (a filter's "any"
// sentinel) has no option here because it is not a value the server accepts.
func parseSelectOptions(t *testing.T, page, id string) map[string]string {
	t.Helper()
	selectRe := regexp.MustCompile(`(?s)<select id="` + id + `"[^>]*>(.*?)</select>`)
	m := selectRe.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("index.html contains no select id=%q", id)
	}
	re := regexp.MustCompile(`<option value="([^"]+)"([^>]*)>`)
	options := map[string]string{}
	for _, om := range re.FindAllStringSubmatch(m[1], -1) {
		options[om[1]] = om[2]
	}
	if len(options) == 0 {
		t.Fatalf("index.html select id=%q contains no options", id)
	}
	return options
}

// selectOptionSequence returns the <option value="..."> values inside the select
// with the given id, in document order. The empty "any" sentinel of a filter is
// skipped because it is not a value the server accepts.
func selectOptionSequence(t *testing.T, page, id string) []string {
	t.Helper()
	selectRe := regexp.MustCompile(`(?s)<select id="` + id + `"[^>]*>(.*?)</select>`)
	m := selectRe.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("index.html contains no select id=%q", id)
	}
	var values []string
	for _, om := range regexp.MustCompile(`<option value="([^"]*)"`).FindAllStringSubmatch(m[1], -1) {
		if om[1] != "" {
			values = append(values, om[1])
		}
	}
	if len(values) == 0 {
		t.Fatalf("index.html select id=%q contains no options", id)
	}
	return values
}

// assertOptionsMatch checks that a select's offered values are exactly the
// server's accepted vocabulary, so neither side can drift without the test
// noticing.
func assertOptionsMatch(t *testing.T, options map[string]string, valid map[string]bool, label string) {
	t.Helper()
	for value := range valid {
		if _, ok := options[value]; !ok {
			t.Errorf("%s option %q missing from index.html", label, value)
		}
	}
	for value := range options {
		if !valid[value] {
			t.Errorf("index.html %s offers unknown value %q", label, value)
		}
	}
}
