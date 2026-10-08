package sessions

import (
	"context"
	"errors"
	"portico.local/hosted/internal/security"
	"time"
)

var ErrReviewNotFound = errors.New("account session not found")
var ErrReviewCursor = errors.New("account session continuation changed")
var ErrReviewInvalid = errors.New("invalid account session review input")

type ReviewedFamily struct {
	ID        string     `json:"id"`
	Mode      string     `json:"mode"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt time.Time  `json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	Status    string     `json:"status"`
	Current   bool       `json:"current"`
}
type ReviewPage struct {
	AccountID       string           `json:"accountId"`
	CurrentFamilyID string           `json:"currentFamilyId"`
	ObservedAt      time.Time        `json:"observedAt"`
	Items           []ReviewedFamily `json:"items"`
	NextCursor      string           `json:"nextCursor"`
}
type ReviewRevocation struct {
	ID      string `json:"id"`
	Current bool   `json:"current"`
	Revoked bool   `json:"revoked"`
}
type ReviewRepository interface {
	ReviewFamilies(context.Context, string, []byte, string) (ReviewPage, error)
	RevokeReviewedFamily(context.Context, string, []byte, string) (ReviewRevocation, error)
}

func (s *Service) Review(ctx context.Context, account, token, cursor string) (ReviewPage, error) {
	if len(cursor) > 2048 || len(token) < 1 || len(token) > 256 {
		return ReviewPage{}, ErrReviewInvalid
	}
	return s.repo.ReviewFamilies(ctx, account, security.Hash(token), cursor)
}
func (s *Service) RevokeReviewed(ctx context.Context, account, token, id string) (ReviewRevocation, error) {
	if len(id) < 1 || len(id) > 128 || len(token) < 1 || len(token) > 256 {
		return ReviewRevocation{}, ErrReviewInvalid
	}
	return s.repo.RevokeReviewedFamily(ctx, account, security.Hash(token), id)
}
