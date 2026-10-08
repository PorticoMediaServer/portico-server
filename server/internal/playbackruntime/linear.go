package playbackruntime

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/capabilityreport"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/supervise"
	workerloop "portico.local/server/internal/worker"
	"strconv"
	"strings"
	"sync"
	"time"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/linearbuffer"
	"portico.local/server/internal/linearinput"
	"portico.local/server/internal/livechannels"
	library "portico.local/server/internal/livechannels/library"
	"portico.local/server/internal/mediaexec"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/producerinput"
)

type LinearConfig struct {
	DB                                             *sql.DB
	Live                                           *livechannels.Store
	Library                                        *library.Store
	Policy                                         playback.CachedPreparationPolicy
	CacheDirectory, LockDirectory, FFmpeg, FFprobe string
	DecoderLibraries                               []string
	Retention                                      time.Duration
	MaxBytes                                       int64
}
type LinearRuntime struct {
	owner *Runtime
	// authority is the work's owner: a v1 channel session.
	authority    *playback.ChannelSessions
	inputs       directInputs
	config       LinearConfig
	locks        *livechannels.PhysicalLocks
	availability string
	mu           sync.Mutex
	entries      map[string]*linearEntry
	skips        map[string]string // session -> last logged reason it can't start
	wg           sync.WaitGroup
}
type linearEntry struct {
	mu          sync.Mutex
	buffer      *linearbuffer.Buffer
	token       string
	generation  int64
	producer    int64
	selection   playback.LinearSelection
	selections  map[int64]playback.LinearSelection
	state, code string
	logged      string // the last start failure written to the log
	cancel      context.CancelFunc
	requests    chan struct{}
}

func privateDirectory(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("linear cache must be a canonical private directory")
	}
	if e := os.MkdirAll(path, 0700); e != nil {
		return e
	}
	canonical, e := filepath.EvalSymlinks(path)
	if e != nil || canonical != path {
		return errors.New("linear cache cannot traverse symlinks")
	}
	info, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !info.IsDir() {
		return errors.New("linear cache must be private")
	}
	return nil
}
func resolveDecoder(name string) (string, error) {
	p, e := exec.LookPath(name)
	if e != nil {
		return "", e
	}
	p, e = filepath.Abs(p)
	if e != nil {
		return "", e
	}
	p, e = filepath.EvalSymlinks(p)
	if e != nil {
		return "", e
	}
	info, e := os.Stat(p)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return "", errors.New("decoder unavailable")
	}
	return p, nil
}

// confinementProbe says why decoders may not run here at all; a variable so
// tests can stand in for the host.
var confinementProbe = decoder.RequiredConfinement

// linearSandboxInUse says whether media jobs with this tool run sandboxed; a
// variable so tests can stand in for a host without a sandbox.
var linearSandboxInUse = func(executable string) bool {
	return mediaexec.Decide(executable).Sandboxed
}

// linearAvailability is why live can't run on this host ("" when it can) and
// the concrete cause. Live runs on every platform: sandboxed where the platform
// can (macOS sandbox-exec, Linux bubblewrap with user namespaces), otherwise
// with the baseline restrictions, which the owner's diagnostics report
// (D-MEDIA-6, BE-MEDIA-02). Only an owner who requires the sandbox on a host
// without one gets decoder_confinement_unavailable.
func linearAvailability(ffmpeg, ffprobe error, probe func() error) (string, error) {
	if err := probe(); err != nil {
		return "decoder_confinement_unavailable", err
	}
	if ffprobe != nil {
		return "ffprobe_not_configured", fmt.Errorf("ffprobe: %w", ffprobe)
	}
	if ffmpeg != nil {
		return "ffmpeg_not_configured", fmt.Errorf("ffmpeg: %w", ffmpeg)
	}
	return "", nil
}

