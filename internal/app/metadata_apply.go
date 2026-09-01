package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type metadataProviderIdentityProposal struct {
	Provider           string
	ExternalID         string
	ExternalType       string
	Confidence         float64
	ExplicitAcceptance bool
	// ProviderAsserted identifies an external ID stated directly by the
	// provider payload (for example, a cross-provider ID or a MusicBrainz
	// release relationship). It may establish the first identity below the
	// generic fuzzy-match floor, but never displaces a different accepted ID.
	ProviderAsserted bool
}

type stagedMetadataImage struct {
	Kind     string
	Provider string
	Path     string
}

type metadataApplyRequest struct {
	MediaID          string
	ExpectedRevision int
	Origin           metadataSourceKind
	Source           string
	Provider         string
	ActorUserID      string
	OperationID      string
	MarkRefreshed    bool
	RefreshIntent    metadataRefreshIntent
	Update           UpdateMediaRequest
	Identities       []metadataProviderIdentityProposal
	ProviderRich     *metadataProviderRichProposal
	StagedImages     []stagedMetadataImage
	// ArtworkMutation lets manual artwork endpoints participate in the same
	// revision transaction as canonical metadata, locks, evidence, and search.
	ArtworkMutation func(*sql.Tx, int, string) error
	StageHook       func(string) error
}

type metadataApplyResult struct {
	Revision int
	ETag     string
}

type metadataArtworkCommitLock struct {
	mu   sync.Mutex
	refs int
}

func (s *Server) acquireMetadataArtworkCommitLock(mediaID string) func() {
	key := strings.TrimSpace(mediaID)
	s.metadataArtworkCommitMu.Lock()
	if s.metadataArtworkCommitLocks == nil {
		s.metadataArtworkCommitLocks = map[string]*metadataArtworkCommitLock{}
	}
	lock := s.metadataArtworkCommitLocks[key]
	if lock == nil {
		lock = &metadataArtworkCommitLock{}
		s.metadataArtworkCommitLocks[key] = lock
	}
	lock.refs++
	s.metadataArtworkCommitMu.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		s.metadataArtworkCommitMu.Lock()
		lock.refs--
		if lock.refs == 0 && s.metadataArtworkCommitLocks[key] == lock {
			delete(s.metadataArtworkCommitLocks, key)
		}
		s.metadataArtworkCommitMu.Unlock()
	}
}

type metadataIdentityRepairResult struct {
	ClearedProviderIDs int
	ClearedCandidates  int
	Revision           int
}

