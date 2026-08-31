package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
	"github.com/PorticoMediaServer/portico-server/internal/librarychannels"
)

var errLibraryChannelPlaybackCapacity = errors.New("Library Channel playback capacity is unavailable")

type libraryChannelHLSSegment struct {
	Entry    librarychannels.ScheduleEntry
	Index    int
	StartsAt time.Time
	Duration time.Duration
}

func libraryChannelHLSPlaylistRoute(channelID string) string {
	return "/api/library-channels/" + urlPathEscape(channelID) + "/hls/playlist.m3u8"
}

func (s *Server) startLibraryChannelPlayback(r *http.Request, user User, channel librarychannels.Channel, entry librarychannels.ScheduleEntry, clientProfile PlaybackClientProfile, intent PlaybackIntent, clientInstanceID string, replacement *PlaybackReplacementRequest, externalReplacement *playbackReplacementPlan) (PlaybackResponse, error) {
	if !canPlayLiveTV(user) || !hasPermission(user, "playMedia") {
		return PlaybackResponse{}, librarychannels.ErrProgramRestricted
	}
	targetRequest := libraryChannelTuneRequest{ClientInstanceID: clientInstanceID, ClientProfile: clientProfile, Intent: intent, Replacement: replacement}
	replacementPlan := playbackReplacementPlan{}
	if externalReplacement == nil {
		var replacementErr *playbackStartHTTPError
		replacementPlan, replacementErr = s.preparePlaybackReplacement(r.Context(), user, clientInstanceID, "library-channel", channel.ID, targetRequest, replacement)
		if replacementErr != nil {
			return PlaybackResponse{}, replacementErr
		}
		if replacementPlan.Committed != nil {
			return *replacementPlan.Committed, nil
		}
	}
	releaseReplacement := replacementPlan.Active
	if releaseReplacement {
		defer func() {
			if releaseReplacement {
				s.rollbackPlaybackReplacement(replacementPlan)
			}
		}()
	}
	if err := s.enforcePlaybackSessionReplacementLimitContext(r.Context(), user.ID, clientInstanceID); err != nil {
		return PlaybackResponse{}, err
	}
	if _, err := s.getMediaPlaybackDetailForUser(r.Context(), user, entry.MediaID); err != nil {
		return PlaybackResponse{}, librarychannels.ErrProgramRestricted
	}
	quality := normalizeTranscodeQuality(channel.QualityProfile)
	if channel.Logo.BugEnabled && !channel.Logo.BugOverheadAccepted {
		return PlaybackResponse{}, errors.New("Library Channel logo bug requires explicit transcode-overhead acceptance")
	}
	sourceURL := libraryChannelHLSPlaylistRoute(channel.ID)
	media := MediaItem{
		ID: channel.ID, LibraryID: channel.ID, Type: "library_channel", Title: channel.Name,
		SortTitle: channel.Name, Summary: channel.Description, DurationSeconds: 0,
		Genres: []string{"Library Channel"}, Tags: []string{"Library Channels"},
		AddedAt: channel.CreatedAt.UTC().Format(time.RFC3339), SourceURL: sourceURL,
		Images: imageSetFor(channel.ID, channel.Name, channel.Description, nil), Actions: []string{"play"},
	}
	if logoURL := libraryChannelLogoURL(channel.Logo); logoURL != "" {
		media.Images.Thumb = logoURL
	}
	decision := PlaybackDecision{
		Mode: "transcode", Reason: "Library Channels use a server-owned continuous HLS timeline.",
		SourceKind: "library_channel", Protocol: "hls", Container: "mpegts",
		RequiresTranscode: true, VideoTranscode: true, AudioTranscode: true, IsProxied: true,
	}
	resolvedPolicy, clientProfile := s.resolvePlaybackPolicyForRequest(r.Context(), r, user, media, intent, clientProfile)
	qualityAuthority, err := issueContinuousPlaybackQualityOffers(
		channel.ID, channel.ID+"\x00"+entry.MediaID,
		entry.MediaID+"\x00"+entry.StartsAt.UTC().Format(time.RFC3339Nano),
		resolvedPolicy, []string{"1080p-high", "1080p-medium", "720p-high", "720p-medium", "480p", "328p"}, false,
	)
	if err != nil {
		return PlaybackResponse{}, err
	}
	resolvedQuality, resolvedPolicy, clientProfile, err := resolvePlaybackQualityForRequest(qualityAuthority, intent.Quality, resolvedPolicy, clientProfile, media.Type)
	if err != nil {
		return PlaybackResponse{}, err
	}
	intent.Quality = normalizedPlaybackQualitySelection(intent.Quality)
	if resolvedQuality.Kind == playbackQualityKindFixed {
		quality = resolvedQuality.PresetID
	} else {
		quality = resolvedLibraryChannelQuality(quality, resolvedPolicy)
	}
	sessionID := randomID("lchplay")
	initialState := "playing"
	if externalReplacement != nil {
		sessionID = externalReplacement.Claim.ReplacementSessionID
		initialState = "handoff_pending"
	} else if replacementPlan.Active {
		sessionID = replacementPlan.Claim.ReplacementSessionID
		initialState = "handoff_pending"
	}
	currentEntryID := randomID("qentry")
	sourceContext := PlaybackSourceContext{Type: "library-channel", ID: channel.ID, Title: channel.Name, MediaIDs: []string{entry.MediaID}}
	if err := s.createPlaybackSessionWithState(r, user, media, currentEntryID, sessionID, decision, clientProfile, intent, "", "", true, clientInstanceID, sourceContext, "off", initialState); err != nil {
		return PlaybackResponse{}, err
	}
	cleanupFailedStart := func() {
		_, _ = s.playbackLifecycle().Terminate(context.Background(), playbackTerminationRequest{
			SessionID: sessionID, UserID: accountIDForUser(user), ProfileID: viewerProfileID(user),
			Cause: playbackTerminationFailedStart, RemoveSession: true,
		})
	}
	resolvedPolicy = resolvedLibraryChannelPlaybackPolicy(resolvedPolicy, clientProfile, quality, channel.Logo.BugEnabled, channel.ConfigRevision)
	if err := s.bindLibraryChannelDeliveryPolicy(r.Context(), sessionID, channel.ID, user, resolvedPolicy); err != nil {
		cleanupFailedStart()
		return PlaybackResponse{}, err
	}
	grant, err := s.issueMediaGrant(r.Context(), user, sessionID, "live_channel", channel.ID)
	if err != nil {
		cleanupFailedStart()
		return PlaybackResponse{}, err
	}
	sourceURL = libraryChannelHLSPlaylistRoute(channel.ID)
	media.SourceURL = sourceURL
	selectedAudioID := "audio_default"
	// The continuous channel compositor does not currently expose a stable
	// current-program subtitle resource. Never advertise text subtitles unless a
	// concrete resource exists; the requested preference remains available to a
	// future compositor that can truthfully satisfy it.
	selectedSubtitleMode := "off"
	resources := []PlaybackResource{{ID: randomID("pres"), SourceURL: sourceURL, StreamFormat: "hls", AudioStreamID: selectedAudioID, SubtitleMode: selectedSubtitleMode, Default: true}}
	playback := PlaybackResponse{
		SessionID: sessionID, CurrentQueueEntryID: currentEntryID, NextEventSequence: 1, MediaGrant: grant, Media: media, SourceURL: sourceURL,
		DirectPlay: false, IsLive: true, StreamFormat: "hls", Decision: decision,
		Policy:                resolvedPolicy,
		QualityOffers:         qualityAuthority.set,
		QualitySelection:      intent.Quality,
		Resources:             resources,
		AudioStreams:          []Stream{{ID: selectedAudioID, Kind: "audio", Codec: "aac", DisplayTitle: "Program audio"}},
		SubtitleStreams:       []Stream{{ID: "sub_none", Kind: "subtitle", DisplayTitle: "None"}},
		SelectedAudioStreamID: selectedAudioID, SelectedSubtitleMode: selectedSubtitleMode,
		Chapters: []Chapter{}, Queue: []PlaybackQueueEntry{}, RepeatMode: "off", QueueRevision: 0, PlaybackRevision: 0,
		SourceContext: sourceContext, Timeline: PlaybackTimeline{Type: "live", SegmentSeconds: hlsSegmentSeconds, CanPause: false, CanSeek: false}, Generation: 0,
	}
	if err := s.ensurePlaybackContinuationCredential(r, user, &playback); err != nil {
		cleanupFailedStart()
		return PlaybackResponse{}, err
	}
	if replacementPlan.Active {
		if err := s.commitPlaybackReplacement(r.Context(), user, replacementPlan, playback); err != nil {
			cleanupFailedStart()
			return PlaybackResponse{}, playbackReplacementCommitHTTPError(err)
		}
		releaseReplacement = false
	}
	return playback, nil
}

