package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	viewerFeedbackVersion            = "v1"
	defaultNotificationRetentionDays = 180
	defaultFeedbackRetentionDays     = 365
	maximumViewerNotificationPage    = 100
	maximumViewerFeedbackPage        = 100
)

// Notification mutations wake the exact recipient through the long-poll
// broker. The timer is therefore only a revocation/expiry safety net instead
// of a database-backed notification poll.
var viewerNotificationStreamAuthorizationInterval = 15 * time.Second
var viewerNotificationStreamCandidateSelectedHook func()

var feedbackIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]*$`)
var errFeedbackRevisionConflict = errors.New("feedback revision conflict")
var errFeedbackRateLimited = errors.New("feedback rate limited")

type notificationRecipient struct {
	Authority string `json:"authority"`
	AccountID string `json:"accountId"`
	ServerID  string `json:"serverId"`
	Audience  string `json:"audience"`
	ProfileID string `json:"profileId,omitempty"`
}

type notificationStreamAuthorization struct {
	SessionID             string
	TokenHash             string
	DeviceID              string
	InstallationID        string
	AuthorizationRevision string
}

type viewerNotificationAction struct {
	ID             string            `json:"id"`
	LabelMessageID string            `json:"labelMessageId"`
	Kind           string            `json:"kind"`
	Target         string            `json:"target"`
	Parameters     map[string]string `json:"parameters"`
}

type viewerNotificationContent struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body"`
}

type viewerNotification struct {
	ID            string                     `json:"id"`
	Recipient     notificationRecipient      `json:"recipient"`
	Kind          string                     `json:"kind"`
	Severity      string                     `json:"severity"`
	MessageID     string                     `json:"messageId"`
	IconID        string                     `json:"iconId"`
	Interpolation map[string]string          `json:"interpolation"`
	Actions       []viewerNotificationAction `json:"actions"`
	Content       *viewerNotificationContent `json:"content,omitempty"`
	CreatedAt     string                     `json:"createdAt"`
	ReadAt        *string                    `json:"readAt,omitempty"`
	ArchivedAt    *string                    `json:"archivedAt,omitempty"`
}

type notificationPageInfo struct {
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

type viewerNotificationPage struct {
	Recipient   notificationRecipient `json:"recipient"`
	Items       []viewerNotification  `json:"items"`
	UnreadCount int                   `json:"unreadCount"`
	Revision    int64                 `json:"revision"`
	PageInfo    notificationPageInfo  `json:"pageInfo"`
}

type notificationMutationRequest struct {
	Read     *bool `json:"read,omitempty"`
	Archived *bool `json:"archived,omitempty"`
}

type notificationReceiptMutation struct {
	Version          string                `json:"version"`
	Recipient        notificationRecipient `json:"recipient"`
	NotificationIDs  []string              `json:"notificationIds"`
	Action           string                `json:"action"`
	ExpectedRevision int64                 `json:"expectedRevision"`
}

type notificationReceiptItem struct {
	NotificationID string  `json:"notificationId"`
	ReadAt         *string `json:"readAt"`
	ArchivedAt     *string `json:"archivedAt"`
}

type notificationReceiptResult struct {
	Recipient   notificationRecipient     `json:"recipient"`
	Receipts    []notificationReceiptItem `json:"receipts"`
	UnreadCount int                       `json:"unreadCount"`
	Revision    int64                     `json:"revision"`
}

type feedbackContextRequest struct {
	MediaID           string `json:"mediaId,omitempty"`
	PlaybackSessionID string `json:"playbackSessionId,omitempty"`
	DeviceClass       string `json:"deviceClass"`
	Platform          string `json:"platform"`
	AppVersion        string `json:"appVersion"`
}

type viewerFeedbackRequest struct {
	Version  string                 `json:"version"`
	Kind     string                 `json:"kind"`
	Category string                 `json:"category"`
	Message  string                 `json:"message"`
	Context  feedbackContextRequest `json:"context"`
}

type viewerFeedbackReceipt struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	SubmittedAt   string `json:"submittedAt"`
	DuplicateOfID string `json:"duplicateOfId,omitempty"`
}

type ownerFeedbackReporter struct {
	Authority    string `json:"authority"`
	AccountID    string `json:"accountId,omitempty"`
	MembershipID string `json:"membershipId,omitempty"`
	AccountName  string `json:"accountName"`
}

type ownerFeedbackResponse struct {
	Message     string `json:"message"`
	RespondedAt string `json:"respondedAt"`
}

type ownerFeedbackRecord struct {
	ID             string                 `json:"id"`
	Reporter       ownerFeedbackReporter  `json:"reporter"`
	Kind           string                 `json:"kind"`
	Category       string                 `json:"category"`
	Message        string                 `json:"message"`
	Diagnostics    map[string]any         `json:"diagnostics"`
	Status         string                 `json:"status"`
	DuplicateCount int                    `json:"duplicateCount"`
	OwnerResponse  *ownerFeedbackResponse `json:"ownerResponse,omitempty"`
	SubmittedAt    string                 `json:"submittedAt"`
	UpdatedAt      string                 `json:"updatedAt"`
	Revision       int64                  `json:"revision"`
}

type ownerFeedbackPage struct {
	Items        []ownerFeedbackRecord `json:"items"`
	PageInfo     notificationPageInfo  `json:"pageInfo"`
	StatusCounts map[string]int        `json:"statusCounts"`
}

type ownerFeedbackUpdateRequest struct {
	Version          string  `json:"version"`
	ExpectedRevision int64   `json:"expectedRevision"`
	Status           *string `json:"status,omitempty"`
	ResponseMessage  *string `json:"responseMessage,omitempty"`
}

type ownerNoticeRequest struct {
	Audience  string `json:"audience"`
	AccountID string `json:"accountId,omitempty"`
	ProfileID string `json:"profileId,omitempty"`
	Message   string `json:"message"`
	Severity  string `json:"severity,omitempty"`
}

type ownerNotificationProfileRecipient struct {
	Authority   string `json:"authority"`
	Audience    string `json:"audience"`
	AccountID   string `json:"accountId"`
	ProfileID   string `json:"profileId"`
	AccountName string `json:"accountName"`
	ProfileName string `json:"profileName"`
}

type ownerNotificationAccountAdminRecipient struct {
	Authority   string `json:"authority"`
	Audience    string `json:"audience"`
	AccountID   string `json:"accountId"`
	AccountName string `json:"accountName"`
}

type ownerNotificationRecipientDirectory struct {
	Profiles      []ownerNotificationProfileRecipient      `json:"profiles"`
	AccountAdmins []ownerNotificationAccountAdminRecipient `json:"accountAdmins"`
}

type viewerFeedbackPolicy struct {
	Enabled                   bool `json:"enabled"`
	OwnerResponsesEnabled     bool `json:"ownerResponsesEnabled"`
	FeedbackRetentionDays     int  `json:"feedbackRetentionDays"`
	NotificationRetentionDays int  `json:"notificationRetentionDays"`
}

