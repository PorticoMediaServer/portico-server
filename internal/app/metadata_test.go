package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

type metadataRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn metadataRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type metadataTrackingBody struct {
	reader     io.Reader
	closeCount int
}

func (body *metadataTrackingBody) Read(p []byte) (int, error) {
	return body.reader.Read(p)
}

func (body *metadataTrackingBody) Close() error {
	body.closeCount++
	return nil
}

type metadataBlockingBody struct {
	started   chan struct{}
	closed    chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func (body *metadataBlockingBody) Read([]byte) (int, error) {
	body.startOnce.Do(func() { close(body.started) })
	<-body.closed
	return 0, io.EOF
}

func (body *metadataBlockingBody) Close() error {
	body.closeOnce.Do(func() { close(body.closed) })
	return nil
}

type metadataErrorReader struct {
	err error
}

func (reader metadataErrorReader) Read([]byte) (int, error) { return 0, reader.err }

func TestDecodeProviderJSONResponseDecodesAndClosesBody(t *testing.T) {
	body := &metadataTrackingBody{reader: strings.NewReader(`{"results":[]}`)}
	var payload tmdbSearchResponse
	if err := decodeProviderJSONResponse(context.Background(), "TMDB", &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       body,
	}, maxMetadataProviderResponseBytes, &payload); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}
	if body.closeCount != 1 {
		t.Fatalf("provider response body close count = %d, expected 1", body.closeCount)
	}
	if payload.Results == nil {
		t.Fatalf("provider payload was not decoded: %#v", payload)
	}
}

func TestDecodeProviderJSONResponseRejectsOversizedBody(t *testing.T) {
	body := &metadataTrackingBody{reader: bytes.NewReader(bytes.Repeat([]byte("x"), int(maxProviderJSONResponseBytes)+1))}
	var payload tmdbSearchResponse
	err := decodeProviderJSONResponse(context.Background(), "TMDB", &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       body,
	}, maxMetadataProviderResponseBytes, &payload)
	if !errors.Is(err, errProviderResponseTooLarge) {
		t.Fatalf("oversized provider response error = %v", err)
	}
	if body.closeCount != 1 {
		t.Fatalf("oversized provider response body close count = %d, expected 1", body.closeCount)
	}
}

func TestDecodeProviderJSONResponseHonorsCancellationDuringSlowBodyRead(t *testing.T) {
	body := &metadataBlockingBody{started: make(chan struct{}), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		var payload tmdbSearchResponse
		result <- decodeProviderJSONResponse(ctx, "TMDB", &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       body,
		}, maxProviderJSONResponseBytes, &payload)
	}()
	select {
	case <-body.started:
	case <-time.After(time.Second):
		t.Fatal("provider body read did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("slow provider body cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("provider response did not honor body-read cancellation")
	}
}

func TestDecodeProviderJSONResponseHonorsDeadlineDuringSlowBodyRead(t *testing.T) {
	body := &metadataBlockingBody{started: make(chan struct{}), closed: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var payload tmdbSearchResponse
	err := decodeProviderJSONResponse(ctx, "TMDB", &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       body,
	}, maxProviderJSONResponseBytes, &payload)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow provider body deadline error = %v", err)
	}
}

func TestDecodeProviderJSONResponseClassifiesBodyAndHTTPFailures(t *testing.T) {
	bodyReadErr := io.ErrUnexpectedEOF
	tests := []struct {
		name       string
		response   *http.Response
		want       error
		statusCode int
	}{
		{
			name: "body read",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       &metadataTrackingBody{reader: metadataErrorReader{err: bodyReadErr}},
			},
			want: errProviderResponseBodyRead,
		},
		{
			name: "malformed json",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       &metadataTrackingBody{reader: strings.NewReader("{"), closeCount: 0},
			},
			want: errProviderResponseMalformed,
		},
		{
			name: "non-2xx",
			response: &http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
				Body:       &metadataTrackingBody{reader: strings.NewReader(`{"error":"upstream"}`)},
			},
			want:       errProviderHTTPStatus,
			statusCode: http.StatusBadGateway,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var payload tmdbSearchResponse
			err := decodeProviderJSONResponse(context.Background(), "TMDB", test.response, maxProviderJSONResponseBytes, &payload)
			if !errors.Is(err, test.want) {
				t.Fatalf("provider response error = %v, want classification %v", err, test.want)
			}
			if test.statusCode != 0 && providerResponseStatusCode(err) != test.statusCode {
				t.Fatalf("provider response status = %d, want %d", providerResponseStatusCode(err), test.statusCode)
			}
		})
	}
}

func TestMetadataRefreshUpdatesMediaFromTMDBSearch(t *testing.T) {
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEElEQVR4nGL6z8AACAAA//8DCQECWLbVUAAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	previousClient := providerArtworkHTTPClient
	providerArtworkHTTPClient = &http.Client{Transport: metadataRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "https" || req.URL.Host != "image.tmdb.org" {
			t.Fatalf("unexpected provider artwork request: %s", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(pngBytes)),
			Request:    req,
		}, nil
	})}
	defer func() { providerArtworkHTTPClient = previousClient }()

	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected TMDB bearer token")
		}
		if got := r.URL.Query().Get("api_key"); got != "test-api-key" {
			t.Fatalf("api_key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/movie":
			_, _ = w.Write([]byte(`{"results":[{"id":42,"title":"Meridian Drift","release_date":"2026-02-14","overview":"Updated TMDB overview.","vote_average":8.4,"genre_ids":[878,12],"poster_path":"/poster.jpg","backdrop_path":"/backdrop.jpg"}]}`))
		case "/movie/42":
			if got := r.URL.Query().Get("append_to_response"); got != "credits,alternative_titles,keywords,release_dates,content_ratings,external_ids,images" {
				t.Fatalf("append_to_response = %q", got)
			}
			_, _ = w.Write([]byte(`{"id":42,"title":"Meridian Drift","release_date":"2026-02-14","overview":"Updated TMDB overview.","vote_average":8.4,"genre_ids":[878,12],"poster_path":"/poster.jpg","backdrop_path":"/backdrop.jpg","credits":{"cast":[{"id":100,"name":"Ari Vega","character":"Captain Sol","profile_path":"/ari.jpg","order":0}],"crew":[{"id":200,"name":"Noor Patel","job":"Director","department":"Directing","profile_path":"/noor.jpg"}]}}`))
		default:
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-api-key", TMDBBaseURL: tmdb.URL})
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	if before.Summary == "Updated TMDB overview." {
		t.Fatalf("seed media already had test summary")
	}

	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh metadata: %v", err)
	}
	if after.Summary != "Updated TMDB overview." {
		t.Fatalf("summary = %q", after.Summary)
	}
	if after.CommunityRating != 8.4 {
		t.Fatalf("community rating = %v", after.CommunityRating)
	}
	if len(after.Genres) != 2 || after.Genres[0] != "Science Fiction" || after.Genres[1] != "Adventure" {
		t.Fatalf("genres = %#v", after.Genres)
	}
	if after.Artwork["source"] != "tmdb" || after.Artwork["posterPath"] != "/poster.jpg" || after.Artwork["backdropPath"] != "/backdrop.jpg" {
		t.Fatalf("artwork = %#v", after.Artwork)
	}
	detail, err := server.getMediaDetail("", after.ID)
	if err != nil {
		t.Fatalf("load refreshed detail: %v", err)
	}
	if !mediaImagesContainProviderCache(detail.MediaImages, "poster", "tmdb") || !mediaImagesContainProviderCache(detail.MediaImages, "backdrop", "tmdb") {
		t.Fatalf("provider media images = %#v", detail.MediaImages)
	}
	if remoteURL, ok := server.mediaImageRemoteURL(after.ID, "poster"); ok || remoteURL != "" {
		t.Fatalf("provider URL leaked through media image lookup: %q ok=%v", remoteURL, ok)
	}
	if !mediaPeopleContain(detail.People, "Actor", "Ari Vega") || !mediaPeopleContain(detail.People, "Director", "Noor Patel") {
		t.Fatalf("people = %#v", detail.People)
	}
	if detail.People[0].Character != "Captain Sol" ||
		!strings.HasPrefix(detail.People[0].ImageURL, "/api/people/") ||
		!strings.HasSuffix(detail.People[0].ImageURL, "/artwork") {
		t.Fatalf("person image/character = %#v", detail.People[0])
	}
}

func TestMetadataAgentsCacheOriginalArtworkStoresProviderImages(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	if _, err := db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'metadataAgents'`, `{"movies":"TMDB","tv":"TMDB","anime":"AniList","music":"MusicBrainz","localNFO":true,"embeddedTags":true,"cacheOriginalArtwork":true,"refreshDays":7,"metadataLanguage":"en-US"}`); err != nil {
		t.Fatalf("enable artwork cache: %v", err)
	}
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEElEQVR4nGL6z8AACAAA//8DCQECWLbVUAAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	previousClient := providerArtworkHTTPClient
	imageRequests := 0
	providerArtworkHTTPClient = &http.Client{Transport: metadataRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		imageRequests++
		if req.URL.Scheme != "https" || req.URL.Host != "image.tmdb.org" || req.URL.Path != "/t/p/w780/cached-poster.png" {
			t.Fatalf("unexpected provider artwork request: %s", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(pngBytes)),
			Request:    req,
		}, nil
	})}
	defer func() { providerArtworkHTTPClient = previousClient }()

	if err := server.replaceProviderMediaImages("movie_meridian", map[string]string{"source": "tmdb", "posterPath": "/cached-poster.png"}); err != nil {
		t.Fatalf("replace provider images: %v", err)
	}
	if imageRequests != 1 {
		t.Fatalf("provider image requests = %d, expected 1", imageRequests)
	}
	detail, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media detail: %v", err)
	}
	var poster MediaImage
	for _, image := range detail.MediaImages {
		if image.Type == "poster" && image.Source == "provider" && image.Provider == "tmdb" {
			poster = image
			break
		}
	}
	if poster.Path == "" || poster.RemoteURL != "" || filepath.Ext(poster.Path) != ".png" {
		t.Fatalf("cached poster image not stored correctly: %#v", poster)
	}
	stored, err := os.ReadFile(poster.Path)
	if err != nil {
		t.Fatalf("read cached poster: %v", err)
	}
	if !bytes.Equal(stored, pngBytes) {
		t.Fatalf("cached poster bytes changed")
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	resp, err := client.Get(serverURL + "/api/artwork/movie_meridian/poster.svg")
	if err != nil {
		t.Fatalf("load cached artwork route: %v", err)
	}
	served, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(served, pngBytes) {
		t.Fatalf("cached artwork route status=%d bytes=%x", resp.StatusCode, served[:min(len(served), 8)])
	}
	if imageRequests != 1 {
		t.Fatalf("cached artwork route should not re-fetch provider image, requests=%d", imageRequests)
	}
}

func TestMetadataRefreshUpdatesAnimeFromAniList(t *testing.T) {
	aniListThrottleMu.Lock()
	aniListLastRequest = time.Time{}
	aniListThrottleMu.Unlock()
	anilist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("AniList method = %s", r.Method)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Fatalf("AniList request should include a User-Agent")
		}
		var payload struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode AniList request: %v", err)
		}
		if payload.Variables["search"] != "Star Rail" {
			t.Fatalf("AniList search variable = %#v", payload.Variables)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"Page":{"media":[{
			"id":151970,
			"idMal":52991,
			"title":{"romaji":"Hoshikuzu Rail","english":"Star Rail","native":"星屑レール"},
			"description":"A found-family train crew.<br>Across the stars.",
			"format":"TV",
			"status":"RELEASING",
			"episodes":12,
			"duration":24,
			"season":"SPRING",
			"seasonYear":2026,
			"averageScore":82,
			"isAdult":false,
			"genres":["Adventure","Sci-Fi"],
			"tags":[{"name":"Space","rank":88}],
			"coverImage":{"extraLarge":"https://img.anili.st/media/151970.jpg","large":"https://img.anili.st/media/151970-large.jpg","medium":"https://img.anili.st/media/151970-med.jpg"},
			"bannerImage":"https://img.anili.st/media/151970-banner.jpg",
			"studios":{"nodes":[{"id":44,"name":"North Harbor Animation"}]},
			"staff":{"edges":[{"role":"Director","node":{"id":20,"name":{"full":"Noor Patel","native":""},"image":{"large":"https://img.anili.st/staff/20.jpg","medium":""}}}]},
			"characters":{"edges":[{"role":"MAIN","node":{"id":10,"name":{"full":"Conductor","native":""},"image":{"large":"","medium":""}},"voiceActors":[{"id":30,"name":{"full":"Ari Vega","native":""},"image":{"large":"https://img.anili.st/staff/30.jpg","medium":""}}]}]}
		}]}}}`))
	}))
	defer anilist.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{AniListBaseURL: anilist.URL})
	before, err := server.getMediaDetail("", "anime_starrail")
	if err != nil {
		t.Fatalf("load anime: %v", err)
	}
	if _, err := server.db.Exec(`DELETE FROM media_provider_ids WHERE media_id = ? AND provider = 'anilist'`, before.ID); err != nil {
		t.Fatalf("clear seeded AniList id: %v", err)
	}
	before.Title = "Star Rail"
	after, err := server.refreshMediaMetadataFromAniList(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh AniList metadata: %v", err)
	}
	if after.Title != "Star Rail" || after.OriginalTitle != "Hoshikuzu Rail" || !strings.Contains(after.Summary, "Across the stars.") || after.Year != 2026 {
		t.Fatalf("anime metadata = %#v", after)
	}
	if after.CommunityRating != 8.2 || after.Studio != "North Harbor Animation" {
		t.Fatalf("anime rating/studio = %#v", after)
	}
	if len(after.Genres) != 2 || after.Genres[0] != "Adventure" || after.Genres[1] != "Sci-Fi" {
		t.Fatalf("anime genres = %#v", after.Genres)
	}
	if after.TypedMetadata["format"] != "TV" || after.TypedMetadata["status"] != "RELEASING" || after.TypedMetadata["episodes"] != "12" || after.TypedMetadata["anilistID"] != "151970" {
		t.Fatalf("anime typed metadata = %#v", after.TypedMetadata)
	}
	if id, ok := server.mediaProviderID(after.ID, "anilist", "anime"); !ok || id != "151970" {
		t.Fatalf("AniList provider id = %q ok=%v", id, ok)
	}
	if id, ok := server.mediaProviderID(after.ID, "mal", "anime"); !ok || id != "52991" {
		t.Fatalf("MAL provider id = %q ok=%v", id, ok)
	}
	detail, err := server.getMediaDetail("", after.ID)
	if err != nil {
		t.Fatalf("load refreshed detail: %v", err)
	}
	for _, image := range detail.MediaImages {
		if image.RemoteURL != "" {
			t.Fatalf("AniList provider URL escaped the local artwork cache: %#v", detail.MediaImages)
		}
	}
	if !mediaPeopleContain(detail.People, "Voice", "Ari Vega") || !mediaPeopleContain(detail.People, "Director", "Noor Patel") {
		t.Fatalf("AniList people = %#v", detail.People)
	}
}