func resolvedLibraryChannelPlaybackPolicy(policy ResolvedPlaybackPolicy, profile PlaybackClientProfile, quality string, overlay bool, revision int64) ResolvedPlaybackPolicy {
	policy.QualityProfile = quality
	policy.DirectPlayPolicy = "never"
	policy.DirectStreamPolicy = "never"
	policy.TranscodePolicy = "require"
	policy.AllowHDR = profile.SupportsHDR
	policy.DeliveryProfile = "library-channel-hls"
	policy.ServerClamps = appendUniqueString(policy.ServerClamps, "continuous-linear-timeline")
	policy.LiveHLS = &LiveHLSPlaybackPolicy{AuthorizationTransport: "header_or_secure_http_only_cookie", PlaylistScope: "playback_session", SegmentScope: "playback_session", CredentialQueryAllowed: false}
	policy.LiveDelivery = &PlaybackDeliveryPolicy{DeliveryMode: "server_hls", GrantRequired: true, AllowedOperationClasses: []string{"manifest", "segment"}, AuthorizationRecheckSeconds: 60, QualityProfile: quality, OverlayTranscode: overlay, ResourceRevision: revision}
	return policy
}

func resolvedLibraryChannelQuality(configured string, policy ResolvedPlaybackPolicy) string {
	order := []string{"original", "1080p-high", "1080p-medium", "720p-high", "720p-medium", "480p", "328p"}
	rank := func(value string) int {
		value = normalizeTranscodeQuality(value)
		for index, candidate := range order {
			if value == candidate {
				return index
			}
		}
		return 2
	}
	selected := max(rank(configured), rank(policy.DeliveryProfile))
	if selected == 0 && (policy.MaxVideoBitrateMbps > 0 || policy.MaxVideoHeight > 0) {
		selected = 1
	}
	for selected < len(order)-1 {
		preset, ok := transcodePresets[order[selected]]
		if !ok || ((policy.MaxVideoBitrateMbps <= 0 || preset.videoK <= policy.MaxVideoBitrateMbps*1000) && (policy.MaxVideoHeight <= 0 || preset.height <= policy.MaxVideoHeight)) {
			break
		}
		selected++
	}
	return order[selected]
}

