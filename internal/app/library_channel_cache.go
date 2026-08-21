package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	libraryChannelSegmentCacheMaxBytes   = int64(4 << 30)
	libraryChannelSegmentCacheMaxFiles   = 8192
	libraryChannelSegmentCacheMaxAge     = 6 * time.Hour
	libraryChannelSegmentCachePruneEvery = 5 * time.Minute
	libraryChannelSegmentTempMaxAge      = 10 * time.Minute
)

type libraryChannelSegmentCacheStatus struct {
	Bytes         int64  `json:"bytes"`
	Files         int    `json:"files"`
	LimitBytes    int64  `json:"limitBytes"`
	LimitFiles    int    `json:"limitFiles"`
	MaxAgeSeconds int    `json:"maxAgeSeconds"`
	PinnedFiles   int    `json:"pinnedFiles"`
	LastPrunedAt  string `json:"lastPrunedAt,omitempty"`
}

func (s *Server) libraryChannelSegmentCacheRoot() string {
	return filepath.Join(s.cfg.AppDataDir, "library-channel-segment-cache")
}

func (s *Server) pinLibraryChannelSegmentCachePath(path string) func() {
	path = filepath.Clean(path)
	s.libraryChannelPlayoutMu.Lock()
	if s.libraryChannelCachePins == nil {
		s.libraryChannelCachePins = map[string]int{}
	}
	s.libraryChannelCachePins[path]++
	s.libraryChannelPlayoutMu.Unlock()
	return func() {
		s.libraryChannelPlayoutMu.Lock()
		if s.libraryChannelCachePins[path] <= 1 {
			delete(s.libraryChannelCachePins, path)
		} else {
			s.libraryChannelCachePins[path]--
		}
		s.libraryChannelPlayoutMu.Unlock()
	}
}

func (s *Server) maybePruneLibraryChannelSegmentCache(now time.Time, force bool) error {
	root := s.libraryChannelSegmentCacheRoot()
	s.libraryChannelPlayoutMu.Lock()
	defer s.libraryChannelPlayoutMu.Unlock()
	if !force && !s.libraryChannelCacheLastPrune.IsZero() && now.Sub(s.libraryChannelCacheLastPrune) < libraryChannelSegmentCachePruneEvery {
		return nil
	}
	s.libraryChannelCacheLastPrune = now
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	type candidate struct {
		path string
		size int64
		at   time.Time
	}
	files := []candidate{}
	dirs := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || !pathInsideRoot(path, root) {
			return err
		}
		if s.libraryChannelCachePins[filepath.Clean(path)] > 0 {
			files = append(files, candidate{path: path, size: info.Size(), at: info.ModTime()})
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".segment-") && now.Sub(info.ModTime()) > libraryChannelSegmentTempMaxAge {
			return os.Remove(path)
		}
		if now.Sub(info.ModTime()) > libraryChannelSegmentCacheMaxAge {
			return os.Remove(path)
		}
		files = append(files, candidate{path: path, size: info.Size(), at: info.ModTime()})
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	totalBytes := int64(0)
	for _, file := range files {
		totalBytes += file.size
	}
	if totalBytes > libraryChannelSegmentCacheMaxBytes || len(files) > libraryChannelSegmentCacheMaxFiles {
		sort.Slice(files, func(i, j int) bool {
			if files[i].at.Equal(files[j].at) {
				return files[i].path < files[j].path
			}
			return files[i].at.Before(files[j].at)
		})
		lowBytes := libraryChannelSegmentCacheMaxBytes * 9 / 10
		lowFiles := libraryChannelSegmentCacheMaxFiles * 9 / 10
		remainingFiles := len(files)
		for _, file := range files {
			if totalBytes <= lowBytes && remainingFiles <= lowFiles {
				break
			}
			if s.libraryChannelCachePins[filepath.Clean(file.path)] > 0 {
				continue
			}
			if err := os.Remove(file.path); err == nil || errors.Is(err, os.ErrNotExist) {
				totalBytes -= file.size
				remainingFiles--
			}
		}
	}
	for index := len(dirs) - 1; index >= 0; index-- {
		_ = os.Remove(dirs[index])
	}
	// Re-scan after removals so owner storage reporting is exact.
	s.libraryChannelCacheBytes, s.libraryChannelCacheFiles = 0, 0
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return walkErr
		}
		info, err := entry.Info()
		if err == nil && info.Mode().IsRegular() {
			s.libraryChannelCacheBytes += info.Size()
			s.libraryChannelCacheFiles++
		}
		return err
	})
	return nil
}

func (s *Server) noteLibraryChannelSegmentCached(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	s.libraryChannelPlayoutMu.Lock()
	s.libraryChannelCacheBytes += info.Size()
	s.libraryChannelCacheFiles++
	s.libraryChannelPlayoutMu.Unlock()
}

func (s *Server) libraryChannelSegmentCacheStatus() libraryChannelSegmentCacheStatus {
	_ = s.maybePruneLibraryChannelSegmentCache(time.Now().UTC(), true)
	s.libraryChannelPlayoutMu.Lock()
	defer s.libraryChannelPlayoutMu.Unlock()
	pinned := 0
	for _, count := range s.libraryChannelCachePins {
		pinned += count
	}
	status := libraryChannelSegmentCacheStatus{
		Bytes: s.libraryChannelCacheBytes, Files: s.libraryChannelCacheFiles,
		LimitBytes: libraryChannelSegmentCacheMaxBytes, LimitFiles: libraryChannelSegmentCacheMaxFiles,
		MaxAgeSeconds: int(libraryChannelSegmentCacheMaxAge.Seconds()), PinnedFiles: pinned,
	}
	if !s.libraryChannelCacheLastPrune.IsZero() {
		status.LastPrunedAt = s.libraryChannelCacheLastPrune.Format(time.RFC3339)
	}
	return status
}

func (s *Server) removeLibraryChannelSegmentCache(channelID string, revision int64) {
	root := s.libraryChannelSegmentCacheRoot()
	target := filepath.Join(root, safePathComponent(channelID))
	if revision > 0 {
		target = filepath.Join(target, strconv.FormatInt(revision, 10))
	}
	if !pathInsideRoot(target, root) {
		return
	}
	s.libraryChannelPlayoutMu.Lock()
	for path, count := range s.libraryChannelCachePins {
		if count > 0 && (path == target || strings.HasPrefix(path, target+string(filepath.Separator))) {
			s.libraryChannelPlayoutMu.Unlock()
			return
		}
	}
	_ = os.RemoveAll(target)
	s.libraryChannelCacheLastPrune = time.Time{}
	s.libraryChannelPlayoutMu.Unlock()
	_ = s.maybePruneLibraryChannelSegmentCache(time.Now().UTC(), true)
}
