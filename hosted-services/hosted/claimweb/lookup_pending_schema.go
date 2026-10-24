package claimweb

import (
	"reflect"

	"portico.local/apikit"
)

// Lookup alone projects a verified count. An absent proof is not null or zero;
// zero is a genuine signed fact and remains a present integer.
func (LookupPending) JSONSchema() map[string]any {
	schema, e := apikit.Schema(reflect.TypeFor[Pending]())
	if e != nil {
		panic(e)
	}
	schema["additionalProperties"] = false
	schema["properties"].(map[string]any)["otherPeopleCount"] = map[string]any{
		"type": "integer", "minimum": 0, "maximum": 10000,
	}
	return schema
}
