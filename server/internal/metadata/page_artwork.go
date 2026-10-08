package metadata

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"portico.local/server/internal/metadataprovider"
)

// Page artwork (Spec — Page Content §2): each episode's own still, each
// season's poster and the cast's photos. All of it comes through the library's
// agent (never for a local-only library), is bounded per show, and is fetched
// and cached by the ordinary artwork worker like every other image.

// seasonImageTarget sends a discovered image to the show's season of that
// number, when the show uses that TVDB episode order.
type seasonImageTarget struct {
	number int
	order  string
}

// seedArtworkCandidate records one image as a candidate for a slot and queues
// it, unless the slot is already held: an owner's lock, local art, a choice
// made under the current fence, or a pending owner job.
func seedArtworkCandidate(ctx context.Context, tx *sql.Tx, t RepairTarget, role, subject, provider, imageID, origin, attribution, fence, now string, rank float64) error {
	entity, err := resolveEntity(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	id, err := insertArtworkCandidate(ctx, tx, t, role, subject, provider, imageID, origin, "", attribution, fence, now, rank)
	if err != nil {
		return err
	}
	var occupied bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_selections a JOIN artwork_candidates c ON c.id=a.candidate_id WHERE a.kind=? AND a.entity_id=? AND a.role=? AND a.subject=? AND (a.locked=1 OR c.provider='local' OR c.source_fence=?)) OR EXISTS(SELECT 1 FROM artwork_jobs WHERE kind=? AND entity_id=? AND role=? AND subject=? AND (actor<>'' OR provider='local') AND status IN('pending','running','retry'))`, t.Kind, entity, role, subject, fence, t.Kind, entity, role, subject).Scan(&occupied); err != nil {
		return err
	}
	if occupied {
		return nil
	}
	return queueArtworkCandidate(ctx, tx, t, id, "", now, false, false)
}

// seedEpisodeStill turns the TVDB episode image already stored with the
// episode's accepted evidence into a still candidate. No request is made here:
// the image came with the episode page the TVDB agent fetched.
func seedEpisodeStill(ctx context.Context, tx *sql.Tx, t RepairTarget, fence, now string) error {
	if t.Kind != "item" {
		return nil
	}
	entity, err := resolveEntity(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	var library, image string
	err = tx.QueryRowContext(ctx, `SELECT cl.library_id,COALESCE(json_extract(pe.payload,'$.image'),'') FROM provider_evidence pe JOIN catalog_entities i ON i.id=pe.item_id JOIN catalog_libraries cl ON cl.id=i.library_id WHERE pe.item_id=? AND pe.provider='tvdb' AND i.kind=4`, entity).Scan(&library, &image)
	if errors.Is(err, sql.ErrNoRows) || err == nil && image == "" {
		return nil
	}
	if err != nil {
		return err
	}
	if local, err := LibraryLocalOnlyTx(ctx, tx, library); err != nil || local {
		return err
	}
	if strings.HasPrefix(image, "/banners/") {
		image = "https://artworks.thetvdb.com" + image
	}
	if !artworkURLAllowed(image, "tvdb") {
		return nil
	}
	return seedArtworkCandidate(ctx, tx, t, "still", "", "tvdb", publicationDigest(image), image, artworkAttribution("tvdb"), fence, now, 0)
}

// seedSeasonArtwork records a season poster discovered with its show.
func seedSeasonArtwork(ctx context.Context, tx *sql.Tx, show RepairTarget, im discoveredArtwork, now string) error {
	showID, err := resolveEntity(ctx, tx, show.ID)
	if err != nil {
		return err
	}
	var order string
	err = tx.QueryRowContext(ctx, `SELECT episode_order FROM tvdb_jobs WHERE show_id=?`, showID).Scan(&order)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if order == "" || order == "default" {
		order = "official"
	}
	if im.season.order != order {
		return nil
	}
	var season int64
	err = tx.QueryRowContext(ctx, `SELECT entity_id FROM catalog_seasons WHERE show_id=? AND number=?`, showID, im.season.number).Scan(&season)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	t := RepairTarget{"season", entityPublic(ctx, tx, season)}
	fence, err := artworkFence(ctx, tx, t)
	if err != nil {
		return err
	}
	return seedArtworkCandidate(ctx, tx, t, im.role, "", im.provider, im.imageID, im.origin, im.attribution, fence, now, im.rank)
}

// tvdbPageArtwork is the season posters and cast photos of a TVDB series, read
// from the extended record the TVDB agent already fetches.
func (s *Service) tvdbPageArtwork(ctx context.Context, id int64) []discoveredArtwork {
	p, ok := s.tvdb.(interface {
		SeriesPageArtwork(context.Context, int64) (metadataprovider.SeriesPageArtwork, error)
	})
	if !ok {
		return nil
	}
	page, err := p.SeriesPageArtwork(ctx, id)
	if err != nil {
		// Series artwork already found stays valid; the page extras are retried
		// with the next discovery.
		return nil
	}
	out := []discoveredArtwork{}
	attribution := artworkAttribution("tvdb")
	for _, person := range page.People {
		if artworkURLAllowed(person.URL, "tvdb") {
			out = append(out, discoveredArtwork{role: "portrait", subject: "tvdb:" + strconv.FormatInt(person.PersonID, 10), provider: "tvdb", imageID: publicationDigest(person.URL), origin: person.URL, attribution: attribution, votes: -1})
		}
	}
	for _, season := range page.Seasons {
		if artworkURLAllowed(season.URL, "tvdb") {
			out = append(out, discoveredArtwork{role: "poster", provider: "tvdb", imageID: publicationDigest(season.URL), origin: season.URL, attribution: attribution, votes: -1, season: &seasonImageTarget{season.Number, season.Order}})
		}
	}
	return out
}

// seedLocalArtwork turns an entity's scanned sidecars (local_artwork, 0263)
// into provider='local' candidates: show posters, backdrops and logos,
// season posters, artist portraits and backdrops, movie logos. Only rows whose
// entity still exists seed anything, so deleted entities leave no candidates.
func seedLocalArtwork(ctx context.Context, tx *sql.Tx, t RepairTarget, fence, now string) error {
	switch t.Kind {
	case "show", "season", "artist", "item":
	default:
		return nil
	}
	entity, err := resolveEntity(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT role,art_key FROM local_artwork WHERE kind=? AND entity_id=? AND (
	 (?='show' AND EXISTS(SELECT 1 FROM catalog_entities e WHERE e.id=local_artwork.entity_id AND e.kind=2)) OR
	 (?='season' AND EXISTS(SELECT 1 FROM catalog_entities e WHERE e.id=local_artwork.entity_id AND e.kind=3)) OR
	 (?='artist' AND EXISTS(SELECT 1 FROM catalog_entities e WHERE e.id=local_artwork.entity_id AND e.kind=5)) OR
	 (?='item' AND EXISTS(SELECT 1 FROM catalog_entities e WHERE e.id=local_artwork.entity_id)))`, t.Kind, entity, t.Kind, t.Kind, t.Kind, t.Kind)
	if err != nil {
		return err
	}
	type local struct{ role, key string }
	locals := []local{}
	for rows.Next() {
		var v local
		if err = rows.Scan(&v.role, &v.key); err != nil {
			rows.Close()
			return err
		}
		locals = append(locals, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range locals {
		origin := "local:" + v.key
		if err = seedArtworkCandidate(ctx, tx, t, v.role, "", "local", publicationDigest(origin), origin, artworkAttribution("local"), fence, now, 0); err != nil {
			return err
		}
	}
	return nil
}
