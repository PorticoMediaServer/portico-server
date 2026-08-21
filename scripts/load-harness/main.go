package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const csrfHeaderName = "X-Portico-CSRF"

type listResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

type libraryInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type mediaInfo struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Images  imageSet `json:"images"`
	Artwork imageSet `json:"artwork"`
}

type browseResponse struct {
	Items []mediaInfo `json:"items"`
}

type imageSet struct {
	Poster   string `json:"poster"`
	Backdrop string `json:"backdrop"`
	Thumb    string `json:"thumb"`
}

type playbackResponse struct {
	SessionID string `json:"sessionId"`
}

type sample struct {
	Name   string
	Method string
	Path   string
	Status int
	Bytes  int
	Took   time.Duration
	Err    string
}

type report struct {
	BaseURL             string                     `json:"baseUrl"`
	Users               int                        `json:"users"`
	Duration            string                     `json:"duration"`
	Requests            int                        `json:"requests"`
	Errors              int                        `json:"errors"`
	Bytes               int64                      `json:"bytes"`
	RequestsPerSecond   float64                    `json:"requestsPerSecond"`
	BytesPerSecond      float64                    `json:"bytesPerSecond"`
	P50Millis           int64                      `json:"p50Millis"`
	P95Millis           int64                      `json:"p95Millis"`
	P99Millis           int64                      `json:"p99Millis"`
	PeakPlayback        int64                      `json:"peakPlayback"`
	StatusCounts        map[int]int                `json:"statusCounts"`
	ByName              map[string]metric          `json:"byName"`
	DiagnosticsBefore   json.RawMessage            `json:"diagnosticsBefore,omitempty"`
	DiagnosticsAfter    json.RawMessage            `json:"diagnosticsAfter,omitempty"`
	DiagnosticsDelta    *diagnosticsDelta          `json:"diagnosticsDelta,omitempty"`
	DiagnosticsTimeline []diagnosticsTimelinePoint `json:"diagnosticsTimeline,omitempty"`
	Scenarios           []string                   `json:"scenarios,omitempty"`
	BudgetFailures      []string                   `json:"budgetFailures,omitempty"`
	StartedAt           string                     `json:"startedAt"`
	FinishedAt          string                     `json:"finishedAt"`
}

type metric struct {
	Requests  int   `json:"requests"`
	Errors    int   `json:"errors"`
	P95Millis int64 `json:"p95Millis"`
}

type diagnosticsSnapshot struct {
	Runtime struct {
		Goroutines     int    `json:"goroutines"`
		HeapAllocBytes uint64 `json:"heapAllocBytes"`
		NumGC          uint32 `json:"numGc"`
	} `json:"runtime"`
	SQLite struct {
		WaitCount              int64 `json:"waitCount"`
		WaitDurationMillis     int64 `json:"waitDurationMillis"`
		WriteOperations        int64 `json:"writeOperations"`
		WriteAttempts          int64 `json:"writeAttempts"`
		LockRetries            int64 `json:"lockRetries"`
		LockRetryWaitMillis    int64 `json:"lockRetryWaitMillis"`
		WALBytes               int64 `json:"walBytes"`
		SQLiteInUseConnections int   `json:"inUseConnections"`
	} `json:"sqlite"`
	Resources struct {
		Status                  string   `json:"status"`
		BackgroundJobsDeferred  bool     `json:"backgroundJobsDeferred"`
		ActivePlaybackSessions  int      `json:"activePlaybackSessions"`
		ActiveTranscodeSessions int      `json:"activeTranscodeSessions"`
		AvailableTranscodeSlots int      `json:"availableTranscodeSlots"`
		QueuedBackgroundJobs    int      `json:"queuedBackgroundJobs"`
		RunningBackgroundJobs   int      `json:"runningBackgroundJobs"`
		SaturatedWorkloadLanes  []string `json:"saturatedWorkloadLanes"`
		SaturatedJobLanes       []string `json:"saturatedJobLanes"`
		Signals                 []string `json:"signals"`
		DegradationActions      []string `json:"degradationActions"`
	} `json:"resources"`
	Admission struct {
		SearchActive          int    `json:"searchActive"`
		SearchRejected        uint64 `json:"searchRejected"`
		DownloadActive        int    `json:"downloadActive"`
		DownloadRejected      uint64 `json:"downloadRejected"`
		StreamActive          int    `json:"streamActive"`
		StreamRejected        uint64 `json:"streamRejected"`
		TranscodeActive       int    `json:"transcodeActive"`
		TranscodeRejected     uint64 `json:"transcodeRejected"`
		TranscodeUserRejected uint64 `json:"transcodeUserRejected"`
	} `json:"admission"`
	JobLanes []struct {
		ID      string `json:"id"`
		Queued  int    `json:"queued"`
		Running int    `json:"running"`
	} `json:"jobLanes"`
	WorkloadLanes []struct {
		ID       string `json:"id"`
		Active   int    `json:"active"`
		Capacity int    `json:"capacity"`
		Rejected uint64 `json:"rejected"`
	} `json:"workloadLanes"`
}

