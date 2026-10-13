package catalog

import (
	"context"
	"portico.local/server/internal/dbwork"
	"reflect"
	"testing"
)

func TestQueueKeysMatchEveryBrowseOrder(t *testing.T) {
	db, s, _ := browseFixture(t)
	v := Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}
	for _, definition := range browseSorts {
		if !containsString(definition.ApplicableKinds, "movie") {
			continue
		}
		for _, direction := range definition.Directions {
			t.Run(definition.ID+"-"+direction, func(t *testing.T) {
				selection := []BrowseSortSelection{{Field: definition.ID, Direction: direction}}
				if definition.ID == "random" {
					selection[0].Seed = 2147483648
				}
				page, err := s.BrowseEntities(v, BrowseRequest{Library: "a", Pivot: "movies", Sort: selection, Limit: 100})
				if err != nil {
					t.Fatal(err)
				}
				tx, close, err := dbwork.BeginRead(context.Background(), db)
				if err != nil {
					t.Fatal(err)
				}
				defer close()
				plan, err := s.QueueKeysTx(context.Background(), tx, v, ResourceActor{}, &QueueQuery{LibraryID: "a", Pivot: "movies", Sort: selection}, "")
				if err != nil {
					t.Fatal(err)
				}
				rows, err := tx.QueryContext(context.Background(), plan.SQL, plan.Args...)
				if err != nil {
					t.Fatal(err)
				}
				defer rows.Close()
				got := []string{}
				for rows.Next() {
					var id string
					if err = rows.Scan(&id); err != nil {
						t.Fatal(err)
					}
					got = append(got, id)
				}
				if err = rows.Err(); err != nil {
					t.Fatal(err)
				}
				want := []string{}
				for _, entry := range page.Entries {
					want = append(want, entry.ID)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("queue order %v; browse %v", got, want)
				}
			})
		}
	}
}
