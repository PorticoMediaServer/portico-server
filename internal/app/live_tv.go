package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const liveTVLifecycleReapInterval = 10 * time.Second

var errLiveTVSourceHasRecordings = errors.New("Live TV source has retained DVR recordings")
var errLiveTVSourceInUse = errors.New("Live TV source is in use")

// runLiveTVLifecycleReaper makes client abandonment a supervised lifecycle
// transition rather than relying on another HTTP request to discover it. The
// playback expiry path stops the owned transcode generation; allocation pruning
// then fences grants and releases the matching tuner lease idempotently.
func (s *Server) runLiveTVLifecycleReaper(ctx context.Context) {
	ticker := time.NewTicker(liveTVLifecycleReapInterval)
	defer ticker.Stop()
	for {
		now := time.Now().UTC()
		if err := s.expireStalePlaybackSessions(now); err != nil {
			s.log.Warn("Live TV stale playback reaping failed", "error", err)
		}
		if err := s.pruneStaleLiveTVTunerAllocations(ctx); err != nil {
			s.log.Warn("Live TV stale tuner pruning failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

const (
	liveTVMaxJSONBytes     = 28 << 20
	liveTVMaxPlaylistBytes = 6 << 20
	liveTVMaxEPGBytes      = 20 << 20
	liveTVMaxGuideOffset   = 10000
	liveTVImportBatchSize  = 500
	maxLiveTVGuidePrograms = 1000
	liveTVUserAgent        = "Portico/0.1 LiveTV"
)

var m3uAttrPattern = regexp.MustCompile(`([A-Za-z0-9_-]+)="([^"]*)"`)
var sharedLiveTVStreams = newLiveTVStreamHub()

type liveTVViewerProfileContextKey struct{}

func withLiveTVViewerProfile(ctx context.Context, profileID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, liveTVViewerProfileContextKey{}, strings.TrimSpace(profileID))
}

func liveTVViewerProfileFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	profileID, _ := ctx.Value(liveTVViewerProfileContextKey{}).(string)
	return strings.TrimSpace(profileID)
}

func liveTVViewerStateSQL(ctx context.Context) (join, favorite, hidden string, args []any) {
	profileID := liveTVViewerProfileFromContext(ctx)
	if profileID == "" {
		return "", "0", "0", nil
	}
	return " LEFT JOIN live_tv_channel_profile_state cps ON cps.channel_id = c.id AND cps.profile_id = ? ",
		"COALESCE(cps.favorite, 0)", "COALESCE(cps.hidden, 0)", []any{profileID}
}

// Imported guide rows are published by the source's active marker. Keeping the
// fence as a correlated predicate means every reader uses the same authority
// without copying generation state into another cache or API model.
func liveTVChannelGenerationPredicate(alias string) string {
	return alias + ".import_generation = COALESCE((SELECT source_generation.active_import_generation FROM live_tv_sources source_generation WHERE source_generation.id = " + alias + ".source_id), '')"
}

func liveTVProgramGenerationPredicate(alias string) string {
	return alias + ".import_generation = COALESCE((SELECT source_generation.active_import_generation FROM live_tv_sources source_generation WHERE source_generation.id = " + alias + ".source_id), '')"
}

type liveTVSourceRecord struct {
	LiveTVSource
	M3UText        string
	EPGText        string
	XtreamPassword string
}

type liveTVChannelImport struct {
	ID          string
	ProviderKey string
	Number      string
	Name        string
	StreamURL   string
	LogoURL     string
	TVGID       string
	GroupTitle  string
	Country     string
	SortOrder   int
}

type liveTVProgramImport struct {
	ID          string
	ChannelID   string
	ChannelRef  string
	Title       string
	Subtitle    string
	Description string
	Category    string
	StartAt     string
	EndAt       string
	EpisodeNum  string
	IsNew       bool
}

func (s *Server) handleLiveTV(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !canViewLiveTV(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view Live TV.")
		return
	}
	sources, err := s.listLiveTVSourcesForProfile(viewerProfileID(user), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "live_tv_failed", "Unable to load Live TV sources.")
		return
	}
	applyLiveTVSourcesActions(sources, user)
	summaries := make([]LiveTVSourceSummary, 0, len(sources))
	for _, source := range sources {
		summaries = append(summaries, liveTVSourceSummary(source))
	}
	writeJSON(w, http.StatusOK, ListResponse[LiveTVSourceSummary]{Items: summaries, Total: len(summaries)})
}

func (s *Server) handleLiveTVRoute(w http.ResponseWriter, r *http.Request, user User) {
	pathValue := strings.TrimPrefix(r.URL.Path, "/api/live-tv/")
	parts := strings.Split(strings.Trim(pathValue, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "Live TV route was not found.")
		return
	}

	if parts[0] == "sources" {
		s.handleLiveTVSourceRoute(w, r, user, parts[1:])
		return
	}
	if parts[0] == "channels" {
		s.handleLiveTVChannelRoute(w, r, user, parts[1:])
		return
	}
	if len(parts) == 1 && parts[0] == "play" {
		s.handleLiveTVPlaybackStart(w, r, user)
		return
	}
	if parts[0] == "streams" {
		if len(parts) == 1 && r.Method == http.MethodGet {
			s.handleLiveTVStreams(w, r, user)
			return
		}
		if len(parts) == 3 && parts[2] == "open" {
			s.handleLiveTVStreamOpen(w, r, user, parts[1])
			return
		}
		if len(parts) == 3 && parts[2] == "close" {
			s.handleLiveTVStreamClose(w, r, user, parts[1])
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "Live TV stream route was not found.")
		return
	}
	if parts[0] == "logos" {
		if len(parts) != 2 || r.Method != http.MethodGet {
			writeError(w, http.StatusNotFound, "not_found", "Live TV logo route was not found.")
			return
		}
		s.handleLiveTVLogo(w, r, user, parts[1])
		return
	}
	if parts[0] == "hls" {
		s.handleLiveTVHLS(w, r, user, parts[1:])
		return
	}

	writeError(w, http.StatusNotFound, "not_found", "Live TV route was not found.")
}

func (s *Server) handleLiveTVStreams(w http.ResponseWriter, r *http.Request, user User) {
	if !canViewLiveTV(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view Live TV streams.")
		return
	}
	now := time.Now().UTC()
	_ = s.expireStalePlaybackSessions(now)
	streams, err := s.liveTVStreamSessions(user, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "live_tv_streams_failed", "Unable to load Live TV streams.")
		return
	}
	writeJSON(w, http.StatusOK, ListResponse[PlaybackSession]{Items: streams, Total: len(streams)})
}

func (s *Server) handleLiveTVSourceRoute(w http.ResponseWriter, r *http.Request, user User, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			if !canManageLiveTVSources(user) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage Live TV sources.")
				return
			}
			sources, err := s.listLiveTVSourcesForProfile(viewerProfileID(user), true)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "live_tv_sources_failed", "Unable to load Live TV sources.")
				return
			}
			applyLiveTVSourcesActions(sources, user)
			writeJSON(w, http.StatusOK, ListResponse[LiveTVSource]{Items: sources, Total: len(sources)})
		case http.MethodPost:
			if !canManageLiveTVSources(user) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to create Live TV sources.")
				return
			}
			var req LiveTVSourceRequest
			if !decodeJSONLimit(w, r, &req, liveTVMaxJSONBytes) {
				return
			}
			if req.TunerCount != nil && !canInteractivelyManageServer(user) {
				writeProductError(w, http.StatusForbidden, "forbidden", "Only the server owner can set tuner capacity.")
				return
			}
			source, err := s.createLiveTVSource(req)
			if err != nil {
				writeError(w, http.StatusBadRequest, "live_tv_source_invalid", err.Error())
				return
			}
			s.recordLog("info", "Live TV source created", map[string]string{"actor": user.Email, "source": source.Name, "type": source.Type})
			applyLiveTVSourceActions(&source.LiveTVSource, user)
			writeJSON(w, http.StatusCreated, source.LiveTVSource)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
		}
		return
	}

	if len(parts) == 1 && parts[0] == "test-add" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		if !canManageLiveTVSources(user) {
			writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to create Live TV sources.")
			return
		}
		var req LiveTVSourceRequest
		if !decodeJSONLimit(w, r, &req, liveTVMaxJSONBytes) {
			return
		}
		if req.TunerCount != nil && !canInteractivelyManageServer(user) {
			writeProductError(w, http.StatusForbidden, "forbidden", "Only the server owner can set tuner capacity.")
			return
		}
		source, err := s.createLiveTVSourceWithInitialImport(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "live_tv_source_invalid", sanitizeLiveTVError(err))
			return
		}
		s.recordLog("info", "Live TV source tested and created", map[string]string{"actor": user.Email, "source": source.Name, "type": source.Type, "channels": strconv.Itoa(source.ChannelCount)})
		applyLiveTVSourceActions(&source.LiveTVSource, user)
		writeJSON(w, http.StatusCreated, source.LiveTVSource)
		return
	}

	if len(parts) == 2 && parts[0] == "hdhomerun" && parts[1] == "discover" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		if !canManageLiveTVSources(user) {
			writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to discover Live TV tuners.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		candidates, err := s.discoverHDHomeRunDevices(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "hdhomerun_discovery_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ListResponse[HDHomeRunDiscoveryCandidate]{Items: candidates, Total: len(candidates)})
		return
	}

	sourceID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			if !canManageLiveTVSources(user) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage Live TV sources.")
				return
			}
			source, err := s.getLiveTVSourceRecord(sourceID)
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "live_tv_source_not_found", "Live TV source was not found.")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "live_tv_source_failed", "Unable to load the Live TV source.")
				return
			}
			applyLiveTVSourceActions(&source.LiveTVSource, user)
			writeJSON(w, http.StatusOK, source.LiveTVSource)
		case http.MethodPatch:
			if !canManageLiveTVSources(user) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to edit Live TV sources.")
				return
			}
			var req LiveTVSourceRequest
			if !decodeJSONLimit(w, r, &req, liveTVMaxJSONBytes) {
				return
			}
			if req.TunerCount != nil && !canInteractivelyManageServer(user) {
				writeProductError(w, http.StatusForbidden, "forbidden", "Only the server owner can set tuner capacity.")
				return
			}
			source, err := s.updateLiveTVSource(sourceID, req)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "live_tv_source_not_found", "Live TV source was not found.")
					return
				}
				writeError(w, http.StatusBadRequest, "live_tv_source_invalid", err.Error())
				return
			}
			s.recordLog("info", "Live TV source updated", map[string]string{"actor": user.Email, "source": source.Name, "type": source.Type})
			applyLiveTVSourceActions(&source.LiveTVSource, user)
			writeJSON(w, http.StatusOK, source.LiveTVSource)
		case http.MethodDelete:
			if !canManageLiveTVSources(user) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to delete Live TV sources.")
				return
			}
			source, err := s.deleteLiveTVSource(sourceID)
			if err != nil {
				if errors.Is(err, errLiveTVSourceInUse) {
					writeProductError(w, http.StatusConflict, "live_tv_source_in_use", "Live TV is currently using this source. Stop playback and try again.")
					return
				}
				if errors.Is(err, errLiveTVSourceHasRecordings) {
					writeProductError(w, http.StatusConflict, "live_tv_source_has_recordings", "This source has DVR recordings. Disable it to preserve those recordings, or delete the recordings before removing the source.")
					return
				}
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "live_tv_source_not_found", "Live TV source was not found.")
					return
				}
				writeError(w, http.StatusInternalServerError, "live_tv_delete_failed", "Unable to delete Live TV source.")
				return
			}
			s.recordLog("warn", "Live TV source deleted", map[string]string{"actor": user.Email, "source": source.Name})
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET, PATCH, or DELETE for this endpoint.")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "refresh" && r.Method == http.MethodPost {
		if !canManageLiveTVSources(user) {
			writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to refresh Live TV sources.")
			return
		}
		source, err := s.refreshLiveTVSource(r.Context(), sourceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "live_tv_refresh_failed", sanitizeLiveTVError(err))
			return
		}
		s.recordLog("info", "Live TV source refreshed", map[string]string{"actor": user.Email, "source": source.Name, "channels": strconv.Itoa(source.ChannelCount)})
		applyLiveTVSourceActions(&source.LiveTVSource, user)
		writeJSON(w, http.StatusOK, source.LiveTVSource)
		return
	}
	if len(parts) == 2 && parts[1] == "guide" && r.Method == http.MethodGet {
		if !canViewLiveTV(user) {
			writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view Live TV.")
			return
		}
		scope := collectionCursorScope("live-tv-guide", sourceID, r.URL.Query().Get("from"), r.URL.Query().Get("hours"), strings.ToLower(strings.TrimSpace(r.URL.Query().Get("query"))), strings.ToLower(strings.TrimSpace(r.URL.Query().Get("filter"))), strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort"))), strings.ToLower(strings.TrimSpace(r.URL.Query().Get("order"))), strings.ToLower(strings.TrimSpace(r.URL.Query().Get("group"))))
		var after liveTVChannelCursor
		cursorErr := s.decodeCollectionCursor(r, scope, viewerProfileID(user), time.Now().UTC(), &after)
		if cursorErr != nil {
			writeCollectionCursorError(w, cursorErr, "Live TV guide")
			return
		}
		ctx, cancel := context.WithTimeout(withLiveTVViewerProfile(r.Context(), viewerProfileID(user)), navigationRequestTimeout)
		defer cancel()
		guide, err := s.liveTVGuideKeysetContextWithGroup(ctx, user.ID, sourceID, canManageLiveTVSources(user), r.URL.Query().Get("from"), r.URL.Query().Get("hours"), r.URL.Query().Get("limit"), r.URL.Query().Get("query"), r.URL.Query().Get("filter"), r.URL.Query().Get("sort"), r.URL.Query().Get("order"), r.URL.Query().Get("group"), after)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "live_tv_source_not_found", "Live TV source was not found.")
				return
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusServiceUnavailable, "live_tv_guide_timeout", "Live TV guide exceeded the foreground request budget. Try a narrower guide window.")
				return
			}
			writeError(w, http.StatusInternalServerError, "live_tv_guide_failed", "Unable to load Live TV guide.")
			return
		}
		var total *int
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("count")), "exact") {
			total = &guide.TotalChannels
		}
		var next liveTVChannelCursor
		if guide.HasMore && len(guide.Channels) > 0 {
			next, err = s.liveTVGuideCursorForChannel(ctx, sourceID, guide.Channels[len(guide.Channels)-1], guide.From, guide.To, r.URL.Query().Get("sort"))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "live_tv_guide_cursor_failed", "Unable to continue the Live TV guide.")
				return
			}
		}
		guide.PageInfo, err = s.collectionPageInfo(scope, viewerProfileID(user), next, guide.HasMore, total, time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "live_tv_guide_cursor_failed", "Unable to continue the Live TV guide.")
			return
		}
		applyLiveTVGuideActions(&guide, user)
		writeJSON(w, http.StatusOK, guide)
		return
	}
	if len(parts) == 2 && parts[1] == "channels" && r.Method == http.MethodGet {
		if !canViewLiveTV(user) {
			writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view Live TV channels.")
			return
		}
		scope := collectionCursorScope("live-tv-channels", sourceID, strings.ToLower(strings.TrimSpace(r.URL.Query().Get("query"))), strconv.FormatBool(queryBool(r, "favoritesOnly", queryBool(r, "favorites", false))), strings.ToLower(strings.TrimSpace(r.URL.Query().Get("group"))))
		var after liveTVChannelCursor
		cursorErr := s.decodeCollectionCursor(r, scope, viewerProfileID(user), time.Now().UTC(), &after)
		if cursorErr != nil {
			writeCollectionCursorError(w, cursorErr, "Live TV channels")
			return
		}
		source, err := s.getLiveTVSourceRecord(sourceID)
		if err != nil {
			writeError(w, http.StatusNotFound, "live_tv_source_not_found", "Live TV source was not found.")
			return
		}
		if !source.Enabled && !canManageLiveTVSources(user) {
			writeError(w, http.StatusForbidden, "forbidden", "This Live TV source is disabled.")
			return
		}
		limit := clampInt(queryInt(r, "limit", 100), 1, 250)
		ctx, cancel := context.WithTimeout(withLiveTVViewerProfile(r.Context(), viewerProfileID(user)), navigationRequestTimeout)
		defer cancel()
		policy := s.userLiveTVChannelPolicy(user.ID)
		filters := liveTVChannelBrowseFilter{
			Query:         r.URL.Query().Get("query"),
			FavoritesOnly: queryBool(r, "favoritesOnly", queryBool(r, "favorites", false)),
			Group:         r.URL.Query().Get("group"),
		}
		channels, total, hasMore, err := s.listLiveTVChannelsForSourceKeysetPageFilteredContext(ctx, sourceID, limit, after, policy, canManageLiveTVSources(user), filters)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusServiceUnavailable, "live_tv_channels_timeout", "Live TV channels exceeded the foreground request budget. Try a narrower page.")
				return
			}
			writeError(w, http.StatusInternalServerError, "live_tv_channels_failed", "Unable to load Live TV channels.")
			return
		}
		groups, err := s.listLiveTVChannelGroupsContext(ctx, sourceID, policy, canManageLiveTVSources(user))
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusServiceUnavailable, "live_tv_channels_timeout", "Live TV channel groups exceeded the foreground request budget. Try again shortly.")
				return
			}
			writeError(w, http.StatusInternalServerError, "live_tv_channels_failed", "Unable to load Live TV channel groups.")
			return
		}
		if channels == nil {
			channels = make([]LiveTVChannel, 0)
		}
		if groups == nil {
			groups = make([]string, 0)
		}
		applyLiveTVChannelsActions(channels, user)
		var pageTotal *int
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("count")), "exact") {
			pageTotal = &total
		}
		var next liveTVChannelCursor
		if hasMore && len(channels) > 0 {
			last := channels[len(channels)-1]
			next = liveTVChannelCursor{PrimaryNumber: last.SortOrder, SortOrder: last.SortOrder, Name: last.Name, ID: last.ID}
		}
		pageInfo, err := s.collectionPageInfo(scope, viewerProfileID(user), next, hasMore, pageTotal, time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "live_tv_channels_cursor_failed", "Unable to continue Live TV channels.")
			return
		}
		writeJSON(w, http.StatusOK, LiveTVChannelPageResponse{Items: channels, PageInfo: pageInfo, Groups: groups})
		return
	}

	writeError(w, http.StatusNotFound, "not_found", "Live TV source route was not found.")
}

func (s *Server) handleLiveTVChannelRoute(w http.ResponseWriter, r *http.Request, user User, parts []string) {
	if len(parts) != 1 || (r.Method != http.MethodGet && r.Method != http.MethodPatch) {
		writeError(w, http.StatusNotFound, "not_found", "Live TV channel route was not found.")
		return
	}
	if !canViewLiveTV(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view Live TV channels.")
		return
	}
	if r.Method == http.MethodGet {
		channel, err := s.getLiveTVChannelForProfileContext(r.Context(), viewerProfileID(user), parts[0])
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "live_tv_channel_not_found", "Live TV channel was not found.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "live_tv_channel_failed", "Unable to load the Live TV channel.")
			return
		}
		applyLiveTVChannelActions(&channel, user)
		writeJSON(w, http.StatusOK, channel)
		return
	}
	var req LiveTVChannelStateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.GuideChannelRef != nil && !canManageLiveTVSources(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to edit Live TV guide mappings.")
		return
	}
	channel, err := s.updateLiveTVChannelStateForUser(r.Context(), user, parts[0], req)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "live_tv_channel_not_found", "Live TV channel was not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "live_tv_channel_update_failed", "Unable to update Live TV channel.")
		return
	}
	applyLiveTVChannelActions(&channel, user)
	writeJSON(w, http.StatusOK, channel)
}

func (s *Server) listLiveTVSources(includeDisabled bool) ([]LiveTVSource, error) {
	where := ""
	if !includeDisabled {
		where = "WHERE s.enabled = 1"
	}
	rows, err := s.queryUserRead(context.Background(), `
		SELECT
			s.id, s.name, s.type, s.enabled, s.m3u_url, s.m3u_text, s.epg_url, s.epg_text,
			s.xtream_base_url, s.xtream_username, s.xtream_password, s.hdhomerun_base_url, s.user_agent,
			s.stream_buffer_seconds, s.max_retry_seconds, s.refresh_interval_hours,
			s.filter_categories, s.filter_countries, s.filter_require_epg, s.keyword_allow, s.keyword_deny,
			s.tuner_count, s.discovered_tuner_count, s.tuner_count_mode, s.sort_order,
			s.last_refreshed_at, s.last_error, s.created_at, s.updated_at,
			COALESCE(s.channel_count, 0), COALESCE(s.program_count, 0), COALESCE(s.logo_count, 0),
			COALESCE(s.hidden_channel_count, 0), COALESCE(s.favorite_channel_count, 0)
		FROM live_tv_sources s
		`+where+`
		ORDER BY s.sort_order ASC, s.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []LiveTVSource
	for rows.Next() {
		source, err := scanLiveTVSource(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source.LiveTVSource)
	}
	return sources, rows.Err()
}

func (s *Server) listLiveTVSourcesForProfile(profileID string, includeDisabled bool) ([]LiveTVSource, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return nil, errors.New("viewer profile is required")
	}
	where := ""
	if !includeDisabled {
		where = "WHERE s.enabled = 1"
	}
	rows, err := s.queryUserRead(context.Background(), `
		SELECT
			s.id, s.name, s.type, s.enabled, s.m3u_url, s.m3u_text, s.epg_url, s.epg_text,
			s.xtream_base_url, s.xtream_username, s.xtream_password, s.hdhomerun_base_url, s.user_agent,
			s.stream_buffer_seconds, s.max_retry_seconds, s.refresh_interval_hours,
			s.filter_categories, s.filter_countries, s.filter_require_epg, s.keyword_allow, s.keyword_deny,
			s.tuner_count, s.discovered_tuner_count, s.tuner_count_mode, s.sort_order,
			s.last_refreshed_at, s.last_error, s.created_at, s.updated_at,
			COALESCE(s.channel_count, 0), COALESCE(s.program_count, 0), COALESCE(s.logo_count, 0),
			(SELECT COUNT(*) FROM live_tv_channel_profile_state cps JOIN live_tv_channels c ON c.id = cps.channel_id WHERE cps.profile_id = ? AND cps.hidden = 1 AND c.source_id = s.id AND c.enabled = 1 AND `+liveTVChannelGenerationPredicate("c")+`),
			(SELECT COUNT(*) FROM live_tv_channel_profile_state cps JOIN live_tv_channels c ON c.id = cps.channel_id WHERE cps.profile_id = ? AND cps.favorite = 1 AND c.source_id = s.id AND c.enabled = 1 AND `+liveTVChannelGenerationPredicate("c")+`)
		FROM live_tv_sources s
		`+where+`
		ORDER BY s.sort_order ASC, s.name ASC`, profileID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []LiveTVSource
	for rows.Next() {
		source, scanErr := scanLiveTVSource(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		sources = append(sources, source.LiveTVSource)
	}
	return sources, rows.Err()
}

func (s *Server) getLiveTVSourceRecord(id string) (liveTVSourceRecord, error) {
	row := s.queryUserRow(context.Background(), `
		SELECT
			s.id, s.name, s.type, s.enabled, s.m3u_url, s.m3u_text, s.epg_url, s.epg_text,
			s.xtream_base_url, s.xtream_username, s.xtream_password, s.hdhomerun_base_url, s.user_agent,
			s.stream_buffer_seconds, s.max_retry_seconds, s.refresh_interval_hours,
			s.filter_categories, s.filter_countries, s.filter_require_epg, s.keyword_allow, s.keyword_deny,
			s.tuner_count, s.discovered_tuner_count, s.tuner_count_mode, s.sort_order,
			s.last_refreshed_at, s.last_error, s.created_at, s.updated_at,
			COALESCE(s.channel_count, 0), COALESCE(s.program_count, 0), COALESCE(s.logo_count, 0),
			COALESCE(s.hidden_channel_count, 0), COALESCE(s.favorite_channel_count, 0)
		FROM live_tv_sources s
		WHERE s.id = ?`, id)
	source, err := scanLiveTVSource(row)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	source.XtreamPassword, err = s.openLiveTVSourceSecret(source.ID, source.XtreamPassword)
	return source, err
}

type sourceScanner interface {
	Scan(dest ...any) error
}

func scanLiveTVSource(row sourceScanner) (liveTVSourceRecord, error) {
	var source liveTVSourceRecord
	var enabled int
	var m3uText string
	var epgText string
	var password string
	var filterCategories string
	var filterCountries string
	var filterRequireEPG int
	var keywordAllow string
	var keywordDeny string
	err := row.Scan(
		&source.ID, &source.Name, &source.Type, &enabled, &source.M3UURL, &m3uText, &source.EPGURL, &epgText,
		&source.XtreamBaseURL, &source.XtreamUsername, &password, &source.HDHomeRunBaseURL, &source.UserAgent,
		&source.StreamBufferSeconds, &source.MaxRetrySeconds, &source.RefreshIntervalHours,
		&filterCategories, &filterCountries, &filterRequireEPG, &keywordAllow, &keywordDeny,
		&source.TunerCount, &source.DiscoveredTunerCount, &source.TunerCountMode, &source.SortOrder,
		&source.LastRefreshedAt, &source.LastError, &source.CreatedAt, &source.UpdatedAt,
		&source.ChannelCount, &source.ProgramCount, &source.LogoCount, &source.HiddenChannelCount, &source.FavoriteChannelCount,
	)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	source.Enabled = enabled == 1
	source.M3UText = m3uText
	source.EPGText = epgText
	source.XtreamPassword = password
	source.FilterCategories = decodeLiveTVList(filterCategories)
	source.FilterCountries = decodeLiveTVList(filterCountries)
	source.FilterRequireEPG = filterRequireEPG == 1
	source.KeywordAllow = decodeLiveTVList(keywordAllow)
	source.KeywordDeny = decodeLiveTVList(keywordDeny)
	source.HasM3UText = strings.TrimSpace(m3uText) != ""
	source.HasEPGText = strings.TrimSpace(epgText) != ""
	source.HasXtreamPassword = password != ""
	return source, nil
}

func (s *Server) createLiveTVSource(req LiveTVSourceRequest) (liveTVSourceRecord, error) {
	req = s.applyLiveTVSourceCreateDefaults(req)
	clean, err := validateLiveTVSourceRequest(req, nil)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	return s.insertLiveTVSource(clean)
}

func (s *Server) createLiveTVSourceWithInitialImport(ctx context.Context, req LiveTVSourceRequest) (liveTVSourceRecord, error) {
	req = s.applyLiveTVSourceCreateDefaults(req)
	clean, err := validateLiveTVSourceRequest(req, nil)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	clean.ID = randomID("ltvsrc")
	channels, programs, err := s.loadLiveTVSourceImport(ctx, clean)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	source, err := s.insertLiveTVSource(clean)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	if err := s.storeLiveTVImport(source, channels, programs); err != nil {
		_, _ = s.deleteLiveTVSource(source.ID)
		return liveTVSourceRecord{}, err
	}
	return s.getLiveTVSourceRecord(source.ID)
}

type liveTVSourceCreateDefaults struct {
	GuideRefreshIntervalHours int
	FilterRequireEPG          bool
}

func (s *Server) applyLiveTVSourceCreateDefaults(req LiveTVSourceRequest) LiveTVSourceRequest {
	defaults := s.liveTVSourceCreateDefaults()
	if req.RefreshIntervalHours <= 0 {
		req.RefreshIntervalHours = defaults.GuideRefreshIntervalHours
	}
	if req.FilterRequireEPG == nil {
		value := defaults.FilterRequireEPG
		req.FilterRequireEPG = &value
	}
	return req
}

func (s *Server) liveTVSourceCreateDefaults() liveTVSourceCreateDefaults {
	defaults := liveTVSourceCreateDefaults{GuideRefreshIntervalHours: 12}
	settings, err := s.loadSettings()
	if err != nil {
		return defaults
	}
	group, _ := settings["dvr"].(map[string]any)
	defaults.GuideRefreshIntervalHours = boundedInt(settingInt(group, "defaultGuideRefreshIntervalHours", defaults.GuideRefreshIntervalHours), 1, 168, defaults.GuideRefreshIntervalHours)
	defaults.FilterRequireEPG = settingBool(group, "defaultGuideRequireEpg", defaults.FilterRequireEPG)
	return defaults
}

func (s *Server) liveTVGuideChannelAutoMatchEnabled() bool {
	settings, err := s.loadSettings()
	if err != nil {
		return true
	}
	group, _ := settings["dvr"].(map[string]any)
	return settingBool(group, "guideChannelAutoMatch", true)
}

func (s *Server) insertLiveTVSource(clean liveTVSourceRecord) (liveTVSourceRecord, error) {
	sortOrder := 100
	_ = s.queryUserRow(context.Background(), `SELECT COALESCE(MAX(sort_order), 90) + 1 FROM live_tv_sources`).Scan(&sortOrder)
	now := time.Now().UTC().Format(time.RFC3339)
	id := strings.TrimSpace(clean.ID)
	if id == "" {
		id = randomID("ltvsrc")
	}
	sealedPassword, err := s.sealLiveTVSourceSecret(id, clean.XtreamPassword)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	if _, err := s.execUserWrite(context.Background(), `
		INSERT INTO live_tv_sources (
			id, name, type, enabled, m3u_url, m3u_text, epg_url, epg_text, xtream_base_url, xtream_username,
			xtream_password, hdhomerun_base_url, user_agent, stream_buffer_seconds, max_retry_seconds, refresh_interval_hours,
			filter_categories, filter_countries, filter_require_epg, keyword_allow, keyword_deny,
			tuner_count, discovered_tuner_count, tuner_count_mode, sort_order, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, clean.Name, clean.Type, boolInt(clean.Enabled), clean.M3UURL, clean.M3UText, clean.EPGURL, clean.EPGText,
		clean.XtreamBaseURL, clean.XtreamUsername, sealedPassword, clean.HDHomeRunBaseURL, clean.UserAgent, clean.StreamBufferSeconds,
		clean.MaxRetrySeconds, clean.RefreshIntervalHours, encodeLiveTVList(clean.FilterCategories), encodeLiveTVList(clean.FilterCountries),
		boolInt(clean.FilterRequireEPG), encodeLiveTVList(clean.KeywordAllow), encodeLiveTVList(clean.KeywordDeny),
		clean.TunerCount, clean.DiscoveredTunerCount, clean.TunerCountMode, sortOrder, now, now); err != nil {
		return liveTVSourceRecord{}, err
	}
	return s.getLiveTVSourceRecord(id)
}

func (s *Server) updateLiveTVSource(id string, req LiveTVSourceRequest) (liveTVSourceRecord, error) {
	current, err := s.getLiveTVSourceRecord(id)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	clean, err := validateLiveTVSourceRequest(req, &current)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	sealedPassword, err := s.sealLiveTVSourceSecret(id, clean.XtreamPassword)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.withUserTxTagged(context.Background(), []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		result, err := tx.Exec(`
			UPDATE live_tv_sources SET
				name = ?, type = ?, enabled = ?, m3u_url = ?, m3u_text = ?, epg_url = ?, epg_text = ?,
				xtream_base_url = ?, xtream_username = ?, xtream_password = ?, hdhomerun_base_url = ?, user_agent = ?,
				stream_buffer_seconds = ?, max_retry_seconds = ?, refresh_interval_hours = ?,
				filter_categories = ?, filter_countries = ?, filter_require_epg = ?, keyword_allow = ?, keyword_deny = ?,
				tuner_count = ?, discovered_tuner_count = ?, tuner_count_mode = ?,
				updated_at = ?
			WHERE id = ?`,
			clean.Name, clean.Type, boolInt(clean.Enabled), clean.M3UURL, clean.M3UText, clean.EPGURL, clean.EPGText,
			clean.XtreamBaseURL, clean.XtreamUsername, sealedPassword, clean.HDHomeRunBaseURL, clean.UserAgent, clean.StreamBufferSeconds,
			clean.MaxRetrySeconds, clean.RefreshIntervalHours, encodeLiveTVList(clean.FilterCategories), encodeLiveTVList(clean.FilterCountries),
			boolInt(clean.FilterRequireEPG), encodeLiveTVList(clean.KeywordAllow), encodeLiveTVList(clean.KeywordDeny),
			clean.TunerCount, clean.DiscoveredTunerCount, clean.TunerCountMode, now, id)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return sql.ErrNoRows
		}
		return refreshLiveTVChannelSearchTx(tx, id)
	}); err != nil {
		return liveTVSourceRecord{}, err
	}
	return s.getLiveTVSourceRecord(id)
}

