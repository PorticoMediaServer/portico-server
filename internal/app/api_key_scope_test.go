package app

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/app/apiroute"
)

func mustURLForTest(t *testing.T, path string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func apiKeyRequestForTest(t *testing.T, method, path, operation string) *http.Request {
	t.Helper()
	req := &http.Request{Method: method, URL: mustURLForTest(t, path)}
	route := apiroute.Route{Method: method, Path: path, OperationID: operation}
	if operation == "postSearch" || operation == "browseLibrary" || operation == "postSavedViewsSavedViewIdBrowse" {
		route.APIKeyScopes = []string{"read"}
	} else if operation == "postMediaIdOptimized" || operation == "deleteMediaIdOptimizedProfile" {
		route.APIKeyScopes = []string{"transcode"}
	}
	return apiroute.WithRoute(req, route)
}

func TestAPIKeyPlaybackScopeIncludesCollectionRootButNotNearPrefix(t *testing.T) {
	user := User{AuthProvider: "api_key", APIKeyID: "key", APIKeyScopes: []string{"playMedia"}}
	for _, path := range []string{"/api/playback-sessions", "/api/playback-sessions/session-1", "/api/playback", "/api/playback/active"} {
		req := apiKeyRequestForTest(t, http.MethodPost, path, "postPlaybackSessions")
		if !apiKeyAllowsRequest(user, req) {
			t.Fatalf("playMedia scope did not authorize canonical playback path %s", path)
		}
	}
	for _, path := range []string{"/api/playback-sessions-evil", "/api/playback-evil"} {
		req := apiKeyRequestForTest(t, http.MethodPost, path, "nearPrefix")
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

func TestAPIKeyReadScopeAllowsReadOnlyPOSTProjections(t *testing.T) {
	user := User{AuthProvider: "api_key", APIKeyID: "key", APIKeyScopes: []string{"read"}}
	for path, operation := range map[string]string{"/api/search": "postSearch", "/api/libraries/lib-1/browse": "browseLibrary", "/api/saved-views/view-1/browse": "postSavedViewsSavedViewIdBrowse"} {
		req := apiKeyRequestForTest(t, http.MethodPost, path, operation)
		if !apiKeyAllowsRequest(user, req) {
			t.Fatalf("read scope did not authorize read-only POST path %s", path)
		}
	}
	for _, path := range []string{"/api/search/history", "/api/libraries/lib-1/scan", "/api/libraries/lib-1/browse-evil"} {
		req := apiKeyRequestForTest(t, http.MethodPost, path, "notAReadProjection")
		if apiKeyAllowsRequest(user, req) {
			t.Fatalf("read scope authorized mutating or near-prefix POST path %s", path)
		}
	}
}

func TestAPIKeyTranscodeScopeAllowsOnlyExactOptimizedVersionOperations(t *testing.T) {
	transcode := User{AuthProvider: "api_key", APIKeyID: "key", APIKeyScopes: []string{"read", "transcode"}}
	for _, test := range []struct {
		method    string
		path      string
		operation string
	}{
		{http.MethodPost, "/api/media/id/optimized", "postMediaIdOptimized"},
		{http.MethodDelete, "/api/media/id/optimized/universal-1080p", "deleteMediaIdOptimizedProfile"},
	} {
		if !apiKeyAllowsRequest(transcode, apiKeyRequestForTest(t, test.method, test.path, test.operation)) {
			t.Fatalf("transcode scope did not authorize %s %s", test.method, test.path)
		}
	}
	readOnly := User{AuthProvider: "api_key", APIKeyID: "read", APIKeyScopes: []string{"read"}}
	if apiKeyAllowsRequest(readOnly, apiKeyRequestForTest(t, http.MethodPost, "/api/media/id/optimized", "postMediaIdOptimized")) {
		t.Fatal("read scope authorized optimized-version creation")
	}
	if apiKeyAllowsRequest(transcode, apiKeyRequestForTest(t, http.MethodPost, "/api/media/id/jobs", "postMediaIdJobs")) {
		t.Fatal("transcode scope authorized the multiplexed media jobs endpoint")
	}
}

func TestAPIKeyExactDVRScopeMatrix(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		method    string
		scopes    []string
		want      bool
	}{
		{"view recording", "getDvrRecordingsId", http.MethodGet, []string{"viewDVR"}, true},
		{"view denied to read", "getDvrRecordingsId", http.MethodGet, []string{"read"}, false},
		{"playback requires both", "postDvrRecordingsIdPlayback", http.MethodPost, []string{"viewDVR", "playMedia"}, true},
		{"playback missing play", "postDvrRecordingsIdPlayback", http.MethodPost, []string{"viewDVR"}, false},
		{"playback missing view", "postDvrRecordingsIdPlayback", http.MethodPost, []string{"playMedia"}, false},
		{"stream requires both", "getDvrRecordingsIdStream", http.MethodGet, []string{"viewDVR", "playMedia"}, true},
		{"delete recording", "deleteDvrRecordingsId", http.MethodDelete, []string{"deleteDVRRecordings"}, true},
		{"delete not schedule", "deleteDvrRecordingsId", http.MethodDelete, []string{"scheduleDVR"}, false},
		{"schedule recording", "postDvrRecordings", http.MethodPost, []string{"scheduleDVR"}, true},
		{"manage aliases schedule", "patchDvrRecordingsId", http.MethodPatch, []string{"manageDVR"}, true},
		{"unknown DVR operation", "postDvrUnknown", http.MethodPost, []string{"manageDVR", "playMedia"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user := User{AuthProvider: "api_key", APIKeyID: "key", APIKeyScopes: test.scopes}
			req := apiKeyRequestForTest(t, test.method, "/api/dvr/recordings/id", test.operation)
			if got := apiKeyAllowsRequest(user, req); got != test.want {
				t.Fatalf("allowed = %t, want %t", got, test.want)
			}
		})
	}
}

func TestAPIKeyExactDownloadAndMediaStateScopeMatrix(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		operation string
		method    string
		scopes    []string
		want      bool
	}{
		{"create preparation", "/api/download-preparations", "createDownloadPreparation", http.MethodPost, []string{"downloadMedia"}, true},
		{"grant preparation", "/api/download-preparations/id/grant", "createDownloadPreparationGrant", http.MethodPost, []string{"downloadMedia"}, true},
		{"grant denied to play", "/api/download-preparations/id/grant", "createDownloadPreparationGrant", http.MethodPost, []string{"playMedia"}, false},
		{"download options", "/api/media/id/download-options", "getMediaIdDownloadOptions", http.MethodGet, []string{"downloadMedia"}, true},
		{"optimized playback", "/api/media/id/optimized/source", "getMediaIdOptimizedProfile", http.MethodGet, []string{"playMedia"}, true},
		{"optimized playback denied to read", "/api/media/id/optimized/source", "getMediaIdOptimizedProfile", http.MethodGet, []string{"read"}, false},
		{"trickplay playback", "/api/media/id/trickplay/set/tiles/0.jpg", "getMediaIdTrickplaySetIdTilesTileIndexJpg", http.MethodGet, []string{"playMedia"}, true},
		{"metadata candidates", "/api/media/id/match-candidates", "getMediaIdMatchCandidates", http.MethodGet, []string{"editMetadata"}, true},
		{"metadata candidates denied to read", "/api/media/id/match-candidates", "getMediaIdMatchCandidates", http.MethodGet, []string{"read"}, false},
		{"watch state", "/api/media/id/watched", "postMediaIdWatched", http.MethodPost, []string{"playMedia"}, true},
		{"rating state", "/api/media/id/rating", "postMediaIdRating", http.MethodPost, []string{"playMedia"}, true},
		{"saved state not metadata", "/api/media/id/watchlist", "postMediaIdWatchlist", http.MethodPost, []string{"editMetadata"}, false},
		{"metadata edit", "/api/media/id/images", "postMediaIdImages", http.MethodPost, []string{"editMetadata"}, true},
		{"delete media exact", "/api/media/id", "deleteMediaId", http.MethodDelete, []string{"deleteMedia"}, true},
		{"delete media not metadata", "/api/media/id", "deleteMediaId", http.MethodDelete, []string{"editMetadata"}, false},
		{"client logs denied", "/api/client-logs", "postClientLogs", http.MethodPost, []string{"read"}, false},
		{"multiplexed jobs denied", "/api/media/id/jobs", "postMediaIdJobs", http.MethodPost, []string{"editMetadata", "transcode"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user := User{AuthProvider: "api_key", APIKeyID: "key", APIKeyScopes: test.scopes}
			req := apiKeyRequestForTest(t, test.method, test.path, test.operation)
			if got := apiKeyAllowsRequest(user, req); got != test.want {
				t.Fatalf("allowed = %t, want %t", got, test.want)
			}
		})
	}
}

func TestAPIKeyUnknownRouteFailsClosed(t *testing.T) {
	user := User{AuthProvider: "api_key", APIKeyID: "key", APIKeyScopes: []string{"read", "playMedia"}}
	request := &http.Request{Method: http.MethodGet, URL: mustURLForTest(t, "/api/future-route")}
	if apiKeyAllowsRequest(user, request) {
		t.Fatal("unregistered route was authorized")
	}
}