func (s *Server) bindLibraryChannelDeliveryPolicy(ctx context.Context, sessionID, channelID string, user User, policy ResolvedPlaybackPolicy) error {
	if policy.LiveDelivery == nil {
		return errMediaGrantDenied
	}
	encoded, err := json.Marshal(policy.LiveDelivery)
	if err != nil {
		return err
	}
	var valid int
	if err := s.queryUserRow(ctx, `
		SELECT COUNT(*) FROM playback_sessions
		WHERE id = ? AND user_id = ? AND profile_id = ? AND is_live = 1 AND ended_at = ''`,
		sessionID, accountIDForUser(user), viewerProfileID(user)).Scan(&valid); err != nil || valid != 1 {
		return errMediaGrantDenied
	}
	now := time.Now().UTC()
	_, err = s.execUserWrite(ctx, `
		INSERT INTO library_channel_playback_policies (playback_session_id, channel_id, policy_json, resource_revision, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(playback_session_id) DO UPDATE SET
			channel_id = excluded.channel_id, policy_json = excluded.policy_json,
			resource_revision = excluded.resource_revision, created_at = excluded.created_at, expires_at = excluded.expires_at`,
		sessionID, channelID, string(encoded), policy.LiveDelivery.ResourceRevision, now.Format(time.RFC3339), now.Add(mediaGrantTTL).Format(time.RFC3339))
	return err
}

func (s *Server) libraryChannelDeliveryPolicyForRequest(r *http.Request, user User, channelID string) (ResolvedPlaybackPolicy, error) {
	grant := mediaGrantFromRequest(r)
	if !strings.HasPrefix(grant, "ptc_mg_") {
		return ResolvedPlaybackPolicy{}, errMediaGrantDenied
	}
	var encoded string
	var revision int64
	err := s.queryUserRow(r.Context(), `
		SELECT p.policy_json, p.resource_revision
		FROM playback_media_grants g
		JOIN playback_sessions ps ON ps.id = g.playback_session_id
		JOIN library_channel_playback_policies p ON p.playback_session_id = ps.id AND p.channel_id = g.resource_id
		WHERE g.token_hash = ? AND g.resource_kind = 'live_channel' AND g.resource_id = ?
			AND g.principal_user_id = ? AND g.profile_id = ? AND g.revoked_at = ''
			AND ps.ended_at = '' AND ps.state <> 'stopped' AND p.expires_at > ?
		LIMIT 1`, hashToken(grant), channelID, accountIDForUser(user), viewerProfileID(user), time.Now().UTC().Format(time.RFC3339)).Scan(&encoded, &revision)
	if err != nil {
		return ResolvedPlaybackPolicy{}, errMediaGrantDenied
	}
	var delivery PlaybackDeliveryPolicy
	if json.Unmarshal([]byte(encoded), &delivery) != nil || delivery.DeliveryMode != "server_hls" || !delivery.GrantRequired || delivery.ResourceRevision <= 0 || revision != delivery.ResourceRevision || !stringListContains(delivery.AllowedOperationClasses, "manifest") || !stringListContains(delivery.AllowedOperationClasses, "segment") {
		return ResolvedPlaybackPolicy{}, errMediaGrantDenied
	}
	delivery.QualityProfile = normalizeTranscodeQuality(delivery.QualityProfile)
	return ResolvedPlaybackPolicy{
		QualityProfile: delivery.QualityProfile,
		LiveHLS:        &LiveHLSPlaybackPolicy{AuthorizationTransport: "header_or_secure_http_only_cookie", PlaylistScope: "playback_session", SegmentScope: "playback_session", CredentialQueryAllowed: false},
		LiveDelivery:   &delivery,
	}, nil
}

