package taskmanager

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"interface-load-test/internal/task"
)

func TestSQLStoreInsert(t *testing.T) {
	db, mock := newTaskMockDB(t)
	store := NewSQLStore(db)
	tk := &task.Task{
		ID:           "task-1",
		ModuleType:   "load_test",
		Name:         "load task",
		Status:       task.StatusPending,
		Config:       []byte(`{"ok":true}`),
		Concurrency:  3,
		TotalCount:   5,
		SuccessCount: 1,
		FailCount:    2,
		ErrMsg:       "not yet",
		CreatedBy:    "tester",
	}

	mock.ExpectExec(regexp.QuoteMeta(insertTaskSQL)).
		WithArgs("task-1", "load_test", "load task", "pending", `{"ok":true}`, 3, 5, 1, 2, "not yet", "tester").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.Insert(context.Background(), tk); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	assertTaskExpectations(t, mock)
}

func TestSQLStoreGet(t *testing.T) {
	db, mock := newTaskMockDB(t)
	store := NewSQLStore(db)
	createdAt := time.Date(2026, 8, 4, 17, 0, 0, 0, time.UTC)
	startedAt := createdAt.Add(time.Minute)
	finishedAt := createdAt.Add(2 * time.Minute)

	mock.ExpectQuery(regexp.QuoteMeta(getTaskSQL)).
		WithArgs("task-1").
		WillReturnRows(sqlmock.NewRows(taskColumns()).
			AddRow("task-1", "load_test", "load task", "success", []byte(`{"ok":true}`), 3, 5, 4, 1, "done", "tester", createdAt, startedAt, finishedAt))

	got, err := store.Get(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.ID != "task-1" || got.ModuleType != "load_test" || got.Name != "load task" {
		t.Fatalf("Get() returned unexpected identity: %#v", got)
	}
	if got.Status != task.StatusSuccess {
		t.Fatalf("Status = %q, want %q", got.Status, task.StatusSuccess)
	}
	if string(got.Config) != `{"ok":true}` {
		t.Fatalf("Config = %s, want JSON", got.Config)
	}
	if got.Concurrency != 3 || got.TotalCount != 5 || got.SuccessCount != 4 || got.FailCount != 1 {
		t.Fatalf("Get() returned unexpected counts: %#v", got)
	}
	if got.ErrMsg != "done" || got.CreatedBy != "tester" || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("Get() returned unexpected metadata: %#v", got)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(startedAt) {
		t.Fatalf("StartedAt = %v, want %v", got.StartedAt, startedAt)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt) {
		t.Fatalf("FinishedAt = %v, want %v", got.FinishedAt, finishedAt)
	}
	assertTaskExpectations(t, mock)
}

func TestSQLStoreGetNotFound(t *testing.T) {
	db, mock := newTaskMockDB(t)
	store := NewSQLStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(getTaskSQL)).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	got, err := store.Get(context.Background(), "missing")
	if got != nil {
		t.Fatalf("Get() task = %#v, want nil", got)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want %v", err, ErrNotFound)
	}
	assertTaskExpectations(t, mock)
}

func TestSQLStoreList(t *testing.T) {
	createdAt := time.Date(2026, 8, 4, 17, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		filter   ListFilter
		query    string
		args     []driver.Value
		wantIDs  []string
		wantSize int
	}{
		{
			name:     "default limit without status",
			filter:   ListFilter{},
			query:    listTasksSQL,
			args:     []driver.Value{defaultListLimit},
			wantIDs:  []string{"task-2", "task-1"},
			wantSize: 2,
		},
		{
			name:     "status filter with explicit limit",
			filter:   ListFilter{Status: task.StatusRunning, Limit: 20},
			query:    listTasksByStatusSQL,
			args:     []driver.Value{"running", 20},
			wantIDs:  []string{"task-3"},
			wantSize: 1,
		},
		{
			name:     "clamps oversized limit",
			filter:   ListFilter{Status: task.StatusFailed, Limit: 999},
			query:    listTasksByStatusSQL,
			args:     []driver.Value{"failed", maxListLimit},
			wantIDs:  []string{},
			wantSize: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := newTaskMockDB(t)
			store := NewSQLStore(db)

			rows := sqlmock.NewRows(taskColumns())
			for i, id := range tt.wantIDs {
				rows.AddRow(id, "load_test", "load task", "running", []byte(`{"ok":true}`), 3, 5, 0, 0, nil, "tester", createdAt.Add(time.Duration(i)*time.Minute), nil, nil)
			}
			expectation := mock.ExpectQuery(regexp.QuoteMeta(tt.query))
			expectation.WithArgs(tt.args...).WillReturnRows(rows)

			got, err := store.List(context.Background(), tt.filter)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(got) != tt.wantSize {
				t.Fatalf("len(List()) = %d, want %d", len(got), tt.wantSize)
			}
			for i, wantID := range tt.wantIDs {
				if got[i].ID != wantID {
					t.Fatalf("task[%d].ID = %q, want %q", i, got[i].ID, wantID)
				}
			}
			assertTaskExpectations(t, mock)
		})
	}
}

func TestSQLStoreMarkRunning(t *testing.T) {
	db, mock := newTaskMockDB(t)
	store := NewSQLStore(db)
	startedAt := time.Date(2026, 8, 4, 18, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(markRunningSQL)).
		WithArgs("running", startedAt, "task-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.MarkRunning(context.Background(), "task-1", startedAt); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}
	assertTaskExpectations(t, mock)
}

func TestSQLStoreMarkFinished(t *testing.T) {
	db, mock := newTaskMockDB(t)
	store := NewSQLStore(db)
	finishedAt := time.Date(2026, 8, 4, 19, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(markFinishedSQL)).
		WithArgs("failed", finishedAt, 3, 2, "module failed", "task-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.MarkFinished(context.Background(), "task-1", task.StatusFailed, finishedAt, 3, 2, "module failed"); err != nil {
		t.Fatalf("MarkFinished() error = %v", err)
	}
	assertTaskExpectations(t, mock)
}

func newTaskMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	return db, mock
}

func taskColumns() []string {
	return []string{
		"id",
		"module_type",
		"name",
		"status",
		"config",
		"concurrency",
		"total_count",
		"success_count",
		"fail_count",
		"err_msg",
		"created_by",
		"created_at",
		"started_at",
		"finished_at",
	}
}

func assertTaskExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
