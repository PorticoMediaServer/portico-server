package postgres

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/sessions"
	"time"
)

type sessionReviewCursor struct {
	Account string    `json:"account"`
	Current string    `json:"current"`
	Epoch   int64     `json:"epoch"`
	Before  time.Time `json:"before"`
	ID      string    `json:"id"`
}

// The caller's credential is checked again inside each read/mutation transaction.
// Revocation takes the account lock first, matching refresh/password/other logout.
func reviewCaller(ctx context.Context, tx *sql.Tx, account string, hash []byte) (string, int64, error) {
	var current string
	var epoch int64
	e := tx.QueryRowContext(ctx, `SELECT s.family_id,a.auth_epoch FROM account_sessions s JOIN accounts a ON a.id=s.account_id JOIN session_families f ON f.id=s.family_id AND f.account_id=s.account_id WHERE s.token_hash=$1 AND s.account_id=$2 AND s.expires_at>statement_timestamp() AND f.revoked_at IS NULL AND f.absolute_expires_at>statement_timestamp() AND f.idle_expires_at>statement_timestamp() AND f.account_epoch=a.auth_epoch`, hash, account).Scan(&current, &epoch)
	if errors.Is(e, sql.ErrNoRows) {
		return "", 0, sessions.ErrGrant
	}
	return current, epoch, e
}
func (s *Store) ReviewFamilies(ctx context.Context, account string, hash []byte, cursor string) (sessions.ReviewPage, error) {
	out := sessions.ReviewPage{AccountID: account, Items: []sessions.ReviewedFamily{}}
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	current, epoch, e := reviewCaller(ctx, tx, account, hash)
	if e != nil {
		return out, e
	}
	out.CurrentFamilyID = current
	if e = tx.QueryRowContext(ctx, `SELECT statement_timestamp()`).Scan(&out.ObservedAt); e != nil {
		return out, e
	}
	var after sessionReviewCursor
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if len(cursor) > 2048 || err != nil || json.Unmarshal(raw, &after) != nil || after.ID == "" || len(after.ID) > 128 || after.Before.IsZero() {
			return out, sessions.ErrReviewInvalid
		}
		if after.Account != account || after.Current != current || after.Epoch != epoch {
			return out, sessions.ErrReviewCursor
		}
	}
	rows, e := tx.QueryContext(ctx, `SELECT id,mode,created_at,LEAST(absolute_expires_at,idle_expires_at),revoked_at,CASE WHEN revoked_at IS NOT NULL OR account_epoch<>$4 THEN 'revoked' WHEN LEAST(absolute_expires_at,idle_expires_at)<=now() THEN 'expired' ELSE 'active' END FROM session_families WHERE account_id=$1 AND ($2='' OR (created_at,id)<($3,$2)) ORDER BY created_at DESC,id DESC LIMIT 41`, account, after.ID, after.Before, epoch)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v sessions.ReviewedFamily
		var revoked sql.NullTime
		if e = rows.Scan(&v.ID, &v.Mode, &v.CreatedAt, &v.ExpiresAt, &revoked, &v.Status); e != nil {
			rows.Close()
			return out, e
		}
		v.Current = v.ID == current
		if revoked.Valid {
			v.RevokedAt = &revoked.Time
		}
		if len(out.Items) == 40 {
			last := out.Items[len(out.Items)-1]
			raw, _ := json.Marshal(sessionReviewCursor{account, current, epoch, last.CreatedAt, last.ID})
			out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
			break
		}
		out.Items = append(out.Items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func (s *Store) RevokeReviewedFamily(ctx context.Context, account string, hash []byte, id string) (sessions.ReviewRevocation, error) {
	out := sessions.ReviewRevocation{ID: id}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	var locked string
	if e = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&locked); errors.Is(e, sql.ErrNoRows) {
		return out, sessions.ErrGrant
	} else if e != nil {
		return out, e
	}
	current, _, e := reviewCaller(ctx, tx, account, hash)
	if e != nil {
		return out, e
	}
	var owned bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM session_families WHERE id=$1 AND account_id=$2)`, id, account).Scan(&owned); e != nil {
		return out, e
	}
	if !owned {
		return out, sessions.ErrReviewNotFound
	}
	if e = revokeFamily(ctx, tx, id, "account_session_review"); e != nil {
		return out, e
	}
	if e = audit(ctx, tx, account, "session.review_revoke", id, 0); e != nil {
		return out, e
	}
	out.Current = current == id
	out.Revoked = true
	return out, tx.Commit()
}
