package catalog

import (
	"portico.local/server/internal/apiwire"
	"reflect"
)

type itemWire Item

func (v Item) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(itemWire(v), map[string]string{"duration": "durationMs", "progressSeconds": "progressMs"})
}
func (Item) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[itemWire](), map[string]string{"duration": "durationMs", "progressSeconds": "progressMs"})
}

type contentEntryWire ContentEntry

func (ContentEntry) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[contentEntryWire](), map[string]string{"duration": "durationMs", "progressSeconds": "progressMs"})
}

type contentPlaybackWire ContentPlayback

func (v ContentPlayback) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(contentPlaybackWire(v), map[string]string{"startSeconds": "startMs"})
}
func (ContentPlayback) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[contentPlaybackWire](), map[string]string{"startSeconds": "startMs"})
}

type personalStateWire PersonalState

func (v PersonalState) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(personalStateWire(v), map[string]string{"progressSeconds": "progressMs"})
}
func (PersonalState) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[personalStateWire](), map[string]string{"progressSeconds": "progressMs"})
}
func (v *PersonalState) UnmarshalJSON(raw []byte) error {
	return apiwire.UnmarshalDurations(raw, (*personalStateWire)(v), map[string]string{"progressSeconds": "progressMs"})
}

type personalMutationWire PersonalMutation

func (v PersonalMutation) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(personalMutationWire(v), map[string]string{"progressSeconds": "progressMs"})
}
func (PersonalMutation) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[personalMutationWire](), map[string]string{"progressSeconds": "progressMs"})
}
func (v *PersonalMutation) UnmarshalJSON(raw []byte) error {
	return apiwire.UnmarshalDurations(raw, (*personalMutationWire)(v), map[string]string{"progressSeconds": "progressMs"})
}

type titleFileWire TitleFile

func (v TitleFile) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(titleFileWire(v), map[string]string{"duration": "durationMs"})
}
func (TitleFile) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[titleFileWire](), map[string]string{"duration": "durationMs"})
}

type bookResumeWire BookResume

func (v BookResume) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(bookResumeWire(v), map[string]string{"positionSeconds": "positionMs"})
}
func (BookResume) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[bookResumeWire](), map[string]string{"positionSeconds": "positionMs"})
}

type bookChapterWire BookChapter

func (v BookChapter) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(bookChapterWire(v), map[string]string{"startSeconds": "startMs", "endSeconds": "endMs"})
}
func (BookChapter) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[bookChapterWire](), map[string]string{"startSeconds": "startMs", "endSeconds": "endMs"})
}

type playHistoryEntryWire PlayHistoryEntry

func (v PlayHistoryEntry) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(playHistoryEntryWire(v), map[string]string{"positionSeconds": "positionMs", "durationSeconds": "durationMs"})
}
func (PlayHistoryEntry) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[playHistoryEntryWire](), map[string]string{"positionSeconds": "positionMs", "durationSeconds": "durationMs"})
}

type historyEntryWire HistoryEntry

func (v HistoryEntry) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(historyEntryWire(v), map[string]string{"positionSeconds": "positionMs"})
}
func (HistoryEntry) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[historyEntryWire](), map[string]string{"positionSeconds": "positionMs"})
}

type playSummaryWire PlaySummary

func (v PlaySummary) MarshalJSON() ([]byte, error) {
	return apiwire.MarshalDurations(playSummaryWire(v), map[string]string{"watchedSeconds": "watchedMs"})
}
func (PlaySummary) JSONSchema() map[string]any {
	return apiwire.DurationSchema(reflect.TypeFor[playSummaryWire](), map[string]string{"watchedSeconds": "watchedMs"})
}
