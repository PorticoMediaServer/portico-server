package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

type libraryScanResult struct {
	FilesIndexed          int
	FilesUnchanged        int
	FilesSkipped          int
	StaleRemoved          int
	MissingMarked         int
	MetadataRefreshQueued int
	AnalysisQueued        int
	DegradedRoots         int
	AbsenceAuthoritative  bool
	CleanupAllowed        bool
}

type scannerPathCandidate struct {
	Path           string
	DirectoryPath  string
	Size           int64
	ModTime        string
	QuickSignature string
}

type scannerDirectoryCheckpoint struct {
	Path           string
	Signature      string
	MediaFileCount int
}

type libraryRuntimeSettings struct {
	ScanOnFilesystemChanges bool
	ScanAutomatically       bool
	AnalysisTier            string
	EmptyTrashAfterScan     bool
	AllowMediaDeletion      bool
	TrashRetentionDays      int
}

type scannerMediaFile struct {
	ID                    string
	FileID                string
	LibraryID             string
	ParentID              string
	ParentScannerKey      string
	ParentTitle           string
	GrandparentID         string
	GrandparentScannerKey string
	GrandparentTitle      string
	Type                  string
	Title                 string
	SortTitle             string
	Artist                string
	SeasonNumber          int
	EpisodeNumber         int
	EpisodeEnd            int
	IndexNumber           int
	SourcePath            string
	ScanRoot              string
	DisplayPath           string
	SourceType            string
	FileSize              int64
	FileModTime           string
	ContentFingerprint    string
	QuickSignature        string
	Version               scannerMediaVersion
	ArtSeed               string
	ExtraKind             string
	LocalMetadata         scannerLocalMetadata
	LocalNFOEnabled       bool
	ReadExternalSidecars  bool
	DiscoverLocalArtwork  bool
	ParentShowMetadata    scannerLocalMetadata
	ParentSeasonMetadata  scannerLocalMetadata
	ParentArtistMetadata  scannerLocalMetadata
	ParentAlbumMetadata   scannerLocalMetadata
	PreparedSubtitles     []scannerPreparedSubtitle
	PreparedLyrics        []scannerPreparedLyric
	ExistingLocalImages   map[string]bool
	FilesystemPrepared    bool
}

type scannerPreparedSubtitle struct {
	CandidatePath string
	NormalizedVTT []byte
	Language      string
	Forced        bool
}

type scannerPreparedLyric struct {
	CandidatePath string
	Text          string
	Format        string
	Language      string
	Synced        bool
}

type scannerSubtitlePublication struct {
	File           scannerMediaFile
	StreamID       string
	StreamIdentity string
	StoragePath    string
	SourceURL      string
	Language       string
	DisplayTitle   string
	NormalizedVTT  []byte
}

type scannerSubtitleReplacement struct {
	File         scannerMediaFile
	Publications []scannerSubtitlePublication
}

type scannerCatalogApplyError struct{ err error }

func (err scannerCatalogApplyError) Error() string { return err.err.Error() }
func (err scannerCatalogApplyError) Unwrap() error { return err.err }

type scannerLocalMetadata struct {
	Title           string
	LocalTitle      string
	SortTitle       string
	OriginalTitle   string
	ShowTitle       string
	Edition         string
	Year            int
	ExactDate       string
	SeasonNumber    int
	EpisodeNumber   int
	RuntimeMinutes  int
	Collection      string
	RatingName      string
	RatingVotes     int
	Summary         string
	Tagline         string
	ContentRating   string
	CommunityRating float64
	CriticRating    int
	Studio          string
	Studios         []string
	Network         string
	Country         string
	Countries       []string
	Genres          []string
	Tags            []string
	Artist          string
	AlbumArtist     string
	AlbumTitle      string
	Series          string
	SeriesIndex     string
	AuthorProvider  string
	AuthorID        string
	SeriesProvider  string
	SeriesID        string
	Label           string
	Publisher       string
	ReleaseCountry  string
	TrackNumber     int
	TrackCount      int
	DiscNumber      int
	DiscCount       int
	BPM             int
	Explicit        string
	ProviderIDs     map[string]string
	TypedMetadata   map[string]string
	ImagePaths      map[string]string
	People          []MediaPerson
	Source          string
}

var (
	videoExtensions = map[string]bool{
		".3g2": true, ".3gp": true, ".asf": true, ".avi": true, ".divx": true, ".dv": true, ".f4v": true, ".flv": true, ".m2ts": true, ".m2v": true, ".m4v": true, ".mkv": true, ".mov": true, ".mp4": true, ".mpeg": true, ".mpg": true, ".mts": true, ".mxf": true, ".ogm": true, ".rm": true, ".rmvb": true, ".ts": true, ".vob": true, ".webm": true, ".wmv": true, ".wtv": true,
	}
	strmExtensions  = map[string]bool{".strm": true}
	audioExtensions = map[string]bool{
		".aac": true, ".aiff": true, ".alac": true, ".ape": true, ".caf": true, ".dff": true, ".dsf": true, ".flac": true, ".m4a": true, ".mka": true, ".mp3": true, ".mpc": true, ".oga": true, ".ogg": true, ".opus": true, ".ra": true, ".tta": true, ".wav": true, ".wma": true, ".wv": true,
	}
	audiobookExtensions = map[string]bool{
		".aax": true, ".m4b": true,
	}
	sidecarSubtitleExtensions = map[string]bool{
		".ass": true, ".dfxp": true, ".sbv": true, ".srt": true, ".ssa": true, ".sub": true, ".ttml": true, ".vtt": true,
	}
	sidecarLyricExtensions = map[string]bool{
		".lrc": true, ".txt": true,
	}
)

const (
	scannerWriteBatchSize        = 50
	scannerReconcileBatchSize    = 500
	scannerCheckpointCommitBatch = 32
	scannerChangeWatchInterval   = 30 * time.Second
)

const (
	scannerSubtitleSidecarLimit int64 = 8 << 20
	scannerLyricsSidecarLimit   int64 = 512 << 10
	scannerLocalMetadataLimit   int64 = 4 << 20
)

var errScannerSidecarTooLarge = errors.New("scanner sidecar exceeds the supported size limit")

var errLibraryFingerprintBudgetExceeded = errors.New("library fingerprint media file budget exceeded")

var scannerFingerprintMediaFileLimit = 25000
var scannerWalkDir = filepath.WalkDir
var scannerResolveMediaPath = func(s *Server, ctx context.Context, request storageIORequest, path string) (string, error) {
	return s.storageEvalSymlinks(ctx, request, path)
}
var scannerStatMediaPath = func(s *Server, ctx context.Context, request storageIORequest, path string) (os.FileInfo, error) {
	return s.storageStat(ctx, request, path)
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func (s *Server) scanLibrary(ctx context.Context, library Library, jobID string) {
	if err := ctx.Err(); err != nil {
		s.recordLog("info", fmt.Sprintf("Scan cancelled for %s.", library.Name), map[string]string{"job": jobID, "library": library.ID})
		return
	}
	if err := s.setJobProgress(jobID, "running", 8, "Validating library folders."); err != nil {
		s.log.Warn("scan job update failed", "job", jobID, "error", err)
	}
	mode := "reconcile"
	jobMetadata := map[string]string{}
	if jobID != "" {
		if job, jobErr := s.getJob(jobID); jobErr == nil {
			mode = normalizeScanMode(job.Metadata["mode"])
			jobMetadata = job.Metadata
		}
	}
	if strings.EqualFold(jobMetadata[scanTriggerMetadataKey], "profile-change") {
		_ = s.setJobProgress(jobID, "running", 18, "Reconciling newly selected scan-profile stages from committed inventory.")
		indexed, analysisQueued, err := s.reconcileScanProfileStages(ctx, library, jobMetadata)
		if err != nil {
			if s.retryJobLater(jobID, err) {
				return
			}
			message := fmt.Sprintf("Scan-profile reconciliation failed for %s: %s", library.Name, err.Error())
			_ = s.setJobMessage(jobID, "failed", 100, message)
			return
		}
		message := fmt.Sprintf("Scan-profile reconciliation completed for %s. Re-enriched %d indexed files and queued %d current-revision analysis stages.", library.Name, indexed, analysisQueued)
		if _, _, err := s.completeLibraryScanOrContinue(jobID, mode, message); err != nil {
			_ = s.setJobMessage(jobID, "failed", 100, "Scan-profile reconciliation could not commit completion.")
			return
		}
		s.invalidateHomeCache()
		s.publishLibraryScanCompleted(library.ID)
		return
	}
	for {
		result, err := s.performLibraryScanWithMode(ctx, library, jobID, mode)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				s.recordLog("info", fmt.Sprintf("Scan cancelled for %s.", library.Name), map[string]string{"job": jobID, "library": library.ID})
				return
			}
			if s.retryJobLater(jobID, err) {
				return
			}
			message := fmt.Sprintf("Scan failed for %s: %s", library.Name, err.Error())
			_ = s.setJobMessage(jobID, "failed", 100, message)
			s.recordLog("error", message, map[string]string{"job": jobID, "library": library.ID})
			return
		}
		if err := ctx.Err(); err != nil {
			s.recordLog("info", fmt.Sprintf("Scan cancelled for %s.", library.Name), map[string]string{"job": jobID, "library": library.ID})
			return
		}
		metadataRefreshSummary := "no metadata refresh queued"
		if result.MetadataRefreshQueued > 0 {
			metadataRefreshSummary = fmt.Sprintf("queued metadata refresh for %d item%s", result.MetadataRefreshQueued, pluralSuffix(result.MetadataRefreshQueued))
		}
		message := fmt.Sprintf("Scan completed for %s. Indexed %d files (%d unchanged), skipped %d, %s, marked %d missing, removed %d trashed records.", library.Name, result.FilesIndexed, result.FilesUnchanged, result.FilesSkipped, metadataRefreshSummary, result.MissingMarked, result.StaleRemoved)
		librarySettings := s.libraryRuntimeSettingsFor(library)
		cleanupRequested := librarySettings.EmptyTrashAfterScan || mode == "remove_missing"
		cleanupRetentionDays := librarySettings.TrashRetentionDays
		if mode == "remove_missing" {
			cleanupRetentionDays = 0
		}
		if cleanupRequested && result.CleanupAllowed {
			if err := ctx.Err(); err != nil {
				s.recordLog("info", fmt.Sprintf("Scan cancelled for %s.", library.Name), map[string]string{"job": jobID, "library": library.ID})
				return
			}
			removed, err := s.emptyMissingMediaTrashForLibrary(library.ID, cleanupRetentionDays)
			if err != nil {
				s.log.Warn("media trash cleanup failed", "error", err)
			} else if removed > 0 {
				result.StaleRemoved = removed
				message = fmt.Sprintf("%s Removed %d missing library item(s).", message, removed)
			}
		} else if cleanupRequested && !result.CleanupAllowed {
			message = fmt.Sprintf("%s Missing-item cleanup was held because the traversal was degraded or incomplete.", message)
		}
		if library.Type == "music" && settingBool(library.Settings, "fetchMissingLyricsAfterScan", false) && !libraryJobRecentlyQueued(s.db, "lyrics_fetch_missing", library.ID, 24*time.Hour) {
			if err := ctx.Err(); err != nil {
				s.recordLog("info", fmt.Sprintf("Scan cancelled for %s.", library.Name), map[string]string{"job": jobID, "library": library.ID})
				return
			}
			if _, err := s.createJobFor("lyrics_fetch_missing", fmt.Sprintf("Missing lyric fetch queued for %s.", library.Name), "library", library.ID); err != nil {
				s.log.Warn("missing lyric fetch queue failed", "library", library.ID, "error", err)
			}
		}
		nextMode, complete, completionErr := s.completeLibraryScanOrContinue(jobID, mode, message)
		if completionErr != nil {
			message = fmt.Sprintf("Scan completion failed for %s: %s", library.Name, completionErr.Error())
			_ = s.setJobMessage(jobID, "failed", 100, message)
			s.recordLog("error", message, map[string]string{"job": jobID, "library": library.ID})
			return
		}
		if !complete {
			mode = nextMode
			continue
		}
		s.invalidateHomeCache()
		// Scanner writes happen in large background transactions where SQL text is
		// intentionally hidden behind the transaction boundary. Publish the
		// user-visible projections explicitly so every connected client refreshes
		// Home, library grids, search, and detail/artwork after a completed scan.
		s.publishLibraryScanCompleted(library.ID)
		s.recordLog("info", message, map[string]string{"job": jobID, "library": library.ID})
		return
	}
}

func (s *Server) publishLibraryScanCompleted(libraryID string) {
	s.publishDataChanged("library.scan.completed", []string{"home", "libraries", "library-items", "media", "metadata", "search"}, "library", strings.TrimSpace(libraryID), nil)
}

func (s *Server) runLibraryStartupScanCatchup(ctx context.Context) {
	timer := time.NewTimer(45 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	s.queueAutomaticLibraryScans()
}

func (s *Server) runLibraryChangeWatcher(ctx context.Context) {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	first := true
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.refreshLibraryWatchFingerprints(ctx, !first)
		first = false
		timer.Reset(scannerChangeWatchInterval)
	}
}

func (s *Server) refreshLibraryWatchFingerprints(ctx context.Context, queueChanges bool) {
	if !s.librarySettings().ScanOnFilesystemChanges {
		return
	}
	libraries, err := s.listLibraries()
	if err != nil {
		s.log.Warn("library change watch lookup failed", "error", err)
		return
	}
	for _, library := range libraries {
		librarySettings := s.libraryRuntimeSettingsFor(library)
		if !settingBool(library.Settings, "scannerEnabled", true) || !librarySettings.ScanAutomatically {
			continue
		}
		watchInterval, watchStrategy := s.libraryChangeWatchPolicy(ctx, library)
		if libraryJobRecentlyQueued(s.db, "library_change_check", library.ID, watchInterval) {
			continue
		}
		metadata := map[string]string{
			"queueChanges":  strconv.FormatBool(queueChanges),
			"libraryName":   library.Name,
			"watchStrategy": watchStrategy,
			"watchInterval": watchInterval.String(),
		}
		if _, err := s.createJobForWithMetadata("library_change_check", fmt.Sprintf("Filesystem change check queued for %s.", library.Name), "library", library.ID, metadata); err != nil {
			s.log.Warn("library change watch queue failed", "library", library.ID, "error", err)
		}
	}
}

func (s *Server) libraryChangeWatchPolicy(ctx context.Context, library Library) (time.Duration, string) {
	// No native watcher dependency is linked today. Report the actual adaptive
	// polling policy rather than implying event-driven coverage: local roots are
	// responsive, while remote/FUSE roots are deliberately gentler.
	interval := 30 * time.Second
	strategy := "adaptive_poll_local"
	evidence := preliminaryLibraryRootEvidence(library)
	s.applyStoredStorageCircuits(ctx, evidence)
	for _, root := range evidence {
		candidate := 30 * time.Second
		name := "adaptive_poll_local"
		switch root.Classification {
		case storageSourceNetwork:
			candidate, name = 2*time.Minute, "adaptive_poll_network"
		case storageSourceFUSE:
			candidate, name = 3*time.Minute, "adaptive_poll_fuse"
		case storageSourceUnknown:
			candidate, name = 5*time.Minute, "adaptive_poll_unknown"
		}
		var failures int
		var circuit string
		if err := s.queryBackgroundRow(ctx, `SELECT consecutive_failures, circuit_state FROM storage_sources WHERE id = ?`, root.SourceID).Scan(&failures, &circuit); err == nil && circuit == "open" {
			if backoff := storageCircuitBackoff(root.Classification, failures); backoff > candidate {
				candidate = backoff
			}
			name += "_backoff"
		}
		if candidate > interval {
			interval, strategy = candidate, name
		}
	}
	return interval, strategy
}

func (s *Server) runLibraryChangeCheck(ctx context.Context, job Job) {
	if job.ResourceType != "library" || job.ResourceID == "" {
		_ = s.setJobMessage(job.ID, "failed", 100, "Filesystem change check failed because no library was selected.")
		return
	}
	library, err := s.getLibrary(job.ResourceID)
	if err != nil {
		_ = s.setJobMessage(job.ID, "failed", 100, "Filesystem change check failed because the library no longer exists.")
		return
	}
	librarySettings := s.libraryRuntimeSettingsFor(library)
	if !settingBool(library.Settings, "scannerEnabled", true) || !librarySettings.ScanAutomatically {
		_ = s.setJobMessage(job.ID, "complete", 100, "Filesystem change check skipped because automatic scanning is disabled.")
		return
	}
	queueChanges := strings.EqualFold(job.Metadata["queueChanges"], "true")
	_ = s.setJobMessage(job.ID, "running", 20, fmt.Sprintf("Checking %s for filesystem changes.", library.Name))
	changed := false
	if s.libraryKnownMediaFileCountExceedsFingerprintBudget(library.ID) {
		changed, err = s.probeLargeLibraryFilesystemChanges(ctx, library, scannerFingerprintMediaFileLimit)
	} else {
		var fingerprint string
		fingerprint, err = s.libraryFilesystemFingerprint(ctx, library)
		if err == nil {
			s.scannerWatchMu.Lock()
			if s.scannerWatch == nil {
				s.scannerWatch = map[string]string{}
			}
			previous := s.scannerWatch[library.ID]
			s.scannerWatch[library.ID] = fingerprint
			s.scannerWatchMu.Unlock()
			changed = previous != "" && previous != fingerprint
		}
	}
	if errors.Is(err, errLibraryFingerprintBudgetExceeded) {
		// The index has not caught up with a large filesystem yet. Treat the
		// bounded fingerprint overflow as positive change evidence and let the
		// full scanner establish the durable index used by rotating probes.
		changed = true
		err = nil
	}
	if err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := fmt.Sprintf("Filesystem change check failed for %s: %s", library.Name, err.Error())
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID, "library": library.ID})
		return
	}
	if !queueChanges || !changed {
		_ = s.setJobMessage(job.ID, "complete", 100, fmt.Sprintf("Filesystem change check completed for %s.", library.Name))
		return
	}
	if _, err := s.queueLibraryScan(library, "targeted", "watcher", fmt.Sprintf("Filesystem change scan queued for %s.", library.Name)); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := fmt.Sprintf("Filesystem change scan queue failed for %s: %s", library.Name, err.Error())
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID, "library": library.ID})
		return
	}
	_ = s.setJobMessage(job.ID, "complete", 100, fmt.Sprintf("Filesystem change detected for %s; scan queued.", library.Name))
}

func (s *Server) librarySettings() libraryRuntimeSettings {
	settings, err := s.loadSettings()
	if err != nil {
		return libraryRuntimeSettings{ScanOnFilesystemChanges: true}
	}
	group, _ := settings["library"].(map[string]any)
	return libraryRuntimeSettings{
		ScanOnFilesystemChanges: settingBool(group, "scanOnFilesystemChanges", true),
		ScanAutomatically:       settingBool(group, "scanAutomatically", true),
		AnalysisTier:            normalizeAnalysisTier(settingString(group, "analysisTier", "")),
		EmptyTrashAfterScan:     settingBool(group, "emptyTrashAfterScan", false),
		AllowMediaDeletion:      settingBool(group, "allowMediaDeletion", false),
		TrashRetentionDays:      max(0, settingInt(group, "trashRetentionDays", 30)),
	}
}

func (s *Server) libraryRuntimeSettingsFor(library Library) libraryRuntimeSettings {
	settings := s.librarySettings()
	settings.ScanAutomatically = librarySettingBool(library, "scanAutomatically", settings.ScanAutomatically)
	if rawTier := settingString(library.Settings, "analysisTier", ""); rawTier != "" {
		settings.AnalysisTier = normalizeAnalysisTier(rawTier)
	}
	settings.EmptyTrashAfterScan = librarySettingBool(library, "emptyTrashAfterScan", settings.EmptyTrashAfterScan)
	settings.TrashRetentionDays = max(0, librarySettingInt(library, "trashRetentionDays", settings.TrashRetentionDays))
	return settings
}

// libraryAnalysisSettingsFor resolves the server's canonical library analysis
// profile and then applies the library's explicit overrides. The Web settings
// surface writes the canonical group, while library-specific APIs write the
// override map; scan and analysis workers must observe the same effective
// profile regardless of which owner surface made the change.
func (s *Server) libraryAnalysisSettingsFor(library Library) map[string]any {
	resolved := map[string]any{}
	if settings, err := s.loadSettings(); err == nil {
		if group, ok := settings["library"].(map[string]any); ok {
			for key, value := range group {
				resolved[key] = value
			}
		}
	}
	for key, value := range library.Settings {
		resolved[key] = value
	}
	return resolved
}

const (
	analysisTierFileListOnly = "file_list_only"
	analysisTierBasic        = "basic"
	analysisTierComplete     = "complete"
	analysisTierCustom       = "custom"
)

func normalizeAnalysisTier(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case analysisTierFileListOnly:
		return analysisTierFileListOnly
	case analysisTierComplete:
		return analysisTierComplete
	case analysisTierCustom:
		return analysisTierCustom
	case analysisTierBasic:
		return analysisTierBasic
	default:
		return analysisTierBasic
	}
}

type libraryScanContentPolicy struct {
	ReadLocalMetadata        bool
	ReadExternalSidecars     bool
	DiscoverLocalArtwork     bool
	ReadEmbeddedTags         bool
	FetchDescriptiveMetadata bool
	ProbeStreams             bool
}

type resolvedLibraryScanProfile struct {
	Capabilities map[string]bool
	Content      libraryScanContentPolicy
	Tier         string
}

// resolveLibraryScanProfile reads the current effective profile from its two
// persisted owners (server defaults plus library overrides). Scanner workers
// call it at content-read boundaries; a Library value captured when a job was
// dispatched is inventory context, not continuing authorization.
func (s *Server) resolveLibraryScanProfile(libraryID string) (resolvedLibraryScanProfile, error) {
	library, err := s.getLibrary(strings.TrimSpace(libraryID))
	if err != nil {
		return resolvedLibraryScanProfile{}, err
	}
	settings := s.libraryAnalysisSettingsFor(library)
	tier := normalizeAnalysisTier(settingString(settings, "analysisTier", analysisTierBasic))
	return resolvedLibraryScanProfile{
		Capabilities: effectiveScanProfile(settings),
		Content:      scanContentPolicy(tier, settings),
		Tier:         tier,
	}, nil
}

func scanProfileRemovedReadPermission(before, after map[string]bool) bool {
	for field := range before {
		if scannerReadCapability[field] && !after[field] {
			return true
		}
	}
	return false
}

func scanContentPolicyOpensObjects(policy libraryScanContentPolicy) bool {
	return policy.ReadLocalMetadata || policy.ReadExternalSidecars || policy.DiscoverLocalArtwork || policy.ReadEmbeddedTags || policy.ProbeStreams
}

func scanContentPolicy(tier string, settings map[string]any) libraryScanContentPolicy {
	tier = normalizeAnalysisTier(tier)
	switch tier {
	case analysisTierFileListOnly:
		return libraryScanContentPolicy{}
	case analysisTierCustom:
		return libraryScanContentPolicy{
			ReadLocalMetadata:        settingBool(settings, "readLocalMetadata", false),
			ReadExternalSidecars:     settingBool(settings, "readExternalSubtitlesAndLyrics", false),
			DiscoverLocalArtwork:     settingBool(settings, "discoverLocalArtwork", false),
			ReadEmbeddedTags:         settingBool(settings, "readEmbeddedTags", false),
			FetchDescriptiveMetadata: settingBool(settings, "fetchDescriptiveMetadata", false),
			ProbeStreams:             settingBool(settings, "probeStreams", false),
		}
	default:
		return libraryScanContentPolicy{
			ReadLocalMetadata:        true,
			ReadExternalSidecars:     true,
			DiscoverLocalArtwork:     true,
			ReadEmbeddedTags:         true,
			FetchDescriptiveMetadata: true,
			ProbeStreams:             true,
		}
	}
}

func (s *Server) libraryKnownMediaFileCountExceedsFingerprintBudget(libraryID string) bool {
	if scannerFingerprintMediaFileLimit <= 0 || strings.TrimSpace(libraryID) == "" {
		return false
	}
	var marker int
	err := s.queryBackgroundRow(context.Background(), `
		SELECT 1
		FROM media_files
		WHERE library_id = ?
			AND available = 1
		LIMIT 1 OFFSET ?`,
		libraryID, scannerFingerprintMediaFileLimit).Scan(&marker)
	return err == nil
}