// ConfigureChannels is called before Start. A real missing binary/confinement
// backend is reported as configuration/support state, never a qualification gate.
func (r *Runtime) ConfigureChannels(c LinearConfig) error {
	if c.DB == nil || c.Live == nil || c.Library == nil || c.Policy == nil || r.Linear != nil {
		return errors.New("invalid channel runtime dependencies")
	}
	if c.Retention == 0 {
		c.Retention = 30 * time.Minute
	}
	if c.MaxBytes == 0 {
		c.MaxBytes = 1 << 30
	}
	if c.Retention < 12*time.Second || c.Retention > 24*time.Hour || c.MaxBytes < 64<<20 || c.MaxBytes > 16<<30 {
		return errors.New("invalid channel retention limits")
	}
	if e := privateDirectory(c.CacheDirectory); e != nil {
		return e
	}
	if e := linearbuffer.Sweep(c.CacheDirectory); e != nil {
		return e
	}
	locks, e := livechannels.NewPhysicalLocks(c.LockDirectory)
	if e != nil {
		return e
	}
	l := &LinearRuntime{owner: r, config: c, locks: locks, entries: map[string]*linearEntry{}}
	var ffmpegErr, ffprobeErr, cause error
	l.config.FFmpeg, ffmpegErr = resolveDecoder(c.FFmpeg)
	l.config.FFprobe, ffprobeErr = resolveDecoder(c.FFprobe)
	l.availability, cause = linearAvailability(ffmpegErr, ffprobeErr, func() error { return confinementProbe(l.config.FFmpeg) })
	if l.availability == "" {
		libraries, e := decoderLibraries(l.config.FFmpeg, l.config.FFprobe, c.DecoderLibraries)
		if e != nil {
			l.availability, cause = "decoder_dependencies_unavailable", fmt.Errorf("FFmpeg's libraries can't be resolved: %w", e)
		} else {
			l.config.DecoderLibraries = libraries
		}
	}
	capabilityreport.Report(capabilityreport.LiveTV, l.availability == "", l.availability, cause)
	resolver := NewChannelResolver(c.Live, c.Library, c.Policy, l.Available)
	if e = r.Channels.ConfigureLinear(resolver, l); e != nil {
		return e
	}
	l.authority = r.Channels
	r.Linear = l
	return nil
}
func (l *LinearRuntime) Available() (bool, string) {
	return l != nil && l.availability == "", func() string {
		if l == nil {
			return "channel_runtime_unavailable"
		}
		return l.availability
	}()
}
func (l *LinearRuntime) run(ctx context.Context) {
	defer l.wg.Wait()
	defer func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		for _, e := range l.entries {
			e.cancel()
		}
	}()
	urgent, unsubscribe := l.owner.wake.Subscribe()
	defer unsubscribe()
	shared := workerloop.NewSignal()
	unregister := dbwork.WakeOnCommit(shared)
	defer unregister()
	lastReconcile := time.Time{}
	workerloop.RunWith(ctx, "playbackruntime.linear", urgent, shared, func(ctx context.Context) time.Duration {
		if time.Since(lastReconcile) >= 5*time.Second {
			check, cancel := context.WithTimeout(ctx, 2*time.Second)
			_, _ = l.locks.ReconcileExpired(check, l.config.DB, time.Now())
			cancel()
			lastReconcile = time.Now()
		}
		if l.availability != "" {
			return 0
		}
		after := ""
		for {
			ids, e := l.owner.Linear.authority.LinearWorkIDs(ctx, after, 128)
			if e != nil {
				break
			}
			for _, id := range ids {
				after = id
				l.mu.Lock()
				existing := l.entries[id]
				l.mu.Unlock()
				if existing != nil {
					continue
				}
				if _, e = l.owner.Linear.authority.LinearWork(ctx, id); e != nil {
					l.skipped(id, e)
					continue
				}
				life, cancel := context.WithCancel(ctx)
				entry := &linearEntry{cancel: cancel, selections: map[int64]playback.LinearSelection{}, requests: make(chan struct{}, 16), state: "preparing"}
				l.mu.Lock()
				l.entries[id] = entry
				l.mu.Unlock()
				l.wg.Add(1)
				supervise.Go("playbackruntime.linear.entry", func() {
					e := entry
					defer l.wg.Done()
					defer e.cancel()
					defer func() {
						e.mu.Lock()
						b := e.buffer
						e.mu.Unlock()
						if b != nil {
							_ = b.Close()
						}
						l.mu.Lock()
						if l.entries[id] == e {
							delete(l.entries, id)
						}
						l.mu.Unlock()
					}()
					l.runOccurrence(life, id, e)
				})
			}
			if len(ids) < 128 {
				break
			}
		}
		l.mu.Lock()
		active := len(l.entries) > 0
		l.mu.Unlock()
		if active {
			return 500 * time.Millisecond
		}
		return 0
	})
}
func waitLinear(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func (l *LinearRuntime) runOccurrence(ctx context.Context, id string, entry *linearEntry) {
	// A buffer that can't be opened is reported on the occurrence (recoverable,
	// with its code) and retried with backoff: never a silent return that the
	// next wake repeats while the viewer waits on "preparing".
	var work playback.LinearWork
	var e error
	for attempt := 0; ; attempt++ {
		if work, e = l.owner.Linear.authority.BeginLinearBuffer(ctx, id); e == nil {
			break
		}
		current, readErr := l.owner.Linear.authority.LinearWork(ctx, id)
		if ctx.Err() != nil {
			return
		}
		if readErr != nil {
			l.skipped(id, readErr)
			return
		}
		l.fail(ctx, entry, current, startFault(e))
		if !waitLinear(ctx, time.Duration(1<<min(attempt, 3))*time.Second) {
			return
		}
	}
	buffer, e := linearbuffer.New(linearbuffer.Options{Parent: l.config.CacheDirectory, OriginMS: time.Now().UnixMilli(), MaxBytes: l.config.MaxBytes, Retention: l.config.Retention, Scheduled: work.Selection.Reference.Kind == "library-channel"})
	if e != nil {
		l.report(ctx, entry, work, "timeshift_storage_unavailable", "delivery", e)
		return
	}
	var secret [32]byte
	if _, e = rand.Read(secret[:]); e != nil {
		_ = buffer.Close()
		return
	}
	entry.mu.Lock()
	entry.buffer = buffer
	entry.generation = work.BufferOrdinal
	entry.token = hex.EncodeToString(secret[:])
	entry.selection = work.Selection
	entry.mu.Unlock()
	attempts := 0
	retryRevision := work.RetryRevision
	selectionKey := linearSelectionKey(work.Selection)
	initialPolicy, initialLease, initialAuthority := work.PolicyRevision, work.LeaseGeneration, work.LeaseAuthorityRevision
	for ctx.Err() == nil {
		changed, refreshErr := l.owner.Linear.authority.RefreshLinearSelection(ctx, id)
		if changed {
			l.owner.Wake()
		}
		work, e = l.owner.Linear.authority.LinearWork(ctx, id)
		if e != nil {
			return
		}
		if work.PolicyRevision != initialPolicy || work.LeaseGeneration != initialLease || work.LeaseAuthorityRevision != initialAuthority {
			return
		}
		entry.mu.Lock()
		entry.selection = work.Selection
		entry.mu.Unlock()
		if key := linearSelectionKey(work.Selection); key != selectionKey {
			selectionKey = key
			attempts = 0
		}
		if work.RetryRevision != retryRevision {
			attempts = 0
			retryRevision = work.RetryRevision
			_ = l.owner.Linear.authority.RestartLinearSource(ctx, id)
			l.owner.Wake()
			continue
		}

		if refreshErr != nil {
			l.fail(ctx, entry, work, refreshErr)
			if !waitLinear(ctx, time.Second) {
				return
			}
			continue
		}
		if attempts >= 5 {
			if !waitLinear(ctx, time.Second) {
				return
			}
			continue
		}
		work, e = l.owner.Linear.authority.BeginLinearProducer(ctx, id, work.SourceRevision, work.BufferOrdinal)
		if e != nil {
			// A source that changed underneath is a quick retry; anything else is a
			// reported failure with backoff.
			var fault *playback.ControlFault
			delay := 250 * time.Millisecond
			if !errors.As(e, &fault) || fault.Code != "source_changed" {
				attempts++
				if current, readErr := l.owner.Linear.authority.LinearWork(ctx, id); readErr == nil {
					l.fail(ctx, entry, current, startFault(e))
				}
				delay = time.Duration(1<<min(attempts-1, 3)) * time.Second
			}
			if !waitLinear(ctx, delay) {
				return
			}
			continue
		}
		entry.mu.Lock()
		entry.producer = work.ProducerOrdinal
		entry.selections[work.ProducerOrdinal] = work.Selection
		entry.state = "preparing"
		entry.code = ""
		entry.mu.Unlock()
		e = l.produceMonitored(ctx, work, entry)
		if ctx.Err() != nil {
			return
		}
		current, authorityErr := l.owner.Linear.authority.LinearWork(ctx, id)
		if authorityErr != nil {
			return
		}
		if current.SourceRevision != work.SourceRevision {
			if linearSelectionKey(current.Selection) != selectionKey {
				attempts = 0
				selectionKey = linearSelectionKey(current.Selection)
			}
			continue
		}
		if current.PolicyRevision != initialPolicy || current.LeaseGeneration != initialLease || current.LeaseAuthorityRevision != initialAuthority {
			return
		}
		attempts++
		if e == nil {
			e = channelFault("source_ended", 503)
		}
		l.fail(ctx, entry, work, e)
		if !waitLinear(ctx, time.Duration(1<<min(attempts-1, 3))*time.Second) {
			return
		}
		if e = l.owner.Linear.authority.RestartLinearSource(ctx, id); e != nil {
			continue
		}
		l.owner.Wake()
	}
}
func (l *LinearRuntime) fail(ctx context.Context, e *linearEntry, w playback.LinearWork, err error) {
	code := "source_unavailable"
	var fault *playback.ControlFault
	switch {
	case errors.As(err, &fault):
		code = fault.Code
	case errors.Is(err, livechannels.ErrCapacity):
		code = "source_capacity_unavailable"
	case errors.Is(err, livechannels.ErrReservation), errors.Is(err, livechannels.ErrLease):
		code = "tuner_reservation_changed"
	case errors.Is(err, decoder.ErrConfinementUnavailable):
		code = "decoder_confinement_unavailable"
	case errors.Is(err, linearbuffer.ErrMedia):
		code = "source_timeline_invalid"
	}
	l.report(ctx, e, w, code, "recovery", err)
}

// report puts the occurrence in "recoverable" with a code the client shows, and
// logs the cause once per occurrence and code.
func (l *LinearRuntime) report(ctx context.Context, e *linearEntry, w playback.LinearWork, code, stage string, cause error) {
	e.mu.Lock()
	if e.state == "recoverable" && e.code == code {
		e.mu.Unlock()
		return // already said, and already recorded
	}
	e.state = "recoverable"
	e.code = code
	first := e.logged != code
	e.logged = code
	e.mu.Unlock()
	if first {
		log.Printf("Channel playback %s can't continue: %s: %s", shortPlaybackID(w.PlaybackID), code, strings.TrimPrefix(fmt.Sprint(cause), code+": "))
	}
	if e := l.owner.Linear.authority.LinearStatus(ctx, w, "recoverable", stage, code); e != nil && ctx.Err() == nil {
		log.Printf("Channel playback %s: its status could not be recorded: %v", shortPlaybackID(w.PlaybackID), e)
	}
}

// skipped logs, once per occurrence and reason, an active channel occurrence
// the runtime can't work on (its lease, controller or authority is gone).
func (l *LinearRuntime) skipped(id string, cause error) {
	code := "channel_start_failed"
	var fault *playback.ControlFault
	if errors.As(cause, &fault) {
		code = fault.Code
	}
	if code == "lease_expired" {
		return // the viewer left; nothing to say
	}
	l.mu.Lock()
	if l.skips == nil {
		l.skips = map[string]string{}
	}
	first := l.skips[id] != code
	if len(l.skips) > 1024 {
		clear(l.skips)
	}
	l.skips[id] = code
	l.mu.Unlock()
	if first {
		log.Printf("Channel playback %s can't start: %s: %v", shortPlaybackID(id), code, cause)
	}
}

// startFault keeps a control fault's own code and names anything else (a
// database or runtime failure) channel_start_failed.
func startFault(e error) error {
	var fault *playback.ControlFault
	if errors.As(e, &fault) {
		return e
	}
	return fmt.Errorf("%w: %w", channelFault("channel_start_failed", 503), e)
}

// linearPreparationPatience is how long a Library Channel waits, without a
// word, for its scheduled title's preparation to be claimed.
const linearPreparationPatience = 15 * time.Second

func shortPlaybackID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
func (l *LinearRuntime) produceMonitored(parent context.Context, w playback.LinearWork, entry *linearEntry) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	done := make(chan struct{})
	supervise.Go("playbackruntime.linear.readiness", func() {
		defer close(done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		ready := false
		check := func(ctx context.Context) (bool, error) {
			changed, e := l.owner.Linear.authority.RefreshLinearSelection(ctx, w.PlaybackID)
			if changed {
				l.owner.Wake()
			}
			if e == nil && !changed {
				e = l.owner.Linear.authority.CheckLinearProducer(ctx, w)
			}
			return changed, e
		}
		persisted := false
		markReady := func(ctx context.Context) {
			entry.mu.Lock()
			buffer := entry.buffer
			entry.mu.Unlock()
			if !buffer.Window().Ready {
				return
			}
			if !ready {
				ready = true
				entry.mu.Lock()
				entry.state = "active"
				entry.code = ""
				entry.mu.Unlock()
			}
			// The stored status follows until one write lands: a write refused
			// under load is retried on the next tick, never dropped.
			if !persisted && l.owner.Linear.authority.LinearStatus(ctx, w, "active", "delivery", "") == nil {
				persisted = true
			}
		}
		logf := func(format string, args ...any) {
			log.Printf("Channel playback %s: producer %d "+format, append([]any{shortPlaybackID(w.PlaybackID), w.ProducerOrdinal}, args...)...)
		}
		if linearMonitor(ctx, ticker.C, check, markReady, logf) {
			cancel()
		}
	})
	var e error
	if w.Selection.Reference.Kind == "live-source" {
		e = l.produceLive(ctx, w, entry)
	} else {
		e = l.produceLibraryDirect(ctx, w, entry)
	}
	cancel()
	<-done
	// Retained selections are bounded by the actual rolling media, not total uptime.
	keep := map[int64]bool{}
	for _, n := range entry.buffer.Producers() {
		keep[n] = true
	}
	entry.mu.Lock()
	for n := range entry.selections {
		if !keep[n] && n != entry.producer {
			delete(entry.selections, n)
		}
	}
	entry.mu.Unlock()
	return e
}

// linearMonitor is a running producer's readiness monitor. At every tick it
// runs check (follow the schedule and the source, then confirm this producer
// is still the current one) and, while the producer stands, markReady. It
// returns true when it decided to stop the producer, having logged why;
// false when ctx ended first.
func linearMonitor(ctx context.Context, ticks <-chan time.Time, check func(context.Context) (bool, error), markReady func(context.Context), logf func(string, ...any)) bool {
	transient := 0
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticks:
			changed, e := check(ctx)
			stop, count := linearMonitorVerdict(changed, e, transient)
			transient = count
			if stop {
				if ctx.Err() != nil {
					return false
				}
				switch {
				case changed:
					logf("stopped for a retune")
				case transient >= linearMonitorPatience || count >= linearMonitorPatience:
					logf("stopped after %d failed checks in a row: %v", linearMonitorPatience, e)
				default:
					logf("stopped by its monitor: %v", e)
				}
				return true
			}
			// Readiness is the buffer's, not the check's: a check that failed
			// for infrastructure (a busy database under load) says nothing
			// about the bytes the viewer can already play, so the producer is
			// still marked ready from its buffer. Only a stop (above) withholds it.
			markReady(ctx)
		}
	}
}

