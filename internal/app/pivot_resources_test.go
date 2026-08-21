package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestLibraryCategoriesUsePrincipalScopedCursorPages(t *testing.T) {
	serverURL, _ := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var first ListResponse[LibraryCategory]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/lib_movies/categories?limit=1", nil, &first)
	if status != http.StatusOK {
		t.Fatalf("categories status=%d body=%s", status, body)
	}
	if len(first.Items) != 1 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first categories page = %#v", first)
	}
	var second ListResponse[LibraryCategory]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/lib_movies/categories?limit=1&cursor="+first.NextCursor, nil, &second)
	if status != http.StatusOK || len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("second categories page status=%d body=%s response=%#v", status, body, second)
	}
}

func TestAudiobookFacetEntitiesAreDurableNamesakeSafeAndKeysetPaged(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_entity_books', 'Entity Books', 'audiobook', 991, '/tmp/entity-books', '{}', ?)`, now); err != nil {
		t.Fatalf("insert audiobook library: %v", err)
	}
	insertBook := func(id, title, metadata string) {
		t.Helper()
		if _, err := db.Exec(`
			INSERT INTO media_items (
				id, library_id, type, title, sort_title, genres_json, tags_json, labels_json,
				typed_metadata_json, added_at, random_key
			) VALUES (?, 'lib_entity_books', 'audiobook', ?, ?, '[]', '[]', '[]', ?, ?, ?)`,
			id, title, title, metadata, now, mediaRandomKey(id)); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insertBook("entity_alex_one", "Alex One", `{"author":"Alex Writer","authorProvider":"books","authorId":"alex-1"}`)
	insertBook("entity_alex_two", "Alex Two", `{"author":"Alex Writer","authorProvider":"books","authorId":"alex-1"}`)
	insertBook("entity_namesake", "Namesake", `{"author":"Alex Writer","authorProvider":"books","authorId":"alex-2"}`)
	insertBook("entity_charlie", "Charlie", `{"author":"Charlie Writer","authorProvider":"books","authorId":"charlie"}`)
	insertBook("entity_weak_one", "Weak One", `{"author":"Weak Namesake"}`)
	insertBook("entity_weak_two", "Weak Two", `{"author":"Weak Namesake"}`)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	loadAuthors := func(path string) ListResponse[LibraryFacetValue] {
		t.Helper()
		var response ListResponse[LibraryFacetValue]
		status, body := doJSON(t, client, http.MethodGet, serverURL+path, nil, &response)
		if status != http.StatusOK {
			t.Fatalf("authors status=%d body=%s", status, body)
		}
		return response
	}
	all := loadAuthors("/api/libraries/lib_entity_books/authors?limit=20")
	if len(all.Items) != 4 || all.Total != 4 {
		t.Fatalf("author entities = %#v", all)
	}
	var mergedID, namesakeID string
	weakIDs := []string{}
	for _, author := range all.Items {
		if !strings.HasPrefix(author.ID, "aent_") || strings.Contains(author.ID, "Alex") {
			t.Fatalf("author ID is not opaque: %#v", author)
		}
		if author.Name == "Alex Writer" && author.Count == 2 {
			mergedID = author.ID
		}
		if author.Name == "Alex Writer" && author.Count == 1 {
			namesakeID = author.ID
		}
		if author.Name == "Weak Namesake" {
			if author.Count != 2 {
				t.Fatalf("normalized-name fallback did not converge weak metadata: %#v", all.Items)
			}
			weakIDs = append(weakIDs, author.ID)
		}
	}
	if mergedID == "" || namesakeID == "" || mergedID == namesakeID {
		t.Fatalf("strong-key namesakes were conflated: %#v", all.Items)
	}
	if len(weakIDs) != 1 {
		t.Fatalf("weak metadata did not use one normalized-name identity: %#v", all.Items)
	}
	var weakMemberID string
	if err := db.QueryRow(`
		SELECT entity_id FROM audiobook_browse_entity_members
		WHERE entity_kind = 'author' AND media_id = 'entity_weak_one'`).Scan(&weakMemberID); err != nil {
		t.Fatalf("load weak member identity: %v", err)
	}

	if _, err := db.Exec(`
		UPDATE media_items
		SET typed_metadata_json = json_set(typed_metadata_json, '$.author', 'Alex Renamed'),
			title = title || ' Rescanned', sort_title = title || ' Rescanned'
		WHERE id IN ('entity_alex_one', 'entity_alex_two')`); err != nil {
		t.Fatalf("rename/rescan author members: %v", err)
	}
	renamed := loadAuthors("/api/libraries/lib_entity_books/authors?limit=20")
	foundRenamed := false
	for _, author := range renamed.Items {
		if author.Name == "Alex Renamed" {
			foundRenamed = author.ID == mergedID && author.Count == 2
		}
	}
	if !foundRenamed {
		t.Fatalf("rename/rescan changed durable identity %q: %#v", mergedID, renamed.Items)
	}
	if _, err := db.Exec(`UPDATE media_items SET typed_metadata_json = '{"author":"Weak Renamed"}' WHERE id = 'entity_weak_one'`); err != nil {
		t.Fatalf("rename weak member: %v", err)
	}
	_ = loadAuthors("/api/libraries/lib_entity_books/authors?limit=20")
	var weakRenamedID string
	if err := db.QueryRow(`
		SELECT entity_id FROM audiobook_browse_entity_members
		WHERE entity_kind = 'author' AND media_id = 'entity_weak_one'`).Scan(&weakRenamedID); err != nil || weakRenamedID != weakMemberID {
		t.Fatalf("weak rename changed durable identity: before=%q after=%q err=%v", weakMemberID, weakRenamedID, err)
	}
	identity, err := server.virtualAudiobookFacetIdentityContext(context.Background(), mergedID)
	if err != nil || identity.Name != "Alex Renamed" || identity.LibraryID != "lib_entity_books" {
		t.Fatalf("durable identity lookup = %#v err=%v", identity, err)
	}

	first := loadAuthors("/api/libraries/lib_entity_books/authors?limit=1")
	if len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first keyset page = %#v", first)
	}
	firstID := first.Items[0].ID
	insertBook("entity_before_cursor", "Before Cursor", `{"author":"Aardvark Writer","authorProvider":"books","authorId":"aardvark"}`)
	second := loadAuthors("/api/libraries/lib_entity_books/authors?limit=20&cursor=" + url.QueryEscape(first.NextCursor))
	for _, item := range second.Items {
		if item.ID == firstID || item.Name == "Aardvark Writer" {
			t.Fatalf("keyset continuation overlapped or admitted insertion before cursor: first=%#v second=%#v", first, second)
		}
	}
}

