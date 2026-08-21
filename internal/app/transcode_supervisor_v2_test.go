package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/ffmpegsupervisor"
)

type fakeTranscodeProcessV2 struct {
	done        chan struct{}
	once        sync.Once
	interrupts  int
	kills       int
	mu          sync.Mutex
	waitStarted chan struct{}
}

func newFakeTranscodeProcessV2() *fakeTranscodeProcessV2 {
	return &fakeTranscodeProcessV2{done: make(chan struct{}), waitStarted: make(chan struct{})}
}

func (p *fakeTranscodeProcessV2) Wait() error {
	p.once.Do(func() { close(p.waitStarted) })
	<-p.done
	return nil
}

func (p *fakeTranscodeProcessV2) Interrupt() error {
	p.mu.Lock()
	p.interrupts++
	p.mu.Unlock()
	return nil
}

func (p *fakeTranscodeProcessV2) Kill() error {
	p.mu.Lock()
	p.kills++
	p.mu.Unlock()
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	return nil
}

func (p *fakeTranscodeProcessV2) counts() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.interrupts, p.kills
}

func TestTranscodeSupervisorV2ShutdownInterruptsKillsAndJoins(t *testing.T) {
	s := newTranscodeSupervisorV2(context.Background(), ffmpegsupervisor.Config{InterruptGrace: 5 * time.Millisecond})
	p := newFakeTranscodeProcessV2()
	if _, err := s.Launch(transcodeLaunchV2{Kind: transcodeWorkPlaybackV2, Key: "movie", Mode: ffmpegsupervisor.ModeVOD, Start: func(context.Context) (ffmpegsupervisor.Process, error) { return p, nil }}); err != nil {
		t.Fatal(err)
	}
	<-p.waitStarted
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	interrupts, kills := p.counts()
	if interrupts != 1 || kills != 1 {
		t.Fatalf("interrupts=%d kills=%d, want 1 each", interrupts, kills)
	}
	if _, err := s.Launch(transcodeLaunchV2{Kind: transcodeWorkPlaybackV2, Key: "late", Mode: ffmpegsupervisor.ModeVOD, Start: func(context.Context) (ffmpegsupervisor.Process, error) { return newFakeTranscodeProcessV2(), nil }}); !errors.Is(err, ffmpegsupervisor.ErrClosed) {
		t.Fatalf("late launch error = %v, want ErrClosed", err)
	}
}

func TestTranscodeSupervisorV2ReplacementAndAbandonment(t *testing.T) {
	s := newTranscodeSupervisorV2(context.Background(), ffmpegsupervisor.Config{InterruptGrace: 5 * time.Millisecond, VODAbandonment: time.Millisecond})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}()
	first, second := newFakeTranscodeProcessV2(), newFakeTranscodeProcessV2()
	g1, err := s.Launch(transcodeLaunchV2{Kind: transcodeWorkPlaybackV2, Key: "same", Mode: ffmpegsupervisor.ModeVOD, Start: func(context.Context) (ffmpegsupervisor.Process, error) { return first, nil }})
	if err != nil {
		t.Fatal(err)
	}
	g2, err := s.Launch(transcodeLaunchV2{Kind: transcodeWorkPlaybackV2, Key: "same", Mode: ffmpegsupervisor.ModeVOD, Start: func(context.Context) (ffmpegsupervisor.Process, error) { return second, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if g2.Generation != g1.Generation+1 {
		t.Fatalf("generations = %d then %d", g1.Generation, g2.Generation)
	}
	s.Sweep(time.Now().Add(time.Second))
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		i1, _ := first.counts()
		i2, _ := second.counts()
		if i1 > 0 && i2 > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("replacement and abandoned current generation were not interrupted")
}

func TestTranscodeSupervisorV2OptimizationIsNotAbandoned(t *testing.T) {
	s := newTranscodeSupervisorV2(context.Background(), ffmpegsupervisor.Config{InterruptGrace: 5 * time.Millisecond, VODAbandonment: time.Millisecond})
	p := newFakeTranscodeProcessV2()
	if _, err := s.Launch(transcodeLaunchV2{Kind: transcodeWorkOptimizationV2, Key: "job", Mode: ffmpegsupervisor.ModeVOD, Start: func(context.Context) (ffmpegsupervisor.Process, error) { return p, nil }}); err != nil {
		t.Fatal(err)
	}
	s.Sweep(time.Now().Add(time.Hour))
	time.Sleep(10 * time.Millisecond)
	interrupts, _ := p.counts()
	if interrupts != 0 {
		t.Fatalf("durable optimization was interrupted %d times", interrupts)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
