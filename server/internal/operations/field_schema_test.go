package operations

import (
	"reflect"
	"testing"

	"portico.local/apikit"
)

// The console registry publishes null defaults for its nullable integer
// settings. The generated decoder must accept those and keep every other
// required member, including non-null defaults, exactly as before.
func TestFieldSchemaAdmitsOnlyTheRequiredDefaultNull(t *testing.T) {
	shape, err := apikit.Schema(reflect.TypeFor[Field]())
	if err != nil {
		t.Fatal(err)
	}
	if shape["type"] != "object" || shape["additionalProperties"] != false {
		t.Fatalf("field schema is not a closed object: %v", shape)
	}
	required := map[string]bool{}
	for _, name := range shape["required"].([]string) {
		required[name] = true
	}
	for _, name := range []string{"id", "group", "scope", "overrideScopes", "type", "default", "saveMode", "applicationMode", "runtimeConsumer", "description", "permission", "secret", "retentionClass", "auditClass"} {
		if !required[name] {
			t.Fatalf("%s is no longer required: %v", name, shape["required"])
		}
	}
	for _, name := range []string{"min", "max", "options"} {
		if required[name] {
			t.Fatalf("%s became required: %v", name, shape["required"])
		}
	}
	properties := shape["properties"].(map[string]any)
	for name, raw := range properties {
		property := raw.(map[string]any)
		nullable, _ := property["nullable"].(bool)
		if name == "default" {
			if property["type"] != "any" || !nullable {
				t.Fatalf("default does not admit null: %v", property)
			}
			continue
		}
		if nullable {
			t.Fatalf("%s unexpectedly admits null: %v", name, property)
		}
	}

	var nullableIntegers, otherDefaults int
	for _, field := range Registry() {
		if field.Type != "nullable-integer" {
			if field.Default != nil {
				otherDefaults++
			}
			continue
		}
		if field.Default != nil {
			t.Fatalf("%s no longer publishes a null default: %#v", field.ID, field.Default)
		}
		nullableIntegers++
	}
	if nullableIntegers == 0 || otherDefaults == 0 {
		t.Fatalf("registry lost its default coverage: %d nullable integers, %d other defaults", nullableIntegers, otherDefaults)
	}
}
