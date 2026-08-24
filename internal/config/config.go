package config

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

const (
	// These are Portico's intentionally distributed TMDB application
	// credentials. They identify the shipped client and are not treated as a
	// secret boundary. Owners may override them.
	DefaultTMDBReadAccessToken = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiI2OGE5Y2M5ZGNhNjJkMDg2ZWEzZWQwNTg1YmMwZDcyYyIsIm5iZiI6MTc3NzU4Njc3MS44MzIsInN1YiI6IjY5ZjNkMjUzMTA2ODk0N2I0NmZiZDUwZiIsInNjb3BlcyI6WyJhcGlfcmVhZCJdLCJ2ZXJzaW9uIjoxfQ.AmRWVmPw6MJOYp5jv83nvOYJUWXRx1YsoEtUDDQht_8" // gitleaks:allow -- intentionally distributed TMDB application credential
	DefaultTMDBAPIKey          = "68a9cc9dca62d086ea3ed0585bc0d72c"                                                                                                                                                                                                                // gitleaks:allow -- intentionally distributed TMDB application credential
	// DefaultTVDBAPIKey is Portico's intentionally distributed TheTVDB v4
	// project key. It identifies the shipped application, not an owner or a
	// server installation, and is therefore not a secret boundary.
	DefaultTVDBAPIKey = "a38235b5-2200-4875-9128-08e0c662c54d" // gitleaks:allow -- intentionally distributed TheTVDB application credential
)

type Config struct {
	// Environment and HostedAPIAuthority are the runtime identity used to
	// constrain every credential-bearing Hosted request. Protected profiles
	// must provide an explicit authority; development and test use the
	// generated Foundation authority unless a custom authority is explicitly
	// configured.
	Environment        string
	HostedAPIAuthority string
	// Release is injected by the host build/runtime and is recorded only as
	// backup provenance. Restore compatibility is decided by the database
	// format, reviewed migration prefix, minimum reader, and reconciliation
	// state—not by release-string equality.
	Release        string
	BuildNumber    string
	BuildChannel   string
	BuildCommit    string
	BuildTimestamp string
	Addr           string
	AppDataDir     string
	ConfigPath     string
	DatabasePath   string
	BackupDir      string
	// RestoreMaxDatabaseBytes bounds one uploaded/imported database. Zero means
	// the database package's reviewed production default.
	RestoreMaxDatabaseBytes  int64
	RestoreSafetyCopyTimeout time.Duration
	RestoreIOTimeout         time.Duration
	WebDistDir               string
	LogFilePath              string
	CookieSecure             bool
	SampleMediaURL           string
	PublicOrigin             string
	AllowedOrigins           []string
	CastReceiverOrigins      []string
	TrustedProxyCIDRs        []netip.Prefix
	TMDBReadAccessToken      string
	TMDBAPIKey               string
	TMDBBaseURL              string
	TVDBAPIKey               string
	TVDBBaseURL              string
	AniListBaseURL           string
	MusicBrainzBaseURL       string
	CoverArtArchiveBaseURL   string
	AcoustIDAPIKey           string
	AcoustIDBaseURL          string
	LRCLibBaseURL            string
	FFmpegPath               string
	FFprobePath              string
	FPcalcPath               string
	TranscodeDir             string
	HostedDocumentPublicKeys map[string]string
}

