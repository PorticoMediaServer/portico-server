package catalog

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/requestinput"
	"strings"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/operations"
)

// Removing a library, and removing one folder from a library.
//
// Removing a library hides it at once (nobody sees it, nothing scans it) and
// then takes it apart in the background in short transactions: every title
// goes, with each profile's state kept under the title's identities so the
// same title added again later gets it back; then the scan records; then the
// library itself. Files on disk are never touched.
//
// Removing a folder marks the files that were under it missing. Their titles
// follow the owner's rule for missing titles, exactly as if the files had been
// deleted, and a folder added back before then brings them back.

// ErrLibraryRemoving: the library is already being removed.
var ErrLibraryRemoving = errors.New("the library is being removed")

const (
	libraryRemovalKind = "library-removal"
	folderRemovalKind  = "folder-removal"
	removalBatch       = 4
)

// RemoveLibrary starts removing a library. name must be the library's current
// name (the owner typed it back) and expected its configuration revision.
// enqueue queues the background work in the same transaction.
func (s *Service) RemoveLibrary(ctx context.Context, id, name string, expected int64, authorize func(*sql.Tx) error, enqueue func(context.Context, *sql.Tx, string, string) error) error {
	if enqueue == nil {
		return ErrAdminQuery
	}
	return dbwork.WithWriteTx(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive), func(tx *sql.Tx) error {
		if err := configurationFence(ctx, tx, id, expected, authorize); err != nil {
			return err
		}
		var current string
		var retired bool
		if err := tx.QueryRowContext(ctx, `SELECT l.name,COALESCE((SELECT retired FROM catalog_libraries WHERE library_id=l.id),0) FROM libraries l WHERE l.id=?`, id).Scan(&current, &retired); err != nil {
			return err
		}
		if retired {
			return ErrLibraryRemoving
		}
		if strings.TrimSpace(name) != current {
			return ErrConfirmationMismatch
		}
		if _, err := compactcatalog.LibraryTx(ctx, tx, id); err != nil {
			return err
		}
		for _, q := range []string{
			`UPDATE catalog_libraries SET retired=1 WHERE library_id=?`,
			`UPDATE jobs SET status='cancelled' WHERE library_id=? AND status IN('queued','running')`,
			`DELETE FROM inventory_source_active WHERE source_id IN(SELECT id FROM library_sources WHERE library_id=?)`,
			`UPDATE library_sources SET enabled=0,health='removing',generation=generation+1,next_scan_at='' WHERE library_id=?`,
			`UPDATE library_revisions SET revision=revision+1 WHERE library_id=?`,
			`UPDATE library_configuration SET revision=revision+1 WHERE library_id=?`,
		} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return err
			}
		}
		return enqueue(ctx, tx, libraryRemovalKind, id)
	})
}

// ErrConfirmationMismatch: what the owner typed is not the library's name.
var ErrConfirmationMismatch = requestinput.New("the confirmation does not match")

// LibraryRemovalAdapter is the background work behind RemoveLibrary.
func (s *Service) LibraryRemovalAdapter() operations.Adapter {
	return operations.Adapter{Kind: libraryRemovalKind, Lane: "background-media", Resource: operations.LaneWriteHeavy,
		ValidateTx: func(ctx context.Context, tx *sql.Tx, id string) error {
			var n int
			return tx.QueryRowContext(ctx, `SELECT 1 FROM catalog_libraries WHERE library_id=? AND retired=1`, id).Scan(&n)
		},
		Step: func(ctx context.Context, id string) (operations.JobObservation, error) {
			return s.removalStep(ctx, func(tx *sql.Tx) (bool, error) { return removeLibraryBatchTx(ctx, tx, id) })
		}}
}

// FolderRemovalAdapter is the background work behind RemoveSource.
func (s *Service) FolderRemovalAdapter() operations.Adapter {
	return operations.Adapter{Kind: folderRemovalKind, Lane: "background-media", Resource: operations.LaneWriteHeavy,
		ValidateTx: func(ctx context.Context, tx *sql.Tx, id string) error {
			var n int
			return tx.QueryRowContext(ctx, `SELECT 1 FROM library_sources WHERE id=? AND enabled=0`, id).Scan(&n)
		},
		Step: func(ctx context.Context, id string) (operations.JobObservation, error) {
			return s.removalStep(ctx, func(tx *sql.Tx) (bool, error) { return removeFolderBatchTx(ctx, tx, id) })
		}}
}

func (s *Service) removalStep(ctx context.Context, batch func(*sql.Tx) (bool, error)) (operations.JobObservation, error) {
	out := operations.JobObservation{State: "running"}
	for ctx.Err() == nil {
		done := false
		err := dbwork.WithWriteTx(ctx, s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
			var e error
			done, e = batch(tx)
			return e
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return operations.JobObservation{State: "succeeded"}, nil
			}
			if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
				break
			}
			return out, err
		}
		if done {
			out.State = "succeeded"
			break
		}
		dbwork.Yield(ctx)
	}
	return out, nil
}

