package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardAnalyticsDoNotGrantAdministratorPeopleAuthority(t *testing.T) {
	f := administrationFixtureNew(t)
	for _, path := range []string{"/v1/admin/play-history", "/v1/admin/play-history/summary", "/v1/admin/play-history/people", "/v1/admin/play-history/dashboard", "/v1/admin/dashboard/status"} {
		for _, token := range []string{f.admin, f.member} {
			w := f.call("GET", path, token, "")
			if w.Code != 403 {
				t.Fatal(path, w.Code, w.Body.String())
			}
		}
		w := f.call("GET", path, f.owner, "")
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("analytics cached")
		}
	}
	for _, path := range []string{"/v1/admin/sessions", "/v1/admin/access/members"} {
		w := f.call("GET", path, f.admin, "")
		if w.Code != 200 {
			t.Fatal("normal administrator boundary changed", path, w.Code, w.Body.String())
		}
	}
}
func TestMediaMeterCountsSuccessfulPartialWritesOnly(t *testing.T) {
	total := 0
	for _, code := range []int{200, 206, 403, 404, 503} {
		out := httptest.NewRecorder()
		w := &mediaTrafficWriter{ResponseWriter: out, record: func(n int) { total += n }}
		w.WriteHeader(code)
		_, _ = w.Write([]byte("12345"))
	}
	if total != 10 {
		t.Fatal("errors counted as streamed media", total)
	}
	var _ http.ResponseWriter = (*mediaTrafficWriter)(nil)
}
