package networking

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestDirectRouteTransportRegistersExactInboundProofAndRetiresIt(t *testing.T) {
	for _, address := range []string{"188.68.34.120", "2605:a144:2327:3137::1"} {
		t.Run(address, func(t *testing.T) {
			f, cert, ctx := certificateFixture(t)
			_, q, _, err := cert.snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			h := &ClaimHandler{store: f.store, runner: cert.runner, signer: modelSigner{f.localKey}, endpoints: endpointRegistrations{pending: make(map[string]endpointPending)}}
			tr := cert.transport
			tr.routeProofs = &h.endpoints
			port := 42424
			if netip.MustParseAddr(address).Is6() {
				port = 32500
			}
			var registration string
			tr.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
				var in struct {
					Address        string `json:"address"`
					Port           int    `json:"port"`
					RegistrationID string `json:"registrationId"`
				}
				if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
					t.Fatal(err)
				}
				if in.Address != address || in.Port != port {
					t.Fatal("changed advertised family or port")
				}
				registration = in.RegistrationID
				if id, err := base64.RawURLEncoding.Strict().DecodeString(registration); err != nil || len(id) != 32 {
					t.Fatal("missing proof registration")
				}
				challenge := endpointChallenge{Kind: "portico.server.endpoint.v1", ServerID: q.Scope.Intent.ServerID, AccountID: q.Scope.Intent.AccountID, OperationID: q.Scope.Intent.OperationID, ClaimGeneration: q.Scope.Intent.ClaimGeneration, CredentialGeneration: q.Scope.Intent.CredentialGeneration, BaseURL: routeURL(netip.MustParseAddr(address), port, "https"), RegistrationID: registration, Nonce: "fresh_nonce", ExpiresAt: time.Now().Add(30 * time.Second).UTC()}
				raw, _ := json.Marshal(challenge)
				req := httptest.NewRequest(http.MethodPost, "/v1/networking/endpoint-proof", bytes.NewReader(raw))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				h.endpointProof(w, req)
				if w.Code != 200 {
					t.Fatalf("inbound proof denied: %d %s", w.Code, w.Body.String())
				}
				var proof struct {
					Payload   string `json:"payload"`
					Signature string `json:"signature"`
				}
				_ = json.Unmarshal(w.Body.Bytes(), &proof)
				signature, _ := base64.RawURLEncoding.DecodeString(proof.Signature)
				if proof.Payload != base64.RawURLEncoding.EncodeToString(raw) || !ed25519.Verify(f.localKey.Public().(ed25519.PublicKey), raw, signature) {
					t.Fatal("inbound key proof did not bind literal and port")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader([]byte(`{}`)))}, nil
			})
			if err := tr.certificateCall(ctx, q.Scope.Intent, "route", "", map[string]any{"address": address, "port": port}, &struct{}{}); err != nil {
				t.Fatal(err)
			}
			h.endpoints.Lock()
			remaining := len(h.endpoints.pending)
			h.endpoints.Unlock()
			if registration == "" || remaining != 0 {
				t.Fatal("ephemeral registration leaked")
			}
		})
	}
}

func TestUnprovenIPv4DoesNotPreventProvenIPv6Candidate(t *testing.T) {
	f, m, topo, _, ctx := contactFixture(t)
	m.h.endpoints.pending = make(map[string]endpointPending)
	cert := m.h.certificates
	installSyntheticCertificate(t, cert, ctx)
	cert.SetListening(32500, true)
	_, q, _, _, key, err := m.snapshot(ctx)
	clear(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE networking_remote_settings SET enabled=1,mapping=0`); err != nil {
		t.Fatal(err)
	}
	base := routeHostedFake(t, cert, ctx)
	fallback := cert.transport.client.Transport
	ipv4Attempts := 0
	cert.transport.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/direct-route") {
			raw, _ := io.ReadAll(r.Body)
			var in struct {
				Address string `json:"address"`
			}
			_ = json.Unmarshal(raw, &in)
			r.Body = io.NopCloser(bytes.NewReader(raw))
			if in.Address == "188.68.34.120" {
				ipv4Attempts++
				return reply(409, `{"error":{"code":"address_unverified"}}`), nil
			}
		}
		return fallback.RoundTrip(r)
	})
	topo.Public = []string{"188.68.34.120", "2a03:4000:6:8::1"}
	_ = m.step(ctx)
	status := remoteStatusOf(t, f)
	if ipv4Attempts != 1 || base.names != 1 || status.IPv6 != "verified" {
		t.Fatalf("one failed family suppressed the other: attempts=%d names=%d status=%+v", ipv4Attempts, base.names, status)
	}
	for _, candidate := range status.Candidates {
		if strings.HasPrefix(candidate.BaseURL, "https://c4-") {
			t.Fatal("unproven IPv4 candidate published")
		}
	}
	if got := reportedAddresses(t, f, q.Scope.ID); got[familyIPv4] != "" || got[familyIPv6] == "" {
		t.Fatal("wrong published families", got)
	}
}
