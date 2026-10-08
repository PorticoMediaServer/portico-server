package httpapi

import (
	"context"
	"testing"

	"portico.local/server/internal/dbwork"
)

// PERF-12: a grid page carries a validator; revalidating with it answers 304
// until anything is written, and only an authorized viewer gets the 304.
func TestGridProjectionsRevalidateWithTheirValidator(t *testing.T) {
	h, owner, member, db := editorHTTPFixtureDB(t)
	for _, path := range []string{"/v1/libraries/private-library/content?view=browse&limit=20", "/v1/libraries/private-library/categories", "/v1/libraries/private-library/collections"} {
		first := editorRequest(t, h, "GET", path, owner, "", nil)
		tag := first.Header().Get("ETag")
		if first.Code != 200 || tag == "" || first.Header().Get("Cache-Control") != revalidate {
			t.Fatalf("%s: first page %d etag %q cache %q: %s", path, first.Code, tag, first.Header().Get("Cache-Control"), first.Body.String())
		}
		again := editorRequestWithHeader(t, h, "GET", path, owner, map[string]string{"If-None-Match": tag})
		if again.Code != 304 || again.Body.Len() != 0 {
			t.Fatalf("%s: unchanged revalidation %d %q", path, again.Code, again.Body.String())
		}
		// Another viewer's validator never matches, and an unauthorized viewer
		// is refused rather than told the page is unchanged.
		if other := editorRequestWithHeader(t, h, "GET", path, member, map[string]string{"If-None-Match": tag}); other.Code == 304 {
			t.Fatalf("%s: another viewer revalidated the owner's page", path)
		}
		if _, e := dbwork.ExecWrite(context.Background(), db, dbwork.ClassInteractive, `INSERT INTO configuration(key,value) VALUES('grid-etag-test',hex(randomblob(4))) ON CONFLICT(key) DO UPDATE SET value=excluded.value`); e != nil {
			t.Fatal(e)
		}
		after := editorRequestWithHeader(t, h, "GET", path, owner, map[string]string{"If-None-Match": tag})
		if after.Code != 200 || after.Header().Get("ETag") == tag {
			t.Fatalf("%s: a write did not change the validator: %d %q", path, after.Code, after.Header().Get("ETag"))
		}
	}
	// A failed request carries no validator.
	if bad := editorRequest(t, h, "GET", "/v1/libraries/private-library/content?start=x", owner, "", nil); bad.Code == 200 || bad.Header().Get("ETag") != "" {
		t.Fatalf("error response carries a validator: %d %q", bad.Code, bad.Header().Get("ETag"))
	}
}