func (s *Server) handleLibraryChannelHLS(w http.ResponseWriter, r *http.Request, user User, channelID, route string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or HEAD for this endpoint.")
		return
	}
	if !canPlayLiveTV(user) || !hasPermission(user, "playMedia") {
		writeProductError(w, http.StatusForbidden, "forbidden", "This profile cannot play Library Channels.")
		return
	}
	switch route {
	case "playlist.m3u8":
		s.handleLibraryChannelHLSManifest(w, r, user, channelID)
	case "segment":
		s.handleLibraryChannelHLSSegment(w, r, user, channelID)
	default:
		writeProductError(w, http.StatusNotFound, "library_channel_stream_not_found", "Library Channel stream resource was not found.")
	}
}

func (s *Server) handleLibraryChannelHLSManifest(w http.ResponseWriter, r *http.Request, user User, channelID string) {
	now := time.Now().UTC().Truncate(time.Second)
	policy, policyErr := s.libraryChannelDeliveryPolicyForRequest(r, user, channelID)
	if policyErr != nil {
		writeProductError(w, http.StatusUnauthorized, "media_grant_denied", "A valid playback media grant is required.")
		return
	}
	store := librarychannels.NewStore(s.dbHandle())
	aggregate, err := store.GetAggregate(r.Context(), channelID)
	if err != nil || !aggregate.Channel.Enabled {
		writeLibraryChannelError(w, firstNonNilError(err, librarychannels.ErrNotFound))
		return
	}
	if policy.LiveDelivery == nil || aggregate.Channel.ConfigRevision != policy.LiveDelivery.ResourceRevision {
		writeProductError(w, http.StatusConflict, "library_channel_playback_policy_stale", "This Library Channel changed. Tune it again to continue.")
		return
	}
	entries, err := store.LoadActiveScheduleForProfile(r.Context(), channelID, now.Add(-16*time.Second), now.Add(64*time.Second), s.libraryChannelAccessDecision(r.Context(), user))
	if err != nil {
		writeLibraryChannelError(w, err)
		return
	}
	segments := libraryChannelHLSPlayableWindow(libraryChannelHLSSegments(entries, now, 3, 12), now, 3, 12)
	if len(segments) == 0 {
		writeProductError(w, http.StatusGone, "library_channel_program_unavailable", "The current Library Channel program is unavailable.")
		return
	}
	quality := policy.DeliveryProfile
	var body strings.Builder
	body.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	body.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", hlsSegmentSeconds))
	body.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", segments[0].StartsAt.Unix()/int64(hlsSegmentSeconds)))
	body.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")
	previousEntry := ""
	for _, segment := range segments {
		if segment.Entry.Kind != librarychannels.EntryMedia || segment.Entry.Availability != "available" {
			break
		}
		if previousEntry != "" && previousEntry != segment.Entry.ID {
			body.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		previousEntry = segment.Entry.ID
		body.WriteString("#EXT-X-PROGRAM-DATE-TIME:" + segment.StartsAt.Format(time.RFC3339Nano) + "\n")
		body.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", segment.Duration.Seconds()))
		body.WriteString(libraryChannelHLSSegmentRoute(channelID, segment, quality) + "\n")
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte(body.String()))
	}
}

func libraryChannelHLSSegments(entries []librarychannels.ScheduleEntry, now time.Time, behind, ahead int) []libraryChannelHLSSegment {
	all := make([]libraryChannelHLSSegment, 0, behind+ahead+4)
	windowStart := now.Add(-time.Duration(behind*hlsSegmentSeconds) * time.Second)
	windowEnd := now.Add(time.Duration(ahead*hlsSegmentSeconds) * time.Second)
	for _, entry := range entries {
		startIndex := max(0, int(windowStart.Sub(entry.StartsAt)/time.Second)/hlsSegmentSeconds)
		for index := startIndex; ; index++ {
			startsAt := entry.StartsAt.Add(time.Duration(index*hlsSegmentSeconds) * time.Second)
			if !startsAt.Before(entry.EndsAt) || !startsAt.Before(windowEnd) {
				break
			}
			endsAt := startsAt.Add(hlsSegmentSeconds * time.Second)
			if endsAt.After(entry.EndsAt) {
				endsAt = entry.EndsAt
			}
			if endsAt.After(windowStart) {
				all = append(all, libraryChannelHLSSegment{Entry: entry, Index: index, StartsAt: startsAt, Duration: endsAt.Sub(startsAt)})
			}
		}
	}
	if len(all) <= behind+ahead {
		return all
	}
	current := 0
	for index, segment := range all {
		if !now.Before(segment.StartsAt) && now.Before(segment.StartsAt.Add(segment.Duration)) {
			current = index
			break
		}
	}
	start := max(0, current-behind)
	end := min(len(all), current+ahead)
	return all[start:end]
}

