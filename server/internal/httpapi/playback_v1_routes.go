package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portico.local/apikit"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/contentaccess"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediaanalysis"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/playbackv1"
)

// v1Route registers one route, failing registry construction loudly: a route
// that cannot be registered is a programming error, not a runtime condition.
func v1Route[Req, Resp any](r *apikit.Registry, m apikit.Metadata, h func(context.Context, *http.Request, Req) (Resp, error)) {
	if err := apikit.Register(r, apikit.Route[Req, Resp]{Metadata: m, Handler: func(ctx context.Context, q *http.Request, in Req) (Resp, error) {
		out, err := h(ctx, q, in)
		return out, v1Error(err)
	}}); err != nil {
		panic(m.ID + ": " + err.Error())
	}
}

// playbackV1 is the router's single v1 resource service.
func (d Dependencies) playbackV1() *playbackv1.Service { return d.v1 }

func newPlaybackV1(d Dependencies) *playbackv1.Service {
	s := buildPlaybackV1(d)
	if s == nil {
		return nil
	}
	// Queue sources and windows are authorized like every other catalog read:
	// library access, then the item's restrictions (SEC-02).
	s.LibraryCheck = func(ctx context.Context, tx *sql.Tx, p identity.Principal, library string) error {
		return d.allowedLibraryTx(ctx, p, library, tx)
	}
	s.Visibility = func(ctx context.Context, tx *sql.Tx, p identity.Principal, item string) (string, []any, error) {
		return contentaccess.VisibleItemsSQL(ctx, tx, p, item, func(library string) error { return d.allowedLibraryTx(ctx, p, library, tx) })
	}
	s.PostPlay = func(ctx context.Context, tx *sql.Tx, p identity.Principal) (bool, int, error) {
		values, _, _, err := operations.EffectivePreferences(tx, p.Viewer, "")
		if err != nil {
			return false, 0, err
		}
		return values.Bool("playback.autoplayNext"), values.Int("playback.upNextCountdownSeconds"), nil
	}
	if d.AudioMedia != nil {
		s.MeasureAudio = d.AudioMedia.MeasureAudio
	}
	if d.Console != nil {
		// Marker skips land in the runtime diagnostics lane; the console store owns
		// validation, retention and the lane cap.
		s.SkipEvidence = func(ctx context.Context, code string, fields map[string]int64) {
			_ = d.Console.Record(ctx, "runtime", "info", "playback", code, fields)
		}
		// The paused-session limit (Plex "Terminate Sessions Paused for Longer
		// Than"): video sessions paused longer than this are ended by the
		// sweeper. Unset or non-positive: off; a read error: off for that tick.
		s.PausedLimit = func(ctx context.Context) time.Duration {
			document, err := d.Console.Settings(ctx, operations.AllowServerScope)
			if err != nil {
				return 0
			}
			if minutes := document.Effective.PausedSessionTimeoutMinutes; minutes > 0 {
				return time.Duration(minutes) * time.Minute
			}
			return 0
		}
	}
	// Channels on v1 sessions (§18.6): the runtime's channel sessions, its wake,
	// and the live sources' tuner capacity.
	if rt := d.PlaybackRuntime; rt != nil && rt.Channels != nil && rt.Channels.Configured() {
		s.Channels = rt.Channels
		s.ChannelWake = rt.Wake
		s.TunerAvailable = liveTunerAvailable
	}
	s.CreatePlaylist = func(ctx context.Context, p identity.Principal, key, name, summary string, items []string) (string, int64, error) {
		if d.Catalog == nil {
			return "", 0, playbackv1.ErrUnsupportedSelector
		}
		m := catalog.PlaylistMutation{OperationID: key, Name: &name, ItemIDs: items}
		if summary != "" {
			m.Summary = &summary
		}
		// The playlists route's own authorization: the caller, and every item's library.
		out, err := d.Catalog.MutatePlaylist(resourceActor(p), "", "create", "", m, func(tx *sql.Tx) error {
			for _, item := range items {
				var library string
				if err := tx.QueryRow(`SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?)`, item).Scan(&library); err != nil {
					return err
				}
				if err := d.resourceAuthorize(tx, p, library); err != nil {
					return err
				}
			}
			return d.resourceAuthorize(tx, p, "")
		})
		if errors.Is(err, catalog.ErrOperationConflict) {
			err = playbackv1.ErrIdempotencyMismatch
		}
		return out.PlaylistID, out.Revision, err
	}
	s.Unrestricted = func(ctx context.Context, tx *sql.Tx, p identity.Principal) (bool, error) {
		return contentaccess.Unrestricted(ctx, tx, p, func(library string) error { return d.allowedLibraryTx(ctx, p, library, tx) })
	}
	return s
}

