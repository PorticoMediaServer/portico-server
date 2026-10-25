package postgres

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/security"
	"strings"
	"time"
)

func (s *AccountSecurity) Mutate(ctx context.Context, token string, q SecurityAction) (identity.SecurityResult, error) {
	var out identity.SecurityResult
	if !q.SecurityIntent.Valid() || q.NeedsProof() && (len(q.Proof) < 32 || len(q.Proof) > 128) || len(q.Proof) > 128 {
		return out, identity.ErrInvalid
	}
	// Receipt recovery enforces current authority (or an exact terminal epoch)
	// before opening the sealed response; it is not an authorization bypass.
	request := s.requestHash(q)
	found, e := s.readReceipt(ctx, token, q.OperationID, q.Purpose, request, &out)
	if e != nil || found {
		return out, e
	}
	var password []byte
	if q.Purpose == "password_set_or_change" {
		password, e = s.Identity.HashPassword(ctx, q.NewPassword)
		if e != nil {
			return out, e
		}
	} else if q.NewPassword != "" {
		return out, identity.ErrInvalid
	}
	if q.Purpose == "email_change" {
		if !s.MailEnabled {
			return out, identity.ErrMailUnavailable
		}
		normalized, err := NormalizeContact(q.Target)
		if err != nil || normalized != q.Target {
			return out, identity.ErrInvalid
		}
	}
	tx, expected, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	found, e = s.readReceiptFrom(ctx, tx, expected.TokenHash, q.OperationID, q.Purpose, request, &out)
	if e != nil || found {
		return out, e
	}
	if q.NeedsProof() {
		if e = s.proofTx(ctx, tx, expected, q.SecurityIntent, q.Proof); e != nil {
			return out, e
		}
	}
	out = identity.SecurityResult{AccountID: expected.AccountID, FamilyID: expected.FamilyID, Status: "updated"}
	switch q.Purpose {
	case "password_set_or_change":
		if _, e = tx.ExecContext(ctx, `UPDATE accounts SET password_hash=$2 WHERE id=$1`, expected.AccountID, password); e != nil {
			return out, e
		}
		if e = s.notifyTx(ctx, tx, expected.AccountID, "password_changed", q.OperationID); e != nil {
			return out, e
		}
		if e = revokeAccountTx(ctx, tx, expected.AccountID, "password_change"); e != nil {
			return out, e
		}
		out.SignOut = true
	case "email_change":
		var current string
		e = tx.QueryRowContext(ctx, `SELECT address FROM account_contact_emails WHERE account_id=$1 AND is_primary`, expected.AccountID).Scan(&current)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return out, e
		}
		if strings.EqualFold(current, q.Target) {
			return out, identity.ErrConflict
		}
		if _, e = tx.ExecContext(ctx, `UPDATE account_email_challenges SET superseded_at=clock_timestamp() WHERE account_id=$1 AND purpose='email_change' AND consumed_at IS NULL AND superseded_at IS NULL`, expected.AccountID); e != nil {
			return out, e
		}
		code, err := identity.NewEmailCode()
		if err != nil {
			return out, err
		}
		id := security.RandomID("email_")
		var expires time.Time
		e = tx.QueryRowContext(ctx, `INSERT INTO account_email_challenges(id,account_id,purpose,address,account_epoch,family_id,family_generation,access_hash,code_hash,expires_at,operation_id,request_hash) VALUES($1,$2,'email_change',$3,$4,$5,$6,$7,$8,statement_timestamp()+interval '10 minutes',$9,$10) RETURNING expires_at`, id, expected.AccountID, q.Target, expected.AuthEpoch, expected.FamilyID, expected.FamilyGeneration, expected.TokenHash, s.digest("email-change", id, code), q.OperationID, request).Scan(&expires)
		if e != nil {
			return out, e
		}
		if e = EnqueueAccountMailTx(ctx, tx, s.Keys, mailoutbox.Intent{ID: security.RandomID("mail_"), Kind: mailoutbox.EmailChange, ResourceID: id, Revision: 1, AccountID: expected.AccountID, AccountEpoch: expected.AuthEpoch, ExpiresAt: expires, Payload: mailoutbox.Payload{To: q.Target, Code: code}}); e != nil {
			return out, e
		}
		out.Status = "contact_pending"
		out.ChallengeID = id
		out.ExpiresAt = &expires
		out.MaskedAddress = maskedContact(q.Target)
		remaining := emailChangeResendLimit
		out.ResendsRemaining = &remaining
	case "mfa_enroll":
		var eligible bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_contact_emails WHERE account_id=$1 AND is_primary) AND NOT EXISTS(SELECT 1 FROM account_mfa WHERE account_id=$1 AND secret IS NOT NULL)`, expected.AccountID).Scan(&eligible)
		if e != nil {
			return out, e
		}
		if !eligible {
			return out, identity.ErrInvalid
		}
		secret, err := identity.NewTOTPSecret()
		if err != nil {
			return out, err
		}
		sealed, err := s.Keys.Seal(secret, "totp:"+expected.AccountID)
		if err != nil {
			return out, err
		}
		id := security.RandomID("enroll_")
		var expires time.Time
		e = tx.QueryRowContext(ctx, `INSERT INTO account_mfa(account_id,pending_secret,pending_id,pending_expires_at,pending_family_id,pending_family_generation,pending_epoch,pending_access_hash) VALUES($1,$2,$3,statement_timestamp()+interval '10 minutes',$4,$5,$6,$7) ON CONFLICT(account_id) DO UPDATE SET pending_secret=EXCLUDED.pending_secret,pending_id=EXCLUDED.pending_id,pending_expires_at=EXCLUDED.pending_expires_at,pending_family_id=EXCLUDED.pending_family_id,pending_family_generation=EXCLUDED.pending_family_generation,pending_epoch=EXCLUDED.pending_epoch,pending_access_hash=EXCLUDED.pending_access_hash,pending_attempts=0,revision=account_mfa.revision+1 RETURNING pending_expires_at`, expected.AccountID, sealed, id, expected.FamilyID, expected.FamilyGeneration, expected.AuthEpoch, expected.TokenHash).Scan(&expires)
		if e != nil {
			return out, e
		}
		var username string
		if e = tx.QueryRowContext(ctx, `SELECT username FROM accounts WHERE id=$1`, expected.AccountID).Scan(&username); e != nil {
			return out, e
		}
		out.Status = "enrollment_pending"
		out.ChallengeID = id
		out.ExpiresAt = &expires
		out.Secret = secret
		out.OTPAuthURI = identity.TOTPURI(secret, username)
	case "mfa_disable":
		result, err := tx.ExecContext(ctx, `UPDATE account_mfa SET secret=NULL,last_step=-1,pending_secret=NULL,pending_id=NULL,pending_expires_at=NULL,revision=revision+1 WHERE account_id=$1 AND secret IS NOT NULL`, expected.AccountID)
		if err != nil {
			return out, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return out, err
		}
		if n != 1 {
			return out, identity.ErrInvalid
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM account_recovery_codes WHERE account_id=$1`, expected.AccountID); e != nil {
			return out, e
		}
		if e = s.notifyTx(ctx, tx, expected.AccountID, "two_step_disabled", q.OperationID); e != nil {
			return out, e
		}
		if e = revokeAccountTx(ctx, tx, expected.AccountID, "mfa_disabled"); e != nil {
			return out, e
		}
		out.SignOut = true
	case "recovery_codes":
		var enabled bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_mfa WHERE account_id=$1 AND secret IS NOT NULL)`, expected.AccountID).Scan(&enabled); e != nil {
			return out, e
		}
		if !enabled {
			return out, identity.ErrInvalid
		}
		out.RecoveryCodes, e = s.replaceRecoveryTx(ctx, tx, expected.AccountID)
		if e != nil {
			return out, e
		}
		if e = s.notifyTx(ctx, tx, expected.AccountID, "recovery_codes_replaced", q.OperationID); e != nil {
			return out, e
		}
	case "provider_unlink":
		var provider string
		e = tx.QueryRowContext(ctx, `SELECT provider FROM account_external_identities WHERE id=$1 AND account_id=$2 AND revoked_at IS NULL FOR UPDATE`, q.Target, expected.AccountID).Scan(&provider)
		if e != nil {
			return out, identity.ErrProviderConflict
		}
		var usable bool
		e = tx.QueryRowContext(ctx, `SELECT a.password_hash IS NOT NULL OR EXISTS(SELECT 1 FROM account_external_identities i JOIN identity_provider_descriptors d ON d.provider=i.provider AND d.enabled WHERE i.account_id=a.id AND i.id<>$2 AND i.revoked_at IS NULL) FROM accounts a WHERE a.id=$1`, expected.AccountID, q.Target).Scan(&usable)
		if e != nil {
			return out, e
		}
		if !usable {
			return out, identity.ErrLastMethod
		}
		if _, e = tx.ExecContext(ctx, `UPDATE account_external_identities SET revoked_at=clock_timestamp(),revision=revision+1 WHERE id=$1`, q.Target); e != nil {
			return out, e
		}
		if e = s.notifyTx(ctx, tx, expected.AccountID, "sign_in_method_removed", q.OperationID); e != nil {
			return out, e
		}
		// Families don't record how they signed in, so every other device is
		// signed out; the caller just acted on a live session and stays
		// signed in (lead rule, 24 Sep, reversible).
		if e = revokeOtherFamiliesTx(ctx, tx, expected.AccountID, expected.FamilyID, "provider_unlinked"); e != nil {
			return out, e
		}
	case "account_deletion_preview":
		preview, err := s.prepareDeletionCurrentTx(ctx, tx, expected)
		if err != nil {
			return out, err
		}
		out.Status = "deletion_preview"
		out.DeletionPreview = &preview
	default:
		return out, identity.ErrInvalid
	}
	if e = audit(ctx, tx, expected.AccountID, "account.security."+q.Purpose, expected.AccountID, expected.AuthEpoch); e != nil {
		return out, e
	}
	if e = s.saveReceipt(ctx, tx, expected, q.SecurityIntent, request, out); e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func (s *AccountSecurity) ConfirmEnrollment(ctx context.Context, token, id, code, operation string) (identity.SecurityResult, error) {
	var out identity.SecurityResult
	if len(operation) < 16 || len(operation) > 128 {
		return out, identity.ErrInvalid
	}
	request := s.requestHash(struct{ ID, Code string }{id, code})
	found, e := s.readReceipt(ctx, token, operation, "mfa_confirm", request, &out)
	if e != nil || found {
		return out, e
	}
	if e = s.Limit(ctx, "mfa-enrollment", id, 10, 15*time.Minute); e != nil {
		return out, e
	}
	tx, expected, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	found, e = s.readReceiptFrom(ctx, tx, expected.TokenHash, operation, "mfa_confirm", request, &out)
	if e != nil || found {
		return out, e
	}
	var cipher []byte
	var challengeExpiry, checkedAt time.Time
	e = tx.QueryRowContext(ctx, `SELECT pending_secret,pending_expires_at FROM account_mfa WHERE account_id=$1 AND pending_id=$2 AND pending_expires_at>clock_timestamp() AND pending_family_id=$3 AND pending_family_generation=$4 AND pending_epoch=$5 AND pending_access_hash=$6 AND pending_attempts<5 AND secret IS NULL FOR UPDATE`, expected.AccountID, id, expected.FamilyID, expected.FamilyGeneration, expected.AuthEpoch, expected.TokenHash).Scan(&cipher, &challengeExpiry)
	if e != nil {
		return out, currentChallengeError(e)
	}
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&checkedAt); e != nil {
		return out, e
	}
	if !challengeExpiry.After(checkedAt) {
		return out, identity.ErrChallenge
	}
	secret, e := s.Keys.Open(cipher, "totp:"+expected.AccountID)
	if e != nil {
		return out, e
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	step, valid := identity.TOTPStep(secret, strings.TrimSpace(code), now, -1)
	if !valid {
		if _, e = tx.ExecContext(ctx, `UPDATE account_mfa SET pending_attempts=pending_attempts+1 WHERE account_id=$1`, expected.AccountID); e != nil {
			return out, e
		}
		if e = tx.Commit(); e != nil {
			return out, e
		}
		return out, identity.ErrChallenge
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_mfa SET secret=pending_secret,pending_secret=NULL,pending_id=NULL,pending_expires_at=NULL,last_step=$2,revision=revision+1 WHERE account_id=$1`, expected.AccountID, step); e != nil {
		return out, e
	}
	codes, e := s.replaceRecoveryTx(ctx, tx, expected.AccountID)
	if e != nil {
		return out, e
	}
	if e = s.notifyTx(ctx, tx, expected.AccountID, "two_step_enabled", operation); e != nil {
		return out, e
	}
	if e = revokeAccountTx(ctx, tx, expected.AccountID, "mfa_enabled"); e != nil {
		return out, e
	}
	out = identity.SecurityResult{AccountID: expected.AccountID, FamilyID: expected.FamilyID, Status: "mfa_enabled", RecoveryCodes: codes, SignOut: true}
	if e = audit(ctx, tx, expected.AccountID, "account.mfa_enabled", expected.AccountID, 0); e != nil {
		return out, e
	}
	if e = s.saveReceipt(ctx, tx, expected, identity.SecurityIntent{Purpose: "mfa_confirm", OperationID: operation}, request, out); e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func (s *AccountSecurity) ConfirmEmailChange(ctx context.Context, token, id, code, operation string) (identity.SecurityResult, error) {
	var out identity.SecurityResult
	if len(operation) < 16 || len(operation) > 128 {
		return out, identity.ErrInvalid
	}
	request := s.requestHash(struct{ ID, Code string }{id, code})
	found, e := s.readReceipt(ctx, token, operation, "email_confirm", request, &out)
	if e != nil || found {
		return out, e
	}
	if e = s.Limit(ctx, "email-confirm", id, 10, 15*time.Minute); e != nil {
		return out, e
	}
	tx, expected, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	found, e = s.readReceiptFrom(ctx, tx, expected.TokenHash, operation, "email_confirm", request, &out)
	if e != nil || found {
		return out, e
	}
	var address string
	var challengeExpiry, checkedAt time.Time
	var hash []byte
	e = tx.QueryRowContext(ctx, `SELECT address,code_hash,expires_at FROM account_email_challenges WHERE id=$1 AND account_id=$2 AND purpose='email_change' AND account_epoch=$3 AND family_id=$4 AND family_generation=$5 AND access_hash=$6 AND consumed_at IS NULL AND superseded_at IS NULL AND expires_at>clock_timestamp() AND attempts<5 FOR UPDATE`, id, expected.AccountID, expected.AuthEpoch, expected.FamilyID, expected.FamilyGeneration, expected.TokenHash).Scan(&address, &hash, &challengeExpiry)
	if e != nil {
		return out, currentChallengeError(e)
	}
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&checkedAt); e != nil {
		return out, e
	}
	if !challengeExpiry.After(checkedAt) {
		return out, identity.ErrChallenge
	}
	if !equalSecret(hash, s.digest("email-change", id, strings.TrimSpace(code))) {
		if _, e = tx.ExecContext(ctx, `UPDATE account_email_challenges SET attempts=attempts+1 WHERE id=$1`, id); e != nil {
			return out, e
		}
		if e = tx.Commit(); e != nil {
			return out, e
		}
		return out, identity.ErrChallenge
	}
	if e = s.notifyTx(ctx, tx, expected.AccountID, "contact_email_changed", operation+":old"); e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_contact_emails SET is_primary=false WHERE account_id=$1`, expected.AccountID); e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO account_contact_emails(id,account_id,address,verified_at,source,is_primary) VALUES($1,$2,$3,clock_timestamp(),'challenge',true) ON CONFLICT(account_id,address) DO UPDATE SET is_primary=true,verified_at=clock_timestamp(),source='challenge',source_identity_id=NULL,private_relay=false`, security.RandomID("contact_"), expected.AccountID, address); e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_email_challenges SET consumed_at=clock_timestamp() WHERE id=$1`, id); e != nil {
		return out, e
	}
	if e = s.notifyTx(ctx, tx, expected.AccountID, "contact_email_verified", operation+":new"); e != nil {
		return out, e
	}
	if e = revokeAccountTx(ctx, tx, expected.AccountID, "email_changed"); e != nil {
		return out, e
	}
	out = identity.SecurityResult{AccountID: expected.AccountID, FamilyID: expected.FamilyID, Status: "email_changed", SignOut: true}
	if e = s.saveReceipt(ctx, tx, expected, identity.SecurityIntent{Purpose: "email_confirm", OperationID: operation}, request, out); e != nil {
		return out, e
	}
	if e = audit(ctx, tx, expected.AccountID, "account.email_changed", expected.AccountID, 0); e != nil {
		return out, e
	}
	return out, tx.Commit()
}
