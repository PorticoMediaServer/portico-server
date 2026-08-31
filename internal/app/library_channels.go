package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/librarychannels"
)

const (
	libraryChannelMaxRequestBytes  = 512 << 10
	libraryChannelGenerationBudget = 12 * time.Second
	libraryChannelGenerationLease  = 30 * time.Second
	libraryChannelGuideMaximum     = 7 * 24 * time.Hour
)

type libraryChannelLogoDocument struct {
	Source              librarychannels.LogoSource    `json:"source"`
	Ref                 string                        `json:"ref,omitempty"`
	URL                 string                        `json:"url,omitempty"`
	MIMEType            string                        `json:"mimeType,omitempty"`
	BugEnabled          bool                          `json:"bugEnabled"`
	BugOverheadAccepted bool                          `json:"bugOverheadAccepted"`
	BugCorner           librarychannels.LogoCorner    `json:"bugCorner"`
	BugWidthPercent     float64                       `json:"bugWidthPercent"`
	BugInsetPercent     float64                       `json:"bugInsetPercent"`
	BugTreatment        librarychannels.LogoTreatment `json:"bugTreatment"`
}

type libraryChannelDocument struct {
	ID               string                     `json:"id"`
	SourceType       string                     `json:"sourceType"`
	Name             string                     `json:"name"`
	Description      string                     `json:"description"`
	Enabled          bool                       `json:"enabled"`
	SortOrder        int                        `json:"sortOrder"`
	Timezone         string                     `json:"timezone"`
	DefaultRuleID    string                     `json:"defaultRuleId"`
	QualityProfile   string                     `json:"qualityProfile"`
	Logo             libraryChannelLogoDocument `json:"logo"`
	TemplateKey      string                     `json:"templateKey,omitempty"`
	TemplateVersion  int                        `json:"templateVersion,omitempty"`
	ConfigRevision   int64                      `json:"configRevision"`
	GeneratedThrough string                     `json:"generatedThrough,omitempty"`
	HealthState      string                     `json:"healthState"`
	HealthMessageID  string                     `json:"healthMessageId,omitempty"`
	CreatedAt        string                     `json:"createdAt"`
	UpdatedAt        string                     `json:"updatedAt"`
}

type libraryChannelSummaryDocument struct {
	ID          string   `json:"id"`
	SourceType  string   `json:"sourceType"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	LogoURL     string   `json:"logoUrl,omitempty"`
	Actions     []string `json:"actions"`
}

type libraryChannelRuleDocument struct {
	ID              string                         `json:"id"`
	Name            string                         `json:"name"`
	Enabled         bool                           `json:"enabled"`
	SortOrder       int                            `json:"sortOrder"`
	Query           json.RawMessage                `json:"query"`
	SelectionMode   librarychannels.SelectionMode  `json:"selectionMode"`
	EpisodeMode     librarychannels.EpisodeMode    `json:"episodeMode"`
	ExhaustionMode  librarychannels.ExhaustionMode `json:"exhaustionMode"`
	DedupeWindow    int                            `json:"dedupeWindow"`
	MaxConsecutive  int                            `json:"maxConsecutive"`
	Config          json.RawMessage                `json:"config"`
	TemplateKey     string                         `json:"templateKey,omitempty"`
	TemplateVersion int                            `json:"templateVersion,omitempty"`
}

type libraryChannelBlockDocument struct {
	ID              string `json:"id"`
	RuleID          string `json:"ruleId"`
	FallbackRuleID  string `json:"fallbackRuleId,omitempty"`
	Name            string `json:"name"`
	Enabled         bool   `json:"enabled"`
	WeekdayMask     uint8  `json:"weekdayMask"`
	StartMinute     int    `json:"startMinute"`
	EndMinute       int    `json:"endMinute"`
	Priority        int    `json:"priority"`
	Anchored        bool   `json:"anchored"`
	AllowOverrun    bool   `json:"allowOverrun"`
	SortOrder       int    `json:"sortOrder"`
	TemplateKey     string `json:"templateKey,omitempty"`
	TemplateVersion int    `json:"templateVersion,omitempty"`
}

type libraryChannelAggregateDocument struct {
	Channel libraryChannelDocument        `json:"channel"`
	Rules   []libraryChannelRuleDocument  `json:"rules"`
	Blocks  []libraryChannelBlockDocument `json:"blocks"`
}

type libraryChannelConfigurationRequest struct {
	ExpectedRevision int64                         `json:"expectedRevision"`
	Name             string                        `json:"name"`
	Description      string                        `json:"description"`
	Enabled          bool                          `json:"enabled"`
	SortOrder        int                           `json:"sortOrder"`
	Timezone         string                        `json:"timezone"`
	Reshuffle        bool                          `json:"reshuffle,omitempty"`
	DefaultRuleID    string                        `json:"defaultRuleId"`
	QualityProfile   string                        `json:"qualityProfile"`
	Logo             libraryChannelLogoDocument    `json:"logo"`
	TemplateKey      string                        `json:"templateKey,omitempty"`
	TemplateVersion  int                           `json:"templateVersion,omitempty"`
	Rules            []libraryChannelRuleDocument  `json:"rules"`
	Blocks           []libraryChannelBlockDocument `json:"blocks"`
}

type libraryChannelGuideEntryDocument struct {
	ID              string                         `json:"id"`
	ChannelID       string                         `json:"channelId"`
	SourceType      string                         `json:"sourceType"`
	Kind            librarychannels.EntryKind      `json:"kind"`
	StartsAt        string                         `json:"startsAt"`
	EndsAt          string                         `json:"endsAt"`
	MediaID         string                         `json:"mediaId,omitempty"`
	Title           string                         `json:"title,omitempty"`
	Subtitle        string                         `json:"subtitle,omitempty"`
	Summary         string                         `json:"summary,omitempty"`
	ContentRating   string                         `json:"contentRating,omitempty"`
	Artwork         map[string]string              `json:"artwork"`
	Availability    string                         `json:"availability"`
	ReasonMessageID librarychannels.ScheduleReason `json:"reasonMessageId,omitempty"`
}

type libraryChannelGuideDocument struct {
	SourceType string                             `json:"sourceType"`
	Channel    libraryChannelSummaryDocument      `json:"channel"`
	From       string                             `json:"from"`
	To         string                             `json:"to"`
	ServerTime string                             `json:"serverTime"`
	Programs   []libraryChannelGuideEntryDocument `json:"programs"`
	PageInfo   CursorPageInfo                     `json:"pageInfo"`
}

type libraryChannelLinearGuideDocument struct {
	SourceType string                             `json:"sourceType"`
	Channels   []libraryChannelSummaryDocument    `json:"channels"`
	Programs   []libraryChannelGuideEntryDocument `json:"programs"`
	From       string                             `json:"from"`
	To         string                             `json:"to"`
	ServerTime string                             `json:"serverTime"`
	PageInfo   CursorPageInfo                     `json:"pageInfo"`
}

type libraryChannelTuneRequest struct {
	At               string                      `json:"at,omitempty"`
	ClientInstanceID string                      `json:"clientInstanceId,omitempty"`
	ClientProfile    PlaybackClientProfile       `json:"clientProfile,omitempty"`
	Intent           PlaybackIntent              `json:"intent,omitempty"`
	Replacement      *PlaybackReplacementRequest `json:"replacement,omitempty"`
}

// startLibraryChannelPlaybackByID resolves the authoritative current program
// and constructs the same canonical playback response used by the tune route.
// Receiver clients use this instead of guessing a media item from guide data.
func (s *Server) startLibraryChannelPlaybackByID(r *http.Request, user User, channelID string, clientProfile PlaybackClientProfile, intent PlaybackIntent, clientInstanceID string, externalReplacement *playbackReplacementPlan) (PlaybackResponse, *playbackStartHTTPError) {
	if !canPlayLiveTV(user) || !hasPermission(user, "playMedia") {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusForbidden, code: "forbidden", message: "This profile cannot play Library Channels."}
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusBadRequest, code: "library_channel_required", message: "A Library Channel ID is required."}
	}
	store := librarychannels.NewStore(s.dbHandle())
	aggregate, err := store.GetAggregate(r.Context(), channelID)
	if err != nil || !aggregate.Channel.Enabled {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusNotFound, code: "library_channel_not_found", message: "Library Channel was not found."}
	}
	now := time.Now().UTC()
	entries, err := store.LoadActiveSchedule(r.Context(), channelID, now, now.Add(time.Second))
	if err != nil {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusServiceUnavailable, code: "library_channel_playback_unavailable", message: "Portico could not load the Library Channel schedule."}
	}
	if len(entries) == 0 {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusNotFound, code: "library_channel_program_unavailable", message: "No Library Channel program is scheduled at this time."}
	}
	playback, err := s.startLibraryChannelPlayback(r, user, aggregate.Channel, entries[0], clientProfile, intent, clientInstanceID, nil, externalReplacement)
	if err == nil {
		return playback, nil
	}
	var startErr *playbackStartHTTPError
	if errors.As(err, &startErr) {
		return PlaybackResponse{}, startErr
	}
	switch {
	case errors.Is(err, errPlaybackReplacementRequired):
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusConflict, code: "replacement_required", message: "This client already owns active playback. Supply its exact replacement authority envelope."}
	case errors.Is(err, errPlaybackSessionLimit):
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusTooManyRequests, code: "playback_session_limit", message: "This profile has reached its active playback limit.", retryAfter: "15"}
	case errors.Is(err, librarychannels.ErrProgramRestricted):
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusForbidden, code: "library_channel_program_restricted", message: "This profile cannot play the current Library Channel program."}
	case errors.Is(err, librarychannels.ErrProgramUnavailable):
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusNotFound, code: "library_channel_program_unavailable", message: "The current Library Channel program is unavailable."}
	default:
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusServiceUnavailable, code: "library_channel_playback_unavailable", message: "Portico could not start Library Channel playback."}
	}
}

type libraryChannelTuneDocument struct {
	SourceType string                        `json:"sourceType"`
	ChannelID  string                        `json:"channelId"`
	ProgramID  string                        `json:"programId"`
	StartsAt   string                        `json:"startsAt"`
	EndsAt     string                        `json:"endsAt"`
	Playout    librarychannels.PlayoutSource `json:"playout"`
	// QualityProfile and VideoOverlay are the complete handoff to the playback
	// plan assembler. That downstream layer must enforce the quality policy and
	// convert an enabled overlay into a transcoding filter; tune never claims
	// the overlay was burned into the stream by itself.
	QualityProfile string                              `json:"qualityProfile"`
	VideoOverlay   *libraryChannelVideoOverlayDocument `json:"videoOverlay,omitempty"`
	Playback       PlaybackResponse                    `json:"playback"`
}

type libraryChannelVideoOverlayDocument struct {
	AssetURL          string                        `json:"assetUrl"`
	Corner            librarychannels.LogoCorner    `json:"corner"`
	WidthPercent      float64                       `json:"widthPercent"`
	InsetPercent      float64                       `json:"insetPercent"`
	Treatment         librarychannels.LogoTreatment `json:"treatment"`
	RequiresTranscode bool                          `json:"requiresTranscode"`
}

type libraryChannelTemplateDocument struct {
	Key                   string                         `json:"key"`
	Version               int                            `json:"version"`
	Name                  string                         `json:"name"`
	Description           string                         `json:"description"`
	RuleName              string                         `json:"ruleName"`
	Query                 json.RawMessage                `json:"query"`
	SelectionMode         librarychannels.SelectionMode  `json:"selectionMode"`
	EpisodeMode           librarychannels.EpisodeMode    `json:"episodeMode"`
	MinimumCandidates     int                            `json:"minimumCandidates"`
	MinimumDistinctSeries int                            `json:"minimumDistinctSeries"`
	RequiredEntityKinds   []string                       `json:"requiredEntityKinds"`
	CandidateLimit        int                            `json:"candidateLimit,omitempty"`
	RecencyDays           int                            `json:"recencyDays,omitempty"`
	Sort                  []librarychannels.TemplateSort `json:"sort"`
	LogoRef               string                         `json:"logoRef"`
}

type libraryChannelTemplateApplicabilityDocument struct {
	TemplateKey    string `json:"templateKey"`
	Applicable     bool   `json:"applicable"`
	CandidateCount int    `json:"candidateCount"`
	DistinctSeries int    `json:"distinctSeries"`
}

type libraryChannelBlockPresetDocument struct {
	Key          string `json:"key"`
	Version      int    `json:"version"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Weekdays     uint8  `json:"weekdays"`
	StartMinute  int    `json:"startMinute"`
	EndMinute    int    `json:"endMinute"`
	Anchored     bool   `json:"anchored"`
	AllowOverrun bool   `json:"allowOverrun"`
}

