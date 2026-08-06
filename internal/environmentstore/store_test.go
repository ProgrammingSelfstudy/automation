package environmentstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateValidatesBeforeSQL(t *testing.T) {
	db, mock := newEnvironmentMockDB(t)
	store := NewSQLStore(db)
	env := Environment{
		Variables:      map[string]string{"BASE_URL": "https://api.example.test"},
		DefaultHeaders: map[string]string{"X-App": "dev"},
	}

	err := store.Create(context.Background(), &env)
	if !errors.Is(err, ErrNameRequired) {
		t.Fatalf("Create() error = %v, want %v", err, ErrNameRequired)
	}
	assertEnvironmentExpectations(t, mock)
}

func TestCreateGeneratesIDAndStoresVariables(t *testing.T) {
	db, mock := newEnvironmentMockDB(t)
	store := NewSQLStore(db)
	env := &Environment{
		Name:           "dev",
		Variables:      map[string]string{"BASE_URL": "https://dev.example.test"},
		DefaultHeaders: map[string]string{"X-App": "dev"},
	}

	mock.ExpectExec(regexp.QuoteMeta(insertEnvironmentSQL)).
		WithArgs(
			anyEnvironmentID{},
			"dev",
			stringMapJSONArg{t: t, label: "variables", want: env.Variables},
			stringMapJSONArg{t: t, label: "default_headers", want: env.DefaultHeaders},
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.Create(context.Background(), env); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if env.ID == "" {
		t.Fatal("Create() did not generate environment ID")
	}
	assertEnvironmentExpectations(t, mock)
}

func TestListMapsRowsWithVariables(t *testing.T) {
	db, mock := newEnvironmentMockDB(t)
	store := NewSQLStore(db)
	firstCreatedAt := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	secondCreatedAt := firstCreatedAt.Add(-time.Minute)
	firstVariables := map[string]string{"BASE_URL": "https://dev.example.test", "TOKEN": "dev-token"}
	secondVariables := map[string]string{"BASE_URL": "https://prod.example.test"}
	firstHeaders := map[string]string{"Authorization": "Bearer {{.env.TOKEN}}"}
	secondHeaders := map[string]string{"X-Env": "prod"}

	mock.ExpectQuery(regexp.QuoteMeta(listEnvironmentsSQL)).
		WillReturnRows(sqlmock.NewRows(environmentColumns()).
			AddRow("env-2", "dev", mustMarshalStringMap(t, firstVariables), mustMarshalStringMap(t, firstHeaders), firstCreatedAt).
			AddRow("env-1", "prod", mustMarshalStringMap(t, secondVariables), mustMarshalStringMap(t, secondHeaders), secondCreatedAt))

	got, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(got))
	}
	if got[0].ID != "env-2" || got[0].Name != "dev" || !got[0].CreatedAt.Equal(firstCreatedAt) {
		t.Fatalf("first environment metadata = %#v", got[0])
	}
	if !reflect.DeepEqual(got[0].Variables, firstVariables) {
		t.Fatalf("first variables = %#v, want %#v", got[0].Variables, firstVariables)
	}
	if !reflect.DeepEqual(got[0].DefaultHeaders, firstHeaders) {
		t.Fatalf("first default headers = %#v, want %#v", got[0].DefaultHeaders, firstHeaders)
	}
	if got[1].ID != "env-1" || got[1].Name != "prod" || !got[1].CreatedAt.Equal(secondCreatedAt) {
		t.Fatalf("second environment metadata = %#v", got[1])
	}
	if !reflect.DeepEqual(got[1].Variables, secondVariables) {
		t.Fatalf("second variables = %#v, want %#v", got[1].Variables, secondVariables)
	}
	if !reflect.DeepEqual(got[1].DefaultHeaders, secondHeaders) {
		t.Fatalf("second default headers = %#v, want %#v", got[1].DefaultHeaders, secondHeaders)
	}
	assertEnvironmentExpectations(t, mock)
}

