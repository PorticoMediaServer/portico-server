package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// stateFiles are the state files a backup carries besides the database, as
// paths relative to the state directory. Directories are carried recursively,
// except managed-storage, which carries only rclone's plain config files.
// Anything absent is skipped; rebuildable data (transcodes, caches, artwork,
// logs, prepared media) and the backups and restore-staged folders themselves
// are never carried.
func stateFiles() []string {
	return []string{
		// The server identity.
		"networking-keys",
		// The TLS private key as a normal .pem file (§6 plain state).
		"networking-tls.pem",
		// The current server authority incarnation.
		"supervisor/authority-incarnation.v1",
		// Rclone's plain mount configs (§6 plain state).
		"managed-storage",
		// Owner-supplied, non-rebuildable files.
		"channel-logos",
		"subtitles",
	}
}

// copyStateFiles copies the state file set from state into the staging folder
// at the same relative paths, and returns the manifest entries. Names use "/".
func copyStateFiles(state, partial string) ([]FileEntry, error) {
	entries := []FileEntry{}
	for _, rel := range stateFiles() {
		src := filepath.Join(state, filepath.FromSlash(rel))
		info, err := os.Lstat(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			entry, err := copyOneFile(src, filepath.Join(partial, filepath.FromSlash(rel)), rel)
			if err != nil {
				return nil, err
			}
			entries = append(entries, entry)
			continue
		}
		if rel == "managed-storage" {
			// Only the plain configs: caches, candidates and anything else
			// the mount service keeps here are rebuildable or transient.
			names, err := os.ReadDir(src)
			if err != nil {
				return nil, err
			}
			for _, name := range names {
				if name.IsDir() || !strings.HasSuffix(name.Name(), ".conf") {
					continue
				}
				fileEntry, err := copyOneFile(filepath.Join(src, name.Name()), filepath.Join(partial, rel, name.Name()), rel+"/"+name.Name())
				if err != nil {
					return nil, err
				}
				entries = append(entries, fileEntry)
			}
			continue
		}
		err = filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name, err := filepath.Rel(state, path)
			if err != nil {
				return err
			}
			name = filepath.ToSlash(name)
			if entry.IsDir() {
				return os.MkdirAll(filepath.Join(partial, filepath.FromSlash(name)), 0700)
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			fileEntry, err := copyOneFile(path, filepath.Join(partial, filepath.FromSlash(name)), name)
			if err != nil {
				return err
			}
			entries = append(entries, fileEntry)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// copyOneFile copies one regular file, refusing symlinks, and hashes it.
func copyOneFile(src, dst, name string) (FileEntry, error) {
	return copyOneFileContext(context.Background(), src, dst, name, nil)
}

func copyOneFileContext(ctx context.Context, src, dst, name string, progress func(int64, int64)) (FileEntry, error) {
	entry := FileEntry{Name: name}
	info, err := os.Lstat(src)
	if err != nil {
		return entry, err
	}
	if !info.Mode().IsRegular() {
		return entry, ErrSourceInvalid
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return entry, err
	}
	in, err := os.Open(src)
	if err != nil {
		return entry, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return entry, err
	}
	hash := sha256.New()
	var written int64
	buffer := make([]byte, 128<<10)
	for {
		if err = ctx.Err(); err != nil {
			break
		}
		n, readErr := in.Read(buffer)
		if n > 0 {
			count, writeErr := out.Write(buffer[:n])
			hash.Write(buffer[:count])
			written += int64(count)
			if progress != nil {
				progress(written, info.Size())
			}
			if writeErr != nil {
				err = writeErr
				break
			}
			if count != n {
				err = io.ErrShortWrite
				break
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			err = readErr
			break
		}
	}
	if syncErr := out.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return entry, err
	}
	entry.Bytes = written
	entry.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return entry, nil
}

// writeManifest stores manifest.json in a finished staging folder.
func writeManifest(partial string, manifest Manifest) error {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err = durableFile(filepath.Join(partial, ManifestName), raw); err != nil {
		return err
	}
	return nil
}

// readManifest parses a backup folder's manifest, checking its identity.
func readManifest(dir string) (Manifest, error) {
	var manifest Manifest
	if err := regularManifestFile(dir, ManifestName); err != nil {
		return manifest, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return manifest, err
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Format != Format || manifest.Version != Version {
		return manifest, ErrInvalid
	}
	return manifest, nil
}

// estimate sizes the coming backup: the database, its WAL, and the state
// files. The free-space check multiplies this by 1.1.
func (s *Service) estimate() (int64, error) {
	var total int64
	for _, name := range []string{"server.sqlite", "server.sqlite-wal"} {
		if info, err := os.Stat(filepath.Join(s.state, name)); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	for _, rel := range stateFiles() {
		src := filepath.Join(s.state, filepath.FromSlash(rel))
		info, err := os.Lstat(src)
		if err != nil || !info.Mode().IsRegular() && !info.IsDir() {
			continue
		}
		if info.Mode().IsRegular() {
			total += info.Size()
			continue
		}
		confOnly := rel == "managed-storage"
		_ = filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if confOnly && !strings.HasSuffix(entry.Name(), ".conf") {
				return nil
			}
			if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
				total += info.Size()
			}
			return nil
		})
	}
	if total < 1<<20 {
		total = 1 << 20
	}
	return total, nil
}

// dirBytes sums the regular files under dir.
func dirBytes(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// manifestFileEntries reads the manifest's file list for hash comparison.
func manifestFileEntries(dir string) ([]FileEntry, error) {
	manifest, err := readManifest(dir)
	if err != nil {
		return nil, err
	}
	return manifest.Files, nil
}

// checkManifestHashes verifies every listed file's sha256 against the tree.
func checkManifestHashes(dir string, files []FileEntry) error {
	for _, entry := range files {
		if !safeRelativeFile(entry.Name) {
			return ErrSourceInvalid
		}
		if err := regularManifestFile(dir, entry.Name); err != nil {
			return err
		}
		path := filepath.Join(dir, filepath.FromSlash(entry.Name))
		// Streamed: a pre-restore backup lists the database itself, which can be
		// larger than memory.
		file, err := os.Open(path)
		if err != nil {
			return ErrSourceInvalid
		}
		hash := sha256.New()
		read, err := io.Copy(hash, file)
		file.Close()
		if err != nil {
			return ErrSourceInvalid
		}
		if hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 || read != entry.Bytes {
			return ErrIntegrity
		}
	}
	return nil
}

func durableFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Refuse links within a backup and special files before opening them. The owner
// may choose a mounted or linked destination; its contents must be plain files.
func regularManifestFile(root, name string) error {
	parts := strings.Split(name, "/")
	current := root
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return ErrSourceInvalid
		}
		if i == len(parts)-1 {
			if !info.Mode().IsRegular() {
				return ErrSourceInvalid
			}
		} else if !info.IsDir() {
			return ErrSourceInvalid
		}
	}
	return nil
}
