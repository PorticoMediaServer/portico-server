package database

import (
	"path/filepath"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestRestoreSpaceAdmissionAggregatesSameVolumePeak(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{AppDataDir: filepath.Join(root, "app"), DatabasePath: filepath.Join(root, "app", "portico.db"), RestoreMaxDatabaseBytes: 1 << 30}
	const source, active = int64(100), int64(80)
	probe := restoreSpaceProbe{
		volume: func(string) (string, error) { return "same", nil },
		free:   func(string) (uint64, error) { return uint64(2*source + 2*active + 128<<20 - 1), nil },
		active: func(string) (int64, error) { return active, nil },
	}
	if err := restoreSpaceAdmissionWithProbe(cfg, filepath.Join(root, "backup.db"), source, probe); err == nil {
		t.Fatal("same-volume peak was under-admitted")
	}
	probe.free = func(string) (uint64, error) { return uint64(2*source + 2*active + 128<<20), nil }
	if err := restoreSpaceAdmissionWithProbe(cfg, filepath.Join(root, "backup.db"), source, probe); err != nil {
		t.Fatalf("same-volume exact peak rejected: %v", err)
	}
}

func TestRestoreSpaceAdmissionKeepsSplitVolumesIndependent(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{AppDataDir: filepath.Join(root, "app"), DatabasePath: filepath.Join(root, "db", "portico.db"), RestoreMaxDatabaseBytes: 1 << 30}
	const source, active = int64(100), int64(80)
	free := map[string]uint64{"stage": uint64(source + active + 128<<20), "active": uint64(source + active + 128<<20)}
	probe := restoreSpaceProbe{
		volume: func(path string) (string, error) {
			if path == filepath.Join(root, "db") {
				return "active", nil
			}
			return "stage", nil
		},
		free: func(path string) (uint64, error) {
			if path == filepath.Join(root, "db") {
				return free["active"], nil
			}
			return free["stage"], nil
		},
		active: func(string) (int64, error) { return active, nil },
	}
	if err := restoreSpaceAdmissionWithProbe(cfg, filepath.Join(root, "backup.db"), source, probe); err != nil {
		t.Fatalf("split-volume peak rejected: %v", err)
	}
}
