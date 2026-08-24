package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type categoryBlueprint struct {
	ID          string
	Name        string
	Group       string
	Description string
	Filter      string
	Source      string
}

func paginateLibraryCategories(categories []LibraryCategory, limit, offset int) ([]LibraryCategory, bool) {
	if offset >= len(categories) {
		return []LibraryCategory{}, false
	}
	end := min(len(categories), offset+limit)
	return categories[offset:end], end < len(categories)
}

type virtualAudiobookFacetIdentity struct {
	ID         string
	LibraryID  string
	EntityKind string
	Name       string
}

type virtualAudiobookFacetCursor struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type audiobookFacetObservation struct {
	mediaID     string
	displayName string
	normalized  string
	strongKey   string
}

type audiobookFacetEntityState struct {
	id          string
	identityKey string
}

func canonicalAudiobookFacetKind(entityKind string) (string, bool) {
	switch strings.TrimSpace(entityKind) {
	case "author":
		return "author", true
	case "audiobook-series":
		return "audiobook-series", true
	default:
		return "", false
	}
}

func normalizeAudiobookFacetName(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func audiobookFacetObservationSQL(entityKind string) (string, string, string, bool) {
	metadata := "CASE WHEN json_valid(typed_metadata_json) THEN typed_metadata_json ELSE '{}' END"
	switch entityKind {
	case "author":
		name := "trim(COALESCE(NULLIF(json_extract(" + metadata + ", '$.author'), ''), NULLIF(studio, ''), ''))"
		provider := "lower(trim(COALESCE(NULLIF(json_extract(" + metadata + ", '$.authorProvider'), ''), 'metadata')))"
		externalID := "trim(COALESCE(NULLIF(json_extract(" + metadata + ", '$.authorId'), ''), ''))"
		return name, provider, externalID, true
	case "audiobook-series":
		name := "trim(COALESCE(json_extract(" + metadata + ", '$.series'), ''))"
		provider := "lower(trim(COALESCE(NULLIF(json_extract(" + metadata + ", '$.seriesProvider'), ''), 'metadata')))"
		externalID := "trim(COALESCE(NULLIF(json_extract(" + metadata + ", '$.seriesId'), ''), ''))"
		return name, provider, externalID, true
	default:
		return "", "", "", false
	}
}

// reconcileAudiobookFacetEntitiesContext establishes an opaque durable entity
// before it is projected. Strong metadata keys may merge several books. When
// metadata provides only a display name, its normalized form is the fallback
// identity. Provider identities always take precedence over that weaker key.
func (s *Server) reconcileAudiobookFacetEntitiesContext(ctx context.Context, libraryID, entityKind string) error {
	entityKind, ok := canonicalAudiobookFacetKind(entityKind)
	if !ok || strings.TrimSpace(libraryID) == "" {
		return sql.ErrNoRows
	}
	nameSQL, providerSQL, externalIDSQL, _ := audiobookFacetObservationSQL(entityKind)
	changed := false
	err := s.withUserTxTagged(ctx, nil, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, `+nameSQL+`, `+providerSQL+`, `+externalIDSQL+`
			FROM media_items
			WHERE library_id = ? AND lower(type) = 'audiobook' AND `+nameSQL+` <> ''
			ORDER BY id ASC`, libraryID)
		if err != nil {
			return err
		}
		observations := []audiobookFacetObservation{}
		for rows.Next() {
			var mediaID, name, provider, externalID string
			if err := rows.Scan(&mediaID, &name, &provider, &externalID); err != nil {
				rows.Close()
				return err
			}
			observation := audiobookFacetObservation{mediaID: mediaID, displayName: strings.Join(strings.Fields(name), " ")}
			observation.normalized = normalizeAudiobookFacetName(observation.displayName)
			if externalID = strings.TrimSpace(externalID); externalID != "" {
				observation.strongKey = "provider:" + strings.ToLower(strings.TrimSpace(provider)) + ":" + externalID
			}
			observations = append(observations, observation)
		}
		if err := rows.Close(); err != nil {
			return err
		}

		entities := map[string]audiobookFacetEntityState{}
		strongEntities := map[string]string{}
		nameEntities := map[string]string{}
		entityRows, err := tx.QueryContext(ctx, `
			SELECT id, identity_key
			FROM audiobook_browse_entities
			WHERE library_id = ? AND entity_kind = ?`, libraryID, entityKind)
		if err != nil {
			return err
		}
		for entityRows.Next() {
			var state audiobookFacetEntityState
			if err := entityRows.Scan(&state.id, &state.identityKey); err != nil {
				entityRows.Close()
				return err
			}
			entities[state.id] = state
			if strings.HasPrefix(state.identityKey, "provider:") {
				strongEntities[state.identityKey] = state.id
			} else if strings.HasPrefix(state.identityKey, "name:") {
				nameEntities[state.identityKey] = state.id
			}
		}
		if err := entityRows.Close(); err != nil {
			return err
		}

		members := map[string]string{}
		memberRows, err := tx.QueryContext(ctx, `
			SELECT member.media_id, member.entity_id
			FROM audiobook_browse_entity_members member
			JOIN audiobook_browse_entities entity ON entity.id = member.entity_id
			WHERE entity.library_id = ? AND member.entity_kind = ?`, libraryID, entityKind)
		if err != nil {
			return err
		}
		for memberRows.Next() {
			var mediaID, entityID string
			if err := memberRows.Scan(&mediaID, &entityID); err != nil {
				memberRows.Close()
				return err
			}
			members[mediaID] = entityID
		}
		if err := memberRows.Close(); err != nil {
			return err
		}

		now := time.Now().UTC().Format(time.RFC3339)
		observed := make(map[string]bool, len(observations))
		displayNames := map[string]string{}
		for _, observation := range observations {
			observed[observation.mediaID] = true
			entityID := members[observation.mediaID]
			state, hasCurrent := entities[entityID]
			if observation.strongKey != "" {
				if strongID := strongEntities[observation.strongKey]; strongID != "" {
					entityID = strongID
				} else if hasCurrent && strings.HasPrefix(state.identityKey, "local:") {
					if _, err := tx.ExecContext(ctx, `UPDATE audiobook_browse_entities SET identity_key = ?, updated_at = ? WHERE id = ?`, observation.strongKey, now, entityID); err != nil {
						return err
					}
					changed = true
					state.identityKey = observation.strongKey
					entities[entityID] = state
					strongEntities[observation.strongKey] = entityID
				} else {
					entityID = randomID("aent")
					if _, err := tx.ExecContext(ctx, `
						INSERT INTO audiobook_browse_entities
							(id, library_id, entity_kind, identity_key, display_name, normalized_name, created_at, updated_at)
						VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
						entityID, libraryID, entityKind, observation.strongKey, observation.displayName, observation.normalized, now, now); err != nil {
						return err
					}
					changed = true
					entities[entityID] = audiobookFacetEntityState{id: entityID, identityKey: observation.strongKey}
					strongEntities[observation.strongKey] = entityID
				}
			} else if !hasCurrent {
				identityKey := "name:" + observation.normalized
				if existingID := nameEntities[identityKey]; existingID != "" {
					entityID = existingID
				} else {
					entityID = randomID("aent")
					if _, err := tx.ExecContext(ctx, `
					INSERT INTO audiobook_browse_entities
						(id, library_id, entity_kind, identity_key, display_name, normalized_name, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
						entityID, libraryID, entityKind, identityKey, observation.displayName, observation.normalized, now, now); err != nil {
						return err
					}
					changed = true
					entities[entityID] = audiobookFacetEntityState{id: entityID, identityKey: identityKey}
					nameEntities[identityKey] = entityID
				}
			}
			if current := displayNames[entityID]; current == "" || normalizeAudiobookFacetName(observation.displayName) < normalizeAudiobookFacetName(current) ||
				(normalizeAudiobookFacetName(observation.displayName) == normalizeAudiobookFacetName(current) && observation.displayName < current) {
				displayNames[entityID] = observation.displayName
			}
			evidenceKey := observation.strongKey
			if evidenceKey == "" {
				evidenceKey = "member:" + observation.mediaID
			}
			result, err := tx.ExecContext(ctx, `
				INSERT INTO audiobook_browse_entity_members (entity_id, entity_kind, media_id, evidence_key, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT(entity_kind, media_id) DO UPDATE SET
					entity_id = excluded.entity_id,
					evidence_key = excluded.evidence_key,
					updated_at = excluded.updated_at
				WHERE audiobook_browse_entity_members.entity_id <> excluded.entity_id
				   OR audiobook_browse_entity_members.evidence_key <> excluded.evidence_key`, entityID, entityKind, observation.mediaID, evidenceKey, now, now)
			if err != nil {
				return err
			}
			if affected, _ := result.RowsAffected(); affected > 0 {
				changed = true
			}
		}
		for mediaID, entityID := range members {
			if !observed[mediaID] {
				result, err := tx.ExecContext(ctx, `DELETE FROM audiobook_browse_entity_members WHERE entity_id = ? AND entity_kind = ? AND media_id = ?`, entityID, entityKind, mediaID)
				if err != nil {
					return err
				}
				if affected, _ := result.RowsAffected(); affected > 0 {
					changed = true
				}
			}
		}
		for entityID, displayName := range displayNames {
			result, err := tx.ExecContext(ctx, `
				UPDATE audiobook_browse_entities
				SET display_name = ?, normalized_name = ?, updated_at = ?
				WHERE id = ? AND (display_name <> ? OR normalized_name <> ?)`,
				displayName, normalizeAudiobookFacetName(displayName), now, entityID,
				displayName, normalizeAudiobookFacetName(displayName))
			if err != nil {
				return err
			}
			if affected, _ := result.RowsAffected(); affected > 0 {
				changed = true
			}
		}
		return nil
	})
	if err == nil && changed {
		s.publishDataChanged("data.changed", []string{"libraries", "library-items", "audiobook-entities"}, "database", "", nil)
	}
	return err
}

func (s *Server) virtualAudiobookFacetIdentityContext(ctx context.Context, id string) (virtualAudiobookFacetIdentity, error) {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, "aent_") || len(id) > 128 {
		return virtualAudiobookFacetIdentity{}, sql.ErrNoRows
	}
	var identity virtualAudiobookFacetIdentity
	rows, err := s.queryUserRead(ctx, `
		SELECT id, library_id, entity_kind, display_name
		FROM audiobook_browse_entities WHERE id = ?`, id)
	if err != nil {
		return virtualAudiobookFacetIdentity{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return virtualAudiobookFacetIdentity{}, err
		}
		return virtualAudiobookFacetIdentity{}, sql.ErrNoRows
	}
	if err := rows.Scan(&identity.ID, &identity.LibraryID, &identity.EntityKind, &identity.Name); err != nil {
		return virtualAudiobookFacetIdentity{}, err
	}
	return identity, nil
}

func (s *Server) audiobookFacetPageContext(ctx context.Context, userID, libraryID, entityKind string, after virtualAudiobookFacetCursor, limit int) ([]LibraryFacetValue, int, bool, error) {
	if err := s.reconcileAudiobookFacetEntitiesContext(ctx, libraryID, entityKind); err != nil {
		return nil, 0, false, err
	}
	where := `WHERE entity.library_id = ? AND entity.entity_kind = ? AND lower(m.type) = 'audiobook'`
	args := []any{libraryID, entityKind}
	where, args = s.applyMediaVisibilityRestrictionSQL(userID, where, args)
	where, args = s.applyLibraryCurationRestrictionSQL(ctx, userID, where, args)
	var total int
	totalRows, err := s.queryUserRead(ctx, `
		SELECT COUNT(DISTINCT entity.id)
		FROM audiobook_browse_entities entity
		JOIN audiobook_browse_entity_members member ON member.entity_id = entity.id AND member.entity_kind = entity.entity_kind
		JOIN media_items m ON m.id = member.media_id
		`+where, args...)
	if err != nil {
		return nil, 0, false, err
	}
	if !totalRows.Next() {
		totalRows.Close()
		return nil, 0, false, sql.ErrNoRows
	}
	if err := totalRows.Scan(&total); err != nil {
		totalRows.Close()
		return nil, 0, false, err
	}
	if err := totalRows.Close(); err != nil {
		return nil, 0, false, err
	}
	if after.ID != "" {
		where += ` AND (entity.normalized_name > ? OR (entity.normalized_name = ? AND entity.id > ?))`
		args = append(args, after.Name, after.Name, after.ID)
	}
	queryArgs := append(append([]any{}, args...), limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT entity.id, entity.display_name, entity.entity_kind, COUNT(m.id), MIN(m.id)
		FROM audiobook_browse_entities entity
		JOIN audiobook_browse_entity_members member ON member.entity_id = entity.id AND member.entity_kind = entity.entity_kind
		JOIN media_items m ON m.id = member.media_id
		`+where+`
		GROUP BY entity.id, entity.display_name, entity.entity_kind, entity.normalized_name
		ORDER BY entity.normalized_name ASC, entity.id ASC
		LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	values := make([]LibraryFacetValue, 0, limit+1)
	representativeIDs := make([]string, 0, limit+1)
	for rows.Next() {
		var value LibraryFacetValue
		var representativeID string
		if err := rows.Scan(&value.ID, &value.Name, &value.EntityKind, &value.Count, &representativeID); err != nil {
			return nil, 0, false, err
		}
		prefix := "author:"
		if value.EntityKind == "audiobook-series" {
			prefix = "series:"
		}
		value.Filter = prefix + value.Name
		values = append(values, value)
		representativeIDs = append(representativeIDs, representativeID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	hasMore := len(values) > limit
	if hasMore {
		values = values[:limit]
		representativeIDs = representativeIDs[:limit]
	}
	for index, representativeID := range representativeIDs {
		item, err := s.getMediaContext(ctx, userID, representativeID)
		if err == nil {
			values[index].Image = firstNonEmpty(item.Images.Backdrop, item.Images.Thumb, item.Images.Poster)
		}
	}
	return values, total, hasMore, nil
}

func (s *Server) listLibraryCategories(userID, libraryID string) ([]LibraryCategory, error) {
	return s.listLibraryCategoriesContext(context.Background(), userID, libraryID)
}

func (s *Server) listLibraryCategoriesContext(ctx context.Context, userID, libraryID string) ([]LibraryCategory, error) {
	return s.listLibraryCategoriesWithCacheScopeContext(ctx, userID, userID, libraryID)
}

func (s *Server) listLibraryCategoriesForUserContext(ctx context.Context, user User, libraryID string) ([]LibraryCategory, error) {
	profileID := viewerProfileID(user)
	cacheScope := homeCacheKey(user)
	if categoryProjectionCanUseSharedReadModel(user) {
		var hasLibraryState int
		err := s.queryUserRow(ctx, `
			SELECT 1
			FROM user_media_state ums
			JOIN media_items m ON m.id = ums.media_id
			WHERE ums.profile_id = ? AND m.library_id = ?
			LIMIT 1`, profileID, libraryID).Scan(&hasLibraryState)
		if errors.Is(err, sql.ErrNoRows) {
			cacheScope = emptyMediaStateCacheScope(user, false)
		}
	}
	return s.listLibraryCategoriesWithCacheScopeContext(ctx, profileID, cacheScope, libraryID)
}

func categoryProjectionCanUseSharedReadModel(user User) bool {
	role := strings.ToLower(strings.TrimSpace(user.Role))
	return role == "owner" &&
		strings.TrimSpace(user.MaxContentRating) == "" && user.MaximumAgeRating == nil && user.AllowUnrated &&
		len(user.BlockedProfileLabels) == 0 && len(user.TagPolicy.AllowedTags) == 0 &&
		len(user.TagPolicy.BlockedTags) == 0 && len(user.TagPolicy.AllowedLabels) == 0 && len(user.TagPolicy.BlockedLabels) == 0
}

func (s *Server) listLibraryCategoriesWithCacheScopeContext(ctx context.Context, userID, cacheScope, libraryID string) ([]LibraryCategory, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if categories, ok := s.cachedLibraryCategories(cacheScope, libraryID); ok {
		return categories, nil
	}
	if wait, owner := s.beginLibraryCategoriesBuild(cacheScope, libraryID); !owner {
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if categories, ok := s.cachedLibraryCategories(cacheScope, libraryID); ok {
			return categories, nil
		}
	} else if wait != nil {
		defer s.finishLibraryCategoriesBuild(cacheScope, libraryID)
	}
	library, err := s.getLibraryContext(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	blueprints := categoryBlueprints(library.Type)
	seenFilters := map[string]bool{}
	for _, blueprint := range blueprints {
		seenFilters[strings.ToLower(blueprint.Filter)] = true
	}

	custom, skipCache, err := s.libraryCategoryFacetBlueprintsContext(ctx, library.ID, library.Type, library.Name, seenFilters)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(custom, func(i, j int) bool {
		return custom[i].Name < custom[j].Name
	})
	blueprints = append(blueprints, custom...)
	customFilters := map[string]bool{}
	for _, blueprint := range custom {
		customFilters[strings.ToLower(blueprint.Filter)] = true
	}
	var aggregates map[string]categoryAggregate
	if strings.HasPrefix(cacheScope, "empty-state\x00") {
		aggregates, err = s.queryEmptyStateCategoryBlueprintAggregatesContext(ctx, library.ID, library.Type, userID, blueprints)
	} else {
		aggregates, err = s.queryLibraryCategoryBlueprintAggregatesContext(ctx, library.ID, library.Type, userID, blueprints)
	}
	if err != nil {
		return nil, err
	}

	categories := make([]LibraryCategory, 0, len(blueprints))
	for _, blueprint := range blueprints {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		category := LibraryCategory{
			ID:          blueprint.ID,
			Name:        blueprint.Name,
			Group:       blueprint.Group,
			Description: blueprint.Description,
			Filter:      blueprint.Filter,
			Source:      blueprint.Source,
		}
		if aggregate, ok := aggregates[strings.ToLower(blueprint.Filter)]; ok {
			category.Count = aggregate.count
			category.Image = aggregate.image
		}
		if !customFilters[strings.ToLower(blueprint.Filter)] || category.Count > 0 {
			categories = append(categories, category)
		}
	}
	if !skipCache {
		s.storeLibraryCategories(cacheScope, libraryID, categories)
	}
	return categories, nil
}

func (s *Server) queryEmptyStateCategoryBlueprintAggregatesContext(ctx context.Context, libraryID, libraryType, userID string, blueprints []categoryBlueprint) (map[string]categoryAggregate, error) {
	readModel, err := s.queryLibraryCategoryCountReadModelContext(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]categoryAggregate, len(blueprints))
	selects := []string{}
	selectArgs := []any{}
	filters := []string{}
	for _, blueprint := range blueprints {
		filter := strings.ToLower(strings.TrimSpace(blueprint.Filter))
		if aggregate, ok := readModel[filter]; ok {
			result[filter] = aggregate
			continue
		}
		switch filter {
		case "watchlisted", "favorite":
			result[filter] = categoryAggregate{}
			continue
		case "unwatched":
			filter = "all"
		}
		filterSQL, args, fromFacet, ok := categoryBlueprintAggregateFilterSQL(filter)
		if !ok || fromFacet {
			continue
		}
		selects = append(selects, "COALESCE(SUM(CASE WHEN "+filterSQL+" THEN 1 ELSE 0 END), 0)")
		selectArgs = append(selectArgs, args...)
		filters = append(filters, strings.ToLower(strings.TrimSpace(blueprint.Filter)))
	}
	if len(selects) == 0 {
		return result, nil
	}
	rootSQL := "m.parent_id IS NULL"
	if libraryType == "show" || libraryType == "anime" {
		rootSQL = "1 = 1"
	}
	restrictionSQL, restrictionArgs := s.categoryFacetRestrictionSQL(ctx, userID)
	queryArgs := append(selectArgs, libraryID)
	queryArgs = append(queryArgs, restrictionArgs...)
	destinations := make([]any, len(filters))
	counts := make([]int, len(filters))
	for index := range counts {
		destinations[index] = &counts[index]
	}
	if err := s.queryUserRow(ctx, `
		SELECT `+strings.Join(selects, ", ")+`
		FROM media_items m
		LEFT JOIN user_media_state ums ON ums.media_id = m.id AND ums.profile_id = ''
		WHERE m.library_id = ? AND `+rootSQL+restrictionSQL, queryArgs...).Scan(destinations...); err != nil {
		return nil, err
	}
	for index, filter := range filters {
		result[filter] = categoryAggregate{count: counts[index]}
	}
	return result, nil
}

func (s *Server) libraryCategoryFacetBlueprints(libraryID, libraryType, libraryName string, seenFilters map[string]bool) ([]categoryBlueprint, error) {
	blueprints, _, err := s.libraryCategoryFacetBlueprintsContext(context.Background(), libraryID, libraryType, libraryName, seenFilters)
	return blueprints, err
}

func (s *Server) libraryCategoryFacetBlueprintsContext(ctx context.Context, libraryID, libraryType, libraryName string, seenFilters map[string]bool) ([]categoryBlueprint, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	facets, err := s.queryLibraryCategoryFacetsContext(ctx, libraryID)
	if err != nil {
		return nil, false, err
	}
	if len(facets) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		s.queueLibraryReadModelRepair(libraryID)
		return []categoryBlueprint{}, true, nil
	}
	custom := make([]categoryBlueprint, 0, len(facets))
	for _, facet := range facets {
		blueprint, ok := categoryBlueprintForFacet(libraryType, libraryName, facet.facetType, facet.value)
		if !ok {
			continue
		}
		key := strings.ToLower(blueprint.Filter)
		if seenFilters[key] {
			continue
		}
		seenFilters[key] = true
		custom = append(custom, blueprint)
	}
	return custom, false, nil
}

type categoryFacetValue struct {
	facetType string
	value     string
}

type categoryAggregate struct {
	count int
	image string
}

const (
	maxCustomCategoryFacetsTotal    = 240
	defaultCustomCategoryFacetLimit = 40
)

var customCategoryFacetLimits = map[string]int{
	"genre":       50,
	"artist":      40,
	"albumartist": 40,
	"label":       30,
	"author":      40,
	"narrator":    30,
	"series":      40,
	"show":        60,
	"season":      40,
	"network":     30,
	"studio":      30,
}

func (s *Server) queryLibraryCategoryFacets(libraryID string) ([]categoryFacetValue, error) {
	return s.queryLibraryCategoryFacetsContext(context.Background(), libraryID)
}

func (s *Server) queryLibraryCategoryFacetsContext(ctx context.Context, libraryID string) ([]categoryFacetValue, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	facets, err := s.queryLibraryCategoryFacetsFromCountsContext(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	return facets, nil
}

func (s *Server) queryLibraryCategoryFacetsFromCountsContext(ctx context.Context, libraryID string) ([]categoryFacetValue, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT filter
		FROM library_category_counts
		WHERE library_id = ? AND count > 0
		ORDER BY count DESC, filter ASC
		LIMIT ?`, libraryID, maxCustomCategoryFacetsTotal*2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facets := []categoryFacetValue{}
	seenByType := map[string]int{}
	seenFilters := map[string]bool{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var filter string
		if err := rows.Scan(&filter); err != nil {
			return nil, err
		}
		facet, ok := categoryFacetValueFromFilter(filter)
		if !ok {
			continue
		}
		typeKey := strings.ToLower(facet.facetType)
		limit := customCategoryFacetLimits[typeKey]
		if limit <= 0 {
			limit = defaultCustomCategoryFacetLimit
		}
		if seenByType[typeKey] >= limit {
			continue
		}
		filterKey := strings.ToLower(facet.facetType + ":" + facet.value)
		if seenFilters[filterKey] {
			continue
		}
		seenFilters[filterKey] = true
		seenByType[typeKey]++
		facets = append(facets, facet)
		if len(facets) >= maxCustomCategoryFacetsTotal {
			break
		}
	}
	return facets, rows.Err()
}

func categoryFacetValueFromFilter(filter string) (categoryFacetValue, bool) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return categoryFacetValue{}, false
	}
	separator := strings.Index(filter, ":")
	if separator <= 0 || separator == len(filter)-1 {
		return categoryFacetValue{}, false
	}
	facetType := strings.TrimSpace(filter[:separator])
	value := strings.TrimSpace(filter[separator+1:])
	if facetType == "" || value == "" {
		return categoryFacetValue{}, false
	}
	facetType = canonicalCategoryFacetType(facetType)
	return categoryFacetValue{facetType: facetType, value: value}, true
}

func canonicalCategoryFacetType(facetType string) string {
	switch strings.ToLower(strings.TrimSpace(facetType)) {
	case "albumartist":
		return "albumArtist"
	case "accesslabel":
		return "accessLabel"
	default:
		return strings.TrimSpace(facetType)
	}
}

func (s *Server) queryLibraryCategoryFacetAggregates(libraryID, userID string) (map[string]categoryAggregate, error) {
	return s.queryLibraryCategoryFacetAggregatesContext(context.Background(), libraryID, userID)
}

func (s *Server) queryLibraryCategoryFacetAggregatesContext(ctx context.Context, libraryID, userID string) (map[string]categoryAggregate, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	restrictionSQL, restrictionArgs := s.categoryFacetRestrictionSQL(ctx, userID)
	if restrictionSQL == "" {
		aggregates, err := s.queryLibraryCategoryCountReadModelContext(ctx, libraryID)
		if err != nil {
			return nil, err
		}
		if len(aggregates) == 0 {
			s.queueLibraryReadModelRepair(libraryID)
		}
		return aggregates, nil
	}
	args := append([]any{libraryID}, restrictionArgs...)
	rows, err := s.queryUserRead(ctx, `
		SELECT f.facet_type, f.value, COUNT(1), MIN(m.sort_title || char(31) || m.id)
		FROM media_category_facets f
		JOIN media_items m ON m.id = f.media_id
		WHERE f.library_id = ?
		`+restrictionSQL+`
		GROUP BY f.facet_type, f.value
		ORDER BY f.facet_type ASC, MIN(f.sort_value) ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	aggregates := map[string]categoryAggregate{}
	representativeIDs := map[string]string{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var facetType, value string
		var count int
		var representativeKey string
		if err := rows.Scan(&facetType, &value, &count, &representativeKey); err != nil {
			return nil, err
		}
		filter := strings.ToLower(facetType + ":" + strings.TrimSpace(value))
		aggregates[filter] = categoryAggregate{count: count}
		if separator := strings.LastIndex(representativeKey, string(rune(31))); separator >= 0 {
			itemID := strings.TrimSpace(representativeKey[separator+1:])
			if itemID == "" {
				continue
			}
			representativeIDs[filter] = itemID
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachCategoryAggregateImagesContext(ctx, aggregates, representativeIDs); err != nil {
		return nil, err
	}
	return aggregates, nil
}

func (s *Server) attachCategoryAggregateImagesContext(ctx context.Context, aggregates map[string]categoryAggregate, representativeIDs map[string]string) error {
	if len(aggregates) == 0 || len(representativeIDs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(representativeIDs))
	for _, id := range representativeIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT id, art_seed, title, COALESCE(artwork_json, '{}')
		FROM media_items
		WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	imagesByID := map[string]string{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var id, artSeed, title, artworkJSON string
		if err := rows.Scan(&id, &artSeed, &title, &artworkJSON); err != nil {
			return err
		}
		var artwork map[string]string
		_ = json.Unmarshal([]byte(artworkJSON), &artwork)
		images := imageSetFor(id, artSeed, title, artwork)
		imagesByID[id] = firstNonEmpty(images.Backdrop, images.Thumb, images.Poster)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for filter, itemID := range representativeIDs {
		aggregate := aggregates[filter]
		if aggregate.image == "" {
			aggregate.image = imagesByID[itemID]
			aggregates[filter] = aggregate
		}
	}
	return nil
}

func (s *Server) queryLibraryCategoryCountReadModelContext(ctx context.Context, libraryID string) (map[string]categoryAggregate, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT filter, count, representative_image
		FROM library_category_counts
		WHERE library_id = ?`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	aggregates := map[string]categoryAggregate{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var filter, image string
		var count int
		if err := rows.Scan(&filter, &count, &image); err != nil {
			return nil, err
		}
		filter = strings.ToLower(strings.TrimSpace(filter))
		if filter == "" {
			continue
		}
		aggregates[filter] = categoryAggregate{count: count, image: image}
	}
	return aggregates, rows.Err()
}

func (s *Server) queryLibraryCategoryBlueprintAggregatesContext(ctx context.Context, libraryID, libraryType, userID string, blueprints []categoryBlueprint) (map[string]categoryAggregate, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	restrictionSQL, restrictionArgs := s.categoryFacetRestrictionSQL(ctx, userID)
	readModelAggregates := map[string]categoryAggregate{}
	if restrictionSQL == "" {
		var err error
		readModelAggregates, err = s.queryLibraryCategoryCountReadModelContext(ctx, libraryID)
		if err != nil {
			return nil, err
		}
	}
	aggregates := make(map[string]categoryAggregate, len(blueprints))
	for _, blueprint := range blueprints {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		filter := strings.ToLower(strings.TrimSpace(blueprint.Filter))
		if filter == "" {
			continue
		}
		if aggregate, ok := readModelAggregates[filter]; ok {
			aggregates[filter] = aggregate
			continue
		}
		aggregate, err := s.queryLibraryCategoryBlueprintAggregateContext(ctx, libraryID, libraryType, userID, blueprint.Filter, restrictionSQL, restrictionArgs)
		if err != nil {
			return nil, err
		}
		aggregates[filter] = aggregate
	}
	return aggregates, nil
}

func (s *Server) queryLibraryCategoryBlueprintAggregateContext(ctx context.Context, libraryID, libraryType, userID, filter, restrictionSQL string, restrictionArgs []any) (categoryAggregate, error) {
	query, args, ok := categoryBlueprintAggregateQuery(libraryID, libraryType, userID, filter, restrictionSQL, restrictionArgs, true)
	if !ok {
		return categoryAggregate{}, nil
	}
	var aggregate categoryAggregate
	if err := s.queryUserRow(ctx, query, args...).Scan(&aggregate.count); err != nil {
		return categoryAggregate{}, err
	}
	if aggregate.count <= 0 {
		return aggregate, nil
	}
	query, args, ok = categoryBlueprintAggregateQuery(libraryID, libraryType, userID, filter, restrictionSQL, restrictionArgs, false)
	if !ok {
		return aggregate, nil
	}
	var itemID, artSeed, title, artworkJSON string
	if err := s.queryUserRow(ctx, query, args...).Scan(&itemID, &artSeed, &title, &artworkJSON); err != nil {
		if err == sql.ErrNoRows {
			return aggregate, nil
		}
		return categoryAggregate{}, err
	}
	var artwork map[string]string
	_ = json.Unmarshal([]byte(artworkJSON), &artwork)
	images := imageSetFor(itemID, artSeed, title, artwork)
	aggregate.image = firstNonEmpty(images.Backdrop, images.Thumb, images.Poster)
	return aggregate, nil
}

func categoryBlueprintAggregateQuery(libraryID, libraryType, userID, filter, restrictionSQL string, restrictionArgs []any, countOnly bool) (string, []any, bool) {
	rootSQL := "m.parent_id IS NULL"
	if libraryType == "show" || libraryType == "anime" {
		rootSQL = "1 = 1"
	}
	filterSQL, filterArgs, fromFacet, ok := categoryBlueprintAggregateFilterSQL(filter)
	if !ok {
		return "", nil, false
	}
	args := []any{}
	fromSQL := "media_items m"
	whereSQL := "m.library_id = ? AND " + rootSQL
	if fromFacet {
		fromSQL = "media_category_facets f JOIN media_items m ON m.id = f.media_id"
		whereSQL = "f.library_id = ? AND " + rootSQL
	}
	if userID != "" {
		fromSQL += " LEFT JOIN user_media_state ums ON ums.media_id = m.id AND ums.profile_id = ?"
		args = append(args, userID)
	} else if strings.Contains(filterSQL, "ums.") {
		fromSQL += " LEFT JOIN user_media_state ums ON ums.media_id = m.id AND ums.profile_id = ''"
	}
	args = append(args, libraryID)
	args = append(args, filterArgs...)
	args = append(args, restrictionArgs...)
	selectSQL := "COUNT(1)"
	orderLimitSQL := ""
	if !countOnly {
		selectSQL = "m.id, m.art_seed, m.title, COALESCE(m.artwork_json, '{}')"
		orderLimitSQL = " ORDER BY m.sort_title ASC, m.id ASC LIMIT 1"
	}
	query := "SELECT " + selectSQL + " FROM " + fromSQL + " WHERE " + whereSQL + " AND " + filterSQL + restrictionSQL + orderLimitSQL
	return query, args, true
}

func categoryBlueprintAggregateFilterSQL(filter string) (string, []any, bool, bool) {
	filter = strings.TrimSpace(filter)
	switch filter {
	case "all", "":
		return "1 = 1", nil, false, true
	case "watchlisted":
		return "COALESCE(ums.watchlisted, 0) = 1", nil, false, true
	case "favorite":
		return "COALESCE(ums.favorite, 0) = 1", nil, false, true
	case "unwatched":
		return "COALESCE(ums.watched, 0) = 0", nil, false, true
	case "recent":
		return "m.added_at >= ?", []any{time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)}, false, true
	case "rating:top":
		return "(m.community_rating >= 8 OR m.critic_rating >= 85 OR COALESCE(ums.rating, 0) >= 8)", nil, false, true
	case "audience:family":
		return "upper(replace(trim(COALESCE(m.content_rating, '')), '_', '-')) IN ('G', 'PG', 'TV-G', 'TV-Y', 'TV-Y7', 'TV-PG')", nil, false, true
	}
	if strings.HasPrefix(filter, "decade:") {
		decade, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(filter, "decade:")))
		if err != nil {
			return "", nil, false, false
		}
		return "m.year >= ? AND m.year < ?", []any{decade, decade + 10}, false, true
	}
	facetType, value, ok := strings.Cut(filter, ":")
	if !ok {
		return "", nil, false, false
	}
	facetType = strings.TrimSpace(facetType)
	value = strings.ToLower(strings.TrimSpace(value))
	switch facetType {
	case "genre", "artist", "albumArtist", "trackArtist", "label", "author", "narrator", "series", "network", "studio", "show", "season":
		return "f.facet_type = ? AND f.sort_value = ?", []any{facetType, value}, true, true
	case "contentRating":
		return "m.content_rating = ?", []any{strings.TrimSpace(strings.TrimPrefix(filter, "contentRating:"))}, false, true
	case "type":
		return "m.type = ?", []any{strings.TrimSpace(strings.TrimPrefix(filter, "type:"))}, false, true
	default:
		return "", nil, false, false
	}
}

