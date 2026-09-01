package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestLibraryHomeRowArtworkShapesFollowMediaSemantics(t *testing.T) {
	tests := []struct {
		name      string
		library   Library
		row       HomeRow
		wantShape string
	}{
		{
			name:      "music rows reserve square artwork",
			library:   Library{Type: "music"},
			row:       HomeRow{ID: "recent_music", Type: "poster"},
			wantShape: "square",
		},
		{
			name:      "music landscape rows remain landscape",
			library:   Library{Type: "music"},
			row:       HomeRow{ID: "featured_music", Type: "landscape"},
			wantShape: "landscape",
		},
		{
			name:      "video rows default to poster artwork",
			library:   Library{Type: "movie"},
			row:       HomeRow{ID: "recent_movies", Type: "poster"},
			wantShape: "poster",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := resolveLibraryHomeRowArtworkShapes([]HomeRow{test.row}, test.library)
			if len(rows) != 1 || rows[0].ArtworkShape != test.wantShape {
				t.Fatalf("artwork shape = %#v, want %q", rows, test.wantShape)
			}
		})
	}
}

func TestSearchPreservesHealthyGroupsAndReportsPerGroupFailure(t *testing.T) {
	server := &Server{}
	response, err := server.executeSearchWithGroupLoader(
		context.Background(),
		User{ID: "viewer", ProfileID: "profile"},
		SearchRequest{Query: "Meridian", Limit: 8},
		func(_ context.Context, _ User, _ SearchRequest, definition searchGroupDefinition, _ int, _ searchResultCursor, _ searchSortSpec) ([]MediaItem, error) {
			switch definition.ID {
			case "movies":
				return []MediaItem{{ID: "movie-healthy", Type: "movie", Title: "Meridian", SortTitle: "Meridian"}}, nil
			case "people":
				return nil, errors.New("people index unavailable")
			default:
				return []MediaItem{}, nil
			}
		},
	)
	if err != nil {
		t.Fatalf("search returned a request-level error: %v", err)
	}
	if len(response.Groups) != 2 {
		t.Fatalf("groups=%#v, want one healthy group and one failed group", response.Groups)
	}
	if healthy := response.Groups[0]; healthy.ID != "movies" || healthy.Status != "success" || len(healthy.Items) != 1 || healthy.Items[0].ID != "movie-healthy" {
		t.Fatalf("healthy group was not preserved: %#v", healthy)
	}
	if failed := response.Groups[1]; failed.ID != "people" || failed.Status != "error" || failed.ErrorCode != "search_group_unavailable" || failed.MessageID != "search.group-unavailable" || len(failed.Items) != 0 || failed.HasMore {
		t.Fatalf("failed group did not expose the canonical partial-failure contract: %#v", failed)
	}
}

func TestSearchBoundsSlowGroupAndPreservesFollowingGroups(t *testing.T) {
	server := &Server{}
	started := time.Now()
	response, err := server.executeSearchWithGroupLoader(
		context.Background(),
		User{ID: "viewer", ProfileID: "profile"},
		SearchRequest{Query: "Meridian", Limit: 8},
		func(ctx context.Context, _ User, _ SearchRequest, definition searchGroupDefinition, _ int, _ searchResultCursor, _ searchSortSpec) ([]MediaItem, error) {
			switch definition.ID {
			case "movies":
				return []MediaItem{{ID: "movie-before-timeout", Type: "movie", Title: "Meridian", SortTitle: "Meridian"}}, nil
			case "shows":
				<-ctx.Done()
				return nil, ctx.Err()
			case "episodes":
				return []MediaItem{{ID: "episode-after-timeout", Type: "episode", Title: "Meridian", SortTitle: "Meridian"}}, nil
			default:
				return []MediaItem{}, nil
			}
		},
	)
	if err != nil {
		t.Fatalf("search returned a request-level error: %v", err)
	}
	if elapsed := time.Since(started); elapsed < mediaSearchBranchTimeout || elapsed > mediaSearchBranchTimeout+500*time.Millisecond {
		t.Fatalf("slow-group bound elapsed = %s, want approximately %s", elapsed, mediaSearchBranchTimeout)
	}
	groups := make(map[string]SearchGroup, len(response.Groups))
	for _, group := range response.Groups {
		groups[group.ID] = group
	}
	if groups["movies"].Status != "success" || groups["shows"].ErrorCode != "search_group_timeout" || groups["episodes"].Status != "success" {
		t.Fatalf("partial search groups = %#v", response.Groups)
	}
}

