package app

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestLibraryItemsPaginationContract(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_pagination_guard', 'Pagination Guard', 'movie', 950, '/tmp/pagination-guard', '{}', ?)`, now); err != nil {
		t.Fatalf("insert pagination library: %v", err)
	}
	for i := 0; i < 16; i++ {
		id := fmt.Sprintf("pagination_movie_%02d", i)
		title := fmt.Sprintf("Pagination Movie %02d", i)
		addedAt := fmt.Sprintf("2026-06-07T00:%02d:00Z", i)
		artist := "Zulu Artist"
		if i < 8 {
			artist = "Alpha Artist"
		}
		typedMetadataJSON, err := json.Marshal(map[string]string{"artist": artist})
		if err != nil {
			t.Fatalf("marshal pagination typed metadata %d: %v", i, err)
		}
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, summary, tagline, source_url, genres_json, added_at, random_key, typed_metadata_json)
			VALUES (?, 'lib_pagination_guard', 'movie', ?, ?, ?, ?, ?, '[]', ?, ?, ?)`,
			id, title, title, strings.Repeat("Long detail synopsis. ", 20), "Detail-only tagline", "https://media.example.test/"+id+".mp4", addedAt, mediaRandomKey(id), string(typedMetadataJSON),
		); err != nil {
			t.Fatalf("insert pagination item %d: %v", i, err)
		}
	}
	if _, err := db.Exec(`UPDATE media_items SET tags_json = '["Favorite"]' WHERE id = 'pagination_movie_03'`); err != nil {
		t.Fatalf("tag pagination item: %v", err)
	}
	if err := server.replaceMediaCategoryFacets("pagination_movie_03"); err != nil {
		t.Fatalf("refresh pagination tag facets: %v", err)
	}

	items, total, sortMode, filterMode, err := server.listLibraryItems("", "lib_pagination_guard", "title", "", "asc", 5, 10)
	if err != nil {
		t.Fatalf("list paged library items: %v", err)
	}
	if total != 16 || len(items) != 5 || sortMode != "title" || filterMode != "all" {
		t.Fatalf("unexpected page metadata total=%d len=%d sort=%q filter=%q", total, len(items), sortMode, filterMode)
	}
	if items[0].ID != "pagination_movie_10" || items[4].ID != "pagination_movie_14" {
		t.Fatalf("unexpected page window: %#v", items)
	}
	cursorItems, cursorTotal, _, _, cursorHasMore, cursor, err := server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "title", "", "asc", "none", "", 5, 0)
	if err != nil {
		t.Fatalf("list cursor first page: %v", err)
	}
	if len(cursorItems) != 5 || cursorItems[0].ID != "pagination_movie_00" || cursorItems[4].ID != "pagination_movie_04" || !cursorHasMore || cursor == "" || cursorTotal != 6 {
		t.Fatalf("unexpected cursor first page total=%d hasMore=%v cursor=%q items=%#v", cursorTotal, cursorHasMore, cursor, cursorItems)
	}
	cursorItems, cursorTotal, _, _, cursorHasMore, cursor, err = server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "title", "", "asc", "none", cursor, 5, 999999)
	if err != nil {
		t.Fatalf("list cursor second page: %v", err)
	}
	if len(cursorItems) != 5 || cursorItems[0].ID != "pagination_movie_05" || cursorItems[4].ID != "pagination_movie_09" || !cursorHasMore || cursor == "" || cursorTotal != 6 {
		t.Fatalf("unexpected cursor second page total=%d hasMore=%v cursor=%q items=%#v", cursorTotal, cursorHasMore, cursor, cursorItems)
	}
	addedItems, addedTotal, _, _, addedHasMore, addedCursor, err := server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "added", "", "desc", "none", "", 5, 0)
	if err != nil {
		t.Fatalf("list added cursor first page: %v", err)
	}
	if len(addedItems) != 5 || addedItems[0].ID != "pagination_movie_15" || addedItems[4].ID != "pagination_movie_11" || !addedHasMore || addedCursor == "" || addedTotal != 6 {
		t.Fatalf("unexpected added cursor first page total=%d hasMore=%v cursor=%q items=%#v", addedTotal, addedHasMore, addedCursor, addedItems)
	}
	canonicalItems, _, canonicalSort, _, _, _, err := server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "dateAdded", "type:movie;titleInitial:P", "desc", "exact", "", 5, 0)
	if err != nil {
		t.Fatalf("list canonical dateAdded/title initial page: %v", err)
	}
	if canonicalSort != "dateAdded" || len(canonicalItems) != 5 || canonicalItems[0].ID != "pagination_movie_15" {
		t.Fatalf("canonical catalogue query sort=%q items=%#v", canonicalSort, canonicalItems)
	}
	_, _, _, _, _, pivotCursor, err := server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "title", "type:movie;titleInitial:P", "asc", "none", "", 5, 0)
	if err != nil || pivotCursor == "" {
		t.Fatalf("create pivot-bound cursor: cursor=%q err=%v", pivotCursor, err)
	}
	if _, _, _, _, _, _, err := server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "title", "type:episode;titleInitial:P", "asc", "none", pivotCursor, 5, 0); !errors.Is(err, errInvalidLibraryCursor) {
		t.Fatalf("cursor crossed entity pivot: %v", err)
	}
	addedItems, addedTotal, _, _, addedHasMore, addedCursor, err = server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "added", "", "desc", "none", addedCursor, 5, 999999)
	if err != nil {
		t.Fatalf("list added cursor second page: %v", err)
	}
	if len(addedItems) != 5 || addedItems[0].ID != "pagination_movie_10" || addedItems[4].ID != "pagination_movie_06" || !addedHasMore || addedCursor == "" || addedTotal != 6 {
		t.Fatalf("unexpected added cursor second page total=%d hasMore=%v cursor=%q items=%#v", addedTotal, addedHasMore, addedCursor, addedItems)
	}
	items, total, _, _, err = server.listLibraryItems("", "lib_pagination_guard", "title", "", "desc", 5, 0)
	if err != nil {
		t.Fatalf("list descending library items: %v", err)
	}
	if total != 16 || len(items) != 5 || items[0].ID != "pagination_movie_15" || items[4].ID != "pagination_movie_11" {
		t.Fatalf("unexpected descending page total=%d items=%#v", total, items)
	}
	items, total, _, _, err = server.listLibraryItems("", "lib_pagination_guard", "title", "", "asc", 5, 15)
	if err != nil {
		t.Fatalf("list final paged library items: %v", err)
	}
	if total != 16 || len(items) != 1 || items[0].ID != "pagination_movie_15" {
		t.Fatalf("unexpected final page total=%d items=%#v", total, items)
	}
	items, total, sortMode, _, err = server.listLibraryItems("", "lib_pagination_guard", "random", "", "asc", 5, 0)
	if err != nil {
		t.Fatalf("list random library items: %v", err)
	}
	if total != 16 || len(items) != 5 || sortMode != "random" {
		t.Fatalf("unexpected random page metadata total=%d len=%d sort=%q", total, len(items), sortMode)
	}
	randomCursorItems, _, _, _, randomHasMore, randomCursor, err := server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "random", "", "asc", "none", "", 5, 0)
	if err != nil {
		t.Fatalf("list random cursor first page: %v", err)
	}
	if len(randomCursorItems) != 5 || !randomHasMore || randomCursor == "" {
		t.Fatalf("unexpected random cursor first page hasMore=%v cursor=%q items=%#v", randomHasMore, randomCursor, randomCursorItems)
	}
	randomCursorItems, _, _, _, randomHasMore, randomCursor, err = server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "random", "", "asc", "none", randomCursor, 5, 999999)
	if err != nil {
		t.Fatalf("list random cursor second page: %v", err)
	}
	if len(randomCursorItems) != 5 || !randomHasMore || randomCursor == "" {
		t.Fatalf("unexpected random cursor second page hasMore=%v cursor=%q items=%#v", randomHasMore, randomCursor, randomCursorItems)
	}
	artistCursorItems, _, _, _, artistHasMore, artistCursor, err := server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "artist", "", "asc", "none", "", 5, 0)
	if err != nil {
		t.Fatalf("list artist cursor first page: %v", err)
	}
	if len(artistCursorItems) != 5 || artistCursorItems[0].ID != "pagination_movie_00" || artistCursorItems[4].ID != "pagination_movie_04" || !artistHasMore || artistCursor == "" {
		t.Fatalf("unexpected artist cursor first page hasMore=%v cursor=%q items=%#v", artistHasMore, artistCursor, artistCursorItems)
	}
	artistCursorItems, _, _, _, artistHasMore, artistCursor, err = server.listLibraryItemsPageContext(context.Background(), "", "lib_pagination_guard", "artist", "", "asc", "none", artistCursor, 5, 999999)
	if err != nil {
		t.Fatalf("list artist cursor second page: %v", err)
	}
	if len(artistCursorItems) != 5 || artistCursorItems[0].ID != "pagination_movie_05" || artistCursorItems[4].ID != "pagination_movie_09" || !artistHasMore || artistCursor == "" {
		t.Fatalf("unexpected artist cursor second page hasMore=%v cursor=%q items=%#v", artistHasMore, artistCursor, artistCursorItems)
	}
	queryPlanRows, err := db.Query(`
		EXPLAIN QUERY PLAN
		SELECT m.id
		FROM media_items m
		WHERE m.library_id = ? AND m.parent_id IS NULL
		ORDER BY m.random_key ASC, m.id ASC
		LIMIT ? OFFSET ?`, "lib_pagination_guard", 5, 0)
	if err != nil {
		t.Fatalf("explain random library items: %v", err)
	}
	defer queryPlanRows.Close()
	randomPlan := []string{}
	for queryPlanRows.Next() {
		var id, parent, notused int
		var detail string
		if err := queryPlanRows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan random query plan: %v", err)
		}
		randomPlan = append(randomPlan, detail)
	}
	if err := queryPlanRows.Err(); err != nil {
		t.Fatalf("scan random query plan rows: %v", err)
	}
	if !strings.Contains(strings.Join(randomPlan, "\n"), "idx_media_library_parent_random") {
		t.Fatalf("random sort query plan did not use random-key index: %v", randomPlan)
	}
	artistPlan := explainQueryPlan(t, server, `
		SELECT m.id
		FROM media_items m
		WHERE m.library_id = ? AND m.parent_id IS NULL
		ORDER BY m.sort_artist_key ASC, m.sort_title ASC, m.id ASC
		LIMIT ? OFFSET ?`, "lib_pagination_guard", 5, 0)
	normalizedArtistPlan := strings.ToLower(strings.Join(artistPlan, "\n"))
	if !strings.Contains(normalizedArtistPlan, "idx_media_library_parent_sort_artist") {
		t.Fatalf("artist sort query plan did not use sort-key index:\n%s", strings.Join(artistPlan, "\n"))
	}
	if strings.Contains(normalizedArtistPlan, "json_extract") {
		t.Fatalf("artist sort query plan still used JSON extraction:\n%s", strings.Join(artistPlan, "\n"))
	}
	artistFilterPlan := explainQueryPlan(t, server, `
		SELECT m.id
		FROM media_items m
		WHERE m.library_id = ? AND m.parent_id IS NULL AND m.filter_artist_key = ?
		ORDER BY m.sort_title ASC, m.id ASC
		LIMIT ? OFFSET ?`, "lib_pagination_guard", "track artist", 5, 0)
	normalizedArtistFilterPlan := strings.ToLower(strings.Join(artistFilterPlan, "\n"))
	if !strings.Contains(normalizedArtistFilterPlan, "idx_media_library_parent_filter_artist_v3") {
		t.Fatalf("artist filter query plan did not use filter-key index:\n%s", strings.Join(artistFilterPlan, "\n"))
	}
	if strings.Contains(normalizedArtistFilterPlan, "json_extract") {
		t.Fatalf("artist filter query plan still used JSON extraction:\n%s", strings.Join(artistFilterPlan, "\n"))
	}
	genreFilterPlan := explainQueryPlan(t, server, `
		SELECT m.id
		FROM media_items m
		WHERE m.library_id = ? AND m.parent_id IS NULL
			AND EXISTS (
				SELECT 1 FROM media_category_facets mcf
				WHERE mcf.media_id = m.id AND mcf.library_id = m.library_id AND mcf.facet_type = 'genre' AND mcf.sort_value = ?
			)
		ORDER BY m.sort_title ASC, m.id ASC
		LIMIT ? OFFSET ?`, "lib_pagination_guard", "drama", 5, 0)
	normalizedGenreFilterPlan := strings.ToLower(strings.Join(genreFilterPlan, "\n"))
	if !strings.Contains(normalizedGenreFilterPlan, "idx_media_category_facets_library") && !strings.Contains(normalizedGenreFilterPlan, "sqlite_autoindex_media_category_facets_1") {
		t.Fatalf("genre filter query plan did not use facet read-model index:\n%s", strings.Join(genreFilterPlan, "\n"))
	}
	if strings.Contains(normalizedGenreFilterPlan, "genres_json") || strings.Contains(normalizedGenreFilterPlan, "json_extract") {
		t.Fatalf("genre filter query plan still used JSON media fields:\n%s", strings.Join(genreFilterPlan, "\n"))
	}
	tagFilterPlan := explainQueryPlan(t, server, `
		SELECT m.id
		FROM media_items m
		WHERE m.library_id = ? AND m.parent_id IS NULL
			AND EXISTS (
				SELECT 1 FROM media_category_facets mcf
				WHERE mcf.media_id = m.id AND mcf.library_id = m.library_id AND mcf.facet_type = 'tag' AND mcf.sort_value = ?
			)
		ORDER BY m.sort_title ASC, m.id ASC
		LIMIT ? OFFSET ?`, "lib_pagination_guard", "favorite", 5, 0)
	normalizedTagFilterPlan := strings.ToLower(strings.Join(tagFilterPlan, "\n"))
	if !strings.Contains(normalizedTagFilterPlan, "idx_media_category_facets_library") && !strings.Contains(normalizedTagFilterPlan, "sqlite_autoindex_media_category_facets_1") {
		t.Fatalf("tag filter query plan did not use facet read-model index:\n%s", strings.Join(tagFilterPlan, "\n"))
	}
	if strings.Contains(normalizedTagFilterPlan, "tags_json") || strings.Contains(normalizedTagFilterPlan, "json_extract") {
		t.Fatalf("tag filter query plan still used JSON media fields:\n%s", strings.Join(tagFilterPlan, "\n"))
	}
	taggedItems, _, _, tagFilterMode, err := server.listLibraryItems("", "lib_pagination_guard", "title", "tag:Favorite", "asc", 5, 0)
	if err != nil {
		t.Fatalf("list tagged library items: %v", err)
	}
	if tagFilterMode != "tag:Favorite" || len(taggedItems) != 1 || taggedItems[0].ID != "pagination_movie_03" {
		t.Fatalf("unexpected tag-filtered items mode=%q items=%#v", tagFilterMode, taggedItems)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	request := BrowseLibraryRequest{
		Pivot: "movies",
		Sort:  []BrowseSort{{Field: "dateAdded", Direction: "desc"}},
		Limit: 5,
	}
	var response BrowseLibraryResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_pagination_guard/browse", request, &response)
	if status != http.StatusOK {
		t.Fatalf("canonical browse first page status = %d, body: %s", status, body)
	}
	if len(response.Items) != 5 || response.Items[0].ID != "pagination_movie_15" || response.Items[4].ID != "pagination_movie_11" || response.PageInfo.NextCursor == nil || !response.PageInfo.HasMore {
		t.Fatalf("unexpected canonical browse first page: %#v", response)
	}
	request.Cursor = *response.PageInfo.NextCursor
	response = BrowseLibraryResponse{}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_pagination_guard/browse", request, &response)
	if status != http.StatusOK {
		t.Fatalf("canonical browse second page status = %d, body: %s", status, body)
	}
	if len(response.Items) != 5 || response.Items[0].ID != "pagination_movie_10" || response.Items[4].ID != "pagination_movie_06" || response.PageInfo.NextCursor == nil || !response.PageInfo.HasMore {
		t.Fatalf("unexpected canonical browse second page: %#v", response)
	}
	request = BrowseLibraryRequest{Pivot: "movies", Sort: []BrowseSort{{Field: "title", Direction: "desc"}}, Limit: 5}
	response = BrowseLibraryResponse{}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_pagination_guard/browse", request, &response)
	if status != http.StatusOK {
		t.Fatalf("descending canonical browse status = %d, body: %s", status, body)
	}
	if response.Items[0].ID != "pagination_movie_15" || response.Items[4].ID != "pagination_movie_11" {
		t.Fatalf("unexpected descending canonical browse window: %#v", response.Items)
	}
	if strings.Contains(body, "Long detail synopsis") || strings.Contains(body, "Detail-only tagline") || strings.Contains(body, "sourceUrl") {
		t.Fatalf("canonical browse response was not lean: %s", body)
	}
}

func TestLibraryItemsAlphabeticalSeekKeepsFollowingPages(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_alpha_seek', 'Alphabet Seek', 'movie', 951, '/tmp/alphabet-seek', '{}', ?)`, now); err != nil {
		t.Fatalf("insert alphabetical seek library: %v", err)
	}
	titles := []string{"Alpha", "Mercury", "Moon", "Nova", "Zulu"}
	for index, title := range titles {
		id := fmt.Sprintf("alpha_seek_%d", index)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, genres_json, added_at, random_key)
			VALUES (?, 'lib_alpha_seek', 'movie', ?, ?, ?, '[]', ?, ?)`, id, title, title, "https://media.example.test/"+id+".mp4", now, mediaRandomKey(id)); err != nil {
			t.Fatalf("insert alphabetical seek item %q: %v", title, err)
		}
	}

	items, total, _, _, hasMore, cursor, err := server.listLibraryItemsPageFromStartContext(context.Background(), "", "lib_alpha_seek", "title", "all", "asc", "M", "exact", "", 2, 0)
	if err != nil {
		t.Fatalf("seek to M: %v", err)
	}
	if total != 4 || len(items) != 2 || items[0].Title != "Mercury" || items[1].Title != "Moon" || !hasMore || cursor == "" {
		t.Fatalf("unexpected M seek page total=%d hasMore=%v cursor=%q items=%#v", total, hasMore, cursor, items)
	}

	items, _, _, _, hasMore, _, err = server.listLibraryItemsPageFromStartContext(context.Background(), "", "lib_alpha_seek", "title", "all", "asc", "M", "none", cursor, 2, 0)
	if err != nil {
		t.Fatalf("continue M seek: %v", err)
	}
	if len(items) != 2 || items[0].Title != "Nova" || items[1].Title != "Zulu" || hasMore {
		t.Fatalf("unexpected M continuation hasMore=%v items=%#v", hasMore, items)
	}

	if _, _, _, _, _, _, err := server.listLibraryItemsPageFromStartContext(context.Background(), "", "lib_alpha_seek", "title", "all", "asc", "N", "none", cursor, 2, 0); !errors.Is(err, errInvalidLibraryCursor) {
		t.Fatalf("seek cursor crossed anchors: %v", err)
	}
	if _, _, _, _, _, _, err := server.listLibraryItemsPageFromStartContext(context.Background(), "", "lib_alpha_seek", "added", "all", "desc", "M", "exact", "", 2, 0); !errors.Is(err, errInvalidLibraryStartAt) {
		t.Fatalf("seek allowed on non-title ordering: %v", err)
	}
}

func TestLibraryListUsesMaintainedRootItemCounts(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES
			('lib_count_read_model_a', 'Count Read Model A', 'movie', 961, '/tmp/count-a', '{}', ?),
			('lib_count_read_model_b', 'Count Read Model B', 'movie', 962, '/tmp/count-b', '{}', ?)`, now, now); err != nil {
		t.Fatalf("insert count libraries: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, parent_id, added_at)
		VALUES
			('count_root_a', 'lib_count_read_model_a', 'movie', 'Count Root A', 'Count Root A', NULL, ?),
			('count_child_a', 'lib_count_read_model_a', 'episode', 'Count Child A', 'Count Child A', 'count_root_a', ?)`, now, now); err != nil {
		t.Fatalf("insert count media: %v", err)
	}
	library, err := server.getLibrary("lib_count_read_model_a")
	if err != nil {
		t.Fatalf("get count library: %v", err)
	}
	if library.Count != 1 {
		t.Fatalf("root count after insert = %d, expected 1", library.Count)
	}
	if _, err := db.Exec(`UPDATE media_items SET parent_id = NULL WHERE id = 'count_child_a'`); err != nil {
		t.Fatalf("promote child media: %v", err)
	}
	if library, err = server.getLibrary("lib_count_read_model_a"); err != nil || library.Count != 2 {
		t.Fatalf("root count after promote = %d err=%v, expected 2", library.Count, err)
	}
	if _, err := db.Exec(`UPDATE media_items SET library_id = 'lib_count_read_model_b' WHERE id = 'count_child_a'`); err != nil {
		t.Fatalf("move root media: %v", err)
	}
	libraries, err := server.listLibraries()
	if err != nil {
		t.Fatalf("list libraries: %v", err)
	}
	counts := map[string]int{}
	for _, library := range libraries {
		if strings.HasPrefix(library.ID, "lib_count_read_model_") {
			counts[library.ID] = library.Count
		}
	}
	if counts["lib_count_read_model_a"] != 1 || counts["lib_count_read_model_b"] != 1 {
		t.Fatalf("root counts after move = %#v, expected one root in each library", counts)
	}
	if _, err := db.Exec(`DELETE FROM media_items WHERE id = 'count_child_a'`); err != nil {
		t.Fatalf("delete root media: %v", err)
	}
	if library, err = server.getLibrary("lib_count_read_model_b"); err != nil || library.Count != 0 {
		t.Fatalf("root count after delete = %d err=%v, expected 0", library.Count, err)
	}
}

func TestLibraryListAvoidsRequestTimeMediaCounts(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "server.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	body := string(source)
	for _, fn := range []string{"listLibraries", "getLibrary"} {
		start := strings.Index(body, "func (s *Server) "+fn)
		if start < 0 {
			t.Fatalf("%s not found", fn)
		}
		end := strings.Index(body[start:], "\nfunc ")
		if end < 0 {
			t.Fatalf("%s end not found", fn)
		}
		functionBody := body[start : start+end]
		if strings.Contains(functionBody, "COUNT(m.id)") || strings.Contains(functionBody, "LEFT JOIN media_items") {
			t.Fatalf("%s should read library_item_counts instead of counting media_items at request time", fn)
		}
		if !strings.Contains(functionBody, "library_item_counts") {
			t.Fatalf("%s should read maintained library item counts", fn)
		}
	}
}

func TestSearchAdmissionRejectsUserWhenConcurrentSearchLimitReached(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)

	server.searchMu.Lock()
	server.searchActive = map[string]int{userID: maxConcurrentSearchesPerUser}
	server.searchMu.Unlock()

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/search", bytes.NewBufferString(`{"query":"Meridian"}`))
	if err != nil {
		t.Fatalf("create search request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeaderName, "1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("limited search request: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	body := string(bodyBytes)
	if resp.StatusCode != http.StatusTooManyRequests || !strings.Contains(body, "search_busy") {
		t.Fatalf("limited search status=%d body=%s", resp.StatusCode, body)
	}
	if resp.Header.Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, expected 1", resp.Header.Get("Retry-After"))
	}
	if rejected := server.admissionDiagnostics().SearchRejected; rejected != 1 {
		t.Fatalf("search rejected diagnostics = %d, expected 1", rejected)
	}

	server.searchMu.Lock()
	server.searchActive = map[string]int{}
	server.searchMu.Unlock()

	var results SearchResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/search", SearchRequest{Query: "Meridian"}, &results)
	if status != http.StatusOK || !searchResponseContains(results, "movie_meridian") {
		t.Fatalf("search after release status=%d body=%s results=%#v", status, body, results)
	}
}

func TestSearchAdmissionRejectsWhenGlobalSearchLimitReached(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	active := map[string]int{}
	for i := 0; i < maxConcurrentSearchesGlobal; i++ {
		active[fmt.Sprintf("usr_global_search_%02d", i)] = 1
	}
	server.searchMu.Lock()
	server.searchActive = active
	server.searchMu.Unlock()

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/search", bytes.NewBufferString(`{"query":"Meridian"}`))
	if err != nil {
		t.Fatalf("create global limited search request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeaderName, "1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("global limited search request: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	body := string(bodyBytes)
	if resp.StatusCode != http.StatusTooManyRequests || !strings.Contains(body, "search_busy") {
		t.Fatalf("global limited search status=%d body=%s", resp.StatusCode, body)
	}
	if diagnostics := server.admissionDiagnostics(); diagnostics.SearchActive != maxConcurrentSearchesGlobal || diagnostics.SearchCapacityGlobal != maxConcurrentSearchesGlobal {
		t.Fatalf("unexpected global search diagnostics: %#v", diagnostics)
	}
}

func TestSearchMediaHonorsCanceledContext(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := server.searchMediaContext(ctx, "", "Meridian", 50)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("searchMediaContext error = %v, expected context.Canceled", err)
	}
}

func TestNavigationQueriesHonorCanceledContext(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, _, _, err := server.listLibraryItemsContext(ctx, "", "lib_movies", "title", "", "asc", 50, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("listLibraryItemsContext error = %v, expected context.Canceled", err)
	}
	if _, err := server.listLibraryCategoriesContext(ctx, "", "lib_movies"); !errors.Is(err, context.Canceled) {
		t.Fatalf("listLibraryCategoriesContext error = %v, expected context.Canceled", err)
	}
	if _, err := server.personMediaContext(ctx, "", "Mara Vale", 50); !errors.Is(err, context.Canceled) {
		t.Fatalf("personMediaContext error = %v, expected context.Canceled", err)
	}
	if _, _, _, _, err := server.loadPlaybackHistoryContext(ctx, playbackHistoryFilters{Limit: 50, Period: "all"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("loadPlaybackHistoryContext error = %v, expected context.Canceled", err)
	}
	if _, err, ok := server.homeRowPageContext(ctx, User{ID: ""}, "watchlist", 50, 0); !ok || !errors.Is(err, context.Canceled) {
		t.Fatalf("homeRowPageContext ok=%v error=%v, expected context.Canceled", ok, err)
	}
	if _, _, err := server.listWatchlistContext(ctx, "", 50, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("listWatchlistContext error = %v, expected context.Canceled", err)
	}
	if _, err := server.listAuditEventsContext(ctx, 50); !errors.Is(err, context.Canceled) {
		t.Fatalf("listAuditEventsContext error = %v, expected context.Canceled", err)
	}
	if _, err := server.listJobsContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("listJobsContext error = %v, expected context.Canceled", err)
	}
}

func TestHomeRowsConcurrentLoaderHonorsCanceledContext(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rows := server.loadHomeRowsConcurrently(ctx, User{ID: ""}, []homeRowTask{{Index: 0, RowID: "watchlist"}, {Index: 1, RowID: "favorites"}})
	if len(rows) != 0 {
		t.Fatalf("canceled home row loader returned rows: %#v", rows)
	}
}

func TestWatchlistEndpointUsesDedicatedBoundedList(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var item MediaItem
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/watchlist", map[string]bool{"watchlisted": true}, &item)
	if status != http.StatusOK {
		t.Fatalf("watchlist mutation status=%d body=%s", status, body)
	}

	var response CursorListResponse[MediaItem]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/watchlist?limit=10", nil, &response)
	if status != http.StatusOK {
		t.Fatalf("watchlist status=%d body=%s", status, body)
	}
	if len(response.Items) == 0 || len(response.Items) > 10 || !mediaIDsContain(response.Items, "movie_meridian") {
		t.Fatalf("unexpected watchlist response: %#v", response)
	}
	for _, item := range response.Items {
		if !item.State.Watchlisted {
			t.Fatalf("watchlist endpoint returned non-watchlisted item: %#v", item)
		}
	}

	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/watchlist?limit=10&filter=unwatched&sort=title&order=asc", nil, &response)
	if status != http.StatusOK {
		t.Fatalf("filtered watchlist status=%d body=%s", status, body)
	}
	if !mediaIDsContain(response.Items, "movie_meridian") {
		t.Fatalf("filtered watchlist metadata/items = %#v", response)
	}
}

func TestAdminListsSupportCursorPagination(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	createdAt := "2035-01-02T03:04:05Z"
	for _, id := range []string{"aud_cursor_c", "aud_cursor_b", "aud_cursor_a"} {
		if _, err := db.Exec(`
			INSERT INTO audit_events (id, actor_user_id, actor_email, action, resource_type, resource_id, severity, metadata_json, client_ip, user_agent, created_at)
			VALUES (?, '', '', 'cursor.test', 'test', ?, 'info', '{}', '', '', ?)`, id, id, createdAt); err != nil {
			t.Fatalf("insert audit cursor event %s: %v", id, err)
		}
	}
	for _, id := range []string{"job_cursor_c", "job_cursor_b", "job_cursor_a"} {
		if _, err := db.Exec(`
			INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, created_at, updated_at)
			VALUES (?, 'cursor_test', 'queued', 0, 'Cursor test', 'test', ?, ?, ?)`, id, id, createdAt, createdAt); err != nil {
			t.Fatalf("insert cursor job %s: %v", id, err)
		}
	}

	auditPage, auditCursor, err := server.listAuditEventsPageContext(context.Background(), 2, "", "")
	if err != nil {
		t.Fatalf("list audit cursor page: %v", err)
	}
	if len(auditPage) != 2 || auditPage[0].ID != "aud_cursor_c" || auditPage[1].ID != "aud_cursor_b" || auditCursor == "" {
		t.Fatalf("unexpected audit first page items=%#v cursor=%q", auditPage, auditCursor)
	}
	auditCreatedAt, auditID, err := decodeTimeIDCursor(auditCursor)
	if err != nil {
		t.Fatalf("decode audit cursor: %v", err)
	}
	auditPage, auditCursor, err = server.listAuditEventsPageContext(context.Background(), 2, auditCreatedAt, auditID)
	if err != nil {
		t.Fatalf("list audit second page: %v", err)
	}
	if len(auditPage) != 1 || auditPage[0].ID != "aud_cursor_a" || auditCursor != "" {
		t.Fatalf("unexpected audit second page items=%#v cursor=%q", auditPage, auditCursor)
	}

	jobPage, jobCursor, err := server.listJobsPageContext(context.Background(), 2, "", "")
	if err != nil {
		t.Fatalf("list jobs cursor page: %v", err)
	}
	if len(jobPage) != 2 || jobPage[0].ID != "job_cursor_c" || jobPage[1].ID != "job_cursor_b" || jobCursor == "" {
		t.Fatalf("unexpected jobs first page items=%#v cursor=%q", jobPage, jobCursor)
	}
	jobCreatedAt, jobID, err := decodeTimeIDCursor(jobCursor)
	if err != nil {
		t.Fatalf("decode job cursor: %v", err)
	}
	jobPage, jobCursor, err = server.listJobsPageContext(context.Background(), 2, jobCreatedAt, jobID)
	if err != nil {
		t.Fatalf("list jobs second page: %v", err)
	}
	if len(jobPage) != 1 || jobPage[0].ID != "job_cursor_a" || jobCursor != "" {
		t.Fatalf("unexpected jobs second page items=%#v cursor=%q", jobPage, jobCursor)
	}

	server.logMu.Lock()
	server.logEvents = []LogEvent{
		{ID: "log_cursor_a", Time: createdAt, Level: "info", Message: "oldest"},
		{ID: "log_cursor_b", Time: createdAt, Level: "info", Message: "middle"},
		{ID: "log_cursor_c", Time: createdAt, Level: "info", Message: "newest"},
	}
	server.logMu.Unlock()
	logPage, logCursor := server.listLogEventsPage(2, "", "")
	if len(logPage) != 2 || logPage[0].ID != "log_cursor_c" || logPage[1].ID != "log_cursor_b" || logCursor == "" {
		t.Fatalf("unexpected log first page items=%#v cursor=%q", logPage, logCursor)
	}
	logTime, logID, err := decodeTimeIDCursor(logCursor)
	if err != nil {
		t.Fatalf("decode log cursor: %v", err)
	}
	logPage, logCursor = server.listLogEventsPage(2, logTime, logID)
	if len(logPage) != 1 || logPage[0].ID != "log_cursor_a" || logCursor != "" {
		t.Fatalf("unexpected log second page items=%#v cursor=%q", logPage, logCursor)
	}
}

func TestOperationalRetentionPrunesPastLimitByCursorThreshold(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	base := time.Date(2035, 1, 2, 3, 4, 5, 0, time.UTC)
	for i := 0; i < 4; i++ {
		createdAt := base.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		auditID := fmt.Sprintf("aud_retention_%d", i)
		if _, err := db.Exec(`
			INSERT INTO audit_events (id, actor_user_id, actor_email, action, resource_type, resource_id, severity, metadata_json, client_ip, user_agent, created_at)
			VALUES (?, '', '', 'retention.test', 'test', ?, 'info', '{}', '', '', ?)`, auditID, auditID, createdAt); err != nil {
			t.Fatalf("insert retention audit event %s: %v", auditID, err)
		}
		jobID := fmt.Sprintf("job_retention_%d", i)
		if _, err := db.Exec(`
			INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, created_at, updated_at)
			VALUES (?, 'retention_test', 'complete', 100, 'Retention test', 'test', ?, ?, ?)`, jobID, jobID, createdAt, createdAt); err != nil {
			t.Fatalf("insert retention job %s: %v", jobID, err)
		}
	}

	if err := server.pruneAuditEventsPastLimit(context.Background(), 2); err != nil {
		t.Fatalf("prune audit events past limit: %v", err)
	}
	if err := server.pruneJobsPastLimit(context.Background(), 2); err != nil {
		t.Fatalf("prune jobs past limit: %v", err)
	}

	var auditIDs, jobIDs string
	if err := db.QueryRow(`
		SELECT group_concat(id, ',')
		FROM (
			SELECT id
			FROM audit_events
			WHERE id LIKE 'aud_retention_%'
			ORDER BY created_at DESC, id DESC
		)`).Scan(&auditIDs); err != nil {
		t.Fatalf("list retained audit events: %v", err)
	}
	if err := db.QueryRow(`
		SELECT group_concat(id, ',')
		FROM (
			SELECT id
			FROM jobs
			WHERE id LIKE 'job_retention_%'
			ORDER BY updated_at DESC, id DESC
		)`).Scan(&jobIDs); err != nil {
		t.Fatalf("list retained jobs: %v", err)
	}
	if auditIDs != "aud_retention_3,aud_retention_2" {
		t.Fatalf("retained audit ids = %q, expected newest two", auditIDs)
	}
	if jobIDs != "job_retention_3,job_retention_2" {
		t.Fatalf("retained job ids = %q, expected newest two", jobIDs)
	}
}

func TestLibraryBrowseRejectsLegacyOffsetField(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var response map[string]any
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_movies/browse", map[string]any{
		"pivot":  "movies",
		"limit":  50,
		"offset": 10001,
	}, &response)
	if status != http.StatusBadRequest {
		t.Fatalf("legacy browse offset status=%d body=%s", status, body)
	}
	if response["code"] != "invalid_browse_request" {
		t.Fatalf("legacy browse offset response = %#v", response)
	}
}

func TestLibrarySourcesRejectLegacyOffsets(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var response map[string]any
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/lib_movies/sources?limit=50&offset=1", nil, &response)
	if status != http.StatusBadRequest {
		t.Fatalf("legacy source offset status=%d body=%s", status, body)
	}
	if response["code"] != "offset_not_supported" {
		t.Fatalf("legacy source offset response = %#v", response)
	}
}

func TestLibrarySourcesUseOpaqueCursorPagination(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_source_cursor', 'Source Cursor', 'movie', 951, '/tmp/source-cursor', '{}', ?)`, now); err != nil {
		t.Fatalf("insert source cursor library: %v", err)
	}
	for index, count := range []int{30, 20, 10} {
		path := fmt.Sprintf("/media/source-%d", index)
		if _, err := db.Exec(`
			INSERT INTO library_source_groups (library_id, kind, path, label, source_type, filter, item_count, file_count, missing_file_count, size_bytes, updated_at)
			VALUES ('lib_source_cursor', 'local', ?, ?, 'local', ?, ?, ?, 0, ?, ?)`,
			path, fmt.Sprintf("Source %d", index), "sourcePath:"+path, count, count, count*1024, now); err != nil {
			t.Fatalf("insert source group %d: %v", index, err)
		}
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var first CursorListResponse[LibrarySourceGroup]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/lib_source_cursor/sources?limit=2", nil, &first)
	if status != http.StatusOK || !first.PageInfo.HasMore || first.PageInfo.NextCursor == nil || len(first.Items) != 2 {
		t.Fatalf("first source page status=%d response=%#v body=%s", status, first, body)
	}
	var second CursorListResponse[LibrarySourceGroup]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/lib_source_cursor/sources?limit=2&cursor="+url.QueryEscape(*first.PageInfo.NextCursor), nil, &second)
	if status != http.StatusOK || second.PageInfo.HasMore || second.PageInfo.NextCursor != nil || len(second.Items) != 1 {
		t.Fatalf("second source page status=%d response=%#v body=%s", status, second, body)
	}
	if first.Items[0].ID == second.Items[0].ID || first.Items[1].ID == second.Items[0].ID {
		t.Fatalf("source cursor repeated an item: first=%#v second=%#v", first.Items, second.Items)
	}
}

