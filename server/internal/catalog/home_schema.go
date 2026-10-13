package catalog

// The destination is one tagged choice. Query uses the same bounded browse
// expression vocabulary as saved views; its recursive contents are validated
// by the browse parser, rather than expanding an unbounded reflected schema.
func (HomeSeeAll) JSONSchema() map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	text := func() map[string]any { return map[string]any{"type": "string", "minLength": 1, "maxLength": 256} }
	tag := func(value string) map[string]any { return map[string]any{"type": "string", "enum": []string{value}} }
	sort := map[string]any{"type": "array", "minItems": 1, "maxItems": BrowseMaximumSorts, "items": object(map[string]any{"field": text(), "direction": map[string]any{"type": "string", "enum": []string{"asc", "desc"}}}, "field", "direction")}
	variants := []any{
		object(map[string]any{"type": tag("library"), "libraryId": text(), "pivot": text(), "query": map[string]any{"type": "object", "additionalProperties": map[string]any{}}, "sort": sort}, "type", "libraryId", "pivot", "sort"),
		object(map[string]any{"type": tag("row"), "rowId": text()}, "type", "rowId"),
		object(map[string]any{"type": tag("saved_view"), "resourceId": text()}, "type", "resourceId"),
	}
	properties := map[string]any{"type": map[string]any{"type": "string", "enum": []string{"library", "row", "saved_view"}}}
	for _, variant := range variants {
		for key, value := range variant.(map[string]any)["properties"].(map[string]any) {
			if key != "type" {
				properties[key] = value
			}
		}
	}
	return map[string]any{"type": "object", "properties": properties, "required": []string{"type"}, "additionalProperties": false, "oneOf": variants}
}

func (ContentSeeAll) JSONSchema() map[string]any {
	return map[string]any{"type": "object", "required": []string{"pivot", "sort"}, "properties": map[string]any{
		"pivot": map[string]any{"type": "string", "minLength": 1, "maxLength": 256},
		"query": map[string]any{"type": "object", "additionalProperties": map[string]any{}},
		"sort":  map[string]any{"type": "array", "maxItems": BrowseMaximumSorts, "items": map[string]any{"type": "object", "required": []string{"field", "direction"}, "properties": map[string]any{"field": map[string]any{"type": "string"}, "direction": map[string]any{"type": "string", "enum": []string{"asc", "desc"}}}}},
	}}
}
