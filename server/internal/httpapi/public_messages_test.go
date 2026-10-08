package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portico.local/server/internal/access"
	"portico.local/server/internal/administration"
	"portico.local/server/internal/downloads"
	"portico.local/server/internal/livechannels"
	librarychannels "portico.local/server/internal/livechannels/library"
	"portico.local/server/internal/lyrics"
	"portico.local/server/internal/mediaanalysis"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/playback"
)

func TestWrappedDomainErrorsNeverExposeCauses(t *testing.T) {
	cases := []struct {
		name  string
		write func(http.ResponseWriter, error)
		err   error
		code  string
	}{
		{"access", accessFailure, access.ErrConflict, "administration_conflict"},
		{"administration", administrationFailure, administration.ErrInput, "invalid_administration_input"},
		{"activity", activityFailure, playback.ErrActivityConflict, "playback_command_conflict"},
		{"console", consoleError, operations.ErrConflict, "console_conflict"},
		{"download", downloadFailure, downloads.ErrConflict, "download_conflict"},
		{"library", libraryFailure, librarychannels.ErrConflict, "library_channel_changed"},
		{"live", liveFailure, livechannels.ErrCapacity, "source_capacity_unavailable"},
		{"lyrics", lyricFailure, lyrics.ErrInput, "invalid_lyrics"},
		{"analysis", analysisFailure, mediaanalysis.ErrInput, "invalid_analysis"},
		{"storage", storageFailure, mounts.ErrCommandConflict, "storage_command_conflict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.write(w, fmt.Errorf("private-database-path-and-token: %w", tc.err))
			if w.Code < 400 || strings.Contains(w.Body.String(), "private-database-path-and-token") || !strings.Contains(w.Body.String(), publicErrorMessage(tc.code)) {
				t.Fatal(w.Code, w.Body)
			}
		})
	}
}
