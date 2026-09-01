package app

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestSavedResourcesAreSeparateCursorPagedProducts(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	for _, title := range []string{"Collection One", "Collection Two", "Collection Three"} {
		var collection Collection
		status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/collections", CollectionCreateRequest{Title: title}, &collection)
		if status != http.StatusCreated || collection.ID == "" || collection.Title != title {
			t.Fatalf("create collection %q status=%d body=%s collection=%#v", title, status, body, collection)
		}
		if !isOpaquePublicResourceID(collection.ID) {
			t.Fatalf("collection ID %q is not an opaque 160-bit identifier", collection.ID)
		}
	}
	var playlist SavedPlaylist
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playlists", PlaylistCreateRequest{Title: "Playlist One"}, &playlist)
	if status != http.StatusCreated || playlist.ID == "" {
		t.Fatalf("create playlist status=%d body=%s playlist=%#v", status, body, playlist)
	}
	if !isOpaquePublicResourceID(playlist.ID) {
		t.Fatalf("playlist ID %q is not an opaque 160-bit identifier", playlist.ID)
	}
	status, _ = doJSON(t, client, http.MethodGet, serverURL+"/api/collections/"+playlist.ID, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("playlist exposed through collection route: status=%d", status)
	}

	var first CollectionPage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/collections?limit=1", nil, &first)
	if status != http.StatusOK || len(first.Items) != 1 || !first.PageInfo.HasMore || first.PageInfo.NextCursor == nil {
		t.Fatalf("first collection page status=%d body=%s page=%#v", status, body, first)
	}
	var second CollectionPage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/collections?limit=1&cursor="+*first.PageInfo.NextCursor, nil, &second)
	if status != http.StatusOK || len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("second collection page status=%d body=%s page=%#v", status, body, second)
	}
	status, _ = doJSON(t, client, http.MethodGet, serverURL+"/api/playlists?limit=1&cursor="+*first.PageInfo.NextCursor, nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("collection cursor replayed in playlists: status=%d", status)
	}
}

