// Command genopenapi normalizes the transitional schema catalog into the
// deterministic runtime contract consumed by apiroute. Route paths, methods,
// operation IDs, response schemas, and standard errors are made mandatory.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var methodNames = []string{"get", "head", "post", "put", "patch", "delete", "options"}
var nonIdentifier = regexp.MustCompile(`[^A-Za-z0-9]+`)
var checkOnly = flag.Bool("check", false, "fail when generated artifacts are stale")

func main() {
	flag.Parse()
	root, err := findRoot()
	check(err)
	source := filepath.Join(root, "api", "openapi", "openapi.yaml")
	body, err := os.ReadFile(source)
	check(err)
	var yamlDoc any
	check(yaml.Unmarshal(body, &yamlDoc))
	doc, ok := normalizeYAML(yamlDoc).(map[string]any)
	if !ok {
		check(errors.New("OpenAPI document is not an object"))
	}
	normalizeContract(doc)
	clientCore, err := clientCoreContract(doc)
	check(err)
	doc["x-portico-client-core-method-map"] = clientCore.Methods
	doc["x-portico-client-core-schema-map"] = clientCore.Schemas
	encoded, err := json.MarshalIndent(doc, "", "  ")
	check(err)
	encoded = append(encoded, '\n')
	check(writeIfChanged(filepath.Join(root, "internal", "app", "apiroute", "contract.json"), encoded))
	check(writeIfChanged(filepath.Join(root, "api", "openapi", "portico-server.openapi.json"), encoded))
	check(writeIfChanged(filepath.Join(root, "packages", "portico-client-core", "src", "operationContract.generated.ts"), clientCore.TypeScript))
}

type clientCoreContractOutput struct {
	Methods    map[string]any
	Schemas    map[string]string
	TypeScript []byte
}

func normalizeContract(doc map[string]any) {
	doc["openapi"] = "3.1.0"
	doc["x-portico-generated"] = true
	doc["x-portico-source"] = "explicit-openapi-route-contract"
	paths := object(doc["paths"])
	components := object(doc["components"])
	schemas := object(components["schemas"])
	components["schemas"] = schemas
	doc["components"] = components

	operationIDs := map[string]string{}
	pathNames := make([]string, 0, len(paths))
	for path := range paths {
		pathNames = append(pathNames, path)
	}
	sort.Strings(pathNames)
	for _, path := range pathNames {
		item := cloneObject(object(resolvePathItem(paths, path, map[string]bool{})))
		for _, method := range methodNames {
			raw, exists := item[method]
			if !exists {
				continue
			}
			op := object(raw)
			operationID := strings.TrimSpace(asString(op["operationId"]))
			if operationID == "" || operationIDs[operationID] != "" {
				operationID = deterministicOperationID(method, path)
			}
			for operationIDs[operationID] != "" {
				operationID += "Operation"
			}
			operationIDs[operationID] = method + " " + path
			op["operationId"] = operationID
			if isLiveDVRLibraryPath(path) {
				audience := strings.TrimSpace(asString(op["x-portico-audience"]))
				if audience != "viewer" && audience != "management" {
					panic(fmt.Sprintf("OpenAPI operation %s %s requires x-portico-audience viewer or management", strings.ToUpper(method), path))
				}
				clientMethod := clientCoreMethodFor(method, path, operationID, op)
				if clientMethod == "" {
					delete(op, "x-portico-client-core-method")
				} else {
					op["x-portico-client-core-method"] = clientMethod
				}
			}
			op["x-portico-auth"] = authFor(op, doc)
			permission := permissionFor(method, path, op, doc)
			op["x-portico-permission"] = permission
			audience := audienceFor(permission, op)
			op["x-portico-audience"] = audience
			op["x-portico-surfaces"] = surfacesFor(audience, op)
			op["x-portico-rate-policy"] = ratePolicyFor(method, path, op)
			op["x-portico-audit-event"] = auditEventFor(method, path, op)
			op["x-portico-runtime-registered"] = true
			responses := object(op["responses"])
			check(ensureSuccessSchema(responses, schemas, operationID, strings.ToUpper(method)+" /api"+path))
			ensureProblemResponses(responses)
			op["responses"] = responses
			if audience := strings.TrimSpace(asString(op["x-portico-audience"])); audience == "viewer" || audience == "management" {
				if schemaName := successResponseSchema(op); schemaName != "" {
					schema := object(schemas[schemaName])
					if audience == "viewer" {
						closeViewerSchemaObjects(schema)
					}
					// A response shared by viewer and management operations is part of
					// the viewer-safe contract. Management-only schemas never override
					// that narrower declaration.
					if audience == "viewer" || strings.TrimSpace(asString(schema["x-portico-audience"])) == "" {
						schema["x-portico-audience"] = audience
						schemas[schemaName] = schema
					}
				}
			}
			item[method] = op
		}
		// Resolve path-item aliases in the generated artifact. This prevents
		// two resources from accidentally sharing operation IDs.
		paths[path] = item
	}
	doc["paths"] = paths
	check(validateAudienceContract(doc, paths, schemas))
}