func (s *Server) deleteLiveTVSource(id string) (LiveTVSource, error) {
	source, err := s.getLiveTVSourceRecord(id)
	if err != nil {
		return LiveTVSource{}, err
	}
	if err := s.pruneStaleLiveTVTunerAllocations(context.Background()); err != nil {
		return LiveTVSource{}, err
	}
	if err := s.withUserTxTagged(context.Background(), []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		var activeAllocations int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM live_tv_tuner_allocations WHERE source_id = ?`, id).Scan(&activeAllocations); err != nil {
			return err
		}
		if activeAllocations > 0 {
			return errLiveTVSourceInUse
		}
		var retainedRecordings int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM live_tv_recordings WHERE source_id = ?`, id).Scan(&retainedRecordings); err != nil {
			return err
		}
		if retainedRecordings > 0 {
			return errLiveTVSourceHasRecordings
		}
		if _, err := tx.Exec(`DELETE FROM live_tv_channel_search WHERE source_id = ?`, id); err != nil {
			return err
		}
		result, err := tx.Exec(`DELETE FROM live_tv_sources WHERE id = ?`, id)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return sql.ErrNoRows
		}
		return nil
	}); err != nil {
		return LiveTVSource{}, err
	}
	return source.LiveTVSource, nil
}

func validateLiveTVSourceRequest(req LiveTVSourceRequest, current *liveTVSourceRecord) (liveTVSourceRecord, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" && current != nil {
		name = current.Name
	}
	if len(name) < 2 {
		return liveTVSourceRecord{}, errors.New("Source name must be at least two characters.")
	}

	sourceType := strings.ToLower(strings.TrimSpace(req.Type))
	if sourceType == "" && current != nil {
		sourceType = current.Type
	}
	if sourceType != "m3u" && sourceType != "xmltv" && sourceType != "xtream" && sourceType != "hdhomerun" {
		return liveTVSourceRecord{}, errors.New("Live TV source type must be m3u, xmltv, xtream, or hdhomerun.")
	}

	filterRequireEPG := false
	if current != nil {
		filterRequireEPG = current.FilterRequireEPG
	}
	if req.FilterRequireEPG != nil {
		filterRequireEPG = *req.FilterRequireEPG
	}
	refreshIntervalFallback := 12
	tunerCount := 1
	discoveredTunerCount := 0
	tunerCountMode := "default"
	if current != nil {
		refreshIntervalFallback = current.RefreshIntervalHours
		tunerCount = max(1, current.TunerCount)
		discoveredTunerCount = max(0, current.DiscoveredTunerCount)
		tunerCountMode = firstNonEmpty(current.TunerCountMode, "default")
	}
	if req.TunerCount != nil {
		if *req.TunerCount < 0 || *req.TunerCount > 64 {
			return liveTVSourceRecord{}, errors.New("Tuner capacity must be automatic or between 1 and 64.")
		}
		if *req.TunerCount == 0 {
			if discoveredTunerCount > 0 {
				tunerCount = discoveredTunerCount
				tunerCountMode = "discovered"
			} else {
				tunerCount = 1
				tunerCountMode = "default"
			}
		} else {
			tunerCount = *req.TunerCount
			tunerCountMode = "overridden"
		}
	}

	clean := liveTVSourceRecord{
		LiveTVSource: LiveTVSource{
			Name:                 name,
			Type:                 sourceType,
			Enabled:              req.Enabled,
			M3UURL:               strings.TrimSpace(req.M3UURL),
			EPGURL:               strings.TrimSpace(req.EPGURL),
			XtreamBaseURL:        strings.TrimRight(strings.TrimSpace(req.XtreamBaseURL), "/"),
			XtreamUsername:       strings.TrimSpace(req.XtreamUsername),
			HDHomeRunBaseURL:     normalizeHDHomeRunBaseURL(req.HDHomeRunBaseURL),
			UserAgent:            strings.TrimSpace(req.UserAgent),
			StreamBufferSeconds:  boundedInt(req.StreamBufferSeconds, 5, 120, 18),
			MaxRetrySeconds:      boundedInt(req.MaxRetrySeconds, 5, 300, 45),
			RefreshIntervalHours: boundedInt(req.RefreshIntervalHours, 1, 168, refreshIntervalFallback),
			TunerCount:           tunerCount,
			DiscoveredTunerCount: discoveredTunerCount,
			TunerCountMode:       tunerCountMode,
			FilterCategories:     normalizeLiveTVList(req.FilterCategories, 80),
			FilterCountries:      normalizeLiveTVList(req.FilterCountries, 120),
			FilterRequireEPG:     filterRequireEPG,
			KeywordAllow:         normalizeLiveTVList(req.KeywordAllow, 80),
			KeywordDeny:          normalizeLiveTVList(req.KeywordDeny, 80),
		},
		M3UText:        strings.TrimSpace(req.M3UText),
		EPGText:        strings.TrimSpace(req.EPGText),
		XtreamPassword: strings.TrimSpace(req.XtreamPassword),
	}
	if current != nil {
		if req.PreserveM3UText && clean.M3UText == "" {
			clean.M3UText = current.M3UText
		}
		if req.PreserveEPGText && clean.EPGText == "" {
			clean.EPGText = current.EPGText
		}
		if req.PreserveXtreamPassword && clean.XtreamPassword == "" {
			clean.XtreamPassword = current.XtreamPassword
		}
	}

	if clean.M3UURL != "" {
		if _, _, err := approveLiveTVEndpoint(clean.M3UURL, "playlist"); err != nil {
			return liveTVSourceRecord{}, errors.New("M3U playlist URL must be a safe HTTP(S) URL.")
		}
	}
	if clean.EPGURL != "" {
		if _, _, err := approveLiveTVEndpoint(clean.EPGURL, "guide"); err != nil {
			return liveTVSourceRecord{}, errors.New("EPG URL must be a safe HTTP(S) URL.")
		}
	}
	if clean.XtreamBaseURL != "" {
		if _, _, err := approveLiveTVEndpoint(clean.XtreamBaseURL, "xtream-api"); err != nil {
			return liveTVSourceRecord{}, errors.New("Xtream base URL must be a safe HTTP(S) URL.")
		}
	}
	if clean.HDHomeRunBaseURL != "" {
		if _, err := validateHDHomeRunURL(clean.HDHomeRunBaseURL); err != nil {
			return liveTVSourceRecord{}, errors.New("HDHomeRun device URL must be a safe HTTP(S) LAN URL.")
		}
	}
	if len(clean.M3UText) > liveTVMaxPlaylistBytes {
		return liveTVSourceRecord{}, errors.New("Uploaded M3U playlist is too large.")
	}
	if len(clean.EPGText) > liveTVMaxEPGBytes {
		return liveTVSourceRecord{}, errors.New("Uploaded EPG XML is too large.")
	}

	if sourceType == "m3u" && clean.M3UURL == "" && clean.M3UText == "" {
		return liveTVSourceRecord{}, errors.New("M3U sources need a playlist URL or uploaded playlist file.")
	}
	if sourceType == "xmltv" && clean.EPGURL == "" && clean.EPGText == "" {
		return liveTVSourceRecord{}, errors.New("XMLTV guide-only sources need a guide URL or uploaded XMLTV file.")
	}
	if sourceType == "xtream" {
		if clean.XtreamBaseURL == "" || clean.XtreamUsername == "" || clean.XtreamPassword == "" {
			return liveTVSourceRecord{}, errors.New("Xtream sources need a base URL, username, and password.")
		}
	}
	if sourceType == "hdhomerun" && clean.HDHomeRunBaseURL == "" {
		return liveTVSourceRecord{}, errors.New("HDHomeRun sources need the device base URL, for example http://192.168.1.50.")
	}

	clean.HasM3UText = clean.M3UText != ""
	clean.HasEPGText = clean.EPGText != ""
	clean.HasXtreamPassword = clean.XtreamPassword != ""
	return clean, nil
}

func (s *Server) refreshLiveTVSource(ctx context.Context, id string) (liveTVSourceRecord, error) {
	source, err := s.getLiveTVSourceRecord(id)
	if err != nil {
		return liveTVSourceRecord{}, err
	}
	channels, programs, err := s.loadLiveTVSourceImport(ctx, source)
	if err != nil {
		_ = s.markLiveTVSourceError(source.ID, err)
		return liveTVSourceRecord{}, err
	}
	if err := s.storeLiveTVImport(source, channels, programs); err != nil {
		_ = s.markLiveTVSourceError(source.ID, err)
		return liveTVSourceRecord{}, err
	}
	return s.getLiveTVSourceRecord(source.ID)
}

func (s *Server) loadLiveTVSourceImport(ctx context.Context, source liveTVSourceRecord) ([]liveTVChannelImport, []liveTVProgramImport, error) {
	switch source.Type {
	case "m3u":
		playlist, err := s.loadLiveTVText(ctx, source.M3UText, source.M3UURL, liveTVMaxPlaylistBytes, source.UserAgent, source.MaxRetrySeconds)
		if err != nil {
			return nil, nil, err
		}
		var approval *liveTVEndpointApproval
		if source.M3UURL != "" {
			if candidate, _, approvalErr := approveLiveTVEndpoint(source.M3UURL, "playlist-child"); approvalErr == nil {
				approval = &candidate
			}
		}
		channels := parseM3UPlaylistApproved(source.ID, playlist, approval)
		if len(channels) == 0 {
			err := errors.New("No playable HTTP(S) channels were found in that playlist.")
			return nil, nil, err
		}
		epg, err := s.loadLiveTVText(ctx, source.EPGText, source.EPGURL, liveTVMaxEPGBytes, source.UserAgent, source.MaxRetrySeconds)
		var programs []liveTVProgramImport
		if err == nil && strings.TrimSpace(epg) != "" {
			programs = parseXMLTVProgramsWithMappings(source.ID, epg, channels, s.liveTVGuideChannelMappings(source.ID), s.liveTVGuideChannelAutoMatchEnabled())
		}
		return channels, programs, nil
	case "xmltv":
		return s.refreshXMLTVGuideOnlySource(ctx, source)
	case "xtream":
		return s.refreshXtreamSource(ctx, source)
	case "hdhomerun":
		return s.refreshHDHomeRunSource(ctx, source)
	default:
		err := errors.New("Live TV source type is not recognized.")
		return nil, nil, err
	}
}

func (s *Server) queueScheduledLiveTVSourceRefreshes(now time.Time) {
	const pageSize = 200
	overdueSources, queuedSources, alreadyQueuedSources := 0, 0, 0
	defer func() {
		if overdueSources > 0 {
			s.log.Info("live TV refresh scheduler evaluated overdue sources", "overdueSources", overdueSources, "queuedSources", queuedSources, "alreadyQueuedSources", alreadyQueuedSources)
		}
	}()
	lastSortOrder, lastName, lastID := 0, "", ""
	hasCursor := false
	for {
		query := `
			SELECT id, name, refresh_interval_hours, COALESCE(last_refreshed_at, ''), sort_order
			FROM live_tv_sources
			WHERE enabled = 1`
		args := []any{}
		if hasCursor {
			query += ` AND (sort_order > ? OR (sort_order = ? AND (name > ? OR (name = ? AND id > ?))))`
			args = append(args, lastSortOrder, lastSortOrder, lastName, lastName, lastID)
		}
		query += ` ORDER BY sort_order ASC, name ASC, id ASC LIMIT ?`
		args = append(args, pageSize)
		rows, err := s.queryBackgroundRead(context.Background(), query, args...)
		if err != nil {
			s.log.Warn("scheduled live tv refresh lookup failed", "error", err)
			return
		}
		type scheduledSource struct {
			id, name, lastRefreshedAt string
			intervalHours, sortOrder  int
		}
		page := make([]scheduledSource, 0, pageSize)
		for rows.Next() {
			var source scheduledSource
			if err := rows.Scan(&source.id, &source.name, &source.intervalHours, &source.lastRefreshedAt, &source.sortOrder); err != nil {
				_ = rows.Close()
				s.log.Warn("scheduled live tv refresh scan failed", "error", err)
				return
			}
			page = append(page, source)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			s.log.Warn("scheduled live tv refresh rows failed", "error", err)
			return
		}
		_ = rows.Close()
		if len(page) == 0 {
			return
		}
		for _, source := range page {
			intervalHours := max(1, min(168, source.intervalHours))
			if !liveTVSourceRefreshDue(now, source.lastRefreshedAt, intervalHours) {
				continue
			}
			overdueSources++
			if s.jobAlreadyQueuedWithin("live_tv_refresh", "live_tv_source", source.id, intervalHours) {
				alreadyQueuedSources++
				continue
			}
			if _, err := s.createJobFor("live_tv_refresh", fmt.Sprintf("Live TV guide refresh queued for %s.", source.name), "live_tv_source", source.id); err != nil {
				s.log.Warn("scheduled live tv refresh queue failed", "source", source.id, "error", err)
			} else {
				queuedSources++
			}
		}
		if len(page) < pageSize {
			return
		}
		last := page[len(page)-1]
		lastSortOrder, lastName, lastID, hasCursor = last.sortOrder, last.name, last.id, true
	}
}

func liveTVSourceRefreshDue(now time.Time, lastRefreshedAt string, intervalHours int) bool {
	lastRefreshedAt = strings.TrimSpace(lastRefreshedAt)
	if lastRefreshedAt == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, lastRefreshedAt)
	if err != nil {
		return true
	}
	return !last.Add(time.Duration(max(1, intervalHours)) * time.Hour).After(now)
}

func (s *Server) runLiveTVSourceRefresh(ctx context.Context, job Job) {
	sourceID := strings.TrimSpace(job.ResourceID)
	if sourceID == "" {
		_ = s.setJobMessage(job.ID, "failed", 100, "Live TV source refresh failed because the source was missing.")
		return
	}
	if err := ctx.Err(); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		s.recordLog("info", "Live TV source refresh cancelled.", map[string]string{"job": job.ID, "source": sourceID})
		return
	}
	source, err := s.getLiveTVSourceRecord(sourceID)
	if errors.Is(err, sql.ErrNoRows) {
		_ = s.setJobMessage(job.ID, "complete", 100, "Live TV source refresh skipped because the source was removed.")
		return
	}
	if err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		_ = s.setJobMessage(job.ID, "failed", 100, "Live TV source refresh failed: "+err.Error())
		return
	}
	_ = s.setJobMessage(job.ID, "running", 20, "Refreshing Live TV guide data for "+source.Name+".")
	refreshed, err := s.refreshLiveTVSource(ctx, sourceID)
	if err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		_ = s.setJobMessage(job.ID, "failed", 100, "Live TV source refresh failed: "+err.Error())
		return
	}
	message := fmt.Sprintf("Live TV source refresh complete for %s: %d channels, %d programs.", refreshed.Name, refreshed.ChannelCount, refreshed.ProgramCount)
	_ = s.setJobMessage(job.ID, "complete", 100, message)
}

func (s *Server) loadLiveTVText(ctx context.Context, inlineText, remoteURL string, maxBytes int64, userAgent string, retryWindowSeconds int) (string, error) {
	if strings.TrimSpace(inlineText) != "" {
		return inlineText, nil
	}
	if strings.TrimSpace(remoteURL) == "" {
		return "", nil
	}
	fetchCtx, cancel := context.WithTimeout(ctx, liveTVRetryWindow(retryWindowSeconds))
	defer cancel()
	return fetchLiveTVText(fetchCtx, remoteURL, maxBytes, userAgent)
}

func liveTVRetryWindow(seconds int) time.Duration {
	return time.Duration(boundedInt(seconds, 5, 300, 45)) * time.Second
}

func liveTVRetryAttempts(seconds int) int {
	windowSeconds := int(liveTVRetryWindow(seconds) / time.Second)
	return max(1, min(12, (windowSeconds+14)/15))
}

func parseM3UPlaylist(sourceID, text string) []liveTVChannelImport {
	return parseM3UPlaylistApproved(sourceID, text, nil)
}

func parseM3UPlaylistApproved(sourceID, text string, approval *liveTVEndpointApproval) []liveTVChannelImport {
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var channels []liveTVChannelImport
	var attrs map[string]string
	var title string
	lineNumber := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF") {
			attrs, title = parseM3UInfo(line)
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		var err error
		if approval != nil {
			_, err = approval.validateURL(line)
		} else {
			_, err = validateExternalURL(line)
		}
		if err != nil {
			attrs = nil
			title = ""
			continue
		}
		lineNumber++
		name := firstNonEmpty(attrs["tvg-name"], title, fmt.Sprintf("Channel %d", lineNumber))
		tvgID := strings.TrimSpace(attrs["tvg-id"])
		key := firstNonEmpty(tvgID, attrs["channel-id"], name, line)
		channel := liveTVChannelImport{
			ID:          stableLiveTVID("ltvch", sourceID, key),
			ProviderKey: key,
			Number:      firstNonEmpty(attrs["tvg-chno"], attrs["channel-number"], attrs["ch-number"]),
			Name:        name,
			StreamURL:   line,
			LogoURL:     firstNonEmpty(attrs["tvg-logo"], attrs["logo"]),
			TVGID:       tvgID,
			GroupTitle:  firstNonEmpty(attrs["group-title"], attrs["group"]),
			Country:     strings.ToUpper(firstNonEmpty(attrs["tvg-country"], attrs["country"])),
			SortOrder:   lineNumber,
		}
		channels = append(channels, channel)
		attrs = nil
		title = ""
	}
	sort.SliceStable(channels, func(i, j int) bool {
		if channels[i].Number != "" && channels[j].Number != "" {
			return naturalLess(channels[i].Number, channels[j].Number)
		}
		return channels[i].SortOrder < channels[j].SortOrder
	})
	for i := range channels {
		channels[i].SortOrder = i + 1
	}
	return channels
}

func parseM3UInfo(line string) (map[string]string, string) {
	attrs := map[string]string{}
	for _, match := range m3uAttrPattern.FindAllStringSubmatch(line, -1) {
		attrs[strings.ToLower(match[1])] = strings.TrimSpace(match[2])
	}
	title := ""
	if comma := strings.LastIndex(line, ","); comma >= 0 && comma < len(line)-1 {
		title = strings.TrimSpace(line[comma+1:])
	}
	return attrs, title
}

func parseXMLTVPrograms(sourceID, text string, channels []liveTVChannelImport) []liveTVProgramImport {
	return parseXMLTVProgramsWithMappings(sourceID, text, channels, nil, true)
}

func parseXMLTVGuideOnlyImport(sourceID, text string) ([]liveTVChannelImport, []liveTVProgramImport) {
	var tv xmlTV
	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.Strict = false
	if err := decoder.Decode(&tv); err != nil {
		return nil, nil
	}
	channels := xmlTVGuideOnlyChannels(sourceID, tv)
	programs := xmlTVProgramsForTV(sourceID, tv, channels, nil, true)
	return channels, programs
}

func xmlTVGuideOnlyChannels(sourceID string, tv xmlTV) []liveTVChannelImport {
	channels := make([]liveTVChannelImport, 0, len(tv.Channels))
	seen := map[string]bool{}
	for index, row := range tv.Channels {
		ref := strings.TrimSpace(row.ID)
		if ref == "" {
			continue
		}
		normalizedRef := normalizeLiveTVKey(ref)
		if normalizedRef == "" || seen[normalizedRef] {
			continue
		}
		seen[normalizedRef] = true
		name, number := xmlTVChannelNameAndNumber(row)
		channels = append(channels, liveTVChannelImport{
			ID:          stableLiveTVID("ltvch", sourceID, ref),
			ProviderKey: ref,
			Number:      number,
			Name:        firstNonEmpty(name, ref),
			TVGID:       ref,
			SortOrder:   index + 1,
		})
	}
	for _, row := range tv.Programmes {
		ref := strings.TrimSpace(row.Channel)
		normalizedRef := normalizeLiveTVKey(ref)
		if normalizedRef == "" || seen[normalizedRef] {
			continue
		}
		seen[normalizedRef] = true
		channels = append(channels, liveTVChannelImport{
			ID:          stableLiveTVID("ltvch", sourceID, ref),
			ProviderKey: ref,
			Name:        ref,
			TVGID:       ref,
			SortOrder:   len(channels) + 1,
		})
	}
	sort.SliceStable(channels, func(i, j int) bool {
		if channels[i].Number != "" && channels[j].Number != "" {
			return naturalLess(channels[i].Number, channels[j].Number)
		}
		return strings.ToLower(channels[i].Name) < strings.ToLower(channels[j].Name)
	})
	for i := range channels {
		channels[i].SortOrder = i + 1
	}
	return channels
}

func xmlTVChannelNameAndNumber(row xmlTVChannel) (string, string) {
	name := ""
	number := ""
	for _, displayName := range row.DisplayName {
		value := strings.TrimSpace(displayName.Value)
		if value == "" {
			continue
		}
		if number == "" && looksLikeChannelNumber(value) {
			number = value
			continue
		}
		if name == "" {
			name = value
		}
	}
	return name, number
}

func looksLikeChannelNumber(value string) bool {
	if value == "" || len(value) > 12 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && r != '.' && r != '-' {
			return false
		}
	}
	return strings.ContainsAny(value, "0123456789")
}

func parseXMLTVProgramsWithMappings(sourceID, text string, channels []liveTVChannelImport, guideMappings map[string]string, autoMatch bool) []liveTVProgramImport {
	var tv xmlTV
	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.Strict = false
	if err := decoder.Decode(&tv); err != nil {
		return nil
	}
	return xmlTVProgramsForTV(sourceID, tv, channels, guideMappings, autoMatch)
}

func xmlTVProgramsForTV(sourceID string, tv xmlTV, channels []liveTVChannelImport, guideMappings map[string]string, autoMatch bool) []liveTVProgramImport {
	channelIDs := liveTVChannelMatcher(channels, tv.Channels, guideMappings, autoMatch)
	var programs []liveTVProgramImport
	for _, row := range tv.Programmes {
		start, ok := parseXMLTVTime(row.Start)
		if !ok {
			continue
		}
		stop, ok := parseXMLTVTime(row.Stop)
		if !ok || !stop.After(start) {
			stop = start.Add(30 * time.Minute)
		}
		title := firstXMLText(row.Titles)
		if title == "" {
			continue
		}
		channelRef := strings.TrimSpace(row.Channel)
		channelID := channelIDs[normalizeLiveTVKey(channelRef)]
		program := liveTVProgramImport{
			ID:          stableLiveTVID("ltvpg", sourceID, channelRef, start.Format(time.RFC3339), title),
			ChannelID:   channelID,
			ChannelRef:  channelRef,
			Title:       title,
			Subtitle:    firstXMLText(row.SubTitles),
			Description: firstXMLText(row.Descriptions),
			Category:    firstXMLText(row.Categories),
			StartAt:     start.UTC().Format(time.RFC3339),
			EndAt:       stop.UTC().Format(time.RFC3339),
			EpisodeNum:  firstXMLText(row.EpisodeNums),
			IsNew:       row.New != nil,
		}
		programs = append(programs, program)
	}
	sort.SliceStable(programs, func(i, j int) bool {
		if programs[i].StartAt == programs[j].StartAt {
			return programs[i].Title < programs[j].Title
		}
		return programs[i].StartAt < programs[j].StartAt
	})
	return normalizeLiveTVProgramImports(programs)
}

func normalizeLiveTVProgramImports(programs []liveTVProgramImport) []liveTVProgramImport {
	if len(programs) < 2 {
		return programs
	}
	sorted := append([]liveTVProgramImport(nil), programs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		iKey := liveTVProgramImportKey(sorted[i])
		jKey := liveTVProgramImportKey(sorted[j])
		if iKey != jKey {
			return iKey < jKey
		}
		if sorted[i].StartAt == sorted[j].StartAt {
			if sorted[i].EndAt == sorted[j].EndAt {
				return sorted[i].Title < sorted[j].Title
			}
			return sorted[i].EndAt < sorted[j].EndAt
		}
		return sorted[i].StartAt < sorted[j].StartAt
	})

	lastEndByChannel := map[string]time.Time{}
	normalized := make([]liveTVProgramImport, 0, len(sorted))
	for _, program := range sorted {
		key := liveTVProgramImportKey(program)
		start, startErr := time.Parse(time.RFC3339, program.StartAt)
		end, endErr := time.Parse(time.RFC3339, program.EndAt)
		if key == "" || startErr != nil || endErr != nil || !end.After(start) {
			continue
		}
		if lastEnd, ok := lastEndByChannel[key]; ok && start.Before(lastEnd) {
			if !end.After(lastEnd) {
				continue
			}
			start = lastEnd
			program.StartAt = start.Format(time.RFC3339)
		}
		lastEndByChannel[key] = end
		normalized = append(normalized, program)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].StartAt == normalized[j].StartAt {
			return normalized[i].Title < normalized[j].Title
		}
		return normalized[i].StartAt < normalized[j].StartAt
	})
	return normalized
}

func liveTVProgramImportKey(program liveTVProgramImport) string {
	return firstNonEmpty(program.ChannelID, normalizeLiveTVKey(program.ChannelRef))
}

type xmlTV struct {
	Channels   []xmlTVChannel   `xml:"channel"`
	Programmes []xmlTVProgramme `xml:"programme"`
}

