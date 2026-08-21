package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingLogWriterKeepsPrivateBoundedBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "porticod.log")
	protected := 0
	writer, err := newRotatingLogWriter(path, 4, 2, func(string, int) error {
		protected++
		return nil
	})
	if err != nil {
		t.Fatalf("create rotating writer: %v", err)
	}
	if _, err := writer.Write([]byte("1234")); err != nil {
		t.Fatalf("write initial log: %v", err)
	}
	if _, err := writer.Write([]byte("56")); err != nil {
		t.Fatalf("rotate and write log: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close rotating writer: %v", err)
	}
	active, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read active log: %v", err)
	}
	backup, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("read rotated log: %v", err)
	}
	if string(active) != "56" || string(backup) != "1234" {
		t.Fatalf("unexpected rotated bytes: active=%q backup=%q", active, backup)
	}
	if protected < 2 {
		t.Fatalf("private artifact protector ran %d times, want initial and post-rotation checks", protected)
	}
	for _, candidate := range []string{path, path + ".1"} {
		info, err := os.Stat(candidate)
		if err != nil {
			t.Fatalf("stat %s: %v", candidate, err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("log artifact %s is not private: %o", candidate, info.Mode().Perm())
		}
	}
}

func TestRotatingLogWriterFailsClosedOnOpenAndRotationFaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "porticod.log")
	sentinel := errors.New("private artifact check failed")
	if _, err := newRotatingLogWriter(path, 4, 1, func(string, int) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("protect failure = %v, want %v", err, sentinel)
	}

	directoryPath := filepath.Join(t.TempDir(), "not-a-file")
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatalf("create open fault directory: %v", err)
	}
	if _, err := newRotatingLogWriter(directoryPath, 4, 1); err == nil {
		t.Fatal("directory path unexpectedly opened as a log file")
	}

	rotationDir := t.TempDir()
	rotationPath := filepath.Join(rotationDir, "porticod.log")
	writer, err := newRotatingLogWriter(rotationPath, 4, 2)
	if err != nil {
		t.Fatalf("create rotation-fault writer: %v", err)
	}
	if _, err := writer.Write([]byte("1234")); err != nil {
		t.Fatalf("write rotation fixture: %v", err)
	}
	if err := os.Mkdir(rotationPath+".2", 0o700); err != nil {
		t.Fatalf("create non-removable backup directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rotationPath+".2", "keep"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("populate non-removable backup directory: %v", err)
	}
	if _, err := writer.Write([]byte("56")); err == nil {
		t.Fatal("rotation unexpectedly succeeded through a non-empty backup directory")
	}
	if _, err := writer.Write([]byte("78")); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("writer did not fail closed after rotation fault: %v", err)
	}
}

func TestRotatingLogWriterBoundsSingleOversizedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "porticod.log")
	writer, err := newRotatingLogWriter(path, 4, 3)
	if err != nil {
		t.Fatalf("create rotating writer: %v", err)
	}
	if n, err := writer.Write([]byte("abcdefghijkl")); err != nil || n != 12 {
		t.Fatalf("oversized write n=%d err=%v", n, err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close rotating writer: %v", err)
	}
	for _, candidate := range []string{path, path + ".1", path + ".2"} {
		info, err := os.Stat(candidate)
		if err != nil {
			t.Fatalf("stat %s: %v", candidate, err)
		}
		if info.Size() > 4 {
			t.Fatalf("oversized write left %s at %d bytes", candidate, info.Size())
		}
	}
}
