package metadata

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
)

const DefaultLocalArtworkLimitBytes int64 = 2 << 30

// MetadataSettings exposes credential presence only. Submitted keys never return
// in JSON or provider failures; blank keys restore Portico's application key.
type MetadataSettings struct {
	OnlineLookups         bool   `json:"onlineLookups"`
	DefaultLanguage       string `json:"defaultLanguage"`
	DefaultRegion         string `json:"defaultRegion"`
	ArtworkPreference     string `json:"artworkPreference"`
	ArtworkLanguage       string `json:"artworkLanguage"`
	RefreshPolicy         string `json:"refreshPolicy"`
	FallbackLanguage      string `json:"fallbackLanguage"`
	AnimeTitles           string `json:"animeTitles"`
	ArtworkMaxEdge        int    `json:"artworkMaxEdge"`
	CastPhotos            bool   `json:"castPhotos"`
	AcoustIDKeyConfigured bool   `json:"acoustidKeyConfigured"`

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
	// Where the key and account in force come from and how OpenSubtitles last answered.
	OpenSubtitles OpenSubtitlesStatus `json:"openSubtitles"`
}

// OpenSubtitlesStatus mirrors subtitles.OwnerStatus field for field (the
// server converts one to the other); it never carries a secret.
type OpenSubtitlesStatus struct {
	// "owner", "environment" or "none".
	KeySource     string `json:"keySource"`
	AccountSource string `json:"accountSource"`
	// "none", "incomplete", "not_checked", "signed_in" or "rejected".
	Account string `json:"account"`
	// The last refusal (provider_key_rejected, provider_sign_in_failed,
	// provider_sign_in_required, provider_quota_exhausted, provider_rate_limited,
	// provider_unreachable); empty when the last exchange worked.
	Problem   string `json:"problem,omitempty"`
	CheckedAt string `json:"checkedAt,omitempty"`
	RetryAt   string `json:"retryAt,omitempty"`
	// As OpenSubtitles last reported them; -1 when unknown.
	DownloadsRemaining int    `json:"downloadsRemaining"`
	DownloadsAllowed   int    `json:"downloadsAllowed"`
	DownloadsResetAt   string `json:"downloadsResetAt,omitempty"`
}

var noSubtitleProvider = OpenSubtitlesStatus{KeySource: "none", AccountSource: "none", Account: "none", DownloadsRemaining: -1, DownloadsAllowed: -1}

func (s *Service) subtitleStatus() OpenSubtitlesStatus {
	if s.SubtitleStatus == nil {
		return noSubtitleProvider
	}
	return s.SubtitleStatus()
}

// CheckOpenSubtitles asks OpenSubtitles whether the credentials in force work
// now (Server › Metadata › Test connection).
func (s *Service) CheckOpenSubtitles(ctx context.Context) (OpenSubtitlesStatus, error) {
	if e := s.loadProviderSettings(ctx); e != nil {
		return noSubtitleProvider, e
	}
	if s.SubtitleCheck == nil {
		return noSubtitleProvider, nil
	}
	return s.SubtitleCheck(ctx), nil
}

