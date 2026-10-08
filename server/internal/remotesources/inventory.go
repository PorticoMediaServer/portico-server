package remotesources

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/storage"
)

type continuation struct{ ID, After string }

func cursor(c continuation) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}
func parseCursor(value string) (continuation, error) {
	var c continuation
	if len(value) > 16<<10 {
		return c, storage.ErrRemoteCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || json.Unmarshal(raw, &c) != nil || c.ID == "" || len(c.After) > 8192 {
		return c, storage.ErrRemoteCursor
	}
	return c, nil
}

// ListPage materializes only this directory in the private adapter cache, then
// returns a bounded immutable page. Partial provider output never becomes a page.
// The durable inventory owner commits these pages and its continuation atomically.
func (s *Service) ListPage(ctx context.Context, job, directory, after string) (storage.RemotePage, error) {
	b, relative, ok := s.find(directory)
	if !ok || b.Removed {
		return storage.RemotePage{}, storage.ErrRemoteOffline
	}
	generation, err := s.generation(ctx, b)
	if err != nil {
		return storage.RemotePage{}, err
	}
	c := continuation{}
	if after != "" {
		c, err = parseCursor(after)
		if err != nil {
			return storage.RemotePage{}, err
		}
	}
	// Each worker quantum performs at most one DAV provider page. Progress and its
	// private token commit together, separately from publishing a full directory.
	release, err := s.claimListing(job + ":" + directory)
	if err != nil {
		return storage.RemotePage{}, err
	}
	defer release()
	if b.Kind == "webdav" {
		select {
		case s.inventory <- struct{}{}:
			defer func() { <-s.inventory }()
		default:
			return storage.RemotePage{}, storage.ErrBusy
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var listedJob, listedDirectory, listedGeneration, sourceID string
	var ready bool
	if c.ID == "" {
		err = s.db.QueryRowContext(ctx, `SELECT id FROM remote_listing_sessions WHERE job_id=? AND directory=?`, job, directory).Scan(&c.ID)
		if errors.Is(err, sql.ErrNoRows) {
			c.ID = identity.Token()
			_, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `INSERT INTO remote_listing_sessions(id,job_id,source_id,generation,directory,created_at) VALUES(?,?,?,?,?,?)`, c.ID, job, b.ID, generation, directory, time.Now().Unix())
		}
		if err != nil {
			return storage.RemotePage{}, err
		}
	}
	err = s.db.QueryRowContext(ctx, `SELECT job_id,directory,generation,source_id,ready FROM remote_listing_sessions WHERE id=?`, c.ID).Scan(&listedJob, &listedDirectory, &listedGeneration, &sourceID, &ready)
	if err != nil || listedJob != job || listedDirectory != directory || sourceID != b.ID {
		return storage.RemotePage{}, storage.ErrRemoteCursor
	}
	if listedGeneration != generation {
		return storage.RemotePage{}, storage.ErrRemoteChanged
	}
	if !ready {
		if b.Kind == "webdav" {
			err = s.buildDAVStep(ctx, b, relative, c.ID, generation)
		} else {
			// The owned producer survives this request/worker quantum. Its
			// partial cache is private until EOF, physical exit and final fence.
			err = s.startNativeListing(ctx, b, relative, c.ID, generation)
		}
		if err != nil {
			s.health(b, generation, err)
			return storage.RemotePage{}, mapError(err)
		}
		if err = s.db.QueryRowContext(ctx, `SELECT ready FROM remote_listing_sessions WHERE id=?`, c.ID).Scan(&ready); err != nil {
			return storage.RemotePage{}, err
		}
		if !ready {
			return storage.RemotePage{ID: c.ID, SourceID: b.ID, Directory: directory, Generation: generation, Cursor: after, Next: cursor(continuation{c.ID, ""}), Entries: []storage.Snapshot{}, Pending: true}, nil
		}
		s.health(b, generation, nil)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot FROM remote_listing_entries WHERE listing_id=? AND name>? ORDER BY name LIMIT 129`, c.ID, c.After)
	if err != nil {
		return storage.RemotePage{}, err
	}
	entries := []storage.Snapshot{}
	for rows.Next() {
		var raw string
		var v storage.Snapshot
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &v); err != nil {
			break
		}
		entries = append(entries, inventorySnapshot(b, generation, v))
	}
	readErr := rows.Err()
	rows.Close()
	if err != nil {
		return storage.RemotePage{}, err
	}
	if readErr != nil {
		return storage.RemotePage{}, readErr
	}
	complete := len(entries) <= 128
	if !complete {
		entries = entries[:128]
	}
	last := c.After
	if len(entries) > 0 {
		last = entries[len(entries)-1].Path
	}
	return storage.RemotePage{ID: c.ID, SourceID: b.ID, Directory: directory, Generation: generation, Cursor: after, Next: cursor(continuation{c.ID, last}), Entries: entries, Complete: complete}, nil
}
func (s *Service) buildListing(ctx context.Context, b binding, relative, id, generation, owner string, retired func()) error {
	handedOff := false
	defer func() {
		if !handedOff {
			retired()
		}
	}()
	var count, bytes int64
	batch := make([]storage.Snapshot, 0, 128)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := s.check(ctx, b, generation); err != nil {
			return err
		}
		gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
		if err != nil {
			return err
		}
		tx := gated.Tx()
		defer gated.Rollback()
		if err = nativeListingFence(ctx, tx, id, owner); err != nil {
			return err
		}
		for _, v := range batch {
			raw, _ := json.Marshal(v)
			count++
			bytes += int64(len(raw))
			if count > 2_000_000 || bytes > 512<<20 {
				return storage.ErrRemoteLimit
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO remote_listing_entries VALUES(?,?,?)`, id, v.Path, string(raw)); err != nil {
				return storage.ErrRemoteConfig
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE remote_native_listings SET entries=?,bytes=?,updated_at=? WHERE listing_id=? AND owner=? AND state='running'`, count, bytes, time.Now().UnixMilli(), id, owner); err != nil {
			return err
		}
		if err = gated.Commit(); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}
	visit := func(v storage.Snapshot) error {
		if filepath.Dir(v.Path) != filepath.Join(b.Root, filepath.FromSlash(relative)) || v.Path == filepath.Join(b.Root, filepath.FromSlash(relative)) {
			return storage.ErrRemoteConfig
		}
		batch = append(batch, v)
		if len(batch) == 128 {
			return flush()
		}
		return ctx.Err()
	}
	var err error
	if b.Kind == "rclone" {
		var n *mounts.NativeBackend
		n, err = s.mounts.Native(ctx, b.ID)
		if err == nil && n.Generation() != generation {
			return storage.ErrRemoteChanged
		}
		if err == nil {
			handedOff = true
			err = n.ListOwned(ctx, relative, func(e mounts.NativeEntry) error {
				child := e.Path
				if relative != "" {
					child = relative + "/" + child
				}
				return visit(e.Snapshot(b.Root, child))
			}, retired)
		}

	} else {
		return storage.ErrRemoteConfig
	}
	if err != nil {
		return err
	}
	if err = flush(); err != nil {
		return err
	}
	if err = s.check(ctx, b, generation); err != nil {
		return err
	}
	gated2, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	if err = nativeListingFence(ctx, tx, id, owner); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE remote_listing_sessions SET ready=1 WHERE id=? AND generation=?`, id, generation); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE remote_native_listings SET state='complete',updated_at=? WHERE listing_id=? AND owner=?`, time.Now().UnixMilli(), id, owner); err != nil {
		return err
	}
	return gated2.Commit()
}

// CommitPage is called in the SAME transaction that inserts scanner children.
// Advancing a cursor before applying its validated page is impossible here.
func (s *Service) CommitPage(ctx context.Context, tx *sql.Tx, p storage.RemotePage) error {
	if tx == nil {
		return storage.ErrRemoteCursor
	}
	var job, gen string
	var removed, ready bool
	err := tx.QueryRowContext(ctx, `SELECT l.job_id,CASE WHEN r.kind='rclone' THEN CAST(m.generation AS TEXT) ELSE CAST(r.generation AS TEXT) END,r.removed,l.ready FROM remote_listing_sessions l JOIN remote_sources r ON r.id=l.source_id LEFT JOIN mount_backend_configs m ON m.mount_id=r.mount_id WHERE l.id=? AND l.source_id=? AND l.directory=? AND l.generation=?`, p.ID, p.SourceID, p.Directory, p.Generation).Scan(&job, &gen, &removed, &ready)
	if err != nil || removed {
		return storage.ErrRemoteCursor
	}
	// Readiness may advance after a Pending response; committing its empty
	// continuation is harmless and must not fail an otherwise healthy scan.
	if p.Pending {
		if len(p.Entries) != 0 || p.Complete {
			return storage.ErrRemoteCursor
		}
	} else if !ready {
		return storage.ErrRemoteCursor
	}
	if gen != p.Generation {
		return storage.ErrRemoteChanged
	}
	var status string
	if tx.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id=?`, job).Scan(&status) != nil || status != "running" {
		return context.Canceled
	}
	var current string
	err = tx.QueryRowContext(ctx, `SELECT cursor FROM remote_scan_cursors WHERE job_id=? AND path=?`, job, p.Directory).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		current = ""
	} else if err != nil {
		return err
	}
	if current != p.Cursor && current != p.Next {
		return storage.ErrRemoteCursor
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO remote_scan_cursors(job_id,path,listing_id,cursor) VALUES(?,?,?,?) ON CONFLICT(job_id,path) DO UPDATE SET cursor=excluded.cursor,listing_id=excluded.listing_id`, job, p.Directory, p.ID, p.Next)
	return err
}
