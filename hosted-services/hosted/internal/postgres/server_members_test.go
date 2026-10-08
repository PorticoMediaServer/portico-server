package postgres_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/postgres"
)

const testChallenge = "challenge-from-the-server-0123456789"

// consent is what a server forwards for an account that signed in to it: that
// account's own identity assertion for the server.
func (f fixture) consent(t *testing.T, account string) json.RawMessage {
	t.Helper()
	signed, e := f.dir.IssueIdentity(account, "someone", "Someone", f.server, testChallenge, "")
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(signed)
	return raw
}

// custody is an account's own custody assertion for the server: it accepts
// custody of the claim (INT M6).
func (f fixture) custody(t *testing.T, account string) json.RawMessage {
	t.Helper()
	signed, e := f.dir.IssueIdentity(account, "someone", "Someone", f.server, testChallenge, "custody")
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(signed)
	return raw
}

// consentIssued is an assertion of the given kind as if Hosted had issued it
// at the given time.
func (f fixture) consentIssued(t *testing.T, account, kind string, at time.Time) json.RawMessage {
	t.Helper()
	claims := directory.IdentityClaims{Kind: kind, AccountID: account, Username: "someone", DisplayName: "Someone", ServerID: f.server, Challenge: testChallenge, Nonce: strings.Repeat("n", 32), IssuedAt: at.UTC(), ExpiresAt: at.UTC().Add(directory.IdentityLifetime)}
	payload, _ := json.Marshal(claims)
	raw, _ := json.Marshal(directory.SignedPolicy{Payload: base64.RawURLEncoding.EncodeToString(payload), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(f.keys.PrivateKey, payload)), KeyID: f.keys.KeyID})
	return raw
}

func (f fixture) listed(t *testing.T, account string) map[string]bool {
	t.Helper()
	items, e := f.dir.List(context.Background(), account)
	if e != nil {
		t.Fatal(e)
	}
	out := map[string]bool{}
	for _, item := range items {
		out[item.ID] = true
	}
	return out
}

func (f fixture) listVersion(t *testing.T, account string) int64 {
	t.Helper()
	var v int64
	if e := f.store.DB.QueryRow(`SELECT server_list_version FROM accounts WHERE id=$1`, account).Scan(&v); e != nil {
		t.Fatal(e)
	}
	return v
}

