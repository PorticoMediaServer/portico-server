package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/driveoauth"
	"portico.local/server/internal/identity"
)

// DriveOAuthStartInput is the owner's own Google OAuth client. The secret is
// used only by the server's exchange and never returned.
type DriveOAuthStartInput struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

type DriveOAuthStartResponse struct {
	Scope            catalog.AdminScope `json:"scope"`
	AuthorizationURL string             `json:"authorizationUrl"`
	State            string             `json:"state"`
}

type DriveOAuthStateInput struct {
	State string `json:"state"`
}

type DriveOAuthPasteInput struct {
	State       string `json:"state"`
	RedirectURL string `json:"redirectUrl"`
}

type DriveOAuthStatusResponse struct {
	Scope catalog.AdminScope `json:"scope"`
	Phase string             `json:"phase"`
	Token string             `json:"token,omitempty"`
}

type DriveOAuthCancelResponse struct {
	Scope     catalog.AdminScope `json:"scope"`
	Cancelled bool               `json:"cancelled"`
}

func driveOAuthActor(p identity.Principal) string {
	actor, _ := json.Marshal([]string{p.Authority, p.AccountID, p.ProfileID})
	return string(actor)
}

func registerDriveOAuth(registry *apikit.Registry, d Dependencies) {
	v1Route(registry, apikit.Metadata{ID: "post_storage_source_drive_oauth_start", Method: "POST", Path: "/v1/storage/sources/rclone/drive/oauth/start", Summary: "Start the owner's own Google OAuth sign-in for a Drive source", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminLong, Cost: apikit.Constant, BodyLimit: 8192, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "administration_unavailable", "internal"}}, func(_ context.Context, r *http.Request, in DriveOAuthStartInput) (DriveOAuthStartResponse, error) {
		p, scope, err := d.storageOwner(r)
		if err != nil {
			return DriveOAuthStartResponse{}, rcloneConnectionFailure(err)
		}
		if d.DriveOAuth == nil {
			return DriveOAuthStartResponse{}, &apierror.Error{Code: "administration_unavailable"}
		}
		if r.URL.RawQuery != "" {
			return DriveOAuthStartResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		authorizationURL, state, err := d.DriveOAuth.Start(driveOAuthActor(p), in.ClientID, in.ClientSecret)
		if err != nil {
			return DriveOAuthStartResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		if _, _, err = d.storageOwner(r); err != nil {
			d.DriveOAuth.Cancel(driveOAuthActor(p), state)
			return DriveOAuthStartResponse{}, rcloneConnectionFailure(err)
		}
		return DriveOAuthStartResponse{Scope: scope, AuthorizationURL: authorizationURL, State: state}, nil
	})
	v1Route(registry, apikit.Metadata{ID: "post_storage_source_drive_oauth_paste", Method: "POST", Path: "/v1/storage/sources/rclone/drive/oauth/paste", Summary: "Take the code from the address Google ended on", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminLong, Cost: apikit.Constant, BodyLimit: 8192, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "administration_unavailable", "internal"}}, func(_ context.Context, r *http.Request, in DriveOAuthPasteInput) (DriveOAuthCancelResponse, error) {
		p, scope, err := d.storageOwner(r)
		if err != nil {
			return DriveOAuthCancelResponse{}, rcloneConnectionFailure(err)
		}
		if d.DriveOAuth == nil {
			return DriveOAuthCancelResponse{}, &apierror.Error{Code: "administration_unavailable"}
		}
		if r.URL.RawQuery != "" {
			return DriveOAuthCancelResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		if err = d.DriveOAuth.Paste(driveOAuthActor(p), in.State, in.RedirectURL); err != nil {
			return DriveOAuthCancelResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		if _, _, err = d.storageOwner(r); err != nil {
			return DriveOAuthCancelResponse{}, rcloneConnectionFailure(err)
		}
		return DriveOAuthCancelResponse{Scope: scope, Cancelled: false}, nil
	})
	v1Route(registry, apikit.Metadata{ID: "post_storage_source_drive_oauth_status", Method: "POST", Path: "/v1/storage/sources/rclone/drive/oauth/status", Summary: "Read the owner's Google sign-in and exchange its one code", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminLong, Cost: apikit.Constant, BodyLimit: 8192, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "source_credentials_required", "administration_unavailable", "internal"}}, func(ctx context.Context, r *http.Request, in DriveOAuthStateInput) (DriveOAuthStatusResponse, error) {
		p, scope, err := d.storageOwner(r)
		if err != nil {
			return DriveOAuthStatusResponse{}, rcloneConnectionFailure(err)
		}
		if d.DriveOAuth == nil {
			return DriveOAuthStatusResponse{}, &apierror.Error{Code: "administration_unavailable"}
		}
		if r.URL.RawQuery != "" {
			return DriveOAuthStatusResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		result, err := d.DriveOAuth.Status(ctx, driveOAuthActor(p), in.State)
		if err != nil {
			if errors.Is(err, driveoauth.ErrExchange) {
				return DriveOAuthStatusResponse{}, &apierror.Error{Code: "source_credentials_required"}
			}
			return DriveOAuthStatusResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		if _, _, err = d.storageOwner(r); err != nil {
			return DriveOAuthStatusResponse{}, rcloneConnectionFailure(err)
		}
		return DriveOAuthStatusResponse{Scope: scope, Phase: string(result.Phase), Token: result.Token}, nil
	})
	v1Route(registry, apikit.Metadata{ID: "post_storage_source_drive_oauth_cancel", Method: "POST", Path: "/v1/storage/sources/rclone/drive/oauth/cancel", Summary: "End the owner's Google sign-in session", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminLong, Cost: apikit.Constant, BodyLimit: 8192, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "administration_unavailable", "internal"}}, func(_ context.Context, r *http.Request, in DriveOAuthStateInput) (DriveOAuthCancelResponse, error) {
		p, scope, err := d.storageOwner(r)
		if err != nil {
			return DriveOAuthCancelResponse{}, rcloneConnectionFailure(err)
		}
		if d.DriveOAuth == nil {
			return DriveOAuthCancelResponse{}, &apierror.Error{Code: "administration_unavailable"}
		}
		if r.URL.RawQuery != "" {
			return DriveOAuthCancelResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		d.DriveOAuth.Cancel(driveOAuthActor(p), in.State)
		return DriveOAuthCancelResponse{Scope: scope, Cancelled: true}, nil
	})
}