func TestSearchResponseSingleflightCoalescesOnlyCurrentCalls(t *testing.T) {
	server := &Server{}
	leader, owner := server.beginSearchResponseInFlight("same-search")
	if !owner {
		t.Fatal("first search call should own the singleflight")
	}

	const waiterCount = 16
	joined := make(chan *searchResponseInFlightCall, waiterCount)
	owners := make(chan bool, waiterCount)
	start := make(chan struct{})
	var waiters sync.WaitGroup
	waiters.Add(waiterCount)
	for index := 0; index < waiterCount; index++ {
		go func() {
			defer waiters.Done()
			<-start
			call, callOwner := server.beginSearchResponseInFlight("same-search")
			joined <- call
			owners <- callOwner
		}()
	}
	close(start)
	waiters.Wait()
	close(joined)
	close(owners)
	for call := range joined {
		if call != leader {
			t.Fatal("current search waiter did not join the active call")
		}
	}
	for callOwner := range owners {
		if callOwner {
			t.Fatal("current search waiter unexpectedly became an owner")
		}
	}

	want := SearchResponse{Groups: []SearchGroup{{ID: "movies", Status: "success", Items: []MediaItem{{ID: "result"}}}}}
	server.finishSearchResponseInFlight("same-search", leader, want, "profile", nil)
	select {
	case <-leader.done:
	default:
		t.Fatal("completed search did not release its current waiters")
	}
	if len(leader.response.Groups) != 1 || len(leader.response.Groups[0].Items) != 1 || leader.response.Groups[0].Items[0].ID != "result" {
		t.Fatalf("current waiters did not receive the completed response: %+v", leader.response)
	}

	fresh, owner := server.beginSearchResponseInFlight("same-search")
	if !owner || fresh == leader {
		t.Fatal("post-completion search call reused a completed response")
	}
	server.finishSearchResponseInFlight("same-search", fresh, SearchResponse{}, "profile", nil)
}

func TestSearchCanonicalSortsUseDeterministicKeysets(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	libraryID := seedSearchSortFixture(t, db)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	tests := []struct {
		name      string
		sort      string
		direction string
		want      []string
	}{
		{name: "title ascending", sort: searchSortTitle, direction: searchDirectionAsc, want: []string{"search_sort_c", "search_sort_a", "search_sort_b", "search_sort_d"}},
		{name: "title descending", sort: searchSortTitle, direction: searchDirectionDesc, want: []string{"search_sort_d", "search_sort_b", "search_sort_a", "search_sort_c"}},
		{name: "release year ascending", sort: searchSortReleaseYear, direction: searchDirectionAsc, want: []string{"search_sort_c", "search_sort_a", "search_sort_b", "search_sort_d"}},
		{name: "release year descending", sort: searchSortReleaseYear, direction: searchDirectionDesc, want: []string{"search_sort_a", "search_sort_b", "search_sort_c", "search_sort_d"}},
		{name: "date added ascending", sort: searchSortDateAdded, direction: searchDirectionAsc, want: []string{"search_sort_d", "search_sort_a", "search_sort_b", "search_sort_c"}},
		{name: "date added descending", sort: searchSortDateAdded, direction: searchDirectionDesc, want: []string{"search_sort_c", "search_sort_a", "search_sort_b", "search_sort_d"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := SearchRequest{
				Query:      "SortContract",
				Group:      "movies",
				LibraryIDs: []string{libraryID},
				Sort:       test.sort,
				Direction:  test.direction,
				Limit:      2,
			}
			got := []string{}
			for page := 0; page < 3; page++ {
				var response SearchResponse
				status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/search", request, &response)
				if status != http.StatusOK {
					t.Fatalf("search status=%d body=%s", status, body)
				}
				if response.Sort != test.sort || response.Direction != test.direction || len(response.Groups) != 1 {
					t.Fatalf("search did not echo the applied sort: %#v", response)
				}
				for _, item := range response.Groups[0].Items {
					got = append(got, item.ID)
				}
				if !response.Groups[0].HasMore {
					break
				}
				if response.Groups[0].NextCursor == "" {
					t.Fatal("search page reported more results without a cursor")
				}
				request.Cursor = response.Groups[0].NextCursor
			}
			if fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("sorted IDs=%v, want %v", got, test.want)
			}
		})
	}
}