func TestPlaybackHistoryCountNoneUsesOverfetch(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("play_count_none_%d", i)
		startedAt := fmt.Sprintf("2035-02-03T04:05:%02dZ", 10-i)
		if _, err := db.Exec(`
			INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, state)
			VALUES (?, ?, ?, 'movie_meridian', 'count_none', ?, ?, ?, 'stopped')`,
			id, userID, userID, id, startedAt, startedAt); err != nil {
			t.Fatalf("insert playback history %s: %v", id, err)
		}
	}

	var response CursorListResponse[PlaybackSession]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/playback/history?period=all&type=count_none&limit=2", nil, &response)
	if status != http.StatusOK {
		t.Fatalf("playback history default-count status=%d body=%s", status, body)
	}
	if len(response.Items) != 2 || !response.PageInfo.HasMore || response.PageInfo.Total != nil {
		t.Fatalf("unexpected default count=none first page: %#v", response)
	}

	response = CursorListResponse[PlaybackSession]{}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/history?period=all&type=count_none&limit=2&count=none", nil, &response)
	if status != http.StatusOK {
		t.Fatalf("playback history count=none status=%d body=%s", status, body)
	}
	if len(response.Items) != 2 || !response.PageInfo.HasMore || response.PageInfo.Total != nil {
		t.Fatalf("unexpected count=none first page: %#v", response)
	}
	if response.Items[0].ID != "play_count_none_0" || response.Items[1].ID != "play_count_none_1" {
		t.Fatalf("unexpected count=none order: %#v", response.Items)
	}
	if response.PageInfo.NextCursor == nil {
		t.Fatalf("expected count=none first page cursor: %#v", response)
	}
	nextCursor := *response.PageInfo.NextCursor
	response = CursorListResponse[PlaybackSession]{}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/history?period=all&type=count_none&limit=2&count=none&cursor="+url.QueryEscape(nextCursor), nil, &response)
	if status != http.StatusOK {
		t.Fatalf("playback history cursor status=%d body=%s", status, body)
	}
	if len(response.Items) != 1 || response.PageInfo.HasMore || response.PageInfo.NextCursor != nil || response.Items[0].ID != "play_count_none_2" {
		t.Fatalf("unexpected count=none cursor page: %#v", response)
	}

	response = CursorListResponse[PlaybackSession]{}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/history?period=all&type=count_none&limit=2&count=exact", nil, &response)
	if status != http.StatusOK {
		t.Fatalf("playback history count=exact status=%d body=%s", status, body)
	}
	if len(response.Items) != 2 || !response.PageInfo.HasMore || response.PageInfo.Total == nil || *response.PageInfo.Total != 3 {
		t.Fatalf("unexpected exact count page: %#v", response)
	}
}

func TestPlaybackHistoryQueryFiltersTitleAndPlayer(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	sessions := []struct {
		id        string
		title     string
		device    string
		app       string
		startedAt string
	}{
		{"play_query_runtime", "Runtime Movie", "Mac Safari", "Portico Web", "2035-02-03T04:05:30Z"},
		{"play_query_roku", "Harbour Episode", "Roku Ultra", "Portico TV", "2035-02-03T04:05:20Z"},
		{"play_query_other", "Coastal Documentary", "Living Room Apple TV", "Portico TV", "2035-02-03T04:05:10Z"},
	}
	for _, session := range sessions {
		if _, err := db.Exec(`
			INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, device, app, state)
			VALUES (?, ?, ?, ?, 'query_filter', ?, ?, ?, ?, ?, 'stopped')`,
			session.id, userID, userID, session.id, session.title, session.startedAt, session.startedAt, session.device, session.app); err != nil {
			t.Fatalf("insert playback history query session %s: %v", session.id, err)
		}
	}

	var response CursorListResponse[PlaybackSession]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/playback/history?period=all&type=query_filter&limit=10&count=exact&query="+url.QueryEscape("Runtime"), nil, &response)
	if status != http.StatusOK {
		t.Fatalf("playback history title query status=%d body=%s", status, body)
	}
	if len(response.Items) != 1 || response.PageInfo.Total == nil || *response.PageInfo.Total != 1 || response.Items[0].ID != "play_query_runtime" {
		t.Fatalf("unexpected playback history title query: %#v", response)
	}

	response = CursorListResponse[PlaybackSession]{}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/history?period=all&type=query_filter&limit=10&count=exact&query="+url.QueryEscape("Roku"), nil, &response)
	if status != http.StatusOK {
		t.Fatalf("playback history player query status=%d body=%s", status, body)
	}
	if len(response.Items) != 1 || response.PageInfo.Total == nil || *response.PageInfo.Total != 1 || response.Items[0].ID != "play_query_roku" {
		t.Fatalf("unexpected playback history player query: %#v", response)
	}
}

func TestPlaybackHistoryExportCSVHonorsFiltersAndEscapes(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	sessions := []struct {
		id        string
		title     string
		device    string
		startedAt string
		endedAt   string
	}{
		{"play_export_formula", `=SUM(1,2)`, "Roku Ultra", "2035-02-03T04:05:30Z", "2035-02-03T04:15:45Z"},
		{"play_export_other", "Coastal Documentary", "Living Room Apple TV", "2035-02-03T04:05:20Z", "2035-02-03T04:07:20Z"},
	}
	for _, session := range sessions {
		if _, err := db.Exec(`
			INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, device, app, location, state, progress, position_seconds, bandwidth_mbps, decision)
			VALUES (?, ?, ?, ?, 'csv_export', ?, ?, ?, ?, ?, 'Portico TV', 'Living Room', 'stopped', 91, 615, 12.5, 'direct_play')`,
			session.id, userID, userID, session.id, session.title, session.startedAt, session.endedAt, session.endedAt, session.device); err != nil {
			t.Fatalf("insert playback history export session %s: %v", session.id, err)
		}
	}

	resp, err := client.Get(serverURL + "/api/playback/history/export.csv?period=all&type=csv_export&limit=1&offset=100&cursor=invalid&query=" + url.QueryEscape("Roku"))
	if err != nil {
		t.Fatalf("get playback history export: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read playback history export: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("playback history export status=%d body=%s", resp.StatusCode, body)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/csv") {
		t.Fatalf("playback history export content type = %q", contentType)
	}
	if disposition := resp.Header.Get("Content-Disposition"); !strings.Contains(disposition, "attachment;") || !strings.Contains(disposition, "portico-play-history-") {
		t.Fatalf("playback history export disposition = %q", disposition)
	}
	if limit := resp.Header.Get("X-Portico-Export-Limit"); limit != strconv.Itoa(maxPlaybackHistoryExportRows) {
		t.Fatalf("playback history export limit header = %q", limit)
	}

	records, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil {
		t.Fatalf("parse playback history export csv: %v\n%s", err, body)
	}
	if len(records) != 2 {
		t.Fatalf("expected header plus one matching row, got %d records: %#v", len(records), records)
	}
	header := records[0]
	row := records[1]
	index := map[string]int{}
	for i, name := range header {
		index[name] = i
	}
	for _, name := range []string{"sessionId", "startedAt", "observedSeconds", "title", "playerDevice", "decision"} {
		if _, ok := index[name]; !ok {
			t.Fatalf("missing csv header %q in %#v", name, header)
		}
	}
	if row[index["sessionId"]] != "play_export_formula" || row[index["playerDevice"]] != "Roku Ultra" {
		t.Fatalf("unexpected exported row: %#v", row)
	}
	if row[index["title"]] != `'=SUM(1,2)` {
		t.Fatalf("expected formula-hardened title, got %q", row[index["title"]])
	}
	if row[index["observedSeconds"]] != "615" {
		t.Fatalf("expected observed seconds 615, got %q", row[index["observedSeconds"]])
	}
	if strings.Contains(string(body), "play_export_other") {
		t.Fatalf("export included non-matching row:\n%s", body)
	}
}

func TestPlaybackHistoryExportCSVRequiresManageServer(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/playback/history/export.csv", nil)
	server.handlePlaybackHistoryExportCSV(rec, req, User{ID: "viewer", Permissions: map[string]bool{}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("playback history export without manageServer status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDVRListsUseBoundedCountNonePages(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_dvr_page', 'DVR Page Source', 'm3u', 1, ?, ?)`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert dvr source: %v", err)
	}
	for index := 0; index < 3; index++ {
		start := now.Add(time.Duration(index) * time.Hour).Format(time.RFC3339)
		end := now.Add(time.Duration(index+1) * time.Hour).Format(time.RFC3339)
		if _, err := db.Exec(`
			INSERT INTO live_tv_recordings (id, user_id, source_id, title, status, starts_at, ends_at, created_at, updated_at)
			VALUES (?, ?, 'src_dvr_page', ?, 'scheduled', ?, ?, ?, ?)`,
			fmt.Sprintf("rec_dvr_page_%d", index), userID, fmt.Sprintf("DVR Page %d", index), start, end, start, start); err != nil {
			t.Fatalf("insert dvr recording %d: %v", index, err)
		}
		if _, err := db.Exec(`
			INSERT INTO live_tv_recording_rules (id, user_id, source_id, title, match_type, retention_days, enabled, created_at, updated_at)
			VALUES (?, ?, 'src_dvr_page', ?, 'single', 30, 1, ?, ?)`,
			fmt.Sprintf("rule_dvr_page_%d", index), userID, fmt.Sprintf("Rule Page %d", index), start, start); err != nil {
			t.Fatalf("insert dvr rule %d: %v", index, err)
		}
	}

	var recordings CursorListResponse[DVRRecording]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/dvr/recordings?limit=2&count=none", nil, &recordings)
	if status != http.StatusOK {
		t.Fatalf("dvr recordings status=%d body=%s", status, body)
	}
	if len(recordings.Items) != 2 || !recordings.PageInfo.HasMore || recordings.PageInfo.NextCursor == nil || recordings.PageInfo.Total != nil {
		t.Fatalf("unexpected dvr recordings page: %#v", recordings)
	}

	var rules CursorListResponse[DVRRecordingRule]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/dvr/rules?limit=2&count=none", nil, &rules)
	if status != http.StatusOK {
		t.Fatalf("dvr rules status=%d body=%s", status, body)
	}
	if len(rules.Items) != 2 || !rules.PageInfo.HasMore || rules.PageInfo.NextCursor == nil || rules.PageInfo.Total != nil {
		t.Fatalf("unexpected dvr rules page: %#v", rules)
	}
}

func TestDVRRecordingLookupUsesPrimaryKey(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_dvr_lookup', 'DVR Lookup Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert dvr lookup source: %v", err)
	}
	for index := 0; index < 10; index++ {
		if _, err := db.Exec(`
			INSERT INTO live_tv_recordings (id, user_id, source_id, title, status, starts_at, ends_at, created_at, updated_at)
			VALUES (?, ?, 'src_dvr_lookup', ?, 'completed', ?, ?, ?, ?)`,
			fmt.Sprintf("rec_dvr_lookup_%02d", index), userID, fmt.Sprintf("Lookup %02d", index), now, now, now, now); err != nil {
			t.Fatalf("insert dvr lookup recording %d: %v", index, err)
		}
	}
	recording, err := server.getDVRRecording("rec_dvr_lookup_07")
	if err != nil {
		t.Fatalf("get dvr recording: %v", err)
	}
	if recording.ID != "rec_dvr_lookup_07" || recording.Title != "Lookup 07" {
		t.Fatalf("unexpected dvr recording lookup: %#v", recording)
	}
	plan := explainQueryPlan(t, server, `
		SELECT id, COALESCE(rule_id, ''), user_id, source_id, COALESCE(channel_id, ''), program_id, title,
			COALESCE(folder, ''), status, starts_at, ends_at, path, size_bytes, error, created_at, updated_at
		FROM live_tv_recordings
		WHERE id = ?`, "rec_dvr_lookup_07")
	normalizedPlan := strings.ToLower(strings.Join(plan, "\n"))
	if strings.Contains(normalizedPlan, "scan live_tv_recordings") {
		t.Fatalf("dvr recording lookup scanned recordings table:\n%s", strings.Join(plan, "\n"))
	}
}

func TestDVRRuleLookupUsesPrimaryKey(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_dvr_rule_lookup', 'DVR Rule Lookup Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert dvr rule lookup source: %v", err)
	}
	for index := 0; index < 10; index++ {
		if _, err := db.Exec(`
			INSERT INTO live_tv_recording_rules (id, user_id, source_id, title, match_type, retention_days, enabled, created_at, updated_at)
			VALUES (?, ?, 'src_dvr_rule_lookup', ?, 'single', 30, 1, ?, ?)`,
			fmt.Sprintf("rule_dvr_lookup_%02d", index), userID, fmt.Sprintf("Rule Lookup %02d", index), now, now); err != nil {
			t.Fatalf("insert dvr lookup rule %d: %v", index, err)
		}
	}
	rule, err := server.getDVRRule("rule_dvr_lookup_07")
	if err != nil {
		t.Fatalf("get dvr rule: %v", err)
	}
	if rule.ID != "rule_dvr_lookup_07" || rule.Title != "Rule Lookup 07" {
		t.Fatalf("unexpected dvr rule lookup: %#v", rule)
	}
	plan := explainQueryPlan(t, server, `
		SELECT id, user_id, source_id, COALESCE(channel_id, ''), program_id, title, match_type,
			COALESCE(folder, ''), start_padding_minutes, end_padding_minutes, retention_days, enabled, created_at, updated_at
		FROM live_tv_recording_rules
		WHERE id = ?`, "rule_dvr_lookup_07")
	normalizedPlan := strings.ToLower(strings.Join(plan, "\n"))
	if strings.Contains(normalizedPlan, "scan live_tv_recording_rules") {
		t.Fatalf("dvr rule lookup scanned rules table:\n%s", strings.Join(plan, "\n"))
	}
}

