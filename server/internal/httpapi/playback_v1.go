package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/access"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/playbackv1"
)

// Playback Protocol v1 (Spec — Playback Protocol v1) is served from the typed
// apikit registry: every route declares its access rule, lane, cost class and
// errors, and the authorizer below enforces the rule before a body is read.

// v1Caller is the admitted principal and the device its token is bound to.
type v1Caller struct {
	identity.Principal
	DeviceID string
}
type v1CallerKey struct{}

func callerFrom(ctx context.Context) v1Caller {
	c, _ := ctx.Value(v1CallerKey{}).(v1Caller)
	return c
}

// v1ErrorDefinitions are the route errors v1 playback adds to the defaults.
// Messages are US English fallbacks; clients localise by code.
func v1ErrorDefinitions() []apierror.Definition {
	return []apierror.Definition{
		{Code: "idempotency_key_required", Status: 428, Message: "Send an Idempotency-Key with this request.", Retry: apierror.Never},
		{Code: "session_ended", Status: 410, Message: "This playback has ended.", Retry: apierror.Never},
		{Code: "stream_limit_reached", Status: 409, Message: "This account is already playing as many streams as it is allowed.", Retry: apierror.Never},
		{Code: "transcode_not_allowed", Status: 403, Message: "This title needs converting, and converting is turned off for this account.", Retry: apierror.Never},
		{Code: "source_unavailable", Status: 503, Message: "The file for this title can't be reached right now.", Retry: apierror.SameRequest},
		{Code: "unsupported_media", Status: 422, Message: "This title can't be played on this device.", Retry: apierror.Never},
		{Code: "feature_restricted", Status: 403, Message: "This feature is not permitted for this profile.", Retry: apierror.Never},
		{Code: "queue_ended", Status: 409, Message: "There is nothing after this in the queue.", Retry: apierror.Never},
		{Code: "unsupported_selector", Status: 422, Message: "This selection can't be played as a queue.", Retry: apierror.Never},
		{Code: "queue_too_large", Status: 422, Message: "This queue is too long. Remove some of it or start a new one.", Retry: apierror.Never},
		{Code: "prepare_not_allowed", Status: 409, Message: "The next item can't be prepared now.", Retry: apierror.AfterRefresh},
		{Code: "prepared_expired", Status: 410, Message: "The prepared next item expired.", Retry: apierror.Never},
		{Code: "prepared_canceled", Status: 409, Message: "The prepared next item was canceled.", Retry: apierror.Never},
		{Code: "not_implemented", Status: 501, Message: "This server doesn't offer this yet.", Retry: apierror.Never},
		{Code: "playlist_capacity", Status: 409, Message: "This is more than a playlist can hold.", Retry: apierror.Never},
		{Code: "queue_building", Status: 503, Message: "This part of the queue is still loading. Try again in a moment.", Retry: apierror.SameRequest},
		{Code: "visibility_building", Status: 503, Message: "Library visibility is being prepared. Retry shortly.", Retry: apierror.SameRequest},
		{Code: "device_not_found", Status: 404, Message: "This device isn't available.", Retry: apierror.Never},
		{Code: "not_controllable", Status: 403, Message: "This device can't be controlled from here.", Retry: apierror.Never},
		{Code: "stream_exists", Status: 409, Message: "This device already has an event stream open.", Retry: apierror.Never},
		// Lane C /v1/events (886f5011). C81: its own code; BE-playback's
		// stream_exists never retries, this one reconnects with the last ID.
		{Code: "event_stream_exists", Status: 409, Message: "An existing stream was retired; reconnect using your last event ID.", Retry: apierror.SameRequest},
		{Code: "stream_unavailable", Status: 503, Message: "This connection cannot stream events.", Retry: apierror.SameRequest},
		{Code: "administration_unavailable", Status: 503, Message: "Administration is temporarily unavailable.", Retry: apierror.SameRequest},
		{Code: "password_change_required", Status: 403, Message: "Change your password before continuing.", Retry: apierror.Never},
		{Code: "device_approval_pending", Status: 403, Message: "This device is waiting for approval.", Retry: apierror.Never},
		{Code: "device_denied", Status: 403, Message: "This device is not approved.", Retry: apierror.Never},
		{Code: "no_tuner_available", Status: 409, Message: "Every tuner is in use.", Retry: apierror.AfterRefresh},
		// Channels on v1 sessions (§18.6).
		{Code: "channel_unavailable", Status: 503, Message: "This channel can't be played right now.", Retry: apierror.SameRequest},
		{Code: "seek_unavailable", Status: 409, Message: "That position isn't in the channel's buffer.", Retry: apierror.AfterRefresh},
		// Lane C invitation preview (33129079).
		{Code: "invitation_not_found", Status: 404, Message: "This invitation is not available.", Retry: apierror.Never},
		{Code: "rate_limited", Status: 429, Message: "Too many attempts. Try again later.", Retry: apierror.SameRequest},
		// Plex-model server backups (spec §4): owner-only, published with the
		// API registry.
		{Code: "restore_source_invalid", Status: 400, Message: "This restore source can't be used. Pick one of the listed backups or a Portico database file on the server.", Retry: apierror.Never},
		{Code: "restore_schema_newer", Status: 409, Message: "This backup was written by a newer Portico. Update this server before restoring it.", Retry: apierror.Never},
		{Code: "restore_pre_release", Status: 409, Message: "This backup was created by an earlier pre-release Portico and can't be restored.", Retry: apierror.Never},
		{Code: "restore_integrity_failed", Status: 422, Message: "This backup failed its integrity check and can't be restored.", Retry: apierror.Never},
		{Code: "restore_not_portico", Status: 422, Message: "This file is not a Portico database.", Retry: apierror.Never},
		{Code: "insufficient_disk", Status: 507, Message: "There isn't enough free disk space for this operation.", Retry: apierror.SameRequest},
	}
}

