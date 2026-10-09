package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"portico.local/apikit"
	"portico.local/server/internal/access"
	"portico.local/server/internal/administration"
	"portico.local/server/internal/backup"
	"portico.local/server/internal/buildinfo"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/downloads"
	"portico.local/server/internal/eventfeed"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/imagework"
	"portico.local/server/internal/ingestion"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/livechannels/dvr"
	librarychannels "portico.local/server/internal/livechannels/library"
	"portico.local/server/internal/lyrics"
	"portico.local/server/internal/mediaanalysis"
	"portico.local/server/internal/metadata"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/playbackruntime"
	"portico.local/server/internal/playbackv1"
	"portico.local/server/internal/preparedmedia"
	"portico.local/server/internal/recordingaccess"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/remotesources"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
)

type Dependencies struct {
	// HTTPSRoute may attest an externally terminated HTTPS route. The default
	// uses the installed certificate manager; an access URL alone is not proof.
	HTTPSRoute func(context.Context) (string, bool, error)
	// SettingsApplied runs after an owner saves console settings so snapshot
	// readers (delivery configuration) refresh off the request path.
	SettingsApplied func(context.Context)
	// PlaybackV1 is the Playback Protocol v1 service; nil builds one from DB,
	// Playback and Subtitles (tests pass one to shorten leases).
	PlaybackV1 *playbackv1.Service
	// AudioMedia measures audio facts and converts audio exactly for the
	// version 2 render plan (spec §18.1, §18.7); nil: plans need no conversion
	// and missing facts stay missing.
	AudioMedia     AudioMedia
	WebDirectory   string
	Administration *administration.Service
	// Backups is the Plex-model backup service; nil answers backups as
	// unavailable, which is the right default for tests that do not run one.
	Backups      *backup.Service
	Prepared     *preparedmedia.Service
	Console      *operations.Store
	Downloads    *downloads.Service
	Scheduler    *operations.Scheduler
	Measurements *operations.Measurements
	Events       *eventfeed.Hub
	Subtitles    *subtitles.Service
	Lyrics       *lyrics.Service
	LyricsBulk   *lyrics.Bulk
	Analysis     *mediaanalysis.Service
	LyricsProbe  string
	Networking   *networking.ClaimHandler
	// CustomCertificate serves the owner's certificate for a custom domain. Nil
	// omits it from the connectivity report, which is the right default for a
	// test that does not run one.
	CustomCertificate *networking.CustomCertificate
	RouteIdentity     http.Handler
	LiveChannels      *livechannels.Store
	DVR               *dvr.Store
	LibraryChannels   *librarychannels.Store
	PlaybackRuntime   *playbackruntime.Runtime
	Storage           *storage.Client
	Mounts            *mounts.Service
	RemoteSources     *remotesources.Service
	DB                *sql.DB
	Identity          *identity.Service
	Catalog           *catalog.Service
	Ingestion         *ingestion.Service
	Playback          *playback.Service
	Hosted            *hosted.Service
	Metadata          *metadata.Service
	Origins           []string
	TrustedProxies    []netip.Prefix
	// Access carries the people, diagnostics and connectivity seams (workstream G2).
	Access AccessArea
	// Watchdog reports database health. Nil leaves the server reporting healthy,
	// which is the right default for a test that does not run one.
	Watchdog *dbwork.Watchdog
	// admission is installed by New; it is not a caller-supplied dependency.
	admission *admission
	// directSchema caches the one-off probe for the direct identity tables. It is
	// installed by New for the same reason: it is a fact about this database, not
	// a choice a caller makes.
	directSchema *recordingaccess.Schema
	// principals is the bounded, authority-fenced principal cache. See
	// principal_cache.go for the discipline it keeps.
	principals *principalCache
	// access is the library-authority cache, kept on the same terms.
	access *accessCache
	// restrictions is the viewer content-restriction cache, kept on the same terms.
	restrictions *restrictionCache
	securePolicy *securePolicyCache
	// v1 is the Playback Protocol v1 resource service (playback_v1*.go),
	// installed by New like the caches above.
	v1 *playbackv1.Service
	// registry is the typed apikit registry (foundation and playback v1).
	registry *apikit.Registry
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// routeMux is the live router's mux. RoutePattern asks it for the registered
// pattern of a request seen outside the mux (where r.Pattern is empty).
var (
	routeMuxMu sync.RWMutex
	routeMux   *http.ServeMux
)

// RoutePattern returns the registered route pattern for r (for example
// "GET /v1/items/{id}"), or "" when no route matches. The pattern never
// carries identifiers, tokens or query strings; only the path is matched.
func RoutePattern(r *http.Request) string {
	if r == nil {
		return ""
	}
	routeMuxMu.RLock()
	mux := routeMux
	routeMuxMu.RUnlock()
	if mux == nil {
		return ""
	}
	_, pattern := mux.Handler(r)
	return pattern
}

// httpFailureWindow is one rate-limit bucket for failure()'s 5xx log: the
// first of each (status, code) logs at once with its cause, the rest collapse
// to at most one line per minute with the suppressed count.
type httpFailureWindow struct {
	started    time.Time
	suppressed int
}

var (
	httpFailureMu      sync.Mutex
	httpFailureWindows = map[string]*httpFailureWindow{}
)

const (
	httpFailureWindowLength = time.Minute
	httpFailureClassCap     = 128
)

// boundFailureDetail keeps the logged cause to one bounded line: no controls,
// at most 160 characters. Bodies, tokens and query strings never reach here:
// only the error value is formatted, never the request.
func boundFailureDetail(err error) string {
	v := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, err.Error())
	if len(v) > 160 {
		v = v[:160] + "…"
	}
	return v
}

