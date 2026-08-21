package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/browsecontract"
	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestCanonicalBrowseContractProjectionUsesSharedVocabularyAndLimits(t *testing.T) {
	contract := canonicalProductContract()
	fields := browsecontract.Fields()
	if len(contract.BrowseFields) != len(fields) {
		t.Fatalf("published browse fields = %d, canonical fields = %d", len(contract.BrowseFields), len(fields))
	}
	for index, field := range fields {
		published := contract.BrowseFields[index]
		if published.ID != field.ID || published.ValueType != string(field.ValueType) {
			t.Fatalf("published field %d drifted: %#v vs %#v", index, published, field)
		}
		if fmt.Sprint(published.Operators) != fmt.Sprint(field.Operators) || fmt.Sprint(published.AllowedValues) != fmt.Sprint(field.AllowedValues) {
			t.Fatalf("published field vocabulary %q drifted: %#v vs %#v", field.ID, published, field)
		}
	}
	limits := contract.QueryLimits
	if limits.MaximumDepth != browsecontract.MaximumDepth || limits.MaximumClauses != browsecontract.MaximumClauses || limits.MaximumBytes != browsecontract.MaximumBytes {
		t.Fatalf("published query limits drifted: %#v", limits)
	}
}

func TestProductContractAndLibraryCapabilitiesAreCanonical(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var contract CanonicalProductContract
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/product-contract", nil, &contract)
	if status != http.StatusOK {
		t.Fatalf("product contract status = %d, body: %s", status, body)
	}
	if contract.APIVersion != productContractRevision || len(contract.LibraryKinds) != 6 || len(contract.BrowseFields) == 0 || len(contract.BrowseSorts) == 0 || len(contract.MediaActions) == 0 {
		t.Fatalf("incomplete product contract: %#v", contract)
	}
	if contract.ActionRevision != mediaActionRevision || len(contract.EntitySemantics) != len(contract.EntityKinds) || len(contract.ArtworkRoles) == 0 {
		t.Fatalf("semantic and action contracts are incomplete: %#v", contract)
	}
	if contract.Search.Revision != productContractRevision || contract.Search.Endpoint != "/api/search" || len(contract.Search.Groups) != 7 || len(contract.Search.Sorts) != 4 || len(contract.Search.Filters) != 3 {
		t.Fatalf("search contract is incomplete: %#v", contract.Search)
	}
	wantSearchGroups := []string{"movies", "shows", "episodes", "music", "audiobooks", "people", "live-tv"}
	for index, want := range wantSearchGroups {
		group := contract.Search.Groups[index]
		if contract.Search.GroupOrder[index] != want || group.ID != want || len(group.ResultKinds) == 0 || len(group.Sorts) == 0 {
			t.Fatalf("search group %d does not match runtime ordering: order=%#v group=%#v", index, contract.Search.GroupOrder, group)
		}
	}
	if contract.Search.Groups[6].SupportsLibraryScope || len(contract.Search.Groups[6].Sorts) != 1 || contract.Search.Groups[6].Sorts[0] != searchSortRelevance {
		t.Fatalf("Live TV search semantics are not explicit: %#v", contract.Search.Groups[6])
	}
	if contract.Search.Limits.QuickInitialGroupLimit != 3 || contract.Search.Limits.QuickMaximumGroups != 6 || contract.Search.Limits.QuickMaximumItemsPerGroup != 6 || contract.Search.Limits.FullDefaultGroupLimit != 50 || contract.Search.Limits.MaximumGroupLimit != 50 {
		t.Fatalf("search surface limits drifted: %#v", contract.Search.Limits)
	}
	if contract.Search.Cursor.Mode != "independent-group" || !contract.Search.Cursor.Opaque || !contract.Search.Cursor.RequiresSingleGroup || !contract.Search.Cursor.PrincipalBound || contract.Search.Cursor.TTLSeconds != int(cursorDefaultTTL.Seconds()) {
		t.Fatalf("search cursor semantics are incomplete: %#v", contract.Search.Cursor)
	}
	if len(contract.Search.ResultSemantics.KindMappings) == 0 || contract.Search.ResultSemantics.ArtworkRoleSource != "entitySemantics.primaryArtworkRole" {
		t.Fatalf("search result presentation semantics are incomplete: %#v", contract.Search.ResultSemantics)
	}
	entityKinds := stringSet(contract.EntityKinds)
	artworkRoles := map[string]bool{}
	for _, role := range contract.ArtworkRoles {
		artworkRoles[role.ID] = true
	}
	for _, semantic := range contract.EntitySemantics {
		if !entityKinds[semantic.ID] || !artworkRoles[semantic.PrimaryArtworkRole] {
			t.Fatalf("entity semantic has an unknown id or artwork role: %#v", semantic)
		}
		for _, related := range append(append([]string{}, semantic.ParentKinds...), semantic.ChildKinds...) {
			if !entityKinds[related] {
				t.Fatalf("entity semantic %q references unknown kind %q", semantic.ID, related)
			}
		}
	}
	for _, action := range contract.MediaActions {
		if action.Command.Kind == "api" && (action.Command.Method == "" || action.Command.PathTemplate == "" || action.Command.Execution == "selection") {
			t.Fatalf("API action %q has no executable command: %#v", action.ID, action)
		}
		if action.Command.Kind == "client-flow" && (action.Command.FlowID == "" || action.Command.Execution != "selection") {
			t.Fatalf("client flow %q has no stable flow id: %#v", action.ID, action)
		}
	}
	if contract.LibraryKinds[0].ID != "movies" || contract.LibraryKinds[1].ID != "tv" || contract.LibraryKinds[3].ID != "music" {
		t.Fatalf("canonical library ordering changed: %#v", contract.LibraryKinds)
	}
	for _, kind := range contract.LibraryKinds {
		for _, pivot := range kind.Pivots {
			for _, entityKind := range pivot.EntityKinds {
				if !entityKinds[entityKind] {
					t.Fatalf("library pivot %s/%s references unknown entity kind %q", kind.ID, pivot.ID, entityKind)
				}
			}
		}
		for _, pivot := range kind.Pivots {
			if pivot.ID == "sources" {
				t.Fatalf("administrative sources leaked into %s browse pivots", kind.ID)
			}
		}
	}

	var libraryID string
	if err := db.QueryRow(`SELECT id FROM libraries WHERE type = 'movie' ORDER BY sort_order LIMIT 1`).Scan(&libraryID); err != nil {
		t.Fatalf("load movie library: %v", err)
	}
	var capabilities LibraryBrowseCapabilities
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/"+libraryID+"/browse-capabilities", nil, &capabilities)
	if status != http.StatusOK {
		t.Fatalf("browse capabilities status = %d, body: %s", status, body)
	}
	if capabilities.Library.ID != libraryID || capabilities.Library.Kind != "movies" {
		t.Fatalf("unexpected library capability scope: %#v", capabilities.Library)
	}
	wantPivots := []string{"discover", "movies", "collections", "categories"}
	if len(capabilities.Pivots) != len(wantPivots) {
		t.Fatalf("movie pivot count = %d, want %d", len(capabilities.Pivots), len(wantPivots))
	}
	for index, want := range wantPivots {
		if capabilities.Pivots[index].ID != want {
			t.Fatalf("movie pivot %d = %q, want %q", index, capabilities.Pivots[index].ID, want)
		}
	}

	var movieCapabilities LibraryBrowseCapabilities
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/"+libraryID+"/browse-capabilities?pivot=movies", nil, &movieCapabilities)
	if status != http.StatusOK || movieCapabilities.ResolvedPivot == nil || movieCapabilities.ResolvedPivot.ID != "movies" {
		t.Fatalf("resolved movie capabilities status=%d body=%s payload=%#v", status, body, movieCapabilities)
	}
	for _, field := range movieCapabilities.Fields {
		if field.ID == "entityKind" && (len(field.AllowedValues) != 1 || field.AllowedValues[0] != "movie") {
			t.Fatalf("movie entity kinds were not narrowed: %#v", field)
		}
		if field.ID == "author" || field.ID == "series" {
			t.Fatalf("non-movie field leaked into resolved capabilities: %#v", field)
		}
		if field.ID == "availability" && len(field.AllowedValues) != 3 {
			t.Fatalf("availability enum values are not published: %#v", field)
		}
		if field.ID == "genre" && (field.FacetSource == nil || field.FacetSource.FilterPrefix != "genre:" || field.FacetSource.ValueField != "name") {
			t.Fatalf("genre facet source is not published: %#v", field)
		}
	}
	var collectionCapabilities LibraryBrowseCapabilities
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/"+libraryID+"/browse-capabilities?pivot=collections", nil, &collectionCapabilities)
	if status != http.StatusOK || collectionCapabilities.ResolvedPivot == nil || collectionCapabilities.ResolvedPivot.BrowseSupported || len(collectionCapabilities.Fields) != 0 || len(collectionCapabilities.Sorts) != 0 || containsString(collectionCapabilities.Actions, "browse") {
		t.Fatalf("separate-resource pivot advertised media browse capabilities status=%d body=%s payload=%#v", status, body, collectionCapabilities)
	}
}

