package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/sessions"
	"strings"
	"sync"
	"testing"
	"time"
)

const refreshOne = "12345678-1234-4123-8123-000000000001"
const refreshTwo = "12345678-1234-4123-8123-000000000002"

func TestRefreshRotationInputOutputRetryAndReuseRevocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	old := f.owner
	if old.RefreshToken == "" || old.FamilyID == "" {
		t.Fatal("login family missing")
	}
	rotated, e := svc.Refresh(ctx, old.RefreshToken, refreshOne, "native")
	if e != nil {
		t.Fatal(e)
	}
	if rotated.RefreshToken == old.RefreshToken || rotated.FamilyID != old.FamilyID || time.Until(rotated.ExpiresAt) > 16*time.Minute {
		t.Fatal("incorrect rotation")
	}
	for _, input := range []string{old.RefreshToken, rotated.RefreshToken} {
		again, e := svc.Refresh(ctx, input, refreshOne, "native")
		if e != nil || again.AccessToken != rotated.AccessToken || again.RefreshToken != rotated.RefreshToken {
			t.Fatalf("retry changed credentials: %v", e)
		}
	}
	_, e = svc.Refresh(ctx, old.RefreshToken, refreshTwo, "native")
	if !errors.Is(e, sessions.ErrReuse) {
		t.Fatalf("reuse not detected: %v", e)
	}
	for _, token := range []string{old.AccessToken, rotated.AccessToken} {
		if _, e = f.ids.Authenticate(ctx, token); !errors.Is(e, identity.ErrCredentials) {
			t.Fatal("reuse left access valid", e)
		}
	}
	if _, e = svc.Refresh(ctx, rotated.RefreshToken, refreshTwo, "native"); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("reuse left refresh valid", e)
	}
	if _, e = f.ids.Authenticate(ctx, f.member.AccessToken); e != nil {
		t.Fatal("unrelated account affected")
	}
}
func TestConcurrentSameRefreshRequestHasOneRotation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	var wg sync.WaitGroup
	out := make(chan identity.Session, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, e := svc.Refresh(ctx, f.owner.RefreshToken, refreshOne, "native")
			out <- s
			errs <- e
		}()
	}
	wg.Wait()
	close(out)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first identity.Session
	for s := range out {
		if first.AccessToken != "" && (first.AccessToken != s.AccessToken || first.RefreshToken != s.RefreshToken) {
			t.Fatal("duplicate result")
		}
		first = s
	}
	var generations, receipts int
	f.store.DB.QueryRow(`SELECT generation FROM session_families WHERE id=$1`, f.owner.FamilyID).Scan(&generations)
	f.store.DB.QueryRow(`SELECT count(*) FROM refresh_receipts WHERE family_id=$1`, f.owner.FamilyID).Scan(&receipts)
	if generations != 1 || receipts != 1 {
		t.Fatal(generations, receipts)
	}
}
func TestRefreshExpirySupersessionAndLogoutAll(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	first, e := svc.Refresh(ctx, f.owner.RefreshToken, refreshOne, "native")
	if e != nil {
		t.Fatal(e)
	}
	second, e := svc.Refresh(ctx, first.RefreshToken, refreshTwo, "native")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`UPDATE refresh_receipts SET expires_at=now()-interval '1 second' WHERE family_id=$1`, f.owner.FamilyID); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Refresh(ctx, second.RefreshToken, refreshTwo, "native"); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("expired receipt recovered")
	}
	other, e := f.ids.Login(ctx, "owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.LogoutAll(ctx, f.owner.Account.ID); e != nil {
		t.Fatal(e)
	}
	for _, s := range []identity.Session{f.owner, first, second, other} {
		if _, e = f.ids.Authenticate(ctx, s.AccessToken); !errors.Is(e, identity.ErrCredentials) {
			t.Fatal("logout-all left access")
		}
		if _, e = svc.Refresh(ctx, s.RefreshToken, "12345678-1234-4123-8123-000000000003", "native"); !errors.Is(e, sessions.ErrGrant) {
			t.Fatal("logout-all left refresh", e)
		}
	}
}
func TestRefreshFamilyLifetimeAndPasswordLogoutBoundaries(t *testing.T) {
	var e error
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	if _, e = f.store.DB.Exec(`UPDATE session_families SET idle_expires_at=now()-interval '1 second' WHERE id=$1`, f.owner.FamilyID); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Refresh(ctx, f.owner.RefreshToken, refreshOne, "native"); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("idle expiry ignored")
	}
	fresh, e := f.ids.Login(ctx, "owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`UPDATE session_families SET absolute_expires_at=now()+interval '30 seconds' WHERE id=$1`, fresh.FamilyID); e != nil {
		t.Fatal(e)
	}
	rotated, e := svc.Refresh(ctx, fresh.RefreshToken, refreshOne, "native")
	if e != nil {
		t.Fatal(e)
	}
	if time.Until(rotated.ExpiresAt) > time.Minute {
		t.Fatal("access exceeds absolute family lifetime")
	}
	if e = f.ids.ChangePassword(ctx, f.owner.Account, "StrongPass9", "ChangedPass8"); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Refresh(ctx, rotated.RefreshToken, refreshTwo, "native"); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("password change left refresh")
	}
	next, e := f.ids.Login(ctx, "owner", "ChangedPass8")
	if e != nil {
		t.Fatal(e)
	}
	if e = f.ids.Revoke(ctx, next.AccessToken); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Refresh(ctx, next.RefreshToken, refreshOne, "native"); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("current logout left refresh")
	}
}
func TestBrowserRefreshCookieModeAndLostHeaderBoundary(t *testing.T) {
	f := setup(t)
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	h := (&httpapi.Handler{Identity: f.ids, Sessions: svc, Security: &postgres.AccountSecurity{Store: f.store, Keys: f.keys, Identity: f.ids, Lifetime: 15 * time.Minute}, AccountOrigin: "https://web.getportico.tv", Origins: map[string]bool{"https://web.getportico.tv": true}}).Routes()
	call := func(path, origin, body string, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if cookie != nil {
			r.AddCookie(cookie)
			r.AddCookie(&http.Cookie{Name: cookie.Name + "_csrf", Value: strings.Repeat("c", 43)})
			r.Header.Set("X-CSRF-Token", strings.Repeat("c", 43))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s status%d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	login := call("/v1/sessions", "https://web.getportico.tv", `{"username":"owner","password":"StrongPass9","sessionMode":"browser","requestId":"`+security.RandomID("")+`"}`, nil, 200)
	var initial identity.Session
	if e := json.Unmarshal(login.Body.Bytes(), &initial); e != nil {
		t.Fatal(e)
	}
	if initial.RefreshToken != "" || initial.FamilyID == "" {
		t.Fatal("browser refresh token exposed")
	}
	var old *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == "__Host-portico_refresh" {
			old = cookie
		}
	}
	if old == nil {
		t.Fatal("missing refresh cookie")
	}
	if old.Name != "__Host-portico_refresh" || !old.Secure || !old.HttpOnly || old.Domain != "" || old.Path != "/" || old.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe cookie", old)
	}
	if login.Header().Get("Access-Control-Allow-Credentials") != "true" || login.Header().Get("Vary") != "Origin" {
		t.Fatal("credential CORS missing")
	}
	body := `{"familyId":"` + initial.FamilyID + `","requestId":"` + refreshOne + `"}`
	first := call("/v1/sessions/refresh", "https://web.getportico.tv", body, old, 200)
	newCookie := first.Result().Cookies()[0]
	if newCookie.Value == old.Value {
		t.Fatal("cookie not rotated")
	}
	for _, c := range []*http.Cookie{old, newCookie} {
		again := call("/v1/sessions/refresh", "https://web.getportico.tv", body, c, 200)
		if again.Body.String() != first.Body.String() || again.Result().Cookies()[0].Value != newCookie.Value {
			t.Fatal("lost-header/body retry differs")
		}
	}
	call("/v1/sessions/refresh", "", body, newCookie, 403)
	call("/v1/sessions/refresh", "https://evil.example", body, newCookie, 403)
	call("/v1/sessions/refresh", "https://web.getportico.tv", `{"requestId":"`+refreshOne+`","refreshToken":"`+newCookie.Value+`"}`, nil, 403)
	revoked := call("/v1/sessions/revoke", "https://web.getportico.tv", `{"familyId":"`+initial.FamilyID+`"}`, newCookie, 204)
	if revoked.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout cookie not cleared")
	}
	call("/v1/sessions/refresh", "https://web.getportico.tv", body, newCookie, 401)
}

