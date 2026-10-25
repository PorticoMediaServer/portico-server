package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"portico.local/hosted/internal/directory"
)

// The discovery index (migration 0017). Each user server decides who its
// members are and pushes changes here; Hosted never decides access and never
// asks. A push is applied only on top of the sequence this server last applied,
// an exact replay is answered as done, and anything else is answered
// applied=false so the server sends its full list.

// bumpServerListVersionsTx gives each account a new server-list version from
// one global sequence. The statement trigger announces it on
// ServerListChannel at commit. Values can commit out of order across accounts
// (there is deliberately no global lock: it would serialise every membership
// change and could deadlock against account row locks), so the version map
// re-reads a trailing window rather than only "greater than the last seen"
// (directory.RefreshServerListVersions). For one account, updates of its row
// are serialised by the row lock and re-evaluate nextval, so its own versions
// always rise in commit order.
func bumpServerListVersionsTx(ctx context.Context, tx *sql.Tx, accounts []string) error {
	if len(accounts) == 0 {
		return nil
	}
	sort.Strings(accounts)
	_, e := tx.ExecContext(ctx, `UPDATE accounts SET server_list_version=nextval('server_list_versions') WHERE id=ANY($1)`, accounts)
	return e
}

// pushDigest identifies a push's content, so a replayed sequence is taken as
// done only when it is the same push (INT M12: a server restored from a backup
// reuses sequence numbers for different changes).
func pushDigest(push directory.MemberPush) []byte {
	type wire struct {
		AccountID, Role string
		Active          bool
		MembershipID    string                      `json:",omitempty"`
		Ending          *directory.MembershipEnding `json:",omitempty"`
	}
	lines := make([]wire, len(push.Members))
	for i, m := range push.Members {
		lines[i] = wire{m.AccountID, m.Role, m.Active, m.MembershipID, m.Ending}
	}
	raw, _ := json.Marshal(struct {
		After, Through int64
		Full           bool
		Members        []wire
	}{push.After, push.Through, push.Full, lines})
	sum := sha256.Sum256(raw)
	return sum[:]
}

