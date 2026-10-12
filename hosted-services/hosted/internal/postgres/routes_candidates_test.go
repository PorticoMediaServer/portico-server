package postgres_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/postgres"
)

// Fixture rows exercise the production projection without a PostgreSQL server.
// PostgreSQL query and signing coverage lives in TestSecureLANRouteDocument.
type routeRowsConnector struct{ rows [][]driver.Value }

func (c routeRowsConnector) Connect(context.Context) (driver.Conn, error) {
	return &routeRowsConn{c.rows}, nil
}
func (c routeRowsConnector) Driver() driver.Driver { return routeRowsDriver{} }

type routeRowsDriver struct{}

func (routeRowsDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use fixture connector")
}

type routeRowsConn struct{ rows [][]driver.Value }

func (*routeRowsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (*routeRowsConn) Close() error              { return nil }
func (*routeRowsConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (c *routeRowsConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &routeRows{rows: c.rows}, nil
}

type routeRows struct {
	rows  [][]driver.Value
	index int
}

func (*routeRows) Columns() []string {
	return []string{"id", "claim", "credential", "generation", "origin", "class", "candidate_generation", "quality", "namespace", "label"}
}
func (*routeRows) Close() error { return nil }
func (r *routeRows) Next(values []driver.Value) error {
	if r.index == len(r.rows) {
		return io.EOF
	}
	copy(values, r.rows[r.index])
	r.index++
	return nil
}
func TestRouteSetsDeduplicateNormalizedOrigins(t *testing.T) {
	const namespace = "ptc-aaaaaaaaaaaaaaaaaaaa"
	const label = "0123456789abcdef0123456789abcdef"
	const oldLabel = "fedcba9876543210fedcba9876543210"
	lan := "https://l-" + label + "." + namespace + ".direct.getportico.tv"
	public := "https://c-" + label + "." + namespace + ".direct.getportico.tv:32500"
	row := func(server, origin, class, quality string, generation int64) []driver.Value {
		return []driver.Value{server, int64(1), int64(1), int64(2), origin, class, generation, quality, namespace, label}
	}
	db := sql.OpenDB(routeRowsConnector{[][]driver.Value{
		row("first", "https://192.168.1.8:32500", "lan", "probe_required", 2),
		row("first", "https://192.168.1.9:32500", "lan", "probe_required", 2),
		row("first", "https://[fd00::8]:32500", "lan", "probe_required", 2),
		row("first", "https://192.168.1.8:32501", "lan", "probe_required", 2),
		row("first", "https://192.168.1.8", "lan", "probe_required", 2),
		row("first", "http://192.168.1.8:32500", "lan", "probe_required", 2),
		row("first", "https://c-"+oldLabel+"."+namespace+".direct.getportico.tv:32500", "public", "probe_required", 2),
		row("first", public, "public", "reachable", 2),
		row("first", "https://c-"+oldLabel+"."+namespace+".direct.getportico.tv:32500", "public", "probe_required", 1),
		row("second", "https://192.168.1.8:32500", "lan", "probe_required", 2),
	}})
	t.Cleanup(func() { db.Close() })
	sets, err := (&postgres.Store{DB: db}).RouteSets(context.Background(), "account")
	if err != nil {
		t.Fatal(err)
	}
	want := []directory.RouteCandidate{
		{BaseURL: lan + ":32500", Class: "lan", Quality: "probe_required", Generation: 2},
		{BaseURL: lan + ":32501", Class: "lan", Quality: "probe_required", Generation: 2},
		{BaseURL: lan, Class: "lan", Quality: "probe_required", Generation: 2},
		{BaseURL: "http://192.168.1.8:32500", Class: "lan", Quality: "probe_required", Generation: 2},
		{BaseURL: public, Class: "public", Quality: "reachable", Generation: 2},
	}
	got := sets["first"].Candidates
	if len(got) != len(want) {
		t.Fatalf("normalized candidates: %+v, want %+v", got, want)
	}
	for i, c := range want {
		if got[i] != c {
			t.Errorf("candidate %d = %+v, want %+v", i, got[i], c)
		}
	}
	if got := sets["second"].Candidates; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("another server lost its candidate: %+v", got)
	}
}
