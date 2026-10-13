package downloads

import (
	"portico.local/server/internal/apiwire"
	"reflect"
)

type progressEntryWire ProgressEntry

func (v ProgressEntry) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(progressEntryWire(v), map[string]string{"positionSeconds": "positionMs"})
}
func (ProgressEntry) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[progressEntryWire](), map[string]string{"positionSeconds": "positionMs"})
}
func (v *ProgressEntry) UnmarshalJSON(raw []byte) error {
	return apiwire.UnmarshalDurations(raw, (*progressEntryWire)(v), map[string]string{"positionSeconds": "positionMs"})
}

type progressResultWire ProgressResult

func (v ProgressResult) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(progressResultWire(v), map[string]string{"positionSeconds": "positionMs"})
}
func (ProgressResult) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[progressResultWire](), map[string]string{"positionSeconds": "positionMs"})
}
func (v *ProgressResult) UnmarshalJSON(raw []byte) error {
	return apiwire.UnmarshalDurations(raw, (*progressResultWire)(v), map[string]string{"positionSeconds": "positionMs"})
}

type sourceDescriptionWire SourceDescription

func (v SourceDescription) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(sourceDescriptionWire(v), map[string]string{"durationSeconds": "durationMs"})
}
func (SourceDescription) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[sourceDescriptionWire](), map[string]string{"durationSeconds": "durationMs"})
}
