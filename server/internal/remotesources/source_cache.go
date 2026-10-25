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
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/storage"
)

// A WebDAV source's own read cache keeps the beginning of a file it has just
// read on this server's disk, one immutable prefix per object version. It is
// private to the source, bounded by that source's saved cache size and floor,
// and never shared with another source or another version of the same object.

const (
	cacheBlockMin = 1 << 20
	cacheEntryMax = 64 << 10
)

type rangeCache struct {
	service    *Service
	source     string
	key        string
	file       *os.File
	size       int64
	cached     int64
	configured int64
	floor      int64
	committed  bool
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
	handle, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, storage.ErrRemoteLimit
	}
	cached := int64(0)
	var used int64
	err = s.db.QueryRowContext(ctx, `SELECT used_bytes FROM remote_source_cache_entries WHERE source_id=? AND cache_key=?`, b.ID, key).Scan(&used)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		handle.Close()
		return nil, err
	}
	if err == nil {
		if info, e := handle.Stat(); e == nil && info.Size() >= used && used <= snapshot.Size {
			cached = used
		} else {
			// A missing or short file is not this version's cached prefix.
			if e = handle.Truncate(0); e == nil {
				if _, e = handle.Seek(0, io.SeekStart); e != nil {
					handle.Close()
					return nil, e
				}
			} else {
				handle.Close()
				return nil, e
			}
		}
	}
	return &rangeCache{service: s, source: b.ID, key: key, file: handle, size: snapshot.Size, cached: cached, configured: bytes, floor: floor}, nil
}

// append fills the cache from the current end to end, from a stream positioned
// at the current end. A failed or short read leaves the committed prefix as it
// was, so a later read never trusts bytes that were not fully copied.
func (c *rangeCache) append(ctx context.Context, stream io.Reader, end int64) error {
	if c == nil || c.file == nil || end <= c.cached || end > c.size {
		return nil
	}
	if !mounts.CacheSpaceAvailable(c.service.private, c.floor) {
		return storage.ErrRemoteLimit
	}
	if _, err := c.file.Seek(c.cached, io.SeekStart); err != nil {
		return err
	}
	n, err := io.CopyN(c.file, stream, end-c.cached)
	if err != nil || n != end-c.cached {
		_ = c.file.Truncate(c.cached)
		return mapError(err)
	}
	c.cached = end
	return nil
}

// commit records the cached prefix and evicts this source's least recently
// used entries until the source is inside its saved size again. It never
// evicts the entry just read, and never touches another source's directory.
func (c *rangeCache) commit(ctx context.Context) error {
	if c == nil || c.file == nil || c.committed {
		return nil
	}
	c.committed = true
	if err := c.file.Sync(); err != nil {
		_ = c.file.Close()
		return err
	}
	name := filepath.Base(c.file.Name())
	if err := c.file.Close(); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if _, err := dbwork.ExecWrite(ctx, c.service.db, dbwork.ClassBackgroundMedia, `INSERT INTO remote_source_cache_entries(source_id,cache_key,file_name,used_bytes,object_size,accessed_ms) VALUES(?,?,?,?,?,?) ON CONFLICT(source_id,cache_key) DO UPDATE SET used_bytes=excluded.used_bytes,object_size=excluded.object_size,accessed_ms=excluded.accessed_ms`, c.source, c.key, name, c.cached, c.size, now); err != nil {
		return err
	}
	return c.service.evictSourceCache(ctx, c.source, c.configured, c.key)
}

// evictSourceCache deletes the least recently used entries of one source,
// except the entry just read, until their total is at or below the configured
// size. Rows are removed first, files after, and only inside this source.
func (s *Service) evictSourceCache(ctx context.Context, source string, limit int64, keep string) error {
	if limit <= 0 {
		return nil
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(used_bytes),0) FROM remote_source_cache_entries WHERE source_id=?`, source).Scan(&total); err != nil {
		return err
	}
	if total <= limit {
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
	for rows.Next() && total > limit {
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
	for _, v := range victims {
		if _, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM remote_source_cache_entries WHERE source_id=? AND cache_key=?`, source, v.key); err != nil {
			return err
		}
		if dirErr == nil && filepath.Base(v.file) == v.file {
			_ = os.Remove(filepath.Join(directory, v.file))
		}
	}
	return nil
}

// clearSourceCache removes every cached prefix of one source, used when its
// configuration or generation changes so no old credential's bytes survive.
func (s *Service) clearSourceCache(ctx context.Context, source string) error {
	if s == nil || s.db == nil {
		return nil
	}
	if _, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM remote_source_cache_entries WHERE source_id=?`, source); err != nil {
		return err
	}
	directory, err := cacheDirectory(s.private, source)
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		_ = os.Remove(filepath.Join(directory, entry.Name()))
	}
	return nil
}

// reconcileSourceCache drops rows whose file is gone and files no row owns,
// bounded per call so startup work stays small.
func (s *Service) reconcileSourceCache(ctx context.Context, source string, private string) {
	directory, err := cacheDirectory(private, source)
	if err != nil {
		return
	}
	known := map[string]bool{}
	rows, err := s.db.QueryContext(ctx, `SELECT cache_key,file_name,used_bytes FROM remote_source_cache_entries WHERE source_id=? LIMIT 256`, source)
	if err == nil {
		type row struct {
			key, file string
			used      int64
		}
		all := []row{}
		for rows.Next() {
			var r row
			if rows.Scan(&r.key, &r.file, &r.used) == nil {
				all = append(all, r)
				known[filepath.Base(r.file)] = true
			}
		}
		rows.Close()
		for _, r := range all {
			if info, statErr := os.Stat(filepath.Join(directory, filepath.Base(r.file))); statErr != nil || info.Size() < r.used {
				_, _ = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM remote_source_cache_entries WHERE source_id=? AND cache_key=?`, source, r.key)
				known[filepath.Base(r.file)] = false
			}
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || known[entry.Name()] || removed >= 256 {
			continue
		}
		_ = os.Remove(filepath.Join(directory, entry.Name()))
		removed++
	}
}

func (s *Service) cacheUsed(ctx context.Context, source string) (int64, error) {
	var used int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(used_bytes),0) FROM remote_source_cache_entries WHERE source_id=?`, source).Scan(&used)
	return used, err
}