func libraryChannelHLSPlayableWindow(segments []libraryChannelHLSSegment, now time.Time, behind, ahead int) []libraryChannelHLSSegment {
	current := -1
	for index, segment := range segments {
		if !now.Before(segment.StartsAt) && now.Before(segment.StartsAt.Add(segment.Duration)) {
			current = index
			break
		}
	}
	if current < 0 || segments[current].Entry.Kind != librarychannels.EntryMedia || segments[current].Entry.Availability != "available" {
		return nil
	}
	start := max(0, current-behind)
	for index := start; index <= current; index++ {
		if segments[index].Entry.Kind != librarychannels.EntryMedia || segments[index].Entry.Availability != "available" {
			start = index + 1
		}
	}
	end := min(len(segments), current+ahead)
	for index := current; index < end; index++ {
		if segments[index].Entry.Kind != librarychannels.EntryMedia || segments[index].Entry.Availability != "available" {
			end = index
			break
		}
	}
	if start >= end || start > current {
		return nil
	}
	return segments[start:end]
}

func libraryChannelHLSSegmentRoute(channelID string, segment libraryChannelHLSSegment, quality string) string {
	_ = quality
	query := "at=" + strconv.FormatInt(segment.StartsAt.Unix(), 10)
	return "/api/library-channels/" + urlPathEscape(channelID) + "/hls/segment?" + query
}

func (s *Server) handleLibraryChannelHLSSegment(w http.ResponseWriter, r *http.Request, user User, channelID string) {
	if !validLibraryChannelSegmentQuery(r) {
		writeProductError(w, http.StatusBadRequest, "library_channel_segment_invalid", "Library Channel segment reference contains unsupported parameters.")
		return
	}
	startUnix, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("at")), 10, 64)
	if err != nil {
		writeProductError(w, http.StatusBadRequest, "library_channel_segment_invalid", "Library Channel segment reference is invalid.")
		return
	}
	segmentStart := time.Unix(startUnix, 0).UTC()
	now := time.Now().UTC()
	if segmentStart.Before(now.Add(-16*time.Second)) || segmentStart.After(now.Add(64*time.Second)) {
		writeProductError(w, http.StatusGone, "library_channel_segment_expired", "Library Channel segment is outside the active guide window.")
		return
	}
	policy, policyErr := s.libraryChannelDeliveryPolicyForRequest(r, user, channelID)
	if policyErr != nil {
		writeProductError(w, http.StatusUnauthorized, "media_grant_denied", "A valid playback media grant is required.")
		return
	}
	store := librarychannels.NewStore(s.dbHandle())
	aggregate, aggregateErr := store.GetAggregate(r.Context(), channelID)
	if aggregateErr != nil || !aggregate.Channel.Enabled {
		writeLibraryChannelError(w, firstNonNilError(aggregateErr, librarychannels.ErrNotFound))
		return
	}
	if policy.LiveDelivery == nil || aggregate.Channel.ConfigRevision != policy.LiveDelivery.ResourceRevision {
		writeProductError(w, http.StatusConflict, "library_channel_playback_policy_stale", "This Library Channel changed. Tune it again to continue.")
		return
	}
	entries, loadErr := store.LoadActiveSchedule(r.Context(), channelID, segmentStart, segmentStart.Add(time.Second))
	if loadErr != nil || len(entries) != 1 {
		writeProductError(w, http.StatusGone, "library_channel_segment_stale", "Library Channel schedule changed before this segment was requested.")
		return
	}
	entry := entries[0]
	offsetSeconds := int(segmentStart.Sub(entry.StartsAt).Seconds())
	if offsetSeconds < 0 || offsetSeconds%hlsSegmentSeconds != 0 {
		writeProductError(w, http.StatusBadRequest, "library_channel_segment_invalid", "Library Channel segment reference is not aligned to the active schedule.")
		return
	}
	index := offsetSeconds / hlsSegmentSeconds
	if decision := s.libraryChannelAccessDecision(r.Context(), user)(entry.MediaID); decision != librarychannels.AccessAllowed {
		writeLibraryChannelError(w, librarychannels.ErrProgramRestricted)
		return
	}
	item, mediaErr := s.getMediaPlaybackDetailForUser(r.Context(), user, entry.MediaID)
	if mediaErr != nil {
		writeLibraryChannelError(w, librarychannels.ErrProgramUnavailable)
		return
	}
	quality := policy.DeliveryProfile
	mediaStart := max(0, entry.MediaOffsetSeconds+index*hlsSegmentSeconds)
	if aggregate.Channel.Logo.BugEnabled {
		if !aggregate.Channel.Logo.BugOverheadAccepted {
			writeProductError(w, http.StatusConflict, "library_channel_logo_bug_overhead_required", "The server owner must accept the logo bug transcode overhead before playback.")
			return
		}
		duration := min(hlsSegmentSeconds, max(1, int(entry.EndsAt.Sub(segmentStart).Seconds())))
		path, releaseCachePin, overlayErr := s.libraryChannelOverlaySegment(r.Context(), user, aggregate.Channel, entry, item, quality, mediaStart, duration)
		if overlayErr != nil {
			w.Header().Set("Retry-After", "2")
			writeProductError(w, http.StatusServiceUnavailable, "library_channel_capacity_unavailable", "Library Channel logo rendering capacity is temporarily unavailable.")
			return
		}
		defer releaseCachePin()
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Cache-Control", "private, no-store")
		if r.Method == http.MethodGet {
			http.ServeFile(w, r, path)
		}
		return
	}
	name := fmt.Sprintf("segment_%05d.ts", mediaStart/hlsSegmentSeconds)
	session, transcodeErr := s.ensureTranscodeSessionForSegment(user.ID, item, quality, "", mediaStart, "transcode", "", false, name)
	if transcodeErr != nil {
		w.Header().Set("Retry-After", "2")
		writeProductError(w, http.StatusServiceUnavailable, "library_channel_capacity_unavailable", "Library Channel playback capacity is temporarily unavailable.")
		return
	}
	path := filepath.Join(session.dir, name)
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(session.dir)+string(filepath.Separator)) {
		writeProductError(w, http.StatusBadRequest, "library_channel_segment_invalid", "Library Channel segment reference is invalid.")
		return
	}
	if waitErr := waitForHLSSegmentFileContext(r.Context(), s.shutdownDone(), session, path); waitErr != nil {
		if errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, errLongPollShutdown) {
			return
		}
		w.Header().Set("Retry-After", "2")
		writeProductError(w, http.StatusServiceUnavailable, "library_channel_segment_starting", "Library Channel segment is still being prepared.")
		return
	}
	releaseReader, ok := session.acquireReader()
	if !ok {
		w.Header().Set("Retry-After", "2")
		writeProductError(w, http.StatusServiceUnavailable, "library_channel_segment_starting", "Library Channel segment is moving to a new playback generation. Retry shortly.")
		return
	}
	defer releaseReader()
	s.noteTranscodeSegmentServed(session, name)
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodGet {
		http.ServeFile(w, r, path)
	}
}