// PushMembers applies one membership push (or one page of a full list) from
// the server that authority names. The server decides membership; Hosted only
// lists a server for an account that consented to it: its current custodian,
// an account already listed, or one whose own assertion for this server the
// push carries, issued within ConsentLifetime and after any departure it
// recorded (INT M6). Custody moves only to an owner whose own custody
// assertion the push carries, issued within CustodyConsentLifetime; when the
// custodian is no longer an owner and custody cannot follow, it is released
// (INT M10).
func (s *Store) PushMembers(ctx context.Context, authority directory.ServerAuthority, push directory.MemberPush) (directory.MemberPushResult, error) {
	server := authority.ServerID
	candidate := custodyCandidate(authority.AccountID, push.Members)
	var also []string
	if candidate != "" {
		also = []string{candidate}
	}
	tx, e := s.beginServerOperationWithSigningState(ctx, authority, true, also...)
	if e != nil {
		return directory.MemberPushResult{}, e
	}
	defer tx.Rollback()
	// A replica's loaded certificate may predate the current rollout. Recheck
	// verified consent against terminal revocations in this transaction, while
	// the shared state lock prevents a rollout crossing the decision.
	rows, e := tx.QueryContext(ctx, `SELECT key_id FROM hosted_signing_revocations`)
	if e != nil {
		return directory.MemberPushResult{}, e
	}
	revoked := map[string]bool{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return directory.MemberPushResult{}, e
		}
		revoked[id] = true
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return directory.MemberPushResult{}, e
	}
	for i := range push.Members {
		if revoked[push.Members[i].ConsentSigner] {
			push.Members[i].ConsentAt = time.Time{}
			push.Members[i].ConsentKind = ""
		}
	}
	// Do not discover a different candidate after account locks were taken.
	// An originally ambiguous owner list remains ambiguous; filtering may
	// invalidate the chosen candidate, never select an unlocked account.
	if candidate != custodyCandidate(authority.AccountID, push.Members) {
		candidate = ""
	}
	var stored int64
	var storedDigest []byte
	var fullThrough sql.NullInt64
	var fullPage sql.NullInt32
	if _, e = tx.ExecContext(ctx, `INSERT INTO server_member_sync(server_id) VALUES($1) ON CONFLICT DO NOTHING`, server); e != nil {
		return directory.MemberPushResult{}, e
	}
	if e = tx.QueryRowContext(ctx, `SELECT sequence,push_digest,full_through,full_page FROM server_member_sync WHERE server_id=$1 FOR UPDATE`, server).Scan(&stored, &storedDigest, &fullThrough, &fullPage); e != nil {
		return directory.MemberPushResult{}, e
	}
	digest := pushDigest(push)
	refuse := func() (directory.MemberPushResult, error) {
		if _, e := tx.ExecContext(ctx, `UPDATE server_member_sync SET full_through=NULL,full_page=NULL WHERE server_id=$1`, server); e != nil {
			return directory.MemberPushResult{}, e
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM server_member_full_prior WHERE server_id=$1`, server); e != nil {
			return directory.MemberPushResult{}, e
		}
		return directory.MemberPushResult{Sequence: stored}, tx.Commit()
	}
	switch {
	case !push.Full && push.Through == stored && push.After <= stored && bytes.Equal(digest, storedDigest):
		// The answer to this push was lost and the server sent it again.
		holder, e := custodianTx(ctx, tx, server)
		if e != nil {
			return directory.MemberPushResult{}, e
		}
		return directory.MemberPushResult{Sequence: stored, Applied: true, Custodian: holder}, tx.Commit()
	case !push.Full && push.After != stored:
		return refuse()
	case push.Full && push.Page > 0 && (!fullPage.Valid || int(fullPage.Int32) != push.Page || !fullThrough.Valid || fullThrough.Int64 != push.Through):
		// A page out of order: start the full list again.
		return refuse()
	}
	if push.Full && push.Page == 0 {
		// A full list replaces the index. What it does not name by its last
		// page is removed then; until then the index keeps serving.
		if _, e = tx.ExecContext(ctx, `DELETE FROM server_member_full_prior WHERE server_id=$1`, server); e != nil {
			return directory.MemberPushResult{}, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO server_member_full_prior(server_id,account_id,role) SELECT server_id,account_id,role FROM server_members WHERE server_id=$1`, server); e != nil {
			return directory.MemberPushResult{}, e
		}
	}
	var custodian sql.NullString
	if e = tx.QueryRowContext(ctx, `SELECT owner_account_id FROM servers WHERE id=$1`, server).Scan(&custodian); e != nil {
		return directory.MemberPushResult{}, e
	}
	var changed, skipped []string
	for _, m := range push.Members {
		if push.Full {
			if _, e = tx.ExecContext(ctx, `DELETE FROM server_member_full_prior WHERE server_id=$1 AND account_id=$2`, server, m.AccountID); e != nil {
				return directory.MemberPushResult{}, e
			}
		}
		var role sql.NullString
		if e = tx.QueryRowContext(ctx, `SELECT role FROM server_members WHERE server_id=$1 AND account_id=$2`, server, m.AccountID).Scan(&role); e != nil && !errors.Is(e, sql.ErrNoRows) {
			return directory.MemberPushResult{}, e
		}
		retired, e := membershipStandingRetiredTx(ctx, tx, server, m, push.Through)
		if e != nil {
			return directory.MemberPushResult{}, e
		}
		if retired {
			skipped = append(skipped, m.AccountID)
			continue
		}
		if !m.Active {
			newEnding, e := acceptMembershipEndingTx(ctx, tx, server, m, push.Through)
			if e != nil {
				return directory.MemberPushResult{}, e
			}
			if newEnding {
				changed = append(changed, m.AccountID)
			}
			if e = retireMembershipIncarnationTx(ctx, tx, server, m, push.Through); e != nil {
				return directory.MemberPushResult{}, e
			}
			if role.Valid {
				if _, e = tx.ExecContext(ctx, `DELETE FROM server_members WHERE server_id=$1 AND account_id=$2`, server, m.AccountID); e != nil {
					return directory.MemberPushResult{}, e
				}
				changed = append(changed, m.AccountID)
			}
			// The server has ended a departure it was told about.
			if _, e = tx.ExecContext(ctx, `UPDATE server_member_departures SET pending=false WHERE server_id=$1 AND account_id=$2`, server, m.AccountID); e != nil {
				return directory.MemberPushResult{}, e
			}
			continue
		}
		if role.Valid && role.String == m.Role {
			if e = admitMembershipIncarnationTx(ctx, tx, server, m, push.Through); e != nil {
				return directory.MemberPushResult{}, e
			}
			continue
		}
		if !role.Valid && m.AccountID != custodian.String {
			var left sql.NullTime
			if e = tx.QueryRowContext(ctx, `SELECT left_at FROM server_member_departures WHERE server_id=$1 AND account_id=$2`, server, m.AccountID).Scan(&left); e != nil && !errors.Is(e, sql.ErrNoRows) {
				return directory.MemberPushResult{}, e
			}
			if m.ConsentAt.IsZero() || time.Since(m.ConsentAt) > directory.ConsentLifetime || (left.Valid && !m.ConsentAt.After(left.Time)) {
				skipped = append(skipped, m.AccountID)
				continue
			}
		}
		// An account Hosted does not know (deleted, or never a Portico Account)
		// is simply not listed.
		result, e := tx.ExecContext(ctx, `INSERT INTO server_members(server_id,account_id,role) SELECT $1,id,$3 FROM accounts WHERE id=$2 ON CONFLICT(server_id,account_id) DO UPDATE SET role=EXCLUDED.role,updated_at=clock_timestamp()`, server, m.AccountID, m.Role)
		if e != nil {
			return directory.MemberPushResult{}, e
		}
		if n, _ := result.RowsAffected(); n == 0 {
			skipped = append(skipped, m.AccountID)
			continue
		}
		if e = admitMembershipIncarnationTx(ctx, tx, server, m, push.Through); e != nil {
			return directory.MemberPushResult{}, e
		}
		changed = append(changed, m.AccountID)
		if _, e = tx.ExecContext(ctx, `DELETE FROM server_member_departures WHERE server_id=$1 AND account_id=$2`, server, m.AccountID); e != nil {
			return directory.MemberPushResult{}, e
		}
	}
	if push.Full && push.More {
		if _, e = tx.ExecContext(ctx, `UPDATE server_member_sync SET full_through=$2,full_page=$3,updated_at=clock_timestamp() WHERE server_id=$1`, server, push.Through, push.Page+1); e != nil {
			return directory.MemberPushResult{}, e
		}
		if e = bumpServerListVersionsTx(ctx, tx, changed); e != nil {
			return directory.MemberPushResult{}, e
		}
		holder, e := custodianTx(ctx, tx, server)
		if e != nil {
			return directory.MemberPushResult{}, e
		}
		return directory.MemberPushResult{Sequence: stored, Applied: true, Skipped: skipped, Custodian: holder}, tx.Commit()
	}
	if push.Full {
		if _, e = tx.ExecContext(ctx, `UPDATE server_member_incarnations SET retained_until=clock_timestamp()+interval '720 hours' WHERE server_id=$1 AND account_id IN(SELECT account_id FROM server_member_full_prior WHERE server_id=$1)`, server); e != nil {
			return directory.MemberPushResult{}, e
		}
		rows, e := tx.QueryContext(ctx, `DELETE FROM server_members WHERE server_id=$1 AND account_id IN(SELECT account_id FROM server_member_full_prior WHERE server_id=$1) RETURNING account_id`, server)
		if e != nil {
			return directory.MemberPushResult{}, e
		}
		for rows.Next() {
			var account string
			if e = rows.Scan(&account); e != nil {
				rows.Close()
				return directory.MemberPushResult{}, e
			}
			changed = append(changed, account)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return directory.MemberPushResult{}, e
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM server_member_full_prior WHERE server_id=$1`, server); e != nil {
			return directory.MemberPushResult{}, e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE server_member_departures SET pending=false WHERE server_id=$1 AND NOT EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=$1 AND m.account_id=server_member_departures.account_id)`, server); e != nil {
			return directory.MemberPushResult{}, e
		}
	}
	followed, e := followServerOwnerTx(ctx, tx, authority, candidate)
	if e != nil {
		return directory.MemberPushResult{}, e
	}
	moved, e := settleCustodyTx(ctx, tx, server)
	if e != nil {
		return directory.MemberPushResult{}, e
	}
	if e = bumpServerListVersionsTx(ctx, tx, append(append(changed, followed...), moved...)); e != nil {
		return directory.MemberPushResult{}, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE server_member_sync SET sequence=$2,push_digest=$3,full_through=NULL,full_page=NULL,updated_at=clock_timestamp() WHERE server_id=$1`, server, push.Through, digest); e != nil {
		return directory.MemberPushResult{}, e
	}
	holder, e := custodianTx(ctx, tx, server)
	if e != nil {
		return directory.MemberPushResult{}, e
	}
	return directory.MemberPushResult{Sequence: push.Through, Applied: true, Skipped: skipped, Custodian: holder}, tx.Commit()
}

// custodyCandidate is the one owner in a push, other than the custodian, whose
// own fresh custody assertion the push carries: the only account custody may
// move to (INT M6). Two such owners name none.
func custodyCandidate(custodian string, members []directory.MemberStanding) string {
	found := ""
	for _, m := range members {
		if !m.Active || m.Role != "owner" || m.AccountID == custodian || m.ConsentKind != directory.CustodyKind || m.ConsentAt.IsZero() || time.Since(m.ConsentAt) > directory.CustodyConsentLifetime {
			continue
		}
		if found != "" {
			return ""
		}
		found = m.AccountID
	}
	return found
}

// settleCustodyTx records whether custody is released: the server's list does
// not name its custodian as owner. It returns the custodian when that changed,
// whose server list then shows the server differently.
func settleCustodyTx(ctx context.Context, tx *sql.Tx, server string) ([]string, error) {
	var owner string
	e := tx.QueryRowContext(ctx, `UPDATE servers s SET custody_released=NOT EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.account_id=s.owner_account_id AND m.role='owner')
 WHERE s.id=$1 AND s.revoked_at IS NULL AND s.custody_released IS DISTINCT FROM NOT EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.account_id=s.owner_account_id AND m.role='owner') RETURNING s.owner_account_id`, server).Scan(&owner)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return []string{owner}, nil
}

// custodianTx is the account holding custody of the server, or "" while it is
// released.
func custodianTx(ctx context.Context, tx *sql.Tx, server string) (string, error) {
	var holder string
	e := tx.QueryRowContext(ctx, `SELECT CASE WHEN custody_released OR owner_account_id IS NULL THEN '' ELSE owner_account_id END FROM servers WHERE id=$1`, server).Scan(&holder)
	return holder, e
}

// memberIndexTx reads what the check-in compares: the sequence this server
// last applied and the digest of its indexed members.
func memberIndexTx(ctx context.Context, tx *sql.Tx, server string) (int64, string, error) {
	var sequence int64
	e := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT sequence FROM server_member_sync WHERE server_id=$1),0)`, server).Scan(&sequence)
	if e != nil {
		return 0, "", e
	}
	rows, e := tx.QueryContext(ctx, `SELECT account_id,role FROM server_members WHERE server_id=$1`, server)
	if e != nil {
		return 0, "", e
	}
	defer rows.Close()
	var members []directory.MemberStanding
	for rows.Next() {
		m := directory.MemberStanding{Active: true}
		if e = rows.Scan(&m.AccountID, &m.Role); e != nil {
			return 0, "", e
		}
		members = append(members, m)
	}
	if e = rows.Err(); e != nil {
		return 0, "", e
	}
	return sequence, directory.MemberDigest(members), nil
}

