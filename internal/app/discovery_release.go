package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func normalizedSearchHistoryQuery(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func (s *Server) recordSearchHistory(ctx context.Context, profileID, query string) error {
	query = strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
	normalized := normalizedSearchHistoryQuery(query)
	if profileID == "" || normalized == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.execUserWrite(ctx, `
		INSERT INTO profile_search_history (profile_id, normalized_query, query, use_count, last_used_at)
		VALUES (?, ?, ?, 1, ?)
		ON CONFLICT(profile_id, normalized_query) DO UPDATE SET
			query = excluded.query,
			use_count = profile_search_history.use_count + 1,
			last_used_at = excluded.last_used_at`, profileID, normalized, query, now); err != nil {
		return err
	}
	_, err := s.execUserWrite(ctx, `
		DELETE FROM profile_search_history
		WHERE profile_id = ? AND normalized_query NOT IN (
			SELECT normalized_query FROM profile_search_history
			WHERE profile_id = ? ORDER BY last_used_at DESC LIMIT 30
		)`, profileID, profileID)
	return err
}

func (s *Server) handleSearchHistory(w http.ResponseWriter, r *http.Request, user User) {
	profileID := viewerProfileID(user)
	switch r.Method {
	case http.MethodGet:
		rows, err := s.queryUserRead(r.Context(), `
			SELECT query, last_used_at, use_count
			FROM profile_search_history WHERE profile_id = ?
			ORDER BY last_used_at DESC LIMIT 30`, profileID)
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "search_history_unavailable", "Recent searches are temporarily unavailable.")
			return
		}
		defer rows.Close()
		items := []SearchHistoryItem{}
		for rows.Next() {
			var item SearchHistoryItem
			if err := rows.Scan(&item.Query, &item.LastUsedAt, &item.UseCount); err != nil {
				writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "search_history_unavailable", "Recent searches are temporarily unavailable.")
				return
			}
			items = append(items, item)
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, SearchHistoryResponse{Items: items})
	case http.MethodDelete:
		query := normalizedSearchHistoryQuery(r.URL.Query().Get("query"))
		var err error
		if query == "" {
			_, err = s.execUserWrite(r.Context(), `DELETE FROM profile_search_history WHERE profile_id = ?`, profileID)
		} else {
			_, err = s.execUserWrite(r.Context(), `DELETE FROM profile_search_history WHERE profile_id = ? AND normalized_query = ?`, profileID, query)
		}
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "search_history_update_failed", "Recent searches could not be updated.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or DELETE for this endpoint.")
	}
}

func stablePersonID(name string) string {
	canonical := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
	if canonical == "" {
		return ""
	}
	return encodePersonIdentityID(personIdentitySelector{Kind: "name", Name: canonical})
}

func stablePersonIdentityID(name string, providerIDs map[string]string) string {
	provider, externalID := canonicalPersonProviderIdentity(providerIDs)
	if provider != "" {
		return encodePersonIdentityID(personIdentitySelector{Kind: "provider", Provider: provider, ExternalID: externalID})
	}
	return stablePersonID(name)
}

func stablePersonIdentityIDForCredit(name string, providerIDs map[string]string, canonicalKey, mediaID, role string) string {
	if canonicalKey = strings.TrimSpace(canonicalKey); canonicalKey != "" {
		return encodePersonIdentityID(personIdentitySelector{Kind: "canonical", CanonicalKey: canonicalKey})
	}
	provider, externalID := canonicalPersonProviderIdentity(providerIDs)
	if provider != "" {
		return encodePersonIdentityID(personIdentitySelector{Kind: "provider", Provider: provider, ExternalID: externalID})
	}
	canonicalName := strings.Join(strings.Fields(strings.TrimSpace(name)), " ")
	return encodePersonIdentityID(personIdentitySelector{
		Kind:    "unresolved",
		MediaID: strings.TrimSpace(mediaID),
		Name:    canonicalName,
		Role:    strings.Join(strings.Fields(strings.TrimSpace(role)), " "),
	})
}

type personIdentitySelector struct {
	Kind         string
	Name         string
	Provider     string
	ExternalID   string
	Fingerprint  string
	CanonicalKey string
	ImageURL     string
	Character    string
	MediaID      string
	Role         string
}

func personFallbackEvidence(imageURL, character string) string {
	if imageURL = strings.TrimSpace(imageURL); imageURL != "" {
		return "image:" + strings.ToLower(imageURL)
	}
	if character = strings.Join(strings.Fields(strings.TrimSpace(character)), " "); character != "" {
		return "character:" + strings.ToLower(character)
	}
	return ""
}

func personFallbackFingerprint(name, evidence string) string {
	material := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " ")) + "\x00" + strings.ToLower(strings.TrimSpace(evidence))
	digest := sha256.Sum256([]byte(material))
	return hex.EncodeToString(digest[:16])
}

