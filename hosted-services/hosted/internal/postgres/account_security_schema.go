package postgres

import (
	"context"
	"database/sql"
	"errors"
)

// InstallAccountSecuritySchemaTx is an additive, feature-owned migration from
// the supplied current Hosted format. It never renumbers another package's
// schema contract. The outer initialization transaction holds the migration lock.
func InstallAccountSecuritySchemaTx(ctx context.Context, tx *sql.Tx) error {
	var installed bool
	if err := tx.QueryRowContext(ctx, `SELECT to_regclass(current_schema()||'.account_security_schema') IS NOT NULL`).Scan(&installed); err != nil {
		return err
	}
	if installed {
		var version int
		if err := tx.QueryRowContext(ctx, `SELECT version FROM account_security_schema WHERE singleton`).Scan(&version); err != nil {
			return err
		}
		if version != 1 {
			return errors.New("unsupported account-security schema")
		}
		return installReceiptAuthorityTx(ctx, tx)
	}
	if _, err := tx.ExecContext(ctx, accountSecuritySchema); err != nil {
		return err
	}
	return installReceiptAuthorityTx(ctx, tx)
}

const accountSecuritySchema = `
CREATE TABLE account_security_schema(singleton boolean PRIMARY KEY CHECK(singleton),version integer NOT NULL CHECK(version=1));
ALTER TABLE device_authorizations ADD COLUMN session_mode text NOT NULL DEFAULT 'native' CHECK(session_mode IN('native','browser'));
CREATE TABLE account_registrations(
 id text PRIMARY KEY, binding_hash bytea NOT NULL CHECK(octet_length(binding_hash)=32),
 request_id text NOT NULL, request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),
 session_mode text NOT NULL CHECK(session_mode IN('native','browser')),
 username text NOT NULL, display_name text NOT NULL, address text NOT NULL,
 password_hash bytea, code_hash bytea, revision bigint NOT NULL DEFAULT 1,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5), resend_count integer NOT NULL DEFAULT 0 CHECK(resend_count BETWEEN 0 AND 5),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL,code_expires_at timestamptz NOT NULL,
 last_resend_request text, verified_at timestamptz, completed_at timestamptz, cancelled_at timestamptz,
 account_id text, family_id text, account_epoch bigint, family_generation bigint, receipt bytea, receipt_expires_at timestamptz,
 UNIQUE(binding_hash,request_id),CHECK(expires_at>created_at),CHECK(code_expires_at<=expires_at)
);
CREATE INDEX account_registration_binding ON account_registrations(binding_hash,created_at DESC);
CREATE INDEX account_registration_expiry ON account_registrations(expires_at,id);
CREATE TABLE account_password_logins(
 binding_hash bytea NOT NULL,request_id text NOT NULL,request_hash bytea NOT NULL,
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 account_epoch bigint NOT NULL,session_mode text NOT NULL,ciphertext bytea NOT NULL,
 expires_at timestamptz NOT NULL,PRIMARY KEY(binding_hash,request_id)
);
CREATE INDEX account_password_login_expiry ON account_password_logins(expires_at);
CREATE TABLE account_email_challenges(
 id text PRIMARY KEY,account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 purpose text NOT NULL CHECK(purpose IN('password_reset','email_change')),
 address text NOT NULL, contact_id text, account_epoch bigint NOT NULL,
 family_id text, family_generation bigint, access_hash bytea,
 password_fingerprint bytea, code_hash bytea NOT NULL UNIQUE,
 revision bigint NOT NULL DEFAULT 1,attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL,
 consumed_at timestamptz, superseded_at timestamptz,
 operation_id text NOT NULL, request_hash bytea NOT NULL, completion_hash bytea,
 UNIQUE(account_id,purpose,operation_id)
);
CREATE INDEX account_email_challenge_expiry ON account_email_challenges(expires_at,id);
CREATE INDEX account_email_challenge_live ON account_email_challenges(account_id,purpose) WHERE consumed_at IS NULL AND superseded_at IS NULL;
CREATE TABLE account_mfa(
 account_id text PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 secret bytea, last_step bigint NOT NULL DEFAULT -1,
 revision bigint NOT NULL DEFAULT 1,
 pending_secret bytea,pending_id text,pending_expires_at timestamptz,
 pending_family_id text,pending_family_generation bigint,pending_epoch bigint,pending_access_hash bytea,
 pending_attempts integer NOT NULL DEFAULT 0 CHECK(pending_attempts BETWEEN 0 AND 5),
 CHECK((pending_secret IS NULL)=(pending_id IS NULL))
);
CREATE TABLE account_recovery_codes(
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 code_hash bytea NOT NULL CHECK(octet_length(code_hash)=32),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),consumed_at timestamptz,
 PRIMARY KEY(account_id,code_hash)
);
CREATE TABLE account_mfa_challenges(
 id text PRIMARY KEY,proof_hash bytea UNIQUE NOT NULL CHECK(octet_length(proof_hash)=32),
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,account_epoch bigint NOT NULL,
 purpose text NOT NULL CHECK(purpose='login'),session_mode text NOT NULL CHECK(session_mode IN('native','browser')),
 identity_transaction_id text REFERENCES identity_transactions(id) ON DELETE CASCADE,
 credential_kind text NOT NULL CHECK(credential_kind IN('password','provider')),
 password_hash bytea,identity_id text,identity_revision bigint,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),consumed_at timestamptz,
 family_id text,family_generation bigint,receipt bytea,receipt_expires_at timestamptz,completion_hash bytea
);
CREATE INDEX account_mfa_challenge_expiry ON account_mfa_challenges(expires_at,id);
CREATE TABLE account_security_receipts(
 account_id text NOT NULL,operation_id text NOT NULL,purpose text NOT NULL,
 family_id text NOT NULL,access_hash bytea NOT NULL,request_hash bytea NOT NULL,
 ciphertext bytea NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL,
 PRIMARY KEY(account_id,operation_id,purpose)
);
CREATE INDEX account_security_receipt_lookup ON account_security_receipts(access_hash,operation_id,purpose);
CREATE INDEX account_security_receipt_expiry ON account_security_receipts(expires_at,account_id,operation_id);
CREATE TABLE account_security_limits(
 subject_hash bytea NOT NULL,kind text NOT NULL,window_start timestamptz NOT NULL,attempts integer NOT NULL,
 PRIMARY KEY(subject_hash,kind)
);
CREATE INDEX account_security_limit_expiry ON account_security_limits(window_start);
CREATE TABLE account_mail_outbox(
 id text PRIMARY KEY CHECK(id ~ '^[A-Za-z0-9_-]{16,128}$'),
 kind text NOT NULL CHECK(kind IN('registration','oidc_contact','password_reset','email_change','security_notice','invitation')),
 resource_id text NOT NULL,revision bigint NOT NULL,account_id text,account_epoch bigint NOT NULL DEFAULT 0,
 ciphertext bytea, payload_hash bytea NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','sending','sent','failed','cancelled')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 8),
 next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL,
 lease_id text,lease_until timestamptz,last_failure text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),sent_at timestamptz,
 CHECK((status='sending')=(lease_id IS NOT NULL AND lease_until IS NOT NULL)),
 CHECK((status IN('pending','sending'))=(ciphertext IS NOT NULL))
);
CREATE INDEX account_mail_due ON account_mail_outbox(next_attempt_at,id) WHERE status='pending';
CREATE INDEX account_mail_lease ON account_mail_outbox(lease_until,id) WHERE status='sending';
CREATE INDEX account_mail_expiry ON account_mail_outbox(expires_at,id);
ALTER TABLE identity_transactions ADD COLUMN onboarding_token_hash bytea;
ALTER TABLE identity_transactions ADD COLUMN onboarding_token_cipher bytea;
ALTER TABLE identity_transactions ADD COLUMN operation_id text;
ALTER TABLE identity_transactions ADD COLUMN start_request_hash bytea;
ALTER TABLE identity_transactions ADD COLUMN authorization_cipher bytea;
ALTER TABLE identity_transactions ADD COLUMN native_nonce_cipher bytea;
CREATE UNIQUE INDEX identity_start_request ON identity_transactions(binding_hash,operation_id) WHERE operation_id IS NOT NULL;
ALTER TABLE identity_transactions ADD COLUMN intent_target text;
ALTER TABLE identity_transactions ADD COLUMN security_intent_cipher bytea;
ALTER TABLE identity_transactions ADD COLUMN mfa_challenge_id text;
ALTER TABLE identity_transactions ADD COLUMN mfa_challenge_cipher bytea;
ALTER TABLE identity_transactions ADD COLUMN native_apple boolean NOT NULL DEFAULT false;
ALTER TABLE identity_verified_assertions ADD COLUMN suggested_name text;
ALTER TABLE identity_contact_challenges ADD COLUMN verified_source text;
ALTER TABLE identity_contact_challenges ADD COLUMN request_id text;
CREATE UNIQUE INDEX identity_contact_request ON identity_contact_challenges(transaction_id,request_id);
ALTER TABLE account_erasure_jobs ADD COLUMN claim_operation_id text;
ALTER TABLE account_erasure_jobs ADD COLUMN claim_generation bigint;
ALTER TABLE account_erasure_jobs ADD COLUMN credential_generation bigint;
-- Null existing jobs are deliberately NOT reassigned to a successor claim.
CREATE INDEX account_erasure_authority ON account_erasure_jobs(server_id,claim_operation_id,claim_generation,credential_generation) WHERE acknowledged_at IS NULL;
ALTER TABLE account_deletions ADD COLUMN expected_family_id text;
ALTER TABLE account_deletions ADD COLUMN expected_account_epoch bigint;
ALTER TABLE account_deletions ADD COLUMN expected_family_generation bigint;
ALTER TABLE account_deletions ADD COLUMN expected_access_hash bytea;
ALTER TABLE account_deletions ADD COLUMN expected_session_mode text;
-- Expand only the identity protocol's existing CHECKs. No neighboring tables,
-- ownership locks or schema-version numbers change.
DO $$ DECLARE c record; BEGIN
 FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='identity_transactions'::regclass AND contype='c' AND (pg_get_constraintdef(oid) LIKE '%mode%' OR pg_get_constraintdef(oid) LIKE '%purpose%' OR pg_get_constraintdef(oid) LIKE '%status%') LOOP
  EXECUTE format('ALTER TABLE identity_transactions DROP CONSTRAINT %I',c.conname);
 END LOOP;
 FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='account_recent_proofs'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%purpose%' LOOP
  EXECUTE format('ALTER TABLE account_recent_proofs DROP CONSTRAINT %I',c.conname);
 END LOOP;
END $$;
ALTER TABLE identity_transactions ADD CONSTRAINT identity_security_mode CHECK(mode IN('login','reauthenticate','link'));
ALTER TABLE identity_transactions ADD CONSTRAINT identity_security_session_mode CHECK(session_mode IN('native','browser'));
ALTER TABLE identity_transactions ADD CONSTRAINT identity_security_purpose CHECK(purpose IN('password_set_or_change','account_deletion_preview','email_change','provider_link','provider_unlink','mfa_enroll','mfa_disable','recovery_codes','server_transfer'));
ALTER TABLE identity_transactions ADD CONSTRAINT identity_security_status CHECK(status IN('started','exchanging','verified','onboarding','contact_pending','completed','proof_issued','cancelled','failed'));
ALTER TABLE identity_transactions ADD CONSTRAINT identity_security_authority CHECK(
 (mode='login' AND purpose IS NULL AND intent_digest IS NULL AND expected_account_id IS NULL AND expected_family_id IS NULL AND expected_account_epoch IS NULL AND expected_family_generation IS NULL AND expected_access_hash IS NULL AND expected_identity_id IS NULL AND expected_identity_revision IS NULL) OR
 (mode IN('reauthenticate','link') AND purpose IS NOT NULL AND intent_digest IS NOT NULL AND expected_account_id IS NOT NULL AND expected_family_id IS NOT NULL AND expected_account_epoch IS NOT NULL AND expected_family_generation IS NOT NULL AND expected_access_hash IS NOT NULL AND ((mode='reauthenticate' AND expected_identity_id IS NOT NULL AND expected_identity_revision IS NOT NULL) OR (mode='link' AND purpose='provider_link' AND expected_identity_id IS NULL AND expected_identity_revision IS NULL))));
ALTER TABLE identity_transactions ADD CONSTRAINT identity_security_onboarding CHECK(status NOT IN('onboarding','contact_pending','completed') OR mode='login');
ALTER TABLE identity_transactions ADD CONSTRAINT identity_security_proof CHECK(status<>'proof_issued' OR mode IN('reauthenticate','link'));
ALTER TABLE identity_transactions ADD CONSTRAINT identity_security_terminal CHECK((status IN('completed','proof_issued','cancelled','failed'))=(terminal_at IS NOT NULL));
ALTER TABLE account_recent_proofs ADD CONSTRAINT account_security_proof_purpose CHECK(purpose IN('password_set_or_change','account_deletion_preview','email_change','provider_link','provider_unlink','mfa_enroll','mfa_disable','recovery_codes','server_transfer'));
INSERT INTO account_security_schema(singleton,version) VALUES(true,1);
`

// Existing receipts have no trustworthy authority snapshot. Leave their new
// fields NULL (fail closed) rather than blessing a possibly revoked receipt.
// Receipts expire after five minutes; no credential or account migration occurs.
func installReceiptAuthorityTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
 ALTER TABLE account_security_receipts ADD COLUMN IF NOT EXISTS account_epoch bigint;
 ALTER TABLE account_security_receipts ADD COLUMN IF NOT EXISTS family_generation bigint;
 ALTER TABLE account_security_receipts ADD COLUMN IF NOT EXISTS result_epoch bigint;
 ALTER TABLE account_security_receipts ADD COLUMN IF NOT EXISTS terminal_reason text NOT NULL DEFAULT '';
 ALTER TABLE account_security_receipts ADD COLUMN IF NOT EXISTS terminal_revoked_at timestamptz;
 ALTER TABLE account_security_receipts ADD COLUMN IF NOT EXISTS mfa_revision bigint;
 `)
	return err
}