func TestPersonMediaReturnsMoviesShowsAndAnimeFromCredits(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE role = 'owner' LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("load owner: %v", err)
	}
	if err := server.replaceProviderMediaPeople("movie_meridian", []MediaPerson{{Name: "Ari Vega", Role: "Actor", Character: "Mara"}}, "tmdb"); err != nil {
		t.Fatalf("seed movie person: %v", err)
	}
	if err := server.replaceProviderMediaPeople("episode_northbridge_101", []MediaPerson{{Name: "Ari Vega", Role: "Actor", Character: "Detective Vale"}}, "tmdb"); err != nil {
		t.Fatalf("seed episode person: %v", err)
	}
	if err := server.replaceProviderMediaPeople("anime_starrail_101", []MediaPerson{{Name: "Ari Vega", Role: "Voice", Character: "Conductor"}}, "tmdb"); err != nil {
		t.Fatalf("seed anime person: %v", err)
	}

	items, err := server.personMedia(userID, "Ari Vega", 20)
	if err != nil {
		t.Fatalf("person media: %v", err)
	}
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
		if item.Type != "movie" && item.Type != "show" && item.Type != "anime" {
			t.Fatalf("unexpected person media item type: %#v", item)
		}
	}
	for _, id := range []string{"movie_meridian", "show_northbridge", "anime_starrail"} {
		if !ids[id] {
			t.Fatalf("expected %s in person media results: %#v", id, items)
		}
	}
}

func TestTVSeasonAndEpisodeDetailsExposeCastAndCrew(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	if err := server.replaceProviderMediaPeople("show_northbridge", []MediaPerson{{Name: "Mara Cho", Role: "Actor", Character: "Chief"}}, "tmdb"); err != nil {
		t.Fatalf("seed show person: %v", err)
	}
	if err := server.replaceProviderMediaPeople("season_northbridge_1", []MediaPerson{{Name: "Noor Patel", Role: "Director"}}, "tmdb"); err != nil {
		t.Fatalf("seed season person: %v", err)
	}
	if err := server.replaceProviderMediaPeople("episode_northbridge_101", []MediaPerson{{Name: "Ari Vega", Role: "Actor", Character: "Detective Vale"}}, "tmdb"); err != nil {
		t.Fatalf("seed episode person: %v", err)
	}

	season, err := server.getMediaDetail("", "season_northbridge_1")
	if err != nil {
		t.Fatalf("load season: %v", err)
	}
	if !mediaPeopleContain(season.People, "Actor", "Ari Vega") || !mediaPeopleContain(season.People, "Actor", "Mara Cho") || !mediaPeopleContain(season.People, "Director", "Noor Patel") {
		t.Fatalf("season people = %#v", season.People)
	}

	episode, err := server.getMediaDetail("", "episode_northbridge_102")
	if err != nil {
		t.Fatalf("load episode: %v", err)
	}
	if !mediaPeopleContain(episode.People, "Actor", "Mara Cho") || !mediaPeopleContain(episode.People, "Director", "Noor Patel") {
		t.Fatalf("episode people = %#v", episode.People)
	}
}

func TestProviderPeoplePublishMediaDataChange(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	events := server.subscribeAppEvents()
	defer server.unsubscribeAppEvents(events)

	if err := server.replaceProviderMediaPeople("movie_meridian", []MediaPerson{{Name: "Ari Vega", Role: "Actor"}}, "tmdb"); err != nil {
		t.Fatalf("replace provider people: %v", err)
	}

	select {
	case event := <-events:
		if !stringSliceContains(event.Tags, "media") || event.Resource != "media" || event.ResourceID != "movie_meridian" {
			t.Fatalf("unexpected event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for media data-change event")
	}
}

func TestMetadataRefreshUpdatesTrackFromMusicBrainz(t *testing.T) {
	musicBrainzThrottleMu.Lock()
	musicBrainzLastRequest = time.Time{}
	musicBrainzThrottleMu.Unlock()
	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/recording" {
			t.Fatalf("unexpected MusicBrainz path: %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Fatalf("MusicBrainz request should include a User-Agent")
		}
		if got := r.URL.Query().Get("fmt"); got != "json" {
			t.Fatalf("fmt = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"recordings":[{
			"id":"rec-123",
			"title":"Platform Lights (Remastered)",
			"length":245000,
			"artist-credit":[{"name":"Mara Vale","artist":{"id":"artist-123","name":"Mara Vale"}}],
			"releases":[{"id":"release-123","title":"Late Trains for Bright Cities","date":"2024-03-01","release-group":{"id":"rg-123","title":"Late Trains for Bright Cities"}}],
			"genres":[{"name":"electronic","count":4},{"name":"ambient","count":2}]
		}]}`))
	}))
	defer mb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{MusicBrainzBaseURL: mb.URL})
	if _, err := server.db.Exec(`DELETE FROM media_provider_ids WHERE media_id = 'album_mara' AND provider = 'musicbrainz'`); err != nil {
		t.Fatalf("clear album provider ids: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET artwork_json = '{}', typed_metadata_json = '{}' WHERE id = 'album_mara'`); err != nil {
		t.Fatalf("clear album metadata: %v", err)
	}
	before, err := server.getMediaDetail("", "track_mara_01")
	if err != nil {
		t.Fatalf("load track: %v", err)
	}
	after, err := server.refreshMediaMetadataFromMusicBrainz(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh music metadata: %v", err)
	}
	if after.Title != "Platform Lights (Remastered)" || after.Studio != "Mara Vale" {
		t.Fatalf("track metadata = %#v", after)
	}
	if after.TypedMetadata["trackArtist"] != "Mara Vale" || after.TypedMetadata["albumArtist"] != "Mara Vale" || after.TypedMetadata["albumTitle"] != "Late Trains for Bright Cities" || after.TypedMetadata["recordingID"] != "rec-123" {
		t.Fatalf("typed track metadata = %#v", after.TypedMetadata)
	}
	if len(after.Genres) != 2 || after.Genres[0] != "Electronic" || after.Genres[1] != "Ambient" {
		t.Fatalf("genres = %#v", after.Genres)
	}
	if id, ok := server.mediaProviderID(after.ID, "musicbrainz", "recording"); !ok || id != "rec-123" {
		t.Fatalf("recording provider id = %q ok=%v", id, ok)
	}
	if id, ok := server.mediaProviderID("album_mara", "musicbrainz", "release-group"); !ok || id != "rg-123" {
		t.Fatalf("album provider id = %q ok=%v", id, ok)
	}
	albums, err := server.queryMedia("", `WHERE m.id = ?`, []any{"album_mara"})
	if err != nil {
		t.Fatalf("load album: %v", err)
	}
	if len(albums) != 1 || albums[0].Artwork["source"] != "musicbrainz" || albums[0].Artwork["releaseID"] != "release-123" || albums[0].Artwork["releaseGroupID"] != "rg-123" {
		t.Fatalf("album artwork = %#v", albums)
	}
	if albums[0].TypedMetadata["albumArtist"] != "Mara Vale" || albums[0].TypedMetadata["albumTitle"] != "Late Trains for Bright Cities" || albums[0].TypedMetadata["releaseGroupID"] != "rg-123" {
		t.Fatalf("typed album metadata = %#v", albums[0].TypedMetadata)
	}
	if url, ok := providerArtworkURL(albums[0].Artwork, "poster"); !ok || url != "https://coverartarchive.org/release/release-123/front" {
		t.Fatalf("cover art url = %q ok=%v", url, ok)
	}
}

func TestMetadataRefreshTrackUsesReleaseArtistAndDoesNotRewriteEstablishedAlbum(t *testing.T) {
	musicBrainzThrottleMu.Lock()
	musicBrainzLastRequest = time.Time{}
	musicBrainzThrottleMu.Unlock()
	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/recording" {
			t.Fatalf("unexpected MusicBrainz path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"recordings":[{
			"id":"rec-featured",
			"title":"Platform Lights",
			"length":245000,
			"artist-credit":[
				{"name":"Mara Vale","artist":{"id":"artist-mara","name":"Mara Vale"}},
				{"name":"Guest Rapper","artist":{"id":"artist-guest","name":"Guest Rapper"}}
			],
			"releases":[{
				"id":"release-featured",
				"title":"Late Trains for Bright Cities",
				"date":"2026-05-01",
				"artist-credit":[{"name":"Mara Vale","artist":{"id":"artist-mara","name":"Mara Vale"}}],
				"release-group":{"id":"rg-featured","title":"Late Trains for Bright Cities"}
			}]
		}]}`))
	}))
	defer mb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{MusicBrainzBaseURL: mb.URL})
	if err := server.upsertMediaProviderID("album_mara", "musicbrainz", "rg-established", "release-group", 1, "test"); err != nil {
		t.Fatalf("seed album release-group: %v", err)
	}
	if _, err := server.db.Exec(`
		UPDATE media_items
		SET studio = 'Mara Vale',
			year = 2024,
			typed_metadata_json = '{"albumArtist":"Mara Vale","albumTitle":"Late Trains for Bright Cities","releaseGroupID":"rg-established"}',
			artwork_json = '{"source":"musicbrainz","releaseGroupID":"rg-established","releaseID":"release-established"}'
		WHERE id = 'album_mara'`); err != nil {
		t.Fatalf("seed established album metadata: %v", err)
	}
	before, err := server.getMediaDetail("", "track_mara_01")
	if err != nil {
		t.Fatalf("load track: %v", err)
	}
	after, err := server.refreshMediaMetadataFromMusicBrainz(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh track: %v", err)
	}
	if after.Studio != "Mara Vale, Guest Rapper" || after.TypedMetadata["trackArtist"] != "Mara Vale, Guest Rapper" {
		t.Fatalf("track artist metadata = studio %q typed %#v", after.Studio, after.TypedMetadata)
	}
	if after.TypedMetadata["albumArtist"] != "Mara Vale" {
		t.Fatalf("album artist should come from release artist-credit, typed = %#v", after.TypedMetadata)
	}
	if id, ok := server.mediaProviderID("album_mara", "musicbrainz", "release-group"); !ok || id != "rg-established" {
		t.Fatalf("album release-group was overwritten: %q ok=%v", id, ok)
	}
	albums, err := server.queryMedia("", `WHERE m.id = ?`, []any{"album_mara"})
	if err != nil {
		t.Fatalf("load album: %v", err)
	}
	if len(albums) != 1 || albums[0].Artwork["releaseGroupID"] != "rg-established" || albums[0].Artwork["releaseID"] != "release-established" || albums[0].Year != 2024 {
		t.Fatalf("established album metadata was changed: %#v", albums)
	}
}

func TestFetchLRCLibLyricsUsesTypedTrackMetadata(t *testing.T) {
	lrclib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/get" {
			t.Fatalf("unexpected LRCLIB path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("track_name") != "Platform Lights" || r.URL.Query().Get("artist_name") != "Mara Vale" || r.URL.Query().Get("album_name") != "Late Trains for Bright Cities" || r.URL.Query().Get("duration") != "244" {
			t.Fatalf("unexpected LRCLIB query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"id":123,"trackName":"Platform Lights","artistName":"Mara Vale","albumName":"Late Trains for Bright Cities","syncedLyrics":"[00:01.00]Remote line","plainLyrics":"Remote line"}`))
	}))
	defer lrclib.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{LRCLibBaseURL: lrclib.URL})
	item, err := server.getMediaDetail("", "track_mara_01")
	if err != nil {
		t.Fatalf("load track: %v", err)
	}
	lyric, err := server.fetchLRCLibLyrics(context.Background(), item)
	if err != nil {
		t.Fatalf("fetch LRCLIB lyrics: %v", err)
	}
	if lyric.Format != "lrc" || !lyric.Synced || lyric.Path != "lrclib:123" || !strings.Contains(lyric.Text, "Remote line") {
		t.Fatalf("lyric = %#v", lyric)
	}
}

