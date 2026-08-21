// Package ffmpegsupervisor owns the lifecycle of FFmpeg process generations.
// It deliberately knows nothing about HLS, DVR, or optimized-version storage;
// those adapters communicate progress and consumer activity through this API.
package ffmpegsupervisor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	ErrClosed   = errors.New("ffmpeg supervisor is closed")
	ErrNotFound = errors.New("ffmpeg generation not found")
)

type Mode string

const (
	ModeVOD  Mode = "vod"
	ModeLive Mode = "live"
)

// LaunchSpec is intentionally write-only from the supervisor's perspective.
// It is passed to the launcher but is never retained or exposed in snapshots.
type LaunchSpec struct {
	Executable string
	Args       []string
	Env        []string
	Dir        string
}

type Process interface {
	Wait() error
	Interrupt() error
	Kill() error
}

type Launcher interface {
	Start(context.Context, LaunchSpec) (Process, error)
}

type ReleaseFunc func(Release)

type Release struct {
	Key        string
	Generation uint64
	Err        error
}

type Config struct {
	InterruptGrace  time.Duration
	VODAbandonment  time.Duration
	LiveAbandonment time.Duration
	StallAfter      time.Duration
	SnapshotLimit   int
}

type StartOptions struct {
	Key     string
	Mode    Mode
	Command LaunchSpec
	Release ReleaseFunc
}

type Snapshot struct {
	Key             string
	Generation      uint64
	Mode            Mode
	StartedAt       time.Time
	LastProgressAt  time.Time
	LastActivityAt  time.Time
	Clients         int
	Stopping        bool
	Stalled         bool
	Discontinuity   bool
	ManifestVersion uint64
	LatestSegment   uint64
}

type Supervisor struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	launcher Launcher
	cfg      Config
	closed   bool
	next     map[string]uint64
	starting map[string]*startAttempt
	active   map[string]*generation
	all      map[generationID]*generation
	wg       sync.WaitGroup
}

// startAttempt reserves a process key while the launcher performs potentially
// slow external work. Callers that arrive during that interval join the same
// start result instead of launching a second child for the same key.
type startAttempt struct {
	done     chan struct{}
	snapshot Snapshot
	err      error
}

type generationID struct {
	key string
	n   uint64
}

type generation struct {
	id                               generationID
	mode                             Mode
	process                          Process
	cancel                           context.CancelFunc
	release                          ReleaseFunc
	releaseOnce                      sync.Once
	stopOnce                         sync.Once
	done                             chan struct{}
	changed                          chan struct{}
	started, progress, activity      time.Time
	clients                          int
	stopping, stalled, discontinuity bool
	manifest, segment                uint64
	err                              error
	abandonTimer                     *time.Timer
}

func New(parent context.Context, launcher Launcher, cfg Config) *Supervisor {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	if cfg.InterruptGrace <= 0 {
		cfg.InterruptGrace = 5 * time.Second
	}
	if cfg.SnapshotLimit <= 0 {
		cfg.SnapshotLimit = 100
	}
	s := &Supervisor{ctx: ctx, cancel: cancel, launcher: launcher, cfg: cfg, next: map[string]uint64{}, starting: map[string]*startAttempt{}, active: map[string]*generation{}, all: map[generationID]*generation{}}
	s.wg.Add(1)
	go func() { defer s.wg.Done(); <-ctx.Done(); s.stopAll() }()
	return s
}

