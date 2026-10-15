package metadata

import (
	"context"
	"database/sql"
	"strings"

	"portico.local/server/internal/dbwork"
)

const DefaultLocalArtworkLimitBytes int64 = 2 << 30

// MetadataSettings exposes credential presence only. Submitted keys never return
// in JSON or provider failures; blank keys restore Portico's application key.
type MetadataSettings struct {
	LocalArtworkUsedBytes    int64 `json:"localArtworkUsedBytes"`
	Revision                 int64 `json:"revision"`
	LocalArtworkLimitBytes   int64 `json:"localArtworkLimitBytes"`
	IncludeAdult             bool  `json:"includeAdult"`
	TMDBCredentialConfigured bool  `json:"tmdbCredentialConfigured"`
	TVDBCredentialConfigured bool  `json:"tvdbCredentialConfigured"`
	// OpenSubtitles, for subtitles found online: whether the owner saved their
	// own API key and password (neither is ever sent back), and the account name.
	OpenSubtitlesKeyConfigured      bool   `json:"openSubtitlesKeyConfigured"`
	OpenSubtitlesUsername           string `json:"openSubtitlesUsername"`
	OpenSubtitlesPasswordConfigured bool   `json:"openSubtitlesPasswordConfigured"`
}
type MetadataSettingsUpdate struct {
	ExpectedRevision       int64   `json:"expectedRevision"`
	LocalArtworkLimitBytes int64   `json:"localArtworkLimitBytes"`
	IncludeAdult           bool    `json:"includeAdult"`
	TMDBReadToken          *string `json:"tmdbReadToken,omitempty"`
	TVDBProjectKey         *string `json:"tvdbProjectKey,omitempty"`
	// Absent: unchanged. Empty: back to the server's own (the environment's).
	OpenSubtitlesAPIKey   *string `json:"openSubtitlesApiKey,omitempty"`
	OpenSubtitlesUsername *string `json:"openSubtitlesUsername,omitempty"`
	OpenSubtitlesPassword *string `json:"openSubtitlesPassword,omitempty"`
}

// subtitleCredentials are the owner's saved OpenSubtitles key and account.
type subtitleCredentials struct{ key, username, password string }

func readMetadataSettings(ctx context.Context, tx *sql.Tx) (MetadataSettings, string, string, error) {
	out, tmdb, tvdb, _, err := readMetadataSettingsWithSubtitles(ctx, tx)
	return out, tmdb, tvdb, err
}
func readMetadataSettingsWithSubtitles(ctx context.Context, tx *sql.Tx) (MetadataSettings, string, string, subtitleCredentials, error) {
	var out MetadataSettings
	var tmdb, tvdb string
	var subs subtitleCredentials
	err := tx.QueryRowContext(ctx, `SELECT revision,local_artwork_limit_bytes,include_adult,tmdb_read_token,tvdb_project_key,opensubtitles_api_key,opensubtitles_username,opensubtitles_password FROM metadata_runtime_settings WHERE singleton=1`).Scan(&out.Revision, &out.LocalArtworkLimitBytes, &out.IncludeAdult, &tmdb, &tvdb, &subs.key, &subs.username, &subs.password)
	out.TMDBCredentialConfigured = tmdb != ""
	out.TVDBCredentialConfigured = tvdb != ""
	out.OpenSubtitlesKeyConfigured = subs.key != ""
	out.OpenSubtitlesUsername = subs.username
	out.OpenSubtitlesPasswordConfigured = subs.password != ""
	return out, tmdb, tvdb, subs, err
}