func TestDVRRecordingGroupsArePagedAndSampled(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC().Truncate(time.Second)
	nowText := now.Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_dvr_groups', 'DVR Groups Source', 'm3u', 1, ?, ?)`, nowText, nowText); err != nil {
		t.Fatalf("insert dvr group source: %v", err)
	}
	for groupIndex := 0; groupIndex < 5; groupIndex++ {
		for itemIndex := 0; itemIndex < 5; itemIndex++ {
			start := now.Add(time.Duration(groupIndex*10+itemIndex) * time.Minute).Format(time.RFC3339)
			end := now.Add(time.Duration(groupIndex*10+itemIndex+1) * time.Minute).Format(time.RFC3339)
			if _, err := db.Exec(`
				INSERT INTO live_tv_recordings (id, user_id, source_id, title, folder, status, starts_at, ends_at, size_bytes, created_at, updated_at)
				VALUES (?, ?, 'src_dvr_groups', ?, 'Evening', 'complete', ?, ?, ?, ?, ?)`,
				fmt.Sprintf("rec_dvr_group_%02d_%02d", groupIndex, itemIndex), userID, fmt.Sprintf("Grouped Show %02d", groupIndex), start, end, int64(100+itemIndex), nowText, nowText); err != nil {
				t.Fatalf("insert dvr group recording %d/%d: %v", groupIndex, itemIndex, err)
			}
		}
	}

	var groups CursorListResponse[DVRRecordingGroup]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/dvr/recording-groups?limit=2&count=exact", nil, &groups)
	if status != http.StatusOK {
		t.Fatalf("dvr recording groups status=%d body=%s", status, body)
	}
	if len(groups.Items) != 2 || !groups.PageInfo.HasMore || groups.PageInfo.NextCursor == nil || groups.PageInfo.Total == nil || *groups.PageInfo.Total != 5 {
		t.Fatalf("unexpected dvr recording groups page: %#v", groups)
	}
	if groups.Items[0].Count != 5 || len(groups.Items[0].Recordings) != dvrRecordingGroupSampleLimit {
		t.Fatalf("group should retain aggregate count with bounded samples: %#v", groups.Items[0])
	}
	if groups.Items[0].Recordings[0].StartsAt < groups.Items[0].Recordings[1].StartsAt {
		t.Fatalf("group samples were not newest-first: %#v", groups.Items[0].Recordings)
	}

	plan := explainQueryPlan(t, server, `
		SELECT
			COALESCE(NULLIF(TRIM(title), ''), 'Untitled Recording') AS group_title,
			COALESCE(folder, '') AS group_folder,
			COUNT(*) AS recording_count,
			COALESCE(SUM(size_bytes), 0) AS size_bytes,
			MAX(starts_at) AS latest_recording_at
		FROM live_tv_recordings
		WHERE status <> 'scheduled'
		GROUP BY group_folder, group_title
		ORDER BY latest_recording_at DESC, group_title ASC
		LIMIT ? OFFSET ?`, 3, 0)
	normalizedPlan := strings.ToLower(strings.Join(plan, "\n"))
	if !strings.Contains(normalizedPlan, "idx_recordings_group_admin") && !strings.Contains(normalizedPlan, "idx_recordings_group_user") && !strings.Contains(normalizedPlan, "idx_recordings_group_profile") {
		t.Fatalf("dvr recording group query did not use grouping index:\n%s", strings.Join(plan, "\n"))
	}
	if strings.Contains(normalizedPlan, "scan live_tv_recordings\n") {
		t.Fatalf("dvr recording group query scanned recordings table without an index:\n%s", strings.Join(plan, "\n"))
	}
}

func TestStorageReportReturnsStaleCacheWhileRefreshing(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	first := server.cachedSystemStorageReport(true)
	if first.GeneratedAt == "" {
		t.Fatalf("initial storage report missing generatedAt")
	}
	optimizedDir := filepath.Join(server.cfg.AppDataDir, "optimized")
	if err := os.MkdirAll(optimizedDir, 0o700); err != nil {
		t.Fatalf("create optimized dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(optimizedDir, "fresh.bin"), []byte("fresh optimized bytes"), 0o600); err != nil {
		t.Fatalf("write optimized file: %v", err)
	}
	staleAt := time.Now().Add(-2 * systemStorageReportCacheTTL)
	server.storageCacheMu.Lock()
	server.storageCacheAt = staleAt
	server.storageCacheMu.Unlock()

	stale := server.cachedSystemStorageReport(false)
	if stale.GeneratedAt != first.GeneratedAt {
		t.Fatalf("stale storage request generated synchronously: first=%q stale=%q", first.GeneratedAt, stale.GeneratedAt)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		server.storageCacheMu.Lock()
		refreshedAt := server.storageCacheAt
		running := server.storageRefreshRunning
		server.storageCacheMu.Unlock()
		if refreshedAt.After(staleAt) && !running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	server.storageCacheMu.Lock()
	defer server.storageCacheMu.Unlock()
	t.Fatalf("storage cache did not refresh asynchronously: at=%s running=%v generatedAt=%q", server.storageCacheAt, server.storageRefreshRunning, server.storageCache.GeneratedAt)
}

func TestStorageReportColdCacheReturnsFastPlaceholder(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	optimizedDir := filepath.Join(server.cfg.AppDataDir, "optimized")
	if err := os.MkdirAll(optimizedDir, 0o700); err != nil {
		t.Fatalf("create optimized dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(optimizedDir, "cold.bin"), []byte("cold optimized bytes"), 0o600); err != nil {
		t.Fatalf("write optimized file: %v", err)
	}
	server.clearSystemStorageCache()

	first := server.cachedSystemStorageReport(false)
	optimized := storageCategoryByKey(first, "optimized")
	if optimized == nil {
		t.Fatalf("cold storage report missing optimized category: %#v", first.Categories)
	}
	if !optimized.Available || optimized.SizeBytes != 0 || optimized.FileCount != 0 {
		t.Fatalf("cold storage report should return fast directory placeholder, got %#v", optimized)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		server.storageCacheMu.Lock()
		report := server.storageCache
		running := server.storageRefreshRunning
		server.storageCacheMu.Unlock()
		if category := storageCategoryByKey(report, "optimized"); category != nil && category.SizeBytes > 0 && category.FileCount > 0 && !running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	server.storageCacheMu.Lock()
	defer server.storageCacheMu.Unlock()
	t.Fatalf("storage cache did not asynchronously fill usage: running=%v report=%#v", server.storageRefreshRunning, server.storageCache)
}

func storageCategoryByKey(report SystemStorageReport, key string) *SystemStorageCategory {
	for i := range report.Categories {
		if report.Categories[i].Key == key {
			return &report.Categories[i]
		}
	}
	return nil
}

func TestSearchInFlightCoalescesIdenticalKeys(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	key := searchInFlightKey("usr_test", mediaSearchQuery("Meridian"), 50)
	first, owner := server.beginSearchInFlight(key)
	if !owner {
		t.Fatal("first search in-flight caller should own the query")
	}
	second, owner := server.beginSearchInFlight(key)
	if owner || second != first {
		t.Fatal("second identical search should wait on the first call")
	}
	server.finishSearchInFlight(key, first, []MediaItem{{ID: "movie_meridian"}}, nil)
	select {
	case <-second.done:
	case <-time.After(time.Second):
		t.Fatal("waiting search was not released")
	}
	if len(second.items) != 1 || second.items[0].ID != "movie_meridian" || second.err != nil {
		t.Fatalf("unexpected coalesced search result items=%#v err=%v", second.items, second.err)
	}
	_, owner = server.beginSearchInFlight(key)
	if !owner {
		t.Fatal("completed in-flight search key should be reusable")
	}
}

func TestPlaybackCommandEventsAreNotificationDriven(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	payload := authenticatedPlaybackRuntimeRequest("movie_meridian")
	payload["clientInstanceId"] = "web-command-events"
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", payload, &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status=%d body=%s", status, body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/playback-sessions/"+playback.SessionID+"/command/events", nil)
	if err != nil {
		t.Fatalf("create command event request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open command event stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("command event stream status=%d", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read initial command stream line: %v", err)
	}
	if strings.TrimSpace(line) != ": ready" {
		t.Fatalf("initial command stream line = %q, expected ready comment", line)
	}

	var command PlaybackCommand
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions/"+playback.SessionID+"/command", PlaybackCommandRequest{Action: "pause"}, &command)
	if status != http.StatusOK || command.ID == "" {
		t.Fatalf("issue command status=%d body=%s command=%#v", status, body, command)
	}

	eventLine := make(chan string, 1)
	errs := make(chan error, 1)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				errs <- err
				return
			}
			if strings.TrimSpace(line) == "event: command" {
				eventLine <- line
				return
			}
		}
	}()
	select {
	case <-eventLine:
	case err := <-errs:
		t.Fatalf("read pushed command event: %v", err)
	case <-time.After(700 * time.Millisecond):
		t.Fatal("timed out waiting for pushed command event")
	}
}

func TestWatchWithFriendsEventsAreNotificationDriven(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var group WatchWithFriendsGroup
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/watch-with-friends/groups", WatchWithFriendsCreateRequest{MediaID: "movie_meridian", Name: "Push Group"}, &group)
	if status != http.StatusCreated || group.ID == "" {
		t.Fatalf("create watch-with-friends group status=%d body=%s group=%#v", status, body, group)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/events", nil)
	if err != nil {
		t.Fatalf("create watch-with-friends event request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open watch-with-friends event stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("watch-with-friends event stream status=%d", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	if eventName, err := readSSEEventName(reader); err != nil || eventName != "group" {
		t.Fatalf("initial watch-with-friends event = %q err=%v", eventName, err)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/member/state", WatchWithFriendsMemberStateRequest{State: "ready", PositionSeconds: 17}, &group)
	if status != http.StatusOK {
		t.Fatalf("update watch-with-friends member state status=%d body=%s", status, body)
	}

	eventNames := make(chan string, 1)
	errs := make(chan error, 1)
	go func() {
		eventName, err := readSSEEventName(reader)
		if err != nil {
			errs <- err
			return
		}
		eventNames <- eventName
	}()
	select {
	case eventName := <-eventNames:
		if eventName != "group" {
			t.Fatalf("pushed watch-with-friends event = %q, expected group", eventName)
		}
	case err := <-errs:
		t.Fatalf("read pushed watch-with-friends event: %v", err)
	case <-time.After(700 * time.Millisecond):
		t.Fatal("timed out waiting for pushed watch-with-friends event")
	}
}

func readSSEEventName(reader *bufio.Reader) (string, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(line, "event: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "event: ")), nil
		}
	}
}

func TestMediaBulkStateAndJobsAreBoundedServerSide(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	ids := []string{"movie_meridian", "movie_orchid"}
	var stateResponse ListResponse[MediaItem]
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/media/bulk/state", map[string]any{
		"mediaIds":    ids,
		"watchlisted": true,
		"favorite":    true,
		"watched":     true,
	}, &stateResponse)
	if status != http.StatusOK {
		t.Fatalf("bulk state status=%d body=%s", status, body)
	}
	if stateResponse.Total != 2 || len(stateResponse.Items) != 2 {
		t.Fatalf("unexpected bulk state response: %#v", stateResponse)
	}
	for _, item := range stateResponse.Items {
		if !item.State.Watchlisted || !item.State.Favorite || !item.State.Watched {
			t.Fatalf("bulk state did not update item: %#v", item)
		}
	}

	var jobsResponse ListResponse[Job]
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/media/bulk/jobs", map[string]any{
		"mediaIds": ids,
		"type":     "metadata_refresh",
	}, &jobsResponse)
	if status != http.StatusCreated {
		t.Fatalf("bulk jobs status=%d body=%s", status, body)
	}
	if jobsResponse.Total != 2 || len(jobsResponse.Items) != 2 {
		t.Fatalf("unexpected bulk jobs response: %#v", jobsResponse)
	}

	var metadataResponse ListResponse[MediaItem]
	expectedRevisions := make(map[string]int, len(stateResponse.Items))
	for _, item := range stateResponse.Items {
		expectedRevisions[item.ID] = item.MetadataRevision
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/media/bulk/metadata", map[string]any{
		"mediaIds":          ids,
		"expectedRevisions": expectedRevisions,
		"patch": map[string]any{
			"tags": []string{"bulk-edited"},
		},
	}, &metadataResponse)
	if status != http.StatusOK {
		t.Fatalf("bulk metadata status=%d body=%s", status, body)
	}
	if metadataResponse.Total != 2 || len(metadataResponse.Items) != 2 {
		t.Fatalf("unexpected bulk metadata response: %#v", metadataResponse)
	}
	for _, item := range metadataResponse.Items {
		if !stringSliceContains(item.Tags, "bulk-edited") {
			t.Fatalf("bulk metadata did not update tags: %#v", item)
		}
		if len(item.Streams) != 0 || len(item.MediaFiles) != 0 || len(item.MediaImages) != 0 || len(item.Children) != 0 || len(item.RecommendationRows) != 0 {
			t.Fatalf("bulk metadata response included detail hydration: streams=%d files=%d images=%d children=%d recommendations=%d",
				len(item.Streams), len(item.MediaFiles), len(item.MediaImages), len(item.Children), len(item.RecommendationRows))
		}
	}

	tooMany := make([]string, maxBulkMediaItems+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("bulk_limit_%03d", index)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/media/bulk/state", map[string]any{
		"mediaIds":    tooMany,
		"watchlisted": true,
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "bulk_limit_exceeded") {
		t.Fatalf("bulk limit status=%d body=%s", status, body)
	}
}

func TestBulkMutationsUseListItemLoaders(t *testing.T) {
	assertFunctionUsesListItemLoader(t, "server.go", "func (s *Server) handleMediaBulkJobs")
	assertFunctionUsesListItemLoader(t, "production.go", "func (s *Server) createPlaylist")
	assertFunctionUsesListItemLoader(t, "production.go", "func (s *Server) addPlaylistItemsBulk")
}

func TestMediaDetailRecommendationsAreLazyAPI(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	if err := server.replaceProviderMediaPeople("movie_meridian", []MediaPerson{{Name: "Ari Vega", Role: "Actor", SortOrder: 0}}, "tmdb"); err != nil {
		t.Fatalf("seed source people: %v", err)
	}
	if err := server.replaceProviderMediaPeople("movie_blackwater", []MediaPerson{{Name: "Ari Vega", Role: "Actor", SortOrder: 0}}, "tmdb"); err != nil {
		t.Fatalf("seed match people: %v", err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var shell MediaItem
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &shell)
	if status != http.StatusOK {
		t.Fatalf("detail shell status=%d body=%s", status, body)
	}
	if strings.Contains(body, "recommendationRows") || len(shell.RecommendationRows) != 0 {
		t.Fatalf("detail shell included recommendations: %s", body)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/watchlist", map[string]bool{"watchlisted": true}, &shell)
	if status != http.StatusOK {
		t.Fatalf("watchlist status=%d body=%s", status, body)
	}
	if strings.Contains(body, "recommendationRows") || len(shell.RecommendationRows) != 0 {
		t.Fatalf("watchlist mutation included recommendations: %s", body)
	}

	var recommendations ListResponse[HomeRow]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian/recommendations", nil, &recommendations)
	if status != http.StatusOK {
		t.Fatalf("recommendations status=%d body=%s", status, body)
	}
	if recommendations.Total == 0 || len(recommendations.Items) == 0 {
		t.Fatalf("expected lazy recommendations, got %#v", recommendations)
	}

	var expanded MediaItem
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian?includeRecommendations=true", nil, &expanded)
	if status != http.StatusOK {
		t.Fatalf("expanded detail status=%d body=%s", status, body)
	}
	if len(expanded.RecommendationRows) == 0 {
		t.Fatalf("expected opt-in recommendations in expanded detail: %#v", expanded)
	}
}

func TestDiscoveryRecommendationQueriesUseIndexedReadModels(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	providerPlan := explainQueryPlan(t, server, `
		SELECT p.external_id, m.id
		FROM media_provider_ids p
		JOIN media_items m ON m.id = p.media_id
		WHERE p.provider = 'tmdb'
			AND p.external_id IN (?, ?)
			AND m.parent_id IS NULL
			AND m.type IN ('movie', 'show', 'anime')`, "100", "101")
	normalizedProviderPlan := strings.ToLower(strings.Join(providerPlan, "\n"))
	if !strings.Contains(normalizedProviderPlan, "idx_media_provider_ids_external") {
		t.Fatalf("TMDB trending match plan did not use provider-id index:\n%s", strings.Join(providerPlan, "\n"))
	}

	genrePlan := explainQueryPlan(t, server, `
		SELECT m.id
		FROM media_items m
		WHERE m.parent_id IS NULL
			AND m.type IN ('movie')
			AND EXISTS (
				SELECT 1
				FROM media_category_facets recommendation_genre
				WHERE recommendation_genre.media_id = m.id
					AND recommendation_genre.library_id = COALESCE(m.library_id, '')
					AND recommendation_genre.facet_type = 'genre'
					AND recommendation_genre.sort_value IN (?, ?)
			)
		ORDER BY (SELECT COUNT(1)
			FROM media_category_facets recommendation_genre_score
			WHERE recommendation_genre_score.media_id = m.id
				AND recommendation_genre_score.library_id = COALESCE(m.library_id, '')
				AND recommendation_genre_score.facet_type = 'genre'
				AND recommendation_genre_score.sort_value IN (?, ?)
		) DESC, m.community_rating DESC
		LIMIT ?`, "drama", "thriller", "drama", "thriller", 24)
	normalizedGenrePlan := strings.ToLower(strings.Join(genrePlan, "\n"))
	if !strings.Contains(normalizedGenrePlan, "media_category_facets") {
		t.Fatalf("detail genre recommendation plan did not use facet read model:\n%s", strings.Join(genrePlan, "\n"))
	}
	if strings.Contains(normalizedGenrePlan, "json_each") || strings.Contains(normalizedGenrePlan, "genres_json") {
		t.Fatalf("detail genre recommendation plan still used JSON expansion:\n%s", strings.Join(genrePlan, "\n"))
	}

	networkPlan := explainQueryPlan(t, server, `
		SELECT m.id
		FROM media_items m
		WHERE m.parent_id IS NULL
			AND m.type IN ('show', 'anime')
			AND m.filter_network_key = ?
			AND m.id <> ?
		ORDER BY m.community_rating DESC, m.year DESC, m.sort_title ASC
		LIMIT ?`, "portico", "show_source", 24)
	normalizedNetworkPlan := strings.ToLower(strings.Join(networkPlan, "\n"))
	if strings.Contains(normalizedNetworkPlan, "json_extract") || strings.Contains(normalizedNetworkPlan, "typed_metadata_json") {
		t.Fatalf("network recommendation plan still used JSON extraction:\n%s", strings.Join(networkPlan, "\n"))
	}
	studioPlan := explainQueryPlan(t, server, `
		SELECT m.id
		FROM media_items m
		WHERE m.parent_id IS NULL
			AND m.type IN ('movie')
			AND (m.filter_studio_key = ? OR m.filter_label_key = ?)
			AND m.id <> ?
		ORDER BY m.community_rating DESC, m.year DESC, m.sort_title ASC
		LIMIT ?`, "portico", "portico", "movie_source", 24)
	normalizedStudioPlan := strings.ToLower(strings.Join(studioPlan, "\n"))
	if strings.Contains(normalizedStudioPlan, "json_extract") || strings.Contains(normalizedStudioPlan, "typed_metadata_json") {
		t.Fatalf("studio recommendation plan still used JSON extraction:\n%s", strings.Join(studioPlan, "\n"))
	}
	artistPlan := explainQueryPlan(t, server, `
		SELECT m.id
		FROM media_items m
		WHERE m.id <> ?
			AND m.type IN ('artist', 'album', 'track')
			AND (m.filter_artist_key = ? OR (m.type = 'artist' AND lower(trim(m.title)) = ?))
		ORDER BY CASE m.type WHEN 'artist' THEN 0 WHEN 'album' THEN 1 ELSE 2 END, m.year DESC, m.index_number ASC, m.sort_title ASC
		LIMIT ?`, "track_source", "mara vale", "mara vale", 24)
	normalizedArtistRecommendationPlan := strings.ToLower(strings.Join(artistPlan, "\n"))
	if strings.Contains(normalizedArtistRecommendationPlan, "json_extract") || strings.Contains(normalizedArtistRecommendationPlan, "typed_metadata_json") {
		t.Fatalf("artist recommendation plan still used JSON extraction:\n%s", strings.Join(artistPlan, "\n"))
	}
	authorPlan := explainQueryPlan(t, server, `
		SELECT m.id
		FROM media_items m
		WHERE m.id <> ?
			AND m.type = 'audiobook'
			AND m.filter_author_key = ?
		ORDER BY m.year DESC, m.sort_title ASC
		LIMIT ?`, "book_source", "mara vale", 24)
	normalizedAuthorPlan := strings.ToLower(strings.Join(authorPlan, "\n"))
	if strings.Contains(normalizedAuthorPlan, "json_extract") || strings.Contains(normalizedAuthorPlan, "typed_metadata_json") {
		t.Fatalf("author recommendation plan still used JSON extraction:\n%s", strings.Join(authorPlan, "\n"))
	}
}

func TestPlaybackLookupUsesMinimalDetailPayload(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	if err := server.replaceProviderMediaPeople("movie_meridian", []MediaPerson{{Name: "Ari Vega", Role: "Actor", SortOrder: 0}}, "tmdb"); err != nil {
		t.Fatalf("seed source people: %v", err)
	}
	if err := server.replaceProviderMediaPeople("movie_blackwater", []MediaPerson{{Name: "Ari Vega", Role: "Actor", SortOrder: 0}}, "tmdb"); err != nil {
		t.Fatalf("seed match people: %v", err)
	}
	var sourceURL string
	if err := db.QueryRow(`SELECT COALESCE(source_url, '') FROM media_items WHERE id = 'movie_meridian'`).Scan(&sourceURL); err != nil {
		t.Fatalf("load source url: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at)
		VALUES
			('playback_seed_selected_file', 'movie_meridian', 'lib_movies', ?, 'local', 1, 2048, ?, ?),
			('playback_seed_extra_file', 'movie_meridian', 'lib_movies', '/tmp/playback-seed/extra-version.mkv', 'local', 1, 4096, ?, ?)`,
		sourceURL, now, now, now, now); err != nil {
		t.Fatalf("seed media files: %v", err)
	}

	item, err := server.getMediaPlaybackDetailForUser(context.Background(), user, "movie_meridian")
	if err != nil {
		t.Fatalf("load playback detail: %v", err)
	}
	if item.ID != "movie_meridian" || item.Title == "" || item.SourceURL == "" {
		t.Fatalf("playback detail did not include source identity: %#v", item)
	}
	if len(item.RecommendationRows) != 0 || len(item.Children) != 0 || len(item.Attachments) != 0 || len(item.Segments) != 0 || len(item.ProviderIDs) != 0 || len(item.MatchCandidates) != 0 || len(item.IdentityEvidence) != 0 {
		t.Fatalf("playback detail included detail-page payload: %#v", item)
	}
	if len(item.MediaFiles) != 1 || item.MediaFiles[0].Path != sourceURL || !item.MediaFiles[0].Selected {
		t.Fatalf("playback detail should include only the selected primary file, got %#v", item.MediaFiles)
	}
}

func TestMediaStreamSeedSkipsPlaybackHydration(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, display_title, source_url)
		VALUES ('stream_seed_subtitle', 'movie_meridian', 'subtitle', 'webvtt', 'Managed subtitle', '/api/media/movie_meridian/subtitles/stream_seed_subtitle')`); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at)
		VALUES ('stream_seed_file', 'movie_meridian', 'lib_movies', '/tmp/stream-seed/movie.mp4', 'local', 1, 1024, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed media file: %v", err)
	}

	item, err := server.getMediaStreamSeedForUser(context.Background(), user, "movie_meridian")
	if err != nil {
		t.Fatalf("load stream seed: %v", err)
	}
	if item.ID != "movie_meridian" || item.SourceURL == "" || item.LibraryID == "" {
		t.Fatalf("stream seed missing source identity: %#v", item)
	}
	if len(item.Streams) != 0 || len(item.MediaFiles) != 0 || len(item.Children) != 0 || len(item.RecommendationRows) != 0 || len(item.MediaImages) != 0 || item.AudioNormalization != nil {
		t.Fatalf("stream seed included playback/detail hydration: streams=%d files=%d children=%d recommendations=%d images=%d normalization=%#v",
			len(item.Streams), len(item.MediaFiles), len(item.Children), len(item.RecommendationRows), len(item.MediaImages), item.AudioNormalization)
	}
}

func TestMediaDownloadSeedOnlyHydratesMediaFiles(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, display_title, source_url)
		VALUES ('download_seed_subtitle', 'movie_meridian', 'subtitle', 'webvtt', 'Managed subtitle', '/api/media/movie_meridian/subtitles/download_seed_subtitle')`); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at)
		VALUES ('download_seed_file', 'movie_meridian', 'lib_movies', '/tmp/download-seed/movie.mp4', 'local', 1, 1024, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed media file: %v", err)
	}

	item, err := server.getMediaDownloadSeedForUser(context.Background(), user, "movie_meridian")
	if err != nil {
		t.Fatalf("load download seed: %v", err)
	}
	if item.ID != "movie_meridian" || item.SourceURL == "" || len(item.MediaFiles) == 0 {
		t.Fatalf("download seed missing expected source/files: %#v", item)
	}
	if len(item.Streams) != 0 || len(item.Children) != 0 || len(item.RecommendationRows) != 0 || len(item.MediaImages) != 0 || item.AudioNormalization != nil {
		t.Fatalf("download seed included full playback/detail hydration: streams=%d children=%d recommendations=%d images=%d normalization=%#v",
			len(item.Streams), len(item.Children), len(item.RecommendationRows), len(item.MediaImages), item.AudioNormalization)
	}
}

func TestBackgroundSourceSeedSkipsPlaybackHydration(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, display_title, source_url)
		VALUES ('background_seed_stream', 'movie_meridian', 'video', 'h264', 'Video', '')`); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at)
		VALUES ('background_seed_file', 'movie_meridian', 'lib_movies', '/tmp/background-seed/movie.mp4', 'local', 1, 1024, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed media file: %v", err)
	}

	item, err := server.getMediaBackgroundSourceSeedContext(context.Background(), "movie_meridian")
	if err != nil {
		t.Fatalf("load background source seed: %v", err)
	}
	if item.ID != "movie_meridian" || item.SourceURL == "" || item.LibraryID == "" || item.Title == "" {
		t.Fatalf("background seed missing source identity: %#v", item)
	}
	if len(item.Streams) != 0 || len(item.MediaFiles) != 0 || len(item.Children) != 0 || len(item.RecommendationRows) != 0 || len(item.MediaImages) != 0 || item.AudioNormalization != nil {
		t.Fatalf("background source seed included playback/detail hydration: streams=%d files=%d children=%d recommendations=%d images=%d normalization=%#v",
			len(item.Streams), len(item.MediaFiles), len(item.Children), len(item.RecommendationRows), len(item.MediaImages), item.AudioNormalization)
	}
}