// closeViewerSchemaObjects makes the consumer contract structurally explicit.
// A missing additionalProperties declaration is permissive in OpenAPI and can
// accidentally publish server-only fields to every present and future client.
// Explicit map-like objects remain open by declaring additionalProperties.
func closeViewerSchemaObjects(schema any) {
	switch value := schema.(type) {
	case map[string]any:
		if asString(value["type"]) == "object" {
			if _, declared := value["additionalProperties"]; !declared {
				value["additionalProperties"] = false
			}
		}
		for _, child := range value {
			closeViewerSchemaObjects(child)
		}
	case []any:
		for _, child := range value {
			closeViewerSchemaObjects(child)
		}
	}
}

func validateAudienceContract(doc, paths, schemas map[string]any) error {
	contract := object(doc["x-portico-audience-contract"])
	if len(contract) == 0 || intValue(contract["version"]) != 1 {
		return errors.New("x-portico-audience-contract version 1 is required")
	}
	if strings.TrimSpace(asString(contract["scope"])) != "live-dvr-operational-boundary" {
		return errors.New("x-portico-audience-contract must explicitly declare the live-dvr-operational-boundary scope")
	}
	for _, key := range []string{"audienceRequiredRoutePrefixes", "managementRoutePrefixes"} {
		items, ok := contract[key].([]any)
		if !ok || len(items) == 0 {
			return fmt.Errorf("x-portico-audience-contract.%s must be a non-empty array", key)
		}
		for _, item := range items {
			if !strings.HasPrefix(strings.TrimSpace(asString(item)), "/") {
				return fmt.Errorf("x-portico-audience-contract.%s contains an invalid route prefix", key)
			}
		}
	}
	methods := object(contract["clientCoreMethods"])
	if len(methods) == 0 {
		return errors.New("x-portico-audience-contract.clientCoreMethods must not be empty")
	}
	audiences := object(contract["schemaAudiences"])
	schemaAudience := map[string]string{}
	viewerSchemaCount := 0
	if len(audiences) != 2 {
		return errors.New("x-portico-audience-contract.schemaAudiences accepts only viewer and management")
	}
	for _, audience := range []string{"viewer", "management"} {
		raw, exists := audiences[audience]
		items, ok := raw.([]any)
		if !exists || !ok {
			return fmt.Errorf("x-portico-audience-contract.schemaAudiences.%s must be an array", audience)
		}
		for _, item := range items {
			name := strings.TrimSpace(asString(item))
			if name == "" || len(object(schemas[name])) == 0 {
				return fmt.Errorf("audience schema %q does not exist", name)
			}
			if prior := schemaAudience[name]; prior != "" {
				return fmt.Errorf("audience schema %s is assigned to both %s and %s", name, prior, audience)
			}
			schemaAudience[name] = audience
			if audience == "viewer" {
				viewerSchemaCount++
			}
			declared := strings.TrimSpace(asString(object(schemas[name])["x-portico-audience"]))
			if declared != audience {
				return fmt.Errorf("schema %s audience %s disagrees with map audience %s", name, declared, audience)
			}
		}
	}
	if viewerSchemaCount == 0 {
		return errors.New("x-portico-audience-contract must publish at least one viewer schema")
	}

	operations := map[string]map[string]any{}
	for _, rawItem := range paths {
		item := object(rawItem)
		for _, method := range methodNames {
			op := object(item[method])
			if id := strings.TrimSpace(asString(op["operationId"])); id != "" {
				operations[id] = op
				if clientMethod := strings.TrimSpace(asString(op["x-portico-client-core-method"])); clientMethod != "" && strings.TrimSpace(asString(methods[clientMethod])) != id {
					return fmt.Errorf("operation %s publishes client method %s outside the canonical audience map", id, clientMethod)
				}
			}
		}
	}
	for clientMethod, rawOperationID := range methods {
		operationID := strings.TrimSpace(asString(rawOperationID))
		op := operations[operationID]
		if op == nil {
			return fmt.Errorf("audience method %s references missing operation %s", clientMethod, operationID)
		}
		if strings.TrimSpace(asString(op["x-portico-client-core-method"])) != clientMethod {
			return fmt.Errorf("operation %s does not publish client method %s", operationID, clientMethod)
		}
		audience := strings.TrimSpace(asString(op["x-portico-audience"]))
		if audience != "viewer" && audience != "management" {
			return fmt.Errorf("operation %s must publish x-portico-audience viewer or management", operationID)
		}
		responseSchema := successResponseSchema(op)
		if responseSchema == "" || schemaAudience[responseSchema] == "" {
			return fmt.Errorf("operation %s response schema must be present in the audience map", operationID)
		}
		if audience == "viewer" && schemaAudience[responseSchema] != "viewer" {
			return fmt.Errorf("viewer operation %s resolves to %s schema %s", operationID, schemaAudience[responseSchema], responseSchema)
		}
	}
	return nil
}

