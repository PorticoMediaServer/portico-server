package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailedStartupPersistsBeforeRecorderCloses(t *testing.T) {
	state := t.TempDir()
	command := exec.Command(serverBinary(t))
	command.Env = append(os.Environ(), "PORTICO_STATE_DIR="+state, "PORTICO_BIND=invalid", "PORTICO_LOG_STDERR=off")
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatal("invalid bind started a server", string(output))
	}
	raw, err := os.ReadFile(filepath.Join(state, "logs", "messages.log"))
	if err != nil || !strings.Contains(string(raw), "Portico stopped:") || !strings.Contains(string(raw), "\terror\tserver\t") {
		t.Fatal("startup failure was lost", string(raw), err)
	}
}