func canonicalPersonProviderIdentity(providerIDs map[string]string) (string, string) {
	providerKeys := make([]string, 0, len(providerIDs))
	normalizedIDs := map[string]string{}
	for provider, externalID := range providerIDs {
		if strings.TrimSpace(provider) != "" && strings.TrimSpace(externalID) != "" {
			key := strings.ToLower(strings.TrimSpace(provider))
			providerKeys = append(providerKeys, key)
			normalizedIDs[key] = strings.TrimSpace(externalID)
		}
	}
	sort.Strings(providerKeys)
	if len(providerKeys) == 0 {
		return "", ""
	}
	provider := providerKeys[0]
	return provider, normalizedIDs[provider]
}

func encodePersonIdentityID(identity personIdentitySelector) string {
	var payload string
	switch identity.Kind {
	case "provider":
		payload = strings.Join([]string{"v2", "provider", strings.ToLower(strings.TrimSpace(identity.Provider)), strings.TrimSpace(identity.ExternalID)}, "\x1f")
	case "fallback":
		payload = strings.Join([]string{"v2", "fallback", strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(identity.Name)), " ")), strings.ToLower(strings.TrimSpace(identity.Fingerprint))}, "\x1f")
	case "canonical":
		payload = strings.Join([]string{"v2", "canonical", strings.TrimSpace(identity.CanonicalKey)}, "\x1f")
	case "unresolved":
		payload = strings.Join([]string{
			"v2", "unresolved", strings.TrimSpace(identity.MediaID),
			strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(identity.Name)), " ")),
			strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(identity.Role)), " ")),
		}, "\x1f")
	case "name":
		payload = strings.Join([]string{"v2", "name", strings.Join(strings.Fields(strings.TrimSpace(identity.Name)), " ")}, "\x1f")
	}
	if strings.HasSuffix(payload, "\x1f") || payload == "" {
		return ""
	}
	return "person_" + base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodePersonIdentityID(id string) (personIdentitySelector, bool) {
	if !strings.HasPrefix(id, "person_") {
		return personIdentitySelector{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, "person_"))
	if err != nil || !utf8.Valid(raw) {
		return personIdentitySelector{}, false
	}
	parts := strings.Split(string(raw), "\x1f")
	if len(parts) >= 3 && parts[0] == "v2" {
		switch parts[1] {
		case "provider":
			if len(parts) == 4 && strings.TrimSpace(parts[2]) != "" && strings.TrimSpace(parts[3]) != "" {
				return personIdentitySelector{Kind: "provider", Provider: strings.ToLower(strings.TrimSpace(parts[2])), ExternalID: strings.TrimSpace(parts[3])}, true
			}
		case "fallback":
			if len(parts) != 4 {
				return personIdentitySelector{}, false
			}
			name := strings.Join(strings.Fields(strings.TrimSpace(parts[2])), " ")
			fingerprint := strings.ToLower(strings.TrimSpace(parts[3]))
			if name != "" && len(fingerprint) == 32 {
				return personIdentitySelector{Kind: "fallback", Name: name, Fingerprint: fingerprint}, true
			}
		case "canonical":
			if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
				return personIdentitySelector{Kind: "canonical", CanonicalKey: strings.TrimSpace(parts[2])}, true
			}
		case "unresolved":
			if len(parts) == 5 && strings.TrimSpace(parts[2]) != "" && strings.TrimSpace(parts[3]) != "" && strings.TrimSpace(parts[4]) != "" {
				return personIdentitySelector{
					Kind:    "unresolved",
					MediaID: strings.TrimSpace(parts[2]),
					Name:    strings.Join(strings.Fields(strings.TrimSpace(parts[3])), " "),
					Role:    strings.Join(strings.Fields(strings.TrimSpace(parts[4])), " "),
				}, true
			}
		case "name":
			name := strings.Join(strings.Fields(strings.TrimSpace(strings.Join(parts[2:], "\x1f"))), " ")
			if name != "" && utf8.RuneCountInString(name) <= 200 {
				return personIdentitySelector{Kind: "name", Name: name}, true
			}
		}
		return personIdentitySelector{}, false
	}
	// Accept the unversioned IDs emitted by early release-candidate builds.
	name := strings.Join(strings.Fields(strings.TrimSpace(parts[0])), " ")
	if len(parts) == 3 && name != "" && strings.TrimSpace(parts[1]) != "" && strings.TrimSpace(parts[2]) != "" {
		return personIdentitySelector{Kind: "provider", Provider: strings.ToLower(strings.TrimSpace(parts[1])), ExternalID: strings.TrimSpace(parts[2])}, true
	}
	if len(parts) == 1 && name != "" && utf8.RuneCountInString(name) <= 200 {
		return personIdentitySelector{Kind: "name", Name: name}, true
	}
	return personIdentitySelector{}, false
}

