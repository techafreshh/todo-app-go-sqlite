package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Handler struct {
	DB *sql.DB
}

type createTodoRequest struct {
	Title    string  `json:"title"`
	Priority string  `json:"priority"`
	DueDate  *string `json:"due_date"`
}

type updateTodoRequest struct {
	Title    string  `json:"title"`
	Priority string  `json:"priority"`
	DueDate  *string `json:"due_date"`
	Done     bool    `json:"done"`
}

var validPriorities = map[string]bool{
	"low":    true,
	"medium": true,
	"high":   true,
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) createTodo(w http.ResponseWriter, r *http.Request) {
	var req createTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	title, priority, dueDate, err := validateTodo(req.Title, req.Priority, req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	todo, err := createTodo(h.DB, title, priority, dueDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create todo")
		return
	}
	writeJSON(w, http.StatusCreated, todo)
}

func (h *Handler) updateTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req updateTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	title, priority, dueDate, err := validateTodo(req.Title, req.Priority, req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	todo, err := updateTodo(h.DB, id, title, priority, dueDate, req.Done)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update todo")
		return
	}
	writeJSON(w, http.StatusOK, todo)
}

func (h *Handler) toggleTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	todo, err := toggleTodo(h.DB, id)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to toggle todo")
		return
	}
	writeJSON(w, http.StatusOK, todo)
}

func (h *Handler) deleteTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := deleteTodo(h.DB, id); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete todo")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateTodo(title, priority string, dueDate *string) (string, string, *string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", "", nil, errors.New("title is required")
	}
	if utf8.RuneCountInString(title) > 255 {
		return "", "", nil, errors.New("title must be at most 255 characters")
	}

	if priority == "" {
		priority = "medium"
	}
	if !validPriorities[priority] {
		return "", "", nil, errors.New("priority must be one of low, medium, high")
	}

	var normalizedDueDate *string
	if dueDate != nil && *dueDate != "" {
		if _, err := time.Parse("2006-01-02", *dueDate); err != nil {
			return "", "", nil, errors.New("due_date must be YYYY-MM-DD")
		}
		normalizedDueDate = dueDate
	}
	return title, priority, normalizedDueDate, nil
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "todo not found")
		return 0, false
	}
	return id, true
}

func (h *Handler) listTodos(w http.ResponseWriter, r *http.Request) {
	todos, err := listTodos(h.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list todos")
		return
	}
	writeJSON(w, http.StatusOK, todos)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