func reportHTTPFailure(status int, code string, err error) {
	if err == nil {
		return
	}
	key := strconv.Itoa(status) + "\x00" + code
	httpFailureMu.Lock()
	now := time.Now()
	current, ok := httpFailureWindows[key]
	if ok && now.Sub(current.started) < httpFailureWindowLength {
		current.suppressed++
		httpFailureMu.Unlock()
		return
	}
	suppressed := 0
	if ok {
		suppressed = current.suppressed
		current.started, current.suppressed = now, 0
	} else {
		if len(httpFailureWindows) >= httpFailureClassCap {
			for k, v := range httpFailureWindows {
				if now.Sub(v.started) >= httpFailureWindowLength {
					delete(httpFailureWindows, k)
				}
			}
			if len(httpFailureWindows) >= httpFailureClassCap {
				clear(httpFailureWindows)
			}
		}
		httpFailureWindows[key] = &httpFailureWindow{started: now}
	}
	httpFailureMu.Unlock()
	if suppressed > 0 {
		log.Printf("request failed (%d %s): %s; %d more like this since the last report", status, code, boundFailureDetail(err), suppressed)
		return
	}
	log.Printf("request failed (%d %s): %s", status, code, boundFailureDetail(err))
}
func failure(w http.ResponseWriter, e error) {
	if revisionFailure(w, e) {
		return
	}
	if errors.Is(e, catalog.ErrVisibilityBuilding) {
		w.Header().Set("Retry-After", "1")
		write(w, http.StatusServiceUnavailable, map[string]any{"error": map[string]any{"code": "visibility_building", "message": "Library visibility is being prepared. Retry shortly.", "retryable": true}})
		return
	}
	if errors.Is(e, preparedmedia.ErrInput) || errors.Is(e, preparedmedia.ErrConflict) || errors.Is(e, preparedmedia.ErrUnavailable) || errors.Is(e, preparedmedia.ErrSourceChanged) {
		preparedFailure(w, e)
		return
	}
	status, code := 400, "invalid_request"
	if errors.Is(e, errFeatureRestricted) {
		status, code = 403, "feature_restricted"
	}
	if errors.Is(e, identity.ErrPasswordChangeRequired) {
		status, code = 403, "password_change_required"
	}
	if errors.Is(e, identity.ErrCurrentPasswordIncorrect) {
		status, code = 403, "current_password_incorrect"
	}
	if errors.Is(e, identity.ErrAccountLocked) {
		status, code = 429, "account_locked"
		var locked *identity.AccountLockError
		if errors.As(e, &locked) {
			w.Header().Set("Retry-After", strconv.FormatInt(locked.RetryAfter, 10))
		} else {
			w.Header().Set("Retry-After", "1")
		}
	}
	if errors.Is(e, identity.ErrProfileSelection) {
		status, code = 409, "profile_selection_required"
	}
	if errors.Is(e, identity.ErrProfilePIN) {
		status, code = 403, "profile_pin_invalid"
	}
	if errors.Is(e, identity.ErrProfileLocked) {
		status, code = 429, "profile_pin_locked"
		w.Header().Set("Retry-After", "60")
	}
	if errors.Is(e, identity.ErrProfileCapacity) {
		status, code = 409, "profile_capacity"
	}
	if errors.Is(e, identity.ErrProfileChanged) {
		status, code = 409, "profile_changed"
	}
	if errors.Is(e, identity.ErrPrimaryProfile) {
		status, code = 409, "primary_profile"
	}
	// Workstream H: profiles, restrictions, devices, two-factor, registration.
	if errors.Is(e, identity.ErrRestrictionInput) || errors.Is(e, identity.ErrDeviceInput) || errors.Is(e, identity.ErrBrowserAccountInput) {
		status, code = 400, "invalid_request"
	}
	if errors.Is(e, identity.ErrRestrictionChanged) {
		status, code = 409, "restrictions_changed"
	}
	if errors.Is(e, identity.ErrContentRestricted) || errors.Is(e, access.ErrContentRating) || errors.Is(e, access.ErrLabelDenied) {
		status, code = 404, "not_found"
	}
	if errors.Is(e, identity.ErrAvatarUpload) {
		status, code = 415, "avatar_unsupported"
	}
	if errors.Is(e, identity.ErrAvatarMissing) {
		status, code = 404, "avatar_not_found"
	}
	if errors.Is(e, identity.ErrFactorRequired) {
		status, code = 401, "two_factor_required"
	}
	if errors.Is(e, identity.ErrFactorInvalid) {
		status, code = 403, "two_factor_invalid"
	}
	if errors.Is(e, identity.ErrFactorEnrolled) {
		status, code = 409, "two_factor_enrolled"
	}
	if errors.Is(e, identity.ErrFactorMissing) {
		status, code = 409, "two_factor_missing"
	}
	if errors.Is(e, identity.ErrDeviceUnknown) {
		status, code = 404, "device_not_found"
	}
	if errors.Is(e, identity.ErrDevicePending) {
		status, code = 403, "device_approval_pending"
	}
	if errors.Is(e, identity.ErrDeviceDenied) {
		status, code = 403, "device_denied"
	}
	if errors.Is(e, identity.ErrRegistrationClosed) {
		status, code = 403, "registration_closed"
	}
	if errors.Is(e, identity.ErrRegistrationInvite) {
		status, code = 403, "registration_invitation_required"
	}
	if errors.Is(e, identity.ErrRegistrationTaken) {
		status, code = 409, "username_taken"
	}
	if errors.Is(e, identity.ErrTopShelfToken) {
		status, code = 401, "topshelf_token_expired"
	}
	if errors.Is(e, catalog.ErrListeningResumeUnavailable) {
		status, code = 409, "listening_resume_unavailable"
	}
	if errors.Is(e, catalog.ErrListeningPreferencesConflict) {
		status, code = 409, "listening_preferences_conflict"
	}
	if errors.Is(e, catalog.ErrLibraryConfigurationConflict) {
		status, code = 409, "library_configuration_conflict"
	}
	if errors.Is(e, catalog.ErrShowSettingsConflict) {
		status, code = 409, "show_settings_conflict"
	}
	if errors.Is(e, catalog.ErrSourceBusy) || errors.Is(e, storage.ErrBusy) {
		status, code = 409, "source_busy"
	}
	if errors.Is(e, catalog.ErrSourceChanged) || errors.Is(e, storage.ErrRemoteChanged) || errors.Is(e, storage.ErrInventoryChanged) {
		status, code = 409, "source_changed"
	}
	if errors.Is(e, playback.ErrStaleChapter) {
		status, code = 409, "stale_chapter_projection"
	}
	if errors.Is(e, playback.ErrChapterCursor) {
		status, code = 400, "invalid_chapter_cursor"
	}
	if errors.Is(e, playback.ErrStaleOffer) {
		status, code = 409, "stale_playback_offer"
	}
	// CD-51: 401 only for a failed credential; a refusal of a valid session is
	// 403, or 404 where existence stays hidden.
	if refusalStatus, refusalCode, ok := identity.Refusal(e); ok {
		status, code = refusalStatus, refusalCode
		if status == 401 {
			code = "unauthorized"
		}
	}
	// A proven Portico Account that is not (or no longer) a member here: the
	// client drops the server or returns to sign-in (Spec — Hosted at Scale).
	if errors.Is(e, identity.ErrAccessRefused) {
		status, code = 403, "access_refused"
	}
	// A session migration 0120 ended: re-admit silently with an identity
	// assertion; this is not a sign-out.
	if errors.Is(e, identity.ErrSessionMigrated) {
		status, code = 401, "session_migrated"
	}
	if errors.Is(e, hosted.ErrClockSkew) {
		status, code = 401, "clock_skew"
	}
	if errors.Is(e, sql.ErrNoRows) {
		status, code = 404, "not_found"
	}
	if errors.Is(e, playback.ErrGrantEnded) {
		status, code = 404, "presentation_ended"
	}
	if errors.Is(e, metadata.ErrMBConflict) {
		status, code = 409, "metadata_conflict"
	}
	if errors.Is(e, metadata.ErrScreenConflict) || errors.Is(e, metadata.ErrTVDBConflict) {
		status, code = 409, "metadata_conflict"
	}
	if errors.Is(e, catalog.ErrCollectionConflict) {
		status, code = 409, "collection_conflict"
	}
	if errors.Is(e, catalog.ErrSourceAlreadyConfigured) {
		status, code = 409, "source_already_configured"
	}
	if errors.Is(e, identity.ErrConflict) {
		status, code = 409, "conflict"
	}
	if errors.Is(e, playback.ErrIncompatible) {
		status, code = 422, "unsupported_source"
	}
	if errors.Is(e, playback.ErrOwnerAccountCap) {
		status, code = 409, "owner_account_cap"
	}
	if errors.Is(e, playback.ErrOwnerServerCap) {
		status, code = 409, "owner_server_cap"
	}
	if errors.Is(e, playback.ErrTranscodingDisabled) {
		status, code = 422, "transcoding_disabled"
	}
	if errors.Is(e, playback.ErrClientProfile) || errors.Is(e, playback.ErrRouteFailure) {
		status, code = 400, "invalid_playback_report"
	}
	if errors.Is(e, playback.ErrClientProfileVersion) {
		status, code = 422, "unsupported_profile_version"
	}
	var refused playback.ErrDeliveryRefused
	if errors.As(e, &refused) {
		// A policy refusal is an answer, not a fault: the code names the
		// preference or clamp that closed every route.
		status, code = 422, refused.Code
	}
	message := "The request could not be completed."
	if errors.Is(e, playback.ErrGrantEnded) {
		message = playback.ErrGrantEnded.Error()
	}
	if status == 401 {
		message = publicErrorMessage("unauthorized")
	} else if status == 403 && errors.Is(e, identity.ErrForbidden) {
		message = identity.ErrForbidden.Error()
	}
	retryable := false
	if errors.Is(e, playback.ErrSegmentPreparing) {
		status, code, retryable = 503, "segment_preparing", true
		w.Header().Set("Retry-After", "1")
	}
	if errors.Is(e, playback.ErrAudioRenditionUnavailable) {
		status, code, retryable = 422, "audio_rendition_unavailable", false
	}
	if errors.Is(e, playback.ErrFiniteUnsupported) {
		status, code, retryable = 422, "unsupported_timing", false
	}
	if errors.Is(e, playback.ErrConversionCapacity) {
		status, code, retryable = 429, "conversion_capacity", true
		w.Header().Set("Retry-After", "2")
	}
	// A volume with no room left is beyond-target and recoverable: the eviction
	// sweep may free space within seconds, so it answers retryable rather than
	// presenting as a failure of the title.
	if errors.Is(e, playback.ErrConversionSpace) {
		status, code, retryable = 503, "conversion_storage_full", true
		w.Header().Set("Retry-After", "10")
	}
	if errors.Is(e, identity.ErrBusy) {
		status, code, retryable = 503, "authentication_busy", true
		w.Header().Set("Retry-After", "2")
	}
	if errors.Is(e, catalog.ErrPlaylistConflict) {
		status, code = 409, "playlist_conflict"
	}
	if errors.Is(e, metadata.ErrLocalMetadataOnly) {
		status, code, message = 409, "local_metadata_only", metadata.ErrLocalMetadataOnly.Error()
	}
	if errors.Is(e, metadata.ErrLibraryAgentConflict) {
		status, code, message = 409, "metadata_conflict", metadata.ErrLibraryAgentConflict.Error()
	}
	if errors.Is(e, metadata.ErrLibraryAgentInput) {
		status, code = 400, "invalid_metadata_source"
	}
	if errors.Is(e, metadata.ErrLibraryLanguageInput) {
		status, code = 400, "invalid_metadata_language"
	}
	if errors.Is(e, catalog.ErrPlaylistCapacity) {
		status, code = 409, "playlist_capacity"
	}
	if errors.Is(e, catalog.ErrPersonalCapacity) {
		status, code, retryable = 429, "personal_capacity", true
	}
	if errors.Is(e, catalog.ErrPersonalConflict) {
		status, code = 409, "personal_state_conflict"
	}
	if errors.Is(e, catalog.ErrOperationExpired) {
		status, code = 409, "operation_expired"
	}
	if errors.Is(e, catalog.ErrPersonalReview) {
		status, code = 409, "personal_needs_review"
	}
	if errors.Is(e, catalog.ErrPersonalResolution) {
		status, code = 409, "personal_needs_resolution"
	}
	if errors.Is(e, catalog.ErrOperationConflict) {
		status, code = 409, "idempotency_key_reused"
	}
	if errors.Is(e, catalog.ErrStaleContinuation) {
		status, code, retryable = 409, "stale_continuation", true
	}
	if errors.Is(e, catalog.ErrCursor) {
		status, code = 400, "invalid_cursor"
	}
	var pathError *os.PathError
	var databaseError interface{ Code() int }
	if errors.As(e, &pathError) {
		status, code, message, retryable = 503, "source_unavailable", "Storage is unavailable. Check the source and retry.", true
	} else if errors.As(e, &databaseError) {
		status, code, message = 500, "persistence_error", "The request could not be saved."
	}
	// A full volume is the one persistence failure with an owner-visible cause
	// and an owner-visible remedy. "The request could not be saved" tells them
	// nothing; this tells them the disk is full.
	if dbwork.Full(e) || errors.Is(e, dbwork.ErrDatabaseFull) {
		status, code, message, retryable = 507, "storage_full", "The server has run out of disk space. Free space on the server's volume and retry.", true
		w.Header().Set("Retry-After", "30")
	}
	if errors.Is(e, imagework.ErrBusy) {
		status, code, message, retryable = 503, "server_busy", "Image processing is busy. Retry shortly.", true
		w.Header().Set("Retry-After", "1")
	}
	if errors.Is(e, context.DeadlineExceeded) {
		status, code, message, retryable = 503, "timeout", "The operation timed out. Retry shortly.", true
	}
	if errors.Is(e, mounts.ErrUnavailable) {
		status, code, message, retryable = 503, "mount_unavailable", mounts.ErrUnavailable.Error(), true
	}
	if errors.Is(e, remotemedia.ErrPolicy) {
		status, code, message = 403, "network_policy", remotemedia.ErrPolicy.Error()
	}
	if errors.Is(e, storage.ErrBusy) || errors.Is(e, storage.ErrPlaybackTimeout) || errors.Is(e, storage.ErrPlaybackSource) {
		status, code, message, retryable = 503, "playback_source_unavailable", "Playback source is unavailable; retry when storage responds.", true
		w.Header().Set("Retry-After", "2")
		if errors.Is(e, storage.ErrBusy) {
			code = "playback_source_busy"
		}
		if errors.Is(e, storage.ErrPlaybackTimeout) {
			code = "playback_source_timeout"
		}
	}
	if errors.Is(e, remotemedia.ErrBusy) {
		status, code, message, retryable = 503, "remote_busy", remotemedia.ErrBusy.Error(), true
	}
	if errors.Is(e, remotemedia.ErrUnavailable) {
		status, code, message, retryable = 502, "remote_unavailable", remotemedia.ErrUnavailable.Error(), true
	}
	if errors.Is(e, remotemedia.ErrStalled) {
		status, code, message, retryable = 504, "remote_stalled", remotemedia.ErrStalled.Error(), true
	}
	if errors.Is(e, remotemedia.ErrDenied) {
		status, code, message = 502, "remote_denied", remotemedia.ErrDenied.Error()
	}
	if errors.Is(e, remotemedia.ErrUnsupported) {
		status, code, message = 422, "unsupported_source", remotemedia.ErrUnsupported.Error()
	}
	if status >= 500 {
		// Server-side failures keep their cause in the log; the client sees only the code.
		reportHTTPFailure(status, code, e)
	}
	write(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "retryable": retryable}})
}