func buildPlaybackV1(d Dependencies) *playbackv1.Service {
	if s := d.PlaybackV1; s != nil {
		if s.DB == nil {
			s.DB = d.DB
		}
		if s.Playback == nil {
			s.Playback = d.Playback
		}
		if s.Subtitles == nil {
			s.Subtitles = d.Subtitles
		}
		return s
	}
	return playbackv1.New(d.DB, d.Playback, d.Subtitles)
}

func jsonStrings(v []string) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

type noBody struct{}

// registerPlaybackV1 adds every Playback Protocol v1 route to the registry.
func registerPlaybackV1(r *apikit.Registry, d Dependencies) {
	registerPlaybackV1Options(r, d)
	registerPlaybackV1Sessions(r, d)
	registerPlaybackV1Queues(r, d)
	registerPlaybackV1Admin(r, d)
}

// AdminTerminateRequest is POST /v1/admin/sessions/{id}:terminate.
type AdminTerminateRequest struct {
	Message string `json:"message,omitempty"`
}

// registerPlaybackV1Admin is Now Playing (spec §14): the active sessions and
// terminate, for administrators. admin.sessions events say when to refetch.
func registerPlaybackV1Admin(r *apikit.Registry, d Dependencies) {
	v1Route(r, apikit.Metadata{ID: "list_admin_sessions", Method: "GET", Path: "/v1/admin/sessions", Summary: "Active playback sessions: who, what, how it's delivered and from where",
		Access: apikit.Admin, Scope: "playback", Lane: apikit.Default, Cost: apikit.PageSized, Query: []string{"limit", "cursor"}, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted"}},
		func(ctx context.Context, q *http.Request, _ noBody) (playbackv1.AdminSessionPage, error) {
			limit := 0
			if text := q.URL.Query().Get("limit"); text != "" {
				n, err := strconv.Atoi(text)
				if err != nil {
					return playbackv1.AdminSessionPage{}, &playbackv1.FieldError{Path: "limit"}
				}
				limit = n
			}
			return d.playbackV1().AdminSessions(ctx, limit, q.URL.Query().Get("cursor"))
		})
	v1Route(r, apikit.Metadata{ID: "terminate_admin_session", Method: "POST", Path: "/v1/admin/sessions/{id}:terminate", Summary: "End a viewer's playback, showing them an optional message; idempotent",
		Access: apikit.Admin, Scope: "playback", Lane: apikit.Default, Cost: apikit.Constant, BodyLimit: 4 << 10, OptionalBody: true, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found"}},
		func(ctx context.Context, q *http.Request, in AdminTerminateRequest) (noBody, error) {
			return noBody{}, d.playbackV1().Terminate(ctx, q.PathValue("id"), in.Message)
		})
}

// v1Caller turns the admitted caller into the service's caller, with media
// admission for new presentations and channel admission for channel starts.
func (d Dependencies) v1ServiceCaller(q *http.Request, c v1Caller) playbackv1.Caller {
	return playbackv1.Caller{Principal: c.Principal, DeviceID: c.DeviceID, Remote: d.v1Remote(q), Admit: func(ctx context.Context, item string) (int, error) {
		// A channel start admits with no item: there is no title to fence,
		// only the account-level limits. The channel allow/deny list runs
		// separately through AdmitChannel.
		if item != "" {
			if err := itemAdmission(d.itemAccess(ctx, c.Principal, item)); err != nil {
				return 0, err
			}
		}
		decision, err := d.admitPlayback(q.WithContext(ctx), c.Principal, item)
		if err != nil {
			return 0, itemAdmission(err)
		}
		return decision.MaxVideoBitrateBPS, nil
	}, AdmitChannel: func(ctx context.Context, channel string) error {
		if err := d.admitChannel(q.WithContext(ctx), c.Principal, channel); err != nil {
			return itemAdmission(err)
		}
		return nil
	}}
}

// v1Remote is whether a request came from outside this server's LAN, decided by
// the server from the connection, never by the client's own claim. Remote
// bitrate caps apply only then.
func (d Dependencies) v1Remote(q *http.Request) bool {
	return d.requestIsRemote(q)
}

// v1Reach is a preview's reach: the same remote flag and member cap a start
// from this connection would get, read without admitting anything.
func (d Dependencies) v1Reach(q *http.Request, p identity.Principal) playbackv1.Reach {
	reach := playbackv1.Reach{Remote: d.v1Remote(q)}
	if !reach.Remote || d.Access.Access == nil || d.DB == nil {
		return reach
	}
	tx, done, err := dbwork.BeginRead(q.Context(), d.DB)
	if err != nil {
		return reach
	}
	defer done()
	if limit, err := d.enforcer(false).RemoteBitrateCap(q.Context(), tx, p); err == nil {
		reach.AdminMaxVideoBitrateBPS = limit
	}
	return reach
}

func setSessionHeaders(ctx context.Context, v playbackv1.SessionView) {
	apikit.SetHeader(ctx, "ETag", `"`+v.Revision+`"`)
}

func registerPlaybackV1Sessions(r *apikit.Registry, d Dependencies) {
	sessionErrors := []string{"invalid_request", "unauthorized", "not_found", "session_ended", "revision_required", "revision_mismatch"}
	v1Route(r, apikit.Metadata{ID: "start_playback_session", Method: "POST", Path: "/v1/playback/sessions", Summary: "Start playback (201), or report that it is being prepared (202)",
		Access: apikit.Device, Lane: apikit.Media, Cost: apikit.Constant, BodyLimit: 8 << 10, Status: 201,
		Errors: []string{"invalid_request", "unauthorized", "not_found", "idempotency_key_required", "idempotency_key_reused", "stream_limit_reached", "transcode_not_allowed", "source_unavailable", "unsupported_media", "feature_restricted", "no_tuner_available", "queue_building"}},
		func(ctx context.Context, q *http.Request, in playbackv1.StartRequest) (playbackv1.SessionView, error) {
			c := d.v1ServiceCaller(q, callerFrom(ctx))
			v, preparing, err := d.playbackV1().Start(ctx, c, q.Header.Get("Idempotency-Key"), in)
			if err != nil {
				return v, err
			}
			if preparing {
				apikit.SetStatus(ctx, 202)
				apikit.SetHeader(ctx, "Retry-After", "1")
			}
			setSessionHeaders(ctx, v)
			return v, nil
		})
	v1Route(r, apikit.Metadata{ID: "get_playback_session", Method: "GET", Path: "/v1/playback/sessions/{id}", Summary: "Read a playback session",
		Access: apikit.Device, Lane: apikit.Media, Cost: apikit.Constant, Status: 200, Errors: []string{"unauthorized", "not_found"}},
		func(ctx context.Context, q *http.Request, _ noBody) (playbackv1.SessionView, error) {
			v, err := d.playbackV1().Get(ctx, d.v1ServiceCaller(q, callerFrom(ctx)), q.PathValue("id"))
			if err == nil {
				setSessionHeaders(ctx, v)
			}
			return v, err
		})
	v1Route(r, apikit.Metadata{ID: "change_playback_session", Method: "PATCH", Path: "/v1/playback/sessions/{id}", Summary: "Change tracks, quality, version or state; a change to the bytes starts a new generation",
		Access: apikit.Device, Lane: apikit.Media, Cost: apikit.Constant, BodyLimit: 4 << 10, Status: 200,
		Errors: append(sessionErrors, "stream_limit_reached", "transcode_not_allowed", "unsupported_media")},
		func(ctx context.Context, q *http.Request, in playbackv1.Change) (playbackv1.SessionView, error) {
			v, err := d.playbackV1().Patch(ctx, d.v1ServiceCaller(q, callerFrom(ctx)), q.PathValue("id"), q.Header.Get("If-Match"), in)
			if err == nil {
				setSessionHeaders(ctx, v)
			}
			return v, err
		})
	v1Route(r, apikit.Metadata{ID: "stop_playback_session", Method: "DELETE", Path: "/v1/playback/sessions/{id}", Summary: "Stop playback; idempotent",
		Access: apikit.Device, Lane: apikit.Media, Cost: apikit.Constant, BodyLimit: 1 << 10, OptionalBody: true, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_found"}},
		func(ctx context.Context, q *http.Request, in playbackv1.StopRequest) (noBody, error) {
			return noBody{}, d.playbackV1().Stop(ctx, d.v1ServiceCaller(q, callerFrom(ctx)), q.PathValue("id"), in.PositionMs)
		})
	v1Route(r, apikit.Metadata{ID: "report_playback_timeline", Method: "POST", Path: "/v1/playback/sessions/{id}/timeline", Summary: "Report progress; renews the lease",
		Access: apikit.Device, Lane: apikit.Media, Cost: apikit.Constant, BodyLimit: 4 << 10, Status: 204, Errors: sessionErrors},
		func(ctx context.Context, q *http.Request, in playbackv1.Report) (noBody, error) {
			every, err := d.playbackV1().Timeline(ctx, d.v1ServiceCaller(q, callerFrom(ctx)), q.PathValue("id"), in)
			if err == nil {
				apikit.SetHeader(ctx, "Report-Every-Ms", strconv.FormatInt(every.Milliseconds(), 10))
			}
			return noBody{}, err
		})
}

func registerPlaybackV1Options(r *apikit.Registry, d Dependencies) {
	v1Route(r, apikit.Metadata{ID: "put_device_capabilities", Method: "PUT", Path: "/v1/me/devices/current/capabilities", Summary: "Replace this device's playback capability profile",
		Access: apikit.Device, Lane: apikit.Default, Cost: apikit.Constant, BodyLimit: 32 << 10, Status: 204,
		Errors: []string{"invalid_request", "unauthorized", "revision_mismatch"}},
		func(ctx context.Context, q *http.Request, in playbackv1.Capabilities) (noBody, error) {
			c := callerFrom(ctx)
			revision, err := d.playbackV1().PutCapabilities(ctx, c.Principal, c.DeviceID, q.Header.Get("If-Match"), in)
			if err == nil {
				apikit.SetHeader(ctx, "ETag", playbackv1.ETag(revision))
			}
			return noBody{}, err
		})
	v1Route(r, apikit.Metadata{ID: "get_device_capabilities", Method: "GET", Path: "/v1/me/devices/current/capabilities", Summary: "Read this device's playback capability profile",
		Access: apikit.Device, Lane: apikit.Default, Cost: apikit.Constant, Status: 200,
		Errors: []string{"unauthorized", "not_found"}},
		func(ctx context.Context, _ *http.Request, _ noBody) (playbackv1.CapabilitiesDocument, error) {
			doc, err := d.playbackV1().Capabilities(ctx, callerFrom(ctx).DeviceID)
			if err == nil {
				apikit.SetHeader(ctx, "ETag", playbackv1.ETag(doc.Revision))
			}
			return doc, err
		})
	v1Route(r, apikit.Metadata{ID: "get_playback_options", Method: "GET", Path: "/v1/items/{itemId}/playback-options", Summary: "Read what a title offers and what this device would get",
		Access: apikit.ViewerItem, Lane: apikit.Media, Cost: apikit.Constant, Status: 200,
		Query:  []string{"versionId", "partId", "audioId", "subtitleId", "quality", "maxVideoBitrateKbps", "maxHeight", "maxAudioBitrateKbps"},
		Errors: []string{"invalid_request", "unauthorized", "not_found"}},
		func(ctx context.Context, q *http.Request, _ noBody) (playbackv1.Options, error) {
			c := callerFrom(ctx)
			preview, err := previewFrom(q)
			if err != nil {
				return playbackv1.Options{}, err
			}
			return d.playbackV1().Options(ctx, c.Principal, q.PathValue("itemId"), preview, d.v1Markers(q, c.Principal), d.v1Reach(q, c.Principal))
		})
}

// previewFrom reads the options preview query (plan §7 item 15).
func previewFrom(q *http.Request) (playbackv1.Preview, error) {
	v := q.URL.Query()
	p := playbackv1.Preview{VersionID: v.Get("versionId"), PartID: v.Get("partId"), AudioID: v.Get("audioId"), SubtitleID: v.Get("subtitleId"), Quality: playbackv1.Quality{Mode: v.Get("quality")}}
	number := func(key string) (int, error) {
		raw := v.Get(key)
		if raw == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return 0, &playbackv1.FieldError{Path: key}
		}
		return n, nil
	}
	var err error
	if p.Quality.MaxVideoBitrateKbps, err = number("maxVideoBitrateKbps"); err != nil {
		return p, err
	}
	if p.Quality.MaxHeight, err = number("maxHeight"); err != nil {
		return p, err
	}
	if p.Quality.MaxAudioBitrateKbps, err = number("maxAudioBitrateKbps"); err != nil {
		return p, err
	}
	if p.Quality.Mode == "" {
		if p.Quality.MaxVideoBitrateKbps+p.Quality.MaxHeight+p.Quality.MaxAudioBitrateKbps > 0 {
			return p, &playbackv1.FieldError{Path: "quality"}
		}
		return p, nil
	}
	return p, p.Quality.Validate("quality")
}

// v1Markers projects intro/credits markers through the analysis viewer
// projection under this viewer's library authorization.
func (d Dependencies) v1Markers(q *http.Request, p identity.Principal) playbackv1.MarkerFunc {
	if d.Catalog == nil || d.Identity == nil {
		return nil
	}
	return func(ctx context.Context, tx *sql.Tx, _ identity.Principal, item, source string) ([]playbackv1.Marker, error) {
		r := q.Clone(ctx)
		r.SetPathValue("id", item)
		library, fence, err := d.detailFenceFor(r, p)
		if err != nil {
			return nil, nil
		}
		set, err := mediaanalysis.ViewerMarkersTx(ctx, tx, d.markerAccess(p, library, item, fence), source)
		if err != nil {
			return nil, nil
		}
		out := make([]playbackv1.Marker, 0, len(set.Markers))
		for _, m := range set.Markers {
			kind := m.Kind
			if kind == "outro" {
				kind = "credits"
			}
			out = append(out, playbackv1.Marker{ID: m.ID, Type: kind, StartMs: int64(m.StartSeconds * 1000), EndMs: int64(m.EndSeconds * 1000), Confidence: "detected", AutomaticSafe: m.AutomaticSafe})
		}
		return out, nil
	}
}

func setQueueETag(ctx context.Context, revision string) {
	apikit.SetHeader(ctx, "ETag", `"`+revision+`"`)
}

func registerPlaybackV1Queues(r *apikit.Registry, d Dependencies) {
	command := []string{"invalid_request", "unauthorized", "not_found", "revision_required", "revision_mismatch", "unsupported_selector", "queue_too_large", "queue_building"}
	reply := func(ctx context.Context, out playbackv1.QueueReply, err error) (playbackv1.QueueReply, error) {
		if err == nil {
			setQueueETag(ctx, out.Queue.Revision)
		}
		return out, err
	}
	caller := func(ctx context.Context, q *http.Request) playbackv1.Caller {
		return d.v1ServiceCaller(q, callerFrom(ctx))
	}
	v1Route(r, apikit.Metadata{ID: "create_queue", Method: "POST", Path: "/v1/queues", Summary: "Play: replace this device's queue from selectors, optionally starting the first session",
		Access: apikit.Device, Lane: apikit.Media, Cost: apikit.SelectionAsync, BodyLimit: 64 << 10, Status: 201,
		Errors: append(command, "idempotency_key_required", "idempotency_key_reused", "stream_limit_reached", "transcode_not_allowed", "unsupported_media", "source_unavailable")},
		func(ctx context.Context, q *http.Request, in playbackv1.CreateQueueRequest) (playbackv1.QueueReply, error) {
			out, err := d.playbackV1().CreateQueue(ctx, caller(ctx, q), q.Header.Get("Idempotency-Key"), in)
			return reply(ctx, out, err)
		})
	v1Route(r, apikit.Metadata{ID: "get_queue", Method: "GET", Path: "/v1/queues/{id}", Summary: "Read a queue's header",
		Access: apikit.Device, Lane: apikit.Browsing, Cost: apikit.Constant, Status: 200, Errors: []string{"unauthorized", "not_found"}},
		func(ctx context.Context, q *http.Request, _ noBody) (playbackv1.QueueView, error) {
			out, err := d.playbackV1().Queue(ctx, caller(ctx, q), q.PathValue("id"))
			if err == nil {
				setQueueETag(ctx, out.Revision)
			}
			return out, err
		})
	number := func(q *http.Request, key string, fallback int64) (int64, error) {
		raw := q.URL.Query().Get(key)
		if raw == "" {
			return fallback, nil
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return 0, &playbackv1.FieldError{Path: key}
		}
		return n, nil
	}
	v1Route(r, apikit.Metadata{ID: "get_queue_window", Method: "GET", Path: "/v1/queues/{id}/window", Summary: "Read the entries around the current one (at most 200)",
		Access: apikit.Device, Lane: apikit.Browsing, Cost: apikit.PageSized, Query: []string{"around", "before", "after"}, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_found"}},
		func(ctx context.Context, q *http.Request, _ noBody) (playbackv1.EntryPage, error) {
			if around := q.URL.Query().Get("around"); around != "" && around != "current" {
				return playbackv1.EntryPage{}, &playbackv1.FieldError{Path: "around"}
			}
			before, err := number(q, "before", 20)
			if err != nil {
				return playbackv1.EntryPage{}, err
			}
			after, err := number(q, "after", 50)
			if err != nil {
				return playbackv1.EntryPage{}, err
			}
			return d.playbackV1().Window(ctx, caller(ctx, q), q.PathValue("id"), before, after)
		})
	v1Route(r, apikit.Metadata{ID: "list_queue_entries", Method: "GET", Path: "/v1/queues/{id}/entries", Summary: "Read a page of entries from a position (at most 200)",
		Access: apikit.Device, Lane: apikit.Browsing, Cost: apikit.PageSized, Query: []string{"from", "limit", "cursor"}, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_found"}},
		func(ctx context.Context, q *http.Request, _ noBody) (playbackv1.EntryPage, error) {
			from, err := number(q, "from", 0)
			if err != nil {
				return playbackv1.EntryPage{}, err
			}
			if cursor := q.URL.Query().Get("cursor"); cursor != "" {
				text, ok := strings.CutPrefix(cursor, "from:")
				if from, err = strconv.ParseInt(text, 10, 64); !ok || err != nil || from < 0 {
					return playbackv1.EntryPage{}, &playbackv1.FieldError{Path: "cursor"}
				}
			}
			limit, err := number(q, "limit", 50)
			if err != nil {
				return playbackv1.EntryPage{}, err
			}
			return d.playbackV1().Entries(ctx, caller(ctx, q), q.PathValue("id"), from, int(limit))
		})
	v1Route(r, apikit.Metadata{ID: "add_queue_segment", Method: "POST", Path: "/v1/queues/{id}/segments", Summary: "Play next, add to the queue, or insert after an entry",
		Access: apikit.Device, Lane: apikit.Media, Cost: apikit.SelectionAsync, BodyLimit: 32 << 10, Status: 200, Errors: command},
		func(ctx context.Context, q *http.Request, in playbackv1.AddSegmentRequest) (playbackv1.QueueReply, error) {
			out, err := d.playbackV1().AddSegment(ctx, caller(ctx, q), q.PathValue("id"), q.Header.Get("If-Match"), in)
			return reply(ctx, out, err)
		})
	v1Route(r, apikit.Metadata{ID: "remove_queue_entry", Method: "DELETE", Path: "/v1/queues/{id}/entries/{entryId}", Summary: "Remove one entry",
		Access: apikit.Device, Lane: apikit.Default, Cost: apikit.Constant, Status: 200, Errors: command},
		func(ctx context.Context, q *http.Request, _ noBody) (playbackv1.QueueReply, error) {
			out, err := d.playbackV1().RemoveEntry(ctx, caller(ctx, q), q.PathValue("id"), q.Header.Get("If-Match"), q.PathValue("entryId"))
			return reply(ctx, out, err)
		})
	v1Route(r, apikit.Metadata{ID: "move_queue_entry", Method: "POST", Path: "/v1/queues/{id}/entries/{entryId}:move", Summary: "Move one entry before or after another",
		Access: apikit.Device, Lane: apikit.Default, Cost: apikit.Constant, BodyLimit: 1 << 10, Status: 200, Errors: command},
		func(ctx context.Context, q *http.Request, in playbackv1.MoveRequest) (playbackv1.QueueReply, error) {
			out, err := d.playbackV1().MoveEntry(ctx, caller(ctx, q), q.PathValue("id"), q.Header.Get("If-Match"), q.PathValue("entryId"), in)
			return reply(ctx, out, err)
		})
	v1Route(r, apikit.Metadata{ID: "update_queue", Method: "PATCH", Path: "/v1/queues/{id}", Summary: "Set repeat, or shuffle on or off",
		Access: apikit.Device, Lane: apikit.Default, Cost: apikit.Constant, BodyLimit: 1 << 10, Status: 200, Errors: command},
		func(ctx context.Context, q *http.Request, in playbackv1.QueueChange) (playbackv1.QueueReply, error) {
			out, err := d.playbackV1().UpdateQueue(ctx, caller(ctx, q), q.PathValue("id"), q.Header.Get("If-Match"), in)
			return reply(ctx, out, err)
		})
	v1Route(r, apikit.Metadata{ID: "advance_queue", Method: "POST", Path: "/v1/queues/{id}:advance", Summary: "Move to the next, previous or a chosen entry; starts its session when one is playing",
		Access: apikit.Device, Lane: apikit.Media, Cost: apikit.Constant, BodyLimit: 1 << 10, Status: 200,
		Errors: append(command, "queue_ended", "stream_limit_reached", "transcode_not_allowed", "unsupported_media")},
		func(ctx context.Context, q *http.Request, in playbackv1.AdvanceRequest) (playbackv1.QueueReply, error) {
			out, err := d.playbackV1().Advance(ctx, caller(ctx, q), q.PathValue("id"), q.Header.Get("If-Match"), in)
			return reply(ctx, out, err)
		})
	v1Route(r, apikit.Metadata{ID: "prepare_queue_next", Method: "POST", Path: "/v1/queues/{id}:prepare-next", Summary: "Prepare the next entry's audio before the current one ends (spec §18.2; private until committed)",
		Access: apikit.Device, Lane: apikit.Media, Cost: apikit.Constant, BodyLimit: 1 << 10, Status: 200,
		Errors: append(command, "idempotency_key_required", "idempotency_key_reused", "queue_ended", "prepare_not_allowed", "not_implemented")},
		func(ctx context.Context, q *http.Request, in playbackv1.PrepareNextRequest) (playbackv1.PreparedNext, error) {
			out, err := d.playbackV1().PrepareNext(ctx, caller(ctx, q), q.PathValue("id"), q.Header.Get("If-Match"), q.Header.Get("Idempotency-Key"), in)
			return out, v1Error(err)
		})
	v1Route(r, apikit.Metadata{ID: "commit_queue_next", Method: "POST", Path: "/v1/queues/{id}:commit-next", Summary: "Commit the prepared next entry at the audio boundary: a new session inheriting state, rate, quality and track (spec §18.3)",
		Access: apikit.Device, Lane: apikit.Media, Cost: apikit.Constant, BodyLimit: 1 << 10, Status: 200,
		Errors: append(command, "prepared_expired", "prepared_canceled", "not_implemented")},
		func(ctx context.Context, q *http.Request, in playbackv1.CommitNextRequest) (playbackv1.QueueReply, error) {
			out, err := d.playbackV1().CommitNext(ctx, caller(ctx, q), q.PathValue("id"), in)
			return reply(ctx, out, err)
		})
	v1Route(r, apikit.Metadata{ID: "save_queue_as_playlist", Method: "POST", Path: "/v1/queues/{id}:save-as-playlist", Summary: "Save the queue, in play order, as a new playlist; a long queue is copied in the background and a replay of the key reports progress",
		Access: apikit.Device, Lane: apikit.Default, Cost: apikit.Constant, BodyLimit: 8 << 10, Status: 201,
		Errors: append(command, "idempotency_key_required", "idempotency_key_reused")},
		func(ctx context.Context, q *http.Request, in playbackv1.SaveAsPlaylistRequest) (playbackv1.SavedPlaylist, error) {
			out, err := d.playbackV1().SaveQueueAsPlaylist(ctx, caller(ctx, q), q.PathValue("id"), q.Header.Get("If-Match"), q.Header.Get("Idempotency-Key"), in)
			return out, v1Error(err)
		})
	v1Route(r, apikit.Metadata{ID: "delete_queue", Method: "DELETE", Path: "/v1/queues/{id}", Summary: "Clear the queue; idempotent",
		Access: apikit.Device, Lane: apikit.Default, Cost: apikit.Constant, Status: 204, Errors: []string{"unauthorized", "not_found"}},
		func(ctx context.Context, q *http.Request, _ noBody) (noBody, error) {
			return noBody{}, d.playbackV1().DeleteQueue(ctx, caller(ctx, q), q.PathValue("id"))
		})
}