// ServerListVersionsAfter streams accounts whose server-list version is above
// after, in version order, at most limit of them. It returns the last version
// read and how many rows there were.
func (s *Store) ServerListVersionsAfter(ctx context.Context, after int64, limit int, fn func(account string, version int64)) (int64, int, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,server_list_version FROM accounts WHERE server_list_version>$1 ORDER BY server_list_version LIMIT $2`, after, limit)
	if e != nil {
		return after, 0, e
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var account string
		var version int64
		if e = rows.Scan(&account, &version); e != nil {
			return after, n, e
		}
		fn(account, version)
		after = version
		n++
	}
	return after, n, rows.Err()
}

// dropServerIndexTx removes a retired server from the discovery index and
// moves its members' (and the former custodian's) list versions, so their
// devices drop it at their next check.
func dropServerIndexTx(ctx context.Context, tx *sql.Tx, server string, custodian string) error {
	rows, e := tx.QueryContext(ctx, `DELETE FROM server_members WHERE server_id=$1 RETURNING account_id`, server)
	if e != nil {
		return e
	}
	var accounts []string
	if custodian != "" {
		accounts = append(accounts, custodian)
	}
	for rows.Next() {
		var account string
		if e = rows.Scan(&account); e != nil {
			rows.Close()
			return e
		}
		if account != custodian {
			accounts = append(accounts, account)
		}
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM server_member_sync WHERE server_id=$1`, server); e != nil {
		return e
	}
	return bumpServerListVersionsTx(ctx, tx, accounts)
}