func (s *Supervisor) Start(opts StartOptions) (Snapshot, error) {
	if opts.Key == "" {
		return Snapshot{}, errors.New("ffmpeg generation key is required")
	}
	if opts.Mode != ModeVOD && opts.Mode != ModeLive {
		return Snapshot{}, fmt.Errorf("unsupported ffmpeg mode %q", opts.Mode)
	}
	now := time.Now()
	s.mu.Lock()
	if s.closed || s.ctx.Err() != nil {
		s.mu.Unlock()
		return Snapshot{}, ErrClosed
	}
	if attempt := s.starting[opts.Key]; attempt != nil {
		done := attempt.done
		s.mu.Unlock()
		select {
		case <-done:
			return attempt.snapshot, attempt.err
		case <-s.ctx.Done():
			return Snapshot{}, ErrClosed
		}
	}
	attempt := &startAttempt{done: make(chan struct{})}
	s.starting[opts.Key] = attempt
	prior := s.active[opts.Key]
	n := s.next[opts.Key] + 1
	s.next[opts.Key] = n
	procCtx, cancel := context.WithCancel(s.ctx)
	// Account for launches before dropping the lock. Shutdown sets closed while
	// holding the same lock, so Wait can never race a later Add.
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()

	p, err := s.launcher.Start(procCtx, opts.Command)
	if err != nil {
		cancel()
		s.mu.Lock()
		s.finishStartLocked(opts.Key, attempt, Snapshot{}, err)
		s.mu.Unlock()
		if opts.Release != nil {
			opts.Release(Release{Key: opts.Key, Generation: n, Err: err})
		}
		return Snapshot{}, err
	}
	g := &generation{id: generationID{opts.Key, n}, mode: opts.Mode, process: p, cancel: cancel, release: opts.Release, done: make(chan struct{}), changed: make(chan struct{}), started: now, progress: now, activity: now, discontinuity: prior != nil}
	s.mu.Lock()
	if s.closed || s.ctx.Err() != nil {
		s.mu.Unlock()
		_ = p.Interrupt()
		_ = p.Kill()
		err := p.Wait()
		g.releaseOnce.Do(func() {
			if g.release != nil {
				g.release(Release{Key: g.id.key, Generation: g.id.n, Err: err})
			}
		})
		cancel()
		s.mu.Lock()
		s.finishStartLocked(opts.Key, attempt, Snapshot{}, ErrClosed)
		s.mu.Unlock()
		return Snapshot{}, ErrClosed
	}
	s.active[opts.Key] = g
	s.all[g.id] = g
	s.scheduleAbandonmentLocked(g, now)
	s.wg.Add(1)
	go s.wait(g)
	started := snapshot(g)
	s.finishStartLocked(opts.Key, attempt, started, nil)
	s.mu.Unlock()
	if prior != nil {
		prior.requestStop(s.cfg.InterruptGrace)
	}
	return started, nil
}

func (s *Supervisor) finishStartLocked(key string, attempt *startAttempt, result Snapshot, err error) {
	if s.starting[key] != attempt {
		return
	}
	delete(s.starting, key)
	attempt.snapshot = result
	attempt.err = err
	close(attempt.done)
}

func (s *Supervisor) wait(g *generation) {
	defer s.wg.Done()
	err := g.process.Wait()
	s.mu.Lock()
	g.err = err
	if g.abandonTimer != nil {
		g.abandonTimer.Stop()
		g.abandonTimer = nil
	}
	if s.active[g.id.key] == g {
		delete(s.active, g.id.key)
	}
	delete(s.all, g.id)
	close(g.done)
	signal(g)
	s.mu.Unlock()
	g.cancel()
	g.releaseOnce.Do(func() {
		if g.release != nil {
			g.release(Release{Key: g.id.key, Generation: g.id.n, Err: err})
		}
	})
}

func (g *generation) requestStop(grace time.Duration) {
	g.stopOnce.Do(func() {
		_ = g.process.Interrupt()
		go func() {
			t := time.NewTimer(grace)
			defer t.Stop()
			select {
			case <-g.done:
			case <-t.C:
				_ = g.process.Kill()
			}
		}()
	})
}

func (s *Supervisor) Stop(key string, generation uint64) error {
	s.mu.Lock()
	g := s.all[generationID{key, generation}]
	if g != nil && !g.stopping {
		g.stopping = true
		signal(g)
	}
	s.mu.Unlock()
	if g == nil {
		return ErrNotFound
	}
	g.requestStop(s.cfg.InterruptGrace)
	return nil
}

func (s *Supervisor) Progress(key string, generationNumber uint64, at time.Time) error {
	return s.mutate(key, generationNumber, func(g *generation) {
		if at.IsZero() {
			at = time.Now()
		}
		g.progress = at
		g.stalled = false
	})
}

// ClientActivity changes the number of attached consumers and refreshes the
// abandonment clock. Delta may be positive or negative; the count never drops below zero.
func (s *Supervisor) ClientActivity(key string, generationNumber uint64, delta int, at time.Time) error {
	return s.mutate(key, generationNumber, func(g *generation) {
		if at.IsZero() {
			at = time.Now()
		}
		g.clients += delta
		if g.clients < 0 {
			g.clients = 0
		}
		g.activity = at
		s.scheduleAbandonmentLocked(g, at)
	})
}

func (s *Supervisor) abandonmentFor(g *generation) time.Duration {
	if g.mode == ModeLive {
		return s.cfg.LiveAbandonment
	}
	return s.cfg.VODAbandonment
}

// scheduleAbandonmentLocked gives each generation ownership of its own idle
// deadline. A generation with readers has no timer; the final detach starts a
// fresh full deadline. This makes reaping independent of HTTP traffic and of
// any application-level sweep loop.
func (s *Supervisor) scheduleAbandonmentLocked(g *generation, now time.Time) {
	if g.abandonTimer != nil {
		g.abandonTimer.Stop()
		g.abandonTimer = nil
	}
	deadline := s.abandonmentFor(g)
	if deadline <= 0 || g.clients != 0 || g.stopping {
		return
	}
	remaining := deadline - now.Sub(g.activity)
	if remaining < 0 {
		remaining = 0
	}
	id := g.id
	g.abandonTimer = time.AfterFunc(remaining, func() { s.abandon(id) })
}