func TestAudiobookSeriesPreviewAndContinuationShareCanonicalOrder(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_ordered_books', 'Ordered Books', 'audiobook', 992, '/tmp/ordered-books', '{}', ?)`, now); err != nil {
		t.Fatalf("insert audiobook library: %v", err)
	}
	books := []struct {
		id, title, seriesIndex, releaseDate string
	}{
		{"ordered_three", "Zulu", "3", "2024-01-03"},
		{"ordered_one_b", "Bravo", "1", "2024-01-02"},
		{"ordered_one_a", "Alpha", "1", "2024-01-01"},
		{"ordered_two", "Middle", "2", "2024-01-01"},
	}
	for _, book := range books {
		metadata, _ := json.Marshal(map[string]string{
			"author": "Ordered Author", "authorId": "ordered-author",
			"series": "Ordered Series", "seriesId": "ordered-series",
			"seriesIndex": book.seriesIndex, "releaseDate": book.releaseDate,
		})
		if _, err := db.Exec(`
			INSERT INTO media_items (
				id, library_id, type, title, sort_title, genres_json, tags_json, labels_json,
				typed_metadata_json, added_at, random_key
			) VALUES (?, 'lib_ordered_books', 'audiobook', ?, ?, '[]', '[]', '[]', ?, ?, ?)`,
			book.id, book.title, book.title, string(metadata), now, mediaRandomKey(book.id)); err != nil {
			t.Fatalf("insert %s: %v", book.id, err)
		}
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var series ListResponse[LibraryFacetValue]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/lib_ordered_books/series?limit=10", nil, &series)
	if status != http.StatusOK || len(series.Items) != 1 || series.Items[0].Count != 4 {
		t.Fatalf("series status=%d body=%s response=%#v", status, body, series)
	}
	seriesID := series.Items[0].ID
	var detail MediaItem
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/"+seriesID, nil, &detail)
	if status != http.StatusOK || len(detail.Children) != 4 {
		t.Fatalf("series detail status=%d body=%s response=%#v", status, body, detail)
	}
	previewIDs := mediaItemIDs(detail.Children)

	continuationIDs := []string{}
	cursor := ""
	for {
		path := serverURL + "/api/media/" + seriesID + "/children?limit=2"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		var page MediaCardPageResponse
		status, body = doJSON(t, client, http.MethodGet, path, nil, &page)
		if status != http.StatusOK {
			t.Fatalf("children status=%d body=%s", status, body)
		}
		for _, item := range page.Items {
			continuationIDs = append(continuationIDs, item.ID)
		}
		if !page.PageInfo.HasMore || page.PageInfo.NextCursor == nil {
			break
		}
		cursor = *page.PageInfo.NextCursor
	}
	if strings.Join(previewIDs, ",") != strings.Join(continuationIDs, ",") {
		t.Fatalf("preview/continuation order diverged: preview=%v continuation=%v", previewIDs, continuationIDs)
	}
	want := []string{"ordered_one_a", "ordered_one_b", "ordered_two", "ordered_three"}
	if strings.Join(previewIDs, ",") != strings.Join(want, ",") {
		t.Fatalf("canonical series order = %v want=%v", previewIDs, want)
	}
	if !sort.StringsAreSorted([]string{detail.Children[0].ReleaseDateKey, detail.Children[1].ReleaseDateKey}) {
		t.Fatalf("same-index books are not release-date ordered: %#v", detail.Children[:2])
	}
}

func TestDVRScheduleIsTypedAndExcludesCompletedRecordings(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_schedule', 'Schedule', 'm3u', ?, ?)`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	for index, status := range []string{"scheduled", "running", "complete"} {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_recordings (id, user_id, source_id, title, status, starts_at, ends_at, created_at, updated_at)
			VALUES (?, ?, 'src_schedule', ?, ?, ?, ?, ?, ?)`,
			"rec_schedule_"+status, user.ID, "Recording "+status, status,
			now.Add(time.Duration(index)*time.Hour).Format(time.RFC3339), now.Add(time.Duration(index+1)*time.Hour).Format(time.RFC3339), nowText, nowText); err != nil {
			t.Fatalf("insert %s recording: %v", status, err)
		}
	}
	recorder := performDVRRouteRequest(server, user, http.MethodGet, "/api/dvr/schedule?limit=1", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("schedule status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var first CursorListResponse[DVRRecording]
	if err := json.Unmarshal(recorder.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode schedule: %v", err)
	}
	if len(first.Items) != 1 || !first.PageInfo.HasMore || first.PageInfo.NextCursor == nil || first.Items[0].Status == "complete" {
		t.Fatalf("schedule page = %#v", first)
	}
}
