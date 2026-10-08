package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/metadata"
)

// Artwork is bytes that never change, and the handler chain's `no-store`
// default forbade a client from keeping any of them: a fifty-poster grid
// re-downloaded fifty images on every render, every scroll-back and every app
// launch, each paying a full authentication and an availability check on the
// way. This is the gate on that never coming back.
func TestArtworkIsStorableRevalidatedAndConditional(t *testing.T) {
	h, owner, member, db := editorHTTPFixtureDB(t)
	editOne := editorItemPublic(t, db, "Alpha")
	state := editorState(t, h, owner, editOne)
	body, contentType := editorUpload(t, state.Revision, editorPNG(t, 24, 36))
	w := editorRequest(t, h, "POST", "/v1/metadata/item/"+editOne+"/art/poster/upload", owner, contentType, body)
	if w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	var uploaded metadata.RepairState
	if e := json.Unmarshal(w.Body.Bytes(), &uploaded); e != nil {
		t.Fatal(e)
	}
	// The upload selects the poster and projects it onto the item, which
	// queues the item's own publication; settle before reading it back.
	settleCompactCatalogue(t, db)

	first := editorRequest(t, h, "GET", "/v1/items/"+editOne+"/art/poster", owner, "", nil)
	if first.Code != 200 {
		t.Fatalf("artwork: %d %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("Cache-Control"); got != revalidate {
		t.Fatalf("artwork answered with Cache-Control %q; a client may not keep a poster at all under the chain default", got)
	}
	tag := first.Header().Get("ETag")
	if tag == "" {
		t.Fatal("artwork carried no ETag, so `no-cache` would make every use a fresh download")
	}
	if first.Body.Len() == 0 {
		t.Fatal("artwork served no bytes")
	}

	// The same viewer, presenting the validator, gets a bodyless 304.
	repeat := editorRequestWithHeader(t, h, "GET", "/v1/items/"+editOne+"/art/poster", owner, map[string]string{"If-None-Match": tag})
	if repeat.Code != 304 {
		t.Fatalf("a matching If-None-Match answered %d rather than 304", repeat.Code)
	}
	if repeat.Body.Len() != 0 {
		t.Fatalf("a 304 carried %d bytes", repeat.Body.Len())
	}
	if repeat.Header().Get("Cache-Control") != revalidate {
		t.Fatal("the 304 dropped the caching policy")
	}

	// Authorisation still runs on the conditional path: whether a private image
	// exists and whether it has changed are not public facts.
	unauthorised := editorRequestWithHeader(t, h, "GET", "/v1/items/"+editOne+"/art/poster", member, map[string]string{"If-None-Match": tag})
	if unauthorised.Code == 304 || unauthorised.Code == 200 {
		t.Fatalf("a viewer without access was answered %d for a private image", unauthorised.Code)
	}
	if anonymous := editorRequestWithHeader(t, h, "GET", "/v1/items/"+editOne+"/art/poster", "", map[string]string{"If-None-Match": tag}); anonymous.Code != 401 {
		t.Fatalf("an unauthenticated conditional request answered %d", anonymous.Code)
	}

	// An edit changes the bytes, so it must change the validator — this is why
	// the policy is `no-cache` and not a max-age.
	replaced, contentType := editorUpload(t, uploaded.Revision, editorPNG(t, 48, 72))
	if got := editorRequest(t, h, "POST", "/v1/metadata/item/"+editOne+"/art/poster/upload", owner, contentType, replaced); got.Code != 200 {
		t.Fatalf("replace: %d %s", got.Code, got.Body.String())
	}
	settleCompactCatalogue(t, db)
	after := editorRequestWithHeader(t, h, "GET", "/v1/items/"+editOne+"/art/poster", owner, map[string]string{"If-None-Match": tag})
	if after.Code != 200 {
		t.Fatalf("an edited image answered %d to the old validator; the viewer would still be looking at the old poster", after.Code)
	}
	if after.Header().Get("ETag") == tag {
		t.Fatal("the validator did not change when the image did")
	}
}

func TestRestrictedContainerArtworkIsAbsent(t *testing.T) {
	d, owner, _ := tl6SupportFixture(t)
	c := catalogtest.New(t, d.DB)
	library := c.Handle("private-library")
	show := c.Show(library, "Hidden", 2020)
	season := c.Season(show, 1)
	episode := c.Episode(show, season, 1, filepath.Join("/private/SOURCE-PATH", "Episode.mkv"))
	c.Attributes(episode.ID, "contentRating", "R")
	c.Drain()
	if _, err := d.DB.Exec(`INSERT INTO profile_restrictions(profile_id,maximum_age,allow_unrated) VALUES('profile',13,0)`); err != nil {
		t.Fatal(err)
	}
	d.Catalog = catalog.New(d.DB)
	d.Metadata = metadata.New(d.DB, "test-token")
	h := New(d)
	for _, target := range []struct{ kind, id string }{{"show", show.Public}, {"season", season.Public}} {
		response := editorRequest(t, h, "GET", "/v1/metadata/"+target.kind+"/"+target.id+"/art/poster?w=400", owner.AccessToken, "", nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("restricted %s artwork answered %d: %s", target.kind, response.Code, response.Body.String())
		}
	}
}

func TestArtworkInInaccessibleLibraryMatchesUnknownTarget(t *testing.T) {
	h, owner, member, db := editorHTTPFixtureDB(t)
	editOne := editorItemPublic(t, db, "Alpha")
	for _, kind := range []string{"item"} {
		denied := editorRequest(t, h, "GET", "/v1/metadata/"+kind+"/"+editOne+"/art/poster", member, "", nil)
		unknown := editorRequest(t, h, "GET", "/v1/metadata/"+kind+"/missing-art-target/art/poster", owner, "", nil)
		if denied.Code != http.StatusNotFound || unknown.Code != http.StatusNotFound {
			t.Fatalf("%s: inaccessible %d, unknown %d", kind, denied.Code, unknown.Code)
		}
	}
}

func TestArtworkThumbnailHasItsOwnBytesAndValidator(t *testing.T) {
	h, owner, member, db := editorHTTPFixtureDB(t)
	editOne := editorItemPublic(t, db, "Alpha")
	state := editorState(t, h, owner, editOne)
	body, contentType := editorUpload(t, state.Revision, editorPNG(t, 800, 1200))
	if w := editorRequest(t, h, "POST", "/v1/metadata/item/"+editOne+"/art/poster/upload", owner, contentType, body); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	settleCompactCatalogue(t, db)
	full := editorRequest(t, h, "GET", "/v1/items/"+editOne+"/art/poster", owner, "", nil)
	thumb := editorRequestWithHeader(t, h, "GET", "/v1/items/"+editOne+"/art/poster?size=thumbnail", owner, map[string]string{"If-None-Match": full.Header().Get("ETag")})
	if thumb.Code != 200 || thumb.Header().Get("ETag") == full.Header().Get("ETag") {
		t.Fatal("thumbnail reused original validator", thumb.Code)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb.Body.Bytes()))
	if err != nil || cfg.Height != 400 {
		t.Fatal(cfg, err)
	}
	cached := editorRequestWithHeader(t, h, "GET", "/v1/items/"+editOne+"/art/poster?size=thumbnail", owner, map[string]string{"If-None-Match": thumb.Header().Get("ETag")})
	if cached.Code != 304 {
		t.Fatal(cached.Code)
	}
	denied := editorRequestWithHeader(t, h, "GET", "/v1/items/"+editOne+"/art/poster?size=thumbnail", member, map[string]string{"If-None-Match": thumb.Header().Get("ETag")})
	if denied.Code == 200 || denied.Code == 304 {
		t.Fatal("private thumbnail disclosed")
	}
}

