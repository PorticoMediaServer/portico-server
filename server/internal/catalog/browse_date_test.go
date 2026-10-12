package catalog

import (
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
)

// A date operand names a whole UTC day, and an RFC 3339 operand the second it
// falls in, whatever its offset: stored dates are UTC text with any fraction.
func TestBrowseDateFiltersCompareDaysAndInstants(t *testing.T) {
	db, s, names := browseFixture(t)
	c := catalogtest.New(t, db)
	c.Fields(names["m1"].ID, map[string]any{"added_text": "2026-09-01T14:23:11.123Z"})
	c.Fields(names["m2"].ID, map[string]any{"added_text": "2026-09-02T23:59:59.999Z"})
	c.Fields(names["m3"].ID, map[string]any{"added_text": "2026-09-03T00:00:00Z"})
	c.Attributes(names["m2"].ID, "releaseDate", "2005-06-01")
	c.Exec(`UPDATE personal_items SET last_played_at='2026-09-10T18:30:00Z' WHERE profile_id='p' AND item_id=?`, names["m2"].ID)
	c.Drain()
	cases := []struct {
		name, query string
		want        []string
	}{
		{"added on a day", `{"field":"dateAdded","operator":"equals","value":"2026-09-01"}`, []string{"m1"}},
		{"added on another day", `{"field":"dateAdded","operator":"not-equals","value":"2026-09-01"}`, []string{"m2", "m3", "m4", "m5"}},
		{"added before a day", `{"field":"dateAdded","operator":"less-than","value":"2026-09-02"}`, []string{"m1"}},
		{"added by the end of a day", `{"field":"dateAdded","operator":"at-most","value":"2026-09-02"}`, []string{"m1", "m2"}},
		{"added after a day", `{"field":"dateAdded","operator":"greater-than","value":"2026-09-02"}`, []string{"m3", "m4", "m5"}},
		{"added from a day", `{"field":"dateAdded","operator":"at-least","value":"2026-09-02"}`, []string{"m2", "m3", "m4", "m5"}},
		{"added between days", `{"field":"dateAdded","operator":"between","value":["2026-09-01","2026-09-02"]}`, []string{"m1", "m2"}},
		{"added at an instant", `{"field":"dateAdded","operator":"equals","value":"2026-09-01T14:23:11Z"}`, []string{"m1"}},
		{"added after an instant", `{"field":"dateAdded","operator":"greater-than","value":"2026-09-01T14:23:11Z"}`, []string{"m2", "m3", "m4", "m5"}},
		{"added from an instant in another zone", `{"field":"dateAdded","operator":"at-least","value":"2026-09-01T16:23:11+02:00"}`, []string{"m1", "m2", "m3", "m4", "m5"}},
		{"added up to an instant in another zone", `{"field":"dateAdded","operator":"at-most","value":"2026-09-03T02:00:00+02:00"}`, []string{"m1", "m2", "m3"}},
		{"added between instants", `{"field":"dateAdded","operator":"between","value":["2026-09-02T23:59:59Z","2026-09-03T00:00:00Z"]}`, []string{"m2", "m3"}},
		{"played on a day", `{"field":"lastPlayedAt","operator":"equals","value":"2026-09-10"}`, []string{"m2"}},
		{"played by the end of a day", `{"field":"lastPlayedAt","operator":"at-most","value":"2026-09-10"}`, []string{"m2"}},
		{"not played on a day", `{"field":"lastPlayedAt","operator":"not-equals","value":"2026-09-10"}`, []string{"m1", "m3", "m4", "m5"}},
		{"released on a day", `{"field":"releaseDate","operator":"equals","value":"2005-06-01"}`, []string{"m2"}},
		{"released between days", `{"field":"releaseDate","operator":"between","value":["2005-01-01","2005-06-01"]}`, []string{"m2"}},
		{"released after a day", `{"field":"releaseDate","operator":"greater-than","value":"2005-06-01"}`, []string{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := browseIDs(t, s, names, test.query)
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("want %v got %v", test.want, got)
			}
		})
	}
}
