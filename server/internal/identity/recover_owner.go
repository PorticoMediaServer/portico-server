package identity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/notify"
)

// RecoverOwner is invoked only by the local executable, never by an HTTP route.
// It returns a temporary password once. It cannot grant a viewing session: the
// ordinary sign-in path requires the owner to replace this credential first.
func (s *Service) RecoverOwner(ctx context.Context, resetMFA bool) (string, error) {
	return s.recoverOwner(ctx, resetMFA)
}

// RecoverOwnerCLI deliberately needs only the existing database. It neither
// starts sign-in services nor prints a setup code. There is no key to lose:
// identity values are plain database columns.
func RecoverOwnerCLI(ctx context.Context, db *sql.DB, resetMFA bool) (string, error) {
	return (&Service{db: db}).recoverOwner(ctx, resetMFA)
}

func (s *Service) recoverOwner(ctx context.Context, resetMFA bool) (string, error) {
	temporary := Token() + "aA1!"
	hash, err := bcrypt.GenerateFromPassword([]byte(temporary), PasswordCost)
	if err != nil {
		return "", err
	}
	err = dbwork.WithWriteTx(ctx, s.db, dbwork.ClassSecurityFence, func(tx *sql.Tx) error {
		var account string
		if err := tx.QueryRowContext(ctx, `SELECT account_id FROM direct_memberships WHERE role='owner'`).Scan(&account); err != nil {
			return err
		}
		if err := s.revokeCredentialsTx(ctx, tx, account); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET password_hash=?,epoch=epoch+1 WHERE id=?`, hash, account); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE direct_memberships SET disabled=0,revision=revision+1 WHERE account_id=?`, account); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO identity_credential_state(account_id,must_change) VALUES(?,1) ON CONFLICT(account_id) DO UPDATE SET must_change=1`, account); err != nil {
			return err
		}
		for _, table := range []string{"identity_credential_attempts", "identity_account_attempt_budget"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE account_id=?`, account); err != nil {
				return err
			}
		}
		// C58: every account whose two-factor setup this recovery removes is told
		// in-app (never by email) and prompted to set it up again.
		affected := map[string]bool{}
		if resetMFA {
			affected[account] = true
		}
		for id := range affected {
			var enabled bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM identity_account_factors WHERE account_id=?)`, id).Scan(&enabled); err != nil {
				return err
			}
			if enabled {
				if err := twoFactorResetNoticeTx(ctx, tx, id); err != nil {
					return err
				}
			}
		}
		if resetMFA {
			for _, table := range []string{"identity_account_factors", "identity_recovery_codes", "identity_factor_challenges"} {
				if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE account_id=?`, account); err != nil {
					return err
				}
			}
		}
		return s.securityNoticeTx(ctx, tx, account, "security.owner_recovery")
	})
	if err != nil {
		return "", err
	}
	return temporary, nil
}

// twoFactorResetNoticeTx tells one account, in the app, that owner recovery
// turned off its two-factor sign-in, with a prompt to set it up again (C58).
func twoFactorResetNoticeTx(ctx context.Context, tx *sql.Tx, account string) error {
	var profile string
	if err := tx.QueryRowContext(ctx, `SELECT profile_id FROM accounts WHERE id=?`, account).Scan(&profile); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	_, err := notify.Raise(tx, time.Now().UnixMilli(), notify.Draft{
		Audience: notify.AudienceProfile, Scope: notify.ProfileScope("local", account, profile), Severity: notify.SeverityWarning, Source: notify.SourceSecurity,
		Category: "two_factor_reset", DedupeKey: "two_factor_reset:" + Token(),
		Title:     "Two-factor sign-in was turned off",
		Body:      "The server owner recovered this server, which turned off two-factor sign-in for your account. Set it up again to keep your account protected.",
		Arguments: map[string]string{"code": "two_factor_reset"},
		Actions:   []notify.Action{{Kind: "navigate", Label: "Set up two-factor sign-in", Target: &notify.NavigateTarget{View: "settings-security"}}},
	})
	return err
}
