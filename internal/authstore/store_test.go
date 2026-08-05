package authstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

func TestCreateUserMapsDuplicateUsername(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	user := &User{ID: "user-1", Username: "admin", PasswordHash: "hash"}

	mock.ExpectExec(regexp.QuoteMeta(insertUserSQL)).
		WithArgs("user-1", "admin", "hash", nil, tinyIntFalse).
		WillReturnError(&mysql.MySQLError{Number: mysqlDuplicateEntry, Message: "duplicate"})

	err := store.CreateUser(context.Background(), user)
	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("CreateUser() error = %v, want %v", err, ErrUsernameTaken)
	}
	assertAuthExpectations(t, mock)
}

func TestCreateUserGeneratesIDAndWritesTOTPFields(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	user := &User{Username: "admin", PasswordHash: "hash", TOTPSecret: "secret", TOTPEnabled: true}

	mock.ExpectExec(regexp.QuoteMeta(insertUserSQL)).
		WithArgs(anyNonEmptyAuthString{}, "admin", "hash", "secret", tinyIntTrue).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.CreateUser(context.Background(), user); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if user.ID == "" {
		t.Fatal("CreateUser() did not generate ID")
	}
	assertAuthExpectations(t, mock)
}

func TestGetUserByUsernameAndID(t *testing.T) {
	createdAt := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		query string
		call  func(*SQLStore) (*User, error)
		arg   string
	}{
		{
			name:  "username",
			query: getUserByUsernameSQL,
			call:  func(store *SQLStore) (*User, error) { return store.GetUserByUsername(context.Background(), "admin") },
			arg:   "admin",
		},
		{
			name:  "id",
			query: getUserByIDSQL,
			call:  func(store *SQLStore) (*User, error) { return store.GetUserByID(context.Background(), "user-1") },
			arg:   "user-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := newAuthMockDB(t)
			store := NewSQLStore(db)
			mock.ExpectQuery(regexp.QuoteMeta(tt.query)).
				WithArgs(tt.arg).
				WillReturnRows(sqlmock.NewRows(userColumns()).
					AddRow("user-1", "admin", "hash", "secret", 1, createdAt))

			user, err := tt.call(store)
			if err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			if user.ID != "user-1" || user.Username != "admin" || user.PasswordHash != "hash" || user.TOTPSecret != "secret" || !user.TOTPEnabled {
				t.Fatalf("user = %#v", user)
			}
			if !user.CreatedAt.Equal(createdAt) {
				t.Fatalf("CreatedAt = %v, want %v", user.CreatedAt, createdAt)
			}
			assertAuthExpectations(t, mock)
		})
	}
}

func TestGetUserNotFound(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	mock.ExpectQuery(regexp.QuoteMeta(getUserByUsernameSQL)).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	user, err := store.GetUserByUsername(context.Background(), "missing")
	if user != nil {
		t.Fatalf("user = %#v, want nil", user)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want %v", err, ErrNotFound)
	}
	assertAuthExpectations(t, mock)
}

func TestUpdateTOTP(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	mock.ExpectExec(regexp.QuoteMeta(updateTOTPSQL)).
		WithArgs("secret", tinyIntTrue, "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.UpdateTOTP(context.Background(), "user-1", "secret", true); err != nil {
		t.Fatalf("UpdateTOTP() error = %v", err)
	}
	assertAuthExpectations(t, mock)
}

func TestReplaceBackupCodesUsesTransaction(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(deleteUnusedCodesSQL)).
		WithArgs("user-1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(insertBackupCodeSQL)).
		WithArgs("user-1", "hash-1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(insertBackupCodeSQL)).
		WithArgs("user-1", "hash-2").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	if err := store.ReplaceBackupCodes(context.Background(), "user-1", []string{"hash-1", "hash-2"}); err != nil {
		t.Fatalf("ReplaceBackupCodes() error = %v", err)
	}
	assertAuthExpectations(t, mock)
}

func TestListUnusedBackupCodes(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	mock.ExpectQuery(regexp.QuoteMeta(listUnusedCodesSQL)).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "code_hash"}).
			AddRow(int64(1), "hash-1").
			AddRow(int64(2), "hash-2"))

	codes, err := store.ListUnusedBackupCodes(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListUnusedBackupCodes() error = %v", err)
	}
	if len(codes) != 2 || codes[0].ID != 1 || codes[0].Hash != "hash-1" || codes[1].ID != 2 || codes[1].Hash != "hash-2" {
		t.Fatalf("codes = %#v", codes)
	}
	assertAuthExpectations(t, mock)
}

