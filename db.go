package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// errNotFound reports that no todo row matches the requested id.
var errNotFound = errors.New("todo not found")

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
	now := nowTimestamp()
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

func updateTodo(db *sql.DB, id int64, title, priority string, dueDate *string, done bool) (Todo, error) {
	var createdAt, previousUpdatedAt string
	err := db.QueryRow(`SELECT created_at, updated_at FROM todos WHERE id = ?`, id).Scan(&createdAt, &previousUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Todo{}, errNotFound
	}
	if err != nil {
		return Todo{}, err
	}

	updatedAt := nextTimestamp(createdAt, previousUpdatedAt)
	if _, err := db.Exec(
		`UPDATE todos SET title = ?, priority = ?, due_date = ?, done = ?, updated_at = ? WHERE id = ?`,
		title, priority, dueDate, done, updatedAt, id,
	); err != nil {
		return Todo{}, err
	}
	return Todo{
		ID:        id,
		Title:     title,
		Done:      done,
		Priority:  priority,
		DueDate:   dueDate,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func toggleTodo(db *sql.DB, id int64) (Todo, error) {
	var (
		t         Todo
		done      int
		dueDate   sql.NullString
		createdAt string
	)
	err := db.QueryRow(
		`SELECT id, title, done, priority, due_date, created_at, updated_at FROM todos WHERE id = ?`, id,
	).Scan(&t.ID, &t.Title, &done, &t.Priority, &dueDate, &createdAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Todo{}, errNotFound
	}
	if err != nil {
		return Todo{}, err
	}

	t.CreatedAt = createdAt
	t.Done = done == 0
	if dueDate.Valid {
		t.DueDate = &dueDate.String
	}
	t.UpdatedAt = nextTimestamp(createdAt, t.UpdatedAt)
	if _, err := db.Exec(`UPDATE todos SET done = ?, updated_at = ? WHERE id = ?`, t.Done, t.UpdatedAt, id); err != nil {
		return Todo{}, err
	}
	return t, nil
}

func deleteTodo(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM todos WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound
	}
	return nil
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// nextTimestamp returns a write timestamp strictly later than every previous
// timestamp given. The stored format has second precision, so a write within the
// same second as a prior one must be bumped to keep updated_at strictly newer.
func nextTimestamp(previous ...string) string {
	now := time.Now().UTC().Truncate(time.Second)
	for _, ts := range previous {
		if prev, err := time.Parse(time.RFC3339, ts); err == nil && !now.After(prev) {
			now = prev.Add(time.Second)
		}
	}
	return now.Format(time.RFC3339)
}
