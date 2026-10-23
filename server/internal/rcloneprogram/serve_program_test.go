package rcloneprogram

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestServeProgramRejectsUnapprovedImageBeforeVersionOrEnforcement(t *testing.T) {
	private := t.TempDir()
	program := filepath.Join(t.TempDir(), "rclone")
	if err := os.WriteFile(program, []byte("\x7fELFnot-an-approved-image"), 0500); err != nil {
		t.Fatal(err)
	}
	called := false
	spec := ServeCommandSpec{SourceID: "source", SourceGeneration: "1", PrivateDirectory: private, Configuration: []byte("[media]\ntype=local\n"), Remote: "media:", Port: 12345, Username: "private", Password: "fixture-only", Connections: 1, RequestsPerSecond: 2, Enforce: func(*exec.Cmd) error { called = true; return nil }}
	for _, digest := range []string{"", "unapproved", string(make([]byte, 64)), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		spec.ApprovedProgramDigest = digest
		if _, err := PrepareServe(context.Background(), Program{Path: program}, spec); !errors.Is(err, ErrServeProgram) {
			t.Fatal("unapproved image reached version command", err)
		}
	}
	if called {
		t.Fatal("unapproved image reached outbound installer")
	}
	entries, err := os.ReadDir(private)
	if err != nil || len(entries) != 0 {
		t.Fatal("unapproved image created credential config", entries, err)
	}
}

func TestServeProgramDigestRejectsEqualSizeAndTimeReplacementAndUnsafePath(t *testing.T) {
	program := filepath.Join(t.TempDir(), "rclone")
	data := []byte("\x7fELFowner-approved-image")
	if err := os.WriteFile(program, data, 0700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if err := checkServeProgram(Program{Path: program}, digest); err != nil {
		t.Fatal("native image format/digest", err)
	}
	info, err := os.Stat(program)
	if err != nil {
		t.Fatal(err)
	}
	replacement := append([]byte(nil), data...)
	replacement[len(replacement)-1] ^= 1
	if err = os.WriteFile(program, replacement, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(program, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err = checkServeProgram(Program{Path: program}, digest); !errors.Is(err, ErrServeProgram) {
		t.Fatal("equal-stat replacement admitted", err)
	}
	if err = os.WriteFile(program, data, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(program), "link")
	if err = os.Symlink(program, link); err != nil {
		t.Fatal(err)
	}
	if err = checkServeProgram(Program{Path: link}, digest); !errors.Is(err, ErrServeProgram) {
		t.Fatal("noncanonical image admitted", err)
	}
	if err = os.Chmod(program, 0770); err != nil {
		t.Fatal(err)
	}
	if err = checkServeProgram(Program{Path: program}, digest); !errors.Is(err, ErrServeProgram) {
		t.Fatal("group-writable image admitted", err)
	}
}
