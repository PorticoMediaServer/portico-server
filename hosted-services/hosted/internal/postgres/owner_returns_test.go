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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
	"portico.local/hosted/trust"
)

type ownerReturnFixture struct {
	claimJourney
	directory        *directory.Service
	keys             *security.Keys
	request          claimweb.Request
	start            claimweb.OwnerStart
	original, signed claimweb.Signed
	pending          claimweb.Pending
	approval         claims.Approval
	token            string
	exchange         serverreturn.OwnerExchange
}

func ownerSigned(t *testing.T, key ed25519.PrivateKey, q any) claimweb.Signed {
	t.Helper()
	raw, e := json.Marshal(q)
	if e != nil {
		t.Fatal(e)
	}
	return claimweb.Signed{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, raw))}
}
func newOwnerReturn(t *testing.T, address string) ownerReturnFixture {
	t.Helper()
	f := currentClaimJourney(t)
	f.input.Purpose = "switch"
	keys, e := fixtureKeys()
	if e != nil {
		t.Fatal(e)
	}
	keys.SealKey = bytes.Repeat([]byte{2}, 32)
	dir := directory.New(f.store, keys)
	claimHandler := &httpapi.ClaimHandler{Service: f.service, CaptureAuthority: f.store.CaptureClaimAuthority, Web: &httpapi.ClaimWeb{Store: f.store, Keys: keys, Audience: "https://hosted.example"}}
	f.handler = (&httpapi.Handler{Claims: claimHandler, Directory: dir, ServerReturns: &httpapi.ServerReturns{Store: f.store, Directory: dir}}).Routes()
	now := time.Now().UTC().Truncate(time.Microsecond)
	q := claimweb.Request{Kind: "portico.claim.web.v2", Audience: "https://hosted.example", OperationID: f.input.OperationID, ServerID: f.input.ServerID, PublicKey: f.input.PublicKey, LocalGeneration: 7, Name: "Actual fixture server", Address: address, Purpose: "switch", IssuedAt: now, ExpiresAt: now.Add(4 * time.Minute)}
	f.input.LocalGeneration = q.LocalGeneration
	original := ownerSigned(t, f.key, q)
	raw, _ := base64.RawURLEncoding.DecodeString(original.Payload)
	start := claimweb.OwnerStart{Kind: claimweb.OwnerStartKind, Audience: q.Audience, ClaimHash: base64.RawURLEncoding.EncodeToString(security.Hash(string(raw))), OperationID: q.OperationID, ServerID: q.ServerID, PublicKey: q.PublicKey, LocalGeneration: q.LocalGeneration, Address: q.Address, Purpose: "switch", State: strings.Repeat("s", 43), CodeChallenge: serverreturn.Challenge(strings.Repeat("v", 43)), InstallationID: strings.Repeat("i", 43), ServerChallenge: strings.Repeat("c", 43), IssuedAt: now, ExpiresAt: q.ExpiresAt}
	signed := ownerSigned(t, f.key, start)
	return ownerReturnFixture{claimJourney: f, directory: dir, keys: keys, request: q, start: start, original: original, signed: signed}
}
func (f *ownerReturnFixture) register(t *testing.T) {
	t.Helper()
	raw := returnCall(t, f.handler, "/v1/server-claims/pending", "", claimweb.PendingRequest{Payload: f.original.Payload, Signature: f.original.Signature, OwnerStart: &f.signed}, 200)
	if json.Unmarshal(raw, &f.pending) != nil || f.pending.OwnerTransactionID == "" || f.pending.ReturnRegistration != nil {
		t.Fatal("pending has no frozen owner transaction or invented registration")
	}
}
func (f *ownerReturnFixture) approve(t *testing.T) {
	t.Helper()
	returnCall(t, f.handler, "/v1/server-claims/approve-code", "claim-owner-access-token-fixture", map[string]string{"code": f.pending.Code}, 200)
	raw, e := f.store.WaitWebClaim(context.Background(), f.request)
	if e != nil {
		t.Fatal(e)
	}
	var envelope claims.Envelope
	if json.Unmarshal(raw, &envelope) != nil {
		t.Fatal("approval envelope")
	}
	payload, _ := base64.RawURLEncoding.DecodeString(envelope.Payload)
	if json.Unmarshal(payload, &f.approval) != nil || f.approval.AccountID != f.expected.AccountID || f.approval.TargetUsername != "strict-owner" || f.approval.Purpose != "switch" {
		t.Fatal("approval binding")
	}
	expiry, e := time.Parse(time.RFC3339Nano, f.approval.ExpiresAt)
	if e != nil || expiry.After(f.start.ExpiresAt) {
		t.Fatal("approval extended original start")
	}
}
func (f *ownerReturnFixture) install(t *testing.T) {
	t.Helper()
	raw, sig := journeyProof(t, f.claimJourney, f.approval, claims.Finalize)
	commit, e := f.service.Finalize(context.Background(), raw, sig)
	if e != nil {
		t.Fatal(e)
	}
	raw, sig = journeyProof(t, f.claimJourney, f.approval, claims.Result)
	result, e := f.service.Result(context.Background(), raw, sig)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.Acknowledge(context.Background(), result.ServerCredential, commit); e != nil {
		t.Fatal(e)
	}
	f.token = result.ServerCredential
	claimGeneration, _ := strconv.ParseInt(commit.ClaimGeneration, 10, 64)
	credentialGeneration, _ := strconv.ParseInt(commit.CredentialGeneration, 10, 64)
	// Explicit fixture verified-route fact, not a DNS/certificate acceptance claim.
	if _, e = f.store.DB.Exec(`INSERT INTO networking_route_state(server_id,operation_id,claim_generation,credential_generation,generation,observation_digest,remote_enabled) VALUES($1,$2,$3,$4,1,$5,true)`, f.input.ServerID, f.input.OperationID, claimGeneration, credentialGeneration, bytes.Repeat([]byte{1}, 32)); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`INSERT INTO networking_route_candidates(server_id,operation_id,generation,origin,class,state,verified_at) VALUES($1,$2,1,'https://installed.example:32500','public','verified',clock_timestamp())`, f.input.ServerID, f.input.OperationID); e != nil {
		t.Fatal(e)
	}
	var reg serverreturn.Registration
	raw = returnCall(t, f.handler, "/v1/servers/"+f.input.ServerID+"/return-registration", f.token, serverreturn.RegistrationRequest{Origin: "https://installed.example:32500"}, 200)
	if json.Unmarshal(raw, &reg) != nil || reg.ID == "" {
		t.Fatal("actual registration")
	}
	f.exchange = serverreturn.OwnerExchange{TransactionID: f.pending.OwnerTransactionID, State: f.start.State, Verifier: strings.Repeat("v", 43), RegistrationID: reg.ID, InstallationID: f.start.InstallationID, ServerChallenge: f.start.ServerChallenge, Purpose: "switch"}
}
func (f *ownerReturnFixture) complete(t *testing.T) []byte {
	t.Helper()
	raw := returnCall(t, f.handler, "/v1/server-returns/"+f.pending.OwnerTransactionID+"/complete", "claim-owner-access-token-fixture", serverreturn.Complete{}, 200)
	var out serverreturn.Return
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("completion")
	}
	u, e := url.Parse(out.Location)
	if e != nil || u.Scheme+"://"+u.Host != f.request.Address || u.Path != serverreturn.ReturnPath || u.RawQuery != "" {
		t.Fatal("original callback")
	}
	values, e := url.ParseQuery(u.Fragment)
	if e != nil || len(values) != 1 || values.Get("transaction") != f.pending.OwnerTransactionID {
		t.Fatal("callback leaked state/code or chose arbitrary origin")
	}
	return raw
}
func TestOwnerReturnActualHTTPFrozenStartInstalledExchange(t *testing.T) {
	for _, origin := range []string{"https://original.example:32500", "http://localhost:32500"} {
		t.Run(origin, func(t *testing.T) {
			f := newOwnerReturn(t, origin)
			f.register(t)
			first := f.pending
			f.register(t)
			if f.pending.OwnerTransactionID != first.OwnerTransactionID || f.pending.Code != first.Code {
				t.Fatal("retry retargeted")
			}
			// No ordinary registration or challenge is needed before the actual switch.
			returnCall(t, f.handler, "/v1/server-returns/"+f.pending.OwnerTransactionID+"/complete", "claim-owner-access-token-fixture", serverreturn.Complete{}, 401)
			f.approve(t)
			if _, e := f.store.ApproveClaim(context.Background(), f.expected, f.input, f.request.Audience); e == nil {
				t.Fatal("direct approval bypassed owner start")
			}
			f.install(t)
			var found claimweb.Pending
			raw := returnCall(t, f.handler, "/v1/server-claims/lookup", "claim-owner-access-token-fixture", map[string]string{"code": f.pending.Code}, 200)
			if json.Unmarshal(raw, &found) != nil || found.ReturnRegistration == nil || found.ReturnRegistration.ID != f.exchange.RegistrationID || found.ReturnRegistration.Origin != "https://installed.example:32500" {
				t.Fatal("lookup inferred registration from original address")
			}
			complete := f.complete(t)
			if !bytes.Equal(complete, f.complete(t)) {
				t.Fatal("completion replay changed callback")
			}
			returnCall(t, f.handler, "/v1/server-returns/exchange", "", f.exchange, 401)
			returnCall(t, f.handler, "/v1/server-returns/exchange", "claim-owner-access-token-fixture", f.exchange, 401)
			var grant serverreturn.Grant
			raw = returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 200)
			if json.Unmarshal(raw, &grant) != nil {
				t.Fatal("grant")
			}
			payload, e := grant.Assertion.Verify(f.keys.RootPublicKey, "documents", time.Now())
			if e != nil {
				t.Fatal(e)
			}
			var assertion serverreturn.Assertion
			if json.Unmarshal(payload, &assertion) != nil || assertion.AccountID != f.expected.AccountID || assertion.Username != f.approval.TargetUsername || assertion.Purpose != "switch" || assertion.ClaimOperationID != f.input.OperationID || assertion.RegistrationID != f.exchange.RegistrationID || assertion.InstallationID != f.start.InstallationID || assertion.Challenge != f.start.ServerChallenge || assertion.CurrentOwnerAccountID != "" || assertion.PreviousOwnerRole != "" || assertion.ExpiresAt.After(f.start.ExpiresAt) || bytes.Contains(payload, []byte("profiles")) {
				t.Fatal("signed unchanged-order identity binding")
			}
			canonical, _ := json.Marshal(assertion)
			if !bytes.Equal(payload, canonical) {
				t.Fatal("noncanonical assertion")
			}
			returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 401)
			var phase string
			var completed, consumed time.Time
			if e = f.store.DB.QueryRow(`SELECT phase,completed_at,consumed_at FROM owner_claim_starts WHERE transaction_id=$1`, f.pending.OwnerTransactionID).Scan(&phase, &completed, &consumed); e != nil || phase != "consumed" || consumed.Before(completed) {
				t.Fatal("consume lifecycle", e)
			}
		})
	}
}
func TestOwnerReturnRejectsRefreshAndPrivateBindingChanges(t *testing.T) {
	f := newOwnerReturn(t, "https://original.example")
	f.register(t)
	// Missing start is refused, and neither time-only refresh nor a new state can
	// reattach the same approval code/operation to a different browser pending.
	returnCall(t, f.handler, "/v1/server-claims/pending", "", f.original, 400)
	q := f.request
	q.IssuedAt = q.IssuedAt.Add(time.Second)
	q.ExpiresAt = q.ExpiresAt.Add(time.Second)
	newer := ownerSigned(t, f.key, q)
	start := f.start
	start.IssuedAt = q.IssuedAt
	start.ExpiresAt = q.ExpiresAt
	payload, _ := base64.RawURLEncoding.DecodeString(newer.Payload)
	start.ClaimHash = base64.RawURLEncoding.EncodeToString(security.Hash(string(payload)))
	signed := ownerSigned(t, f.key, start)
	if _, e := f.store.RegisterOwnerStart(context.Background(), newer, signed, q.Audience, f.keys.SealKey); e == nil {
		t.Fatal("time-only refresh replaced frozen claim")
	}
	start = f.start
	start.State = strings.Repeat("x", 43)
	signed = ownerSigned(t, f.key, start)
	if _, e := f.store.RegisterOwnerStart(context.Background(), f.original, signed, q.Audience, f.keys.SealKey); e == nil {
		t.Fatal("state rebound")
	}
	f.approve(t)
	f.install(t)
	f.complete(t)
	changes := []struct {
		name   string
		change func(*serverreturn.OwnerExchange)
	}{
		{"state", func(q *serverreturn.OwnerExchange) { q.State = strings.Repeat("x", 43) }},
		{"verifier", func(q *serverreturn.OwnerExchange) { q.Verifier = strings.Repeat("x", 43) }},
		{"installation", func(q *serverreturn.OwnerExchange) { q.InstallationID = strings.Repeat("x", 43) }},
		{"challenge", func(q *serverreturn.OwnerExchange) { q.ServerChallenge = strings.Repeat("x", 43) }},
		{"registration", func(q *serverreturn.OwnerExchange) { q.RegistrationID = "wrong_reg" }},
		{"transaction", func(q *serverreturn.OwnerExchange) { q.TransactionID = "wrong_tx" }},
	}
	for _, c := range changes {
		t.Run(c.name, func(t *testing.T) {
			q := f.exchange
			c.change(&q)
			returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, q, 401)
		})
	}
	returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 200)
}
func TestOwnerReturnConcurrentConsumeHasOneSignedGrant(t *testing.T) {
	f := newOwnerReturn(t, "https://original.example")
	f.register(t)
	f.approve(t)
	f.install(t)
	f.complete(t)
	a, e := f.directory.CaptureServerAuthority(context.Background(), f.token)
	if e != nil {
		t.Fatal(e)
	}
	var success, signatures atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, e := f.store.ExchangeOwnerReturn(context.Background(), a, f.exchange, func(q serverreturn.Assertion) (outEnvelope trust.Envelope, err error) {
				signatures.Add(1)
				return f.directory.SignServerReturn(q)
			})
			if e == nil {
				success.Add(1)
			}
		})
	}
	wg.Wait()
	if success.Load() != 1 || signatures.Load() != 1 {
		t.Fatal("duplicate consume/sign", success.Load(), signatures.Load())
	}
}