type xmlTVChannel struct {
	ID          string    `xml:"id,attr"`
	DisplayName []xmlText `xml:"display-name"`
}

type xmlTVProgramme struct {
	Start        string    `xml:"start,attr"`
	Stop         string    `xml:"stop,attr"`
	Channel      string    `xml:"channel,attr"`
	Titles       []xmlText `xml:"title"`
	SubTitles    []xmlText `xml:"sub-title"`
	Descriptions []xmlText `xml:"desc"`
	Categories   []xmlText `xml:"category"`
	EpisodeNums  []xmlText `xml:"episode-num"`
	New          *struct{} `xml:"new"`
}

type xmlText struct {
	Value string `xml:",chardata"`
}

func liveTVChannelMatcher(channels []liveTVChannelImport, xmlChannels []xmlTVChannel, guideMappings map[string]string, autoMatch bool) map[string]string {
	matches := map[string]string{}
	channelIDs := map[string]bool{}
	for _, channel := range channels {
		channelIDs[channel.ID] = true
		if autoMatch {
			for _, key := range []string{channel.TVGID, channel.Name, channel.Number, channel.ID} {
				if normalized := normalizeLiveTVKey(key); normalized != "" {
					matches[normalized] = channel.ID
				}
			}
		}
	}
	for guideRef, channelID := range guideMappings {
		if !channelIDs[channelID] {
			continue
		}
		if normalized := normalizeLiveTVKey(guideRef); normalized != "" {
			matches[normalized] = channelID
		}
	}
	if !autoMatch {
		return matches
	}
	for _, xmlChannel := range xmlChannels {
		xmlID := normalizeLiveTVKey(xmlChannel.ID)
		if xmlID == "" {
			continue
		}
		if _, ok := matches[xmlID]; ok {
			continue
		}
		for _, name := range xmlChannel.DisplayName {
			if channelID := matches[normalizeLiveTVKey(name.Value)]; channelID != "" {
				matches[xmlID] = channelID
				break
			}
		}
	}
	return matches
}

func firstXMLText(values []xmlText) string {
	for _, value := range values {
		if text := strings.TrimSpace(value.Value); text != "" {
			return text
		}
	}
	return ""
}

func (s *Server) liveTVGuideChannelMappings(sourceID string) map[string]string {
	rows, err := s.queryBackgroundRead(context.Background(), `
		SELECT guide_channel_ref, channel_id
		FROM live_tv_channel_mappings
		WHERE source_id = ?`, strings.TrimSpace(sourceID))
	if err != nil {
		return map[string]string{}
	}
	defer rows.Close()
	mappings := map[string]string{}
	for rows.Next() {
		var guideRef, channelID string
		if err := rows.Scan(&guideRef, &channelID); err != nil {
			return map[string]string{}
		}
		guideRef = strings.TrimSpace(guideRef)
		channelID = strings.TrimSpace(channelID)
		if guideRef != "" && channelID != "" {
			mappings[guideRef] = channelID
		}
	}
	return mappings
}

func parseXMLTVTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	candidates := []string{value}
	if fields := strings.Fields(value); len(fields) >= 2 {
		candidates = append(candidates, fields[0]+" "+fields[1])
		candidates = append(candidates, fields[0])
	}
	for _, candidate := range candidates {
		for _, layout := range []string{"20060102150405 -0700", "20060102150405Z", "20060102150405"} {
			if parsed, err := time.Parse(layout, candidate); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func (s *Server) refreshXMLTVGuideOnlySource(ctx context.Context, source liveTVSourceRecord) ([]liveTVChannelImport, []liveTVProgramImport, error) {
	epg, err := s.loadLiveTVText(ctx, source.EPGText, source.EPGURL, liveTVMaxEPGBytes, source.UserAgent, source.MaxRetrySeconds)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(epg) == "" {
		return nil, nil, errors.New("XMLTV guide-only sources need guide XML to import.")
	}
	channels, programs := parseXMLTVGuideOnlyImport(source.ID, epg)
	if len(channels) == 0 {
		return nil, nil, errors.New("No XMLTV guide channels were found.")
	}
	return channels, programs, nil
}

func (s *Server) refreshXtreamSource(ctx context.Context, source liveTVSourceRecord) ([]liveTVChannelImport, []liveTVProgramImport, error) {
	streamsURL, err := xtreamAPIURL(source, "get_live_streams", nil)
	if err != nil {
		return nil, nil, err
	}
	streamsCtx, cancelStreams := context.WithTimeout(ctx, liveTVRetryWindow(source.MaxRetrySeconds))
	defer cancelStreams()
	streamsText, err := fetchLiveTVText(streamsCtx, streamsURL, liveTVMaxPlaylistBytes, source.UserAgent)
	if err != nil {
		return nil, nil, err
	}
	var rows []xtreamLiveStream
	if err := json.Unmarshal([]byte(streamsText), &rows); err != nil {
		return nil, nil, errors.New("Xtream live stream response was not valid JSON.")
	}
	channels := make([]liveTVChannelImport, 0, len(rows))
	for i, row := range rows {
		name := strings.TrimSpace(row.Name)
		if name == "" {
			name = fmt.Sprintf("Channel %d", i+1)
		}
		streamID := xtreamStreamID(row.StreamID)
		if streamID == "" || streamID == "0" {
			continue
		}
		streamURL := xtreamStreamURL(source, streamID)
		approval, _, approvalErr := approveLiveTVEndpoint(source.XtreamBaseURL, "xtream-stream")
		if approvalErr != nil {
			continue
		}
		if _, err := approval.validateURL(streamURL); err != nil {
			continue
		}
		channels = append(channels, liveTVChannelImport{
			ID:          stableLiveTVID("ltvch", source.ID, firstNonEmpty(row.EPGChannelID, streamID, name)),
			ProviderKey: streamID,
			Number:      firstNonEmpty(strconv.Itoa(row.Num), streamID),
			Name:        name,
			StreamURL:   streamURL,
			LogoURL:     strings.TrimSpace(row.StreamIcon),
			TVGID:       strings.TrimSpace(row.EPGChannelID),
			GroupTitle:  strings.TrimSpace(row.CategoryName),
			Country:     strings.ToUpper(strings.TrimSpace(row.Country)),
			SortOrder:   i + 1,
		})
	}
	if len(channels) == 0 {
		return nil, nil, errors.New("No playable Xtream live channels were returned.")
	}

	var programs []liveTVProgramImport
	epgURL, err := xtreamXMLTVURL(source)
	if err == nil {
		epgCtx, cancelEPG := context.WithTimeout(ctx, liveTVRetryWindow(source.MaxRetrySeconds))
		defer cancelEPG()
		if epgText, epgErr := fetchLiveTVText(epgCtx, epgURL, liveTVMaxEPGBytes, source.UserAgent); epgErr == nil {
			programs = parseXMLTVProgramsWithMappings(source.ID, epgText, channels, s.liveTVGuideChannelMappings(source.ID), s.liveTVGuideChannelAutoMatchEnabled())
		}
	}
	return channels, programs, nil
}

type xtreamLiveStream struct {
	Num          int             `json:"num"`
	Name         string          `json:"name"`
	StreamType   string          `json:"stream_type"`
	StreamID     json.RawMessage `json:"stream_id"`
	StreamIcon   string          `json:"stream_icon"`
	CategoryName string          `json:"category_name"`
	EPGChannelID string          `json:"epg_channel_id"`
	Country      string          `json:"country"`
}

func (s *Server) refreshHDHomeRunSource(ctx context.Context, source liveTVSourceRecord) ([]liveTVChannelImport, []liveTVProgramImport, error) {
	discoverURL, err := hdhomerunURL(source.HDHomeRunBaseURL, "discover.json")
	if err != nil {
		return nil, nil, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	discoverText, err := fetchHDHomeRunText(fetchCtx, discoverURL, 512*1024, source.UserAgent)
	if err != nil {
		return nil, nil, err
	}
	var discover hdhomerunDiscover
	if err := json.Unmarshal([]byte(discoverText), &discover); err != nil {
		return nil, nil, errors.New("HDHomeRun discover response was not valid JSON.")
	}
	if discover.TunerCount > 0 {
		discovered := min(64, discover.TunerCount)
		_, _ = s.execBackgroundWrite(ctx, `
			UPDATE live_tv_sources
			SET discovered_tuner_count = ?,
				tuner_count = CASE WHEN tuner_count_mode = 'overridden' THEN tuner_count ELSE ? END,
				tuner_count_mode = CASE WHEN tuner_count_mode = 'overridden' THEN tuner_count_mode ELSE 'discovered' END,
				updated_at = ?
			WHERE id = ?`, discovered, discovered, time.Now().UTC().Format(time.RFC3339), source.ID)
	}
	lineupURL := strings.TrimSpace(discover.LineupURL)
	if lineupURL == "" {
		lineupURL, err = hdhomerunURL(firstNonEmpty(discover.BaseURL, source.HDHomeRunBaseURL), "lineup.json")
		if err != nil {
			return nil, nil, err
		}
	}
	if _, err := validateHDHomeRunURL(lineupURL); err != nil {
		return nil, nil, errors.New("HDHomeRun lineup URL is not a safe LAN URL.")
	}

	lineupCtx, cancelLineup := context.WithTimeout(ctx, 20*time.Second)
	defer cancelLineup()
	lineupText, err := fetchHDHomeRunText(lineupCtx, lineupURL, liveTVMaxPlaylistBytes, source.UserAgent)
	if err != nil {
		return nil, nil, err
	}
	var rows []hdhomerunLineupChannel
	if err := json.Unmarshal([]byte(lineupText), &rows); err != nil {
		return nil, nil, errors.New("HDHomeRun lineup response was not valid JSON.")
	}
	channels := parseHDHomeRunLineup(source.ID, rows)
	if len(channels) == 0 {
		return nil, nil, errors.New("No playable HDHomeRun channels were returned.")
	}

	var programs []liveTVProgramImport
	epg, err := s.loadLiveTVText(ctx, source.EPGText, source.EPGURL, liveTVMaxEPGBytes, source.UserAgent, source.MaxRetrySeconds)
	if err == nil && strings.TrimSpace(epg) != "" {
		programs = parseXMLTVProgramsWithMappings(source.ID, epg, channels, s.liveTVGuideChannelMappings(source.ID), s.liveTVGuideChannelAutoMatchEnabled())
	}
	return channels, programs, nil
}

type hdhomerunDiscover struct {
	FriendlyName    string `json:"FriendlyName"`
	ModelNumber     string `json:"ModelNumber"`
	FirmwareName    string `json:"FirmwareName"`
	FirmwareVersion string `json:"FirmwareVersion"`
	DeviceID        string `json:"DeviceID"`
	DeviceAuth      string `json:"DeviceAuth"`
	BaseURL         string `json:"BaseURL"`
	LineupURL       string `json:"LineupURL"`
	TunerCount      int    `json:"TunerCount"`
}

type hdhomerunLineupChannel struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	URL         string `json:"URL"`
	HD          int    `json:"HD"`
	DRM         int    `json:"DRM"`
	Favorite    int    `json:"Favorite"`
}

func (s *Server) discoverHDHomeRunDevices(ctx context.Context) ([]HDHomeRunDiscoveryCandidate, error) {
	locations, err := discoverHDHomeRunLocations(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]HDHomeRunDiscoveryCandidate, 0, len(locations))
	seen := map[string]bool{}
	for _, location := range locations {
		baseURL, ok := hdhomerunBaseURLFromSSDP(location)
		if !ok || seen[baseURL] {
			continue
		}
		seen[baseURL] = true
		discoverURL, err := hdhomerunURL(baseURL, "discover.json")
		if err != nil {
			continue
		}
		fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		text, err := fetchHDHomeRunText(fetchCtx, discoverURL, 512*1024, "")
		cancel()
		if err != nil {
			continue
		}
		var discover hdhomerunDiscover
		if err := json.Unmarshal([]byte(text), &discover); err != nil {
			continue
		}
		candidate := hdhomerunDiscoveryCandidate(baseURL, discover)
		if candidate.BaseURL == "" || seen["candidate:"+candidate.BaseURL] {
			continue
		}
		seen["candidate:"+candidate.BaseURL] = true
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return strings.ToLower(candidates[i].Name) < strings.ToLower(candidates[j].Name)
	})
	return candidates, nil
}

func discoverHDHomeRunLocations(ctx context.Context) ([]string, error) {
	addr, err := net.ResolveUDPAddr("udp4", "239.255.255.250:1900")
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	request := strings.Join([]string{
		"M-SEARCH * HTTP/1.1",
		"HOST: 239.255.255.250:1900",
		`MAN: "ssdp:discover"`,
		"MX: 1",
		"ST: ssdp:all",
		"",
		"",
	}, "\r\n")
	if _, err := conn.WriteTo([]byte(request), addr); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(3 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = conn.SetDeadline(deadline)
	locations := []string{}
	seen := map[string]bool{}
	buffer := make([]byte, 8192)
	for {
		n, _, err := conn.ReadFrom(buffer)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return locations, ctx.Err()
			}
			break
		}
		location, ok := hdhomerunSSDPResponseLocation(string(buffer[:n]))
		if ok && !seen[location] {
			seen[location] = true
			locations = append(locations, location)
		}
	}
	return locations, nil
}

func hdhomerunSSDPResponseLocation(response string) (string, bool) {
	headers := strings.Split(strings.ReplaceAll(response, "\r\n", "\n"), "\n")
	isHDHomeRun := false
	location := ""
	for _, header := range headers {
		key, value, ok := strings.Cut(header, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "location":
			location = value
		case "server", "usn", "st":
			if strings.Contains(strings.ToLower(value), "hdhomerun") || strings.Contains(strings.ToLower(value), "silicondust") {
				isHDHomeRun = true
			}
		}
	}
	if location == "" || !isHDHomeRun {
		return "", false
	}
	if _, ok := hdhomerunBaseURLFromSSDP(location); !ok {
		return "", false
	}
	return location, true
}