func TestArtworkImmutableWidthsAndReplacedVersion(t *testing.T) {
	h, owner, _, db := editorHTTPFixtureDB(t)
	editOne := editorItemPublic(t, db, "Alpha")
	state := editorState(t, h, owner, editOne)
	body, contentType := editorUpload(t, state.Revision, editorPNG(t, 1600, 2400))
	upload := editorRequest(t, h, "POST", "/v1/metadata/item/"+editOne+"/art/poster/upload", owner, contentType, body)
	if upload.Code != 200 {
		t.Fatal(upload.Code, upload.Body.String())
	}
	var saved metadata.RepairState
	if err := json.Unmarshal(upload.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	settleCompactCatalogue(t, db)
	full := editorRequest(t, h, "GET", "/v1/items/"+editOne+"/art/poster", owner, "", nil)
	version := strings.Trim(full.Header().Get("ETag"), `"`)
	for _, width := range []int{400, 800, 1920} {
		path := fmt.Sprintf("/v1/metadata/item/%s/art/poster?v=%s&w=%d", editOne, version, width)
		response := editorRequest(t, h, "GET", path, owner, "", nil)
		if response.Code != 200 {
			t.Fatal(width, response.Code, response.Body.String())
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
		if err != nil || max(cfg.Width, cfg.Height) != width {
			t.Fatal(width, cfg, err)
		}
		if response.Header().Get("Cache-Control") != "private, max-age=31536000, immutable" {
			t.Fatal(response.Header())
		}
		cached := editorRequestWithHeader(t, h, "GET", path, owner, map[string]string{"If-None-Match": response.Header().Get("ETag")})
		if cached.Code != 304 || cached.Body.Len() != 0 {
			t.Fatal(width, cached.Code)
		}
	}
	for _, tc := range []struct{ requested, bucket int }{{1, 400}, {500, 800}, {801, 1920}, {5000, 1920}} {
		path := fmt.Sprintf("/v1/metadata/item/%s/art/poster?v=%s&w=%d&size=thumbnail", editOne, version, tc.requested)
		response := editorRequest(t, h, "GET", path, owner, "", nil)
		cfg, _, err := image.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
		if response.Code != 200 || err != nil || max(cfg.Width, cfg.Height) != tc.bucket {
			t.Fatalf("width %d should use bucket %d: code=%d cfg=%+v err=%v", tc.requested, tc.bucket, response.Code, cfg, err)
		}
	}
	body, contentType = editorUpload(t, saved.Revision, editorPNG(t, 30, 40))
	if w := editorRequest(t, h, "POST", "/v1/metadata/item/"+editOne+"/art/poster/upload", owner, contentType, body); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	settleCompactCatalogue(t, db)
	stale := editorRequest(t, h, "GET", "/v1/items/"+editOne+"/art/poster?v="+version+"&w=800", owner, "", nil)
	if stale.Code == 200 || stale.Code == 304 || strings.Contains(stale.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("old address served new bytes", stale.Code)
	}
}

func TestArtworkMissingWidthRegeneratesFromSelectedVersion(t *testing.T) {
	d, owner, _ := tl6SupportFixture(t)
	variant := tl6FixtureMovie(t, d, "private-library", "variant-item.mkv", "Variant", 2020)
	service := metadata.New(d.DB, "test-token")
	artworkDir := t.TempDir()
	if err := service.SetArtworkDirectory(artworkDir); err != nil {
		t.Fatal(err)
	}
	d.Metadata = service
	h := New(d)
	state := editorState(t, h, owner.AccessToken, variant.Public)
	body, contentType := editorUpload(t, state.Revision, editorPNG(t, 1600, 2400))
	if response := editorRequest(t, h, "POST", "/v1/metadata/item/"+variant.Public+"/art/poster/upload", owner.AccessToken, contentType, body); response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body.String())
	}
	settleCompactCatalogue(t, d.DB)
	full := editorRequest(t, h, "GET", "/v1/items/"+variant.Public+"/art/poster", owner.AccessToken, "", nil)
	version := strings.Trim(full.Header().Get("ETag"), `"`)
	for _, width := range []int{400, 800} {
		if _, err := d.DB.Exec(`DELETE FROM artwork_variants WHERE source_digest=? AND width=?`, version, width); err != nil {
			t.Fatal(err)
		}
		path := fmt.Sprintf("/v1/metadata/item/%s/art/poster?v=%s&w=%d", variant.Public, version, width)
		response := editorRequest(t, h, "GET", path, owner.AccessToken, "", nil)
		cfg, _, err := image.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
		if response.Code != http.StatusOK || err != nil || max(cfg.Width, cfg.Height) != width {
			t.Fatalf("regenerated %d: code=%d cfg=%+v err=%v", width, response.Code, cfg, err)
		}
		var count int
		if err = d.DB.QueryRow(`SELECT count(*) FROM artwork_variants WHERE source_digest=? AND width=?`, version, width).Scan(&count); err != nil || count != 1 {
			t.Fatalf("regenerated bucket %d not cached: %d %v", width, count, err)
		}
	}
	var variantDigest string
	if err := d.DB.QueryRow(`SELECT digest FROM artwork_variants WHERE source_digest=? AND width=400`, version).Scan(&variantDigest); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(artworkDir, variantDigest+".img")); err != nil {
		t.Fatal(err)
	}
	repaired := editorRequest(t, h, "GET", "/v1/metadata/item/"+variant.Public+"/art/poster?v="+version+"&w=400", owner.AccessToken, "", nil)
	if repaired.Code != http.StatusOK {
		t.Fatalf("missing physical variant was not rebuilt: %d %s", repaired.Code, repaired.Body.String())
	}
}
