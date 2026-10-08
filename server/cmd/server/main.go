package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	adminops "portico.local/server/internal/administration"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/audiofacts"
	"portico.local/server/internal/backup"
	"portico.local/server/internal/buildinfo"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/childprocess"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/enrichment"
	"slices"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/downloads"
	"portico.local/server/internal/eventfeed"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/hostlimits"
	"portico.local/server/internal/httpapi"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/ingestion"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/livechannels/dvr"
	librarychannels "portico.local/server/internal/livechannels/library"
	"portico.local/server/internal/localmetadata"
	"portico.local/server/internal/lyrics"
	"portico.local/server/internal/mediaexec"
	"portico.local/server/internal/metadata"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/playbackruntime"
	"portico.local/server/internal/preparedmedia"
	"portico.local/server/internal/recordingaccess"
	"portico.local/server/internal/recordingmedia"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/remotesources"
	"portico.local/server/internal/scanevents"
	"portico.local/server/internal/sourceaccess"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
	"portico.local/server/internal/subtitlevideo"
	"portico.local/server/internal/supervise"
	"portico.local/server/internal/worker"
	"portico.local/server/internal/workpolicy"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	// The media process shim (mediaexec) replaces itself with the sandboxed
	// tool; it must not start anything else first.
	if handled, e := mediaexec.RunHelper(os.Args[1:]); handled {
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "recover-owner" {
		if err := recoverOwner(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if handled, e := decoder.RunHelper(os.Args[1:]); handled {
		if e != nil {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--health-check" {
		os.Exit(healthCheck())
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Info())
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--portico-rclone-guardian" {
		if mounts.Guardian(os.Stdin) != nil {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--portico-rclone-native" {
		if mounts.NativeHelper(os.Stdin, os.Stdout) != nil {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--portico-storage-helper" {
		if storage.Helper(os.Stdin, os.Stdout) != nil {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--portico-subtitle-helper" {
		if subtitles.Helper(os.Stdin, os.Stdout) != nil {
			os.Exit(1)
		}
		return
	}
	if e := run(); e != nil {
		if errors.Is(e, errRestartRequested) {
			log.Print("Restarting to apply the staged restore")
			os.Exit(75)
		}
		log.Fatal(e)
	}
}
func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

// defaultBind is the first-run LAN-reachable default (ONB-02 item 2). Setup
// still accepts only private peers and public traffic still requires TLS, so
// reachability does not widen trust.
const defaultBind = "0.0.0.0:32500"

// stateDirFreshThisProcess reports whether this process created the state
// directory's database file. ONB-02 item 4: a foreign-schema warning about a
// directory this process just created can only describe objects this same
// build installed, so housekeeping downgrades it.
var stateDirFreshThisProcess bool

// errRestartRequested ends the process with status 75 so the service manager
// restarts it: a staged restore applies on the next start.
var errRestartRequested = errors.New("restart requested: staged restore applies on next start")

// restartRequested is set when a restore is staged and answered, and read when
// the HTTP server has drained.
var restartRequested atomic.Bool

// firstPrivateLANAddress returns the machine's first private LAN IPv4 address
// (10/8, 172.16/12, 192.168/16), or "" when none is configured. ONB-02 item 1
// names it in the first-run setup block so a phone or TV on the same network
// can reach the server.
func firstPrivateLANAddress() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			continue
		}
		ip = ip.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			continue
		}
		if ip.IsPrivate() {
			return ip.String()
		}
	}
	return ""
}

// logSetupBlock prints the friendly first-run block while setupRequired is
// true: the LAN URL every start, plus localhost when bound to loopback.
func logSetupBlock(port int, loopback bool) {
	if lan := firstPrivateLANAddress(); lan != "" {
		log.Printf("Portico is ready to set up.\nOpen http://%s:%d in a browser on this network.", lan, port)
	} else {
		log.Printf("Portico is ready to set up.\nOpen http://localhost:%d in a browser on this computer.", port)
	}
	if loopback {
		log.Printf("Open http://localhost:%d to set up Portico.", port)
	}
}
func run() error {
	// Before anything opens a socket or a file: lift the descriptor limit to what
	// the kernel will allow, and give the collector a ceiling to work to where the
	// cgroup provides one. Both are invisible when healthy and both are the
	// difference between an honest slow server and one the OOM killer removes
	// without a log line.
	hostlimits.Apply()
	// On Windows this puts the server in a job object that kills every child when
	// the last handle closes — which is when this process ends, however it ends.
	// Children join the job automatically, so ffmpeg, the helpers and the
	// helpers' own rclone are all covered by this one call. Unix already lets a
	// parent reach its descendants and uses process groups per child instead.
	if err := childprocess.AdoptTree(); err != nil {
		log.Printf("Child processes are not bound to this server's lifetime (%v); a crash may leave media processes running", err)
	}
	state := env("PORTICO_STATE_DIR", "../../runtime/server")
	state, e := filepath.Abs(state)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(state, 0700); e != nil {
		return e
	}
	// New state folders start private. An existing folder is never
	// re-tightened here: the console warns about loose permissions and the
	// owner fixes them with an explicit action.
	bind := env("PORTICO_BIND", defaultBind)
	host, _, e := net.SplitHostPort(bind)
	if e != nil {
		return e
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("server requires an explicit IP bind")
	}
	// The socket is bound before anything expensive happens, so a client that
	// arrives during startup gets an honest "starting" answer with phase progress
	// instead of a connection refusal, and a supervisor's health check cannot
	// race the database open.
	reporter := newStartupReporter()
	handlerSwitch := newSwitchableHandler(reporter)
	reporter.start("listener", "Bind the network listener")
	listener, e := net.Listen("tcp", bind)
	if e != nil {
		reporter.finish("listener", e)
		return e
	}
	reporter.finish("listener", nil)
	defer listener.Close()
	// One process owns one state directory. The lock is taken now — after the
	// listener is bound, so a second instance's refusal is visible, and before
	// anything opens the database or the journal, so a second instance never
	// writes a byte.
	doneInstance := reporter.step("instance", "Take the single-instance lock")
	instance, lockErr := acquireInstanceLock(filepath.Join(state, "server.lock"))
	if errors.Is(lockErr, errInstanceLockHeld) {
		doneInstance(lockErr)
		return fmt.Errorf("another Portico server is already using %s", state)
	}
	if lockErr != nil {
		// Unsupported locking, or a filesystem that answered in a way we cannot
		// interpret: say so once and carry on unguarded rather than refuse to run.
		log.Printf("Single-instance lock unavailable on %s (%v); run only one server against this state directory", state, lockErr)
	} else {
		defer instance.Close()
	}
	doneInstance(nil)
	// A staged restore applies now, before the authority or the database is
	// opened: the current database moves into a pre-restore backup, the staged
	// copy moves into place, and verification runs the ordinary migrations. A
	// failure moves the pre-restore copy back and the server still starts.
	doneRestore := reporter.step("restore", "Apply a staged restore")
	if lockErr != nil {
		// Without the instance lock another server may be running on this state
		// folder: moving its database would pull it out from under that server.
		// A staged restore waits for a start that holds the lock.
		if _, err := os.Stat(filepath.Join(state, backup.RestoreStagedDir)); err == nil {
			log.Printf("A staged restore is waiting: it applies on a start that holds the single-instance lock")
		}
	} else {
		if err := backup.CleanupPartials(state); err != nil {
			log.Printf("Backup staging cleanup: %v", err)
		}
		if err := backup.ApplyStaged(context.Background(), state, nil); err != nil {
			if backup.RestoreUnresolved(state) {
				// Files may already have moved: starting now could serve a
				// half-restored folder or a new empty database. Nothing has been
				// deleted, and the next start resumes the restore.
				log.Fatalf("A restore didn't finish (%v). Portico won't start on a half-restored state folder; nothing has been deleted, and the previous data is kept under %s. Fix the cause (for example free disk space or folder permissions), then start Portico again: it resumes the restore.", err, filepath.Join(state, backup.BackupsDir))
			}
			log.Printf("Staged restore did not apply (%v); continuing on the current state", err)
		}
	}
	doneRestore(nil)
	// Every FFmpeg and ffprobe this server starts goes through mediaexec. Now
	// that this process owns the state directory, stop what a crashed previous
	// run left encoding (BE-MEDIA-06) and turn on the limits and the ledger.
	// Without the lock another server may still be running on this state
	// directory, so its live jobs are not orphans: configure without the kill.
	if reaped, mediaErr := mediaexec.Configure(mediaexec.Options{StateDir: state, SkipOrphanKill: lockErr != nil, Libraries: append(filepath.SplitList(os.Getenv("PORTICO_DECODER_LIBRARIES")), filepath.SplitList(os.Getenv("PORTICO_LINEAR_DECODER_LIBRARIES"))...)}); mediaErr != nil {
		log.Printf("Media process limits and the orphan ledger are unavailable (%v); media still runs sandboxed where the platform allows", mediaErr)
	} else if reaped > 0 {
		log.Printf("Stopped %d media process group(s) a previous run of this server left running", reaped)
	}
	doneAuthority := reporter.step("authority", "Open the current server authority")
	claimRunner, e := openCurrentAuthority(state)
	doneAuthority(e)
	if e != nil {
		return e
	}
	// Resolve one verified pair. Qualification overlaps independent schema work,
	// but is joined below before constructing the first playback service.
	doneQualification := reporter.step("toolchain", "Verify and qualify the media tools")
	ffmpegTool, ffprobeTool := decoder.ResolveTools()
	qualificationCtx, qualificationCancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer qualificationCancel()
	type toolchainResult struct {
		facts decoder.ToolchainFacts
		err   error
	}
	toolchainReady := make(chan toolchainResult, 1)
	supervise.Go("cmd.toolchain-qualification", func() {
		facts, err := decoder.ProbeToolchain(qualificationCtx, ffmpegTool)
		toolchainReady <- toolchainResult{facts, err}
	})
	doneDatabase := reporter.step("database", "Open and install the database")
	if _, err := os.Stat(filepath.Join(state, "server.sqlite")); os.IsNotExist(err) {
		stateDirFreshThisProcess = true
	}
	db, e := persistence.Open(filepath.Join(state, "server.sqlite"))
	doneDatabase(e)
	if e != nil {
		return e
	}
	defer db.Close()
	// A copied state folder rebases Portico's own absolute paths on first
	// start (spec §3): storage stays absolute and the move repairs it.
	if e = persistence.RebaseStatePaths(context.Background(), db, state); e != nil {
		return e
	}
	workAdmission := workpolicy.Service{DB: db}
	adminOperations := adminops.NewAt(db, state)
	adminOperations.LogoFetch = func(ctx context.Context, locator string) ([]byte, error) {
		return livechannels.ReadSource(ctx, locator, remotemedia.Policy{ReadTimeout: 10 * time.Second}, adminops.LogoUploadBytes)
	}
	adminOperations.Fetch = adminops.FetchReleaseFeed
	if e = adminOperations.RecoverFileOperations(context.Background()); e != nil {
		return e
	}
	// One bounded barrier for every long-lived loop, registered here so it runs
	// before the database closes and after the HTTP server has stopped accepting.
	drain := &drainBarrier{}
	defer drain.wait()
	databasePath := filepath.Join(state, "server.sqlite")
	watchdog := dbwork.NewWatchdog(db)
	watchdog.Reopen = func() (*sql.DB, error) { return dbwork.OpenHandle(databasePath, dbwork.DefaultPolicy()) }
	doneClaims := reporter.step("networking-claims", "Verify the networking claim schema")
	e = persistence.VerifyNetworkingClaims(context.Background(), db)
	doneClaims(e)
	if e != nil {
		return e
	}
	doneKeys := reporter.step("claim-keys", "Open the networking claim keys")
	claimKeys, e := networking.OpenCurrentKeys(context.Background(), db, state, claimRunner)
	doneKeys(e)
	if e != nil {
		return e
	}
	defer claimKeys.Close()
	doneLive := reporter.step("live-channels", "Open live channel storage")
	liveSources, e := initializeLiveChannels(db, state)
	doneLive(e)
	if e != nil {
		return e
	}
	doneIdentity := reporter.step("identity", "Load the server identity")
	ident, e := identity.New(db, state)
	doneIdentity(e)
	if e != nil {
		return e
	}
	wireNativeServerIdentity(ident)
	// A devtrust build on a throwaway e2e state directory seeds its test owner
	// (identity.SeedDevE2EOwner); every other build and state does nothing.
	if e = ident.SeedDevE2EOwner(context.Background()); e != nil {
		return e
	}
	helperBinary, e := os.Executable()
	if e != nil {
		return e
	}
	// Integration merge: lane C's configureLocalMedia (15040d6: source guard only
	// on add/change, managed mount root exempt) plus lane B's owner background
	// priority (b54b6c1) applied to the same storage supervisor.
	storeIO, managed, e := configureLocalMedia(db, state, helperBinary, adminOperations)
	if e != nil {
		return e
	}
	var backgroundPriority string
	if e = db.QueryRow(`SELECT background_priority FROM maintenance_policy WHERE singleton=1`).Scan(&backgroundPriority); e != nil {
		return e
	}
	e = storeIO.Supervisor.SetBackgroundTaskPriority(backgroundPriority)
	if e != nil {
		return e
	}
	sources, e := remotesources.New(db, state, managed)
	if e != nil {
		return e
	}
	storeIO.Remote = sources
	storeIO.Guard = managed.Guard
	storeIO.MountedRoot = managed.RootFor
	cat := catalog.New(db)
	cat.SetStorage(storeIO)
	// This is a barrier, not a background best-effort capability update. No
	// playback service is constructed until qualification completes or times out.
	select {
	case result := <-toolchainReady:
		if result.err != nil {
			log.Printf("Media toolchain qualification unavailable: %v", result.err)
		} else {
			decoder.ConfigureToolchain(result.facts)
			log.Printf("Media toolchain: %s ass=%t zscale=%t", ffmpegTool, result.facts.Filters["ass"], result.facts.Filters["zscale"])
			if missing := result.facts.MissingRequired(); len(missing) > 0 {
				log.Printf("Media toolchain attention: missing %v", missing)
			}
		}
	case <-qualificationCtx.Done():
		log.Printf("Media toolchain qualification unavailable: %v", qualificationCtx.Err())
	}
	qualificationCancel()
	doneQualification(nil)
	scanner := ingestion.New(db, cat, assets.Probe{Binary: ffprobeTool, Supervisor: storeIO.Supervisor})
	scanner.Admission = workAdmission.Admit
	scanner.SetStorage(storeIO)
	localMeta, e := localmetadata.New(filepath.Join(state, "local-artwork"), ffmpegTool, storeIO)
	if e != nil {
		return e
	}
	scanner.LocalMetadata = localMeta
	lyricProvider, e := lyrics.NewLRCLIB(env("PORTICO_LYRICS_PROVIDER_ORIGIN", "https://lrclib.net"))
	if e != nil {
		return e
	}
	lyricService := &lyrics.Service{DB: db, Local: storeIO, Provider: lyricProvider}
	lyricFetch := &lyrics.Bulk{Service: lyricService}
	player := playback.New(db)
	player.PersonalProgress = cat.AcceptPlaybackProgress
	player.StorageGuard = managed.Guard
	remote := playback.NewRemote(db, storeIO, assets.Probe{Binary: ffprobeTool, Supervisor: storeIO.Supervisor})
	player.ConfigureRemote(remote)
	analysisRoots := sourceaccess.New(storeIO, managed)
	defer analysisRoots.Close()
	remote.AnalysisRoot = func(ctx context.Context, library, path string) (*storage.RootLease, error) {
		sources, err := cat.LibrarySources(ctx, library)
		if err != nil {
			return nil, err
		}
		var selected catalog.LibrarySource
		relative := ""
		for _, source := range sources {
			if !source.Enabled || source.Kind != "local" {
				continue
			}
			rel, err := filepath.Rel(source.ResolvedPath, path)
			if err == nil && filepath.IsLocal(rel) && rel != "." {
				if selected.ID != "" {
					return nil, sourceaccess.ErrAuthority
				}
				selected, relative = source, rel
			}
		}
		if selected.ID == "" || !selected.IdentityConfirmed {
			return nil, sourceaccess.ErrAuthority
		}
		validate := func(check context.Context) error {
			current, err := cat.LibrarySource(check, selected.ID)
			if err != nil {
				return err
			}
			if !current.Enabled || current.LibraryID != library || current.Kind != "local" || current.ResolvedPath != selected.ResolvedPath || current.RootIdentity != selected.RootIdentity || current.Incarnation != selected.Incarnation || current.Generation != selected.Generation {
				return sourceaccess.ErrAuthority
			}
			return nil
		}
		return analysisRoots.Borrow(ctx, sourceaccess.Authority{OwnerID: "strm-analysis:" + identity.Token(), RootID: selected.ID, RootPath: selected.ResolvedPath, RelativePath: relative, Revision: selected.Incarnation + ":" + strconv.FormatInt(selected.Generation, 10), Lifetime: ctx, Validate: validate})
	}
	scanner.AnalyzeSTRM = remote.AnalyzeDescriptor
	hostedOrigin, hostedRoot, hostedRootID, e := hosted.DefaultConfig(os.Getenv("PORTICO_HOSTED_ORIGIN"), os.Getenv("PORTICO_HOSTED_PUBLIC_KEY"), os.Getenv("PORTICO_HOSTED_KEY_ID"))
	if e != nil {
		return e
	}
	control, e := hosted.New(db, ident, hostedOrigin, hostedRoot, hostedRootID)
	if e != nil {
		return e
	}
	subtitleAuthority := httpapi.Dependencies{DB: db, Identity: ident, Hosted: control, Catalog: cat}
	doneSubtitles := reporter.step("subtitles", "Open subtitle storage")
	subtitleService, e := subtitles.New(subtitles.Options{DB: db, Directory: filepath.Join(state, "subtitles"), Storage: storeIO, HelperBinary: helperBinary, FFmpeg: ffmpegTool, Authorize: subtitleAuthority.AuthorizeSubtitles, Provider: subtitles.ProviderFromEnvironment()})
	if e != nil {
		return e
	}
	defer subtitleService.Close()
	doneSubtitles(nil)
	player.Subtitles = subtitleService
	subtitleRenderer, e := subtitlevideo.New(subtitlevideo.Options{DB: db, Storage: storeIO, Mounts: managed, Directory: filepath.Join(state, "subtitle-video"), FFmpeg: ffmpegTool, FFprobe: ffprobeTool, Libraries: filepath.SplitList(os.Getenv("PORTICO_DECODER_LIBRARIES"))})
	if e != nil {
		return e
	}
	defer subtitleRenderer.Close()
	subtitleRenderer.RemoteStorage = subtitlevideo.RemoteStorageAdapter{Match: sources.Handles, Acquire: func(ctx context.Context, path, purpose string) (subtitlevideo.RemoteObject, error) {
		return sources.Acquire(ctx, path, purpose)
	}}
	doneAnalysis := reporter.step("analysis", "Open media analysis storage")
	analysisService, e := initializeAnalysis(db, state, subtitleRenderer, ffmpegTool)
	doneAnalysis(e)
	if e != nil {
		return e
	}
	defer analysisService.Close()
	analysisService.Admission = workAdmission.Admit
	scanner.DeepAnalysis = analysisService
	donePrepared := reporter.step("prepared-media", "Open prepared media storage")
	preparedVersions, e := preparedmedia.New(preparedmedia.Options{Sandbox: env("PORTICO_PREPARED_SANDBOX", "bwrap"), DB: db, Directory: filepath.Join(state, "prepared-media"), FFmpeg: ffmpegTool, FFprobe: ffprobeTool, Libraries: filepath.SplitList(os.Getenv("PORTICO_DECODER_LIBRARIES")), Supervisor: storeIO.Supervisor, OpenInput: subtitleRenderer.OpenMediaInput, Authorize: subtitleAuthority.AuthorizePreparedMedia, Continue: subtitleAuthority.ContinuePreparedMedia})
	if e != nil {
		return e
	}
	defer preparedVersions.Close()
	donePrepared(nil)
	player.Prepared = preparedVersions
	// Offline downloads read original bytes through the same isolated media
	// process playback uses, and prepared bytes through the producer that owns
	// them. It produces nothing itself.
	offlineDownloads, e := downloads.New(downloads.Options{DB: db, SourceSnapshotRoot: filepath.Join(state, "downloads", "source-snapshots"), OpenSource: func(ctx context.Context, path string, size, modified int64) (io.ReadSeekCloser, error) {
		return storeIO.OpenPlayback(ctx, path, size, modified)
	}, Artifacts: preparedVersions, Optimizer: preparedVersions})
	if e != nil {
		return e
	}
	offlineDownloads.Start()
	defer offlineDownloads.Close()
	subtitleService.Extractor = subtitleRenderer
	subtitleService.SourceInputs = subtitleRenderer
	scanner.Subtitles = subtitleService
	player.ConfigureSubtitleDelivery(subtitleService)
	remote.SubtitleInputs = subtitleRenderer
	remote.SubtitleInspector = subtitleRenderer
	// The browser origins that may call this server: the Portico web app and the shared Cast
	// receiver page. They are built in because a server installed from a package has no
	// environment to set; PORTICO_ALLOWED_ORIGINS adds to them (development hosts, a self-hosted
	// web app) and never has to repeat them.
	origins := []string{"https://web.getportico.tv", "https://cast.getportico.tv"}
	for _, extra := range strings.Split(env("PORTICO_ALLOWED_ORIGINS", ""), ",") {
		if extra = strings.TrimSpace(extra); extra != "" && !slices.Contains(origins, extra) {
			origins = append(origins, extra)
		}
	}
	claimHandler, e := initializeCurrentClaimControl(db, state, ident, claimRunner, claimKeys, control, origins)
	if e != nil {
		return e
	}
	if claimHandler != nil {
		if e = claimHandler.ConfigureRemote(context.Background(), bind, control.NetworkChanged); e != nil {
			return e
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	preparedVersions.Start(ctx)
	defer cancel()
	generatedChannels, e := librarychannels.New(db)
	if e != nil {
		return errors.New("Library Channel storage could not be initialized")
	}
	recordingsRoot, e := filepath.Abs(env("PORTICO_RECORDINGS_DIR", filepath.Join(state, "recordings")))
	if e != nil {
		return errors.New("recording root must be an absolute local path")
	}
	doneRecording := reporter.step("recording", "Open recording storage")
	recorder, e := recordingmedia.New(recordingsRoot, ffmpegTool, ffprobeTool, filepath.SplitList(os.Getenv("PORTICO_LINEAR_DECODER_LIBRARIES")))
	if e != nil {
		return errors.New("private recording storage could not be initialized")
	}
	recordingStore, e := dvr.New(db, liveSources, recordingaccess.Policy{Cached: control}.Durable)
	if e != nil {
		_ = recorder.Close()
		return e
	}
	recordingLocks, e := livechannels.NewPhysicalLocks(filepath.Join(state, "live-source-locks"))
	if e != nil {
		_ = recorder.Close()
		return e
	}
	if e = recordingStore.ConfigureCapture(recorder, recordingLocks); e != nil {
		_ = recorder.Close()
		return e
	}
	doneRecording(nil)
	storeIO.RecordingRoot = recordingsRoot
	recordingCtx, stopRecordings := context.WithCancel(ctx)
	recordingsDone := make(chan struct{})
	supervise.Go("cmd.recordings", func() {
		defer close(recordingsDone)
		supervise.Loop(recordingCtx, "cmd.recordings", recordingStore.Run)
	})
	defer func() {
		stopRecordings()
		select {
		case <-recordingsDone:
			_ = recorder.Close()
		case <-time.After(15 * time.Second):
			log.Print("Recording shutdown awaits physical reader retirement; durable recovery will reconcile remaining work")
		}
	}()
	donePlaybackRuntime := reporter.step("playback-runtime", "Start the playback runtime")
	currentPlayback, e := playbackruntime.New(db, ident, storeIO)
	donePlaybackRuntime(e)
	supervise.Supervise(ctx, "cmd.subtitles", subtitleService.Run)
	if e != nil {
		return e
	}
	if e = currentPlayback.ConfigureChannels(playbackruntime.LinearConfig{DB: db, Live: liveSources, Library: generatedChannels, Policy: recordingaccess.Policy{Cached: control}, CacheDirectory: filepath.Join(state, "linear-media"), LockDirectory: filepath.Join(state, "live-source-locks"), FFmpeg: ffmpegTool, FFprobe: ffprobeTool, DecoderLibraries: filepath.SplitList(os.Getenv("PORTICO_LINEAR_DECODER_LIBRARIES"))}); e != nil {
		return e
	}
	// Channels on v1 sessions open a Library Channels title the way every v1
	// media read does (spec §18.6, B5).
	if currentPlayback.Linear != nil {
		currentPlayback.Linear.UseMediaInputs(func(ctx context.Context, item, asset string) (subtitles.RenderInput, error) {
			return subtitleRenderer.OpenSubtitleInput(ctx, item, asset, "")
		})
	}
	if e = currentPlayback.Start(ctx); e != nil {
		return e
	}
	defer func() {
		stop, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if err := currentPlayback.Shutdown(stop); err != nil {
			log.Printf("Playback shutdown incomplete: %v", err)
		}
	}()
	// Delivery seams: owner settings default until the administration registry
	// is wired to them, viewer preferences come from the preference registry, and
	// the hardware detector probes this host's ffmpeg once per identity.
	// The registry is the authority for transcoding policy; the console store
	// that publishes it is created below, so the adapter binds to it late.
	deliverySettings := &httpapi.RegistryDeliverySettings{}
	doneDecoder := reporter.step("decoder", "Resolve the media decoder")
	ffmpegPath, ffmpegErr := exec.LookPath(ffmpegTool)
	detector := &decoder.HardwareDetector{}
	if libraries, libErr := decoder.ResolveLibraries(ffmpegPath, ffprobeTool, filepath.SplitList(os.Getenv("PORTICO_LINEAR_DECODER_LIBRARIES"))); libErr == nil {
		detector.Libraries = libraries
	}
	if ffmpegErr != nil {
		ffmpegPath = ""
	}
	doneDecoder(nil)
	player.ConfigureDelivery(deliverySettings, func(tx *sql.Tx, v identity.Viewer, deviceClass string) (playback.DeliveryPreferenceReader, error) {
		values, _, _, err := operations.EffectivePreferences(tx, v, deviceClass)
		return values, err
	}, detector, ffmpegPath)
	// Probe encoders once at startup, off the request path, so the first session
	// never waits on ffmpeg. Sessions that arrive earlier take software until then.
	player.ConfigureProbe(assets.Probe{Binary: ffprobeTool, Supervisor: storeIO.Supervisor})
	supervise.Go("cmd.hardware-report", func() { player.HardwareReport(context.Background()) })
	hls, e := playback.NewHLS(ctx, db, filepath.Join(state, "hls"), ffmpegTool)
	if e == nil {
		hls.SourceStorage = storeIO
		hls.WindowInputs = subtitleRenderer
		hls.ConfigureSettings(deliverySettings)
		hls.ConfigureProbe(ffprobeTool)
		if env("PORTICO_FINITE_VOD", "0") == "1" {
			if e = hls.EnableFinite(ffprobeTool); e != nil {
				return e
			}
		}
		player.ConfigureHLS(hls)
		supervise.Supervise(ctx, "cmd.hls", hls.Run)
		// Converters are cancelled and then waited for: an ffmpeg child that
		// outlives the process holds its source file open across a restart.
		hlsDrained := make(chan struct{})
		converter := hls
		drain.add("hls-producers", func() {
			supervise.Go("cmd.hls-drain", func() {
				defer close(hlsDrained)
				stop, done := context.WithTimeout(context.Background(), drainBudget)
				defer done()
				if err := converter.Shutdown(stop); err != nil {
					log.Printf("Media conversion shutdown incomplete: %v", err)
				}
			})
		}, hlsDrained)
	} else {
		log.Print("Media conversion unavailable; compatible direct playback remains available")
	}
	supervise.Supervise(ctx, "cmd.remote-sources", sources.Run)
	supervise.Supervise(ctx, "cmd.library-channels", generatedChannels.Run)
	supervise.Supervise(ctx, "cmd.live-refresh", func(life context.Context) { liveSources.RunRefresh(life, livechannels.SourceFetcher{}) })
	supervise.Supervise(ctx, "cmd.live-logo-import", adminOperations.RunLogoImports)
	supervise.Supervise(ctx, "cmd.guide-image-import", adminOperations.RunGuideImageImports)
	supervise.Supervise(ctx, "cmd.managed-mounts", managed.Run)
	scanDone := make(chan struct{})
	retentionDone := make(chan struct{})
	supervise.Go("cmd.scanner", func() {
		defer close(scanDone)
		supervise.Loop(ctx, "cmd.scanner", scanner.Run)
	})
	supervise.Go("cmd.analysis-retention", func() {
		defer close(retentionDone)
		supervise.Loop(ctx, "cmd.analysis-retention", analysisService.RunRetention)
	})
	drain.add("scanner", cancel, scanDone)
	drain.add("analysis-retention", nil, retentionDone)
	meta := metadata.New(db, os.Getenv("PORTICO_TMDB_READ_ACCESS_TOKEN"))
	meta.Admission = workAdmission.Admit
	if e = meta.ConfigureAcoustID(os.Getenv("PORTICO_ACOUSTID_CLIENT_KEY")); e != nil {
		return e
	}
	cat.OwnerMetadataRevision = meta.OwnerTextRevision
	// The Portico title dataset: a daily manifest check and download for the
	// domains this server has libraries for, and works matched as they gain
	// provider ids. Off until a release is published (enrichment.Published*).
	titles := enrichment.New(db, enrichment.ConfigFromEnvironment(filepath.Join(state, "title-dataset")))
	if titles.Enabled() {
		supervise.Supervise(ctx, "cmd.title-dataset", func(life context.Context) {
			wake := worker.NewSignal()
			unregister := dbwork.WakeOnTables(wake, "catalog_external_ids", "catalog_libraries")
			defer unregister()
			worker.Run(life, "cmd.title-dataset", wake, func(ctx context.Context) time.Duration {
				more, err := titles.Step(dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia), 500)
				if err != nil {
					log.Printf("Title dataset: %v", err)
					return time.Hour
				}
				if more {
					return time.Second
				}
				return time.Hour
			})
		})
	}
	// Ratings are classified after publication by this bounded background
	// worker. Restricted reads treat a pending spelling as unrated immediately.
	supervise.Supervise(ctx, "cmd.rating-classification", func(life context.Context) {
		wake := worker.NewSignal()
		unregister := dbwork.WakeOnTables(wake, "catalog_item_attributes")
		defer unregister()
		worker.Run(life, "cmd.rating-classification", wake, func(ctx context.Context) time.Duration {
			more, err := cat.ClassifyPendingRatings(ctx)
			if err != nil {
				log.Printf("Content rating classification: %v", err)
				return 15 * time.Second
			}
			if more {
				return time.Millisecond
			}
			return 0
		})
	})
	// Audio facts for client-side decoding (spec §18.7): a background pass over
	// the library's audio assets, woken by catalog changes, one measurement at a
	// time; a play measures a missing one itself (bounded).
	supervise.Supervise(ctx, "cmd.audio-facts", func(life context.Context) {
		wake := worker.NewSignal()
		unregister := dbwork.WakeOnTables(wake, "assets", "item_assets")
		defer unregister()
		backfill := &audiofacts.Backfill{DB: db, Measure: subtitleRenderer.MeasureAudio}
		worker.Run(life, "cmd.audio-facts", wake, backfill.Step)
	})
	// Author/series navigation identities are derived from book and tag
	// publications. Keeping the batch here makes audiobook reads read-only.
	// Only books and book_files commits can name a new author or series (every
	// audiobook ingest writes book_files); another library's scan does not wake
	// it (B85). The step runs a pass at startup and after such a change.
	supervise.Supervise(ctx, "cmd.listening-groups", func(life context.Context) {
		wake, changed := worker.NewSignal(), worker.NewSignal()
		unregisterWake := dbwork.WakeOnTables(wake, "books", "book_files")
		defer unregisterWake()
		unregisterChanged := dbwork.WakeOnTables(changed, "books", "book_files")
		defer unregisterChanged()
		worker.Run(life, "cmd.listening-groups", wake, cat.ListeningGroupStep(changed))
	})
	meta.LocalArtwork = localMeta.Open
	if e = meta.SetArtworkDirectory(filepath.Join(state, "artwork")); e != nil {
		return e
	}
	supervise.Supervise(ctx, "cmd.metadata", meta.Run)
	supervise.Supervise(ctx, "cmd.artwork", meta.RunArtwork)
	supervise.Supervise(ctx, "cmd.hosted-control", control.Run)
	setupCtx, stopSetup := context.WithCancel(ctx)
	setupDone := make(chan struct{})
	supervise.Go("cmd.setup-maintenance", func() {
		defer close(setupDone)
		supervise.Loop(setupCtx, "cmd.setup-maintenance", ident.RunSetupMaintenance)
	})
	drain.add("setup-maintenance", stopSetup, setupDone)
	// Four retention sweeps that used to run every minute for the life of the
	// process. Every one of them only has something to remove after a write, so
	// they are woken by commits: a server nobody is using runs none of them and
	// asks the database nothing, and a busy one runs them sooner than a minute
	// rather than later.
	supervise.Supervise(ctx, "cmd.minute-cleanup", func(ctx context.Context) {
		wake := worker.NewSignal()
		unregister := dbwork.WakeOnCommit(wake)
		defer unregister()
		lastArtwork := time.Time{}
		worker.Run(ctx, "cmd.minute-cleanup", wake, func(ctx context.Context) time.Duration {
			_ = player.Cleanup()
			_ = cat.CleanupPersonalReceipts()
			_ = cat.CleanupBulkJobs(dbwork.WithClass(ctx, dbwork.ClassMaintenance), 250*time.Millisecond)
			_ = lyricService.Cleanup(ctx)
			if time.Since(lastArtwork) >= metadata.ArtCleanupInterval {
				lastArtwork = time.Now()
				_ = meta.CleanupArtwork(ctx)
			}
			return 0
		})
	})
	trustedProxies, e := httpapi.ParseTrustedProxyCIDRs(os.Getenv("PORTICO_TRUSTED_PROXY_CIDRS"))
	if e != nil {
		return e
	}
	doneConsole := reporter.step("console", "Open the operations console")
	console, scheduler, e := initializeConsole(db, scanner, player, hls, cat, lyricFetch, state)
	doneConsole(e)
	if e != nil {
		return e
	}
	// Plain state (spec §6): loose permissions warn, never refuse. The check
	// runs beside startup so it never delays listening.
	supervise.Go("cmd.state-permissions", func() {
		exposed, err := backup.CheckPermissions(state)
		if err != nil {
			log.Printf("State permissions check: %v", err)
			return
		}
		if err = console.Alert(context.Background(), "state-permissions", "warning", exposed); err != nil {
			log.Printf("State permissions check: %v", err)
		} else if exposed {
			log.Print("State permissions: other accounts on this computer can read your Portico data, including sign-in secrets; the console offers a fix")
		}
	})
	deliverySettings.Console = console
	deliverySettings.Refresh(ctx)
	if e = scheduler.Register(preparedVersions.Adapter()); e != nil {
		return e
	}
	supervise.Supervise(ctx, "cmd.job-scheduler", scheduler.Run)
	supervise.Supervise(ctx, "cmd.lyric-fetch", lyricFetch.Run)
	measurements := operations.NewMeasurements(db, state)
	measurements.ActiveTranscodes = func(ctx context.Context) (int, error) {
		status, err := player.DeliveryDiagnostics(ctx)
		if err != nil {
			return 0, err
		}
		if status.ActiveConversionSessions == nil {
			return 0, nil
		}
		return *status.ActiveConversionSessions, nil
	}
	_, bindPort, _ := net.SplitHostPort(bind)
	bindPortNumber, _ := strconv.Atoi(bindPort)
	doneAdministration := reporter.step("administration", "Open administration and access")
	administration, closeAdministration := initializeAdministration(ctx, db, console, ident, state, bindPortNumber)
	doneAdministration(nil)
	defer closeAdministration()
	// The owner's preferred LAN interface is read from the console once per
	// route observation; a read error keeps the last value (automatic until read).
	if manager := claimHandler.Remote(); manager != nil && console != nil {
		var advertisedMu sync.Mutex
		var lastAdvertised string
		manager.SetAdvertisedInterface(func(ctx context.Context) string {
			document, err := console.Settings(ctx, operations.AllowServerScope)
			advertisedMu.Lock()
			defer advertisedMu.Unlock()
			if err == nil {
				lastAdvertised = document.Effective.AdvertisedInterface
			}
			return lastAdvertised
		})
	}
	// The owner's certificate for a custom domain. The TLS handshake asks for
	// its source on every connection, so the settings are cached: read at most
	// every 30 seconds, and at once after any settings save (SettingsApplied
	// invalidates). A read error keeps the last values. The same instance serves
	// handshakes and the connectivity report.
	customCertificate := networking.NewCustomCertificate()
	invalidateCustomSource := func() {}
	if console != nil {
		var customMu sync.Mutex
		var customRead time.Time
		var lastPath, lastKey, lastDomain string
		invalidateCustomSource = func() {
			customMu.Lock()
			customRead = time.Time{}
			customMu.Unlock()
		}
		customCertificate.SetSource(func(ctx context.Context) (string, string, string) {
			customMu.Lock()
			defer customMu.Unlock()
			if !customRead.IsZero() && time.Since(customRead) < 30*time.Second {
				return lastPath, lastKey, lastDomain
			}
			if ctx == nil {
				ctx = context.Background()
			}
			document, err := console.Settings(ctx, operations.AllowServerScope)
			customRead = time.Now()
			if err == nil {
				lastPath, lastKey, lastDomain = document.Effective.CustomCertificatePath, document.Effective.CustomCertificateKeyPath, document.Effective.CustomCertificateDomain
			}
			return lastPath, lastKey, lastDomain
		})
	}
	doneRouter := reporter.step("router", "Build the request router")
	adminOperations.DVR = recordingStore
	events := eventfeed.New(db)
	backupService := backup.New(state, db, ident.ID(), buildinfo.Info()["version"], persistence.SchemaVersion())
	backupService.SetRestartFunc(func() {
		restartRequested.Store(true)
		// The staged reply must flush before leaving the way SIGTERM does.
		supervise.Go("cmd.restore-restart", func() {
			time.Sleep(2 * time.Second)
			cancel()
		})
	})
	// The backup schedule: every five minutes a window holding the backup
	// task may start one scheduled backup, then prune to the keep count.
	backupScheduler := backupService.Schedule(db, func(scheduleCtx context.Context) (int, error) {
		return adminOperations.BackupKeepCount(scheduleCtx), nil
	})
	supervise.Supervise(ctx, "cmd.backup-schedule", backupScheduler.Run)
	events.LibraryVisible = func(tx *sql.Tx, p identity.Principal, library string) bool {
		return control.AllowedTx(p, library, tx) == nil
	}
	// Published access URLs come from the console settings (plus the custom
	// certificate domain); a settings save wakes the remote manager so the
	// changed list publishes at once — the route digest already covers the
	// observation, so it publishes exactly once.
	if manager := claimHandler.Remote(); manager != nil {
		manager.SetAccessURLs(accessURLsProvider(console, manager))
	}
	handler := httpapi.New(httpapi.Dependencies{Events: events, Administration: adminOperations, Backups: backupService, Access: administration, SettingsApplied: func(ctx context.Context) {
		control.NotifySettingsChanged(ctx)
		supervise.Go("delivery-settings.refresh", func() { deliverySettings.Refresh(ctx) })
		claimHandler.Remote().Wake()
		invalidateCustomSource()
	}, WebDirectory: os.Getenv("PORTICO_WEB_DIR"), Analysis: analysisService, Prepared: preparedVersions, Downloads: offlineDownloads, DVR: recordingStore, Subtitles: subtitleService, Lyrics: lyricService, LyricsBulk: lyricFetch, LyricsProbe: ffprobeTool, LibraryChannels: generatedChannels, Console: console, Scheduler: scheduler, Measurements: measurements, Networking: claimHandler, CustomCertificate: customCertificate, RouteIdentity: networking.NewRouteIdentityHandler(claimKeys, claimRunner), LiveChannels: liveSources, PlaybackRuntime: currentPlayback, AudioMedia: subtitleRenderer, Storage: storeIO, Mounts: managed, RemoteSources: sources, TrustedProxies: trustedProxies, DB: db, Watchdog: watchdog, Identity: ident, Catalog: cat, Ingestion: scanner, Playback: player, Hosted: control, Metadata: meta, Origins: origins})
	// The timeout and 5xx log lines name the registered route pattern, which
	// only the router can resolve for a request seen outside the mux.
	measurements.Pattern = httpapi.RoutePattern
	doneRouter(nil)
	supervise.Supervise(ctx, "cmd.api-events", events.Run)
	supervise.Supervise(ctx, "cmd.scan-events", func(ctx context.Context) { scanevents.Run(ctx, db, cat.InventoryStatus) })
	// The outermost wrapper: a handler panic is counted and attributed here, and
	// answered with an honest 500 when nothing has reached the client yet.
	handlerSwitch.set(supervise.HandlerMiddleware(measurements.Wrap(handler)))
	serverReadyHook()
	supervise.Supervise(ctx, "cmd.visibility-rebuilder", cat.RunVisibilityRebuilder)
	supervise.Supervise(ctx, "cmd.compact-catalogue", compactcatalog.NewWorker(db).Run)
	server := &http.Server{Addr: bind, Handler: handlerSwitch, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	// The watchdog and the deferred database housekeeping both start only now, so
	// neither competes with the work of coming up.
	supervise.Supervise(ctx, "cmd.database-watchdog", watchdog.Run)
	supervise.Go("cmd.deferred-maintenance", func() {
		runDeferredMaintenance(dbwork.WithClass(ctx, dbwork.ClassMaintenance), db)
		// A changed episode name reader reads the sources the old one left with
		// a naming issue once more (NEW-26); a scan never re-reads them.
		if err := cat.ReparseEpisodicIssues(dbwork.WithClass(ctx, dbwork.ClassMaintenance)); err != nil && ctx.Err() == nil {
			log.Printf("Episode names couldn't all be read again; the rest resume on the next start: %v", err)
		}
	})
	supervise.Supervise(dbwork.WithClass(ctx, dbwork.ClassMaintenance), "cmd.database-housekeeping", func(life context.Context) {
		runDatabaseHousekeeping(life, db)
	})
	supervise.Go("hosted.storage", func() { control.ObserveStorage(ctx, state) })
	supervise.Go("cmd.http-shutdown", func() {
		<-ctx.Done()
		stop, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		// The Hosted notice has its own 2 s budget and runs beside the drain (A51).
		notified := make(chan struct{})
		supervise.Go("hosted.shutdown-notice", func() { defer close(notified); _ = control.NotifyShutdown(stop) })
		_ = server.Shutdown(stop)
		<-notified
	})
	certificates := claimHandler.Certificates()

	// A custom certificate serves even with no Hosted manager (unclaimed /
	// Direct Sign-In): the listener exists whenever either source can serve.
	customConfigured := false
	if console != nil {
		if document, err := console.Settings(ctx, operations.AllowServerScope); err == nil {
			customConfigured = document.Effective.CustomCertificatePath != "" && document.Effective.CustomCertificateKeyPath != "" && document.Effective.CustomCertificateDomain != ""
		}
	}
	if certificates != nil || !ip.IsLoopback() || customConfigured {
		direct, err := boundedDirectListener(listener, maximumConnections, func(raw net.Listener) (*networking.DirectListener, error) {
			return networking.NewDirectListenerWithCustom(raw, certificates, customCertificate)
		})
		if err != nil {
			return err
		}
		defer direct.Close()
		_, port, _ := net.SplitHostPort(listener.Addr().String())
		numericPort, _ := strconv.Atoi(port)
		if certificates != nil {
			certificates.SetListening(numericPort, true)
			defer certificates.SetListening(numericPort, false)
			certificateCtx, cancelCertificates := context.WithCancel(ctx)
			certificateDone := make(chan struct{})
			supervise.Go("cmd.certificates", func() {
				defer close(certificateDone)
				supervise.Loop(certificateCtx, "cmd.certificates", certificates.Run)
			})
			// Joined through the barrier, before the claim keys and the database
			// close, and bounded like everything else.
			drain.add("certificates", cancelCertificates, certificateDone)
		}
		listener = direct
	}
	if manager := claimHandler.Remote(); manager != nil {
		control.OnRouteLabel(manager.RouteLabel)
		networkCtx, stopNetwork := context.WithCancel(ctx)
		networkDone := make(chan struct{})
		supervise.Go("cmd.remote-network", func() {
			defer close(networkDone)
			supervise.Loop(networkCtx, "cmd.remote-network", manager.Run)
		})
		drain.add("remote-network", stopNetwork, networkDone)
	}
	if env("PORTICO_DISCOVERY", "1") != "0" {
		administration.Advertiser.Bind(ctx, func(life context.Context) {
			networking.NewDiscovery(claimKeys, claimRunner, ident.Name).Run(life, listener.Addr().String())
		})
		discoveryDone := make(chan struct{})
		drain.add("discovery", func() {
			supervise.Go("cmd.discovery.stop", func() { administration.Advertiser.Close(); close(discoveryDone) })
		}, discoveryDone)
	}

	// The lanes protect handlers; this protects the process. Every accepted
	// connection is a goroutine and two buffers before any lane is consulted.
	if certificates == nil && ip.IsLoopback() {
		listener = capConnections(listener, maximumConnections)
	}
	log.Printf("Portico server listening on %s (remote traffic requires TLS; recovery HTTP is private-LAN only)", listener.Addr())
	// ONB-02 item 1: while setup is unfinished, every start names the address
	// to open. The port comes from the bound listener, not the configured
	// string, so a :0 or overridden bind still prints a reachable URL.
	if ident.SetupRequired() {
		if _, portStr, err := net.SplitHostPort(listener.Addr().String()); err == nil {
			if port, err := strconv.Atoi(portStr); err == nil {
				logSetupBlock(port, ip.IsLoopback())
			}
		}
	}
	e = server.Serve(listener)
	if errors.Is(e, http.ErrServerClosed) {
		if restartRequested.Load() {
			return errRestartRequested
		}
		return nil
	}
	return e
}
