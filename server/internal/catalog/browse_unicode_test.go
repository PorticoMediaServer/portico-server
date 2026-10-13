package catalog

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
)

func TestBrowseTextPredicatesFoldUnicodeAndAccents(t *testing.T) {
	db, s, names := browseFixture(t)
	c := catalogtest.New(t, db)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetFieldsTx(ctx, tx, names["m1"].ID, compactcatalog.Automatic, map[string]any{"title": "Amélie 100%_"})
	})
	c.Genres(names["m1"].ID, "tmdb", "Comédie")
	c.Drain()
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{`{"field":"title","operator":"contains","value":"amelie"}`, []string{"m1"}},
		{`{"field":"title","operator":"starts-with","value":"AMÉLIE"}`, []string{"m1"}},
		{`{"field":"title","operator":"equals","value":"AMELIE 100%_"}`, []string{"m1"}},
		{`{"field":"genre","operator":"contains-any","value":["COMÉDIE"]}`, []string{"m1"}},
		{`{"field":"genre","operator":"contains-all","value":["comedie"]}`, []string{"m1"}},
		{`{"field":"title","operator":"contains","value":"%_"}`, []string{"m1"}},
		{`{"field":"title","operator":"contains","value":"_100"}`, []string{}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			got := browseIDs(t, s, names, tc.query)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("want %v got %v", tc.want, got)
			}
		})
	}
}
