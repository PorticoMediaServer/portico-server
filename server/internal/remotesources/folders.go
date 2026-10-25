package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

// The folder explorer inside a saved WebDAV source lists one folder at a time,
// as the owner opens it, and downloads nothing. Its paths are the absolute
// paths the catalog stores for this source's folders, so a folder chosen here
// can be handed straight to the library that will use it.
type SourceFolder struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Relative string `json:"relative"`
	Hidden   bool   `json:"hidden"`
}

type SourceFolderPage struct {
	SourceID   string         `json:"sourceId"`
	Generation string         `json:"generation"`
	Path       string         `json:"path"`
	Relative   string         `json:"relative"`
	Entries    []SourceFolder `json:"entries"`
	NextCursor string         `json:"nextCursor"`
}

const (
	// The drawn explorer shows 500 folders at a time and a "Show more folders"
	// action; the local picker's page is 200. Both are bounded here.
	folderPageMax     = 500
	folderPageDefault = 200
	folderNameMax     = 255
)

func validFolderCursor(after string) bool {
	if after == "" {
		return true
	}
	return utf8.ValidString(after) && len(after) <= folderNameMax && !strings.ContainsAny(after, "/\\\x00") && strings.IndexFunc(after, unicode.IsControl) < 0
}

// folderRelative validates the path the owner typed or opened, inside the
// source. The empty string is the source's root; everything else must be a
// clean relative path, never an absolute one.
func folderRelative(directory string) (string, error) {
	if directory == "" {
		return "", nil
	}
	if len(directory) > 8192 || !utf8.ValidString(directory) || strings.ContainsAny(directory, "\\\x00%") || strings.IndexFunc(directory, unicode.IsControl) >= 0 ||
		strings.HasPrefix(directory, "/") || path.Clean(directory) != directory || directory == "." || directory == ".." || strings.HasPrefix(directory, "../") {
		return "", storage.ErrRemoteConfig
	}
	return directory, nil
}

// ListSourceFolders reads one folder of a saved WebDAV source from the storage
// itself. Only folders are returned: the explorer chooses libraries' folders,
// and listing files is what a scan does, not what opening a folder does.
func (s *Service) ListSourceFolders(ctx context.Context, identities *identity.Service, owner identity.Principal, source, directory string, showHidden bool, after string, limit int) (SourceFolderPage, error) {
	empty := SourceFolderPage{Entries: []SourceFolder{}}
	if s == nil || s.db == nil || s.mounts == nil || source == "" {
		return empty, storage.ErrRemoteConfig
	}
	if limit == 0 {
		limit = folderPageDefault
	}
	if limit < 1 || limit > folderPageMax || !validFolderCursor(after) {
		return empty, storage.ErrRemoteConfig
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer snapshot.Rollback()
	if err = currentLocalOwnerTx(ctx, snapshot.Tx(), identities, owner); err != nil {
		return empty, err
	}
	row, err := sourceRowTx(ctx, snapshot.Tx(), source)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty, storage.ErrRemoteOffline
		}
		return empty, err
	}
	relative, err := folderRelative(directory)
	if err != nil {
		return empty, err
	}
	b := binding{ID: row.ID, Kind: string(row.Kind), Root: row.RootPath}
	generation := strconv.FormatInt(row.Generation, 10)
	entries := make([]SourceFolder, 0, 32)
	switch row.Kind {
	case SourceWebDAV:
		d, current, err := s.dav(ctx, row.ID)
		if err != nil {
			s.health(b, generation, err)
			return empty, mapError(err)
		}
		if current != generation {
			return empty, storage.ErrRemoteChanged
		}
		deadline, cancel := context.WithTimeout(ctx, 35*time.Second)
		defer cancel()
		page, err := d.List(deadline, relative)
		if err != nil {
			s.health(b, generation, err)
			return empty, mapError(err)
		}
		s.health(b, generation, nil)
		for _, entry := range page.Entries {
			if !entry.Directory || entry.Relative == relative || entry.Relative == "" && relative == "" {
				continue
			}
			// Depth:1 should only answer direct children; a deeper href is not one.
			if path.Dir(entry.Relative) != relative && !(relative == "" && !strings.Contains(entry.Relative, "/")) {
				continue
			}
			name := path.Base(entry.Relative)
			if !validFolderName(name) || !showHidden && strings.HasPrefix(name, ".") {
				continue
			}
			entries = append(entries, SourceFolder{
				Name:     name,
				Path:     filepath.Join(row.RootPath, filepath.FromSlash(entry.Relative)),
				Relative: entry.Relative,
				Hidden:   strings.HasPrefix(name, "."),
			})
		}
	default:
		// A mounted remote is a normalized mount on this server; its folders are
		// read here exactly as the scanner will read them, never through the
		// network from this request.
		folders, err := mountedFolders(row.RootPath, relative, showHidden)
		if err != nil {
			return empty, err
		}
		entries = folders
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	out := SourceFolderPage{SourceID: row.ID, Generation: generation, Path: filepath.Join(row.RootPath, filepath.FromSlash(relative)), Relative: relative, Entries: entries, NextCursor: ""}
	if after != "" {
		kept := entries[:0]
		for _, entry := range entries {
			if entry.Name > after {
				kept = append(kept, entry)
			}
		}
		out.Entries = kept
	}
	if len(out.Entries) > limit {
		out.Entries = out.Entries[:limit]
	}
	if len(out.Entries) == limit && len(entries) > limit {
		out.NextCursor = out.Entries[len(out.Entries)-1].Name
	}
	entries = nil
	return out, nil
}

func sourceRowTx(ctx context.Context, tx *sql.Tx, id string) (Source, error) {
	var row Source
	err := tx.QueryRowContext(ctx, `SELECT id,kind,name,root,generation,state,error_code,range_support,origin,insecure,credential_present,mount_id FROM remote_sources WHERE id=? AND removed=0`, id).Scan(&row.ID, &row.Kind, &row.Name, &row.RootPath, &row.Generation, &row.State, &row.ErrorCode, &row.RangeSupport, &row.Origin, &row.InsecureLocal, &row.CredentialPresent, &row.MountID)
	return row, err
}

func validFolderName(name string) bool {
	return name != "" && name != "." && name != "/" && len(name) <= folderNameMax && utf8.ValidString(name) && strings.IndexFunc(name, unicode.IsControl) < 0
}

// mountedFolders lists the folders of a mounted remote from the mount the
// scanner itself will read. Symlinks are never followed: a folder offered here
// must stay inside the source the owner configured.
func mountedFolders(root, relative string, showHidden bool) ([]SourceFolder, error) {
	directory := root
	if relative != "" {
		directory = filepath.Join(root, filepath.FromSlash(relative))
		if !filepath.IsLocal(filepath.FromSlash(relative)) {
			return nil, storage.ErrRemoteConfig
		}
	}
	items, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return nil, storage.ErrRemoteOffline
		}
		return nil, storage.ErrRemoteOffline
	}
	out := make([]SourceFolder, 0, len(items))
	for _, item := range items {
		if !item.IsDir() || item.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := item.Name()
		if !validFolderName(name) || !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		inner := name
		if relative != "" {
			inner = relative + "/" + name
		}
		out = append(out, SourceFolder{
			Name:     name,
			Path:     filepath.Join(root, filepath.FromSlash(inner)),
			Relative: inner,
			Hidden:   strings.HasPrefix(name, "."),
		})
	}
	return out, nil
}
