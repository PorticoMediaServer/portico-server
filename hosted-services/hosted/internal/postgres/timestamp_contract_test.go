package postgres_test

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Run with TZ=Europe/Berlin as well as UTC: pgx's default decoder follows the
// process timezone even when the database's own TimeZone setting is UTC.
func TestStoreTimestampsUseUTCContract(t *testing.T) {
	store := database(t)
	var instant time.Time
	var nullable sql.NullTime
	// The ordinary pooled path uses the binary codec; casting the timestamp
	// also checks that an explicit non-UTC SQL input keeps the same instant.
	if err := store.DB.QueryRow(`SELECT '2026-09-18T17:45:20.251724+02:00'::timestamptz, '2026-09-18T17:45:20.251724+02:00'::timestamptz`).Scan(&instant, &nullable); err != nil {
		t.Fatal(err)
	}
	for _, value := range []time.Time{instant, nullable.Time} {
		encoded, err := json.Marshal(value)
		if err != nil || value.Location() != time.UTC || !strings.HasSuffix(string(encoded), "Z\"") || value.Hour() != 15 {
			t.Fatalf("account timestamp must preserve its instant and serialize in UTC: %s (%v)", encoded, err)
		}
	}
}
