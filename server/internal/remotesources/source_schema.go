package remotesources

import (
	"reflect"

	"portico.local/apikit"
)

// Source kinds describe the saved producer. Endpoint admission remains with
// its native protocol and the current source's recorded approval.
func (Source) JSONSchema() map[string]any {
	type source Source
	schema, err := apikit.Schema(reflect.TypeFor[source]())
	if err != nil {
		panic(err)
	}
	schema["properties"].(map[string]any)["kind"] = map[string]any{
		"type": "string", "enum": []string{"rclone", "webdav", "sftp", "smb"},
	}
	return schema
}