func TestArtworkSeedSkipsPlaybackHydration(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, display_title, source_url)
		VALUES ('artwork_seed_stream', 'movie_meridian', 'video', 'h264', 'Video', '')`); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at)
		VALUES ('artwork_seed_file', 'movie_meridian', 'lib_movies', '/tmp/artwork-seed/movie.mp4', 'local', 1, 1024, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed media file: %v", err)
	}

	item, err := server.getMediaArtworkSeedContext(context.Background(), userID, "movie_meridian")
	if err != nil {
		t.Fatalf("load artwork seed: %v", err)
	}
	if item.ID != "movie_meridian" || item.Images.Poster == "" || item.SourceURL == "" {
		t.Fatalf("artwork seed missing expected image/source fields: %#v", item)
	}
	if len(item.Streams) != 0 || len(item.MediaFiles) != 0 || len(item.Children) != 0 || len(item.RecommendationRows) != 0 || len(item.MediaImages) != 0 || item.AudioNormalization != nil {
		t.Fatalf("artwork seed included playback/detail hydration: streams=%d files=%d children=%d recommendations=%d images=%d normalization=%#v",
			len(item.Streams), len(item.MediaFiles), len(item.Children), len(item.RecommendationRows), len(item.MediaImages), item.AudioNormalization)
	}
}

func TestMediaAccessSummarySkipsPlaybackHydration(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	item, err := server.getMediaAccessSummaryContext(context.Background(), "", "movie_meridian")
	if err != nil {
		t.Fatalf("getMediaAccessSummaryContext: %v", err)
	}
	if item.ID != "movie_meridian" || item.Title == "" {
		t.Fatalf("unexpected media access summary: %#v", item)
	}
	if len(item.Streams) != 0 || len(item.MediaFiles) != 0 || len(item.Children) != 0 || len(item.RecommendationRows) != 0 || len(item.MediaImages) != 0 {
		t.Fatalf("access summary included playback/detail hydration: streams=%d files=%d children=%d recommendations=%d images=%d",
			len(item.Streams), len(item.MediaFiles), len(item.Children), len(item.RecommendationRows), len(item.MediaImages))
	}
}

func TestMediaMutationResponseUsesListItemPayload(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, display_title, source_url)
		VALUES ('mutation_stream', 'movie_meridian', 'subtitle', 'webvtt', 'Managed subtitle', '/api/media/movie_meridian/subtitles/mutation_stream')`); err != nil {
		t.Fatalf("seed mutation stream: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at)
		VALUES ('mutation_file', 'movie_meridian', 'lib_movies', '/tmp/mutation/movie.mp4', 'local', 1, 1024, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed mutation file: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_images (id, media_id, image_type, source, provider, path, remote_url, preferred, created_at)
		VALUES ('mutation_image', 'movie_meridian', 'poster', 'manual', 'upload', '/tmp/mutation/poster.jpg', '', 1, ?)`, now); err != nil {
		t.Fatalf("seed mutation image: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_lyrics (id, media_id, source, provider, format, language, path, text, synced, created_at)
		VALUES ('mutation_lyric', 'movie_meridian', 'manual', 'upload', 'txt', 'und', 'mutation.txt', 'line one', 0, ?)`, now); err != nil {
		t.Fatalf("seed mutation lyric: %v", err)
	}

	item := server.mediaMutationResponseContext(context.Background(), "", "movie_meridian", MediaItem{ID: "movie_meridian", Title: "Fallback"})
	if item.ID != "movie_meridian" || item.Title == "Fallback" {
		t.Fatalf("mutation response did not load list item: %#v", item)
	}
	if len(item.Streams) != 0 || len(item.MediaFiles) != 0 || len(item.MediaImages) != 0 || len(item.Lyrics) != 0 || len(item.Children) != 0 || len(item.RecommendationRows) != 0 {
		t.Fatalf("mutation response included detail hydration: streams=%d files=%d images=%d lyrics=%d children=%d recommendations=%d",
			len(item.Streams), len(item.MediaFiles), len(item.MediaImages), len(item.Lyrics), len(item.Children), len(item.RecommendationRows))
	}
}

func TestMetadataUpdateUsesBaseMediaPayload(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, display_title, source_url)
		VALUES ('metadata_stream', 'movie_meridian', 'subtitle', 'webvtt', 'Managed subtitle', '/api/media/movie_meridian/subtitles/metadata_stream')`); err != nil {
		t.Fatalf("seed metadata stream: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at)
		VALUES ('metadata_file', 'movie_meridian', 'lib_movies', '/tmp/metadata/movie.mp4', 'local', 1, 1024, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed metadata file: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_images (id, media_id, image_type, source, provider, path, remote_url, preferred, created_at)
		VALUES ('metadata_image', 'movie_meridian', 'poster', 'manual', 'upload', '/tmp/metadata/poster.jpg', '', 1, ?)`, now); err != nil {
		t.Fatalf("seed metadata image: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_lyrics (id, media_id, source, provider, format, language, path, text, synced, created_at)
		VALUES ('metadata_lyric', 'movie_meridian', 'manual', 'upload', 'txt', 'und', 'metadata.txt', 'line one', 0, ?)`, now); err != nil {
		t.Fatalf("seed metadata lyric: %v", err)
	}

	summary := "Updated by background metadata refresh."
	item, err := server.updateMediaForMetadata("", "movie_meridian", UpdateMediaRequest{Summary: &summary})
	if err != nil {
		t.Fatalf("updateMediaForMetadata: %v", err)
	}
	if item.ID != "movie_meridian" || item.Summary != summary {
		t.Fatalf("metadata update did not return updated base media item: %#v", item)
	}
	if len(item.Streams) != 0 || len(item.MediaFiles) != 0 || len(item.MediaImages) != 0 || len(item.Lyrics) != 0 || len(item.Children) != 0 || len(item.RecommendationRows) != 0 {
		t.Fatalf("metadata update included detail hydration: streams=%d files=%d images=%d lyrics=%d children=%d recommendations=%d",
			len(item.Streams), len(item.MediaFiles), len(item.MediaImages), len(item.Lyrics), len(item.Children), len(item.RecommendationRows))
	}
}

func TestMediaLyricLookupSeedKeepsTrackContextWithoutPlaybackHydration(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	item, err := server.getMediaLyricLookupSeedContext(context.Background(), "", "track_mara_01")
	if err != nil {
		t.Fatalf("getMediaLyricLookupSeedContext: %v", err)
	}
	artist := firstNonEmpty(item.TypedMetadata["trackArtist"], item.TypedMetadata["albumArtist"], item.GrandparentTitle, item.Studio)
	album := firstNonEmpty(item.TypedMetadata["albumTitle"], item.ParentTitle)
	if item.ID != "track_mara_01" || item.Type != "track" || artist == "" || album == "" || item.DurationSeconds == 0 {
		t.Fatalf("unexpected lyric lookup seed: %#v", item)
	}
	if len(item.Streams) != 0 || len(item.MediaFiles) != 0 || len(item.Children) != 0 || len(item.RecommendationRows) != 0 || len(item.MediaImages) != 0 {
		t.Fatalf("lyric seed included playback/detail hydration: streams=%d files=%d children=%d recommendations=%d images=%d",
			len(item.Streams), len(item.MediaFiles), len(item.Children), len(item.RecommendationRows), len(item.MediaImages))
	}
}

func TestMissingLyricsJobAvoidsFullDetailHydration(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "server.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "func (s *Server) fetchMissingLyricsForLibrary")
	if start < 0 {
		t.Fatalf("fetchMissingLyricsForLibrary not found")
	}
	end := strings.Index(body[start:], "\nfunc (s *Server) setJobProgress")
	if end < 0 {
		t.Fatalf("fetchMissingLyricsForLibrary end not found")
	}
	functionBody := body[start : start+end]
	if strings.Contains(functionBody, "getMediaDetailShell") || strings.Contains(functionBody, "getMediaDetail(") {
		t.Fatalf("missing lyrics job should use lyric lookup seeds, not full detail hydration")
	}
	if !strings.Contains(functionBody, "missingMediaLyricLookupSeedsContext") {
		t.Fatalf("missing lyrics job should use batched lyric lookup seeds")
	}
	if strings.Contains(functionBody, "queryMedia(") || strings.Contains(functionBody, "queryMediaContext(") {
		t.Fatalf("missing lyrics job should not use general media hydration for candidate discovery")
	}
	if strings.Contains(functionBody, "getMediaLyricLookupSeedContext") {
		t.Fatalf("missing lyrics job should not re-fetch each lyric lookup seed")
	}
}

func TestManagementJobRoutesAvoidFullDetailHydration(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "server.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	body := string(source)
	functionBody := func(signature string) string {
		t.Helper()
		start := strings.Index(body, signature)
		if start < 0 {
			t.Fatalf("management route %q not found", signature)
		}
		tail := body[start+len(signature):]
		end := strings.Index(tail, "\nfunc ")
		if end < 0 {
			t.Fatalf("management route %q end not found", signature)
		}
		return body[start : start+len(signature)+end]
	}
	assertAccessSummaryOnly := func(name, routeBody, required string) {
		t.Helper()
		for _, forbidden := range []string{"getMediaDetail(", "getMediaDetailShell", "getMediaDetailWithOptions", "cachedMediaDetail"} {
			if strings.Contains(routeBody, forbidden) {
				t.Fatalf("%s should validate with an access summary, not full detail hydration: found %q", name, forbidden)
			}
		}
		if !strings.Contains(routeBody, required) {
			t.Fatalf("%s should use the profile-aware access summary: missing %q", name, required)
		}
	}

	metadataRepairBody := functionBody("func (s *Server) handleMetadataRepair")
	assertAccessSummaryOnly("metadata repair", metadataRepairBody,
		"s.getMediaAccessSummaryContext(r.Context(), viewerProfileID(user), req.MediaID)")

	mediaRouteBody := functionBody("func (s *Server) handleMediaRoute")
	jobsStart := strings.Index(mediaRouteBody, `if len(parts) == 2 && parts[1] == "jobs" && r.Method == http.MethodPost {`)
	if jobsStart < 0 {
		t.Fatalf("media job route branch not found")
	}
	jobsEnd := strings.Index(mediaRouteBody[jobsStart:], "\n\twriteError(w, http.StatusNotFound")
	if jobsEnd < 0 {
		t.Fatalf("media job route branch end not found")
	}
	mediaJobsBody := mediaRouteBody[jobsStart : jobsStart+jobsEnd]
	assertAccessSummaryOnly("media jobs", mediaJobsBody,
		"s.getMediaAccessSummaryContext(r.Context(), viewerProfileID(user), mediaID)")
}

func TestInstantMixSeedAvoidsFullDetailHydration(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "server.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "func (s *Server) instantMixItemsContext")
	if start < 0 {
		t.Fatalf("instantMixItemsContext not found")
	}
	end := strings.Index(body[start:], "\nfunc (s *Server) instantMixGenreItems")
	if end < 0 {
		t.Fatalf("instantMixItemsContext end not found")
	}
	functionBody := body[start : start+end]
	if strings.Contains(functionBody, "getMediaDetailShell") || strings.Contains(functionBody, "getMediaDetailWithOptions") {
		t.Fatalf("instant mix seed lookup should use a list item, not full detail hydration")
	}
	if !strings.Contains(functionBody, "getMediaListItemContext") {
		t.Fatalf("instant mix seed lookup should use getMediaListItemContext")
	}
}

func TestMetadataMatchSearchAvoidsFullDetailHydration(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "metadata.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read metadata source: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "func (s *Server) searchMediaMatchCandidatesWithOptions")
	if start < 0 {
		t.Fatalf("searchMediaMatchCandidatesWithOptions not found")
	}
	end := strings.Index(body[start:], "\nfunc (s *Server) seriesTitleForTMDBSearch")
	if end < 0 {
		t.Fatalf("searchMediaMatchCandidatesWithOptions end not found")
	}
	functionBody := body[start : start+end]
	if strings.Contains(functionBody, "getMediaDetailShell") || strings.Contains(functionBody, "getMediaDetailWithOptions") {
		t.Fatalf("metadata match search should use base media and candidate rows, not full detail hydration")
	}
	if !strings.Contains(functionBody, "getMediaContext") || !strings.Contains(functionBody, "matchCandidatesForMediaContext") {
		t.Fatalf("metadata match search should use getMediaContext and matchCandidatesForMediaContext")
	}
}

func TestDetailRecommendationRowsUseListItemHydration(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "discovery.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read discovery source: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "func (s *Server) moreEpisodesFromShowContext")
	if start < 0 {
		t.Fatalf("detail recommendation helper start not found")
	}
	end := strings.Index(body[start:], "\nfunc (s *Server) importantRecommendationPeople")
	if end < 0 {
		t.Fatalf("detail recommendation helper end not found")
	}
	helperBody := body[start : start+end]
	if strings.Contains(helperBody, "queryMediaContext(ctx, userID") {
		t.Fatalf("detail recommendation row helpers should use list-item hydration instead of full media hydration")
	}
	if count := strings.Count(helperBody, "queryMediaListItemsContext(ctx, userID"); count < 8 {
		t.Fatalf("expected detail recommendation helpers to use list-item hydration, found %d uses", count)
	}
}

func TestActivePlaybackLookupUsesExistenceCheckAndActiveIndex(t *testing.T) {
	_, db, _ := newDiscoveryTestServer(t, config.Config{})
	rows, err := db.Query(`
		EXPLAIN QUERY PLAN
		SELECT 1
		FROM playback_sessions
		WHERE media_id = ? AND ended_at = '' AND state <> 'stopped' AND last_seen_at >= ?
		LIMIT 1`, "movie_meridian", time.Now().UTC().Add(-45*time.Second).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("explain active playback lookup: %v", err)
	}
	defer rows.Close()
	plan := []string{}
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan active playback plan: %v", err)
		}
		plan = append(plan, strings.ToLower(detail))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("active playback plan rows: %v", err)
	}
	normalizedPlan := strings.Join(plan, "\n")
	if !strings.Contains(normalizedPlan, "idx_playback_sessions_media_active") {
		t.Fatalf("active playback lookup plan = %s, expected media active index", normalizedPlan)
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "server.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "func (s *Server) hasActivePlaybackForMedia")
	if start < 0 {
		t.Fatalf("hasActivePlaybackForMedia not found")
	}
	end := strings.Index(body[start:], "\nfunc ")
	if end < 0 {
		t.Fatalf("hasActivePlaybackForMedia end not found")
	}
	functionBody := body[start : start+end]
	if strings.Contains(strings.ToUpper(functionBody), "COUNT(") {
		t.Fatalf("active playback lookup should use SELECT 1 LIMIT 1, not COUNT")
	}
	if !strings.Contains(functionBody, "LIMIT 1") {
		t.Fatalf("active playback lookup should stop at the first matching active session")
	}
}

func TestPlaybackSessionActiveHelperUsesExistenceCheck(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "server.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "func playbackSessionActiveForUser")
	if start < 0 {
		t.Fatalf("playbackSessionActiveForUser not found")
	}
	end := strings.Index(body[start:], "\nfunc ")
	if end < 0 {
		t.Fatalf("playbackSessionActiveForUser end not found")
	}
	functionBody := body[start : start+end]
	if strings.Contains(strings.ToUpper(functionBody), "COUNT(") {
		t.Fatalf("playbackSessionActiveForUser should use SELECT 1 LIMIT 1, not COUNT")
	}
	if !strings.Contains(functionBody, "SELECT 1") || !strings.Contains(functionBody, "LIMIT 1") {
		t.Fatalf("playbackSessionActiveForUser should stop at the first matching active session")
	}
}

func TestHomeNavigationRowsUseListItemHydration(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	appDir := filepath.Dir(filename)
	serverSource, err := os.ReadFile(filepath.Join(appDir, "server.go"))
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	discoverySource, err := os.ReadFile(filepath.Join(appDir, "discovery.go"))
	if err != nil {
		t.Fatalf("read discovery source: %v", err)
	}
	for _, check := range []struct {
		name   string
		source string
		end    string
	}{
		{"homeRowPageContext", string(serverSource), "\nfunc (s *Server) onDeckHomeItems"},
		{"onDeckHomeItemsContext", string(serverSource), "\nfunc removeListeningMediaFromMixedHomeRows"},
		{"libraryContinueRowContext", string(discoverySource), "\nfunc (s *Server) libraryRecentRowContext"},
	} {
		start := strings.Index(check.source, "func (s *Server) "+check.name)
		if start < 0 {
			t.Fatalf("%s not found", check.name)
		}
		end := strings.Index(check.source[start:], check.end)
		if end < 0 {
			t.Fatalf("%s end not found", check.name)
		}
		functionBody := check.source[start : start+end]
		if strings.Contains(functionBody, "queryMediaContext") {
			t.Fatalf("%s should use list-item hydration for navigation rows, not queryMediaContext", check.name)
		}
		if !strings.Contains(functionBody, "queryMediaListItemsContext") {
			t.Fatalf("%s should use queryMediaListItemsContext", check.name)
		}
	}
}

func TestHomeOnDeckNewestEpisodesUsesTypeAddedIndex(t *testing.T) {
	_, db, _ := newDiscoveryTestServer(t, config.Config{})
	rows, err := db.Query(`
		EXPLAIN QUERY PLAN
		SELECT m.id
		FROM media_items m
		WHERE m.type = 'episode'
		ORDER BY m.added_at DESC
		LIMIT ?`, homeRowItemLimit+1)
	if err != nil {
		t.Fatalf("explain newest on-deck query: %v", err)
	}
	defer rows.Close()
	plan := explainPlanDetails(t, rows)
	normalizedPlan := strings.Join(plan, "\n")
	if !strings.Contains(normalizedPlan, "idx_media_type_added") {
		t.Fatalf("newest on-deck plan = %s, expected media type/added index", normalizedPlan)
	}
	if strings.Contains(normalizedPlan, "use temp b-tree") {
		t.Fatalf("newest on-deck plan sorts with temp b-tree: %s", normalizedPlan)
	}
}

func TestHomeStateRowsUseUserStateIndexes(t *testing.T) {
	_, db, _ := newDiscoveryTestServer(t, config.Config{})
	checks := []struct {
		name      string
		query     string
		args      []any
		indexName string
	}{
		{
			name: "watchlist",
			query: `
				EXPLAIN QUERY PLAN
				SELECT m.id
				FROM media_items m
				LEFT JOIN user_media_state ums ON ums.media_id = m.id AND ums.profile_id = ?
				WHERE ums.watchlisted = 1
				ORDER BY ums.updated_at DESC
				LIMIT ?`,
			args:      []any{"usr_perf_home", homeRowItemLimit + 1},
			indexName: "idx_user_state_watchlist_updated",
		},
		{
			name: "favorites",
			query: `
				EXPLAIN QUERY PLAN
				SELECT m.id
				FROM media_items m
				LEFT JOIN user_media_state ums ON ums.media_id = m.id AND ums.profile_id = ?
				WHERE ums.favorite = 1
				ORDER BY ums.updated_at DESC, m.sort_title ASC
				LIMIT ?`,
			args:      []any{"usr_perf_home", homeRowItemLimit + 1},
			indexName: "idx_user_state_favorite_updated",
		},
		{
			name: "resume",
			query: `
				EXPLAIN QUERY PLAN
				SELECT m.id
				FROM media_items m
				LEFT JOIN user_media_state ums ON ums.media_id = m.id AND ums.profile_id = ?
				WHERE ums.progress_seconds > 0 AND ums.watched = 0
				ORDER BY COALESCE(ums.last_played_at, ums.updated_at) DESC
				LIMIT ?`,
			args:      []any{"usr_perf_home", homeRowItemLimit + 1},
			indexName: "idx_user_state_resume_recent",
		},
	}
	for _, check := range checks {
		rows, err := db.Query(check.query, check.args...)
		if err != nil {
			t.Fatalf("explain %s home state query: %v", check.name, err)
		}
		plan := explainPlanDetails(t, rows)
		_ = rows.Close()
		normalizedPlan := strings.Join(plan, "\n")
		if !strings.Contains(normalizedPlan, check.indexName) {
			t.Fatalf("%s home state plan = %s, expected %s", check.name, normalizedPlan, check.indexName)
		}
	}
}

func TestProgressWritesCoalesceRepeatedIdenticalUpdates(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	if err := server.setProgress(userID, "movie_meridian", 600, false); err != nil {
		t.Fatalf("set initial progress: %v", err)
	}
	stableUpdatedAt := "2026-06-08T12:00:00Z"
	stableLastPlayedAt := time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339)
	if _, err := db.Exec(`
		UPDATE user_media_state
		SET updated_at = ?, last_played_at = ?
		WHERE user_id = ? AND media_id = 'movie_meridian'`,
		stableUpdatedAt, stableLastPlayedAt, userID); err != nil {
		t.Fatalf("stabilize progress state: %v", err)
	}
	if err := server.setProgress(userID, "movie_meridian", 600, false); err != nil {
		t.Fatalf("set repeated progress: %v", err)
	}
	var updatedAt, lastPlayedAt string
	if err := db.QueryRow(`
		SELECT updated_at, last_played_at
		FROM user_media_state
		WHERE user_id = ? AND media_id = 'movie_meridian'`, userID).Scan(&updatedAt, &lastPlayedAt); err != nil {
		t.Fatalf("load repeated progress state: %v", err)
	}
	if updatedAt != stableUpdatedAt || lastPlayedAt != stableLastPlayedAt {
		t.Fatalf("identical progress write churned timestamps: updatedAt=%q lastPlayedAt=%q", updatedAt, lastPlayedAt)
	}

	staleUpdatedAt := "2026-06-08T12:01:00Z"
	staleLastPlayedAt := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`
		UPDATE user_media_state
		SET updated_at = ?, last_played_at = ?
		WHERE user_id = ? AND media_id = 'movie_meridian'`,
		staleUpdatedAt, staleLastPlayedAt, userID); err != nil {
		t.Fatalf("stabilize stale progress state: %v", err)
	}
	if err := server.setProgress(userID, "movie_meridian", 600, false); err != nil {
		t.Fatalf("refresh stale progress: %v", err)
	}
	if err := db.QueryRow(`
		SELECT updated_at, last_played_at
		FROM user_media_state
		WHERE user_id = ? AND media_id = 'movie_meridian'`, userID).Scan(&updatedAt, &lastPlayedAt); err != nil {
		t.Fatalf("load stale progress state: %v", err)
	}
	if updatedAt == staleUpdatedAt || lastPlayedAt == staleLastPlayedAt {
		t.Fatalf("stale progress heartbeat was not refreshed: updatedAt=%q lastPlayedAt=%q", updatedAt, lastPlayedAt)
	}
}

func TestMediaDetailChildrenAreBounded(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_detail_children', 'Detail Children', 'tv', 955, '/tmp/detail-children', '{}', ?)`, now); err != nil {
		t.Fatalf("insert detail children library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, tags_json, labels_json, added_at, typed_metadata_json)
		VALUES ('detail_huge_show', 'lib_detail_children', 'show', 'Huge Detail Show', 'Huge Detail Show', '[]', '[]', '[]', ?, '{}')`, now); err != nil {
		t.Fatalf("insert detail show: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin detail seed: %v", err)
	}
	stmt, err := tx.Prepare(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, genres_json, tags_json, labels_json, added_at, typed_metadata_json, index_number)
		VALUES (?, 'lib_detail_children', ?, ?, ?, ?, '[]', '[]', '[]', ?, '{}', ?)`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("prepare detail seed: %v", err)
	}
	for season := 1; season <= 6; season++ {
		seasonID := fmt.Sprintf("detail_huge_show_s%02d", season)
		seasonTitle := fmt.Sprintf("Season %02d", season)
		if _, err := stmt.Exec(seasonID, "detail_huge_show", "season", seasonTitle, seasonTitle, now, season); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			t.Fatalf("insert season %d: %v", season, err)
		}
		for episode := 1; episode <= maxDetailNestedChildrenPerGroup+5; episode++ {
			episodeID := fmt.Sprintf("%s_e%03d", seasonID, episode)
			episodeTitle := fmt.Sprintf("Episode %03d", episode)
			if _, err := stmt.Exec(episodeID, seasonID, "episode", episodeTitle, episodeTitle, now, episode); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				t.Fatalf("insert episode %d/%d: %v", season, episode, err)
			}
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatalf("close detail seed statement: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit detail seed: %v", err)
	}

	item, err := server.getMediaDetail("", "detail_huge_show")
	if err != nil {
		t.Fatalf("load huge show detail: %v", err)
	}
	if !item.ChildrenTruncated {
		t.Fatalf("expected detail children to be marked truncated")
	}
	if len(item.Children) != 6 {
		t.Fatalf("season count = %d, expected 6", len(item.Children))
	}
	nested := 0
	for _, season := range item.Children {
		if len(season.Children) > maxDetailNestedChildrenPerGroup {
			t.Fatalf("season %s children = %d, over per-group cap %d", season.ID, len(season.Children), maxDetailNestedChildrenPerGroup)
		}
		nested += len(season.Children)
	}
	if nested > maxDetailNestedChildrenTotal {
		t.Fatalf("nested detail children = %d, over global cap %d", nested, maxDetailNestedChildrenTotal)
	}
}

func TestMediaBulkStateRejectsInvisibleIDsBeforeWriting(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_bulk_visibility', 'Bulk Visibility', 'movie', 954, '/tmp/bulk-visibility', '{}', ?)`, now); err != nil {
		t.Fatalf("insert bulk visibility library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, max_content_rating, created_at, updated_at)
		VALUES ('usr_bulk_restricted', 'bulk-restricted', 'bulk-restricted@example.test', 'Bulk Restricted', 'hash', 'user', '{}', '{}', 'PG', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("insert restricted bulk user: %v", err)
	}
	var bulkProfileID string
	if err := db.QueryRow(`SELECT id FROM profiles WHERE account_id = 'usr_bulk_restricted' AND is_primary = 1`).Scan(&bulkProfileID); err != nil {
		t.Fatalf("load restricted bulk profile: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES ('usr_bulk_restricted', 'lib_bulk_visibility', ?)`, now); err != nil {
		t.Fatalf("grant bulk visibility library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, content_rating, genres_json, tags_json, labels_json, added_at, typed_metadata_json)
		VALUES
			('bulk_visible_movie', 'lib_bulk_visibility', 'movie', 'Bulk Visible', 'Bulk Visible', 'PG', '[]', '[]', '[]', ?, '{}'),
			('bulk_hidden_movie', 'lib_bulk_visibility', 'movie', 'Bulk Hidden', 'Bulk Hidden', 'R', '[]', '[]', '[]', ?, '{}')`,
		now, now); err != nil {
		t.Fatalf("insert bulk visibility media: %v", err)
	}

	body := strings.NewReader(`{"mediaIds":["bulk_visible_movie","bulk_hidden_movie"],"watchlisted":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/media/bulk/state", body)
	rec := httptest.NewRecorder()
	server.handleMediaBulkState(rec, req, User{ID: "usr_bulk_restricted", AccountID: "usr_bulk_restricted", ProfileID: bulkProfileID, ProfileIsPrimary: true})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bulk state status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stateRows int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM user_media_state
		WHERE user_id = 'usr_bulk_restricted'
			AND media_id IN ('bulk_visible_movie', 'bulk_hidden_movie')`).Scan(&stateRows); err != nil {
		t.Fatalf("count restricted bulk state rows: %v", err)
	}
	if stateRows != 0 {
		t.Fatalf("bulk state wrote %d rows before validating visibility", stateRows)
	}
}

func TestBulkWatchedExpandsSubtreesInOneBoundedWrite(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_bulk_watched_subtree', 'Bulk Watched Subtree', 'tv', 953, '/tmp/bulk-watched-subtree', '{}', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, added_at, duration_seconds, season_number, episode_number, index_number)
		VALUES
			('bulk_watch_show', 'lib_bulk_watched_subtree', NULL, 'show', 'Bulk Watch', 'Bulk Watch', ?, 0, 0, 0, 0),
			('bulk_watch_season', 'lib_bulk_watched_subtree', 'bulk_watch_show', 'season', 'Season 1', 'Season 1', ?, 0, 1, 0, 1),
			('bulk_watch_episode_1', 'lib_bulk_watched_subtree', 'bulk_watch_season', 'episode', 'Episode 1', 'Episode 1', ?, 1200, 1, 1, 1),
			('bulk_watch_episode_2', 'lib_bulk_watched_subtree', 'bulk_watch_season', 'episode', 'Episode 2', 'Episode 2', ?, 1200, 1, 2, 2)`,
		now, now, now, now); err != nil {
		t.Fatalf("insert media subtree: %v", err)
	}
	var ownerID string
	if err := db.QueryRow(`SELECT id FROM users WHERE role = 'owner' LIMIT 1`).Scan(&ownerID); err != nil {
		t.Fatalf("load owner: %v", err)
	}
	if err := server.bulkSetWatched(ownerID, []string{"bulk_watch_show"}, true); err != nil {
		t.Fatalf("bulk watched show: %v", err)
	}
	rows, err := db.Query(`
		SELECT media_id, watched
		FROM user_media_state
		WHERE user_id = ?
			AND media_id LIKE 'bulk_watch_%'
		ORDER BY media_id ASC`, ownerID)
	if err != nil {
		t.Fatalf("query watched rows: %v", err)
	}
	defer rows.Close()
	got := []string{}
	for rows.Next() {
		var mediaID string
		var watched int
		if err := rows.Scan(&mediaID, &watched); err != nil {
			t.Fatalf("scan watched row: %v", err)
		}
		got = append(got, fmt.Sprintf("%s=%d", mediaID, watched))
	}
	if strings.Join(got, ",") != "bulk_watch_episode_1=1,bulk_watch_episode_2=1" {
		t.Fatalf("bulk watched rows = %v, expected only playable descendants", got)
	}
}

func TestLibraryCategoryFacetsUseReadModelIndex(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_category_read_model', 'Category Read Model', 'music', 951, '/tmp/category-read-model', '{}', ?)`, now); err != nil {
		t.Fatalf("insert read model library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, studio, genres_json, added_at, typed_metadata_json)
		VALUES
			('category_read_model_track_1', 'lib_category_read_model', 'track', 'Indexed Track One', 'Indexed Track One', '', '["Indexed Genre"]', ?, '{"albumArtist":"Indexed Album Artist","label":"Indexed Label"}'),
			('category_read_model_track_2', 'lib_category_read_model', 'track', 'Indexed Track Two', 'Indexed Track Two', '', '[]', ?, '{"albumArtist":"Indexed Album Artist","label":"Indexed Label"}')`,
		now, now); err != nil {
		t.Fatalf("insert read model media: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM media_category_facets WHERE library_id = 'lib_category_read_model'`); err != nil {
		t.Fatalf("clear read model facets: %v", err)
	}

	categories, err := server.listLibraryCategories("", "lib_category_read_model")
	if err != nil {
		t.Fatalf("list read model categories: %v", err)
	}
	if libraryCategoriesContainFilter(categories, "albumArtist:Indexed Album Artist") || libraryCategoriesContainFilter(categories, "label:Indexed Label") {
		t.Fatalf("category listing used empty read model synchronously: %#v", categories)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.waitForLibraryReadModelRepair(ctx, "lib_category_read_model"); err != nil {
		t.Fatalf("wait for category read-model repair: %v", err)
	}
	categories, err = server.listLibraryCategories("", "lib_category_read_model")
	if err != nil {
		t.Fatalf("list repaired read model categories: %v", err)
	}
	if !libraryCategoriesContainFilterFold(categories, "genre:Indexed Genre") || !libraryCategoriesContainFilterFold(categories, "albumArtist:Indexed Album Artist") || !libraryCategoriesContainFilterFold(categories, "label:Indexed Label") {
		t.Fatalf("categories missing read model facets: %#v", categories)
	}
	albumArtist := libraryCategoryByFilterFold(categories, "albumArtist:Indexed Album Artist")
	if albumArtist.Count != 2 || albumArtist.Image == "" {
		t.Fatalf("album artist aggregate count/image = count:%d image:%q", albumArtist.Count, albumArtist.Image)
	}

	var stored int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_category_facets WHERE library_id = 'lib_category_read_model'`).Scan(&stored); err != nil {
		t.Fatalf("count stored read model facets: %v", err)
	}
	if stored == 0 {
		t.Fatalf("expected background repair to rebuild facet read model")
	}
	var aggregateCount int
	var aggregateImage string
	if err := db.QueryRow(`
		SELECT count, representative_image
		FROM library_category_counts
		WHERE library_id = 'lib_category_read_model' AND lower(filter) = 'albumartist:indexed album artist'`).Scan(&aggregateCount, &aggregateImage); err != nil {
		t.Fatalf("load category aggregate read model: %v", err)
	}
	if aggregateCount != 2 || aggregateImage == "" {
		t.Fatalf("category aggregate read model count/image = %d/%q, expected 2 with image", aggregateCount, aggregateImage)
	}
	if _, err := db.Exec(`DELETE FROM media_category_facets WHERE library_id = 'lib_category_read_model'`); err != nil {
		t.Fatalf("clear facet rows after aggregate read model build: %v", err)
	}
	aggregates, err := server.queryLibraryCategoryFacetAggregates("lib_category_read_model", "")
	if err != nil {
		t.Fatalf("query category aggregate read model: %v", err)
	}
	if aggregate := aggregates["albumartist:indexed album artist"]; aggregate.count != 2 || aggregate.image == "" {
		t.Fatalf("aggregate read model fallback = %#v, expected count 2 with image", aggregate)
	}

	rows, err := db.Query(`
		EXPLAIN QUERY PLAN
		SELECT filter
		FROM library_category_counts
		WHERE library_id = ? AND count > 0
		ORDER BY count DESC, filter ASC
		LIMIT ?`, "lib_category_read_model", maxCustomCategoryFacetsTotal*2)
	if err != nil {
		t.Fatalf("explain facet query: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan query plan: %v", err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read query plan: %v", err)
	}
	if !strings.Contains(plan.String(), "idx_library_category_counts_library") {
		t.Fatalf("facet count query did not use read model index:\n%s", plan.String())
	}
	aggregateQuery, aggregateArgs, ok := categoryBlueprintAggregateQuery("lib_category_read_model", "music", "", "genre:Indexed Genre", "", nil, true)
	if !ok {
		t.Fatalf("build category aggregate query")
	}
	aggregatePlan := explainQueryPlan(t, server, aggregateQuery, aggregateArgs...)
	if !strings.Contains(strings.Join(aggregatePlan, "\n"), "idx_media_category_facets_library") {
		t.Fatalf("category aggregate query did not start from facet read-model index:\n%s", strings.Join(aggregatePlan, "\n"))
	}
}

func TestLibraryCategoryFacetsAreServedFromBoundedCountReadModel(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_category_count_facets', 'Category Count Facets', 'music', 954, '/tmp/category-count-facets', '{}', ?)`, now); err != nil {
		t.Fatalf("insert count facet library: %v", err)
	}
	for i := 0; i < customCategoryFacetLimits["albumartist"]+12; i++ {
		filter := fmt.Sprintf("albumartist:count artist %02d", i)
		if _, err := db.Exec(`
			INSERT INTO library_category_counts (library_id, filter, count, representative_media_id, representative_image, updated_at)
			VALUES ('lib_category_count_facets', ?, ?, ?, ?, ?)`,
			filter, 1000-i, fmt.Sprintf("count_artist_%02d", i), fmt.Sprintf("/images/count-artist-%02d.jpg", i), now); err != nil {
			t.Fatalf("insert album artist count %d: %v", i, err)
		}
	}
	for i := 0; i < 8; i++ {
		filter := fmt.Sprintf("label:count label %02d", i)
		if _, err := db.Exec(`
			INSERT INTO library_category_counts (library_id, filter, count, representative_media_id, representative_image, updated_at)
			VALUES ('lib_category_count_facets', ?, ?, ?, ?, ?)`,
			filter, 500-i, fmt.Sprintf("count_label_%02d", i), fmt.Sprintf("/images/count-label-%02d.jpg", i), now); err != nil {
			t.Fatalf("insert label count %d: %v", i, err)
		}
	}
	if _, err := db.Exec(`DELETE FROM media_category_facets WHERE library_id = 'lib_category_count_facets'`); err != nil {
		t.Fatalf("clear raw category facets: %v", err)
	}

	facets, err := server.queryLibraryCategoryFacets("lib_category_count_facets")
	if err != nil {
		t.Fatalf("query bounded count facets: %v", err)
	}
	albumArtistCount := 0
	labelCount := 0
	for _, facet := range facets {
		switch facet.facetType {
		case "albumArtist":
			albumArtistCount++
		case "label":
			labelCount++
		}
	}
	if albumArtistCount != customCategoryFacetLimits["albumartist"] {
		t.Fatalf("album artist facets = %d, expected cap %d", albumArtistCount, customCategoryFacetLimits["albumartist"])
	}
	if labelCount != 8 {
		t.Fatalf("label facets = %d, expected 8", labelCount)
	}

	categories, err := server.listLibraryCategories("", "lib_category_count_facets")
	if err != nil {
		t.Fatalf("list bounded count categories: %v", err)
	}
	if !libraryCategoriesContainFilterFold(categories, "albumArtist:count artist 00") {
		t.Fatalf("categories did not include read-model album artist: %#v", categories)
	}
	if libraryCategoriesContainFilterFold(categories, "albumArtist:count artist 50") {
		t.Fatalf("categories included album artist beyond server cap: %#v", categories)
	}
	label := libraryCategoryByFilterFold(categories, "label:count label 00")
	if label.Count != 500 || label.Image == "" {
		t.Fatalf("label category aggregate = count:%d image:%q, expected read-model values", label.Count, label.Image)
	}
}

func TestLibrarySourceGroupsUseReadModel(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_source_read_model', 'Source Read Model', 'movie', 953, '/tmp/source-read-model', '{}', ?)`, now); err != nil {
		t.Fatalf("insert source read model library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO library_source_groups (library_id, kind, path, label, source_type, filter, item_count, file_count, missing_file_count, size_bytes, updated_at)
		VALUES ('lib_source_read_model', 'remote', 'https://cdn.example.test', 'Remote: cdn.example.test', 'remote', 'sourcePath:https://cdn.example.test', 123, 456, 7, 890, ?)`, now); err != nil {
		t.Fatalf("insert source read model: %v", err)
	}

	groups, err := server.listLibrarySourceGroups("lib_source_read_model")
	if err != nil {
		t.Fatalf("list source groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("source groups = %#v, expected one read-model group", groups)
	}
	group := groups[0]
	if group.Kind != "remote" || group.Path != "https://cdn.example.test" || group.ItemCount != 123 || group.FileCount != 456 || group.MissingFileCount != 7 || group.SizeBytes != 890 {
		t.Fatalf("unexpected source read-model group: %#v", group)
	}

	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_source_repair_queue', 'Source Repair Queue', 'movie', 954, '/tmp/source-repair-queue', '{}', ?)`, now); err != nil {
		t.Fatalf("insert source repair library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at)
		VALUES ('source_repair_movie', 'lib_source_repair_queue', 'movie', 'Repair Movie', 'Repair Movie', ?)`, now); err != nil {
		t.Fatalf("insert source repair media: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at)
		VALUES ('source_repair_file', 'source_repair_movie', 'lib_source_repair_queue', '/media/movies/Repair Movie.mkv', 'local', 1, 42, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source repair file: %v", err)
	}
	groups, err = server.listLibrarySourceGroups("lib_source_repair_queue")
	if err != nil {
		t.Fatalf("list empty source read model: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("source groups used foreground repair: %#v", groups)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.waitForLibraryReadModelRepair(ctx, "lib_source_repair_queue"); err != nil {
		t.Fatalf("wait for source read-model repair: %v", err)
	}
	groups, err = server.listLibrarySourceGroups("lib_source_repair_queue")
	if err != nil {
		t.Fatalf("list repaired source read model: %v", err)
	}
	if len(groups) != 1 || groups[0].ItemCount != 1 || groups[0].FileCount != 1 || groups[0].SizeBytes != 42 {
		t.Fatalf("repaired source groups = %#v, expected one computed read-model group", groups)
	}
}

func TestLibrarySourceGroupPaginationAvoidsExactCount(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "server.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "func (s *Server) listLibrarySourceGroupsPage")
	if start < 0 {
		t.Fatalf("listLibrarySourceGroupsPage not found")
	}
	end := strings.Index(body[start:], "\nfunc (s *Server) queryLibrarySourceGroupReadModel")
	if end < 0 {
		t.Fatalf("listLibrarySourceGroupsPage end not found")
	}
	functionBody := body[start : start+end]
	if strings.Contains(functionBody, "countLibrarySourceGroupReadModel") || strings.Contains(strings.ToUpper(functionBody), "COUNT(*)") {
		t.Fatalf("library source group pagination should use limit+1 conservative totals, not foreground exact counts")
	}
	if !strings.Contains(functionBody, "boolInt(hasMore)") {
		t.Fatalf("library source group pagination should expose conservative has-more totals")
	}

	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_source_page_no_count', 'Source Page No Count', 'movie', 956, '/tmp/source-page-no-count', '{}', ?)`, now); err != nil {
		t.Fatalf("insert source page library: %v", err)
	}
	for index := 0; index < 3; index++ {
		if _, err := db.Exec(`
			INSERT INTO library_source_groups (library_id, kind, path, label, source_type, filter, item_count, file_count, missing_file_count, size_bytes, updated_at)
			VALUES ('lib_source_page_no_count', 'local', ?, ?, 'local', ?, ?, 1, 0, 42, ?)`,
			fmt.Sprintf("/media/source-%d", index), fmt.Sprintf("Source %d", index), fmt.Sprintf("sourcePath:/media/source-%d", index), 10-index, now); err != nil {
			t.Fatalf("insert source page group %d: %v", index, err)
		}
	}
	groups, total, err := server.listLibrarySourceGroupsPage("lib_source_page_no_count", 2, 0)
	if err != nil {
		t.Fatalf("list source page: %v", err)
	}
	if len(groups) != 2 || total != 3 {
		t.Fatalf("source group page len=%d total=%d, expected conservative total 3", len(groups), total)
	}
	groups, total, err = server.listLibrarySourceGroupsPage("lib_source_page_no_count", 2, 2)
	if err != nil {
		t.Fatalf("list final source page: %v", err)
	}
	if len(groups) != 1 || total != 3 {
		t.Fatalf("final source group page len=%d total=%d, expected total 3", len(groups), total)
	}
}

func TestDashboardResponsesUseShortTTLCache(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var first DashboardResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard?mode=history&period=24h", nil, &first)
	if status != http.StatusOK {
		t.Fatalf("first dashboard status=%d body=%s", status, body)
	}
	var second DashboardResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard?mode=history&period=24h", nil, &second)
	if status != http.StatusOK {
		t.Fatalf("second dashboard status=%d body=%s", status, body)
	}
	if first.GeneratedAt == "" || first.GeneratedAt != second.GeneratedAt {
		t.Fatalf("dashboard generatedAt first=%q second=%q, expected cached response", first.GeneratedAt, second.GeneratedAt)
	}
}

func TestDashboardActivityReturnsCompactCachedSnapshot(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, state, bandwidth_mbps, decision)
		VALUES
			('play_activity_direct', ?, ?, 'movie_activity_direct', 'movie', 'Activity Direct', ?, ?, '', 'playing', 8.5, 'Direct'),
			('play_activity_transcode', ?, ?, 'movie_activity_transcode', 'movie', 'Activity Transcode', ?, ?, '', 'buffering', 4.4, 'Transcode'),
			('play_activity_stopped', ?, ?, 'movie_activity_stopped', 'movie', 'Activity Stopped', ?, ?, '', 'stopped', 20.0, 'Direct')`,
		userID, userID, now, now, userID, userID, now, now, userID, userID, now, now); err != nil {
		t.Fatalf("insert activity sessions: %v", err)
	}
	const gib = int64(1024 * 1024 * 1024)
	server.telemetryMu.Lock()
	server.telemetry = []systemTelemetryPoint{{
		At:                  time.Now().UTC(),
		ServerCPU:           12,
		SystemCPU:           42,
		ServerRAM:           8,
		SystemRAM:           60,
		SystemRAMUsedBytes:  6 * gib,
		SystemRAMFreeBytes:  4 * gib,
		SystemRAMTotalBytes: 10 * gib,
	}}
	server.telemetryMu.Unlock()

	var first ServerActivityResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard/activity", nil, &first)
	if status != http.StatusOK {
		t.Fatalf("activity status=%d body=%s", status, body)
	}
	if first.ServerName == "" || first.ActiveStreams != 2 || first.ActiveTranscodes != 0 {
		t.Fatalf("unexpected activity counters: %#v", first)
	}
	if first.CPUPercent != 42 || first.MemoryUsedBytes != 6*gib || first.MemoryFreeBytes != 4*gib || first.MemoryTotalBytes != 10*gib {
		t.Fatalf("unexpected activity resources: %#v", first)
	}
	if first.BandwidthMbps < 12.8 || first.BandwidthMbps > 13.0 || first.RefreshAfterMs != 1000 {
		t.Fatalf("unexpected activity bandwidth/refresh: %#v", first)
	}

	var second ServerActivityResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard/activity", nil, &second)
	if status != http.StatusOK {
		t.Fatalf("second activity status=%d body=%s", status, body)
	}
	if first.GeneratedAt == "" || first.GeneratedAt != second.GeneratedAt {
		t.Fatalf("activity generatedAt first=%q second=%q, expected one-second cache", first.GeneratedAt, second.GeneratedAt)
	}
}

func TestLiveDashboardSkipsHistoryAnalytics(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	startedAt := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	endedAt := time.Now().UTC().Add(-90 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, state)
		VALUES ('play_dashboard_history_only', ?, ?, 'movie_meridian', 'movie', 'Meridian', ?, ?, ?, 'stopped')`,
		userID, userID, startedAt, endedAt, endedAt); err != nil {
		t.Fatalf("insert dashboard history session: %v", err)
	}
	if _, err := server.refreshDashboardPlaybackRollupsContext(context.Background(), time.Now().UTC().Add(-24*time.Hour), 100); err != nil {
		t.Fatalf("refresh dashboard history rollups: %v", err)
	}

	var history DashboardResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard?mode=history&period=24h", nil, &history)
	if status != http.StatusOK {
		t.Fatalf("history dashboard status=%d body=%s", status, body)
	}
	if len(history.TopUsers) == 0 || len(history.PlayHistory) == 0 || len(history.TopPlayed) == 0 {
		t.Fatalf("history dashboard missing seeded analytics: %#v", history)
	}

	var live DashboardResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard?mode=live&period=5m", nil, &live)
	if status != http.StatusOK {
		t.Fatalf("live dashboard status=%d body=%s", status, body)
	}
	if len(live.TopUsers) != 0 || len(live.PlayHistory) != 0 || len(live.TopPlayed) != 0 {
		t.Fatalf("live dashboard should not include history analytics: %#v", live)
	}
}

func TestDashboardSectionsSkipUnrequestedWork(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	startedAt := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	endedAt := time.Now().UTC().Add(-90 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (
			id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, state,
			location, bandwidth_mbps, decision
		)
		VALUES ('play_dashboard_sections', ?, ?, 'movie_sections', 'movie', 'Sectional', ?, ?, ?, 'stopped', 'Remote', 12.4, 'Direct')`,
		userID, userID, startedAt, endedAt, endedAt); err != nil {
		t.Fatalf("insert dashboard history session: %v", err)
	}
	if _, err := server.refreshDashboardPlaybackRollupsContext(context.Background(), time.Now().UTC().Add(-24*time.Hour), 100); err != nil {
		t.Fatalf("refresh section dashboard rollups: %v", err)
	}

	var response DashboardResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard?mode=history&period=24h&sections=topUsers", nil, &response)
	if status != http.StatusOK {
		t.Fatalf("dashboard status=%d body=%s", status, body)
	}
	if len(response.TopUsers) == 0 {
		t.Fatalf("section-limited dashboard missing top users: %#v", response)
	}
	if len(response.PlayHistory) != 0 || len(response.TopPlayed) != 0 {
		t.Fatalf("section-limited dashboard included unrequested history analytics: %#v", response)
	}
	if len(response.Metrics) != 0 || len(response.Bandwidth) != 0 || len(response.NowPlaying) != 0 || len(response.Libraries) != 0 || len(response.Jobs) != 0 || len(response.Alerts) != 0 {
		t.Fatalf("section-limited dashboard included core overview work: %#v", response)
	}

	response = DashboardResponse{}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard?mode=history&period=24h&sections=bandwidth&userId="+url.QueryEscape(userID), nil, &response)
	if status != http.StatusOK {
		t.Fatalf("bandwidth dashboard status=%d body=%s", status, body)
	}
	if len(response.Bandwidth) == 0 {
		t.Fatalf("bandwidth-only dashboard missing samples: %#v", response)
	}
	remoteTotal := 0
	for _, sample := range response.Bandwidth {
		remoteTotal += sample.Remote
	}
	if remoteTotal == 0 {
		t.Fatalf("bandwidth-only dashboard did not apply rollup samples: %#v", response.Bandwidth)
	}
	if response.UserID != userID {
		t.Fatalf("bandwidth-only dashboard userId=%q, expected %q", response.UserID, userID)
	}
	if len(response.Metrics) != 0 || len(response.NowPlaying) != 0 || len(response.Libraries) != 0 || len(response.Jobs) != 0 || len(response.Alerts) != 0 || len(response.TopUsers) != 0 || len(response.PlayHistory) != 0 || len(response.TopPlayed) != 0 {
		t.Fatalf("bandwidth-only dashboard included unrequested work: %#v", response)
	}
}

func TestDashboardRollupRefreshRunsInBackgroundBatches(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC().Truncate(time.Second)
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin playback seed: %v", err)
	}
	stmt, err := tx.Prepare(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, state)
		VALUES (?, ?, ?, ?, 'movie', ?, ?, ?, ?, 'stopped')`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("prepare playback seed: %v", err)
	}
	for index := 0; index < 1200; index++ {
		startedAt := now.Add(-time.Duration(index+1) * time.Minute)
		endedAt := startedAt.Add(3 * time.Minute)
		if _, err := stmt.Exec(
			fmt.Sprintf("play_dashboard_overview_usage_%04d", index),
			userID,
			userID,
			fmt.Sprintf("movie_overview_usage_%02d", index%5),
			fmt.Sprintf("Overview Usage %02d", index%5),
			startedAt.Format(time.RFC3339),
			endedAt.Format(time.RFC3339),
			endedAt.Format(time.RFC3339),
		); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			t.Fatalf("insert playback seed %d: %v", index, err)
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatalf("close playback seed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit playback seed: %v", err)
	}

	server.runDashboardRollupRefresh(context.Background(), Job{
		ID:       "job_dashboard_rollup_refresh_test",
		Type:     "dashboard_rollup_refresh",
		Status:   "running",
		Metadata: map[string]string{"since": now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)},
	})
	var rollupCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dashboard_playback_rollups WHERE session_id LIKE 'play_dashboard_overview_usage_%'`).Scan(&rollupCount); err != nil {
		t.Fatalf("count dashboard rollups: %v", err)
	}
	if rollupCount != 1200 {
		t.Fatalf("background rollup refresh wrote %d rollups, expected all 1200 seeded sessions", rollupCount)
	}
	var usage DashboardOverviewUsageResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard/overview-usage?topUsersPeriod=7d&usageHistoryPeriod=30d&topPlayedPeriod=30d", nil, &usage)
	if status != http.StatusOK {
		t.Fatalf("overview usage status=%d body=%s", status, body)
	}
	if len(usage.TopUsers) == 0 || len(usage.PlayHistory) == 0 || len(usage.TopPlayed) == 0 {
		t.Fatalf("overview usage missing history sections: %#v", usage)
	}
	var cached DashboardOverviewUsageResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard/overview-usage?topUsersPeriod=7d&usageHistoryPeriod=30d&topPlayedPeriod=30d", nil, &cached)
	if status != http.StatusOK {
		t.Fatalf("cached overview usage status=%d body=%s", status, body)
	}
	if usage.GeneratedAt == "" || usage.GeneratedAt != cached.GeneratedAt {
		t.Fatalf("overview usage generatedAt first=%q cached=%q, expected cached response", usage.GeneratedAt, cached.GeneratedAt)
	}
}

func TestDashboardHistoryReadsDoNotBulkRefreshRollupsInline(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "server.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	body := string(source)
	for _, fn := range []string{"loadDashboardContext", "dashboardOverviewUsageContext"} {
		start := strings.Index(body, "func (s *Server) "+fn)
		if start < 0 {
			t.Fatalf("%s not found", fn)
		}
		end := strings.Index(body[start:], "\nfunc (s *Server)")
		if end < 0 {
			t.Fatalf("%s end not found", fn)
		}
		functionBody := body[start : start+end]
		if strings.Contains(functionBody, "refreshDashboardPlaybackRollupsContext") {
			t.Fatalf("%s should not bulk refresh dashboard rollups inline", fn)
		}
		if !strings.Contains(functionBody, "queueDashboardPlaybackRollupRefresh") {
			t.Fatalf("%s should queue dashboard rollup refresh work for the background worker", fn)
		}
	}
}

func TestDashboardHistoryAnalyticsUsePlaybackRollups(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	startedAt := time.Now().UTC().Add(-2 * time.Hour)
	endedAt := time.Now().UTC().Add(-90 * time.Minute)
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, state)
		VALUES ('play_dashboard_rollup_only', ?, ?, 'movie_meridian', 'movie', 'Meridian', ?, ?, ?, 'stopped')`,
		userID, userID, startedAt.Format(time.RFC3339), endedAt.Format(time.RFC3339), endedAt.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert dashboard rollup session: %v", err)
	}
	if _, err := server.refreshDashboardPlaybackRollupsContext(context.Background(), startedAt.Add(-time.Minute), 100); err != nil {
		t.Fatalf("refresh dashboard rollups: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM playback_sessions WHERE id = 'play_dashboard_rollup_only'`); err != nil {
		t.Fatalf("delete raw dashboard session: %v", err)
	}
	filters := dashboardFilters{Mode: "history", Period: "24h", Since: time.Now().UTC().Add(-24 * time.Hour)}
	topUsers, err := server.queryDashboardTopUsersContext(context.Background(), filters)
	if err != nil {
		t.Fatalf("query dashboard top users: %v", err)
	}
	if len(topUsers) == 0 || topUsers[0].UserID != userID || topUsers[0].DurationSeconds <= 0 {
		t.Fatalf("top users from rollups = %#v", topUsers)
	}
	history, err := server.queryDashboardPlayHistoryContext(context.Background(), filters, time.Now().UTC())
	if err != nil {
		t.Fatalf("query dashboard play history: %v", err)
	}
	if len(history) == 0 || !dashboardHistoryHasMovieSeconds(history) {
		t.Fatalf("play history from rollups = %#v", history)
	}
	topPlayed, err := server.queryDashboardTopPlayedContext(context.Background(), filters)
	if err != nil {
		t.Fatalf("query dashboard top played: %v", err)
	}
	if len(topPlayed) == 0 || len(topPlayed[0].Items) == 0 || topPlayed[0].Items[0].ID != "movie_meridian" {
		t.Fatalf("top played from rollups = %#v", topPlayed)
	}
}

func TestDashboardHistorySessionsUsePlaybackRollups(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	startedAt := time.Now().UTC().Add(-3 * time.Hour)
	endedAt := time.Now().UTC().Add(-150 * time.Minute)
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (
			id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, state,
			location, bandwidth_mbps, decision
		)
		VALUES ('play_dashboard_session_rollup_only', ?, ?, 'movie_meridian', 'movie', 'Meridian Session', ?, ?, ?, 'stopped', 'Remote', 5.5, 'Transcode')`,
		userID, userID, startedAt.Format(time.RFC3339), endedAt.Format(time.RFC3339), endedAt.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert dashboard session rollup seed: %v", err)
	}
	if _, err := server.refreshDashboardPlaybackRollupsContext(context.Background(), startedAt.Add(-time.Minute), 100); err != nil {
		t.Fatalf("refresh dashboard session rollup: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM playback_sessions WHERE id = 'play_dashboard_session_rollup_only'`); err != nil {
		t.Fatalf("delete raw dashboard session seed: %v", err)
	}
	sessions, err := server.loadDashboardPlaybackSessionsContext(context.Background(), User{ID: userID, Permissions: map[string]bool{"manageServer": true}}, dashboardFilters{
		Mode:   "history",
		Period: "24h",
		Since:  time.Now().UTC().Add(-24 * time.Hour),
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("load dashboard history sessions: %v", err)
	}
	if len(sessions) == 0 || sessions[0].ID != "play_dashboard_session_rollup_only" || sessions[0].Decision != "Transcode" || sessions[0].Media.ID != "movie_meridian" {
		t.Fatalf("dashboard history sessions from rollups = %#v", sessions)
	}
}

func TestDashboardBandwidthSamplesAreBucketedInSQL(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC().Truncate(time.Second)
	startedAt := now.Add(-40 * time.Minute).Format(time.RFC3339)
	lastSeenAt := now.Add(-20 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (
			id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, state,
			location, bandwidth_mbps, decision
		)
		VALUES
			('play_dashboard_bandwidth_local', ?, ?, 'movie_meridian', 'movie', 'Local Bandwidth', ?, ?, ?, 'stopped', 'LAN', 8.5, 'Direct'),
			('play_dashboard_bandwidth_remote', ?, ?, 'movie_meridian', 'movie', 'Remote Bandwidth', ?, ?, ?, 'stopped', 'Remote', 4.4, 'Transcode')`,
		userID, userID, startedAt, lastSeenAt, lastSeenAt,
		userID, userID, startedAt, lastSeenAt, lastSeenAt); err != nil {
		t.Fatalf("insert dashboard bandwidth sessions: %v", err)
	}
	if _, err := server.refreshDashboardPlaybackRollupsContext(context.Background(), now.Add(-1*time.Hour), 100); err != nil {
		t.Fatalf("refresh dashboard bandwidth rollups: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM playback_sessions WHERE id LIKE 'play_dashboard_bandwidth_%'`); err != nil {
		t.Fatalf("delete raw dashboard bandwidth sessions: %v", err)
	}
	samples, err := server.loadDashboardBandwidthSamplesContext(context.Background(), dashboardFilters{
		Mode:   "history",
		Period: "24h",
		Since:  now.Add(-1 * time.Hour),
	}, now)
	if err != nil {
		t.Fatalf("load dashboard bandwidth samples: %v", err)
	}
	if len(samples) != 30 {
		t.Fatalf("dashboard bandwidth samples = %d, expected 30", len(samples))
	}
	localTotal, remoteTotal, transcodeTotal := 0, 0, 0
	for _, sample := range samples {
		localTotal += sample.Local
		remoteTotal += sample.Remote
		transcodeTotal += sample.Transcode
	}
	if localTotal == 0 || remoteTotal == 0 || transcodeTotal == 0 {
		t.Fatalf("dashboard bandwidth aggregation missed sample data: local=%d remote=%d transcode=%d samples=%#v", localTotal, remoteTotal, transcodeTotal, samples)
	}
}

func TestDashboardBandwidthHistoryUsesRollupEndIndex(t *testing.T) {
	_, db, _ := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC()
	rows, err := db.Query(`
		EXPLAIN QUERY PLAN
		SELECT started_at, last_seen_at, ended_at, location, bandwidth_mbps, decision
		FROM dashboard_playback_rollups INDEXED BY idx_dashboard_playback_rollups_ended_started
		WHERE started_at <= ? AND ended_at >= ?`,
		now.Format(time.RFC3339), now.Add(-24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("explain dashboard bandwidth rollup query: %v", err)
	}
	defer rows.Close()
	plan := []string{}
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan dashboard bandwidth plan: %v", err)
		}
		plan = append(plan, strings.ToLower(detail))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("dashboard bandwidth plan rows: %v", err)
	}
	normalizedPlan := strings.Join(plan, "\n")
	if !strings.Contains(normalizedPlan, "idx_dashboard_playback_rollups_ended_started") {
		t.Fatalf("dashboard bandwidth history plan = %s, expected rollup ended/started index", normalizedPlan)
	}
}

func TestDashboardHistorySessionsUseRollupLastSeenIndex(t *testing.T) {
	_, db, _ := newDiscoveryTestServer(t, config.Config{})
	rows, err := db.Query(`
		EXPLAIN QUERY PLAN
		SELECT session_id, user_id, user_name
		FROM dashboard_playback_rollups
		WHERE started_at >= ?
		ORDER BY last_seen_at DESC, session_id DESC
		LIMIT 50`,
		time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("explain dashboard history sessions query: %v", err)
	}
	defer rows.Close()
	plan := []string{}
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan dashboard history sessions plan: %v", err)
		}
		plan = append(plan, strings.ToLower(detail))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("dashboard history sessions plan rows: %v", err)
	}
	normalizedPlan := strings.Join(plan, "\n")
	if !strings.Contains(normalizedPlan, "idx_dashboard_playback_rollups_last_seen") {
		t.Fatalf("dashboard history sessions plan = %s, expected rollup last-seen index", normalizedPlan)
	}
}

