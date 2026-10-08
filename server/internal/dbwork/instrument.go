package dbwork

import (
	"context"
	"database/sql/driver"
	"log"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Per-read instrumentation lives in the driver rather than in the helpers above
// it, for one reason: a statement issued through `s.db.Query` and a statement
// issued through dbwork.Query cost the database exactly the same, and a counter
// that only sees the well-behaved call sites would report the number we wish
// were true. The driver sees every statement, every transaction and every
// pooled-connection acquisition, and it sees them with the caller's context, so
// the cost of one request can be attributed to that request.
//
// The one honest limitation: `database/sql`'s context-free forms (`db.Query`,
// `db.QueryRow`) hand the driver `context.Background()`, so their statements
// reach the global counters but cannot be attributed to a request. Converting a
// read path to the context form is therefore also what makes it measurable.

// DriverName is the registered name of the observed SQLite driver. It wraps the
// pure-Go modernc driver; the underlying behaviour is unchanged.
const DriverName = "sqlite-observed"

// SlowReadThreshold is the duration above which a foreground read is logged.
// The old server logged at 250 ms and that number is still the right one: it is
// long enough that nothing healthy trips it and short enough to catch a query
// shape that has quietly become linear in library size.
const SlowReadThreshold = 250 * time.Millisecond

// Cost is what one unit of work asked of the database. Statements is the number
// the audit's gate is written against, because a latency budget can be met by a
// fast machine hiding a bad query shape and a statement count cannot be.
type Cost struct {
	Statements   int64 `json:"statements"`
	Transactions int64 `json:"transactions"`
	Acquisitions int64 `json:"acquisitions"`
	ReadMillis   int64 `json:"readMillis"`
}

type counters struct {
	statements   atomic.Int64
	transactions atomic.Int64
	acquisitions atomic.Int64
	readNanos    atomic.Int64
}

func (c *counters) cost() Cost {
	if c == nil {
		return Cost{}
	}
	return Cost{
		Statements:   c.statements.Load(),
		Transactions: c.transactions.Load(),
		Acquisitions: c.acquisitions.Load(),
		ReadMillis:   c.readNanos.Load() / int64(time.Millisecond),
	}
}

type costKey struct{}

// Measure attaches a fresh cost counter to ctx and returns a reader for it. The
// HTTP layer calls it once per request; a test calls it around one call to read
// exactly what that call cost.
func Measure(ctx context.Context) (context.Context, func() Cost) {
	if ctx == nil {
		ctx = context.Background()
	}
	c := &counters{}
	return context.WithValue(ctx, costKey{}, c), c.cost
}

// CostFrom reports what has been spent under ctx so far, or a zero Cost when ctx
// carries no counter.
func CostFrom(ctx context.Context) Cost {
	return costFrom(ctx).cost()
}

func costFrom(ctx context.Context) *counters {
	if ctx == nil {
		return nil
	}
	value, _ := ctx.Value(costKey{}).(*counters)
	return value
}

// --- global observation ----------------------------------------------------

var global struct {
	statements   atomic.Uint64
	transactions atomic.Uint64
	acquisitions atomic.Uint64
	slow         atomic.Uint64

	mu      sync.Mutex
	byClass map[Class]*classReads
}

type classReads struct {
	statements uint64
	nanos      uint64
	maxNanos   uint64
}

// ClassReadStats is one work class's share of the read load.
type ClassReadStats struct {
	Class      string `json:"class"`
	Statements uint64 `json:"statements"`
	TotalMs    uint64 `json:"totalMillis"`
	MaxMs      uint64 `json:"maxMillis"`
}

// ReadStats is the process-wide statement picture. It is the counterpart to the
// pool and gate statistics: those say how long callers waited, this says how
// much work they asked for.
type ReadStats struct {
	Statements   uint64           `json:"statements"`
	Transactions uint64           `json:"transactions"`
	Acquisitions uint64           `json:"acquisitions"`
	SlowReads    uint64           `json:"slowReads"`
	ByClass      []ClassReadStats `json:"byClass"`
}

// Reads snapshots the process-wide statement counters.
func Reads() ReadStats {
	out := ReadStats{
		Statements:   global.statements.Load(),
		Transactions: global.transactions.Load(),
		Acquisitions: global.acquisitions.Load(),
		SlowReads:    global.slow.Load(),
		ByClass:      []ClassReadStats{},
	}
	global.mu.Lock()
	defer global.mu.Unlock()
	for _, class := range Classes() {
		entry := global.byClass[class]
		if entry == nil {
			continue
		}
		out.ByClass = append(out.ByClass, ClassReadStats{
			Class:      class.String(),
			Statements: entry.statements,
			TotalMs:    entry.nanos / uint64(time.Millisecond),
			MaxMs:      entry.maxNanos / uint64(time.Millisecond),
		})
	}
	return out
}

// ResetReads zeroes the process-wide counters. It exists for tests and for the
// load harness, which measures one run rather than the life of the process.
func ResetReads() {
	global.statements.Store(0)
	global.transactions.Store(0)
	global.acquisitions.Store(0)
	global.slow.Store(0)
	global.mu.Lock()
	global.byClass = nil
	global.mu.Unlock()
}

type traceKey struct{}

// traceBox collects statement text for one traced context.
type traceBox struct {
	mu    sync.Mutex
	lines []string
}

// TraceStatements collects the statements issued under ctx and returns a reader
// for them.
//
// It is opt-in per context and off everywhere else, which is what keeps it
// compatible with the rule that metrics never carry query text: nothing here
// reaches a metric, a log or the diagnostics endpoint. It exists because a
// statement count tells you a route is expensive and only the shape tells you
// which statement is repeated.
func TraceStatements(ctx context.Context) (context.Context, func() []string) {
	box := &traceBox{}
	return context.WithValue(ctx, traceKey{}, box), func() []string {
		box.mu.Lock()
		defer box.mu.Unlock()
		return append([]string{}, box.lines...)
	}
}

func traceStatement(ctx context.Context, query string) {
	if ctx == nil {
		return
	}
	box, _ := ctx.Value(traceKey{}).(*traceBox)
	if box == nil {
		return
	}
	q := strings.Join(strings.Fields(query), " ")
	if len(q) > 110 {
		q = q[:110]
	}
	box.mu.Lock()
	box.lines = append(box.lines, q)
	box.mu.Unlock()
}

func observe(ctx context.Context, query string, elapsed time.Duration, inTx bool) {
	profileStatement(query, elapsed)
	global.statements.Add(1)
	if !inTx {
		global.acquisitions.Add(1)
	}
	if c := costFrom(ctx); c != nil {
		c.statements.Add(1)
		c.readNanos.Add(int64(elapsed))
		if !inTx {
			c.acquisitions.Add(1)
		}
	}
	class := ClassFrom(ctx, ClassInteractive)
	global.mu.Lock()
	if global.byClass == nil {
		global.byClass = map[Class]*classReads{}
	}
	entry := global.byClass[class]
	if entry == nil {
		entry = &classReads{}
		global.byClass[class] = entry
	}
	entry.statements++
	entry.nanos += uint64(elapsed)
	if uint64(elapsed) > entry.maxNanos {
		entry.maxNanos = uint64(elapsed)
	}
	global.mu.Unlock()
	if elapsed >= SlowReadThreshold && !class.Background() {
		global.slow.Add(1)
		if slowReadLog.Load() {
			log.Printf("slow database read: %s on the %s class", elapsed.Round(time.Millisecond), class)
		}
	}
}

// slowReadLog keeps the slow-read line out of test output by default. The server
// turns it on at startup.
var slowReadLog atomic.Bool

// LogSlowReads turns the slow-read log line on or off.
func LogSlowReads(on bool) { slowReadLog.Store(on) }

func observeTransaction(ctx context.Context) {
	global.transactions.Add(1)
	global.acquisitions.Add(1)
	if c := costFrom(ctx); c != nil {
		c.transactions.Add(1)
		c.acquisitions.Add(1)
	}
}

// --- driver wrapper --------------------------------------------------------

type observedDriver struct{ inner driver.Driver }

func (d observedDriver) Open(name string) (driver.Conn, error) {
	inner, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &observedConn{inner: inner}, nil
}

type observedConn struct {
	inner   driver.Conn
	inTx    bool
	changes *changeSet
}

func (c *observedConn) Prepare(query string) (driver.Stmt, error) {
	inner, err := c.inner.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &observedStmt{inner: inner, conn: c, query: query}, nil
}

func (c *observedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	preparer, ok := c.inner.(driver.ConnPrepareContext)
	if !ok {
		return c.Prepare(query)
	}
	inner, err := preparer.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &observedStmt{inner: inner, conn: c, query: query}, nil
}

func (c *observedConn) Close() error { return c.inner.Close() }

// Unwrap exposes the driver connection underneath. `sql.Conn.Raw` hands out
// whatever connection the driver returned, and a wrapper that hid the real one
// would silently break the online-backup path, which reaches for a method only
// the concrete modernc connection has.
func (c *observedConn) Unwrap() driver.Conn { return c.inner }

func (c *observedConn) Begin() (driver.Tx, error) {
	inner, err := c.inner.Begin()
	if err != nil {
		return nil, err
	}
	observeTransaction(context.Background())
	c.inTx = true
	c.changes = nil
	return &observedTx{inner: inner, conn: c}, nil
}

func (c *observedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	beginner, ok := c.inner.(driver.ConnBeginTx)
	if !ok {
		return c.Begin()
	}
	inner, err := beginner.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	observeTransaction(ctx)
	c.inTx = true
	c.changes, _ = ctx.Value(changeKey{}).(*changeSet)
	return &observedTx{inner: inner, conn: c}, nil
}

func (c *observedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := c.inner.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	start := time.Now()
	traceStatement(ctx, query)
	out, err := execer.ExecContext(ctx, query, args)
	if err == nil {
		noticeAuthorityWrite(query)
		c.changes.note(query)
	}
	observe(ctx, query, time.Since(start), c.inTx)
	return out, err
}

func (c *observedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := c.inner.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	start := time.Now()
	traceStatement(ctx, query)
	out, err := queryer.QueryContext(ctx, query, args)
	if err != nil {
		observe(ctx, query, time.Since(start), c.inTx)
		return nil, err
	}
	c.changes.note(query)
	return &observedRows{inner: out, ctx: ctx, query: query, start: start, inTx: c.inTx}, nil
}

func (c *observedConn) Ping(ctx context.Context) error {
	if pinger, ok := c.inner.(driver.Pinger); ok {
		return pinger.Ping(ctx)
	}
	return nil
}

func (c *observedConn) ResetSession(ctx context.Context) error {
	c.inTx = false
	if resetter, ok := c.inner.(driver.SessionResetter); ok {
		return resetter.ResetSession(ctx)
	}
	return nil
}

func (c *observedConn) IsValid() bool {
	if validator, ok := c.inner.(driver.Validator); ok {
		return validator.IsValid()
	}
	return true
}

type observedTx struct {
	inner driver.Tx
	conn  *observedConn
}

func (t *observedTx) Commit() error {
	t.conn.inTx = false
	return t.inner.Commit()
}

func (t *observedTx) Rollback() error {
	t.conn.inTx = false
	return t.inner.Rollback()
}

type observedStmt struct {
	inner driver.Stmt
	conn  *observedConn
	query string
}

func (s *observedStmt) Close() error  { return s.inner.Close() }
func (s *observedStmt) NumInput() int { return s.inner.NumInput() }

func (s *observedStmt) Exec(args []driver.Value) (driver.Result, error) {
	start := time.Now()
	out, err := s.inner.Exec(args)
	if err == nil {
		s.conn.changes.note(s.query)
	}
	observe(context.Background(), "", time.Since(start), s.conn.inTx)
	return out, err
}

func (s *observedStmt) Query(args []driver.Value) (driver.Rows, error) {
	start := time.Now()
	out, err := s.inner.Query(args)
	if err != nil {
		observe(context.Background(), "", time.Since(start), s.conn.inTx)
		return nil, err
	}
	s.conn.changes.note(s.query)
	return &observedRows{inner: out, ctx: context.Background(), start: start, inTx: s.conn.inTx}, nil
}

func (s *observedStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := s.inner.(driver.StmtExecContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	start := time.Now()
	out, err := execer.ExecContext(ctx, args)
	if err == nil {
		s.conn.changes.note(s.query)
	}
	observe(ctx, "", time.Since(start), s.conn.inTx)
	return out, err
}

func (s *observedStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := s.inner.(driver.StmtQueryContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	start := time.Now()
	out, err := queryer.QueryContext(ctx, args)
	if err != nil {
		observe(ctx, "", time.Since(start), s.conn.inTx)
		return nil, err
	}
	s.conn.changes.note(s.query)
	return &observedRows{inner: out, ctx: ctx, start: start, inTx: s.conn.inTx}, nil
}

// observedRows times a read to the end of iteration rather than to the return of
// Query. A statement that streams its rows does most of its work in Next, so
// timing the call alone would report the expensive shapes as free.
type observedRows struct {
	inner    driver.Rows
	ctx      context.Context
	query    string
	start    time.Time
	inTx     bool
	observed bool
}

func (r *observedRows) Columns() []string { return r.inner.Columns() }

func (r *observedRows) Next(dest []driver.Value) error { return r.inner.Next(dest) }

func (r *observedRows) Close() error {
	if !r.observed {
		r.observed = true
		observe(r.ctx, r.query, time.Since(r.start), r.inTx)
	}
	return r.inner.Close()
}

func (r *observedRows) ColumnTypeDatabaseTypeName(index int) string {
	if inner, ok := r.inner.(driver.RowsColumnTypeDatabaseTypeName); ok {
		return inner.ColumnTypeDatabaseTypeName(index)
	}
	return ""
}

func (r *observedRows) ColumnTypeLength(index int) (int64, bool) {
	if inner, ok := r.inner.(driver.RowsColumnTypeLength); ok {
		return inner.ColumnTypeLength(index)
	}
	return 0, false
}

func (r *observedRows) ColumnTypeNullable(index int) (bool, bool) {
	if inner, ok := r.inner.(driver.RowsColumnTypeNullable); ok {
		return inner.ColumnTypeNullable(index)
	}
	return false, false
}

func (r *observedRows) ColumnTypePrecisionScale(index int) (int64, int64, bool) {
	if inner, ok := r.inner.(driver.RowsColumnTypePrecisionScale); ok {
		return inner.ColumnTypePrecisionScale(index)
	}
	return 0, 0, false
}

var anyType = reflect.TypeOf(new(any)).Elem()

func (r *observedRows) ColumnTypeScanType(index int) reflect.Type {
	if inner, ok := r.inner.(driver.RowsColumnTypeScanType); ok {
		return inner.ColumnTypeScanType(index)
	}
	return anyType
}

var (
	_ driver.Conn                           = (*observedConn)(nil)
	_ driver.ConnBeginTx                    = (*observedConn)(nil)
	_ driver.ConnPrepareContext             = (*observedConn)(nil)
	_ driver.ExecerContext                  = (*observedConn)(nil)
	_ driver.QueryerContext                 = (*observedConn)(nil)
	_ driver.Pinger                         = (*observedConn)(nil)
	_ driver.SessionResetter                = (*observedConn)(nil)
	_ driver.Validator                      = (*observedConn)(nil)
	_ driver.Stmt                           = (*observedStmt)(nil)
	_ driver.StmtExecContext                = (*observedStmt)(nil)
	_ driver.StmtQueryContext               = (*observedStmt)(nil)
	_ driver.Rows                           = (*observedRows)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName = (*observedRows)(nil)
	_ driver.RowsColumnTypeLength           = (*observedRows)(nil)
	_ driver.RowsColumnTypeNullable         = (*observedRows)(nil)
	_ driver.RowsColumnTypePrecisionScale   = (*observedRows)(nil)
	_ driver.RowsColumnTypeScanType         = (*observedRows)(nil)
)

// --- authority fencing -----------------------------------------------------

// `authority.go` moves the authority generation when a *gated* write commits.
// That covers every write the server makes through `internal/dbwork`, which is
// all of them — and "all of them" is exactly the kind of claim that is true
// until it is not. The old server's rule (audit principle 22) was two signals,
// not one: the caller's declared intent *and* a marker in the SQL itself, so
// that a write which slips past the first is still caught by the second.
//
// The driver is where the second signal lives, because it sees every statement
// whatever helper issued it. A direct `db.Exec` that revokes a session takes the
// gate nowhere and moves no class, and this is what stops it leaving a cached
// principal live.

// authorityTables are the tables whose contents decide whether a principal is
// still who it says it is, or may still see what it used to. The list is
// deliberately short and deliberately over-inclusive: a false bump costs a cache
// miss, and a missed bump costs a revoked session.
// They are named exactly, because the name is now read off the head of the
// write rather than looked for anywhere in its text. A near-miss costs a missed
// bump, so the list is wide: everything identity, membership, restriction,
// enrolment and access-policy lives in, plus the two playback tables that decide
// whether a viewer may play at all. `internal/identity` takes the security-fence
// class for all of these anyway, which is the declared signal; this is the
// textual one, for the paths that take their class from a background loop.
var authorityTables = []string{
	"libraries", "library_sources",
	"accounts", "direct_profiles", "direct_memberships", "direct_profile_trust",
	"restrictions", "profile_restrictions", "restriction_outbox",
	"policy", "library_network_policy", "playback_owner_policy",
	"authorization_session_families", "authorization_family_tokens", "authorization_family_renewals",
	"dvr_private_libraries", "onboarding_state_v1", "quick_connect_v1",
	"api_keys", "access_api_keys", "identity_account_factors", "identity_browser_accounts",
	"hosted_profile_revisions", "hosted_profile_erasure_receipts", "social_group_members",
}

// noticeAuthorityWrite bumps the generation when a statement writes an authority
// table. Reads are ignored: the check runs only for a statement that begins with
// a mutation keyword, which is a one-character comparison for the overwhelming
// majority of traffic.
func noticeAuthorityWrite(query string) {
	if len(query) < 6 {
		return
	}
	switch query[0] {
	case 'i', 'I', 'u', 'U', 'd', 'D', 'r', 'R':
	default:
		return
	}
	lowered := strings.ToLower(query)
	if !strings.HasPrefix(lowered, "insert") && !strings.HasPrefix(lowered, "update") &&
		!strings.HasPrefix(lowered, "delete") && !strings.HasPrefix(lowered, "replace") {
		return
	}
	// The table a write names is the table it writes. Reading it off the
	// statement's head is both cheaper and far more precise than looking for the
	// name anywhere in the text: `playback_viewer_leases SET status='revoked'`
	// and `playback_delivery_plans(…,policy,…)` are not authority writes, and
	// under a substring rule they were — measured on the smoke tier, the playback
	// leg alone moved the authority generation a hundred and thirty-eight times
	// in a second, which is the principal, restriction and library-grant caches
	// never holding.
	if target, ok := writtenTable(lowered); ok {
		for _, table := range authorityTables {
			if target == table {
				BumpAuthority()
				return
			}
		}
		return
	}
	// A shape this cannot read — a CTE in front of the write, a form a future
	// driver introduces — falls back to the older, over-inclusive rule. A false
	// bump costs a cache miss; a missed bump costs a revoked session.
	for _, table := range authorityTables {
		if namesTable(lowered, table) {
			BumpAuthority()
			return
		}
	}
}

// writtenTable reads the target of a single-table write off its head:
// `insert [or …] into X`, `replace into X`, `update [or …] X`, `delete from X`.
// It reports false for anything else, which the caller treats conservatively.
func writtenTable(statement string) (string, bool) {
	fields := strings.Fields(statement)
	for index := 0; index < len(fields) && index < 6; index++ {
		switch fields[index] {
		case "into", "from":
			if index+1 < len(fields) {
				return bareName(fields[index+1]), true
			}
			return "", false
		case "update":
			// `update X set …`, and `update or replace X set …`.
			next := index + 1
			if next < len(fields) && fields[next] == "or" {
				next += 2
			}
			if next < len(fields) {
				return bareName(fields[next]), true
			}
			return "", false
		}
	}
	return "", false
}

// bareName strips what a table name can be written with and still be that
// table: a trailing column list, quotes, brackets, a schema qualifier.
func bareName(token string) string {
	if cut := strings.IndexAny(token, "(,;"); cut >= 0 {
		token = token[:cut]
	}
	token = strings.Trim(token, "`\"[]")
	if dot := strings.LastIndexByte(token, '.'); dot >= 0 {
		token = token[dot+1:]
	}
	return token
}

// namesTable reports whether a statement names this table, rather than merely
// containing its spelling.
//
// The list is over-inclusive on purpose — a false bump costs a cache miss and a
// missed bump costs a revoked session — but a substring match is over-inclusive
// in a way that costs more than a cache miss. `playback_sessions` contains
// `sessions`, so every playback session created, reported on or stopped bumped
// the authority generation and flushed the principal, restriction and
// library-grant caches for every viewer on the server. Under two hundred people
// watching, that is the caches never holding at all: measured on the release
// tier, the playback leg alone bumped the generation about seventeen hundred
// times in two minutes.
//
// An identifier boundary before the name is what distinguishes the two. The
// list's own entries stay exactly as they are.
func namesTable(statement, table string) bool {
	from := 0
	for {
		at := strings.Index(statement[from:], table)
		if at < 0 {
			return false
		}
		at += from
		if at == 0 || !identifierByte(statement[at-1]) {
			return true
		}
		from = at + 1
	}
}

func identifierByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}
