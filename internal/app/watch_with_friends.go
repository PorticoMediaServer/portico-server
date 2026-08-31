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
	"unicode/utf8"
)

const (
	maxWatchWithFriendsMembers         = 32
	maxWatchWithFriendsCommandReceipts = 128
	watchWithFriendsMemberStaleAfter   = 2 * time.Minute
	// Authorization changes must close an already-open group event stream
	// promptly. This ticker is deliberately separate from the inexpensive SSE
	// keep-alive so policy revocation is never delayed for twenty seconds.
	watchWithFriendsAuthorizationRecheck = 2 * time.Second
)

var (
	errWatchWithFriendsHostRequired       = errors.New("Only the group host can change group playback, playback order, or queue items.")
	errWatchWithFriendsPermissionRequired = errors.New("This profile is not allowed to use Watch With Friends.")
	errWatchWithFriendsRevisionConflict   = errors.New("Watch With Friends state changed before this command was applied.")
	errWatchWithFriendsInvalidRequest     = errors.New("Watch With Friends request is invalid.")
)

func writeWatchWithFriendsMutationError(w http.ResponseWriter, err error, code string) {
	if errors.Is(err, errWatchWithFriendsPermissionRequired) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to use Watch With Friends.")
		return
	}
	if errors.Is(err, errWatchWithFriendsHostRequired) {
		writeError(w, http.StatusForbidden, "watch_with_friends_host_required", errWatchWithFriendsHostRequired.Error())
		return
	}
	if errors.Is(err, errWatchWithFriendsRevisionConflict) {
		writeError(w, http.StatusConflict, "watch_with_friends_revision_conflict", errWatchWithFriendsRevisionConflict.Error())
		return
	}
	if errors.Is(err, errWatchWithFriendsInvalidRequest) {
		writeError(w, http.StatusBadRequest, "watch_with_friends_invalid_request", errWatchWithFriendsInvalidRequest.Error())
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "watch_with_friends_not_found", "Watch With Friends group was not found.")
		return
	}
	writeError(w, http.StatusBadRequest, code, "Unable to update the Watch With Friends group.")
}

func watchWithFriendsExpectedRevisionQuery(w http.ResponseWriter, r *http.Request) (*int64, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("expectedRevision"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "watch_with_friends_invalid_revision", "Expected revision is required for this mutation.")
		return nil, false
	}
	revision, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || revision < 0 {
		writeError(w, http.StatusBadRequest, "watch_with_friends_invalid_revision", "Expected revision must be a non-negative integer.")
		return nil, false
	}
	return &revision, true
}

func watchWithFriendsRequiredIdempotencyKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" || utf8.RuneCountInString(key) > 120 {
		return "", errWatchWithFriendsInvalidRequest
	}
	return key, nil
}

func watchWithFriendsIdempotencyKeyQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	key, err := watchWithFriendsRequiredIdempotencyKey(r.URL.Query().Get("idempotencyKey"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "watch_with_friends_invalid_request", "A nonempty idempotency key of at most 120 characters is required for this mutation.")
		return "", false
	}
	return key, true
}

func (s *Server) handleWatchWithFriendsGroups(w http.ResponseWriter, r *http.Request, user User) {
	// Watch With Friends is a viewer capability. Administrative authority must
	// never imply it, and the selected profile's effective policy is the only
	// authority used at this boundary.
	if !s.watchWithFriendsProfilePermissionAllowedContext(r.Context(), viewerProfileID(user)) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to use Watch With Friends.")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/watch-with-friends/groups"), "/")
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			groups, err := s.listWatchWithFriendsGroupsContext(r.Context(), user)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "watch_with_friends_failed", "Unable to load Watch With Friends groups.")
				return
			}
			writeJSON(w, http.StatusOK, ListResponse[WatchWithFriendsGroup]{Items: groups, Total: len(groups)})
		case http.MethodPost:
			var req WatchWithFriendsCreateRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			group, err := s.createWatchWithFriendsGroupContext(r.Context(), user, req)
			if err != nil {
				writeError(w, http.StatusBadRequest, "watch_with_friends_create_failed", "Unable to create the Watch With Friends group.")
				return
			}
			s.notifyWatchWithFriendsGroup(group.ID)
			writeJSON(w, http.StatusCreated, group)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
		}
		return
	}

	parts := strings.Split(path, "/")
	groupID := strings.TrimSpace(parts[0])
	if groupID == "" {
		writeError(w, http.StatusNotFound, "not_found", "Watch With Friends group was not found.")
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			group, err := s.watchWithFriendsGroupForUserContext(r.Context(), user, groupID, false)
			if err != nil {
				writeError(w, http.StatusNotFound, "watch_with_friends_not_found", "Watch With Friends group was not found.")
				return
			}
			writeJSON(w, http.StatusOK, group)
		case http.MethodDelete:
			idempotencyKey, ok := watchWithFriendsIdempotencyKeyQuery(w, r)
			if !ok {
				return
			}
			expectedRevision, ok := watchWithFriendsExpectedRevisionQuery(w, r)
			if !ok {
				return
			}
			group, err := s.endWatchWithFriendsGroupExpectedContext(r.Context(), user, groupID, expectedRevision, idempotencyKey)
			if err != nil {
				writeWatchWithFriendsMutationError(w, err, "watch_with_friends_end_failed")
				return
			}
			s.notifyWatchWithFriendsGroup(groupID)
			writeJSON(w, http.StatusOK, group)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or DELETE for this endpoint.")
		}
		return
	}
	switch parts[1] {
	case "events":
		if len(parts) == 3 && parts[2] == "poll" {
			s.handleWatchWithFriendsGroupEventsPoll(w, r, user, groupID)
			return
		}
		if len(parts) != 2 {
			writeError(w, http.StatusNotFound, "not_found", "Watch With Friends event route was not found.")
			return
		}
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
			return
		}
		s.streamWatchWithFriendsGroupEvents(w, r, user, groupID)
	case "join":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		group, err := s.joinWatchWithFriendsGroupContext(r.Context(), user, groupID)
		if err != nil {
			writeWatchWithFriendsMutationError(w, err, "watch_with_friends_join_failed")
			return
		}
		s.notifyWatchWithFriendsGroup(groupID)
		writeJSON(w, http.StatusOK, group)
	case "leave":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		group, err := s.leaveWatchWithFriendsGroupContext(r.Context(), user, groupID)
		if err != nil {
			writeWatchWithFriendsMutationError(w, err, "watch_with_friends_leave_failed")
			return
		}
		s.notifyWatchWithFriendsGroup(groupID)
		writeJSON(w, http.StatusOK, group)
	case "state":
		if r.Method != http.MethodPatch {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PATCH for this endpoint.")
			return
		}
		var req WatchWithFriendsStateRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		group, err := s.updateWatchWithFriendsStateContext(r.Context(), user, groupID, req)
		if err != nil {
			writeWatchWithFriendsMutationError(w, err, "watch_with_friends_state_failed")
			return
		}
		s.notifyWatchWithFriendsGroup(groupID)
		writeJSON(w, http.StatusOK, group)
	case "settings":
		if r.Method != http.MethodPatch {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PATCH for this endpoint.")
			return
		}
		var req WatchWithFriendsSettingsRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		group, err := s.updateWatchWithFriendsSettingsExpectedContext(r.Context(), user, groupID, req, req.ExpectedRevision)
		if err != nil {
			writeWatchWithFriendsMutationError(w, err, "watch_with_friends_settings_failed")
			return
		}
		s.notifyWatchWithFriendsGroup(groupID)
		writeJSON(w, http.StatusOK, group)
	case "member":
		if len(parts) < 3 || parts[2] != "state" {
			writeError(w, http.StatusNotFound, "not_found", "Watch With Friends member route was not found.")
			return
		}
		if r.Method != http.MethodPatch {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PATCH for this endpoint.")
			return
		}
		var req WatchWithFriendsMemberStateRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		group, err := s.updateWatchWithFriendsMemberStateContext(r.Context(), user, groupID, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "watch_with_friends_member_state_failed", "Unable to update participant state.")
			return
		}
		s.notifyWatchWithFriendsGroup(groupID)
		writeJSON(w, http.StatusOK, group)
	case "queue":
		if r.Method == http.MethodPost && len(parts) == 2 {
			var req WatchWithFriendsQueueRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			group, err := s.addWatchWithFriendsQueueItemExpectedContext(r.Context(), user, groupID, req, req.ExpectedRevision)
			if err != nil {
				writeWatchWithFriendsMutationError(w, err, "watch_with_friends_queue_failed")
				return
			}
			s.notifyWatchWithFriendsGroup(groupID)
			writeJSON(w, http.StatusOK, group)
			return
		}
		if r.Method == http.MethodPatch && len(parts) == 2 {
			var req WatchWithFriendsQueueOrderRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			group, err := s.reorderWatchWithFriendsQueueExpectedContext(r.Context(), user, groupID, req, req.ExpectedRevision)
			if err != nil {
				writeWatchWithFriendsMutationError(w, err, "watch_with_friends_queue_failed")
				return
			}
			s.notifyWatchWithFriendsGroup(groupID)
			writeJSON(w, http.StatusOK, group)
			return
		}
		if r.Method == http.MethodDelete && len(parts) == 3 {
			entryID, err := url.PathUnescape(parts[2])
			if err != nil {
				writeError(w, http.StatusBadRequest, "watch_with_friends_queue_failed", "Queue media item is invalid.")
				return
			}
			idempotencyKey, ok := watchWithFriendsIdempotencyKeyQuery(w, r)
			if !ok {
				return
			}
			expectedRevision, ok := watchWithFriendsExpectedRevisionQuery(w, r)
			if !ok {
				return
			}
			group, err := s.removeWatchWithFriendsQueueItemExpectedContext(r.Context(), user, groupID, entryID, expectedRevision, idempotencyKey)
			if err != nil {
				writeWatchWithFriendsMutationError(w, err, "watch_with_friends_queue_failed")
				return
			}
			s.notifyWatchWithFriendsGroup(groupID)
			writeJSON(w, http.StatusOK, group)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST /queue, PATCH /queue, or DELETE /queue/{entryId}.")
	default:
		writeError(w, http.StatusNotFound, "not_found", "Watch With Friends route was not found.")
	}
}

