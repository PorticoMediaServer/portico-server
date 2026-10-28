package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"portico.local/server/internal/storagepaths"
	"sort"
	"strings"
)

// stateFiles are the state files a backup carries besides the database, as
// paths relative to the state directory. Directories are carried recursively,
// except managed-storage, which carries only rclone's plain config files.
// Anything absent is skipped; rebuildable data (transcodes, caches, artwork,
// logs, prepared media) and the backups and restore-staged folders themselves
// are never carried.
func stateFiles() []string { return []string{"portico.toml", "Metadata", "Keys"} }

func copyStateFilesForBackup(ctx context.Context, state, partial string, includeArtwork bool) ([]FileEntry, error) {
	entries := []FileEntry{}
	for _, rel := range stateFiles() {
		src := storagepaths.Path(state, filepath.FromSlash(rel))
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
			if !includeArtwork && excludedDownloadedMetadata(name) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return ErrSourceInvalid
			}
			if entry.IsDir() {
				return os.MkdirAll(filepath.Join(partial, filepath.FromSlash(name)), 0700)
			}
			if rebuildable(name) && strings.Contains(name, artworkTombMarker) {
				return nil // a collector's file in transit, carried by its own name if needed
			}
			info, err := entry.Info()
			if err != nil {
				if rebuildable(name) && errors.Is(err, fs.ErrNotExist) {
					return nil // collected while the walk ran; it can be fetched again
				}
				return err
			}
			if !info.Mode().IsRegular() {
				return ErrSourceInvalid
			}
			fileEntry, err := copyOneFileContext(ctx, path, filepath.Join(partial, filepath.FromSlash(name)), name, nil)
			if err != nil {
				if rebuildable(name) && errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			entries = append(entries, fileEntry)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	// The owner's artwork is always carried, and checked against its digest,
	// even when the whole folder was walked: the walk skips a file a
	// collector had moved aside, and owner artwork must not be lost that way.
	copied := make(map[string]string, len(entries))
	for _, entry := range entries {
		copied[entry.Name] = entry.SHA256
	}
	owner, err := copyOwnerArtwork(ctx, state, partial, copied)
	if err != nil {
		return nil, err
	}
	entries = append(entries, owner...)
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
	opened, err := in.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return entry, ErrSourceInvalid
	}
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
	after, statErr := in.Stat()
	current, pathErr := os.Lstat(src)
	if statErr != nil || pathErr != nil || !os.SameFile(info, current) || !os.SameFile(info, after) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) || written != info.Size() {
		return entry, ErrIntegrity
	}
	entry.Bytes = written
	entry.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return entry, nil
}

// writeManifest stores manifest.json in a finished staging folder.
const maxManifestBytes = 64 << 20

