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

func (s *Store) Register(ctx context.Context, a identity.Account, password, token []byte, expiry time.Time, family identity.FamilySeed) error {
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
	return v, e
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
