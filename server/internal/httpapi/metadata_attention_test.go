package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/metadata"
)

func TestTypedMetadataAttentionAndSettingsRoutes(t *testing.T) {
	d, owner := logoutHTTPFixture(t)
	d.Metadata = metadata.New(d.DB, "")
	c := catalogtest.New(t, d.DB)
	library := c.Library("metadata-fixture-lib", "Films", "movie", "/metadata-fixture")
	item := c.Movie(library, "/metadata-fixture/Film.mp4", "Film", 2008)
	c.Drain()
	handler := New(d)
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	settings := call("GET", "/v1/admin/metadata/settings", "", owner.AccessToken)
	if settings.Code != 200 {
		t.Fatal(settings.Code, settings.Body.String())
	}
	var current metadata.MetadataSettings
	if e := json.Unmarshal(settings.Body.Bytes(), &current); e != nil {
		t.Fatal(e)
	}
	token := "fixture-owner-token"
	body, _ := json.Marshal(metadata.MetadataSettingsUpdate{ExpectedRevision: current.Revision, LocalArtworkLimitBytes: 4 << 30, TMDBReadToken: &token})
	saved := call("PUT", "/v1/admin/metadata/settings", string(body), owner.AccessToken)
	if saved.Code != 200 || strings.Contains(saved.Body.String(), token) {
		t.Fatal("settings response", saved.Code, saved.Body.String())
	}
	stale := call("PUT", "/v1/admin/metadata/settings", string(body), owner.AccessToken)
	if stale.Code != 412 {
		t.Fatal("stale settings accepted", stale.Code)
	}
	path := "/v1/libraries/metadata-fixture-lib/metadata/attention"
	list := call("GET", path, "", owner.AccessToken)
	if list.Code != 200 {
		t.Fatal(list.Code, list.Body.String())
	}
	var page metadata.AttentionPage
	if e := json.Unmarshal(list.Body.Bytes(), &page); e != nil || len(page.Entries) != 1 {
		t.Fatal(page, e)
	}
	body, _ = json.Marshal(metadata.AttentionCommand{Kind: "item", EntityID: item.Public, ExpectedRevision: page.Entries[0].Revision, Action: "dismiss"})
	if result := call("POST", path, string(body), owner.AccessToken); result.Code != 202 {
		t.Fatal(result.Code, result.Body.String())
	}
	result := call("POST", "/v1/items/"+item.Public+"/metadata/refresh", "{}", owner.AccessToken)
	if result.Code != 202 {
		t.Fatal(result.Code, result.Body.String())
	}
	for _, route := range []string{path, "/v1/admin/metadata/settings"} {
		if denied := call("GET", route, "", ""); denied.Code != 401 {
			t.Fatal("unauthed metadata route", route, denied.Code)
		}
	}
}

func TestGlobalMetadataSettingsAndRefreshHTTPContract(t *testing.T) {
	d, owner := logoutHTTPFixture(t)
	d.Metadata = metadata.New(d.DB, "")
	handler := New(d)
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	read := call("GET", "/v1/admin/metadata/settings", "", owner.AccessToken)
	if read.Code != 200 {
		t.Fatal(read.Code, read.Body.String())
	}
	var current metadata.MetadataSettings
	if e := json.Unmarshal(read.Body.Bytes(), &current); e != nil {
		t.Fatal(e)
	}
	key, lang, region, anime, refresh := "fixture-private-acoustid", "ja", "JP", "native", "never"
	online := true
	body, _ := json.Marshal(metadata.MetadataSettingsUpdate{ExpectedRevision: current.Revision, LocalArtworkLimitBytes: current.LocalArtworkLimitBytes, DefaultLanguage: &lang, DefaultRegion: &region, AnimeTitles: &anime, RefreshPolicy: &refresh, AcoustIDKey: &key, OnlineLookups: &online})
	saved := call("PUT", "/v1/admin/metadata/settings", string(body), owner.AccessToken)
	if saved.Code != 200 || strings.Contains(saved.Body.String(), key) {
		t.Fatal(saved.Code, saved.Body.String())
	}
	if e := json.Unmarshal(saved.Body.Bytes(), &current); e != nil || current.DefaultLanguage != "ja" || !current.AcoustIDKeyConfigured {
		t.Fatal(current, e)
	}
	body, _ = json.Marshal(metadata.MetadataRefreshRequest{ExpectedRevision: current.Revision})
	for _, method := range []string{"GET", "POST"} {
		if result := call(method, "/v1/admin/metadata/refresh", string(body), ""); result.Code != 401 {
			t.Fatal("unauthenticated", method, result.Code)
		}
	}
	result := call("POST", "/v1/admin/metadata/refresh", string(body), owner.AccessToken)
	if result.Code != 202 || !strings.Contains(result.Body.String(), `"queueing":true`) {
		t.Fatal(result.Code, result.Body.String())
	}
	status := call("GET", "/v1/admin/metadata/refresh", "", owner.AccessToken)
	if status.Code != 200 || !strings.Contains(status.Body.String(), `"queueing":true`) {
		t.Fatal(status.Code, status.Body.String())
	}
	bad := call("PUT", "/v1/admin/metadata/settings", `{"expectedRevision":1,"localArtworkLimitBytes":1,"artworkPreference":"folders|online"}`, owner.AccessToken)
	if bad.Code != 400 {
		t.Fatal("bad enum", bad.Code, bad.Body.String())
	}
}