func hdhomerunBaseURLFromSSDP(location string) (string, bool) {
	parsed, err := validateHDHomeRunURL(location)
	if err != nil {
		return "", false
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = ""
	return strings.TrimRight(parsed.String(), "/"), true
}

func hdhomerunDiscoveryCandidate(fallbackBaseURL string, discover hdhomerunDiscover) HDHomeRunDiscoveryCandidate {
	baseURL := normalizeHDHomeRunBaseURL(firstNonEmpty(discover.BaseURL, fallbackBaseURL))
	if _, err := validateHDHomeRunURL(baseURL); err != nil {
		baseURL = fallbackBaseURL
	}
	lineupURL := strings.TrimSpace(discover.LineupURL)
	if _, err := validateHDHomeRunURL(lineupURL); err != nil {
		lineupURL = ""
	}
	name := firstNonEmpty(discover.FriendlyName, discover.ModelNumber, discover.DeviceID, "HDHomeRun")
	return HDHomeRunDiscoveryCandidate{
		Name:            name,
		BaseURL:         baseURL,
		DeviceID:        strings.TrimSpace(discover.DeviceID),
		ModelNumber:     strings.TrimSpace(discover.ModelNumber),
		FirmwareName:    strings.TrimSpace(discover.FirmwareName),
		FirmwareVersion: strings.TrimSpace(discover.FirmwareVersion),
		LineupURL:       lineupURL,
		TunerCount:      discover.TunerCount,
	}
}

func parseHDHomeRunLineup(sourceID string, rows []hdhomerunLineupChannel) []liveTVChannelImport {
	channels := make([]liveTVChannelImport, 0, len(rows))
	for i, row := range rows {
		if row.DRM != 0 {
			continue
		}
		streamURL := strings.TrimSpace(row.URL)
		if _, err := validateHDHomeRunURL(streamURL); err != nil {
			continue
		}
		name := strings.TrimSpace(row.GuideName)
		if name == "" {
			name = fmt.Sprintf("Channel %d", i+1)
		}
		number := strings.TrimSpace(row.GuideNumber)
		channels = append(channels, liveTVChannelImport{
			ID:          stableLiveTVID("ltvch", sourceID, firstNonEmpty(number, name, streamURL)),
			ProviderKey: firstNonEmpty(number, name),
			Number:      number,
			Name:        name,
			StreamURL:   streamURL,
			TVGID:       firstNonEmpty(number, name),
			GroupTitle:  "HDHomeRun",
			SortOrder:   i + 1,
		})
	}
	sort.SliceStable(channels, func(i, j int) bool {
		if channels[i].Number != "" && channels[j].Number != "" {
			return naturalLess(channels[i].Number, channels[j].Number)
		}
		return channels[i].SortOrder < channels[j].SortOrder
	})
	for i := range channels {
		channels[i].SortOrder = i + 1
	}
	return channels
}

func xtreamAPIURL(source liveTVSourceRecord, action string, extra map[string]string) (string, error) {
	_, base, err := approveLiveTVEndpoint(source.XtreamBaseURL, "xtream-api")
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/player_api.php"
	values := base.Query()
	values.Set("username", source.XtreamUsername)
	values.Set("password", source.XtreamPassword)
	values.Set("action", action)
	for key, value := range extra {
		values.Set(key, value)
	}
	base.RawQuery = values.Encode()
	return base.String(), nil
}

func xtreamXMLTVURL(source liveTVSourceRecord) (string, error) {
	_, base, err := approveLiveTVEndpoint(source.XtreamBaseURL, "xtream-guide")
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/xmltv.php"
	values := base.Query()
	values.Set("username", source.XtreamUsername)
	values.Set("password", source.XtreamPassword)
	base.RawQuery = values.Encode()
	return base.String(), nil
}

func xtreamStreamURL(source liveTVSourceRecord, streamID string) string {
	base := strings.TrimRight(source.XtreamBaseURL, "/")
	return fmt.Sprintf("%s/live/%s/%s/%s.ts", base, url.PathEscape(source.XtreamUsername), url.PathEscape(source.XtreamPassword), url.PathEscape(streamID))
}

func (s *Server) storeLiveTVImport(source liveTVSourceRecord, channels []liveTVChannelImport, programs []liveTVProgramImport) error {
	now := time.Now().UTC().Format(time.RFC3339)
	importGeneration := randomID("epg")
	programs = normalizeLiveTVProgramImports(programs)
	if err := validateLiveTVProgramChannelRefs(channels, programs); err != nil {
		return err
	}
	channels, programs = applyLiveTVSourceFilters(source, channels, programs)
	err := s.withBackgroundTxTagged(context.Background(), []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		// The whole source refresh is one publication transaction. Readers on
		// another SQLite connection see either the prior complete guide or this
		// complete generation; they never observe a channel/program half-refresh.
		var provisionalToStable map[string]string
		var err error
		channels, provisionalToStable, err = resolveLiveTVChannelIdentities(tx, source, channels, now)
		if err != nil {
			return err
		}
		for index := range programs {
			if stableID := provisionalToStable[programs[index].ChannelID]; stableID != "" {
				programs[index].ChannelID = stableID
			}
		}
		for start := 0; start < len(channels); start += liveTVImportBatchSize {
			end := min(start+liveTVImportBatchSize, len(channels))
			if err := storeLiveTVChannelImportBatchTx(tx, source.ID, source.Type, now, channels[start:end], importGeneration); err != nil {
				return err
			}
		}
		if err := deleteStaleLiveTVChannelsTx(tx, source.ID, now); err != nil {
			return err
		}
		// Publish the generation inside the same transaction before rebuilding
		// derived channel search and summary rows. Other connections still see
		// the prior marker until this transaction commits, and a failure rolls
		// the marker back with the imported rows.
		if _, err := tx.Exec(`UPDATE live_tv_sources SET active_import_generation = ? WHERE id = ?`, importGeneration, source.ID); err != nil {
			return err
		}
		if err := bindDVRGuideGenerationTx(tx, source.ID, importGeneration, now); err != nil {
			return err
		}
		if err := refreshLiveTVChannelSearchTx(tx, source.ID); err != nil {
			return err
		}
		for start := 0; start < len(programs); start += liveTVImportBatchSize {
			end := min(start+liveTVImportBatchSize, len(programs))
			if err := storeLiveTVProgramImportBatchTx(tx, source.ID, now, importGeneration, programs[start:end]); err != nil {
				return err
			}
		}
		if err := deleteStaleLiveTVProgramsTx(tx, source.ID, importGeneration); err != nil {
			return err
		}
		if err := refreshLiveTVSourceSummaryTx(tx, source.ID, now); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE live_tv_sources SET last_refreshed_at = ?, last_error = '', updated_at = ? WHERE id = ?`, now, now, source.ID)
		return err
	})
	if err != nil {
		return err
	}
	// Reconcile only after the complete imported generation (including stale
	// program expiry/repair) is committed. Keeping this at the storage boundary
	// covers initial imports, manual refresh, and background refresh uniformly.
	s.reconcileDVRSeriesRulesForSource(source.ID)
	return nil
}

func validateLiveTVProgramChannelRefs(channels []liveTVChannelImport, programs []liveTVProgramImport) error {
	knownChannels := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		if id := strings.TrimSpace(channel.ID); id != "" {
			knownChannels[id] = struct{}{}
		}
	}
	for _, program := range programs {
		channelID := strings.TrimSpace(program.ChannelID)
		if channelID == "" {
			// An unmatched guide row remains channel_ref-only and is discarded by
			// the source filter. It is not a relational reference to validate.
			continue
		}
		if _, ok := knownChannels[channelID]; !ok {
			return fmt.Errorf("live TV program %q references unknown channel %q", strings.TrimSpace(program.ID), channelID)
		}
	}
	return nil
}

func resolveLiveTVChannelIdentities(tx *sql.Tx, source liveTVSourceRecord, channels []liveTVChannelImport, now string) ([]liveTVChannelImport, map[string]string, error) {
	resolved := make([]liveTVChannelImport, len(channels))
	provisionalToStable := make(map[string]string, len(channels))
	claimed := map[string]bool{}
	for index, channel := range channels {
		provisionalID := channel.ID
		providerKey := strings.TrimSpace(channel.ProviderKey)
		if providerKey == "" {
			providerKey = firstNonEmpty(channel.TVGID, channel.Number)
		}
		candidateSet := map[string]bool{}
		conditions := []string{}
		args := []any{source.ID}
		if providerKey != "" {
			conditions = append(conditions, `(provider_kind = ? AND provider_key = ?)`)
			args = append(args, source.Type, providerKey)
		}
		if strings.TrimSpace(channel.StreamURL) != "" {
			conditions = append(conditions, `stream_url = ?`)
			args = append(args, strings.TrimSpace(channel.StreamURL))
		}
		if strings.TrimSpace(channel.TVGID) != "" {
			conditions = append(conditions, `tvg_id = ?`)
			args = append(args, strings.TrimSpace(channel.TVGID))
		}
		if len(conditions) > 0 {
			rows, err := tx.Query(`
				SELECT DISTINCT channel_id
				FROM live_tv_channel_locators
				WHERE source_id = ? AND (`+strings.Join(conditions, ` OR `)+`)
				LIMIT 3`, args...)
			if err != nil {
				return nil, nil, err
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return nil, nil, err
				}
				candidateSet[id] = true
			}
			if err := rows.Close(); err != nil {
				return nil, nil, err
			}
		}

		// A channel number and normalized name together are bounded move evidence.
		// Either alone is too weak because lineup numbers and names are routinely reused.
		if len(candidateSet) == 0 && strings.TrimSpace(channel.Number) != "" && normalizeLiveTVKey(channel.Name) != "" {
			rows, err := tx.Query(`
				SELECT DISTINCT channel_id
				FROM live_tv_channel_locators
				WHERE source_id = ? AND channel_number = ? AND normalized_name = ?
				LIMIT 3`, source.ID, strings.TrimSpace(channel.Number), normalizeLiveTVKey(channel.Name))
			if err != nil {
				return nil, nil, err
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return nil, nil, err
				}
				candidateSet[id] = true
			}
			if err := rows.Close(); err != nil {
				return nil, nil, err
			}
		}

		candidateIDs := make([]string, 0, len(candidateSet))
		for id := range candidateSet {
			candidateIDs = append(candidateIDs, id)
		}
		sort.Strings(candidateIDs)
		stableID := ""
		if len(candidateIDs) > 1 {
			var openSubjectID string
			err := tx.QueryRow(`
				SELECT subject_id
				FROM identity_reconciliation_reviews
				WHERE domain = 'live_tv_channel' AND library_or_source_id = ? AND status = 'open'
				  AND candidate_locator = ? AND evidence_value = ? AND subject_id <> ''
				ORDER BY created_at DESC, id DESC LIMIT 1`, source.ID, channel.StreamURL, providerKey).Scan(&openSubjectID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, nil, err
			}
			if openSubjectID != "" && candidateSet[openSubjectID] && !claimed[openSubjectID] {
				stableID = openSubjectID
			}
		}
		if stableID == "" && len(candidateIDs) == 1 && !claimed[candidateIDs[0]] {
			stableID = candidateIDs[0]
		} else if stableID == "" && len(candidateIDs) > 0 {
			stableID = randomID("ch")
			encoded, _ := json.Marshal(candidateIDs)
			if _, err := tx.Exec(`
				INSERT INTO identity_reconciliation_reviews (
					id, domain, library_or_source_id, subject_id, candidate_locator, evidence_kind,
					evidence_value, candidate_ids_json, created_at
				) VALUES (?, 'live_tv_channel', ?, ?, ?, 'provider_locator_ambiguous', ?, ?, ?)`,
				randomID("idrev"), source.ID, stableID, channel.StreamURL, providerKey, string(encoded), now); err != nil {
				return nil, nil, err
			}
		}
		if stableID == "" {
			stableID = randomID("ch")
		}
		claimed[stableID] = true
		channel.ID = stableID
		channel.ProviderKey = providerKey
		resolved[index] = channel
		provisionalToStable[provisionalID] = stableID
	}
	return resolved, provisionalToStable, nil
}

func (s *Server) deleteStaleLiveTVChannels(sourceID, lastSeenAt string) error {
	return s.withBackgroundTxTagged(context.Background(), []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		return deleteStaleLiveTVChannelsTx(tx, sourceID, lastSeenAt)
	})
}

func (s *Server) deleteStaleLiveTVPrograms(sourceID, importGeneration string) error {
	return s.withBackgroundTxTagged(context.Background(), []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		return deleteStaleLiveTVProgramsTx(tx, sourceID, importGeneration)
	})
}

func deleteStaleLiveTVChannelsTx(tx *sql.Tx, sourceID, lastSeenAt string) error {
	if _, err := tx.Exec(`
		UPDATE live_tv_channels
		SET enabled = 0, updated_at = ?
		WHERE source_id = ? AND last_seen_at <> ?`, lastSeenAt, sourceID, lastSeenAt); err != nil {
		return err
	}
	_, err := tx.Exec(`
		UPDATE live_tv_channel_locators
		SET active = 0
		WHERE source_id = ? AND last_seen_at <> ?`, sourceID, lastSeenAt)
	return err
}

func deleteStaleLiveTVProgramsTx(tx *sql.Tx, sourceID, importGeneration string) error {
	return deleteLiveTVRowsInBatchesTx(tx, `
		DELETE FROM live_tv_programs
		WHERE id IN (
			SELECT id
			FROM live_tv_programs
			WHERE source_id = ? AND import_generation <> ?
			LIMIT ?
		)`, sourceID, importGeneration)
}

func (s *Server) deleteLiveTVRowsInBatches(query string, firstArg string, secondArg string) error {
	return s.withBackgroundTxTagged(context.Background(), []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		return deleteLiveTVRowsInBatchesTx(tx, query, firstArg, secondArg)
	})
}

func deleteLiveTVRowsInBatchesTx(tx *sql.Tx, query string, firstArg string, secondArg string) error {
	for {
		result, err := tx.Exec(query, firstArg, secondArg, liveTVImportBatchSize)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil || affected == 0 {
			return err
		}
		if affected < int64(liveTVImportBatchSize) {
			return nil
		}
	}
}

func (s *Server) storeLiveTVChannelImportBatch(sourceID, providerKind, now string, channels []liveTVChannelImport, importGeneration ...string) (err error) {
	if len(channels) == 0 {
		return nil
	}
	return s.withBackgroundTxTagged(context.Background(), []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		return storeLiveTVChannelImportBatchTx(tx, sourceID, providerKind, now, channels, importGeneration...)
	})
}

func storeLiveTVChannelImportBatchTx(tx *sql.Tx, sourceID, providerKind, now string, channels []liveTVChannelImport, importGeneration ...string) (err error) {
	if len(channels) == 0 {
		return nil
	}
	generation := ""
	if len(importGeneration) > 0 {
		generation = strings.TrimSpace(importGeneration[0])
	}
	stmt, err := tx.Prepare(`
			INSERT INTO live_tv_channels (
				id, source_id, number, name, stream_url, logo_url, tvg_id, group_title,
				country, enabled, favorite, hidden, sort_order, last_seen_at, created_at, updated_at, import_generation
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, 0, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				number = excluded.number,
				name = excluded.name,
				stream_url = excluded.stream_url,
				logo_url = excluded.logo_url,
				tvg_id = excluded.tvg_id,
				group_title = excluded.group_title,
				country = excluded.country,
				enabled = 1,
				sort_order = excluded.sort_order,
				last_seen_at = excluded.last_seen_at,
				import_generation = excluded.import_generation,
				updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, channel := range channels {
		if _, err := stmt.Exec(
			channel.ID, sourceID, channel.Number, channel.Name, channel.StreamURL, channel.LogoURL, channel.TVGID,
			channel.GroupTitle, channel.Country, channel.SortOrder, now, now, now, generation); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE live_tv_channel_locators SET active = 0 WHERE channel_id = ?`, channel.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`
				INSERT INTO live_tv_channel_locators (
					id, channel_id, source_id, provider_kind, provider_key, stream_url,
					tvg_id, channel_number, normalized_name, active, first_seen_at, last_seen_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
				ON CONFLICT(channel_id, provider_kind, provider_key, stream_url, tvg_id, channel_number, normalized_name) DO UPDATE SET
					source_id = excluded.source_id,
					active = 1,
					last_seen_at = excluded.last_seen_at`,
			randomID("chloc"), channel.ID, sourceID, providerKind, channel.ProviderKey,
			channel.StreamURL, channel.TVGID, channel.Number, normalizeLiveTVKey(channel.Name), now, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) storeLiveTVProgramImportBatch(sourceID, now, importGeneration string, programs []liveTVProgramImport) (err error) {
	if len(programs) == 0 {
		return nil
	}
	return s.withBackgroundTxTagged(context.Background(), []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		return storeLiveTVProgramImportBatchTx(tx, sourceID, now, importGeneration, programs)
	})
}

func storeLiveTVProgramImportBatchTx(tx *sql.Tx, sourceID, now, importGeneration string, programs []liveTVProgramImport) (err error) {
	if len(programs) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`
			INSERT INTO live_tv_programs (
				id, source_id, channel_id, channel_ref, title, subtitle, description, category,
				start_at, end_at, episode_num, is_new, created_at, import_generation
			) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				source_id = excluded.source_id,
				channel_id = excluded.channel_id,
				channel_ref = excluded.channel_ref,
				title = excluded.title,
				subtitle = excluded.subtitle,
				description = excluded.description,
				category = excluded.category,
				start_at = excluded.start_at,
				end_at = excluded.end_at,
				episode_num = excluded.episode_num,
				is_new = excluded.is_new,
				import_generation = excluded.import_generation`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, program := range programs {
		if _, err := stmt.Exec(
			program.ID, sourceID, program.ChannelID, program.ChannelRef, program.Title, program.Subtitle,
			program.Description, program.Category, program.StartAt, program.EndAt, program.EpisodeNum, boolInt(program.IsNew), now, importGeneration); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) refreshLiveTVChannelSearch(ctx context.Context, sourceID string) error {
	return s.withBackgroundTxTagged(ctx, []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		return refreshLiveTVChannelSearchTx(tx, sourceID)
	})
}

func refreshLiveTVChannelSearchTx(tx *sql.Tx, sourceID string) error {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return nil
	}
	if _, err := tx.Exec(`DELETE FROM live_tv_channel_search WHERE source_id = ?`, sourceID); err != nil {
		return err
	}
	_, err := tx.Exec(`
		INSERT INTO live_tv_channel_search (
			channel_id, source_id, name, number, tvg_id, group_title, country, source_name
		)
		SELECT
			c.id, c.source_id, c.name, c.number, c.tvg_id, c.group_title, c.country, s.name
		FROM live_tv_channels c
		JOIN live_tv_sources s ON s.id = c.source_id
		WHERE c.source_id = ?
		  AND c.enabled = 1
		  AND `+liveTVChannelGenerationPredicate("c")+`
		  AND s.enabled = 1`, sourceID)
	return err
}

func refreshLiveTVChannelSearchRowTx(tx *sql.Tx, channelID string) error {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil
	}
	if _, err := tx.Exec(`DELETE FROM live_tv_channel_search WHERE channel_id = ?`, channelID); err != nil {
		return err
	}
	_, err := tx.Exec(`
		INSERT INTO live_tv_channel_search (
			channel_id, source_id, name, number, tvg_id, group_title, country, source_name
		)
		SELECT
			c.id, c.source_id, c.name, c.number, c.tvg_id, c.group_title, c.country, s.name
		FROM live_tv_channels c
		JOIN live_tv_sources s ON s.id = c.source_id
		WHERE c.id = ?
		  AND c.enabled = 1
		  AND `+liveTVChannelGenerationPredicate("c")+`
		  AND s.enabled = 1`, channelID)
	return err
}

func refreshLiveTVSourceSummaryTx(tx *sql.Tx, sourceID string, now string) error {
	_, err := tx.Exec(`
		UPDATE live_tv_sources
		SET
			channel_count = (
				SELECT COUNT(*)
				FROM live_tv_channels c
				WHERE c.source_id = live_tv_sources.id AND c.enabled = 1
				  AND `+liveTVChannelGenerationPredicate("c")+`
			),
			program_count = (
				SELECT COUNT(*)
				FROM live_tv_programs p
				WHERE p.source_id = live_tv_sources.id
				  AND `+liveTVProgramGenerationPredicate("p")+`
			),
			logo_count = (
				SELECT COUNT(*)
				FROM live_tv_channels c
				WHERE c.source_id = live_tv_sources.id AND c.enabled = 1 AND trim(COALESCE(c.logo_url, '')) <> ''
				  AND `+liveTVChannelGenerationPredicate("c")+`
			),
			hidden_channel_count = (
				SELECT COUNT(*)
				FROM live_tv_channels c
				WHERE c.source_id = live_tv_sources.id AND c.enabled = 1 AND c.hidden = 1
				  AND `+liveTVChannelGenerationPredicate("c")+`
			),
			favorite_channel_count = (
				SELECT COUNT(*)
				FROM live_tv_channels c
				WHERE c.source_id = live_tv_sources.id AND c.enabled = 1 AND c.favorite = 1
				  AND `+liveTVChannelGenerationPredicate("c")+`
			),
			summary_updated_at = ?
		WHERE id = ?`, now, strings.TrimSpace(sourceID))
	return err
}

func applyLiveTVSourceFilters(source liveTVSourceRecord, channels []liveTVChannelImport, programs []liveTVProgramImport) ([]liveTVChannelImport, []liveTVProgramImport) {
	programCounts := map[string]int{}
	for _, program := range programs {
		if key := liveTVProgramImportKey(program); key != "" {
			programCounts[key]++
		}
	}

	kept := map[string]bool{}
	filtered := make([]liveTVChannelImport, 0, len(channels))
	for _, channel := range channels {
		if !liveTVChannelAllowedBySource(source, channel, programCounts) {
			continue
		}
		channel.SortOrder = len(filtered) + 1
		filtered = append(filtered, channel)
		kept[channel.ID] = true
	}

	programs = normalizeLiveTVProgramImports(programs)
	filteredPrograms := make([]liveTVProgramImport, 0, len(programs))
	for _, program := range programs {
		if kept[program.ChannelID] {
			filteredPrograms = append(filteredPrograms, program)
		}
	}
	return filtered, filteredPrograms
}

func liveTVChannelAllowedBySource(source liveTVSourceRecord, channel liveTVChannelImport, programCounts map[string]int) bool {
	if len(source.FilterCategories) > 0 && !liveTVListContains(source.FilterCategories, channel.GroupTitle) {
		return false
	}
	if len(source.FilterCountries) > 0 && !liveTVListContains(source.FilterCountries, channel.Country) && !liveTVListContains(source.FilterCountries, channel.GroupTitle) {
		return false
	}
	if source.FilterRequireEPG && programCounts[channel.ID] == 0 {
		return false
	}
	searchText := strings.ToLower(strings.Join([]string{channel.Number, channel.Name, channel.TVGID, channel.GroupTitle, channel.Country}, " "))
	if len(source.KeywordAllow) > 0 && !liveTVTextContainsAny(searchText, source.KeywordAllow) {
		return false
	}
	if liveTVTextContainsAny(searchText, source.KeywordDeny) {
		return false
	}
	return true
}

func liveTVListContains(values []string, target string) bool {
	normalized := normalizeLiveTVKey(target)
	if normalized == "" {
		return false
	}
	for _, value := range values {
		if normalizeLiveTVKey(value) == normalized {
			return true
		}
	}
	return false
}

func liveTVTextContainsAny(text string, needles []string) bool {
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func (s *Server) markLiveTVSourceError(sourceID string, refreshErr error) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.execBackgroundWrite(context.Background(), `UPDATE live_tv_sources SET last_error = ?, updated_at = ? WHERE id = ?`, sanitizeLiveTVError(refreshErr), now, sourceID)
	return err
}

func (s *Server) liveTVGuide(userID string, sourceID string, includeDisabled bool, fromValue string, hoursValue string, limitValue string, offsetValue string, queryValue string, filterValue string, sortValue string, orderValue string) (LiveTVGuideResponse, error) {
	return s.liveTVGuideContext(context.Background(), userID, sourceID, includeDisabled, fromValue, hoursValue, limitValue, offsetValue, queryValue, filterValue, sortValue, orderValue)
}

func (s *Server) liveTVGuideContext(ctx context.Context, userID string, sourceID string, includeDisabled bool, fromValue string, hoursValue string, limitValue string, offsetValue string, queryValue string, filterValue string, sortValue string, orderValue string) (LiveTVGuideResponse, error) {
	return s.liveTVGuideContextWithGroup(ctx, userID, sourceID, includeDisabled, fromValue, hoursValue, limitValue, offsetValue, queryValue, filterValue, sortValue, orderValue, "")
}

func (s *Server) liveTVGuideContextWithGroup(ctx context.Context, userID string, sourceID string, includeDisabled bool, fromValue string, hoursValue string, limitValue string, offsetValue string, queryValue string, filterValue string, sortValue string, orderValue string, groupValue string) (LiveTVGuideResponse, error) {
	source, err := s.getLiveTVSourceRecord(sourceID)
	if err != nil {
		return LiveTVGuideResponse{}, err
	}
	if !source.Enabled && !includeDisabled {
		return LiveTVGuideResponse{}, sql.ErrNoRows
	}
	from := time.Now().UTC().Add(-10 * time.Minute).Truncate(10 * time.Minute)
	if strings.TrimSpace(fromValue) != "" {
		if parsed, err := time.Parse(time.RFC3339, fromValue); err == nil {
			from = parsed.UTC()
		}
	}
	hours := boundedIntFromString(hoursValue, 1, 12, 4)
	to := from.Add(time.Duration(hours) * time.Hour)
	pageLimit := boundedIntFromString(limitValue, 1, 200, 80)
	pageOffset := boundedIntFromString(offsetValue, 0, liveTVMaxGuideOffset, 0)

	policy := s.userLiveTVChannelPolicy(userID)
	channels, totalChannels, hasMore, err := s.listLiveTVGuideChannelsPageFilteredContext(ctx, sourceID, pageLimit, pageOffset, policy, false, from, to, queryValue, filterValue, sortValue, orderValue, groupValue)
	if err != nil {
		return LiveTVGuideResponse{}, err
	}
	channelGroups, err := s.listLiveTVChannelGroupsContext(ctx, sourceID, policy, false)
	if err != nil {
		return LiveTVGuideResponse{}, err
	}
	programs, programsTruncated, err := s.listLiveTVProgramsForChannelsPageContext(ctx, sourceID, liveTVChannelIDs(channels), from, to, maxLiveTVGuidePrograms)
	if err != nil {
		return LiveTVGuideResponse{}, err
	}
	programs = normalizeLiveTVGuidePrograms(programs)
	if channels == nil {
		channels = []LiveTVChannel{}
	}
	if programs == nil {
		programs = []LiveTVProgram{}
	}
	return LiveTVGuideResponse{
		Source:            source.LiveTVSource,
		Channels:          channels,
		Programs:          programs,
		ChannelGroups:     channelGroups,
		From:              from.Format(time.RFC3339),
		To:                to.Format(time.RFC3339),
		ServerTime:        time.Now().UTC().Format(time.RFC3339),
		TotalChannels:     totalChannels,
		Limit:             pageLimit,
		Offset:            pageOffset,
		HasMore:           hasMore,
		ProgramsTruncated: programsTruncated,
	}, nil
}

type liveTVChannelCursor struct {
	PrimaryText   string `json:"primaryText,omitempty"`
	PrimaryNumber int    `json:"primaryNumber,omitempty"`
	SortOrder     int    `json:"sortOrder"`
	Name          string `json:"name"`
	ID            string `json:"id"`
	WindowFrom    string `json:"windowFrom,omitempty"`
	WindowTo      string `json:"windowTo,omitempty"`
}

func (s *Server) liveTVGuideKeysetContextWithGroup(ctx context.Context, userID, sourceID string, includeDisabled bool, fromValue, hoursValue, limitValue, queryValue, filterValue, sortValue, orderValue, groupValue string, after liveTVChannelCursor) (LiveTVGuideResponse, error) {
	source, err := s.getLiveTVSourceRecord(sourceID)
	if err != nil {
		return LiveTVGuideResponse{}, err
	}
	if !source.Enabled && !includeDisabled {
		return LiveTVGuideResponse{}, sql.ErrNoRows
	}
	from := time.Now().UTC().Add(-10 * time.Minute).Truncate(10 * time.Minute)
	if strings.TrimSpace(fromValue) != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, fromValue); parseErr == nil {
			from = parsed.UTC()
		}
	}
	hours := boundedIntFromString(hoursValue, 1, 12, 4)
	to := from.Add(time.Duration(hours) * time.Hour)
	if after.WindowFrom != "" || after.WindowTo != "" {
		cursorFrom, fromErr := time.Parse(time.RFC3339, after.WindowFrom)
		cursorTo, toErr := time.Parse(time.RFC3339, after.WindowTo)
		if fromErr != nil || toErr != nil || !cursorTo.After(cursorFrom) {
			return LiveTVGuideResponse{}, errInvalidCursor
		}
		from, to = cursorFrom.UTC(), cursorTo.UTC()
	}
	pageLimit := boundedIntFromString(limitValue, 1, 200, 80)
	policy := s.userLiveTVChannelPolicy(userID)
	channels, totalChannels, hasMore, err := s.listLiveTVGuideChannelsKeysetPageFilteredContext(ctx, sourceID, pageLimit, after, policy, false, from, to, queryValue, filterValue, sortValue, orderValue, groupValue)
	if err != nil {
		return LiveTVGuideResponse{}, err
	}
	channelGroups, err := s.listLiveTVChannelGroupsContext(ctx, sourceID, policy, false)
	if err != nil {
		return LiveTVGuideResponse{}, err
	}
	programs, programsTruncated, err := s.listLiveTVProgramsForChannelsPageContext(ctx, sourceID, liveTVChannelIDs(channels), from, to, maxLiveTVGuidePrograms)
	if err != nil {
		return LiveTVGuideResponse{}, err
	}
	programs = normalizeLiveTVGuidePrograms(programs)
	if channels == nil {
		channels = []LiveTVChannel{}
	}
	if programs == nil {
		programs = []LiveTVProgram{}
	}
	return LiveTVGuideResponse{
		Source: source.LiveTVSource, Channels: channels, Programs: programs, ChannelGroups: channelGroups,
		From: from.Format(time.RFC3339), To: to.Format(time.RFC3339), ServerTime: time.Now().UTC().Format(time.RFC3339),
		TotalChannels: totalChannels, Limit: pageLimit, Offset: 0, HasMore: hasMore, ProgramsTruncated: programsTruncated,
	}, nil
}

func liveTVSourceSummary(source LiveTVSource) LiveTVSourceSummary {
	return LiveTVSourceSummary{
		ID: source.ID, Name: source.Name, Type: source.Type, Enabled: source.Enabled,
		SortOrder: source.SortOrder, ChannelCount: source.ChannelCount, ProgramCount: source.ProgramCount,
		LogoCount: source.LogoCount, HiddenChannelCount: source.HiddenChannelCount,
		FavoriteChannelCount: source.FavoriteChannelCount, LastRefreshedAt: source.LastRefreshedAt,
		Actions: append([]string(nil), source.Actions...),
	}
}

// MarshalJSON keeps the internal/admin source model useful to guide assembly
// while guaranteeing that consumer guide responses contain only the safe
// source descriptor.
func (response LiveTVGuideResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Source            LiveTVSourceSummary     `json:"source"`
		Channels          []LiveTVChannel         `json:"channels"`
		Programs          []LiveTVProgram         `json:"programs"`
		ChannelGroups     []string                `json:"channelGroups"`
		From              string                  `json:"from"`
		To                string                  `json:"to"`
		ServerTime        string                  `json:"serverTime"`
		PageInfo          CursorPageInfo          `json:"pageInfo"`
		ProgramsTruncated bool                    `json:"programsTruncated,omitempty"`
		Capabilities      LiveTVGuideCapabilities `json:"capabilities"`
	}{
		Source: liveTVSourceSummary(response.Source), Channels: response.Channels, Programs: response.Programs,
		ChannelGroups: response.ChannelGroups, From: response.From, To: response.To, ServerTime: response.ServerTime,
		PageInfo: response.PageInfo, ProgramsTruncated: response.ProgramsTruncated, Capabilities: response.Capabilities,
	})
}

func liveTVChannelIDs(channels []LiveTVChannel) []string {
	ids := make([]string, 0, len(channels))
	for _, channel := range channels {
		if strings.TrimSpace(channel.ID) != "" {
			ids = append(ids, channel.ID)
		}
	}
	return ids
}

func (s *Server) listLiveTVChannels(sourceID string) ([]LiveTVChannel, error) {
	return s.listLiveTVChannelsContext(context.Background(), sourceID)
}

func (s *Server) listLiveTVChannelsContext(ctx context.Context, sourceID string) ([]LiveTVChannel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.queryUserRead(ctx, `
		WITH program_counts AS (
			SELECT p.channel_id, COUNT(*) AS program_count
			FROM live_tv_programs p
			WHERE p.source_id = ? AND `+liveTVProgramGenerationPredicate("p")+`
			GROUP BY p.channel_id
		),
		guide_mappings AS (
			SELECT channel_id, MIN(guide_channel_ref) AS guide_channel_ref
			FROM live_tv_channel_mappings
			WHERE source_id = ?
			GROUP BY channel_id
		)
		SELECT
			c.id, c.source_id, c.number, c.name, c.logo_url, c.tvg_id, c.group_title, c.country,
			c.enabled, c.favorite, c.hidden, c.sort_order,
			COALESCE(gm.guide_channel_ref, ''),
			COALESCE(pc.program_count, 0)
		FROM live_tv_channels c
		LEFT JOIN guide_mappings gm ON gm.channel_id = c.id
		LEFT JOIN program_counts pc ON pc.channel_id = c.id
		WHERE c.source_id = ? AND c.enabled = 1 AND `+liveTVChannelGenerationPredicate("c")+`
		ORDER BY c.sort_order ASC, c.name ASC`, sourceID, sourceID, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLiveTVChannelsContext(ctx, rows)
}

func (s *Server) listLiveTVChannelsPage(sourceID string, limit int, offset int) ([]LiveTVChannel, int, bool, error) {
	limit = clampInt(limit, 1, 200)
	offset = clampInt(offset, 0, liveTVMaxGuideOffset)
	rows, err := s.queryUserRead(context.Background(), `
		SELECT
			c.id, c.source_id, c.number, c.name, c.logo_url, c.tvg_id, c.group_title, c.country,
			c.enabled, c.favorite, c.hidden, c.sort_order,
			COALESCE((SELECT m.guide_channel_ref FROM live_tv_channel_mappings m WHERE m.channel_id = c.id LIMIT 1), ''),
			0
		FROM live_tv_channels c
		WHERE c.source_id = ? AND c.enabled = 1 AND c.hidden = 0 AND `+liveTVChannelGenerationPredicate("c")+`
		ORDER BY c.sort_order ASC, c.name ASC
		LIMIT ? OFFSET ?`, sourceID, limit+1, offset)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	channels, err := scanLiveTVChannels(rows)
	if err != nil {
		return nil, 0, false, err
	}
	hasMore := len(channels) > limit
	if hasMore {
		channels = channels[:limit]
	}
	total := offset + len(channels) + boolInt(hasMore)
	return channels, total, hasMore, nil
}

func (s *Server) listLiveTVGuideChannelsPage(sourceID string, limit int, offset int, policy UserChannelPolicy, includeHidden bool, from time.Time, to time.Time, queryValue string, filterValue string, sortValue string, orderValue string) ([]LiveTVChannel, int, bool, error) {
	return s.listLiveTVGuideChannelsPageContext(context.Background(), sourceID, limit, offset, policy, includeHidden, from, to, queryValue, filterValue, sortValue, orderValue)
}

func (s *Server) listLiveTVGuideChannelsPageContext(ctx context.Context, sourceID string, limit int, offset int, policy UserChannelPolicy, includeHidden bool, from time.Time, to time.Time, queryValue string, filterValue string, sortValue string, orderValue string) ([]LiveTVChannel, int, bool, error) {
	return s.listLiveTVGuideChannelsPageFilteredContext(ctx, sourceID, limit, offset, policy, includeHidden, from, to, queryValue, filterValue, sortValue, orderValue, "")
}

func (s *Server) listLiveTVGuideChannelsPageFilteredContext(ctx context.Context, sourceID string, limit int, offset int, policy UserChannelPolicy, includeHidden bool, from time.Time, to time.Time, queryValue string, filterValue string, sortValue string, orderValue string, groupValue string) ([]LiveTVChannel, int, bool, error) {
	limit = clampInt(limit, 1, 200)
	offset = clampInt(offset, 0, liveTVMaxGuideOffset)
	stateJoin, favoriteSQL, hiddenSQL, stateArgs := liveTVViewerStateSQL(ctx)
	where := []string{"c.source_id = ?", "c.enabled = 1", liveTVChannelGenerationPredicate("c")}
	args := []any{sourceID}
	if !includeHidden {
		where = append(where, hiddenSQL+" = 0")
	}
	if len(policy.AllowedChannelIDs) > 0 {
		allowed := normalizePolicyIDList(policy.AllowedChannelIDs)
		if len(allowed) == 0 {
			return []LiveTVChannel{}, offset, false, nil
		}
		where = append(where, "c.id IN ("+sqlPlaceholders(len(allowed))+")")
		for _, id := range allowed {
			args = append(args, id)
		}
	}
	if len(policy.BlockedChannelIDs) > 0 {
		blocked := normalizePolicyIDList(policy.BlockedChannelIDs)
		if len(blocked) > 0 {
			where = append(where, "c.id NOT IN ("+sqlPlaceholders(len(blocked))+")")
			for _, id := range blocked {
				args = append(args, id)
			}
		}
	}
	appendLiveTVGuideTextSearch(&where, &args, from, to, queryValue)
	appendLiveTVGuideFilter(&where, &args, from, to, filterValue, favoriteSQL)
	if group := strings.TrimSpace(groupValue); group != "" && !strings.EqualFold(group, "all") {
		where = append(where, "LOWER(TRIM(COALESCE(c.group_title, ''))) = LOWER(?)")
		args = append(args, truncateStringRunes(group, 160))
	}
	orderSQL, orderArgs := liveTVGuideChannelOrderSQL(from, to, sortValue, orderValue, favoriteSQL)
	args = append(args, orderArgs...)
	args = append(args, limit+1, offset)
	queryArgs := append(append([]any{}, stateArgs...), args...)
	rows, err := s.queryUserRead(ctx, `
		SELECT
			c.id, c.source_id, c.number, c.name, c.logo_url, c.tvg_id, c.group_title, c.country,
			c.enabled, `+favoriteSQL+`, `+hiddenSQL+`, c.sort_order,
			COALESCE((SELECT m.guide_channel_ref FROM live_tv_channel_mappings m WHERE m.channel_id = c.id LIMIT 1), ''),
			0
		FROM live_tv_channels c `+stateJoin+`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY `+orderSQL+`
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	channels, err := scanLiveTVChannels(rows)
	if err != nil {
		return nil, 0, false, err
	}
	hasMore := len(channels) > limit
	if hasMore {
		channels = channels[:limit]
	}
	total := offset + len(channels) + boolInt(hasMore)
	return channels, total, hasMore, nil
}

func liveTVGuideKeysetOrder(from, to time.Time, sortValue, orderValue string, favoriteSQL string) (expression, direction string, expressionArgs []any, textPrimary bool) {
	direction = "ASC"
	if strings.EqualFold(strings.TrimSpace(orderValue), "desc") {
		direction = "DESC"
	}
	switch strings.ToLower(strings.TrimSpace(sortValue)) {
	case "name":
		return "LOWER(c.name)", direction, nil, true
	case "now":
		return "COALESCE((SELECT LOWER(p.title) FROM live_tv_programs p WHERE p.source_id = c.source_id AND p.channel_id = c.id AND " + liveTVProgramGenerationPredicate("p") + " AND p.start_at < ? AND p.end_at > ? ORDER BY p.start_at DESC LIMIT 1), '')", direction, []any{to.Format(time.RFC3339), from.Format(time.RFC3339)}, true
	case "favorites", "favorite":
		return favoriteSQL, direction, nil, false
	default:
		return "c.sort_order", direction, nil, false
	}
}

func (s *Server) listLiveTVGuideChannelsKeysetPageFilteredContext(ctx context.Context, sourceID string, limit int, after liveTVChannelCursor, policy UserChannelPolicy, includeHidden bool, from, to time.Time, queryValue, filterValue, sortValue, orderValue, groupValue string) ([]LiveTVChannel, int, bool, error) {
	limit = clampInt(limit, 1, 200)
	stateJoin, favoriteSQL, hiddenSQL, stateArgs := liveTVViewerStateSQL(ctx)
	where := []string{"c.source_id = ?", "c.enabled = 1", liveTVChannelGenerationPredicate("c")}
	args := []any{sourceID}
	if !includeHidden {
		where = append(where, hiddenSQL+" = 0")
	}
	if len(policy.AllowedChannelIDs) > 0 {
		allowed := normalizePolicyIDList(policy.AllowedChannelIDs)
		if len(allowed) == 0 {
			return []LiveTVChannel{}, 0, false, nil
		}
		where = append(where, "c.id IN ("+sqlPlaceholders(len(allowed))+")")
		for _, id := range allowed {
			args = append(args, id)
		}
	}
	if len(policy.BlockedChannelIDs) > 0 {
		blocked := normalizePolicyIDList(policy.BlockedChannelIDs)
		if len(blocked) > 0 {
			where = append(where, "c.id NOT IN ("+sqlPlaceholders(len(blocked))+")")
			for _, id := range blocked {
				args = append(args, id)
			}
		}
	}
	appendLiveTVGuideTextSearch(&where, &args, from, to, queryValue)
	appendLiveTVGuideFilter(&where, &args, from, to, filterValue, favoriteSQL)
	if group := strings.TrimSpace(groupValue); group != "" && !strings.EqualFold(group, "all") {
		where = append(where, "LOWER(TRIM(COALESCE(c.group_title, ''))) = LOWER(?)")
		args = append(args, truncateStringRunes(group, 160))
	}
	baseWhere := append([]string{}, where...)
	baseArgs := append([]any{}, args...)
	expression, direction, expressionArgs, textPrimary := liveTVGuideKeysetOrder(from, to, sortValue, orderValue, favoriteSQL)
	if after.ID != "" {
		op := ">"
		if direction == "DESC" {
			op = "<"
		}
		where = append(where, "("+expression+" "+op+" ? OR ("+expression+" = ? AND (c.sort_order > ? OR (c.sort_order = ? AND (c.name > ? OR (c.name = ? AND c.id > ?))))))")
		primary := any(after.PrimaryNumber)
		if textPrimary {
			primary = after.PrimaryText
		}
		args = append(args, expressionArgs...)
		args = append(args, primary)
		args = append(args, expressionArgs...)
		args = append(args, primary, after.SortOrder, after.SortOrder, after.Name, after.Name, after.ID)
	}
	orderArgs := append([]any{}, expressionArgs...)
	args = append(args, orderArgs...)
	args = append(args, limit+1)
	queryArgs := append(append([]any{}, stateArgs...), args...)
	rows, err := s.queryUserRead(ctx, `
		SELECT
			c.id, c.source_id, c.number, c.name, c.logo_url, c.tvg_id, c.group_title, c.country,
			c.enabled, `+favoriteSQL+`, `+hiddenSQL+`, c.sort_order,
			COALESCE((SELECT m.guide_channel_ref FROM live_tv_channel_mappings m WHERE m.channel_id = c.id LIMIT 1), ''),
			0
		FROM live_tv_channels c `+stateJoin+`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY `+expression+` `+direction+`, c.sort_order ASC, c.name ASC, c.id ASC
		LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	channels, err := scanLiveTVChannels(rows)
	if err != nil {
		return nil, 0, false, err
	}
	hasMore := len(channels) > limit
	if hasMore {
		channels = channels[:limit]
	}
	var total int
	countArgs := append(append([]any{}, stateArgs...), baseArgs...)
	if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM live_tv_channels c `+stateJoin+` WHERE `+strings.Join(baseWhere, " AND "), countArgs...).Scan(&total); err != nil {
		return nil, 0, false, err
	}
	return channels, total, hasMore, nil
}

func (s *Server) liveTVGuideCursorForChannel(ctx context.Context, sourceID string, channel LiveTVChannel, fromValue, toValue, sortValue string) (liveTVChannelCursor, error) {
	cursor := liveTVChannelCursor{SortOrder: channel.SortOrder, Name: channel.Name, ID: channel.ID, WindowFrom: fromValue, WindowTo: toValue}
	switch strings.ToLower(strings.TrimSpace(sortValue)) {
	case "name":
		cursor.PrimaryText = strings.ToLower(channel.Name)
	case "now":
		from, fromErr := time.Parse(time.RFC3339, fromValue)
		to, toErr := time.Parse(time.RFC3339, toValue)
		if fromErr != nil || toErr != nil {
			return liveTVChannelCursor{}, errInvalidCursor
		}
		if err := s.queryUserRow(ctx, `
			SELECT COALESCE((SELECT LOWER(p.title) FROM live_tv_programs p
				WHERE p.source_id = ? AND p.channel_id = ? AND `+liveTVProgramGenerationPredicate("p")+` AND p.start_at < ? AND p.end_at > ?
				ORDER BY p.start_at DESC LIMIT 1), '')`, sourceID, channel.ID, to.UTC().Format(time.RFC3339), from.UTC().Format(time.RFC3339)).Scan(&cursor.PrimaryText); err != nil {
			return liveTVChannelCursor{}, err
		}
	case "favorites", "favorite":
		cursor.PrimaryNumber = boolInt(channel.Favorite)
	default:
		cursor.PrimaryNumber = channel.SortOrder
	}
	return cursor, nil
}

func (s *Server) listLiveTVChannelsForSourcePage(sourceID string, limit int, offset int, policy UserChannelPolicy, includeHidden bool) ([]LiveTVChannel, int, bool, error) {
	return s.listLiveTVChannelsForSourcePageContext(context.Background(), sourceID, limit, offset, policy, includeHidden)
}

func (s *Server) listLiveTVChannelsForSourcePageContext(ctx context.Context, sourceID string, limit int, offset int, policy UserChannelPolicy, includeHidden bool) ([]LiveTVChannel, int, bool, error) {
	return s.listLiveTVChannelsForSourcePageFilteredContext(ctx, sourceID, limit, offset, policy, includeHidden, liveTVChannelBrowseFilter{})
}

type liveTVChannelBrowseFilter struct {
	Query         string
	FavoritesOnly bool
	Group         string
}

func (s *Server) listLiveTVChannelsForSourcePageFilteredContext(ctx context.Context, sourceID string, limit int, offset int, policy UserChannelPolicy, includeHidden bool, filters liveTVChannelBrowseFilter) ([]LiveTVChannel, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit = clampInt(limit, 1, 250)
	offset = clampInt(offset, 0, liveTVMaxGuideOffset)
	stateJoin, favoriteSQL, hiddenSQL, stateArgs := liveTVViewerStateSQL(ctx)
	where := []string{"c.source_id = ?", "c.enabled = 1", liveTVChannelGenerationPredicate("c")}
	args := []any{sourceID}
	if !includeHidden {
		where = append(where, hiddenSQL+" = 0")
	}
	if len(policy.AllowedChannelIDs) > 0 {
		allowed := normalizePolicyIDList(policy.AllowedChannelIDs)
		if len(allowed) == 0 {
			return []LiveTVChannel{}, offset, false, nil
		}
		where = append(where, "c.id IN ("+sqlPlaceholders(len(allowed))+")")
		for _, id := range allowed {
			args = append(args, id)
		}
	}
	if len(policy.BlockedChannelIDs) > 0 {
		blocked := normalizePolicyIDList(policy.BlockedChannelIDs)
		if len(blocked) > 0 {
			where = append(where, "c.id NOT IN ("+sqlPlaceholders(len(blocked))+")")
			for _, id := range blocked {
				args = append(args, id)
			}
		}
	}
	if query := strings.ToLower(strings.TrimSpace(truncateStringRunes(filters.Query, 160))); query != "" {
		where = append(where, liveTVGuideChannelTextSQL()+" LIKE ?")
		args = append(args, "%"+query+"%")
	}
	if filters.FavoritesOnly {
		where = append(where, favoriteSQL+" = 1")
	}
	if group := strings.TrimSpace(filters.Group); group != "" && !strings.EqualFold(group, "all") {
		where = append(where, "LOWER(TRIM(COALESCE(c.group_title, ''))) = LOWER(?)")
		args = append(args, truncateStringRunes(group, 160))
	}
	queryArgs := []any{sourceID, sourceID}
	queryArgs = append(queryArgs, stateArgs...)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, limit+1, offset)
	rows, err := s.queryUserRead(ctx, `
		WITH program_counts AS (
			SELECT p.channel_id, COUNT(*) AS program_count
			FROM live_tv_programs p
			WHERE p.source_id = ? AND `+liveTVProgramGenerationPredicate("p")+`
			GROUP BY p.channel_id
		),
		guide_mappings AS (
			SELECT channel_id, MIN(guide_channel_ref) AS guide_channel_ref
			FROM live_tv_channel_mappings
			WHERE source_id = ?
			GROUP BY channel_id
		)
		SELECT
			c.id, c.source_id, c.number, c.name, c.logo_url, c.tvg_id, c.group_title, c.country,
			c.enabled, `+favoriteSQL+`, `+hiddenSQL+`, c.sort_order,
			COALESCE(gm.guide_channel_ref, ''),
			COALESCE(pc.program_count, 0)
		FROM live_tv_channels c `+stateJoin+`
		LEFT JOIN guide_mappings gm ON gm.channel_id = c.id
		LEFT JOIN program_counts pc ON pc.channel_id = c.id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY c.sort_order ASC, c.name ASC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	channels, err := scanLiveTVChannelsContext(ctx, rows)
	if err != nil {
		return nil, 0, false, err
	}
	hasMore := len(channels) > limit
	if hasMore {
		channels = channels[:limit]
	}
	total := offset + len(channels) + boolInt(hasMore)
	return channels, total, hasMore, nil
}

func (s *Server) listLiveTVChannelsForSourceKeysetPageFilteredContext(ctx context.Context, sourceID string, limit int, after liveTVChannelCursor, policy UserChannelPolicy, includeHidden bool, filters liveTVChannelBrowseFilter) ([]LiveTVChannel, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit = clampInt(limit, 1, 250)
	stateJoin, favoriteSQL, hiddenSQL, stateArgs := liveTVViewerStateSQL(ctx)
	where := []string{"c.source_id = ?", "c.enabled = 1", liveTVChannelGenerationPredicate("c")}
	args := []any{sourceID}
	if !includeHidden {
		where = append(where, hiddenSQL+" = 0")
	}
	if len(policy.AllowedChannelIDs) > 0 {
		allowed := normalizePolicyIDList(policy.AllowedChannelIDs)
		if len(allowed) == 0 {
			return []LiveTVChannel{}, 0, false, nil
		}
		where = append(where, "c.id IN ("+sqlPlaceholders(len(allowed))+")")
		for _, id := range allowed {
			args = append(args, id)
		}
	}
	if len(policy.BlockedChannelIDs) > 0 {
		blocked := normalizePolicyIDList(policy.BlockedChannelIDs)
		if len(blocked) > 0 {
			where = append(where, "c.id NOT IN ("+sqlPlaceholders(len(blocked))+")")
			for _, id := range blocked {
				args = append(args, id)
			}
		}
	}
	if query := strings.ToLower(strings.TrimSpace(truncateStringRunes(filters.Query, 160))); query != "" {
		where = append(where, liveTVGuideChannelTextSQL()+" LIKE ?")
		args = append(args, "%"+query+"%")
	}
	if filters.FavoritesOnly {
		where = append(where, favoriteSQL+" = 1")
	}
	if group := strings.TrimSpace(filters.Group); group != "" && !strings.EqualFold(group, "all") {
		where = append(where, "LOWER(TRIM(COALESCE(c.group_title, ''))) = LOWER(?)")
		args = append(args, truncateStringRunes(group, 160))
	}
	baseWhere := append([]string{}, where...)
	baseArgs := append([]any{}, args...)
	if after.ID != "" {
		where = append(where, "(c.sort_order > ? OR (c.sort_order = ? AND (c.name > ? OR (c.name = ? AND c.id > ?))))")
		args = append(args, after.PrimaryNumber, after.PrimaryNumber, after.Name, after.Name, after.ID)
	}
	queryArgs := []any{sourceID, sourceID}
	queryArgs = append(queryArgs, stateArgs...)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, limit+1)
	rows, err := s.queryUserRead(ctx, `
		WITH program_counts AS (
			SELECT p.channel_id, COUNT(*) AS program_count FROM live_tv_programs p WHERE p.source_id = ? AND `+liveTVProgramGenerationPredicate("p")+` GROUP BY p.channel_id
		), guide_mappings AS (
			SELECT channel_id, MIN(guide_channel_ref) AS guide_channel_ref FROM live_tv_channel_mappings WHERE source_id = ? GROUP BY channel_id
		)
		SELECT
			c.id, c.source_id, c.number, c.name, c.logo_url, c.tvg_id, c.group_title, c.country,
			c.enabled, `+favoriteSQL+`, `+hiddenSQL+`, c.sort_order,
			COALESCE(gm.guide_channel_ref, ''), COALESCE(pc.program_count, 0)
		FROM live_tv_channels c `+stateJoin+`
		LEFT JOIN guide_mappings gm ON gm.channel_id = c.id
		LEFT JOIN program_counts pc ON pc.channel_id = c.id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY c.sort_order ASC, c.name ASC, c.id ASC
		LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	channels, err := scanLiveTVChannelsContext(ctx, rows)
	if err != nil {
		return nil, 0, false, err
	}
	hasMore := len(channels) > limit
	if hasMore {
		channels = channels[:limit]
	}
	var total int
	countArgs := append(append([]any{}, stateArgs...), baseArgs...)
	if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM live_tv_channels c `+stateJoin+` WHERE `+strings.Join(baseWhere, " AND "), countArgs...).Scan(&total); err != nil {
		return nil, 0, false, err
	}
	return channels, total, hasMore, nil
}

func (s *Server) listLiveTVChannelGroupsContext(ctx context.Context, sourceID string, policy UserChannelPolicy, includeHidden bool) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	stateJoin, _, hiddenSQL, stateArgs := liveTVViewerStateSQL(ctx)
	where := []string{"c.source_id = ?", "c.enabled = 1", liveTVChannelGenerationPredicate("c"), "TRIM(COALESCE(c.group_title, '')) <> ''"}
	args := []any{sourceID}
	if !includeHidden {
		where = append(where, hiddenSQL+" = 0")
	}
	if len(policy.AllowedChannelIDs) > 0 {
		allowed := normalizePolicyIDList(policy.AllowedChannelIDs)
		if len(allowed) == 0 {
			return []string{}, nil
		}
		where = append(where, "c.id IN ("+sqlPlaceholders(len(allowed))+")")
		for _, id := range allowed {
			args = append(args, id)
		}
	}
	if len(policy.BlockedChannelIDs) > 0 {
		blocked := normalizePolicyIDList(policy.BlockedChannelIDs)
		if len(blocked) > 0 {
			where = append(where, "c.id NOT IN ("+sqlPlaceholders(len(blocked))+")")
			for _, id := range blocked {
				args = append(args, id)
			}
		}
	}
	queryArgs := append(append([]any{}, stateArgs...), args...)
	rows, err := s.queryUserRead(ctx, `
		SELECT DISTINCT TRIM(c.group_title)
		FROM live_tv_channels c `+stateJoin+`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY LOWER(TRIM(c.group_title)) ASC, TRIM(c.group_title) ASC`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []string{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var group string
		if err := rows.Scan(&group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func appendLiveTVGuideTextSearch(where *[]string, args *[]any, from time.Time, to time.Time, queryValue string) {
	query := strings.ToLower(strings.TrimSpace(queryValue))
	if query == "" {
		return
	}
	match := mediaSearchQuery(query)
	if match == "" {
		return
	}
	pattern := "%" + query + "%"
	*where = append(*where, "(c.id IN (SELECT channel_id FROM live_tv_channel_search WHERE live_tv_channel_search MATCH ? AND source_id = c.source_id) OR "+liveTVGuideProgramExistsSQL("LOWER(COALESCE(p.title, '') || ' ' || COALESCE(p.category, '') || ' ' || COALESCE(p.channel_ref, '')) LIKE ?")+")")
	*args = append(*args, match, to.Format(time.RFC3339), from.Format(time.RFC3339), pattern)
}

func appendLiveTVGuideFilter(where *[]string, args *[]any, from time.Time, to time.Time, filterValue string, favoriteSQL string) {
	switch strings.ToLower(strings.TrimSpace(filterValue)) {
	case "", "all":
		return
	case "favorites", "favorite":
		*where = append(*where, favoriteSQL+" = 1")
	case "hd":
		appendLiveTVGuideTerms(where, args, from, to, []string{"hd", "1080", "720", "uhd", "4k"})
	case "sports":
		appendLiveTVGuideTerms(where, args, from, to, []string{"sport", "nfl", "nba", "nhl", "mlb", "soccer", "football", "hockey", "baseball", "basketball", "tennis", "golf"})
	case "news":
		appendLiveTVGuideTerms(where, args, from, to, []string{"news", "weather", "business", "world"})
	case "movies":
		appendLiveTVGuideTerms(where, args, from, to, []string{"movie", "movies", "film", "cinema"})
	}
}

func appendLiveTVGuideTerms(where *[]string, args *[]any, from time.Time, to time.Time, terms []string) {
	if len(terms) == 0 {
		return
	}
	channelText := liveTVGuideChannelTextSQL()
	programText := "LOWER(COALESCE(p.title, '') || ' ' || COALESCE(p.category, ''))"
	channelPredicates := make([]string, 0, len(terms))
	programPredicates := make([]string, 0, len(terms))
	channelArgs := make([]any, 0, len(terms))
	programArgs := make([]any, 0, len(terms))
	for _, term := range terms {
		pattern := "%" + strings.ToLower(strings.TrimSpace(term)) + "%"
		if pattern == "%%" {
			continue
		}
		channelPredicates = append(channelPredicates, channelText+" LIKE ?")
		channelArgs = append(channelArgs, pattern)
		programPredicates = append(programPredicates, programText+" LIKE ?")
		programArgs = append(programArgs, pattern)
	}
	if len(channelPredicates) == 0 {
		return
	}
	*where = append(*where, "(("+strings.Join(channelPredicates, " OR ")+") OR "+liveTVGuideProgramExistsSQL(strings.Join(programPredicates, " OR "))+")")
	*args = append(*args, channelArgs...)
	*args = append(*args, to.Format(time.RFC3339), from.Format(time.RFC3339))
	*args = append(*args, programArgs...)
}

func liveTVGuideChannelTextSQL() string {
	return "LOWER(COALESCE(c.number, '') || ' ' || COALESCE(c.name, '') || ' ' || COALESCE(c.group_title, '') || ' ' || COALESCE(c.country, '') || ' ' || COALESCE(c.tvg_id, ''))"
}

func liveTVGuideProgramExistsSQL(predicate string) string {
	return "EXISTS (SELECT 1 FROM live_tv_programs p WHERE p.source_id = c.source_id AND p.channel_id = c.id AND " + liveTVProgramGenerationPredicate("p") + " AND p.start_at < ? AND p.end_at > ? AND (" + predicate + ") LIMIT 1)"
}

func liveTVGuideChannelOrderSQL(from time.Time, to time.Time, sortValue string, orderValue string, favoriteSQL string) (string, []any) {
	direction := "ASC"
	if strings.EqualFold(strings.TrimSpace(orderValue), "desc") {
		direction = "DESC"
	}
	switch strings.ToLower(strings.TrimSpace(sortValue)) {
	case "name":
		return "LOWER(c.name) " + direction + ", c.sort_order ASC, c.name ASC", nil
	case "now":
		return "COALESCE((SELECT LOWER(p.title) FROM live_tv_programs p WHERE p.source_id = c.source_id AND p.channel_id = c.id AND " + liveTVProgramGenerationPredicate("p") + " AND p.start_at < ? AND p.end_at > ? ORDER BY p.start_at DESC LIMIT 1), '') " + direction + ", c.sort_order ASC, c.name ASC", []any{to.Format(time.RFC3339), from.Format(time.RFC3339)}
	case "favorites", "favorite":
		return favoriteSQL + " " + direction + ", c.sort_order ASC, c.name ASC", nil
	case "recent":
		return "c.sort_order " + direction + ", c.name ASC", nil
	default:
		return "c.sort_order " + direction + ", c.name ASC", nil
	}
}

type liveTVChannelRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanLiveTVChannels(rows liveTVChannelRows) ([]LiveTVChannel, error) {
	return scanLiveTVChannelsContext(context.Background(), rows)
}

func scanLiveTVChannelsContext(ctx context.Context, rows liveTVChannelRows) ([]LiveTVChannel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var channels []LiveTVChannel
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var channel LiveTVChannel
		var enabled, favorite, hidden int
		var rawLogoURL string
		if err := rows.Scan(&channel.ID, &channel.SourceID, &channel.Number, &channel.Name, &rawLogoURL, &channel.TVGID, &channel.GroupTitle, &channel.Country, &enabled, &favorite, &hidden, &channel.SortOrder, &channel.GuideChannelRef, &channel.ProgramCount); err != nil {
			return nil, err
		}
		channel.LogoURL = liveTVLogoProxyURL(channel.ID, rawLogoURL)
		channel.Enabled = enabled == 1
		channel.Favorite = favorite == 1
		channel.Hidden = hidden == 1
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (s *Server) updateLiveTVChannelStateForUser(ctx context.Context, user User, channelID string, req LiveTVChannelStateRequest) (LiveTVChannel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	profileID := viewerProfileID(user)
	accountID := accountIDForUser(user)
	current, err := s.getLiveTVChannelForProfileContext(ctx, profileID, channelID)
	if err != nil {
		return LiveTVChannel{}, err
	}
	favorite := current.Favorite
	hidden := current.Hidden
	if req.Favorite != nil {
		favorite = *req.Favorite
	}
	if req.Hidden != nil {
		hidden = *req.Hidden
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.withUserTxTagged(ctx, []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		var enabled int
		if err := tx.QueryRowContext(ctx, `SELECT enabled FROM live_tv_channels c WHERE c.id = ? AND `+liveTVChannelGenerationPredicate("c"), strings.TrimSpace(channelID)).Scan(&enabled); err != nil {
			return err
		}
		if enabled != 1 {
			return sql.ErrNoRows
		}
		if req.Favorite != nil || req.Hidden != nil {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO live_tv_channel_profile_state (profile_id, user_id, channel_id, favorite, hidden, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(profile_id, channel_id) DO UPDATE SET
					user_id = excluded.user_id,
					favorite = excluded.favorite,
					hidden = excluded.hidden,
					updated_at = excluded.updated_at`,
				profileID, accountID, current.ID, boolInt(favorite), boolInt(hidden), now, now); err != nil {
				return err
			}
		}
		if req.GuideChannelRef != nil {
			guideRef := strings.TrimSpace(*req.GuideChannelRef)
			if len(guideRef) > 200 {
				return errors.New("Guide channel reference must be 200 characters or fewer.")
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM live_tv_channel_mappings WHERE channel_id = ?`, current.ID); err != nil {
				return err
			}
			if guideRef != "" {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO live_tv_channel_mappings (source_id, channel_id, guide_channel_ref, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?)
					ON CONFLICT(source_id, guide_channel_ref) DO UPDATE SET channel_id = excluded.channel_id, updated_at = excluded.updated_at`,
					current.SourceID, current.ID, guideRef, now, now); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return LiveTVChannel{}, err
	}
	return s.getLiveTVChannelForProfileContext(ctx, profileID, channelID)
}

func (s *Server) getLiveTVChannelForProfileContext(ctx context.Context, profileID, channelID string) (LiveTVChannel, error) {
	row := s.queryUserRow(ctx, `
		SELECT
			c.id, c.source_id, c.number, c.name, c.logo_url, c.tvg_id, c.group_title, c.country,
			c.enabled, COALESCE(cps.favorite, 0), COALESCE(cps.hidden, 0), c.sort_order,
			COALESCE((SELECT m.guide_channel_ref FROM live_tv_channel_mappings m WHERE m.channel_id = c.id LIMIT 1), ''),
			(SELECT COUNT(*) FROM live_tv_programs p WHERE p.channel_id = c.id AND `+liveTVProgramGenerationPredicate("p")+`)
		FROM live_tv_channels c
		LEFT JOIN live_tv_channel_profile_state cps ON cps.channel_id = c.id AND cps.profile_id = ?
		WHERE c.id = ? AND c.enabled = 1 AND `+liveTVChannelGenerationPredicate("c"), strings.TrimSpace(profileID), strings.TrimSpace(channelID))
	var channel LiveTVChannel
	var rawLogoURL string
	var enabled, favorite, hidden int
	if err := row.Scan(&channel.ID, &channel.SourceID, &channel.Number, &channel.Name, &rawLogoURL, &channel.TVGID, &channel.GroupTitle, &channel.Country, &enabled, &favorite, &hidden, &channel.SortOrder, &channel.GuideChannelRef, &channel.ProgramCount); err != nil {
		return LiveTVChannel{}, err
	}
	channel.LogoURL = liveTVLogoProxyURL(channel.ID, rawLogoURL)
	channel.Enabled = enabled == 1
	channel.Favorite = favorite == 1
	channel.Hidden = hidden == 1
	return channel, nil
}

func (s *Server) updateLiveTVChannelState(channelID string, req LiveTVChannelStateRequest) (LiveTVChannel, error) {
	current, err := s.getLiveTVChannel(channelID)
	if err != nil {
		return LiveTVChannel{}, err
	}
	favorite := current.Favorite
	hidden := current.Hidden
	if req.Favorite != nil {
		favorite = *req.Favorite
	}
	if req.Hidden != nil {
		hidden = *req.Hidden
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.withUserTxTagged(context.Background(), []string{"live-tv", "dvr"}, func(tx *sql.Tx) error {
		result, err := tx.Exec(`UPDATE live_tv_channels SET favorite = ?, hidden = ?, updated_at = ? WHERE id = ? AND enabled = 1 AND `+liveTVChannelGenerationPredicate("live_tv_channels"),
			boolInt(favorite), boolInt(hidden), now, strings.TrimSpace(channelID))
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return sql.ErrNoRows
		}
		if req.GuideChannelRef != nil {
			guideRef := strings.TrimSpace(*req.GuideChannelRef)
			if len(guideRef) > 200 {
				return errors.New("Guide channel reference must be 200 characters or fewer.")
			}
			if _, err := tx.Exec(`DELETE FROM live_tv_channel_mappings WHERE channel_id = ?`, current.ID); err != nil {
				return err
			}
			if guideRef != "" {
				if _, err := tx.Exec(`
					INSERT INTO live_tv_channel_mappings (source_id, channel_id, guide_channel_ref, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?)
					ON CONFLICT(source_id, guide_channel_ref) DO UPDATE SET
						channel_id = excluded.channel_id,
						updated_at = excluded.updated_at`,
					current.SourceID, current.ID, guideRef, now, now); err != nil {
					return err
				}
			}
		}
		hiddenDelta := boolInt(hidden) - boolInt(current.Hidden)
		favoriteDelta := boolInt(favorite) - boolInt(current.Favorite)
		if hiddenDelta != 0 || favoriteDelta != 0 {
			if _, err := tx.Exec(`
				UPDATE live_tv_sources
				SET
					hidden_channel_count = max(0, hidden_channel_count + ?),
					favorite_channel_count = max(0, favorite_channel_count + ?),
					summary_updated_at = ?
				WHERE id = ?`, hiddenDelta, favoriteDelta, now, current.SourceID); err != nil {
				return err
			}
		}
		if hiddenDelta != 0 {
			if err := refreshLiveTVChannelSearchRowTx(tx, current.ID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return LiveTVChannel{}, err
	}
	return s.getLiveTVChannel(channelID)
}

func (s *Server) getLiveTVChannel(channelID string) (LiveTVChannel, error) {
	row := s.queryUserRow(context.Background(), `
		SELECT
			c.id, c.source_id, c.number, c.name, c.logo_url, c.tvg_id, c.group_title, c.country,
			c.enabled, c.favorite, c.hidden, c.sort_order,
			COALESCE((SELECT m.guide_channel_ref FROM live_tv_channel_mappings m WHERE m.channel_id = c.id LIMIT 1), ''),
			(SELECT COUNT(*) FROM live_tv_programs p WHERE p.channel_id = c.id AND `+liveTVProgramGenerationPredicate("p")+`)
		FROM live_tv_channels c
		WHERE c.id = ? AND c.enabled = 1 AND `+liveTVChannelGenerationPredicate("c"), strings.TrimSpace(channelID))
	var channel LiveTVChannel
	var rawLogoURL string
	var enabled, favorite, hidden int
	if err := row.Scan(&channel.ID, &channel.SourceID, &channel.Number, &channel.Name, &rawLogoURL, &channel.TVGID, &channel.GroupTitle, &channel.Country, &enabled, &favorite, &hidden, &channel.SortOrder, &channel.GuideChannelRef, &channel.ProgramCount); err != nil {
		return LiveTVChannel{}, err
	}
	channel.LogoURL = liveTVLogoProxyURL(channel.ID, rawLogoURL)
	channel.Enabled = enabled == 1
	channel.Favorite = favorite == 1
	channel.Hidden = hidden == 1
	return channel, nil
}

func (s *Server) applyUserLiveTVChannelPolicy(userID string, channels []LiveTVChannel) []LiveTVChannel {
	policy := s.userLiveTVChannelPolicy(userID)
	if len(policy.AllowedChannelIDs) == 0 && len(policy.BlockedChannelIDs) == 0 || len(channels) == 0 {
		return channels
	}
	filtered := channels[:0]
	for _, channel := range channels {
		if liveTVChannelAllowedByPolicy(channel.ID, policy) {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}

func (s *Server) userLiveTVChannelAllowed(userID string, channelID string) bool {
	policy, err := s.userLiveTVChannelPolicyContext(context.Background(), userID)
	return err == nil && liveTVChannelAllowedByPolicy(channelID, policy)
}

func (s *Server) userLiveTVChannelAllowedForUser(user User, channelID string) bool {
	accountPolicy, err := s.userLiveTVChannelPolicyContext(context.Background(), accountIDForUser(user))
	return err == nil && liveTVChannelAllowedByPolicy(channelID, accountPolicy) && liveTVChannelAllowedByPolicy(channelID, user.ChannelPolicy)
}

func (s *Server) userLiveTVChannelPolicy(userID string) UserChannelPolicy {
	policy, _ := s.userLiveTVChannelPolicyContext(context.Background(), userID)
	return policy
}

func (s *Server) userLiveTVChannelPolicyContext(ctx context.Context, userID string) (UserChannelPolicy, error) {
	if strings.TrimSpace(userID) == "" {
		return UserChannelPolicy{}, errors.New("Live TV channel policy principal is required")
	}
	var role string
	var raw string
	if err := s.queryUserRow(ctx, `SELECT role, preferences_json FROM users WHERE id = ? AND COALESCE(disabled_at, '') = ''`, userID).Scan(&role, &raw); err != nil {
		return UserChannelPolicy{}, err
	}
	if role == "owner" {
		return UserChannelPolicy{}, nil
	}
	return decodeUserChannelPolicy(raw), nil
}

func liveTVChannelAllowedByPolicy(channelID string, policy UserChannelPolicy) bool {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return false
	}
	if len(policy.AllowedChannelIDs) > 0 {
		allowed := false
		for _, allowedID := range policy.AllowedChannelIDs {
			if allowedID == channelID {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	for _, blockedID := range policy.BlockedChannelIDs {
		if blockedID == channelID {
			return false
		}
	}
	return true
}

func combineUserChannelPolicies(account, profile UserChannelPolicy) UserChannelPolicy {
	account = normalizeUserChannelPolicy(account)
	profile = normalizeUserChannelPolicy(profile)
	combined := UserChannelPolicy{BlockedChannelIDs: append([]string(nil), account.BlockedChannelIDs...)}
	for _, channelID := range profile.BlockedChannelIDs {
		combined.BlockedChannelIDs = appendUniqueString(combined.BlockedChannelIDs, channelID)
	}
	switch {
	case len(account.AllowedChannelIDs) == 0:
		combined.AllowedChannelIDs = append([]string(nil), profile.AllowedChannelIDs...)
	case len(profile.AllowedChannelIDs) == 0:
		combined.AllowedChannelIDs = append([]string(nil), account.AllowedChannelIDs...)
	default:
		profileAllowed := stringSet(profile.AllowedChannelIDs)
		for _, channelID := range account.AllowedChannelIDs {
			if profileAllowed[channelID] {
				combined.AllowedChannelIDs = append(combined.AllowedChannelIDs, channelID)
			}
		}
	}
	return normalizeUserChannelPolicy(combined)
}

func liveTVLogoProxyURL(channelID, rawLogoURL string) string {
	if strings.TrimSpace(rawLogoURL) == "" {
		return ""
	}
	return "/api/live-tv/logos/" + url.PathEscape(channelID)
}

func (s *Server) listLiveTVPrograms(sourceID string, from time.Time, to time.Time) ([]LiveTVProgram, error) {
	rows, err := s.queryUserRead(context.Background(), `
		SELECT id, source_id, COALESCE(channel_id, ''), channel_ref, title, subtitle, description, category, start_at, end_at, episode_num, is_new
		FROM live_tv_programs
		WHERE source_id = ? AND `+liveTVProgramGenerationPredicate("live_tv_programs")+` AND start_at < ? AND end_at > ?
		ORDER BY start_at ASC, title ASC`, sourceID, to.Format(time.RFC3339), from.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var programs []LiveTVProgram
	for rows.Next() {
		var program LiveTVProgram
		var isNew int
		if err := rows.Scan(&program.ID, &program.SourceID, &program.ChannelID, &program.ChannelRef, &program.Title, &program.Subtitle, &program.Description, &program.Category, &program.StartAt, &program.EndAt, &program.EpisodeNum, &isNew); err != nil {
			return nil, err
		}
		program.IsNew = isNew == 1
		programs = append(programs, program)
	}
	return programs, rows.Err()
}

func (s *Server) listLiveTVProgramsForChannels(sourceID string, channelIDs []string, from time.Time, to time.Time) ([]LiveTVProgram, error) {
	return s.listLiveTVProgramsForChannelsContext(context.Background(), sourceID, channelIDs, from, to)
}

func (s *Server) listLiveTVProgramsForChannelsContext(ctx context.Context, sourceID string, channelIDs []string, from time.Time, to time.Time) ([]LiveTVProgram, error) {
	programs, _, err := s.listLiveTVProgramsForChannelsPageContext(ctx, sourceID, channelIDs, from, to, maxLiveTVGuidePrograms)
	return programs, err
}

func (s *Server) listLiveTVProgramsForChannelsPageContext(ctx context.Context, sourceID string, channelIDs []string, from time.Time, to time.Time, limit int) ([]LiveTVProgram, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(channelIDs) == 0 {
		return []LiveTVProgram{}, false, nil
	}
	limit = clampInt(limit, 1, maxLiveTVGuidePrograms)
	placeholders := make([]string, len(channelIDs))
	args := []any{sourceID, to.Format(time.RFC3339), from.Format(time.RFC3339)}
	for index, channelID := range channelIDs {
		placeholders[index] = "?"
		args = append(args, channelID)
	}
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT id, source_id, COALESCE(channel_id, ''), channel_ref, title, subtitle, description, category, start_at, end_at, episode_num, is_new
		FROM live_tv_programs
		WHERE source_id = ? AND `+liveTVProgramGenerationPredicate("live_tv_programs")+` AND start_at < ? AND end_at > ? AND channel_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY channel_id ASC, start_at ASC, title ASC
		LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var programs []LiveTVProgram
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		var program LiveTVProgram
		var isNew int
		if err := rows.Scan(&program.ID, &program.SourceID, &program.ChannelID, &program.ChannelRef, &program.Title, &program.Subtitle, &program.Description, &program.Category, &program.StartAt, &program.EndAt, &program.EpisodeNum, &isNew); err != nil {
			return nil, false, err
		}
		program.IsNew = isNew == 1
		programs = append(programs, program)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(programs) > limit
	if truncated {
		programs = programs[:limit]
	}
	return programs, truncated, nil
}

func normalizeLiveTVGuidePrograms(programs []LiveTVProgram) []LiveTVProgram {
	if len(programs) < 2 {
		return programs
	}
	sorted := append([]LiveTVProgram(nil), programs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		iKey := liveTVGuideProgramKey(sorted[i])
		jKey := liveTVGuideProgramKey(sorted[j])
		if iKey != jKey {
			return iKey < jKey
		}
		if sorted[i].StartAt == sorted[j].StartAt {
			if sorted[i].EndAt == sorted[j].EndAt {
				return sorted[i].Title < sorted[j].Title
			}
			return sorted[i].EndAt < sorted[j].EndAt
		}
		return sorted[i].StartAt < sorted[j].StartAt
	})

	lastEndByChannel := map[string]time.Time{}
	normalized := make([]LiveTVProgram, 0, len(sorted))
	for _, program := range sorted {
		key := liveTVGuideProgramKey(program)
		start, startErr := time.Parse(time.RFC3339, program.StartAt)
		end, endErr := time.Parse(time.RFC3339, program.EndAt)
		if key == "" || startErr != nil || endErr != nil || !end.After(start) {
			continue
		}
		if lastEnd, ok := lastEndByChannel[key]; ok && start.Before(lastEnd) {
			if !end.After(lastEnd) {
				continue
			}
			start = lastEnd
			program.StartAt = start.Format(time.RFC3339)
		}
		lastEndByChannel[key] = end
		normalized = append(normalized, program)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].StartAt == normalized[j].StartAt {
			return normalized[i].Title < normalized[j].Title
		}
		return normalized[i].StartAt < normalized[j].StartAt
	})
	return normalized
}

func liveTVGuideProgramKey(program LiveTVProgram) string {
	return firstNonEmpty(program.ChannelID, normalizeLiveTVKey(program.ChannelRef))
}

func (s *Server) handleLiveTVPlaybackStart(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var req LiveTVPlaybackSessionCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Intent.Quality.Mode) == "" {
		writeError(w, http.StatusBadRequest, "invalid_playback_quality", "intent.quality is required and must be Automatic or an exact server-issued explicit offer.")
		return
	}
	s.startLiveTVPlayback(w, r, user, req.ChannelID, req.ClientProfile, req.Intent, req.ClientInstanceID, req.Replacement)
}

func (s *Server) handleLiveTVStreamOpen(w http.ResponseWriter, r *http.Request, user User, channelID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var req LiveTVPlaybackSessionCreateRequest
	if r.Body != nil && r.ContentLength != 0 {
		if !decodeJSON(w, r, &req) {
			return
		}
	}
	if strings.TrimSpace(req.Intent.Quality.Mode) == "" {
		writeError(w, http.StatusBadRequest, "invalid_playback_quality", "intent.quality is required and must be Automatic or an exact server-issued explicit offer.")
		return
	}
	s.startLiveTVPlayback(w, r, user, firstNonEmpty(req.ChannelID, channelID), req.ClientProfile, req.Intent, req.ClientInstanceID, req.Replacement)
}

func (s *Server) handleLiveTVStreamClose(w http.ResponseWriter, r *http.Request, user User, channelID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var req LiveTVPlaybackCloseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		writeError(w, http.StatusBadRequest, "live_tv_session_required", "A playback session ID is required.")
		return
	}
	terminalReq, terminalErr := normalizePlaybackSessionStopRequest(PlaybackSessionStopRequest{RequestID: strings.TrimSpace(req.RequestID), Terminal: req.Terminal})
	if terminalErr != nil {
		writePlaybackStartError(w, terminalErr)
		return
	}
	ack, err := s.closeLiveTVStream(r.Context(), user, channelID, req.SessionID, terminalReq)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "playback_session_not_found", "Live TV playback session was not found.")
			return
		}
		if errors.Is(err, errPlaybackTerminalReceiptConflict) {
			writeError(w, http.StatusConflict, "playback_terminal_request_conflict", "The close request does not match the accepted terminal receipt.")
			return
		}
		if errors.Is(err, errPlaybackTerminalAuthorizationChanged) {
			writeError(w, http.StatusConflict, "playback_terminal_scope_changed", "Playback authorization changed. Reconcile the Live TV session before retrying close.")
			return
		}
		writeError(w, http.StatusConflict, "playback_session_failed", "Unable to close Live TV stream with the supplied terminal evidence.")
		return
	}
	writeJSON(w, http.StatusOK, ack)
}

func (s *Server) startLiveTVPlayback(w http.ResponseWriter, r *http.Request, user User, channelID string, clientProfile PlaybackClientProfile, intent PlaybackIntent, clientInstanceID string, replacement *PlaybackReplacementRequest) {
	playback, startErr := s.startLiveTVPlaybackForRequest(r, user, channelID, clientProfile, intent, clientInstanceID, replacement, "live-tv", channelID, nil)
	if startErr != nil {
		writePlaybackStartError(w, startErr)
		return
	}
	setPlaybackMediaGrantCookie(w, r, playback)
	writeJSON(w, http.StatusOK, playback)
}

// startLiveTVPlaybackForRequest is the canonical non-HTTP live playback
// constructor. Cast, DVR and ordinary Live TV entry points all use this path
// so target-specific clients cannot drift in policy, grant, tuner allocation,
// timeline or continuation semantics.
func (s *Server) startLiveTVPlaybackForRequest(r *http.Request, user User, channelID string, clientProfile PlaybackClientProfile, intent PlaybackIntent, clientInstanceID string, replacement *PlaybackReplacementRequest, replacementTargetKind, replacementTargetID string, externalReplacement *playbackReplacementPlan) (PlaybackResponse, *playbackStartHTTPError) {
	if !canPlayLiveTV(user) {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusForbidden, code: "forbidden", message: "You do not have permission to play Live TV."}
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusBadRequest, code: "live_tv_channel_required", message: "A Live TV channel ID is required."}
	}
	targetRequest := LiveTVPlaybackSessionCreateRequest{ChannelID: channelID, ClientInstanceID: clientInstanceID, ClientProfile: clientProfile, Intent: intent, Replacement: replacement}
	replacementPlan := playbackReplacementPlan{}
	if externalReplacement == nil {
		var replacementErr *playbackStartHTTPError
		replacementPlan, replacementErr = s.preparePlaybackReplacement(r.Context(), user, clientInstanceID, replacementTargetKind, replacementTargetID, targetRequest, replacement)
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
	channel, source, err := s.getLiveTVChannelForPlayback(channelID)
	if err != nil {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusNotFound, code: "live_tv_channel_not_found", message: "Live TV channel was not found."}
	}
	if !s.userLiveTVChannelAllowedForUser(user, channel.ID) {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusNotFound, code: "live_tv_channel_not_found", message: "Live TV channel was not found."}
	}
	if !source.Enabled && !canManageLiveTVSources(user) {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusForbidden, code: "forbidden", message: "This Live TV source is disabled."}
	}
	if err := s.enforcePlaybackSessionReplacementLimitContext(r.Context(), user.ID, clientInstanceID); err != nil {
		if errors.Is(err, errPlaybackSessionLimit) {
			return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusTooManyRequests, code: "playback_session_limit", message: "This account has reached its maximum active playback sessions.", retryAfter: "15"}
		}
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusInternalServerError, code: "playback_policy_failed", message: "Unable to verify playback session policy."}
	}

	streamFormat := "hls"
	providerStreamFormat := "direct"
	if isHLSURL(channel.streamURL) {
		providerStreamFormat = "hls"
	}
	sourceURL := liveTVHLSPlaylistRoute(channel.ID, "")
	media := MediaItem{
		ID:              channel.ID,
		LibraryID:       source.ID,
		Type:            "live_channel",
		Title:           channel.Name,
		SortTitle:       channel.Name,
		Summary:         firstNonEmpty(channel.groupTitle, "Live TV"),
		DurationSeconds: 0,
		Genres:          []string{"Live TV"},
		Tags:            []string{source.Name},
		Labels:          []string{source.Type},
		AddedAt:         time.Now().UTC().Format(time.RFC3339),
		Images:          imageSetFor(channel.ID, channel.groupTitle, channel.Name, nil),
		SourceURL:       sourceURL,
	}
	if channel.logoURL != "" {
		media.Images.Thumb = liveTVLogoProxyURL(channel.ID, channel.logoURL)
	}
	applyMediaActionsToItem(&media, user)
	policy, clientProfile := s.resolvePlaybackPolicyForRequest(r.Context(), r, user, media, intent, clientProfile)
	qualityAuthority, err := issueContinuousPlaybackQualityOffers(
		channel.ID, channel.ID+"\x00"+source.ID,
		channel.streamURL+"\x00"+providerStreamFormat,
		policy, []string{"1080p-high", "1080p-medium", "720p-high", "720p-medium", "480p", "328p"}, true,
	)
	if err != nil {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusUnprocessableEntity, code: "playback_quality_unavailable", message: "Portico could not issue a complete quality offer set for this channel."}
	}
	quality, policy, clientProfile, selectedQuality, err := resolveContinuousPlaybackQualityForRequest(qualityAuthority, intent.Quality, policy, clientProfile)
	if err != nil {
		return PlaybackResponse{}, playbackQualityStartError(err)
	}
	intent.Quality = normalizedPlaybackQualitySelection(intent.Quality)
	decision := decideLiveTVHLSDelivery(channel.streamURL, source, clientProfile)
	decision = applyResolvedDeliveryMode(decision, media, policy, false, s.userCanTranscode(user))
	decision = s.applyTranscodeCapabilityNotes(decision, media)
	if decision.Mode == "unavailable" {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusUnprocessableEntity, code: "delivery_policy_unsatisfied", message: "This Live TV channel cannot be delivered within the selected playback policy."}
	}
	if quality.Kind == playbackQualityKindOriginal && decision.VideoTranscode {
		return PlaybackResponse{}, playbackQualityStartError(&ExplicitQualityUnavailableError{Offers: qualityAuthority.set})
	}
	if decision.RequiresTranscode && !s.userCanTranscode(user) {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusUnprocessableEntity, code: "transcode_not_authorized", message: "This profile cannot create the compatible Live TV stream required for this channel."}
	}
	sessionID := randomID("live")
	initialState := "playing"
	if externalReplacement != nil {
		sessionID = externalReplacement.Claim.ReplacementSessionID
		initialState = "handoff_pending"
	} else if replacementPlan.Active {
		sessionID = replacementPlan.Claim.ReplacementSessionID
		initialState = "handoff_pending"
	}
	currentEntryID := randomID("qentry")
	s.dvrAllocationMu.Lock()
	allocationLease, allocationErr := s.reserveLiveTVTunerAllocation(r.Context(), source.ID, channel.ID, "live_session", sessionID)
	if allocationErr != nil {
		s.dvrAllocationMu.Unlock()
		if errors.Is(allocationErr, errLiveTVTunerCapacity) {
			return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusConflict, code: "live_tv_tuner_capacity", message: "Every tuner for this Live TV source is currently in use.", retryAfter: "15"}
		}
		s.log.Warn("reserve live tv tuner failed", "error", allocationErr, "source", source.ID, "channel", channel.ID)
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusServiceUnavailable, code: "live_tv_tuner_allocation_failed", message: "Portico could not reserve a tuner for this channel."}
	}
	createSessionErr := s.createPlaybackSessionWithState(r, user, media, currentEntryID, sessionID, decision, clientProfile, intent, "", "", true, clientInstanceID, PlaybackSourceContext{Type: "library", ID: source.ID, Title: source.Name}, "off", initialState)
	s.dvrAllocationMu.Unlock()
	if createSessionErr != nil {
		if allocationLease.Created {
			s.releaseLiveTVTunerAllocationLease(r.Context(), "live_session", sessionID, allocationLease.Token)
		}
		if errors.Is(createSessionErr, errPlaybackSessionLimit) {
			return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusTooManyRequests, code: "playback_session_limit", message: "This account has reached its maximum active playback sessions.", retryAfter: "15"}
		}
		if errors.Is(createSessionErr, errPlaybackReplacementRequired) {
			return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusConflict, code: "replacement_required", message: "This client already owns active playback. Supply its exact replacement authority envelope."}
		}
		s.log.Warn("create live playback session failed", "error", createSessionErr, "channel", channel.ID)
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusInternalServerError, code: "playback_session_failed", message: "Unable to start Live TV playback session."}
	}
	cleanupFailedStart := func() {
		_, cleanupErr := s.playbackLifecycle().Terminate(context.Background(), playbackTerminationRequest{
			SessionID: sessionID, UserID: accountIDForUser(user), ProfileID: viewerProfileID(user),
			Cause: playbackTerminationFailedStart, RemoveSession: true,
		})
		if cleanupErr != nil && !errors.Is(cleanupErr, sql.ErrNoRows) {
			s.log.Error("failed Live TV start cleanup failed", "error", cleanupErr, "session", sessionID, "channel", channel.ID)
		}
	}
	policy.LiveDelivery = &PlaybackDeliveryPolicy{DeliveryMode: "server_hls", GrantRequired: true, AllowedOperationClasses: []string{"manifest", "segment"}, AuthorizationRecheckSeconds: 60, QualityProfile: selectedQuality}
	mediaGrant, err := s.issueLiveMediaGrantForPlayback(r.Context(), user, sessionID, channel.ID, decision, selectedQuality, true)
	if err != nil {
		cleanupFailedStart()
		s.log.Error("issue Live TV media grant failed", "error", err, "session", sessionID, "channel", channel.ID)
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusInternalServerError, code: "media_grant_failed", message: "Unable to authorize Live TV media resources."}
	}
	sourceURL = liveTVHLSPlaybackURL(channel.ID, selectedQuality)
	media.SourceURL = sourceURL
	resources := liveTVPlaybackResources(channel.ID, selectedQuality)
	playback := PlaybackResponse{
		SessionID:           sessionID,
		CurrentQueueEntryID: currentEntryID,
		NextEventSequence:   1,
		MediaGrant:          mediaGrant,
		Media:               media,
		SourceURL:           sourceURL,
		DirectPlay:          !decision.RequiresTranscode,
		IsLive:              true,
		StreamFormat:        streamFormat,
		Resources:           resources,
		Decision:            decision,
		Policy:              policy,
		QualityOffers:       qualityAuthority.set,
		QualitySelection:    intent.Quality,
		AudioStreams:        []Stream{{ID: "audio_default", Kind: "audio", Codec: "provider", DisplayTitle: "Provider default"}},
		SubtitleStreams:     []Stream{{ID: "sub_none", Kind: "subtitle", DisplayTitle: "None"}},
		Chapters:            []Chapter{},
		Queue:               []PlaybackQueueEntry{},
		RepeatMode:          "off",
		QueueRevision:       0,
		Timeline:            PlaybackTimeline{Type: "live", CanPause: false, CanSeek: false},
		Generation:          0,
		PlaybackRevision:    0,
	}
	if err := s.ensurePlaybackContinuationCredential(r, user, &playback); err != nil {
		cleanupFailedStart()
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusInternalServerError, code: "playback_continuity_failed", message: "Unable to establish playback continuity."}
	}
	if replacementPlan.Active {
		if err := s.commitPlaybackReplacement(r.Context(), user, replacementPlan, playback); err != nil {
			cleanupFailedStart()
			return PlaybackResponse{}, playbackReplacementCommitHTTPError(err)
		}
		releaseReplacement = false
	}
	s.recordLog("info", "Live TV playback started", map[string]string{"user": user.Email, "source": source.Name, "channel": channel.Name})
	return playback, nil
}

