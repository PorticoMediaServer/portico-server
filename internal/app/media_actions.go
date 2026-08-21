package app

import (
	"context"
	"errors"
	"strings"
)

// Media action identifiers are a cross-client product contract. Clients must
// render only the actions projected by the server; in particular, they must
// never infer playability from an entity kind or the presence of artwork.
const (
	mediaActionPlay            = "play"
	mediaActionPlayBeginning   = "play.from-beginning"
	mediaActionDownload        = "download"
	mediaActionQueueAdd        = "queue.add"
	mediaActionWatchlistAdd    = "watchlist.add"
	mediaActionWatchlistRemove = "watchlist.remove"
	mediaActionFavoriteAdd     = "favorite.add"
	mediaActionFavoriteRemove  = "favorite.remove"
	mediaActionWatchedSet      = "watched.set"
	mediaActionWatchedMark     = "watched.mark"
	mediaActionWatchedUnmark   = "watched.unmark"
	mediaActionReactionSet     = "reaction.set"
	mediaActionRatingSet       = "rating.set"
	mediaActionCollectionAdd   = "collection.add"
	mediaActionPlaylistAdd     = "playlist.add"
	mediaActionMetadataEdit    = "metadata.edit"
	mediaActionMetadataRefresh = "metadata.refresh"
	mediaActionMediaAnalyze    = "media.analyze"
	mediaActionMediaOptimize   = "media.optimize"
	mediaActionMediaDelete     = "media.delete"
	mediaActionLivePlay        = "live.play"
	mediaActionDVRRecord       = "dvr.record"
	mediaActionDVRRecordSeries = "dvr.record-series"
	mediaActionDVRPlay         = "dvr.play"
	mediaActionDVRCancel       = "dvr.cancel"
	mediaActionDVRDelete       = "dvr.delete"
	mediaActionDVREdit         = "dvr.edit"
	mediaActionDVREnable       = "dvr.enable"
	mediaActionDVRDisable      = "dvr.disable"
	mediaActionDVRRuleCreate   = "dvr.rule.create"
	mediaActionReportProblem   = "feedback.report-problem"
	mediaActionRequestQuality  = "feedback.request-higher-quality"
	mediaActionWatchTogether   = "watch-with-friends.start"
)

type mediaActionContextKey struct{}

func contextWithMediaActionUser(ctx context.Context, user User) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, mediaActionContextKey{}, user)
}

func mediaActionUserFromContext(ctx context.Context, userID string) (User, bool) {
	if ctx == nil {
		return User{}, false
	}
	user, ok := ctx.Value(mediaActionContextKey{}).(User)
	if !ok || strings.TrimSpace(user.ID) == "" {
		return User{}, false
	}
	identityID := strings.TrimSpace(userID)
	return user, identityID != "" && (identityID == viewerProfileID(user) || identityID == accountIDForUser(user))
}

func (s *Server) mediaActionUserContext(ctx context.Context, userID string) (User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return User{}, nil
	}
	if user, ok := mediaActionUserFromContext(ctx, userID); ok {
		return user, nil
	}
	accountID, profileID := s.accountAndProfileIDsContext(ctx, userID)
	principal, err := s.resolveRequestPrincipalContext(ctx, accountID, profileID)
	if errors.Is(err, errProfileNotFound) {
		return User{}, nil
	}
	if err != nil {
		return User{}, err
	}
	user := User{ID: accountID, AccountID: accountID, ProfileID: profileID}
	if err := s.queryUserRow(ctx, `SELECT role, COALESCE(auth_origin, 'local') FROM users WHERE id = ?`, accountID).Scan(&user.Role, &user.AuthOrigin); err != nil {
		return User{}, err
	}
	applyRequestPrincipal(&user, principal)
	return user, nil
}

