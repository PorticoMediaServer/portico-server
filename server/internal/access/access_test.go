package access

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
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

// A tier change applies live. The account's sessions take the new tier in the
// same transaction and stay signed in; sessions that were issued narrower than
// the account on purpose stay narrow.
func TestRoleChangeRequiresRevisionAppliesLiveAndRefusesPeers(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	testauth.InsertSession(t, db, "hash", "member", "member-profile", "local", "member", "2099-01-01T00:00:00Z")
	// An account session held between sign-in and choosing a profile.
	testauth.InsertSession(t, db, "account-hash", "member", "member-profile", "local", "account", "2099-01-01T00:00:00Z")
	// A viewer-only device.
	testauth.InsertSession(t, db, "viewer-hash", "member", "member-profile", "local", "member", "2099-01-01T00:00:00Z")
	if _, err := db.Exec(`INSERT INTO authorization_family_grants(family_id,grant_kind) VALUES('fixture-family-viewer-hash','viewer_only')`); err != nil {
		t.Fatal(err)
	}
	// A second profile of the same account, which is a member whatever the account is.
	if _, err := db.Exec(`INSERT INTO direct_profiles(id,account_id,name,is_primary) VALUES('second-profile','member','Second',0)`); err != nil {
		t.Fatal(err)
	}
	testauth.InsertSession(t, db, "second-hash", "member", "second-profile", "local", "member", "2099-01-01T00:00:00Z")
	session := func(hash string) (role string, revoked, epoch int) {
		t.Helper()
		if err := db.QueryRow(`SELECT role,revoked,epoch FROM authorization_access WHERE hash=?`, hash).Scan(&role, &revoked, &epoch); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := store.SetRole(ctx, nil, owner(), "member", RoleChange{ExpectedRevision: 9, OperationID: "operation-1", Role: identity.TierAdmin}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	out, err := store.SetRole(ctx, nil, owner(), "member", RoleChange{ExpectedRevision: 1, OperationID: "operation-1", Role: identity.TierAdmin})
	if err != nil || out.Role != identity.TierAdmin || out.Revision != 2 {
		t.Fatalf("promote: %v %+v", err, out)
	}
	for hash, want := range map[string]string{"hash": "admin", "account-hash": "account", "viewer-hash": "member", "second-hash": "member"} {
		if role, revoked, _ := session(hash); role != want || revoked != 0 {
			t.Fatalf("after promotion, session %s: role=%s revoked=%d, want role=%s and still signed in", hash, role, revoked, want)
		}
	}
	var epoch int
	if err = db.QueryRow(`SELECT epoch FROM accounts WHERE id='member'`).Scan(&epoch); err != nil || epoch != 1 {
		t.Fatalf("a tier change moved the account epoch (which ends every session): %d %v", epoch, err)
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
	// An admin may not edit a peer. The refusal is of a valid session: a
	// refusal (403), not a failed credential (401).
	admin := identity.Principal{Viewer: identity.Viewer{AccountID: "admin-actor", Role: identity.TierAdmin}}
	if _, err = store.SetRole(ctx, nil, admin, "member", RoleChange{ExpectedRevision: 2, OperationID: "operation-2", Role: identity.TierMember}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("an admin edited a peer: %v", err)
	}
	if _, err = store.SetRole(ctx, nil, owner(), "owner", RoleChange{ExpectedRevision: 1, OperationID: "operation-self", Role: identity.TierMember}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("the owner changed its own tier: %v", err)
	}
	// Demotion is live too: the administrator's devices stop being an
	// administrator's at once, and stay signed in.
	if out, err = store.SetRole(ctx, nil, owner(), "member", RoleChange{ExpectedRevision: 2, OperationID: "operation-3", Role: identity.TierMember}); err != nil || out.Role != identity.TierMember {
		t.Fatalf("demote: %v %+v", err, out)
	}
	if role, revoked, _ := session("hash"); role != "member" || revoked != 0 {
		t.Fatalf("after demotion: role=%s revoked=%d", role, revoked)
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

func TestAdmissionAppliesStreamsAndTheRemoteClamp(t *testing.T) {
	store, db, ids := fixture(t)
	ctx := context.Background()
	limits := Limits{MaxStreams: 1, MaxContentRating: "PG", AllowUnrated: true, Tags: TagPolicy{DeniedLabels: []string{"spoilers"}}}
	if _, err := store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: 1, OperationID: "limits-admission", Limits: limits}); err != nil {
		t.Fatal(err)
	}
	member := identity.Principal{Viewer: identity.Viewer{AccountID: "member", ProfileID: "member-profile", Authority: "local", Role: identity.TierMember}}
	enforcer := &Enforcer{Store: store}
	run := func(_ string, remote bool) (Decision, error) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		return enforcer.AdmitPlayback(ctx, tx, member, Admission{Remote: remote})
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

func TestCredentialScopesAreValidatedAndListedInPublishedOrder(t *testing.T) {
	for _, bad := range [][]string{nil, {}, {"read", "read"}, {"session"}, {"open"}, {"full"}, {"read", "playback", "library", "admin", "read"}} {
		if _, ok := NormalizeScopes(bad); ok {
			t.Fatalf("%v accepted", bad)
		}
	}
	got, ok := NormalizeScopes([]string{"admin", "read"})
	if !ok || len(got) != 2 || got[0] != "read" || got[1] != "admin" {
		t.Fatalf("order: %v %v", got, ok)
	}
}

func TestCredentialStoreRefusesWhatItCannotIssueAndUnknownSecrets(t *testing.T) {
	store, _, _ := fixture(t)
	ctx := context.Background()
	// Without an issuer nothing is made; a hosted-policy or profile-less
	// session cannot hold a credential at all.
	if _, err := store.CreateCredential(ctx, nil, nil, owner(), identity.CredentialKindKey, CredentialRequest{OperationID: "key-create-1", Name: "Automation", Scopes: []string{"read"}}); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("no issuer: %v", err)
	}
	if _, err := store.ResolveCredential(ctx, identity.Digest("pk.unknown")); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("unknown secret: %v", err)
	}
	if _, err := store.ResolveCredential(ctx, ""); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("empty digest: %v", err)
	}
	page, err := store.Credentials(ctx, nil, CredentialQuery{Kind: identity.CredentialKindKey})
	if err != nil || page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("empty list: %v %+v", err, page)
	}
	if _, err = store.Credentials(ctx, nil, CredentialQuery{Kind: identity.CredentialKindKey, Limit: "1000"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an unbounded page was accepted: %v", err)
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

// An invitation grants access to an account that has none here. An account
// that is already an active member keeps the tier and libraries the owner set
// for it, whichever way the invitation would have moved them: a lower tier
// must not demote it and a higher tier must not promote it. The owner changes
// an active member through member management, never through a link.
func TestPorticoAcceptanceLeavesAnActiveMemberAsItIs(t *testing.T) {
	for _, c := range []struct {
		name, held, heldLibraries, invited string
		invitedLibraries                   []string
	}{
		{"a lower tier does not demote", identity.TierAdmin, `["lib","other"]`, identity.TierMember, []string{}},
		{"a higher tier does not promote", identity.TierMember, `["lib"]`, identity.TierAdmin, []string{"lib", "other"}},
		{"the same tier does not replace libraries", identity.TierMember, `["lib"]`, identity.TierMember, []string{"other"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			store, db, _ := fixture(t)
			ctx := context.Background()
			signedIn := func(ctx context.Context, tx *sql.Tx) (identity.DirectSignIn, error) {
				return identity.DirectSignIn{}, nil
			}
			first, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "first-invite", Role: identity.TierMember})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: first.Code, AccountID: "acc_held", Username: "held", DisplayName: "Held"}, signedIn); err != nil {
				t.Fatal(err)
			}
			// The owner then sets this member's standing through member management.
			if _, err = db.Exec(`UPDATE direct_memberships SET role=?,allowed_libraries=?,revision=revision+1 WHERE account_id=(SELECT account_id FROM account_portico_links WHERE hosted_account_id='acc_held')`, c.held, c.heldLibraries); err != nil {
				t.Fatal(err)
			}
			var before int64
			if err = db.QueryRow(`SELECT m.revision FROM direct_memberships m JOIN account_portico_links l ON l.account_id=m.account_id WHERE l.hosted_account_id='acc_held'`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`DELETE FROM hosted_membership_journal`); err != nil {
				t.Fatal(err)
			}
			other, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "other-invite", Role: c.invited, AllowedLibraries: c.invitedLibraries})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: other.Code, AccountID: "acc_held", Username: "held", DisplayName: "Held"}, signedIn); err != nil {
				t.Fatalf("an active member could not accept: %v", err)
			}
			var role, libraries, state string
			var disabled bool
			var revision int64
			var accounts, journal int
			if err = db.QueryRow(`SELECT m.role,m.allowed_libraries,m.disabled,m.revision,(SELECT count(*) FROM account_portico_links WHERE hosted_account_id='acc_held'),(SELECT count(*) FROM hosted_membership_journal WHERE hosted_account_id='acc_held'),(SELECT state FROM access_invitations WHERE id=?) FROM direct_memberships m JOIN account_portico_links l ON l.account_id=m.account_id WHERE l.hosted_account_id='acc_held'`, other.ID).Scan(&role, &libraries, &disabled, &revision, &accounts, &journal, &state); err != nil {
				t.Fatal(err)
			}
			if role != c.held || libraries != c.heldLibraries || disabled || revision != before {
				t.Fatalf("an invitation rewrote an active member: role=%s libraries=%s disabled=%v revision=%d (was %s %s revision %d)", role, libraries, disabled, revision, c.held, c.heldLibraries, before)
			}
			// It is still one membership, the invitation is spent, and the fresh
			// consent is still pushed.
			if accounts != 1 || state != "accepted" || journal == 0 {
				t.Fatalf("accounts=%d invitation=%s journal=%d", accounts, state, journal)
			}
		})
	}
}