// removeFolderBatchTx marks a few more of a removed folder's files missing.
// A folder that was added back in the meantime is left alone.
func removeFolderBatchTx(ctx context.Context, tx *sql.Tx, source string) (bool, error) {
	var enabled bool
	if err := tx.QueryRowContext(ctx, `SELECT enabled FROM library_sources WHERE id=?`, source).Scan(&enabled); err != nil {
		return false, err
	}
	if enabled {
		return true, nil
	}
	type file struct{ object, token string }
	rows, err := tx.QueryContext(ctx, `SELECT id,asset_id FROM inventory_objects INDEXED BY inventory_objects_source WHERE source_id=? AND retired=0 AND state<>'missing' ORDER BY id LIMIT ?`, source, removalBatch*4)
	if err != nil {
		return false, err
	}
	var files []file
	for rows.Next() {
		var f file
		if err = rows.Scan(&f.object, &f.token); err != nil {
			rows.Close()
			return false, err
		}
		files = append(files, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, f := range files {
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET state='missing',missing_since=CASE WHEN missing_since='' THEN ? ELSE missing_since END WHERE id=?`, now, f.object); err != nil {
			return false, err
		}
		id, e := compactcatalog.AssetByTokenTx(ctx, tx, f.token)
		if e != nil {
			return false, e
		}
		if id == 0 {
			continue
		}
		var available bool
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT available FROM inventory_asset_availability WHERE asset_id=?),0)`, f.token).Scan(&available); err != nil {
			return false, err
		}
		if err = compactcatalog.SetAssetAvailableTx(ctx, tx, id, available); err != nil {
			return false, err
		}
		if err = compactcatalog.SettleAssetMissingTx(ctx, tx, id); err != nil {
			return false, err
		}
	}
	return len(files) < removalBatch*4, nil
}

// removeLibraryBatchTx does the next small piece of taking a library apart
// and reports whether nothing is left to do. Each call finds its place from
// what still exists, so a restart carries on where it stopped.
func removeLibraryBatchTx(ctx context.Context, tx *sql.Tx, library string) (bool, error) {
	var handle int64
	var retired bool
	if err := tx.QueryRowContext(ctx, `SELECT id,retired FROM catalog_libraries WHERE library_id=?`, library).Scan(&handle, &retired); err != nil {
		return false, err
	}
	if !retired {
		return true, nil
	}
	// 1. Everything that plays. Removing a title keeps each profile's state and
	//    takes its emptied show, season, album or book with it.
	titles, err := scanInt64s(tx.QueryContext(ctx, `SELECT e.id FROM catalog_entities e INDEXED BY catalog_entities_library JOIN catalog_kinds k ON k.id=e.kind AND k.playable=1 WHERE e.library_id=? ORDER BY e.kind,e.id LIMIT ?`, handle, removalBatch))
	if err != nil {
		return false, err
	}
	for _, title := range titles {
		if err = RemoveTitleTx(ctx, tx, title); err != nil {
			return false, err
		}
	}
	if len(titles) > 0 {
		return false, nil
	}
	// 2. Scan records for files that never became titles.
	type file struct{ object, token string }
	rows, err := tx.QueryContext(ctx, `SELECT o.id,o.asset_id FROM library_sources s CROSS JOIN inventory_objects o INDEXED BY inventory_objects_source ON o.source_id=s.id WHERE s.library_id=? ORDER BY o.id LIMIT ?`, library, removalBatch*4)
	if err != nil {
		return false, err
	}
	var files []file
	for rows.Next() {
		var f file
		if err = rows.Scan(&f.object, &f.token); err != nil {
			rows.Close()
			return false, err
		}
		files = append(files, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, f := range files {
		if _, err = tx.ExecContext(ctx, `DELETE FROM inventory_analysis_queue WHERE object_id=?`, f.object); err != nil {
			return false, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM inventory_objects WHERE id=?`, f.object); err != nil {
			return false, err
		}
		var used bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_objects WHERE asset_id=?1) OR EXISTS(SELECT 1 FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id WHERE a.token=?1)`, f.token).Scan(&used); err != nil {
			return false, err
		}
		if !used {
			if id, e := compactcatalog.AssetByTokenTx(ctx, tx, f.token); e != nil {
				return false, e
			} else if id != 0 {
				if e = forgetAssetTx(ctx, tx, id, f.token); e != nil {
					return false, e
				}
			}
		}
	}
	if len(files) > 0 {
		return false, nil
	}
	// 3. Scan bookkeeping, a bounded slice at a time.
	for _, q := range []string{
		`DELETE FROM inventory_directories WHERE rowid IN(SELECT d.rowid FROM jobs j INDEXED BY jobs_library_directory CROSS JOIN inventory_directories d ON d.job_id=j.id WHERE j.library_id=?1 LIMIT 2000)`,
		`DELETE FROM scan_queue WHERE rowid IN(SELECT q.rowid FROM jobs j INDEXED BY jobs_library_directory CROSS JOIN scan_queue q ON q.job_id=j.id WHERE j.library_id=?1 LIMIT 2000)`,
		`DELETE FROM inventory_analysis_queue WHERE rowid IN(SELECT q.rowid FROM jobs j INDEXED BY jobs_library_directory CROSS JOIN inventory_analysis_queue q ON q.job_id=j.id WHERE j.library_id=?1 LIMIT 2000)`,
	} {
		res, e := tx.ExecContext(ctx, q, library)
		if e != nil {
			return false, e
		}
		if n, _ := res.RowsAffected(); n > 0 {
			return false, nil
		}
	}
	// 4. What is left of the catalogue: containers nobody plays from any more.
	//    One that something still refers to stays behind, out of sight.
	containers, err := scanInt64s(tx.QueryContext(ctx, `SELECT id FROM catalog_entities INDEXED BY catalog_entities_library WHERE library_id=? ORDER BY CASE kind WHEN 3 THEN 0 WHEN 6 THEN 1 WHEN 8 THEN 2 WHEN 10 THEN 3 WHEN 2 THEN 4 WHEN 5 THEN 5 ELSE 6 END,id LIMIT 400`, handle))
	if err != nil {
		return false, err
	}
	removed := 0
	for _, id := range containers {
		if _, err = tx.ExecContext(ctx, `SAVEPOINT library_container`); err != nil {
			return false, err
		}
		if e := forceRemoveContainerTx(ctx, tx, id); e != nil {
			if _, err = tx.ExecContext(ctx, `ROLLBACK TO library_container`); err != nil {
				return false, err
			}
		} else {
			removed++
		}
		if _, err = tx.ExecContext(ctx, `RELEASE library_container`); err != nil {
			return false, err
		}
	}
	if removed > 0 {
		// Another pass: a show goes once its seasons have, an artist once its albums have.
		return false, nil
	}
	if clear, err := compactcatalog.ClearLibraryTx(ctx, tx, library, 64); err != nil {
		return false, err
	} else if !clear {
		return false, nil
	}
	// 5. The library itself. Its folders are freed for another library first,
	//    so the rows that remain can never get in the way of adding it again.
	for _, q := range []string{
		`DELETE FROM inventory_operation_jobs WHERE job_id IN(SELECT id FROM jobs WHERE library_id=?1)`,
		`DELETE FROM inventory_runs WHERE job_id IN(SELECT id FROM jobs WHERE library_id=?1)`,
		`DELETE FROM jobs WHERE library_id=?1`,
		`DELETE FROM audio_tag_evidence WHERE library_id=?1`,
		`DELETE FROM audio_source_metadata WHERE library_id=?1`,
		`DELETE FROM episodic_sources WHERE library_id=?1`,
		`DELETE FROM inventory_policy_pending WHERE library_id=?1`,
		`DELETE FROM catalog_missing_titles WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)`,
		`DELETE FROM admin_documents WHERE scope='library:'||?1`,
		`UPDATE library_sources SET root='removed:'||id,configured_root='removed:'||id WHERE library_id=?1 AND root NOT LIKE 'removed:%'`,
		`UPDATE libraries SET root='removed:'||id WHERE id=?1 AND root NOT LIKE 'removed:%'`,
		`UPDATE catalog_libraries SET root='removed:'||library_id WHERE library_id=?1 AND root NOT LIKE 'removed:%'`,
	} {
		if _, err = tx.ExecContext(ctx, q, library); err != nil {
			return false, err
		}
	}
	if _, err = tx.ExecContext(ctx, `SAVEPOINT library_row`); err != nil {
		return false, err
	}
	gone := true
	for _, q := range []string{
		`DELETE FROM library_scan_policies WHERE library_id=?1`,
		`DELETE FROM library_sources WHERE library_id=?1`,
		`DELETE FROM libraries WHERE id=?1`,
	} {
		if _, e := tx.ExecContext(ctx, q, library); e != nil {
			// Something derived still points at the library. It stays as a
			// hidden, empty row; nothing can reach it and its folders are free.
			gone = false
			break
		}
	}
	if !gone {
		if _, err = tx.ExecContext(ctx, `ROLLBACK TO library_row`); err != nil {
			return false, err
		}
	}
	_, err = tx.ExecContext(ctx, `RELEASE library_row`)
	return true, err
}

// forceRemoveContainerTx removes a show, season, album, artist, book, author
// or collection of a library that is being removed, keeping what people had
// marked on it.
func forceRemoveContainerTx(ctx context.Context, tx *sql.Tx, id int64) error {
	if err := retainStateTx(ctx, tx, id); err != nil {
		return err
	}
	if err := compactcatalog.ForgetIdentitiesTx(ctx, tx, id); err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM container_personal_state WHERE container_id=?`,
		`DELETE FROM container_personal_undo WHERE container_id=?`,
		`DELETE FROM playback_sessions WHERE item_id=?`,
		`DELETE FROM book_resume WHERE book_id=?`,
		`DELETE FROM catalog_song_artists WHERE artist_id=?`,
		`DELETE FROM catalog_book_group_members WHERE book_id=?`,
		`DELETE FROM catalog_book_series_spellings WHERE book_id=?`,
		`DELETE FROM catalog_book_series_terms WHERE book_id=?`,
		`DELETE FROM catalog_book_position_terms WHERE book_id=?`,
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return compactcatalog.DeleteEntityTx(ctx, tx, id)
}
