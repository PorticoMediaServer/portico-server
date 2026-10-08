package postgres_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

func TestDeletionNoHistoryRevokesCredentialsAndConcurrentIdempotence(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a, e := f.ids.Register(ctx, "erase-me", "Personal Name", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.ids.PrepareDeletion(ctx, a.Account, "WrongPass9"); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal(e)
	}
	p, e := f.ids.PrepareDeletion(ctx, a.Account, "StrongPass9")
	if e != nil || !p.CanDelete || len(p.AffectedServers) != 0 {
		t.Fatal(p, e)
	}
	var wg sync.WaitGroup
	results := make(chan identity.DeletionReceipt, 4)
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); r, e := f.ids.AcceptDeletion(ctx, p.Token, ""); results <- r; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var id string
	for r := range results {
		if r.Status != "complete" || len(r.PendingServers) != 0 || id != "" && r.ID != id {
			t.Fatal(r)
		}
		id = r.ID
	}
	if _, e = f.ids.Authenticate(ctx, a.AccessToken); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal("session survives", e)
	}
	if _, e = f.ids.Login(ctx, "erase-me", "StrongPass9"); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal("sign-in survives", e)
	}
	for _, table := range []string{"profiles", "session_families", "account_sessions", "hosted_devices"} {
		var n int
		if e = f.store.DB.QueryRow(`SELECT count(*) FROM `+table+` WHERE account_id=$1`, a.Account.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal(table, n, e)
		}
	}
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, identity.DeletePersonalResources); !errors.Is(e, identity.ErrDeletionConflict) {
		t.Fatal(e)
	}
	if _, e = f.ids.DeletionStatus(ctx, "del_invalid-unpredictable-token-value"); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal(e)
	}
	if _, e = f.ids.Register(ctx, "erase-me", "New identity", "StrongPass9"); e != nil {
		t.Fatal("username not erased", e)
	}
	f.store.DB.Exec(`UPDATE account_deletions SET receipt_expires_at=now()-interval '1 second' WHERE id=$1`, id)
	if e = f.store.PruneExpired(ctx, 10); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ids.DeletionStatus(ctx, p.Token); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal("expired receipt retained", e)
	}
}
func TestDeletionChangedPreviewAndPasswordBlock(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p, e := f.ids.PrepareDeletion(ctx, f.member.Account, "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`INSERT INTO profiles(id,account_id,name) VALUES('prof_new',$1,'New profile')`, f.member.Account.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, identity.DeletePersonalResources); !errors.Is(e, identity.ErrDeletionConflict) {
		t.Fatal(e)
	}
	p, e = f.ids.PrepareDeletion(ctx, f.member.Account, "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	if e = f.ids.ChangePassword(ctx, f.member.Account, "StrongPass9", "ChangedPass8"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, identity.DeletePersonalResources); !errors.Is(e, identity.ErrDeletionConflict) {
		t.Fatal(e)
	}
}
func TestDeletionSignedErasureRequiresExactServerAck(t *testing.T) {
	f := setup(t)
	f.installErasureClaimFixture(t)
	ctx := context.Background()
	p, e := f.ids.PrepareDeletion(ctx, f.member.Account, "StrongPass9")
	if e != nil || len(p.AffectedServers) != 1 || p.RequiredDisposition != identity.DeletePersonalResources {
		t.Fatal(p, e)
	}
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, ""); !errors.Is(e, identity.ErrDeletionConflict) {
		t.Fatal("implicit disposition accepted", e)
	}
	r, e := f.ids.AcceptDeletion(ctx, p.Token, identity.DeletePersonalResources)
	if e != nil || r.Status != "pending-offline-servers" || len(r.PendingServers) != 1 {
		t.Fatal(r, e)
	}
	// The server learns of the erasure at its next check-in (woken for it).
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	checkIn, e := f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{})
	if e != nil || !checkIn.CleanupPending {
		t.Fatal("check-in did not report the erasure", checkIn, e)
	}
	jobs, e := f.dir.Erasures(ctx, f.server)
	if e != nil || len(jobs) != 1 {
		t.Fatal(jobs, e)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(jobs[0].Payload)
	sig, _ := base64.RawURLEncoding.DecodeString(jobs[0].Signature)
	if !ed25519.Verify(f.keys.PrivateKey.Public().(ed25519.PublicKey), raw, sig) {
		t.Fatal("signature")
	}
	var j directory.ErasureJob
	if e = json.Unmarshal(raw, &j); e != nil || j.Kind != "portico.account-erasure.v1" || j.ServerID != f.server || j.AccountID != f.member.Account.ID || len(j.ProfileIDs) != 1 || j.ProfileIDs[0] != f.member.Profiles[0].ID || j.Disposition != identity.DeletePersonalResources {
		t.Fatal(j, e)
	}
	if e = f.dir.AcknowledgeErasure(ctx, "different", j.ID, j.Revision); !errors.Is(e, directory.ErrDenied) {
		t.Fatal(e)
	}
	var wake int64
	f.store.DB.QueryRow(`SELECT desired_revision FROM wake_state WHERE server_id=$1`, f.server).Scan(&wake)
	if _, e = f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{WakeRevision: wake}); e != nil {
		t.Fatal(e)
	}
	r, e = f.ids.DeletionStatus(ctx, p.Token)
	if e != nil || r.Status == "complete" {
		t.Fatal("a check-in falsely erased", r, e)
	}
	if e = f.dir.AcknowledgeErasure(ctx, f.server, j.ID, j.Revision); e != nil {
		t.Fatal(e)
	}
	if e = f.dir.AcknowledgeErasure(ctx, f.server, j.ID, j.Revision); e != nil {
		t.Fatal(e)
	}
	r, e = f.ids.DeletionStatus(ctx, p.Token)
	if e != nil || r.Status != "complete" || len(r.PendingServers) != 0 {
		t.Fatal(r, e)
	}
	if checkIn, e = f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{}); e != nil || checkIn.CleanupPending {
		t.Fatal("acknowledged erasure still pending", checkIn, e)
	}
	// Minimal pending erasure obligations cannot expire into false completion.
	f.store.DB.Exec(`UPDATE account_erasure_jobs SET acknowledged_at=NULL WHERE id=$1`, j.ID)
	f.store.DB.Exec(`UPDATE account_deletions SET completed_at=NULL,receipt_expires_at=now()-interval '1 day' WHERE id=$1`, r.ID)
	if e = f.store.PruneExpired(ctx, 100); e != nil {
		t.Fatal(e)
	}
	jobs, e = f.dir.Erasures(ctx, f.server)
	if e != nil || len(jobs) != 1 {
		t.Fatal("offline obligation expired", jobs, e)
	}
}

