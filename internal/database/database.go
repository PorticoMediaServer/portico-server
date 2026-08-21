package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	_ "modernc.org/sqlite"
)

type StartupPhase struct {
	ID          string
	Label       string
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration
	Error       string
}

type StartupReporter func(StartupPhase)

type sqliteExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Begin() (*sql.Tx, error)
}

// sqliteResourcePolicy is the intentionally conservative runtime SQLite
// preset. SQLite serializes writers, so a large pool only multiplies lock
// competition and each connection's private page cache. The preset keeps the
// configured page-cache ceiling at 32 MiB (8 connections x 4 MiB), retains at
// most four idle handles after a read burst, and disables mmap so address-space
// use is not an untracked part of the database budget. Temporary query memory
// is workload-dependent and is not included in this page-cache ceiling.
type sqliteResourcePolicy struct {
	MaxOpenConns    int
	MaxIdleConns    int
	CacheSizeKiB    int64
	MmapSizeBytes   int64
	ConnMaxLifetime time.Duration
}

const (
	sqliteSafeMaxOpenConns    = 8
	sqliteSafeMaxIdleConns    = 4
	sqliteSafeCacheSizeKiB    = 4 * 1024
	sqliteSafeMmapSizeBytes   = 0
	sqliteSafeConnMaxLifetime = 30 * time.Minute
)

func defaultSQLiteResourcePolicy() sqliteResourcePolicy {
	return sqliteResourcePolicy{
		MaxOpenConns:    sqliteSafeMaxOpenConns,
		MaxIdleConns:    sqliteSafeMaxIdleConns,
		CacheSizeKiB:    sqliteSafeCacheSizeKiB,
		MmapSizeBytes:   sqliteSafeMmapSizeBytes,
		ConnMaxLifetime: sqliteSafeConnMaxLifetime,
	}
}

// sqliteResourcePolicyForConfig deliberately returns the bounded preset. The
// current server Config has no validated host-memory budget; unrelated inputs
// such as the restore upload limit must not be used as a proxy for one.
func sqliteResourcePolicyForConfig(_ config.Config) sqliteResourcePolicy {
	return defaultSQLiteResourcePolicy()
}

func (policy sqliteResourcePolicy) validate() error {
	switch {
	case policy.MaxOpenConns < 1 || policy.MaxOpenConns > sqliteSafeMaxOpenConns:
		return fmt.Errorf("sqlite max open connections %d is outside [1,%d]", policy.MaxOpenConns, sqliteSafeMaxOpenConns)
	case policy.MaxIdleConns < 1 || policy.MaxIdleConns > policy.MaxOpenConns || policy.MaxIdleConns > sqliteSafeMaxIdleConns:
		return fmt.Errorf("sqlite max idle connections %d is outside [1,%d] and must not exceed max open connections %d", policy.MaxIdleConns, sqliteSafeMaxIdleConns, policy.MaxOpenConns)
	case policy.CacheSizeKiB < 1 || policy.CacheSizeKiB > sqliteSafeCacheSizeKiB:
		return fmt.Errorf("sqlite cache size %d KiB is outside [1,%d]", policy.CacheSizeKiB, sqliteSafeCacheSizeKiB)
	case policy.MmapSizeBytes != sqliteSafeMmapSizeBytes:
		return fmt.Errorf("sqlite mmap size %d bytes is not the safe disabled value", policy.MmapSizeBytes)
	case policy.ConnMaxLifetime <= 0:
		return fmt.Errorf("sqlite connection max lifetime %s must be positive", policy.ConnMaxLifetime)
	default:
		return nil
	}
}

func applySQLiteResourcePolicy(db *sql.DB, policy sqliteResourcePolicy) error {
	if db == nil {
		return errors.New("database is required")
	}
	if err := policy.validate(); err != nil {
		return err
	}
	db.SetMaxOpenConns(policy.MaxOpenConns)
	db.SetMaxIdleConns(policy.MaxIdleConns)
	db.SetConnMaxLifetime(policy.ConnMaxLifetime)
	return nil
}

func sqliteRuntimePragmas(policy sqliteResourcePolicy) string {
	return fmt.Sprintf(`
		PRAGMA foreign_keys = ON;
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
		PRAGMA busy_timeout = 15000;
		PRAGMA wal_autocheckpoint = 1000;
		PRAGMA temp_store = MEMORY;
		PRAGMA cache_size = -%d;
		PRAGMA mmap_size = %d;
	`, policy.CacheSizeKiB, policy.MmapSizeBytes)
}

func Open(cfg config.Config) (*sql.DB, error) {
	return OpenContext(context.Background(), cfg)
}

func OpenWithReporter(cfg config.Config, reporter StartupReporter) (*sql.DB, error) {
	return OpenWithReporterContext(context.Background(), cfg, reporter)
}

// OpenContext is the host-supervised open/migrate path. SQLite calls made
// through the migration connection receive this context; a platform syscall
// which cannot be interrupted still keeps its caller in maintenance until it
// returns, so a timeout never permits a concurrent replacement.
func OpenContext(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	return OpenWithReporterContext(ctx, cfg, nil)
}

func OpenWithReporterContext(ctx context.Context, cfg config.Config, reporter StartupReporter) (*sql.DB, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate runtime configuration: %w", err)
	}
	if err := ValidateEmbeddedMigrationBundle(); err != nil {
		return nil, fmt.Errorf("validate embedded migration bundle: %w", err)
	}
	reportPhase := func(id, label string, started time.Time, err error) {
		if reporter == nil {
			return
		}
		completed := time.Now()
		phase := StartupPhase{
			ID:          id,
			Label:       label,
			StartedAt:   started,
			CompletedAt: completed,
			Duration:    completed.Sub(started),
		}
		if err != nil {
			phase.Error = err.Error()
		}
		reporter(phase)
	}
	runPhase := func(id, label string, fn func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		started := time.Now()
		err := fn()
		if err == nil {
			err = ctx.Err()
		}
		reportPhase(id, label, started, err)
		return err
	}

	db, err := openConfiguredSQLiteHandleContext(ctx, cfg, reportPhase)
	if err != nil {
		return nil, err
	}

	if err := runPhase("sqlite_migrate", "Run database migrations", func() error {
		return migrateWithPathContext(ctx, db, cfg.DatabasePath)
	}); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := runPhase("sqlite_seed", "Seed baseline data", func() error {
		return seed(db, cfg)
	}); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func OpenRuntimeHandle(cfg config.Config) (*sql.DB, error) {
	return openConfiguredSQLiteHandle(context.Background(), cfg, nil)
}

func openConfiguredSQLiteHandle(ctx context.Context, cfg config.Config, reportPhase func(string, string, time.Time, error)) (*sql.DB, error) {
	return openConfiguredSQLiteHandleContext(ctx, cfg, reportPhase)
}

