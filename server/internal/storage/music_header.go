package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"time"
)

// ReadScanAudioPrefix reads at most one MiB through the scan's existing root,
// helper and authority. It never reads the rest of a large music file.
func ReadScanAudioPrefix(ctx context.Context, path string) ([]byte, error) {
	if err := CheckScanRead(ctx, path); err != nil {
		return nil, err
	}
	r, input, err := inventoryReadRequest(ctx, path)
	if err != nil {
		return nil, err
	}
	if input == nil {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return scanAudioPrefix(f)
	}
	c := input.client
	if c.IsRemote(path) {
		return nil, ErrRemoteRange
	}
	if c.Guard != nil {
		if err := c.Guard(path); err != nil {
			return nil, err
		}
	}
	req := request{Operation: "inventory-audio-prefix", Path: path, Inventory: &r}
	if c.MountedRoot != nil {
		req.MountedRoot = c.MountedRoot(path)
	}
	raw, _ := json.Marshal(req)
	cmd := exec.Command(c.Binary, "--portico-storage-helper")
	cmd.Stdin = bytes.NewReader(raw)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var prefix []byte
	err = c.Supervisor.Run(ctx, "scan:"+input.source, cmd, func(rd io.Reader) error {
		var err error
		prefix, err = io.ReadAll(io.LimitReader(rd, (1<<20)+1))
		if len(prefix) > 1<<20 {
			return ErrInventoryChanged
		}
		return err
	})
	return prefix, err
}

func scanAudioPrefix(f *os.File) ([]byte, error) {
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return nil, ErrInventoryChanged
	}
	prefix, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || before.Size() != after.Size() || before.ModTime() != after.ModTime() {
		return nil, ErrInventoryChanged
	}
	return prefix, nil
}

func inventoryAudioPrefixHelper(r request, out io.Writer) error {
	if r.Inventory == nil {
		return ErrInventoryChanged
	}
	f, err := openInventoryContent(*r.Inventory)
	if err != nil {
		return err
	}
	defer f.Close()
	prefix, err := scanAudioPrefix(f)
	if err != nil {
		return err
	}
	_, err = out.Write(prefix)
	return err
}
