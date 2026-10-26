package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrChallenge = errors.New("challenge invalid or expired")
	// Preserve the existing challenge class while distinguishing a rejected
	// factor on an otherwise live reset link for the route presenter.
	ErrResetFactor        = fmt.Errorf("reset factor invalid: %w", ErrChallenge)
	ErrResetLinkMalformed = fmt.Errorf("reset link incomplete: %w", ErrChallenge)
	ErrRateLimited        = errors.New("account security rate limit")
	ErrMailUnavailable    = errors.New("account mail is not configured")
	ErrPushUnavailable    = errors.New("device push is not configured")
	ErrLastMethod         = errors.New("cannot remove final usable sign-in method")
	ErrRecentAuth         = errors.New("fresh account authentication required")
	ErrProviderConflict   = errors.New("provider identity unavailable")
)

// SecurityIntent is an explicit, one-use authorization for one operation on one
// account family. Target is a normalized address, provider, or link ID, never a
// password. OperationID is caller-generated and retained across lost responses.
type SecurityIntent struct {
	Purpose     string `json:"purpose"`
	Target      string `json:"target"`
	OperationID string `json:"operationId"`
}

func (i SecurityIntent) Valid() bool {
	if len(i.OperationID) < 16 || len(i.OperationID) > 128 || strings.ContainsAny(i.OperationID, "\r\n\x00") || len(i.Target) > 320 {
		return false
	}
	switch i.Purpose {
	case "password_set_or_change", "account_deletion_preview", "email_change", "provider_link", "provider_unlink", "mfa_enroll", "mfa_disable", "recovery_codes", "server_transfer", "server_remove", "account_export":
		return true
	}
	return false
}

// NeedsProof reports whether the action needs a fresh proof (current password,
// or provider re-authentication, plus 2FA when enabled). Justin's rule: only
// sensitive credential changes do: password, email and the second factor.
// Account management (server removal and transfer, provider link and unlink,
// deletion preview, export) needs a signed-in session only (A80); the final
// account deletion is confirmed by typing the account's email instead.
func (i SecurityIntent) NeedsProof() bool {
	switch i.Purpose {
	case "password_set_or_change", "email_change", "mfa_enroll", "mfa_disable", "recovery_codes":
		return true
	}
	return false
}

type RecentProof struct {
	Proof     string    `json:"proof"`
	ExpiresAt time.Time `json:"expiresAt"`
	AccountID string    `json:"accountId"`
	FamilyID  string    `json:"familyId"`
}
type Registration struct {
	Kind            string    `json:"kind"`
	ID              string    `json:"registrationId"`
	Status          string    `json:"status"`
	Revision        string    `json:"revision"`
	MaskedAddress   string    `json:"maskedAddress"`
	ExpiresAt       time.Time `json:"expiresAt"`
	ContactVerified bool      `json:"contactVerified"`
}
type MFAChallenge struct {
	MFARequired    bool      `json:"mfaRequired"`
	ChallengeID    string    `json:"challengeId"`
	ChallengeToken string    `json:"challengeToken"`
	ExpiresAt      time.Time `json:"expiresAt"`
}
type SignInResult struct {
	Session   *Session
	Challenge *MFAChallenge
}
type SecurityProvider struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Revision int64  `json:"revision,string"`
}
type SecurityState struct {
	AccountID              string             `json:"accountId"`
	FamilyID               string             `json:"familyId"`
	ContactAddress         string             `json:"contactAddress"`
	PrivateEmail           bool               `json:"privateEmail"`
	HasPassword            bool               `json:"hasPassword"`
	MFAEnabled             bool               `json:"mfaEnabled"`
	RecoveryCodesRemaining int                `json:"recoveryCodesRemaining"`
	Providers              []SecurityProvider `json:"providers"`
}
type SecurityResult struct {
	AccountID        string           `json:"accountId"`
	FamilyID         string           `json:"familyId"`
	Status           string           `json:"status"`
	SignOut          bool             `json:"signOut,omitempty"`
	ChallengeID      string           `json:"challengeId,omitempty"`
	MaskedAddress    string           `json:"maskedAddress,omitempty"`
	ResendsRemaining *int             `json:"resendsRemaining,omitempty"`
	ExpiresAt        *time.Time       `json:"expiresAt,omitempty"`
	Secret           string           `json:"secret,omitempty"`
	OTPAuthURI       string           `json:"otpAuthUri,omitempty"`
	RecoveryCodes    []string         `json:"recoveryCodes,omitempty"`
	DeletionPreview  *DeletionPreview `json:"deletionPreview,omitempty"`
}

// PasswordCheck performs the same dummy-hash work for absent and wrong users.
// Callers MUST recheck the returned hash under the account lock before using it.
func (s *Service) PasswordCheck(ctx context.Context, username, password string) (Account, []byte, error) {
	if len(password) > 72 || len(username) > 64 {
		return Account{}, nil, ErrCredentials
	}
	release, err := s.passwordSlot(ctx)
	if err != nil {
		return Account{}, nil, err
	}
	defer release()
	a, h, lookup := s.repo.Password(ctx, strings.ToLower(strings.TrimSpace(username)))
	valid := s.passwordMatches(h, password)
	if lookup != nil && !errors.Is(lookup, ErrCredentials) {
		return Account{}, nil, lookup
	}
	if lookup != nil || !valid {
		return Account{}, nil, ErrCredentials
	}
	return a, h, nil
}
func (s *Service) HashPassword(ctx context.Context, password string) ([]byte, error) {
	if !validPassword(password) {
		return nil, ErrInvalid
	}
	release, err := s.passwordSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
}
func ValidUsername(value string) bool { return usernamePattern.MatchString(value) }
func ValidPassword(value string) bool { return validPassword(value) }

// CoolOff is a password attempt refused only because this client network
// failed recently. It is a rate limit (HTTP 429 with Retry-After), never a
// judgement on the password.
type CoolOff struct{ Until time.Time }

func (c CoolOff) Error() string        { return "password attempts are cooling off" }
func (c CoolOff) Is(target error) bool { return target == ErrRateLimited }
