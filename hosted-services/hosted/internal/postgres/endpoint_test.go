package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"strings"
	"testing"
)

type currentEndpointProof func(*http.Request) (*http.Response, error)

func (f currentEndpointProof) Do(r *http.Request) (*http.Response, error) { return f(r) }
func TestCurrentEndpointClaimAckDiscoveryAndLateCancellation(t *testing.T) {
	f := currentClaimJourney(t)
	keys := &security.Keys{PrivateKey: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32)), KeyID: "hosted_fixture", SealKey: bytes.Repeat([]byte{2}, 32)}
	cancelDuringProof := false
	var commit claims.Commit
	service := directory.NewWithOptions(f.store, keys, directory.Options{ProofClient: currentEndpointProof(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		var q directory.EndpointChallenge
		if json.Unmarshal(raw, &q) != nil || q.ServerID != f.input.ServerID || q.OperationID != commit.OperationID || q.ClaimGeneration != commit.ClaimGeneration || q.CredentialGeneration != commit.CredentialGeneration || r.Header.Get("Authorization") != "" {
			t.Fatal("incorrect proof binding")
		}
		body, _ := json.Marshal(proofEnvelope(raw, ed25519.Sign(f.key, raw)))
		if cancelDuringProof {
			assertion, sig := cancelAssertion(t, f, commit.ClaimGeneration)
			claimHTTP(t, f.handler, "/v1/server-claims/cancel", "", proofEnvelope(assertion, sig), nil, 200)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})})
	root := (&httpapi.Handler{Claims: f.handler, Identity: identity.New(f.store), Directory: service}).Routes()
	f.handler = root
	approval := journeyApproval(t, f)
	raw, sig := journeyProof(t, f, approval, claims.Finalize)
	claimHTTP(t, root, "/v1/server-claims/finalize", "", proofEnvelope(raw, sig), &commit, 200)
	raw, sig = journeyProof(t, f, approval, claims.Result)
	var result claims.ResultValue
	claimHTTP(t, root, "/v1/server-claims/result", "", proofEnvelope(raw, sig), &result, 200)
	request := directory.EndpointRequest{BaseURL: "https://demo.example.com", RegistrationID: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}
	path := "/v1/servers/" + commit.ServerID + "/endpoint"
	claimHTTP(t, root, path, result.ServerCredential, request, nil, 403)
	claimHTTP(t, root, "/v1/server-claims/ack", result.ServerCredential, commit, nil, 200)
	var receipt directory.EndpointReceipt
	claimHTTP(t, root, path, result.ServerCredential, request, &receipt, 200)
	if receipt.BaseURL != request.BaseURL || receipt.VerifiedAt.IsZero() {
		t.Fatal("invalid receipt")
	}
	list, err := service.List(context.Background(), f.expected.AccountID)
	if err != nil || len(list) != 1 || !list[0].EndpointVerified || list[0].BaseURL != request.BaseURL {
		t.Fatal("discovery missing verified endpoint", err)
	}
	captured, err := service.CaptureServerAuthority(context.Background(), result.ServerCredential)
	if err != nil {
		t.Fatal(err)
	}
	cancelDuringProof = true
	request.BaseURL = "https://other.example.com"
	claimHTTP(t, root, path, result.ServerCredential, request, nil, 403)
	list, err = service.List(context.Background(), f.expected.AccountID)
	if err != nil || len(list) != 0 {
		t.Fatal("cancelled endpoint discoverable", err)
	}
	var origin string
	if err = f.store.DB.QueryRow(`SELECT origin FROM current_server_endpoints WHERE server_id=$1`, commit.ServerID).Scan(&origin); err != nil || origin != "https://demo.example.com" {
		t.Fatal("late endpoint mutated stored evidence", err)
	}
	if _, err = f.store.RegisterEndpoint(context.Background(), captured, directory.EndpointEvidence{BaseURL: request.BaseURL, Digest: make([]byte, 32), ExpiresAt: receipt.VerifiedAt}); err == nil {
		t.Fatal("stale captured authority admitted")
	}
}
