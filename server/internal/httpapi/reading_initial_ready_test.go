package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestReadingInitialReadyHasNoPollDelayOrHiddenHandle(t *testing.T) {
	f := newReadingFixture(t, "book", "pdf")
	readingSeriesOwnerField(t, f, f.item, "author", "Tiny author")
	readingSeriesDrain(t, f)
	store, _ := readingFacetTestStore(t, f, false)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/libraries/{id}/browse", f.d.readingBrowsePreparedHandler)
	mux.Handle("GET /v1/libraries/{id}/facets", readingFacetTestHandler(t, f))
	base := "/v1/libraries/" + f.library
	for _, path := range []string{"browse", "facets"} {
		method := http.MethodGet
		if path == "browse" {
			method = http.MethodPost
		}
		w := preparedReadingHTTP(t, mux, method, base+"/"+path, f.f.owner.AccessToken, map[string]any{"limit": 1}, nil)
		if w.Code != 200 || w.Header().Get("Location") != "" || w.Header().Get("Retry-After") != "" || strings.Contains(w.Body.String(), `"state":"preparing"`) {
			t.Fatalf("initial %s: %d %s %v", path, w.Code, w.Body.String(), w.Header())
		}
		if stats := store.Stats(); stats.Handles != 0 || stats.Deliveries != 0 || stats.DeliveryBytes != 0 {
			t.Fatalf("initial %s leaked private demand/delivery: %+v", path, stats)
		}
	}
}
