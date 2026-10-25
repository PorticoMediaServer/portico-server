package remotesources

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/storage"
)

// A WebDAV source's own read cache keeps the beginning of a file it has just
// read on this server's disk, one immutable prefix per object version. It is
// private to the source, bounded by that source's saved cache size and floor,
// and never shared with another source or another version of the same object.

type rangeCache struct {
	service       *Service
	source        string
	key           string
	file          *os.File
	size          int64
	cached        int64
	configured    int64
	floor         int64
	committed     bool
	guard         *cacheGuard
	budget        *sourceCacheBudget
	reservedBytes int64
	generation    string
	binding       binding
	release       func()
}

func cacheDirectory(private, source string) (string, error) {
	if source == "" || len(source) > 128 {
		return "", storage.ErrRemoteConfig
	}
	return filepath.Join(private, "source-cache", source), nil
}

func cacheKey(generation string, snapshot storage.Snapshot) string {
	sum := sha256.Sum256([]byte(generation + "\x00" + snapshot.Path + "\x00" + snapshot.Revision + "\x00" + strconv.FormatInt(snapshot.Size, 10)))
	return hex.EncodeToString(sum[:])
}

// sourceCacheConfigTx reads the saved WebDAV cache size and free-space floor.
func sourceCacheConfigTx(ctx context.Context, tx *sql.Tx, source string) (int64, int64, error) {
	var bytes, floor int64
	err := tx.QueryRowContext(ctx, `SELECT cache_bytes,cache_floor_bytes FROM remote_source_settings WHERE source_id=?`, source).Scan(&bytes, &floor)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	return bytes, floor, err
}

// openRangeCache pins one object version's prefix file for a source. It
// returns nil when this source has no cache configured.
func (s *Service) openRangeCache(ctx context.Context, b binding, generation string, snapshot storage.Snapshot) (*rangeCache, error) {
	if s == nil || s.db == nil || s.private == "" {
		return nil, nil
	}
	snapshotTx, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return nil, err
	}
	bytes, floor, err := sourceCacheConfigTx(ctx, snapshotTx.Tx(), b.ID)
	snapshotTx.Rollback()
	if err != nil || bytes <= 0 {
		return nil, err
	}
	directory, err := cacheDirectory(s.private, b.ID)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, storage.ErrRemoteLimit
	}
	key := cacheKey(generation, snapshot)
	file := filepath.Join(directory, key+".range")
	guard := s.guardFor(b.ID, key)
	guard.mu.Lock()
	defer guard.mu.Unlock()
	handle, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, storage.ErrRemoteLimit
	}
	if !guard.loaded {
		cached := int64(0)
		var used int64
		rowErr := s.db.QueryRowContext(ctx, `SELECT used_bytes FROM remote_source_cache_entries WHERE source_id=? AND cache_key=?`, b.ID, key).Scan(&used)
		if rowErr != nil && !errors.Is(rowErr, sql.ErrNoRows) {
			handle.Close()
			return nil, rowErr
		}
		if rowErr == nil && used >= 0 && used <= snapshot.Size {
			if info, statErr := handle.Stat(); statErr == nil && info.Size() >= used {
				cached = used
			}
		}
		if err = handle.Truncate(cached); err != nil {
			handle.Close()
			return nil, err
		}
		guard.cached = cached
		guard.loaded = true
	}
	// A file shorter than the shared prefix (truncated outside this process) is
	// not trusted: the prefix restarts at zero for everyone.
	if info, statErr := handle.Stat(); statErr != nil || info.Size() < guard.cached {
		if err = handle.Truncate(0); err != nil {
			handle.Close()
			return nil, err
		}
		guard.cached = 0
	}
	guard.pins++
	return &rangeCache{service: s, source: b.ID, key: key, file: handle, size: snapshot.Size, configured: bytes, floor: floor, guard: guard, generation: generation, binding: b}, nil
}

