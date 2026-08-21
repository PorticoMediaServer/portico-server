package app

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/cookiejar"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPublishedContractSchemasTrackRuntimeTypes(t *testing.T) {
	openAPI := readJSONDocument(t, "../../api/openapi/portico-server.openapi.json")
	components := schemaObject(t, schemaObject(t, openAPI, "components"), "schemas")

	homeRow := schemaObject(t, components, "HomeRow")
	homeProperties := schemaObject(t, homeRow, "properties")
	for _, name := range []string{"total", "limit", "offset"} {
		property := schemaObject(t, homeProperties, name)
		if got := property["type"]; got != "integer" {
			t.Fatalf("HomeRow.%s type = %#v, want integer", name, got)
		}
	}

	progress := schemaObject(t, components, "PlaybackProgressEvent")
	progressProperties := schemaObject(t, progress, "properties")
	for name, want := range map[string]string{
		"positionSeconds":      "number",
		"clientFallbackReason": "string",
		"isPlaying":            "boolean",
	} {
		property := schemaObject(t, progressProperties, name)
		if got := property["type"]; got != want {
			t.Fatalf("PlaybackProgressEvent.%s type = %#v, want %s", name, got, want)
		}
	}

	productSchema := readJSONDocument(t, "../../api/schema/product-contract.schema.json")
	required := stringSetFromSchemaList(t, productSchema["required"])
	if !required["mediaActions"] {
		t.Fatal("Product Contract JSON Schema does not require mediaActions")
	}
	for _, field := range []string{"eventTransports", "longPoll"} {
		if !required[field] {
			t.Fatalf("Product Contract JSON Schema does not require %s", field)
		}
	}
	productProperties := schemaObject(t, productSchema, "properties")
	mediaActions := schemaObject(t, productProperties, "mediaActions")
	if mediaActions["type"] != "array" {
		t.Fatalf("Product Contract mediaActions type = %#v, want array", mediaActions["type"])
	}
	eventTransports := schemaObject(t, productProperties, "eventTransports")
	if eventTransports["type"] != "array" {
		t.Fatalf("Product Contract eventTransports type = %#v, want array", eventTransports["type"])
	}
	if _, ok := productProperties["longPoll"].(map[string]any); !ok {
		t.Fatalf("Product Contract longPoll schema = %#v, want object reference", productProperties["longPoll"])
	}

	profilePolicySchema := readJSONDocument(t, "../../api/schema/profile-policy.schema.json")
	assertSchemaValid(t, profilePolicySchema, profilePolicySchema, map[string]any{
		"version":               "v1",
		"maximumAgeRating":      float64(13),
		"allowUnrated":          false,
		"blockedLabels":         []any{"Explicit"},
		"allowDownloads":        true,
		"allowLiveTV":           true,
		"allowDvr":              false,
		"allowWatchWithFriends": true,
		"allowFeedback":         true,
	}, "Profile Policy schema fixture")
}

func TestPublishedTelemetryStatusFieldsAreRequired(t *testing.T) {
	openAPI := readJSONDocument(t, "../../api/openapi/portico-server.openapi.json")
	components := schemaObject(t, schemaObject(t, openAPI, "components"), "schemas")

	wantRequired := map[string][]string{
		"ServerActivityResponse": {"cpuStatus", "memoryStatus"},
		"DashboardSystemSample":  {"serverStatus", "systemStatus"},
		"DashboardGPUSample":     {"usageStatus", "memoryStatus", "encoderStatus", "headroomStatus"},
	}
	for schemaName, fields := range wantRequired {
		schema := schemaObject(t, components, schemaName)
		required := stringSetFromSchemaList(t, schema["required"])
		properties := schemaObject(t, schema, "properties")
		for _, field := range fields {
			if !required[field] {
				t.Errorf("%s does not require telemetry status field %q", schemaName, field)
			}
			property := schemaObject(t, properties, field)
			if reference := property["$ref"]; reference != "#/components/schemas/TelemetryMetricStatus" {
				t.Errorf("%s.%s schema = %#v, want TelemetryMetricStatus reference", schemaName, field, reference)
			}
		}
	}
}

