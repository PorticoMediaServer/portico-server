package access

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/testauth"
)

// catalogueIDs are the public ids (what the API takes) and integer ids (what
// the tables hold) of the two catalogue titles the admission tests use.
type catalogueIDs struct {
	adult, family     string
	adultID, familyID int64
	asset             string
}

func fixture(t *testing.T) (*Store, *sql.DB, catalogueIDs) {
	t.Helper()
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`INSERT INTO accounts VALUES('owner','owner',x'00','owner-profile',1);
 INSERT INTO accounts VALUES('member','member',x'00','member-profile',1);
 INSERT INTO libraries(id,name,kind,root) VALUES('lib','Library','movie','/library');`); err != nil {
		t.Fatal(err)
	}
	return New(db), db, seedCatalogue(t, db)
}

// seedCatalogue creates the two movies the admission tests decide on, with
// the write API: an above-ceiling title and a family title carrying a denied
// label, plus the asset playback sessions point at.
func seedCatalogue(t *testing.T, db *sql.DB) catalogueIDs {
	t.Helper()
	ctx := context.Background()
	var ids catalogueIDs
	err := dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		handle, e := compactcatalog.LibraryTx(ctx, tx, "lib")
		if e != nil {
			return e
		}
		seed := func(key, title string) (int64, error) {
			id, _, e := compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{
				Library: handle, Kind: compactcatalog.Movie, Key: key, Title: title,
			})
			return id, e
		}
		if ids.adultID, e = seed("access-test:adult", "Adult"); e != nil {
			return e
		}
		if ids.familyID, e = seed("access-test:family", "Family"); e != nil {
			return e
		}
		if e = compactcatalog.SetAttributesTx(ctx, tx, ids.adultID, "contentRating", []string{"TV-MA"}); e != nil {
			return e
		}
		if e = compactcatalog.SetAttributesTx(ctx, tx, ids.familyID, "contentRating", []string{"PG"}); e != nil {
			return e
		}
		if e = compactcatalog.SetAttributesTx(ctx, tx, ids.familyID, "label", []string{"Spoilers"}); e != nil {
			return e
		}
		var asset int64
		if asset, ids.asset, e = compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{
			Path: "/a", Size: 1, ModifiedNS: 1,
			Container: "mkv", VideoCodec: "h264", AudioCodec: "aac",
			Width: 1920, Height: 1080, Duration: 1.0,
		}); e != nil {
			return e
		}
		_ = asset
		if ids.adult, e = entityid.Public(ctx, tx, ids.adultID); e != nil {
			return e
		}
		ids.family, e = entityid.Public(ctx, tx, ids.familyID)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func owner() identity.Principal {
	return identity.Principal{Viewer: identity.Viewer{AccountID: "owner", ProfileID: "owner-profile", Authority: "local", Role: identity.TierOwner}}
}

// The tier predicate is the whole authorization rule for this area, so it is
// asserted directly rather than only through the routes that call it.
func TestTierPredicateOrdersOwnerAdminMemberAndRefusesUnknown(t *testing.T) {
	for _, c := range []struct {
		role, needed string
		want         bool
	}{
		{identity.TierOwner, identity.TierAdmin, true},
		{identity.TierAdmin, identity.TierAdmin, true},
		{identity.TierMember, identity.TierAdmin, false},
		{identity.TierAdmin, identity.TierOwner, false},
		{"root", identity.TierMember, false},
		{"", identity.TierMember, false},
	} {
		if got := identity.Grants(c.role, c.needed); got != c.want {
			t.Fatalf("Grants(%q,%q)=%v", c.role, c.needed, got)
		}
	}
	// An admin manages strictly below itself; the owner manages everyone but
	// the owner tier, which only an ownership transfer moves.
	for _, c := range []struct {
		actor, target string
		want          bool
	}{
		{identity.TierAdmin, identity.TierMember, true},
		{identity.TierAdmin, identity.TierAdmin, false},
		{identity.TierAdmin, identity.TierOwner, false},
		{identity.TierOwner, identity.TierAdmin, true},
		{identity.TierOwner, identity.TierOwner, false},
		{identity.TierMember, identity.TierMember, false},
	} {
		if got := identity.ManagesTier(c.actor, c.target); got != c.want {
			t.Fatalf("ManagesTier(%q,%q)=%v", c.actor, c.target, got)
		}
	}
}

