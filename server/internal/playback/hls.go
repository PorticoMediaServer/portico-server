package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediaexec"
	"portico.local/server/internal/mediatools"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
	"portico.local/server/internal/supervise"
	"portico.local/server/internal/worker"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrConversionSpace is the answer when the volume holding generated media has
// no room left to start another conversion. It is a beyond-target condition:
// nothing a viewer does at target load reaches it, and it is retryable because
// the eviction sweep may free room within seconds.
var ErrConversionSpace = errors.New("not enough free space to start media conversion; retry shortly")

// Only owner-configured conversion limits govern admission. CPU count and
// measured server load are diagnostics, never a playback quality or admission rule.

// HLS owns conversion lifetimes only; planning and grants remain in Service.
type HLS struct {
	subtitleService *subtitles.Service
	WindowInputs    subtitles.WindowInputs
	// fmp4 turns on fragmented MP4 output for plans that need it.
	fmp4          bool
	finite        *FiniteHLS
	SourceStorage *storage.Client
	db            *sql.DB
	root, binary  string
	mu            sync.Mutex
	cleanupMu     sync.Mutex
	cleaning      map[string]bool
	active        map[string]context.CancelFunc
	windows       map[string]*hlsWindow
	// restarts bounds how often one session may restart its producer, and
	// segmentWaits shares one wait between every request for the same segment.
	// Both exist for clients on bad networks; see hls_bad_network.go.
	restarts     map[string]*restartBudget
	segmentWaits map[string]*segmentWait
	// abandoned counts conversions reclaimed because nobody was fetching from
	// them any more.
	abandoned atomic.Uint64
	// conversionsRefused counts new conversions turned away by the host-capacity
	// bound, so an owner whose machine is too small for their library sees the
	// number rather than guessing from latency.
	conversionsRefused atomic.Uint64
	// hardwareFailures counts windows whose hardware encoder failed on a real
	// source and were run again in software.
	hardwareFailures atomic.Uint64
	// reclaimed is the lowest slot each session still retains, so retention
	// never re-walks the whole played region.
	reclaimed map[string]int
	// timelines holds the cut list of every live copy session; a session absent
	// from it is on the fixed six-second grid. ffprobe indexes those cut lists.
	timelines map[string]*copyTimeline
	ffprobe   string
	// usage is the measured footprint of each generated session and when a viewer
	// last saw a segment of it. It is what makes the per-second sweep cheap and
	// what orders eviction under the global byte budget; see hls_sweep.go.
	usage       map[string]*hlsUsage
	budgetSwept time.Time
	// producers joins every produce goroutine, so shutdown can wait for the
	// converters it just cancelled instead of leaving them to be killed with the
	// process and orphan their ffmpeg children.
	producers sync.WaitGroup
	settings  DeliverySettings
	ctx       context.Context
}

// hlsWindow is one producer run. A session has at most one at a time; a seek
// past its converted edge replaces it with a window that starts where the
// viewer actually is, which is the whole point of the fixed segment grid.
type hlsWindow struct {
	publicationError error
	videoConversion  bool
	start            int
	served           int
	produced         int
	throttled        bool
	relocating       bool
	cancel           context.CancelFunc
	done             chan struct{}
}

// coversDemand keeps near-edge sequential fetches and the first output of a
// newly relocated window together. Requested position is never delivered progress.
func (w *hlsWindow) coversDemand(from int) bool {
	return !w.relocating && !w.throttled && from >= w.start && from <= max(w.start, w.produced)+2
}

// ConfigureSettings wires owner administration into conversion lifetimes. A nil
// source keeps the published defaults.
func (h *HLS) ConfigureSettings(s DeliverySettings) { h.settings = s }

func (h *HLS) configuration() DeliveryConfiguration { return deliveryConfiguration(h.settings) }

// fragmentedOutput reports whether this producer can write fragmented MP4 HLS.
// Planning asks, so a build that cannot is never handed a plan that needs it.
func (h *HLS) fragmentedOutput() bool { return h.fragmentedOutputFor(decoder.CurrentToolchain()) }

func (h *HLS) fragmentedOutputFor(facts *decoder.ToolchainFacts) bool {
	if h == nil || !h.fmp4 {
		return false
	}
	return facts != nil && facts.Muxers["mp4"] && facts.Muxers["hls"]
}

func NewHLS(ctx context.Context, db *sql.DB, root, binary string) (*HLS, error) {
	if binary == "" {
		binary = mediatools.Resolve("ffmpeg")
	}
	resolved, e := exec.LookPath(binary)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	helper, e := os.Executable()
	if e != nil {
		return nil, e
	}
	return &HLS{fmp4: true, SourceStorage: storage.New(helper), db: db, root: root, binary: resolved, active: map[string]context.CancelFunc{}, windows: map[string]*hlsWindow{}, reclaimed: map[string]int{}, usage: map[string]*hlsUsage{}, timelines: map[string]*copyTimeline{}, ffprobe: siblingProbe(resolved), ctx: ctx}, nil
}

