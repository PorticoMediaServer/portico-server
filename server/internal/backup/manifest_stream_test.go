package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/persistence"
	"portico.local/server/internal/storagepaths"
)

func TestManifestStreamTraversesWithoutRetainingAllEntries(t *testing.T) {
	dir := t.TempDir()
	w, err := newStreamManifestWriter(filepath.Join(dir, ManifestName), Manifest{Format: Format, Version: StreamVersion})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(nil)
	for n := 0; n < 1025; n++ {
		if err = w.add(FileEntry{Name: fmt.Sprintf("Keys/key-%04d", n), SHA256: hex.EncodeToString(sum[:])}); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.finish(); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 0 {
		t.Fatal("stream accumulated a file registry")
	}
	count := 0
	var damage []ManifestDamage
	if err = scanStream(t.Context(), m, func(FileEntry, streamPosition) error { count++; return nil }, func(d ManifestDamage) { damage = append(damage, d) }); err != nil {
		t.Fatal(err)
	}
	if count != 1025 || len(damage) != 0 {
		t.Fatalf("entries=%d damage=%+v", count, damage)
	}
}

func TestStreamRestoreReportsExactDamagedLineAndRecoversLaterFiles(t *testing.T) {
	s, state, _ := testService(t)
	for _, name := range []string{"Metadata/first.txt", "Metadata/second.txt", "Keys/intact.key"} {
		if err := writeTestFile(filepath.Join(state, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	id, err := s.Create(t.Context(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	folder := storagepaths.Path(state, BackupsDir, id)
	raw, err := os.ReadFile(filepath.Join(folder, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.SplitAfter(raw, []byte("\n"))
	var offset, start, end, line int64
	for n, b := range lines {
		if bytes.Contains(b, []byte("Metadata/first.txt")) {
			start = offset
			end = offset + int64(len(b))
			line = int64(n + 1)
			lines[n] = append(bytes.Repeat([]byte{'!'}, len(b)-1), '\n')
			break
		}
		offset += int64(len(b))
	}
	if line < 1 {
		t.Fatal("fixture entry missing")
	}
	if err = os.WriteFile(filepath.Join(folder, ManifestName), bytes.Join(lines, nil), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := s.Stage(t.Context(), Source{BackupID: id})
	if err != nil {
		t.Fatal(err)
	}
	if result.Validation.Recovery == nil || result.Validation.Recovery.ManifestDamage == nil {
		t.Fatal("damage was not reported")
	}
	report := result.Validation.Recovery.ManifestDamage
	found := false
	for _, d := range report.Regions {
		if d.FirstLine == line && d.StartByte == start && d.EndByte == end && d.Reason == "unreadable_record" {
			found = true
		}
	}
	if !found {
		t.Fatalf("wrong precise report: %+v", report)
	}
	staged := storagepaths.Path(state, RestoreStagedDir)
	if _, err = os.Stat(filepath.Join(staged, "Metadata/first.txt")); !os.IsNotExist(err) {
		t.Fatal("unverifiable file installed", err)
	}
	for _, name := range []string{"Metadata/second.txt", "Keys/intact.key"} {
		got, err := os.ReadFile(filepath.Join(staged, name))
		if err != nil || string(got) != name {
			t.Fatal("readable file was lost", name, err)
		}
	}
}

func TestStreamRestoreNeverBlessesMismatchedKeys(t *testing.T) {
	s, state, _ := testService(t)
	name := "Keys/private.key"
	if err := writeTestFile(filepath.Join(state, name), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := s.Create(t.Context(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	folder := storagepaths.Path(state, BackupsDir, id)
	if err = os.WriteFile(filepath.Join(folder, name), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := s.Stage(t.Context(), Source{BackupID: id})
	if err != nil {
		t.Fatal(err)
	}
	r := out.Validation.Recovery.ManifestDamage
	if r.MissingFiles != 1 || r.Count != 1 || r.Regions[0].Reason != "payload_hash_mismatch" {
		t.Fatalf("wrong missing-key report: %+v", r)
	}
	if _, err = os.Stat(storagepaths.Path(state, RestoreStagedDir, name)); !os.IsNotExist(err) {
		t.Fatal("changed key was blessed", err)
	}
}

func TestStreamRestoreRecoversReadableDatabaseWithExplicitHashDamage(t *testing.T) {
	s, state, db := testService(t)
	if _, err := db.Exec(`INSERT INTO configuration(key,value) VALUES('stream_damage','ORIGINAL_STREAM_VALUE')`); err != nil {
		t.Fatal(err)
	}
	id, err := s.Create(t.Context(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	folder := storagepaths.Path(state, BackupsDir, id)
	raw, err := os.ReadFile(filepath.Join(folder, DatabaseName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(raw, []byte("ORIGINAL_STREAM_VALUE")) != 1 {
		t.Fatal("fixture value missing")
	}
	raw = bytes.Replace(raw, []byte("ORIGINAL_STREAM_VALUE"), []byte("MODIFIED_STREAM_VALUE"), 1)
	if err = os.WriteFile(filepath.Join(folder, DatabaseName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := s.Stage(t.Context(), Source{BackupID: id})
	if err != nil {
		t.Fatal(err)
	}
	r := out.Validation.Recovery.ManifestDamage
	if !r.UnverifiedDatabase || r.Regions[0].Reason != "database_hash_mismatch" {
		t.Fatalf("unreported database integrity recovery: %+v", r)
	}
}

func TestStreamRestoreCursorResumesAfterDurableFileMove(t *testing.T) {
	f := stageAfterWrite(t)
	previous := applyStep
	defer func() { applyStep = previous }()
	applyStep = func(step string) error {
		if step == "stream:switch:"+DatabaseName {
			return errCrash
		}
		return nil
	}
	if err := ApplyStaged(t.Context(), f.state, nil); !errors.Is(err, errCrash) {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(storagepaths.Path(f.state, RestoreStagedDir, RestoreMarkerFile))
	if err != nil {
		t.Fatal(err)
	}
	var marker restoreMarker
	if err = json.Unmarshal(raw, &marker); err != nil {
		t.Fatal(err)
	}
	if marker.ManifestOffset <= 0 || marker.ManifestLine < 2 {
		t.Fatalf("no durable cursor: %+v", marker)
	}
	if err = restart(t, f.state, nil); err != nil {
		t.Fatal(err)
	}
	if probe(t, storagepaths.Path(f.state, DatabaseName)) != "backup" {
		t.Fatal("resumed wrong database")
	}
}

func TestStreamManifestDamageSummaryBoundAndEOFConvention(t *testing.T) {
	dir := t.TempDir()
	w, err := newStreamManifestWriter(filepath.Join(dir, ManifestName), Manifest{Format: Format, Version: StreamVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.finish(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, ManifestName), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 25; n++ {
		if _, err = f.WriteString("broken\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()
	m, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	report := &persistence.ManifestRecovery{}
	if err = scanStream(context.Background(), m, func(FileEntry, streamPosition) error { return nil }, func(d ManifestDamage) { addManifestDamage(report, d) }); err != nil {
		t.Fatal(err)
	}
	if report.Count != 25 || len(report.Regions) != 20 || !report.RegionsTruncated {
		t.Fatalf("wrong bounded summary: %+v", report)
	}
}
