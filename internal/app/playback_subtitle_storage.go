package app

import (
	"context"
	"errors"
	"fmt"
)

const playbackSubtitleFileLimit int64 = 8 << 20

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