func canonicalMediaActionCapabilities() []MediaActionCapability {
	api := func(id string, mutating, bulk bool, method, path, result string, body map[string]any, inputs, invalidates []string, confirmation bool) MediaActionCapability {
		execution := "single"
		if bulk {
			execution = "per-item"
		}
		return MediaActionCapability{
			ID: id, Mutating: mutating, BulkSupported: bulk,
			Presentation: mediaActionPresentation(id),
			Command:      MediaActionCommand{Kind: "api", Execution: execution, Method: method, PathTemplate: path, StaticBody: body, RequiredInputs: nonNilStrings(inputs), ResultHandling: result},
			Confirmation: MediaActionConfirmation{Required: confirmation, Tone: map[bool]string{true: "destructive", false: "none"}[confirmation]},
			Invalidates:  nonNilStrings(invalidates),
		}
	}
	flow := func(id string, mutating, bulk bool, flowID string, invalidates []string) MediaActionCapability {
		return MediaActionCapability{ID: id, Mutating: mutating, BulkSupported: bulk,
			Presentation: mediaActionPresentation(id),
			Command:      MediaActionCommand{Kind: "client-flow", Execution: "selection", FlowID: flowID, RequiredInputs: []string{}, ResultHandling: "flow"},
			Confirmation: MediaActionConfirmation{Tone: "none"}, Invalidates: nonNilStrings(invalidates)}
	}
	return []MediaActionCapability{
		api(mediaActionPlay, false, false, "POST", "/api/playback-sessions", "playback-session", nil, []string{"mediaId", "clientProfile"}, []string{"playback.active", "activity"}, false),
		api(mediaActionPlayBeginning, false, false, "POST", "/api/playback-sessions", "playback-session", map[string]any{"startSeconds": 0}, []string{"mediaId", "clientProfile"}, []string{"playback.active", "activity"}, false),
		api(mediaActionDownload, false, true, "POST", "/api/media/{mediaId}/download-grants", "download-grant", map[string]any{"profile": "source"}, []string{"mediaId"}, nil, false),
		flow(mediaActionQueueAdd, true, true, "queue-selection", []string{"playback.queue"}),
		api(mediaActionWatchlistAdd, true, true, "POST", "/api/media/{mediaId}/watchlist", "json", map[string]any{"watchlisted": true}, []string{"mediaId"}, []string{"media", "watchlist", "home"}, false),
		api(mediaActionWatchlistRemove, true, true, "POST", "/api/media/{mediaId}/watchlist", "json", map[string]any{"watchlisted": false}, []string{"mediaId"}, []string{"media", "watchlist", "home"}, false),
		api(mediaActionFavoriteAdd, true, true, "POST", "/api/media/{mediaId}/favorite", "json", map[string]any{"favorite": true}, []string{"mediaId"}, []string{"media", "favorites", "home"}, false),
		api(mediaActionFavoriteRemove, true, true, "POST", "/api/media/{mediaId}/favorite", "json", map[string]any{"favorite": false}, []string{"mediaId"}, []string{"media", "favorites", "home"}, false),
		api(mediaActionWatchedSet, true, true, "POST", "/api/media/{mediaId}/watched", "json", nil, []string{"mediaId", "watched"}, []string{"media", "home", "history"}, false),
		api(mediaActionWatchedMark, true, true, "POST", "/api/media/{mediaId}/watched", "json", map[string]any{"watched": true}, []string{"mediaId"}, []string{"media", "home", "history"}, false),
		api(mediaActionWatchedUnmark, true, true, "POST", "/api/media/{mediaId}/watched", "json", map[string]any{"watched": false}, []string{"mediaId"}, []string{"media", "home", "history"}, false),
		api(mediaActionReactionSet, true, false, "POST", "/api/media/{mediaId}/reaction", "json", nil, []string{"mediaId", "reaction"}, []string{"media", "recommendations"}, false),
		api(mediaActionRatingSet, true, false, "POST", "/api/media/{mediaId}/rating", "json", nil, []string{"mediaId", "rating"}, []string{"media", "recommendations"}, false),
		flow(mediaActionCollectionAdd, true, true, "collection-picker", []string{"collections", "media"}),
		flow(mediaActionPlaylistAdd, true, true, "playlist-picker", []string{"playlists", "media"}),
		flow(mediaActionMetadataEdit, true, true, "metadata-editor", []string{"media", "libraries"}),
		api(mediaActionMetadataRefresh, true, true, "POST", "/api/media/{mediaId}/jobs", "job", map[string]any{"type": "metadata_refresh"}, []string{"mediaId"}, []string{"media", "jobs"}, false),
		api(mediaActionMediaAnalyze, true, true, "POST", "/api/media/{mediaId}/jobs", "job", map[string]any{"type": "media_analyze"}, []string{"mediaId", "analysisMode"}, []string{"media", "jobs"}, false),
		api(mediaActionMediaOptimize, true, true, "POST", "/api/media/{mediaId}/jobs", "job", map[string]any{"type": "optimize_version"}, []string{"mediaId", "profile"}, []string{"media", "jobs"}, false),
		api(mediaActionMediaDelete, true, true, "DELETE", "/api/media/{mediaId}", "json", nil, []string{"mediaId"}, []string{"media", "libraries", "home", "saved"}, true),
		flow(mediaActionLivePlay, false, false, "live-playback", []string{"playback.active", "activity"}),
		api(mediaActionDVRRecord, true, false, "POST", "/api/dvr/recordings", "json", nil, []string{"sourceId", "channelId", "programId", "title", "startsAt", "endsAt"}, []string{"dvr.recordings", "dvr.schedule", "live-tv.guide"}, false),
		api(mediaActionDVRRecordSeries, true, false, "POST", "/api/dvr/rules", "json", map[string]any{"matchType": "series"}, []string{"sourceId", "channelId", "programId", "title"}, []string{"dvr.rules", "dvr.schedule", "live-tv.guide"}, false),
		api(mediaActionDVRPlay, false, false, "POST", "/api/dvr/recordings/{id}/playback", "playback-session", nil, []string{"id", "clientProfile"}, []string{"playback.active", "activity"}, false),
		api(mediaActionDVRCancel, true, false, "DELETE", "/api/dvr/recordings/{id}", "json", nil, []string{"id"}, []string{"dvr.recordings", "dvr.schedule", "dvr.conflicts"}, true),
		api(mediaActionDVRDelete, true, false, "DELETE", "/api/dvr/recordings/{id}", "json", nil, []string{"id"}, []string{"dvr.recordings", "dvr.schedule", "recorded-tv"}, true),
		flow(mediaActionDVREdit, true, false, "dvr-recording-editor", []string{"dvr.recordings", "dvr.schedule"}),
		api(mediaActionDVREnable, true, false, "PATCH", "/api/dvr/rules/{id}", "json", map[string]any{"enabled": true}, []string{"id", "expectedRevision"}, []string{"dvr.rules", "dvr.schedule", "dvr.conflicts"}, false),
		api(mediaActionDVRDisable, true, false, "PATCH", "/api/dvr/rules/{id}", "json", map[string]any{"enabled": false}, []string{"id", "expectedRevision"}, []string{"dvr.rules", "dvr.schedule", "dvr.conflicts"}, false),
		flow(mediaActionDVRRuleCreate, true, false, "dvr-rule-editor", []string{"dvr.rules", "dvr.schedule", "dvr.conflicts"}),
		flow(mediaActionReportProblem, false, false, "feedback-report", nil),
		flow(mediaActionRequestQuality, false, false, "quality-request", nil),
		flow(mediaActionWatchTogether, false, false, "watch-with-friends", []string{"playback.active", "watch-with-friends"}),
	}
}

