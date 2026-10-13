package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"portico.local/apikit"
	"portico.local/server/internal/access"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

func issueCredential(t *testing.T, f administrationFixture, path, session, body string) access.IssuedCredential {
	t.Helper()
	w := f.call("POST", path, session, body)
	if w.Code != 201 {
		t.Fatalf("create %s: %d %s", path, w.Code, w.Body)
	}
	var out access.IssuedCredential
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func registryErrorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Error.Code
}

func TestAPIKeyIsShownOnceAndAuthenticatesWithinItsScopes(t *testing.T) {
	f := administrationFixtureNew(t)
	request := `{"operationId":"key-create-1","name":"Automation","scopes":["read"]}`
	key := issueCredential(t, f, "/v1/admin/api-keys", f.owner, request)
	if !strings.HasPrefix(key.Secret, identity.APIKeyPrefix) || key.Credential.State != "active" || key.Credential.Kind != "key" || len(key.Credential.Scopes) != 1 {
		t.Fatalf("issued key: %+v", key.Credential)
	}
	if strings.Contains(key.Credential.Hint, key.Secret) || len(key.Credential.Hint) != 6 {
		t.Fatalf("hint reveals too much: %q", key.Credential.Hint)
	}
	// The same request again names the key and cannot show the secret.
	w := f.call("POST", "/v1/admin/api-keys", f.owner, request)
	if w.Code != 201 || strings.Contains(w.Body.String(), key.Secret) || !strings.Contains(w.Body.String(), key.Credential.ID) {
		t.Fatalf("replay: %d %s", w.Code, w.Body)
	}
	// Only the digest is stored.
	var stored string
	if err := f.deps.DB.QueryRow(`SELECT secret_hash FROM api_credentials WHERE id=?`, key.Credential.ID).Scan(&stored); err != nil || stored != identity.Digest(key.Secret) || strings.Contains(stored, key.Secret) {
		t.Fatalf("stored secret: %q %v", stored, err)
	}
	var plain int
	if err := f.deps.DB.QueryRow(`SELECT (SELECT count(*) FROM api_credentials WHERE instr(secret_hash||hint||name,?)>0)+(SELECT count(*) FROM authorization_family_tokens WHERE token_hash=?)+(SELECT count(*) FROM access_receipts WHERE instr(body,?)>0)`, key.Secret, key.Secret, key.Secret).Scan(&plain); err != nil || plain != 0 {
		t.Fatalf("the secret is stored somewhere: %d %v", plain, err)
	}
	// A read works and carries the limit headers.
	w = f.call("GET", "/v1/me", key.Secret, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"accountId":"owner"`) {
		t.Fatalf("read with a read key: %d %s", w.Code, w.Body)
	}
	if w.Header().Get("RateLimit-Policy") == "" || w.Header().Get("RateLimit") == "" {
		t.Fatalf("no limit headers: %v", w.Header())
	}
	// Scopes it was not given are refused, on hand-registered and typed routes.
	for _, refused := range []struct{ method, path, body, code string }{
		{"PATCH", "/v1/preferences", `{}`, "insufficient_scope"},
		{"POST", "/v1/libraries/abc/scans", ``, "insufficient_scope"},
		{"GET", "/v1/admin/access/members", ``, "insufficient_scope"},
		{"GET", "/v1/admin/backups", ``, "insufficient_scope"},
		{"POST", "/v1/queues", `{}`, "insufficient_scope"},
		// Session-only whatever the scopes.
		{"GET", "/v1/admin/api-keys", ``, "session_required"},
		{"POST", "/v1/admin/api-keys", `{"operationId":"key-create-2","name":"Wider","scopes":["admin"]}`, "session_required"},
		{"POST", "/v1/me/access-tokens", `{"operationId":"token-create-9","name":"Wider","scopes":["admin"]}`, "session_required"},
		{"DELETE", "/v1/sessions/current", ``, "session_required"},
		{"POST", "/v1/direct/profile-switch", `{}`, "session_required"},
		{"POST", "/v1/quick-connect/decision", `{}`, "session_required"},
		{"POST", "/v1/direct/password", `{}`, "session_required"},
		{"GET", "/v1/devices", ``, "session_required"},
		{"POST", "/v1/auth/refresh", `{}`, "session_required"},
	} {
		w = f.call(refused.method, refused.path, key.Secret, refused.body)
		if w.Code != 403 || registryErrorCode(t, w) != refused.code {
			t.Fatalf("%s %s with a read key: %d %s", refused.method, refused.path, w.Code, w.Body)
		}
	}
	// It is listed with a last-used time only after the use is written; the
	// list never carries the secret.
	w = f.call("GET", "/v1/admin/api-keys", f.owner, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Automation"`) || strings.Contains(w.Body.String(), key.Secret) {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	// Revoked is refused at once, and revoking again is not an error.
	for range 2 {
		w = f.call("DELETE", "/v1/admin/api-keys/"+key.Credential.ID, f.owner, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"revoked"`) {
			t.Fatalf("revoke: %d %s", w.Code, w.Body)
		}
	}
	if w = f.call("GET", "/v1/me", key.Secret, ""); w.Code != 401 {
		t.Fatalf("revoked key still authenticates: %d %s", w.Code, w.Body)
	}
}

func TestCredentialScopeNeverRaisesTheMemberBehindIt(t *testing.T) {
	f := administrationFixtureNew(t)
	// The owner's admin key administers.
	key := issueCredential(t, f, "/v1/admin/api-keys", f.owner, `{"operationId":"key-admin-1","name":"Console script","scopes":["admin"]}`)
	if w := f.call("GET", "/v1/admin/access/members", key.Secret, ""); w.Code != 200 {
		t.Fatalf("admin key on an admin route: %d %s", w.Code, w.Body)
	}
	// A member may hold every scope and is still a member.
	token := issueCredential(t, f, "/v1/me/access-tokens", f.member, `{"operationId":"token-create-1","name":"My script","scopes":["read","playback","library","admin"]}`)
	if !strings.HasPrefix(token.Secret, identity.AccessTokenPrefix) || token.Credential.Member.AccountID != "member" {
		t.Fatalf("issued token: %+v", token.Credential)
	}
	if w := f.call("GET", "/v1/me", token.Secret, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"accountId":"member"`) {
		t.Fatalf("member token read: %d %s", w.Code, w.Body)
	}
	for _, path := range []string{"/v1/admin/access/members", "/v1/admin/backups", "/v1/admin/logs"} {
		if w := f.call("GET", path, token.Secret, ""); w.Code != 403 {
			t.Fatalf("member token reached %s: %d %s", path, w.Code, w.Body)
		}
	}
	// A member cannot make an owner key, see other members' tokens, or end one
	// that is not theirs.
	if w := f.call("POST", "/v1/admin/api-keys", f.member, `{"operationId":"key-member-1","name":"Nope","scopes":["read"]}`); w.Code != 403 {
		t.Fatalf("member made an API key: %d %s", w.Code, w.Body)
	}
	if w := f.call("GET", "/v1/admin/access-tokens", f.member, ""); w.Code != 403 {
		t.Fatalf("member listed everyone's tokens: %d %s", w.Code, w.Body)
	}
	other := issueCredential(t, f, "/v1/me/access-tokens", f.admin, `{"operationId":"token-create-2","name":"Admin script","scopes":["read"]}`)
	if w := f.call("DELETE", "/v1/me/access-tokens/"+other.Credential.ID, f.member, ""); w.Code != 404 {
		t.Fatalf("member ended another member's token: %d %s", w.Code, w.Body)
	}
	if w := f.call("GET", "/v1/me/access-tokens", f.member, ""); w.Code != 200 || strings.Contains(w.Body.String(), other.Credential.ID) || !strings.Contains(w.Body.String(), token.Credential.ID) {
		t.Fatalf("member's own list: %d %s", w.Code, w.Body)
	}
	// The owner sees and can end any member's token.
	if w := f.call("GET", "/v1/admin/access-tokens", f.owner, ""); w.Code != 200 || !strings.Contains(w.Body.String(), token.Credential.ID) || !strings.Contains(w.Body.String(), other.Credential.ID) {
		t.Fatalf("owner's list of tokens: %d %s", w.Code, w.Body)
	}
	if w := f.call("DELETE", "/v1/admin/access-tokens/"+token.Credential.ID, f.owner, ""); w.Code != 200 {
		t.Fatalf("owner ended a member's token: %d %s", w.Code, w.Body)
	}
	if w := f.call("GET", "/v1/me", token.Secret, ""); w.Code != 401 {
		t.Fatalf("ended token still works: %d", w.Code)
	}
}

func TestCredentialEndsWithTheMembersSessionsAndItsOwnExpiry(t *testing.T) {
	f := administrationFixtureNew(t)
	expiring := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	token := issueCredential(t, f, "/v1/me/access-tokens", f.member, `{"operationId":"token-expiry-1","name":"Short","scopes":["read"],"expiresAt":"`+expiring+`"}`)
	lasting := issueCredential(t, f, "/v1/me/access-tokens", f.member, `{"operationId":"token-expiry-2","name":"Long","scopes":["read"]}`)
	if token.Credential.ExpiresAt == "" || lasting.Credential.ExpiresAt != "" {
		t.Fatalf("expiry: %q %q", token.Credential.ExpiresAt, lasting.Credential.ExpiresAt)
	}
	for _, bad := range []string{`"2001-01-01T00:00:00Z"`, `"soon"`, `"2999-01-01T00:00:00Z"`} {
		if w := f.call("POST", "/v1/me/access-tokens", f.member, `{"operationId":"token-expiry-bad","name":"Bad","scopes":["read"],"expiresAt":`+bad+`}`); w.Code != 400 {
			t.Fatalf("expiry %s accepted: %d %s", bad, w.Code, w.Body)
		}
	}
	for _, bad := range []string{`[]`, `["read","read"]`, `["everything"]`, `["session"]`, `["open"]`} {
		if w := f.call("POST", "/v1/me/access-tokens", f.member, `{"operationId":"token-scope-bad","name":"Bad","scopes":`+bad+`}`); w.Code != 400 {
			t.Fatalf("scopes %s accepted: %d %s", bad, w.Code, w.Body)
		}
	}
	// Past its expiry the store refuses it; so does the family's own horizon.
	store := f.deps.Access.Access
	real := store.Now
	store.Now = func() time.Time { return time.Now().Add(3 * time.Hour) }
	dbwork.BumpAuthority()
	if w := f.call("GET", "/v1/me", token.Secret, ""); w.Code != 401 {
		t.Fatalf("expired token works: %d %s", w.Code, w.Body)
	}
	store.Now = real
	if _, err := f.deps.DB.Exec(`UPDATE authorization_session_families SET authorization_horizon='2001-01-01T00:00:00Z' WHERE id=(SELECT family_id FROM api_credentials WHERE id=?)`, token.Credential.ID); err != nil {
		t.Fatal(err)
	}
	dbwork.BumpAuthority()
	if w := f.call("GET", "/v1/me", token.Secret, ""); w.Code != 401 {
		t.Fatalf("token outlived its family horizon: %d %s", w.Code, w.Body)
	}
	// Whatever ends every session of the member ends their tokens.
	if w := f.call("GET", "/v1/me", lasting.Secret, ""); w.Code != 200 {
		t.Fatalf("lasting token: %d %s", w.Code, w.Body)
	}
	if _, err := f.deps.DB.Exec(`UPDATE accounts SET epoch=epoch+1 WHERE id='member'`); err != nil {
		t.Fatal(err)
	}
	dbwork.BumpAuthority()
	if w := f.call("GET", "/v1/me", lasting.Secret, ""); w.Code != 401 {
		t.Fatalf("token survived the member being signed out everywhere: %d %s", w.Code, w.Body)
	}
	w := f.call("GET", "/v1/admin/access-tokens", f.owner, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"ended"`) {
		t.Fatalf("ended token is not listed as ended: %d %s", w.Code, w.Body)
	}
}

func TestCredentialIsNeverAnUnscopedSession(t *testing.T) {
	f := administrationFixtureNew(t)
	key := issueCredential(t, f, "/v1/admin/api-keys", f.owner, `{"operationId":"key-shape-1","name":"Shape","scopes":["read"]}`)
	// Authenticated anywhere but through the gate, the secret is not a session.
	if _, err := f.deps.Identity.Authenticate(key.Secret); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("a key authenticated without the gate: %v", err)
	}
	// A stamp for one credential does not vouch for another secret.
	stamped := identity.WithAPICredential(context.Background(), identity.APICredential{ID: "other", Digest: identity.Digest("pk.someone-elses-secret-that-is-long-enough-000")})
	if _, err := f.deps.Identity.AuthenticateContext(stamped, key.Secret); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("a foreign stamp admitted the key: %v", err)
	}
	if err := f.deps.Identity.LogoutToken(context.Background(), key.Secret); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("a key signed itself out: %v", err)
	}
	// Two Authorization headers are never read two ways.
	r := httptest.NewRequest("GET", "/v1/me", nil)
	r.Header.Add("Authorization", "Bearer "+f.owner)
	r.Header.Add("Authorization", "Bearer "+key.Secret)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("ambiguous credentials were accepted: %d %s", w.Code, w.Body)
	}
	// Unknown and malformed secrets are 401, never treated as a session.
	for _, secret := range []string{"pk.unknown-unknown-unknown-unknown-unknown-unknown", "pat.", "x.y", strings.Repeat("a", 300) + "."} {
		if w = f.call("GET", "/v1/me", secret, ""); w.Code != 401 {
			t.Fatalf("%q: %d %s", secret, w.Code, w.Body)
		}
	}
	// A session token cannot be mistaken for a credential.
	if identity.CredentialShaped(f.owner) || !identity.CredentialShaped(key.Secret) {
		t.Fatal("credential shape is not distinct from a session token")
	}
	// A route nothing has classified is closed to credentials.
	if legacyRouteScope("GET /v1/not-yet-classified") != apikit.ScopeSession {
		t.Fatal("an unclassified route is open to credentials")
	}
}

func TestCredentialWaitsForDeviceApprovalWhenTheOwnerRequiresIt(t *testing.T) {
	f := administrationFixtureNew(t)
	if err := f.deps.Identity.SetDeviceApprovalPolicy(identity.DeviceApprovalOwner); err != nil {
		t.Fatal(err)
	}
	token := issueCredential(t, f, "/v1/me/access-tokens", f.member, `{"operationId":"token-approval-1","name":"Waiting","scopes":["read"]}`)
	if token.Credential.State != "awaiting_approval" {
		t.Fatalf("state: %q", token.Credential.State)
	}
	if w := f.call("GET", "/v1/me", token.Secret, ""); w.Code != 401 {
		t.Fatalf("an unapproved token works: %d %s", w.Code, w.Body)
	}
	if _, err := f.deps.DB.Exec(`UPDATE identity_devices SET approval_state='approved' WHERE installation_id=?`, "api-credential-"+token.Credential.ID); err != nil {
		t.Fatal(err)
	}
	dbwork.BumpAuthority()
	if w := f.call("GET", "/v1/me", token.Secret, ""); w.Code != 200 {
		t.Fatalf("an approved token does not work: %d %s", w.Code, w.Body)
	}
	// The owner's own key is not held for the owner's own approval.
	key := issueCredential(t, f, "/v1/admin/api-keys", f.owner, `{"operationId":"key-approval-1","name":"Mine","scopes":["read"]}`)
	if key.Credential.State != "active" {
		t.Fatalf("owner key state: %q", key.Credential.State)
	}
}

func TestCredentialGateHoldsOneCredentialToItsRateAndShare(t *testing.T) {
	gate := newCredentialGate()
	now := time.Now()
	for i := range 5 {
		if _, _, ok := gate.take("a", 5, now); !ok {
			t.Fatalf("request %d refused inside the limit", i)
		}
	}
	if _, wait, ok := gate.take("a", 5, now); ok || wait <= 0 || wait > 13*time.Second {
		t.Fatalf("sixth request: ok=%v wait=%v", ok, wait)
	}
	if _, _, ok := gate.take("b", 5, now); !ok {
		t.Fatal("one credential's spending refused another")
	}
	if _, _, ok := gate.take("a", 5, now.Add(13*time.Second)); !ok {
		t.Fatal("the bucket did not refill")
	}
	for i := range credentialConcurrency {
		if !gate.enter("a", false) {
			t.Fatalf("request %d refused inside the share", i)
		}
	}
	if gate.enter("a", false) {
		t.Fatal("a credential exceeded its share of the server")
	}
	if !gate.enter("b", false) {
		t.Fatal("one credential's share refused another")
	}
	gate.leave("a", false)
	if !gate.enter("a", false) {
		t.Fatal("a released slot was not returned")
	}
	// Credentials together never take more than their global share.
	wide := newCredentialGate()
	admitted := 0
	for i := range 100 {
		if wide.enter(string(rune('a'+i%26))+string(rune('a'+i/26)), false) {
			admitted++
		}
	}
	if admitted != credentialConcurrencyGlobal {
		t.Fatalf("credentials together hold %d slots, want %d", admitted, credentialConcurrencyGlobal)
	}
	for range credentialStreams {
		if !gate.enter("a", true) {
			t.Fatal("a stream was refused inside the share")
		}
	}
	if gate.enter("a", true) {
		t.Fatal("a credential exceeded its share of streams")
	}
}

func TestCredentialRateLimitAnswers429WithRetryAfter(t *testing.T) {
	f := administrationFixtureNew(t)
	key := issueCredential(t, f, "/v1/admin/api-keys", f.owner, `{"operationId":"key-rate-1","name":"Busy","scopes":["read"]}`)
	if w := f.call("GET", "/v1/me", key.Secret, ""); w.Code != 200 {
		t.Fatalf("first request: %d %s", w.Code, w.Body)
	}
	f.deps.credentials.mu.Lock()
	f.deps.credentials.buckets[key.Credential.ID].tokens = 0
	f.deps.credentials.buckets[key.Credential.ID].at = time.Now()
	f.deps.credentials.mu.Unlock()
	w := f.call("GET", "/v1/me", key.Secret, "")
	if w.Code != 429 || registryErrorCode(t, w) != "rate_limited" || w.Header().Get("Retry-After") == "" {
		t.Fatalf("over the limit: %d %v %s", w.Code, w.Header(), w.Body)
	}
	// The owner's own session is not charged to the key.
	if w = f.call("GET", "/v1/me", f.owner, ""); w.Code != 200 {
		t.Fatalf("a busy key refused its owner's session: %d", w.Code)
	}
}

// Every route this package registers must say what a key or token needs to use
// it: on the typed registry, or in routeScopes. A missing entry is refused to
// credentials at run time; this is what makes someone decide.
func TestEveryRouteDeclaresACredentialScope(t *testing.T) {
	registry, err := FoundationRegistry(Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	typed := map[string]apikit.Scope{}
	for _, route := range registry.Definitions() {
		if !apikit.ValidScope(route.Requires) {
			t.Errorf("%s %s declares no credential scope", route.Method, route.Path)
		}
		typed[route.Method+" "+route.Path] = route.Requires
		if legacy, ok := routeScopes[muxPattern(route.Method, route.Path)]; ok && legacy != route.Requires && !v1SharedPatterns[muxPattern(route.Method, route.Path)] {
			t.Errorf("%s %s: routeScopes says %s, the registry %s", route.Method, route.Path, legacy, route.Requires)
		}
	}
	for pattern := range v1SharedPatterns {
		if routeScopes[pattern] == "" {
			t.Errorf("shared pattern %s has no scope for its hand-written handler", pattern)
		}
	}
	literal := regexp.MustCompile(`"((?:GET|POST|PUT|PATCH|DELETE|HEAD) /[^"]*)"`)
	missing := map[string]bool{}
	check := func(route string) {
		if strings.HasSuffix(route, "/") && route != "GET /v1/admin/" {
			return // a prefix a route is concatenated onto; the whole routes are listed
		}
		if _, ok := routeScopes[route]; ok {
			return
		}
		if _, ok := typed[route]; ok {
			return
		}
		missing[route] = true
	}
	for _, dir := range []string{".", "../networking"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "route_scopes.go" {
				continue
			}
			data, err := os.ReadFile(dir + "/" + entry.Name())
			if err != nil {
				t.Fatal(err)
			}
			if developmentOnlySource(data) {
				continue
			}
			for _, match := range literal.FindAllStringSubmatch(string(data), -1) {
				check(match[1])
			}
		}
	}
	for _, route := range composedRoutes {
		check(route)
	}
	for route := range routeLanes {
		if strings.Contains(route, " /") {
			check(route)
		}
	}
	for route := range missing {
		t.Errorf("route has no credential scope: %s", route)
	}
	for route, scope := range routeScopes {
		if !apikit.ValidScope(scope) {
			t.Errorf("%s has an unknown scope %q", route, scope)
		}
	}
}