func TestPublishedPresentationFieldsAreAcceptedByBrowseValidation(t *testing.T) {
	published := canonicalPresentationFields()
	validated, issues := validateBrowsePresentation(published)
	if len(issues) != 0 {
		t.Fatalf("published presentation fields were rejected: %#v", issues)
	}
	if len(validated) != len(published) {
		t.Fatalf("validated presentation fields = %#v, want %#v", validated, published)
	}
	for index := range published {
		if validated[index] != published[index] {
			t.Fatalf("validated presentation field %d = %q, want %q", index, validated[index], published[index])
		}
	}
}

func TestBrowseQueryCapabilitiesArePivotScoped(t *testing.T) {
	movieKind, ok := productLibraryKindByID("movies")
	if !ok {
		t.Fatal("movie library kind is unavailable")
	}
	moviePivot, ok := browsePivotByID(movieKind, "movies")
	if !ok {
		t.Fatal("movie pivot is unavailable")
	}
	for _, expression := range []BrowseExpression{
		{Field: "author", Operator: "contains", Value: json.RawMessage(`"Le Guin"`)},
		{Field: "entityKind", Operator: "equals", Value: json.RawMessage(`"show"`)},
	} {
		if issues := validateBrowseQueryCapabilities(expression, moviePivot, "query"); len(issues) != 1 {
			t.Fatalf("pivot validation accepted %#v: %#v", expression, issues)
		}
	}
	valid := BrowseExpression{Field: "entityKind", Operator: "equals", Value: json.RawMessage(`"movie"`)}
	if issues := validateBrowseQueryCapabilities(valid, moviePivot, "query"); len(issues) != 0 {
		t.Fatalf("pivot validation rejected movie entity kind: %#v", issues)
	}
}