func TestFetchMissingLyricsForLibrarySkipsTracksWithLyrics(t *testing.T) {
	requests := 0
	lrclib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/get" {
			t.Fatalf("unexpected LRCLIB path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("track_name") != "No Lyrics Yet" || r.URL.Query().Get("artist_name") != "Mara Vale" {
			t.Fatalf("unexpected LRCLIB query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"id":456,"trackName":"No Lyrics Yet","artistName":"Mara Vale","albumName":"Late Trains for Bright Cities","plainLyrics":"Fetched plain lyric"}`))
	}))
	defer lrclib.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{LRCLibBaseURL: lrclib.URL})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO media_items (
			id, library_id, parent_id, type, title, sort_title, duration_seconds, genres_json, typed_metadata_json, source_url, added_at
		) VALUES (
			'track_missing_lyrics', 'lib_music', 'album_mara', 'track', 'No Lyrics Yet', 'No Lyrics Yet', 201, '[]',
			'{"trackArtist":"Mara Vale","albumTitle":"Late Trains for Bright Cities"}', '/music/no-lyrics-yet.mp3', ?
		)`, now); err != nil {
		t.Fatalf("insert track: %v", err)
	}
	result, err := server.fetchMissingLyricsForLibrary(context.Background(), "lib_music", "")
	if err != nil {
		t.Fatalf("fetch missing lyrics: %v", err)
	}
	if result.Fetched != 1 || result.Skipped != 0 || result.Failed != 0 || requests != 1 {
		t.Fatalf("result = %#v requests=%d", result, requests)
	}
	var text string
	if err := server.db.QueryRow(`SELECT text FROM media_lyrics WHERE media_id = ? AND provider = 'lrclib'`, "track_missing_lyrics").Scan(&text); err != nil {
		t.Fatalf("load saved lyrics: %v", err)
	}
	if !strings.Contains(text, "Fetched plain lyric") {
		t.Fatalf("lyrics text = %q", text)
	}
}

func TestSearchAndApplyLRCLibLyricsCandidate(t *testing.T) {
	lrclib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search":
			if r.URL.Query().Get("q") == "" {
				t.Fatalf("missing LRCLIB search query")
			}
			_, _ = w.Write([]byte(`[{"id":789,"trackName":"Platform Lights","artistName":"Mara Vale","albumName":"Late Trains for Bright Cities","duration":244,"syncedLyrics":"[00:01.00]Candidate line"}]`))
		case "/api/get/789":
			_, _ = w.Write([]byte(`{"id":789,"trackName":"Platform Lights","artistName":"Mara Vale","albumName":"Late Trains for Bright Cities","syncedLyrics":"[00:01.00]Applied candidate line"}`))
		default:
			t.Fatalf("unexpected LRCLIB path: %s", r.URL.Path)
		}
	}))
	defer lrclib.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{LRCLibBaseURL: lrclib.URL})
	item, err := server.getMediaDetail("", "track_mara_01")
	if err != nil {
		t.Fatalf("load track: %v", err)
	}
	candidates, err := server.searchLRCLibLyrics(context.Background(), item, "Platform Lights Mara Vale")
	if err != nil {
		t.Fatalf("search lyrics: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ExternalID != "789" || !candidates[0].Synced || candidates[0].Format != "lrc" {
		t.Fatalf("candidates = %#v", candidates)
	}
	lyric, err := server.fetchLRCLibLyricsByID(context.Background(), candidates[0].ExternalID)
	if err != nil {
		t.Fatalf("fetch lyric by id: %v", err)
	}
	if err := server.saveLRCLibLyrics(item.ID, lyric); err != nil {
		t.Fatalf("save lyric: %v", err)
	}
	var text string
	if err := server.db.QueryRow(`SELECT text FROM media_lyrics WHERE media_id = ? AND provider = 'lrclib'`, item.ID).Scan(&text); err != nil {
		t.Fatalf("load applied lyric: %v", err)
	}
	if !strings.Contains(text, "Applied candidate line") {
		t.Fatalf("applied lyric text = %q", text)
	}
}

func TestMetadataRefreshUpdatesAlbumArtworkFromMusicBrainzReleaseGroup(t *testing.T) {
	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/release-group" {
			t.Fatalf("unexpected MusicBrainz path: %s", r.URL.Path)
		}
		if !strings.Contains(r.URL.Query().Get("inc"), "releases") {
			t.Fatalf("release-group lookup should request releases, inc=%q", r.URL.Query().Get("inc"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"release-groups":[{
			"id":"7999d30a-30d6-3cf4-a151-bb2c70f64f7d",
			"title":"United We Fall",
			"first-release-date":"2005",
			"artist-credit":[{"name":"Sweatshop Union","artist":{"id":"artist-su","name":"Sweatshop Union"}}],
			"releases":[{"id":"c45d87c7-3ea4-4614-8c73-007175e4cd7b","title":"United We Fall"}],
			"tags":[{"name":"hip hop","count":1}]
		}]}`))
	}))
	defer mb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{MusicBrainzBaseURL: mb.URL})
	if _, err := server.db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_music_test', 'Music Test', 'music', 500, '/tmp/music-test', '{}', ?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	item := MediaItem{ID: "album_su", LibraryID: "lib_music_test", Type: "album", Title: "United We Fall", SortTitle: "United We Fall", ParentTitle: "Sweatshop Union"}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at)
		VALUES (?, ?, ?, ?, ?, ?)`, item.ID, item.LibraryID, item.Type, item.Title, item.SortTitle, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert album: %v", err)
	}
	updated, err := server.refreshMediaMetadataFromMusicBrainz(context.Background(), item)
	if err != nil {
		t.Fatalf("refresh album metadata: %v", err)
	}
	if updated.Artwork["source"] != "musicbrainz" || updated.Artwork["releaseID"] != "c45d87c7-3ea4-4614-8c73-007175e4cd7b" || updated.Artwork["releaseGroupID"] != "7999d30a-30d6-3cf4-a151-bb2c70f64f7d" {
		t.Fatalf("album artwork = %#v", updated.Artwork)
	}
	if url, ok := providerArtworkURL(updated.Artwork, "poster"); !ok || url != "https://coverartarchive.org/release/c45d87c7-3ea4-4614-8c73-007175e4cd7b/front" {
		t.Fatalf("cover art url = %q ok=%v", url, ok)
	}
}

func TestMetadataRefreshUsesPersistedTMDBIDBeforeSearch(t *testing.T) {
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/movie" {
			t.Fatalf("metadata refresh should not search when provider id is known")
		}
		if r.URL.Path != "/movie/42" {
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"title":"The Meridian Job","release_date":"2025-02-14","overview":"Resolved by persisted provider ID.","vote_average":7.6,"genre_ids":[18,28],"poster_path":"/meridian.jpg"}`))
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-api-key", TMDBBaseURL: tmdb.URL})
	if err := server.upsertMediaProviderID("movie_meridian", "tmdb", "42", "movie", 1, "test"); err != nil {
		t.Fatalf("upsert provider id: %v", err)
	}
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh metadata: %v", err)
	}
	if after.Title != "The Meridian Job" || after.Summary != "Resolved by persisted provider ID." {
		t.Fatalf("metadata = %#v", after)
	}
}

func TestMetadataRefreshCanUseConfiguredTMDBAPIKey(t *testing.T) {
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected TMDB bearer token")
		}
		if got := r.URL.Query().Get("api_key"); got != "configured-api-key" {
			t.Fatalf("api_key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":42,"title":"Meridian Drift","release_date":"2026-02-14","overview":"Updated from API key."}]}`))
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBBaseURL: tmdb.URL})
	if err := server.saveSecretSetting(tmdbAPIKeySettingKey, "configured-api-key"); err != nil {
		t.Fatalf("save TMDB API key: %v", err)
	}
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh metadata: %v", err)
	}
	if after.Summary != "Updated from API key." {
		t.Fatalf("summary = %q", after.Summary)
	}
}

func TestMetadataRefreshCanUseConfiguredTMDBReadAccessToken(t *testing.T) {
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "" {
			t.Fatalf("unexpected TMDB api_key query")
		}
		if r.Header.Get("Authorization") != "Bearer configured-read-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":42,"title":"Meridian Drift","release_date":"2026-02-14","overview":"Updated from read token."}]}`))
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBBaseURL: tmdb.URL})
	if err := server.saveSecretSetting(tmdbReadAccessTokenSettingKey, "configured-read-token"); err != nil {
		t.Fatalf("save TMDB read access token: %v", err)
	}
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh metadata: %v", err)
	}
	if after.Summary != "Updated from read token." {
		t.Fatalf("summary = %q", after.Summary)
	}
}

func TestMetadataRefreshUsesRuntimeOwnerTMDBReadAccessToken(t *testing.T) {
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer runtime-owner-read-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if r.URL.Query().Get("api_key") != "" {
			t.Fatalf("unexpected TMDB api_key query")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":42,"title":"Meridian Drift","release_date":"2026-02-14","overview":"Updated from runtime owner credential."}]}`))
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{
		TMDBReadAccessToken: "runtime-owner-read-token",
		TMDBBaseURL:         tmdb.URL,
	})
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh metadata: %v", err)
	}
	if after.Summary != "Updated from runtime owner credential." {
		t.Fatalf("summary = %q", after.Summary)
	}
}

func TestMetadataRefreshRequiresOwnerSuppliedTMDBCredentials(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	if _, err := server.refreshMediaMetadataFromTMDB(context.Background(), before); !errors.Is(err, errTMDBCredentialsMissing) {
		t.Fatalf("refresh without owner credentials error = %v", err)
	}
}

func TestMetadataRefreshRetriesSimplifiedTMDBMovieTitle(t *testing.T) {
	var queries []string
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/movie":
			query := r.URL.Query().Get("query")
			queries = append(queries, query)
			if query == "F1" {
				_, _ = w.Write([]byte(`{"results":[{"id":911430,"title":"F1","release_date":"2025-06-25","overview":"Racing drama.","poster_path":"/f1-poster.jpg","backdrop_path":"/f1-backdrop.jpg"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"results":[]}`))
		case "/movie/911430":
			_, _ = w.Write([]byte(`{"id":911430,"title":"F1","release_date":"2025-06-25","overview":"Racing drama.","poster_path":"/f1-poster.jpg","backdrop_path":"/f1-backdrop.jpg"}`))
		default:
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-api-key", TMDBBaseURL: tmdb.URL})
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	before.Title = "F1 The Movie 1080p BluRay x264"
	before.OriginalTitle = ""
	before.SourceURL = ""
	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh metadata: %v", err)
	}
	if after.Title != "F1" {
		t.Fatalf("title = %q", after.Title)
	}
	if len(queries) < 2 || queries[0] != "F1 The Movie" || queries[1] != "F1" {
		t.Fatalf("queries = %#v", queries)
	}
	if after.Artwork["source"] != "tmdb" || after.Artwork["posterPath"] != "/f1-poster.jpg" || after.Artwork["backdropPath"] != "/f1-backdrop.jpg" {
		t.Fatalf("artwork = %#v", after.Artwork)
	}
}

func TestMetadataRefreshRanksTMDBCandidatesByTitleYearAndPopularity(t *testing.T) {
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/movie":
			_, _ = w.Write([]byte(`{"results":[
				{"id":1239561,"title":"Waffle F1 - The Movie","release_date":"2022-01-01","popularity":0.4},
				{"id":911430,"title":"F1 The Movie","release_date":"2025-06-25","overview":"Racing drama.","popularity":350.0,"poster_path":"/f1-poster.jpg"}
			]}`))
		case "/movie/911430":
			_, _ = w.Write([]byte(`{"id":911430,"title":"F1 The Movie","release_date":"2025-06-25","overview":"Racing drama.","popularity":350.0,"poster_path":"/f1-poster.jpg"}`))
		default:
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-api-key", TMDBBaseURL: tmdb.URL})
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	before.Title = "F1 The Movie"
	before.Year = 2025
	before.SourceURL = "/Movies/F1.The.Movie.2025.HYBRID.1080p.BluRay.x264.mp4"
	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh metadata: %v", err)
	}
	if after.Title != "F1 The Movie" || after.Year != 2025 {
		t.Fatalf("matched wrong candidate: title=%q year=%d", after.Title, after.Year)
	}
	var externalID string
	if err := server.db.QueryRow(`SELECT external_id FROM media_provider_ids WHERE media_id = ? AND provider = 'tmdb' AND external_type = 'movie'`, before.ID).Scan(&externalID); err != nil {
		t.Fatalf("provider id: %v", err)
	}
	if externalID != "911430" {
		t.Fatalf("external id = %s, expected 911430", externalID)
	}
	var accepted int
	var reasons string
	if err := server.db.QueryRow(`
		SELECT CASE WHEN status = 'accepted' THEN 1 ELSE 0 END, reason_codes_json
		FROM media_match_candidates
		WHERE media_id = ? AND provider = 'tmdb' AND external_id = '911430'
		ORDER BY created_at DESC LIMIT 1`, before.ID).Scan(&accepted, &reasons); err != nil {
		t.Fatalf("match candidate evidence: %v", err)
	}
	if accepted != 1 || !strings.Contains(reasons, "title_exact") || !strings.Contains(reasons, "year_exact") || !strings.Contains(reasons, "popular_candidate") {
		t.Fatalf("candidate evidence accepted=%d reasons=%s", accepted, reasons)
	}
}

func TestMetadataRefreshRespectsLockedFields(t *testing.T) {
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/movie":
			_, _ = w.Write([]byte(`{"results":[{"id":42,"title":"Human Title","release_date":"2026-02-14","overview":"Provider summary.","vote_average":8.4,"genre_ids":[878],"poster_path":"/provider.jpg"}]}`))
		case "/movie/42":
			_, _ = w.Write([]byte(`{"id":42,"title":"Human Title","release_date":"2026-02-14","overview":"Provider summary.","vote_average":8.4,"genre_ids":[878],"poster_path":"/provider.jpg"}`))
		default:
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-api-key", TMDBBaseURL: tmdb.URL})
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	before.Title = "Human Title"
	before.Summary = "Human summary."
	before.Year = 1999
	if _, err := server.db.Exec(`UPDATE media_items SET title = ?, sort_title = ?, summary = ?, year = ? WHERE id = ?`, before.Title, before.SortTitle, before.Summary, before.Year, before.ID); err != nil {
		t.Fatalf("seed manual metadata: %v", err)
	}
	if err := server.replaceMetadataLocks(before.ID, []string{"title", "summary", "year"}, "test-user"); err != nil {
		t.Fatalf("lock fields: %v", err)
	}
	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh metadata: %v", err)
	}
	if after.Title != "Human Title" || after.Summary != "Human summary." || after.Year != 1999 {
		t.Fatalf("locked fields changed: title=%q summary=%q year=%d", after.Title, after.Summary, after.Year)
	}
	if after.CommunityRating != 8.4 {
		t.Fatalf("unlocked rating was not refreshed: %v", after.CommunityRating)
	}
}