// A membership that was disabled (the account left at Hosted, or the owner
// turned it off) has no access here, so a fresh invitation is what admits it
// again, and it comes back with exactly what that invitation grants. Its old
// tier is never restored: a member-tier link must not bring an admin back.
func TestPorticoAcceptanceRestoresADisabledMemberWithTheInvitationsAccess(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	signedIn := func(ctx context.Context, tx *sql.Tx) (identity.DirectSignIn, error) {
		return identity.DirectSignIn{}, nil
	}
	first, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "first-invite", Role: identity.TierAdmin, AllowedLibraries: []string{"lib", "other"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: first.Code, AccountID: "acc_left", Username: "left", DisplayName: "Left"}, signedIn); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE direct_memberships SET disabled=1,revision=revision+1 WHERE account_id=(SELECT account_id FROM account_portico_links WHERE hosted_account_id='acc_left')`); err != nil {
		t.Fatal(err)
	}
	again, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "again-invite", Role: identity.TierMember, AllowedLibraries: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: again.Code, AccountID: "acc_left", Username: "left", DisplayName: "Left"}, signedIn); err != nil {
		t.Fatal(err)
	}
	var role, libraries string
	var disabled bool
	var accounts int
	if err = db.QueryRow(`SELECT m.role,m.allowed_libraries,m.disabled,(SELECT count(*) FROM account_portico_links WHERE hosted_account_id='acc_left') FROM direct_memberships m JOIN account_portico_links l ON l.account_id=m.account_id WHERE l.hosted_account_id='acc_left'`).Scan(&role, &libraries, &disabled, &accounts); err != nil {
		t.Fatal(err)
	}
	if role != identity.TierMember || libraries != `["lib"]` || disabled || accounts != 1 {
		t.Fatalf("role=%s libraries=%s disabled=%v accounts=%d", role, libraries, disabled, accounts)
	}
}

// The owner is never touched by an invitation, active or not.
func TestPorticoAcceptanceNeverChangesTheOwner(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO account_portico_links(account_id,hosted_account_id,consent) VALUES('owner','acc_owner','')`); err != nil {
		t.Fatal(err)
	}
	invite, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "owner-opens-it", Role: identity.TierMember, AllowedLibraries: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: invite.Code, AccountID: "acc_owner", Username: "owner", DisplayName: "Owner"}, func(ctx context.Context, tx *sql.Tx) (identity.DirectSignIn, error) {
		return identity.DirectSignIn{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	var role, libraries string
	var disabled bool
	if err = db.QueryRow(`SELECT role,allowed_libraries,disabled FROM direct_memberships WHERE account_id='owner'`).Scan(&role, &libraries, &disabled); err != nil {
		t.Fatal(err)
	}
	if role != identity.TierOwner || libraries != `[]` || disabled {
		t.Fatalf("an invitation changed the owner: role=%s libraries=%s disabled=%v", role, libraries, disabled)
	}
}

// The account rating limit is one value of one published rating system, or any
// spelling the rating table knows, and resolves to an admission age.
func TestRatingLimitIsValidatedAndResolvedToAnAge(t *testing.T) {
	store, _, _ := fixture(t)
	ctx := context.Background()
	revision := int64(1)
	for i, c := range []struct {
		system, rating string
		age            int
		ok             bool
	}{
		{"", "", -1, true},
		{"", "PG", 8, true},
		{"", "tv-pg", 8, true},
		{"", "  pg-13 ", 13, true},
		{"mpaa", "PG-13", 13, true},
		{"us-tv", "TV-14", 14, true},
		{"bbfc", "15", 15, true},
		{"fsk", "16", 16, true},
		{"mpaa", "TV-14", 0, false},
		{"nonsense", "PG", 0, false},
		{"", "Banana", 0, false},
		{"", "NR", 0, false},
	} {
		limits := DefaultLimits()
		limits.RatingSystem, limits.MaxContentRating = c.system, c.rating
		out, err := store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: revision, OperationID: "rating-limit-" + string(rune('a'+i)) + "-00000", Limits: limits})
		if !c.ok {
			var fields *ValidationError
			if !errors.As(err, &fields) || len(fields.Fields) != 1 || fields.Fields[0] != "limits.maxContentRating" {
				t.Fatalf("%q %q was accepted: %v", c.system, c.rating, err)
			}
			continue
		}
		if err != nil || out.Limits.MaximumAge != c.age {
			t.Fatalf("%q %q: age %d want %d: %v", c.system, c.rating, out.Limits.MaximumAge, c.age, err)
		}
		revision = out.Revision
		read, err := store.Limits(ctx, nil, "member")
		if err != nil || read.Limits.MaximumAge != c.age || read.Limits.Ceiling() != c.age {
			t.Fatalf("%q %q read back: %+v %v", c.system, c.rating, read.Limits, err)
		}
	}
	// A system with no rating means no ceiling, and is stored without the system.
	limits := DefaultLimits()
	limits.RatingSystem = "mpaa"
	out, err := store.SetLimits(ctx, nil, "member", LimitsChange{ExpectedRevision: revision, OperationID: "rating-limit-system-only", Limits: limits})
	if err != nil || out.Limits.RatingSystem != "" || out.Limits.MaximumAge != -1 {
		t.Fatalf("system without a rating: %+v %v", out.Limits, err)
	}
	// A stored rating the table no longer knows admits all-ages titles only.
	if got := (Limits{MaxContentRating: "no-longer-known"}).Ceiling(); got != 0 {
		t.Fatalf("an unreadable stored rating resolved to %d", got)
	}
}