func TestCanonicalLibraryBrowseUsesBoundKeysetCursorAndLeanCards(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_canonical_browse', 'Canonical Browse', 'movie', 990, '/tmp/canonical-browse', '{}', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	for index := 0; index < 7; index++ {
		id := fmt.Sprintf("canonical_movie_%02d", index)
		title := fmt.Sprintf("Canonical Movie %02d", index)
		if _, err := db.Exec(`
			INSERT INTO media_items (
				id, library_id, type, title, sort_title, year, duration_seconds,
				summary, source_url, genres_json, tags_json, labels_json, added_at, random_key
			) VALUES (?, 'lib_canonical_browse', 'movie', ?, ?, ?, 5400, ?, ?, '["Science Fiction"]', '[]', '[]', ?, ?)`,
			id, title, title, 2000+index, "This summary must not be serialized in a media card.", "/private/library/"+id+".mkv", fmt.Sprintf("2026-07-%02dT00:00:00Z", index+1), mediaRandomKey(id)); err != nil {
			t.Fatalf("insert media %d: %v", index, err)
		}
		if err := (&Server{db: db}).replaceMediaCategoryFacets(id); err != nil {
			t.Fatalf("index facets %d: %v", index, err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, tags_json, labels_json, added_at, random_key)
		VALUES ('canonical_movie_hash', 'lib_canonical_browse', 'movie', '# Archive', '# Archive', '[]', '[]', '[]', ?, ?)`, now, mediaRandomKey("canonical_movie_hash")); err != nil {
		t.Fatalf("insert nonletter seek fixture: %v", err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	queryValue, _ := json.Marshal("Canonical")
	request := BrowseLibraryRequest{
		Pivot:        "movies",
		Query:        &BrowseExpression{Field: "title", Operator: "starts-with", Value: queryValue},
		Sort:         []BrowseSort{{Field: "dateAdded", Direction: "desc"}, {Field: "title", Direction: "asc"}},
		Presentation: BrowsePresentation{Fields: []string{"year", "availability"}},
		Limit:        2,
	}
	var first BrowseLibraryResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_canonical_browse/browse", request, &first)
	if status != http.StatusOK {
		t.Fatalf("first browse status = %d, body: %s", status, body)
	}
	if len(first.Items) != 2 || !first.PageInfo.HasMore || first.PageInfo.NextCursor == nil || *first.PageInfo.NextCursor == "" {
		t.Fatalf("unexpected first browse page: %#v", first)
	}
	if first.Items[0].ID != "canonical_movie_06" || first.Items[1].ID != "canonical_movie_05" {
		t.Fatalf("unexpected first browse ordering: %#v", first.Items)
	}
	if first.Items[0].Fields["year"] == nil || first.Items[0].Availability.Status == "" {
		t.Fatalf("requested card projection fields are missing: %#v", first.Items[0])
	}
	if string(body) == "" || containsPrivateBrowseDetail(body) {
		t.Fatalf("browse card leaked detail-only or source data: %s", body)
	}

	request.Cursor = *first.PageInfo.NextCursor
	var second BrowseLibraryResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_canonical_browse/browse", request, &second)
	if status != http.StatusOK {
		t.Fatalf("second browse status = %d, body: %s", status, body)
	}
	if len(second.Items) != 2 || second.Items[0].ID != "canonical_movie_04" || second.Items[1].ID != "canonical_movie_03" {
		t.Fatalf("keyset page repeated or skipped items: %#v", second.Items)
	}

	replayed := request
	replayed.Sort = []BrowseSort{{Field: "title", Direction: "asc"}}
	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_canonical_browse/browse", replayed, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("cursor replay across sort status = %d, want 400", status)
	}

	tampered := request
	token := []byte(tampered.Cursor)
	// Mutate a significant Base64 character. The final unpadded character can
	// contain unused bits and is therefore not a reliable tamper vector.
	if token[0] == 'A' {
		token[0] = 'B'
	} else {
		token[0] = 'A'
	}
	tampered.Cursor = string(token)
	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_canonical_browse/browse", tampered, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("tampered cursor status = %d, want 400", status)
	}

	seek := BrowseLibraryRequest{Pivot: "movies", Sort: []BrowseSort{{Field: "title", Direction: "asc"}}, Seek: &BrowseSeek{Prefix: "#"}, Limit: 1}
	seen := []string{}
	for {
		var page BrowseLibraryResponse
		status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_canonical_browse/browse", seek, &page)
		if status != http.StatusOK || len(page.Items) != 1 {
			t.Fatalf("hash seek page status=%d body=%s response=%#v", status, body, page)
		}
		seen = append(seen, page.Items[0].ID)
		if !page.PageInfo.HasMore || page.PageInfo.NextCursor == nil {
			break
		}
		seek.Cursor = *page.PageInfo.NextCursor
	}
	if seen[0] != "canonical_movie_hash" || !containsString(seen, "canonical_movie_00") {
		t.Fatalf("hash seek filtered later lettered titles: %#v", seen)
	}
	letterSeek := BrowseLibraryRequest{Pivot: "movies", Sort: []BrowseSort{{Field: "title", Direction: "asc"}}, Seek: &BrowseSeek{Prefix: "C"}, Limit: 1}
	var letterFirst BrowseLibraryResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_canonical_browse/browse", letterSeek, &letterFirst)
	if status != http.StatusOK || letterFirst.PageInfo.NextCursor == nil {
		t.Fatalf("letter seek first page status=%d body=%s response=%#v", status, body, letterFirst)
	}
	letterSeek.Cursor = *letterFirst.PageInfo.NextCursor
	var letterSecond BrowseLibraryResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_canonical_browse/browse", letterSeek, &letterSecond)
	if status != http.StatusOK || len(letterSecond.Items) != 1 || letterSecond.Items[0].ID == letterFirst.Items[0].ID {
		t.Fatalf("letter seek continuation status=%d body=%s first=%#v second=%#v", status, body, letterFirst, letterSecond)
	}
}

func TestReleaseDateBrowseUsesFullNormalizedDateAndStableKeyset(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_release_date_window', 'Release Date Window', 'movie', 991, '/tmp/release-date-window', '{}', ?)`, now); err != nil {
		t.Fatalf("insert release-date library: %v", err)
	}
	fixtures := []struct{ id, releaseDate string }{
		{"release_window_a", "2025-02-01"},
		{"release_window_b", "2025-01-15"},
		{"release_window_c", "2025-01-15T22:30:00Z"},
		{"release_window_d", "2025-01-01"},
	}
	for _, fixture := range fixtures {
		typed, _ := json.Marshal(map[string]string{"release_date": fixture.releaseDate})
		title := "Release Date Window " + fixture.id
		if _, err := db.Exec(`
			INSERT INTO media_items (
				id, library_id, type, title, sort_title, year, duration_seconds,
				genres_json, tags_json, labels_json, typed_metadata_json, added_at, random_key
			) VALUES (?, 'lib_release_date_window', 'movie', ?, ?, 2025, 5400, '[]', '[]', '[]', ?, ?, ?)`,
			fixture.id, title, title, string(typed), now, mediaRandomKey(fixture.id)); err != nil {
			t.Fatalf("insert %s: %v", fixture.id, err)
		}
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	request := BrowseLibraryRequest{Pivot: "movies", Sort: []BrowseSort{{Field: "releaseDate", Direction: "asc"}}, Limit: 2}
	var first BrowseLibraryResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_release_date_window/browse", request, &first)
	if status != http.StatusOK || mediaCardIDs(first.Items)[0] != "release_window_d" || mediaCardIDs(first.Items)[1] != "release_window_b" || first.PageInfo.NextCursor == nil {
		t.Fatalf("first release-date page status=%d body=%s response=%#v", status, body, first)
	}
	request.Cursor = *first.PageInfo.NextCursor
	var second BrowseLibraryResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_release_date_window/browse", request, &second)
	if status != http.StatusOK || fmt.Sprint(mediaCardIDs(second.Items)) != fmt.Sprint([]string{"release_window_c", "release_window_a"}) {
		t.Fatalf("second release-date page status=%d body=%s response=%#v", status, body, second)
	}

	filterValue, _ := json.Marshal("2025-01-15")
	request.Cursor = ""
	request.Limit = 10
	request.Query = &BrowseExpression{Field: "releaseDate", Operator: "at-least", Value: filterValue}
	var filtered BrowseLibraryResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_release_date_window/browse", request, &filtered)
	if status != http.StatusOK || fmt.Sprint(mediaCardIDs(filtered.Items)) != fmt.Sprint([]string{"release_window_b", "release_window_c", "release_window_a"}) {
		t.Fatalf("release-date filter status=%d body=%s response=%#v", status, body, filtered)
	}
}

func mediaCardIDs(items []MediaCard) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestCanonicalLibraryBrowseRejectsUnknownAndInvalidExpressions(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var libraryID string
	if err := db.QueryRow(`SELECT id FROM libraries WHERE type = 'movie' ORDER BY sort_order LIMIT 1`).Scan(&libraryID); err != nil {
		t.Fatalf("load movie library: %v", err)
	}
	value, _ := json.Marshal("Drama")
	request := BrowseLibraryRequest{
		Pivot: "movies",
		Query: &BrowseExpression{Field: "genre", Operator: "less-than", Value: value},
		Sort:  []BrowseSort{{Field: "title", Direction: "asc"}},
	}
	var problem struct {
		Code   string                  `json:"code"`
		Errors []BrowseValidationIssue `json:"errors"`
	}
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/"+libraryID+"/browse", request, &problem)
	if status != http.StatusUnprocessableEntity || problem.Code != "validation_failed" || len(problem.Errors) == 0 {
		t.Fatalf("invalid operator status=%d problem=%#v body=%s", status, problem, body)
	}

	status, _ = doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/"+libraryID+"/items", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("superseded library items route status = %d, want 404", status)
	}
	status, _ = doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/capabilities", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("superseded library capabilities route status = %d, want 404", status)
	}
}