func TestDeviceRefreshFamilyCancelAndLogoutAllFencesApprovals(t *testing.T) {
	store, devices, _, a, keys := activation(t)
	ctx := context.Background()
	svc := sessions.New(store, keys, 15*time.Minute)
	auth := startTV(t, devices, 1)
	approveTV(t, devices, a, auth)
	due(t, store)
	first, e := devices.Poll(ctx, auth.DeviceCode)
	if e != nil {
		t.Fatal(e)
	}
	if first.RefreshToken == "" || first.FamilyID == "" {
		t.Fatal("device family missing")
	}
	rotated, e := svc.Refresh(ctx, first.RefreshToken, refreshOne, "native")
	if e != nil {
		t.Fatal(e)
	}
	due(t, store)
	_, e = devices.Poll(ctx, auth.DeviceCode)
	codeError(t, e, "access_denied")
	if e = devices.Cancel(ctx, auth.DeviceCode); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Refresh(ctx, rotated.RefreshToken, refreshTwo, "native"); !errors.Is(e, sessions.ErrGrant) {
		t.Fatal("device cancel did not revoke rotated family")
	}
	pending := startTV(t, devices, 2)
	approveTV(t, devices, a, pending)
	if e = svc.LogoutAll(ctx, a.Account.ID); e != nil {
		t.Fatal(e)
	}
	due(t, store)
	_, e = devices.Poll(ctx, pending.DeviceCode)
	codeError(t, e, "access_denied")
}
func TestConcurrentDifferentRefreshRequestsRevokeFamily(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for _, id := range []string{refreshOne, refreshTwo} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, e := svc.Refresh(ctx, f.owner.RefreshToken, id, "native")
			out <- e
		}(id)
	}
	wg.Wait()
	close(out)
	success, reuse := 0, 0
	for e := range out {
		if e == nil {
			success++
		} else if errors.Is(e, sessions.ErrReuse) {
			reuse++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || reuse != 1 {
		t.Fatal(success, reuse)
	}
	if _, e := f.ids.Authenticate(ctx, f.owner.AccessToken); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal("reuse access remains")
	}
}

