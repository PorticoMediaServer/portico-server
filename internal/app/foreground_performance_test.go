package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestForegroundAuthorizationProjectionUsesRequestScopedSnapshotWithoutDatabaseReads(t *testing.T) {
	server := &Server{}
	user := User{
		ID:                  "account-foreground",
		AccountID:           "account-foreground",
		ProfileID:           "profile-foreground",
		ProfileIsPrimary:    true,
		Role:                "member",
		AuthOrigin:          "local",
		Permissions:         map[string]bool{"playMedia": true},
		LibraryIDs:          []string{"library-foreground"},
		TagPolicy:           UserTagPolicy{BlockedTags: []string{"spoiler"}},
		ChannelPolicy:       UserChannelPolicy{AllowedChannelIDs: []string{"channel-foreground"}},
		AllowUnrated:        true,
		MaxContentRating:    "PG-13",
		MaxActiveStreams:    2,
		AllowFeedback:       true,
		Preferences:         UserPreferences{PlaybackProgress: PlaybackProgressPreferences{StartedThresholdPercent: 5}},
		ProfileImageURL:     "/api/profiles/profile-foreground/avatar",
		DisplayName:         "Foreground Viewer",
		PorticoUserID:       "hosted-account",
		PorticoMembershipID: "hosted-membership",
	}
	ctx := contextWithMediaActionUser(context.Background(), user)
	principal, err := server.requestPrincipalForIdentityContext(ctx, user.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if principal.AccountID != user.AccountID || principal.ProfileID != user.ProfileID || principal.MembershipEnvelope.Role != user.Role {
		t.Fatalf("request-scoped principal identity = %+v", principal)
	}
	if len(principal.MembershipEnvelope.LibraryIDs) != 1 || principal.MembershipEnvelope.LibraryIDs[0] != "library-foreground" {
		t.Fatalf("request-scoped library policy = %+v", principal.MembershipEnvelope.LibraryIDs)
	}
	if reads := server.sqliteDiagnostics().ReadOperations; reads != 0 {
		t.Fatalf("request-scoped principal projection performed %d database reads", reads)
	}
	principal.MembershipEnvelope.LibraryIDs[0] = "mutated"
	if user.LibraryIDs[0] != "library-foreground" {
		t.Fatal("request-scoped principal shared mutable policy slices")
	}
}

func TestAppDataArtworkAuthorizationAvoidsCatalogReads(t *testing.T) {
	appDataDir := t.TempDir()
	artworkPath := filepath.Join(appDataDir, "artwork", "provider", "poster.png")
	if err := os.MkdirAll(filepath.Dir(artworkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artworkPath, []byte("local artwork"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: config.Config{AppDataDir: appDataDir}}
	resolved, ok := server.validatedLocalContentPath(context.Background(), artworkPath)
	if !ok || resolved == "" {
		t.Fatalf("app-data artwork was not admitted: resolved=%q ok=%v", resolved, ok)
	}
	if reads := server.sqliteDiagnostics().ReadOperations; reads != 0 {
		t.Fatalf("app-data artwork authorization performed %d catalog reads", reads)
	}
}

func TestArtworkRequestNeverTransformsMissingRendition(t *testing.T) {
	appDataDir := t.TempDir()
	artworkPath := filepath.Join(appDataDir, "artwork", "poster.png")
	if err := os.MkdirAll(filepath.Dir(artworkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	original := image.NewRGBA(image.Rect(0, 0, 128, 192))
	for y := 0; y < 192; y++ {
		for x := 0; x < 128; x++ {
			original.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 72, A: 255})
		}
	}
	if err := png.Encode(&encoded, original); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artworkPath, encoded.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: config.Config{AppDataDir: appDataDir}}
	request := httptest.NewRequest("GET", "/api/artwork/item/poster.svg?rendition=small&v=revision", nil)
	response := httptest.NewRecorder()
	started := time.Now()
	server.serveArtworkFile(response, request, artworkPath, "poster")
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("missing prepared rendition delayed request for %s", elapsed)
	}
	if response.Code != 404 {
		t.Fatalf("missing prepared rendition status=%d, want 404", response.Code)
	}
	if err := server.prepareArtworkRenditions(artworkPath, "poster"); err != nil {
		t.Fatal(err)
	}
	cachePath, ok := server.artworkRenditionCachePath(artworkPath, "poster", artworkRenditionSmall)
	if !ok {
		t.Fatal("small rendition cache path was not resolved")
	}
	before, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.serveArtworkFile(response, request, artworkPath, "poster")
	if response.Code != 200 {
		t.Fatalf("prepared rendition status=%d body=%q, want 200", response.Code, response.Body.String())
	}
	decoded, format, err := image.Decode(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" || decoded.Bounds().Dx() != 128 || decoded.Bounds().Dy() != 192 {
		t.Fatalf("prepared rendition response format=%s bounds=%v", format, decoded.Bounds())
	}
	if got := response.Header().Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
		t.Fatalf("prepared rendition cache control=%q", got)
	}
	if got := response.Header().Get("X-Portico-Artwork-Rendition"); got != "small" {
		t.Fatalf("prepared rendition header=%q", got)
	}
	after, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatal("interactive request modified the prepared rendition")
	}
}