func v1Catalogue() *apierror.Registry {
	catalogue, err := apierror.New(v1ErrorDefinitions()...)
	if err != nil {
		panic(err)
	}
	return catalogue
}

// v1Error maps a domain or service error to a registered API error. Restricted
// content is not_found on every path (SEC-02).
func v1Error(err error) error {
	var public *apierror.Error
	if err == nil || errors.As(err, &public) {
		return err
	}
	var field *playbackv1.FieldError
	var refused playback.ErrDeliveryRefused
	var stale *playbackv1.RevisionError
	var fault *playback.ControlFault
	if errors.As(err, &stale) {
		return &apierror.Error{Code: "revision_mismatch", Current: stale.Current, Cause: err}
	}
	code := ""
	switch {
	case errors.As(err, &field):
		fields := []apierror.Field{}
		if field.Path != "" {
			fields = append(fields, apierror.Field{Path: field.Path, Code: "invalid", Message: "Check this value."})
		}
		return &apierror.Error{Code: "invalid_request", Fields: fields, Cause: err}
	case errors.Is(err, identity.ErrNotVisible):
		code = "not_found"
	case errors.Is(err, identity.ErrForbidden):
		code = "not_permitted"
	case errors.Is(err, identity.ErrUnauthorized):
		code = "unauthorized"
	case errors.Is(err, identity.ErrContentRestricted), errors.Is(err, access.ErrContentRating), errors.Is(err, access.ErrLabelDenied), errors.Is(err, access.ErrChannelDenied),
		errors.Is(err, sql.ErrNoRows), errors.Is(err, playbackv1.ErrNotFound):
		code = "not_found"
	case errors.Is(err, errFeatureRestricted):
		code = "feature_restricted"
	case errors.Is(err, access.ErrStreamLimit), errors.Is(err, access.ErrSchedule), errors.Is(err, playback.ErrOwnerAccountCap), errors.Is(err, playback.ErrOwnerServerCap):
		code = "stream_limit_reached"
	case errors.Is(err, playback.ErrTranscodingDisabled):
		code = "transcode_not_allowed"
	case errors.As(err, &refused), errors.Is(err, playback.ErrIncompatible), errors.Is(err, playback.ErrQualityUnavailable):
		code = "unsupported_media"
	case errors.Is(err, playbackv1.ErrEnded):
		code = "session_ended"
	case errors.Is(err, playbackv1.ErrRevisionRequired):
		code = "revision_required"
	case errors.Is(err, playbackv1.ErrRevision):
		code = "revision_mismatch"
	case errors.Is(err, playbackv1.ErrIdempotencyMismatch):
		code = "idempotency_key_reused"
	case errors.Is(err, playbackv1.ErrKeyRequired):
		code = "idempotency_key_required"
	case errors.Is(err, playbackv1.ErrQueueEnded):
		code = "queue_ended"
	case errors.Is(err, playbackv1.ErrUnsupportedSelector):
		code = "unsupported_selector"
	case errors.Is(err, playbackv1.ErrQueueTooLarge):
		code = "queue_too_large"
	case errors.Is(err, playbackv1.ErrQueueBuilding):
		code = "queue_building"
	case errors.Is(err, catalog.ErrVisibilityBuilding):
		code = "visibility_building"
	case errors.Is(err, catalog.ErrPlaylistCapacity):
		code = "playlist_capacity"
	case errors.Is(err, playbackv1.ErrPrepareNotAllowed):
		code = "prepare_not_allowed"
	case errors.Is(err, playbackv1.ErrPreparedExpired):
		code = "prepared_expired"
	case errors.Is(err, playbackv1.ErrPreparedCanceled):
		code = "prepared_canceled"
	case errors.Is(err, playbackv1.ErrNotImplemented):
		code = "not_implemented"
	case errors.Is(err, playbackv1.ErrNotControllable):
		code = "not_controllable"
	case errors.Is(err, playbackv1.ErrStreamExists):
		code = "stream_exists"
	case errors.Is(err, access.ErrDeviceTrust):
		code = "not_permitted"
	case errors.Is(err, identity.ErrDeviceUnknown):
		code = "device_not_found"
	case errors.Is(err, playbackv1.ErrNoTuner):
		code = "no_tuner_available"
	case errors.Is(err, playbackv1.ErrChannelsUnavailable):
		code = "channel_unavailable"
	case errors.As(err, &fault):
		switch fault.Code {
		case "not_found", "access_revoked":
			code = "not_found" // SEC-02: a channel the viewer can't see isn't there
		case "lease_expired", "playback_ended":
			code = "session_ended"
		case "window_expired", "media_gap", "invalid_seek", "seek_identity_conflict", "media_generation_expired":
			code = "seek_unavailable"
		case "source_capacity_unavailable":
			code = "no_tuner_available"
		default:
			code = "channel_unavailable"
		}
	default:
		return err
	}
	if code == "queue_building" || code == "visibility_building" {
		return &apierror.Error{Code: code, RetryAfterSeconds: 2, Cause: err}
	}
	var transition *playbackv1.TransitionError
	if errors.As(err, &transition) {
		return &apierror.Error{Code: code, Reason: transition.Reason, Cause: err}
	}
	return &apierror.Error{Code: code, Cause: err}
}