func (s *Server) liveTVPlaybackResponseForSession(r *http.Request, user User, sessionID string, channelID string, clientProfile PlaybackClientProfile, intent PlaybackIntent) (PlaybackResponse, error) {
	channel, source, err := s.getLiveTVChannelForPlayback(channelID)
	if err != nil {
		return PlaybackResponse{}, err
	}
	if !s.userLiveTVChannelAllowedForUser(user, channel.ID) {
		return PlaybackResponse{}, sql.ErrNoRows
	}
	if !source.Enabled && !canManageLiveTVSources(user) {
		return PlaybackResponse{}, sql.ErrNoRows
	}
	streamFormat := "hls"
	providerStreamFormat := "direct"
	if isHLSURL(channel.streamURL) {
		providerStreamFormat = "hls"
	}
	sourceURL := liveTVHLSPlaylistRoute(channel.ID, "")
	media := MediaItem{
		ID:              channel.ID,
		LibraryID:       source.ID,
		Type:            "live_channel",
		Title:           channel.Name,
		SortTitle:       channel.Name,
		Summary:         firstNonEmpty(channel.groupTitle, "Live TV"),
		DurationSeconds: 0,
		Genres:          []string{"Live TV"},
		Tags:            []string{source.Name},
		Labels:          []string{source.Type},
		AddedAt:         time.Now().UTC().Format(time.RFC3339),
		Images:          imageSetFor(channel.ID, channel.groupTitle, channel.Name, nil),
		SourceURL:       sourceURL,
	}
	if channel.logoURL != "" {
		media.Images.Thumb = liveTVLogoProxyURL(channel.ID, channel.logoURL)
	}
	applyMediaActionsToItem(&media, user)
	policy, clientProfile := s.resolvePlaybackPolicyForRequest(r.Context(), r, user, media, intent, clientProfile)
	qualityAuthority, err := issueContinuousPlaybackQualityOffers(
		channel.ID, channel.ID+"\x00"+source.ID,
		channel.streamURL+"\x00"+providerStreamFormat,
		policy, []string{"1080p-high", "1080p-medium", "720p-high", "720p-medium", "480p", "328p"}, true,
	)
	if err != nil {
		return PlaybackResponse{}, err
	}
	quality, policy, clientProfile, selectedQuality, err := resolveContinuousPlaybackQualityForRequest(qualityAuthority, intent.Quality, policy, clientProfile)
	if err != nil {
		return PlaybackResponse{}, err
	}
	intent.Quality = normalizedPlaybackQualitySelection(intent.Quality)
	decision := decideLiveTVHLSDelivery(channel.streamURL, source, clientProfile)
	decision = applyResolvedDeliveryMode(decision, media, policy, false, s.userCanTranscode(user))
	decision = s.applyTranscodeCapabilityNotes(decision, media)
	if decision.Mode == "unavailable" {
		return PlaybackResponse{}, errors.New("live TV delivery policy cannot be satisfied")
	}
	if quality.Kind == playbackQualityKindOriginal && decision.VideoTranscode {
		return PlaybackResponse{}, &ExplicitQualityUnavailableError{Offers: qualityAuthority.set}
	}
	if decision.RequiresTranscode && !s.userCanTranscode(user) {
		return PlaybackResponse{}, errors.New("live TV transcoding is not authorized for this profile")
	}
	policy.LiveDelivery = &PlaybackDeliveryPolicy{DeliveryMode: "server_hls", GrantRequired: true, AllowedOperationClasses: []string{"manifest", "segment"}, AuthorizationRecheckSeconds: 60, QualityProfile: selectedQuality}
	s.dvrAllocationMu.Lock()
	allocationLease, allocationErr := s.reserveLiveTVTunerAllocation(r.Context(), source.ID, channel.ID, "live_session", sessionID)
	s.dvrAllocationMu.Unlock()
	if allocationErr != nil {
		return PlaybackResponse{}, allocationErr
	}
	mediaGrant, err := s.issueLiveMediaGrantForPlayback(r.Context(), user, sessionID, channel.ID, decision, selectedQuality, true)
	if err != nil {
		if allocationLease.Created {
			s.releaseLiveTVTunerAllocationLease(context.Background(), "live_session", sessionID, allocationLease.Token)
		}
		return PlaybackResponse{}, err
	}
	sourceURL = liveTVHLSPlaybackURL(channel.ID, selectedQuality)
	media.SourceURL = sourceURL
	resources := liveTVPlaybackResources(channel.ID, selectedQuality)
	var currentEntryID string
	if err := s.queryUserRow(r.Context(), `SELECT current_entry_id FROM playback_sessions WHERE id = ? AND profile_id = ?`, sessionID, viewerProfileID(user)).Scan(&currentEntryID); err != nil {
		return PlaybackResponse{}, err
	}
	playback := PlaybackResponse{
		SessionID:           sessionID,
		CurrentQueueEntryID: currentEntryID,
		NextEventSequence:   s.nextPlaybackProgressEventSequence(user, sessionID),
		MediaGrant:          mediaGrant,
		Media:               media,
		SourceURL:           sourceURL,
		DirectPlay:          !decision.RequiresTranscode,
		IsLive:              true,
		StreamFormat:        streamFormat,
		Resources:           resources,
		Decision:            decision,
		Policy:              policy,
		QualityOffers:       qualityAuthority.set,
		QualitySelection:    intent.Quality,
		AudioStreams:        []Stream{{ID: "audio_default", Kind: "audio", Codec: "provider", DisplayTitle: "Provider default"}},
		SubtitleStreams:     []Stream{{ID: "sub_none", Kind: "subtitle", DisplayTitle: "None"}},
		Chapters:            []Chapter{},
		Queue:               []PlaybackQueueEntry{},
		RepeatMode:          "off",
		QueueRevision:       0,
		Timeline:            PlaybackTimeline{Type: "live", CanPause: false, CanSeek: false},
		Generation:          0,
		PlaybackRevision:    0,
	}
	_ = s.queryUserRow(r.Context(), `SELECT renegotiation_revision, playback_generation FROM playback_sessions WHERE id = ? AND profile_id = ?`, sessionID, viewerProfileID(user)).Scan(&playback.PlaybackRevision, &playback.Generation)
	if err := s.ensurePlaybackContinuationCredential(r, user, &playback); err != nil {
		return PlaybackResponse{}, err
	}
	return playback, nil
}