func TestConcurrentRefreshAndLogoutAllCannotLeaveCredentials(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	var wg sync.WaitGroup
	wg.Add(2)
	var refreshErr, logoutErr error
	go func() { defer wg.Done(); _, refreshErr = svc.Refresh(ctx, f.owner.RefreshToken, refreshOne, "native") }()
	go func() { defer wg.Done(); logoutErr = svc.LogoutAll(ctx, f.owner.Account.ID) }()
	wg.Wait()
	if logoutErr != nil {
		t.Fatal(logoutErr)
	}
	if refreshErr != nil && !errors.Is(refreshErr, sessions.ErrGrant) {
		t.Fatal(refreshErr)
	}
	var count int
	if e := f.store.DB.QueryRow(`SELECT count(*) FROM account_sessions WHERE account_id=$1`, f.owner.Account.ID).Scan(&count); e != nil || count != 0 {
		t.Fatalf("credentials survived: %d %v", count, e)
	}
}

func TestBrowserExpectedFamilyCannotMutateNewLogin(t *testing.T) {
	f := setup(t)
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	h := (&httpapi.Handler{Identity: f.ids, Sessions: svc, Security: &postgres.AccountSecurity{Store: f.store, Keys: f.keys, Identity: f.ids, Lifetime: 15 * time.Minute}, AccountOrigin: "https://web.getportico.tv", Origins: map[string]bool{"https://web.getportico.tv": true}}).Routes()
	call := func(path, body string, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://web.getportico.tv")
		if cookie != nil {
			r.AddCookie(cookie)
			r.AddCookie(&http.Cookie{Name: cookie.Name + "_csrf", Value: strings.Repeat("c", 43)})
			r.Header.Set("X-CSRF-Token", strings.Repeat("c", 43))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	login := func() (identity.Session, *http.Cookie) {
		t.Helper()
		w := call("/v1/sessions", `{"username":"owner","password":"StrongPass9","sessionMode":"browser","requestId":"`+security.RandomID("")+`"}`, nil, 200)
		var s identity.Session
		if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
			t.Fatal(err)
		}
		for _, cookie := range w.Result().Cookies() {
			if cookie.Name == "__Host-portico_refresh" {
				return s, cookie
			}
		}
		t.Fatal("missing browser refresh cookie")
		return s, nil
	}
	old, oldCookie := login()
	newer, newCookie := login()
	if old.FamilyID == newer.FamilyID {
		t.Fatal("families reused")
	}
	stale := `{"familyId":"` + old.FamilyID + `"}`
	w := call("/v1/sessions/revoke", stale, newCookie, 204)
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("stale revoke cleared newer cookie")
	}
	body := `{"familyId":"` + old.FamilyID + `","requestId":"` + refreshOne + `"}`
	w = call("/v1/sessions/refresh", body, newCookie, 409)
	if len(w.Result().Cookies()) != 0 || !strings.Contains(w.Body.String(), `"code":"family_changed"`) || !strings.Contains(w.Body.String(), `"retryable":false`) {
		t.Fatal("family mismatch response unsafe", w.Body.String())
	}
	// Neither family changed as a consequence of the stale intent.
	var generation int
	for _, family := range []string{old.FamilyID, newer.FamilyID} {
		if err := f.store.DB.QueryRow(`SELECT generation FROM session_families WHERE id=$1 AND revoked_at IS NULL`, family).Scan(&generation); err != nil || generation != 0 {
			t.Fatalf("family mutated: %d %v", generation, err)
		}
	}
	for _, token := range []string{old.AccessToken, newer.AccessToken} {
		if _, err := f.ids.Authenticate(context.Background(), token); err != nil {
			t.Fatal("stale intent revoked access", err)
		}
	}
	body = `{"familyId":"` + newer.FamilyID + `","requestId":"` + refreshOne + `"}`
	rotated := call("/v1/sessions/refresh", body, newCookie, 200)
	nextCookie := rotated.Result().Cookies()[0]
	// The same operation is recoverable with either input or output cookie.
	for _, c := range []*http.Cookie{newCookie, nextCookie} {
		again := call("/v1/sessions/refresh", body, c, 200)
		if again.Body.String() != rotated.Body.String() {
			t.Fatal("bound recovery changed result")
		}
	}
	w = call("/v1/sessions/revoke", `{"familyId":"`+newer.FamilyID+`"}`, nextCookie, 204)
	// The refresh cookie and its CSRF cookie are both cleared (A16).
	if cleared := w.Result().Cookies(); len(cleared) != 2 || cleared[0].MaxAge != -1 || cleared[1].MaxAge != -1 || cleared[1].Name != cleared[0].Name+"_csrf" {
		t.Fatal("matching revoke did not clear cookie")
	}
	call("/v1/sessions/refresh", body, nextCookie, 401)
	// The older family remains independently usable.
	call("/v1/sessions/refresh", `{"familyId":"`+old.FamilyID+`","requestId":"`+refreshOne+`"}`, oldCookie, 200)
}

