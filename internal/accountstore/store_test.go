package accountstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateEncryptsPasswordAndExecutesInsert(t *testing.T) {
	db, mock := newAccountMockDB(t)
	store := newTestStore(t, db, testKey(1))
	acc := &Account{
		GroupID:  "group-1",
		Username: "alice",
		Password: "plain-secret",
		Enabled:  true,
	}
	var encrypted string

	mock.ExpectExec(regexp.QuoteMeta(insertAccountSQL)).
		WithArgs(anyNonEmptyString{}, "group-1", "alice", encryptedPasswordArg{
			t:         t,
			store:     store,
			plaintext: "plain-secret",
			captured:  &encrypted,
		}, nil, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.Create(context.Background(), acc); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if acc.ID == "" {
		t.Fatal("Create() did not generate acc.ID")
	}
	if encrypted == "" || encrypted == acc.Password {
		t.Fatalf("encrypted password = %q, plaintext = %q", encrypted, acc.Password)
	}
	assertAccountExpectations(t, mock)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	store := newTestStore(t, nil, testKey(1))

	encrypted, err := store.encryptPassword("plain-secret")
	if err != nil {
		t.Fatalf("encryptPassword() error = %v", err)
	}
	if encrypted == "plain-secret" {
		t.Fatal("encrypted password equals plaintext")
	}

	decrypted, err := store.decryptPassword(encrypted)
	if err != nil {
		t.Fatalf("decryptPassword() error = %v", err)
	}
	if decrypted != "plain-secret" {
		t.Fatalf("decryptPassword() = %q, want %q", decrypted, "plain-secret")
	}
}

func TestEncryptUsesRandomNonce(t *testing.T) {
	store := newTestStore(t, nil, testKey(1))

	first, err := store.encryptPassword("same-password")
	if err != nil {
		t.Fatalf("first encryptPassword() error = %v", err)
	}
	second, err := store.encryptPassword("same-password")
	if err != nil {
		t.Fatalf("second encryptPassword() error = %v", err)
	}
	if first == second {
		t.Fatal("two encryptions of the same password produced identical ciphertext")
	}
}

func TestGetReturnsDecryptErrorWithWrongKey(t *testing.T) {
	source := newTestStore(t, nil, testKey(1))
	encrypted, err := source.encryptPassword("plain-secret")
	if err != nil {
		t.Fatalf("encryptPassword() error = %v", err)
	}

	db, mock := newAccountMockDB(t)
	store := newTestStore(t, db, testKey(2))
	createdAt := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(getAccountSQL)).
		WithArgs("acc-1").
		WillReturnRows(sqlmock.NewRows(accountColumns()).
			AddRow("acc-1", "group-1", "alice", encrypted, nil, 1, createdAt))

	acc, err := store.Get(context.Background(), "acc-1")
	if acc != nil {
		t.Fatalf("Get() account = %#v, want nil", acc)
	}
	if !errors.Is(err, ErrDecryptPassword) {
		t.Fatalf("Get() error = %v, want %v", err, ErrDecryptPassword)
	}
	assertAccountExpectations(t, mock)
}

func TestNewSQLStoreRejectsInvalidKeyLength(t *testing.T) {
	for _, key := range [][]byte{make([]byte, 16), make([]byte, 33)} {
		store, err := NewSQLStore(nil, key)
		if store != nil {
			t.Fatalf("NewSQLStore(%d-byte key) store is non-nil", len(key))
		}
		if !errors.Is(err, ErrInvalidKeyLength) {
			t.Fatalf("NewSQLStore(%d-byte key) error = %v, want %v", len(key), err, ErrInvalidKeyLength)
		}
	}
}

func TestCreateValidatesRequiredFieldsBeforeSQL(t *testing.T) {
	tests := []struct {
		name    string
		account Account
		wantErr error
	}{
		{
			name:    "group id",
			account: Account{Username: "alice", Password: "secret"},
			wantErr: ErrGroupIDRequired,
		},
		{
			name:    "username",
			account: Account{GroupID: "group-1", Password: "secret"},
			wantErr: ErrUsernameRequired,
		},
		{
			name:    "password",
			account: Account{GroupID: "group-1", Username: "alice"},
			wantErr: ErrPasswordRequired,
		},
		{
			name:    "group id too long",
			account: Account{GroupID: strings.Repeat("a", maxGroupIDLen+1), Username: "alice", Password: "secret"},
			wantErr: ErrGroupIDTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := newAccountMockDB(t)
			store := newTestStore(t, db, testKey(1))

			err := store.Create(context.Background(), &tt.account)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
			}
			assertAccountExpectations(t, mock)
		})
	}
}

func TestGetNotFound(t *testing.T) {
	db, mock := newAccountMockDB(t)
	store := newTestStore(t, db, testKey(1))

	mock.ExpectQuery(regexp.QuoteMeta(getAccountSQL)).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	acc, err := store.Get(context.Background(), "missing")
	if acc != nil {
		t.Fatalf("Get() account = %#v, want nil", acc)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want %v", err, ErrNotFound)
	}
	assertAccountExpectations(t, mock)
}

