package playback

import (
	"reflect"

	"portico.local/apikit"
)

func (PlaybackInfo) JSONSchema() map[string]any {
	type wire PlaybackInfo
	shape, _ := apikit.Schema(reflect.TypeFor[wire]())
	shape["properties"].(map[string]any)["basis"].(map[string]any)["enum"] = []string{"committed_delivery_plan"}
	return shape
}