func TestDeletionHTTPPreviewReplayAndServerRouteAuthorization(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a, e := f.ids.Register(ctx, "http-delete", "HTTP delete", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	sec := &postgres.AccountSecurity{Store: f.store, Keys: f.keys, Identity: f.ids, Lifetime: 15 * time.Minute}
	h := (&httpapi.Handler{Identity: f.ids, Directory: f.dir, Security: sec}).Routes()
	request := func(path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	// A80: the preview needs a signed-in session only, no proof.
	intent := identity.SecurityIntent{Purpose: "account_deletion_preview", OperationID: security.RandomID("")}
	action, err := json.Marshal(postgres.SecurityAction{SecurityIntent: intent})
	if err != nil {
		t.Fatal(err)
	}
	if w := request("/v1/account/deletion/preview", "", string(action)); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := request("/v1/account/deletion/preview", a.AccessToken, string(action))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var p identity.DeletionPreview
	if e = json.Unmarshal(w.Body.Bytes(), &p); e != nil {
		t.Fatal(e)
	}
	// A80: the final step is confirmed by typing the account's email (its
	// username when it has none), not a password.
	var confirm string
	if e = f.store.DB.QueryRow(`SELECT COALESCE((SELECT address FROM account_contact_emails WHERE account_id=a.id AND is_primary),a.username) FROM accounts a WHERE a.id=$1`, a.Account.ID).Scan(&confirm); e != nil {
		t.Fatal(e)
	}
	wrong, _ := json.Marshal(map[string]string{"deletionToken": p.Token, "disposition": "", "confirmation": "someone-else@example.test"})
	if w = request("/v1/account/deletion", a.AccessToken, string(wrong)); w.Code != 400 || !strings.Contains(w.Body.String(), "deletion_confirmation_mismatch") {
		t.Fatal("wrong confirmation accepted", w.Code, w.Body.String())
	}
	body, _ := json.Marshal(map[string]string{"deletionToken": p.Token, "disposition": "", "confirmation": strings.ToUpper(confirm)})
	w = request("/v1/account/deletion", "", string(body))
	if w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("/v1/account/deletion", a.AccessToken, string(body))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("/v1/account/deletion", a.AccessToken, string(body))
	if w.Code != 200 {
		t.Fatal("ambiguous committed retry failed", w.Code)
	}
	if w = request("/v1/servers/other/erasures/unknown/ack", f.token, `{"revision":1}`); w.Code != 403 {
		t.Fatal("server-bound route", w.Code)
	}
}

func TestDeletionTransactionFailureRestoresAuthorityAndPolicy(t *testing.T) {
	f := setup(t)
	f.installErasureClaimFixture(t)
	ctx := context.Background()
	p, e := f.ids.PrepareDeletion(ctx, f.member.Account, "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`CREATE FUNCTION reject_erasure_test() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''injected rollback''; END'; CREATE TRIGGER reject_erasure_test BEFORE DELETE ON accounts FOR EACH ROW EXECUTE FUNCTION reject_erasure_test()`); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, identity.DeletePersonalResources); e == nil {
		t.Fatal("expected transaction failure")
	}
	if _, e = f.ids.Authenticate(ctx, f.member.AccessToken); e != nil {
		t.Fatal("failed deletion revoked session", e)
	}
	var revision, members int
	if e = f.store.DB.QueryRow(`SELECT policy_revision,(SELECT count(*) FROM memberships WHERE server_id=$1) FROM servers WHERE id=$1`, f.server).Scan(&revision, &members); e != nil || revision != 2 || members != 2 {
		t.Fatal(revision, members, e)
	}
	var n int
	if e = f.store.DB.QueryRow(`SELECT count(*) FROM account_erasure_jobs`).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	if _, e = f.store.DB.Exec(`DROP TRIGGER reject_erasure_test ON accounts; DROP FUNCTION reject_erasure_test()`); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, identity.DeletePersonalResources); e != nil {
		t.Fatal("safe retry failed", e)
	}
}

func (f fixture) installErasureClaimFixture(t *testing.T) {
	t.Helper()
	// setup now installs a real current-format claim for every directory fixture.
	// Assert rather than creating a second conflicting installed operation.
	a, err := f.store.CaptureServerAuthority(context.Background(), security.Hash(f.token))
	if err != nil || a.ServerID != f.server || a.AccountID != f.owner.Account.ID || a.ClaimGeneration != 1 || a.CredentialGeneration != 1 {
		t.Fatalf("missing installed erasure authority: %+v %v", a, err)
	}
}