type libraryChannelRestoreDefaultsRequest struct {
	Timezone     string   `json:"timezone"`
	Mode         string   `json:"mode,omitempty"`
	TemplateKeys []string `json:"templateKeys,omitempty"`
}

type libraryChannelRestoreDefaultsDocument struct {
	Items         []libraryChannelAggregateDocument `json:"items"`
	CreatedCount  int                               `json:"createdCount"`
	ExistingCount int                               `json:"existingCount"`
	SkippedCount  int                               `json:"skippedCount"`
}

type libraryChannelReorderItem struct {
	ID               string `json:"id"`
	ExpectedRevision int64  `json:"expectedRevision"`
}

type libraryChannelReorderRequest struct {
	Items []libraryChannelReorderItem `json:"items"`
}

type libraryChannelGenerationDocument struct {
	ChannelID        string   `json:"channelId"`
	GenerationID     string   `json:"generationId"`
	ConfigRevision   int64    `json:"configRevision"`
	HorizonStart     string   `json:"horizonStart"`
	HorizonEnd       string   `json:"horizonEnd"`
	GeneratedThrough string   `json:"generatedThrough"`
	EntryCount       int      `json:"entryCount"`
	Warnings         []string `json:"warnings"`
}

func (s *Server) handleLibraryChannels(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !canViewLiveTV(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "This profile cannot view Library Channels.")
		return
	}
	channels, err := librarychannels.NewStore(s.dbHandle()).ListChannels(r.Context(), false)
	if err != nil {
		writeLibraryChannelError(w, err)
		return
	}
	limit := clampInt(queryInt(r, "limit", 100), 1, 250)
	offset, cursorErr := s.decodeMaterializedPageCursor(r.URL.Query().Get("cursor"), "library-channels", viewerProfileID(user), time.Now().UTC())
	if cursorErr != nil {
		writeCollectionCursorError(w, cursorErr, "Library Channels")
		return
	}
	total := len(channels)
	if offset > total {
		writeCollectionCursorError(w, errInvalidCursor, "Library Channels")
		return
	}
	end := min(total, offset+limit)
	documents := make([]libraryChannelSummaryDocument, 0, end-offset)
	for _, channel := range channels[offset:end] {
		documents = append(documents, libraryChannelSummaryDocumentFrom(channel, user))
	}
	hasMore := end < total
	var nextCursor *string
	if hasMore {
		token, encodeErr := s.encodeMaterializedPageCursor("library-channels", viewerProfileID(user), end, time.Now().UTC())
		if encodeErr != nil {
			writeProductError(w, http.StatusInternalServerError, "library_channel_cursor_failed", "Portico could not continue Library Channels.")
			return
		}
		nextCursor = &token
	}
	writeJSON(w, http.StatusOK, map[string]any{"sourceType": "library-channel", "items": documents, "pageInfo": CursorPageInfo{NextCursor: nextCursor, HasMore: hasMore, Total: &total}})
}

func (s *Server) handleLibraryChannelsGuide(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !canViewLiveTV(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "This profile cannot view Library Channels.")
		return
	}
	now := time.Now().UTC().Truncate(time.Second)
	from, err := parseLibraryChannelTime(r.URL.Query().Get("from"), now)
	if err != nil {
		writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "Guide start must be an RFC 3339 timestamp.")
		return
	}
	to, err := parseLibraryChannelTime(r.URL.Query().Get("to"), from.Add(3*time.Hour))
	if err != nil || !to.After(from) || to.Sub(from) > libraryChannelGuideMaximum {
		writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "Guide range must be positive and no longer than seven days.")
		return
	}
	store := librarychannels.NewStore(s.dbHandle())
	allChannels, err := store.ListChannels(r.Context(), false)
	if err != nil {
		writeLibraryChannelError(w, err)
		return
	}
	limit := clampInt(queryInt(r, "limit", 50), 1, 1000)
	offset, cursorErr := s.decodeMaterializedPageCursor(r.URL.Query().Get("cursor"), "library-channels-guide", viewerProfileID(user), now)
	if cursorErr != nil || offset > len(allChannels) {
		writeCollectionCursorError(w, firstNonNilError(cursorErr, errInvalidCursor), "Library Channels guide")
		return
	}
	end := min(len(allChannels), offset+limit)
	channels := make([]libraryChannelSummaryDocument, 0, end-offset)
	programs := []libraryChannelGuideEntryDocument{}
	for _, channel := range allChannels[offset:end] {
		channels = append(channels, libraryChannelSummaryDocumentFrom(channel, user))
		entries, loadErr := store.LoadActiveSchedule(r.Context(), channel.ID, from, to)
		if loadErr != nil {
			writeLibraryChannelError(w, loadErr)
			return
		}
		decisions, decisionErr := s.libraryChannelGuideAccessDecisions(r.Context(), user, channel.ID, from, to)
		if decisionErr != nil {
			writeLibraryChannelError(w, decisionErr)
			return
		}
		entries = librarychannels.ProjectSchedule(entries, func(mediaID string) librarychannels.AccessDecision {
			if decision, exists := decisions[mediaID]; exists {
				return decision
			}
			return librarychannels.AccessRestricted
		})
		for _, entry := range entries {
			programs = append(programs, libraryChannelGuideEntryDocumentFrom(entry))
		}
	}
	hasMore := end < len(allChannels)
	var nextCursor *string
	if hasMore {
		token, encodeErr := s.encodeMaterializedPageCursor("library-channels-guide", viewerProfileID(user), end, now)
		if encodeErr != nil {
			writeProductError(w, http.StatusInternalServerError, "library_channel_cursor_failed", "Portico could not continue the Library Channels guide.")
			return
		}
		nextCursor = &token
	}
	total := len(allChannels)
	w.Header().Set("Cache-Control", "private, max-age=15")
	writeJSON(w, http.StatusOK, libraryChannelLinearGuideDocument{
		SourceType: "library-channel", Channels: channels, Programs: programs,
		From: from.Format(time.RFC3339), To: to.Format(time.RFC3339), ServerTime: now.Format(time.RFC3339),
		PageInfo: CursorPageInfo{NextCursor: nextCursor, HasMore: hasMore, Total: &total},
	})
}