func (s *Server) categoryFacetRestrictionSQL(ctx context.Context, userID string) (string, []any) {
	restrictionSQL := ""
	userRestrictionSQL, restrictionArgs := s.mediaVisibilityRestrictionSQL(userID)
	restrictionSQL += userRestrictionSQL
	curationSQL, curationArgs := s.libraryCurationRestrictionSQL(ctx, userID)
	if curationSQL == "" {
		return restrictionSQL, restrictionArgs
	}
	args := append([]any{}, restrictionArgs...)
	args = append(args, curationArgs...)
	return restrictionSQL + curationSQL, args
}

func contentRatingRankSQL(column string) string {
	normalized := "upper(replace(trim(COALESCE(" + column + ", '')), '_', '-'))"
	return `CASE
		WHEN ` + normalized + ` IN ('TV-Y', 'TV-G', 'G', 'U', 'C') THEN 1
		WHEN ` + normalized + ` IN ('TV-Y7', 'C8') THEN 2
		WHEN ` + normalized + ` IN ('TV-PG', 'PG') THEN 3
		WHEN ` + normalized + ` IN ('TV-14', 'PG-13', '12', '12A', '14A') THEN 4
		WHEN ` + normalized + ` IN ('R', '15', '18A') THEN 5
		WHEN ` + normalized + ` IN ('TV-MA', 'NC-17', '18', 'A') THEN 6
		ELSE 0
	END`
}

