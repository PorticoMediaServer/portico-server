package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveRuntimePathsPublishesCompletePrivateDocument(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "portico.config.json")
	if err := SaveRuntimePaths(path, RuntimePaths{DatabasePath: "/srv/portico/portico.db", BackupDirectory: "/srv/portico/backups"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRuntimePaths(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DatabasePath != "/srv/portico/portico.db" || loaded.BackupDirectory != "/srv/portico/backups" {
		t.Fatalf("loaded runtime paths = %#v", loaded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("runtime path mode = %o, want 600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".portico.config-") {
			t.Fatalf("temporary runtime-path publication survived: %s", entry.Name())
		}
	}
}

func TestLoadRuntimePathsRejectsPartialJSONWithoutChangingAuthority(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "portico.config.json")
	if err := SaveRuntimePaths(path, RuntimePaths{DatabasePath: "/old/portico.db"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"databasePath":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntimePaths(path); err == nil {
		t.Fatal("partial runtime-path document was accepted")
	}
}
