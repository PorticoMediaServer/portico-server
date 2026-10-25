package catalog

import (
	"errors"
	"reflect"
	"testing"
)

func TestShowWorkspaceEpisodeDirectionPagesAndBindsCursor(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		t.Run(map[bool]string{false: "season", true: "group"}[grouped], func(t *testing.T) {
			s, db, names := workspaceFixture(t)
			r := workspaceRequest(names)
			r.SelectedSeasonID = names["season1"].Public
			if grouped {
				if _, err := db.Exec(`INSERT INTO catalog_show_settings(entity_id,hide_seasons,revision) VALUES(?,1,1)`, names["show"].ID); err != nil {
					t.Fatal(err)
				}
				r.SelectedSeasonID = ""
				r.Group = "all"
			}
			collect := func(direction string) []string {
				t.Helper()
				q := r
				q.EpisodeDirection = direction
				var ids []string
				for {
					w, err := s.ShowWorkspace(q)
					if err != nil {
						t.Fatal(err)
					}
					if w.Episodes.Query.Direction != direction || w.Episodes.Query.Sort != "episode" {
						t.Fatalf("actual order missing: %+v", w.Episodes.Query)
					}
					section := w.Episodes.Sections[0]
					for _, entry := range section.Entries {
						ids = append(ids, entry.ID)
					}
					if section.NextCursor == "" {
						break
					}
					q.Cursor = section.NextCursor
				}
				return ids
			}
			asc, desc := collect("asc"), collect("desc")
			for i, j := 0, len(desc)-1; i < j; i, j = i+1, j-1 {
				desc[i], desc[j] = desc[j], desc[i]
			}
			if len(asc) < 200 || !reflect.DeepEqual(asc, desc) {
				t.Fatalf("orders do not cover the same whole source: %d/%d", len(asc), len(desc))
			}
			first, err := s.ShowWorkspace(r)
			if err != nil {
				t.Fatal(err)
			}
			r.Cursor = first.Episodes.Sections[0].NextCursor
			for _, mutate := range []func(*ShowWorkspaceRequest){func(q *ShowWorkspaceRequest) { q.EpisodeDirection = "desc" }, func(q *ShowWorkspaceRequest) { q.PlayState = "unplayed" }, func(q *ShowWorkspaceRequest) { q.Viewer.Fence = "another"; q.ViewerFence = "another" }} {
				changed := r
				mutate(&changed)
				if _, err = s.ShowWorkspace(changed); !errors.Is(err, ErrCursor) {
					t.Fatalf("cursor accepted changed query: %v", err)
				}
			}
			r.Cursor = ""
			r.EpisodeDirection = "sideways"
			if _, err = s.ShowWorkspace(r); !errors.Is(err, ErrShowContext) {
				t.Fatalf("invalid order accepted: %v", err)
			}
		})
	}
}
