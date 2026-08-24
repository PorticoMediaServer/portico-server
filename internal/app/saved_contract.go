package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	savedDefaultLimit               = 50
	savedMaximumLimit               = 200
	savedShareCandidateDefaultLimit = 20
	savedShareCandidateMaximumLimit = 50
	savedShareCandidateMaximumQuery = 80
)

var errSavedRevisionConflict = errors.New("saved resource revision conflict")

type SavedResourceShare struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	CanEdit     bool   `json:"canEdit"`
}

type SavedResourceShareRequest struct {
	UserID  string `json:"userId"`
	CanEdit bool   `json:"canEdit"`
}

// SavedShareCandidate is deliberately narrower than User. Sharing controls may
// disclose that another active member exists, but they must not disclose that
// member's login, email address, permissions, restrictions, or auth metadata.
type SavedShareCandidate struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
}

type SavedShareCandidatePage struct {
	Items   []SavedShareCandidate `json:"items"`
	HasMore bool                  `json:"hasMore"`
}

type CollectionSummary struct {
	ID          string `json:"id"`
	OwnerUserID string `json:"ownerUserId"`
	Title       string `json:"title"`
	Summary     string `json:"summary,omitempty"`
	Visibility  string `json:"visibility"`
	CanEdit     bool   `json:"canEdit"`
	ItemCount   int    `json:"itemCount"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type Collection struct {
	CollectionSummary
	Shares []SavedResourceShare `json:"shares"`
}

type PlaylistSummary struct {
	ID          string `json:"id"`
	OwnerUserID string `json:"ownerUserId"`
	Title       string `json:"title"`
	Summary     string `json:"summary,omitempty"`
	Visibility  string `json:"visibility"`
	CanEdit     bool   `json:"canEdit"`
	ItemCount   int    `json:"itemCount"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type SavedPlaylist struct {
	PlaylistSummary
	Shares []SavedResourceShare `json:"shares"`
}

type CollectionPage struct {
	Items    []CollectionSummary `json:"items"`
	PageInfo CursorPageInfo      `json:"pageInfo"`
}

type PlaylistPage struct {
	Items    []PlaylistSummary `json:"items"`
	PageInfo CursorPageInfo    `json:"pageInfo"`
}

type SavedMediaPage struct {
	Items    []MediaCard    `json:"items"`
	PageInfo CursorPageInfo `json:"pageInfo"`
}

type PlaylistEntry struct {
	EntryID  string    `json:"entryId"`
	Media    MediaCard `json:"media"`
	Position int       `json:"position"`
}

type PlaylistEntryPage struct {
	Items    []PlaylistEntry `json:"items"`
	PageInfo CursorPageInfo  `json:"pageInfo"`
}

type CollectionCreateRequest struct {
	Title      string                      `json:"title"`
	Summary    string                      `json:"summary,omitempty"`
	Visibility string                      `json:"visibility,omitempty"`
	Shares     []SavedResourceShareRequest `json:"shares,omitempty"`
	MediaIDs   []string                    `json:"mediaIds,omitempty"`
}

type PlaylistCreateRequest struct {
	Title      string                      `json:"title"`
	Summary    string                      `json:"summary,omitempty"`
	Visibility string                      `json:"visibility,omitempty"`
	Shares     []SavedResourceShareRequest `json:"shares,omitempty"`
	MediaIDs   []string                    `json:"mediaIds,omitempty"`
}

type CollectionUpdateRequest struct {
	Title      *string                      `json:"title,omitempty"`
	Summary    *string                      `json:"summary,omitempty"`
	Visibility *string                      `json:"visibility,omitempty"`
	Shares     *[]SavedResourceShareRequest `json:"shares,omitempty"`
}

type PlaylistUpdateRequest struct {
	Title      *string                      `json:"title,omitempty"`
	Summary    *string                      `json:"summary,omitempty"`
	Visibility *string                      `json:"visibility,omitempty"`
	Shares     *[]SavedResourceShareRequest `json:"shares,omitempty"`
}

type CollectionMembershipBatchRequest struct {
	AddMediaIDs       []string `json:"addMediaIds,omitempty"`
	RemoveMediaIDs    []string `json:"removeMediaIds,omitempty"`
	ExpectedUpdatedAt string   `json:"expectedUpdatedAt,omitempty"`
}

type PlaylistItemsBatchRequest struct {
	AddMediaIDs       []string `json:"addMediaIds,omitempty"`
	RemoveEntryIDs    []string `json:"removeEntryIds,omitempty"`
	OrderEntryIDs     []string `json:"orderEntryIds,omitempty"`
	ExpectedUpdatedAt string   `json:"expectedUpdatedAt,omitempty"`
}

type CollectionMembershipBatchResponse struct {
	Collection Collection `json:"collection"`
	Added      int        `json:"added"`
	Removed    int        `json:"removed"`
	Unchanged  int        `json:"unchanged"`
}

type PlaylistItemsBatchResponse struct {
	Playlist  SavedPlaylist `json:"playlist"`
	Added     int           `json:"added"`
	Removed   int           `json:"removed"`
	Unchanged int           `json:"unchanged"`
}

type SavedView struct {
	ID           string             `json:"id"`
	OwnerUserID  string             `json:"ownerUserId"`
	ProfileID    string             `json:"-"`
	Title        string             `json:"title"`
	LibraryID    string             `json:"libraryId"`
	LibraryName  string             `json:"libraryName"`
	Pivot        string             `json:"pivot"`
	Query        *BrowseExpression  `json:"query,omitempty"`
	Sort         []BrowseSort       `json:"sort"`
	Presentation BrowsePresentation `json:"presentation"`
	IsPinned     bool               `json:"isPinned"`
	CreatedAt    string             `json:"createdAt"`
	UpdatedAt    string             `json:"updatedAt"`
}

type SavedViewPage struct {
	Items    []SavedView    `json:"items"`
	PageInfo CursorPageInfo `json:"pageInfo"`
}

type SavedViewCreateRequest struct {
	Title        string             `json:"title"`
	LibraryID    string             `json:"libraryId"`
	Pivot        string             `json:"pivot"`
	Query        *BrowseExpression  `json:"query,omitempty"`
	Sort         []BrowseSort       `json:"sort,omitempty"`
	Presentation BrowsePresentation `json:"presentation,omitempty"`
	IsPinned     bool               `json:"isPinned,omitempty"`
}

type SavedViewUpdateRequest = SavedViewCreateRequest

type SavedViewBrowseRequest struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func (s *Server) handleSavedShareCandidates(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) > savedShareCandidateMaximumQuery {
		writeError(w, http.StatusBadRequest, "invalid_share_candidate_query", fmt.Sprintf("Search text must be at most %d characters.", savedShareCandidateMaximumQuery))
		return
	}
	limit := savedShareCandidateDefaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > savedShareCandidateMaximumLimit {
			writeError(w, http.StatusBadRequest, "invalid_share_candidate_limit", fmt.Sprintf("Limit must be between 1 and %d.", savedShareCandidateMaximumLimit))
			return
		}
		limit = parsed
	}
	page, err := s.listSavedShareCandidates(r.Context(), user.ID, query, limit, s.porticoAccountMode())
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "share_candidates_failed", "Unable to load people available for sharing.")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) listSavedShareCandidates(ctx context.Context, currentUserID, query string, limit int, porticoMode bool) (SavedShareCandidatePage, error) {
	where := savedShareCandidateEligibilitySQL(porticoMode)
	args := []any{strings.TrimSpace(currentUserID)}
	query = strings.TrimSpace(query)
	if query != "" {
		where += ` AND (
			lower(COALESCE(NULLIF(u.username, ''), u.display_name)) LIKE ? ESCAPE '\'
			OR lower(u.display_name) LIKE ? ESCAPE '\'
		)`
		pattern := "%" + escapeSQLLike(strings.ToLower(query)) + "%"
		args = append(args, pattern, pattern)
	}
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT u.id, COALESCE(NULLIF(u.username, ''), u.display_name)
		FROM users u
		WHERE u.id <> ? AND `+where+`
		ORDER BY lower(COALESCE(NULLIF(u.username, ''), u.display_name)), u.id
		LIMIT ?`, args...)
	if err != nil {
		return SavedShareCandidatePage{}, err
	}
	defer rows.Close()
	items := make([]SavedShareCandidate, 0, limit+1)
	for rows.Next() {
		var item SavedShareCandidate
		if err := rows.Scan(&item.UserID, &item.DisplayName); err != nil {
			return SavedShareCandidatePage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return SavedShareCandidatePage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return SavedShareCandidatePage{Items: items, HasMore: hasMore}, nil
}

func savedShareCandidateEligibilitySQL(porticoMode bool) string {
	base := `COALESCE(u.disabled_at, '') = '' AND COALESCE(u.auth_origin, 'local') <> 'portico_deleted'`
	if porticoMode {
		return base + ` AND COALESCE(u.auth_origin, 'local') = 'portico' AND EXISTS (
			SELECT 1 FROM remote_access_members ram
			WHERE ram.local_user_id = u.id AND ram.status = 'active'
				AND (COALESCE(u.portico_membership_id, '') = '' OR ram.portico_membership_id = u.portico_membership_id)
		)`
	}
	return base + ` AND COALESCE(u.auth_origin, 'local') = 'local' AND u.password_hash IS NOT NULL AND u.password_hash <> ''`
}

type savedResourceCursor struct {
	UpdatedAt string `json:"updatedAt"`
	ID        string `json:"id"`
}

type savedMembershipCursor struct {
	SortOrder int    `json:"sortOrder"`
	MediaID   string `json:"mediaId"`
}

type playlistEntryCursor struct {
	SortOrder int    `json:"sortOrder"`
	EntryID   string `json:"entryId"`
}

func (s *Server) handleCollections(w http.ResponseWriter, r *http.Request, user User) {
	s.handleSavedResource(w, r, user, "collection")
}

func (s *Server) handleSavedPlaylists(w http.ResponseWriter, r *http.Request, user User) {
	s.handleSavedResource(w, r, user, "playlist")
}

func (s *Server) handleSavedResource(w http.ResponseWriter, r *http.Request, user User, kind string) {
	base := "/api/playlists"
	label := "Playlist"
	if kind == "collection" {
		base = "/api/collections"
		label = "Collection"
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, base), "/")
	if path == "" {
		s.handleSavedResourceRoot(w, r, user, kind, label)
		return
	}
	parts := strings.Split(path, "/")
	resourceID := strings.TrimSpace(parts[0])
	if resourceID == "" {
		writeError(w, http.StatusNotFound, "not_found", label+" was not found.")
		return
	}
	if len(parts) == 1 {
		s.handleSavedResourceDetail(w, r, user, kind, label, resourceID)
		return
	}
	if len(parts) == 2 && parts[1] == "items" && r.Method == http.MethodGet {
		s.handleSavedResourceItems(w, r, user, kind, label, resourceID)
		return
	}
	if len(parts) == 2 && kind == "collection" && parts[1] == "memberships:batch" && r.Method == http.MethodPost {
		var request CollectionMembershipBatchRequest
		if !decodeSavedRequest(w, r, &request) {
			return
		}
		result, err := s.mutateSavedResourceMemberships(r.Context(), user, kind, resourceID, request.AddMediaIDs, request.RemoveMediaIDs, nil, request.ExpectedUpdatedAt)
		if err != nil {
			writeSavedMutationError(w, err, label)
			return
		}
		collection, err := s.getCollection(r.Context(), user, resourceID)
		if err != nil {
			writeSavedMutationError(w, err, label)
			return
		}
		s.recordAudit(r, user, "collection.memberships_updated", "collection", resourceID, "info", map[string]string{"added": strconv.Itoa(result.added), "removed": strconv.Itoa(result.removed)})
		writeJSON(w, http.StatusOK, CollectionMembershipBatchResponse{Collection: collection, Added: result.added, Removed: result.removed, Unchanged: result.unchanged})
		return
	}
	if len(parts) == 2 && kind == "playlist" && parts[1] == "items:batch" && r.Method == http.MethodPost {
		var request PlaylistItemsBatchRequest
		if !decodeSavedRequest(w, r, &request) {
			return
		}
		var order *[]string
		if request.OrderEntryIDs != nil {
			request.OrderEntryIDs = normalizePublicResourceIDs(request.OrderEntryIDs)
			order = &request.OrderEntryIDs
		}
		request.RemoveEntryIDs = normalizePublicResourceIDs(request.RemoveEntryIDs)
		result, err := s.mutateSavedResourceMemberships(r.Context(), user, kind, resourceID, request.AddMediaIDs, request.RemoveEntryIDs, order, request.ExpectedUpdatedAt)
		if err != nil {
			writeSavedMutationError(w, err, label)
			return
		}
		playlist, err := s.getSavedPlaylist(r.Context(), user, resourceID)
		if err != nil {
			writeSavedMutationError(w, err, label)
			return
		}
		s.recordAudit(r, user, "playlist.items_updated", "playlist", resourceID, "info", map[string]string{"added": strconv.Itoa(result.added), "removed": strconv.Itoa(result.removed)})
		writeJSON(w, http.StatusOK, PlaylistItemsBatchResponse{Playlist: playlist, Added: result.added, Removed: result.removed, Unchanged: result.unchanged})
		return
	}
	writeError(w, http.StatusNotFound, "not_found", label+" route was not found.")
}

func (s *Server) handleSavedResourceRoot(w http.ResponseWriter, r *http.Request, user User, kind, label string) {
	switch r.Method {
	case http.MethodGet:
		limit, ok := savedLimitFromRequest(w, r)
		if !ok {
			return
		}
		libraryID := strings.TrimSpace(r.URL.Query().Get("libraryId"))
		if libraryID != "" {
			allowed, err := s.libraryAccessAllowedContext(r.Context(), user, libraryID)
			if err != nil {
				writeDatabaseAccessError(w, err, http.StatusInternalServerError, "library_access_failed", "Unable to inspect library access.")
				return
			}
			if !allowed {
				writeError(w, http.StatusNotFound, "library_not_found", "Library was not found.")
				return
			}
			if _, err := s.getCanonicalLibraryScopeContext(r.Context(), libraryID); err != nil {
				writeSavedDetailError(w, err, "Library")
				return
			}
		}
		items, nextCursor, hasMore, err := s.listSavedResourcesPage(r.Context(), user, kind, libraryID, strings.TrimSpace(r.URL.Query().Get("cursor")), limit, time.Now().UTC())
		if err != nil {
			writeSavedCursorOrDatabaseError(w, err, strings.ToLower(label)+"_list_failed", "Unable to load "+strings.ToLower(label)+"s.")
			return
		}
		pageInfo := CursorPageInfo{NextCursor: nextCursor, HasMore: hasMore}
		if kind == "collection" {
			collections := make([]CollectionSummary, 0, len(items))
			for _, item := range items {
				collections = append(collections, collectionSummary(item))
			}
			writeJSON(w, http.StatusOK, CollectionPage{Items: collections, PageInfo: pageInfo})
		} else {
			playlists := make([]PlaylistSummary, 0, len(items))
			for _, item := range items {
				playlists = append(playlists, playlistSummary(item))
			}
			writeJSON(w, http.StatusOK, PlaylistPage{Items: playlists, PageInfo: pageInfo})
		}
	case http.MethodPost:
		if kind == "collection" {
			var request CollectionCreateRequest
			if !decodeSavedRequest(w, r, &request) {
				return
			}
			resource, err := s.createCanonicalSavedResource(r.Context(), user, kind, request.Title, request.Summary, request.Visibility, request.Shares, request.MediaIDs)
			if err != nil {
				writeSavedMutationError(w, err, label)
				return
			}
			collection, err := s.getCollection(r.Context(), user, resource.ID)
			if err != nil {
				writeSavedMutationError(w, err, label)
				return
			}
			s.recordAudit(r, user, "collection.created", "collection", resource.ID, "info", map[string]string{"title": resource.Title})
			writeJSON(w, http.StatusCreated, collection)
			return
		}
		var request PlaylistCreateRequest
		if !decodeSavedRequest(w, r, &request) {
			return
		}
		resource, err := s.createCanonicalSavedResource(r.Context(), user, kind, request.Title, request.Summary, request.Visibility, request.Shares, request.MediaIDs)
		if err != nil {
			writeSavedMutationError(w, err, label)
			return
		}
		playlist, err := s.getSavedPlaylist(r.Context(), user, resource.ID)
		if err != nil {
			writeSavedMutationError(w, err, label)
			return
		}
		s.recordAudit(r, user, "playlist.created", "playlist", resource.ID, "info", map[string]string{"title": resource.Title})
		writeJSON(w, http.StatusCreated, playlist)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
	}
}

func (s *Server) handleSavedResourceDetail(w http.ResponseWriter, r *http.Request, user User, kind, label, resourceID string) {
	switch r.Method {
	case http.MethodGet:
		if kind == "collection" {
			collection, err := s.getCollection(r.Context(), user, resourceID)
			if err != nil {
				writeSavedDetailError(w, err, label)
				return
			}
			writeJSON(w, http.StatusOK, collection)
			return
		}
		playlist, err := s.getSavedPlaylist(r.Context(), user, resourceID)
		if err != nil {
			writeSavedDetailError(w, err, label)
			return
		}
		writeJSON(w, http.StatusOK, playlist)
	case http.MethodPatch:
		if kind == "collection" {
			var request CollectionUpdateRequest
			if !decodeSavedRequest(w, r, &request) {
				return
			}
			if err := s.updateCanonicalSavedResource(r.Context(), user, kind, resourceID, request.Title, request.Summary, request.Visibility, request.Shares); err != nil {
				writeSavedMutationError(w, err, label)
				return
			}
			collection, err := s.getCollection(r.Context(), user, resourceID)
			if err != nil {
				writeSavedMutationError(w, err, label)
				return
			}
			s.recordAudit(r, user, "collection.updated", "collection", resourceID, "info", map[string]string{"title": collection.Title})
			writeJSON(w, http.StatusOK, collection)
			return
		}
		var request PlaylistUpdateRequest
		if !decodeSavedRequest(w, r, &request) {
			return
		}
		if err := s.updateCanonicalSavedResource(r.Context(), user, kind, resourceID, request.Title, request.Summary, request.Visibility, request.Shares); err != nil {
			writeSavedMutationError(w, err, label)
			return
		}
		playlist, err := s.getSavedPlaylist(r.Context(), user, resourceID)
		if err != nil {
			writeSavedMutationError(w, err, label)
			return
		}
		s.recordAudit(r, user, "playlist.updated", "playlist", resourceID, "info", map[string]string{"title": playlist.Title})
		writeJSON(w, http.StatusOK, playlist)
	case http.MethodDelete:
		resource, err := s.getPlaylistContext(r.Context(), user, resourceID, false)
		if err != nil || resource.Kind != kind || resource.Smart {
			writeSavedDetailError(w, sql.ErrNoRows, label)
			return
		}
		if resource.ProfileID != viewerProfileID(user) && !canInteractivelyManageServer(user) {
			writeError(w, http.StatusForbidden, "forbidden", "You cannot delete this "+strings.ToLower(label)+".")
			return
		}
		if _, err := s.execUserWrite(r.Context(), `DELETE FROM playlists WHERE id = ?`, resourceID); err != nil {
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "saved_delete_failed", "Unable to delete the "+strings.ToLower(label)+".")
			return
		}
		s.recordAudit(r, user, kind+".deleted", kind, resourceID, "warn", map[string]string{"title": resource.Title})
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET, PATCH, or DELETE for this endpoint.")
	}
}

func (s *Server) handleSavedResourceItems(w http.ResponseWriter, r *http.Request, user User, kind, label, resourceID string) {
	limit, ok := savedLimitFromRequest(w, r)
	if !ok {
		return
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if kind == "playlist" {
		page, err := s.savedPlaylistEntriesPage(r.Context(), user, resourceID, cursor, limit, time.Now().UTC())
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeSavedDetailError(w, err, label)
				return
			}
			writeSavedCursorOrDatabaseError(w, err, "saved_items_failed", "Unable to load playlist entries.")
			return
		}
		writeJSON(w, http.StatusOK, page)
		return
	}
	page, err := s.savedResourceItemsPage(r.Context(), user, kind, resourceID, cursor, limit, time.Now().UTC())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeSavedDetailError(w, err, label)
			return
		}
		writeSavedCursorOrDatabaseError(w, err, "saved_items_failed", "Unable to load "+strings.ToLower(label)+" items.")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func savedLimitFromRequest(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return savedDefaultLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > savedMaximumLimit {
		writeError(w, http.StatusBadRequest, "invalid_limit", fmt.Sprintf("Limit must be between 1 and %d.", savedMaximumLimit))
		return 0, false
	}
	return limit, true
}

func decodeSavedRequest[T any](w http.ResponseWriter, r *http.Request, target *T) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Request body must be valid JSON with known fields.")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Request body must contain one JSON object.")
		return false
	}
	return true
}

func collectionSummary(resource Playlist) CollectionSummary {
	return CollectionSummary{
		ID: resource.ID, OwnerUserID: resource.UserID, Title: resource.Title, Summary: resource.Summary,
		Visibility: resource.Visibility, CanEdit: resource.CanEdit, ItemCount: resource.ItemCount,
		CreatedAt: resource.CreatedAt, UpdatedAt: resource.UpdatedAt,
	}
}

func playlistSummary(resource Playlist) PlaylistSummary {
	return PlaylistSummary{
		ID: resource.ID, OwnerUserID: resource.UserID, Title: resource.Title, Summary: resource.Summary,
		Visibility: resource.Visibility, CanEdit: resource.CanEdit, ItemCount: resource.ItemCount,
		CreatedAt: resource.CreatedAt, UpdatedAt: resource.UpdatedAt,
	}
}

func savedResourceShares(shares []PlaylistShare) []SavedResourceShare {
	result := make([]SavedResourceShare, 0, len(shares))
	for _, share := range shares {
		result = append(result, SavedResourceShare{UserID: share.UserID, DisplayName: share.DisplayName, Email: share.Email, CanEdit: share.CanEdit})
	}
	return result
}

func (s *Server) getCollection(ctx context.Context, user User, resourceID string) (Collection, error) {
	resource, err := s.getPlaylistContext(ctx, user, resourceID, false)
	if err != nil || resource.Kind != "collection" || resource.Smart {
		if err == nil {
			err = sql.ErrNoRows
		}
		return Collection{}, err
	}
	return Collection{CollectionSummary: collectionSummary(resource), Shares: savedResourceShares(resource.Shares)}, nil
}

func (s *Server) getSavedPlaylist(ctx context.Context, user User, resourceID string) (SavedPlaylist, error) {
	resource, err := s.getPlaylistContext(ctx, user, resourceID, false)
	if err != nil || resource.Kind != "playlist" || resource.Smart {
		if err == nil {
			err = sql.ErrNoRows
		}
		return SavedPlaylist{}, err
	}
	return SavedPlaylist{PlaylistSummary: playlistSummary(resource), Shares: savedResourceShares(resource.Shares)}, nil
}

func (s *Server) listSavedResourcesPage(ctx context.Context, user User, kind, libraryID, cursor string, limit int, now time.Time) ([]Playlist, *string, bool, error) {
	libraryID = strings.TrimSpace(libraryID)
	scope := "saved-resource-list:" + kind + ":library=" + libraryID
	var after savedResourceCursor
	if cursor != "" {
		if err := s.decodeContractCursor(cursor, scope, viewerProfileID(user), &after, now); err != nil {
			return nil, nil, false, err
		}
	}
	where := `WHERE p.kind = ? AND COALESCE(p.smart_filter_json, '{}') IN ('', '{}') AND (p.profile_id = ? OR p.visibility = 'server' OR ps_self.user_id IS NOT NULL)`
	args := []any{user.ID, kind, viewerProfileID(user)}
	if libraryID != "" {
		where += ` AND EXISTS (
			SELECT 1
			FROM playlist_items pi_scope
			JOIN media_items m_scope ON m_scope.id = pi_scope.media_id
			WHERE pi_scope.playlist_id = p.id AND m_scope.library_id = ?
		)`
		args = append(args, libraryID)
	}
	if after.UpdatedAt != "" {
		where += ` AND (p.updated_at < ? OR (p.updated_at = ? AND p.id < ?))`
		args = append(args, after.UpdatedAt, after.UpdatedAt, after.ID)
	}
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT p.id, p.user_id, p.profile_id, p.kind, p.title, p.summary, p.visibility,
			COUNT(pi.media_id), COALESCE(MAX(ps_self.can_edit), 0), p.created_at, p.updated_at
		FROM playlists p
		LEFT JOIN playlist_items pi ON pi.playlist_id = p.id
		LEFT JOIN playlist_shares ps_self ON ps_self.playlist_id = p.id AND ps_self.user_id = ?
		`+where+`
		GROUP BY p.id, p.user_id, p.profile_id, p.kind, p.title, p.summary, p.visibility, p.created_at, p.updated_at
		ORDER BY p.updated_at DESC, p.id DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, nil, false, err
	}
	defer rows.Close()
	resources := []Playlist{}
	for rows.Next() {
		var resource Playlist
		var sharedCanEdit int
		if err := rows.Scan(&resource.ID, &resource.UserID, &resource.ProfileID, &resource.Kind, &resource.Title, &resource.Summary, &resource.Visibility, &resource.ItemCount, &sharedCanEdit, &resource.CreatedAt, &resource.UpdatedAt); err != nil {
			return nil, nil, false, err
		}
		resource.CanEdit = resource.ProfileID == viewerProfileID(user) || canInteractivelyManageServer(user) || sharedCanEdit != 0
		resources = append(resources, resource)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, false, err
	}
	for index := range resources {
		if resources[index].ProfileID != viewerProfileID(user) && !canInteractivelyManageServer(user) {
			count, err := s.playlistVisibleItemCountContext(ctx, viewerProfileID(user), resources[index].ID)
			if err != nil {
				return nil, nil, false, err
			}
			resources[index].ItemCount = count
		}
	}
	hasMore := len(resources) > limit
	if hasMore {
		resources = resources[:limit]
	}
	var nextCursor *string
	if hasMore && len(resources) > 0 {
		last := resources[len(resources)-1]
		token, err := s.encodeContractCursor(scope, viewerProfileID(user), savedResourceCursor{UpdatedAt: last.UpdatedAt, ID: last.ID}, now)
		if err != nil {
			return nil, nil, false, err
		}
		nextCursor = &token
	}
	return resources, nextCursor, hasMore, nil
}

func normalizeSavedVisibility(user User, raw string) (string, error) {
	visibility := strings.ToLower(strings.TrimSpace(raw))
	if visibility == "" {
		visibility = "private"
	}
	if visibility != "private" && visibility != "server" {
		return "", errors.New("Visibility must be private or server.")
	}
	if visibility == "server" && !canInteractivelyManageServer(user) {
		return "", errors.New("Only server managers can create server-visible saved resources.")
	}
	return visibility, nil
}

func normalizeOptionalSavedMediaIDs(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	return normalizePlaylistBulkMediaIDs(raw)
}

func normalizeOptionalPlaylistEntryMediaIDs(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		if id := strings.TrimSpace(value); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("Choose at least one media item.")
	}
	if len(ids) > maxBulkMediaItems {
		return nil, fmt.Errorf("Bulk playlist requests are limited to %d media items.", maxBulkMediaItems)
	}
	return ids, nil
}

func normalizeOptionalPlaylistEntryIDs(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		id := strings.TrimSpace(value)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.New("Choose at least one playlist entry.")
	}
	if len(ids) > maxBulkMediaItems {
		return nil, fmt.Errorf("Bulk playlist requests are limited to %d entries.", maxBulkMediaItems)
	}
	return ids, nil
}

func normalizePublicResourceIDs(raw []string) []string {
	if len(raw) == 0 {
		return raw
	}
	resolved := make([]string, len(raw))
	for index, value := range raw {
		value = strings.TrimSpace(value)
		resolved[index] = value
	}
	return resolved
}

func (s *Server) validateSavedShares(ctx context.Context, ownerID string, raw []SavedResourceShareRequest) ([]SavedResourceShareRequest, error) {
	seen := map[string]bool{}
	result := []SavedResourceShareRequest{}
	eligibility := savedShareCandidateEligibilitySQL(s.porticoAccountMode())
	for _, share := range raw {
		userID := strings.TrimSpace(share.UserID)
		if userID == "" || userID == ownerID || seen[userID] {
			continue
		}
		var exists int
		if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM users u WHERE u.id = ? AND `+eligibility, userID).Scan(&exists); err != nil {
			return nil, err
		}
		if exists == 0 {
			return nil, errors.New("Share target is not an active server member.")
		}
		seen[userID] = true
		result = append(result, SavedResourceShareRequest{UserID: userID, CanEdit: share.CanEdit})
	}
	return result, nil
}