// probeLargeLibraryFilesystemChanges rotates through the indexed catalogue in
// bounded pages instead of walking a million-item filesystem every few
// minutes. File stat tuples detect edits/deletes; parent-directory mtimes
// detect additions. A six-hour full scan remains the correctness backstop.
func (s *Server) probeLargeLibraryFilesystemChanges(ctx context.Context, library Library, limit int) (bool, error) {
	if limit <= 0 {
		limit = 25000
	}
	s.scannerWatchMu.Lock()
	if s.scannerWatchCursor == nil {
		s.scannerWatchCursor = map[string]string{}
	}
	cursor := s.scannerWatchCursor[library.ID]
	s.scannerWatchMu.Unlock()
	evidence := s.inspectLibraryRootsWithContext(ctx, library)
	roots := healthyScanRoots(evidence)
	if len(roots) == 0 || len(roots) != len(evidence) {
		return false, errors.New("filesystem change evidence is incomplete because a storage source is unavailable")
	}

	rows, err := s.queryBackgroundRead(ctx, `
		SELECT path, MAX(size_bytes), MAX(mod_time), MAX(last_seen_at)
		FROM media_files
		WHERE library_id = ? AND available = 1 AND path > ?
		GROUP BY path
		ORDER BY path
		LIMIT ?`, library.ID, cursor, limit)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	changed := false
	nextCursor := cursor
	count := 0
	directories := map[string]time.Time{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		var path, modTime, lastSeenRaw string
		var size int64
		if err := rows.Scan(&path, &size, &modTime, &lastSeenRaw); err != nil {
			return false, err
		}
		count++
		nextCursor = path
		root, ok := storageRootForPath(roots, path)
		if !ok {
			changed = true
			continue
		}
		info, statErr := s.storageStat(ctx, storageRequestForRoot(root, "watch file stat"), path)
		if storageErrorAffectsAuthority(statErr) {
			return false, statErr
		}
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != size || fileModTime(info) != modTime {
			changed = true
			continue
		}
		lastSeen, parseErr := time.Parse(time.RFC3339, lastSeenRaw)
		if parseErr != nil {
			continue
		}
		dir := filepath.Dir(path)
		if previous, ok := directories[dir]; !ok || lastSeen.After(previous) {
			directories[dir] = lastSeen
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	for dir, lastSeen := range directories {
		root, ok := storageRootForPath(roots, dir)
		if !ok {
			changed = true
			break
		}
		info, statErr := s.storageStat(ctx, storageRequestForRoot(root, "watch directory stat"), dir)
		if storageErrorAffectsAuthority(statErr) {
			return false, statErr
		}
		if statErr != nil || !info.IsDir() || info.ModTime().UTC().After(lastSeen.Add(time.Second)) {
			changed = true
			break
		}
	}
	if count < limit {
		nextCursor = ""
	}
	s.scannerWatchMu.Lock()
	s.scannerWatchCursor[library.ID] = nextCursor
	s.scannerWatchMu.Unlock()
	return changed, nil
}

func librarySettingBool(library Library, key string, fallback bool) bool {
	if _, ok := library.Settings[key]; !ok {
		return fallback
	}
	return settingBool(library.Settings, key, fallback)
}

func librarySettingInt(library Library, key string, fallback int) int {
	if _, ok := library.Settings[key]; !ok {
		return fallback
	}
	return settingInt(library.Settings, key, fallback)
}

func (s *Server) pruneMediaTrash(retentionDays int) (int, error) {
	trashRoot := filepath.Join(s.cfg.AppDataDir, "media-trash")
	entries, err := os.ReadDir(trashRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(max(1, retentionDays)) * 24 * time.Hour)
	removed := 0
	for _, entry := range entries {
		path := filepath.Join(trashRoot, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().UTC().After(cutoff) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func (s *Server) emptyMissingMediaTrash(retentionDays int) (int, error) {
	return s.emptyMissingMediaTrashForLibraryContext(context.Background(), "", retentionDays)
}

func (s *Server) emptyMissingMediaTrashForLibrary(libraryID string, retentionDays int) (int, error) {
	return s.emptyMissingMediaTrashForLibraryContext(context.Background(), libraryID, retentionDays)
}

func (s *Server) emptyMissingMediaTrashForLibraryContext(ctx context.Context, libraryID string, retentionDays int) (int, error) {
	cutoff := time.Now().UTC()
	if retentionDays > 0 {
		cutoff = cutoff.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	}
	removed := 0
	err := s.withBackgroundTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		var err error
		removed, err = emptyMissingMediaTrashTx(tx, cutoff.Format(time.RFC3339), strings.TrimSpace(libraryID))
		return err
	})
	return removed, err
}

func emptyMissingMediaTrashTx(tx *sql.Tx, cutoff string, libraryID string) (int, error) {
	query := `DELETE FROM media_files WHERE available = 0 AND missing_since <> '' AND missing_since <= ?`
	args := []any{cutoff}
	if libraryID != "" {
		query += ` AND library_id = ?`
		args = append(args, libraryID)
	}
	if _, err := tx.Exec(query, args...); err != nil {
		return 0, err
	}
	removed, err := pruneOrphanScannedPlayableItems(tx, libraryID)
	if err != nil {
		return 0, err
	}
	return int(removed), pruneEmptyScannedParents(tx, libraryID)
}

func pruneOrphanScannedPlayableItems(tx *sql.Tx, libraryID string) (int64, error) {
	query := `
		SELECT m.id
		FROM media_items m
		WHERE (m.id LIKE 'scan_%' OR EXISTS (
				SELECT 1 FROM media_scanner_identity_aliases a WHERE a.media_id = m.id
			))
			AND m.source_url = ''
			AND m.type NOT IN ('show', 'anime', 'season', 'album', 'artist')
			AND NOT EXISTS (SELECT 1 FROM media_files mf WHERE mf.media_id = m.id)
			AND NOT EXISTS (SELECT 1 FROM media_items child WHERE child.parent_id = m.id)`
	args := []any{}
	if libraryID != "" {
		query += ` AND m.library_id = ?`
		args = append(args, libraryID)
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM media_scanner_identity_aliases WHERE media_id = ?`, id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`DELETE FROM media_search WHERE media_id = ?`, id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`DELETE FROM media_items WHERE id = ?`, id); err != nil {
			return 0, err
		}
	}
	return int64(len(ids)), nil
}

func pruneEmptyScannedParents(tx *sql.Tx, libraryID string) error {
	for {
		query := `
			DELETE FROM media_items
			WHERE id LIKE 'scan_%'
				AND type IN ('season', 'album', 'artist', 'show', 'anime')
				AND NOT EXISTS (SELECT 1 FROM media_items child WHERE child.parent_id = media_items.id)`
		args := []any{}
		if libraryID != "" {
			query += ` AND library_id = ?`
			args = append(args, libraryID)
		}
		result, err := tx.Exec(query, args...)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return nil
		}
	}
}

func (s *Server) queueAutomaticLibraryScans() {
	libraries, err := s.listLibraries()
	if err != nil {
		s.log.Warn("automatic library scan lookup failed", "error", err)
		return
	}
	for _, library := range libraries {
		librarySettings := s.libraryRuntimeSettingsFor(library)
		if !settingBool(library.Settings, "scannerEnabled", true) || !librarySettings.ScanAutomatically {
			continue
		}
		if _, err := s.queueLibraryScan(library, "reconcile", "startup", fmt.Sprintf("Startup scan queued for %s.", library.Name)); err != nil {
			s.log.Warn("automatic library scan queue failed", "library", library.ID, "error", err)
		}
	}
}

func (s *Server) libraryScanRecentlyQueued(libraryID string, within time.Duration) bool {
	cutoff := time.Now().UTC().Add(-within).Format(time.RFC3339)
	var count int
	_ = s.queryBackgroundRow(context.Background(), `SELECT COUNT(*) FROM jobs WHERE type = 'library_scan' AND resource_type = 'library' AND resource_id = ? AND created_at >= ?`, libraryID, cutoff).Scan(&count)
	return count > 0
}

func (s *Server) libraryFilesystemFingerprint(parent context.Context, library Library) (string, error) {
	return s.libraryFilesystemFingerprintWithLimit(parent, library, scannerFingerprintMediaFileLimit)
}

func (s *Server) libraryFilesystemFingerprintWithLimit(parent context.Context, library Library, mediaFileLimit int) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	evidence := s.inspectLibraryRootsWithContext(ctx, library)
	roots := healthyScanRoots(evidence)
	if len(roots) == 0 || len(roots) != len(evidence) {
		return "", errors.New("filesystem fingerprint is incomplete because a storage source is unavailable")
	}
	hash := sha256.New()
	visitedMediaFiles := 0
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		_, _ = hash.Write([]byte(root.real))
		err := s.walkScannerRoot(ctx, root, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				if shouldSkipScanDir(entry.Name()) && path != root.real {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			realPath, err := s.storageEvalSymlinks(ctx, storageRequestForRoot(root, "fingerprint resolve"), path)
			if err != nil {
				return err
			}
			if !pathWithinRoot(realPath, root.real) || !isMediaFileForLibrary(library.Type, realPath) {
				return nil
			}
			visitedMediaFiles++
			if mediaFileLimit > 0 && visitedMediaFiles > mediaFileLimit {
				return fmt.Errorf("%w after %d media files", errLibraryFingerprintBudgetExceeded, mediaFileLimit)
			}
			info, err := s.storageStat(ctx, storageRequestForRoot(root, "fingerprint stat"), realPath)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root.real, realPath)
			_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%d\x00", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Server) performLibraryScan(library Library, jobID string) (libraryScanResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
	defer cancel()
	return s.performLibraryScanWithContext(ctx, library, jobID)
}

func (s *Server) performLibraryScanWithContext(ctx context.Context, library Library, jobID string) (result libraryScanResult, returnedErr error) {
	return s.performLibraryScanWithMode(ctx, library, jobID, "reconcile")
}

func (s *Server) performLibraryScanWithMode(ctx context.Context, library Library, jobID, mode string) (result libraryScanResult, returnedErr error) {
	release, err := s.acquireLibraryScan(ctx, library.ID)
	if err != nil {
		return result, err
	}
	defer release()
	// Metadata-agent settings are independent of scan-profile authorization.
	// The latter is deliberately re-resolved at each content-read boundary.
	metadataSettings := s.metadataAgentSettings()
	localRootEvidence := s.inspectLibraryRootsWithContext(ctx, library)
	remoteRootEvidence, err := s.remoteLibraryRootEvidence(ctx, library.ID)
	if err != nil {
		return result, err
	}
	rootEvidence := append(localRootEvidence, remoteRootEvidence...)
	mode = normalizeScanMode(mode)
	run, err := s.beginLibraryScanRun(ctx, library, jobID, mode, rootEvidence)
	if err != nil {
		return result, err
	}
	warnings := make([]string, 0)
	unsupportedMedia := map[string]int{}
	defer func() {
		status := "healthy"
		if errors.Is(returnedErr, context.Canceled) || errors.Is(returnedErr, context.DeadlineExceeded) {
			status = "cancelled"
		} else if returnedErr != nil {
			status = "failed"
		} else if result.DegradedRoots > 0 {
			status = "degraded"
		}
		finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if finishErr := s.finishLibraryScanRun(finishCtx, run, status, result, warnings); finishErr != nil && returnedErr == nil {
			returnedErr = fmt.Errorf("finish scan run: %w", finishErr)
		}
	}()
	for _, root := range rootEvidence {
		if root.Health != "healthy" {
			result.DegradedRoots++
			warnings = append(warnings, fmt.Sprintf("%s: %s", root.ConfiguredPath, root.ErrorClass))
		}
	}
	roots := healthyScanRoots(localRootEvidence)
	if len(roots) == 0 && len(remoteRootEvidence) == 0 {
		return result, errors.New("no readable library folders were found")
	}
	checkpoints, err := s.loadLibraryScanDirectoryCheckpoints(ctx, library.ID)
	if err != nil {
		return result, err
	}
	continuation, err := s.beginOrResumeScannerContinuation(ctx, library.ID, mode)
	if err != nil {
		return result, err
	}
	checkpointMode := mode != "force_full" && mode != "remove_missing" && len(checkpoints) > 0 && !s.libraryHasUnassignedMediaDirectories(ctx, library.ID)
	now := time.Now().UTC().Format(time.RFC3339)
	scanGeneration := continuation.ScanGeneration
	if len(remoteRootEvidence) > 0 {
		remoteResult, err := s.scanRemoteStorageSources(ctx, library, run, scanGeneration, now)
		if err != nil {
			return result, err
		}
		mergeLibraryScanResult(&result, remoteResult)
	}
	pending := make([]scannerMediaFile, 0, scannerWriteBatchSize)
	pendingPaths := make([]scannerPathCandidate, 0, scannerReconcileBatchSize)
	directorySignatures := map[string]string{}
	directoryUnchanged := map[string]bool{}
	visitedDirectories := map[string]scannerDirectoryCheckpoint{}
	changedDirectories := map[string]bool{}
	discoveredCount := 0
	flushPending := func() error {
		if len(pending) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !s.waitForForegroundPressureToEase(ctx) {
			return ctx.Err()
		}
		batch := pending
		pending = make([]scannerMediaFile, 0, scannerWriteBatchSize)
		_ = s.setJobProgress(jobID, "running", 58, fmt.Sprintf("Writing media index (%d discovered).", discoveredCount))
		// Commit path-derived inventory before any source-content, embedded-tag,
		// NFO, subtitle, lyric, or artwork read. Promotion into the public media
		// catalog remains atomic after enrichment, preserving move reconciliation
		// and never publishing a destructive half-enriched projection.
		if err := s.persistScannerInventoryBatch(ctx, library.ID, batch, now, scanGeneration); err != nil {
			return err
		}
		metadataQueued, analysisQueued, indexed := 0, 0, 0
		authorizedReads := map[string]bool{}
		for index := range batch {
			if !s.waitForForegroundPressureToEase(ctx) {
				return ctx.Err()
			}
			root, ok := storageRootForPath(roots, batch[index].SourcePath)
			if !ok {
				return errors.New("discovered media escaped admitted storage roots")
			}
			profile, err := s.resolveLibraryScanProfile(library.ID)
			if err != nil {
				return err
			}
			if !scanContentPolicyOpensObjects(profile.Content) {
				batch[index].FilesystemPrepared = true
				continue
			}
			if err := s.boundedStorageIO(ctx, storageRequestForRoot(root, "scanner basic enrichment"), func() error {
				if err := ctx.Err(); err != nil {
					return err
				}
				// Admission can wait behind another source user. Resolve again inside
				// the admitted callback, immediately before the first object open.
				live, err := s.resolveLibraryScanProfile(library.ID)
				if err != nil {
					return err
				}
				policy := live.Content
				if !scanContentPolicyOpensObjects(policy) {
					batch[index].FilesystemPrepared = true
					return nil
				}
				for field := range live.Capabilities {
					if scannerReadCapability[field] {
						authorizedReads[field] = true
					}
				}
				info, _ := os.Stat(batch[index].SourcePath)
				if policy.ProbeStreams {
					batch[index].ContentFingerprint = scannerContentFingerprint(batch[index].SourcePath, info)
				}
				batch[index].LocalNFOEnabled = policy.ReadLocalMetadata && metadataSettings.LocalNFO
				batch[index].ReadExternalSidecars = policy.ReadExternalSidecars
				batch[index].DiscoverLocalArtwork = policy.DiscoverLocalArtwork
				if batch[index].LocalNFOEnabled {
					batch[index].LocalMetadata = scannerMetadataForLocalMode(mergeScannerMetadata(batch[index].LocalMetadata, localMetadataForMediaFile(batch[index].SourcePath, batch[index].ScanRoot)), localMetadataModeForLibrary(library))
				}
				if library.Type == "audiobook" && batch[index].LocalNFOEnabled {
					batch[index].LocalMetadata = scannerMetadataForLocalMode(mergeScannerMetadata(batch[index].LocalMetadata, audiobookLocalMetadataForFile(batch[index].SourcePath, batch[index].ScanRoot)), localMetadataModeForLibrary(library))
				}
				if library.Type == "music" {
					customMetadataSettings := metadataSettings
					customMetadataSettings.EmbeddedTags = metadataSettings.EmbeddedTags && policy.ReadEmbeddedTags
					batch[index] = s.enrichScannedMusicFileWithSettings(ctx, batch[index], library, customMetadataSettings)
				}
				batch[index].FilesystemPrepared = false
				return nil
			}); err != nil {
				return err
			}
		}
		publishProfile, err := s.resolveLibraryScanProfile(library.ID)
		if err != nil {
			return err
		}
		if scanProfileRemovedReadPermission(authorizedReads, publishProfile.Capabilities) {
			// One already-entered bounded filesystem call may finish after a
			// downgrade, but facts derived under the removed permission never
			// cross this publication boundary.
			return context.Canceled
		}
		_, indexed, metadataQueued, analysisQueued, err = s.writeScannedMediaBatch(ctx, library, batch, now, scanGeneration,
			publishProfile.Content.FetchDescriptiveMetadata, capabilitiesIntersect(publishProfile.Capabilities, analysisCapability))
		if err != nil {
			return err
		}
		result.FilesIndexed += indexed
		result.MetadataRefreshQueued += metadataQueued
		result.AnalysisQueued += analysisQueued
		return nil
	}
	addDiscovered := func(files []scannerMediaFile) error {
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return err
			}
			pending = append(pending, file)
			if len(pending) >= scannerWriteBatchSize {
				if err := flushPending(); err != nil {
					return err
				}
			}
		}
		return nil
	}
	flushPendingPaths := func() error {
		if len(pendingPaths) == 0 {
			return nil
		}
		candidates := pendingPaths
		pendingPaths = make([]scannerPathCandidate, 0, scannerReconcileBatchSize)
		if !s.waitForForegroundPressureToEase(ctx) {
			return ctx.Err()
		}
		unchanged, err := s.markUnchangedScannedPaths(ctx, library.ID, candidates, now, scanGeneration)
		if err != nil {
			return err
		}
		for _, candidate := range candidates {
			if unchanged[candidate.Path] {
				result.FilesIndexed++
				result.FilesUnchanged++
				continue
			}
			root, ok := storageRootForPath(roots, candidate.Path)
			if !ok {
				return errors.New("discovered media escaped admitted storage roots")
			}
			var file scannerMediaFile
			if err := s.boundedStorageIO(ctx, storageRequestForRoot(root, "scanner media lookup"), func() error {
				file = scannerFileForPath(library, root.real, candidate.Path, false, false)
				file.QuickSignature = candidate.QuickSignature
				return nil
			}); err != nil {
				return err
			}
			if err := addDiscovered(expandMultiEpisodeScannedFile(file)); err != nil {
				return err
			}
		}
		return nil
	}
	for i, root := range roots {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		_ = s.setJobProgress(jobID, "running", 10+((i*25)/max(1, len(roots))), "Scanning "+root.display)
		_ = s.updateScanRootEvidence(ctx, run, root, "running", "", "", 0, 0)
		rootDirectories := 0
		rootFiles := 0
		rootErrorClass := ""
		rootErrorMessage := ""
		rootTraversalUnsafe := false
		openDirectories := make([]string, 0, 16)
		completedContinuations := map[string]scannerContinuationDirectory{}
		recordRootFailure := func(failure error) {
			if failure == nil {
				return
			}
			rootTraversalUnsafe = true
			completedContinuations = map[string]scannerContinuationDirectory{}
			if rootErrorClass == "" {
				rootErrorClass = storageErrorClass(failure)
				rootErrorMessage = boundedStorageError(failure)
			}
		}
		persistCompletedCheckpoints := func(force bool) error {
			if len(completedContinuations) == 0 || (!force && len(completedContinuations) < scannerCheckpointCommitBatch) {
				return nil
			}
			if rootTraversalUnsafe {
				completedContinuations = map[string]scannerContinuationDirectory{}
				return nil
			}
			// A traversal checkpoint becomes resumable only after every media row
			// from those completed directories is durable. Replaying a directory
			// remains safe if the process stops before this boundary.
			if err := flushPendingPaths(); err != nil {
				return err
			}
			if err := flushPending(); err != nil {
				return err
			}
			if err := s.persistScannerContinuationDirectories(ctx, continuation, completedContinuations); err != nil {
				return err
			}
			for path, directory := range completedContinuations {
				continuation.Directories[path] = directory
			}
			completedContinuations = map[string]scannerContinuationDirectory{}
			_ = s.updateScanRootEvidence(ctx, run, root, "running", "", "", rootDirectories, rootFiles)
			return nil
		}
		completeExitedDirectories := func(nextPath string) error {
			for len(openDirectories) > 0 && !pathWithinRoot(nextPath, openDirectories[len(openDirectories)-1]) {
				completedPath := openDirectories[len(openDirectories)-1]
				openDirectories = openDirectories[:len(openDirectories)-1]
				if checkpoint, ok := visitedDirectories[completedPath]; ok && changedDirectories[completedPath] {
					completedContinuations[completedPath] = scannerContinuationDirectory{
						Path: checkpoint.Path, Signature: checkpoint.Signature,
						MediaFileCount: checkpoint.MediaFileCount, Changed: true,
					}
				}
			}
			return persistCompletedCheckpoints(false)
		}
		walkErr := s.walkScannerRoot(ctx, root, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := completeExitedDirectories(path); err != nil {
				return err
			}
			if walkErr != nil {
				result.FilesSkipped++
				recordRootFailure(walkErr)
				if rootErrorClass == "" {
					rootErrorClass = storageErrorClass(walkErr)
					rootErrorMessage = boundedStorageError(walkErr)
				}
				if entry == nil || entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				rootDirectories++
				openDirectories = append(openDirectories, path)
				var logical scannerMediaFile
				var logicalOK bool
				if err := s.boundedStorageIO(ctx, storageRequestForRoot(root, "scanner disc lookup"), func() error {
					logical, logicalOK = logicalDiscSourceForPath(library, root.real, path)
					return nil
				}); err != nil {
					return err
				}
				if logicalOK && path != root.real {
					if err := addDiscovered([]scannerMediaFile{logical}); err != nil {
						return err
					}
					discoveredCount++
					rootFiles++
					return filepath.SkipDir
				}
				if shouldSkipScanDir(entry.Name()) && path != root.real {
					return filepath.SkipDir
				}
				var signature string
				var mediaCount int
				checkpointCache := cloneScannerSignatureCache(directorySignatures)
				if err := s.boundedStorageIO(ctx, storageRequestForRoot(root, "scanner directory checkpoint"), func() error {
					signature, mediaCount = scannerDirectoryCheckpointState(path, library.Type, checkpointCache)
					return nil
				}); err != nil {
					return err
				}
				directorySignatures = checkpointCache
				checkpoint := scannerDirectoryCheckpoint{Path: path, Signature: signature, MediaFileCount: mediaCount}
				visitedDirectories[path] = checkpoint
				if resumed, ok := continuation.Directories[path]; ok && resumed.Signature == signature && resumed.MediaFileCount == mediaCount {
					leaf := false
					if path != root.real {
						if err := s.boundedStorageIO(ctx, storageRequestForRoot(root, "scanner continuation leaf lookup"), func() error {
							leaf = scannerDirectoryIsLeaf(path)
							return nil
						}); err != nil {
							return err
						}
					}
					if leaf {
						directoryUnchanged[path] = true
						changedDirectories[path] = resumed.Changed
						result.FilesIndexed += mediaCount
						result.FilesUnchanged += mediaCount
						return filepath.SkipDir
					}
				}
				previous, known := checkpoints[path]
				if checkpointMode && known && previous.Signature == signature && previous.MediaFileCount == mediaCount {
					directoryUnchanged[path] = true
					result.FilesIndexed += mediaCount
					result.FilesUnchanged += mediaCount
					leaf := false
					if path != root.real {
						if err := s.boundedStorageIO(ctx, storageRequestForRoot(root, "scanner leaf lookup"), func() error {
							leaf = scannerDirectoryIsLeaf(path)
							return nil
						}); err != nil {
							return err
						}
					}
					if leaf {
						return filepath.SkipDir
					}
				} else {
					changedDirectories[path] = true
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				result.FilesSkipped++
				return nil
			}
			rootFiles++
			realPath, err := scannerResolveMediaPath(s, ctx, storageRequestForRoot(root, "scanner resolve"), path)
			if err != nil {
				result.FilesSkipped++
				recordRootFailure(err)
				return nil
			}
			if !pathWithinRoot(realPath, root.real) {
				result.FilesSkipped++
				return nil
			}
			if !isMediaFileForLibrary(library.Type, realPath) {
				ext := strings.ToLower(filepath.Ext(realPath))
				if knownMediaLikeExtension(ext) {
					result.FilesSkipped++
					unsupportedMedia[ext]++
				}
				return nil
			}
			if directoryUnchanged[filepath.Dir(realPath)] {
				return nil
			}
			info, err := scannerStatMediaPath(s, ctx, storageRequestForRoot(root, "scanner stat"), realPath)
			if err != nil {
				result.FilesSkipped++
				recordRootFailure(err)
				return nil
			}
			if !info.Mode().IsRegular() {
				result.FilesSkipped++
				return nil
			}
			quickSignature := ""
			signatureCache := cloneScannerSignatureCache(directorySignatures)
			if err := s.boundedStorageIO(ctx, storageRequestForRoot(root, "scanner signature"), func() error {
				quickSignature = scannerQuickSignature(realPath, info, signatureCache)
				return nil
			}); err != nil {
				return err
			}
			directorySignatures = signatureCache
			pendingPaths = append(pendingPaths, scannerPathCandidate{
				Path: realPath, DirectoryPath: filepath.Dir(realPath), Size: info.Size(), ModTime: fileModTime(info),
				QuickSignature: quickSignature,
			})
			discoveredCount++
			if len(pendingPaths) >= scannerReconcileBatchSize {
				return flushPendingPaths()
			}
			return nil
		})
		if walkErr == nil {
			for len(openDirectories) > 0 {
				completedPath := openDirectories[len(openDirectories)-1]
				openDirectories = openDirectories[:len(openDirectories)-1]
				if checkpoint, ok := visitedDirectories[completedPath]; ok && changedDirectories[completedPath] {
					completedContinuations[completedPath] = scannerContinuationDirectory{
						Path: checkpoint.Path, Signature: checkpoint.Signature,
						MediaFileCount: checkpoint.MediaFileCount, Changed: true,
					}
				}
			}
		}
		if err := persistCompletedCheckpoints(true); err != nil && walkErr == nil {
			walkErr = err
		}
		if walkErr != nil {
			if errors.Is(walkErr, context.Canceled) || errors.Is(walkErr, context.DeadlineExceeded) {
				_ = s.updateScanRootEvidence(context.Background(), run, root, "cancelled", storageErrorClass(walkErr), boundedStorageError(walkErr), rootDirectories, rootFiles)
				return result, walkErr
			}
			var catalogErr scannerCatalogApplyError
			if errors.As(walkErr, &catalogErr) {
				return result, catalogErr.err
			}
			rootErrorClass = storageErrorClass(walkErr)
			rootErrorMessage = boundedStorageError(walkErr)
		}
		if rootErrorClass != "" {
			result.DegradedRoots++
			warnings = append(warnings, fmt.Sprintf("%s: %s", root.display, rootErrorClass))
			_ = s.updateScanRootEvidence(ctx, run, root, "degraded", rootErrorClass, rootErrorMessage, rootDirectories, rootFiles)
		} else if err := s.updateScanRootEvidence(ctx, run, root, "healthy", "", "", rootDirectories, rootFiles); err != nil {
			return result, err
		}
	}
	if err := flushPendingPaths(); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := flushPending(); err != nil {
		return result, err
	}
	if len(unsupportedMedia) > 0 {
		exts := make([]string, 0, len(unsupportedMedia))
		for ext := range unsupportedMedia {
			exts = append(exts, ext)
		}
		sort.Strings(exts)
		parts := make([]string, 0, len(exts))
		for _, ext := range exts {
			parts = append(parts, fmt.Sprintf("%s (%d)", ext, unsupportedMedia[ext]))
		}
		warnings = append(warnings, "Media-like files not accepted by this library type: "+strings.Join(parts, ", "))
	}

	result.AbsenceAuthoritative = result.DegradedRoots == 0 && len(roots) == len(localRootEvidence)
	result.CleanupAllowed = result.AbsenceAuthoritative
	if result.AbsenceAuthoritative {
		_ = s.setJobProgress(jobID, "running", 84, "Reconciling missing media.")
		if checkpointMode {
			removedDirectories := scannerRemovedDirectories(checkpoints, visitedDirectories)
			if result.MissingMarked, err = s.reconcileChangedScanDirectories(ctx, library.ID, now, scanGeneration, changedDirectories, removedDirectories); err != nil {
				return result, err
			}
		} else if result.MissingMarked, err = s.reconcileScannedMedia(ctx, library.ID, now, scanGeneration); err != nil {
			return result, err
		}
		if err := s.pruneScannerInventory(ctx, library.ID, scanGeneration, checkpointMode, changedDirectories, scannerRemovedDirectories(checkpoints, visitedDirectories)); err != nil {
			return result, err
		}
	} else {
		_ = s.setJobProgress(jobID, "running", 84, "Holding missing-media reconciliation because storage evidence is incomplete.")
	}
	if result.AbsenceAuthoritative {
		if err := s.persistLibraryScanDirectoryCheckpoints(ctx, library.ID, visitedDirectories, checkpoints, now, true); err != nil {
			return result, err
		}
		if err := s.clearScannerContinuation(ctx, continuation); err != nil {
			return result, err
		}
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if result.MetadataRefreshQueued > 0 || result.AnalysisQueued > 0 {
		if _, dispatchErr := s.dispatchScannerBacklog(ctx); dispatchErr != nil && !errors.Is(dispatchErr, context.Canceled) {
			s.log.Warn("scanner backlog handoff deferred", "library", library.ID, "error", dispatchErr)
		}
		s.signalJobWake()
	}
	// Catalog commits are authoritative before obsolete managed files are
	// removed. This keeps a failed transaction from pruning the artifact it
	// still references while ensuring every successful scan collects stale
	// copied sidecars rather than waiting for a process restart.
	if reconcileErr := s.reconcileSubtitleArtifactsAfterScan(ctx, library.ID); reconcileErr != nil && !errors.Is(reconcileErr, context.Canceled) {
		s.log.Warn("subtitle artifact cleanup deferred", "library", library.ID, "error", reconcileErr)
	}
	s.queueLibraryReadModelRepair(library.ID)
	return result, nil
}

func (s *Server) pruneScannerInventory(ctx context.Context, libraryID, scanGeneration string, checkpointMode bool, changedDirectories map[string]bool, removedDirectories []string) error {
	if strings.TrimSpace(libraryID) == "" || strings.TrimSpace(scanGeneration) == "" {
		return nil
	}
	return s.withBackgroundTxTagged(ctx, []string{"scanner_inventory"}, func(tx *sql.Tx) error {
		if !checkpointMode {
			_, err := tx.Exec(`DELETE FROM scanner_inventory_entries WHERE library_id = ? AND scan_generation <> ?`, libraryID, scanGeneration)
			return err
		}
		directories := make(map[string]bool, len(changedDirectories)+len(removedDirectories))
		for directory := range changedDirectories {
			directories[filepath.Clean(directory)] = true
		}
		for _, directory := range removedDirectories {
			directories[filepath.Clean(directory)] = true
		}
		for directory := range directories {
			prefix := escapeSQLLike(directory+string(filepath.Separator)) + "%"
			if _, err := tx.Exec(`
				DELETE FROM scanner_inventory_entries
				WHERE library_id = ? AND scan_generation <> ?
					AND (path = ? OR path LIKE ? ESCAPE '\')`,
				libraryID, scanGeneration, directory, prefix); err != nil {
				return err
			}
		}
		return nil
	})
}

func knownMediaLikeExtension(ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	return videoExtensions[ext] || audioExtensions[ext] || audiobookExtensions[ext]
}

func cloneScannerSignatureCache(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func scannerDirectoryIsLeaf(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() && !shouldSkipScanDir(entry.Name()) {
			return false
		}
	}
	return true
}

func filepathRootForCandidate(roots []scanRoot, path string) string {
	for _, root := range roots {
		if pathWithinRoot(path, root.real) {
			return root.real
		}
	}
	return filepath.Dir(path)
}

func scannerQuickSignature(path string, info os.FileInfo, directoryCache map[string]string) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%d\x00%s\x00", fileSize(info), fileModTime(info))
	// Sidecar metadata, subtitles, lyrics, and local artwork can change while
	// the media inode remains untouched. Hash directory entry metadata once per
	// directory (plus the parent for show/artist-level assets) and reuse it for
	// every file in that folder. No media bytes are opened on this path.
	for _, dir := range []string{filepath.Dir(path), filepath.Dir(filepath.Dir(path))} {
		signature, ok := directoryCache[dir]
		if !ok {
			signature = scannerDirectoryEntrySignature(dir)
			directoryCache[dir] = signature
		}
		_, _ = io.WriteString(hash, dir)
		_, _ = io.WriteString(hash, "\x00")
		_, _ = io.WriteString(hash, signature)
		_, _ = io.WriteString(hash, "\x00")
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func scannerDirectoryEntrySignature(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "unreadable"
	}
	hash := sha256.New()
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%d\x00%d\x00", entry.Name(), info.Size(), info.ModTime().UnixNano(), info.Mode().Type())
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// scannerDirectorySidecarSignature fingerprints only regular files owned by a
// directory. Child directory metadata is deliberately excluded: changing one
// child updates its directory entry in the parent, but must not invalidate
// every sibling checkpoint. Parent-level NFO, artwork, subtitle, and lyric
// changes still invalidate children because those are regular files here.
func scannerDirectorySidecarSignature(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "unreadable"
	}
	hash := sha256.New()
	for _, entry := range entries {
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%d\x00%d\x00", entry.Name(), info.Size(), info.ModTime().UnixNano(), info.Mode().Type())
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func scannerDirectoryCheckpointState(dir, libraryType string, directoryCache map[string]string) (string, int) {
	hash := sha256.New()
	for index, candidate := range []string{dir, filepath.Dir(dir)} {
		cacheKey := candidate
		if index == 1 {
			cacheKey = "sidecars\x00" + candidate
		}
		signature, ok := directoryCache[cacheKey]
		if !ok {
			if index == 0 {
				signature = scannerDirectoryEntrySignature(candidate)
			} else {
				signature = scannerDirectorySidecarSignature(candidate)
			}
			directoryCache[cacheKey] = signature
		}
		_, _ = io.WriteString(hash, candidate)
		_, _ = io.WriteString(hash, "\x00")
		_, _ = io.WriteString(hash, signature)
		_, _ = io.WriteString(hash, "\x00")
	}
	mediaCount := 0
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if entry.Type().IsRegular() && isMediaFileForLibrary(libraryType, filepath.Join(dir, entry.Name())) {
				mediaCount++
			}
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), mediaCount
}

func (s *Server) loadLibraryScanDirectoryCheckpoints(ctx context.Context, libraryID string) (map[string]scannerDirectoryCheckpoint, error) {
	rows, err := s.queryBackgroundRead(ctx, `
		SELECT path, signature, media_file_count
		FROM library_scan_directories
		WHERE library_id = ?`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]scannerDirectoryCheckpoint{}
	for rows.Next() {
		var checkpoint scannerDirectoryCheckpoint
		if err := rows.Scan(&checkpoint.Path, &checkpoint.Signature, &checkpoint.MediaFileCount); err != nil {
			return nil, err
		}
		checkpoint.Path = filepath.Clean(checkpoint.Path)
		out[checkpoint.Path] = checkpoint
	}
	return out, rows.Err()
}

func (s *Server) libraryHasUnassignedMediaDirectories(ctx context.Context, libraryID string) bool {
	var exists int
	err := s.queryBackgroundRow(ctx, `
		SELECT 1 FROM media_files
		WHERE library_id = ? AND available = 1 AND COALESCE(directory_path, '') = ''
		LIMIT 1`, libraryID).Scan(&exists)
	return err == nil
}

func scannerRemovedDirectories(existing, visited map[string]scannerDirectoryCheckpoint) []string {
	removed := make([]string, 0)
	for path := range existing {
		if _, ok := visited[path]; !ok {
			removed = append(removed, path)
		}
	}
	sort.Strings(removed)
	return removed
}

func (s *Server) persistLibraryScanDirectoryCheckpoints(ctx context.Context, libraryID string, visited, existing map[string]scannerDirectoryCheckpoint, now string, removeUnseen bool) error {
	if !s.waitForForegroundPressureToEase(ctx) {
		return ctx.Err()
	}
	return s.withBackgroundTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		for path, checkpoint := range visited {
			if prior, ok := existing[path]; ok && prior.Signature == checkpoint.Signature && prior.MediaFileCount == checkpoint.MediaFileCount {
				continue
			}
			if _, err := tx.Exec(`
				INSERT INTO library_scan_directories (library_id, path, signature, media_file_count, updated_at)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(library_id, path) DO UPDATE SET
					signature = excluded.signature,
					media_file_count = excluded.media_file_count,
					updated_at = excluded.updated_at`,
				libraryID, path, checkpoint.Signature, checkpoint.MediaFileCount, now); err != nil {
				return err
			}
		}
		if removeUnseen {
			for _, path := range scannerRemovedDirectories(existing, visited) {
				if _, err := tx.Exec(`DELETE FROM library_scan_directories WHERE library_id = ? AND path = ?`, libraryID, path); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *Server) reconcileChangedScanDirectories(ctx context.Context, libraryID, now, scanGeneration string, changed map[string]bool, removed []string) (int, error) {
	if !s.waitForForegroundPressureToEase(ctx) {
		return 0, ctx.Err()
	}
	missing := 0
	err := s.withBackgroundTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		for dir := range changed {
			result, err := tx.Exec(`
				UPDATE media_files
				SET available = 0, missing_since = CASE WHEN missing_since = '' THEN ? ELSE missing_since END
				WHERE library_id = ? AND available = 1 AND directory_path = ? AND scan_generation <> ?`,
				now, libraryID, dir, scanGeneration)
			if err != nil {
				return err
			}
			missing += int(rowsAffected(result))
		}
		for _, dir := range removed {
			prefix := escapeSQLLike(dir+string(filepath.Separator)) + "%"
			result, err := tx.Exec(`
				UPDATE media_files
				SET available = 0, missing_since = CASE WHEN missing_since = '' THEN ? ELSE missing_since END
				WHERE library_id = ? AND available = 1
					AND (directory_path = ? OR directory_path LIKE ? ESCAPE '\')`,
				now, libraryID, dir, prefix)
			if err != nil {
				return err
			}
			missing += int(rowsAffected(result))
		}
		if missing > 0 {
			if _, err := tx.Exec(`
				UPDATE media_items
				SET source_url = COALESCE((
					SELECT path
					FROM media_files
					WHERE media_id = media_items.id AND available = 1
					ORDER BY quality_rank DESC, size_bytes DESC, path ASC
					LIMIT 1
				), '')
				WHERE id IN (SELECT DISTINCT media_id FROM media_files WHERE library_id = ? AND missing_since = ?)`,
				libraryID, now); err != nil {
				return err
			}
		}
		if _, err := pruneOrphanScannedPlayableItems(tx, libraryID); err != nil {
			return err
		}
		return pruneEmptyScannedParents(tx, libraryID)
	})
	return missing, err
}

// markUnchangedScannedPaths is the full-scan fast path. It compares one
// filesystem stat tuple per path against a whole SQLite batch and marks every
// matching media-file row seen with one update. Unchanged files never incur
// content hashing, NFO/tag parsing, artwork discovery, stream replacement,
// search rewrites, provider work, or analysis scheduling.
func (s *Server) markUnchangedScannedPaths(ctx context.Context, libraryID string, candidates []scannerPathCandidate, now, scanGeneration string) (map[string]bool, error) {
	unchanged := map[string]bool{}
	if len(candidates) == 0 {
		return unchanged, nil
	}
	paths := make([]string, 0, len(candidates))
	expected := make(map[string]scannerPathCandidate, len(candidates))
	for _, candidate := range candidates {
		candidate.Path = filepath.Clean(candidate.Path)
		paths = append(paths, candidate.Path)
		expected[candidate.Path] = candidate
	}
	err := s.withBackgroundTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		args := make([]any, 0, len(paths)+1)
		args = append(args, libraryID)
		for _, path := range paths {
			args = append(args, path)
		}
		rows, err := tx.Query(`
			SELECT path, COALESCE(directory_path, ''), size_bytes, mod_time, COALESCE(identity_evidence, '')
			FROM media_files
			WHERE library_id = ? AND available = 1 AND path IN (`+sqlPlaceholders(len(paths))+`)`, args...)
		if err != nil {
			return err
		}
		matched := map[string]bool{}
		for rows.Next() {
			var path, directoryPath, modTime, identityEvidence string
			var size int64
			if err := rows.Scan(&path, &directoryPath, &size, &modTime, &identityEvidence); err != nil {
				rows.Close()
				return err
			}
			candidate, ok := expected[filepath.Clean(path)]
			if !ok {
				continue
			}
			matches := directoryPath == candidate.DirectoryPath && size == candidate.Size && modTime == candidate.ModTime && identityEvidence == "scanner:v2:"+candidate.QuickSignature
			if previous, seen := matched[candidate.Path]; !seen {
				matched[candidate.Path] = matches
			} else {
				matched[candidate.Path] = previous && matches
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for path, matches := range matched {
			if matches {
				unchanged[path] = true
			}
		}
		if len(unchanged) == 0 {
			return nil
		}
		updateArgs := []any{now, scanGeneration, libraryID}
		unchangedPaths := make([]string, 0, len(unchanged))
		for path := range unchanged {
			unchangedPaths = append(unchangedPaths, path)
		}
		sort.Strings(unchangedPaths)
		for _, path := range unchangedPaths {
			updateArgs = append(updateArgs, path)
		}
		_, err = tx.Exec(`
			UPDATE media_files
			SET available = 1, missing_since = '', last_seen_at = ?, scan_generation = ?
			WHERE library_id = ? AND path IN (`+sqlPlaceholders(len(unchangedPaths))+`)`, updateArgs...)
		return err
	})
	return unchanged, err
}

func (s *Server) writeScannedMediaBatch(ctx context.Context, library Library, batch []scannerMediaFile, now string, scanGeneration string, allowMetadata, enqueueAnalysis bool) (map[string]bool, int, int, int, error) {
	batchSeen := map[string]bool{}
	indexed := 0
	metadataQueued := 0
	analysisQueued := 0
	analysisCapabilities := effectiveScanProfile(s.libraryAnalysisSettingsFor(library))
	for index := range batch {
		if err := ctx.Err(); err != nil {
			return batchSeen, indexed, metadataQueued, analysisQueued, err
		}
		if batch[index].FilesystemPrepared {
			continue
		}
		request := s.storageRequestForPath(ctx, foundationcontract.WorkClassBackgroundMedia, batch[index].SourcePath, "scanner sidecar preflight")
		if err := s.boundedStorageIO(ctx, request, func() error {
			prepared, err := s.prepareScannedMediaFilesystem(batch[index])
			if err == nil {
				batch[index] = prepared
			}
			return err
		}); err != nil {
			return batchSeen, indexed, metadataQueued, analysisQueued, err
		}
	}
	metadataProvider := normalizedMetadataProvider(settingString(library.Settings, "metadataProvider", ""))
	enqueueMetadata := allowMetadata && metadataProvider != "local" && metadataProvider != "none"
	subtitleReplacements := make([]scannerSubtitleReplacement, 0, len(batch))
	wasNew := make([]bool, len(batch))
	// Resolve opaque identities and allocate stable public subtitle IDs before
	// publishing files. This short transaction writes only retry-safe identity
	// and minimum hierarchy rows; critically, it does not advance media_files'
	// scan signature, so a publication failure is always retried.
	err := s.withBackgroundTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		attemptBatch := append([]scannerMediaFile(nil), batch...)
		attemptWasNew := make([]bool, len(batch))
		attemptReplacements := make([]scannerSubtitleReplacement, 0, len(batch))
		for index := range attemptBatch {
			if err := ensureRemoteScannerSourceTx(tx, attemptBatch[index]); err != nil {
				return err
			}
			file, err := resolveScannedIdentity(tx, attemptBatch[index], now, scanGeneration)
			if err != nil {
				return err
			}
			file, err = resolveScannedParentIdentities(tx, library, file, now)
			if err != nil {
				return err
			}
			attemptWasNew[index], err = scannedMediaIsNew(tx, file.ID)
			if err != nil {
				return err
			}
			if !attemptWasNew[index] {
				attemptWasNew[index], err = scannedMediaHasNoFiles(tx, file.ID)
				if err != nil {
					return err
				}
			}
			if file.ParentID != "" {
				identityFile := file
				identityFile.LocalNFOEnabled = false
				identityFile.ParentShowMetadata = scannerLocalMetadata{}
				identityFile.ParentSeasonMetadata = scannerLocalMetadata{}
				identityFile.ParentArtistMetadata = scannerLocalMetadata{}
				identityFile.ParentAlbumMetadata = scannerLocalMetadata{}
				if err = upsertScannedParent(tx, library, identityFile, now); err != nil {
					return err
				}
			}
			if err = ensureScannedMediaIdentityTx(tx, file, now); err != nil {
				return err
			}
			replacement, err := s.planScannedSidecarSubtitles(tx, file)
			if err != nil {
				return err
			}
			attemptBatch[index] = file
			attemptReplacements = append(attemptReplacements, replacement)
		}
		batch = attemptBatch
		wasNew = attemptWasNew
		subtitleReplacements = attemptReplacements
		return nil
	})
	if err != nil {
		return batchSeen, indexed, metadataQueued, analysisQueued, scannerCatalogApplyError{err: err}
	}
	if err := ctx.Err(); err != nil {
		return batchSeen, indexed, metadataQueued, analysisQueued, err
	}
	if err := s.publishScannedSidecarSubtitles(subtitleReplacements); err != nil {
		return batchSeen, indexed, metadataQueued, analysisQueued, scannerCatalogApplyError{err: err}
	}
	err = s.withBackgroundTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		localSeen := map[string]bool{}
		localIndexed := 0
		localMetadataQueued := 0
		localAnalysisQueued := 0
		for index := range batch {
			file := batch[index]
			if err := ensureRemoteScannerSourceTx(tx, file); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			isNewMedia := wasNew[index]
			if file.ParentID != "" {
				if err = upsertScannedParent(tx, library, file, now); err != nil {
					return err
				}
			}
			revision, locks, err := applyScannedMetadataRevisionTx(ctx, tx, file, now)
			if err != nil {
				return err
			}
			if err = upsertScannedMediaFile(tx, file, now, scanGeneration); err != nil {
				return err
			}
			if err = s.replaceScannedStreams(tx, file); err != nil {
				return err
			}
			if err = replaceScannedSidecarSubtitleRows(tx, subtitleReplacements[index:index+1]); err != nil {
				return err
			}
			if err = replaceScannedSidecarLyrics(tx, file, now); err != nil {
				return err
			}
			if !locks["artwork"] {
				if err = replaceScannedLocalImages(tx, file, now); err != nil {
					return err
				}
				if _, err = tx.Exec(`UPDATE media_images SET metadata_revision = ? WHERE media_id = ? AND source = 'local' AND provider = 'scanner'`, revision, file.ID); err != nil {
					return err
				}
			}
			if err = replaceMediaSearchTx(ctx, tx, file.ID, revision, firstNonEmpty(file.LocalMetadata.Source, "scanner")); err != nil {
				return err
			}
			if err = replaceMediaCategoryFacetsTx(ctx, tx, file.ID, revision, firstNonEmpty(file.LocalMetadata.Source, "scanner")); err != nil {
				return err
			}
			localSeen[file.FileID] = true
			if enqueueMetadata && shouldQueueScannedMetadataRefresh(tx, isNewMedia, file) {
				if library.Type == "anime" && file.Type == "episode" && file.GrandparentID != "" {
					queued, err := enqueueScannerBacklogTx(tx, library.ID, file.GrandparentID, scannerBacklogMetadata, scanGeneration, now)
					if err != nil {
						return err
					}
					localMetadataQueued += queued
				} else {
					queued, err := enqueueScannerBacklogTx(tx, library.ID, file.ID, scannerBacklogMetadata, scanGeneration, now)
					if err != nil {
						return err
					}
					localMetadataQueued += queued
					if file.ParentID != "" && shouldRefreshScannedParentMetadata(file) {
						queued, err = enqueueScannerBacklogTx(tx, library.ID, file.ParentID, scannerBacklogMetadata, scanGeneration, now)
						if err != nil {
							return err
						}
						localMetadataQueued += queued
					}
					if file.GrandparentID != "" && shouldRefreshScannedGrandparentMetadata(file) {
						queued, err = enqueueScannerBacklogTx(tx, library.ID, file.GrandparentID, scannerBacklogMetadata, scanGeneration, now)
						if err != nil {
							return err
						}
						localMetadataQueued += queued
					}
				}
			}
			if enqueueAnalysis && scannerAnalysisEligibleForProfile(file, analysisCapabilities) {
				revision := scannerAnalysisSourceRevision(file)
				queued, err := enqueueScannerBacklogTx(tx, library.ID, file.ID, scannerBacklogAnalysis, revision, now)
				if err != nil {
					return err
				}
				localAnalysisQueued += queued
			}
			localIndexed++
		}
		batchSeen = localSeen
		indexed = localIndexed
		metadataQueued = localMetadataQueued
		analysisQueued = localAnalysisQueued
		return nil
	})
	if err != nil {
		return batchSeen, indexed, metadataQueued, analysisQueued, scannerCatalogApplyError{err: err}
	}
	return batchSeen, indexed, metadataQueued, analysisQueued, nil
}