func TestMetadataRefreshJobEnrichmentBatchesDuplicateMedia(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	jobs := []Job{
		{ID: "job_meta_enrich_a", Type: "metadata_refresh", ResourceType: "media", ResourceID: "movie_meridian", Metadata: map[string]string{}},
		{ID: "job_meta_enrich_b", Type: "metadata_refresh", ResourceType: "media", ResourceID: "movie_meridian", Metadata: map[string]string{}},
		{ID: "job_scan_ignored", Type: "library_scan", ResourceType: "library", ResourceID: "lib_movies", Metadata: map[string]string{}},
	}
	server.enrichMetadataRefreshJobsContext(context.Background(), jobs)
	for _, index := range []int{0, 1} {
		if jobs[index].Metadata["mediaTitle"] == "" || jobs[index].Metadata["libraryId"] != "lib_movies" || jobs[index].Metadata["libraryName"] == "" {
			t.Fatalf("metadata refresh job %d was not enriched from batched lookup: %#v", index, jobs[index])
		}
	}
	if len(jobs[2].Metadata) != 0 {
		t.Fatalf("non-media metadata refresh job was enriched unexpectedly: %#v", jobs[2])
	}
}

func dashboardHistoryHasMovieSeconds(points []PlayHistoryPoint) bool {
	for _, point := range points {
		if point.MoviesSeconds > 0 {
			return true
		}
	}
	return false
}

