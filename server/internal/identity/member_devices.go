package identity

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"portico.local/server/internal/dbwork"
)

// The owner's view of every member's devices. There is one device record
// (identity_devices): the one a session is bound to and the one every request
// is checked against. This file lists those records across accounts for an
// administrator and lets one be approved or blocked, so "require approval
// before a device can be used" is a single setting over a single mechanism.

// MemberDevice is one device as administration sees it. Trust is the device's
// approval in the words the console uses: pending, approved or blocked.
type MemberDevice struct {
	ID         string `json:"id"`
	AccountID  string `json:"accountId"`
	Username   string `json:"username"`
	ProfileID  string `json:"profileId,omitempty"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	App        string `json:"app"`
	AppVersion string `json:"appVersion"`
	Trust      string `json:"trust"`
	FirstSeen  string `json:"firstSeenAt"`
	LastSeen   string `json:"lastSeenAt"`
	Revision   int64  `json:"revision"`
}

type MemberDevicePage struct {
	Limit      int    `json:"limit"`
	NextCursor string `json:"nextCursor,omitempty"`
	// ApprovalRequired is the owner's policy, so a console renders the list
	// and the rule that governs it from one read.
	ApprovalRequired bool           `json:"approvalRequired"`
	Items            []MemberDevice `json:"items"`
}

type MemberDeviceQuery struct {
	Cursor  string
	Limit   string
	Trust   string
	Account string
}

// MemberDeviceTrust is an administrator's decision about one device.
type MemberDeviceTrust struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
	Trust            string `json:"trust"`
}

// ErrDeviceChanged is a decision made against an older state of the device.
type DeviceChangedError struct{ CurrentRevision int64 }

func (e *DeviceChangedError) Error() string {
	return "This device changed. Refresh before trying again."
}

func trustOf(state string) string {
	if state == "denied" {
		return "blocked"
	}
	return state
}

func stateOf(trust string) (string, bool) {
	switch trust {
	case "approved", "pending":
		return trust, true
	case "blocked":
		return "denied", true
	}
	return "", false
}

const memberDeviceColumns = `d.id,d.account_id,a.username,d.last_profile_id,d.name,d.platform,d.app,d.app_version,d.approval_state,d.first_seen,d.last_seen,d.revision`

func scanMemberDevice(row interface{ Scan(...any) error }) (MemberDevice, error) {
	var d MemberDevice
	var state string
	if e := row.Scan(&d.ID, &d.AccountID, &d.Username, &d.ProfileID, &d.Name, &d.Platform, &d.App, &d.AppVersion, &state, &d.FirstSeen, &d.LastSeen, &d.Revision); e != nil {
		return d, e
	}
	d.Trust = trustOf(state)
	return d, nil
}

// managesDevicesOf is the tier rule for another account's devices: the owner
// manages everyone's; an administrator manages the devices of accounts below
// it, and its own are the owner's to decide.
func managesDevicesOf(actor Principal, account, role string) bool {
	if actor.Role == TierOwner {
		return true
	}
	return ManagesTier(actor.Role, role) && account != actor.AccountID
}

// memberDeviceAuthorityTx checks both the session and its account in the same
// snapshot or transaction that reads or changes another person's devices.
func (s *Service) memberDeviceAuthorityTx(ctx context.Context, tx *sql.Tx, actor Principal) (Principal, error) {
	record, err := s.familyTokenTx(ctx, tx, actor.Hash)
	if err != nil {
		return Principal{}, err
	}
	if record.principal.AccountID != actor.AccountID || record.principal.ProfileID != actor.ProfileID || record.principal.Authority != actor.Authority || record.principal.Epoch != actor.Epoch {
		return Principal{}, ErrUnauthorized
	}
	family, err := s.sessionFamilyRecordTx(ctx, tx, record.principal, record)
	if err != nil {
		return Principal{}, err
	}
	// A tier change is a live permission change, not an ended sign-in. Read
	// the token's current tier so a concurrent demotion answers forbidden.
	actor = record.principal
	actor.DeviceID = family.DeviceID
	if err := actor.PermitsRequest(ctx); err != nil {
		return Principal{}, err
	}
	if actor.Authority != "local" || !Administrative(actor.Role) {
		return Principal{}, ErrForbidden
	}
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM direct_memberships WHERE account_id=? AND disabled=0`, actor.AccountID).Scan(&role); err != nil {
		return Principal{}, err
	}
	if !Administrative(role) {
		return Principal{}, ErrForbidden
	}
	// A narrower session never borrows its account's tier.
	if Grants(actor.Role, role) {
		actor.Role = role
	}
	return actor, nil
}