func signedInNowhere(context.Context, *sql.Tx) (identity.DirectSignIn, error) {
	return identity.DirectSignIn{}, nil
}

func membershipOf(t *testing.T, db *sql.DB, hosted string) (role, libraries string, disabled bool, at int64, reason string) {
	t.Helper()
	if err := db.QueryRow(`SELECT m.role,m.allowed_libraries,m.disabled,m.disabled_ms,m.disabled_reason FROM direct_memberships m JOIN account_portico_links l ON l.account_id=m.account_id WHERE l.hosted_account_id=?`, hosted).Scan(&role, &libraries, &disabled, &at, &reason); err != nil {
		t.Fatal(err)
	}
	return
}

// Turning an account off records when and why, whichever statement does it,
// and turning it back on clears both.
func TestTurningAnAccountOffRecordsWhenAndWhy(t *testing.T) {
	_, db, _ := fixture(t)
	read := func() (bool, int64, string) {
		var disabled bool
		var at int64
		var reason string
		if err := db.QueryRow(`SELECT disabled,disabled_ms,disabled_reason FROM direct_memberships WHERE account_id='member'`).Scan(&disabled, &at, &reason); err != nil {
			t.Fatal(err)
		}
		return disabled, at, reason
	}
	if disabled, at, reason := read(); disabled || at != 0 || reason != "" {
		t.Fatalf("an account that is on: %v %d %q", disabled, at, reason)
	}
	before := time.Now().UnixMilli()
	if _, err := db.Exec(`UPDATE direct_memberships SET disabled=1 WHERE account_id='member'`); err != nil {
		t.Fatal(err)
	}
	if disabled, at, reason := read(); !disabled || at < before-1000 || at > time.Now().UnixMilli()+1000 || reason != "owner" {
		t.Fatalf("turned off with nothing said: %v %d %q", disabled, at, reason)
	}
	if _, err := db.Exec(`UPDATE direct_memberships SET disabled=0 WHERE account_id='member'`); err != nil {
		t.Fatal(err)
	}
	if disabled, at, reason := read(); disabled || at != 0 || reason != "" {
		t.Fatalf("turned back on: %v %d %q", disabled, at, reason)
	}
	// A writer that knows better (the person left, and when) is kept.
	if _, err := db.Exec(`UPDATE direct_memberships SET disabled=1,disabled_ms=12345,disabled_reason='left' WHERE account_id='member'`); err != nil {
		t.Fatal(err)
	}
	if disabled, at, reason := read(); !disabled || at != 12345 || reason != "left" {
		t.Fatalf("turned off as having left: %v %d %q", disabled, at, reason)
	}
	if _, err := db.Exec(`UPDATE direct_memberships SET disabled_reason='because' WHERE account_id='member'`); err == nil {
		t.Fatal("an unknown reason was stored")
	}
}

