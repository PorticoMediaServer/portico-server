package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/database"
	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

const artifactCapacityHeadroom = int64(4 << 20)
const artifactOrphanGrace = time.Hour

// publishPrivateArtifact makes a private app-owned artifact visible only after
// the complete payload and its containing directory have been made durable.
// The reservation is deliberately a directory: its creation is atomic across
// processes and cannot be mistaken for a served artifact after a crash.
func publishPrivateArtifact(root, target string, body []byte) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if !pathInsideRoot(target, root) {
		return errors.New("artifact target escaped its managed root")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	available, _, err := filesystemSpace(filepath.Dir(target))
	if err != nil {
		return fmt.Errorf("inspect artifact capacity: %w", err)
	}
	if required := int64(len(body)) + artifactCapacityHeadroom; available < required {
		return fmt.Errorf("artifact publication requires %d bytes but only %d are available", required, available)
	}
	reservation := target + ".reserve"
	if err := os.Mkdir(reservation, 0o700); err != nil {
		return fmt.Errorf("reserve artifact publication: %w", err)
	}
	defer os.Remove(reservation)
	if err := database.WriteAtomicPrivateFile(target, body); err != nil {
		return fmt.Errorf("publish artifact: %w", err)
	}
	return nil
}

type trashMoveJournal struct {
	Version   int    `json:"version"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Published bool   `json:"published"`
}

type durableTrashMove struct {
	journalPath string
	journal     trashMoveJournal
	server      *Server
}

func (s *Server) stageMediaFileToTrash(path string) (durableTrashMove, error) {
	request := s.storageRequestForPath(context.Background(), foundationcontract.WorkClassMaintenance, path, "trash source admission")
	var info os.FileInfo
	err := s.boundedStorageIO(context.Background(), request, func() error {
		var statErr error
		info, statErr = os.Lstat(path)
		return statErr
	})
	if err != nil {
		return durableTrashMove{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return durableTrashMove{}, errors.New("refusing to trash a non-regular media file")
	}
	trashDir := filepath.Join(s.cfg.AppDataDir, "media-trash", time.Now().UTC().Format("2006-01-02"))
	if err := os.MkdirAll(trashDir, 0o700); err != nil {
		return durableTrashMove{}, err
	}
	target := filepath.Join(trashDir, randomID("trash")+"-"+filepath.Base(path))
	journalPath := target + ".move.json"
	journal := trashMoveJournal{Version: 1, Source: filepath.Clean(path), Target: target, Size: info.Size()}
	if err := database.WriteAtomicPrivateFile(journalPath, mustJSON(journal)); err != nil {
		return durableTrashMove{}, fmt.Errorf("reserve trash move: %w", err)
	}
	move := durableTrashMove{journalPath: journalPath, journal: journal, server: s}
	request.Operation = "trash copy publication"
	var hash string
	err = s.boundedStorageProgressIO(context.Background(), request, func(progress func()) error {
		var copyErr error
		hash, copyErr = copyPublishAndRemoveWithProgress(path, target, info.Size(), progress)
		return copyErr
	})
	if err != nil {
		// The copy protocol may have crossed its publish/remove boundary before
		// a final directory sync failed. Roll it back when possible and retain
		// the journal when it is not, so startup reconciliation can finish it.
		// A timed-out kernel call may still complete. The durable journal is the
		// sole authority in that case; racing it with rollback could delete a
		// successfully published copy while the source removal is still pending.
		if !storageErrorTransient(err) {
			_ = move.rollback()
		}
		return durableTrashMove{}, err
	}
	move.journal.SHA256 = hash
	move.journal.Published = true
	if err := database.WriteAtomicPrivateFile(journalPath, mustJSON(move.journal)); err != nil {
		_ = move.rollback()
		return durableTrashMove{}, fmt.Errorf("record trash publication: %w", err)
	}
	return move, nil
}

func (move durableTrashMove) commit() error {
	if err := os.Remove(move.journalPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return database.SyncDirectory(filepath.Dir(move.journalPath))
}

func (move durableTrashMove) rollback() error {
	if move.server == nil {
		return move.rollbackWithProgress(func() {})
	}
	request := move.server.storageRequestForPath(context.Background(), foundationcontract.WorkClassMaintenance, move.journal.Source, "trash rollback")
	request.RecoveryProbe = true
	return move.server.boundedStorageProgressIO(context.Background(), request, move.rollbackWithProgress)
}

func (move durableTrashMove) rollbackWithProgress(progress func()) error {
	if _, err := os.Stat(move.journal.Target); err == nil {
		if _, sourceErr := os.Lstat(move.journal.Source); sourceErr == nil {
			sourceHash, sourceHashErr := fileDigestWithProgress(move.journal.Source, progress)
			targetHash, targetHashErr := fileDigestWithProgress(move.journal.Target, progress)
			if sourceHashErr != nil || targetHashErr != nil || sourceHash != targetHash {
				return errors.New("refusing to overwrite a recreated source while rolling back trash move")
			}
			if err := os.Remove(move.journal.Target); err != nil {
				return err
			}
			if err := database.SyncDirectory(filepath.Dir(move.journal.Target)); err != nil {
				return err
			}
			return move.commit()
		} else if !os.IsNotExist(sourceErr) {
			return sourceErr
		}
		info, statErr := os.Stat(move.journal.Target)
		if statErr != nil {
			return statErr
		}
		if _, err := copyPublishAndRemoveWithProgress(move.journal.Target, move.journal.Source, info.Size(), progress); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return move.commit()
}

func copyPublishAndRemove(source, target string, expectedSize int64) (string, error) {
	return copyPublishAndRemoveWithProgress(source, target, expectedSize, func() {})
}

func copyPublishAndRemoveWithProgress(source, target string, expectedSize int64, progress func()) (string, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", err
	}
	available, _, err := filesystemSpace(filepath.Dir(target))
	if err != nil {
		return "", fmt.Errorf("inspect trash capacity: %w", err)
	}
	if available < expectedSize+artifactCapacityHeadroom {
		return "", fmt.Errorf("trash destination requires %d bytes but only %d are available", expectedSize+artifactCapacityHeadroom, available)
	}
	sourceHash, err := fileDigestWithProgress(source, progress)
	if err != nil {
		return "", err
	}
	temp, err := os.OpenFile(target+".partial", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(temp.Name())
		}
	}()
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	written, copyErr := copyWithProgress(temp, in, progress)
	closeInErr := in.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeInErr != nil {
		return "", closeInErr
	}
	if written != expectedSize {
		return "", fmt.Errorf("trash copy was short: copied %d of %d bytes", written, expectedSize)
	}
	if err := temp.Sync(); err != nil {
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := database.ReplaceFileAtomically(temp.Name(), target); err != nil {
		return "", err
	}
	removeTemp = false
	if err := database.SyncDirectory(filepath.Dir(target)); err != nil {
		return "", err
	}
	targetHash, err := fileDigestWithProgress(target, progress)
	if err != nil {
		return "", err
	}
	if targetHash != sourceHash {
		return "", errors.New("trash copy checksum did not match source")
	}
	if err := os.Remove(source); err != nil {
		return "", err
	}
	if err := database.SyncDirectory(filepath.Dir(source)); err != nil {
		return "", err
	}
	return targetHash, nil
}

func fileDigest(path string) (string, error) {
	return fileDigestWithProgress(path, func() {})
}

func fileDigestWithProgress(path string, progress func()) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := copyWithProgress(hash, file, progress); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyWithProgress(dst io.Writer, src io.Reader, progress func()) (int64, error) {
	buffer := make([]byte, 256*1024)
	var written int64
	for {
		read, readErr := src.Read(buffer)
		if read > 0 {
			count, writeErr := dst.Write(buffer[:read])
			written += int64(count)
			progress()
			if writeErr != nil {
				return written, writeErr
			}
			if count != read {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func mustJSON(value any) []byte {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}

func (s *Server) reconcileFilesystemPublications(ctx context.Context) {
	if err := s.reconcileTrashMoves(ctx); err != nil {
		s.log.Warn("trash move reconciliation failed", "error", err)
	}
	if err := s.reconcileSubtitleArtifacts(ctx); err != nil {
		s.log.Warn("subtitle artifact reconciliation failed", "error", err)
	}
	if err := s.reconcileWaveformArtifacts(ctx); err != nil {
		s.log.Warn("waveform artifact reconciliation failed", "error", err)
	}
}

func (s *Server) reconcileTrashMoves(ctx context.Context) error {
	root := filepath.Join(s.cfg.AppDataDir, "media-trash")
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".partial") {
			return os.Remove(path)
		}
		if !strings.HasSuffix(entry.Name(), ".move.json") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var journal trashMoveJournal
		if json.Unmarshal(body, &journal) != nil || journal.Version != 1 || journal.Source == "" || journal.Target == "" {
			return os.Rename(path, path+".invalid")
		}
		if !pathInsideRoot(journal.Target, root) || filepath.Clean(path) != filepath.Clean(journal.Target)+".move.json" {
			return os.Rename(path, path+".invalid")
		}
		move := durableTrashMove{journalPath: path, journal: journal, server: s}
		var references int
		err = s.queryBackgroundRow(ctx, `SELECT COUNT(*) FROM media_files WHERE path IN (?, ?)`, journal.Source, "file://"+journal.Source).Scan(&references)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if references > 0 {
			return move.rollback()
		}
		return move.commit()
	})
}

func (s *Server) reconcileSubtitleArtifacts(ctx context.Context) error {
	return s.reconcileSubtitleArtifactsScoped(ctx, "")
}

func (s *Server) reconcileSubtitleArtifactsAfterScan(ctx context.Context, libraryID string) error {
	return s.reconcileSubtitleArtifactsScoped(ctx, strings.TrimSpace(libraryID))
}

func (s *Server) reconcileSubtitleArtifactsScoped(ctx context.Context, immediateLibraryID string) error {
	root := filepath.Join(s.cfg.AppDataDir, "subtitles")
	staleBefore := time.Now().UTC().Add(-artifactOrphanGrace)
	entries := map[string]struct{}{}
	immediateMedia := map[string]struct{}{}
	rows, err := s.queryBackgroundRead(ctx, `SELECT media_id, COALESCE(NULLIF(storage_key, ''), id) FROM media_streams WHERE kind = 'subtitle' AND source_url <> ''`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var mediaID, storageKey string
		if err := rows.Scan(&mediaID, &storageKey); err != nil {
			rows.Close()
			return err
		}
		entries[filepath.Join(root, safePathComponent(mediaID), safePathComponent(storageKey)+".vtt")] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if immediateLibraryID != "" {
		mediaRows, err := s.queryBackgroundRead(ctx, `SELECT id FROM media_items WHERE library_id = ?`, immediateLibraryID)
		if err != nil {
			return err
		}
		for mediaRows.Next() {
			var mediaID string
			if err := mediaRows.Scan(&mediaID); err != nil {
				mediaRows.Close()
				return err
			}
			immediateMedia[safePathComponent(mediaID)] = struct{}{}
		}
		if err := mediaRows.Close(); err != nil {
			return err
		}
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		stale := info.ModTime().UTC().Before(staleBefore)
		if entry.IsDir() {
			if strings.HasSuffix(entry.Name(), ".reserve") && stale {
				return os.RemoveAll(path)
			}
			return nil
		}
		name := entry.Name()
		if stale && (strings.Contains(name, ".tmp-") || strings.HasSuffix(name, ".partial")) {
			return os.Remove(path)
		}
		immediate := false
		if immediateLibraryID != "" {
			if rel, relErr := filepath.Rel(root, path); relErr == nil {
				parts := strings.Split(filepath.ToSlash(rel), "/")
				_, immediate = immediateMedia[parts[0]]
			}
		}
		if (stale || immediate) && strings.HasSuffix(name, ".vtt") {
			if _, ok := entries[path]; !ok {
				return os.Remove(path)
			}
		}
		return nil
	})
}
