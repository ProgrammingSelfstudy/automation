package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"interface-load-test/internal/authstore"
)

const testLoginIP = "203.0.113.10"

func TestCreateUser(t *testing.T) {
	t.Run("weak password", func(t *testing.T) {
		service := NewService(newFakeAuthStore())
		user, err := service.CreateUser(context.Background(), "admin", "short")
		if user != nil {
			t.Fatalf("user = %#v, want nil", user)
		}
		if !errors.Is(err, ErrWeakPassword) {
			t.Fatalf("error = %v, want %v", err, ErrWeakPassword)
		}
	})

	t.Run("username taken", func(t *testing.T) {
		store := newFakeAuthStore()
		store.createUserErr = authstore.ErrUsernameTaken
		service := NewService(store)

		user, err := service.CreateUser(context.Background(), "admin", "strong-password")
		if user != nil {
			t.Fatalf("user = %#v, want nil", user)
		}
		if !errors.Is(err, authstore.ErrUsernameTaken) {
			t.Fatalf("error = %v, want %v", err, authstore.ErrUsernameTaken)
		}
	})
}

func TestLoginInvalidCredentialsAreIndistinguishable(t *testing.T) {
	store := newFakeAuthStore()
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
		TOTPEnabled:  true,
		TOTPSecret:   testTOTPSecret(t),
	}
	service := NewService(store)

	missingUser, missingSession, missingErr := service.Login(context.Background(), testLoginIP, "missing", "correct-password", "123456", "")
	badPasswordUser, badPasswordSession, badPasswordErr := service.Login(context.Background(), testLoginIP, "admin", "wrong-password", "123456", "")

	if missingUser != nil || missingSession != nil || badPasswordUser != nil || badPasswordSession != nil {
		t.Fatalf("invalid credential returns = (%#v,%#v) and (%#v,%#v), want nils", missingUser, missingSession, badPasswordUser, badPasswordSession)
	}
	if missingErr != ErrInvalidCredentials || badPasswordErr != ErrInvalidCredentials {
		t.Fatalf("errors = (%v,%v), want identical ErrInvalidCredentials", missingErr, badPasswordErr)
	}
}

func TestLoginRequiresTOTPSetup(t *testing.T) {
	store := newFakeAuthStore()
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
		TOTPEnabled:  false,
	}
	service := NewService(store)

	user, session, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", "123456", "")
	if user != nil || session != nil {
		t.Fatalf("Login() = (%#v,%#v), want nils", user, session)
	}
	if !errors.Is(err, ErrTOTPSetupRequired) {
		t.Fatalf("error = %v, want %v", err, ErrTOTPSetupRequired)
	}
}

func TestLoginWithTOTPCode(t *testing.T) {
	store := newFakeAuthStore()
	secret := testTOTPSecret(t)
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
		TOTPEnabled:  true,
		TOTPSecret:   secret,
	}
	service := NewService(store)
	code := currentTOTPCode(t, secret)

	user, session, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", code, "")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if user == nil || user.ID != "user-1" || session == nil || session.ID == "" {
		t.Fatalf("Login() = (%#v,%#v), want user and session", user, session)
	}

	badUser, badSession, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", wrongTOTPCode(code), "")
	if badUser != nil || badSession != nil {
		t.Fatalf("bad Login() = (%#v,%#v), want nils", badUser, badSession)
	}
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("bad Login() error = %v, want %v", err, ErrInvalidCode)
	}
}

func TestLoginRequiresCodeForEnabledTOTP(t *testing.T) {
	store := newFakeAuthStore()
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
		TOTPEnabled:  true,
		TOTPSecret:   testTOTPSecret(t),
	}
	service := NewService(store)

	_, _, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", "", "")
	if !errors.Is(err, ErrTOTPCodeRequired) {
		t.Fatalf("error = %v, want %v", err, ErrTOTPCodeRequired)
	}
}

func TestLoginWithBackupCodeConsumesCode(t *testing.T) {
	store := newFakeAuthStore()
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
		TOTPEnabled:  true,
		TOTPSecret:   testTOTPSecret(t),
	}
	store.backupCodes["user-1"] = []authstore.BackupCodeRef{
		{ID: 1, Hash: hashPasswordForTest(t, "WRONG-CODE")},
		{ID: 2, Hash: hashPasswordForTest(t, "RIGHT-CODE")},
	}
	service := NewService(store)

	_, session, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", "", "RIGHT-CODE")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if session == nil || session.ID == "" {
		t.Fatalf("session = %#v, want signed session", session)
	}
	if len(store.markedBackupCodeIDs) != 1 || store.markedBackupCodeIDs[0] != 2 {
		t.Fatalf("markedBackupCodeIDs = %#v, want [2]", store.markedBackupCodeIDs)
	}

	_, _, err = service.Login(context.Background(), testLoginIP, "admin", "correct-password", "", "RIGHT-CODE")
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("second Login() error = %v, want %v", err, ErrInvalidCode)
	}
}