type MetadataSettingsUpdate struct {
	OnlineLookups     *bool   `json:"onlineLookups,omitempty"`
	DefaultLanguage   *string `json:"defaultLanguage,omitempty"`
	DefaultRegion     *string `json:"defaultRegion,omitempty"`
	ArtworkPreference *string `json:"artworkPreference,omitempty"`
	ArtworkLanguage   *string `json:"artworkLanguage,omitempty"`
	RefreshPolicy     *string `json:"refreshPolicy,omitempty"`
	FallbackLanguage  *string `json:"fallbackLanguage,omitempty"`
	AnimeTitles       *string `json:"animeTitles,omitempty"`
	ArtworkMaxEdge    *int    `json:"artworkMaxEdge,omitempty"`
	CastPhotos        *bool   `json:"castPhotos,omitempty"`
	AcoustIDKey       *string `json:"acoustidKey,omitempty"`

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
	err := tx.QueryRowContext(ctx, `SELECT r.revision+c.revision-1,r.local_artwork_limit_bytes,r.include_adult,r.tmdb_read_token,r.tvdb_project_key,r.opensubtitles_api_key,r.opensubtitles_username,r.opensubtitles_password,c.confirmed,r.default_language,r.default_region,r.artwork_preference,r.artwork_language,r.refresh_policy,r.fallback_language,r.anime_titles,r.artwork_max_edge,r.cast_photos,r.acoustid_key<>'' FROM metadata_runtime_settings r CROSS JOIN screen_metadata_consent c WHERE r.singleton=1 AND c.singleton=1`).Scan(&out.Revision, &out.LocalArtworkLimitBytes, &out.IncludeAdult, &tmdb, &tvdb, &subs.key, &subs.username, &subs.password, &out.OnlineLookups, &out.DefaultLanguage, &out.DefaultRegion, &out.ArtworkPreference, &out.ArtworkLanguage, &out.RefreshPolicy, &out.FallbackLanguage, &out.AnimeTitles, &out.ArtworkMaxEdge, &out.CastPhotos, &out.AcoustIDKeyConfigured)
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
	out.OpenSubtitles = s.subtitleStatus()
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
	if in.ExpectedRevision < 1 || in.LocalArtworkLimitBytes < 1 || in.LocalArtworkLimitBytes > 1<<50 || !validMetadataPreferences(in) || !validProviderCredential(in.AcoustIDKey) || !validProviderCredential(in.TMDBReadToken) || !validProviderCredential(in.TVDBProjectKey) || !validProviderCredential(in.OpenSubtitlesAPIKey) || !validSubtitleAccount(in.OpenSubtitlesUsername) || !validSubtitleAccount(in.OpenSubtitlesPassword) {
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
	out, e := ApplyMetadataSettingsTx(ctx, tx, in, s.publicationTime())
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
	out.OpenSubtitles = s.subtitleStatus()
	return out, e
}

// ReadMetadataSettingsTx is the sanitized view for same-transaction configuration
// projections. No credential value or assumed provider health leaves this boundary.
func ReadMetadataSettingsTx(ctx context.Context, tx *sql.Tx) (MetadataSettings, error) {
	out, _, _, err := readMetadataSettings(ctx, tx)
	out.OpenSubtitles = noSubtitleProvider
	return out, err
}

// ApplyMetadataSettingsTx is the normalized, validated writer used by the owner
// facade and atomic configuration imports. The caller authorizes and commits the
// transaction. The provider worker installs committed keys on its next wake.
func ApplyMetadataSettingsTx(ctx context.Context, tx *sql.Tx, in MetadataSettingsUpdate, now time.Time) (MetadataSettings, error) {
	if in.ExpectedRevision < 1 || in.LocalArtworkLimitBytes < 1 || in.LocalArtworkLimitBytes > 1<<50 || !validMetadataPreferences(in) || !validProviderCredential(in.AcoustIDKey) || !validProviderCredential(in.TMDBReadToken) || !validProviderCredential(in.TVDBProjectKey) || !validProviderCredential(in.OpenSubtitlesAPIKey) || !validSubtitleAccount(in.OpenSubtitlesUsername) || !validSubtitleAccount(in.OpenSubtitlesPassword) {
		return MetadataSettings{}, ErrRepairInput
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

	if e = applyMetadataPreferencesTx(ctx, tx, in, current, now); e != nil {
		return current, e
	}
	// Previously final authentication failures can recover after a key correction.
	if in.TMDBReadToken != nil || in.TVDBProjectKey != nil {
		if _, e = tx.ExecContext(ctx, `UPDATE screen_metadata_work SET status='pending',next_attempt='',attempts=0,error='',generation=generation+1,revision=revision+1 WHERE status='unavailable' AND error IN('request_rejected','provider_not_configured')`); e != nil {
			return current, e
		}
	}
	return ReadMetadataSettingsTx(ctx, tx)
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
	if e = s.loadAcoustIDSettings(ctx); e != nil {
		return e
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