func TestMarkBackupCodeUsed(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	mock.ExpectExec(regexp.QuoteMeta(markBackupCodeUsedSQL)).
		WithArgs(sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.MarkBackupCodeUsed(context.Background(), 1); err != nil {
		t.Fatalf("MarkBackupCodeUsed() error = %v", err)
	}
	assertAuthExpectations(t, mock)
}

func TestMarkBackupCodeUsedReturnsNotFoundWhenNoRowsAffected(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	mock.ExpectExec(regexp.QuoteMeta(markBackupCodeUsedSQL)).
		WithArgs(sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.MarkBackupCodeUsed(context.Background(), 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("MarkBackupCodeUsed() error = %v, want %v", err, ErrNotFound)
	}
	assertAuthExpectations(t, mock)
}

func TestCreateSessionAndDeleteSession(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	expiresAt := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(insertSessionSQL)).
		WithArgs("session-1", "user-1", expiresAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(deleteSessionSQL)).
		WithArgs("session-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.CreateSession(context.Background(), &Session{ID: "session-1", UserID: "user-1", ExpiresAt: expiresAt}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := store.DeleteSession(context.Background(), "session-1"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	assertAuthExpectations(t, mock)
}

func TestGetSessionFiltersExpiredInSQL(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	mock.ExpectQuery(regexp.QuoteMeta(getSessionSQL)).
		WithArgs("expired").
		WillReturnError(sql.ErrNoRows)

	session, err := store.GetSession(context.Background(), "expired")
	if session != nil {
		t.Fatalf("session = %#v, want nil", session)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want %v", err, ErrNotFound)
	}
	assertAuthExpectations(t, mock)
}

func TestGetSessionMapsRow(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	expiresAt := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(getSessionSQL)).
		WithArgs("session-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "expires_at"}).
			AddRow("session-1", "user-1", expiresAt))

	session, err := store.GetSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if session.ID != "session-1" || session.UserID != "user-1" || !session.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("session = %#v", session)
	}
	assertAuthExpectations(t, mock)
}

func TestRecordFailedLoginAttempt(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	mock.ExpectExec(regexp.QuoteMeta(insertLoginAttemptSQL)).
		WithArgs("203.0.113.10").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.RecordFailedLoginAttempt(context.Background(), "203.0.113.10"); err != nil {
		t.Fatalf("RecordFailedLoginAttempt() error = %v", err)
	}
	assertAuthExpectations(t, mock)
}

func TestCountRecentFailedLoginAttempts(t *testing.T) {
	db, mock := newAuthMockDB(t)
	store := NewSQLStore(db)
	since := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(countLoginAttemptsSQL)).
		WithArgs("203.0.113.10", since).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

	count, err := store.CountRecentFailedLoginAttempts(context.Background(), "203.0.113.10", since)
	if err != nil {
		t.Fatalf("CountRecentFailedLoginAttempts() error = %v", err)
	}
	if count != 4 {
		t.Fatalf("count = %d, want 4", count)
	}
	assertAuthExpectations(t, mock)
}

type anyNonEmptyAuthString struct{}

func (anyNonEmptyAuthString) Match(value driver.Value) bool {
	s, ok := value.(string)
	return ok && s != ""
}

func newAuthMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
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

func userColumns() []string {
	return []string{"id", "username", "password_hash", "totp_secret", "totp_enabled", "created_at"}
}

func assertAuthExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
