package ingestion

import (
	"database/sql"
	"testing"

	"portico.local/server/internal/catalogtest"
)

// drainDerivedCatalogue runs queued derived-data work after a scan writes facts.
func drainDerivedCatalogue(t *testing.T, db *sql.DB) {
	t.Helper()
	catalogtest.New(t, db).Drain()
}
