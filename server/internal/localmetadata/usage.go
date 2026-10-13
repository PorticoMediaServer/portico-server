package localmetadata

import (
	"context"
	"errors"
	"io"
	"os"
)

// ArtworkUsage returns the same byte ledger used to enforce the local-artwork
// limit. Imported provider caches are separate and do not count here.
func (s *Service) ArtworkUsage(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.artworkAccounted {
		return s.artworkBytes, nil
	}
	dir, err := os.Open(s.root)
	if err != nil {
		return 0, err
	}
	defer dir.Close()
	var total int64
	for {
		if err = ctx.Err(); err != nil {
			return 0, err
		}
		entries, e := dir.ReadDir(128)
		for _, entry := range entries {
			info, e := entry.Info()
			if e != nil {
				return 0, e
			}
			total += info.Size()
		}
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return 0, e
		}
	}
	s.artworkBytes, s.artworkAccounted = total, true
	return total, nil
}
