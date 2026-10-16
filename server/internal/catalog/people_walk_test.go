package catalog

import (
	"context"
	"fmt"
	"testing"

	"portico.local/server/internal/catalogtest"
)

// A broad name search (more than peopleRankedMaximum matches) pages people in
// name order, those whose name starts with the typed text first, with counts
// read from the maintained per-person credit counts; its total is a lower
// bound.
func TestBroadPeopleSearchWalksNamesInOrder(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("m", "Movies", "movie", "/m")
	film := c.Movie(lib, "/m/a.mkv", "Film", 2000)
	c.Drain()
	c.Exec(`INSERT INTO catalog_credit_labels(label) VALUES('Actor'),('Acting') ON CONFLICT DO NOTHING`)
	c.Exec(`WITH RECURSIVE n(i) AS(SELECT 0 UNION ALL SELECT i+1 FROM n WHERE i<1199)
	 INSERT INTO catalog_people(token,identity_key,name,sort_name) SELECT printf('tok-%04d',i),printf('fixture:%d',i),CASE WHEN i%2=0 THEN printf('Zed Person %04d',i) ELSE printf('Person %04d',i) END,CASE WHEN i%2=0 THEN printf('zed person %04d',i) ELSE printf('person %04d',i) END FROM n`)
	c.Exec(`INSERT INTO catalog_credits(entity_id,ord,person_id,role_id,department_id,credited_name,provider,credit_id,source_ordinal)
	 SELECT ?,p.id,p.id,(SELECT id FROM catalog_credit_labels WHERE label='Actor'),(SELECT id FROM catalog_credit_labels WHERE label='Acting'),p.name,'fixture',p.token,0 FROM catalog_people p`, film.ID)
	s := New(c.DB)
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"m"}}
	page, err := s.People(viewer, "", "Person", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.People) != 20 {
		t.Fatalf("%d people", len(page.People))
	}
	for i, person := range page.People {
		if want := fmt.Sprintf("Person %04d", 2*i+1); person.Name != want || person.CreditCount != 1 || len(person.Roles) != 1 {
			t.Fatalf("%d: %+v, want %s with one credit", i, person, want)
		}
	}
	out, err := s.Search(context.Background(), SearchRequest{Viewer: viewer, Q: "Person", Group: "people", Limit: 12})
	if err != nil || len(out.Groups) != 1 || len(out.Groups[0].Items) != 12 || out.Groups[0].TotalCountExact || !out.Groups[0].HasMore {
		t.Fatalf("search people: %+v %v", out.Groups, err)
	}
	next, err := s.Search(context.Background(), SearchRequest{Viewer: viewer, Q: "Person", Group: "people", Limit: 12, Cursor: out.Groups[0].NextCursor})
	if err != nil || len(next.Groups[0].Items) != 12 || next.Groups[0].Items[0].Title != "Person 0025" {
		t.Fatalf("next people: %+v %v", next.Groups, err)
	}
}