// decode reads one JSON object into v. On a PUT, PATCH or DELETE a body type
// with an expectedRevision field is a revision-fenced change of something that
// exists: absence (or null) is 428 revision_required, never a silent revision
// 0 (BE-API-09); an explicit 0 is kept where 0 is a valid revision. A POST
// shares body types with creations, so a POST that changes an existing
// resource opts in with decodeLegacyRevision.
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		return errors.New("invalid JSON request")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(v); e != nil {
		return errors.New("invalid JSON request")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("one JSON object required")
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch, http.MethodDelete:
		return requireRevisionJSON(raw, v)
	}
	return nil
}

// principal resolves the Authorization header to an authenticated, library-
// authorised viewer.
//
// The three checks it runs — the session family, the recovery route, and
// library authority — used to open three read transactions and therefore take
// three pooled connections out of eight, before the handler had done any work.
// They now share one read snapshot, which is both cheaper and stricter: all
// three see one state rather than three consecutive ones. The snapshot is bound
// by the request context, so the lane's budget reaches authentication instead of
// stopping at the handler door.
func (d Dependencies) principal(r *http.Request) (identity.Principal, error) {
	p, err := d.resolvePrincipal(r)
	if err == nil {
		err = d.featureAllowed(r.Context(), p, requestFeature(r))
	}
	return p, err
}
func (d Dependencies) resolvePrincipal(r *http.Request) (identity.Principal, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return identity.Principal{}, identity.ErrUnauthorized
	}
	token := strings.TrimPrefix(header, "Bearer ")
	private := d.privateSetupPeer(r)
	// The cache is keyed on the token's digest, never on the token, and it is
	// consulted only on lanes where a few seconds of lag in a revocation is
	// acceptable. Everything that touches authority or playback resolves live.
	key := ""
	strict := strictPrincipal(dbwork.ClassFrom(r.Context(), dbwork.ClassInteractive))
	if !strict && !private {
		key = identity.Digest(token)
		if cached, ok := d.principals.lookup(key, time.Now()); ok {
			d.admission.rememberCredential(token, cached)
			return cached, nil
		}
	}
	generation := dbwork.AuthorityGeneration()
	var p identity.Principal
	resolve := func(ctx context.Context) error {
		var e error
		if p, e = d.Identity.AuthenticateContext(ctx, token); e != nil {
			return e
		}
		if e = d.Identity.CheckRecoveryRoute(ctx, p, private); e != nil {
			return e
		}
		return d.allowedLibrary(ctx, p, "")
	}
	var err error
	if d.DB == nil {
		err = resolve(r.Context())
	} else {
		err = dbwork.WithReadSnapshot(r.Context(), d.DB, resolve)
	}
	// A denial is never cached: the reason may have been transient, and a cached
	// refusal is the one mistake here a viewer would actually see.
	if err == nil && key != "" {
		d.principals.store(key, p, time.Now(), generation)
	}
	if err == nil {
		d.admission.rememberCredential(token, p)
	}
	return p, err
}