// An invitation brings a turned-off member back only if it was made after
// they were turned off. An older one, still lying around, does not undo the
// later decision; it stays usable by someone else.
func TestAnOlderInvitationDoesNotBringBackAMemberWhoWasTurnedOffLater(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	clock := time.Now().UnixMilli()
	store.Now = func() time.Time { return time.UnixMilli(clock) }
	join, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "join-invite", Role: identity.TierAdmin, AllowedLibraries: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: join.Code, AccountID: "acc_guest", Username: "guest", DisplayName: "Guest"}, signedInNowhere); err != nil {
		t.Fatal(err)
	}
	// A second invitation exists before the owner turns the member off.
	clock += 1000
	stale, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "stale-invite", Role: identity.TierAdmin, AllowedLibraries: []string{"lib", "other"}})
	if err != nil {
		t.Fatal(err)
	}
	clock += 1000
	if _, err = db.Exec(`UPDATE direct_memberships SET disabled=1,disabled_ms=?,disabled_reason='owner',revision=revision+1 WHERE account_id=(SELECT account_id FROM account_portico_links WHERE hosted_account_id='acc_guest')`, clock); err != nil {
		t.Fatal(err)
	}
	clock += 1000
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: stale.Code, AccountID: "acc_guest", Username: "guest", DisplayName: "Guest"}, signedInNowhere); !errors.Is(err, ErrExpired) {
		t.Fatalf("an invitation older than the turning off brought the member back: %v", err)
	}
	if _, _, disabled, _, reason := membershipOf(t, db, "acc_guest"); !disabled || reason != "owner" {
		t.Fatalf("the member after a refused invitation: disabled=%v reason=%q", disabled, reason)
	}
	var state string
	if err = db.QueryRow(`SELECT state FROM access_invitations WHERE id=?`, stale.ID).Scan(&state); err != nil || state != "pending" {
		t.Fatalf("the refused invitation was spent: %q %v", state, err)
	}
	// Someone else can still use it.
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: stale.Code, AccountID: "acc_other", Username: "other", DisplayName: "Other"}, signedInNowhere); err != nil {
		t.Fatalf("the older invitation used by someone else: %v", err)
	}
	// Inviting the member again, after turning them off, brings them back
	// with what the new invitation grants and nothing they had before.
	fresh, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "fresh-invite", Role: identity.TierMember, AllowedLibraries: []string{"other"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: fresh.Code, AccountID: "acc_guest", Username: "guest", DisplayName: "Guest"}, signedInNowhere); err != nil {
		t.Fatalf("an invitation made after the turning off: %v", err)
	}
	if role, libraries, disabled, at, reason := membershipOf(t, db, "acc_guest"); disabled || role != identity.TierMember || libraries != `["other"]` || at != 0 || reason != "" {
		t.Fatalf("the member brought back: role=%s libraries=%s disabled=%v at=%d reason=%q", role, libraries, disabled, at, reason)
	}
}