func (s *Server) streamWatchWithFriendsGroupEvents(w http.ResponseWriter, r *http.Request, user User, groupID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "stream_unsupported", "Streaming is not supported by this connection.")
		return
	}
	if _, err := s.watchWithFriendsGroupForUserContext(r.Context(), user, groupID, true); err != nil {
		writeError(w, http.StatusNotFound, "watch_with_friends_not_found", "Watch With Friends group was not found.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Portico-Stream-Resume", "none")
	if streamResumeResetRequested(r, w, flusher) {
		return
	}
	updates, unsubscribe := s.subscribeWatchWithFriendsGroup(groupID)
	defer unsubscribe()
	lastRevision := int64(-1)
	writeGroup := func(group WatchWithFriendsGroup, force bool) bool {
		if !force && group.Revision == lastRevision {
			return true
		}
		data, err := json.Marshal(group)
		if err != nil {
			return false
		}
		prepareStreamWrite(w)
		if _, err := fmt.Fprintf(w, "id: %d\nevent: group\ndata: %s\n\n", group.Revision, data); err != nil {
			return false
		}
		flusher.Flush()
		lastRevision = group.Revision
		return true
	}
	writeWatchWithFriendsEvent := func(force bool) (bool, bool) {
		group, ended, err := s.materializeWatchWithFriendsEvent(r.Context(), user, groupID)
		if err != nil {
			return false, false
		}
		return writeGroup(group, force || ended), ended
	}
	if ok, ended := writeWatchWithFriendsEvent(true); !ok || ended {
		return
	}
	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()
	authorizationRecheck := time.NewTicker(watchWithFriendsAuthorizationRecheck)
	defer authorizationRecheck.Stop()
	shutdownDone := s.shutdownDone()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-shutdownDone:
			return
		case <-updates:
			if ok, ended := writeWatchWithFriendsEvent(true); !ok || ended {
				return
			}
		case <-authorizationRecheck.C:
			if !s.longPollUserAuthorizationCurrent(r, user) || !s.watchWithFriendsStreamAuthorizedContext(r.Context(), user, groupID) {
				return
			}
		case <-keepAlive.C:
			prepareStreamWrite(w)
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) watchWithFriendsStreamAuthorizedContext(ctx context.Context, user User, groupID string) bool {
	profileID := viewerProfileID(user)
	if !s.watchWithFriendsProfilePermissionAllowedContext(ctx, profileID) {
		return false
	}
	var memberCount int
	err := s.queryUserRow(ctx, `
		SELECT COUNT(*)
		FROM watch_with_friends_members AS member
		JOIN watch_with_friends_groups AS watch_group ON watch_group.id = member.group_id
		WHERE member.group_id = ? AND member.profile_id = ? AND watch_group.ended_at = ''`, groupID, profileID).Scan(&memberCount)
	return err == nil && memberCount == 1
}

func (s *Server) subscribeWatchWithFriendsGroup(groupID string) (<-chan struct{}, func()) {
	groupID = strings.TrimSpace(groupID)
	updates := make(chan struct{}, 1)
	if groupID == "" {
		return updates, func() {}
	}
	s.watchWithFriendsMu.Lock()
	if s.watchWithFriendsWatchers == nil {
		s.watchWithFriendsWatchers = map[string]map[chan struct{}]struct{}{}
	}
	watchers := s.watchWithFriendsWatchers[groupID]
	if watchers == nil {
		watchers = map[chan struct{}]struct{}{}
		s.watchWithFriendsWatchers[groupID] = watchers
	}
	watchers[updates] = struct{}{}
	s.watchWithFriendsMu.Unlock()
	return updates, func() {
		s.watchWithFriendsMu.Lock()
		if watchers := s.watchWithFriendsWatchers[groupID]; watchers != nil {
			delete(watchers, updates)
			if len(watchers) == 0 {
				delete(s.watchWithFriendsWatchers, groupID)
			}
		}
		s.watchWithFriendsMu.Unlock()
		close(updates)
	}
}

func (s *Server) notifyWatchWithFriendsGroup(groupID string) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return
	}
	s.watchWithFriendsMu.Lock()
	defer s.watchWithFriendsMu.Unlock()
	for watcher := range s.watchWithFriendsWatchers[groupID] {
		select {
		case watcher <- struct{}{}:
		default:
		}
	}
	if s.longPoll != nil {
		s.longPoll.broker.publish("watch-with-friends:" + groupID)
	}
}

func (s *Server) listWatchWithFriendsGroups(user User) ([]WatchWithFriendsGroup, error) {
	return s.listWatchWithFriendsGroupsContext(context.Background(), user)
}

func (s *Server) listWatchWithFriendsGroupsContext(ctx context.Context, user User) ([]WatchWithFriendsGroup, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT id
		FROM watch_with_friends_groups
		WHERE ended_at = ''
		ORDER BY updated_at DESC
		LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []WatchWithFriendsGroup{}
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return nil, err
		}
		group, err := s.watchWithFriendsGroupForUserContext(ctx, user, groupID, false)
		if err == nil {
			groups = append(groups, group)
		}
	}
	return groups, rows.Err()
}

func (s *Server) createWatchWithFriendsGroup(user User, req WatchWithFriendsCreateRequest) (WatchWithFriendsGroup, error) {
	return s.createWatchWithFriendsGroupContext(context.Background(), user, req)
}