func Load() Config {
	appData := getenv("PORTICO_APP_DATA", "var")
	environment := strings.TrimSpace(getenv("PORTICO_ENVIRONMENT", foundationcontract.DefaultEnvironment))
	hostedAuthority := strings.TrimSpace(os.Getenv("PORTICO_HOSTED_API_AUTHORITY"))
	if hostedAuthority == "" {
		hostedAuthority = strings.TrimSpace(foundationcontract.HostedAPIAuthorityByEnvironment[environment])
	}
	configPath := getenv("PORTICO_CONFIG_FILE", filepath.Join(appData, "portico.config.json"))
	runtimePaths, _ := LoadRuntimePaths(configPath)
	port := configuredServicePort(getenv("PORTICO_PORT", "32500"))
	addr := getenv("PORTICO_ADDR", "0.0.0.0:"+strconv.Itoa(port))
	secureCookie, _ := strconv.ParseBool(getenv("PORTICO_COOKIE_SECURE", "false"))
	databasePath := firstConfiguredPath(getenv("PORTICO_DATABASE_PATH", ""), runtimePaths.DatabasePath, filepath.Join(appData, "portico.db"))
	backupDir := firstConfiguredPath(getenv("PORTICO_BACKUP_DIR", ""), runtimePaths.BackupDirectory, filepath.Join(appData, "backups"))
	restoreMaxBytes := configuredRestoreMaxDatabaseBytes(getenv("PORTICO_RESTORE_MAX_DATABASE_BYTES", ""))
	restoreSafetyTimeout := configuredRestoreDuration(getenv("PORTICO_RESTORE_SAFETY_COPY_TIMEOUT", ""), 30*time.Minute, time.Minute, 24*time.Hour)
	restoreIOTimeout := configuredRestoreDuration(getenv("PORTICO_RESTORE_IO_TIMEOUT", ""), 10*time.Minute, time.Minute, 24*time.Hour)
	tmdbReadAccessToken, tmdbAPIKey := configuredTMDBCredentials()

	ffmpegPath := resolveConfiguredPath(getenv("PORTICO_FFMPEG_PATH", defaultBundledToolPath("ffmpeg", "ffmpeg")))
	ffprobePath := resolveConfiguredPath(getenv("PORTICO_FFPROBE_PATH", defaultBundledToolPath("ffprobe", "ffprobe")))
	fpcalcPath := resolveConfiguredPath(getenv("PORTICO_FPCALC_PATH", "fpcalc"))

	return Config{
		Environment:              environment,
		HostedAPIAuthority:       hostedAuthority,
		Release:                  getenv("PORTICO_RELEASE", "dev"),
		Addr:                     addr,
		AppDataDir:               appData,
		ConfigPath:               configPath,
		DatabasePath:             databasePath,
		BackupDir:                backupDir,
		RestoreMaxDatabaseBytes:  restoreMaxBytes,
		RestoreSafetyCopyTimeout: restoreSafetyTimeout,
		RestoreIOTimeout:         restoreIOTimeout,
		WebDistDir:               getenv("PORTICO_WEB_DIST", filepath.Join("web", "dist")),
		LogFilePath:              getenv("PORTICO_LOG_FILE", ""),
		CookieSecure:             secureCookie,
		SampleMediaURL:           getenv("PORTICO_SAMPLE_MEDIA_URL", ""),
		PublicOrigin:             strings.TrimRight(strings.TrimSpace(getenv("PORTICO_PUBLIC_ORIGIN", "")), "/"),
		AllowedOrigins:           defaultAllowedOrigins(splitCSV(getenv("PORTICO_ALLOWED_ORIGINS", ""))),
		CastReceiverOrigins:      castReceiverOrigins(getenv("PORTICO_CAST_RECEIVER_ORIGINS", "")),
		TrustedProxyCIDRs:        trustedProxyCIDRsFromEnv(getenv("PORTICO_TRUSTED_PROXY_CIDRS", "")),
		TMDBReadAccessToken:      tmdbReadAccessToken,
		TMDBAPIKey:               tmdbAPIKey,
		TMDBBaseURL:              strings.TrimRight(getenv("PORTICO_TMDB_BASE_URL", "https://api.themoviedb.org/3"), "/"),
		TVDBAPIKey:               strings.TrimSpace(getenv("PORTICO_TVDB_API_KEY", DefaultTVDBAPIKey)),
		TVDBBaseURL:              strings.TrimRight(getenv("PORTICO_TVDB_BASE_URL", "https://api4.thetvdb.com/v4"), "/"),
		AniListBaseURL:           strings.TrimRight(getenv("PORTICO_ANILIST_BASE_URL", "https://graphql.anilist.co"), "/"),
		MusicBrainzBaseURL:       strings.TrimRight(getenv("PORTICO_MUSICBRAINZ_BASE_URL", "https://musicbrainz.org/ws/2"), "/"),
		CoverArtArchiveBaseURL:   strings.TrimRight(getenv("PORTICO_COVER_ART_ARCHIVE_BASE_URL", "https://coverartarchive.org"), "/"),
		AcoustIDAPIKey:           getenv("PORTICO_ACOUSTID_API_KEY", ""),
		AcoustIDBaseURL:          strings.TrimRight(getenv("PORTICO_ACOUSTID_BASE_URL", "https://api.acoustid.org/v2"), "/"),
		LRCLibBaseURL:            strings.TrimRight(getenv("PORTICO_LRCLIB_BASE_URL", "https://lrclib.net"), "/"),
		FFmpegPath:               ffmpegPath,
		FFprobePath:              ffprobePath,
		FPcalcPath:               fpcalcPath,
		TranscodeDir:             getenv("PORTICO_TRANSCODE_DIR", filepath.Join(appData, "transcodes")),
		HostedDocumentPublicKeys: parseStringMap(getenv("PORTICO_HOSTED_DOCUMENT_PUBLIC_KEYS_JSON", "")),
	}
}

