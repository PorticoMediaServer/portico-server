package postgres

import (
	"context"
	"database/sql"
	"errors"
)

// InstallClaimSchemaTx is a fresh-current-schema proposal. Root installs it in
// the single initialization transaction only with the current servers contract.
// It does not upgrade, adopt or alter any existing schema.
func InstallClaimSchemaTx(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return errors.New("claim schema requires transaction")
	}
	_, err := tx.ExecContext(ctx, claimSchema)
	return err
}

const claimSchema = `
CREATE TABLE claim_operations(
 id text PRIMARY KEY CHECK(id ~ '^[A-Za-z0-9_-]{1,128}$'),
 server_id text NOT NULL CHECK(server_id ~ '^[A-Za-z0-9_-]{1,128}$'),
 account_id text REFERENCES accounts(id) ON DELETE SET NULL,
 public_key bytea NOT NULL CHECK(octet_length(public_key)=32),
 local_generation bigint NOT NULL CHECK(local_generation>=0),
 server_name text NOT NULL CHECK(octet_length(server_name) BETWEEN 1 AND 120),
 owner_profile_id text REFERENCES profiles(id) ON DELETE SET NULL,
 intent_digest bytea NOT NULL CHECK(octet_length(intent_digest)=32),
 state text NOT NULL CHECK(state IN('approved','committed_pending_install','installed','cancelled')),
 claim_generation bigint CHECK(claim_generation>0),
 credential_generation bigint CHECK(credential_generation>0),
 committed_token_hash bytea CHECK(octet_length(committed_token_hash)=32),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 installed_at timestamptz, cancelled_at timestamptz,
 CHECK(state='cancelled' OR (account_id IS NOT NULL AND owner_profile_id IS NOT NULL)),
 CHECK(state<>'approved' OR claim_generation IS NULL),
 CHECK((claim_generation IS NULL)=(credential_generation IS NULL)),
 CHECK((claim_generation IS NULL)=(committed_token_hash IS NULL)),
 CHECK(state NOT IN('committed_pending_install','installed') OR claim_generation IS NOT NULL),
 CHECK((state='installed')=(installed_at IS NOT NULL AND cancelled_at IS NULL) OR state='cancelled'),
 CHECK((state='cancelled')=(cancelled_at IS NOT NULL)),
 UNIQUE(id,account_id)
);
CREATE UNIQUE INDEX claim_operation_committed_server ON claim_operations(server_id) WHERE state IN('committed_pending_install','installed');
CREATE INDEX claim_operation_account ON claim_operations(account_id,created_at,id);
CREATE TABLE claim_approvals(
 operation_id text PRIMARY KEY,
 account_id text NOT NULL,
 family_id text NOT NULL,
 access_hash bytea NOT NULL CHECK(octet_length(access_hash)=32),
 account_epoch bigint NOT NULL CHECK(account_epoch>=0),
 family_generation bigint NOT NULL CHECK(family_generation>=0),
 session_mode text NOT NULL CHECK(session_mode IN('native','browser')),
 revision bigint NOT NULL CHECK(revision>0),
 issued_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 FOREIGN KEY(operation_id,account_id) REFERENCES claim_operations(id,account_id) ON DELETE RESTRICT,
 FOREIGN KEY(family_id,account_id) REFERENCES session_families(id,account_id) ON DELETE RESTRICT,
 CHECK(expires_at>issued_at AND expires_at<=issued_at+interval '5 minutes')
);
CREATE TABLE claim_nonces(
 id text PRIMARY KEY CHECK(id ~ '^[A-Za-z0-9_-]{1,128}$'),
 operation_id text NOT NULL REFERENCES claim_operations(id) ON DELETE RESTRICT,
 approval_revision bigint NOT NULL CHECK(approval_revision>0),
 purpose text NOT NULL CHECK(purpose IN('claim-finalize','claim-result')),
 request_id text NOT NULL CHECK(request_id ~ '^[A-Za-z0-9_-]{1,128}$'),
 request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),
 nonce bytea NOT NULL CHECK(octet_length(nonce)=32),
 issued_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz,
 CHECK(expires_at>issued_at AND expires_at<=issued_at+interval '30 seconds'),
 CHECK(consumed_at IS NULL OR (consumed_at>=issued_at AND consumed_at<expires_at)),
 UNIQUE(operation_id,purpose,request_id)
);
CREATE INDEX claim_nonce_operation ON claim_nonces(operation_id,issued_at,id);
CREATE TABLE claim_cancellations(
 operation_id text PRIMARY KEY REFERENCES claim_operations(id) ON DELETE RESTRICT,
 request_id text NOT NULL CHECK(request_id ~ '^[A-Za-z0-9_-]{1,128}$'),
 request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),
 cancelled_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
`
