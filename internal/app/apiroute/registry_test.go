package apiroute

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryDispatchesExactMethodAndReturnsAllow(t *testing.T) {
	mux := http.NewServeMux()
	registry := New(mux, nil, nil)
	registry.Public("/api/readiness", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/readiness", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("GET status = %d, want %d", response.Code, http.StatusNoContent)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/readiness", nil)
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

func TestRegistryAttachesSelectedContractRouteToRequest(t *testing.T) {
	mux := http.NewServeMux()
	registry := New(mux, nil, nil)
	var observed Route
	var observedOK bool
	registry.Session("/api/libraries/", func(_ http.ResponseWriter, request *http.Request) {
		observed, observedOK = RouteFromRequest(request)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/libraries/lib_movies", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK && response.Code != http.StatusNoContent {
		// The test handler does not write a status; this guard only protects
		// against dispatch failures while keeping the assertion below focused.
		t.Fatalf("dispatch status = %d", response.Code)
	}
	if !observedOK {
		t.Fatal("registry did not attach a selected route")
	}
	if observed.Method != http.MethodGet || observed.Path != "/api/libraries/{id}" || observed.Permission == "" || observed.RatePolicy == "" || observed.AuditEvent == "" {
		t.Fatalf("selected route metadata = %#v", observed)
	}
}

func TestGeneratedOperationsHaveRequiredMetadata(t *testing.T) {
	mux := http.NewServeMux()
	registry := New(mux, nil, nil)
	registry.Session("/api/libraries/", func(http.ResponseWriter, *http.Request) {})
	routes := registry.Routes()
	if len(routes) == 0 {
		t.Fatal("expected generated library routes")
	}
	for _, route := range routes {
		if route.OperationID == "" || route.Method == "" || route.Path == "" {
			t.Fatalf("incomplete route metadata: %#v", route)
		}
		if route.SuccessStatus == 0 || route.ResponseSchema == "" {
			t.Fatalf("route missing response contract: %#v", route)
		}
		if route.Auth != AuthSession || route.Permission == "" || route.Audience == "" || len(route.Surfaces) == 0 || route.RatePolicy == "" || route.AuditEvent == "" {
			t.Fatalf("route missing enforced auth metadata: %#v", route)
		}
		if err := validateAudienceSurfaces(route.Audience, route.Surfaces); err != nil {
			t.Fatalf("route has invalid client audience metadata: %#v: %v", route, err)
		}
	}
}

func TestGeneratedContractPublishesAllConstrainedClientLongPollOperations(t *testing.T) {
	registry := New(http.NewServeMux(), nil, nil)
	want := map[string]struct {
		operationID, permission, responseSchema string
	}{
		"/events/poll":               {"pollApplicationEvents", "authenticated", "ApplicationEventLongPollEnvelope"},
		"/notifications/events/poll": {"pollViewerNotificationInvalidations", "authenticated", "NotificationInvalidationLongPollEnvelope"},
		"/playback-sessions/{sessionId}/command/events/poll": {"pollPlaybackSessionCommands", "play-media", "PlaybackCommandLongPollEnvelope"},
		"/playback/receivers/{receiverId}/events/poll":       {"pollPlaybackReceiverEvents", "play-media", "PlaybackReceiverLongPollEnvelope"},
		"/watch-with-friends/groups/{groupId}/events/poll":   {"pollWatchWithFriendsGroupEvents", "play-media", "WatchWithFriendsLongPollEnvelope"},
	}
	for path, expected := range want {
		item := registry.pathItems[path]
		raw, ok := item["get"]
		if !ok {
			t.Errorf("generated contract is missing GET %s", path)
			continue
		}
		route := RouteFromOperation(http.MethodGet, "/api"+path, raw, AuthSession)
		if route.OperationID != expected.operationID || route.Permission != expected.permission || route.Audience != "viewer" || route.RatePolicy != "long-poll" || route.ResponseSchema != expected.responseSchema {
			t.Errorf("GET %s route = %#v", path, route)
		}
	}
}

func TestLibraryDiscoverIsAnExactTypedOperation(t *testing.T) {
	mux := http.NewServeMux()
	registry := New(mux, nil, nil)
	called := false
	registry.Session("/api/libraries/", func(http.ResponseWriter, *http.Request) { called = true })
	request := httptest.NewRequest(http.MethodGet, "/api/libraries/lib_movies/discover?limit=12", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if !called {
		t.Fatalf("GET library discover was not dispatched; status=%d", response.Code)
	}
	var found bool
	for _, route := range registry.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/libraries/{id}/discover" {
			found = true
			if route.OperationID != "getLibraryDiscover" || route.ResponseSchema != "SuggestionsResponse" || !route.TypedAdapter {
				t.Fatalf("unexpected library discover contract: %#v", route)
			}
		}
	}
	if !found {
		t.Fatal("generated route catalog is missing GET /api/libraries/{id}/discover")
	}
}

func TestLiveTVHLSFamilyMountsNestedPlaylistsAndSegments(t *testing.T) {
	mux := http.NewServeMux()
	registry := New(mux, nil, nil)
	called := map[string]bool{}
	registry.Media("/api/live-tv/", func(_ http.ResponseWriter, request *http.Request) {
		called[request.URL.Path] = true
	})

	for _, path := range []string{
		"/api/live-tv/hls/channel-1/playlist.m3u8",
		"/api/live-tv/hls/channel-1/item?uri=encoded",
		"/api/live-tv/hls/channel-1/segment?name=segment_00001.ts",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if !called[request.URL.Path] {
			t.Fatalf("GET %s was not dispatched; status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestLibraryFamilyIsCompleteAndUsesNamedResponseContracts(t *testing.T) {
	mux := http.NewServeMux()
	registry := New(mux, nil, nil)
	called := false
	registry.Session("/api/libraries/", func(http.ResponseWriter, *http.Request) { called = true })

	want := map[string]struct {
		schema string
		status int
	}{
		http.MethodGet + " /api/libraries/{id}":                            {schema: "Library", status: http.StatusOK},
		http.MethodPatch + " /api/libraries/{id}":                          {schema: "Library", status: http.StatusOK},
		http.MethodDelete + " /api/libraries/{id}":                         {schema: "SuccessResponse", status: http.StatusOK},
		http.MethodGet + " /api/libraries/{libraryId}/browse-capabilities": {schema: "LibraryBrowseCapabilities", status: http.StatusOK},
		http.MethodPost + " /api/libraries/{libraryId}/browse":             {schema: "BrowseLibraryResponse", status: http.StatusOK},
		http.MethodGet + " /api/libraries/{id}/categories":                 {schema: "LibraryCategoryListResponse", status: http.StatusOK},
		http.MethodGet + " /api/libraries/{id}/sources":                    {schema: "LibrarySourceGroupListResponse", status: http.StatusOK},
		http.MethodPost + " /api/libraries/{id}/scan":                      {schema: "Job", status: http.StatusCreated},
		http.MethodPost + " /api/libraries/{id}/lyrics":                    {schema: "Job", status: http.StatusCreated},
		http.MethodPost + " /api/libraries/trash/empty":                    {schema: "LibraryTrashCleanupResponse", status: http.StatusAccepted},
	}

	for _, route := range registry.Routes() {
		if !strings.HasPrefix(route.Path, "/api/libraries/") {
			continue
		}
		if strings.Contains(route.Path, "...") {
			t.Fatalf("complete library family still exposes a catch-all adapter: %#v", route)
		}
		if !route.TypedAdapter {
			t.Fatalf("library operation is not tied to a named response contract: %#v", route)
		}
		key := route.Method + " " + route.Path
		if expected, ok := want[key]; ok {
			if route.ResponseSchema != expected.schema || route.SuccessStatus != expected.status {
				t.Fatalf("%s contract = %s/%d, want %s/%d", key, route.ResponseSchema, route.SuccessStatus, expected.schema, expected.status)
			}
			delete(want, key)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing named library contracts: %#v", want)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/libraries/lib_movies/not-a-resource", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if called {
		t.Fatal("unknown route was dispatched through a hidden library catch-all")
	}
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown library resource status = %d, want 404", response.Code)
	}
}

func TestTemplateMatcherSupportsSuffixAndCatchAll(t *testing.T) {
	for _, test := range []struct {
		template string
		path     string
		want     bool
	}{
		{"/api/media/{id}/trickplay/{setId}/tiles/{tileIndex}.jpg", "/api/media/m1/trickplay/s1/tiles/4.jpg", true},
		{"/api/media/{id}/trickplay/{setId}/tiles/{tileIndex}.jpg", "/api/media/m1/trickplay/s1/tiles/4.png", false},
		{"/debug/{tail...}", "/debug/accounts/a1/test", true},
	} {
		if got := matchPath(test.template, test.path); got != test.want {
			t.Errorf("matchPath(%q, %q) = %v, want %v", test.template, test.path, got, test.want)
		}
	}
}

func TestNoContentOperationUsesAnHonestResponseMarker(t *testing.T) {
	raw := []byte(`{
		"operationId": "deleteExample",
		"x-portico-auth": "session",
		"x-portico-permission": "authenticated",
		"x-portico-audience": "viewer",
		"x-portico-surfaces": ["web", "mobile", "television"],
		"x-portico-rate-policy": "state-mutation",
		"x-portico-audit-event": "api.deleteExample",
		"responses": {"204": {"description": "Deleted"}}
	}`)
	route := RouteFromOperation(http.MethodDelete, "/api/example", raw, AuthSession)
	if route.SuccessStatus != http.StatusNoContent || route.ResponseSchema != "NoContent" || !route.TypedAdapter {
		t.Fatalf("unexpected no-content contract: %#v", route)
	}
}

func TestRouteFromOperationRejectsIncompleteOrCrossAudienceSurfaces(t *testing.T) {
	for _, test := range []struct {
		name     string
		audience string
		surfaces string
	}{
		{name: "missing audience", audience: "", surfaces: `["web"]`},
		{name: "missing surfaces", audience: "viewer", surfaces: `[]`},
		{name: "viewer on admin surface", audience: "viewer", surfaces: `["web-admin"]`},
		{name: "management on viewer surface", audience: "management", surfaces: `["web"]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := []byte(`{
				"operationId": "getExample",
				"x-portico-auth": "session",
				"x-portico-permission": "authenticated",
				"x-portico-audience": "` + test.audience + `",
				"x-portico-surfaces": ` + test.surfaces + `,
				"x-portico-rate-policy": "interactive-read",
				"x-portico-audit-event": "none",
				"responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Example"}}}}}
			}`)
			defer func() {
				if recover() == nil {
					t.Fatal("invalid client audience metadata was accepted")
				}
			}()
			RouteFromOperation(http.MethodGet, "/api/example", raw, AuthSession)
		})
	}
}

func TestAPIRootAndUnknownPathsReturnProblemJSON(t *testing.T) {
	mux := http.NewServeMux()
	_ = New(mux, nil, nil)
	for _, path := range []string{"/api", "/api/not-a-resource"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", path, response.Code)
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "application/problem+json; charset=utf-8" {
			t.Errorf("GET %s content type = %q", path, contentType)
		}
	}
}