func writeManifest(partial string, manifest Manifest) error {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > maxManifestBytes {
		return ErrInvalid
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
	info, err := os.Stat(filepath.Join(dir, ManifestName))
	if err != nil || info.Size() > maxManifestBytes {
		return manifest, ErrSourceInvalid
	}
	file, err := os.Open(filepath.Join(dir, ManifestName))
	if err != nil {
		return manifest, err
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	file.Close()
	if err != nil {
		return manifest, err
	}
	if len(raw) > maxManifestBytes {
		return manifest, ErrSourceInvalid
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Format != Format || (manifest.Version != Version && manifest.Version != LegacyVersion) {
		return manifest, ErrInvalid
	}
	if _, ok := stateFileNames(manifest.Files); !ok {
		return manifest, ErrSourceInvalid
	}
	database := false
	for _, entry := range manifest.Files {
		digest, err := hex.DecodeString(entry.SHA256)
		if entry.Bytes < 0 || err != nil || len(digest) != sha256.Size {
			return manifest, ErrSourceInvalid
		}
		database = database || entry.Name == DatabaseName
	}
	// Version 1 ordinary backups omitted the database. Keep that deliberate
	// recovery path, with SQLite/schema validation still mandatory in Stage.
	if manifest.Version == Version && !database {
		return manifest, ErrSourceInvalid
	}
	return manifest, nil
}

// estimate sizes the coming backup: the database, its WAL, and the state
// files. The free-space check multiplies this by 1.1.
func (s *Service) estimate(ctx context.Context, includeArtwork bool) (int64, error) {
	var total int64
	for _, name := range []string{"Database/server.sqlite", "Database/server.sqlite-wal"} {
		if info, err := os.Stat(storagepaths.Path(s.state, name)); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	for _, rel := range stateFiles() {
		src := storagepaths.Path(s.state, filepath.FromSlash(rel))
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
			name, _ := filepath.Rel(s.state, path)
			name = filepath.ToSlash(name)
			if !includeArtwork && excludedDownloadedMetadata(name) {
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
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
	if !includeArtwork {
		var ownerBytes int64
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(sum(bytes),0) FROM artwork_blobs WHERE digest IN (`+ownerArtworkReferences+`)`).Scan(&ownerBytes); err != nil {
			return 0, err
		}
		total += ownerBytes
	}
	if total < 1<<20 {
		total = 1 << 20
	}
	return total, nil
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
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
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

// checkSourceHashes also refuses unverified WAL frames: SQLite would replay
// them over an otherwise verified database during candidate inspection. Empty
// unlisted WALs are harmless and can be created by a concurrent read-only open.
func checkSourceHashes(dir string, manifest Manifest) error {
	if err := checkManifestHashes(dir, manifest.Files); err != nil {
		return err
	}
	for _, entry := range manifest.Files {
		if entry.Name == DatabaseName+"-wal" {
			return nil
		}
	}
	info, err := os.Lstat(filepath.Join(dir, DatabaseName+"-wal"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return ErrSourceInvalid
	}
	if info.Size() != 0 {
		return ErrIntegrity
	}
	return nil
}

// hashFile hashes a finalized regular payload without loading it into memory.
func hashFile(ctx context.Context, folder, name string) (FileEntry, error) {
	entry := FileEntry{Name: name}
	if err := regularManifestFile(folder, name); err != nil {
		return entry, err
	}
	file, err := os.Open(filepath.Join(folder, filepath.FromSlash(name)))
	if err != nil {
		return entry, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(hash, &scrubReader{ctx: ctx, r: file})
	closeErr := file.Close()
	if copyErr != nil {
		return entry, copyErr
	}
	if closeErr != nil {
		return entry, closeErr
	}
	entry.Bytes, entry.SHA256 = size, hex.EncodeToString(hash.Sum(nil))
	return entry, nil
}

// Owner references come from the exact finalized SQLite snapshot, including undo
// history. Remote images are excluded unless explicitly selected/edited by an owner.
func copyOwnerArtwork(ctx context.Context, state, partial string, copied map[string]string) ([]FileEntry, error) {
	u := url.URL{Scheme: "file", Path: filepath.Join(partial, DatabaseName), RawQuery: "mode=ro"}
	db, e := sql.Open("sqlite", u.String())
	if e != nil {
		return nil, e
	}
	defer db.Close()
	rows, e := db.QueryContext(ctx, ownerArtworkReferences)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	entries := []FileEntry{}
	for rows.Next() {
		var digest string
		if e = rows.Scan(&digest); e != nil {
			return nil, e
		}
		raw, e := hex.DecodeString(digest)
		if e != nil || len(raw) != sha256.Size {
			return nil, ErrIntegrity
		}
		name := "Metadata/artwork/" + digest + ".img"
		if hash, ok := copied[name]; ok {
			// Already carried by the whole-folder walk: its bytes must still
			// be the ones the database names.
			if hash != digest {
				return nil, ErrIntegrity
			}
			continue
		}
		entry, e := copyArtworkBlob(ctx, state, partial, name)
		if e != nil {
			return nil, e
		}
		if entry.SHA256 != digest {
			return nil, ErrIntegrity
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// artworkTombMarker is the artwork collector's name for a file it has moved
// aside while it checks whether the file is still named
// (metadata/artwork_lifecycle.go): <digest>.img.tomb-<token>.
const artworkTombMarker = ".img.tomb-"

// copyArtworkBlob copies one artwork file. A collector that marked the file
// before this backup's hold may have moved it aside for a moment; it puts it
// back when it finds the file named, but the backup does not wait for that:
// the moved-aside file has the same bytes (the name is their digest, and the
// caller checks it).
func copyArtworkBlob(ctx context.Context, state, partial, name string) (FileEntry, error) {
	source := filepath.Join(state, filepath.FromSlash(name))
	entry, err := copyOneFileContext(ctx, source, filepath.Join(partial, filepath.FromSlash(name)), name, nil)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return entry, err
	}
	tombs, globErr := filepath.Glob(source + strings.TrimPrefix(artworkTombMarker, ".img") + "*")
	if globErr != nil {
		return entry, err
	}
	for _, tomb := range tombs {
		if moved, e := copyOneFileContext(ctx, tomb, filepath.Join(partial, filepath.FromSlash(name)), name, nil); e == nil {
			return moved, nil
		}
	}
	return entry, err
}

// rebuildable is a file under a folder of downloaded metadata, which can be
// fetched again.
func rebuildable(name string) bool {
	for _, dir := range []string{"Metadata/artwork/", "Metadata/people/", "Metadata/guide-images/", "Metadata/analysis-artifacts/"} {
		if strings.HasPrefix(name, dir) {
			return true
		}
	}
	return false
}

// Keys/config must remain stable across the online database snapshot. Their
// writers use atomic files; a concurrent rotation makes this run fail closed.
func authorityFingerprint(ctx context.Context, state string) (string, error) {
	h := sha256.New()
	for _, name := range []string{"portico.toml", "Keys"} {
		root := filepath.Join(state, name)
		e := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
			if os.IsNotExist(e) && p == root {
				return nil
			}
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return ErrSourceInvalid
			}
			entry, e := hashFile(ctx, state, filepath.ToSlash(mustRelative(state, p)))
			if e != nil {
				return e
			}
			b, _ := json.Marshal(entry)
			h.Write(b)
			return nil
		})
		if e != nil {
			return "", e
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func mustRelative(root, p string) string { r, _ := filepath.Rel(root, p); return r }

func excludedDownloadedMetadata(name string) bool {
	return name == "Metadata/artwork" || name == "Metadata/people" || name == "Metadata/guide-images" || name == "Metadata/analysis-artifacts"
}

// ownerArtworkReferences is the backup's artwork closure: every blob of every
// source an owner made irreplaceable (uploads, locked or owner-chosen
// selections, and undo history pins). Each source carries both slots and its
// ambient effect; a blob shared by several sources is listed once. Provider
// art is re-fetched after a restore and is not carried.
const ownerArtworkReferences = `WITH roots(digest) AS (
 SELECT digest FROM artwork_uploads
 UNION SELECT digest FROM artwork_selections WHERE locked=1 OR actor<>''
 UNION SELECT j.value FROM metadata_owner_history h,json_tree(h.before_json) j WHERE j.type='text' AND length(j.value)=64
 UNION SELECT j.value FROM metadata_owner_history h,json_tree(h.after_json) j WHERE j.type='text' AND length(j.value)=64
 ), sources AS MATERIALIZED (SELECT s.small_blob,s.large_blob,s.ambient_blob FROM roots r JOIN artwork_sources s ON s.digest=r.digest)
 SELECT small_blob FROM sources UNION SELECT large_blob FROM sources UNION SELECT ambient_blob FROM sources`
