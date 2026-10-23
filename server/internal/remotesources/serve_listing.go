package remotesources

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/rcloneprogram"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

type serveListingCapture struct {
	job     string
	owner   string
	source  catalog.LibrarySource
	profile SourceReadProfile
}

func (c serveListingCapture) same(other serveListingCapture) bool {
	a, b := c.source, other.source
	return c.job == other.job && c.owner == other.owner && c.profile == other.profile && a.ID == b.ID && a.LibraryID == b.LibraryID && a.Generation == b.Generation && a.Incarnation == b.Incarnation && a.RootIdentity == b.RootIdentity && a.ResolvedPath == b.ResolvedPath && a.IdentityConfirmed == b.IdentityConfirmed && a.FollowSymlinks == b.FollowSymlinks
}

// This JSON is private listing staging, never a DTO or byte-version grant.
// File facts are enriched after the complete listing body has closed, avoiding
// metadata deadlock on a one-connection source. Publication uses the captured
// expected observation revision in the catalogue's own transaction.
type serveListingEntry struct {
	storage.Snapshot
	Observation *ScanObservation  `json:"scanObservation,omitempty"`
	Expected    int64             `json:"expectedObservationRevision,omitempty"`
	Epoch       string            `json:"processEpoch"`
	Profile     SourceReadProfile `json:"readProfile"`
	rawBytes    int
}

