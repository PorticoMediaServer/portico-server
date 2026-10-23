package httpapi

import (
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestArtworkWidthIncludesLargerStorageBucket(t *testing.T) {
	for requested, want := range map[int]int{1: 400, 400: 400, 401: 800, 800: 800, 801: 1920, 1920: 1920, 1921: 2560, 2560: 2560, 9000: 2560} {
		if got := artworkWidth(httptest.NewRequest("GET", "/art?w="+strconv.Itoa(requested), nil)); got != want {
			t.Fatal(requested, got, want)
		}
	}
}
