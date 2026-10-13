package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadata"
)

type attentionReceipt struct {
	Status string `json:"status"`
}

func metadataAttentionError(err error) error {
	if errors.Is(err, metadata.ErrScreenConflict) {
		return &apierror.Error{Code: "revision_mismatch", Cause: err}
	}
	if errors.Is(err, metadata.ErrRepairInput) || errors.Is(err, metadata.ErrLocalMetadataOnly) || errors.Is(err, metadata.ErrRefreshKind) {
		return &apierror.Error{Code: "invalid_request", Cause: err}
	}
	return bulkError(err)
}

// errMetadataUnavailable is what these routes answer on a server running
// without the metadata service.
func errMetadataUnavailable() error { return &apierror.Error{Code: "metadata_unavailable"} }

func registerMetadataAttention(registry *apikit.Registry, d Dependencies) {
	failures := []string{"unauthorized", "not_permitted", "not_found", "invalid_request", "revision_mismatch", "rate_limited", "internal"}
	rate := &personalLimiter{}
	v1Route(registry, apikit.Metadata{ID: "get_metadata_attention", Method: "GET", Path: "/v1/libraries/{id}/metadata/attention", Summary: "List unmatched and uncertain titles with their best candidates", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200, Query: []string{"cursor", "limit"}, Errors: failures}, func(ctx context.Context, r *http.Request, _ noBody) (metadata.AttentionPage, error) {
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
		if d.Metadata == nil {
			return metadata.AttentionPage{}, errMetadataUnavailable()
		}
		out, e := d.Metadata.Attention(ctx, r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
		if e != nil {
			return out, metadataAttentionError(e)
		}
		_, e = d.owner(r)
		return out, e
	})
	v1Route(registry, apikit.Metadata{ID: "post_metadata_attention", Method: "POST", Path: "/v1/libraries/{id}/metadata/attention", Summary: "Accept a candidate, search, enter a provider ID or dismiss a title", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Cost: apikit.Constant, Status: 202, BodyLimit: 4096, Errors: failures}, func(ctx context.Context, r *http.Request, in metadata.AttentionCommand) (attentionReceipt, error) {
		p, e := d.owner(r)
		if e != nil {
			return attentionReceipt{}, e
		}
		if !rate.allow(viewerScope(p)) {
			return attentionReceipt{}, &apierror.Error{Code: "rate_limited"}
		}
		if d.Metadata == nil {
			return attentionReceipt{}, errMetadataUnavailable()
		}
		e = d.Metadata.AttentionAction(ctx, r.PathValue("id"), in, metadata.MBActor{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID}, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		return attentionReceipt{Status: "accepted"}, metadataAttentionError(e)
	})
}

func registerMetadataSettings(registry *apikit.Registry, d Dependencies) {
	failures := []string{"unauthorized", "not_permitted", "invalid_request", "revision_mismatch", "internal"}
	v1Route(registry, apikit.Metadata{ID: "get_metadata_settings", Method: "GET", Path: "/v1/admin/metadata/settings", Summary: "Read provider credentials status and artwork storage limit", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Cost: apikit.Constant, Status: 200, Errors: failures}, func(ctx context.Context, r *http.Request, _ noBody) (metadata.MetadataSettings, error) {
		if _, e := d.owner(r); e != nil {
			return metadata.MetadataSettings{}, e
		}
		if d.Metadata == nil {
			return metadata.MetadataSettings{}, errMetadataUnavailable()
		}
		out, e := d.Metadata.MetadataSettings(ctx)
		if e != nil {
			return out, e
		}
		_, e = d.owner(r)
		return out, e
	})
	v1Route(registry, apikit.Metadata{ID: "put_metadata_settings", Method: "PUT", Path: "/v1/admin/metadata/settings", Summary: "Save provider credential overrides and artwork storage limit", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Cost: apikit.Constant, Status: 200, BodyLimit: 32768, Errors: failures}, func(ctx context.Context, r *http.Request, in metadata.MetadataSettingsUpdate) (metadata.MetadataSettings, error) {
		p, e := d.owner(r)
		if e != nil {
			return metadata.MetadataSettings{}, e
		}
		if d.Metadata == nil {
			return metadata.MetadataSettings{}, errMetadataUnavailable()
		}
		out, e := d.Metadata.UpdateMetadataSettings(ctx, in, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		return out, metadataAttentionError(e)
	})
}

type refreshReceipt struct {
	ItemID string `json:"itemId"`
	Status string `json:"status"`
}

func registerMetadataRefresh(registry *apikit.Registry, d Dependencies) {
	for _, kind := range []string{"items", "shows"} {
		v1Route(registry, apikit.Metadata{ID: "refresh_" + kind + "_metadata", Method: "POST", Path: "/v1/" + kind + "/{id}/metadata/refresh", Summary: "Fetch current metadata for a title", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Cost: apikit.Constant, Status: 202, OptionalBody: true, BodyLimit: 4096, Errors: []string{"unauthorized", "not_permitted", "not_found", "invalid_request", "internal"}}, func(ctx context.Context, r *http.Request, _ noBody) (refreshReceipt, error) {
			p, e := d.owner(r)
			if e != nil {
				return refreshReceipt{}, e
			}
			write, e := dbwork.Begin(ctx, d.DB, dbwork.ClassInteractive)
			if e != nil {
				return refreshReceipt{}, e
			}
			defer write.Rollback()
			if e = d.resourceAuthorize(write.Tx(), p, ""); e != nil {
				return refreshReceipt{}, e
			}
			if d.Metadata == nil {
				return refreshReceipt{}, errMetadataUnavailable()
			}
			e = d.Metadata.QueueRefreshTx(ctx, write.Tx(), r.PathValue("id"))
			if e != nil {
				return refreshReceipt{}, metadataAttentionError(e)
			}
			return refreshReceipt{ItemID: r.PathValue("id"), Status: "queued"}, write.Commit()
		})
	}
}

func registerScreenMetadataReads(registry *apikit.Registry, d Dependencies) {
	for _, target := range []struct{ route, kind string }{{"items", "item"}, {"shows", "show"}} {
		kind := target.kind
		v1Route(registry, apikit.Metadata{ID: "get_" + target.route + "_screen_metadata", Method: "GET", Path: "/v1/" + target.route + "/{id}/metadata/screen", Summary: "Read match evidence, field provenance and provider candidates", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200, Errors: []string{"unauthorized", "not_permitted", "not_found", "internal"}}, func(ctx context.Context, r *http.Request, _ noBody) (metadata.ScreenState, error) {
			if _, e := d.owner(r); e != nil {
				return metadata.ScreenState{}, e
			}
			if d.Metadata == nil {
				return metadata.ScreenState{}, errMetadataUnavailable()
			}
			out, e := d.Metadata.ScreenState(ctx, kind, r.PathValue("id"))
			if e != nil {
				return out, metadataAttentionError(e)
			}
			if _, e = d.owner(r); e != nil {
				return out, e
			}
			_, _, fence, e := d.homeScope(r)
			if e != nil {
				return out, e
			}
			out.ServerID = d.Identity.ID()
			out.ViewerFence = fence
			return out, nil
		})
	}
}
