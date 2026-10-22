package storagepaths

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const ownershipMarker = ".portico-owned-v1"

func (l Layout) ownership(name string) string {
	return "Portico disposable directory v1\n" + filepath.Clean(l.Data) + "\n" + name + "\n"
}

func verifyOwnedRoot(root *os.Root, expected string) error {
	info, err := root.Lstat(ownershipMarker)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return errors.New("refusing to adopt a preexisting unowned Cache or Temp directory; choose a fresh data folder")
	}
	f, err := root.Open(ownershipMarker)
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return errors.New("disposable directory ownership changed")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return err
	}
	if string(raw) != expected {
		return errors.New("disposable directory does not belong to this data layout")
	}
	return nil
}

func (l Layout) verifyOwned(name string) error {
	dir := filepath.Join(l.Data, name)
	if err := canonicalParents(dir); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return verifyOwnedRoot(root, l.ownership(name))
}

func (l Layout) prepareOwned(name string) error {
	dir := filepath.Join(l.Data, name)
	if err := os.Mkdir(dir, 0700); errors.Is(err, os.ErrExist) {
		return l.verifyOwned(name)
	} else if err != nil {
		return err
	}
	// Mkdir succeeded: this scope was created by us. Check its pinned handle is
	// still empty before recording ownership; do not mint a marker over data.
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	folder, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := folder.ReadDir(1)
	folder.Close()
	if len(entries) != 0 || readErr != io.EOF {
		return fmt.Errorf("new disposable directory is not empty: %s", dir)
	}
	marker, err := root.OpenFile(ownershipMarker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := io.WriteString(marker, l.ownership(name))
	if writeErr == nil {
		writeErr = marker.Sync()
	}
	return errors.Join(writeErr, marker.Close())
}