// linearMonitorVerdict decides whether a running producer's readiness monitor
// stops it. A new selection (a retune) or an authority refusal stops it at once:
// the producer is no longer the right one or the viewer may no longer watch. An
// infrastructure error (a busy database, a slow read under a scan) is not a
// verdict on the stream: the producer keeps running and the check is retried on
// the next tick, up to linearMonitorPatience ticks in a row (NEW-28: on the demo
// a check that failed once under scan load killed a healthy first producer, and
// the viewer saw "The channel's source isn't responding").
func linearMonitorVerdict(changed bool, e error, transient int) (stop bool, count int) {
	if changed {
		return true, 0
	}
	if e == nil {
		return false, 0
	}
	var fault *playback.ControlFault
	if errors.As(e, &fault) || errors.Is(e, identity.ErrUnauthorized) || errors.Is(e, identity.ErrNotVisible) || errors.Is(e, identity.ErrContentRestricted) || errors.Is(e, identity.ErrForbidden) {
		return true, 0
	}
	transient++
	return transient >= linearMonitorPatience, transient
}

// linearMonitorPatience is how many consecutive monitor ticks (500 ms each) may
// fail for infrastructure reasons before the producer is stopped anyway.
const linearMonitorPatience = 10

func (l *LinearRuntime) produceLive(ctx context.Context, w playback.LinearWork, entry *linearEntry) (result error) {
	var allocation livechannels.Allocation
	var input livechannels.Input
	e := l.owner.Linear.authority.WithLinearWriteTx(ctx, w.PlaybackID, func(tx *sql.Tx, current playback.LinearWork) error {
		if current.SourceRevision != w.SourceRevision || current.ProducerOrdinal != w.ProducerOrdinal {
			return channelFault("source_changed", 409)
		}
		var e error
		allocation, e = livechannels.AcquireTx(ctx, tx, livechannels.Allocation{SourceID: w.Selection.Reference.SourceID, ChannelID: w.Selection.Reference.ChannelID, ResourceID: w.PlaybackID, Kind: "live", Owner: livechannels.Owner{Authority: w.Principal.Authority, AccountID: w.Principal.AccountID, ProfileID: w.Principal.ProfileID}}, time.Now())
		if e != nil {
			return e
		}
		input, e = l.config.Live.InputTx(ctx, tx, allocation.SourceID, allocation.ChannelID, w.Selection.Reference.Generation)
		return e
	})
	if e != nil {
		return e
	}
	// Allocation is committed before physical acquisition; every exit closes the
	// decoder and gateway before releasing its exact token/generation.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		gated, e := dbwork.Begin(cleanup, l.config.DB, dbwork.ClassEstablishedPlayback)
		tx := gated.Tx()
		if e == nil {
			e = livechannels.ReleaseAllocationTx(cleanup, tx, allocation)
			if e == nil {
				e = gated.Commit()
			} else {
				_ = gated.Rollback()
			}
		}
		result = errors.Join(result, e)
	}()
	lock, e := l.locks.Lock(allocation)
	if e != nil {
		return e
	}
	defer lock.Close()
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	renewed := make(chan struct{})
	supervise.Go("playbackruntime.linear.renewal", func() {
		defer close(renewed)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-ticker.C:
				e := l.owner.Linear.authority.WithLinearWriteTx(work, w.PlaybackID, func(tx *sql.Tx, current playback.LinearWork) error {
					if current.SourceRevision != w.SourceRevision || current.ProducerOrdinal != w.ProducerOrdinal {
						return channelFault("source_changed", 409)
					}
					_, e := livechannels.RenewAllocationTx(work, tx, allocation, time.Now())
					return e
				})
				if e != nil {
					cancel()
					return
				}
			}
		}
	})
	defer func() { cancel(); <-renewed }()
	gateway, e := linearinput.Open(work, input)
	if e != nil {
		return e
	}
	defer gateway.Close()
	reservation, e := gateway.Reserve()
	if e != nil {
		return e
	}
	probeCtx, probeCancel := context.WithTimeout(work, 20*time.Second)
	raw, e := decoder.LinearProbe(probeCtx, l.owner.probeSupervisor, w.PlaybackID+"-live-probe", l.config.FFprobe, gateway.URL(), gateway.HLS(), reservation, lock, l.config.DecoderLibraries...)
	probeCancel()
	if e != nil {
		return e
	}
	plan, e := linearPlan(raw, w)
	if e != nil {
		return e
	}
	plan.HLSInput = gateway.HLS()
	publisher, e := entry.buffer.Publisher(linearbuffer.Producer{ID: w.ProducerOrdinal}, func() error { return l.owner.Linear.authority.CheckLinearProducer(work, w) })
	if e != nil {
		return e
	}
	defer func() { _ = gateway.Close(); _ = publisher.Close() }()
	supervise.Go("playbackruntime.linear.publisher-watch", func() {
		select {
		case <-publisher.Failed():
			cancel()
		case <-work.Done():
		}
	})
	output, e := gateway.AttachOutput(publisher)
	if e != nil {
		return e
	}
	reservation, e = gateway.Reserve()
	if e != nil {
		return e
	}
	result = decoder.RunLinearHLS(work, l.owner.probeSupervisor, w.PlaybackID+"-"+strconv.FormatInt(w.ProducerOrdinal, 10), l.config.FFmpeg, gateway.URL(), output, plan, reservation, lock, l.config.DecoderLibraries...)
	if result == nil {
		finish, done := context.WithTimeout(work, 5*time.Second)
		result = publisher.Finish(finish)
		done()
	}
	_ = gateway.Close()
	return errors.Join(result, publisher.Result())
}
func privateListeners() (*net.TCPListener, *net.TCPListener, error) {
	v4, e := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		return nil, nil, e
	}
	v6, e := net.ListenTCP("tcp6", &net.TCPAddr{IP: net.IPv6loopback, Port: v4.Addr().(*net.TCPAddr).Port})
	if e != nil {
		v4.Close()
		return nil, nil, e
	}
	return v4, v6, nil
}
func (l *LinearRuntime) produceLibrary(ctx context.Context, w playback.LinearWork, entry *linearEntry, borrow producerinput.BorrowedInput) (result error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	v4, v6, e := privateListeners()
	if e != nil {
		return e
	}
	bridge, e := producerinput.NewPair(ctx, borrow, l.config.CacheDirectory, v4, v6)
	if e != nil {
		return e
	}
	defer func() { result = errors.Join(result, bridge.Shutdown(context.Background())) }()
	reservation, e := decoder.ReserveEndpoints(v4, v6)
	if e != nil {
		return e
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 20*time.Second)
	raw, e := decoder.LinearProbe(probeCtx, l.owner.probeSupervisor, w.PlaybackID+"-library-probe", l.config.FFprobe, bridge.URL(), false, reservation, nil, l.config.DecoderLibraries...)
	probeCancel()
	if e != nil {
		return e
	}
	plan, e := linearPlan(raw, w)
	if e != nil {
		return e
	}
	now := time.Now().UnixMilli()
	if now >= w.Selection.EntryEndMS {
		return channelFault("programme_boundary", 409)
	}
	plan.Finite = true
	plan.SeekUS = max(int64(0), w.Selection.SourceOffsetMS+now-w.Selection.EntryStartMS) * 1000
	plan.ClipEndUS = (w.Selection.SourceOffsetMS + w.Selection.EntryEndMS - w.Selection.EntryStartMS) * 1000
	publisher, e := entry.buffer.Publisher(linearbuffer.Producer{ID: w.ProducerOrdinal, Scheduled: true, TimelineBaseMS: w.Selection.EntryStartMS - w.Selection.SourceOffsetMS, EndMS: w.Selection.EntryEndMS}, func() error {
		if e := l.owner.Linear.authority.CheckLinearProducer(ctx, w); e != nil {
			return e
		}
		_, e := borrow.Validate(ctx)
		return e
	})
	if e != nil {
		return e
	}
	defer func() { _ = bridge.Shutdown(context.Background()); _ = publisher.Close() }()
	supervise.Go("playbackruntime.linear.bridge-watch", func() {
		select {
		case <-publisher.Failed():
			cancel()
		case <-ctx.Done():
		}
	})
	output, e := bridge.AttachOutput(publisher)
	if e != nil {
		return e
	}
	reservation, e = decoder.ReserveEndpoints(v4, v6)
	if e != nil {
		return e
	}
	e = decoder.RunLinearHLS(ctx, l.owner.probeSupervisor, w.PlaybackID+"-"+strconv.FormatInt(w.ProducerOrdinal, 10), l.config.FFmpeg, bridge.URL(), output, plan, reservation, nil, l.config.DecoderLibraries...)
	if e == nil {
		finish, done := context.WithTimeout(ctx, 5*time.Second)
		e = publisher.Finish(finish)
		done()
	}
	e = errors.Join(e, bridge.Shutdown(context.Background()), publisher.Result())
	if e != nil {
		return e
	}
	// Read-rate burst intentionally prepares ahead. Do not transition merely
	// because FFmpeg reached the planned end before its scheduled wall boundary.
	if !waitLinear(ctx, time.Until(time.UnixMilli(w.Selection.EntryEndMS))) {
		return ctx.Err()
	}
	return nil
}
func (l *LinearRuntime) entry(id string) *linearEntry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.entries[id]
}
func (l *LinearRuntime) Snapshot(id string) playback.LinearMedia {
	result := playback.LinearMedia{Gaps: []playback.LinearInterval{}, State: "preparing"}
	e := l.entry(id)
	if e == nil {
		return result
	}
	e.mu.Lock()
	buffer, generation, producer, token, state, code, logo := e.buffer, e.generation, e.producer, e.token, e.state, e.code, e.selection.LogoItemID
	e.mu.Unlock()
	if buffer == nil {
		return result
	}
	w := buffer.Window()
	result.BufferGeneration = strconv.FormatInt(generation, 10)
	if producer > 0 {
		result.PresentationGeneration = strconv.FormatInt(generation, 10)
	}
	result.OriginMS = w.OriginMS
	result.WindowRevision = strconv.FormatInt(w.Revision, 10)
	result.WindowStartUS = strconv.FormatInt(w.StartUS, 10)
	result.WindowEndUS = strconv.FormatInt(w.EndUS, 10)
	result.LiveEdgeUS = strconv.FormatInt(w.LiveUS, 10)
	result.State = state
	result.ErrorCode = code
	if w.Ready {
		result.StreamURL = fmt.Sprintf("/v1/media/linear/%s/%d/%s/index.m3u8", id, generation, token)
	}
	if logo != "" {
		result.LogoURL = fmt.Sprintf("/v1/media/linear/%s/%d/%s/logo", id, generation, token)
	}
	for _, gap := range w.Gaps {
		result.Gaps = append(result.Gaps, playback.LinearInterval{StartUS: strconv.FormatInt(gap.StartUS, 10), EndUS: strconv.FormatInt(gap.EndUS, 10)})
	}
	return result
}
func (l *LinearRuntime) ResolveSeek(id, generation, kind string, raw *string) (string, error) {
	e := l.entry(id)
	if e == nil {
		return "", channelFault("window_expired", 410)
	}
	e.mu.Lock()
	buffer, gen := e.buffer, e.generation
	e.mu.Unlock()
	if buffer == nil || generation != strconv.FormatInt(gen, 10) {
		return "", channelFault("window_expired", 410)
	}
	var position *int64
	if kind == "position" {
		if raw == nil {
			return "", channelFault("invalid_seek", 400)
		}
		v, err := strconv.ParseInt(*raw, 10, 64)
		if err != nil || strconv.FormatInt(v, 10) != *raw {
			return "", channelFault("invalid_seek", 400)
		}
		position = &v
	} else if kind != "live" {
		return "", channelFault("invalid_seek", 400)
	}
	target, err := buffer.Resolve(position)
	if errors.Is(err, linearbuffer.ErrGap) {
		return "", channelFault("media_gap", 409)
	}
	if err != nil {
		return "", channelFault("window_expired", 410)
	}
	return strconv.FormatInt(target, 10), nil
}
func (l *LinearRuntime) SelectionAt(id, generation, position string) (*playback.LinearSelection, error) {
	e := l.entry(id)
	if e == nil {
		return nil, channelFault("window_expired", 410)
	}
	n, err := strconv.ParseInt(position, 10, 64)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	b, gen := e.buffer, e.generation
	e.mu.Unlock()
	if b == nil || generation != strconv.FormatInt(gen, 10) {
		return nil, channelFault("window_expired", 410)
	}
	producer, err := b.ProducerAt(n)
	if err != nil {
		return nil, channelFault("window_expired", 410)
	}
	e.mu.Lock()
	selection, ok := e.selections[producer]
	e.mu.Unlock()
	if !ok {
		return nil, channelFault("window_expired", 410)
	}
	return &selection, nil
}
func (l *LinearRuntime) mediaEntry(id, generation, token string) (*linearEntry, error) {
	e := l.entry(id)
	if e == nil {
		return nil, channelFault("media_generation_expired", 410)
	}
	e.mu.Lock()
	valid := e.buffer != nil && generation == strconv.FormatInt(e.generation, 10) && subtle.ConstantTimeCompare([]byte(token), []byte(e.token)) == 1
	e.mu.Unlock()
	if !valid {
		return nil, identity.ErrUnauthorized
	}
	return e, nil
}

