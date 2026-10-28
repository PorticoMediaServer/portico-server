package catalog

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/operations"
)

const catalogMutationKind = "catalog-mutation"

// enqueueCatalogMutationTx admits the domain's existing operations-scheduler
// job in the same writer as the actual mutation. It stores no callback and
// uses the scheduler's domain capacity and event protocol.
func enqueueCatalogMutationTx(ctx context.Context, tx *sql.Tx, ref compactcatalog.MutationRef) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM console_operations WHERE kind=? AND resource=? AND state IN('queued','running','cancellation-requested','reconciling','paused'))`, catalogMutationKind, ref.ID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM console_operations WHERE trigger='domain' AND state IN('queued','running','cancellation-requested','reconciling','paused')`).Scan(&n); err != nil {
		return err
	}
	if n >= 10000 {
		return operations.ErrCapacity
	}
	now := time.Now().UnixMilli()
	_, err := tx.ExecContext(ctx, `INSERT INTO console_operations(id,kind,resource,actor,trigger,state,phase,revision,attempt,created_ms,updated_ms,next_ms,domain_id,error_code,predecessor,settings_revision) SELECT ?,?,?,'','domain','queued','catalog mutation',1,0,?,?,?,'','','',revision FROM console_settings_revision WHERE singleton=1`, "cm-"+ref.ID, catalogMutationKind, ref.ID, now, now, now)
	if err != nil {
		return err
	}
	return operations.AppendOperationTx(tx, "cm-"+ref.ID)
}