func (s *Server) createWatchWithFriendsGroupContext(ctx context.Context, user User, req WatchWithFriendsCreateRequest) (WatchWithFriendsGroup, error) {
	if !s.watchWithFriendsProfilePermissionAllowedContext(ctx, viewerProfileID(user)) {
		return WatchWithFriendsGroup{}, errWatchWithFriendsPermissionRequired
	}
	mediaID := strings.TrimSpace(req.MediaID)
	if mediaID == "" {
		return WatchWithFriendsGroup{}, errors.New("Watch With Friends groups require a media item.")
	}
	item, err := s.getMediaListItemContext(ctx, viewerProfileID(user), mediaID)
	if err != nil {
		return WatchWithFriendsGroup{}, errors.New("Media item is not accessible.")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	name := strings.Join(strings.Fields(strings.TrimSpace(req.Name)), " ")
	if name == "" {
		name = item.Title
	}
	if len(name) > 80 {
		name = name[:80]
	}
	groupID := randomID("spg")
	currentEntryID := randomID("wfentry")
	command := PlaybackCommand{ID: randomID("pcmd"), Action: "pause", MediaID: item.ID, PositionSeconds: max(0, item.State.ProgressSeconds), IssuedByUserID: user.ID, IssuedByProfileID: viewerProfileID(user), IssuedAt: now}
	commandJSON, _ := json.Marshal(command)
	err = s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_with_friends_groups (id, owner_user_id, owner_profile_id, name, media_id, current_entry_id, media_title, state, position_seconds, position_updated_at, playback_rate, revision, playback_revision, command_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'paused', ?, ?, 1, 1, 1, ?, ?, ?)`,
			groupID, accountIDForUser(user), viewerProfileID(user), name, item.ID, currentEntryID, item.Title, command.PositionSeconds, now, string(commandJSON), now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_with_friends_members (group_id, user_id, profile_id, joined_at, last_seen_at)
			VALUES (?, ?, ?, ?, ?)`, groupID, accountIDForUser(user), viewerProfileID(user), now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_with_friends_queue (group_id, entry_id, media_id, media_title, sort_order, added_by_user_id, added_by_profile_id, added_at)
			VALUES (?, ?, ?, ?, 0, ?, ?, ?)`, groupID, currentEntryID, item.ID, item.Title, accountIDForUser(user), viewerProfileID(user), now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	return s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
}

func (s *Server) joinWatchWithFriendsGroup(user User, groupID string) (WatchWithFriendsGroup, error) {
	return s.joinWatchWithFriendsGroupContext(context.Background(), user, groupID)
}

func (s *Server) joinWatchWithFriendsGroupContext(ctx context.Context, user User, groupID string) (WatchWithFriendsGroup, error) {
	group, err := s.watchWithFriendsGroupForUserContext(ctx, user, groupID, false)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	fullGroup, err := s.watchWithFriendsGroupContext(ctx, groupID)
	if err != nil {
		return WatchWithFriendsGroup{}, sql.ErrNoRows
	}
	if _, accessErr := s.getMediaAccessSummaryContext(ctx, viewerProfileID(user), fullGroup.MediaID); accessErr != nil {
		return WatchWithFriendsGroup{}, sql.ErrNoRows
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		var memberCount int
		var alreadyMember int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(CASE WHEN profile_id = ? THEN 1 END) FROM watch_with_friends_members WHERE group_id = ?`, viewerProfileID(user), groupID).Scan(&memberCount, &alreadyMember); err != nil {
			return err
		}
		if memberCount >= maxWatchWithFriendsMembers && alreadyMember == 0 {
			return errors.New("Watch With Friends group is full.")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_with_friends_members (group_id, user_id, profile_id, joined_at, last_seen_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(group_id, profile_id) DO UPDATE SET last_seen_at = excluded.last_seen_at`,
			groupID, accountIDForUser(user), viewerProfileID(user), now, now); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET revision = revision + 1, reconnect_generation = reconnect_generation + 1, updated_at = ? WHERE id = ? AND ended_at = '' AND revision = ?`, now, groupID, group.Revision)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return errWatchWithFriendsRevisionConflict
		}
		return nil
	})
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	return s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
}

func (s *Server) leaveWatchWithFriendsGroup(user User, groupID string) (WatchWithFriendsGroup, error) {
	return s.leaveWatchWithFriendsGroupContext(context.Background(), user, groupID)
}

func (s *Server) leaveWatchWithFriendsGroupContext(ctx context.Context, user User, groupID string) (WatchWithFriendsGroup, error) {
	group, err := s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	if group.OwnerProfileID == viewerProfileID(user) {
		return s.endWatchWithFriendsGroupWithoutReceiptContext(ctx, user, group)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM watch_with_friends_members WHERE group_id = ? AND profile_id = ?`, groupID, viewerProfileID(user))
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return sql.ErrNoRows
		}
		result, err = tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET revision = revision + 1, reconnect_generation = reconnect_generation + 1, updated_at = ? WHERE id = ? AND ended_at = '' AND revision = ?`, now, groupID, group.Revision)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return errWatchWithFriendsRevisionConflict
		}
		return nil
	})
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	group.Revision++
	group.ReconnectGeneration++
	group.UpdatedAt = now
	group.Members = []WatchWithFriendsMember{}
	return group, nil
}

func (s *Server) updateWatchWithFriendsState(user User, groupID string, req WatchWithFriendsStateRequest) (WatchWithFriendsGroup, error) {
	return s.updateWatchWithFriendsStateContext(context.Background(), user, groupID, req)
}

func (s *Server) updateWatchWithFriendsStateContext(ctx context.Context, user User, groupID string, req WatchWithFriendsStateRequest) (WatchWithFriendsGroup, error) {
	idempotencyKey, err := watchWithFriendsRequiredIdempotencyKey(req.IdempotencyKey)
	if err != nil || req.ExpectedRevision == nil || *req.ExpectedRevision < 0 {
		return WatchWithFriendsGroup{}, errWatchWithFriendsInvalidRequest
	}
	fingerprint := watchWithFriendsStateRequestFingerprint(req)
	if receiptGroup, found, err := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || err != nil {
		return receiptGroup, err
	}
	group, err := s.watchWithFriendsGroupForHostContext(ctx, user, groupID)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	if *req.ExpectedRevision != group.Revision {
		return WatchWithFriendsGroup{}, errWatchWithFriendsRevisionConflict
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "play", "pause", "seek", "stop", "load", "next", "previous":
	default:
		return WatchWithFriendsGroup{}, errors.New("Watch With Friends action is not supported.")
	}
	mediaID := group.MediaID
	currentEntryID := group.CurrentEntryID
	mediaTitle := group.MediaTitle
	commandAction := action
	createOccurrence := false
	if action == "load" || action == "next" || action == "previous" {
		if action == "next" || action == "previous" {
			if strings.TrimSpace(req.EntryID) != "" || strings.TrimSpace(req.MediaID) != "" {
				return WatchWithFriendsGroup{}, errors.New("Adjacent playback is selected from the authoritative group queue.")
			}
			item, err := watchWithFriendsAdjacentQueueItem(group, action)
			if err != nil {
				return WatchWithFriendsGroup{}, err
			}
			mediaID = item.MediaID
			currentEntryID = item.EntryID
		} else {
			entryID := strings.TrimSpace(req.EntryID)
			mediaID = strings.TrimSpace(req.MediaID)
			if (entryID == "") == (mediaID == "") {
				return WatchWithFriendsGroup{}, errors.New("Load requires exactly one of entryId or mediaId.")
			}
			if entryID != "" {
				index := watchWithFriendsQueueEntryIndex(group.Queue, entryID)
				if index < 0 || group.Queue[index].Unavailable {
					return WatchWithFriendsGroup{}, errors.New("Load queue occurrence is not accessible.")
				}
				mediaID = group.Queue[index].MediaID
				currentEntryID = group.Queue[index].EntryID
			} else {
				currentEntryID = randomID("wfentry")
				createOccurrence = true
			}
		}
		if mediaID == "" {
			return WatchWithFriendsGroup{}, errors.New("Load actions require a media item.")
		}
		item, err := s.getMediaListItemContext(ctx, viewerProfileID(user), mediaID)
		if err != nil {
			return WatchWithFriendsGroup{}, errors.New("Load media item is not accessible.")
		}
		if !s.watchWithFriendsMediaVisibleToAllMembersContext(ctx, groupID, mediaID) {
			return WatchWithFriendsGroup{}, errors.New("That item is not available to every participant in this group.")
		}
		mediaTitle = item.Title
		commandAction = "load"
	}
	position := max(0, req.PositionSeconds)
	state := group.State
	switch action {
	case "play", "load", "next", "previous":
		state = "playing"
	case "pause":
		state = "paused"
	case "stop":
		state = "stopped"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if action == "next" || action == "previous" {
		position = 0
	}
	playbackRate := req.PlaybackRate
	if playbackRate <= 0 || playbackRate > 2 {
		playbackRate = group.PlaybackRate
	}
	if playbackRate <= 0 {
		playbackRate = 1
	}
	command := PlaybackCommand{ID: randomID("pcmd"), Action: commandAction, MediaID: mediaID, PositionSeconds: position, IssuedByUserID: user.ID, IssuedByProfileID: viewerProfileID(user), IssuedAt: now}
	commandJSON, _ := json.Marshal(command)
	endedAt := ""
	if action == "stop" {
		endedAt = now
	}
	err = s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE watch_with_friends_groups
			SET media_id = ?, current_entry_id = ?, media_title = ?, state = ?, position_seconds = ?, position_updated_at = ?, playback_rate = ?,
				command_json = ?, updated_at = ?, revision = revision + 1, playback_revision = playback_revision + 1,
				reconnect_generation = reconnect_generation + CASE WHEN ? <> '' THEN 1 ELSE 0 END,
				last_idempotency_key = ?, ended_at = CASE WHEN ? <> '' THEN ? ELSE ended_at END
			WHERE id = ? AND ended_at = '' AND revision = ?`,
			mediaID, currentEntryID, mediaTitle, state, position, now, playbackRate, string(commandJSON), now, endedAt, idempotencyKey, endedAt, endedAt, groupID, group.Revision)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return errWatchWithFriendsRevisionConflict
		}
		if createOccurrence {
			var nextOrder int
			_ = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), -1) + 1 FROM watch_with_friends_queue WHERE group_id = ?`, groupID).Scan(&nextOrder)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO watch_with_friends_queue (group_id, entry_id, media_id, media_title, sort_order, added_by_user_id, added_by_profile_id, added_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				groupID, currentEntryID, mediaID, mediaTitle, nextOrder, accountIDForUser(user), viewerProfileID(user), now); err != nil {
				return err
			}
		}
		return insertWatchWithFriendsMutationReceiptContext(ctx, tx, groupID, idempotencyKey, fingerprint, group.Revision+1, group.PlaybackRevision+1, now)
	})
	if err != nil {
		if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
			return receiptGroup, receiptErr
		}
		return WatchWithFriendsGroup{}, err
	}
	if action == "stop" {
		return s.watchWithFriendsGroupSnapshotForUserContext(ctx, user, groupID, true)
	}
	return s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
}