func (s *Server) handleLibraryChannelRoute(w http.ResponseWriter, r *http.Request, user User) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/library-channels/"), "/"), "/")
	if len(parts) == 2 && parts[0] == "logos" {
		s.handleLibraryChannelLogoRoute(w, r, user)
		return
	}
	if len(parts) == 3 && strings.TrimSpace(parts[0]) != "" && parts[1] == "hls" {
		s.handleLibraryChannelHLS(w, r, user, parts[0], parts[2])
		return
	}
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		writeProductError(w, http.StatusNotFound, "library_channel_not_found", "Library Channel was not found.")
		return
	}
	switch parts[1] {
	case "guide":
		s.handleLibraryChannelGuide(w, r, user, parts[0])
	case "tune":
		s.handleLibraryChannelTune(w, r, user, parts[0])
	default:
		writeProductError(w, http.StatusNotFound, "library_channel_not_found", "Library Channel was not found.")
	}
}

func (s *Server) handleLibraryChannelGuide(w http.ResponseWriter, r *http.Request, user User, channelID string) {
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !canViewLiveTV(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "This profile cannot view Library Channels.")
		return
	}
	now := time.Now().UTC().Truncate(time.Second)
	from, err := parseLibraryChannelTime(r.URL.Query().Get("from"), now)
	if err != nil {
		writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "Guide start must be an RFC 3339 timestamp.")
		return
	}
	to, err := parseLibraryChannelTime(r.URL.Query().Get("to"), from.Add(libraryChannelGuideMaximum))
	if err != nil || !to.After(from) || to.Sub(from) > libraryChannelGuideMaximum {
		writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "Guide range must be positive and no longer than seven days.")
		return
	}
	store := librarychannels.NewStore(s.dbHandle())
	aggregate, err := store.GetAggregate(r.Context(), channelID)
	if err != nil || !aggregate.Channel.Enabled {
		if err == nil {
			err = librarychannels.ErrNotFound
		}
		writeLibraryChannelError(w, err)
		return
	}
	entries, err := store.LoadActiveSchedule(r.Context(), channelID, from, to)
	if err != nil {
		writeLibraryChannelError(w, err)
		return
	}
	decisions, err := s.libraryChannelGuideAccessDecisions(r.Context(), user, channelID, from, to)
	if err != nil {
		writeLibraryChannelError(w, err)
		return
	}
	entries = librarychannels.ProjectSchedule(entries, func(mediaID string) librarychannels.AccessDecision {
		if decision, exists := decisions[mediaID]; exists {
			return decision
		}
		return librarychannels.AccessRestricted
	})
	limit := clampInt(queryInt(r, "limit", 250), 1, 500)
	scope := fmt.Sprintf("library-channel-guide:%s:%s:%s:%s", channelID, aggregate.Channel.ActiveGenerationID, from.Format(time.RFC3339), to.Format(time.RFC3339))
	offset, cursorErr := s.decodeMaterializedPageCursor(r.URL.Query().Get("cursor"), scope, viewerProfileID(user), time.Now().UTC())
	if cursorErr != nil || offset > len(entries) {
		writeCollectionCursorError(w, firstNonNilError(cursorErr, errInvalidCursor), "Library Channel guide")
		return
	}
	total := len(entries)
	end := min(total, offset+limit)
	programs := make([]libraryChannelGuideEntryDocument, 0, end-offset)
	for _, entry := range entries[offset:end] {
		programs = append(programs, libraryChannelGuideEntryDocumentFrom(entry))
	}
	hasMore := end < total
	var nextCursor *string
	if hasMore {
		token, encodeErr := s.encodeMaterializedPageCursor(scope, viewerProfileID(user), end, time.Now().UTC())
		if encodeErr != nil {
			writeProductError(w, http.StatusInternalServerError, "library_channel_cursor_failed", "Portico could not continue the Library Channel guide.")
			return
		}
		nextCursor = &token
	}
	w.Header().Set("Cache-Control", "private, max-age=15")
	writeJSON(w, http.StatusOK, libraryChannelGuideDocument{SourceType: "library-channel", Channel: libraryChannelSummaryDocumentFrom(aggregate.Channel, user), From: from.Format(time.RFC3339), To: to.Format(time.RFC3339), ServerTime: now.Format(time.RFC3339), Programs: programs, PageInfo: CursorPageInfo{NextCursor: nextCursor, HasMore: hasMore, Total: &total}})
}

func (s *Server) handleLibraryChannelTune(w http.ResponseWriter, r *http.Request, user User, channelID string) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if !canPlayLiveTV(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "This profile cannot play Library Channels.")
		return
	}
	var request libraryChannelTuneRequest
	if !decodeJSONLimit(w, r, &request, 8<<10) {
		return
	}
	request.ClientInstanceID = normalizePlaybackClientInstanceID(request.ClientInstanceID)
	if request.ClientInstanceID == "" {
		writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "A client instance identifier is required for playback ownership.")
		return
	}
	if strings.TrimSpace(request.Intent.Quality.Mode) == "" {
		writeProductError(w, http.StatusBadRequest, "invalid_playback_quality", "intent.quality is required and must be Automatic or an exact server-issued explicit offer.")
		return
	}
	request.ClientProfile = normalizePlaybackProfile(request.ClientProfile)
	now := time.Now().UTC().Truncate(time.Second)
	if strings.TrimSpace(request.At) != "" && !isLibraryChannelOwner(user) {
		writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "Consumer tune uses authoritative server time.")
		return
	}
	at, err := parseLibraryChannelTime(request.At, now)
	if err != nil {
		writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "Tune time must be an RFC 3339 timestamp.")
		return
	}
	store := librarychannels.NewStore(s.dbHandle())
	aggregate, err := store.GetAggregate(r.Context(), channelID)
	if err != nil || !aggregate.Channel.Enabled {
		if err == nil {
			err = librarychannels.ErrNotFound
		}
		writeLibraryChannelError(w, err)
		return
	}
	entries, err := store.LoadActiveSchedule(r.Context(), channelID, at, at.Add(time.Second))
	if err != nil {
		writeLibraryChannelError(w, err)
		return
	}
	if len(entries) == 0 {
		writeProductError(w, http.StatusNotFound, "library_channel_program_unavailable", "No Library Channel program is scheduled at that time.")
		return
	}
	entry := entries[0]
	playout, err := librarychannels.ResolvePlayoutSource(entry, at, s.libraryChannelAccessDecision(r.Context(), user))
	if err != nil {
		writeLibraryChannelError(w, err)
		return
	}
	var overlay *libraryChannelVideoOverlayDocument
	if aggregate.Channel.Logo.BugEnabled {
		overlay = &libraryChannelVideoOverlayDocument{AssetURL: libraryChannelLogoURL(aggregate.Channel.Logo), Corner: aggregate.Channel.Logo.BugCorner, WidthPercent: aggregate.Channel.Logo.BugWidthPct, InsetPercent: aggregate.Channel.Logo.BugInsetPct, Treatment: aggregate.Channel.Logo.BugTreatment, RequiresTranscode: true}
	}
	playback, err := s.startLibraryChannelPlayback(r, user, aggregate.Channel, entry, request.ClientProfile, request.Intent, request.ClientInstanceID, request.Replacement, nil)
	if err != nil {
		var startErr *playbackStartHTTPError
		if errors.As(err, &startErr) {
			writePlaybackStartError(w, startErr)
			return
		}
		writeLibraryChannelPlaybackError(w, err)
		return
	}
	setPlaybackMediaGrantCookie(w, r, playback)
	writeJSON(w, http.StatusOK, libraryChannelTuneDocument{SourceType: "library-channel", ChannelID: channelID, ProgramID: entry.ID, StartsAt: entry.StartsAt.Format(time.RFC3339), EndsAt: entry.EndsAt.Format(time.RFC3339), Playout: playout, QualityProfile: aggregate.Channel.QualityProfile, VideoOverlay: overlay, Playback: playback})
}

