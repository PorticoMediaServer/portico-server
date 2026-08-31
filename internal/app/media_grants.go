package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

const (
	mediaGrantCookieName = "portico_media_grant"
	mediaGrantTTL        = 20 * time.Minute
)

var errMediaGrantDenied = errors.New("media grant is invalid, expired, revoked, or out of scope")

type MediaGrant struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

type mediaGrantScope struct {
	ResourceKind   string
	ResourceID     string
	OperationClass string
}

type mediaGrantDelivery struct {
	OperationClasses []string
	DeliveryMode     string
	TranscodeQuality string
	AllowedQualities []string
}

// withMediaResourceAuth requires an operation-scoped credential on byte and
// manifest routes. A normal account credential may mint or renew an operation,
// but must never bypass its profile, resource, lifetime, or revocation scope.
func (s *Server) withMediaResourceAuth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, scopedMediaResource := mediaGrantScopeForRequest(r)
		if !scopedMediaResource && mediaDownloadGrantFromRequest(r) == "" {
			user, ok, err := s.currentUserWithError(w, r)
			if err != nil {
				writeDatabaseAccessError(w, err, http.StatusUnauthorized, "unauthorized", "Authentication is required.")
				return
			}
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication is required.")
				return
			}
			if !s.enforceRoutePolicy(w, r, user) {
				return
			}
			if !apiKeyAllowsRequest(user, r) {
				writeError(w, http.StatusForbidden, "api_key_scope_denied", "This API key is not scoped for that request.")
				return
			}
			// Library Channel manifests and segments are playback capabilities, not
			// ordinary account resources. A cookie or API key identifies the caller
			// but must never substitute for the operation-scoped media grant minted
			// by tune. This narrow guard composes with the shared grant verifier so
			// authorization-revision hardening applies in one place.
			if liveChannelHLSRequiresMediaGrant(r) {
				grantUser, grantErr := s.userForMediaGrant(r)
				if grantErr != nil || accountIDForUser(grantUser) != accountIDForUser(user) || viewerProfileID(grantUser) != viewerProfileID(user) {
					w.Header().Set("Cache-Control", "no-store")
					writeError(w, http.StatusUnauthorized, "media_grant_denied", "A valid playback media grant is required.")
					return
				}
				user = grantUser
				if liveTVTunerHLSRequiresAllocation(r) && !s.heartbeatLiveTVTunerAllocationForGrant(r.Context(), mediaGrantFromRequest(r)) {
					writeError(w, http.StatusUnauthorized, "media_grant_denied", "The Live TV tuner allocation expired. Tune the channel again.")
					return
				}
			}
			r = r.WithContext(contextWithMediaActionUser(r.Context(), user))
			next(w, r, user)
			return
		}
		if mediaDownloadGrantFromRequest(r) != "" {
			user, err := s.consumeMediaDownloadGrant(r)
			if err != nil {
				w.Header().Set("Cache-Control", "no-store")
				writeError(w, http.StatusUnauthorized, "download_grant_denied", "A valid browser download grant is required.")
				return
			}
			if !s.enforceRoutePolicy(w, r, user) {
				return
			}
			w.Header().Set("Referrer-Policy", "no-referrer")
			r = r.WithContext(contextWithMediaActionUser(r.Context(), user))
			next(w, r, user)
			return
		}

		user, err := s.userForMediaGrant(r)
		if err != nil {
			w.Header().Set("Cache-Control", "no-store")
			writeError(w, http.StatusUnauthorized, "media_grant_denied", "A valid playback media grant is required.")
			return
		}
		if !s.enforceRoutePolicy(w, r, user) {
			return
		}
		if liveTVTunerHLSRequiresAllocation(r) && !s.heartbeatLiveTVTunerAllocationForGrant(r.Context(), mediaGrantFromRequest(r)) {
			writeError(w, http.StatusUnauthorized, "media_grant_denied", "The Live TV tuner allocation expired. Tune the channel again.")
			return
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		r = r.WithContext(contextWithMediaActionUser(r.Context(), user))
		next(w, r, user)
	}
}

func liveChannelHLSRequiresMediaGrant(r *http.Request) bool {
	if r == nil {
		return false
	}
	scope, ok := mediaGrantScopeForRequest(r)
	return ok && scope.ResourceKind == "live_channel" && (scope.OperationClass == "manifest" || scope.OperationClass == "segment")
}

