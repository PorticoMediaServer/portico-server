package backup

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"portico.local/server/internal/persistence"
)

// Source selects the restore source: one listed backup, or a path on the
// server. A path is a backup folder (with a manifest) or a bare .db file that
// is a Portico database. Exactly one of the two is set.
type Source struct {
	BackupID string `json:"backupId,omitempty"`
	Path     string `json:"path,omitempty"`
}

// Validation summarises the staged restore's checks for the reply.
type Validation struct {
	SchemaVersion int    `json:"schemaVersion"`
	Integrity     string `json:"integrity"`
}

// StageResult answers a staged restore: restart to apply it.
type StageResult struct {
	Staged          bool       `json:"staged"`
	RestartRequired bool       `json:"restartRequired"`
	Validation      Validation `json:"validation"`
}

// restoreMarker is restore-staged/restore.json: the one staged-restore marker.
// Its states and how each resumes after a crash are described on ApplyStaged.
type restoreMarker struct {
	OperationID   string   `json:"operationId,omitempty"`
	JobID         string   `json:"jobId,omitempty"`
	State         string   `json:"state"`
	Source        Source   `json:"source"`
	SchemaVersion int      `json:"schemaVersion"`
	Files         []string `json:"files"`
	PreRestore    string   `json:"preRestore,omitempty"`
	// Reason is why a rolling_back restore is being rolled back.
	Reason      string `json:"reason,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
}

const (
	markerStaged      = "staged"
	markerPreparing   = "preparing"
	markerSwitching   = "switching"
	markerApplied     = "applied"
	markerRollingBack = "rolling_back"
	markerRestored    = "restored"
	markerRolledBack  = "rolled_back"
)

// Stage validates the source and copies it into restore-staged/.incoming,
// writes the marker, and renames it into place. It answers the validation
// summary and, through the injected restart func, asks the server to restart
// so the next start applies the staged copy.
func (s *Service) Stage(ctx context.Context, source Source) (StageResult, error) {
	var out StageResult
	if (source.BackupID == "") == (source.Path == "") {
		return out, ErrSourceInvalid
	}
	if s.restoreUnavailable {
		return out, ErrRestoreLockUnavailable
	}
	s.stageMu.Lock()
	defer s.stageMu.Unlock()
	// Staging replaces restore-staged; never while it holds a restore that has
	// started moving files.
	if RestoreUnresolved(s.state) {
		return out, ErrRestorePending
	}
	s.restoreProgress("verify-files", 0, 0)
	srcDB, srcDir, files, err := s.resolveSource(source)
	if err != nil {
		return out, err
	}
	var estimate int64
	if info, err := os.Stat(srcDB); err != nil || !info.Mode().IsRegular() {
		return out, ErrSourceInvalid
	} else {
		estimate = info.Size()
	}
	if wal, err := os.Stat(srcDB + "-wal"); err == nil && wal.Mode().IsRegular() {
		estimate += wal.Size()
	}
	for _, name := range files {
		if info, err := os.Stat(filepath.Join(srcDir, filepath.FromSlash(name))); err != nil || !info.Mode().IsRegular() {
			return out, ErrSourceInvalid
		} else {
			estimate += info.Size()
		}
	}
	if err = checkFreeSpace(s.state, estimate); err != nil {
		return out, err
	}
	staged := filepath.Join(s.state, RestoreStagedDir)
	incoming := filepath.Join(staged, ".incoming")
	complete := filepath.Join(s.state, ".staged-complete")
	// A previous staged restore is superseded. The marker only ever appears
	// at restore-staged/restore.json, so a crash anywhere below leaves either
	// the previous staged copy or nothing behind.
	_ = os.RemoveAll(complete)
	_ = os.RemoveAll(staged)
	if err = os.MkdirAll(incoming, 0700); err != nil {
		return out, err
	}
	failed := true
	defer func() {
		if failed {
			_ = os.RemoveAll(incoming)
		}
	}()
	if _, err = copyOneFileContext(ctx, srcDB, filepath.Join(incoming, DatabaseName), DatabaseName, func(done, total int64) { s.restoreProgress("copy", done, total) }); err != nil {
		return out, ErrSourceInvalid
	}
	if wal, statErr := os.Lstat(srcDB + "-wal"); statErr == nil {
		if !wal.Mode().IsRegular() {
			return out, ErrSourceInvalid
		}
		if _, err = copyOneFileContext(ctx, srcDB+"-wal", filepath.Join(incoming, DatabaseName+"-wal"), DatabaseName+"-wal", nil); err != nil {
			return out, ErrSourceInvalid
		}
	} else if !os.IsNotExist(statErr) {
		return out, ErrSourceInvalid
	}
	for _, name := range files {
		if _, err = copyOneFile(filepath.Join(srcDir, filepath.FromSlash(name)), filepath.Join(incoming, filepath.FromSlash(name)), name); err != nil {
			return out, ErrSourceInvalid
		}
	}
	s.restoreProgress("integrity", estimate, estimate)
	schemaVersion, err := persistence.InspectCandidate(ctx, filepath.Join(incoming, DatabaseName))
	if err != nil {
		return out, err
	}
	if err = syncRestoreDirectories(incoming, incoming, files); err != nil {
		return out, err
	}
	marker := restoreMarker{State: markerStaged, Source: source, SchemaVersion: schemaVersion, Files: files}
	if progress := s.RestoreProgress(); progress != nil {
		marker.JobID = progress.JobID
		s.mu.Lock()
		marker.OperationID = s.restoreOperation
		s.mu.Unlock()
	}
	if err = writeMarker(incoming, marker); err != nil {
		return out, err
	}
	if err = os.MkdirAll(filepath.Dir(staged), 0700); err != nil {
		return out, err
	}
	if err = os.Rename(incoming, complete); err != nil {
		return out, err
	}
	// incoming is out of staged now; drop the emptied husk so the final
	// rename lands on a clean path on every platform.
	if err = os.RemoveAll(staged); err != nil {
		_ = os.RemoveAll(complete)
		return out, err
	}
	if err = os.Rename(complete, staged); err != nil {
		_ = os.RemoveAll(complete)
		return out, err
	}
	if err = syncDirectory(s.state); err != nil {
		return out, err
	}
	failed = false
	out = StageResult{Staged: true, RestartRequired: true, Validation: Validation{SchemaVersion: schemaVersion, Integrity: "ok"}}
	if s.onStaged != nil {
		s.onStaged()
	}
	return out, nil
}

// stateFileNames returns a manifest's state files: every entry except the
// database, which restore stages on its own. An ordinary backup lists only state
// files; a pre-restore backup (made by moving the live files) also lists the
// database and its WAL, which checkManifestHashes has already verified.
func stateFileNames(files []FileEntry) ([]string, bool) {
	names := make([]string, 0, len(files))
	seen := make(map[string]bool, len(files))
	for _, entry := range files {
		name := entry.Name
		if !safeRelativeFile(name) || !restorableFile(name) {
			return nil, false
		}
		key := name
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			return nil, false
		}
		seen[key] = true
		if name == DatabaseName || name == DatabaseName+"-wal" {
			continue
		}
		names = append(names, name)
	}
	return names, true
}

// Only files owned by the backup format may be installed in live state.
func safeRelativeFile(name string) bool {
	if name == "" || name == "." || name == ".." || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\:\x00") {
		return false
	}
	return runtime.GOOS != "windows" || safeWindowsBackupName(name)
}

// Windows normalizes some names and reserves device names in every directory.
// Refuse aliases before resolving a manifest entry inside the backup tree.
func safeWindowsBackupName(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if strings.Trim(part, " ") != part || strings.TrimRight(part, ".") != part || strings.ContainsAny(part, "<>\"\\|?*:") {
			return false
		}
		for _, r := range part {
			if r < 32 {
				return false
			}
		}
		stem, _, _ := strings.Cut(strings.ToUpper(part), ".")
		switch stem {
		case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "COM¹", "COM²", "COM³", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9", "LPT¹", "LPT²", "LPT³":
			return false
		}
	}
	return true
}

func restorableFile(name string) bool {
	if name == DatabaseName || name == DatabaseName+"-wal" {
		return true
	}
	for _, root := range stateFiles() {
		if name == root {
			return true
		}
		if root == "supervisor/authority-incarnation.v1" || root == "networking-tls.pem" {
			continue
		}
		if strings.HasPrefix(name, root+"/") {
			if root == "managed-storage" {
				return !strings.Contains(strings.TrimPrefix(name, root+"/"), "/") && strings.HasSuffix(name, ".conf")
			}
			return true
		}
	}
	return false
}

// resolveSource maps a restore source to its database file, the directory its
// state files live in, and those files' relative names.
func (s *Service) resolveSource(source Source) (db, dir string, files []string, err error) {
	if source.BackupID != "" {
		if err = checkID(source.BackupID); err != nil {
			return "", "", nil, ErrSourceInvalid
		}
		folder, err := s.backupPath(source.BackupID)
		if err != nil {
			return "", "", nil, ErrSourceInvalid
		}
		manifest, err := readManifest(folder)
		if err != nil {
			return "", "", nil, ErrSourceInvalid
		}
		names, ok := stateFileNames(manifest.Files)
		if !ok {
			return "", "", nil, ErrSourceInvalid
		}
		if err = checkManifestHashes(folder, manifest.Files); err != nil {
			return "", "", nil, err
		}
		return filepath.Join(folder, DatabaseName), folder, names, nil
	}
	path := source.Path
	if path == "" || !filepath.IsAbs(path) || path != filepath.Clean(path) {
		return "", "", nil, ErrSourceInvalid
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", "", nil, ErrSourceInvalid
	}
	if info.IsDir() {
		manifest, err := readManifest(path)
		if err != nil {
			return "", "", nil, ErrSourceInvalid
		}
		names, ok := stateFileNames(manifest.Files)
		if !ok {
			return "", "", nil, ErrSourceInvalid
		}
		if err = checkManifestHashes(path, manifest.Files); err != nil {
			return "", "", nil, err
		}
		return filepath.Join(path, DatabaseName), path, names, nil
	}
	if !info.Mode().IsRegular() {
		return "", "", nil, ErrSourceInvalid
	}
	return path, "", nil, nil
}
