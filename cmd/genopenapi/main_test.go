package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLiveDVRLibraryContractPublishesAudienceMethodsAndSchemas(t *testing.T) {
	root, err := findRoot()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "api", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	doc := normalizeYAML(raw).(map[string]any)
	normalizeContract(doc)
	contract, err := clientCoreContract(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Methods) == 0 || len(contract.Schemas) == 0 {
		t.Fatalf("missing generated maps: methods=%d schemas=%d", len(contract.Methods), len(contract.Schemas))
	}
	for key, expected := range map[string]struct{ audience, method, schema string }{
		"GET /live-tv":                {"viewer", "liveTv", "LiveTVSourceSummaryListResponse"},
		"GET /live-tv/sources":        {"management", "adminLiveTvSources", "LiveTVSourceListResponse"},
		"GET /dvr/status":             {"viewer", "dvrStatus", "DVRConsumerStatus"},
		"GET /admin/dvr/status":       {"management", "adminDvrOperationalStatus", "DVROperationalStatus"},
		"GET /library-channels":       {"viewer", "libraryChannels", "LibraryChannelListResponse"},
		"GET /admin/library-channels": {"management", "adminLibraryChannels", "AdminLibraryChannelListResponse"},
	} {
		entry := object(contract.Methods[key])
		if entry["audience"] != expected.audience || entry["clientCoreMethod"] != expected.method {
			t.Fatalf("%s mapping=%#v", key, entry)
		}
		operationID := asString(entry["operationId"])
		if contract.Schemas[operationID] != expected.schema {
			t.Fatalf("%s schema=%q want=%q", key, contract.Schemas[operationID], expected.schema)
		}
	}
	ts := string(contract.TypeScript)
	for _, forbidden := range []string{"liveTvSources", "dvrOperationalStatus\""} {
		if strings.Contains(ts, forbidden) {
			t.Fatalf("generated consumer contract retained misleading alias %q", forbidden)
		}
	}
	paths := object(doc["paths"])
	for _, operation := range []struct{ path, method string }{
		{"/live-tv/sources", "get"}, {"/live-tv/sources", "post"},
		{"/live-tv/sources/test-add", "post"}, {"/live-tv/sources/hdhomerun/discover", "post"},
		{"/live-tv/sources/{sourceId}", "patch"}, {"/live-tv/sources/{sourceId}", "delete"},
		{"/live-tv/sources/{sourceId}/refresh", "post"},
	} {
		op := object(object(paths[operation.path])[operation.method])
		if op["x-portico-audience"] != "management" || op["x-portico-permission"] != "manage-server" || op["x-portico-role"] != "owner" || op["x-portico-principal"] != "interactive-session" {
			t.Fatalf("%s %s management boundary=%#v", strings.ToUpper(operation.method), operation.path, op)
		}
	}
	mediaGrant := object(object(object(doc["components"])["securitySchemes"])["mediaGrantAuth"])
	if mediaGrant["in"] != "cookie" || mediaGrant["name"] != "portico_media_grant" {
		t.Fatalf("mediaGrantAuth retained URL transport: %#v", mediaGrant)
	}
	if strings.Contains(string(body), "name: media_grant, in: query") {
		t.Fatal("OpenAPI retained a media grant query parameter")
	}
}

func TestNormalizedOperationsDeclareCanonicalAudienceAndSurfaces(t *testing.T) {
	root, err := findRoot()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "api", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	doc := object(normalizeYAML(raw))
	normalizeContract(doc)
	paths := object(doc["paths"])
	allowed := map[string]map[string]bool{
		"viewer":     {"web": true, "mobile": true, "television": true},
		"management": {"web-admin": true},
	}
	operationCount := 0
	for path, rawItem := range paths {
		item := object(rawItem)
		for _, method := range methodNames {
			rawOperation, exists := item[method]
			if !exists {
				continue
			}
			operationCount++
			op := object(rawOperation)
			operationID := asString(op["operationId"])
			if operationID == "" {
				t.Errorf("%s %s is missing operationId", method, path)
			}
			audience := asString(op["x-portico-audience"])
			canonicalSurfaces, ok := allowed[audience]
			if !ok {
				t.Errorf("%s %s (%s) audience = %q", method, path, operationID, audience)
				continue
			}
			values, ok := op["x-portico-surfaces"].([]any)
			if !ok || len(values) == 0 {
				t.Errorf("%s %s (%s) is missing x-portico-surfaces", method, path, operationID)
				continue
			}
			seen := map[string]bool{}
			for _, value := range values {
				surface := asString(value)
				if !canonicalSurfaces[surface] {
					t.Errorf("%s %s (%s) surface %q is invalid for audience %q", method, path, operationID, surface, audience)
				}
				if seen[surface] {
					t.Errorf("%s %s (%s) repeats surface %q", method, path, operationID, surface)
				}
				seen[surface] = true
			}
		}
	}
	if operationCount == 0 {
		t.Fatal("normalized contract contains no operations")
	}
}

func TestAudienceAndSurfaceDefaultsRespectManagementBoundary(t *testing.T) {
	if got := audienceFor("manage-server", map[string]any{}); got != "management" {
		t.Fatalf("manage-server audience = %q", got)
	}
	if got := audienceFor("browse", map[string]any{}); got != "viewer" {
		t.Fatalf("browse audience = %q", got)
	}
	if got := surfacesFor("management", map[string]any{}); len(got) != 1 || asString(got[0]) != "web-admin" {
		t.Fatalf("management surfaces = %#v", got)
	}
}