// append fills the cache from the current end to end. Growth is checked
// against the settings in force right now, reclaimed from this source's least
// recently used entries when the budget is full, and reserved against both the
// source budget and the host's outstanding growth before a byte is copied.
func (c *rangeCache) append(ctx context.Context, stream io.Reader, end int64) error {
	if c == nil || c.file == nil || end <= c.guard.cached || end > c.size {
		return nil
	}
	var configured, floor int64
	if err := c.service.db.QueryRowContext(ctx, `SELECT cache_bytes,cache_floor_bytes FROM remote_source_settings WHERE source_id=?`, c.source).Scan(&configured, &floor); err != nil || configured <= 0 {
		return storage.ErrRemoteLimit
	}
	if c.guard.cached+(end-c.guard.cached) > configured {
		return storage.ErrRemoteLimit
	}
	growth := end - c.guard.cached
	if !reserveDiskGrowth(c.service.private, floor, growth) {
		return storage.ErrRemoteLimit
	}
	if c.budget == nil {
		c.budget = c.service.budgetFor(c.source)
	}
	budget := c.budget
	if err := budget.load(ctx, c.service, c.source, configured); err != nil {
		releaseDiskGrowth(c.service.private, growth)
		return err
	}
	if !budget.reserve(configured, floor, growth) {
		c.service.evictSourceCache(ctx, c.source, configured, c.key, growth)
		if !budget.reserve(configured, floor, growth) {
			releaseDiskGrowth(c.service.private, growth)
			return storage.ErrRemoteLimit
		}
	}
	if _, err := c.file.Seek(c.guard.cached, io.SeekStart); err != nil {
		budget.release(growth)
		releaseDiskGrowth(c.service.private, growth)
		return err
	}
	n, err := io.CopyN(c.file, stream, growth)
	releaseDiskGrowth(c.service.private, growth)
	if err != nil || n != growth {
		budget.release(growth)
		_ = c.file.Truncate(c.guard.cached)
		return mapError(err)
	}
	budget.accept(growth)
	c.guard.cached = end
	c.reservedBytes += growth
	return nil
}

// Disk growth reservations are fleet-wide for one private directory, so two
// sources cannot each pass a floor check and then cross it together.
var diskGrowth = struct {
	sync.Mutex
	byPath map[string]int64
}{byPath: map[string]int64{}}

func reserveDiskGrowth(path string, floor, growth int64) bool {
	if growth <= 0 {
		return false
	}
	diskGrowth.Lock()
	defer diskGrowth.Unlock()
	pending := diskGrowth.byPath[path]
	if !mounts.CacheSpaceFor(path, floor, growth+pending) {
		return false
	}
	diskGrowth.byPath[path] = pending + growth
	return true
}

func releaseDiskGrowth(path string, growth int64) {
	diskGrowth.Lock()
	defer diskGrowth.Unlock()
	diskGrowth.byPath[path] -= growth
	if diskGrowth.byPath[path] <= 0 {
		delete(diskGrowth.byPath, path)
	}
}

// commit publishes the prefix only inside one statement that also checks the
// source is still the same generation with the same cache size and floor, so a
// reconfigure cannot slip between a read and a write. A stale or retired entry
// discards its file and never returns to the table.
func (c *rangeCache) commit(ctx context.Context) error {
	if c == nil || c.file == nil || c.committed {
		return nil
	}
	c.committed = true
	guard := c.guard
	guard.mu.Lock()
	defer guard.mu.Unlock()
	name := filepath.Base(c.file.Name())
	if err := c.file.Sync(); err != nil {
		_ = c.file.Close()
		return err
	}
	if err := c.file.Close(); err != nil {
		return err
	}
	if c.service.cacheGuardRetired(guard) {
		return c.discardLocked(name)
	}
	now := time.Now().UnixMilli()
	result, err := dbwork.ExecWrite(ctx, c.service.db, dbwork.ClassBackgroundMedia, `INSERT INTO remote_source_cache_entries(source_id,cache_key,file_name,used_bytes,object_size,accessed_ms)
SELECT ?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM remote_sources r JOIN remote_source_settings st ON st.source_id=r.id WHERE r.id=? AND r.removed=0 AND r.generation=? AND st.cache_bytes=? AND st.cache_floor_bytes=?)
ON CONFLICT(source_id,cache_key) DO UPDATE SET used_bytes=excluded.used_bytes,object_size=excluded.object_size,accessed_ms=excluded.accessed_ms`,
		c.source, c.key, name, guard.cached, c.size, now, c.source, mustInt(c.generation), c.configured, c.floor)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return c.discardLocked(name)
	}
	return c.service.evictSourceCache(ctx, c.source, c.configured, c.key, 0)
}