func TestPublishedContractRuntimeConformance(t *testing.T) {
	openAPI := readJSONDocument(t, "../../api/openapi/portico-server.openapi.json")
	components := schemaObject(t, schemaObject(t, openAPI, "components"), "schemas")
	productJSONSchema := readJSONDocument(t, "../../api/schema/product-contract.schema.json")

	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	if _, err := db.Exec(`
		UPDATE media_items SET source_url = 'https://media.example.com/contract-fixture.mp4' WHERE id = 'movie_meridian';
		DELETE FROM media_analysis_facts WHERE media_id = 'movie_meridian';
		DELETE FROM media_files WHERE media_id = 'movie_meridian';
		UPDATE media_streams SET codec = 'h264', profile = 'main', pixel_format = 'yuv420p', bit_depth = 8,
			width = 1920, height = 1080, frame_rate = 24, stream_index = 0 WHERE media_id = 'movie_meridian' AND kind = 'video';
		UPDATE media_streams SET codec = 'aac', profile = 'lc', channels = 2, channel_layout = 'stereo', sample_rate = 48000,
			stream_index = 1 WHERE media_id = 'movie_meridian' AND kind = 'audio'`); err != nil {
		t.Fatalf("seed exact playback stream facts: %v", err)
	}
	var productPayload any
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/product-contract", nil, &productPayload)
	if status != http.StatusOK {
		t.Fatalf("Product Contract status=%d body=%s", status, body)
	}
	assertSchemaValid(t, openAPI, schemaObject(t, components, "ProductContract"), productPayload, "ProductContract response")
	assertSchemaValid(t, productJSONSchema, productJSONSchema, productPayload, "Product Contract JSON Schema")

	var homePayload any
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/home/rows/continue?limit=1", nil, &homePayload)
	if status != http.StatusOK {
		t.Fatalf("HomeRow status=%d body=%s", status, body)
	}
	assertSchemaValid(t, openAPI, schemaObject(t, components, "HomeRow"), homePayload, "HomeRow response")

	var playback PlaybackResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", map[string]any{
		"mediaId":     "movie_meridian",
		"skipPreroll": true,
		"clientProfile": map[string]any{
			"capabilitySchemaVersion": playbackCapabilitySchemaV2,
			"clientFamily":            "safari",
			"clientVersion":           "18",
			"platform":                "web",
			"device":                  "Contract Fixture",
			"capabilityEvidence": []any{map[string]any{
				"id": "contract-fixture-runtime-v1", "source": "unauthenticated_probe", "confidence": "high",
				"producer": "published-contract-test", "reviewedAt": time.Now().UTC().Format(time.RFC3339),
				"tuples": []any{map[string]any{
					"mediaKind": "audiovisual", "protocol": "http", "container": "mp4",
					"video":    map[string]any{"codec": "h264", "dynamicRange": "sdr", "bitDepth": 8, "maxWidth": 1920, "maxHeight": 1080, "maxFrameRate": 30},
					"audio":    map[string]any{"codec": "aac", "layout": "stereo", "route": "decode", "maxChannels": 2},
					"subtitle": map[string]any{"mode": "none"},
				}},
			}},
		},
	}, &playback)
	if status != http.StatusOK {
		t.Fatalf("create playback status=%d body=%s", status, body)
	}
	progressPayload := map[string]any{
		"eventSequence":        1,
		"recordedAt":           time.Now().UTC().Format(time.RFC3339Nano),
		"positionSeconds":      15.5,
		"clientFallbackReason": "native-player-recovery",
		"isPlaying":            false,
	}
	assertSchemaValid(t, openAPI, schemaObject(t, components, "PlaybackProgressEvent"), progressPayload, "PlaybackProgressEvent request")
	var acknowledgementPayload any
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, progressPayload, &acknowledgementPayload)
	if status != http.StatusOK {
		t.Fatalf("playback progress status=%d body=%s", status, body)
	}
	assertSchemaValid(t, openAPI, schemaObject(t, components, "PlaybackProgressAcknowledgement"), acknowledgementPayload, "PlaybackProgressAcknowledgement response")
	acknowledgement, ok := acknowledgementPayload.(map[string]any)
	if !ok || acknowledgement["accepted"] != true || acknowledgement["sessionState"] != "paused" {
		t.Fatalf("playback progress fields were not applied: %#v", acknowledgementPayload)
	}
}

