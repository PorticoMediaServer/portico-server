package playback

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"
)

// sweepOrphanDirectories reclaims crash leftovers even below the global budget.
// Work is bounded to 128 entries per pass; a live session/producers own their
// output. No path saved in the database is followed.
func (h *HLS) sweepOrphanDirectories(ctx context.Context) {
	if h.db == nil {
		return
	}
	entries := h.nextOrphanEntries(ctx)
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		if !entry.IsDir() || !generatedDirectoryID.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < 2*time.Minute {
			continue
		}
		id := entry.Name()
		var exists bool
		if err := h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM playback_sessions WHERE id=?) OR EXISTS(SELECT 1 FROM playback_artifacts WHERE session_id=?) OR EXISTS(SELECT 1 FROM playback_hls_reservations WHERE session_id=?)`, id, id, id).Scan(&exists); err != nil || exists {
			continue
		}
		h.mu.Lock()
		if h.sessionActiveLocked(id) || h.cleaning[id] {
			h.mu.Unlock()
			continue
		}
		retired := filepath.Join(h.root, ".evicted-"+id+"-"+time.Now().Format("150405.000000000"))
		err = os.Rename(filepath.Join(h.root, id), retired)
		h.mu.Unlock()
		if err == nil {
			_ = os.RemoveAll(retired)
		}
	}
}

func (h *HLS) nextOrphanEntries(ctx context.Context) []os.DirEntry {
	h.orphanMu.Lock()
	defer h.orphanMu.Unlock()
	if ctx.Err() != nil {
		return nil
	}
	if h.orphanDir == nil {
		var err error
		h.orphanDir, err = os.Open(h.root)
		if err != nil {
			return nil
		}
	}
	entries, err := h.orphanDir.ReadDir(128)
	if err != nil {
		_ = h.orphanDir.Close()
		h.orphanDir = nil
		if err != io.EOF {
			return nil
		}
	}
	return entries
}

func (h *HLS) closeOrphanDirectory() {
	h.orphanMu.Lock()
	defer h.orphanMu.Unlock()
	if h.orphanDir != nil {
		_ = h.orphanDir.Close()
		h.orphanDir = nil
	}
}
