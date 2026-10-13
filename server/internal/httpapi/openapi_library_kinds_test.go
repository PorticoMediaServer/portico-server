package httpapi

import (
	"encoding/json"
	"os"
	"testing"
)

// Library creation and listing publish the same kind vocabulary as the server.
func TestOpenAPILibraryKindsMatchServer(t *testing.T) {
	raw, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]struct {
					Schema struct {
						Properties map[string]struct {
							Type string
							Enum []string
						}
					}
				}
			}
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	kind := doc.Paths["/v1/libraries"]["post"].RequestBody.Content["application/json"].Schema.Properties["kind"]
	if kind.Type != "string" || len(kind.Enum) != 5 {
		t.Fatal("library creation has no complete kind vocabulary")
	}
	want := []string{"movie", "tv", "anime", "music", "audiobook"}
	for i, expected := range want {
		if kind.Enum[i] != expected {
			t.Fatalf("library kind %d: got %s, want %s", i, kind.Enum[i], expected)
		}
	}
}
