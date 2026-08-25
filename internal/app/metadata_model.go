package app

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type metadataSourceKind string

const automaticMetadataMinimumScore = 0.85

const (
	metadataSourceManual   metadataSourceKind = "manual"
	metadataSourceProvider metadataSourceKind = "provider"
	metadataSourceEmbedded metadataSourceKind = "embedded"
	metadataSourceNFO      metadataSourceKind = "nfo"
	metadataSourceFile     metadataSourceKind = "file"
	metadataSourceScanner  metadataSourceKind = "scanner"
	metadataSourceSystem   metadataSourceKind = "system"
)

type metadataIdentityStatus string

const (
	metadataIdentityCandidate  metadataIdentityStatus = "candidate"
	metadataIdentityAccepted   metadataIdentityStatus = "accepted"
	metadataIdentityRejected   metadataIdentityStatus = "rejected"
	metadataIdentitySuperseded metadataIdentityStatus = "superseded"
)

var errMetadataRevisionConflict = errors.New("metadata revision conflict")

func metadataSourceRequestsIdentityAcceptance(source string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), "manual")
}

// upsertMediaProviderIdentityTx preserves every observed provider identity.
// Automated evidence may establish the first accepted identity or reinforce
// the same identity, but it cannot displace an already accepted different ID.
// An explicit manual acceptance supersedes the prior winner transactionally.
func upsertMediaProviderIdentityTx(tx *sql.Tx, mediaID, provider, externalID, externalType string, confidence float64, source string, explicitAcceptance bool, acceptedByUserID, now string) error {
	return upsertMediaProviderIdentityTxWithPolicy(tx, mediaID, provider, externalID, externalType, confidence, source, explicitAcceptance, acceptedByUserID, now, false)
}

// upsertMediaProviderIdentityTxWithPolicy applies the normal automatic
// identity floor unless the provider has asserted the relationship directly
// in its payload. Provider-asserted identities can establish the first
// identity below that floor, but automated evidence still cannot replace a
// different accepted identity; only explicit manual acceptance can do that.
func upsertMediaProviderIdentityTxWithPolicy(tx *sql.Tx, mediaID, provider, externalID, externalType string, confidence float64, source string, explicitAcceptance bool, acceptedByUserID, now string, providerAsserted bool) error {
	if tx == nil {
		return errors.New("metadata identity transaction is required")
	}
	mediaID = strings.TrimSpace(mediaID)
	provider = normalizedMetadataProvider(provider)
	externalID = strings.TrimSpace(externalID)
	externalType = strings.TrimSpace(externalType)
	if mediaID == "" || provider == "" || externalID == "" {
		return nil
	}
	if validateTypedProviderIdentity(provider, externalID, externalType) != nil {
		return nil
	}
	if confidence < 0 {
		confidence = 0
	} else if confidence > 1 {
		confidence = 1
	}
	if strings.TrimSpace(now) == "" {
		now = time.Now().UTC().Format(time.RFC3339Nano)
	}

	var acceptedID string
	err := tx.QueryRow(`
		SELECT external_id
		FROM media_provider_ids
		WHERE media_id = ? AND provider = ? AND external_type = ? AND status = 'accepted'
		LIMIT 1`, mediaID, provider, externalType).Scan(&acceptedID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	status := metadataIdentityCandidate
	if (errors.Is(err, sql.ErrNoRows) && (confidence >= automaticMetadataMinimumScore || providerAsserted)) || acceptedID == externalID {
		status = metadataIdentityAccepted
	} else if explicitAcceptance {
		if _, err := tx.Exec(`
			UPDATE media_provider_ids
			SET status = 'superseded', updated_at = ?
			WHERE media_id = ? AND provider = ? AND external_type = ? AND status = 'accepted'`,
			now, mediaID, provider, externalType); err != nil {
			return err
		}
		status = metadataIdentityAccepted
	}
	acceptedAt := ""
	if status == metadataIdentityAccepted {
		acceptedAt = now
	}
	_, err = tx.Exec(`
		INSERT INTO media_provider_ids (
			media_id, provider, external_id, external_type, confidence, source, status,
			observed_at, accepted_at, accepted_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_id, provider, external_type, external_id) DO UPDATE SET
			confidence = MAX(media_provider_ids.confidence, excluded.confidence),
			source = excluded.source,
			status = CASE
				WHEN media_provider_ids.status = 'accepted' AND excluded.status <> 'accepted' THEN media_provider_ids.status
				ELSE excluded.status
			END,
			observed_at = excluded.observed_at,
			accepted_at = CASE WHEN excluded.status = 'accepted' THEN excluded.accepted_at ELSE media_provider_ids.accepted_at END,
			accepted_by_user_id = CASE WHEN excluded.status = 'accepted' THEN excluded.accepted_by_user_id ELSE media_provider_ids.accepted_by_user_id END,
			updated_at = excluded.updated_at`,
		mediaID, provider, externalID, externalType, confidence, strings.TrimSpace(source), status,
		now, acceptedAt, strings.TrimSpace(acceptedByUserID), now, now)
	if err != nil {
		return fmt.Errorf("upsert metadata provider identity: %w", err)
	}
	return nil
}

func validateTypedProviderIdentity(provider, externalID, externalType string) error {
	switch provider {
	case "tmdb":
		if id, err := strconv.Atoi(externalID); err != nil || id <= 0 {
			return fmt.Errorf("TMDB external ID must be a positive integer")
		}
		if !oneOfString(externalType, "movie", "tv") {
			return fmt.Errorf("TMDB external type %q is not supported", externalType)
		}
	case "tvdb":
		if id, err := strconv.Atoi(externalID); err != nil || id <= 0 {
			return fmt.Errorf("TheTVDB external ID must be a positive integer")
		}
		if !oneOfString(externalType, "movie", "series") {
			return fmt.Errorf("TheTVDB external type %q is not supported", externalType)
		}
	}
	return nil
}
