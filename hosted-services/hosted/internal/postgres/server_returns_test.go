package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
	"portico.local/hosted/trust"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type returnFixture struct {
	claimJourney
	handler      http.Handler
	directory    *directory.Service
	keys         *security.Keys
	start        serverreturn.Start
	accountToken string
	completion   json.RawMessage
}

func registeredReturnFixture(t *testing.T) returnFixture {
	t.Helper()
	f := currentClaimJourney(t)
	if path := os.Getenv("PORTICO_RETURN_CONSUMER_INPUT"); path != "" {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		var input struct{ ServerID, PublicKey, PrivateKey, OperationID, Challenge, InstallationID string }
		if json.Unmarshal(raw, &input) != nil {
			t.Fatal("bad fixture input")
		}
		key, e := base64.RawURLEncoding.DecodeString(input.PrivateKey)
		if e != nil || len(key) != 64 {
			t.Fatal("bad fixture key")
		}
		f.key = ed25519.PrivateKey(key)
		f.input.ServerID = input.ServerID
		f.input.PublicKey = input.PublicKey
		f.input.OperationID = input.OperationID
	}
	ctx := context.Background()
	a := journeyApproval(t, f)
	raw, sig := journeyProof(t, f, a, claims.Finalize)
	commit, e := f.service.Finalize(ctx, raw, sig)
	if e != nil {
		t.Fatal(e)
	}
	raw, sig = journeyProof(t, f, a, claims.Result)
	result, e := f.service.Result(ctx, raw, sig)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.Acknowledge(ctx, result.ServerCredential, commit); e != nil {
		t.Fatal(e)
	}
	const origin = "https://server.example:32500"
	_, e = f.store.DB.Exec(`INSERT INTO networking_route_state(server_id,operation_id,claim_generation,credential_generation,generation,observation_digest,remote_enabled) VALUES($1,$2,1,1,1,$3,true);`, f.input.ServerID, f.input.OperationID, bytes.Repeat([]byte{1}, 32))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`INSERT INTO networking_route_candidates(server_id,operation_id,generation,origin,class,state,verified_at) VALUES($1,$2,1,$3,'public','verified',clock_timestamp())`, f.input.ServerID, f.input.OperationID, origin); e != nil {
		t.Fatal(e)
	}
	keys, e := fixtureKeys()
	if e != nil {
		t.Fatal(e)
	}
	dir := directory.New(f.store, keys)
	handler := (&httpapi.Handler{Directory: dir, ServerReturns: &httpapi.ServerReturns{Store: f.store, Directory: dir}}).Routes()
	q := serverreturn.Start{ServerID: f.input.ServerID, Purpose: "setup", ClaimOperationID: f.input.OperationID, RegistrationID: serverreturn.RegistrationID(f.input.ServerID, f.input.OperationID, 1, 1, 1, origin), State: strings.Repeat("s", 43), CodeChallenge: serverreturn.Challenge(strings.Repeat("v", 43)), ServerChallenge: strings.Repeat("c", 32), InstallationID: strings.Repeat("i", 43)}
	if path := os.Getenv("PORTICO_RETURN_CONSUMER_INPUT"); path != "" {
		raw, _ := os.ReadFile(path)
		var input struct{ Challenge, InstallationID string }
		if json.Unmarshal(raw, &input) != nil {
			t.Fatal("bad fixture binding")
		}
		q.ServerChallenge = input.Challenge
		q.InstallationID = input.InstallationID
	}
	return returnFixture{claimJourney: f, handler: handler, directory: dir, keys: keys, start: q}
}
func returnCall(t *testing.T, h http.Handler, path, token string, q any, want int) []byte {
	t.Helper()
	raw, _ := json.Marshal(q)
	r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s status %d want %d: %s", path, w.Code, want, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cacheable")
	}
	return w.Body.Bytes()
}
func (f returnFixture) returnAccountToken() string {
	if f.accountToken != "" {
		return f.accountToken
	}
	return "claim-owner-access-token-fixture"
}
func (f returnFixture) begin(t *testing.T) serverreturn.Started {
	t.Helper()
	var out serverreturn.Started
	raw := returnCall(t, f.handler, "/v1/server-returns/start", f.returnAccountToken(), f.start, 201)
	if json.Unmarshal(raw, &out) != nil || bytes.Contains(raw, []byte("code")) {
		t.Fatal("start disclosed code")
	}
	return out
}
func (f *returnFixture) complete(t *testing.T, id string) serverreturn.Exchange {
	t.Helper()
	var out serverreturn.Return
	raw := returnCall(t, f.handler, "/v1/server-returns/"+id+"/complete", f.returnAccountToken(), struct{}{}, 200)
	f.completion = append(json.RawMessage(nil), raw...)
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("bad return")
	}
	u, e := url.Parse(out.Location)
	if e != nil || u.Scheme != "https" || u.Host != "server.example:32500" || u.Path != serverreturn.ReturnPath || u.RawQuery != "" {
		t.Fatal("arbitrary return")
	}
	v, _ := url.ParseQuery(u.Fragment)
	if v.Get("state") != f.start.State || v.Get("transaction") != id {
		t.Fatal("binding")
	}
	return serverreturn.Exchange{Code: v.Get("code"), TransactionID: id, Verifier: strings.Repeat("v", 43), RegistrationID: f.start.RegistrationID, ServerChallenge: f.start.ServerChallenge, InstallationID: f.start.InstallationID, Purpose: f.start.Purpose}
}
func TestRegisteredReturnActualHTTPPrivacyPKCERaceReplay(t *testing.T) {
	f := registeredReturnFixture(t)
	started := f.begin(t)
	q := f.complete(t, started.TransactionID)
	bad := q
	bad.Verifier = strings.Repeat("x", 43)
	returnCall(t, f.handler, "/v1/server-returns/exchange", "", bad, 401)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			out, e := f.store.ExchangeServerReturn(context.Background(), q, f.directory.SignServerReturn)
			if e == nil {
				successes.Add(1)
				raw, _ := base64.RawURLEncoding.DecodeString(out.Assertion.Payload)
				var a serverreturn.Assertion
				if json.Unmarshal(raw, &a) != nil || a.Username == "" || a.AccountID != f.expected.AccountID || a.Purpose != "setup" || a.RegistrationID != f.start.RegistrationID || a.DisplayName != "Fixture" || a.Email != "" || !bytes.Contains(raw, []byte(`"email":""`)) || bytes.Contains(raw, []byte(`"profiles"`)) {
					t.Error("identity privacy/binding")
				}
				if _, e = out.Assertion.Verify(f.keys.RootPublicKey, "documents", time.Now()); e != nil {
					t.Error(e)
				}
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("race did not spend exactly once", successes.Load())
	}
	raw := returnCall(t, f.handler, "/v1/server-returns/exchange", "", q, 401)
	if bytes.Contains(raw, []byte(q.Code)) || bytes.Contains(raw, []byte(q.Verifier)) {
		t.Fatal("secret error")
	}
	var hash []byte
	if e := f.store.DB.QueryRow(`SELECT code_hash FROM server_returns WHERE id=$1`, q.TransactionID).Scan(&hash); e != nil || bytes.Equal(hash, []byte(q.Code)) || len(hash) != 32 {
		t.Fatal("unhashed code")
	}
}
func TestRegisteredReturnLiveFences(t *testing.T) {
	for _, stage := range []string{"complete", "exchange"} {
		for name, mutation := range map[string]string{
			"account_epoch":      `UPDATE accounts SET auth_epoch=auth_epoch+1 WHERE id='strict-owner'`,
			"family_logout":      `UPDATE session_families SET revoked_at=clock_timestamp() WHERE id='strict-owner-family'`,
			"access_logout":      `DELETE FROM account_sessions WHERE account_id='strict-owner'`,
			"family_generation":  `UPDATE session_families SET generation=generation+1 WHERE id='strict-owner-family'`,
			"removed_membership": `DELETE FROM server_members`,
			"withdrawal":         `DELETE FROM networking_route_candidates`,
			"route_generation":   `UPDATE networking_route_state SET generation=generation+1`,
			"claim_generation":   `UPDATE servers SET claim_generation=claim_generation+1`,
			"retired":            `UPDATE servers SET revoked_at=clock_timestamp(),owner_account_id=NULL,token_hash=NULL,token_ciphertext=NULL,base_url=NULL,endpoint_verified=false`,
			"custody_release":    `UPDATE servers SET custody_released=true`,
			"expiry":             `UPDATE server_returns SET expires_at=clock_timestamp()-interval '1 second',code_expires_at=CASE WHEN code_hash IS NULL THEN NULL ELSE clock_timestamp()-interval '2 seconds' END`,
		} {
			t.Run(stage+"/"+name, func(t *testing.T) {
				f := registeredReturnFixture(t)
				started := f.begin(t)
				var q serverreturn.Exchange
				if stage == "exchange" {
					q = f.complete(t, started.TransactionID)
				}
				if _, e := f.store.DB.Exec(mutation); e != nil {
					t.Fatal(e)
				}
				if stage == "complete" {
					returnCall(t, f.handler, "/v1/server-returns/"+started.TransactionID+"/complete", "claim-owner-access-token-fixture", struct{}{}, 401)
				} else {
					returnCall(t, f.handler, "/v1/server-returns/exchange", "", q, 401)
				}
			})
		}
	}
}
func TestRegisteredReturnRejectsArbitraryOriginsAndImmutableTuple(t *testing.T) {
	f := registeredReturnFixture(t)
	for _, origin := range []string{"https://attacker.example", "https://server.example:443", "https://server.examp1e:32500", "https://user@server.example:32500", "javascript:alert(1)", "http://server.example:32500", "https://server.example:32500/path", "https://server.example:32500?x=1", "https://servér.example:32500"} {
		t.Run(origin, func(t *testing.T) {
			q := f.start
			q.RegistrationID = serverreturn.RegistrationID(q.ServerID, q.ClaimOperationID, 1, 1, 1, origin)
			returnCall(t, f.handler, "/v1/server-returns/start", "claim-owner-access-token-fixture", q, 401)
		})
	}
	started := f.begin(t)
	q := f.complete(t, started.TransactionID)
	for _, change := range []func(*serverreturn.Exchange){func(q *serverreturn.Exchange) { q.Purpose = "switch" }, func(q *serverreturn.Exchange) { q.InstallationID = strings.Repeat("j", 43) }, func(q *serverreturn.Exchange) { q.ServerChallenge = strings.Repeat("d", 32) }, func(q *serverreturn.Exchange) { q.RegistrationID = "ret_wrong" }, func(q *serverreturn.Exchange) { q.TransactionID = "srt_wrong" }} {
		bad := q
		change(&bad)
		returnCall(t, f.handler, "/v1/server-returns/exchange", "", bad, 401)
	}
	returnCall(t, f.handler, "/v1/server-returns/"+started.TransactionID+"/complete", "strict-other-access", struct{}{}, 401)
	returnCall(t, f.handler, "/v1/server-returns/exchange", "claim-owner-access-token-fixture", q, 401)
}
func TestRegisteredReturnRawProducerArtifact(t *testing.T) {
	path := os.Getenv("PORTICO_RETURN_PRODUCER_JSON")
	if path == "" {
		t.Skip("artifact requested by consumer contract driver")
	}
	f := registeredReturnFixture(t)
	if os.Getenv("PORTICO_RETURN_PRODUCER_PURPOSE") == "sign-in" {
		f = signInReturnFixture(t, f)
	}
	started := f.begin(t)
	q := f.complete(t, started.TransactionID)
	raw := returnCall(t, f.handler, "/v1/server-returns/exchange", "", q, 200)
	artifact := map[string]any{"grant": json.RawMessage(raw), "completion": f.completion, "start": started, "root": base64.RawURLEncoding.EncodeToString(f.keys.RootPublicKey), "request": q, "serverPublicKey": f.input.PublicKey}
	encoded, _ := json.Marshal(artifact)
	if e := os.WriteFile(path, encoded, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestClaimV2SignedPurposeCannotChooseCurrentOwner(t *testing.T) {
	f := registeredReturnFixture(t)
	in := f.input
	in.OperationID = "change_operation"
	in.Purpose = "change-owner"
	in.CurrentOwnerAccountID = f.expected.AccountID
	in.PreviousOwnerRole = "admin"
	if _, e := f.store.ApproveClaim(context.Background(), f.expected, in, "https://hosted.example"); e == nil {
		t.Fatal("selected current owner")
	}
	// Signature covers each binding, including an empty provisional registration.
	a := claims.Approval{TargetUsername: "strict-other", Kind: "portico.claim.approval", Version: "2", Audience: "https://hosted.example", OperationID: in.OperationID, ServerID: in.ServerID, AccountID: "strict-other", PublicKey: in.PublicKey, ApprovalRevision: 1, Purpose: "change-owner", CurrentOwnerAccountID: f.expected.AccountID, PreviousOwnerRole: "admin", IssuedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)}
	env, e := claims.SignApproval(a, f.keys.PrivateKey, f.keys.KeyID, f.keys.Certificate)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(env.Payload)
	a.PreviousOwnerRole = "remove"
	changed, _ := json.Marshal(a)
	if ed25519.Verify(f.keys.PrivateKey.Public().(ed25519.PublicKey), changed, mustSignature(env)) {
		t.Fatal("unsigned role substitution")
	}
	if !bytes.Contains(raw, []byte("currentOwnerAccountId")) {
		t.Fatal("missing binding")
	}
}
func mustSignature(e trust.Envelope) []byte {
	raw, _ := base64.RawURLEncoding.DecodeString(e.Signature)
	return raw
}

func TestClaimV2OwnershipOutcomesAndOldReturnRevocation(t *testing.T) {
	for _, role := range []string{"admin", "member", "remove"} {
		t.Run(role, func(t *testing.T) {
			f := registeredReturnFixture(t)
			ctx := context.Background()
			pending := f.begin(t)
			oldCode := f.complete(t, pending.TransactionID)
			target, e := f.store.CaptureClaimAuthority(ctx, security.Hash("strict-other-access"))
			if e != nil {
				t.Fatal(e)
			}
			in := f.input
			in.OperationID = "transfer_" + role
			in.Purpose = "change-owner"
			in.CurrentOwnerAccountID = f.expected.AccountID
			in.PreviousOwnerRole = role
			signed, e := f.service.Approve(ctx, target, in)
			if e != nil {
				t.Fatal("approval", e)
			}
			raw, _ := base64.RawURLEncoding.DecodeString(signed.Payload)
			var approval claims.Approval
			if json.Unmarshal(raw, &approval) != nil || approval.PreviousOwnerRole != role || approval.CurrentOwnerAccountID != f.expected.AccountID {
				t.Fatal("signed outcome")
			}
			journey := f.claimJourney
			journey.input = in
			proof, sig := journeyProof(t, journey, approval, claims.Finalize)
			commit, e := f.service.Finalize(ctx, proof, sig)
			if e != nil {
				t.Fatal("transfer commit", e)
			}
			proof, sig = journeyProof(t, journey, approval, claims.Result)
			result, e := f.service.Result(ctx, proof, sig)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.service.Acknowledge(ctx, result.ServerCredential, commit); e != nil {
				t.Fatal(e)
			}
			var owner string
			if e = f.store.DB.QueryRow(`SELECT owner_account_id FROM servers WHERE id=$1`, in.ServerID).Scan(&owner); e != nil || owner != target.AccountID {
				t.Fatal("owner", owner, e)
			}
			var previous string
			e = f.store.DB.QueryRow(`SELECT role FROM server_members WHERE server_id=$1 AND account_id=$2`, in.ServerID, f.expected.AccountID).Scan(&previous)
			if role == "remove" {
				if e == nil {
					t.Fatal("former owner still member")
				}
			} else if e != nil || previous != role {
				t.Fatal("previous tier", previous, e)
			}
			var legacyOwnerCount int
			if e = f.store.DB.QueryRow(`SELECT count(*) FROM server_members WHERE server_id=$1 AND role='owner'`, in.ServerID).Scan(&legacyOwnerCount); e != nil || legacyOwnerCount != 1 {
				t.Fatal("duplicate owner", e)
			}
			returnCall(t, f.handler, "/v1/server-returns/exchange", "", oldCode, 401)
			// New installation has no route registration: the old exact origin grants no return.
			request := f.start
			request.ClaimOperationID = in.OperationID
			request.Purpose = "change-owner"
			returnCall(t, f.handler, "/v1/server-returns/start", "strict-other-access", request, 401)
			var transfers int
			if e = f.store.DB.QueryRow(`SELECT count(*) FROM server_ownership_transfers WHERE server_id=$1 AND new_account_id=$2`, in.ServerID, target.AccountID).Scan(&transfers); e != nil || transfers != 1 {
				t.Fatal("transfer receipt", e)
			}
		})
	}
}

func TestRegisteredReturnCallerServerAndOwnerProjectionBoundaries(t *testing.T) {
	f := registeredReturnFixture(t)
	otherToken := "claim-other-access-token-fixture"
	if _, e := f.store.DB.Exec(`INSERT INTO account_sessions(token_hash,account_id,family_id,expires_at) VALUES($1,'strict-other','strict-other-family',clock_timestamp()+interval '15 minutes')`, security.Hash(otherToken)); e != nil {
		t.Fatal(e)
	}
	// The same authenticated account cannot use this registration on another
	// server; another account cannot use it on this server.
	changed := f.start
	changed.ServerID = "srv_another_server"
	returnCall(t, f.handler, "/v1/server-returns/start", "claim-owner-access-token-fixture", changed, 401)
	returnCall(t, f.handler, "/v1/server-returns/start", otherToken, f.start, 401)
	if _, e := f.store.DB.Exec(`UPDATE accounts SET display_name='Private personal name never projected' WHERE id='strict-owner'`); e != nil {
		t.Fatal(e)
	}
	q := claimweb.Request{Kind: "portico.claim.web.v2", Audience: "https://hosted.example", OperationID: "owner_projection", ServerID: f.input.ServerID, PublicKey: f.input.PublicKey, Name: "Shared server", Address: "https://server.example:32500", Purpose: "change-owner", CurrentOwnerAccountID: f.expected.AccountID, PreviousOwnerRole: "admin", IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(4 * time.Minute)}
	pending, e := f.store.RegisterWebClaim(context.Background(), q, f.keys.SealKey)
	if e != nil {
		t.Fatal(e)
	}
	handler := &httpapi.ClaimHandler{Web: &httpapi.ClaimWeb{Store: f.store, Keys: f.keys, Audience: q.Audience}, Service: f.service, CaptureAuthority: f.store.CaptureClaimAuthority}
	raw := returnCall(t, handler, "/v1/server-claims/lookup", otherToken, httpapi.ClaimCodeRequest{Code: pending.Code}, 200)
	var result claimweb.Pending
	if json.Unmarshal(raw, &result) != nil || result.CurrentOwnerUsername != "strict-owner" || result.Purpose != "change-owner" || result.PreviousOwnerRole != "admin" {
		t.Fatal("unverified owner projection")
	}
	for _, private := range []string{"Private personal name never projected", "displayName", "email", "completionCode", "src_"} {
		if bytes.Contains(raw, []byte(private)) {
			t.Fatal("owner privacy projection", private)
		}
	}
}