// repairMetadataIdentity advances the canonical revision and clears the active
// identity decision set atomically. Historical revisions and provider
// snapshots remain immutable audit evidence; the new revision deliberately
// omits external-identity relationships and rebuilds its derived projections.
func (s *Server) repairMetadataIdentity(ctx context.Context, mediaID, actorUserID, operationID string) (metadataIdentityRepairResult, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return metadataIdentityRepairResult{}, errors.New("metadata repair media ID is required")
	}
	var result metadataIdentityRepairResult
	err := s.withUserTxTaggedForViewer(ctx, actorUserID, "", []string{"media", "metadata", "library-items", "search", "libraries"}, func(tx *sql.Tx) error {
		state, err := loadMetadataCanonicalStateTx(tx, mediaID)
		if err != nil {
			return err
		}
		baseRevision := state.Revision
		nextRevision := baseRevision + 1
		now := time.Now().UTC().Format(time.RFC3339Nano)
		revisionID := randomID("mrev")
		if _, err := tx.Exec(`INSERT INTO media_metadata_revisions
			(id, media_id, revision, base_revision, state, trigger_kind, actor_user_id, started_at, completed_at, detail)
			VALUES (?, ?, ?, ?, 'applied', 'repair', ?, ?, ?, 'identity evidence cleared for rematch')`,
			revisionID, mediaID, nextRevision, baseRevision, strings.TrimSpace(actorUserID), now, now); err != nil {
			return fmt.Errorf("record metadata identity repair revision: %w", err)
		}
		if err := carryForwardMetadataEvidenceTx(tx, mediaID, baseRevision, revisionID, nextRevision, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM media_metadata_relationships WHERE revision_id = ? AND relationship_type = 'external_id'`, revisionID); err != nil {
			return fmt.Errorf("clear repaired identity relationships: %w", err)
		}
		providerResult, err := tx.Exec(`DELETE FROM media_provider_ids WHERE media_id = ?`, mediaID)
		if err != nil {
			return fmt.Errorf("clear provider identities: %w", err)
		}
		candidateResult, err := tx.Exec(`DELETE FROM media_match_candidates WHERE media_id = ?`, mediaID)
		if err != nil {
			return fmt.Errorf("clear identity candidates: %w", err)
		}
		state.Revision = nextRevision
		state.ETag, err = metadataCanonicalETag(state)
		if err != nil {
			return err
		}
		if err := persistMetadataCanonicalStateTx(tx, state, baseRevision); err != nil {
			return err
		}
		for _, statement := range []string{
			`UPDATE media_people SET metadata_revision = ? WHERE media_id = ?`,
			`UPDATE media_images SET metadata_revision = ? WHERE media_id = ?`,
			`UPDATE media_metadata_locks SET metadata_revision = ? WHERE media_id = ?`,
		} {
			if _, err := tx.Exec(statement, nextRevision, mediaID); err != nil {
				return err
			}
		}
		if err := replaceMediaSearchTx(ctx, tx, mediaID, nextRevision, "identity-repair"); err != nil {
			return err
		}
		if state.LibraryID != "" {
			if err := replaceMediaCategoryFacetsTx(ctx, tx, mediaID, nextRevision, "identity-repair"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`INSERT INTO media_metadata_refresh_outcomes
			(id, media_id, revision_id, operation_id, expected_revision, resulting_revision, status, detail, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'applied', 'identity evidence cleared and projections rebuilt', ?)`,
			randomID("mout"), mediaID, revisionID, strings.TrimSpace(operationID), baseRevision, nextRevision, now); err != nil {
			return fmt.Errorf("record metadata identity repair outcome: %w", err)
		}
		result = metadataIdentityRepairResult{ClearedProviderIDs: int(rowsAffected(providerResult)), ClearedCandidates: int(rowsAffected(candidateResult)), Revision: nextRevision}
		return nil
	})
	if err != nil {
		return metadataIdentityRepairResult{}, err
	}
	s.invalidateHomeCache()
	s.invalidateCategoryCache()
	return result, nil
}

type metadataCanonicalState struct {
	ID                  string
	LibraryID           string
	MediaType           string
	Title               string
	SortTitle           string
	OriginalTitle       string
	Edition             string
	Year                int
	DurationSeconds     int
	Summary             string
	Tagline             string
	ContentRating       string
	CommunityRating     float64
	CriticRating        int
	Studio              string
	Network             string
	Country             string
	Genres              []string
	Tags                []string
	Labels              []string
	SeasonNumber        int
	EpisodeNumber       int
	IndexNumber         int
	ArtSeed             string
	Artwork             map[string]string
	TypedMetadata       map[string]string
	SourceURL           string
	MetadataRefreshedAt string
	Revision            int
	ETag                string
}

func (s *Server) applyMetadata(ctx context.Context, req metadataApplyRequest) (metadataApplyResult, error) {
	if s == nil {
		return metadataApplyResult{}, errors.New("metadata server is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req.MediaID = strings.TrimSpace(req.MediaID)
	if req.MediaID == "" {
		return metadataApplyResult{}, errors.New("metadata media ID is required")
	}
	if req.Origin == "" {
		req.Origin = metadataSourceSystem
	}
	if req.ExpectedRevision < 0 {
		return metadataApplyResult{}, errors.New("metadata expected revision cannot be negative")
	}
	if err := callMetadataApplyStage(req.StageHook, "canonical"); err != nil {
		return metadataApplyResult{}, err
	}

	var result metadataApplyResult
	apply := func(tx *sql.Tx) error {
		state, err := loadMetadataCanonicalStateTx(tx, req.MediaID)
		if err != nil {
			return err
		}
		if state.Revision != req.ExpectedRevision {
			return fmt.Errorf("%w: media %s expected %d, found %d", errMetadataRevisionConflict, req.MediaID, req.ExpectedRevision, state.Revision)
		}
		locks, err := metadataLocksTx(tx, req.MediaID)
		if err != nil {
			return err
		}
		update := cloneMetadataUpdate(req.Update)
		if req.Origin != metadataSourceManual || req.MarkRefreshed {
			filterMetadataUpdateByLocks(&update, locks, state.TypedMetadata)
		}
		if err := filterMetadataUpdateForIntent(tx, state, &update, req.RefreshIntent); err != nil {
			return err
		}
		changed := metadataChangedFields(update)
		if req.ArtworkMutation != nil {
			changed["artwork"] = true
		}
		if err := applyMetadataUpdateToState(&state, update); err != nil {
			return err
		}
		nextRevision := state.Revision + 1
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if req.Origin != metadataSourceManual {
			state.MetadataRefreshedAt = now
		}
		state.Revision = nextRevision
		state.ETag, err = metadataCanonicalETag(state)
		if err != nil {
			return err
		}
		revisionID := randomID("mrev")
		trigger := metadataRevisionTrigger(req.Origin)
		primaryExternalType, primaryExternalID := primaryMetadataIdentity(req.Identities, req.Provider)
		if _, err := tx.Exec(`
			INSERT INTO media_metadata_revisions (
				id, media_id, revision, base_revision, state, trigger_kind, actor_user_id,
				provider, external_type, external_id, started_at, completed_at
			) VALUES (?, ?, ?, ?, 'applied', ?, ?, ?, ?, ?, ?, ?)`,
			revisionID, req.MediaID, nextRevision, req.ExpectedRevision, trigger,
			strings.TrimSpace(req.ActorUserID), normalizedMetadataProvider(req.Provider), primaryExternalType, primaryExternalID, now, now); err != nil {
			return fmt.Errorf("record metadata revision: %w", err)
		}
		if err := carryForwardMetadataEvidenceTx(tx, req.MediaID, req.ExpectedRevision, revisionID, nextRevision, now); err != nil {
			return err
		}
		if err := persistMetadataCanonicalStateTx(tx, state, req.ExpectedRevision); err != nil {
			return err
		}

		if err := callMetadataApplyStage(req.StageHook, "identities"); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE media_provider_ids SET evidence_revision = ?, updated_at = ? WHERE media_id = ?`, nextRevision, now, req.MediaID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE media_people SET metadata_revision = ? WHERE media_id = ?`, nextRevision, req.MediaID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE media_images SET metadata_revision = ? WHERE media_id = ?`, nextRevision, req.MediaID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE media_metadata_locks SET metadata_revision = ? WHERE media_id = ?`, nextRevision, req.MediaID); err != nil {
			return err
		}
		for _, identity := range req.Identities {
			if err := upsertMediaProviderIdentityTxWithPolicy(
				tx, req.MediaID, identity.Provider, identity.ExternalID, identity.ExternalType,
				identity.Confidence, firstNonEmpty(req.Source, string(req.Origin)), identity.ExplicitAcceptance,
				req.ActorUserID, now, identity.ProviderAsserted,
			); err != nil {
				return err
			}
			if _, err := tx.Exec(`
				UPDATE media_provider_ids SET evidence_revision = ?, updated_at = ?
				WHERE media_id = ? AND provider = ? AND external_type = ? AND external_id = ?`,
				nextRevision, now, req.MediaID, normalizedMetadataProvider(identity.Provider), strings.TrimSpace(identity.ExternalType), strings.TrimSpace(identity.ExternalID)); err != nil {
				return err
			}
		}
		if err := persistProviderRichEvidenceTx(tx, req.MediaID, revisionID, nextRevision, req, locks, now); err != nil {
			return err
		}

		if err := callMetadataApplyStage(req.StageHook, "people"); err != nil {
			return err
		}
		if update.People != nil {
			source := firstNonEmpty(strings.TrimSpace(req.Source), string(req.Origin))
			if req.Origin == metadataSourceProvider && normalizedMetadataProvider(req.Provider) != "" {
				source = normalizedMetadataProvider(req.Provider)
			}
			people := *update.People
			if req.Origin == metadataSourceManual {
				people = normalizeEditableMediaPeople(people)
				source = "manual"
			}
			if err := replaceScannedPeople(tx, req.MediaID, people, source, now); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE media_people SET metadata_revision = ? WHERE media_id = ? AND source = ?`, nextRevision, req.MediaID, source); err != nil {
				return err
			}
		}

		if err := callMetadataApplyStage(req.StageHook, "artwork"); err != nil {
			return err
		}
		if err := replaceStagedMetadataImagesTx(tx, req.MediaID, req.StagedImages, nextRevision, now); err != nil {
			return err
		}
		if req.ArtworkMutation != nil {
			if err := req.ArtworkMutation(tx, nextRevision, now); err != nil {
				return err
			}
		}

		if err := callMetadataApplyStage(req.StageHook, "locks"); err != nil {
			return err
		}
		nextLocks := locks
		if req.Origin == metadataSourceManual {
			if update.LockedFields != nil {
				nextLocks = metadataStringSet(normalizeMetadataLockFields(*update.LockedFields))
			} else {
				nextLocks = cloneStringSet(locks)
			}
			for field := range changed {
				if metadataFieldCanLock(field) {
					nextLocks[field] = true
				}
			}
			if err := replaceMetadataLocksTx(tx, req.MediaID, nextLocks, req.ActorUserID, nextRevision, now); err != nil {
				return err
			}
		}

		if update.ContentRating != nil {
			provider := normalizedMetadataProvider(req.Provider)
			if provider == "" {
				provider = string(req.Origin)
			}
			if err := upsertMediaRatingEvidenceTx(tx, req.MediaID, provider, firstNonEmpty(req.Source, string(req.Origin)), state.Country, state.ContentRating, now); err != nil {
				return err
			}
		}
		if err := writeMetadataEvidenceTx(tx, state, revisionID, req, changed, nextLocks, now); err != nil {
			return err
		}

		if err := callMetadataApplyStage(req.StageHook, "facets_search"); err != nil {
			return err
		}
		if err := replaceMediaSearchTx(ctx, tx, req.MediaID, nextRevision, firstNonEmpty(req.Source, string(req.Origin))); err != nil {
			return err
		}
		if err := replaceMediaCategoryFacetsTx(ctx, tx, req.MediaID, nextRevision, firstNonEmpty(req.Source, string(req.Origin))); err != nil {
			return err
		}

		if err := callMetadataApplyStage(req.StageHook, "refresh_outcome"); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO media_metadata_refresh_outcomes (
				id, media_id, revision_id, operation_id, provider, expected_revision,
				resulting_revision, status, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, 'applied', ?)`,
			randomID("mout"), req.MediaID, revisionID, strings.TrimSpace(req.OperationID),
			normalizedMetadataProvider(req.Provider), req.ExpectedRevision, nextRevision, now); err != nil {
			return fmt.Errorf("record metadata refresh outcome: %w", err)
		}
		result = metadataApplyResult{Revision: nextRevision, ETag: state.ETag}
		return nil
	}

	// A provider continuation may apply hundreds of item revisions. Publishing
	// broad catalog membership tags for every item makes every active browse and
	// Discover query refetch even though no item was added or removed. Detail
	// consumers still receive the metadata tag; manual edits retain the broad
	// immediate invalidation expected by the editing client.
	tags := metadataApplyInvalidationTags(req.Origin)
	var err error
	if req.Origin == metadataSourceManual {
		err = s.withUserTxTaggedForViewer(ctx, req.ActorUserID, "", tags, apply)
	} else {
		// The generic transaction publisher cannot attach the exact media ID and
		// would turn each provider item into a global metadata invalidation.
		err = s.withBackgroundTxTagged(ctx, nil, apply)
	}
	if err != nil {
		return metadataApplyResult{}, err
	}
	if req.Origin == metadataSourceManual {
		s.invalidateHomeCache()
		s.invalidateCategoryCache()
	} else {
		// A provider continuation can touch hundreds of items. Expire only the
		// changed detail; unrelated open pages retain their warm projections.
		s.invalidateMediaDetailCacheForMedia(req.MediaID)
		s.publishDataChanged("data.changed", tags, "media", req.MediaID, map[string]string{"source": string(req.Origin)})
	}
	return result, nil
}

func metadataApplyInvalidationTags(origin metadataSourceKind) []string {
	if origin == metadataSourceManual {
		return []string{"media", "metadata", "library-items", "search", "libraries"}
	}
	return []string{"metadata"}
}

func callMetadataApplyStage(hook func(string) error, stage string) error {
	if hook == nil {
		return nil
	}
	if err := hook(stage); err != nil {
		return fmt.Errorf("metadata apply %s: %w", stage, err)
	}
	return nil
}

func primaryMetadataIdentity(identities []metadataProviderIdentityProposal, provider string) (string, string) {
	provider = normalizedMetadataProvider(provider)
	for _, identity := range identities {
		if normalizedMetadataProvider(identity.Provider) == provider && strings.TrimSpace(identity.ExternalID) != "" {
			return strings.TrimSpace(identity.ExternalType), strings.TrimSpace(identity.ExternalID)
		}
	}
	for _, identity := range identities {
		if strings.TrimSpace(identity.ExternalID) != "" {
			return strings.TrimSpace(identity.ExternalType), strings.TrimSpace(identity.ExternalID)
		}
	}
	return "", ""
}

// carryForwardMetadataEvidenceTx makes every applied revision a complete,
// independently projectable evidence snapshot. Provider and canonical writers
// replace their owned rows later in the same transaction.
func carryForwardMetadataEvidenceTx(tx *sql.Tx, mediaID string, previousRevision int, revisionID string, revision int, now string) error {
	if previousRevision <= 0 {
		return nil
	}
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO media_metadata_field_values (
			id, media_id, revision_id, field_key, ordinal, locale, value_json, normalized_value,
			source_kind, provider, snapshot_id, confidence, decision, reason_code, created_at
		)
		SELECT 'mfld_' || lower(hex(randomblob(16))), f.media_id, ?, f.field_key, f.ordinal,
			f.locale, f.value_json, f.normalized_value, f.source_kind, f.provider, f.snapshot_id,
			f.confidence, f.decision, 'carried-forward', ?
		FROM media_metadata_field_values f
		JOIN media_metadata_revisions r ON r.id = f.revision_id
		WHERE f.media_id = ? AND r.revision = ?`, revisionID, now, mediaID, previousRevision); err != nil {
		return fmt.Errorf("carry metadata field evidence into revision %d: %w", revision, err)
	}
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO media_metadata_relationships (
			id, media_id, revision_id, relationship_type, target_kind, target_key, display_value,
			target_provider, target_external_id, language, country, role, ordinal, attributes_json,
			source_kind, source, provider, snapshot_id, confidence, decision, reason_code, created_at
		)
		SELECT 'mrel_' || lower(hex(randomblob(16))), rel.media_id, ?, rel.relationship_type,
			rel.target_kind, rel.target_key, rel.display_value, rel.target_provider,
			rel.target_external_id, rel.language, rel.country, rel.role, rel.ordinal,
			rel.attributes_json, rel.source_kind, rel.source, rel.provider, rel.snapshot_id,
			rel.confidence, rel.decision, 'carried-forward', ?
		FROM media_metadata_relationships rel
		JOIN media_metadata_revisions r ON r.id = rel.revision_id
		WHERE rel.media_id = ? AND r.revision = ?`, revisionID, now, mediaID, previousRevision); err != nil {
		return fmt.Errorf("carry metadata relationship evidence into revision %d: %w", revision, err)
	}
	return nil
}

func loadMetadataCanonicalStateTx(tx *sql.Tx, mediaID string) (metadataCanonicalState, error) {
	var state metadataCanonicalState
	var genresJSON, tagsJSON, labelsJSON, artworkJSON, typedJSON string
	err := tx.QueryRow(`
		SELECT id, COALESCE(library_id, ''), type, title, sort_title, original_title, edition,
			year, duration_seconds, summary, tagline, content_rating, community_rating,
			critic_rating, studio, network, country, genres_json, tags_json, labels_json,
			season_number, episode_number, index_number, art_seed, artwork_json,
			typed_metadata_json, source_url, metadata_refreshed_at, metadata_revision, metadata_etag
		FROM media_items WHERE id = ?`, mediaID).Scan(
		&state.ID, &state.LibraryID, &state.MediaType, &state.Title, &state.SortTitle,
		&state.OriginalTitle, &state.Edition, &state.Year, &state.DurationSeconds,
		&state.Summary, &state.Tagline, &state.ContentRating, &state.CommunityRating,
		&state.CriticRating, &state.Studio, &state.Network, &state.Country,
		&genresJSON, &tagsJSON, &labelsJSON, &state.SeasonNumber, &state.EpisodeNumber,
		&state.IndexNumber, &state.ArtSeed, &artworkJSON, &typedJSON, &state.SourceURL,
		&state.MetadataRefreshedAt, &state.Revision, &state.ETag,
	)
	if err != nil {
		return metadataCanonicalState{}, err
	}
	_ = json.Unmarshal([]byte(genresJSON), &state.Genres)
	_ = json.Unmarshal([]byte(tagsJSON), &state.Tags)
	_ = json.Unmarshal([]byte(labelsJSON), &state.Labels)
	_ = json.Unmarshal([]byte(artworkJSON), &state.Artwork)
	_ = json.Unmarshal([]byte(typedJSON), &state.TypedMetadata)
	state.Genres = normalizeStringList(state.Genres)
	state.Tags = normalizeStringList(state.Tags)
	state.Labels = normalizeStringList(state.Labels)
	state.Artwork = normalizeArtworkMap(state.Artwork)
	state.TypedMetadata = sanitizeTypedMetadataForType(state.MediaType, normalizeTypedMetadataMap(state.TypedMetadata))
	return state, nil
}

func persistMetadataCanonicalStateTx(tx *sql.Tx, state metadataCanonicalState, expectedRevision int) error {
	genresJSON, err := json.Marshal(state.Genres)
	if err != nil {
		return err
	}
	tagsJSON, err := json.Marshal(state.Tags)
	if err != nil {
		return err
	}
	labelsJSON, err := json.Marshal(state.Labels)
	if err != nil {
		return err
	}
	artworkJSON, err := json.Marshal(state.Artwork)
	if err != nil {
		return err
	}
	typedJSON, err := json.Marshal(state.TypedMetadata)
	if err != nil {
		return err
	}
	result, err := tx.Exec(`
		UPDATE media_items SET
			title = ?, sort_title = ?, original_title = ?, edition = ?, year = ?, duration_seconds = ?,
			summary = ?, tagline = ?, content_rating = ?, community_rating = ?, critic_rating = ?,
			studio = ?, network = ?, country = ?, genres_json = ?, tags_json = ?, labels_json = ?,
			season_number = ?, episode_number = ?, index_number = ?, art_seed = ?, artwork_json = ?,
			typed_metadata_json = ?, source_url = ?, metadata_refreshed_at = ?, metadata_revision = ?, metadata_etag = ?
		WHERE id = ? AND metadata_revision = ?`,
		state.Title, state.SortTitle, state.OriginalTitle, state.Edition, state.Year, state.DurationSeconds,
		state.Summary, state.Tagline, state.ContentRating, state.CommunityRating, state.CriticRating,
		state.Studio, state.Network, state.Country, string(genresJSON), string(tagsJSON), string(labelsJSON),
		state.SeasonNumber, state.EpisodeNumber, state.IndexNumber, state.ArtSeed, string(artworkJSON),
		string(typedJSON), state.SourceURL, state.MetadataRefreshedAt, state.Revision, state.ETag,
		state.ID, expectedRevision)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("%w: media %s changed while applying revision %d", errMetadataRevisionConflict, state.ID, state.Revision)
	}
	return nil
}

func applyMetadataUpdateToState(state *metadataCanonicalState, update UpdateMediaRequest) error {
	if state == nil {
		return errors.New("metadata canonical state is required")
	}
	if update.Title != nil {
		state.Title = strings.TrimSpace(*update.Title)
	}
	if update.SortTitle != nil {
		state.SortTitle = strings.TrimSpace(*update.SortTitle)
	}
	if update.OriginalTitle != nil {
		state.OriginalTitle = strings.TrimSpace(*update.OriginalTitle)
	}
	if update.Edition != nil {
		state.Edition = strings.TrimSpace(*update.Edition)
	}
	if update.Year != nil {
		state.Year = max(0, *update.Year)
	}
	if update.DurationSeconds != nil {
		const maximumMetadataDurationSeconds = 31 * 24 * 60 * 60
		if *update.DurationSeconds < 0 || *update.DurationSeconds > maximumMetadataDurationSeconds {
			return fmt.Errorf("duration must be between 0 and %d days", maximumMetadataDurationSeconds/(24*60*60))
		}
		state.DurationSeconds = *update.DurationSeconds
	}
	if update.Summary != nil {
		state.Summary = strings.TrimSpace(*update.Summary)
	}
	if update.Tagline != nil {
		state.Tagline = strings.TrimSpace(*update.Tagline)
	}
	if update.ContentRating != nil {
		state.ContentRating = strings.TrimSpace(*update.ContentRating)
	}
	if update.CommunityRating != nil {
		state.CommunityRating = math.Max(0, math.Min(10, *update.CommunityRating))
	}
	if update.CriticRating != nil {
		state.CriticRating = max(0, min(100, *update.CriticRating))
	}
	if update.Studio != nil {
		state.Studio = strings.TrimSpace(*update.Studio)
	}
	if update.Network != nil {
		state.Network = strings.TrimSpace(*update.Network)
	}
	if update.Country != nil {
		state.Country = strings.TrimSpace(*update.Country)
	}
	if update.Genres != nil {
		state.Genres = normalizeStringList(*update.Genres)
	}
	if update.Tags != nil {
		state.Tags = normalizeStringList(*update.Tags)
	}
	if update.Labels != nil {
		state.Labels = normalizeStringList(*update.Labels)
	}
	if update.SeasonNumber != nil {
		state.SeasonNumber = max(0, *update.SeasonNumber)
	}
	if update.EpisodeNumber != nil {
		state.EpisodeNumber = max(0, *update.EpisodeNumber)
	}
	if update.IndexNumber != nil {
		state.IndexNumber = max(0, *update.IndexNumber)
	}
	if update.ArtSeed != nil {
		state.ArtSeed = strings.TrimSpace(*update.ArtSeed)
	}
	if update.Artwork != nil {
		state.Artwork = normalizeArtworkMap(*update.Artwork)
	}
	if update.TypedMetadata != nil {
		state.TypedMetadata = sanitizeTypedMetadataForType(state.MediaType, normalizeTypedMetadataMap(*update.TypedMetadata))
	}
	if update.SourceURL != nil {
		state.SourceURL = strings.TrimSpace(*update.SourceURL)
	}
	if state.Title == "" {
		return errors.New("Title is required.")
	}
	if state.SortTitle == "" {
		state.SortTitle = state.Title
	}
	return nil
}

func cloneMetadataUpdate(update UpdateMediaRequest) UpdateMediaRequest {
	if update.Artwork != nil {
		value := cloneStringMap(*update.Artwork)
		update.Artwork = &value
	}
	if update.TypedMetadata != nil {
		value := cloneStringMap(*update.TypedMetadata)
		update.TypedMetadata = &value
	}
	if update.Genres != nil {
		value := append([]string(nil), (*update.Genres)...)
		update.Genres = &value
	}
	if update.Tags != nil {
		value := append([]string(nil), (*update.Tags)...)
		update.Tags = &value
	}
	if update.Labels != nil {
		value := append([]string(nil), (*update.Labels)...)
		update.Labels = &value
	}
	if update.People != nil {
		value := append([]MediaPerson(nil), (*update.People)...)
		update.People = &value
	}
	if update.LockedFields != nil {
		value := append([]string(nil), (*update.LockedFields)...)
		update.LockedFields = &value
	}
	return update
}

func cloneStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func metadataLocksTx(tx *sql.Tx, mediaID string) (map[string]bool, error) {
	rows, err := tx.Query(`SELECT field FROM media_metadata_locks WHERE media_id = ?`, mediaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	locks := map[string]bool{}
	for rows.Next() {
		var field string
		if err := rows.Scan(&field); err != nil {
			return nil, err
		}
		locks[field] = true
	}
	return locks, rows.Err()
}

func filterMetadataUpdateByLocks(update *UpdateMediaRequest, locks map[string]bool, currentTyped map[string]string) {
	if update == nil {
		return
	}
	for field := range locks {
		switch field {
		case "title":
			update.Title = nil
		case "sortTitle":
			update.SortTitle = nil
		case "originalTitle":
			update.OriginalTitle = nil
		case "edition":
			update.Edition = nil
		case "year":
			update.Year = nil
		case "durationSeconds":
			update.DurationSeconds = nil
		case "summary":
			update.Summary = nil
		case "tagline":
			update.Tagline = nil
		case "contentRating":
			update.ContentRating = nil
		case "communityRating":
			update.CommunityRating = nil
		case "criticRating":
			update.CriticRating = nil
		case "studio":
			update.Studio = nil
		case "network":
			update.Network = nil
		case "country":
			update.Country = nil
		case "genres":
			update.Genres = nil
		case "tags":
			update.Tags = nil
		case "labels":
			update.Labels = nil
		case "seasonNumber":
			update.SeasonNumber = nil
		case "episodeNumber":
			update.EpisodeNumber = nil
		case "indexNumber":
			update.IndexNumber = nil
		case "artwork":
			update.Artwork = nil
		case "people":
			update.People = nil
		case "typedMetadata":
			update.TypedMetadata = nil
		default:
			if strings.HasPrefix(field, "typedMetadata.") && update.TypedMetadata != nil {
				key := strings.TrimPrefix(field, "typedMetadata.")
				if value, ok := currentTyped[key]; ok {
					(*update.TypedMetadata)[key] = value
				} else {
					delete(*update.TypedMetadata, key)
				}
			}
		}
	}
}

func metadataChangedFields(update UpdateMediaRequest) map[string]bool {
	changed := map[string]bool{}
	checks := []struct {
		name string
		set  bool
	}{
		{"title", update.Title != nil}, {"sortTitle", update.SortTitle != nil},
		{"originalTitle", update.OriginalTitle != nil}, {"edition", update.Edition != nil},
		{"year", update.Year != nil}, {"durationSeconds", update.DurationSeconds != nil},
		{"summary", update.Summary != nil}, {"tagline", update.Tagline != nil},
		{"contentRating", update.ContentRating != nil}, {"communityRating", update.CommunityRating != nil},
		{"criticRating", update.CriticRating != nil}, {"studio", update.Studio != nil},
		{"network", update.Network != nil}, {"country", update.Country != nil},
		{"genres", update.Genres != nil}, {"tags", update.Tags != nil}, {"labels", update.Labels != nil},
		{"seasonNumber", update.SeasonNumber != nil}, {"episodeNumber", update.EpisodeNumber != nil},
		{"indexNumber", update.IndexNumber != nil}, {"artwork", update.Artwork != nil},
		{"people", update.People != nil}, {"typedMetadata", update.TypedMetadata != nil},
	}
	for _, check := range checks {
		if check.set {
			changed[check.name] = true
		}
	}
	if update.TypedMetadata != nil {
		for key := range *update.TypedMetadata {
			changed["typedMetadata."+key] = true
		}
	}
	return changed
}

func metadataFieldCanLock(field string) bool {
	if strings.HasPrefix(field, "typedMetadata.") {
		return true
	}
	for _, allowed := range normalizeMetadataLockFields([]string{field}) {
		if allowed == field {
			return true
		}
	}
	return false
}

func replaceMetadataLocksTx(tx *sql.Tx, mediaID string, locks map[string]bool, userID string, revision int, now string) error {
	if _, err := tx.Exec(`DELETE FROM media_metadata_locks WHERE media_id = ?`, mediaID); err != nil {
		return err
	}
	fields := make([]string, 0, len(locks))
	for field := range locks {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		kind := "scalar"
		switch field {
		case "people":
			kind = "credit"
		case "artwork":
			kind = "artwork"
		case "genres", "tags", "labels":
			kind = "relationship"
		}
		if _, err := tx.Exec(`
			INSERT INTO media_metadata_locks (
				media_id, field, value_json, source, user_id, updated_at, lock_kind, value_hash, metadata_revision
			) VALUES (?, ?, 'true', 'manual', ?, ?, ?, ?, ?)`,
			mediaID, field, userID, now, kind, metadataValueHash(true), revision); err != nil {
			return err
		}
	}
	return nil
}

func replaceStagedMetadataImagesTx(tx *sql.Tx, mediaID string, images []stagedMetadataImage, revision int, now string) error {
	for _, image := range images {
		image.Kind = strings.TrimSpace(image.Kind)
		image.Provider = normalizedMetadataProvider(image.Provider)
		image.Path = strings.TrimSpace(image.Path)
		if image.Kind == "" || image.Provider == "" || image.Path == "" {
			continue
		}
		var protected int
		if err := tx.QueryRow(`
			SELECT COUNT(*) FROM media_images
			WHERE media_id = ? AND image_type = ? AND preferred = 1
				AND NOT (source = 'provider' AND provider = ?)`, mediaID, image.Kind, image.Provider).Scan(&protected); err != nil {
			return err
		}
		preferred := 1
		if protected > 0 {
			preferred = 0
		}
		if _, err := tx.Exec(`
			UPDATE media_images SET selection_state = 'superseded', preferred = 0, metadata_revision = ?
			WHERE media_id = ? AND image_type = ? AND source = 'provider' AND provider = ? AND path <> ?`,
			revision, mediaID, image.Kind, image.Provider, image.Path); err != nil {
			return err
		}
		id := randomOpaquePublicID()
		if err := tx.QueryRow(`
			SELECT id FROM media_images
			WHERE media_id = ? AND image_type = ? AND source = 'provider' AND provider = ? AND path = ?
			LIMIT 1`, mediaID, image.Kind, image.Provider, image.Path).Scan(&id); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO media_images (
				id, media_id, image_type, source, provider, path, remote_url, width, height,
				language, rating, preferred, created_at, selection_state, confidence, observed_at, metadata_revision
			) VALUES (?, ?, ?, 'provider', ?, ?, '', 0, 0, '', 0, ?, ?, 'accepted', 1, ?, ?)
			ON CONFLICT(media_id, image_type, source, path, remote_url) DO UPDATE SET
				preferred = excluded.preferred, selection_state = 'accepted', observed_at = excluded.observed_at,
				metadata_revision = excluded.metadata_revision`,
			id, mediaID, image.Kind, image.Provider, image.Path, preferred, now, now, revision); err != nil {
			return err
		}
	}
	return nil
}