func watchWithFriendsStateRequestFingerprint(req WatchWithFriendsStateRequest) string {
	payload := struct {
		Operation        string  `json:"operation"`
		Action           string  `json:"action"`
		EntryID          string  `json:"entryId"`
		MediaID          string  `json:"mediaId"`
		PositionSeconds  int     `json:"positionSeconds"`
		PlaybackRate     float64 `json:"playbackRate"`
		ExpectedRevision int64   `json:"expectedRevision"`
	}{
		Operation:        "state",
		Action:           strings.ToLower(strings.TrimSpace(req.Action)),
		EntryID:          strings.TrimSpace(req.EntryID),
		MediaID:          strings.TrimSpace(req.MediaID),
		PositionSeconds:  max(0, req.PositionSeconds),
		PlaybackRate:     req.PlaybackRate,
		ExpectedRevision: *req.ExpectedRevision,
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func watchWithFriendsMutationRequestFingerprint(operation string, request any) string {
	payload := struct {
		Operation string `json:"operation"`
		Request   any    `json:"request"`
	}{Operation: operation, Request: request}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func insertWatchWithFriendsMutationReceiptContext(ctx context.Context, tx *sql.Tx, groupID, idempotencyKey, fingerprint string, responseRevision, responsePlaybackRevision int64, createdAt string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO watch_with_friends_command_receipts (
			group_id, idempotency_key, request_fingerprint, response_revision, response_playback_revision, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		groupID, idempotencyKey, fingerprint, responseRevision, responsePlaybackRevision, createdAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		DELETE FROM watch_with_friends_command_receipts
		WHERE group_id = ? AND rowid NOT IN (
			SELECT rowid
			FROM watch_with_friends_command_receipts
			WHERE group_id = ?
			ORDER BY created_at DESC, rowid DESC
			LIMIT ?
		)`, groupID, groupID, maxWatchWithFriendsCommandReceipts)
	return err
}

func (s *Server) watchWithFriendsMutationReceiptContext(ctx context.Context, user User, groupID, idempotencyKey, fingerprint string) (WatchWithFriendsGroup, bool, error) {
	var storedFingerprint string
	err := s.queryUserRow(ctx, `
		SELECT request_fingerprint
		FROM watch_with_friends_command_receipts
		WHERE group_id = ? AND idempotency_key = ?`, strings.TrimSpace(groupID), strings.TrimSpace(idempotencyKey)).Scan(&storedFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return WatchWithFriendsGroup{}, false, nil
	}
	if err != nil {
		return WatchWithFriendsGroup{}, false, err
	}
	if storedFingerprint != fingerprint {
		return WatchWithFriendsGroup{}, true, errWatchWithFriendsInvalidRequest
	}
	// Receipt revisions are audit evidence for the atomic commit. A replay
	// intentionally returns the current materialized snapshot so a delayed
	// response cannot roll a client back past commands committed afterward.
	group, err := s.watchWithFriendsGroupSnapshotForUserContext(ctx, user, groupID, true)
	if err != nil {
		return WatchWithFriendsGroup{}, true, err
	}
	if group.OwnerProfileID != viewerProfileID(user) {
		return WatchWithFriendsGroup{}, true, errWatchWithFriendsHostRequired
	}
	return group, true, nil
}

func (s *Server) updateWatchWithFriendsMemberState(user User, groupID string, req WatchWithFriendsMemberStateRequest) (WatchWithFriendsGroup, error) {
	return s.updateWatchWithFriendsMemberStateContext(context.Background(), user, groupID, req)
}

func (s *Server) updateWatchWithFriendsMemberStateContext(ctx context.Context, user User, groupID string, req WatchWithFriendsMemberStateRequest) (WatchWithFriendsGroup, error) {
	if _, err := s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true); err != nil {
		return WatchWithFriendsGroup{}, err
	}
	state := strings.ToLower(strings.TrimSpace(req.State))
	switch state {
	case "joined", "ready", "buffering", "playing", "paused":
	default:
		return WatchWithFriendsGroup{}, errors.New("Watch With Friends member state is not supported.")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE watch_with_friends_members
			SET state = ?, position_seconds = ?, last_seen_at = ?
			WHERE group_id = ? AND profile_id = ?`,
			state, max(0, req.PositionSeconds), now, groupID, viewerProfileID(user))
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	return s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
}

func (s *Server) updateWatchWithFriendsSettings(user User, groupID string, req WatchWithFriendsSettingsRequest) (WatchWithFriendsGroup, error) {
	return s.updateWatchWithFriendsSettingsContext(context.Background(), user, groupID, req)
}

func (s *Server) updateWatchWithFriendsSettingsContext(ctx context.Context, user User, groupID string, req WatchWithFriendsSettingsRequest) (WatchWithFriendsGroup, error) {
	return s.updateWatchWithFriendsSettingsExpectedContext(ctx, user, groupID, req, req.ExpectedRevision)
}

func (s *Server) updateWatchWithFriendsSettingsExpectedContext(ctx context.Context, user User, groupID string, req WatchWithFriendsSettingsRequest, expectedRevision *int64) (WatchWithFriendsGroup, error) {
	idempotencyKey, err := watchWithFriendsRequiredIdempotencyKey(req.IdempotencyKey)
	if err != nil || expectedRevision == nil || *expectedRevision < 0 {
		return WatchWithFriendsGroup{}, errWatchWithFriendsInvalidRequest
	}
	repeatMode := strings.ToLower(strings.TrimSpace(req.RepeatMode))
	switch repeatMode {
	case "", "none", "one", "all":
	default:
		return WatchWithFriendsGroup{}, errors.New("Watch With Friends repeat mode is not supported.")
	}
	if req.ShuffleEnabled == nil && repeatMode == "" {
		return WatchWithFriendsGroup{}, errWatchWithFriendsInvalidRequest
	}
	fingerprint := watchWithFriendsMutationRequestFingerprint("settings", struct {
		ShuffleEnabled   *bool  `json:"shuffleEnabled,omitempty"`
		RepeatMode       string `json:"repeatMode,omitempty"`
		ExpectedRevision int64  `json:"expectedRevision"`
	}{req.ShuffleEnabled, repeatMode, *expectedRevision})
	if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
		return receiptGroup, receiptErr
	}
	group, err := s.watchWithFriendsGroupForHostContext(ctx, user, groupID)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	if *expectedRevision != group.Revision {
		return WatchWithFriendsGroup{}, errWatchWithFriendsRevisionConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		var result sql.Result
		var updateErr error
		if req.ShuffleEnabled != nil && repeatMode != "" {
			result, updateErr = tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET shuffle_enabled = ?, repeat_mode = ?, last_idempotency_key = ?, updated_at = ?, revision = revision + 1 WHERE id = ? AND ended_at = '' AND revision = ?`, boolToInt(*req.ShuffleEnabled), repeatMode, idempotencyKey, now, groupID, group.Revision)
		} else if req.ShuffleEnabled != nil {
			result, updateErr = tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET shuffle_enabled = ?, last_idempotency_key = ?, updated_at = ?, revision = revision + 1 WHERE id = ? AND ended_at = '' AND revision = ?`, boolToInt(*req.ShuffleEnabled), idempotencyKey, now, groupID, group.Revision)
		} else {
			result, updateErr = tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET repeat_mode = ?, last_idempotency_key = ?, updated_at = ?, revision = revision + 1 WHERE id = ? AND ended_at = '' AND revision = ?`, repeatMode, idempotencyKey, now, groupID, group.Revision)
		}
		if updateErr != nil {
			return updateErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return errWatchWithFriendsRevisionConflict
		}
		return insertWatchWithFriendsMutationReceiptContext(ctx, tx, groupID, idempotencyKey, fingerprint, group.Revision+1, group.PlaybackRevision, now)
	})
	if err != nil {
		if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
			return receiptGroup, receiptErr
		}
		return WatchWithFriendsGroup{}, err
	}
	return s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
}

func (s *Server) addWatchWithFriendsQueueItem(user User, groupID string, req WatchWithFriendsQueueRequest) (WatchWithFriendsGroup, error) {
	return s.addWatchWithFriendsQueueItemContext(context.Background(), user, groupID, req)
}

func (s *Server) addWatchWithFriendsQueueItemContext(ctx context.Context, user User, groupID string, req WatchWithFriendsQueueRequest) (WatchWithFriendsGroup, error) {
	return s.addWatchWithFriendsQueueItemExpectedContext(ctx, user, groupID, req, req.ExpectedRevision)
}

func (s *Server) addWatchWithFriendsQueueItemExpectedContext(ctx context.Context, user User, groupID string, req WatchWithFriendsQueueRequest, expectedRevision *int64) (WatchWithFriendsGroup, error) {
	idempotencyKey, err := watchWithFriendsRequiredIdempotencyKey(req.IdempotencyKey)
	if err != nil || expectedRevision == nil || *expectedRevision < 0 {
		return WatchWithFriendsGroup{}, errWatchWithFriendsInvalidRequest
	}
	mediaID := strings.TrimSpace(req.MediaID)
	if mediaID == "" {
		return WatchWithFriendsGroup{}, errors.New("Queue items require a media item.")
	}
	fingerprint := watchWithFriendsMutationRequestFingerprint("queue_add", struct {
		MediaID          string `json:"mediaId"`
		ExpectedRevision int64  `json:"expectedRevision"`
	}{mediaID, *expectedRevision})
	if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
		return receiptGroup, receiptErr
	}
	group, err := s.watchWithFriendsGroupForHostContext(ctx, user, groupID)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	if *expectedRevision != group.Revision {
		return WatchWithFriendsGroup{}, errWatchWithFriendsRevisionConflict
	}
	item, err := s.getMediaListItemContext(ctx, viewerProfileID(user), mediaID)
	if err != nil {
		return WatchWithFriendsGroup{}, errors.New("Queue media item is not accessible.")
	}
	if !s.watchWithFriendsMediaVisibleToAllMembersContext(ctx, groupID, mediaID) {
		return WatchWithFriendsGroup{}, errors.New("Everyone in this Watch With Friends group must be able to access a queued item.")
	}
	entryID := randomID("wfentry")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		var queueCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_with_friends_queue WHERE group_id = ?`, groupID).Scan(&queueCount); err != nil {
			return err
		}
		if queueCount >= maxPlaybackQueueItems {
			return fmt.Errorf("Watch With Friends queues are limited to %d media items.", maxPlaybackQueueItems)
		}
		var nextOrder int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), -1) + 1 FROM watch_with_friends_queue WHERE group_id = ?`, groupID).Scan(&nextOrder); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO watch_with_friends_queue (group_id, entry_id, media_id, media_title, sort_order, added_by_user_id, added_by_profile_id, added_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			groupID, entryID, item.ID, item.Title, nextOrder, accountIDForUser(user), viewerProfileID(user), now); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET last_idempotency_key = ?, updated_at = ?, revision = revision + 1 WHERE id = ? AND ended_at = '' AND revision = ?`, idempotencyKey, now, groupID, group.Revision)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return errWatchWithFriendsRevisionConflict
		}
		return insertWatchWithFriendsMutationReceiptContext(ctx, tx, groupID, idempotencyKey, fingerprint, group.Revision+1, group.PlaybackRevision, now)
	})
	if err != nil {
		if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
			return receiptGroup, receiptErr
		}
		return WatchWithFriendsGroup{}, err
	}
	return s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
}

