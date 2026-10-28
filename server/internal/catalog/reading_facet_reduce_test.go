package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/bits"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

type fixtureFacetInput struct {
	members           []readingFacetMember
	rejected          map[int64]bool
	at, work, visited int
}

func (in *fixtureFacetInput) next(_ *Service, b *readingFacetBudget) (readingFacetMember, bool, bool, bool, error) {
	// Three nested history rows per fixture member; deliberately resumable even
	// when a quantum has room for less than one complete title.
	for b.available() {
		b.charge(32)
		in.visited++
		if in.at == len(in.members) {
			return readingFacetMember{}, false, false, true, nil
		}
		in.work++
		if in.work < 4 {
			continue
		}
		in.work = 0
		m := in.members[in.at]
		in.at++
		return m, !in.rejected[m.Item], true, false, nil
	}
	return readingFacetMember{}, false, false, false, nil
}
func (in *fixtureFacetInput) reset()     { in.at, in.work = 0, 0 }
func (in *fixtureFacetInput) bytes() int { return 64 } // fixture corpus is source data, not reducer state
func fixtureFacetMembers(labels []string, counts []int) []readingFacetMember {
	var rows []readingFacetMember
	for i, label := range labels {
		for j := 0; j < counts[i]; j++ {
			rows = append(rows, readingFacetMember{Key: compactcatalog.ReadingValueKey(label), Label: label, Source: "folder", Item: int64(len(rows) + 1)})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		if a.Label != b.Label {
			return a.Label < b.Label
		}
		return a.Item < b.Item
	})
	return rows
}
func runFixtureFacet(t *testing.T, request FacetRequest, in *fixtureFacetInput) *readingFacetReduction {
	t.Helper()
	r, err := newReadingFacetReduction(request, "full-witness", in, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for turns := 0; turns < 100000; turns++ {
		b := newReadingFacetBudget(7, 512, time.Second)
		before := in.visited
		done, err := r.step(nil, "full-witness", b)
		if err != nil {
			t.Fatal(err)
		}
		if in.visited-before > 7 {
			t.Fatal("raw work budget exceeded")
		}
		if len(r.page) > r.request.Limit {
			t.Fatal("unbounded page")
		}
		if done {
			return r
		}
	}
	t.Fatal("no progress")
	return nil
}
func TestReadingFacetReductionCountOffsetsAndRawProgress(t *testing.T) {
	labels := []string{"A", "a", "B", "C", "c", "D", "E", "F", "G", "H", "I", "J"}
	counts := []int{2, 3, 7, 1, 2, 6, 1, 4, 7, 2, 3, 5}
	rows := fixtureFacetMembers(labels, counts)
	// Expected title/count ordering from the old SQL oracle's value counts.
	want := []readingFacetChoice{{"a", "a", 5}, {"b", "B", 7}, {"c", "c", 3}, {"d", "D", 6}, {"e", "E", 1}, {"f", "F", 4}, {"g", "G", 7}, {"h", "H", 2}, {"i", "I", 3}, {"j", "J", 5}}
	for _, order := range []string{"title", "count"} {
		expected := append([]readingFacetChoice{}, want...)
		if order == "count" {
			sort.Slice(expected, func(i, j int) bool {
				if expected[i].count != expected[j].count {
					return expected[i].count > expected[j].count
				}
				return expected[i].key < expected[j].key
			})
		}
		for offset := 0; offset < 15; offset++ {
			for limit := 1; limit < 5; limit++ {
				r := runFixtureFacet(t, FacetRequest{Library: "reading", Field: "author", Sort: order, Offset: offset, Limit: limit}, &fixtureFacetInput{members: rows})
				page, ok := r.result()
				if !ok || page.Total != len(expected) {
					t.Fatal(page, ok)
				}
				lo := min(offset, len(expected))
				hi := min(lo+limit, len(expected))
				if len(page.Values) != hi-lo {
					t.Fatalf("%s offset %d: %+v", order, offset, page)
				}
				for i, v := range page.Values {
					e := expected[lo+i]
					if v.Value != e.label || v.Count != e.count {
						t.Fatalf("%s offset %d got %+v want %+v", order, offset, v, e)
					}
				}
				if r.passes > bits.Len(uint(len(rows)))+3 {
					t.Fatalf("offset-dependent passes: %d", r.passes)
				}
			}
		}
	}
}
func TestReadingFacetReductionSpellingUnicodeAndRejectedOwners(t *testing.T) {
	rows := fixtureFacetMembers([]string{"Café  Author", "CAFÉ AUTHOR", "café author", "Cafe Author", "Space\tName", "SPACE NAME"}, []int{2, 2, 8, 3, 1, 1})
	rejected := map[int64]bool{}
	for i := range rows {
		if rows[i].Label == "CAFÉ AUTHOR" {
			rows[i].Source = "owner"
		}
		if rows[i].Label == "Café  Author" {
			rows[i].Source = "owner"
			rejected[rows[i].Item] = true
		}
	}
	r := runFixtureFacet(t, FacetRequest{Library: "reading", Field: "series", Q: "É", Limit: 20}, &fixtureFacetInput{members: rows, rejected: rejected})
	page, _ := r.result()
	if page.Total != 1 || len(page.Values) != 1 || page.Values[0].Value != "CAFÉ AUTHOR" || page.Values[0].Count != 10 || string(page.Values[0].SeriesID) != compactcatalog.ReadingSeriesID("reading", "CAFÉ AUTHOR") {
		t.Fatal(page)
	}
	// Owner frequency wins; total automatic frequency cannot outvote an owner.
	for i := range rows {
		if rows[i].Label == "Café  Author" {
			delete(rejected, rows[i].Item)
		}
	}
	r = runFixtureFacet(t, FacetRequest{Library: "reading", Field: "author", Q: "é", Limit: 20}, &fixtureFacetInput{members: rows, rejected: rejected})
	page, _ = r.result()
	if page.Values[0].Value != "CAFÉ AUTHOR" || page.Values[0].Count != 12 {
		t.Fatal(page)
	} // owner tie: binary capitals first
}
func TestReadingFacetReductionWitnessAndLosslessScalar(t *testing.T) {
	rows := fixtureFacetMembers([]string{"Label"}, []int{2})
	r, err := newReadingFacetReduction(FacetRequest{Field: "author"}, "w", &fixtureFacetInput{members: rows}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.result(); ok {
		t.Fatal("partial exposed")
	}
	if _, err = r.step(nil, "changed", newReadingFacetBudget(4, 4096, time.Second)); !errors.Is(err, ErrReadingProjectionChanged) {
		t.Fatal(err)
	}
	if _, ok := r.result(); ok {
		t.Fatal("stale result exposed")
	}
	b := newReadingFacetBudget(4096, 1024, time.Second)
	b.charge(100000)
	if b.Bytes != -98976 || b.available() || !b.oversized {
		t.Fatal("scalar truncated or not charged", b)
	}
	huge := strings.Repeat("é", 20000)
	in := &fixtureFacetInput{members: fixtureFacetMembers([]string{huge}, []int{1})}
	r, err = newReadingFacetReduction(FacetRequest{Field: "author", Limit: 1}, "w", in, 4096)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		done, e := r.step(nil, "w", newReadingFacetBudget(4, 4096, time.Second))
		if e != nil {
			t.Fatal(e)
		}
		if done {
			break
		}
	}
	page, ok := r.result()
	if !ok || page.Values[0].Value != huge || !r.uncacheable || r.bytes() < len(huge) {
		t.Fatal("oversize scalar lost or cacheable")
	}
}

func TestReadingFacetSQLInputRequiresProductionOrderedIndex(t *testing.T) {
	c, s, viewer, _ := readingBrowseFixture(t, "book")
	c.Drain()
	err := dbwork.WithReadSnapshot(context.Background(), c.DB, func(ctx context.Context) error {
		_, err := s.WithContext(ctx).newReadingFacetSQLInput(viewer, FacetRequest{Library: "reading", Field: "author"}, time.Now())
		return err
	})
	if err != nil && !errors.Is(err, errReadingFacetIndex) {
		t.Fatal(err)
	}
}

func runSQLFacet(t *testing.T, s *Service, viewer Viewer, request FacetRequest) (FacetPage, *readingFacetReduction) {
	t.Helper()
	var in *readingFacetSQLInput
	err := dbwork.WithReadSnapshot(context.Background(), s.db, func(ctx context.Context) error {
		var err error
		in, err = s.WithContext(ctx).newReadingFacetSQLInput(viewer, request, time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC))
		return err
	})
	if errors.Is(err, errReadingFacetIndex) {
		t.Skip("production ordered facet baseline index pending parent slot; no fixture-only index")
	}
	if err != nil {
		t.Fatal(err)
	}
	r, err := newReadingFacetReduction(request, "captured-test-witness", in, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for turns := 0; turns < 10000; turns++ {
		done := false
		err = dbwork.WithReadSnapshot(context.Background(), s.db, func(ctx context.Context) error {
			var err error
			b := newReadingFacetBudget(2, 4096, time.Second)
			done, err = r.step(s.WithContext(ctx), "captured-test-witness", b)
			if b.Raw < 0 {
				t.Fatal("nested raw work exceeded")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if done {
			page, ok := r.result()
			if !ok {
				t.Fatal("result absent")
			}
			return page, r
		}
	}
	t.Fatal("SQL source made no progress")
	return FacetPage{}, nil
}
func TestReadingFacetSQLInputOldOracleParity(t *testing.T) {
	for _, kind := range []string{"book", "comic"} {
		t.Run(kind, func(t *testing.T) {
			c, s, viewer, items := readingBrowseFixture(t, kind)
			c.Exec(`INSERT INTO catalog_item_details(entity_id,added_text) VALUES(?,'2026-10-27T12:00:00Z') ON CONFLICT(entity_id) DO UPDATE SET added_text=excluded.added_text`, items[0].ID)
			// An unsupported version precedes a valid publication; unavailable remains
			// eligible. Retiring one title must reject its raw attribution member.
			c.Write(func(ctx context.Context, tx *sql.Tx) error {
				asset, _, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: fmt.Sprintf("/extra-%s.epub", kind), Container: "epub"})
				if err != nil {
					return err
				}
				if err = compactcatalog.LinkAssetTx(ctx, tx, items[0].ID, asset, compactcatalog.Link{}); err != nil {
					return err
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO reading_publications(item_id,asset_id,format) VALUES(?,?,'epub')`, items[0].ID, asset)
				return err
			})
			c.Drain()
			queries := []*BrowseNode{nil, {Field: "author", Operator: "contains", Value: "café author"}, {Not: &BrowseNode{Field: "series", Operator: "equals", Value: "missing"}}, {All: []BrowseNode{{Field: "title", Operator: "contains", Value: "title"}, {Field: "year", Operator: "is-missing"}}}, {Field: "dateAdded", Operator: "within-last-days", Value: float64(2)}}
			s.Clock = func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }
			for _, field := range []string{"author", "series"} {
				for _, order := range []string{"title", "count"} {
					for _, query := range queries {
						for _, needle := range []string{"", "é", "a"} {
							for offset := 0; offset < 3; offset++ {
								request := FacetRequest{Library: "reading", Field: field, Sort: order, Query: query, Q: needle, Offset: offset, Limit: 1}
								want, err := s.Facets(viewer, request)
								if err != nil {
									t.Fatal(err)
								}
								got, _ := runSQLFacet(t, s, viewer, request)
								got.Revision = want.Revision
								if !reflect.DeepEqual(got, want) {
									t.Fatalf("%s %s %q offset %d query %+v\ngot %+v\nwant %+v", field, order, needle, offset, query, got, want)
								}
							}
						}
					}
				}
			}
			age := 10
			viewer.Restrictions = identity.ContentRestrictions{MaximumAge: &age, BlockUnrated: true}
			// Current membership/attribute semantics, including no-membership admits.
			request := FacetRequest{Library: "reading", Field: "author", Limit: 20}
			want, err := s.Facets(viewer, request)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := runSQLFacet(t, s, viewer, request)
			got.Revision = want.Revision
			if !reflect.DeepEqual(got, want) {
				t.Fatal(got, want)
			}
		})
	}
}

func TestReadingFacetScalarCompilerOldOracle(t *testing.T) {
	c, s, _, items := readingBrowseFixture(t, "book")
	c.Drain()
	nodes := []BrowseNode{}
	for _, field := range []string{"author", "series"} {
		for _, op := range []string{"equals", "contains", "not-equals", "starts-with"} {
			nodes = append(nodes, BrowseNode{Field: field, Operator: op, Value: "café author"})
		}
		for _, op := range []string{"in", "not-in", "contains-any", "contains-all"} {
			nodes = append(nodes, BrowseNode{Field: field, Operator: op, Value: []any{"café author", "CAFÉ\tAUTHOR"}})
		}
		for _, op := range []string{"is-present", "is-missing"} {
			nodes = append(nodes, BrowseNode{Field: field, Operator: op})
		}
	}
	nodes = append(nodes, BrowseNode{Field: "title", Operator: "contains", Value: "title"}, BrowseNode{Field: "year", Operator: "is-missing"}, BrowseNode{Field: "dateAdded", Operator: "not-equals", Value: "2026-10-27"}, BrowseNode{Not: &BrowseNode{Any: []BrowseNode{{Field: "author", Operator: "is-missing"}, {Field: "title", Operator: "starts-with", Value: "Title 1"}}}})
	err := dbwork.WithReadSnapshot(context.Background(), c.DB, func(ctx context.Context) error {
		ss := s.WithContext(ctx)
		scope, err := ss.readingBrowseScope("reading")
		if err != nil {
			return err
		}
		for _, node := range nodes {
			now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
			old := &browseCompiler{reading: scope, now: now}
			oracle, err := old.compile(&node)
			if err != nil {
				return err
			}
			direct := &browseCompiler{reading: scope, now: now}
			leaves := 0
			scalar, err := compileReadingFacetNode(direct, &node, 0, &leaves)
			if err != nil {
				return err
			}
			for _, item := range items {
				var want, got bool
				err = ss.read().QueryRow(`SELECT COALESCE((`+oracle+`),0) FROM catalog_browse_rows e WHERE e.entity_id=?`, append(append([]any{}, old.args...), item.ID)...).Scan(&want)
				if err != nil {
					return err
				}
				err = ss.read().QueryRow(`SELECT COALESCE((`+scalar+`),0) FROM catalog_browse_rows e JOIN reading_attribution_members cm ON cm.item_id=e.entity_id AND cm.library_id=e.library_id AND cm.generation=? WHERE e.entity_id=?`, append(append([]any{}, direct.args...), scope.generation.Ready, item.ID)...).Scan(&got)
				if err != nil {
					return err
				}
				if got != want {
					t.Fatalf("node %+v item %d direct %v old %v", node, item.ID, got, want)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadingFacetNestedMembershipAttributesResume(t *testing.T) {
	c, s, _, items := readingBrowseFixture(t, "book")
	values := make([]string, 31)
	for i := range values {
		values[i] = fmt.Sprintf("label-%02d", i)
	}
	c.Attributes(items[0].ID, "label", values...)
	c.Attributes(items[0].ID, "contentRating", "PG")
	c.Drain()
	age := 12
	policies := []identity.ContentRestrictions{{MaximumAge: &age, BlockUnrated: true}, {BlockedLabels: []string{"label-30"}}, {MemberDeniedLabels: []string{"LABEL-30"}}, {BlockUnrated: true}, {}}
	for _, policy := range policies {
		var in *readingFacetSQLInput
		want := false
		err := dbwork.WithReadSnapshot(context.Background(), c.DB, func(ctx context.Context) error {
			ss := s.WithContext(ctx)
			scope, err := ss.readingBrowseScope("reading")
			if err != nil {
				return err
			}
			clause, args := EntityRestrictionSQL("e.entity_id", policy)
			if err = ss.read().QueryRow(`SELECT `+clause+` FROM catalog_browse_rows e WHERE e.entity_id=?`, append(args, items[0].ID)...).Scan(&want); err != nil {
				return err
			}
			// Starts after raw-member admission: no fixture index, and this test makes
			// no claim about ordered member seeking until the production slot lands.
			in = &readingFacetSQLInput{scope: *scope, library: "reading", field: "author", clause: "1", restrictions: policy, current: readingFacetMember{Item: items[0].ID}, phase: "scalar"}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		finished := false
		turns := 0
		for ; turns < 1000; turns++ {
			err = dbwork.WithReadSnapshot(context.Background(), c.DB, func(ctx context.Context) error {
				b := newReadingFacetBudget(1, readingFacetByteLimit, time.Second)
				_, got, present, eof, err := in.next(s.WithContext(ctx), b)
				if b.Raw < 0 {
					t.Fatal("nested cursor exceeded single raw unit")
				}
				if eof {
					t.Fatal("member disappeared")
				}
				if present {
					finished = true
					if got != want {
						t.Fatalf("policy %+v got %v want %v", policy, got, want)
					}
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if finished {
				break
			}
		}
		if !finished {
			t.Fatal("nested cursor failed to progress")
		}
		if policy.Active() && turns < 10 {
			t.Fatal("attribute history hidden as one raw unit", turns)
		}
	}
}
