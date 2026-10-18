package postgres

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"time"
)

// Unknown-username cool-off (HOST03). Password backoff lives on account rows,
// so without this an existing username reached a cool-off (429) after its free
// failures while an unknown one kept answering "invalid credentials", which
// told a guesser which usernames exist. A well-formed unknown username now
// runs admitPasswordAttempt's per-network and account-wide schedule, so the two
// answer alike, attempt for attempt.
//
// The state is durable and shared like the account rows: it lives in
// account_security_limits, so a restart or another replica sees the same
// counts. Names supplied by an attacker are never stored. Each (name, network)
// and each name maps through the stable seal-key HMAC onto one of a fixed set
// of buckets, so the table holds at most 3 * unknownPasswordBuckets of these
// rows however many names are tried. Two names (or networks) sharing a bucket
// share its count; that only ever cools an unknown name off sooner, never an
// account.
const (
	unknownPasswordBuckets = 1 << 16

	unknownPasswordNetwork = "password-unknown-network"
	unknownPasswordLock    = "password-unknown-lock"
	unknownPasswordWide    = "password-unknown-wide"
)

// unknownPasswordSubject names the row of the given kind for the bucket that
// (username, network) falls in.
func (s *AccountSecurity) unknownPasswordSubject(kind, username string, network []byte) []byte {
	return s.unknownPasswordBucket(kind, binary.BigEndian.Uint32(s.digest("password-unknown-slot", hex.EncodeToString(network), username))%unknownPasswordBuckets)
}
func (s *AccountSecurity) unknownPasswordBucket(kind string, slot uint32) []byte {
	return s.digest("rate", kind, strconv.FormatUint(uint64(slot), 10))
}

// admitUnknownPasswordAttempt counts one attempt for a well-formed username
// with no account, exactly as admitPasswordAttempt counts one for an account.
// The attempt then fails, so nothing is ever cleared. Rows are locked in one
// order (network, its cool-off, account-wide), and the network row serializes
// parallel attempts as the account row does.
func (s *AccountSecurity) admitUnknownPasswordAttempt(ctx context.Context, username string, network []byte) (bool, time.Time, error) {
	netKey := s.unknownPasswordSubject(unknownPasswordNetwork, username, network)
	lockKey := s.unknownPasswordSubject(unknownPasswordLock, username, network)
	wideKey := s.unknownPasswordSubject(unknownPasswordWide, username, allNetworks)
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return false, time.Time{}, e
	}
	defer tx.Rollback()
	const lockRow = `INSERT INTO account_security_limits(subject_hash,kind,window_start,attempts) VALUES($1,$2,clock_timestamp(),0) ON CONFLICT(subject_hash,kind) DO UPDATE SET attempts=account_security_limits.attempts RETURNING attempts,window_start,clock_timestamp()`
	var failures, ignored int
	var windowStart, until, now time.Time
	if e = tx.QueryRowContext(ctx, lockRow, netKey, unknownPasswordNetwork).Scan(&failures, &windowStart, &now); e != nil {
		return false, time.Time{}, e
	}
	// The cool-off row's window_start is the time the cool-off ends.
	if e = tx.QueryRowContext(ctx, lockRow, lockKey, unknownPasswordLock).Scan(&ignored, &until, &now); e != nil {
		return false, time.Time{}, e
	}
	if until.After(now) {
		return false, until, tx.Commit()
	}
	var wide int
	if e = tx.QueryRowContext(ctx, `INSERT INTO account_security_limits(subject_hash,kind,window_start,attempts) VALUES($1,$2,clock_timestamp(),1) ON CONFLICT(subject_hash,kind) DO UPDATE SET window_start=CASE WHEN account_security_limits.window_start<clock_timestamp()-($3 * interval '1 second') THEN clock_timestamp() ELSE account_security_limits.window_start END,attempts=CASE WHEN account_security_limits.window_start<clock_timestamp()-($3 * interval '1 second') THEN 1 ELSE LEAST(account_security_limits.attempts+1,1000000) END RETURNING attempts,clock_timestamp()`, wideKey, unknownPasswordWide, int(accountWideWindow.Seconds())).Scan(&wide, &now); e != nil {
		return false, time.Time{}, e
	}
	failures, windowStart, until = passwordFailure(failures, windowStart, now, wide > accountWideCeiling)
	if _, e = tx.ExecContext(ctx, `UPDATE account_security_limits SET attempts=$3,window_start=$4 WHERE subject_hash=$1 AND kind=$2`, netKey, unknownPasswordNetwork, failures, windowStart); e != nil {
		return false, time.Time{}, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_security_limits SET window_start=$3 WHERE subject_hash=$1 AND kind=$2`, lockKey, unknownPasswordLock, until); e != nil {
		return false, time.Time{}, e
	}
	return true, time.Time{}, tx.Commit()
}

// passwordFailure is admitPasswordAttempt's per-network arithmetic: the
// network's count and window after one more failure, and when its cool-off
// ends.
func passwordFailure(failures int, windowStart, now time.Time, underAttack bool) (int, time.Time, time.Time) {
	floor := time.Duration(0)
	if underAttack {
		floor = underAttackCoolOff
	}
	switch {
	case windowStart.Before(now.Add(-networkWindow)):
		return 1, now, now.Add(floor)
	case failures+1 <= networkFreeFailures:
		return failures + 1, windowStart, now.Add(floor)
	default:
		return min(failures+1, 1000000), windowStart, now.Add(min(coolOffCap, max(floor, coolOffBase<<min(failures-networkFreeFailures, 10))))
	}
}