// A refresh answer lost in an outage longer than five minutes (and longer than
// the access token's life) replays exactly until the successor refresh token is
// first used; after that the predecessor is reuse and revokes the family.
func TestRefreshReceiptOutlivesAnOutageUntilTheSuccessorIsUsed(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := sessions.New(f.store, f.keys, 15*time.Minute)
	rotated, e := svc.Refresh(ctx, f.owner.RefreshToken, refreshOne, "native")
	if e != nil {
		t.Fatal(e)
	}
	var receipts int
	var horizon bool
	if e = f.store.DB.QueryRow(`SELECT count(*),bool_and(expires_at>now()+interval '1 day') FROM refresh_receipts WHERE family_id=$1`, f.owner.FamilyID).Scan(&receipts, &horizon); e != nil || receipts != 1 || !horizon {
		t.Fatalf("receipt count %d, lives to the horizon %v, %v", receipts, horizon, e)
	}
	// The outage outlasts the access token (and the old five-minute receipt).
	if _, e = f.store.DB.Exec(`UPDATE account_sessions SET expires_at=now()-interval '1 second' WHERE family_id=$1`, f.owner.FamilyID); e != nil {
		t.Fatal(e)
	}
	// It also outlasts the one-day retention of consumed refresh tokens:
	// retention runs, and must keep the predecessor its live receipt names.
	if _, e = f.store.DB.Exec(`UPDATE refresh_tokens SET consumed_at=now()-interval '3 days' WHERE family_id=$1 AND consumed_at IS NOT NULL`, f.owner.FamilyID); e != nil {
		t.Fatal(e)
	}
	if e = f.store.PruneUntilCaughtUp(ctx, 100); e != nil {
		t.Fatal(e)
	}
	replayed, e := svc.Refresh(ctx, f.owner.RefreshToken, refreshOne, "native")
	if e != nil || replayed.AccessToken != rotated.AccessToken || replayed.RefreshToken != rotated.RefreshToken || replayed.FamilyID != rotated.FamilyID {
		t.Fatalf("replay after the outage did not return the exact answer: %v", e)
	}
	// The device moves on with the successor.
	next, e := svc.Refresh(ctx, rotated.RefreshToken, refreshTwo, "native")
	if e != nil {
		t.Fatal(e)
	}
	if e = f.store.DB.QueryRow(`SELECT count(*) FROM refresh_receipts WHERE family_id=$1`, f.owner.FamilyID).Scan(&receipts); e != nil || receipts != 1 {
		t.Fatalf("older receipts kept: %d %v", receipts, e)
	}
	// Now the predecessor's replay is reuse: the family is revoked.
	if _, e = svc.Refresh(ctx, f.owner.RefreshToken, refreshOne, "native"); !errors.Is(e, sessions.ErrReuse) {
		t.Fatalf("predecessor replay after the successor was used: %v", e)
	}
	if _, e = f.ids.Authenticate(ctx, next.AccessToken); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal("reuse left the family's access valid", e)
	}
}