func TestReadAPIKeyBrowsesSavedViewWithOpaqueCursor(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	interactive := &http.Client{Jar: jar}
	loginUser(t, interactive, serverURL)
	var libraryID, ownerID string
	if err := db.QueryRow(`SELECT id FROM libraries WHERE type = 'movie' ORDER BY sort_order LIMIT 1`).Scan(&libraryID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM users WHERE role = 'owner' LIMIT 1`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	var view SavedView
	status, body := doJSON(t, interactive, http.MethodPost, serverURL+"/api/saved-views", SavedViewCreateRequest{Title: "API browse", LibraryID: libraryID, Pivot: "movies", Sort: []BrowseSort{{Field: "title", Direction: "asc"}}}, &view)
	if status != http.StatusCreated {
		t.Fatalf("create saved view status=%d body=%s", status, body)
	}
	_, token, err := server.createAPIKey(ownerID, APIKeyCreateRequest{Name: "Saved view reader", Scopes: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	browse := func(input SavedViewBrowseRequest) BrowseLibraryResponse {
		t.Helper()
		payload, _ := json.Marshal(input)
		request, requestErr := http.NewRequest(http.MethodPost, serverURL+"/api/saved-views/"+view.ID+"/browse", bytes.NewReader(payload))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer response.Body.Close()
		var result BrowseLibraryResponse
		responseBody, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK || json.Unmarshal(responseBody, &result) != nil {
			t.Fatalf("API-key saved browse status=%d body=%s", response.StatusCode, responseBody)
		}
		return result
	}
	first := browse(SavedViewBrowseRequest{Limit: 1})
	if len(first.Items) != 1 || !first.PageInfo.HasMore || first.PageInfo.NextCursor == nil {
		t.Fatalf("first saved-view page=%#v", first)
	}
	second := browse(SavedViewBrowseRequest{Limit: 1, Cursor: *first.PageInfo.NextCursor})
	if len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("cursor did not advance: first=%#v second=%#v", first, second)
	}
}

func isOpaquePublicResourceID(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func TestSavedResourceBatchMutationsAreAtomicAndItemsUseKeysetCursors(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playlist SavedPlaylist
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playlists", PlaylistCreateRequest{
		Title:    "Road Trip",
		MediaIDs: []string{"movie_meridian", "movie_saffron", "movie_meridian"},
	}, &playlist)
	if status != http.StatusCreated || playlist.ItemCount != 3 {
		t.Fatalf("create playlist status=%d body=%s playlist=%#v", status, body, playlist)
	}
	initialRevision := playlist.UpdatedAt
	var seeded PlaylistEntryPage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playlists/"+playlist.ID+"/items?limit=10", nil, &seeded)
	if status != http.StatusOK || len(seeded.Items) != 3 || seeded.Items[0].Position != 0 || seeded.Items[2].Position != 2 {
		t.Fatalf("seeded playlist entries status=%d body=%s page=%#v", status, body, seeded)
	}
	if seeded.Items[0].Media.ID != "movie_meridian" || seeded.Items[2].Media.ID != "movie_meridian" || seeded.Items[0].EntryID == seeded.Items[2].EntryID {
		t.Fatalf("duplicate media entries do not have stable distinct identities: %#v", seeded.Items)
	}
	for _, entry := range seeded.Items {
		if !isOpaquePublicResourceID(entry.EntryID) {
			t.Fatalf("playlist entry ID %q is not an opaque 160-bit identifier", entry.EntryID)
		}
	}
	var saffronEntryID string
	for _, entry := range seeded.Items {
		if entry.Media.ID == "movie_saffron" {
			saffronEntryID = entry.EntryID
		}
	}
	if saffronEntryID == "" {
		t.Fatal("seeded playlist omitted the saffron entry")
	}
	var mutation PlaylistItemsBatchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{
		AddMediaIDs:       []string{"movie_neon", "movie_neon"},
		RemoveEntryIDs:    []string{saffronEntryID},
		ExpectedUpdatedAt: initialRevision,
	}, &mutation)
	if status != http.StatusOK || mutation.Added != 2 || mutation.Removed != 1 || mutation.Playlist.ItemCount != 4 {
		t.Fatalf("playlist batch status=%d body=%s response=%#v", status, body, mutation)
	}

	var first PlaylistEntryPage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playlists/"+playlist.ID+"/items?limit=1", nil, &first)
	if status != http.StatusOK || len(first.Items) != 1 || first.Items[0].Media.ID != "movie_meridian" || first.Items[0].Position != 0 || first.PageInfo.NextCursor == nil {
		t.Fatalf("first playlist item page status=%d body=%s page=%#v", status, body, first)
	}
	var second PlaylistEntryPage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playlists/"+playlist.ID+"/items?limit=1&cursor="+*first.PageInfo.NextCursor, nil, &second)
	if status != http.StatusOK || len(second.Items) != 1 || second.Items[0].Media.ID != "movie_meridian" || second.Items[0].Position != 1 || !second.PageInfo.HasMore {
		t.Fatalf("second playlist item page status=%d body=%s page=%#v", status, body, second)
	}
	var all PlaylistEntryPage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playlists/"+playlist.ID+"/items?limit=10", nil, &all)
	if status != http.StatusOK || len(all.Items) != 4 || playlistEntryMediaCount(all, "movie_meridian") != 2 || playlistEntryMediaCount(all, "movie_neon") != 2 {
		t.Fatalf("duplicate playlist entry page status=%d body=%s page=%#v", status, body, all)
	}
	order := make([]string, 0, len(all.Items))
	for index := len(all.Items) - 1; index >= 0; index-- {
		order = append(order, all.Items[index].EntryID)
	}
	var reordered PlaylistItemsBatchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{
		OrderEntryIDs:     order,
		ExpectedUpdatedAt: mutation.Playlist.UpdatedAt,
	}, &reordered)
	if status != http.StatusOK || reordered.Playlist.ItemCount != 4 {
		t.Fatalf("playlist reorder status=%d body=%s response=%#v", status, body, reordered)
	}
	var ordered PlaylistEntryPage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playlists/"+playlist.ID+"/items?limit=10", nil, &ordered)
	if status != http.StatusOK || len(ordered.Items) != len(order) {
		t.Fatalf("ordered playlist page status=%d body=%s page=%#v", status, body, ordered)
	}
	for index, entryID := range order {
		if ordered.Items[index].EntryID != entryID || ordered.Items[index].Position != index {
			t.Fatalf("ordered entry %d=%#v want entry=%s position=%d", index, ordered.Items[index], entryID, index)
		}
	}
	var removedDuplicate PlaylistItemsBatchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{
		RemoveEntryIDs:    []string{ordered.Items[len(ordered.Items)-1].EntryID},
		ExpectedUpdatedAt: reordered.Playlist.UpdatedAt,
	}, &removedDuplicate)
	if status != http.StatusOK || removedDuplicate.Removed != 1 || removedDuplicate.Playlist.ItemCount != 3 {
		t.Fatalf("remove one duplicate status=%d body=%s response=%#v", status, body, removedDuplicate)
	}

	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{
		AddMediaIDs:       []string{"movie_saffron"},
		ExpectedUpdatedAt: initialRevision,
	}, nil)
	if status != http.StatusConflict {
		t.Fatalf("stale revision status=%d, want 409", status)
	}
	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{
		AddMediaIDs:       []string{"movie_saffron"},
		OrderEntryIDs:     []string{ordered.Items[0].EntryID},
		ExpectedUpdatedAt: removedDuplicate.Playlist.UpdatedAt,
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("incomplete atomic reorder status=%d, want 400", status)
	}
	var after PlaylistEntryPage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playlists/"+playlist.ID+"/items?limit=10", nil, &after)
	if status != http.StatusOK || playlistEntryMediaCount(after, "movie_saffron") != 0 || len(after.Items) != 3 {
		t.Fatalf("failed batch was not rolled back status=%d body=%s page=%#v", status, body, after)
	}

	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items", map[string]string{"mediaId": "movie_saffron"}, nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("legacy single-item mutation status=%d, want 405 for the read-only items resource", status)
	}
	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items/bulk", map[string]any{"mediaIds": []string{"movie_saffron"}}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("legacy bulk alias status=%d, want 404", status)
	}
}

func TestCollectionMembershipBatchIsSetBased(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var collection Collection
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/collections", CollectionCreateRequest{
		Title:    "Award Winners",
		MediaIDs: []string{"movie_meridian", "movie_saffron"},
	}, &collection)
	if status != http.StatusCreated {
		t.Fatalf("create collection status=%d body=%s", status, body)
	}
	var mutation CollectionMembershipBatchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/collections/"+collection.ID+"/memberships:batch", CollectionMembershipBatchRequest{
		AddMediaIDs:       []string{"movie_neon", "movie_meridian"},
		RemoveMediaIDs:    []string{"movie_saffron"},
		ExpectedUpdatedAt: collection.UpdatedAt,
	}, &mutation)
	if status != http.StatusOK || mutation.Added != 1 || mutation.Removed != 1 || mutation.Unchanged != 1 || mutation.Collection.ItemCount != 2 {
		t.Fatalf("collection membership batch status=%d body=%s response=%#v", status, body, mutation)
	}
}

func TestSavedViewsPersistCanonicalBrowseDefinitions(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var libraryID string
	if err := db.QueryRow(`SELECT id FROM libraries WHERE type = 'movie' ORDER BY sort_order LIMIT 1`).Scan(&libraryID); err != nil {
		t.Fatalf("load movie library: %v", err)
	}
	queryValue, _ := json.Marshal("movie")
	request := SavedViewCreateRequest{
		Title:     "All movies by title",
		LibraryID: libraryID,
		Pivot:     "movies",
		Query:     &BrowseExpression{Field: "entityKind", Operator: "equals", Value: queryValue},
		Sort:      []BrowseSort{{Field: "title", Direction: "asc"}},
		IsPinned:  true,
	}
	var view SavedView
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/saved-views", request, &view)
	if status != http.StatusCreated || view.ID == "" || view.LibraryID != libraryID || view.Query == nil {
		t.Fatalf("create saved view status=%d body=%s view=%#v", status, body, view)
	}
	if !isOpaquePublicResourceID(view.ID) {
		t.Fatalf("saved view ID %q is not an opaque 160-bit identifier", view.ID)
	}
	var result BrowseLibraryResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/saved-views/"+view.ID+"/browse", SavedViewBrowseRequest{Limit: 2}, &result)
	if status != http.StatusOK || len(result.Items) == 0 {
		t.Fatalf("browse saved view status=%d body=%s result=%#v", status, body, result)
	}
	for _, item := range result.Items {
		if item.EntityKind != "movie" {
			t.Fatalf("saved view returned item outside persisted AST: %#v", item)
		}
	}
	var page SavedViewPage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/saved-views?libraryId="+libraryID+"&limit=1", nil, &page)
	if status != http.StatusOK || len(page.Items) != 1 || page.Items[0].ID != view.ID {
		t.Fatalf("list saved views status=%d body=%s page=%#v", status, body, page)
	}
}

func playlistEntryMediaCount(page PlaylistEntryPage, mediaID string) int {
	count := 0
	for _, item := range page.Items {
		if item.Media.ID == mediaID {
			count++
		}
	}
	return count
}