// MemberDevices lists devices across accounts, newest activity first. The
// caller has already been resolved as an administrator against the live
// membership row; an administrator who is not the owner sees the devices it
// may decide about, which are those of plain members.
func (s *Service) MemberDevices(ctx context.Context, actor Principal, q MemberDeviceQuery) (MemberDevicePage, error) {
	out := MemberDevicePage{Limit: 25, Items: []MemberDevice{}}
	if !Administrative(actor.Role) {
		return out, ErrForbidden
	}
	if q.Limit != "" {
		n, e := strconv.Atoi(q.Limit)
		if e != nil || n < 1 || n > 100 {
			return out, ErrDeviceInput
		}
		out.Limit = n
	}
	state := ""
	if q.Trust != "" {
		var ok bool
		if state, ok = stateOf(q.Trust); !ok {
			return out, ErrDeviceInput
		}
	}
	if q.Account != "" && !validFamilyID(q.Account) {
		return out, ErrDeviceInput
	}
	afterSeen, afterID := "", ""
	if q.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		seen, id, ok := strings.Cut(string(raw), "\x00")
		if e != nil || len(raw) > 256 || !ok || !validFamilyID(id) || len(seen) > 40 {
			return out, ErrDeviceInput
		}
		afterSeen, afterID = seen, id
	}
	gated, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	actor, e = s.memberDeviceAuthorityTx(ctx, tx, actor)
	if e != nil {
		return out, e
	}
	policy, e := deviceApprovalPolicyTx(ctx, tx)
	if e != nil {
		return out, e
	}
	out.ApprovalRequired = policy == DeviceApprovalOwner
	owner := 0
	if actor.Role == TierOwner {
		owner = 1
	}
	rows, e := tx.QueryContext(ctx, `SELECT `+memberDeviceColumns+` FROM identity_devices d JOIN accounts a ON a.id=d.account_id JOIN direct_memberships m ON m.account_id=d.account_id
 WHERE d.authority='local' AND (?='' OR d.approval_state=?) AND (?='' OR d.account_id=?)
 AND (?=1 OR (m.role='member' AND d.account_id<>?))
 AND (?='' OR (d.last_seen,d.id)<(?,?)) ORDER BY d.last_seen DESC,d.id DESC LIMIT ?`,
		state, state, q.Account, q.Account, owner, actor.AccountID, afterSeen, afterSeen, afterID, out.Limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		d, e := scanMemberDevice(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, d)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > out.Limit {
		out.Items = out.Items[:out.Limit]
		last := out.Items[out.Limit-1]
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.LastSeen + "\x00" + last.ID))
	}
	return out, nil
}

