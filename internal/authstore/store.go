package authstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

const (
	mysqlDuplicateEntry = 1062

	insertUserSQL         = "INSERT INTO `user` (id, username, password_hash, totp_secret, totp_enabled) VALUES (?, ?, ?, ?, ?)"
	getUserByUsernameSQL  = "SELECT id, username, password_hash, totp_secret, totp_enabled, created_at FROM `user` WHERE username = ?"
	getUserByIDSQL        = "SELECT id, username, password_hash, totp_secret, totp_enabled, created_at FROM `user` WHERE id = ?"
	updateTOTPSQL         = "UPDATE `user` SET totp_secret = ?, totp_enabled = ? WHERE id = ?"
	deleteUnusedCodesSQL  = "DELETE FROM backup_code WHERE user_id = ? AND used_at IS NULL"
	insertBackupCodeSQL   = "INSERT INTO backup_code (user_id, code_hash) VALUES (?, ?)"
	listUnusedCodesSQL    = "SELECT id, code_hash FROM backup_code WHERE user_id = ? AND used_at IS NULL ORDER BY id ASC"
	markBackupCodeUsedSQL = "UPDATE backup_code SET used_at = ? WHERE id = ? AND used_at IS NULL"
	insertSessionSQL      = "INSERT INTO `session` (id, user_id, expires_at) VALUES (?, ?, ?)"
	getSessionSQL         = "SELECT id, user_id, expires_at FROM `session` WHERE id = ? AND expires_at >= NOW()"
	deleteSessionSQL      = "DELETE FROM `session` WHERE id = ?"
	tinyIntTrue           = 1
	tinyIntFalse          = 0
)

var (
	ErrNotFound      = errors.New("not found")
	ErrUsernameTaken = errors.New("username already exists")
)

type User struct {
	ID           string
	Username     string
	PasswordHash string
	TOTPSecret   string
	TOTPEnabled  bool
	CreatedAt    time.Time
}

type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
}

type BackupCodeRef struct {
	ID   int64
	Hash string
}

type Store interface {
	CreateUser(ctx context.Context, u *User) error
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	UpdateTOTP(ctx context.Context, userID, secret string, enabled bool) error
	ReplaceBackupCodes(ctx context.Context, userID string, hashes []string) error
	ListUnusedBackupCodes(ctx context.Context, userID string) ([]BackupCodeRef, error)
	MarkBackupCodeUsed(ctx context.Context, id int64) error
	CreateSession(ctx context.Context, s *Session) error
	GetSession(ctx context.Context, id string) (*Session, error)
	DeleteSession(ctx context.Context, id string) error
}

type SQLStore struct {
	db *sql.DB
}

func NewSQLStore(db *sql.DB) *SQLStore {
	return &SQLStore{db: db}
}

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

func (s *SQLStore) CreateUser(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}

	_, err := s.db.ExecContext(
		ctx,
		insertUserSQL,
		u.ID,
		u.Username,
		u.PasswordHash,
		nullString(u.TOTPSecret),
		boolToTinyInt(u.TOTPEnabled),
	)
	if isDuplicateEntry(err) {
		return ErrUsernameTaken
	}
	return err
}

func (s *SQLStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, getUserByUsernameSQL, username))
}

func (s *SQLStore) GetUserByID(ctx context.Context, id string) (*User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, getUserByIDSQL, id))
}

func (s *SQLStore) UpdateTOTP(ctx context.Context, userID, secret string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, updateTOTPSQL, nullString(secret), boolToTinyInt(enabled), userID)
	return err
}

func (s *SQLStore) ReplaceBackupCodes(ctx context.Context, userID string, hashes []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, deleteUnusedCodesSQL, userID); err != nil {
		return err
	}
	for _, hash := range hashes {
		if _, err := tx.ExecContext(ctx, insertBackupCodeSQL, userID, hash); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLStore) ListUnusedBackupCodes(ctx context.Context, userID string) ([]BackupCodeRef, error) {
	rows, err := s.db.QueryContext(ctx, listUnusedCodesSQL, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	codes := make([]BackupCodeRef, 0)
	for rows.Next() {
		var code BackupCodeRef
		if err := rows.Scan(&code.ID, &code.Hash); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *SQLStore) MarkBackupCodeUsed(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, markBackupCodeUsedSQL, time.Now(), id)
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

func (s *SQLStore) CreateSession(ctx context.Context, sess *Session) error {
	_, err := s.db.ExecContext(ctx, insertSessionSQL, sess.ID, sess.UserID, sess.ExpiresAt)
	return err
}

func (s *SQLStore) GetSession(ctx context.Context, id string) (*Session, error) {
	var sess Session
	err := s.db.QueryRowContext(ctx, getSessionSQL, id).Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *SQLStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, deleteSessionSQL, id)
	return err
}

type userScanner interface {
	Scan(dest ...any) error
}

func (s *SQLStore) scanUser(scanner userScanner) (*User, error) {
	var u User
	var secret sql.NullString
	var enabled int
	err := scanner.Scan(&u.ID, &u.Username, &u.PasswordHash, &secret, &enabled, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if secret.Valid {
		u.TOTPSecret = secret.String
	}
	u.TOTPEnabled = enabled != 0
	return &u, nil
}

func isDuplicateEntry(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDuplicateEntry
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func boolToTinyInt(value bool) int {
	if value {
		return tinyIntTrue
	}
	return tinyIntFalse
}