func TestMetadataUpdateRespectsIndividualTypedMetadataLocks(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	before, err := server.getMediaDetail("", "track_mara_01")
	if err != nil {
		t.Fatalf("load track: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET typed_metadata_json = ? WHERE id = ?`, `{"trackArtist":"Mara Vale","albumArtist":"Mara Vale","albumTitle":"Late Trains for Bright Cities"}`, before.ID); err != nil {
		t.Fatalf("seed typed metadata: %v", err)
	}
	if err := server.replaceMetadataLocks(before.ID, []string{"typedMetadata.albumArtist"}, "test-user"); err != nil {
		t.Fatalf("lock typed metadata: %v", err)
	}
	nextMetadata := map[string]string{
		"trackArtist": "New Track Artist",
		"albumArtist": "Provider Album Artist",
		"albumTitle":  "New Album Title",
	}
	after, err := server.updateMediaForMetadata("", before.ID, UpdateMediaRequest{TypedMetadata: &nextMetadata})
	if err != nil {
		t.Fatalf("update media: %v", err)
	}
	if after.TypedMetadata["albumArtist"] != "Mara Vale" {
		t.Fatalf("locked album artist changed: %#v", after.TypedMetadata)
	}
	if after.TypedMetadata["trackArtist"] != "New Track Artist" || after.TypedMetadata["albumTitle"] != "New Album Title" {
		t.Fatalf("unlocked typed metadata was not updated: %#v", after.TypedMetadata)
	}
}

func TestMetadataUpdateRespectsPeopleLock(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	mediaID := "track_mara_01"
	original := []MediaPerson{{Name: "Mara Vale", Role: "Actor", Character: "Captain Sol"}}
	if err := server.replaceManualMediaPeople(mediaID, original); err != nil {
		t.Fatalf("seed manual people: %v", err)
	}
	if err := server.replaceMetadataLocks(mediaID, []string{"people"}, "test-user"); err != nil {
		t.Fatalf("lock people: %v", err)
	}
	providerPeople := []MediaPerson{{Name: "Replacement Person", Role: "Actor", Character: "Someone Else"}}
	_, err := server.updateMediaForMetadata("", mediaID, UpdateMediaRequest{People: &providerPeople})
	if err != nil {
		t.Fatalf("apply scanner metadata: %v", err)
	}
	updated, err := server.getMediaDetail("", mediaID)
	if err != nil {
		t.Fatalf("reload media credits: %v", err)
	}
	if !mediaPeopleContain(updated.People, "Actor", "Mara Vale") || mediaPeopleContain(updated.People, "Actor", "Replacement Person") {
		t.Fatalf("people lock did not preserve manual credits: %#v", updated.People)
	}
	for _, person := range updated.People {
		if person.Name == "Mara Vale" && person.Character != "Captain Sol" {
			t.Fatalf("manual character was not preserved: %#v", person)
		}
	}
}

func TestMediaVisibilityRestrictionKeepsTopLevelLimitSeparated(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	clause, _ := server.applyMediaVisibilityRestrictionToClause("missing-profile", "WHERE m.id = ? LIMIT 1", []any{"media-id"})
	if strings.Contains(clause, "0LIMIT") || !strings.Contains(clause, "1 = 0 LIMIT 1") {
		t.Fatalf("visibility restriction merged with LIMIT clause: %q", clause)
	}
}

func TestTypedMetadataIsLimitedToMediaType(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	movieMetadata := map[string]string{
		"trackArtist": "Wrong Artist",
		"author":      "Wrong Author",
		"network":     "Wrong Network",
	}
	movie, err := server.updateMedia("", "movie_meridian", UpdateMediaRequest{TypedMetadata: &movieMetadata})
	if err != nil {
		t.Fatalf("update movie: %v", err)
	}
	if len(movie.TypedMetadata) != 0 {
		t.Fatalf("movie typed metadata should be empty: %#v", movie.TypedMetadata)
	}

	trackMetadata := map[string]string{
		"trackArtist": "Mara Vale",
		"albumTitle":  "Late Trains for Bright Cities",
		"author":      "Wrong Author",
		"network":     "Wrong Network",
	}
	track, err := server.updateMedia("", "track_mara_01", UpdateMediaRequest{TypedMetadata: &trackMetadata})
	if err != nil {
		t.Fatalf("update track: %v", err)
	}
	if track.TypedMetadata["trackArtist"] != "Mara Vale" || track.TypedMetadata["albumTitle"] != "Late Trains for Bright Cities" {
		t.Fatalf("track typed metadata missing expected fields: %#v", track.TypedMetadata)
	}
	if _, ok := track.TypedMetadata["author"]; ok {
		t.Fatalf("track stored audiobook author: %#v", track.TypedMetadata)
	}
	if _, ok := track.TypedMetadata["network"]; ok {
		t.Fatalf("track stored TV network: %#v", track.TypedMetadata)
	}

	if _, err := server.db.Exec(`UPDATE media_items SET typed_metadata_json = ? WHERE id = ?`, `{"author":"Wrong Author","trackArtist":"Wrong Artist"}`, "movie_meridian"); err != nil {
		t.Fatalf("seed dirty movie typed metadata: %v", err)
	}
	reloaded, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("reload movie: %v", err)
	}
	if len(reloaded.TypedMetadata) != 0 {
		t.Fatalf("dirty movie typed metadata leaked through API: %#v", reloaded.TypedMetadata)
	}
}

