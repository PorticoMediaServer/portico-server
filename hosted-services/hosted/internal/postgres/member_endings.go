package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/hosted/internal/directory"
)

// A stale full list cannot resurrect an ended incarnation or replace a newer
// admitted membership. Legacy publications carry no proof of a fresh rejoin.
func membershipStandingRetiredTx(ctx context.Context, tx *sql.Tx, server string, member directory.MemberStanding, sequence int64) (bool, error) {
	var incarnation string
	var admitted, ended int64
	err := tx.QueryRowContext(ctx, `SELECT membership_id,standing_sequence,ending_sequence FROM server_member_incarnations WHERE server_id=$1 AND account_id=$2 FOR UPDATE`, server, member.AccountID).Scan(&incarnation, &admitted, &ended)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !member.Active {
		return sequence <= admitted || member.MembershipID != "" && member.MembershipID != incarnation || member.Ending != nil && member.Ending.Sequence <= admitted, nil
	}
	if member.MembershipID == "" {
		return ended > 0, nil
	}
	if sequence < admitted || member.MembershipID == incarnation && ended > 0 {
		return true, nil
	}
	return member.MembershipID != incarnation && sequence <= max(admitted, ended), nil
}

func admitMembershipIncarnationTx(ctx context.Context, tx *sql.Tx, server string, member directory.MemberStanding, sequence int64) error {
	if member.MembershipID == "" {
		return nil // Legacy standing is not evidence of an invented incarnation.
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO server_member_incarnations(server_id,account_id,membership_id,standing_sequence,server_name,retained_until) SELECT $1,$2,$3,$4,name,clock_timestamp()+interval '720 hours' FROM servers WHERE id=$1 ON CONFLICT(server_id,account_id) DO UPDATE SET membership_id=excluded.membership_id,standing_sequence=excluded.standing_sequence,ending_sequence=CASE WHEN server_member_incarnations.membership_id=excluded.membership_id THEN server_member_incarnations.ending_sequence ELSE 0 END,server_name=excluded.server_name,retained_until=excluded.retained_until WHERE excluded.standing_sequence>=server_member_incarnations.standing_sequence AND (excluded.standing_sequence>server_member_incarnations.ending_sequence) AND (excluded.standing_sequence>server_member_incarnations.standing_sequence OR server_member_incarnations.membership_id=excluded.membership_id)`, server, member.AccountID, member.MembershipID, sequence)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n == 0 {
		return err
	}
	// Only an admitted current standing supersedes a notice. A full-list omission
	// never does, and an older full snapshot cannot rewrite this monotonic row.
	_, err = tx.ExecContext(ctx, `UPDATE server_member_endings SET superseded_at=clock_timestamp() WHERE server_id=$1 AND account_id=$2 AND sequence<$3 AND superseded_at IS NULL`, server, member.AccountID, sequence)
	return err
}

// Explicit inactive standing retires an admitted incarnation even when its cause
// is unknown. This stores no notice and never runs for a full-list omission.
func retireMembershipIncarnationTx(ctx context.Context, tx *sql.Tx, server string, member directory.MemberStanding, through int64) error {
	expires := time.Now().Add(30 * 24 * time.Hour)
	if member.Ending != nil && member.Ending.ExpiresAt.Before(expires) {
		expires = member.Ending.ExpiresAt
	}
	_, err := tx.ExecContext(ctx, `UPDATE server_member_incarnations SET standing_sequence=$3,ending_sequence=CASE WHEN ending_sequence=0 THEN $3 ELSE ending_sequence END,retained_until=$5 WHERE server_id=$1 AND account_id=$2 AND standing_sequence<$3 AND ($4='' OR membership_id=$4)`, server, member.AccountID, through, member.MembershipID, expires)
	return err
}

func acceptMembershipEndingTx(ctx context.Context, tx *sql.Tx, server string, member directory.MemberStanding, through int64) (bool, error) {
	event := member.Ending
	if event == nil || !event.ExpiresAt.After(time.Now()) {
		return false, nil
	}
	if member.Active || event.Cause != "owner_removed" || event.Sequence < 1 || event.Sequence > through {
		return false, directory.ErrInvalid
	}
	var incarnation, name string
	var admitted, ended int64
	err := tx.QueryRowContext(ctx, `SELECT membership_id,server_name,standing_sequence,ending_sequence FROM server_member_incarnations WHERE server_id=$1 AND account_id=$2 FOR UPDATE`, server, member.AccountID).Scan(&incarnation, &name, &admitted, &ended)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil // Never manufacture prior membership from this event.
	}
	if err != nil {
		return false, err
	}
	if incarnation != member.MembershipID || event.Sequence <= admitted || (ended > 0 && event.Sequence != ended) {
		return false, nil
	}
	var left bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM server_member_departures WHERE server_id=$1 AND account_id=$2)`, server, member.AccountID).Scan(&left); err != nil || left {
		return false, err // Account-initiated leaving is never this notice.
	}
	var oldSequence int64
	var oldMembership, oldCause string
	var oldEnded, oldExpiry time.Time
	err = tx.QueryRowContext(ctx, `SELECT sequence,membership_id,cause,ended_at,expires_at FROM server_member_endings WHERE server_id=$1 AND account_id=$2`, server, member.AccountID).Scan(&oldSequence, &oldMembership, &oldCause, &oldEnded, &oldExpiry)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil && event.Sequence <= oldSequence {
		if event.Sequence == oldSequence && (oldMembership != member.MembershipID || oldCause != event.Cause || !oldEnded.Equal(event.EndedAt) || !oldExpiry.Equal(event.ExpiresAt)) {
			return false, directory.ErrInvalid
		}
		return false, nil
	}
	if event.Sequence == ended {
		return false, nil // Expired/retired evidence cannot become a new event.
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO server_member_endings(server_id,account_id,membership_id,sequence,cause,server_name,ended_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(server_id,account_id) DO UPDATE SET membership_id=excluded.membership_id,sequence=excluded.sequence,cause=excluded.cause,server_name=excluded.server_name,ended_at=excluded.ended_at,expires_at=excluded.expires_at,superseded_at=NULL WHERE excluded.sequence>server_member_endings.sequence`, server, member.AccountID, member.MembershipID, event.Sequence, event.Cause, name, event.EndedAt, event.ExpiresAt)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE server_member_incarnations SET ending_sequence=$3,retained_until=$4 WHERE server_id=$1 AND account_id=$2`, server, member.AccountID, event.Sequence, event.ExpiresAt)
	return err == nil, err
}

// MembershipEndingNotice projects only this account's explicit evidence and a
// server name captured while it was actually admitted. No fresh hidden server
// metadata, endpoint, route, person or library fact is read for the notice.
func (s *Store) MembershipEndingNotice(ctx context.Context, account string) (*directory.MembershipEndingNotice, error) {
	var notice directory.MembershipEndingNotice
	err := s.DB.QueryRowContext(ctx, `SELECT e.server_id,e.membership_id,e.sequence,e.server_name,e.expires_at,e.cause FROM server_member_endings e JOIN server_member_incarnations i ON i.server_id=e.server_id AND i.account_id=e.account_id AND i.membership_id=e.membership_id AND i.ending_sequence=e.sequence WHERE e.account_id=$1 AND e.superseded_at IS NULL AND e.expires_at>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM server_member_departures d WHERE d.server_id=e.server_id AND d.account_id=e.account_id) AND NOT EXISTS(SELECT 1 FROM server_members m JOIN servers s ON s.id=m.server_id WHERE m.account_id=$1 AND s.revoked_at IS NULL) AND NOT EXISTS(SELECT 1 FROM servers s WHERE s.owner_account_id=$1 AND NOT s.custody_released AND s.revoked_at IS NULL) ORDER BY e.ended_at DESC,e.server_id,e.sequence DESC LIMIT 1`, account).Scan(&notice.ServerID, &notice.MembershipID, &notice.Sequence, &notice.ServerName, &notice.ExpiresAt, &notice.Cause)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &notice, nil
}
