package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/livechannels"
)

func directoryQuery(q url.Values) (livechannels.DirectoryQuery, error) {
	out := livechannels.DirectoryQuery{Kind: "all", Sort: "number", Limit: 50}
	for key, values := range q {
		if len(values) != 1 {
			return out, livechannels.ErrInvalid
		}
		switch key {
		case "kind", "sourceId", "search", "group", "favorites", "includeHidden", "sort", "offset", "limit", "revision", "timezone", "anchorChannelId", "anchorSourceId", "anchorProvenance", "direction":
		default:
			return out, livechannels.ErrInvalid
		}
	}
	if q.Has("kind") {
		out.Kind = q.Get("kind")
	}
	if q.Has("sort") {
		out.Sort = q.Get("sort")
	}
	out.SourceID = q.Get("sourceId")
	out.Search = q.Get("search")
	out.Group = q.Get("group")
	out.Revision = q.Get("revision")
	out.AnchorChannelID = q.Get("anchorChannelId")
	out.AnchorSourceID = q.Get("anchorSourceId")
	out.AnchorProvenance = q.Get("anchorProvenance")
	out.Direction = q.Get("direction")
	for key, target := range map[string]*int{"offset": &out.Offset, "limit": &out.Limit} {
		if q.Has(key) {
			v, err := strconv.Atoi(q.Get(key))
			if err != nil || strconv.Itoa(v) != q.Get(key) {
				return out, livechannels.ErrInvalid
			}
			*target = v
		}
	}
	for key, target := range map[string]*bool{"favorites": &out.FavoritesOnly, "includeHidden": &out.IncludeHidden} {
		if q.Has(key) {
			if q.Get(key) != "true" && q.Get(key) != "false" {
				return out, livechannels.ErrInvalid
			}
			*target = q.Get(key) == "true"
		}
	}
	if zone := q.Get("timezone"); zone != "" {
		if _, err := time.LoadLocation(zone); err != nil {
			return out, livechannels.ErrInvalid
		}
	}
	if out.Direction != "" {
		if q.Has("offset") || (q.Has("limit") && out.Limit != 1) {
			return out, livechannels.ErrInvalid
		}
		out.Limit = 1
	}
	return out, nil
}

func (d Dependencies) guideDirectoryAuthority(p identity.Principal, restrictionFence string) livechannels.DirectoryAuthority {
	return func(ctx context.Context, tx *sql.Tx) (livechannels.DirectoryScope, error) {
		fence, allowed, err := d.liveAuthority(p)(ctx, tx, false)
		if err != nil {
			return livechannels.DirectoryScope{}, err
		}
		scope := livechannels.DirectoryScope{Viewer: channelSubject(p), AllowedLiveSources: []string{}, LibraryRows: []livechannels.DirectoryRow{}}
		rows, err := tx.QueryContext(ctx, `SELECT id FROM live_sources WHERE state='active' ORDER BY id`)
		if err != nil {
			return scope, livechannels.ErrUnavailable
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return scope, livechannels.ErrUnavailable
			}
			if allowed(id, "") {
				scope.AllowedLiveSources = append(scope.AllowedLiveSources, id)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return scope, livechannels.ErrUnavailable
		}
		libraryFence := ""
		if d.LibraryChannels != nil {
			scope.LibraryRows, libraryFence, err = d.LibraryChannels.DirectoryRowsTx(ctx, tx, d.libraryAuthority(p), channelSubject(p))
			if err != nil {
				return scope, err
			}
		}
		hash := sha256.Sum256([]byte(fence + ":" + libraryFence + ":" + restrictionFence))
		scope.Fence = hex.EncodeToString(hash[:])
		return scope, nil
	}
}

