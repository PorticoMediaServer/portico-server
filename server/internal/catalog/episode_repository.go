package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"strconv"
	"strings"
	"time"
)

type Show struct {
	WatchedCount     *int     `json:"watchedCount,omitempty"`
	UnwatchedCount   *int     `json:"unwatchedCount,omitempty"`
	PosterURL        string   `json:"posterUrl,omitempty"`
	BackdropURL      string   `json:"backdropUrl,omitempty"`
	LogoURL          string   `json:"logoUrl,omitempty"`
	OriginalTitle    string   `json:"originalTitle,omitempty"`
	Tagline          string   `json:"tagline,omitempty"`
	ContentRating    string   `json:"contentRating,omitempty"`
	Studio           string   `json:"studio,omitempty"`
	Network          string   `json:"network,omitempty"`
	Country          string   `json:"country,omitempty"`
	OriginalLanguage string   `json:"originalLanguage,omitempty"`
	Genres           []string `json:"genres,omitempty"`
	Overview         string   `json:"overview,omitempty"`
	// Status is the provider's series status ("Continuing", "Ended", …).
	Status string `json:"status,omitempty"`
	// FirstAired is the series premiere (provider date, YYYY-MM-DD).
	FirstAired string `json:"firstAired,omitempty"`
	// LastAired is the latest air date among this library's episodes.
	LastAired string `json:"lastAired,omitempty"`
	// Rating is the provider's community rating.
	Rating              *ShowRating `json:"rating,omitempty"`
	ProviderSourceURL   string      `json:"providerSourceUrl,omitempty"`
	ProviderObservedAt  string      `json:"providerObservedAt,omitempty"`
	ID                  string      `json:"id"`
	LibraryID           string      `json:"libraryId"`
	Title               string      `json:"title"`
	Year                int         `json:"year,omitempty"`
	LibraryKind         string      `json:"libraryKind"`
	ProviderMatchStatus string      `json:"providerMatchStatus"`
}

// ShowRating is a community rating on its own scale (TMDB and TVDB: 10).
type ShowRating struct {
	Value float64 `json:"value"`
	Scale int     `json:"scale"`
	Votes int     `json:"votes,omitempty"`
}
type Season struct {
	UnwatchedCount *int   `json:"unwatchedCount,omitempty"`
	ID             string `json:"id"`
	ShowID         string `json:"showId"`
	Number         int    `json:"number"`
	Title          string `json:"title"`
	EpisodeCount   *int   `json:"episodeCount,omitempty"`
	WatchedCount   *int   `json:"watchedCount,omitempty"`
	// PosterURL is the season's own poster (TVDB, local season01.jpg or an
	// owner upload), never the show's.
	PosterURL string `json:"posterUrl,omitempty"`
}
type EpisodeInfo struct {
	AirDate             string `json:"airDate,omitempty"`
	ShowID              string `json:"showId"`
	ShowTitle           string `json:"showTitle"`
	SeasonID            string `json:"seasonId,omitempty"`
	SeasonNumber        *int   `json:"seasonNumber,omitempty"`
	Numbering           string `json:"numbering"`
	Number              int    `json:"number"`
	LocalIdentityStatus string `json:"localIdentityStatus"`
	ProviderMatchStatus string `json:"providerMatchStatus"`
	OrderingBasis       string `json:"orderingBasis"`
	SourceBoundary      string `json:"sourceBoundary"`
}
type EpisodeIssue struct {
	AssetID    string `json:"assetId"`
	SourceName string `json:"sourceName"`
	Code       string `json:"code"`
}
type EpisodeAssignment struct {
	AirDate        string `json:"airDate,omitempty"`
	AssetID        string `json:"assetId"`
	ShowTitle      string `json:"showTitle"`
	Year           int    `json:"year"`
	Numbering      string `json:"numbering"`
	SeasonNumber   *int   `json:"seasonNumber"`
	EpisodeNumbers []int  `json:"episodeNumbers"`
}

