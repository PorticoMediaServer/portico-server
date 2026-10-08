package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var ErrInventoryChanged = errors.New("inventory directory or root changed")

// Inventory cursors are adapter-owned and are valid only for one directory
// observation. No object contents (including sidecars/descriptors) are opened.
type InventoryRequest struct {
	// Stream is an internal isolated-helper protocol flag, not a catalog cursor.
	Stream            bool   `json:"stream,omitempty"`
	Root              string `json:"root"`
	RelativePath      string `json:"relativePath"`
	RootIdentity      string `json:"rootIdentity"`
	DirectoryRevision string `json:"directoryRevision"`
	Cursor            string `json:"cursor"`
	Limit             int    `json:"limit"`
	FollowSymlinks    bool   `json:"followSymlinks"`
}
type InventoryPage struct {
	Remote            *RemotePage `json:"-"`
	Pending           bool        `json:"-"`
	RootIdentity      string      `json:"rootIdentity"`
	DirectoryIdentity string      `json:"directoryIdentity"`
	DirectoryRevision string      `json:"directoryRevision"`
	Entries           []Snapshot  `json:"entries"`
	NextCursor        string      `json:"nextCursor"`
	Complete          bool        `json:"complete"`
}

func InventoryRevision(v Snapshot) string {
	b, _ := json.Marshal([]any{v.ObjectIdentity, v.ChangeToken, v.Size, v.ModifiedNS})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func RootIdentity(v Snapshot) string {
	// dev/inode (or platform equivalent) distinguishes an unmounted backing
	// folder from the admitted mount. Never substitute a pathname as identity.
	return v.ObjectIdentity
}
func (c *Client) inventoryRequest(ctx context.Context, key, op string, r InventoryRequest, out any) error {
	if c.Guard != nil {
		if err := c.Guard(filepath.Join(r.Root, r.RelativePath)); err != nil {
			return err
		}
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req := request{Operation: op, Path: filepath.Join(r.Root, r.RelativePath), Inventory: &r}
	if c.MountedRoot != nil {
		req.MountedRoot = c.MountedRoot(req.Path)
	}
	data, _ := json.Marshal(req)
	cmd := exec.Command(c.Binary, "--portico-storage-helper")
	cmd.Stdin = bytes.NewReader(data)
	return c.Supervisor.Run(ctx, "inventory:"+key, cmd, func(reader io.Reader) error {
		d := json.NewDecoder(io.LimitReader(reader, 2<<20))
		if err := d.Decode(out); err != nil {
			return err
		}
		var extra any
		if err := d.Decode(&extra); err != io.EOF {
			return errors.New("invalid inventory response")
		}
		return nil
	})
}
func (c *Client) InventoryPage(ctx context.Context, key string, r InventoryRequest) (InventoryPage, error) {
	var page InventoryPage
	if c.IsRemote(r.Root) {
		return page, ErrRemoteCursor
	}
	if runtime.GOOS != "linux" {
		return c.retainedInventoryPage(ctx, key, r)
	}
	err := c.inventoryRequest(ctx, key, "inventory-page", r, &page)
	return page, err
}
func (c *Client) InventoryDirectory(ctx context.Context, key string, r InventoryRequest) (Snapshot, error) {
	var v Snapshot
	if c.IsRemote(r.Root) {
		return c.remoteInventoryStat(ctx, r, true)
	}
	err := c.inventoryRequest(ctx, key, "inventory-directory", r, &v)
	return v, err
}

// LocalInventoryPage is also used by isolated deterministic tests. Production
// always calls through Client so a blocked OS-mounted filesystem is killable.
func LocalInventoryPage(r InventoryRequest) (InventoryPage, error) {
	var p InventoryPage
	if r.Limit < 1 || r.Limit > 128 || len(r.Cursor) > 4096 {
		return p, errors.New("invalid inventory page")
	}
	retained, err := openInventoryRoot(r)
	if err != nil {
		return p, err
	}
	defer retained.Close()
	root, err := retained.Stat(".")
	if err != nil || !root.IsDir() {
		return p, ErrInventoryChanged
	}
	rootID := RootIdentity(snapshot(r.Root, root))
	if r.RootIdentity != "" && rootID != r.RootIdentity {
		return p, ErrInventoryChanged
	}
	path, err := inventoryPath(r)
	if err != nil {
		return p, err
	}
	anchored, err := filepath.Rel(r.Root, path)
	if err != nil || !filepath.IsLocal(anchored) {
		return p, ErrInventoryChanged
	}
	f, err := retained.Open(anchored)
	if err != nil {
		return p, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		return p, ErrInventoryChanged
	}
	before := snapshot(path, info)
	rev := InventoryRevision(before)
	if r.DirectoryRevision != "" && rev != r.DirectoryRevision {
		return p, ErrInventoryChanged
	}
	p = InventoryPage{RootIdentity: rootID, DirectoryIdentity: before.ObjectIdentity, DirectoryRevision: rev, Entries: []Snapshot{}}
	readNames := inventoryNames
	if r.Stream || runtime.GOOS != "linux" {
		// Open by the already resolved absolute path, not os.Root's display name.
		// SameFile below compares it with this anchored descriptor before use.
		readNames = func(f *os.File, cursor string, limit int) ([]string, string, bool, error) {
			return retainedInventoryNamesAt(f, path, cursor, limit)
		}
	}
	names, next, done, err := readNames(f, r.Cursor, r.Limit)
	if err != nil {
		return p, err
	}
	for _, name := range names {
		if name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
			return p, ErrInventoryChanged
		}
		child := filepath.Join(path, name)
		info, e := os.Lstat(child)
		if e != nil {
			return p, e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if !r.FollowSymlinks {
				continue
			}
			resolved, e := filepath.EvalSymlinks(child)
			if e != nil {
				return p, e
			}
			rel, e := filepath.Rel(r.Root, resolved)
			if e != nil || !filepath.IsLocal(rel) {
				return p, errors.New("inventory symlink escapes root")
			}
			info, e = retained.Stat(rel)
			if e != nil {
				return p, e
			}
		} else {
			info, e = retained.Stat(filepath.Join(anchored, name))
			if e != nil {
				return p, e
			}
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		v := snapshot(filepath.Join(r.Root, r.RelativePath, name), info)
		v.Name = name
		p.Entries = append(p.Entries, v)
	}
	after, err := f.Stat()
	if err != nil || InventoryRevision(snapshot(path, after)) != rev {
		return p, ErrInventoryChanged
	}
	root, err = os.Stat(r.Root)
	if err != nil || RootIdentity(snapshot(r.Root, root)) != rootID {
		return p, ErrInventoryChanged
	}
	// Re-resolving catches replaced ancestors and changed symlink targets too.
	current, err := os.Stat(path)
	if err != nil || InventoryRevision(snapshot(path, current)) != rev {
		return p, ErrInventoryChanged
	}
	p.NextCursor, p.Complete = next, done
	return p, nil
}
func inventoryPageHelper(r InventoryRequest, out io.Writer) error {
	if r.Stream {
		return inventoryStreamHelper(r, out)
	}
	p, err := LocalInventoryPage(r)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(p)
}
func inventoryPath(r InventoryRequest) (string, error) {
	if !filepath.IsAbs(r.Root) || r.RelativePath != "." && !filepath.IsLocal(r.RelativePath) {
		return "", errors.New("invalid inventory root")
	}
	root := filepath.Clean(r.Root)
	path := root
	for _, part := range strings.Split(filepath.Clean(r.RelativePath), string(filepath.Separator)) {
		if part == "." {
			continue
		}
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if !r.FollowSymlinks {
				return "", errors.New("inventory symlink disabled")
			}
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(root, resolved)
			if err != nil || !filepath.IsLocal(rel) {
				return "", errors.New("inventory symlink escapes root")
			}
			path = resolved
		}
	}
	return path, nil
}
func inspectInventoryDirectory(r InventoryRequest) (Snapshot, error) {
	path, err := inventoryPath(r)
	if err != nil {
		return Snapshot{}, err
	}
	root, err := openInventoryRoot(r)
	if err != nil {
		return Snapshot{}, err
	}
	defer root.Close()
	relative, err := filepath.Rel(r.Root, path)
	if err != nil || !filepath.IsLocal(relative) {
		return Snapshot{}, ErrInventoryChanged
	}
	info, err := root.Stat(relative)
	if err != nil || !info.IsDir() {
		return Snapshot{}, ErrInventoryChanged
	}
	return snapshot(path, info), nil
}

// LocalInventoryDirectory performs only listing/stat metadata operations.
func LocalInventoryDirectory(r InventoryRequest) (Snapshot, error) {
	return inspectInventoryDirectory(r)
}
func parseInventoryOffset(cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	v, e := strconv.ParseInt(cursor, 10, 64)
	if e != nil || v < 0 {
		return 0, ErrInventoryChanged
	}
	return v, nil
}

func inspectInventoryObject(r InventoryRequest) (Snapshot, error) {
	path, err := inventoryPath(r)
	if err != nil {
		return Snapshot{}, err
	}
	root, err := openInventoryRoot(r)
	if err != nil {
		return Snapshot{}, err
	}
	defer root.Close()
	relative, err := filepath.Rel(r.Root, path)
	if err != nil || !filepath.IsLocal(relative) {
		return Snapshot{}, ErrInventoryChanged
	}
	info, err := root.Stat(relative)
	if err != nil || !info.Mode().IsRegular() {
		return Snapshot{}, ErrInventoryChanged
	}
	return snapshot(filepath.Join(r.Root, r.RelativePath), info), nil
}
func (c *Client) InventoryStat(ctx context.Context, key string, r InventoryRequest) (Snapshot, error) {
	var v Snapshot
	if c.IsRemote(r.Root) {
		return c.remoteInventoryStat(ctx, r, false)
	}
	err := c.inventoryRequest(ctx, key, "inventory-stat", r, &v)
	return v, err
}
func LocalInventoryStat(r InventoryRequest) (Snapshot, error) { return inspectInventoryObject(r) }
