package persistence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStorageStatusListsAndDeletesOnlyUpgradeCopies(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "portico.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	copies := filepath.Join(dir, "migration-backups")
	if err = os.Mkdir(copies, 0700); err != nil {
		t.Fatal(err)
	}
	name := "before-20261013T120000.000000000Z.sqlite"
	if err = os.WriteFile(filepath.Join(copies, name), []byte("backup"), 0600); err != nil {
		t.Fatal(err)
	}
	status, err := ReadStorageStatus(context.Background(), db)
	if err != nil || len(status.UpgradeCopies) != 1 || status.UpgradeCopies[0].Bytes != 6 {
		t.Fatal(status, err)
	}
	if err = DeleteUpgradeCopy(context.Background(), db, "../portico.db"); err == nil {
		t.Fatal("accepted traversal")
	}
	if err = DeleteUpgradeCopy(context.Background(), db, name); err != nil {
		t.Fatal(err)
	}
	status, err = ReadStorageStatus(context.Background(), db)
	if err != nil || len(status.UpgradeCopies) != 0 {
		t.Fatal(status, err)
	}
}
