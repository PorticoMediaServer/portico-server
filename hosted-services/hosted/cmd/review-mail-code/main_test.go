//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateReadRejectsSharedAndSymlinkedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request")
	if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := privateRead(path); err != nil {
		t.Fatal("private file refused", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := privateRead(link); err == nil {
		t.Fatal("symlink read")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := privateRead(path); err == nil {
		t.Fatal("shared file read")
	}
}
func TestPublishRefusesReplacedRegistrationAndKeepsPrivateMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "registration.json")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	_, original, err := privateRead(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(dir, "replacement")
	if err = os.WriteFile(replacement, []byte("successor"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if err = publish(path, original, []byte("must not replace successor")); err == nil {
		t.Fatal("replaced successor")
	}
	_, current, err := privateRead(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = publish(path, current, []byte("issued fixture")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("code not private", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "issued fixture" {
		t.Fatal("atomic output missing", err)
	}
}
func TestMailReaderRefusesRealRecipientOrForeignOriginBeforeDatabase(t *testing.T) {
	for _, request := range []string{
		`{"registrationId":"reg_abcdefghijklmnop","contactAddress":"real@example.com","origin":"https://web.getportico.tv"}`,
		`{"registrationId":"reg_abcdefghijklmnop","contactAddress":"fixture@m0.getportico.invalid","origin":"https://foreign.example"}`,
	} {
		path := filepath.Join(t.TempDir(), "request")
		if err := os.WriteFile(path, []byte(request), 0600); err != nil {
			t.Fatal(err)
		}
		if err := run("nonexistent-database-file", "nonexistent-seal-file", path); err == nil {
			t.Fatal("real or foreign registration admitted")
		}
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != request {
			t.Fatal("refusal changed request", err)
		}
	}
}

func TestNonRootMainRefusesBeforePrivateFileIO(t *testing.T) {
	if os.Getenv("PORTICO_MAIL_ROOT_REFUSAL_CHILD") == "1" {
		os.Args = []string{"review-mail-code", "--database-url-file", "unreachable", "--seal-file", "unreachable", "--registration-file", "unreachable"}
		main()
		return
	}
	if os.Geteuid() == 0 {
		t.Skip("non-root runner required for direct CLI refusal")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestNonRootMainRefusesBeforePrivateFileIO$")
	cmd.Env = append(os.Environ(), "PORTICO_MAIL_ROOT_REFUSAL_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil || strings.TrimSpace(string(out)) != "Root operator required." {
		t.Fatalf("nonroot CLI did not refuse before private reads: %q", out)
	}
}