// MemberDevice reads one device as the list shows it, so a console can read
// again the record it is deciding about without walking the list. A device the
// caller may not decide about is not found, as it is absent from the list.
func (s *Service) MemberDevice(ctx context.Context, actor Principal, id string) (MemberDevice, error) {
	if !Administrative(actor.Role) {
		return MemberDevice{}, ErrForbidden
	}
	if !validFamilyID(id) {
		return MemberDevice{}, ErrDeviceInput
	}
	gated, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return MemberDevice{}, e
	}
	defer gated.Rollback()
	tx := gated.Tx()
	if actor, e = s.memberDeviceAuthorityTx(ctx, tx, actor); e != nil {
		return MemberDevice{}, e
	}
	var role string
	row := tx.QueryRowContext(ctx, `SELECT `+memberDeviceColumns+`,m.role FROM identity_devices d JOIN accounts a ON a.id=d.account_id JOIN direct_memberships m ON m.account_id=d.account_id WHERE d.authority='local' AND d.id=?`, id)
	var d MemberDevice
	var state string
	e = row.Scan(&d.ID, &d.AccountID, &d.Username, &d.ProfileID, &d.Name, &d.Platform, &d.App, &d.AppVersion, &state, &d.FirstSeen, &d.LastSeen, &d.Revision, &role)
	if errors.Is(e, sql.ErrNoRows) {
		return MemberDevice{}, ErrDeviceUnknown
	}
	if e != nil {
		return MemberDevice{}, e
	}
	if !managesDevicesOf(actor, d.AccountID, role) {
		return MemberDevice{}, ErrDeviceUnknown
	}
	d.Trust = trustOf(state)
	return d, nil
}

// SetMemberDeviceTrust approves a device, blocks it, or puts it back to
// waiting. Blocking ends what the device is doing now, not only what it does
// next: its sessions are revoked through the family hook, so playback it had
// open is retired with them. Repeating a decision that already holds is not an
// error, so a retried request gets the same answer.
func (s *Service) SetMemberDeviceTrust(ctx context.Context, actor Principal, id string, c MemberDeviceTrust) (MemberDevice, error) {
	state, ok := stateOf(c.Trust)
	if !ok || !validFamilyID(id) || c.ExpectedRevision < 1 {
		return MemberDevice{}, ErrDeviceInput
	}
	if !Administrative(actor.Role) {
		return MemberDevice{}, ErrForbidden
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return MemberDevice{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	actor, e = s.memberDeviceAuthorityTx(ctx, tx, actor)
	if e != nil {
		return MemberDevice{}, e
	}
	read := func() (MemberDevice, string, error) {
		var role string
		row := tx.QueryRowContext(ctx, `SELECT `+memberDeviceColumns+`,m.role FROM identity_devices d JOIN accounts a ON a.id=d.account_id JOIN direct_memberships m ON m.account_id=d.account_id WHERE d.authority='local' AND d.id=?`, id)
		var d MemberDevice
		var current string
		e := row.Scan(&d.ID, &d.AccountID, &d.Username, &d.ProfileID, &d.Name, &d.Platform, &d.App, &d.AppVersion, &current, &d.FirstSeen, &d.LastSeen, &d.Revision, &role)
		d.Trust = trustOf(current)
		return d, role, e
	}
	device, role, e := read()
	if errors.Is(e, sql.ErrNoRows) {
		return MemberDevice{}, ErrDeviceUnknown
	}
	if e != nil {
		return MemberDevice{}, e
	}
	if !managesDevicesOf(actor, device.AccountID, role) {
		return MemberDevice{}, ErrForbidden
	}
	if device.Trust == c.Trust {
		return device, nil
	}
	if device.Revision != c.ExpectedRevision {
		return MemberDevice{}, &DeviceChangedError{device.Revision}
	}
	if e = s.setDeviceApprovalTx(ctx, tx, deviceCaller{authority: "local", account: device.AccountID}, id, state); e != nil {
		return MemberDevice{}, e
	}
	if device, _, e = read(); e != nil {
		return MemberDevice{}, e
	}
	return device, gated.Commit()
}

// setDeviceApprovalTx is the one write of a device's approval. A device that
// is no longer approved loses its sessions in the same transaction.
func (s *Service) setDeviceApprovalTx(ctx context.Context, tx *sql.Tx, owner deviceCaller, id, state string) error {
	if _, e := tx.ExecContext(ctx, `UPDATE identity_devices SET approval_state=?,revision=revision+1 WHERE id=? AND authority=? AND account_id=? AND approval_state<>?`, state, id, owner.authority, owner.account, state); e != nil {
		return e
	}
	if state == "approved" {
		return nil
	}
	return s.revokeDeviceFamiliesTx(ctx, tx, owner, id, RevokedDeviceDisapproved)
}