func TestOwnerReturnLiveAuthorityFences(t *testing.T) {
	cases := []struct{ name, mutation string }{
		{"account epoch", `UPDATE accounts SET auth_epoch=auth_epoch+1 WHERE id='strict-owner'`},
		{"family successor", `UPDATE session_families SET generation=generation+1 WHERE id='strict-owner-family'`},
		{"family revoked", `UPDATE session_families SET revoked_at=clock_timestamp() WHERE id='strict-owner-family'`},
		{"family expired", `UPDATE session_families SET idle_expires_at=clock_timestamp()-interval '1 second' WHERE id='strict-owner-family'`},
		{"access expired", `UPDATE account_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE account_id='strict-owner'`},
		{"username changed", `UPDATE accounts SET username='changed-owner' WHERE id='strict-owner'`},
		{"start expired", `UPDATE owner_claim_starts SET expires_at=issued_at+interval '1 microsecond'`},
		{"server revoked", `UPDATE servers SET revoked_at=clock_timestamp(),owner_account_id=NULL,token_hash=NULL,token_ciphertext=NULL,base_url=NULL,endpoint_verified=false`},
		{"credential successor", `UPDATE servers SET credential_generation=credential_generation+1`},
		{"operation cancelled", `UPDATE claim_operations SET state='cancelled',cancelled_at=clock_timestamp()`},
		{"owner changed", `UPDATE servers SET owner_account_id='strict-other'`},
		{"route generation changed", `UPDATE networking_route_state SET generation=generation+1`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newOwnerReturn(t, "https://original.example")
			f.register(t)
			f.approve(t)
			f.install(t)
			f.complete(t)
			if _, e := f.store.DB.Exec(c.mutation); e != nil {
				t.Fatal(e)
			}
			returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 401)
			var count int
			if e := f.store.DB.QueryRow(`SELECT count(*) FROM owner_claim_starts WHERE phase='consumed'`).Scan(&count); e != nil || count != 0 {
				t.Fatal("invalid authority consumed", e)
			}
		})
	}
}
func TestOwnerReturnRegistrationFirstBindCannotRetarget(t *testing.T) {
	f := newOwnerReturn(t, "http://localhost:32500")
	f.register(t)
	f.approve(t)
	f.install(t)
	// An exact resolver replay is idempotent and leaves the original pending frozen.
	q := serverreturn.RegistrationRequest{Origin: "https://installed.example:32500"}
	returnCall(t, f.handler, "/v1/servers/"+f.input.ServerID+"/return-registration", f.token, q, 200)
	if _, e := f.store.DB.Exec(`UPDATE networking_route_state SET generation=2; UPDATE networking_route_candidates SET generation=2`); e != nil {
		t.Fatal(e)
	}
	var changed serverreturn.Registration
	raw := returnCall(t, f.handler, "/v1/servers/"+f.input.ServerID+"/return-registration", f.token, q, 200)
	if json.Unmarshal(raw, &changed) != nil || changed.ID == f.exchange.RegistrationID {
		t.Fatal("new actual route not resolved")
	}
	var phase, reg string
	if e := f.store.DB.QueryRow(`SELECT phase,registration_id FROM owner_claim_starts`).Scan(&phase, &reg); e != nil || phase != "superseded" || reg != f.exchange.RegistrationID {
		t.Fatal("frozen registration retargeted", phase, e)
	}
	returnCall(t, f.handler, "/v1/server-returns/"+f.pending.OwnerTransactionID+"/complete", "claim-owner-access-token-fixture", serverreturn.Complete{}, 401)
	f.exchange.RegistrationID = changed.ID
	returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 401)
}
func TestOwnerReturnHTTPVariantsAndCapturedFamily(t *testing.T) {
	f := newOwnerReturn(t, "https://original.example")
	f.register(t)
	f.approve(t)
	f.install(t)
	// A second current family of the same account is not the captured approver.
	if _, e := f.store.DB.Exec(`INSERT INTO session_families(id,account_id,mode,current_refresh_hash,account_epoch,absolute_expires_at,idle_expires_at) VALUES('second-family','strict-owner','native',$1,0,clock_timestamp()+interval '1 hour',clock_timestamp()+interval '30 minutes')`, security.Hash("second-refresh")); e != nil {
		t.Fatal(e)
	}
	const second = "second-current-access-token"
	if _, e := f.store.DB.Exec(`INSERT INTO account_sessions(token_hash,account_id,family_id,expires_at) VALUES($1,'strict-owner','second-family',clock_timestamp()+interval '10 minutes')`, security.Hash(second)); e != nil {
		t.Fatal(e)
	}
	returnCall(t, f.handler, "/v1/server-returns/"+f.pending.OwnerTransactionID+"/complete", second, serverreturn.Complete{}, 401)
	returnCall(t, f.handler, "/v1/server-returns/"+f.pending.OwnerTransactionID+"/complete", "strict-other-access", serverreturn.Complete{}, 401)
	f.complete(t)
	raw, _ := json.Marshal(f.exchange)
	cases := []struct {
		name, body string
		headers    http.Header
	}{
		{"duplicate authorization", string(raw), http.Header{"Authorization": []string{"Bearer " + f.token, "Bearer " + f.token}}},
		{"cookie", string(raw), http.Header{"Authorization": []string{"Bearer " + f.token}, "Cookie": []string{"portico_session=ignored"}}},
		{"unknown field", strings.TrimSuffix(string(raw), "}") + `,"unexpected":true}`, http.Header{"Authorization": []string{"Bearer " + f.token}}},
		{"mixed code", strings.TrimSuffix(string(raw), "}") + `,"code":"dummy_code"}`, http.Header{"Authorization": []string{"Bearer " + f.token}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/v1/server-returns/exchange", strings.NewReader(c.body))
			r.Header = c.headers
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, r)
			if w.Code < 400 {
				t.Fatal("invalid variant admitted")
			}
		})
	}
	normal := serverreturn.Exchange{TransactionID: f.exchange.TransactionID, Code: strings.Repeat("k", 43), Verifier: f.exchange.Verifier, RegistrationID: f.exchange.RegistrationID, InstallationID: f.exchange.InstallationID, ServerChallenge: f.exchange.ServerChallenge, Purpose: "switch"}
	returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, normal, 401)
	returnCall(t, f.handler, "/v1/server-returns/exchange", "", normal, 401)
	returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 200)
}
func TestOwnerReturnCancellationAndProofRetention(t *testing.T) {
	t.Run("cancel before install", func(t *testing.T) {
		f := newOwnerReturn(t, "https://original.example")
		f.register(t)
		f.approve(t)
		raw, sig := cancelAssertion(t, f.claimJourney, "")
		if _, e := f.service.Cancel(context.Background(), raw, sig); e != nil {
			t.Fatal(e)
		}
		returnCall(t, f.handler, "/v1/server-returns/"+f.pending.OwnerTransactionID+"/complete", "claim-owner-access-token-fixture", serverreturn.Complete{}, 401)
		var phase string
		if e := f.store.DB.QueryRow(`SELECT phase FROM owner_claim_starts`).Scan(&phase); e != nil || phase != "cancelled" {
			t.Fatal(phase, e)
		}
	})
	t.Run("signed departure purges proofs", func(t *testing.T) {
		f := newOwnerReturn(t, "https://original.example")
		f.register(t)
		f.approve(t)
		f.install(t)
		f.complete(t)
		w := claims.LeaveWire{Kind: "portico.server.leave", Version: "1", Audience: f.request.Audience, OperationID: f.input.OperationID, ServerID: f.input.ServerID, PublicKey: f.input.PublicKey, ClaimGeneration: "1", CredentialGeneration: "1", NoticeID: "owner_leave_notice", Reason: "moving_to_local"}
		signed := ownerSigned(t, f.key, w)
		returnCallEmpty := func() {
			raw, _ := json.Marshal(signed)
			r := httptest.NewRequest("POST", "/v1/servers/leave", bytes.NewReader(raw))
			r.Header.Set("Content-Type", "application/json")
			out := httptest.NewRecorder()
			f.handler.ServeHTTP(out, r)
			if out.Code != 204 || out.Body.Len() != 0 {
				t.Fatal("departure receipt", out.Code)
			}
		}
		returnCallEmpty()
		returnCallEmpty()
		var count int
		if e := f.store.DB.QueryRow(`SELECT count(*) FROM owner_claim_starts`).Scan(&count); e != nil || count != 0 {
			t.Fatal("departure retained full proofs", e)
		}
		returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 401)
	})
	t.Run("expired snapshot cannot be refreshed", func(t *testing.T) {
		f := newOwnerReturn(t, "https://original.example")
		f.register(t)
		if _, e := f.store.DB.Exec(`UPDATE owner_claim_starts SET expires_at=issued_at+interval '1 microsecond'`); e != nil {
			t.Fatal(e)
		}
		if e := f.store.PruneExpired(context.Background(), 100); e != nil {
			t.Fatal(e)
		}
		var count int
		if e := f.store.DB.QueryRow(`SELECT count(*) FROM owner_claim_starts`).Scan(&count); e != nil || count != 1 {
			t.Fatal("expired snapshot lost freeze", e)
		}
		// Retention is bounded, while signed proof bytes are removed at departure.
		if _, e := f.store.DB.Exec(`UPDATE owner_claim_starts SET issued_at=clock_timestamp()-interval '2 days',expires_at=clock_timestamp()-interval '2 days'+interval '1 second'`); e != nil {
			t.Fatal(e)
		}
		if e := f.store.PruneExpired(context.Background(), 100); e != nil {
			t.Fatal(e)
		}
		if e := f.store.DB.QueryRow(`SELECT count(*) FROM owner_claim_starts`).Scan(&count); e != nil || count != 0 {
			t.Fatal("unbounded snapshot retention", e)
		}
	})
}

