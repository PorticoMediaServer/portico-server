package certificates

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/acme"
	"portico.local/hosted/internal/security"
)

type custodyRoundTrip func(*http.Request) (*http.Response, error)

func (f custodyRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCustodyRecoveryOnlyLooksUpExistingProviderAccounts(t *testing.T) {
	for _, mode := range []string{"valid", "old URL changed", "old deactivated", "candidate exists", "candidate lookup uncertain", "damaged old seal"} {
		t.Run(mode, func(t *testing.T) {
			sealKey := make([]byte, 32)
			if _, err := rand.Read(sealKey); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "seal")
			if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(sealKey)), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := &Config{Provider: "google", Environment: "production", AcceptTerms: true, SealKeyFile: path}
			keys := &security.Keys{SealKey: sealKey}
			oldKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			newKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			sealed := func(key *ecdsa.PrivateKey) []byte {
				raw, err := x509.MarshalPKCS8PrivateKey(key)
				if err != nil {
					t.Fatal(err)
				}
				defer clear(raw)
				cipher, err := keys.Seal(base64.RawStdEncoding.EncodeToString(raw), "portico/acme-account/v1/google-production")
				if err != nil {
					t.Fatal(err)
				}
				return cipher
			}
			origin := "https://dv.acme-v02.api.pki.goog"
			original := Account{Cipher: sealed(oldKey), URL: origin + "/existing-account"}
			candidate := Account{Cipher: sealed(newKey)}
			if mode == "damaged old seal" {
				original.Cipher[0] ^= 1
			}
			posts := 0
			factory := func(key *ecdsa.PrivateKey, directory string) *acme.Client {
				if directory != origin+"/directory" {
					t.Fatal("provider directory changed")
				}
				old := key.Equal(oldKey)
				transport := custodyRoundTrip(func(r *http.Request) (*http.Response, error) {
					status, body := 200, ""
					header := make(http.Header)
					header.Set("Replay-Nonce", "bm9uY2UtY3VzdG9keS1sb29rdXA")
					header.Set("Content-Type", "application/json")
					switch {
					case r.Method == "GET" && r.URL.String() == directory:
						body = `{"newNonce":"` + origin + `/nonce","newAccount":"` + origin + `/lookup","newOrder":"` + origin + `/forbidden-order"}`
					case r.Method == "HEAD" && r.URL.String() == origin+"/nonce":
						status = 204
					case r.Method == "POST" && r.URL.String() == origin+"/lookup":
						posts++
						var jws struct {
							Payload string `json:"payload"`
						}
						if json.NewDecoder(r.Body).Decode(&jws) != nil {
							t.Fatal("invalid signed lookup")
						}
						raw, err := base64.RawURLEncoding.DecodeString(jws.Payload)
						if err != nil {
							t.Fatal(err)
						}
						var payload map[string]any
						if json.Unmarshal(raw, &payload) != nil || len(payload) != 1 || payload["onlyReturnExisting"] != true {
							t.Fatal("recovery attempted provider creation")
						}
						if old {
							location := original.URL
							if mode == "old URL changed" {
								location = origin + "/different-account"
							}
							header.Set("Location", location)
							body = `{"status":"valid"}`
							if mode == "old deactivated" {
								body = `{"status":"deactivated"}`
							}
						} else if mode == "candidate exists" {
							header.Set("Location", origin+"/candidate-account")
							body = `{"status":"valid"}`
						} else if mode == "candidate lookup uncertain" {
							status = 503
							body = `{"type":"urn:ietf:params:acme:error:serverInternal","status":503}`
						} else {
							status = 400
							body = `{"type":"urn:ietf:params:acme:error:accountDoesNotExist","status":400}`
						}
					default:
						t.Fatal("recovery called a non-lookup provider endpoint")
					}
					return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				})
				return &acme.Client{Key: key, DirectoryURL: directory, HTTPClient: &http.Client{Transport: transport}, RetryBackoff: noACMERetry}
			}
			err := verifyCustodyRecovery(context.Background(), cfg, original, candidate, factory)
			if mode == "valid" {
				if err != nil || posts != 2 {
					t.Fatal("existing-account proof failed", err)
				}
			} else if err == nil {
				t.Fatal("uncertain custody accepted")
			}
			if mode == "damaged old seal" && posts != 0 {
				t.Fatal("invalid sealed custody reached provider")
			}
		})
	}
}