func successResponseSchema(op map[string]any) string {
	responses := object(op["responses"])
	for _, code := range []string{"200", "201", "202", "206"} {
		response := object(responses[code])
		content := object(response["content"])
		for _, rawMedia := range content {
			ref := strings.TrimSpace(asString(object(object(rawMedia)["schema"])["$ref"]))
			if strings.HasPrefix(ref, "#/components/schemas/") {
				return strings.TrimPrefix(ref, "#/components/schemas/")
			}
		}
	}
	return ""
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(typed)
		return parsed
	default:
		return 0
	}
}

func clientCoreMethodFor(method, path, operationID string, op map[string]any) string {
	if value := strings.TrimSpace(asString(op["x-portico-client-core-method"])); value != "" {
		return value
	}
	if path == "/playback/cast/sessions/{sessionId}/{operation}" {
		return ""
	}
	key := strings.ToUpper(method) + " " + path
	overrides := map[string]string{
		"POST /playback/cast/bootstrap":                   "createCastBootstrap",
		"POST /playback/cast/redeem":                      "redeemCastBootstrap",
		"POST /playback/cast/reconnect":                   "reconnectCast",
		"POST /playback/cast/transfer-status":             "castTransferStatus",
		"POST /playback-sessions/{sessionId}/renegotiate": "renegotiatePlayback",
		"GET /live-tv":                                    "liveTv", "GET /live-tv/sources": "adminLiveTvSources", "POST /live-tv/sources": "createLiveTvSource",
		"POST /live-tv/sources/test-add": "testAddLiveTvSource", "POST /live-tv/sources/hdhomerun/discover": "discoverHDHomeRunSources",
		"GET /live-tv/sources/{sourceId}": "adminLiveTvSource", "PATCH /live-tv/sources/{sourceId}": "updateLiveTvSource", "DELETE /live-tv/sources/{sourceId}": "deleteLiveTvSource",
		"POST /live-tv/sources/{sourceId}/refresh": "refreshLiveTvSource", "GET /live-tv/sources/{sourceId}/guide": "liveTvGuide", "GET /live-tv/sources/{sourceId}/channels": "liveTvChannels",
		"GET /live-tv/channels/{channelId}": "liveTvChannel", "PATCH /live-tv/channels/{channelId}": "updateLiveTvChannel", "POST /live-tv/play": "startLiveTvPlayback",
		"GET /live-tv/streams": "liveTvStreams", "POST /live-tv/streams/{channelId}/open": "openLiveTvStream", "POST /live-tv/streams/{channelId}/close": "closeLiveTvStream",
		"GET /live-tv/logos/{channelId}": "liveTvLogoUrl", "GET /live-tv/hls/{channelId}/playlist.m3u8": "liveTvHlsPlaylistUrl", "HEAD /live-tv/hls/{channelId}/playlist.m3u8": "liveTvHlsPlaylistUrl",
		"GET /live-tv/hls/{channelId}/item": "liveTvHlsItemUrl", "HEAD /live-tv/hls/{channelId}/item": "liveTvHlsItemUrl", "GET /live-tv/hls/{channelId}/segment": "liveTvHlsSegmentUrl", "HEAD /live-tv/hls/{channelId}/segment": "liveTvHlsSegmentUrl",
		"GET /dvr/status": "dvrStatus", "GET /admin/dvr/status": "adminDvrOperationalStatus", "GET /dvr/rules": "dvrRules", "POST /dvr/rules": "createDvrRule",
		"GET /dvr/schedule": "dvrSchedule", "GET /dvr/rules/{id}": "dvrRule", "PATCH /dvr/rules/{id}": "updateDvrRule", "DELETE /dvr/rules/{id}": "deleteDvrRule",
		"GET /dvr/recording-groups": "dvrRecordingGroups", "GET /dvr/recordings": "dvrRecordings", "POST /dvr/recordings": "createDvrRecording",
		"GET /dvr/recordings/{id}": "dvrRecording", "PATCH /dvr/recordings/{id}": "updateDvrRecording", "DELETE /dvr/recordings/{id}": "deleteDvrRecording",
		"POST /dvr/recordings/{id}/playback": "playDvrRecording", "GET /dvr/recordings/{id}/stream": "dvrRecordingStreamUrl", "HEAD /dvr/recordings/{id}/stream": "dvrRecordingStreamUrl",
		"GET /dvr/recordings/{id}/hls/{resource}": "dvrRecordingHlsUrl", "HEAD /dvr/recordings/{id}/hls/{resource}": "dvrRecordingHlsUrl",
		"GET /library-channels": "libraryChannels", "GET /library-channels/guide": "libraryChannelsGuide", "GET /library-channels/{channelId}/guide": "libraryChannelGuide",
		"POST /library-channels/{channelId}/tune": "tuneLibraryChannel", "GET /library-channels/{channelId}/hls/{resource}": "libraryChannelHlsUrl", "HEAD /library-channels/{channelId}/hls/{resource}": "libraryChannelHlsUrl",
		"GET /library-channels/logos/{assetId}": "libraryChannelLogoUrl", "GET /admin/library-channels": "adminLibraryChannels", "POST /admin/library-channels": "createAdminLibraryChannel",
		"POST /admin/library-channels/logos": "uploadAdminLibraryChannelLogo", "DELETE /admin/library-channels/logos/{assetId}": "deleteAdminLibraryChannelLogo",
		"POST /admin/library-channels/reorder": "reorderAdminLibraryChannels", "GET /admin/library-channels/templates": "adminLibraryChannelTemplates",
		"GET /admin/library-channels/templates/applicability": "adminLibraryChannelTemplateApplicability", "POST /admin/library-channels/restore-defaults": "restoreAdminLibraryChannelDefaults",
		"GET /admin/library-channels/health": "adminLibraryChannelHealth", "GET /admin/library-channels/{channelId}": "adminLibraryChannel",
		"PUT /admin/library-channels/{channelId}": "replaceAdminLibraryChannel", "DELETE /admin/library-channels/{channelId}": "deleteAdminLibraryChannel",
		"POST /admin/library-channels/{channelId}/regenerate": "regenerateAdminLibraryChannel",
	}
	if value := overrides[key]; value != "" {
		return value
	}
	return operationID
}