func (s *Server) handleLiveTVPlaybackRenegotiation(w http.ResponseWriter, r *http.Request, user User, sessionID, channelID string, req PlaybackRenegotiationRequest, revision int64, generation int, lastRequest, lastFingerprint, fingerprint string) {
	if req.Quality == nil || req.AudioStreamID != nil || req.SubtitleStreamID != nil || req.SubtitleMode != nil || req.VersionID != nil {
		writeError(w, http.StatusBadRequest, "invalid_live_tv_renegotiation", "Live TV playback can change only the advertised quality.")
		return
	}
	var storedProfileJSON, storedIntentJSON string
	if err := s.queryUserRow(r.Context(), `SELECT client_profile_json, playback_intent_json FROM playback_sessions WHERE id = ? AND profile_id = ?`, sessionID, viewerProfileID(user)).Scan(&storedProfileJSON, &storedIntentJSON); err != nil {
		writeError(w, http.StatusNotFound, "playback_session_not_found", "Playback session was not found.")
		return
	}
	var profile PlaybackClientProfile
	var intent PlaybackIntent
	_ = json.Unmarshal([]byte(storedProfileJSON), &profile)
	_ = json.Unmarshal([]byte(storedIntentJSON), &intent)
	if !isZeroPlaybackClientProfile(req.ClientProfile) {
		profile = req.ClientProfile
	}
	if !isZeroPlaybackIntent(req.Intent) {
		req.Intent.Quality = PlaybackQualitySelection{}
		intent = req.Intent
	}
	intent.Quality = normalizedPlaybackQualitySelection(*req.Quality)
	_, _, _, err := s.liveTVQualityAuthorityForRequest(r, user, channelID, profile, intent)
	if err != nil {
		writePlaybackStartError(w, playbackQualityStartError(err))
		return
	}
	if lastRequest == req.RequestID {
		if lastFingerprint != fingerprint {
			writeError(w, http.StatusConflict, "renegotiation_request_conflict", "requestId was already used with a different playback selection.")
			return
		}
		playback, err := s.liveTVPlaybackResponseForSession(r, user, sessionID, channelID, profile, intent)
		if err != nil {
			writePlaybackQualityOrFallbackError(w, err, http.StatusBadRequest, "renegotiation_failed", "Unable to restore Live TV playback after the quality change.")
			return
		}
		setPlaybackMediaGrantCookie(w, r, playback)
		writeJSON(w, http.StatusOK, playback)
		return
	}
	if revision != req.ExpectedRevision {
		writeError(w, http.StatusConflict, "renegotiation_revision_conflict", "Playback selection changed on another client. Reload the playback response before renegotiating.")
		return
	}
	profileJSON, _ := json.Marshal(profile)
	intentJSON, _ := json.Marshal(intent)
	result, err := s.execUserWriteTaggedForViewer(r.Context(), accountIDForUser(user), viewerProfileID(user), []string{"playback"}, `
		UPDATE playback_sessions SET renegotiation_revision = renegotiation_revision + 1,
			client_profile_json = ?, playback_intent_json = ?, playback_generation = playback_generation + 1,
			last_renegotiation_request_id = ?, last_renegotiation_fingerprint = ?, last_seen_at = ?
		WHERE id = ? AND profile_id = ? AND ended_at = '' AND state <> 'stopped' AND is_live = 1
			AND renegotiation_revision = ? AND playback_generation = ?`,
		string(profileJSON), string(intentJSON), req.RequestID, fingerprint, time.Now().UTC().Format(time.RFC3339Nano),
		sessionID, viewerProfileID(user), revision, generation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "renegotiation_failed", "Unable to change Live TV quality.")
		return
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		writeError(w, http.StatusConflict, "renegotiation_revision_conflict", "Playback selection changed on another client. Reload the playback response before renegotiating.")
		return
	}
	_, _ = s.execSecurityFenceWriteTagged(r.Context(), []string{"playback"}, `UPDATE playback_media_grants SET revoked_at = ? WHERE playback_session_id = ? AND revoked_at = ''`, time.Now().UTC().Format(time.RFC3339Nano), sessionID)
	s.forgetMediaGrantsForPlaybackSession(sessionID)
	playback, err := s.liveTVPlaybackResponseForSession(r, user, sessionID, channelID, profile, intent)
	if err != nil {
		writePlaybackQualityOrFallbackError(w, err, http.StatusBadRequest, "renegotiation_failed", "Unable to change Live TV quality.")
		return
	}
	setPlaybackMediaGrantCookie(w, r, playback)
	writeJSON(w, http.StatusOK, playback)
}

