package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionReviewQueryAllowlist(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{"GET", "/v1/account/sessions?cursor=opaque", true}, {"POST", "/v1/account/sessions?cursor=x", false}, {"GET", "/v1/account/sessions?cursor=x&cursor=y", false}, {"GET", "/v1/account/sessions?cursor=x&accessToken=y", false}, {"GET", "/v1/account/sessions?cursor=%zz", false}, {"GET", "/v1/account/sessions?cursor=", false}, {"GET", "/v1/account/sessions?cursor=" + strings.Repeat("a", 2049), false}, {"GET", "/v1/sessions/refresh?cursor=x", false}} {
		if got := sessionReviewQueryAllowed(httptest.NewRequest(tc.method, tc.path, nil)); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