type diagnosticsDelta struct {
	Runtime                      runtimeDiagnosticsDelta `json:"runtime"`
	SQLite                       sqliteDiagnosticsDelta  `json:"sqlite"`
	ResourceStatusBefore         string                  `json:"resourceStatusBefore"`
	ResourceStatusAfter          string                  `json:"resourceStatusAfter"`
	BackgroundDeferredBefore     bool                    `json:"backgroundDeferredBefore"`
	BackgroundDeferredAfter      bool                    `json:"backgroundDeferredAfter"`
	ActivePlaybackDelta          int                     `json:"activePlaybackDelta"`
	ActiveTranscodeDelta         int                     `json:"activeTranscodeDelta"`
	AvailableTranscodeSlotsAfter int                     `json:"availableTranscodeSlotsAfter"`
	QueuedBackgroundJobsDelta    int                     `json:"queuedBackgroundJobsDelta"`
	RunningBackgroundJobsDelta   int                     `json:"runningBackgroundJobsDelta"`
	SaturatedWorkloadLanesAfter  []string                `json:"saturatedWorkloadLanesAfter"`
	SaturatedJobLanesAfter       []string                `json:"saturatedJobLanesAfter"`
	ResourceSignalsAfter         []string                `json:"resourceSignalsAfter"`
	DegradationActionsAfter      []string                `json:"degradationActionsAfter"`
	AdmissionRejectedDelta       admissionRejectedDelta  `json:"admissionRejectedDelta"`
	WorkloadRejectedDelta        map[string]uint64       `json:"workloadRejectedDelta"`
	JobQueuedDelta               map[string]int          `json:"jobQueuedDelta"`
	JobRunningDelta              map[string]int          `json:"jobRunningDelta"`
}

type runtimeDiagnosticsDelta struct {
	Goroutines     int   `json:"goroutines"`
	HeapAllocBytes int64 `json:"heapAllocBytes"`
	NumGC          int64 `json:"numGc"`
}

type sqliteDiagnosticsDelta struct {
	WaitCount           int64 `json:"waitCount"`
	WaitDurationMillis  int64 `json:"waitDurationMillis"`
	WriteOperations     int64 `json:"writeOperations"`
	WriteAttempts       int64 `json:"writeAttempts"`
	LockRetries         int64 `json:"lockRetries"`
	LockRetryWaitMillis int64 `json:"lockRetryWaitMillis"`
	WALBytes            int64 `json:"walBytes"`
}

type admissionRejectedDelta struct {
	Search        uint64 `json:"search"`
	Download      uint64 `json:"download"`
	Stream        uint64 `json:"stream"`
	Transcode     uint64 `json:"transcode"`
	TranscodeUser uint64 `json:"transcodeUser"`
}

type diagnosticsTimelinePoint struct {
	CapturedAt                string `json:"capturedAt"`
	Goroutines                int    `json:"goroutines"`
	HeapAllocBytes            uint64 `json:"heapAllocBytes"`
	NumGC                     uint32 `json:"numGc"`
	SQLiteWaitCount           int64  `json:"sqliteWaitCount"`
	SQLiteWaitDurationMillis  int64  `json:"sqliteWaitDurationMillis"`
	SQLiteLockRetries         int64  `json:"sqliteLockRetries"`
	SQLiteLockRetryWaitMillis int64  `json:"sqliteLockRetryWaitMillis"`
	SQLiteWALBytes            int64  `json:"sqliteWalBytes"`
	ResourceStatus            string `json:"resourceStatus"`
	BackgroundJobsDeferred    bool   `json:"backgroundJobsDeferred"`
	ActivePlaybackSessions    int    `json:"activePlaybackSessions"`
	ActiveTranscodeSessions   int    `json:"activeTranscodeSessions"`
	QueuedBackgroundJobs      int    `json:"queuedBackgroundJobs"`
	RunningBackgroundJobs     int    `json:"runningBackgroundJobs"`
	WorkloadRejectionsTotal   uint64 `json:"workloadRejectionsTotal"`
	AdmissionRejectionsTotal  uint64 `json:"admissionRejectionsTotal"`
}

