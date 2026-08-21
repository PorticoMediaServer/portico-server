package app

import (
	"context"
	"sort"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/catalogkind"
)

// metadataRecommendationEvidence is the storage-neutral input to the rich
// recommendation projection. Callers must supply the media item's current
// applied revision; rows from any other revision are deliberately ignored.
type metadataRecommendationEvidence struct {
	MediaID         string
	Kind            string
	CurrentRevision int
	Values          []metadataRecommendationValue
	Relationships   []metadataRecommendationRelationship
	Identities      []metadataRecommendationIdentity
}

type metadataRecommendationValue struct {
	Revision                                     int
	Field, NormalizedValue, SourceKind, Decision string
}

type metadataRecommendationRelationship struct {
	Revision                                                     int
	Type, StableKey, NormalizedValue, Role, SourceKind, Decision string
}

type metadataRecommendationIdentity struct {
	EvidenceRevision                           int
	Provider, ExternalType, ExternalID, Status string
}

// metadataRecommendationInput is intentionally independent of the current
// scoring implementation. It is a deterministic, normalized boundary that a
// future scorer can consume without reading stale provider evidence.
type metadataRecommendationInput struct {
	MediaID, Kind string
	Values        map[string][]string
	Relationships map[string][]string
	ProviderIDs   []string
}

func projectMetadataRecommendationInput(e metadataRecommendationEvidence) (metadataRecommendationInput, bool) {
	kind := string(catalogkind.Public(e.Kind))
	if strings.TrimSpace(e.MediaID) == "" || e.CurrentRevision <= 0 {
		return metadataRecommendationInput{}, false
	}
	switch catalogkind.EntityKind(kind) {
	case catalogkind.Movie, catalogkind.Show, catalogkind.Season, catalogkind.Episode,
		catalogkind.Artist, catalogkind.Album, catalogkind.Track, catalogkind.Author,
		catalogkind.AudiobookSeries, catalogkind.Book, catalogkind.Recording, catalogkind.Collection:
		// Catalog media and curated collections may contribute recommendation features.
	default:
		return metadataRecommendationInput{}, false
	}
	out := metadataRecommendationInput{MediaID: strings.TrimSpace(e.MediaID), Kind: kind, Values: map[string][]string{}, Relationships: map[string][]string{}}
	valueWinners := map[string]metadataRecommendationValue{}
	for _, row := range e.Values {
		field, value := normalizeRecommendationToken(row.Field), normalizeRecommendationToken(row.NormalizedValue)
		if row.Revision != e.CurrentRevision || field == "" || value == "" || !recommendationDecision(row.Decision) {
			continue
		}
		key := field + "\x00" + value
		if current, exists := valueWinners[key]; !exists || recommendationPriority(row.SourceKind, row.Decision) > recommendationPriority(current.SourceKind, current.Decision) {
			valueWinners[key] = row
		}
	}
	for key := range valueWinners {
		parts := strings.SplitN(key, "\x00", 2)
		out.Values[parts[0]] = append(out.Values[parts[0]], parts[1])
	}
	relWinners := map[string]metadataRecommendationRelationship{}
	for _, row := range e.Relationships {
		typ, value := normalizeRecommendationToken(row.Type), normalizeRecommendationToken(row.NormalizedValue)
		if row.Revision != e.CurrentRevision || typ == "" || value == "" || !recommendationDecision(row.Decision) {
			continue
		}
		stable := normalizeRecommendationToken(row.StableKey)
		if stable == "" {
			stable = value
		}
		key := typ + "\x00" + stable + "\x00" + normalizeRecommendationToken(row.Role)
		if current, exists := relWinners[key]; !exists || recommendationPriority(row.SourceKind, row.Decision) > recommendationPriority(current.SourceKind, current.Decision) {
			relWinners[key] = row
		}
	}
	for _, row := range relWinners {
		typ := normalizeRecommendationToken(row.Type)
		value := normalizeRecommendationToken(row.NormalizedValue)
		if role := normalizeRecommendationToken(row.Role); role != "" {
			value += "|" + role
		}
		out.Relationships[typ] = append(out.Relationships[typ], value)
	}
	for _, identity := range e.Identities {
		if identity.EvidenceRevision != e.CurrentRevision || strings.ToLower(strings.TrimSpace(identity.Status)) != "accepted" {
			continue
		}
		provider, externalID := normalizeRecommendationToken(identity.Provider), strings.TrimSpace(identity.ExternalID)
		if provider == "" || externalID == "" {
			continue
		}
		out.ProviderIDs = append(out.ProviderIDs, provider+":"+normalizeRecommendationToken(identity.ExternalType)+":"+externalID)
	}
	for _, values := range out.Values {
		sort.Strings(values)
	}
	for _, values := range out.Relationships {
		sort.Strings(values)
	}
	sort.Strings(out.ProviderIDs)
	return out, true
}

