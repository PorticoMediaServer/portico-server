package networking

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ClaimRequestOwner is constructed from root's current local owner principal.
// Guard must recheck that exact token family/generation in the supplied tx.
// Authorize must enforce root's origin/CSRF contract before returning this value.
type ClaimRequestOwner struct {
	Owner LocalOwner
	Guard ClaimRequestGuard
}
type ClaimRequestAuthorizer func(*http.Request) (ClaimRequestOwner, error)
type ClaimExpected struct {
	OperationID     string `json:"operationId"`
	LocalGeneration int64  `json:"localGeneration,string"`
	Revision        int64  `json:"revision,string"`
}
type ClaimIdentityExpected struct {
	ServerID        string `json:"serverId"`
	LocalGeneration int64  `json:"localGeneration,string"`
}
type ClaimApprovalRequest struct {
	OperationID      string `json:"operationId"`
	ServerID         string `json:"serverId"`
	PublicKey        string `json:"publicKey"`
	LocalGeneration  int64  `json:"localGeneration,string"`
	Name             string `json:"name"`
	ExpectedRevision int64  `json:"expectedRevision,string"`
}
type ClaimStatus struct {
	ApprovalURL              string                `json:"approvalUrl,omitempty"`
	ClaimCode                string                `json:"claimCode,omitempty"`
	State                    string                `json:"state"`
	Identity                 ClaimIdentityExpected `json:"identity"`
	Operation                *ClaimExpected        `json:"operation,omitempty"`
	AccountID                string                `json:"accountId,omitempty"`
	InstallationAcknowledged bool                  `json:"installationAcknowledged"`
	ApprovalRequired         bool                  `json:"approvalRequired"`
	Actions                  []string              `json:"actions"`
	ApprovalRequest          *ClaimApprovalRequest `json:"approvalRequest,omitempty"`
}
type ClaimHandler struct {
	webWaiters          sync.Map
	certificates        *CertificateManager
	remote              *RemoteManager
	store               *SQLiteStore
	runner              *AuthorityRunner
	transport           Transport
	signer              Signer
	authorize           ClaimRequestAuthorizer
	managementAuthorize func(*http.Request) (ClaimRequestGuard, error)
	name                func() string
	endpoints           endpointRegistrations
	failures            *failureLog
}

