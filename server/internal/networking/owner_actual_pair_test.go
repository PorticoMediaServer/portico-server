package networking_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"portico.local/server/internal/access"
	"portico.local/server/internal/authoritygate"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/claimweb"
	"portico.local/server/internal/hosted"
	trust "portico.local/server/internal/hostedtrust"
	"portico.local/server/internal/httpapi"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/serverreturn"
)

type ownerPairReply struct {
	Status                             int
	Headers                            http.Header
	Body                               json.RawMessage
	BodyEmpty                          bool
	FinalizeRequests, ExchangeRequests int
}
type ownerPairBridge struct {
	t                                     *testing.T
	cmd                                   *exec.Cmd
	in                                    io.WriteCloser
	out                                   *bufio.Scanner
	mu                                    sync.Mutex
	approved                              chan struct{}
	Root, AccountID, OwnerToken, Audience string
}

// The private producer is an actual checked Hosted test executable. Every
// ephemeral credential and raw response stays in these pipes, even on failure.
func newOwnerPairBridge(t *testing.T) *ownerPairBridge {
	t.Helper()
	binary := os.Getenv("PORTICO_OWNER_CONTINUATION_BINARY")
	if binary == "" {
		t.Skip("checked Hosted executable and isolated PostgreSQL required for actual pair")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestOwnerContinuationActualProducerBridge$", "-test.count=1")
	cmd.Env = append(os.Environ(), "PORTICO_OWNER_CONTINUATION_BRIDGE=1")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("private producer input unavailable")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("private producer output unavailable")
	}
	// No child diagnostic bytes are copied to logs: a failing producer may
	// include a raw response in its own test diagnostics.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		t.Fatal("private producer failed to start")
	}
	b := &ownerPairBridge{t: t, cmd: cmd, in: in, out: bufio.NewScanner(out), approved: make(chan struct{})}
	b.out.Buffer(make([]byte, 4096), 256<<10)
	t.Cleanup(func() {
		b.mu.Lock()
		_ = json.NewEncoder(b.in).Encode(map[string]string{"Action": "stop"})
		_ = b.in.Close()
		b.mu.Unlock()
		if b.cmd.Wait() != nil {
			t.Error("private producer failed or did not stop cleanly")
		}
	})
	var hello struct{ Root, AccountID, OwnerToken, Audience string }
	b.read(&hello)
	b.Root, b.AccountID, b.OwnerToken, b.Audience = hello.Root, hello.AccountID, hello.OwnerToken, hello.Audience
	root, err := base64.RawURLEncoding.Strict().DecodeString(b.Root)
	if err != nil || len(root) != 32 || len(b.OwnerToken) != 43 || b.AccountID == "" || b.Audience != "https://hosted.example" {
		t.Fatal("invalid private producer initialization")
	}
	return b
}
func (b *ownerPairBridge) read(out any) {
	b.t.Helper()
	for b.out.Scan() {
		line := b.out.Text()
		if strings.HasPrefix(line, "PORTICO_OWNER_JSON:") {
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "PORTICO_OWNER_JSON:")), out) != nil {
				b.t.Fatal("invalid private producer reply")
			}
			return
		}
	}
	b.t.Fatal("private producer stopped before its reply")
}
func (b *ownerPairBridge) call(command any) ownerPairReply {
	b.t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if json.NewEncoder(b.in).Encode(command) != nil {
		b.t.Fatal("private producer command failed")
	}
	var out ownerPairReply
	b.read(&out)
	return out
}
func (b *ownerPairBridge) http(method, path, token string, body any) ownerPairReply {
	b.t.Helper()
	return b.call(map[string]any{"Action": "http", "Method": method, "Path": path, "Token": token, "Body": body})
}
func (b *ownerPairBridge) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.Host != "hosted.example" {
		return nil, errors.New("unexpected pair destination")
	}
	if r.URL.Path == "/v1/server-claims/events" {
		// Delay only this bounded network request until the real HTTP approval
		// commits. The serial private driver cannot process approval behind an
		// open event wait. No approval/status/claim bytes are fabricated here.
		select {
		case <-b.approved:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var body any
	if len(raw) > 0 && json.Unmarshal(raw, &body) != nil {
		return nil, errors.New("invalid pair HTTP request")
	}
	out := b.call(map[string]any{"Action": "http", "Method": r.Method, "Path": r.URL.RequestURI(), "Headers": r.Header, "Body": body})
	payload := out.Body
	if out.BodyEmpty {
		payload = nil
	}
	return &http.Response{StatusCode: out.Status, Header: out.Headers, Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload)), Request: r}, nil
}

