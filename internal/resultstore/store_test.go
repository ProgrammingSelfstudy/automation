package resultstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"interface-load-test/internal/accountpool"
	"interface-load-test/internal/scenario"
)

func TestSaveExecutesInsertWithExpectedArguments(t *testing.T) {
	db, mock := newMockDB(t)
	store := New(db)
	acc := &accountpool.Account{ID: "acc-1", Username: "alice"}
	result := scenario.Result{
		Success:       true,
		FormulaResult: 42.5,
		ErrMsg:        "",
		CostMs:        123,
		Steps: []scenario.StepLog{
			{
				Name:     "login",
				Request:  `{"username":"alice"}`,
				Response: `{"ok":true}`,
				CostMs:   12,
				Success:  true,
			},
		},
	}
	stepsJSON := mustMarshalSteps(t, result.Steps)

	mock.ExpectExec(regexp.QuoteMeta(insertTaskResultSQL)).
		WithArgs("task-1", "acc-1", "alice", 7, stepsJSON, 42.5, 1, "", int64(123)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.Save(context.Background(), "task-1", acc, 7, result); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	assertExpectations(t, mock)
}

func TestSaveReturnsExecError(t *testing.T) {
	db, mock := newMockDB(t)
	store := New(db)
	wantErr := errors.New("insert failed")

	mock.ExpectExec(regexp.QuoteMeta(insertTaskResultSQL)).
		WithArgs("task-1", "acc-1", "alice", 1, "[]", 0.0, 0, "failed", int64(9)).
		WillReturnError(wantErr)

	err := store.Save(context.Background(), "task-1", &accountpool.Account{ID: "acc-1", Username: "alice"}, 1, scenario.Result{
		Success: false,
		ErrMsg:  "failed",
		CostMs:  9,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Save() error = %v, want %v", err, wantErr)
	}
	assertExpectations(t, mock)
}

func TestSaveTruncatesDefensiveStringFields(t *testing.T) {
	db, mock := newMockDB(t)
	store := New(db)
	accountName := strings.Repeat("名", maxAccountNameRunes+5)
	errMsg := strings.Repeat("错", maxErrMsgBytes)

	mock.ExpectExec(regexp.QuoteMeta(insertTaskResultSQL)).
		WithArgs("task-1", "acc-1", truncateRunes(accountName, maxAccountNameRunes), 1, "[]", 0.0, 0, truncateUTF8Bytes(errMsg, maxErrMsgBytes), int64(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.Save(context.Background(), "task-1", &accountpool.Account{ID: "acc-1", Username: accountName}, 1, scenario.Result{
		ErrMsg: errMsg,
		CostMs: 1,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	assertExpectations(t, mock)
}

func TestListByTaskGroupedByAccountReturnsDeterministicGroups(t *testing.T) {
	db, mock := newMockDB(t)
	store := New(db)
	createdAt := time.Date(2026, 8, 4, 13, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows(resultColumns()).
		AddRow(int64(1), "task-1", "acc-1", "alice", 1, []byte(`[{"Name":"s1"}]`), 10.5, 1, "", int64(100), createdAt).
		AddRow(int64(2), "task-1", "acc-1", "alice", 2, []byte(`[{"Name":"s2"}]`), 11.5, 0, "boom", int64(110), createdAt.Add(time.Second)).
		AddRow(int64(3), "task-1", "acc-2", "bob", 1, []byte(`[{"Name":"s1"}]`), 20.5, 1, "", int64(200), createdAt.Add(2*time.Second)).
		AddRow(int64(4), "task-1", "acc-3", "carol", 1, []byte(`[{"Name":"s1"}]`), 30.5, 1, "", int64(300), createdAt.Add(3*time.Second)).
		AddRow(int64(5), "task-1", "acc-3", "carol", 2, []byte(`[{"Name":"s2"}]`), 31.5, 1, "", int64(310), createdAt.Add(4*time.Second))

	mock.ExpectQuery(regexp.QuoteMeta(listByTaskSQL)).
		WithArgs("task-1").
		WillReturnRows(rows)

	groups, err := store.ListByTaskGroupedByAccount(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("ListByTaskGroupedByAccount() error = %v", err)
	}

	if got, want := len(groups), 3; got != want {
		t.Fatalf("len(groups) = %d, want %d", got, want)
	}
	assertGroup(t, groups[0], "acc-1", "alice", []int{1, 2})
	assertGroup(t, groups[1], "acc-2", "bob", []int{1})
	assertGroup(t, groups[2], "acc-3", "carol", []int{1, 2})

	first := groups[0].Rows[0]
	if got, want := first.ID, int64(1); got != want {
		t.Fatalf("first row ID = %d, want %d", got, want)
	}
	if got, want := string(first.Steps), `[{"Name":"s1"}]`; got != want {
		t.Fatalf("first row Steps = %s, want %s", got, want)
	}
	if got, want := first.FormulaResult, 10.5; got != want {
		t.Fatalf("first row FormulaResult = %v, want %v", got, want)
	}
	if !first.Success {
		t.Fatal("first row Success = false, want true")
	}
	if got, want := groups[0].Rows[1].ErrMsg, "boom"; got != want {
		t.Fatalf("second row ErrMsg = %q, want %q", got, want)
	}
	assertExpectations(t, mock)
}

func TestListByTaskGroupedByAccountReturnsEmptySliceForNoRows(t *testing.T) {
	db, mock := newMockDB(t)
	store := New(db)

	mock.ExpectQuery(regexp.QuoteMeta(listByTaskSQL)).
		WithArgs("task-empty").
		WillReturnRows(sqlmock.NewRows(resultColumns()))

	groups, err := store.ListByTaskGroupedByAccount(context.Background(), "task-empty")
	if err != nil {
		t.Fatalf("ListByTaskGroupedByAccount() error = %v", err)
	}
	if groups == nil {
		t.Fatal("groups = nil, want empty slice")
	}
	if got := len(groups); got != 0 {
		t.Fatalf("len(groups) = %d, want 0", got)
	}
	assertExpectations(t, mock)
}

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
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

func resultColumns() []string {
	return []string{
		"id",
		"task_id",
		"account_id",
		"account_name",
		"seq_no",
		"steps",
		"formula_result",
		"success",
		"err_msg",
		"cost_ms",
		"created_at",
	}
}

func mustMarshalSteps(t *testing.T, steps []scenario.StepLog) string {
	t.Helper()

	data, err := json.Marshal(steps)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(data)
}

func assertGroup(t *testing.T, group AccountResults, accountID string, accountName string, seqNos []int) {
	t.Helper()

	if got := group.AccountID; got != accountID {
		t.Fatalf("AccountID = %q, want %q", got, accountID)
	}
	if got := group.AccountName; got != accountName {
		t.Fatalf("AccountName = %q, want %q", got, accountName)
	}
	if got, want := len(group.Rows), len(seqNos); got != want {
		t.Fatalf("len(Rows) = %d, want %d", got, want)
	}
	for i, seqNo := range seqNos {
		if got := group.Rows[i].SeqNo; got != seqNo {
			t.Fatalf("Rows[%d].SeqNo = %d, want %d", i, got, seqNo)
		}
	}
}

func assertExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