func TestOwnerReturnRegisterIsAtomicAndCannotAdoptApprovedOperation(t *testing.T) {
	f := newOwnerReturn(t, "https://original.example")
	bad := f.signed
	bad.Signature = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	if _, e := f.store.RegisterOwnerStart(context.Background(), f.original, bad, f.request.Audience, f.keys.SealKey); e == nil {
		t.Fatal("invalid full proof")
	}
	var pending, owner int
	if e := f.store.DB.QueryRow(`SELECT (SELECT count(*) FROM claim_web_pending),(SELECT count(*) FROM owner_claim_starts)`).Scan(&pending, &owner); e != nil || pending != 0 || owner != 0 {
		t.Fatal("partial invalid registration", e)
	}
	// A prior ordinary approval is not retroactively relabelled as the bound
	// browser operation, even when every visible server field matches.
	journeyApproval(t, f.claimJourney)
	if _, e := f.store.RegisterOwnerStart(context.Background(), f.original, f.signed, f.request.Audience, f.keys.SealKey); e == nil {
		t.Fatal("adopted prior approval")
	}
	if e := f.store.DB.QueryRow(`SELECT (SELECT count(*) FROM claim_web_pending),(SELECT count(*) FROM owner_claim_starts)`).Scan(&pending, &owner); e != nil || pending != 0 || owner != 0 {
		t.Fatal("failed transaction retained partial registration", e)
	}
}
func TestOwnerReturnExpiredApprovalAndCancelledOperationCannotResume(t *testing.T) {
	f := newOwnerReturn(t, "https://original.example")
	f.register(t)
	f.approve(t)
	if _, e := f.store.DB.Exec(`UPDATE owner_claim_starts SET expires_at=issued_at+interval '1 microsecond'`); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	w := claims.Wire{Kind: claims.NonceRequest, Version: "1", Audience: f.request.Audience, Purpose: claims.Finalize, OperationID: f.input.OperationID, ServerID: f.input.ServerID, AccountID: f.expected.AccountID, PublicKey: f.input.PublicKey, LocalGeneration: f.input.LocalGeneration, ApprovalRevision: f.approval.ApprovalRevision, RequestID: "expired_request", IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(20 * time.Second).Format(time.RFC3339Nano), ReservationAccountID: f.expected.AccountID, ReservationUsername: f.approval.TargetUsername, ReservationGeneration: "7", ReservationOperationID: f.input.OperationID}
	signed := ownerSigned(t, f.key, w)
	raw, _ := base64.RawURLEncoding.DecodeString(signed.Payload)
	sig, _ := base64.RawURLEncoding.DecodeString(signed.Signature)
	if _, e := f.service.Nonce(context.Background(), raw, sig); e == nil {
		t.Fatal("expired start allowed finalization challenge")
	}
	raw, sig = cancelAssertion(t, f.claimJourney, "")
	if _, e := f.service.Cancel(context.Background(), raw, sig); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.RegisterOwnerStart(context.Background(), f.original, f.signed, f.request.Audience, f.keys.SealKey); e == nil {
		t.Fatal("cancelled start resumed")
	}
}
func TestOwnerReturnFamilyDeletionPurgesImmutableProofs(t *testing.T) {
	f := newOwnerReturn(t, "https://original.example")
	f.register(t)
	f.approve(t)
	f.install(t)
	if _, e := f.store.DB.Exec(`DELETE FROM session_families WHERE id='strict-owner-family'`); e != nil {
		t.Fatal(e)
	}
	var count int
	if e := f.store.DB.QueryRow(`SELECT count(*) FROM owner_claim_starts`).Scan(&count); e != nil || count != 0 {
		t.Fatal("family deletion retained immutable signed proof", e)
	}
	returnCall(t, f.handler, "/v1/server-returns/"+f.pending.OwnerTransactionID+"/complete", "claim-owner-access-token-fixture", serverreturn.Complete{}, 401)
}

