package metadata

import (
	"context"
	"database/sql"
	"portico.local/server/internal/dbwork"
	"slices"
	"strings"
	"time"
)

func validMetadataPreferences(in MetadataSettingsUpdate) bool {
	if in.AcoustIDKey != nil && len(*in.AcoustIDKey) > 256 {
		return false
	}
	enum := func(v *string, choices string) bool {
		return v == nil || slices.Contains(strings.Split(choices, "|"), *v)
	}
	if !enum(in.ArtworkPreference, "folders|online") || !enum(in.ArtworkLanguage, "same|en|textless") || !enum(in.RefreshPolicy, "regular|recent|never") || !enum(in.FallbackLanguage, "en|original|blank") || !enum(in.AnimeTitles, "language|english|romaji|native") {
		return false
	}
	if in.ArtworkMaxEdge != nil && *in.ArtworkMaxEdge != 1280 && *in.ArtworkMaxEdge != 1920 && *in.ArtworkMaxEdge != 2560 {
		return false
	}
	if in.DefaultLanguage != nil && !LibraryLanguageOffered("movie", AgentOnline, *in.DefaultLanguage) {
		return false
	}
	if in.DefaultRegion != nil {
		v := *in.DefaultRegion
		if len(v) != 2 || v[0] < 'A' || v[0] > 'Z' || v[1] < 'A' || v[1] > 'Z' {
			return false
		}
	}
	return true
}