// validSubtitleAccount bounds an account name or password: printable, one line.
func validSubtitleAccount(v *string) bool {
	if v == nil {
		return true
	}
	if len(*v) > 512 {
		return false
	}
	for _, r := range *v {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
func (s *Service) MetadataSettings(ctx context.Context) (MetadataSettings, error) {
	read, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return MetadataSettings{}, e
	}
	defer read.Rollback()
	out, _, _, e := readMetadataSettings(ctx, read.Tx())
	read.Rollback()
	if e == nil && s.LocalArtworkUsage != nil {
		out.LocalArtworkUsedBytes, e = s.LocalArtworkUsage(ctx)
	}
	return out, e
}
func LocalArtworkLimit(ctx context.Context, db *sql.DB) int64 {
	var limit int64
	if dbwork.QueryRow(ctx, db, `SELECT local_artwork_limit_bytes FROM metadata_runtime_settings WHERE singleton=1`).Scan(&limit) != nil || limit < 1 {
		return DefaultLocalArtworkLimitBytes
	}
	return limit
}
func validProviderCredential(v *string) bool {
	if v == nil {
		return true
	}
	if len(*v) > 8192 {
		return false
	}
	for _, r := range *v {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}
func (s *Service) UpdateMetadataSettings(ctx context.Context, in MetadataSettingsUpdate, authorize func(*sql.Tx) error) (MetadataSettings, error) {
	if in.ExpectedRevision < 1 || in.LocalArtworkLimitBytes < 1 || in.LocalArtworkLimitBytes > 1<<50 || !validProviderCredential(in.TMDBReadToken) || !validProviderCredential(in.TVDBProjectKey) || !validProviderCredential(in.OpenSubtitlesAPIKey) || !validSubtitleAccount(in.OpenSubtitlesUsername) || !validSubtitleAccount(in.OpenSubtitlesPassword) {
		return MetadataSettings{}, ErrRepairInput
	}
	write, e := dbwork.Begin(ctx, s.db, dbwork.ClassInteractive)
	if e != nil {
		return MetadataSettings{}, e
	}
	defer write.Rollback()
	tx := write.Tx()
	if authorize != nil {
		if e = authorize(tx); e != nil {
			return MetadataSettings{}, e
		}
	}
	current, tmdb, tvdb, subs, e := readMetadataSettingsWithSubtitles(ctx, tx)
	if e != nil {
		return current, e
	}
	if current.Revision != in.ExpectedRevision {
		return current, ErrScreenConflict
	}
	if in.TMDBReadToken != nil {
		tmdb = strings.TrimSpace(*in.TMDBReadToken)
	}
	if in.TVDBProjectKey != nil {
		tvdb = strings.TrimSpace(*in.TVDBProjectKey)
	}
	if in.OpenSubtitlesAPIKey != nil {
		subs.key = strings.TrimSpace(*in.OpenSubtitlesAPIKey)
	}
	if in.OpenSubtitlesUsername != nil {
		username := strings.TrimSpace(*in.OpenSubtitlesUsername)
		// A different account (or none) never keeps the previous account's password.
		if username != subs.username {
			subs.password = ""
		}
		subs.username = username
	}
	if in.OpenSubtitlesPassword != nil {
		subs.password = *in.OpenSubtitlesPassword
	}
	_, e = tx.ExecContext(ctx, `UPDATE metadata_runtime_settings SET revision=revision+1,local_artwork_limit_bytes=?,include_adult=?,tmdb_read_token=?,tvdb_project_key=?,opensubtitles_api_key=?,opensubtitles_username=?,opensubtitles_password=? WHERE singleton=1`, in.LocalArtworkLimitBytes, in.IncludeAdult, tmdb, tvdb, subs.key, subs.username, subs.password)
	if e != nil {
		return current, e
	}
	// Previously final authentication failures can recover after a key correction.
	if in.TMDBReadToken != nil || in.TVDBProjectKey != nil {
		if _, e = tx.ExecContext(ctx, `UPDATE screen_metadata_work SET status='pending',next_attempt='',attempts=0,error='',generation=generation+1,revision=revision+1 WHERE status='unavailable' AND error IN('request_rejected','provider_not_configured')`); e != nil {
			return current, e
		}
	}
	out, _, _, e := readMetadataSettings(ctx, tx)
	if e != nil {
		return out, e
	}
	if e = write.Commit(); e != nil {
		return out, e
	}
	if e = s.loadProviderSettings(ctx); e != nil {
		return out, e
	}
	if s.LocalArtworkUsage != nil {
		out.LocalArtworkUsedBytes, e = s.LocalArtworkUsage(ctx)
	}
	return out, e
}
func (s *Service) loadProviderSettings(ctx context.Context) error {
	// Serialize key installation so an older settings read cannot undo a newer save.
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	read, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return e
	}
	out, tmdb, tvdb, subs, e := readMetadataSettingsWithSubtitles(ctx, read.Tx())
	read.Rollback()
	if e != nil {
		return e
	}
	if s.settingsRevision == out.Revision {
		return nil
	}
	if s.SubtitleCredentials != nil {
		s.SubtitleCredentials(subs.key, subs.username, subs.password)
	}
	if tmdb == "" {
		tmdb = s.token
	}
	if tvdb == "" {
		tvdb = TVDBProjectKey
	}
	if s.tmdbPaced != nil {
		s.tmdbPaced.SetCredential(tmdb)
		s.tmdbPaced.SetIncludeAdult(out.IncludeAdult)
	}
	if p, ok := s.tvdb.(interface {
		SetCredential(context.Context, string) error
	}); ok {
		if e = p.SetCredential(ctx, tvdb); e != nil {
			return e
		}
	}
	s.settingsRevision = out.Revision
	return nil
}