func (d Dependencies) annotateDirectory(ctx context.Context, channels []livechannels.Channel) error {
	if d.Administration != nil {
		ids := []string{}
		for _, c := range channels {
			if c.Provenance == livechannels.LiveSource {
				ids = append(ids, "live:"+c.SourceID+":"+c.ID)
			}
		}
		paths, err := d.Administration.ChannelLogoPaths(ctx, ids)
		if err != nil {
			return err
		}
		for i := range channels {
			if channels[i].Provenance == livechannels.LiveSource {
				channels[i].LogoPath = paths["live:"+channels[i].SourceID+":"+channels[i].ID]
			}
		}
	}
	available, reason := d.guideDeliveryAvailability()
	for i := range channels {
		c := &channels[i]
		c.TuneAvailable = available && c.Generation != ""
		c.TuneUnavailableReason = reason
		if available && c.Generation == "" {
			c.TuneUnavailableReason = "schedule_preparing"
		}
		if c.Provenance == livechannels.LiveSource {
			c.RecordAvailable = d.DVR != nil && d.DVR.CaptureAvailable() && c.Generation != ""
			if c.RecordAvailable {
				c.RecordUnavailableReason = ""
			}
		}
	}
	return nil
}
func (d Dependencies) guideDeliveryAvailability() (bool, string) {
	if d.PlaybackRuntime != nil && d.PlaybackRuntime.Linear != nil {
		return d.PlaybackRuntime.Linear.Available()
	}
	return false, "channel_runtime_unavailable"
}

func (d Dependencies) guideDirectoryRoutes(mux *http.ServeMux, store *livechannels.Store, handle func(bool, func(context.Context, http.ResponseWriter, *http.Request, identity.Principal) (any, error)) http.HandlerFunc) {
	mux.HandleFunc("GET /v1/guide/channels", handle(false, func(ctx context.Context, _ http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		query, err := directoryQuery(r.URL.Query())
		if err != nil {
			return nil, err
		}
		if query.IncludeHidden && !channelOwner(p) {
			return nil, livechannels.ErrDenied
		}
		_, fence, err := d.viewerRestrictions(r, p)
		if err != nil {
			return nil, err
		}
		query.DeliveryAvailable, _ = d.guideDeliveryAvailability()
		directory, err := store.ChannelDirectory(ctx, d.guideDirectoryAuthority(p, fence), query)
		if err != nil {
			return nil, err
		}
		if err = d.annotateDirectory(ctx, directory.Channels); err != nil {
			return nil, err
		}
		_, current, err := d.viewerRestrictions(r, p)
		if err != nil {
			return nil, err
		}
		if current != fence {
			return nil, livechannels.ErrCursor
		}
		return map[string]any{"protocolVersion": "1.0", "serverId": d.Identity.ID(), "directory": directory}, nil
	}))
	mux.HandleFunc("GET /v1/guide/sources", handle(false, func(ctx context.Context, _ http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		q := r.URL.Query()
		for key, values := range q {
			if len(values) != 1 || (key != "timezone" && key != "includeHidden") {
				return nil, livechannels.ErrInvalid
			}
		}
		query, err := directoryQuery(q)
		if err != nil {
			return nil, err
		}
		if query.IncludeHidden && !channelOwner(p) {
			return nil, livechannels.ErrDenied
		}
		_, fence, err := d.viewerRestrictions(r, p)
		if err != nil {
			return nil, err
		}
		summary, err := store.ChannelSourceSummary(ctx, d.guideDirectoryAuthority(p, fence), query.IncludeHidden, time.Now())
		if err != nil {
			return nil, err
		}
		available, reason := d.guideDeliveryAvailability()
		for i := range summary.Sources {
			s := &summary.Sources[i]
			s.RecordAvailable = s.Provenance == livechannels.LiveSource && d.DVR != nil && d.DVR.CaptureAvailable()
			if !available {
				s.UnavailableReasons[reason] = s.ChannelCount
			}
		}
		_, current, err := d.viewerRestrictions(r, p)
		if err != nil {
			return nil, err
		}
		if current != fence {
			return nil, livechannels.ErrCursor
		}
		return map[string]any{"protocolVersion": "1.0", "serverId": d.Identity.ID(), "viewerFence": summary.ViewerFence, "revision": summary.Revision, "sources": summary.Sources}, nil
	}))
}
