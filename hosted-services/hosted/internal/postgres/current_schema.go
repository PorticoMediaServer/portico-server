package postgres

import (
	"context"
	"database/sql"
	"errors"
)

// Numbered migrations start at 0001 for the pre-release reset. No schema DDL
// executes on a restart with an up-to-date migration history. Future migrations
// must append a version and run once under Store.Migrate's advisory lock.
// CurrentHostedSchemaVersion is the exact format this build provisions and
// accepts. Tests assert against it so a bump cannot leave a stale expectation.
const CurrentHostedSchemaVersion = 19

func InitializeCurrentHostedSchemaTx(ctx context.Context, tx *sql.Tx) error {
	var marker bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relname='hosted_schema_contract' AND c.relkind='r')`).Scan(&marker); err != nil {
		return err
	}
	if marker {
		var version int
		var kind string
		if err := tx.QueryRowContext(ctx, `SELECT kind,version FROM hosted_schema_contract WHERE singleton`).Scan(&kind, &version); err != nil {
			return err
		}
		if kind != "portico-hosted" || version < 1 || version > CurrentHostedSchemaVersion {
			return errors.New("unsupported Hosted schema; provision the current format")
		}
		// A pre-release schema without numbered history is deliberately rejected.
		// Reset it explicitly; never re-run historical DDL during a restart.
		var history bool
		if err := tx.QueryRowContext(ctx, `SELECT to_regclass(current_schema()||'.hosted_schema_migrations') IS NOT NULL`).Scan(&history); err != nil {
			return err
		}
		if !history {
			if version != 2 {
				return errors.New("Hosted migration history missing; reset pre-release schema")
			}
			// The pre-migration production format: bridge it to 0001, then migrate.
			if _, err := tx.ExecContext(ctx, legacyV2Bridge); err != nil {
				return err
			}
			return runHostedMigrations(ctx, tx, 1)
		}
		var count, latest int
		if err := tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(max(version),0) FROM hosted_schema_migrations`).Scan(&count, &latest); err != nil {
			return err
		}
		if count != version || latest != version {
			return errors.New("unsupported Hosted migration history")
		}
		var apps bool
		if err := tx.QueryRowContext(ctx, `SELECT to_regclass(current_schema()||'.app_authorizations') IS NOT NULL`).Scan(&apps); err != nil {
			return err
		}
		if !apps {
			return errors.New("Hosted sign-in schema missing; reset pre-release schema")
		}
		return runHostedMigrations(ctx, tx, version)
	}
	var occupied bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() ) OR EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema()) OR EXISTS(SELECT 1 FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace WHERE n.nspname=current_schema())`).Scan(&occupied); err != nil {
		return err
	}
	if occupied {
		return errors.New("unrecognized Hosted schema; provision the current format")
	}
	if err := install0001Tx(ctx, tx); err != nil {
		return err
	}
	return runHostedMigrations(ctx, tx, 1)
}

// install0001Tx installs migration 0001 on an empty schema and records it.
func install0001Tx(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, schema+deletionSchema+sessionReviewSchema+appAuthorizationSchema); err != nil {
		return err
	}
	if err := InstallIdentitySchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := InstallClaimSchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := InstallEndpointSchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := InstallDeviceSetupSchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := InstallRouteSchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := InstallCertificateSchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := InstallAccountSecuritySchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := InstallProfilesMembershipSchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := InstallNotifySchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := InstallAccountIdentitySchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := InstallAppleNotificationSchemaTx(ctx, tx); err != nil {
		return err
	}
	// The marker cannot survive partial initialization because this transaction
	// creates it only after the complete current schema and disabled policy.
	if _, err := tx.ExecContext(ctx, migration0001CompletionProof); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `CREATE TABLE hosted_schema_contract(singleton boolean PRIMARY KEY CHECK(singleton),kind text NOT NULL CHECK(kind='portico-hosted'),version integer NOT NULL CHECK(version>0)); CREATE TABLE hosted_schema_migrations(version integer PRIMARY KEY CHECK(version>0),name text NOT NULL,applied_at timestamptz NOT NULL DEFAULT clock_timestamp()); INSERT INTO hosted_schema_migrations(version,name) VALUES(1,'0001_initial'); INSERT INTO hosted_schema_contract(singleton,kind,version) VALUES(true,'portico-hosted',1);`)
	return err
}

func runHostedMigrations(ctx context.Context, tx *sql.Tx, current int) error {
	if current < 2 {
		if _, err := tx.ExecContext(ctx, migration0002); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(2,'0002_hosted_remediation'); UPDATE hosted_schema_contract SET version=2 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 3 {
		if _, err := tx.ExecContext(ctx, migration0003); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(3,'0003_device_push'); UPDATE hosted_schema_contract SET version=3 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 4 {
		if _, err := tx.ExecContext(ctx, migration0004); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(4,'0004_retirement_erasure'); UPDATE hosted_schema_contract SET version=4 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 5 {
		if _, err := tx.ExecContext(ctx, migration0005); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(5,'0005_network_password_backoff'); UPDATE hosted_schema_contract SET version=5 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 6 {
		if _, err := tx.ExecContext(ctx, migration0006); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(6,'0006_queue_notifications'); UPDATE hosted_schema_contract SET version=6 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 7 {
		if _, err := tx.ExecContext(ctx, migration0007); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(7,'0007_cgnat_in_memory'); UPDATE hosted_schema_contract SET version=7 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 8 {
		if _, err := tx.ExecContext(ctx, migration0008); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(8,'0008_signing_key_alert'); UPDATE hosted_schema_contract SET version=8 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 9 {
		if _, err := tx.ExecContext(ctx, migration0009); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(9,'0009_password_cool_off'); UPDATE hosted_schema_contract SET version=9 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 10 {
		if _, err := tx.ExecContext(ctx, migration0010); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(10,'0010_in_app_notifications'); UPDATE hosted_schema_contract SET version=10 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 11 {
		if _, err := tx.ExecContext(ctx, migration0011); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(11,'0011_unbound_web_claims'); UPDATE hosted_schema_contract SET version=11 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 12 {
		if _, err := tx.ExecContext(ctx, migration0012); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(12,'0012_member_label_rotation'); UPDATE hosted_schema_contract SET version=12 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 13 {
		if _, err := tx.ExecContext(ctx, migration0013); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(13,'0013_server_presence'); UPDATE hosted_schema_contract SET version=13 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 14 {
		if _, err := tx.ExecContext(ctx, migration0014); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(14,'0014_member_label_grace'); UPDATE hosted_schema_contract SET version=14 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 15 {
		if _, err := tx.ExecContext(ctx, migration0015); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(15,'0015_queued_label_rotation'); UPDATE hosted_schema_contract SET version=15 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 16 {
		if _, err := tx.ExecContext(ctx, migration0016); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(16,'0016_native_completion_opt_in'); UPDATE hosted_schema_contract SET version=16 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 17 {
		if _, err := tx.ExecContext(ctx, migration0017); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(17,'0017_server_member_index'); UPDATE hosted_schema_contract SET version=17 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 18 {
		if _, err := tx.ExecContext(ctx, migration0018); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(18,'0018_drop_presence_seen'); UPDATE hosted_schema_contract SET version=18 WHERE singleton`); err != nil {
			return err
		}
	}
	if current < 19 {
		if _, err := tx.ExecContext(ctx, migration0019); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_schema_migrations(version,name) VALUES(19,'0019_refresh_receipt_input'); UPDATE hosted_schema_contract SET version=19 WHERE singleton`); err != nil {
			return err
		}
	}
	return nil
}

// The completion proof is returned only on the browser that completed OAuth.
// It is never exposed by polling, nor recoverable from the initiating binding.
const migration0001CompletionProof = `
CREATE TABLE identity_native_completions (
 transaction_id text PRIMARY KEY REFERENCES identity_transactions(id) ON DELETE CASCADE,
 code_hash bytea NOT NULL CHECK(octet_length(code_hash)=32),
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz
);
`

const retentionIndexes = `
CREATE INDEX audit_actor ON security_audit(actor_id,created_at);
CREATE INDEX audit_target ON security_audit(target_id,created_at);
CREATE INDEX audit_expiry ON security_audit(created_at);
CREATE INDEX address_history_expiry ON direct_address_events(observed_at);
CREATE INDEX refresh_consumed ON refresh_tokens(consumed_at) WHERE consumed_at IS NOT NULL;
CREATE INDEX refresh_receipt_expiry ON refresh_receipts(expires_at);
CREATE INDEX family_expiry ON session_families((LEAST(COALESCE(revoked_at,'infinity'),idle_expires_at,absolute_expires_at)));
CREATE INDEX claim_nonce_expiry ON claim_nonces(expires_at);
CREATE INDEX certificate_namespace_history ON certificate_orders(namespace,created_at DESC,id DESC) WHERE state='issued';
CREATE INDEX appstore_payload_expiry ON appstore_notifications(received_at) WHERE payload<>'{}'::jsonb;
CREATE INDEX notification_processed ON account_notification_events(processed_at) WHERE processed_at IS NOT NULL;
CREATE INDEX server_offline_detection ON servers(presence_expires_at) WHERE revoked_at IS NULL;
CREATE INDEX device_installation ON hosted_devices(account_id,installation_id) WHERE revoked_at IS NULL;
`
