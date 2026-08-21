package app

import (
	"net/http"
	"net/url"
	"testing"
)

func mustURLForTest(t *testing.T, path string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestAPIKeyPlaybackScopeIncludesCollectionRootButNotNearPrefix(t *testing.T) {
	user := User{AuthProvider: "api_key", APIKeyID: "key", APIKeyScopes: []string{"playMedia"}}
	for _, path := range []string{"/api/playback-sessions", "/api/playback-sessions/session-1", "/api/playback", "/api/playback/active"} {
		req := &http.Request{Method: http.MethodPost, URL: mustURLForTest(t, path)}
		if !apiKeyAllowsRequest(user, req) {
			t.Fatalf("playMedia scope did not authorize canonical playback path %s", path)
		}
	}
	for _, path := range []string{"/api/playback-sessions-evil", "/api/playback-evil"} {
		req := &http.Request{Method: http.MethodPost, URL: mustURLForTest(t, path)}
		if apiKeyAllowsRequest(user, req) {
			t.Fatalf("near-prefix path %s was authorized", path)
		}
	}
}

func TestAPIKeyOwnerOnlyPathUsesSegmentBoundaries(t *testing.T) {
	if apiKeyOwnerOnlyPath("/api/system/storage/paths") != true {
		t.Fatal("canonical owner path was not classified")
	}
	if apiKeyOwnerOnlyPath("/api/systematic") {
		t.Fatal("near-prefix owner path was classified")
	}
}