func insertSavedShares(tx *sql.Tx, resourceID string, shares []SavedResourceShareRequest, now string) error {
	for _, share := range shares {
		if _, err := tx.Exec(`
			INSERT INTO playlist_shares (playlist_id, user_id, can_edit, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)`, resourceID, share.UserID, boolInt(share.CanEdit), now, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) createCanonicalSavedResource(ctx context.Context, user User, kind, title, summary, visibility string, rawShares []SavedResourceShareRequest, rawMediaIDs []string) (Playlist, error) {
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 160 {
		return Playlist{}, errors.New("Title is required and must be at most 160 characters.")
	}
	visibility, err := normalizeSavedVisibility(user, visibility)
	if err != nil {
		return Playlist{}, err
	}
	shares, err := s.validateSavedShares(ctx, user.ID, rawShares)
	if err != nil {
		return Playlist{}, err
	}
	var mediaIDs []string
	if kind == "playlist" {
		mediaIDs, err = normalizeOptionalPlaylistEntryMediaIDs(rawMediaIDs)
	} else {
		mediaIDs, err = normalizeOptionalSavedMediaIDs(rawMediaIDs)
	}
	if err != nil {
		return Playlist{}, err
	}
	if len(mediaIDs) > 0 {
		items, err := s.mediaListItemsByOrderedIDsContext(ctx, viewerProfileID(user), mediaIDs)
		if err != nil {
			return Playlist{}, err
		}
		if len(items) != len(mediaIDs) {
			return Playlist{}, errors.New("One or more media items were not found or are not accessible.")
		}
	}
	id := randomOpaquePublicID()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, savedResourceEventTags(kind), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`
			INSERT INTO playlists (id, user_id, profile_id, kind, title, summary, visibility, smart_filter_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '{}', ?, ?)`, id, accountIDForUser(user), viewerProfileID(user), kind, title, strings.TrimSpace(summary), visibility, now, now); err != nil {
			return err
		}
		for index, mediaID := range mediaIDs {
			if _, err := tx.Exec(`INSERT INTO playlist_items (entry_id, playlist_id, media_id, sort_order, added_at) VALUES (?, ?, ?, ?, ?)`, randomOpaquePublicID(), id, mediaID, index+1, now); err != nil {
				return err
			}
		}
		return insertSavedShares(tx, id, shares, now)
	})
	if err != nil {
		return Playlist{}, err
	}
	return s.getPlaylistContext(ctx, user, id, false)
}