func main() {
	baseURL := flag.String("base-url", "http://127.0.0.1:32500", "Portico server base URL")
	login := flag.String("login", "admin", "Portico login username or email")
	password := flag.String("password", "", "Portico password")
	users := flag.Int("users", 24, "number of concurrent virtual users")
	duration := flag.Duration("duration", 30*time.Second, "test duration")
	timeout := flag.Duration("timeout", 10*time.Second, "per-request timeout")
	profileFlag := flag.String("profile", "mixed", "comma-separated workload profile: mixed, browsing, playback, search, dashboard, download, stream, transcode")
	diagnosticsInterval := flag.Duration("diagnostics-interval", 0, "capture compact diagnostics timeline at this interval during the run; 0 disables")
	enablePlayback := flag.Bool("playback", true, "include playback start/heartbeat/end operations")
	scanDuringRun := flag.Bool("scan-during-run", false, "queue a library scan during the load run")
	metadataRefreshDuringRun := flag.Bool("metadata-refresh-during-run", false, "queue a metadata refresh job for the discovered media fixture during the load run")
	liveTVGuide := flag.Bool("live-tv-guide", false, "include Live TV source and guide requests when a Live TV source is configured")
	maxErrors := flag.Int("max-errors", 0, "maximum allowed request errors before non-zero exit")
	maxP95Millis := flag.Int64("max-p95-ms", 0, "maximum allowed overall p95 latency in milliseconds; 0 disables")
	maxSQLiteLockRetries := flag.Int64("max-sqlite-lock-retries", 0, "maximum allowed SQLite lock retry delta; 0 disables")
	maxWorkloadRejections := flag.Uint64("max-workload-rejections", 0, "maximum allowed workload rejection delta across all lanes; 0 disables")
	maxAdmissionRejections := flag.Uint64("max-admission-rejections", 0, "maximum allowed per-user admission rejection delta across search, downloads, direct streams, and transcodes; 0 disables")
	flag.Parse()

	if strings.TrimSpace(*password) == "" {
		fail("missing -password")
	}
	if *users < 1 {
		fail("-users must be at least 1")
	}
	if *duration <= 0 {
		fail("-duration must be positive")
	}
	if *diagnosticsInterval < 0 {
		fail("-diagnostics-interval cannot be negative")
	}
	profile, profileScenarios, err := parseLoadProfile(*profileFlag)
	if err != nil {
		fail("invalid -profile: %v", err)
	}
	if !*enablePlayback {
		profile.Playback = false
	}
	root, err := normalizeBaseURL(*baseURL)
	if err != nil {
		fail("invalid -base-url: %v", err)
	}

	coordinator, err := newVirtualUser(root, *login, *password, *timeout)
	if err != nil {
		fail("login coordinator: %v", err)
	}
	fixture, err := discoverFixture(coordinator)
	if err != nil {
		fail("discover libraries/media: %v", err)
	}
	diagnosticsBefore := coordinator.rawJSON("/api/system/diagnostics")

	started := time.Now().UTC()
	deadline := time.Now().Add(*duration)
	samples := make(chan sample, *users*32)
	var wg sync.WaitGroup
	var diagnosticsWG sync.WaitGroup
	var playback playbackTracker
	diagnosticsDone := make(chan struct{})
	diagnosticsTimeline := []diagnosticsTimelinePoint{}
	diagnosticsTimelineMu := sync.Mutex{}
	if *diagnosticsInterval > 0 {
		diagnosticsWG.Add(1)
		go func() {
			defer diagnosticsWG.Done()
			captureDiagnosticsTimeline(coordinator, *diagnosticsInterval, diagnosticsDone, &diagnosticsTimelineMu, &diagnosticsTimeline)
		}()
	}
	scenarios := append([]string(nil), profileScenarios...)
	if profile.Playback {
		scenarios = append(scenarios, "playback")
	}
	if *scanDuringRun {
		scenarios = append(scenarios, "library_scan_during_run")
		wg.Add(1)
		go func() {
			defer wg.Done()
			delay := minDuration(2*time.Second, *duration/4)
			if delay > 0 {
				time.Sleep(delay)
			}
			path := "/api/libraries/" + url.PathEscape(fixture.Library.ID) + "/scan"
			samples <- coordinator.doJSON("queue-library-scan", http.MethodPost, path, nil, nil)
		}()
	}
	if *metadataRefreshDuringRun {
		scenarios = append(scenarios, "metadata_refresh_during_run")
		wg.Add(1)
		go func() {
			defer wg.Done()
			delay := minDuration(3*time.Second, *duration/3)
			if delay > 0 {
				time.Sleep(delay)
			}
			mediaID := url.PathEscape(fixture.Media[0].ID)
			path := "/api/media/" + mediaID + "/jobs"
			samples <- coordinator.doJSON("queue-metadata-refresh", http.MethodPost, path, map[string]string{"type": "metadata_refresh"}, nil)
		}()
	}
	includeLiveTVGuide := *liveTVGuide && fixture.LiveTVSourceID != ""
	if includeLiveTVGuide {
		scenarios = append(scenarios, "live_tv_guide")
	} else if *liveTVGuide {
		scenarios = append(scenarios, "live_tv_guide_unavailable")
	}
	for i := 0; i < *users; i++ {
		user, err := newVirtualUser(root, *login, *password, *timeout)
		if err != nil {
			fail("login virtual user %d: %v", i+1, err)
		}
		wg.Add(1)
		go func(id int, vu *virtualUser) {
			defer wg.Done()
			runUser(id, vu, fixture, deadline, profile, includeLiveTVGuide, samples, &playback)
		}(i, user)
	}
	wg.Wait()
	close(diagnosticsDone)
	diagnosticsWG.Wait()
	close(samples)

	finished := time.Now().UTC()
	diagnosticsAfter := coordinator.rawJSON("/api/system/diagnostics")
	result := summarize(root, *users, *duration, started, finished, samples)
	result.PeakPlayback = playback.max.Load()
	result.DiagnosticsBefore = diagnosticsBefore
	result.DiagnosticsAfter = diagnosticsAfter
	result.DiagnosticsDelta = diagnosticsDeltaFor(diagnosticsBefore, diagnosticsAfter)
	result.DiagnosticsTimeline = diagnosticsTimeline
	result.Scenarios = scenarios
	budgets := loadBudgets{
		MaxErrors:              *maxErrors,
		MaxP95Millis:           *maxP95Millis,
		MaxSQLiteLockRetries:   *maxSQLiteLockRetries,
		MaxWorkloadRejections:  *maxWorkloadRejections,
		MaxAdmissionRejections: *maxAdmissionRejections,
	}
	result.BudgetFailures = budgetFailures(result, budgets)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fail("encode report: %v", err)
	}
	if len(result.BudgetFailures) > 0 {
		os.Exit(1)
	}
}