func TestLoginWithBackupCodeRejectsConcurrentConsume(t *testing.T) {
	store := newFakeAuthStore()
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
		TOTPEnabled:  true,
		TOTPSecret:   testTOTPSecret(t),
	}
	store.backupCodes["user-1"] = []authstore.BackupCodeRef{
		{ID: 2, Hash: hashPasswordForTest(t, "RIGHT-CODE")},
	}
	store.markBackupCodeErrByID[2] = authstore.ErrNotFound
	service := NewService(store)

	user, session, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", "", "RIGHT-CODE")
	if user != nil || session != nil {
		t.Fatalf("Login() = (%#v,%#v), want nils", user, session)
	}
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCode)
	}
	if len(store.markedBackupCodeIDs) != 0 {
		t.Fatalf("markedBackupCodeIDs = %#v, want empty", store.markedBackupCodeIDs)
	}
}

func TestLoginRateLimitRejectsBeforeCredentials(t *testing.T) {
	store := newFakeAuthStore()
	secret := testTOTPSecret(t)
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
		TOTPEnabled:  true,
		TOTPSecret:   secret,
	}
	store.seedFailedLoginAttempts(testLoginIP, maxFailedLoginAttempts)
	service := NewService(store)

	user, session, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", currentTOTPCode(t, secret), "")
	if user != nil || session != nil {
		t.Fatalf("Login() = (%#v,%#v), want nils", user, session)
	}
	if !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("Login() error = %v, want %v", err, ErrTooManyAttempts)
	}
	if len(store.sessions) != 0 {
		t.Fatalf("sessions = %#v, want none", store.sessions)
	}
}

func TestLoginRateLimitAllowsBelowThreshold(t *testing.T) {
	store := newFakeAuthStore()
	secret := testTOTPSecret(t)
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
		TOTPEnabled:  true,
		TOTPSecret:   secret,
	}
	store.seedFailedLoginAttempts(testLoginIP, maxFailedLoginAttempts-1)
	service := NewService(store)

	user, session, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", currentTOTPCode(t, secret), "")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if user == nil || session == nil {
		t.Fatalf("Login() = (%#v,%#v), want success", user, session)
	}
}

func TestLoginDoesNotRecordTOTPFlowPrompts(t *testing.T) {
	t.Run("setup required", func(t *testing.T) {
		store := newFakeAuthStore()
		store.usersByUsername["admin"] = &authstore.User{
			ID:           "user-1",
			Username:     "admin",
			PasswordHash: hashPasswordForTest(t, "correct-password"),
			TOTPEnabled:  false,
		}
		service := NewService(store)

		_, _, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", "123456", "")
		if !errors.Is(err, ErrTOTPSetupRequired) {
			t.Fatalf("Login() error = %v, want %v", err, ErrTOTPSetupRequired)
		}
		if got := store.recordedFailedLoginAttempts(testLoginIP); got != 0 {
			t.Fatalf("recorded attempts = %d, want 0", got)
		}
	})

	t.Run("code required", func(t *testing.T) {
		store := newFakeAuthStore()
		store.usersByUsername["admin"] = &authstore.User{
			ID:           "user-1",
			Username:     "admin",
			PasswordHash: hashPasswordForTest(t, "correct-password"),
			TOTPEnabled:  true,
			TOTPSecret:   testTOTPSecret(t),
		}
		service := NewService(store)

		_, _, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", "", "")
		if !errors.Is(err, ErrTOTPCodeRequired) {
			t.Fatalf("Login() error = %v, want %v", err, ErrTOTPCodeRequired)
		}
		if got := store.recordedFailedLoginAttempts(testLoginIP); got != 0 {
			t.Fatalf("recorded attempts = %d, want 0", got)
		}
	})
}

func TestLoginRecordsCredentialFailures(t *testing.T) {
	store := newFakeAuthStore()
	secret := testTOTPSecret(t)
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
		TOTPEnabled:  true,
		TOTPSecret:   secret,
	}
	service := NewService(store)

	_, _, err := service.Login(context.Background(), testLoginIP, "admin", "wrong-password", "123456", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want %v", err, ErrInvalidCredentials)
	}
	_, _, err = service.Login(context.Background(), testLoginIP, "admin", "correct-password", wrongTOTPCode(currentTOTPCode(t, secret)), "")
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("wrong TOTP error = %v, want %v", err, ErrInvalidCode)
	}
	_, _, err = service.Login(context.Background(), testLoginIP, "admin", "correct-password", "", "WRONG-CODE")
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("wrong backup code error = %v, want %v", err, ErrInvalidCode)
	}
	if got := store.recordedFailedLoginAttempts(testLoginIP); got != 3 {
		t.Fatalf("recorded attempts = %d, want 3", got)
	}
}

