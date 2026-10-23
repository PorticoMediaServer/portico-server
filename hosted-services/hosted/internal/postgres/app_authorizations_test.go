package postgres_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"portico.local/hosted/internal/appauth"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func appRequest(server string) appauth.Request {
	// 24 bytes: the size a local server issues.
	return appauth.Request{ServerID: server, Challenge: base64.RawURLEncoding.EncodeToString(make([]byte, 24)), RedirectURI: "https://outside.example/finish?keep=yes", State: security.RandomID(""), AppName: "Outside Player", CodeChallenge: appauth.Challenge(strings.Repeat("v", 43))}
}
func appBrowser(t *testing.T, f fixture) identity.Session {
	t.Helper()
	s, e := f.ids.LoginMode(context.Background(), "owner", "StrongPass9", "browser")
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func appCode(t *testing.T, f fixture, s identity.Session, q appauth.Request) (appauth.Review, appauth.Exchange) {
	t.Helper()
	r, e := f.store.ReviewAppAuthorization(context.Background(), security.Hash(s.RefreshToken), q)
	if e != nil {
		t.Fatal(e)
	}
	ret, e := f.store.DecideAppAuthorization(context.Background(), security.Hash(s.RefreshToken), r.ID, appauth.Decision{AccountID: s.Account.ID, Approve: true})
	if e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(ret.Location)
	if e != nil || u.Query().Get("state") != q.State || u.Query().Get("keep") != "yes" {
		t.Fatal(ret, e)
	}
	return r, appauth.Exchange{Code: u.Query().Get("code"), Verifier: strings.Repeat("v", 43), RedirectURI: q.RedirectURI}
}
func appSigner(f fixture) func(identity.Account, string, string) (directory.SignedPolicy, error) {
	return func(a identity.Account, server, challenge string) (directory.SignedPolicy, error) {
		return f.dir.IssueIdentity(a.ID, a.Username, a.DisplayName, "", server, challenge, "")
	}
}
func TestAppAuthorizationExactScopePKCEAndSingleUse(t *testing.T) {
	f := setup(t)
	s := appBrowser(t, f)
	q := appRequest(f.server)
	_, ex := appCode(t, f, s, q)
	for _, bad := range []appauth.Exchange{{Code: ex.Code, Verifier: strings.Repeat("x", 43), RedirectURI: q.RedirectURI}, {Code: ex.Code, Verifier: ex.Verifier, RedirectURI: "https://outside.example/finish"}} {
		if _, e := f.store.ExchangeAppAuthorization(context.Background(), bad, appSigner(f)); !errors.Is(e, appauth.ErrGrant) {
			t.Fatal("wrong PKCE/return", e)
		}
	}
	grant, e := f.store.ExchangeAppAuthorization(context.Background(), ex, appSigner(f))
	if e != nil {
		t.Fatal(e)
	}
	raw, e := grant.Assertion.Verify(f.keys.RootPublicKey, "documents", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	var c directory.IdentityClaims
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	if c.Kind != directory.IdentityKind || c.AccountID != s.Account.ID || c.ServerID != q.ServerID || c.Challenge != q.Challenge || c.ExpiresAt.Sub(c.IssuedAt) != directory.IdentityLifetime {
		t.Fatal(c)
	}
	if _, e = f.store.ExchangeAppAuthorization(context.Background(), ex, appSigner(f)); !errors.Is(e, appauth.ErrGrant) {
		t.Fatal("replay", e)
	}
	var n int
	if e = f.store.DB.QueryRow(`SELECT count(*) FROM session_families WHERE account_id=$1`, s.Account.ID).Scan(&n); e != nil || n != 2 {
		t.Fatal("minted Hosted sessions", n, e)
	}
}
func TestAppAuthorizationConcurrentExchangeAndSigningRollback(t *testing.T) {
	f := setup(t)
	s := appBrowser(t, f)
	_, ex := appCode(t, f, s, appRequest(f.server))
	fail := func(identity.Account, string, string) (directory.SignedPolicy, error) {
		return directory.SignedPolicy{}, errors.New("signer unavailable")
	}
	if _, e := f.store.ExchangeAppAuthorization(context.Background(), ex, fail); e == nil {
		t.Fatal("signing failed open")
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.store.ExchangeAppAuthorization(context.Background(), ex, appSigner(f))
			if e == nil {
				wins.Add(1)
			} else if !errors.Is(e, appauth.ErrGrant) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("single-use race", wins.Load())
	}
}
func TestAppAuthorizationConsentFamilyAndQuota(t *testing.T) {
	f := setup(t)
	s := appBrowser(t, f)
	ctx := context.Background()
	q := appRequest(f.server)
	r, e := f.store.ReviewAppAuthorization(ctx, security.Hash(s.RefreshToken), q)
	if e != nil {
		t.Fatal(e)
	}
	other := appBrowser(t, f)
	for _, x := range []struct{ token, account string }{{other.RefreshToken, s.Account.ID}, {s.RefreshToken, "other-account"}} {
		if _, e = f.store.DecideAppAuthorization(ctx, security.Hash(x.token), r.ID, appauth.Decision{AccountID: x.account, Approve: true}); !errors.Is(e, appauth.ErrGrant) {
			t.Fatal("switched family/account approved", e)
		}
	}
	ret, e := f.store.DecideAppAuthorization(ctx, security.Hash(s.RefreshToken), r.ID, appauth.Decision{AccountID: s.Account.ID})
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(ret.Location)
	if u.Query().Get("error") != "access_denied" || u.Query().Has("code") {
		t.Fatal(ret)
	}
	if _, e = f.store.DecideAppAuthorization(ctx, security.Hash(s.RefreshToken), r.ID, appauth.Decision{AccountID: s.Account.ID, Approve: true}); e == nil {
		t.Fatal("revised denial")
	}
	for range 9 {
		if _, e = f.store.ReviewAppAuthorization(ctx, security.Hash(s.RefreshToken), q); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = f.store.ReviewAppAuthorization(ctx, security.Hash(s.RefreshToken), q); !errors.Is(e, identity.ErrRateLimited) {
		t.Fatal("review quota", e)
	}
}
func TestAppAuthorizationExpiryAndRevocation(t *testing.T) {
	for _, kind := range []string{"code", "request", "family", "account", "server"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			s := appBrowser(t, f)
			ctx := context.Background()
			r, ex := appCode(t, f, s, appRequest(f.server))
			var q, id string
			switch kind {
			case "code":
				q = `UPDATE app_authorizations SET code_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`
				id = r.ID
			case "request":
				q = `UPDATE app_authorizations SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`
				id = r.ID
			case "family":
				q = `UPDATE session_families SET revoked_at=clock_timestamp() WHERE id=$1`
				id = s.FamilyID
			case "account":
				q = `UPDATE accounts SET auth_epoch=auth_epoch+1 WHERE id=$1`
				id = s.Account.ID
			case "server":
				q = `UPDATE servers SET revoked_at=clock_timestamp(),owner_account_id=NULL,token_hash=NULL,token_ciphertext=NULL,base_url=NULL,endpoint_verified=false WHERE id=$1`
				id = f.server
			}
			if _, e := f.store.DB.Exec(q, id); e != nil {
				t.Fatal(e)
			}
			if _, e := f.store.ExchangeAppAuthorization(ctx, ex, appSigner(f)); !errors.Is(e, appauth.ErrGrant) {
				t.Fatal(kind, e)
			}
		})
	}
}
func TestAppAuthorizationHTTPConsentAndOutsideExchange(t *testing.T) {
	f := setup(t)
	s := appBrowser(t, f)
	q := appRequest(f.server)
	h := (&httpapi.Handler{AppAuthorizations: &httpapi.AppAuthorizations{Store: f.store, Directory: f.dir}, Directory: f.dir, Identity: f.ids, AccountOrigin: "https://web.getportico.tv", Origins: map[string]bool{"https://web.getportico.tv": true}}).Routes()
	call := func(path, origin string, body any, cookie bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", path, strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if cookie {
			r.Header.Set("X-CSRF-Token", strings.Repeat("c", 43))
			r.AddCookie(&http.Cookie{Name: "__Host-portico_refresh", Value: s.RefreshToken})
			r.AddCookie(&http.Cookie{Name: "__Host-portico_refresh_csrf", Value: strings.Repeat("c", 43)})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("/v1/app-authorizations", "https://outside.example", q, true)
	if w.Code != 403 {
		t.Fatal("outside consent", w.Code)
	}
	w = call("/v1/app-authorizations", "https://web.getportico.tv", q, true)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var r appauth.Review
	if e := json.Unmarshal(w.Body.Bytes(), &r); e != nil {
		t.Fatal(e)
	}
	w = call("/v1/app-authorizations/"+r.ID+"/decision", "https://web.getportico.tv", appauth.Decision{AccountID: r.AccountID, Approve: true}, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var ret appauth.Return
	json.Unmarshal(w.Body.Bytes(), &ret)
	u, _ := url.Parse(ret.Location)
	ex := appauth.Exchange{Code: u.Query().Get("code"), Verifier: strings.Repeat("v", 43), RedirectURI: q.RedirectURI}
	w = call("/v1/app-authorizations/token", "https://outside.example", ex, true)
	if w.Code == 200 {
		t.Fatal("cookie exchange")
	}
	w = call("/v1/app-authorizations/token", "https://outside.example", ex, false)
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal(w.Code, w.Body.String(), w.Header())
	}
	var grant map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &grant)
	if len(grant) != 1 || grant["assertion"] == nil || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("broader grant", grant, w.Header())
	}
}