func TestLibraryFiltersUseTypedMetadataFields(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_typed_filters', 'Typed Filters', 'music', 900, '/tmp/typed-filters', '{}', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_typed_books', 'Typed Books', 'audiobook', 901, '/tmp/typed-books', '{}', ?)`, now); err != nil {
		t.Fatalf("insert audiobook library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_typed_tv', 'Typed TV', 'show', 902, '/tmp/typed-tv', '{}', ?)`, now); err != nil {
		t.Fatalf("insert tv library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, added_at, typed_metadata_json)
		VALUES
			('typed_track_1', 'lib_typed_filters', 'track', 'Track One', 'Track One', '[]', ?, '{"trackArtist":"Track Artist","albumArtist":"Album Artist","albumTitle":"Album One","label":"Label One"}'),
			('typed_track_2', 'lib_typed_filters', 'track', 'Track Two', 'Track Two', '[]', ?, '{"trackArtist":"Other Artist","albumArtist":"Other Album Artist","albumTitle":"Album Two","label":"Label Two"}'),
			('typed_book_1', 'lib_typed_books', 'audiobook', 'Book One', 'Book One', '[]', ?, '{"author":"Author One","narrator":"Narrator One","series":"Series One"}')`, now, now, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, genres_json, added_at, season_number, episode_number, index_number, network)
		VALUES
			('typed_show_1', 'lib_typed_tv', NULL, 'show', 'Typed Show', 'Typed Show', '[]', ?, 0, 0, 0, 'Typed Network'),
			('typed_season_1', 'lib_typed_tv', 'typed_show_1', 'season', 'Season 1', 'Season 1', '[]', ?, 1, 0, 1, ''),
			('typed_episode_1', 'lib_typed_tv', 'typed_season_1', 'episode', 'Pilot', 'Pilot', '["Episode Only"]', ?, 1, 1, 1, 'Episode Network')`, now, now, now); err != nil {
		t.Fatalf("insert tv media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_provider_ids (media_id, provider, external_id, external_type, confidence, source, updated_at)
		VALUES ('typed_track_1', 'musicbrainz', 'rec-typed-1', 'recording', 1, 'test', ?)`, now); err != nil {
		t.Fatalf("insert provider id: %v", err)
	}
	tracks, _, _, filter, err := server.listLibraryItems("", "lib_typed_filters", "artist", "type:track;albumArtist:Album Artist", "asc", 120, 0)
	if err != nil {
		t.Fatalf("filter album artist: %v", err)
	}
	if filter != "albumArtist:Album Artist" || len(tracks) != 1 || tracks[0].ID != "typed_track_1" {
		t.Fatalf("album artist filter returned filter=%q items=%#v", filter, tracks)
	}
	tracks, _, sortMode, filter, err := server.listLibraryItems("", "lib_typed_filters", "trackArtist", "type:track;trackArtist:Track Artist", "asc", 120, 0)
	if err != nil {
		t.Fatalf("filter track artist: %v", err)
	}
	if sortMode != "trackArtist" || filter != "trackArtist:Track Artist" || len(tracks) != 1 || tracks[0].ID != "typed_track_1" {
		t.Fatalf("track artist filter returned sort=%q filter=%q items=%#v", sortMode, filter, tracks)
	}
	matched, _, _, filter, err := server.listLibraryItems("", "lib_typed_filters", "title", "type:track;matched", "asc", 120, 0)
	if err != nil {
		t.Fatalf("filter matched: %v", err)
	}
	if filter != "matched" || len(matched) != 1 || matched[0].ID != "typed_track_1" {
		t.Fatalf("matched filter returned filter=%q items=%#v", filter, matched)
	}
	unmatched, _, _, filter, err := server.listLibraryItems("", "lib_typed_filters", "title", "type:track;unmatched", "asc", 120, 0)
	if err != nil {
		t.Fatalf("filter unmatched: %v", err)
	}
	if filter != "unmatched" || len(unmatched) != 1 || unmatched[0].ID != "typed_track_2" {
		t.Fatalf("unmatched filter returned filter=%q items=%#v", filter, unmatched)
	}
	books, _, sortMode, filter, err := server.listLibraryItems("", "lib_typed_books", "author", "type:audiobook;author:Author One", "asc", 120, 0)
	if err != nil {
		t.Fatalf("filter author: %v", err)
	}
	if sortMode != "author" || filter != "author:Author One" || len(books) != 1 || books[0].ID != "typed_book_1" {
		t.Fatalf("author filter returned sort=%q filter=%q items=%#v", sortMode, filter, books)
	}
	for _, libraryID := range []string{"lib_typed_filters", "lib_typed_books", "lib_typed_tv"} {
		if err := server.rebuildLibraryCategoryFacets(libraryID); err != nil {
			t.Fatalf("rebuild typed category facets for %s: %v", libraryID, err)
		}
	}
	categories, err := server.listLibraryCategories("", "lib_typed_filters")
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	if !libraryCategoriesContainFilter(categories, "albumArtist:Album Artist") {
		t.Fatalf("categories missing album artist: %#v", categories)
	}
	if !libraryCategoriesContainFilter(categories, "label:Label One") {
		t.Fatalf("categories missing record label: %#v", categories)
	}
	categories, err = server.listLibraryCategories("", "lib_typed_books")
	if err != nil {
		t.Fatalf("list audiobook categories: %v", err)
	}
	if !libraryCategoriesContainFilter(categories, "author:Author One") {
		t.Fatalf("categories missing author: %#v", categories)
	}
	categories, err = server.listLibraryCategories("", "lib_typed_tv")
	if err != nil {
		t.Fatalf("list tv categories: %v", err)
	}
	if !libraryCategoriesContainFilter(categories, "show:Typed Show") || !libraryCategoriesContainFilter(categories, "season:Season 1") {
		t.Fatalf("categories missing show/season: %#v", categories)
	}
	if !libraryCategoriesContainFilter(categories, "genre:Episode Only") || !libraryCategoriesContainFilter(categories, "network:Episode Network") {
		t.Fatalf("categories missing episode descendant metadata: %#v", categories)
	}
	seasons, _, _, filter, err := server.listLibraryItems("", "lib_typed_tv", "title", "season:Season 1", "asc", 120, 0)
	if err != nil {
		t.Fatalf("filter season: %v", err)
	}
	if filter != "season:Season 1" || len(seasons) != 1 || seasons[0].ID != "typed_season_1" {
		t.Fatalf("season filter returned filter=%q items=%#v", filter, seasons)
	}
}

func libraryCategoriesContainFilter(categories []LibraryCategory, filter string) bool {
	for _, category := range categories {
		if category.Filter == filter {
			return true
		}
	}
	return false
}

func TestLibraryCategoryCacheInvalidatesAfterMetadataMutation(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	categories, err := server.listLibraryCategories("", "lib_movies")
	if err != nil {
		t.Fatalf("list initial categories: %v", err)
	}
	if len(categories) == 0 {
		t.Fatalf("expected seeded movie categories")
	}
	categories[0].Count = 999999
	cached, err := server.listLibraryCategories("", "lib_movies")
	if err != nil {
		t.Fatalf("list cached categories: %v", err)
	}
	if cached[0].Count == 999999 {
		t.Fatalf("category cache returned mutable slice")
	}
	if libraryCategoriesContainFilter(cached, "genre:Cache Genre") {
		t.Fatalf("unexpected cache genre before update: %#v", cached)
	}
	genres := []string{"Cache Genre"}
	if _, err := server.updateMedia("", "movie_meridian", UpdateMediaRequest{Genres: &genres}); err != nil {
		t.Fatalf("update media genres: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.waitForLibraryReadModelRepair(ctx, "lib_movies"); err != nil {
		t.Fatalf("wait for category read-model repair: %v", err)
	}
	updated, err := server.listLibraryCategories("", "lib_movies")
	if err != nil {
		t.Fatalf("list updated categories: %v", err)
	}
	if !libraryCategoriesContainFilter(updated, "genre:Cache Genre") {
		t.Fatalf("category cache was not invalidated after metadata update: %#v", updated)
	}
}

func mediaImagesContainRemote(images []MediaImage, imageType, provider, remoteURL string) bool {
	for _, image := range images {
		if image.Type == imageType && image.Provider == provider && image.RemoteURL == remoteURL {
			return true
		}
	}
	return false
}

func mediaImagesContainProviderCache(images []MediaImage, imageType, provider string) bool {
	for _, image := range images {
		if image.Type == imageType && image.Provider == provider && image.Path != "" && image.RemoteURL == "" {
			return true
		}
	}
	return false
}

func TestMusicMetadataFromTagsExtractsMusicSpecificFields(t *testing.T) {
	metadata := musicMetadataFromTags(map[string]string{
		"title":                      "Track One",
		"artist":                     "Track Artist",
		"album_artist":               "Album Artist",
		"album":                      "Album One",
		"track":                      "3/12",
		"disc":                       "2/4",
		"bpm":                        "94",
		"explicit":                   "true",
		"label":                      "Portico Records",
		"releasecountry":             "CA",
		"mood":                       "Energetic;Late Night",
		"performer":                  "Session Player",
		"performer:drums":            "Drummer One",
		"musicbrainz_trackid":        "recording-123",
		"musicbrainz_releasegroupid": "release-group-123",
		"musicbrainz_albumartistid":  "artist-123",
	})
	if metadata.Title != "Track One" || metadata.Artist != "Track Artist" || metadata.AlbumArtist != "Album Artist" || metadata.AlbumTitle != "Album One" || metadata.Label != "Portico Records" || metadata.ReleaseCountry != "CA" {
		t.Fatalf("basic metadata = %#v", metadata)
	}
	if len(metadata.Tags) != 2 || metadata.Tags[0] != "Energetic" || metadata.Tags[1] != "Late Night" {
		t.Fatalf("music mood tags = %#v", metadata.Tags)
	}
	if metadata.TrackNumber != 3 || metadata.TrackCount != 12 || metadata.DiscNumber != 2 || metadata.DiscCount != 4 || metadata.BPM != 94 || metadata.Explicit != "true" {
		t.Fatalf("music-specific fields = %#v", metadata)
	}
	if metadata.ProviderIDs["musicbrainz:recording"] != "recording-123" || metadata.ProviderIDs["musicbrainz:release-group"] != "release-group-123" || metadata.ProviderIDs["musicbrainz:album-artist"] != "artist-123" {
		t.Fatalf("musicbrainz provider ids = %#v", metadata.ProviderIDs)
	}
	if !mediaPeopleContain(metadata.People, "Performer", "Session Player") {
		t.Fatalf("music people = %#v", metadata.People)
	}
	if !mediaPeopleContain(metadata.People, "Performer: Drums", "Drummer One") {
		t.Fatalf("instrument performer people = %#v", metadata.People)
	}
	if provider, externalType := scannerProviderIDTarget("musicbrainz:album-artist", "track"); provider != "musicbrainz" || externalType != "artist" {
		t.Fatalf("provider target provider=%q externalType=%q", provider, externalType)
	}
}

func mediaPeopleContain(people []MediaPerson, role, name string) bool {
	for _, person := range people {
		if person.Role == role && person.Name == name {
			return true
		}
	}
	return false
}

func TestManualMusicCreditUpdatePersistsEditablePeople(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	people := []MediaPerson{
		{Name: "Mara Vale", Role: "Composer"},
		{Name: "Roen Porter", Role: "Producer"},
		{Name: "Bad Actor", Role: "Network"},
		{Name: "", Role: "Lyricist"},
	}
	updated, err := server.updateMedia("", "track_mara_01", UpdateMediaRequest{People: &people})
	if err != nil {
		t.Fatalf("update media people: %v", err)
	}
	if !mediaPeopleContain(updated.People, "Composer", "Mara Vale") || !mediaPeopleContain(updated.People, "Producer", "Roen Porter") {
		t.Fatalf("manual people were not returned: %#v", updated.People)
	}
	if mediaPeopleContain(updated.People, "Network", "Bad Actor") || mediaPeopleContain(updated.People, "Lyricist", "") {
		t.Fatalf("invalid manual people were not sanitized: %#v", updated.People)
	}
	var source string
	if err := server.db.QueryRow(`SELECT source FROM media_people WHERE media_id = ? AND role = ? AND name = ?`, "track_mara_01", "Composer", "Mara Vale").Scan(&source); err != nil || source != "manual" {
		t.Fatalf("manual person source = %q err=%v", source, err)
	}
}

func TestAudiobookMetadataFromTagsExtractsBookSpecificFields(t *testing.T) {
	metadata := musicMetadataFromTags(map[string]string{
		"title":       "Project Hail Mary",
		"author":      "Andy Weir",
		"narrator":    "Ray Porter",
		"series":      "Hail Mary",
		"seriesindex": "1",
		"publisher":   "Random House Audio",
		"genre":       "Science Fiction",
		"date":        "2021-05-04",
		"tracknumber": "1/2",
	})
	if metadata.Title != "Project Hail Mary" || metadata.Artist != "Andy Weir" || metadata.Studio != "Ray Porter" || metadata.Series != "Hail Mary" || metadata.SeriesIndex != "1" || metadata.Publisher != "Random House Audio" {
		t.Fatalf("audiobook tag metadata = %#v", metadata)
	}
	if metadata.Year != 2021 || metadata.TrackNumber != 1 || metadata.TrackCount != 2 || len(metadata.Genres) != 1 || metadata.Genres[0] != "Science Fiction" {
		t.Fatalf("audiobook parsed fields = %#v", metadata)
	}
	typed := typedMetadataFromMusicMetadata(MediaItem{Type: "audiobook"}, metadata)
	if typed["author"] != "Andy Weir" || typed["narrator"] != "Ray Porter" || typed["series"] != "Hail Mary" || typed["seriesIndex"] != "1" || typed["publisher"] != "Random House Audio" {
		t.Fatalf("audiobook typed metadata = %#v", typed)
	}
}

func TestMetadataRefreshIgnoresStaleTMDBProviderIDWhenFilenameDisagrees(t *testing.T) {
	var searchYear string
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/movie/1239561":
			_, _ = w.Write([]byte(`{"id":1239561,"title":"Waffle F1 - The Movie","release_date":"2022-01-01","popularity":0.4}`))
		case "/search/movie":
			searchYear = r.URL.Query().Get("year")
			_, _ = w.Write([]byte(`{"results":[{"id":911430,"title":"F1 The Movie","release_date":"2025-06-25","overview":"Racing drama.","popularity":350.0}]}`))
		case "/movie/911430":
			_, _ = w.Write([]byte(`{"id":911430,"title":"F1 The Movie","release_date":"2025-06-25","overview":"Racing drama.","popularity":350.0}`))
		default:
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-api-key", TMDBBaseURL: tmdb.URL})
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	before.Title = "F1 The Movie"
	before.Year = 2022
	before.SourceURL = "/Movies/F1.The.Movie.2025.HYBRID.1080p.BluRay.x264.mp4"
	if err := server.upsertMediaProviderID(before.ID, "tmdb", "1239561", "movie", 0.85, "bad-test"); err != nil {
		t.Fatalf("seed provider id: %v", err)
	}
	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh metadata: %v", err)
	}
	if after.Title != "F1 The Movie" || after.Year != 2025 {
		t.Fatalf("stale provider id was trusted: title=%q year=%d", after.Title, after.Year)
	}
	if searchYear != "2025" {
		t.Fatalf("search year = %q, expected filename year 2025", searchYear)
	}
}

func TestManualTMDBMatchAppliesSelectedCandidateEvenWhenTitleDisagrees(t *testing.T) {
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/911430" {
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":911430,"title":"F1 The Movie","release_date":"2025-06-25","overview":"Racing drama.","vote_average":7.8,"genre_ids":[18],"poster_path":"/f1.jpg"}`))
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-api-key", TMDBBaseURL: tmdb.URL})
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET title = ?, sort_title = ? WHERE id = ?`, "Waffle F1", "Waffle F1", before.ID); err != nil {
		t.Fatalf("seed stale title: %v", err)
	}
	after, err := server.applyManualMediaMatch(context.Background(), "", before.ID, ManualMediaMatchRequest{
		Provider:     "tmdb",
		ExternalID:   "911430",
		ExternalType: "movie",
	})
	if err != nil {
		t.Fatalf("apply manual match: %v", err)
	}
	if after.Title != "F1 The Movie" || after.Year != 2025 || after.Artwork["posterPath"] != "/f1.jpg" {
		t.Fatalf("manual match metadata = title=%q year=%d artwork=%#v", after.Title, after.Year, after.Artwork)
	}
	if externalID, ok := server.mediaProviderID(after.ID, "tmdb", "movie"); !ok || externalID != "911430" {
		t.Fatalf("provider id = %q ok=%v", externalID, ok)
	}
	var accepted int
	var reasons string
	if err := server.db.QueryRow(`
		SELECT CASE WHEN status = 'accepted' THEN 1 ELSE 0 END, reason_codes_json
		FROM media_match_candidates
		WHERE media_id = ? AND provider = 'tmdb' AND external_id = '911430'
		ORDER BY created_at DESC LIMIT 1`, before.ID).Scan(&accepted, &reasons); err != nil {
		t.Fatalf("manual match candidate evidence: %v", err)
	}
	if accepted != 1 || !strings.Contains(reasons, "manual_match") {
		t.Fatalf("candidate evidence accepted=%d reasons=%s", accepted, reasons)
	}
}

func TestMetadataRepairListsStaleProviderIDsAndQueuesRematch(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	staleAt := time.Now().UTC().AddDate(-1, 0, 0).Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT OR REPLACE INTO media_provider_ids (media_id, provider, external_id, external_type, confidence, source, updated_at)
		VALUES (?, 'tmdb', 'bad-42', 'movie', 0.42, 'stale-test', ?)`, "movie_meridian", staleAt); err != nil {
		t.Fatalf("seed stale provider id: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_match_candidates (id, media_id, provider, external_id, external_type, source, score, status, reason_codes_json, raw_query, raw_result_json, created_at)
		VALUES ('candidate_stale_1', ?, 'tmdb', 'bad-42', 'movie', 'test', 12, 'candidate', '[{"code":"title_mismatch","delta":-40,"detail":"test"}]', 'F1', '{}', ?)`,
		"movie_meridian", staleAt); err != nil {
		t.Fatalf("seed match candidate: %v", err)
	}

	var repair MetadataRepairResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/metadata/repair?limit=10&staleDays=30", nil, &repair)
	if status != http.StatusOK {
		t.Fatalf("repair status = %d, body: %s", status, body)
	}
	if len(repair.Items) == 0 {
		t.Fatalf("expected stale provider id in repair response")
	}
	found := false
	for _, item := range repair.Items {
		if item.MediaID == "movie_meridian" && item.ExternalID == "bad-42" {
			found = true
			if item.Reason == "" || item.LatestCandidateScore != 12 || len(item.LatestCandidateReasons) == 0 {
				t.Fatalf("repair item missing diagnostic context: %+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("stale provider id not listed: %+v", repair.Items)
	}

	var action MetadataRepairActionResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/metadata/repair", MetadataRepairRequest{MediaID: "movie_meridian", ClearProviderIDs: true}, &action)
	if status != http.StatusCreated {
		t.Fatalf("repair action status = %d, body: %s", status, body)
	}
	if action.Job.Type != "metadata_refresh" || action.Job.ResourceID != "movie_meridian" {
		t.Fatalf("repair action job = %+v", action.Job)
	}
	if action.ClearedProviderIDs != 1 || action.ClearedCandidates != 1 {
		t.Fatalf("cleared provider IDs/candidates = %d/%d", action.ClearedProviderIDs, action.ClearedCandidates)
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_provider_ids WHERE media_id = ?`, "movie_meridian").Scan(&remaining); err != nil {
		t.Fatalf("count provider IDs: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("provider IDs remaining = %d", remaining)
	}
	var revision, repairRevisions, outcomes int
	if err := db.QueryRow(`SELECT metadata_revision FROM media_items WHERE id = ?`, "movie_meridian").Scan(&revision); err != nil {
		t.Fatalf("read repaired metadata revision: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_metadata_revisions WHERE media_id = ? AND revision = ? AND trigger_kind = 'repair' AND state = 'applied'`, "movie_meridian", revision).Scan(&repairRevisions); err != nil {
		t.Fatalf("count repair revisions: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_metadata_refresh_outcomes WHERE media_id = ? AND resulting_revision = ? AND status = 'applied'`, "movie_meridian", revision).Scan(&outcomes); err != nil {
		t.Fatalf("count repair outcomes: %v", err)
	}
	if repairRevisions != 1 || outcomes != 1 {
		t.Fatalf("repair audit records = revisions %d outcomes %d", repairRevisions, outcomes)
	}
	var projectedRevision int
	if err := db.QueryRow(`SELECT metadata_revision FROM media_category_facets WHERE media_id = ? LIMIT 1`, "movie_meridian").Scan(&projectedRevision); err != nil {
		t.Fatalf("read repaired facet projection: %v", err)
	}
	if projectedRevision != revision {
		t.Fatalf("facet revision = %d, canonical revision = %d", projectedRevision, revision)
	}
}

func TestMetadataHealthReportsActionableLibraryIssues(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, duration_seconds, artwork_json, source_url, added_at)
		VALUES ('health_raw_movie', 'lib_movies', 'movie', 'AAA.Raw.Movie.2024.1080p.WEB-DL.x264', 'AAA Raw Movie', 0, '{}', '', ?)`, now); err != nil {
		t.Fatalf("seed health movie: %v", err)
	}
	if err := server.refreshMetadataHealthIssuesContext(context.Background(), "lib_movies"); err != nil {
		t.Fatalf("refresh metadata health issues: %v", err)
	}

	var health MetadataHealthResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/metadata/health?limit=200", nil, &health)
	if status != http.StatusOK {
		t.Fatalf("health status = %d, body: %s", status, body)
	}
	if health.Total == 0 || health.GeneratedAt == "" || len(health.Summary) == 0 {
		t.Fatalf("metadata health missing totals: %#v", health)
	}
	for _, category := range []string{"unmatched", "missing_artwork", "missing_duration", "unavailable_source", "raw_title"} {
		if !metadataHealthHasIssue(health.Items, "health_raw_movie", category) {
			t.Fatalf("metadata health did not include %s for seeded item: %#v", category, health.Items)
		}
		if !metadataHealthSummaryHasCategory(health.Summary, category) {
			t.Fatalf("metadata health summary missing %s: %#v", category, health.Summary)
		}
	}

	var rawOnly MetadataHealthResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/metadata/health?category=raw-title&libraryId=lib_movies&limit=1", nil, &rawOnly)
	if status != http.StatusOK {
		t.Fatalf("raw health status = %d, body: %s", status, body)
	}
	rawSummaryCount := 0
	for _, item := range rawOnly.Summary {
		if item.Category == "raw_title" {
			rawSummaryCount = item.Count
			break
		}
	}
	if rawOnly.Total != rawSummaryCount || len(rawOnly.Items) > 1 {
		t.Fatalf("raw title page should use summary total with capped items, total=%d summary=%d items=%#v", rawOnly.Total, rawSummaryCount, rawOnly.Items)
	}
	if !metadataHealthHasIssue(rawOnly.Items, "health_raw_movie", "raw_title") {
		t.Fatalf("raw title filter did not include seeded item: %#v", rawOnly.Items)
	}
	for _, issue := range rawOnly.Items {
		if issue.Category != "raw_title" || issue.LibraryID != "lib_movies" {
			t.Fatalf("raw title filter leaked issue: %#v", issue)
		}
	}
}

func TestMetadataHealthUsesShortTTLCache(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if err := server.refreshMetadataHealthIssuesContext(context.Background(), "lib_movies"); err != nil {
		t.Fatalf("refresh initial metadata health issues: %v", err)
	}
	first, err := server.metadataHealthReport(300, "raw-title", "lib_movies")
	if err != nil {
		t.Fatalf("first metadata health report: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, duration_seconds, artwork_json, source_url, added_at)
		VALUES ('health_cached_raw_movie', 'lib_movies', 'movie', 'Cached.Raw.Movie.2024.1080p.WEB-DL.x264', 'Cached Raw Movie', 0, '{}', '', ?)`, now); err != nil {
		t.Fatalf("seed cached health movie: %v", err)
	}
	cached, err := server.metadataHealthReport(300, "raw-title", "lib_movies")
	if err != nil {
		t.Fatalf("cached metadata health report: %v", err)
	}
	if cached.GeneratedAt != first.GeneratedAt || metadataHealthHasIssue(cached.Items, "health_cached_raw_movie", "raw_title") {
		t.Fatalf("metadata health did not use cached response: first=%q cached=%q items=%#v", first.GeneratedAt, cached.GeneratedAt, cached.Items)
	}
	server.invalidateMetadataHealthCache()
	if err := server.refreshMetadataHealthIssuesContext(context.Background(), "lib_movies"); err != nil {
		t.Fatalf("refresh metadata health issues after seed: %v", err)
	}
	refreshed, err := server.metadataHealthReport(300, "raw-title", "lib_movies")
	if err != nil {
		t.Fatalf("refreshed metadata health report: %v", err)
	}
	if !metadataHealthHasIssue(refreshed.Items, "health_cached_raw_movie", "raw_title") {
		t.Fatalf("metadata health refresh did not include seeded issue: %#v", refreshed.Items)
	}
}

func TestLibraryBrowseUserAndAvailabilityFiltersAndSourceGroups(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, year, duration_seconds, artwork_json, source_url, added_at)
		VALUES
			('org_attention_movie', 'lib_movies', 'movie', 'Raw.Movie.2025.1080p.WEB-DL.x264', 'Raw Movie', 2025, 0, '{}', '', ?),
			('org_duplicate_a', 'lib_movies', 'movie', 'Duplicate Candidate', 'Duplicate Candidate', 2025, 5400, '{"posterPath":"/poster-a.jpg"}', '/media/duplicate-a.mp4', ?),
			('org_duplicate_b', 'lib_movies', 'movie', 'Duplicate Candidate', 'Duplicate Candidate', 2025, 5400, '{"posterPath":"/poster-b.jpg"}', '/media/duplicate-b.mp4', ?),
			('org_local_movie', 'lib_movies', 'movie', 'Local Source Movie', 'Local Source Movie', 2024, 5400, '{"posterPath":"/poster-local.jpg"}', '/media/local.mp4', ?),
			('org_remote_movie', 'lib_movies', 'movie', 'Remote Source Movie', 'Remote Source Movie', 2024, 5400, '{"posterPath":"/poster-remote.jpg"}', 'https://cdn.example.test/remote.mp4', ?),
			('org_optimized_movie', 'lib_movies', 'movie', 'Optimized Movie', 'Optimized Movie', 2024, 5400, '{"posterPath":"/poster-optimized.jpg"}', '/media/optimized-source.mp4', ?)`,
		now, now, now, now, now, now); err != nil {
		t.Fatalf("seed organization media: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, source, source_type, available, first_seen_at, last_seen_at)
		VALUES
			('org_attention_file', 'org_attention_movie', 'lib_movies', '/missing/raw.mp4', '', 'local', 0, ?, ?),
			('org_duplicate_a_file', 'org_duplicate_a', 'lib_movies', '/media/duplicate-a.mp4', '', 'local', 1, ?, ?),
			('org_duplicate_b_file', 'org_duplicate_b', 'lib_movies', '/media/duplicate-b.mp4', '', 'local', 1, ?, ?),
			('org_local_file', 'org_local_movie', 'lib_movies', '/media/local.mp4', '', 'local', 1, ?, ?),
			('org_remote_file', 'org_remote_movie', 'lib_movies', 'https://cdn.example.test/remote.mp4', 'https', 'remote', 1, ?, ?),
			('org_optimized_file', 'org_optimized_movie', 'lib_movies', '/media/optimized-source.mp4', '', 'local', 1, ?, ?)`,
		now, now, now, now, now, now, now, now, now, now, now, now); err != nil {
		t.Fatalf("seed organization files: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO optimized_versions (id, media_id, profile, path, size_bytes, created_at, updated_at)
		VALUES ('org_optimized_version', 'org_optimized_movie', '720p-medium', '/optimized/org_optimized_movie/720p.mp4', 1234, ?, ?)`,
		now, now); err != nil {
		t.Fatalf("seed optimized version: %v", err)
	}

	var favorite MediaItem
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/media/org_local_movie/favorite", map[string]bool{"favorite": true}, &favorite)
	if status != http.StatusOK {
		t.Fatalf("favorite status = %d, body: %s", status, body)
	}

	assertLibraryBrowseContains(t, client, serverURL, BrowseExpression{
		Field: "favorite", Operator: "is", Value: json.RawMessage("true"),
	}, "org_local_movie")
	assertLibraryBrowseContains(t, client, serverURL, BrowseExpression{
		Field: "availability", Operator: "equals", Value: json.RawMessage(`"unavailable"`),
	}, "org_attention_movie")
	assertLibraryBrowseContains(t, client, serverURL, BrowseExpression{
		Field: "title", Operator: "contains", Value: json.RawMessage(`"Duplicate Candidate"`),
	}, "org_duplicate_a", "org_duplicate_b")
	server.queueLibraryReadModelRepair("lib_movies")
	repairCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.waitForLibraryReadModelRepair(repairCtx, "lib_movies"); err != nil {
		t.Fatalf("wait for library source read-model repair: %v", err)
	}

	var sources CursorListResponse[LibrarySourceGroup]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/lib_movies/sources", nil, &sources)
	if status != http.StatusOK {
		t.Fatalf("library sources status = %d, body: %s", status, body)
	}
	localSource := findLibrarySourceGroup(t, sources.Items, "local", "/media")
	remoteSource := findLibrarySourceGroup(t, sources.Items, "remote", "https://cdn.example.test")
	if localSource.ItemCount < 3 || localSource.FileCount < 3 {
		t.Fatalf("local source counts too low: %#v", localSource)
	}
	if remoteSource.ItemCount < 1 || remoteSource.FileCount < 1 {
		t.Fatalf("remote source counts too low: %#v", remoteSource)
	}
}

func assertLibraryBrowseContains(t *testing.T, client *http.Client, serverURL string, query BrowseExpression, ids ...string) {
	t.Helper()
	var response BrowseLibraryResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/libraries/lib_movies/browse", BrowseLibraryRequest{
		Pivot: "movies",
		Query: &query,
		Limit: 200,
	}, &response)
	if status != http.StatusOK {
		t.Fatalf("library browse query %#v status = %d, body: %s", query, status, body)
	}
	for _, id := range ids {
		found := false
		for _, item := range response.Items {
			if item.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("library browse query %#v missing %s: %#v", query, id, response.Items)
		}
	}
}

func findLibrarySourceGroup(t *testing.T, groups []LibrarySourceGroup, kind, path string) LibrarySourceGroup {
	t.Helper()
	for _, group := range groups {
		if group.Kind == kind && group.Path == path {
			return group
		}
	}
	t.Fatalf("missing source group kind=%s path=%s in %#v", kind, path, groups)
	return LibrarySourceGroup{}
}

func metadataHealthHasIssue(items []MetadataHealthIssue, mediaID, category string) bool {
	for _, item := range items {
		if item.MediaID == mediaID && item.Category == category {
			return true
		}
	}
	return false
}

func metadataHealthSummaryHasCategory(summary []MetadataHealthSummary, category string) bool {
	for _, item := range summary {
		if item.Category == category && item.Count > 0 {
			return true
		}
	}
	return false
}

func TestMetadataRefreshDiagnosticSummaryReportsScoresAndReasons(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	if _, err := server.db.Exec(`
		INSERT INTO media_match_candidates (id, media_id, provider, external_id, external_type, source, score, status, reason_codes_json, raw_query, raw_result_json, created_at)
		VALUES ('candidate_diag_1', ?, 'tmdb', '911430', 'movie', 'provider-search', 87.5, 'accepted', '[{"code":"title_exact","delta":45},{"code":"year_exact","delta":20}]', 'F1 The Movie', '{}', ?)`,
		"movie_meridian", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("seed match candidate: %v", err)
	}
	summary := server.metadataRefreshDiagnosticSummary([]string{"movie_meridian"})
	if !strings.Contains(summary, "Matched tmdb movie:911430 with score 87.5") || !strings.Contains(summary, "title_exact") || !strings.Contains(summary, "year_exact") {
		t.Fatalf("diagnostic summary = %q", summary)
	}
}

func TestIdentityEvidenceContributesToCandidateScoring(t *testing.T) {
	item := MediaItem{
		Title: "Waffle F1",
		IdentityEvidence: []IdentityEvidence{
			{Source: "path-parser", Field: "title", Value: "F1 The Movie", Confidence: 0.95},
			{Source: "path-parser", Field: "year", Value: "2025", Confidence: 0.9},
		},
	}
	score := tmdbResultCandidateScore(tmdbSearchResult{ID: 911430, Title: "F1 The Movie", ReleaseDate: "2025-06-25"}, item, []string{item.Title}, 0)
	reasons := score.reasonCodesJSON()
	if !strings.Contains(reasons, "evidence_title_exact") || !strings.Contains(reasons, "evidence_year_exact") {
		t.Fatalf("evidence reasons missing from score %.2f: %s", score.Score, reasons)
	}
}

func TestTMDBMatchingPrefersSourceFileYearOverStaleMetadataYear(t *testing.T) {
	item := MediaItem{
		Title:     "Nativity",
		Year:      1978,
		SourceURL: "/media/Nativity.2009.1080p.BluRay.x265-RARBG.mp4",
		MediaFiles: []MediaFileVersion{
			{Path: "/media/Nativity.2009.1080p.BluRay.x265-RARBG.mp4", OriginalFilename: "Nativity.2009.1080p.BluRay.x265-RARBG.mp4"},
		},
	}
	year := metadataSearchYear(item)
	if year != 2009 {
		t.Fatalf("metadataSearchYear = %d, expected source file year 2009", year)
	}
	titles := tmdbQueryTitlesForItem(item)
	correct := tmdbResultCandidateScore(tmdbSearchResult{ID: 49522, Title: "Nativity!", ReleaseDate: "2009-11-27"}, item, titles, year)
	stale := tmdbResultCandidateScore(tmdbSearchResult{ID: 293380, Title: "The Nativity", ReleaseDate: "1978-12-01"}, item, titles, year)
	if correct.Score <= stale.Score {
		t.Fatalf("source file year did not outrank stale metadata: correct %.2f stale %.2f titles=%v", correct.Score, stale.Score, titles)
	}
	if !strings.Contains(correct.reasonCodesJSON(), "year_exact") || !strings.Contains(stale.reasonCodesJSON(), "year_conflict") {
		t.Fatalf("unexpected candidate reasons: correct=%s stale=%s", correct.reasonCodesJSON(), stale.reasonCodesJSON())
	}
}

func TestMusicBrainzReleaseGroupRankingUsesTitleArtistAndYear(t *testing.T) {
	item := MediaItem{Title: "United We Fall", Studio: "Sweatshop Union", Year: 2005}
	groups := []musicBrainzReleaseGroup{
		{ID: "wrong", Title: "United", Score: 100, FirstReleaseDate: "2020-01-01", ArtistCredit: []musicBrainzArtistCredit{{Name: "Other Artist"}}},
		{ID: "right", Title: "United We Fall", Score: 88, FirstReleaseDate: "2005-01-01", ArtistCredit: []musicBrainzArtistCredit{{Name: "Sweatshop Union"}}},
	}
	best := bestMusicBrainzReleaseGroup(groups, item)
	if best.ID != "right" {
		t.Fatalf("best group = %#v", best)
	}
}

func TestMetadataRefreshUsesTMDBEpisodeDetails(t *testing.T) {
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/tv":
			if got := r.URL.Query().Get("query"); got != "Northbridge" {
				t.Fatalf("query = %q", got)
			}
			if got := r.URL.Query().Get("first_air_date_year"); got != "2024" {
				t.Fatalf("first_air_date_year = %q, expected show year", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":42,"name":"Northbridge","first_air_date":"2024-01-01","overview":"Show overview.","vote_average":7.1,"genre_ids":[18,80]}]}`))
		case "/tv/42":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"name":"Northbridge","first_air_date":"2024-01-01","overview":"Show overview.","vote_average":7.1,"genre_ids":[18,80]}`))
		case "/tv/42/season/1/episode/2":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"The Red Ledger","air_date":"2024-02-02","overview":"Episode-specific TMDB overview.","vote_average":8.9,"still_path":"/still.jpg"}`))
		default:
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-token", TMDBBaseURL: tmdb.URL})
	if _, err := server.db.Exec(`UPDATE media_items SET year = 2026 WHERE id = 'episode_northbridge_102'`); err != nil {
		t.Fatalf("set episode air year: %v", err)
	}
	before, err := server.getMediaDetail("", "episode_northbridge_102")
	if err != nil {
		t.Fatalf("load episode: %v", err)
	}
	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh episode metadata: %v", err)
	}
	if after.Summary != "Episode-specific TMDB overview." {
		t.Fatalf("summary = %q", after.Summary)
	}
	if after.CommunityRating != 8.9 {
		t.Fatalf("community rating = %v", after.CommunityRating)
	}
	if len(after.Genres) != 2 || after.Genres[0] != "Drama" || after.Genres[1] != "Crime" {
		t.Fatalf("genres = %#v", after.Genres)
	}
	if after.Artwork["source"] != "tmdb" || after.Artwork["thumbPath"] != "/still.jpg" {
		t.Fatalf("episode artwork = %#v", after.Artwork)
	}
}

