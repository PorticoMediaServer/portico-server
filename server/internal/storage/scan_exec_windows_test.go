//go:build windows

package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLockedScanInputExcludesReplacementAndAncestorRename(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "library")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "media.mkv")
	if err := os.WriteFile(path, []byte("admitted"), 0600); err != nil {
		t.Fatal(err)
	}
	retained, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Close()
	locked, name, closeInput, err := lockWindowsMediaInput(retained)
	if err != nil {
		t.Fatal(err)
	}
	defer closeInput()
	if data, err := os.ReadFile(name); err != nil || string(data) != "admitted" {
		t.Fatalf("wrong acquired input: %q %v", data, err)
	}
	if _, err := os.OpenFile(path, os.O_WRONLY, 0600); err == nil {
		t.Fatal("writer admitted while probe input is locked")
	}
	if err := os.Rename(path, path+".old"); err == nil {
		t.Fatal("file replacement was admitted")
	}
	if err := os.Rename(root, root+".old"); err == nil {
		t.Fatal("ancestor rename was admitted")
	}
	a, _, err := windowsFileBinding(retained)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := windowsFileBinding(locked)
	if err != nil || a != b {
		t.Fatalf("locked handle is not admitted object: %q %q %v", a, b, err)
	}
}

func TestLockedScanInputRejectsPathSubstitution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media.mkv")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	_, name, closeInput, err := lockWindowsMediaInput(file)
	if err == nil {
		defer closeInput()
		// A retained handle can legitimately resolve to its renamed old path;
		// it must never name the replacement.
		if data, e := os.ReadFile(name); e != nil || string(data) != "old" {
			t.Fatalf("substituted media: %q %v", data, e)
		}
	}
}

func TestWindowsInventoryCommandRunsOnRetainedMedia(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "media.mkv")
	if err := os.WriteFile(path, []byte("retained fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client := New(exe)
	ctx := WithScanInput(context.Background(), client, "fixture", InventoryRequest{Root: root})
	var out bytes.Buffer
	handled, err := RunScanCommand(ctx, exe, []string{"-test.run=^TestWindowsScanProbeFixture$", "--", path}, path, &out)
	if err != nil || !handled || out.String() != "retained fixture" {
		t.Fatalf("Windows inventory command failed: %t %q %v", handled, out.String(), err)
	}
}

func TestWindowsScanProbeFixture(t *testing.T) {
	if len(os.Args) < 4 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	data, err := os.ReadFile(os.Args[len(os.Args)-1])
	if err != nil {
		os.Exit(2)
	}
	fmt.Fprint(os.Stdout, string(data))
	os.Exit(0)
}

func TestWindowsEmbeddedLyricProbeUsesAdmittedMedia(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media.flac")
	if err := os.WriteFile(path, []byte(`{"format":{"tags":{"lyrics":"admitted lyrics"}},"streams":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tags, err := probeLyricTags(file, []string{exe, "-test.run=^TestWindowsScanProbeFixture$", "--", descriptorInput})
	if err != nil || len(tags) != 1 || tags[0]["lyrics"] != "admitted lyrics" {
		t.Fatalf("embedded lyrics unavailable: %v %v", tags, err)
	}
}