// followServerOwnerTx moves the claim's custody to the account the server now
// reports as its owner, when that account accepted custody (candidate, see
// custodyCandidate). Custody is what retiring the server, its certificates and
// its endpoint follow; the installation identity, the credential and the
// original claim proof audience never move.
//
// It acts only when the index holds exactly one owner and that is the
// candidate. An owner that is not a Portico Account (a direct account) is not
// in the index, and one that has not accepted custody is no candidate: custody
// is then released (settleCustodyTx) until an owner accepts it.
//
// The candidate's accounts row was locked with the custodian's, in id order,
// when the operation began (lockServerAuthorityOwnerTx, INT N5). It returns
// the former and the new custodian when custody moved: both lists change.
func followServerOwnerTx(ctx context.Context, tx *sql.Tx, authority directory.ServerAuthority, candidate string) ([]string, error) {
	server := authority.ServerID
	if candidate == "" {
		return nil, nil
	}
	rows, e := tx.QueryContext(ctx, `SELECT account_id FROM server_members WHERE server_id=$1 AND role='owner' LIMIT 2`, server)
	if e != nil {
		return nil, e
	}
	var owners []string
	for rows.Next() {
		var account string
		if e = rows.Scan(&account); e != nil {
			rows.Close()
			return nil, e
		}
		owners = append(owners, account)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(owners) != 1 || owners[0] != candidate {
		return nil, nil
	}
	target := candidate
	// Do not move an in-flight certificate custody lease: the push fails as
	// busy and the server's retry (with backoff) carries the change again.
	var busy bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM certificate_orders WHERE operation_id=$1 AND lease_until>clock_timestamp())`, authority.OperationID).Scan(&busy); e != nil {
		return nil, e
	}
	if busy {
		return nil, ErrMembershipBusy
	}
	for _, q := range []string{
		`DELETE FROM claim_approvals WHERE operation_id=$1 AND $2<>'' `,
		`UPDATE claim_operations SET account_id=$2 WHERE id=$1 AND state='installed'`,
		`UPDATE current_server_endpoints SET account_id=$2 WHERE operation_id=$1 `,
		`UPDATE certificate_orders SET account_id=$2 WHERE operation_id=$1 `,
	} {
		if _, e = tx.ExecContext(ctx, q, authority.OperationID, target); e != nil {
			return nil, e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE servers SET owner_account_id=$2,custody_released=false WHERE id=$1 AND owner_account_id=$3`, server, target, authority.AccountID); e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO server_ownership_transfers(server_id,operation_id,claim_operation_id,previous_account_id,new_account_id,policy_revision) SELECT $1,'push:'||gen_random_uuid()::text,$2,$3,$4,policy_revision FROM servers WHERE id=$1`, server, authority.OperationID, authority.AccountID, target); e != nil {
		return nil, e
	}
	return []string{authority.AccountID, target}, audit(ctx, tx, authority.AccountID, "server.ownership.transferred", server, 0)
}

// LeaveServer removes a server from an account's own list and tells the
// server, which ends the membership at its next check-in (it is woken now).
// The custodian cannot leave while it holds custody; it transfers ownership
// (the new owner accepts custody) or retires the server. Once custody is
// released it leaves like anyone else (INT M10).
func (s *Store) LeaveServer(ctx context.Context, account, server string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var custodian sql.NullString
	var revoked sql.NullTime
	var released bool
	e = tx.QueryRowContext(ctx, `SELECT owner_account_id,revoked_at,custody_released FROM servers WHERE id=$1 FOR UPDATE`, server).Scan(&custodian, &revoked, &released)
	if errors.Is(e, sql.ErrNoRows) {
		return directory.ErrDenied
	}
	if e != nil {
		return e
	}
	if custodian.String == account && !released {
		return directory.ErrConflict
	}
	result, e := tx.ExecContext(ctx, `DELETE FROM server_members WHERE server_id=$1 AND account_id=$2`, server, account)
	if e != nil {
		return e
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return directory.ErrDenied
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO server_member_departures(server_id,account_id) VALUES($1,$2) ON CONFLICT(server_id,account_id) DO UPDATE SET left_at=clock_timestamp(),pending=true`, server, account); e != nil {
		return e
	}
	if e = bumpServerListVersionsTx(ctx, tx, []string{account}); e != nil {
		return e
	}
	if !revoked.Valid {
		var wake int64
		if e = tx.QueryRowContext(ctx, `UPDATE servers SET policy_revision=CASE WHEN policy_revision<9223372036854775807 THEN policy_revision+1 ELSE policy_revision END WHERE id=$1 RETURNING policy_revision`, server).Scan(&wake); e != nil {
			return e
		}
		if e = enqueueWake(ctx, tx, server, wake); e != nil {
			return e
		}
	}
	if e = audit(ctx, tx, account, "server.left", server, 0); e != nil {
		return e
	}
	return tx.Commit()
}

