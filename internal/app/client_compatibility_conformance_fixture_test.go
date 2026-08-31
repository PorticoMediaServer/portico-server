package app

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"os"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestClientCompatibilityFixtureTracksServerContracts(t *testing.T) {
	bytes, err := os.ReadFile("../../api/openapi/fixtures/client-compatibility-conformance.json")
	if err != nil {
		t.Fatalf("read compatibility fixture: %v", err)
	}
	var fixture struct {
		System struct {
			APIVersion    string                `json:"apiVersion"`
			Compatibility CompatibilityEnvelope `json:"compatibility"`
		} `json:"system"`
		ProductContract struct {
			APIVersion         string                    `json:"apiVersion"`
			ServerCapabilities []string                  `json:"serverCapabilities"`
			SemanticIdentity   *SemanticDocumentIdentity `json:"semanticIdentity"`
		} `json:"productContract"`
		Negotiation struct {
			UnknownCapabilitiesAreAllowed bool `json:"unknownCapabilitiesAreAllowed"`
			NewerRevisionsRequireUpdate   bool `json:"newerRevisionsRequireClientUpdate"`
		} `json:"negotiation"`
	}
	if err := json.Unmarshal(bytes, &fixture); err != nil {
		t.Fatalf("decode compatibility fixture: %v", err)
	}
	if fixture.System.APIVersion != systemAPIVersion {
		t.Fatalf("fixture API revision=%q server=%q", fixture.System.APIVersion, systemAPIVersion)
	}
	if fixture.ProductContract.APIVersion != productContractRevision {
		t.Fatalf("fixture Product Contract API version=%q server=%q", fixture.ProductContract.APIVersion, productContractRevision)
	}
	if !fixture.Negotiation.UnknownCapabilitiesAreAllowed || !fixture.Negotiation.NewerRevisionsRequireUpdate {
		t.Fatalf("fixture omitted compatibility negotiation policy: %#v", fixture.Negotiation)
	}

	capabilities := map[string]bool{}
	for _, capability := range canonicalProductContract().ServerCapabilities {
		capabilities[capability] = true
	}
	for _, required := range fixture.ProductContract.ServerCapabilities {
		if !capabilities[required] {
			t.Fatalf("required capability %q is not advertised by the Product Contract", required)
		}
	}

	serverURL := newAuthTestServer(t)
	var status struct {
		APIVersion    string                `json:"apiVersion"`
		Compatibility CompatibilityEnvelope `json:"compatibility"`
	}
	code, body := doJSON(t, http.DefaultClient, http.MethodGet, serverURL+"/api/system", nil, &status)
	if code != http.StatusOK || status.APIVersion != fixture.System.APIVersion {
		t.Fatalf("system compatibility response status=%d body=%s response=%#v", code, body, status)
	}
	if !reflect.DeepEqual(status.Compatibility, fixture.System.Compatibility) {
		t.Fatalf("system compatibility drifted from fixture: got=%#v want=%#v", status.Compatibility, fixture.System.Compatibility)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var contract CanonicalProductContract
	code, body = doJSON(t, client, http.MethodGet, serverURL+"/api/product-contract", nil, &contract)
	if code != http.StatusOK || contract.APIVersion != fixture.ProductContract.APIVersion {
		t.Fatalf("Product Contract compatibility response status=%d body=%s response=%#v", code, body, contract)
	}
	if !reflect.DeepEqual(contract.SemanticIdentity, fixture.ProductContract.SemanticIdentity) || !reflect.DeepEqual(contract.ServerCapabilities, fixture.ProductContract.ServerCapabilities) {
		t.Fatalf("Product Contract compatibility drifted from fixture")
	}
}

func TestServerBuildIdentityUsesCanonicalNullableTimestamp(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatalf("read Server OpenAPI source: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode Server OpenAPI source: %v", err)
	}
	components, ok := document["components"].(map[string]any)
	if !ok {
		t.Fatalf("Server OpenAPI components=%#v", document["components"])
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatalf("Server OpenAPI schemas=%#v", components["schemas"])
	}
	build, ok := schemas["BuildIdentity"].(map[string]any)
	if !ok {
		t.Fatal("Server OpenAPI source omits BuildIdentity")
	}
	required := map[string]bool{}
	for _, field := range build["required"].([]any) {
		required[field.(string)] = true
	}
	if !required["timestamp"] {
		t.Fatalf("Server BuildIdentity does not require timestamp: %#v", build["required"])
	}
	properties, ok := build["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Server BuildIdentity properties=%#v", build["properties"])
	}
	timestamp, ok := properties["timestamp"].(map[string]any)
	if !ok {
		t.Fatalf("Server BuildIdentity timestamp=%#v", properties["timestamp"])
	}
	types, ok := timestamp["type"].([]any)
	if !ok || len(types) != 2 || types[0] != "string" || types[1] != "null" || timestamp["format"] != "date-time" {
		t.Fatalf("Server BuildIdentity timestamp schema=%#v, want nullable date-time", timestamp)
	}

	_, _, server := newAuthTestServerWithInstance(t)
	marshalBuild := func() map[string]any {
		t.Helper()
		encoded, err := json.Marshal(server.compatibilityEnvelope())
		if err != nil {
			t.Fatalf("marshal Server compatibility envelope: %v", err)
		}
		var envelope map[string]any
		if err := json.Unmarshal(encoded, &envelope); err != nil {
			t.Fatalf("decode Server compatibility envelope: %v", err)
		}
		build, ok := envelope["build"].(map[string]any)
		if !ok {
			t.Fatalf("Server compatibility envelope build=%#v", envelope["build"])
		}
		return build
	}
	wireBuild := marshalBuild()
	timestampValue, present := wireBuild["timestamp"]
	if !present || timestampValue != nil {
		t.Fatalf("development Server BuildIdentity timestamp=%#v, want explicit null", timestampValue)
	}

	server.cfg.BuildTimestamp = " 2026-08-17T12:34:56Z "
	wireBuild = marshalBuild()
	if got := wireBuild["timestamp"]; got != "2026-08-17T12:34:56Z" {
		t.Fatalf("release Server BuildIdentity timestamp=%#v, want canonical RFC3339 string", got)
	}
}