func TestActualOwnerContinuationInstallsAndSpendsOneGenuineHostedGrant(t *testing.T) {
	b := newOwnerPairBridge(t)
	initial := b.call(map[string]string{"Action": "snapshot"})
	var empty struct {
		Server      any            `json:"server"`
		OwnerStarts []any          `json:"ownerStarts"`
		Operations  []any          `json:"operations"`
		Counts      map[string]int `json:"counts"`
	}
	if initial.Status != 200 || json.Unmarshal(initial.Body, &empty) != nil || empty.Server != nil || len(empty.OwnerStarts) != 0 || len(empty.Operations) != 0 || len(empty.Counts) != 6 {
		t.Fatal("pair did not start with an empty claim/registration namespace")
	}
	for _, count := range empty.Counts {
		if count != 0 {
			t.Fatal("pair started with invented claim/member/route state")
		}
	}
	ctx := context.Background()
	state := t.TempDir()
	db, err := persistence.Open(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	gate, err := authoritygate.NewAuthorityGate(strings.Repeat("a", 64), false)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := networking.NewAuthorityRunner(func(ctx context.Context) (networking.LifecycleLease, error) { return gate.Acquire(ctx) })
	if err != nil {
		t.Fatal(err)
	}
	keys, err := networking.OpenCurrentKeys(ctx, db, state, runner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = keys.Close() })
	id, err := identity.New(db, state)
	if err != nil {
		t.Fatal(err)
	}
	const installation = "f89bfbc4-cd90-4cda-a925-a123bb4a8b8a"
	issuing, err := identity.WithIssuingDevice(ctx, identity.DeviceRegistration{InstallationID: installation, Name: "Owner browser", Platform: "web", App: "portico"}, "127.0.0.1:1234")
	if err != nil {
		t.Fatal(err)
	}
	setupToken, err := id.BrowserSetupToken(issuing)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := id.SetupWithOptions(issuing, identity.SetupOptions{SetupToken: setupToken, AuthMode: "local", Username: "owner", Password: "Pair-" + identity.Token() + "9!", Name: "Genuine paired server"}, true)
	if err != nil {
		t.Fatal(err)
	}
	root, err := base64.RawURLEncoding.DecodeString(b.Root)
	if err != nil {
		t.Fatal("private root unavailable")
	}
	control, err := hosted.New(db, id, b.Audience, b.Root, trust.KeyID(root))
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := networking.NewApprovalVerifier(b.Audience, trust.KeyID(root), root, control.ValidateTrust)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := networking.OpenClaimCipher(ctx, db, state)
	if err != nil {
		t.Fatal(err)
	}
	store, err := networking.NewSQLiteStore(db, cipher, networking.GuardLocalOwner, networking.GuardInstalledClaim, verifier, b.Audience)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := networking.NewHTTPTransport(b.Audience, store)
	if err != nil {
		t.Fatal(err)
	}
	transport.UseRoundTripper(b)
	if err = store.SetTerminalRevoker(control.RevokeClaimTx); err != nil {
		t.Fatal(err)
	}
	if err = control.UseCurrentClaims(store, runner, transport); err != nil {
		t.Fatal(err)
	}
	if err = control.ConfigureIdentityDepartures(keys); err != nil {
		t.Fatal(err)
	}
	id.PrepareIdentityDeparture = control.PrepareIdentityDeparture
	claims, err := networking.NewClaimHandler(store, runner, transport, keys, httpapi.NetworkingClaimAuthorizer(id, nil), id.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err = claims.ConfigureCertificates(ctx, state, networking.CertificateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = claims.ConfigureRemote(ctx, "0.0.0.0:32500", control.NetworkChanged); err != nil {
		t.Fatal(err)
	}
	api := httpapi.New(httpapi.Dependencies{DB: db, Identity: id, Catalog: catalog.New(db), Hosted: control, Networking: claims, Console: operations.New(db), Access: httpapi.AccessArea{Access: access.New(db)}, Origins: []string{"https://server.example"}})
	request := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		defer clear(raw)
		r := httptest.NewRequest(method, "https://server.example"+path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://server.example")
		r.Header.Set("Authorization", "Bearer "+setup.AccessToken)
		r.Header.Set("X-Portico-Installation-Id", installation)
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		return w
	}
	read := func(w *httptest.ResponseRecorder, out any) {
		t.Helper()
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), out) != nil {
			t.Fatalf("actual local handler failed: status=%d", w.Code)
		}
	}
	var move identity.AccountMove
	read(request("POST", "/v1/admin/sign-in/move/preview", identity.MovePreviewRequest{To: identity.SignInPortico}), &move)
	if w := request("POST", "/v1/admin/sign-in/move/confirm", identity.MoveConfirmRequest{MoveID: move.ID, ExpectedRevision: move.Revision}); w.Code != 200 {
		t.Fatalf("actual move confirmation status=%d", w.Code)
	}
	var status networking.ClaimStatus
	read(request("GET", "/v1/networking/claim", nil), &status)
	read(request("POST", "/v1/networking/claim/prepare", networking.ClaimPrepareRequest{Purpose: "switch", Expected: status.Identity}), &status)
	if status.Operation == nil {
		t.Fatal("actual claim operation absent")
	}
	stateBytes, verifierBytes := sha256.Sum256([]byte(identity.Token())), sha256.Sum256([]byte(identity.Token()))
	stateValue, pkce := base64.RawURLEncoding.EncodeToString(stateBytes[:]), base64.RawURLEncoding.EncodeToString(verifierBytes[:])
	challenge := sha256.Sum256([]byte(pkce))
	read(request("POST", "/v1/networking/claim/web-approval", networking.ClaimActionRequest{Expected: *status.Operation, OwnerStart: &networking.ClaimOwnerStartRequest{State: stateValue, CodeChallenge: base64.RawURLEncoding.EncodeToString(challenge[:]), InstallationID: installation}}), &status)
	if status.OwnerContinuation == nil || status.OwnerContinuation.Claim == nil || status.OwnerContinuation.Start == nil || status.ClaimCode == "" {
		t.Fatal("actual full pre-handoff capture absent")
	}
	_, start, err := claimweb.VerifyOwnerStart(*status.OwnerContinuation.Claim, *status.OwnerContinuation.Start, b.Audience, time.Now())
	if err != nil {
		t.Fatal("actual initiating proofs invalid")
	}
	transaction := status.OwnerContinuation.TransactionID
	approved := b.http("POST", "/v1/server-claims/approve-code", b.OwnerToken, map[string]string{"code": status.ClaimCode})
	if approved.Status != 200 {
		t.Fatalf("actual approval status=%d", approved.Status)
	}
	close(b.approved)
	read(request("POST", "/v1/networking/claim/await-approval", map[string]any{"expected": status.Operation}), &status)
	if status.State != "installed" || !status.InstallationAcknowledged || status.Operation == nil {
		t.Fatal("actual approval did not install and ACK")
	}
	for _, path := range []string{"/v1/direct/account", "/v1/admin/sign-in", "/v1/admin/access/members"} {
		if w := request("GET", path, nil); w.Code != 401 && w.Code != 403 {
			t.Fatalf("narrow continuation reached unrelated authority: status=%d", w.Code)
		}
	}
	origin, generation := networking.OwnerPairHTTPSForTest(t, claims)
	var installed networking.Intent
	if err = runner.Do(ctx, func(ctx context.Context) error {
		var e error
		installed, e = store.InstalledIntent(ctx)
		if e != nil {
			return e
		}
		secret, e := store.InstalledCredential(ctx, installed)
		if e != nil {
			return e
		}
		defer secret.Clear()
		return secret.Use(func(token []byte) error {
			out := b.call(map[string]any{"Action": "verified-route-fact", "ServerID": id.ID(), "Origin": origin, "Generation": generation, "Token": string(token)})
			if out.Status != 204 && out.Status != 200 {
				return errors.New("actual installed route fact refused")
			}
			return nil
		})
	}); err != nil {
		t.Fatal("installed route fact failed", err)
	}
	read(request("POST", "/v1/networking/claim/continue", networking.ClaimActionRequest{Expected: *status.Operation}), &status)
	if status.OwnerContinuation == nil || status.OwnerContinuation.ReturnRegistration == nil {
		t.Fatal("actual installed resolver registration absent")
	}
	registration := *status.OwnerContinuation.ReturnRegistration
	completed := b.http("POST", "/v1/server-returns/"+url.PathEscape(transaction)+"/complete", b.OwnerToken, map[string]any{})
	var callback struct {
		Location string `json:"location"`
	}
	if completed.Status != 200 || json.Unmarshal(completed.Body, &callback) != nil {
		t.Fatalf("actual owner completion status=%d", completed.Status)
	}
	u, err := url.Parse(callback.Location)
	if err != nil || u.Scheme != "https" || u.Host != "server.example" || u.Path != "/auth/portico-return" || u.RawQuery != "" || u.Fragment != "transaction="+transaction {
		t.Fatal("owner callback was not transaction-only on original origin")
	}
	q := serverreturn.OwnerExchange{TransactionID: transaction, State: stateValue, Verifier: pkce, RegistrationID: registration.ID, InstallationID: installation, ServerChallenge: start.ServerChallenge, Purpose: "switch"}
	var admitted identity.DirectSignIn
	read(request("POST", "/v1/auth/portico-return/exchange", q), &admitted)
	if admitted.Session == nil || admitted.Account.ID != setup.Viewer.AccountID || admitted.ServerID != id.ID() || admitted.Account.HostedAccountID != b.AccountID {
		t.Fatal("actual owner grant changed the captured account/server")
	}
	if _, err = id.AuthenticateContext(ctx, setup.AccessToken); err == nil {
		t.Fatal("initiating family survived final admission")
	}
	if _, err = id.AuthenticateContext(ctx, admitted.Session.AccessToken); err != nil {
		t.Fatal("actual completed owner grant unusable", err)
	}
	if w := request("POST", "/v1/auth/portico-return/exchange", q); w.Code == 200 {
		t.Fatal("consumed owner exchange replay admitted")
	}
	snapshot := b.call(map[string]string{"Action": "snapshot", "ServerID": id.ID()})
	if snapshot.Status != 200 || snapshot.FinalizeRequests != 1 || snapshot.ExchangeRequests != 1 {
		t.Fatal("owner pair repeated central claim or exchange")
	}
	var phase string
	var spent sql.NullInt64
	if err = db.QueryRow(`SELECT phase,consumed_ms FROM identity_owner_completions WHERE operation_id=?`, installed.OperationID).Scan(&phase, &spent); err != nil || phase != "completed" || !spent.Valid || spent.Int64 <= 0 {
		t.Fatal("local final spend was not durable", err)
	}
}
