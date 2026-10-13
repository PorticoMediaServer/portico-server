package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/wake"
	"portico.local/hosted/trust"
)

func database(t *testing.T) *postgres.Store {
	t.Helper()
	path := os.Getenv("PORTICO_TEST_DATABASE_URL_FILE")
	if path == "" {
		t.Skip("set PORTICO_TEST_DATABASE_URL_FILE to isolated PostgreSQL test URL file")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	dsn, e := url.Parse(strings.TrimSpace(string(raw)))
	if e != nil {
		t.Fatal("invalid test DSN")
	}
	if !strings.Contains(dsn.Path, "test") {
		t.Fatal("refusing database not named test")
	}
	ctx := context.Background()
	admin, e := postgres.Open(ctx, dsn.String())
	if e != nil {
		t.Fatal("test PostgreSQL unavailable")
	}
	schema := "test_" + strings.ToLower(strings.ReplaceAll(security.RandomID("")[:15], "-", "x"))
	schema = strings.ReplaceAll(schema, "_", "x")
	if _, e = admin.DB.ExecContext(ctx, `CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	// Register owned-schema cleanup before opening or initializing its store so
	// a rejected/failed current-format initialization cannot leak the fixture.
	var store *postgres.Store
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := admin.DB.ExecContext(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error("owned schema cleanup failed", err)
		}
		if err := admin.DB.Close(); err != nil {
			t.Error("fixture admin close failed", err)
		}
	})
	q := dsn.Query()
	q.Set("search_path", schema)
	dsn.RawQuery = q.Encode()
	store, e = postgres.Open(ctx, dsn.String())
	if e != nil {
		t.Fatal("open test schema")
	}
	t.Cleanup(func() {
		if err := store.DB.Close(); err != nil {
			t.Error("fixture store close failed", err)
		}
	})
	if e = store.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	return store
}

type fixture struct {
	store         *postgres.Store
	ids           *identity.Service
	dir           *directory.Service
	keys          *security.Keys
	owner, member identity.Session
	server        string
	token         string
}

func setup(t *testing.T) fixture {
	t.Helper()
	store := database(t)
	keys, e := fixtureKeys()
	if e != nil {
		t.Fatal(e)
	}
	ids := identity.New(store)
	owner, e := ids.Register(context.Background(), "owner", "Owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	member, e := ids.Register(context.Background(), "member", "Member", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	d := directory.New(store, keys)
	pub, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	id, e := claims.ServerID(pub)
	if e != nil {
		t.Fatal(e)
	}
	token := security.RandomID("hst_")
	sealed, e := keys.Seal(token, id)
	if e != nil {
		t.Fatal(e)
	}
	// Seed an already installed current-format claim. This fixture exercises
	// directory/session behavior, not the retired pre-claim enrollment protocol.
	// Keep the actual installed authority tuple, owner membership and wake intent.
	tx, e := store.DB.BeginTx(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO servers(id,owner_account_id,name,base_url,public_key,token_hash,token_ciphertext,claim_generation,credential_generation) VALUES($1,$2,'Local','http://127.0.0.1:19411',$3,$4,$5,1,1)`, []any{id, owner.Account.ID, base64.RawURLEncoding.EncodeToString(pub), security.Hash(token), sealed}},
		{`INSERT INTO claim_operations(id,server_id,account_id,public_key,local_generation,server_name,owner_profile_id,intent_digest,state,claim_generation,credential_generation,committed_token_hash,installed_at) VALUES('fixture_installed_claim',$1,$2,$3,0,'Local',$4,$5,'installed',1,1,$6,clock_timestamp())`, []any{id, owner.Account.ID, []byte(pub), owner.Profiles[0].ID, security.Hash("fixture installed claim"), security.Hash(token)}},
		{`INSERT INTO memberships(server_id,account_id,profile_id,role) VALUES($1,$2,$3,'owner')`, []any{id, owner.Account.ID, owner.Profiles[0].ID}},
		{`INSERT INTO wake_state(server_id,kind,desired_revision) VALUES($1,'policy',1)`, []any{id}},
	} {
		if _, e = tx.Exec(q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	f := fixture{store, ids, d, keys, owner, member, id, token}
	// A member under the retired Hosted membership model (rows the drop
	// migration removes), which account deletion and retirement still read.
	if _, e = store.DB.Exec(`INSERT INTO memberships(server_id,account_id,profile_id,role,allowed_libraries) VALUES($1,$2,$3,'member','{a,b}')`, id, member.Account.ID, member.Profiles[0].ID); e != nil {
		t.Fatal(e)
	}
	if _, e = store.DB.Exec(`UPDATE servers SET policy_revision=2 WHERE id=$1`, id); e != nil {
		t.Fatal(e)
	}
	if _, e = store.DB.Exec(`UPDATE wake_state SET desired_revision=2 WHERE server_id=$1`, id); e != nil {
		t.Fatal(e)
	}
	return f
}

// wakeEndpoint is the origin the fixture server registers when a test needs it
// to be wakeable. A server becomes a wake target only by proving a public https
// endpoint through RegisterEndpoint, so the fixture seeds exactly that evidence;
// Hosted never dials an unverified base URL.
const wakeEndpoint = "https://wake.fixture.invalid"

func (f fixture) verifyEndpoint(t *testing.T) string {
	t.Helper()
	tx, e := f.store.DB.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`INSERT INTO current_server_endpoints(server_id,operation_id,account_id,claim_generation,credential_generation,origin,proof_digest) VALUES($1,'fixture_installed_claim',$2,1,1,$3,decode(repeat('00',32),'hex'))`, f.server, f.owner.Account.ID, wakeEndpoint); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(`UPDATE servers SET base_url=$2,endpoint_verified=true WHERE id=$1`, f.server, wakeEndpoint); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	return wakeEndpoint
}

// endpointRewrite sends a request addressed to the verified public endpoint to a
// local test receiver instead. The worker still builds and signs exactly the
// request it would send in production; only the transport is redirected.
type endpointRewrite struct{ target *url.URL }

func (r endpointRewrite) RoundTrip(request *http.Request) (*http.Response, error) {
	routed := request.Clone(request.Context())
	routed.Host = request.URL.Host
	routed.URL.Scheme = r.target.Scheme
	routed.URL.Host = r.target.Host
	return http.DefaultTransport.RoundTrip(routed)
}
func TestPasswordSessionsPersistAndRevoke(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, e := f.ids.Login(ctx, "owner", "wrong"); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal(e)
	}
	logged, e := f.ids.Login(ctx, "OWNER", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = identity.New(f.store).Authenticate(ctx, logged.AccessToken); e != nil {
		t.Fatal(e)
	}
	if e = f.ids.Revoke(ctx, logged.AccessToken); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ids.Authenticate(ctx, logged.AccessToken); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal(e)
	}
	var count int
	if e = f.store.DB.QueryRow(`SELECT count(*) FROM account_sessions WHERE token_hash=$1`, []byte(f.owner.AccessToken)).Scan(&count); e != nil || count != 0 {
		t.Fatal("raw token stored")
	}
}
func equal(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }
func TestEarlyAckAndLeaseCompletionCannotLoseAcknowledgement(t *testing.T) {
	f := setup(t)
	f.verifyEndpoint(t)
	ctx := context.Background()
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	ack := func(revision int64) {
		t.Helper()
		if _, e := f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{WakeRevision: revision}); e != nil {
			t.Fatal(e)
		}
	}
	ack(2)
	if _, ok, e := f.store.Claim(ctx, "before", time.Now()); e != nil || ok {
		t.Fatal(ok, e)
	}
	// Something new to wake the server for (an erasure, a rotated label).
	if _, e = f.store.DB.Exec(`UPDATE wake_state SET desired_revision=3,next_attempt_at=now() WHERE server_id=$1`, f.server); e != nil {
		t.Fatal(e)
	}
	w, ok, e := f.store.Claim(ctx, "inflight", time.Now())
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	ack(3)
	if e = f.store.Finish(ctx, w, 429, time.Now().Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	ack(1)
	var desired, acked int64
	if e = f.store.DB.QueryRow(`SELECT desired_revision,acked_revision FROM wake_state WHERE server_id=$1`, f.server).Scan(&desired, &acked); e != nil || desired != 3 || acked != 3 {
		t.Fatal(desired, acked, e)
	}
	if _, ok, e = f.store.Claim(ctx, "after", time.Now().Add(time.Hour)); e != nil || ok {
		t.Fatal(ok, e)
	}
}
func TestWake429BackoffAndRestartDurability(t *testing.T) {
	f := setup(t)
	var addressed string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addressed = "https://" + r.Host + r.URL.Path
		var signed directory.SignedPolicy
		if e := json.NewDecoder(r.Body).Decode(&signed); e != nil {
			t.Error(e)
		}
		raw, _ := base64.RawURLEncoding.DecodeString(signed.Payload)
		sig, _ := base64.RawURLEncoding.DecodeString(signed.Signature)
		if !ed25519.Verify(f.keys.PrivateKey.Public().(ed25519.PublicKey), raw, sig) {
			t.Error("invalid wake signature")
		}
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	}))
	defer receiver.Close()
	origin := f.verifyEndpoint(t)
	local, e := url.Parse(receiver.URL)
	if e != nil {
		t.Fatal(e)
	}
	worker := wake.Worker{Repo: f.store, Keys: f.keys, Client: &http.Client{Transport: endpointRewrite{local}}}
	before := time.Now()
	ok, e := worker.Once(context.Background())
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	// The delivery must be addressed to the endpoint the server actually proved.
	if addressed != origin+"/v1/hosted/wake" {
		t.Fatal("wake was not addressed to the verified endpoint", addressed)
	}
	var next time.Time
	var status int
	if e = f.store.DB.QueryRow(`SELECT next_attempt_at,last_status FROM wake_state WHERE server_id=$1`, f.server).Scan(&next, &status); e != nil || status != 429 || next.Before(before.Add(120*time.Second)) {
		t.Fatal(next, status, e)
	}
	reopened := &postgres.Store{DB: f.store.DB}
	if _, ok, e = reopened.Claim(context.Background(), "restart", time.Now()); e != nil || ok {
		t.Fatal(ok, e)
	}
}
func TestHTTPRealDatabaseJourneyAndOriginBoundaries(t *testing.T) {
	f := setup(t)
	handler := (&httpapi.Handler{Identity: f.ids, Directory: f.dir, Origins: map[string]bool{"http://127.0.0.1:19412": true}}).Routes()
	server := httptest.NewServer(handler)
	defer server.Close()
	call := func(method, path, token, origin string, body any) (int, []byte) {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		res, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, raw
	}
	if code, _ := call("GET", "/v1/me", f.owner.AccessToken, "https://evil.invalid", nil); code != 403 {
		t.Fatal(code)
	}
	push := map[string]any{"after": 0, "through": 1, "full": true, "members": []map[string]any{{"accountId": f.member.Account.ID, "role": "member", "active": true, "consent": f.consent(t, f.member.Account.ID)}}}
	if code, _ := call("POST", "/v1/servers/"+f.server+"/members", f.owner.AccessToken, "", push); code != 403 {
		t.Fatal("an account token pushed members", code)
	}
	if code, body := call("POST", "/v1/servers/"+f.server+"/members", f.token, "", push); code != 200 {
		t.Fatal(code, string(body))
	}
	// The member finds the server, with a version and a watch token; the
	// version check answers from memory; a matching ETag is a 304.
	if e := f.dir.RefreshServerListVersions(context.Background()); e != nil {
		t.Fatal(e)
	}
	code, body := call("GET", "/v1/servers", f.member.AccessToken, "", nil)
	var list httpapi.ServerList
	if code != 200 || json.Unmarshal(body, &list) != nil || len(list.Items) != 1 || list.Items[0].ID != f.server || list.Version == "0" || list.Watch == "" {
		t.Fatal(code, string(body))
	}
	check := func(watch, held string) int {
		res, e := http.Get(server.URL + "/v1/server-list/" + watch + "/" + held)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := check(list.Watch, list.Version); code != 304 {
		t.Fatal("unchanged list not a 304", code)
	}
	if code := check(list.Watch, "0"); code != 200 {
		t.Fatal("changed list not a 200", code)
	}
	if code := check("bm90LWEtdG9rZW4.AAAA", "0"); code != 404 {
		t.Fatal("forged watch token", code)
	}
	// An identity assertion for one server, for the member to present there.
	if code, body = call("POST", "/v1/servers/"+f.server+"/identity", f.member.AccessToken, "", map[string]string{"challenge": testChallenge}); code != 200 {
		t.Fatal(code, string(body))
	}
	if code, _ = call("POST", "/v1/servers/"+f.server+"/identity", "", "", map[string]string{"challenge": testChallenge}); code != 401 {
		t.Fatal("identity without a session", code)
	}
	if code, _ = call("GET", "/v1/me?token=secret", f.owner.AccessToken, "", nil); code != 400 {
		t.Fatal(code)
	}
}

// A check-in carries no policy (Spec — Hosted at Scale). It sets the cadence,
// acknowledges the wake it answers (never past what was sent, never
// backwards) and says whether Hosted's member index agrees with the server.
func TestCheckInAcknowledgesWakesMonotonically(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`INSERT INTO wake_state(server_id,kind,desired_revision,acked_revision) VALUES($1,'policy',5,0) ON CONFLICT(server_id,kind) DO UPDATE SET desired_revision=5,acked_revision=0`, f.server); e != nil {
		t.Fatal(e)
	}
	result, e := f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{WakeRevision: 5})
	if e != nil || result.HeartbeatAfterSeconds != int(directory.CheckInUnreachable/time.Second) || result.CleanupPending {
		t.Fatal(result, e)
	}
	// A server Hosted cannot wake checks in daily; its presence window covers that with slack.
	if remaining := time.Until(result.PresenceExpiresAt); remaining < 47*time.Hour || remaining > 49*time.Hour {
		t.Fatal(remaining)
	}
	for _, report := range []int64{3, 999} {
		if _, e = f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{WakeRevision: report}); e != nil {
			t.Fatal(e)
		}
		var ack int64
		if e = f.store.DB.QueryRow(`SELECT acked_revision FROM wake_state WHERE server_id=$1`, f.server).Scan(&ack); e != nil || ack != 5 {
			t.Fatal("wake acknowledgement", report, ack, e)
		}
	}
	if _, e = f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{WakeRevision: -1}); !errors.Is(e, directory.ErrInvalid) {
		t.Fatal(e)
	}
}
func TestStaleWakeFinisherDoesNotOverwriteNewLease(t *testing.T) {
	f := setup(t)
	f.verifyEndpoint(t)
	ctx := context.Background()
	first, ok, e := f.store.Claim(ctx, "old", time.Now())
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	second, ok, e := f.store.Claim(ctx, "new", time.Now().Add(time.Minute))
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	if e = f.store.Finish(ctx, first, 500, time.Now().Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	var lease string
	if e = f.store.DB.QueryRow(`SELECT lease_token FROM wake_state WHERE server_id=$1`, f.server).Scan(&lease); e != nil || lease != second.LeaseToken {
		t.Fatal(lease, e)
	}
}
func TestDatabaseOutageIsNotBadPassword(t *testing.T) {
	f := setup(t)
	f.store.DB.Close()
	if _, e := f.ids.Login(context.Background(), "owner", "StrongPass9"); e == nil || errors.Is(e, identity.ErrCredentials) {
		t.Fatal("database failure misclassified as invalid credentials")
	}
}

func fixtureKeys() (*security.Keys, error) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	seal := make([]byte, 32)
	if _, err = rand.Read(seal); err != nil {
		return nil, err
	}
	rootPublic, root, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	id := security.RandomID("test_")
	cert, err := trust.Certify(root, private.Public().(ed25519.PublicKey), id, "documents", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour), 1, nil)
	if err != nil {
		return nil, err
	}
	return &security.Keys{RootPublicKey: rootPublic, Certificate: cert, PrivateKey: private, SealKey: seal, KeyID: id}, nil
}
