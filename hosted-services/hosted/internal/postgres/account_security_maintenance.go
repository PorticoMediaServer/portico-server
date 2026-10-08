package postgres

import "context"

// Each sweep is bounded; expired proofs remain unusable even before cleanup.
func (s *AccountSecurity) Prune(ctx context.Context) error {
	statements := []string{
		`DELETE FROM account_password_logins WHERE (binding_hash,request_id) IN(SELECT binding_hash,request_id FROM account_password_logins WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT 100)`,
		`DELETE FROM account_registrations WHERE id IN(SELECT id FROM account_registrations WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT 100)`,
		`DELETE FROM account_email_challenges WHERE id IN(SELECT id FROM account_email_challenges WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT 100)`,
		`DELETE FROM account_mfa_challenges WHERE id IN(SELECT id FROM account_mfa_challenges WHERE expires_at<clock_timestamp() AND COALESCE(receipt_expires_at,expires_at)<clock_timestamp() ORDER BY expires_at LIMIT 100)`,
		`DELETE FROM account_recent_proofs WHERE proof_hash IN(SELECT proof_hash FROM account_recent_proofs WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT 100)`,
		`DELETE FROM account_security_receipts WHERE (account_id,operation_id,purpose) IN(SELECT account_id,operation_id,purpose FROM account_security_receipts WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT 100)`,
		`UPDATE account_mfa SET pending_secret=NULL,pending_id=NULL,pending_expires_at=NULL WHERE account_id IN(SELECT account_id FROM account_mfa WHERE pending_expires_at<clock_timestamp() LIMIT 100)`,
		`UPDATE account_mail_outbox SET status='cancelled',ciphertext=NULL,lease_id=NULL,lease_until=NULL,last_failure='expired' WHERE id IN(SELECT id FROM account_mail_outbox WHERE expires_at<clock_timestamp() AND status IN('pending','sending') ORDER BY expires_at LIMIT 100)`,
		`DELETE FROM account_mail_outbox WHERE id IN(SELECT id FROM account_mail_outbox WHERE expires_at<clock_timestamp()-interval '7 days' AND status IN('sent','failed','cancelled') ORDER BY expires_at LIMIT 100)`,
		`DELETE FROM deleted_identity_markers WHERE (marker_key_id,subject_digest) IN(SELECT marker_key_id,subject_digest FROM deleted_identity_markers WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT 100)`,
		`DELETE FROM account_security_limits WHERE (subject_hash,kind) IN(SELECT subject_hash,kind FROM account_security_limits WHERE window_start<clock_timestamp()-interval '2 days' ORDER BY window_start LIMIT 100)`,
	}
	return s.Store.pruneStatements(ctx, statements)
}