// EnqueueMBAdoptionTx MUST be part of the successful final album pin writer.
// It verifies the cause and actual accepted pin, and never enumerates matches.
func EnqueueMBAdoptionTx(ctx context.Context, tx *sql.Tx, album int64, albumIncarnation, releaseRevision, releaseID, causeOperation string) error {
	var policyInc string
	var policyRev int64
	err := tx.QueryRowContext(ctx, `SELECT p.incarnation,p.revision FROM catalog_entities e JOIN mb_album_heads h ON h.album_id=e.id JOIN mb_album_links l ON l.album_id=e.id JOIN catalog_libraries cl ON cl.id=e.library_id AND cl.retired=0 JOIN mb_provider_policies p ON p.library_id=cl.library_id AND p.enabled=1 WHERE e.id=? AND h.incarnation=? AND l.release_revision=? AND l.release_id=? AND EXISTS(SELECT 1 FROM mb_publication_operations o WHERE o.id=? AND o.entity_id=e.id AND o.kind='album' AND o.status IN('claimed','staged','applied'))`, album, albumIncarnation, releaseRevision, releaseID, causeOperation).Scan(&policyInc, &policyRev)
	if err != nil {
		return err
	}
	ref, err := compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: album, OwnerKind: "mb-adoption", OwnerID: policyInc + ":" + causeOperation, OwnerGeneration: policyRev, Mode: "adopt", Provider: "musicbrainz", ProviderKind: "release", ProviderID: releaseID, AcceptedRevision: releaseRevision, CauseOperation: causeOperation})
	if err != nil {
		return err
	}
	return enqueueCatalogMutationTx(ctx, tx, ref)
}
func enqueueIdentityAdoptionTx(ctx context.Context, tx *sql.Tx, entity int64, p compactcatalog.ProviderIdentity) error {
	var public string
	if err := tx.QueryRowContext(ctx, `SELECT pid(public_id) FROM catalog_entities WHERE id=?`, entity).Scan(&public); err != nil {
		return err
	}
	// Reuse an active root; a later real identity event gets a new generation.
	var ref compactcatalog.MutationRef
	err := tx.QueryRowContext(ctx, `SELECT mutation_id,generation FROM catalog_entity_mutations WHERE owner_kind='identity' AND owner_id=? AND status<>'terminal' AND provider=? AND provider_kind=? AND provider_id=?`, public, p.Provider, p.Kind, p.ID).Scan(&ref.ID, &ref.Generation)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var generation int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(owner_generation),0)+1 FROM catalog_entity_mutations WHERE owner_kind='identity' AND owner_id=?`, public).Scan(&generation); err != nil {
		return err
	}
	ref, err = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: entity, OwnerKind: "identity", OwnerID: public, OwnerGeneration: generation, Mode: "adopt", Provider: p.Provider, ProviderKind: p.Kind, ProviderID: p.ID})
	if err != nil {
		return err
	}
	return enqueueCatalogMutationTx(ctx, tx, ref)
}
func adoptionCurrentTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation) error {
	var current bool
	if m.OwnerKind == "mb-adoption" {
		// owner_id captures the policy incarnation as well as immutable cause.
		err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM mb_album_links l JOIN mb_album_heads h ON h.album_id=l.album_id JOIN catalog_entities e ON e.id=l.album_id JOIN catalog_libraries cl ON cl.id=e.library_id AND cl.retired=0 JOIN mb_provider_policies p ON p.library_id=cl.library_id AND p.enabled=1 WHERE l.album_id=? AND h.incarnation=? AND l.release_revision=? AND l.release_id=? AND p.incarnation||':'||?=? AND p.revision=?)`, m.Entity, m.Incarnation, m.AcceptedRevision, m.ProviderID, m.CauseOperation, m.OwnerID, m.OwnerGeneration).Scan(&current)
		if err != nil {
			return err
		}
	} else {
		err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_external_ids x JOIN catalog_entities e ON e.id=x.entity_id JOIN catalog_libraries cl ON cl.id=e.library_id AND cl.retired=0 WHERE x.entity_id=? AND x.provider=? AND x.provider_kind=? AND x.provider_id=?)`, m.Entity, m.Provider, m.ProviderKind, m.ProviderID).Scan(&current)
		if err != nil {
			return err
		}
	}
	if !current {
		return compactcatalog.ErrMutationStale
	}
	return nil
}
func AdoptProviderIdentityStepTx(ctx context.Context, tx *sql.Tx, ref compactcatalog.MutationRef) (compactcatalog.MutationStep, error) {
	m, step, err := checkedMutationTx(ctx, tx, ref)
	if err != nil || m.Status != "active" || step.Complete || step.Outcome == "blocked" {
		return step, err
	}
	ctx = compactcatalog.WithMutationFamily(ctx, m.Family)
	if err = adoptionCurrentTx(ctx, tx, m); err != nil {
		if errors.Is(err, compactcatalog.ErrMutationStale) {
			return staleMutationStepTx(ctx, tx, m)
		}
		return compactcatalog.MutationStep{}, err
	}
	if m.Child != "" {
		return advanceMutationChildTx(ctx, tx, m)
	}
	if m.Phase == "admitted" {
		if m.OwnerKind == "mb-adoption" {
			if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_external_ids WHERE entity_id=? AND provider='musicbrainz' AND provider_kind='release' AND provider_id<>?`, m.Entity, m.ProviderID); err != nil {
				return compactcatalog.MutationStep{}, err
			}
			if err = compactcatalog.RecordProviderIdentityTx(ctx, tx, m.Entity, compactcatalog.ProviderIdentity{Provider: m.Provider, Kind: m.ProviderKind, ID: m.ProviderID}); err != nil {
				return compactcatalog.MutationStep{}, err
			}
		}
		return compactcatalog.PendingMutationStep(), compactcatalog.SetMutationPhaseTx(ctx, tx, ref, "album-identity")
	}
	if m.Phase == "artist-identity" {
		// Exactly one release credit, matching the current artist's existing
		// identity policy. The child root captures the artist lifetime too.
		var artist int64
		var id string
		err = tx.QueryRowContext(ctx, `SELECT a.artist_id,c.artist_id FROM catalog_albums a JOIN mb_release_credits c ON c.revision_id=? AND c.ordinal=0 WHERE a.entity_id=? AND (SELECT count(*) FROM mb_release_credits WHERE revision_id=?)=1 AND c.artist_id<>'' AND NOT EXISTS(SELECT 1 FROM catalog_external_ids x WHERE x.entity_id=a.artist_id AND x.provider='musicbrainz' AND x.provider_kind='artist' AND x.provider_id<>c.artist_id)`, m.AcceptedRevision, m.Entity, m.AcceptedRevision).Scan(&artist, &id)
		if errors.Is(err, sql.ErrNoRows) {
			return compactcatalog.FinishEntityMutationTx(ctx, tx, ref, "retained")
		}
		if err != nil {
			return compactcatalog.MutationStep{}, err
		}
		p := compactcatalog.ProviderIdentity{Provider: "musicbrainz", Kind: "artist", ID: id}
		if err = compactcatalog.RecordProviderIdentityTx(ctx, tx, artist, p); err != nil {
			return compactcatalog.MutationStep{}, err
		}
		child, err := compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: artist, OwnerKind: "identity", OwnerID: m.Ref.ID, OwnerGeneration: m.OwnerGeneration, Mode: "adopt", Provider: p.Provider, ProviderKind: p.Kind, ProviderID: p.ID, Parent: m.Ref.ID, Family: m.Family})
		if err != nil {
			return compactcatalog.MutationStep{}, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE catalog_entity_mutations SET current_child=?,current_key=?,phase='finalize' WHERE mutation_id=? AND generation=?`, child.ID, artist, m.Ref.ID, m.Ref.Generation)
		return compactcatalog.PendingMutationStep(), err
	}
	if m.Phase == "finalize" {
		return compactcatalog.FinishEntityMutationTx(ctx, tx, ref, "retained")
	}
	var other int64
	err = tx.QueryRowContext(ctx, `SELECT other.id FROM catalog_entities me CROSS JOIN catalog_external_ids x INDEXED BY catalog_external_ids_lookup ON x.provider=?2 AND x.provider_kind=?3 AND x.provider_id=?4 CROSS JOIN catalog_entities other ON other.id=x.entity_id AND other.library_id=me.library_id AND other.kind=me.kind AND other.id<>me.id WHERE me.id=?1 AND other.id>?5 AND NOT EXISTS(SELECT 1 FROM catalog_kept_apart k WHERE k.entity_id IN(me.id,other.id)) AND (me.kind<>?6 OR EXISTS(SELECT 1 FROM catalog_shows mine JOIN catalog_shows theirs ON theirs.source_edition=mine.source_edition WHERE mine.entity_id=me.id AND theirs.entity_id=other.id)) ORDER BY other.id LIMIT 1`, m.Entity, m.Provider, m.ProviderKind, m.ProviderID, m.Cursor, compactcatalog.Show).Scan(&other)
	if err == nil {
		return compactcatalog.PendingMutationStep(), mutationChildTx(ctx, tx, m, other, m.Entity, "merge")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return compactcatalog.MutationStep{}, err
	}
	if err = restoreRetainedTx(ctx, tx, m.Entity); err != nil {
		return compactcatalog.MutationStep{}, err
	}
	if m.OwnerKind == "mb-adoption" {
		return compactcatalog.PendingMutationStep(), compactcatalog.SetMutationPhaseTx(ctx, tx, ref, "artist-identity")
	}
	return compactcatalog.FinishEntityMutationTx(ctx, tx, ref, "retained")
}
func (s *Service) MutationAdapter() operations.Adapter {
	return operations.Adapter{Kind: catalogMutationKind, Lane: "background-media", Resource: operations.LaneWriteHeavy,
		ValidateTx: func(ctx context.Context, tx *sql.Tx, id string) error {
			var one int
			return tx.QueryRowContext(ctx, `SELECT 1 FROM catalog_entity_mutations WHERE mutation_id=?`, id).Scan(&one)
		},
		Step: func(ctx context.Context, id string) (operations.JobObservation, error) {
			out := operations.JobObservation{State: "running", Phase: "catalog mutation"}
			var step compactcatalog.MutationStep
			err := dbwork.WithWriteTx(MutationWriteContext(ctx), s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
				ref, e := compactcatalog.MutationRefTx(ctx, tx, id)
				if e != nil {
					return e
				}
				m, e := compactcatalog.ReadEntityMutationTx(ctx, tx, ref)
				if e != nil {
					return e
				}
				// Automatic identity/MB custody is revalidated by the root on EVERY
				// turn, including while recursively draining its current child.
				// Manual requests have no persisted management grant or callback.
				if m.OwnerKind != "identity" && m.OwnerKind != "mb-adoption" {
					out.State = "paused"
					out.Phase = "authorized owner resume required"
					return nil
				}
				if m.OwnerKind == "identity" && m.Mode == "merge" {
					var current bool
					e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_identities i JOIN catalog_entities target ON target.public_id=i.public_id WHERE target.id=?1 AND i.source_key=?2 AND i.root=(SELECT cl.root FROM catalog_libraries cl WHERE cl.id=target.library_id)) AND NOT EXISTS(SELECT 1 FROM catalog_kept_apart WHERE entity_id IN(?1,?3)) AND NOT EXISTS(SELECT 1 FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id JOIN inventory_objects o ON o.asset_id=a.token JOIN library_sources s ON s.id=o.source_id WHERE l.entity_id=?3 AND (o.state<>'available' OR o.retired<>0 OR s.enabled=0 OR s.health IN('offline','root_changed','removing') OR EXISTS(SELECT 1 FROM inventory_source_active active WHERE active.source_id=s.id)))`, m.Target, m.ProviderID, m.Entity).Scan(&current)
					if e != nil {
						return e
					}
					if !current {
						step, e = staleMutationStepTx(ctx, tx, m)
						return e
					}
				}

				step, e = CatalogMutationStepTx(ctx, tx, ref)
				return e
			})
			if step.Complete {
				out.State = "succeeded"
				out.Phase = step.Outcome
			} else if !step.Progressed && step.Outcome == "blocked" {
				out.State = "paused"
				out.Phase = "mutation custody blocked"
			}
			return out, err
		}}
}