// discardLocked runs under commit's guard. Reacquiring it would deadlock the
// acquisition's Close before it can unpin the cache and release its source slot.
func (c *rangeCache) discardLocked(name string) error {
	sole := c.guard.pins <= 1
	if sole {
		c.guard.cached, c.guard.loaded = 0, false
	}
	if sole {
		directory, err := cacheDirectory(c.service.private, c.source)
		if err == nil {
			_ = os.Remove(filepath.Join(directory, name))
		}
	}
	// Settle against the exact bucket this allocation reserved from.
	if c.budget != nil {
		c.budget.subtract(c.reservedBytes)
	}
	return nil
}

func mustInt(value string) int64 {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func (s *Service) cacheGuardRetired(g *cacheGuard) bool {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	return g.retired
}

// evictSourceCache deletes the least recently used entries of one source,
// except the entry just read, until their total is at or below the configured
// size. Rows are removed first, files after, and only inside this source.
func (s *Service) evictSourceCache(ctx context.Context, source string, limit int64, keep string, need int64) error {
	if limit <= 0 {
		return nil
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(used_bytes),0) FROM remote_source_cache_entries WHERE source_id=?`, source).Scan(&total); err != nil {
		return err
	}
	if total+need <= limit {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT cache_key,file_name,used_bytes FROM remote_source_cache_entries WHERE source_id=? AND cache_key<>? ORDER BY accessed_ms, cache_key LIMIT 64`, source, keep)
	if err != nil {
		return err
	}
	type victim struct {
		key, file string
		bytes     int64
	}
	victims := []victim{}
	for rows.Next() && total+need > limit {
		var v victim
		if err = rows.Scan(&v.key, &v.file, &v.bytes); err != nil {
			break
		}
		victims = append(victims, v)
		total -= v.bytes
	}
	readErr := rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if readErr != nil {
		return readErr
	}
	directory, dirErr := cacheDirectory(s.private, source)
	budget := s.budgetFor(source)
	for _, v := range victims {
		g := s.guardFor(source, v.key)
		// Never block while another key's guard is held by this caller: a pair
		// of growths would otherwise wait on each other. Skipping is safe; the
		// next attempt reclaims.
		if !g.mu.TryLock() {
			continue
		}
		if g.readers > 0 || g.pins > 0 {
			g.mu.Unlock()
			continue
		}
		if _, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM remote_source_cache_entries WHERE source_id=? AND cache_key=?`, source, v.key); err != nil {
			g.mu.Unlock()
			return err
		}
		if dirErr == nil && filepath.Base(v.file) == v.file {
			_ = os.Remove(filepath.Join(directory, v.file))
		}
		// The inode is gone; the shared prefix for that key restarts empty.
		g.cached, g.loaded = 0, false
		g.mu.Unlock()
		budget.subtract(v.bytes)
	}
	return nil
}

// clearSourceCache removes every cached prefix of one source, used when its
// configuration or generation changes so no old credential's bytes survive.
func (s *Service) clearSourceCache(ctx context.Context, source string) error {
	if s == nil || s.db == nil {
		return nil
	}
	s.retireSourceCacheGuards(source)
	// A new budget object owns all future allocations; old allocations settle
	// only against the object they reserved from.
	s.cacheMu.Lock()
	delete(s.cacheBudgets, source)
	s.cacheMu.Unlock()
	rows, err := s.db.QueryContext(ctx, `SELECT cache_key,file_name FROM remote_source_cache_entries WHERE source_id=? LIMIT 1024`, source)
	if err != nil {
		return err
	}
	type owned struct{ key, file string }
	files := []owned{}
	for rows.Next() {
		var one owned
		if rows.Scan(&one.key, &one.file) == nil {
			files = append(files, one)
		}
	}
	rows.Close()
	if _, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM remote_source_cache_entries WHERE source_id=?`, source); err != nil {
		return err
	}
	directory, dirErr := cacheDirectory(s.private, source)
	if dirErr != nil {
		return nil
	}
	for _, one := range files {
		if filepath.Base(one.file) != one.file || one.key == "" {
			continue
		}
		g := s.guardFor(source, one.key)
		if !g.mu.TryLock() {
			continue
		}
		if g.pins == 0 && g.readers == 0 {
			_ = os.Remove(filepath.Join(directory, one.file))
			g.cached, g.loaded = 0, false
		}
		g.mu.Unlock()
	}
	return nil
}