func liveTVTunerHLSRequiresAllocation(r *http.Request) bool {
	return r != nil && strings.HasPrefix(strings.Trim(r.URL.Path, "/"), "api/live-tv/hls/")
}

func mediaGrantScopeForRequest(r *http.Request) (mediaGrantScope, bool) {
	if r == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return mediaGrantScope{}, false
	}
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "media" {
		scope := mediaGrantScope{ResourceKind: "media", ResourceID: parts[2]}
		switch {
		case len(parts) == 4 && parts[3] == "stream":
			scope.OperationClass = "byte_range"
		case len(parts) == 5 && parts[3] == "optimized":
			scope.OperationClass = "byte_range"
		case len(parts) >= 5 && parts[3] == "hls" && (parts[4] == "master.m3u8" || parts[4] == "variant.m3u8"):
			scope.OperationClass = "manifest"
		case len(parts) >= 5 && parts[3] == "hls" && parts[4] == "subtitles.m3u8":
			scope.OperationClass = "subtitle"
		case len(parts) >= 5 && parts[3] == "hls" && parts[4] == "segment":
			scope.OperationClass = "segment"
		case len(parts) >= 5 && parts[3] == "hls" && parts[4] == "subtitle.vtt":
			scope.OperationClass = "subtitle"
		case len(parts) == 5 && parts[3] == "subtitles":
			scope.OperationClass = "subtitle"
		case len(parts) >= 6 && parts[3] == "trickplay":
			scope.OperationClass = "trickplay"
		default:
			return mediaGrantScope{}, false
		}
		return scope, scope.ResourceID != ""
	}
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "live-tv" {
		scope := mediaGrantScope{ResourceKind: "live_channel"}
		switch {
		case len(parts) == 4 && parts[2] == "streams":
			scope.ResourceID, scope.OperationClass = parts[3], "byte_range"
		case len(parts) >= 5 && parts[2] == "hls":
			scope.ResourceID = parts[3]
			if parts[4] == "playlist.m3u8" {
				scope.OperationClass = "manifest"
			} else if parts[4] == "segment" || parts[4] == "item" {
				scope.OperationClass = "segment"
			}
		}
		return scope, scope.ResourceID != "" && scope.OperationClass != ""
	}
	if len(parts) >= 5 && parts[0] == "api" && parts[1] == "library-channels" && parts[3] == "hls" {
		scope := mediaGrantScope{ResourceKind: "live_channel", ResourceID: parts[2]}
		if parts[4] == "playlist.m3u8" {
			scope.OperationClass = "manifest"
		} else if parts[4] == "segment" {
			scope.OperationClass = "segment"
		}
		return scope, scope.ResourceID != "" && scope.OperationClass != ""
	}
	return mediaGrantScope{}, false
}

func mediaGrantFromRequest(r *http.Request) string {
	if r == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return ""
	}
	if authorization := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(authorization, "PorticoMedia ") {
		return strings.TrimSpace(strings.TrimPrefix(authorization, "PorticoMedia "))
	}
	cookie, err := r.Cookie(mediaGrantCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

// liveMediaGrantDeliveryMode returns the server-resolved delivery mode bound
// to an already authenticated Live TV resource request. It deliberately reads
// the persisted grant instead of trusting a query parameter, so a client cannot
// switch a direct provider session into an unadmitted transcode (or vice versa).
func (s *Server) liveMediaGrantDeliveryMode(ctx context.Context, r *http.Request, channelID string) (string, error) {
	token := mediaGrantFromRequest(r)
	channelID = strings.TrimSpace(channelID)
	if !strings.HasPrefix(token, "ptc_mg_") || channelID == "" {
		return "", errMediaGrantDenied
	}
	var mode string
	err := s.queryUserRow(ctx, `
		SELECT COALESCE(delivery_mode, '')
		FROM playback_media_grants
		WHERE token_hash = ? AND resource_kind = 'live_channel' AND resource_id = ?
			AND revoked_at = '' AND expires_at > ?
		LIMIT 1`, hashToken(token), channelID, time.Now().UTC().Format(time.RFC3339)).Scan(&mode)
	if err != nil {
		return "", errMediaGrantDenied
	}
	return strings.ToLower(strings.TrimSpace(mode)), nil
}

func requestUsesTLS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	// The outer requestTransport middleware is the sole authority for
	// forwarded transport. Never let an arbitrary client-supplied
	// X-Forwarded-Proto value mark a capability cookie Secure.
	secure, _ := r.Context().Value(requestTransportSecureKey{}).(bool)
	return secure
}

