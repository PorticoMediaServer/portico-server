package persistence

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 0093: the statements the sign-out and account-removal triggers run
// search their tables by index instead of scanning them.
func TestTriggerJoinsSearchInsteadOfScan(t *testing.T) {
	db, err := OpenFresh(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for statement, table := range map[string]string{
		`UPDATE playback_sessions SET state='stopped' WHERE session_hash IN(SELECT token_hash FROM authorization_family_tokens WHERE family_id='f')`: "playback_sessions",
		`DELETE FROM topshelf_tokens WHERE authority='local' AND account_id='a'`:                                                                     "topshelf_tokens",
		`DELETE FROM identity_devices WHERE authority='local' AND account_id='a'`:                                                                    "direct_profile_pin_attempts",
		`DELETE FROM identity_device_families WHERE device_id IN (SELECT id FROM identity_devices WHERE authority='local' AND account_id='a')`:       "identity_devices",
	} {
		rows, err := db.Query(`EXPLAIN QUERY PLAN ` + statement)
		if err != nil {
			t.Fatal(err)
		}
		plan := []string{}
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		rows.Close()
		joined := strings.Join(plan, "\n")
		if strings.Contains(joined, "SCAN "+table) || !strings.Contains(joined, "SEARCH "+table) {
			t.Errorf("%s is not searched by index:\n%s", table, joined)
		}
	}
}
