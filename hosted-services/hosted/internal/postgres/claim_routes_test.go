package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/trust"
)

type claimJourney struct {
	currentFamilyFixture
	service *claims.Service
	handler http.Handler
	key     ed25519.PrivateKey
	input   claims.ApprovalInput
}

func currentClaimJourney(t *testing.T) claimJourney {
	t.Helper()
	f := currentFamilySetup(t)
	ctx := context.Background()
	if _, e := f.store.DB.ExecContext(ctx, `INSERT INTO profiles(id,account_id,name) VALUES('claim_profile','strict-owner','Owner'),('other_profile','strict-other','Other')`); e != nil {
		t.Fatal(e)
	}
	token := "claim-owner-access-token-fixture"
	f.expected.TokenHash = security.Hash(token)
	if _, e := f.store.DB.ExecContext(ctx, `INSERT INTO account_sessions(token_hash,account_id,family_id,expires_at) VALUES($1,'strict-owner','strict-owner-family',clock_timestamp()+interval '15 minutes')`, f.expected.TokenHash); e != nil {
		t.Fatal(e)
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	server, e := claims.ServerID(key.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	hosted := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	keys, e := fixtureKeys()
	if e != nil {
		t.Fatal(e)
	}
	keys.PrivateKey = hosted
	keys.KeyID = "hosted_fixture"
	root := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, 32))
	keys.RootPublicKey = root.Public().(ed25519.PublicKey)
	keys.Certificate, e = trust.Certify(root, hosted.Public().(ed25519.PublicKey), keys.KeyID, "documents", time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 1, nil)
	if e != nil {
		t.Fatal(e)
	}
	keys.SealKey = bytes.Repeat([]byte{2}, 32)
	service, e := claims.New(f.store, keys, "https://hosted.example")
	if e != nil {
		t.Fatal(e)
	}
	return claimJourney{f, service, &httpapi.ClaimHandler{Service: service, CaptureAuthority: f.store.CaptureClaimAuthority}, key, claims.ApprovalInput{OperationID: "operation_a", ServerID: server, PublicKey: base64.RawURLEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), Name: "Server", OwnerProfileID: "claim_profile"}}
}
func claimHTTP(t *testing.T, h http.Handler, path, token string, input, output any, want int) {
	t.Helper()
	raw, e := json.Marshal(input)
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s returned %d, expected %d", path, w.Code, want)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("claim response cacheable")
	}
	if output != nil && json.Unmarshal(w.Body.Bytes(), output) != nil {
		t.Fatal("invalid response JSON")
	}
}
func journeyApproval(t *testing.T, f claimJourney) claims.Approval {
	t.Helper()
	var signed claims.Envelope
	claimHTTP(t, f.handler, "/v1/server-claims/approve", "claim-owner-access-token-fixture", f.input, &signed, 200)
	raw, e := base64.RawURLEncoding.DecodeString(signed.Payload)
	if e != nil {
		t.Fatal(e)
	}
	signature, e := base64.RawURLEncoding.DecodeString(signed.Signature)
	if e != nil {
		t.Fatal(e)
	}
	hosted := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	if signed.KeyID != "hosted_fixture" || !ed25519.Verify(hosted.Public().(ed25519.PublicKey), raw, signature) {
		t.Fatal("unverified approval")
	}
	var a claims.Approval
	if json.Unmarshal(raw, &a) != nil {
		t.Fatal("invalid approval")
	}
	return a
}
func journeyProof(t *testing.T, f claimJourney, a claims.Approval, purpose string) ([]byte, []byte) {
	t.Helper()
	now := time.Now().UTC()
	w := claims.Wire{Kind: claims.NonceRequest, Version: "1", Audience: a.Audience, Purpose: purpose, OperationID: a.OperationID, ServerID: a.ServerID, AccountID: a.AccountID, PublicKey: a.PublicKey, LocalGeneration: a.LocalGeneration, ApprovalRevision: a.ApprovalRevision, RequestID: security.RandomID("req_"), IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(25 * time.Second).Format(time.RFC3339Nano)}
	raw, e := json.Marshal(w)
	if e != nil {
		t.Fatal(e)
	}
	sig := ed25519.Sign(f.key, raw)
	var nonce claims.Challenge
	claimHTTP(t, f.handler, "/v1/server-claims/nonce", "", struct {
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}{base64.RawURLEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(sig)}, &nonce, 200)
	w.Kind = claims.ClaimProof
	w.NonceID = nonce.NonceID
	w.Nonce = nonce.Nonce
	w.IssuedAt = nonce.IssuedAt.UTC().Format(time.RFC3339Nano)
	w.ExpiresAt = nonce.ExpiresAt.UTC().Format(time.RFC3339Nano)
	raw, e = json.Marshal(w)
	if e != nil {
		t.Fatal(e)
	}
	return raw, ed25519.Sign(f.key, raw)
}
func cancelAssertion(t *testing.T, f claimJourney, generation string) ([]byte, []byte) {
	t.Helper()
	mode := "intent"
	if generation != "" {
		mode = "committed"
	}
	w := claims.CancelWire{Kind: "portico.claim.cancel", Version: "1", Audience: "https://hosted.example", OperationID: f.input.OperationID, ServerID: f.input.ServerID, AccountID: f.expected.AccountID, PublicKey: f.input.PublicKey, LocalGeneration: f.input.LocalGeneration, RequestID: "cancel_" + f.input.OperationID, Mode: mode, ClaimGeneration: generation}
	raw, e := json.Marshal(w)
	if e != nil {
		t.Fatal(e)
	}
	return raw, ed25519.Sign(f.key, raw)
}