func setPlaybackMediaGrantCookie(w http.ResponseWriter, r *http.Request, playback PlaybackResponse) {
	if strings.TrimSpace(playback.MediaGrant.Token) == "" || strings.TrimSpace(playback.Media.ID) == "" {
		return
	}
	expires, _ := time.Parse(time.RFC3339, playback.MediaGrant.ExpiresAt)
	maxAge := int(time.Until(expires).Seconds())
	paths := []string{"/api/media/" + urlPathEscape(playback.Media.ID)}
	switch strings.ToLower(strings.TrimSpace(playback.Media.Type)) {
	case "library_channel":
		paths = []string{"/api/library-channels/" + urlPathEscape(playback.Media.ID) + "/hls"}
	case "live_channel":
		paths = []string{"/api/live-tv/hls/" + urlPathEscape(playback.Media.ID), "/api/live-tv/streams/" + urlPathEscape(playback.Media.ID)}
	default:
		if playback.IsLive {
			paths = []string{
				"/api/live-tv/hls/" + urlPathEscape(playback.Media.ID),
				"/api/live-tv/streams/" + urlPathEscape(playback.Media.ID),
				"/api/library-channels/" + urlPathEscape(playback.Media.ID) + "/hls",
			}
		}
	}
	for _, path := range paths {
		http.SetCookie(w, &http.Cookie{Name: mediaGrantCookieName, Value: playback.MediaGrant.Token, Path: path, Expires: expires, MaxAge: maxAge, HttpOnly: true, Secure: requestUsesTLS(r), SameSite: http.SameSiteStrictMode})
	}
}

