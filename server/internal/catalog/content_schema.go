package catalog

import (
	"portico.local/apikit"
	"reflect"
)

// Media is a single wrapper around a catalogue card (for example a saved
// entry). Publish the card's concrete fields at both levels, without allowing
// a recursively nested response that none of the catalogue projections emits.
func (ContentEntry) JSONSchema() map[string]any {
	source := reflect.TypeFor[ContentEntry]()
	fields := make([]reflect.StructField, 0, source.NumField()-1)
	for n := 0; n < source.NumField(); n++ {
		field := source.Field(n)
		if field.Name != "Media" {
			fields = append(fields, field)
		}
	}
	leaf := reflect.StructOf(fields)
	root, err := apikit.Schema(leaf)
	if err != nil {
		panic(err)
	}
	media, err := apikit.Schema(leaf)
	if err != nil {
		panic(err)
	}
	root["properties"].(map[string]any)["media"] = media
	return root
}
