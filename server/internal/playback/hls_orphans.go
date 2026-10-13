package playback

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// sweepOrphanDirectories reclaims crash leftovers even below the global budget.
// Work is bounded to 128 directories per pass; a live session/producers own their
// output. No path saved in the database is followed.
func (h *HLS) sweepOrphanDirectories(ctx context.Context) {
	entries, err := os.ReadDir(h.root)
	if err != nil {
		return
	}
	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() || !generatedDirectoryID.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < 2*time.Minute {
			continue
		}
		if checked >= 128 || ctx.Err() != nil {
			return
		}
		checked++
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
