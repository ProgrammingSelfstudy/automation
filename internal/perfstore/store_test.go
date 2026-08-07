package perfstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateValidatesUserID(t *testing.T) {
	db, mock := newPerfTaskMockDB(t)
	store := NewSQLStore(db)
	task := &PerfTask{
		PerfTaskSummary: PerfTaskSummary{DeviceID: "device-1"},
	}

	err := store.Create(context.Background(), task)
	if !errors.Is(err, ErrUserIDRequired) {
		t.Fatalf("Create() error = %v, want %v", err, ErrUserIDRequired)
	}
	assertPerfTaskExpectations(t, mock)
}

func TestCreateValidatesDeviceID(t *testing.T) {
	db, mock := newPerfTaskMockDB(t)
	store := NewSQLStore(db)
	task := &PerfTask{
		PerfTaskSummary: PerfTaskSummary{UserID: "user-1"},
	}

	err := store.Create(context.Background(), task)
	if !errors.Is(err, ErrDeviceIDRequired) {
		t.Fatalf("Create() error = %v, want %v", err, ErrDeviceIDRequired)
	}
	assertPerfTaskExpectations(t, mock)
}

func TestCreateGeneratesIDAndStoresSamples(t *testing.T) {
	db, mock := newPerfTaskMockDB(t)
	store := NewSQLStore(db)
	startTime := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	stopTime := startTime.Add(time.Minute)
	task := &PerfTask{
		PerfTaskSummary: PerfTaskSummary{
			UserID:           "user-1",
			DeviceID:         "device-1",
			PackageName:      "com.example.app",
			ProcessName:      "com.example.app",
			Platform:         "android",
			DeviceModel:      "Pixel 7",
			Status:           "finished",
			StartTime:        startTime,
			StopTime:         stopTime,
			SampleIntervalMS: 1000,
			SampleCount:      2,
		},
		Samples: []byte(`[{"cpu":1.2},{"cpu":1.5}]`),
	}

	mock.ExpectExec(regexp.QuoteMeta(insertPerfTaskSQL)).
		WithArgs(
			anyPerfTaskID{},
			"user-1",
			"device-1",
			"com.example.app",
			"com.example.app",
			"android",
			"Pixel 7",
			"finished",
			startTime,
			stopTime,
			int64(1000),
			2,
			nil,
			string(task.Samples),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.Create(context.Background(), task); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if task.ID == "" {
		t.Fatal("Create() did not generate task ID")
	}
	assertPerfTaskExpectations(t, mock)
}

func TestCreateDefaultsEmptySamples(t *testing.T) {
	db, mock := newPerfTaskMockDB(t)
	store := NewSQLStore(db)
	task := &PerfTask{
		PerfTaskSummary: PerfTaskSummary{
			UserID:   "user-1",
			DeviceID: "device-1",
			Platform: "ios",
			Status:   "finished",
		},
	}

	mock.ExpectExec(regexp.QuoteMeta(insertPerfTaskSQL)).
		WithArgs(
			anyPerfTaskID{},
			"user-1",
			"device-1",
			"",
			"",
			"ios",
			nil,
			"finished",
			task.StartTime,
			task.StopTime,
			int64(0),
			0,
			nil,
			"[]",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.Create(context.Background(), task); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	assertPerfTaskExpectations(t, mock)
}

func TestGetReturnsFullRecord(t *testing.T) {
	db, mock := newPerfTaskMockDB(t)
	store := NewSQLStore(db)
	startTime := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	stopTime := startTime.Add(time.Minute)
	createdAt := stopTime.Add(time.Second)
	samples := `[{"cpu":1.2}]`

	mock.ExpectQuery(regexp.QuoteMeta(getPerfTaskSQL)).
		WithArgs("task-1").
		WillReturnRows(sqlmock.NewRows(perfTaskDetailColumns()).
			AddRow("task-1", "user-1", "device-1", "com.example.app", "com.example.app",
				"android", "Pixel 7", "finished", startTime, stopTime, int64(1000), 1, nil, samples, createdAt))

	got, err := store.Get(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != "task-1" || got.UserID != "user-1" || got.DeviceID != "device-1" {
		t.Fatalf("task metadata = %#v", got)
	}
	if string(got.Samples) != samples {
		t.Fatalf("samples = %s, want %s", got.Samples, samples)
	}
	assertPerfTaskExpectations(t, mock)
}

func TestGetReturnsNotFound(t *testing.T) {
	db, mock := newPerfTaskMockDB(t)
	store := NewSQLStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(getPerfTaskSQL)).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want %v", err, ErrNotFound)
	}
	assertPerfTaskExpectations(t, mock)
}

func TestListMapsRowsWithoutSamples(t *testing.T) {
	db, mock := newPerfTaskMockDB(t)
	store := NewSQLStore(db)
	firstStart := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	secondStart := firstStart.Add(-time.Hour)
	createdAt := firstStart.Add(time.Minute)

	mock.ExpectQuery(regexp.QuoteMeta(listPerfTaskSQL)).
		WillReturnRows(sqlmock.NewRows(perfTaskSummaryColumns()).
			AddRow("task-2", "user-1", "device-1", "com.example.app", "com.example.app",
				"android", "Pixel 7", "finished", firstStart, firstStart.Add(time.Minute), int64(1000), 1, nil, createdAt).
			AddRow("task-1", "user-1", "device-2", "com.example.other", "com.example.other",
				"ios", nil, "interrupted", secondStart, secondStart.Add(time.Minute), int64(500), 3, "device disconnected", createdAt))

	got, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(got))
	}
	if got[0].ID != "task-2" || got[0].DeviceModel != "Pixel 7" {
		t.Fatalf("first summary = %#v", got[0])
	}
	if got[1].ID != "task-1" || got[1].DeviceModel != "" || got[1].LastError != "device disconnected" {
		t.Fatalf("second summary = %#v", got[1])
	}
	assertPerfTaskExpectations(t, mock)
}

