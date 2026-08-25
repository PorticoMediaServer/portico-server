package app

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	accountPasswordMinRunes      = 8
	accountPasswordMaxBytes      = 72
	currentPasswordBcryptCost    = 10
	accountPasswordPolicyMessage = "Password must be at least 8 characters with an uppercase letter, a lowercase letter, and a number or special character, and no more than 72 UTF-8 bytes."
)

// This is a cost-10 hash for a fixed value that is never accepted. It keeps
// unknown-user, disabled-password, and malformed-hash verification on the same
// expensive bcrypt path as valid accounts without generating work per request.
const passwordTimingEqualizerHash = "$2b$10$07qULlrI5oie4ig4LNMDY.IY7BoDvXw7LKFvt5u3G/xjHyxnnWYEe"

var errAccountPasswordPolicy = errors.New("password does not satisfy the account password policy")
var errPasswordCredentialChanged = errors.New("password credential changed during authentication")

func validAccountPassword(password string) bool {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < accountPasswordMinRunes || len([]byte(password)) > accountPasswordMaxBytes {
		return false
	}
	var upper, lower, numberOrSpecial bool
	for _, char := range password {
		upper = upper || unicode.IsUpper(char)
		lower = lower || unicode.IsLower(char)
		numberOrSpecial = numberOrSpecial || unicode.IsDigit(char) || (!unicode.IsLetter(char) && !unicode.IsDigit(char))
	}
	return upper && lower && numberOrSpecial
}

func hashAccountPassword(ctx context.Context, callsite kdfCallsite, password string) (string, error) {
	if !validAccountPassword(password) {
		return "", errAccountPasswordPolicy
	}
	hash, err := runKDF(ctx, callsite, kdfLaneForCallsite(callsite), func() ([]byte, error) {
		return bcrypt.GenerateFromPassword([]byte(password), currentPasswordBcryptCost)
	})
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// verifyAccountPasswordSnapshot returns the exact credential hash that was
// compared successfully.  Callers that turn a password comparison into an
// authority must carry this value into their commit transaction and compare
// it again with users.password_hash; a later password rotation then wins over
// the stale request monotonically.
func verifyAccountPasswordSnapshot(ctx context.Context, callsite kdfCallsite, passwordHash, password string) (valid bool, upgrade bool, verifiedHash string, err error) {
	passwordHash = strings.TrimSpace(passwordHash)
	eligible := passwordHash != "" && utf8.ValidString(password) && len([]byte(password)) <= accountPasswordMaxBytes
	hash := []byte(passwordHash)
	candidate := []byte(password)
	cost, costErr := bcrypt.Cost(hash)
	// Historical hashes below the current policy are still valid and are
	// upgraded after authentication.  A hash at a newer cost is equally valid
	// and must never be replaced with a weaker current-cost hash.
	if !eligible || costErr != nil || cost < bcrypt.MinCost {
		hash = []byte(passwordTimingEqualizerHash)
		candidate = []byte("portico-invalid-password")
		eligible = false
	}
	compareErr, err := runKDF(ctx, callsite, kdfLaneCompare, func() (error, error) {
		return bcrypt.CompareHashAndPassword(hash, candidate), nil
	})
	if err != nil {
		return false, false, "", err
	}
	if compareErr != nil || !eligible {
		return false, false, "", nil
	}
	return true, cost < currentPasswordBcryptCost, passwordHash, nil
}

func verifyAccountPassword(ctx context.Context, callsite kdfCallsite, passwordHash, password string) (valid bool, upgrade bool, err error) {
	valid, upgrade, _, err = verifyAccountPasswordSnapshot(ctx, callsite, passwordHash, password)
	return valid, upgrade, err
}

func (s *Server) verifyAndUpgradeLocalPassword(ctx context.Context, callsite kdfCallsite, userID, passwordHash, password string) (bool, error) {
	valid, upgrade, _, err := verifyAccountPasswordSnapshot(ctx, callsite, passwordHash, password)
	if err != nil {
		return false, err
	}
	if !valid {
		return false, nil
	}
	if upgrade {
		// Authentication completion never performs a second expensive bcrypt.
		// A bounded, de-duplicated worker upgrades the exact hash with CAS after
		// the successful login has left the cold-start wave.
		schedulePasswordHashUpgrade(s, userID, passwordHash, password)
	}
	return true, nil
}

func (s *Server) verifyAndUpgradeLocalPasswordSnapshot(ctx context.Context, callsite kdfCallsite, userID, passwordHash, password string) (bool, string, error) {
	valid, upgrade, verifiedHash, err := verifyAccountPasswordSnapshot(ctx, callsite, passwordHash, password)
	if err != nil || !valid {
		return valid, "", err
	}
	if upgrade {
		schedulePasswordHashUpgrade(s, userID, verifiedHash, password)
	}
	return true, verifiedHash, nil
}