func (s *Server) removeWatchWithFriendsQueueItem(user User, groupID string, entryID string) (WatchWithFriendsGroup, error) {
	return s.removeWatchWithFriendsQueueItemContext(context.Background(), user, groupID, entryID)
}

func (s *Server) removeWatchWithFriendsQueueItemContext(ctx context.Context, user User, groupID string, entryID string) (WatchWithFriendsGroup, error) {
	return s.removeWatchWithFriendsQueueItemExpectedContext(ctx, user, groupID, entryID, nil, "")
}

func (s *Server) removeWatchWithFriendsQueueItemExpectedContext(ctx context.Context, user User, groupID string, entryID string, expectedRevision *int64, rawIdempotencyKey string) (WatchWithFriendsGroup, error) {
	idempotencyKey, err := watchWithFriendsRequiredIdempotencyKey(rawIdempotencyKey)
	if err != nil || expectedRevision == nil || *expectedRevision < 0 {
		return WatchWithFriendsGroup{}, errWatchWithFriendsInvalidRequest
	}
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return WatchWithFriendsGroup{}, errWatchWithFriendsInvalidRequest
	}
	fingerprint := watchWithFriendsMutationRequestFingerprint("queue_remove", struct {
		EntryID          string `json:"entryId"`
		ExpectedRevision int64  `json:"expectedRevision"`
	}{entryID, *expectedRevision})
	if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
		return receiptGroup, receiptErr
	}
	group, err := s.watchWithFriendsGroupForHostContext(ctx, user, groupID)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	if *expectedRevision != group.Revision {
		return WatchWithFriendsGroup{}, errWatchWithFriendsRevisionConflict
	}
	if entryID == group.CurrentEntryID {
		return WatchWithFriendsGroup{}, errors.New("The currently loaded item cannot be removed from the Watch With Friends queue.")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM watch_with_friends_queue WHERE group_id = ? AND entry_id = ?`, groupID, entryID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return sql.ErrNoRows
		}
		result, err = tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET last_idempotency_key = ?, updated_at = ?, revision = revision + 1 WHERE id = ? AND ended_at = '' AND revision = ?`, idempotencyKey, now, groupID, group.Revision)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return errWatchWithFriendsRevisionConflict
		}
		return insertWatchWithFriendsMutationReceiptContext(ctx, tx, groupID, idempotencyKey, fingerprint, group.Revision+1, group.PlaybackRevision, now)
	})
	if err != nil {
		if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
			return receiptGroup, receiptErr
		}
		return WatchWithFriendsGroup{}, err
	}
	return s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
}

