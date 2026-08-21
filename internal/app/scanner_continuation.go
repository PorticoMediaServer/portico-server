package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"time"
)

type scannerContinuationDirectory struct {
	Path           string
	Signature      string
	MediaFileCount int
	Changed        bool
}

type scannerContinuation struct {
	LibraryID      string
	Mode           string
	ScanGeneration string
	Directories    map[string]scannerContinuationDirectory
}

func (s *Server) beginOrResumeScannerContinuation(ctx context.Context, libraryID, mode string) (scannerContinuation, error) {
	mode = normalizeScanMode(mode)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	continuation := scannerContinuation{LibraryID: libraryID, Mode: mode, Directories: map[string]scannerContinuationDirectory{}}
	err := s.withBackgroundTxTagged(ctx, []string{"library_scan_continuations", "library_scan_continuation_directories"}, func(tx *sql.Tx) error {
		var storedMode, generation string
		err := tx.QueryRowContext(ctx, `SELECT mode, scan_generation FROM library_scan_continuations WHERE library_id = ?`, libraryID).Scan(&storedMode, &generation)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if errors.Is(err, sql.ErrNoRows) || normalizeScanMode(storedMode) != mode || generation == "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM library_scan_continuations WHERE library_id = ?`, libraryID); err != nil {
				return err
			}
			generation = randomID("scan")
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO library_scan_continuations (library_id, mode, scan_generation, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?)`, libraryID, mode, generation, now, now); err != nil {
				return err
			}
		}
		continuation.ScanGeneration = generation
		rows, err := tx.QueryContext(ctx, `
			SELECT path, signature, media_file_count, changed
			FROM library_scan_continuation_directories WHERE library_id = ?`, libraryID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var directory scannerContinuationDirectory
			var changed int
			if err := rows.Scan(&directory.Path, &directory.Signature, &directory.MediaFileCount, &changed); err != nil {
				return err
			}
			directory.Path = filepath.Clean(directory.Path)
			directory.Changed = changed == 1
			continuation.Directories[directory.Path] = directory
		}
		return rows.Err()
	})
	return continuation, err
}

func (s *Server) persistScannerContinuationDirectories(ctx context.Context, continuation scannerContinuation, directories map[string]scannerContinuationDirectory) error {
	if len(directories) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withBackgroundTxTagged(ctx, []string{"library_scan_continuations", "library_scan_continuation_directories"}, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE library_scan_continuations SET updated_at = ?
			WHERE library_id = ? AND mode = ? AND scan_generation = ?`,
			now, continuation.LibraryID, continuation.Mode, continuation.ScanGeneration)
		if err != nil {
			return err
		}
		if rowsAffected(result) != 1 {
			return errors.New("scanner continuation generation changed")
		}
		for _, directory := range directories {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO library_scan_continuation_directories (
					library_id, path, signature, media_file_count, changed, updated_at
				) VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT(library_id, path) DO UPDATE SET
					signature = excluded.signature,
					media_file_count = excluded.media_file_count,
					changed = excluded.changed,
					updated_at = excluded.updated_at`,
				continuation.LibraryID, filepath.Clean(directory.Path), directory.Signature,
				directory.MediaFileCount, boolToInt(directory.Changed), now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Server) clearScannerContinuation(ctx context.Context, continuation scannerContinuation) error {
	result, err := s.execBackgroundWriteTagged(ctx, []string{"library_scan_continuations", "library_scan_continuation_directories"}, `
		DELETE FROM library_scan_continuations
		WHERE library_id = ? AND mode = ? AND scan_generation = ?`,
		continuation.LibraryID, continuation.Mode, continuation.ScanGeneration)
	if err != nil {
		return err
	}
	if rowsAffected(result) != 1 {
		return errors.New("scanner continuation generation changed before completion")
	}
	return nil
}