func validLibraryChannelSegmentQuery(r *http.Request) bool {
	query := r.URL.Query()
	if len(query["at"]) != 1 || strings.TrimSpace(query.Get("at")) == "" {
		return false
	}
	for key, values := range query {
		if key != "at" {
			return false
		}
		if len(values) != 1 {
			return false
		}
	}
	return true
}

func (s *Server) libraryChannelOverlaySegment(ctx context.Context, user User, channel librarychannels.Channel, entry librarychannels.ScheduleEntry, item MediaItem, quality string, mediaStart, duration int) (path string, release func(), err error) {
	root := filepath.Join(s.cfg.AppDataDir, "library-channel-segment-cache")
	key := fmt.Sprintf("%s:%d:%s:%d:%s", channel.ID, channel.ConfigRevision, entry.ID, mediaStart, quality)
	target := filepath.Join(root, safePathComponent(channel.ID), strconv.FormatInt(channel.ConfigRevision, 10), safePathComponent(entry.ID), fmt.Sprintf("%06d-%s.ts", mediaStart, safePathComponent(quality)))
	if !pathInsideRoot(target, root) {
		return "", nil, errors.New("Library Channel segment cache path is invalid")
	}
	_ = s.maybePruneLibraryChannelSegmentCache(time.Now().UTC(), false)
	releasePin := s.pinLibraryChannelSegmentCachePath(target)
	success := false
	defer func() {
		if !success {
			releasePin()
		}
	}()
	if info, err := os.Stat(target); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
		now := time.Now().UTC()
		_ = os.Chtimes(target, now, now)
		success = true
		return target, releasePin, nil
	}

	for {
		s.libraryChannelPlayoutMu.Lock()
		if s.libraryChannelPlayoutInFlight == nil {
			s.libraryChannelPlayoutInFlight = map[string]chan struct{}{}
		}
		if wait := s.libraryChannelPlayoutInFlight[key]; wait != nil {
			s.libraryChannelPlayoutMu.Unlock()
			select {
			case <-ctx.Done():
				return "", nil, ctx.Err()
			case <-wait:
			}
			if info, err := os.Stat(target); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
				now := time.Now().UTC()
				_ = os.Chtimes(target, now, now)
				success = true
				return target, releasePin, nil
			}
			continue
		}
		wait := make(chan struct{})
		s.libraryChannelPlayoutInFlight[key] = wait
		s.libraryChannelPlayoutMu.Unlock()
		defer func() {
			s.libraryChannelPlayoutMu.Lock()
			delete(s.libraryChannelPlayoutInFlight, key)
			close(wait)
			s.libraryChannelPlayoutMu.Unlock()
		}()
		break
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", nil, err
	}
	if err := ensureMediaWriteCapacity(filepath.Dir(target), mediaWriteMinimumFreeBytes); err != nil {
		return "", nil, err
	}
	logoPath, err := s.libraryChannelOverlayLogoPath(channel)
	if err != nil {
		return "", nil, err
	}
	sourcePath, err := s.sourcePathForHLSTranscode(item)
	if err != nil {
		return "", nil, err
	}
	ffmpegPath := firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg")
	if _, err := exec.LookPath(ffmpegPath); err != nil && filepath.Base(ffmpegPath) == ffmpegPath {
		return "", nil, err
	}
	preset := transcodePresets[quality]
	if quality == "original" {
		preset = sourceEquivalentTranscodePreset(item)
	}
	settings := s.transcodeSettings()
	releaseAdmission, admissionErr := s.acquireLibraryChannelOverlayTranscode(user, channel, quality, key)
	if admissionErr != nil {
		return "", nil, admissionErr
	}
	defer releaseAdmission()
	filter := libraryChannelOverlayFilter(channel.Logo, transcodeVideoFilter(preset, item, settings))
	temporary, err := os.CreateTemp(filepath.Dir(target), ".segment-*.ts")
	if err != nil {
		return "", nil, err
	}
	temporaryPath := temporary.Name()
	_ = temporary.Close()
	defer os.Remove(temporaryPath)
	commandCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	args := []string{"-hide_banner", "-nostdin", "-y", "-protocol_whitelist", transcodeProtocolWhitelist(sourcePath), "-ss", strconv.Itoa(mediaStart), "-t", strconv.Itoa(duration), "-i", sourcePath, "-loop", "1", "-i", logoPath, "-filter_complex", filter, "-map", "[vout]", "-map", "0:a:0?", "-c:v", "libx264"}
	args = append(args, videoEncodingArgs("libx264", preset, settings)...)
	args = append(args, "-c:a", "aac", "-b:a", fmt.Sprintf("%dk", preset.audioK), "-ac", "2", "-ar", "48000")
	args = append(args, normalizedHLSOutputTimestampArgs(mediaStart)...)
	args = append(args, "-f", "mpegts", temporaryPath)
	diagnostics, err := runLibraryChannelOverlayFFmpeg(commandCtx, ffmpegPath, args)
	if err != nil {
		logFields := map[string]string{
			"channel":         channel.ID,
			"entry":           entry.ID,
			"command":         diagnostics.CommandIdentity,
			"error":           diagnostics.Text,
			"stderrBytes":     strconv.FormatInt(diagnostics.Bytes, 10),
			"stderrLines":     strconv.FormatInt(diagnostics.Lines, 10),
			"stderrTruncated": strconv.FormatBool(diagnostics.Truncated),
			"errorLines":      strconv.FormatInt(diagnostics.ErrorLines, 10),
			"exitCode":        strconv.Itoa(diagnostics.ExitCode),
			"signal":          diagnostics.Signal,
		}
		s.recordLog("warn", "Library Channel logo segment failed", logFields)
		return "", nil, err
	}
	if info, err := os.Stat(temporaryPath); err != nil || info.Size() == 0 {
		return "", nil, errors.New("Library Channel logo segment was empty")
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return "", nil, err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return "", nil, err
	}
	s.noteLibraryChannelSegmentCached(target)
	success = true
	return target, releasePin, nil
}

