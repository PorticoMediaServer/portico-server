package identity

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"portico.local/server/internal/dbwork"
)

// A backup is a plain copy of the database, and a restore puts an older copy
// back. Two things must not follow from that:
//
//   - A copy of a backup must not be a way in. The database holds, for every
//     signed-in device, what is needed to keep it signed in: the stored answer
//     to its last renewal (its current tokens), pending sign-in codes and their
//     answers, the first-run receipt. Anyone who could read a backup could use
//     them.
//   - A restore must not bring a session back to life. A device that was signed
//     out, a member who was turned off and a television that was blocked after
//     the backup was taken would all be signed in again, because the backup
//     remembers them as they were.
//
// Both are closed the same way, with no step for the owner. Every backup copy
// has its sessions ended and its live secrets removed as it is written
// (ScrubBackupCopy), so a backup holds no sign-in at all. And when a restore
// has just been applied, everyone is signed out before the server answers a
// request (SignEveryoneOutAfterRestore), whatever the restored copy held: an
// older backup, one made by another build, or a copy made by hand.
//
// API keys and personal access tokens are the one exception, on purpose: what
// an owner has connected to the server keeps working across a restore, as it
// does across a password change. A key that was live when the backup was taken
// is live again after it is restored, including one revoked in between; the
// owner sees it in the key list and can revoke it again.
//
// People sign in again after a restore. Their accounts, profiles, passwords,
// second factors, devices and everything they saved are untouched; so is each
// device's approval. What is deliberately not removed is anything that is not
// a session: password hashes and second-factor secrets stay in the database
// and in its backups, in the clear, as the server stores them.

// sessionScrub is every statement that ends sessions and removes the secrets
// that could continue or start one, with the table each needs. A table this
// build does not have is skipped, so the list never stops a backup.
var sessionScrub = []struct{ table, statement string }{
	// Every signed-in device, account session and paired screen.
	{"authorization_session_families", `UPDATE authorization_session_families SET revoked=1,revoked_reason='restored',revoked_at=?1 WHERE revoked=0`},
	// The tokens of every session that is now ended. An API key or personal
	// access token is not a sign-in and is not ended here: it has a lifetime of
	// its own (api_credentials.go), the schema keeps its family from being
	// swept above, and so its one token stays. What a backup holds of it is a
	// digest of its secret, which is no use to a reader of the file.
	{"authorization_family_tokens", `UPDATE authorization_family_tokens SET retired=1 WHERE retired=0 AND family_id NOT IN(SELECT id FROM authorization_session_families WHERE revoked=0)`},
	// Renewal: the credentials themselves, and the stored answers that hold a
	// session's current tokens in the clear.
	{"identity_refresh_receipts", `DELETE FROM identity_refresh_receipts`},
	{"identity_refresh_predecessors", `DELETE FROM identity_refresh_predecessors`},
	{"identity_refresh_credentials", `DELETE FROM identity_refresh_credentials`},
	{"authorization_family_renewals", `DELETE FROM authorization_family_renewals`},
	// Sign-ins in progress: television codes and their sealed answers, second
	// factor challenges, Portico Account challenges, the first-run receipt.
	{"quick_connect_v1", `DELETE FROM quick_connect_v1`},
	{"identity_factor_challenges", `DELETE FROM identity_factor_challenges`},
	{"hosted_identity_challenges", `DELETE FROM hosted_identity_challenges`},
	{"onboarding_state_v1", `UPDATE onboarding_state_v1 SET receipt=NULL WHERE receipt IS NOT NULL`},
	// Things a device holds instead of entering a PIN or a password again.
	{"direct_profile_trust", `DELETE FROM direct_profile_trust`},
	{"topshelf_tokens", `DELETE FROM topshelf_tokens`},
	// Screens paired as Cast targets renew with their own device token.
	{"social_cast_devices", `UPDATE social_cast_devices SET state='revoked' WHERE state IN('pending','active')`},
	{"social_cast_bootstraps", `UPDATE social_cast_bootstraps SET state='cancelled' WHERE state='pending'`},
}