func (s *Server) handleAdminLibraryChannels(w http.ResponseWriter, r *http.Request, user User) {
	if !isLibraryChannelOwner(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Only the server owner can administer Library Channels.")
		return
	}
	store := librarychannels.NewStore(s.dbHandle())
	switch r.Method {
	case http.MethodGet:
		channels, err := store.ListChannels(r.Context(), true)
		if err != nil {
			writeLibraryChannelError(w, err)
			return
		}
		limit := clampInt(queryInt(r, "limit", 100), 1, 250)
		offset, cursorErr := s.decodeMaterializedPageCursor(r.URL.Query().Get("cursor"), "admin-library-channels", viewerProfileID(user), time.Now().UTC())
		if cursorErr != nil || offset > len(channels) {
			writeCollectionCursorError(w, firstNonNilError(cursorErr, errInvalidCursor), "Library Channel administration")
			return
		}
		total := len(channels)
		end := min(total, offset+limit)
		items := make([]libraryChannelDocument, 0, end-offset)
		for _, channel := range channels[offset:end] {
			items = append(items, libraryChannelDocumentFrom(channel))
		}
		hasMore := end < total
		var nextCursor *string
		if hasMore {
			token, encodeErr := s.encodeMaterializedPageCursor("admin-library-channels", viewerProfileID(user), end, time.Now().UTC())
			if encodeErr != nil {
				writeProductError(w, http.StatusInternalServerError, "library_channel_cursor_failed", "Portico could not continue Library Channel administration.")
				return
			}
			nextCursor = &token
		}
		writeJSON(w, http.StatusOK, map[string]any{"sourceType": "library-channel", "items": items, "pageInfo": CursorPageInfo{NextCursor: nextCursor, HasMore: hasMore, Total: &total}})
	case http.MethodPost:
		var request libraryChannelConfigurationRequest
		if !decodeJSONLimit(w, r, &request, libraryChannelMaxRequestBytes) {
			return
		}
		if request.ExpectedRevision != 0 {
			writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "A new Library Channel must use expectedRevision 0.")
			return
		}
		s.libraryChannelAssetMu.Lock()
		defer s.libraryChannelAssetMu.Unlock()
		if err := s.validateLibraryChannelLogoReference(request.Logo); err != nil {
			writeProductError(w, http.StatusBadRequest, "library_channel_logo_invalid", err.Error())
			return
		}
		aggregate := request.libraryChannelAggregate(randomID("lch"), 0)
		saved, err := store.SaveAggregate(r.Context(), aggregate, 0)
		if err != nil {
			writeLibraryChannelError(w, err)
			return
		}
		saved, err = store.GetAggregate(r.Context(), saved.Channel.ID)
		if err != nil {
			writeLibraryChannelError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, libraryChannelAggregateDocumentFrom(saved))
	default:
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
	}
}

func (s *Server) handleAdminLibraryChannelRoute(w http.ResponseWriter, r *http.Request, user User) {
	if !isLibraryChannelOwner(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Only the server owner can administer Library Channels.")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/library-channels/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		switch parts[0] {
		case "reorder":
			s.handleAdminLibraryChannelReorder(w, r, user)
			return
		case "templates":
			s.handleAdminLibraryChannelTemplates(w, r, user)
			return
		case "restore-defaults":
			s.handleAdminLibraryChannelRestoreDefaults(w, r, user)
			return
		case "health":
			s.handleAdminLibraryChannelHealth(w, r, user)
			return
		case "logos":
			s.handleAdminLibraryChannelLogoUpload(w, r, user)
			return
		}
	}
	if len(parts) == 2 && parts[0] == "logos" {
		s.handleAdminLibraryChannelLogoDelete(w, r, user, parts[1])
		return
	}
	if len(parts) == 2 && parts[0] == "templates" && parts[1] == "applicability" {
		s.handleAdminLibraryChannelApplicability(w, r, user)
		return
	}
	if len(parts) == 1 {
		s.handleAdminLibraryChannelResource(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "regenerate" {
		s.handleAdminLibraryChannelRegenerate(w, r, parts[0])
		return
	}
	writeProductError(w, http.StatusNotFound, "library_channel_not_found", "Library Channel was not found.")
}

func (s *Server) handleAdminLibraryChannelResource(w http.ResponseWriter, r *http.Request, channelID string) {
	store := librarychannels.NewStore(s.dbHandle())
	switch r.Method {
	case http.MethodGet:
		aggregate, err := store.GetAggregate(r.Context(), channelID)
		if err != nil {
			writeLibraryChannelError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, libraryChannelAggregateDocumentFrom(aggregate))
	case http.MethodPut:
		var request libraryChannelConfigurationRequest
		if !decodeJSONLimit(w, r, &request, libraryChannelMaxRequestBytes) {
			return
		}
		if request.ExpectedRevision < 1 {
			writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "expectedRevision must identify the configuration being replaced.")
			return
		}
		existing, err := store.GetAggregate(r.Context(), channelID)
		if err != nil {
			writeLibraryChannelError(w, err)
			return
		}
		s.libraryChannelAssetMu.Lock()
		defer s.libraryChannelAssetMu.Unlock()
		if err := s.validateLibraryChannelLogoReference(request.Logo); err != nil {
			writeProductError(w, http.StatusBadRequest, "library_channel_logo_invalid", err.Error())
			return
		}
		aggregate := request.libraryChannelAggregate(channelID, request.ExpectedRevision)
		// Ordinary edits preserve the server-owned deterministic seed. A new
		// seed is minted only through the explicit reshuffle intent, so changing
		// a name, logo, or schedule block cannot silently reorder programming.
		aggregate.Channel.Seed = libraryChannelReplacementSeed(existing.Channel.Seed, request.Reshuffle)
		saved, err := store.SaveAggregate(r.Context(), aggregate, request.ExpectedRevision)
		if err != nil {
			writeLibraryChannelError(w, err)
			return
		}
		saved, err = store.GetAggregate(r.Context(), channelID)
		if err != nil {
			writeLibraryChannelError(w, err)
			return
		}
		s.removeLibraryChannelSegmentCache(channelID, existing.Channel.ConfigRevision)
		writeJSON(w, http.StatusOK, libraryChannelAggregateDocumentFrom(saved))
	case http.MethodDelete:
		revision, err := strconv.ParseInt(r.URL.Query().Get("expectedRevision"), 10, 64)
		if err != nil || revision < 1 {
			writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "expectedRevision is required.")
			return
		}
		if err := store.DeleteChannel(r.Context(), channelID, revision); err != nil {
			writeLibraryChannelError(w, err)
			return
		}
		s.removeLibraryChannelSegmentCache(channelID, 0)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET, PUT, or DELETE for this endpoint.")
	}
}

func (s *Server) handleAdminLibraryChannelReorder(w http.ResponseWriter, r *http.Request, user User) {
	if !isLibraryChannelOwner(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Server administration permission is required.")
		return
	}
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var request libraryChannelReorderRequest
	if !decodeJSONLimit(w, r, &request, 64<<10) {
		return
	}
	order := make([]librarychannels.ChannelOrder, len(request.Items))
	for index, item := range request.Items {
		order[index] = librarychannels.ChannelOrder{ID: item.ID, ExpectedRevision: item.ExpectedRevision, SortOrder: index}
	}
	channels, err := librarychannels.NewStore(s.dbHandle()).ReorderChannels(r.Context(), order)
	if err != nil {
		writeLibraryChannelError(w, err)
		return
	}
	for _, item := range request.Items {
		s.removeLibraryChannelSegmentCache(item.ID, item.ExpectedRevision)
	}
	items := make([]libraryChannelDocument, len(channels))
	for index, channel := range channels {
		items[index] = libraryChannelDocumentFrom(channel)
	}
	total := len(items)
	writeJSON(w, http.StatusOK, map[string]any{"sourceType": "library-channel", "items": items, "pageInfo": CursorPageInfo{HasMore: false, Total: &total}})
}