func openConfiguredSQLiteHandleContext(ctx context.Context, cfg config.Config, reportPhase func(string, string, time.Time, error)) (*sql.DB, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	policy := sqliteResourcePolicyForConfig(cfg)
	if err := policy.validate(); err != nil {
		return nil, fmt.Errorf("validate sqlite resource policy: %w", err)
	}
	report := func(id, label string, started time.Time, err error) {
		if reportPhase != nil {
			reportPhase(id, label, started, err)
		}
	}
	runPhase := func(id, label string, fn func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		started := time.Now()
		err := fn()
		if err == nil {
			err = ctx.Err()
		}
		report(id, label, started, err)
		return err
	}

	if err := preparePrivateDataPaths(cfg); err != nil {
		return nil, err
	}

	openStarted := time.Now()
	db, err := sql.Open("sqlite", sqliteDSN(cfg.DatabasePath))
	report("sqlite_open", "Open SQLite handle", openStarted, err)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// Keep one writer serialized by the application scheduler while allowing a
	// bounded number of WAL readers. The policy is deliberately small because
	// more SQLite handles do not create more write capacity and multiply
	// connection-local cache and lock contention.
	if err := applySQLiteResourcePolicy(db, policy); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply sqlite resource policy: %w", err)
	}
	if err := runPhase("sqlite_pragmas", "Configure SQLite pragmas", func() error {
		_, err := retrySQLiteLock(ctx, migrationRetryOptions, func() error {
			_, execErr := db.ExecContext(ctx, sqliteRuntimePragmas(policy))
			return execErr
		})
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}
	if err := runPhase("sqlite_path_permissions", "Verify SQLite sidecar permissions", func() error {
		return enforcePrivateSQLiteArtifacts(cfg)
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("verify sqlite path permissions: %w", err)
	}
	return db, nil
}

func sqliteDSN(path string) string {
	policy := sqliteResourcePolicyForConfig(config.Config{})
	values := url.Values{}
	values.Add("_pragma", "busy_timeout=15000")
	values.Add("_pragma", "foreign_keys(ON)")
	values.Add("_pragma", "journal_mode(WAL)")
	values.Add("_pragma", "synchronous(NORMAL)")
	values.Add("_pragma", "wal_autocheckpoint(1000)")
	values.Add("_pragma", "temp_store(MEMORY)")
	values.Add("_pragma", fmt.Sprintf("cache_size(-%d)", policy.CacheSizeKiB))
	values.Add("_pragma", fmt.Sprintf("mmap_size(%d)", policy.MmapSizeBytes))
	return path + "?" + values.Encode()
}

func maintainSQLiteAfterOpen(db *sql.DB) error {
	if _, err := db.Exec(`
		PRAGMA analysis_limit = 1000;
		PRAGMA optimize;
		ANALYZE;
		PRAGMA wal_checkpoint(PASSIVE);
	`); err != nil {
		return fmt.Errorf("maintain sqlite after open: %w", err)
	}
	return nil
}

func migrate(db *sql.DB) error {
	return migrateWithPath(db, "")
}

func migrateWithPath(db *sql.DB, databasePath string) error {
	return migrateWithPathContext(context.Background(), db, databasePath)
}

func migrateWithPathContext(ctx context.Context, db *sql.DB, _ string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil {
		return errors.New("database is required")
	}
	policy := sqliteResourcePolicyForConfig(config.Config{})
	if err := policy.validate(); err != nil {
		return fmt.Errorf("validate sqlite resource policy: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer func() {
		_ = applySQLiteResourcePolicy(db, policy)
	}()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve migration connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `
		PRAGMA foreign_keys = ON;
		PRAGMA busy_timeout = 15000;
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
	`); err != nil {
		return fmt.Errorf("configure migration connection: %w", err)
	}
	if err := runMigrationsOnConnection(ctx, conn, embeddedMigrationSource()); err != nil {
		return err
	}
	return ctx.Err()
}

func quoteSQLiteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

type usersDependentSchemaObject struct {
	Kind string
	Name string
	SQL  string
}

func verifySQLiteIntegrity(db sqliteExecutor) error {
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("integrity_check=%q", integrity)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table, rowID, parent, foreignKey any
		if err := rows.Scan(&table, &rowID, &parent, &foreignKey); err != nil {
			return fmt.Errorf("read foreign_key_check: %w", err)
		}
		return fmt.Errorf("foreign_key_check row=(%v,%v,%v,%v)", table, rowID, parent, foreignKey)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate foreign_key_check: %w", err)
	}
	return nil
}

func mediaRandomKey(id string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(strings.TrimSpace(id)))
	return fmt.Sprintf("%016x", hash.Sum64())
}

type librarySeed struct {
	ID        string
	Name      string
	Type      string
	SortOrder int
	Path      string
}

type mediaSeed struct {
	ID              string
	LibraryID       string
	ParentID        string
	Type            string
	Title           string
	SortTitle       string
	Year            int
	DurationSeconds int
	Summary         string
	Tagline         string
	ContentRating   string
	CommunityRating float64
	CriticRating    int
	Studio          string
	Genres          []string
	AddedAt         string
	SeasonNumber    int
	EpisodeNumber   int
	IndexNumber     int
	ArtSeed         string
	SourceURL       string
}

