package httpapi

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
)

func TestTitleCursorSeeksSortIndex(t *testing.T) {
	f := administrationFixtureNew(t)
	c := catalogtest.New(t, f.deps.DB)
	library := c.Library("movies", "Movies", "movie", "/movies")
	for i, title := range []string{"Alpha", "Bravo", "Bravo", "Charlie", "Delta", "Echo"} {
		c.Movie(library, fmt.Sprintf("/movies/%d.mkv", i), title, 2000+i/2)
	}
	c.Drain()
	var seq int
	var name, path string
	if err := f.deps.DB.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	var statement string
	captured := sql.OpenDB(titleQueryConnector{driver: f.deps.DB.Driver(), path: path, statement: &statement})
	captured.SetMaxOpenConns(1)
	t.Cleanup(func() { captured.Close() })
	service := catalog.New(captured)
	viewer := catalog.Viewer{Profile: "owner-profile", Libraries: []string{"movies"}, Fence: "owner"}
	for _, sort := range []string{"title", "year", "added"} {
		for _, order := range []string{"asc", "desc"} {
			t.Run(sort+"/"+order, func(t *testing.T) {
				query := catalog.TitleQuery{Library: "movies", Kind: "movie", Sort: sort, Order: order, Limit: 2}
				seen := map[string]bool{}
				for {
					statement = ""
					page, err := service.QueryTitles(viewer, query)
					if err != nil {
						t.Fatal(err)
					}
					for _, item := range page.Items {
						if seen[item.ID] {
							t.Fatalf("cursor repeated title %s", item.ID)
						}
						seen[item.ID] = true
					}
					if query.Cursor != "" {
						found := false
						for _, sql := range []string{statement} {
							if !strings.Contains(sql, "FROM catalog_browse_rows_serving e JOIN catalog_entities ce") {
								continue
							}
							found = true
							args := make([]any, strings.Count(sql, "?"))
							rows, err := f.deps.DB.Query("EXPLAIN QUERY PLAN "+sql, args...)
							if err != nil {
								t.Fatal(err)
							}
							// The page is read from the serving rows: the current
							// generation and, during a rebuild, the one it replaces.
							// Each is entered at the cursor's sort key through its
							// order index, and the two are walked together in order,
							// so a page costs its own rows however large the library.
							seek, retained, sorted := false, false, false
							for rows.Next() {
								var id, parent, unused int
								var detail string
								if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
									rows.Close()
									t.Fatal(err)
								}
								t.Log(detail)
								ranged := strings.Contains(detail, "kind=? AND") && (strings.Contains(detail, ">?") || strings.Contains(detail, "<?"))
								if strings.Contains(detail, "SEARCH r USING INDEX catalog_browse_") && ranged {
									seek = true
								}
								if strings.Contains(detail, "SEARCH p USING INDEX catalog_browse_") && strings.Contains(detail, "_previous") && ranged {
									retained = true
								}
								if strings.Contains(detail, "USE TEMP B-TREE") || strings.HasPrefix(detail, "SCAN ") || strings.Contains(detail, "CO-ROUTINE") {
									sorted = true
								}
							}
							err = rows.Err()
							rows.Close()
							if err != nil {
								t.Fatal(err)
							}
							if !seek || !retained {
								t.Error("cursor query scans earlier library rows instead of seeking to its sort key")
							}
							if sorted {
								t.Error("cursor query builds and sorts every later row instead of reading its page in order")
							}
						}
						if !found {
							t.Fatal("title query was not traced")
						}
					}
					if page.NextCursor == "" {
						break
					}
					query.Cursor = page.NextCursor
				}
				if len(seen) != 6 {
					t.Fatalf("walk returned %d titles, want 6", len(seen))
				}
			})
		}
	}
}

// Capture the full statement at the SQL boundary. The general diagnostic trace
// intentionally truncates SQL, which cannot establish an index seek's plan.
type titleQueryConnector struct {
	driver    driver.Driver
	path      string
	statement *string
}

func (c titleQueryConnector) Driver() driver.Driver { return c.driver }
func (c titleQueryConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := c.driver.Open(c.path)
	if err != nil {
		return nil, err
	}
	return &titleQueryCapture{Conn: conn, statement: c.statement}, nil
}

type titleQueryCapture struct {
	driver.Conn
	statement *string
}

func (c *titleQueryCapture) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM catalog_browse_rows_serving e JOIN catalog_entities ce") {
		*c.statement = query
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}
