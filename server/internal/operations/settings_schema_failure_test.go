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
	for _, populated := range []bool{false, true} {
		t.Run(fmt.Sprintf("populated=%v", populated), func(t *testing.T) {
			db, err := persistence.Open(filepath.Join(t.TempDir(), "server.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if populated {
				if _, err := db.Exec(`INSERT INTO server_settings(singleton) VALUES(1)`); err != nil {
					t.Fatal(err)
				}
			}
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
		})
	}
}

func TestIntegrationReaderRejectsMissingCanonicalColumn(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		// Use the actual canonical table. Before the additive producer lands its
		// column is already absent; afterwards remove exactly that one column
		// in this isolated fixture to retain the same failure condition.
		var present bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM pragma_table_info('server_settings') WHERE name='webhookRetries')`).Scan(&present); err != nil {
			return err
		}
		if present {
			if _, err := tx.Exec(`ALTER TABLE server_settings RENAME COLUMN "webhookRetries" TO missing_webhookRetries`); err != nil {
				return err
			}
		}
		out, err := ReadIntegrationSettingsTx(context.Background(), tx)
		if err == nil || !strings.Contains(err.Error(), "webhookRetries") || !reflect.DeepEqual(out, IntegrationSettings{}) {
			return fmt.Errorf("missing integration policy was replaced with defaults: %+v, %v", out, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