func (s *Server) liveTVQualityAuthorityForRequest(r *http.Request, user User, channelID string, profile PlaybackClientProfile, intent PlaybackIntent) (playbackQualityOfferAuthority, PlaybackQualitySelection, string, error) {
	channel, source, err := s.getLiveTVChannelForPlayback(channelID)
	if err != nil {
		return playbackQualityOfferAuthority{}, PlaybackQualitySelection{}, "", err
	}
	providerStreamFormat := "direct"
	if isHLSURL(channel.streamURL) {
		providerStreamFormat = "hls"
	}
	media := MediaItem{ID: channel.ID, LibraryID: source.ID, Type: "live_channel", Title: channel.Name, SourceURL: liveTVHLSPlaylistRoute(channel.ID, "")}
	policy, profile := s.resolvePlaybackPolicyForRequest(r.Context(), r, user, media, intent, profile)
	authority, err := issueContinuousPlaybackQualityOffers(
		channel.ID, channel.ID+"\x00"+source.ID,
		channel.streamURL+"\x00"+providerStreamFormat,
		policy, []string{"1080p-high", "1080p-medium", "720p-high", "720p-medium", "480p", "328p"}, true,
	)
	if err != nil {
		return playbackQualityOfferAuthority{}, PlaybackQualitySelection{}, "", err
	}
	quality, _, profile, selected, err := resolveContinuousPlaybackQualityForRequest(authority, intent.Quality, policy, profile)
	if err != nil {
		return authority, PlaybackQualitySelection{}, "", err
	}
	decision := decideLiveTVHLSDelivery(channel.streamURL, source, profile)
	if quality.Kind == playbackQualityKindOriginal && decision.VideoTranscode {
		return authority, PlaybackQualitySelection{}, "", &ExplicitQualityUnavailableError{Offers: authority.set}
	}
	return authority, normalizedPlaybackQualitySelection(intent.Quality), selected, nil
}

func (s *Server) closeLiveTVStream(ctx context.Context, user User, channelID string, sessionID string, req PlaybackSessionStopRequest) (PlaybackSessionTerminalAcknowledgement, error) {
	sessionID = strings.TrimSpace(sessionID)
	var mediaID string
	var isLive int
	if err := s.queryUserRow(ctx, `
		SELECT media_id, is_live FROM playback_sessions
		WHERE id = ? AND (profile_id = ? OR ? = 1)`,
		sessionID, viewerProfileID(user), boolInt(canManageLiveTVSources(user))).Scan(&mediaID, &isLive); err != nil {
		return PlaybackSessionTerminalAcknowledgement{}, err
	}
	if mediaID != strings.TrimSpace(channelID) || isLive != 1 {
		return PlaybackSessionTerminalAcknowledgement{}, sql.ErrNoRows
	}
	if duplicate, receiptErr := s.playbackTerminalReceiptForUser(ctx, user, sessionID, req); receiptErr == nil {
		return duplicate, nil
	} else if !errors.Is(receiptErr, sql.ErrNoRows) {
		return PlaybackSessionTerminalAcknowledgement{}, receiptErr
	}
	return s.terminatePlaybackWithReceipt(ctx, user, sessionID, req, "", "")
}

func (s *Server) liveTVStreamSessions(user User, now time.Time) ([]PlaybackSession, error) {
	where := "WHERE ps.is_live = 1 AND ps.ended_at = '' AND ps.last_seen_at >= ?"
	args := []any{now.Add(-30 * time.Second).Format(time.RFC3339)}
	if !canManageLiveTVSources(user) {
		where += " AND ps.user_id = ?"
		args = append(args, user.ID)
	}
	rows, err := s.queryUserRead(context.Background(), `
		SELECT
			ps.id, ps.user_id, u.display_name, ps.media_id, ps.media_type, ps.title,
			ps.started_at, ps.last_seen_at, ps.ended_at, ps.client_ip, ps.device,
			ps.app, ps.location, ps.state, ps.progress, ps.bandwidth_mbps,
			ps.decision, ps.video_decision, ps.video_source, ps.video_target,
			ps.audio_decision, ps.audio_source, ps.audio_target, ps.subtitle_decision,
			ps.is_live, ps.command_json,
			COALESCE(c.source_id, ''), COALESCE(c.group_title, ''), COALESCE(c.logo_url, '')
		FROM playback_sessions ps
		JOIN users u ON u.id = ps.user_id
		LEFT JOIN live_tv_channels c ON c.id = ps.media_id AND `+liveTVChannelGenerationPredicate("c")+`
		`+where+`
		ORDER BY ps.last_seen_at DESC
		LIMIT 50`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	streams := []PlaybackSession{}
	for rows.Next() {
		var session PlaybackSession
		var mediaID, mediaType, title, sourceID, groupTitle, logoURL, commandJSON string
		var isLive int
		if err := rows.Scan(
			&session.ID, &session.UserID, &session.User, &mediaID, &mediaType, &title,
			&session.StartedAt, &session.LastSeenAt, &session.EndedAt, &session.ClientIP, &session.Device,
			&session.App, &session.Location, &session.State, &session.Progress, &session.BandwidthMbps,
			&session.Decision, &session.VideoDecision, &session.VideoSource, &session.VideoTarget,
			&session.AudioDecision, &session.AudioSource, &session.AudioTarget, &session.SubtitleDecision,
			&isLive, &commandJSON, &sourceID, &groupTitle, &logoURL,
		); err != nil {
			return nil, err
		}
		session.IsLive = isLive == 1
		session.Media = MediaItem{
			ID:        mediaID,
			LibraryID: sourceID,
			Type:      mediaType,
			Title:     title,
			SortTitle: title,
			Summary:   firstNonEmpty(groupTitle, "Live TV"),
			Genres:    []string{"Live TV"},
			Images:    imageSetFor(mediaID, groupTitle, title, nil),
		}
		if logoURL != "" {
			session.Media.Images.Thumb = liveTVLogoProxyURL(mediaID, logoURL)
		}
		session.Command = decodePlaybackCommand(commandJSON)
		streams = append(streams, session)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.attachActiveTranscodeDetails(streams)
	return streams, nil
}

type liveTVPlaybackChannel struct {
	ID         string
	SourceID   string
	Number     string
	Name       string
	streamURL  string
	groupTitle string
	logoURL    string
}

type liveTVQualityConstraint struct {
	id           string
	maxBandwidth int
	maxHeight    int
	source       bool
}

func liveTVQualityForResolvedPolicy(policy ResolvedPlaybackPolicy) string {
	requested := normalizeLiveTVQualityID(policy.DeliveryProfile)
	if requested != "auto" {
		constraint := liveTVQualityConstraintFor(requested)
		if constraint.source {
			if policy.MaxVideoBitrateMbps <= 0 && policy.MaxVideoHeight <= 0 {
				return requested
			}
		} else if (policy.MaxVideoBitrateMbps <= 0 || constraint.maxBandwidth <= policy.MaxVideoBitrateMbps*1_000_000) &&
			(policy.MaxVideoHeight <= 0 || constraint.maxHeight <= policy.MaxVideoHeight) {
			return requested
		}
	}
	selected := "480p"
	switch firstNonEmpty(policy.DeliveryProfile, resolvedDeliveryProfile("live_channel", policy)) {
	case "video-original":
		selected = "source"
	case "video-high", "video-standard":
		selected = "1080p-high"
	case "video-data-saver":
		selected = "720p-high"
	case "video-low":
		selected = "480p"
	}
	for _, candidate := range []string{selected, "1080p-high", "1080p-medium", "720p-high", "720p-medium", "480p", "328p"} {
		constraint := liveTVQualityConstraintFor(candidate)
		if constraint.source {
			if policy.MaxVideoBitrateMbps <= 0 && policy.MaxVideoHeight <= 0 {
				return constraint.id
			}
			continue
		}
		if policy.MaxVideoBitrateMbps > 0 && constraint.maxBandwidth > policy.MaxVideoBitrateMbps*1_000_000 {
			continue
		}
		if policy.MaxVideoHeight > 0 && constraint.maxHeight > policy.MaxVideoHeight {
			continue
		}
		return constraint.id
	}
	return "328p"
}

func liveTVHLSPlaybackURL(channelID, qualityID string) string {
	return liveTVHLSPlaylistRoute(channelID, "") + "?quality=" + url.QueryEscape(normalizeLiveTVQualityID(qualityID))
}

func liveTVPlaybackResources(channelID, selectedQuality string) []PlaybackResource {
	selectedQuality = normalizeLiveTVQualityID(selectedQuality)
	return []PlaybackResource{{
		ID:           randomID("pres"),
		SourceURL:    liveTVHLSPlaybackURL(channelID, selectedQuality),
		StreamFormat: "hls",
		SubtitleMode: "off",
		Default:      true,
	}}
}

func normalizeLiveTVQualityID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "auto"
	}
	switch value {
	case "auto", "source", "original", "1080p-high", "1080p-medium", "720p-high", "720p-medium", "480p", "328p":
		if value == "original" {
			return "source"
		}
		return value
	default:
		return "auto"
	}
}

func liveTVQualityConstraintFor(value string) liveTVQualityConstraint {
	switch normalizeLiveTVQualityID(value) {
	case "source":
		return liveTVQualityConstraint{id: "source", source: true}
	case "1080p-high":
		return liveTVQualityConstraint{id: "1080p-high", maxBandwidth: 8_000_000, maxHeight: 1080}
	case "1080p-medium":
		return liveTVQualityConstraint{id: "1080p-medium", maxBandwidth: 5_000_000, maxHeight: 1080}
	case "720p-high":
		return liveTVQualityConstraint{id: "720p-high", maxBandwidth: 4_000_000, maxHeight: 720}
	case "720p-medium":
		return liveTVQualityConstraint{id: "720p-medium", maxBandwidth: 2_500_000, maxHeight: 720}
	case "480p":
		return liveTVQualityConstraint{id: "480p", maxBandwidth: 1_500_000, maxHeight: 480}
	case "328p":
		return liveTVQualityConstraint{id: "328p", maxBandwidth: 700_000, maxHeight: 360}
	default:
		return liveTVQualityConstraint{id: "auto"}
	}
}

func (s *Server) getLiveTVChannelForPlayback(channelID string) (liveTVPlaybackChannel, liveTVSourceRecord, error) {
	var channel liveTVPlaybackChannel
	row := s.queryUserRow(context.Background(), `
		SELECT id, source_id, number, name, stream_url, group_title, logo_url
		FROM live_tv_channels
		WHERE id = ? AND enabled = 1 AND `+liveTVChannelGenerationPredicate("live_tv_channels"), strings.TrimSpace(channelID))
	if err := row.Scan(&channel.ID, &channel.SourceID, &channel.Number, &channel.Name, &channel.streamURL, &channel.groupTitle, &channel.logoURL); err != nil {
		return liveTVPlaybackChannel{}, liveTVSourceRecord{}, err
	}
	if strings.TrimSpace(channel.streamURL) == "" {
		return liveTVPlaybackChannel{}, liveTVSourceRecord{}, sql.ErrNoRows
	}
	source, err := s.getLiveTVSourceRecord(channel.SourceID)
	if err != nil {
		return liveTVPlaybackChannel{}, liveTVSourceRecord{}, err
	}
	return channel, source, nil
}

func (s *Server) handleLiveTVLogo(w http.ResponseWriter, r *http.Request, user User, channelID string) {
	if !canViewLiveTV(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view Live TV logos.")
		return
	}
	var logoURL string
	var userAgent string
	var sourceEnabled int
	row := s.queryUserRow(context.Background(), `
		SELECT c.logo_url, s.user_agent, s.enabled
		FROM live_tv_channels c
		JOIN live_tv_sources s ON s.id = c.source_id
		WHERE c.id = ? AND c.enabled = 1 AND `+liveTVChannelGenerationPredicate("c"), strings.TrimSpace(channelID))
	if err := row.Scan(&logoURL, &userAgent, &sourceEnabled); err != nil || (sourceEnabled != 1 && !canManageLiveTVSources(user)) {
		writeError(w, http.StatusNotFound, "live_tv_logo_not_found", "Live TV logo was not found.")
		return
	}
	if !s.userLiveTVChannelAllowedForUser(user, channelID) {
		writeError(w, http.StatusNotFound, "live_tv_logo_not_found", "Live TV logo was not found.")
		return
	}
	s.proxyLiveTVLogo(w, r, logoURL, userAgent)
}

func (s *Server) handleLiveTVHLS(w http.ResponseWriter, r *http.Request, user User, parts []string) {
	if !canPlayLiveTV(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to play Live TV.")
		return
	}
	if len(parts) < 2 {
		writeError(w, http.StatusNotFound, "not_found", "Live TV HLS route was not found.")
		return
	}
	channel, source, err := s.getLiveTVChannelForPlayback(parts[0])
	if err != nil || (!source.Enabled && !canManageLiveTVSources(user)) {
		writeError(w, http.StatusNotFound, "live_tv_channel_not_found", "Live TV channel was not found.")
		return
	}
	if !s.userLiveTVChannelAllowedForUser(user, channel.ID) {
		writeError(w, http.StatusNotFound, "live_tv_channel_not_found", "Live TV channel was not found.")
		return
	}
	switch parts[1] {
	case "playlist.m3u8":
		transcode, deliveryErr := s.liveTVRequestRequiresTranscode(r, channel)
		if deliveryErr != nil {
			writeError(w, http.StatusUnauthorized, "media_grant_denied", "A valid playback media grant is required.")
			return
		}
		if !transcode {
			s.rewriteLiveTVHLS(w, r, channel.ID, channel.streamURL, channel.streamURL, source.UserAgent)
			return
		}
		s.handleLiveTVTranscodeManifest(w, r, user, channel, source)
	case "segment":
		transcode, deliveryErr := s.liveTVRequestRequiresTranscode(r, channel)
		if deliveryErr != nil {
			writeError(w, http.StatusUnauthorized, "media_grant_denied", "A valid playback media grant is required.")
			return
		}
		if !transcode {
			writeError(w, http.StatusNotFound, "segment_not_found", "The Live TV segment is not available.")
			return
		}
		s.handleLiveTVTranscodeSegment(w, r, user, channel, source)
	case "item":
		transcode, deliveryErr := s.liveTVRequestRequiresTranscode(r, channel)
		if deliveryErr != nil {
			writeError(w, http.StatusUnauthorized, "media_grant_denied", "A valid playback media grant is required.")
			return
		}
		if transcode {
			writeError(w, http.StatusNotFound, "hls_item_not_found", "The Live TV HLS item is not available.")
			return
		}
		if len(parts) != 2 {
			writeError(w, http.StatusBadRequest, "bad_hls_reference", "The HLS item reference is missing.")
			return
		}
		rawURI, ok := s.resolveLiveTVHLSReference(channel.ID, channel.streamURL, r.URL.Query().Get("ref"), mediaGrantFromRequest(r))
		if !ok {
			writeError(w, http.StatusGone, "hls_reference_expired", "Reload the Live TV playlist to continue.")
			return
		}
		approval, _, err := approveLiveTVEndpoint(channel.streamURL, "hls-child")
		if err != nil {
			writeError(w, http.StatusBadRequest, "unsafe_stream_url", "The provider stream URL is not allowed.")
			return
		}
		if _, err := approval.validateURL(rawURI); err != nil {
			writeError(w, http.StatusBadRequest, "unsafe_stream_url", "The provider stream URL is not allowed.")
			return
		}
		s.proxyOrRewriteLiveTVHLSItem(w, r, liveTVHLSItemBinding{channelID: channel.ID, sourceURL: channel.streamURL, approval: approval}, rawURI, source.UserAgent)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Live TV HLS route was not found.")
	}
}

func (s *Server) liveTVRequestRequiresTranscode(r *http.Request, channel liveTVPlaybackChannel) (bool, error) {
	mode, err := s.liveMediaGrantDeliveryMode(r.Context(), r, channel.ID)
	if err != nil {
		return false, err
	}
	if !isHLSURL(channel.streamURL) {
		return true, nil
	}
	return mode == "transcode_required" || mode == "transcode", nil
}

func (s *Server) rewriteLiveTVHLS(w http.ResponseWriter, r *http.Request, channelID string, sourceURL string, playlistURL string, userAgent string) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	text, err := fetchLiveTVText(ctx, playlistURL, liveTVMaxPlaylistBytes, userAgent)
	if err != nil {
		writeError(w, http.StatusBadGateway, "hls_fetch_failed", "Unable to load the provider playlist.")
		return
	}
	rewritten, err := rewriteHLSPlaylist(channelID, playlistURL, text, r.URL.Query().Get("quality"), func(resolved string, qualityID string) string {
		return s.issueLiveTVHLSReference(channelID, sourceURL, resolved, qualityID, mediaGrantFromRequest(r))
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "hls_rewrite_failed", "Unable to prepare the provider playlist.")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(rewritten))
}

func playbackURLMediaGrant(r *http.Request) string {
	// The browser carries the scoped grant in an HttpOnly cookie; manifests
	// never copy credentials into nested resource URLs.
	return ""
}

func (s *Server) proxyOrRewriteLiveTVHLSItem(w http.ResponseWriter, r *http.Request, binding liveTVHLSItemBinding, itemURL string, userAgent string) {
	if isHLSURL(itemURL) {
		s.rewriteLiveTVHLS(w, r, binding.channelID, binding.sourceURL, itemURL, userAgent)
		return
	}
	proxyCachedLiveTVHLSItem(w, r, binding, itemURL, userAgent)
}

func (s *Server) handleLiveTVTranscodeManifest(w http.ResponseWriter, r *http.Request, user User, channel liveTVPlaybackChannel, source liveTVSourceRecord) {
	item := liveTVTranscodeItem(channel, source)
	quality := liveTVTranscodeQuality(r.URL.Query().Get("quality"))
	inputTransport, err := startLiveTVInputTransport(s.backgroundCtx, channel.ID, channel.streamURL, nil, source.UserAgent)
	if err != nil {
		s.writeLiveTVTranscodeFailure(w, err, channel.ID, quality)
		return
	}
	item.SourceURL = inputTransport.URL
	session, err := s.ensureTranscodeSessionWithInputTransport(user.ID, item, quality, "", 0, "transcode", "", false, inputTransport)
	if err != nil {
		s.writeLiveTVTranscodeFailure(w, err, channel.ID, quality)
		return
	}
	manifest, err := s.readLiveTVTranscodeManifest(session, channel.ID, quality, playbackURLMediaGrant(r))
	if err != nil {
		if session.transcodeError() != nil {
			s.writeLiveTVTranscodeFailure(w, session.transcodeError(), channel.ID, quality)
			return
		}
		w.Header().Set("Retry-After", "2")
		writeError(w, http.StatusAccepted, "live_tv_hls_starting", "The Live TV HLS session is starting. Retry shortly.")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(manifest))
}

func (s *Server) handleLiveTVTranscodeSegment(w http.ResponseWriter, r *http.Request, user User, channel liveTVPlaybackChannel, source liveTVSourceRecord) {
	name := filepath.Base(r.URL.Query().Get("name"))
	if name == "." || name == string(filepath.Separator) || !validHLSSegmentName(name) {
		writeError(w, http.StatusBadRequest, "bad_segment", "HLS segment name is invalid.")
		return
	}
	requestedGeneration, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("generation")))
	if err != nil || requestedGeneration < 0 {
		writeError(w, http.StatusBadRequest, "bad_generation", "The Live TV segment generation is invalid.")
		return
	}
	item := liveTVTranscodeItem(channel, source)
	quality := liveTVTranscodeQuality(r.URL.Query().Get("quality"))
	for recoveryPass := 0; recoveryPass < 2; recoveryPass++ {
		inputTransport, err := startLiveTVInputTransport(s.backgroundCtx, channel.ID, channel.streamURL, nil, source.UserAgent)
		if err != nil {
			s.writeLiveTVTranscodeFailure(w, err, channel.ID, quality)
			return
		}
		inputItem := item
		inputItem.SourceURL = inputTransport.URL
		session, err := s.ensureTranscodeSessionForSegmentWithIntentAndInput(user.ID, inputItem, quality, "", 0, "transcode", "", false, name, false, inputTransport)
		if err != nil {
			s.writeLiveTVTranscodeFailure(w, err, channel.ID, quality)
			return
		}
		if session.snapshot().generation != requestedGeneration {
			writeError(w, http.StatusGone, "live_tv_generation_expired", "Reload the Live TV playlist to continue from the current stream generation.")
			return
		}
		path := filepath.Join(session.dir, name)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(session.dir)+string(filepath.Separator)) {
			writeError(w, http.StatusBadRequest, "bad_segment", "HLS segment name is invalid.")
			return
		}
		if err := waitForHLSSegmentFileContext(r.Context(), s.shutdownDone(), session, path); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, errLongPollShutdown) {
				return
			}
			if session.snapshot().err != nil {
				recoveryTransport, transportErr := startLiveTVInputTransport(s.backgroundCtx, channel.ID, channel.streamURL, nil, source.UserAgent)
				if transportErr == nil {
					recoveryItem := item
					recoveryItem.SourceURL = recoveryTransport.URL
					recoveryRequest := transcodeStartRequest{userID: user.ID, item: recoveryItem, sourcePath: recoveryTransport.URL, quality: quality, audioMode: "transcode", inputTransport: recoveryTransport}
					recovered, recoverErr := s.recoverTranscodeSessionForDemandGuarded(r.Context(), s.transcodeSettings(), recoveryRequest, session, err)
					if recoverErr == nil && recovered != nil && recovered != session {
						continue
					}
					if recoverErr != nil || recovered == nil || recovered.inputTransport != recoveryTransport {
						recoveryTransport.Close()
					}
				}
				s.writeLiveTVTranscodeFailure(w, session.transcodeError(), channel.ID, quality)
				return
			}
			if session.isRunning() {
				w.Header().Set("Retry-After", "2")
				writeError(w, http.StatusServiceUnavailable, "segment_starting", "HLS segment is still being prepared. Retry shortly.")
				return
			}
			writeError(w, http.StatusNotFound, "segment_not_found", "HLS segment is not available.")
			return
		}
		releaseReader, ok := session.acquireReader()
		if !ok {
			continue
		}
		defer releaseReader()
		s.noteTranscodeSegmentServed(session, name)
		w.Header().Set("Content-Type", hlsSegmentContentType(name))
		w.Header().Set("Cache-Control", "private, max-age=30")
		http.ServeFile(w, r, path)
		return
	}
	writeError(w, http.StatusServiceUnavailable, "live_tv_hls_unavailable", "The Live TV HLS session could not recover in time.")
}

func liveTVTranscodeQuality(value string) string {
	switch normalized := normalizeLiveTVQualityID(value); normalized {
	case "source":
		return "original"
	default:
		return normalizeTranscodeQuality(normalized)
	}
}

func (s *Server) writeLiveTVTranscodeFailure(w http.ResponseWriter, err error, channelID, quality string) {
	message := ""
	if err != nil {
		message = strings.ToLower(err.Error())
		s.log.Warn("Live TV transcode failed", "error", err, "channel", channelID, "quality", quality)
	}
	if strings.Contains(message, "session limit") || strings.Contains(message, "under load") || strings.Contains(message, "admission") {
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusServiceUnavailable, "live_tv_transcode_capacity", "Live TV transcoding is at capacity. Try again shortly.")
		return
	}
	writeError(w, http.StatusServiceUnavailable, "live_tv_hls_unavailable", "Portico could not prepare this Live TV stream.")
}

