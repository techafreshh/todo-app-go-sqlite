package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

// listFilter is the validated set of query inputs for a list request. The zero
// value means "all statuses, any priority, any text, oldest first".
type listFilter struct {
	Status   string // "all", "active", or "done"
	Priority string // "" means any
	Query    string // "" means any; case-insensitive substring of title
	Sort     string // "due", "created", or "priority"
}

func listTodos(db *sql.DB, filter listFilter) ([]Todo, error) {
	var (
		clauses []string
		args    []any
	)
	switch filter.Status {
	case "active":
		clauses = append(clauses, "done = 0")
	case "done":
		clauses = append(clauses, "done = 1")
	}
	if filter.Priority != "" {
		clauses = append(clauses, "priority = ?")
		args = append(args, filter.Priority)
	}
	if filter.Query != "" {
		clauses = append(clauses, `title LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(filter.Query)+"%")
	}

	query := `SELECT id, title, done, priority, due_date, created_at, updated_at FROM todos`
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY " + orderByClause(filter.Sort)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	todos := []Todo{}
	for rows.Next() {
		t, err := scanTodo(rows)
		if err != nil {
			return nil, err
		}
		todos = append(todos, t)
	}
	return todos, rows.Err()
}

// orderByClause maps a sort key to an ORDER BY list. Every case ends in id so
// rows sharing a sort key (created_at has second precision) stay stable.
func orderByClause(sort string) string {
	switch sort {
	case "due":
		return "due_date IS NULL, due_date, id"
	case "priority":
		return priorityOrderClause() + ", id"
	default:
		return "created_at, id"
	}
}

// priorityOrderClause renders the ORDER BY CASE that ranks the most urgent
// priority first, built from the priorities owner so the SQL sort rank cannot
// drift from the validated set. The interpolated priorities are compile-time
// constants, never user input.
func priorityOrderClause() string {
	var b strings.Builder
	b.WriteString("CASE priority")
	for i, p := range priorities {
		fmt.Fprintf(&b, " WHEN '%s' THEN %d", p, len(priorities)-1-i)
	}
	fmt.Fprintf(&b, " ELSE %d END", len(priorities))
	return b.String()
}

// escapeLike quotes the LIKE metacharacters so a query matches the literal text
// a user typed rather than treating % and _ as wildcards.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanTodo reads the full todo column set into a Todo. Both listTodos and
// toggleTodo read the same columns, so the persisted-representation mapping
// lives here once.
func scanTodo(row rowScanner) (Todo, error) {
	var (
		t       Todo
		done    int
		dueDate sql.NullString
	)
	if err := row.Scan(&t.ID, &t.Title, &done, &t.Priority, &dueDate, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return Todo{}, err
	}
	t.Done = done != 0
	if dueDate.Valid {
		t.DueDate = &dueDate.String
	}
	return t, nil
}

func updateTodo(db *sql.DB, id int64, title, priority string, dueDate *string, done bool) (Todo, error) {
	tx, err := db.Begin()
	if err != nil {
		return Todo{}, err
	}
	defer tx.Rollback()

	var createdAt, previousUpdatedAt string
	err = tx.QueryRow(`SELECT created_at, updated_at FROM todos WHERE id = ?`, id).Scan(&createdAt, &previousUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Todo{}, errNotFound
	}
	if err != nil {
		return Todo{}, err
	}

	updatedAt := nextTimestamp(previousUpdatedAt)
	res, err := tx.Exec(
		`UPDATE todos SET title = ?, priority = ?, due_date = ?, done = ?, updated_at = ? WHERE id = ?`,
		title, priority, dueDate, done, updatedAt, id,
	)
	if err != nil {
		return Todo{}, err
	}
	if err := requireRowAffected(res); err != nil {
		return Todo{}, err
	}
	if err := tx.Commit(); err != nil {
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
	tx, err := db.Begin()
	if err != nil {
		return Todo{}, err
	}
	defer tx.Rollback()

	t, err := scanTodo(tx.QueryRow(
		`SELECT id, title, done, priority, due_date, created_at, updated_at FROM todos WHERE id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Todo{}, errNotFound
	}
	if err != nil {
		return Todo{}, err
	}

	t.Done = !t.Done
	t.UpdatedAt = nextTimestamp(t.UpdatedAt)
	res, err := tx.Exec(`UPDATE todos SET done = ?, updated_at = ? WHERE id = ?`, t.Done, t.UpdatedAt, id)
	if err != nil {
		return Todo{}, err
	}
	if err := requireRowAffected(res); err != nil {
		return Todo{}, err
	}
	if err := tx.Commit(); err != nil {
		return Todo{}, err
	}
	return t, nil
}

// requireRowAffected maps a write that matched no row to errNotFound, so a row
// that vanished between the read and the write is reported as missing rather
// than a phantom success.
func requireRowAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound
	}
	return nil
}

func deleteTodo(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM todos WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// nextTimestamp returns a write timestamp strictly later than the previous
// updated_at. The stored format has second precision, so a write within the same
// second as the previous one is bumped to keep updated_at strictly newer.
func nextTimestamp(previous string) string {
	now := time.Now().UTC().Truncate(time.Second)
	if prev, err := time.Parse(time.RFC3339, previous); err == nil && !now.After(prev) {
		now = prev.Add(time.Second)
	}
	return now.Format(time.RFC3339)
}