func mediaActionPresentation(id string) MediaActionPresentation {
	all := []string{"web", "mobile", "television"}
	mobile := []string{"mobile"}
	admin := []string{"web-admin"}
	presentations := map[string]MediaActionPresentation{
		mediaActionPlay:            {LabelMessageID: "action.play", IconID: "action.play", Group: "playback", Priority: 100, Surfaces: all},
		mediaActionPlayBeginning:   {LabelMessageID: "action.play-from-beginning", IconID: "action.restart", Group: "playback", Priority: 95, Surfaces: all},
		mediaActionLivePlay:        {LabelMessageID: "action.play", IconID: "action.play", Group: "playback", Priority: 100, Surfaces: all},
		mediaActionDVRRecord:       {LabelMessageID: "action.record-once", IconID: "action.record", Group: "recording", Priority: 90, Surfaces: all},
		mediaActionDVRRecordSeries: {LabelMessageID: "action.record-series", IconID: "action.record", Group: "recording", Priority: 85, Surfaces: all},
		mediaActionDVRPlay:         {LabelMessageID: "action.play-recording", IconID: "action.play", Group: "playback", Priority: 100, Surfaces: all},
		mediaActionDVRCancel:       {LabelMessageID: "action.cancel-recording", IconID: "action.cancel", Group: "recording", Priority: 70, Surfaces: all},
		mediaActionDVRDelete:       {LabelMessageID: "action.delete-recording", IconID: "action.delete", Group: "recording", Priority: 60, Surfaces: all},
		mediaActionDVREdit:         {LabelMessageID: "action.edit-recording", IconID: "action.settings", Group: "recording", Priority: 55, Surfaces: all},
		mediaActionDVREnable:       {LabelMessageID: "action.enable-recording-rule", IconID: "action.record", Group: "recording", Priority: 50, Surfaces: all},
		mediaActionDVRDisable:      {LabelMessageID: "action.disable-recording-rule", IconID: "action.record", Group: "recording", Priority: 50, Surfaces: all},
		mediaActionDVRRuleCreate:   {LabelMessageID: "action.create-rule", IconID: "action.record", Group: "recording", Priority: 65, Surfaces: all},
		mediaActionDownload:        {LabelMessageID: "action.download", IconID: "action.download", Group: "playback", Priority: 70, Surfaces: mobile},
		mediaActionQueueAdd:        {LabelMessageID: "action.add-queue", IconID: "action.queue", Group: "playback", Priority: 60, Surfaces: all},
		mediaActionWatchlistAdd:    {LabelMessageID: "action.add-watchlist", IconID: "action.watchlist", Group: "saved", Priority: 90, Surfaces: all},
		mediaActionWatchlistRemove: {LabelMessageID: "action.remove-watchlist", IconID: "action.watchlist", Group: "saved", Priority: 90, Surfaces: all},
		mediaActionFavoriteAdd:     {LabelMessageID: "action.add-favorite", IconID: "action.favorite", Group: "saved", Priority: 80, Surfaces: all},
		mediaActionFavoriteRemove:  {LabelMessageID: "action.remove-favorite", IconID: "action.favorite", Group: "saved", Priority: 80, Surfaces: all},
		mediaActionWatchedSet:      {LabelMessageID: "action.mark-watched", IconID: "action.watched", Group: "state", Priority: 70, Surfaces: all},
		mediaActionWatchedMark:     {LabelMessageID: "action.mark-watched", IconID: "action.watched", Group: "state", Priority: 70, Surfaces: all},
		mediaActionWatchedUnmark:   {LabelMessageID: "action.mark-unwatched", IconID: "action.watched", Group: "state", Priority: 70, Surfaces: all},
		mediaActionReactionSet:     {LabelMessageID: "action.react", IconID: "action.reaction", Group: "state", Priority: 50, Surfaces: all},
		mediaActionRatingSet:       {LabelMessageID: "action.rate", IconID: "action.rating", Group: "state", Priority: 50, Surfaces: all},
		mediaActionCollectionAdd:   {LabelMessageID: "action.add-collection", IconID: "action.collection", Group: "saved", Priority: 40, Surfaces: all},
		mediaActionPlaylistAdd:     {LabelMessageID: "action.add-playlist", IconID: "action.playlist", Group: "saved", Priority: 40, Surfaces: all},
		mediaActionMetadataEdit:    {LabelMessageID: "action.edit-metadata", IconID: "action.metadata", Group: "administration", Priority: 50, Surfaces: admin},
		mediaActionMetadataRefresh: {LabelMessageID: "action.refresh-metadata", IconID: "action.refresh", Group: "administration", Priority: 40, Surfaces: admin},
		mediaActionMediaAnalyze:    {LabelMessageID: "action.analyze-media", IconID: "action.metadata", Group: "administration", Priority: 30, Surfaces: admin},
		mediaActionMediaOptimize:   {LabelMessageID: "action.optimize-media", IconID: "action.optimize", Group: "administration", Priority: 20, Surfaces: admin},
		mediaActionMediaDelete:     {LabelMessageID: "action.delete-media", IconID: "action.delete", Group: "administration", Priority: 10, Surfaces: admin},
		mediaActionReportProblem:   {LabelMessageID: "action.report-problem", IconID: "action.report", Group: "feedback", Priority: 20, Surfaces: all},
		mediaActionRequestQuality:  {LabelMessageID: "action.request-higher-quality", IconID: "action.quality", Group: "feedback", Priority: 10, Surfaces: all},
		mediaActionWatchTogether:   {LabelMessageID: "action.watch-with-friends", IconID: "action.people", Group: "playback", Priority: 55, Surfaces: all},
	}
	return presentations[id]
}