func personNameFromID(id string) (string, bool) {
	identity, ok := decodePersonIdentityID(id)
	return identity.Name, ok && identity.Kind == "name"
}

func personProviderIdentityFromID(id string) (string, string) {
	identity, ok := decodePersonIdentityID(id)
	if !ok || identity.Kind != "provider" {
		return "", ""
	}
	return identity.Provider, identity.ExternalID
}

func personIdentityKey(identity personIdentitySelector) string {
	switch identity.Kind {
	case "provider":
		return "provider\x1f" + identity.Provider + "\x1f" + identity.ExternalID
	case "fallback":
		fingerprint := strings.ToLower(strings.TrimSpace(identity.Fingerprint))
		if fingerprint == "" {
			if evidence := personFallbackEvidence(identity.ImageURL, identity.Character); evidence != "" {
				fingerprint = personFallbackFingerprint(identity.Name, evidence)
			}
		}
		if fingerprint != "" {
			return "fallback\x1f" + strings.ToLower(strings.Join(strings.Fields(identity.Name), " ")) + "\x1f" + fingerprint
		}
		return ""
	case "canonical":
		return "canonical\x1f" + identity.CanonicalKey
	case "unresolved":
		return strings.Join([]string{
			"unresolved", strings.TrimSpace(identity.MediaID),
			strings.ToLower(strings.Join(strings.Fields(identity.Name), " ")),
			strings.ToLower(strings.Join(strings.Fields(identity.Role), " ")),
		}, "\x1f")
	case "name":
		return "name\x1f" + strings.ToLower(identity.Name)
	default:
		return ""
	}
}

func personIDFromIdentityKey(key string) string {
	parts := strings.Split(key, "\x1f")
	switch {
	case len(parts) == 3 && parts[0] == "provider":
		return encodePersonIdentityID(personIdentitySelector{Kind: "provider", Provider: parts[1], ExternalID: parts[2]})
	case len(parts) == 3 && parts[0] == "fallback":
		name := strings.Join(strings.Fields(strings.TrimSpace(parts[1])), " ")
		evidence := strings.TrimSpace(parts[2])
		return encodePersonIdentityID(personIdentitySelector{Kind: "fallback", Name: name, Fingerprint: personFallbackFingerprint(name, evidence)})
	case len(parts) == 2 && parts[0] == "canonical":
		return encodePersonIdentityID(personIdentitySelector{Kind: "canonical", CanonicalKey: parts[1]})
	case len(parts) == 4 && parts[0] == "unresolved":
		return encodePersonIdentityID(personIdentitySelector{Kind: "unresolved", MediaID: parts[1], Name: parts[2], Role: parts[3]})
	case len(parts) == 2 && parts[0] == "name":
		return stablePersonID(parts[1])
	default:
		return ""
	}
}

func personIdentityKeySQL(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "p"
	}
	return `COALESCE(
		CASE WHEN trim(` + alias + `.canonical_person_key) <> ''
			THEN 'canonical' || CHAR(31) || trim(` + alias + `.canonical_person_key)
		END,
		 (SELECT 'provider' || CHAR(31) || lower(trim(person_provider.key)) || CHAR(31) || trim(CAST(person_provider.value AS TEXT))
		 FROM json_each(CASE WHEN json_valid(` + alias + `.provider_ids_json) THEN ` + alias + `.provider_ids_json ELSE '{}' END) person_provider
		 WHERE trim(person_provider.key) <> '' AND trim(CAST(person_provider.value AS TEXT)) <> ''
		 ORDER BY lower(trim(person_provider.key)), trim(CAST(person_provider.value AS TEXT)) LIMIT 1),
		'unresolved' || CHAR(31) || trim(` + alias + `.media_id) || CHAR(31) || lower(trim(` + alias + `.name)) || CHAR(31) || lower(trim(` + alias + `.role)))`
}

func personIdentityPredicateSQL(identity personIdentitySelector, alias string) (string, []any) {
	if alias == "" {
		alias = "p"
	}
	switch identity.Kind {
	case "provider":
		return "json_extract(" + alias + ".provider_ids_json, '$.' || ?) = ?", []any{identity.Provider, identity.ExternalID}
	case "fallback":
		if identity.ImageURL != "" {
			return "lower(trim(" + alias + ".name)) = lower(trim(?)) AND lower(trim(" + alias + ".image_url)) = lower(trim(?))", []any{identity.Name, identity.ImageURL}
		}
		if identity.Character != "" {
			return "lower(trim(" + alias + ".name)) = lower(trim(?)) AND trim(" + alias + ".image_url) = '' AND lower(trim(" + alias + ".character)) = lower(trim(?))", []any{identity.Name, identity.Character}
		}
		return "lower(trim(" + alias + ".name)) = lower(trim(?))", []any{identity.Name}
	case "canonical":
		return alias + ".canonical_person_key = ?", []any{identity.CanonicalKey}
	case "unresolved":
		return alias + ".media_id = ? AND lower(trim(" + alias + ".name)) = ? AND lower(trim(" + alias + ".role)) = ?", []any{
			identity.MediaID,
			strings.ToLower(strings.Join(strings.Fields(identity.Name), " ")),
			strings.ToLower(strings.Join(strings.Fields(identity.Role), " ")),
		}
	case "name":
		return "lower(trim(" + alias + ".name)) = lower(trim(?))", []any{identity.Name}
	default:
		return "1 = 0", nil
	}
}

