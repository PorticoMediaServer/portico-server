package operations

import (
	"portico.local/apikit"
	"reflect"
)

// Settings publishes the exact confidence choices accepted by its write path.
func (Settings) JSONSchema() map[string]any {
	type wire Settings
	schema, err := apikit.Schema(reflect.TypeFor[wire]())
	if err != nil {
		panic(err)
	}
	properties := schema["properties"].(map[string]any)
	properties["segments.tvMode"].(map[string]any)["enum"] = []string{"server", "standard", "thorough", "off"}
	properties["segments.animeMode"].(map[string]any)["enum"] = []string{"server", "standard", "thorough", "off"}
	properties["segments.movieMode"].(map[string]any)["enum"] = []string{"server", "standard", "thorough", "off"}
	properties["segments.useChapterNames"] = map[string]any{"type": "boolean"}
	properties["segments.viewerConfidencePercent"].(map[string]any)["enum"] = []int{30, 50, 70}
	properties["segments.automaticConfidencePercent"].(map[string]any)["enum"] = []int{70, 85, 95}
	return schema
}