func TestLoginFailuresAreScopedByIP(t *testing.T) {
	store := newFakeAuthStore()
	secret := testTOTPSecret(t)
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
		TOTPEnabled:  true,
		TOTPSecret:   secret,
	}
	store.seedFailedLoginAttempts("203.0.113.20", maxFailedLoginAttempts)
	service := NewService(store)

	user, session, err := service.Login(context.Background(), testLoginIP, "admin", "correct-password", currentTOTPCode(t, secret), "")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if user == nil || session == nil {
		t.Fatalf("Login() = (%#v,%#v), want success for unaffected IP", user, session)
	}
}

func TestConfirmTOTP(t *testing.T) {
	store := newFakeAuthStore()
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
	}
	service := NewService(store)

	secret, _, err := service.SetupTOTP(context.Background(), "admin", "correct-password")
	if err != nil {
		t.Fatalf("SetupTOTP() error = %v", err)
	}
	code := currentTOTPCode(t, secret)
	user, session, backupCodes, err := service.ConfirmTOTP(context.Background(), "admin", "correct-password", code)
	if err != nil {
		t.Fatalf("ConfirmTOTP() error = %v", err)
	}
	if user == nil || !user.TOTPEnabled {
		t.Fatalf("user = %#v, want TOTP enabled", user)
	}
	if session == nil || session.ID == "" || session.UserID != "user-1" {
		t.Fatalf("session = %#v, want signed session for user-1", session)
	}
	if len(backupCodes) != backupCodeCount {
		t.Fatalf("len(backupCodes) = %d, want %d", len(backupCodes), backupCodeCount)
	}
	if len(store.backupCodes["user-1"]) != backupCodeCount {
		t.Fatalf("stored backup code hashes = %d, want %d", len(store.backupCodes["user-1"]), backupCodeCount)
	}
}

func TestConfirmTOTPRejectsInvalidCode(t *testing.T) {
	store := newFakeAuthStore()
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: hashPasswordForTest(t, "correct-password"),
	}
	service := NewService(store)

	secret, _, err := service.SetupTOTP(context.Background(), "admin", "correct-password")
	if err != nil {
		t.Fatalf("SetupTOTP() error = %v", err)
	}
	user, session, backupCodes, err := service.ConfirmTOTP(context.Background(), "admin", "correct-password", wrongTOTPCode(currentTOTPCode(t, secret)))
	if user != nil || session != nil || backupCodes != nil {
		t.Fatalf("ConfirmTOTP() = (%#v,%#v,%#v), want nils", user, session, backupCodes)
	}
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("ConfirmTOTP() error = %v, want %v", err, ErrInvalidCode)
	}
}

func BenchmarkLoginInvalidCredentialsTiming(b *testing.B) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		b.Fatalf("GenerateFromPassword() error = %v", err)
	}
	store := newFakeAuthStore()
	store.usersByUsername["admin"] = &authstore.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: string(hash),
		TOTPEnabled:  true,
		TOTPSecret:   testTOTPSecretForBenchmark(b),
	}
	service := NewService(store)

	b.Run("missing user", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, err := service.Login(context.Background(), fmt.Sprintf("203.0.113.%d", i), "missing", "wrong-password", "123456", "")
			if !errors.Is(err, ErrInvalidCredentials) {
				b.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
			}
		}
	})

	b.Run("bad password", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, err := service.Login(context.Background(), fmt.Sprintf("198.51.100.%d", i), "admin", "wrong-password", "123456", "")
			if !errors.Is(err, ErrInvalidCredentials) {
				b.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
			}
		}
	})
}

type fakeAuthStore struct {
	mu                    sync.Mutex
	usersByID             map[string]*authstore.User
	usersByUsername       map[string]*authstore.User
	sessions              map[string]*authstore.Session
	backupCodes           map[string][]authstore.BackupCodeRef
	usedBackupCodeIDs     map[int64]bool
	markBackupCodeErrByID map[int64]error
	markedBackupCodeIDs   []int64
	failedLoginAttempts   map[string][]time.Time
	createUserErr         error
}

func newFakeAuthStore() *fakeAuthStore {
	return &fakeAuthStore{
		usersByID:             make(map[string]*authstore.User),
		usersByUsername:       make(map[string]*authstore.User),
		sessions:              make(map[string]*authstore.Session),
		backupCodes:           make(map[string][]authstore.BackupCodeRef),
		usedBackupCodeIDs:     make(map[int64]bool),
		markBackupCodeErrByID: make(map[int64]error),
		failedLoginAttempts:   make(map[string][]time.Time),
	}
}