func (s *Server) userForMediaGrant(r *http.Request) (User, error) {
	terminateAuthorization := func(sessionID string) {
		_, _ = s.playbackLifecycle().Terminate(r.Context(), playbackTerminationRequest{
			SessionID: sessionID, Cause: playbackTerminationAuthorization,
		})
	}
	scope, ok := mediaGrantScopeForRequest(r)
	if !ok {
		return User{}, errMediaGrantDenied
	}
	token := mediaGrantFromRequest(r)
	if !strings.HasPrefix(token, "ptc_mg_") {
		return User{}, errMediaGrantDenied
	}
	now := time.Now().UTC()
	tokenHash := hashToken(token)
	if cached, ok := s.cachedMediaGrantSnapshot(tokenHash); ok {
		if cached.resourceKind != scope.ResourceKind || cached.resourceID != scope.ResourceID || !cached.expiresAt.After(now) || !mediaGrantRequestAllowed(cached.operationClasses, cached.deliveryMode, cached.transcodeQuality, cached.allowedQualities, scope, r) {
			s.forgetMediaGrant(tokenHash)
			return User{}, errMediaGrantDenied
		}
		// Scope and hard expiry remain synchronous on every request. Established
		// segment delivery stays DB-free inside the short cache lease, while
		// manifests and other operation classes recheck authorization immediately;
		// this prevents a newly disabled profile or channel policy from renewing a
		// playback timeline without adding one SQLite read to every media segment.
		if scope.OperationClass == "segment" && now.Sub(cached.verifiedAt) < mediaGrantVerifyInterval {
			return cached.user, nil
		}
		probeCtx, cancel := context.WithTimeout(r.Context(), 75*time.Millisecond)
		terminal, terminalErr := s.mediaGrantTerminalProbeContext(probeCtx, tokenHash, scope)
		if terminalErr != nil {
			cancel()
			if mediaGrantTransientDatabaseError(terminalErr, r.Context()) && now.Sub(cached.verifiedAt) <= mediaGrantBusyFallbackTTL {
				s.mediaGrantCacheCounters.busyFallbacks.Add(1)
				return cached.user, nil
			}
			return User{}, errMediaGrantDenied
		}
		terminalDenied := terminal.revokedAt != "" || !terminal.expiresAt.After(now) || terminal.playbackEndedAt != "" || terminal.playbackState == "stopped" || terminal.userDisabledAt != ""
		if !terminalDenied {
			currentRevision, revisionErr := s.authorizationRevisionForUserContextStrict(probeCtx, cached.user)
			cancel()
			if revisionErr != nil {
				if mediaGrantTransientDatabaseError(revisionErr, r.Context()) && now.Sub(cached.verifiedAt) <= mediaGrantBusyFallbackTTL {
					s.mediaGrantCacheCounters.busyFallbacks.Add(1)
					return cached.user, nil
				}
				return User{}, errMediaGrantDenied
			}
			terminalDenied = terminal.storedAuthRevision == "" || terminal.storedAuthRevision != cached.authorizationRevision || currentRevision != cached.authorizationRevision
		} else {
			cancel()
		}
		if terminalDenied {
			s.mediaGrantCacheCounters.terminalDenials.Add(1)
			terminateAuthorization(cached.playbackSessionID)
			return User{}, errMediaGrantDenied
		}
		cached.verifiedAt = now
		s.rememberVerifiedMediaGrant(cached)
		return cached.user, nil
	}
	var userID string
	var profileID string
	var playbackSessionID string
	var operationClassesJSON string
	var lastAuthorizedAt string
	var authorizationRevision string
	var deliveryMode string
	var transcodeQuality string
	var allowedQualitiesJSON string
	var expiresAt string
	err := s.queryUserRow(r.Context(), `
		SELECT g.principal_user_id, g.profile_id, g.playback_session_id, g.operation_classes_json, g.last_authorized_at, g.authorization_revision, g.expires_at,
			COALESCE(g.delivery_mode, ''), COALESCE(g.transcode_quality, ''), COALESCE(g.allowed_qualities_json, '[]')
		FROM playback_media_grants g
		JOIN playback_sessions ps ON ps.id = g.playback_session_id
		JOIN users u ON u.id = g.principal_user_id
		WHERE g.token_hash = ?
			AND g.resource_kind = ? AND g.resource_id = ?
			AND g.revoked_at = '' AND g.expires_at > ?
			AND ps.user_id = g.principal_user_id
			AND ps.profile_id = g.profile_id
			AND ps.ended_at = '' AND ps.state <> 'stopped'
			AND COALESCE(u.disabled_at, '') = ''
		LIMIT 1`, tokenHash, scope.ResourceKind, scope.ResourceID, now.Format(time.RFC3339)).Scan(&userID, &profileID, &playbackSessionID, &operationClassesJSON, &lastAuthorizedAt, &authorizationRevision, &expiresAt, &deliveryMode, &transcodeQuality, &allowedQualitiesJSON)
	if err != nil {
		return User{}, errMediaGrantDenied
	}
	var allowed []string
	if json.Unmarshal([]byte(operationClassesJSON), &allowed) != nil || !stringListContains(allowed, scope.OperationClass) {
		return User{}, errMediaGrantDenied
	}
	var allowedQualities []string
	if json.Unmarshal([]byte(allowedQualitiesJSON), &allowedQualities) != nil || !mediaGrantRequestAllowed(allowed, deliveryMode, transcodeQuality, allowedQualities, scope, r) {
		return User{}, errMediaGrantDenied
	}
	tunerAllocationCount := 0
	if scope.ResourceKind == "live_channel" {
		var libraryPolicyCount int
		if err := s.queryUserRow(r.Context(), `SELECT COUNT(*) FROM library_channel_playback_policies WHERE playback_session_id = ? AND channel_id = ?`, playbackSessionID, scope.ResourceID).Scan(&libraryPolicyCount); err != nil {
			return User{}, errMediaGrantDenied
		}
		if err := s.queryUserRow(r.Context(), `SELECT COUNT(*) FROM live_tv_tuner_allocations WHERE allocation_kind = 'live_session' AND consumer_id = ? AND channel_id = ?`, playbackSessionID, scope.ResourceID).Scan(&tunerAllocationCount); err != nil || (libraryPolicyCount == 0 && tunerAllocationCount != 1) {
			return User{}, errMediaGrantDenied
		}
	}
	lastAuthorized, _ := time.Parse(time.RFC3339, lastAuthorizedAt)
	if lastAuthorized.IsZero() || now.Sub(lastAuthorized) >= time.Minute {
		_, _ = s.execUserWrite(r.Context(), `UPDATE playback_media_grants SET last_authorized_at = ? WHERE token_hash = ? AND last_authorized_at = ?`, now.Format(time.RFC3339), hashToken(token), lastAuthorizedAt)
	}
	principal, principalErr := s.resolveRequestPrincipalContext(r.Context(), userID, profileID)
	if principalErr != nil {
		terminateAuthorization(playbackSessionID)
		return User{}, errMediaGrantDenied
	}
	user := User{ID: userID, AccountID: userID, ProfileID: profileID}
	applyRequestPrincipal(&user, principal)
	user = s.hydratePlaybackVisibilityUserContext(r.Context(), user)
	currentAuthorizationRevision, revisionErr := s.authorizationRevisionForUserContextStrict(r.Context(), user)
	if revisionErr != nil || authorizationRevision == "" || authorizationRevision != currentAuthorizationRevision {
		terminateAuthorization(playbackSessionID)
		return User{}, errMediaGrantDenied
	}
	if !hasPermission(user, "playMedia") || (scope.ResourceKind == "live_channel" && !canPlayLiveTV(user)) {
		terminateAuthorization(playbackSessionID)
		return User{}, errMediaGrantDenied
	}
	if scope.ResourceKind == "live_channel" && tunerAllocationCount == 1 {
		channel, source, channelErr := s.getLiveTVChannelForPlayback(scope.ResourceID)
		if channelErr != nil || !source.Enabled || !s.userLiveTVChannelAllowedForUser(user, channel.ID) {
			terminateAuthorization(playbackSessionID)
			return User{}, errMediaGrantDenied
		}
	}
	if scope.ResourceKind == "media" {
		if _, err := s.getMediaPlaybackDetailForUser(r.Context(), user, scope.ResourceID); err != nil {
			return User{}, errMediaGrantDenied
		}
	} else if !canPlayLiveTV(user) {
		return User{}, errMediaGrantDenied
	}
	parsedExpires, parseErr := time.Parse(time.RFC3339, expiresAt)
	if parseErr != nil || !parsedExpires.After(now) {
		return User{}, errMediaGrantDenied
	}
	s.rememberVerifiedMediaGrant(verifiedMediaGrantSnapshot{
		tokenHash: tokenHash, resourceKind: scope.ResourceKind, resourceID: scope.ResourceID, playbackSessionID: playbackSessionID,
		user: user, operationClasses: allowed, deliveryMode: deliveryMode, transcodeQuality: transcodeQuality,
		allowedQualities: allowedQualities, expiresAt: parsedExpires, authorizationRevision: authorizationRevision, verifiedAt: now,
	})
	return user, nil
}