func TestDashboardSessionsSectionSkipsCoreAggregates(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (
			id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, state,
			client_ip, device, app, location, progress, position_seconds, bandwidth_mbps,
			decision, video_decision, audio_decision, subtitle_decision, is_live
		)
		VALUES (
			'play_dashboard_sessions_only', ?, ?, 'movie_sessions_only', 'movie', 'Sessions Only',
			?, ?, '', 'playing', '127.0.0.1', 'Browser', 'Portico Web', 'LAN',
			45, 120, 18.5, 'direct', 'direct', 'direct', 'none', 1
		)`, userID, userID, now, now); err != nil {
		t.Fatalf("insert live dashboard session: %v", err)
	}

	var response DashboardResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard?mode=live&period=5m&sections=sessions", nil, &response)
	if status != http.StatusOK {
		t.Fatalf("dashboard status=%d body=%s", status, body)
	}
	if len(response.NowPlaying) != 1 || response.NowPlaying[0].ID != "play_dashboard_sessions_only" {
		t.Fatalf("sessions-only dashboard missing active playback session: %#v", response)
	}
	if len(response.Metrics) != 0 || len(response.Bandwidth) != 0 || len(response.Libraries) != 0 || len(response.Jobs) != 0 || len(response.Alerts) != 0 || len(response.Conversions) != 0 {
		t.Fatalf("sessions-only dashboard included core overview work: %#v", response)
	}
	if len(response.System.CPU) != 0 || len(response.System.RAM) != 0 || len(response.System.GPU) != 0 || response.System.GPUInfo.Available || response.System.GPUInfo.Provider != "" {
		t.Fatalf("sessions-only dashboard included system telemetry: %#v", response.System)
	}
	if len(response.TopUsers) != 0 || len(response.PlayHistory) != 0 || len(response.TopPlayed) != 0 {
		t.Fatalf("sessions-only dashboard included history analytics: %#v", response)
	}
}

func TestLibraryCategoryFacetAggregatesRespectRestrictedUsers(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	preferencesJSON, _ := marshalUserPreferencesWithPolicies(defaultUserPreferences(), UserAccessSchedule{}, UserTagPolicy{
		AllowedTags: []string{"kids"},
		BlockedTags: []string{"blocked"},
	}, UserDevicePolicy{Mode: "any"}, UserChannelPolicy{})
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_restricted_category_facets', 'Restricted Category Facets', 'music', 952, '/tmp/restricted-category-facets', '{}', ?)`, now); err != nil {
		t.Fatalf("insert restricted facet library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, max_content_rating, created_at, updated_at)
		VALUES ('usr_restricted_facets', 'restricted-facets', 'restricted-facets@example.test', 'Restricted Facets', 'hash', 'user', '{}', ?, 'PG', ?, ?)`,
		string(preferencesJSON), now, now); err != nil {
		t.Fatalf("insert restricted facet user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES ('usr_restricted_facets', 'lib_restricted_category_facets', ?)`, now); err != nil {
		t.Fatalf("grant restricted facet library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, content_rating, genres_json, tags_json, added_at, typed_metadata_json)
		VALUES
			('restricted_facet_allowed', 'lib_restricted_category_facets', 'track', 'Allowed Track', 'Allowed Track', 'PG', '[]', '["kids"]', ?, '{"albumArtist":"Shared Restricted Artist"}'),
			('restricted_facet_rating_blocked', 'lib_restricted_category_facets', 'track', 'Rating Blocked Track', 'Rating Blocked Track', 'R', '[]', '["kids"]', ?, '{"albumArtist":"Shared Restricted Artist"}'),
			('restricted_facet_tag_blocked', 'lib_restricted_category_facets', 'track', 'Tag Blocked Track', 'Tag Blocked Track', 'PG', '[]', '["kids","blocked"]', ?, '{"albumArtist":"Shared Restricted Artist"}')`,
		now, now, now); err != nil {
		t.Fatalf("insert restricted facet media: %v", err)
	}
	if err := server.rebuildLibraryCategoryFacets("lib_restricted_category_facets"); err != nil {
		t.Fatalf("rebuild restricted facets: %v", err)
	}

	aggregates, err := server.queryLibraryCategoryFacetAggregates("lib_restricted_category_facets", "usr_restricted_facets")
	if err != nil {
		t.Fatalf("query restricted facet aggregates: %v", err)
	}
	aggregate := aggregates["albumartist:shared restricted artist"]
	if aggregate.count != 1 {
		t.Fatalf("restricted aggregate count = %d, expected 1", aggregate.count)
	}
	if aggregate.image == "" {
		t.Fatalf("restricted aggregate image was empty after grouped read-model query")
	}
	restrictedPlan := explainQueryPlan(t, server, `
		SELECT f.facet_type, f.value, COUNT(1), MIN(m.sort_title || char(31) || m.id)
		FROM media_category_facets f
		JOIN media_items m ON m.id = f.media_id
		WHERE f.library_id = ?
			AND EXISTS (SELECT 1 FROM user_library_access ula WHERE ula.user_id = ? AND ula.library_id = m.library_id)
			AND (`+contentRatingRankSQL("m.content_rating")+`) BETWEEN 1 AND ?
			AND EXISTS (
				SELECT 1
				FROM media_category_facets access_term
				WHERE access_term.media_id = m.id
					AND access_term.library_id = m.library_id
					AND access_term.facet_type = ?
					AND access_term.sort_value IN (?)
			)
			AND NOT EXISTS (
				SELECT 1
				FROM media_category_facets access_term
				WHERE access_term.media_id = m.id
					AND access_term.library_id = m.library_id
					AND access_term.facet_type = ?
					AND access_term.sort_value IN (?)
			)
		GROUP BY f.facet_type, f.value`, "lib_restricted_category_facets", "usr_restricted_facets", contentRatingRank("PG"), "tag", "kids", "tag", "blocked")
	normalizedRestrictedPlan := strings.ToLower(strings.Join(restrictedPlan, "\n"))
	if !strings.Contains(normalizedRestrictedPlan, "idx_media_category_facets_library") {
		t.Fatalf("restricted category aggregate plan did not use facet read-model index:\n%s", strings.Join(restrictedPlan, "\n"))
	}
	if strings.Contains(normalizedRestrictedPlan, "json_each") {
		t.Fatalf("restricted category aggregate plan used JSON expansion:\n%s", strings.Join(restrictedPlan, "\n"))
	}

	categories, err := server.listLibraryCategories("usr_restricted_facets", "lib_restricted_category_facets")
	if err != nil {
		t.Fatalf("list restricted facet categories: %v", err)
	}
	albumArtist := libraryCategoryByFilterFold(categories, "albumArtist:Shared Restricted Artist")
	if albumArtist.Count != 1 {
		t.Fatalf("restricted album artist category count = %d, expected 1", albumArtist.Count)
	}
}

func TestRestrictedUserLibraryAndSearchPagesFilterBeforeLimit(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	preferencesJSON, _ := marshalUserPreferencesWithPolicies(defaultUserPreferences(), UserAccessSchedule{}, UserTagPolicy{
		AllowedTags:   []string{"kids"},
		BlockedLabels: []string{"private"},
	}, UserDevicePolicy{Mode: "any"}, UserChannelPolicy{})
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_visibility_page_guard', 'Visibility Page Guard', 'movie', 953, '/tmp/visibility-page-guard', '{}', ?)`, now); err != nil {
		t.Fatalf("insert visibility guard library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_visibility_page_guard', 'visibility-page-guard', 'visibility-page-guard@example.test', 'Visibility Page Guard', 'hash', 'user', '{}', ?, ?, ?)`,
		string(preferencesJSON), now, now); err != nil {
		t.Fatalf("insert visibility guard user: %v", err)
	}
	var visibilityProfileID string
	if err := db.QueryRow(`SELECT id FROM profiles WHERE account_id = 'usr_visibility_page_guard' AND is_primary = 1`).Scan(&visibilityProfileID); err != nil {
		t.Fatalf("load visibility guard profile: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES ('usr_visibility_page_guard', 'lib_visibility_page_guard', ?)`, now); err != nil {
		t.Fatalf("grant visibility guard library: %v", err)
	}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("visibility_blocked_%02d", i)
		title := fmt.Sprintf("Common Visibility A Blocked %02d", i)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, content_rating, genres_json, tags_json, labels_json, added_at, typed_metadata_json)
			VALUES (?, 'lib_visibility_page_guard', 'movie', ?, ?, 'PG', '[]', '["kids"]', '["private"]', ?, '{}')`,
			id, title, title, now); err != nil {
			t.Fatalf("insert blocked visibility row %d: %v", i, err)
		}
		if _, err := db.Exec(`INSERT INTO media_search (media_id, title, summary, genres) VALUES (?, ?, '', '')`, id, title); err != nil {
			t.Fatalf("insert blocked visibility search %d: %v", i, err)
		}
		if _, err := db.Exec(`
			INSERT INTO user_media_state (profile_id, user_id, media_id, watchlisted, updated_at)
			VALUES (?, 'usr_visibility_page_guard', ?, 1, ?)`, visibilityProfileID, id, now); err != nil {
			t.Fatalf("insert blocked visibility state %d: %v", i, err)
		}
	}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("visibility_allowed_%02d", i)
		title := fmt.Sprintf("Common Visibility B Allowed %02d", i)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, content_rating, genres_json, tags_json, labels_json, added_at, typed_metadata_json)
			VALUES (?, 'lib_visibility_page_guard', 'movie', ?, ?, 'PG', '[]', '["kids"]', '[]', ?, '{}')`,
			id, title, title, now); err != nil {
			t.Fatalf("insert allowed visibility row %d: %v", i, err)
		}
		if _, err := db.Exec(`INSERT INTO media_search (media_id, title, summary, genres) VALUES (?, ?, '', '')`, id, title); err != nil {
			t.Fatalf("insert allowed visibility search %d: %v", i, err)
		}
		if _, err := db.Exec(`
			INSERT INTO user_media_state (profile_id, user_id, media_id, watchlisted, updated_at)
			VALUES (?, 'usr_visibility_page_guard', ?, 1, ?)`, visibilityProfileID, id, now); err != nil {
			t.Fatalf("insert allowed visibility state %d: %v", i, err)
		}
	}
	if err := server.rebuildLibraryCategoryFacets("lib_visibility_page_guard"); err != nil {
		t.Fatalf("rebuild visibility facets: %v", err)
	}

	items, total, _, _, err := server.listLibraryItems(visibilityProfileID, "lib_visibility_page_guard", "title", "all", "asc", 5, 0)
	if err != nil {
		t.Fatalf("list restricted library page: %v", err)
	}
	if total != 5 || len(items) != 5 {
		t.Fatalf("restricted library page total=%d len=%d items=%#v, expected full visible page", total, len(items), items)
	}
	for _, item := range items {
		if !strings.HasPrefix(item.ID, "visibility_allowed_") {
			t.Fatalf("restricted library page included blocked item %#v", item)
		}
	}

	searchItems, err := server.searchMedia(visibilityProfileID, "Common Visibility", 5)
	if err != nil {
		t.Fatalf("search restricted page: %v", err)
	}
	if len(searchItems) != 5 {
		t.Fatalf("restricted search len=%d items=%#v, expected full visible page", len(searchItems), searchItems)
	}
	for _, item := range searchItems {
		if !strings.HasPrefix(item.ID, "visibility_allowed_") {
			t.Fatalf("restricted search included blocked item %#v", item)
		}
	}

	homeRow, err, ok := server.homeRowPageContext(context.Background(), User{ID: "usr_visibility_page_guard", AccountID: "usr_visibility_page_guard", ProfileID: visibilityProfileID, ProfileIsPrimary: true, Role: "user", Preferences: defaultUserPreferences()}, "watchlist", 5, 0)
	if !ok || err != nil {
		t.Fatalf("home watchlist ok=%v err=%v", ok, err)
	}
	if homeRow.Total != 5 || len(homeRow.Items) != 5 {
		t.Fatalf("restricted home row total=%d len=%d items=%#v, expected full visible page", homeRow.Total, len(homeRow.Items), homeRow.Items)
	}
	for _, item := range homeRow.Items {
		if !strings.HasPrefix(item.ID, "visibility_allowed_") {
			t.Fatalf("restricted home row included blocked item %#v", item)
		}
	}

	rawListItems, err := server.queryMediaListItemsContext(context.Background(), visibilityProfileID, `
		WHERE m.library_id = ?
		ORDER BY m.sort_title ASC
		LIMIT ?`, []any{"lib_visibility_page_guard", 1})
	if err != nil {
		t.Fatalf("raw restricted list query: %v", err)
	}
	if len(rawListItems) != 1 || rawListItems[0].ID != "visibility_allowed_00" {
		t.Fatalf("raw list helper should inject visibility before LIMIT, got %#v", rawListItems)
	}

	rawDetailItems, err := server.queryMediaContext(context.Background(), visibilityProfileID, `
		WHERE m.library_id = ?
		ORDER BY m.sort_title ASC
		LIMIT ?`, []any{"lib_visibility_page_guard", 1})
	if err != nil {
		t.Fatalf("raw restricted detail query: %v", err)
	}
	if len(rawDetailItems) != 1 || rawDetailItems[0].ID != "visibility_allowed_00" {
		t.Fatalf("raw detail helper should inject visibility before LIMIT, got %#v", rawDetailItems)
	}
}

func TestLibraryMetadataRefreshItemsAreBoundedAndFilterUnsupportedTypes(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-api-key"})
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_metadata_refresh_cap', 'Metadata Refresh Cap', 'movie', 951, '/tmp/metadata-refresh-cap', '{}', ?)`, nowText); err != nil {
		t.Fatalf("insert metadata refresh library: %v", err)
	}
	for i := 0; i < libraryMetadataRefreshMaxLimit+50; i++ {
		addedAt := now.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		movieID := fmt.Sprintf("metadata_refresh_movie_%03d", i)
		movieTitle := fmt.Sprintf("Metadata Refresh Movie %03d", i)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, typed_metadata_json)
			VALUES (?, 'lib_metadata_refresh_cap', 'movie', ?, ?, ?, '{}')`,
			movieID, movieTitle, movieTitle, addedAt); err != nil {
			t.Fatalf("insert metadata refresh movie %d: %v", i, err)
		}
		extraID := fmt.Sprintf("metadata_refresh_extra_%03d", i)
		extraTitle := fmt.Sprintf("Metadata Refresh Extra %03d", i)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, typed_metadata_json)
			VALUES (?, 'lib_metadata_refresh_cap', 'extra', ?, ?, ?, '{}')`,
			extraID, extraTitle, extraTitle, addedAt); err != nil {
			t.Fatalf("insert metadata refresh extra %d: %v", i, err)
		}
	}

	items, err := server.libraryMetadataRefreshItems(context.Background(), "lib_metadata_refresh_cap", map[string]string{"limit": "5000"})
	if err != nil {
		t.Fatalf("load metadata refresh items: %v", err)
	}
	if len(items) != libraryMetadataRefreshMaxLimit {
		t.Fatalf("metadata refresh items = %d, expected cap %d", len(items), libraryMetadataRefreshMaxLimit)
	}
	for _, item := range items {
		if item.Type != "movie" {
			t.Fatalf("metadata refresh included unsupported item %#v", item)
		}
	}
}