func TestRoleChangeRequiresRevisionRevokesSessionsAndRefusesPeers(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	testauth.InsertSession(t, db, "hash", "member", "member-profile", "local", "member", "2099-01-01T00:00:00Z")
	if _, err := store.SetRole(ctx, nil, owner(), "member", RoleChange{ExpectedRevision: 9, OperationID: "operation-1", Role: identity.TierAdmin}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	out, err := store.SetRole(ctx, nil, owner(), "member", RoleChange{ExpectedRevision: 1, OperationID: "operation-1", Role: identity.TierAdmin})
	if err != nil || out.Role != identity.TierAdmin || out.Revision != 2 {
		t.Fatalf("promote: %v %+v", err, out)
	}
	var revoked int
	if err = db.QueryRow(`SELECT revoked FROM authorization_access WHERE hash='hash'`).Scan(&revoked); err != nil || revoked != 1 {
		t.Fatalf("sessions survived a tier change: %v %d", err, revoked)
	}
	// A replay of the same operation returns the same document rather than
	// applying a second time.
	replay, err := store.SetRole(ctx, nil, owner(), "member", RoleChange{ExpectedRevision: 1, OperationID: "operation-1", Role: identity.TierAdmin})
	if err != nil || replay.Revision != 2 {
		t.Fatalf("replay: %v %+v", err, replay)
	}
	if _, err = store.SetRole(ctx, nil, owner(), "member", RoleChange{ExpectedRevision: 1, OperationID: "operation-1", Role: identity.TierMember}); !errors.Is(err, ErrIdempotencyKeyReused) {
		t.Fatalf("reused key: %v", err)
	}
	admin := identity.Principal{Viewer: identity.Viewer{AccountID: "admin-actor", Role: identity.TierAdmin}}
	if _, err = store.SetRole(ctx, nil, admin, "member", RoleChange{ExpectedRevision: 2, OperationID: "operation-2", Role: identity.TierMember}); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("an admin edited a peer: %v", err)
	}
}

func TestLimitsDefaultToUnrestrictedAndRoundTrip(t *testing.T) {
	store, _, _ := fixture(t)
	ctx := context.Background()
	first, err := store.Limits(ctx, nil, "member")
	if err != nil || first.Revision != 1 || first.Limits.MaxStreams != 0 || !first.Limits.AllowUnrated {
		t.Fatalf("default limits: %v %+v", err, first)
	}
	change := LimitsChange{ExpectedRevision: 1, OperationID: "limits-write-1", Limits: Limits{MaxStreams: 1, RemoteBitrateKbps: 4000,
		MaxContentRating: "pg-13", Schedule: AccessSchedule{Timezone: "UTC", Windows: []ScheduleWindow{{Days: []int{3, 1}, StartMinute: 60, EndMinute: 120}}},
		Channels: ChannelPolicy{Mode: "deny", Channels: []string{"library:news"}}, Tags: TagPolicy{DeniedLabels: []string{" Spoilers "}}}}
	out, err := store.SetLimits(ctx, nil, "member", change)
	if err != nil || out.Revision != 2 {
		t.Fatalf("write: %v %+v", err, out)
	}
	if out.Limits.MaxContentRating != "PG-13" || out.Limits.Tags.DeniedLabels[0] != "Spoilers" || out.Limits.Schedule.Windows[0].Days[0] != 1 {
		t.Fatalf("values were not normalised: %+v", out.Limits)
	}
	if _, err = store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: 1, OperationID: "limits-write-2", Limits: DefaultLimits()}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write was accepted: %v", err)
	}
	bad := LimitsChange{ExpectedRevision: 2, OperationID: "limits-write-3", Limits: Limits{MaxContentRating: "XX", Schedule: AccessSchedule{Timezone: "Mars/Olympus"}}}
	var fields *ValidationError
	if _, err = store.SetLimits(ctx, nil, "member", bad); !errors.As(err, &fields) || len(fields.Fields) != 2 {
		t.Fatalf("validation: %v", err)
	}
}

