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
	// The signed transition mode is optional and non-null: omission is the legacy
	// ordinary switch, and only the closed pair can ever be projected.
	schema["properties"].(map[string]any)["transitionMode"] = map[string]any{
		"type": "string", "enum": []string{"switch", "start-over"},
	}
	return schema
}
