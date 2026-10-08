package administration

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
	"runtime"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
)

// File operations use a write-ahead filesystem manifest and the SQL operation
// receipt as their commit record. Before SQL commits every move is reversible;
// irreversible removal runs only after that receipt is durable. A restart or
// ambiguous Commit error reads the receipt before choosing either direction.
// This is a recovery manifest, not a second catalog or settings store.
type fileIOObserverKey struct{}

func observeFileIO(ctx context.Context) {
	if fn, ok := ctx.Value(fileIOObserverKey{}).(func()); ok {
		fn()
	}
}

type fileMove struct{ From, To, Temporary, DestinationIdentity string }
type fileOperation struct {
	Version                    int
	TrashEntryID               string `json:",omitempty"`
	Scope, OperationID, Digest string
	Moves                      []fileMove
	RemoveAfterCommit          []string
	path                       string
}

// Persist each newly-created directory entry before moving the only source
// copy beneath it. Syncing the child alone does not persist its parent entry.
func durableMkdirAll(path string, mode os.FileMode) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return err
	}
	if err = durableMkdirAll(parent, mode); err != nil {
		return err
	}
	if err = os.Mkdir(path, mode); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return syncDirectory(parent)
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	} // Windows cannot fsync a directory handle via os.File.
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func (j *fileOperation) save() error {
	if err := durableMkdirAll(filepath.Dir(j.path), 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(j)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(j.path), ".stage-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, j.path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(j.path))
}
func (j *fileOperation) move(ctx context.Context, from, to string) error {
	observeFileIO(ctx)
	sum := sha256.Sum256([]byte(j.Scope + "\x00" + j.OperationID + "\x00" + fmt.Sprint(len(j.Moves))))
	temporary := filepath.Join(filepath.Dir(to), ".portico-move-"+hex.EncodeToString(sum[:]))
	j.Moves = append(j.Moves, fileMove{From: from, To: to, Temporary: temporary})
	if err := j.save(); err != nil {
		return err
	}
	published := false
	err := moveFileWithHooks(ctx, from, to, temporary, func(path string) error {
		identity, err := fileIdentity(path)
		if err != nil {
			return err
		}
		j.Moves[len(j.Moves)-1].DestinationIdentity = identity
		return j.save()
	}, func() { published = true })
	if err != nil && !published {
		// No destination was ever installed by this attempt. In particular, a
		// concurrent file at To belongs to its creator, even when bytes are equal.
		j.Moves = j.Moves[:len(j.Moves)-1]
		return errors.Join(err, j.save())
	}
	return err
}
func (j *fileOperation) removeAfterCommit(path string) error {
	j.RemoveAfterCommit = append(j.RemoveAfterCommit, path)
	return j.save()
}
func sameContents(a, b string) (bool, error) {
	fa, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer fa.Close()
	fb, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer fb.Close()
	ai, err := fa.Stat()
	if err != nil {
		return false, err
	}
	bi, err := fb.Stat()
	if err != nil {
		return false, err
	}
	if !ai.Mode().IsRegular() || !bi.Mode().IsRegular() {
		return false, ErrConflict
	}
	if os.SameFile(ai, bi) {
		return true, nil
	}
	if ai.Size() != bi.Size() {
		return false, nil
	}
	ah, bh := sha256.New(), sha256.New()
	if _, err = io.Copy(ah, fa); err != nil {
		return false, err
	}
	if _, err = io.Copy(bh, fb); err != nil {
		return false, err
	}
	return string(ah.Sum(nil)) == string(bh.Sum(nil)), nil
}
func reverseMove(ctx context.Context, move fileMove) error {
	fromInfo, fromErr := os.Lstat(move.From)
	_, toErr := os.Lstat(move.To)
	if toErr != nil && !errors.Is(toErr, os.ErrNotExist) {
		return toErr
	}
	if fromErr != nil && !errors.Is(fromErr, os.ErrNotExist) {
		return fromErr
	}
	if errors.Is(toErr, os.ErrNotExist) {
		if fromErr == nil {
			return nil
		}
		return fmt.Errorf("both recovery paths unavailable: %w", ErrUnavailable)
	}
	if move.DestinationIdentity == "" {
		return fmt.Errorf("missing recovery ownership evidence: %w", ErrConflict)
	}
	identity, err := fileIdentity(move.To)
	if err != nil {
		return err
	}
	if identity != move.DestinationIdentity {
		return fmt.Errorf("recovery destination belongs to another file: %w", ErrConflict)
	}
	if fromErr == nil {
		if !fromInfo.Mode().IsRegular() {
			return ErrConflict
		}
		same, err := sameContents(move.From, move.To)
		if err != nil {
			return err
		}
		if !same {
			return fmt.Errorf("recovery original changed: %w", ErrConflict)
		}
		// Exclusive publication's persisted file identity proves ownership. Equal
		// bytes alone never authorize deleting another creator's destination.
		if err = os.Remove(move.To); err != nil {
			return err
		}
		return syncDirectory(filepath.Dir(move.To))
	}
	return moveFileContext(ctx, move.To, move.From)
}
func (s *Service) finishFileOperation(ctx context.Context, j *fileOperation) error {
	if _, err := os.Stat(j.path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	gated, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return err
	}
	defer gated.Rollback()
	var committed bool
	if j.Scope == "job-trash" {
		if j.TrashEntryID == "" {
			return ErrUnavailable
		}
		err = gated.Tx().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_trash WHERE id=? AND operation_id=?)`, j.TrashEntryID, j.OperationID).Scan(&committed)
	} else {
		var digest string
		err = gated.Tx().QueryRowContext(ctx, `SELECT digest FROM admin_receipts WHERE scope=? AND operation_id=?`, j.Scope, j.OperationID).Scan(&digest)
		committed = err == nil && digest == j.Digest
		if err == nil && !committed {
			return ErrConflict
		}
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	// Commit evidence is immutable while the caller holds s.files. Release the
	// snapshot before copying/removing files; recovery must not hold a writer.
	gated.Rollback()
	observeFileIO(ctx)
	if committed {
		for _, path := range j.RemoveAfterCommit {
			if err = os.RemoveAll(path); err != nil {
				return err
			}
			if err = syncDirectory(filepath.Dir(path)); err != nil {
				return err
			}
		}
	} else {
		for i := len(j.Moves) - 1; i >= 0; i-- {
			if err = reverseMove(ctx, j.Moves[i]); err != nil {
				return err
			}
		}
	}
	for _, move := range j.Moves {
		if move.Temporary != "" {
			if err = os.Remove(move.Temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	if err = os.Remove(j.path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(j.path))
}
func (s *Service) fileJournal(scope, operationID, digest string) *fileOperation {
	sum := sha256.Sum256([]byte(scope + "\x00" + operationID))
	return &fileOperation{Version: 1, Scope: scope, OperationID: operationID, Digest: digest, path: filepath.Join(s.StateDirectory(), "file-operations", hex.EncodeToString(sum[:])+".json")}
}
func (s *Service) recoverFileOperations(ctx context.Context) error {
	root := filepath.Join(s.StateDirectory(), "file-operations")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if !entry.Type().IsRegular() {
			return ErrUnavailable
		}
		raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return err
		}
		var j fileOperation
		if len(raw) > 16<<20 || json.Unmarshal(raw, &j) != nil || j.Version != 1 || !oneOf(j.Scope, "media-delete", "trash-restore", "trash-empty", "job-trash") || j.OperationID == "" || !validOperationID(j.OperationID) {
			return ErrUnavailable
		}
		j.path = filepath.Join(root, entry.Name())
		if err = s.finishFileOperation(ctx, &j); err != nil {
			return fmt.Errorf("recover %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// RecoverFileOperations must run after opening the database, before any worker
// can publish catalog changes. Request paths also call it before their next move.
func (s *Service) RecoverFileOperations(ctx context.Context) error {
	if s.StateDirectory() == "" {
		return ErrUnavailable
	}
	s.files.Lock()
	defer s.files.Unlock()
	return s.recoverFileOperations(ctx)
}
func (s *Service) settleFileOperation(j *fileOperation, problem error) error {
	// Request cancellation does not cancel recovery of bytes already moved.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return errors.Join(problem, s.finishFileOperation(ctx, j))
}

// No overwrite, including a restore racing with a newly-created original path.
// Cross-volume copies retain the source until a complete fsynced destination
// exists. A partial copy is private and can never be mistaken for restored media.
func moveFileContext(ctx context.Context, from, to string) error {
	return moveFileWithTemporary(ctx, from, to, "")
}
func moveFileWithTemporary(ctx context.Context, from, to, temporary string) error {
	return moveFileWithHooks(ctx, from, to, temporary, nil, nil)
}
func moveFileWithHooks(ctx context.Context, from, to, temporary string, beforePublish func(string) error, afterPublish func()) error {
	return moveFileWithLink(ctx, from, to, temporary, beforePublish, afterPublish, os.Link)
}
func moveFileWithLink(ctx context.Context, from, to, temporary string, beforePublish func(string) error, afterPublish func(), link func(string, string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrDenied
	}
	if err = durableMkdirAll(filepath.Dir(to), 0700); err != nil {
		return err
	}
	if beforePublish != nil {
		if err = beforePublish(from); err != nil {
			return err
		}
	}
	if err = link(from, to); err == nil {
		if afterPublish != nil {
			afterPublish()
		}
		if err = syncDirectory(filepath.Dir(to)); err != nil {
			return err
		}
		linked, inspectErr := os.Lstat(to)
		if inspectErr != nil {
			return inspectErr
		}
		original, inspectErr := os.Lstat(from)
		if inspectErr != nil {
			return inspectErr
		}
		if !os.SameFile(info, linked) || !os.SameFile(info, original) {
			return ErrConflict
		}
		if err = os.Remove(from); err != nil {
			return err
		}
		return syncDirectory(filepath.Dir(from))
	}
	if _, err = os.Lstat(to); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	current, err := source.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) {
		return ErrConflict
	}
	var destination *os.File
	if temporary == "" {
		destination, err = os.CreateTemp(filepath.Dir(to), ".portico-move-*")
	} else {
		destination, err = os.OpenFile(temporary, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	}
	if err != nil {
		return err
	}
	temporary = destination.Name()
	defer os.Remove(temporary)
	defer destination.Close()
	buffer := make([]byte, 1024*1024)
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			if _, err = destination.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	// Preserve source facts on a cross-volume restore: the catalog and immutable
	// media readers fence the original size/modification time.
	if err = destination.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err = os.Chtimes(temporary, info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	if err = destination.Sync(); err != nil {
		return err
	}
	if err = destination.Close(); err != nil {
		return err
	}
	current, err = source.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) || info.Size() != current.Size() || !info.ModTime().Equal(current.ModTime()) {
		return ErrConflict
	}
	// Exclusive publication cannot overwrite a
	// file that appeared after the preceding existence check.
	if beforePublish != nil {
		if err = beforePublish(temporary); err != nil {
			return err
		}
	}
	if err = renameExclusive(temporary, to); err != nil {
		return err
	}
	if afterPublish != nil {
		afterPublish()
	}
	if err = syncDirectory(filepath.Dir(to)); err != nil {
		return err
	}
	current, err = os.Lstat(from)
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) || info.Size() != current.Size() || !info.ModTime().Equal(current.ModTime()) {
		return ErrConflict
	}
	if err = os.Remove(from); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(from))
}
