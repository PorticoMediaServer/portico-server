//go:build linux

package mediaexec

import (
	"os"
	"slices"
	"sync"
)

// This memo saves repeated ELF parsing, not custody checks. Every hit stats the
// exact executable/dependency aliases and searched directories again. Changes
// force the original bounded resolver; it still decides every mounted path.
type libraryWatch map[string]os.FileInfo

func (w libraryWatch) record(path string) {
	if _, exists := w[path]; exists {
		return
	}
	info, _ := os.Stat(path)
	// Permission and other errors also record nil. unchanged accepts nil only
	// for a still-missing path, so an unobservable search directory is not cached.
	w[path] = info
}

func (w libraryWatch) unchanged() bool {
	for path, previous := range w {
		now, err := os.Stat(path)
		if previous == nil {
			if !os.IsNotExist(err) {
				return false
			}
			continue
		}
		if err != nil || !os.SameFile(previous, now) || previous.Mode() != now.Mode() || previous.Size() != now.Size() || !previous.ModTime().Equal(now.ModTime()) {
			return false
		}
	}
	return true
}

type libraryClosure struct {
	paths []string
	watch libraryWatch
}

var libraryClosures = struct {
	sync.Mutex
	entries map[string]*libraryClosure
}{entries: map[string]*libraryClosure{}}

func cachedLibraryClosure(key string) []string {
	libraryClosures.Lock()
	entry := libraryClosures.entries[key]
	libraryClosures.Unlock()
	if entry == nil || !entry.watch.unchanged() {
		return nil
	}
	return slices.Clone(entry.paths)
}

func rememberLibraryClosure(key string, paths []string, watch libraryWatch) {
	libraryClosures.Lock()
	defer libraryClosures.Unlock()
	// Each closure already has maxLibraryFiles as a hard bound. Keep at most
	// 32 tool/explicit-library combinations, independent of request volume.
	if len(libraryClosures.entries) >= 32 {
		for oldest := range libraryClosures.entries {
			delete(libraryClosures.entries, oldest)
			break
		}
	}
	libraryClosures.entries[key] = &libraryClosure{slices.Clone(paths), watch}
}