type loadBudgets struct {
	MaxErrors              int
	MaxP95Millis           int64
	MaxSQLiteLockRetries   int64
	MaxWorkloadRejections  uint64
	MaxAdmissionRejections uint64
}

func budgetFailures(result report, budgets loadBudgets) []string {
	failures := []string{}
	if result.Errors > budgets.MaxErrors {
		failures = append(failures, fmt.Sprintf("errors=%d exceeds max-errors=%d", result.Errors, budgets.MaxErrors))
	}
	if budgets.MaxP95Millis > 0 && result.P95Millis > budgets.MaxP95Millis {
		failures = append(failures, fmt.Sprintf("p95Millis=%d exceeds max-p95-ms=%d", result.P95Millis, budgets.MaxP95Millis))
	}
	if result.DiagnosticsDelta != nil {
		if budgets.MaxSQLiteLockRetries > 0 && result.DiagnosticsDelta.SQLite.LockRetries > budgets.MaxSQLiteLockRetries {
			failures = append(failures, fmt.Sprintf("sqlite.lockRetries=%d exceeds max-sqlite-lock-retries=%d", result.DiagnosticsDelta.SQLite.LockRetries, budgets.MaxSQLiteLockRetries))
		}
		rejections := totalWorkloadRejections(result.DiagnosticsDelta.WorkloadRejectedDelta)
		if budgets.MaxWorkloadRejections > 0 && rejections > budgets.MaxWorkloadRejections {
			failures = append(failures, fmt.Sprintf("workload.rejections=%d exceeds max-workload-rejections=%d", rejections, budgets.MaxWorkloadRejections))
		}
		admissionRejections := totalAdmissionRejections(result.DiagnosticsDelta.AdmissionRejectedDelta)
		if budgets.MaxAdmissionRejections > 0 && admissionRejections > budgets.MaxAdmissionRejections {
			failures = append(failures, fmt.Sprintf("admission.rejections=%d exceeds max-admission-rejections=%d", admissionRejections, budgets.MaxAdmissionRejections))
		}
	}
	return failures
}

func totalWorkloadRejections(rejections map[string]uint64) uint64 {
	var total uint64
	for _, count := range rejections {
		total += count
	}
	return total
}

func totalAdmissionRejections(rejections admissionRejectedDelta) uint64 {
	return rejections.Search + rejections.Download + rejections.Stream + rejections.Transcode + rejections.TranscodeUser
}

type fixtureData struct {
	Library        libraryInfo
	Media          []mediaInfo
	LiveTVSourceID string
}

type loadProfile struct {
	Browsing  bool
	Search    bool
	Dashboard bool
	Download  bool
	Stream    bool
	Artwork   bool
	Playback  bool
	Transcode bool
}