// reconcileSourceCache drops entries whose file is missing or shorter than the
// bytes they claim. It pages by (source,key) across successive wakes, uses an
// exact key/bytes re-check and row count before removing a file, and never
// treats a file it did not see in a page as an orphan.
func (s *Service) reconcileSourceCachePage(ctx context.Context) {
	s.cacheMu.Lock()
	cursorSource, cursorKey := s.cacheReconcile.source, s.cacheReconcile.key
	s.cacheMu.Unlock()
	rows, err := s.db.QueryContext(ctx, `SELECT source_id,cache_key,file_name,used_bytes FROM remote_source_cache_entries WHERE (source_id>? OR (source_id=? AND cache_key>?)) ORDER BY source_id,cache_key LIMIT 64`, cursorSource, cursorSource, cursorKey)
	if err != nil {
		return
	}
	type entry struct {
		source, key, file string
		used              int64
	}
	page := []entry{}
	for rows.Next() {
		var e entry
		if rows.Scan(&e.source, &e.key, &e.file, &e.used) == nil {
			page = append(page, e)
		}
	}
	readErr := rows.Err()
	rows.Close()
	if readErr != nil {
		return
	}
	if len(page) == 0 {
		s.cacheMu.Lock()
		s.cacheReconcile.source, s.cacheReconcile.key = "", ""
		s.cacheMu.Unlock()
		return
	}
	for _, e := range page {
		cursorSource, cursorKey = e.source, e.key
		if filepath.Base(e.file) != e.file {
			continue
		}
		directory, dirErr := cacheDirectory(s.private, e.source)
		if dirErr != nil {
			continue
		}
		g := s.guardFor(e.source, e.key)
		if !g.mu.TryLock() {
			continue
		}
		if g.readers > 0 || g.pins > 0 {
			g.mu.Unlock()
			continue
		}
		info, statErr := os.Stat(filepath.Join(directory, e.file))
		if statErr == nil && info.Size() >= e.used {
			g.mu.Unlock()
			continue
		}
		result, delErr := dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM remote_source_cache_entries WHERE source_id=? AND cache_key=? AND used_bytes=?`, e.source, e.key, e.used)
		if delErr == nil {
			if n, _ := result.RowsAffected(); n == 1 {
				_ = os.Remove(filepath.Join(directory, e.file))
				s.budgetFor(e.source).subtract(e.used)
			}
		}
		g.mu.Unlock()
	}
	s.cacheMu.Lock()
	s.cacheReconcile.source, s.cacheReconcile.key = cursorSource, cursorKey
	s.cacheMu.Unlock()
}

type sourceCacheBudget struct {
	mu       sync.Mutex
	used     int64
	reserved int64
	limit    int64
	loaded   bool
}

type cacheGuard struct {
	mu      sync.Mutex
	readers int
	retired bool
	// cached is the committed prefix both readers and writers share for this
	// source+key; it only moves under mu.
	cached int64
	loaded bool
	// pins counts acquisitions holding this entry open, so eviction never
	// unlinks a file an acquisition may still publish from.
	pins int
}

// guardFor returns the per source+key ownership record, creating it lazily.
func (s *Service) guardFor(source, key string) *cacheGuard {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.cacheGuards == nil {
		s.cacheGuards = map[string]*cacheGuard{}
	}
	id := source + "\x00" + key
	if g := s.cacheGuards[id]; g != nil {
		return g
	}
	g := &cacheGuard{}
	s.cacheGuards[id] = g
	return g
}

func (s *Service) budgetFor(source string) *sourceCacheBudget {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.cacheBudgets == nil {
		s.cacheBudgets = map[string]*sourceCacheBudget{}
	}
	if b := s.cacheBudgets[source]; b != nil {
		return b
	}
	b := &sourceCacheBudget{}
	s.cacheBudgets[source] = b
	return b
}

// load reads the committed total once and keeps it in memory under the budget
// lock, so concurrent writers reserve against a single number.
func (b *sourceCacheBudget) load(ctx context.Context, service *Service, source string, limit int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.limit = limit
	if b.loaded {
		return nil
	}
	var used int64
	if err := service.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(used_bytes),0) FROM remote_source_cache_entries WHERE source_id=?`, source).Scan(&used); err != nil {
		return err
	}
	b.used = used
	b.loaded = true
	return nil
}

