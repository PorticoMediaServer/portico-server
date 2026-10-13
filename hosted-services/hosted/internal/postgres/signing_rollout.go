package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"time"

	"portico.local/hosted/internal/security"
	"portico.local/hosted/trust"
)

const signingRolloutSchema = `
CREATE TABLE hosted_signing_state(singleton boolean PRIMARY KEY CHECK(singleton),revision numeric(20,0) NOT NULL DEFAULT 0,certificate_hash bytea NOT NULL DEFAULT '\x',pending boolean NOT NULL DEFAULT false,after_server text NOT NULL DEFAULT '',certificate bytea NOT NULL DEFAULT '\x');
INSERT INTO hosted_signing_state(singleton) VALUES(true);
CREATE TABLE hosted_signing_revocations(key_id text PRIMARY KEY);
`

// RecordSigningCertificate rejects rollback and forgotten terminal revocations.
// A new root-certified revision starts one durable, checkpointed wake sweep.
// The ordinary steady state creates no new wake or periodic traffic.
func (s *Store) RecordSigningCertificate(ctx context.Context, keys *security.Keys) error {
	if e := keys.ValidateSigner(time.Now()); e != nil {
		return e
	}
	cert := keys.Certificate
	raw, e := json.Marshal(cert)
	if e != nil {
		return e
	}
	digest := security.Hash(string(raw))
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var stored string
	var previous []byte
	if e = tx.QueryRowContext(ctx, `SELECT revision::text,certificate_hash FROM hosted_signing_state WHERE singleton FOR UPDATE`).Scan(&stored, &previous); e != nil {
		return e
	}
	old, ok := new(big.Int).SetString(stored, 10)
	if !ok {
		return trust.ErrTrust
	}
	next := new(big.Int).SetUint64(cert.Revision)
	if next.Cmp(old) < 0 || next.Cmp(old) == 0 && !bytes.Equal(digest, previous) {
		return trust.ErrTrust
	}
	if next.Cmp(old) == 0 {
		return tx.Commit()
	}
	// A root revocation is permanent even if a later operator mistakenly leaves
	// it off a certificate. Refuse that rollout before replacing runtime keys.
	incoming := map[string]bool{}
	for _, id := range cert.Revoked {
		incoming[id] = true
	}
	rows, e := tx.QueryContext(ctx, `SELECT key_id FROM hosted_signing_revocations`)
	if e != nil {
		return e
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		if !incoming[id] {
			e = trust.ErrTrust
			break
		}
	}
	readErr := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if readErr != nil {
		return readErr
	}
	for _, id := range cert.Revoked {
		if _, e = tx.ExecContext(ctx, `INSERT INTO hosted_signing_revocations(key_id) VALUES($1) ON CONFLICT DO NOTHING`, id); e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE hosted_signing_state SET revision=$1,certificate_hash=$2,pending=true,after_server='',certificate=$3 WHERE singleton`, strconv.FormatUint(cert.Revision, 10), digest, raw)
	if e != nil {
		return e
	}
	return tx.Commit()
}

// SigningRolloutBatch checkpoints at most 128 servers. The jobs use the existing
// signed-wake delivery/acknowledgment queue, so delivery limits, backoff and
// endpoint proof remain exactly the same. A competing replica shares the cursor.
func (s *Store) SigningRolloutBatch(ctx context.Context, revision uint64) (bool, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return false, e
	}
	defer tx.Rollback()
	var held, after string
	var pending bool
	if e = tx.QueryRowContext(ctx, `SELECT revision::text,pending,after_server FROM hosted_signing_state WHERE singleton FOR UPDATE`).Scan(&held, &pending, &after); e != nil {
		return false, e
	}
	if held != strconv.FormatUint(revision, 10) {
		return false, trust.ErrTrust
	}
	if !pending {
		return false, tx.Commit()
	}
	// Only server rows are locked here. No code in this batch waits on accounts,
	// claim operations or families while holding one of these rows.
	rows, e := tx.QueryContext(ctx, `SELECT id FROM servers WHERE id>$1 AND revoked_at IS NULL ORDER BY id FOR UPDATE LIMIT 128`, after)
	if e != nil {
		return false, e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	readErr := rows.Err()
	rows.Close()
	if e != nil {
		return false, e
	}
	if readErr != nil {
		return false, readErr
	}
	for _, id := range ids {
		var wake int64
		if e = tx.QueryRowContext(ctx, `UPDATE servers SET policy_revision=policy_revision+1 WHERE id=$1 AND policy_revision<9223372036854775807 RETURNING policy_revision`, id).Scan(&wake); errors.Is(e, sql.ErrNoRows) {
			return false, errors.New("server wake revision exhausted")
		} else if e != nil {
			return false, e
		}
		if e = enqueueWake(ctx, tx, id, wake); e != nil {
			return false, e
		}
		after = id
	}
	more := len(ids) == 128
	if _, e = tx.ExecContext(ctx, `UPDATE hosted_signing_state SET after_server=$1,pending=$2 WHERE singleton`, after, more); e != nil {
		return false, e
	}
	return more, tx.Commit()
}

func (s *Store) RunSigningRollout(ctx context.Context, revision uint64) {
	for ctx.Err() == nil {
		step, cancel := context.WithTimeout(ctx, 5*time.Second)
		more, e := s.SigningRolloutBatch(step, revision)
		cancel()
		if errors.Is(e, trust.ErrTrust) {
			return
		}
		if e == nil && !more {
			return
		}
		delay := 250 * time.Millisecond
		if e != nil {
			delay = 30 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// Existing processes learn a newly persisted root revocation as well as new
// processes. Unavailable storage retains the last verified knowledge; only a
// positive terminal revocation disables this process's shared signer fence.
func (s *Store) RunSignerRevocationFence(ctx context.Context, keys *security.Keys) {
	if keys.SignerRevoked == nil {
		return
	}
	timer := time.NewTicker(30 * time.Second)
	defer timer.Stop()
	for {
		step, cancel := context.WithTimeout(ctx, 5*time.Second)
		var revoked bool
		e := s.DB.QueryRowContext(step, `SELECT EXISTS(SELECT 1 FROM hosted_signing_revocations WHERE key_id=$1)`, keys.KeyID).Scan(&revoked)
		cancel()
		if e == nil && revoked {
			keys.SignerRevoked.Store(true)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
