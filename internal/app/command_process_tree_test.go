//go:build !windows

package app

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestManagedCommandOutputLimitKillsUnixProcessGroupPromptly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process groups are platform-specific")
	}
	directory := t.TempDir()
	sentinel := filepath.Join(directory, "output-limit-child-survived")
	script := filepath.Join(directory, "output-limit-command.sh")
	contents := "#!/bin/sh\n(sleep 0.75; touch \"$1\") &\nprintf x >&2\nhead -c 65537 /dev/zero\nsleep 5\n"
	if err := os.WriteFile(script, []byte(contents), 0o700); err != nil {
		t.Fatalf("write command fixture: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	readinessReader, readinessWriter := io.Pipe()
	defer readinessReader.Close()
	defer readinessWriter.Close()
	cmd := exec.Command(script, sentinel)
	cmd.Stderr = readinessWriter
	result := make(chan error, 1)
	go func() {
		_, err := managedCommandOutputLimit(ctx, cmd, 16)
		result <- err
	}()

	marker := make([]byte, 1)
	if _, err := io.ReadFull(readinessReader, marker); err != nil {
		t.Fatalf("read command readiness marker: %v", err)
	}
	started := time.Now()
	select {
	case err := <-result:
		if !errors.Is(err, errManagedCommandOutputLimit) {
			t.Fatalf("command error = %v, want output-limit error", err)
		}
		if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
			t.Fatalf("output-limited command was not terminated promptly after producing output: %s", elapsed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("output-limited command was not terminated promptly after producing output")
	}
	time.Sleep(900 * time.Millisecond)
	if _, statErr := os.Stat(sentinel); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output-limit child survived process-group cleanup: stat error=%v", statErr)
	}
}