func sqlPlaceholders(count int) string {
	if count <= 0 {
		return ""
	}
	placeholders := make([]string, count)
	for i := range placeholders {
		placeholders[i] = "?"
	}
	return strings.Join(placeholders, ", ")
}

func (s *Server) rebuildLibraryCategoryFacets(libraryID string) error {
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return s.withBackgroundTxTagged(context.Background(), []string{"libraries", "library-items"}, func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id, metadata_revision FROM media_items WHERE library_id = ? ORDER BY id`, libraryID)
		if err != nil {
			return fmt.Errorf("load library facet projection inputs: %w", err)
		}
		type projection struct {
			mediaID  string
			revision int
		}
		items := []projection{}
		for rows.Next() {
			var item projection
			if err := rows.Scan(&item.mediaID, &item.revision); err != nil {
				_ = rows.Close()
				return err
			}
			items = append(items, item)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM media_category_facets WHERE library_id = ?`, libraryID); err != nil {
			return fmt.Errorf("clear library category facets: %w", err)
		}
		for _, item := range items {
			if err := replaceMediaCategoryFacetsTx(context.Background(), tx, item.mediaID, item.revision, "read-model-repair"); err != nil {
				return fmt.Errorf("rebuild facets for %s: %w", item.mediaID, err)
			}
		}
		if err := rebuildLibraryCategoryCountsTx(tx, libraryID, now); err != nil {
			return err
		}
		return nil
	})
}
func (s *Server) replaceMediaCategoryFacets(mediaID string) error {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return nil
	}
	return s.withBackgroundTxTagged(context.Background(), []string{"libraries", "library-items"}, func(tx *sql.Tx) error {
		return replaceMediaCategoryFacetsTx(context.Background(), tx, mediaID, 0, "legacy")
	})
}

