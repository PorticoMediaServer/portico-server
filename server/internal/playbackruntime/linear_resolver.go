package playbackruntime

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"portico.local/server/internal/contentaccess"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/livechannels"
	library "portico.local/server/internal/livechannels/library"
	"portico.local/server/internal/playback"
)

// The resolver consumes already-established identity/family authority and the
// same cached local policy as source preparation. It never calls Hosted.
type channelResolver struct {
	available func() (bool, string)
	live      *livechannels.Store
	library   *library.Store
	policy    playback.CachedPreparationPolicy
}

// NewChannelResolver is the transaction boundary shared by the linear runtime
// and the HTTP control service. It applies live library policy to each selected
// programme before control grants are minted or retained.
func NewChannelResolver(live *livechannels.Store, scheduled *library.Store, policy playback.CachedPreparationPolicy, available func() (bool, string)) playback.LinearResolver {
	return &channelResolver{live: live, library: scheduled, policy: policy, available: available}
}

func channelFault(code string, status int) error {
	return &playback.ControlFault{Code: code, HTTPStatus: status}
}
func channelError(e error) error {
	if e == nil {
		return nil
	}
	switch {
	case errors.Is(e, identity.ErrContentRestricted):
		return e // mapped to not_found by the control service (SEC-02)
	case errors.Is(e, livechannels.ErrDenied):
		return channelFault("channel_permission_denied", 403)
	case errors.Is(e, library.ErrConflict), errors.Is(e, livechannels.ErrConflict):
		return channelFault("guide_refresh_required", 409)
	case errors.Is(e, livechannels.ErrCapacity), errors.Is(e, livechannels.ErrReservation):
		return channelFault("source_capacity_unavailable", 409)
	default:
		return channelFault("channel_source_unavailable", 503)
	}
}
func libraryChannelError(e error) error {
	if errors.Is(e, library.ErrDenied) || errors.Is(e, identity.ErrContentRestricted) {
		return channelFault("not_found", 404)
	}
	return channelError(e)
}
func (r *channelResolver) scope(ctx context.Context, tx *sql.Tx, p identity.Principal) (library.Scope, error) {
	if e := r.policy.AllowedTxContext(ctx, p, "", tx); e != nil {
		return library.Scope{}, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT id FROM libraries ORDER BY id`)
	if e != nil {
		return library.Scope{}, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return library.Scope{}, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return library.Scope{}, e
	}
	allowed := map[string]bool{}
	for _, id := range ids {
		e = r.policy.AllowedTxContext(ctx, p, id, tx)
		if e == nil {
			allowed[id] = true
		} else if !errors.Is(e, identity.ErrUnauthorized) {
			return library.Scope{}, e
		}
	}
	return library.Scope{Principal: p, Fence: p.ServerID + ":" + p.Authority + ":" + p.AccountID + ":" + p.ProfileID, Owner: p.Authority == "local" && p.Role == "owner", AllowsLibrary: func(id string) bool { return allowed[id] }}, nil
}
func (r *channelResolver) liveSelection(ctx context.Context, tx *sql.Tx, p identity.Principal, ref playback.LinearReference, checkGeneration bool) (playback.LinearSelection, error) {
	var v playback.LinearSelection
	if e := r.policy.AllowedTxContext(ctx, p, "", tx); e != nil {
		return v, e
	}
	var access, gen, state, name string
	var revision int64
	e := tx.QueryRowContext(ctx, `SELECT s.state,s.active_generation,s.revision,c.name,COALESCE(x.viewer_access,'owner-only') FROM live_sources s JOIN live_channel_versions c ON c.generation_id=s.active_generation LEFT JOIN live_source_settings x ON x.source_id=s.id WHERE s.id=? AND c.channel_id=?`, ref.SourceID, ref.ChannelID).Scan(&state, &gen, &revision, &name, &access)
	if errors.Is(e, sql.ErrNoRows) {
		return v, channelFault("channel_removed", 410)
	}
	if e != nil {
		return v, e
	}
	if state != "active" {
		return v, channelFault("source_disabled", 410)
	}
	if !(p.Authority == "local" && p.Role == "owner") && access != "server-members" {
		return v, identity.ErrUnauthorized
	}
	if checkGeneration && ref.Generation != gen {
		return v, channelFault("guide_refresh_required", 409)
	}
	ref.Generation = gen
	v = playback.LinearSelection{Reference: ref, Name: name, SourceFence: strconv.FormatInt(revision, 10), Quality: playback.ControlQuality{Mode: "automatic", AllowLossy: true}}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var programme playback.LinearProgramme
	var start, end string
	e = tx.QueryRowContext(ctx, `SELECT id,title,start_utc,end_utc FROM live_programmes WHERE generation_id=? AND channel_id=? AND start_utc<=? AND end_utc>? ORDER BY start_utc DESC LIMIT 1`, gen, ref.ChannelID, now, now).Scan(&programme.ID, &programme.Title, &start, &end)
	if e == nil {
		a, ae := time.Parse(time.RFC3339Nano, start)
		b, be := time.Parse(time.RFC3339Nano, end)
		if ae == nil && be == nil {
			v.EntryID = programme.ID
			v.Title = programme.Title
			v.EntryStartMS = a.UnixMilli()
			v.EntryEndMS = b.UnixMilli()
		}
	}
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return v, e
	}
	return v, nil
}

// liveTVAllowed applies the profile's Live TV switch (BE-MEDIA-03, SEC-03) to
// every channel resolution: a start, the running producer's checks (every
// 500 ms) and every media request. A switch turned off mid-play therefore
// stops the producer at once (a control fault, which the readiness monitor
// never treats as transient) and refuses the next segment. Library Channels
// are channels too: the switch covers both kinds, as the start admission does.
func liveTVAllowed(ctx context.Context, tx *sql.Tx, p identity.Principal) error {
	restrictions, e := identity.RestrictionsForViewerTx(ctx, tx, p.Viewer)
	if e != nil {
		return e
	}
	if !restrictions.AllowLiveTV {
		return channelFault("feature_restricted", 403)
	}
	return nil
}

func (r *channelResolver) ResolveTx(ctx context.Context, tx *sql.Tx, p identity.Principal, ref playback.LinearReference, existing string, now time.Time) (playback.LinearSelection, error) {
	if e := liveTVAllowed(ctx, tx, p); e != nil {
		return playback.LinearSelection{}, e
	}
	if existing == "" && r.available != nil {
		if ok, code := r.available(); !ok {
			return playback.LinearSelection{}, channelFault(code, 503)
		}
	}
	if ref.Kind == "live-source" {
		return r.liveSelection(ctx, tx, p, ref, existing == "")
	}
	scope, e := r.scope(ctx, tx, p)
	if e != nil {
		return playback.LinearSelection{}, e
	}
	t, e := r.library.ResolveTuneTx(ctx, tx, scope, ref.ChannelID, ref.Generation, existing, now)
	if e != nil {
		return playback.LinearSelection{}, libraryChannelError(e)
	}
	if e = contentaccess.VisibleItemTx(ctx, tx, p, t.Entry.ItemID); e != nil {
		return playback.LinearSelection{}, e
	}
	if t.Next != nil && t.Next.ItemID != "" && contentaccess.VisibleItemTx(ctx, tx, p, t.Next.ItemID) != nil {
		t.Next.ItemID = ""
		t.Next.Title = "Unavailable program"
		t.Next.SlateReason = "unavailable"
	}
	ref.Generation = t.Generation
	ref.SourceID = t.ChannelID
	v := playback.LinearSelection{Reference: ref, Name: t.Name, ItemID: t.Entry.ItemID, AssetID: t.Entry.AssetID, LibraryID: t.Entry.LibraryID, SourceFence: t.Entry.SourceFence, EntryID: t.Entry.ID, EntryStartMS: t.Entry.StartMS, EntryEndMS: t.Entry.EndMS, SourceOffsetMS: t.Entry.SourceOffsetMS, DurationMS: t.Candidate.DurationMS, Title: t.Entry.Title, LogoItemID: t.LogoItemID, Overlay: playback.LinearOverlay(t.Overlay), Quality: playback.ControlQuality{Mode: t.Quality.Mode, AllowLossy: t.Quality.AllowLossy, AllowHDRToSDR: t.Quality.AllowHDRToSDR}}
	if t.Quality.MaxBitrate > 0 {
		s := int(t.Quality.MaxBitrate)
		v.Quality.MaxBitrateBPS = &s
	}
	if t.Quality.MaxHeight > 0 {
		n := t.Quality.MaxHeight
		v.Quality.MaxHeight = &n
	}
	if t.Next != nil {
		v.Next = &playback.LinearProgramme{ID: t.Next.ID, ItemID: t.Next.ItemID, Title: t.Next.Title, StartMS: t.Next.StartMS, EndMS: t.Next.EndMS}
	}
	// Logo metadata is subject to its own library permission, not the programme's.
	if v.LogoItemID != "" {
		var lid string
		e = tx.QueryRowContext(ctx, `SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?)`, v.LogoItemID).Scan(&lid)
		if e != nil || !scope.AllowsLibrary(lid) || contentaccess.VisibleItemTx(ctx, tx, p, v.LogoItemID) != nil {
			v.LogoItemID = ""
			v.Overlay.Enabled = false
		}
	}
	return v, nil
}
func (r *channelResolver) CheckTx(ctx context.Context, tx *sql.Tx, p identity.Principal, v playback.LinearSelection) error {
	if e := liveTVAllowed(ctx, tx, p); e != nil {
		return e
	}
	if v.Reference.Kind == "live-source" {
		_, e := r.liveSelection(ctx, tx, p, v.Reference, false)
		return e
	}
	scope, e := r.scope(ctx, tx, p)
	if e != nil {
		return e
	}
	_, e = r.library.CheckRetainedTx(ctx, tx, scope, v.Reference.ChannelID, library.Candidate{ItemID: v.ItemID, AssetID: v.AssetID, LibraryID: v.LibraryID, SourceFence: v.SourceFence, DurationMS: v.DurationMS, Title: v.Title})
	if e != nil {
		return libraryChannelError(e)
	}
	return contentaccess.VisibleItemTx(ctx, tx, p, v.ItemID)
}
func (r *channelResolver) BindTx(ctx context.Context, tx *sql.Tx, id string, v playback.LinearSelection, expiry time.Time) error {
	if v.Reference.Kind != "library-channel" {
		return nil
	}
	return library.BindPlaybackTx(ctx, tx, id, library.Tune{ChannelID: v.Reference.ChannelID, Generation: v.Reference.Generation, Entry: library.Entry{ID: v.EntryID, EndMS: v.EntryEndMS}}, expiry)
}