func ensureRemoteScannerSourceTx(tx *sql.Tx, file scannerMediaFile) error {
	if file.SourceType != "rclone" && file.SourceType != "webdav" && !strings.HasPrefix(file.SourcePath, "portico-storage://") {
		return nil
	}
	locator, err := url.Parse(file.SourcePath)
	if err != nil || locator.Scheme != "portico-storage" || strings.TrimSpace(locator.Host) == "" {
		return errRemoteStorageSourceRemoved
	}
	var exists int
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM storage_sources WHERE id=? AND backend_kind IN ('rclone','webdav'))`, locator.Host).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return errRemoteStorageSourceRemoved
	}
	return nil
}

func (s *Server) persistScannerInventoryBatch(ctx context.Context, libraryID string, batch []scannerMediaFile, now, scanGeneration string) error {
	if strings.TrimSpace(libraryID) == "" || strings.TrimSpace(scanGeneration) == "" || len(batch) == 0 {
		return nil
	}
	return s.withBackgroundTxTagged(ctx, []string{"scanner_inventory"}, func(tx *sql.Tx) error {
		for _, file := range batch {
			if strings.TrimSpace(file.SourcePath) == "" {
				continue
			}
			if _, err := tx.Exec(`
				INSERT INTO scanner_inventory_entries (
					library_id, path, scan_generation, size_bytes, mod_time, quick_signature,
					media_type, source_type, discovered_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(library_id, path) DO UPDATE SET
					scan_generation=excluded.scan_generation,
					size_bytes=excluded.size_bytes,
					mod_time=excluded.mod_time,
					quick_signature=excluded.quick_signature,
					media_type=excluded.media_type,
					source_type=excluded.source_type,
					updated_at=excluded.updated_at`,
				libraryID, file.SourcePath, scanGeneration, file.FileSize, file.FileModTime,
				file.QuickSignature, file.Type, firstNonEmpty(file.SourceType, "local"), now, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func scannerAnalysisEligible(file scannerMediaFile) bool {
	if strings.TrimSpace(file.ID) == "" || strings.TrimSpace(file.SourcePath) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(file.SourceType)) {
	case "disc-image", "dvd-structure", "bluray-structure", "strm":
		return false
	default:
		return true
	}
}

func scannerAnalysisEligibleForProfile(file scannerMediaFile, capabilities map[string]bool) bool {
	if strings.EqualFold(strings.TrimSpace(file.SourceType), "strm") || isSTRMDescriptor(file.SourcePath) {
		return strings.TrimSpace(file.ID) != "" && strings.TrimSpace(file.SourcePath) != "" && capabilities["analyzeSTRMTarget"]
	}
	return scannerAnalysisEligible(file)
}

func (s *Server) reconcileScannedMedia(ctx context.Context, libraryID string, now string, scanGeneration string) (int, error) {
	missing := 0
	for {
		if err := ctx.Err(); err != nil {
			return missing, err
		}
		marked, err := s.markMissingScannedMediaFilesBatch(ctx, libraryID, now, scanGeneration, scannerReconcileBatchSize)
		if err != nil {
			return missing, err
		}
		missing += marked
		if marked < scannerReconcileBatchSize {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return missing, err
	}
	err := s.withBackgroundTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		if _, err := pruneOrphanScannedPlayableItems(tx, libraryID); err != nil {
			return err
		}
		return pruneEmptyScannedParents(tx, libraryID)
	})
	return missing, err
}

func scannedMediaIsNew(tx *sql.Tx, mediaID string) (bool, error) {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM media_items WHERE id = ?`, mediaID).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