// itemAdmission maps an item check made after authentication (SEC-02): a title
// the caller may not reach, for any reason (its library isn't shared with them,
// a restriction, it doesn't exist), is not found. It is never 401, which a
// client would answer by refreshing or signing out (review P1).
func itemAdmission(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, identity.ErrUnauthorized) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, identity.ErrContentRestricted) || errors.Is(err, access.ErrContentRating) || errors.Is(err, access.ErrLabelDenied) || errors.Is(err, playbackv1.ErrNotFound) {
		return &apierror.Error{Code: "not_found", Cause: err}
	}
	return v1Error(err)
}

// v1Authorize enforces a route's access rule and hands the handler the caller.
func (d Dependencies) v1Authorize(ctx context.Context, r *http.Request, m apikit.Metadata) (context.Context, error) {
	// Integration merge (BE-playback registry x lane C's typed routes, C81):
	// one authorizer for every typed route, carrying lane C's rules - a valid
	// non-owner on an owner route is not_permitted, the "direct_account" admin
	// scope is the direct account's own manage right, and sign-in states
	// (password change, device approval) keep their own codes.
	var p identity.Principal
	var err error
	switch {
	case m.Access == apikit.Owner:
		p, err = d.owner(r)
		if errors.Is(err, identity.ErrUnauthorized) {
			if _, valid := d.principal(r); valid == nil {
				return nil, &apierror.Error{Code: "not_permitted"}
			}
		}
	case m.Access == apikit.Admin && m.Scope == "direct_account":
		if p, err = d.principal(r); err == nil {
			if d.Identity == nil {
				return nil, &apierror.Error{Code: "not_permitted"}
			}
			account, e := d.Identity.DirectMe(ctx, directBearer(r))
			if e != nil {
				return nil, v1AuthError(e)
			}
			if !account.CanManage {
				return nil, &apierror.Error{Code: "not_permitted"}
			}
		}
	case m.Access == apikit.Admin:
		p, err = d.administrator(r)
	default:
		p, err = d.principal(r)
	}
	if err != nil {
		return nil, v1AuthError(err)
	}
	// Playback routes act for a device; other typed routes don't need one.
	device, err := d.deviceOf(ctx, p)
	if err != nil && (m.Access == apikit.Device || m.Access == apikit.ViewerItem) {
		return nil, v1Error(err)
	}
	if m.Access == apikit.ViewerItem {
		if item := r.PathValue("itemId"); item != "" {
			if err = itemAdmission(d.itemAccess(ctx, p, item)); err != nil {
				return nil, err
			}
		}
	}
	ctx = context.WithValue(ctx, foundationPrincipalKey{}, p)
	return context.WithValue(ctx, v1CallerKey{}, v1Caller{Principal: p, DeviceID: device}), nil
}