func (s *Server) reorderWatchWithFriendsQueue(user User, groupID string, req WatchWithFriendsQueueOrderRequest) (WatchWithFriendsGroup, error) {
	return s.reorderWatchWithFriendsQueueContext(context.Background(), user, groupID, req)
}

func (s *Server) reorderWatchWithFriendsQueueContext(ctx context.Context, user User, groupID string, req WatchWithFriendsQueueOrderRequest) (WatchWithFriendsGroup, error) {
	return s.reorderWatchWithFriendsQueueExpectedContext(ctx, user, groupID, req, req.ExpectedRevision)
}

func (s *Server) reorderWatchWithFriendsQueueExpectedContext(ctx context.Context, user User, groupID string, req WatchWithFriendsQueueOrderRequest, expectedRevision *int64) (WatchWithFriendsGroup, error) {
	idempotencyKey, err := watchWithFriendsRequiredIdempotencyKey(req.IdempotencyKey)
	if err != nil || expectedRevision == nil || *expectedRevision < 0 {
		return WatchWithFriendsGroup{}, errWatchWithFriendsInvalidRequest
	}
	entryID := strings.TrimSpace(req.EntryID)
	destinationEntryID := strings.TrimSpace(req.DestinationEntryID)
	placement := strings.ToLower(strings.TrimSpace(req.Placement))
	if entryID == "" || destinationEntryID == "" || entryID == destinationEntryID || (placement != "before" && placement != "after") {
		return WatchWithFriendsGroup{}, errors.New("Queue reorder requires distinct entryId and destinationEntryId values plus before or after placement.")
	}
	fingerprint := watchWithFriendsMutationRequestFingerprint("queue_reorder", struct {
		EntryID            string `json:"entryId"`
		DestinationEntryID string `json:"destinationEntryId"`
		Placement          string `json:"placement"`
		ExpectedRevision   int64  `json:"expectedRevision"`
	}{entryID, destinationEntryID, placement, *expectedRevision})
	if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
		return receiptGroup, receiptErr
	}
	group, err := s.watchWithFriendsGroupForHostContext(ctx, user, groupID)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	if *expectedRevision != group.Revision {
		return WatchWithFriendsGroup{}, errWatchWithFriendsRevisionConflict
	}
	from := watchWithFriendsQueueEntryIndex(group.Queue, entryID)
	if from < 0 {
		return WatchWithFriendsGroup{}, errors.New("entryId is not in the current queue.")
	}
	ordered := append([]WatchWithFriendsQueueItem(nil), group.Queue...)
	moved := ordered[from]
	ordered = append(ordered[:from], ordered[from+1:]...)
	destination := watchWithFriendsQueueEntryIndex(ordered, destinationEntryID)
	if destination < 0 {
		return WatchWithFriendsGroup{}, errors.New("destinationEntryId is not in the current queue.")
	}
	if placement == "after" {
		destination++
	}
	ordered = append(ordered, WatchWithFriendsQueueItem{})
	copy(ordered[destination+1:], ordered[destination:])
	ordered[destination] = moved
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		for index, item := range ordered {
			if _, err := tx.ExecContext(ctx, `UPDATE watch_with_friends_queue SET sort_order = ? WHERE group_id = ? AND entry_id = ?`, index, groupID, item.EntryID); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET last_idempotency_key = ?, updated_at = ?, revision = revision + 1 WHERE id = ? AND ended_at = '' AND revision = ?`, idempotencyKey, now, groupID, group.Revision)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return errWatchWithFriendsRevisionConflict
		}
		return insertWatchWithFriendsMutationReceiptContext(ctx, tx, groupID, idempotencyKey, fingerprint, group.Revision+1, group.PlaybackRevision, now)
	})
	if err != nil {
		if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
			return receiptGroup, receiptErr
		}
		return WatchWithFriendsGroup{}, err
	}
	return s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
}

func (s *Server) endWatchWithFriendsGroup(user User, groupID string) (WatchWithFriendsGroup, error) {
	return s.endWatchWithFriendsGroupContext(context.Background(), user, groupID)
}

func (s *Server) endWatchWithFriendsGroupContext(ctx context.Context, user User, groupID string) (WatchWithFriendsGroup, error) {
	group, err := s.watchWithFriendsGroupForHostContext(ctx, user, groupID)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	return s.endWatchWithFriendsGroupWithoutReceiptContext(ctx, user, group)
}

func (s *Server) endWatchWithFriendsGroupExpectedContext(ctx context.Context, user User, groupID string, expectedRevision *int64, rawIdempotencyKey string) (WatchWithFriendsGroup, error) {
	idempotencyKey, err := watchWithFriendsRequiredIdempotencyKey(rawIdempotencyKey)
	if err != nil || expectedRevision == nil || *expectedRevision < 0 {
		return WatchWithFriendsGroup{}, errWatchWithFriendsInvalidRequest
	}
	fingerprint := watchWithFriendsMutationRequestFingerprint("end", struct {
		ExpectedRevision int64 `json:"expectedRevision"`
	}{*expectedRevision})
	if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
		return receiptGroup, receiptErr
	}
	group, err := s.watchWithFriendsGroupForHostContext(ctx, user, groupID)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	if *expectedRevision != group.Revision {
		return WatchWithFriendsGroup{}, errWatchWithFriendsRevisionConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		result, updateErr := tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET state = 'stopped', ended_at = ?, last_idempotency_key = ?, updated_at = ?, position_updated_at = ?, revision = revision + 1, playback_revision = playback_revision + 1, reconnect_generation = reconnect_generation + 1 WHERE id = ? AND ended_at = '' AND revision = ?`, now, idempotencyKey, now, now, groupID, group.Revision)
		if updateErr != nil {
			return updateErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return errWatchWithFriendsRevisionConflict
		}
		return insertWatchWithFriendsMutationReceiptContext(ctx, tx, groupID, idempotencyKey, fingerprint, group.Revision+1, group.PlaybackRevision+1, now)
	})
	if err != nil {
		if receiptGroup, found, receiptErr := s.watchWithFriendsMutationReceiptContext(ctx, user, groupID, idempotencyKey, fingerprint); found || receiptErr != nil {
			return receiptGroup, receiptErr
		}
		return WatchWithFriendsGroup{}, err
	}
	return s.watchWithFriendsGroupSnapshotForUserContext(ctx, user, groupID, true)
}

