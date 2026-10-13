package operations

import (
	"portico.local/server/internal/apiwire"
	"reflect"
)

type playbackHistoryWire PlaybackHistoryEntry

func (v PlaybackHistoryEntry) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(playbackHistoryWire(v), map[string]string{"durationSeconds": "durationMs"})
}
func (PlaybackHistoryEntry) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[playbackHistoryWire](), map[string]string{"durationSeconds": "durationMs"})
}
func (v *PlaybackHistoryEntry) UnmarshalJSON(raw []byte) error {
	return apiwire.UnmarshalDurations(raw, (*playbackHistoryWire)(v), map[string]string{"durationSeconds": "durationMs"})
}