func mediaActionsForItem(item MediaItem, user User) []string {
	actions := []string{}
	if strings.TrimSpace(user.ID) == "" {
		return actions
	}
	if item.Type == "live_channel" {
		if canPlayLiveTV(user) {
			return append(actions, mediaActionLivePlay)
		}
		return actions
	}

	personalStateAllowed := true
	savedResourceMutationAllowed := true
	if user.AuthProvider == "api_key" {
		scopes := stringSet(user.APIKeyScopes)
		personalStateAllowed = scopes["all"] || scopes["editMetadata"] || scopes["deleteMedia"]
		savedResourceMutationAllowed = scopes["all"] || scopes["playMedia"] || scopes["editMetadata"]
	}
	if personalStateAllowed {
		if item.State.Watchlisted {
			actions = append(actions, mediaActionWatchlistRemove)
		} else {
			actions = append(actions, mediaActionWatchlistAdd)
		}
		if item.State.Favorite {
			actions = append(actions, mediaActionFavoriteRemove)
		} else {
			actions = append(actions, mediaActionFavoriteAdd)
		}
		if item.State.Watched {
			actions = append(actions, mediaActionWatchedUnmark)
		} else {
			actions = append(actions, mediaActionWatchedMark)
		}
		actions = append(actions, mediaActionReactionSet, mediaActionRatingSet)
	}
	if savedResourceMutationAllowed {
		actions = append(actions, mediaActionCollectionAdd, mediaActionPlaylistAdd)
	}

	available := item.FileCount == 0 || item.MissingFileCount < item.FileCount
	playable := playableMediaType(item.Type) && available && mediaItemHasPlayableFile(item)
	if playable && hasPermission(user, "playMedia") {
		actions = append([]string{mediaActionPlay, mediaActionQueueAdd}, actions...)
		if item.State.ProgressSeconds > 0 || item.State.Watched {
			actions = append([]string{mediaActionPlay, mediaActionPlayBeginning, mediaActionQueueAdd}, actions[2:]...)
		}
		if hasPermission(user, "watchWithFriends") && user.Preferences.Privacy.IncludeInWatchWithFriends {
			actions = append(actions, mediaActionWatchTogether)
		}
	}
	if playable && hasPermission(user, "downloadMedia") {
		actions = append(actions, mediaActionDownload)
	}
	if user.AllowFeedback && user.AuthProvider != "api_key" {
		actions = append(actions, mediaActionReportProblem)
		if playable {
			actions = append(actions, mediaActionRequestQuality)
		}
	}
	if hasPermission(user, "editMetadata") {
		actions = append(actions, mediaActionMetadataEdit)
	}
	if hasPermission(user, "manageLibraries") {
		actions = append(actions, mediaActionMetadataRefresh, mediaActionMediaAnalyze, mediaActionMediaOptimize)
	}
	if hasPermission(user, "deleteMedia") {
		actions = append(actions, mediaActionMediaDelete)
	}
	return actions
}