func (s *Server) readLiveTVTranscodeManifest(session *transcodeSession, channelID string, quality string, accessToken string) (string, error) {
	deadline := time.Now().Add(transcodeManifestReadTimeout(false))
	for {
		bytes, err := os.ReadFile(session.manifest)
		if err == nil && len(bytes) > 0 {
			generation := session.snapshot().generation
			return rewriteLiveTVTranscodeHLSManifest(channelID, quality, accessToken, generation, string(bytes)), nil
		}
		state := session.snapshot()
		if time.Now().After(deadline) {
			if state.err != nil || state.terminalErr != nil {
				return "", session.transcodeError()
			}
			return "", err
		}
		wait := time.Until(deadline)
		if wait > time.Second {
			wait = time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-session.updateSignal():
			if !timer.Stop() {
				<-timer.C
			}
		case <-session.done:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func rewriteLiveTVTranscodeHLSManifest(channelID string, quality string, accessToken string, generation int, manifest string) string {
	var out strings.Builder
	insertedGeneration := false
	insertedBoundary := false
	for _, line := range strings.Split(strings.ReplaceAll(manifest, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !insertedGeneration && trimmed == "#EXTM3U" {
			out.WriteString(line)
			out.WriteByte('\n')
			out.WriteString("#PORTICO-TRANSCODE-GENERATION:")
			out.WriteString(strconv.Itoa(max(0, generation)))
			out.WriteByte('\n')
			out.WriteString("#EXT-X-DISCONTINUITY-SEQUENCE:")
			out.WriteString(strconv.Itoa(max(0, generation-1)))
			out.WriteByte('\n')
			insertedGeneration = true
			continue
		}
		if strings.HasPrefix(trimmed, "#EXT-X-MEDIA-SEQUENCE:") {
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "#EXT-X-MEDIA-SEQUENCE:"))
			sequence, err := strconv.ParseInt(raw, 10, 64)
			if err == nil && sequence >= 0 {
				out.WriteString("#EXT-X-MEDIA-SEQUENCE:")
				out.WriteString(strconv.FormatInt((int64(max(0, generation))<<32)+sequence, 10))
				out.WriteByte('\n')
				continue
			}
		}
		if strings.HasPrefix(trimmed, "#") {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		if generation > 0 && !insertedBoundary {
			out.WriteString("#EXT-X-DISCONTINUITY\n")
			insertedBoundary = true
		}
		out.WriteString(liveTVHLSSegmentRoute(channelID, quality, accessToken, generation, trimmed))
		out.WriteByte('\n')
	}
	return out.String()
}

func liveTVTranscodeItem(channel liveTVPlaybackChannel, source liveTVSourceRecord) MediaItem {
	return MediaItem{
		ID:              channel.ID,
		LibraryID:       source.ID,
		Type:            "live_channel",
		Title:           channel.Name,
		Labels:          []string{source.Type},
		SourceURL:       channel.streamURL,
		SourceUserAgent: source.UserAgent,
	}
}

func liveTVHLSPlaylistRoute(channelID string, accessToken string) string {
	var out strings.Builder
	out.WriteString("/api/live-tv/hls/")
	out.WriteString(url.PathEscape(channelID))
	out.WriteString("/playlist.m3u8")
	_ = accessToken
	return out.String()
}

func liveTVHLSSegmentRoute(channelID string, quality string, accessToken string, generation int, name string) string {
	var out strings.Builder
	out.WriteString("/api/live-tv/hls/")
	out.WriteString(url.PathEscape(channelID))
	out.WriteString("/segment?name=")
	out.WriteString(url.QueryEscape(filepath.Base(name)))
	out.WriteString("&quality=")
	out.WriteString(url.QueryEscape(normalizeLiveTVQualityID(quality)))
	out.WriteString("&generation=")
	out.WriteString(strconv.Itoa(max(0, generation)))
	_ = accessToken
	return out.String()
}

func proxyLiveTVURL(w http.ResponseWriter, r *http.Request, upstreamURL string, userAgent string, retryBeforeHeaders bool) {
	proxyLiveTVURLWithClient(w, r, upstreamURL, userAgent, retryBeforeHeaders, false)
}

func proxySharedLiveTVURLWithClient(w http.ResponseWriter, r *http.Request, upstreamURL string, userAgent string, retryBeforeHeaders bool, allowHDHomeRunLAN bool, maxRetrySeconds int) {
	approval, parsed, err := approveLiveTVEndpoint(upstreamURL, "stream")
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsafe_stream_url", "The provider stream URL is not allowed.")
		return
	}
	client, err := newApprovedLiveTVHTTPClient(r.Context(), approval, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsafe_stream_url", "The provider stream address is not allowed.")
		return
	}
	attempts := 1
	if retryBeforeHeaders {
		attempts = liveTVRetryAttempts(maxRetrySeconds)
	}
	sharedLiveTVStreams.serve(w, r, parsed.String(), effectiveLiveTVUserAgent(userAgent), client, attempts)
}

func proxyLiveTVURLWithClient(w http.ResponseWriter, r *http.Request, upstreamURL string, userAgent string, retryBeforeHeaders bool, allowHDHomeRunLAN bool) {
	approval, parsed, err := approveLiveTVEndpoint(upstreamURL, "stream")
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsafe_stream_url", "The provider stream URL is not allowed.")
		return
	}
	client := liveTVHTTPClientForContext(r.Context())
	if _, injected := r.Context().Value(liveTVHTTPClientContextKey{}).(*http.Client); !injected {
		client, err = newApprovedLiveTVHTTPClient(r.Context(), approval, nil)
		if err != nil {
			writeError(w, http.StatusBadRequest, "unsafe_stream_url", "The provider stream address is not allowed.")
			return
		}
	}
	attempts := 1
	if retryBeforeHeaders {
		attempts = 3
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, parsed.String(), nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", effectiveLiveTVUserAgent(userAgent))
		req.Header.Set("Accept", "*/*")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < attempts-1 {
				if !sleepContext(r.Context(), time.Duration(attempt+1)*700*time.Millisecond) {
					return
				}
			}
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("provider returned status %d", resp.StatusCode)
			if attempt < attempts-1 {
				if !sleepContext(r.Context(), time.Duration(attempt+1)*700*time.Millisecond) {
					return
				}
			}
			continue
		}
		copyLiveTVHeaders(w.Header(), resp.Header)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	_ = lastErr
	writeError(w, http.StatusBadGateway, "stream_unavailable", "The provider stream is not responding yet. Try Restart Stream.")
}

type liveTVStreamHub struct {
	mu                         sync.Mutex
	streams                    map[string]*sharedLiveTVStream
	slowSubscriberDisconnects  int
	slowSubscriberQueuedBytes  int64
	slowSubscriberQueuedMillis int64
	lastSlowSubscriberAt       time.Time
}

const liveTVStreamTerminalHeader = "X-Portico-Stream-Terminal"

type liveTVStreamTerminal struct {
	Code        string
	Action      string
	QueuedBytes int
	QueuedFor   time.Duration
}

type liveTVStreamSubscriber struct {
	chunks      chan []byte
	terminal    chan liveTVStreamTerminal
	queuedBytes int
	queuedSince time.Time
}

type liveTVStreamHubMetrics struct {
	SlowSubscriberDisconnects  int
	SlowSubscriberQueuedBytes  int64
	SlowSubscriberQueuedMillis int64
	LastSlowSubscriberAt       time.Time
}

type sharedLiveTVStream struct {
	key         string
	upstreamURL string
	userAgent   string
	client      *http.Client
	attempts    int
	cancel      context.CancelFunc
	onDone      func(string)

	mu              sync.Mutex
	ready           chan struct{}
	done            chan struct{}
	headers         http.Header
	status          int
	err             error
	subscribers     map[*liveTVStreamSubscriber]struct{}
	onSubscriberLag func(liveTVStreamTerminal)
}

func newLiveTVStreamHub() *liveTVStreamHub {
	return &liveTVStreamHub{streams: map[string]*sharedLiveTVStream{}}
}

func (h *liveTVStreamHub) metrics() liveTVStreamHubMetrics {
	h.mu.Lock()
	defer h.mu.Unlock()
	return liveTVStreamHubMetrics{
		SlowSubscriberDisconnects:  h.slowSubscriberDisconnects,
		SlowSubscriberQueuedBytes:  h.slowSubscriberQueuedBytes,
		SlowSubscriberQueuedMillis: h.slowSubscriberQueuedMillis,
		LastSlowSubscriberAt:       h.lastSlowSubscriberAt,
	}
}

func (h *liveTVStreamHub) serve(w http.ResponseWriter, r *http.Request, upstreamURL string, userAgent string, client *http.Client, attempts int) {
	key := strings.Join([]string{upstreamURL, userAgent}, "\x00")
	stream := h.stream(key, upstreamURL, userAgent, client, attempts)
	subscriber, release, err := stream.subscribe(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "stream_unavailable", "The provider stream is not responding yet. Try Restart Stream.")
		return
	}
	defer release()
	w.Header().Add("Trailer", liveTVStreamTerminalHeader)
	copySharedLiveTVHeaders(w.Header(), stream.headers)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(stream.status)
	flusher, _ := w.(http.Flusher)
	for {
		select {
		case terminal, ok := <-subscriber.terminal:
			if ok {
				w.Header().Set(liveTVStreamTerminalHeader, formatLiveTVStreamTerminal(terminal))
			}
			return
		case chunk, ok := <-subscriber.chunks:
			if !ok {
				select {
				case terminal := <-subscriber.terminal:
					w.Header().Set(liveTVStreamTerminalHeader, formatLiveTVStreamTerminal(terminal))
				default:
				}
				return
			}
			stream.markSubscriberDelivered(subscriber, len(chunk))
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (h *liveTVStreamHub) stream(key string, upstreamURL string, userAgent string, client *http.Client, attempts int) *sharedLiveTVStream {
	h.mu.Lock()
	defer h.mu.Unlock()
	if stream, ok := h.streams[key]; ok {
		return stream
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream := &sharedLiveTVStream{
		key:         key,
		upstreamURL: upstreamURL,
		userAgent:   userAgent,
		client:      client,
		attempts:    max(1, attempts),
		cancel:      cancel,
		ready:       make(chan struct{}),
		done:        make(chan struct{}),
		subscribers: map[*liveTVStreamSubscriber]struct{}{},
		onDone: func(key string) {
			h.mu.Lock()
			delete(h.streams, key)
			h.mu.Unlock()
		},
	}
	stream.onSubscriberLag = func(terminal liveTVStreamTerminal) {
		h.mu.Lock()
		h.slowSubscriberDisconnects++
		h.slowSubscriberQueuedBytes += int64(terminal.QueuedBytes)
		h.slowSubscriberQueuedMillis += terminal.QueuedFor.Milliseconds()
		h.lastSlowSubscriberAt = time.Now().UTC()
		h.mu.Unlock()
	}
	h.streams[key] = stream
	go stream.run(ctx)
	return stream
}

func (s *sharedLiveTVStream) subscribe(ctx context.Context) (*liveTVStreamSubscriber, func(), error) {
	subscriber := &liveTVStreamSubscriber{chunks: make(chan []byte, 32), terminal: make(chan liveTVStreamTerminal, 1)}
	s.mu.Lock()
	select {
	case <-s.done:
		err := s.err
		s.mu.Unlock()
		if err == nil {
			err = errors.New("live tv stream ended")
		}
		return nil, func() {}, err
	default:
		s.subscribers[subscriber] = struct{}{}
	}
	s.mu.Unlock()
	select {
	case <-s.ready:
	case <-ctx.Done():
		s.unsubscribe(subscriber)
		return nil, func() {}, ctx.Err()
	}
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	if err != nil {
		s.unsubscribe(subscriber)
		return nil, func() {}, err
	}
	return subscriber, func() { s.unsubscribe(subscriber) }, nil
}

func (s *sharedLiveTVStream) run(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		terminal := liveTVStreamTerminal{Code: "live_tv_stream_ended", Action: "restart_stream"}
		if s.err != nil {
			terminal = liveTVStreamTerminal{Code: "live_tv_stream_error", Action: "restart_stream"}
		}
		for subscriber := range s.subscribers {
			subscriber.closeWithTerminal(terminal)
			delete(s.subscribers, subscriber)
		}
		s.mu.Unlock()
		close(s.done)
		s.onDone(s.key)
	}()
	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < s.attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.upstreamURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", s.userAgent)
		req.Header.Set("Accept", "*/*")
		resp, err = s.client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < s.attempts-1 {
				if !sleepContext(ctx, time.Duration(attempt+1)*700*time.Millisecond) {
					return
				}
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("provider returned status %d", resp.StatusCode)
			_ = resp.Body.Close()
			resp = nil
			if attempt < s.attempts-1 {
				if !sleepContext(ctx, time.Duration(attempt+1)*700*time.Millisecond) {
					return
				}
			}
			continue
		}
		break
	}
	if resp == nil {
		s.mu.Lock()
		s.err = lastErr
		s.mu.Unlock()
		close(s.ready)
		return
	}
	defer resp.Body.Close()
	s.mu.Lock()
	s.headers = resp.Header.Clone()
	s.status = resp.StatusCode
	s.mu.Unlock()
	close(s.ready)
	buffer := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			s.broadcast(buffer[:n])
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.mu.Lock()
				s.err = err
				s.mu.Unlock()
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (s *sharedLiveTVStream) broadcast(chunk []byte) {
	payload := append([]byte(nil), chunk...)
	s.mu.Lock()
	defer s.mu.Unlock()
	for subscriber := range s.subscribers {
		select {
		case subscriber.chunks <- payload:
			if subscriber.queuedBytes == 0 {
				subscriber.queuedSince = time.Now().UTC()
			}
			subscriber.queuedBytes += len(payload)
		default:
			terminal := liveTVStreamTerminal{
				Code:        "live_tv_subscriber_lag",
				Action:      "reconnect_stream",
				QueuedBytes: subscriber.queuedBytes,
			}
			if !subscriber.queuedSince.IsZero() {
				terminal.QueuedFor = time.Since(subscriber.queuedSince)
			}
			subscriber.closeWithTerminal(terminal)
			delete(s.subscribers, subscriber)
			if s.onSubscriberLag != nil {
				s.onSubscriberLag(terminal)
			}
		}
	}
	if len(s.subscribers) == 0 {
		s.cancel()
	}
}

func (s *sharedLiveTVStream) markSubscriberDelivered(subscriber *liveTVStreamSubscriber, size int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if subscriber.queuedBytes > 0 {
		subscriber.queuedBytes = max(0, subscriber.queuedBytes-size)
		if subscriber.queuedBytes == 0 {
			subscriber.queuedSince = time.Time{}
		}
	}
}

func (s *sharedLiveTVStream) unsubscribe(subscriber *liveTVStreamSubscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscribers[subscriber]; !ok {
		return
	}
	subscriber.closeWithTerminal(liveTVStreamTerminal{})
	delete(s.subscribers, subscriber)
	if len(s.subscribers) == 0 {
		s.cancel()
	}
}

func (subscriber *liveTVStreamSubscriber) closeWithTerminal(terminal liveTVStreamTerminal) {
	if terminal.Code != "" {
		subscriber.terminal <- terminal
	}
	close(subscriber.chunks)
	close(subscriber.terminal)
}

func formatLiveTVStreamTerminal(terminal liveTVStreamTerminal) string {
	return fmt.Sprintf("code=%s; action=%s; queued_bytes=%d; queued_ms=%d", terminal.Code, terminal.Action, terminal.QueuedBytes, terminal.QueuedFor.Milliseconds())
}

func copySharedLiveTVHeaders(dst http.Header, src http.Header) {
	if value := src.Get("Content-Type"); value != "" {
		dst.Set("Content-Type", value)
	}
}

func (s *Server) proxyLiveTVLogo(w http.ResponseWriter, r *http.Request, upstreamURL string, userAgent string) {
	parsed, err := validateExternalURL(upstreamURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsafe_logo_url", "The provider logo URL is not allowed.")
		return
	}
	cacheKey := sha256.Sum256([]byte(parsed.String() + "\x00" + effectiveLiveTVUserAgent(userAgent)))
	cacheRoot := filepath.Join(s.cfg.AppDataDir, "image-cache", "live-tv-logos")
	cacheBase := filepath.Join(cacheRoot, hex.EncodeToString(cacheKey[:]))
	cachePath := cacheBase + ".bin"
	mimePath := cacheBase + ".mime"
	missingPath := cacheBase + ".missing"
	writeMiss := func() {
		if os.MkdirAll(cacheRoot, 0o755) == nil {
			_ = os.WriteFile(missingPath, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
		}
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.WriteHeader(http.StatusNoContent)
	}
	if data, readErr := os.ReadFile(cachePath); readErr == nil && len(data) > 0 {
		contentType := ""
		if storedType, typeErr := os.ReadFile(mimePath); typeErr == nil {
			contentType = strings.TrimSpace(string(storedType))
		}
		if contentType == "" {
			contentType = http.DetectContentType(data)
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
		w.Header().Set("ETag", `"`+hex.EncodeToString(cacheKey[:])+`"`)
		_, _ = w.Write(data)
		return
	}
	if missing, statErr := os.Stat(missingPath); statErr == nil && time.Since(missing.ModTime()) < 5*time.Minute {
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "logo_request_failed", "Unable to load the provider logo.")
		return
	}
	req.Header.Set("User-Agent", effectiveLiveTVUserAgent(userAgent))
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	resp, err := liveTVHTTPClientForContext(r.Context()).Do(req)
	if err != nil {
		writeMiss()
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeMiss()
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) == 0 || len(data) > 4<<20 {
		writeMiss()
		return
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if contentType == "" {
		contentType = strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	}
	if !strings.HasPrefix(contentType, "image/") {
		writeMiss()
		return
	}
	if err := os.MkdirAll(cacheRoot, 0o755); err == nil {
		_ = os.Remove(missingPath)
		tempPath := cachePath + ".tmp-" + randomID("asset")
		if writeErr := os.WriteFile(tempPath, data, 0o644); writeErr == nil {
			if renameErr := os.Rename(tempPath, cachePath); renameErr == nil {
				_ = os.WriteFile(mimePath, []byte(contentType+"\n"), 0o644)
			} else {
				_ = os.Remove(tempPath)
			}
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	w.Header().Set("ETag", `"`+hex.EncodeToString(cacheKey[:])+`"`)
	_, _ = w.Write(data)
}

type liveTVHLSItemURLBuilder func(resolved string, qualityID string) string

func rewriteHLSPlaylist(channelID string, baseURL string, text string, qualityID string, itemURL liveTVHLSItemURLBuilder) (string, error) {
	base, err := validateExternalURL(baseURL)
	if err != nil {
		return "", err
	}
	if itemURL == nil {
		return "", errors.New("HLS item URL builder is required")
	}
	qualityID = normalizeLiveTVQualityID(qualityID)
	if qualityID != "" && qualityID != "auto" && strings.Contains(text, "#EXT-X-STREAM-INF") {
		return rewriteFilteredHLSMasterPlaylist(channelID, base, text, qualityID, itemURL)
	}
	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			out.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-MAP:") || strings.HasPrefix(line, "#EXT-X-MEDIA:") || strings.HasPrefix(line, "#EXT-X-I-FRAME-STREAM-INF:") {
			out.WriteString(rewriteHLSURILine(base, line, qualityID, itemURL))
			out.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(line, "#") {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		resolved, err := resolveProviderURL(base, line)
		if err != nil {
			return "", err
		}
		out.WriteString(itemURL(resolved, qualityID))
		out.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return out.String(), nil
}

func rewriteFilteredHLSMasterPlaylist(channelID string, base *url.URL, text string, qualityID string, itemURL liveTVHLSItemURLBuilder) (string, error) {
	lines, err := scanHLSLines(text)
	if err != nil {
		return "", err
	}
	variants := hlsMasterVariants(lines)
	if len(variants) == 0 {
		return rewriteHLSPlaylist(channelID, base.String(), text, "auto", itemURL)
	}
	selected := selectHLSVariant(variants, qualityID)
	var out strings.Builder
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if variant, ok := variantStartingAt(variants, i); ok {
			if variant.uriIndex == selected.uriIndex {
				out.WriteString(rewriteHLSURILine(base, line, qualityID, itemURL))
				out.WriteByte('\n')
				resolved, err := resolveProviderURL(base, lines[variant.uriIndex])
				if err != nil {
					return "", err
				}
				out.WriteString(itemURL(resolved, qualityID))
				out.WriteByte('\n')
			}
			i = variant.uriIndex
			continue
		}
		if line == "" {
			out.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-MAP:") || strings.HasPrefix(line, "#EXT-X-MEDIA:") || strings.HasPrefix(line, "#EXT-X-I-FRAME-STREAM-INF:") {
			out.WriteString(rewriteHLSURILine(base, line, qualityID, itemURL))
			out.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(line, "#") {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		resolved, err := resolveProviderURL(base, line)
		if err != nil {
			return "", err
		}
		out.WriteString(itemURL(resolved, qualityID))
		out.WriteByte('\n')
	}
	return out.String(), nil
}

func scanHLSLines(text string) ([]string, error) {
	lines := []string{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

type hlsVariant struct {
	infoIndex int
	uriIndex  int
	bandwidth int
	height    int
}

func hlsMasterVariants(lines []string) []hlsVariant {
	var variants []hlsVariant
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			candidate := strings.TrimSpace(lines[j])
			if candidate == "" {
				continue
			}
			if strings.HasPrefix(candidate, "#") {
				continue
			}
			variants = append(variants, hlsVariant{
				infoIndex: i,
				uriIndex:  j,
				bandwidth: hlsAttributeInt(line, "BANDWIDTH"),
				height:    hlsResolutionHeight(line),
			})
			i = j
			break
		}
	}
	return variants
}

func variantStartingAt(variants []hlsVariant, index int) (hlsVariant, bool) {
	for _, variant := range variants {
		if variant.infoIndex == index {
			return variant, true
		}
	}
	return hlsVariant{}, false
}

func selectHLSVariant(variants []hlsVariant, qualityID string) hlsVariant {
	const veryHigh = int(^uint(0) >> 1)
	constraint := liveTVQualityConstraintFor(qualityID)
	if constraint.source {
		return highestHLSVariant(variants)
	}
	selected := hlsVariant{bandwidth: -1}
	for _, variant := range variants {
		if constraint.maxBandwidth > 0 && variant.bandwidth > constraint.maxBandwidth {
			continue
		}
		if constraint.maxHeight > 0 && variant.height > constraint.maxHeight {
			continue
		}
		if variant.bandwidth > selected.bandwidth {
			selected = variant
		}
	}
	if selected.bandwidth >= 0 {
		return selected
	}
	lowest := hlsVariant{bandwidth: veryHigh}
	for _, variant := range variants {
		bandwidth := variant.bandwidth
		if bandwidth == 0 {
			bandwidth = veryHigh - 1
		}
		if bandwidth < lowest.bandwidth {
			lowest = variant
			lowest.bandwidth = bandwidth
		}
	}
	return lowest
}

func highestHLSVariant(variants []hlsVariant) hlsVariant {
	selected := variants[0]
	for _, variant := range variants[1:] {
		if variant.bandwidth > selected.bandwidth || (variant.bandwidth == selected.bandwidth && variant.height > selected.height) {
			selected = variant
		}
	}
	return selected
}

func hlsAttributeInt(line string, name string) int {
	value := hlsAttributeValue(line, name)
	if value == "" {
		return 0
	}
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func hlsResolutionHeight(line string) int {
	value := hlsAttributeValue(line, "RESOLUTION")
	parts := strings.Split(value, "x")
	if len(parts) != 2 {
		return 0
	}
	height, _ := strconv.Atoi(parts[1])
	return height
}

func hlsAttributeValue(line string, name string) string {
	prefix := name + "="
	index := strings.Index(line, prefix)
	if index < 0 {
		return ""
	}
	rest := line[index+len(prefix):]
	if strings.HasPrefix(rest, `"`) {
		rest = strings.TrimPrefix(rest, `"`)
		end := strings.Index(rest, `"`)
		if end < 0 {
			return rest
		}
		return rest[:end]
	}
	end := strings.Index(rest, ",")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func rewriteHLSURILine(base *url.URL, line string, qualityID string, itemURL liveTVHLSItemURLBuilder) string {
	const marker = `URI="`
	start := strings.Index(line, marker)
	if start < 0 {
		return line
	}
	start += len(marker)
	end := strings.Index(line[start:], `"`)
	if end < 0 {
		return line
	}
	raw := line[start : start+end]
	resolved, err := resolveProviderURL(base, raw)
	if err != nil {
		return line
	}
	replacement := itemURL(resolved, qualityID)
	return line[:start] + replacement + line[start+end:]
}

func resolveProviderURL(base *url.URL, value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(parsed)
	approval, _, err := approveLiveTVEndpoint(base.String(), "playlist-child")
	if err != nil {
		return "", err
	}
	if _, err := approval.validateURL(resolved.String()); err != nil {
		return "", err
	}
	return resolved.String(), nil
}

func copyLiveTVHeaders(dst http.Header, src http.Header) {
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

func isHLSURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return strings.Contains(strings.ToLower(value), ".m3u8")
	}
	return strings.Contains(strings.ToLower(parsed.Path), ".m3u8")
}

func fetchLiveTVText(ctx context.Context, rawURL string, maxBytes int64, userAgent string) (string, error) {
	approval, parsed, err := approveLiveTVEndpoint(rawURL, "provider-read")
	if err != nil {
		return "", errors.New("Provider URL is not allowed.")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", errors.New("Provider request could not be created.")
	}
	req.Header.Set("User-Agent", effectiveLiveTVUserAgent(userAgent))
	req.Header.Set("Accept", "*/*")
	client := liveTVHTTPClientForContext(ctx)
	if _, injected := ctx.Value(liveTVHTTPClientContextKey{}).(*http.Client); !injected {
		client, err = newApprovedLiveTVHTTPClient(ctx, approval, nil)
		if err != nil {
			return "", errors.New("Provider address is not allowed.")
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("Provider did not respond.")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Provider returned status %d.", resp.StatusCode)
	}
	reader := io.LimitReader(resp.Body, maxBytes+1)
	bytes, err := io.ReadAll(reader)
	if err != nil {
		return "", errors.New("Provider response could not be read.")
	}
	if int64(len(bytes)) > maxBytes {
		return "", errors.New("Provider response is larger than the configured safety limit.")
	}
	return string(bytes), nil
}

func fetchHDHomeRunText(ctx context.Context, rawURL string, maxBytes int64, userAgent string) (string, error) {
	parsed, err := validateHDHomeRunURL(rawURL)
	if err != nil {
		return "", errors.New("HDHomeRun URL is not allowed.")
	}
	approval, _, err := approveLiveTVEndpoint(parsed.String(), "hdhomerun-read")
	if err != nil {
		return "", errors.New("HDHomeRun URL is not allowed.")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", errors.New("HDHomeRun request could not be created.")
	}
	req.Header.Set("User-Agent", effectiveLiveTVUserAgent(userAgent))
	req.Header.Set("Accept", "application/json,*/*")
	client, err := newApprovedLiveTVHTTPClient(ctx, approval, nil)
	if err != nil {
		return "", errors.New("HDHomeRun address is not allowed.")
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("HDHomeRun device did not respond.")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HDHomeRun device returned status %d.", resp.StatusCode)
	}
	reader := io.LimitReader(resp.Body, maxBytes+1)
	bytes, err := io.ReadAll(reader)
	if err != nil {
		return "", errors.New("HDHomeRun response could not be read.")
	}
	if int64(len(bytes)) > maxBytes {
		return "", errors.New("HDHomeRun response is larger than the configured safety limit.")
	}
	return string(bytes), nil
}

// liveTVHTTPClientContextKey is intentionally private to this package. It lets
// handler-level tests exercise the complete HLS proxy and media-grant path with
// a deterministic provider transport, while production requests always retain
// the SSRF-hardened client below.
type liveTVHTTPClientContextKey struct{}

func liveTVHTTPClientForContext(ctx context.Context) *http.Client {
	if ctx != nil {
		if client, ok := ctx.Value(liveTVHTTPClientContextKey{}).(*http.Client); ok && client != nil {
			return client
		}
	}
	return liveTVHTTPClient()
}

var (
	liveTVSharedHTTPClient    = newLiveTVHTTPClient(safeLiveTVDialContext, 64)
	hdhomerunSharedHTTPClient = newLiveTVHTTPClient(hdhomerunDialContext, 16)
)

func newLiveTVHTTPClient(dialContext func(context.Context, string, string) (net.Conn, error), maxConnectionsPerHost int) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 20 * time.Second
	transport.DialContext = dialContext
	transport.MaxIdleConns = max(128, maxConnectionsPerHost*2)
	transport.MaxIdleConnsPerHost = max(16, maxConnectionsPerHost/2)
	transport.MaxConnsPerHost = maxConnectionsPerHost
	transport.IdleConnTimeout = 90 * time.Second
	transport.ForceAttemptHTTP2 = true
	return &http.Client{Transport: transport}
}

func liveTVHTTPClient() *http.Client {
	return liveTVSharedHTTPClient
}

func hdhomerunHTTPClient() *http.Client {
	return hdhomerunSharedHTTPClient
}

func safeLiveTVDialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isUnsafeIP(ip.IP) {
			continue
		}
		dialer := net.Dialer{Timeout: 20 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
	}
	return nil, errors.New("provider host resolved to a blocked network")
}

func hdhomerunDialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok || !isHDHomeRunAddr(addr) {
			continue
		}
		dialer := net.Dialer{Timeout: 20 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
	}
	return nil, errors.New("HDHomeRun host did not resolve to a supported LAN address")
}

func validateExternalURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("only http and https URLs are supported")
	}
	host := parsed.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") {
		return nil, errors.New("host is not allowed")
	}
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if isUnsafeAddr(ip) {
			return nil, errors.New("host is not allowed")
		}
	}
	return parsed, nil
}

func normalizeHDHomeRunBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if strings.HasSuffix(parsed.Path, "/discover.json") || strings.HasSuffix(parsed.Path, "/lineup.json") {
		parsed.Path = strings.TrimSuffix(strings.TrimSuffix(parsed.Path, "/discover.json"), "/lineup.json")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}

func hdhomerunURL(baseURL string, path string) (string, error) {
	base, err := validateHDHomeRunURL(baseURL)
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(path, "/")
	base.RawQuery = ""
	base.Fragment = ""
	return base.String(), nil
}

func validateHDHomeRunURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("only http and https URLs are supported")
	}
	if parsed.User != nil {
		return nil, errors.New("credentials in URLs are not supported")
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, errors.New("host is not allowed")
	}
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if !isHDHomeRunAddr(ip) {
			return nil, errors.New("host is not a supported HDHomeRun LAN address")
		}
	}
	return parsed, nil
}

func isUnsafeIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	return isUnsafeAddr(addr)
}

func isUnsafeAddr(addr netip.Addr) bool {
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified()
}

func isHDHomeRunAddr(addr netip.Addr) bool {
	if addr.IsLoopback() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	return addr.IsPrivate() || addr.IsLinkLocalUnicast()
}

func effectiveLiveTVUserAgent(userAgent string) string {
	userAgent = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, userAgent)
	userAgent = strings.Join(strings.Fields(userAgent), " ")
	if userAgent == "" {
		return liveTVUserAgent
	}
	runes := []rune(userAgent)
	if len(runes) > 160 {
		return string(runes[:160])
	}
	return userAgent
}

func decodeJSONLimit(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", "Request body must be valid JSON and within the upload size limit.")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "bad_json", "Request body must contain exactly one JSON value.")
		return false
	}
	return true
}

func sanitizeLiveTVError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, marker := range []string{"password=", "username=", "token=", "auth=", "key="} {
		lower := strings.ToLower(message)
		if idx := strings.Index(lower, marker); idx >= 0 {
			message = message[:idx] + marker + "redacted"
		}
	}
	if len(message) > 220 {
		message = message[:220]
	}
	return message
}

func stableLiveTVID(prefix string, values ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return prefix + "_" + hex.EncodeToString(hash[:])[:24]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeLiveTVKey(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func normalizeLiveTVList(values []string, limit int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t'
		}) {
			part = strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
			if part == "" {
				continue
			}
			if len(part) > 80 {
				part = part[:80]
			}
			key := normalizeLiveTVKey(part)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, part)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func encodeLiveTVList(values []string) string {
	normalized := normalizeLiveTVList(values, 120)
	if len(normalized) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func decodeLiveTVList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err == nil {
		return normalizeLiveTVList(values, 120)
	}
	return normalizeLiveTVList([]string{value}, 120)
}

func naturalLess(left string, right string) bool {
	leftFloat, leftErr := strconv.ParseFloat(strings.TrimSpace(left), 64)
	rightFloat, rightErr := strconv.ParseFloat(strings.TrimSpace(right), 64)
	if leftErr == nil && rightErr == nil {
		return leftFloat < rightFloat
	}
	return strings.ToLower(left) < strings.ToLower(right)
}

func boundedInt(value int, minValue int, maxValue int, fallback int) int {
	if value == 0 {
		value = fallback
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func boundedIntFromString(value string, minValue int, maxValue int, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return boundedInt(parsed, minValue, maxValue, fallback)
}

func xtreamStreamID(raw json.RawMessage) string {
	var number int
	if err := json.Unmarshal(raw, &number); err == nil && number > 0 {
		return strconv.Itoa(number)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	return ""
}