// start admits one producer window for a session. A window already covering the
// requested slot is left alone; a window behind it is replaced, because a viewer
// who has jumped forward is not served by a converter still grinding through the
// part they skipped.
func (h *HLS) start(ctx context.Context, id string, from int) error {
	// Resolve durable state before taking the producer mutex. A starved read
	// pool must not hold up already-running segment producers.
	var sessionState, expires, artifactStatus string
	var videoConversion bool
	if e := dbwork.QueryRow(ctx, h.db, `SELECT ps.state,ps.expires_at,COALESCE(a.status,''),COALESCE(d.video_action,'convert')='convert' FROM playback_sessions ps LEFT JOIN playback_artifacts a ON a.session_id=ps.id LEFT JOIN playback_delivery_plans d ON d.session_id=ps.id WHERE ps.id=?`, id).Scan(&sessionState, &expires, &artifactStatus, &videoConversion); e != nil {
		return e
	}
	if sessionState == "stopped" || sessionState == "ended" || sessionState == "failed" || expires <= time.Now().UTC().Format(time.RFC3339) {
		return errors.New("playback stopped")
	}
	if artifactStatus == "failed" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ctx.Err() != nil {
		return h.ctx.Err()
	}
	if h.cleaning[id] {
		return errors.New("playback output is being reclaimed")
	}
	if _, ok := h.active[id]; ok {
		window := h.windows[id]
		// A two-rendition audio session has no grid; one run produces everything.
		if window == nil {
			return nil
		}
		if window.coversDemand(from) {
			return nil
		}
		if window.relocating {
			h.mu.Unlock()
			select {
			case <-window.done:
			case <-ctx.Done():
				h.mu.Lock()
				return ctx.Err()
			}
			err := h.start(ctx, id, from)
			h.mu.Lock()
			return err
		}
		if !h.restartAllowedLocked(id) {
			// The producer this session already has keeps running. The caller's
			// poll answers "not ready yet", which is retryable and is exactly what
			// it would have answered while a restart was in progress anyway.
			return nil
		}
		window.relocating = true
		window.cancel()
		for key, cancel := range h.active {
			if producerSession(key) == id && key != id {
				cancel()
			}
		}
		h.mu.Unlock()
		<-window.done
		h.mu.Lock()

	}
	if _, ok := h.active[id]; ok {
		return nil
	}
	if !h.roomToProduce() {
		// Beyond-target behaviour only, and never a surprise: the caller turns this
		// into the typed, retryable 503 it already answers for a busy source.
		return ErrConversionSpace
	}
	ctx, cancel := context.WithCancel(h.ctx)
	h.active[id] = cancel
	h.windows[id] = &hlsWindow{videoConversion: videoConversion, start: from, served: -1, produced: from - 1, cancel: cancel, done: make(chan struct{})}
	h.producers.Add(1)
	supervise.Go("playback.hls.produce", func() {
		defer h.producers.Done()
		h.produce(ctx, id, from)
	})
	// A producer starts because somebody asked for it, so this is the moment the
	// viewer was last known to be there. Without it a conversion that is never
	// fetched from has no "served" time at all and could never be reclaimed.
	h.noteSessionServedLocked(id)
	return nil
}

// ConversionCapacity reports the host-capacity bound on simultaneous
// conversions, how many are running, and how many starts it has turned away.
// An owner whose machine is too small for their household sees the third number
// grow; nothing else in the server would tell them.
type ConversionCapacity struct {
	Active   int    `json:"active"`
	Capacity int    `json:"capacity"`
	Refused  uint64 `json:"refused"`
}

// Conversions snapshots conversion admission.
func (h *HLS) Conversions() ConversionCapacity {
	if h == nil {
		return ConversionCapacity{}
	}
	h.mu.Lock()
	active := 0
	for id, window := range h.windows {
		if _, running := h.active[id]; running && window.videoConversion {
			active++
		}
	}
	h.mu.Unlock()
	return ConversionCapacity{Active: active, Capacity: h.configuration().MaxConversions, Refused: h.conversionsRefused.Load()}
}

// Shutdown cancels every producer and waits for them. HLS.Run cancelled them and
// returned immediately, so ffmpeg children could outlive the process that
// started them, holding a source file open across a restart.
func (h *HLS) Shutdown(ctx context.Context) error {
	if h == nil {
		return nil
	}
	h.cancelAll()
	done := make(chan struct{})
	supervise.Go("playback.hls.shutdown", func() { h.producers.Wait(); close(done) })
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		// The caller's budget is the bound. Saying so by name is what lets the
		// drain barrier report which loop did not finish.
		return ctx.Err()
	}
}

