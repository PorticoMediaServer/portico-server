package app

import (
	"context"
	"errors"
	"fmt"
	"os"
)

const playbackSubtitleFileLimit int64 = 8 << 20

func readBoundedRegularFile(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return nil, fmt.Errorf("file exceeds the supported size limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds the supported size limit")
	}
	return data, nil
}

// readPlaybackSubtitleFile keeps sidecar/network subtitle I/O behind the same
// W3 admission, no-progress, and source-health boundary as media delivery. The
// explicit limit prevents a malformed subtitle selection from becoming an
// unbounded allocation.
func (s *Server) readPlaybackSubtitleFile(ctx context.Context, path string) ([]byte, error) {
	request := s.playbackStorageRequest(ctx, path, playbackStorageDirect, "stat subtitle")
	info, err := s.storageStat(ctx, request, path)
	if err != nil {
		return nil, classifyPlaybackStorageError(playbackStorageDirect, "stat subtitle", err)
	}
	if info.IsDir() || info.Size() < 0 || info.Size() > playbackSubtitleFileLimit {
		return nil, fmt.Errorf("subtitle resource exceeds the supported size limit")
	}
	request = s.playbackStorageRequest(ctx, path, playbackStorageDirect, "read subtitle")
	data, err := s.storageReadRange(ctx, request, path, 0, info.Size())
	if err != nil {
		return nil, classifyPlaybackStorageError(playbackStorageDirect, "read subtitle", err)
	}
	if len(data) == 0 {
		return nil, errors.New("subtitle resource is empty")
	}
	return data, nil
}