func runLibraryChannelOverlayFFmpeg(ctx context.Context, executable string, args []string) (ffmpegDiagnosticReport, error) {
	diagnosticRecorder := newFFmpegDiagnosticRecorder(executable, args)
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stderr = diagnosticRecorder
	err := cmd.Run()
	return diagnosticRecorder.Report(err), err
}

func (s *Server) acquireLibraryChannelOverlayTranscode(user User, channel librarychannels.Channel, quality, segmentKey string) (func(), error) {
	settings := s.transcodeSettings()
	if !settings.Enabled {
		return nil, errLibraryChannelPlaybackCapacity
	}
	admissionContext, releaseAdmission, admissionErr := s.restoreBarrier.acquire(context.Background())
	if admissionErr != nil || admissionContext.Err() != nil {
		if admissionErr == nil {
			releaseAdmission()
		}
		return nil, errLibraryChannelPlaybackCapacity
	}
	registryKey := "library-overlay:" + segmentKey
	s.transcodeMu.Lock()
	if s.transcodes == nil {
		s.transcodes = map[string]*transcodeSession{}
	}
	if err := s.checkBaseTranscodeAdmissionLocked(settings, accountIDForUser(user), false); err != nil {
		releaseAdmission()
		s.transcodeMu.Unlock()
		return nil, errLibraryChannelPlaybackCapacity
	}
	if !s.softwareEncodeSlotAvailableLocked(settings) {
		s.transcodeRejected.Add(1)
		releaseAdmission()
		s.transcodeMu.Unlock()
		return nil, errLibraryChannelPlaybackCapacity
	}
	releaseResources, ok := s.mediaResourceGovernor().tryAcquire(mediaResourceRequest{class: foundationcontract.WorkClassPlaybackStart, cpu: 1, disk: 1})
	if !ok {
		s.transcodeRejected.Add(1)
		releaseAdmission()
		s.transcodeMu.Unlock()
		return nil, errLibraryChannelPlaybackCapacity
	}
	done := make(chan struct{})
	session := &transcodeSession{
		key: registryKey, userID: accountIDForUser(user), mediaID: channel.ID,
		quality: quality, method: "software-overlay", filter: "Library Channel on-screen logo",
		startedAt: time.Now().UTC(), done: done, updateCh: make(chan struct{}), segmentSeconds: hlsSegmentSeconds,
		admissionActive: true, resourceRelease: releaseResources,
	}
	var once sync.Once
	releaseOverlay := func() {
		once.Do(func() {
			s.transcodeMu.Lock()
			session.stateMu.Lock()
			session.admissionActive = false
			session.signalUpdateLocked()
			session.stateMu.Unlock()
			close(done)
			if s.transcodes[registryKey] == session {
				delete(s.transcodes, registryKey)
			}
			s.transcodeMu.Unlock()
			session.releaseMediaResources()
		})
	}
	// Quiescence may need to stop an overlay before its caller reaches the
	// ordinary cleanup path. Publish the same once-guarded cleanup as the
	// session cancel function before registering it.
	session.cancel = releaseOverlay
	s.transcodes[registryKey] = session
	s.transcodeMu.Unlock()
	// The session is registered before releasing the restore admission lease.
	// quiesceForRestore will cancel and drain it independently; retaining this
	// lease until the overlay caller finishes would deadlock that drain.
	releaseAdmission()
	return releaseOverlay, nil
}

