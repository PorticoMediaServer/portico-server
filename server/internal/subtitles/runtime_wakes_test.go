package subtitles

import (
	"portico.local/server/internal/dbwork/waketest"
	"testing"
)

func TestRuntimeCollectionWakesOnlyForRealInputs(t *testing.T) {
	waketest.Inputs(t, watchSubtitleCollection, "subtitle_provider_candidates", "subtitle_resources", "subtitle_revisions", "subtitle_original_files", "playback_subtitle_pins", "playback_sessions", "prepared_media_session_pins")
}