func (s *Server) handleViewerNotifications(w http.ResponseWriter, r *http.Request, user User) {
	if r.URL.Path != "/api/notifications" {
		s.handleViewerNotificationRoute(w, r, user)
		return
	}
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	recipient, ok := s.notificationRecipientForRequest(w, r, user)
	if !ok {
		return
	}
	limit := parsePreferenceLimit(r.URL.Query().Get("limit"), 25, maximumViewerNotificationPage)
	cursorTime, cursorID, err := decodeTimeIDCursor(strings.TrimSpace(r.URL.Query().Get("cursor")))
	if err != nil {
		writeProductError(w, http.StatusBadRequest, "invalid_cursor", "This notifications page link is invalid. Start again from the first page.")
		return
	}
	includeArchived := r.URL.Query().Get("includeArchived") == "true"
	page, err := s.listViewerNotificationsContext(r.Context(), recipient, limit, cursorTime, cursorID, includeArchived)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "notification_load_failed", "Unable to load notifications.")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleViewerNotificationRoute(w http.ResponseWriter, r *http.Request, user User) {
	path := strings.TrimPrefix(r.URL.Path, "/api/notifications/")
	if path == "read-all" {
		s.handleViewerNotificationReadAll(w, r, user)
		return
	}
	if path == "receipts" {
		s.handleViewerNotificationReceipts(w, r, user)
		return
	}
	if path == "events" {
		s.handleViewerNotificationEvents(w, r, user)
		return
	}
	if path == "events/poll" {
		s.handleViewerNotificationEventsPoll(w, r, user)
		return
	}
	if path == "" || strings.Contains(path, "/") {
		writeProductError(w, http.StatusNotFound, "notification_not_found", "Notification not found.")
		return
	}
	if r.Method != http.MethodPatch {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PATCH for this endpoint.")
		return
	}
	recipient, ok := s.notificationRecipientForRequest(w, r, user)
	if !ok {
		return
	}
	var request notificationMutationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Read == nil && request.Archived == nil {
		writeProductError(w, http.StatusBadRequest, "invalid_notification_action", "Choose whether to mark this notification read or archive it.")
		return
	}
	updated, err := s.mutateViewerNotificationContext(r.Context(), recipient, path, request)
	if errors.Is(err, sql.ErrNoRows) {
		writeProductError(w, http.StatusNotFound, "notification_not_found", "Notification not found.")
		return
	}
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "notification_update_failed", "Unable to update this notification.")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) notificationRecipientForRequest(w http.ResponseWriter, r *http.Request, user User) (notificationRecipient, bool) {
	if _, err := s.currentInteractiveSessionBindingContext(r.Context(), r, user); err != nil {
		writeProductError(w, http.StatusForbidden, "interactive_session_required", "Notifications require a signed-in app session.")
		return notificationRecipient{}, false
	}
	audience := strings.TrimSpace(r.URL.Query().Get("audience"))
	if audience == "" {
		audience = "profile"
	}
	if audience != "profile" && audience != "account-admin" {
		writeProductError(w, http.StatusBadRequest, "invalid_notification_audience", "Choose profile or account-administrator notifications.")
		return notificationRecipient{}, false
	}
	if audience == "account-admin" && !selectedProfileMayManageAccount(user) {
		writeProductError(w, http.StatusForbidden, "primary_profile_required", "Switch to the primary profile to view account notifications.")
		return notificationRecipient{}, false
	}
	serverID, err := s.profileDirectoryServerIDContext(r.Context())
	if err != nil {
		writeProductError(w, http.StatusInternalServerError, "notification_load_failed", "Unable to load notifications.")
		return notificationRecipient{}, false
	}
	authority := "local"
	if user.AuthOrigin == "portico" || user.AuthProvider == "portico" {
		authority = "hosted"
	}
	recipient := notificationRecipient{Authority: authority, AccountID: accountIDForUser(user), ServerID: serverID, Audience: audience}
	if audience == "profile" {
		recipient.ProfileID = viewerProfileID(user)
	}
	return recipient, true
}

func (s *Server) listViewerNotificationsContext(ctx context.Context, recipient notificationRecipient, limit int, cursorTime, cursorID string, includeArchived bool) (viewerNotificationPage, error) {
	_ = s.pruneViewerCommunicationContext(ctx)
	whereCursor, cursorArgs := "", []any{}
	if cursorTime != "" {
		whereCursor = " AND (n.created_at < ? OR (n.created_at = ? AND n.id < ?))"
		cursorArgs = append(cursorArgs, cursorTime, cursorTime, cursorID)
	}
	archiveClause := " AND COALESCE(receipt.archived_at, '') = ''"
	if includeArchived {
		archiveClause = ""
	}
	args := []any{recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience}
	args = append(args, cursorArgs...)
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT n.id, n.kind, n.severity, n.message_id, n.icon_id, n.interpolation_json, n.content_json, n.actions_json,
		       n.created_at, COALESCE(receipt.read_at, ''), COALESCE(receipt.archived_at, '')
		FROM viewer_notifications n
		LEFT JOIN viewer_notification_receipts receipt
		  ON receipt.notification_id = n.id AND receipt.authority = n.authority AND receipt.account_id = n.account_id AND receipt.server_id = n.server_id
		 AND receipt.profile_id = n.profile_id AND receipt.audience = n.audience
		WHERE n.authority = ? AND n.account_id = ? AND n.server_id = ? AND n.profile_id = ? AND n.audience = ?
		  AND n.expires_at > CURRENT_TIMESTAMP`+archiveClause+whereCursor+`
		ORDER BY n.created_at DESC, n.id DESC LIMIT ?`, args...)
	if err != nil {
		return viewerNotificationPage{}, err
	}
	defer rows.Close()
	items := []viewerNotification{}
	for rows.Next() {
		var item viewerNotification
		var interpolationRaw, contentRaw, actionsRaw, readAt, archivedAt string
		if err := rows.Scan(&item.ID, &item.Kind, &item.Severity, &item.MessageID, &item.IconID, &interpolationRaw, &contentRaw, &actionsRaw, &item.CreatedAt, &readAt, &archivedAt); err != nil {
			return viewerNotificationPage{}, err
		}
		item.Recipient = recipient
		if err := json.Unmarshal([]byte(interpolationRaw), &item.Interpolation); err != nil {
			return viewerNotificationPage{}, err
		}
		if err := json.Unmarshal([]byte(actionsRaw), &item.Actions); err != nil {
			return viewerNotificationPage{}, err
		}
		if contentRaw != "{}" {
			var content viewerNotificationContent
			if err := json.Unmarshal([]byte(contentRaw), &content); err != nil {
				return viewerNotificationPage{}, err
			}
			if content.Body != "" {
				item.Content = &content
			}
		}
		if item.Interpolation == nil {
			item.Interpolation = map[string]string{}
		}
		if item.Actions == nil {
			item.Actions = []viewerNotificationAction{}
		}
		if readAt != "" {
			value := readAt
			item.ReadAt = &value
		}
		if archivedAt != "" {
			value := archivedAt
			item.ArchivedAt = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return viewerNotificationPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor *string
	if hasMore && len(items) > 0 {
		value := encodeTimeIDCursor(items[len(items)-1].CreatedAt, items[len(items)-1].ID)
		nextCursor = &value
	}
	unread, revision, err := notificationCountsContext(s, ctx, recipient)
	if err != nil {
		return viewerNotificationPage{}, err
	}
	return viewerNotificationPage{Recipient: recipient, Items: items, UnreadCount: unread, Revision: revision, PageInfo: notificationPageInfo{NextCursor: nextCursor, HasMore: hasMore}}, nil
}

func notificationCountsContext(s *Server, ctx context.Context, recipient notificationRecipient) (int, int64, error) {
	var unread int
	err := s.queryUserRow(ctx, `
		SELECT COUNT(*) FROM viewer_notifications n
		LEFT JOIN viewer_notification_receipts receipt
		  ON receipt.notification_id = n.id AND receipt.authority = n.authority AND receipt.account_id = n.account_id AND receipt.server_id = n.server_id
		 AND receipt.profile_id = n.profile_id AND receipt.audience = n.audience
		WHERE n.authority = ? AND n.account_id = ? AND n.server_id = ? AND n.profile_id = ? AND n.audience = ?
		  AND n.expires_at > CURRENT_TIMESTAMP AND COALESCE(receipt.read_at, '') = '' AND COALESCE(receipt.archived_at, '') = ''`,
		recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience).Scan(&unread)
	if err != nil {
		return 0, 0, err
	}
	var revision int64
	err = s.queryUserRow(ctx, `SELECT revision FROM viewer_notification_revisions WHERE authority = ? AND account_id = ? AND server_id = ? AND profile_id = ? AND audience = ?`,
		recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return unread, 0, nil
	}
	return unread, revision, err
}

func (s *Server) mutateViewerNotificationContext(ctx context.Context, recipient notificationRecipient, notificationID string, request notificationMutationRequest) (viewerNotification, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.withUserTxTagged(ctx, nil, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM viewer_notifications WHERE id = ? AND authority = ? AND account_id = ? AND server_id = ? AND profile_id = ? AND audience = ? AND expires_at > ?`,
			notificationID, recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience, now).Scan(&exists); err != nil {
			return err
		}
		if exists != 1 {
			return sql.ErrNoRows
		}
		var readAt, archivedAt string
		_ = tx.QueryRow(`SELECT read_at, archived_at FROM viewer_notification_receipts WHERE notification_id = ? AND authority = ? AND account_id = ? AND server_id = ? AND profile_id = ? AND audience = ?`,
			notificationID, recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience).Scan(&readAt, &archivedAt)
		if request.Read != nil {
			if *request.Read {
				readAt = now
			} else {
				readAt = ""
			}
		}
		if request.Archived != nil {
			if *request.Archived {
				archivedAt = now
			} else {
				archivedAt = ""
			}
		}
		if _, err := tx.Exec(`
			INSERT INTO viewer_notification_receipts (notification_id, authority, account_id, server_id, profile_id, audience, read_at, archived_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(notification_id, authority, account_id, server_id, profile_id, audience) DO UPDATE SET
				read_at = excluded.read_at, archived_at = excluded.archived_at, updated_at = excluded.updated_at`,
			notificationID, recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience, readAt, archivedAt, now); err != nil {
			return err
		}
		return bumpNotificationRevisionTx(tx, recipient, now)
	})
	if err != nil {
		return viewerNotification{}, err
	}
	s.notifyLongPollNotification(recipient)
	page, err := s.listViewerNotificationsContext(ctx, recipient, 1, "", "", true)
	if err != nil {
		return viewerNotification{}, err
	}
	for _, item := range page.Items {
		if item.ID == notificationID {
			return item, nil
		}
	}
	return s.viewerNotificationByIDContext(ctx, recipient, notificationID)
}

