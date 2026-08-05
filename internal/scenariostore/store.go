package scenariostore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"interface-load-test/internal/scenario"
)

const (
	insertScenarioSQL = `INSERT INTO scenario (id, name, definition) VALUES (?, ?, ?)`
	listScenariosSQL  = `SELECT id, name, definition, created_at FROM scenario ORDER BY created_at DESC`
)

var (
	ErrNameRequired  = errors.New("scenario name is required")
	ErrStepsRequired = errors.New("scenario must contain at least one step")
	ErrNotFound      = errors.New("scenario not found")
)

// Scenario is a saved scenario template.
type Scenario struct {
	ID         string
	Name       string
	Definition scenario.Scenario
	CreatedAt  time.Time
}

// Store persists reusable scenario templates.
type Store interface {
	Create(ctx context.Context, s *Scenario) error
	List(ctx context.Context) ([]Scenario, error)
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

// Create validates and stores a scenario template.
func (s *SQLStore) Create(ctx context.Context, scen *Scenario) error {
	if strings.TrimSpace(scen.Name) == "" {
		return ErrNameRequired
	}
	if len(scen.Definition.Steps) == 0 {
		return ErrStepsRequired
	}
	if scen.ID == "" {
		scen.ID = uuid.NewString()
	}

	definition, err := json.Marshal(scen.Definition)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, insertScenarioSQL, scen.ID, scen.Name, string(definition))
	return err
}

// List returns saved scenarios in reverse creation order.
func (s *SQLStore) List(ctx context.Context) ([]Scenario, error) {
	rows, err := s.db.QueryContext(ctx, listScenariosSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scenarios := make([]Scenario, 0)
	for rows.Next() {
		scen, err := scanScenario(rows)
		if err != nil {
			return nil, err
		}
		scenarios = append(scenarios, scen)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return scenarios, nil
}

type scenarioScanner interface {
	Scan(dest ...any) error
}

func scanScenario(scanner scenarioScanner) (Scenario, error) {
	var scen Scenario
	var definition []byte

	err := scanner.Scan(&scen.ID, &scen.Name, &definition, &scen.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Scenario{}, ErrNotFound
		}
		return Scenario{}, err
	}
	if err := json.Unmarshal(definition, &scen.Definition); err != nil {
		return Scenario{}, err
	}

	return scen, nil
}
