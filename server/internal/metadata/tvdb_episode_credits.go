package metadata

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadataprovider"
)

// tvdbEpisodeCharacterProvider is the one TVDB call the episode-credits step
// needs. It is asserted, not part of TVDBProvider, so the show pipeline's
// fakes keep working unchanged; production's *TVDB implements it.
type tvdbEpisodeCharacterProvider interface {
	EpisodeCharacters(context.Context, int64) ([]metadataprovider.TVDBEpisodeCharacter, error)
}

// tvdbEpisodeCreditPage bounds one step: one upstream request per episode.
const tvdbEpisodeCreditPage = 8

// tvdbEpisodeCreditAttempts bounds retries for an episode TVDB cannot supply
// (a deleted upstream record must not be refetched forever).
const tvdbEpisodeCreditAttempts = 5

// tvdbEpisodeCreditsStep gives TVDB-matched episodes their cast. TVDB episodes
// are delegated past the screen pipeline (screenEpisodeStep answers
// delegated_tvdb), so nothing ever writes their screen credits; this step
// claims a bounded page of matched episodes with no credit evidence, fetches
// each one's /episodes/{id}/extended characters, and writes them as the
// episode item's tvdb credits. It runs when TVDBStep has no projection or
// acquisition work, so it never delays a show publication.
func (s *Service) tvdbEpisodeCreditsStep(ctx context.Context) error {
	provider, ok := s.tvdb.(tvdbEpisodeCharacterProvider)
	if !ok {
		return nil
	}
	type claim struct {
		item       int64
		providerID int64
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT pe.item_id,pe.provider_id FROM provider_evidence pe JOIN catalog_episodes e ON e.entity_id=pe.item_id JOIN catalog_entities i ON i.id=pe.item_id JOIN catalog_libraries cl ON cl.id=i.library_id JOIN tvdb_provider_policies tp ON tp.library_id=cl.library_id AND tp.enabled=1 WHERE pe.provider='tvdb' AND pe.provider_id>0 AND i.kind=4 AND NOT EXISTS(SELECT 1 FROM tvdb_episode_credit_evidence ce WHERE ce.item_id=pe.item_id AND ce.provider_id=pe.provider_id AND ce.credits IS NOT NULL) AND COALESCE((SELECT ce.attempts FROM tvdb_episode_credit_evidence ce WHERE ce.item_id=pe.item_id AND ce.provider_id=pe.provider_id),0)<? ORDER BY pe.item_id LIMIT ?`, tvdbEpisodeCreditAttempts, tvdbEpisodeCreditPage)
	if err != nil {
		return err
	}
	claims := []claim{}
	for rows.Next() {
		var c claim
		if err = rows.Scan(&c.item, &c.providerID); err != nil {
			rows.Close()
			return err
		}
		claims = append(claims, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if err = gated.Commit(); err != nil {
		return err
	}
	// Fetches run outside the claim transaction: one slow episode must not hold
	// the write gate while TVDB answers.
	type result struct {
		credits []metadataprovider.TVDBEpisodeCharacter
		err     error
	}
	results := make([]result, len(claims))
	for i, c := range claims {
		if err = ctx.Err(); err != nil {
			return err
		}
		results[i].credits, results[i].err = provider.EpisodeCharacters(ctx, c.providerID)
	}
	return dbwork.WithWriteTx(ctx, s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		stamp := tvdbStamp(s.publicationTime())
		for i, c := range claims {
			if results[i].err != nil {
				if _, err := tx.ExecContext(ctx, `INSERT INTO tvdb_episode_credit_evidence(item_id,provider_id,observed_at,attempts) VALUES(?,?,?,1) ON CONFLICT(item_id,provider_id) DO UPDATE SET observed_at=excluded.observed_at,attempts=tvdb_episode_credit_evidence.attempts+1`, c.item, c.providerID, stamp); err != nil {
					return err
				}
				continue
			}
			if err := writeTVDBEpisodeCredits(ctx, tx, c.item, results[i].credits); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO tvdb_episode_credit_evidence(item_id,provider_id,observed_at,credits,attempts) VALUES(?,?,?,COALESCE((SELECT count(*) FROM catalog_credits WHERE entity_id=? AND provider='tvdb'),0),0) ON CONFLICT(item_id,provider_id) DO UPDATE SET observed_at=excluded.observed_at,credits=excluded.credits,attempts=0`, c.item, c.providerID, stamp, c.item); err != nil {
				return err
			}
		}
		// A rematch leaves evidence for a pair the catalog no longer holds;
		// drop it so the table stays the size of the matched catalog.
		_, err := tx.ExecContext(ctx, `DELETE FROM tvdb_episode_credit_evidence WHERE NOT EXISTS(SELECT 1 FROM provider_evidence pe WHERE pe.item_id=tvdb_episode_credit_evidence.item_id AND pe.provider='tvdb' AND pe.provider_id=tvdb_episode_credit_evidence.provider_id)`)
		return err
	})
}

// writeTVDBEpisodeCredits replaces one episode item's tvdb credit family with
// the extended record's characters. Other providers' rows (and NFO rows) are
// untouched: a selected provider owns only its own family. Person identity is
// the TVDB person id, the same key movies and show cast use, so a guest star
// is one /v1/people person everywhere.
func writeTVDBEpisodeCredits(ctx context.Context, tx *sql.Tx, item int64, chars []metadataprovider.TVDBEpisodeCharacter) error {
	credits := make([]compactcatalog.Credit, 0, len(chars))
	for ordinal, c := range chars {
		department := "Guest"
		switch {
		case strings.EqualFold(c.Type, "Actor") || strings.EqualFold(c.Type, "Acting") || strings.EqualFold(c.Type, "Cast"):
			department = "Acting"
		case strings.EqualFold(c.Type, "Director") || strings.EqualFold(c.Type, "Directing"):
			department = "Directing"
		case strings.EqualFold(c.Type, "Writer") || strings.EqualFold(c.Type, "Writing"):
			department = "Writing"
		case strings.EqualFold(c.Type, "Creator") || strings.EqualFold(c.Type, "Production") || strings.EqualFold(c.Type, "Producer"):
			department = c.Type
		}
		role := c.Character
		if role == "" {
			role = c.Type
		}
		person := strconv.FormatInt(c.PersonID, 10)
		credits = append(credits, compactcatalog.Credit{
			PersonKey:        "tvdb:" + person,
			PersonName:       c.PersonName,
			ProviderPersonID: person,
			CreditID:         person + ":" + strconv.Itoa(ordinal),
			CreditedName:     c.PersonName,
			Role:             role,
			Department:       department,
			Ordinal:          ordinal,
		})
	}
	return compactcatalog.SetCreditsTx(ctx, tx, item, "tvdb", credits)
}