func TestSearchVisibilityIsAppliedBeforePageLimitAndCursor(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	for _, library := range []struct{ id, settings string }{
		{"lib_search_hidden_window", `{"hideFromSearch":true}`},
		{"lib_search_visible_window", `{}`},
	} {
		if _, err := db.Exec(`
			INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
			VALUES (?, ?, 'movie', 995, ?, ?, ?)`, library.id, library.id, "/tmp/"+library.id, library.settings, now); err != nil {
			t.Fatalf("insert %s: %v", library.id, err)
		}
	}
	for index := 0; index < 5; index++ {
		libraryID := "lib_search_hidden_window"
		visibility := "Hidden"
		if index >= 3 {
			libraryID = "lib_search_visible_window"
			visibility = "Visible"
		}
		id := fmt.Sprintf("search_window_%d", index)
		title := fmt.Sprintf("Visibility Window %s %d", visibility, index)
		if _, err := db.Exec(`
			INSERT INTO media_items (
				id, library_id, type, title, sort_title, year, duration_seconds,
				genres_json, tags_json, labels_json, typed_metadata_json, added_at, random_key
			) VALUES (?, ?, 'movie', ?, ?, 2026, 3600, '[]', '[]', '[]', '{}', ?, ?)`,
			id, libraryID, title, title, now, mediaRandomKey(id)); err != nil {
			t.Fatalf("insert search window item %s: %v", id, err)
		}
		if _, err := db.Exec(`INSERT INTO media_search (media_id, title, summary, genres) VALUES (?, ?, '', '')`, id, title); err != nil {
			t.Fatalf("index search window item %s: %v", id, err)
		}
		if _, err := db.Exec(`
			INSERT INTO media_people (id, media_id, name, role, source, sort_order, provider_ids_json, created_at)
			VALUES (?, ?, ?, 'Actor', 'test', 0, '{}', ?)`,
			"search-window-person-credit-"+strconv.Itoa(index), id, fmt.Sprintf("Visibility Window Person %d", index), now); err != nil {
			t.Fatalf("insert search window person %d: %v", index, err)
		}
	}
	var accountID string
	if err := db.QueryRow(`SELECT id FROM users WHERE role IN ('owner', 'admin') ORDER BY created_at LIMIT 1`).Scan(&accountID); err != nil {
		t.Fatalf("load search viewer: %v", err)
	}
	user := User{ID: accountID, AccountID: accountID, ProfileID: accountID, ProfileIsPrimary: true, Role: "owner", Permissions: ownerPermissions()}
	request := SearchRequest{Query: "Visibility Window", Group: "movies", Limit: 1}
	seen := []string{}
	for page := 0; page < 3; page++ {
		response, err := server.executeSearch(context.Background(), user, request)
		if err != nil || len(response.Groups) != 1 || len(response.Groups[0].Items) != 1 {
			t.Fatalf("search page %d response=%#v err=%v", page, response, err)
		}
		seen = append(seen, response.Groups[0].Items[0].ID)
		if !response.Groups[0].HasMore {
			break
		}
		request.Cursor = response.Groups[0].NextCursor
	}
	if fmt.Sprint(seen) != fmt.Sprint([]string{"search_window_3", "search_window_4"}) {
		t.Fatalf("hidden libraries consumed search windows or cursor positions: %v", seen)
	}
	people, err := server.searchPeopleContext(context.Background(), user, "Visibility Window Person", nil, 10, searchResultCursor{}, searchSortSpec{Field: searchSortRelevance, Direction: searchDirectionDesc})
	if err != nil || len(people) != 2 {
		t.Fatalf("people search visibility items=%#v err=%v", people, err)
	}
	for _, person := range people {
		if strings.Contains(person.Title, " 0") || strings.Contains(person.Title, " 1") || strings.Contains(person.Title, " 2") {
			t.Fatalf("hidden library person appeared in search: %#v", person)
		}
	}
}