// The server is the authority; Hosted only indexes what it pushes. A push
// applies on top of the sequence Hosted holds, an exact replay is done, a gap
// is refused so the server sends its full list, and every change to an
// account's server set moves that account's list version.
func TestMemberPushIndexesServersForDiscovery(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	member := f.member.Account.ID
	if f.listed(t, member)[f.server] {
		t.Fatal("a legacy Hosted membership still grants discovery")
	}
	// Discovery and routes come from the index alone.
	if _, e = f.store.DB.Exec(`DELETE FROM memberships WHERE account_id=$1`, member); e != nil {
		t.Fatal(e)
	}
	before := f.listVersion(t, member)

	full := directory.MemberPush{After: 0, Through: 3, Full: true, Members: []directory.MemberStanding{
		{AccountID: f.owner.Account.ID, Role: "owner", Active: true},
		{AccountID: member, Role: "member", Active: true, Consent: f.consent(t, member)},
		{AccountID: "acc_unknown_to_hosted", Role: "member", Active: true, Consent: f.consent(t, "acc_unknown_to_hosted")},
	}}
	result, e := f.dir.PushMembers(ctx, authority, full)
	if e != nil || !result.Applied || result.Sequence != 3 {
		t.Fatal(result, e)
	}
	if !f.listed(t, member)[f.server] {
		t.Fatal("pushed member cannot find the server")
	}
	afterFull := f.listVersion(t, member)
	if afterFull <= before {
		t.Fatal("list version did not move", before, afterFull)
	}
	var unknown int
	f.store.DB.QueryRow(`SELECT count(*) FROM server_members WHERE account_id='acc_unknown_to_hosted'`).Scan(&unknown)
	if unknown != 0 {
		t.Fatal("an account Hosted does not know was indexed")
	}

	// A role change is a delta on top of 3.
	delta := directory.MemberPush{After: 3, Through: 5, Members: []directory.MemberStanding{{AccountID: member, Role: "admin", Active: true}}}
	if result, e = f.dir.PushMembers(ctx, authority, delta); e != nil || !result.Applied || result.Sequence != 5 {
		t.Fatal(result, e)
	}
	afterRole := f.listVersion(t, member)
	if afterRole <= afterFull {
		t.Fatal("role change did not move the version")
	}
	// The same push again (its answer was lost): done, nothing moves.
	if result, e = f.dir.PushMembers(ctx, authority, delta); e != nil || !result.Applied || result.Sequence != 5 || f.listVersion(t, member) != afterRole {
		t.Fatal("replay was not idempotent", result, e)
	}
	// A gap: Hosted says what it holds and applies nothing.
	gap := directory.MemberPush{After: 7, Through: 9, Members: []directory.MemberStanding{{AccountID: member, Active: false}}}
	if result, e = f.dir.PushMembers(ctx, authority, gap); e != nil || result.Applied || result.Sequence != 5 || !f.listed(t, member)[f.server] {
		t.Fatal("gap applied", result, e)
	}
	// Removal: gone from the list, version moves, the members-only label
	// rotation is queued.
	if _, e = f.store.DB.Exec(`DELETE FROM member_label_rotations`); e != nil {
		t.Fatal(e)
	}
	removal := directory.MemberPush{After: 5, Through: 6, Members: []directory.MemberStanding{{AccountID: member, Active: false}}}
	if result, e = f.dir.PushMembers(ctx, authority, removal); e != nil || !result.Applied {
		t.Fatal(result, e)
	}
	if f.listed(t, member)[f.server] || f.listVersion(t, member) <= afterRole {
		t.Fatal("removed member still finds the server")
	}
	var queued int
	f.store.DB.QueryRow(`SELECT count(*) FROM member_label_rotations WHERE server_id=$1`, f.server).Scan(&queued)
	if queued != 1 {
		t.Fatal("removal did not queue a label rotation", queued)
	}
	// The owner is untouched by a delta that does not name it.
	if !f.listed(t, f.owner.Account.ID)[f.server] {
		t.Fatal("owner lost the server")
	}

	// The check-in compares the index with the server's own digest.
	digest := directory.MemberDigest([]directory.MemberStanding{{AccountID: f.owner.Account.ID, Role: "owner", Active: true}})
	check, e := f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{MemberSequence: 6, MemberDigest: digest})
	if e != nil || !check.MemberDigestMatches || check.MemberSequence != 6 {
		t.Fatal("digest", check, e)
	}
	if check, e = f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{MemberSequence: 6, MemberDigest: "0"}); e != nil || check.MemberDigestMatches {
		t.Fatal("a wrong digest matched", check, e)
	}

	for name, bad := range map[string]directory.MemberPush{
		"through before after": {After: 6, Through: 5, Members: []directory.MemberStanding{}},
		"full not from zero":   {After: 1, Through: 7, Full: true, Members: []directory.MemberStanding{}},
		"no members list":      {After: 6, Through: 7},
		"bad role":             {After: 6, Through: 7, Members: []directory.MemberStanding{{AccountID: member, Role: "root", Active: true}}},
		"duplicate":            {After: 6, Through: 7, Members: []directory.MemberStanding{{AccountID: member, Role: "member", Active: true}, {AccountID: member, Active: false}}},
	} {
		if _, e = f.dir.PushMembers(ctx, authority, bad); !errors.Is(e, directory.ErrInvalid) {
			t.Fatal(name, e)
		}
	}
}

// Devices check their list version from memory, with a token that needs no
// account session. A change is never hidden, the token cannot be forged for
// another account, and before the map has loaded nothing says "unchanged".
func TestServerListVersionCheckFromMemory(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	member := f.member.Account.ID
	watch := f.dir.WatchToken(member)
	if _, _, e := f.dir.CheckServerList(watch, "0"); !errors.Is(e, directory.ErrUnavailable) {
		t.Fatal("an unloaded map answered", e)
	}
	if e := f.dir.RefreshServerListVersions(ctx); e != nil {
		t.Fatal(e)
	}
	held, same, e := f.dir.CheckServerList(watch, "0")
	if e != nil || !same || held != "0" {
		t.Fatal("fresh account", held, same, e)
	}
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.dir.PushMembers(ctx, authority, directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: member, Role: "member", Active: true, Consent: f.consent(t, member)}}}); e != nil {
		t.Fatal(e)
	}
	if e = f.dir.RefreshServerListVersions(ctx); e != nil {
		t.Fatal(e)
	}
	current, same, e := f.dir.CheckServerList(watch, held)
	if e != nil || same || current == held {
		t.Fatal("a change was hidden", current, same, e)
	}
	if v := f.listVersion(t, member); strconv.FormatInt(v, 10) != current {
		t.Fatal("memory and database disagree", v, current)
	}
	if _, same, e = f.dir.CheckServerList(watch, current); e != nil || !same {
		t.Fatal("unchanged list reported as changed", same, e)
	}
	// Another account's id under this account's MAC, or a truncated token.
	encoded := base64.RawURLEncoding.EncodeToString([]byte(f.owner.Account.ID))
	forged := encoded + watch[len(base64.RawURLEncoding.EncodeToString([]byte(member))):]
	for _, bad := range []string{forged, watch[:len(watch)-2], "x", ""} {
		if _, _, e = f.dir.CheckServerList(bad, current); !errors.Is(e, directory.ErrDenied) {
			t.Fatal("forged watch token accepted", bad, e)
		}
	}
	// The GET /v1/servers answer pairs a list with the version it reflects.
	if f.dir.ServerListVersion(member) != current {
		t.Fatal("list version")
	}
}