func parseLoadProfile(raw string) (loadProfile, []string, error) {
	tokens := strings.Split(strings.TrimSpace(raw), ",")
	if len(tokens) == 0 {
		tokens = []string{"mixed"}
	}
	profile := loadProfile{}
	scenarios := []string{}
	seen := map[string]bool{}
	for _, token := range tokens {
		name := strings.ToLower(strings.TrimSpace(token))
		if name == "" {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		scenarios = append(scenarios, "profile_"+name)
		switch name {
		case "mixed":
			profile.Browsing = true
			profile.Search = true
			profile.Dashboard = true
			profile.Download = true
			profile.Artwork = true
			profile.Playback = true
		case "browsing", "browse":
			profile.Browsing = true
			profile.Artwork = true
		case "playback":
			profile.Playback = true
		case "search":
			profile.Search = true
		case "dashboard", "admin":
			profile.Dashboard = true
		case "download", "downloads":
			profile.Download = true
		case "stream", "streams", "direct-stream", "direct-play":
			profile.Stream = true
		case "transcode", "transcodes":
			profile.Transcode = true
		default:
			return loadProfile{}, nil, fmt.Errorf("unknown profile %q", token)
		}
	}
	if !profile.Browsing && !profile.Search && !profile.Dashboard && !profile.Download && !profile.Stream && !profile.Artwork && !profile.Playback && !profile.Transcode {
		return loadProfile{}, nil, fmt.Errorf("at least one profile is required")
	}
	return profile, scenarios, nil
}

func discoverFixture(vu *virtualUser) (fixtureData, error) {
	var libraries listResponse[libraryInfo]
	if sample := vu.doJSON("libraries", http.MethodGet, "/api/libraries", nil, &libraries); sample.Err != "" || sample.Status != http.StatusOK {
		return fixtureData{}, fmt.Errorf("libraries status=%d error=%s", sample.Status, sample.Err)
	}
	if len(libraries.Items) == 0 {
		return fixtureData{}, fmt.Errorf("server has no libraries")
	}
	library := libraries.Items[0]
	var media browseResponse
	path := "/api/libraries/" + url.PathEscape(library.ID) + "/browse"
	payload := map[string]any{"pivot": defaultLibraryPivot(library.Type), "limit": 48, "sort": []map[string]string{{"field": "dateAdded", "direction": "desc"}}}
	if sample := vu.doJSON("library-items", http.MethodPost, path, payload, &media); sample.Err != "" || sample.Status != http.StatusOK {
		return fixtureData{}, fmt.Errorf("library items status=%d error=%s", sample.Status, sample.Err)
	}
	if len(media.Items) == 0 {
		return fixtureData{}, fmt.Errorf("library %q has no media items", library.Name)
	}
	fixture := fixtureData{Library: library, Media: media.Items}
	var liveTV listResponse[libraryInfo]
	if sample := vu.doJSON("live-tv-sources", http.MethodGet, "/api/live-tv", nil, &liveTV); sample.Err == "" && sample.Status == http.StatusOK && len(liveTV.Items) > 0 {
		fixture.LiveTVSourceID = liveTV.Items[0].ID
	}
	return fixture, nil
}

func defaultLibraryPivot(libraryType string) string {
	switch strings.ToLower(strings.TrimSpace(libraryType)) {
	case "tv", "show", "anime":
		return "shows"
	case "music":
		return "albums"
	case "audiobook", "audiobooks":
		return "books"
	case "recorded-tv", "recorded_tv":
		return "recordings"
	default:
		return "movies"
	}
}

type virtualUser struct {
	root   string
	client *http.Client
	rng    *rand.Rand
}

func newVirtualUser(root, login, password string, timeout time.Duration) (*virtualUser, error) {
	jar, _ := cookiejar.New(nil)
	vu := &virtualUser{
		root: root,
		client: &http.Client{
			Jar:     jar,
			Timeout: timeout,
		},
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	sample := vu.doJSON("login", http.MethodPost, "/api/auth/login", map[string]string{
		"login":    login,
		"password": password,
	}, nil)
	if sample.Err != "" {
		return nil, fmt.Errorf("%s", sample.Err)
	}
	if sample.Status != http.StatusOK {
		return nil, fmt.Errorf("login status=%d", sample.Status)
	}
	return vu, nil
}

type playbackTracker struct {
	active atomic.Int64
	max    atomic.Int64
}

func (tracker *playbackTracker) begin() {
	active := tracker.active.Add(1)
	for {
		previous := tracker.max.Load()
		if active <= previous || tracker.max.CompareAndSwap(previous, active) {
			return
		}
	}
}

func (tracker *playbackTracker) end() {
	tracker.active.Add(-1)
}

func runUser(id int, vu *virtualUser, fixture fixtureData, deadline time.Time, profile loadProfile, liveTVGuide bool, out chan<- sample, tracker *playbackTracker) {
	libraryID := url.PathEscape(fixture.Library.ID)
	iteration := 0
	for time.Now().Before(deadline) {
		item := fixture.Media[(id+iteration)%len(fixture.Media)]
		mediaID := url.PathEscape(item.ID)
		if profile.Browsing {
			requests := []struct {
				name    string
				method  string
				path    string
				payload any
			}{
				{"home", http.MethodGet, "/api/home", nil},
				{"library-page-added", http.MethodPost, "/api/libraries/" + libraryID + "/browse", map[string]any{"pivot": defaultLibraryPivot(fixture.Library.Type), "limit": 48, "sort": []map[string]string{{"field": "dateAdded", "direction": "desc"}}}},
				{"library-page-title", http.MethodPost, "/api/libraries/" + libraryID + "/browse", map[string]any{"pivot": defaultLibraryPivot(fixture.Library.Type), "limit": 48, "sort": []map[string]string{{"field": "title", "direction": "asc"}}}},
				{"categories", http.MethodGet, "/api/libraries/" + libraryID + "/categories", nil},
				{"detail", http.MethodGet, "/api/media/" + mediaID, nil},
			}
			for _, request := range requests {
				out <- vu.doJSON(request.name, request.method, request.path, request.payload, nil)
			}
		}
		if profile.Search {
			out <- vu.doJSON("search", http.MethodGet, "/api/search?q="+url.QueryEscape(searchTerm(item.Title)), nil, nil)
		}
		if profile.Dashboard {
			out <- vu.doJSON("settings", http.MethodGet, "/api/settings", nil, nil)
			out <- vu.doJSON("dashboard", http.MethodGet, "/api/dashboard?mode=live&period=5m", nil, nil)
		}
		if liveTVGuide {
			sourceID := url.PathEscape(fixture.LiveTVSourceID)
			out <- vu.doJSON("live-tv", http.MethodGet, "/api/live-tv", nil, nil)
			out <- vu.doJSON("live-tv-guide", http.MethodGet, "/api/live-tv/sources/"+sourceID+"/guide?hours=6", nil, nil)
		}
		if profile.Download {
			out <- vu.doJSON("download-head", http.MethodHead, "/api/media/"+mediaID+"/download", nil, nil)
		}
		if profile.Stream {
			out <- vu.doJSONWithHeaders("direct-stream-range", http.MethodGet, "/api/media/"+mediaID+"/stream", nil, nil, map[string]string{"Range": "bytes=0-65535"})
		}
		if profile.Artwork {
			artworkPath := firstNonEmpty(item.Artwork.Poster, item.Artwork.Thumb, item.Artwork.Backdrop, item.Images.Poster, item.Images.Thumb, item.Images.Backdrop)
			if artworkPath != "" {
				out <- vu.doJSON("artwork", http.MethodGet, artworkPath, nil, nil)
			}
		}
		if profile.Playback && iteration%3 == 0 {
			runPlayback(vu, item.ID, out, tracker)
		}
		if profile.Transcode && iteration%3 == 0 {
			runTranscode(vu, item.ID, out)
		}
		iteration++
	}
}

func runPlayback(vu *virtualUser, mediaID string, out chan<- sample, tracker *playbackTracker) {
	var started playbackResponse
	startSample := vu.doJSON("playback-start", http.MethodPost, "/api/playback/start", map[string]any{
		"mediaId":     mediaID,
		"skipPreroll": true,
		"clientProfile": map[string]any{
			"device":               "load-harness",
			"platform":             "web",
			"supportedContainers":  []string{"mp4"},
			"supportedVideoCodecs": []string{"h264"},
			"supportedAudioCodecs": []string{"aac"},
		},
	}, &started)
	out <- startSample
	if startSample.Err != "" || startSample.Status != http.StatusOK || started.SessionID == "" {
		return
	}
	tracker.begin()
	defer tracker.end()
	for i := 0; i < 2; i++ {
		out <- vu.doJSON("playback-heartbeat", http.MethodPatch, "/api/playback/"+url.PathEscape(started.SessionID), map[string]any{
			"state":           "playing",
			"progressSeconds": 30 + i*15,
			"durationSeconds": 7200,
			"bandwidthMbps":   8.5,
		}, nil)
	}
	out <- vu.doJSON("playback-end", http.MethodDelete, "/api/playback/"+url.PathEscape(started.SessionID), nil, nil)
}

func runTranscode(vu *virtualUser, mediaID string, out chan<- sample) {
	path := "/api/media/" + url.PathEscape(mediaID) + "/hls/master.m3u8?quality=720p-low"
	out <- vu.doJSON("transcode-manifest", http.MethodGet, path, nil, nil)
}

func (vu *virtualUser) doJSON(name, method, path string, payload any, out any) sample {
	return vu.doJSONWithHeaders(name, method, path, payload, out, nil)
}

func (vu *virtualUser) doJSONWithHeaders(name, method, path string, payload any, out any, headers map[string]string) sample {
	var body io.Reader
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return sample{Name: name, Method: method, Path: path, Err: err.Error()}
		}
		body = bytes.NewReader(payloadBytes)
	}
	endpoint := vu.root + path
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		endpoint = path
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return sample{Name: name, Method: method, Path: path, Err: err.Error()}
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set(csrfHeaderName, "1")
	}
	start := time.Now()
	resp, err := vu.client.Do(req)
	took := time.Since(start)
	item := sample{Name: name, Method: method, Path: path, Took: took}
	if err != nil {
		item.Err = err.Error()
		return item
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	item.Status = resp.StatusCode
	item.Bytes = len(responseBody)
	if err != nil {
		item.Err = err.Error()
		return item
	}
	if out != nil && len(responseBody) > 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.Unmarshal(responseBody, out); err != nil {
			item.Err = err.Error()
		}
	}
	return item
}