func TestSearchSortDefaultsAreCanonical(t *testing.T) {
	tests := []struct {
		request       SearchRequest
		wantSort      string
		wantDirection string
	}{
		{request: SearchRequest{Query: "default", Group: "movies"}, wantSort: searchSortRelevance, wantDirection: searchDirectionDesc},
		{request: SearchRequest{Query: "default", Group: "movies", Sort: searchSortTitle}, wantSort: searchSortTitle, wantDirection: searchDirectionAsc},
		{request: SearchRequest{Query: "default", Group: "movies", Sort: searchSortReleaseYear}, wantSort: searchSortReleaseYear, wantDirection: searchDirectionDesc},
		{request: SearchRequest{Query: "default", Group: "movies", Sort: searchSortDateAdded}, wantSort: searchSortDateAdded, wantDirection: searchDirectionDesc},
	}
	for _, test := range tests {
		request, spec, err := normalizeSearchRequest(test.request)
		if err != nil {
			t.Fatalf("normalize %#v: %v", test.request, err)
		}
		if request.Sort != test.wantSort || request.Direction != test.wantDirection || spec.Field != test.wantSort || spec.Direction != test.wantDirection {
			t.Fatalf("normalize %#v = request %#v spec %#v, want %s %s", test.request, request, spec, test.wantSort, test.wantDirection)
		}
	}
}

func TestSearchQueryLengthMatchesPublishedContract(t *testing.T) {
	if _, _, err := normalizeSearchRequest(SearchRequest{Query: strings.Repeat("a", 120)}); err != nil {
		t.Fatalf("120-character query rejected: %v", err)
	}
	if _, _, err := normalizeSearchRequest(SearchRequest{Query: strings.Repeat("é", 121)}); !errors.Is(err, errInvalidSearchRequest) {
		t.Fatalf("121-character Unicode query error=%v, want invalid search request", err)
	}
}

func TestSearchEntityKindsMatchPublishedRequestContract(t *testing.T) {
	for _, kind := range []string{"movie", "show", "season", "episode", "artist", "album", "track", "book", "person", "live-channel"} {
		if _, _, err := normalizeSearchRequest(SearchRequest{Query: "contract", EntityKinds: []string{kind}}); err != nil {
			t.Errorf("published entity kind %q rejected: %v", kind, err)
		}
	}
}

func TestSearchCursorRejectsSortOrDirectionReplay(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	libraryID := seedSearchSortFixture(t, db)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	request := SearchRequest{Query: "SortContract", Group: "movies", LibraryIDs: []string{libraryID}, Sort: searchSortTitle, Direction: searchDirectionAsc, Limit: 2}
	var first SearchResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/search", request, &first)
	if status != http.StatusOK || len(first.Groups) != 1 || first.Groups[0].NextCursor == "" {
		t.Fatalf("first sorted search status=%d body=%s response=%#v", status, body, first)
	}

	request.Cursor = first.Groups[0].NextCursor
	request.Direction = searchDirectionDesc
	var problem map[string]any
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/search", request, &problem)
	if status != http.StatusBadRequest || problem["code"] != "invalid_cursor" {
		t.Fatalf("cross-direction cursor status=%d body=%s", status, body)
	}
	request.Direction = searchDirectionAsc
	request.Sort = searchSortDateAdded
	problem = map[string]any{}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/search", request, &problem)
	if status != http.StatusBadRequest || problem["code"] != "invalid_cursor" {
		t.Fatalf("cross-sort cursor status=%d body=%s", status, body)
	}
}

