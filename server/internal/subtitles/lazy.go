package subtitles

import (
	"context"
	"portico.local/server/internal/identity"
	"strconv"
)

type lazyCacheKey struct{}
type lazyCacheTarget struct {
	resource, discovery, source string
	revision                    int64
}

// ensureCached fills only an already-authorized discovered resource. Publication
// rechecks exact source/inventory evidence, and changes bytes only on cache miss.
func (s *Service) ensureCached(ctx context.Context, p identity.Principal, item, id string, revision int64) error {
	snapshot, p, e := s.readTx(ctx, p, item)
	if e != nil {
		return e
	}
	r, e := scanResource(snapshot.Tx().QueryRowContext(ctx, `SELECT `+resourceColumns+` FROM subtitle_resources r JOIN subtitle_revisions v ON v.resource_id=r.id WHERE r.id=? AND v.revision=? AND r.item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, id, revision, item))
	if e != nil {
		snapshot.Rollback()
		return e
	}
	if !visible(r, p) {
		snapshot.Rollback()
		return identity.ErrUnauthorized
	}
	if r.digest != "" {
		snapshot.Rollback()
		return nil
	}
	var discoveryRevision int64
	e = snapshot.Tx().QueryRowContext(ctx, `SELECT discovery_revision FROM subtitle_resources WHERE id=? AND current_revision=? AND deleted=0`, id, revision).Scan(&discoveryRevision)
	snapshot.Rollback()
	if e != nil {
		return ErrConflict
	}
	ctx = context.WithValue(ctx, lazyCacheKey{}, lazyCacheTarget{id, r.DiscoveryID, r.SourceID, revision})
	_, e = s.Import(ctx, p, item, ImportRequest{OperationID: identity.Digest("cache:" + id + ":" + strconv.FormatInt(revision, 10)), SourceID: r.SourceID, DiscoveryID: r.DiscoveryID, ExpectedRevision: discoveryRevision, Scope: "shared", Render: "text"})
	return e
}