func (vu *virtualUser) rawJSON(path string) json.RawMessage {
	req, err := http.NewRequest(http.MethodGet, vu.root+path, nil)
	if err != nil {
		return nil
	}
	resp, err := vu.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	return json.RawMessage(body)
}

func diagnosticsDeltaFor(beforeRaw, afterRaw json.RawMessage) *diagnosticsDelta {
	if len(beforeRaw) == 0 || len(afterRaw) == 0 {
		return nil
	}
	var before diagnosticsSnapshot
	var after diagnosticsSnapshot
	if err := json.Unmarshal(beforeRaw, &before); err != nil {
		return nil
	}
	if err := json.Unmarshal(afterRaw, &after); err != nil {
		return nil
	}
	return &diagnosticsDelta{
		Runtime: runtimeDiagnosticsDelta{
			Goroutines:     after.Runtime.Goroutines - before.Runtime.Goroutines,
			HeapAllocBytes: int64(after.Runtime.HeapAllocBytes) - int64(before.Runtime.HeapAllocBytes),
			NumGC:          int64(after.Runtime.NumGC) - int64(before.Runtime.NumGC),
		},
		SQLite: sqliteDiagnosticsDelta{
			WaitCount:           after.SQLite.WaitCount - before.SQLite.WaitCount,
			WaitDurationMillis:  after.SQLite.WaitDurationMillis - before.SQLite.WaitDurationMillis,
			WriteOperations:     after.SQLite.WriteOperations - before.SQLite.WriteOperations,
			WriteAttempts:       after.SQLite.WriteAttempts - before.SQLite.WriteAttempts,
			LockRetries:         after.SQLite.LockRetries - before.SQLite.LockRetries,
			LockRetryWaitMillis: after.SQLite.LockRetryWaitMillis - before.SQLite.LockRetryWaitMillis,
			WALBytes:            after.SQLite.WALBytes - before.SQLite.WALBytes,
		},
		ResourceStatusBefore:         firstNonEmpty(before.Resources.Status, "unknown"),
		ResourceStatusAfter:          firstNonEmpty(after.Resources.Status, "unknown"),
		BackgroundDeferredBefore:     before.Resources.BackgroundJobsDeferred,
		BackgroundDeferredAfter:      after.Resources.BackgroundJobsDeferred,
		ActivePlaybackDelta:          after.Resources.ActivePlaybackSessions - before.Resources.ActivePlaybackSessions,
		ActiveTranscodeDelta:         after.Resources.ActiveTranscodeSessions - before.Resources.ActiveTranscodeSessions,
		AvailableTranscodeSlotsAfter: after.Resources.AvailableTranscodeSlots,
		QueuedBackgroundJobsDelta:    after.Resources.QueuedBackgroundJobs - before.Resources.QueuedBackgroundJobs,
		RunningBackgroundJobsDelta:   after.Resources.RunningBackgroundJobs - before.Resources.RunningBackgroundJobs,
		SaturatedWorkloadLanesAfter:  append([]string(nil), after.Resources.SaturatedWorkloadLanes...),
		SaturatedJobLanesAfter:       append([]string(nil), after.Resources.SaturatedJobLanes...),
		ResourceSignalsAfter:         append([]string(nil), after.Resources.Signals...),
		DegradationActionsAfter:      append([]string(nil), after.Resources.DegradationActions...),
		AdmissionRejectedDelta: admissionRejectedDelta{
			Search:        uintDelta(before.Admission.SearchRejected, after.Admission.SearchRejected),
			Download:      uintDelta(before.Admission.DownloadRejected, after.Admission.DownloadRejected),
			Stream:        uintDelta(before.Admission.StreamRejected, after.Admission.StreamRejected),
			Transcode:     uintDelta(before.Admission.TranscodeRejected, after.Admission.TranscodeRejected),
			TranscodeUser: uintDelta(before.Admission.TranscodeUserRejected, after.Admission.TranscodeUserRejected),
		},
		WorkloadRejectedDelta: workloadRejectedDelta(before.WorkloadLanes, after.WorkloadLanes),
		JobQueuedDelta:        jobBacklogDelta(before.JobLanes, after.JobLanes, "queued"),
		JobRunningDelta:       jobBacklogDelta(before.JobLanes, after.JobLanes, "running"),
	}
}

