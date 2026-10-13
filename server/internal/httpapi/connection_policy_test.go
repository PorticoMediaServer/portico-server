package httpapi

import (
	"portico.local/server/internal/medialimits"
	"portico.local/server/internal/operations"
	"testing"
)

func TestOlderSettingsReceiptCannotLowerRunningSubtitleLimit(t *testing.T) {
	defer medialimits.ConfigureSubtitles(medialimits.DefaultSubtitleTracks, medialimits.DefaultSubtitleFileMiB)
	cache := &connectionPolicyCache{}
	current := operations.DefaultSettings()
	current.SubtitleTrackLimit = 256
	cache.set(operations.SettingsDocument{Revision: 3, Effective: current})
	old := current
	old.SubtitleTrackLimit = 32
	cache.set(operations.SettingsDocument{Revision: 2, Effective: old})
	if medialimits.SubtitleTracks() != 256 || cache.setting.Load().revision != 3 {
		t.Fatal("older receipt replaced current runtime settings")
	}
}
