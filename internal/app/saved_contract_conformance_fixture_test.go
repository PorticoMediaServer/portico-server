package app

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestSavedResourceConformanceFixtureMatchesOpenAPI(t *testing.T) {
	fixtureBytes, err := os.ReadFile("../../api/openapi/fixtures/saved-resources-conformance.json")
	if err != nil {
		t.Fatalf("read saved resource conformance fixture: %v", err)
	}
	var fixture struct {
		ContractRevision string `json:"contractRevision"`
		PlaylistEntry    struct {
			RequiredFields []string `json:"requiredFields"`
			PositionBase   int      `json:"positionBase"`
		} `json:"playlistEntry"`
		PlaylistMutation struct {
			AddField      string `json:"addField"`
			RemoveField   string `json:"removeField"`
			OrderField    string `json:"orderField"`
			RevisionField string `json:"revisionField"`
		} `json:"playlistMutation"`
		ItemPaging []struct {
			Kind           string `json:"kind"`
			Method         string `json:"method"`
			Path           string `json:"path"`
			RequestSchema  string `json:"requestSchema"`
			ResponseSchema string `json:"responseSchema"`
		} `json:"itemPaging"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("decode saved resource conformance fixture: %v", err)
	}
	if fixture.ContractRevision != "v1" || len(fixture.ItemPaging) != 3 {
		t.Fatalf("fixture revision or paging coverage is incomplete: %#v", fixture)
	}

	contractBytes, err := os.ReadFile("../../api/openapi/portico-server.openapi.json")
	if err != nil {
		t.Fatalf("read generated OpenAPI contract: %v", err)
	}
	var contract map[string]any
	if err := json.Unmarshal(contractBytes, &contract); err != nil {
		t.Fatalf("decode generated OpenAPI contract: %v", err)
	}
	schemas := nestedObject(t, contract, "components", "schemas")
	entrySchema := nestedObject(t, schemas, "PlaylistEntry")
	required := stringValues(t, entrySchema["required"])
	sort.Strings(required)
	wantRequired := append([]string{}, fixture.PlaylistEntry.RequiredFields...)
	sort.Strings(wantRequired)
	if !reflect.DeepEqual(required, wantRequired) {
		t.Fatalf("playlist entry required fields=%v want=%v", required, wantRequired)
	}
	position := nestedObject(t, nestedObject(t, entrySchema, "properties"), "position")
	if minimum, ok := position["minimum"].(float64); !ok || int(minimum) != fixture.PlaylistEntry.PositionBase {
		t.Fatalf("playlist position minimum=%v want=%d", position["minimum"], fixture.PlaylistEntry.PositionBase)
	}

	mutationProperties := nestedObject(t, nestedObject(t, schemas, "PlaylistItemsBatchRequest"), "properties")
	for _, field := range []string{fixture.PlaylistMutation.AddField, fixture.PlaylistMutation.RemoveField, fixture.PlaylistMutation.OrderField, fixture.PlaylistMutation.RevisionField} {
		if _, ok := mutationProperties[field]; !ok {
			t.Fatalf("playlist mutation contract omitted %s", field)
		}
	}
	for _, legacy := range []string{"removeMediaIds", "orderMediaIds"} {
		if _, ok := mutationProperties[legacy]; ok {
			t.Fatalf("playlist mutation contract retained ambiguous field %s", legacy)
		}
	}

	paths := nestedObject(t, contract, "paths")
	for _, paging := range fixture.ItemPaging {
		operation := nestedObject(t, nestedObject(t, paths, paging.Path), strings.ToLower(paging.Method))
		response := nestedObject(t, nestedObject(t, nestedObject(t, operation, "responses"), "200"), "content", "application/json", "schema")
		if reference, _ := response["$ref"].(string); reference != "#/components/schemas/"+paging.ResponseSchema {
			t.Fatalf("%s response schema=%q want=%q", paging.Kind, reference, paging.ResponseSchema)
		}
		if paging.RequestSchema != "" {
			request := nestedObject(t, operation, "requestBody", "content", "application/json", "schema")
			if reference, _ := request["$ref"].(string); reference != "#/components/schemas/"+paging.RequestSchema {
				t.Fatalf("%s request schema=%q want=%q", paging.Kind, reference, paging.RequestSchema)
			}
			requestProperties := nestedObject(t, schemas, paging.RequestSchema, "properties")
			if _, cursor := requestProperties["cursor"]; !cursor {
				t.Fatalf("%s request omitted cursor", paging.Kind)
			}
			if _, limit := requestProperties["limit"]; !limit {
				t.Fatalf("%s request omitted limit", paging.Kind)
			}
			continue
		}
		parameterNames := map[string]bool{}
		for _, value := range objectValues(t, operation["parameters"]) {
			if name, _ := value["name"].(string); name != "" {
				parameterNames[name] = true
			}
		}
		if !parameterNames["cursor"] || !parameterNames["limit"] {
			t.Fatalf("%s paging parameters=%v", paging.Kind, parameterNames)
		}
	}
}

func nestedObject(t *testing.T, root map[string]any, path ...string) map[string]any {
	t.Helper()
	current := root
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("contract path %s is missing or not an object", strings.Join(path, "."))
		}
		current = next
	}
	return current
}

func objectValues(t *testing.T, value any) []map[string]any {
	t.Helper()
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("contract value is not an array: %#v", value)
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("contract array item is not an object: %#v", value)
		}
		result = append(result, item)
	}
	return result
}

func stringValues(t *testing.T, value any) []string {
	t.Helper()
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("contract value is not an array: %#v", value)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		item, ok := value.(string)
		if !ok {
			t.Fatalf("contract array item is not a string: %#v", value)
		}
		result = append(result, item)
	}
	return result
}