func TestChannelPolicyAcceptsOnlyCanonicalV1IDs(t *testing.T) {
	store, _, _ := fixture(t)
	ctx := context.Background()
	for i, id := range []string{"library:news", "library:channel-1", "live:source:ch", "live:src-1:ch_2"} {
		current, err := store.Limits(ctx, nil, "member")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: current.Revision, OperationID: "canon-allow-000" + string(rune('0'+i)) + "-" + id, Limits: Limits{Channels: ChannelPolicy{Mode: "deny", Channels: []string{id}}}}); err != nil {
			t.Fatalf("canonical %q was rejected: %v", id, err)
		}
		// Reset for the next id (each write bumps the revision).
		current, err = store.Limits(ctx, nil, "member")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: current.Revision, OperationID: "canon-reset-000" + string(rune('0'+i)) + "-" + id, Limits: DefaultLimits()}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"news", "sports", "live:", "live:onlyone", "live::ch", "live:src:", "live:a:b:c", "library:", "library:a:b", "Live:src:ch", "LIBRARY:ch"} {
		current, err := store.Limits(ctx, nil, "member")
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: current.Revision, OperationID: "canon-reject-" + id + "-000000", Limits: Limits{Channels: ChannelPolicy{Mode: "deny", Channels: []string{id}}}})
		var fields *ValidationError
		if !errors.As(err, &fields) || len(fields.Fields) != 1 || fields.Fields[0] != "limits.channelPolicy.channels" {
			t.Fatalf("non-canonical %q: %v %+v", id, err, fields)
		}
	}
	// Empty is rejected too (its operation id cannot carry it).
	{
		current, err := store.Limits(ctx, nil, "member")
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: current.Revision, OperationID: "canon-reject-empty-000000", Limits: Limits{Channels: ChannelPolicy{Mode: "deny", Channels: []string{""}}}})
		var fields *ValidationError
		if !errors.As(err, &fields) || len(fields.Fields) != 1 || fields.Fields[0] != "limits.channelPolicy.channels" {
			t.Fatalf("non-canonical empty: %v %+v", err, fields)
		}
	}
}

func TestAdmissionAppliesStreamsRatingLabelsAndSchedule(t *testing.T) {
	store, db, ids := fixture(t)
	ctx := context.Background()
	limits := Limits{MaxStreams: 1, MaxContentRating: "PG", AllowUnrated: true, Tags: TagPolicy{DeniedLabels: []string{"spoilers"}}}
	if _, err := store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: 1, OperationID: "limits-admission", Limits: limits}); err != nil {
		t.Fatal(err)
	}
	member := identity.Principal{Viewer: identity.Viewer{AccountID: "member", ProfileID: "member-profile", Authority: "local", Role: identity.TierMember}}
	enforcer := &Enforcer{Store: store}
	run := func(item string, remote bool) (Decision, error) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		return enforcer.AdmitPlayback(ctx, tx, member, Admission{ItemID: item, Remote: remote})
	}
	if _, err := run(ids.adult, false); !errors.Is(err, ErrContentRating) {
		t.Fatalf("a TV-MA title passed a PG ceiling: %v", err)
	}
	if _, err := run(ids.family, false); !errors.Is(err, ErrLabelDenied) {
		t.Fatalf("a denied label passed: %v", err)
	}
	// Remove the label and the same title is admitted, with the remote clamp.
	if err := dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		return compactcatalog.SetAttributesTx(ctx, tx, ids.familyID, "label", nil)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: 2, OperationID: "limits-bitrate", Limits: Limits{MaxStreams: 1, RemoteBitrateKbps: 3000, AllowUnrated: true}}); err != nil {
		t.Fatal(err)
	}
	decision, err := run(ids.family, true)
	if err != nil || decision.MaxVideoBitrateBPS != 3_000_000 {
		t.Fatalf("remote clamp: %v %+v", err, decision)
	}
	if decision, err = run(ids.family, false); err != nil || decision.MaxVideoBitrateBPS != 0 {
		t.Fatalf("a LAN request must not carry the remote clamp: %v %+v", err, decision)
	}
	// One live lease already exists, so the second is refused.
	if _, err = db.Exec(`INSERT INTO playback_sessions(id,session_hash,account_id,profile_id,item_id,asset_id,generation,state,grant_hash,grant_token,expires_at,request_id,duration)
 VALUES('s','h','member','member-profile',?, ?,1,'playing','g','t','2099-01-01T00:00:00Z','r',1)`, ids.familyID, ids.asset); err != nil {
		t.Fatal(err)
	}
	if _, err = run(ids.family, false); !errors.Is(err, ErrStreamLimit) {
		t.Fatalf("stream limit: %v", err)
	}
}