func (s *Server) handleAdminLibraryChannelTemplates(w http.ResponseWriter, r *http.Request, user User) {
	if !isLibraryChannelOwner(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Server administration permission is required.")
		return
	}
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	templates := librarychannels.BuiltInChannelTemplates()
	documents := make([]libraryChannelTemplateDocument, len(templates))
	for index, template := range templates {
		documents[index] = libraryChannelTemplateDocumentFrom(template)
	}
	presets := librarychannels.BuiltInBlockPresets()
	presetDocuments := make([]libraryChannelBlockPresetDocument, len(presets))
	for index, preset := range presets {
		presetDocuments[index] = libraryChannelBlockPresetDocument{Key: preset.Key, Version: preset.Version, Name: preset.Name, Description: preset.Description, Weekdays: uint8(preset.Weekdays), StartMinute: preset.StartMinute, EndMinute: preset.EndMinute, Anchored: preset.Anchored, AllowOverrun: preset.AllowOverrun}
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": documents, "blockPresets": presetDocuments})
}

func (s *Server) handleAdminLibraryChannelApplicability(w http.ResponseWriter, r *http.Request, user User) {
	if !isLibraryChannelOwner(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Server administration permission is required.")
		return
	}
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), libraryChannelGenerationBudget)
	defer cancel()
	result, err := s.libraryChannelTemplateApplicability(ctx)
	if err != nil {
		writeLibraryChannelGenerationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result, "total": len(result)})
}

func (s *Server) handleAdminLibraryChannelRestoreDefaults(w http.ResponseWriter, r *http.Request, user User) {
	if !isLibraryChannelOwner(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Server administration permission is required.")
		return
	}
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var request libraryChannelRestoreDefaultsRequest
	if !decodeJSONLimit(w, r, &request, 32<<10) {
		return
	}
	if _, err := time.LoadLocation(strings.TrimSpace(request.Timezone)); err != nil {
		writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "timezone must be a recognized IANA time zone.")
		return
	}
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	if request.Mode == "" {
		request.Mode = "recommended"
	}
	if request.Mode != "recommended" && request.Mode != "all" {
		writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", "mode must be recommended or all.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), libraryChannelGenerationBudget)
	defer cancel()
	result, err := s.restoreApplicableLibraryChannelDefaults(ctx, request)
	if err != nil {
		writeLibraryChannelGenerationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAdminLibraryChannelHealth(w http.ResponseWriter, r *http.Request, user User) {
	if !isLibraryChannelOwner(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Server administration permission is required.")
		return
	}
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	channels, err := librarychannels.NewStore(s.dbHandle()).ListChannels(r.Context(), true)
	if err != nil {
		writeLibraryChannelError(w, err)
		return
	}
	items := make([]map[string]any, len(channels))
	for index, channel := range channels {
		items[index] = map[string]any{"channelId": channel.ID, "enabled": channel.Enabled, "configRevision": channel.ConfigRevision, "healthState": channel.HealthState, "healthMessageId": channel.HealthMessage, "generatedThrough": formatOptionalTime(channel.GeneratedThrough)}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items), "segmentCache": s.libraryChannelSegmentCacheStatus()})
}

func (s *Server) handleAdminLibraryChannelRegenerate(w http.ResponseWriter, r *http.Request, channelID string) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), libraryChannelGenerationBudget)
	defer cancel()
	result, err := s.generateLibraryChannelSchedule(ctx, channelID)
	if err != nil {
		writeLibraryChannelGenerationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) generateLibraryChannelSchedule(ctx context.Context, channelID string) (libraryChannelGenerationDocument, error) {
	store := librarychannels.NewStore(s.dbHandle())
	aggregate, err := store.GetAggregate(ctx, channelID)
	if err != nil {
		return libraryChannelGenerationDocument{}, err
	}
	if !aggregate.Channel.Enabled {
		return libraryChannelGenerationDocument{}, librarychannels.ErrNotFound
	}
	candidates, err := librarychannels.ResolveCandidateSnapshot(ctx, aggregate, libraryChannelCandidateResolver{server: s})
	if err != nil {
		return libraryChannelGenerationDocument{}, err
	}
	now := time.Now().UTC().Truncate(time.Second)
	request := librarychannels.GenerateRequest{GenerationID: randomID("lcg"), Channel: aggregate.Channel, Rules: aggregate.Rules, Blocks: aggregate.Blocks, Candidates: candidates, InitialCursors: map[string]librarychannels.RuleCursor{}, Start: now, Days: 7, Now: now}
	if active, activeErr := store.GetActiveGeneration(ctx, channelID); activeErr == nil {
		entries, loadErr := store.LoadActiveSchedule(ctx, channelID, active.HorizonStart, active.HorizonEnd)
		if loadErr != nil {
			return libraryChannelGenerationDocument{}, loadErr
		}
		currentIndex := -1
		checkpointIndex := -1
		for index, entry := range entries {
			if !now.Before(entry.StartsAt) && now.Before(entry.EndsAt) {
				currentIndex = index
			}
			if currentIndex >= 0 && index >= currentIndex && len(entry.CursorAfter) > 0 {
				checkpointIndex = index
				break
			}
		}
		if currentIndex >= 0 && checkpointIndex >= currentIndex {
			request.Start = entries[currentIndex].StartsAt
			request.TailStart = entries[checkpointIndex].EndsAt
			request.PreservedEntries = append([]librarychannels.ScheduleEntry(nil), entries[currentIndex:checkpointIndex+1]...)
		}
	}
	generated, err := librarychannels.Generate(ctx, request)
	if err != nil {
		return libraryChannelGenerationDocument{}, err
	}
	lease, err := store.AcquireGeneration(ctx, generated.Generation, libraryChannelGenerationLease)
	if err != nil {
		return libraryChannelGenerationDocument{}, err
	}
	stopRenewal := renewLibraryChannelGenerationLease(ctx, store, generated.Generation.ID, lease.Token)
	defer stopRenewal()
	if err := store.CommitGeneration(ctx, generated.Generation, generated.Entries, lease.Token); err != nil {
		return libraryChannelGenerationDocument{}, err
	}
	return libraryChannelGenerationDocument{ChannelID: channelID, GenerationID: generated.Generation.ID, ConfigRevision: generated.Generation.ConfigRevision, HorizonStart: generated.Generation.HorizonStart.Format(time.RFC3339), HorizonEnd: generated.Generation.HorizonEnd.Format(time.RFC3339), GeneratedThrough: generated.Generation.HorizonEnd.Format(time.RFC3339), EntryCount: len(generated.Entries), Warnings: generated.Warnings}, nil
}

type libraryChannelCandidateResolver struct{ server *Server }

type libraryChannelCandidateConfig struct {
	CandidateLimit int                            `json:"candidateLimit"`
	RecencyDays    int                            `json:"recencyDays"`
	Sort           []librarychannels.TemplateSort `json:"sort"`
}

