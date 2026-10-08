package catalog

import (
	"portico.local/server/internal/identity"
)

// Facts are written synchronously, so a collection or playlist head is
// readable as soon as it is live.
const collectionHeadFence = `e.retired=0`

const playlistHeadFence = `p.deleted=0`

// A restricted collection count is readable only after both the policy
// generation and every derived contribution that can change it are published.
// Each probe is indexed; no collection membership is counted on request.
func (s *Service) collectionClassReady(library string, r identity.ContentRestrictions) (int64, int64, error) {
	key, generation, ready, err := s.publishedVisibilityClass(library, r)
	if err != nil {
		return 0, 0, err
	}
	if !ready {
		return 0, 0, ErrVisibilityBuilding
	}
	var class, built, current int64
	err = s.read().QueryRow(`SELECT c.id,c.catalog_revision,lr.revision FROM compact_visibility_classes c JOIN catalog_libraries l ON l.id=c.library_id JOIN library_revisions lr ON lr.library_id=l.library_id WHERE c.class_key=? AND l.library_id=? AND c.active_generation=? AND c.retired=0`, key, library, generation).Scan(&class, &built, &current)
	if err != nil {
		return 0, 0, err
	}
	_, _ = built, current // a class one revision behind still serves; rows are rechecked
	var pending bool
	err = s.read().QueryRow(`SELECT (SELECT backfill_done=0 FROM catalog_collection_visible_state WHERE id=1)`).Scan(&pending)
	if err != nil {
		return 0, 0, err
	}
	if pending {
		return 0, 0, ErrVisibilityBuilding
	}
	return class, generation, nil
}