// reserve accepts growth only when the source's own budget and the host floor
// both still hold, counting other writers' reservations.
func (b *sourceCacheBudget) reserve(limit, floor, growth int64) bool {
	if growth <= 0 || limit <= 0 || growth > limit {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used+b.reserved+growth > limit {
		return false
	}
	b.reserved += growth
	return true
}

func (b *sourceCacheBudget) accept(growth int64) {
	b.mu.Lock()
	b.reserved -= growth
	b.used += growth
	if b.reserved < 0 {
		b.reserved = 0
	}
	b.mu.Unlock()
}

func (b *sourceCacheBudget) release(growth int64) {
	b.mu.Lock()
	b.reserved -= growth
	if b.reserved < 0 {
		b.reserved = 0
	}
	b.mu.Unlock()
}

func (b *sourceCacheBudget) subtract(bytes int64) {
	b.mu.Lock()
	b.used -= bytes
	if b.used < 0 {
		b.used = 0
	}
	b.mu.Unlock()
}

// retireSourceCacheGuards stops any finishing writer of this source from
// publishing after its configuration or generation changed.
func (s *Service) retireSourceCacheGuards(source string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	prefix := source + "\x00"
	for id, g := range s.cacheGuards {
		if strings.HasPrefix(id, prefix) {
			g.retired = true
		}
	}
}

// cacheReadCloser keeps the acquisition lease and its cancellation live for the
// whole body, exactly like a provider range read.
type cacheReadCloser struct {
	reader  io.Reader
	ctx     context.Context
	release func()
	guard   *cacheGuard
}

func (r *cacheReadCloser) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (r *cacheReadCloser) Close() error {
	if r.guard != nil {
		r.guard.mu.Lock()
		if r.guard.readers > 0 {
			r.guard.readers--
		}
		r.guard.mu.Unlock()
		r.guard = nil
	}
	if r.release != nil {
		r.release()
		r.release = nil
	}
	return nil
}

func (c *rangeCache) holdReader(ctx context.Context, offset, length int64, release func()) io.ReadCloser {
	c.guard.mu.Lock()
	defer c.guard.mu.Unlock()
	return c.holdReaderLocked(ctx, offset, length, release)
}

// holdReaderLocked is the atomic form for a caller already holding the guard:
// lookup and reader pin cannot be separated from the fill decision.
func (c *rangeCache) holdReaderLocked(ctx context.Context, offset, length int64, release func()) io.ReadCloser {
	c.guard.readers++
	return &cacheReadCloser{reader: io.NewSectionReader(c.file, offset, length), ctx: ctx, release: release, guard: c.guard}
}

// unpin releases the acquisition's hold on this entry after its cache has been
// committed or discarded.
func (c *rangeCache) unpin() {
	if c == nil || c.guard == nil {
		return
	}
	c.guard.mu.Lock()
	if c.guard.pins > 0 {
		c.guard.pins--
	}
	c.guard.mu.Unlock()
}

func (s *Service) cacheUsed(ctx context.Context, source string) (int64, error) {
	var used int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(used_bytes),0) FROM remote_source_cache_entries WHERE source_id=?`, source).Scan(&used)
	return used, err
}
