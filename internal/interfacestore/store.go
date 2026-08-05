package interfacestore

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
	insertInterfaceSQL = `INSERT INTO api_interface (id, name, definition) VALUES (?, ?, ?)`
	listInterfacesSQL  = `SELECT id, name, definition, created_at FROM api_interface ORDER BY created_at DESC`
)

var (
	ErrNameRequired   = errors.New("interface name is required")
	ErrMethodRequired = errors.New("method is required")
	ErrURLRequired    = errors.New("url is required")
)

// Interface is a reusable HTTP interface definition.
type Interface struct {
	ID        string
	Name      string
	Step      scenario.Step
	CreatedAt time.Time
}

// Store persists reusable interface definitions.
type Store interface {
	Create(ctx context.Context, i *Interface) error
	List(ctx context.Context) ([]Interface, error)
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

// Create validates and stores an interface definition.
func (s *SQLStore) Create(ctx context.Context, iface *Interface) error {
	if strings.TrimSpace(iface.Name) == "" {
		return ErrNameRequired
	}
	if strings.TrimSpace(iface.Step.Method) == "" {
		return ErrMethodRequired
	}
	if strings.TrimSpace(iface.Step.URL) == "" {
		return ErrURLRequired
	}
	if iface.ID == "" {
		iface.ID = uuid.NewString()
	}

	definition, err := json.Marshal(iface.Step)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, insertInterfaceSQL, iface.ID, iface.Name, string(definition))
	return err
}

// List returns saved interfaces in reverse creation order.
func (s *SQLStore) List(ctx context.Context) ([]Interface, error) {
	rows, err := s.db.QueryContext(ctx, listInterfacesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	interfaces := make([]Interface, 0)
	for rows.Next() {
		iface, err := scanInterface(rows)
		if err != nil {
			return nil, err
		}
		interfaces = append(interfaces, iface)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return interfaces, nil
}

type interfaceScanner interface {
	Scan(dest ...any) error
}

func scanInterface(scanner interfaceScanner) (Interface, error) {
	var iface Interface
	var definition []byte

	if err := scanner.Scan(&iface.ID, &iface.Name, &definition, &iface.CreatedAt); err != nil {
		return Interface{}, err
	}
	if err := json.Unmarshal(definition, &iface.Step); err != nil {
		return Interface{}, err
	}

	return iface, nil
}
