package postgres

import (
	"context"
	"database/sql"
)

// Named, additive authority metadata. Generic and TV requests still reserve codes
// in the one device_authorizations uniqueness domain; no parallel token issuer.
func InstallDeviceSetupSchemaTx(ctx context.Context, tx *sql.Tx) error {
	_, e := tx.ExecContext(ctx, `ALTER TABLE device_authorizations
 ADD COLUMN IF NOT EXISTS protocol text NOT NULL DEFAULT '',
 ADD COLUMN IF NOT EXISTS tv_public_key text NOT NULL DEFAULT '',
 ADD COLUMN IF NOT EXISTS client_request_id text NOT NULL DEFAULT '',
 ADD COLUMN IF NOT EXISTS tv_grant bytea;
 ALTER TABLE device_authorizations ALTER COLUMN user_hash DROP NOT NULL;
 CREATE INDEX IF NOT EXISTS device_setup_recovery ON device_authorizations(recovery_until) WHERE tv_grant IS NOT NULL;`)
	return e
}
