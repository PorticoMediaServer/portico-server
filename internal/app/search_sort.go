package app

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/PorticoMediaServer/portico-server/internal/catalogkind"
)

const (
	searchSortRelevance   = "relevance"
	searchSortTitle       = "title"
	searchSortReleaseYear = "releaseYear"
	searchSortDateAdded   = "dateAdded"
	searchDirectionAsc    = "asc"
	searchDirectionDesc   = "desc"
)

var errInvalidSearchRequest = errors.New("invalid search request")

type searchRequestValidationError struct {
	detail string
}

func (e *searchRequestValidationError) Error() string {
	return e.detail
}

func (e *searchRequestValidationError) Unwrap() error {
	return errInvalidSearchRequest
}

type searchSortSpec struct {
	Field     string
	Direction string
}

func invalidSearchRequest(detail string) error {
	return &searchRequestValidationError{detail: detail}
}

func searchRequestErrorDetail(err error) string {
	var validation *searchRequestValidationError
	if errors.As(err, &validation) && strings.TrimSpace(validation.detail) != "" {
		return validation.detail
	}
	return "Search request is not supported."
}

func normalizeSearchRequest(request SearchRequest) (SearchRequest, searchSortSpec, error) {
	request.Query = strings.TrimSpace(request.Query)
	request.Group = strings.ToLower(strings.TrimSpace(request.Group))
	request.Cursor = strings.TrimSpace(request.Cursor)
	request.Sort = canonicalSearchSort(request.Sort)
	request.Direction = strings.ToLower(strings.TrimSpace(request.Direction))
	request.EntityKinds = normalizedUniqueStrings(request.EntityKinds, true)
	request.LibraryIDs = normalizedUniqueStrings(request.LibraryIDs, false)
	if utf8.RuneCountInString(request.Query) > 120 {
		return SearchRequest{}, searchSortSpec{}, invalidSearchRequest("Query must not exceed 120 characters.")
	}

	if request.Sort == "" {
		return SearchRequest{}, searchSortSpec{}, invalidSearchRequest("Sort must be relevance, title, releaseYear, or dateAdded.")
	}
	if request.Direction == "" {
		switch request.Sort {
		case searchSortTitle:
			request.Direction = searchDirectionAsc
		default:
			request.Direction = searchDirectionDesc
		}
	}
	if request.Direction != searchDirectionAsc && request.Direction != searchDirectionDesc {
		return SearchRequest{}, searchSortSpec{}, invalidSearchRequest("Direction must be asc or desc.")
	}
	if request.Sort == searchSortRelevance && request.Direction != searchDirectionDesc {
		return SearchRequest{}, searchSortSpec{}, invalidSearchRequest("Relevance search only supports desc, which returns the best matches first.")
	}
	if request.Group != "" && !knownSearchGroup(request.Group) {
		return SearchRequest{}, searchSortSpec{}, invalidSearchRequest("Group is not a supported search result group.")
	}
	for _, kind := range request.EntityKinds {
		if !knownSearchEntityKind(kind) {
			return SearchRequest{}, searchSortSpec{}, invalidSearchRequest(fmt.Sprintf("Entity kind %q is not supported by search.", kind))
		}
	}
	if request.Group != "" && len(request.EntityKinds) > 0 && !searchEntityKindsIncludeGroup(request.EntityKinds, request.Group) {
		return SearchRequest{}, searchSortSpec{}, invalidSearchRequest("Entity kinds do not include the requested result group.")
	}
	if request.Cursor != "" && request.Group == "" {
		return SearchRequest{}, searchSortSpec{}, invalidSearchRequest("A continuation cursor requires one result group.")
	}
	if request.Query == "" && request.Cursor != "" {
		return SearchRequest{}, searchSortSpec{}, invalidSearchRequest("A continuation cursor requires a non-empty query.")
	}
	if request.Sort != searchSortRelevance && searchRequestIncludesLiveTV(request) {
		return SearchRequest{}, searchSortSpec{}, invalidSearchRequest("Live TV search results only support relevance desc. Scope this request to media libraries, entity kinds, or a non-Live-TV group to use another sort.")
	}
	if request.Group == "people" && request.Sort != searchSortRelevance && request.Sort != searchSortTitle {
		return SearchRequest{}, searchSortSpec{}, invalidSearchRequest("People search supports relevance or title sorting.")
	}
	return request, searchSortSpec{Field: request.Sort, Direction: request.Direction}, nil
}

func canonicalSearchSort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", searchSortRelevance:
		return searchSortRelevance
	case searchSortTitle:
		return searchSortTitle
	case "releaseyear":
		return searchSortReleaseYear
	case "dateadded":
		return searchSortDateAdded
	default:
		return ""
	}
}

func normalizedUniqueStrings(values []string, lower bool) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if lower {
			value = strings.ToLower(value)
		}
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func knownSearchGroup(group string) bool {
	for _, definition := range searchGroupDefinitions {
		if definition.ID == group {
			return true
		}
	}
	return false
}

func knownSearchEntityKind(kind string) bool {
	for _, definition := range searchGroupDefinitions {
		if definition.EntityKind == kind {
			return true
		}
		for _, storageType := range definition.Types {
			if string(catalogkind.Public(storageType)) == kind {
				return true
			}
		}
	}
	return false
}