func (h *HLS) produce(ctx context.Context, id string, from int) {
	defer func() {
		h.mu.Lock()
		delete(h.active, id)
		if window := h.windows[id]; window != nil && window.start == from {
			close(window.done)
			delete(h.windows, id)
		}
		h.mu.Unlock()
	}()
	// A panic-injection point, after this producer's own cleanup is registered:
	// PORTICO_CHAOS_PANIC=playback.hls.produce:0.01 fails one conversion in a
	// hundred the way a bad source would, and the session must fail cleanly with
	// no orphan and no other session affected.
	supervise.Chaos("playback.hls.produce")
	var aid, state, video string
	var generation int
	var duration float64
	e := dbwork.QueryRow(ctx, h.db, `SELECT ps.asset_id,ps.state,a.video_codec,ps.duration,ps.generation FROM playback_sessions ps JOIN catalog_assets a ON a.token=ps.asset_id WHERE ps.id=? AND ps.mode='hls'`, id).Scan(&aid, &state, &video, &duration, &generation)
	if e != nil || state == "stopped" || state == "ended" || state == "failed" {
		return
	}
	ctx = withProducerGeneration(ctx, generation)
	dir := filepath.Join(h.root, id)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return
	}
	if _, e = dbwork.ExecWrite(ctx, h.db, dbwork.ClassEstablishedPlayback, `INSERT INTO playback_artifacts(session_id,directory,status) VALUES(?,?,'building') ON CONFLICT(session_id) DO UPDATE SET status='building',error=''`, id, dir); e != nil {
		return
	}
	delivery, e := loadDeliveryPlan(h.db, id)
	if e != nil {
		h.failed(ctx, id)
		return
	}
	input, remoteInput, e := h.openWindowInput(ctx, id, delivery)
	if e != nil {
		h.failed(ctx, id)
		return
	}
	defer input.Close()
	if remoteInput != nil {
		defer remoteInput.Close()
	}
	path := input.Argument
	if delivery != nil {
		if e = h.loadTextRenditions(ctx, id, delivery); e != nil {
			h.failed(ctx, id)
			return
		}
	}
	if duration > 0 && !hlsDurationSupported(duration) {
		h.failedWith(ctx, id, "hls_duration_unsupported", "")
		return
	}
	grid := hlsSegmentCount(duration) > 0
	// A copied picture is cut on the source's own keyframes; see
	// hls_copy_timeline.go. A source that cannot be indexed is converted instead,
	// and the stored plan is changed to say so.
	var timeline *copyTimeline
	if grid && delivery != nil && delivery.VideoAction == "copy" {
		timeline, e = h.copyTimelineFor(ctx, id, aid, input, duration)
		if e != nil {
			if delivery, e = h.downgradeCopyPlan(ctx, id, delivery); e != nil {
				h.failedWith(ctx, id, FailureCopyTimeline, "")
				return
			}
		}
	}
	if grid {
		manifest := hlsTimelineManifest(duration)
		if timeline != nil {
			manifest = copyTimelineManifest(timeline)
		}
		if delivery != nil && delivery.OutputContainer == "fmp4_hls" {
			manifest = fragmentedManifest(manifest)
		}
		if e = publishRenditionManifests(dir, delivery, manifest); e != nil {
			h.failed(ctx, id)
			return
		}
	}
	if e = writeNamedManifest(dir, "generation", []byte(strconv.Itoa(generation))); e != nil {
		h.failed(ctx, id)
		return
	}
	h.mu.Lock()
	if timeline != nil {
		h.timelines[id] = timeline
	} else {
		delete(h.timelines, id)
	}
	h.mu.Unlock()
	position := float64(from) * HLSSegmentSeconds
	staging := ""
	if timeline != nil {
		if from >= timeline.count() {
			return
		}
		position = timeline.Boundaries[from]
		staging = filepath.Join(dir, "window-"+strconv.Itoa(from))
		_ = os.RemoveAll(staging)
		if e = os.MkdirAll(staging, 0700); e != nil {
			h.failed(ctx, id)
			return
		}
		defer os.RemoveAll(staging)
	}
	burn, cleanupBurn, burnErr := h.prepareBurnInput(ctx, id, dir, position, delivery)
	if burnErr != nil {
		h.failedWith(ctx, id, FailureConverter, "subtitle input unavailable")
		return
	}
	defer cleanupBurn()
	build := func(plan *DeliveryPlan) ([]string, []decoder.HardwareStage, error) {
		offset := strconv.FormatFloat(position, 'f', 6, 64)
		protocols := "file,pipe"
		if remoteInput != nil {
			protocols = "http,tcp"
		}
		args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-protocol_whitelist", protocols}
		if grid && from > 0 {
			// Input-side seek, so the decoder skips rather than decodes-and-drops.
			// A copy window seeks a hair past its keyframe: the demuxer lands on
			// the index entry at or before the target, and rounding the target
			// down by a tick must not send it to the keyframe before.
			seek := offset
			if timeline != nil {
				seek = strconv.FormatFloat(position+0.002, 'f', 6, 64)
			}
			args = append(args, "-ss", seek)
		}
		var inputArgs, codecArgs []string
		var stages []decoder.HardwareStage
		var extraInput []string
		videoMap := "0:v:0"
		if plan != nil {
			var err error
			graph, graphErr := plan.conversionGraph(burn)
			err = graphErr
			inputArgs, codecArgs, stages = graph.Input, append(graph.Video, graph.Audio...), graph.Stages
			extraInput = graph.ExtraInput
			if graph.VideoMap != "" {
				videoMap = graph.VideoMap
			}
			if err != nil {
				return nil, nil, err
			}
		}
		args = append(args, inputArgs...)
		args = append(args, "-i", path)
		args = append(args, extraInput...)
		// The plan names the programme audio by absolute stream index. A plan
		// without a choice (an older session) takes the first track, as before.
		audioMap := "0:a:0?"
		if plan != nil && plan.AudioStream >= 0 {
			audioMap = "0:" + strconv.Itoa(plan.AudioStream)
		}
		switch {
		case (plan != nil && plan.VideoAction == "none") || (plan == nil && video == ""):
			args = append(args, "-map", strings.TrimSuffix(audioMap, "?"), "-vn", "-sn", "-dn")
		case plan != nil && (plan.AudioAction == "none" || len(plan.Renditions) > 0):
			args = append(args, "-map", videoMap, "-an", "-sn", "-dn")
		default:
			args = append(args, "-map", videoMap, "-map", audioMap, "-sn", "-dn")
		}
		if plan != nil {
			args = append(args, codecArgs...)
		} else {
			// Existing sessions predate immutable delivery snapshots. Preserve their
			// original output policy; new sessions always consume their stored plan.
			if video == "h264" {
				args = append(args, "-c:v", "copy")
			} else if video != "" {
				args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-pix_fmt", "yuv420p", "-force_key_frames", "expr:gte(t,n_forced*6)")
			}
			args = append(args, "-c:a", "aac", "-b:a", "192k", "-ac", "2")
		}
		fragmented := plan != nil && plan.OutputContainer == "fmp4_hls"
		if fragmented {
			args = append(args, "-map_metadata", "-1", "-fflags", "+bitexact")
			if plan.OutputVideoCodec == "hevc" {
				args = append(args, "-tag:v", hevcSampleEntry(plan))
				if hevcSampleEntry(plan) == "dvh1" {
					args = append(args, "-strict", "unofficial")
				}
			}
		}
		args = append(args, "-max_muxing_queue_size", "1024")
		if timeline != nil {
			args = append(args, "-muxdelay", "0", "-muxpreload", "0")
			return append(args, copySegmentArgs(timeline, from, staging, fragmented)...), stages, nil
		}
		ext := "ts"
		if fragmented {
			ext = "m4s"
			args = append(args, "-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init-"+strconv.Itoa(from)+".mp4")
		}
		args = append(args, "-f", "hls", "-hls_time", strconv.Itoa(HLSSegmentSeconds), "-hls_list_size", "0", "-hls_flags", "independent_segments+temp_file")
		if grid {
			// Absolute source timestamps and a grid-aligned start number are what
			// make a restarted window continuous with everything already produced.
			args = append(args, "-output_ts_offset", offset, "-muxdelay", "0", "-muxpreload", "0", "-start_number", strconv.Itoa(from), "-hls_segment_filename", filepath.Join(dir, "segment-%06d."+ext), filepath.Join(dir, "window.m3u8"))
		} else {
			args = append(args, "-hls_playlist_type", "event", "-hls_segment_filename", filepath.Join(dir, "segment-%06d."+ext), filepath.Join(dir, "master.m3u8"))
		}
		return args, stages, nil
	}
	_, _ = dbwork.ExecWrite(ctx, h.db, dbwork.ClassEstablishedPlayback, `INSERT INTO playback_hls_windows(session_id,start_index,status,started_at_ms) VALUES(?,?,'running',?) ON CONFLICT(session_id,start_index) DO UPDATE SET status='running',started_at_ms=excluded.started_at_ms`, id, from, time.Now().UnixMilli())
	processCtx, cancel := context.WithTimeout(ctx, 6*time.Hour)
	defer cancel()
	watchCtx, stopWatch := context.WithCancel(processCtx)
	watchDone := make(chan struct{})
	if grid {
		limit := -1
		if timeline != nil {
			limit = min(from+copyWindowSegments, timeline.count()) - 1
		}
		supervise.Go("playback.hls.watch-window", func() {
			defer close(watchDone)
			h.watchWindow(watchCtx, id, dir, staging, from, limit, cancel, delivery, timeline)
		})
	} else {
		close(watchDone)
	}
	run := func(plan *DeliveryPlan) (error, string) {
		if remoteInput != nil {
			err := remoteInput.Run(processCtx, dir, func(url string) ([]string, error) {
				path = url
				args, stages, err := build(plan)
				if err == nil {
					h.recordStages(ctx, id, plan, stages)
				}
				return args, err
			})
			return err, ""
		}

		args, stages, err := build(plan)
		if err != nil {
			return err, ""
		}
		if len(stages) > 0 {
			h.recordStages(ctx, id, plan, stages)
		}
		// Every child starts at the beginning of the source. On platforms where
		// /dev/fd shares the file offset with this process, the index probe that
		// ran before has moved it.
		if input.File != nil {
			_, _ = input.File.Seek(0, io.SeekStart)
		}
		// The one media executor (mediaexec): sandboxed where the platform can,
		// the input only as descriptor 3, writes only into this session's folder,
		// and its own process group, so cancelling reaches the whole sandbox.
		cmd, err := mediaexec.CommandContext(processCtx, mediaexec.Job{Executable: h.binary, Args: args, Files: input.ExtraFiles(), ReadPaths: input.ReadPaths(), WriteDirs: []string{dir}, Fonts: true, Hardware: plan != nil && plan.HardwareBackend != "" && plan.HardwareBackend != string(decoder.BackendSoftware)})
		if err != nil {
			return err, ""
		}
		cmd.WaitDelay = 2 * time.Second
		tail := &tailWriter{limit: 4096}
		cmd.Stderr = tail
		err = cmd.Run()
		return err, sanitizeConverterOutput(tail.String(), input.Argument, h.root)
	}
	e, diagnostic := run(delivery)
	stopWatch()
	<-watchDone
	h.mu.Lock()
	var publicationError error
	if w := h.windows[id]; w != nil {
		publicationError = w.publicationError
	}
	h.mu.Unlock()
	if publicationError != nil {
		h.fragmentedFailure(ctx, id, delivery, position, publicationError)
		return
	}
	throttled := func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		window := h.windows[id]
		return window != nil && window.start == from && window.throttled
	}
	// A hardware encoder that probed clean at startup can still refuse a real
	// source: a resolution the block does not take, a driver that has since
	// wedged, a session limit on a consumer GPU. The viewer is not told about
	// any of that. The same window is run again in software, and the stored plan
	// is changed so later windows do not repeat the failure.
	if e != nil && !throttled() && ctx.Err() == nil && delivery != nil && delivery.VideoAction == "convert" && delivery.HardwareBackend != "" && delivery.HardwareBackend != string(decoder.BackendSoftware) {
		failedBackend := delivery.HardwareBackend
		software := *delivery
		software.HardwareBackend, software.HardwareNative, software.HardwareDevice = string(decoder.BackendSoftware), false, ""
		software.ReasonCodes = append(append([]string{}, delivery.ReasonCodes...), ReasonHardwareFailedSoftware)
		h.recordSoftwareFallback(ctx, id, failedBackend, diagnostic)
		delivery = &software
		if staging != "" {
			_ = os.RemoveAll(staging)
			_ = os.MkdirAll(staging, 0700)
		}
		watchCtx, stopWatch = context.WithCancel(processCtx)
		watchDone = make(chan struct{})
		if grid {
			supervise.Go("playback.hls.watch-software-window", func() {
				defer close(watchDone)
				h.watchWindow(watchCtx, id, dir, staging, from, -1, cancel, delivery, timeline)
			})
		} else {
			close(watchDone)
		}
		e, diagnostic = run(delivery)
		stopWatch()
		<-watchDone
		h.mu.Lock()
		if w := h.windows[id]; w != nil {
			publicationError = w.publicationError
		}
		h.mu.Unlock()
		if publicationError != nil {
			h.fragmentedFailure(ctx, id, delivery, position, publicationError)
			return
		}
	}
	if e != nil && !throttled() && ctx.Err() == nil {
		h.failedWith(ctx, id, classifyConverterFailure(diagnostic), diagnostic)
		return
	}
	status := "complete"
	if throttled() {
		status = "throttled"
	} else if e != nil {
		status = "failed"
	}
	// Record the final edge here as well as on the watcher's tick: a window that
	// finishes inside one tick would otherwise publish no progress at all.
	produced := from - 1
	if timeline != nil {
		if delivery != nil && delivery.OutputContainer == "fmp4_hls" {
			var promoteErr error
			produced, promoteErr = promoteFMP4Segments(staging, dir, timeline, from, e == nil)
			if promoteErr != nil {
				h.fragmentedFailure(ctx, id, delivery, position, promoteErr)
				return
			}
		} else {
			produced = promoteCopySegments(staging, dir, from, e == nil)
		}
	} else if grid {
		produced = highestProducedSegment(dir, from)
		if delivery != nil && delivery.OutputContainer == "fmp4_hls" {
			if err := promoteConvertedInit(dir, from); err != nil {
				h.fragmentedFailure(ctx, id, delivery, position, err)
				return
			}
		}
	}
	_, _ = dbwork.ExecWrite(ctx, h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_hls_windows SET status=?,highest_produced=max(highest_produced,?) WHERE session_id=? AND start_index=?`, status, produced, id, from)
	if e == nil && (timeline == nil || produced >= timeline.count()-1) {
		_, _ = dbwork.ExecWrite(ctx, h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_artifacts SET status='complete' WHERE session_id=?`, id)
	}
}

