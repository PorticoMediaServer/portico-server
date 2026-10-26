package postgres

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
)

const CurrentHostedSchemaVersion = 30

//go:embed schema.sql
var currentSchema string

// InitializeCurrentHostedSchemaTx installs exactly one current baseline in the
// caller's transaction. Pre-release schemas are reset explicitly, never adopted
// or migrated. The marker commits with every current constraint.
func InitializeCurrentHostedSchemaTx(ctx context.Context, tx *sql.Tx) error {
	var marker bool
	if e := tx.QueryRowContext(ctx, `SELECT to_regclass(current_schema()||'.hosted_schema_contract') IS NOT NULL`).Scan(&marker); e != nil {
		return e
	}
	if marker {
		var kind string
		var version int
		if e := tx.QueryRowContext(ctx, `SELECT kind,version FROM hosted_schema_contract WHERE singleton`).Scan(&kind, &version); e != nil {
			return e
		}
		if kind != "portico-hosted" || version != CurrentHostedSchemaVersion {
			return errors.New("unsupported Hosted schema; provision the current format")
		}
		var valid bool
		if e := tx.QueryRowContext(ctx, `SELECT to_regclass(current_schema()||'.identity_security_returns') IS NOT NULL AND to_regclass(current_schema()||'.account_security_return_intents') IS NOT NULL AND to_regclass(current_schema()||'.app_authorizations') IS NOT NULL AND to_regclass(current_schema()||'.hosted_signing_state') IS NOT NULL AND to_regclass(current_schema()||'.server_leave_receipts') IS NOT NULL AND to_regclass(current_schema()||'.owner_claim_starts') IS NOT NULL AND to_regclass(current_schema()||'.setup_claim_starts') IS NOT NULL AND to_regclass(current_schema()||'.server_member_incarnations') IS NOT NULL AND to_regclass(current_schema()||'.server_member_endings') IS NOT NULL AND NOT EXISTS(SELECT 1 FROM pg_tables WHERE schemaname=current_schema() AND tablename IN('profiles','memberships','restrictions','invitations','profile_server_policies','account_erasure_jobs','hosted_schema_migrations'))`).Scan(&valid); e != nil {
			return e
		}
		if !valid {
			return errors.New("unsupported Hosted relations; reset pre-release schema")
		}
		return nil
	}
	var occupied bool
	if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class WHERE relnamespace=current_schema()::regnamespace) OR EXISTS(SELECT 1 FROM pg_proc WHERE pronamespace=current_schema()::regnamespace) OR EXISTS(SELECT 1 FROM pg_type WHERE typnamespace=current_schema()::regnamespace)`).Scan(&occupied); e != nil {
		return e
	}
	if occupied {
		return errors.New("unrecognized Hosted schema; provision the current format")
	}
	_, e := tx.ExecContext(ctx, currentSchema)
	return e
}
