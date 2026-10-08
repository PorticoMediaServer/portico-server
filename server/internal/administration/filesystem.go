package administration

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FilesystemRoot is a place the picker can start from. Windows publishes drive
// letters, macOS publishes the boot volume and /Volumes, Linux publishes / and
// the mount table's real mount points.
type FilesystemRoot struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

// FilesystemEntry is one directory (or, when files are requested, one file).
type FilesystemEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"`
	// Readable says the server could list it; an unreadable directory is still
	// shown so the owner can see why the picker stops there.
	Readable bool `json:"readable"`
	// Symlink is reported because following one changes what a scan will walk.
	Symlink bool  `json:"symlink"`
	Bytes   int64 `json:"bytes,omitempty"`
}

// FilesystemPage is one bounded page of a directory listing.
type FilesystemPage struct {
	// Path is empty when the caller asked for the roots.
	Path       string            `json:"path"`
	Parent     string            `json:"parent"`
	Separator  string            `json:"separator"`
	Platform   string            `json:"platform"`
	Roots      []FilesystemRoot  `json:"roots"`
	Entries    []FilesystemEntry `json:"entries"`
	NextCursor string            `json:"nextCursor"`
	// Writable reports whether the server can create files here, which is what a
	// recordings or backup folder actually needs.
	Writable bool `json:"writable"`
	// Truncated says the root list or directory scan was bounded.
	Truncated bool `json:"truncated"`
}

// pickerNameBefore is the folder picker's total order: case-insensitive, then
// exact.
func pickerNameBefore(a, b string) bool {
	if la, lb := strings.ToLower(a), strings.ToLower(b); la != lb {
		return la < lb
	}
	return a < b
}

// maxDirectoryNames bounds the read of one directory so a folder with a million
// entries cannot hold a request open.
const maxDirectoryNames = 20000

// hiddenName filters the names a folder picker should never offer: dotfiles on
// every platform plus the system directories that are never media roots.
func hiddenName(parent, name string) bool {
	if name == "" || strings.HasPrefix(name, ".") {
		return true
	}
	lower := strings.ToLower(name)
	switch lower {
	case "$recycle.bin", "system volume information", "$winreagent", "recovery", "config.msi":
		return true
	}
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(parent)))
	// Kernel and device trees are never library roots and walking them is
	// actively harmful.
	for _, blocked := range []string{"/proc", "/sys", "/dev", "/run"} {
		if clean == blocked || strings.HasPrefix(clean, blocked+"/") {
			return true
		}
	}
	if clean == "/" && (lower == "proc" || lower == "sys" || lower == "dev" || lower == "run") {
		return true
	}
	return false
}

// systemPath refuses a browse into a tree that is not a media location and that
// would leak the shape of the host's private state.
func systemPath(path string) bool {
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	for _, blocked := range []string{"/proc", "/sys", "/dev", "/run"} {
		if clean == blocked || strings.HasPrefix(clean, blocked+"/") {
			return true
		}
	}
	return false
}

// Browse lists one directory, or the platform roots when path is empty. Entries
// are directories unless includeFiles is set; files are offered for pickers that
// choose a single file (a playlist, a guide) rather than a folder.
func (s *Service) Browse(ctx context.Context, path, token string, limit int, includeFiles bool) (FilesystemPage, error) {
	policy := s.pickerPolicy()
	out := FilesystemPage{Separator: string(os.PathSeparator), Platform: platformName(), Roots: []FilesystemRoot{}, Entries: []FilesystemEntry{}}
	if len(policy.Roots) == 0 {
		for _, root := range FilesystemRoots() {
			if policy.CheckBrowse(root.Path) == nil {
				out.Roots = append(out.Roots, root)
			}
		}
	} else {
		for _, root := range policy.SafeRoots() {
			out.Roots = append(out.Roots, FilesystemRoot{Path: root, Name: filepath.Base(root), Kind: "configured", Description: "Configured media root"})
		}
	}
	if len(out.Roots) > 64 {
		out.Roots, out.Truncated = out.Roots[:64], true
	}
	size, err := pageSize(limit)
	if err != nil {
		return out, err
	}
	c, err := decodeCursor("filesystem", token)
	if err != nil {
		return out, err
	}
	if path == "" {
		return out, nil
	}
	if !safeText(path, 4096) || !filepath.IsAbs(path) {
		return out, ErrInput
	}
	clean := filepath.Clean(path)
	if systemPath(clean) || policy.CheckBrowse(clean) != nil {
		return out, ErrDenied
	}
	clean, err = filepath.EvalSymlinks(clean)
	if err != nil {
		return out, ErrNotFound
	}
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() {
		return out, ErrNotFound
	}
	out.Path = clean
	if parent := filepath.Dir(clean); parent != clean && policy.CheckBrowse(parent) == nil {
		out.Parent = parent
	}
	// Listing is read-only. A browse must never update a media share's mtime.
	directory, err := os.Open(clean)
	if err != nil {
		return out, ErrDenied
	}
	defer directory.Close()
	names, err := directory.Readdirnames(maxDirectoryNames + 1)
	if err != nil && !errors.Is(err, io.EOF) && len(names) == 0 {
		return out, ErrDenied
	}
	if len(names) > maxDirectoryNames {
		names, out.Truncated = names[:maxDirectoryNames], true
	}
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if !hiddenName(clean, name) {
			kept = append(kept, name)
		}
	}
	// Case-insensitive order, with the exact name breaking ties, so "Films" and
	// "films" (distinct folders on Linux) are both reachable across a page
	// boundary: the cursor resumes strictly after the last name served.
	sort.Slice(kept, func(i, j int) bool { return pickerNameBefore(kept[i], kept[j]) })
	for _, name := range kept {
		if c.ID != "" && !pickerNameBefore(c.ID, name) {
			continue
		}
		full := filepath.Join(clean, name)
		if policy.CheckBrowse(full) != nil {
			continue
		}
		entry := FilesystemEntry{Name: name, Path: full, Kind: "directory", Readable: true}
		link, err := os.Lstat(full)
		if err != nil {
			continue
		}
		entry.Symlink = link.Mode()&os.ModeSymlink != 0
		resolved, err := os.Stat(full)
		if err != nil {
			// A broken link or an entry the server cannot stat is still shown, so
			// the owner can see it exists and why it cannot be chosen.
			entry.Readable = false
			if !includeFiles {
				continue
			}
			entry.Kind = "unknown"
		} else if resolved.IsDir() {
			if handle, err := os.Open(full); err == nil {
				handle.Close()
			} else {
				entry.Readable = false
			}
		} else {
			if !includeFiles {
				continue
			}
			entry.Kind, entry.Bytes = "file", resolved.Size()
		}
		out.Entries = append(out.Entries, entry)
		if len(out.Entries) > size {
			out.Entries = out.Entries[:size]
			out.NextCursor = encodeCursor("filesystem", 0, out.Entries[size-1].Name)
			break
		}
	}
	return out, nil
}
