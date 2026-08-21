package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/ffmpegsupervisor"
)

// transcodeWorkKindV2 distinguishes otherwise identical playback and durable
// optimization keys. The prefix is part of the supervisor identity so a media
// playback cannot accidentally supersede an optimization for the same item.
type transcodeWorkKindV2 string

const (
	transcodeWorkPlaybackV2     transcodeWorkKindV2 = "playback"
	transcodeWorkOptimizationV2 transcodeWorkKindV2 = "optimization"
)

// transcodeProcessFactoryV2 builds and starts a process using the exact
// generation context owned by ffmpegsupervisor. This preserves caller-specific
// stdin/stdout/stderr wiring without letting an exec.Cmd outlive shutdown.
type transcodeProcessFactoryV2 func(context.Context) (ffmpegsupervisor.Process, error)

// supervisedExecFactoryV2 adapts an exec.Cmd builder to the supervisor's
// single-owner Wait contract. The builder may attach pipes and writers, but it
// must not start or wait for the command itself.
func supervisedExecFactoryV2(build func(context.Context) (*exec.Cmd, error)) transcodeProcessFactoryV2 {
	return func(ctx context.Context) (ffmpegsupervisor.Process, error) {
		if build == nil {
			return nil, errors.New("ffmpeg command builder is required")
		}
		cmd, err := build(ctx)
		if err != nil {
			return nil, err
		}
		if cmd == nil {
			return nil, errors.New("ffmpeg command builder returned nil")
		}
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return supervisedExecProcessV2{cmd: cmd}, nil
	}
}

type supervisedExecProcessV2 struct{ cmd *exec.Cmd }

func (p supervisedExecProcessV2) Wait() error { return p.cmd.Wait() }

func (p supervisedExecProcessV2) Interrupt() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return p.cmd.Process.Signal(os.Interrupt)
}

func (p supervisedExecProcessV2) Kill() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return p.cmd.Process.Kill()
}

type transcodeLaunchV2 struct {
	Kind    transcodeWorkKindV2
	Key     string
	Mode    ffmpegsupervisor.Mode
	Start   transcodeProcessFactoryV2
	Release ffmpegsupervisor.ReleaseFunc
}

type transcodeGenerationV2 struct {
	Key        string
	Generation uint64
	Kind       transcodeWorkKindV2
}

type transcodeSupervisorV2 struct {
	bridge     *transcodeLauncherBridgeV2
	supervisor *ffmpegsupervisor.Supervisor
}

func newTranscodeSupervisorV2(parent context.Context, cfg ffmpegsupervisor.Config) *transcodeSupervisorV2 {
	bridge := &transcodeLauncherBridgeV2{pending: make(map[string]transcodeProcessFactoryV2)}
	return &transcodeSupervisorV2{
		bridge:     bridge,
		supervisor: ffmpegsupervisor.New(parent, bridge, cfg),
	}
}

func (s *transcodeSupervisorV2) Launch(spec transcodeLaunchV2) (transcodeGenerationV2, error) {
	if s == nil || s.supervisor == nil || s.bridge == nil {
		return transcodeGenerationV2{}, errors.New("ffmpeg supervisor is not configured")
	}
	if spec.Kind != transcodeWorkPlaybackV2 && spec.Kind != transcodeWorkOptimizationV2 {
		return transcodeGenerationV2{}, fmt.Errorf("unsupported ffmpeg work kind %q", spec.Kind)
	}
	if spec.Key == "" {
		return transcodeGenerationV2{}, errors.New("ffmpeg work key is required")
	}
	if spec.Start == nil {
		return transcodeGenerationV2{}, errors.New("ffmpeg process factory is required")
	}
	key := string(spec.Kind) + ":" + spec.Key
	token := s.bridge.register(spec.Start)
	snapshot, err := s.supervisor.Start(ffmpegsupervisor.StartOptions{
		Key:     key,
		Mode:    spec.Mode,
		Command: ffmpegsupervisor.LaunchSpec{Executable: token},
		Release: spec.Release,
	})
	s.bridge.discard(token)
	if err != nil {
		return transcodeGenerationV2{}, err
	}
	result := transcodeGenerationV2{Key: snapshot.Key, Generation: snapshot.Generation, Kind: spec.Kind}
	// Optimization is durable work, not consumer-owned streaming work. Pinning
	// it prevents the VOD abandonment policy from cancelling a valid job.
	if spec.Kind == transcodeWorkOptimizationV2 {
		_ = s.supervisor.ClientActivity(result.Key, result.Generation, 1, time.Now())
	}
	return result, nil
}

func (s *transcodeSupervisorV2) Stop(g transcodeGenerationV2) error {
	if s == nil || s.supervisor == nil {
		return ffmpegsupervisor.ErrClosed
	}
	return s.supervisor.Stop(g.Key, g.Generation)
}

func (s *transcodeSupervisorV2) Progress(g transcodeGenerationV2, at time.Time) error {
	return s.supervisor.Progress(g.Key, g.Generation, at)
}

func (s *transcodeSupervisorV2) ClientActivity(g transcodeGenerationV2, delta int, at time.Time) error {
	if g.Kind != transcodeWorkPlaybackV2 {
		return errors.New("client activity is only valid for playback generations")
	}
	return s.supervisor.ClientActivity(g.Key, g.Generation, delta, at)
}

func (s *transcodeSupervisorV2) ManifestReady(g transcodeGenerationV2, version uint64) error {
	return s.supervisor.ManifestReady(g.Key, g.Generation, version)
}

func (s *transcodeSupervisorV2) SegmentReady(g transcodeGenerationV2, segment uint64) error {
	return s.supervisor.SegmentReady(g.Key, g.Generation, segment)
}

func (s *transcodeSupervisorV2) Sweep(now time.Time) {
	if s != nil && s.supervisor != nil {
		s.supervisor.Sweep(now)
	}
}

func (s *Server) runFFmpegSupervisorSweeper(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if s != nil && s.ffmpegSupervisor != nil {
				s.ffmpegSupervisor.Sweep(now.UTC())
			}
		}
	}
}

func (s *transcodeSupervisorV2) Snapshots() []ffmpegsupervisor.Snapshot {
	if s == nil || s.supervisor == nil {
		return nil
	}
	return s.supervisor.Snapshots()
}

// Shutdown closes launch admission, interrupts all generations, escalates to
// kill after the configured grace period, and joins every process waiter.
func (s *transcodeSupervisorV2) Shutdown(ctx context.Context) error {
	if s == nil || s.supervisor == nil {
		return nil
	}
	return s.supervisor.Shutdown(ctx)
}

type transcodeLauncherBridgeV2 struct {
	mu      sync.Mutex
	next    atomic.Uint64
	pending map[string]transcodeProcessFactoryV2
}

func (b *transcodeLauncherBridgeV2) register(factory transcodeProcessFactoryV2) string {
	token := fmt.Sprintf("portico-process-factory-%d", b.next.Add(1))
	b.mu.Lock()
	b.pending[token] = factory
	b.mu.Unlock()
	return token
}

func (b *transcodeLauncherBridgeV2) discard(token string) {
	b.mu.Lock()
	delete(b.pending, token)
	b.mu.Unlock()
}

func (b *transcodeLauncherBridgeV2) Start(ctx context.Context, command ffmpegsupervisor.LaunchSpec) (ffmpegsupervisor.Process, error) {
	b.mu.Lock()
	factory := b.pending[command.Executable]
	delete(b.pending, command.Executable)
	b.mu.Unlock()
	if factory == nil {
		return nil, errors.New("ffmpeg process factory is unavailable")
	}
	return factory(ctx)
}