// strictPrincipalFor resolves a principal against live state whatever lane the
// request is on. A long-lived transport calls it before it parks, so a durable
// revocation cannot leave a stream authorised until the cache entry expires.
func (d Dependencies) strictPrincipalFor(r *http.Request) (identity.Principal, error) {
	return d.principal(r.WithContext(dbwork.WithClass(r.Context(), dbwork.ClassSecurityFence)))
}
func (d Dependencies) owner(r *http.Request) (identity.Principal, error) {
	return d.ownerContext(r.Context(), r)
}
func (d Dependencies) itemAccess(ctx context.Context, p identity.Principal, id string) error {
	lib, e := d.Catalog.WithContext(ctx).LibraryForItem(id)
	if e != nil {
		return e
	}
	if err := d.allowedLibrary(ctx, p, lib); err != nil {
		return err
	}
	return d.restrictedItem((&http.Request{}).WithContext(ctx), p, id)
}
func New(d Dependencies) http.Handler {
	d.configurePlaybackEvidence()
	if d.Storage == nil {
		if binary, e := os.Executable(); e == nil {
			d.Storage = storage.New(binary)
		}
	}
	mux := http.NewServeMux()
	for _, add := range devRoutes {
		add(d, mux)
	}
	// Admission is created with the router and lives as long as it. Its lanes
	// publish this server's foreground-pressure signal, which is what background
	// loops yield to.
	d.directSchema = recordingaccess.NewSchema()
	d.principals = newPrincipalCache()
	d.access = newAccessCache()
	d.restrictions = newRestrictionCache()
	d.securePolicy = &securePolicyCache{}
	// The typed registry captures d, so everything its handlers read must be
	// installed first.
	if d.Console == nil && d.DB != nil {
		d.Console = operations.New(d.DB)
	}
	if d.Scheduler != nil && d.Catalog != nil {
		found := false
		for _, kind := range d.Scheduler.Kinds() {
			if kind.Kind == "personal-state" {
				found = true
			}
		}
		if !found {
			if err := d.Scheduler.Register(d.Catalog.BulkAdapter(d.bulkAccess)); err != nil {
				panic(err)
			}
			if err := d.Scheduler.Register(d.Catalog.BulkTrashAdapter(d.bulkAccess)); err != nil {
				panic(err)
			}
			if err := d.Scheduler.Register(d.Catalog.ContainerResetAdapter()); err != nil {
				panic(err)
			}
		}
	}
	gatekeeper := newAdmission()
	gatekeeper.trustedProxies = d.TrustedProxies
	gatekeeper.registerPressure()
	d.admission = gatekeeper
	// Before the registry: lane C's /v1/events handler reads d.Events (merge).
	if d.Events == nil && d.DB != nil {
		d.Events = eventfeed.New(d.DB)
	}
	if d.Events != nil && d.Hosted != nil && d.Events.LibraryVisible == nil {
		d.Events.LibraryVisible = func(tx *sql.Tx, p identity.Principal, library string) bool {
			return d.Hosted.AllowedTx(p, library, tx) == nil
		}
	}
	// Before the registry, which captures d (lane C's typed admin routes).
	if d.Administration == nil && d.DB != nil {
		d.Administration = administration.New(d.DB)
	}
	if d.Catalog != nil {
		d.Catalog.BulkCommands = d.bulkCommandHooks()
	}
	if d.Downloads != nil {
		d.Downloads.SetRequestAccess(d.downloadRequestAccess)
	}
	d.v1 = newPlaybackV1(d)
	if registry, err := FoundationRegistry(d); err != nil {
		panic(err)
	} else {
		d.registry = registry
	}
	d.installationRoutes(mux)
	d.foundationRoutes(mux)
	d.consoleRoutes(mux)
	if d.RouteIdentity != nil {
		mux.Handle("POST /v1/networking/identity-proof", d.RouteIdentity)
	}
	d.directIdentityRoutes(mux)
	d.identityRoutes(mux)
	if d.Networking != nil {
		d.Networking.SetManagementAuthorizer(func(r *http.Request) (networking.ClaimRequestGuard, error) {
			p, err := d.owner(r)
			if err != nil {
				return nil, err
			}
			return func(ctx context.Context, tx *sql.Tx) error { return d.ownerAuthorityTx(ctx, tx, p) }, nil
		})
		d.Networking.Register(mux)
	} else {
		d.unconfiguredNetworkingRoutes(mux)
	}
	d.liveChannelRoutes(mux, d.LiveChannels)
	d.guideImageRoutes(mux)
	d.dvrRoutes(mux)
	d.libraryChannelRoutes(mux)
	authRate := &limiter{trustedProxies: d.TrustedProxies, proxies: func(r *http.Request) []netip.Prefix { return d.trustedProxyPrefixes(r.Context()) }}
	d.linearMediaRoutes(mux)
	d.mountRoutes(mux)
	d.remoteSourceRoutes(mux)
	d.episodeRoutes(mux)
	d.playbackChapterRoutes(mux)
	d.playbackOfferRoutes(mux)
	d.preparedMediaRoutes(mux)
	d.audioRoutes(mux)
	d.subtitleRoutes(mux)
	d.lyricRoutes(mux)
	d.analysisRoutes(mux)
	d.trickplayRoutes(mux)
	d.searchRoutes(mux)
	d.peopleRoutes(mux)
	d.onboardingRoutes(mux)
	d.administrationRoutes(mux)
	d.adminRoutes(mux)
	d.accessAreaRoutes(mux)
	d.playHistoryRoutes(mux)
	d.inventoryRoutes(mux)
	d.manualMetadataRoutes(mux)
	d.metadataRepairRoutes(mux)
	d.metadataEditorRoutes(mux)
	d.supportRoutes(mux)
	d.operationsRoutes(mux)
	d.playbackDiagnosticsRoutes(mux)
	d.concurrencyRoutes(mux)
	d.pprofRoutes(mux)
	d.telemetryRoutes(mux)
	d.deliveryRoutes(mux)
	d.activityRoutes(mux)
	d.discoveryRoutes(mux)
	d.contentHomeRoutes(mux)
	d.browseRoutes(mux)
	d.notificationRoutes(mux)
	d.downloadRoutes(mux)
	d.socialRoutes(mux)
	mux.HandleFunc("GET /v1/attribution", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"providers": []map[string]string{{"name": "TMDB", "notice": metadata.Attribution, "url": "https://www.themoviedb.org"}, {"name": "TheTVDB", "notice": "Metadata provided by TheTVDB.", "url": "https://thetvdb.com"}, {"name": "MusicBrainz", "notice": "Music metadata provided by MusicBrainz.", "url": "https://musicbrainz.org"}, {"name": "Cover Art Archive", "notice": "Cover art provided by Cover Art Archive / MusicBrainz and Internet Archive. Images copyright their respective owners.", "url": "https://coverartarchive.org"}}})
	})
	// hostedConfigured says this build can reach the Portico Account service at all (every release
	// build can); hostedAttached says this server actually signs people in with Portico Accounts
	// (it holds Hosted policy). A sign-in page decides by the second: a server set up for direct
	// sign-in must never be shown the Portico Account hand-off.
	mux.HandleFunc("GET /v1/system", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"id": d.Identity.ID(), "name": d.Identity.Name(), "setupRequired": d.Identity.SetupRequired(), "hostedConfigured": d.Hosted.Configured(), "hostedAttached": d.Identity.Hosted(), "version": buildinfo.Info()["version"], "buildId": buildinfo.Info()["buildId"], "sourceDigest": buildinfo.Info()["sourceDigest"], "builtAt": buildinfo.Info()["builtAt"]})
	})
	mux.HandleFunc("POST /v1/setup", func(w http.ResponseWriter, r *http.Request) {
		var body identity.SetupOptions
		if e := decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		if e := d.Identity.SetupLimit(r.Context(), setupPeerSubject(r), "setup", 10); e != nil {
			setupFailure(w, e)
			return
		}
		result, e := d.Identity.SetupWithOptions(r.Context(), body, d.privateSetupPeer(r))
		if e != nil {
			failure(w, e)
			return
		}
		writeAuth(w, r, 201, result)
	})
	mux.HandleFunc("POST /v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if e := decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		if e := d.Identity.SetupLimit(r.Context(), setupPeerSubject(r), "login", 20); e != nil {
			setupFailure(w, e)
			return
		}
		result, e := d.Identity.LoginFrom(r.Context(), body.Username, body.Password, d.privateSetupPeer(r))
		if e != nil {
			failure(w, e)
			return
		}
		// Remote sign-in policy and per-member session limits. See
		// access_admission.go; a denied session is revoked before it is returned.
		if e = d.admitRemoteSignIn(r, result); e != nil {
			accessFailure(w, e)
			return
		}
		if e = d.admitSession(r, result); e != nil {
			accessFailure(w, e)
			return
		}
		writeAuth(w, r, 200, result)
	})
	mux.HandleFunc("GET /v1/me", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, map[string]any{"viewer": p.Viewer})
	})
	mux.HandleFunc("DELETE /v1/sessions/current", d.sessionFamilyLogout)
	mux.HandleFunc("POST /v1/hosted/wake", func(w http.ResponseWriter, r *http.Request) {
		var body hosted.Signed
		if e := decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		if e := d.Hosted.Wake(r.Context(), body); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(202)
	})
	// Membership is this server's own (migration 0120): Hosted tickets, policy
	// restrictions and offline profile proofs are gone. A Portico Account signs
	// in with an identity assertion at POST /v1/direct/sign-in.
	for _, route := range []string{"POST /v1/hosted/attach", "POST /v1/hosted/restrictions", "POST /v1/hosted/profiles/offline-select"} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			write(w, 410, map[string]any{"error": map[string]any{"code": "moved", "message": "Sign in with the Portico Account at /v1/direct/sign-in.", "retryable": false}})
		})
	}
	mux.HandleFunc("GET /v1/libraries", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		visible, e := d.visibleLibraries(r, p)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, map[string]any{"items": visible})
	})
	mux.HandleFunc("POST /v1/libraries", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
			Path string `json:"path"`
			// MetadataAgent is the library's metadata source: "online" (the
			// default when omitted) or "local" for local metadata only.
			MetadataAgent string `json:"metadataAgent,omitempty"`
			// MetadataLanguage is one of the chosen agent's Languages for
			// Kind (see GET /v1/library-kinds/{kind}/metadata-agents).
			// Omitted changes nothing.
			MetadataLanguage string `json:"metadataLanguage,omitempty"`
		}
		if e := decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		// The language is validated before creating anything: a rejected
		// language creates no library and queues no scan. An unknown agent
		// still reports invalid_metadata_source from the creation
		// transaction, so only pre-check languages for a known agent.
		if body.MetadataLanguage != "" && metadata.ValidLibraryKind(body.Kind) {
			effective := body.MetadataAgent
			if effective == "" {
				effective = metadata.AgentOnline
			}
			if (effective == metadata.AgentOnline || effective == metadata.AgentLocal) && !metadata.LibraryLanguageOffered(body.Kind, effective, body.MetadataLanguage) {
				failure(w, metadata.ErrLibraryLanguageInput)
				return
			}
		}
		// The first scan is queued in the same transaction (B71): the dialog
		// promises titles right away, and a library must never sit configured
		// with nothing queued until someone presses Scan now. The scan itself
		// runs in the background lane; queueing is idempotent per source.
		queue := func(tx *sql.Tx, l catalog.Library) error {
			// The source is chosen before the first scan is queued, so a
			// local-only library never makes a single online request.
			if body.MetadataAgent != "" && body.MetadataAgent != metadata.AgentOnline {
				if err := metadata.ApplyLibraryAgentTx(r.Context(), tx, l.ID, body.MetadataAgent); err != nil {
					return err
				}
			}
			if body.MetadataLanguage != "" {
				if err := metadata.ApplyLibraryLanguageTx(r.Context(), tx, l.ID, body.MetadataLanguage); err != nil {
					return err
				}
			}
			if d.Ingestion == nil {
				return nil
			}
			_, err := d.Ingestion.QueueTx(r.Context(), tx, l.ID)
			return err
		}
		l, e := d.Catalog.CreateAuthorizedThen(r.Context(), body.Name, body.Kind, body.Path, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") }, queue)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 201, l)
	})
	mux.HandleFunc("POST /v1/libraries/{id}/network-roots", func(w http.ResponseWriter, r *http.Request) {
		p, _, e := d.storageOwner(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			Roots            []string `json:"roots"`
			ExpectedRevision *int64   `json:"expectedRevision,omitempty"`
		}
		if e := decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		approvals, e := d.Playback.ApproveNetworkRootsAuthorized(r.Context(), r.PathValue("id"), body.Roots, body.ExpectedRevision, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, r.PathValue("id")) })
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, map[string]any{"approvals": approvals})
	})
	mux.HandleFunc("POST /v1/libraries/{id}/scans", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		j, e := d.Ingestion.QueueMode(r.Context(), r.PathValue("id"), "", r.URL.Query().Get("mode"), func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 202, j)
	})
	mux.HandleFunc("GET /v1/admin/ingestion/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.owner(r); e != nil {
			failure(w, e)
			return
		}
		j, e := d.Ingestion.Get(r.PathValue("id"))
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, j)
	})
	mux.HandleFunc("DELETE /v1/admin/ingestion/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		if e := d.Ingestion.Control(r.Context(), r.PathValue("id"), "cancel", func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") }); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /v1/items", func(w http.ResponseWriter, r *http.Request) {
		p, libraries, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		lib := r.URL.Query().Get("libraryId")
		if lib != "" {
			if e = d.allowedLibrary(r.Context(), p, lib); e != nil {
				failure(w, e)
				return
			}
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		viewer, e := d.catalogViewer(r, p, libraries, fence)
		if e != nil {
			failure(w, e)
			return
		}
		items, next, e := d.Catalog.WithContext(r.Context()).List(viewer, lib, r.URL.Query().Get("cursor"), limit)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, map[string]any{"items": items, "nextCursor": next})
	})
	mux.HandleFunc("GET /v1/items/{id}/art/{kind}", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e == nil {
			e = d.itemAccess(r.Context(), p, r.PathValue("id"))
		}
		if e == nil {
			e = d.restrictedItem(r, p, r.PathValue("id"))
		}
		if e != nil {
			failure(w, e)
			return
		}
		if d.Metadata == nil {
			failure(w, errors.New("artwork unavailable"))
			return
		}
		f, mime, e := d.Metadata.EntityArtworkVariant(r.Context(), metadata.RepairTarget{Kind: "item", ID: r.PathValue("id")}, r.PathValue("kind"), "", artworkWidth(r), r.URL.Query().Get("v"))
		if e != nil {
			if artworkVersionFailure(w, e) {
				return
			}
			if errors.Is(e, metadata.ErrArtworkPending) {
				w.Header().Set("Retry-After", "5")
				w.Header().Set("Cache-Control", "no-store")
				policyError(w, "artwork_pending")
				return
			}
			failure(w, e)
			return
		}
		defer f.Close()
		serveArtwork(w, r, f, mime)
	})
	mux.HandleFunc("GET /v1/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e == nil {
			e = d.itemAccess(r.Context(), p, r.PathValue("id"))
		}
		if e == nil {
			e = d.restrictedItem(r, p, r.PathValue("id"))
		}
		if e != nil {
			failure(w, e)
			return
		}
		item, e := d.Catalog.WithContext(r.Context()).Get(identity.PersonalKey(p.Viewer), r.PathValue("id"))
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, item)
	})
	mux.HandleFunc("DELETE /v1/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.owner(r); e != nil {
			failure(w, e)
			return
		}
		if e := d.Catalog.Delete(r.PathValue("id")); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	})
	d.playbackSessionRoutes(mux)
	d.audioDecodeRoutes(mux)
	mux.HandleFunc("GET /v1/media/{grant}/{file}", func(w http.ResponseWriter, r *http.Request) {
		grant := r.PathValue("grant")
		_, p, item, e := d.Playback.ResolveGrantContext(r.Context(), grant)
		if e == nil {
			e = d.itemAccess(r.Context(), p, item)
			if e == nil {
				d.admission.rememberCredential(grant, p)
			}
		}
		if e != nil {
			failure(w, e)
			return
		}
		path, e := d.Playback.HLSFileContext(r.Context(), grant, r.PathValue("file"))
		if e != nil {
			failure(w, e)
			return
		}
		f, e := os.Open(path)
		if e != nil {
			w.Header().Set("Retry-After", "1")
			write(w, 503, map[string]any{"error": map[string]any{"code": "stream_not_ready", "message": "Stream segment is not ready.", "retryable": true}})
			return
		}
		defer f.Close()
		info, e := f.Stat()
		if e != nil {
			failure(w, e)
			return
		}
		check := func() error {
			_, p, item, e := d.Playback.ResolveGrantContext(r.Context(), grant)
			if e == nil {
				e = d.itemAccess(r.Context(), p, item)
				if e == nil {
					d.admission.rememberCredential(grant, p)
				}
			}
			return e
		}
		if strings.HasSuffix(path, ".m3u8") {
			serveHLSManifest(w, r, f, check)
			return
		}
		switch filepath.Ext(path) {
		case ".mp4":
			w.Header().Set("Content-Type", "video/mp4")
		case ".m4s":
			w.Header().Set("Content-Type", "video/iso.segment")
		default:
			w.Header().Set("Content-Type", "video/mp2t")
		}
		// A rolling deadline: what is bounded is the gap between two successful
		// writes, never the length of the response, so a slow-but-alive client on a
		// long file is never cut off while a client that has gone silent is
		// reclaimed with its slot, its file and its helper.
		body := withRollingDeadline(w)
		defer body.release()
		http.ServeContent(body, r, r.PathValue("file"), info.ModTime(), &guardedFile{File: f, check: newStreamAuthority(check).check})
	})
	mux.HandleFunc("GET /v1/media/{grant}", func(w http.ResponseWriter, r *http.Request) {
		grant := r.PathValue("grant")
		aid, p, item, e := d.Playback.ResolveGrantContext(r.Context(), grant)
		if e == nil {
			e = d.itemAccess(r.Context(), p, item)
			if e == nil {
				d.admission.rememberCredential(grant, p)
			}
		}
		if e != nil {
			failure(w, e)
			return
		}
		prepared, handled, preparedErr := d.Playback.OpenPrepared(r.Context(), grant)
		if preparedErr != nil {
			preparedFailure(w, preparedErr)
			return
		}
		if handled {
			defer prepared.Close()
			mime := "video/mp4"
			if prepared.Version.Facts.VideoCodec == "" {
				mime = "audio/mp4"
			}
			w.Header().Set("Content-Type", mime)
			w.Header().Set("Cache-Control", "private, no-store")
			body := withRollingDeadline(w)
			defer body.release()
			http.ServeContent(body, r, "prepared.mp4", time.Time{}, &guardedReadSeeker{ReadSeeker: prepared, check: newStreamAuthority(func() error {
				_, p, item, e := d.Playback.ResolveGrantContext(r.Context(), grant)
				if e == nil {
					e = d.itemAccess(r.Context(), p, item)
					if e == nil {
						d.admission.rememberCredential(grant, p)
					}
				}
				return e
			}).check})
			return
		}
		d.serveOriginal(w, r, grant, aid, false, func() error {
			_, p, item, e := d.Playback.ResolveGrantContext(r.Context(), grant)
			if e == nil {
				e = d.itemAccess(r.Context(), p, item)
				if e == nil {
					d.admission.rememberCredential(grant, p)
				}
			}
			return e
		})
	})
	admitted := gatekeeper.wrap(mux, mux)
	routeMuxMu.Lock()
	routeMux = mux
	routeMuxMu.Unlock()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = detachCanceledConnection(w, r)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://www.gstatic.com; style-src 'self'; connect-src 'self' https://*.direct.getportico.tv https://web.getportico.tv; img-src 'self' blob: data: https://*.direct.getportico.tv; media-src 'self' blob:; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'; object-src 'none'")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := sameRequestOrigin(r, origin)
			for _, v := range d.Origins {
				if origin == v {
					allowed = true
				}
			}
			if !allowed {
				write(w, http.StatusForbidden, map[string]any{"error": map[string]any{"code": "origin_not_allowed", "message": "This origin is not permitted.", "retryable": false}})
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Range, If-Match, If-None-Match, Idempotency-Key, Last-Event-ID, Portico-Client, X-Playback-Controller-Token, X-Portico-Transport-Class, X-Portico-Device-Class, X-Portico-Device-Id, X-Portico-Device-Name, X-Portico-Device-Platform, X-Portico-Installation-Id, X-Portico-App, X-Portico-App-Version")
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Accept-Ranges, Content-Length, Location, Retry-After, ETag, Idempotent-Replayed, RateLimit, RateLimit-Policy")
			// Without this a browser remembers a preflight for about five seconds, so the hosted web
			// app (always cross-origin here, always sending Authorization) pays an OPTIONS before
			// nearly every call. The answer only says which methods and headers this origin may use;
			// the real request is still checked against the origin list every time. Browsers cap the
			// value themselves (two hours in Chromium, a day in Firefox).
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == "OPTIONS" {
			if d.enforceSecureConnections(w, r) {
				return
			}
			w.WriteHeader(204)
			return
		}
		if d.enforceSecureConnections(w, r) {
			return
		}
		r = r.WithContext(identity.WithSignInSource(r.Context(), d.signInPeer(r)))
		if installation := r.Header.Get("X-Portico-Installation-Id"); installation != "" {
			claim := identity.DeviceRegistration{
				InstallationID: installation,
				Name:           r.Header.Get("X-Portico-Device-Name"),
				Platform:       r.Header.Get("X-Portico-Device-Platform"),
				App:            r.Header.Get("X-Portico-App"),
				AppVersion:     r.Header.Get("X-Portico-App-Version"),
			}
			ctx, err := identity.WithIssuingDevice(r.Context(), claim, d.signInPeer(r))
			if err != nil {
				failure(w, err)
				return
			}
			r = r.WithContext(ctx)
		}
		if r.Method == "POST" && (strings.HasPrefix(r.URL.Path, "/v1/direct/") || strings.HasPrefix(r.URL.Path, "/v1/auth/") || r.URL.Path == "/v1/sessions" || r.URL.Path == "/v1/setup" || r.URL.Path == "/v1/cast/redeem" || r.URL.Path == "/v1/cast/reconnect") {
			if !authRate.allow(r) {
				w.Header().Set("Retry-After", "60")
				write(w, 429, map[string]any{"error": map[string]any{"code": "rate_limited", "message": "Too many authentication attempts. Try again shortly.", "retryable": true}})
				return
			}
		}
		// Every request carries the edge's own view of how far the client is;
		// delivery planning reads it from the context rather than guessing.
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			if _, pattern := mux.Handler(r); pattern == "" {
				// No route answers this method and path: the mux's own reply is
				// a text body, so answer in the envelope the clients read
				// (BE-API-08). A path that exists for other methods is 405.
				unmatchedRoute(w, r, mux)
				return
			}
		}
		admitted.ServeHTTP(w, d.withDelivery(r))
		if d.Hosted != nil && r.Method != "GET" && r.Method != "HEAD" && membershipPath(r.URL.Path) {
			// The journal triggers already recorded any change in the writer's
			// transaction; this only tells the push loop to look now rather
			// than at its next deadline. A request that changed nothing costs
			// the loop one indexed read.
			d.Hosted.MembershipChanged()
		}
	})
}