func TestLibraryMetadataRefreshItemsUseContinuationCursor(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-api-key"})
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_metadata_refresh_cursor', 'Metadata Refresh Cursor', 'movie', 952, '/tmp/metadata-refresh-cursor', '{}', ?)`, nowText); err != nil {
		t.Fatalf("insert metadata refresh cursor library: %v", err)
	}
	for i := 0; i < 6; i++ {
		addedAt := now.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		movieID := fmt.Sprintf("metadata_refresh_cursor_%02d", i)
		movieTitle := fmt.Sprintf("Metadata Refresh Cursor %02d", i)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, typed_metadata_json)
			VALUES (?, 'lib_metadata_refresh_cursor', 'movie', ?, ?, ?, '{}')`,
			movieID, movieTitle, movieTitle, addedAt); err != nil {
			t.Fatalf("insert metadata refresh cursor movie %d: %v", i, err)
		}
	}

	first, hasMore, err := server.libraryMetadataRefreshItemsPage(context.Background(), "lib_metadata_refresh_cursor", map[string]string{"limit": "3"})
	if err != nil {
		t.Fatalf("load first cursor page: %v", err)
	}
	if len(first) != 3 || !hasMore || first[0].ID != "metadata_refresh_cursor_00" || first[2].ID != "metadata_refresh_cursor_02" {
		t.Fatalf("unexpected first cursor page hasMore=%v items=%#v", hasMore, first)
	}
	next, hasMore, err := server.libraryMetadataRefreshItemsPage(context.Background(), "lib_metadata_refresh_cursor", map[string]string{
		"limit":         "3",
		"cursorAddedAt": first[2].AddedAt,
		"cursorId":      first[2].ID,
	})
	if err != nil {
		t.Fatalf("load second cursor page: %v", err)
	}
	if len(next) != 3 || hasMore || next[0].ID != "metadata_refresh_cursor_03" || next[2].ID != "metadata_refresh_cursor_05" {
		t.Fatalf("unexpected second cursor page hasMore=%v items=%#v", hasMore, next)
	}
	job, err := server.createJobForWithMetadata("metadata_refresh_library", "Refresh metadata cursor library.", "library", "lib_metadata_refresh_cursor", map[string]string{"limit": "3", "refreshDays": "14", "mediaIds": "ignored"})
	if err != nil || !server.claimJobForRun(job.ID) {
		t.Fatalf("create/claim metadata cursor parent: job=%#v err=%v", job, err)
	}
	library := Library{ID: "lib_metadata_refresh_cursor", Name: "Metadata Refresh Cursor"}
	continuation, err := server.queueLibraryMetadataRefreshContinuation(job, library, first[2])
	if err != nil {
		t.Fatalf("queue metadata continuation: %v", err)
	}
	if continuation.Metadata["cursorAddedAt"] != first[2].AddedAt || continuation.Metadata["cursorId"] != first[2].ID || continuation.Metadata["mediaIds"] != "" {
		t.Fatalf("continuation metadata = %#v", continuation.Metadata)
	}
}

func TestLibraryMetadataRefreshItemsRespectCancellation(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := server.libraryMetadataRefreshItems(ctx, "lib_movies", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("libraryMetadataRefreshItems error = %v, expected context.Canceled", err)
	}
	if _, err := server.libraryMetadataRefreshItems(ctx, "lib_movies", map[string]string{"mediaIds": "movie_meridian"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("targeted libraryMetadataRefreshItems error = %v, expected context.Canceled", err)
	}
}

func TestLibraryBrowseSkipsPerItemResumeChapterLookup(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	now := time.Now().UTC().Format(time.RFC3339)
	userID := adminUserID(t, db)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_list_resume_books', 'List Resume Books', 'audiobook', 950, '/tmp/list-resume-books', '{}', ?)`, now); err != nil {
		t.Fatalf("insert list resume library: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES (?, 'lib_list_resume_books', ?)`, userID, now); err != nil {
		t.Fatalf("grant list resume library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, duration_seconds, genres_json, added_at, typed_metadata_json)
		VALUES ('book_list_resume', 'lib_list_resume_books', 'audiobook', 'List Chaptered Book', 'List Chaptered Book', 7200, '[]', ?, '{"author":"Author One","narrator":"Narrator One"}')`, now); err != nil {
		t.Fatalf("insert list resume book: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_chapters (id, media_id, title, start_seconds, end_seconds, sort_order)
		VALUES
			('book_list_resume_chapter_1', 'book_list_resume', 'Opening', 0, 1800, 0),
			('book_list_resume_chapter_2', 'book_list_resume', 'The Middle', 1800, 3600, 1)`); err != nil {
		t.Fatalf("insert list resume chapters: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES (?, ?, 'book_list_resume', 0, 1900, ?, ?)`, userID, userID, now, now); err != nil {
		t.Fatalf("insert list resume progress: %v", err)
	}

	var response BrowseLibraryResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_list_resume_books/browse", BrowseLibraryRequest{
		Pivot: "books",
		Limit: 10,
	}, &response)
	if status != http.StatusOK {
		t.Fatalf("library browse status=%d body=%s", status, body)
	}
	if len(response.Items) != 1 {
		t.Fatalf("library browse items = %#v", response.Items)
	}
	item := response.Items[0]
	if item.UserState.ProgressSeconds != 1900 {
		t.Fatalf("browse card progress = %d, expected 1900", item.UserState.ProgressSeconds)
	}
	if strings.Contains(body, `"resume"`) {
		t.Fatalf("browse card unexpectedly included detail-only resume chapter info: %s", body)
	}
}