func TestScopesDoNotIncludeOneAnotherExceptAdmin(t *testing.T) {
	read := []apikit.Scope{apikit.ScopeRead}
	if !apikit.ScopeAllows(read, apikit.ScopeRead) || !apikit.ScopeAllows(read, apikit.ScopeOpen) {
		t.Fatal("read does not allow read")
	}
	for _, need := range []apikit.Scope{apikit.ScopePlayback, apikit.ScopeLibrary, apikit.ScopeAdmin, apikit.ScopeSession, "", "nonsense"} {
		if apikit.ScopeAllows(read, need) {
			t.Fatalf("read allows %q", need)
		}
	}
	admin := []apikit.Scope{apikit.ScopeAdmin}
	for _, need := range []apikit.Scope{apikit.ScopeRead, apikit.ScopePlayback, apikit.ScopeLibrary, apikit.ScopeAdmin} {
		if !apikit.ScopeAllows(admin, need) {
			t.Fatalf("admin does not allow %q", need)
		}
	}
	if apikit.ScopeAllows(admin, apikit.ScopeSession) || apikit.ScopeAllows(nil, apikit.ScopeRead) {
		t.Fatal("session-only or empty grants were allowed")
	}
	if apikit.Grantable(apikit.ScopeSession) || apikit.Grantable(apikit.ScopeOpen) {
		t.Fatal("session or open can be granted")
	}
	var _ = http.MethodGet
}