// watchWindow enforces the throttle and published progress. Conversion that has
// run far enough ahead of the viewer is stopped rather than paused: a stopped
// converter costs nothing, and the fixed grid means the next demand restarts it
// exactly where the viewer is instead of where it left off.
func (h *HLS) watchWindow(ctx context.Context, id, dir, staging string, from, limit int, cancel context.CancelFunc, plan *DeliveryPlan, timeline *copyTimeline) {
	cfg := h.configuration()
	interval := 500 * time.Millisecond
	if staging != "" {
		// A copy window's segments only become visible when this loop promotes
		// them, so it runs at the pace a player waits at.
		interval = 100 * time.Millisecond
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		produced := from - 1
		if staging != "" {
			if plan != nil && plan.OutputContainer == "fmp4_hls" {
				var err error
				produced, err = promoteFMP4Segments(staging, dir, timeline, from, false)
				if err != nil {
					h.publicationFailed(id, err)
					cancel()
					return
				}
			} else {
				produced = promoteCopySegments(staging, dir, from, false)
			}
		} else {
			produced = highestProducedSegment(dir, from)
			if plan != nil && plan.OutputContainer == "fmp4_hls" {
				if err := promoteConvertedInit(dir, from); err != nil && !os.IsNotExist(err) {
					h.publicationFailed(id, err)
					cancel()
					return
				}
			}
		}
		h.mu.Lock()
		window := h.windows[id]
		if window == nil || window.start != from {
			h.mu.Unlock()
			return
		}
		if produced > window.produced {
			window.produced = produced
			_, _ = dbwork.ExecWrite(ctx, h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_hls_windows SET highest_produced=? WHERE session_id=? AND start_index=?`, produced, id, from)
		}
		ahead := (window.produced - max(window.served, window.start-1)) * HLSSegmentSeconds
		// A copy window that has used up its cut list stops the same way a
		// throttled one does: the next request continues from there.
		exhausted := limit >= 0 && window.produced >= limit-1 && limit < h.timelines[id].count()-1
		if (cfg.ThrottleBufferSeconds > 0 && window.produced >= from && ahead >= cfg.ThrottleBufferSeconds) || exhausted {
			window.throttled = true
			for key, stop := range h.active {
				if key != id && producerSession(key) == id {
					stop()
				}
			}
			h.mu.Unlock()
			_, _ = dbwork.ExecWrite(ctx, h.db, dbwork.ClassEstablishedPlayback, `INSERT INTO playback_hls_demand(session_id,highest_served,throttled) VALUES(?,?,1) ON CONFLICT(session_id) DO UPDATE SET throttled=1`, id, produced)
			cancel()
			return
		}
		h.mu.Unlock()
	}
}

// recordStages publishes what the conversion graph actually did, so the
// occurrence diagnostic can say where each operation ran without re-deriving it.
// persist is the context a "this must be recorded" write runs under. The
// original used context.Background(), which is unbounded: called while serving
// a .ts segment, a contended gate would hold the write — and the media-body slot
// behind it — long past the request's own deadline. WithoutCancel keeps the
// intent (the record survives the request being abandoned) and the timeout keeps
// the promise that nothing on the request path is unbounded.
func persist(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
}

func (h *HLS) recordStages(ctx context.Context, id string, plan *DeliveryPlan, stages []decoder.HardwareStage) {
	raw, err := json.Marshal(stages)
	if err != nil {
		return
	}
	backend := string(decoder.BackendSoftware)
	if plan != nil && plan.HardwareBackend != "" {
		backend = plan.HardwareBackend
	}
	write, cancel := persist(ctx)
	defer cancel()
	_, _ = dbwork.ExecWrite(write, h.db, dbwork.ClassEstablishedPlayback, `INSERT INTO playback_hls_stages(session_id,backend,stages_json) VALUES(?,?,?) ON CONFLICT(session_id) DO UPDATE SET backend=excluded.backend,stages_json=excluded.stages_json`, id, backend, string(raw))
}

// producerGenerationKey carries the session generation a producer started
// for. A subtitle change (burn-in on or off) replans a session under a new
// generation while the old producer may still be winding down; its failure
// must not fail the presentation that replaced it.
type producerGenerationKey struct{}

func withProducerGeneration(ctx context.Context, generation int) context.Context {
	return context.WithValue(ctx, producerGenerationKey{}, generation)
}

// failed fails the session's conversion, and reports whether it did: a
// producer whose generation has been replaced fails nothing.
func (h *HLS) failed(ctx context.Context, id string) bool {
	write, cancel := persist(ctx)
	defer cancel()
	query, args := `UPDATE playback_sessions SET state='failed' WHERE id=? AND state NOT IN ('stopped','ended')`, []any{id}
	if generation, ok := ctx.Value(producerGenerationKey{}).(int); ok {
		query, args = query+` AND generation=?`, append(args, generation)
	}
	result, err := dbwork.ExecWrite(write, h.db, dbwork.ClassEstablishedPlayback, query, args...)
	if err == nil {
		if n, e := result.RowsAffected(); e == nil && n == 0 {
			if _, fenced := ctx.Value(producerGenerationKey{}).(int); fenced {
				return false
			}
		}
	}
	_, _ = dbwork.ExecWrite(write, h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_artifacts SET status='failed',error='Media conversion failed; source may use unsupported streams.' WHERE session_id=?`, id)
	return true
}
func (h *HLS) Ready(ctx context.Context, id string) error {
	if h.finite != nil {
		plan, err := loadDeliveryPlan(h.db, id)
		if err != nil {
			return err
		}
		if plan != nil && plan.Profile == finiteDeliveryProfile {
			_, err = h.finite.ensure(ctx, id)
			return err
		}
	}
	if e := h.start(ctx, id, 0); e != nil {
		return e
	}
	wait, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		var status, state string
		var generation int
		if err := dbwork.QueryRow(ctx, h.db, `SELECT ps.state,COALESCE(a.status,''),ps.generation FROM playback_sessions ps LEFT JOIN playback_artifacts a ON a.session_id=ps.id WHERE ps.id=?`, id).Scan(&state, &status, &generation); err != nil {
			return err
		}
		if state == "stopped" || state == "ended" {
			return errors.New("playback stopped")
		}
		if status == "failed" {
			return errors.New("media conversion failed")
		}
		if info, e := os.Stat(filepath.Join(h.root, id, "master.m3u8")); e == nil && info.Size() > 0 {
			stamp, err := os.ReadFile(filepath.Join(h.root, id, "generation"))
			if err == nil && string(stamp) == strconv.Itoa(generation) {
				return nil
			}
		}
		select {
		case <-wait.Done():
			return errors.New("conversion startup not ready; retry the same requestId")
		case <-tick.C:
		}
	}
}

var artifactName = regexp.MustCompile(`^(master\.m3u8|video\.m3u8|text-[0-9]{1,2}\.m3u8|audio-[0-9]{1,2}\.m3u8|init\.mp4|segment-[0-9]{6}\.(?:ts|m4s)|audio-[0-9]{1,2}-[0-9]{6}\.ts)$`)

func (h *HLS) File(grant, name string) (string, error) {
	return h.FileContext(context.Background(), grant, name)
}

// FileContext resolves one artifact and, for a segment the converter has not
// reached, moves the converter. A seek is nothing more than a request for a
// slot further along the grid: if it already exists the read is instant, and if
// it does not the session's own producer restarts there rather than converting
// everything in between.
func (h *HLS) FileContext(ctx context.Context, grant, name string) (string, error) {
	if !artifactName.MatchString(name) {
		return "", errors.New("unknown stream artifact")
	}
	var id string
	var generation int
	var duration float64
	e := dbwork.QueryRow(ctx, h.db, `SELECT id,duration,generation FROM playback_sessions WHERE grant_hash=? AND mode='hls' AND state NOT IN('stopped','ended','failed') AND expires_at>?`, identity.Digest(grant), time.Now().UTC().Format(time.RFC3339)).Scan(&id, &duration, &generation)
	if e != nil {
		return "", e
	}
	index, segment := hlsSegmentIndex(name)
	if strings.HasPrefix(name, "segment-") && !segment {
		return "", errors.New("unknown stream artifact")
	}
	stamp, stampErr := os.ReadFile(filepath.Join(h.root, id, "generation"))
	if stampErr != nil || string(stamp) != strconv.Itoa(generation) {
		// Only a wholly evicted directory is recoverable here. An existing
		// directory with a different generation must never become current.
		if os.IsNotExist(stampErr) {
			if _, err := os.Stat(filepath.Join(h.root, id)); os.IsNotExist(err) {
				from, segment := hlsSegmentIndex(name)
				if !segment {
					from = 0
				}
				if segment && from >= hlsSegmentCount(duration) {
					return "", errors.New("unknown stream artifact")
				}
				if err := h.start(ctx, id, from); err != nil {
					return "", err
				}
			}
		}
		return "", ErrSegmentPreparing
	}
	delivery, err := loadDeliveryPlan(h.db, id)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(name, "audio-") {
		if delivery == nil || len(delivery.Renditions) == 0 {
			return "", errors.New("unknown stream artifact")
		}
		return h.renditionFile(ctx, id, name, delivery)
	}
	path := filepath.Join(h.root, id, name)
	if name == "init.mp4" {
		return h.awaitSegment(ctx, id, -1, func(ctx context.Context) (string, error) {
			wait, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			tick := time.NewTicker(100 * time.Millisecond)
			defer tick.Stop()
			for {
				if segmentReady(path) {
					return path, nil
				}
				select {
				case <-wait.Done():
					return "", ErrSegmentPreparing
				case <-tick.C:
				}
			}
		})
	}
	if !segment || hlsSegmentCount(duration) == 0 {
		return path, nil
	}
	count := hlsSegmentCount(duration)
	if timeline := h.sessionTimeline(ctx, id); timeline != nil {
		count = timeline.count()
	}
	if index >= count {
		return "", errors.New("unknown stream artifact")
	}
	cfg := h.configuration()
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		h.noteServed(ctx, id, index, cfg)
		return path, nil
	}
	if e = h.start(ctx, id, index); e != nil {
		return "", e
	}
	// Everybody waiting for this segment waits once. Ten reconnecting clients
	// asking for the same segment used to run ten poll loops between them.
	return h.awaitSegment(ctx, id, index, func(ctx context.Context) (string, error) {
		wait, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			var state, status string
			if err := dbwork.QueryRow(ctx, h.db, `SELECT ps.state,COALESCE(a.status,'') FROM playback_sessions ps LEFT JOIN playback_artifacts a ON a.session_id=ps.id WHERE ps.id=?`, id).Scan(&state, &status); err != nil {
				return "", err
			}
			if state == "stopped" || state == "ended" || state == "failed" {
				return "", errors.New("playback stopped")
			}
			if status == "failed" {
				return "", errors.New("media conversion failed")
			}
			if segmentReady(path) {
				h.noteServed(ctx, id, index, cfg)
				return path, nil
			}
			select {
			case <-wait.Done():
				return "", ErrSegmentPreparing
			case <-tick.C:
			}
		}
	})
}