func TestOwnerReturnRechecksDeadlineAfterSigningAndKeepsFailedAttemptUnspent(t *testing.T) {
	f := newOwnerReturn(t, "https://original.example")
	f.register(t)
	f.approve(t)
	f.install(t)
	f.complete(t)
	a, e := f.directory.CaptureServerAuthority(context.Background(), f.token)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`UPDATE owner_claim_starts SET expires_at=clock_timestamp()+interval '150 milliseconds'`); e != nil {
		t.Fatal(e)
	}
	out, e := f.store.ExchangeOwnerReturn(context.Background(), a, f.exchange, func(q serverreturn.Assertion) (trust.Envelope, error) {
		time.Sleep(200 * time.Millisecond)
		return f.directory.SignServerReturn(q)
	})
	if e == nil || out.Assertion.Payload != "" {
		t.Fatal("delayed signature escaped expired attempt")
	}
	var phase string
	var consumed bool
	if e = f.store.DB.QueryRow(`SELECT phase,consumed_at IS NOT NULL FROM owner_claim_starts`).Scan(&phase, &consumed); e != nil || phase != "completed" || consumed {
		t.Fatal("failed attempt committed", e)
	}
	// A lost successful response has no durable assertion replay. A storage/signing
	// failure before commit may retry only while all original guards remain live.
}
func TestOwnerReturnNonLocalHTTPAndWrongSignedBindingCreateNoRows(t *testing.T) {
	cases := []struct {
		name   string
		change func(*ownerReturnFixture)
	}{
		{"network HTTP", func(f *ownerReturnFixture) {
			f.request.Address = "http://192.0.2.12:32500"
			f.start.Address = f.request.Address
		}},
		{"wrong original hash", func(f *ownerReturnFixture) {
			f.start.ClaimHash = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
		}},
		{"wrong operation", func(f *ownerReturnFixture) { f.start.OperationID = "another_operation" }},
		{"extended original challenge cap", func(f *ownerReturnFixture) { f.start.ExpiresAt = f.start.IssuedAt.Add(6 * time.Minute) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newOwnerReturn(t, "https://original.example")
			c.change(&f)
			f.original = ownerSigned(t, f.key, f.request)
			if c.name == "network HTTP" {
				raw, _ := base64.RawURLEncoding.DecodeString(f.original.Payload)
				f.start.ClaimHash = base64.RawURLEncoding.EncodeToString(security.Hash(string(raw)))
			}
			f.signed = ownerSigned(t, f.key, f.start)
			if _, e := f.store.RegisterOwnerStart(context.Background(), f.original, f.signed, f.request.Audience, f.keys.SealKey); e == nil {
				t.Fatal("invalid signed binding registered")
			}
			var count int
			if e := f.store.DB.QueryRow(`SELECT (SELECT count(*) FROM claim_web_pending)+(SELECT count(*) FROM owner_claim_starts)`).Scan(&count); e != nil || count != 0 {
				t.Fatal("refused start retained rows", e)
			}
		})
	}
}

