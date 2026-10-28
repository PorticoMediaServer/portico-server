package httpapi

import (
	"encoding/json"
	"testing"

	"portico.local/apikit"
)

func TestGeneratedOpenAPIReadBodiesRequiredListsAndRecommendationQueries(t *testing.T) {
	registry, err := FoundationRegistry(Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	components, wire, err := PublishedSchemas()
	if err != nil {
		t.Fatal(err)
	}
	generated, err := apikit.GenerateWith(registry.Definitions(), apikit.Options{Errors: registry.ErrorDefinitions(), Components: components, WireSchemas: wire})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err = json.Unmarshal(generated.OpenAPI, &document); err != nil {
		t.Fatal(err)
	}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "properties" {
					// These keys are property names, not schema keywords. A
					// property named "required" still has its own schema to check.
					properties, ok := child.(map[string]any)
					if !ok {
						t.Fatalf("invalid schema properties member: %#v", child)
					}
					for _, schema := range properties {
						walk(schema)
					}
					continue
				}
				if key == "required" {
					// Parameters and request bodies use a boolean. A schema's
					// required member must be an array of unique property names.
					if _, ok := child.(bool); !ok {
						names, ok := child.([]any)
						if !ok {
							t.Fatalf("invalid schema required member: %#v", child)
						}
						seen := map[string]bool{}
						for _, name := range names {
							text, ok := name.(string)
							if !ok || text == "" || seen[text] {
								t.Fatalf("invalid required property name: %#v", name)
							}
							seen[text] = true
						}
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(document)
	paths, ok := document["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatal("generated document has no paths")
	}
	for path, raw := range paths {
		operations := raw.(map[string]any)
		for _, method := range []string{"get", "head"} {
			if operation, ok := operations[method].(map[string]any); ok {
				if _, present := operation["requestBody"]; present {
					t.Fatalf("%s %s declares a request body", method, path)
				}
			}
		}
	}
	for _, path := range []string{"/v1/suggestions", "/v1/items/{id}/recommendations"} {
		operation := paths[path].(map[string]any)["get"].(map[string]any)
		parameters, _ := operation["parameters"].([]any)
		found := false
		for _, raw := range parameters {
			parameter := raw.(map[string]any)
			found = found || parameter["in"] == "query" && parameter["name"] == "limit"
		}
		if !found {
			t.Fatalf("%s omits the limit query parameter", path)
		}
	}
}
