package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
	"portico.local/hosted/internal/identity"
	"sync"
	"sync/atomic"
	"time"
)

type Store struct {
	claimWatch claimWatchers
	// Fleet-wide CGNAT prevalence, counted in memory so route observations never
	// serialize on one row (A49). A Prometheus counter per process; it restarts at zero.
	cgnatReports, cgnatShared atomic.Int64
	DB                        *sql.DB
	// url is kept so a notification listener can open a connection of its own, outside the
	// request pool: a pooled connection would be recycled by ConnMaxLifetime while it waited.
	url          string
	listenerOnce sync.Once
	listener     *Listener
}

// Notifications returns this database's one listener. Workers subscribe to it at wiring time
// and it runs for the life of the process; readiness asks the same listener whether it is
// connected (a fresh listener never is, which made /ready always fail).
func (s *Store) Notifications() *Listener {
	s.listenerOnce.Do(func() { s.listener = NewListener(s.url) })
	return s.listener
}

func Open(ctx context.Context, url string) (*Store, error) {
	config, e := pgx.ParseConfig(url)
	if e != nil {
		return nil, e
	}
	// Public account contracts serialize timestamps with a UTC Z suffix. pgx's
	// default binary timestamptz decoder uses the process's local timezone, so
	// a non-UTC VPS otherwise breaks browser SSO, registration and MFA parsing.
	db := stdlib.OpenDB(*config, stdlib.OptionAfterConnect(func(_ context.Context, conn *pgx.Conn) error {
		conn.TypeMap().RegisterType(&pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID, Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}})
		return nil
	}))
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(30 * time.Minute)
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, e
	}
	return &Store{DB: db, url: url}, nil
}

