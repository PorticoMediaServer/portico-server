package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/metadataprovider"
)

type discoveredArtwork struct {
	role, subject, provider, imageID, origin, locale, attribution string
	rank                                                          float64
	votes                                                         int // -1: the provider publishes no count
	// season, when set, sends the image to the show's season of that number
	// in that TVDB episode order instead of to the show itself.
	season *seasonImageTarget
}

func (s *Service) seedArtwork(ctx context.Context) error {
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT kind,entity_id FROM artwork_dirty ORDER BY kind,entity_id LIMIT 32`)
	if err != nil {
		return err
	}
	targets := []RepairTarget{}
	dirtyEntities := []int64{}
	for rows.Next() {
		var t RepairTarget
		var entity int64
		if err = rows.Scan(&t.Kind, &entity); err != nil {
			rows.Close()
			return err
		}
		public, err := entityid.Public(ctx, tx, entity)
		if errors.Is(err, entityid.ErrNotFound) {
			// The entity is gone; drop its dirty row like the fence-miss below.
			if _, err = tx.ExecContext(ctx, `DELETE FROM artwork_dirty WHERE kind=? AND entity_id=?`, t.Kind, entity); err != nil {
				rows.Close()
				return err
			}
			continue
		}
		if err != nil {
			rows.Close()
			return err
		}
		t.ID = public
		targets = append(targets, t)
		dirtyEntities = append(dirtyEntities, entity)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	now := s.publicationTime().Format(time.RFC3339Nano)
	for n, t := range targets {
		entity := dirtyEntities[n]
		fence, e := artworkFence(ctx, tx, t)
		if errors.Is(e, sql.ErrNoRows) {
			_, err = tx.ExecContext(ctx, `DELETE FROM artwork_dirty WHERE kind=? AND entity_id=?`, t.Kind, entity)
			if err != nil {
				return err
			}
			continue
		}
		if e != nil {
			return e
		}
		screen, hasScreen, e := acceptedScreenArtwork(ctx, tx, t)
		if e != nil {
			return e
		}
		if screen != nil {
			if e = seedScreenArtwork(ctx, tx, t, *screen, fence, now); e != nil {
				return e
			}
		}
		if t.Kind == "item" {
			var poster, backdrop string
			if err = tx.QueryRowContext(ctx, `SELECT COALESCE(poster_url,''),COALESCE(backdrop_url,'') FROM catalog_item_details WHERE entity_id=?`, entity).Scan(&poster, &backdrop); errors.Is(err, sql.ErrNoRows) {
				poster, backdrop = "", ""
			} else if err != nil {
				return err
			}
			for role, origin := range map[string]string{"poster": poster, "backdrop": backdrop} {
				provider := artworkProvider(origin)
				if provider == "" || (hasScreen && provider != "local") {
					continue
				}
				attribution := artworkAttribution(provider)
				id, e := insertArtworkCandidate(ctx, tx, t, role, "", provider, publicationDigest(origin), origin, "", attribution, fence, now, 0)
				if e != nil {
					return e
				}
				var selected bool
				if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_selections a JOIN artwork_candidates c ON c.id=a.candidate_id WHERE a.kind=? AND a.entity_id=? AND a.role=? AND (a.locked=1 OR c.provider='local' OR c.source_fence=?)) OR EXISTS(SELECT 1 FROM artwork_jobs WHERE kind=? AND entity_id=? AND role=? AND (actor<>'' OR provider='local') AND status IN('pending','retry','running'))`, t.Kind, entity, role, fence, t.Kind, entity, role).Scan(&selected); e != nil {
					return e
				}
				if !selected {
					if e = queueArtworkCandidate(ctx, tx, t, id, "", now, false, false); e != nil {
						return e
					}
				}
			}
			// The episode's own still, from the TVDB page it was matched on.
			if e = seedEpisodeStill(ctx, tx, t, fence, now); e != nil {
				return e
			}
		}
		// Sidecar images the scan recorded for this entity, as local
		// candidates (Spec — Page Content §0.5).
		if e = seedLocalArtwork(ctx, tx, t, fence, now); e != nil {
			return e
		}
		if t.Kind == "book" || t.Kind == "album" {
			table, column := "catalog_book_files", "book_id"
			if t.Kind == "album" {
				table, column = "catalog_songs", "album_id"
			}
			var origin string
			err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT d.poster_url FROM `+table+` f JOIN catalog_item_details d ON d.entity_id=f.entity_id WHERE f.`+column+`=? AND d.poster_url LIKE 'local:%' ORDER BY f.entity_id LIMIT 1),'')`, entity).Scan(&origin)
			if err != nil {
				return err
			}
			if origin != "" {
				id, e := insertArtworkCandidate(ctx, tx, t, "cover", "", "local", publicationDigest(origin), origin, "", "Local artwork", fence, now, 1)
				if e != nil {
					return e
				}
				var occupied bool
				if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_selections a JOIN artwork_candidates c ON c.id=a.candidate_id WHERE a.kind=? AND a.entity_id=? AND a.role='cover' AND (a.locked=1 OR c.provider='local' OR c.source_fence=?)) OR EXISTS(SELECT 1 FROM artwork_jobs WHERE kind=? AND entity_id=? AND role='cover' AND (actor<>'' OR provider='local') AND status IN('pending','retry','running'))`, t.Kind, entity, fence, t.Kind, entity).Scan(&occupied); e != nil {
					return e
				}
				if !occupied {
					if e = queueArtworkCandidate(ctx, tx, t, id, "", now, false, false); e != nil {
						return e
					}
				}
			}
		}
		provider, id := "", ""
		if t.Kind == "album" {
			provider = "coverartarchive"
			err = tx.QueryRowContext(ctx, `SELECT l.release_id FROM mb_album_links l JOIN mb_jobs j ON j.kind='album' AND j.entity_id=l.album_id AND j.selected_id=l.requested_id WHERE l.album_id=? AND j.selected_id<>''`, entity).Scan(&id)
		} else if hasScreen {
			err = sql.ErrNoRows
			if screen != nil && (screen.Identity.Provider == "tvdb" && t.Kind == "show" || screen.Identity.Provider == "tmdb" && screen.Identity.Type == "movie") {
				provider, id, err = screen.Identity.Provider, screen.Identity.ID, nil
			}
		} else if t.Kind == "show" {
			provider = "tvdb"
			err = tx.QueryRowContext(ctx, `SELECT CAST(provider_id AS TEXT) FROM tvdb_jobs WHERE show_id=? AND provider_id>0`, entity).Scan(&id)
		} else if t.Kind == "item" {
			provider = "tmdb"
			err = tx.QueryRowContext(ctx, `SELECT CAST(p.provider_id AS TEXT) FROM provider_evidence p JOIN catalog_entities e ON e.id=p.item_id WHERE p.item_id=? AND p.provider='tmdb' AND e.kind=1`, entity).Scan(&id)
		} else {
			err = sql.ErrNoRows
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if id != "" {
			_, err = tx.ExecContext(ctx, `INSERT INTO artwork_discovery(kind,entity_id,provider,identity,source_fence) VALUES(?,?,?,?,?) ON CONFLICT(kind,entity_id,provider) DO UPDATE SET identity=excluded.identity,source_fence=excluded.source_fence,status='pending',attempts=0,next_attempt='',lease='',lease_until='',error='' WHERE artwork_discovery.identity<>excluded.identity OR artwork_discovery.source_fence<>excluded.source_fence`, t.Kind, entity, provider, id, fence)
			if err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM artwork_dirty WHERE kind=? AND entity_id=?`, t.Kind, entity); err != nil {
			return err
		}
	}
	return gated.Commit()
}
func artworkAttribution(provider string) string {
	switch provider {
	case "tmdb":
		return Attribution
	case "tvdb":
		return "Metadata provided by TheTVDB. Artwork also supplied by TheTVDB. https://thetvdb.com"
	case "coverartarchive":
		return "Cover art provided by Cover Art Archive / MusicBrainz and Internet Archive. Images copyright their respective owners. https://coverartarchive.org"
	case "commons":
		return "Artist image from Wikimedia Commons. See the file page for the licence. https://commons.wikimedia.org"
	case "local":
		return "Local artwork"
	}
	return provider
}
func (s *Service) artworkProviderEnabled(ctx context.Context, tx *sql.Tx, t RepairTarget, provider string) (bool, error) {
	ent, err := readRepairEntity(ctx, tx, t)
	if err != nil {
		return false, err
	}
	var enabled bool
	if provider != "local" {
		if local, err := LibraryLocalOnlyTx(ctx, tx, ent.library); err != nil || local {
			return false, err
		}
	}
	switch provider {
	case "tmdb":
		err = tx.QueryRowContext(ctx, `SELECT enabled FROM metadata_provider_policies WHERE library_id=? AND provider='tmdb'`, ent.library).Scan(&enabled)
	case "tvdb":
		err = tx.QueryRowContext(ctx, `SELECT enabled FROM tvdb_provider_policies WHERE library_id=?`, ent.library).Scan(&enabled)
	case "coverartarchive":
		err = tx.QueryRowContext(ctx, `SELECT enabled FROM mb_provider_policies WHERE library_id=?`, ent.library).Scan(&enabled)
	case "commons":
		err = tx.QueryRowContext(ctx, `SELECT enabled FROM mb_provider_policies WHERE library_id=?`, ent.library).Scan(&enabled)
	case "local":
		return true, nil
	default:
		return false, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil || !enabled {
		return enabled, err
	}
	if provider == "tmdb" || provider == "tvdb" {
		// Screen consent is independent from a legacy provider's enabled bit.
		var configured bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM screen_metadata_policies WHERE library_id=?)`, ent.library).Scan(&configured); err != nil {
			return false, err
		}
		if configured {
			err = tx.QueryRowContext(ctx, `SELECT p.enabled AND c.confirmed AND EXISTS(SELECT 1 FROM json_each(p.providers) WHERE value=?) FROM screen_metadata_policies p CROSS JOIN screen_metadata_consent c WHERE p.library_id=? AND c.singleton=1`, provider, ent.library).Scan(&enabled)
		}
	}
	return enabled, err
}
func (s *Service) ArtworkDiscoveryStep(ctx context.Context) error {
	if s.cacheRoot == "" {
		return nil
	}
	if err := s.seedArtwork(ctx); err != nil {
		return err
	}
	gated2, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	var t RepairTarget
	var entity int64
	var provider, id, fence string
	var attempts int
	stamp := s.publicationTime().Format(time.RFC3339Nano)
	err = tx.QueryRowContext(ctx, `SELECT kind,entity_id,provider,identity,source_fence,attempts FROM artwork_discovery WHERE ((status IN('pending','retry') AND next_attempt<=?) OR(status='running' AND lease_until<=?)) ORDER BY next_attempt,kind,entity_id LIMIT 1`, stamp, stamp).Scan(&t.Kind, &entity, &provider, &id, &fence, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if t.ID, err = entityid.Public(ctx, tx, entity); errors.Is(err, entityid.ErrNotFound) {
		// The entity is gone; drop its discovery row instead of wedging the step.
		if _, err = tx.ExecContext(ctx, `DELETE FROM artwork_discovery WHERE kind=? AND entity_id=? AND provider=?`, t.Kind, entity, provider); err != nil {
			return err
		}
		return gated2.Commit()
	} else if err != nil {
		return err
	}
	enabled, err := s.artworkProviderEnabled(ctx, tx, t, provider)
	if err != nil {
		return err
	}
	if !enabled {
		_, err = tx.ExecContext(ctx, `UPDATE artwork_discovery SET status='disabled',error='provider_disabled' WHERE kind=? AND entity_id=? AND provider=?`, t.Kind, entity, provider)
		if err != nil {
			return err
		}
		return gated2.Commit()
	}
	currentFence, err := artworkFence(ctx, tx, t)
	if err != nil {
		return err
	}
	if currentFence != fence {
		if _, err = tx.ExecContext(ctx, `UPDATE artwork_discovery SET status='stale',error='source_match_or_policy_changed',lease='',lease_until='' WHERE kind=? AND entity_id=? AND provider=?`, t.Kind, entity, provider); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO artwork_dirty VALUES(?,?)`, t.Kind, entity); err != nil {
			return err
		}
		return gated2.Commit()
	}

	lease := identity.Token()
	_, err = tx.ExecContext(ctx, `UPDATE artwork_discovery SET status='running',attempts=attempts+1,lease=?,lease_until=? WHERE kind=? AND entity_id=? AND provider=?`, lease, s.publicationTime().Add(2*time.Minute).Format(time.RFC3339Nano), t.Kind, entity, provider)
	if err != nil {
		return err
	}
	if err = gated2.Commit(); err != nil {
		return err
	}
	work, cancel := s.artworkRequestContext(ctx, t, provider, fence)
	defer cancel()
	images, problem := s.discoverArtwork(work, provider, id)
	var gated3 *dbwork.Write
	gated3, err = dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx = gated3.Tx()
	defer gated3.Rollback()
	var active bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_discovery WHERE kind=? AND entity_id=? AND provider=? AND lease=? AND lease_until>? AND status='running')`, t.Kind, entity, provider, lease, s.publicationTime().Format(time.RFC3339Nano)).Scan(&active)
	if err != nil {
		return err
	}
	if !active {
		return nil
	}
	current, err := artworkFence(ctx, tx, t)
	if err != nil {
		return err
	}
	enabled, err = s.artworkProviderEnabled(ctx, tx, t, provider)
	if err != nil {
		return err
	}
	status, code := "complete", ""
	if current != fence || !enabled {
		status = "stale"
		code = "source_match_or_policy_changed"
	} else if problem != nil {
		status = "retry"
		code = "provider_unavailable"
		if attempts >= 5 {
			status = "failed"
		}
		if problem.Error() == "not_found" {
			status = "not_found"
			code = "not_found"
		}
	}
	if status == "complete" {
		// Retain current and owner-history candidates. Retire stale optional choices
		// before adding a bounded provider response; no unbounded search accumulation.
		_, err = tx.ExecContext(ctx, `DELETE FROM artwork_candidates WHERE kind=? AND entity_id=? AND provider=? AND NOT EXISTS(SELECT 1 FROM artwork_selections a WHERE a.candidate_id=artwork_candidates.id) AND NOT EXISTS(SELECT 1 FROM metadata_owner_history h WHERE instr(h.before_json,artwork_candidates.id)>0 OR instr(h.after_json,artwork_candidates.id)>0) AND NOT EXISTS(SELECT 1 FROM artwork_jobs j WHERE j.candidate_id=artwork_candidates.id AND j.status IN('pending','running','retry'))`, t.Kind, entity, provider)
		if err != nil {
			return err
		}
		queued := map[string]bool{}
		for _, im := range images {
			if im.season != nil {
				if e := seedSeasonArtwork(ctx, tx, t, im, stamp); e != nil {
					return e
				}
				continue
			}
			candidate, e := insertArtworkCandidateVotes(ctx, tx, t, im.role, im.subject, provider, im.imageID, im.origin, im.locale, im.attribution, fence, stamp, im.rank, im.votes)
			if e != nil {
				return e
			}
			key := im.role + ":" + im.subject
			if queued[key] {
				continue
			}
			var exists bool
			if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_selections a JOIN artwork_candidates c ON c.id=a.candidate_id WHERE a.kind=? AND a.entity_id=? AND a.role=? AND a.subject=? AND (a.locked=1 OR c.provider='local' OR c.source_fence=?))`, t.Kind, entity, im.role, im.subject, fence).Scan(&exists); e != nil {
				return e
			}
			// Fill missing roles or replace unlocked art from a superseded identity.
			// Ranking changes alone never displace a current selection.
			if !exists {
				var ownerPending bool
				if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_jobs WHERE kind=? AND entity_id=? AND role=? AND subject=? AND (actor<>'' OR provider='local') AND status IN('pending','running','retry'))`, t.Kind, entity, im.role, im.subject).Scan(&ownerPending); e != nil {
					return e
				}
				if ownerPending {
					continue
				}
				if e = queueArtworkCandidate(ctx, tx, t, candidate, "", stamp, false, false); e != nil {
					return e
				}
				queued[key] = true
			}
		}
	}
	next := s.publicationTime().Add(time.Duration(30*(1<<min(attempts, 8))) * time.Second)
	var retry *artworkRetry
	if errors.As(problem, &retry) {
		code = retry.code
		if retry.after.After(next) {
			next = retry.after
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE artwork_discovery SET status=?,error=?,next_attempt=?,lease='',lease_until='' WHERE kind=? AND entity_id=? AND provider=? AND lease=?`, status, code, next.Format(time.RFC3339Nano), t.Kind, entity, provider, lease)
	if err != nil {
		return err
	}
	return gated3.Commit()
}
func (s *Service) discoverArtwork(ctx context.Context, provider, id string) ([]discoveredArtwork, error) {
	if provider == "coverartarchive" {
		return s.discoverCAA(ctx, id)
	}
	if provider == "tvdb" {
		return s.discoverTVDBArtwork(ctx, id)
	}
	if provider != "tmdb" {
		return nil, ErrRepairInput
	}
	n, err := strconv.Atoi(id)
	if err != nil || n <= 0 {
		return nil, ErrRepairInput
	}
	var data struct {
		ID     int `json:"id"`
		Images struct {
			Posters, Backdrops, Logos []struct {
				FilePath string  `json:"file_path"`
				Language string  `json:"iso_639_1"`
				Vote     float64 `json:"vote_average"`
				Votes    int     `json:"vote_count"`
			}
		} `json:"images"`
		Credits struct {
			Cast []struct {
				ID      int    `json:"id"`
				Profile string `json:"profile_path"`
			}
			Crew []struct {
				ID      int    `json:"id"`
				Profile string `json:"profile_path"`
			}
		} `json:"credits"`
	}
	raw, err := s.tmdbGet(ctx, "/movie/"+id+"?append_to_response=images,credits")
	if err != nil {
		var refused *metadataprovider.Error
		if errors.As(err, &refused) && refused.Status == 429 {
			return nil, &artworkRetry{code: "rate_limited", after: s.publicationTime().Add(refused.RetryAfter)}
		}
		if ctx.Err() != nil {
			return nil, errors.New("offline")
		}
		return nil, errors.New("provider_unavailable")
	}
	if json.Unmarshal(raw, &data) != nil || data.ID != n {
		return nil, ErrRepairInput
	}
	out := []discoveredArtwork{}
	add := func(role, subject, path, locale string, rank float64, votes int) {
		origin := imageURL(path, "original")
		if origin != "" && len(out) < 160 {
			out = append(out, discoveredArtwork{role: role, subject: subject, provider: provider, imageID: path, origin: origin, locale: locale, attribution: Attribution, rank: rank, votes: votes})
		}
	}
	for _, im := range data.Images.Posters {
		if len(out) >= 30 {
			break
		}
		add("poster", "", im.FilePath, im.Language, im.Vote, max(im.Votes, 0))
	}
	for i, im := range data.Images.Backdrops {
		if i >= 20 {
			break
		}
		add("backdrop", "", im.FilePath, im.Language, im.Vote, max(im.Votes, 0))
	}
	for i, im := range data.Images.Logos {
		if i >= 10 {
			break
		}
		add("logo", "", im.FilePath, im.Language, im.Vote, max(im.Votes, 0))
	}
	seen := map[int]bool{}
	for i, p := range data.Credits.Cast {
		if i >= 80 {
			break
		}
		if p.ID > 0 && !seen[p.ID] {
			add("portrait", "tmdb:"+strconv.Itoa(p.ID), p.Profile, "", 0, -1)
			seen[p.ID] = true
		}
	}
	for i, p := range data.Credits.Crew {
		if i >= 20 {
			break
		}
		if p.ID > 0 && !seen[p.ID] {
			add("portrait", "tmdb:"+strconv.Itoa(p.ID), p.Profile, "", 0, -1)
			seen[p.ID] = true
		}
	}
	return out, nil
}
func (s *Service) discoverCAA(ctx context.Context, id string) ([]discoveredArtwork, error) {
	if !mbIdentifier.MatchString(id) {
		return nil, ErrRepairInput
	}
	raw, err := s.fetchArtworkRemote(ctx, "https://coverartarchive.org/release/"+strings.ToLower(id)+"/", "coverartarchive", 1<<20, "application/json")
	if err != nil {
		return nil, err
	}
	var data struct {
		Images []struct {
			ID       json.Number `json:"id"`
			Front    bool        `json:"front"`
			Approved bool        `json:"approved"`
			Types    []string    `json:"types"`
		}
	}
	if json.Unmarshal(raw, &data) != nil || len(data.Images) > 200 {
		return nil, ErrRepairInput
	}
	out := []discoveredArtwork{}
	for _, im := range data.Images {
		if !im.Approved {
			continue
		}
		n, err := strconv.ParseInt(string(im.ID), 10, 64)
		if err != nil || n <= 0 {
			continue
		}
		rank := 0.
		if im.Front {
			rank = 100
		}
		out = append(out, discoveredArtwork{role: "cover", provider: "coverartarchive", imageID: string(im.ID), origin: fmt.Sprintf("https://coverartarchive.org/release/%s/%d-1200", strings.ToLower(id), n), attribution: artworkAttribution("coverartarchive"), rank: rank, votes: -1})
		if len(out) >= 40 {
			break
		}
	}
	// API order is not a promise of front-first. Rank locally only within art roles,
	// never as identity evidence. Cover does not masquerade as a square crop.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].rank > out[i].rank {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}