func captureDiagnosticsTimeline(vu *virtualUser, interval time.Duration, done <-chan struct{}, mu *sync.Mutex, points *[]diagnosticsTimelinePoint) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case capturedAt := <-ticker.C:
			if point, ok := diagnosticsTimelinePointFor(vu.rawJSON("/api/system/diagnostics"), capturedAt.UTC()); ok {
				mu.Lock()
				*points = append(*points, point)
				mu.Unlock()
			}
		}
	}
}

func diagnosticsTimelinePointFor(raw json.RawMessage, capturedAt time.Time) (diagnosticsTimelinePoint, bool) {
	if len(raw) == 0 {
		return diagnosticsTimelinePoint{}, false
	}
	var snapshot diagnosticsSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return diagnosticsTimelinePoint{}, false
	}
	return diagnosticsTimelinePoint{
		CapturedAt:                capturedAt.Format(time.RFC3339),
		Goroutines:                snapshot.Runtime.Goroutines,
		HeapAllocBytes:            snapshot.Runtime.HeapAllocBytes,
		NumGC:                     snapshot.Runtime.NumGC,
		SQLiteWaitCount:           snapshot.SQLite.WaitCount,
		SQLiteWaitDurationMillis:  snapshot.SQLite.WaitDurationMillis,
		SQLiteLockRetries:         snapshot.SQLite.LockRetries,
		SQLiteLockRetryWaitMillis: snapshot.SQLite.LockRetryWaitMillis,
		SQLiteWALBytes:            snapshot.SQLite.WALBytes,
		ResourceStatus:            firstNonEmpty(snapshot.Resources.Status, "unknown"),
		BackgroundJobsDeferred:    snapshot.Resources.BackgroundJobsDeferred,
		ActivePlaybackSessions:    snapshot.Resources.ActivePlaybackSessions,
		ActiveTranscodeSessions:   snapshot.Resources.ActiveTranscodeSessions,
		QueuedBackgroundJobs:      snapshot.Resources.QueuedBackgroundJobs,
		RunningBackgroundJobs:     snapshot.Resources.RunningBackgroundJobs,
		WorkloadRejectionsTotal:   totalWorkloadRejected(snapshot.WorkloadLanes),
		AdmissionRejectionsTotal:  totalAdmissionRejected(snapshot),
	}, true
}