func TestEpisodeManualMatchSearchUsesSeriesTitleByDefault(t *testing.T) {
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/tv" {
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "Northbridge" {
			t.Fatalf("query = %q, expected series title", got)
		}
		if got := r.URL.Query().Get("first_air_date_year"); got != "2024" {
			t.Fatalf("first_air_date_year = %q, expected show year", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":42,"name":"Northbridge","first_air_date":"2024-01-01","overview":"Show overview.","vote_average":7.1,"genre_ids":[18]}]}`))
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-token", TMDBBaseURL: tmdb.URL})
	if _, err := server.db.Exec(`UPDATE media_items SET year = 2026 WHERE id = 'episode_northbridge_102'`); err != nil {
		t.Fatalf("set episode air year: %v", err)
	}
	candidates, err := server.searchMediaMatchCandidates(context.Background(), "", "episode_northbridge_102", "")
	if err != nil {
		t.Fatalf("search episode candidates: %v", err)
	}
	if len(candidates) == 0 || candidates[0].ExternalID != "42" {
		t.Fatalf("episode match candidates = %#v", candidates)
	}
}

func TestMovieManualMatchSearchFallsBackWhenQueryHasExtraWords(t *testing.T) {
	var queries []string
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/movie" {
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
		query := r.URL.Query().Get("query")
		queries = append(queries, query)
		w.Header().Set("Content-Type", "application/json")
		if query == "I LIKE TO SEE THE WHEELS TURN" {
			_, _ = w.Write([]byte(`{"results":[{"id":1178644,"title":"I Like to See the Wheels Turn","release_date":"1981-01-01","overview":"Documentary short."}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-token", TMDBBaseURL: tmdb.URL})
	if _, err := server.db.Exec(`
		UPDATE media_items
		SET title = 'I LIKE TO SEE THE WHEELS TURN K C IRVING',
			sort_title = 'I LIKE TO SEE THE WHEELS TURN K C IRVING',
			year = 1981,
			source_url = '/movies/I LIKE TO SEE THE WHEELS TURN - K.C. IRVING (1981 Documentary).mp4'
		WHERE id = 'movie_meridian'`); err != nil {
		t.Fatalf("seed credited movie: %v", err)
	}
	candidates, err := server.searchMediaMatchCandidatesWithOptions(context.Background(), "", "movie_meridian", "I LIKE TO SEE THE WHEELS TURN K C IRVING", 1981, "")
	if err != nil {
		t.Fatalf("search movie candidates: %v", err)
	}
	if len(candidates) == 0 || candidates[0].ExternalID != "1178644" {
		t.Fatalf("movie match candidates = %#v", candidates)
	}
	if !stringSliceContains(queries, "I LIKE TO SEE THE WHEELS TURN K C IRVING") || !stringSliceContains(queries, "I LIKE TO SEE THE WHEELS TURN") {
		t.Fatalf("TMDB queries = %#v, expected exact and fuzzy fallback searches", queries)
	}
}

func TestMetadataRefreshUsesConfiguredTMDBEpisodeGroup(t *testing.T) {
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/tv":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":42,"name":"Northbridge","first_air_date":"2024-01-01","overview":"Show overview.","vote_average":7.1,"genre_ids":[18,80]}]}`))
		case "/tv/42":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"name":"Northbridge","first_air_date":"2024-01-01","overview":"Show overview.","vote_average":7.1,"genre_ids":[18,80]}`))
		case "/tv/episode_group/group_absolute":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"group_absolute","name":"Absolute Order","groups":[{"id":"g1","name":"Season 1","order":1,"episodes":[{"episode_number":2,"season_number":1,"name":"Absolute Red Ledger","air_date":"2024-03-03","overview":"Episode group overview.","vote_average":9.1,"still_path":"/group-still.jpg"}]}]}`))
		default:
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-token", TMDBBaseURL: tmdb.URL})
	before, err := server.getMediaDetail("", "episode_northbridge_102")
	if err != nil {
		t.Fatalf("load episode: %v", err)
	}
	settingsJSON := `{"tmdbEpisodeGroupId":"group_absolute"}`
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json = ? WHERE id = ?`, settingsJSON, before.LibraryID); err != nil {
		t.Fatalf("set library episode group: %v", err)
	}
	after, err := server.refreshMediaMetadataFromTMDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh episode metadata: %v", err)
	}
	if after.Title != "Absolute Red Ledger" || after.Summary != "Episode group overview." {
		t.Fatalf("episode group metadata = %#v", after)
	}
	if after.Artwork["thumbPath"] != "/group-still.jpg" {
		t.Fatalf("episode group artwork = %#v", after.Artwork)
	}
}

func TestMetadataRefreshCascadesShowToSeasonsAndEpisodes(t *testing.T) {
	searches := 0
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/tv":
			searches++
			if got := r.URL.Query().Get("query"); got != "Northbridge" {
				t.Fatalf("query = %q", got)
			}
			_, _ = w.Write([]byte(`{"results":[{"id":42,"name":"Northbridge","first_air_date":"2024-01-01","overview":"Refreshed show overview.","vote_average":7.1,"genre_ids":[18,80],"poster_path":"/show-poster.jpg","backdrop_path":"/show-backdrop.jpg"}]}`))
		case "/tv/42":
			_, _ = w.Write([]byte(`{"id":42,"name":"Northbridge","first_air_date":"2024-01-01","overview":"Refreshed show overview.","vote_average":7.1,"genre_ids":[18,80],"poster_path":"/show-poster.jpg","backdrop_path":"/show-backdrop.jpg"}`))
		case "/tv/42/season/1":
			_, _ = w.Write([]byte(`{"name":"Season One","air_date":"2024-01-01","overview":"Refreshed season overview.","poster_path":"/season-poster.jpg"}`))
		case "/tv/42/season/1/episode/1":
			_, _ = w.Write([]byte(`{"name":"Cold Archive","air_date":"2024-01-08","overview":"Refreshed episode one.","vote_average":8.1,"still_path":"/episode-1.jpg"}`))
		case "/tv/42/season/1/episode/2":
			_, _ = w.Write([]byte(`{"name":"The Red Ledger","air_date":"2024-01-15","overview":"Refreshed episode two.","vote_average":8.2,"still_path":"/episode-2.jpg"}`))
		case "/tv/42/season/1/episode/3":
			_, _ = w.Write([]byte(`{"name":"Blue Hour","air_date":"2024-01-22","overview":"Refreshed episode three.","vote_average":8.3,"still_path":"/episode-3.jpg"}`))
		default:
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
	}))
	defer tmdb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "test-token", TMDBBaseURL: tmdb.URL})
	if _, err := server.db.Exec(`UPDATE media_items SET season_number = 0 WHERE id = 'season_northbridge_1'`); err != nil {
		t.Fatalf("seed legacy season number: %v", err)
	}
	show, err := server.getMediaDetail("", "show_northbridge")
	if err != nil {
		t.Fatalf("load show: %v", err)
	}
	updated, count, errs := server.refreshMediaMetadataCascade(context.Background(), show)
	if len(errs) > 0 {
		t.Fatalf("cascade errors: %v", errs)
	}
	if count != 5 {
		t.Fatalf("refreshed count = %d", count)
	}
	if updated.Summary != "Refreshed show overview." || updated.Artwork["posterPath"] != "/show-poster.jpg" {
		t.Fatalf("updated show = %#v", updated)
	}
	season, err := server.getMediaDetail("", "season_northbridge_1")
	if err != nil {
		t.Fatalf("load season: %v", err)
	}
	if season.Title != "Season One" || season.Artwork["posterPath"] != "/season-poster.jpg" {
		t.Fatalf("season metadata = %#v", season)
	}
	if season.SeasonNumber != 1 {
		t.Fatalf("season number was not repaired from scanned index: %#v", season)
	}
	episode, err := server.getMediaDetail("", "episode_northbridge_102")
	if err != nil {
		t.Fatalf("load episode: %v", err)
	}
	if episode.Summary != "Refreshed episode two." || episode.Artwork["thumbPath"] != "/episode-2.jpg" {
		t.Fatalf("episode metadata = %#v", episode)
	}
	if searches != 1 {
		t.Fatalf("search count = %d", searches)
	}
}

func TestTVArtworkInheritanceDoesNotClaimRelatedPosterAsItemMetadata(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE media_items SET artwork_json = '{"source":"tmdb","posterPath":"/show-poster.jpg"}' WHERE id = 'show_northbridge'`); err != nil {
		t.Fatalf("update show artwork: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET artwork_json = '{}' WHERE id IN ('season_northbridge_1', 'episode_northbridge_102')`); err != nil {
		t.Fatalf("clear child artwork: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_images (id, media_id, image_type, source, provider, path, remote_url, preferred, created_at)
		VALUES ('stale_season_poster', 'season_northbridge_1', 'poster', 'local', '', '/missing/season.jpg', '', 1, ?)`,
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed stale season image: %v", err)
	}
	season, err := server.getMediaDetail("", "season_northbridge_1")
	if err != nil {
		t.Fatalf("load season: %v", err)
	}
	if !strings.Contains(season.Images.Poster, "/api/artwork/season_northbridge_1/poster.svg") {
		t.Fatalf("season poster URL must continue to describe the season artwork slot: %s", season.Images.Poster)
	}
	inherited, inheritedKind, ok := server.inheritedArtworkItem("", season, "poster")
	if !ok || inherited.ID != "show_northbridge" || inheritedKind != "poster" {
		t.Fatalf("season inherited artwork = %#v kind=%q ok=%v", inherited, inheritedKind, ok)
	}
	previousSeasonPoster := season.Images.Poster
	if _, err := server.db.Exec(`UPDATE media_items SET artwork_json = '{"source":"tmdb","posterPath":"/show-poster-2.jpg"}' WHERE id = 'show_northbridge'`); err != nil {
		t.Fatalf("update show artwork again: %v", err)
	}
	season, err = server.getMediaDetail("", "season_northbridge_1")
	if err != nil {
		t.Fatalf("reload season: %v", err)
	}
	if season.Images.Poster != previousSeasonPoster {
		t.Fatalf("changing related show artwork must not rewrite the season artwork slot: before=%s after=%s", previousSeasonPoster, season.Images.Poster)
	}
	episode, err := server.getMediaDetail("", "episode_northbridge_102")
	if err != nil {
		t.Fatalf("load episode: %v", err)
	}
	if !strings.Contains(episode.Images.Poster, "/api/artwork/episode_northbridge_102/poster.svg") {
		t.Fatalf("episode poster URL must continue to describe the episode artwork slot: %s", episode.Images.Poster)
	}
	inherited, inheritedKind, ok = server.inheritedArtworkItem("", episode, "poster")
	if !ok || inherited.ID != "show_northbridge" || inheritedKind != "poster" {
		t.Fatalf("episode poster inherited artwork = %#v kind=%q ok=%v", inherited, inheritedKind, ok)
	}
	if _, _, ok = server.inheritedArtworkItem("", episode, "thumb"); ok {
		t.Fatalf("episode thumbnails should use episode stills or generated file thumbnails, not show poster inheritance")
	}
	if len(episode.Artwork) != 0 {
		t.Fatalf("presentation inheritance must not fill episode artwork metadata: %#v", episode.Artwork)
	}
}

func TestTVBackdropInheritanceIsPresentationOnly(t *testing.T) {
	server := newScannerTestServer(t)
	showBackdrop := filepath.Join(server.cfg.AppDataDir, "test-artwork", "show-backdrop.jpg")
	if err := os.MkdirAll(filepath.Dir(showBackdrop), 0o700); err != nil {
		t.Fatalf("create artwork directory: %v", err)
	}
	if err := os.WriteFile(showBackdrop, []byte("show-backdrop"), 0o600); err != nil {
		t.Fatalf("write show backdrop: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_images (id, media_id, image_type, source, provider, path, remote_url, preferred, created_at)
		VALUES ('show_backdrop', 'show_northbridge', 'backdrop', 'provider', 'tmdb', ?, '', 1, ?)`,
		showBackdrop, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed show backdrop: %v", err)
	}
	if _, err := server.db.Exec(`DELETE FROM media_images WHERE media_id = 'episode_northbridge_102' AND image_type = 'backdrop'`); err != nil {
		t.Fatalf("clear episode backdrops: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET artwork_json = '{}' WHERE id = 'episode_northbridge_102'`); err != nil {
		t.Fatalf("clear episode artwork metadata: %v", err)
	}

	episode, err := server.getMediaDetail("", "episode_northbridge_102")
	if err != nil {
		t.Fatalf("load episode: %v", err)
	}
	if episode.DisplayImages == nil || !strings.Contains(episode.DisplayImages.Backdrop, "/api/artwork/show_northbridge/backdrop.svg") {
		t.Fatalf("episode display backdrop = %#v", episode.DisplayImages)
	}
	if !strings.Contains(episode.Images.Backdrop, "/api/artwork/episode_northbridge_102/backdrop.svg") {
		t.Fatalf("episode-owned backdrop slot was rewritten: %s", episode.Images.Backdrop)
	}
	if len(episode.Artwork) != 0 {
		t.Fatalf("presentation fallback leaked into artwork metadata: %#v", episode.Artwork)
	}
	for _, image := range episode.MediaImages {
		if image.Type == "backdrop" {
			t.Fatalf("presentation fallback leaked into episode media images: %#v", episode.MediaImages)
		}
	}
	playbackEpisode, err := server.getMediaPlaybackDetailForUser(context.Background(), User{}, "episode_northbridge_102")
	if err != nil {
		t.Fatalf("load playback episode: %v", err)
	}
	if playbackEpisode.DisplayImages == nil || playbackEpisode.DisplayImages.Backdrop != episode.DisplayImages.Backdrop {
		t.Fatalf("playback did not receive the same presentation-only fallback: detail=%#v playback=%#v", episode.DisplayImages, playbackEpisode.DisplayImages)
	}
}

func TestMetadataRefreshCascadesAlbumThroughTracks(t *testing.T) {
	musicBrainzThrottleMu.Lock()
	musicBrainzLastRequest = time.Time{}
	musicBrainzThrottleMu.Unlock()
	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/release-group":
			_, _ = w.Write([]byte(`{"release-groups":[{
				"id":"rg-album-only",
				"title":"Late Trains for Bright Cities",
				"first-release-date":"2024-03-01",
				"artist-credit":[{"name":"Mara Vale","artist":{"id":"artist-album-only","name":"Mara Vale"}}],
				"releases":[{"id":"release-album-only","date":"2024-03-01"}],
				"genres":[{"name":"electronic","count":4}]
			}]}`))
		case "/recording":
			_, _ = w.Write([]byte(`{"recordings":[{
				"id":"rec-album-cascade","title":"Platform Lights (Cascade)","length":245000,
				"artist-credit":[{"name":"Mara Vale","artist":{"id":"artist-album-only","name":"Mara Vale"}}],
				"releases":[{"id":"release-album-only","title":"Late Trains for Bright Cities","date":"2024-03-01","release-group":{"id":"rg-album-only","title":"Late Trains for Bright Cities"}}]
			}]}`))
		default:
			t.Fatalf("unexpected MusicBrainz path: %s", r.URL.Path)
		}
	}))
	defer mb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{MusicBrainzBaseURL: mb.URL})
	album, err := server.getMediaDetail("", "album_mara")
	if err != nil {
		t.Fatalf("load album: %v", err)
	}
	_, count, errs := server.refreshMediaMetadataCascade(context.Background(), album)
	if len(errs) > 0 {
		t.Fatalf("cascade errors: %v", errs)
	}
	if count != 2 {
		t.Fatalf("refreshed count = %d", count)
	}
	track, err := server.getMediaDetail("", "track_mara_01")
	if err != nil {
		t.Fatalf("load track: %v", err)
	}
	if track.TypedMetadata["recordingID"] != "rec-album-cascade" {
		t.Fatalf("track was not refreshed by album cascade: %#v", track.TypedMetadata)
	}
}

