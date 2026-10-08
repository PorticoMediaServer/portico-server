package postgres

import (
	"context"
	"database/sql"
	"errors"
)

// InstallIdentitySchemaTx creates the current OIDC relations in a fresh schema.
// Root must use the current accounts declaration with nullable password_hash
// and all absent-password guards. There is no prior-format upgrade path here.
// It enables no routes or provider policy; the caller owns the transaction.
func InstallIdentitySchemaTx(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return errors.New("identity migration requires a transaction")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, identitySchema)
	return err
}

const identitySchema = `
CREATE TABLE identity_protocol_policy(
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 enabled boolean NOT NULL DEFAULT false,
 marker_key_id text CHECK(marker_key_id ~ '^[a-f0-9]{64}$'),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(NOT enabled OR marker_key_id IS NOT NULL)
);
INSERT INTO identity_protocol_policy(singleton) VALUES(true);
CREATE FUNCTION identity_policy_revision_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'identity policy cannot be deleted'; END IF;
 IF NEW.revision<>OLD.revision+1 OR NEW.singleton<>OLD.singleton THEN RAISE EXCEPTION 'identity policy revision must advance'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER identity_policy_revision BEFORE UPDATE OR DELETE ON identity_protocol_policy FOR EACH ROW EXECUTE FUNCTION identity_policy_revision_guard();

CREATE TABLE identity_provider_descriptors(
 id text PRIMARY KEY CHECK(octet_length(id) BETWEEN 1 AND 128),
 provider text NOT NULL CHECK(provider IN('google','apple')),
 revision bigint NOT NULL CHECK(revision>0), enabled boolean NOT NULL DEFAULT false,
 issuer text NOT NULL CHECK(octet_length(issuer) BETWEEN 1 AND 256),
 audience text NOT NULL CHECK(octet_length(audience) BETWEEN 1 AND 256),
 callback_uri text NOT NULL CHECK(octet_length(callback_uri) BETWEEN 9 AND 2048 AND callback_uri LIKE 'https://%'),
 CHECK((provider='google' AND issuer='https://accounts.google.com') OR (provider='apple' AND issuer='https://appleid.apple.com')),
 UNIQUE(id,provider)
);
CREATE UNIQUE INDEX identity_provider_one_enabled ON identity_provider_descriptors(provider) WHERE enabled;

CREATE TABLE account_external_identities(
 id text PRIMARY KEY CHECK(octet_length(id) BETWEEN 1 AND 128),
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
 provider text NOT NULL CHECK(provider IN('google','apple')),
 issuer text NOT NULL CHECK(octet_length(issuer) BETWEEN 1 AND 256),
 subject text NOT NULL CHECK(octet_length(subject) BETWEEN 1 AND 255),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), verified_at timestamptz NOT NULL,
 revoked_at timestamptz,
 CHECK((provider='google' AND issuer='https://accounts.google.com') OR (provider='apple' AND issuer='https://appleid.apple.com')),
 UNIQUE(issuer,subject), UNIQUE(id,account_id)
);
CREATE INDEX account_external_identity_account ON account_external_identities(account_id,id);
CREATE FUNCTION identity_link_revision_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.id<>OLD.id OR NEW.account_id<>OLD.account_id OR NEW.provider<>OLD.provider OR NEW.issuer<>OLD.issuer OR NEW.subject<>OLD.subject OR NEW.created_at<>OLD.created_at OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'identity link authority is immutable and revision must advance';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER identity_link_revision BEFORE UPDATE ON account_external_identities FOR EACH ROW EXECUTE FUNCTION identity_link_revision_guard();

CREATE TABLE account_contact_emails(
 id text PRIMARY KEY CHECK(octet_length(id) BETWEEN 1 AND 128),
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 address text NOT NULL CHECK(octet_length(address) BETWEEN 3 AND 320),
 verified_at timestamptz NOT NULL,
 source text NOT NULL CHECK(source IN('provider','challenge')),
 source_identity_id text,
 private_relay boolean NOT NULL DEFAULT false, is_primary boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(source_identity_id,account_id) REFERENCES account_external_identities(id,account_id) ON DELETE CASCADE,
 CHECK((source='provider' AND source_identity_id IS NOT NULL) OR (source='challenge' AND source_identity_id IS NULL)),
 UNIQUE(account_id,address)
);
CREATE UNIQUE INDEX account_contact_one_primary ON account_contact_emails(account_id) WHERE is_primary;
CREATE TABLE deleted_identity_markers(
 marker_key_id text NOT NULL CHECK(marker_key_id ~ '^[a-f0-9]{64}$'),
 subject_digest bytea NOT NULL CHECK(octet_length(subject_digest)=32),
 deleted_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
 PRIMARY KEY(marker_key_id,subject_digest),
 CHECK(expires_at>deleted_at AND expires_at<=deleted_at+interval '15 minutes')
);
CREATE INDEX deleted_identity_marker_expiry ON deleted_identity_markers(expires_at,marker_key_id,subject_digest);
CREATE TABLE family_cookie_names(
 suffix text PRIMARY KEY CHECK(suffix ~ '^[a-f0-9]{64}$'),
 family_id text NOT NULL UNIQUE REFERENCES session_families(id) ON DELETE CASCADE
);

CREATE TABLE identity_browser_bindings(
 binding_hash bytea PRIMARY KEY CHECK(octet_length(binding_hash)=32),
 generation bigint NOT NULL DEFAULT 0 CHECK(generation>=0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '24 hours')
);
CREATE INDEX identity_binding_expiry ON identity_browser_bindings(expires_at,binding_hash);
CREATE TABLE identity_transactions(
 id text PRIMARY KEY CHECK(octet_length(id) BETWEEN 1 AND 128),
 binding_hash bytea NOT NULL REFERENCES identity_browser_bindings(binding_hash) ON DELETE CASCADE,
 binding_generation bigint NOT NULL CHECK(binding_generation>=0),
 state_hash bytea NOT NULL UNIQUE CHECK(octet_length(state_hash)=32),
 nonce_hash bytea NOT NULL CHECK(octet_length(nonce_hash)=32),
 descriptor_id text NOT NULL, provider text NOT NULL,
 descriptor_revision bigint NOT NULL CHECK(descriptor_revision>0),
 policy_revision bigint NOT NULL CHECK(policy_revision>0),
 marker_key_id text NOT NULL CHECK(marker_key_id ~ '^[a-f0-9]{64}$'),
 mode text NOT NULL CHECK(mode IN('login','reauthenticate')),
 session_mode text NOT NULL CHECK(session_mode IN('native','browser')),
 purpose text CHECK(purpose IN('password_set_or_change','account_deletion_preview')),
 intent_digest bytea CHECK(octet_length(intent_digest)=32),
 expected_account_id text REFERENCES accounts(id) ON DELETE CASCADE,
 expected_family_id text,
 expected_account_epoch bigint CHECK(expected_account_epoch>=0),
 expected_family_generation bigint CHECK(expected_family_generation>=0),
 expected_access_hash bytea CHECK(octet_length(expected_access_hash)=32),
 expected_identity_id text, expected_identity_revision bigint CHECK(expected_identity_revision>0),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 status text NOT NULL DEFAULT 'started' CHECK(status IN('started','exchanging','verified','onboarding','contact_pending','completed','proof_issued','cancelled','failed')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 terminal_at timestamptz,
 FOREIGN KEY(descriptor_id,provider) REFERENCES identity_provider_descriptors(id,provider) ON DELETE RESTRICT,
 FOREIGN KEY(expected_family_id,expected_account_id) REFERENCES session_families(id,account_id) ON DELETE CASCADE,
 FOREIGN KEY(expected_identity_id,expected_account_id) REFERENCES account_external_identities(id,account_id) ON DELETE CASCADE,
 CHECK((mode='login' AND purpose IS NULL AND intent_digest IS NULL AND expected_account_id IS NULL AND expected_family_id IS NULL AND expected_account_epoch IS NULL AND expected_family_generation IS NULL AND expected_access_hash IS NULL AND expected_identity_id IS NULL AND expected_identity_revision IS NULL) OR
       (mode='reauthenticate' AND purpose IS NOT NULL AND intent_digest IS NOT NULL AND expected_account_id IS NOT NULL AND expected_family_id IS NOT NULL AND expected_account_epoch IS NOT NULL AND expected_family_generation IS NOT NULL AND expected_access_hash IS NOT NULL AND expected_identity_id IS NOT NULL AND expected_identity_revision IS NOT NULL)),
 CHECK(status NOT IN('onboarding','contact_pending','completed') OR mode='login'),
 CHECK(status<>'proof_issued' OR mode='reauthenticate'),
 CHECK((status IN('completed','proof_issued','cancelled','failed'))=(terminal_at IS NOT NULL)),
 CHECK(terminal_at IS NULL OR (terminal_at>=created_at AND terminal_at<=expires_at)),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '10 minutes'),
 UNIQUE(id,expires_at)
);
CREATE INDEX identity_transaction_binding_live ON identity_transactions(binding_hash,created_at,id) WHERE terminal_at IS NULL;
CREATE INDEX identity_transaction_account ON identity_transactions(expected_account_id,id) WHERE expected_account_id IS NOT NULL;
CREATE INDEX identity_transaction_expiry ON identity_transactions(expires_at,id);
CREATE TABLE identity_exchange_claims(
 transaction_id text PRIMARY KEY, transaction_expires_at timestamptz NOT NULL,
 lease_id text NOT NULL UNIQUE CHECK(octet_length(lease_id) BETWEEN 1 AND 128),
 transaction_revision bigint NOT NULL CHECK(transaction_revision>0), binding_generation bigint NOT NULL CHECK(binding_generation>=0),
 acquired_at timestamptz NOT NULL DEFAULT clock_timestamp(), lease_until timestamptz NOT NULL,
 FOREIGN KEY(transaction_id,transaction_expires_at) REFERENCES identity_transactions(id,expires_at) ON DELETE CASCADE,
 CHECK(lease_until>acquired_at AND lease_until<=acquired_at+interval '30 seconds' AND lease_until<=transaction_expires_at)
);
CREATE INDEX identity_exchange_lease_expiry ON identity_exchange_claims(lease_until,transaction_id);
CREATE TABLE identity_exchange_secrets(
 transaction_id text PRIMARY KEY, transaction_expires_at timestamptz NOT NULL,
 sealed_verifier bytea CHECK(octet_length(sealed_verifier) BETWEEN 28 AND 1024),
 sealed_code bytea CHECK(octet_length(sealed_code) BETWEEN 28 AND 16384),
 expires_at timestamptz NOT NULL,
 FOREIGN KEY(transaction_id,transaction_expires_at) REFERENCES identity_transactions(id,expires_at) ON DELETE CASCADE,
 CHECK(expires_at<=transaction_expires_at), CHECK(sealed_verifier IS NOT NULL OR sealed_code IS NOT NULL)
);
CREATE TABLE identity_verified_assertions(
 transaction_id text PRIMARY KEY REFERENCES identity_transactions(id) ON DELETE CASCADE,
 issuer text NOT NULL CHECK(octet_length(issuer) BETWEEN 1 AND 256), subject text NOT NULL CHECK(octet_length(subject) BETWEEN 1 AND 255),
 audience text NOT NULL CHECK(octet_length(audience) BETWEEN 1 AND 256),
 provider_issued_at timestamptz NOT NULL, verified_at timestamptz NOT NULL,
 nonce_hash bytea NOT NULL CHECK(octet_length(nonce_hash)=32),
 provider_email text CHECK(octet_length(provider_email) BETWEEN 3 AND 320), email_verified boolean NOT NULL DEFAULT false,
 private_relay boolean NOT NULL DEFAULT false,
 transaction_revision bigint NOT NULL CHECK(transaction_revision>0),
 lease_id text NOT NULL CHECK(octet_length(lease_id) BETWEEN 1 AND 128),
 CHECK(NOT email_verified OR provider_email IS NOT NULL), CHECK(NOT private_relay OR provider_email IS NOT NULL)
);
CREATE INDEX identity_assertion_subject ON identity_verified_assertions(issuer,subject,transaction_id);
CREATE TABLE identity_completion_receipts(
 transaction_id text PRIMARY KEY, transaction_expires_at timestamptz NOT NULL,
 binding_generation bigint NOT NULL CHECK(binding_generation>=0),
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 family_id text NOT NULL,
 FOREIGN KEY(family_id,account_id) REFERENCES session_families(id,account_id) ON DELETE CASCADE,
 account_epoch bigint NOT NULL CHECK(account_epoch>=0), family_generation bigint NOT NULL CHECK(family_generation>=0),
 ciphertext bytea NOT NULL CHECK(octet_length(ciphertext) BETWEEN 28 AND 32768),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 FOREIGN KEY(transaction_id,transaction_expires_at) REFERENCES identity_transactions(id,expires_at) ON DELETE CASCADE,
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '5 minutes' AND expires_at<=transaction_expires_at)
);
CREATE INDEX identity_receipt_expiry ON identity_completion_receipts(expires_at,transaction_id);
CREATE INDEX identity_receipt_account ON identity_completion_receipts(account_id,transaction_id);

CREATE TABLE identity_onboarding(
 transaction_id text PRIMARY KEY REFERENCES identity_transactions(id) ON DELETE CASCADE,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 username text CHECK(username ~ '^[a-z0-9][a-z0-9._-]{1,30}[a-z0-9]$'),
 display_name text CHECK(octet_length(display_name) BETWEEN 1 AND 120),
 CHECK((username IS NULL)=(display_name IS NULL))
);
CREATE TABLE identity_contact_challenges(
 id text PRIMARY KEY CHECK(octet_length(id) BETWEEN 1 AND 128),
 transaction_id text NOT NULL, transaction_expires_at timestamptz NOT NULL,
 address text NOT NULL CHECK(octet_length(address) BETWEEN 3 AND 320),
 challenge_hash bytea NOT NULL UNIQUE CHECK(octet_length(challenge_hash)=32),
 revision bigint NOT NULL CHECK(revision>0), is_current boolean NOT NULL DEFAULT true,
 issued_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 verified_at timestamptz, attempt_count integer NOT NULL DEFAULT 0 CHECK(attempt_count BETWEEN 0 AND 5),
 FOREIGN KEY(transaction_id,transaction_expires_at) REFERENCES identity_transactions(id,expires_at) ON DELETE CASCADE,
 CHECK(expires_at>issued_at AND expires_at<=issued_at+interval '10 minutes' AND expires_at<=transaction_expires_at),
 CHECK(verified_at IS NULL OR (verified_at>=issued_at AND verified_at<expires_at)),
 UNIQUE(id,revision,expires_at)
);
CREATE UNIQUE INDEX identity_contact_current_challenge ON identity_contact_challenges(transaction_id) WHERE is_current;
CREATE INDEX identity_contact_challenge_expiry ON identity_contact_challenges(expires_at,id);
CREATE TABLE identity_contact_outbox(
 id text PRIMARY KEY CHECK(octet_length(id) BETWEEN 1 AND 128),
 challenge_id text NOT NULL, challenge_revision bigint NOT NULL, challenge_expires_at timestamptz NOT NULL,
 ciphertext bytea CHECK(octet_length(ciphertext) BETWEEN 28 AND 4096),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','sending','sent','failed','cancelled')),
 lease_id text CHECK(octet_length(lease_id) BETWEEN 1 AND 128), lease_until timestamptz, acquired_at timestamptz,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),
 next_attempt_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
 last_failure text CHECK(last_failure IN('unavailable','rejected','expired','superseded')),
 FOREIGN KEY(challenge_id,challenge_revision,challenge_expires_at) REFERENCES identity_contact_challenges(id,revision,expires_at) ON DELETE CASCADE,
 CHECK(expires_at<=challenge_expires_at),
 CHECK((status='sending' AND lease_id IS NOT NULL AND lease_until IS NOT NULL AND acquired_at IS NOT NULL AND lease_until>acquired_at AND lease_until<=acquired_at+interval '30 seconds' AND lease_until<=expires_at) OR (status<>'sending' AND lease_id IS NULL AND lease_until IS NULL AND acquired_at IS NULL)),
 CHECK((status IN('pending','sending') AND ciphertext IS NOT NULL) OR (status IN('sent','failed','cancelled') AND ciphertext IS NULL))
);
CREATE INDEX identity_outbox_due ON identity_contact_outbox(next_attempt_at,id) WHERE status='pending';
CREATE INDEX identity_outbox_lease ON identity_contact_outbox(lease_until,id) WHERE status='sending';
CREATE INDEX identity_outbox_expiry ON identity_contact_outbox(expires_at,id);
CREATE TABLE account_recent_proofs(
 proof_hash bytea PRIMARY KEY CHECK(octet_length(proof_hash)=32),
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 family_id text NOT NULL,
 FOREIGN KEY(family_id,account_id) REFERENCES session_families(id,account_id) ON DELETE CASCADE,
 account_epoch bigint NOT NULL CHECK(account_epoch>=0), family_generation bigint NOT NULL CHECK(family_generation>=0),
 session_mode text NOT NULL CHECK(session_mode IN('native','browser')),
 captured_access_hash bytea NOT NULL CHECK(octet_length(captured_access_hash)=32),
 credential_kind text NOT NULL CHECK(credential_kind IN('password','provider')),
 identity_id text, identity_revision bigint CHECK(identity_revision>0),
 purpose text NOT NULL CHECK(purpose IN('password_set_or_change','account_deletion_preview')),
 intent_digest bytea NOT NULL CHECK(octet_length(intent_digest)=32),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL, consumed_at timestamptz,
 FOREIGN KEY(identity_id,account_id) REFERENCES account_external_identities(id,account_id) ON DELETE CASCADE,
 CHECK((credential_kind='password' AND identity_id IS NULL AND identity_revision IS NULL) OR (credential_kind='provider' AND identity_id IS NOT NULL AND identity_revision IS NOT NULL)),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '5 minutes'),
 CHECK(consumed_at IS NULL OR (consumed_at>=created_at AND consumed_at<expires_at))
);
CREATE INDEX account_recent_proof_live ON account_recent_proofs(account_id,created_at,proof_hash) WHERE consumed_at IS NULL;
CREATE INDEX account_recent_proof_expiry ON account_recent_proofs(expires_at,proof_hash);
`
