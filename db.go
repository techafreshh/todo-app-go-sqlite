package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Todo struct {
	ID        int64   `json:"id"`
	Title     string  `json:"title"`
	Done      bool    `json:"done"`
	Priority  string  `json:"priority"`
	DueDate   *string `json:"due_date"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

func openDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer; a single connection avoids SQLITE_BUSY under
	// concurrent requests for this single-user service.
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS todos (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	title      TEXT    NOT NULL,
	done       INTEGER NOT NULL DEFAULT 0,
	priority   TEXT    NOT NULL DEFAULT 'medium',
	due_date   TEXT,
	created_at TEXT    NOT NULL,
	updated_at TEXT    NOT NULL
);`
	_, err := db.Exec(schema)
	return err
}

func createTodo(db *sql.DB, title, priority string, dueDate *string) (Todo, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`INSERT INTO todos (title, done, priority, due_date, created_at, updated_at)
		 VALUES (?, 0, ?, ?, ?, ?)`,
		title, priority, dueDate, now, now,
	)
	if err != nil {
		return Todo{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Todo{}, err
	}
	return Todo{
		ID:        id,
		Title:     title,
		Done:      false,
		Priority:  priority,
		DueDate:   dueDate,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func listTodos(db *sql.DB) ([]Todo, error) {
	rows, err := db.Query(
		`SELECT id, title, done, priority, due_date, created_at, updated_at
		 FROM todos ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	todos := []Todo{}
	for rows.Next() {
		var (
			t       Todo
			done    int
			dueDate sql.NullString
		)
		if err := rows.Scan(&t.ID, &t.Title, &done, &t.Priority, &dueDate, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Done = done != 0
		if dueDate.Valid {
			t.DueDate = &dueDate.String
		}
		todos = append(todos, t)
	}
	return todos, rows.Err()
}
