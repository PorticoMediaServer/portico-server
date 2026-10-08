package localmetadata

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
)

var errMissing = errors.New("missing")

func sidecarPNG(t *testing.T) []byte {
	t.Helper()
	var raw bytes.Buffer
	if e := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 8, 8))); e != nil {
		t.Fatal(e)
	}
	return raw.Bytes()
}

// fakeSidecarCalls counts reads per path.
func TestReadSidecarsShowSeasonAndRootGuard(t *testing.T) {
	png := sidecarPNG(t)
	present := map[string]bool{
		"/root/Show/poster.jpg":                          true,
		"/root/Show/fanart.jpg":                          true,
		"/root/Show/logo.png":                            true,
		"/root/Show/Season 1/season01.jpg":               true,
		"/root/Show/Season 1/season-specials-poster.jpg": true,
		"/root/Show2/poster.jpg":                         true,
		"/root/logo.png":                                 true,
	}
	calls := map[string]int{}
	service, e := New(t.TempDir(), "", nil)
	if e != nil {
		t.Fatal(e)
	}
	service.readSmall = func(_ context.Context, _ string, path string, _ int64) ([]byte, error) {
		calls[path]++
		if present[path] {
			return png, nil
		}
		return nil, errMissing
	}
	got := service.ReadSidecars(context.Background(), "lib", "tv", "/root", "/root/Show/Season 1/ep.mkv", 1)
	for _, key := range []string{"show/poster", "show/backdrop", "show/logo", "season/1/poster"} {
		if got[key] == "" {
			t.Fatalf("missing %s: %v", key, got)
		}
	}
	if len(got) != 4 {
		t.Fatalf("unexpected keys: %v", got)
	}
	// Nothing above the library root is ever read.
	for path := range calls {
		if !strings.HasPrefix(path, "/root/") {
			t.Fatalf("read above root: %s", path)
		}
	}
	// A second scan shares the cached reads.
	before := len(calls)
	service.ReadSidecars(context.Background(), "lib", "tv", "/root", "/root/Show/Season 1/other.mkv", 1)
	if len(calls) != before {
		t.Fatalf("cache missed: %d vs %d reads", len(calls), before)
	}
	// Movies read logos from their own folder only.
	movie := service.ReadSidecars(context.Background(), "lib", "movie", "/root", "/root/Film/film.mkv", -1)
	if len(movie) != 0 {
		t.Fatalf("movie without logo: %v", movie)
	}
	// A show filed directly under the root never inherits the root's files.
	flat := service.ReadSidecars(context.Background(), "lib", "tv", "/root", "/root/Show2/ep.mkv", -1)
	if flat["show/poster"] == "" || flat["show/logo"] != "" {
		t.Fatalf("root files leaked into a show: %v", flat)
	}
}
