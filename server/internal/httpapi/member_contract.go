package httpapi

import (
	"context"
	"errors"
	"net/http"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/identity"
)

// Deleting a member. The owner (or an administrator, for a plain member) first
// reads what will be removed, then confirms with the revision that preview
// showed. Turning a member off stays on PATCH /v1/direct/members/{id}; this is
// the one that removes the person and their data.
func registerMemberRoutes(registry *apikit.Registry, d Dependencies) error {
	if err := apikit.Register(registry, apikit.Route[struct{}, identity.MemberDeletionPreview]{Metadata: apikit.Metadata{Admission: laneAuth, ID: "get_direct_member_deletion_preview", Method: "GET", Path: "/v1/direct/members/{id}/deletion-preview", Summary: "Read what deleting a member will remove", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Cost: apikit.Constant, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "not_found", "internal"}}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (identity.MemberDeletionPreview, error) {
		out, err := d.Identity.PreviewDirectMemberDeletion(ctx, directBearer(r), r.PathValue("id"))
		return out, foundationAuthErrorOrNil(err)
	}}); err != nil {
		return err
	}
	// Every member's devices, for the owner and administrators: the same device
	// records sessions are bound to, with their approval. An administrator
	// who is not the owner sees and decides about plain members' devices.
	if err := apikit.Register(registry, apikit.Route[struct{}, identity.MemberDevicePage]{Metadata: apikit.Metadata{Admission: laneAdmin, ID: "get_admin_access_devices", Method: "GET", Path: "/v1/admin/access/devices", Summary: "List members' devices and whether each is approved", Access: apikit.Admin, Requires: apikit.ScopeAdmin, AdminScope: "members", Lane: apikit.Security, Cost: apikit.PageSized, Query: []string{"cursor", "limit", "trust", "account"}, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "internal"}}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (identity.MemberDevicePage, error) {
		p, err := d.foundationCaller(r.WithContext(ctx))
		if err != nil {
			return identity.MemberDevicePage{}, foundationAuthError(err)
		}
		query := r.URL.Query()
		out, err := d.Identity.MemberDevices(ctx, p, identity.MemberDeviceQuery{Cursor: query.Get("cursor"), Limit: query.Get("limit"), Trust: query.Get("trust"), Account: query.Get("account")})
		return out, memberDeviceError(err)
	}}); err != nil {
		return err
	}
	if err := apikit.Register(registry, apikit.Route[identity.MemberDeviceTrust, identity.MemberDevice]{Metadata: apikit.Metadata{Admission: laneSecurityFence, ID: "put_admin_access_device_trust", Method: "PUT", Path: "/v1/admin/access/devices/{id}/trust", Summary: "Approve a member's device, block it, or put it back to waiting", Access: apikit.Admin, Requires: apikit.ScopeSession, AdminScope: "members", Lane: apikit.Security, Cost: apikit.Constant, BodyLimit: 4096, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "not_found", "revision_mismatch", "internal"}}, Handler: func(ctx context.Context, r *http.Request, body identity.MemberDeviceTrust) (identity.MemberDevice, error) {
		p, err := d.foundationCaller(r.WithContext(ctx))
		if err != nil {
			return identity.MemberDevice{}, foundationAuthError(err)
		}
		out, err := d.Identity.SetMemberDeviceTrust(ctx, p, r.PathValue("id"), body)
		return out, memberDeviceError(err)
	}}); err != nil {
		return err
	}
	return apikit.Register(registry, apikit.Route[identity.MemberDeletion, struct{}]{Metadata: apikit.Metadata{Admission: laneSecurityFence, ID: "delete_direct_member", Method: "DELETE", Path: "/v1/direct/members/{id}", Summary: "Delete a member, their profiles, devices and saved data", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Cost: apikit.Constant, BodyLimit: 4096, Status: 204, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "not_found", "revision_mismatch", "internal"}}, Handler: func(ctx context.Context, r *http.Request, body identity.MemberDeletion) (struct{}, error) {
		err := d.Identity.DeleteDirectMember(ctx, directBearer(r), r.PathValue("id"), body)
		return struct{}{}, foundationAuthErrorOrNil(err)
	}})
}

// RememberedAccountsRequest names the installation whose remembered accounts
// are wanted or changed. The installation id travels in the body: it is what a
// device renews its sign-in with, and an address ends up in proxy and access
// logs.
type RememberedAccountsRequest struct {
	InstallationID string `json:"installationId"`
}

type RememberedAccountsDocument struct {
	Items []identity.BrowserAccount `json:"items"`
}

// registerRememberedAccountRoutes: the account switcher a browser shows before
// anyone has signed in, and forgetting one entry of it.
func registerRememberedAccountRoutes(registry *apikit.Registry, d Dependencies) error {
	if err := apikit.Register(registry, apikit.Route[RememberedAccountsRequest, RememberedAccountsDocument]{Metadata: apikit.Metadata{Admission: laneAuth, // Unauthenticated by design: a browser that has not signed in is exactly
		// the caller that needs the account switcher, and no entry is a credential.
		ID: "post_auth_remembered_accounts", Method: "POST", Path: "/v1/auth/remembered-accounts", Summary: "List the accounts remembered on this installation", Access: apikit.Public, Requires: apikit.ScopeSession, Lane: apikit.Security, Cost: apikit.Constant, BodyLimit: 4096, Status: 200, Errors: []string{"invalid_request", "internal"},
	}, Handler: func(ctx context.Context, _ *http.Request, body RememberedAccountsRequest) (RememberedAccountsDocument, error) {
		items, err := d.Identity.BrowserAccounts(ctx, body.InstallationID)
		if err != nil {
			return RememberedAccountsDocument{}, rememberedAccountError(err)
		}
		return RememberedAccountsDocument{Items: items}, nil
	}}); err != nil {
		return err
	}
	return apikit.Register(registry, apikit.Route[RememberedAccountsRequest, struct{}]{Metadata: apikit.Metadata{Admission: laneAuth, ID: "delete_direct_remembered_account", Method: "DELETE", Path: "/v1/direct/remembered-accounts/{accountId}", Summary: "Forget one account remembered on this installation", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Cost: apikit.Constant, BodyLimit: 4096, Status: 204, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "internal"}}, Handler: func(ctx context.Context, r *http.Request, body RememberedAccountsRequest) (struct{}, error) {
		err := d.Identity.ForgetBrowserAccount(ctx, directBearer(r), body.InstallationID, r.PathValue("accountId"))
		return struct{}{}, rememberedAccountError(err)
	}})
}

func rememberedAccountError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, identity.ErrBrowserAccountInput) {
		return &apierror.Error{Code: "invalid_request"}
	}
	return foundationAuthError(err)
}

func memberDeviceError(err error) error {
	if err == nil {
		return nil
	}
	var changed *identity.DeviceChangedError
	if errors.As(err, &changed) {
		return &apierror.Error{Code: "revision_mismatch", Cause: err}
	}
	return foundationAuthError(err)
}
