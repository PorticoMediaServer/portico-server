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

// tvdbEpisodeCreditCleanup bounds the orphaned evidence one step removes, so
// the cleanup stays inside the writer's per-transaction effect budget.
const tvdbEpisodeCreditCleanup = 32

// tvdbEpisodeCreditsStep gives TVDB-matched episodes their cast. TVDB episodes
// are delegated past the screen pipeline (screenEpisodeStep answers
// delegated_tvdb), so nothing ever writes their screen credits; this step
// claims a bounded page of matched episodes with no credit evidence, fetches
// each one's /episodes/{id}/extended characters, and writes them as the
// episode item's tvdb credits. It runs when TVDBStep has no projection or
// acquisition work, so it never delays a show publication.
func (s *Service) tvdbEpisodeCreditsStep(ctx context.Context) error {
	ctx = s.lookupContext(ctx)
	provider, ok := s.tvdb.(tvdbEpisodeCharacterProvider)
	if !ok {
		return nil
	}
	type claim struct {
		item       int64
		providerID int64
	}
	claims := []claim{}
	stale := []claim{}
	// This is a read-pick, not a reservation. Admit the catalogue walk as
	// background reading and close its snapshot before fetching or publishing.
	err := dbwork.WithReadSnapshot(dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia), s.db, func(readCtx context.Context) error {
		rows, err := dbwork.Query(readCtx, s.db, `SELECT pe.item_id,pe.provider_id FROM provider_evidence pe JOIN catalog_episodes e ON e.entity_id=pe.item_id JOIN catalog_entities i ON i.id=pe.item_id JOIN catalog_libraries cl ON cl.id=i.library_id JOIN tvdb_provider_policies tp ON tp.library_id=cl.library_id AND tp.enabled=1 WHERE pe.provider='tvdb' AND pe.provider_id>0 AND i.kind=4 AND NOT EXISTS(SELECT 1 FROM tvdb_episode_credit_evidence ce WHERE ce.item_id=pe.item_id AND ce.provider_id=pe.provider_id AND ce.credits IS NOT NULL) AND COALESCE((SELECT ce.attempts FROM tvdb_episode_credit_evidence ce WHERE ce.item_id=pe.item_id AND ce.provider_id=pe.provider_id),0)<? ORDER BY pe.item_id LIMIT ?`, tvdbEpisodeCreditAttempts, tvdbEpisodeCreditPage)
		if err != nil {
			return err
		}
		for rows.Next() {
			var c claim
			if err := rows.Scan(&c.item, &c.providerID); err != nil {
				rows.Close()
				return err
			}
			claims = append(claims, c)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		// A rematch leaves evidence for a pair the catalog no longer holds.
		// Find at most one writer-sized batch here, outside the writer; each
		// key is rechecked before it is deleted.
		rows, err = dbwork.Query(readCtx, s.db, `SELECT ce.item_id,ce.provider_id FROM tvdb_episode_credit_evidence ce WHERE NOT EXISTS(SELECT 1 FROM provider_evidence pe WHERE pe.item_id=ce.item_id AND pe.provider='tvdb' AND pe.provider_id=ce.provider_id) ORDER BY ce.item_id,ce.provider_id LIMIT ?`, tvdbEpisodeCreditCleanup)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c claim
			if err := rows.Scan(&c.item, &c.providerID); err != nil {
				return err
			}
			stale = append(stale, c)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	if len(claims) == 0 && len(stale) == 0 {
		return nil
	}
	// Fetches run outside the read snapshot. Publication retains the original
	// context and all its lookup guards, never the closed read transaction.
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
		if err := checkLookupContextTx(ctx, tx); err != nil {
			return err
		}
		stamp := tvdbStamp(s.publicationTime())
		for i, c := range claims {
			// The pair was picked before the fetch. A rematch, a kind or
			// library move, a disabled policy or a deleted episode in the
			// meantime means these credits no longer belong to this item: drop
			// the result and leave no evidence or attempt for the old pair.
			var current bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM provider_evidence pe JOIN catalog_episodes e ON e.entity_id=pe.item_id JOIN catalog_entities i ON i.id=pe.item_id JOIN catalog_libraries cl ON cl.id=i.library_id JOIN tvdb_provider_policies tp ON tp.library_id=cl.library_id AND tp.enabled=1 WHERE pe.item_id=? AND pe.provider='tvdb' AND pe.provider_id=? AND i.kind=4)`, c.item, c.providerID).Scan(&current); err != nil {
				return err
			}
			if !current {
				continue
			}
			if results[i].err != nil {
				if _, err := tx.ExecContext(ctx, `INSERT INTO tvdb_episode_credit_evidence(item_id,provider_id,observed_at,attempts) VALUES(?,?,?,1) ON CONFLICT(item_id,provider_id) DO UPDATE SET observed_at=excluded.observed_at,attempts=tvdb_episode_credit_evidence.attempts+1`, c.item, c.providerID, stamp); err != nil {
					return err
				}
				continue
			}
			if err := writeTVDBEpisodeCredits(ctx, tx, c.item, results[i].credits, "tvdb:episode:"+strconv.FormatInt(c.providerID, 10), stamp); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO tvdb_episode_credit_evidence(item_id,provider_id,observed_at,credits,attempts) VALUES(?,?,?,COALESCE((SELECT count(*) FROM catalog_credits WHERE entity_id=? AND provider='tvdb'),0),0) ON CONFLICT(item_id,provider_id) DO UPDATE SET observed_at=excluded.observed_at,credits=excluded.credits,attempts=0`, c.item, c.providerID, stamp, c.item); err != nil {
				return err
			}
		}
		for _, c := range stale {
			if _, err := tx.ExecContext(ctx, `DELETE FROM tvdb_episode_credit_evidence WHERE item_id=? AND provider_id=? AND NOT EXISTS(SELECT 1 FROM provider_evidence pe WHERE pe.item_id=? AND pe.provider='tvdb' AND pe.provider_id=?)`, c.item, c.providerID, c.item, c.providerID); err != nil {
				return err
			}
		}
		return nil
	})
}

// writeTVDBEpisodeCredits replaces one episode item's tvdb credit family with
// the extended record's characters. Other providers' rows (and NFO rows) are
// untouched: a selected provider owns only its own family. Person identity is
// the TVDB person id, the same key movies and show cast use, so a guest star
// is one /v1/people person everywhere.
func writeTVDBEpisodeCredits(ctx context.Context, tx *sql.Tx, item int64, chars []metadataprovider.TVDBEpisodeCharacter, accepted ...string) error {
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
	if len(accepted) == 2 {
		return compactcatalog.SetCreditsFromRecordTx(ctx, tx, item, "tvdb", accepted[0], accepted[1], credits)
	}
	if len(accepted) != 0 {
		return ErrRepairInput
	}
	return compactcatalog.SetCreditsTx(ctx, tx, item, "tvdb", credits)
}
