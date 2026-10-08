//go:build linux || darwin

package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A scan command runs in the real helper, which moves the admitted file to
// descriptor 3 and execs the tool. On macOS the helper's Go runtime could hold
// 3 for its kqueue, and the move broke its poller: every probe failed.
func TestScanCommandReadsTheFileThroughTheRealHelper(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := New(binary)
	root := t.TempDir()
	path := filepath.Join(root, "track.flac")
	if err = os.WriteFile(path, []byte("probe me"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		ctx := WithScanInput(context.Background(), c, "source", InventoryRequest{Root: root})
		var out bytes.Buffer
		handled, err := RunScanCommand(ctx, "/bin/cat", []string{path}, path, &out)
		if !handled || err != nil || out.String() != "probe me" {
			t.Fatalf("run %d: handled=%v err=%v out=%q", i, handled, err, out.String())
		}
	}
}

// The helper must start with descriptor 3 already taken (see scanHelperCommand).
// The crash it prevents is a race, so this checks the precondition itself.
func TestScanHelperStartsWithDescriptorThreeReserved(t *testing.T) {
	cmd, err := scanHelperCommand("/bin/true", []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.ExtraFiles) != 1 || cmd.ExtraFiles[0] == nil {
		t.Fatalf("the helper's descriptor 3 is not reserved: %v", cmd.ExtraFiles)
	}
}