func TestSearchCursorRejectsCrossKindReplayWithinSharedGroup(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	var libraryID string
	if err := db.QueryRow(`SELECT id FROM libraries WHERE type = 'music' ORDER BY sort_order LIMIT 1`).Scan(&libraryID); err != nil {
		t.Fatalf("load music library: %v", err)
	}
	for index := 0; index < 3; index++ {
		id := fmt.Sprintf("search_kind_track_%02d", index)
		title := fmt.Sprintf("KindScope Track %02d", index)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, tags_json, labels_json, added_at, random_key)
			VALUES (?, ?, 'track', ?, ?, '[]', '[]', '[]', ?, ?)`,
			id, libraryID, title, title, time.Now().UTC().Format(time.RFC3339Nano), mediaRandomKey(id)); err != nil {
			t.Fatalf("insert track %d: %v", index, err)
		}
		if _, err := db.Exec(`INSERT INTO media_search (media_id, title, summary, genres) VALUES (?, ?, '', '')`, id, title); err != nil {
			t.Fatalf("index track %d: %v", index, err)
		}
	}
	for index, mediaType := range []string{"artist", "album"} {
		id := fmt.Sprintf("search_kind_%s_%02d", mediaType, index)
		title := fmt.Sprintf("KindScope %s", mediaType)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, tags_json, labels_json, added_at, random_key)
			VALUES (?, ?, ?, ?, ?, '[]', '[]', '[]', ?, ?)`,
			id, libraryID, mediaType, title, title, time.Now().UTC().Format(time.RFC3339Nano), mediaRandomKey(id)); err != nil {
			t.Fatalf("insert %s: %v", mediaType, err)
		}
		if _, err := db.Exec(`INSERT INTO media_search (media_id, title, summary, genres) VALUES (?, ?, '', '')`, id, title); err != nil {
			t.Fatalf("index %s: %v", mediaType, err)
		}
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	request := SearchRequest{
		Query: "KindScope", Group: "music", EntityKinds: []string{"track"},
		LibraryIDs: []string{libraryID}, Limit: 2,
	}
	var first SearchResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/search", request, &first)
	if status != http.StatusOK || len(first.Groups) != 1 || first.Groups[0].NextCursor == "" {
		t.Fatalf("track cursor status=%d body=%s response=%#v", status, body, first)
	}
	for _, replayKind := range []string{"artist", "album"} {
		replay := request
		replay.EntityKinds = []string{replayKind}
		replay.Cursor = first.Groups[0].NextCursor
		var problem map[string]any
		status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/search", replay, &problem)
		if status != http.StatusBadRequest || problem["code"] != "invalid_cursor" {
			t.Fatalf("track cursor replayed as %s: status=%d body=%s", replayKind, status, body)
		}
	}
}

func TestSearchCursorKindScopeIsOrderIndependentAndAliasCanonical(t *testing.T) {
	spec := searchSortSpec{Field: searchSortRelevance, Direction: searchDirectionDesc}
	left := searchCursorScope("query", "music", []string{"track", "album", "track"}, []string{"library-b", "library-a"}, spec)
	right := searchCursorScope("query", "music", []string{"album", "track"}, []string{"library-a", "library-b"}, spec)
	if left != right {
		t.Fatalf("equivalent kind scopes differ:\nleft  %s\nright %s", left, right)
	}
	liveCanonical := searchCursorScope("query", "live-tv", []string{"live-channel"}, nil, spec)
	if strings.Contains(liveCanonical, "live_channel") {
		t.Fatalf("Live TV cursor scope exposed a storage-only kind: %q", liveCanonical)
	}
}

func TestSearchRejectsUnsupportedSortCombinations(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	libraryID := seedSearchSortFixture(t, db)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	tests := []SearchRequest{
		{Query: "SortContract", Group: "movies", Sort: searchSortRelevance, Direction: searchDirectionAsc},
		{Query: "SortContract", Group: "live-tv", Sort: searchSortTitle, Direction: searchDirectionAsc},
		{Query: "SortContract", Sort: searchSortTitle, Direction: searchDirectionAsc},
		{Query: "SortContract", Group: "movies", Sort: "popularity", Direction: searchDirectionDesc},
		{Query: "SortContract", Group: "movies", Sort: searchSortTitle, Direction: "sideways"},
	}
	for _, request := range tests {
		var problem map[string]any
		status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/search", request, &problem)
		if status != http.StatusBadRequest || problem["code"] != "invalid_search_request" {
			t.Fatalf("unsupported search %#v status=%d body=%s", request, status, body)
		}
	}

	var allowed SearchResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/search", SearchRequest{
		Query: "SortContract", LibraryIDs: []string{libraryID}, Sort: searchSortTitle, Direction: searchDirectionAsc,
	}, &allowed)
	if status != http.StatusOK || allowed.Sort != searchSortTitle || allowed.Direction != searchDirectionAsc {
		t.Fatalf("media-scoped sort status=%d body=%s response=%#v", status, body, allowed)
	}
}