func isLiveDVRLibraryPath(path string) bool {
	return strings.HasPrefix(path, "/live-tv") || strings.HasPrefix(path, "/dvr") || strings.HasPrefix(path, "/admin/dvr") || strings.HasPrefix(path, "/library-channels") || strings.HasPrefix(path, "/admin/library-channels") || strings.HasPrefix(path, "/playback/cast") || path == "/playback-sessions/{sessionId}/renegotiate"
}

func clientCoreContract(doc map[string]any) (clientCoreContractOutput, error) {
	methods := map[string]any{}
	schemas := map[string]string{}
	paths := object(doc["paths"])
	pathNames := make([]string, 0, len(paths))
	for path := range paths {
		if isLiveDVRLibraryPath(path) {
			pathNames = append(pathNames, path)
		}
	}
	sort.Strings(pathNames)
	entries := make([]map[string]string, 0)
	for _, path := range pathNames {
		item := object(paths[path])
		for _, method := range methodNames {
			op, ok := item[method]
			if !ok {
				continue
			}
			operation := object(op)
			operationID := asString(operation["operationId"])
			clientMethod := asString(operation["x-portico-client-core-method"])
			if clientMethod == "" {
				continue
			}
			audience := asString(operation["x-portico-audience"])
			schema := successSchemaName(operation)
			key := strings.ToUpper(method) + " " + path
			methods[key] = map[string]any{"operationId": operationID, "clientCoreMethod": clientMethod, "audience": audience}
			if schema != "" {
				schemas[operationID] = schema
			}
			entries = append(entries, map[string]string{"method": strings.ToUpper(method), "path": path, "operationId": operationID, "clientCoreMethod": clientMethod, "audience": audience, "responseSchema": schema})
		}
	}
	body, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return clientCoreContractOutput{}, err
	}
	ts := append([]byte("// Generated by cmd/genopenapi. Do not edit.\nexport const liveDvrLibraryOperationContract = "), body...)
	ts = append(ts, []byte(" as const;\n\nexport type LiveDvrLibraryOperation = (typeof liveDvrLibraryOperationContract)[number];\n")...)
	return clientCoreContractOutput{Methods: methods, Schemas: schemas, TypeScript: ts}, nil
}

