package app

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestFavoritesEndpointIsIndependentFromWatchlist(t *testing.T) {
	serverURL, _ := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/favorite", map[string]bool{"favorite": true}, nil)
	if status != http.StatusOK {
		t.Fatalf("favorite mutation status=%d body=%s", status, body)
	}
	var favorites ListResponse[MediaItem]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/favorites?limit=10", nil, &favorites)
	if status != http.StatusOK {
		t.Fatalf("favorites status=%d body=%s", status, body)
	}
	found := false
	for _, item := range favorites.Items {
		if item.ID == "movie_meridian" {
			found = true
			if item.State.Watchlisted || !item.State.Favorite {
				t.Fatalf("favorites resource collapsed favorite into watchlist state: %#v", item.State)
			}
		}
	}
	if !found {
		t.Fatalf("favorite-only media was absent: %#v", favorites.Items)
	}
	var watchlist ListResponse[MediaItem]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/watchlist?limit=10", nil, &watchlist)
	if status != http.StatusOK {
		t.Fatalf("watchlist status=%d body=%s", status, body)
	}
	for _, item := range watchlist.Items {
		if item.ID == "movie_meridian" {
			t.Fatalf("favorite-only media leaked into watchlist: %#v", item)
		}
	}
}
