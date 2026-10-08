package networking

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Real TLS/HTTP exchange: retained-key proofs never consult the revoked bearer.
func TestClaimCleanupHTTPUsesOriginalKeyWithoutBearer(t *testing.T) {
	public, key, _ := ed25519.GenerateKey(rand.Reader)
	serverID, _ := ServerIdentity(public)
	v := Intent{Binding: Binding{OperationID: "original", ServerID: serverID, AccountID: "deleted", PublicKey: public}, Revision: 2, Stage: Cancelled, ClaimGeneration: "7", CredentialGeneration: "9"}
	calls := 0
	status := http.StatusOK
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/server-claims/cleanup" || r.Method != "POST" || r.Header.Get("Authorization") != "" {
			t.Error("incorrect cleanup transport")
		}
		var envelope proofEnvelope
		if json.NewDecoder(r.Body).Decode(&envelope) != nil {
			t.Error("bad JSON")
		}
		raw, _ := base64.RawURLEncoding.DecodeString(envelope.Payload)
		sig, _ := base64.RawURLEncoding.DecodeString(envelope.Signature)
		var wire CleanupWire
		if !ed25519.Verify(public, raw, sig) || json.Unmarshal(raw, &wire) != nil || wire.OperationID != "original" || wire.ClaimGeneration != "7" || wire.CredentialGeneration != "9" {
			t.Error("lost original proof")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == 200 {
			json.NewEncoder(w).Encode(PolicyEnvelope{Payload: "signed-response", Signature: "signature", KeyID: "pin"})
		} else {
			w.Write([]byte(`{"code":"unauthorized"}`))
		}
	}))
	defer ts.Close()
	credentials := &credentialFixture{}
	transport, e := NewHTTPTransport(ts.URL, credentials)
	if e != nil {
		t.Fatal(e)
	}
	transport.client = ts.Client()
	transport.client.Timeout = time.Second
	transport.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	now := time.Now().UTC()
	wire := CleanupWire{Kind: "portico.claim.cleanup", Version: "1", Audience: ts.URL, OperationID: v.OperationID, ServerID: v.ServerID, PublicKey: base64.RawURLEncoding.EncodeToString(public), ClaimGeneration: v.ClaimGeneration, CredentialGeneration: v.CredentialGeneration, RequestID: "nonce", IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(30 * time.Second).Format(time.RFC3339Nano)}
	raw, _ := json.Marshal(wire)
	proof := SignedProof{Payload: raw, Signature: ed25519.Sign(key, raw)}
	out, e := transport.ClaimCleanup(fixtureClaimContext(t), v, proof)
	if e != nil || out.Payload != "signed-response" || credentials.calls != 0 || calls != 1 {
		t.Fatalf("exchange %v %v %d", out, e, calls)
	}
	status = 401
	if _, e = transport.ClaimCleanup(fixtureClaimContext(t), v, proof); e == nil || e == ErrCancelled || e == ErrStale {
		t.Fatalf("401 became terminal authority: %v", e)
	}
	if _, e = transport.ClaimCleanup(context.Background(), v, proof); e == nil {
		t.Fatal("no lifecycle fence accepted")
	}
	if calls != 2 {
		t.Fatal("ungated network request")
	}
}
