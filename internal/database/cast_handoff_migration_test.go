package database

import (
	"path/filepath"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestReleaseBaselineIncludesCastHandoffSourceColumns(t *testing.T) {
	t.Chdir("../..")
	appData := t.TempDir()
	db, err := Open(config.Config{AppDataDir: appData, DatabasePath: filepath.Join(appData, "portico.db")})
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer db.Close()
	for _, column := range []string{"source_playback_session_id"} {
		for _, table := range []string{"cast_bootstraps", "cast_receiver_sessions"} {
			if !databaseColumnExistsForTest(t, db, table, column) {
				t.Fatalf("041 cast handoff migration omitted %s.%s", table, column)
			}
		}
	}
}
