package operations

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

func TestSettingsReaderRejectsMissingCanonicalColumn(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		// Damage only this isolated canonical database. No replacement schema:
		// an incompatible reader must report the real missing column, rather
		// than interpreting its quoted name as a stored string or a default.
		if _, err := tx.Exec(`ALTER TABLE server_settings RENAME COLUMN "analysisStorageGiB" TO "missing_analysisStorageGiB"`); err != nil {
			return err
		}
		current := DefaultSettings()
		before := DefaultSettings()
		var readErr error
		var panicked any
		func() {
			defer func() { panicked = recover() }()
			readErr = readStoredSettings(tx, &current)
		}()
		if panicked != nil {
			return fmt.Errorf("missing canonical column panicked instead of returning its schema error: %v", panicked)
		}
		if readErr == nil || !strings.Contains(readErr.Error(), "analysisStorageGiB") {
			return fmt.Errorf("missing required column was not reported: %v", readErr)
		}
		if !reflect.DeepEqual(current, before) {
			return fmt.Errorf("failed schema read partially changed the settings projection")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
