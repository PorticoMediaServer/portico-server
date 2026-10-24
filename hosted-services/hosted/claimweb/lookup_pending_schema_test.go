package claimweb

import "testing"

func TestLookupCountSchemaIsOptionalBoundedAndNonnullable(t *testing.T) {
	schema := (LookupPending{}).JSONSchema()
	count := schema["properties"].(map[string]any)["otherPeopleCount"].(map[string]any)
	if count["type"] != "integer" || count["minimum"] != 0 || count["maximum"] != 10000 || schema["additionalProperties"] != false {
		t.Fatal("lookup count is not a closed bounded integer")
	}
	for _, field := range schema["required"].([]string) {
		if field == "otherPeopleCount" {
			t.Fatal("absent count became required")
		}
	}
}