func mediaGrantRequestAllowed(operationClasses []string, deliveryMode, transcodeQuality string, allowedQualities []string, scope mediaGrantScope, r *http.Request) bool {
	if !stringListContains(operationClasses, scope.OperationClass) {
		return false
	}
	if scope.OperationClass != "manifest" && scope.OperationClass != "segment" {
		return true
	}
	rawQuality := strings.TrimSpace(r.URL.Query().Get("quality"))
	requestedQuality := normalizeTranscodeQuality(rawQuality)
	comparisonQuality := normalizeTranscodeQuality(transcodeQuality)
	if rawQuality == "" {
		requestedQuality = comparisonQuality
	}
	if scope.ResourceKind == "live_channel" {
		comparisonQuality = normalizeLiveTVQualityID(transcodeQuality)
		if rawQuality == "" {
			requestedQuality = comparisonQuality
		} else {
			requestedQuality = normalizeLiveTVQualityID(rawQuality)
		}
	}
	if !stringListContains(allowedQualities, requestedQuality) && requestedQuality != comparisonQuality {
		return false
	}
	return r.URL.Query().Get("directStream") == "" || deliveryMode == "direct_stream"
}

func (s *Server) issueMediaGrant(ctx context.Context, user User, playbackSessionID, resourceKind, resourceID string) (MediaGrant, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if resourceKind == "live_channel" {
		delivery := mediaGrantDelivery{OperationClasses: []string{"manifest", "segment"}}
		var encodedPolicy string
		if err := s.queryUserRow(ctx, `SELECT policy_json FROM library_channel_playback_policies WHERE playback_session_id = ? AND channel_id = ?`, playbackSessionID, resourceID).Scan(&encodedPolicy); err == nil {
			var policy PlaybackDeliveryPolicy
			if json.Unmarshal([]byte(encodedPolicy), &policy) != nil || !policy.GrantRequired || len(policy.AllowedOperationClasses) == 0 {
				return MediaGrant{}, errMediaGrantDenied
			}
			delivery.OperationClasses = policy.AllowedOperationClasses
			delivery.DeliveryMode = policy.DeliveryMode
			delivery.TranscodeQuality = policy.QualityProfile
			delivery.AllowedQualities = []string{normalizeTranscodeQuality(policy.QualityProfile)}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return MediaGrant{}, errMediaGrantDenied
		}
		return s.issueMediaGrantBound(ctx, user, playbackSessionID, resourceKind, resourceID, true, true, delivery)
	}
	return s.issueMediaGrantWithOptions(ctx, user, playbackSessionID, resourceKind, resourceID, true, true)
}