func (s *Server) endWatchWithFriendsGroupWithoutReceiptContext(ctx context.Context, user User, group WatchWithFriendsGroup) (WatchWithFriendsGroup, error) {
	if group.OwnerProfileID != viewerProfileID(user) {
		return WatchWithFriendsGroup{}, errWatchWithFriendsHostRequired
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if !watchWithFriendsPermissionAllowedTx(tx, user) {
			return errWatchWithFriendsPermissionRequired
		}
		result, updateErr := tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET state = 'stopped', ended_at = ?, updated_at = ?, position_updated_at = ?, revision = revision + 1, playback_revision = playback_revision + 1, reconnect_generation = reconnect_generation + 1 WHERE id = ? AND ended_at = '' AND revision = ?`, now, now, now, group.ID, group.Revision)
		if updateErr != nil {
			return updateErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return errWatchWithFriendsRevisionConflict
		}
		return nil
	})
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	return s.watchWithFriendsGroupSnapshotForUserContext(ctx, user, group.ID, true)
}

func (s *Server) watchWithFriendsGroupForUser(user User, groupID string, requireMember bool) (WatchWithFriendsGroup, error) {
	return s.watchWithFriendsGroupForUserContext(context.Background(), user, groupID, requireMember)
}

func (s *Server) watchWithFriendsGroupForHostContext(ctx context.Context, user User, groupID string) (WatchWithFriendsGroup, error) {
	group, err := s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	if group.OwnerProfileID != viewerProfileID(user) {
		return WatchWithFriendsGroup{}, errWatchWithFriendsHostRequired
	}
	return group, nil
}

func (s *Server) watchWithFriendsGroupForUserContext(ctx context.Context, user User, groupID string, requireMember bool) (WatchWithFriendsGroup, error) {
	group, err := s.watchWithFriendsGroupContext(ctx, groupID)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	profileID := viewerProfileID(user)
	permissionAllowed := s.watchWithFriendsProfilePermissionAllowedContext(ctx, profileID)
	memberBeforeReconcile := watchWithFriendsGroupHasMember(group, profileID)
	if permissionAllowed && memberBeforeReconcile {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, _ = s.execUserWriteTagged(ctx, []string{}, `UPDATE watch_with_friends_members SET last_seen_at = ? WHERE group_id = ? AND profile_id = ?`, now, group.ID, profileID)
		for index := range group.Members {
			if group.Members[index].ProfileID == profileID {
				group.Members[index].LastSeenAt = now
			}
		}
	}
	changed, ended, err := s.reconcileWatchWithFriendsGroupContext(ctx, group)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	if ended || !permissionAllowed {
		return WatchWithFriendsGroup{}, sql.ErrNoRows
	}
	if changed {
		group, err = s.watchWithFriendsGroupContext(ctx, groupID)
		if err != nil {
			return WatchWithFriendsGroup{}, err
		}
	}
	if _, err := s.getMediaAccessSummaryContext(ctx, viewerProfileID(user), group.MediaID); err != nil {
		return WatchWithFriendsGroup{}, sql.ErrNoRows
	}
	isMember := watchWithFriendsGroupHasMember(group, profileID)
	if requireMember && !isMember {
		return WatchWithFriendsGroup{}, sql.ErrNoRows
	}
	if !isMember {
		if !s.userPrivacyPreferencesForProfileContext(ctx, viewerProfileID(user)).IncludeInWatchWithFriends {
			return WatchWithFriendsGroup{}, sql.ErrNoRows
		}
		if !s.userPrivacyPreferencesForProfileContext(ctx, group.OwnerProfileID).IncludeInWatchWithFriends {
			return WatchWithFriendsGroup{}, sql.ErrNoRows
		}
	}
	return s.decorateWatchWithFriendsGroupForUserContext(ctx, user, group), nil
}

func (s *Server) watchWithFriendsGroupSnapshotForUserContext(ctx context.Context, user User, groupID string, includeEnded bool) (WatchWithFriendsGroup, error) {
	group, err := s.watchWithFriendsGroupSnapshotContext(ctx, groupID, includeEnded)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	if !s.watchWithFriendsProfilePermissionAllowedContext(ctx, viewerProfileID(user)) || !watchWithFriendsGroupHasMember(group, viewerProfileID(user)) {
		return WatchWithFriendsGroup{}, sql.ErrNoRows
	}
	if _, err := s.getMediaAccessSummaryContext(ctx, viewerProfileID(user), group.MediaID); err != nil {
		return WatchWithFriendsGroup{}, sql.ErrNoRows
	}
	return s.decorateWatchWithFriendsGroupForUserContext(ctx, user, group), nil
}

func (s *Server) decorateWatchWithFriendsGroupForUserContext(ctx context.Context, user User, group WatchWithFriendsGroup) WatchWithFriendsGroup {
	group.Members = s.visibleWatchWithFriendsMembersContext(ctx, user, group)
	group.Permissions = WatchWithFriendsPermissions{
		IsHost:         group.OwnerProfileID == viewerProfileID(user),
		CanControl:     group.OwnerProfileID == viewerProfileID(user),
		CanManageQueue: group.OwnerProfileID == viewerProfileID(user),
	}
	visibleQueue := make([]WatchWithFriendsQueueItem, 0, len(group.Queue))
	for _, item := range group.Queue {
		if _, err := s.getMediaAccessSummaryContext(ctx, viewerProfileID(user), item.MediaID); err != nil {
			item.MediaID = ""
			item.MediaTitle = "Unavailable"
			item.Unavailable = true
		}
		visibleQueue = append(visibleQueue, item)
	}
	group.Queue = visibleQueue
	return group
}

func watchWithFriendsGroupHasMember(group WatchWithFriendsGroup, profileID string) bool {
	for _, member := range group.Members {
		if member.ProfileID == profileID {
			return true
		}
	}
	return false
}

func (s *Server) watchWithFriendsProfilePermissionAllowedContext(ctx context.Context, profileID string) bool {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return false
	}
	var accountID string
	if err := s.queryUserRow(ctx, `SELECT account_id FROM profiles WHERE id = ?`, profileID).Scan(&accountID); err != nil {
		return false
	}
	// resolveRequestPrincipalContext is the single authoritative policy
	// intersection for membership permissions and typed profile restrictions.
	// In particular, an administrative account never bypasses a child profile's
	// allowWatchWithFriends=false restriction.
	principal, err := s.resolveRequestPrincipalContext(ctx, accountID, profileID)
	return err == nil && principal.Permissions["watchWithFriends"]
}

func watchWithFriendsPermissionAllowedTx(tx *sql.Tx, user User) bool {
	if tx == nil {
		return false
	}
	principal, err := resolveRequestPrincipalTx(tx, accountIDForUser(user), viewerProfileID(user))
	return err == nil && principal.Permissions["watchWithFriends"]
}

func (s *Server) reconcileWatchWithFriendsGroupContext(ctx context.Context, group WatchWithFriendsGroup) (changed bool, ended bool, err error) {
	cutoff := time.Now().UTC().Add(-watchWithFriendsMemberStaleAfter)
	invalidMembers := make([]WatchWithFriendsMember, 0)
	hostValid := false
	hostLastSeenAt := ""
	for _, member := range group.Members {
		lastSeen, parseErr := time.Parse(time.RFC3339Nano, member.LastSeenAt)
		stale := parseErr != nil || lastSeen.Before(cutoff)
		_, currentAccessErr := s.getMediaAccessSummaryContext(ctx, member.ProfileID, group.MediaID)
		valid := !stale && s.watchWithFriendsProfilePermissionAllowedContext(ctx, member.ProfileID) && currentAccessErr == nil
		if member.ProfileID == group.OwnerProfileID {
			hostValid = valid
			hostLastSeenAt = member.LastSeenAt
		}
		if !valid {
			invalidMembers = append(invalidMembers, member)
		}
	}
	if !hostValid {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, writeErr := s.execUserWriteTagged(ctx, []string{}, `
			UPDATE watch_with_friends_groups
			SET state = 'stopped', ended_at = ?, updated_at = ?, position_updated_at = ?,
				revision = revision + 1, playback_revision = playback_revision + 1,
				reconnect_generation = reconnect_generation + 1
			WHERE id = ? AND ended_at = '' AND revision = ?
				AND NOT EXISTS (
					SELECT 1 FROM watch_with_friends_members
					WHERE group_id = ? AND profile_id = ? AND last_seen_at <> ?
				)`, now, now, now, group.ID, group.Revision, group.ID, group.OwnerProfileID, hostLastSeenAt)
		if writeErr != nil {
			return false, false, writeErr
		}
		count, _ := result.RowsAffected()
		return count == 1, count == 1, nil
	}
	if len(invalidMembers) == 0 {
		return false, false, nil
	}
	err = s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		removed := int64(0)
		for _, member := range invalidMembers {
			result, deleteErr := tx.ExecContext(ctx, `DELETE FROM watch_with_friends_members WHERE group_id = ? AND profile_id = ? AND last_seen_at = ?`, group.ID, member.ProfileID, member.LastSeenAt)
			if deleteErr != nil {
				return deleteErr
			}
			count, _ := result.RowsAffected()
			removed += count
		}
		if removed == 0 {
			return nil
		}
		result, updateErr := tx.ExecContext(ctx, `UPDATE watch_with_friends_groups SET revision = revision + 1, reconnect_generation = reconnect_generation + 1, updated_at = ? WHERE id = ? AND ended_at = '' AND revision = ?`, time.Now().UTC().Format(time.RFC3339Nano), group.ID, group.Revision)
		if updateErr != nil {
			return updateErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return errWatchWithFriendsRevisionConflict
		}
		changed = true
		return nil
	})
	return changed, false, err
}

func (s *Server) watchWithFriendsGroup(groupID string) (WatchWithFriendsGroup, error) {
	return s.watchWithFriendsGroupContext(context.Background(), groupID)
}

func (s *Server) watchWithFriendsGroupContext(ctx context.Context, groupID string) (WatchWithFriendsGroup, error) {
	return s.watchWithFriendsGroupSnapshotContext(ctx, groupID, false)
}

func (s *Server) watchWithFriendsGroupSnapshotContext(ctx context.Context, groupID string, includeEnded bool) (WatchWithFriendsGroup, error) {
	var group WatchWithFriendsGroup
	var commandJSON string
	var shuffleEnabled int
	where := "g.id = ? AND g.ended_at = ''"
	if includeEnded {
		where = "g.id = ?"
	}
	err := s.queryUserRow(ctx, `
		SELECT g.id, g.name, g.owner_user_id, g.owner_profile_id, p.display_name, g.media_id, g.current_entry_id, g.media_title, g.state, g.position_seconds,
			g.position_updated_at, g.playback_rate, g.revision, g.playback_revision, g.reconnect_generation, g.last_idempotency_key,
			g.shuffle_enabled, g.repeat_mode, g.command_json, g.created_at, g.updated_at
		FROM watch_with_friends_groups g
		JOIN profiles p ON p.id = g.owner_profile_id
		WHERE `+where, strings.TrimSpace(groupID)).
		Scan(&group.ID, &group.Name, &group.OwnerUserID, &group.OwnerProfileID, &group.OwnerName, &group.MediaID, &group.CurrentEntryID, &group.MediaTitle, &group.State, &group.PositionSeconds,
			&group.PositionUpdatedAt, &group.PlaybackRate, &group.Revision, &group.PlaybackRevision, &group.ReconnectGeneration, &group.LastIdempotencyKey,
			&shuffleEnabled, &group.RepeatMode, &commandJSON, &group.CreatedAt, &group.UpdatedAt)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	group.ShuffleEnabled = shuffleEnabled != 0
	group.ServerTime = time.Now().UTC().Format(time.RFC3339Nano)
	group.Command = decodePlaybackCommand(commandJSON)
	members, err := s.watchWithFriendsMembersContext(ctx, group.ID)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	group.Members = members
	queue, err := s.watchWithFriendsQueueContext(ctx, group.ID)
	if err != nil {
		return WatchWithFriendsGroup{}, err
	}
	group.Queue = queue
	return group, nil
}

func (s *Server) watchWithFriendsQueue(groupID string) ([]WatchWithFriendsQueueItem, error) {
	return s.watchWithFriendsQueueContext(context.Background(), groupID)
}

func (s *Server) watchWithFriendsQueueContext(ctx context.Context, groupID string) ([]WatchWithFriendsQueueItem, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT entry_id, media_id, media_title, sort_order, added_by_user_id, added_by_profile_id, added_at
		FROM watch_with_friends_queue
		WHERE group_id = ?
		ORDER BY sort_order ASC, entry_id ASC
		LIMIT ?`, groupID, maxPlaybackQueueItems)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	queue := []WatchWithFriendsQueueItem{}
	for rows.Next() {
		var item WatchWithFriendsQueueItem
		if err := rows.Scan(&item.EntryID, &item.MediaID, &item.MediaTitle, &item.SortOrder, &item.AddedByUserID, &item.AddedByProfileID, &item.AddedAt); err != nil {
			return nil, err
		}
		queue = append(queue, item)
	}
	return queue, rows.Err()
}

