package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// DiscMemberManifestMax bounds one title's consumed-member manifest.
const DiscMemberManifestMax = 256

// DiscMemberManifest observes the identities of the consumed files a disc
// title read depends on, relative to the admitted disc folder. It is the
// current-source witness around a probe, not a stored result digest.
func (c *Client) DiscMemberManifest(ctx context.Context, key string, r InventoryRequest, members []string) ([]Snapshot, error) {
	var out []Snapshot
	if c.Guard != nil {
		if err := c.Guard(filepath.Join(r.Root, r.RelativePath)); err != nil {
			return nil, err
		}
	}
	if err := CheckScanRead(ctx, filepath.Join(r.Root, r.RelativePath)); err != nil {
		return nil, err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req := request{Operation: "disc-members", Path: filepath.Join(r.Root, r.RelativePath), Inventory: &r, Members: members}
	if c.MountedRoot != nil {
		req.MountedRoot = c.MountedRoot(req.Path)
	}
	data, _ := json.Marshal(req)
	cmd := exec.Command(c.Binary, "--portico-storage-helper")
	cmd.Stdin = bytes.NewReader(data)
	err := c.Supervisor.Run(ctx, "inventory:"+key, cmd, func(reader io.Reader) error {
		d := json.NewDecoder(io.LimitReader(reader, 2<<20))
		if e := d.Decode(&out); e != nil {
			return e
		}
		var extra any
		if e := d.Decode(&extra); e != io.EOF {
			return errors.New("invalid inventory response")
		}
		return nil
	})
	return out, err
}

// LocalDiscMemberManifest is the in-process form for deterministic tests.
func LocalDiscMemberManifest(r InventoryRequest, members []string) ([]Snapshot, error) {
	return discMemberManifest(r, members)
}

func discMemberManifest(r InventoryRequest, members []string) ([]Snapshot, error) {
	if len(members) == 0 || len(members) > DiscMemberManifestMax {
		return nil, errors.New("invalid disc member manifest")
	}
	root, err := openInventoryRoot(r)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := inventoryPath(r)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(r.Root, dir)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		return nil, ErrInventoryChanged
	}
	out := make([]Snapshot, 0, len(members))
	for _, member := range members {
		if len(member) > 4096 || !filepath.IsLocal(member) || member == "." {
			return nil, errors.New("invalid disc member path")
		}
		info, e := root.Stat(filepath.Join(relative, member))
		if e != nil {
			if os.IsNotExist(e) {
				continue
			}
			return nil, e
		}
		if !info.Mode().IsRegular() {
			continue
		}
		entry := snapshot(member, info)
		entry.Path = member
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