// unmatchedRoute answers a /v1 request no route serves: 405 with Allow when
// the path exists for other methods, else 404, both in the error envelope.
func unmatchedRoute(w http.ResponseWriter, r *http.Request, mux *http.ServeMux) {
	allowed := []string{}
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		if method == r.Method {
			continue
		}
		probe := r.Clone(r.Context())
		probe.Method = method
		if _, pattern := mux.Handler(probe); pattern != "" {
			allowed = append(allowed, method)
		}
	}
	if len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		write(w, http.StatusMethodNotAllowed, map[string]any{"error": map[string]any{"code": "method_not_allowed", "message": "This method is not supported here.", "retryable": false}})
		return
	}
	write(w, http.StatusNotFound, map[string]any{"error": map[string]any{"code": "not_found", "message": "The request could not be completed.", "retryable": false}})
}

// membershipPath lists the routes that can change who is a member here.
func membershipPath(path string) bool {
	return strings.HasPrefix(path, "/v1/admin/access/members") || strings.HasPrefix(path, "/v1/direct/members") ||
		path == "/v1/direct/ownership" || path == "/v1/direct/ownership/custody" || path == "/v1/access/invitations/accept"
}

type guardedFile struct {
	*os.File
	check func() error
	last  time.Time
}

func (f *guardedFile) Read(p []byte) (int, error) {
	if time.Since(f.last) > 100*time.Millisecond {
		if e := f.check(); e != nil {
			return 0, e
		}
		f.last = time.Now()
	}
	return f.File.Read(p)
}

type guardedReader struct {
	readError error
	io.Reader
	check func() error
	last  time.Time
}

func (r *guardedReader) Read(p []byte) (int, error) {
	if time.Since(r.last) > 100*time.Millisecond {
		if e := r.check(); e != nil {
			return 0, e
		}
		r.last = time.Now()
	}
	n, e := r.Reader.Read(p)
	if e != nil && e != io.EOF {
		r.readError = e
	}
	return n, e
}

// Guard before and after IPC: a revocation during a blocked read discards bytes.
type guardedReadSeeker struct {
	io.ReadSeeker
	check func() error
}

func (f *guardedReadSeeker) Read(p []byte) (int, error) {
	if e := f.check(); e != nil {
		return 0, e
	}
	n, e := f.ReadSeeker.Read(p)
	if guard := f.check(); guard != nil {
		return 0, guard
	}
	return n, e
}
