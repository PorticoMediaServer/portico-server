package postgres

import (
	"context"
	"database/sql"
	"errors"
)

func InstallAppleNotificationSchemaTx(ctx context.Context, tx *sql.Tx) error {
	var installed bool
	if err := tx.QueryRowContext(ctx, `SELECT to_regclass(current_schema()||'.apple_notification_schema') IS NOT NULL`).Scan(&installed); err != nil {
		return err
	}
	if installed {
		var version int
		if err := tx.QueryRowContext(ctx, `SELECT version FROM apple_notification_schema WHERE singleton`).Scan(&version); err != nil {
			return err
		}
		if version != 1 {
			return errors.New("unsupported Apple notification schema")
		}
		return nil
	}
	_, err := tx.ExecContext(ctx, `
 CREATE TABLE apple_notification_schema(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton), version integer NOT NULL CHECK(version=1));
 CREATE TABLE apple_account_notifications(id_hash bytea PRIMARY KEY CHECK(octet_length(id_hash)=32), payload_hash bytea NOT NULL CHECK(octet_length(payload_hash)=32), event_type text NOT NULL, received_at timestamptz NOT NULL DEFAULT clock_timestamp());
 CREATE TABLE apple_identity_events(subject_hash bytea PRIMARY KEY CHECK(octet_length(subject_hash)=32), revoked_at timestamptz, deleted_at timestamptz, authorized_at timestamptz);
 CREATE TABLE apple_relay_events(subject_hash bytea NOT NULL CHECK(octet_length(subject_hash)=32), email_hash bytea NOT NULL CHECK(octet_length(email_hash)=32), event_at timestamptz NOT NULL, disabled boolean NOT NULL, PRIMARY KEY(subject_hash,email_hash));
 CREATE INDEX apple_relay_disabled ON apple_relay_events(email_hash) WHERE disabled;
 ALTER TABLE account_external_identities ADD COLUMN apple_revocation_at timestamptz;
 INSERT INTO apple_notification_schema(singleton,version) VALUES(true,1);
 `)
	return err
}