func normalizeRecommendationToken(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}
func recommendationDecision(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "accepted" || value == "locked"
}
func recommendationPriority(source, decision string) int {
	priority := 0
	if strings.EqualFold(strings.TrimSpace(source), "manual") {
		priority += 2
	}
	if strings.EqualFold(strings.TrimSpace(decision), "locked") {
		priority += 4
	}
	return priority
}

const metadataRecommendationQueryBatch = 400

// metadataRecommendationInputsContext loads complete, current-revision
// recommendation evidence in bounded batches. It never falls back to stale
// provider snapshots or candidate identities.
func (s *Server) metadataRecommendationInputsContext(ctx context.Context, mediaIDs []string) (map[string]metadataRecommendationInput, error) {
	mediaIDs = uniqueNonEmptyStrings(mediaIDs)
	result := make(map[string]metadataRecommendationInput, len(mediaIDs))
	for start := 0; start < len(mediaIDs); start += metadataRecommendationQueryBatch {
		end := min(len(mediaIDs), start+metadataRecommendationQueryBatch)
		batch := mediaIDs[start:end]
		args := make([]any, len(batch))
		for index, mediaID := range batch {
			args[index] = mediaID
		}
		evidence := make(map[string]*metadataRecommendationEvidence, len(batch))
		rows, err := s.queryUserRead(ctx, `SELECT id, type, metadata_revision FROM media_items WHERE id IN (`+sqlPlaceholders(len(batch))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var mediaID, kind string
			var revision int
			if err := rows.Scan(&mediaID, &kind, &revision); err != nil {
				rows.Close()
				return nil, err
			}
			evidence[mediaID] = &metadataRecommendationEvidence{MediaID: mediaID, Kind: kind, CurrentRevision: revision}
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}

		rows, err = s.queryUserRead(ctx, `
			SELECT value.media_id, revision.revision, value.field_key, value.normalized_value, value.source_kind, value.decision
			FROM media_metadata_field_values value
			JOIN media_metadata_revisions revision ON revision.id = value.revision_id AND revision.media_id = value.media_id
			JOIN media_items media ON media.id = value.media_id AND media.metadata_revision = revision.revision
			WHERE value.media_id IN (`+sqlPlaceholders(len(batch))+`) AND revision.state = 'applied'
				AND value.decision IN ('accepted','locked')`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var mediaID string
			var row metadataRecommendationValue
			if err := rows.Scan(&mediaID, &row.Revision, &row.Field, &row.NormalizedValue, &row.SourceKind, &row.Decision); err != nil {
				rows.Close()
				return nil, err
			}
			if item := evidence[mediaID]; item != nil {
				item.Values = append(item.Values, row)
			}
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}

		rows, err = s.queryUserRead(ctx, `
			SELECT relationship.media_id, revision.revision, relationship.relationship_type,
				relationship.target_key, relationship.display_value, relationship.role,
				relationship.source_kind, relationship.decision
			FROM media_metadata_relationships relationship
			JOIN media_metadata_revisions revision ON revision.id = relationship.revision_id AND revision.media_id = relationship.media_id
			JOIN media_items media ON media.id = relationship.media_id AND media.metadata_revision = revision.revision
			WHERE relationship.media_id IN (`+sqlPlaceholders(len(batch))+`) AND revision.state = 'applied'
				AND relationship.decision IN ('accepted','locked')`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var mediaID string
			var row metadataRecommendationRelationship
			if err := rows.Scan(&mediaID, &row.Revision, &row.Type, &row.StableKey, &row.NormalizedValue, &row.Role, &row.SourceKind, &row.Decision); err != nil {
				rows.Close()
				return nil, err
			}
			if item := evidence[mediaID]; item != nil {
				item.Relationships = append(item.Relationships, row)
			}
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}

		rows, err = s.queryUserRead(ctx, `
			SELECT identity.media_id, identity.evidence_revision, identity.provider,
				identity.external_type, identity.external_id, identity.status
			FROM media_provider_ids identity
			JOIN media_items media ON media.id = identity.media_id AND media.metadata_revision = identity.evidence_revision
			WHERE identity.media_id IN (`+sqlPlaceholders(len(batch))+`) AND identity.status = 'accepted'`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var mediaID string
			var row metadataRecommendationIdentity
			if err := rows.Scan(&mediaID, &row.EvidenceRevision, &row.Provider, &row.ExternalType, &row.ExternalID, &row.Status); err != nil {
				rows.Close()
				return nil, err
			}
			if item := evidence[mediaID]; item != nil {
				item.Identities = append(item.Identities, row)
			}
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		for mediaID, item := range evidence {
			if projected, ok := projectMetadataRecommendationInput(*item); ok {
				result[mediaID] = projected
			}
		}
	}
	return result, nil
}