// MediaGrant rechecks durable family/lease authority; URL possession alone never
// survives sign-out, permission revocation or buffer generation replacement.
func (l *LinearRuntime) MediaGrant(ctx context.Context, id, generation, token string) (identity.Principal, playback.LinearSelection, error) {
	e, err := l.mediaEntry(id, generation, token)
	if err != nil {
		return identity.Principal{}, playback.LinearSelection{}, err
	}
	e.mu.Lock()
	gen, selection := e.generation, e.selection
	e.mu.Unlock()
	p, err := l.owner.Linear.authority.AuthorizeLinearMedia(ctx, id, gen, &selection)
	return p, selection, err
}
func (l *LinearRuntime) ServeMedia(w http.ResponseWriter, r *http.Request, id, generation, token, name string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	e, err := l.mediaEntry(id, generation, token)
	if err != nil {
		http.Error(w, "media expired", 410)
		return
	}
	select {
	case e.requests <- struct{}{}:
		defer func() { <-e.requests }()
	default:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "media busy", 429)
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	e.mu.Lock()
	buffer, gen := e.buffer, e.generation
	e.mu.Unlock()
	authorize := func(selection *playback.LinearSelection) error {
		_, err := l.owner.Linear.authority.AuthorizeLinearMedia(r.Context(), id, gen, selection)
		return err
	}
	if err = authorize(nil); err != nil {
		http.Error(w, "media authority expired", 403)
		return
	}
	if name == "index.m3u8" {
		// Playlist access checks every retained programme as well as the current one.
		for _, producer := range buffer.Producers() {
			e.mu.Lock()
			selection, ok := e.selections[producer]
			e.mu.Unlock()
			if !ok || authorize(&selection) != nil {
				http.Error(w, "retained media unavailable", 403)
				return
			}
		}
		data, err := buffer.Playlist()
		if err != nil {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "media preparing", 503)
			return
		}
		if authorize(nil) != nil {
			http.Error(w, "media authority expired", 403)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method == "GET" {
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Second))
			_, _ = w.Write(data)
		}
		return
	}
	reader, err := buffer.Open(name)
	if err != nil {
		http.Error(w, "timeshift window expired", 410)
		return
	}
	defer reader.Close()
	e.mu.Lock()
	selection, ok := e.selections[reader.Producer]
	e.mu.Unlock()
	if !ok || authorize(&selection) != nil {
		http.Error(w, "media authority expired", 403)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Content-Length", strconv.FormatInt(reader.Size, 10))
	if r.Method == "HEAD" {
		return
	}
	// HLS uses complete immutable objects. Each disk read is fenced again before
	// emission; no ServeFile fallback can bypass current authorization.
	chunk := make([]byte, 64<<10)
	for {
		if authorize(&selection) != nil {
			return
		}
		n, err := reader.File.Read(chunk)
		if n > 0 {
			if authorize(&selection) != nil {
				return
			}
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Second))
			if _, e := w.Write(chunk[:n]); e != nil {
				return
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return
			}
			break
		}
	}
}

func linearSelectionKey(s playback.LinearSelection) string {
	key := s.Reference.Kind + ":" + s.Reference.Generation + ":" + s.SourceFence
	if s.Reference.Kind == "library-channel" {
		key += ":" + s.EntryID
	}
	return key
}