func mediaItemHasPlayableFile(item MediaItem) bool {
	if len(item.MediaFiles) == 0 {
		return true
	}
	for _, file := range item.MediaFiles {
		switch strings.ToLower(strings.TrimSpace(file.SourceType)) {
		case "disc-image", "dvd-structure", "bluray-structure":
			continue
		default:
			if file.Available {
				return true
			}
		}
	}
	return false
}

func applyMediaActionsToItem(item *MediaItem, user User) {
	if item == nil {
		return
	}
	item.Actions = mediaActionsForItem(*item, user)
	for index := range item.Children {
		applyMediaActionsToItem(&item.Children[index], user)
	}
	for extraIndex := range item.Extras {
		for itemIndex := range item.Extras[extraIndex].Items {
			applyMediaActionsToItem(&item.Extras[extraIndex].Items[itemIndex], user)
		}
	}
	for rowIndex := range item.RecommendationRows {
		for itemIndex := range item.RecommendationRows[rowIndex].Items {
			applyMediaActionsToItem(&item.RecommendationRows[rowIndex].Items[itemIndex], user)
		}
	}
}

func (s *Server) applyMediaActionProjectionContext(ctx context.Context, userID string, items []MediaItem) ([]MediaItem, error) {
	if items == nil {
		items = []MediaItem{}
	}
	if len(items) == 0 {
		return items, nil
	}
	user, err := s.mediaActionUserContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !s.viewerFeedbackPolicyContext(ctx).Enabled {
		user.AllowFeedback = false
	}
	for index := range items {
		applyMediaActionsToItem(&items[index], user)
	}
	return items, nil
}