func (s *Service) commitEpisodes(tx *sql.Tx, library, asset, name string, plan EpisodeNaming, manual bool) error {
	var locked int
	err := tx.QueryRow(`SELECT manual FROM episodic_sources WHERE library_id=? AND asset_id=?`, library, asset).Scan(&locked)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if locked != 0 && !manual {
		return nil
	}
	_, err = tx.Exec(`INSERT INTO episodic_sources(library_id,asset_id,manual,issue,source_name) VALUES(?,?,?,?,?) ON CONFLICT(library_id,asset_id) DO UPDATE SET manual=excluded.manual,issue=excluded.issue,source_name=excluded.source_name`, library, asset, manual, plan.Issue, name)
	if err != nil {
		return err
	}
	if plan.Issue != "" {
		return nil
	}
	ctx := context.Background()
	handle, err := compactcatalog.LibraryTx(ctx, tx, library)
	if err != nil {
		return err
	}
	assetID, err := compactcatalog.AssetByTokenTx(ctx, tx, asset)
	if err != nil {
		return err
	}
	if assetID == 0 {
		return sql.ErrNoRows
	}
	key := strings.ToLower(strings.Join(strings.Fields(plan.ShowTitle), " ")) + ":" + strconv.Itoa(plan.Year)
	show, created, err := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: compactcatalog.Show, Key: compactcatalog.ShowKey(key), Title: plan.ShowTitle, Year: plan.Year})
	if err != nil {
		return err
	}
	if created {
		if err = compactcatalog.SetFactsTx(ctx, tx, show, map[string]any{"local_key": key}); err != nil {
			return err
		}
	}
	var season int64
	parent := show
	if plan.Numbering == "seasonal" || plan.Numbering == "date" {
		title := fmt.Sprintf("Season %d", plan.Season)
		if plan.Season == 0 {
			title = "Specials"
		}
		season, created, err = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: compactcatalog.Season, Parent: show, Key: compactcatalog.SeasonKey(key, plan.Season), Title: title})
		if err != nil {
			return err
		}
		if created {
			if err = compactcatalog.SetFactsTx(ctx, tx, season, map[string]any{"show_id": show, "number": plan.Season}); err != nil {
				return err
			}
		}
		parent = season
	}
	// Replace this physical source's interpretation only. Other sources linked to
	// the same logical episode remain attached; physical bytes are never deleted.
	linked, err := scanInt64s(tx.Query(`SELECT l.entity_id FROM catalog_asset_links l INDEXED BY catalog_asset_links_asset JOIN catalog_entities e ON e.id=l.entity_id WHERE l.asset_id=? AND e.library_id=? AND e.kind=?`, assetID, handle, compactcatalog.Episode))
	if err != nil {
		return err
	}
	for _, item := range linked {
		if err = compactcatalog.UnlinkAssetTx(ctx, tx, item, assetID); err != nil {
			return err
		}
	}
	status := "parsed"
	if manual {
		status = "manual"
	}
	for _, number := range plan.Numbers {
		var item int64
		err = tx.QueryRow(`SELECT entity_id FROM catalog_episodes INDEXED BY catalog_episodes_position WHERE show_id=? AND COALESCE(season_id,0)=? AND numbering=? AND number=?`, show, season, plan.Numbering, number).Scan(&item)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			title := fmt.Sprintf("Episode %d", number)
			if plan.Numbering == "absolute" {
				title = fmt.Sprintf("Absolute episode %d", number)
			} else if plan.Numbering == "date" {
				title = plan.AirDate
			}
			item, _, err = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: compactcatalog.Episode, Parent: parent, Key: compactcatalog.EpisodeKey(key, plan.Numbering, plan.Season, number), Title: title, Added: catalogedNow()})
			if err != nil {
				return err
			}
			var airDate any
			basis := "unspecified"
			if plan.Numbering == "date" {
				date, err := time.Parse("2006-01-02", plan.AirDate)
				if err != nil {
					return err
				}
				airDate, basis = date.Unix()/86400, "date"
			}
			var seasonRef any
			if season != 0 {
				seasonRef = season
			}
			if err = compactcatalog.SetFactsTx(ctx, tx, item, map[string]any{"show_id": show, "season_id": seasonRef, "numbering": plan.Numbering, "number": number, "local_identity_status": status, "air_date": airDate, "ordering_basis": basis}); err != nil {
				return err
			}
		case err != nil:
			return err
		case manual:
			if err = compactcatalog.SetFactsTx(ctx, tx, item, map[string]any{"local_identity_status": "manual"}); err != nil {
				return err
			}
		}
		if err = compactcatalog.LinkAssetTx(ctx, tx, item, assetID, compactcatalog.Link{}); err != nil {
			return err
		}
		boundary := "whole_source"
		if len(plan.Numbers) > 1 {
			boundary = "unknown_multi_episode"
		}
		if _, err = tx.Exec(`INSERT INTO episode_asset_boundaries(item_id,asset_id,status) VALUES(?,?,?) ON CONFLICT(item_id,asset_id) DO UPDATE SET status=excluded.status`, item, asset, boundary); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE playback_sessions SET state='stopped' WHERE asset_id=?1 AND NOT EXISTS(SELECT 1 FROM catalog_asset_links l WHERE l.asset_id=?2 AND l.entity_id=playback_sessions.item_id)`, asset, assetID)
	return err
}

// scanInt64s reads one column of integer ids.
func scanInt64s(rows *sql.Rows, err error) ([]int64, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Service) AssignEpisodes(ctx context.Context, library string, body EpisodeAssignment) error {
	if len(body.ShowTitle) == 0 || len(body.ShowTitle) > 200 || body.Year < 0 || body.Year > 2200 || len(body.EpisodeNumbers) == 0 || len(body.EpisodeNumbers) > 8 {
		return errors.New("invalid episode assignment")
	}
	plan := EpisodeNaming{ShowTitle: strings.TrimSpace(body.ShowTitle), Year: body.Year, Season: -1, Numbering: body.Numbering, Numbers: body.EpisodeNumbers}
	if plan.Numbering == "date" {
		date, err := time.Parse("2006-01-02", body.AirDate)
		if err != nil || date.Year() < 1900 || body.SeasonNumber != nil && *body.SeasonNumber != date.Year() {
			return errors.New("valid airDate and matching year season required")
		}
		plan.AirDate, plan.Season, plan.Numbers = body.AirDate, date.Year(), []int{date.YearDay()}
	}
	if plan.ShowTitle == "" || (plan.Numbering != "seasonal" && plan.Numbering != "absolute" && plan.Numbering != "date") {
		return errors.New("explicit seasonal, absolute, or date numbering is required")
	}
	if plan.Numbering == "seasonal" {
		if body.SeasonNumber == nil || *body.SeasonNumber < 0 || *body.SeasonNumber > 9999 {
			return errors.New("seasonNumber is required for seasonal numbering")
		}
		plan.Season = *body.SeasonNumber
	} else if plan.Numbering == "absolute" && body.SeasonNumber != nil {
		return errors.New("absolute episodes must remain unassigned to a season")
	}
	seen := map[int]bool{}
	for _, n := range plan.Numbers {
		if n < 1 || n > 9999 || seen[n] {
			return errors.New("episode numbers must be distinct positive numbers")
		}
		seen[n] = true
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	var name, kind string
	if err = tx.QueryRow(`SELECT e.source_name,l.kind FROM episodic_sources e JOIN libraries l ON l.id=e.library_id WHERE e.library_id=? AND e.asset_id=?`, library, body.AssetID).Scan(&name, &kind); err != nil {
		return err
	}
	if plan.Numbering == "absolute" && kind != "anime" {
		return errors.New("absolute numbering requires an anime library")
	}
	if err = s.commitEpisodes(tx, library, body.AssetID, name, plan, true); err != nil {
		return err
	}
	return gated.Commit()
}
func (s *Service) Shows(viewer Viewer, library, cursor string, limit int) ([]Show, string, error) {
	if !viewer.AllowsLibrary(library) {
		return nil, "", sql.ErrNoRows
	}
	if err := s.prepareViewer(viewer); err != nil {
		return nil, "", err
	}
	limit = pageLimit(limit)
	var after int64
	if cursor != "" {
		var err error
		if after, err = strconv.ParseInt(cursor, 10, 64); err != nil || after < 0 {
			return nil, "", errors.New("invalid show cursor")
		}
	}
	restriction, bound := ItemRestrictionSQL("item.id", viewer.EffectiveRestrictions())
	args := append([]any{compactcatalog.Show, library}, bound...)
	args = append(args, after, limit+1)
	rows, err := s.read().Query(`SELECT s.id,pid(s.public_id),l.library_id,s.title,s.year,CASE l.kind WHEN 2 THEN 'tv' WHEN 3 THEN 'anime' END,d.provider_match_status,
 COALESCE((SELECT json_extract(value,'$') FROM screen_metadata_fields WHERE target_kind='show' AND target_id=s.id AND field='overview'),c.overview,''),
 COALESCE((SELECT source_url FROM screen_metadata_fields WHERE target_kind='show' AND target_id=s.id AND field='overview'),CASE WHEN c.provider_id IS NULL THEN '' ELSE 'https://thetvdb.com/dereferrer/series/'||c.provider_id END),
 COALESCE((SELECT observed_at FROM screen_metadata_fields WHERE target_kind='show' AND target_id=s.id AND field='overview'),c.observed_at,'')
 FROM catalog_entities s JOIN catalog_shows d ON d.entity_id=s.id JOIN catalog_libraries l ON l.id=s.library_id
 LEFT JOIN tvdb_jobs j ON j.show_id=s.id
 LEFT JOIN tvdb_series_candidates c ON c.show_id=s.id AND c.provider_id=j.provider_id AND NOT EXISTS(SELECT 1 FROM screen_metadata_work sw WHERE sw.target_kind='show' AND sw.target_id=s.id AND sw.accepted_publication<>'')
 WHERE s.kind=? AND l.library_id=? AND l.retired=0 AND s.retired=0
 AND EXISTS(SELECT 1 FROM catalog_episodes ep JOIN catalog_entities item ON item.id=ep.entity_id JOIN catalog_asset_links a ON a.entity_id=item.id WHERE ep.show_id=s.id AND `+restriction+`)
 AND s.id>? ORDER BY s.id LIMIT ?`, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Show{}
	var last int64
	for rows.Next() {
		var show Show
		if err = rows.Scan(&last, &show.ID, &show.LibraryID, &show.Title, &show.Year, &show.LibraryKind, &show.ProviderMatchStatus, &show.Overview, &show.ProviderSourceURL, &show.ProviderObservedAt); err != nil {
			return nil, "", err
		}
		out = append(out, show)
		if len(out) == limit {
			break
		}
	}
	next := ""
	if rows.Next() {
		next = strconv.FormatInt(last, 10)
	}
	return out, next, rows.Err()
}
func (s *Service) LibraryForShow(show string) (string, error) {
	// A malformed id is nobody's show: not found, before it reaches SQL.
	if _, ok := entityid.Decode(show); !ok {
		return "", sql.ErrNoRows
	}
	var id string
	err := s.read().QueryRow(`SELECT l.library_id FROM catalog_entities s JOIN catalog_libraries l ON l.id=s.library_id WHERE s.public_id=pid_blob(?) AND s.kind=? AND s.retired=0`, show, compactcatalog.Show).Scan(&id)
	return id, err
}
func (s *Service) LibraryForSeason(season string) (string, error) {
	// A malformed id is nobody's season: not found, before it reaches SQL.
	if _, ok := entityid.Decode(season); !ok {
		return "", sql.ErrNoRows
	}
	var id string
	err := s.read().QueryRow(`SELECT l.library_id FROM catalog_entities se JOIN catalog_libraries l ON l.id=se.library_id WHERE se.public_id=pid_blob(?) AND se.kind=? AND se.retired=0`, season, compactcatalog.Season).Scan(&id)
	return id, err
}
func (s *Service) Seasons(viewer Viewer, show string) ([]Season, error) {
	if err := s.prepareViewer(viewer); err != nil {
		return nil, err
	}
	restriction, bound := ItemRestrictionSQL("item.id", viewer.EffectiveRestrictions())
	args := append([]any{show, viewer.librariesJSON()}, bound...)
	rows, err := s.read().Query(`SELECT pid(se.public_id),pid(sh.public_id),d.number FROM catalog_entities sh JOIN catalog_seasons d ON d.show_id=sh.id JOIN catalog_entities se ON se.id=d.entity_id JOIN catalog_libraries l ON l.id=sh.library_id
 WHERE sh.public_id=pid_blob(?) AND l.library_id IN(SELECT value FROM json_each(?)) AND se.retired=0
 AND EXISTS(SELECT 1 FROM catalog_episodes ep JOIN catalog_entities item ON item.id=ep.entity_id JOIN catalog_asset_links a ON a.entity_id=item.id WHERE ep.season_id=se.id AND `+restriction+`) ORDER BY d.number`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Season{}
	for rows.Next() {
		var row Season
		if err = rows.Scan(&row.ID, &row.ShowID, &row.Number); err != nil {
			return nil, err
		}
		row.Title = fmt.Sprintf("Season %d", row.Number)
		if row.Number == 0 {
			row.Title = "Specials"
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		var visible int
		if err := s.read().QueryRow(`SELECT 1 FROM catalog_entities sh JOIN catalog_episodes ep ON ep.show_id=sh.id JOIN catalog_entities item ON item.id=ep.entity_id JOIN catalog_libraries l ON l.id=sh.library_id JOIN catalog_asset_links a ON a.entity_id=item.id WHERE sh.public_id=pid_blob(?) AND l.library_id IN(SELECT value FROM json_each(?)) AND `+restriction+` LIMIT 1`, args...).Scan(&visible); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (s *Service) Episodes(viewer Viewer, show, season, cursor string, limit int) ([]Item, string, error) {
	if err := s.prepareViewer(viewer); err != nil {
		return nil, "", err
	}
	if season != "" {
		var parent string
		if err := s.read().QueryRow(`SELECT pid(sh.public_id) FROM catalog_entities se JOIN catalog_seasons d ON d.entity_id=se.id JOIN catalog_entities sh ON sh.id=d.show_id WHERE se.public_id=pid_blob(?) AND se.retired=0`, season).Scan(&parent); err != nil {
			return nil, "", err
		}
		if show != "" && show != parent {
			return nil, "", sql.ErrNoRows
		}
		show = parent
	}
	if show != "" {
		visible, args := viewer.itemVisibilitySQL("item.id")
		args = append([]any{show}, args...)
		var present int
		if err := s.read().QueryRow(`SELECT 1 FROM catalog_entities sh JOIN catalog_episodes ep ON ep.show_id=sh.id JOIN catalog_entities item ON item.id=ep.entity_id JOIN catalog_asset_links a ON a.entity_id=item.id WHERE sh.public_id=pid_blob(?) AND `+visible+` LIMIT 1`, args...).Scan(&present); err != nil {
			return nil, "", err
		}
	}
	limit = pageLimit(limit)
	after := 0
	if cursor != "" {
		var err error
		after, err = strconv.Atoi(cursor)
		if err != nil || after < 0 {
			return nil, "", errors.New("invalid episode cursor")
		}
	}
	restriction, bound := ItemRestrictionSQL("item.id", viewer.EffectiveRestrictions())
	// PERF-13: show- and season-scoped id shapes so the season page uses
	// the compact season order index.
	var rows *sql.Rows
	var err error
	const tail = ` AND l.library_id IN(SELECT value FROM json_each(?)) AND item.retired=0 AND EXISTS(SELECT 1 FROM catalog_asset_links a WHERE a.entity_id=item.id) AND `
	switch {
	case season != "":
		args := append([]any{season, after, viewer.librariesJSON()}, bound...)
		args = append(args, limit+1)
		rows, err = s.read().Query(`SELECT pid(item.public_id) FROM catalog_entities se JOIN catalog_episodes ep ON ep.season_id=se.id JOIN catalog_entities item ON item.id=ep.entity_id JOIN catalog_libraries l ON l.id=item.library_id WHERE se.public_id=pid_blob(?) AND ep.number>?`+tail+restriction+` ORDER BY ep.number,item.id LIMIT ?`, args...)
	case show != "":
		args := append([]any{show, after, viewer.librariesJSON()}, bound...)
		args = append(args, limit+1)
		rows, err = s.read().Query(`SELECT pid(item.public_id) FROM catalog_entities sh JOIN catalog_episodes ep ON ep.show_id=sh.id JOIN catalog_entities item ON item.id=ep.entity_id JOIN catalog_libraries l ON l.id=item.library_id WHERE sh.public_id=pid_blob(?) AND ep.season_id IS NULL AND ep.number>?`+tail+restriction+` ORDER BY ep.number,item.id LIMIT ?`, args...)
	default:
		args := append([]any{after, viewer.librariesJSON()}, bound...)
		args = append(args, limit+1)
		rows, err = s.read().Query(`SELECT pid(item.public_id) FROM catalog_episodes ep JOIN catalog_entities item ON item.id=ep.entity_id JOIN catalog_libraries l ON l.id=item.library_id WHERE ep.season_id IS NULL AND ep.number>?`+tail+restriction+` ORDER BY ep.number,item.id LIMIT ?`, args...)
	}
	if err != nil {
		return nil, "", err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, "", err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, "", err
	}
	if len(ids) == 0 && cursor == "" && season != "" {
		return nil, "", sql.ErrNoRows
	}
	more := len(ids) > limit
	if more {
		ids = ids[:limit]
	}
	// PERF-13: hydrate the page with the batched mediaPage (two bounded reads
	// plus one batched per-kind enrichment) instead of one Get — three
	// statements — per row.
	out, err := s.mediaPage(viewer.Profile, ids, false)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if more && len(out) > 0 {
		last := out[len(out)-1]
		if last.Episode == nil {
			return nil, "", sql.ErrNoRows
		}
		next = strconv.Itoa(last.Episode.Number)
	}
	return out, next, nil
}
func (s *Service) episodeInfo(id string) (*EpisodeInfo, error) {
	var out EpisodeInfo
	var season sql.NullInt64
	err := s.read().QueryRow(`SELECT pid(sh.public_id),sh.title,COALESCE(pid(se.public_id),''),sd.number,ep.numbering,ep.number,ep.local_identity_status,
 COALESCE((SELECT NULLIF(status,'delegated_tvdb') FROM screen_metadata_work WHERE target_kind='item' AND target_id=item.id AND accepted_publication<>''),(SELECT status FROM tvdb_episode_links WHERE item_id=item.id),'unmatched'),
 ep.ordering_basis,COALESCE(strftime('%Y-%m-%d',ep.air_date*86400,'unixepoch'),''),
 CASE WHEN EXISTS(SELECT 1 FROM episode_asset_boundaries b WHERE b.item_id=item.id AND b.status='unknown_multi_episode') THEN 'unknown_multi_episode' ELSE 'whole_source' END
 FROM catalog_entities item JOIN catalog_episodes ep ON ep.entity_id=item.id JOIN catalog_entities sh ON sh.id=ep.show_id
 LEFT JOIN catalog_entities se ON se.id=ep.season_id LEFT JOIN catalog_seasons sd ON sd.entity_id=se.id
 WHERE item.public_id=pid_blob(?)`, id).Scan(&out.ShowID, &out.ShowTitle, &out.SeasonID, &season, &out.Numbering, &out.Number, &out.LocalIdentityStatus, &out.ProviderMatchStatus, &out.OrderingBasis, &out.AirDate, &out.SourceBoundary)
	if err != nil {
		return nil, err
	}
	if season.Valid {
		number := int(season.Int64)
		out.SeasonNumber = &number
	}
	return &out, nil
}
func (s *Service) EpisodeIssues(library, cursor string, limit int) ([]EpisodeIssue, string, error) {
	limit = pageLimit(limit)
	rows, err := s.read().Query(`SELECT asset_id,source_name,issue FROM episodic_sources WHERE library_id=? AND issue<>'' AND asset_id>? ORDER BY asset_id LIMIT ?`, library, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []EpisodeIssue{}
	for rows.Next() {
		var row EpisodeIssue
		if err = rows.Scan(&row.AssetID, &row.SourceName, &row.Code); err != nil {
			return nil, "", err
		}
		out = append(out, row)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].AssetID
	}
	return out, next, rows.Err()
}
