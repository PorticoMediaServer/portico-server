package metadata

import (
	"context"
	"database/sql"
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
		s.compactionMu.Unlock()
		var old string
		err := s.db.QueryRowContext(ctx, `SELECT o.digest FROM artwork_objects o INDEXED BY artwork_objects_compaction WHERE o.status='ready' AND (o.bytes>500000 OR o.width>1920 OR o.height>1920) AND NOT EXISTS(SELECT 1 FROM artwork_compaction_failures f WHERE f.digest=o.digest) ORDER BY o.bytes DESC,o.digest LIMIT 1`).Scan(&old)
		return old, err
	}
	s.compactionMu.Lock()
	defer s.compactionMu.Unlock()
	if s.compactionEdge != policy.edge {
		s.compactionEdge, s.compactionCursor = policy.edge, ""
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
		return "", sql.ErrNoRows
	}
	for _, o := range page {
		s.compactionCursor = o.digest
		if o.status != "ready" || o.failed {
			continue
		}
		edge, ceiling := policy.edge, policy.ceiling
		if o.bytes > 500000 || max(o.width, o.height) > edge {
			derived, err := s.artworkDerivedEdge(ctx, o.digest)
			if err != nil {
				return "", err
			}
			if derived > 0 {
				edge, ceiling = derived, 500000
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
	return s.compactionEdge != artworkLargeEdge && s.compactionCursor != ""
}