func TestListByGroupMapsRowsInStableOrder(t *testing.T) {
	db, mock := newAccountMockDB(t)
	store := newTestStore(t, db, testKey(1))
	firstPassword := encryptForTest(t, store, "secret-1")
	secondPassword := encryptForTest(t, store, "secret-2")
	createdAt := time.Date(2026, 8, 5, 11, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(listByGroupSQL)).
		WithArgs("group-1").
		WillReturnRows(sqlmock.NewRows(accountColumns()).
			AddRow("acc-1", "group-1", "alice", firstPassword, `{"role":"admin"}`, 1, createdAt).
			AddRow("acc-2", "group-1", "bob", secondPassword, nil, 0, createdAt.Add(time.Second)))

	accounts, err := store.ListByGroup(context.Background(), "group-1")
	if err != nil {
		t.Fatalf("ListByGroup() error = %v", err)
	}
	if got, want := len(accounts), 2; got != want {
		t.Fatalf("len(accounts) = %d, want %d", got, want)
	}

	if accounts[0].ID != "acc-1" || accounts[0].Username != "alice" || accounts[0].Password != "secret-1" {
		t.Fatalf("first account mapping = %#v", accounts[0])
	}
	if string(accounts[0].Extra) != `{"role":"admin"}` {
		t.Fatalf("first Extra = %s, want JSON", accounts[0].Extra)
	}
	if !accounts[0].Enabled {
		t.Fatal("first Enabled = false, want true")
	}
	if accounts[1].ID != "acc-2" || accounts[1].Username != "bob" || accounts[1].Password != "secret-2" {
		t.Fatalf("second account mapping = %#v", accounts[1])
	}
	if accounts[1].Extra != nil {
		t.Fatalf("second Extra = %s, want nil", accounts[1].Extra)
	}
	if accounts[1].Enabled {
		t.Fatal("second Enabled = true, want false")
	}
	assertAccountExpectations(t, mock)
}

func TestFilterEnabledKeepsOrder(t *testing.T) {
	accounts := []Account{
		{ID: "acc-1", Enabled: false},
		{ID: "acc-2", Enabled: true},
		{ID: "acc-3", Enabled: true},
		{ID: "acc-4", Enabled: false},
	}

	filtered := FilterEnabled(accounts)
	if got, want := len(filtered), 2; got != want {
		t.Fatalf("len(filtered) = %d, want %d", got, want)
	}
	if filtered[0].ID != "acc-2" || filtered[1].ID != "acc-3" {
		t.Fatalf("filtered order = %#v, want acc-2 then acc-3", filtered)
	}
}

func TestToPoolAccounts(t *testing.T) {
	accounts := []Account{
		{ID: "acc-1", Username: "alice", Password: "secret-1"},
		{ID: "acc-2", Username: "bob", Password: "secret-2"},
	}

	poolAccounts := ToPoolAccounts(accounts)
	if got, want := len(poolAccounts), 2; got != want {
		t.Fatalf("len(poolAccounts) = %d, want %d", got, want)
	}
	for i, acc := range accounts {
		if poolAccounts[i].ID != acc.ID || poolAccounts[i].Username != acc.Username || poolAccounts[i].Password != acc.Password {
			t.Fatalf("poolAccounts[%d] = %#v, want from %#v", i, poolAccounts[i], acc)
		}
	}
}

type encryptedPasswordArg struct {
	t         *testing.T
	store     *SQLStore
	plaintext string
	captured  *string
}

func (a encryptedPasswordArg) Match(value driver.Value) bool {
	encrypted, ok := value.(string)
	if !ok {
		a.t.Logf("password arg has type %T, want string", value)
		return false
	}
	if encrypted == a.plaintext {
		a.t.Log("password arg equals plaintext")
		return false
	}

	decrypted, err := a.store.decryptPassword(encrypted)
	if err != nil {
		a.t.Logf("decrypt password arg: %v", err)
		return false
	}
	if decrypted != a.plaintext {
		a.t.Logf("decrypted password = %q, want %q", decrypted, a.plaintext)
		return false
	}

	*a.captured = encrypted
	return true
}

type anyNonEmptyString struct{}

func (anyNonEmptyString) Match(value driver.Value) bool {
	s, ok := value.(string)
	return ok && s != ""
}

func newAccountMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
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

func newTestStore(t *testing.T, db *sql.DB, key []byte) *SQLStore {
	t.Helper()

	store, err := NewSQLStore(db, key)
	if err != nil {
		t.Fatalf("NewSQLStore() error = %v", err)
	}
	return store
}

func testKey(seed byte) []byte {
	key := make([]byte, aes256KeySize)
	for i := range key {
		key[i] = seed
	}
	return key
}

func encryptForTest(t *testing.T, store *SQLStore, password string) string {
	t.Helper()

	encrypted, err := store.encryptPassword(password)
	if err != nil {
		t.Fatalf("encryptPassword() error = %v", err)
	}
	return encrypted
}

func accountColumns() []string {
	return []string{"id", "group_id", "username", "password", "extra", "enabled", "created_at"}
}

func assertAccountExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestExtraSQLValue(t *testing.T) {
	if got := extraSQLValue(nil); got != nil {
		t.Fatalf("extraSQLValue(nil) = %v, want nil", got)
	}

	extra := json.RawMessage(`{"x":1}`)
	if got, want := extraSQLValue(extra), `{"x":1}`; got != want {
		t.Fatalf("extraSQLValue(JSON) = %v, want %v", got, want)
	}
}
