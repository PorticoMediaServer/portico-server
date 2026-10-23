package identity

import (
	"context"
	"errors"
	"portico.local/hosted/internal/security"
	"time"
)

var ErrDeletionConflict = errors.New("deletion preview changed or expired")

// ErrDeletionConfirmation: the typed confirmation is not the account's email
// (or, for an account without one, its username).
var ErrDeletionConfirmation = errors.New("deletion confirmation does not match the account")
var ErrOwnedServers = errors.New("resolve owned servers before account deletion")

type DeletionServer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type DeletionPreview struct {
	Token           string           `json:"deletionToken"`
	ExpiresAt       time.Time        `json:"expiresAt"`
	OwnedServers    []DeletionServer `json:"ownedServers"`
	AffectedServers []DeletionServer `json:"affectedServers"`
	CanDelete       bool             `json:"canDelete"`
}
type DeletionReceipt struct {
	AccountDeleted   bool      `json:"accountDeleted"`
	ID               string    `json:"deletionId"`
	Status           string    `json:"status"`
	PendingServers   []string  `json:"pendingServers"`
	AcceptedAt       time.Time `json:"acceptedAt"`
	ReceiptExpiresAt time.Time `json:"receiptExpiresAt"`
}
type DeletionRepository interface {
	PrepareDeletion(context.Context, string, []byte, []byte, time.Time) (DeletionPreview, error)
	AcceptDeletion(context.Context, []byte, string) (DeletionReceipt, error)
	DeletionStatus(context.Context, []byte) (DeletionReceipt, error)
}

func (s *Service) PrepareDeletion(ctx context.Context, a Account, password string) (DeletionPreview, error) {
	if len(password) == 0 || len(password) > 72 {
		return DeletionPreview{}, ErrInvalid
	}
	release, e := s.passwordSlot(ctx)
	if e != nil {
		return DeletionPreview{}, e
	}
	defer release()
	current, hash, e := s.repo.Password(ctx, a.Username)
	if e != nil {
		return DeletionPreview{}, e
	}
	valid := s.passwordMatches(hash, password)
	if current.ID != a.ID || !valid {
		return DeletionPreview{}, ErrCredentials
	}
	token := security.RandomID("del_")
	p, e := s.repo.(DeletionRepository).PrepareDeletion(ctx, a.ID, hash, security.Hash(token), time.Now().UTC().Add(5*time.Minute))
	if e == nil {
		p.Token = token
	}
	return p, e
}
func (s *Service) AcceptDeletion(ctx context.Context, token, disposition string) (DeletionReceipt, error) {
	if len(token) < 32 || len(token) > 128 {
		return DeletionReceipt{}, ErrCredentials
	}
	if disposition != "" {
		return DeletionReceipt{}, ErrInvalid
	}
	return s.repo.(DeletionRepository).AcceptDeletion(ctx, security.Hash(token), disposition)
}
func (s *Service) DeletionStatus(ctx context.Context, token string) (DeletionReceipt, error) {
	if len(token) < 32 || len(token) > 128 {
		return DeletionReceipt{}, ErrCredentials
	}
	return s.repo.(DeletionRepository).DeletionStatus(ctx, security.Hash(token))
}
