package persistence

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UpgradeCopy struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Bytes     int64  `json:"bytes"`
	Path      string `json:"path"`
}
type StorageStatus struct {
	DatabasePath       string        `json:"databasePath"`
	CacheDirectory     string        `json:"cacheDirectory"`
	BackupDirectory    string        `json:"backupDirectory"`
	ReclaimableBytes   int64         `json:"reclaimableBytes"`
	LastReclaimedAt    string        `json:"lastReclaimedAt,omitempty"`
	LastReclaimedBytes int64         `json:"lastReclaimedBytes"`
	UpgradeCopies      []UpgradeCopy `json:"upgradeCopies"`
}

func databasePath(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA database_list")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		var name, path string
		if err = rows.Scan(&n, &name, &path); err != nil {
			return "", err
		}
		if name == "main" {
			return path, nil
		}
	}
	return "", rows.Err()
}
func upgradeCopyTime(id string) (time.Time, error) {
	if !strings.HasPrefix(id, "before-") || !strings.HasSuffix(id, ".sqlite") || filepath.Base(id) != id {
		return time.Time{}, os.ErrInvalid
	}
	return time.Parse("20060102T150405.000000000Z", strings.TrimSuffix(strings.TrimPrefix(id, "before-"), ".sqlite"))
}
func ReadStorageStatus(ctx context.Context, db *sql.DB) (StorageStatus, error) {
	out := StorageStatus{UpgradeCopies: []UpgradeCopy{}}
	var err error
	out.DatabasePath, err = databasePath(ctx, db)
	if err != nil {
		return out, err
	}
	var pages, size, last int64
	if err = db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&pages); err != nil {
		return out, err
	}
	if err = db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&size); err != nil {
		return out, err
	}
	out.ReclaimableBytes = pages * size
	if err = db.QueryRowContext(ctx, `SELECT last_ran_ms,last_reclaimed_bytes FROM database_maintenance_status WHERE singleton=1`).Scan(&last, &out.LastReclaimedBytes); err != nil {
		return out, err
	}
	if last > 0 {
		out.LastReclaimedAt = time.UnixMilli(last).UTC().Format(time.RFC3339)
	}
	if out.DatabasePath == "" {
		return out, nil
	}
	directory := filepath.Join(filepath.Dir(out.DatabasePath), "migration-backups")
	dir, err := os.Open(directory)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer dir.Close()
	// Startup keeps three copies; the bound also protects an externally filled folder.
	entries, err := dir.ReadDir(257)
	if err != nil && !errors.Is(err, io.EOF) {
		return out, err
	}
	if len(entries) > 256 {
		return out, errors.New("too many upgrade copies")
	}
	for _, entry := range entries {
		when, e := upgradeCopyTime(entry.Name())
		if e != nil {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return out, e
		}
		if !info.Mode().IsRegular() {
			continue
		}
		out.UpgradeCopies = append(out.UpgradeCopies, UpgradeCopy{entry.Name(), when.UTC().Format(time.RFC3339Nano), info.Size(), filepath.Join(directory, entry.Name())})
	}
	return out, nil
}
func DeleteUpgradeCopy(ctx context.Context, db *sql.DB, id string) error {
	if _, err := upgradeCopyTime(id); err != nil {
		return os.ErrInvalid
	}
	path, err := databasePath(ctx, db)
	if err != nil {
		return err
	}
	if path == "" {
		return os.ErrNotExist
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	directory, err := root.OpenRoot("migration-backups")
	if err != nil {
		return err
	}
	defer directory.Close()
	info, err := directory.Lstat(id)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return os.ErrInvalid
	}
	return directory.Remove(id)
}
