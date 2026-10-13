package httpapi

import (
	"portico.local/server/internal/playback"
	"testing"
)

func TestFailureMemoryIsOwnerOnlyAndCanBeCleared(t *testing.T) {
	f := newV1Fixture(t, 1)
	var page playback.RouteFailurePage
	f.call("GET", "/v1/admin/playback/route-failures", nil, nil, 200, &page)
	if page.Items == nil {
		t.Fatal("missing empty list")
	}
	f.call("DELETE", "/v1/admin/playback/route-failures/file", nil, nil, 204, nil)
	member, _ := f.member()
	for _, method := range []string{"GET", "DELETE"} {
		path := "/v1/admin/playback/route-failures"
		if method == "DELETE" {
			path += "/file"
		}
		w := f.raw(method, path, member, nil, nil)
		if w.Code != 403 && w.Code != 401 {
			t.Fatal("non-owner reached failure memory", method, w.Code, w.Body.String())
		}
	}
}