// Someone who left at their Portico Account comes back with an invitation
// made after they left, even if this server only heard about the leaving later.
func TestAMemberWhoLeftComesBackWithAnInvitationMadeAfterTheyLeft(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	clock := time.Now().UnixMilli()
	store.Now = func() time.Time { return time.UnixMilli(clock) }
	join, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "join-invite", Role: identity.TierMember, AllowedLibraries: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: join.Code, AccountID: "acc_leaver", Username: "leaver", DisplayName: "Leaver"}, signedInNowhere); err != nil {
		t.Fatal(err)
	}
	leftAt := clock + 1000
	clock += 2000
	back, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "back-invite", Role: identity.TierMember, AllowedLibraries: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}
	// The server hears of the leaving only now, with the time it happened.
	clock += 5000
	if _, err = db.Exec(`UPDATE direct_memberships SET disabled=1,disabled_ms=?,disabled_reason='left',revision=revision+1 WHERE account_id=(SELECT account_id FROM account_portico_links WHERE hosted_account_id='acc_leaver')`, leftAt); err != nil {
		t.Fatal(err)
	}
	if _, _, disabled, at, reason := membershipOf(t, db, "acc_leaver"); !disabled || at != leftAt || reason != "left" {
		t.Fatalf("the leaving as recorded: %v %d %q", disabled, at, reason)
	}
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: back.Code, AccountID: "acc_leaver", Username: "leaver", DisplayName: "Leaver"}, signedInNowhere); err != nil {
		t.Fatalf("an invitation made after the person left, heard about later: %v", err)
	}
	if _, _, disabled, _, _ := membershipOf(t, db, "acc_leaver"); disabled {
		t.Fatal("the person was not brought back")
	}
}