func TestPresenceBrowsePredicatesRequireCanonicalNullValue(t *testing.T) {
	for _, test := range []struct {
		field    string
		operator string
	}{
		{field: "releaseDate", operator: "is-present"},
		{field: "lastPlayedAt", operator: "is-missing"},
		{field: "personalRating", operator: "is-present"},
	} {
		_, _, issues := compileBrowsePredicate(BrowseExpression{Field: test.field, Operator: test.operator, Value: json.RawMessage(`"ignored"`)}, "query")
		if len(issues) != 1 || issues[0].Code != "invalid_value" || issues[0].Field != "query.value" {
			t.Fatalf("%s/%s accepted non-null presence value: %#v", test.field, test.operator, issues)
		}
		_, _, issues = compileBrowsePredicate(BrowseExpression{Field: test.field, Operator: test.operator, Value: json.RawMessage(`null`)}, "query")
		if len(issues) != 0 {
			t.Fatalf("%s/%s rejected canonical null: %#v", test.field, test.operator, issues)
		}
	}
}

func TestCanonicalAudiobookFacetFieldsCompileAgainstIndexedKeys(t *testing.T) {
	for _, test := range []struct {
		field string
		want  string
	}{
		{field: "author", want: "media_category_facets"},
		{field: "series", want: "media_category_facets"},
	} {
		t.Run(test.field, func(t *testing.T) {
			value, _ := json.Marshal("Ursula K. Le Guin")
			query, args, issues := compileBrowsePredicate(BrowseExpression{
				Field: test.field, Operator: "equals", Value: value,
			}, "query")
			if len(issues) != 0 {
				t.Fatalf("compile %s issues = %#v", test.field, issues)
			}
			if !containsAny(query, test.want) || len(args) != 2 || args[0] != test.field || args[1] != "Ursula K. Le Guin" {
				t.Fatalf("compile %s query=%q args=%#v", test.field, query, args)
			}
		})
	}
}

func containsPrivateBrowseDetail(body string) bool {
	return containsAny(body, "This summary must not be serialized", "/private/library/", "sourceUrl", "mediaFiles", "streams")
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && len(value) >= len(needle) {
			for index := 0; index+len(needle) <= len(value); index++ {
				if value[index:index+len(needle)] == needle {
					return true
				}
			}
		}
	}
	return false
}