func searchEntityKindsIncludeGroup(kinds []string, group string) bool {
	for _, definition := range searchGroupDefinitions {
		if definition.ID != group {
			continue
		}
		for _, kind := range kinds {
			if searchKindsSelectDefinition(map[string]bool{kind: true}, definition) {
				return true
			}
		}
		return false
	}
	return false
}

func searchKindsSelectDefinition(kinds map[string]bool, definition searchGroupDefinition) bool {
	if kinds[definition.EntityKind] {
		return true
	}
	for _, storageType := range definition.Types {
		if kinds[string(catalogkind.Public(storageType))] {
			return true
		}
	}
	return false
}

func searchRequestIncludesLiveTV(request SearchRequest) bool {
	if request.Group != "" {
		return request.Group == "live-tv"
	}
	for _, kind := range request.EntityKinds {
		if kind == "live-tv" || kind == "live-channel" {
			return true
		}
	}
	if len(request.EntityKinds) > 0 || len(request.LibraryIDs) > 0 {
		return false
	}
	return true
}

func validateSearchResultCursor(cursor searchResultCursor, definition searchGroupDefinition, spec searchSortSpec) error {
	if cursor.Mode == "" {
		return nil
	}
	if cursor.Sort != spec.Field || cursor.Direction != spec.Direction {
		return fmt.Errorf("%w: cursor sort does not match this request", errInvalidCursor)
	}
	if definition.Live {
		if cursor.Mode != "live-keyset" || cursor.ID == "" || spec.Field != searchSortRelevance || spec.Direction != searchDirectionDesc {
			return fmt.Errorf("%w: cursor mode does not match this result group", errInvalidCursor)
		}
		return nil
	}
	if cursor.Mode != "media-keyset" || cursor.ID == "" {
		return fmt.Errorf("%w: cursor mode does not match this result group", errInvalidCursor)
	}
	return nil
}

func searchMediaOrderAndKeyset(spec searchSortSpec, cursor searchResultCursor) (string, []any, string) {
	if cursor.Mode == "" {
		return "", nil, searchMediaOrderSQL(spec)
	}
	switch spec.Field {
	case searchSortTitle:
		operator := searchDirectionOperator(spec.Direction)
		return fmt.Sprintf("(m.sort_title COLLATE NOCASE %s ? OR (m.sort_title COLLATE NOCASE = ? AND m.id %s ?))", operator, operator),
			[]any{cursor.SortTitle, cursor.SortTitle, cursor.ID}, searchMediaOrderSQL(spec)
	case searchSortReleaseYear:
		operator := searchDirectionOperator(spec.Direction)
		missing := "CASE WHEN COALESCE(m.year, 0) > 0 THEN 0 ELSE 1 END"
		return fmt.Sprintf("(%s > ? OR (%s = ? AND (COALESCE(m.year, 0) %s ? OR (COALESCE(m.year, 0) = ? AND (m.sort_title COLLATE NOCASE > ? OR (m.sort_title COLLATE NOCASE = ? AND m.id > ?))))))", missing, missing, operator),
			[]any{cursor.YearMissing, cursor.YearMissing, cursor.Year, cursor.Year, cursor.SortTitle, cursor.SortTitle, cursor.ID}, searchMediaOrderSQL(spec)
	case searchSortDateAdded:
		operator := searchDirectionOperator(spec.Direction)
		return fmt.Sprintf("(m.added_at %s ? OR (m.added_at = ? AND (m.sort_title COLLATE NOCASE > ? OR (m.sort_title COLLATE NOCASE = ? AND m.id > ?))))", operator),
			[]any{cursor.AddedAt, cursor.AddedAt, cursor.SortTitle, cursor.SortTitle, cursor.ID}, searchMediaOrderSQL(spec)
	default:
		return "(search_rank.relevance > ? OR (search_rank.relevance = ? AND (m.sort_title COLLATE NOCASE > ? OR (m.sort_title COLLATE NOCASE = ? AND m.id > ?))))",
			[]any{cursor.Rank, cursor.Rank, cursor.SortTitle, cursor.SortTitle, cursor.ID}, searchMediaOrderSQL(spec)
	}
}

func searchMediaOrderSQL(spec searchSortSpec) string {
	switch spec.Field {
	case searchSortTitle:
		direction := strings.ToUpper(spec.Direction)
		return "m.sort_title COLLATE NOCASE " + direction + ", m.id " + direction
	case searchSortReleaseYear:
		return "CASE WHEN COALESCE(m.year, 0) > 0 THEN 0 ELSE 1 END ASC, COALESCE(m.year, 0) " + strings.ToUpper(spec.Direction) + ", m.sort_title COLLATE NOCASE ASC, m.id ASC"
	case searchSortDateAdded:
		return "m.added_at " + strings.ToUpper(spec.Direction) + ", m.sort_title COLLATE NOCASE ASC, m.id ASC"
	default:
		return "search_rank.relevance ASC, m.sort_title COLLATE NOCASE ASC, m.id ASC"
	}
}

func searchDirectionOperator(direction string) string {
	if direction == searchDirectionDesc {
		return "<"
	}
	return ">"
}

func nextSearchMediaCursor(spec searchSortSpec, item MediaItem, rank float64) searchResultCursor {
	return searchResultCursor{
		Mode:        "media-keyset",
		Sort:        spec.Field,
		Direction:   spec.Direction,
		Rank:        rank,
		SortTitle:   item.SortTitle,
		Year:        item.Year,
		YearMissing: boolInt(item.Year <= 0),
		AddedAt:     item.AddedAt,
		ID:          item.ID,
	}
}
