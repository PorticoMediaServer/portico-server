package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppleAppSiteAssociation(t *testing.T) {
	path := "/.well-known/apple-app-site-association"
	for _, team := range []string{"", "placeholder", "ABC1234567"} {
		w := httptest.NewRecorder()
		webHandler(t.TempDir(), team).ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if team == "ABC1234567" {
			if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
				t.Fatal(w.Code, w.Header())
			}
			var got map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			details := got["applinks"].(map[string]any)["details"].([]any)
			entry := details[0].(map[string]any)
			if entry["appIDs"].([]any)[0] != "ABC1234567.tv.getportico.portico.ios" || len(entry["components"].([]any)) != 5 {
				t.Fatal(got)
			}
		} else if w.Code != 404 {
			t.Fatal(team, w.Code)
		}
	}
}
