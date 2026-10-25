//go:build windows

package storage

import (
	"errors"
	"io"
	"os"
	"strings"
)

// RootedDir is one directory confined by an os.Root. Windows has no openat
// O_NOFOLLOW equivalent here, so every step re-checks that the next component
// is a real directory and not a reparse point before it is used.
type RootedDir struct {
	root *os.Root
	rel  string
}

func OpenRootedDir(path string) (*RootedDir, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &RootedDir{root: root, rel: "."}, nil
}

func (d *RootedDir) path(name string) string {
	if d.rel == "." {
		return name
	}
	return d.rel + "/" + name
}

func (d *RootedDir) Child(name string) (*RootedDir, error) {
	if d == nil || d.root == nil || name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return nil, ErrRemoteConfig
	}
	next := d.path(name)
	info, err := d.root.Lstat(next)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrRemoteConfig
	}
	return &RootedDir{root: d.root, rel: next}, nil
}

func (d *RootedDir) Close() error {
	if d == nil || d.root == nil {
		return nil
	}
	return d.root.Close()
}

func (d *RootedDir) Names(limit int) ([]string, bool, error) {
	if d == nil || d.root == nil || limit < 1 {
		return nil, false, ErrRemoteConfig
	}
	directory, err := d.root.Open(d.rel)
	if err != nil {
		return nil, false, err
	}
	defer directory.Close()
	out := make([]string, 0, 64)
	for len(out) < limit {
		batch := 256
		if remaining := limit - len(out); remaining < batch {
			batch = remaining
		}
		names, err := directory.Readdirnames(batch)
		out = append(out, names...)
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return out, false, nil
		}
		return nil, false, err
	}
	probe, err := directory.Readdirnames(1)
	if len(probe) > 0 {
		return out, true, nil
	}
	if err == nil || errors.Is(err, io.EOF) {
		return out, false, nil
	}
	return nil, false, err
}

func (d *RootedDir) IsDirectoryEntry(name string) bool {
	if d == nil || d.root == nil || name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return false
	}
	info, err := d.root.Lstat(d.path(name))
	if err != nil {
		return false
	}
	return info.IsDir() && info.Mode()&os.ModeSymlink == 0
}