// noteServed records the viewer's real position in the conversion, which is what
// both the throttle and the retention window are measured against.
func (h *HLS) noteServed(ctx context.Context, id string, index int, cfg DeliveryConfiguration) {
	h.noteSessionServed(id)
	h.mu.Lock()
	restart := false
	if window := h.windows[id]; window != nil {
		if index > window.served {
			window.served = index
		}
		restart = window.throttled
	}
	from := h.reclaimed[id]
	h.mu.Unlock()
	write, cancel := persist(ctx)
	defer cancel()
	_, _ = dbwork.ExecWrite(write, h.db, dbwork.ClassEstablishedPlayback, `INSERT INTO playback_hls_demand(session_id,highest_served,throttled) VALUES(?,?,0) ON CONFLICT(session_id) DO UPDATE SET highest_served=max(playback_hls_demand.highest_served,excluded.highest_served),throttled=0`, id, index)
	if _, reclaimed := prunePlayedSegments(filepath.Join(h.root, id), from, index, cfg.PlayedRetentionSeconds); reclaimed > from {
		h.mu.Lock()
		if h.reclaimed[id] < reclaimed {
			h.reclaimed[id] = reclaimed
		}
		h.mu.Unlock()
	}
	if restart {
		_ = h.start(ctx, id, index+1)
	}
}

