package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestTVDBRefreshUsesNFOProviderIdentityAndCachesToken(t *testing.T) {
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEElEQVR4nGL6z8AACAAA//8DCQECWLbVUAAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	previousClient := providerArtworkHTTPClient
	providerArtworkHTTPClient = &http.Client{Transport: metadataRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "artworks.thetvdb.com" {
			t.Fatalf("unexpected artwork host: %s", req.URL.Host)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(pngBytes)), Request: req}, nil
	})}
	defer func() { providerArtworkHTTPClient = previousClient }()

	var logins atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login":
			logins.Add(1)
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode login: %v", err)
			}
			if request["apikey"] != "portico-test-key" {
				t.Fatalf("apikey = %q", request["apikey"])
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"token":"short-lived-test-token"}}`))
		case "/movies/42/extended":
			if r.Header.Get("Authorization") != "Bearer short-lived-test-token" {
				t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":42,"name":"The Meridian Job","overview":"Resolved through the accepted TheTVDB identity.","firstAired":"2025-02-14","score":8.1,"image":"https://artworks.thetvdb.com/example.jpg","genres":[{"id":1,"name":"Drama"}],"remoteIds":[{"id":"tt0042","sourceName":"IMDB"}]}}`))
		default:
			t.Fatalf("unexpected TheTVDB path: %s", r.URL.Path)
		}
	}))
	defer provider.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TVDBAPIKey: "portico-test-key", TVDBBaseURL: provider.URL})
	if err := server.upsertMediaProviderID("movie_meridian", "tvdb", "42", "movie", 1, "nfo"); err != nil {
		t.Fatalf("upsert provider id: %v", err)
	}
	before, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatalf("load media: %v", err)
	}
	after, err := server.refreshMediaMetadataFromTVDB(context.Background(), before)
	if err != nil {
		t.Fatalf("refresh metadata: %v", err)
	}
	if after.Title != "The Meridian Job" || after.Summary != "Resolved through the accepted TheTVDB identity." {
		t.Fatalf("metadata = %#v", after)
	}
	if _, err := server.tvdbDetails(context.Background(), "movie", 42); err != nil {
		t.Fatalf("second details request: %v", err)
	}
	if got := logins.Load(); got != 1 {
		t.Fatalf("login requests = %d, want 1", got)
	}
	var crosswalkID, crosswalkStatus string
	if err := server.db.QueryRow(`SELECT external_id, status FROM media_provider_ids WHERE media_id = 'movie_meridian' AND provider = 'imdb' AND external_type = 'movie'`).Scan(&crosswalkID, &crosswalkStatus); err != nil {
		t.Fatalf("load IMDb crosswalk evidence: %v", err)
	}
	if crosswalkID != "tt0042" || crosswalkStatus != string(metadataIdentityCandidate) {
		t.Fatalf("IMDb crosswalk evidence = %q status %q", crosswalkID, crosswalkStatus)
	}
	var localPoster string
	if err := server.db.QueryRow(`SELECT path FROM media_images WHERE media_id = 'movie_meridian' AND provider = 'tvdb' AND image_type = 'poster' AND selection_state = 'accepted' AND preferred = 1`).Scan(&localPoster); err != nil {
		t.Fatalf("load localized TVDB artwork: %v", err)
	}
	if _, err := os.Stat(localPoster); err != nil {
		t.Fatalf("localized TVDB artwork is unavailable: %v", err)
	}
}

func TestTVDBOwnerOverrideIsUsedWithoutReturningSecret(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode login: %v", err)
		}
		if request["apikey"] != "owner-tvdb-key" {
			t.Fatalf("apikey = %q", request["apikey"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"token":"owner-token"}}`))
	}))
	defer provider.Close()

	_, _, server := newDiscoveryTestServer(t, config.Config{TVDBAPIKey: "bundled-key", TVDBBaseURL: provider.URL})
	if err := server.saveSecretSetting(tvdbAPIKeySettingKey, "owner-tvdb-key"); err != nil {
		t.Fatalf("save override: %v", err)
	}
	if _, err := server.tvdbAccessToken(context.Background(), false); err != nil {
		t.Fatalf("login: %v", err)
	}
	settings, err := server.loadSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	body, _ := json.Marshal(server.clientSettings(settings))
	if strings.Contains(string(body), "owner-tvdb-key") || !strings.Contains(string(body), `"tvdbAPIKey":{"present":true}`) {
		t.Fatalf("client settings = %s", body)
	}
}

func TestManualTVDBIdentityRemainsProviderScopedForRefresh(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "tmdb-key", TVDBAPIKey: "tvdb-key"})
	item, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		INSERT INTO media_provider_ids (
			media_id, provider, external_id, external_type, confidence, source, status,
			observed_at, accepted_at, accepted_by_user_id, created_at, updated_at
		) VALUES (?, 'tvdb', '42', 'movie', 1, 'manual-match', 'accepted', ?, ?, 'owner', ?, ?)`, item.ID, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if provider := server.metadataProviderForItem(item); provider != "tvdb" {
		t.Fatalf("refresh provider = %q, expected manually selected tvdb", provider)
	}
}

