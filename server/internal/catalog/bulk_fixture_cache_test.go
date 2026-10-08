package catalog

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Only the opt-in Linux acceptance run caches its immutable, pre-action seed.
// Normal unit tests always create a fresh temporary database.
func bulkFixtureCachePath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Fatal("bulk acceptance fixtures belong on the Linux runner")
	}
	home, e := os.UserHomeDir()
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(home, "work", "jobs-fixtures")
	if e = os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	h := sha256.New()
	h.Write([]byte("jobs-seed-v1:5000 episodes;5 seasons;1000 episodes/season;base d823f999"))
	migrations, e := filepath.Glob("../persistence/migrations/*.sql")
	if e != nil || len(migrations) == 0 {
		t.Fatal("migration digest", e)
	}
	for _, path := range migrations {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		h.Write(raw)
	}
	return filepath.Join(dir, "5k-"+hex.EncodeToString(h.Sum(nil))[:16]+".db")
}
func bulkFileHash(t *testing.T, path string) string {
	t.Helper()
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		t.Fatal(e)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func loadBulkFixture(t *testing.T, destination string) bool {
	t.Helper()
	path := bulkFixtureCachePath(t)
	raw, e := os.ReadFile(path + ".json")
	if os.IsNotExist(e) {
		return false
	}
	if e != nil {
		t.Fatal(e)
	}
	var manifest map[string]string
	if e = json.Unmarshal(raw, &manifest); e != nil {
		t.Fatal(e)
	}
	if got := bulkFileHash(t, path); got != manifest["sha256"] {
		t.Fatal("cached fixture checksum mismatch", path)
	}
	in, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer in.Close()
	out, e := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal(copyErr, closeErr)
	}
	t.Log("reused immutable fixture", path)
	return true
}
func saveBulkFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	path := bulkFixtureCachePath(t)
	if _, e := os.Stat(path); e == nil {
		t.Fatal("unmanifested fixture exists; review before replacing", path)
	} else if !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if _, e := db.Exec(`VACUUM INTO ?`, path); e != nil {
		t.Fatal(e)
	}
	stat, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	manifest := map[string]string{"sha256": bulkFileHash(t, path), "base": "d823f999", "generator": "bulkFixture v1; episodes=5000; seasons=5; perSeason=1000; normal triggers"}
	raw, e := json.MarshalIndent(manifest, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path+".json", raw, 0600); e != nil {
		t.Fatal(e)
	}
	t.Logf("retained immutable fixture %s bytes=%d", path, stat.Size())
}