func (s *Server) updateCanonicalSavedResource(ctx context.Context, user User, kind, resourceID string, title, summary, visibility *string, rawShares *[]SavedResourceShareRequest) error {
	resource, err := s.getPlaylistContext(ctx, user, resourceID, false)
	if err != nil || resource.Kind != kind || resource.Smart {
		return sql.ErrNoRows
	}
	if resource.ProfileID != viewerProfileID(user) && !canInteractivelyManageServer(user) {
		return errors.New("Only the owner or a server manager can edit this resource.")
	}
	nextTitle := resource.Title
	if title != nil {
		nextTitle = strings.TrimSpace(*title)
		if nextTitle == "" || len([]rune(nextTitle)) > 160 {
			return errors.New("Title is required and must be at most 160 characters.")
		}
	}
	nextSummary := resource.Summary
	if summary != nil {
		nextSummary = strings.TrimSpace(*summary)
	}
	nextVisibility := resource.Visibility
	if visibility != nil {
		nextVisibility, err = normalizeSavedVisibility(user, *visibility)
		if err != nil {
			return err
		}
	}
	var shares []SavedResourceShareRequest
	if rawShares != nil {
		shares, err = s.validateSavedShares(ctx, resource.UserID, *rawShares)
		if err != nil {
			return err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withUserTxTagged(ctx, savedResourceEventTags(kind), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE playlists SET title = ?, summary = ?, visibility = ?, updated_at = ? WHERE id = ? AND kind = ?`, nextTitle, nextSummary, nextVisibility, now, resourceID, kind); err != nil {
			return err
		}
		if rawShares != nil {
			if _, err := tx.Exec(`DELETE FROM playlist_shares WHERE playlist_id = ?`, resourceID); err != nil {
				return err
			}
			if err := insertSavedShares(tx, resourceID, shares, now); err != nil {
				return err
			}
		}
		return nil
	})
}

type savedMutationResult struct {
	added     int
	removed   int
	unchanged int
}

func (s *Server) mutateSavedResourceMemberships(ctx context.Context, user User, kind, resourceID string, rawAdd, rawRemove []string, order *[]string, expectedUpdatedAt string) (savedMutationResult, error) {
	resource, err := s.getPlaylistContext(ctx, user, resourceID, false)
	if err != nil || resource.Kind != kind || resource.Smart {
		return savedMutationResult{}, sql.ErrNoRows
	}
	if !resource.CanEdit {
		return savedMutationResult{}, errors.New("You cannot edit this resource.")
	}
	var addIDs, removeIDs []string
	if kind == "playlist" {
		addIDs, err = normalizeOptionalPlaylistEntryMediaIDs(rawAdd)
		if err == nil {
			removeIDs, err = normalizeOptionalPlaylistEntryIDs(rawRemove)
		}
	} else {
		addIDs, err = normalizeOptionalSavedMediaIDs(rawAdd)
		if err == nil {
			removeIDs, err = normalizeOptionalSavedMediaIDs(rawRemove)
		}
	}
	if err != nil {
		return savedMutationResult{}, err
	}
	if len(addIDs) == 0 && len(removeIDs) == 0 && order == nil {
		return savedMutationResult{}, errors.New("Choose at least one membership change.")
	}
	if kind == "collection" {
		removeSet := stringSet(removeIDs)
		for _, id := range addIDs {
			if removeSet[id] {
				return savedMutationResult{}, errors.New("A media item cannot be added and removed in the same batch.")
			}
		}
	}
	if len(addIDs) > 0 {
		items, err := s.mediaListItemsByOrderedIDsContext(ctx, viewerProfileID(user), addIDs)
		if err != nil {
			return savedMutationResult{}, err
		}
		if len(items) != len(addIDs) {
			return savedMutationResult{}, errors.New("One or more media items were not found or are not accessible.")
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result := savedMutationResult{}
	err = s.withUserTxTagged(ctx, savedResourceEventTags(kind), func(tx *sql.Tx) error {
		var currentUpdatedAt string
		if err := tx.QueryRow(`SELECT updated_at FROM playlists WHERE id = ? AND kind = ?`, resourceID, kind).Scan(&currentUpdatedAt); err != nil {
			return err
		}
		if expected := strings.TrimSpace(expectedUpdatedAt); expected != "" && expected != currentUpdatedAt {
			return errSavedRevisionConflict
		}
		for _, removeID := range removeIDs {
			query := `DELETE FROM playlist_items WHERE playlist_id = ? AND media_id = ?`
			if kind == "playlist" {
				query = `DELETE FROM playlist_items WHERE playlist_id = ? AND entry_id = ?`
			}
			mutation, err := tx.Exec(query, resourceID, removeID)
			if err != nil {
				return err
			}
			if rowsAffected(mutation) > 0 {
				result.removed++
			} else {
				result.unchanged++
			}
		}
		var sortOrder int
		if err := tx.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) FROM playlist_items WHERE playlist_id = ?`, resourceID).Scan(&sortOrder); err != nil {
			return err
		}
		for _, mediaID := range addIDs {
			if kind == "collection" {
				var exists int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM playlist_items WHERE playlist_id = ? AND media_id = ?`, resourceID, mediaID).Scan(&exists); err != nil {
					return err
				}
				if exists > 0 {
					result.unchanged++
					continue
				}
			}
			sortOrder++
			mutation, err := tx.Exec(`INSERT INTO playlist_items (entry_id, playlist_id, media_id, sort_order, added_at) VALUES (?, ?, ?, ?, ?)`, randomOpaquePublicID(), resourceID, mediaID, sortOrder, now)
			if err != nil {
				return err
			}
			if rowsAffected(mutation) > 0 {
				result.added++
			} else {
				result.unchanged++
			}
		}
		if order != nil {
			if kind != "playlist" {
				return errors.New("Collections do not have a manual item order.")
			}
			requested, err := normalizeOptionalPlaylistEntryIDs(*order)
			if err != nil {
				return err
			}
			rows, err := tx.Query(`SELECT entry_id FROM playlist_items WHERE playlist_id = ? ORDER BY sort_order ASC, entry_id ASC`, resourceID)
			if err != nil {
				return err
			}
			existing := []string{}
			for rows.Next() {
				var entryID string
				if err := rows.Scan(&entryID); err != nil {
					_ = rows.Close()
					return err
				}
				existing = append(existing, entryID)
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				return err
			}
			if !sameStringSet(existing, requested) {
				return errors.New("orderEntryIds must contain every current playlist entry exactly once.")
			}
			for index, entryID := range requested {
				if _, err := tx.Exec(`UPDATE playlist_items SET sort_order = ? WHERE playlist_id = ? AND entry_id = ?`, index+1, resourceID, entryID); err != nil {
					return err
				}
			}
		}
		if err := normalizeSavedMembershipPositions(tx, resourceID); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE playlists SET updated_at = ? WHERE id = ?`, now, resourceID)
		return err
	})
	return result, err
}

func savedResourceEventTags(kind string) []string {
	if strings.EqualFold(strings.TrimSpace(kind), "collection") {
		return []string{"saved", "collections"}
	}
	return []string{"saved", "playlists"}
}

func normalizeSavedMembershipPositions(tx *sql.Tx, resourceID string) error {
	rows, err := tx.Query(`SELECT entry_id FROM playlist_items WHERE playlist_id = ? ORDER BY sort_order ASC, entry_id ASC`, resourceID)
	if err != nil {
		return err
	}
	entryIDs := []string{}
	for rows.Next() {
		var entryID string
		if err := rows.Scan(&entryID); err != nil {
			_ = rows.Close()
			return err
		}
		entryIDs = append(entryIDs, entryID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for index, entryID := range entryIDs {
		if _, err := tx.Exec(`UPDATE playlist_items SET sort_order = ? WHERE playlist_id = ? AND entry_id = ?`, index+1, resourceID, entryID); err != nil {
			return err
		}
	}
	return nil
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftSet := stringSet(left)
	rightSet := stringSet(right)
	if len(leftSet) != len(left) || len(rightSet) != len(right) {
		return false
	}
	for value := range rightSet {
		if !leftSet[value] {
			return false
		}
	}
	return true
}

func (s *Server) savedPlaylistEntriesPage(ctx context.Context, user User, resourceID, cursor string, limit int, now time.Time) (PlaylistEntryPage, error) {
	resource, err := s.getPlaylistContext(ctx, user, resourceID, false)
	if err != nil || resource.Kind != "playlist" || resource.Smart {
		return PlaylistEntryPage{}, sql.ErrNoRows
	}
	scope := "playlist-entries:" + resourceID
	var after playlistEntryCursor
	if cursor != "" {
		if err := s.decodeContractCursor(cursor, scope, viewerProfileID(user), &after, now); err != nil {
			return PlaylistEntryPage{}, err
		}
	}
	query := `SELECT entry_id, media_id, sort_order FROM playlist_items WHERE playlist_id = ?`
	args := []any{resourceID}
	if after.EntryID != "" {
		query += ` AND (sort_order > ? OR (sort_order = ? AND entry_id > ?))`
		args = append(args, after.SortOrder, after.SortOrder, after.EntryID)
	}
	query += ` ORDER BY sort_order ASC, entry_id ASC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, query, args...)
	if err != nil {
		return PlaylistEntryPage{}, err
	}
	type membership struct {
		entryID  string
		mediaID  string
		position int
	}
	memberships := []membership{}
	for rows.Next() {
		var item membership
		var sortOrder int
		if err := rows.Scan(&item.entryID, &item.mediaID, &sortOrder); err != nil {
			_ = rows.Close()
			return PlaylistEntryPage{}, err
		}
		item.position = max(0, sortOrder-1)
		memberships = append(memberships, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return PlaylistEntryPage{}, err
	}
	if err := rows.Close(); err != nil {
		return PlaylistEntryPage{}, err
	}
	hasMore := len(memberships) > limit
	if hasMore {
		memberships = memberships[:limit]
	}
	mediaIDs := make([]string, 0, len(memberships))
	for _, item := range memberships {
		mediaIDs = append(mediaIDs, item.mediaID)
	}
	mediaItems, err := s.mediaListItemsByOrderedIDsContext(ctx, viewerProfileID(user), mediaIDs)
	if err != nil {
		return PlaylistEntryPage{}, err
	}
	mediaByID := map[string]MediaItem{}
	for _, item := range mediaItems {
		mediaByID[item.ID] = item
	}
	entries := make([]PlaylistEntry, 0, len(memberships))
	for _, item := range memberships {
		media, allowed := mediaByID[item.mediaID]
		if !allowed {
			continue
		}
		entries = append(entries, PlaylistEntry{EntryID: item.entryID, Media: mediaCardForBrowse(media, user, nil), Position: item.position})
	}
	var nextCursor *string
	if hasMore && len(memberships) > 0 {
		last := memberships[len(memberships)-1]
		token, err := s.encodeContractCursor(scope, viewerProfileID(user), playlistEntryCursor{SortOrder: last.position + 1, EntryID: last.entryID}, now)
		if err != nil {
			return PlaylistEntryPage{}, err
		}
		nextCursor = &token
	}
	return PlaylistEntryPage{Items: entries, PageInfo: CursorPageInfo{NextCursor: nextCursor, HasMore: hasMore}}, nil
}

func (s *Server) savedResourceItemsPage(ctx context.Context, user User, kind, resourceID, cursor string, limit int, now time.Time) (SavedMediaPage, error) {
	resource, err := s.getPlaylistContext(ctx, user, resourceID, false)
	if err != nil || resource.Kind != kind || resource.Smart {
		return SavedMediaPage{}, sql.ErrNoRows
	}
	scope := "saved-resource-items:" + kind + ":" + resourceID
	var after savedMembershipCursor
	if cursor != "" {
		if err := s.decodeContractCursor(cursor, scope, viewerProfileID(user), &after, now); err != nil {
			return SavedMediaPage{}, err
		}
	}
	where := `JOIN playlist_items pi ON pi.media_id = m.id WHERE pi.playlist_id = ?`
	args := []any{resourceID}
	if after.MediaID != "" {
		where += ` AND (pi.sort_order > ? OR (pi.sort_order = ? AND pi.media_id > ?))`
		args = append(args, after.SortOrder, after.SortOrder, after.MediaID)
	}
	where += ` ORDER BY pi.sort_order ASC, pi.media_id ASC LIMIT ?`
	args = append(args, limit+1)
	items, err := s.queryMediaListItemsContext(ctx, viewerProfileID(user), where, args)
	if err != nil {
		return SavedMediaPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	cards := make([]MediaCard, 0, len(items))
	for _, item := range items {
		cards = append(cards, mediaCardForBrowse(item, user, nil))
	}
	var nextCursor *string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		var sortOrder int
		if err := s.queryUserRow(ctx, `SELECT sort_order FROM playlist_items WHERE playlist_id = ? AND media_id = ?`, resourceID, last.ID).Scan(&sortOrder); err != nil {
			return SavedMediaPage{}, err
		}
		token, err := s.encodeContractCursor(scope, viewerProfileID(user), savedMembershipCursor{SortOrder: sortOrder, MediaID: last.ID}, now)
		if err != nil {
			return SavedMediaPage{}, err
		}
		nextCursor = &token
	}
	return SavedMediaPage{Items: cards, PageInfo: CursorPageInfo{NextCursor: nextCursor, HasMore: hasMore}}, nil
}

func writeSavedDetailError(w http.ResponseWriter, err error, label string) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, strings.ToLower(label)+"_not_found", label+" was not found.")
		return
	}
	writeDatabaseAccessError(w, err, http.StatusInternalServerError, "saved_detail_failed", "Unable to load the "+strings.ToLower(label)+".")
}

func writeSavedMutationError(w http.ResponseWriter, err error, label string) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, strings.ToLower(label)+"_not_found", label+" was not found.")
	case errors.Is(err, errSavedRevisionConflict):
		writeError(w, http.StatusConflict, "revision_conflict", "The saved resource changed. Reload it before applying this batch.")
	default:
		writeError(w, http.StatusBadRequest, "saved_mutation_failed", err.Error())
	}
}

func writeSavedCursorOrDatabaseError(w http.ResponseWriter, err error, code, detail string) {
	switch {
	case errors.Is(err, errCursorExpired):
		writeError(w, http.StatusBadRequest, "cursor_expired", "The cursor expired. Restart from the first page.")
	case errors.Is(err, errInvalidCursor):
		writeError(w, http.StatusBadRequest, "invalid_cursor", "The cursor is invalid for this result set.")
	default:
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, code, detail)
	}
}

func (s *Server) handleSavedViews(w http.ResponseWriter, r *http.Request, user User) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/saved-views"), "/")
	if path == "" {
		s.handleSavedViewsRoot(w, r, user)
		return
	}
	parts := strings.Split(path, "/")
	viewID := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		s.handleSavedViewDetail(w, r, user, viewID)
		return
	}
	if len(parts) == 2 && parts[1] == "browse" && r.Method == http.MethodPost {
		var request SavedViewBrowseRequest
		if !decodeSavedRequest(w, r, &request) {
			return
		}
		s.handleSavedViewBrowse(w, r, user, viewID, request)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Saved view route was not found.")
}

func (s *Server) handleSavedViewsRoot(w http.ResponseWriter, r *http.Request, user User) {
	switch r.Method {
	case http.MethodGet:
		limit, ok := savedLimitFromRequest(w, r)
		if !ok {
			return
		}
		page, err := s.listSavedViewsPage(r.Context(), user, strings.TrimSpace(r.URL.Query().Get("cursor")), strings.TrimSpace(r.URL.Query().Get("libraryId")), limit, time.Now().UTC())
		if err != nil {
			writeSavedCursorOrDatabaseError(w, err, "saved_views_failed", "Unable to load saved views.")
			return
		}
		writeJSON(w, http.StatusOK, page)
	case http.MethodPost:
		var request SavedViewCreateRequest
		if !decodeSavedRequest(w, r, &request) {
			return
		}
		view, err := s.createSavedView(r.Context(), user, request)
		if err != nil {
			writeSavedViewMutationError(w, err)
			return
		}
		s.recordAudit(r, user, "saved_view.created", "saved_view", view.ID, "info", map[string]string{"title": view.Title, "libraryId": view.LibraryID, "pivot": view.Pivot})
		writeJSON(w, http.StatusCreated, view)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
	}
}

func (s *Server) handleSavedViewDetail(w http.ResponseWriter, r *http.Request, user User, viewID string) {
	switch r.Method {
	case http.MethodGet:
		view, err := s.getSavedView(r.Context(), user, viewID)
		if err != nil {
			writeSavedViewMutationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	case http.MethodPatch:
		var request SavedViewUpdateRequest
		if !decodeSavedRequest(w, r, &request) {
			return
		}
		view, err := s.updateSavedView(r.Context(), user, viewID, request)
		if err != nil {
			writeSavedViewMutationError(w, err)
			return
		}
		s.recordAudit(r, user, "saved_view.updated", "saved_view", view.ID, "info", map[string]string{"title": view.Title, "libraryId": view.LibraryID, "pivot": view.Pivot})
		writeJSON(w, http.StatusOK, view)
	case http.MethodDelete:
		result, err := s.execUserWrite(r.Context(), `DELETE FROM saved_views WHERE id = ? AND profile_id = ?`, viewID, viewerProfileID(user))
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "saved_view_delete_failed", "Unable to delete the saved view.")
			return
		}
		if rowsAffected(result) == 0 {
			writeError(w, http.StatusNotFound, "saved_view_not_found", "Saved view was not found.")
			return
		}
		s.recordAudit(r, user, "saved_view.deleted", "saved_view", viewID, "warn", nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET, PATCH, or DELETE for this endpoint.")
	}
}

func (s *Server) handleSavedViewBrowse(w http.ResponseWriter, r *http.Request, user User, viewID string, request SavedViewBrowseRequest) {
	view, err := s.getSavedView(r.Context(), user, viewID)
	if err != nil {
		writeSavedViewMutationError(w, err)
		return
	}
	library, err := s.getCanonicalLibraryScopeContext(r.Context(), view.LibraryID)
	if err != nil {
		writeSavedViewMutationError(w, err)
		return
	}
	kindID, ok := canonicalLibraryKind(library.Type)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "unsupported_library_kind", "The saved view library no longer supports canonical browsing.")
		return
	}
	kind, _ := productLibraryKindByID(kindID)
	browseRequest := BrowseLibraryRequest{Pivot: view.Pivot, Query: view.Query, Sort: view.Sort, Presentation: view.Presentation, Cursor: strings.TrimSpace(request.Cursor), Limit: request.Limit}
	response, err := s.browseLibraryContext(r.Context(), user, library, kind, browseRequest, time.Now().UTC())
	if err != nil {
		var validation *browseValidationProblems
		switch {
		case errors.As(err, &validation):
			writeBrowseValidationProblem(w, validation.Issues)
		case errors.Is(err, errCursorExpired):
			writeError(w, http.StatusBadRequest, "cursor_expired", "The browse cursor expired. Restart from the first page.")
		case errors.Is(err, errInvalidCursor):
			writeError(w, http.StatusBadRequest, "invalid_cursor", "The browse cursor is invalid for this saved view.")
		default:
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "saved_view_browse_failed", "Unable to browse the saved view.")
		}
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func normalizeSavedViewRequest(request SavedViewCreateRequest) SavedViewCreateRequest {
	request.Title = strings.TrimSpace(request.Title)
	request.LibraryID = strings.TrimSpace(request.LibraryID)
	request.Pivot = strings.TrimSpace(request.Pivot)
	request.Query = normalizeBrowseExpression(request.Query)
	for index := range request.Sort {
		request.Sort[index].Field = strings.TrimSpace(request.Sort[index].Field)
		request.Sort[index].Direction = strings.ToLower(strings.TrimSpace(request.Sort[index].Direction))
	}
	for index := range request.Presentation.Fields {
		request.Presentation.Fields[index] = strings.TrimSpace(request.Presentation.Fields[index])
	}
	return request
}

func (s *Server) validateSavedViewRequest(ctx context.Context, user User, request SavedViewCreateRequest) (Library, ProductLibraryKind, SavedViewCreateRequest, error) {
	request = normalizeSavedViewRequest(request)
	if request.Title == "" || len([]rune(request.Title)) > 160 {
		return Library{}, ProductLibraryKind{}, request, errors.New("Title is required and must be at most 160 characters.")
	}
	allowed, err := s.libraryAccessAllowedContext(ctx, user, request.LibraryID)
	if err != nil {
		return Library{}, ProductLibraryKind{}, request, err
	}
	if !allowed {
		return Library{}, ProductLibraryKind{}, request, sql.ErrNoRows
	}
	library, err := s.getCanonicalLibraryScopeContext(ctx, request.LibraryID)
	if err != nil {
		return Library{}, ProductLibraryKind{}, request, err
	}
	kindID, ok := canonicalLibraryKind(library.Type)
	if !ok {
		return Library{}, ProductLibraryKind{}, request, errors.New("Library kind does not support saved views.")
	}
	kind, _ := productLibraryKindByID(kindID)
	pivot, ok := browsePivotByID(kind, request.Pivot)
	if !ok || !pivot.BrowseSupported {
		return Library{}, ProductLibraryKind{}, request, &browseValidationProblems{Issues: []BrowseValidationIssue{{Field: "pivot", Code: "unsupported_pivot", Message: "Pivot is not available for saved views."}}}
	}
	if len(request.Sort) == 0 {
		request.Sort = append([]BrowseSort(nil), pivot.DefaultSort...)
	}
	if _, issues := compileBrowseSorts(request.Sort, pivot); len(issues) > 0 {
		return Library{}, ProductLibraryKind{}, request, &browseValidationProblems{Issues: issues}
	}
	presentation, issues := validateBrowsePresentation(request.Presentation.Fields)
	if len(issues) > 0 {
		return Library{}, ProductLibraryKind{}, request, &browseValidationProblems{Issues: issues}
	}
	request.Presentation.Fields = presentation
	if _, _, issues := compileBrowseWhere(library.ID, kind.ID, pivot, request.Query); len(issues) > 0 {
		return Library{}, ProductLibraryKind{}, request, &browseValidationProblems{Issues: issues}
	}
	return library, kind, request, nil
}

func (s *Server) createSavedView(ctx context.Context, user User, request SavedViewCreateRequest) (SavedView, error) {
	_, _, request, err := s.validateSavedViewRequest(ctx, user, request)
	if err != nil {
		return SavedView{}, err
	}
	queryJSON, sortJSON, presentationJSON, err := marshalSavedViewDefinition(request)
	if err != nil {
		return SavedView{}, err
	}
	id := randomOpaquePublicID()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.execUserWrite(ctx, `
		INSERT INTO saved_views (id, user_id, profile_id, title, library_id, pivot, query_json, sort_json, presentation_json, is_pinned, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, accountIDForUser(user), viewerProfileID(user), request.Title, request.LibraryID, request.Pivot, queryJSON, sortJSON, presentationJSON, boolInt(request.IsPinned), now, now)
	if err != nil {
		return SavedView{}, err
	}
	return s.getSavedView(ctx, user, id)
}

func (s *Server) updateSavedView(ctx context.Context, user User, viewID string, request SavedViewUpdateRequest) (SavedView, error) {
	if _, err := s.getSavedView(ctx, user, viewID); err != nil {
		return SavedView{}, err
	}
	_, _, request, err := s.validateSavedViewRequest(ctx, user, request)
	if err != nil {
		return SavedView{}, err
	}
	queryJSON, sortJSON, presentationJSON, err := marshalSavedViewDefinition(request)
	if err != nil {
		return SavedView{}, err
	}
	_, err = s.execUserWrite(ctx, `
		UPDATE saved_views
		SET title = ?, library_id = ?, pivot = ?, query_json = ?, sort_json = ?, presentation_json = ?, is_pinned = ?, updated_at = ?
		WHERE id = ? AND profile_id = ?`, request.Title, request.LibraryID, request.Pivot, queryJSON, sortJSON, presentationJSON, boolInt(request.IsPinned), time.Now().UTC().Format(time.RFC3339Nano), viewID, viewerProfileID(user))
	if err != nil {
		return SavedView{}, err
	}
	return s.getSavedView(ctx, user, viewID)
}

func marshalSavedViewDefinition(request SavedViewCreateRequest) (string, string, string, error) {
	queryJSON := ""
	if request.Query != nil {
		encoded, err := json.Marshal(request.Query)
		if err != nil {
			return "", "", "", err
		}
		queryJSON = string(encoded)
	}
	sortJSON, err := json.Marshal(request.Sort)
	if err != nil {
		return "", "", "", err
	}
	presentationJSON, err := json.Marshal(request.Presentation.Fields)
	if err != nil {
		return "", "", "", err
	}
	return queryJSON, string(sortJSON), string(presentationJSON), nil
}

func scanSavedView(scanner interface{ Scan(...any) error }) (SavedView, error) {
	var view SavedView
	var queryJSON, sortJSON, presentationJSON string
	var pinned int
	if err := scanner.Scan(&view.ID, &view.OwnerUserID, &view.ProfileID, &view.Title, &view.LibraryID, &view.LibraryName, &view.Pivot, &queryJSON, &sortJSON, &presentationJSON, &pinned, &view.CreatedAt, &view.UpdatedAt); err != nil {
		return SavedView{}, err
	}
	if strings.TrimSpace(queryJSON) != "" {
		var query BrowseExpression
		if err := json.Unmarshal([]byte(queryJSON), &query); err != nil {
			return SavedView{}, err
		}
		view.Query = &query
	}
	if err := json.Unmarshal([]byte(sortJSON), &view.Sort); err != nil {
		return SavedView{}, err
	}
	if err := json.Unmarshal([]byte(presentationJSON), &view.Presentation.Fields); err != nil {
		return SavedView{}, err
	}
	view.IsPinned = pinned != 0
	return view, nil
}

func (s *Server) getSavedView(ctx context.Context, user User, viewID string) (SavedView, error) {
	row := s.queryUserRow(ctx, `
		SELECT sv.id, sv.user_id, sv.profile_id, sv.title, sv.library_id, l.name, sv.pivot, sv.query_json, sv.sort_json, sv.presentation_json, sv.is_pinned, sv.created_at, sv.updated_at
		FROM saved_views sv
		JOIN libraries l ON l.id = sv.library_id
		WHERE sv.id = ? AND sv.profile_id = ?`, strings.TrimSpace(viewID), viewerProfileID(user))
	view, err := scanSavedView(row)
	if err != nil {
		return SavedView{}, err
	}
	allowed, err := s.libraryAccessAllowedContext(ctx, user, view.LibraryID)
	if err != nil {
		return SavedView{}, err
	}
	if !allowed {
		return SavedView{}, sql.ErrNoRows
	}
	return view, nil
}

func (s *Server) listSavedViewsPage(ctx context.Context, user User, cursor, libraryID string, limit int, now time.Time) (SavedViewPage, error) {
	libraryID = strings.TrimSpace(libraryID)
	scope := "saved-view-list:library=" + libraryID
	var after savedResourceCursor
	if cursor != "" {
		if err := s.decodeContractCursor(cursor, scope, viewerProfileID(user), &after, now); err != nil {
			return SavedViewPage{}, err
		}
	}
	where := `WHERE sv.profile_id = ?`
	args := []any{viewerProfileID(user)}
	if user.Role != "owner" {
		where += ` AND EXISTS (SELECT 1 FROM user_library_access ula WHERE ula.user_id = ? AND ula.library_id = sv.library_id)`
		args = append(args, user.ID)
	}
	if libraryID != "" {
		where += ` AND sv.library_id = ?`
		args = append(args, libraryID)
	}
	if after.UpdatedAt != "" {
		where += ` AND (sv.updated_at < ? OR (sv.updated_at = ? AND sv.id < ?))`
		args = append(args, after.UpdatedAt, after.UpdatedAt, after.ID)
	}
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT sv.id, sv.user_id, sv.profile_id, sv.title, sv.library_id, l.name, sv.pivot, sv.query_json, sv.sort_json, sv.presentation_json, sv.is_pinned, sv.created_at, sv.updated_at
		FROM saved_views sv
		JOIN libraries l ON l.id = sv.library_id
		`+where+`
		ORDER BY sv.updated_at DESC, sv.id DESC
		LIMIT ?`, args...)
	if err != nil {
		return SavedViewPage{}, err
	}
	defer rows.Close()
	views := []SavedView{}
	for rows.Next() {
		view, err := scanSavedView(rows)
		if err != nil {
			return SavedViewPage{}, err
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return SavedViewPage{}, err
	}
	hasMore := len(views) > limit
	if hasMore {
		views = views[:limit]
	}
	var nextCursor *string
	if hasMore && len(views) > 0 {
		last := views[len(views)-1]
		token, err := s.encodeContractCursor(scope, viewerProfileID(user), savedResourceCursor{UpdatedAt: last.UpdatedAt, ID: last.ID}, now)
		if err != nil {
			return SavedViewPage{}, err
		}
		nextCursor = &token
	}
	return SavedViewPage{Items: views, PageInfo: CursorPageInfo{NextCursor: nextCursor, HasMore: hasMore}}, nil
}

func writeSavedViewMutationError(w http.ResponseWriter, err error) {
	var validation *browseValidationProblems
	switch {
	case errors.As(err, &validation):
		writeBrowseValidationProblem(w, validation.Issues)
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "saved_view_not_found", "Saved view was not found.")
	default:
		writeError(w, http.StatusBadRequest, "saved_view_failed", err.Error())
	}
}