// An export's repeatable-read scan can be proportional to that account's data.
// It uses its own one-connection pool so it cannot consume a foreground slot.
func (s *Store) backgroundReadDB() (*sql.DB, error) {
	config, e := pgx.ParseConfig(s.url)
	if e != nil {
		return nil, e
	}
	db := stdlib.OpenDB(*config, stdlib.OptionAfterConnect(func(_ context.Context, conn *pgx.Conn) error {
		conn.TypeMap().RegisterType(&pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID, Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}})
		return nil
	}))
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	return db, nil
}
func (s *Store) Migrate(ctx context.Context) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1941001)`); e != nil {
		return e
	}
	if e = InitializeCurrentHostedSchemaTx(ctx, tx); e != nil {
		return e
	}
	return tx.Commit()
}

// MigrateDryRun runs the whole migration in a transaction and rolls it back,
// reporting the schema version before and after. Operators run it against a
// restored copy of production before a deploy.
func (s *Store) MigrateDryRun(ctx context.Context) (before, after int, err error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return 0, 0, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1941001)`); e != nil {
		return 0, 0, e
	}
	// A76: an empty database has no contract table; reading it would abort the transaction.
	var present bool
	if e = tx.QueryRowContext(ctx, `SELECT to_regclass(current_schema()||'.hosted_schema_contract') IS NOT NULL`).Scan(&present); e != nil {
		return 0, 0, e
	}
	if present {
		if e = tx.QueryRowContext(ctx, `SELECT version FROM hosted_schema_contract WHERE singleton`).Scan(&before); e != nil {
			return 0, 0, e
		}
	}
	if e = InitializeCurrentHostedSchemaTx(ctx, tx); e != nil {
		return before, 0, e
	}
	if e = tx.QueryRowContext(ctx, `SELECT version FROM hosted_schema_contract WHERE singleton`).Scan(&after); e != nil {
		return before, 0, e
	}
	return before, after, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS accounts(id text PRIMARY KEY,username text UNIQUE NOT NULL,display_name text NOT NULL,password_hash bytea CHECK(password_hash IS NULL OR octet_length(password_hash)>0),auth_epoch bigint NOT NULL DEFAULT 0,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS session_families(id text PRIMARY KEY,account_id text NOT NULL REFERENCES accounts(id),mode text NOT NULL CHECK(mode IN ('native','browser')),current_refresh_hash bytea NOT NULL,generation bigint NOT NULL DEFAULT 0,account_epoch bigint NOT NULL,absolute_expires_at timestamptz NOT NULL,idle_expires_at timestamptz NOT NULL,revoked_at timestamptz,revoke_reason text,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(id,account_id));
CREATE TABLE IF NOT EXISTS profiles(id text PRIMARY KEY,account_id text NOT NULL REFERENCES accounts(id),name text NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS profiles_account ON profiles(account_id,id);
CREATE TABLE IF NOT EXISTS account_sessions(token_hash bytea PRIMARY KEY,account_id text NOT NULL REFERENCES accounts(id),expires_at timestamptz NOT NULL,family_id text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),FOREIGN KEY(family_id,account_id) REFERENCES session_families(id,account_id) ON DELETE CASCADE);
CREATE INDEX IF NOT EXISTS account_sessions_expiry ON account_sessions(expires_at);
` + CurrentClaimServerDeclaration + `
CREATE TABLE IF NOT EXISTS memberships(server_id text NOT NULL REFERENCES servers(id),account_id text NOT NULL REFERENCES accounts(id),profile_id text NOT NULL REFERENCES profiles(id),role text NOT NULL CHECK(role IN ('owner','member')),allowed_libraries text[] NOT NULL DEFAULT '{}',revoked boolean NOT NULL DEFAULT false,PRIMARY KEY(server_id,account_id,profile_id));
CREATE INDEX IF NOT EXISTS memberships_account ON memberships(account_id,server_id) WHERE NOT revoked;
CREATE UNIQUE INDEX IF NOT EXISTS membership_one_owner ON memberships(server_id) WHERE role='owner';
CREATE TABLE IF NOT EXISTS restrictions(server_id text NOT NULL REFERENCES servers(id),account_id text NOT NULL,profile_id text NOT NULL,local_revision bigint NOT NULL,allowed_libraries text[] NOT NULL,revoked boolean NOT NULL,PRIMARY KEY(server_id,account_id,profile_id),FOREIGN KEY(server_id,account_id,profile_id) REFERENCES memberships(server_id,account_id,profile_id));
CREATE TABLE IF NOT EXISTS attachment_tickets(token_hash bytea PRIMARY KEY,server_id text NOT NULL REFERENCES servers(id),account_id text NOT NULL REFERENCES accounts(id),profile_id text NOT NULL REFERENCES profiles(id),expires_at timestamptz NOT NULL,consumed_at timestamptz);
CREATE INDEX IF NOT EXISTS attachment_expiry ON attachment_tickets(expires_at);
CREATE TABLE IF NOT EXISTS mutation_receipts(server_id text NOT NULL REFERENCES servers(id),operation_id text NOT NULL,kind text NOT NULL,request_hash bytea NOT NULL,local_revision bigint NOT NULL,policy_revision bigint NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(server_id,operation_id));
CREATE TABLE IF NOT EXISTS wake_state(server_id text NOT NULL REFERENCES servers(id),kind text NOT NULL,desired_revision bigint NOT NULL DEFAULT 0,acked_revision bigint NOT NULL DEFAULT 0,lease_token text,lease_until timestamptz,next_attempt_at timestamptz NOT NULL DEFAULT now(),attempts integer NOT NULL DEFAULT 0,last_status integer,first_attempt_at timestamptz,wake_unreachable boolean NOT NULL DEFAULT false,PRIMARY KEY(server_id,kind));
CREATE INDEX IF NOT EXISTS wake_due ON wake_state(next_attempt_at) WHERE desired_revision>acked_revision;
CREATE TABLE IF NOT EXISTS security_audit(id bigserial PRIMARY KEY,actor_id text NOT NULL,action text NOT NULL,target_id text NOT NULL,revision bigint,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS invitations(id text PRIMARY KEY,server_id text NOT NULL REFERENCES servers(id),owner_id text NOT NULL REFERENCES accounts(id),recipient_id text NOT NULL REFERENCES accounts(id),operation_id text NOT NULL,request_hash bytea NOT NULL,allowed_libraries text[] NOT NULL,expected_policy_revision bigint NOT NULL,expected_local_revision bigint NOT NULL,expires_at timestamptz NOT NULL,status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','accepted','cancelled')),accepted_profile_id text REFERENCES profiles(id),accepted_policy_revision bigint,accepted_local_revision bigint,UNIQUE(server_id,operation_id));
CREATE INDEX IF NOT EXISTS invitations_recipient ON invitations(recipient_id,expires_at) WHERE status='pending';
CREATE UNIQUE INDEX IF NOT EXISTS verified_server_endpoint ON servers(base_url) WHERE endpoint_verified;
CREATE TABLE IF NOT EXISTS device_rate_limits(subject_hash bytea NOT NULL,kind text NOT NULL,window_start timestamptz NOT NULL,attempts integer NOT NULL,PRIMARY KEY(subject_hash,kind));
CREATE INDEX IF NOT EXISTS device_rate_expiry ON device_rate_limits(window_start);
CREATE TABLE IF NOT EXISTS device_authorizations(id text PRIMARY KEY,creation_hash bytea NOT NULL UNIQUE,request_hash bytea NOT NULL,creation_cipher bytea NOT NULL,device_hash bytea NOT NULL UNIQUE,user_hash bytea NOT NULL UNIQUE,device_name text NOT NULL,platform text NOT NULL,app_version text NOT NULL,expires_at timestamptz NOT NULL,next_poll_at timestamptz NOT NULL,interval_seconds integer NOT NULL DEFAULT 5,status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','denied','cancelled','consumed')),approved_account_id text REFERENCES accounts(id),approved_epoch bigint,decided_at timestamptz,session_hash bytea,session_cipher bytea,session_expires_at timestamptz,recovery_until timestamptz,consumed_at timestamptz,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS device_authorization_expiry ON device_authorizations(expires_at);
CREATE TABLE IF NOT EXISTS hosted_devices(id text PRIMARY KEY,account_id text NOT NULL REFERENCES accounts(id),name text NOT NULL,platform text NOT NULL,app_version text NOT NULL,session_hash bytea NOT NULL,family_id text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),revoked_at timestamptz,FOREIGN KEY(family_id,account_id) REFERENCES session_families(id,account_id) ON DELETE CASCADE);
CREATE INDEX IF NOT EXISTS hosted_devices_account ON hosted_devices(account_id);
CREATE INDEX IF NOT EXISTS session_families_account ON session_families(account_id);
CREATE INDEX IF NOT EXISTS session_families_expiry ON session_families(absolute_expires_at);
CREATE TABLE IF NOT EXISTS refresh_tokens(token_hash bytea PRIMARY KEY,family_id text NOT NULL REFERENCES session_families(id) ON DELETE CASCADE,generation bigint NOT NULL,consumed_at timestamptz,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(family_id,generation));
CREATE TABLE IF NOT EXISTS refresh_receipts(family_id text NOT NULL REFERENCES session_families(id) ON DELETE CASCADE,request_id text NOT NULL,input_hash bytea NOT NULL,output_hash bytea NOT NULL,access_hash bytea NOT NULL,ciphertext bytea,expires_at timestamptz NOT NULL,PRIMARY KEY(family_id,request_id));
CREATE INDEX IF NOT EXISTS refresh_receipts_expiry ON refresh_receipts(expires_at) WHERE ciphertext IS NOT NULL;
CREATE INDEX IF NOT EXISTS account_sessions_family ON account_sessions(family_id);
`

func (s *Store) Register(ctx context.Context, a identity.Account, p identity.Profile, password, token []byte, expiry time.Time, family identity.FamilySeed) error {
	if !identity.PasswordInstalled(password) {
		return identity.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT INTO accounts(id,username,display_name,password_hash) VALUES($1,$2,$3,$4)`, a.ID, a.Username, a.DisplayName, password); e != nil {
		var pg *pgconn.PgError
		if errors.As(e, &pg) && pg.Code == "23505" {
			return identity.ErrConflict
		}
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO profiles(id,account_id,name) VALUES($1,$2,$3)`, p.ID, a.ID, p.Name); e != nil {
		return e
	}
	if e = insertFamily(ctx, tx, a.ID, token, expiry, family); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO security_audit(actor_id,action,target_id) VALUES($1,'account.register',$1)`, a.ID); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Password(ctx context.Context, username string) (identity.Account, []byte, error) {
	var a identity.Account
	var h []byte
	e := s.DB.QueryRowContext(ctx, `SELECT id,username,display_name,password_hash FROM accounts WHERE username=$1`, username).Scan(&a.ID, &a.Username, &a.DisplayName, &h)
	if errors.Is(e, sql.ErrNoRows) {
		e = identity.ErrCredentials
	}
	return a, h, e
}
func (s *Store) CreateSession(ctx context.Context, account string, hash []byte, expiry time.Time, expectedPassword []byte, family identity.FamilySeed) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var current []byte
	if e = tx.QueryRowContext(ctx, `SELECT password_hash FROM accounts WHERE id=$1 FOR SHARE`, account).Scan(&current); e != nil {
		return e
	}
	if !identity.PasswordInstalled(current) || !identity.PasswordInstalled(expectedPassword) || !bytes.Equal(current, expectedPassword) {
		return identity.ErrCredentials
	}
	if e = insertFamily(ctx, tx, account, hash, expiry, family); e != nil {
		return e
	}
	if e = audit(ctx, tx, account, "session.create", account, 0); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Authenticate(ctx context.Context, hash []byte) (identity.Viewer, error) {
	var v identity.Viewer
	e := s.DB.QueryRowContext(ctx, `SELECT a.id,a.username,a.display_name FROM account_sessions s JOIN accounts a ON a.id=s.account_id JOIN session_families f ON f.id=s.family_id AND f.account_id=s.account_id WHERE s.token_hash=$1 AND s.expires_at>statement_timestamp() AND f.revoked_at IS NULL AND f.absolute_expires_at>statement_timestamp() AND f.idle_expires_at>statement_timestamp() AND f.account_epoch=a.auth_epoch`, hash).Scan(&v.Account.ID, &v.Account.Username, &v.Account.DisplayName)
	if errors.Is(e, sql.ErrNoRows) {
		return v, identity.ErrCredentials
	}
	if e != nil {
		return v, e
	}
	v.Profiles, e = s.Profiles(ctx, v.Account.ID)
	return v, e
}
func (s *Store) Profiles(ctx context.Context, id string) ([]identity.Profile, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,name FROM profiles WHERE account_id=$1 AND deleted_at IS NULL ORDER BY position,created_at,id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	result := []identity.Profile{}
	for rows.Next() {
		var p identity.Profile
		if e = rows.Scan(&p.ID, &p.Name); e != nil {
			return nil, e
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
func (s *Store) Revoke(ctx context.Context, hash []byte) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var account string
	var family string
	e = tx.QueryRowContext(ctx, `SELECT account_id,family_id FROM account_sessions WHERE token_hash=$1`, hash).Scan(&account, &family)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR SHARE`, account); e != nil {
		return e
	}
	if e = revokeFamily(ctx, tx, family, "logout"); e != nil {
		return e
	}
	if e = audit(ctx, tx, account, "session.revoke", account, 0); e != nil {
		return e
	}
	return tx.Commit()
}