func seedSearchSortFixture(t *testing.T, db *sql.DB) string {
	t.Helper()
	var libraryID string
	if err := db.QueryRow(`SELECT id FROM libraries WHERE type = 'movie' ORDER BY sort_order LIMIT 1`).Scan(&libraryID); err != nil {
		t.Fatalf("load search-sort library: %v", err)
	}
	items := []struct {
		id, title, addedAt string
		year               int
	}{
		{id: "search_sort_a", title: "SortContract Same", year: 2020, addedAt: "2024-01-02T12:00:00Z"},
		{id: "search_sort_b", title: "SortContract Same", year: 2020, addedAt: "2024-01-02T12:00:00Z"},
		{id: "search_sort_c", title: "SortContract Alpha", year: 1990, addedAt: "2024-01-03T12:00:00Z"},
		{id: "search_sort_d", title: "SortContract Zulu", year: 0, addedAt: "2024-01-01T12:00:00Z"},
	}
	for _, item := range items {
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, year, genres_json, tags_json, labels_json, added_at, random_key)
			VALUES (?, ?, 'movie', ?, ?, ?, '[]', '[]', '[]', ?, ?)`,
			item.id, libraryID, item.title, item.title, item.year, item.addedAt, mediaRandomKey(item.id)); err != nil {
			t.Fatalf("insert search-sort media %s: %v", item.id, err)
		}
		if _, err := db.Exec(`INSERT INTO media_search (media_id, title, summary, genres) VALUES (?, ?, '', '')`, item.id, item.title); err != nil {
			t.Fatalf("index search-sort media %s: %v", item.id, err)
		}
	}
	return libraryID
}

func TestHomeManifestIsDescriptorFirstAndBounded(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var home HomeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &home)
	if status != http.StatusOK {
		t.Fatalf("home status = %d, body: %s", status, body)
	}
	if len(body) > 64<<10 {
		t.Fatalf("home manifest is %d bytes; budget is 64 KiB", len(body))
	}
	if len(home.Rows) < 5 {
		t.Fatalf("expected a deep row manifest, got %#v", home.Rows)
	}
	embedded := 0
	for _, row := range home.Rows {
		if row.ID == "server_watching_week" {
			t.Fatal("single-member fixture exposed an activity aggregation row")
		}
		if row.Endpoint != "/api/home/rows/"+row.ID || !row.CursorCapable {
			t.Fatalf("row is not independently loadable: %#v", row)
		}
		if !row.Critical && len(row.Items) != 0 {
			t.Fatalf("deep row %q unexpectedly embedded %d items", row.ID, len(row.Items))
		}
		if len(row.Items) > homeManifestPreviewLimit {
			t.Fatalf("row %q exceeded preview limit: %d", row.ID, len(row.Items))
		}
		embedded += len(row.Items)
	}
	if embedded > 4*homeManifestPreviewLimit {
		t.Fatalf("manifest embedded %d items", embedded)
	}
}

func TestSearchMediaContinuationUsesBoundKeysetCursor(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	var libraryID string
	if err := db.QueryRow(`SELECT id FROM libraries WHERE type = 'movie' ORDER BY sort_order LIMIT 1`).Scan(&libraryID); err != nil {
		t.Fatalf("load movie library: %v", err)
	}
	for index := 0; index < 5; index++ {
		id := fmt.Sprintf("search_keyset_%02d", index)
		title := fmt.Sprintf("Keyset Result %02d", index)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, tags_json, labels_json, added_at, random_key)
			VALUES (?, ?, 'movie', ?, ?, '[]', '[]', '[]', ?, ?)`,
			id, libraryID, title, title, time.Now().UTC().Format(time.RFC3339), mediaRandomKey(id)); err != nil {
			t.Fatalf("insert search item %d: %v", index, err)
		}
		if _, err := db.Exec(`INSERT INTO media_search (media_id, title, summary, genres) VALUES (?, ?, '', '')`, id, title); err != nil {
			t.Fatalf("index search item %d: %v", index, err)
		}
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	request := SearchRequest{Query: "Keyset", Group: "movies", LibraryIDs: []string{libraryID}, Limit: 2}
	var first SearchResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/search", request, &first)
	if status != http.StatusOK || len(first.Groups) != 1 || len(first.Groups[0].Items) != 2 || !first.Groups[0].HasMore || first.Groups[0].NextCursor == "" {
		t.Fatalf("first keyset search status=%d response=%#v body=%s", status, first, body)
	}
	if containsAny(first.Groups[0].NextCursor, "search_keyset_", "Keyset Result") {
		t.Fatal("search cursor exposed its keyset values")
	}
	request.Cursor = first.Groups[0].NextCursor
	var second SearchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/search", request, &second)
	if status != http.StatusOK || len(second.Groups) != 1 || len(second.Groups[0].Items) != 2 {
		t.Fatalf("second keyset search status=%d response=%#v body=%s", status, second, body)
	}
	if second.Groups[0].Items[0].ID == first.Groups[0].Items[0].ID || second.Groups[0].Items[0].ID == first.Groups[0].Items[1].ID {
		t.Fatalf("search continuation repeated first page: first=%#v second=%#v", first.Groups[0].Items, second.Groups[0].Items)
	}
	replayed := request
	replayed.LibraryIDs = nil
	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/search", replayed, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("cross-scope search cursor status = %d, want 400", status)
	}
}