// The identity assertion the server verifies offline: signed by the documents
// key under the root, short-lived, one audience, fresh nonce each time, and in
// the exact field order the server re-marshals.
func TestIdentityAssertionIsShortLivedAndAudienceBound(t *testing.T) {
	f := setup(t)
	signed, e := f.dir.IssueIdentity(f.member.Account.ID, "member", "Member", f.server, testChallenge, "")
	if e != nil {
		t.Fatal(e)
	}
	// The fixture's key has no certificate; the chain to the root is the
	// signing key's (trust.Envelope.Verify), checked on the server side.
	raw, e := base64.RawURLEncoding.DecodeString(signed.Payload)
	if e != nil {
		t.Fatal(e)
	}
	signature, _ := base64.RawURLEncoding.DecodeString(signed.Signature)
	if !ed25519.Verify(f.keys.PrivateKey.Public().(ed25519.PublicKey), raw, signature) || signed.KeyID != f.keys.KeyID {
		t.Fatal("assertion not signed by the documents key")
	}
	var c directory.IdentityClaims
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	if c.Kind != "portico.identity.v1" || c.AccountID != f.member.Account.ID || c.ServerID != f.server || c.ExpiresAt.Sub(c.IssuedAt) != directory.IdentityLifetime || len(c.Nonce) < 22 {
		t.Fatalf("claims %+v", c)
	}
	if !strings.HasPrefix(string(raw), `{"kind":"portico.identity.v1","accountId":`) || c.Challenge != testChallenge {
		t.Fatal("wire order", string(raw))
	}
	again, _ := f.dir.IssueIdentity(f.member.Account.ID, "member", "Member", f.server, testChallenge, "")
	if again.Payload == signed.Payload {
		t.Fatal("nonce reused")
	}
	if _, e = f.dir.IssueIdentity(f.member.Account.ID, "member", "Member", "srv_short", testChallenge, ""); !errors.Is(e, directory.ErrInvalid) {
		t.Fatal("malformed server id accepted", e)
	}
}