func (s *fakeAuthStore) CreateUser(_ context.Context, user *authstore.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createUserErr != nil {
		return s.createUserErr
	}
	if user.ID == "" {
		user.ID = fmt.Sprintf("user-%d", len(s.usersByID)+1)
	}
	cloned := cloneUser(user)
	s.usersByID[cloned.ID] = cloned
	s.usersByUsername[cloned.Username] = cloned
	return nil
}

func (s *fakeAuthStore) GetUserByUsername(_ context.Context, username string) (*authstore.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.usersByUsername[username]
	if !ok {
		return nil, authstore.ErrNotFound
	}
	return cloneUser(user), nil
}

func (s *fakeAuthStore) GetUserByID(_ context.Context, id string) (*authstore.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.usersByID[id]
	if !ok {
		return nil, authstore.ErrNotFound
	}
	return cloneUser(user), nil
}

func (s *fakeAuthStore) UpdateTOTP(_ context.Context, userID, secret string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.usersByID[userID]
	if !ok {
		for _, candidate := range s.usersByUsername {
			if candidate.ID == userID {
				user = candidate
				ok = true
				break
			}
		}
	}
	if !ok {
		return authstore.ErrNotFound
	}
	user.TOTPSecret = secret
	user.TOTPEnabled = enabled
	s.usersByID[user.ID] = user
	s.usersByUsername[user.Username] = user
	return nil
}

func (s *fakeAuthStore) ReplaceBackupCodes(_ context.Context, userID string, hashes []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	refs := make([]authstore.BackupCodeRef, 0, len(hashes))
	for i, hash := range hashes {
		refs = append(refs, authstore.BackupCodeRef{ID: int64(i + 1), Hash: hash})
	}
	s.backupCodes[userID] = refs
	s.usedBackupCodeIDs = make(map[int64]bool)
	return nil
}

func (s *fakeAuthStore) ListUnusedBackupCodes(_ context.Context, userID string) ([]authstore.BackupCodeRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	refs := make([]authstore.BackupCodeRef, 0, len(s.backupCodes[userID]))
	for _, ref := range s.backupCodes[userID] {
		if !s.usedBackupCodeIDs[ref.ID] {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func (s *fakeAuthStore) MarkBackupCodeUsed(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.markBackupCodeErrByID[id]; err != nil {
		return err
	}
	s.usedBackupCodeIDs[id] = true
	s.markedBackupCodeIDs = append(s.markedBackupCodeIDs, id)
	return nil
}

func (s *fakeAuthStore) CreateSession(_ context.Context, session *authstore.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = &authstore.Session{ID: session.ID, UserID: session.UserID, ExpiresAt: session.ExpiresAt}
	return nil
}

func (s *fakeAuthStore) GetSession(_ context.Context, id string) (*authstore.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, authstore.ErrNotFound
	}
	return &authstore.Session{ID: session.ID, UserID: session.UserID, ExpiresAt: session.ExpiresAt}, nil
}

func (s *fakeAuthStore) DeleteSession(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *fakeAuthStore) RecordFailedLoginAttempt(_ context.Context, ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedLoginAttempts[ip] = append(s.failedLoginAttempts[ip], time.Now())
	return nil
}

func (s *fakeAuthStore) CountRecentFailedLoginAttempts(_ context.Context, ip string, since time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, at := range s.failedLoginAttempts[ip] {
		if !at.Before(since) {
			count++
		}
	}
	return count, nil
}

func (s *fakeAuthStore) seedFailedLoginAttempts(ip string, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < count; i++ {
		s.failedLoginAttempts[ip] = append(s.failedLoginAttempts[ip], time.Now())
	}
}

func (s *fakeAuthStore) recordedFailedLoginAttempts(ip string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.failedLoginAttempts[ip])
}

func hashPasswordForTest(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	return string(hash)
}

func testTOTPSecret(t *testing.T) string {
	t.Helper()
	key, err := totp.Generate(totp.GenerateOpts{Issuer: TOTPIssuer, AccountName: "admin"})
	if err != nil {
		t.Fatalf("totp.Generate() error = %v", err)
	}
	return key.Secret()
}

func testTOTPSecretForBenchmark(b *testing.B) string {
	b.Helper()
	key, err := totp.Generate(totp.GenerateOpts{Issuer: TOTPIssuer, AccountName: "admin"})
	if err != nil {
		b.Fatalf("totp.Generate() error = %v", err)
	}
	return key.Secret()
}

func currentTOTPCode(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}
	return code
}

func wrongTOTPCode(valid string) string {
	if valid == "000000" {
		return "111111"
	}
	return "000000"
}

func cloneUser(user *authstore.User) *authstore.User {
	if user == nil {
		return nil
	}
	cloned := *user
	return &cloned
}
