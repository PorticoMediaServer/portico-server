package identity

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"portico.local/hosted/internal/security"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrCredentials = errors.New("invalid credentials")
var ErrConflict = errors.New("username unavailable")

// Only a confirmed public username uniqueness collision has this class.
var ErrUsernameUnavailable = fmt.Errorf("public username collision: %w", ErrConflict)
var ErrInvalid = errors.New("invalid account input")

type Account struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}
type Viewer struct {
	Account Account `json:"account"`
}
type Session struct {
	RefreshToken     string     `json:"refreshToken,omitempty"`
	FamilyID         string     `json:"familyId,omitempty"`
	RefreshExpiresAt *time.Time `json:"refreshExpiresAt,omitempty"`
	Mode             string     `json:"-"`
	AccessToken      string     `json:"accessToken"`
	ExpiresAt        time.Time  `json:"expiresAt"`
	Account          Account    `json:"account"`
}
type Repository interface {
	LifecycleRepository
	Register(context.Context, Account, []byte, []byte, time.Time, FamilySeed) error
	Password(context.Context, string) (Account, []byte, error)
	CreateSession(context.Context, string, []byte, time.Time, []byte, FamilySeed) error
	Authenticate(context.Context, []byte) (Viewer, error)
	IdentityEmail(context.Context, string) (string, error)
	Revoke(context.Context, []byte) error
}
type Service struct {
	repo           Repository
	dummy          []byte
	slots          chan struct{}
	accessLifetime time.Duration
}

func New(repo Repository) *Service { return NewWithAccessLifetime(repo, 12*time.Hour) }
func NewWithAccessLifetime(repo Repository, ttl time.Duration) *Service {
	d, e := bcrypt.GenerateFromPassword([]byte(security.RandomID("dummy")), PasswordCost)
	if e != nil {
		panic(e)
	}
	return &Service{repo: repo, dummy: d, slots: make(chan struct{}, 4), accessLifetime: ttl}
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,30}[a-z0-9]$`)

// PasswordCost is the bcrypt cost of every account password. PasswordInstalled
// accepts exactly this cost, so changing it needs a rehash-on-sign-in path.
const PasswordCost = 10

// validPassword is the only password rule: at least 8 characters (runes, not
// bytes) and no more than bcrypt's 72 bytes. There is no character-class rule;
// clients show an advisory strength hint instead.
func validPassword(p string) bool {
	return utf8.ValidString(p) && utf8.RuneCountInString(p) >= 8 && len(p) <= 72
}
func (s *Service) passwordSlot(ctx context.Context) (func(), error) {
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (s *Service) Register(ctx context.Context, username, name, password string) (Session, error) {
	return s.RegisterMode(ctx, username, name, password, "native")
}
func (s *Service) RegisterMode(ctx context.Context, username, name, password, mode string) (Session, error) {
	if !ValidMode(mode) {
		return Session{}, ErrInvalid
	}
	username = strings.ToLower(strings.TrimSpace(username))
	name = strings.TrimSpace(name)
	if !usernamePattern.MatchString(username) || len(name) < 1 || len(name) > 120 || !validPassword(password) {
		return Session{}, ErrInvalid
	}
	release, e := s.passwordSlot(ctx)
	if e != nil {
		return Session{}, e
	}
	defer release()
	hash, e := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
	if e != nil {
		return Session{}, e
	}
	a := Account{security.RandomID("acct_"), username, name}
	token := security.RandomID("hat_")
	expiry := time.Now().UTC().Add(s.accessLifetime)
	family, refresh := NewFamily(mode)
	if e = s.repo.Register(ctx, a, hash, security.Hash(token), expiry, family); e != nil {
		return Session{}, e
	}
	return Session{AccessToken: token, ExpiresAt: expiry, Account: a, RefreshToken: refresh, FamilyID: family.ID, RefreshExpiresAt: &family.RefreshExpiresAt, Mode: mode}, nil
}
func (s *Service) Login(ctx context.Context, username, password string) (Session, error) {
	return s.LoginMode(ctx, username, password, "native")
}
func (s *Service) LoginMode(ctx context.Context, username, password, mode string) (Session, error) {
	if !ValidMode(mode) {
		return Session{}, ErrInvalid
	}
	if len(password) > 72 || len(username) > 64 {
		return Session{}, ErrCredentials
	}
	release, e := s.passwordSlot(ctx)
	if e != nil {
		return Session{}, e
	}
	defer release()
	a, hash, lookup := s.repo.Password(ctx, strings.ToLower(strings.TrimSpace(username)))
	valid := s.passwordMatches(hash, password)
	if lookup != nil && !errors.Is(lookup, ErrCredentials) {
		return Session{}, lookup
	}
	if lookup != nil || !valid {
		return Session{}, ErrCredentials
	}
	token := security.RandomID("hat_")
	expiry := time.Now().UTC().Add(s.accessLifetime)
	family, refresh := NewFamily(mode)
	if e = s.repo.CreateSession(ctx, a.ID, security.Hash(token), expiry, hash, family); e != nil {
		return Session{}, e
	}
	return Session{AccessToken: token, ExpiresAt: expiry, Account: a, RefreshToken: refresh, FamilyID: family.ID, RefreshExpiresAt: &family.RefreshExpiresAt, Mode: mode}, nil
}
func (s *Service) Authenticate(ctx context.Context, token string) (Viewer, error) {
	if len(token) < 20 || len(token) > 128 {
		return Viewer{}, ErrCredentials
	}
	return s.repo.Authenticate(ctx, security.Hash(token))
}
func (s *Service) Revoke(ctx context.Context, token string) error {
	return s.repo.Revoke(ctx, security.Hash(token))
}

func (s *Service) IdentityEmail(ctx context.Context, account string) (string, error) {
	return s.repo.IdentityEmail(ctx, account)
}
