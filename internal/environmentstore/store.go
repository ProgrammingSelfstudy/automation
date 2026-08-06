package environmentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

const (
	insertEnvironmentSQL = "INSERT INTO environment (id, name, variables, default_headers) VALUES (?, ?, ?, ?)"
	getEnvironmentSQL    = "SELECT id, name, variables, default_headers, created_at FROM environment WHERE id = ?"
	listEnvironmentsSQL  = "SELECT id, name, variables, default_headers, created_at FROM environment ORDER BY created_at DESC"
	updateEnvironmentSQL = "UPDATE environment SET name = ?, variables = ?, default_headers = ? WHERE id = ?"
)

var (
	ErrNameRequired = errors.New("environment name is required")
	ErrNotFound     = errors.New("environment not found")
)

// Environment is a named set of variables for previewing interface templates.
type Environment struct {
	ID             string
	Name           string
	Variables      map[string]string
	DefaultHeaders map[string]string
	CreatedAt      time.Time
}

// Store persists environments.
type Store interface {
	Create(ctx context.Context, env *Environment) error
	Get(ctx context.Context, id string) (*Environment, error)
	List(ctx context.Context) ([]Environment, error)
	Update(ctx context.Context, env *Environment) error
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

// Create validates and stores an environment.
func (s *SQLStore) Create(ctx context.Context, env *Environment) error {
	variables, defaultHeaders, err := marshalEnvironmentMaps(env)
	if err != nil {
		return err
	}
	if env.ID == "" {
		env.ID = uuid.NewString()
	}

	_, err = s.db.ExecContext(ctx, insertEnvironmentSQL, env.ID, env.Name, string(variables), string(defaultHeaders))
	return err
}

// Get returns one environment by ID.
func (s *SQLStore) Get(ctx context.Context, id string) (*Environment, error) {
	env, err := scanEnvironment(s.db.QueryRowContext(ctx, getEnvironmentSQL, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &env, nil
}

// List returns saved environments in reverse creation order.
func (s *SQLStore) List(ctx context.Context) ([]Environment, error) {
	rows, err := s.db.QueryContext(ctx, listEnvironmentsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	environments := make([]Environment, 0)
	for rows.Next() {
		env, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		environments = append(environments, env)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return environments, nil
}

// Update replaces an environment's name and variable map.
func (s *SQLStore) Update(ctx context.Context, env *Environment) error {
	variables, defaultHeaders, err := marshalEnvironmentMaps(env)
	if err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, updateEnvironmentSQL, env.Name, string(variables), string(defaultHeaders), env.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func marshalEnvironmentMaps(env *Environment) (variables []byte, defaultHeaders []byte, err error) {
	if strings.TrimSpace(env.Name) == "" {
		return nil, nil, ErrNameRequired
	}
	if env.Variables == nil {
		env.Variables = map[string]string{}
	}
	if env.DefaultHeaders == nil {
		env.DefaultHeaders = map[string]string{}
	}
	variables, err = json.Marshal(env.Variables)
	if err != nil {
		return nil, nil, err
	}
	defaultHeaders, err = json.Marshal(env.DefaultHeaders)
	if err != nil {
		return nil, nil, err
	}
	return variables, defaultHeaders, nil
}

type environmentScanner interface {
	Scan(dest ...any) error
}

func scanEnvironment(scanner environmentScanner) (Environment, error) {
	var env Environment
	var variables []byte
	var defaultHeaders []byte

	if err := scanner.Scan(&env.ID, &env.Name, &variables, &defaultHeaders, &env.CreatedAt); err != nil {
		return Environment{}, err
	}
	if err := unmarshalStringMap(variables, &env.Variables); err != nil {
		return Environment{}, err
	}
	if err := unmarshalStringMap(defaultHeaders, &env.DefaultHeaders); err != nil {
		return Environment{}, err
	}

	return env, nil
}

func unmarshalStringMap(data []byte, target *map[string]string) error {
	if len(data) == 0 {
		*target = map[string]string{}
		return nil
	}
	if err := json.Unmarshal(data, target); err != nil {
		return err
	}
	if *target == nil {
		*target = map[string]string{}
	}
	return nil
}
