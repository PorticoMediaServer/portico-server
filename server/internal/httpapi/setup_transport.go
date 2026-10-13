package httpapi

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/identity"
)

// Native clients may omit Origin. Browser writes require the request's exact
// origin, including for callers otherwise permitted by cross-origin API CORS.
func (d Dependencies) setupRequestAllowed(r *http.Request) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || d.sameRequestOrigin(r, origin)
}
func (d Dependencies) serveSetup(w http.ResponseWriter, r *http.Request) {
	if !d.setupRequestAllowed(r) {
		failure(w, identity.ErrUnauthorized)
		return
	}
	var body identity.SetupOptions
	if e := decode(w, r, &body); e != nil {
		failure(w, e)
		return
	}
	if e := d.Identity.SetupLimit(r.Context(), d.setupPeerSubject(r), "setup", 10); e != nil {
		setupFailure(w, e)
		return
	}
	result, e := d.Identity.SetupWithOptions(r.Context(), body, d.privateSetupPeer(r))
	if e != nil {
		failure(w, e)
		return
	}
	writeAuth(w, r, 201, result)
}

type BrowserSetupResponse struct {
	ServerID   string `json:"serverId"`
	SetupToken string `json:"setupToken"`
}

func registerSetupTransport(registry *apikit.Registry, d Dependencies) error {
	err := apikit.RegisterRaw(registry, apikit.RawRoute[identity.SetupOptions, identity.Envelope]{Metadata: apikit.Metadata{
		ID: "setup_server", Method: "POST", Path: "/v1/setup", Summary: "Create the first owner", Access: apikit.Public, Lane: apikit.Security, Cost: apikit.Constant, Status: 201, BodyLimit: 65536, Errors: []string{"unauthorized", "invalid_request", "rate_limited", "setup_already_complete", "internal"},
	}, Serve: d.serveSetup})
	if err != nil {
		return err
	}
	return apikit.Register(registry, apikit.Route[struct{}, BrowserSetupResponse]{Metadata: apikit.Metadata{
		ID: "start_browser_setup", Method: "POST", Path: "/v1/setup/browser", Summary: "Start first-run setup from this origin", Access: apikit.Public, Lane: apikit.Security, Cost: apikit.Constant, Status: 200, BodyLimit: 4096, Errors: []string{"unauthorized", "invalid_request", "rate_limited", "setup_already_complete", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (BrowserSetupResponse, error) {
		if !d.setupRequestAllowed(r) || !d.sameRequestOrigin(r, r.Header.Get("Origin")) {
			return BrowserSetupResponse{}, foundationAuthError(identity.ErrUnauthorized)
		}
		if err := d.Identity.SetupLimit(ctx, d.setupPeerSubject(r), "browser-setup", 10); err != nil {
			return BrowserSetupResponse{}, setupTransportError(err)
		}
		token, err := d.Identity.BrowserSetupToken(ctx)
		if err != nil {
			return BrowserSetupResponse{}, setupTransportError(err)
		}
		return BrowserSetupResponse{d.Identity.ID(), token}, nil
	}})
}

func setupTransportError(err error) error {
	if errors.Is(err, identity.ErrConflict) {
		return &apierror.Error{Code: "setup_already_complete"}
	}
	var protocol *identity.SetupProtocolError
	if errors.As(err, &protocol) && protocol.Code == "rate_limited" {
		return &apierror.Error{Code: "rate_limited", RetryAfterSeconds: protocol.Interval}
	}
	return foundationAuthError(err)
}
func connectionErrorDefinitions() []apierror.Definition {
	return []apierror.Definition{{Code: "setup_already_complete", Status: 409, Message: "This server is already set up.", Retry: apierror.Never}}
}