func mediaGrantDeliveryForPlayback(plan playbackExecutionPlan) mediaGrantDelivery {
	mode := "transcode_required"
	switch plan.Plan.Mode {
	case playbackplan.DirectPlay:
		mode = "direct_play"
		if plan.OptimizedArtifactID != "" {
			mode = "optimized_version"
		}
	case playbackplan.Remux, playbackplan.DirectStream:
		mode = "direct_stream"
	}
	delivery := mediaGrantDelivery{DeliveryMode: mode}
	plannedQuality := plan.Quality
	if mode == "direct_play" || mode == "optimized_version" {
		plannedQuality = "original"
	}
	delivery.TranscodeQuality = plannedQuality
	delivery.AllowedQualities = []string{plannedQuality}
	switch mode {
	case "direct_play", "optimized_version":
		delivery.OperationClasses = []string{"byte_range", "subtitle", "trickplay"}
	case "direct_stream", "transcode_required":
		delivery.OperationClasses = []string{"manifest", "segment", "subtitle", "trickplay"}
	default:
		delivery.OperationClasses = []string{"subtitle", "trickplay"}
	}
	return delivery
}

func (s *Server) issueMediaGrantForPlayback(ctx context.Context, user User, playbackSessionID string, item MediaItem, decision PlaybackDecision, policy ResolvedPlaybackPolicy, requireSessionResource, revokeExisting bool) (MediaGrant, error) {
	binding, _, err := playbackPlanPersistence(decision)
	if err != nil {
		return MediaGrant{}, errMediaGrantDenied
	}
	return s.issueMediaGrantBoundToPlan(ctx, user, playbackSessionID, "media", item.ID, requireSessionResource, revokeExisting, mediaGrantDeliveryForPlayback(binding), &binding)
}

func (s *Server) issueLiveMediaGrantForPlayback(ctx context.Context, user User, playbackSessionID, channelID string, decision PlaybackDecision, selectedQuality string, revokeExisting bool) (MediaGrant, error) {
	selectedQuality = normalizeLiveTVQualityID(selectedQuality)
	return s.issueMediaGrantBound(ctx, user, playbackSessionID, "live_channel", channelID, true, revokeExisting, mediaGrantDelivery{
		OperationClasses: []string{"manifest", "segment"},
		DeliveryMode:     decision.Mode,
		TranscodeQuality: selectedQuality,
		AllowedQualities: []string{selectedQuality},
	})
}

func (s *Server) rotateMediaGrantForSession(ctx context.Context, user User, playbackSessionID, resourceKind, resourceID string) (MediaGrant, error) {
	var classesJSON string
	var deliveryMode string
	var quality string
	var allowedQualitiesJSON string
	err := s.queryUserRow(ctx, `
		SELECT operation_classes_json, COALESCE(delivery_mode, ''), COALESCE(transcode_quality, ''), COALESCE(allowed_qualities_json, '[]')
		FROM playback_media_grants
		WHERE playback_session_id = ? AND principal_user_id = ? AND profile_id = ?
			AND resource_kind = ? AND resource_id = ?
		ORDER BY issued_at DESC LIMIT 1`, playbackSessionID, accountIDForUser(user), viewerProfileID(user), resourceKind, resourceID).
		Scan(&classesJSON, &deliveryMode, &quality, &allowedQualitiesJSON)
	if err != nil {
		return MediaGrant{}, errMediaGrantDenied
	}
	var classes []string
	if json.Unmarshal([]byte(classesJSON), &classes) != nil || len(classes) == 0 {
		return MediaGrant{}, errMediaGrantDenied
	}
	var allowedQualities []string
	_ = json.Unmarshal([]byte(allowedQualitiesJSON), &allowedQualities)
	return s.issueMediaGrantBound(ctx, user, playbackSessionID, resourceKind, resourceID, true, true, mediaGrantDelivery{
		OperationClasses: classes,
		DeliveryMode:     deliveryMode,
		TranscodeQuality: quality,
		AllowedQualities: allowedQualities,
	})
}

