package metadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/supervise"

	"portico.local/server/internal/metadataprovider"
)

// A pending new owner identity must not rediscover art from the previous
// provider_evidence row and stamp it with the new identity's authority.
func acceptedScreenArtwork(ctx context.Context, tx *sql.Tx, t RepairTarget) (*metadataprovider.ScreenRecord, bool, error) {
	var desired metadataprovider.ScreenID
	var accepted, raw string
	entity, err := entityid.Resolve(ctx, tx, t.ID)
	if errors.Is(err, entityid.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	err = tx.QueryRowContext(ctx, `SELECT provider,provider_type,provider_id,accepted_publication FROM screen_metadata_work WHERE target_kind=? AND target_id=?`, t.Kind, entity).Scan(&desired.Provider, &desired.Type, &desired.ID, &accepted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if accepted == "" {
		return nil, true, nil
	}
	if err = tx.QueryRowContext(ctx, `SELECT payload FROM screen_metadata_publications WHERE id=? AND decision='accepted'`, accepted).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return nil, true, nil
	}
	if err != nil {
		return nil, true, err
	}
	var r metadataprovider.ScreenRecord
	if err = json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, true, err
	}
	if desired != r.Identity {
		return nil, true, nil
	}
	return &r, true, nil
}

func seedScreenArtwork(ctx context.Context, tx *sql.Tx, t RepairTarget, r metadataprovider.ScreenRecord, fence, now string) error {
	for role, path := range map[string]string{"poster": r.PosterPath, "backdrop": r.BackdropPath, "still": r.StillPath} {
		origin := path
		if r.Identity.Provider == "tmdb" {
			origin = imageURL(path, "original")
		}
		// Other provider formats remain attributed evidence rather than arbitrary
		// URLs. Only the artwork transport's existing allowlisted providers qualify.
		if origin == "" || artworkProvider(origin) != r.Identity.Provider {
			continue
		}
		id, err := insertArtworkCandidate(ctx, tx, t, role, "", r.Identity.Provider, publicationDigest(origin), origin, "", artworkAttribution(r.Identity.Provider), fence, now, 0)
		if err != nil {
			return err
		}
		var occupied bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_selections a JOIN artwork_candidates c ON c.id=a.candidate_id WHERE a.kind=? AND a.entity_id=? AND a.role=? AND a.subject='' AND (a.locked=1 OR c.provider='local' OR c.source_fence=?)) OR EXISTS(SELECT 1 FROM artwork_jobs WHERE kind=? AND entity_id=? AND role=? AND subject='' AND (actor<>'' OR provider='local') AND status IN('pending','running','retry'))`, t.Kind, t.ID, role, fence, t.Kind, t.ID, role).Scan(&occupied)
		if err != nil {
			return err
		}
		if !occupied {
			if err = queueArtworkCandidate(ctx, tx, t, id, "", now, false, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func integratedArtworkAuthority(ctx context.Context, tx *sql.Tx, t RepairTarget, library string) (string, error) {
	h := sha256.New()
	enc := json.NewEncoder(h)
	var configuration int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT revision FROM library_configuration WHERE library_id=?),0)`, library).Scan(&configuration); err != nil {
		return "", err
	}
	_ = enc.Encode(configuration)
	rows, err := tx.QueryContext(ctx, `SELECT id,generation,incarnation,root_identity,identity_confirmed,enabled,health,configured_root,root FROM library_sources WHERE library_id=? ORDER BY id`, library)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var id, inc, rootID, health, configured, root string
		var gen int64
		var confirmed, enabled bool
		if err = rows.Scan(&id, &gen, &inc, &rootID, &confirmed, &enabled, &health, &configured, &root); err != nil {
			rows.Close()
			return "", err
		}
		_ = enc.Encode([]any{id, gen, inc, rootID, confirmed, enabled, health, configured, root})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	var screen string
	err = tx.QueryRowContext(ctx, `SELECT p.revision||':'||p.enabled||':'||c.revision||':'||c.confirmed FROM screen_metadata_policies p CROSS JOIN screen_metadata_consent c WHERE p.library_id=? AND c.singleton=1`, library).Scan(&screen)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_ = enc.Encode(screen)
	var selection string
	entity, err := artworkEntity(ctx, tx, t.ID)
	if err != nil {
		return "", err
	}
	err = tx.QueryRowContext(ctx, `SELECT provider||':'||provider_type||':'||provider_id||':'||selection_revision||':'||generation||':'||accepted_publication FROM screen_metadata_work WHERE target_kind=? AND target_id=?`, t.Kind, entity).Scan(&selection)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_ = enc.Encode(selection)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Long-running downloads are cancelled when source/selection/consent changes;
// the publication transaction still independently rechecks the full fence.
func (s *Service) artworkRequestContext(parent context.Context, t RepairTarget, provider, fence string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, 50*time.Second)
	supervise.Go("metadata.artwork-integration.watch", func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				gated, err := dbwork.BeginSnapshot(ctx, s.db)
				if err != nil {
					cancel()
					return
				}
				tx := gated.Tx()
				current, e := artworkFence(ctx, tx, t)
				enabled, p := s.artworkProviderEnabled(ctx, tx, t, provider)
				_ = gated.Rollback()
				if e != nil || p != nil || !enabled || current != fence {
					cancel()
					return
				}
			}
		}
	})
	return ctx, cancel
}
