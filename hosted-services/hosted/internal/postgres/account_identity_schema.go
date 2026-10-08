package postgres

import (
	"context"
	"database/sql"
	"errors"
)

// InstallAccountIdentitySchemaTx adds the workstream-H account surface: the
// columns a device list needs on the existing hosted_devices rows, account-level
// region and privacy preferences, and the store for decoded App Store Server
// Notifications.
//
// It follows the account-security precedent: its own marker table and its own
// version, so the hosted format version is not renumbered. Every statement is
// idempotent, because it runs inside the initialization transaction on every
// start.
func InstallAccountIdentitySchemaTx(ctx context.Context, tx *sql.Tx) error {
	var installed bool
	if err := tx.QueryRowContext(ctx, `SELECT to_regclass(current_schema()||'.account_identity_schema') IS NOT NULL`).Scan(&installed); err != nil {
		return err
	}
	if installed {
		var version int
		if err := tx.QueryRowContext(ctx, `SELECT version FROM account_identity_schema WHERE singleton`).Scan(&version); err != nil {
			return err
		}
		if version != 1 {
			return errors.New("unsupported account-identity schema")
		}
		return nil
	}
	_, err := tx.ExecContext(ctx, accountIdentitySchema)
	return err
}

const accountIdentitySchema = `
CREATE TABLE account_identity_schema(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),version integer NOT NULL);

-- hosted_devices already records what a device said at approval. A device list
-- also needs what it is doing now, and whether the account holder trusts it.
ALTER TABLE hosted_devices ADD COLUMN IF NOT EXISTS app text NOT NULL DEFAULT '';
ALTER TABLE hosted_devices ADD COLUMN IF NOT EXISTS installation_id text NOT NULL DEFAULT '';
ALTER TABLE hosted_devices ADD COLUMN IF NOT EXISTS ip text NOT NULL DEFAULT '';
ALTER TABLE hosted_devices ADD COLUMN IF NOT EXISTS last_seen_at timestamptz NOT NULL DEFAULT clock_timestamp();
ALTER TABLE hosted_devices ADD COLUMN IF NOT EXISTS trusted boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS hosted_devices_recent ON hosted_devices(account_id,last_seen_at DESC) WHERE revoked_at IS NULL;

-- Account-level region and privacy. These are account facts, not per-server
-- viewer preferences: a person has one home time zone and one answer about
-- whether their activity may be shown to the other members of their household.
CREATE TABLE account_preferences(
 account_id text PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 locale text NOT NULL DEFAULT '' CHECK(length(locale)<=35),
 time_zone text NOT NULL DEFAULT '' CHECK(length(time_zone)<=64),
 hour_cycle text NOT NULL DEFAULT 'auto' CHECK(hour_cycle IN('auto','h11','h12','h23','h24')),
 pause_watch_history boolean NOT NULL DEFAULT false,
 show_activity_to_members boolean NOT NULL DEFAULT true,
 include_in_watch_together boolean NOT NULL DEFAULT true,
 marketing_email boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

-- Decoded App Store Server Notifications. The signed payload is verified before
-- anything is written, and the raw JWS is never stored: it carries the same
-- transaction facts as the decoded columns and adds a credential-shaped blob to
-- the backup surface for nothing.
CREATE TABLE appstore_notifications(
 uuid text PRIMARY KEY,
 notification_type text NOT NULL,
 subtype text NOT NULL DEFAULT '',
 bundle_id text NOT NULL DEFAULT '',
 environment text NOT NULL DEFAULT '' CHECK(environment IN('','Sandbox','Production')),
 original_transaction_id text NOT NULL DEFAULT '',
 app_account_token text NOT NULL DEFAULT '',
 account_id text REFERENCES accounts(id) ON DELETE SET NULL,
 signed_at timestamptz,
 received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 payload jsonb NOT NULL
);
CREATE INDEX appstore_notifications_transaction ON appstore_notifications(original_transaction_id,received_at DESC);
CREATE INDEX appstore_notifications_account ON appstore_notifications(account_id,received_at DESC) WHERE account_id IS NOT NULL;

-- A verification failure is recorded without its body so an operator can see
-- that something is arriving and being refused, and why, without the refused
-- bytes becoming durable state.
CREATE TABLE appstore_notification_failures(
 id bigserial PRIMARY KEY,
 reason text NOT NULL,
 observed_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX appstore_notification_failure_recent ON appstore_notification_failures(observed_at DESC);

INSERT INTO account_identity_schema(singleton,version) VALUES(true,1);
`
