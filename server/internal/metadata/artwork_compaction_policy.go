package metadata

import (
	"context"
	"database/sql"
	"strconv"
)

func (s *Service) artworkDerivedEdge(ctx context.Context, digest string) (int, error) {
	var small bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_selections WHERE thumbnail_digest=?) OR EXISTS(SELECT 1 FROM artwork_previews WHERE digest=?) OR EXISTS(SELECT 1 FROM artwork_uploads WHERE thumbnail_digest=?) OR EXISTS(SELECT 1 FROM artwork_variants WHERE digest=? AND width=400)`, digest, digest, digest, digest).Scan(&small)
	if err != nil || small {
		return 400, err
	}
	var medium bool
	err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_variants WHERE digest=? AND width=800)`, digest).Scan(&medium)
	if medium {
		return 800, err
	}
	return 0, err
}

// Standard keeps the existing candidate index. Other sizes use a bounded
// primary-key walk: no new index or complete object-table scan is required.
// The cursor is disposable scheduling progress, never persisted authority.
func (s *Service) nextArtworkCompaction(ctx context.Context, policy artworkEncodingPolicy) (string, error) {
	if policy.edge == artworkLargeEdge {
		s.compactionMu.Lock()
		s.compactionEdge, s.compactionCursor = policy.edge, ""
		s.compactionDone = false
		s.compactionMu.Unlock()
		var old string
		// The partial index still names the old 500,000 B cap, a superset of
		// today's candidates; objects inside the 1.5x allowance are legal
		// outliers and are filtered out, never re-encoded.
		err := s.db.QueryRowContext(ctx, `SELECT o.digest FROM artwork_objects o INDEXED BY artwork_objects_compaction WHERE o.status='ready' AND (o.bytes>500000 OR o.width>1920 OR o.height>1920) AND (o.bytes>`+strconv.Itoa(artworkLargeCap[artworkLargeEdge]*3/2)+` OR o.width>1920 OR o.height>1920) AND NOT EXISTS(SELECT 1 FROM artwork_compaction_failures f WHERE f.digest=o.digest) ORDER BY o.bytes DESC,o.digest LIMIT 1`).Scan(&old)
		return old, err
	}
	s.compactionMu.Lock()
	defer s.compactionMu.Unlock()
	if s.compactionEdge != policy.edge {
		s.compactionEdge, s.compactionCursor = policy.edge, ""
		s.compactionDone = false
	}
	// New acquisitions already capture this policy; immutable objects cannot
	// grow behind the completed walk. Restart only for a changed size or a new
	// Service after restore, rather than rescanning on unrelated commit wakes.
	if s.compactionDone {
		return "", sql.ErrNoRows
	}
	rows, err := s.db.QueryContext(ctx, `SELECT o.digest,o.width,o.height,o.bytes,o.status,EXISTS(SELECT 1 FROM artwork_compaction_failures f WHERE f.digest=o.digest) FROM artwork_objects o WHERE o.digest>? ORDER BY o.digest LIMIT 64`, s.compactionCursor)
	if err != nil {
		return "", err
	}
	type object struct {
		digest, status       string
		width, height, bytes int
		failed               bool
	}
	page := []object{}
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.digest, &o.width, &o.height, &o.bytes, &o.status, &o.failed); err != nil {
			rows.Close()
			return "", err
		}
		page = append(page, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	if len(page) == 0 {
		s.compactionCursor = ""
		s.compactionDone = true
		return "", sql.ErrNoRows
	}
	for _, o := range page {
		s.compactionCursor = o.digest
		if o.status != "ready" || o.failed {
			continue
		}
		// An object within its slot's 1.5x allowance is a legal outlier, not a
		// candidate: re-encoding it would only lose quality.
		edge, ceiling := policy.edge, policy.ceiling*3/2
		if o.bytes > ceiling || max(o.width, o.height) > edge {
			derived, err := s.artworkDerivedEdge(ctx, o.digest)
			if err != nil {
				return "", err
			}
			if derived > 0 {
				edge, ceiling = artworkSmallEdge, artworkSmallCap*3/2
			}
		}
		if o.bytes > ceiling || max(o.width, o.height) > edge {
			return o.digest, nil
		}
	}
	return "", nil
}

func (s *Service) artworkCompactionHasBacklog() bool {
	s.compactionMu.Lock()
	defer s.compactionMu.Unlock()
	return !s.compactionDone && s.compactionEdge != artworkLargeEdge && s.compactionCursor != ""
}
