package postgres

import (
	"context"
	"database/sql"
)

// Named, additive P02 migration; does not claim a sibling's schema version.
func InstallCertificateSchemaTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
 CREATE TABLE IF NOT EXISTS certificate_namespaces (
   operation_id text PRIMARY KEY,
   member_label text NOT NULL DEFAULT '' CHECK(member_label='' OR member_label ~ '^[0-9a-f]{32}$'),
   namespace text NOT NULL UNIQUE CHECK(namespace ~ '^ptc-[a-z2-7]{20}$'),
   created_at timestamptz NOT NULL DEFAULT now()
 );
 CREATE TABLE IF NOT EXISTS certificate_order_admissions (
   id bigserial PRIMARY KEY, issuer text NOT NULL, order_id text NOT NULL,
   admitted_at timestamptz NOT NULL DEFAULT clock_timestamp()
 );
 CREATE INDEX IF NOT EXISTS certificate_order_admissions_window ON certificate_order_admissions(issuer,admitted_at DESC);
 CREATE SEQUENCE IF NOT EXISTS direct_address_sequence;
 CREATE TABLE IF NOT EXISTS direct_addresses (
   namespace text PRIMARY KEY REFERENCES certificate_namespaces(namespace) ON DELETE CASCADE,
   server_id text NOT NULL,
   ipv4 text NOT NULL DEFAULT '', ipv6 text NOT NULL DEFAULT '',lan_ipv4 text NOT NULL DEFAULT '',lan_ipv6 text NOT NULL DEFAULT '',
   sequence bigint NOT NULL UNIQUE,
   updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
 );
 CREATE TABLE IF NOT EXISTS direct_address_events (
   id bigserial PRIMARY KEY, server_id text NOT NULL, namespace text NOT NULL,
   address text NOT NULL, previous text NOT NULL DEFAULT '',
   observed_at timestamptz NOT NULL DEFAULT clock_timestamp()
 );
 CREATE INDEX IF NOT EXISTS direct_address_events_server ON direct_address_events(server_id,observed_at DESC);
 CREATE TABLE IF NOT EXISTS certificate_acme_accounts (
   issuer text PRIMARY KEY, directory_url text NOT NULL,
   key_cipher bytea NOT NULL, account_url text NOT NULL DEFAULT '',
   created_at timestamptz NOT NULL DEFAULT now()
 );
 CREATE TABLE IF NOT EXISTS certificate_orders (
   id text PRIMARY KEY, request_id text NOT NULL, operation_id text NOT NULL,
   server_id text NOT NULL, account_id text NOT NULL,
   claim_generation bigint NOT NULL, credential_generation bigint NOT NULL,
   token_hash bytea NOT NULL, public_key bytea NOT NULL,
   namespace text NOT NULL REFERENCES certificate_namespaces(namespace), issuer text NOT NULL,
   csr bytea NOT NULL CHECK(octet_length(csr) BETWEEN 100 AND 8192), csr_hash bytea NOT NULL,
   state text NOT NULL CHECK(state IN ('queued','creating','ordering','finalizing','issued','failed','reconciliation_required','cancelled','revoked')),
   provider_url text NOT NULL DEFAULT '', chain bytea, not_before timestamptz, not_after timestamptz,
   creation_attempted boolean NOT NULL DEFAULT false, cancel_requested boolean NOT NULL DEFAULT false, revocation_done boolean NOT NULL DEFAULT false,
   next_attempt_at timestamptz NOT NULL DEFAULT now(), attempts integer NOT NULL DEFAULT 0,
   lease_token text NOT NULL DEFAULT '', lease_until timestamptz,
   error_code text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
   UNIQUE(operation_id, request_id)
 );
 CREATE UNIQUE INDEX IF NOT EXISTS certificate_one_active ON certificate_orders(operation_id)
   WHERE state IN ('queued','creating','ordering','finalizing','reconciliation_required');
 CREATE UNIQUE INDEX IF NOT EXISTS certificate_provider_order ON certificate_orders(issuer,provider_url) WHERE provider_url<>'';
 CREATE INDEX IF NOT EXISTS certificate_due ON certificate_orders(issuer,next_attempt_at,created_at);
 CREATE INDEX IF NOT EXISTS certificate_claim_orders ON certificate_orders(operation_id,created_at DESC);
 CREATE INDEX IF NOT EXISTS certificate_server_orders ON certificate_orders(server_id,created_at DESC);
 CREATE INDEX IF NOT EXISTS certificate_account_orders ON certificate_orders(account_id,created_at DESC);
 CREATE TABLE IF NOT EXISTS certificate_challenges (
   order_id text NOT NULL REFERENCES certificate_orders(id), authorization_url text NOT NULL,
   challenge_url text NOT NULL, namespace text NOT NULL REFERENCES certificate_namespaces(namespace),
   txt_value text NOT NULL CHECK(length(txt_value)=43),
   cleanup_done boolean NOT NULL DEFAULT false, cleanup_after timestamptz NOT NULL DEFAULT now(),
   PRIMARY KEY(order_id,authorization_url), UNIQUE(namespace,txt_value)
 );
 `)
	if err != nil {
		return err
	}
	return installCertificateQueueTx(ctx, tx)
}

// This additive migration separates retained receipts from due work. Backfill is
// performed once under the caller's migration lock, never on worker idle ticks.
func installCertificateQueueTx(ctx context.Context, tx *sql.Tx) error {
	var installed bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='certificate_orders' AND column_name='cleanup_due_at')`).Scan(&installed); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
 ALTER TABLE certificate_orders ADD COLUMN IF NOT EXISTS cleanup_due_at timestamptz;
 ALTER TABLE certificate_orders ADD COLUMN IF NOT EXISTS authority_check_at timestamptz NOT NULL DEFAULT now();
 CREATE INDEX IF NOT EXISTS certificate_active_work ON certificate_orders(issuer,next_attempt_at,created_at,id)
 WHERE state IN ('queued','creating','ordering','finalizing') OR (state='failed' AND NOT revocation_done) OR (cancel_requested AND NOT revocation_done AND state<>'reconciliation_required');
 CREATE INDEX IF NOT EXISTS certificate_quarantine_work ON certificate_orders(issuer,next_attempt_at,created_at,id)
 WHERE state='reconciliation_required' AND NOT cancel_requested;
 CREATE INDEX IF NOT EXISTS certificate_cleanup_work ON certificate_orders(issuer,(GREATEST(next_attempt_at,cleanup_due_at)),created_at,id)
 WHERE cleanup_due_at IS NOT NULL AND state IN ('issued','failed','cancelled','revoked','reconciliation_required');
 CREATE INDEX IF NOT EXISTS certificate_authority_work ON certificate_orders(issuer,authority_check_at,id)
 WHERE state IN ('issued','reconciliation_required') AND NOT cancel_requested;
 CREATE INDEX IF NOT EXISTS certificate_pending_challenges ON certificate_challenges(order_id,cleanup_after) WHERE NOT cleanup_done;
 `); err != nil {
		return err
	}
	if !installed {
		_, err := tx.ExecContext(ctx, `UPDATE certificate_orders c SET cleanup_due_at=t.due FROM (SELECT order_id,min(cleanup_after) AS due FROM certificate_challenges WHERE NOT cleanup_done GROUP BY order_id) t WHERE c.id=t.order_id`)
		return err
	}
	return nil
}