func TestConfigurationAwareCapabilityAvailabilityIsFailClosed(t *testing.T) {
	_, _, server := newAuthTestServerWithInstance(t)
	if got := castCapabilityState(server); got == "available" {
		t.Fatal("unconfigured Cast capability was available")
	}
	server.cfg.PublicOrigin = "https://media.example.test"
	server.cfg.CastReceiverOrigins = []string{"https://cast.example.test"}
	if got := castCapabilityState(server); got != "available" {
		t.Fatalf("fully configured Cast state=%q", got)
	}
	server.cfg.PublicOrigin = "http://media.example.test"
	if got := castCapabilityState(server); got == "available" {
		t.Fatal("plaintext Cast public origin was available")
	}

	settings, err := server.remoteAccessSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.ClaimStatus = "claimed"
	settings.ServerID = "srv_capability"
	settings.AssignedHostname = "ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv"
	settings.CertificateStatus = "valid"
	settings.LastPublicIPAddress = "203.0.113.10"
	settings.LastReachabilityResult = "public_reachable"
	if err := server.saveRemoteAccessSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := server.deleteSetting(remoteAccessCredentialKey); err != nil {
		t.Fatal(err)
	}
	if credential := server.secretSetting(remoteAccessCredentialKey); credential != "" {
		t.Fatalf("remote access credential remained after test reset: %q", credential)
	}
	if got := remoteAccessCapabilityState(server); got != "requires_configuration" {
		t.Fatalf("remote access without credential state=%q", got)
	}
	if err := server.saveSecretSetting(remoteAccessCredentialKey, "credential"); err != nil {
		t.Fatal(err)
	}
	if got := remoteAccessCapabilityState(server); got != "available" {
		t.Fatalf("fully configured remote access state=%q", got)
	}
	for _, diagnostic := range []string{"public_checking", "public_unreachable", "heartbeat_failed", "repair_network_changed"} {
		settings.LastReachabilityResult = diagnostic
		if err := server.saveRemoteAccessSettings(settings); err != nil {
			t.Fatal(err)
		}
		if got := remoteAccessCapabilityState(server); got != "available" {
			t.Fatalf("configured remote access with operational diagnostic %q state=%q, want available", diagnostic, got)
		}
	}
	settings.CertificateStatus = "pending"
	if err := server.saveRemoteAccessSettings(settings); err != nil {
		t.Fatal(err)
	}
	if got := remoteAccessCapabilityState(server); got != "degraded" {
		t.Fatalf("unready certificate remote access state=%q", got)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id,name,type,last_error,created_at,updated_at) VALUES ('source_capability','Source','m3u','',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if got := liveTVCapabilityState(server); got != "degraded" {
		t.Fatalf("source without channels state=%q", got)
	}
	if _, err := server.db.Exec(`INSERT INTO live_tv_channels (id,source_id,name,stream_url,last_seen_at,created_at,updated_at) VALUES ('channel_capability','source_capability','Channel','https://example.test/live',?,?,?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if got := liveTVCapabilityState(server); got != "available" {
		t.Fatalf("healthy source/channel state=%q", got)
	}
	if _, err := server.db.Exec(`UPDATE live_tv_channels SET enabled = 0 WHERE id = 'channel_capability'`); err != nil {
		t.Fatal(err)
	}
	if got := liveTVCapabilityState(server); got != "degraded" {
		t.Fatalf("disabled-only channel state=%q", got)
	}
}

func TestCapabilityRegistryIsAnchoredToMountedRoutesAndSharedByProjections(t *testing.T) {
	_, _, server := newAuthTestServerWithInstance(t)
	routes := map[string]bool{}
	for _, route := range server.APIRoutes() {
		routes[capabilityRouteKey(route.Method, route.Path)] = true
	}
	for _, definition := range serverCapabilityDefinitions {
		if len(definition.RequiredRoutes) == 0 {
			t.Errorf("capability %q has no required routes", definition.ID)
		}
		for _, required := range capabilityRequiredRoutes(definition) {
			if !routes[capabilityRouteKey(required.Method, required.Path)] {
				t.Errorf("capability %q is not anchored to a mounted route: %s %s", definition.ID, required.Method, required.Path)
			}
		}
	}
	definitions := map[string]serverCapabilityDefinition{}
	for _, definition := range serverCapabilityDefinitions {
		definitions[definition.ID] = definition
	}
	expectedRoutes := expectedBroadCapabilityRoutes()
	if len(expectedRoutes) != len(definitions) {
		t.Fatalf("independent route map covers %d capabilities; registry has %d", len(expectedRoutes), len(definitions))
	}
	for capabilityID := range definitions {
		if _, ok := expectedRoutes[capabilityID]; !ok {
			t.Fatalf("independent route map omits capability %q", capabilityID)
		}
	}
	for capabilityID, expected := range expectedRoutes {
		definition, ok := definitions[capabilityID]
		if !ok {
			t.Fatalf("independent route map references unknown capability %q", capabilityID)
		}
		actual := map[string]bool{}
		for _, route := range capabilityRequiredRoutes(definition) {
			actual[capabilityRouteKey(route.Method, route.Path)] = true
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("capability %q route surface drifted: got=%#v want=%#v", capabilityID, actual, expected)
		}
		for routeKey := range expected {
			withoutRoute := make(map[string]bool, len(routes))
			for key, mounted := range routes {
				withoutRoute[key] = mounted
			}
			delete(withoutRoute, routeKey)
			if state := serverCapabilityState(server, definition, withoutRoute); state != "unavailable" {
				t.Errorf("capability %q stayed %q after required route %s disappeared", capabilityID, state, routeKey)
			}
		}
	}
	seenIDs := map[string]bool{}
	for _, definition := range serverCapabilityDefinitions {
		if seenIDs[definition.ID] {
			t.Errorf("duplicate/orphan capability definition %q", definition.ID)
		}
		seenIDs[definition.ID] = true
	}
	for _, capability := range server.serverCapabilities() {
		if !seenIDs[capability.ID] {
			t.Errorf("capability projection %q has no registry definition", capability.ID)
		}
		delete(seenIDs, capability.ID)
	}
	if len(seenIDs) != 0 {
		t.Fatalf("registry definitions omitted from projection: %#v", seenIDs)
	}

	for _, definition := range serverCapabilityDefinitions {
		if len(capabilityRequiredRoutes(definition)) < 2 {
			continue
		}
		missingSecondary := make(map[string]bool, len(routes))
		for key, mounted := range routes {
			missingSecondary[key] = mounted
		}
		secondary := capabilityRequiredRoutes(definition)[1]
		delete(missingSecondary, capabilityRouteKey(secondary.Method, secondary.Path))
		if state := serverCapabilityState(server, definition, missingSecondary); state != "unavailable" {
			t.Errorf("capability %q stayed %q after required secondary route %s %s disappeared", definition.ID, state, secondary.Method, secondary.Path)
		}
	}

	envelope := server.compatibilityEnvelope()
	if len(envelope.Capabilities) != len(serverCapabilityDefinitions) {
		t.Fatalf("capability snapshot=%d registry=%d", len(envelope.Capabilities), len(serverCapabilityDefinitions))
	}
	contract := canonicalProductContract()
	if !reflect.DeepEqual(contract.ServerCapabilities, serverCapabilityCatalogIDs()) {
		t.Fatalf("Product Contract does not expose the stable capability catalog")
	}
	if contract.SemanticIdentity == nil || envelope.SemanticDocuments["productContract"] != *contract.SemanticIdentity {
		t.Fatalf("System does not reference the stable Product Contract semantics")
	}
	states := map[string]string{}
	for _, capability := range envelope.Capabilities {
		states[capability.ID] = capability.State
	}
	for _, id := range []string{"live-tv", "playback.google-cast-custom-receiver", "remote-access.direct"} {
		if states[id] == "available" {
			t.Errorf("unconfigured capability %q was advertised available", id)
		}
	}
}

func expectedBroadCapabilityRoutes() map[string]map[string]bool {
	routes := func(values ...string) map[string]bool {
		result := make(map[string]bool, len(values))
		for _, value := range values {
			result[value] = true
		}
		return result
	}
	return map[string]map[string]bool{
		"downloads": routes(
			"GET /api/download-preparations", "POST /api/download-preparations", "GET /api/download-preparations/{}", "PATCH /api/download-preparations/{}", "DELETE /api/download-preparations/{}", "POST /api/download-preparations/{}/grant",
			"POST /api/offline-download-authorizations/revalidate",
			"GET /api/media/{}/download-options", "GET /api/media/{}/download", "HEAD /api/media/{}/download",
		),
		"home.lazy-rows":                routes("GET /api/home", "GET /api/home/rows/{}"),
		"library.canonical-browse":      routes("POST /api/libraries/{}/browse"),
		"library.capability-resolution": routes("GET /api/libraries/{}/browse-capabilities"),
		"live-tv":                       routes("GET /api/live-tv", "POST /api/live-tv/play", "GET /api/live-tv/hls/{}/playlist.m3u8"),
		"notifications":                 routes("GET /api/notifications", "GET /api/notifications/events", "GET /api/notifications/events/poll", "POST /api/notifications/read-all", "POST /api/notifications/receipts", "PATCH /api/notifications/{}"),
		"watch-with-friends": routes(
			"GET /api/watch-with-friends/groups", "POST /api/watch-with-friends/groups", "GET /api/watch-with-friends/groups/{}", "DELETE /api/watch-with-friends/groups/{}", "POST /api/watch-with-friends/groups/{}/join", "POST /api/watch-with-friends/groups/{}/leave",
			"GET /api/watch-with-friends/groups/{}/events", "GET /api/watch-with-friends/groups/{}/events/poll", "PATCH /api/watch-with-friends/groups/{}/state", "PATCH /api/watch-with-friends/groups/{}/member/state", "PATCH /api/watch-with-friends/groups/{}/settings",
			"POST /api/watch-with-friends/groups/{}/queue", "PATCH /api/watch-with-friends/groups/{}/queue", "DELETE /api/watch-with-friends/groups/{}/queue/{}",
		),
		"saved.favorites-resource": routes("GET /api/favorites", "POST /api/media/{}/favorite"),
		"saved.first-class-resources": routes(
			"GET /api/playlists", "POST /api/playlists", "GET /api/playlists/{}", "PATCH /api/playlists/{}", "DELETE /api/playlists/{}", "GET /api/playlists/{}/items", "POST /api/playlists/{}/items:batch",
			"GET /api/collections", "POST /api/collections", "GET /api/collections/{}", "PATCH /api/collections/{}", "DELETE /api/collections/{}", "GET /api/collections/{}/items", "POST /api/collections/{}/memberships:batch",
			"GET /api/saved-views", "POST /api/saved-views", "GET /api/saved-views/{}", "PATCH /api/saved-views/{}", "DELETE /api/saved-views/{}", "POST /api/saved-views/{}/browse",
		),
		"remote-access.direct": routes(
			"GET /api/remote-access/status", "GET /api/remote-access/health", "GET /api/remote-access/routes/local", "POST /api/remote-access/claim/start", "POST /api/remote-access/claim/cancel", "PATCH /api/remote-access/settings", "POST /api/remote-access/policy-sync",
			"POST /api/remote-access/certificates/renew", "PATCH /api/remote-access/members/{}", "POST /api/remote-access/test-direct", "POST /api/remote-access/unclaim",
		),
		"playback.google-cast-custom-receiver": routes("POST /api/playback/cast/bootstrap", "POST /api/playback/cast/reconnect", "POST /api/playback/cast/redeem", "GET /api/playback/cast/sessions/{}/state", "POST /api/playback/cast/sessions/{}/{}"),
		"media.actions": routes(
			"GET /api/product-contract", "POST /api/playback-sessions", "POST /api/download-preparations", "POST /api/media/{}/watchlist", "POST /api/media/{}/favorite", "POST /api/media/{}/watched", "POST /api/media/{}/reaction", "POST /api/media/{}/rating", "POST /api/media/{}/jobs", "DELETE /api/media/{}",
			"POST /api/dvr/recordings", "POST /api/dvr/rules", "POST /api/dvr/recordings/{}/playback", "DELETE /api/dvr/recordings/{}", "PATCH /api/dvr/rules/{}",
		),
		"search.grouped-cursors":    routes("POST /api/search"),
		"settings.typed-revisioned": routes("GET /api/settings", "PATCH /api/settings"),
	}
}
