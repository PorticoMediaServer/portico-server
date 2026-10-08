package identity

import (
	"context"
	"golang.org/x/crypto/bcrypt"
	"strings"
)

type LifecycleRepository interface {
	RenameAccount(context.Context, string, string) error
	ChangePassword(context.Context, string, []byte, []byte) error
}

func validName(name string) bool { return len(name) > 0 && len(name) <= 120 }
func (s *Service) RenameAccount(ctx context.Context, account, name string) error {
	name = strings.TrimSpace(name)
	if !validName(name) {
		return ErrInvalid
	}
	return s.repo.RenameAccount(ctx, account, name)
}

// Updating credentials invalidates all sessions atomically, including the caller.
func (s *Service) ChangePassword(ctx context.Context, account Account, current, next string) error {
	if len(current) > 72 || !validPassword(next) {
		return ErrInvalid
	}
	release, e := s.passwordSlot(ctx)
	if e != nil {
		return e
	}
	defer release()
	a, old, e := s.repo.Password(ctx, account.Username)
	if e != nil {
		return e
	}
	valid := s.passwordMatches(old, current)
	if a.ID != account.ID || !valid {
		return ErrCredentials
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(next), PasswordCost)
	if e != nil {
		return e
	}
	return s.repo.ChangePassword(ctx, account.ID, old, hash)
}
