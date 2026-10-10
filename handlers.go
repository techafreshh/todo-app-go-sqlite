package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
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

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

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

	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if utf8.RuneCountInString(title) > 255 {
		writeError(w, http.StatusBadRequest, "title must be at most 255 characters")
		return
	}

	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}
	if !validPriorities[priority] {
		writeError(w, http.StatusBadRequest, "priority must be one of low, medium, high")
		return
	}

	var dueDate *string
	if req.DueDate != nil && *req.DueDate != "" {
		if !datePattern.MatchString(*req.DueDate) {
			writeError(w, http.StatusBadRequest, "due_date must be YYYY-MM-DD")
			return
		}
		if _, err := time.Parse("2006-01-02", *req.DueDate); err != nil {
			writeError(w, http.StatusBadRequest, "due_date must be YYYY-MM-DD")
			return
		}
		dueDate = req.DueDate
	}

	todo, err := createTodo(h.DB, title, priority, dueDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create todo")
		return
	}
	writeJSON(w, http.StatusCreated, todo)
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