type personCreditCursor struct {
	Year      int    `json:"year"`
	SortTitle string `json:"sortTitle"`
	ID        string `json:"id"`
}

type mediaChildrenCursor struct {
	Kind               string  `json:"kind,omitempty"`
	IndexNumber        int     `json:"indexNumber,omitempty"`
	SeriesIndexMissing int     `json:"seriesIndexMissing,omitempty"`
	SeriesIndex        float64 `json:"seriesIndex,omitempty"`
	ReleaseDate        string  `json:"releaseDate,omitempty"`
	SortTitle          string  `json:"sortTitle"`
	ID                 string  `json:"id"`
}

func virtualAudiobookSeriesIndexSQL(alias string) (string, string) {
	if strings.TrimSpace(alias) == "" {
		alias = "m"
	}
	metadata := "CASE WHEN json_valid(" + alias + ".typed_metadata_json) THEN " + alias + ".typed_metadata_json ELSE '{}' END"
	raw := "trim(COALESCE(json_extract(" + metadata + ", '$.seriesIndex'), ''))"
	missing := "CASE WHEN " + raw + " = '' THEN 1 ELSE 0 END"
	value := "CASE WHEN " + raw + " = '' THEN 0 ELSE CAST(" + raw + " AS REAL) END"
	return missing, value
}

func virtualAudiobookChildrenOrderSQL(alias string) string {
	missing, seriesIndex := virtualAudiobookSeriesIndexSQL(alias)
	return missing + ` ASC, ` + seriesIndex + ` ASC, ` + normalizedReleaseDateSQL(alias) + ` ASC, ` + alias + `.sort_title COLLATE NOCASE ASC, ` + alias + `.id ASC`
}

func virtualAudiobookChildCursorForItem(item MediaItem) mediaChildrenCursor {
	raw := strings.TrimSpace(item.TypedMetadata["seriesIndex"])
	missing := 0
	value := 0.0
	if raw == "" {
		missing = 1
	} else if parsed, err := strconv.ParseFloat(raw, 64); err == nil {
		value = parsed
	}
	return mediaChildrenCursor{
		Kind: "audiobook-facet", SeriesIndexMissing: missing, SeriesIndex: value,
		ReleaseDate: normalizedMediaReleaseDate(item), SortTitle: item.SortTitle, ID: item.ID,
	}
}

func virtualAudiobookChildrenAfterSQL(alias string, after mediaChildrenCursor) (string, []any) {
	missing, seriesIndex := virtualAudiobookSeriesIndexSQL(alias)
	releaseDate := normalizedReleaseDateSQL(alias)
	title := alias + ".sort_title COLLATE NOCASE"
	return ` AND (` + missing + ` > ? OR (` + missing + ` = ? AND (` +
			seriesIndex + ` > ? OR (` + seriesIndex + ` = ? AND (` +
			releaseDate + ` > ? OR (` + releaseDate + ` = ? AND (` +
			title + ` > ? OR (` + title + ` = ? AND ` + alias + `.id > ?))))))))`, []any{
			after.SeriesIndexMissing, after.SeriesIndexMissing,
			after.SeriesIndex, after.SeriesIndex,
			after.ReleaseDate, after.ReleaseDate,
			after.SortTitle, after.SortTitle, after.ID,
		}
}