func (s *Server) issueQueuedMediaGrant(ctx context.Context, user User, playbackSessionID, resourceID string) (MediaGrant, error) {
	return s.issueMediaGrantWithOptions(ctx, user, playbackSessionID, "media", resourceID, false, false)
}

func (s *Server) issueMediaGrantWithOptions(ctx context.Context, user User, playbackSessionID, resourceKind, resourceID string, requireSessionResource, revokeExisting bool) (MediaGrant, error) {
	return s.issueMediaGrantBound(ctx, user, playbackSessionID, resourceKind, resourceID, requireSessionResource, revokeExisting, mediaGrantDelivery{
		OperationClasses: []string{"byte_range", "manifest", "segment", "subtitle", "trickplay"},
	})
}

func (s *Server) issueMediaGrantBound(ctx context.Context, user User, playbackSessionID, resourceKind, resourceID string, requireSessionResource, revokeExisting bool, delivery mediaGrantDelivery) (MediaGrant, error) {
	return s.issueMediaGrantBoundToPlan(ctx, user, playbackSessionID, resourceKind, resourceID, requireSessionResource, revokeExisting, delivery, nil)
}

func (s *Server) issueMediaGrantBoundToPlan(ctx context.Context, user User, playbackSessionID, resourceKind, resourceID string, requireSessionResource, revokeExisting bool, delivery mediaGrantDelivery, explicitPlan *playbackExecutionPlan) (MediaGrant, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	playbackSessionID = strings.TrimSpace(playbackSessionID)
	resourceKind = strings.TrimSpace(resourceKind)
	resourceID = strings.TrimSpace(resourceID)
	if playbackSessionID == "" || user.ID == "" || resourceID == "" || (resourceKind != "media" && resourceKind != "live_channel") {
		return MediaGrant{}, errMediaGrantDenied
	}
	var sessionOwner string
	var sessionProfileID string
	var sessionMediaID string
	var isLive int
	var planDigest string
	var planJSON string
	var sourceRevision string
	var playbackGeneration int
	if err := s.queryUserRow(ctx, `SELECT user_id, profile_id, media_id, is_live, COALESCE(plan_digest, ''), COALESCE(plan_json, '{}'), COALESCE(source_revision, ''), COALESCE(playback_generation, 0) FROM playback_sessions WHERE id = ? AND ended_at = '' AND state <> 'stopped'`, playbackSessionID).Scan(&sessionOwner, &sessionProfileID, &sessionMediaID, &isLive, &planDigest, &planJSON, &sourceRevision, &playbackGeneration); err != nil {
		return MediaGrant{}, err
	}
	if sessionOwner != accountIDForUser(user) || sessionProfileID != viewerProfileID(user) || (resourceKind == "live_channel") != (isLive == 1) {
		return MediaGrant{}, errMediaGrantDenied
	}
	if requireSessionResource && sessionMediaID != resourceID {
		return MediaGrant{}, errMediaGrantDenied
	}
	if explicitPlan != nil {
		candidate := *explicitPlan
		storedDigest := candidate.Digest
		if err := candidate.seal(); err != nil || candidate.Digest != storedDigest {
			return MediaGrant{}, errMediaGrantDenied
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return MediaGrant{}, errMediaGrantDenied
		}
		planDigest, planJSON, sourceRevision, playbackGeneration = candidate.Digest, string(encoded), candidate.Plan.SourceRevision, candidate.generation()
	} else if _, err := decodePlaybackExecutionPlan(planJSON); err != nil {
		return MediaGrant{}, errMediaGrantDenied
	}
	principal, principalErr := s.resolveRequestPrincipalContext(ctx, accountIDForUser(user), viewerProfileID(user))
	if principalErr != nil {
		return MediaGrant{}, errMediaGrantDenied
	}
	authoritativeUser := User{ID: accountIDForUser(user), AccountID: accountIDForUser(user), ProfileID: viewerProfileID(user)}
	applyRequestPrincipal(&authoritativeUser, principal)
	authoritativeUser = s.hydratePlaybackVisibilityUserContext(ctx, authoritativeUser)
	if !requireSessionResource {
		var queued int
		_ = s.queryUserRow(ctx, `SELECT COUNT(*) FROM playback_session_queue WHERE session_id = ? AND media_id = ?`, playbackSessionID, resourceID).Scan(&queued)
		if queued == 0 {
			if _, err := s.getMediaPlaybackDetailForUser(ctx, user, resourceID); err != nil {
				return MediaGrant{}, errMediaGrantDenied
			}
		}
	}
	now := time.Now().UTC()
	expires := now.Add(mediaGrantTTL)
	token := "ptc_mg_" + randomToken()
	classes := delivery.OperationClasses
	if len(classes) == 0 {
		return MediaGrant{}, errMediaGrantDenied
	}
	classesJSON, _ := json.Marshal(classes)
	allowedQualitiesJSON, _ := json.Marshal(delivery.AllowedQualities)
	authorizationRevision, revisionErr := s.authorizationRevisionForUserContextStrict(ctx, authoritativeUser)
	if revisionErr != nil {
		return MediaGrant{}, errMediaGrantDenied
	}
	withGrantTx := s.withPlaybackTxTagged
	if revokeExisting {
		withGrantTx = s.withSecurityFenceTxTagged
	}
	err := withGrantTx(ctx, []string{"playback"}, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM playback_media_grants
			WHERE expires_at < ? OR (revoked_at <> '' AND revoked_at < ?)`,
			now.Add(-24*time.Hour).Format(time.RFC3339), now.Add(-24*time.Hour).Format(time.RFC3339)); err != nil {
			return err
		}
		if revokeExisting {
			if _, err := tx.ExecContext(ctx, `UPDATE playback_media_grants SET revoked_at = ? WHERE playback_session_id = ? AND revoked_at = ''`, now.Format(time.RFC3339), playbackSessionID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
		INSERT INTO playback_media_grants (
			id, token_hash, playback_session_id, principal_user_id, profile_id, resource_kind, resource_id,
			operation_classes_json, issued_at, expires_at, last_authorized_at, revoked_at, authorization_revision,
			delivery_mode, transcode_quality, allowed_qualities_json, plan_digest, plan_json, source_revision, playback_generation
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, ?, ?, ?, ?, ?, ?, ?)`,
			randomID("mgr"), hashToken(token), playbackSessionID, accountIDForUser(user), viewerProfileID(user), resourceKind, resourceID,
			string(classesJSON), now.Format(time.RFC3339), expires.Format(time.RFC3339), authorizationRevision,
			strings.TrimSpace(delivery.DeliveryMode), normalizeTranscodeQuality(delivery.TranscodeQuality), string(allowedQualitiesJSON),
			planDigest, planJSON, sourceRevision, playbackGeneration); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return MediaGrant{}, err
	}
	return MediaGrant{Token: token, ExpiresAt: expires.Format(time.RFC3339)}, nil
}

