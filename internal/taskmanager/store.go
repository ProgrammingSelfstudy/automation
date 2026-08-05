package taskmanager

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"interface-load-test/internal/task"
)

var ErrNotFound = errors.New("task not found")

const (
	insertTaskSQL        = `INSERT INTO task (id, module_type, name, status, config, concurrency, total_count, success_count, fail_count, err_msg, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	getTaskSQL           = `SELECT id, module_type, name, status, config, concurrency, total_count, success_count, fail_count, err_msg, created_by, created_at, started_at, finished_at FROM task WHERE id = ?`
	listTasksSQL         = `SELECT id, module_type, name, status, config, concurrency, total_count, success_count, fail_count, err_msg, created_by, created_at, started_at, finished_at FROM task ORDER BY created_at DESC LIMIT ?`
	listTasksByStatusSQL = `SELECT id, module_type, name, status, config, concurrency, total_count, success_count, fail_count, err_msg, created_by, created_at, started_at, finished_at FROM task WHERE status = ? ORDER BY created_at DESC LIMIT ?`
	markRunningSQL       = `UPDATE task SET status = ?, started_at = ? WHERE id = ?`
	markFinishedSQL      = `UPDATE task SET status = ?, finished_at = ?, success_count = ?, fail_count = ?, err_msg = ? WHERE id = ?`
	maxTaskErrMsgBytes   = 65535
	maxTaskNameRunes     = 128
	maxCreatedByRunes    = 64
	defaultEmptyJSONText = `{}`
	defaultListLimit     = 50
	maxListLimit         = 200
)

// ListFilter limits task list results.
type ListFilter struct {
	Status task.Status
	Limit  int
}

// Store persists task lifecycle state.
type Store interface {
	Insert(ctx context.Context, t *task.Task) error
	Get(ctx context.Context, taskID string) (*task.Task, error)
	List(ctx context.Context, filter ListFilter) ([]*task.Task, error)
	MarkRunning(ctx context.Context, taskID string, startedAt time.Time) error
	MarkFinished(ctx context.Context, taskID string, status task.Status, finishedAt time.Time, successCount, failCount int, errMsg string) error
}

// SQLStore is a database/sql backed Store.
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore builds a Store from an already-open database handle.
func NewSQLStore(db *sql.DB) *SQLStore {
	return &SQLStore{db: db}
}

// NewMySQLStore opens a MySQL connection and pings it once.
func NewMySQLStore(dsn string) (*SQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return NewSQLStore(db), nil
}

// Insert creates a pending task row.
func (s *SQLStore) Insert(ctx context.Context, t *task.Task) error {
	config := t.Config
	if len(config) == 0 {
		config = []byte(defaultEmptyJSONText)
	}

	_, err := s.db.ExecContext(
		ctx,
		insertTaskSQL,
		t.ID,
		t.ModuleType,
		truncateRunes(t.Name, maxTaskNameRunes),
		string(t.Status),
		string(config),
		t.Concurrency,
		t.TotalCount,
		t.SuccessCount,
		t.FailCount,
		truncateUTF8Bytes(t.ErrMsg, maxTaskErrMsgBytes),
		truncateRunes(t.CreatedBy, maxCreatedByRunes),
	)
	return err
}

// Get returns one task by ID.
func (s *SQLStore) Get(ctx context.Context, taskID string) (*task.Task, error) {
	row := s.db.QueryRowContext(ctx, getTaskSQL, taskID)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// List returns recent tasks, optionally filtered by status.
func (s *SQLStore) List(ctx context.Context, filter ListFilter) ([]*task.Task, error) {
	limit := normalizeListLimit(filter.Limit)

	var (
		rows *sql.Rows
		err  error
	)
	if filter.Status == "" {
		rows, err = s.db.QueryContext(ctx, listTasksSQL, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, listTasksByStatusSQL, string(filter.Status), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]*task.Task, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

// MarkRunning marks a task running.
func (s *SQLStore) MarkRunning(ctx context.Context, taskID string, startedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, markRunningSQL, string(task.StatusRunning), startedAt, taskID)
	return err
}

// MarkFinished records final task status and counts.
func (s *SQLStore) MarkFinished(ctx context.Context, taskID string, status task.Status, finishedAt time.Time, successCount, failCount int, errMsg string) error {
	_, err := s.db.ExecContext(
		ctx,
		markFinishedSQL,
		string(status),
		finishedAt,
		successCount,
		failCount,
		truncateUTF8Bytes(errMsg, maxTaskErrMsgBytes),
		taskID,
	)
	return err
}

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (*task.Task, error) {
	var t task.Task
	var status string
	var config []byte
	var errMsg sql.NullString
	var createdBy sql.NullString
	var startedAt sql.NullTime
	var finishedAt sql.NullTime

	err := scanner.Scan(
		&t.ID,
		&t.ModuleType,
		&t.Name,
		&status,
		&config,
		&t.Concurrency,
		&t.TotalCount,
		&t.SuccessCount,
		&t.FailCount,
		&errMsg,
		&createdBy,
		&t.CreatedAt,
		&startedAt,
		&finishedAt,
	)
	if err != nil {
		return nil, err
	}

	t.Status = task.Status(status)
	t.Config = append(t.Config[:0], config...)
	if errMsg.Valid {
		t.ErrMsg = errMsg.String
	}
	if createdBy.Valid {
		t.CreatedBy = createdBy.String
	}
	if startedAt.Valid {
		t.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		t.FinishedAt = &finishedAt.Time
	}

	return &t, nil
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

func truncateUTF8Bytes(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}

	end := 0
	for i := range s {
		if i > maxBytes {
			break
		}
		end = i
	}
	return s[:end]
}

func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}