func TestHomeRowUsesOpaqueCursorInsteadOfOffset(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var manifest HomeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &manifest)
	if status != http.StatusOK {
		t.Fatalf("manifest status = %d, body: %s", status, body)
	}
	rowID := "continue"
	for _, descriptor := range manifest.Rows {
		if len(descriptor.ID) > len("recent_") && descriptor.ID[:len("recent_")] == "recent_" {
			rowID = descriptor.ID
			break
		}
	}
	var row HomeRow
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/home/rows/"+rowID+"?limit=1", nil, &row)
	if status != http.StatusOK {
		t.Fatalf("home row status = %d, body: %s", status, body)
	}
	if row.HasMore && row.NextCursor == "" {
		t.Fatal("paged row omitted nextCursor")
	}
	status, _ = doJSON(t, client, http.MethodGet, serverURL+"/api/home/rows/"+rowID+"?cursor=not-a-cursor", nil, &row)
	if status != http.StatusBadRequest {
		t.Fatalf("malformed cursor status = %d", status)
	}
}

func TestSearchPOSTGroupsAndFiltersOnServer(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var title, libraryID string
	if err := db.QueryRow(`SELECT title, library_id FROM media_items WHERE type = 'movie' LIMIT 1`).Scan(&title, &libraryID); err != nil {
		t.Fatalf("load searchable movie: %v", err)
	}
	var response SearchResponse
	request := SearchRequest{Query: title, EntityKinds: []string{"movie"}, LibraryIDs: []string{libraryID}, Limit: 4}
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/search", request, &response)
	if status != http.StatusOK {
		t.Fatalf("search status = %d, body: %s", status, body)
	}
	if response.Query != request.Query || len(response.Groups) != 1 || response.Groups[0].ID != "movies" {
		t.Fatalf("unexpected grouped response: %#v", response)
	}
	if response.Sort != searchSortRelevance || response.Direction != searchDirectionDesc {
		t.Fatalf("default search sort = %s %s, want relevance desc", response.Sort, response.Direction)
	}
	for _, item := range response.Groups[0].Items {
		if item.Type != "movie" || item.LibraryID != libraryID {
			t.Fatalf("server-side filters leaked item: %#v", item)
		}
	}
	status, _ = doJSON(t, client, http.MethodGet, serverURL+"/api/search?q="+url.QueryEscape(title), nil, nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("legacy GET search status = %d", status)
	}
}

func TestOpaqueCursorRejectsCrossGroupReplay(t *testing.T) {
	server := &Server{cfg: config.Config{AppDataDir: t.TempDir()}}
	now := time.Now().UTC()
	cursor, err := server.encodeMaterializedPageCursor("search:q:movies", "usr_cursor", 8, now)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	if _, err := server.decodeMaterializedPageCursor(cursor, "search:q:shows", "usr_cursor", now); err == nil {
		t.Fatal("cursor was accepted for another group")
	}
	if _, err := server.decodeMaterializedPageCursor(cursor, "search:q:movies", "usr_other", now); err == nil {
		t.Fatal("cursor was accepted for another principal")
	}
	if _, err := server.encodeMaterializedPageCursor("home:continue", "usr_cursor", -1, now); err == nil {
		t.Fatal("negative cursor offset was accepted")
	}
}