// custodyReleasedTx reports a server whose custody is released: its own
// member list no longer names its custodian as owner, and custody could not
// follow (a direct owner, or a Portico owner that has not accepted it).
// Retiring the claim waits for an owner who accepts custody (INT M10).
func custodyReleasedTx(ctx context.Context, tx *sql.Tx, server string) (bool, error) {
	var released bool
	e := tx.QueryRowContext(ctx, `SELECT custody_released FROM servers WHERE id=$1`, server).Scan(&released)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	return released, e
}

// serverAudienceTx is everyone whose server list shows this server: its
// custodian and the accounts listed for it.
func serverAudienceTx(ctx context.Context, tx *sql.Tx, server string) ([]string, error) {
	rows, e := tx.QueryContext(ctx, `SELECT owner_account_id FROM servers WHERE id=$1 AND owner_account_id IS NOT NULL AND NOT custody_released UNION SELECT account_id FROM server_members WHERE server_id=$1`, server)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var account string
		if e = rows.Scan(&account); e != nil {
			return nil, e
		}
		out = append(out, account)
	}
	return out, rows.Err()
}

// touchServerListTx moves the list version of everyone who lists this server,
// for a change to what the list shows about it: a rename or a new endpoint
// (INT M11). These are rare, and each costs its members one list fetch.
func touchServerListTx(ctx context.Context, tx *sql.Tx, server string) error {
	accounts, e := serverAudienceTx(ctx, tx, server)
	if e != nil {
		return e
	}
	return bumpServerListVersionsTx(ctx, tx, accounts)
}
