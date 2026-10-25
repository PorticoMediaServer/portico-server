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

// DiscMemberManifestMax bounds one folder disc's consumed-member manifest.
const DiscMemberManifestMax = 256

// DiscMember is one consumed disc file's observed identity. Required members
// must exist as regular files; optional structure files may be absent.
type DiscMember struct {
	Path           string `json:"path"`
	ObjectIdentity string `json:"objectIdentity,omitempty"`
	Size           int64  `json:"size"`
	ModifiedNS     int64  `json:"modifiedNs"`
	Required       bool   `json:"required"`
}

// DiscMemberManifest observes the identities of the consumed files a folder
// disc publication and its title reads depend on, relative to the admitted
// disc folder. Required members that are missing or not regular fail closed;
// optional ones may be absent.
func (c *Client) DiscMemberManifest(ctx context.Context, key string, r InventoryRequest, required, optional []string) ([]DiscMember, error) {
	var out []DiscMember
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
	req := request{Operation: "disc-members", Path: filepath.Join(r.Root, r.RelativePath), Inventory: &r, Members: required, OptionalMembers: optional}
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
func LocalDiscMemberManifest(r InventoryRequest, required, optional []string) ([]DiscMember, error) {
	return discMemberManifest(r, required, optional)
}

func discMemberManifest(r InventoryRequest, required, optional []string) ([]DiscMember, error) {
	if len(required) == 0 || len(required) > DiscMemberManifestMax || len(optional) > DiscMemberManifestMax || len(required)+len(optional) > DiscMemberManifestMax {
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
	out := make([]DiscMember, 0, len(required)+len(optional))
	observe := func(paths []string, mustExist bool) error {
		for _, member := range paths {
			if len(member) > 4096 || !filepath.IsLocal(member) || member == "." {
				return errors.New("invalid disc member path")
			}
			info, e := root.Stat(filepath.Join(relative, member))
			if e != nil {
				if !mustExist && os.IsNotExist(e) {
					continue
				}
				return e
			}
			if !info.Mode().IsRegular() {
				return errors.New("disc member is not a regular file: " + member)
			}
			entry := snapshot(member, info)
			out = append(out, DiscMember{Path: member, ObjectIdentity: entry.ObjectIdentity, Size: entry.Size, ModifiedNS: entry.ModifiedNS, Required: mustExist})
		}
		return nil
	}
	if err = observe(required, true); err != nil {
		return nil, err
	}
	if err = observe(optional, false); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
