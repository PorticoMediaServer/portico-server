//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/hosted/internal/postgres"
)

func TestCustodyPrivateInputsRejectSharingLinksAndDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(path, []byte("protected fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := privateRead(path)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string][]byte{path: raw}
	if !unchangedPrivateInputs(inputs) {
		t.Fatal("unchanged input refused")
	}
	if err = os.WriteFile(path, []byte("changed fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if unchangedPrivateInputs(inputs) {
		t.Fatal("changed configuration accepted")
	}
	link := filepath.Join(filepath.Dir(path), "link")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err = privateRead(link); err == nil {
		t.Fatal("symlink accepted")
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = privateRead(path); err == nil {
		t.Fatal("shared input accepted")
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, make([]byte, 32769), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = privateRead(path); err == nil {
		t.Fatal("unbounded input accepted")
	}
}

func TestCustodyBackupPreservesEncryptedCandidateAndNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "candidate.json")
	want := postgres.ACMECustody{Issuer: "google-production", Directory: "https://dv.acme-v02.api.pki.goog/directory", Cipher: []byte("sealed candidate fixture")}
	if err := writeCandidateBackup(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("backup permissions", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got postgres.ACMECustody
	if err = json.Unmarshal(raw, &got); err != nil || !sameCustody(got, want) {
		t.Fatal("encrypted backup mismatch", err)
	}
	if writeCandidateBackup(path, postgres.ACMECustody{}) == nil {
		t.Fatal("existing backup replaced")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("original backup changed", err)
	}
	link := filepath.Join(dir, "link")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if writeCandidateBackup(link, want) == nil {
		t.Fatal("symlink backup overwritten")
	}
	if err = os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if backupCandidate(filepath.Join(dir, "unsafe.json"), want) == nil {
		t.Fatal("shared backup parent accepted")
	}
}

func TestCustodyRequiresLoadedStoppedWorker(t *testing.T) {
	for _, raw := range []string{"LoadState=loaded\nActiveState=active\nMainPID=42\n", "LoadState=loaded\nActiveState=inactive\nMainPID=42\n", "LoadState=not-found\nActiveState=inactive\nMainPID=0\n", "LoadState=loaded\nActiveState=failed\nMainPID=0\n", ""} {
		if stoppedUnit([]byte(raw)) {
			t.Fatal("live/unknown worker accepted")
		}
	}
	if !stoppedUnit([]byte("LoadState=loaded\nActiveState=inactive\nMainPID=0\n")) {
		t.Fatal("stopped worker refused")
	}
}

func TestCustodyNonRootRefusesBeforePrivateIO(t *testing.T) {
	if os.Getenv("PORTICO_CUSTODY_ROOT_REFUSAL_CHILD") == "1" {
		os.Args = []string{"recover-acme-custody", "--source-database-url-file", "unreachable", "--current-database-url-file", "unreachable", "--config-file", "unreachable", "--apply"}
		main()
		return
	}
	if os.Geteuid() == 0 {
		t.Skip("non-root runner required")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCustodyNonRootRefusesBeforePrivateIO$")
	cmd.Env = append(os.Environ(), "PORTICO_CUSTODY_ROOT_REFUSAL_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil || strings.TrimSpace(string(out)) != "Root operator required." {
		t.Fatalf("nonroot refusal did not precede private I/O: %q", out)
	}
}