func TestOwnerReturnExistingClaimRequiresExactOriginalCurrentOwner(t *testing.T) {
	f := newOwnerReturn(t, "https://original.example")
	prior := f.claimJourney
	prior.input.Purpose = "setup"
	prior.input.LocalGeneration = 0
	prior.input.OperationID = "prior_setup"
	a := journeyApproval(t, prior)
	raw, sig := journeyProof(t, prior, a, claims.Finalize)
	commit, e := prior.service.Finalize(context.Background(), raw, sig)
	if e != nil {
		t.Fatal(e)
	}
	raw, sig = journeyProof(t, prior, a, claims.Result)
	result, e := prior.service.Result(context.Background(), raw, sig)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = prior.service.Acknowledge(context.Background(), result.ServerCredential, commit); e != nil {
		t.Fatal(e)
	}
	f.request.CurrentOwnerAccountID = f.expected.AccountID
	f.input.CurrentOwnerAccountID = f.expected.AccountID
	f.original = ownerSigned(t, f.key, f.request)
	payload, _ := base64.RawURLEncoding.DecodeString(f.original.Payload)
	f.start.ClaimHash = base64.RawURLEncoding.EncodeToString(security.Hash(string(payload)))
	f.signed = ownerSigned(t, f.key, f.start)
	f.register(t)
	f.approve(t)
	f.install(t)
	f.complete(t)
	var grant serverreturn.Grant
	raw = returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 200)
	if json.Unmarshal(raw, &grant) != nil {
		t.Fatal("grant")
	}
	payload, _ = base64.RawURLEncoding.DecodeString(grant.Assertion.Payload)
	var assertion serverreturn.Assertion
	if json.Unmarshal(payload, &assertion) != nil || assertion.CurrentOwnerAccountID != f.expected.AccountID || assertion.ClaimGeneration != 2 || assertion.CredentialGeneration != 2 {
		t.Fatal("original owner/generation binding")
	}
}