func TestScheduleWindowsWrapMidnightAndDenyOutsideHours(t *testing.T) {
	monday := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) // Monday noon
	open := AccessSchedule{Timezone: "UTC", Windows: []ScheduleWindow{{Days: []int{1}, StartMinute: 600, EndMinute: 780}}}
	if err := withinSchedule(open, monday); err != nil {
		t.Fatalf("inside the window: %v", err)
	}
	if err := withinSchedule(open, monday.Add(3*time.Hour)); !errors.Is(err, ErrSchedule) {
		t.Fatalf("outside the window: %v", err)
	}
	// 22:00 to 02:00 wraps: Monday 23:00 and Tuesday 01:00 are both inside.
	wrap := AccessSchedule{Timezone: "UTC", Windows: []ScheduleWindow{{Days: []int{1}, StartMinute: 1320, EndMinute: 120}}}
	if err := withinSchedule(wrap, monday.Add(11*time.Hour)); err != nil {
		t.Fatalf("Monday 23:00: %v", err)
	}
	if err := withinSchedule(wrap, monday.Add(13*time.Hour)); err != nil {
		t.Fatalf("Tuesday 01:00: %v", err)
	}
	if err := withinSchedule(wrap, monday.Add(15*time.Hour)); !errors.Is(err, ErrSchedule) {
		t.Fatalf("Tuesday 03:00 should be outside: %v", err)
	}
	if err := withinSchedule(AccessSchedule{}, monday); err != nil {
		t.Fatalf("no windows means no restriction: %v", err)
	}
}

func TestInvitationLifecycleCreatesTheAccountItDescribes(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	invitation, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "invite-1", Email: "Guest@Example.com", Role: identity.TierMember, AllowedLibraries: []string{"lib"}})
	if err != nil || invitation.Code == "" || invitation.Email != "guest@example.com" {
		t.Fatalf("invite: %v %+v", err, invitation)
	}
	// The code is returned once: a replay of the operation omits it.
	replay, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "invite-1", Email: "Guest@Example.com", Role: identity.TierMember, AllowedLibraries: []string{"lib"}})
	if err != nil || replay.Code != "" || replay.ID != invitation.ID {
		t.Fatalf("replay leaked or diverged: %v %+v", err, replay)
	}
	if _, err = store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "invite-2", Email: "guest@example.com"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a second pending invitation for one address: %v", err)
	}
	// An admin may not invite a peer.
	admin := identity.Principal{Viewer: identity.Viewer{AccountID: "a", Role: identity.TierAdmin}}
	if _, err = store.Invite(ctx, nil, admin, InvitationRequest{OperationID: "invite-3", Email: "peer@example.com", Role: identity.TierAdmin}); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("an admin invited an admin: %v", err)
	}
	page, err := store.Invitations(ctx, nil, InvitationQuery{State: "pending"})
	if err != nil || len(page.Items) != 1 || page.Items[0].Code != "" {
		t.Fatalf("list: %v %+v", err, page)
	}
	hash, err := HashPassword("Correct-Horse-9")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Accept(ctx, Acceptance{Code: "wrong-code-that-is-long-enough-here", Username: "guest", Password: "Correct-Horse-9"}, hash); !errors.Is(err, ErrExpired) {
		t.Fatalf("a wrong code was accepted: %v", err)
	}
	member, err := store.Accept(ctx, Acceptance{Code: invitation.Code, Username: "Guest", Password: "Correct-Horse-9", Name: "Guest"}, hash)
	if err != nil || member.Username != "guest" || member.Role != identity.TierMember || len(member.AllowedLibraries) != 1 {
		t.Fatalf("accept: %v %+v", err, member)
	}
	if _, err = store.Accept(ctx, Acceptance{Code: invitation.Code, Username: "guest2", Password: "Correct-Horse-9"}, hash); !errors.Is(err, ErrExpired) {
		t.Fatalf("an accepted invitation was redeemed twice: %v", err)
	}
	var state string
	if err = db.QueryRow(`SELECT state FROM access_invitations WHERE id=?`, invitation.ID).Scan(&state); err != nil || state != "accepted" {
		t.Fatalf("state: %v %q", err, state)
	}
	// A weak password is refused before any row is written.
	if _, err = HashPassword("short"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("password policy: %v", err)
	}
}