func scannedMediaHasNoFiles(tx *sql.Tx, mediaID string) (bool, error) {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM media_files WHERE media_id = ?`, mediaID).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *Server) automaticMetadataRefreshSupported(item MediaItem) bool {
	switch s.metadataProviderForItem(item) {
	case "tmdb":
		return tmdbSearchType(item.Type) != ""
	case "tvdb":
		return tvdbSearchType(item.Type) != ""
	case "anilist":
		return item.Type == "anime"
	case "musicbrainz":
		return item.Type == "artist" || item.Type == "album" || item.Type == "track"
	default:
		return false
	}
}

func metadataRefreshRecentlyQueued(db *sql.DB, mediaID string, within time.Duration) bool {
	cutoff := time.Now().UTC().Add(-within).Format(time.RFC3339)
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'metadata_refresh' AND resource_type = 'media' AND resource_id = ? AND created_at >= ?`, mediaID, cutoff).Scan(&count)
	return count > 0
}

func libraryJobRecentlyQueued(db *sql.DB, jobType, libraryID string, within time.Duration) bool {
	cutoff := time.Now().UTC().Add(-within).Format(time.RFC3339)
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = ? AND resource_type = 'library' AND resource_id = ? AND created_at >= ?`, jobType, libraryID, cutoff).Scan(&count)
	return count > 0
}

func shouldRefreshScannedMetadata(file scannerMediaFile) bool {
	switch file.Type {
	case "movie", "show", "anime", "episode", "artist", "album", "track", "audiobook":
		return true
	default:
		return false
	}
}

func shouldQueueScannedMetadataRefresh(tx *sql.Tx, isNewMedia bool, file scannerMediaFile) bool {
	if !shouldRefreshScannedMetadata(file) {
		return false
	}
	if isNewMedia {
		return true
	}
	return false
}

func scannerPrimaryMetadataProvider(mediaType string) string {
	switch mediaType {
	case "movie", "show", "episode":
		return "tmdb"
	case "anime":
		return "anilist"
	case "artist", "album", "track":
		return "musicbrainz"
	default:
		return ""
	}
}

func shouldRefreshScannedParentMetadata(file scannerMediaFile) bool {
	switch file.Type {
	case "episode", "track":
		return true
	default:
		return false
	}
}

func shouldRefreshScannedGrandparentMetadata(file scannerMediaFile) bool {
	switch file.Type {
	case "episode", "track":
		return true
	default:
		return false
	}
}

func mediaAnalysisRecentlyQueued(db *sql.DB, mediaID string, within time.Duration) bool {
	cutoff := time.Now().UTC().Add(-within).Format(time.RFC3339)
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze' AND resource_type = 'media' AND resource_id = ? AND created_at >= ?`, mediaID, cutoff).Scan(&count)
	return count > 0
}

type scanRoot struct {
	sourceID       string
	configured     string
	display        string
	real           string
	classification storageSourceClass
}

func resolvedLibraryRoots(paths []string) ([]scanRoot, error) {
	roots := []scanRoot{}
	for _, raw := range paths {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		abs, err := filepath.Abs(raw)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", raw, err)
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			continue
		}
		info, err := os.Stat(real)
		if err != nil || !info.IsDir() {
			continue
		}
		roots = append(roots, scanRoot{display: abs, real: filepath.Clean(real)})
	}
	return roots, nil
}

