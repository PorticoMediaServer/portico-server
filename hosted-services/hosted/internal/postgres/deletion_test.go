package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
	for _, table := range []string{"session_families", "account_sessions", "hosted_devices"} {
		var n int
		if e = f.store.DB.QueryRow(`SELECT count(*) FROM `+table+` WHERE account_id=$1`, a.Account.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal(table, n, e)
		}
	}
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, "retired_remote_erasure"); !errors.Is(e, identity.ErrInvalid) {
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
	if _, e = f.store.DB.Exec(`UPDATE servers SET name='Changed server' WHERE id=$1`, f.server); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, ""); !errors.Is(e, identity.ErrDeletionConflict) {
		t.Fatal(e)
	}
	p, e = f.ids.PrepareDeletion(ctx, f.member.Account, "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	if e = f.ids.ChangePassword(ctx, f.member.Account, "StrongPass9", "ChangedPass8"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, ""); !errors.Is(e, identity.ErrDeletionConflict) {
		t.Fatal(e)
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
	if w = request("/v1/servers/other/erasures/unknown/ack", f.token, `{"revision":1}`); w.Code != 404 {
		t.Fatal("retired erasure route", w.Code)
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
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, ""); e == nil {
		t.Fatal("expected transaction failure")
	}
	if _, e = f.ids.Authenticate(ctx, f.member.AccessToken); e != nil {
		t.Fatal("failed deletion revoked session", e)
	}
	var revision, members int
	if e = f.store.DB.QueryRow(`SELECT policy_revision,(SELECT count(*) FROM server_members WHERE server_id=$1) FROM servers WHERE id=$1`, f.server).Scan(&revision, &members); e != nil || revision != 2 || members != 2 {
		t.Fatal(revision, members, e)
	}
	var n int
	if e = f.store.DB.QueryRow(`SELECT count(*) FROM server_member_departures`).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	if _, e = f.store.DB.Exec(`DROP TRIGGER reject_erasure_test ON accounts; DROP FUNCTION reject_erasure_test()`); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ids.AcceptDeletion(ctx, p.Token, ""); e != nil {
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