func TestInvitationPreviewIsMaskedReadOnlyAndOpaque(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	invite, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "preview-invite-1", Email: "Justin@example.com", AllowedLibraries: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := store.Preview(ctx, invite.Code)
	if err != nil || preview.Email != "j***@example.com" || preview.LibraryCount != 1 || preview.Role != identity.TierMember || preview.ExpiresAt != invite.ExpiresAt {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if _, err = store.Preview(ctx, invite.Code); err != nil {
		t.Fatalf("preview consumed invitation: %v", err)
	}
	if _, err = store.Preview(ctx, "wrong"); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("wrong code: %v", err)
	}
	if _, err = db.Exec(`UPDATE access_invitations SET expires_ms=0 WHERE id=?`, invite.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Preview(ctx, invite.Code); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("expired code: %v", err)
	}
	if _, err = db.Exec(`UPDATE access_invitations SET expires_ms=?,state='accepted' WHERE id=?`, time.Now().Add(time.Hour).UnixMilli(), invite.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Preview(ctx, invite.Code); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("used code: %v", err)
	}
}

func TestRevokedInvitationFreesTheAddressAndCannotUndoAcceptance(t *testing.T) {
	store, _, _ := fixture(t)
	ctx := context.Background()
	invitation, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "invite-revoke-1", Email: "guest@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RevokeInvitation(ctx, nil, invitation.ID, InvitationRevoke{ExpectedRevision: 9, OperationID: "revoke-1"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revoke: %v", err)
	}
	out, err := store.RevokeInvitation(ctx, nil, invitation.ID, InvitationRevoke{ExpectedRevision: 1, OperationID: "revoke-1"})
	if err != nil || out.State != "revoked" {
		t.Fatalf("revoke: %v %+v", err, out)
	}
	if _, err = store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "invite-revoke-2", Email: "guest@example.com"}); err != nil {
		t.Fatalf("the address stayed occupied after a revoke: %v", err)
	}
}