func writeMetadataEvidenceTx(tx *sql.Tx, state metadataCanonicalState, revisionID string, req metadataApplyRequest, changed, locks map[string]bool, now string) error {
	values := metadataCanonicalEvidenceValues(state)
	fields := make([]string, 0, len(changed))
	for field := range changed {
		if strings.HasPrefix(field, "typedMetadata.") || field == "people" || field == "artwork" {
			continue
		}
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		value, ok := values[field]
		if !ok {
			continue
		}
		valueJSON, _ := json.Marshal(value)
		decision := "accepted"
		if locks[field] {
			decision = "locked"
		}
		if _, err := tx.Exec(`DELETE FROM media_metadata_field_values WHERE revision_id = ? AND field_key = ?`, revisionID, field); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO media_metadata_field_values (
				id, media_id, revision_id, field_key, value_json, normalized_value,
				source_kind, provider, confidence, decision, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			randomID("mfld"), state.ID, revisionID, field, string(valueJSON), strings.ToLower(strings.TrimSpace(fmt.Sprint(value))),
			metadataEvidenceSourceKind(req.Origin), normalizedMetadataProvider(req.Provider), decision, now); err != nil {
			return err
		}
	}
	return replaceMetadataRelationshipEvidenceTx(tx, state, revisionID, req, changed, now)
}