func (resolver libraryChannelCandidateResolver) ResolveScheduleCandidates(ctx context.Context, rule librarychannels.Rule) ([]librarychannels.Candidate, error) {
	if resolver.server == nil {
		return nil, errors.New("library channel candidate resolver is not configured")
	}
	where := "1=1"
	args := []any{}
	config := libraryChannelCandidateConfig{CandidateLimit: librarychannels.MaximumCandidatesPerRule}
	if trimmedConfig := strings.TrimSpace(string(rule.Config)); trimmedConfig != "" && trimmedConfig != "{}" {
		if err := json.Unmarshal(rule.Config, &config); err != nil {
			return nil, fmt.Errorf("decode Library Channel rule configuration: %w", err)
		}
	}
	if config.CandidateLimit == 0 {
		config.CandidateLimit = librarychannels.MaximumCandidatesPerRule
	}
	if config.CandidateLimit < 1 || config.CandidateLimit > librarychannels.MaximumCandidatesPerRule {
		return nil, &librarychannels.ValidationError{Field: "rule.config.candidateLimit", Message: fmt.Sprintf("must be between 1 and %d", librarychannels.MaximumCandidatesPerRule)}
	}
	if config.RecencyDays < 0 || config.RecencyDays > 36500 {
		return nil, &librarychannels.ValidationError{Field: "rule.config.recencyDays", Message: "must be between 0 and 36500"}
	}
	sortSpecs := make([]browseSortSpec, 0, len(config.Sort))
	if len(config.Sort) == 0 {
		sortSpecs = append(sortSpecs, browseSortSpec{Field: "sortTitle", Direction: "ASC", SQL: "m.sort_title COLLATE NOCASE"})
	} else {
		if len(config.Sort) > 3 {
			return nil, &librarychannels.ValidationError{Field: "rule.config.sort", Message: "must contain no more than three fields"}
		}
		seenSort := map[string]bool{}
		for index, item := range config.Sort {
			field := strings.TrimSpace(item.Field)
			direction := strings.ToLower(strings.TrimSpace(item.Direction))
			if field == "lastPlayedAt" {
				return nil, &librarychannels.ValidationError{Field: fmt.Sprintf("rule.config.sort[%d].field", index), Message: "must be server-global; profile playback state cannot define a shared channel schedule"}
			}
			if seenSort[field] {
				return nil, &librarychannels.ValidationError{Field: fmt.Sprintf("rule.config.sort[%d].field", index), Message: "must not repeat a field"}
			}
			spec, ok := browseSortSpecFor(field, direction)
			if !ok || direction != "asc" && direction != "desc" {
				return nil, &librarychannels.ValidationError{Field: fmt.Sprintf("rule.config.sort[%d]", index), Message: "must use a supported canonical browse field and asc or desc direction"}
			}
			seenSort[field] = true
			sortSpecs = append(sortSpecs, spec)
		}
	}
	trimmed := strings.TrimSpace(string(rule.Query))
	if trimmed != "" && trimmed != "{}" {
		var expression BrowseExpression
		if err := json.Unmarshal(rule.Query, &expression); err != nil {
			return nil, err
		}
		clauses := 0
		compiled, compiledArgs, issues := compileBrowseExpression(expression, "query", 1, &clauses)
		if len(issues) > 0 {
			return nil, &browseValidationProblems{Issues: issues}
		}
		if strings.Contains(compiled, "ums.") {
			return nil, &librarychannels.ValidationError{Field: "rule.query", Message: "must be server-global; profile playback state cannot define a shared channel schedule"}
		}
		where, args = "("+compiled+")", compiledArgs
	}
	if config.RecencyDays > 0 {
		where += " AND m.added_at >= datetime('now', ?)"
		args = append(args, fmt.Sprintf("-%d days", config.RecencyDays))
	}
	orderSQL := browseOrderSQL(sortSpecs)
	rows, err := resolver.server.queryUserRead(ctx, `
		SELECT m.id,m.type,m.title,COALESCE(m.summary,''),COALESCE(m.content_rating,''),COALESCE(m.duration_seconds,0),
			COALESCE(m.season_number,0),COALESCE(m.episode_number,0),COALESCE(parent.id,''),COALESCE(grandparent.id,''),
			COALESCE(availability.file_count,0),COALESCE(availability.missing_file_count,0)
		FROM media_items m
		LEFT JOIN media_items parent ON parent.id=m.parent_id
		LEFT JOIN media_items grandparent ON grandparent.id=parent.parent_id
		LEFT JOIN media_availability availability ON availability.media_id=m.id
		LEFT JOIN user_media_state ums ON 1=0
		WHERE `+where+`
		ORDER BY `+orderSQL+`
		LIMIT ?`, append(args, config.CandidateLimit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := []librarychannels.Candidate{}
	showIDs := []string{}
	seen := map[string]bool{}
	order := int64(0)
	for rows.Next() {
		var id, mediaType, title, summary, rating, parentID, grandparentID string
		var duration, season, episode, files, missing int
		if err := rows.Scan(&id, &mediaType, &title, &summary, &rating, &duration, &season, &episode, &parentID, &grandparentID, &files, &missing); err != nil {
			return nil, err
		}
		switch mediaType {
		case "show", "anime":
			showIDs = append(showIDs, id)
		case "movie", "episode", "recording", "special":
			if duration > 0 && files > missing && !seen[id] && len(candidates) < config.CandidateLimit {
				seriesID := grandparentID
				if seriesID == "" && mediaType == "episode" {
					seriesID = parentID
				}
				candidates = append(candidates, librarychannels.Candidate{MediaID: id, Title: title, Summary: summary, ContentRating: rating, Artwork: json.RawMessage(`{}`), Duration: time.Duration(duration) * time.Second, Weight: 1, Order: order, SeriesID: seriesID, SeasonNumber: season, EpisodeNumber: episode, Availability: "available"})
				seen[id], order = true, order+1
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(showIDs) > 0 && len(candidates) < config.CandidateLimit {
		if len(showIDs) > librarychannels.MaximumCandidatesPerRule {
			return nil, fmt.Errorf("candidate query selected too many series")
		}
		placeholders := strings.TrimRight(strings.Repeat("?,", len(showIDs)), ",")
		episodeRows, err := resolver.server.queryUserRead(ctx, `
			SELECT episode.id,episode.title,COALESCE(episode.summary,''),COALESCE(episode.content_rating,''),COALESCE(episode.duration_seconds,0),
				COALESCE(episode.season_number,0),COALESCE(episode.episode_number,0),COALESCE(show_item.id,''),
				COALESCE(availability.file_count,0),COALESCE(availability.missing_file_count,0)
			FROM media_items episode
			LEFT JOIN media_items season ON season.id=episode.parent_id
			LEFT JOIN media_items show_item ON show_item.id=season.parent_id OR (show_item.id=episode.parent_id AND show_item.type IN ('show','anime'))
			LEFT JOIN media_availability availability ON availability.media_id=episode.id
			WHERE episode.type='episode' AND show_item.id IN (`+placeholders+`)
			ORDER BY show_item.sort_title COLLATE NOCASE,episode.season_number,episode.episode_number,episode.id
			LIMIT ?`, append(libraryChannelAnyStrings(showIDs), config.CandidateLimit-len(candidates)+1)...)
		if err != nil {
			return nil, err
		}
		defer episodeRows.Close()
		for episodeRows.Next() {
			var id, title, summary, rating, seriesID string
			var duration, season, episode, files, missing int
			if err := episodeRows.Scan(&id, &title, &summary, &rating, &duration, &season, &episode, &seriesID, &files, &missing); err != nil {
				return nil, err
			}
			if duration > 0 && files > missing && !seen[id] && len(candidates) < config.CandidateLimit {
				candidates = append(candidates, librarychannels.Candidate{MediaID: id, Title: title, Summary: summary, ContentRating: rating, Artwork: json.RawMessage(`{}`), Duration: time.Duration(duration) * time.Second, Weight: 1, Order: order, SeriesID: seriesID, SeasonNumber: season, EpisodeNumber: episode, Availability: "available"})
				seen[id], order = true, order+1
			}
		}
		if err := episodeRows.Err(); err != nil {
			return nil, err
		}
	}
	if len(candidates) > librarychannels.MaximumCandidatesPerRule {
		return nil, fmt.Errorf("candidate query exceeds the Library Channel limit")
	}
	return candidates, nil
}

func (s *Server) libraryChannelAccessDecision(ctx context.Context, user User) func(string) librarychannels.AccessDecision {
	return func(mediaID string) librarychannels.AccessDecision {
		var files, missing int
		if err := s.queryUserRow(ctx, `SELECT COALESCE(file_count,0),COALESCE(missing_file_count,0) FROM media_availability WHERE media_id=?`, mediaID).Scan(&files, &missing); err != nil || files == 0 || files <= missing {
			return librarychannels.AccessUnavailable
		}
		if _, err := s.getMediaAccessSummaryContext(ctx, viewerProfileID(user), mediaID); err != nil {
			return librarychannels.AccessRestricted
		}
		return librarychannels.AccessAllowed
	}
}

// libraryChannelGuideAccessDecisions projects an entire guide window with one
// set-based visibility query. It deliberately does not accept a list of media
// IDs, so a maximum-size guide never runs into SQLite placeholder limits and
// query count remains independent of the number of programs.
func (s *Server) libraryChannelGuideAccessDecisions(ctx context.Context, user User, channelID string, from, to time.Time) (map[string]librarychannels.AccessDecision, error) {
	where := `c.id=? AND c.enabled=1 AND c.active_generation_id=e.generation_id AND e.ends_at>? AND e.starts_at<?`
	args := []any{channelID, from.UTC().Unix(), to.UTC().Unix()}
	where, args = s.applyMediaVisibilityRestrictionSQL(viewerProfileID(user), where, args)
	where, args = s.applyLibraryCurationRestrictionSQL(ctx, viewerProfileID(user), where, args)
	rows, err := s.queryUserRead(ctx, `
		SELECT DISTINCT e.media_id,
			CASE WHEN COALESCE(a.file_count,0)>COALESCE(a.missing_file_count,0) THEN 1 ELSE 0 END
		FROM library_channel_schedule_entries e
		JOIN library_channels c ON c.id=e.channel_id
		JOIN media_items m ON m.id=e.media_id
		LEFT JOIN media_availability a ON a.media_id=m.id
		WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	decisions := make(map[string]librarychannels.AccessDecision)
	for rows.Next() {
		var mediaID string
		var available int
		if err := rows.Scan(&mediaID, &available); err != nil {
			return nil, err
		}
		if available == 1 {
			decisions[mediaID] = librarychannels.AccessAllowed
		} else {
			decisions[mediaID] = librarychannels.AccessUnavailable
		}
	}
	return decisions, rows.Err()
}

func (s *Server) libraryChannelTemplateApplicability(ctx context.Context) ([]libraryChannelTemplateApplicabilityDocument, error) {
	resolver := libraryChannelCandidateResolver{server: s}
	result := []libraryChannelTemplateApplicabilityDocument{}
	for _, template := range librarychannels.BuiltInChannelTemplates() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rule := ruleFromTemplate("template", template)
		candidates, err := resolver.ResolveScheduleCandidates(ctx, rule)
		if err != nil {
			return nil, err
		}
		series := map[string]bool{}
		kinds := map[string]int{}
		for _, candidate := range candidates {
			if candidate.SeriesID != "" {
				series[candidate.SeriesID] = true
				kinds["show"]++
			} else {
				kinds["movie"]++
			}
		}
		inventory := librarychannels.TemplateInventory{CandidateCount: len(candidates), DistinctSeries: len(series), EntityKinds: kinds}
		result = append(result, libraryChannelTemplateApplicabilityDocument{TemplateKey: template.Key, Applicable: librarychannels.EvaluateTemplateApplicability(template, inventory), CandidateCount: len(candidates), DistinctSeries: len(series)})
	}
	return result, nil
}

func (s *Server) restoreApplicableLibraryChannelDefaults(ctx context.Context, request libraryChannelRestoreDefaultsRequest) (libraryChannelRestoreDefaultsDocument, error) {
	applicability, err := s.libraryChannelTemplateApplicability(ctx)
	if err != nil {
		return libraryChannelRestoreDefaultsDocument{}, err
	}
	allowed := map[string]bool{}
	applicable := map[string]bool{}
	selected := map[string]bool{}
	for _, item := range applicability {
		applicable[item.TemplateKey] = item.Applicable
	}
	if len(request.TemplateKeys) == 0 {
		for _, item := range applicability {
			selected[item.TemplateKey] = true
			allowed[item.TemplateKey] = request.Mode == "all" || item.Applicable
		}
	} else {
		for _, key := range request.TemplateKeys {
			if key = strings.TrimSpace(key); key != "" {
				selected[key] = true
			}
		}
		for _, item := range applicability {
			allowed[item.TemplateKey] = selected[item.TemplateKey] && (request.Mode == "all" || item.Applicable)
		}
	}
	store := librarychannels.NewStore(s.dbHandle())
	existing, err := store.ListChannels(ctx, true)
	if err != nil {
		return libraryChannelRestoreDefaultsDocument{}, err
	}
	result := libraryChannelRestoreDefaultsDocument{Items: []libraryChannelAggregateDocument{}}
	for key := range selected {
		if !allowed[key] {
			result.SkippedCount++
		}
	}
	seen := map[string]bool{}
	for _, channel := range existing {
		if channel.TemplateKey == "" || !allowed[channel.TemplateKey] {
			continue
		}
		result.ExistingCount++
		seen[channel.TemplateKey] = true
	}
	for _, template := range librarychannels.BuiltInChannelTemplates() {
		if !allowed[template.Key] || seen[template.Key] {
			continue
		}
		channelID := randomID("lch")
		ruleID := randomID("lcr")
		logo := defaultLibraryChannelLogo()
		logo.Source = librarychannels.LogoBuiltIn
		logo.Ref = builtInLibraryChannelLogoRef(template.Key)
		logo.MIMEType = "image/svg+xml"
		enabled := applicable[template.Key]
		healthState := "pending"
		healthMessage := ""
		if !enabled {
			healthState = "disabled"
			healthMessage = "library-channel.default-disabled-no-playable-media"
		}
		channel := librarychannels.Channel{ID: channelID, Name: template.Name, Description: template.Description, Enabled: enabled, SortOrder: len(existing) + result.CreatedCount, Timezone: request.Timezone, Seed: randomID("seed"), DefaultRuleID: ruleID, QualityProfile: "original", Logo: logo, TemplateKey: template.Key, TemplateVersion: template.Version, ConfigRevision: 1, HealthState: healthState, HealthMessage: healthMessage}
		aggregate := librarychannels.Aggregate{Channel: channel, Rules: []librarychannels.Rule{ruleFromTemplate(channelID, template)}}
		aggregate.Rules[0].ID = ruleID
		saved, err := store.SaveAggregate(ctx, aggregate, 0)
		if err != nil {
			if errors.Is(err, librarychannels.ErrTemplateExists) {
				// Another restore request won the uniqueness race. The desired
				// default already exists, so this request remains idempotent.
				if _, loadErr := store.GetAggregateByTemplateKey(ctx, template.Key); loadErr != nil {
					return libraryChannelRestoreDefaultsDocument{}, loadErr
				}
				result.ExistingCount++
				seen[template.Key] = true
				continue
			}
			return libraryChannelRestoreDefaultsDocument{}, err
		}
		result.Items = append(result.Items, libraryChannelAggregateDocumentFrom(saved))
		result.CreatedCount++
		seen[template.Key] = true
	}
	return result, nil
}

func ruleFromTemplate(channelID string, template librarychannels.ChannelTemplate) librarychannels.Rule {
	config, _ := json.Marshal(map[string]any{"candidateLimit": template.CandidateLimit, "recencyDays": template.RecencyDays, "sort": template.Sort})
	return librarychannels.Rule{ID: randomID("lcr"), ChannelID: channelID, Name: template.RuleName, Enabled: true, Query: append(json.RawMessage(nil), template.Query...), SelectionMode: template.SelectionMode, EpisodeMode: template.EpisodeMode, ExhaustionMode: librarychannels.ExhaustionLoop, DedupeWindow: 12, MaxConsecutive: 3, Config: config, TemplateKey: template.Key, TemplateVersion: template.Version}
}

func (request libraryChannelConfigurationRequest) libraryChannelAggregate(channelID string, expectedRevision int64) librarychannels.Aggregate {
	seed := randomID("seed")
	channel := librarychannels.Channel{ID: channelID, Name: request.Name, Description: request.Description, Enabled: request.Enabled, SortOrder: request.SortOrder, Timezone: request.Timezone, Seed: seed, DefaultRuleID: request.DefaultRuleID, QualityProfile: request.QualityProfile, Logo: request.Logo.domain(), TemplateKey: request.TemplateKey, TemplateVersion: request.TemplateVersion, ConfigRevision: expectedRevision + 1}
	if !channel.Enabled {
		channel.HealthState = "disabled"
	} else {
		channel.HealthState = "pending"
	}
	rules := make([]librarychannels.Rule, len(request.Rules))
	for index, document := range request.Rules {
		rules[index] = librarychannels.Rule{ID: document.ID, ChannelID: channelID, Name: document.Name, Enabled: document.Enabled, SortOrder: document.SortOrder, Query: document.Query, SelectionMode: document.SelectionMode, EpisodeMode: document.EpisodeMode, ExhaustionMode: document.ExhaustionMode, DedupeWindow: document.DedupeWindow, MaxConsecutive: document.MaxConsecutive, Config: document.Config, TemplateKey: document.TemplateKey, TemplateVersion: document.TemplateVersion}
	}
	blocks := make([]librarychannels.WeeklyBlock, len(request.Blocks))
	for index, document := range request.Blocks {
		blocks[index] = librarychannels.WeeklyBlock{ID: document.ID, ChannelID: channelID, RuleID: document.RuleID, FallbackRuleID: document.FallbackRuleID, Name: document.Name, Enabled: document.Enabled, Weekdays: librarychannels.WeekdayMask(document.WeekdayMask), StartMinute: document.StartMinute, EndMinute: document.EndMinute, Priority: document.Priority, Anchored: document.Anchored, AllowOverrun: document.AllowOverrun, SortOrder: document.SortOrder, TemplateKey: document.TemplateKey, TemplateVersion: document.TemplateVersion}
	}
	return librarychannels.Aggregate{Channel: channel, Rules: rules, Blocks: blocks}
}

func libraryChannelReplacementSeed(current string, reshuffle bool) string {
	if !reshuffle && strings.TrimSpace(current) != "" {
		return current
	}
	return randomID("seed")
}

func (document libraryChannelLogoDocument) domain() librarychannels.LogoConfig {
	return librarychannels.LogoConfig{Source: document.Source, Ref: document.Ref, MIMEType: document.MIMEType, BugEnabled: document.BugEnabled, BugOverheadAccepted: document.BugOverheadAccepted, BugCorner: document.BugCorner, BugWidthPct: document.BugWidthPercent, BugInsetPct: document.BugInsetPercent, BugTreatment: document.BugTreatment}
}

func defaultLibraryChannelLogo() librarychannels.LogoConfig {
	return librarychannels.LogoConfig{Source: librarychannels.LogoNone, BugCorner: librarychannels.LogoTopRight, BugWidthPct: 8, BugInsetPct: 2, BugTreatment: librarychannels.LogoColor}
}

func libraryChannelAggregateDocumentFrom(aggregate librarychannels.Aggregate) libraryChannelAggregateDocument {
	rules := make([]libraryChannelRuleDocument, len(aggregate.Rules))
	for index, rule := range aggregate.Rules {
		rules[index] = libraryChannelRuleDocument{ID: rule.ID, Name: rule.Name, Enabled: rule.Enabled, SortOrder: rule.SortOrder, Query: rule.Query, SelectionMode: rule.SelectionMode, EpisodeMode: rule.EpisodeMode, ExhaustionMode: rule.ExhaustionMode, DedupeWindow: rule.DedupeWindow, MaxConsecutive: rule.MaxConsecutive, Config: rule.Config, TemplateKey: rule.TemplateKey, TemplateVersion: rule.TemplateVersion}
	}
	blocks := make([]libraryChannelBlockDocument, len(aggregate.Blocks))
	for index, block := range aggregate.Blocks {
		blocks[index] = libraryChannelBlockDocument{ID: block.ID, RuleID: block.RuleID, FallbackRuleID: block.FallbackRuleID, Name: block.Name, Enabled: block.Enabled, WeekdayMask: uint8(block.Weekdays), StartMinute: block.StartMinute, EndMinute: block.EndMinute, Priority: block.Priority, Anchored: block.Anchored, AllowOverrun: block.AllowOverrun, SortOrder: block.SortOrder, TemplateKey: block.TemplateKey, TemplateVersion: block.TemplateVersion}
	}
	return libraryChannelAggregateDocument{Channel: libraryChannelDocumentFrom(aggregate.Channel), Rules: rules, Blocks: blocks}
}

func libraryChannelDocumentFrom(channel librarychannels.Channel) libraryChannelDocument {
	return libraryChannelDocument{ID: channel.ID, SourceType: "library-channel", Name: channel.Name, Description: channel.Description, Enabled: channel.Enabled, SortOrder: channel.SortOrder, Timezone: channel.Timezone, DefaultRuleID: channel.DefaultRuleID, QualityProfile: channel.QualityProfile, Logo: libraryChannelLogoDocument{Source: channel.Logo.Source, Ref: channel.Logo.Ref, URL: libraryChannelLogoURL(channel.Logo), MIMEType: channel.Logo.MIMEType, BugEnabled: channel.Logo.BugEnabled, BugOverheadAccepted: channel.Logo.BugOverheadAccepted, BugCorner: channel.Logo.BugCorner, BugWidthPercent: channel.Logo.BugWidthPct, BugInsetPercent: channel.Logo.BugInsetPct, BugTreatment: channel.Logo.BugTreatment}, TemplateKey: channel.TemplateKey, TemplateVersion: channel.TemplateVersion, ConfigRevision: channel.ConfigRevision, GeneratedThrough: formatOptionalTime(channel.GeneratedThrough), HealthState: channel.HealthState, HealthMessageID: channel.HealthMessage, CreatedAt: formatOptionalTime(channel.CreatedAt), UpdatedAt: formatOptionalTime(channel.UpdatedAt)}
}

func libraryChannelSummaryDocumentFrom(channel librarychannels.Channel, user User) libraryChannelSummaryDocument {
	actions := []string{}
	if canPlayLiveTV(user) && hasPermission(user, "playMedia") {
		actions = append(actions, "play")
	}
	return libraryChannelSummaryDocument{ID: channel.ID, SourceType: "library-channel", Name: channel.Name, Description: channel.Description, LogoURL: libraryChannelLogoURL(channel.Logo), Actions: actions}
}

func libraryChannelGuideEntryDocumentFrom(entry librarychannels.ScheduleEntry) libraryChannelGuideEntryDocument {
	artwork := map[string]string{}
	if entry.MediaID != "" {
		mediaID := url.PathEscape(entry.MediaID)
		artwork = map[string]string{"poster": "/api/artwork/" + mediaID + "/poster.svg", "backdrop": "/api/artwork/" + mediaID + "/backdrop.svg", "thumb": "/api/artwork/" + mediaID + "/thumb.svg"}
	}
	return libraryChannelGuideEntryDocument{ID: entry.ID, ChannelID: entry.ChannelID, SourceType: "library-channel", Kind: entry.Kind, StartsAt: entry.StartsAt.Format(time.RFC3339), EndsAt: entry.EndsAt.Format(time.RFC3339), MediaID: entry.MediaID, Title: entry.Title, Subtitle: entry.Subtitle, Summary: entry.Summary, ContentRating: entry.ContentRating, Artwork: artwork, Availability: entry.Availability, ReasonMessageID: entry.ReasonCode}
}

func libraryChannelTemplateDocumentFrom(template librarychannels.ChannelTemplate) libraryChannelTemplateDocument {
	return libraryChannelTemplateDocument{Key: template.Key, Version: template.Version, Name: template.Name, Description: template.Description, RuleName: template.RuleName, Query: template.Query, SelectionMode: template.SelectionMode, EpisodeMode: template.EpisodeMode, MinimumCandidates: template.MinimumCandidates, MinimumDistinctSeries: template.MinimumDistinctSeries, RequiredEntityKinds: template.RequiredEntityKinds, CandidateLimit: template.CandidateLimit, RecencyDays: template.RecencyDays, Sort: template.Sort, LogoRef: builtInLibraryChannelLogoRef(template.Key)}
}

func libraryChannelLogoURL(logo librarychannels.LogoConfig) string {
	if logo.Source == librarychannels.LogoNone || strings.TrimSpace(logo.Ref) == "" {
		return ""
	}
	return "/api/library-channels/logos/" + logo.Ref
}

func isLibraryChannelOwner(user User) bool {
	return canInteractivelyManageServer(user)
}

func parseLibraryChannelTime(value string, fallback time.Time) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return fallback.UTC().Truncate(time.Second), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	return parsed.UTC().Truncate(time.Second), err
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func libraryChannelAnyStrings(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func firstNonNilError(value error, fallback error) error {
	if value != nil {
		return value
	}
	return fallback
}

func writeLibraryChannelGenerationError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		w.Header().Set("Retry-After", "5")
		writeProductError(w, http.StatusServiceUnavailable, "library_channel_generation_timeout", "Library Channel generation exceeded its request budget. Try again.")
		return
	}
	writeLibraryChannelError(w, err)
}

func writeLibraryChannelError(w http.ResponseWriter, err error) {
	var validation *librarychannels.ValidationError
	switch {
	case errors.As(err, &validation):
		writeProductError(w, http.StatusBadRequest, "library_channel_invalid_request", validation.Error())
	case errors.Is(err, librarychannels.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		writeProductError(w, http.StatusNotFound, "library_channel_not_found", "Library Channel was not found.")
	case errors.Is(err, librarychannels.ErrRevisionConflict), errors.Is(err, librarychannels.ErrGenerationStale):
		writeProductError(w, http.StatusConflict, "library_channel_revision_conflict", "Library Channel changed elsewhere. Reload it and try again.")
	case errors.Is(err, librarychannels.ErrTemplateExists):
		writeProductError(w, http.StatusConflict, "library_channel_template_exists", "That default Library Channel is already configured.")
	case errors.Is(err, librarychannels.ErrGenerationInProgress):
		w.Header().Set("Retry-After", "5")
		writeProductError(w, http.StatusConflict, "library_channel_generation_in_progress", "This Library Channel schedule is already being generated.")
	case errors.Is(err, librarychannels.ErrNoPlayableSchedule):
		writeProductError(w, http.StatusConflict, "library_channel_no_playable_schedule", "The current rules did not produce a playable Library Channel schedule.")
	case errors.Is(err, librarychannels.ErrProgramRestricted):
		writeProductError(w, http.StatusForbidden, "library_channel_program_restricted", "This scheduled program is not available to the current profile.")
	case errors.Is(err, librarychannels.ErrProgramUnavailable):
		writeProductError(w, http.StatusGone, "library_channel_program_unavailable", "This scheduled program is no longer available.")
	default:
		writeProductError(w, http.StatusInternalServerError, "library_channel_request_failed", "Portico could not complete the Library Channel request.")
	}
}
