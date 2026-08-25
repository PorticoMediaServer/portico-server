package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	errInvalidCredentials       = errors.New("invalid credentials")
	errAccountEmailInUse        = errors.New("account email is already in use")
	errPrivilegedSessionChanged = errors.New("interactive authorization session changed")
)

// verifyLocalPasswordSnapshot loads and compares the account credential.  The
// returned hash is the exact value that was compared and must be passed to
// validatePasswordSessionTx before a privileged mutation is committed.
func (s *Server) verifyLocalPasswordSnapshot(ctx context.Context, callsite kdfCallsite, accountID, password string) (string, error) {
	accountID = strings.TrimSpace(accountID)
	var passwordHash string
	if err := s.queryUserRow(ctx, `SELECT COALESCE(password_hash, '') FROM users WHERE id = ? AND COALESCE(disabled_at, '') = ''`, accountID).Scan(&passwordHash); err != nil {
		return "", err
	}
	valid, _, verifiedHash, err := verifyAccountPasswordSnapshot(ctx, callsite, passwordHash, password)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", errInvalidCredentials
	}
	return verifiedHash, nil
}

// validatePasswordSessionTx is the final authority fence for a request that
// authenticated with an account password.  It is intentionally a single
// transaction query so password rotation, account disablement, device/session
// revocation, and the privileged write are ordered by the database commit.
// expectedHash may be empty for PIN-authorized operations, which still require
// an active account and bound interactive session.
func validatePasswordSessionTx(tx *sql.Tx, accountID, profileID, sessionID, expectedHash string, now time.Time) error {
	accountID = strings.TrimSpace(accountID)
	profileID = strings.TrimSpace(profileID)
	sessionID = strings.TrimSpace(sessionID)
	if profileID == "" {
		profileID = accountID
	}
	if accountID == "" || sessionID == "" {
		return errPrivilegedSessionChanged
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var count int
	err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM users u
		JOIN sessions session ON session.user_id = u.id
		LEFT JOIN devices device ON device.id = session.device_id AND device.user_id = session.user_id
		WHERE u.id = ? AND COALESCE(u.disabled_at, '') = ''
		  AND (? = '' OR COALESCE(u.password_hash, '') = ?)
		  AND session.id = ? AND session.user_id = ?
		  AND COALESCE(NULLIF(session.profile_id, ''), session.user_id) = ?
		  AND session.expires_at > ?
		  AND (
			session.device_id = '' OR (
				device.id IS NOT NULL
				AND device.user_id = session.user_id
				AND COALESCE(device.revoked_at, '') = ''
			)
		  )`,
		accountID, expectedHash, expectedHash, sessionID, accountID, profileID, now.UTC().Format(time.RFC3339Nano)).Scan(&count)
	if err != nil {
		return err
	}
	if count != 1 {
		if expectedHash != "" {
			return errPasswordCredentialChanged
		}
		return errPrivilegedSessionChanged
	}
	return nil
}