func watchWithFriendsAdjacentQueueItem(group WatchWithFriendsGroup, direction string) (WatchWithFriendsQueueItem, error) {
	if len(group.Queue) == 0 {
		return WatchWithFriendsQueueItem{}, errors.New("Watch With Friends queue is empty.")
	}
	currentIndex := -1
	for index, item := range group.Queue {
		if item.EntryID == group.CurrentEntryID {
			currentIndex = index
			break
		}
	}
	if currentIndex < 0 {
		return WatchWithFriendsQueueItem{}, errors.New("The current item is not in the Watch With Friends queue.")
	}
	if group.RepeatMode == "one" {
		return group.Queue[currentIndex], nil
	}
	if group.ShuffleEnabled && len(group.Queue) > 1 {
		candidates := make([]WatchWithFriendsQueueItem, 0, len(group.Queue)-1)
		for index, item := range group.Queue {
			if index != currentIndex {
				candidates = append(candidates, item)
			}
		}
		return candidates[int(time.Now().UnixNano()%int64(len(candidates)))], nil
	}
	nextIndex := currentIndex + 1
	if direction == "previous" {
		nextIndex = currentIndex - 1
	}
	if group.RepeatMode == "all" {
		if nextIndex < 0 {
			nextIndex = len(group.Queue) - 1
		}
		if nextIndex >= len(group.Queue) {
			nextIndex = 0
		}
	}
	if nextIndex < 0 || nextIndex >= len(group.Queue) {
		return WatchWithFriendsQueueItem{}, errors.New("No adjacent Watch With Friends queue item is available.")
	}
	return group.Queue[nextIndex], nil
}

func watchWithFriendsQueueEntryIndex(queue []WatchWithFriendsQueueItem, rawEntryID string) int {
	entryID := strings.TrimSpace(rawEntryID)
	for index, item := range queue {
		if item.EntryID == entryID {
			return index
		}
	}
	return -1
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Server) watchWithFriendsMembers(groupID string) ([]WatchWithFriendsMember, error) {
	return s.watchWithFriendsMembersContext(context.Background(), groupID)
}

func (s *Server) watchWithFriendsMembersContext(ctx context.Context, groupID string) ([]WatchWithFriendsMember, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT m.user_id, m.profile_id, p.display_name, m.state, m.position_seconds, m.joined_at, m.last_seen_at
		FROM watch_with_friends_members m
		JOIN profiles p ON p.id = m.profile_id
		WHERE m.group_id = ?
		ORDER BY m.joined_at ASC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []WatchWithFriendsMember{}
	for rows.Next() {
		var member WatchWithFriendsMember
		if err := rows.Scan(&member.UserID, &member.ProfileID, &member.DisplayName, &member.State, &member.PositionSeconds, &member.JoinedAt, &member.LastSeenAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (s *Server) visibleWatchWithFriendsMembersContext(ctx context.Context, viewer User, group WatchWithFriendsGroup) []WatchWithFriendsMember {
	visible := make([]WatchWithFriendsMember, 0, len(group.Members))
	for _, member := range group.Members {
		if member.ProfileID == viewerProfileID(viewer) || s.userPrivacyPreferencesForProfileContext(ctx, member.ProfileID).ShowActivityToMembers {
			visible = append(visible, member)
		}
	}
	return visible
}

func (s *Server) watchWithFriendsMediaVisibleToAllMembersContext(ctx context.Context, groupID, mediaID string) bool {
	rows, err := s.queryUserRead(ctx, `SELECT profile_id FROM watch_with_friends_members WHERE group_id = ?`, groupID)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var profileID string
		if rows.Scan(&profileID) != nil {
			return false
		}
		if !s.watchWithFriendsProfilePermissionAllowedContext(ctx, profileID) {
			return false
		}
		if _, err := s.getMediaAccessSummaryContext(ctx, profileID, mediaID); err != nil {
			return false
		}
	}
	return rows.Err() == nil
}