func (s *Server) viewerNotificationByIDContext(ctx context.Context, recipient notificationRecipient, notificationID string) (viewerNotification, error) {
	var item viewerNotification
	var interpolationRaw, contentRaw, actionsRaw, readAt, archivedAt string
	err := s.queryUserRow(ctx, `
		SELECT n.id, n.kind, n.severity, n.message_id, n.icon_id, n.interpolation_json, n.content_json, n.actions_json, n.created_at,
		       COALESCE(receipt.read_at, ''), COALESCE(receipt.archived_at, '')
		FROM viewer_notifications n LEFT JOIN viewer_notification_receipts receipt
		  ON receipt.notification_id = n.id AND receipt.authority = n.authority AND receipt.account_id = n.account_id AND receipt.server_id = n.server_id
		 AND receipt.profile_id = n.profile_id AND receipt.audience = n.audience
		WHERE n.id = ? AND n.authority = ? AND n.account_id = ? AND n.server_id = ? AND n.profile_id = ? AND n.audience = ?`,
		notificationID, recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience).
		Scan(&item.ID, &item.Kind, &item.Severity, &item.MessageID, &item.IconID, &interpolationRaw, &contentRaw, &actionsRaw, &item.CreatedAt, &readAt, &archivedAt)
	if err != nil {
		return viewerNotification{}, err
	}
	item.Recipient = recipient
	if err := json.Unmarshal([]byte(interpolationRaw), &item.Interpolation); err != nil {
		return viewerNotification{}, err
	}
	if err := json.Unmarshal([]byte(actionsRaw), &item.Actions); err != nil {
		return viewerNotification{}, err
	}
	if contentRaw != "{}" {
		var content viewerNotificationContent
		if err := json.Unmarshal([]byte(contentRaw), &content); err != nil {
			return viewerNotification{}, err
		}
		if content.Body != "" {
			item.Content = &content
		}
	}
	if readAt != "" {
		item.ReadAt = &readAt
	}
	if archivedAt != "" {
		item.ArchivedAt = &archivedAt
	}
	return item, nil
}

func (s *Server) handleViewerNotificationReadAll(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	recipient, ok := s.notificationRecipientForRequest(w, r, user)
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.withUserTxTagged(r.Context(), nil, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO viewer_notification_receipts (notification_id, authority, account_id, server_id, profile_id, audience, read_at, archived_at, updated_at)
			SELECT n.id, n.authority, n.account_id, n.server_id, n.profile_id, n.audience, ?, COALESCE(receipt.archived_at, ''), ?
			FROM viewer_notifications n LEFT JOIN viewer_notification_receipts receipt
			  ON receipt.notification_id = n.id AND receipt.authority = n.authority AND receipt.account_id = n.account_id AND receipt.server_id = n.server_id
			 AND receipt.profile_id = n.profile_id AND receipt.audience = n.audience
			WHERE n.authority = ? AND n.account_id = ? AND n.server_id = ? AND n.profile_id = ? AND n.audience = ? AND n.expires_at > ?
			ON CONFLICT(notification_id, authority, account_id, server_id, profile_id, audience) DO UPDATE SET read_at = excluded.read_at, updated_at = excluded.updated_at`,
			now, now, recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience, now)
		if err != nil {
			return err
		}
		return bumpNotificationRevisionTx(tx, recipient, now)
	})
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "notification_update_failed", "Unable to mark notifications read.")
		return
	}
	s.notifyLongPollNotification(recipient)
	unread, revision, err := notificationCountsContext(s, r.Context(), recipient)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "notification_update_failed", "Unable to mark notifications read.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recipient": recipient, "revision": revision, "unreadCount": unread})
}

func (s *Server) handleViewerNotificationReceipts(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	recipient, ok := s.notificationRecipientForRequest(w, r, user)
	if !ok {
		return
	}
	var request notificationReceiptMutation
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Version != "v1" || request.ExpectedRevision < 0 || !oneOf(request.Action, "mark-read", "mark-unread", "archive") ||
		len(request.NotificationIDs) == 0 || len(request.NotificationIDs) > 100 || !sameNotificationRecipientValue(request.Recipient, recipient) {
		writeProductError(w, http.StatusBadRequest, "invalid_notification_action", "This notification update is invalid for the current viewer.")
		return
	}
	seen := map[string]bool{}
	for _, id := range request.NotificationIDs {
		if strings.TrimSpace(id) == "" || len(id) > 128 || seen[id] {
			writeProductError(w, http.StatusBadRequest, "invalid_notification_action", "Notification identifiers must be unique.")
			return
		}
		seen[id] = true
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result := notificationReceiptResult{Recipient: recipient, Receipts: []notificationReceiptItem{}}
	err := s.withUserTxTagged(r.Context(), nil, func(tx *sql.Tx) error {
		var revision int64
		err := tx.QueryRow(`SELECT revision FROM viewer_notification_revisions WHERE authority = ? AND account_id = ? AND server_id = ? AND profile_id = ? AND audience = ?`,
			recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience).Scan(&revision)
		if errors.Is(err, sql.ErrNoRows) {
			revision = 0
		} else if err != nil {
			return err
		}
		if revision != request.ExpectedRevision {
			return errPreferenceConflict
		}
		for _, notificationID := range request.NotificationIDs {
			var exists int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM viewer_notifications WHERE id = ? AND authority = ? AND account_id = ? AND server_id = ? AND profile_id = ? AND audience = ? AND expires_at > ?`,
				notificationID, recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience, now).Scan(&exists); err != nil {
				return err
			}
			if exists != 1 {
				return sql.ErrNoRows
			}
			var readAt, archivedAt string
			_ = tx.QueryRow(`SELECT read_at, archived_at FROM viewer_notification_receipts WHERE notification_id = ? AND authority = ? AND account_id = ? AND server_id = ? AND profile_id = ? AND audience = ?`,
				notificationID, recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience).Scan(&readAt, &archivedAt)
			switch request.Action {
			case "mark-read":
				readAt = now
			case "mark-unread":
				readAt = ""
			case "archive":
				archivedAt, readAt = now, now
			}
			if _, err := tx.Exec(`
				INSERT INTO viewer_notification_receipts (notification_id, authority, account_id, server_id, profile_id, audience, read_at, archived_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(notification_id, authority, account_id, server_id, profile_id, audience) DO UPDATE SET read_at = excluded.read_at, archived_at = excluded.archived_at, updated_at = excluded.updated_at`,
				notificationID, recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience, readAt, archivedAt, now); err != nil {
				return err
			}
			receipt := notificationReceiptItem{NotificationID: notificationID}
			if readAt != "" {
				receipt.ReadAt = &readAt
			}
			if archivedAt != "" {
				receipt.ArchivedAt = &archivedAt
			}
			result.Receipts = append(result.Receipts, receipt)
		}
		return bumpNotificationRevisionTx(tx, recipient, now)
	})
	if errors.Is(err, errPreferenceConflict) {
		writeProductError(w, http.StatusConflict, "notification_receipt_conflict", "Notifications changed elsewhere. Refresh and try again.")
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeProductError(w, http.StatusNotFound, "notification_not_found", "Notification not found.")
		return
	}
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "notification_update_failed", "Unable to update notifications.")
		return
	}
	s.notifyLongPollNotification(recipient)
	result.UnreadCount, result.Revision, err = notificationCountsContext(s, r.Context(), recipient)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "notification_update_failed", "Unable to update notifications.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func sameNotificationRecipientValue(left, right notificationRecipient) bool {
	return left.Authority == right.Authority && left.AccountID == right.AccountID && left.ServerID == right.ServerID && left.Audience == right.Audience && left.ProfileID == right.ProfileID
}

func bumpNotificationRevisionTx(tx *sql.Tx, recipient notificationRecipient, now string) error {
	_, err := tx.Exec(`
		INSERT INTO viewer_notification_revisions (authority, account_id, server_id, profile_id, audience, revision, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?)
		ON CONFLICT(authority, account_id, server_id, profile_id, audience) DO UPDATE SET revision = revision + 1, updated_at = excluded.updated_at`,
		recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience, now)
	return err
}