// An ownership transfer on the server reaches Hosted as an owner-change delta
// in the same push stream. Custody of the claim follows it only once the new
// owner accepts custody with its own custody assertion (INT M6); until then
// custody is released (INT M10). After it moves, the new owner sees the
// server as theirs and can retire it, the old owner cannot, and the server's
// own credential keeps working. The account rows involved are locked in id
// order (INT N5).
func TestOwnerChangeMovesCustodyOnlyWithTheNewOwnersConsent(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	owner, member := f.owner.Account.ID, f.member.Account.ID
	if _, e = f.store.DB.Exec(`DELETE FROM memberships WHERE account_id=$1`, member); e != nil {
		t.Fatal(e)
	}
	full := directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: owner, Role: "owner", Active: true}, {AccountID: member, Role: "member", Active: true, Consent: f.consent(t, member)}}}
	if result, e := f.dir.PushMembers(ctx, authority, full); e != nil || result.Custodian != owner {
		t.Fatal(result, e)
	}
	custody := func() (string, string, bool) {
		t.Helper()
		var custodian, operationAccount string
		var released bool
		if e := f.store.DB.QueryRow(`SELECT s.owner_account_id,o.account_id,s.custody_released FROM servers s JOIN claim_operations o ON o.server_id=s.id AND o.state='installed' WHERE s.id=$1`, f.server).Scan(&custodian, &operationAccount, &released); e != nil {
			t.Fatal(e)
		}
		return custodian, operationAccount, released
	}
	owned := func(account string) bool {
		items, e := f.dir.List(ctx, account)
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range items {
			if item.ID == f.server {
				return item.OwnedByMe
			}
		}
		t.Fatal("server missing from", account)
		return false
	}
	retire := func(account string) error {
		tx, e := f.store.DB.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback()
		released, e := postgres.CustodyReleasedTxForTest(ctx, tx, f.server)
		if e != nil {
			t.Fatal(e)
		}
		if released {
			return directory.ErrConflict
		}
		if e = postgres.RetireServerTxForTest(ctx, tx, account, f.server, 1, 1, "owner_removed", ""); e != nil {
			return e
		}
		return tx.Commit()
	}

	// The server makes the member its owner. An identity assertion is not
	// custody consent, and neither is none: custody is released, not moved.
	for name, consent := range map[string]json.RawMessage{"none": nil, "identity": f.consent(t, member), "stale custody": f.consentIssued(t, member, directory.CustodyKind, time.Now().Add(-directory.CustodyConsentLifetime-time.Hour))} {
		push := directory.MemberPush{Through: 3, Full: true, Members: []directory.MemberStanding{{AccountID: owner, Role: "member", Active: true}, {AccountID: member, Role: "owner", Active: true, Consent: consent}}}
		result, e := f.dir.PushMembers(ctx, authority, push)
		if e != nil || !result.Applied || result.Custodian != "" {
			t.Fatal(name, result, e)
		}
		if custodian, operationAccount, released := custody(); custodian != owner || operationAccount != owner || !released {
			t.Fatal(name, "custody moved without the new owner's custody consent", custodian, operationAccount, released)
		}
	}
	if owned(owner) || !owned(member) {
		t.Fatal("ownedByMe while custody is released")
	}
	if e = retire(owner); !errors.Is(e, directory.ErrConflict) {
		t.Fatal("retired a server whose custody is released", e)
	}

	// The new owner accepts custody: its custody assertion arrives with its
	// standing, and custody follows.
	oldVersion := f.listVersion(t, member)
	accept := directory.MemberPush{After: 3, Through: 4, Members: []directory.MemberStanding{{AccountID: member, Role: "owner", Active: true, Consent: f.custody(t, member)}}}
	result, e := f.dir.PushMembers(ctx, authority, accept)
	if e != nil || !result.Applied || result.Custodian != member {
		t.Fatal(result, e)
	}
	if custodian, operationAccount, released := custody(); custodian != member || operationAccount != member || released {
		t.Fatal("custody did not follow the accepting owner", custodian, operationAccount, released)
	}
	if f.listVersion(t, member) <= oldVersion {
		t.Fatal("new owner's list version did not move")
	}
	if !owned(member) || owned(owner) {
		t.Fatal("ownedByMe did not follow the transfer")
	}
	// The server's credential still works, now under the new custodian.
	next, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil || next.AccountID != member {
		t.Fatal("server credential after transfer", next.AccountID, e)
	}
	if result, e := f.dir.PushMembers(ctx, next, directory.MemberPush{After: 4, Through: 5, Members: []directory.MemberStanding{}}); e != nil || !result.Applied || result.Custodian != member {
		t.Fatal("push after transfer", result, e)
	}
	var transfers int
	f.store.DB.QueryRow(`SELECT count(*) FROM server_ownership_transfers WHERE server_id=$1 AND previous_account_id=$2 AND new_account_id=$3`, f.server, owner, member).Scan(&transfers)
	if transfers != 1 {
		t.Fatal("transfer not recorded", transfers)
	}
	// The former custodian, a member now, can leave; retiring follows custody.
	if e = retire(owner); e == nil {
		t.Fatal("the old owner retired the server")
	}
	if e = retire(member); e != nil {
		t.Fatal("the new owner could not retire", e)
	}
}

