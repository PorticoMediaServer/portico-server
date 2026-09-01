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

func TestArtworkTransformCapacityFallsBackToOriginalWithoutWaiting(t *testing.T) {
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
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	server := &Server{
		cfg:                      config.Config{AppDataDir: appDataDir},
		artworkWorkSlots:         slots,
		artworkTransformInFlight: map[string]*artworkTransformCall{},
		artworkIngestInFlight:    map[string]*artworkIngestCall{},
	}
	request := httptest.NewRequest("GET", "/api/artwork/item/poster.svg?width=64&height=64", nil)
	response := httptest.NewRecorder()
	started := time.Now()
	server.serveArtworkFile(response, request, artworkPath)
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("busy transform delayed original fallback for %s", elapsed)
	}
	decoded, format, err := image.Decode(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" || decoded.Bounds().Dx() != 128 || decoded.Bounds().Dy() != 192 {
		t.Fatalf("busy transform response format=%s bounds=%v", format, decoded.Bounds())
	}
	if got := response.Header().Get("Cache-Control"); got != "private, max-age=60" {
		t.Fatalf("busy transform fallback cache control=%q", got)
	}
	if got := response.Header().Get("X-Portico-Artwork-Variant"); got != "original-fallback" {
		t.Fatalf("busy transform fallback variant=%q", got)
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