func (s *Server) handleViewerNotificationEvents(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	recipient, ok := s.notificationRecipientForRequest(w, r, user)
	if !ok {
		return
	}
	authorization, err := s.notificationStreamAuthorizationContext(r.Context(), r, user, recipient)
	if err != nil {
		writeProductError(w, http.StatusForbidden, "interactive_session_required", "Notifications require a signed-in app session.")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProductError(w, http.StatusNotImplemented, "streaming_unavailable", "Live notification updates are unavailable.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Portico-Stream-Resume", "none")
	if streamResumeResetRequested(r, w, flusher) {
		return
	}
	authorizationTicker := time.NewTicker(viewerNotificationStreamAuthorizationInterval)
	defer authorizationTicker.Stop()
	heartbeatTicker := time.NewTicker(25 * time.Second)
	defer heartbeatTicker.Stop()
	// Production servers initialize this eagerly. The lazy fallback keeps
	// narrowly constructed test/embedded servers safe without weakening the
	// signal-driven freshness guarantee.
	runtime := s.ensureLongPollRuntime()
	signal, unsubscribe := runtime.broker.subscribe(s.notificationLongPollKey(recipient))
	defer unsubscribe()
	lastRevision := int64(-1)
	writeInvalidation := func() bool {
		revision, err := notificationRevisionContext(s, r.Context(), recipient)
		if err != nil || !s.notificationStreamAuthorizationCurrentContext(r.Context(), recipient, authorization) {
			return false
		}
		if revision == lastRevision {
			return true
		}
		// The revision comparison is the linearization point for this
		// content-free wake-up. Authorization may change after this point,
		// so the frame must never contain recipient or inbox state.
		if viewerNotificationStreamCandidateSelectedHook != nil {
			viewerNotificationStreamCandidateSelectedHook()
		}
		payload, _ := json.Marshal(map[string]any{
			"version": "v1", "kind": "notifications.invalidated", "occurredAt": time.Now().UTC().Format(time.RFC3339Nano),
		})
		prepareStreamWrite(w)
		if _, err := fmt.Fprintf(w, "event: notification-invalidation\ndata: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		lastRevision = revision
		return true
	}
	if !writeInvalidation() {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.shutdownDone():
			return
		case <-signal:
			if !writeInvalidation() {
				return
			}
		case <-authorizationTicker.C:
			// Signals provide normal low-latency delivery. This bounded fallback
			// also recovers from a missed in-process notification (for example an
			// administrative/import write) while revalidating authorization before
			// every frame.
			if !writeInvalidation() {
				return
			}
		case <-heartbeatTicker.C:
			prepareStreamWrite(w)
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) notificationStreamAuthorizationContext(ctx context.Context, r *http.Request, user User, recipient notificationRecipient) (notificationStreamAuthorization, error) {
	if user.APIKeyID != "" || user.AuthProvider == "api_key" {
		return notificationStreamAuthorization{}, errInvalidProfileAdministrationProof
	}
	for tokenHash := range s.currentSessionTokenHashes(r) {
		authorization, authProvider, authOrigin, isPrimary, err := s.notificationStreamSessionContext(ctx, tokenHash, "", recipient)
		if err != nil {
			continue
		}
		if !notificationStreamAuthorityMatches(recipient, authProvider, authOrigin, isPrimary) {
			continue
		}
		if !s.notificationStreamDeviceAuthorizedContext(ctx, recipient.AccountID, authorization.DeviceID) {
			continue
		}
		authorization.AuthorizationRevision, err = s.notificationStreamAuthorizationRevisionContext(ctx, recipient)
		if err != nil {
			continue
		}
		return authorization, nil
	}
	return notificationStreamAuthorization{}, errInvalidProfileAdministrationProof
}

func (s *Server) notificationStreamAuthorizationCurrentContext(ctx context.Context, recipient notificationRecipient, expected notificationStreamAuthorization) bool {
	current, authProvider, authOrigin, isPrimary, err := s.notificationStreamSessionContext(ctx, expected.TokenHash, expected.SessionID, recipient)
	if err != nil || current.DeviceID != expected.DeviceID {
		return false
	}
	if !notificationStreamAuthorityMatches(recipient, authProvider, authOrigin, isPrimary) {
		return false
	}
	if !s.notificationStreamDeviceAuthorizedContext(ctx, recipient.AccountID, current.DeviceID) {
		return false
	}
	currentRevision, err := s.notificationStreamAuthorizationRevisionContext(ctx, recipient)
	return err == nil && currentRevision == expected.AuthorizationRevision
}

func (s *Server) notificationStreamSessionContext(ctx context.Context, tokenHash, sessionID string, recipient notificationRecipient) (notificationStreamAuthorization, string, string, bool, error) {
	query := `
		SELECT session.id, session.token_hash, session.device_id, device.installation_id,
		       COALESCE(NULLIF(session.auth_provider, ''), 'local'), COALESCE(account.auth_origin, 'local'), profile.is_primary
		FROM sessions session
		JOIN users account ON account.id = session.user_id AND COALESCE(account.disabled_at, '') = ''
		JOIN profiles profile ON profile.id = COALESCE(NULLIF(session.profile_id, ''), session.user_id)
			AND profile.account_id = session.user_id AND profile.disabled_at = ''
		JOIN devices device ON device.id = session.device_id AND device.user_id = session.user_id
			AND COALESCE(device.revoked_at, '') = ''
		WHERE session.token_hash = ? AND session.user_id = ?
		  AND COALESCE(NULLIF(session.profile_id, ''), session.user_id) = ?
		  AND session.expires_at > ?`
	args := []any{tokenHash, recipient.AccountID, notificationStreamRecipientProfileID(recipient), time.Now().UTC().Format(time.RFC3339Nano)}
	if sessionID != "" {
		query += " AND session.id = ?"
		args = append(args, sessionID)
	}
	var authorization notificationStreamAuthorization
	var authProvider, authOrigin string
	var primary int
	err := s.queryUserRow(ctx, query, args...).Scan(
		&authorization.SessionID, &authorization.TokenHash, &authorization.DeviceID, &authorization.InstallationID,
		&authProvider, &authOrigin, &primary)
	return authorization, authProvider, authOrigin, primary == 1, err
}

func notificationStreamRecipientProfileID(recipient notificationRecipient) string {
	if recipient.ProfileID != "" {
		return recipient.ProfileID
	}
	return recipient.AccountID
}

func notificationStreamAuthorityMatches(recipient notificationRecipient, authProvider, authOrigin string, isPrimary bool) bool {
	if strings.EqualFold(strings.TrimSpace(authProvider), "api_key") {
		return false
	}
	authority := "local"
	if normalizeAuthProvider(authProvider) == "portico" || strings.EqualFold(strings.TrimSpace(authOrigin), "portico") {
		authority = "hosted"
	}
	return recipient.Authority == authority && (recipient.Audience != "account-admin" || isPrimary)
}

func (s *Server) notificationStreamDeviceAuthorizedContext(ctx context.Context, accountID, deviceID string) bool {
	settings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return false
	}
	devices, _ := settings["devices"].(map[string]any)
	if settingBool(devices, "requireTrustedDevices", false) && !s.deviceTrustedContext(ctx, deviceID) {
		return false
	}
	return s.userDevicePolicyAllowsContext(ctx, accountID, deviceID)
}

func (s *Server) notificationStreamAuthorizationRevisionContext(ctx context.Context, recipient notificationRecipient) (string, error) {
	var userUpdated, permissions, accountDisabled, profileUpdated, profilePolicyUpdated, restrictions, profileDisabled string
	var profilesAllowed, pinRevision, hostedRevision int64
	err := s.queryUserRow(ctx, `
		SELECT account.updated_at, account.permissions_json, COALESCE(account.disabled_at, ''), COALESCE(account.allow_account_profiles, 1),
			profile.updated_at, profile.policy_updated_at, profile.restrictions_json, COALESCE(profile.disabled_at, ''), profile.pin_revision,
			COALESCE((SELECT revision FROM hosted_profile_snapshot_state WHERE account_id = account.id AND quarantined_at = ''), 0)
		FROM users account JOIN profiles profile ON profile.account_id = account.id
		WHERE account.id = ? AND profile.id = ?`, recipient.AccountID, notificationStreamRecipientProfileID(recipient)).Scan(
		&userUpdated, &permissions, &accountDisabled, &profilesAllowed,
		&profileUpdated, &profilePolicyUpdated, &restrictions, &profileDisabled, &pinRevision, &hostedRevision)
	if err != nil {
		return "", err
	}
	material := strings.Join([]string{recipient.AccountID, notificationStreamRecipientProfileID(recipient), userUpdated, permissions, accountDisabled,
		strconv.FormatInt(profilesAllowed, 10), profileUpdated, profilePolicyUpdated, restrictions, profileDisabled,
		strconv.FormatInt(pinRevision, 10), strconv.FormatInt(hostedRevision, 10)}, "\x00")
	digest := sha256.Sum256([]byte(material))
	return "authz_" + hex.EncodeToString(digest[:16]), nil
}

func notificationRevisionContext(s *Server, ctx context.Context, recipient notificationRecipient) (int64, error) {
	var revision int64
	err := s.queryUserRow(ctx, `SELECT revision FROM viewer_notification_revisions WHERE authority = ? AND account_id = ? AND server_id = ? AND profile_id = ? AND audience = ?`,
		recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return revision, err
}

func (s *Server) handleViewerFeedback(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	policy := s.viewerFeedbackPolicyContext(r.Context())
	if !policy.Enabled || !user.AllowFeedback || user.AuthProvider == "api_key" {
		writeProductError(w, http.StatusForbidden, "feedback_disabled", "Messages to the server owner are not available for this profile.")
		return
	}
	var request viewerFeedbackRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := validateViewerFeedbackRequest(request); err != nil {
		writeProductError(w, http.StatusBadRequest, "invalid_feedback", err.Error())
		return
	}
	diagnostics, mediaTitle, err := s.viewerFeedbackDiagnosticsContext(r.Context(), user, request)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeProductError(w, http.StatusNotFound, "feedback_context_not_found", "This media or playback session is no longer available.")
			return
		}
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "feedback_failed", "Unable to confirm the problem context.")
		return
	}
	diagnosticsRaw, _ := json.Marshal(diagnostics)
	hash := feedbackDuplicateHash(user, request)
	now := time.Now().UTC()
	authority := "local"
	if user.AuthOrigin == "portico" || user.AuthProvider == "portico" {
		authority = "hosted"
	}
	id := randomID("feedback")
	expires := now.Add(time.Duration(policy.FeedbackRetentionDays) * 24 * time.Hour)
	notificationExpires := now.Add(time.Duration(policy.NotificationRetentionDays) * 24 * time.Hour)
	if expires.Before(notificationExpires) {
		notificationExpires = expires
	}
	serverID, err := s.profileDirectoryServerIDContext(r.Context())
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "feedback_failed", "Unable to send this message.")
		return
	}
	var existing viewerFeedbackReceipt
	duplicateFound := false
	err = s.withUserTxTagged(r.Context(), nil, func(tx *sql.Tx) error {
		var recent int
		if err := tx.QueryRow(`
			SELECT COALESCE(SUM(1 + duplicate_count), 0)
			FROM viewer_feedback
			WHERE account_id = ? AND profile_id = ? AND (created_at > ? OR updated_at > ?)`,
			accountIDForUser(user), viewerProfileID(user), now.Add(-time.Hour).Format(time.RFC3339Nano), now.Add(-time.Hour).Format(time.RFC3339Nano)).Scan(&recent); err != nil {
			return err
		}
		if recent >= 10 {
			return errFeedbackRateLimited
		}
		err := tx.QueryRow(`
			SELECT id, status, created_at FROM viewer_feedback
			WHERE account_id = ? AND profile_id = ? AND duplicate_hash = ? AND created_at > ?
			ORDER BY created_at DESC LIMIT 1`, accountIDForUser(user), viewerProfileID(user), hash, now.Add(-24*time.Hour).Format(time.RFC3339Nano)).
			Scan(&existing.ID, &existing.Status, &existing.SubmittedAt)
		if err == nil {
			result, err := tx.Exec(`UPDATE viewer_feedback SET duplicate_count = duplicate_count + 1, revision = revision + 1, updated_at = ? WHERE id = ?`, now.Format(time.RFC3339Nano), existing.ID)
			if err != nil {
				return err
			}
			if affected, err := result.RowsAffected(); err != nil || affected != 1 {
				return errors.New("duplicate feedback update did not affect one record")
			}
			duplicateFound = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO viewer_feedback (
				id, authority, account_id, profile_id, kind, category, message, media_id, playback_session_id,
				device_class, platform, app_version, error_category, diagnostics_json, duplicate_hash, status,
				created_at, updated_at, expires_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'new', ?, ?, ?)`,
			id, authority, accountIDForUser(user), viewerProfileID(user), request.Kind, request.Category, strings.TrimSpace(request.Message),
			strings.TrimSpace(request.Context.MediaID), strings.TrimSpace(request.Context.PlaybackSessionID), request.Context.DeviceClass,
			strings.TrimSpace(request.Context.Platform), strings.TrimSpace(request.Context.AppVersion), "",
			string(diagnosticsRaw), hash, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		owners, err := feedbackOwnerRecipientsTx(tx, serverID)
		if err != nil {
			return err
		}
		if len(owners) == 0 {
			return errors.New("server owner recipient is unavailable")
		}
		for _, recipient := range owners {
			if err := createViewerNotificationTx(tx, recipient, "feedback.received", "informational", "notification.feedback-received", "status.feedback",
				map[string]string{}, nil, []viewerNotificationAction{{ID: "view-feedback", LabelMessageID: "action.open", Kind: "navigate", Target: "feedback.detail", Parameters: map[string]string{"feedbackId": id}}}, id,
				notificationExpires); err != nil {
				return err
			}
		}
		return nil
	})
	_ = mediaTitle
	if errors.Is(err, errFeedbackRateLimited) {
		w.Header().Set("Retry-After", "3600")
		writeProductError(w, http.StatusTooManyRequests, "feedback_rate_limited", "You have sent several messages recently. Try again later.")
		return
	}
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "feedback_failed", "Unable to send this message.")
		return
	}
	if duplicateFound {
		existing.DuplicateOfID = existing.ID
		writeJSON(w, http.StatusOK, existing)
		return
	}
	s.recordAudit(r, user, "viewer.feedback_received", "viewer_feedback", id, "info", map[string]string{"kind": request.Kind, "category": request.Category})
	writeJSON(w, http.StatusCreated, viewerFeedbackReceipt{ID: id, Status: "new", SubmittedAt: now.Format(time.RFC3339Nano)})
}

func feedbackOwnerRecipientsTx(tx *sql.Tx, serverID string) ([]notificationRecipient, error) {
	rows, err := tx.Query(`
		SELECT id, auth_origin
		FROM users
		WHERE role = 'owner' AND COALESCE(disabled_at, '') = ''
		ORDER BY CASE role WHEN 'owner' THEN 0 ELSE 1 END, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recipients := []notificationRecipient{}
	for rows.Next() {
		var accountID, authOrigin string
		if err := rows.Scan(&accountID, &authOrigin); err != nil {
			return nil, err
		}
		authority := "local"
		if authOrigin == "portico" {
			authority = "hosted"
		}
		recipients = append(recipients, notificationRecipient{Authority: authority, AccountID: accountID, ServerID: serverID, Audience: "account-admin"})
	}
	return recipients, rows.Err()
}

func (s *Server) handleViewerFeedbackCapabilities(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	policy := s.viewerFeedbackPolicyContext(r.Context())
	enabled := policy.Enabled && user.AllowFeedback && user.AuthProvider != "api_key"
	allowedKinds := []string{}
	if enabled {
		allowedKinds = []string{"general", "playback", "media", "quality"}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": "v1", "enabled": enabled, "allowedKinds": allowedKinds,
		"messageMaxLength": 1000, "retentionDays": policy.FeedbackRetentionDays,
	})
}

func validateViewerFeedbackRequest(request viewerFeedbackRequest) error {
	if request.Version != viewerFeedbackVersion || !oneOf(request.Kind, "general", "playback", "media", "quality") {
		return errors.New("This message format is not supported.")
	}
	allowed := map[string][]string{
		"general":  {"other"},
		"playback": {"wont-play", "buffering", "playback-stopped", "wrong-video", "wrong-audio", "wrong-subtitles", "other"},
		"media":    {"wrong-video", "wrong-audio", "wrong-subtitles", "incorrect-media-information", "other"},
		"quality":  {"higher-quality-request", "other"},
	}
	if !oneOf(request.Category, allowed[request.Kind]...) {
		return errors.New("Choose a category that matches this message.")
	}
	message := strings.TrimSpace(request.Message)
	if utf8.RuneCountInString(message) > 1000 || ((request.Kind == "general" || request.Category == "other") && message == "") {
		return errors.New("Write a message of up to 1,000 characters.")
	}
	if !oneOf(request.Context.DeviceClass, "web", "mobile", "television") || !boundedIdentifier(request.Context.Platform, 64) || !boundedText(request.Context.AppVersion, 64, false) {
		return errors.New("The app information for this message is invalid.")
	}
	if (request.Kind == "media" || request.Kind == "quality") && !boundedText(request.Context.MediaID, 128, false) {
		return errors.New("Choose media before sending this message.")
	}
	if request.Kind == "playback" && !boundedText(request.Context.PlaybackSessionID, 128, false) {
		return errors.New("The playback session is no longer available.")
	}
	return nil
}

func (s *Server) viewerFeedbackDiagnosticsContext(ctx context.Context, user User, request viewerFeedbackRequest) (map[string]any, string, error) {
	diagnostics := map[string]any{
		"deviceClass": request.Context.DeviceClass,
		"platform":    strings.TrimSpace(request.Context.Platform),
		"appVersion":  strings.TrimSpace(request.Context.AppVersion),
		"occurredAt":  time.Now().UTC().Format(time.RFC3339Nano),
	}
	mediaTitle := ""
	if mediaID := strings.TrimSpace(request.Context.MediaID); mediaID != "" {
		item, err := s.getMediaPlaybackDetailForUser(ctx, user, mediaID)
		if err != nil {
			return nil, "", err
		}
		diagnostics["mediaId"] = item.ID
		mediaTitle = item.Title
	}
	if sessionID := strings.TrimSpace(request.Context.PlaybackSessionID); sessionID != "" {
		mediaID, isLive, _, err := s.playbackSessionState(user, sessionID)
		if err != nil {
			return nil, "", err
		}
		_ = isLive
		diagnostics["playbackDecisionId"] = sessionID
		if _, exists := diagnostics["mediaId"]; !exists && mediaID != "" {
			diagnostics["mediaId"] = mediaID
		}
	}
	return diagnostics, mediaTitle, nil
}

func feedbackDuplicateHash(user User, request viewerFeedbackRequest) string {
	material := strings.Join([]string{accountIDForUser(user), viewerProfileID(user), request.Kind, request.Category,
		strings.TrimSpace(request.Message), strings.TrimSpace(request.Context.MediaID), strings.TrimSpace(request.Context.PlaybackSessionID),
		request.Context.DeviceClass}, "\x00")
	digest := sha256.Sum256([]byte(material))
	return hex.EncodeToString(digest[:])
}

func (s *Server) handleAdminViewerFeedback(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Server owner access is required.")
		return
	}
	if r.URL.Path != "/api/admin/viewer-feedback" {
		s.handleAdminViewerFeedbackRoute(w, r, user)
		return
	}
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !oneOf(status, "new", "read", "resolved", "dismissed") {
		writeProductError(w, http.StatusBadRequest, "invalid_feedback_status", "Choose a valid message status.")
		return
	}
	limit := parsePreferenceLimit(r.URL.Query().Get("limit"), 25, maximumViewerFeedbackPage)
	cursorTime, cursorID, err := decodeTimeIDCursor(strings.TrimSpace(r.URL.Query().Get("cursor")))
	if err != nil {
		writeProductError(w, http.StatusBadRequest, "invalid_cursor", "This messages page link is invalid.")
		return
	}
	page, err := s.listOwnerFeedbackContext(r.Context(), status, limit, cursorTime, cursorID)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "feedback_load_failed", "Unable to load viewer messages.")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) listOwnerFeedbackContext(ctx context.Context, status string, limit int, cursorTime, cursorID string) (ownerFeedbackPage, error) {
	_ = s.pruneViewerCommunicationContext(ctx)
	where, args := " WHERE feedback.expires_at > CURRENT_TIMESTAMP", []any{}
	if status != "" {
		where += " AND feedback.status = ?"
		args = append(args, status)
	}
	if cursorTime != "" {
		where += " AND (feedback.created_at < ? OR (feedback.created_at = ? AND feedback.id < ?))"
		args = append(args, cursorTime, cursorTime, cursorID)
	}
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT feedback.id, feedback.authority,
		       CASE WHEN feedback.authority = 'local' THEN feedback.account_id ELSE '' END,
		       CASE WHEN feedback.authority = 'hosted' THEN account.portico_membership_id ELSE '' END,
		       account.display_name,
		       feedback.kind, feedback.category, feedback.message, feedback.diagnostics_json, feedback.status,
		       feedback.duplicate_count, feedback.owner_response, feedback.responded_at,
		       feedback.created_at, feedback.updated_at, feedback.revision
		FROM viewer_feedback feedback
		JOIN users account ON account.id = feedback.account_id`+where+`
		ORDER BY feedback.created_at DESC, feedback.id DESC LIMIT ?`, args...)
	if err != nil {
		return ownerFeedbackPage{}, err
	}
	defer rows.Close()
	items := []ownerFeedbackRecord{}
	for rows.Next() {
		var item ownerFeedbackRecord
		var diagnosticsRaw, ownerResponse, respondedAt string
		if err := rows.Scan(&item.ID, &item.Reporter.Authority, &item.Reporter.AccountID, &item.Reporter.MembershipID, &item.Reporter.AccountName,
			&item.Kind, &item.Category, &item.Message, &diagnosticsRaw, &item.Status, &item.DuplicateCount,
			&ownerResponse, &respondedAt, &item.SubmittedAt, &item.UpdatedAt, &item.Revision); err != nil {
			return ownerFeedbackPage{}, err
		}
		if err := json.Unmarshal([]byte(diagnosticsRaw), &item.Diagnostics); err != nil {
			return ownerFeedbackPage{}, err
		}
		if ownerResponse != "" {
			item.OwnerResponse = &ownerFeedbackResponse{Message: ownerResponse, RespondedAt: respondedAt}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ownerFeedbackPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor *string
	if hasMore && len(items) > 0 {
		value := encodeTimeIDCursor(items[len(items)-1].SubmittedAt, items[len(items)-1].ID)
		nextCursor = &value
	}
	counts := map[string]int{"new": 0, "read": 0, "resolved": 0, "dismissed": 0}
	countRows, err := s.queryUserRead(ctx, `SELECT status, COUNT(*) FROM viewer_feedback WHERE expires_at > CURRENT_TIMESTAMP GROUP BY status`)
	if err != nil {
		return ownerFeedbackPage{}, err
	}
	defer countRows.Close()
	for countRows.Next() {
		var state string
		var count int
		if err := countRows.Scan(&state, &count); err != nil {
			return ownerFeedbackPage{}, err
		}
		counts[state] = count
	}
	if err := countRows.Err(); err != nil {
		return ownerFeedbackPage{}, err
	}
	return ownerFeedbackPage{Items: items, PageInfo: notificationPageInfo{NextCursor: nextCursor, HasMore: hasMore}, StatusCounts: counts}, nil
}

func (s *Server) handleAdminViewerFeedbackRoute(w http.ResponseWriter, r *http.Request, user User) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/viewer-feedback/"), "/")
	if id == "" || strings.Contains(id, "/") {
		writeProductError(w, http.StatusNotFound, "feedback_not_found", "Viewer message not found.")
		return
	}
	if r.Method != http.MethodPatch {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PATCH for this endpoint.")
		return
	}
	var request ownerFeedbackUpdateRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Version != viewerFeedbackVersion || request.ExpectedRevision < 0 || (request.Status == nil && request.ResponseMessage == nil) {
		writeProductError(w, http.StatusBadRequest, "invalid_feedback_update", "This viewer message update is invalid.")
		return
	}
	policy := s.viewerFeedbackPolicyContext(r.Context())
	if request.Status != nil && !oneOf(*request.Status, "new", "read", "resolved", "dismissed") {
		writeProductError(w, http.StatusBadRequest, "invalid_feedback_status", "Choose a valid message status.")
		return
	}
	response := ""
	if request.ResponseMessage != nil {
		response = strings.TrimSpace(*request.ResponseMessage)
		if !policy.OwnerResponsesEnabled {
			writeProductError(w, http.StatusForbidden, "feedback_response_disabled", "Responses to viewers are disabled in server settings.")
			return
		}
		if response == "" || utf8.RuneCountInString(response) > 1000 {
			writeProductError(w, http.StatusBadRequest, "invalid_feedback_response", "Write a response of up to 1,000 characters.")
			return
		}
	}
	record, err := s.updateOwnerFeedbackContext(r.Context(), user, id, request.ExpectedRevision, request.Status, response, policy)
	if errors.Is(err, sql.ErrNoRows) {
		writeProductError(w, http.StatusNotFound, "feedback_not_found", "Viewer message not found.")
		return
	}
	if errors.Is(err, errFeedbackRevisionConflict) {
		writeProductError(w, http.StatusConflict, "feedback_conflict", "This viewer message changed elsewhere. Refresh and try again.")
		return
	}
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "feedback_update_failed", "Unable to update this viewer message.")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) updateOwnerFeedbackContext(ctx context.Context, user User, feedbackID string, expectedRevision int64, requestedStatus *string, response string, policy viewerFeedbackPolicy) (ownerFeedbackRecord, error) {
	serverID, err := s.profileDirectoryServerIDContext(ctx)
	if err != nil {
		return ownerFeedbackRecord{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, nil, func(tx *sql.Tx) error {
		var accountID, profileID, authority, currentStatus, feedbackExpiresAt string
		var currentRevision int64
		if err := tx.QueryRow(`SELECT account_id, profile_id, authority, status, revision, expires_at FROM viewer_feedback WHERE id = ? AND expires_at > ?`, feedbackID, now).Scan(&accountID, &profileID, &authority, &currentStatus, &currentRevision, &feedbackExpiresAt); err != nil {
			return err
		}
		if currentRevision != expectedRevision {
			return errFeedbackRevisionConflict
		}
		status := currentStatus
		if requestedStatus != nil {
			status = *requestedStatus
		}
		ownerResponse, respondedBy, respondedAt := "", "", ""
		if response != "" {
			ownerResponse, respondedBy, respondedAt, status = response, accountIDForUser(user), now, "resolved"
		}
		result, err := tx.Exec(`UPDATE viewer_feedback SET status = ?, owner_response = CASE WHEN ? <> '' THEN ? ELSE owner_response END, responded_by_account_id = CASE WHEN ? <> '' THEN ? ELSE responded_by_account_id END, responded_at = CASE WHEN ? <> '' THEN ? ELSE responded_at END, revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ?`,
			status, ownerResponse, ownerResponse, respondedBy, respondedBy, respondedAt, respondedAt, now, feedbackID, expectedRevision)
		if err != nil {
			return err
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return errFeedbackRevisionConflict
		}
		if response != "" {
			recipient := notificationRecipient{Authority: authority, AccountID: accountID, ServerID: serverID, Audience: "profile", ProfileID: profileID}
			notificationExpires := time.Now().UTC().Add(time.Duration(policy.NotificationRetentionDays) * 24 * time.Hour)
			if feedbackExpires, parseErr := time.Parse(time.RFC3339Nano, feedbackExpiresAt); parseErr == nil && feedbackExpires.Before(notificationExpires) {
				notificationExpires = feedbackExpires
			}
			return createViewerNotificationTx(tx, recipient, "feedback.updated", "informational", "notification.feedback-updated", "status.feedback",
				map[string]string{}, &viewerNotificationContent{Body: response}, []viewerNotificationAction{{ID: "view-feedback", LabelMessageID: "action.open", Kind: "navigate", Target: "notifications", Parameters: map[string]string{}}}, feedbackID,
				notificationExpires)
		}
		return nil
	})
	if err != nil {
		return ownerFeedbackRecord{}, err
	}
	return s.ownerFeedbackRecordContext(ctx, feedbackID)
}

func (s *Server) ownerFeedbackRecordContext(ctx context.Context, feedbackID string) (ownerFeedbackRecord, error) {
	var item ownerFeedbackRecord
	var diagnosticsRaw, ownerResponse, respondedAt string
	err := s.queryUserRow(ctx, `
		SELECT feedback.id, feedback.authority,
		       CASE WHEN feedback.authority = 'local' THEN feedback.account_id ELSE '' END,
		       CASE WHEN feedback.authority = 'hosted' THEN account.portico_membership_id ELSE '' END,
		       account.display_name,
		       feedback.kind, feedback.category, feedback.message, feedback.diagnostics_json, feedback.status,
		       feedback.duplicate_count, feedback.owner_response, feedback.responded_at,
		       feedback.created_at, feedback.updated_at, feedback.revision
		FROM viewer_feedback feedback JOIN users account ON account.id = feedback.account_id
		WHERE feedback.id = ? AND feedback.expires_at > CURRENT_TIMESTAMP`, feedbackID).Scan(
		&item.ID, &item.Reporter.Authority, &item.Reporter.AccountID, &item.Reporter.MembershipID, &item.Reporter.AccountName,
		&item.Kind, &item.Category, &item.Message, &diagnosticsRaw, &item.Status, &item.DuplicateCount,
		&ownerResponse, &respondedAt, &item.SubmittedAt, &item.UpdatedAt, &item.Revision)
	if err != nil {
		return ownerFeedbackRecord{}, err
	}
	if err := json.Unmarshal([]byte(diagnosticsRaw), &item.Diagnostics); err != nil {
		return ownerFeedbackRecord{}, err
	}
	if ownerResponse != "" {
		item.OwnerResponse = &ownerFeedbackResponse{Message: ownerResponse, RespondedAt: respondedAt}
	}
	return item, nil
}

func (s *Server) handleAdminViewerNotices(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Server owner access is required.")
		return
	}
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var request ownerNoticeRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Message = strings.TrimSpace(request.Message)
	if request.Message == "" || utf8.RuneCountInString(request.Message) > 1000 || !oneOf(request.Audience, "profile", "account-admin") {
		writeProductError(w, http.StatusBadRequest, "invalid_notification", "Choose a recipient and write a message of up to 1,000 characters.")
		return
	}
	if request.Severity == "" {
		request.Severity = "informational"
	}
	if !oneOf(request.Severity, "informational", "warning", "error") {
		writeProductError(w, http.StatusBadRequest, "invalid_notification", "Choose a valid message importance.")
		return
	}
	if (request.Audience == "profile" && strings.TrimSpace(request.AccountID) != "") || (request.Audience == "account-admin" && strings.TrimSpace(request.ProfileID) != "") {
		writeProductError(w, http.StatusBadRequest, "invalid_notification", "Choose one recipient type for this notice.")
		return
	}
	serverID, err := s.profileDirectoryServerIDContext(r.Context())
	if err != nil {
		writeProductError(w, http.StatusInternalServerError, "notification_create_failed", "Unable to send this notice.")
		return
	}
	recipient := notificationRecipient{Authority: "local", ServerID: serverID, Audience: request.Audience}
	if request.Audience == "profile" {
		if request.ProfileID == "" {
			writeProductError(w, http.StatusBadRequest, "invalid_notification", "Choose a viewing profile.")
			return
		}
		if err := s.queryUserRow(r.Context(), `
			SELECT profile.account_id
			FROM profiles profile JOIN users account ON account.id = profile.account_id
			WHERE profile.id = ? AND profile.origin = 'local' AND profile.disabled_at = ''
			  AND COALESCE(account.auth_origin, 'local') <> 'portico' AND COALESCE(account.disabled_at, '') = ''`, request.ProfileID).Scan(&recipient.AccountID); err != nil {
			writeProductError(w, http.StatusNotFound, "profile_not_found", "Profile not found.")
			return
		}
		recipient.ProfileID = request.ProfileID
	} else {
		recipient.AccountID = strings.TrimSpace(request.AccountID)
		if recipient.AccountID == "" {
			writeProductError(w, http.StatusBadRequest, "invalid_notification", "Choose an account.")
			return
		}
		var origin string
		if err := s.queryUserRow(r.Context(), `SELECT auth_origin FROM users WHERE id = ? AND COALESCE(disabled_at, '') = ''`, recipient.AccountID).Scan(&origin); err != nil {
			writeProductError(w, http.StatusNotFound, "account_not_found", "Account not found.")
			return
		}
		if origin == "portico" {
			recipient.Authority = "hosted"
		}
	}
	policy := s.viewerFeedbackPolicyContext(r.Context())
	serverName := s.serverFriendlyNameContext(r.Context())
	var created viewerNotification
	err = s.withUserTxTagged(r.Context(), nil, func(tx *sql.Tx) error {
		return createViewerNotificationTx(tx, recipient, "server.message", request.Severity, "notification.server-message", "status.notification",
			map[string]string{}, &viewerNotificationContent{Title: serverName, Body: request.Message}, []viewerNotificationAction{}, "",
			time.Now().UTC().Add(time.Duration(policy.NotificationRetentionDays)*24*time.Hour))
	})
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "notification_create_failed", "Unable to send this notice.")
		return
	}
	s.notifyLongPollNotification(recipient)
	page, err := s.listViewerNotificationsContext(r.Context(), recipient, 1, "", "", true)
	if err == nil && len(page.Items) > 0 {
		created = page.Items[0]
	}
	s.recordAudit(r, user, "viewer.notification_sent", "profile", recipient.ProfileID, "info", map[string]string{"audience": recipient.Audience})
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleAdminViewerNotificationRecipients(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Server owner access is required.")
		return
	}
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	directory, err := s.ownerNotificationRecipientDirectoryContext(r.Context())
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "notification_recipients_failed", "Unable to load message recipients.")
		return
	}
	writeJSON(w, http.StatusOK, directory)
}

func (s *Server) ownerNotificationRecipientDirectoryContext(ctx context.Context) (ownerNotificationRecipientDirectory, error) {
	directory := ownerNotificationRecipientDirectory{
		Profiles:      []ownerNotificationProfileRecipient{},
		AccountAdmins: []ownerNotificationAccountAdminRecipient{},
	}
	profileRows, err := s.queryUserRead(ctx, `
		SELECT profile.id, profile.account_id, account.display_name, profile.display_name
		FROM profiles profile
		JOIN users account ON account.id = profile.account_id
		WHERE profile.origin = 'local' AND profile.disabled_at = ''
		  AND COALESCE(account.auth_origin, 'local') <> 'portico'
		  AND COALESCE(account.disabled_at, '') = ''
		ORDER BY LOWER(account.display_name), account.id, profile.sort_order, LOWER(profile.display_name), profile.id`)
	if err != nil {
		return directory, err
	}
	for profileRows.Next() {
		item := ownerNotificationProfileRecipient{Authority: "local", Audience: "profile"}
		if err := profileRows.Scan(&item.ProfileID, &item.AccountID, &item.AccountName, &item.ProfileName); err != nil {
			_ = profileRows.Close()
			return directory, err
		}
		directory.Profiles = append(directory.Profiles, item)
	}
	if err := profileRows.Err(); err != nil {
		_ = profileRows.Close()
		return directory, err
	}
	if err := profileRows.Close(); err != nil {
		return directory, err
	}

	accountRows, err := s.queryUserRead(ctx, `
		SELECT id, CASE WHEN auth_origin = 'portico' THEN 'hosted' ELSE 'local' END, display_name
		FROM users
		WHERE COALESCE(disabled_at, '') = ''
		ORDER BY LOWER(display_name), id`)
	if err != nil {
		return directory, err
	}
	defer accountRows.Close()
	for accountRows.Next() {
		item := ownerNotificationAccountAdminRecipient{Audience: "account-admin"}
		if err := accountRows.Scan(&item.AccountID, &item.Authority, &item.AccountName); err != nil {
			return directory, err
		}
		directory.AccountAdmins = append(directory.AccountAdmins, item)
	}
	return directory, accountRows.Err()
}

func createViewerNotificationTx(tx *sql.Tx, recipient notificationRecipient, kind, severity, messageID, iconID string, interpolation map[string]string, content *viewerNotificationContent, actions []viewerNotificationAction, feedbackID string, expires time.Time) error {
	if err := validateViewerNotificationActions(actions); err != nil {
		return err
	}
	interpolationRaw, err := json.Marshal(interpolation)
	if err != nil {
		return err
	}
	actionsRaw, err := json.Marshal(actions)
	if err != nil {
		return err
	}
	contentRaw := []byte("{}")
	if content != nil {
		contentRaw, err = json.Marshal(content)
		if err != nil {
			return err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.Exec(`
		INSERT INTO viewer_notifications (
			id, authority, account_id, server_id, profile_id, audience, kind, severity, message_id, icon_id,
			interpolation_json, content_json, actions_json, source_feedback_id, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		randomID("notification"), recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience,
		kind, severity, messageID, iconID, string(interpolationRaw), string(contentRaw), string(actionsRaw), feedbackID, now, expires.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return bumpNotificationRevisionTx(tx, recipient, now)
}

type viewerNotificationActionRule struct {
	Kind     string
	Label    string
	Required []string
	Optional []string
}

var viewerNotificationActionRules = map[string]viewerNotificationActionRule{
	"account.security":         {Kind: "navigate", Label: "action.open"},
	"downloads":                {Kind: "navigate", Label: "action.open", Optional: []string{"downloadId"}},
	"dvr.conflicts":            {Kind: "navigate", Label: "action.open", Optional: []string{"recordingId"}},
	"feedback.detail":          {Kind: "navigate", Label: "action.open", Required: []string{"feedbackId"}},
	"media.detail":             {Kind: "navigate", Label: "action.open", Required: []string{"mediaId"}},
	"notifications":            {Kind: "navigate", Label: "action.open"},
	"download.retry":           {Kind: "command", Label: "action.retry", Required: []string{"downloadId"}},
	"notification.archive":     {Kind: "command", Label: "action.archive", Required: []string{"notificationId"}},
	"notification.mark-read":   {Kind: "command", Label: "action.mark-read", Required: []string{"notificationId"}},
	"notification.mark-unread": {Kind: "command", Label: "action.mark-unread", Required: []string{"notificationId"}},
}

func validateViewerNotificationActions(actions []viewerNotificationAction) error {
	if len(actions) > 3 {
		return errors.New("notification has too many actions")
	}
	seen := map[string]bool{}
	for _, action := range actions {
		rule, ok := viewerNotificationActionRules[action.Target]
		if !ok || action.Kind != rule.Kind || action.LabelMessageID != rule.Label || !boundedIdentifier(action.ID, 64) || seen[action.ID] {
			return errors.New("notification action is not in the portable allowlist")
		}
		seen[action.ID] = true
		allowed := map[string]bool{}
		for _, key := range rule.Required {
			allowed[key] = true
			if !boundedText(action.Parameters[key], 128, false) {
				return errors.New("notification action is missing a required parameter")
			}
		}
		for _, key := range rule.Optional {
			allowed[key] = true
		}
		for key, value := range action.Parameters {
			if !allowed[key] || !boundedText(value, 128, false) {
				return errors.New("notification action parameter is invalid")
			}
		}
	}
	return nil
}

func (s *Server) viewerFeedbackPolicyContext(ctx context.Context) viewerFeedbackPolicy {
	policy := viewerFeedbackPolicy{Enabled: true, OwnerResponsesEnabled: true, FeedbackRetentionDays: defaultFeedbackRetentionDays, NotificationRetentionDays: defaultNotificationRetentionDays}
	settings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return policy
	}
	group, _ := settings["viewerFeedback"].(map[string]any)
	if value, ok := group["enabled"].(bool); ok {
		policy.Enabled = value
	}
	if value, ok := group["ownerResponsesEnabled"].(bool); ok {
		policy.OwnerResponsesEnabled = value
	}
	if value, ok := numericSetting(group["feedbackRetentionDays"]); ok {
		policy.FeedbackRetentionDays = clampViewerRetention(value)
	}
	if value, ok := numericSetting(group["notificationRetentionDays"]); ok {
		policy.NotificationRetentionDays = clampViewerRetention(value)
	}
	return policy
}

