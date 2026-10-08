package postgres

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"time"
)

const exportSchema = `CREATE TABLE account_exports(id text PRIMARY KEY,account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,operation_id text NOT NULL,state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','ready')),ciphertext bytea,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '24 hours',UNIQUE(account_id,operation_id)); CREATE INDEX export_pending ON account_exports(created_at) WHERE state='pending'; CREATE INDEX export_expiry ON account_exports(expires_at);`

type ExportJob struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// ExportRequest names one export request so a retried request returns the same job.
type ExportRequest struct {
	OperationID string `json:"operationId"`
}

// RequestExport starts the account's data export. A signed-in session is
// enough (Justin, 23 Sep: only email, password and two-step changes ask for
// the current password).
func (s *AccountSecurity) RequestExport(ctx context.Context, token string, q ExportRequest) (ExportJob, error) {
	var out ExportJob
	if len(q.OperationID) < 16 || len(q.OperationID) > 128 {
		return out, identity.ErrInvalid
	}
	tx, a, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	e = tx.QueryRowContext(ctx, `INSERT INTO account_exports(id,account_id,operation_id) SELECT $1,$2,$3 WHERE NOT EXISTS(SELECT 1 FROM account_exports WHERE account_id=$2 AND expires_at>clock_timestamp()) ON CONFLICT(account_id,operation_id) DO NOTHING RETURNING id,state,expires_at`, security.RandomID("export_"), a.AccountID, q.OperationID).Scan(&out.ID, &out.State, &out.ExpiresAt)
	if errors.Is(e, sql.ErrNoRows) {
		e = tx.QueryRowContext(ctx, `SELECT id,state,expires_at FROM account_exports WHERE account_id=$1 AND expires_at>clock_timestamp() ORDER BY created_at DESC LIMIT 1`, a.AccountID).Scan(&out.ID, &out.State, &out.ExpiresAt)
	}
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}

// Export reads the account's own export; with download set, it also returns
// the document once ready. The export was requested with a fresh security
// proof and is readable only by this account's session for 24 hours, so no
// separate mailed download token is needed.
func (s *AccountSecurity) Export(ctx context.Context, token, id string, download bool) (ExportJob, []byte, error) {
	var out ExportJob
	tx, a, e := s.authorize(ctx, token)
	if e != nil {
		return out, nil, e
	}
	defer tx.Rollback()
	var cipher []byte
	e = tx.QueryRowContext(ctx, `SELECT id,state,expires_at,ciphertext FROM account_exports WHERE id=$1 AND account_id=$2 AND expires_at>clock_timestamp()`, id, a.AccountID).Scan(&out.ID, &out.State, &out.ExpiresAt, &cipher)
	if errors.Is(e, sql.ErrNoRows) {
		e = identity.ErrInvalid
	}
	if e != nil {
		return out, nil, e
	}
	var data []byte
	if download {
		if out.State != "ready" {
			return out, nil, identity.ErrChallenge
		}
		raw, err := s.Keys.Open(cipher, "account-export:"+id)
		if err != nil {
			return out, nil, err
		}
		data = []byte(raw)
	}
	return out, data, tx.Commit()
}
