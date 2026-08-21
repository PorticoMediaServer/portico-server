//go:build !windows

package app

import (
	"context"
	"errors"
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
	contents := "#!/bin/sh\n(sleep 0.25; touch \"$1\") &\nhead -c 65537 /dev/zero\nsleep 5\n"
	if err := os.WriteFile(script, []byte(contents), 0o700); err != nil {
		t.Fatalf("write command fixture: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	_, err := managedCommandOutputLimit(ctx, exec.Command(script, sentinel), 16)
	if !errors.Is(err, errManagedCommandOutputLimit) {
		t.Fatalf("command error = %v, want output-limit error", err)
	}
	if elapsed := time.Since(started); elapsed >= 250*time.Millisecond {
		t.Fatalf("output-limited command was not terminated promptly: %s", elapsed)
	}
	time.Sleep(400 * time.Millisecond)
	if _, statErr := os.Stat(sentinel); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output-limit child survived process-group cleanup: stat error=%v", statErr)
	}
}