func (h *HLS) Run(ctx context.Context) {
	wake := worker.NewSignal()
	unregister := dbwork.WakeOnCommit(wake)
	defer unregister()
	defer h.cancelAll()
	worker.Run(ctx, "playback.hls-maintenance", wake, func(ctx context.Context) time.Duration {
		h.sweep(ctx)
		cleanupErr := h.CleanupGenerated(ctx)
		if cleanupErr != nil || len(h.activeSnapshot()) > 0 {
			return time.Second
		}
		return 0
	})
}

// CleanupGenerated reclaims only server-owned, terminal/expired output. The same
// per-session claim fences start and finite conversion; all IO runs outside the
// producer mutex.
// Database-provided paths are never followed; live producers retain their output.
func (h *HLS) CleanupGenerated(ctx context.Context) error {
	h.cleanupMu.Lock()
	defer h.cleanupMu.Unlock()
	rows, err := h.db.QueryContext(ctx, `SELECT a.session_id FROM (SELECT session_id FROM playback_artifacts UNION SELECT session_id FROM playback_hls_reservations) a LEFT JOIN playback_sessions ps ON ps.id=a.session_id WHERE ps.id IS NULL OR ps.state IN ('stopped','ended','failed') OR ps.expires_at<=? ORDER BY a.session_id LIMIT 128`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return err
		}
		if !generatedDirectoryID.MatchString(id) {
			return errors.New("invalid generated artifact identity")
		}
		h.mu.Lock()
		if h.sessionActiveLocked(id) {
			h.mu.Unlock()
			continue
		}
		if h.cleaning == nil {
			h.cleaning = map[string]bool{}
		}
		h.cleaning[id] = true
		h.mu.Unlock()
		err = func() error {
			defer func() { h.mu.Lock(); delete(h.cleaning, id); h.mu.Unlock() }()
			var reclaim bool
			if e := h.db.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM playback_sessions WHERE id=? AND state NOT IN ('stopped','ended','failed') AND expires_at>?)`, id, time.Now().UTC().Format(time.RFC3339)).Scan(&reclaim); e != nil {
				return e
			}
			if !reclaim {
				return nil
			}
			// RemoveAll does not follow a directory symlink. The root is configured by
			// the server and id has no separators/dot segments; source files are excluded.
			if e := os.RemoveAll(filepath.Join(h.root, id)); e != nil {
				return e
			}
			gated, e := dbwork.Begin(ctx, h.db, dbwork.ClassPlaybackStart)
			if e != nil {
				return e
			}
			tx := gated.Tx()
			defer gated.Rollback()
			if _, e = tx.ExecContext(ctx, `DELETE FROM playback_artifacts WHERE session_id=?`, id); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `DELETE FROM playback_hls_reservations WHERE session_id=?`, id); e != nil {
				return e
			}
			return gated.Commit()
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

var generatedDirectoryID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,160}$`)