// endEverySessionTx runs the scrub in tx. It reports how many sessions it
// ended and how many rows it changed in all.
func endEverySessionTx(ctx context.Context, tx *sql.Tx, now time.Time) (ended, changed int64, err error) {
	stamp := now.UTC().Format(time.RFC3339)
	var live int64
	for _, step := range sessionScrub {
		var present bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)`, step.table).Scan(&present); err != nil {
			return 0, 0, err
		}
		if !present {
			continue
		}
		var result sql.Result
		if step.table == "authorization_session_families" {
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM authorization_session_families WHERE revoked=0`).Scan(&live); err != nil {
				return 0, 0, err
			}
			result, err = tx.ExecContext(ctx, step.statement, stamp)
		} else {
			result, err = tx.ExecContext(ctx, step.statement)
		}
		if err != nil {
			return 0, 0, err
		}
		n, _ := result.RowsAffected()
		changed += n
		if step.table == "authorization_session_families" {
			// Counted afterwards: the statement also touches the families the
			// schema keeps, and those did not end.
			var kept int64
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM authorization_session_families WHERE revoked=0`).Scan(&kept); err != nil {
				return 0, 0, err
			}
			ended = live - kept
		}
	}
	return ended, changed, nil
}

// ScrubBackupCopy ends every session in a finished backup copy of the database
// and removes the live secrets from it, before the copy becomes a backup. It
// works on the copy's private candidate handle, never the running server's
// database: it takes no write-gate ticket and has no live effect. The backup
// service calls it on each copy it writes; it is safe to run again on a copy
// that was already scrubbed. The caller closes the candidate, which folds the
// copy's write-ahead log back into the file.
func ScrubBackupCopy(ctx context.Context, copy *dbwork.Candidate) error {
	if copy == nil {
		return errors.New("no backup copy to scrub")
	}
	return copy.WithTx(ctx, func(tx *sql.Tx) error {
		// Deleted rows are overwritten, not just unlinked, so the secrets do
		// not linger in the copy's free pages for a reader of the file to
		// find. This costs a write per freed page, not a rewrite of the
		// database. The setting belongs to a connection, so it is made on the
		// one that does the deleting, and checked.
		var secure int
		if err := tx.QueryRowContext(ctx, `PRAGMA secure_delete=ON`).Scan(&secure); err != nil {
			return err
		}
		if secure != 1 {
			return errors.New("the backup copy cannot be scrubbed: deleted content would stay readable")
		}
		_, _, err := endEverySessionTx(ctx, tx, time.Now())
		return err
	})
}

const restoreHandledKey = "identity.restoreHandled"

// SignEveryoneOutAfterRestore signs everyone out once for each restore that
// was applied. restore identifies the restore (the time its outcome was
// recorded); the database remembers the last one it handled, and a restored
// database remembers an older one or none, so the first start after a restore
// always acts and later starts do not. It runs before the server accepts
// requests, and reports whether it acted.
func (s *Service) SignEveryoneOutAfterRestore(ctx context.Context, restore string) (bool, error) {
	if restore == "" {
		return false, nil
	}
	acted := false
	var ended int64
	err := dbwork.WithWriteTx(ctx, s.db, dbwork.ClassSecurityFence, func(tx *sql.Tx) error {
		acted, ended = false, 0
		var handled string
		err := tx.QueryRowContext(ctx, `SELECT value FROM configuration WHERE key=?`, restoreHandledKey).Scan(&handled)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if handled == restore {
			return nil
		}
		if ended, _, err = endEverySessionTx(ctx, tx, time.Now()); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO configuration(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, restoreHandledKey, restore); err != nil {
			return err
		}
		acted = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if acted {
		log.Printf("This server was restored from a backup. Everyone has been signed out (%d sessions ended); accounts, passwords and devices are unchanged, and people sign in again.", ended)
	}
	return acted, nil
}