func successSchemaName(operation map[string]any) string {
	responses := object(operation["responses"])
	for _, code := range []string{"200", "201", "202", "206"} {
		response := object(responses[code])
		content := object(response["content"])
		mediaTypes := make([]string, 0, len(content))
		for mediaType := range content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		for _, mediaType := range mediaTypes {
			ref := asString(object(object(content[mediaType])["schema"])["$ref"])
			if strings.HasPrefix(ref, "#/components/schemas/") {
				return strings.TrimPrefix(ref, "#/components/schemas/")
			}
		}
	}
	return ""
}

func resolvePathItem(paths map[string]any, path string, seen map[string]bool) any {
	if seen[path] {
		panic("cyclic OpenAPI path reference: " + path)
	}
	seen[path] = true
	item := object(paths[path])
	ref := asString(item["$ref"])
	if !strings.HasPrefix(ref, "#/paths/") {
		return item
	}
	target := strings.TrimPrefix(ref, "#/paths/")
	target = strings.ReplaceAll(strings.ReplaceAll(target, "~1", "/"), "~0", "~")
	return resolvePathItem(paths, target, seen)
}

func ensureSuccessSchema(responses, schemas map[string]any, operationID, route string) error {
	for _, code := range []string{"200", "201", "202", "204", "206", "302"} {
		responseRaw, ok := responses[code]
		if !ok {
			continue
		}
		if code == "204" {
			return nil
		}
		response := object(responseRaw)
		content := object(response["content"])
		if len(content) == 0 {
			return fmt.Errorf("OpenAPI success response is missing content: %s (%s)", route, operationID)
		}
		mediaTypes := make([]string, 0, len(content))
		for mediaType := range content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		for _, mediaType := range mediaTypes {
			media := object(content[mediaType])
			schema := object(media["schema"])
			if len(schema) == 0 {
				continue
			}
			if strings.TrimSpace(asString(schema["$ref"])) != "" {
				return nil
			}
			name := availableResponseComponentName(schemas, operationID)
			schemas[name] = cloneObject(schema)
			media["schema"] = map[string]any{"$ref": "#/components/schemas/" + name}
			content[mediaType] = media
			response["content"] = content
			responses[code] = response
			return nil
		}
		return fmt.Errorf("OpenAPI success response is missing a schema: %s (%s)", route, operationID)
	}
	return fmt.Errorf("OpenAPI operation is missing a success response: %s (%s)", route, operationID)
}

