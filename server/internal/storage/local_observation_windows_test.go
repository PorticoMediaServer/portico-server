//go:build windows

package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalObjectBindingSurvivesRenameAndDetectsReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "media.mkv")
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	binding := func(path string) Snapshot {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		s := snapshot(path, info)
		if s.ObjectIdentity == "" || s.ChangeToken == "" {
			t.Fatalf("missing binding: %+v", s)
		}
		return s
	}
	first := binding(path)
	renamed := filepath.Join(dir, "renamed.mkv")
	if err := os.Rename(path, renamed); err != nil {
		t.Fatal(err)
	}
	if got := binding(renamed); got.ObjectIdentity != first.ObjectIdentity {
		t.Fatal("rename changed object identity")
	}
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := binding(path); got.ObjectIdentity == first.ObjectIdentity {
		t.Fatal("replacement reused object identity")
	}
	if got := binding(dir); !got.Directory {
		t.Fatal("directory binding lost type")
	}
}

func TestLocalObjectBindingRejectsStaleAttributes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media.mkv")
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed size"), 0600); err != nil {
		t.Fatal(err)
	}
	if object, _, err := observedObjectBinding(info, path); err == nil || object != "" {
		t.Fatal("stale attributes accepted")
	}
}