func scannerFileForPath(library Library, root, path string, localNFOEnabled, allowMediaContentRead bool) scannerMediaFile {
	if logical, ok := logicalDiscSourceForPath(library, root, path); ok {
		return logical
	}
	rel, _ := filepath.Rel(root, path)
	ext := strings.ToLower(filepath.Ext(path))
	sourceType := ""
	if strmExtensions[ext] {
		sourceType = "strm"
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	info, _ := os.Stat(path)
	contentFingerprint := ""
	// STRM is a private locator descriptor, not media content. Scans derive its
	// identity from the local name/stat revision and only an authorized playback
	// request may open it. Higher scan tiers therefore do not fingerprint it.
	if allowMediaContentRead && sourceType != "strm" {
		contentFingerprint = scannerContentFingerprint(path, info)
	}
	version := parseMediaVersionInfo(path)
	id := scannedID("scan", filepath.Join(library.ID, cleanMediaTitle(base)))
	mediaType := library.Type
	title := cleanMediaTitle(base)
	parentID := ""
	seasonNumber := 0
	episodeNumber := 0
	indexNumber := 0
	localMode := localMetadataModeForLibrary(library)
	local := scannerLocalMetadata{}
	if localNFOEnabled {
		local = localMetadataForMediaFile(path, root)
	}
	local = scannerMetadataForLocalMode(local, localMode)

	switch library.Type {
	case "show", "anime":
		mediaType = "episode"
		episodeInfo := parseEpisodeInfo(library.Type, rel, base)
		seasonNumber = episodeInfo.Season
		episodeNumber = episodeInfo.Episode
		if episodeInfo.Year > 0 && local.Year == 0 {
			local.Year = episodeInfo.Year
		}
		indexNumber = episodeNumber
		showTitle := showTitleFromPath(rel, base)
		showScannerKey := scannedID("scan_show", filepath.Join(library.ID, showTitle))
		seasonScannerKey := scannedID("scan_season", filepath.Join(showScannerKey, strconv.Itoa(seasonNumber)))
		parentTitle := fmt.Sprintf("Season %d", seasonNumber)
		title = episodeTitleForShow(base, episodeInfo, showTitle)
		episodeIdentity := strconv.Itoa(episodeNumber)
		if episodeNumber == 0 {
			episodeIdentity = "unmatched:" + strings.ToLower(cleanMediaTitle(base))
		}
		id = scannedID("scan_episode", filepath.Join(library.ID, showTitle, strconv.Itoa(seasonNumber), episodeIdentity))
		return scannerMediaFile{ID: id, FileID: scannerFileID(id, path), LibraryID: library.ID, ParentScannerKey: seasonScannerKey, ParentTitle: parentTitle, GrandparentScannerKey: showScannerKey, GrandparentTitle: showTitle, Type: mediaType, Title: title, SortTitle: sortableTitle(title), SeasonNumber: seasonNumber, EpisodeNumber: episodeNumber, EpisodeEnd: episodeInfo.EpisodeEnd, IndexNumber: indexNumber, SourcePath: path, ScanRoot: root, SourceType: sourceType, FileSize: fileSize(info), FileModTime: fileModTime(info), ContentFingerprint: contentFingerprint, Version: version, ArtSeed: strings.TrimPrefix(ext, "."), LocalMetadata: local, LocalNFOEnabled: localNFOEnabled}
	case "movie":
		if local.Year == 0 {
			local.Year = movieYearFromName(base)
		}
		if local.Edition == "" {
			local.Edition = movieEditionFromName(base)
		}
		if extraKind, parentTitle, ok := extraInfoFromPath(rel); ok {
			mediaType = "extra"
			parentScannerKey := scannedID("scan_movie", movieScannerIdentity(library.ID, cleanMediaTitle(parentTitle), local.Year))
			indexNumber = extraSortOrder(extraKind)
			id = scannedID("scan_extra", filepath.Join(library.ID, parentTitle, extraKind, title))
			return scannerMediaFile{ID: id, FileID: scannerFileID(id, path), LibraryID: library.ID, ParentScannerKey: parentScannerKey, Type: mediaType, Title: title, SortTitle: sortableTitle(title), IndexNumber: indexNumber, SourcePath: path, ScanRoot: root, SourceType: sourceType, FileSize: fileSize(info), FileModTime: fileModTime(info), ContentFingerprint: contentFingerprint, Version: version, ArtSeed: extraKind, ExtraKind: extraKind, LocalMetadata: local, LocalNFOEnabled: localNFOEnabled}
		}
		id = scannedID("scan_movie", movieScannerIdentity(library.ID, title, local.Year))
	case "music":
		mediaType = "track"
		albumTitle := albumTitleFromPath(rel)
		artist := artistTitleFromPath(rel)
		artistScannerKey := scannedID("scan_artist", filepath.Join(library.ID, artist))
		albumScannerKey := scannedID("scan_album", filepath.Join(artistScannerKey, albumTitle))
		indexNumber = trackNumberFromName(base)
		id = scannedID("scan_track", filepath.Join(library.ID, artist, albumTitle, strconv.Itoa(indexNumber), title))
		return scannerMediaFile{ID: id, FileID: scannerFileID(id, path), LibraryID: library.ID, ParentScannerKey: albumScannerKey, ParentTitle: albumTitle, GrandparentScannerKey: artistScannerKey, GrandparentTitle: artist, Type: mediaType, Title: title, SortTitle: sortableTitle(title), Artist: artist, IndexNumber: indexNumber, SourcePath: path, ScanRoot: root, SourceType: sourceType, FileSize: fileSize(info), FileModTime: fileModTime(info), ContentFingerprint: contentFingerprint, Version: version, ArtSeed: strings.TrimPrefix(ext, "."), LocalMetadata: local, LocalNFOEnabled: localNFOEnabled}
	case "audiobook":
		if allowMediaContentRead {
			local = scannerMetadataForLocalMode(mergeScannerMetadata(local, audiobookLocalMetadataForFile(path, root)), localMode)
		}
		mediaType = "audiobook"
		folderAuthor, folderTitle := audiobookAuthorTitleFromPath(rel, title)
		if local.Artist == "" {
			local.Artist = folderAuthor
		}
		if local.Title != "" {
			title = local.Title
		} else {
			title = folderTitle
		}
		indexNumber = trackNumberFromName(base)
		id = scannedID("scan_audiobook", filepath.Join(library.ID, firstNonEmpty(local.Artist, folderAuthor), title))
	}

	return scannerMediaFile{ID: id, FileID: scannerFileID(id, path), LibraryID: library.ID, ParentID: parentID, Type: mediaType, Title: title, SortTitle: sortableTitle(title), IndexNumber: indexNumber, SourcePath: path, ScanRoot: root, SourceType: sourceType, FileSize: fileSize(info), FileModTime: fileModTime(info), ContentFingerprint: contentFingerprint, Version: version, ArtSeed: strings.TrimPrefix(ext, "."), LocalMetadata: local, LocalNFOEnabled: localNFOEnabled}
}

// scannerContentFingerprint is deliberately independent of a file's name and
// location. Small files are hashed completely; large files use bounded samples
// from both ends plus the byte length. A unique match is strong move evidence,
// while duplicate matches are sent to reconciliation review.
func scannerContentFingerprint(path string, info os.FileInfo) string {
	if info == nil || !info.Mode().IsRegular() || info.Size() < 0 {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	hash := sha256.New()
	_, _ = io.WriteString(hash, strconv.FormatInt(info.Size(), 10))
	_, _ = io.WriteString(hash, "\x00")
	const sampleSize int64 = 128 * 1024
	if info.Size() <= sampleSize*2 {
		if _, err := io.Copy(hash, file); err != nil {
			return ""
		}
	} else {
		if _, err := io.CopyN(hash, file, sampleSize); err != nil {
			return ""
		}
		if _, err := file.Seek(-sampleSize, io.SeekEnd); err != nil {
			return ""
		}
		if _, err := io.CopyN(hash, file, sampleSize); err != nil {
			return ""
		}
	}
	return "sha256-sampled:" + hex.EncodeToString(hash.Sum(nil))
}

func resolveScannedIdentity(tx *sql.Tx, file scannerMediaFile, now, scanGeneration string) (scannerMediaFile, error) {
	scannerKey := file.ID
	sourceType := strings.TrimSpace(file.SourceType)
	if sourceType == "" {
		sourceType = "local"
	}
	var existingFileID, existingMediaID string
	exactQuery := `
		SELECT f.id, f.media_id
		FROM media_files f
		JOIN media_items m ON m.id = f.media_id
		WHERE f.library_id = ? AND f.path = ? AND m.type = ?`
	exactArgs := []any{file.LibraryID, file.SourcePath, file.Type}
	if file.Type == "episode" {
		exactQuery += ` AND m.season_number = ? AND m.episode_number = ?`
		exactArgs = append(exactArgs, file.SeasonNumber, file.EpisodeNumber)
	} else if file.Type == "track" {
		exactQuery += ` AND m.index_number = ?`
		exactArgs = append(exactArgs, file.IndexNumber)
	}
	exactQuery += ` ORDER BY f.last_seen_at DESC, f.id LIMIT 1`
	err := tx.QueryRow(exactQuery, exactArgs...).Scan(&existingFileID, &existingMediaID)
	if err == nil {
		file.ID = existingMediaID
		file.FileID = existingFileID
		if err := upsertScannerIdentityAlias(tx, file.LibraryID, scannerKey, file.ID, now); err != nil {
			return file, err
		}
		return file, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return file, err
	}

	// The scanner key is a mutable discovery alias, never the resource ID. It
	// groups multiple versions or audiobook parts discovered in the same scan.
	// A rename may change the alias, in which case content evidence below takes
	// over and teaches the new alias about the retained Portico identity.
	var aliasedMediaID string
	err = tx.QueryRow(`
		SELECT a.media_id
		FROM media_scanner_identity_aliases a
		JOIN media_items m ON m.id = a.media_id
		WHERE a.library_id = ? AND a.scanner_key = ? AND m.type = ?
		LIMIT 1`, file.LibraryID, scannerKey, file.Type).Scan(&aliasedMediaID)
	if err == nil {
		file.ID = aliasedMediaID
		file.FileID = randomOpaquePublicID()
		if err := upsertScannerIdentityAlias(tx, file.LibraryID, scannerKey, file.ID, now); err != nil {
			return file, err
		}
		return file, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return file, err
	}

	if file.ContentFingerprint != "" {
		query := `
			SELECT f.id, f.media_id
			FROM media_files f
			JOIN media_items m ON m.id = f.media_id
			WHERE f.library_id = ?
			  AND f.source_type = ?
			  AND f.content_fingerprint = ?
			  AND f.scan_generation <> ?
			  AND m.type = ?`
		args := []any{file.LibraryID, sourceType, file.ContentFingerprint, scanGeneration, file.Type}
		if file.Type == "episode" {
			query += ` AND m.season_number = ? AND m.episode_number = ?`
			args = append(args, file.SeasonNumber, file.EpisodeNumber)
		} else if file.Type == "track" {
			query += ` AND m.index_number = ?`
			args = append(args, file.IndexNumber)
		}
		query += ` ORDER BY f.last_seen_at DESC, f.id LIMIT 3`
		rows, queryErr := tx.Query(query, args...)
		if queryErr != nil {
			return file, queryErr
		}
		type candidate struct{ fileID, mediaID string }
		candidates := []candidate{}
		for rows.Next() {
			var candidate candidate
			if err := rows.Scan(&candidate.fileID, &candidate.mediaID); err != nil {
				rows.Close()
				return file, err
			}
			candidates = append(candidates, candidate)
		}
		if err := rows.Close(); err != nil {
			return file, err
		}
		if len(candidates) == 1 {
			file.ID = candidates[0].mediaID
			file.FileID = candidates[0].fileID
			if err := upsertScannerIdentityAlias(tx, file.LibraryID, scannerKey, file.ID, now); err != nil {
				return file, err
			}
			return file, nil
		}
		if len(candidates) > 1 {
			candidateIDs := make([]string, 0, len(candidates))
			for _, candidate := range candidates {
				candidateIDs = append(candidateIDs, candidate.mediaID)
			}
			encoded, _ := json.Marshal(candidateIDs)
			file.ID = randomOpaqueMediaID()
			file.FileID = randomOpaquePublicID()
			if _, err := tx.Exec(`
				INSERT INTO identity_reconciliation_reviews (
					id, domain, library_or_source_id, subject_id, candidate_locator, evidence_kind,
					evidence_value, candidate_ids_json, created_at
				) VALUES (?, 'media', ?, ?, ?, 'content_fingerprint_ambiguous', ?, ?, ?)`,
				randomID("idrev"), file.LibraryID, file.ID, file.SourcePath, file.ContentFingerprint, string(encoded), now); err != nil {
				return file, err
			}
			if err := upsertScannerIdentityAlias(tx, file.LibraryID, scannerKey, file.ID, now); err != nil {
				return file, err
			}
			return file, nil
		}
	}

	file.ID = randomOpaqueMediaID()
	file.FileID = randomOpaquePublicID()
	if err := upsertScannerIdentityAlias(tx, file.LibraryID, scannerKey, file.ID, now); err != nil {
		return file, err
	}
	return file, nil
}

// resolveScannedParentIdentities turns mutable scanner hierarchy keys into
// opaque Portico-owned media IDs. Exact aliases and the already-resolved
// playable item's current lineage win first; provider IDs are used only when
// they identify one eligible parent. Ambiguity is durable review work, never a
// silent merge.
func resolveScannedParentIdentities(tx *sql.Tx, library Library, file scannerMediaFile, now string) (scannerMediaFile, error) {
	switch file.Type {
	case "episode":
		showKey := firstNonEmpty(file.GrandparentScannerKey, scannedID("scan_show", filepath.Join(file.LibraryID, file.GrandparentTitle)))
		seasonKey := firstNonEmpty(file.ParentScannerKey, scannedID("scan_season", filepath.Join(showKey, strconv.Itoa(file.SeasonNumber))))
		if seasonID, showID, ok, err := existingEpisodeHierarchy(tx, file.ID, file.SeasonNumber); err != nil {
			return file, err
		} else if ok {
			file.ParentID, file.GrandparentID = seasonID, showID
			if err := recordResolvedParentAliases(tx, file.LibraryID, showKey, showID, library.Type, file.ParentShowMetadata, now); err != nil {
				return file, err
			}
			if err := recordResolvedParentAliases(tx, file.LibraryID, seasonKey, seasonID, "season", file.ParentSeasonMetadata, now); err != nil {
				return file, err
			}
			return file, nil
		}
		showID, err := resolveScannedParentIdentity(tx, file.LibraryID, "", library.Type, showKey, file.SourcePath, file.ParentShowMetadata, now)
		if err != nil {
			return file, err
		}
		seasonID, err := resolveScannedParentIdentity(tx, file.LibraryID, showID, "season", seasonKey, file.SourcePath, file.ParentSeasonMetadata, now)
		if err != nil {
			return file, err
		}
		file.ParentID, file.GrandparentID = seasonID, showID
	case "track":
		artistKey := firstNonEmpty(file.GrandparentScannerKey, scannedID("scan_artist", filepath.Join(file.LibraryID, file.GrandparentTitle)))
		albumKey := firstNonEmpty(file.ParentScannerKey, scannedID("scan_album", filepath.Join(artistKey, file.ParentTitle)))
		if albumID, artistID, ok, err := existingTrackHierarchy(tx, file.ID); err != nil {
			return file, err
		} else if ok {
			file.ParentID, file.GrandparentID = albumID, artistID
			if err := recordResolvedParentAliases(tx, file.LibraryID, artistKey, artistID, "artist", file.ParentArtistMetadata, now); err != nil {
				return file, err
			}
			if err := recordResolvedParentAliases(tx, file.LibraryID, albumKey, albumID, "album", file.ParentAlbumMetadata, now); err != nil {
				return file, err
			}
			return file, nil
		}
		artistID, err := resolveScannedParentIdentity(tx, file.LibraryID, "", "artist", artistKey, file.SourcePath, file.ParentArtistMetadata, now)
		if err != nil {
			return file, err
		}
		albumID, err := resolveScannedParentIdentity(tx, file.LibraryID, artistID, "album", albumKey, file.SourcePath, file.ParentAlbumMetadata, now)
		if err != nil {
			return file, err
		}
		file.ParentID, file.GrandparentID = albumID, artistID
	case "extra":
		movieKey := firstNonEmpty(file.ParentScannerKey, scannedID("scan_movie", filepath.Join(file.LibraryID, cleanMediaTitle(file.ParentTitle))))
		if parentID, ok, err := existingDirectParent(tx, file.ID, "movie"); err != nil {
			return file, err
		} else if ok {
			file.ParentID = parentID
			if err := recordResolvedParentAliases(tx, file.LibraryID, movieKey, parentID, "movie", scannerLocalMetadata{}, now); err != nil {
				return file, err
			}
			return file, nil
		}
		parentID, err := resolveScannedParentIdentity(tx, file.LibraryID, "", "movie", movieKey, file.SourcePath, scannerLocalMetadata{}, now)
		if err != nil {
			return file, err
		}
		file.ParentID = parentID
	}
	return file, nil
}

func existingEpisodeHierarchy(tx *sql.Tx, mediaID string, seasonNumber int) (string, string, bool, error) {
	var seasonID, showID string
	var storedSeason int
	err := tx.QueryRow(`
		SELECT season.id, show.id, season.season_number
		FROM media_items episode
		JOIN media_items season ON season.id = episode.parent_id AND season.type = 'season'
		JOIN media_items show ON show.id = season.parent_id AND show.type IN ('show', 'anime')
		WHERE episode.id = ? AND episode.type = 'episode'`, mediaID).Scan(&seasonID, &showID, &storedSeason)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if storedSeason != seasonNumber {
		return "", "", false, nil
	}
	return seasonID, showID, true, nil
}

func existingTrackHierarchy(tx *sql.Tx, mediaID string) (string, string, bool, error) {
	var albumID, artistID string
	err := tx.QueryRow(`
		SELECT album.id, artist.id
		FROM media_items track
		JOIN media_items album ON album.id = track.parent_id AND album.type = 'album'
		JOIN media_items artist ON artist.id = album.parent_id AND artist.type = 'artist'
		WHERE track.id = ? AND track.type = 'track'`, mediaID).Scan(&albumID, &artistID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	return albumID, artistID, err == nil, err
}

func existingDirectParent(tx *sql.Tx, mediaID, parentType string) (string, bool, error) {
	var parentID string
	err := tx.QueryRow(`
		SELECT parent.id
		FROM media_items item
		JOIN media_items parent ON parent.id = item.parent_id AND parent.type = ?
		WHERE item.id = ?`, parentType, mediaID).Scan(&parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return parentID, err == nil, err
}

func resolveScannedParentIdentity(tx *sql.Tx, libraryID, parentID, mediaType, scannerKey, locator string, metadata scannerLocalMetadata, now string) (string, error) {
	aliasKey := scannerParentAliasKey(mediaType, scannerKey)
	var existingID string
	err := tx.QueryRow(`
		SELECT alias.media_id
		FROM media_scanner_identity_aliases alias
		JOIN media_items item ON item.id = alias.media_id
		WHERE alias.library_id = ? AND alias.scanner_key = ? AND item.library_id = ? AND item.type = ?
		  AND (? = '' OR item.parent_id = ?)
		LIMIT 1`, libraryID, aliasKey, libraryID, mediaType, parentID, parentID).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	candidateSet := map[string]bool{}
	providerAliases := scannerParentProviderAliases(mediaType, metadata)
	for _, providerAlias := range providerAliases {
		rows, queryErr := tx.Query(`
			SELECT DISTINCT item.id
			FROM media_provider_ids provider_id
			JOIN media_items item ON item.id = provider_id.media_id
			WHERE item.library_id = ? AND item.type = ? AND (? = '' OR item.parent_id = ?)
			  AND provider_id.status = 'accepted'
			  AND provider_id.provider = ? AND provider_id.external_id = ?
			  AND (? = '' OR provider_id.external_type = ?)
			LIMIT 3`, libraryID, mediaType, parentID, parentID, providerAlias.Provider, providerAlias.ExternalID, providerAlias.ExternalType, providerAlias.ExternalType)
		if queryErr != nil {
			return "", queryErr
		}
		for rows.Next() {
			var candidateID string
			if err := rows.Scan(&candidateID); err != nil {
				rows.Close()
				return "", err
			}
			candidateSet[candidateID] = true
		}
		if err := rows.Close(); err != nil {
			return "", err
		}
	}

	candidateIDs := make([]string, 0, len(candidateSet))
	for candidateID := range candidateSet {
		candidateIDs = append(candidateIDs, candidateID)
	}
	sort.Strings(candidateIDs)
	resolvedID := ""
	if len(candidateIDs) == 1 {
		resolvedID = candidateIDs[0]
	} else {
		resolvedID = randomOpaqueMediaID()
		if len(candidateIDs) > 1 {
			encodedCandidates, _ := json.Marshal(candidateIDs)
			encodedEvidence, _ := json.Marshal(providerAliases)
			if _, err := tx.Exec(`
				INSERT INTO identity_reconciliation_reviews (
					id, domain, library_or_source_id, subject_id, candidate_locator, evidence_kind,
					evidence_value, candidate_ids_json, created_at
				) VALUES (?, 'media_parent', ?, ?, ?, 'provider_identity_ambiguous', ?, ?, ?)`,
				randomID("idrev"), libraryID, resolvedID, locator, string(encodedEvidence), string(encodedCandidates), now); err != nil {
				return "", err
			}
		}
	}
	if err := upsertScannerIdentityAlias(tx, libraryID, aliasKey, resolvedID, now); err != nil {
		return "", err
	}
	if len(candidateIDs) <= 1 {
		for _, providerAlias := range providerAliases {
			if err := upsertScannerIdentityAlias(tx, libraryID, providerAlias.AliasKey, resolvedID, now); err != nil {
				return "", err
			}
		}
	}
	return resolvedID, nil
}

type scannerParentProviderAlias struct {
	Provider     string `json:"provider"`
	ExternalID   string `json:"externalId"`
	ExternalType string `json:"externalType,omitempty"`
	AliasKey     string `json:"-"`
}

func scannerParentProviderAliases(mediaType string, metadata scannerLocalMetadata) []scannerParentProviderAlias {
	aliases := []scannerParentProviderAlias{}
	keys := make([]string, 0, len(metadata.ProviderIDs))
	for key := range metadata.ProviderIDs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		provider, externalType := scannerProviderIDTarget(key, mediaType)
		externalID := strings.TrimSpace(metadata.ProviderIDs[key])
		if provider == "" || externalID == "" {
			continue
		}
		aliases = append(aliases, scannerParentProviderAlias{
			Provider: provider, ExternalID: externalID, ExternalType: externalType,
			AliasKey: strings.Join([]string{"parent-provider", mediaType, provider, externalType, externalID}, ":"),
		})
	}
	return aliases
}

func scannerParentAliasKey(mediaType, scannerKey string) string {
	return "parent-scanner:" + mediaType + ":" + strings.TrimSpace(scannerKey)
}

func recordResolvedParentAliases(tx *sql.Tx, libraryID, scannerKey, mediaID, mediaType string, metadata scannerLocalMetadata, now string) error {
	if err := upsertScannerIdentityAlias(tx, libraryID, scannerParentAliasKey(mediaType, scannerKey), mediaID, now); err != nil {
		return err
	}
	for _, providerAlias := range scannerParentProviderAliases(mediaType, metadata) {
		if err := upsertScannerIdentityAlias(tx, libraryID, providerAlias.AliasKey, mediaID, now); err != nil {
			return err
		}
	}
	return nil
}

func upsertScannerIdentityAlias(tx *sql.Tx, libraryID, scannerKey, mediaID, now string) error {
	if strings.TrimSpace(scannerKey) == "" || strings.TrimSpace(mediaID) == "" {
		return nil
	}
	_, err := tx.Exec(`
		INSERT INTO media_scanner_identity_aliases (library_id, scanner_key, media_id, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(library_id, scanner_key) DO UPDATE SET
			media_id = excluded.media_id,
			last_seen_at = excluded.last_seen_at`, libraryID, scannerKey, mediaID, now, now)
	return err
}

func scannerFileID(mediaID, path string) string {
	return randomOpaquePublicID()
}

func localMetadataModeForLibrary(library Library) string {
	mode := strings.ToLower(strings.TrimSpace(settingString(library.Settings, "localMetadataMode", "")))
	if mode == "" {
		return "prefer"
	}
	switch mode {
	case "off", "disabled", "none":
		return "off"
	case "supplement", "supplemental":
		return "supplement"
	case "prefer", "override", "prefer_local":
		return "prefer"
	default:
		return "prefer"
	}
}

func scannerMetadataForLocalMode(metadata scannerLocalMetadata, mode string) scannerLocalMetadata {
	switch mode {
	case "off":
		return scannerLocalMetadata{}
	case "supplement":
		return scannerLocalMetadata{
			ProviderIDs: metadata.ProviderIDs,
			ImagePaths:  metadata.ImagePaths,
			People:      metadata.People,
			Source:      metadata.Source,
		}
	default:
		return metadata
	}
}

func expandMultiEpisodeScannedFile(file scannerMediaFile) []scannerMediaFile {
	if file.Type != "episode" || file.EpisodeEnd <= file.EpisodeNumber || file.EpisodeEnd-file.EpisodeNumber > 12 {
		return []scannerMediaFile{file}
	}
	episodes := make([]scannerMediaFile, 0, file.EpisodeEnd-file.EpisodeNumber+1)
	for episode := file.EpisodeNumber; episode <= file.EpisodeEnd; episode++ {
		next := file
		next.EpisodeNumber = episode
		next.IndexNumber = episode
		next.ID = scannedID("scan_episode", filepath.Join(file.LibraryID, file.GrandparentTitle, strconv.Itoa(file.SeasonNumber), strconv.Itoa(episode)))
		next.FileID = scannerFileID(next.ID, next.SourcePath)
		episodes = append(episodes, next)
	}
	return episodes
}

func (s *Server) enrichScannedMusicFile(ctx context.Context, file scannerMediaFile, library Library) scannerMediaFile {
	return s.enrichScannedMusicFileWithSettings(ctx, file, library, s.metadataAgentSettings())
}

func (s *Server) enrichScannedMusicFileWithSettings(ctx context.Context, file scannerMediaFile, library Library, metadataSettings metadataAgentSettings) scannerMediaFile {
	if file.Type != "track" || strings.TrimSpace(file.SourcePath) == "" {
		return file
	}
	localMode := localMetadataModeForLibrary(library)
	if localMode != "off" && metadataSettings.EmbeddedTags {
		if metadata, ok := s.embeddedMusicMetadata(ctx, file.SourcePath); ok {
			metadata = scannerMetadataForLocalMode(metadata, localMode)
			if !settingBool(library.Settings, "preferEmbeddedTitles", true) {
				metadata.Title = ""
				metadata.SortTitle = ""
			}
			file.LocalMetadata = mergeScannerMetadata(file.LocalMetadata, metadata)
		}
	}
	albumTitle := firstNonEmpty(file.LocalMetadata.AlbumTitle, file.ParentTitle, cleanMediaTitle(filepath.Base(filepath.Dir(file.SourcePath))))
	albumArtist := firstNonEmpty(file.LocalMetadata.AlbumArtist, file.LocalMetadata.Artist, file.LocalMetadata.Studio, file.Artist, file.GrandparentTitle, "Unknown Artist")
	trackArtist := firstNonEmpty(file.LocalMetadata.Artist, file.LocalMetadata.Studio, albumArtist)
	title := firstNonEmpty(file.LocalMetadata.Title, file.Title)
	indexNumber := firstPositiveInt(file.LocalMetadata.TrackNumber, file.IndexNumber)

	file.Title = title
	file.SortTitle = sortableTitle(title)
	file.ParentTitle = albumTitle
	file.GrandparentTitle = albumArtist
	file.Artist = albumArtist
	file.IndexNumber = indexNumber
	file.LocalMetadata.Studio = trackArtist
	file.GrandparentScannerKey = scannedID("scan_artist", filepath.Join(file.LibraryID, albumArtist))
	file.ParentScannerKey = scannedID("scan_album", filepath.Join(file.GrandparentScannerKey, albumTitle))
	file.GrandparentID = ""
	file.ParentID = ""
	file.ID = scannedID("scan_track", filepath.Join(file.LibraryID, albumArtist, albumTitle, strconv.Itoa(indexNumber), title))
	return file
}

func (s *Server) embeddedMusicMetadata(ctx context.Context, path string) (scannerLocalMetadata, bool) {
	ffprobePath := strings.TrimSpace(s.cfg.FFprobePath)
	if ffprobePath == "" {
		return scannerLocalMetadata{}, false
	}
	if _, err := exec.LookPath(ffprobePath); err != nil && filepath.Base(ffprobePath) == ffprobePath {
		return scannerLocalMetadata{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-protocol_whitelist", "file,pipe",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	output, err := managedCommandOutput(ctx, cmd)
	if err != nil {
		return scannerLocalMetadata{}, false
	}
	var payload ffprobePayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return scannerLocalMetadata{}, false
	}
	metadata := musicMetadataFromFFprobe(payload)
	if metadata.Title == "" && metadata.Artist == "" && metadata.AlbumTitle == "" && metadata.TrackNumber == 0 && len(metadata.Genres) == 0 {
		return scannerLocalMetadata{}, false
	}
	metadata.Source = "embedded:" + path
	return metadata, true
}

func mergeScannerMetadata(base scannerLocalMetadata, override scannerLocalMetadata) scannerLocalMetadata {
	if override.Title != "" {
		base.Title = override.Title
		base.SortTitle = firstNonEmpty(override.SortTitle, sortableTitle(override.Title))
	}
	if override.OriginalTitle != "" {
		base.OriginalTitle = override.OriginalTitle
	}
	if override.LocalTitle != "" {
		base.LocalTitle = override.LocalTitle
	}
	if override.ShowTitle != "" {
		base.ShowTitle = override.ShowTitle
	}
	if override.ExactDate != "" {
		base.ExactDate = override.ExactDate
	}
	if override.SeasonNumber > 0 {
		base.SeasonNumber = override.SeasonNumber
	}
	if override.EpisodeNumber > 0 {
		base.EpisodeNumber = override.EpisodeNumber
	}
	if override.RuntimeMinutes > 0 {
		base.RuntimeMinutes = override.RuntimeMinutes
	}
	if override.Collection != "" {
		base.Collection = override.Collection
	}
	if override.RatingName != "" {
		base.RatingName = override.RatingName
	}
	if override.RatingVotes > 0 {
		base.RatingVotes = override.RatingVotes
	}
	if override.Year > 0 {
		base.Year = override.Year
	}
	if override.Summary != "" {
		base.Summary = override.Summary
	}
	if override.Tagline != "" {
		base.Tagline = override.Tagline
	}
	if override.ContentRating != "" {
		base.ContentRating = override.ContentRating
	}
	if override.CommunityRating > 0 {
		base.CommunityRating = override.CommunityRating
	}
	if override.CriticRating > 0 {
		base.CriticRating = override.CriticRating
	}
	if override.Studio != "" {
		base.Studio = override.Studio
	}
	if len(override.Studios) > 0 {
		base.Studios = override.Studios
	}
	if override.Network != "" {
		base.Network = override.Network
	}
	if override.Country != "" {
		base.Country = override.Country
	}
	if len(override.Countries) > 0 {
		base.Countries = override.Countries
	}
	if len(override.Genres) > 0 {
		base.Genres = override.Genres
	}
	if len(override.Tags) > 0 {
		base.Tags = override.Tags
	}
	if override.Artist != "" {
		base.Artist = override.Artist
	}
	if override.AlbumArtist != "" {
		base.AlbumArtist = override.AlbumArtist
	}
	if override.AlbumTitle != "" {
		base.AlbumTitle = override.AlbumTitle
	}
	if override.Series != "" {
		base.Series = override.Series
	}
	if override.SeriesIndex != "" {
		base.SeriesIndex = override.SeriesIndex
	}
	if override.AuthorProvider != "" {
		base.AuthorProvider = override.AuthorProvider
	}
	if override.AuthorID != "" {
		base.AuthorID = override.AuthorID
	}
	if override.SeriesProvider != "" {
		base.SeriesProvider = override.SeriesProvider
	}
	if override.SeriesID != "" {
		base.SeriesID = override.SeriesID
	}
	if override.Label != "" {
		base.Label = override.Label
	}
	if override.Publisher != "" {
		base.Publisher = override.Publisher
	}
	if override.ReleaseCountry != "" {
		base.ReleaseCountry = override.ReleaseCountry
	}
	if override.TrackNumber > 0 {
		base.TrackNumber = override.TrackNumber
	}
	if override.TrackCount > 0 {
		base.TrackCount = override.TrackCount
	}
	if override.DiscNumber > 0 {
		base.DiscNumber = override.DiscNumber
	}
	if override.DiscCount > 0 {
		base.DiscCount = override.DiscCount
	}
	if override.BPM > 0 {
		base.BPM = override.BPM
	}
	if override.Explicit != "" {
		base.Explicit = override.Explicit
	}
	if override.Source != "" {
		base.Source = override.Source
	}
	if len(override.ProviderIDs) > 0 {
		if base.ProviderIDs == nil {
			base.ProviderIDs = map[string]string{}
		}
		for provider, id := range override.ProviderIDs {
			base.ProviderIDs[provider] = id
		}
	}
	if len(override.TypedMetadata) > 0 {
		if base.TypedMetadata == nil {
			base.TypedMetadata = map[string]string{}
		}
		for key, value := range override.TypedMetadata {
			base.TypedMetadata[key] = value
		}
	}
	if len(override.ImagePaths) > 0 {
		if base.ImagePaths == nil {
			base.ImagePaths = map[string]string{}
		}
		for kind, path := range override.ImagePaths {
			kind = strings.TrimSpace(kind)
			path = strings.TrimSpace(path)
			if kind != "" && path != "" {
				base.ImagePaths[kind] = path
			}
		}
	}
	if len(override.People) > 0 {
		base.People = append(base.People, override.People...)
	}
	return base
}

func fileSize(info os.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.Size()
}

func fileModTime(info os.FileInfo) string {
	if info == nil {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339)
}

func upsertScannedParent(tx *sql.Tx, library Library, file scannerMediaFile, now string) error {
	localMode := localMetadataModeForLibrary(library)
	switch file.Type {
	case "episode":
		showID := file.GrandparentID
		showTitle := file.GrandparentTitle
		if showTitle == "" {
			showTitle = showTitleFromPath(file.SourcePath, file.Title)
		}
		if showID == "" {
			showID = randomOpaqueMediaID()
		}
		if err := upsertParentRow(tx, showID, library.ID, "", library.Type, showTitle, "", 0, now, scannerMetadataForLocalMode(nfoMetadataForParent(file.LocalNFOEnabled, file.ParentShowMetadata), localMode)); err != nil {
			return err
		}
		seasonTitle := file.ParentTitle
		if seasonTitle == "" {
			seasonTitle = fmt.Sprintf("Season %d", file.SeasonNumber)
		}
		seasonID := file.ParentID
		if seasonID == "" {
			seasonID = randomOpaqueMediaID()
		}
		return upsertParentRow(tx, seasonID, library.ID, showID, "season", seasonTitle, "", file.SeasonNumber, now, scannerMetadataForLocalMode(nfoMetadataForParent(file.LocalNFOEnabled, file.ParentSeasonMetadata), localMode))
	case "extra":
		parentTitle := cleanMediaTitle(filepath.Base(filepath.Dir(filepath.Dir(file.SourcePath))))
		if parentTitle == "Untitled" {
			parentTitle = library.Name
		}
		parentID := file.ParentID
		if parentID == "" {
			parentID = randomOpaqueMediaID()
		}
		return upsertParentRow(tx, parentID, library.ID, "", "movie", parentTitle, "", 0, now)
	case "track":
		albumTitle := file.ParentTitle
		if albumTitle == "" {
			albumTitle = cleanMediaTitle(filepath.Base(filepath.Dir(file.SourcePath)))
		}
		artistID := file.GrandparentID
		artistTitle := file.GrandparentTitle
		if artistTitle == "" {
			artistTitle = file.Artist
			if artistTitle == "" {
				artistTitle = "Unknown Artist"
			}
		}
		if artistID == "" {
			artistID = randomOpaqueMediaID()
		}
		if err := upsertParentRow(tx, artistID, library.ID, "", "artist", artistTitle, "", 0, now, scannerMetadataForLocalMode(nfoMetadataForParent(file.LocalNFOEnabled, file.ParentArtistMetadata), localMode)); err != nil {
			return err
		}
		albumID := file.ParentID
		if albumID == "" {
			albumID = randomOpaqueMediaID()
		}
		return upsertParentRow(tx, albumID, library.ID, artistID, "album", albumTitle, file.Artist, 0, now, scannerMetadataForLocalMode(nfoMetadataForParent(file.LocalNFOEnabled, file.ParentAlbumMetadata), localMode))
	default:
		return nil
	}
}

func nfoMetadataForParent(enabled bool, metadata scannerLocalMetadata) scannerLocalMetadata {
	if !enabled {
		return scannerLocalMetadata{}
	}
	return metadata
}

func upsertParentRow(tx *sql.Tx, id, libraryID, parentID, mediaType, title, studio string, indexNumber int, now string, localMetadata ...scannerLocalMetadata) error {
	metadata := scannerLocalMetadata{}
	if len(localMetadata) > 0 {
		metadata = localMetadata[0]
	}
	seasonNumber := 0
	if mediaType == "season" {
		seasonNumber = max(0, indexNumber)
	}
	if metadata.Title != "" {
		title = metadata.Title
	}
	if metadata.Studio != "" {
		studio = metadata.Studio
	}
	genres := metadata.Genres
	if genres == nil {
		genres = []string{}
	}
	tags := metadata.Tags
	if tags == nil {
		tags = []string{}
	}
	genresJSON, _ := json.Marshal(genres)
	tagsJSON, _ := json.Marshal(tags)
	_, err := tx.Exec(`
		INSERT INTO media_items (
			id, library_id, parent_id, type, title, sort_title, studio, added_at, index_number, art_seed,
			season_number, original_title, year, summary, tagline, content_rating, community_rating, critic_rating,
			network, country, genres_json, tags_json, random_key
		) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			library_id = excluded.library_id,
			parent_id = excluded.parent_id,
			random_key = CASE WHEN COALESCE(media_items.random_key, '') = '' THEN excluded.random_key ELSE media_items.random_key END`,
		id, libraryID, parentID, mediaType, title, sortableTitle(title), studio, now, indexNumber, mediaType, seasonNumber,
		metadata.OriginalTitle, metadata.Year, metadata.Summary, metadata.Tagline, metadata.ContentRating,
		metadata.CommunityRating, metadata.CriticRating, metadata.Network, metadata.Country, string(genresJSON), string(tagsJSON), mediaRandomKey(id))
	if err != nil {
		return err
	}
	if strings.TrimSpace(metadata.Source) == "" && len(metadata.ProviderIDs) == 0 && len(metadata.People) == 0 {
		return nil
	}
	parentFile := scannerMediaFile{
		ID: id, LibraryID: libraryID, ParentID: parentID, Type: mediaType, Title: title,
		SortTitle: sortableTitle(title), Artist: studio, IndexNumber: indexNumber,
		SeasonNumber: seasonNumber, ArtSeed: mediaType, LocalMetadata: metadata,
	}
	revision, _, err := applyScannedMetadataRevisionTx(context.Background(), tx, parentFile, now)
	if err != nil {
		return err
	}
	if err := replaceMediaSearchTx(context.Background(), tx, id, revision, firstNonEmpty(metadata.Source, "scanner")); err != nil {
		return err
	}
	return replaceMediaCategoryFacetsTx(context.Background(), tx, id, revision, firstNonEmpty(metadata.Source, "scanner"))
}

func replaceScannedPeople(tx *sql.Tx, mediaID string, people []MediaPerson, source string, now string) error {
	if mediaID == "" {
		return nil
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "scanner"
	}
	previousCanonicalKeys := map[string]map[string]bool{}
	rows, err := tx.Query(`
		SELECT name, role, canonical_person_key
		FROM media_people
		WHERE media_id = ? AND trim(canonical_person_key) <> ''`, mediaID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name, role, canonicalKey string
		if err := rows.Scan(&name, &role, &canonicalKey); err != nil {
			rows.Close()
			return err
		}
		creditKey := normalizedPersonCreditKey(name, role)
		if previousCanonicalKeys[creditKey] == nil {
			previousCanonicalKeys[creditKey] = map[string]bool{}
		}
		previousCanonicalKeys[creditKey][strings.TrimSpace(canonicalKey)] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM media_people WHERE media_id = ? AND source = ?`, mediaID, source); err != nil {
		return err
	}
	usedPreviousKeys := map[string]map[string]bool{}
	assignedWeakKeys := map[string]bool{}
	for index, person := range people {
		person.Name = strings.TrimSpace(person.Name)
		person.Role = strings.TrimSpace(person.Role)
		person.Character = strings.TrimSpace(person.Character)
		person.ImageURL = strings.TrimSpace(person.ImageURL)
		if parsed, parseErr := url.Parse(person.ImageURL); parseErr == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
			// Provider portraits must be downloaded by the metadata ingestion path
			// before this persistence boundary. Local/NFO scanner inputs can still
			// contain remote references; discard those instead of retaining a URL
			// that could later leak or trigger an unbounded provider fetch.
			person.ImageURL = ""
		}
		if person.Name == "" || person.Role == "" {
			continue
		}
		providerIDsJSON, _ := json.Marshal(person.ProviderIDs)
		canonicalPersonKey := strings.TrimSpace(person.CanonicalPersonKey)
		creditKey := normalizedPersonCreditKey(person.Name, person.Role)
		if canonicalPersonKey == "" {
			canonicalPersonKey = nextPreviousCanonicalPersonKey(previousCanonicalKeys[creditKey], usedPreviousKeys[creditKey])
			if canonicalPersonKey != "" {
				if usedPreviousKeys[creditKey] == nil {
					usedPreviousKeys[creditKey] = map[string]bool{}
				}
				usedPreviousKeys[creditKey][canonicalPersonKey] = true
			}
		}
		provider, externalID := canonicalPersonProviderIdentity(person.ProviderIDs)
		if canonicalPersonKey == "" && provider != "" {
			var conflict bool
			canonicalPersonKey, conflict, err = existingCanonicalPersonKeyForProviders(tx, person.ProviderIDs)
			if err != nil {
				return err
			}
			if conflict {
				canonicalPersonKey = conflictingProviderSetCanonicalPersonKey(person.ProviderIDs)
			} else if canonicalPersonKey == "" {
				canonicalPersonKey = providerCanonicalPersonKey(provider, externalID)
			}
		}
		if canonicalPersonKey == "" {
			canonicalPersonKey = durablePersonCanonicalKey(mediaID, person.Name, person.Role, person.ImageURL)
			if strings.HasPrefix(canonicalPersonKey, "credit:") && assignedWeakKeys[canonicalPersonKey] {
				canonicalPersonKey = "local:" + randomID("person")
			}
		}
		if strings.HasPrefix(canonicalPersonKey, "credit:") || strings.HasPrefix(canonicalPersonKey, "local:") {
			assignedWeakKeys[canonicalPersonKey] = true
		}
		id := scannedID("person", strings.Join([]string{mediaID, source, person.Role, person.Name, strconv.Itoa(index)}, "\x00"))
		if _, err := tx.Exec(`
				INSERT INTO media_people (id, media_id, name, role, character, source, sort_order, image_url, provider_ids_json, canonical_person_key, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(media_id, name, role, source, sort_order) DO UPDATE SET
					character = excluded.character,
					image_url = excluded.image_url,
					provider_ids_json = excluded.provider_ids_json,
					canonical_person_key = excluded.canonical_person_key,
					created_at = excluded.created_at`,
			id, mediaID, person.Name, person.Role, person.Character, source, index, person.ImageURL, string(providerIDsJSON), canonicalPersonKey, now); err != nil {
			return err
		}
	}
	return nil
}

func nextPreviousCanonicalPersonKey(keys, used map[string]bool) string {
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		if !used[key] {
			ordered = append(ordered, key)
		}
	}
	sort.Strings(ordered)
	if len(ordered) == 0 {
		return ""
	}
	return ordered[0]
}

func existingCanonicalPersonKeyForProvider(tx *sql.Tx, provider, externalID string) (string, error) {
	rows, err := tx.Query(`
		SELECT DISTINCT trim(p.canonical_person_key)
		FROM media_people p, json_each(CASE WHEN json_valid(p.provider_ids_json) THEN p.provider_ids_json ELSE '{}' END) provider_id
		WHERE trim(p.canonical_person_key) <> ''
		  AND lower(trim(provider_id.key)) = ?
		  AND trim(CAST(provider_id.value AS TEXT)) = ?
		ORDER BY trim(p.canonical_person_key)
		LIMIT 2`, strings.ToLower(strings.TrimSpace(provider)), strings.TrimSpace(externalID))
	if err != nil {
		return "", err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return "", err
		}
		keys = append(keys, strings.TrimSpace(key))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(keys) == 1 {
		return keys[0], nil
	}
	return "", nil
}

func existingCanonicalPersonKeyForProviders(tx *sql.Tx, providerIDs map[string]string) (string, bool, error) {
	providers := make([]string, 0, len(providerIDs))
	values := map[string]string{}
	for provider, externalID := range providerIDs {
		provider = strings.ToLower(strings.TrimSpace(provider))
		externalID = strings.TrimSpace(externalID)
		if provider == "" || externalID == "" {
			continue
		}
		providers = append(providers, provider)
		values[provider] = externalID
	}
	sort.Strings(providers)
	keys := map[string]bool{}
	for _, provider := range providers {
		key, err := existingCanonicalPersonKeyForProvider(tx, provider, values[provider])
		if err != nil {
			return "", false, err
		}
		if key != "" {
			keys[key] = true
		}
	}
	if len(keys) > 1 {
		return "", true, nil
	}
	for key := range keys {
		return key, false, nil
	}
	return "", false, nil
}

func conflictingProviderSetCanonicalPersonKey(providerIDs map[string]string) string {
	pairs := make([]string, 0, len(providerIDs))
	for provider, externalID := range providerIDs {
		provider = strings.ToLower(strings.TrimSpace(provider))
		externalID = strings.TrimSpace(externalID)
		if provider != "" && externalID != "" {
			pairs = append(pairs, provider+"\x1f"+externalID)
		}
	}
	sort.Strings(pairs)
	if len(pairs) == 0 {
		return ""
	}
	digest := sha256.Sum256([]byte(strings.Join(pairs, "\x1e")))
	return "provider-set:" + hex.EncodeToString(digest[:16])
}

func providerCanonicalPersonKey(provider, externalID string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	externalID = strings.TrimSpace(externalID)
	if provider == "" || externalID == "" {
		return ""
	}
	return "provider:" + hex.EncodeToString([]byte(provider+"\x1f"+externalID))
}

func normalizedPersonCreditKey(name, role string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " ")) + "\x1f" +
		strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(role)), " "))
}

func durablePersonCanonicalKey(mediaID, name, role, imageURL string) string {
	canonicalName := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
	if canonicalName == "" {
		return ""
	}
	if imageURL = strings.ToLower(strings.TrimSpace(imageURL)); imageURL != "" {
		return "portrait:" + hex.EncodeToString([]byte(canonicalName+"\x1f"+imageURL))
	}
	canonicalRole := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(role)), " "))
	return "credit:" + hex.EncodeToString([]byte(strings.TrimSpace(mediaID)+"\x1f"+canonicalName+"\x1f"+canonicalRole))
}

// ensureScannedMediaIdentityTx publishes only the structural row needed to
// allocate stable child resources. Canonical local metadata is deliberately
// deferred until after filesystem publication succeeds.
func ensureScannedMediaIdentityTx(tx *sql.Tx, file scannerMediaFile, now string) error {
	_, err := tx.Exec(`
		INSERT INTO media_items (
			id, library_id, parent_id, type, title, sort_title, added_at,
			season_number, episode_number, index_number, art_seed, source_url, random_key
		) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			library_id = excluded.library_id,
			parent_id = excluded.parent_id,
			type = excluded.type,
			random_key = CASE WHEN COALESCE(media_items.random_key, '') = '' THEN excluded.random_key ELSE media_items.random_key END`,
		file.ID, file.LibraryID, file.ParentID, file.Type, file.Title, file.SortTitle, now,
		file.SeasonNumber, file.EpisodeNumber, file.IndexNumber, file.ArtSeed, file.SourcePath, mediaRandomKey(file.ID))
	return err
}

func scannedMetadataUpdate(file scannerMediaFile) UpdateMediaRequest {
	metadata := file.LocalMetadata
	update := UpdateMediaRequest{}
	setString := func(target **string, value string) {
		if strings.TrimSpace(value) != "" {
			copy := value
			*target = &copy
		}
	}
	setInt := func(target **int, value int) {
		if value > 0 {
			copy := value
			*target = &copy
		}
	}
	setFloat := func(target **float64, value float64) {
		if value > 0 {
			copy := value
			*target = &copy
		}
	}
	title := firstNonEmpty(metadata.Title, file.Title)
	sortTitle := firstNonEmpty(metadata.SortTitle, file.SortTitle, sortableTitle(title))
	setString(&update.Title, title)
	setString(&update.SortTitle, sortTitle)
	setString(&update.OriginalTitle, metadata.OriginalTitle)
	setString(&update.Edition, metadata.Edition)
	setInt(&update.Year, metadata.Year)
	if metadata.RuntimeMinutes > 0 {
		durationSeconds := metadata.RuntimeMinutes * 60
		update.DurationSeconds = &durationSeconds
	}
	setString(&update.Summary, metadata.Summary)
	setString(&update.Tagline, metadata.Tagline)
	setString(&update.ContentRating, metadata.ContentRating)
	setFloat(&update.CommunityRating, metadata.CommunityRating)
	setInt(&update.CriticRating, metadata.CriticRating)
	setString(&update.Studio, firstNonEmpty(metadata.Studio, file.Artist))
	setString(&update.Network, metadata.Network)
	setString(&update.Country, metadata.Country)
	if len(metadata.Genres) > 0 {
		values := append([]string(nil), metadata.Genres...)
		update.Genres = &values
	}
	if len(metadata.Tags) > 0 {
		values := append([]string(nil), metadata.Tags...)
		update.Tags = &values
	}
	if typed := typedMetadataForScannedFile(file); len(typed) > 0 {
		update.TypedMetadata = &typed
	}
	if metadata.People != nil {
		people := append([]MediaPerson(nil), metadata.People...)
		update.People = &people
	}
	seasonNumber := firstPositiveInt(metadata.SeasonNumber, file.SeasonNumber)
	episodeNumber := firstPositiveInt(metadata.EpisodeNumber, file.EpisodeNumber)
	indexNumber := file.IndexNumber
	update.SeasonNumber = &seasonNumber
	update.EpisodeNumber = &episodeNumber
	update.IndexNumber = &indexNumber
	return update
}

func scannedMetadataOrigin(source string) metadataSourceKind {
	source = strings.ToLower(strings.TrimSpace(source))
	switch {
	case strings.HasPrefix(source, "nfo"):
		return metadataSourceNFO
	case strings.HasPrefix(source, "embedded"), strings.HasPrefix(source, "tags"), strings.HasPrefix(source, "ffprobe"):
		return metadataSourceEmbedded
	case strings.HasPrefix(source, "file"):
		return metadataSourceFile
	default:
		return metadataSourceScanner
	}
}

// applyScannedMetadataRevisionTx applies local metadata as one CAS-protected
// canonical revision and stamps every scanner-owned evidence projection with
// that same revision.
func applyScannedMetadataRevisionTx(ctx context.Context, tx *sql.Tx, file scannerMediaFile, now string) (int, map[string]bool, error) {
	if err := ensureScannedMediaIdentityTx(tx, file, now); err != nil {
		return 0, nil, err
	}
	state, err := loadMetadataCanonicalStateTx(tx, file.ID)
	if err != nil {
		return 0, nil, err
	}
	expectedRevision := state.Revision
	locks, err := metadataLocksTx(tx, file.ID)
	if err != nil {
		return 0, nil, err
	}
	update := scannedMetadataUpdate(file)
	filterMetadataUpdateByLocks(&update, locks, state.TypedMetadata)
	changed := metadataChangedFields(update)
	if err := applyMetadataUpdateToState(&state, update); err != nil {
		return 0, nil, err
	}
	state.SourceURL = file.SourcePath
	state.Revision = expectedRevision + 1
	state.MetadataRefreshedAt = now
	state.ETag, err = metadataCanonicalETag(state)
	if err != nil {
		return 0, nil, err
	}
	sourceDetail := firstNonEmpty(file.LocalMetadata.Source, "scanner")
	origin := scannedMetadataOrigin(sourceDetail)
	// Persist the canonical source kind, never an NFO/embedded file path. The
	// scanner owns path-level acquisition diagnostics separately; metadata
	// evidence is viewer-projectable provenance and must remain path-free.
	source := string(origin)
	revisionID := randomID("mrev")
	if _, err = tx.Exec(`
		INSERT INTO media_metadata_revisions (
			id, media_id, revision, base_revision, state, trigger_kind, provider, started_at, completed_at
		) VALUES (?, ?, ?, ?, 'applied', ?, 'local', ?, ?)`,
		revisionID, file.ID, state.Revision, expectedRevision, metadataRevisionTrigger(origin), now, now); err != nil {
		return 0, nil, err
	}
	if err = carryForwardMetadataEvidenceTx(tx, file.ID, expectedRevision, revisionID, state.Revision, now); err != nil {
		return 0, nil, err
	}
	if err = persistMetadataCanonicalStateTx(tx, state, expectedRevision); err != nil {
		return 0, nil, err
	}
	for _, statement := range []string{
		`UPDATE media_provider_ids SET evidence_revision = ?, updated_at = ? WHERE media_id = ?`,
		`UPDATE media_people SET metadata_revision = ? WHERE media_id = ?`,
		`UPDATE media_images SET metadata_revision = ? WHERE media_id = ?`,
		`UPDATE media_metadata_locks SET metadata_revision = ? WHERE media_id = ?`,
	} {
		args := []any{state.Revision, file.ID}
		if strings.Contains(statement, "updated_at") {
			args = []any{state.Revision, now, file.ID}
		}
		if _, err = tx.Exec(statement, args...); err != nil {
			return 0, nil, err
		}
	}
	if update.People != nil && !locks["people"] {
		if err = replaceScannedPeople(tx, file.ID, *update.People, source, now); err != nil {
			return 0, nil, err
		}
		if _, err = tx.Exec(`UPDATE media_people SET metadata_revision = ? WHERE media_id = ? AND source = ?`, state.Revision, file.ID, source); err != nil {
			return 0, nil, err
		}
	}
	if err = upsertScannerMetadataHints(tx, file, now); err != nil {
		return 0, nil, err
	}
	if _, err = tx.Exec(`UPDATE media_provider_ids SET evidence_revision = ?, updated_at = ? WHERE media_id = ?`, state.Revision, now, file.ID); err != nil {
		return 0, nil, err
	}
	if update.ContentRating != nil {
		if err = upsertMediaRatingEvidenceTx(tx, file.ID, "local", source, state.Country, state.ContentRating, now); err != nil {
			return 0, nil, err
		}
	}
	req := metadataApplyRequest{MediaID: file.ID, Origin: origin, Source: source, Provider: "local", Update: update}
	if err = writeMetadataEvidenceTx(tx, state, revisionID, req, changed, locks, now); err != nil {
		return 0, nil, err
	}
	return state.Revision, locks, nil
}

func typedMetadataForScannedFile(file scannerMediaFile) map[string]string {
	out := map[string]string{}
	for key, value := range file.LocalMetadata.TypedMetadata {
		out[key] = value
	}
	add := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			out[key] = value
		}
	}
	addInt := func(key string, value int) {
		if value > 0 {
			out[key] = strconv.Itoa(value)
		}
	}
	add("localTitle", file.LocalMetadata.LocalTitle)
	add("showTitle", file.LocalMetadata.ShowTitle)
	add("releaseDate", file.LocalMetadata.ExactDate)
	addInt("season", file.LocalMetadata.SeasonNumber)
	addInt("episode", file.LocalMetadata.EpisodeNumber)
	addInt("runtimeMinutes", file.LocalMetadata.RuntimeMinutes)
	add("collection", file.LocalMetadata.Collection)
	add("ratingName", file.LocalMetadata.RatingName)
	addInt("ratingVotes", file.LocalMetadata.RatingVotes)
	if len(file.LocalMetadata.Studios) > 0 {
		add("studios", strings.Join(file.LocalMetadata.Studios, "\x1f"))
	}
	if len(file.LocalMetadata.Countries) > 0 {
		add("countries", strings.Join(file.LocalMetadata.Countries, "\x1f"))
	}
	switch file.Type {
	case "artist":
		add("artist", file.Title)
	case "album":
		add("albumTitle", file.Title)
		add("albumArtist", file.Artist)
		add("label", file.LocalMetadata.Label)
		add("releaseCountry", file.LocalMetadata.ReleaseCountry)
		addInt("trackCount", file.LocalMetadata.TrackCount)
		addInt("discCount", file.LocalMetadata.DiscCount)
	case "track":
		add("artist", firstNonEmpty(file.LocalMetadata.AlbumArtist, file.GrandparentTitle, file.Artist))
		add("albumArtist", firstNonEmpty(file.LocalMetadata.AlbumArtist, file.GrandparentTitle, file.Artist))
		add("trackArtist", firstNonEmpty(file.LocalMetadata.Artist, file.LocalMetadata.Studio, file.Artist))
		add("albumTitle", firstNonEmpty(file.LocalMetadata.AlbumTitle, file.ParentTitle))
		add("label", file.LocalMetadata.Label)
		add("releaseCountry", file.LocalMetadata.ReleaseCountry)
		addInt("trackNumber", firstPositiveInt(file.LocalMetadata.TrackNumber, file.IndexNumber))
		addInt("trackCount", file.LocalMetadata.TrackCount)
		addInt("discNumber", file.LocalMetadata.DiscNumber)
		addInt("discCount", file.LocalMetadata.DiscCount)
		addInt("bpm", file.LocalMetadata.BPM)
		add("explicit", file.LocalMetadata.Explicit)
	case "audiobook":
		add("author", file.LocalMetadata.Artist)
		add("authorProvider", file.LocalMetadata.AuthorProvider)
		add("authorId", file.LocalMetadata.AuthorID)
		add("narrator", file.LocalMetadata.Studio)
		add("series", file.LocalMetadata.Series)
		add("seriesProvider", file.LocalMetadata.SeriesProvider)
		add("seriesId", file.LocalMetadata.SeriesID)
		add("seriesIndex", file.LocalMetadata.SeriesIndex)
		add("publisher", file.LocalMetadata.Publisher)
	}
	return out
}

func upsertScannedMediaFile(tx *sql.Tx, file scannerMediaFile, now string, scanGeneration string) error {
	fileID := file.FileID
	if fileID == "" {
		fileID = scannerFileID(file.ID, file.SourcePath)
	}
	sourceType := strings.TrimSpace(file.SourceType)
	if sourceType == "" {
		sourceType = "local"
	}
	extensionSource := firstNonEmpty(file.DisplayPath, file.SourcePath)
	container := strings.TrimPrefix(strings.ToLower(filepath.Ext(extensionSource)), ".")
	identityEvidence := "scanner"
	if strings.TrimSpace(file.QuickSignature) != "" {
		identityEvidence = "scanner:v2:" + strings.TrimSpace(file.QuickSignature)
	}
	if _, err := tx.Exec(`
		INSERT INTO media_files (
			id, media_id, library_id, path, directory_path, quality, container, version_label, resolution, source, source_type, video_codec, audio_codec, dynamic_range,
			release_group, three_d, version_group, quality_rank,
			size_bytes, mod_time, available, missing_since, first_seen_at, last_seen_at, scan_generation,
			content_fingerprint, identity_evidence
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, '', ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			media_id = excluded.media_id,
			library_id = excluded.library_id,
			path = excluded.path,
			directory_path = excluded.directory_path,
			quality = excluded.quality,
			container = excluded.container,
			version_label = excluded.version_label,
			resolution = excluded.resolution,
			source = excluded.source,
			source_type = excluded.source_type,
			video_codec = excluded.video_codec,
			audio_codec = excluded.audio_codec,
			dynamic_range = excluded.dynamic_range,
			release_group = excluded.release_group,
			three_d = excluded.three_d,
			version_group = excluded.version_group,
			quality_rank = excluded.quality_rank,
			size_bytes = excluded.size_bytes,
			mod_time = excluded.mod_time,
			available = 1,
			missing_since = '',
			last_seen_at = excluded.last_seen_at,
			scan_generation = excluded.scan_generation,
			content_fingerprint = excluded.content_fingerprint,
			identity_evidence = excluded.identity_evidence`,
		fileID, file.ID, file.LibraryID, file.SourcePath, filepath.Dir(file.SourcePath), file.Version.Resolution, container, file.Version.Label, file.Version.Resolution, file.Version.Source, sourceType, file.Version.VideoCodec, file.Version.AudioCodec, file.Version.DynamicRange,
		file.Version.ReleaseGroup, boolInt(file.Version.ThreeD), file.ID, file.Version.QualityRank, file.FileSize, file.FileModTime, now, now, scanGeneration, file.ContentFingerprint, identityEvidence); err != nil {
		return err
	}
	_, err := tx.Exec(`
		UPDATE media_items
		SET source_url = (
			SELECT path
			FROM media_files
			WHERE media_id = media_items.id AND available = 1
			ORDER BY quality_rank DESC, size_bytes DESC, path ASC
			LIMIT 1
		)
		WHERE id = ?`, file.ID)
	return err
}

func (s *Server) replaceScannedStreams(tx *sql.Tx, file scannerMediaFile) error {
	existing := map[string]string{}
	rows, err := tx.Query(`SELECT kind, id FROM media_streams WHERE media_id = ? AND source_kind = 'scanner' AND (? = '' OR file_id = ?)`, file.ID, file.FileID, file.FileID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var kind, id string
		if err := rows.Scan(&kind, &id); err != nil {
			_ = rows.Close()
			return err
		}
		existing[kind] = id
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM media_streams WHERE media_id = ? AND source_kind = 'scanner' AND (? = '' OR file_id = ?)`, file.ID, file.FileID, file.FileID); err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(firstNonEmpty(file.DisplayPath, file.SourcePath)))
	if videoExtensions[ext] {
		streamID := existing["video"]
		if streamID == "" {
			var idErr error
			streamID, idErr = stableOpaquePublicResourceIDTx(tx, "media-stream", strings.Join([]string{"scanner", firstNonEmpty(file.FileID, file.ID), "video"}, "\x00"))
			if idErr != nil {
				return idErr
			}
		}
		if _, err := tx.Exec(`INSERT INTO media_streams (id, media_id, file_id, source_kind, source_identity, storage_key, stream_index, kind, codec, display_title) VALUES (?, ?, ?, 'scanner', ?, ?, -1, 'video', ?, ?)`,
			streamID, file.ID, file.FileID, firstNonEmpty(file.FileID, file.ID)+"\x1fvideo", streamID, scannedVideoCodecForExtension(ext), strings.ToUpper(strings.TrimPrefix(ext, "."))+" video"); err != nil {
			return err
		}
	}
	if audioExtensions[ext] || audiobookExtensions[ext] || videoExtensions[ext] {
		codec := scannedAudioCodecForExtension(ext)
		if audioExtensions[ext] || audiobookExtensions[ext] {
			codec = codecForExtension(ext)
		}
		streamID := existing["audio"]
		if streamID == "" {
			var idErr error
			streamID, idErr = stableOpaquePublicResourceIDTx(tx, "media-stream", strings.Join([]string{"scanner", firstNonEmpty(file.FileID, file.ID), "audio"}, "\x00"))
			if idErr != nil {
				return idErr
			}
		}
		_, err := tx.Exec(`INSERT INTO media_streams (id, media_id, file_id, source_kind, source_identity, storage_key, stream_index, kind, codec, language, channels, display_title) VALUES (?, ?, ?, 'scanner', ?, ?, -1, 'audio', ?, 'und', 2, ?)`,
			streamID, file.ID, file.FileID, firstNonEmpty(file.FileID, file.ID)+"\x1faudio", streamID, codec, strings.ToUpper(strings.TrimPrefix(ext, "."))+" audio")
		return err
	}
	return nil
}

var (
	scannerSidecarReadDir  = os.ReadDir
	scannerSidecarReadFile = os.ReadFile
	scannerSidecarStat     = os.Stat
	scannerSidecarEvalPath = filepath.EvalSymlinks
	scannerPublishArtifact = publishPrivateArtifact
)

// prepareScannedMediaFilesystem captures a complete filesystem snapshot before
// any catalog write transaction begins. Discovery/read failures are returned,
// never converted into an empty snapshot that could erase valid catalog rows.
func (s *Server) prepareScannedMediaFilesystem(file scannerMediaFile) (scannerMediaFile, error) {
	if file.FilesystemPrepared {
		return file, nil
	}
	if file.LocalNFOEnabled {
		file.ParentShowMetadata = showLocalMetadataForFile(file)
		file.ParentSeasonMetadata = seasonLocalMetadataForFile(file)
		file.ParentArtistMetadata = artistLocalMetadataForFile(file)
		file.ParentAlbumMetadata = albumLocalMetadataForFile(file)
	}

	if file.ReadExternalSidecars && file.SourcePath != "" && (file.Type == "movie" || file.Type == "episode" || file.Type == "audiobook") {
		candidates, err := scannerSidecarCandidatesStrict(file.SourcePath, sidecarSubtitleExtensions)
		if err != nil {
			return file, fmt.Errorf("discover sidecar subtitles: %w", err)
		}
		for _, candidate := range candidates {
			candidate, err = resolveScannerPathWithinRoot(scannerRootForFile(file), candidate)
			if err != nil {
				s.log.Warn("sidecar subtitle skipped", "path", candidate, "error", err)
				continue
			}
			contents, err := readScannerSidecarFile(candidate, scannerSubtitleSidecarLimit)
			if err != nil {
				if errors.Is(err, errScannerSidecarTooLarge) {
					s.log.Warn("sidecar subtitle skipped", "path", candidate, "error", err)
					continue
				}
				return file, fmt.Errorf("read sidecar subtitle %q: %w", candidate, err)
			}
			if len(contents) == 0 {
				continue
			}
			normalized, err := normalizeUploadedSubtitle(candidate, contents)
			if err != nil {
				s.log.Warn("sidecar subtitle skipped", "path", candidate, "error", err)
				continue
			}
			language, forced := sidecarSubtitleLanguage(candidate, file.SourcePath)
			file.PreparedSubtitles = append(file.PreparedSubtitles, scannerPreparedSubtitle{
				CandidatePath: candidate,
				NormalizedVTT: normalized,
				Language:      language,
				Forced:        forced,
			})
		}
	}

	if file.ReadExternalSidecars && file.SourcePath != "" && file.Type == "track" {
		candidates, err := scannerSidecarCandidatesStrict(file.SourcePath, sidecarLyricExtensions)
		if err != nil {
			return file, fmt.Errorf("discover sidecar lyrics: %w", err)
		}
		for _, candidate := range candidates {
			candidate, err = resolveScannerPathWithinRoot(scannerRootForFile(file), candidate)
			if err != nil {
				s.log.Warn("sidecar lyric skipped", "path", candidate, "error", err)
				continue
			}
			contents, err := readScannerSidecarFile(candidate, scannerLyricsSidecarLimit)
			if err != nil {
				if errors.Is(err, errScannerSidecarTooLarge) {
					s.log.Warn("sidecar lyric skipped", "path", candidate, "error", err)
					continue
				}
				return file, fmt.Errorf("read sidecar lyric %q: %w", candidate, err)
			}
			if len(contents) == 0 {
				continue
			}
			format := strings.TrimPrefix(strings.ToLower(filepath.Ext(candidate)), ".")
			file.PreparedLyrics = append(file.PreparedLyrics, scannerPreparedLyric{
				CandidatePath: candidate,
				Text:          string(contents),
				Format:        format,
				Language:      sidecarLyricLanguage(candidate, file.SourcePath),
				Synced:        format == "lrc" || bytesLookLikeLRC(contents),
			})
		}
	}

	if file.DiscoverLocalArtwork {
		file.ExistingLocalImages = map[string]bool{}
		imageProbe := file
		imageProbe.ID = firstNonEmpty(imageProbe.ID, "scanner-self")
		imageProbe.ParentID = firstNonEmpty(imageProbe.ParentID, "scanner-parent")
		imageProbe.GrandparentID = firstNonEmpty(imageProbe.GrandparentID, "scanner-grandparent")
		for _, candidate := range localImageCandidatesForScannedFile(imageProbe) {
			_, err := resolveScannerPathWithinRoot(scannerRootForFile(file), candidate.Path)
			if err == nil {
				file.ExistingLocalImages[filepath.Clean(candidate.Path)] = true
				continue
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return file, fmt.Errorf("inspect local artwork %q: %w", candidate.Path, err)
			}
		}
	}
	file.FilesystemPrepared = true
	return file, nil
}

func scannerRootForFile(file scannerMediaFile) string {
	if strings.TrimSpace(file.ScanRoot) != "" {
		return file.ScanRoot
	}
	return filepath.Dir(file.SourcePath)
}

func resolveScannerPathWithinRoot(root, path string) (string, error) {
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	if root == "" || path == "" {
		return "", errors.New("scanner path has no trusted library root")
	}
	resolvedRoot, err := scannerSidecarEvalPath(root)
	if err != nil {
		return "", err
	}
	resolvedPath, err := scannerSidecarEvalPath(path)
	if err != nil {
		return "", err
	}
	info, err := scannerSidecarStat(resolvedPath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || !pathInsideRoot(resolvedPath, resolvedRoot) {
		return "", errors.New("scanner sidecar escaped its library root")
	}
	return resolvedPath, nil
}

func readScannerSidecarFile(path string, limit int64) ([]byte, error) {
	info, err := scannerSidecarStat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return nil, errScannerSidecarTooLarge
	}
	contents, err := scannerSidecarReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > limit {
		return nil, errScannerSidecarTooLarge
	}
	return contents, nil
}

func scannerSidecarCandidatesStrict(mediaPath string, extensions map[string]bool) ([]string, error) {
	dir := filepath.Dir(mediaPath)
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	entries, err := scannerSidecarReadDir(dir)
	if err != nil {
		return nil, err
	}
	prefix := strings.ToLower(base)
	out := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !extensions[ext] {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		lowerStem := strings.ToLower(stem)
		if lowerStem == prefix || strings.HasPrefix(lowerStem, prefix+".") || strings.HasPrefix(lowerStem, prefix+"-") || strings.HasPrefix(lowerStem, prefix+"_") {
			out = append(out, filepath.Join(dir, name))
		}
	}
	sort.Strings(out)
	return out, nil
}

type SubtitleProvider interface {
	ID() string
	CandidatePaths(file scannerMediaFile) []string
}

type sidecarSubtitleProvider struct{}

func (sidecarSubtitleProvider) ID() string { return "sidecar" }

func (sidecarSubtitleProvider) CandidatePaths(file scannerMediaFile) []string {
	return sidecarSubtitleCandidates(file.SourcePath)
}

func (s *Server) planScannedSidecarSubtitles(tx *sql.Tx, file scannerMediaFile) (scannerSubtitleReplacement, error) {
	replacement := scannerSubtitleReplacement{File: file}
	if file.SourcePath == "" || file.ID == "" || (file.Type != "movie" && file.Type != "episode" && file.Type != "audiobook") {
		return replacement, nil
	}
	for _, prepared := range file.PreparedSubtitles {
		streamIdentity := strings.Join([]string{"sidecar", file.ID, file.FileID, filepath.Clean(prepared.CandidatePath)}, "\x00")
		streamID, err := stableOpaquePublicResourceIDTx(tx, "media-stream", streamIdentity)
		if err != nil {
			return replacement, err
		}
		label := strings.ToUpper(prepared.Language)
		if label == "UND" {
			label = "Sidecar"
		}
		if prepared.Forced {
			label += " Forced"
		}
		sourceURL := "/api/media/" + url.PathEscape(file.ID) + "/subtitles/" + url.PathEscape(streamID)
		replacement.Publications = append(replacement.Publications, scannerSubtitlePublication{
			File:           file,
			StreamID:       streamID,
			StreamIdentity: streamIdentity,
			StoragePath:    filepath.Join(s.cfg.AppDataDir, "subtitles", safePathComponent(file.ID), safePathComponent(streamID)+".vtt"),
			SourceURL:      sourceURL,
			Language:       prepared.Language,
			DisplayTitle:   label,
			NormalizedVTT:  prepared.NormalizedVTT,
		})
	}
	return replacement, nil
}

func (s *Server) publishScannedSidecarSubtitles(replacements []scannerSubtitleReplacement) error {
	root := filepath.Join(s.cfg.AppDataDir, "subtitles")
	for _, replacement := range replacements {
		for _, publication := range replacement.Publications {
			if err := scannerPublishArtifact(root, publication.StoragePath, publication.NormalizedVTT); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceScannedSidecarSubtitleRows(tx *sql.Tx, replacements []scannerSubtitleReplacement) error {
	for _, replacement := range replacements {
		file := replacement.File
		if file.SourcePath == "" || file.ID == "" || (file.Type != "movie" && file.Type != "episode" && file.Type != "audiobook") {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM media_streams WHERE media_id = ? AND kind = 'subtitle' AND source_kind = 'sidecar' AND (? = '' OR file_id = ?)`, file.ID, file.FileID, file.FileID); err != nil {
			return err
		}
		for _, publication := range replacement.Publications {
			if _, err := tx.Exec(`
				INSERT INTO media_streams (id, media_id, file_id, source_kind, source_identity, storage_key, stream_index, kind, codec, language, display_title, source_url)
				VALUES (?, ?, ?, 'sidecar', ?, ?, -1, 'subtitle', 'webvtt', ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET
					codec = excluded.codec,
					language = excluded.language,
					display_title = excluded.display_title,
					source_url = excluded.source_url`,
				publication.StreamID, file.ID, file.FileID, publication.StreamIdentity, publication.StreamID,
				publication.Language, publication.DisplayTitle, publication.SourceURL); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceScannedLocalImages(tx *sql.Tx, file scannerMediaFile, now string) error {
	return replaceScannedImages(tx, file, now, localImageProvider{})
}

type ImageProvider interface {
	ID() string
	Candidates(file scannerMediaFile) []scannedLocalImageCandidate
}

type localImageProvider struct{}

func (localImageProvider) ID() string { return "local" }

func (localImageProvider) Candidates(file scannerMediaFile) []scannedLocalImageCandidate {
	return localImageCandidatesForScannedFile(file)
}

func replaceScannedImages(tx *sql.Tx, file scannerMediaFile, now string, providers ...ImageProvider) error {
	if file.SourcePath == "" || file.ID == "" {
		return nil
	}
	var candidates []scannedLocalImageCandidate
	for _, provider := range providers {
		candidates = append(candidates, provider.Candidates(file)...)
	}
	targetScopes := map[string]scannedLocalImageCandidate{}
	for _, candidate := range candidates {
		targetScopes[candidate.MediaID+"\x00"+candidate.DiscoveryScope] = candidate
	}
	for _, candidate := range targetScopes {
		if _, err := tx.Exec(`
			DELETE FROM media_images
			WHERE media_id = ? AND source = 'local' AND provider = 'scanner' AND discovery_scope = ?`,
			candidate.MediaID, candidate.DiscoveryScope); err != nil {
			return err
		}
	}
	for _, candidate := range candidates {
		if !file.ExistingLocalImages[filepath.Clean(candidate.Path)] {
			continue
		}
		preferred := 0
		if candidate.Preferred {
			preferred = 1
		}
		id := randomOpaquePublicID()
		if _, err := tx.Exec(`
			INSERT INTO media_images (
				id, media_id, image_type, source, provider, path, remote_url, width, height, language, rating, preferred, created_at, discovery_scope
			) VALUES (?, ?, ?, ?, 'scanner', ?, '', 0, 0, '', 0, ?, ?, ?)
			ON CONFLICT(media_id, image_type, source, path, remote_url) DO UPDATE SET
				provider = excluded.provider,
				preferred = excluded.preferred,
				created_at = excluded.created_at,
				discovery_scope = excluded.discovery_scope`,
			id, candidate.MediaID, candidate.Type, candidate.Source, candidate.Path, preferred, now, candidate.DiscoveryScope); err != nil {
			return err
		}
	}
	return nil
}

type scannedLocalImageCandidate struct {
	MediaID        string
	Type           string
	Source         string
	Path           string
	Preferred      bool
	DiscoveryScope string
}

func localImageCandidatesForScannedFile(file scannerMediaFile) []scannedLocalImageCandidate {
	dir := filepath.Dir(file.SourcePath)
	base := strings.TrimSuffix(filepath.Base(file.SourcePath), filepath.Ext(file.SourcePath))
	addCandidates := func(mediaID, imageType, source, imageDir string, names []string, preferred bool, out *[]scannedLocalImageCandidate) {
		if mediaID == "" {
			return
		}
		for _, name := range names {
			*out = append(*out, scannedLocalImageCandidate{
				MediaID:        mediaID,
				Type:           imageType,
				Source:         source,
				Path:           filepath.Join(imageDir, name),
				Preferred:      preferred,
				DiscoveryScope: filepath.Clean(imageDir),
			})
		}
	}
	var out []scannedLocalImageCandidate
	for kind, imagePath := range file.LocalMetadata.ImagePaths {
		if !supportedLocalImageKind(kind) {
			continue
		}
		imagePath = strings.TrimSpace(imagePath)
		if imagePath != "" {
			out = append(out, scannedLocalImageCandidate{MediaID: file.ID, Type: kind, Source: "local", Path: imagePath, Preferred: true, DiscoveryScope: filepath.Clean(dir)})
		}
	}
	switch file.Type {
	case "movie":
		for _, kind := range []string{"poster", "backdrop", "thumb", "logo", "banner", "disc", "clearart"} {
			addCandidates(file.ID, kind, "local", dir, localArtworkNamesForTypeWithBase(file.Type, kind, base), kind == "poster" || kind == "backdrop", &out)
		}
	case "episode":
		seasonDir := dir
		showDir := dir
		if isSeasonLikeFolder(filepath.Base(dir)) {
			showDir = filepath.Dir(dir)
		}
		addCandidates(file.ID, "thumb", "local", dir, localArtworkNamesForTypeWithBase(file.Type, "thumb", base), true, &out)
		addCandidates(file.ParentID, "poster", "local", seasonDir, localArtworkNamesForType("season", "poster"), true, &out)
		addCandidates(file.ParentID, "backdrop", "local", seasonDir, localArtworkNamesForType("season", "backdrop"), false, &out)
		addCandidates(file.ParentID, "banner", "local", seasonDir, localArtworkNamesForType("season", "banner"), false, &out)
		addCandidates(file.GrandparentID, "poster", "local", showDir, localArtworkNamesForType("show", "poster"), true, &out)
		addCandidates(file.GrandparentID, "backdrop", "local", showDir, localArtworkNamesForType("show", "backdrop"), true, &out)
		addCandidates(file.GrandparentID, "logo", "local", showDir, localArtworkNamesForType("show", "logo"), false, &out)
		addCandidates(file.GrandparentID, "banner", "local", showDir, localArtworkNamesForType("show", "banner"), false, &out)
	case "track":
		addCandidates(file.ParentID, "poster", "local", dir, localArtworkNamesForType("album", "poster"), true, &out)
		addCandidates(file.ParentID, "disc", "local", dir, localArtworkNamesForType("album", "disc"), false, &out)
		addCandidates(file.GrandparentID, "poster", "local", filepath.Dir(filepath.Dir(file.SourcePath)), localArtworkNamesForType("artist", "poster"), false, &out)
		addCandidates(file.GrandparentID, "backdrop", "local", filepath.Dir(filepath.Dir(file.SourcePath)), localArtworkNamesForType("artist", "backdrop"), false, &out)
		addCandidates(file.GrandparentID, "logo", "local", filepath.Dir(filepath.Dir(file.SourcePath)), localArtworkNamesForType("artist", "logo"), false, &out)
	case "audiobook":
		addCandidates(file.ID, "poster", "local", dir, localArtworkNamesForTypeWithBase(file.Type, "poster", base), true, &out)
	}
	return out
}

func localArtworkNamesForTypeWithBase(mediaType, kind, base string) []string {
	var stems []string
	if strings.TrimSpace(base) != "" && (kind == "poster" || kind == "thumb" || kind == "logo" || kind == "banner" || kind == "disc" || kind == "clearart") {
		stems = append(stems, base)
	}
	return uniqueNonEmptyStrings(append(localArtworkNamesFromStems(stems...), localArtworkNamesForType(mediaType, kind)...))
}

func localArtworkNamesForType(mediaType, kind string) []string {
	switch kind {
	case "poster":
		if mediaType == "album" || mediaType == "track" {
			return localArtworkNamesFromStems("cover", "folder", "albumart", "front", "jacket", "poster", "default")
		}
		if mediaType == "artist" {
			return localArtworkNamesFromStems("artist", "folder", "poster", "cover", "default")
		}
		if mediaType == "season" {
			return localArtworkNamesFromStems("season", "poster", "folder", "cover", "default")
		}
		return localArtworkNamesFromStems("poster", "folder", "cover", "default", "movie", "show", "front")
	case "backdrop":
		return localArtworkNamesFromStems("backdrop", "fanart", "background", "landscape")
	case "thumb":
		return localArtworkNamesFromStems("thumb", "thumbnail", "still")
	case "logo":
		return localArtworkNamesFromStems("logo", "clearlogo", "clear-logo")
	case "banner":
		return localArtworkNamesFromStems("banner", "wide", "art")
	case "disc":
		return localArtworkNamesFromStems("disc", "disk", "cdart", "discart", "media")
	case "clearart":
		return localArtworkNamesFromStems("clearart", "clear-art", "characterart")
	default:
		return nil
	}
}

func supportedLocalImageKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "poster", "backdrop", "thumb", "logo", "banner", "disc", "clearart":
		return true
	default:
		return false
	}
}

func localArtworkNamesFromStems(stems ...string) []string {
	imageExts := []string{".jpg", ".jpeg", ".png", ".webp"}
	var out []string
	for _, stem := range stems {
		stem = strings.TrimSpace(stem)
		if stem == "" {
			continue
		}
		for _, ext := range imageExts {
			out = append(out, stem+ext)
		}
	}
	return uniqueNonEmptyStrings(out)
}

func isSeasonLikeFolder(name string) bool {
	name = strings.TrimSpace(name)
	if seasonFolderPattern.MatchString(name) {
		return true
	}
	switch strings.ToLower(name) {
	case "special", "specials", "extras":
		return true
	default:
		return false
	}
}

func sidecarSubtitleCandidates(mediaPath string) []string {
	dir := filepath.Dir(mediaPath)
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	prefix := strings.ToLower(base)
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !sidecarSubtitleExtensions[ext] {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		lowerStem := strings.ToLower(stem)
		if lowerStem == prefix || strings.HasPrefix(lowerStem, prefix+".") || strings.HasPrefix(lowerStem, prefix+"-") || strings.HasPrefix(lowerStem, prefix+"_") {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

func replaceScannedSidecarLyrics(tx *sql.Tx, file scannerMediaFile, now string) error {
	return replaceScannedLyrics(tx, file, now, sidecarLyricProvider{})
}

type LyricProvider interface {
	ID() string
	CandidatePaths(file scannerMediaFile) []string
}

type sidecarLyricProvider struct{}

func (sidecarLyricProvider) ID() string { return "sidecar" }

func (sidecarLyricProvider) CandidatePaths(file scannerMediaFile) []string {
	return sidecarLyricCandidates(file.SourcePath)
}

func replaceScannedLyrics(tx *sql.Tx, file scannerMediaFile, now string, providers ...LyricProvider) error {
	if file.SourcePath == "" || file.ID == "" || file.Type != "track" {
		return nil
	}
	if _, err := tx.Exec(`DELETE FROM media_lyrics WHERE media_id = ? AND source = 'local'`, file.ID); err != nil {
		return err
	}
	for _, prepared := range file.PreparedLyrics {
		id := randomOpaquePublicID()
		synced := 0
		if prepared.Synced {
			synced = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO media_lyrics (id, media_id, source, provider, format, language, path, text, synced, created_at)
			VALUES (?, ?, 'local', '', ?, ?, ?, ?, ?, ?)
			ON CONFLICT(media_id, source, path, language, format) DO UPDATE SET
				text = excluded.text,
				synced = excluded.synced,
				created_at = excluded.created_at`,
			id, file.ID, prepared.Format, prepared.Language, prepared.CandidatePath, prepared.Text, synced, now); err != nil {
			return err
		}
	}
	return nil
}

func sidecarLyricCandidates(mediaPath string) []string {
	dir := filepath.Dir(mediaPath)
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	prefix := strings.ToLower(base)
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !sidecarLyricExtensions[ext] {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		lowerStem := strings.ToLower(stem)
		if lowerStem == prefix || strings.HasPrefix(lowerStem, prefix+".") || strings.HasPrefix(lowerStem, prefix+"-") || strings.HasPrefix(lowerStem, prefix+"_") {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

func sidecarLyricLanguage(lyricPath, mediaPath string) string {
	stem := strings.TrimSuffix(filepath.Base(lyricPath), filepath.Ext(lyricPath))
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	suffix := strings.Trim(strings.TrimPrefix(stem, base), ".-_ ")
	for _, part := range strings.FieldsFunc(suffix, func(r rune) bool { return r == '.' || r == '-' || r == '_' || r == ' ' }) {
		part = strings.ToLower(strings.TrimSpace(part))
		if len(part) >= 2 && len(part) <= 3 && part != "lyrics" {
			return part
		}
	}
	return "und"
}

func bytesLookLikeLRC(bytes []byte) bool {
	return regexp.MustCompile(`(?m)\[\d{1,2}:\d{2}(?:\.\d{1,3})?\]`).Match(bytes)
}

func sidecarSubtitleLanguage(subtitlePath, mediaPath string) (string, bool) {
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	stem := strings.TrimSuffix(filepath.Base(subtitlePath), filepath.Ext(subtitlePath))
	suffix := strings.TrimPrefix(stem, base)
	suffix = strings.TrimLeft(suffix, ".-_ ")
	parts := strings.FieldsFunc(strings.ToLower(suffix), func(r rune) bool {
		return r == '.' || r == '-' || r == '_' || r == ' '
	})
	language := "und"
	forced := false
	for _, part := range parts {
		switch part {
		case "forced", "force":
			forced = true
		case "sdh", "cc", "hi", "default":
			continue
		default:
			if language == "und" && len(part) >= 2 && len(part) <= 3 {
				language = part
			}
		}
	}
	return language, forced
}

func upsertScannerMetadataHints(tx *sql.Tx, file scannerMediaFile, now string) error {
	if err := upsertScannerIdentityEvidence(tx, file, now); err != nil {
		return err
	}
	return upsertLocalMetadataHints(tx, file.ID, file.Type, file.LocalMetadata, now)
}

func upsertLocalMetadataHints(tx *sql.Tx, mediaID, mediaType string, metadata scannerLocalMetadata, now string) error {
	if metadata.Source == "" {
		return nil
	}
	raw, _ := json.Marshal(metadata)
	if _, err := tx.Exec(`
		INSERT INTO media_scanner_hints (media_id, source, title, year, raw_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_id, source) DO UPDATE SET
			title = excluded.title,
			year = excluded.year,
			raw_json = excluded.raw_json,
			updated_at = excluded.updated_at`,
		mediaID, metadata.Source, metadata.Title, metadata.Year, string(raw), now); err != nil {
		return err
	}
	for provider, externalID := range metadata.ProviderIDs {
		provider, externalType := scannerProviderIDTarget(provider, mediaType)
		externalID = strings.TrimSpace(externalID)
		if provider == "" || externalID == "" {
			continue
		}
		if err := upsertMediaProviderIdentityTx(tx, mediaID, provider, externalID, externalType, 1.0, metadata.Source, false, "", now); err != nil {
			return err
		}
	}
	return nil
}

func scannerProviderIDTarget(providerKey, mediaType string) (string, string) {
	providerKey = strings.TrimSpace(providerKey)
	if before, after, ok := strings.Cut(providerKey, ":"); ok {
		provider := normalizedMetadataProvider(before)
		externalType := strings.TrimSpace(after)
		if provider == "musicbrainz" && externalType == "album-artist" {
			externalType = "artist"
		}
		if externalType == "" {
			externalType = providerExternalType(provider, mediaType)
		}
		return provider, externalType
	}
	provider := normalizedMetadataProvider(providerKey)
	return provider, providerExternalType(provider, mediaType)
}

func scannerProviderEvidenceField(providerKey string) string {
	provider, externalType := scannerProviderIDTarget(providerKey, "")
	if externalType != "" {
		return "provider:" + provider + ":" + externalType
	}
	return "provider:" + provider
}

func upsertScannerIdentityEvidence(tx *sql.Tx, file scannerMediaFile, now string) error {
	pathSource := "path:" + file.Type
	pathRaw := map[string]any{
		"title":            file.Title,
		"parentTitle":      file.ParentTitle,
		"grandparentTitle": file.GrandparentTitle,
		"seasonNumber":     file.SeasonNumber,
		"episodeNumber":    file.EpisodeNumber,
		"indexNumber":      file.IndexNumber,
		"path":             file.SourcePath,
	}
	if err := upsertIdentityEvidenceTx(tx, file.ID, pathSource, "title", file.Title, 0.55, file.SourcePath, pathRaw, now); err != nil {
		return err
	}
	if file.ParentTitle != "" {
		if err := upsertIdentityEvidenceTx(tx, file.ID, pathSource, "parent_title", file.ParentTitle, 0.5, file.SourcePath, pathRaw, now); err != nil {
			return err
		}
	}
	if file.GrandparentTitle != "" {
		if err := upsertIdentityEvidenceTx(tx, file.ID, pathSource, "grandparent_title", file.GrandparentTitle, 0.5, file.SourcePath, pathRaw, now); err != nil {
			return err
		}
	}
	if file.SeasonNumber > 0 {
		if err := upsertIdentityEvidenceTx(tx, file.ID, pathSource, "season_number", strconv.Itoa(file.SeasonNumber), 0.7, file.SourcePath, pathRaw, now); err != nil {
			return err
		}
	}
	if file.EpisodeNumber > 0 {
		if err := upsertIdentityEvidenceTx(tx, file.ID, pathSource, "episode_number", strconv.Itoa(file.EpisodeNumber), 0.7, file.SourcePath, pathRaw, now); err != nil {
			return err
		}
	}
	if file.IndexNumber > 0 {
		if err := upsertIdentityEvidenceTx(tx, file.ID, pathSource, "index_number", strconv.Itoa(file.IndexNumber), 0.65, file.SourcePath, pathRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.Year > 0 {
		if err := upsertIdentityEvidenceTx(tx, file.ID, pathSource, "year", strconv.Itoa(file.LocalMetadata.Year), 0.65, file.SourcePath, pathRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.Edition != "" {
		if err := upsertIdentityEvidenceTx(tx, file.ID, pathSource, "edition", file.LocalMetadata.Edition, 0.55, file.SourcePath, pathRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.Source == "" {
		return nil
	}
	localRaw := file.LocalMetadata
	if file.LocalMetadata.Title != "" {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, "title", file.LocalMetadata.Title, 0.9, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.Year > 0 {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, "year", strconv.Itoa(file.LocalMetadata.Year), 0.9, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.Artist != "" {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, "artist", file.LocalMetadata.Artist, 0.9, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.AlbumArtist != "" {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, "album_artist", file.LocalMetadata.AlbumArtist, 0.9, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.AlbumTitle != "" {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, "album_title", file.LocalMetadata.AlbumTitle, 0.9, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.TrackNumber > 0 {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, "track_number", strconv.Itoa(file.LocalMetadata.TrackNumber), 0.9, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.TrackCount > 0 {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, "track_count", strconv.Itoa(file.LocalMetadata.TrackCount), 0.85, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.DiscNumber > 0 {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, "disc_number", strconv.Itoa(file.LocalMetadata.DiscNumber), 0.9, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.DiscCount > 0 {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, "disc_count", strconv.Itoa(file.LocalMetadata.DiscCount), 0.85, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	if file.LocalMetadata.BPM > 0 {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, "bpm", strconv.Itoa(file.LocalMetadata.BPM), 0.75, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	for provider, externalID := range file.LocalMetadata.ProviderIDs {
		if err := upsertIdentityEvidenceTx(tx, file.ID, file.LocalMetadata.Source, scannerProviderEvidenceField(provider), externalID, 1.0, file.SourcePath, localRaw, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) markMissingScannedMediaFilesBatch(ctx context.Context, libraryID string, now string, scanGeneration string, limit int) (int, error) {
	if limit <= 0 {
		limit = scannerReconcileBatchSize
	}
	missing := 0
	err := s.withBackgroundTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT id, media_id
			FROM media_files
			WHERE library_id = ?
				AND available = 1
				AND source_type NOT IN ('rclone','webdav')
				AND COALESCE(scan_generation, '') <> ?
			ORDER BY id ASC
			LIMIT ?`, libraryID, scanGeneration, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		fileIDs := []string{}
		mediaIDSet := map[string]bool{}
		mediaIDs := []string{}
		for rows.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var fileID string
			var mediaID string
			if err := rows.Scan(&fileID, &mediaID); err != nil {
				return err
			}
			fileIDs = append(fileIDs, fileID)
			if mediaID != "" && !mediaIDSet[mediaID] {
				mediaIDSet[mediaID] = true
				mediaIDs = append(mediaIDs, mediaID)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(fileIDs) == 0 {
			return nil
		}
		args := make([]any, 0, len(fileIDs)+1)
		args = append(args, now)
		for _, fileID := range fileIDs {
			args = append(args, fileID)
		}
		result, err := tx.Exec(`
			UPDATE media_files
			SET available = 0,
				missing_since = CASE WHEN missing_since = '' THEN ? ELSE missing_since END
			WHERE id IN (`+sqlPlaceholders(len(fileIDs))+`)`, args...)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		missing = int(affected)
		if len(mediaIDs) == 0 {
			return nil
		}
		mediaArgs := make([]any, 0, len(mediaIDs))
		for _, mediaID := range mediaIDs {
			mediaArgs = append(mediaArgs, mediaID)
		}
		_, err = tx.Exec(`
			UPDATE media_items
			SET source_url = COALESCE((
				SELECT path
				FROM media_files
				WHERE media_id = media_items.id AND available = 1
				ORDER BY quality_rank DESC, size_bytes DESC, path ASC
				LIMIT 1
			), '')
			WHERE id IN (`+sqlPlaceholders(len(mediaIDs))+`)`, mediaArgs...)
		return err
	})
	return missing, err
}

func shouldSkipScanDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".svn", "@eadir", "#recycle", ".stfolder", "node_modules", "sample", "samples":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

func isMediaFileForLibrary(libraryType, path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if strmExtensions[ext] {
		return true
	}
	switch libraryType {
	case "music":
		return audioExtensions[ext]
	case "audiobook":
		return audioExtensions[ext] || audiobookExtensions[ext]
	default:
		return videoExtensions[ext] || (libraryType == "movie" && ext == ".iso")
	}
}

func pathWithinRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func scannedID(prefix, value string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(value)))
	return prefix + "_" + hex.EncodeToString(sum[:])[:24]
}

func sortableTitle(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, article) {
			return strings.TrimSpace(value[len(article):])
		}
	}
	return value
}

func trackNumberFromName(base string) int {
	fields := strings.FieldsFunc(base, func(r rune) bool {
		return r == ' ' || r == '.' || r == '_' || r == '-'
	})
	if len(fields) == 0 {
		return 0
	}
	value, _ := strconv.Atoi(fields[0])
	return value
}

func extraInfoFromPath(rel string) (string, string, bool) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 3 {
		return "", "", false
	}
	for i, part := range parts[:len(parts)-1] {
		kind := normalizeExtraKind(part)
		if kind == "" || i == 0 {
			continue
		}
		return kind, parts[i-1], true
	}
	return "", "", false
}

func normalizeExtraKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "trailer", "trailers":
		return "trailer"
	case "featurette", "featurettes", "behind the scenes", "behind-the-scenes", "extras", "bonus", "bonus features", "deleted scenes":
		return "extra"
	default:
		return ""
	}
}

func extraSortOrder(kind string) int {
	if kind == "trailer" {
		return 10
	}
	return 20
}

func codecForExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".mkv", ".webm":
		return "matroska"
	case ".mp4", ".m4v", ".m4a", ".m4b":
		return "mp4"
	case ".flac":
		return "flac"
	case ".mp3":
		return "mp3"
	case ".wav":
		return "pcm"
	default:
		return strings.TrimPrefix(strings.ToLower(ext), ".")
	}
}

func scannedVideoCodecForExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v", ".mov", ".mkv":
		return "h264"
	case ".webm":
		return "vp9"
	default:
		return ""
	}
}

func scannedAudioCodecForExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v", ".mov", ".mkv", ".webm":
		return "aac"
	default:
		return ""
	}
}

type nfoDocument struct {
	XMLName       xml.Name      `xml:""`
	Title         string        `xml:"title"`
	LocalTitle    string        `xml:"localtitle"`
	SortTitle     string        `xml:"sorttitle"`
	OriginalTitle string        `xml:"originaltitle"`
	ShowTitle     string        `xml:"showtitle"`
	Year          int           `xml:"year"`
	Premiered     string        `xml:"premiered"`
	Released      string        `xml:"releasedate"`
	Aired         string        `xml:"aired"`
	DateAdded     string        `xml:"dateadded"`
	Season        int           `xml:"season"`
	Episode       int           `xml:"episode"`
	Runtime       int           `xml:"runtime"`
	Set           string        `xml:"set"`
	Plot          string        `xml:"plot"`
	Outline       string        `xml:"outline"`
	Tagline       string        `xml:"tagline"`
	MPAA          string        `xml:"mpaa"`
	ContentRating string        `xml:"contentrating"`
	Rating        float64       `xml:"rating"`
	Votes         int           `xml:"votes"`
	Ratings       []nfoRating   `xml:"ratings>rating"`
	UserRating    float64       `xml:"userrating"`
	CriticRating  int           `xml:"criticrating"`
	Studios       []string      `xml:"studio"`
	Network       string        `xml:"network"`
	Countries     []string      `xml:"country"`
	TMDBID        string        `xml:"tmdbid"`
	IMDBID        string        `xml:"imdbid"`
	TVDBID        string        `xml:"tvdbid"`
	Genres        []string      `xml:"genre"`
	Tags          []string      `xml:"tag"`
	Directors     []string      `xml:"director"`
	Writers       []string      `xml:"writer"`
	Credits       []string      `xml:"credits"`
	Producers     []string      `xml:"producer"`
	Actors        []nfoActor    `xml:"actor"`
	UniqueIDs     []nfoUniqueID `xml:"uniqueid"`
}

type nfoActor struct {
	Name      string        `xml:"name"`
	Role      string        `xml:"role"`
	Order     int           `xml:"order"`
	Thumb     string        `xml:"thumb"`
	TMDB      string        `xml:"tmdbid"`
	IMDB      string        `xml:"imdbid"`
	TVDB      string        `xml:"tvdbid"`
	UniqueIDs []nfoUniqueID `xml:"uniqueid"`
}

type nfoRating struct {
	Name  string  `xml:"name,attr"`
	Value float64 `xml:"value"`
	Votes int     `xml:"votes"`
}

type nfoUniqueID struct {
	Type    string `xml:"type,attr"`
	Default string `xml:"default,attr"`
	Value   string `xml:",chardata"`
}

type LocalMetadataProvider interface {
	ID() string
	CandidatePaths(path string) []string
	Read(path string) (scannerLocalMetadata, bool)
}

type nfoLocalMetadataProvider struct{}
type opfLocalMetadataProvider struct{}

func (nfoLocalMetadataProvider) ID() string { return "nfo" }

func (nfoLocalMetadataProvider) CandidatePaths(path string) []string {
	return nfoCandidatePaths(path)
}

func (nfoLocalMetadataProvider) Read(path string) (scannerLocalMetadata, bool) {
	return readNFO(path)
}

func (opfLocalMetadataProvider) ID() string { return "opf" }

func (opfLocalMetadataProvider) CandidatePaths(path string) []string {
	return opfCandidatePaths(path)
}

func (opfLocalMetadataProvider) Read(path string) (scannerLocalMetadata, bool) {
	return readOPF(path)
}

func localMetadataForMediaFile(path, root string) scannerLocalMetadata {
	return firstLocalMetadataFromProviders(path, root, nfoLocalMetadataProvider{})
}

func showLocalMetadataForFile(file scannerMediaFile) scannerLocalMetadata {
	showDir := filepath.Dir(filepath.Dir(file.SourcePath))
	if strings.Contains(strings.ToLower(filepath.Base(filepath.Dir(file.SourcePath))), "season") || seasonFolderPattern.MatchString(filepath.Base(filepath.Dir(file.SourcePath))) || strings.EqualFold(filepath.Base(filepath.Dir(file.SourcePath)), "Specials") {
		return firstLocalMetadataFromPaths(file.ScanRoot, filepath.Join(showDir, "tvshow.nfo"), filepath.Join(showDir, "show.nfo"))
	}
	return firstLocalMetadataFromPaths(file.ScanRoot, filepath.Join(filepath.Dir(file.SourcePath), "tvshow.nfo"), filepath.Join(filepath.Dir(file.SourcePath), "show.nfo"))
}

func seasonLocalMetadataForFile(file scannerMediaFile) scannerLocalMetadata {
	return firstLocalMetadataFromPaths(file.ScanRoot, filepath.Join(filepath.Dir(file.SourcePath), "season.nfo"))
}

func albumLocalMetadataForFile(file scannerMediaFile) scannerLocalMetadata {
	return firstLocalMetadataFromPaths(file.ScanRoot, filepath.Join(filepath.Dir(file.SourcePath), "album.nfo"))
}

func artistLocalMetadataForFile(file scannerMediaFile) scannerLocalMetadata {
	artistDir := filepath.Dir(filepath.Dir(file.SourcePath))
	return firstLocalMetadataFromPaths(file.ScanRoot, filepath.Join(artistDir, "artist.nfo"))
}

func audiobookLocalMetadataForFile(path, root string) scannerLocalMetadata {
	return firstLocalMetadataFromProviders(path, root, opfLocalMetadataProvider{})
}

func firstLocalMetadataFromPaths(root string, paths ...string) scannerLocalMetadata {
	return firstLocalMetadataFromCandidatePaths(root, nfoLocalMetadataProvider{}, paths...)
}

func firstLocalMetadataFromProviders(path, root string, providers ...LocalMetadataProvider) scannerLocalMetadata {
	for _, provider := range providers {
		if metadata := firstLocalMetadataFromCandidatePaths(root, provider, provider.CandidatePaths(path)...); metadata.Source != "" {
			return metadata
		}
	}
	return scannerLocalMetadata{}
}

func firstLocalMetadataFromCandidatePaths(root string, provider LocalMetadataProvider, paths ...string) scannerLocalMetadata {
	for _, path := range paths {
		resolved, err := resolveScannerPathWithinRoot(root, path)
		if err != nil {
			continue
		}
		if metadata, ok := provider.Read(resolved); ok {
			return metadata
		}
	}
	return scannerLocalMetadata{}
}

func opfCandidatePaths(path string) []string {
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	candidates := []string{
		filepath.Join(dir, base+".opf"),
		filepath.Join(dir, "metadata.opf"),
		filepath.Join(dir, "book.opf"),
	}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".opf") {
				candidates = append(candidates, filepath.Join(dir, entry.Name()))
			}
		}
	}
	return uniqueNonEmptyStrings(candidates)
}

func nfoCandidatePaths(path string) []string {
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return uniqueNonEmptyStrings([]string{
		filepath.Join(dir, base+".nfo"),
		filepath.Join(dir, "movie.nfo"),
		filepath.Join(dir, "episode.nfo"),
		filepath.Join(dir, "VIDEO_TS.nfo"),
		filepath.Join(dir, "video_ts.nfo"),
	})
}

func readNFO(path string) (scannerLocalMetadata, bool) {
	bytes, err := readBoundedRegularFile(path, scannerLocalMetadataLimit)
	if err != nil || len(bytes) == 0 {
		return scannerLocalMetadata{}, false
	}
	var doc nfoDocument
	if err := xml.Unmarshal(bytes, &doc); err != nil {
		return scannerLocalMetadata{}, false
	}
	metadata := scannerLocalMetadata{
		Title:           firstNonEmpty(strings.TrimSpace(doc.LocalTitle), strings.TrimSpace(doc.Title)),
		LocalTitle:      strings.TrimSpace(doc.LocalTitle),
		SortTitle:       strings.TrimSpace(doc.SortTitle),
		OriginalTitle:   strings.TrimSpace(doc.OriginalTitle),
		ShowTitle:       strings.TrimSpace(doc.ShowTitle),
		ExactDate:       exactDateFromNFO(firstNonEmpty(doc.Premiered, doc.Released, doc.Aired, doc.DateAdded)),
		Year:            firstPositiveInt(doc.Year, yearFromNFODate(doc.Premiered), yearFromNFODate(doc.Released), yearFromNFODate(doc.Aired)),
		SeasonNumber:    doc.Season,
		EpisodeNumber:   doc.Episode,
		RuntimeMinutes:  doc.Runtime,
		Collection:      strings.TrimSpace(doc.Set),
		Summary:         firstNonEmpty(strings.TrimSpace(doc.Plot), strings.TrimSpace(doc.Outline)),
		Tagline:         strings.TrimSpace(doc.Tagline),
		ContentRating:   firstNonEmpty(strings.TrimSpace(doc.ContentRating), strings.TrimSpace(doc.MPAA)),
		CommunityRating: firstPositiveFloat(doc.UserRating, doc.Rating),
		CriticRating:    doc.CriticRating,
		Studios:         normalizeStringList(doc.Studios),
		Network:         strings.TrimSpace(doc.Network),
		Countries:       normalizeStringList(doc.Countries),
		Genres:          normalizeStringList(doc.Genres),
		Tags:            normalizeStringList(doc.Tags),
		People:          peopleFromNFO(doc),
		ProviderIDs:     map[string]string{},
		Source:          "nfo:" + path,
	}
	metadata.Studio = firstListValue(metadata.Studios)
	metadata.Country = firstListValue(metadata.Countries)
	for index := range metadata.People {
		thumb := strings.TrimSpace(metadata.People[index].ImageURL)
		if thumb == "" {
			continue
		}
		candidate := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(thumb)))
		_, candidateErr := resolveScannerPathWithinRoot(filepath.Dir(path), candidate)
		if candidateErr != nil {
			metadata.People[index].ImageURL = ""
			continue
		}
		metadata.People[index].ImageURL = candidate
	}
	for _, rating := range doc.Ratings {
		if rating.Value > 0 {
			metadata.RatingName = strings.TrimSpace(rating.Name)
			metadata.CommunityRating = rating.Value
			metadata.RatingVotes = rating.Votes
			break
		}
	}
	if metadata.RatingVotes == 0 {
		metadata.RatingVotes = doc.Votes
	}
	if value := strings.TrimSpace(doc.TMDBID); value != "" {
		metadata.ProviderIDs["tmdb"] = value
	}
	if value := strings.TrimSpace(doc.IMDBID); value != "" {
		metadata.ProviderIDs["imdb"] = value
	}
	if value := strings.TrimSpace(doc.TVDBID); value != "" {
		metadata.ProviderIDs["tvdb"] = value
	}
	for _, id := range doc.UniqueIDs {
		provider := normalizedMetadataProvider(id.Type)
		value := strings.TrimSpace(id.Value)
		if provider != "" && value != "" {
			metadata.ProviderIDs[provider] = value
		}
	}
	if metadata.Title == "" && metadata.Summary == "" && len(metadata.ProviderIDs) == 0 {
		return scannerLocalMetadata{}, false
	}
	return metadata, true
}

type opfPackage struct {
	Metadata opfMetadata `xml:"metadata"`
	Manifest opfManifest `xml:"manifest"`
}

type opfMetadata struct {
	Title       string          `xml:"title"`
	Creators    []opfText       `xml:"creator"`
	Description string          `xml:"description"`
	Subjects    []string        `xml:"subject"`
	Dates       []string        `xml:"date"`
	Identifiers []opfIdentifier `xml:"identifier"`
	Meta        []opfMeta       `xml:"meta"`
}

type opfText struct {
	Role  string `xml:"role,attr"`
	Value string `xml:",chardata"`
}

type opfIdentifier struct {
	Scheme string `xml:"scheme,attr"`
	ID     string `xml:"id,attr"`
	Value  string `xml:",chardata"`
}

type opfMeta struct {
	Name     string `xml:"name,attr"`
	Property string `xml:"property,attr"`
	Content  string `xml:"content,attr"`
	Value    string `xml:",chardata"`
}

type opfManifest struct {
	Items []opfManifestItem `xml:"item"`
}

type opfManifestItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

func readOPF(path string) (scannerLocalMetadata, bool) {
	bytes, err := readBoundedRegularFile(path, scannerLocalMetadataLimit)
	if err != nil || len(bytes) == 0 {
		return scannerLocalMetadata{}, false
	}
	var doc opfPackage
	if err := xml.Unmarshal(bytes, &doc); err != nil {
		return scannerLocalMetadata{}, false
	}
	meta := doc.Metadata
	author := ""
	narrator := ""
	for _, creator := range meta.Creators {
		value := strings.TrimSpace(creator.Value)
		if value == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(creator.Role))
		if role == "nrt" || strings.Contains(role, "narr") {
			narrator = firstNonEmpty(narrator, value)
		} else {
			author = firstNonEmpty(author, value)
		}
	}
	metadata := scannerLocalMetadata{
		Title:          strings.TrimSpace(meta.Title),
		SortTitle:      sortableTitle(meta.Title),
		Summary:        strings.TrimSpace(meta.Description),
		Artist:         author,
		Studio:         firstNonEmpty(narrator, author),
		Genres:         normalizeStringList(meta.Subjects),
		Year:           firstOPFYear(meta.Dates),
		Series:         opfMetaValue(meta.Meta, "calibre:series", "series"),
		SeriesIndex:    opfMetaValue(meta.Meta, "calibre:series_index", "series_index", "seriesindex"),
		AuthorProvider: opfMetaValue(meta.Meta, "portico:author_provider", "author_provider", "authorprovider"),
		AuthorID:       opfMetaValue(meta.Meta, "portico:author_id", "author_id", "authorid"),
		SeriesProvider: opfMetaValue(meta.Meta, "portico:series_provider", "series_provider", "seriesprovider"),
		SeriesID:       opfMetaValue(meta.Meta, "portico:series_id", "calibre:series_id", "series_id", "seriesid"),
		ProviderIDs:    map[string]string{},
		Source:         "opf:" + path,
	}
	if coverPath := opfCoverPath(path, doc); coverPath != "" {
		metadata.ImagePaths = map[string]string{"poster": coverPath}
	}
	if narrator == "" {
		metadata.Studio = firstNonEmpty(opfMetaValue(meta.Meta, "narrator", "narrators"), metadata.Studio)
	}
	for _, identifier := range meta.Identifiers {
		scheme := strings.ToLower(firstNonEmpty(identifier.Scheme, identifier.ID))
		value := strings.TrimSpace(identifier.Value)
		if value == "" {
			continue
		}
		if strings.Contains(scheme, "isbn") || looksLikeISBN(value) {
			metadata.ProviderIDs["isbn:isbn"] = value
		}
	}
	if metadata.Title == "" && metadata.Summary == "" && metadata.Artist == "" && len(metadata.ProviderIDs) == 0 {
		return scannerLocalMetadata{}, false
	}
	return metadata, true
}

func opfCoverPath(opfPath string, doc opfPackage) string {
	coverID := opfMetaValue(doc.Metadata.Meta, "cover")
	for _, item := range doc.Manifest.Items {
		id := strings.TrimSpace(item.ID)
		href := strings.TrimSpace(item.Href)
		if href == "" {
			continue
		}
		properties := strings.ToLower(item.Properties)
		mediaType := strings.ToLower(item.MediaType)
		isCover := coverID != "" && id == coverID
		isCover = isCover || strings.Contains(properties, "cover-image")
		isCover = isCover || (strings.Contains(strings.ToLower(id), "cover") && strings.HasPrefix(mediaType, "image/"))
		if !isCover {
			continue
		}
		if path := resolveOPFRelativePath(opfPath, href); path != "" {
			return path
		}
	}
	return ""
}

func resolveOPFRelativePath(opfPath, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if parsed, err := url.Parse(href); err == nil && parsed.Scheme != "" {
		return ""
	}
	dir := filepath.Dir(opfPath)
	candidate := filepath.Clean(filepath.Join(dir, filepath.FromSlash(href)))
	_, err := resolveScannerPathWithinRoot(dir, candidate)
	if err != nil {
		return ""
	}
	return candidate
}

func opfMetaValue(values []opfMeta, names ...string) string {
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(firstNonEmpty(value.Name, value.Property)))
		for _, name := range names {
			if key == strings.ToLower(name) {
				return strings.TrimSpace(firstNonEmpty(value.Content, value.Value))
			}
		}
	}
	return ""
}

func firstOPFYear(values []string) int {
	for _, value := range values {
		if year := yearFromNFODate(value); year > 0 {
			return year
		}
	}
	return 0
}

func looksLikeISBN(value string) bool {
	digits := 0
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits++
		}
	}
	return digits == 10 || digits == 13
}

func peopleFromNFO(doc nfoDocument) []MediaPerson {
	var people []MediaPerson
	addNames := func(role string, names []string) {
		for _, name := range names {
			for _, part := range peopleNamesFromTag(name) {
				people = append(people, MediaPerson{Name: part, Role: role})
			}
		}
	}
	addNames("Director", doc.Directors)
	addNames("Writer", append(doc.Writers, doc.Credits...))
	addNames("Producer", doc.Producers)
	for _, actor := range doc.Actors {
		name := strings.TrimSpace(actor.Name)
		if name == "" {
			continue
		}
		person := MediaPerson{
			Name:        name,
			Role:        "Actor",
			Character:   strings.TrimSpace(actor.Role),
			SortOrder:   actor.Order,
			ProviderIDs: map[string]string{},
		}
		for provider, value := range map[string]string{"tmdb": actor.TMDB, "imdb": actor.IMDB, "tvdb": actor.TVDB} {
			if value = strings.TrimSpace(value); value != "" {
				person.ProviderIDs[provider] = value
			}
		}
		for _, uniqueID := range actor.UniqueIDs {
			provider, value := normalizedMetadataProvider(uniqueID.Type), strings.TrimSpace(uniqueID.Value)
			if provider != "" && value != "" {
				person.ProviderIDs[provider] = value
			}
		}
		// Actor artwork may only reference a local sidecar. Remote NFO URLs are
		// deliberately ignored so discovery cannot smuggle arbitrary URLs into
		// canonical person records.
		if thumb := strings.TrimSpace(actor.Thumb); thumb != "" && !strings.Contains(thumb, "://") && !filepath.IsAbs(thumb) {
			person.ImageURL = thumb
		}
		if len(person.ProviderIDs) == 0 {
			person.ProviderIDs = nil
		}
		people = append(people, person)
	}
	return people
}

func peopleNamesFromTag(value string) []string {
	return normalizeStringList(strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == '/' || r == '|'
	}))
}

func yearFromNFODate(value string) int {
	value = strings.TrimSpace(value)
	if len(value) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(value[:4])
	if year >= 1800 && year <= time.Now().Year()+5 {
		return year
	}
	return 0
}

func exactDateFromNFO(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 10 {
		if parsed, err := time.Parse("2006-01-02", value[:10]); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return ""
}

func firstListValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func providerExternalType(provider, mediaType string) string {
	if provider == "musicbrainz" {
		switch mediaType {
		case "artist":
			return "artist"
		case "album":
			return "release-group"
		case "track":
			return "recording"
		default:
			return ""
		}
	}
	if provider == "tmdb" {
		return tmdbSearchType(mediaType)
	}
	if provider == "tvdb" {
		return tvdbSearchType(mediaType)
	}
	return ""
}