func applyMetadataPreferencesTx(ctx context.Context, tx *sql.Tx, in MetadataSettingsUpdate, old MetadataSettings, now time.Time) error {
	str := func(v *string) any {
		if v == nil {
			return nil
		}
		return *v
	}
	boolean := func(v *bool) any {
		if v == nil {
			return nil
		}
		return *v
	}
	integer := func(v *int) any {
		if v == nil {
			return nil
		}
		return *v
	}
	_, err := tx.ExecContext(ctx, `UPDATE metadata_runtime_settings SET default_language=COALESCE(?,default_language),default_region=COALESCE(?,default_region),artwork_preference=COALESCE(?,artwork_preference),artwork_language=COALESCE(?,artwork_language),refresh_policy=COALESCE(?,refresh_policy),fallback_language=COALESCE(?,fallback_language),anime_titles=COALESCE(?,anime_titles),artwork_max_edge=COALESCE(?,artwork_max_edge),cast_photos=COALESCE(?,cast_photos),acoustid_key=COALESCE(?,acoustid_key) WHERE singleton=1`, str(in.DefaultLanguage), str(in.DefaultRegion), str(in.ArtworkPreference), str(in.ArtworkLanguage), str(in.RefreshPolicy), str(in.FallbackLanguage), str(in.AnimeTitles), integer(in.ArtworkMaxEdge), boolean(in.CastPhotos), str(in.AcoustIDKey))
	if err != nil {
		return err
	}
	changed := in.ArtworkPreference != nil && *in.ArtworkPreference != old.ArtworkPreference || in.ArtworkLanguage != nil && *in.ArtworkLanguage != old.ArtworkLanguage || in.ArtworkMaxEdge != nil && *in.ArtworkMaxEdge != old.ArtworkMaxEdge || in.CastPhotos != nil && *in.CastPhotos != old.CastPhotos
	if changed {
		if _, err = tx.ExecContext(ctx, `UPDATE metadata_artwork_sweep SET running=1,cursor_kind='',cursor_id=0 WHERE singleton=1`); err != nil {
			return err
		}
	}
	if in.ArtworkMaxEdge != nil && *in.ArtworkMaxEdge != old.ArtworkMaxEdge {
		if _, err = tx.ExecContext(ctx, `UPDATE metadata_artwork_size_sweep SET running=?,cursor_kind='',cursor_id=0,cursor_role='',cursor_subject='' WHERE singleton=1`, *in.ArtworkMaxEdge > old.ArtworkMaxEdge); err != nil {
			return err
		}
	}
	if in.AcoustIDKey != nil {
		if _, err = tx.ExecContext(ctx, `UPDATE mb_jobs SET status='pending',revision=revision+1,generation=generation+1,lease='',lease_until='',next_attempt='',attempts=0,error='' WHERE kind='song' AND status IN('unresolved','unavailable','needs_selection') AND selected_id='' AND manual=0 AND EXISTS(SELECT 1 FROM music_match_observations o WHERE o.kind=mb_jobs.kind AND o.entity_id=mb_jobs.entity_id AND o.fingerprint_status IN('credential_required','provider_disabled','provider_unavailable'))`); err != nil {
			return err
		}
	}
	if in.OnlineLookups != nil && *in.OnlineLookups != old.OnlineLookups {
		if _, err = tx.ExecContext(ctx, `UPDATE screen_metadata_consent SET confirmed=?,revision=revision+1,confirmed_at=? WHERE singleton=1`, *in.OnlineLookups, now.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
		if *in.OnlineLookups {
			if _, err = tx.ExecContext(ctx, `UPDATE metadata_artwork_size_requests SET next_attempt='' WHERE (kind,entity_id,role,subject) IN(SELECT kind,entity_id,role,subject FROM metadata_artwork_size_requests ORDER BY next_attempt,kind,entity_id,role,subject LIMIT 32)`); err != nil {
				return err
			}
		}
		// The existing consent revision fences screen work; wake parked work on either edge.
		if _, err = tx.ExecContext(ctx, `UPDATE screen_metadata_work SET status='pending',generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error=''`); err != nil {
			return err
		}
	}
	return nil
}

// globalOnlineTx is an additional inhibition. It never enables a library agent,
// provider, fingerprint permission or a connection to Portico Services.
func globalOnlineTx(ctx context.Context, tx *sql.Tx) (bool, error) {
	var enabled bool
	err := tx.QueryRowContext(ctx, `SELECT confirmed FROM screen_metadata_consent WHERE singleton=1`).Scan(&enabled)
	return enabled, err
}

type artworkSizeKey struct{}

func withArtworkSize(ctx context.Context, edge int) context.Context {
	return context.WithValue(ctx, artworkSizeKey{}, edge)
}
func artworkSize(ctx context.Context) int {
	if edge, ok := ctx.Value(artworkSizeKey{}).(int); ok && (edge == 1280 || edge == 1920 || edge == 2560) {
		return edge
	}
	return artworkLargeEdge
}
func (s *Service) artworkSizeContext(ctx context.Context) (context.Context, error) {
	var edge int
	err := s.db.QueryRowContext(ctx, `SELECT artwork_max_edge FROM metadata_runtime_settings WHERE singleton=1`).Scan(&edge)
	return withArtworkSize(ctx, edge), err
}
func artworkByteLimit(edge int) int {
	if edge == 2560 {
		return 888_888
	}
	return artworkDisplayBytes
}

func (s *Service) artworkPreferences(ctx context.Context) (string, bool, error) {
	var locale string
	var photos bool
	err := s.db.QueryRowContext(ctx, `SELECT artwork_language,cast_photos FROM metadata_runtime_settings WHERE singleton=1`).Scan(&locale, &photos)
	return locale, photos, err
}

func preferredArtworkLanguageRank(image, locale, preference string) int {
	if preference == "textless" {
		if image == "" || image == "null" {
			return 0
		}
		return 1 + artworkLanguageRank(image, locale)
	}
	return artworkLanguageRank(image, locale)
}
func (s *Service) artworkJobEnabled(ctx context.Context, tx *sql.Tx, j artworkWork) (bool, error) {
	if j.role == "portrait" && (j.provider == "tmdb" || j.provider == "tvdb") && j.actor == "" {
		var enabled bool
		if e := tx.QueryRowContext(ctx, `SELECT cast_photos FROM metadata_runtime_settings WHERE singleton=1`).Scan(&enabled); e != nil || !enabled {
			return false, e
		}
	}
	return s.artworkProviderEnabled(ctx, tx, j.target, j.provider)
}

// GlobalArtworkPreferencesStep reseeds automatic choices after a global artwork
// preference change. Owner selections remain locked; the ordinary candidate and
// publication guards enforce folder/online precedence and provider consent.
func (s *Service) GlobalArtworkPreferencesStep(ctx context.Context) error {
	w, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer w.Rollback()
	tx := w.Tx()
	var kind string
	var cursor int64
	var running bool
	if err = tx.QueryRowContext(ctx, `SELECT running,cursor_kind,cursor_id FROM metadata_artwork_sweep WHERE singleton=1`).Scan(&running, &kind, &cursor); err != nil || !running {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT kind,entity_id FROM artwork_entity_heads WHERE (kind,entity_id)>(?,?) ORDER BY kind,entity_id LIMIT 32`, kind, cursor)
	if err != nil {
		return err
	}
	type target struct {
		kind string
		id   int64
	}
	targets := []target{}
	for rows.Next() {
		var t target
		if err = rows.Scan(&t.kind, &t.id); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, t := range targets {
		kind, cursor = t.kind, t.id
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO artwork_dirty(kind,entity_id) VALUES(?,?)`, t.kind, t.id); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE metadata_artwork_sweep SET running=?,cursor_kind=?,cursor_id=? WHERE singleton=1`, len(targets) == 32, kind, cursor); err != nil {
		return err
	}
	return w.Commit()
}

func (s *Service) requireGlobalOnline(ctx context.Context) error {
	var enabled bool
	if err := dbwork.QueryRow(ctx, s.db, `SELECT confirmed FROM screen_metadata_consent WHERE singleton=1`).Scan(&enabled); err != nil {
		return err
	}
	if !enabled {
		return ErrLocalMetadataOnly
	}
	return nil
}