// An invitation made for one Portico Account is that account's only.
func TestAnInvitationMadeForOnePorticoAccountIsOnlyThatAccounts(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	invite, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "bound-invite", Role: identity.TierMember, AllowedLibraries: []string{"lib"}, PorticoUsername: "  Ada  "})
	if err != nil || invite.PorticoUsername != "ada" {
		t.Fatalf("invite: %+v %v", invite, err)
	}
	preview, err := store.Preview(ctx, invite.Code)
	if err != nil || !preview.ForPorticoAccount {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	accounts := func() int {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := accounts()
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: invite.Code, AccountID: "acc_mallory", Username: "mallory", DisplayName: "Mallory"}, signedInNowhere); !errors.Is(err, ErrInvitationRecipient) {
		t.Fatalf("another Portico Account accepted it: %v", err)
	}
	hash, err := HashPassword("Testing1!")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Accept(ctx, Acceptance{Code: invite.Code, Username: "ada", Password: "Testing1!", Name: "Ada"}, hash); !errors.Is(err, ErrInvitationRecipient) {
		t.Fatalf("it became a password account: %v", err)
	}
	if got := accounts(); got != before {
		t.Fatalf("a refused acceptance left an account behind: %d, was %d", got, before)
	}
	var state string
	if err = db.QueryRow(`SELECT state FROM access_invitations WHERE id=?`, invite.ID).Scan(&state); err != nil || state != "pending" {
		t.Fatalf("a refused acceptance spent the invitation: %q %v", state, err)
	}
	if _, err = store.AcceptPortico(ctx, PorticoAcceptance{Code: invite.Code, AccountID: "acc_ada", Username: "Ada", DisplayName: "Ada L"}, signedInNowhere); err != nil {
		t.Fatalf("the account it was made for: %v", err)
	}
	// An invitation with nobody named is still open to whoever holds the code.
	open, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "open-invite", Role: identity.TierMember})
	if err != nil || open.PorticoUsername != "" {
		t.Fatalf("%+v %v", open, err)
	}
	if p, err := store.Preview(ctx, open.Code); err != nil || p.ForPorticoAccount {
		t.Fatalf("open preview: %+v %v", p, err)
	}
	if _, err = store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "bad-bound-invite", PorticoUsername: "two words"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a username with a space: %v", err)
	}
}