func (s *Supervisor) abandon(id generationID) {
	var stop *generation
	now := time.Now()
	s.mu.Lock()
	g := s.all[id]
	if g != nil {
		g.abandonTimer = nil
		deadline := s.abandonmentFor(g)
		if deadline > 0 && g.clients == 0 && !g.stopping && now.Sub(g.activity) >= deadline {
			g.stopping = true
			signal(g)
			stop = g
		} else {
			s.scheduleAbandonmentLocked(g, now)
		}
	}
	s.mu.Unlock()
	if stop != nil {
		stop.requestStop(s.cfg.InterruptGrace)
	}
}

func (s *Supervisor) ManifestReady(key string, generationNumber, version uint64) error {
	return s.mutate(key, generationNumber, func(g *generation) {
		if version > g.manifest {
			g.manifest = version
		}
	})
}

func (s *Supervisor) SegmentReady(key string, generationNumber, segment uint64) error {
	return s.mutate(key, generationNumber, func(g *generation) {
		if segment > g.segment {
			g.segment = segment
		}
	})
}

func (s *Supervisor) mutate(key string, n uint64, fn func(*generation)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.all[generationID{key, n}]
	if g == nil {
		return ErrNotFound
	}
	fn(g)
	signal(g)
	return nil
}

func signal(g *generation) { close(g.changed); g.changed = make(chan struct{}) }

func (s *Supervisor) WaitManifest(ctx context.Context, key string, generationNumber, minimum uint64) error {
	return s.waitValue(ctx, key, generationNumber, func(g *generation) bool { return g.manifest >= minimum })
}

func (s *Supervisor) WaitSegment(ctx context.Context, key string, generationNumber, minimum uint64) error {
	return s.waitValue(ctx, key, generationNumber, func(g *generation) bool { return g.segment >= minimum })
}

func (s *Supervisor) waitValue(ctx context.Context, key string, n uint64, ready func(*generation) bool) error {
	for {
		s.mu.Lock()
		g := s.all[generationID{key, n}]
		if g == nil {
			s.mu.Unlock()
			return ErrNotFound
		}
		if ready(g) {
			s.mu.Unlock()
			return nil
		}
		changed, done := g.changed, g.done
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return ErrNotFound
		case <-changed:
		}
	}
}

// Sweep evaluates time-based policies. Callers can drive it from their own
// scheduler, which also makes policy decisions deterministic in tests.
func (s *Supervisor) Sweep(now time.Time) {
	var stop []*generation
	s.mu.Lock()
	for _, g := range s.all {
		if s.cfg.StallAfter > 0 && now.Sub(g.progress) >= s.cfg.StallAfter && !g.stalled {
			g.stalled = true
			signal(g)
		}
		deadline := s.abandonmentFor(g)
		if deadline > 0 && g.clients == 0 && now.Sub(g.activity) >= deadline && !g.stopping {
			g.stopping = true
			if g.abandonTimer != nil {
				g.abandonTimer.Stop()
				g.abandonTimer = nil
			}
			signal(g)
			stop = append(stop, g)
		}
	}
	s.mu.Unlock()
	for _, g := range stop {
		g.requestStop(s.cfg.InterruptGrace)
	}
}

func (s *Supervisor) Snapshots() []Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Snapshot, 0, len(s.active))
	for _, g := range s.active {
		out = append(out, snapshot(g))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].Generation < out[j].Generation
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > s.cfg.SnapshotLimit {
		out = out[:s.cfg.SnapshotLimit]
	}
	return out
}

func snapshot(g *generation) Snapshot {
	return Snapshot{Key: g.id.key, Generation: g.id.n, Mode: g.mode, StartedAt: g.started, LastProgressAt: g.progress, LastActivityAt: g.activity, Clients: g.clients, Stopping: g.stopping, Stalled: g.stalled, Discontinuity: g.discontinuity, ManifestVersion: g.manifest, LatestSegment: g.segment}
}

func (s *Supervisor) stopAll() {
	s.mu.Lock()
	all := make([]*generation, 0, len(s.all))
	for _, g := range s.all {
		if g.abandonTimer != nil {
			g.abandonTimer.Stop()
			g.abandonTimer = nil
		}
		if !g.stopping {
			g.stopping = true
			signal(g)
		}
		all = append(all, g)
	}
	s.mu.Unlock()
	for _, g := range all {
		g.requestStop(s.cfg.InterruptGrace)
	}
}

// Shutdown prevents launches, stops every generation, and joins every waiter.
func (s *Supervisor) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.cancel()
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