func responseComponentName(operationID string) string {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return "OperationResponse"
	}
	return strings.ToUpper(operationID[:1]) + operationID[1:] + "Response"
}

func availableResponseComponentName(schemas map[string]any, operationID string) string {
	base := responseComponentName(operationID)
	if _, exists := schemas[base]; !exists {
		return base
	}
	base += "Payload"
	if _, exists := schemas[base]; !exists {
		return base
	}
	for index := 2; ; index++ {
		candidate := base + strconv.Itoa(index)
		if _, exists := schemas[candidate]; !exists {
			return candidate
		}
	}
}

func ensureProblemResponses(responses map[string]any) {
	for _, code := range []string{"400", "401", "403", "404", "405", "409", "429", "500"} {
		status, _ := strconv.Atoi(code)
		response := object(responses[code])
		if strings.TrimSpace(asString(response["description"])) == "" {
			response["description"] = fmt.Sprintf("%d %s", status, statusText(status))
		}
		content := object(response["content"])
		problem := object(content["application/problem+json"])
		problem["schema"] = map[string]any{"$ref": "#/components/schemas/Error"}
		content["application/problem+json"] = problem
		response["content"] = content
		responses[code] = response
	}
}

func authFor(op, doc map[string]any) string {
	security, exists := op["security"]
	if !exists {
		security = doc["security"]
	}
	items, _ := security.([]any)
	if len(items) == 0 {
		return "public"
	}
	for _, item := range items {
		entry := object(item)
		if _, ok := entry["castReceiverAuth"]; ok {
			// Receiver bearer validation is performed by the Cast session handler;
			// the outer account-session middleware must not become a second actor.
			return "public"
		}
		if _, ok := entry["mediaGrantAuth"]; ok {
			return "media-grant-or-session"
		}
		if _, ok := entry["downloadGrantAuth"]; ok {
			return "media-grant-or-session"
		}
	}
	return "session"
}

func permissionFor(method, path string, op, doc map[string]any) string {
	if authFor(op, doc) == "public" {
		return "public"
	}
	if value := strings.TrimSpace(asString(op["x-portico-permission"])); value != "" {
		return value
	}
	method = strings.ToLower(method)
	switch {
	case strings.HasPrefix(path, "/playback"), strings.HasPrefix(path, "/watch-with-friends"), strings.HasPrefix(path, "/live-tv") && method != "patch" && !strings.Contains(path, "/sources"):
		return "play-media"
	case strings.Contains(path, "/download"):
		return "download-media"
	case strings.HasPrefix(path, "/users"), strings.HasPrefix(path, "/devices"), strings.Contains(path, "/invites"), strings.Contains(path, "/members"):
		return "manage-users"
	case strings.HasPrefix(path, "/libraries") && method != "get", strings.Contains(path, "/scan"):
		return "manage-libraries"
	case strings.HasPrefix(path, "/metadata"), strings.Contains(path, "/lyrics"), strings.Contains(path, "/subtitles"), strings.Contains(path, "/images"):
		return "edit-metadata"
	case strings.HasPrefix(path, "/system"), strings.HasPrefix(path, "/settings"), strings.HasPrefix(path, "/remote-access"), strings.HasPrefix(path, "/dashboard"), strings.HasPrefix(path, "/logs"), strings.HasPrefix(path, "/activity"), strings.HasPrefix(path, "/tasks"), strings.HasPrefix(path, "/backups"), strings.HasPrefix(path, "/dvr"):
		return "manage-server"
	default:
		return "authenticated"
	}
}

