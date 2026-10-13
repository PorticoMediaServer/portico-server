package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/metadata"
)

type attentionReceipt struct {
	Status string `json:"status"`
}

func metadataAttentionError(err error) error {
	if errors.Is(err, metadata.ErrScreenConflict) {
		return &apierror.Error{Code: "revision_mismatch", Cause: err}
	}
	if errors.Is(err, metadata.ErrRepairInput) || errors.Is(err, metadata.ErrLocalMetadataOnly) {
		return &apierror.Error{Code: "invalid_request", Cause: err}
	}
	return bulkError(err)
}
func registerMetadataAttention(registry *apikit.Registry, d Dependencies) {
	failures := []string{"unauthorized", "not_permitted", "not_found", "invalid_request", "revision_mismatch", "rate_limited", "internal"}
	rate := &personalLimiter{}
	v1Route(registry, apikit.Metadata{ID: "get_metadata_attention", Method: "GET", Path: "/v1/libraries/{id}/metadata/attention", Summary: "List unmatched and uncertain titles with their best candidates", Access: apikit.Owner, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200, Query: []string{"cursor", "limit"}, Errors: failures}, func(ctx context.Context, r *http.Request, _ noBody) (metadata.AttentionPage, error) {
		if _, e := d.owner(r); e != nil {
			return metadata.AttentionPage{}, e
		}
		limit := 25
		if value := r.URL.Query().Get("limit"); value != "" {
			n, e := strconv.Atoi(value)
			if e != nil || n < 1 || n > 50 {
				return metadata.AttentionPage{}, &apierror.Error{Code: "invalid_request"}
			}
			limit = n
		}
		out, e := d.Metadata.Attention(ctx, r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
		if e != nil {
			return out, metadataAttentionError(e)
		}
		_, e = d.owner(r)
		return out, e
	})
	v1Route(registry, apikit.Metadata{ID: "post_metadata_attention", Method: "POST", Path: "/v1/libraries/{id}/metadata/attention", Summary: "Accept a candidate, search, enter a provider ID or dismiss a title", Access: apikit.Owner, Lane: apikit.Browsing, Cost: apikit.Constant, Status: 202, BodyLimit: 4096, Errors: failures}, func(ctx context.Context, r *http.Request, in metadata.AttentionCommand) (attentionReceipt, error) {
		p, e := d.owner(r)
		if e != nil {
			return attentionReceipt{}, e
		}
		if !rate.allow(viewerScope(p)) {
			return attentionReceipt{}, &apierror.Error{Code: "rate_limited"}
		}
		e = d.Metadata.AttentionAction(ctx, r.PathValue("id"), in, metadata.MBActor{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID}, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		return attentionReceipt{Status: "accepted"}, metadataAttentionError(e)
	})
}
