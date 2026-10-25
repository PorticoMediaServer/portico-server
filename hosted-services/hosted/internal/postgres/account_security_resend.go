package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/security"
)

const emailChangeResendLimit = 5

// ResendEmailChange can rotate only an already proven, still-pending challenge
// belonging to this exact family, epoch, generation and access credential. It
// never changes the destination, extends the deadline or resets failed guesses.
func (s *AccountSecurity) ResendEmailChange(ctx context.Context, token, id, operation string) (identity.SecurityResult, error) {
	var out identity.SecurityResult
	if len(id) < 1 || len(id) > 160 || len(operation) < 16 || len(operation) > 128 || strings.ContainsAny(id+operation, "\r\n\x00") {
		return out, identity.ErrInvalid
	}
	request := s.requestHash(struct{ ChallengeID string }{id})
	found, err := s.readReceipt(ctx, token, operation, "email_resend", request, &out)
	if err != nil || found {
		return out, err
	}
	if !s.MailEnabled {
		return out, identity.ErrMailUnavailable
	}
	tx, expected, err := s.authorize(ctx, token)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	found, err = s.readReceiptFrom(ctx, tx, expected.TokenHash, operation, "email_resend", request, &out)
	if err != nil || found {
		return out, err
	}
	var address string
	var previous []byte
	var expires, checked time.Time
	var revision int64
	err = tx.QueryRowContext(ctx, `SELECT address,expires_at,revision,code_hash FROM account_email_challenges WHERE id=$1 AND account_id=$2 AND purpose='email_change' AND account_epoch=$3 AND family_id=$4 AND family_generation=$5 AND access_hash=$6 AND consumed_at IS NULL AND superseded_at IS NULL AND expires_at>clock_timestamp() AND attempts<5 FOR UPDATE`, id, expected.AccountID, expected.AuthEpoch, expected.FamilyID, expected.FamilyGeneration, expected.TokenHash).Scan(&address, &expires, &revision, &previous)
	if err != nil {
		return out, currentChallengeError(err)
	}
	if revision < 1 || revision > emailChangeResendLimit {
		return out, identity.ErrRateLimited
	}
	var code string
	var digest []byte
	for n := 0; n < 8; n++ {
		code, err = identity.NewEmailCode()
		if err != nil {
			return out, err
		}
		digest = s.digest("email-change", id, code)
		if !equalSecret(digest, previous) {
			break
		}
		code = ""
	}
	if code == "" {
		return out, errors.New("could not rotate email challenge")
	}
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&checked); err != nil {
		return out, err
	}
	if !expires.After(checked) {
		return out, identity.ErrChallenge
	}
	// The existing outbox authorization compares this revision before delivery,
	// so any older queued code becomes undeliverable with the same transaction.
	if _, err = tx.ExecContext(ctx, `UPDATE account_email_challenges SET code_hash=$2,revision=revision+1 WHERE id=$1`, id, digest); err != nil {
		return out, err
	}
	if err = EnqueueAccountMailTx(ctx, tx, s.Keys, mailoutbox.Intent{ID: security.RandomID("mail_"), Kind: mailoutbox.EmailChange, ResourceID: id, Revision: revision + 1, AccountID: expected.AccountID, AccountEpoch: expected.AuthEpoch, ExpiresAt: expires, Payload: mailoutbox.Payload{To: address, Code: code}}); err != nil {
		return out, err
	}
	remaining := emailChangeResendLimit - int(revision)
	out = identity.SecurityResult{AccountID: expected.AccountID, FamilyID: expected.FamilyID, Status: "contact_pending", ChallengeID: id, MaskedAddress: maskedContact(address), ExpiresAt: &expires, ResendsRemaining: &remaining}
	if err = s.saveReceipt(ctx, tx, expected, identity.SecurityIntent{Purpose: "email_resend", OperationID: operation}, request, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