func persistProviderRichEvidenceTx(tx *sql.Tx, mediaID, revisionID string, revision int, req metadataApplyRequest, locks map[string]bool, now string) error {
	if req.ProviderRich == nil {
		return nil
	}
	if err := persistOneProviderRichEvidenceTx(tx, mediaID, revisionID, revision, req, locks, now, req.ProviderRich); err != nil {
		return err
	}
	for index := range req.ProviderRich.Supplements {
		if err := persistOneProviderRichEvidenceTx(tx, mediaID, revisionID, revision, req, locks, now, &req.ProviderRich.Supplements[index]); err != nil {
			return err
		}
	}
	return nil
}

func persistOneProviderRichEvidenceTx(tx *sql.Tx, mediaID, revisionID string, revision int, req metadataApplyRequest, locks map[string]bool, now string, proposal *metadataProviderRichProposal) error {
	provider := normalizedMetadataProvider(firstNonEmpty(proposal.Provider, req.Provider))
	if provider == "" || len(proposal.Snapshot) == 0 || !json.Valid(proposal.Snapshot) {
		return errors.New("provider metadata snapshot is invalid")
	}
	storedHash := sha256.Sum256(proposal.Snapshot)
	if _, err := hex.DecodeString(strings.TrimSpace(proposal.SourceHash)); err != nil {
		return errors.New("provider metadata source snapshot digest is invalid")
	}
	if len(proposal.Snapshot) > metadataProviderSnapshotMaxBytes || !strings.EqualFold(hex.EncodeToString(storedHash[:]), strings.TrimSpace(proposal.SnapshotHash)) || proposal.SourceBytes < len(proposal.Snapshot) {
		return errors.New("provider metadata snapshot exceeds its bounded evidence contract")
	}
	if !proposal.SnapshotCut && (!strings.EqualFold(proposal.SourceHash, proposal.SnapshotHash) || proposal.SourceBytes != len(proposal.Snapshot)) {
		return errors.New("provider metadata source and stored snapshot integrity do not match")
	}
	externalID, externalType := "", ""
	for _, identity := range req.Identities {
		if normalizedMetadataProvider(identity.Provider) == provider && strings.TrimSpace(identity.ExternalID) != "" {
			externalID = strings.TrimSpace(identity.ExternalID)
			externalType = strings.TrimSpace(identity.ExternalType)
			break
		}
	}
	if externalID == "" {
		externalID = strings.TrimSpace(proposal.PrimaryExternalID)
		externalType = strings.TrimSpace(proposal.PrimaryExternalType)
	}
	if externalID == "" {
		return errors.New("provider metadata snapshot requires a primary external identity")
	}
	snapshotID := randomID("msnap")
	if _, err := tx.Exec(`
		INSERT INTO media_provider_snapshots (
			id, media_id, revision_id, provider, external_type, external_id, schema_version,
			mapping_version, payload_json, payload_sha256, byte_length, source_payload_sha256,
			source_byte_length, truncated, result_status, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshotID, mediaID, revisionID, provider, externalType, externalID,
		firstNonEmpty(strings.TrimSpace(proposal.MappingVersion), metadataProviderRichMappingVersion),
		string(proposal.Snapshot), strings.ToLower(strings.TrimSpace(proposal.SnapshotHash)), len(proposal.Snapshot),
		strings.ToLower(strings.TrimSpace(proposal.SourceHash)), proposal.SourceBytes, boolInt(proposal.SnapshotCut),
		map[bool]string{true: "degraded", false: "ok"}[proposal.SnapshotCut], now); err != nil {
		return fmt.Errorf("persist provider metadata snapshot: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE media_provider_ids SET snapshot_id = ?, evidence_revision = ?
		WHERE media_id = ? AND provider = ? AND external_type = ? AND external_id = ?`,
		snapshotID, revision, mediaID, provider, externalType, externalID); err != nil {
		return err
	}
	occupiedFields, occupiedRelationships, occupiedImages := map[string]bool{}, map[string]bool{}, map[string]bool{}
	if req.RefreshIntent == metadataRefreshFillMissing {
		rows, err := tx.Query(`SELECT DISTINCT lower(field_key) FROM media_metadata_field_values WHERE revision_id=? AND decision IN ('accepted','locked')`, revisionID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var field string
			if err := rows.Scan(&field); err != nil {
				rows.Close()
				return err
			}
			occupiedFields[strings.ToLower(strings.TrimSpace(field))] = true
		}
		if err := rows.Close(); err != nil {
			return err
		}
		rows, err = tx.Query(`SELECT DISTINCT lower(relationship_type) FROM media_metadata_relationships WHERE revision_id=? AND decision IN ('accepted','locked')`, revisionID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var kind string
			if err := rows.Scan(&kind); err != nil {
				rows.Close()
				return err
			}
			occupiedRelationships[strings.ToLower(strings.TrimSpace(kind))] = true
		}
		if err := rows.Close(); err != nil {
			return err
		}
		rows, err = tx.Query(`SELECT DISTINCT lower(image_type) FROM media_images WHERE media_id=? AND selection_state IN ('accepted','candidate')`, mediaID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var kind string
			if err := rows.Scan(&kind); err != nil {
				rows.Close()
				return err
			}
			occupiedImages[strings.ToLower(strings.TrimSpace(kind))] = true
		}
		if err := rows.Close(); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`
			DELETE FROM media_metadata_field_values AS f
			WHERE f.revision_id = ? AND f.provider = ?
				AND NOT EXISTS (
					SELECT 1 FROM media_metadata_locks l
					WHERE l.media_id = f.media_id AND (
						lower(l.field) = lower(f.field_key)
						OR (lower(f.field_key) IN ('alternatetitle','alias','title') AND l.field = 'alternateTitles')
						OR (lower(f.field_key) IN ('status','format','season','seasonyear','episodes','runtimeminutes','durationminutes','durationmilliseconds') AND l.field = 'typedMetadata')
					)
				)`, revisionID, provider); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			DELETE FROM media_metadata_relationships AS rel
			WHERE rel.revision_id = ? AND rel.provider = ?
				AND NOT EXISTS (
					SELECT 1 FROM media_metadata_locks l
					WHERE l.media_id = rel.media_id AND (
						l.field = 'relationship.' || rel.relationship_type
						OR (rel.relationship_type = 'genre' AND l.field = 'genres')
						OR (rel.relationship_type IN ('tag','keyword') AND l.field = 'tags')
						OR (rel.relationship_type IN ('studio','company') AND l.field = 'studio')
						OR (rel.relationship_type = 'network' AND l.field = 'network')
						OR (rel.relationship_type = 'country' AND l.field = 'country')
						OR (rel.relationship_type IN ('person','creator','character') AND l.field = 'people')
					)
				)`, revisionID, provider); err != nil {
			return err
		}
	}

	fieldOrdinals := map[string]int{}
	for _, value := range proposal.Values {
		field := strings.TrimSpace(value.Field)
		textValue := strings.TrimSpace(value.Value)
		if field == "" || textValue == "" || metadataRichFieldLocked(field, locks) || occupiedFields[strings.ToLower(field)] {
			continue
		}
		ordinal := 1000 + fieldOrdinals[field]
		fieldOrdinals[field]++
		encoded, _ := json.Marshal(textValue)
		if _, err := tx.Exec(`
			INSERT INTO media_metadata_field_values (
				id, media_id, revision_id, field_key, ordinal, locale, value_json, normalized_value,
				source_kind, provider, snapshot_id, confidence, decision, reason_code, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'provider', ?, ?, 1, 'accepted', '', ?)`,
			randomID("mfld"), mediaID, revisionID, field, ordinal, strings.TrimSpace(value.Locale), string(encoded), strings.ToLower(textValue),
			provider, snapshotID, now); err != nil {
			return err
		}
	}

	for _, relationship := range proposal.Relationships {
		kind := canonicalMetadataRelationshipKind(relationship.Kind, relationship.Role)
		if kind == "" || metadataRichRelationshipLocked(kind, locks) || occupiedRelationships[strings.ToLower(kind)] {
			continue
		}
		name := strings.TrimSpace(relationship.Name)
		targetProvider, targetExternalID := firstMetadataExternalID(relationship.ExternalIDs)
		if name == "" && targetExternalID == "" {
			continue
		}
		targetKey := metadataRelationshipTargetKey(kind, name, relationship.Role, relationship.ExternalIDs)
		attributes := cloneStringMap(relationship.Attributes)
		for key, value := range relationship.ExternalIDs {
			attributes["externalId."+strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
		attributesJSON, _ := json.Marshal(attributes)
		language := firstNonEmpty(attributes["language"], attributes["iso639-1"])
		country := firstNonEmpty(attributes["country"], attributes["iso3166-1"])
		if _, err := tx.Exec(`
			INSERT INTO media_metadata_relationships (
				id, media_id, revision_id, relationship_type, target_kind, target_key, display_value,
				target_provider, target_external_id, language, country, role, ordinal, attributes_json, source_kind,
				source, provider, snapshot_id, confidence, decision, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'provider', ?, ?, ?, 1, 'accepted', ?)
			ON CONFLICT(revision_id, relationship_type, target_key, source_kind, provider, language, country, role)
			DO UPDATE SET display_value = excluded.display_value, ordinal = MIN(media_metadata_relationships.ordinal, excluded.ordinal), attributes_json = excluded.attributes_json`,
			randomID("mrel"), mediaID, revisionID, kind, strings.TrimSpace(relationship.Kind), targetKey, name,
			targetProvider, targetExternalID, language, country, strings.TrimSpace(relationship.Role), max(0, relationship.Order), string(attributesJSON),
			firstNonEmpty(req.Source, provider), provider, snapshotID, now); err != nil {
			return err
		}
	}

	clearedKinds := map[string]bool{}
	for index, image := range proposal.Images {
		kind := strings.TrimSpace(image.Kind)
		providerImageID := strings.TrimSpace(image.Path)
		if kind == "" || providerImageID == "" || locks["artwork"] || locks["artwork."+kind] || occupiedImages[strings.ToLower(kind)] {
			continue
		}
		if !clearedKinds[kind] {
			if _, err := tx.Exec(`DELETE FROM media_images WHERE media_id = ? AND image_type = ? AND source = 'provider' AND provider = ? AND selection_state = 'candidate'`, mediaID, kind, provider); err != nil {
				return err
			}
			clearedKinds[kind] = true
		}
		evidencePath := strings.TrimSpace(image.LocalPath)
		if evidencePath == "" {
			evidencePath = "provider-evidence:" + metadataValueHash(provider+"\x00"+kind+"\x00"+providerImageID)
		}
		confidence := math.Max(0, math.Min(1, image.VoteAverage/10))
		if image.VoteAverage <= 0 {
			confidence = 0
		}
		if _, err := tx.Exec(`
			INSERT INTO media_images (
				id, media_id, image_type, source, provider, path, remote_url, width, height,
				language, rating, preferred, created_at, sort_order, provider_image_id,
				selection_state, confidence, observed_at, snapshot_id, metadata_revision
			) VALUES (?, ?, ?, 'provider', ?, ?, '', ?, ?, ?, ?, 0, ?, ?, ?, 'candidate', ?, ?, ?, ?)`,
			randomOpaquePublicID(), mediaID, kind, provider, evidencePath, max(0, image.Width), max(0, image.Height),
			strings.TrimSpace(image.Locale), image.VoteAverage, now, index, providerImageID, confidence, now, snapshotID, revision); err != nil {
			return err
		}
	}
	return nil
}

func canonicalMetadataRelationshipKind(kind, role string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "alternatetitle", "alternate_title", "alias":
		return "alternate_title"
	case "spokenlanguage", "language":
		return "language"
	case "country":
		return "country"
	case "keyword":
		return "keyword"
	case "company":
		return "company"
	case "studio":
		return "studio"
	case "network":
		return "network"
	case "person":
		if strings.EqualFold(strings.TrimSpace(role), "creator") {
			return "creator"
		}
		return "person"
	case "character":
		return "character"
	case "collection":
		return "collection"
	case "franchise":
		return "franchise"
	case "contentrating", "certification":
		return "certification"
	case "genre":
		return "genre"
	case "tag":
		return "tag"
	case "status":
		return "status"
	case "externalid", "external_id":
		return "external_id"
	case "release":
		return "release"
	case "track":
		return "track"
	case "recording":
		return "recording"
	case "work":
		return "work"
	case "relatedmedia", "related_media":
		return "related_media"
	case "artwork":
		return "artwork"
	case "providercoverage", "provider_coverage":
		return "provider_coverage"
	case "label":
		return "label"
	case "medium":
		return "medium"
	case "format":
		return "format"
	default:
		return ""
	}
}

func metadataRelationshipTargetKey(kind, name, role string, externalIDs map[string]string) string {
	cleaned := cleanStringMap(externalIDs)
	encoded, _ := json.Marshal(cleaned)
	base := strings.ToLower(strings.TrimSpace(kind + ":" + name + ":" + role))
	if len(cleaned) == 0 {
		return base
	}
	digest := sha256.Sum256(encoded)
	provider, externalID := firstMetadataExternalID(cleaned)
	return "provider:" + provider + ":" + externalID + ":" + hex.EncodeToString(digest[:8])
}

func providerRichImageURL(provider, imageID string) string {
	imageID = strings.TrimSpace(imageID)
	if strings.HasPrefix(imageID, "https://") {
		return imageID
	}
	if normalizedMetadataProvider(provider) == "tmdb" && strings.HasPrefix(imageID, "/") {
		return "https://image.tmdb.org/t/p/original" + imageID
	}
	return ""
}

func firstMetadataExternalID(values map[string]string) (string, string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return normalizedMetadataProvider(key), value
		}
	}
	return "", ""
}

func metadataRichFieldLocked(field string, locks map[string]bool) bool {
	if locks[field] {
		return true
	}
	switch strings.ToLower(field) {
	case "alternatetitle", "alias":
		return locks["alternateTitles"]
	case "status", "format", "season", "seasonyear", "episodes", "runtimeminutes", "durationminutes", "durationmilliseconds":
		return locks["typedMetadata"] || locks["typedMetadata."+field]
	default:
		return false
	}
}

func metadataRichRelationshipLocked(kind string, locks map[string]bool) bool {
	if locks["relationship."+kind] {
		return true
	}
	switch kind {
	case "genre":
		return locks["genres"]
	case "tag", "keyword":
		return locks["tags"]
	case "studio", "company":
		return locks["studio"]
	case "network":
		return locks["network"]
	case "country":
		return locks["country"]
	case "person", "creator", "character":
		return locks["people"]
	case "alternate_title":
		return locks["alternateTitles"]
	case "artwork":
		return locks["artwork"]
	default:
		return false
	}
}

func replaceMetadataRelationshipEvidenceTx(tx *sql.Tx, state metadataCanonicalState, revisionID string, req metadataApplyRequest, changed map[string]bool, now string) error {
	type relationship struct {
		kind, key, display string
		ordinal            int
	}
	relationships := []relationship{}
	appendValues := func(kind string, values []string) {
		for ordinal, value := range normalizeStringList(values) {
			relationships = append(relationships, relationship{kind: kind, key: strings.ToLower(value), display: value, ordinal: ordinal})
		}
	}
	if changed["genres"] {
		appendValues("genre", state.Genres)
	}
	if changed["tags"] {
		appendValues("tag", state.Tags)
	}
	if changed["country"] && state.Country != "" {
		appendValues("country", []string{state.Country})
	}
	if changed["studio"] && state.Studio != "" {
		appendValues("studio", []string{state.Studio})
	}
	if changed["network"] && state.Network != "" {
		appendValues("network", []string{state.Network})
	}
	for _, kind := range []string{"genre", "tag", "country", "studio", "network"} {
		field := map[string]string{"genre": "genres", "tag": "tags", "country": "country", "studio": "studio", "network": "network"}[kind]
		if changed[field] {
			if _, err := tx.Exec(`DELETE FROM media_metadata_relationships WHERE revision_id = ? AND relationship_type = ?`, revisionID, kind); err != nil {
				return err
			}
		}
	}
	for _, relationship := range relationships {
		if _, err := tx.Exec(`
			INSERT INTO media_metadata_relationships (
				id, media_id, revision_id, relationship_type, target_key, display_value, ordinal,
				source_kind, source, provider, confidence, decision, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'accepted', ?)`,
			randomID("mrel"), state.ID, revisionID, relationship.kind, relationship.key,
			relationship.display, relationship.ordinal, metadataEvidenceSourceKind(req.Origin),
			firstNonEmpty(req.Source, string(req.Origin)), normalizedMetadataProvider(req.Provider), now); err != nil {
			return err
		}
	}
	return nil
}

func metadataCanonicalEvidenceValues(state metadataCanonicalState) map[string]any {
	return map[string]any{
		"title": state.Title, "sortTitle": state.SortTitle, "originalTitle": state.OriginalTitle,
		"edition": state.Edition, "year": state.Year, "durationSeconds": state.DurationSeconds,
		"summary": state.Summary, "tagline": state.Tagline, "contentRating": state.ContentRating,
		"communityRating": state.CommunityRating, "criticRating": state.CriticRating,
		"studio": state.Studio, "network": state.Network, "country": state.Country,
		"genres": state.Genres, "tags": state.Tags, "labels": state.Labels,
		"seasonNumber": state.SeasonNumber, "episodeNumber": state.EpisodeNumber,
		"indexNumber": state.IndexNumber,
	}
}

func metadataCanonicalETag(state metadataCanonicalState) (string, error) {
	payload := struct {
		Revision int               `json:"revision"`
		Values   map[string]any    `json:"values"`
		Artwork  map[string]string `json:"artwork"`
		Typed    map[string]string `json:"typed"`
	}{state.Revision, metadataCanonicalEvidenceValues(state), state.Artwork, state.TypedMetadata}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return `"` + hex.EncodeToString(digest[:]) + `"`, nil
}

func metadataValueHash(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func metadataRevisionTrigger(origin metadataSourceKind) string {
	switch origin {
	case metadataSourceManual:
		return "manual"
	case metadataSourceProvider:
		return "provider"
	case metadataSourceScanner:
		return "scanner"
	case metadataSourceEmbedded:
		return "embedded"
	case metadataSourceNFO, metadataSourceFile:
		return "local"
	default:
		return "system"
	}
}

func metadataEvidenceSourceKind(origin metadataSourceKind) string {
	switch origin {
	case metadataSourceManual, metadataSourceProvider, metadataSourceEmbedded, metadataSourceNFO,
		metadataSourceFile, metadataSourceScanner, metadataSourceSystem:
		return string(origin)
	default:
		return string(metadataSourceSystem)
	}
}

func metadataStringSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	return set
}

func cloneStringSet(input map[string]bool) map[string]bool {
	output := make(map[string]bool, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func stageMetadataProviderArtwork(ctx context.Context, s *Server, mediaID string, artwork map[string]string) []stagedMetadataImage {
	provider := normalizedMetadataProvider(artwork["source"])
	if s == nil || provider == "" || provider == "local" {
		return nil
	}
	images := []stagedMetadataImage{}
	for _, kind := range []string{"poster", "backdrop", "thumb"} {
		if provider == "musicbrainz" && kind != "poster" {
			continue
		}
		remoteURL, ok := providerArtworkURL(artwork, kind)
		if !ok {
			continue
		}
		path, ok := s.cacheProviderOriginalArtwork(ctx, mediaID, kind, provider, remoteURL)
		if ok && strings.TrimSpace(path) != "" {
			images = append(images, stagedMetadataImage{Kind: kind, Provider: provider, Path: path})
		}
	}
	return images
}

// stageMetadataProviderRichOriginals implements cacheOriginalArtwork for
// non-selected provider candidates. Selected artwork is always ingested
// locally for reliable playback and privacy; this opt-in preserves a bounded
// set of additional original candidates for later owner selection.
func stageMetadataProviderRichOriginals(ctx context.Context, s *Server, mediaID string, proposal *metadataProviderRichProposal) {
	if s == nil || proposal == nil || !s.metadataAgentSettings().CacheOriginalArtwork {
		return
	}
	const maxOriginalCandidates = 32
	staged := 0
	stageMetadataProviderRichOriginalsBounded(ctx, s, mediaID, proposal, &staged, maxOriginalCandidates)
}

func stageMetadataProviderRichOriginalsBounded(ctx context.Context, s *Server, mediaID string, proposal *metadataProviderRichProposal, staged *int, limit int) {
	if proposal == nil || *staged >= limit {
		return
	}
	provider := normalizedMetadataProvider(proposal.Provider)
	if provider != "" {
		for index := range proposal.Images {
			if *staged >= limit {
				break
			}
			remoteURL := providerRichImageURL(provider, proposal.Images[index].Path)
			if remoteURL == "" {
				continue
			}
			if path, ok := s.cacheProviderOriginalArtwork(ctx, mediaID, proposal.Images[index].Kind, provider, remoteURL); ok {
				proposal.Images[index].LocalPath = path
				(*staged)++
			}
		}
	}
	for index := range proposal.Supplements {
		stageMetadataProviderRichOriginalsBounded(ctx, s, mediaID, &proposal.Supplements[index], staged, limit)
	}
}

func (s *Server) cleanupUnreferencedStagedMetadataArtwork(images []stagedMetadataImage, proposal *metadataProviderRichProposal, people ...[]MediaPerson) {
	if s == nil {
		return
	}
	paths := map[string]struct{}{}
	for _, image := range images {
		if path := strings.TrimSpace(image.Path); path != "" {
			paths[path] = struct{}{}
		}
	}
	if proposal != nil {
		collectStagedProviderRichPaths(proposal, paths)
	}
	for _, stagedPeople := range people {
		for _, person := range stagedPeople {
			if path := strings.TrimSpace(person.ImageURL); path != "" {
				paths[path] = struct{}{}
			}
		}
	}
	root := filepath.Join(s.cfg.AppDataDir, "artwork", "provider")
	for path := range paths {
		if !pathInsideRoot(path, root) {
			continue
		}
		var references int
		if err := s.db.QueryRow(`
			SELECT
				(SELECT COUNT(*) FROM media_images WHERE path = ?) +
				(SELECT COUNT(*) FROM media_people WHERE image_url = ?)`, path, path).Scan(&references); err == nil && references == 0 {
			_ = os.Remove(path)
		}
	}
}

func collectStagedProviderRichPaths(proposal *metadataProviderRichProposal, paths map[string]struct{}) {
	if proposal == nil {
		return
	}
	for _, image := range proposal.Images {
		if path := strings.TrimSpace(image.LocalPath); path != "" {
			paths[path] = struct{}{}
		}
	}
	for index := range proposal.Supplements {
		collectStagedProviderRichPaths(&proposal.Supplements[index], paths)
	}
}

func stageMetadataProviderPeople(ctx context.Context, s *Server, mediaID, provider string, people []MediaPerson) []MediaPerson {
	staged := append([]MediaPerson(nil), people...)
	provider = normalizedMetadataProvider(provider)
	if s == nil || provider == "" {
		return staged
	}
	for index := range staged {
		remoteURL := strings.TrimSpace(staged[index].ImageURL)
		staged[index].ImageURL = ""
		if remoteURL == "" {
			continue
		}
		identity := strings.TrimSpace(staged[index].Name) + "\x00" + strings.TrimSpace(staged[index].Role)
		if encoded, err := json.Marshal(staged[index].ProviderIDs); err == nil {
			identity += "\x00" + string(encoded)
		}
		digest := sha256.Sum256([]byte(identity))
		kind := "person-" + hex.EncodeToString(digest[:8])
		if path, ok := s.cacheProviderOriginalArtwork(ctx, mediaID, kind, provider, remoteURL); ok {
			staged[index].ImageURL = path
		}
	}
	return staged
}