// v1AuthError keeps lane C's sign-in state codes; everything else maps as v1.
func v1AuthError(err error) error {
	if errors.Is(err, identity.ErrPasswordChangeRequired) || errors.Is(err, identity.ErrDevicePending) || errors.Is(err, identity.ErrDeviceDenied) {
		return foundationAuthError(err)
	}
	return v1Error(err)
}

// deviceOf is the identity device a bearer token is bound to. Every issued
// token belongs to a device family (an anonymous device when the client sent no
// installation), so a miss means the token is not a device session.
func (d Dependencies) deviceOf(ctx context.Context, p identity.Principal) (string, error) {
	if d.DB == nil || p.Hash == "" {
		return "", identity.ErrUnauthorized
	}
	var device string
	err := d.DB.QueryRowContext(ctx, `SELECT d.device_id FROM authorization_family_tokens t JOIN identity_device_families d ON d.family_id=t.family_id WHERE t.token_hash=?`, p.Hash).Scan(&device)
	if errors.Is(err, sql.ErrNoRows) {
		return "", identity.ErrUnauthorized
	}
	return device, err
}

// muxPattern is the ServeMux pattern that carries an apikit route. ServeMux
// wildcards must be whole segments, so "{id}:advance" is mounted as "{id}" and
// the registry matches the suffix itself.
func muxPattern(method, path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, "{") {
			if end := strings.IndexByte(part, '}'); end > 0 && end < len(part)-1 {
				parts[i] = part[:end+1]
			}
		}
	}
	return method + " " + strings.Join(parts, "/")
}

// v1SharedPatterns are mux patterns a legacy handler still owns; that handler
// hands v1 requests to the registry (see playback_sessions.go).
var v1SharedPatterns = map[string]bool{
	"POST /v1/playback/sessions":        true,
	"GET /v1/playback/sessions/{id}":    true,
	"DELETE /v1/playback/sessions/{id}": true,
}

// mountRegistry hands every registry route to the registry, except the patterns
// a legacy handler shares (it dispatches v1 requests itself).
func (d Dependencies) mountRegistry(mux *http.ServeMux, registry *apikit.Registry) {
	mounted := map[string]bool{}
	for _, route := range registry.Definitions() {
		pattern := muxPattern(route.Method, route.Path)
		// With no legacy playback service the shared patterns are the registry's.
		if mounted[pattern] || v1SharedPatterns[pattern] && d.Playback != nil {
			continue
		}
		mounted[pattern] = true
		mux.Handle(pattern, registry)
	}
}

// v1OrLegacy serves a pattern shared with a deprecated legacy handler: v1
// requests go to the registry, everything else to the legacy handler, which
// is marked deprecated until clients migrate (strangler).
func (d Dependencies) v1OrLegacy(isV1 func(*http.Request) bool, legacy http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.registry != nil && d.v1 != nil && isV1(r) {
			d.registry.ServeHTTP(w, r)
			return
		}
		markDeprecated(w)
		legacy(w, r)
	}
}

// markDeprecated labels a response from a route Playback Protocol v1 replaces.
func markDeprecated(w http.ResponseWriter) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Link", `</v1/playback/sessions>; rel="successor-version"`)
}

func (d Dependencies) isV1Start(r *http.Request) bool { return r.Header.Get("Idempotency-Key") != "" }
func (d Dependencies) isV1Session(r *http.Request) bool {
	return d.v1.IsV1Session(r.Context(), r.PathValue("id"))
}