// Review and decision look the browser session up by whatever cookie the
// caller sends, before anything is authenticated. That lookup must cost the
// same however many sessions exist.
func TestAppAuthorizationSessionLookupDoesNotScanEverySession(t *testing.T) {
	f := setup(t)
	s := appBrowser(t, f)
	if _, e := f.store.DB.Exec(`INSERT INTO session_families(id,account_id,mode,current_refresh_hash,account_epoch,absolute_expires_at,idle_expires_at) SELECT 'fam_bulk_'||n,$1,'browser',sha256(('bulk'||n)::bytea),0,now()+interval '1 day',now()+interval '1 day' FROM generate_series(1,4000) n`, s.Account.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.DB.Exec(`INSERT INTO refresh_tokens(token_hash,family_id,generation) SELECT current_refresh_hash,id,0 FROM session_families WHERE id LIKE 'fam_bulk_%'; ANALYZE session_families; ANALYZE refresh_tokens`); e != nil {
		t.Fatal(e)
	}
	for name, hash := range map[string][]byte{"unknown cookie": security.Hash("not a session"), "signed-in cookie": security.Hash(s.RefreshToken)} {
		var raw []byte
		if e := f.store.DB.QueryRow(`EXPLAIN (ANALYZE,FORMAT JSON) `+postgres.AppBrowserFamilySQLForTest, hash).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var nodes []struct{ Plan map[string]any }
		if e := json.Unmarshal(raw, &nodes); e != nil || len(nodes) != 1 {
			t.Fatal("bad explain", e)
		}
		examined := float64(0)
		var walk func(map[string]any)
		walk = func(node map[string]any) {
			loops, _ := node["Actual Loops"].(float64)
			for _, key := range []string{"Actual Rows", "Rows Removed by Filter", "Rows Removed by Join Filter"} {
				n, _ := node[key].(float64)
				examined += n * loops
			}
			children, _ := node["Plans"].([]any)
			for _, child := range children {
				walk(child.(map[string]any))
			}
		}
		walk(nodes[0].Plan)
		if examined > 8 {
			t.Fatalf("%s: lookup examined %.0f rows", name, examined)
		}
	}
	// The lookup still finds the session, and only its current cookie.
	if _, e := f.store.ReviewAppAuthorization(context.Background(), security.Hash(s.RefreshToken), appRequest(f.server)); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.ReviewAppAuthorization(context.Background(), security.Hash("not a session"), appRequest(f.server)); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal("unknown cookie", e)
	}
}