func TestBackgroundMetadataInvalidationDoesNotFanOutToBrowse(t *testing.T) {
	if got, want := metadataApplyInvalidationTags(metadataSourceProvider), []string{"metadata"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("provider invalidation tags=%v, want %v", got, want)
	}
	if got, want := metadataApplyInvalidationTags(metadataSourceManual), []string{"media", "metadata", "library-items", "search", "libraries"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("manual invalidation tags=%v, want %v", got, want)
	}
}

func TestMediaDetailCacheInvalidationTargetsOnlyChangedMedia(t *testing.T) {
	server := &Server{mediaDetailCache: map[string]mediaDetailCacheEntry{
		"profile-a\x00auth-a\x00movie-a\x00false\x00false": {item: MediaItem{ID: "movie-a"}, expiresAt: time.Now().Add(time.Minute)},
		"profile-a\x00auth-a\x00movie-b\x00false\x00false": {item: MediaItem{ID: "movie-b"}, expiresAt: time.Now().Add(time.Minute)},
		"profile-b\x00auth-b\x00movie-a\x00false\x00false": {item: MediaItem{ID: "movie-a"}, expiresAt: time.Now().Add(time.Minute)},
	}}

	server.invalidateMediaDetailCacheForMedia("movie-a")

	if len(server.mediaDetailCache) != 1 {
		t.Fatalf("cache entries = %d, want only unrelated media entry", len(server.mediaDetailCache))
	}
	for key := range server.mediaDetailCache {
		if !strings.Contains(key, "\x00movie-b\x00") {
			t.Fatalf("unexpected surviving cache key %q", key)
		}
	}
}

func TestLibraryDiscoverCacheSingleflightAndInvalidationFence(t *testing.T) {
	server := &Server{
		suggestionsCache:    map[string]suggestionsCacheEntry{},
		suggestionsInFlight: map[string]chan struct{}{},
	}
	user := User{ProfileID: "profile-a", Role: "owner", Permissions: map[string]bool{"playMedia": true}}
	key := libraryDiscoverCacheKey(user, "library-a", 12)
	wait, leader, generation := server.beginSuggestionsFlight(key)
	if !leader {
		t.Fatal("first Discover caller did not own the singleflight")
	}
	follower, followerLeader, _ := server.beginSuggestionsFlight(key)
	if followerLeader || follower != wait {
		t.Fatal("concurrent Discover caller did not join the existing singleflight")
	}
	server.invalidateSuggestionsCache()
	server.storeSuggestionsAtGeneration(key, SuggestionsResponse{Total: 12}, generation)
	if _, ok := server.cachedSuggestions(key); ok {
		t.Fatal("an invalidated in-flight Discover response repopulated the cache")
	}
	server.finishSuggestionsFlight(key, wait)
}

func TestLibraryDiscoveryNormalizationReusesAlreadyHydratedItems(t *testing.T) {
	server := &Server{}
	item := MediaItem{ID: "movie-a", LibraryID: "library-a", Type: "movie", Title: "Movie A", Summary: "Hydrated"}
	items := server.normalizeLibraryDiscoveryItemsContext(context.Background(), "profile-a", Library{ID: "library-a", Type: "movie"}, []MediaItem{item}, 12)
	if len(items) != 1 || items[0].ID != item.ID || items[0].Summary != item.Summary {
		t.Fatalf("normalized items = %#v, want existing hydrated item", items)
	}
}

func TestLibraryDataChangeInvalidatesCachedContentRoots(t *testing.T) {
	server := &Server{
		localContentRoots:           []string{"/old/library/root"},
		localContentRootsLoaded:     true,
		localContentRootsGeneration: 7,
	}
	server.publishDataChanged("data.changed", []string{"libraries"}, "library", "lib-new", nil)
	server.localContentRootsMu.Lock()
	defer server.localContentRootsMu.Unlock()
	if server.localContentRootsLoaded || server.localContentRoots != nil || server.localContentRootsGeneration != 8 {
		t.Fatalf("cached roots survived library change: loaded=%v roots=%v generation=%d", server.localContentRootsLoaded, server.localContentRoots, server.localContentRootsGeneration)
	}
}