func TestMetadataRefreshCascadesArtistThroughAlbumsAndTracks(t *testing.T) {
	musicBrainzThrottleMu.Lock()
	musicBrainzLastRequest = time.Time{}
	musicBrainzThrottleMu.Unlock()
	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/artist":
			_, _ = w.Write([]byte(`{"artists":[{"id":"artist-cascade","name":"Mara Vale","sort-name":"Vale, Mara","disambiguation":"cascade artist"}]}`))
		case "/release-group":
			_, _ = w.Write([]byte(`{"release-groups":[{
				"id":"rg-cascade",
				"title":"Late Trains for Bright Cities",
				"first-release-date":"2024-03-01",
				"artist-credit":[{"name":"Mara Vale","artist":{"id":"artist-cascade","name":"Mara Vale"}}],
				"releases":[{"id":"release-cascade","date":"2024-03-01"}],
				"genres":[{"name":"electronic","count":4}]
			}]}`))
		case "/recording":
			_, _ = w.Write([]byte(`{"recordings":[{
				"id":"rec-artist-cascade","title":"Platform Lights (Artist Cascade)","length":245000,
				"artist-credit":[{"name":"Mara Vale","artist":{"id":"artist-cascade","name":"Mara Vale"}}],
				"releases":[{"id":"release-cascade","title":"Late Trains for Bright Cities","date":"2024-03-01","release-group":{"id":"rg-cascade","title":"Late Trains for Bright Cities"}}]
			}]}`))
		default:
			t.Fatalf("unexpected MusicBrainz path: %s", r.URL.Path)
		}
	}))
	defer mb.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{MusicBrainzBaseURL: mb.URL})
	artist, err := server.getMediaDetail("", "artist_mara")
	if err != nil {
		t.Fatalf("load artist: %v", err)
	}
	if len(artist.Children) != 1 || len(artist.Children[0].Children) != 1 {
		t.Fatalf("artist hierarchy was not hydrated for cascade: %#v", artist.Children)
	}
	_, count, errs := server.refreshMediaMetadataCascade(context.Background(), artist)
	if len(errs) > 0 {
		t.Fatalf("cascade errors: %v", errs)
	}
	if count != 3 {
		t.Fatalf("refreshed count = %d", count)
	}
	track, err := server.getMediaDetail("", "track_mara_01")
	if err != nil {
		t.Fatalf("load track: %v", err)
	}
	if track.TypedMetadata["recordingID"] != "rec-artist-cascade" {
		t.Fatalf("track was not refreshed by artist cascade: %#v", track.TypedMetadata)
	}
	if id, ok := server.mediaProviderID("album_mara", "musicbrainz", "release-group"); !ok || id != "rg-cascade" {
		t.Fatalf("album release-group id = %q ok=%v", id, ok)
	}
}