func TestAllLocalOpenAPIReferencesResolve(t *testing.T) {
	root, err := findRoot()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "api", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	doc := object(normalizeYAML(raw))

	var walk func(any, string)
	walk = func(value any, path string) {
		switch typed := value.(type) {
		case map[string]any:
			if ref := asString(typed["$ref"]); strings.HasPrefix(ref, "#/") && !localOpenAPIReferenceResolves(doc, ref) {
				t.Errorf("%s contains unresolved local reference %q", path, ref)
			}
			for key, child := range typed {
				walk(child, path+"/"+key)
			}
		case []any:
			for _, child := range typed {
				walk(child, path+"[]")
			}
		}
	}
	walk(doc, "#")
}

func localOpenAPIReferenceResolves(doc map[string]any, ref string) bool {
	var current any = doc
	for _, escaped := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		key := strings.ReplaceAll(strings.ReplaceAll(escaped, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current, ok = object[key]
		if !ok {
			return false
		}
	}
	return true
}

func TestConstrainedClientLongPollOperationsAreTypedAndPolicyBound(t *testing.T) {
	root, err := findRoot()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "api", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	doc := object(normalizeYAML(raw))
	normalizeContract(doc)
	paths := object(doc["paths"])

	tests := []struct {
		path, operationID, permission, responseSchema, eventSchema string
	}{
		{"/events/poll", "pollApplicationEvents", "authenticated", "ApplicationEventLongPollEnvelope", "AppEvent"},
		{"/notifications/events/poll", "pollViewerNotificationInvalidations", "authenticated", "NotificationInvalidationLongPollEnvelope", "NotificationInvalidation"},
		{"/playback-sessions/{sessionId}/command/events/poll", "pollPlaybackSessionCommands", "play-media", "PlaybackCommandLongPollEnvelope", "PlaybackCommand"},
		{"/watch-with-friends/groups/{groupId}/events/poll", "pollWatchWithFriendsGroupEvents", "play-media", "WatchWithFriendsLongPollEnvelope", "WatchWithFriendsGroup"},
	}
	components := object(object(doc["components"])["schemas"])
	for _, test := range tests {
		op := object(object(paths[test.path])["get"])
		if asString(op["operationId"]) != test.operationID || asString(op["x-portico-auth"]) != "session" || asString(op["x-portico-permission"]) != test.permission || asString(op["x-portico-audience"]) != "viewer" || asString(op["x-portico-rate-policy"]) != "long-poll" {
			t.Errorf("GET %s policy metadata = %#v", test.path, op)
		}
		if got := successResponseSchema(op); got != test.responseSchema {
			t.Errorf("GET %s response schema = %q, want %q", test.path, got, test.responseSchema)
		}
		parameters, _ := op["parameters"].([]any)
		seen := map[string]map[string]any{}
		for _, value := range parameters {
			parameter := object(value)
			seen[asString(parameter["name"])] = object(parameter["schema"])
		}
		wait := seen["waitSeconds"]
		if intValue(wait["minimum"]) != 0 || intValue(wait["maximum"]) != 25 || intValue(wait["default"]) != 20 {
			t.Errorf("GET %s waitSeconds = %#v", test.path, wait)
		}
		if seen["cursor"] == nil {
			t.Errorf("GET %s is missing cursor", test.path)
		}
		envelope := object(components[test.responseSchema])
		properties := object(envelope["properties"])
		events := object(properties["events"])
		ref := asString(object(events["items"])["$ref"])
		if ref != "#/components/schemas/"+test.eventSchema {
			t.Errorf("%s event item = %q, want %s", test.responseSchema, ref, test.eventSchema)
		}
	}
}

func TestSharedPlaybackDVRAndEventCapabilitiesArePublishedInSource(t *testing.T) {
	root, err := findRoot()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "api", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	doc := object(normalizeYAML(raw))
	components := object(object(doc["components"])["schemas"])

	playback := object(components["PlaybackSessionCreateRequest"])
	if _, ok := object(playback["properties"])["versionId"]; !ok {
		t.Fatal("PlaybackSessionCreateRequest does not publish optional versionId")
	}
	dvr := object(components["DVRConsumerStatus"])
	if _, ok := object(dvr["properties"])["storage"]; !ok || !containsStringValue(dvr["required"], "storage") {
		t.Fatal("DVRConsumerStatus does not require consumer-safe storage totals")
	}
	product := object(components["ProductContract"])
	for _, field := range []string{"eventTransports", "longPoll"} {
		if _, ok := object(product["properties"])[field]; !ok || !containsStringValue(product["required"], field) {
			t.Errorf("ProductContract does not require %s", field)
		}
	}
	longPoll := object(components["LongPollCapabilities"])
	limits := object(longPoll["properties"])
	if intValue(object(limits["defaultWaitSeconds"])["const"]) != 20 || intValue(object(limits["maximumWaitSeconds"])["const"]) != 25 || intValue(object(limits["maximumConcurrentStreams"])["const"]) != 4 {
		t.Fatalf("long-poll capability limits = %#v", limits)
	}
}

func containsStringValue(raw any, want string) bool {
	values, _ := raw.([]any)
	for _, value := range values {
		if asString(value) == want {
			return true
		}
	}
	return false
}