func (s *Server) handleMediaChildren(w http.ResponseWriter, r *http.Request, user User, mediaID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	virtualIdentity, identityErr := s.virtualAudiobookFacetIdentityContext(r.Context(), mediaID)
	virtual := identityErr == nil
	if identityErr != nil && !errors.Is(identityErr, sql.ErrNoRows) {
		writeDatabaseAccessError(w, identityErr, http.StatusServiceUnavailable, "media_children_unavailable", "Episodes or tracks are temporarily unavailable.")
		return
	}
	if !virtual {
		if _, err := s.getMediaAccessSummaryContext(r.Context(), viewerProfileID(user), mediaID); err != nil {
			writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
			return
		}
	}
	limit := clampInt(queryInt(r, "limit", 50), 1, 200)
	scope := "media-children:" + mediaID
	var after mediaChildrenCursor
	if cursor := strings.TrimSpace(r.URL.Query().Get("cursor")); cursor != "" {
		if err := s.decodeContractCursor(cursor, scope, viewerProfileID(user), &after, time.Now().UTC()); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "The media children cursor is invalid or expired.")
			return
		}
		if virtual && after.Kind != "audiobook-facet" {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "The media children cursor is invalid or expired.")
			return
		}
		if !virtual && after.Kind != "" {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "The media children cursor is invalid or expired.")
			return
		}
	}
	where := "WHERE m.parent_id = ? AND NOT " + mediaExtraSQLPredicate("m")
	args := []any{mediaID}
	if virtual {
		var ok bool
		where, args, ok = s.virtualAudiobookFacetWhere(viewerProfileID(user), virtualIdentity)
		if !ok {
			writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
			return
		}
	}
	order := "m.index_number ASC, m.sort_title COLLATE NOCASE ASC, m.id ASC"
	if virtual {
		order = virtualAudiobookChildrenOrderSQL("m")
	}
	if after.ID != "" && virtual {
		predicate, cursorArgs := virtualAudiobookChildrenAfterSQL("m", after)
		where += predicate
		args = append(args, cursorArgs...)
	} else if after.ID != "" {
		where += " AND (m.index_number > ? OR (m.index_number = ? AND (m.sort_title COLLATE NOCASE > ? OR (m.sort_title COLLATE NOCASE = ? AND m.id > ?))))"
		args = append(args, after.IndexNumber, after.IndexNumber, after.SortTitle, after.SortTitle, after.ID)
	}
	args = append(args, limit+1)
	items, err := s.queryMediaListItemsContext(r.Context(), viewerProfileID(user), where+" ORDER BY "+order+" LIMIT ?", args)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "media_children_unavailable", "Episodes or tracks are temporarily unavailable.")
		return
	}
	if err := s.populateMediaCardSummariesContext(r.Context(), items); err != nil {
		writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "media_children_unavailable", "Episodes or tracks are temporarily unavailable.")
		return
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var next *string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		cursor := mediaChildrenCursor{IndexNumber: last.IndexNumber, SortTitle: last.SortTitle, ID: last.ID}
		if virtual {
			cursor = virtualAudiobookChildCursorForItem(last)
		}
		token, err := s.encodeContractCursor(scope, viewerProfileID(user), cursor, time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "media_children_cursor_failed", "Unable to continue this list.")
			return
		}
		next = &token
	}
	cards := make([]MediaCard, 0, len(items))
	for _, item := range items {
		cards = append(cards, mediaCardForBrowse(item, user, canonicalPresentationFields()))
	}
	w.Header().Set("Cache-Control", "private, max-age=15, stale-while-revalidate=45")
	writeJSON(w, http.StatusOK, MediaCardPageResponse{Items: cards, PageInfo: CursorPageInfo{NextCursor: next, HasMore: hasMore}})
}

func (s *Server) populateMediaCardSummariesContext(ctx context.Context, items []MediaItem) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	byID := make(map[string]*MediaItem, len(items))
	for index := range items {
		ids = append(ids, items[index].ID)
		byID[items[index].ID] = &items[index]
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.queryUserRead(ctx, `SELECT id, COALESCE(summary, '') FROM media_items WHERE id IN (`+sqlPlaceholders(len(ids))+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, summary string
		if err := rows.Scan(&id, &summary); err != nil {
			return err
		}
		if item := byID[id]; item != nil {
			item.Summary = strings.TrimSpace(summary)
		}
	}
	return rows.Err()
}