func TestGetMapsRowWithVariablesAndDefaultHeaders(t *testing.T) {
	db, mock := newEnvironmentMockDB(t)
	store := NewSQLStore(db)
	createdAt := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	variables := map[string]string{"BASE_URL": "https://dev.example.test"}
	headers := map[string]string{"X-Env": "{{.env.BASE_URL}}"}

	mock.ExpectQuery(regexp.QuoteMeta(getEnvironmentSQL)).
		WithArgs("env-1").
		WillReturnRows(sqlmock.NewRows(environmentColumns()).
			AddRow("env-1", "dev", mustMarshalStringMap(t, variables), mustMarshalStringMap(t, headers), createdAt))

	got, err := store.Get(context.Background(), "env-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != "env-1" || got.Name != "dev" || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("environment metadata = %#v", got)
	}
	if !reflect.DeepEqual(got.Variables, variables) {
		t.Fatalf("variables = %#v, want %#v", got.Variables, variables)
	}
	if !reflect.DeepEqual(got.DefaultHeaders, headers) {
		t.Fatalf("default headers = %#v, want %#v", got.DefaultHeaders, headers)
	}
	assertEnvironmentExpectations(t, mock)
}

func TestGetReturnsNotFound(t *testing.T) {
	db, mock := newEnvironmentMockDB(t)
	store := NewSQLStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(getEnvironmentSQL)).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want %v", err, ErrNotFound)
	}
	assertEnvironmentExpectations(t, mock)
}

func TestListReturnsEmptySliceForNoRows(t *testing.T) {
	db, mock := newEnvironmentMockDB(t)
	store := NewSQLStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(listEnvironmentsSQL)).
		WillReturnRows(sqlmock.NewRows(environmentColumns()))

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
	assertEnvironmentExpectations(t, mock)
}

func TestUpdateStoresVariables(t *testing.T) {
	db, mock := newEnvironmentMockDB(t)
	store := NewSQLStore(db)
	env := &Environment{
		ID:             "env-1",
		Name:           "dev updated",
		Variables:      map[string]string{"BASE_URL": "https://dev2.example.test"},
		DefaultHeaders: map[string]string{"X-App": "dev2"},
	}

	mock.ExpectExec(regexp.QuoteMeta(updateEnvironmentSQL)).
		WithArgs(
			"dev updated",
			stringMapJSONArg{t: t, label: "variables", want: env.Variables},
			stringMapJSONArg{t: t, label: "default_headers", want: env.DefaultHeaders},
			"env-1",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Update(context.Background(), env); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	assertEnvironmentExpectations(t, mock)
}

func TestUpdateReturnsNotFoundWhenNoRowsAffected(t *testing.T) {
	db, mock := newEnvironmentMockDB(t)
	store := NewSQLStore(db)
	env := &Environment{
		ID:             "missing",
		Name:           "dev updated",
		Variables:      map[string]string{"BASE_URL": "https://dev2.example.test"},
		DefaultHeaders: map[string]string{"X-App": "dev2"},
	}

	mock.ExpectExec(regexp.QuoteMeta(updateEnvironmentSQL)).
		WithArgs(
			"dev updated",
			stringMapJSONArg{t: t, label: "variables", want: env.Variables},
			stringMapJSONArg{t: t, label: "default_headers", want: env.DefaultHeaders},
			"missing",
		).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.Update(context.Background(), env)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update() error = %v, want %v", err, ErrNotFound)
	}
	assertEnvironmentExpectations(t, mock)
}

type anyEnvironmentID struct{}

func (anyEnvironmentID) Match(value driver.Value) bool {
	id, ok := value.(string)
	return ok && id != ""
}

type stringMapJSONArg struct {
	t     *testing.T
	label string
	want  map[string]string
}

func (a stringMapJSONArg) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		a.t.Logf("%s arg has type %T, want string", a.label, value)
		return false
	}

	var got map[string]string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		a.t.Logf("%s arg is invalid JSON: %v", a.label, err)
		return false
	}
	if !reflect.DeepEqual(got, a.want) {
		a.t.Logf("%s = %#v, want %#v", a.label, got, a.want)
		return false
	}
	return true
}

func mustMarshalStringMap(t *testing.T, value map[string]string) []byte {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return data
}

func newEnvironmentMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
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

func environmentColumns() []string {
	return []string{"id", "name", "variables", "default_headers", "created_at"}
}

func assertEnvironmentExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