func totalAdmissionRejected(snapshot diagnosticsSnapshot) uint64 {
	return snapshot.Admission.SearchRejected + snapshot.Admission.DownloadRejected + snapshot.Admission.StreamRejected + snapshot.Admission.TranscodeRejected + snapshot.Admission.TranscodeUserRejected
}

func uintDelta(before, after uint64) uint64 {
	if after <= before {
		return 0
	}
	return after - before
}

func totalWorkloadRejected(lanes []struct {
	ID       string `json:"id"`
	Active   int    `json:"active"`
	Capacity int    `json:"capacity"`
	Rejected uint64 `json:"rejected"`
}) uint64 {
	var total uint64
	for _, lane := range lanes {
		total += lane.Rejected
	}
	return total
}

func workloadRejectedDelta(before, after []struct {
	ID       string `json:"id"`
	Active   int    `json:"active"`
	Capacity int    `json:"capacity"`
	Rejected uint64 `json:"rejected"`
}) map[string]uint64 {
	beforeByID := map[string]uint64{}
	for _, lane := range before {
		beforeByID[lane.ID] = lane.Rejected
	}
	deltas := map[string]uint64{}
	for _, lane := range after {
		previous := beforeByID[lane.ID]
		if lane.Rejected > previous {
			deltas[lane.ID] = lane.Rejected - previous
		}
	}
	return deltas
}

func jobBacklogDelta(before, after []struct {
	ID      string `json:"id"`
	Queued  int    `json:"queued"`
	Running int    `json:"running"`
}, field string) map[string]int {
	beforeByID := map[string]int{}
	for _, lane := range before {
		beforeByID[lane.ID] = jobBacklogValue(lane.Queued, lane.Running, field)
	}
	deltas := map[string]int{}
	for _, lane := range after {
		delta := jobBacklogValue(lane.Queued, lane.Running, field) - beforeByID[lane.ID]
		if delta != 0 {
			deltas[lane.ID] = delta
		}
	}
	return deltas
}

func jobBacklogValue(queued, running int, field string) int {
	if field == "running" {
		return running
	}
	return queued
}

func summarize(baseURL string, users int, duration time.Duration, started, finished time.Time, samples <-chan sample) report {
	var durations []time.Duration
	statusCounts := map[int]int{}
	byNameDurations := map[string][]time.Duration{}
	byNameErrors := map[string]int{}
	var errors int
	var bytes int64
	for item := range samples {
		durations = append(durations, item.Took)
		statusCounts[item.Status]++
		bytes += int64(item.Bytes)
		byNameDurations[item.Name] = append(byNameDurations[item.Name], item.Took)
		if item.Err != "" || item.Status < 200 || item.Status >= 300 {
			errors++
			byNameErrors[item.Name]++
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	byName := map[string]metric{}
	for name, values := range byNameDurations {
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		byName[name] = metric{
			Requests:  len(values),
			Errors:    byNameErrors[name],
			P95Millis: percentile(values, 95).Milliseconds(),
		}
	}
	elapsed := finished.Sub(started).Seconds()
	if elapsed <= 0 {
		elapsed = duration.Seconds()
	}
	requestsPerSecond := 0.0
	bytesPerSecond := 0.0
	if elapsed > 0 {
		requestsPerSecond = float64(len(durations)) / elapsed
		bytesPerSecond = float64(bytes) / elapsed
	}
	return report{
		BaseURL:           baseURL,
		Users:             users,
		Duration:          duration.String(),
		Requests:          len(durations),
		Errors:            errors,
		Bytes:             bytes,
		RequestsPerSecond: requestsPerSecond,
		BytesPerSecond:    bytesPerSecond,
		P50Millis:         percentile(durations, 50).Milliseconds(),
		P95Millis:         percentile(durations, 95).Milliseconds(),
		P99Millis:         percentile(durations, 99).Milliseconds(),
		StatusCounts:      statusCounts,
		ByName:            byName,
		StartedAt:         started.Format(time.RFC3339),
		FinishedAt:        finished.Format(time.RFC3339),
	}
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := (len(sorted)*p + 99) / 100
	if index < 1 {
		index = 1
	}
	if index > len(sorted) {
		index = len(sorted)
	}
	return sorted[index-1]
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func normalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("expected absolute http(s) URL")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func searchTerm(title string) string {
	parts := strings.Fields(title)
	if len(parts) == 0 {
		return title
	}
	return parts[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func fail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