// A server cannot list someone who never joined it: without the account's own
// assertion for this server it is skipped, and once it removes the server from
// its list only a newer consent brings it back (INT M6). The server learns of
// the departure at its check-in.
func TestMemberPushNeedsConsentAndHonoursDepartures(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	member := f.member.Account.ID
	if _, e = f.store.DB.Exec(`DELETE FROM memberships WHERE account_id=$1`, member); e != nil {
		t.Fatal(e)
	}
	oldConsent := f.consent(t, member)
	// Another server's assertion is not consent to this one.
	other, _ := f.dir.IssueIdentity(member, "m", "M", "srv_"+strings.Repeat("B", 43), testChallenge, "")
	otherRaw, _ := json.Marshal(other)
	for name, c := range map[string]json.RawMessage{"none": nil, "other server": otherRaw, "garbage": json.RawMessage(`{"payload":"x"}`)} {
		result, e := f.dir.PushMembers(ctx, authority, directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: member, Role: "member", Active: true, Consent: c}}})
		if e != nil || len(result.Skipped) != 1 || f.listed(t, member)[f.server] {
			t.Fatal(name, "listed without consent", result, e)
		}
	}
	// A consent older than ConsentLifetime lists no one.
	expired := f.consentIssued(t, member, directory.IdentityKind, time.Now().Add(-directory.ConsentLifetime-time.Hour))
	if result, e := f.dir.PushMembers(ctx, authority, directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: member, Role: "member", Active: true, Consent: expired}}}); e != nil || len(result.Skipped) != 1 || f.listed(t, member)[f.server] {
		t.Fatal("listed on an expired consent", result, e)
	}
	if result, e := f.dir.PushMembers(ctx, authority, directory.MemberPush{Through: 2, Full: true, Members: []directory.MemberStanding{{AccountID: f.owner.Account.ID, Role: "owner", Active: true}, {AccountID: member, Role: "member", Active: true, Consent: oldConsent}}}); e != nil || len(result.Skipped) != 0 || !f.listed(t, member)[f.server] {
		t.Fatal("consented member not listed", result, e)
	}
	// The member removes the server from its list; the custodian cannot.
	if e = f.dir.LeaveServer(ctx, f.owner.Account.ID, f.server); !errors.Is(e, directory.ErrConflict) {
		t.Fatal("the custodian left its own server", e)
	}
	before := f.listVersion(t, member)
	if e = f.dir.LeaveServer(ctx, member, f.server); e != nil {
		t.Fatal(e)
	}
	if f.listed(t, member)[f.server] || f.listVersion(t, member) <= before {
		t.Fatal("left server still listed")
	}
	checkIn, e := f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{MemberDigest: "x"})
	if e != nil || len(checkIn.Departures) != 1 || checkIn.Departures[0].AccountID != member || checkIn.Departures[0].LeftAt.IsZero() {
		t.Fatal("server not told of the departure", checkIn, e)
	}
	// The server's old consent does not re-list it; the server confirms the
	// departure by pushing the member inactive.
	if result, _ := f.dir.PushMembers(ctx, authority, directory.MemberPush{After: 2, Through: 3, Members: []directory.MemberStanding{{AccountID: member, Role: "member", Active: true, Consent: oldConsent}}}); len(result.Skipped) != 1 {
		t.Fatal("re-listed on a consent older than the departure", result)
	}
	if _, e = f.dir.PushMembers(ctx, authority, directory.MemberPush{After: 3, Through: 4, Members: []directory.MemberStanding{{AccountID: member}}}); e != nil {
		t.Fatal(e)
	}
	if checkIn, _ = f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{MemberDigest: "x"}); len(checkIn.Departures) != 0 {
		t.Fatal("departure still pending after the server ended it", checkIn.Departures)
	}
	time.Sleep(10 * time.Millisecond)
	if result, _ := f.dir.PushMembers(ctx, authority, directory.MemberPush{After: 4, Through: 5, Members: []directory.MemberStanding{{AccountID: member, Role: "member", Active: true, Consent: f.consent(t, member)}}}); len(result.Skipped) != 0 || !f.listed(t, member)[f.server] {
		t.Fatal("a new consent did not re-list the member", result)
	}
}

