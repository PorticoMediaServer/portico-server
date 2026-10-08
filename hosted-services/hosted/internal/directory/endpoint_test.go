package directory

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type endpointRepo struct {
	Repository
	called bool
	err    error
}

func (r *endpointRepo) RegisterEndpoint(_ context.Context, a ServerAuthority, e EndpointEvidence) (EndpointReceipt, error) {
	r.called = true
	return EndpointReceipt{e.BaseURL, time.Now().UTC()}, r.err
}

type endpointProofClient func(*http.Request) (*http.Response, error)

func (f endpointProofClient) Do(r *http.Request) (*http.Response, error) { return f(r) }
func TestEndpointExactProofAndCommitAuthority(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	a := ServerAuthority{ServerID: "server", AccountID: "account", OperationID: "op", PublicKey: pub, TokenHash: make([]byte, 32), ClaimGeneration: 1, CredentialGeneration: 2}
	for _, mode := range []string{"good", "mutated", "bad-signature", "stale-commit", "redirect", "custom-port"} {
		t.Run(mode, func(t *testing.T) {
			origin := "https://demo.example.com"
			if mode == "custom-port" {
				origin += ":444"
			}
			repo := &endpointRepo{}
			if mode == "stale-commit" {
				repo.err = ErrDenied
			}
			s := NewWithOptions(repo, nil, Options{ProofClient: endpointProofClient(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != origin+"/v1/networking/endpoint-proof" || r.Header.Get("Authorization") != "" {
					t.Fatal("unsafe proof request")
				}
				raw, _ := io.ReadAll(r.Body)
				var q EndpointChallenge
				if json.Unmarshal(raw, &q) != nil || q.OperationID != a.OperationID || q.CredentialGeneration != "2" || q.Nonce == "" {
					t.Fatal("wrong binding")
				}
				if mode == "mutated" {
					q.OperationID = "other"
					raw, _ = json.Marshal(q)
				}
				sig := ed25519.Sign(key, raw)
				if mode == "bad-signature" {
					sig[0] ^= 1
				}
				body, _ := json.Marshal(struct {
					Payload   string `json:"payload"`
					Signature string `json:"signature"`
				}{base64.RawURLEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(sig)})
				status := 200
				if mode == "redirect" {
					status = 302
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})})
			_, err := s.RegisterEndpoint(context.Background(), a, EndpointRequest{origin, base64.RawURLEncoding.EncodeToString(make([]byte, 32))})
			if (err == nil) != (mode == "good" || mode == "custom-port") {
				t.Fatal(mode, err)
			}
			if repo.called != (mode == "good" || mode == "custom-port" || mode == "stale-commit") {
				t.Fatal("unexpected persistence")
			}
		})
	}
}
func TestEndpointRejectsUnsafeOriginBeforeFetch(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	a := ServerAuthority{ServerID: "s", AccountID: "a", OperationID: "o", PublicKey: pub, TokenHash: make([]byte, 32), ClaimGeneration: 1, CredentialGeneration: 1}
	s := NewWithOptions(&endpointRepo{}, nil, Options{ProofClient: endpointProofClient(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected request"); return nil, nil })})
	for _, origin := range []string{"http://demo.example.com", "https://127.0.0.1", "https://169.254.169.254", "https://demo.example.com:0", "https://user@demo.example.com", "https://demo.example.com/path", "https://demo.local", "https://demo.example.com/"} {
		if _, err := s.RegisterEndpoint(context.Background(), a, EndpointRequest{origin, base64.RawURLEncoding.EncodeToString(make([]byte, 32))}); err == nil {
			t.Fatal(origin)
		}
	}
}
