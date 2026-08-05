package accountstore

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

const (
	aes256KeySize = 32
	maxGroupIDLen = 36 // matches account.group_id VARCHAR(36) in schema.sql

	insertAccountSQL = `INSERT INTO account (id, group_id, username, password, extra, enabled) VALUES (?, ?, ?, ?, ?, ?)`
	getAccountSQL    = `SELECT id, group_id, username, password, extra, enabled, created_at FROM account WHERE id = ?`
	listByGroupSQL   = `SELECT id, group_id, username, password, extra, enabled, created_at FROM account WHERE group_id = ? ORDER BY created_at ASC, id ASC`
)

var (
	ErrNotFound         = errors.New("account not found")
	ErrGroupIDRequired  = errors.New("group id is required")
	ErrGroupIDTooLong   = fmt.Errorf("group id must be %d characters or fewer", maxGroupIDLen)
	ErrUsernameRequired = errors.New("username is required")
	ErrPasswordRequired = errors.New("password is required")
	ErrInvalidKeyLength = errors.New("account encryption key must be 32 bytes")
	ErrDecryptPassword  = errors.New("decrypt account password")
)

// Account is the persisted account model.
//
// Password is plaintext in Go memory. API layers must redact it before returning
// Account values to clients.
type Account struct {
	ID        string
	GroupID   string
	Username  string
	Password  string
	Extra     json.RawMessage
	Enabled   bool
	CreatedAt time.Time
}

// Store persists accounts and returns decrypted passwords.
type Store interface {
	Create(ctx context.Context, acc *Account) error
	Get(ctx context.Context, id string) (*Account, error)
	ListByGroup(ctx context.Context, groupID string) ([]Account, error)
}

// SQLStore is a database/sql backed Store using AES-256-GCM for passwords.
//
// Callers should load a base64-encoded 32-byte key from configuration, for
// example ACCOUNT_ENCRYPTION_KEY, decode it, and pass the raw bytes here.
type SQLStore struct {
	db   *sql.DB
	aead cipher.AEAD
}

// NewSQLStore builds a SQLStore from an opened database and a raw 32-byte key.
func NewSQLStore(db *sql.DB, key []byte) (*SQLStore, error) {
	if len(key) != aes256KeySize {
		return nil, ErrInvalidKeyLength
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &SQLStore{db: db, aead: aead}, nil
}

// NewMySQLStore opens a MySQL connection, pings it, and validates the key.
func NewMySQLStore(dsn string, key []byte) (*SQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	store, err := NewSQLStore(db, key)
	if err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

// Create inserts an account, encrypting Password before it reaches the DB.
func (s *SQLStore) Create(ctx context.Context, acc *Account) error {
	if acc.GroupID == "" {
		return ErrGroupIDRequired
	}
	if len([]rune(acc.GroupID)) > maxGroupIDLen {
		return ErrGroupIDTooLong
	}
	if acc.Username == "" {
		return ErrUsernameRequired
	}
	if acc.Password == "" {
		return ErrPasswordRequired
	}
	if acc.ID == "" {
		acc.ID = uuid.NewString()
	}

	encryptedPassword, err := s.encryptPassword(acc.Password)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(
		ctx,
		insertAccountSQL,
		acc.ID,
		acc.GroupID,
		acc.Username,
		encryptedPassword,
		extraSQLValue(acc.Extra),
		boolToTinyInt(acc.Enabled),
	)
	return err
}

// Get returns one account by ID with Password decrypted.
func (s *SQLStore) Get(ctx context.Context, id string) (*Account, error) {
	row := s.db.QueryRowContext(ctx, getAccountSQL, id)
	acc, err := s.scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return acc, nil
}

// ListByGroup returns all accounts in stable creation order, including disabled ones.
func (s *SQLStore) ListByGroup(ctx context.Context, groupID string) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, listByGroupSQL, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]Account, 0)
	for rows.Next() {
		acc, err := s.scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, *acc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

type accountScanner interface {
	Scan(dest ...any) error
}

func (s *SQLStore) scanAccount(scanner accountScanner) (*Account, error) {
	var acc Account
	var encryptedPassword string
	var extra sql.NullString
	var enabled int

	err := scanner.Scan(
		&acc.ID,
		&acc.GroupID,
		&acc.Username,
		&encryptedPassword,
		&extra,
		&enabled,
		&acc.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	password, err := s.decryptPassword(encryptedPassword)
	if err != nil {
		return nil, err
	}
	acc.Password = password
	if extra.Valid {
		acc.Extra = append(json.RawMessage(nil), extra.String...)
	}
	acc.Enabled = enabled != 0

	return &acc, nil
}

func (s *SQLStore) encryptPassword(password string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := s.aead.Seal(nil, nonce, []byte(password), nil)
	payload := make([]byte, 0, len(nonce)+len(ciphertext))
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)

	return base64.StdEncoding.EncodeToString(payload), nil
}

func (s *SQLStore) decryptPassword(encryptedPassword string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(encryptedPassword)
	if err != nil {
		return "", fmt.Errorf("%w: invalid base64: %v", ErrDecryptPassword, err)
	}

	nonceSize := s.aead.NonceSize()
	if len(payload) < nonceSize {
		return "", fmt.Errorf("%w: ciphertext is too short", ErrDecryptPassword)
	}

	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDecryptPassword, err)
	}

	return string(plaintext), nil
}

func extraSQLValue(extra json.RawMessage) any {
	if extra == nil {
		return nil
	}
	return string(extra)
}

func boolToTinyInt(enabled bool) int {
	if enabled {
		return 1
	}
	return 0
}