func configuredTMDBCredentials() (string, string) {
	readAccessToken := strings.TrimSpace(os.Getenv("PORTICO_TMDB_READ_ACCESS_TOKEN"))
	apiKey := strings.TrimSpace(os.Getenv("PORTICO_TMDB_API_KEY"))
	if readAccessToken != "" {
		return readAccessToken, apiKey
	}
	// An explicit API-key-only configuration must not be shadowed by a bearer
	// token, because TMDB requests prefer bearer auth.
	if apiKey != "" {
		return "", apiKey
	}
	return DefaultTMDBReadAccessToken, DefaultTMDBAPIKey
}

func configuredRestoreDuration(value string, fallback, minimum, maximum time.Duration) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed < minimum || parsed > maximum {
		return fallback
	}
	return parsed
}

func configuredRestoreMaxDatabaseBytes(value string) int64 {
	const (
		defaultBytes = int64(16 << 30)
		minimumBytes = int64(1 << 20)
		maximumBytes = int64(64 << 30)
	)
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed == 0 {
		return defaultBytes
	}
	if parsed < minimumBytes || parsed > maximumBytes {
		return defaultBytes
	}
	return parsed
}

func castReceiverOrigins(raw string) []string {
	origins := splitCSV(raw)
	if len(origins) == 0 {
		return []string{"https://cast.getportico.tv"}
	}
	return origins
}

func configuredServicePort(value string) int {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return 32500
	}
	return port
}

func parseStringMap(raw string) map[string]string {
	values := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return values
	}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return map[string]string{}
	}
	for key, value := range values {
		normalizedKey := strings.TrimSpace(key)
		normalizedValue := strings.TrimSpace(value)
		delete(values, key)
		if normalizedKey != "" && normalizedValue != "" {
			values[normalizedKey] = normalizedValue
		}
	}
	return values
}

type RuntimePaths struct {
	DatabasePath    string `json:"databasePath,omitempty"`
	BackupDirectory string `json:"backupDirectory,omitempty"`
}

func LoadRuntimePaths(path string) (RuntimePaths, error) {
	var paths RuntimePaths
	if strings.TrimSpace(path) == "" {
		return paths, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return paths, nil
		}
		return paths, err
	}
	// The private-path bootstrap creates lifecycle files before SQLite opens so
	// they cannot inherit unsafe permissions. A fresh install therefore has an
	// empty config file until the owner changes a runtime path; that file has
	// the same meaning as an absent config.
	if len(strings.TrimSpace(string(body))) == 0 {
		return paths, nil
	}
	if err := json.Unmarshal(body, &paths); err != nil {
		return paths, err
	}
	paths.DatabasePath = strings.TrimSpace(paths.DatabasePath)
	paths.BackupDirectory = strings.TrimSpace(paths.BackupDirectory)
	return paths, nil
}

func SaveRuntimePaths(path string, paths RuntimePaths) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("runtime config path is not configured")
	}
	paths.DatabasePath = strings.TrimSpace(paths.DatabasePath)
	paths.BackupDirectory = strings.TrimSpace(paths.BackupDirectory)
	body, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".portico.config-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	defer cleanup()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	if err := syncRuntimePathsDirectory(directory); err != nil {
		return err
	}
	return nil
}

func trustedProxyCIDRsFromEnv(value string) []netip.Prefix {
	prefixes, err := parseTrustedProxyCIDRs(value)
	if err != nil {
		return []netip.Prefix{}
	}
	return prefixes
}

func parseTrustedProxyCIDRs(value string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if addr, err := netip.ParseAddr(part); err == nil {
			prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, fmt.Errorf("%s is not a valid proxy CIDR or address", part)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func resolveConfiguredPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) || !strings.ContainsAny(value, `/\`) {
		return value
	}
	if abs, err := filepath.Abs(value); err == nil {
		return abs
	}
	return value
}

func defaultBundledToolPath(tool string, fallback string) string {
	name := tool
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name += ".exe"
	}
	for _, root := range bundledToolRoots() {
		path := filepath.Join(root, "third_party", "ffmpeg", "bin", name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return fallback
}

func bundledToolRoots() []string {
	roots := []string{}
	if executable, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(executable)
		roots = append(roots, exeDir)
		if filepath.Base(exeDir) == "dist" {
			roots = append(roots, filepath.Dir(exeDir))
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	out := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		clean := filepath.Clean(root)
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	return out
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func firstConfiguredPath(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func defaultAllowedOrigins(configured []string) []string {
	if len(configured) > 0 {
		return configured
	}
	return []string{"https://web.getportico.tv", "https://beta-web.getportico.tv"}
}
