package scenariostore

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
		name     string
		scenario Scenario
		wantErr  error
	}{
		{
			name: "name",
			scenario: Scenario{
				Definition: scenario.Scenario{Steps: []scenario.Step{{Name: "login", Method: "GET", URL: "https://example.test"}}},
			},
			wantErr: ErrNameRequired,
		},
		{
			name: "steps",
			scenario: Scenario{
				Name:       "login flow",
				Definition: scenario.Scenario{},
			},
			wantErr: ErrStepsRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := newScenarioMockDB(t)
			store := NewSQLStore(db)

			err := store.Create(context.Background(), &tt.scenario)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
			}
			assertScenarioExpectations(t, mock)
		})
	}
}

func TestCreateGeneratesIDAndStoresDefinition(t *testing.T) {
	db, mock := newScenarioMockDB(t)
	store := NewSQLStore(db)
	scen := &Scenario{
		Name:       "checkout",
		Definition: testDefinition(),
	}

	mock.ExpectExec(regexp.QuoteMeta(insertScenarioSQL)).
		WithArgs(anyScenarioID{}, "checkout", scenarioDefinitionArg{t: t, want: scen.Definition}).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.Create(context.Background(), scen); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if scen.ID == "" {
		t.Fatal("Create() did not generate scenario ID")
	}
	assertScenarioExpectations(t, mock)
}

func TestListMapsRowsWithDefinition(t *testing.T) {
	db, mock := newScenarioMockDB(t)
	store := NewSQLStore(db)
	firstCreatedAt := time.Date(2026, 8, 5, 14, 0, 0, 0, time.UTC)
	secondCreatedAt := firstCreatedAt.Add(-time.Minute)
	firstDefinition := testDefinition()
	secondDefinition := scenario.Scenario{
		Steps: []scenario.Step{
			{Name: "ping", Method: "GET", URL: "https://api.example.test/ping"},
		},
		Formula: "ok",
	}

	mock.ExpectQuery(regexp.QuoteMeta(listScenariosSQL)).
		WillReturnRows(sqlmock.NewRows(scenarioColumns()).
			AddRow("scen-2", "checkout", mustMarshalDefinition(t, firstDefinition), firstCreatedAt).
			AddRow("scen-1", "ping", mustMarshalDefinition(t, secondDefinition), secondCreatedAt))

	got, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(got))
	}
	if got[0].ID != "scen-2" || got[0].Name != "checkout" || !got[0].CreatedAt.Equal(firstCreatedAt) {
		t.Fatalf("first scenario metadata = %#v", got[0])
	}
	if !reflect.DeepEqual(got[0].Definition, firstDefinition) {
		t.Fatalf("first definition = %#v, want %#v", got[0].Definition, firstDefinition)
	}
	if got[1].ID != "scen-1" || got[1].Name != "ping" || !got[1].CreatedAt.Equal(secondCreatedAt) {
		t.Fatalf("second scenario metadata = %#v", got[1])
	}
	if !reflect.DeepEqual(got[1].Definition, secondDefinition) {
		t.Fatalf("second definition = %#v, want %#v", got[1].Definition, secondDefinition)
	}
	assertScenarioExpectations(t, mock)
}

func TestListReturnsEmptySliceForNoRows(t *testing.T) {
	db, mock := newScenarioMockDB(t)
	store := NewSQLStore(db)

	mock.ExpectQuery(regexp.QuoteMeta(listScenariosSQL)).
		WillReturnRows(sqlmock.NewRows(scenarioColumns()))

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
	assertScenarioExpectations(t, mock)
}

type anyScenarioID struct{}

func (anyScenarioID) Match(value driver.Value) bool {
	id, ok := value.(string)
	return ok && id != ""
}

type scenarioDefinitionArg struct {
	t    *testing.T
	want scenario.Scenario
}

func (a scenarioDefinitionArg) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		a.t.Logf("definition arg has type %T, want string", value)
		return false
	}

	var got scenario.Scenario
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		a.t.Logf("definition arg is invalid JSON: %v", err)
		return false
	}
	if !reflect.DeepEqual(got, a.want) {
		a.t.Logf("definition = %#v, want %#v", got, a.want)
		return false
	}
	return true
}

func testDefinition() scenario.Scenario {
	return scenario.Scenario{
		Steps: []scenario.Step{
			{
				Name:    "login",
				Method:  "POST",
				URL:     "https://api.example.test/login",
				BodyTpl: `{"username":"{{.Account.Username}}"}`,
				Headers: map[string]string{"Content-Type": "application/json"},
				Extract: map[string]string{"token": "data.token"},
			},
			{
				Name:    "profile",
				Method:  "GET",
				URL:     "https://api.example.test/profile",
				Headers: map[string]string{"Authorization": "Bearer {{.token}}"},
				Extract: map[string]string{"score": "data.score"},
			},
		},
		Formula: "score * 2",
	}
}

func mustMarshalDefinition(t *testing.T, definition scenario.Scenario) []byte {
	t.Helper()

	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return data
}

func newScenarioMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
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

func scenarioColumns() []string {
	return []string{"id", "name", "definition", "created_at"}
}

func assertScenarioExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