func TestListReturnsEmptySliceForNoRows(t *testing.T) {
	db, mock := newPerfTaskMockDB(t)
	store := NewSQLStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(listPerfTaskSQL)).
		WillReturnRows(sqlmock.NewRows(perfTaskSummaryColumns()))

	got, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got == nil {
		t.Fatal("List() = nil, want empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len(List()) = %d, want 0", len(got))
	}
	assertPerfTaskExpectations(t, mock)
}

func TestDeleteRemovesRow(t *testing.T) {
	db, mock := newPerfTaskMockDB(t)
	store := NewSQLStore(db)

	mock.ExpectExec(regexp.QuoteMeta(deletePerfTaskSQL)).
		WithArgs("task-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Delete(context.Background(), "task-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	assertPerfTaskExpectations(t, mock)
}

func TestDeleteReturnsNotFoundWhenNoRowsAffected(t *testing.T) {
	db, mock := newPerfTaskMockDB(t)
	store := NewSQLStore(db)

	mock.ExpectExec(regexp.QuoteMeta(deletePerfTaskSQL)).
		WithArgs("missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want %v", err, ErrNotFound)
	}
	assertPerfTaskExpectations(t, mock)
}

type anyPerfTaskID struct{}

func (anyPerfTaskID) Match(value driver.Value) bool {
	id, ok := value.(string)
	return ok && id != ""
}

func newPerfTaskMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
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

func perfTaskDetailColumns() []string {
	return []string{
		"id", "user_id", "device_id", "package_name", "process_name", "platform", "device_model",
		"status", "start_time", "stop_time", "sample_interval_ms", "sample_count", "last_error", "samples", "created_at",
	}
}

func perfTaskSummaryColumns() []string {
	return []string{
		"id", "user_id", "device_id", "package_name", "process_name", "platform", "device_model",
		"status", "start_time", "stop_time", "sample_interval_ms", "sample_count", "last_error", "created_at",
	}
}

func assertPerfTaskExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