func TestCleanProviderSearchTitleRemovesReleaseInfo(t *testing.T) {
	got := cleanProviderSearchTitle("F1.The.Movie.2025.HYBRID.1080p.BluRay.x264.AAC5.1-[YTS.MX]")
	if got != "F1 The Movie" {
		t.Fatalf("cleaned title = %q", got)
	}
	got = cleanProviderSearchTitle("The Rookie S08E15 Survive the Streets 1080p AMZN WEB-DL H264-Kitsune")
	if got != "The Rookie S08E15 Survive the Streets" {
		t.Fatalf("cleaned episode title = %q", got)
	}
	got = cleanProviderSearchTitle("I LIKE TO SEE THE WHEELS TURN - K.C. IRVING (1981 Documentary)")
	if got != "I LIKE TO SEE THE WHEELS TURN" {
		t.Fatalf("cleaned credited title = %q", got)
	}
}

func TestTMDBQueryTitleCandidatesIncludeFuzzyFallbacks(t *testing.T) {
	candidates := tmdbQueryTitleCandidates("I LIKE TO SEE THE WHEELS TURN K C IRVING")
	if !stringSliceContains(candidates, "I LIKE TO SEE THE WHEELS TURN") {
		t.Fatalf("fuzzy candidates = %#v, expected title without trailing credit words", candidates)
	}
	candidates = tmdbQueryTitleCandidates("Mission Impossible - Dead Reckoning Part One 2023")
	if !stringSliceContains(candidates, "Mission Impossible Dead Reckoning Part One") {
		t.Fatalf("dash title candidates = %#v, expected canonical dashed title to remain searchable", candidates)
	}
}

func TestMetadataProviderSelectionUsesSettings(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'metadataAgents'`, `{"movies":"None","tv":"TMDB","anime":"AniList","music":"MusicBrainz","localNFO":true,"embeddedTags":true,"refreshDays":7}`); err != nil {
		t.Fatalf("save metadata settings: %v", err)
	}
	if provider := server.metadataProviderFor("movie"); provider != "none" {
		t.Fatalf("movie provider = %s, expected none", provider)
	}
	if provider := server.metadataProviderFor("show"); provider != "tmdb" {
		t.Fatalf("show provider = %s, expected tmdb", provider)
	}
	if provider := server.metadataProviderFor("anime"); provider != "anilist" {
		t.Fatalf("anime provider = %s, expected anilist", provider)
	}
	if provider := server.metadataProviderFor("music"); provider != "musicbrainz" {
		t.Fatalf("music provider = %s, expected musicbrainz", provider)
	}
}

func TestMetadataProviderSelectionUsesLibraryOverride(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'metadataAgents'`, `{"movies":"TMDB","tv":"TMDB","localNFO":true,"embeddedTags":true,"refreshDays":7}`); err != nil {
		t.Fatalf("save metadata settings: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json = ? WHERE id = ?`, `{"metadataProvider":"None","localMetadataMode":"prefer","metadataLanguage":"fr-FR"}`, "lib_movies"); err != nil {
		t.Fatalf("save library settings: %v", err)
	}
	item, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load movie: %v", err)
	}
	if provider := server.metadataProviderForItem(item); provider != "none" {
		t.Fatalf("provider = %s, expected none", provider)
	}
	if language := server.metadataLanguageForItem(item); language != "fr-FR" {
		t.Fatalf("language = %s, expected fr-FR", language)
	}
}

func TestMetadataProviderSelectionUsesLibraryOrderAndDisabledProviders(t *testing.T) {
	server := newScannerTestServer(t)
	server.cfg.TMDBAPIKey = "runtime-owner-api-key"
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json = ? WHERE id = ?`, `{"metadataProviderOrder":"MusicBrainz, Local Media Assets, TMDB","disabledMetadataProviders":"local"}`, "lib_movies"); err != nil {
		t.Fatalf("save library provider order: %v", err)
	}
	item, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load movie: %v", err)
	}
	if provider := server.metadataProviderForItem(item); provider != "tmdb" {
		t.Fatalf("provider = %s, expected tmdb", provider)
	}
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json = ? WHERE id = ?`, `{"metadataProviderOrder":"MusicBrainz, Local Media Assets, TMDB"}`, "lib_movies"); err != nil {
		t.Fatalf("save library provider order without disabled provider: %v", err)
	}
	item, err = server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("reload movie: %v", err)
	}
	if provider := server.metadataProviderForItem(item); provider != "tmdb" {
		t.Fatalf("provider = %s, expected tmdb", provider)
	}
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json = ? WHERE id = ?`, `{"metadataProviderOrder":"TMDB","disabledMetadataProviders":"TMDB"}`, "lib_movies"); err != nil {
		t.Fatalf("save library provider order with disabled provider: %v", err)
	}
	item, err = server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("reload movie with disabled provider: %v", err)
	}
	if provider := server.metadataProviderForItem(item); provider != "none" {
		t.Fatalf("provider = %s, expected none", provider)
	}
}

func TestMetadataProviderSelectionDegradesWithoutTMDBCredentials(t *testing.T) {
	server := newScannerTestServer(t)
	item, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load movie: %v", err)
	}
	if provider := server.metadataProviderForItem(item); provider != "none" {
		t.Fatalf("provider = %s, expected no-credential degradation", provider)
	}
	if server.automaticMetadataRefreshSupported(item) {
		t.Fatal("automatic metadata refresh should be disabled without TMDB credentials")
	}
}

func TestMetadataHealthMissingArtworkActionQueuesRefreshJobs(t *testing.T) {
	server := newScannerTestServer(t)
	// This test exercises health remediation and queueing, not the production
	// credential gate. Give the fixture an owner-supplied test credential so
	// missing-artwork candidates are eligible for the refresh action.
	server.cfg.TMDBAPIKey = "test-api-key"
	release := server.acquireJobLane("metadata_refresh")
	defer release()
	if _, err := server.db.Exec(`UPDATE media_items SET artwork_json = '{}' WHERE id = 'movie_meridian'`); err != nil {
		t.Fatalf("clear movie artwork json: %v", err)
	}
	if _, err := server.db.Exec(`DELETE FROM media_images WHERE media_id = 'movie_meridian'`); err != nil {
		t.Fatalf("clear movie images: %v", err)
	}

	action, err := server.queueMissingArtworkRefreshesContext(context.Background(), "lib_movies", 10)
	if err != nil {
		t.Fatalf("queue missing artwork refreshes: %v", err)
	}
	if action.Queued == 0 {
		t.Fatalf("queued = 0, action = %#v", action)
	}
	if action.Category != "missing_artwork" || action.Total == 0 {
		t.Fatalf("unexpected action summary: %#v", action)
	}
	if len(action.Jobs) != action.Queued {
		t.Fatalf("jobs length = %d, queued = %d", len(action.Jobs), action.Queued)
	}

	var count int
	if err := server.db.QueryRow(`
		SELECT COUNT(*)
		FROM jobs
		WHERE type = 'metadata_refresh'
			AND resource_type = 'media'
			AND resource_id = 'movie_meridian'
			AND json_extract(metadata_json, '$.healthCategory') = 'missing_artwork'`).Scan(&count); err != nil {
		t.Fatalf("count metadata refresh jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("missing artwork refresh jobs = %d, expected 1", count)
	}
}

func TestNormalizeMetadataLanguage(t *testing.T) {
	cases := map[string]string{
		"":      "en-US",
		"fr":    "fr-FR",
		"es_es": "es-ES",
		"ja-JP": "ja-JP",
		"pt-BR": "pt-BR",
	}
	for input, expected := range cases {
		if actual := normalizeMetadataLanguage(input); actual != expected {
			t.Fatalf("normalizeMetadataLanguage(%q) = %q, expected %q", input, actual, expected)
		}
	}
}
