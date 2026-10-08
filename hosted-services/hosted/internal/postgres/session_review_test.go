package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/sessions"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAccountSessionReviewOwnershipFactsAndRevocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	other, e := f.ids.LoginMode(ctx, "owner", "StrongPass9", "browser")
	if e != nil {
		t.Fatal(e)
	}
	page, e := svc.Review(ctx, f.owner.Account.ID, f.owner.AccessToken, "")
	if e != nil || page.AccountID != f.owner.Account.ID || page.CurrentFamilyID != f.owner.FamilyID || len(page.Items) != 2 {
		t.Fatal(page, e)
	}
	found := false
	for _, row := range page.Items {
		if row.ID == other.FamilyID {
			found = true
			if row.Mode != "browser" || row.Status != "active" || row.Current || row.CreatedAt.IsZero() || row.ExpiresAt.After(*other.RefreshExpiresAt) {
				t.Fatal(row)
			}
		}
	}
	if !found {
		t.Fatal("missing browser family")
	}
	if _, e = svc.RevokeReviewed(ctx, f.member.Account.ID, f.member.AccessToken, other.FamilyID); !errors.Is(e, sessions.ErrReviewNotFound) {
		t.Fatal("foreignfamily", e)
	}
	if _, e = svc.Review(ctx, f.member.Account.ID, f.owner.AccessToken, ""); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("forged account", e)
	}
	rotated, e := svc.Refresh(ctx, other.RefreshToken, refreshOne, "browser")
	if e != nil {
		t.Fatal(e)
	}
	for range 2 {
		result, e := svc.RevokeReviewed(ctx, f.owner.Account.ID, f.owner.AccessToken, other.FamilyID)
		if e != nil || !result.Revoked || result.Current || result.ID != other.FamilyID {
			t.Fatal(result, e)
		}
	}
	for _, token := range []string{other.AccessToken, rotated.AccessToken} {
		if _, e = f.ids.Authenticate(ctx, token); !errors.Is(e, identity.ErrCredentials) {
			t.Fatal("access survived", e)
		}
	}
	for _, token := range []string{other.RefreshToken, rotated.RefreshToken} {
		if _, e = svc.Refresh(ctx, token, refreshOne, "browser"); !errors.Is(e, sessions.ErrGrant) {
			t.Fatal("receipt restored revoked family", e)
		}
	}
	if _, e = f.ids.Authenticate(ctx, f.owner.AccessToken); e != nil {
		t.Fatal("current family affected", e)
	}
	if _, e = f.ids.Authenticate(ctx, f.member.AccessToken); e != nil {
		t.Fatal("other account affected", e)
	}
	// Revalidation happens after the account lock, independently of middleware.
	if _, e = svc.RevokeReviewed(ctx, f.owner.Account.ID, other.AccessToken, f.owner.FamilyID); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("revoked caller mutated another family", e)
	}
	current, e := svc.RevokeReviewed(ctx, f.owner.Account.ID, f.owner.AccessToken, f.owner.FamilyID)
	if e != nil || !current.Current || !current.Revoked {
		t.Fatal(current, e)
	}
	if _, e = svc.Review(ctx, f.owner.Account.ID, f.owner.AccessToken, ""); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("ended caller read", e)
	}
}
func TestAccountSessionReviewExpiryAndBoundedPages(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	for n := 1; n <= 105; n++ {
		if _, e := f.store.DB.Exec(`INSERT INTO session_families(id,account_id,mode,current_refresh_hash,account_epoch,absolute_expires_at,idle_expires_at,created_at) SELECT $1,id,'native',$3,auth_epoch,now()+interval '30 days',now()+interval '7 days',now()-($4*interval '1 minute') FROM accounts WHERE id=$2`, fmt.Sprintf("review-page-%03d", n), f.owner.Account.ID, security.Hash(fmt.Sprintf("test-refresh-%d", n)), n); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.store.DB.Exec(`UPDATE session_families SET idle_expires_at=now()-interval '1 minute' WHERE id='review-page-105'`); e != nil {
		t.Fatal(e)
	}
	cursor := ""
	seen := map[string]bool{}
	var firstCursor string
	pageCount := 0
	for {
		page, e := svc.Review(ctx, f.owner.Account.ID, f.owner.AccessToken, cursor)
		if e != nil || len(page.Items) > 40 {
			t.Fatal(page, e)
		}
		pageCount++
		for _, row := range page.Items {
			if seen[row.ID] {
				t.Fatal("duplicate session")
			}
			seen[row.ID] = true
			if row.ID == "review-page-105" && row.Status != "expired" {
				t.Fatal("expired active", row)
			}
		}
		if firstCursor == "" {
			firstCursor = page.NextCursor
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 106 || pageCount != 3 {
		t.Fatal(len(seen), pageCount)
	}
	another, e := f.ids.Login(ctx, "owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Review(ctx, f.owner.Account.ID, another.AccessToken, firstCursor); !errors.Is(e, sessions.ErrReviewCursor) {
		t.Fatal("cursor crossed family", e)
	}
	// Real HTTP continuation verifies the global query gate and strips credentials.
	h := (&httpapi.Handler{Identity: f.ids, Directory: f.dir, Sessions: svc, Origins: map[string]bool{"http://127.0.0.1:19439": true}, DevelopmentCookies: true}).Routes()
	r := httptest.NewRequest("GET", "/v1/account/sessions?cursor="+url.QueryEscape(firstCursor), nil)
	r.Header.Set("Authorization", "Bearer "+f.owner.AccessToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var decoded sessions.ReviewPage
	if e = json.Unmarshal(w.Body.Bytes(), &decoded); e != nil || len(decoded.Items) != 40 {
		t.Fatal(decoded, e)
	}
	for _, secret := range []string{"legacySessionsPresent", "current_refresh_hash", "tokenHash", "accessToken", "refreshToken", "password", f.owner.AccessToken} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("sensitive field in review", secret)
		}
	}
	if _, e = f.store.DB.Exec(`UPDATE session_families SET idle_expires_at=now()-interval '1 second' WHERE id=$1`, f.owner.FamilyID); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Review(ctx, f.owner.Account.ID, f.owner.AccessToken, ""); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("expired caller read", e)
	}
}
func TestAccountSessionReviewRevokeRacesRefreshAndMutualRevocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	other, e := f.ids.Login(ctx, "owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var rotated identity.Session
	var refreshErr, revokeErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		rotated, refreshErr = svc.Refresh(ctx, other.RefreshToken, refreshOne, "native")
	}()
	go func() {
		defer wg.Done()
		_, revokeErr = svc.RevokeReviewed(ctx, f.owner.Account.ID, f.owner.AccessToken, other.FamilyID)
	}()
	wg.Wait()
	if revokeErr != nil {
		t.Fatal(revokeErr)
	}
	if refreshErr != nil && !errors.Is(refreshErr, sessions.ErrGrant) {
		t.Fatal(refreshErr)
	}
	for _, token := range []string{other.AccessToken, rotated.AccessToken} {
		if token != "" {
			if _, e = f.ids.Authenticate(ctx, token); !errors.Is(e, identity.ErrCredentials) {
				t.Fatal("race access survived", e)
			}
		}
	}
	if _, e = svc.Refresh(ctx, other.RefreshToken, refreshOne, "native"); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("race replay", e)
	}
	a, e := f.ids.Login(ctx, "owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	b, e := f.ids.Login(ctx, "owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, e := svc.RevokeReviewed(ctx, a.Account.ID, a.AccessToken, b.FamilyID)
		errs <- e
	}()
	go func() {
		defer wg.Done()
		_, e := svc.RevokeReviewed(ctx, b.Account.ID, b.AccessToken, a.FamilyID)
		errs <- e
	}()
	wg.Wait()
	close(errs)
	success, denied := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, sessions.ErrGrant) {
			denied++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatal("revoked caller won race", success, denied)
	}
}
func TestAccountSessionReviewHTTPRevokeDoesNotTouchCookies(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	other, e := f.ids.Login(ctx, "owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	origin := "http://127.0.0.1:19439"
	h := (&httpapi.Handler{Identity: f.ids, Directory: f.dir, Sessions: svc, Origins: map[string]bool{origin: true}, DevelopmentCookies: true}).Routes()
	request := func(token, id, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("DELETE", "/v1/account/sessions/"+id, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request(f.member.AccessToken, other.FamilyID, origin); w.Code != 404 || len(w.Header().Values("Set-Cookie")) != 0 {
		t.Fatal(w.Code)
	}
	if w := request(f.owner.AccessToken, other.FamilyID, "http://untrusted.example"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request(f.owner.AccessToken, other.FamilyID, origin); w.Code != 200 || len(w.Header().Values("Set-Cookie")) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(f.owner.AccessToken, f.owner.FamilyID, origin); w.Code != 200 || len(w.Header().Values("Set-Cookie")) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(f.owner.AccessToken, f.member.FamilyID, origin); w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestAccountSessionReviewPasswordChangeRaceFencesCaller(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	other, e := f.ids.Login(ctx, "owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var passwordErr, reviewErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		passwordErr = f.ids.ChangePassword(ctx, f.owner.Account, "StrongPass9", "ChangedPass8")
	}()
	go func() {
		defer wg.Done()
		_, reviewErr = svc.RevokeReviewed(ctx, f.owner.Account.ID, f.owner.AccessToken, other.FamilyID)
	}()
	wg.Wait()
	if passwordErr != nil {
		t.Fatal(passwordErr)
	}
	if reviewErr != nil && !errors.Is(reviewErr, sessions.ErrGrant) {
		t.Fatal(reviewErr)
	}
	for _, token := range []string{f.owner.AccessToken, other.AccessToken} {
		if _, e = f.ids.Authenticate(ctx, token); !errors.Is(e, identity.ErrCredentials) {
			t.Fatal("old authority survives", e)
		}
	}
	fresh, e := f.ids.Login(ctx, "owner", "ChangedPass8")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.RevokeReviewed(ctx, f.owner.Account.ID, f.owner.AccessToken, fresh.FamilyID); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("stale caller revoked new family", e)
	}
	if _, e = f.ids.Authenticate(ctx, fresh.AccessToken); e != nil {
		t.Fatal("new family affected", e)
	}
}

func TestAccountSessionReviewCallerExpiresWhileWaitingForAccountLock(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	other, e := f.ids.Login(ctx, "owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	lock, e := f.store.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Rollback()
	var locked string
	if e = lock.QueryRow(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, f.owner.Account.ID).Scan(&locked); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		_, e := svc.RevokeReviewed(ctx, f.owner.Account.ID, f.owner.AccessToken, other.FamilyID)
		done <- e
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if e = f.store.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid<>pg_backend_pid() AND wait_event_type='Lock' AND query='SELECT id FROM accounts WHERE id=$1 FOR UPDATE')`).Scan(&waiting); e != nil {
			t.Fatal(e)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("revocation did not reach account lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The waiting transaction began before this expiry; its transaction now() is stale.
	if _, e = lock.Exec(`UPDATE account_sessions SET expires_at=clock_timestamp()+interval '50 milliseconds' WHERE token_hash=$1`, security.Hash(f.owner.AccessToken)); e != nil {
		t.Fatal(e)
	}
	time.Sleep(100 * time.Millisecond)
	if e = lock.Commit(); e != nil {
		t.Fatal(e)
	}
	if e = <-done; !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("expired waiting caller was authorized", e)
	}
	if _, e = f.ids.Authenticate(ctx, other.AccessToken); e != nil {
		t.Fatal("target was incorrectly revoked", e)
	}
}