func TestAPIKeysAuthenticateAsADistinctPrincipalAndStopWhenRevoked(t *testing.T) {
	store, _, _ := fixture(t)
	ctx := context.Background()
	key, err := store.CreateAPIKey(ctx, nil, owner(), APIKeyRequest{OperationID: "key-create-1", Name: "Automation", Scope: "playback"})
	if err != nil || key.Secret == "" || key.Hint == "" {
		t.Fatalf("create: %v %+v", err, key)
	}
	principal, err := store.AuthenticateKey(ctx, key.Secret)
	if err != nil || principal.Principal.Authority != "api-key" || principal.Scope != "playback" || principal.Principal.Role != identity.TierOwner {
		t.Fatalf("authenticate: %v %+v", err, principal)
	}
	page, err := store.APIKeys(ctx, nil, APIKeyQuery{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Secret != "" || page.Items[0].LastUsed == "" {
		t.Fatalf("list leaked a secret or lost lastUsed: %v %+v", err, page)
	}
	if !ScopeAllows("playback", "read-only") || ScopeAllows("playback", "full") || ScopeAllows("nonsense", "read-only") {
		t.Fatal("scope ordering")
	}
	if _, err = store.RevokeAPIKey(ctx, nil, key.ID, APIKeyRevoke{ExpectedRevision: 1, OperationID: "key-revoke-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AuthenticateKey(ctx, key.Secret); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("a revoked key still authenticated: %v", err)
	}
}

func TestDeviceTrustGatesPlaybackAndBlockingStopsLiveLeases(t *testing.T) {
	store, db, ids := fixture(t)
	ctx := context.Background()
	member := identity.Principal{Viewer: identity.Viewer{AccountID: "member", ProfileID: "member-profile", Authority: "local", Role: identity.TierMember}}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Observe(ctx, tx, member, "living-room", "Living Room", "tvos"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	page, err := store.Devices(ctx, nil, DeviceQuery{}, true)
	if err != nil || len(page.Items) != 1 || page.Items[0].Trust != "pending" || !page.ApprovalRequired {
		t.Fatalf("inventory: %v %+v", err, page)
	}
	enforcer := &Enforcer{Store: store, ApprovalRequired: true}
	admit := func() error {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		_, err = enforcer.AdmitPlayback(ctx, tx, member, Admission{ItemID: ids.family, DeviceID: "living-room"})
		return err
	}
	if err = admit(); !errors.Is(err, ErrDeviceTrust) {
		t.Fatalf("a pending device played: %v", err)
	}
	if _, err = store.SetDeviceTrust(ctx, nil, "living-room", TrustChange{ExpectedRevision: 1, OperationID: "trust-set-1", Trust: "approved"}); err != nil {
		t.Fatal(err)
	}
	if err = admit(); err != nil {
		t.Fatalf("an approved device was refused: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO playback_sessions(id,session_hash,account_id,profile_id,item_id,asset_id,generation,state,grant_hash,grant_token,expires_at,request_id,duration)
 VALUES('s','h','member','member-profile',?, ?,1,'playing','g','t','2099-01-01T00:00:00Z','r',1)`, ids.familyID, ids.asset); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetDeviceTrust(ctx, nil, "living-room", TrustChange{ExpectedRevision: 2, OperationID: "trust-set-2", Trust: "blocked"}); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = db.QueryRow(`SELECT state FROM playback_sessions WHERE id='s'`).Scan(&state); err != nil || state != "stopped" {
		t.Fatalf("blocking left a lease running: %v %q", err, state)
	}
}

func TestListsPageByCursorRatherThanOffset(t *testing.T) {
	store, _, _ := fixture(t)
	ctx := context.Background()
	for i, address := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		store.Now = func() time.Time { return time.Unix(int64(1_700_000_000+i), 0) }
		if _, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "page-invite-" + address, Email: address}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.Invitations(ctx, nil, InvitationQuery{Limit: "2"})
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page: %v %+v", err, first)
	}
	second, err := store.Invitations(ctx, nil, InvitationQuery{Limit: "2", Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second page: %v %+v", err, second)
	}
	if second.Items[0].ID == first.Items[0].ID || second.Items[0].ID == first.Items[1].ID {
		t.Fatal("pages overlapped")
	}
	if _, err = store.Invitations(ctx, nil, InvitationQuery{Cursor: "not-a-cursor"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a malformed cursor was accepted: %v", err)
	}
	if _, err = store.Invitations(ctx, nil, InvitationQuery{Limit: "9999"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an oversized limit was accepted: %v", err)
	}
}

func TestVisibilityClauseHidesWhatAdmissionWouldRefuse(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	if _, err := store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: 1, OperationID: "limits-visibility",
		Limits: Limits{MaxContentRating: "PG", AllowUnrated: true, Tags: TagPolicy{DeniedLabels: []string{"spoilers"}}}}); err != nil {
		t.Fatal(err)
	}
	member := identity.Principal{Viewer: identity.Viewer{AccountID: "member", Role: identity.TierMember}}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	clause, args, err := (&Enforcer{Store: store}).VisibilityClause(ctx, tx, member, "e.id")
	if err != nil || clause == "" {
		t.Fatalf("clause: %v %q", err, clause)
	}
	rows, err := tx.Query(`SELECT pid(e.public_id) FROM catalog_entities e WHERE `+clause+` ORDER BY 1`, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	visible := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		visible = append(visible, id)
	}
	// The adult title is above the ceiling and the family title carries a
	// denied label, so a member with these limits sees neither.
	if len(visible) != 0 {
		t.Fatalf("visible: %v", visible)
	}
}

// Invitations are shared as a link and code; an address is optional. Any number
// of addressless invitations may be pending, the preview shows no address, and
// either can be accepted as a direct account or as a Portico Account.
func TestAddresslessInvitationsArePendingSideBySideAndAcceptEitherWay(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	first, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "blank-invite-1", Email: "  ", Role: identity.TierMember, AllowedLibraries: []string{"lib"}})
	if err != nil || first.Code == "" || first.Email != "" {
		t.Fatalf("first: %v %+v", err, first)
	}
	second, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "blank-invite-2", Role: identity.TierMember})
	if err != nil || second.Code == "" {
		t.Fatalf("a second addressless invitation: %v", err)
	}
	if _, err = store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "bad-address-invite", Email: "not an address"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an invalid address was accepted: %v", err)
	}
	page, err := store.Invitations(ctx, nil, InvitationQuery{State: "pending"})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("both pending: %v %+v", err, page)
	}
	preview, err := store.Preview(ctx, first.Code)
	if err != nil || preview.Email != "" || preview.LibraryCount != 1 {
		t.Fatalf("preview: %v %+v", err, preview)
	}
	hash, err := HashPassword("Correct-Horse-9")
	if err != nil {
		t.Fatal(err)
	}
	direct, err := store.Accept(ctx, Acceptance{Code: first.Code, Username: "guest", Password: "Correct-Horse-9"}, hash)
	if err != nil || direct.Username != "guest" {
		t.Fatalf("direct accept: %v %+v", err, direct)
	}
	var linked string
	_, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: second.Code, AccountID: "acc_portico", Username: "portico", DisplayName: "Portico"}, func(ctx context.Context, tx *sql.Tx) (identity.DirectSignIn, error) {
		return identity.DirectSignIn{}, tx.QueryRowContext(ctx, `SELECT account_id FROM account_portico_links WHERE hosted_account_id='acc_portico'`).Scan(&linked)
	})
	if err != nil || linked == "" {
		t.Fatalf("Portico accept: %v %q", err, linked)
	}
	var accepted int
	if err = db.QueryRow(`SELECT count(*) FROM access_invitations WHERE state='accepted'`).Scan(&accepted); err != nil || accepted != 2 {
		t.Fatalf("accepted %d %v", accepted, err)
	}
}

// A Portico Account invited as an admin joins as a local admin, with one
// primary profile named after it, and the journal queues its standing for
// Hosted, which indexes the role as pushed (INT N3, N4).
func TestPorticoAdminInvitationJoinsAsAdmin(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	invite, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "admin-invite", Role: identity.TierAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: invite.Code, AccountID: "acc_admin", Username: "ada", DisplayName: "Ada L"}, func(ctx context.Context, tx *sql.Tx) (identity.DirectSignIn, error) {
		return identity.DirectSignIn{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	var role, profile string
	var primaries, journal int
	if err = db.QueryRow(`SELECT m.role,(SELECT count(*) FROM direct_profiles p WHERE p.account_id=l.account_id AND p.is_primary=1 AND p.deleted=0),(SELECT name FROM direct_profiles p WHERE p.account_id=l.account_id AND p.is_primary=1),(SELECT count(*) FROM hosted_membership_journal WHERE hosted_account_id='acc_admin') FROM account_portico_links l JOIN direct_memberships m ON m.account_id=l.account_id WHERE l.hosted_account_id='acc_admin'`).Scan(&role, &primaries, &profile, &journal); err != nil {
		t.Fatal(err)
	}
	if role != identity.TierAdmin || primaries != 1 || profile != "Ada L" || journal == 0 {
		t.Fatalf("role=%s primaries=%d profile=%q journal=%d", role, primaries, profile, journal)
	}
}

// Accepting an invitation is a fresh consent to be listed, so it is always
// pushed, even when the account's standing here did not change (it left at
// Hosted, and the server has not heard yet; F-apple e2e, 23 Sep).
func TestPorticoAcceptanceAlwaysPushes(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	accept := func(op string) {
		t.Helper()
		invite, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: op, Role: identity.TierMember})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: invite.Code, AccountID: "acc_again", Username: "again", DisplayName: "Again"}, func(ctx context.Context, tx *sql.Tx) (identity.DirectSignIn, error) {
			return identity.DirectSignIn{}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	accept("first-invite")
	if _, err := db.Exec(`DELETE FROM hosted_membership_journal`); err != nil {
		t.Fatal(err)
	}
	accept("second-invite")
	var rows int
	if err := db.QueryRow(`SELECT count(*) FROM hosted_membership_journal WHERE hosted_account_id='acc_again'`).Scan(&rows); err != nil || rows == 0 {
		t.Fatalf("a repeated acceptance was not pushed: %d %v", rows, err)
	}
}

// A rating change applies synchronously now: there is no publication lag for
// a pending classification to hide behind.
func TestMemberVisibilityDeniesChangedRating(t *testing.T) {
	store, db, ids := fixture(t)
	ctx := context.Background()
	_, err := store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: 1, OperationID: "compact-pending", Limits: Limits{MaxContentRating: "PG", AllowUnrated: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err = dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		return compactcatalog.SetAttributesTx(ctx, tx, ids.familyID, "contentRating", []string{"TV-MA"})
	}); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	p := identity.Principal{Viewer: identity.Viewer{AccountID: "member", Role: identity.TierMember}}
	enforcer := &Enforcer{Store: store}
	if err = enforcer.VisibleItem(ctx, tx, p, ids.family); !errors.Is(err, ErrContentRating) {
		t.Fatalf("changed rating admitted: %v", err)
	}
	clause, args, err := enforcer.VisibilityClause(ctx, tx, p, "e.id")
	if err != nil {
		t.Fatal(err)
	}
	var visible bool
	queryArgs := append([]any{ids.familyID}, args...)
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM catalog_entities e WHERE e.id=? AND `+clause+`)`, queryArgs...).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible {
		t.Fatal("set predicate admitted changed rating")
	}
}