func seed(db *sql.DB, cfg config.Config) error {
	now := time.Now().UTC()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = seedLocalizationCatalog(tx, now); err != nil {
		return err
	}

	if cfg.SampleMediaURL != "" {
		libraries := []librarySeed{
			{ID: "lib_movies", Name: "Movies", Type: "movie", SortOrder: 10, Path: "/media/movies"},
			{ID: "lib_movies_4k", Name: "4K Movies", Type: "movie", SortOrder: 11, Path: "/media/movies-4k"},
			{ID: "lib_family_movies", Name: "Family Movies", Type: "movie", SortOrder: 12, Path: "/media/family-movies"},
			{ID: "lib_tv", Name: "TV Shows", Type: "show", SortOrder: 20, Path: "/media/tv"},
			{ID: "lib_tv_classics", Name: "Classic TV", Type: "show", SortOrder: 21, Path: "/media/tv-classics"},
			{ID: "lib_anime", Name: "Anime", Type: "anime", SortOrder: 30, Path: "/media/anime"},
			{ID: "lib_music", Name: "Music", Type: "music", SortOrder: 40, Path: "/media/music"},
			{ID: "lib_audiobooks", Name: "Audiobooks", Type: "audiobook", SortOrder: 50, Path: "/media/audiobooks"},
		}
		for _, lib := range libraries {
			_, err = tx.Exec(`
			INSERT INTO libraries (id, name, type, sort_order, path, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				type = excluded.type,
				sort_order = excluded.sort_order,
				path = CASE WHEN libraries.path = '' THEN excluded.path ELSE libraries.path END`,
				lib.ID, lib.Name, lib.Type, lib.SortOrder, lib.Path, now.Format(time.RFC3339))
			if err != nil {
				return fmt.Errorf("seed library %s: %w", lib.ID, err)
			}
			_, err = tx.Exec(`
			INSERT INTO library_paths (id, library_id, path, sort_order, created_at)
			SELECT ?, ?, ?, 0, ?
			WHERE ? <> ''
				AND NOT EXISTS (SELECT 1 FROM library_paths WHERE library_id = ?)`,
				"path_"+lib.ID, lib.ID, lib.Path, now.Format(time.RFC3339), lib.Path, lib.ID)
			if err != nil {
				return fmt.Errorf("seed library path %s: %w", lib.ID, err)
			}
		}

		items := seededMedia(now, cfg.SampleMediaURL)
		for _, item := range items {
			genres, marshalErr := json.Marshal(item.Genres)
			if marshalErr != nil {
				return marshalErr
			}
			_, err = tx.Exec(`
			INSERT INTO media_items (
				id, library_id, parent_id, type, title, sort_title, year, duration_seconds,
				summary, tagline, content_rating, community_rating, critic_rating, studio, genres_json,
				added_at, season_number, episode_number, index_number, art_seed, source_url, random_key
			) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				library_id = excluded.library_id,
				parent_id = excluded.parent_id,
				random_key = CASE WHEN COALESCE(media_items.random_key, '') = '' THEN excluded.random_key ELSE media_items.random_key END,
				studio = CASE WHEN media_items.studio = '' THEN excluded.studio ELSE media_items.studio END,
				season_number = excluded.season_number,
				episode_number = excluded.episode_number,
				index_number = excluded.index_number,
				source_url = CASE WHEN media_items.source_url = '' THEN excluded.source_url ELSE media_items.source_url END`,
				item.ID, item.LibraryID, item.ParentID, item.Type, item.Title, item.SortTitle, item.Year, item.DurationSeconds,
				item.Summary, item.Tagline, item.ContentRating, item.CommunityRating, item.CriticRating, item.Studio, string(genres),
				item.AddedAt, item.SeasonNumber, item.EpisodeNumber, item.IndexNumber, item.ArtSeed, item.SourceURL, mediaRandomKey(item.ID))
			if err != nil {
				return fmt.Errorf("seed media %s: %w", item.ID, err)
			}
		}

		for _, item := range items {
			if item.Type == "season" {
				continue
			}
			if _, err = tx.Exec(`DELETE FROM media_search WHERE media_id = ?`, item.ID); err != nil {
				return fmt.Errorf("reset search %s: %w", item.ID, err)
			}
			keywords := []string{
				item.Type,
				item.ContentRating,
				item.Studio,
			}
			if item.Year > 0 {
				keywords = append(keywords, strconv.Itoa(item.Year))
			}
			keywords = append(keywords, item.Genres...)
			_, err = tx.Exec(`INSERT INTO media_search (media_id, title, summary, genres) VALUES (?, ?, ?, ?)`,
				item.ID, strings.Join(nonEmptySeedSearchParts([]string{item.Title, item.SortTitle}), " "), strings.Join(nonEmptySeedSearchParts([]string{item.Summary, item.Tagline}), " "), strings.Join(nonEmptySeedSearchParts(keywords), " "))
			if err != nil {
				return fmt.Errorf("seed search %s: %w", item.ID, err)
			}
		}

		_, err = tx.Exec(`
			INSERT INTO media_lyrics (id, media_id, source, provider, format, language, path, text, synced, created_at)
			VALUES (
				lower(hex(randomblob(20))),
				'track_mara_01',
				'local',
				'seed',
				'lrc',
				'en',
				'',
				'[00:01.00]Platform lights are blinking in time
[00:12.00]Late trains carry static and shine
[00:24.00]Every echo finds its way home',
				1,
				?
			)
			ON CONFLICT(media_id, source, path, language, format) DO UPDATE SET
				text = excluded.text,
				synced = excluded.synced,
				created_at = excluded.created_at`,
			now.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("seed lyrics: %w", err)
		}

		streamRows := []struct {
			ID, MediaID, Kind, Codec, Language, DisplayTitle string
			Channels, Bitrate, Width, Height                 int
		}{
			{"stream_m1_video", "movie_meridian", "video", "hevc", "", "4K DoVi/HDR10 (HEVC Main 10)", 0, 18400, 3840, 2160},
			{"stream_m1_audio", "movie_meridian", "audio", "truehd", "English", "English - TrueHD 7.1", 8, 4200, 0, 0},
			{"stream_m1_sub", "movie_meridian", "subtitle", "srt", "English", "English", 0, 0, 0, 0},
			{"stream_m2_video", "movie_neon", "video", "h264", "", "1080p (H.264)", 0, 8200, 1920, 1080},
			{"stream_m2_audio", "movie_neon", "audio", "aac", "English", "English - AAC 5.1", 6, 640, 0, 0},
			{"stream_e1_video", "episode_northbridge_101", "video", "h264", "", "1080p (H.264)", 0, 6400, 1920, 1080},
			{"stream_e1_audio", "episode_northbridge_101", "audio", "eac3", "English", "English - EAC3 5.1", 6, 768, 0, 0},
			{"stream_a1_video", "anime_starrail_101", "video", "h264", "", "1080p (H.264)", 0, 4800, 1920, 1080},
			{"stream_a1_audio", "anime_starrail_101", "audio", "aac", "Japanese", "Japanese - AAC Stereo", 2, 256, 0, 0},
		}
		for _, row := range streamRows {
			identityKey := "seed\x00" + row.ID
			if _, err = tx.Exec(`
				INSERT INTO public_resource_identity_keys (resource_kind, identity_key, resource_id, created_at, updated_at)
				VALUES ('media-stream', ?, lower(hex(randomblob(20))), ?, ?)
				ON CONFLICT(resource_kind, identity_key) DO UPDATE SET updated_at = excluded.updated_at`,
				identityKey, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
				return fmt.Errorf("seed stream identity %s: %w", row.ID, err)
			}
			var streamID string
			if err = tx.QueryRow(`SELECT resource_id FROM public_resource_identity_keys WHERE resource_kind = 'media-stream' AND identity_key = ?`, identityKey).Scan(&streamID); err != nil {
				return fmt.Errorf("read seed stream identity %s: %w", row.ID, err)
			}
			_, err = tx.Exec(`
			INSERT INTO media_streams (id, media_id, source_kind, source_identity, storage_key, stream_index, kind, codec, language, channels, bitrate, width, height, display_title)
			VALUES (?, ?, 'seed', ?, ?, -1, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				media_id = excluded.media_id,
				kind = excluded.kind,
				codec = excluded.codec,
				language = excluded.language,
				channels = excluded.channels,
				bitrate = excluded.bitrate,
				width = excluded.width,
				height = excluded.height,
				display_title = excluded.display_title`,
				streamID, row.MediaID, row.ID, streamID, row.Kind, row.Codec, row.Language, row.Channels, row.Bitrate, row.Width, row.Height, row.DisplayTitle)
			if err != nil {
				return fmt.Errorf("seed stream %s: %w", row.ID, err)
			}
		}
	}

	defaultSettings := map[string]any{
		"server": map[string]any{
			"friendlyName":        "Portico",
			"operatorNote":        "",
			"remoteAccess":        false,
			"secureConnections":   "preferred",
			"preferredInterface":  "automatic",
			"claimMode":           "local",
			"manualPortMapping":   false,
			"manualPort":          32500,
			"publishServer":       false,
			"scanAutomatically":   true,
			"emptyTrashAfterScan": false,
		},
		"remoteAccess": map[string]any{
			"enabled":        false,
			"manualPort":     32500,
			"connectionMode": "directOnly",
			"status":         "private",
		},
		"metadataAgents": map[string]any{
			"movies":               "TMDB",
			"tv":                   "TMDB",
			"music":                "MusicBrainz",
			"localNFO":             true,
			"embeddedTags":         true,
			"cacheOriginalArtwork": false,
			"refreshDays":          7,
			"metadataLanguage":     "en-US",
		},
		"library": map[string]any{
			"scanAutomatically":       true,
			"scanOnFilesystemChanges": true,
			"analyzeOnScan":           true,
			"emptyTrashAfterScan":     false,
			"trashRetentionDays":      30,
			"allowMediaDeletion":      false,
			"generateVideoPreview":    "scheduled",
			"chapterThumbnailMode":    "scheduled",
		},
		"network": map[string]any{
			"secureConnections": "preferred",
			"lanNetworks":       "",
			"customAccessUrls":  "",
			"enableIpv6":        true,
		},
		"transcoder": map[string]any{
			"enabled":                true,
			"quality":                "automatic",
			"temporaryDirectory":     "",
			"throttleBufferSeconds":  60,
			"playedRetentionSeconds": 300,
			"x264Preset":             "veryfast",
			"hdrToneMapping":         true,
			"hardwareAcceleration":   true,
			"hardwareEncoding":       false,
			"hardwareDecodeHEVC":     true,
			"hardwareDevice":         "auto",
			"maxConcurrentSessions":  2,
			"backgroundTranscodeFPS": 60,
		},
		"languages": map[string]any{
			"audio":            "English",
			"subtitle":         "English",
			"subtitleMode":     "foreignAudio",
			"preferForcedSubs": true,
		},
		"dlna": map[string]any{
			"enabled":            false,
			"friendlyName":       "",
			"advertiseUrl":       "",
			"exposedLibraries":   []any{},
			"reportTimeline":     true,
			"clientDiscoverySec": 60,
		},
		"scheduledTasks": map[string]any{
			"enabled":                  true,
			"maintenanceWindow":        "overnight",
			"maintenanceDays":          "every-day",
			"startHour":                2,
			"endHour":                  5,
			"backupDatabase":           true,
			"backupCadence":            "daily",
			"backupRetentionDays":      14,
			"scanLibraries":            true,
			"libraryScanCadence":       "daily",
			"libraryScanIntervalHours": 24,
			"refreshMetadata":          false,
			"metadataRefreshCadence":   "daily",
			"metadataRefreshDays":      14,
			"analyzeMedia":             true,
			"analysisCadence":          "daily",
			"emptyTrash":               false,
			"trashRetentionDays":       30,
			"trickplayIntervalSeconds": 0,
			"trickplayTileWidth":       160,
			"trickplayMaxTiles":        240,
			"taskTriggers": map[string]any{
				"database-backup":  map[string]any{"enabled": true, "intervalHours": 24},
				"library-scan":     map[string]any{"enabled": true, "intervalHours": 24},
				"metadata-refresh": map[string]any{"enabled": false, "intervalHours": 24},
				"media-analysis":   map[string]any{"enabled": true, "intervalHours": 24},
			},
		},
		"extras": map[string]any{
			"cinemaTrailers": 0,
		},
		"optimizedVersions": map[string]any{
			"defaultProfile": "720p-medium",
			"templates": []map[string]any{
				{"id": "template-1080p-high", "name": "1080p High", "profile": "1080p-high", "enabled": true},
				{"id": "template-1080p-medium", "name": "1080p Medium", "profile": "1080p-medium", "enabled": true},
				{"id": "template-720p-high", "name": "720p High", "profile": "720p-high", "enabled": true},
				{"id": "template-720p-medium", "name": "720p Medium", "profile": "720p-medium", "enabled": true},
				{"id": "template-480p", "name": "480p", "profile": "480p", "enabled": true},
			},
			"preferOptimizedPlayback": false,
			"storageDirectory":        "",
			"maxConcurrentJobs":       1,
			"autoDelete":              false,
			"retentionDays":           0,
			"maxPerItem":              3,
			"maxStorageMB":            0,
		},
		"liveTv": map[string]any{
			"enabled":         false,
			"guideDays":       14,
			"transcodeLiveTv": true,
		},
		"dvr": map[string]any{
			"defaultStartPaddingMinutes": 0,
			"defaultEndPaddingMinutes":   0,
			"defaultRetentionDays":       30,
			"defaultFolder":              "",
			"recordingPathTemplate":      "{folder}/{year}/{month}/{title}-{start}",
			"recordingProfile":           "copy",
			"saveNFO":                    false,
			"saveImageSidecars":          false,
			"preserveAllStreams":         true,
		},
		"troubleshooting": map[string]any{
			"logLevel": "info",
		},
		"notifications": map[string]any{
			"enabled":       true,
			"minAlertLevel": "warn",
		},
		"console": map[string]any{
			"level":     "info",
			"tailLines": 200,
		},
		"plugins": map[string]any{
			"enabled":       false,
			"allowUnsigned": false,
		},
		"onlineMediaSources": map[string]any{
			"discover": false,
			"liveTv":   false,
			"podcasts": false,
		},
		"authorizedDevices": map[string]any{
			"requirePinForNewDevices": false,
			"allowDownloads":          true,
		},
		"watchlist": map[string]any{
			"syncAcrossUsers": false,
			"showOnHome":      true,
		},
		"webhooks": map[string]any{
			"enabled":  false,
			"endpoint": "",
		},
		"streamingServices": map[string]any{
			"showAvailability": false,
			"region":           "US",
		},
		"home": map[string]any{
			"showOnDeck":        true,
			"showRecentlyAdded": true,
			"showWatchlist":     true,
		},
		"libraryAccess": map[string]any{
			"allowSharing":    false,
			"requireApproval": true,
		},
		"privacy": map[string]any{
			"sendTelemetry":     false,
			"storeWatchHistory": true,
		},
		"web": map[string]any{
			"theme":               "system",
			"rememberFilters":     true,
			"autoPlayTrailers":    false,
			"showUnwatchedBadges": true,
			"compactRows":         false,
		},
	}
	for key, value := range defaultSettings {
		bytes, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return marshalErr
		}
		_, err = tx.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO NOTHING`, key, string(bytes), now.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("seed setting %s: %w", key, err)
		}
	}
	if err := seedUserLibraryAccess(tx, now); err != nil {
		return err
	}
	if err := seedLibraryNavigationPreferences(tx, now); err != nil {
		return err
	}
	return tx.Commit()
}

type localizationOptionSeed struct {
	Kind      string
	ID        string
	Label     string
	Labels    map[string]string
	SortOrder int
}

type ratingSystemSeed struct {
	Country   string
	System    string
	Label     string
	Labels    map[string]string
	SortOrder int
	Ratings   []ratingValueSeed
}

type ratingValueSeed struct {
	ID         string
	Label      string
	Labels     map[string]string
	Rank       int
	MinimumAge int
	SortOrder  int
}

func seedLocalizationCatalog(tx *sql.Tx, now time.Time) error {
	nowText := now.Format(time.RFC3339)
	for _, option := range localizationOptionSeeds() {
		labelsJSON, err := json.Marshal(option.Labels)
		if err != nil {
			return fmt.Errorf("marshal localization labels %s:%s: %w", option.Kind, option.ID, err)
		}
		if _, err := tx.Exec(`
			INSERT INTO localization_options (kind, id, label, labels_json, sort_order, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(kind, id) DO UPDATE SET
				label = excluded.label,
				labels_json = excluded.labels_json,
				sort_order = excluded.sort_order,
				updated_at = excluded.updated_at`,
			option.Kind, option.ID, option.Label, string(labelsJSON), option.SortOrder, nowText); err != nil {
			return fmt.Errorf("seed localization option %s:%s: %w", option.Kind, option.ID, err)
		}
	}
	for _, system := range ratingSystemSeeds() {
		labelsJSON, err := json.Marshal(system.Labels)
		if err != nil {
			return fmt.Errorf("marshal rating system labels %s:%s: %w", system.Country, system.System, err)
		}
		if _, err := tx.Exec(`
			INSERT INTO localization_rating_systems (country, system, label, labels_json, sort_order, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(country, system) DO UPDATE SET
				label = excluded.label,
				labels_json = excluded.labels_json,
				sort_order = excluded.sort_order,
				updated_at = excluded.updated_at`,
			system.Country, system.System, system.Label, string(labelsJSON), system.SortOrder, nowText); err != nil {
			return fmt.Errorf("seed rating system %s:%s: %w", system.Country, system.System, err)
		}
		for _, rating := range system.Ratings {
			ratingLabelsJSON, err := json.Marshal(rating.Labels)
			if err != nil {
				return fmt.Errorf("marshal rating labels %s:%s:%s: %w", system.Country, system.System, rating.ID, err)
			}
			if _, err := tx.Exec(`
				INSERT INTO localization_rating_values (
					country, system, rating, label, labels_json, rank, minimum_age, sort_order, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(country, system, rating) DO UPDATE SET
					label = excluded.label,
					labels_json = excluded.labels_json,
					rank = excluded.rank,
					minimum_age = excluded.minimum_age,
					sort_order = excluded.sort_order,
					updated_at = excluded.updated_at`,
				system.Country, system.System, rating.ID, rating.Label, string(ratingLabelsJSON), rating.Rank, rating.MinimumAge, rating.SortOrder, nowText); err != nil {
				return fmt.Errorf("seed rating value %s:%s:%s: %w", system.Country, system.System, rating.ID, err)
			}
		}
	}
	return nil
}

func nonEmptySeedSearchParts(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func localizationOptionSeeds() []localizationOptionSeed {
	return []localizationOptionSeed{
		{Kind: "locale", ID: "en-US", Label: "English (United States)", Labels: map[string]string{"en-US": "English (United States)", "fr-CA": "anglais (Etats-Unis)"}, SortOrder: 10},
		{Kind: "locale", ID: "en-CA", Label: "English (Canada)", Labels: map[string]string{"en-US": "English (Canada)", "fr-CA": "anglais (Canada)"}, SortOrder: 20},
		{Kind: "locale", ID: "en-GB", Label: "English (United Kingdom)", Labels: map[string]string{"en-US": "English (United Kingdom)", "fr-CA": "anglais (Royaume-Uni)"}, SortOrder: 30},
		{Kind: "locale", ID: "es-US", Label: "Spanish (United States)", Labels: map[string]string{"en-US": "Spanish (United States)", "fr-CA": "espagnol (Etats-Unis)"}, SortOrder: 40},
		{Kind: "locale", ID: "es-ES", Label: "Spanish (Spain)", Labels: map[string]string{"en-US": "Spanish (Spain)", "fr-CA": "espagnol (Espagne)"}, SortOrder: 50},
		{Kind: "locale", ID: "es-MX", Label: "Spanish (Mexico)", Labels: map[string]string{"en-US": "Spanish (Mexico)", "fr-CA": "espagnol (Mexique)"}, SortOrder: 60},
		{Kind: "locale", ID: "fr-CA", Label: "French (Canada)", Labels: map[string]string{"en-US": "French (Canada)", "fr-CA": "francais (Canada)"}, SortOrder: 70},
		{Kind: "locale", ID: "fr-FR", Label: "French (France)", Labels: map[string]string{"en-US": "French (France)", "fr-CA": "francais (France)"}, SortOrder: 80},
		{Kind: "locale", ID: "de-DE", Label: "German", Labels: map[string]string{"en-US": "German", "fr-CA": "allemand"}, SortOrder: 90},
		{Kind: "locale", ID: "it-IT", Label: "Italian", Labels: map[string]string{"en-US": "Italian", "fr-CA": "italien"}, SortOrder: 100},
		{Kind: "locale", ID: "pt-BR", Label: "Portuguese (Brazil)", Labels: map[string]string{"en-US": "Portuguese (Brazil)", "fr-CA": "portugais (Bresil)"}, SortOrder: 110},
		{Kind: "locale", ID: "pt-PT", Label: "Portuguese (Portugal)", Labels: map[string]string{"en-US": "Portuguese (Portugal)", "fr-CA": "portugais (Portugal)"}, SortOrder: 120},
		{Kind: "locale", ID: "nl-NL", Label: "Dutch", Labels: map[string]string{"en-US": "Dutch", "fr-CA": "neerlandais"}, SortOrder: 130},
		{Kind: "locale", ID: "pl-PL", Label: "Polish", Labels: map[string]string{"en-US": "Polish", "fr-CA": "polonais"}, SortOrder: 140},
		{Kind: "locale", ID: "sv-SE", Label: "Swedish", Labels: map[string]string{"en-US": "Swedish", "fr-CA": "suedois"}, SortOrder: 150},
		{Kind: "locale", ID: "nb-NO", Label: "Norwegian Bokmal", Labels: map[string]string{"en-US": "Norwegian Bokmal", "fr-CA": "norvegien bokmal"}, SortOrder: 160},
		{Kind: "locale", ID: "da-DK", Label: "Danish", Labels: map[string]string{"en-US": "Danish", "fr-CA": "danois"}, SortOrder: 170},
		{Kind: "locale", ID: "ja-JP", Label: "Japanese", Labels: map[string]string{"en-US": "Japanese", "fr-CA": "japonais"}, SortOrder: 180},
		{Kind: "language", ID: "en", Label: "English", Labels: map[string]string{"en-US": "English", "fr-CA": "anglais"}, SortOrder: 10},
		{Kind: "language", ID: "es", Label: "Spanish", Labels: map[string]string{"en-US": "Spanish", "fr-CA": "espagnol"}, SortOrder: 20},
		{Kind: "language", ID: "fr", Label: "French", Labels: map[string]string{"en-US": "French", "fr-CA": "francais"}, SortOrder: 30},
		{Kind: "language", ID: "de", Label: "German", Labels: map[string]string{"en-US": "German", "fr-CA": "allemand"}, SortOrder: 40},
		{Kind: "language", ID: "it", Label: "Italian", Labels: map[string]string{"en-US": "Italian", "fr-CA": "italien"}, SortOrder: 50},
		{Kind: "language", ID: "pt", Label: "Portuguese", Labels: map[string]string{"en-US": "Portuguese", "fr-CA": "portugais"}, SortOrder: 60},
		{Kind: "language", ID: "nl", Label: "Dutch", Labels: map[string]string{"en-US": "Dutch", "fr-CA": "neerlandais"}, SortOrder: 70},
		{Kind: "language", ID: "pl", Label: "Polish", Labels: map[string]string{"en-US": "Polish", "fr-CA": "polonais"}, SortOrder: 80},
		{Kind: "language", ID: "sv", Label: "Swedish", Labels: map[string]string{"en-US": "Swedish", "fr-CA": "suedois"}, SortOrder: 90},
		{Kind: "language", ID: "nb", Label: "Norwegian Bokmal", Labels: map[string]string{"en-US": "Norwegian Bokmal", "fr-CA": "norvegien bokmal"}, SortOrder: 100},
		{Kind: "language", ID: "da", Label: "Danish", Labels: map[string]string{"en-US": "Danish", "fr-CA": "danois"}, SortOrder: 110},
		{Kind: "language", ID: "ja", Label: "Japanese", Labels: map[string]string{"en-US": "Japanese", "fr-CA": "japonais"}, SortOrder: 120},
		{Kind: "country", ID: "CA", Label: "Canada", Labels: map[string]string{"en-US": "Canada", "fr-CA": "Canada"}, SortOrder: 10},
		{Kind: "country", ID: "FR", Label: "France", Labels: map[string]string{"en-US": "France", "fr-CA": "France"}, SortOrder: 20},
		{Kind: "country", ID: "DE", Label: "Germany", Labels: map[string]string{"en-US": "Germany", "fr-CA": "Allemagne"}, SortOrder: 30},
		{Kind: "country", ID: "IT", Label: "Italy", Labels: map[string]string{"en-US": "Italy", "fr-CA": "Italie"}, SortOrder: 40},
		{Kind: "country", ID: "JP", Label: "Japan", Labels: map[string]string{"en-US": "Japan", "fr-CA": "Japon"}, SortOrder: 50},
		{Kind: "country", ID: "PT", Label: "Portugal", Labels: map[string]string{"en-US": "Portugal", "fr-CA": "Portugal"}, SortOrder: 60},
		{Kind: "country", ID: "ES", Label: "Spain", Labels: map[string]string{"en-US": "Spain", "fr-CA": "Espagne"}, SortOrder: 70},
		{Kind: "country", ID: "GB", Label: "United Kingdom", Labels: map[string]string{"en-US": "United Kingdom", "fr-CA": "Royaume-Uni"}, SortOrder: 80},
		{Kind: "country", ID: "US", Label: "United States", Labels: map[string]string{"en-US": "United States", "fr-CA": "Etats-Unis"}, SortOrder: 90},
		{Kind: "country", ID: "MX", Label: "Mexico", Labels: map[string]string{"en-US": "Mexico", "fr-CA": "Mexique"}, SortOrder: 100},
		{Kind: "country", ID: "BR", Label: "Brazil", Labels: map[string]string{"en-US": "Brazil", "fr-CA": "Bresil"}, SortOrder: 110},
		{Kind: "country", ID: "NL", Label: "Netherlands", Labels: map[string]string{"en-US": "Netherlands", "fr-CA": "Pays-Bas"}, SortOrder: 120},
		{Kind: "country", ID: "PL", Label: "Poland", Labels: map[string]string{"en-US": "Poland", "fr-CA": "Pologne"}, SortOrder: 130},
		{Kind: "country", ID: "SE", Label: "Sweden", Labels: map[string]string{"en-US": "Sweden", "fr-CA": "Suede"}, SortOrder: 140},
		{Kind: "country", ID: "NO", Label: "Norway", Labels: map[string]string{"en-US": "Norway", "fr-CA": "Norvege"}, SortOrder: 150},
		{Kind: "country", ID: "DK", Label: "Denmark", Labels: map[string]string{"en-US": "Denmark", "fr-CA": "Danemark"}, SortOrder: 160},
		{Kind: "time_zone", ID: "UTC", Label: "UTC", Labels: map[string]string{"en-US": "UTC", "fr-CA": "UTC"}, SortOrder: 10},
		{Kind: "time_zone", ID: "America/Halifax", Label: "America/Halifax", Labels: map[string]string{"en-US": "America/Halifax", "fr-CA": "Amerique/Halifax"}, SortOrder: 20},
		{Kind: "time_zone", ID: "America/St_Johns", Label: "America/St_Johns", Labels: map[string]string{"en-US": "America/St_Johns", "fr-CA": "Amerique/St_Johns"}, SortOrder: 30},
		{Kind: "time_zone", ID: "America/Toronto", Label: "America/Toronto", Labels: map[string]string{"en-US": "America/Toronto", "fr-CA": "Amerique/Toronto"}, SortOrder: 40},
		{Kind: "time_zone", ID: "America/New_York", Label: "America/New_York", Labels: map[string]string{"en-US": "America/New_York", "fr-CA": "Amerique/New_York"}, SortOrder: 50},
		{Kind: "time_zone", ID: "America/Chicago", Label: "America/Chicago", Labels: map[string]string{"en-US": "America/Chicago", "fr-CA": "Amerique/Chicago"}, SortOrder: 60},
		{Kind: "time_zone", ID: "America/Denver", Label: "America/Denver", Labels: map[string]string{"en-US": "America/Denver", "fr-CA": "Amerique/Denver"}, SortOrder: 70},
		{Kind: "time_zone", ID: "America/Los_Angeles", Label: "America/Los_Angeles", Labels: map[string]string{"en-US": "America/Los_Angeles", "fr-CA": "Amerique/Los_Angeles"}, SortOrder: 80},
		{Kind: "time_zone", ID: "America/Phoenix", Label: "America/Phoenix", Labels: map[string]string{"en-US": "America/Phoenix", "fr-CA": "Amerique/Phoenix"}, SortOrder: 90},
		{Kind: "time_zone", ID: "America/Anchorage", Label: "America/Anchorage", Labels: map[string]string{"en-US": "America/Anchorage", "fr-CA": "Amerique/Anchorage"}, SortOrder: 100},
		{Kind: "time_zone", ID: "America/Mexico_City", Label: "America/Mexico_City", Labels: map[string]string{"en-US": "America/Mexico_City", "fr-CA": "Amerique/Mexico_City"}, SortOrder: 110},
		{Kind: "time_zone", ID: "America/Sao_Paulo", Label: "America/Sao_Paulo", Labels: map[string]string{"en-US": "America/Sao_Paulo", "fr-CA": "Amerique/Sao_Paulo"}, SortOrder: 120},
		{Kind: "time_zone", ID: "Pacific/Honolulu", Label: "Pacific/Honolulu", Labels: map[string]string{"en-US": "Pacific/Honolulu", "fr-CA": "Pacifique/Honolulu"}, SortOrder: 130},
		{Kind: "time_zone", ID: "Europe/London", Label: "Europe/London", Labels: map[string]string{"en-US": "Europe/London", "fr-CA": "Europe/London"}, SortOrder: 140},
		{Kind: "time_zone", ID: "Europe/Paris", Label: "Europe/Paris", Labels: map[string]string{"en-US": "Europe/Paris", "fr-CA": "Europe/Paris"}, SortOrder: 150},
		{Kind: "time_zone", ID: "Europe/Berlin", Label: "Europe/Berlin", Labels: map[string]string{"en-US": "Europe/Berlin", "fr-CA": "Europe/Berlin"}, SortOrder: 160},
		{Kind: "time_zone", ID: "Europe/Madrid", Label: "Europe/Madrid", Labels: map[string]string{"en-US": "Europe/Madrid", "fr-CA": "Europe/Madrid"}, SortOrder: 170},
		{Kind: "time_zone", ID: "Europe/Rome", Label: "Europe/Rome", Labels: map[string]string{"en-US": "Europe/Rome", "fr-CA": "Europe/Rome"}, SortOrder: 180},
		{Kind: "time_zone", ID: "Europe/Amsterdam", Label: "Europe/Amsterdam", Labels: map[string]string{"en-US": "Europe/Amsterdam", "fr-CA": "Europe/Amsterdam"}, SortOrder: 190},
		{Kind: "time_zone", ID: "Europe/Warsaw", Label: "Europe/Warsaw", Labels: map[string]string{"en-US": "Europe/Warsaw", "fr-CA": "Europe/Warsaw"}, SortOrder: 200},
		{Kind: "time_zone", ID: "Europe/Stockholm", Label: "Europe/Stockholm", Labels: map[string]string{"en-US": "Europe/Stockholm", "fr-CA": "Europe/Stockholm"}, SortOrder: 210},
		{Kind: "time_zone", ID: "Europe/Oslo", Label: "Europe/Oslo", Labels: map[string]string{"en-US": "Europe/Oslo", "fr-CA": "Europe/Oslo"}, SortOrder: 220},
		{Kind: "time_zone", ID: "Europe/Copenhagen", Label: "Europe/Copenhagen", Labels: map[string]string{"en-US": "Europe/Copenhagen", "fr-CA": "Europe/Copenhagen"}, SortOrder: 230},
		{Kind: "time_zone", ID: "Europe/Lisbon", Label: "Europe/Lisbon", Labels: map[string]string{"en-US": "Europe/Lisbon", "fr-CA": "Europe/Lisbon"}, SortOrder: 240},
		{Kind: "time_zone", ID: "Asia/Tokyo", Label: "Asia/Tokyo", Labels: map[string]string{"en-US": "Asia/Tokyo", "fr-CA": "Asie/Tokyo"}, SortOrder: 250},
		{Kind: "time_zone", ID: "Australia/Sydney", Label: "Australia/Sydney", Labels: map[string]string{"en-US": "Australia/Sydney", "fr-CA": "Australie/Sydney"}, SortOrder: 260},
	}
}

func ratingSystemSeeds() []ratingSystemSeed {
	return []ratingSystemSeed{
		{Country: "US", System: "MPA", Label: "Motion Picture Association", Labels: map[string]string{"en-US": "Motion Picture Association", "fr-CA": "Motion Picture Association"}, SortOrder: 10, Ratings: []ratingValueSeed{
			{ID: "G", Label: "G", Labels: map[string]string{"en-US": "G", "fr-CA": "G"}, Rank: 1, MinimumAge: 0, SortOrder: 10},
			{ID: "PG", Label: "PG", Labels: map[string]string{"en-US": "PG", "fr-CA": "PG"}, Rank: 3, MinimumAge: 0, SortOrder: 20},
			{ID: "PG-13", Label: "PG-13", Labels: map[string]string{"en-US": "PG-13", "fr-CA": "PG-13"}, Rank: 4, MinimumAge: 13, SortOrder: 30},
			{ID: "R", Label: "R", Labels: map[string]string{"en-US": "R", "fr-CA": "R"}, Rank: 5, MinimumAge: 17, SortOrder: 40},
			{ID: "NC-17", Label: "NC-17", Labels: map[string]string{"en-US": "NC-17", "fr-CA": "NC-17"}, Rank: 6, MinimumAge: 18, SortOrder: 50},
		}},
		{Country: "US", System: "US-TV", Label: "US TV Parental Guidelines", Labels: map[string]string{"en-US": "US TV Parental Guidelines", "fr-CA": "Classification tele americaine"}, SortOrder: 20, Ratings: []ratingValueSeed{
			{ID: "TV-Y", Label: "TV-Y", Labels: map[string]string{"en-US": "TV-Y", "fr-CA": "TV-Y"}, Rank: 1, MinimumAge: 0, SortOrder: 10},
			{ID: "TV-Y7", Label: "TV-Y7", Labels: map[string]string{"en-US": "TV-Y7", "fr-CA": "TV-Y7"}, Rank: 2, MinimumAge: 7, SortOrder: 20},
			{ID: "TV-G", Label: "TV-G", Labels: map[string]string{"en-US": "TV-G", "fr-CA": "TV-G"}, Rank: 1, MinimumAge: 0, SortOrder: 30},
			{ID: "TV-PG", Label: "TV-PG", Labels: map[string]string{"en-US": "TV-PG", "fr-CA": "TV-PG"}, Rank: 3, MinimumAge: 0, SortOrder: 40},
			{ID: "TV-14", Label: "TV-14", Labels: map[string]string{"en-US": "TV-14", "fr-CA": "TV-14"}, Rank: 4, MinimumAge: 14, SortOrder: 50},
			{ID: "TV-MA", Label: "TV-MA", Labels: map[string]string{"en-US": "TV-MA", "fr-CA": "TV-MA"}, Rank: 6, MinimumAge: 17, SortOrder: 60},
		}},
		{Country: "CA", System: "CHVRS", Label: "Canadian Home Video Rating System", Labels: map[string]string{"en-US": "Canadian Home Video Rating System", "fr-CA": "Systeme canadien de classification des videos"}, SortOrder: 30, Ratings: []ratingValueSeed{
			{ID: "G", Label: "G", Labels: map[string]string{"en-US": "G", "fr-CA": "G"}, Rank: 1, MinimumAge: 0, SortOrder: 10},
			{ID: "PG", Label: "PG", Labels: map[string]string{"en-US": "PG", "fr-CA": "PG"}, Rank: 3, MinimumAge: 0, SortOrder: 20},
			{ID: "14A", Label: "14A", Labels: map[string]string{"en-US": "14A", "fr-CA": "14A"}, Rank: 4, MinimumAge: 14, SortOrder: 30},
			{ID: "18A", Label: "18A", Labels: map[string]string{"en-US": "18A", "fr-CA": "18A"}, Rank: 5, MinimumAge: 18, SortOrder: 40},
			{ID: "R", Label: "R", Labels: map[string]string{"en-US": "R", "fr-CA": "R"}, Rank: 5, MinimumAge: 18, SortOrder: 50},
			{ID: "A", Label: "A", Labels: map[string]string{"en-US": "A", "fr-CA": "A"}, Rank: 6, MinimumAge: 18, SortOrder: 60},
		}},
		{Country: "GB", System: "BBFC", Label: "British Board of Film Classification", Labels: map[string]string{"en-US": "British Board of Film Classification", "fr-CA": "British Board of Film Classification"}, SortOrder: 40, Ratings: []ratingValueSeed{
			{ID: "U", Label: "U", Labels: map[string]string{"en-US": "U", "fr-CA": "U"}, Rank: 1, MinimumAge: 0, SortOrder: 10},
			{ID: "PG", Label: "PG", Labels: map[string]string{"en-US": "PG", "fr-CA": "PG"}, Rank: 3, MinimumAge: 0, SortOrder: 20},
			{ID: "12", Label: "12", Labels: map[string]string{"en-US": "12", "fr-CA": "12"}, Rank: 4, MinimumAge: 12, SortOrder: 30},
			{ID: "12A", Label: "12A", Labels: map[string]string{"en-US": "12A", "fr-CA": "12A"}, Rank: 4, MinimumAge: 12, SortOrder: 40},
			{ID: "15", Label: "15", Labels: map[string]string{"en-US": "15", "fr-CA": "15"}, Rank: 5, MinimumAge: 15, SortOrder: 50},
			{ID: "18", Label: "18", Labels: map[string]string{"en-US": "18", "fr-CA": "18"}, Rank: 6, MinimumAge: 18, SortOrder: 60},
		}},
	}
}

func seedUserLibraryAccess(tx *sql.Tx, now time.Time) error {
	if _, err := tx.Exec(`
		INSERT INTO user_library_access (user_id, library_id, created_at)
		SELECT users.id, libraries.id, ?
		FROM users
		CROSS JOIN libraries
		WHERE users.role = 'owner'
		ON CONFLICT(user_id, library_id) DO NOTHING
	`, now.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("seed admin library access: %w", err)
	}
	return nil
}

func seedLibraryNavigationPreferences(tx *sql.Tx, now time.Time) error {
	if _, err := tx.Exec(`
		INSERT INTO user_library_navigation (profile_id, user_id, library_id, pinned, sort_order, created_at, updated_at)
		SELECT users.id, users.id, libraries.id, 1, libraries.sort_order, ?, ?
		FROM users
		JOIN libraries
		  ON users.role = 'owner'
		  OR EXISTS (
			SELECT 1 FROM user_library_access access
			WHERE access.user_id = users.id AND access.library_id = libraries.id
		  )
		ON CONFLICT(profile_id, library_id) DO NOTHING
	`, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("seed library navigation preferences: %w", err)
	}
	return nil
}

func seededMedia(now time.Time, sourceURL string) []mediaSeed {
	daysAgo := func(days int) string {
		return now.Add(time.Duration(-days) * 24 * time.Hour).Format(time.RFC3339)
	}

	return []mediaSeed{
		{
			ID: "movie_meridian", LibraryID: "lib_movies", Type: "movie", Title: "The Meridian Job", SortTitle: "Meridian Job", Year: 2025, DurationSeconds: 7420,
			Summary: "A meticulous orbital salvage crew crosses paths with a fugitive cartographer and a corporate retrieval team racing for the same derelict station.",
			Tagline: "Every orbit has a price.", ContentRating: "PG-13", CommunityRating: 8.4, CriticRating: 91,
			Genres: []string{"Science Fiction", "Thriller", "Adventure"}, AddedAt: daysAgo(1), ArtSeed: "ember-space-cyan", SourceURL: sourceURL,
		},
		{
			ID: "movie_neon", LibraryID: "lib_movies", Type: "movie", Title: "Neon Harbor", SortTitle: "Neon Harbor", Year: 2023, DurationSeconds: 6380,
			Summary: "A private investigator follows a missing composer through a rain-soaked port city where every nightclub keeps a different version of the truth.",
			Tagline: "The city keeps its own tempo.", ContentRating: "R", CommunityRating: 7.7, CriticRating: 84,
			Genres: []string{"Mystery", "Crime", "Drama"}, AddedAt: daysAgo(3), ArtSeed: "magenta-noir-teal", SourceURL: sourceURL,
		},
		{
			ID: "movie_saffron", LibraryID: "lib_movies", Type: "movie", Title: "Saffron Road", SortTitle: "Saffron Road", Year: 2021, DurationSeconds: 7020,
			Summary: "Three siblings inherit a roadside restaurant and discover that their grandmother's recipe book is also a map of family debts and old promises.",
			Tagline: "Some recipes remember everything.", ContentRating: "PG", CommunityRating: 8.1, CriticRating: 88,
			Genres: []string{"Drama", "Comedy"}, AddedAt: daysAgo(7), ArtSeed: "saffron-red-cream", SourceURL: sourceURL,
		},
		{
			ID: "movie_pacific", LibraryID: "lib_movies", Type: "movie", Title: "Pacific Static", SortTitle: "Pacific Static", Year: 2024, DurationSeconds: 6120,
			Summary: "A late-night radio host receives impossible distress calls from fishing vessels that vanished decades earlier.",
			Tagline: "The signal is still alive.", ContentRating: "PG-13", CommunityRating: 7.9, CriticRating: 86,
			Genres: []string{"Horror", "Mystery"}, AddedAt: daysAgo(13), ArtSeed: "blue-static-gold", SourceURL: sourceURL,
		},
		{
			ID: "movie_clockwork", LibraryID: "lib_family_movies", Type: "movie", Title: "Clockwork Summer", SortTitle: "Clockwork Summer", Year: 2019, DurationSeconds: 5580,
			Summary: "A retired engineer builds a backyard observatory for his granddaughter and accidentally brings an entire town back into conversation.",
			Tagline: "A small town learns to look up.", ContentRating: "PG", CommunityRating: 7.3, CriticRating: 79,
			Genres: []string{"Family", "Drama"}, AddedAt: daysAgo(21), ArtSeed: "summer-copper-sky", SourceURL: sourceURL,
		},
		{
			ID: "movie_orchid", LibraryID: "lib_movies_4k", Type: "movie", Title: "Orchid Protocol", SortTitle: "Orchid Protocol", Year: 2026, DurationSeconds: 6840,
			Summary: "An exfiltration specialist is hired to recover a stolen climate model from a glass tower whose security system predicts human hesitation.",
			Tagline: "The cleanest escape is the one nobody sees.", ContentRating: "PG-13", CommunityRating: 8.3, CriticRating: 89,
			Genres: []string{"Action", "Thriller"}, AddedAt: daysAgo(5), ArtSeed: "orchid-steel-green", SourceURL: sourceURL,
		},
		{
			ID: "movie_blackwater", LibraryID: "lib_movies_4k", Type: "movie", Title: "Blackwater Sun", SortTitle: "Blackwater Sun", Year: 2022, DurationSeconds: 7320,
			Summary: "A survey team on a tidal planet discovers that the ocean records every transmission ever sent from Earth.",
			Tagline: "Some signals should stay lost.", ContentRating: "R", CommunityRating: 7.8, CriticRating: 85,
			Genres: []string{"Science Fiction", "Horror"}, AddedAt: daysAgo(15), ArtSeed: "blackwater-amber-blue", SourceURL: sourceURL,
		},
		{
			ID: "movie_lantern", LibraryID: "lib_family_movies", Type: "movie", Title: "Lantern League", SortTitle: "Lantern League", Year: 2020, DurationSeconds: 6040,
			Summary: "A group of neighborhood kids find an old projection booth that turns their hand-drawn maps into real places after sunset.",
			Tagline: "Draw the door. Light the way.", ContentRating: "PG", CommunityRating: 7.9, CriticRating: 83,
			Genres: []string{"Family", "Adventure", "Animation"}, AddedAt: daysAgo(9), ArtSeed: "lantern-indigo-gold", SourceURL: sourceURL,
		},
		{
			ID: "show_northbridge", LibraryID: "lib_tv", Type: "show", Title: "Northbridge", SortTitle: "Northbridge", Year: 2024, DurationSeconds: 0,
			Summary:       "A small university town becomes the center of a national investigation after a campus archivist uncovers a decades-old surveillance program.",
			ContentRating: "TV-MA", CommunityRating: 8.6, CriticRating: 93, Genres: []string{"Drama", "Thriller"}, AddedAt: daysAgo(2), ArtSeed: "forest-amber-ink",
		},
		{ID: "season_northbridge_1", LibraryID: "lib_tv", ParentID: "show_northbridge", Type: "season", Title: "Season 1", SortTitle: "Season 1", Year: 2024, AddedAt: daysAgo(2), SeasonNumber: 1, IndexNumber: 1, ArtSeed: "forest-amber-ink"},
		{
			ID: "episode_northbridge_101", LibraryID: "lib_tv", ParentID: "season_northbridge_1", Type: "episode", Title: "Cold Archive", SortTitle: "Cold Archive", Year: 2024, DurationSeconds: 3120,
			Summary:       "Mara finds a reel-to-reel tape with her father's voice on it. Detective Vale presses the university for records that do not officially exist.",
			ContentRating: "TV-MA", CommunityRating: 8.8, CriticRating: 94, Genres: []string{"Drama", "Thriller"}, AddedAt: daysAgo(2), SeasonNumber: 1, EpisodeNumber: 1, IndexNumber: 1, ArtSeed: "forest-amber-ink", SourceURL: sourceURL,
		},
		{
			ID: "episode_northbridge_102", LibraryID: "lib_tv", ParentID: "season_northbridge_1", Type: "episode", Title: "The Red Ledger", SortTitle: "Red Ledger", Year: 2024, DurationSeconds: 3180,
			Summary:       "A ledger hidden inside a donated piano leads Mara to an off-grid cabin and a name the faculty still refuses to say aloud.",
			ContentRating: "TV-MA", CommunityRating: 8.5, CriticRating: 91, Genres: []string{"Drama", "Thriller"}, AddedAt: daysAgo(2), SeasonNumber: 1, EpisodeNumber: 2, IndexNumber: 2, ArtSeed: "ledger-rust-green", SourceURL: sourceURL,
		},
		{
			ID: "episode_northbridge_103", LibraryID: "lib_tv", ParentID: "season_northbridge_1", Type: "episode", Title: "Blue Hour", SortTitle: "Blue Hour", Year: 2024, DurationSeconds: 3060,
			Summary:       "Vale traces the missing files to an old radio tower while Mara receives a warning from someone inside the chancellor's office.",
			ContentRating: "TV-MA", CommunityRating: 8.7, CriticRating: 92, Genres: []string{"Drama", "Thriller"}, AddedAt: daysAgo(2), SeasonNumber: 1, EpisodeNumber: 3, IndexNumber: 3, ArtSeed: "blue-hour-rust", SourceURL: sourceURL,
		},
		{
			ID: "show_signal", LibraryID: "lib_tv", Type: "show", Title: "Signal Fires", SortTitle: "Signal Fires", Year: 2022, DurationSeconds: 0,
			Summary:       "Volunteer firefighters in a coastal town balance ordinary emergencies with the uncanny weather pattern nobody can explain.",
			ContentRating: "TV-14", CommunityRating: 8.0, CriticRating: 82, Genres: []string{"Drama", "Mystery"}, AddedAt: daysAgo(12), ArtSeed: "coastal-fire-navy",
		},
		{ID: "season_signal_1", LibraryID: "lib_tv", ParentID: "show_signal", Type: "season", Title: "Season 1", SortTitle: "Season 1", Year: 2022, AddedAt: daysAgo(12), SeasonNumber: 1, IndexNumber: 1, ArtSeed: "coastal-fire-navy"},
		{
			ID: "episode_signal_101", LibraryID: "lib_tv", ParentID: "season_signal_1", Type: "episode", Title: "Dry Lightning", SortTitle: "Dry Lightning", Year: 2022, DurationSeconds: 2840,
			Summary:       "The crew answers three calls in one night and notices every fire began under a cloudless sky.",
			ContentRating: "TV-14", CommunityRating: 7.9, CriticRating: 80, Genres: []string{"Drama", "Mystery"}, AddedAt: daysAgo(12), SeasonNumber: 1, EpisodeNumber: 1, IndexNumber: 1, ArtSeed: "coastal-fire-navy", SourceURL: sourceURL,
		},
		{
			ID: "show_valley", LibraryID: "lib_tv_classics", Type: "show", Title: "Valley House", SortTitle: "Valley House", Year: 1998, DurationSeconds: 0,
			Summary:       "A character-driven family mystery about an inherited house, a vanished architect, and a town that keeps changing its street names.",
			ContentRating: "TV-PG", CommunityRating: 7.6, CriticRating: 78, Genres: []string{"Drama", "Mystery"}, AddedAt: daysAgo(28), ArtSeed: "valley-house-green",
		},
		{ID: "season_valley_1", LibraryID: "lib_tv_classics", ParentID: "show_valley", Type: "season", Title: "Season 1", SortTitle: "Season 1", Year: 1998, AddedAt: daysAgo(28), SeasonNumber: 1, IndexNumber: 1, ArtSeed: "valley-house-green"},
		{
			ID: "episode_valley_101", LibraryID: "lib_tv_classics", ParentID: "season_valley_1", Type: "episode", Title: "The Locked Solarium", SortTitle: "Locked Solarium", Year: 1998, DurationSeconds: 2640,
			Summary:       "The family returns for a funeral and discovers a sealed room that appears in no blueprint.",
			ContentRating: "TV-PG", CommunityRating: 7.7, CriticRating: 79, Genres: []string{"Drama", "Mystery"}, AddedAt: daysAgo(28), SeasonNumber: 1, EpisodeNumber: 1, IndexNumber: 1, ArtSeed: "valley-house-green", SourceURL: sourceURL,
		},
		{
			ID: "anime_starrail", LibraryID: "lib_anime", Type: "anime", Title: "Starrail Lullaby", SortTitle: "Starrail Lullaby", Year: 2025, DurationSeconds: 0,
			Summary:       "A courier guild ferries dreams between moon cities while a runaway conductor tries to stop the night train from reaching its final station.",
			ContentRating: "TV-14", CommunityRating: 8.9, CriticRating: 95, Genres: []string{"Anime", "Fantasy", "Adventure"}, AddedAt: daysAgo(4), ArtSeed: "violet-rail-gold",
		},
		{ID: "season_starrail_1", LibraryID: "lib_anime", ParentID: "anime_starrail", Type: "season", Title: "Season 1", SortTitle: "Season 1", Year: 2025, AddedAt: daysAgo(4), SeasonNumber: 1, IndexNumber: 1, ArtSeed: "violet-rail-gold"},
		{
			ID: "anime_starrail_101", LibraryID: "lib_anime", ParentID: "season_starrail_1", Type: "episode", Title: "Moon Platform Seven", SortTitle: "Moon Platform Seven", Year: 2025, DurationSeconds: 1460,
			Summary:       "Nia accepts a forbidden parcel and boards a train whose route does not appear on any map.",
			ContentRating: "TV-14", CommunityRating: 9.1, CriticRating: 96, Genres: []string{"Anime", "Fantasy"}, AddedAt: daysAgo(4), SeasonNumber: 1, EpisodeNumber: 1, IndexNumber: 1, ArtSeed: "violet-rail-gold", SourceURL: sourceURL,
		},
		{
			ID: "anime_embercode", LibraryID: "lib_anime", Type: "anime", Title: "Embercode", SortTitle: "Embercode", Year: 2023, DurationSeconds: 0,
			Summary:       "Apprentice mages debug spell engines in a city powered by contracts written in fire.",
			ContentRating: "TV-PG", CommunityRating: 8.2, CriticRating: 87, Genres: []string{"Anime", "Action", "Comedy"}, AddedAt: daysAgo(18), ArtSeed: "ember-code-black",
		},
		{
			ID: "artist_mara", LibraryID: "lib_music", Type: "artist", Title: "Mara Vale", SortTitle: "Mara Vale", Year: 2024, DurationSeconds: 0,
			Summary:         "Electronic composer and producer building nocturnal records from station ambience and piano fragments.",
			CommunityRating: 8.2, CriticRating: 89, Studio: "Mara Vale", Genres: []string{"Electronic", "Ambient"}, AddedAt: daysAgo(5), ArtSeed: "album-electric-indigo",
		},
		{
			ID: "album_mara", LibraryID: "lib_music", Type: "album", Title: "Late Trains for Bright Cities", SortTitle: "Late Trains for Bright Cities", Year: 2024, DurationSeconds: 2860,
			Summary:         "A nocturnal electronic album built from station ambience, modular synths, and close-mic piano fragments.",
			CommunityRating: 8.2, CriticRating: 89, Studio: "Mara Vale", Genres: []string{"Electronic", "Ambient"}, AddedAt: daysAgo(5), ArtSeed: "album-electric-indigo", SourceURL: sourceURL, ParentID: "artist_mara",
		},
		{
			ID: "track_mara_01", LibraryID: "lib_music", ParentID: "album_mara", Type: "track", Title: "Platform Lights", SortTitle: "Platform Lights", Year: 2024, DurationSeconds: 244,
			Summary: "Opening track from Late Trains for Bright Cities.", CommunityRating: 8.0, Studio: "Mara Vale", Genres: []string{"Electronic"}, AddedAt: daysAgo(5), IndexNumber: 1, ArtSeed: "album-electric-indigo", SourceURL: sourceURL,
		},
		{
			ID: "artist_roen", LibraryID: "lib_music", Type: "artist", Title: "Roen Ash", SortTitle: "Roen Ash", Year: 2021, DurationSeconds: 0,
			Summary:         "Folk songwriter known for warm acoustic arrangements and lakeside chapel recordings.",
			CommunityRating: 7.8, CriticRating: 84, Studio: "Roen Ash", Genres: []string{"Folk", "Singer-Songwriter"}, AddedAt: daysAgo(17), ArtSeed: "album-lantern-green",
		},
		{
			ID: "album_roen", LibraryID: "lib_music", Type: "album", Title: "Paper Lantern Weather", SortTitle: "Paper Lantern Weather", Year: 2021, DurationSeconds: 3340,
			Summary:         "Warm acoustic arrangements recorded in a lakeside chapel during a week of summer storms.",
			CommunityRating: 7.8, CriticRating: 84, Studio: "Roen Ash", Genres: []string{"Folk", "Singer-Songwriter"}, AddedAt: daysAgo(17), ArtSeed: "album-lantern-green", SourceURL: sourceURL, ParentID: "artist_roen",
		},
		{
			ID: "book_atlas", LibraryID: "lib_audiobooks", Type: "audiobook", Title: "The Atlas of Borrowed Doors", SortTitle: "Atlas of Borrowed Doors", Year: 2020, DurationSeconds: 36240,
			Summary:         "A locksmith discovers that certain doors remember everyone who has ever crossed their thresholds.",
			CommunityRating: 8.5, CriticRating: 90, Genres: []string{"Fantasy", "Mystery"}, AddedAt: daysAgo(6), ArtSeed: "book-atlas-green-gold", SourceURL: sourceURL,
		},
		{
			ID: "book_weather", LibraryID: "lib_audiobooks", Type: "audiobook", Title: "Weather for Astronauts", SortTitle: "Weather for Astronauts", Year: 2018, DurationSeconds: 28800,
			Summary:         "An essay collection about isolation, exploration, family, and the strange poetry of technical manuals.",
			CommunityRating: 7.6, CriticRating: 81, Genres: []string{"Essays", "Science"}, AddedAt: daysAgo(24), ArtSeed: "book-weather-blue", SourceURL: sourceURL,
		},
	}
}

func HasNoUsers(db *sql.DB) (bool, error) {
	return HasNoUsersContext(context.Background(), db)
}

func HasNoUsersContext(ctx context.Context, db *sql.DB) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	return count == 0, nil
}
