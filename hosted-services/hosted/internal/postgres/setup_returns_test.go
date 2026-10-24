package postgres_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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

type setupReturnFixture struct {
	claimJourney
	directory        *directory.Service
	keys             *security.Keys
	request          claimweb.Request
	start            claimweb.SetupStart
	original, signed claimweb.Signed
	pending          claimweb.Pending
	approval         claims.Approval
	token            string
	exchange         serverreturn.SetupExchange
}

func newSetupReturn(t *testing.T, address string) setupReturnFixture {
	t.Helper()
	f := currentClaimJourney(t)
	f.input.Purpose = "setup"
	keys, e := fixtureKeys()
	if e != nil {
		t.Fatal(e)
	}
	keys.SealKey = bytes.Repeat([]byte{2}, 32)
	dir := directory.New(f.store, keys)
	claimHandler := &httpapi.ClaimHandler{Service: f.service, CaptureAuthority: f.store.CaptureClaimAuthority, Web: &httpapi.ClaimWeb{Store: f.store, Keys: keys, Audience: "https://hosted.example"}}
	f.handler = (&httpapi.Handler{Claims: claimHandler, Directory: dir, ServerReturns: &httpapi.ServerReturns{Store: f.store, Directory: dir}}).Routes()
	now := time.Now().UTC().Truncate(time.Microsecond)
	q := claimweb.Request{Kind: "portico.claim.web.v2", Audience: "https://hosted.example", OperationID: f.input.OperationID, ServerID: f.input.ServerID, PublicKey: f.input.PublicKey, LocalGeneration: 7, Name: "Actual fixture server", Address: address, Purpose: "setup", IssuedAt: now, ExpiresAt: now.Add(4 * time.Minute)}
	f.input.LocalGeneration = q.LocalGeneration
	original := ownerSigned(t, f.key, q)
	raw, _ := base64.RawURLEncoding.DecodeString(original.Payload)
	start := claimweb.SetupStart{Kind: claimweb.SetupStartKind, SetupRequestID: "82a879b3-abf5-48c6-bba0-02ffd762f99c", SetupRevision: 3, Audience: q.Audience, ClaimHash: base64.RawURLEncoding.EncodeToString(security.Hash(string(raw))), OperationID: q.OperationID, ServerID: q.ServerID, PublicKey: q.PublicKey, LocalGeneration: q.LocalGeneration, Address: q.Address, Purpose: "setup", State: strings.Repeat("s", 43), CodeChallenge: serverreturn.Challenge(strings.Repeat("v", 43)), InstallationID: strings.Repeat("i", 43), ServerChallenge: strings.Repeat("c", 43), IssuedAt: now, ExpiresAt: q.ExpiresAt}
	signed := ownerSigned(t, f.key, start)
	return setupReturnFixture{claimJourney: f, directory: dir, keys: keys, request: q, start: start, original: original, signed: signed}
}
func (f *setupReturnFixture) register(t *testing.T) {
	t.Helper()
	raw := returnCall(t, f.handler, "/v1/server-claims/pending", "", claimweb.PendingRequest{Payload: f.original.Payload, Signature: f.original.Signature, SetupStart: &f.signed}, 200)
	if json.Unmarshal(raw, &f.pending) != nil || f.pending.SetupTransactionID == "" || f.pending.ReturnRegistration != nil {
		t.Fatal("pending has no frozen owner transaction or invented registration")
	}
}
func (f *setupReturnFixture) approve(t *testing.T) {
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
	if json.Unmarshal(payload, &f.approval) != nil || f.approval.AccountID != f.expected.AccountID || f.approval.TargetUsername != "" || f.approval.Purpose != "setup" {
		t.Fatal("approval binding")
	}
	expiry, e := time.Parse(time.RFC3339Nano, f.approval.ExpiresAt)
	if e != nil || expiry.After(f.start.ExpiresAt) {
		t.Fatal("approval extended original start")
	}
}
func (f *setupReturnFixture) install(t *testing.T) {
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
	f.exchange = serverreturn.SetupExchange{TransactionID: f.pending.SetupTransactionID, State: f.start.State, Verifier: strings.Repeat("v", 43), RegistrationID: reg.ID, InstallationID: f.start.InstallationID, ServerChallenge: f.start.ServerChallenge, Purpose: "setup"}
}
func (f *setupReturnFixture) complete(t *testing.T) []byte {
	t.Helper()
	raw := returnCall(t, f.handler, "/v1/server-returns/"+f.pending.SetupTransactionID+"/complete", "claim-owner-access-token-fixture", serverreturn.Complete{}, 200)
	var out serverreturn.Return
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("completion")
	}
	u, e := url.Parse(out.Location)
	if e != nil || u.Scheme+"://"+u.Host != f.request.Address || u.Path != serverreturn.ReturnPath || u.RawQuery != "" {
		t.Fatal("original callback")
	}
	values, e := url.ParseQuery(u.Fragment)
	if e != nil || len(values) != 1 || values.Get("transaction") != f.pending.SetupTransactionID {
		t.Fatal("callback leaked state/code or chose arbitrary origin")
	}
	return raw
}
func TestSetupReturnActualHTTPFrozenStartInstalledExchange(t *testing.T) {
	for _, origin := range []string{"https://original.example:32500", "http://localhost:32500"} {
		t.Run(origin, func(t *testing.T) {
			f := newSetupReturn(t, origin)
			f.register(t)
			first := f.pending
			f.register(t)
			if f.pending.SetupTransactionID != first.SetupTransactionID || f.pending.Code != first.Code {
				t.Fatal("retry retargeted")
			}
			// No ordinary registration or challenge is needed before the actual switch.
			returnCall(t, f.handler, "/v1/server-returns/"+f.pending.SetupTransactionID+"/complete", "claim-owner-access-token-fixture", serverreturn.Complete{}, 401)
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
			if json.Unmarshal(payload, &assertion) != nil || assertion.AccountID != f.expected.AccountID || assertion.Username != "strict-owner" || assertion.Purpose != "setup" || assertion.ClaimOperationID != f.input.OperationID || assertion.RegistrationID != f.exchange.RegistrationID || assertion.InstallationID != f.start.InstallationID || assertion.Challenge != f.start.ServerChallenge || assertion.CurrentOwnerAccountID != "" || assertion.PreviousOwnerRole != "" || assertion.ExpiresAt.After(f.start.ExpiresAt) || bytes.Contains(payload, []byte("profiles")) {
				t.Fatal("signed unchanged-order identity binding")
			}
			canonical, _ := json.Marshal(assertion)
			if !bytes.Equal(payload, canonical) {
				t.Fatal("noncanonical assertion")
			}
			returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 401)
			var phase string
			var completed, consumed time.Time
			if e = f.store.DB.QueryRow(`SELECT phase,completed_at,consumed_at FROM setup_claim_starts WHERE transaction_id=$1`, f.pending.SetupTransactionID).Scan(&phase, &completed, &consumed); e != nil || phase != "consumed" || consumed.Before(completed) {
				t.Fatal("consume lifecycle", e)
			}
		})
	}
}
func TestSetupReturnRejectsRefreshAndPrivateBindingChanges(t *testing.T) {
	f := newSetupReturn(t, "https://original.example")
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
	if _, e := f.store.RegisterSetupStart(context.Background(), newer, signed, q.Audience, f.keys.SealKey); e == nil {
		t.Fatal("time-only refresh replaced frozen claim")
	}
	start = f.start
	start.State = strings.Repeat("x", 43)
	signed = ownerSigned(t, f.key, start)
	if _, e := f.store.RegisterSetupStart(context.Background(), f.original, signed, q.Audience, f.keys.SealKey); e == nil {
		t.Fatal("state rebound")
	}
	f.approve(t)
	f.install(t)
	f.complete(t)
	changes := []struct {
		name   string
		change func(*serverreturn.SetupExchange)
	}{
		{"state", func(q *serverreturn.SetupExchange) { q.State = strings.Repeat("x", 43) }},
		{"verifier", func(q *serverreturn.SetupExchange) { q.Verifier = strings.Repeat("x", 43) }},
		{"installation", func(q *serverreturn.SetupExchange) { q.InstallationID = strings.Repeat("x", 43) }},
		{"challenge", func(q *serverreturn.SetupExchange) { q.ServerChallenge = strings.Repeat("x", 43) }},
		{"registration", func(q *serverreturn.SetupExchange) { q.RegistrationID = "wrong_reg" }},
		{"transaction", func(q *serverreturn.SetupExchange) { q.TransactionID = "wrong_tx" }},
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
func TestSetupReturnConcurrentConsumeHasOneSignedGrant(t *testing.T) {
	f := newSetupReturn(t, "https://original.example")
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
			_, e := f.store.ExchangeSetupReturn(context.Background(), a, f.exchange, func(q serverreturn.Assertion) (outEnvelope trust.Envelope, err error) {
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
func TestSetupReturnLiveAuthorityFences(t *testing.T) {
	cases := []struct{ name, mutation string }{
		{"account epoch", `UPDATE accounts SET auth_epoch=auth_epoch+1 WHERE id='strict-owner'`},
		{"family successor", `UPDATE session_families SET generation=generation+1 WHERE id='strict-owner-family'`},
		{"family revoked", `UPDATE session_families SET revoked_at=clock_timestamp() WHERE id='strict-owner-family'`},
		{"family expired", `UPDATE session_families SET idle_expires_at=clock_timestamp()-interval '1 second' WHERE id='strict-owner-family'`},
		{"access expired", `UPDATE account_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE account_id='strict-owner'`},
		{"username changed", `UPDATE accounts SET username='changed-owner' WHERE id='strict-owner'`},
		{"start expired", `UPDATE setup_claim_starts SET expires_at=issued_at+interval '1 microsecond'`},
		{"server revoked", `UPDATE servers SET revoked_at=clock_timestamp(),owner_account_id=NULL,token_hash=NULL,token_ciphertext=NULL,base_url=NULL,endpoint_verified=false`},
		{"credential successor", `UPDATE servers SET credential_generation=credential_generation+1`},
		{"operation cancelled", `UPDATE claim_operations SET state='cancelled',cancelled_at=clock_timestamp()`},
		{"owner changed", `UPDATE servers SET owner_account_id='strict-other'`},
		{"route generation changed", `UPDATE networking_route_state SET generation=generation+1`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newSetupReturn(t, "https://original.example")
			f.register(t)
			f.approve(t)
			f.install(t)
			f.complete(t)
			if _, e := f.store.DB.Exec(c.mutation); e != nil {
				t.Fatal(e)
			}
			returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 401)
			var count int
			if e := f.store.DB.QueryRow(`SELECT count(*) FROM setup_claim_starts WHERE phase='consumed'`).Scan(&count); e != nil || count != 0 {
				t.Fatal("invalid authority consumed", e)
			}
		})
	}
}

func TestSetupReturnCannotCrossPurposeOrReplaceSetupBinding(t *testing.T) {
	f := newSetupReturn(t, "https://original.example")
	f.register(t)
	for _, change := range []func(*claimweb.SetupStart){
		func(v *claimweb.SetupStart) { v.SetupRequestID = "a-different-request" },
		func(v *claimweb.SetupStart) { v.SetupRevision++ },
	} {
		next := f.start
		change(&next)
		if _, err := f.store.RegisterSetupStart(context.Background(), f.original, ownerSigned(t, f.key, next), f.request.Audience, f.keys.SealKey); err == nil {
			t.Fatal("replaced setup binding accepted")
		}
	}
	if _, err := f.store.RegisterOwnerStart(context.Background(), f.original, f.signed, f.request.Audience, f.keys.SealKey); err == nil {
		t.Fatal("setup proof accepted by switch boundary")
	}
	f.approve(t)
	f.install(t)
	f.complete(t)
	wrong := serverreturn.OwnerExchange{TransactionID: f.exchange.TransactionID, State: f.exchange.State, Verifier: f.exchange.Verifier, RegistrationID: f.exchange.RegistrationID, InstallationID: f.exchange.InstallationID, ServerChallenge: f.exchange.ServerChallenge, Purpose: "switch"}
	returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, wrong, 401)
	returnCall(t, f.handler, "/v1/server-returns/exchange", f.token, f.exchange, 200)
}