func numericSetting(value any) (int, bool) {
	switch number := value.(type) {
	case float64:
		return int(number), true
	case int:
		return number, true
	case json.Number:
		parsed, err := strconv.Atoi(string(number))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func clampViewerRetention(value int) int {
	if value < 7 {
		return 7
	}
	if value > 730 {
		return 730
	}
	return value
}

func (s *Server) pruneViewerCommunicationContext(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withUserTxTagged(ctx, nil, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT DISTINCT authority, account_id, server_id, profile_id, audience
			FROM viewer_notifications
			WHERE expires_at <= ? OR source_feedback_id IN (SELECT id FROM viewer_feedback WHERE expires_at <= ?)`, now, now)
		if err != nil {
			return err
		}
		recipients := []notificationRecipient{}
		for rows.Next() {
			var recipient notificationRecipient
			if err := rows.Scan(&recipient.Authority, &recipient.AccountID, &recipient.ServerID, &recipient.ProfileID, &recipient.Audience); err != nil {
				rows.Close()
				return err
			}
			recipients = append(recipients, recipient)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM viewer_notifications WHERE expires_at <= ? OR source_feedback_id IN (SELECT id FROM viewer_feedback WHERE expires_at <= ?)`, now, now); err != nil {
			return err
		}
		for _, recipient := range recipients {
			if err := bumpNotificationRevisionTx(tx, recipient, now); err != nil {
				return err
			}
		}
		for _, statement := range []string{
			`DELETE FROM viewer_feedback WHERE expires_at <= ?`,
			`DELETE FROM local_profile_admin_proofs WHERE expires_at <= ?`,
			`DELETE FROM automatic_profile_selection_trusts WHERE expires_at <= ? OR revoked_at <> ''`,
		} {
			if _, err := tx.Exec(statement, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func boundedIdentifier(value string, maximum int) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= maximum && feedbackIdentifierPattern.MatchString(value)
}

func boundedIdentifierOptional(value string, maximum int) bool {
	return strings.TrimSpace(value) == "" || boundedIdentifier(value, maximum)
}

func boundedText(value string, maximum int, optional bool) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return optional
	}
	return utf8.RuneCountInString(value) <= maximum && !strings.ContainsAny(value, "\r\n\x00")
}