func TestSmartPlaylistItemsSkipPerItemResumeChapterLookup(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	userID := adminUserID(t, db)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_smart_resume_books', 'Smart Resume Books', 'audiobook', 951, '/tmp/smart-resume-books', '{}', ?)`, now); err != nil {
		t.Fatalf("insert smart resume library: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES (?, 'lib_smart_resume_books', ?)`, userID, now); err != nil {
		t.Fatalf("grant smart resume library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, duration_seconds, genres_json, added_at, typed_metadata_json)
		VALUES ('book_smart_resume', 'lib_smart_resume_books', 'audiobook', 'Smart Chaptered Book', 'Smart Chaptered Book', 7200, '[]', ?, '{"author":"Author One","narrator":"Narrator One"}')`, now); err != nil {
		t.Fatalf("insert smart resume book: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_chapters (id, media_id, title, start_seconds, end_seconds, sort_order)
		VALUES
			('book_smart_resume_chapter_1', 'book_smart_resume', 'Opening', 0, 1800, 0),
			('book_smart_resume_chapter_2', 'book_smart_resume', 'The Middle', 1800, 3600, 1)`); err != nil {
		t.Fatalf("insert smart resume chapters: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES (?, ?, 'book_smart_resume', 0, 1900, ?, ?)`, userID, userID, now, now); err != nil {
		t.Fatalf("insert smart resume progress: %v", err)
	}

	user, err := server.getUser(userID)
	if err != nil {
		t.Fatalf("load smart resume user: %v", err)
	}
	items, err := server.smartPlaylistItemsWithLimit(user, SmartFilter{
		Enabled:   true,
		LibraryID: "lib_smart_resume_books",
		Type:      "audiobook",
		Sort:      "title",
		Limit:     10,
	}, 10)
	if err != nil {
		t.Fatalf("smart playlist items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("smart playlist items = %#v", items)
	}
	item := items[0]
	if item.State.ProgressSeconds != 1900 {
		t.Fatalf("smart playlist item progress = %d, expected 1900", item.State.ProgressSeconds)
	}
	if item.State.Resume != nil {
		t.Fatalf("smart playlist item unexpectedly included resume chapter info: %#v", item.State.Resume)
	}
}

func TestSmartPlaylistResultsUseShortTTLCache(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	filter := SmartFilter{Enabled: true, LibraryID: "lib_movies", Type: "movie", Sort: "title", Limit: 10}
	first, err := server.smartPlaylistItemsWithLimit(user, filter, 10)
	if err != nil {
		t.Fatalf("first smart playlist load: %v", err)
	}
	if len(first) == 0 {
		t.Fatalf("expected seeded movie smart playlist items")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, content_rating, genres_json, tags_json, labels_json, added_at)
		VALUES ('movie_smart_cache_new', 'lib_movies', 'movie', 'AAA Smart Cache New', 'AAA Smart Cache New', 'PG', '[]', '[]', '[]', ?)`, now); err != nil {
		t.Fatalf("insert smart cache movie: %v", err)
	}
	cached, err := server.smartPlaylistItemsWithLimit(user, filter, 10)
	if err != nil {
		t.Fatalf("cached smart playlist load: %v", err)
	}
	if mediaIDsContain(cached, "movie_smart_cache_new") {
		t.Fatalf("smart playlist cache unexpectedly included new direct DB row: %#v", cached)
	}
	server.invalidateSmartPlaylistCache()
	refreshed, err := server.smartPlaylistItemsWithLimit(user, filter, 10)
	if err != nil {
		t.Fatalf("refreshed smart playlist load: %v", err)
	}
	if !mediaIDsContain(refreshed, "movie_smart_cache_new") {
		t.Fatalf("smart playlist cache invalidation did not refresh results: %#v", refreshed)
	}
}

func libraryCategoryByFilter(categories []LibraryCategory, filter string) LibraryCategory {
	for _, category := range categories {
		if category.Filter == filter {
			return category
		}
	}
	return LibraryCategory{}
}

func libraryCategoryByFilterFold(categories []LibraryCategory, filter string) LibraryCategory {
	for _, category := range categories {
		if strings.EqualFold(category.Filter, filter) {
			return category
		}
	}
	return LibraryCategory{}
}

func libraryCategoriesContainFilterFold(categories []LibraryCategory, filter string) bool {
	return libraryCategoryByFilterFold(categories, filter).Filter != ""
}

func TestLargeLibraryEndpointPerformanceBudgets(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	tier := currentPerformanceTestTier()
	seedPerformanceLibrary(t, db, "lib_perf_movies", tier.catalogItems)
	if err := server.rebuildLibraryCategoryFacets("lib_perf_movies"); err != nil {
		t.Fatalf("rebuild performance category facets: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	cases := []struct {
		name        string
		path        string
		maxP95      time.Duration
		maxBytes    int
		minContains string
	}{
		{
			name:        "home",
			path:        "/api/home",
			maxP95:      1500 * time.Millisecond,
			maxBytes:    700_000,
			minContains: "Recently Added in Performance Movies",
		},
		{
			name:        "library first page",
			path:        "/api/libraries/lib_perf_movies/browse",
			maxP95:      900 * time.Millisecond,
			maxBytes:    350_000,
			minContains: "Performance Movie",
		},
		{
			name:        "categories",
			path:        "/api/libraries/lib_perf_movies/categories",
			maxP95:      1200 * time.Millisecond,
			maxBytes:    90_000,
			minContains: "Perf Genre",
		},
		{
			name:        "search",
			path:        "/api/search?q=BudgetSignal",
			maxP95:      900 * time.Millisecond,
			maxBytes:    350_000,
			minContains: "Performance Movie",
		},
		{
			name:        "dashboard",
			path:        "/api/dashboard?mode=live&period=5m",
			maxP95:      900 * time.Millisecond,
			maxBytes:    600_000,
			minContains: "Performance Movies",
		},
		{
			name:     "settings",
			path:     "/api/settings",
			maxP95:   600 * time.Millisecond,
			maxBytes: 300_000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			readsBefore := server.sqliteDiagnostics().ReadOperations
			var durations []time.Duration
			var body []byte
			for i := 0; i < 5; i++ {
				var status int
				var responseBody []byte
				var elapsed time.Duration
				if tc.name == "search" {
					sample := timedJSONRequest(client, http.MethodPost, serverURL+"/api/search", SearchRequest{Query: "BudgetSignal"}, nil)
					if sample.err != nil {
						t.Fatalf("search request failed: %v", sample.err)
					}
					status, responseBody, elapsed = sample.status, []byte(sample.body), sample.elapsed
				} else if tc.name == "library first page" {
					sample := timedJSONRequest(client, http.MethodPost, serverURL+tc.path, BrowseLibraryRequest{
						Pivot: "movies",
						Sort:  []BrowseSort{{Field: "dateAdded", Direction: "desc"}},
						Limit: 50,
					}, nil)
					if sample.err != nil {
						t.Fatalf("library browse request failed: %v", sample.err)
					}
					status, responseBody, elapsed = sample.status, []byte(sample.body), sample.elapsed
				} else {
					status, responseBody, elapsed = timedGET(t, client, serverURL+tc.path)
				}
				if status != http.StatusOK {
					t.Fatalf("%s status = %d, body: %s", tc.name, status, responseBody)
				}
				durations = append(durations, elapsed)
				body = responseBody
			}
			if len(body) > tc.maxBytes {
				t.Fatalf("%s response size = %d bytes, budget = %d", tc.name, len(body), tc.maxBytes)
			}
			if tc.minContains != "" && !strings.Contains(string(body), tc.minContains) {
				t.Fatalf("%s response did not contain %q", tc.name, tc.minContains)
			}
			sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
			p95 := durations[(len(durations)*95+99)/100-1]
			if p95 > tc.maxP95 {
				t.Fatalf("%s p95 = %s, budget = %s, samples = %v", tc.name, p95, tc.maxP95, durations)
			}
			reads := server.sqliteDiagnostics().ReadOperations - readsBefore
			t.Logf("%s p95=%s bytes=%d sqlite_reads=%d reads_per_request=%.1f samples=%v", tc.name, p95, len(body), reads, float64(reads)/float64(len(durations)), durations)
			if tc.name == "dashboard" {
				var dashboard DashboardResponse
				if err := json.Unmarshal(body, &dashboard); err != nil {
					t.Fatalf("decode dashboard guardrail response: %v", err)
				}
				for name, count := range map[string]int{
					"bandwidth": len(dashboard.Bandwidth),
					"cpu":       len(dashboard.System.CPU),
					"ram":       len(dashboard.System.RAM),
					"gpu":       len(dashboard.System.GPU),
					"diskIo":    len(dashboard.System.DiskIO),
				} {
					if count != dashboardLiveBuckets {
						t.Fatalf("dashboard %s samples = %d, want fixed ceiling %d", name, count, dashboardLiveBuckets)
					}
				}
				if len(dashboard.Jobs) > 80 || len(dashboard.NowPlaying) > 50 {
					t.Fatalf("dashboard bounded lists jobs=%d nowPlaying=%d", len(dashboard.Jobs), len(dashboard.NowPlaying))
				}

				transport := &http.Transport{DisableCompression: true}
				rawClient := &http.Client{Jar: jar, Transport: transport}
				defer transport.CloseIdleConnections()
				compressedDurations := make([]time.Duration, 0, 5)
				wireBytes := 0
				for index := 0; index < 5; index++ {
					request, err := http.NewRequest(http.MethodGet, serverURL+tc.path, nil)
					if err != nil {
						t.Fatal(err)
					}
					request.Header.Set("Accept-Encoding", "gzip")
					startedAt := time.Now()
					response, err := rawClient.Do(request)
					if err != nil {
						t.Fatalf("compressed dashboard request: %v", err)
					}
					wireBody, readErr := io.ReadAll(response.Body)
					closeErr := response.Body.Close()
					if readErr != nil || closeErr != nil {
						t.Fatalf("read compressed dashboard: read=%v close=%v", readErr, closeErr)
					}
					if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "gzip" {
						t.Fatalf("compressed dashboard status=%d encoding=%q", response.StatusCode, response.Header.Get("Content-Encoding"))
					}
					reader, err := gzip.NewReader(bytes.NewReader(wireBody))
					if err != nil {
						t.Fatal(err)
					}
					decoded, err := io.ReadAll(reader)
					if err != nil {
						t.Fatal(err)
					}
					if err := reader.Close(); err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(decoded), tc.minContains) {
						t.Fatal("compressed dashboard did not decode to expected response")
					}
					compressedDurations = append(compressedDurations, time.Since(startedAt))
					wireBytes = len(wireBody)
				}
				sort.Slice(compressedDurations, func(i, j int) bool { return compressedDurations[i] < compressedDurations[j] })
				compressedP95 := compressedDurations[(len(compressedDurations)*95+99)/100-1]
				t.Logf("dashboard gzip wire_bytes=%d ratio=%.3f compressed_p95=%s samples=%v", wireBytes, float64(wireBytes)/float64(len(body)), compressedP95, compressedDurations)
			}
		})
	}
}

func TestMixedUserBrowsingLoadSmoke(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	var serverLog bytes.Buffer
	server.log = slog.New(slog.NewTextHandler(&serverLog, nil))
	tier := currentPerformanceTestTier()
	seedPerformanceLibrary(t, db, "lib_load_movies", tier.catalogItems)
	seedExactPlaybackFactsForFixture(t, server, "perf_movie_0001")
	if err := server.rebuildLibraryCategoryFacets("lib_load_movies"); err != nil {
		t.Fatalf("rebuild mixed-load category facets: %v", err)
	}

	clients := performanceViewerClients(t, db, serverURL, tier)

	paths := []string{
		"/api/home",
		"/api/libraries/lib_load_movies/browse?sort=dateAdded",
		"/api/libraries/lib_load_movies/browse?sort=title",
		"/api/libraries/lib_load_movies/categories",
		"/api/search",
		"/api/media/perf_movie_0001",
		"/api/settings",
		"/api/dashboard?mode=live&period=5m",
		"/api/artwork/perf_movie_0001/poster.svg?w=320&h=480",
	}

	results := make(chan loadSmokeSample, len(clients)*(len(paths)+4))
	var wg sync.WaitGroup
	start := make(chan struct{})
	active := make(chan struct{}, tier.concurrentViewers)
	for i, client := range clients {
		wg.Add(1)
		go func(worker int, client *http.Client) {
			defer wg.Done()
			<-start
			active <- struct{}{}
			defer func() { <-active }()
			for _, path := range paths {
				if path == "/api/search" {
					results <- timedJSONRequest(client, http.MethodPost, serverURL+path, SearchRequest{Query: "BudgetSignal"}, nil)
					continue
				}
				if strings.HasPrefix(path, "/api/libraries/lib_load_movies/browse") {
					sortField := "dateAdded"
					direction := "desc"
					if strings.Contains(path, "sort=title") {
						sortField = "title"
						direction = "asc"
					}
					endpoint := serverURL + strings.SplitN(path, "?", 2)[0]
					results <- timedJSONRequest(client, http.MethodPost, endpoint, BrowseLibraryRequest{
						Pivot: "movies",
						Sort:  []BrowseSort{{Field: sortField, Direction: direction}},
						Limit: 48,
					}, nil)
					continue
				}
				results <- timedRequest(client, http.MethodGet, serverURL+path, nil)
			}
			var playback PlaybackResponse
			startSample := timedJSONRequest(client, http.MethodPost, serverURL+"/api/playback-sessions", PlaybackSessionCreateRequest{
				MediaID:     "perf_movie_0001",
				SkipPreroll: true,
				Intent:      automaticPlaybackIntent(),
				ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{
					Device:               fmt.Sprintf("load-smoke-%02d", worker),
					Platform:             "web",
					SupportedContainers:  []string{"mp4"},
					SupportedVideoCodecs: []string{"h264"},
					SupportedAudioCodecs: []string{"aac"},
				}),
			}, &playback)
			results <- startSample
			if startSample.err != nil || startSample.status != http.StatusOK || playback.SessionID == "" {
				return
			}
			for beat := 0; beat < 2; beat++ {
				progressSeconds := 30 + beat*15
				results <- timedJSONRequest(client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, PlaybackProgressEvent{
					EventSequence:   int64(beat + 1),
					RecordedAt:      time.Now().UTC().Format(time.RFC3339Nano),
					State:           "playing",
					ProgressSeconds: &progressSeconds,
					DurationSeconds: 7200,
					BandwidthMbps:   8.5,
				}, nil)
			}
			results <- timedJSONRequest(client, http.MethodDelete, serverURL+"/api/playback-sessions/"+playback.SessionID, stoppedPlaybackRequest(playback), nil)
		}(i, client)
	}
	close(start)
	wg.Wait()
	close(results)

	var samples []loadSmokeSample
	for sample := range results {
		samples = append(samples, sample)
	}
	if len(samples) == 0 {
		t.Fatalf("mixed load smoke produced no samples")
	}
	var failures []string
	failureCounts := map[string]int{}
	var durations []time.Duration
	durationsByRoute := map[string][]time.Duration{}
	var totalBytes int
	for _, sample := range samples {
		durations = append(durations, sample.elapsed)
		routePath := sample.path
		if strings.HasPrefix(routePath, "/api/playback-sessions/") {
			routePath = "/api/playback-sessions/:id"
		}
		routeKey := sample.method + " " + routePath
		durationsByRoute[routeKey] = append(durationsByRoute[routeKey], sample.elapsed)
		totalBytes += sample.bytes
		if sample.err != nil {
			key := fmt.Sprintf("%s %s transport", sample.method, sample.path)
			failureCounts[key]++
			if len(failures) < 20 {
				failures = append(failures, fmt.Sprintf("%s %s failed: %v", sample.method, sample.path, sample.err))
			}
			continue
		}
		if tier.allowBoundedOverload && sample.status == http.StatusTooManyRequests && strings.HasPrefix(sample.path, "/api/search") && strings.Contains(sample.body, "search_busy") {
			continue
		}
		if tier.allowBoundedOverload && sample.status == http.StatusServiceUnavailable && strings.HasPrefix(sample.path, "/api/search") && strings.Contains(sample.body, "server_busy") {
			continue
		}
		if tier.allowBoundedOverload && sample.status == http.StatusServiceUnavailable && strings.HasPrefix(sample.path, "/api/libraries/") && strings.Contains(sample.body, "library_browse_timeout") {
			continue
		}
		if sample.status < 200 || sample.status >= 300 {
			key := fmt.Sprintf("%s %s status=%d", sample.method, sample.path, sample.status)
			failureCounts[key]++
			if len(failures) < 20 {
				failures = append(failures, fmt.Sprintf("%s %s status=%d body=%s", sample.method, sample.path, sample.status, sample.body))
			}
		}
	}
	if len(failureCounts) > 0 {
		keys := make([]string, 0, len(failureCounts))
		totalFailures := 0
		for key, count := range failureCounts {
			keys = append(keys, key)
			totalFailures += count
		}
		sort.Strings(keys)
		counts := make([]string, 0, len(keys))
		for _, key := range keys {
			counts = append(counts, fmt.Sprintf("%s: %d", key, failureCounts[key]))
		}
		t.Fatalf("mixed load smoke had %d failed requests (%s); first %d:\n%s\nserver log:\n%s", totalFailures, strings.Join(counts, ", "), len(failures), strings.Join(failures, "\n"), serverLog.String())
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	for _, routeDurations := range durationsByRoute {
		sort.Slice(routeDurations, func(i, j int) bool { return routeDurations[i] < routeDurations[j] })
	}
	for _, route := range []string{
		"POST /api/playback-sessions",
		"PATCH /api/playback-sessions/:id",
		"DELETE /api/playback-sessions/:id",
	} {
		routeDurations := durationsByRoute[route]
		if len(routeDurations) == 0 {
			t.Fatalf("mixed load did not exercise protected playback route %s", route)
		}
		if routeP95, routeP99 := percentileDuration(routeDurations, 95), percentileDuration(routeDurations, 99); routeP95 > tier.maximumP95 || routeP99 > tier.maximumP99 {
			t.Fatalf("protected playback route %s exceeded %s continuity budget: p95=%s (max %s) p99=%s (max %s)", route, tier.name, routeP95, tier.maximumP95, routeP99, tier.maximumP99)
		}
	}
	p95 := percentileDuration(durations, 95)
	p99 := percentileDuration(durations, 99)
	if p95 > tier.maximumP95 || p99 > tier.maximumP99 {
		for route, routeDurations := range durationsByRoute {
			t.Logf("mixed load route %s: p95=%s p99=%s samples=%d", route,
				percentileDuration(routeDurations, 95), percentileDuration(routeDurations, 99), len(routeDurations))
		}
		t.Fatalf("mixed load latency exceeded %s budget: p95=%s (max %s) p99=%s (max %s) samples=%d", tier.name, p95, tier.maximumP95, p99, tier.maximumP99, len(durations))
	}
	diagnostics := server.sqliteDiagnostics()
	if diagnostics.LockRetries > 10 || diagnostics.LockRetryWaitMillis > 2_000 {
		t.Fatalf("mixed load caused excessive sqlite lock retries: %#v", diagnostics)
	}
	if diagnostics.OpenConnections > diagnostics.MaxOpenConnections {
		t.Fatalf("sqlite pool exceeded configured max during mixed load: %#v", diagnostics)
	}
	t.Logf("mixed load %s: catalog=%d users=%d samples=%d bytes=%d p95=%s p99=%s sqlite_retries=%d sqlite_retry_wait_ms=%d",
		tier.name, tier.catalogItems, len(clients), len(samples), totalBytes, p95, p99, diagnostics.LockRetries, diagnostics.LockRetryWaitMillis)
}

type performanceTestTier struct {
	name                 string
	catalogItems         int
	sharedIdentities     int
	concurrentViewers    int
	maximumP95           time.Duration
	maximumP99           time.Duration
	allowBoundedOverload bool
}

// PORTICO_PERFORMANCE_TIER keeps the ordinary suite fast while making the
// production-shaped release and scheduled deep gates deterministic and
// repeatable on reference hardware.
func currentPerformanceTestTier() performanceTestTier {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PORTICO_PERFORMANCE_TIER"))) {
	case "release", "100k":
		return performanceTestTier{name: "release-100k", catalogItems: 100_000, sharedIdentities: 200, concurrentViewers: 100, maximumP95: 750 * time.Millisecond, maximumP99: 1500 * time.Millisecond}
	case "deep", "1m", "million", "nightly":
		return performanceTestTier{name: "deep-1m", catalogItems: 1_000_000, sharedIdentities: 200, concurrentViewers: 100, maximumP95: time.Second, maximumP99: 2 * time.Second}
	default:
		return performanceTestTier{name: "smoke", catalogItems: 1_500, sharedIdentities: 24, concurrentViewers: 24, maximumP95: 2 * time.Second, maximumP99: 3 * time.Second, allowBoundedOverload: true}
	}
}

func TestMillionItemMostlyUnchangedScannerCheckpointGate(t *testing.T) {
	tier := currentPerformanceTestTier()
	if tier.name != "deep-1m" {
		t.Skip("set PORTICO_PERFORMANCE_TIER=deep for the million-item scanner checkpoint gate")
	}
	server := newScannerTestServer(t)
	root := t.TempDir()
	const directoryCount = 1000
	filesPerDirectory := tier.catalogItems / directoryCount
	if filesPerDirectory*directoryCount != tier.catalogItems {
		t.Fatalf("deep catalogue size %d is not divisible by %d directories", tier.catalogItems, directoryCount)
	}
	for directoryIndex := 0; directoryIndex < directoryCount; directoryIndex++ {
		dir := filepath.Join(root, fmt.Sprintf("bucket-%04d", directoryIndex))
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("create scanner bucket %d: %v", directoryIndex, err)
		}
		for fileIndex := 0; fileIndex < filesPerDirectory; fileIndex++ {
			path := filepath.Join(dir, fmt.Sprintf("Movie-%04d-%04d.mp4", directoryIndex, fileIndex))
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatalf("create scanner fixture %s: %v", path, err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("close scanner fixture %s: %v", path, err)
			}
		}
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Million Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create million-item library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	cache := map[string]string{}
	checkpoints := map[string]scannerDirectoryCheckpoint{}
	for directoryIndex := 0; directoryIndex < directoryCount; directoryIndex++ {
		dir := filepath.Join(root, fmt.Sprintf("bucket-%04d", directoryIndex))
		signature, count := scannerDirectoryCheckpointState(dir, library.Type, cache)
		checkpoints[dir] = scannerDirectoryCheckpoint{Path: dir, Signature: signature, MediaFileCount: count}
	}
	rootSignature, rootCount := scannerDirectoryCheckpointState(root, library.Type, cache)
	checkpoints[root] = scannerDirectoryCheckpoint{Path: root, Signature: rootSignature, MediaFileCount: rootCount}
	if err := server.persistLibraryScanDirectoryCheckpoints(context.Background(), library.ID, checkpoints, nil, now, true); err != nil {
		t.Fatalf("seed scanner checkpoints: %v", err)
	}
	unchangedDir := filepath.Join(root, "bucket-0000")
	unchangedPath := filepath.Join(unchangedDir, "Movie-0000-0000.mp4")
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, added_at)
		VALUES ('perf_scanner_sentinel', ?, 'movie', 'Scanner Sentinel', 'Scanner Sentinel', ?, ?);
		INSERT INTO media_files (id, media_id, library_id, path, directory_path, source_type, available, size_bytes, mod_time, first_seen_at, last_seen_at, scan_generation, identity_evidence)
		VALUES ('perf_scanner_sentinel_file', 'perf_scanner_sentinel', ?, ?, ?, 'local', 1, 0, ?, ?, '2001-02-03T04:05:06Z', 'old', 'scanner:v2:seed')`,
		library.ID, unchangedPath, now, library.ID, unchangedPath, unchangedDir, fileModTime(mustStatPerformanceFile(t, unchangedPath)), now); err != nil {
		t.Fatalf("seed unchanged sentinel: %v", err)
	}
	changedDir := filepath.Join(root, "bucket-0999")
	changedPath := filepath.Join(changedDir, "Movie-0999-0000.mp4")
	if err := os.WriteFile(changedPath, []byte("changed"), 0o600); err != nil {
		t.Fatalf("modify one scanner fixture: %v", err)
	}
	started := time.Now()
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("million-item checkpoint scan: %v", err)
	}
	if result.FilesIndexed != tier.catalogItems || result.FilesUnchanged != tier.catalogItems-filesPerDirectory {
		t.Fatalf("million-item checkpoint result = indexed %d unchanged %d, expected %d/%d", result.FilesIndexed, result.FilesUnchanged, tier.catalogItems, tier.catalogItems-filesPerDirectory)
	}
	var sentinelLastSeen string
	if err := server.db.QueryRow(`SELECT last_seen_at FROM media_files WHERE id = 'perf_scanner_sentinel_file'`).Scan(&sentinelLastSeen); err != nil {
		t.Fatalf("read unchanged sentinel: %v", err)
	}
	if sentinelLastSeen != "2001-02-03T04:05:06Z" {
		t.Fatalf("unchanged directory media row was rewritten: last_seen_at=%q", sentinelLastSeen)
	}
	var changedRows int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_files WHERE library_id = ? AND directory_path = ?`, library.ID, changedDir).Scan(&changedRows); err != nil {
		t.Fatalf("count changed-directory rows: %v", err)
	}
	if changedRows != filesPerDirectory {
		t.Fatalf("changed directory indexed %d rows, expected %d", changedRows, filesPerDirectory)
	}
	t.Logf("million-item mostly-unchanged scanner gate: elapsed=%s unchanged=%d changed=%d", time.Since(started), result.FilesUnchanged, filesPerDirectory)
}

func mustStatPerformanceFile(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat performance fixture %s: %v", path, err)
	}
	return info
}

type performanceBearerTransport struct {
	token string
	base  http.RoundTripper
}

func (transport performanceBearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.Header = request.Header.Clone()
	copy.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.base.RoundTrip(copy)
}

func performanceViewerClients(t *testing.T, db *sql.DB, serverURL string, tier performanceTestTier) []*http.Client {
	t.Helper()
	clients := make([]*http.Client, 0, tier.sharedIdentities)
	if tier.name == "smoke" {
		for i := 0; i < tier.sharedIdentities; i++ {
			jar, _ := cookiejar.New(nil)
			client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
			loginUser(t, client, serverURL)
			clients = append(clients, client)
		}
		return clients
	}

	now := time.Now().UTC()
	permissionsJSON, err := json.Marshal(ownerPermissions())
	if err != nil {
		t.Fatalf("marshal performance viewer permissions: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin performance identity seed: %v", err)
	}
	defer tx.Rollback()
	for i := 0; i < tier.sharedIdentities; i++ {
		userID := fmt.Sprintf("perf_viewer_%04d", i)
		deviceID := fmt.Sprintf("perf_device_%04d", i)
		token := fmt.Sprintf("ptc_loc_performance_%04d", i)
		stamp := now.Format(time.RFC3339Nano)
		if _, err := tx.Exec(`
			INSERT INTO users (id, username, email, display_name, role, auth_origin, permissions_json, preferences_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'owner', 'local', ?, '{}', ?, ?)`,
			userID, userID, userID+"@example.test", fmt.Sprintf("Performance Viewer %d", i+1), string(permissionsJSON), stamp, stamp); err != nil {
			t.Fatalf("seed performance viewer %d: %v", i, err)
		}
		if _, err := tx.Exec(`
			INSERT INTO devices (id, user_id, installation_id, name, app, platform, trusted, created_at, last_seen_at)
			VALUES (?, ?, ?, ?, 'portico-web', 'web', 1, ?, ?)`,
			deviceID, userID, "perf-installation-"+userID, fmt.Sprintf("Performance Device %d", i+1), stamp, stamp); err != nil {
			t.Fatalf("seed performance device %d: %v", i, err)
		}
		if _, err := tx.Exec(`
			INSERT INTO sessions (id, user_id, profile_id, auth_provider, device_id, token_hash, expires_at, created_at, last_seen_at)
			VALUES (?, ?, ?, 'local', ?, ?, ?, ?, ?)`,
			"perf_session_"+userID, userID, userID, deviceID, hashToken(token), now.Add(24*time.Hour).Format(time.RFC3339Nano), stamp, stamp); err != nil {
			t.Fatalf("seed performance session %d: %v", i, err)
		}
		clients = append(clients, &http.Client{
			Transport: performanceBearerTransport{token: token, base: http.DefaultTransport},
			Timeout:   10 * time.Second,
		})
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit performance identities: %v", err)
	}
	// The mixed workload measures steady active viewers, not a simultaneous
	// cold-login storm. Validate every durable session and populate the bounded
	// authorization snapshots before releasing the synchronized browsing burst;
	// cold authentication remains covered by the dedicated auth suites.
	for index, client := range clients {
		response, err := client.Get(serverURL + "/api/auth/me")
		if err != nil {
			t.Fatalf("warm performance viewer %d: %v", index, err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("warm performance viewer %d status=%d", index, response.StatusCode)
		}
	}
	return clients
}

func searchResponseContains(response SearchResponse, mediaID string) bool {
	for _, group := range response.Groups {
		if mediaIDsContain(group.Items, mediaID) {
			return true
		}
	}
	return false
}

func TestLibraryBrowseQueryPlansUseCompositeIndexes(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	cases := []struct {
		name        string
		query       string
		args        []any
		indexName   string
		requireSort bool
	}{
		{
			name:        "top level title sort",
			query:       `SELECT m.id FROM media_items m WHERE m.library_id = ? AND m.parent_id IS NULL ORDER BY m.sort_title ASC LIMIT 50`,
			args:        []any{"lib_movies"},
			indexName:   "idx_media_library_parent_sort",
			requireSort: true,
		},
		{
			name:        "top level recently added sort",
			query:       `SELECT m.id FROM media_items m WHERE m.library_id = ? AND m.parent_id IS NULL ORDER BY m.added_at DESC LIMIT 50`,
			args:        []any{"lib_movies"},
			indexName:   "idx_media_library_parent_added",
			requireSort: true,
		},
		{
			name:        "child index sort",
			query:       `SELECT m.id FROM media_items m WHERE m.parent_id = ? ORDER BY m.index_number ASC LIMIT 100`,
			args:        []any{"show_atlas"},
			indexName:   "idx_media_parent_index",
			requireSort: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := explainQueryPlan(t, server, tc.query, tc.args...)
			normalized := strings.ToLower(strings.Join(plan, "\n"))
			if !strings.Contains(normalized, strings.ToLower(tc.indexName)) {
				t.Fatalf("query plan did not use %s:\n%s", tc.indexName, strings.Join(plan, "\n"))
			}
			if tc.requireSort && strings.Contains(normalized, "use temp b-tree") {
				t.Fatalf("query plan used temporary sort:\n%s", strings.Join(plan, "\n"))
			}
		})
	}
}

func TestSearchQueryPlanUsesFTSReadModel(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	plan := explainQueryPlan(t, server, `
		SELECT m.id
		FROM media_items m
		JOIN media_search ON media_search.media_id = m.id
		WHERE media_search MATCH ?
		ORDER BY bm25(media_search), m.sort_title ASC
		LIMIT 50`, mediaSearchQuery("Meridian"))
	normalized := strings.ToLower(strings.Join(plan, "\n"))
	if !strings.Contains(normalized, "virtual table") || !strings.Contains(normalized, "media_search") {
		t.Fatalf("search query plan did not use media_search FTS virtual table:\n%s", strings.Join(plan, "\n"))
	}
}

func TestPublishedNavigationQueryPlansAtRepresentativeScale(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	tier := currentPerformanceTestTier()
	const libraryID = "lib_query_plan_scale"
	seedPerformanceLibrary(t, db, libraryID, tier.catalogItems)
	for index := 0; index < 12; index++ {
		filter := fmt.Sprintf("genre:Perf Genre %02d", index)
		if _, err := db.Exec(`
			INSERT INTO library_category_counts (library_id, filter, count, representative_media_id, representative_image, updated_at)
			VALUES (?, ?, ?, ?, '', ?)
			ON CONFLICT(library_id, filter) DO UPDATE SET count = excluded.count`,
			libraryID, filter, tier.catalogItems/12, fmt.Sprintf("perf_movie_%04d", index), time.Now().UTC().Format(time.RFC3339)); err != nil {
			t.Fatalf("seed category read model %d: %v", index, err)
		}
	}
	if _, err := db.Exec(`ANALYZE`); err != nil {
		t.Fatalf("analyze representative catalogue: %v", err)
	}

	cases := []struct {
		name          string
		query         string
		args          []any
		require       []string
		allowTempSort bool
		allowedScans  []string
	}{
		{
			name:    "Home recently added",
			query:   `SELECT m.id FROM media_items m WHERE m.library_id = ? AND m.parent_id IS NULL ORDER BY m.added_at DESC LIMIT 6`,
			args:    []any{libraryID},
			require: []string{"idx_media_library_parent_added"},
		},
		{
			name:    "library browse title pivot",
			query:   `SELECT m.id FROM media_items m WHERE m.library_id = ? AND m.parent_id IS NULL ORDER BY m.sort_title ASC LIMIT 50`,
			args:    []any{libraryID},
			require: []string{"idx_media_library_parent_sort"},
		},
		{
			name: "global search pivot",
			query: `SELECT m.id FROM media_items m
				JOIN media_search ON media_search.media_id = m.id
				WHERE media_search MATCH ?
				ORDER BY bm25(media_search), m.sort_title ASC LIMIT 50`,
			args:          []any{mediaSearchQuery("BudgetSignal")},
			require:       []string{"media_search", "virtual table"},
			allowTempSort: true, // FTS rank order cannot be supplied by a B-tree index.
			allowedScans:  []string{"scan media_search virtual table"},
		},
		{
			name:    "category pivot",
			query:   `SELECT filter, count FROM library_category_counts WHERE library_id = ? ORDER BY count DESC, filter ASC LIMIT 50`,
			args:    []any{libraryID},
			require: []string{"idx_library_category_counts_library"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := explainQueryPlan(t, server, tc.query, tc.args...)
			normalized := strings.ToLower(strings.Join(plan, "\n"))
			for _, required := range tc.require {
				if !strings.Contains(normalized, strings.ToLower(required)) {
					t.Fatalf("representative %s plan omitted %q:\n%s", tc.name, required, strings.Join(plan, "\n"))
				}
			}
			if !tc.allowTempSort && strings.Contains(normalized, "use temp b-tree") {
				t.Fatalf("representative %s plan introduced a temporary sort:\n%s", tc.name, strings.Join(plan, "\n"))
			}
			for _, detail := range plan {
				detail = strings.ToLower(strings.TrimSpace(detail))
				if !strings.HasPrefix(detail, "scan ") {
					continue
				}
				allowed := false
				for _, prefix := range tc.allowedScans {
					if strings.HasPrefix(detail, prefix) {
						allowed = true
						break
					}
				}
				if !allowed {
					t.Fatalf("representative %s plan introduced a full scan: %q\n%s", tc.name, detail, strings.Join(plan, "\n"))
				}
			}
		})
	}
	t.Logf("published navigation query-plan gate: tier=%s catalogue=%d", tier.name, tier.catalogItems)
}

func TestLiveTVChannelSearchQueryPlanUsesFTSReadModel(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	plan := explainQueryPlan(t, server, `
		SELECT c.id
		FROM live_tv_channel_search
		JOIN live_tv_channels c ON c.id = live_tv_channel_search.channel_id
		JOIN live_tv_sources s ON s.id = c.source_id
		WHERE live_tv_channel_search MATCH ?
		  AND c.enabled = 1
		  AND c.hidden = 0
		  AND s.enabled = 1
		ORDER BY bm25(live_tv_channel_search), c.sort_order ASC, c.name ASC
		LIMIT 50`, mediaSearchQuery("Portico News"))
	normalized := strings.ToLower(strings.Join(plan, "\n"))
	if !strings.Contains(normalized, "virtual table") || !strings.Contains(normalized, "live_tv_channel_search") {
		t.Fatalf("live tv search query plan did not use live_tv_channel_search FTS virtual table:\n%s", strings.Join(plan, "\n"))
	}
	if strings.Contains(normalized, " like ") {
		t.Fatalf("live tv search query plan still used LIKE filtering:\n%s", strings.Join(plan, "\n"))
	}
}

func TestLiveTVGuideChannelQueryUsesFTSReadModel(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	from := time.Now().UTC().Truncate(time.Second)
	to := from.Add(2 * time.Hour)
	where := []string{"c.source_id = ?", "c.enabled = 1", "c.hidden = 0"}
	args := []any{"src_filter_page"}
	appendLiveTVGuideTextSearch(&where, &args, from, to, "Portico News")
	plan := explainQueryPlan(t, server, `
		SELECT c.id
		FROM live_tv_channels c
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY c.sort_order ASC, c.name ASC
		LIMIT 25`, args...)
	normalized := strings.ToLower(strings.Join(plan, "\n"))
	if !strings.Contains(normalized, "virtual table") || !strings.Contains(normalized, "live_tv_channel_search") {
		t.Fatalf("live tv guide channel query did not use live_tv_channel_search FTS virtual table:\n%s", strings.Join(plan, "\n"))
	}
	if strings.Contains(normalized, "coalesce(c.number") {
		t.Fatalf("live tv guide channel query still used concatenated channel LIKE text:\n%s", strings.Join(plan, "\n"))
	}
}

func TestSearchPrefixQueryStaysBoundedAndUsesFTS(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	items, err := server.searchMedia("", "Meri", 50)
	if err != nil {
		t.Fatalf("prefix search failed: %v", err)
	}
	if !mediaItemsContainID(items, "movie_meridian") {
		t.Fatalf("prefix search did not find Meridian fixture: %#v", mediaItemIDs(items))
	}
	query := mediaSearchQuery("alpha beta gamma delta epsilon zeta eta theta " + strings.Repeat("overflow", 40))
	if strings.Count(query, "*") != maxSearchTerms {
		t.Fatalf("prefix query was not capped to %d terms: %q", maxSearchTerms, query)
	}
	if strings.Contains(query, "theta") || strings.Contains(query, "overflow") {
		t.Fatalf("prefix query included terms beyond cap: %q", query)
	}
}

func TestDashboardInFlightWaitHonorsContextCancellation(t *testing.T) {
	server := &Server{dashboardInFlight: map[string]chan struct{}{}}
	user := User{ID: "usr_dashboard_wait", Role: "owner", AuthProvider: "local", Permissions: ownerPermissions()}
	filters := dashboardFilters{Mode: "live", Period: "5m"}
	key := dashboardCacheKey(user, filters)
	server.dashboardInFlight[key] = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := server.cachedDashboard(ctx, user, filters)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cachedDashboard error = %v, expected context.Canceled", err)
	}
}

func mediaItemsContainID(items []MediaItem, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func mediaItemIDs(items []MediaItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func assertFunctionUsesListItemLoader(t *testing.T, filename string, signature string) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(currentFile), filename)
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	body := string(source)
	start := strings.Index(body, signature)
	if start < 0 {
		t.Fatalf("%s not found in %s", signature, filename)
	}
	end := strings.Index(body[start+len(signature):], "\nfunc ")
	if end < 0 {
		end = len(body) - start - len(signature)
	}
	functionBody := body[start : start+len(signature)+end]
	if strings.Contains(functionBody, "mediaByOrderedIDs(") {
		t.Fatalf("%s should avoid heavy ordered media hydration", signature)
	}
	if !strings.Contains(functionBody, "mediaListItemsByOrderedIDs(") {
		t.Fatalf("%s should validate selected media with mediaListItemsByOrderedIDs", signature)
	}
}

func TestQueryMediaUsesAvailabilityReadModel(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	plan := explainQueryPlan(t, server, `
		SELECT m.id, COALESCE(availability.file_count, 0), COALESCE(availability.missing_file_count, 0)
		FROM media_items m
		LEFT JOIN media_availability availability ON availability.media_id = m.id
		WHERE m.library_id = ? AND m.parent_id IS NULL
		ORDER BY m.added_at DESC
		LIMIT 50`, "lib_movies")
	normalized := strings.ToLower(strings.Join(plan, "\n"))
	if !strings.Contains(normalized, "media_availability") {
		t.Fatalf("query plan did not use media_availability read model:\n%s", strings.Join(plan, "\n"))
	}
	if strings.Contains(normalized, "media_files") {
		t.Fatalf("query plan still touched media_files for availability counts:\n%s", strings.Join(plan, "\n"))
	}
}

func TestLibrarySourceAndHDRFiltersUseAvailabilityReadModel(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "server.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	body := string(source)
	checks := []struct {
		name          string
		startMarker   string
		endMarker     string
		requiredField string
	}{
		{"local source filter", "func librarySourceLocalSQL()", "\nfunc librarySourceRemoteSQL()", "availability.has_local_source"},
		{"remote source filter", "func librarySourceRemoteSQL()", "\nfunc librarySourcePathSQL()", "availability.has_remote_source"},
		{"unavailable filter", "func libraryUnavailableSourceSQL()", "\nfunc libraryDuplicateSQL()", "availability.missing_file_count"},
	}
	for _, check := range checks {
		functionBody := sourceBetweenMarkers(t, body, check.startMarker, check.endMarker)
		if !strings.Contains(functionBody, check.requiredField) {
			t.Fatalf("%s should use %s", check.name, check.requiredField)
		}
		if strings.Contains(functionBody, "FROM media_files") || strings.Contains(functionBody, "FROM media_streams") {
			t.Fatalf("%s should not scan raw media files or streams:\n%s", check.name, functionBody)
		}
	}
	filterBody := sourceBetweenMarkers(t, body, `case "hdr", "4k", "uhd":`, `case "downloaded", "downloadable":`)
	if !strings.Contains(filterBody, "availability.has_hdr_source") {
		t.Fatalf("HDR filter should use availability.has_hdr_source")
	}
	if strings.Contains(filterBody, "FROM media_files") || strings.Contains(filterBody, "FROM media_streams") {
		t.Fatalf("HDR filter should not scan raw media files or streams:\n%s", filterBody)
	}
}

func sourceBetweenMarkers(t *testing.T, source string, startMarker string, endMarker string) string {
	t.Helper()
	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("source marker %q not found", startMarker)
	}
	end := strings.Index(source[start+len(startMarker):], endMarker)
	if end < 0 {
		t.Fatalf("source end marker %q not found after %q", endMarker, startMarker)
	}
	return source[start : start+len(startMarker)+end]
}

func explainQueryPlan(t *testing.T, server *Server, query string, args ...any) []string {
	t.Helper()
	rows, err := server.db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("explain query plan: %v", err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan query plan: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read query plan: %v", err)
	}
	return details
}

func timedGET(t *testing.T, client *http.Client, endpoint string) (int, []byte, time.Duration) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("create timed request: %v", err)
	}
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("send timed request: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read timed response: %v", err)
	}
	return resp.StatusCode, body, elapsed
}

type loadSmokeSample struct {
	method  string
	path    string
	status  int
	bytes   int
	body    string
	rawBody []byte
	elapsed time.Duration
	err     error
}

func timedJSONRequest(client *http.Client, method string, endpoint string, payload any, out any) loadSmokeSample {
	var requestBody io.Reader
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return loadSmokeSample{method: method, path: endpoint, err: err}
		}
		requestBody = bytes.NewReader(payloadBytes)
	}
	sample := timedRequest(client, method, endpoint, requestBody)
	if out != nil && sample.err == nil && sample.status >= 200 && sample.status < 300 && len(sample.rawBody) > 0 {
		if err := json.Unmarshal(sample.rawBody, out); err != nil {
			sample.err = err
		}
	}
	return sample
}

func timedRequest(client *http.Client, method string, endpoint string, body io.Reader) loadSmokeSample {
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return loadSmokeSample{method: method, path: endpoint, err: err}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		req.Header.Set(csrfHeaderName, "1")
	}
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	sample := loadSmokeSample{method: method, path: req.URL.Path, elapsed: elapsed, err: err}
	if err != nil {
		return sample
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	sample.status = resp.StatusCode
	sample.bytes = len(responseBody)
	sample.rawBody = responseBody
	if len(responseBody) > 2_000 {
		sample.body = string(responseBody[:2_000])
	} else {
		sample.body = string(responseBody)
	}
	if err != nil {
		sample.err = err
	}
	return sample
}

func percentileDuration(sorted []time.Duration, percentile int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := (len(sorted)*percentile + 99) / 100
	if index < 1 {
		index = 1
	}
	if index > len(sorted) {
		index = len(sorted)
	}
	return sorted[index-1]
}

func explainPlanDetails(t *testing.T, rows *sql.Rows) []string {
	t.Helper()
	plan := []string{}
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan query plan: %v", err)
		}
		plan = append(plan, strings.ToLower(detail))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("query plan rows: %v", err)
	}
	return plan
}

func seedPerformanceLibrary(t *testing.T, db *sql.DB, libraryID string, count int) {
	t.Helper()
	now := time.Now().UTC()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin performance catalogue seed: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES (?, 'Performance Movies', 'movie', 990, '/tmp/performance-movies', '{}', ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, type = excluded.type`,
		libraryID, now.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("insert performance library: %v", err)
	}
	if count <= 0 {
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit empty performance catalogue seed: %v", err)
		}
		return
	}
	if _, err := tx.Exec(`
		WITH RECURSIVE sequence(i) AS (
			SELECT 0
			UNION ALL
			SELECT i + 1 FROM sequence WHERE i + 1 < ?
		)
		INSERT INTO media_items (
			id, library_id, type, title, sort_title, year, duration_seconds,
			summary, tagline, content_rating, community_rating, critic_rating,
			studio, genres_json, added_at, art_seed
		)
		SELECT
			printf('perf_movie_%04d', i), ?, 'movie',
			printf('Performance Movie %04d', i), printf('Performance Movie %04d', i),
			1990 + (i % 35), 7200,
			printf('BudgetSignal performance fixture %04d for endpoint response budgets.', i),
			'BudgetSignal fixture',
			CASE WHEN i % 3 = 0 THEN 'PG' ELSE 'PG-13' END,
			6.0 + CAST(i % 30 AS REAL) / 10.0, 70 + (i % 25),
			printf('Perf Studio %02d', i % 8),
			printf('["Perf Genre %02d","BudgetSignal"]', i % 12),
			strftime('%Y-%m-%dT%H:%M:%SZ', ?, printf('-%d minutes', i)),
			printf('perf_movie_%04d', i)
		FROM sequence
		WHERE true
		ON CONFLICT(id) DO UPDATE SET
			library_id = excluded.library_id,
			title = excluded.title,
			sort_title = excluded.sort_title,
			summary = excluded.summary,
			genres_json = excluded.genres_json,
			added_at = excluded.added_at`, count, libraryID, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert performance media set: %v", err)
	}
	if _, err := tx.Exec(`DELETE FROM media_search WHERE media_id IN (SELECT id FROM media_items WHERE library_id = ?)`, libraryID); err != nil {
		t.Fatalf("reset performance search set: %v", err)
	}
	if _, err := tx.Exec(`
		WITH RECURSIVE sequence(i) AS (
			SELECT 0
			UNION ALL
			SELECT i + 1 FROM sequence WHERE i + 1 < ?
		)
		INSERT INTO media_search (media_id, title, summary, genres)
		SELECT
			printf('perf_movie_%04d', i),
			printf('Performance Movie %04d BudgetSignal', i),
			printf('Endpoint budget fixture %04d', i),
			printf('Perf Genre %02d BudgetSignal Perf Studio %02d', i % 12, i % 8)
		FROM sequence`, count); err != nil {
		t.Fatalf("insert performance search set: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit performance catalogue seed: %v", err)
	}
}