func audienceFor(permission string, op map[string]any) string {
	if explicit := strings.TrimSpace(asString(op["x-portico-audience"])); explicit != "" {
		if explicit != "viewer" && explicit != "management" {
			panic("invalid x-portico-audience: " + explicit)
		}
		return explicit
	}
	switch permission {
	case "edit-metadata", "manage-libraries", "manage-server", "manage-users":
		return "management"
	default:
		return "viewer"
	}
}

func surfacesFor(audience string, op map[string]any) []any {
	defaults := []any{"web", "mobile", "television"}
	allowed := map[string]bool{"web": true, "mobile": true, "television": true}
	if audience == "management" {
		defaults = []any{"web-admin"}
		allowed = map[string]bool{"web-admin": true}
	}

	raw, exists := op["x-portico-surfaces"]
	if !exists {
		return defaults
	}
	values, ok := raw.([]any)
	if !ok || len(values) == 0 {
		panic("x-portico-surfaces must be a non-empty array")
	}
	seen := map[string]bool{}
	result := make([]any, 0, len(values))
	for _, value := range values {
		surface := strings.TrimSpace(asString(value))
		if !allowed[surface] {
			panic(fmt.Sprintf("surface %q is not valid for %s operations", surface, audience))
		}
		if !seen[surface] {
			seen[surface] = true
			result = append(result, surface)
		}
	}
	if len(result) == 0 {
		panic("x-portico-surfaces must contain a canonical surface")
	}
	return result
}

func ratePolicyFor(method, path string, op map[string]any) string {
	if value := strings.TrimSpace(asString(op["x-portico-rate-policy"])); value != "" {
		return value
	}
	method = strings.ToLower(method)
	switch {
	case strings.HasPrefix(path, "/auth/"):
		return "auth-sensitive"
	case strings.Contains(path, "/stream"), strings.Contains(path, "/download"), strings.Contains(path, "/hls/"):
		return "media-delivery"
	case strings.HasPrefix(path, "/playback"), strings.HasPrefix(path, "/live-tv"):
		return "playback-control"
	case path == "/search" || strings.Contains(path, "/discover"):
		return "search"
	case method == "get" || method == "head":
		return "interactive-read"
	default:
		return "state-mutation"
	}
}

func auditEventFor(method, path string, op map[string]any) string {
	if value := strings.TrimSpace(asString(op["x-portico-audit-event"])); value != "" {
		return value
	}
	method = strings.ToLower(method)
	if method == "get" || method == "head" || method == "options" {
		return "none"
	}
	operation := strings.TrimSpace(asString(op["operationId"]))
	if operation == "" {
		operation = deterministicOperationID(method, path)
	}
	return "api." + operation
}

func deterministicOperationID(method, path string) string {
	parts := nonIdentifier.Split(strings.Trim(path, "/"), -1)
	result := strings.ToLower(method)
	for _, part := range parts {
		if part == "" {
			continue
		}
		result += strings.ToUpper(part[:1]) + part[1:]
	}
	return result
}

func normalizeYAML(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = normalizeYAML(item)
		}
		return result
	case map[any]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[fmt.Sprint(key)] = normalizeYAML(item)
		}
		return result
	case []any:
		for index := range typed {
			typed[index] = normalizeYAML(typed[index])
		}
		return typed
	default:
		return value
	}
}

func object(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if result, ok := value.(map[string]any); ok {
		return result
	}
	return map[string]any{}
}

func cloneObject(value map[string]any) map[string]any {
	body, err := json.Marshal(value)
	check(err)
	var result map[string]any
	check(json.Unmarshal(body, &result))
	return result
}

func asString(value any) string { result, _ := value.(string); return result }

func statusText(status int) string {
	switch status {
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 405:
		return "Method Not Allowed"
	case 409:
		return "Conflict"
	case 429:
		return "Too Many Requests"
	case 500:
		return "Internal Server Error"
	default:
		return "Error"
	}
}

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find repository root")
		}
		dir = parent
	}
}

func writeIfChanged(path string, body []byte) error {
	existing, _ := os.ReadFile(path)
	if string(existing) == string(body) {
		return nil
	}
	if *checkOnly {
		return fmt.Errorf("generated API artifact is stale: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