func TestCurrentHostedClaimMiddlewareJourney(t *testing.T) {
	f := currentClaimJourney(t)
	keys := &security.Keys{PrivateKey: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32)), KeyID: "hosted_fixture", SealKey: bytes.Repeat([]byte{2}, 32)}
	root := (&httpapi.Handler{Claims: f.handler, Identity: identity.New(f.store), Directory: directory.New(f.store, keys), Origins: map[string]bool{"https://web.getportico.tv": true}}).Routes()
	f.handler = root
	a := journeyApproval(t, f)
	raw, sig := journeyProof(t, f, a, claims.Finalize)
	var commit claims.Commit
	claimHTTP(t, root, "/v1/server-claims/finalize", "", proofEnvelope(raw, sig), &commit, 200)
	raw, sig = journeyProof(t, f, a, claims.Result)
	var result claims.ResultValue
	claimHTTP(t, root, "/v1/server-claims/result", "", proofEnvelope(raw, sig), &result, 200)
	request := func(method, path, token, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		root.ServeHTTP(w, r)
		return w
	}
	// The server's own control-plane call (its membership push) is admitted
	// only once the claim is acknowledged, and keeps the origin boundary.
	push := func(token, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/servers/"+commit.ServerID+"/members", strings.NewReader(`{"after":0,"through":0,"full":true,"members":[]}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		root.ServeHTTP(w, r)
		return w
	}
	if w := push(result.ServerCredential, ""); w.Code != 403 {
		t.Fatal("pre-ack normal authority", w.Code)
	}
	claimHTTP(t, root, "/v1/server-claims/ack", result.ServerCredential, commit, nil, 200)
	// The claiming account's devices find the new server at their next check.
	var listVersion int64
	if e := f.store.DB.QueryRow(`SELECT server_list_version FROM accounts a JOIN servers s ON s.owner_account_id=a.id WHERE s.id=$1`, commit.ServerID).Scan(&listVersion); e != nil || listVersion == 0 {
		t.Fatal("claim did not move the owner's list version", listVersion, e)
	}
	w := push(result.ServerCredential, "https://web.getportico.tv")
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "https://web.getportico.tv" {
		t.Fatal("current server middleware", w.Code, w.Body.String())
	}
	if w = push(result.ServerCredential, "https://evil.example"); w.Code != 403 {
		t.Fatal("origin bypass", w.Code)
	}
	if w = request("POST", "/v1/servers", "claim-owner-access-token-fixture", ""); w.Code != 405 {
		t.Fatal("legacy enrollment route remains", w.Code)
	}
	if w = request("POST", "/v1/account/deletion", "claim-owner-access-token-fixture", ""); w.Code != 503 {
		t.Fatal("erasure accepted before cleanup", w.Code)
	}
	raw, sig = cancelAssertion(t, f, commit.ClaimGeneration)
	claimHTTP(t, root, "/v1/server-claims/cancel", "", proofEnvelope(raw, sig), nil, 200)
	if w = push(result.ServerCredential, ""); w.Code != 403 {
		t.Fatal("cancelled bearer admitted", w.Code)
	}
}
func proofEnvelope(raw, sig []byte) any {
	return struct {
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}{base64.RawURLEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(sig)}
}