// serveListingFenceTx resolves the actual running inventory, including the
// original job behind a verification listing. A private listing ID or a source
// picker record is never inventory authority. No network runs in this method.
func (s *Service) serveListingFenceTx(ctx context.Context, tx *sql.Tx, b binding, relative, id, generation string) (serveListingCapture, error) {
	var capture serveListingCapture
	directory := filepath.Join(b.Root, filepath.FromSlash(relative))
	err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT c.job_id FROM remote_inventory_checks c WHERE c.check_job=l.job_id),l.job_id) FROM remote_listing_sessions l WHERE l.id=? AND l.source_id=? AND l.generation=? AND l.directory=? AND l.ready=0`, id, b.ID, generation, directory).Scan(&capture.job)
	if err != nil {
		return capture, storage.ErrRemoteCursor
	}
	capture.source, err = catalog.InventoryFenceTx(ctx, tx, capture.job)
	if err != nil {
		return capture, err
	}
	source := capture.source
	child, err := filepath.Rel(source.ResolvedPath, directory)
	if err != nil || source.Kind != "remote" || source.RootIdentity == "" || source.Incarnation == "" || !filepath.IsLocal(child) {
		return capture, storage.ErrRemoteChanged
	}
	var capturedPolicy, currentPolicy int64
	if err = tx.QueryRowContext(ctx, `SELECT r.policy_revision,p.revision FROM inventory_runs r JOIN library_scan_policies p ON p.library_id=? WHERE r.job_id=?`, source.LibraryID, capture.job).Scan(&capturedPolicy, &currentPolicy); err != nil {
		return capture, err
	}
	if capturedPolicy < 1 || capturedPolicy != currentPolicy {
		return capture, storage.ErrRemoteChanged
	}
	if err = s.ValidateScan(ctx, tx, capture.job, source.ResolvedPath); err != nil {
		return capture, err
	}
	current, err := observationGenerationTx(ctx, tx, b.ID)
	if err != nil {
		return capture, err
	}
	if current != generation {
		return capture, storage.ErrRemoteChanged
	}
	var root string
	if err = tx.QueryRowContext(ctx, `SELECT root FROM remote_sources WHERE id=? AND removed=0`, b.ID).Scan(&root); err != nil {
		return capture, err
	}
	actual, _, ok := s.find(directory)
	if root != b.Root || !ok || actual.ID != b.ID || actual.Removed {
		return capture, storage.ErrRemoteChanged
	}
	capture.profile, err = SourceReadProfileTx(ctx, tx, b.ID)
	if err == nil {
		var state string
		e := tx.QueryRowContext(ctx, `SELECT owner,state FROM remote_native_listings WHERE listing_id=?`, id).Scan(&capture.owner, &state)
		if e != nil && e != sql.ErrNoRows {
			return capture, e
		}
		if e == nil && (state != "running" || capture.owner == "") {
			return capture, storage.ErrRemoteCursor
		}
	}
	return capture, err
}

// buildServeSnapshot retains the old two-million-child/512MiB private staging
// budget, with only 128 entries held per SQL batch. No partial body, duplicate,
// failed job/profile/process fence or late parse error publishes ready. This is
// directory evidence, not a file observation or content/decode permission.
func (s *Service) buildServeSnapshot(ctx context.Context, process *rcloneprogram.ServeProcess, b binding, relative, id, generation string) error {
	if process == nil {
		return storage.ErrRemoteConfig
	}
	endpoint, err := process.Endpoint(ctx)
	if err != nil {
		return err
	}
	initial, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer initial.Rollback()
	capture, err := s.serveListingFenceTx(ctx, initial.Tx(), b, relative, id, generation)
	if err == nil {
		err = process.CheckLocal(ctx)
	}
	if err == nil {
		_, err = initial.Tx().ExecContext(ctx, `DELETE FROM remote_listing_entries WHERE listing_id=?`, id)
	}
	if err == nil {
		_, err = initial.Tx().ExecContext(ctx, `DELETE FROM remote_listing_progress WHERE listing_id=?`, id)
	}
	if err == nil {
		err = initial.Commit()
	} else {
		initial.Rollback()
	}
	if err != nil {
		return err
	}
	fence := func(ctx context.Context, tx *sql.Tx) error {
		current, err := s.serveListingFenceTx(ctx, tx, b, relative, id, generation)
		if err != nil {
			return err
		}
		if !current.same(capture) {
			return storage.ErrRemoteChanged
		}
		_, err = s.currentServeProcessTx(ctx, tx, b.ID, capture.profile, process.Generation())
		return err
	}
	check := func(ctx context.Context) error {
		if err := process.Check(ctx); err != nil {
			return err
		}
		view, err := dbwork.BeginSnapshot(ctx, s.db)
		if err != nil {
			return err
		}
		defer view.Rollback()
		return fence(ctx, view.Tx())
	}
	var count, bytes int64
	batch := make([]storage.Snapshot, 0, 128)
	flush := func(ready bool) error {
		writer, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
		if err != nil {
			return err
		}
		defer writer.Rollback()
		if err = fence(ctx, writer.Tx()); err != nil {
			return err
		}
		for _, entry := range batch {
			raw, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			count++
			bytes += int64(len(raw))
			if count > 2_000_000 || bytes > 512<<20 {
				return storage.ErrRemoteLimit
			}
			if _, err = writer.Tx().ExecContext(ctx, `INSERT INTO remote_listing_entries VALUES(?,?,?)`, id, entry.Path, string(raw)); err != nil {
				return storage.ErrRemoteConfig
			}
		}
		if ready {
			receipt, err := json.Marshal(serveListingReceipt{Epoch: process.Generation(), Profile: capture.profile})
			if err != nil {
				return err
			}
			if _, err = writer.Tx().ExecContext(ctx, `INSERT INTO remote_listing_progress(listing_id,token,mode,steps,restarted) VALUES(?,?,'serve',1,0)`, id, receipt); err != nil {
				return err
			}
			result, err := writer.Tx().ExecContext(ctx, `UPDATE remote_listing_sessions SET ready=1 WHERE id=? AND source_id=? AND generation=? AND ready=0`, id, b.ID, generation)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil || n != 1 {
				return storage.ErrRemoteChanged
			}
			if capture.owner != "" {
				if _, err = writer.Tx().ExecContext(ctx, `UPDATE remote_native_listings SET state='complete',entries=?,bytes=? WHERE listing_id=? AND owner=? AND state='running'`, count, bytes, id, capture.owner); err != nil {
					return err
				}
			}
		} else if capture.owner != "" {
			if _, err = writer.Tx().ExecContext(ctx, `UPDATE remote_native_listings SET entries=?,bytes=? WHERE listing_id=? AND owner=? AND state='running'`, count, bytes, id, capture.owner); err != nil {
				return err
			}
		}
		if err = writer.Commit(); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}
	err = endpoint.VisitSnapshot(ctx, relative, check, func(entry remotemedia.DAVEntry) error {
		value := inventorySnapshot(b, generation, davSnapshot(entry, b.Root))
		if filepath.Dir(value.Path) != filepath.Join(b.Root, filepath.FromSlash(relative)) {
			return storage.ErrRemoteConfig
		}
		batch = append(batch, value)
		if len(batch) == cap(batch) {
			return flush(false)
		}
		return nil
	})
	if err != nil {
		return mapError(err)
	}
	if err = flush(false); err != nil {
		return err
	}
	// The complete listing request is closed before any backend metadata call.
	// Only bounded batches live in memory; every rewritten row remains private.
	after := ""
	for {
		rows, err := s.db.QueryContext(ctx, `SELECT name,snapshot FROM remote_listing_entries WHERE listing_id=? AND name>? ORDER BY name LIMIT 128`, id, after)
		if err != nil {
			return err
		}
		values := make([]serveListingEntry, 0, 128)
		for rows.Next() {
			var raw string
			var entry serveListingEntry
			if err = rows.Scan(&after, &raw); err == nil {
				err = json.Unmarshal([]byte(raw), &entry.Snapshot)
			}
			if err != nil {
				break
			}
			entry.rawBytes = len(raw)
			entry.Epoch, entry.Profile = process.Generation(), capture.profile
			values = append(values, entry)
		}
		rowErr := rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if rowErr != nil {
			return rowErr
		}
		if len(values) == 0 {
			break
		}
		for i := range values {
			entry := &values[i]
			if entry.Directory {
				continue
			}
			object, err := filepath.Rel(b.Root, entry.Path)
			if err != nil || !validObservationObject(filepath.ToSlash(object)) {
				return storage.ErrRemoteConfig
			}
			observed, err := observeProcessFile(ctx, process, filepath.ToSlash(object), check)
			if err != nil {
				return mapError(err)
			}
			if observed.Size != entry.Size {
				return storage.ErrRemoteChanged
			}
			entry.Snapshot, err = observationSnapshot(b, generation, observed)
			if err != nil {
				return err
			}
			entry.Observation = &ScanObservation{Facts: observed, ScanRevision: entry.Revision}
			view, err := dbwork.BeginSnapshot(ctx, s.db)
			if err != nil {
				return err
			}
			previous, err := readScanObservationTx(ctx, view.Tx(), b.ID, filepath.ToSlash(object))
			view.Rollback()
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			entry.Expected = previous.Ref.Revision
		}
		writer, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
		if err != nil {
			return err
		}
		if err = fence(ctx, writer.Tx()); err == nil {
			for _, entry := range values {
				var raw []byte
				raw, err = json.Marshal(entry)
				if err != nil {
					break
				}
				bytes += int64(len(raw) - entry.rawBytes)
				if bytes > 512<<20 {
					err = storage.ErrRemoteLimit
					break
				}
				_, err = writer.Tx().ExecContext(ctx, `UPDATE remote_listing_entries SET snapshot=? WHERE listing_id=? AND name=?`, string(raw), id, entry.Path)
				if err != nil {
					break
				}
			}
		}
		if err == nil {
			err = writer.Commit()
		} else {
			writer.Rollback()
		}
		if err != nil {
			return err
		}
	}
	if err = check(ctx); err != nil {
		return err
	}
	return flush(true)
}