func readJSONDocument(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return document
}

func schemaObject(t *testing.T, value map[string]any, key string) map[string]any {
	t.Helper()
	result, ok := value[key].(map[string]any)
	if !ok {
		t.Fatalf("schema key %q is %#v, want object", key, value[key])
	}
	return result
}

func stringSetFromSchemaList(t *testing.T, value any) map[string]bool {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("schema list is %#v", value)
	}
	result := make(map[string]bool, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("schema list item is %#v", item)
		}
		result[text] = true
	}
	return result
}

func assertSchemaValid(t *testing.T, root, schema map[string]any, value any, label string) {
	t.Helper()
	if err := validatePublishedSchema(root, schema, value, "$", map[string]bool{}); err != nil {
		t.Fatalf("%s does not match the published schema: %v", label, err)
	}
}

// validatePublishedSchema intentionally supports the JSON Schema/OpenAPI
// vocabulary used by the representative contracts above. Keeping this small
// validator in the server test suite makes runtime conformance deterministic
// without introducing a second contract parser into production code.
func validatePublishedSchema(root, schema map[string]any, value any, path string, refs map[string]bool) error {
	if ref, _ := schema["$ref"].(string); ref != "" {
		if refs[ref] {
			return nil
		}
		resolved, err := resolvePublishedSchemaRef(root, ref)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		nextRefs := make(map[string]bool, len(refs)+1)
		for key, present := range refs {
			nextRefs[key] = present
		}
		nextRefs[ref] = true
		return validatePublishedSchema(root, resolved, value, path, nextRefs)
	}
	if alternatives, ok := schema["oneOf"].([]any); ok {
		matches := 0
		for _, raw := range alternatives {
			candidate, ok := raw.(map[string]any)
			if ok && validatePublishedSchema(root, candidate, value, path, refs) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%s: matched %d oneOf alternatives", path, matches)
		}
		return nil
	}
	if types, ok := schema["type"].([]any); ok {
		matches := 0
		for _, raw := range types {
			typeName, ok := raw.(string)
			if !ok {
				return fmt.Errorf("%s: invalid union schema type %#v", path, raw)
			}
			candidate := make(map[string]any, len(schema))
			for key, candidateValue := range schema {
				candidate[key] = candidateValue
			}
			candidate["type"] = typeName
			if validatePublishedSchema(root, candidate, value, path, refs) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%s: matched %d union schema types", path, matches)
		}
		return nil
	}
	if schemas, ok := schema["allOf"].([]any); ok {
		for _, raw := range schemas {
			candidate, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s: invalid allOf schema", path)
			}
			if err := validatePublishedSchema(root, candidate, value, path, refs); err != nil {
				return err
			}
		}
	}
	if enum, ok := schema["enum"].([]any); ok {
		matched := false
		for _, candidate := range enum {
			if reflect.DeepEqual(candidate, value) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: value %#v is not in enum", path, value)
		}
	}

	switch schema["type"] {
	case nil:
		return nil
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: got %T, want object", path, value)
		}
		if required, ok := schema["required"].([]any); ok {
			for _, raw := range required {
				name, _ := raw.(string)
				if _, exists := object[name]; !exists {
					return fmt.Errorf("%s: missing required property %q", path, name)
				}
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for name, childValue := range object {
			raw, declared := properties[name]
			if !declared {
				switch additional := schema["additionalProperties"].(type) {
				case bool:
					if !additional {
						return fmt.Errorf("%s: unexpected property %q", path, name)
					}
				case map[string]any:
					if err := validatePublishedSchema(root, additional, childValue, path+"."+name, refs); err != nil {
						return err
					}
				}
				continue
			}
			childSchema, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.%s: invalid property schema", path, name)
			}
			if err := validatePublishedSchema(root, childSchema, childValue, path+"."+name, refs); err != nil {
				return err
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: got %T, want array", path, value)
		}
		if minimum, ok := schemaNumber(schema["minItems"]); ok && float64(len(items)) < minimum {
			return fmt.Errorf("%s: has %d items, minimum is %.0f", path, len(items), minimum)
		}
		if itemSchema, ok := schema["items"].(map[string]any); ok {
			for index, item := range items {
				if err := validatePublishedSchema(root, itemSchema, item, fmt.Sprintf("%s[%d]", path, index), refs); err != nil {
					return err
				}
			}
		}
		if unique, _ := schema["uniqueItems"].(bool); unique {
			seen := map[string]bool{}
			for _, item := range items {
				encoded, _ := json.Marshal(item)
				key := string(encoded)
				if seen[key] {
					return fmt.Errorf("%s: contains duplicate item %s", path, key)
				}
				seen[key] = true
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s: got %T, want string", path, value)
		}
		if minimum, ok := schemaNumber(schema["minLength"]); ok && float64(len([]rune(text))) < minimum {
			return fmt.Errorf("%s: string is shorter than %.0f characters", path, minimum)
		}
		if pattern, _ := schema["pattern"].(string); pattern != "" {
			compiled, err := regexp.Compile(pattern)
			if err != nil || !compiled.MatchString(text) {
				return fmt.Errorf("%s: %q does not match %q", path, text, pattern)
			}
		}
	case "integer":
		number, ok := schemaNumber(value)
		if !ok || math.Trunc(number) != number {
			return fmt.Errorf("%s: got %#v, want integer", path, value)
		}
		if err := validateSchemaNumberBounds(schema, number, path); err != nil {
			return err
		}
	case "number":
		number, ok := schemaNumber(value)
		if !ok {
			return fmt.Errorf("%s: got %T, want number", path, value)
		}
		if err := validateSchemaNumberBounds(schema, number, path); err != nil {
			return err
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: got %T, want boolean", path, value)
		}
	case "null":
		if value != nil {
			return fmt.Errorf("%s: got %T, want null", path, value)
		}
	default:
		return fmt.Errorf("%s: unsupported schema type %#v", path, schema["type"])
	}
	return nil
}

func resolvePublishedSchemaRef(root map[string]any, ref string) (map[string]any, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("unsupported external schema reference %q", ref)
	}
	var current any = root
	for _, token := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("schema reference %q crosses a non-object", ref)
		}
		current, ok = object[token]
		if !ok {
			return nil, fmt.Errorf("schema reference %q does not exist", ref)
		}
	}
	result, ok := current.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema reference %q is not an object", ref)
	}
	return result, nil
}

func schemaNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case int32:
		return float64(number), true
	default:
		return 0, false
	}
}

func validateSchemaNumberBounds(schema map[string]any, number float64, path string) error {
	if minimum, ok := schemaNumber(schema["minimum"]); ok && number < minimum {
		return fmt.Errorf("%s: %.2f is below minimum %.2f", path, number, minimum)
	}
	if maximum, ok := schemaNumber(schema["maximum"]); ok && number > maximum {
		return fmt.Errorf("%s: %.2f exceeds maximum %.2f", path, number, maximum)
	}
	return nil
}
