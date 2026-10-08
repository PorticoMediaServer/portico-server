package backup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	State         string   `json:"state"`
	Source        Source   `json:"source"`
	SchemaVersion int      `json:"schemaVersion"`
	Files         []string `json:"files"`
	PreRestore    string   `json:"preRestore,omitempty"`
	// Reason is why a rolling_back restore is being rolled back.
	Reason string `json:"reason,omitempty"`
}

const (
	markerStaged      = "staged"
	markerPreparing   = "preparing"
	markerSwitching   = "switching"
	markerApplied     = "applied"
	markerRollingBack = "rolling_back"
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
	s.stageMu.Lock()
	defer s.stageMu.Unlock()
	// Staging replaces restore-staged; never while it holds a restore that has
	// started moving files.
	if RestoreUnresolved(s.state) {
		return out, ErrRestorePending
	}
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
	if _, err = copyOneFile(srcDB, filepath.Join(incoming, DatabaseName), DatabaseName); err != nil {
		return out, ErrSourceInvalid
	}
	if _, err = os.Stat(srcDB + "-wal"); err == nil {
		_, _ = copyOneFile(srcDB+"-wal", filepath.Join(incoming, DatabaseName+"-wal"), DatabaseName+"-wal")
	}
	for _, name := range files {
		if _, err = copyOneFile(filepath.Join(srcDir, filepath.FromSlash(name)), filepath.Join(incoming, filepath.FromSlash(name)), name); err != nil {
			return out, ErrSourceInvalid
		}
	}
	schemaVersion, err := persistence.InspectCandidate(ctx, filepath.Join(incoming, DatabaseName))
	if err != nil {
		return out, err
	}
	marker := restoreMarker{State: markerStaged, Source: source, SchemaVersion: schemaVersion, Files: files}
	raw, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return out, err
	}
	if err = os.WriteFile(filepath.Join(incoming, RestoreMarkerFile), append(raw, '\n'), 0600); err != nil {
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
	for _, entry := range files {
		if entry.Name == ManifestName || strings.Contains(entry.Name, "\\") || strings.HasPrefix(entry.Name, "/") || strings.Contains(entry.Name, "../") {
			return nil, false
		}
		if entry.Name == DatabaseName || entry.Name == DatabaseName+"-wal" {
			continue
		}
		names = append(names, entry.Name)
	}
	return names, true
}

// resolveSource maps a restore source to its database file, the directory its
// state files live in, and those files' relative names.
func (s *Service) resolveSource(source Source) (db, dir string, files []string, err error) {
	if source.BackupID != "" {
		if err = checkID(source.BackupID); err != nil {
			return "", "", nil, ErrSourceInvalid
		}
		folder := filepath.Join(s.state, BackupsDir, source.BackupID)
		manifest, err := readManifest(folder)
		if err != nil {
			return "", "", nil, ErrSourceInvalid
		}
		if err = checkManifestHashes(folder, manifest.Files); err != nil {
			return "", "", nil, err
		}
		names, ok := stateFileNames(manifest.Files)
		if !ok {
			return "", "", nil, ErrSourceInvalid
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
		if err = checkManifestHashes(path, manifest.Files); err != nil {
			return "", "", nil, err
		}
		names, ok := stateFileNames(manifest.Files)
		if !ok {
			return "", "", nil, ErrSourceInvalid
		}
		return filepath.Join(path, DatabaseName), path, names, nil
	}
	if !info.Mode().IsRegular() {
		return "", "", nil, ErrSourceInvalid
	}
	return path, "", nil, nil
}