func TestTVDBBoundedResponseAndUnauthorizedTokenRefresh(t *testing.T) {
	var logins atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/login" {
			count := logins.Add(1)
			_, _ = w.Write([]byte(`{"status":"success","data":{"token":"token-` + string(rune('0'+count)) + `"}}`))
			return
		}
		if r.Header.Get("Authorization") == "Bearer token-1" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":"failure"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":[]}`))
	}))
	defer provider.Close()
	server := &Server{cfg: config.Config{TVDBAPIKey: "key", TVDBBaseURL: provider.URL}}
	var result tvdbEnvelope[[]tvdbSearchResult]
	if err := server.getTVDB(context.Background(), "/search", map[string]string{"query": "Meridian"}, &result); err != nil {
		t.Fatalf("request after token refresh: %v", err)
	}
	if got := logins.Load(); got != 2 {
		t.Fatalf("login requests = %d, want 2", got)
	}
}

func TestMetadataRefreshFallsBackFromTMDBToTVDBOnNoMatch(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/movie":
			_, _ = w.Write([]byte(`{"page":1,"results":[],"total_pages":0,"total_results":0}`))
		case "/login":
			_, _ = w.Write([]byte(`{"status":"success","data":{"token":"fallback-token"}}`))
		case "/search":
			_, _ = w.Write([]byte(`{"status":"success","data":[{"tvdb_id":"42","name":"The Meridian Job","year":"2025","type":"movie","overview":"TVDB fallback candidate"}]}`))
		case "/movies/42/extended":
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":42,"name":"Meridian","overview":"Resolved by the configured TheTVDB fallback.","firstAired":"2025-01-01","score":8.2}}`))
		default:
			t.Fatalf("unexpected metadata path: %s", r.URL.Path)
		}
	}))
	defer provider.Close()
	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "tmdb-key", TMDBBaseURL: provider.URL, TVDBAPIKey: "tvdb-key", TVDBBaseURL: provider.URL})
	item, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := server.refreshMediaMetadata(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Summary != "Resolved by the configured TheTVDB fallback." {
		t.Fatalf("updated=%+v", updated)
	}
	if id, ok := server.mediaProviderID(updated.ID, "tvdb", "movie"); !ok || id != "42" {
		t.Fatalf("tvdb identity=%q ok=%v", id, ok)
	}
}

func TestMetadataRefreshFallsBackFromTMDBProviderFailure(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/movie":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status_message":"temporarily unavailable"}`))
		case "/login":
			_, _ = w.Write([]byte(`{"status":"success","data":{"token":"fallback-token"}}`))
		case "/search":
			_, _ = w.Write([]byte(`{"status":"success","data":[{"tvdb_id":"42","name":"The Meridian Job","year":"2025","type":"movie"}]}`))
		case "/movies/42/extended":
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":42,"name":"Meridian","overview":"Resolved after primary provider failure.","firstAired":"2025-01-01"}}`))
		default:
			t.Fatalf("unexpected metadata path: %s", r.URL.Path)
		}
	}))
	defer provider.Close()
	_, _, server := newDiscoveryTestServer(t, config.Config{TMDBAPIKey: "tmdb-key", TMDBBaseURL: provider.URL, TVDBAPIKey: "tvdb-key", TVDBBaseURL: provider.URL})
	item, err := server.getMediaDetail("", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := server.refreshMediaMetadata(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Summary != "Resolved after primary provider failure." {
		t.Fatalf("updated=%+v", updated)
	}
}

func TestTVDBAutomaticMatchRejectsWrongEntityKind(t *testing.T) {
	item := MediaItem{Type: "movie", Title: "The Meridian Job", Year: 2025}
	result := bestTVDBResult([]tvdbSearchResult{{TVDBID: "42", Name: "The Meridian Job", Year: "2025", Type: "series"}}, item, []string{item.Title}, item.Year)
	if result.TVDBID != "" {
		t.Fatalf("wrong-kind TVDB candidate was accepted: %#v", result)
	}
}