func (s *Server) renewMediaGrantsForSession(ctx context.Context, user User, playbackSessionID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()
	result, err := s.execPlaybackWrite(ctx, `
		UPDATE playback_media_grants
		SET expires_at = ?
		WHERE playback_session_id = ? AND principal_user_id = ? AND profile_id = ? AND revoked_at = ''
			AND expires_at <= ?
			AND EXISTS (
				SELECT 1 FROM playback_sessions ps
				WHERE ps.id = playback_media_grants.playback_session_id
					AND ps.user_id = ? AND ps.profile_id = ? AND ps.ended_at = '' AND ps.state <> 'stopped'
				)`, now.Add(mediaGrantTTL).Format(time.RFC3339), playbackSessionID, accountIDForUser(user), viewerProfileID(user), now.Add(5*time.Minute).Format(time.RFC3339), accountIDForUser(user), viewerProfileID(user))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	s.forgetMediaGrantsForPlaybackSession(playbackSessionID)
	return nil
}

func appendMediaGrant(rawURL string, grant MediaGrant) string {
	// Credentials are delivered only through a Secure HttpOnly cookie or the
	// native Authorization header. Resource URLs must remain credential-free.
	return rawURL
}

func (s *Server) revokeMediaGrantsForSession(ctx context.Context, playbackSessionID string) {
	if ctx == nil {
		ctx = context.Background()
	}
	_, _ = s.execSecurityFenceWriteTagged(ctx, []string{"playback"}, `UPDATE playback_media_grants SET revoked_at = ? WHERE playback_session_id = ? AND revoked_at = ''`, time.Now().UTC().Format(time.RFC3339), playbackSessionID)
	s.forgetMediaGrantsForPlaybackSession(playbackSessionID)
}
