package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

// RestoreSpaceError is stable API-safe admission evidence. The preflight is
// advisory (filesystem allocation can change after it returns), while every
// subsequent write remains transactional and cleans/quarantines its partial
// artifact on ENOSPC.
type RestoreSpaceError struct {
	Path      string
	Required  uint64
	Available uint64
}

func (e *RestoreSpaceError) Error() string {
	return fmt.Sprintf("restore requires more free space on %s (required=%d available=%d)", filepath.Dir(e.Path), e.Required, e.Available)
}

// RestoreSpaceAdmission is retained for callers that do not have a source
// path. The supervised upload API requires a declared size; this wrapper is
// for internal callers that already know the exact source size.
func RestoreSpaceAdmission(cfg config.Config, sourceBytes int64) error {
	return RestoreSpaceAdmissionForPath(cfg, "", sourceBytes)
}

// RestoreSpaceAdmissionForPath performs a conservative peak-allocation
// calculation. During a replacement the destination may contain all of the
// following at once: the staged database, the logical safety snapshot of the
// active database, and an install/quarantine temporary copy. Paths that
// resolve to the same filesystem are grouped before comparing the aggregate
// requirement with free space; checking staging and active directories
// independently would under-admit the normal same-volume layout.
func RestoreSpaceAdmissionForPath(cfg config.Config, sourcePath string, sourceBytes int64) error {
	return restoreSpaceAdmissionWithProbe(cfg, sourcePath, sourceBytes, restoreSpaceProbe{
		volume: restoreVolumeKey,
		free:   restoreFreeBytes,
		active: statRegularRestoreFile,
	})
}

type restoreSpaceProbe struct {
	volume func(string) (string, error)
	free   func(string) (uint64, error)
	active func(string) (int64, error)
}

func restoreSpaceAdmissionWithProbe(cfg config.Config, sourcePath string, sourceBytes int64, probe restoreSpaceProbe) error {
	if sourceBytes <= 0 {
		return errors.New("restore source size must be positive")
	}
	maximum := cfg.RestoreMaxDatabaseBytes
	if maximum <= 0 {
		maximum = RestoreMaxDatabaseBytes
	}
	if sourceBytes > maximum {
		return &RestoreSpaceError{Path: cfg.DatabasePath, Required: uint64(sourceBytes), Available: 0}
	}
	if probe.volume == nil || probe.free == nil || probe.active == nil {
		return errors.New("restore space probe is incomplete")
	}
	activeBytes := int64(0)
	if info, err := probe.active(cfg.DatabasePath); err == nil && info > 0 {
		activeBytes = info
	}
	stagingPath := filepath.Join(cfg.AppDataDir, "restore")
	activePath := filepath.Dir(cfg.DatabasePath)
	const margin = int64(128 << 20)
	needs := map[string]*restoreSpaceNeed{}
	add := func(path string, bytes int64) error {
		if bytes < 0 || sourceBytes > maximum {
			return errors.New("restore space requirement overflow")
		}
		volume, err := probe.volume(path)
		if err != nil {
			return fmt.Errorf("identify restore storage volume: %w", err)
		}
		volume = strings.TrimSpace(volume)
		if volume == "" {
			return errors.New("identify restore storage volume: empty volume identity")
		}
		need := needs[volume]
		if need == nil {
			need = &restoreSpaceNeed{path: path}
			needs[volume] = need
		}
		if bytes > 0 && need.required > (1<<63-1)-bytes {
			return errors.New("restore space requirement overflow")
		}
		need.required += bytes
		return nil
	}
	// A source backup is already accounted for by the filesystem's free-byte
	// count. Only newly allocated copies are added below. The private restore
	// volume now retains both the staged source and the logical safety snapshot,
	// so it needs staged + active. The active volume retains the original active
	// set while the install is prepared and then retains the failed/restored set
	// while rollback can copy the safety snapshot into a fresh install; the
	// additional active-side allocation is staged + active. When both roots
	// share a volume these add to 2*staged + 2*active, including rollback
	// headroom.
	if activeBytes > (1<<63-1)-sourceBytes {
		return errors.New("restore space requirement overflow")
	}
	if err := add(stagingPath, sourceBytes+activeBytes); err != nil {
		return err
	}
	if err := add(activePath, sourceBytes+activeBytes); err != nil {
		return err
	}
	for volume, need := range needs {
		if need.required > (1<<63-1)-margin {
			return errors.New("restore space requirement overflow")
		}
		need.required += margin
		available, err := probe.free(need.path)
		if err != nil {
			// Space is advisory. An unavailable free-space probe must not make a
			// valid restore impossible; the writes below still fail transactionally.
			continue
		}
		if uint64(need.required) > available {
			_ = volume
			return &RestoreSpaceError{Path: need.path, Required: uint64(need.required), Available: available}
		}
	}
	return nil
}

type restoreSpaceNeed struct {
	path     string
	required int64
}

func statRegularRestoreFile(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0, err
	}
	return info.Size(), nil
}