func (s *Server) handlePersonRoute(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	personPath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/people/"), "/")
	if strings.HasSuffix(personPath, "/artwork") {
		s.handlePersonArtwork(w, r, user, strings.TrimSuffix(personPath, "/artwork"))
		return
	}
	identity, personID, ok := s.personIdentityForPublicID(r.Context(), personPath)
	if !ok {
		writeError(w, http.StatusNotFound, "person_not_found", "Person was not found.")
		return
	}
	resolvedIdentity, name, roles, imageURL, found, err := s.personIdentityContext(r.Context(), viewerProfileID(user), identity)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "person_detail_unavailable", "Person details are temporarily unavailable.")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "person_not_found", "Person was not found.")
		return
	}
	identity = resolvedIdentity
	limit := clampInt(queryInt(r, "limit", 50), 1, 100)
	scope := "person-credits:" + personID
	var after personCreditCursor
	if cursor := strings.TrimSpace(r.URL.Query().Get("cursor")); cursor != "" {
		if err := s.decodeContractCursor(cursor, scope, viewerProfileID(user), &after, time.Now().UTC()); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "The person credits cursor is invalid or expired.")
			return
		}
	}
	identitySQL, args := personIdentityPredicateSQL(identity, "p")
	where := ""
	if after.ID != "" {
		where = " AND (m.year < ? OR (m.year = ? AND (m.sort_title COLLATE NOCASE > ? OR (m.sort_title COLLATE NOCASE = ? AND m.id > ?))))"
		args = append(args, after.Year, after.Year, after.SortTitle, after.SortTitle, after.ID)
	}
	args = append(args, limit+1)
	items, err := s.queryMediaListItemsContext(r.Context(), viewerProfileID(user), `
		JOIN (
			SELECT CASE
				WHEN credited.type IN ('movie', 'show', 'anime', 'artist', 'album', 'audiobook') THEN credited.id
				WHEN parent.type IN ('show', 'anime', 'album', 'audiobook') THEN parent.id
				WHEN grandparent.type IN ('show', 'anime') THEN grandparent.id
				ELSE credited.id END AS root_id
			FROM media_people p
			JOIN media_items credited ON credited.id = p.media_id
			LEFT JOIN media_items parent ON parent.id = credited.parent_id
			LEFT JOIN media_items grandparent ON grandparent.id = parent.parent_id
			WHERE `+identitySQL+`
			GROUP BY root_id
		) person_match ON person_match.root_id = m.id
		WHERE 1 = 1`+where+`
		ORDER BY m.year DESC, m.sort_title COLLATE NOCASE ASC, m.id ASC LIMIT ?`, args)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "person_detail_unavailable", "Person details are temporarily unavailable.")
		return
	}
	if len(items) == 0 && after.ID == "" {
		writeError(w, http.StatusNotFound, "person_not_found", "Person was not found.")
		return
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var next *string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		token, err := s.encodeContractCursor(scope, viewerProfileID(user), personCreditCursor{Year: last.Year, SortTitle: last.SortTitle, ID: last.ID}, time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "person_cursor_failed", "Unable to continue these credits.")
			return
		}
		next = &token
	}
	credits := make([]MediaCard, 0, len(items))
	for _, item := range items {
		credits = append(credits, mediaCardForBrowse(item, user, canonicalPresentationFields()))
	}
	if localArtworkFileExists(imageURL) {
		imageURL = personArtworkURL(personID)
	} else {
		imageURL = ""
	}
	writeJSON(w, http.StatusOK, PersonDetailResponse{
		Person:  PersonSummary{ID: personID, Name: name, ImageURL: imageURL, Roles: roles},
		Credits: credits, PageInfo: CursorPageInfo{NextCursor: next, HasMore: hasMore},
	})
}

func (s *Server) personIdentityContext(ctx context.Context, profileID string, identity personIdentitySelector) (personIdentitySelector, string, []string, string, bool, error) {
	identitySQL, args := personIdentityPredicateSQL(identity, "p")
	restrictionSQL, restrictionArgs := s.mediaVisibilityRestrictionSQL(profileID)
	args = append(args, restrictionArgs...)
	rows, err := s.queryUserRead(ctx, `
		SELECT p.name, p.role, p.image_url, p.character
		FROM media_people p
		JOIN media_items m ON m.id = p.media_id
		WHERE `+identitySQL+restrictionSQL+`
		ORDER BY lower(trim(p.name)), CASE WHEN p.image_url <> '' THEN 0 ELSE 1 END, p.sort_order, p.role`, args...)
	if err != nil {
		return personIdentitySelector{}, "", nil, "", false, err
	}
	defer rows.Close()
	name := ""
	roles := []string{}
	seen := map[string]bool{}
	image := ""
	found := false
	for rows.Next() {
		var candidateName, role, candidateImage, candidateCharacter string
		if err := rows.Scan(&candidateName, &role, &candidateImage, &candidateCharacter); err != nil {
			return personIdentitySelector{}, "", nil, "", false, err
		}
		candidateName = strings.Join(strings.Fields(strings.TrimSpace(candidateName)), " ")
		candidateImage = strings.TrimSpace(candidateImage)
		candidateCharacter = strings.Join(strings.Fields(strings.TrimSpace(candidateCharacter)), " ")
		if identity.Kind == "fallback" && personFallbackFingerprint(candidateName, personFallbackEvidence(candidateImage, candidateCharacter)) != identity.Fingerprint {
			continue
		}
		found = true
		if identity.Kind == "fallback" && identity.ImageURL == "" {
			identity.ImageURL = candidateImage
			identity.Character = candidateCharacter
		}
		if name == "" {
			name = candidateName
		}
		role = strings.TrimSpace(role)
		if role != "" && !seen[strings.ToLower(role)] {
			seen[strings.ToLower(role)] = true
			roles = append(roles, role)
		}
		if image == "" {
			image = candidateImage
		}
	}
	sort.Strings(roles)
	return identity, name, roles, image, found, rows.Err()
}