// A replayed sequence is done only if it is the same push; a server restored
// from a backup that reuses a number for other changes gets a full list asked
// for instead (INT M12). A full list arrives in pages and removes only what it
// never named.
func TestMemberPushReplayDigestAndPages(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	member := f.member.Account.ID
	add := directory.MemberPush{Through: 2, Full: true, Members: []directory.MemberStanding{{AccountID: member, Role: "member", Active: true, Consent: f.consent(t, member)}}}
	if _, e = f.dir.PushMembers(ctx, authority, add); e != nil {
		t.Fatal(e)
	}
	delta := directory.MemberPush{After: 2, Through: 3, Members: []directory.MemberStanding{{AccountID: member, Role: "admin", Active: true}}}
	if result, e := f.dir.PushMembers(ctx, authority, delta); e != nil || !result.Applied {
		t.Fatal(result, e)
	}
	if result, _ := f.dir.PushMembers(ctx, authority, delta); !result.Applied {
		t.Fatal("an exact replay was refused")
	}
	different := directory.MemberPush{After: 2, Through: 3, Members: []directory.MemberStanding{{AccountID: member}}}
	if result, _ := f.dir.PushMembers(ctx, authority, different); result.Applied {
		t.Fatal("a different push under a replayed number was acknowledged")
	}
	// Paged full list: page 0 keeps what later pages name; the owner, never
	// named, goes at the end.
	page0 := directory.MemberPush{Through: 9, Full: true, Page: 0, More: true, Members: []directory.MemberStanding{{AccountID: member, Role: "admin", Active: true}}}
	if result, e := f.dir.PushMembers(ctx, authority, page0); e != nil || !result.Applied || result.Sequence == 9 {
		t.Fatal("page 0", result, e)
	}
	if !f.listed(t, member)[f.server] {
		t.Fatal("member dropped mid-list")
	}
	if result, _ := f.dir.PushMembers(ctx, authority, directory.MemberPush{Through: 9, Full: true, Page: 2, Members: []directory.MemberStanding{}}); result.Applied {
		t.Fatal("an out-of-order page was applied")
	}
	if _, e = f.dir.PushMembers(ctx, authority, page0); e != nil {
		t.Fatal(e)
	}
	last := directory.MemberPush{Through: 9, Full: true, Page: 1, Members: []directory.MemberStanding{}}
	if result, e := f.dir.PushMembers(ctx, authority, last); e != nil || !result.Applied || result.Sequence != 9 {
		t.Fatal("last page", result, e)
	}
	var rows int
	f.store.DB.QueryRow(`SELECT count(*) FROM server_members WHERE server_id=$1`, f.server).Scan(&rows)
	if rows != 1 {
		t.Fatal("full list left", rows, "rows")
	}
}

// Ownership moved on the server to a direct account: custody is released.
// The former custodian no longer sees the server as its own, can leave it,
// and retiring waits for an owner who accepts custody (INT M10).
func TestCustodyIsReleasedWhenTheOwnerIsNotListed(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	owner := f.owner.Account.ID
	if _, e = f.dir.PushMembers(ctx, authority, directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: owner, Role: "owner", Active: true}}}); e != nil {
		t.Fatal(e)
	}
	if e = f.dir.LeaveServer(ctx, owner, f.server); !errors.Is(e, directory.ErrConflict) {
		t.Fatal("the custodian left while holding custody", e)
	}
	before := f.listVersion(t, owner)
	result, e := f.dir.PushMembers(ctx, authority, directory.MemberPush{After: 1, Through: 2, Members: []directory.MemberStanding{{AccountID: owner, Role: "admin", Active: true}}})
	if e != nil || result.Custodian != "" {
		t.Fatal("custody not released", result, e)
	}
	if f.listVersion(t, owner) <= before {
		t.Fatal("the former custodian's list did not move")
	}
	tx, e := f.store.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	released, e := postgres.CustodyReleasedTxForTest(ctx, tx, f.server)
	tx.Rollback()
	if e != nil || !released {
		t.Fatal("custody not released", released, e)
	}
	if e = f.dir.LeaveServer(ctx, owner, f.server); e != nil {
		t.Fatal("the former custodian could not leave", e)
	}
	if f.listed(t, owner)[f.server] {
		t.Fatal("the server is still in the former custodian's list")
	}
}

// What the list shows about a server changing moves its members' versions;
// the dormant list has its own validator (INT M11).
func TestServerListVersionMovesOnRenameAndVariesByList(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	owner := f.owner.Account.ID
	before := f.listVersion(t, owner)
	if e = f.store.ServerEvent(ctx, authority, postgres.ServerEvent{Kind: "rename", Name: "Den"}); e != nil {
		t.Fatal(e)
	}
	if f.listVersion(t, owner) <= before {
		t.Fatal("rename did not move the list version")
	}
	if e = f.dir.RefreshServerListVersions(ctx); e != nil {
		t.Fatal(e)
	}
	h := (&httpapi.Handler{Identity: f.ids, Directory: f.dir}).Routes()
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+f.owner.AccessToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	plain, dormant := get("/v1/servers").Header().Get("ETag"), get("/v1/servers?include=dormant").Header().Get("ETag")
	if plain == "" || plain == dormant {
		t.Fatal("list validators", plain, dormant)
	}
}
