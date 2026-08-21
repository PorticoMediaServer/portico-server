package app

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	accountPasswordMinRunes      = 8
	accountPasswordMaxBytes      = 72
	currentPasswordBcryptCost    = 12
	accountPasswordPolicyMessage = "Password must be at least 8 characters with an uppercase letter, a lowercase letter, and a number or special character, and no more than 72 UTF-8 bytes."
)

// This is a cost-12 hash for a fixed value that is never accepted. It keeps
// unknown-user, disabled-password, and malformed-hash verification on the same
// expensive bcrypt path as valid accounts without generating work per request.
const passwordTimingEqualizerHash = "$2y$12$9yHLhtrw/pfDmCaOhQFOMOhLGSeFNFk7PTKQhs2LBGg5NWSJPLMKK"

var errAccountPasswordPolicy = errors.New("password does not satisfy the account password policy")

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

func hashAccountPassword(password string) (string, error) {
	if !validAccountPassword(password) {
		return "", errAccountPasswordPolicy
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), currentPasswordBcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyAccountPassword(passwordHash, password string) (valid bool, upgrade bool) {
	passwordHash = strings.TrimSpace(passwordHash)
	eligible := passwordHash != "" && utf8.ValidString(password) && len([]byte(password)) <= accountPasswordMaxBytes
	hash := []byte(passwordHash)
	candidate := []byte(password)
	if !eligible {
		hash = []byte(passwordTimingEqualizerHash)
		candidate = []byte("portico-invalid-password")
	}
	if bcrypt.CompareHashAndPassword(hash, candidate) != nil || !eligible {
		return false, false
	}
	cost, err := bcrypt.Cost(hash)
	if err != nil {
		return false, false
	}
	return true, cost < currentPasswordBcryptCost
}

func (s *Server) verifyAndUpgradeLocalPassword(ctx context.Context, userID, passwordHash, password string) (bool, error) {
	valid, upgrade := verifyAccountPassword(passwordHash, password)
	if !valid {
		return false, nil
	}
	if !upgrade {
		return true, nil
	}
	// Cost upgrades intentionally accept existing passwords that predate the
	// current creation policy so policy changes cannot lock out valid accounts.
	replacementBytes, err := bcrypt.GenerateFromPassword([]byte(password), currentPasswordBcryptCost)
	if err != nil {
		return false, err
	}
	replacement := string(replacementBytes)
	result, err := s.execUserWrite(ctx, `
		UPDATE users SET password_hash = ?, updated_at = ?
		WHERE id = ? AND password_hash = ?`, replacement, time.Now().UTC().Format(time.RFC3339), userID, passwordHash)
	if err != nil {
		return false, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		// A concurrent successful login may have already upgraded the same hash.
		var currentHash string
		if err := s.queryUserRow(ctx, `SELECT COALESCE(password_hash, '') FROM users WHERE id = ?`, userID).Scan(&currentHash); err != nil {
			return false, err
		}
		currentValid, _ := verifyAccountPassword(currentHash, password)
		return currentValid, nil
	}
	return true, nil
}