func normalizeAccessLabelFacetSortValuesTx(tx *sql.Tx, scopeColumn, scopeID string) error {
	if tx == nil || (scopeColumn != "library_id" && scopeColumn != "media_id") {
		return errors.New("invalid access-label facet normalization scope")
	}
	rows, err := tx.Query(`SELECT rowid, value FROM media_category_facets WHERE facet_type = 'accessLabel' AND `+scopeColumn+` = ?`, scopeID)
	if err != nil {
		return fmt.Errorf("read access-label facets for normalization: %w", err)
	}
	type facet struct {
		rowID int64
		value string
	}
	var facets []facet
	for rows.Next() {
		var item facet
		if err := rows.Scan(&item.rowID, &item.value); err != nil {
			rows.Close()
			return err
		}
		facets = append(facets, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range facets {
		if _, err := tx.Exec(`UPDATE media_category_facets SET sort_value = ? WHERE rowid = ?`, normalizeProfilePolicyComparable(item.value), item.rowID); err != nil {
			return fmt.Errorf("normalize access-label facet: %w", err)
		}
	}
	return nil
}

func rebuildLibraryCategoryCountsTx(tx *sql.Tx, libraryID, now string) error {
	if _, err := tx.Exec(`DELETE FROM library_category_counts WHERE library_id = ?`, libraryID); err != nil {
		return fmt.Errorf("clear library category counts: %w", err)
	}
	rows, err := tx.Query(`
		SELECT f.facet_type, f.value, m.id, m.art_seed, m.title, COALESCE(m.artwork_json, '{}')
		FROM media_category_facets f
		JOIN media_items m ON m.id = f.media_id
		WHERE f.library_id = ?
		ORDER BY f.facet_type ASC, f.sort_value ASC, m.sort_title ASC`, libraryID)
	if err != nil {
		return err
	}
	defer rows.Close()
	aggregates := map[string]categoryAggregate{}
	representatives := map[string]string{}
	for rows.Next() {
		var facetType, value string
		var itemID, artSeed, title, artworkJSON string
		if err := rows.Scan(&facetType, &value, &itemID, &artSeed, &title, &artworkJSON); err != nil {
			return err
		}
		filter := canonicalCategoryFacetType(facetType) + ":" + strings.TrimSpace(value)
		aggregate := aggregates[filter]
		aggregate.count++
		if aggregate.image == "" {
			var artwork map[string]string
			_ = json.Unmarshal([]byte(artworkJSON), &artwork)
			images := imageSetFor(itemID, artSeed, title, artwork)
			aggregate.image = firstNonEmpty(images.Backdrop, images.Thumb, images.Poster)
			representatives[filter] = itemID
		}
		aggregates[filter] = aggregate
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for filter, aggregate := range aggregates {
		if _, err := tx.Exec(`
			INSERT OR REPLACE INTO library_category_counts (library_id, filter, count, representative_media_id, representative_image, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			libraryID, filter, aggregate.count, representatives[filter], aggregate.image, now); err != nil {
			return fmt.Errorf("store library category count %s: %w", filter, err)
		}
	}
	return rebuildLibraryBuiltinCategoryCountsTx(tx, libraryID, now)
}

// rebuildLibraryBuiltinCategoryCountsTx materializes catalogue-only quick-pick
// and decade counts beside facet counts. Empty-state category requests can then
// remain O(number of categories) instead of scanning every media row inside a
// foreground deadline. Viewer-owned watchlist/favorite state is intentionally
// excluded from this shared read model.
func rebuildLibraryBuiltinCategoryCountsTx(tx *sql.Tx, libraryID, now string) error {
	var libraryType string
	if err := tx.QueryRow(`SELECT type FROM libraries WHERE id = ?`, libraryID).Scan(&libraryType); err != nil {
		return err
	}
	filters := []string{}
	selects := []string{}
	args := []any{}
	seen := map[string]bool{}
	for _, blueprint := range categoryBlueprints(libraryType) {
		filter := strings.ToLower(strings.TrimSpace(blueprint.Filter))
		if filter == "" || filter == "watchlisted" || filter == "favorite" || seen[filter] {
			continue
		}
		seen[filter] = true
		effectiveFilter := filter
		if effectiveFilter == "unwatched" {
			effectiveFilter = "all"
		}
		filterSQL, filterArgs, fromFacet, ok := categoryBlueprintAggregateFilterSQL(effectiveFilter)
		if effectiveFilter == "rating:top" {
			// The shared catalogue projection cannot include a viewer's personal
			// rating. Personal state is layered separately at request time.
			filterSQL = "(m.community_rating >= 8 OR m.critic_rating >= 85)"
			filterArgs, fromFacet, ok = nil, false, true
		}
		if !ok || fromFacet {
			continue
		}
		filters = append(filters, filter)
		selects = append(selects, "COALESCE(SUM(CASE WHEN "+filterSQL+" THEN 1 ELSE 0 END), 0)")
		args = append(args, filterArgs...)
	}
	if len(selects) == 0 {
		return nil
	}
	rootSQL := "m.parent_id IS NULL"
	if libraryType == "show" || libraryType == "anime" {
		rootSQL = "1 = 1"
	}
	args = append(args, libraryID)
	counts := make([]int, len(filters))
	destinations := make([]any, len(filters))
	for index := range counts {
		destinations[index] = &counts[index]
	}
	if err := tx.QueryRow(`SELECT `+strings.Join(selects, ", ")+` FROM media_items m WHERE m.library_id = ? AND `+rootSQL, args...).Scan(destinations...); err != nil {
		return fmt.Errorf("build library quick-pick counts: %w", err)
	}
	for index, filter := range filters {
		if _, err := tx.Exec(`
			INSERT INTO library_category_counts (library_id, filter, count, representative_media_id, representative_image, updated_at)
			VALUES (?, ?, ?, '', '', ?)
			ON CONFLICT(library_id, filter) DO UPDATE SET count = excluded.count, updated_at = excluded.updated_at`,
			libraryID, filter, counts[index], now); err != nil {
			return fmt.Errorf("store library quick-pick count %s: %w", filter, err)
		}
	}
	return nil
}

func (s *Server) cachedLibraryCategories(userID, libraryID string) ([]LibraryCategory, bool) {
	key := categoryCacheKey(userID, libraryID)
	if key == "" {
		return nil, false
	}
	now := time.Now()
	s.categoryCacheMu.Lock()
	defer s.categoryCacheMu.Unlock()
	if s.categoryCache == nil {
		s.categoryCache = map[string]categoryCacheEntry{}
		return nil, false
	}
	entry, ok := s.categoryCache[key]
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		delete(s.categoryCache, key)
		return nil, false
	}
	return cloneLibraryCategories(entry.categories), true
}

func (s *Server) storeLibraryCategories(userID, libraryID string, categories []LibraryCategory) {
	key := categoryCacheKey(userID, libraryID)
	if key == "" {
		return
	}
	s.categoryCacheMu.Lock()
	defer s.categoryCacheMu.Unlock()
	if s.categoryCache == nil {
		s.categoryCache = map[string]categoryCacheEntry{}
	}
	now := time.Now()
	for cachedKey, entry := range s.categoryCache {
		if !now.Before(entry.expiresAt) {
			delete(s.categoryCache, cachedKey)
		}
	}
	for len(s.categoryCache) >= 512 {
		for cachedKey := range s.categoryCache {
			delete(s.categoryCache, cachedKey)
			break
		}
	}
	s.categoryCache[key] = categoryCacheEntry{
		categories: cloneLibraryCategories(categories),
		expiresAt:  now.Add(categoryCacheTTL),
	}
}

func (s *Server) beginLibraryCategoriesBuild(userID, libraryID string) (chan struct{}, bool) {
	key := categoryCacheKey(userID, libraryID)
	if key == "" {
		return nil, true
	}
	s.categoryCacheMu.Lock()
	defer s.categoryCacheMu.Unlock()
	if s.categoryInFlight == nil {
		s.categoryInFlight = map[string]chan struct{}{}
	}
	if wait, ok := s.categoryInFlight[key]; ok {
		return wait, false
	}
	wait := make(chan struct{})
	s.categoryInFlight[key] = wait
	return wait, true
}

func (s *Server) finishLibraryCategoriesBuild(userID, libraryID string) {
	key := categoryCacheKey(userID, libraryID)
	if key == "" {
		return
	}
	s.categoryCacheMu.Lock()
	defer s.categoryCacheMu.Unlock()
	wait, ok := s.categoryInFlight[key]
	if !ok {
		return
	}
	delete(s.categoryInFlight, key)
	close(wait)
}

func (s *Server) invalidateCategoryCache() {
	s.categoryCacheMu.Lock()
	defer s.categoryCacheMu.Unlock()
	if len(s.categoryCache) > 0 {
		s.categoryCache = map[string]categoryCacheEntry{}
	}
}

func categoryCacheKey(userID, libraryID string) string {
	userID = strings.TrimSpace(userID)
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return ""
	}
	return userID + "\x00" + libraryID
}

func cloneLibraryCategories(categories []LibraryCategory) []LibraryCategory {
	if len(categories) == 0 {
		return []LibraryCategory{}
	}
	cloned := make([]LibraryCategory, len(categories))
	copy(cloned, categories)
	return cloned
}

func (s *Server) libraryCategoryItems(userID, libraryID, libraryType string) ([]MediaItem, error) {
	return s.libraryCategoryItemsContext(context.Background(), userID, libraryID, libraryType)
}

func (s *Server) libraryCategoryItemsContext(ctx context.Context, userID, libraryID, libraryType string) ([]MediaItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	where := "WHERE m.library_id = ? AND m.parent_id IS NULL"
	if libraryType == "show" || libraryType == "anime" {
		where = "WHERE m.library_id = ?"
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT
			m.id, COALESCE(m.library_id, ''), COALESCE(m.parent_id, ''), m.type, m.title,
			m.year, m.content_rating, m.community_rating, m.critic_rating, m.studio, m.network,
			m.genres_json, m.added_at, m.art_seed, COALESCE(m.artwork_json, '{}'), COALESCE(m.typed_metadata_json, '{}'),
			COALESCE(ums.watchlisted, 0), COALESCE(ums.favorite, 0), COALESCE(ums.watched, 0), COALESCE(ums.rating, 0),
			COALESCE(parent.title, ''), COALESCE(grandparent.id, ''), COALESCE(grandparent.title, '')
		FROM media_items m
		LEFT JOIN user_media_state ums ON ums.media_id = m.id AND ums.profile_id = ?
		LEFT JOIN media_items parent ON parent.id = m.parent_id
		LEFT JOIN media_items grandparent ON grandparent.id = parent.parent_id
		`+where+`
		ORDER BY m.sort_title ASC`,
		userID, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []MediaItem{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var item MediaItem
		var genresJSON string
		var artworkJSON string
		var typedMetadataJSON string
		var watchlisted, favorite, watched int
		if err := rows.Scan(
			&item.ID, &item.LibraryID, &item.ParentID, &item.Type, &item.Title,
			&item.Year, &item.ContentRating, &item.CommunityRating, &item.CriticRating, &item.Studio, &item.Network,
			&genresJSON, &item.AddedAt, &item.ArtSeed, &artworkJSON, &typedMetadataJSON,
			&watchlisted, &favorite, &watched, &item.State.Rating,
			&item.ParentTitle, &item.GrandparentID, &item.GrandparentTitle,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genresJSON), &item.Genres)
		_ = json.Unmarshal([]byte(artworkJSON), &item.Artwork)
		_ = json.Unmarshal([]byte(typedMetadataJSON), &item.TypedMetadata)
		item.TypedMetadata = sanitizeTypedMetadataForType(item.Type, normalizeTypedMetadataMap(item.TypedMetadata))
		item.State.Watchlisted = watchlisted == 1
		item.State.Favorite = favorite == 1
		item.State.Watched = watched == 1
		item.Images = imageSetFor(item.ID, item.ArtSeed, item.Title, item.Artwork)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.applyUserContentRestrictionsContext(ctx, userID, s.applyUserLibraryRestrictionsContext(ctx, userID, items)), nil
}

func typedCategoryFacets(libraryType string, item MediaItem) []categoryBlueprint {
	metadata := item.TypedMetadata
	if metadata == nil {
		metadata = map[string]string{}
	}
	var facets []categoryBlueprint
	add := func(prefix, name, group, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		facets = append(facets, categoryBlueprint{
			ID:          categoryID(prefix, name),
			Name:        name,
			Group:       group,
			Description: fmt.Sprintf("Browse %s.", name),
			Filter:      prefix + ":" + name,
			Source:      source,
		})
	}
	switch libraryType {
	case "music":
		add("artist", firstNonEmpty(metadata["trackArtist"], metadata["artist"], item.GrandparentTitle, item.Studio), "Artists", "Music metadata")
		add("albumArtist", firstNonEmpty(metadata["albumArtist"], metadata["artist"], item.Studio), "Album Artists", "Music metadata")
		add("label", metadata["label"], "Record Labels", "Music metadata")
	case "audiobook":
		add("author", firstNonEmpty(metadata["author"], item.Studio), "Authors", "Audiobook metadata")
		add("narrator", metadata["narrator"], "Narrators", "Audiobook metadata")
		add("series", metadata["series"], "Series", "Audiobook metadata")
	case "show", "anime":
		if item.Type == "show" || item.Type == "anime" {
			add("show", item.Title, "Shows", "TV hierarchy")
		}
		if item.Type == "season" {
			add("show", item.ParentTitle, "Shows", "TV hierarchy")
			add("season", item.Title, "Seasons", "TV hierarchy")
		}
		if item.Type == "episode" {
			add("show", item.GrandparentTitle, "Shows", "TV hierarchy")
			add("season", item.ParentTitle, "Seasons", "TV hierarchy")
		}
		add("network", firstNonEmpty(metadata["network"], item.Network), "Networks", "TV metadata")
		add("studio", firstNonEmpty(metadata["studio"], item.Studio), "Studios", "TV metadata")
	}
	return facets
}

func categoryBlueprintForFacet(libraryType, libraryName, facetType, value string) (categoryBlueprint, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return categoryBlueprint{}, false
	}
	blueprint := categoryBlueprint{
		ID:          categoryID(facetType, value),
		Name:        value,
		Description: fmt.Sprintf("Browse %s.", value),
		Filter:      facetType + ":" + value,
		Source:      "Library metadata",
	}
	switch facetType {
	case "genre":
		blueprint.Group = "Library Genres"
		blueprint.Description = fmt.Sprintf("%s items using the %s genre.", libraryName, value)
	case "artist":
		if libraryType != "music" {
			return categoryBlueprint{}, false
		}
		blueprint.Group = "Artists"
		blueprint.Source = "Music metadata"
	case "albumArtist":
		if libraryType != "music" {
			return categoryBlueprint{}, false
		}
		blueprint.Group = "Album Artists"
		blueprint.Source = "Music metadata"
	case "label":
		if libraryType != "music" {
			return categoryBlueprint{}, false
		}
		blueprint.Group = "Record Labels"
		blueprint.Source = "Music metadata"
	case "author":
		if libraryType != "audiobook" {
			return categoryBlueprint{}, false
		}
		blueprint.Group = "Authors"
		blueprint.Source = "Audiobook metadata"
	case "narrator":
		if libraryType != "audiobook" {
			return categoryBlueprint{}, false
		}
		blueprint.Group = "Narrators"
		blueprint.Source = "Audiobook metadata"
	case "series":
		if libraryType != "audiobook" {
			return categoryBlueprint{}, false
		}
		blueprint.Group = "Series"
		blueprint.Source = "Audiobook metadata"
	case "show":
		if libraryType != "show" && libraryType != "anime" {
			return categoryBlueprint{}, false
		}
		blueprint.Group = "Shows"
		blueprint.Source = "TV hierarchy"
	case "season":
		if libraryType != "show" && libraryType != "anime" {
			return categoryBlueprint{}, false
		}
		blueprint.Group = "Seasons"
		blueprint.Source = "TV hierarchy"
	case "network":
		if libraryType != "show" && libraryType != "anime" {
			return categoryBlueprint{}, false
		}
		blueprint.Group = "Networks"
		blueprint.Source = "TV metadata"
	case "studio":
		if libraryType != "show" && libraryType != "anime" {
			return categoryBlueprint{}, false
		}
		blueprint.Group = "Studios"
		blueprint.Source = "TV metadata"
	default:
		return categoryBlueprint{}, false
	}
	return blueprint, true
}

func categoryBlueprints(libraryType string) []categoryBlueprint {
	blueprints := append([]categoryBlueprint{}, categoryQuickPicks()...)
	switch libraryType {
	case "movie":
		blueprints = append(blueprints, genreBlueprints("Genres", "TMDB movie genre", []string{
			"Action", "Adventure", "Animation", "Comedy", "Crime", "Documentary", "Drama", "Family",
			"Fantasy", "History", "Horror", "Music", "Mystery", "Romance", "Science Fiction",
			"TV Movie", "Thriller", "War", "Western",
		})...)
		blueprints = append(blueprints, decadeBlueprints([]int{2020, 2010, 2000, 1990})...)
	case "show":
		blueprints = append(blueprints, genreBlueprints("Genres", "TMDB TV genre", []string{
			"Action & Adventure", "Animation", "Comedy", "Crime", "Documentary", "Drama", "Family",
			"Kids", "Mystery", "News", "Reality", "Sci-Fi & Fantasy", "Soap", "Talk",
			"War & Politics", "Western",
		})...)
		blueprints = append(blueprints, genreBlueprints("Library Genres", "Library metadata", []string{
			"Thriller", "Science Fiction", "Fantasy",
		})...)
		blueprints = append(blueprints, decadeBlueprints([]int{2020, 2010, 2000, 1990})...)
	case "anime":
		blueprints = append(blueprints, genreBlueprints("Genres", "Anime metadata vocabulary", []string{
			"Action", "Adventure", "Comedy", "Drama", "Fantasy", "Horror", "Mystery", "Romance",
			"Science Fiction", "Slice of Life", "Sports", "Supernatural", "Thriller",
		})...)
		blueprints = append(blueprints, genreBlueprints("Anime Styles", "Anime metadata vocabulary", []string{
			"Isekai", "Mecha", "Shonen", "Shojo", "Seinen", "Josei", "Cyberpunk", "Magical Girl",
		})...)
	case "music":
		blueprints = append(blueprints, genreBlueprints("Genres", "Music metadata vocabulary", []string{
			"Alternative", "Ambient", "Blues", "Classical", "Country", "Electronic", "Folk",
			"Hip-Hop", "Jazz", "Latin", "Metal", "Pop", "Punk", "R&B", "Rock", "Singer-Songwriter",
			"Soul", "Soundtrack", "World",
		})...)
	case "audiobook":
		blueprints = append(blueprints, genreBlueprints("Genres", "Audiobook metadata vocabulary", []string{
			"Biography", "Business", "Essays", "Fantasy", "Fiction", "History", "Kids", "Memoir",
			"Mystery", "Nonfiction", "Romance", "Science", "Science Fiction", "Self-Help",
			"Thriller", "Young Adult",
		})...)
	}
	return dedupeCategoryBlueprints(blueprints)
}

func categoryQuickPicks() []categoryBlueprint {
	return []categoryBlueprint{
		{ID: "recently-added", Name: "Recently Added", Group: "Quick Picks", Description: "Newer items in this library.", Filter: "recent", Source: "Portico"},
		{ID: "continue-unwatched", Name: "Unwatched", Group: "Quick Picks", Description: "Items not marked watched.", Filter: "unwatched", Source: "Portico"},
		{ID: "watchlist", Name: "Watchlist", Group: "Quick Picks", Description: "Items saved for later.", Filter: "watchlisted", Source: "Portico"},
		{ID: "favorites", Name: "Favorites", Group: "Quick Picks", Description: "Items explicitly marked as favorites.", Filter: "favorite", Source: "Portico"},
		{ID: "highly-rated", Name: "Highly Rated", Group: "Quick Picks", Description: "Items with strong critic or audience ratings.", Filter: "rating:top", Source: "Portico"},
		{ID: "family-friendly", Name: "Family Friendly", Group: "Quick Picks", Description: "Items with family-safe content ratings.", Filter: "audience:family", Source: "Portico"},
	}
}

func genreBlueprints(group, source string, names []string) []categoryBlueprint {
	blueprints := make([]categoryBlueprint, 0, len(names))
	for _, name := range names {
		blueprints = append(blueprints, categoryBlueprint{
			ID:          categoryID("genre", name),
			Name:        name,
			Group:       group,
			Description: fmt.Sprintf("Browse %s.", strings.ToLower(name)),
			Filter:      "genre:" + name,
			Source:      source,
		})
	}
	return blueprints
}

func decadeBlueprints(decades []int) []categoryBlueprint {
	blueprints := make([]categoryBlueprint, 0, len(decades))
	for _, decade := range decades {
		name := fmt.Sprintf("%ds", decade)
		blueprints = append(blueprints, categoryBlueprint{
			ID:          categoryID("decade", strconv.Itoa(decade)),
			Name:        name,
			Group:       "Decades",
			Description: fmt.Sprintf("Released from %d through %d.", decade, decade+9),
			Filter:      fmt.Sprintf("decade:%d", decade),
			Source:      "Portico",
		})
	}
	return blueprints
}

func dedupeCategoryBlueprints(blueprints []categoryBlueprint) []categoryBlueprint {
	seen := map[string]bool{}
	deduped := make([]categoryBlueprint, 0, len(blueprints))
	for _, blueprint := range blueprints {
		key := strings.ToLower(blueprint.Filter)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, blueprint)
	}
	return deduped
}

func categoryMatches(item MediaItem, filter string, now time.Time) bool {
	switch filter {
	case "all", "":
		return true
	case "watchlisted":
		return item.State.Watchlisted
	case "favorite":
		return item.State.Favorite
	case "unwatched":
		return !item.State.Watched
	case "recent":
		addedAt, err := time.Parse(time.RFC3339, item.AddedAt)
		return err == nil && addedAt.After(now.Add(-30*24*time.Hour))
	case "rating:top":
		return item.CommunityRating >= 8 || item.CriticRating >= 85 || item.State.Rating >= 8
	case "audience:family":
		return isFamilyContentRating(item.ContentRating)
	}
	if strings.HasPrefix(filter, "genre:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "genre:"))
		return hasGenre(item, value)
	}
	if strings.HasPrefix(filter, "contentRating:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "contentRating:"))
		return strings.EqualFold(item.ContentRating, value)
	}
	if strings.HasPrefix(filter, "artist:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "artist:"))
		return typedMetadataEquals(item, value, "trackArtist", "albumArtist", "artist") || strings.EqualFold(item.GrandparentTitle, value) || strings.EqualFold(item.Studio, value)
	}
	if strings.HasPrefix(filter, "albumArtist:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "albumArtist:"))
		return typedMetadataEquals(item, value, "albumArtist", "artist") || strings.EqualFold(item.Studio, value)
	}
	if strings.HasPrefix(filter, "label:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "label:"))
		return typedMetadataEquals(item, value, "label")
	}
	if strings.HasPrefix(filter, "author:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "author:"))
		return typedMetadataEquals(item, value, "author") || strings.EqualFold(item.Studio, value)
	}
	if strings.HasPrefix(filter, "narrator:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "narrator:"))
		return typedMetadataEquals(item, value, "narrator")
	}
	if strings.HasPrefix(filter, "series:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "series:"))
		return typedMetadataEquals(item, value, "series")
	}
	if strings.HasPrefix(filter, "network:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "network:"))
		return typedMetadataEquals(item, value, "network") || strings.EqualFold(item.Network, value)
	}
	if strings.HasPrefix(filter, "show:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "show:"))
		return strings.EqualFold(item.Title, value) || strings.EqualFold(item.GrandparentTitle, value) || strings.EqualFold(item.ParentTitle, value)
	}
	if strings.HasPrefix(filter, "season:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "season:"))
		return strings.EqualFold(item.Title, value) || strings.EqualFold(item.ParentTitle, value)
	}
	if strings.HasPrefix(filter, "studio:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "studio:"))
		return typedMetadataEquals(item, value, "studio") || strings.EqualFold(item.Studio, value)
	}
	if strings.HasPrefix(filter, "decade:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "decade:"))
		decade, err := strconv.Atoi(value)
		return err == nil && item.Year >= decade && item.Year < decade+10
	}
	if strings.HasPrefix(filter, "type:") {
		value := strings.TrimSpace(strings.TrimPrefix(filter, "type:"))
		return strings.EqualFold(item.Type, value)
	}
	return false
}

func typedMetadataEquals(item MediaItem, value string, keys ...string) bool {
	for _, key := range keys {
		if strings.EqualFold(item.TypedMetadata[key], value) {
			return true
		}
	}
	return false
}

func hasGenre(item MediaItem, genre string) bool {
	for _, itemGenre := range item.Genres {
		if strings.EqualFold(itemGenre, genre) {
			return true
		}
	}
	return false
}

func isFamilyContentRating(rating string) bool {
	switch strings.ToUpper(strings.TrimSpace(rating)) {
	case "G", "PG", "TV-G", "TV-Y", "TV-Y7", "TV-PG":
		return true
	default:
		return false
	}
}

func categoryID(prefix, value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("&", "and", "+", "plus", "/", "-", " ", "-", ":", "-", "'", "", ".", "")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	if value == "" {
		value = "unknown"
	}
	return prefix + "-" + value
}
