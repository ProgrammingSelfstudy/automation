package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"interface-load-test/internal/authstore"
)

const (
	sessionTTL             = 7 * 24 * time.Hour
	backupCodeCount        = 10
	minPasswordLength      = 8
	maxFailedLoginAttempts = 5
	loginLockoutWindow     = 15 * time.Minute
	totpIssuer             = "接口压测控制台"
	sessionTokenBytes      = 32
	backupCodeBytes        = 10
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrTOTPSetupRequired  = errors.New("two-factor authentication has not been set up for this account")
	ErrTOTPAlreadyEnabled = errors.New("two-factor authentication is already enabled")
	ErrTOTPCodeRequired   = errors.New("two-factor verification code is required")
	ErrInvalidCode        = errors.New("invalid verification code")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrTooManyAttempts    = errors.New("too many failed login attempts, try again later")

	dummyPasswordHash = mustGenerateDummyPasswordHash()
)

type Service struct {
	store authstore.Store
}

func NewService(store authstore.Store) *Service {
	return &Service{store: store}
}

func (s *Service) CreateUser(ctx context.Context, username, password string) (*authstore.User, error) {
	if len(password) < minPasswordLength {
		return nil, ErrWeakPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &authstore.User{
		Username:     username,
		PasswordHash: string(hash),
		TOTPEnabled:  false,
	}
	if err := s.store.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) SetupTOTP(ctx context.Context, username, password string) (secret, otpauthURL string, err error) {
	user, err := s.getUserByUsernameWithPassword(ctx, username, password)
	if err != nil {
		return "", "", err
	}
	if user.TOTPEnabled {
		return "", "", ErrTOTPAlreadyEnabled
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: user.Username,
	})
	if err != nil {
		return "", "", err
	}
	secret = key.Secret()
	if err := s.store.UpdateTOTP(ctx, user.ID, secret, false); err != nil {
		return "", "", err
	}
	return secret, key.URL(), nil
}

func (s *Service) ConfirmTOTP(ctx context.Context, username, password, code string) (*authstore.User, *authstore.Session, []string, error) {
	user, err := s.getUserByUsernameWithPassword(ctx, username, password)
	if err != nil {
		return nil, nil, nil, err
	}
	if user.TOTPSecret == "" {
		return nil, nil, nil, ErrTOTPSetupRequired
	}
	if !totp.Validate(code, user.TOTPSecret) {
		return nil, nil, nil, ErrInvalidCode
	}

	plainCodes, hashes, err := generateBackupCodes()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := s.store.UpdateTOTP(ctx, user.ID, user.TOTPSecret, true); err != nil {
		return nil, nil, nil, err
	}
	if err := s.store.ReplaceBackupCodes(ctx, user.ID, hashes); err != nil {
		return nil, nil, nil, err
	}
	user.TOTPEnabled = true
	_, session, err := s.createSession(ctx, user)
	if err != nil {
		return nil, nil, nil, err
	}
	return user, session, plainCodes, nil
}

func (s *Service) Login(ctx context.Context, ip, username, password, code, backupCode string) (*authstore.User, *authstore.Session, error) {
	count, err := s.store.CountRecentFailedLoginAttempts(ctx, ip, time.Now().Add(-loginLockoutWindow))
	if err != nil {
		return nil, nil, err
	}
	if count >= maxFailedLoginAttempts {
		return nil, nil, ErrTooManyAttempts
	}

	user, session, err := s.loginWithCredentials(ctx, username, password, code, backupCode)
	if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrInvalidCode) {
		if recordErr := s.store.RecordFailedLoginAttempt(ctx, ip); recordErr != nil {
			return nil, nil, recordErr
		}
	}
	return user, session, err
}

func (s *Service) loginWithCredentials(ctx context.Context, username, password, code, backupCode string) (*authstore.User, *authstore.Session, error) {
	user, err := s.getUserByUsernameWithPassword(ctx, username, password)
	if err != nil {
		return nil, nil, err
	}
	if !user.TOTPEnabled {
		return nil, nil, ErrTOTPSetupRequired
	}

	if strings.TrimSpace(code) == "" && strings.TrimSpace(backupCode) == "" {
		return nil, nil, ErrTOTPCodeRequired
	}
	if strings.TrimSpace(code) != "" {
		if !totp.Validate(code, user.TOTPSecret) {
			return nil, nil, ErrInvalidCode
		}
		return s.createSession(ctx, user)
	}

	ok, err := s.consumeBackupCode(ctx, user.ID, backupCode)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, ErrInvalidCode
	}
	return s.createSession(ctx, user)
}

func (s *Service) RegenerateBackupCodes(ctx context.Context, userID, password string) ([]string, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if errors.Is(err, authstore.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !passwordMatches(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}

	plainCodes, hashes, err := generateBackupCodes()
	if err != nil {
		return nil, err
	}
	if err := s.store.ReplaceBackupCodes(ctx, user.ID, hashes); err != nil {
		return nil, err
	}
	return plainCodes, nil
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.store.DeleteSession(ctx, sessionID)
}

func (s *Service) CurrentUser(ctx context.Context, sessionID string) (*authstore.User, error) {
	session, err := s.store.GetSession(ctx, sessionID)
	if errors.Is(err, authstore.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	user, err := s.store.GetUserByID(ctx, session.UserID)
	if errors.Is(err, authstore.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) getUserByUsernameWithPassword(ctx context.Context, username, password string) (*authstore.User, error) {
	user, err := s.store.GetUserByUsername(ctx, username)
	if errors.Is(err, authstore.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !passwordMatches(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) createSession(ctx context.Context, user *authstore.User) (*authstore.User, *authstore.Session, error) {
	token, err := randomHex(sessionTokenBytes)
	if err != nil {
		return nil, nil, err
	}
	session := &authstore.Session{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(sessionTTL),
	}
	if err := s.store.CreateSession(ctx, session); err != nil {
		return nil, nil, err
	}
	return user, session, nil
}

func (s *Service) consumeBackupCode(ctx context.Context, userID, backupCode string) (bool, error) {
	refs, err := s.store.ListUnusedBackupCodes(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, ref := range refs {
		if passwordMatches(ref.Hash, backupCode) {
			if err := s.store.MarkBackupCodeUsed(ctx, ref.ID); err != nil {
				if errors.Is(err, authstore.ErrNotFound) {
					return false, nil
				}
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func passwordMatches(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func mustGenerateDummyPasswordHash() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-invalid-users"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return hash
}

func generateBackupCodes() ([]string, []string, error) {
	plainCodes := make([]string, 0, backupCodeCount)
	hashes := make([]string, 0, backupCodeCount)
	for i := 0; i < backupCodeCount; i++ {
		code, err := randomBackupCode()
		if err != nil {
			return nil, nil, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return nil, nil, err
		}
		plainCodes = append(plainCodes, code)
		hashes = append(hashes, string(hash))
	}
	return plainCodes, hashes, nil
}

func randomBackupCode() (string, error) {
	raw := make([]byte, backupCodeBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	if len(encoded) > 16 {
		encoded = encoded[:16]
	}
	return encoded[:4] + "-" + encoded[4:8] + "-" + encoded[8:12] + "-" + encoded[12:], nil
}

func randomHex(byteCount int) (string, error) {
	raw := make([]byte, byteCount)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