func NewClaimHandler(store *SQLiteStore, runner *AuthorityRunner, transport Transport, signer Signer, authorize ClaimRequestAuthorizer, name func() string) (*ClaimHandler, error) {
	if store == nil || runner == nil || transport == nil || signer == nil || authorize == nil || name == nil {
		return nil, ErrInvalid
	}
	return &ClaimHandler{store: store, runner: runner, transport: transport, signer: signer, authorize: authorize, name: name, endpoints: endpointRegistrations{pending: make(map[string]endpointPending)}, failures: sharedFailureLog}, nil
}
func (h *ClaimHandler) Register(mux *http.ServeMux) {
	if h == nil {
		return
	}
	mux.HandleFunc("POST /v1/networking/claim/await-approval", h.awaitWebClaim)
	h.registerCertificates(mux)
	h.registerRemote(mux)
	mux.Handle("GET /v1/networking/claim", h)
	mux.HandleFunc("POST /v1/networking/claim/endpoint", h.endpointRegister)
	mux.HandleFunc("POST /v1/networking/endpoint-proof", h.endpointProof)
	for _, action := range []string{"prepare", "approve", "continue", "cancel", "web-approval"} {
		mux.Handle("POST /v1/networking/claim/"+action, h)
	}
}
func (h *ClaimHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, e := h.authorize(r)
	if e != nil || !validOwner(actor.Owner) || actor.Guard == nil {
		writeClaimFailure(w, http.StatusUnauthorized, "owner_required", "Sign in as this server’s current owner.", false)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var result ClaimStatus
	e = h.runner.Do(ctx, func(ctx context.Context) error {
		ctx, e = guardedClaimRequest(ctx, h.store, actor.Guard)
		if e != nil {
			return e
		}
		if e = checkClaimRequest(ctx); e != nil {
			return e
		}
		result, e = h.handle(ctx, w, r, actor.Owner)
		if e != nil {
			return e
		}
		return checkClaimRequest(ctx)
	})
	if e != nil {
		h.failure(w, r, e)
		return
	}
	if h.certificates != nil && r.Method == http.MethodPost {
		h.certificates.Wake()
		if h.remote != nil {
			h.remote.Invalidate()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
func readClaimBody(w http.ResponseWriter, r *http.Request, out any) error {
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || media != "application/json" {
		return ErrInvalid
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 20<<10))
	if e != nil {
		return ErrInvalid
	}
	defer clear(raw)
	// Canonical wrapper rejects duplicate fields and numeric aliases. The signed
	// envelope string remains byte-for-byte unchanged after JSON string decoding.
	return canonicalApprovalJSON(raw, out)
}
func (h *ClaimHandler) handle(ctx context.Context, w http.ResponseWriter, r *http.Request, owner LocalOwner) (ClaimStatus, error) {
	if r.Method == http.MethodGet && r.URL.Path == "/v1/networking/claim" {
		return h.snapshot(ctx)
	}
	if r.Method != http.MethodPost {
		return ClaimStatus{}, ErrInvalid
	}
	if r.URL.Path == "/v1/networking/claim/prepare" {
		// accountId is optional (omitempty): the canonical decoder compares the
		// request with its own re-encoding, so a body without one, which is what
		// the web console's Plex-style Connect sends, must re-encode without one.
		var body struct {
			AccountID string                `json:"accountId,omitempty"`
			Expected  ClaimIdentityExpected `json:"expected"`
		}
		if e := readClaimBody(w, r, &body); e != nil {
			return ClaimStatus{}, e
		}
		// Web approval (Plex-style): no account is needed to prepare. The person
		// who approves the code on web.getportico.tv becomes the owner.
		if body.AccountID == "" {
			body.AccountID = UnboundAccount
		}
		if !validID(body.AccountID) || !validID(body.Expected.ServerID) || body.Expected.LocalGeneration < 0 {
			return ClaimStatus{}, ErrInvalid
		}
		gated, e := h.store.tx(ctx)
		if e != nil {
			return ClaimStatus{}, e
		}
		tx := gated.Tx()
		identity, active, installed, e := readCurrentIdentityTx(ctx, tx)
		gated.Rollback()
		if e != nil {
			return ClaimStatus{}, e
		}
		if active.Valid || installed.Valid || identity.ServerID != body.Expected.ServerID || identity.ResetGeneration != body.Expected.LocalGeneration {
			return ClaimStatus{}, ErrStale
		}
		nonce := make([]byte, 24)
		if _, e = rand.Read(nonce); e != nil {
			return ClaimStatus{}, e
		}
		binding := Binding{OperationID: base64.RawURLEncoding.EncodeToString(nonce), ServerID: identity.ServerID, AccountID: body.AccountID, PublicKey: identity.PublicKey, LocalGeneration: identity.ResetGeneration}
		if _, e = h.store.Prepare(ctx, binding, owner); e != nil {
			return ClaimStatus{}, e
		}
		return h.snapshot(ctx)
	}
	var body struct {
		Expected         ClaimExpected `json:"expected"`
		ApprovalEnvelope string        `json:"approvalEnvelope,omitempty"`
	}
	if e := readClaimBody(w, r, &body); e != nil {
		return ClaimStatus{}, e
	}
	if !validID(body.Expected.OperationID) || body.Expected.LocalGeneration < 0 || body.Expected.Revision < 1 {
		return ClaimStatus{}, ErrInvalid
	}
	expected, e := h.expected(ctx, body.Expected)
	if e != nil {
		return ClaimStatus{}, e
	}
	if r.URL.Path == "/v1/networking/claim/web-approval" {
		address := returnOrigin(r)
		pending, e := h.beginWebClaim(ctx, expected, address)
		if e != nil {
			return ClaimStatus{}, e
		}
		status, e := h.snapshot(ctx)
		status.ApprovalURL = pending.URL
		status.ClaimCode = pending.Code
		return status, e
	}
	if r.URL.Path == "/v1/networking/claim/approve" {
		if len(body.ApprovalEnvelope) < 1 || len(body.ApprovalEnvelope) > 16<<10 {
			return ClaimStatus{}, ErrInvalid
		}
		if _, e = h.store.Approve(ctx, expected, []byte(body.ApprovalEnvelope)); e != nil {
			return ClaimStatus{}, e
		}
		return h.snapshot(ctx)
	}
	if body.ApprovalEnvelope != "" {
		return ClaimStatus{}, ErrInvalid
	}
	if r.URL.Path != "/v1/networking/claim/continue" && r.URL.Path != "/v1/networking/claim/cancel" {
		return ClaimStatus{}, ErrInvalid
	}
	coordinator, e := NewCoordinator(expected.OperationID, h.store.audience, claimExpectedStore{Store: h.store, expected: expected}, h.transport, h.signer)
	if e != nil {
		return ClaimStatus{}, e
	}
	if r.URL.Path == "/v1/networking/claim/cancel" {
		_, e = coordinator.Cancel(ctx)
	} else {
		_, e = coordinator.Step(ctx)
	}
	if e != nil {
		return ClaimStatus{}, e
	}
	return h.snapshot(ctx)
}
func (h *ClaimHandler) expected(ctx context.Context, want ClaimExpected) (Intent, error) {
	gated2, e := h.store.tx(ctx)
	if e != nil {
		return Intent{}, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	_, active, _, e := readCurrentIdentityTx(ctx, tx)
	if e != nil {
		return Intent{}, e
	}
	if !active.Valid || active.String != want.OperationID {
		return Intent{}, ErrStale
	}
	v, e := loadIntentTx(ctx, tx, want.OperationID)
	if e != nil {
		return Intent{}, e
	}
	if v.LocalGeneration != want.LocalGeneration || v.Revision != want.Revision {
		return Intent{}, ErrStale
	}
	return v.Intent, nil
}
func (h *ClaimHandler) snapshot(ctx context.Context) (ClaimStatus, error) {
	// Name is display metadata and may read the same single-connection database.
	// Resolve it before borrowing the transactional claim connection.
	name := h.name()
	gated3, e := h.store.tx(ctx)
	if e != nil {
		return ClaimStatus{}, e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	id, active, _, e := readCurrentIdentityTx(ctx, tx)
	if e != nil {
		return ClaimStatus{}, e
	}
	result := ClaimStatus{State: "unconfigured", Identity: ClaimIdentityExpected{id.ServerID, id.ResetGeneration}, Actions: []string{"prepare"}}
	if !active.Valid {
		return result, nil
	}
	stored, e := loadIntentTx(ctx, tx, active.String)
	if e != nil {
		return ClaimStatus{}, e
	}
	v := stored.Intent
	result.State = string(v.Stage)
	result.Operation = &ClaimExpected{v.OperationID, v.LocalGeneration, v.Revision}
	result.AccountID = v.AccountID
	result.InstallationAcknowledged = v.InstallationAcknowledged
	result.Actions = []string{}
	if v.Stage == CancelPending {
		result.Actions = []string{"continue"}
		return result, nil
	}
	result.Actions = append(result.Actions, "cancel")
	if v.Stage == Installed {
		var state string
		e = tx.QueryRowContext(ctx, `SELECT state FROM networking_claim_authority WHERE operation_id=?`, v.OperationID).Scan(&state)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return ClaimStatus{}, e
		}
		if terminalClaimState(state) {
			result.State = "disconnected"
			return result, nil
		}
		if !v.InstallationAcknowledged {
			result.Actions = append(result.Actions, "continue")
		}
		return result, nil
	}
	result.ApprovalRequired = v.Stage == Prepared || !h.store.now().Before(v.ApprovalExpiresAt)
	if result.ApprovalRequired {
		if name == "" || len(name) > 120 {
			return ClaimStatus{}, ErrInvalid
		}
		result.Actions = append(result.Actions, "approve")
		result.ApprovalRequest = &ClaimApprovalRequest{v.OperationID, v.ServerID, base64.RawURLEncoding.EncodeToString(v.PublicKey), v.LocalGeneration, name, v.ApprovalRevision}
	} else {
		result.Actions = append(result.Actions, "continue")
	}
	return result, nil
}

type claimExpectedStore struct {
	Store
	expected Intent
}

func (s claimExpectedStore) Load(ctx context.Context, id string) (Intent, error) {
	v, e := s.Store.Load(ctx, id)
	if e != nil {
		return Intent{}, e
	}
	if !sameBinding(v.Binding, s.expected.Binding) || v.Revision != s.expected.Revision || v.Stage != s.expected.Stage || v.ApprovalRevision != s.expected.ApprovalRevision || v.ClaimGeneration != s.expected.ClaimGeneration || v.CredentialGeneration != s.expected.CredentialGeneration {
		return Intent{}, ErrStale
	}
	return v, nil
}
func writeClaimFailure(w http.ResponseWriter, status int, code, message string, retry bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": message, "retryable": retry}})
}
func (h *ClaimHandler) failure(w http.ResponseWriter, r *http.Request, e error) {
	switch {
	case errors.Is(e, ErrClaimOwnerRequired):
		writeClaimFailure(w, 401, "owner_required", "Sign in as this server’s current owner.", false)
	case errors.Is(e, ErrInvalid):
		writeClaimFailure(w, 400, "invalid_claim_request", "Check the claim request and try again.", false)
	case errors.Is(e, ErrStale):
		writeClaimFailure(w, 409, "claim_stale", "Server setup changed. Refresh its status before continuing.", false)
	case errors.Is(e, ErrApprovalRequired):
		writeClaimFailure(w, 409, "approval_required", "Approve this server with your Portico Account to continue.", false)
	case errors.Is(e, ErrCancelled):
		writeClaimFailure(w, 409, "claim_cancelled", "This server connection was cancelled or revoked.", false)
	default:
		var request context.Context
		if r != nil {
			request = r.Context()
		}
		h.failures.report(routeLabel(r, "networking claim route"), 503, request, e)
		writeClaimFailure(w, 503, "claim_unavailable", "Server setup could not finish. Refresh its status and retry when the connection is available.", true)
	}
}

// SetManagementAuthorizer wires ordinary server-owner administration without
// changing the separate local-owner bootstrap/claim authority. Call before Register.
func (h *ClaimHandler) SetManagementAuthorizer(authorize func(*http.Request) (ClaimRequestGuard, error)) {
	h.managementAuthorize = authorize
}
func (h *ClaimHandler) authorizeManagement(r *http.Request) (ClaimRequestGuard, error) {
	if h.managementAuthorize != nil {
		return h.managementAuthorize(r)
	}
	actor, err := h.authorize(r)
	if err != nil {
		return nil, err
	}
	if !validOwner(actor.Owner) || actor.Guard == nil {
		return nil, ErrClaimOwnerRequired
	}
	return actor.Guard, nil
}

// returnOrigin is where the web approval page sends the person back: the
// origin their browser used for this request (A60). A same-origin Origin header
// is exact, including the scheme behind a TLS-terminating proxy; otherwise the
// request's own scheme and host.
func returnOrigin(r *http.Request) string {
	if origin := r.Header.Get("Origin"); origin != "" {
		if u, e := url.Parse(origin); e == nil && (u.Scheme == "http" || u.Scheme == "https") && strings.EqualFold(u.Host, r.Host) && u.Path == "" && u.User == nil {
			return u.Scheme + "://" + u.Host
		}
	}
	if r.TLS != nil {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}