// Invite-only sign-up spends a member invitation inside the registration.
func TestInvitationSpentAtRegistrationOnlyWhenItIsAPlainMembersAndUnspent(t *testing.T) {
	store, db, _ := fixture(t)
	ctx := context.Background()
	clock := time.Now().UnixMilli()
	store.Now = func() time.Time { return time.UnixMilli(clock) }
	redeem := store.RegistrationRedeemer()
	try := func(code string, commit bool) ([]string, error) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		libraries, err := redeem(ctx, tx, code, "member")
		if err == nil && commit {
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
		}
		return libraries, err
	}
	member, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "register-member", Role: identity.TierMember, AllowedLibraries: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}
	admin, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "register-admin", Role: identity.TierAdmin})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "register-bound", Role: identity.TierMember, PorticoUsername: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	for name, code := range map[string]string{"an administrator's invitation": admin.Code, "one made for a Portico Account": bound.Code, "an unknown code": strings.Repeat("x", 43), "a short code": "nope"} {
		if _, err := try(code, true); !errors.Is(err, identity.ErrRegistrationInvite) {
			t.Fatalf("%s was spent at registration: %v", name, err)
		}
	}
	// A registration that fails after redeeming leaves the invitation unspent.
	if libraries, err := try(member.Code, false); err != nil || len(libraries) != 1 || libraries[0] != "lib" {
		t.Fatalf("redeem: %v %v", libraries, err)
	}
	var state string
	if err = db.QueryRow(`SELECT state FROM access_invitations WHERE id=?`, member.ID).Scan(&state); err != nil || state != "pending" {
		t.Fatalf("a rolled-back registration spent the invitation: %q %v", state, err)
	}
	if _, err = try(member.Code, true); err != nil {
		t.Fatal(err)
	}
	if _, err = try(member.Code, true); !errors.Is(err, identity.ErrRegistrationInvite) {
		t.Fatalf("an invitation was spent twice: %v", err)
	}
	expired, err := store.Invite(ctx, nil, owner(), InvitationRequest{OperationID: "register-expired", Role: identity.TierMember, ExpiresInHours: 1})
	if err != nil {
		t.Fatal(err)
	}
	clock += int64(2 * time.Hour / time.Millisecond)
	if _, err = try(expired.Code, true); !errors.Is(err, identity.ErrRegistrationInvite) {
		t.Fatalf("an expired invitation was spent: %v", err)
	}
}
