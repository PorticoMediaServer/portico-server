// Package sessions owns rotating refresh families independently of password authentication.
package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"regexp"
	"strings"
	"time"
)

var ErrGrant = errors.New("invalid refresh grant")
var ErrReuse = errors.New("refresh token reuse detected")
var ErrConflict = errors.New("rotation request conflict")
var ErrFamilyChanged = errors.New("browser session family changed")

type Lookup struct {
	FamilyID, Mode    string
	AbsoluteExpiresAt time.Time
}
type Rotation struct {
	FamilyID, RequestID, Mode                 string
	InputHash, OutputHash, AccessHash, Cipher []byte
	ExpiresAt                                 time.Time
}
type Rotated struct {
	Cipher           []byte
	Viewer           identity.Viewer
	RefreshExpiresAt time.Time
}
type Repository interface {
	ReviewRepository
	LookupRefresh(context.Context, []byte) (Lookup, error)
	RotateRefresh(context.Context, Rotation) (Rotated, error)
	RevokeRefresh(context.Context, []byte, string) error
	LogoutAll(context.Context, string) error
}
type Service struct {
	repo           Repository
	keys           *security.Keys
	accessLifetime time.Duration
}

func New(repo Repository, keys *security.Keys, ttl time.Duration) *Service {
	return &Service{repo, keys, ttl}
}

var requestID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

func validToken(token string) bool { return strings.HasPrefix(token, "hrt_") && len(token) == 47 }
func (s *Service) Refresh(ctx context.Context, token, id, mode string) (identity.Session, error) {
	return s.RefreshExpected(ctx, token, id, mode, "")
}

// RefreshExpected binds a browser intent to its original family. Token-to-family
// ownership is immutable, so this check remains valid through the rotation lock.
func (s *Service) RefreshExpected(ctx context.Context, token, id, mode, expectedFamily string) (identity.Session, error) {
	id = strings.ToLower(id)
	if !validToken(token) || !requestID.MatchString(id) || !identity.ValidMode(mode) {
		return identity.Session{}, ErrGrant
	}
	input := security.Hash(token)
	known, e := s.repo.LookupRefresh(ctx, input)
	if e != nil {
		return identity.Session{}, e
	}
	if known.Mode != mode {
		return identity.Session{}, ErrGrant
	}
	if mode == "browser" && expectedFamily != "" && known.FamilyID != expectedFamily {
		return identity.Session{}, ErrFamilyChanged
	}
	pair := identity.Session{AccessToken: security.RandomID("hat_"), RefreshToken: security.RandomID("hrt_"), FamilyID: known.FamilyID, ExpiresAt: time.Now().UTC().Add(s.accessLifetime), Mode: mode}
	if pair.ExpiresAt.After(known.AbsoluteExpiresAt) {
		pair.ExpiresAt = known.AbsoluteExpiresAt.UTC()
	}
	raw, _ := json.Marshal(pair)
	cipher, e := s.keys.Seal(string(raw), "refresh:"+known.FamilyID+":"+id)
	if e != nil {
		return identity.Session{}, e
	}
	rotated, e := s.repo.RotateRefresh(ctx, Rotation{FamilyID: known.FamilyID, RequestID: id, Mode: mode, InputHash: input, OutputHash: security.Hash(pair.RefreshToken), AccessHash: security.Hash(pair.AccessToken), Cipher: cipher, ExpiresAt: pair.ExpiresAt})
	if e != nil {
		return identity.Session{}, e
	}
	rawPair, e := s.keys.Open(rotated.Cipher, "refresh:"+known.FamilyID+":"+id)
	if e != nil {
		return identity.Session{}, e
	}
	if e = json.Unmarshal([]byte(rawPair), &pair); e != nil {
		return identity.Session{}, e
	}
	pair.Account = rotated.Viewer.Account
	pair.Profiles = rotated.Viewer.Profiles
	pair.ExpiresAt = pair.ExpiresAt.UTC()
	refreshExpiresAt := rotated.RefreshExpiresAt.UTC()
	pair.RefreshExpiresAt = &refreshExpiresAt
	pair.Mode = mode
	return pair, nil
}
func (s *Service) Revoke(ctx context.Context, token, mode string) error {
	if !validToken(token) || !identity.ValidMode(mode) {
		return ErrGrant
	}
	return s.repo.RevokeRefresh(ctx, security.Hash(token), mode)
}
func (s *Service) LogoutAll(ctx context.Context, account string) error {
	return s.repo.LogoutAll(ctx, account)
}

// RevokeExpected reports whether the caller may clear the browser cookie. A stale
// browser intent must neither revoke nor clear another family's credential.
func (s *Service) RevokeExpected(ctx context.Context, token, mode, expectedFamily string) (bool, error) {
	if mode == "browser" && expectedFamily != "" {
		if !validToken(token) {
			return false, ErrGrant
		}
		known, err := s.repo.LookupRefresh(ctx, security.Hash(token))
		if errors.Is(err, ErrGrant) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if known.Mode != mode {
			return false, ErrGrant
		}
		if known.FamilyID != expectedFamily {
			return false, nil
		}
	}
	err := s.Revoke(ctx, token, mode)
	return err == nil, err
}