func (s *Server) libraryChannelOverlayLogoPath(channel librarychannels.Channel) (string, error) {
	if channel.Logo.Source == librarychannels.LogoCustom {
		path, _, err := s.libraryChannelCustomLogoPath(channel.Logo.Ref)
		return path, err
	}
	data, ok := builtInLibraryChannelLogoPNG(channel.Logo.Ref)
	if !ok {
		return "", errors.New("Library Channel logo asset is unavailable")
	}
	root := filepath.Join(s.cfg.AppDataDir, "library-channel-overlay-assets")
	target := filepath.Join(root, safePathComponent(channel.Logo.Ref)+".png")
	if !pathInsideRoot(target, root) {
		return "", errors.New("Library Channel logo cache path is invalid")
	}
	s.libraryChannelAssetMu.Lock()
	defer s.libraryChannelAssetMu.Unlock()
	if info, err := os.Stat(target); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
		return target, nil
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(root, ".overlay-*.png")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return "", err
	}
	return target, nil
}

func libraryChannelOverlayFilter(logo librarychannels.LogoConfig, baseFilter string) string {
	width := math.Max(2.0, math.Min(25.0, logo.BugWidthPct)) / 100
	inset := math.Max(0.0, math.Min(15.0, logo.BugInsetPct)) / 100
	treatment := "null"
	if logo.BugTreatment == librarychannels.LogoWhite {
		treatment = "lutrgb=r=255:g=255:b=255"
	} else if logo.BugTreatment == librarychannels.LogoBlack {
		treatment = "lutrgb=r=0:g=0:b=0"
	}
	x := fmt.Sprintf("main_w*%.4f", inset)
	y := fmt.Sprintf("main_h*%.4f", inset)
	if logo.BugCorner == librarychannels.LogoTopRight || logo.BugCorner == librarychannels.LogoBottomRight {
		x = fmt.Sprintf("main_w-overlay_w-main_w*%.4f", inset)
	}
	if logo.BugCorner == librarychannels.LogoBottomLeft || logo.BugCorner == librarychannels.LogoBottomRight {
		y = fmt.Sprintf("main_h-overlay_h-main_h*%.4f", inset)
	}
	return fmt.Sprintf("[0:v]%s[base];[1:v][base]scale2ref=w=main_w*%.4f:h=ow/mdar[logo][video];[logo]%s[bug];[video][bug]overlay=x=%s:y=%s:format=auto[vout]", baseFilter, width, treatment, x, y)
}

func writeLibraryChannelPlaybackError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errPlaybackSessionLimit):
		w.Header().Set("Retry-After", "15")
		writeProductError(w, http.StatusTooManyRequests, "playback_session_limit", "This profile has reached its active playback limit.")
	case errors.Is(err, librarychannels.ErrProgramRestricted), errors.Is(err, librarychannels.ErrProgramUnavailable):
		writeLibraryChannelError(w, err)
	default:
		writeProductError(w, http.StatusServiceUnavailable, "library_channel_playback_unavailable", "Portico could not start Library Channel playback.")
	}
}

func urlPathEscape(value string) string { return strings.ReplaceAll(urlQueryEscape(value), "+", "%20") }
func urlQueryEscape(value string) string {
	return strings.NewReplacer("%", "%25", " ", "%20", "&", "%26", "+", "%2B", "?", "%3F", "#", "%23", "/", "%2F").Replace(value)
}
