package httpapi

import (
	"net/http"
	"strings"
)

// titleRoutePrefixes are the routes whose {id} is a catalogue title.
var titleRoutePrefixes = []string{"/v1/items/", "/v1/shows/", "/v1/seasons/", "/v1/albums/", "/v1/artists/", "/v1/books/", "/v1/containers/", "/v1/admin/items/"}

// mergedTitlePath sends a request that names a merged-away title to the title
// that absorbed it, so old links, bookmarks and offline receipts keep working.
func (d Dependencies) mergedTitlePath(q *http.Request, name, value string) string {
	if d.Catalog == nil {
		return value
	}
	if name == "id" && strings.HasPrefix(routePathOf(q.Pattern), "/v1/people/") {
		// A person first known by name alone keeps their old page id.
		return d.Catalog.SurvivingPersonID(q.Context(), value)
	}
	if !titlePathValue(q.Pattern, name) {
		return value
	}
	return d.Catalog.SurvivingTitleID(q.Context(), value)
}

func routePathOf(pattern string) string {
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		return pattern[i+1:]
	}
	return pattern
}

func titlePathValue(pattern, name string) bool {
	if name == "itemId" {
		return true
	}
	if name != "id" {
		return false
	}
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		pattern = pattern[i+1:]
	}
	for _, prefix := range titleRoutePrefixes {
		if strings.HasPrefix(pattern, prefix) {
			return true
		}
	}
	return false
}
