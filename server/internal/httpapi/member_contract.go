package httpapi

import (
	"context"
	"net/http"

	"portico.local/apikit"
	"portico.local/server/internal/identity"
)

// Deleting a member. The owner (or an administrator, for a plain member) first
// reads what will be removed, then confirms with the revision that preview
// showed. Turning a member off stays on PATCH /v1/direct/members/{id}; this is
// the one that removes the person and their data.
func registerMemberRoutes(registry *apikit.Registry, d Dependencies) error {
	if err := apikit.Register(registry, apikit.Route[struct{}, identity.MemberDeletionPreview]{Metadata: apikit.Metadata{
		ID: "get_direct_member_deletion_preview", Method: "GET", Path: "/v1/direct/members/{id}/deletion-preview", Summary: "Read what deleting a member will remove", Access: apikit.Device, Lane: apikit.Security, Cost: apikit.Constant, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "not_found", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (identity.MemberDeletionPreview, error) {
		out, err := d.Identity.PreviewDirectMemberDeletion(ctx, directBearer(r), r.PathValue("id"))
		return out, foundationAuthErrorOrNil(err)
	}}); err != nil {
		return err
	}
	return apikit.Register(registry, apikit.Route[identity.MemberDeletion, struct{}]{Metadata: apikit.Metadata{
		ID: "delete_direct_member", Method: "DELETE", Path: "/v1/direct/members/{id}", Summary: "Delete a member, their profiles, devices and saved data", Access: apikit.Device, Lane: apikit.Security, Cost: apikit.Constant, BodyLimit: 4096, Status: 204, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "not_found", "revision_mismatch", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, body identity.MemberDeletion) (struct{}, error) {
		err := d.Identity.DeleteDirectMember(ctx, directBearer(r), r.PathValue("id"), body)
		return struct{}{}, foundationAuthErrorOrNil(err)
	}})
}