func (s *Server) searchPeopleContext(ctx context.Context, user User, query string, requestedLibraries []string, limit int, cursor searchResultCursor, spec searchSortSpec) ([]MediaItem, error) {
	query = strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
	if query == "" {
		return []MediaItem{}, nil
	}
	where := []string{"lower(trim(p.name)) LIKE lower(?) ESCAPE '\\'"}
	whereArgs := []any{"%" + escapeSQLLike(query) + "%"}
	allowedLibraries := []string{}
	for _, libraryID := range requestedLibraries {
		if s.canAccessLibrary(user, libraryID) {
			allowedLibraries = append(allowedLibraries, libraryID)
		}
	}
	if len(requestedLibraries) > 0 && len(allowedLibraries) == 0 {
		return []MediaItem{}, nil
	}
	if len(allowedLibraries) > 0 {
		where = append(where, "m.library_id IN ("+sqlPlaceholders(len(allowedLibraries))+")")
		for _, libraryID := range allowedLibraries {
			whereArgs = append(whereArgs, libraryID)
		}
	}
	restrictionSQL, restrictionArgs := s.mediaVisibilityRestrictionSQL(viewerProfileID(user))
	whereArgs = append(whereArgs, restrictionArgs...)
	visibilitySQL := librarySurfaceVisibilityRestrictionSQL("search")
	identityKeySQL := personIdentityKeySQL("p")
	rankSQL := "CASE WHEN lower(trim(p.name)) = lower(?) THEN 0 WHEN lower(trim(p.name)) LIKE lower(?) ESCAPE '\\' THEN 1 ELSE 2 END"
	outerWhere := ""
	outerArgs := []any{}
	order := "person_rank ASC, person_name COLLATE NOCASE ASC"
	if spec.Field == searchSortTitle {
		direction := strings.ToUpper(spec.Direction)
		order = "person_name COLLATE NOCASE " + direction + ", person_identity ASC"
	} else {
		order += ", person_identity ASC"
	}
	if cursor.Mode != "" {
		cursorIdentityKey := strings.TrimSpace(cursor.IdentityKey)
		if cursorIdentityKey == "" {
			cursorIdentity, _, validIdentity := s.personIdentityForPublicID(ctx, cursor.ID)
			if !validIdentity {
				return nil, errInvalidCursor
			}
			cursorIdentityKey = personIdentityKey(cursorIdentity)
			if cursorIdentityKey == "" {
				return nil, errInvalidCursor
			}
		}
		if spec.Field == searchSortTitle {
			op := searchDirectionOperator(spec.Direction)
			outerWhere = "WHERE person_name COLLATE NOCASE " + op + " ? OR (person_name COLLATE NOCASE = ? AND person_identity > ?)"
			outerArgs = append(outerArgs, cursor.SortTitle, cursor.SortTitle, cursorIdentityKey)
		} else {
			outerWhere = "WHERE person_rank > ? OR (person_rank = ? AND (person_name COLLATE NOCASE > ? OR (person_name COLLATE NOCASE = ? AND person_identity > ?)))"
			outerArgs = append(outerArgs, cursor.Rank, cursor.Rank, cursor.SortTitle, cursor.SortTitle, cursorIdentityKey)
		}
	}
	args := []any{query, escapeSQLLike(query) + "%"}
	args = append(args, whereArgs...)
	args = append(args, outerArgs...)
	args = append(args, limit)
	rows, err := s.queryUserRead(ctx, `
		SELECT person_identity, person_name, image_url, person_rank
		FROM (
			SELECT `+identityKeySQL+` AS person_identity,
			       MIN(trim(p.name)) AS person_name,
			       COALESCE(MAX(NULLIF(p.image_url, '')), '') AS image_url,
			       MIN(`+rankSQL+`) AS person_rank
			FROM media_people p
			JOIN media_items m ON m.id = p.media_id
			WHERE `+strings.Join(where, " AND ")+` `+restrictionSQL+` `+visibilitySQL+`
			GROUP BY person_identity
		) people_search
		`+outerWhere+`
		ORDER BY `+order+` LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []MediaItem{}
	for rows.Next() {
		var identityKey, name, image string
		var rank float64
		if err := rows.Scan(&identityKey, &name, &image, &rank); err != nil {
			return nil, err
		}
		identity, ok := personIdentityFromKey(identityKey)
		if !ok {
			continue
		}
		id, err := s.publicPersonIDForIdentity(ctx, identity)
		if err != nil {
			continue
		}
		if localArtworkFileExists(image) {
			image = personArtworkURL(id)
		} else {
			image = ""
		}
		items = append(items, MediaItem{ID: id, Type: "person", Title: name, SortTitle: name, Images: ImageSet{Poster: image, Thumb: image}, SearchRank: rank, RandomKey: identityKey, Actions: []string{}})
	}
	return items, rows.Err()
}

func consumerMediaDetailProjection(item MediaItem, user User) MediaItem {
	// /media/{id} is a viewer resource even when the current principal can also
	// administer the server. A client-controlled query string must never turn
	// this response into the management projection.
	// Clone every reference-backed field that this projection edits. Media
	// details are cached and a shallow projection would otherwise redact the
	// cached value itself, making later requests vary by request order.
	item.Streams = append([]Stream(nil), item.Streams...)
	item.MediaFiles = append([]MediaFileVersion(nil), item.MediaFiles...)
	for index := range item.MediaFiles {
		item.MediaFiles[index].Streams = append([]Stream(nil), item.MediaFiles[index].Streams...)
	}
	item.OptimizedVersions = append([]OptimizedVersion(nil), item.OptimizedVersions...)
	item.MediaImages = append([]MediaImage(nil), item.MediaImages...)
	item.Lyrics = append([]MediaLyric(nil), item.Lyrics...)
	item.Segments = append([]MediaSegment(nil), item.Segments...)
	item.People = append([]MediaPerson(nil), item.People...)
	item.Children = append([]MediaItem(nil), item.Children...)
	item.RecommendationRows = append([]HomeRow(nil), item.RecommendationRows...)
	for index := range item.RecommendationRows {
		item.RecommendationRows[index].Items = append([]MediaItem(nil), item.RecommendationRows[index].Items...)
	}
	item.Extras = append([]MediaExtraRelationship(nil), item.Extras...)
	for index := range item.Extras {
		item.Extras[index].Items = append([]MediaItem(nil), item.Extras[index].Items...)
	}
	if item.AudioNormalization != nil {
		copy := *item.AudioNormalization
		item.AudioNormalization = &copy
	}
	const surface = "web"
	allActions := append([]string(nil), item.Actions...)
	item.Actions = actionsForSurface(allActions, surface)
	if surface == "web" && (hasPermission(user, "editMetadata") || hasPermission(user, "manageLibraries") || hasPermission(user, "deleteMedia")) {
		seen := stringSet(item.Actions)
		for _, action := range actionsForSurface(allActions, "web-admin") {
			if !seen[action] {
				item.Actions = append(item.Actions, action)
				seen[action] = true
			}
		}
	}
	canViewSourcePaths := canInteractivelyManageServer(user)
	item.SourceURL = ""
	item.ProviderIDs = nil
	item.MatchCandidates = nil
	item.IdentityEvidence = nil
	item.LockedFields = nil
	for index := range item.Streams {
		item.Streams[index].SourceURL = ""
	}
	for index := range item.MediaFiles {
		for streamIndex := range item.MediaFiles[index].Streams {
			item.MediaFiles[index].Streams[streamIndex].SourceURL = ""
		}
		if !canViewSourcePaths {
			item.MediaFiles[index].Path = ""
			item.MediaFiles[index].OriginalFilename = ""
			item.MediaFiles[index].SourceType = ""
			item.MediaFiles[index].Analysis = ""
			item.MediaFiles[index].Source = ""
			item.MediaFiles[index].ReleaseGroup = ""
		}
	}
	for index := range item.OptimizedVersions {
		if !canViewSourcePaths {
			item.OptimizedVersions[index].Path = ""
		}
	}
	for index := range item.MediaImages {
		item.MediaImages[index].Source = ""
		item.MediaImages[index].Provider = ""
		item.MediaImages[index].Path = ""
		item.MediaImages[index].RemoteURL = ""
	}
	for index := range item.Lyrics {
		item.Lyrics[index].Source = ""
		item.Lyrics[index].Provider = ""
		item.Lyrics[index].Path = ""
	}
	for index := range item.Segments {
		item.Segments[index].Source = ""
		item.Segments[index].Provider = ""
	}
	if item.AudioNormalization != nil {
		item.AudioNormalization.Source = ""
	}
	for index := range item.People {
		item.People[index].Source = ""
		item.People[index].ProviderIDs = nil
	}
	for index := range item.Children {
		item.Children[index] = consumerMediaDetailProjection(item.Children[index], user)
	}
	for rowIndex := range item.RecommendationRows {
		for mediaIndex := range item.RecommendationRows[rowIndex].Items {
			item.RecommendationRows[rowIndex].Items[mediaIndex] = consumerMediaDetailProjection(item.RecommendationRows[rowIndex].Items[mediaIndex], user)
		}
	}
	for extraIndex := range item.Extras {
		for mediaIndex := range item.Extras[extraIndex].Items {
			item.Extras[extraIndex].Items[mediaIndex] = consumerMediaDetailProjection(item.Extras[extraIndex].Items[mediaIndex], user)
		}
	}
	if item.PlaybackTarget != nil {
		projected := consumerMediaDetailProjection(*item.PlaybackTarget, user)
		item.PlaybackTarget = &projected
	}
	return item
}

func actionsForSurface(actions []string, surface string) []string {
	result := make([]string, 0, len(actions))
	for _, action := range actions {
		presentation := mediaActionPresentation(action)
		if containsString(presentation.Surfaces, surface) {
			result = append(result, action)
		}
	}
	return result
}

func alphaInitial(value string) string {
	for _, r := range strings.TrimSpace(value) {
		if unicode.IsLetter(r) {
			return strings.ToUpper(string(r))
		}
		return "#"
	}
	return "#"
}
