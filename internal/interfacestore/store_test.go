package interfacestore

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

	"interface-load-test/internal/scenario"
)

func TestCreateValidatesBeforeSQL(t *testing.T) {
	tests := []struct {
		name      string
		iface     Interface
		wantError error
	}{
		{
			name: "name",
			iface: Interface{
				Step: scenario.Step{Method: "GET", URL: "https://example.test"},
			},
			wantError: ErrNameRequired,
		},
		{
			name: "method",
			iface: Interface{
				Name: "login",
				Step: scenario.Step{URL: "https://example.test"},
			},
			wantError: ErrMethodRequired,
		},
		{
			name: "url",
			iface: Interface{
				Name: "login",
				Step: scenario.Step{Method: "POST"},
			},
			wantError: ErrURLRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := newInterfaceMockDB(t)
			store := NewSQLStore(db)

			err := store.Create(context.Background(), &tt.iface)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Create() error = %v, want %v", err, tt.wantError)
			}
			assertInterfaceExpectations(t, mock)
		})
	}
}

func TestCreateGeneratesIDAndStoresStep(t *testing.T) {
	db, mock := newInterfaceMockDB(t)
	store := NewSQLStore(db)
	iface := &Interface{
		Name: "login api",
		Step: testStep(),
	}

	mock.ExpectExec(regexp.QuoteMeta(insertInterfaceSQL)).
		WithArgs(anyInterfaceID{}, "login api", interfaceStepArg{t: t, want: iface.Step}).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.Create(context.Background(), iface); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if iface.ID == "" {
		t.Fatal("Create() did not generate interface ID")
	}
	assertInterfaceExpectations(t, mock)
}

func TestListMapsRowsWithStep(t *testing.T) {
	db, mock := newInterfaceMockDB(t)
	store := NewSQLStore(db)
	firstCreatedAt := time.Date(2026, 8, 5, 14, 0, 0, 0, time.UTC)
	secondCreatedAt := firstCreatedAt.Add(-time.Minute)
	firstStep := testStep()
	secondStep := scenario.Step{
		Name:   "profile",
		Method: "GET",
		URL:    "https://api.example.test/profile",
		Headers: map[string]string{
			"Authorization": "Bearer {{.token}}",
		},
		Extract: map[string]string{"score": "data.score"},
	}

	mock.ExpectQuery(regexp.QuoteMeta(listInterfacesSQL)).
		WillReturnRows(sqlmock.NewRows(interfaceColumns()).
			AddRow("iface-2", "login api", mustMarshalStep(t, firstStep), firstCreatedAt).
			AddRow("iface-1", "profile api", mustMarshalStep(t, secondStep), secondCreatedAt))

	got, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(got))
	}
	if got[0].ID != "iface-2" || got[0].Name != "login api" || !got[0].CreatedAt.Equal(firstCreatedAt) {
		t.Fatalf("first interface metadata = %#v", got[0])
	}
	if !reflect.DeepEqual(got[0].Step, firstStep) {
		t.Fatalf("first step = %#v, want %#v", got[0].Step, firstStep)
	}
	if got[1].ID != "iface-1" || got[1].Name != "profile api" || !got[1].CreatedAt.Equal(secondCreatedAt) {
		t.Fatalf("second interface metadata = %#v", got[1])
	}
	if !reflect.DeepEqual(got[1].Step, secondStep) {
		t.Fatalf("second step = %#v, want %#v", got[1].Step, secondStep)
	}
	assertInterfaceExpectations(t, mock)
}

func TestListReturnsEmptySliceForNoRows(t *testing.T) {
	db, mock := newInterfaceMockDB(t)
	store := NewSQLStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(listInterfacesSQL)).
		WillReturnRows(sqlmock.NewRows(interfaceColumns()))

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
	assertInterfaceExpectations(t, mock)
}

type anyInterfaceID struct{}

func (anyInterfaceID) Match(value driver.Value) bool {
	id, ok := value.(string)
	return ok && id != ""
}

type interfaceStepArg struct {
	t    *testing.T
	want scenario.Step
}

func (a interfaceStepArg) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		a.t.Logf("definition arg has type %T, want string", value)
		return false
	}

	var got scenario.Step
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		a.t.Logf("definition arg is invalid JSON: %v", err)
		return false
	}
	if !reflect.DeepEqual(got, a.want) {
		a.t.Logf("step = %#v, want %#v", got, a.want)
		return false
	}
	return true
}

func testStep() scenario.Step {
	return scenario.Step{
		Name:    "login",
		Method:  "POST",
		URL:     "https://api.example.test/login",
		BodyTpl: `{"username":"{{.Account.Username}}","password":"{{.Account.Password}}"}`,
		Headers: map[string]string{"Content-Type": "application/json"},
		Extract: map[string]string{"token": "data.token"},
	}
}

func mustMarshalStep(t *testing.T, step scenario.Step) []byte {
	t.Helper()

	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return data
}

func newInterfaceMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
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

func interfaceColumns() []string {
	return []string{"id", "name", "definition", "created_at"}
}

func assertInterfaceExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
