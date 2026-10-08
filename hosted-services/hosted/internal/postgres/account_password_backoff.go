package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"time"
)

// Password backoff (SEC-17; Justin, 23 Sep: a correct username and password
// always signs in, and failures only earn a cool-off).
//
// Failures are counted per (account, client network): an IPv4 address or an
// IPv6 /64, hashed. After networkFreeFailures failures within networkWindow, a
// network must wait before its next attempt, doubling from coolOffBase up to
// coolOffCap. The wait is the only effect: once it's over, a correct password
// signs in, and success clears the count. Another network is never affected,
// so an attacker elsewhere can't lock the owner out.
//
// One more row per account (the zero network) counts failures from every
// network. While a distributed attack keeps it past accountWideCeiling within
// accountWideWindow, every failing network's cool-off starts at a minute
// instead of after its free failures. It still only follows failures: a network
// that hasn't failed signs in at once.
//
// Each attempt is counted before the password is checked, under the row lock,
// so a burst of parallel guesses gets no more tries than a sequence.
const (
	networkFreeFailures = 4
	networkWindow       = 15 * time.Minute
	coolOffBase         = 30 * time.Second
	coolOffCap          = 15 * time.Minute
	accountWideCeiling  = 100
	accountWideWindow   = time.Hour
	underAttackCoolOff  = time.Minute
)

var allNetworks = make([]byte, 32)

func passwordNetwork(network string) []byte {
	sum := sha256.Sum256([]byte("portico-password-network:" + network))
	return sum[:]
}

// admitPasswordAttempt counts one attempt. It reports false with the time the
// cool-off ends when this network must wait before trying again.
func (s *AccountSecurity) admitPasswordAttempt(ctx context.Context, account string, network []byte) (bool, time.Time, error) {
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return false, time.Time{}, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT INTO account_password_backoff(account_id,network) VALUES($1,$2),($1,$3) ON CONFLICT DO NOTHING`, account, network, allNetworks); e != nil {
		return false, time.Time{}, e
	}
	var until time.Time
	var cooling bool
	if e = tx.QueryRowContext(ctx, `SELECT locked_until,locked_until>clock_timestamp() FROM account_password_backoff WHERE account_id=$1 AND network=$2 FOR UPDATE`, account, network).Scan(&until, &cooling); e != nil {
		return false, time.Time{}, e
	}
	if cooling {
		return false, until, tx.Commit()
	}
	var wide int
	if e = tx.QueryRowContext(ctx, `UPDATE account_password_backoff SET
 failures=CASE WHEN window_start<clock_timestamp()-$2*interval '1 second' THEN 1 ELSE failures+1 END,
 window_start=CASE WHEN window_start<clock_timestamp()-$2*interval '1 second' THEN clock_timestamp() ELSE window_start END
 WHERE account_id=$1 AND network=$3 RETURNING failures`, account, accountWideWindow.Seconds(), allNetworks).Scan(&wide); e != nil {
		return false, time.Time{}, e
	}
	// The count is provisional: success resets it. The cool-off it sets applies
	// to the next attempt only if this one fails.
	if _, e = tx.ExecContext(ctx, `UPDATE account_password_backoff SET
 failures=CASE WHEN window_start<clock_timestamp()-$3*interval '1 second' THEN 1 ELSE failures+1 END,
 window_start=CASE WHEN window_start<clock_timestamp()-$3*interval '1 second' THEN clock_timestamp() ELSE window_start END,
 locked_until=clock_timestamp()+CASE
  WHEN window_start<clock_timestamp()-$3*interval '1 second' THEN CASE WHEN $6 THEN $7 ELSE 0 END
  WHEN failures+1<=$4 THEN CASE WHEN $6 THEN $7 ELSE 0 END
  ELSE LEAST($8,GREATEST(CASE WHEN $6 THEN $7 ELSE 0 END,$5*power(2,LEAST(failures+1-$4-1,10))))
 END*interval '1 second'
 WHERE account_id=$1 AND network=$2`, account, network, networkWindow.Seconds(), networkFreeFailures, coolOffBase.Seconds(), wide > accountWideCeiling, underAttackCoolOff.Seconds(), coolOffCap.Seconds()); e != nil {
		return false, time.Time{}, e
	}
	return true, time.Time{}, tx.Commit()
}

// passwordSucceededTx clears this network's failures, including this attempt's
// provisional count and cool-off. It runs in the login transaction.
func passwordSucceededTx(ctx context.Context, tx *sql.Tx, account string, network []byte) error {
	if _, e := tx.ExecContext(ctx, `UPDATE account_password_backoff SET failures=0,window_start=clock_timestamp(),locked_until=clock_timestamp() WHERE account_id=$1 AND network=$2`, account, network); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `UPDATE account_password_backoff SET failures=GREATEST(failures-1,0) WHERE account_id=$1 AND network=$2`, account, allNetworks)
	return e
}
