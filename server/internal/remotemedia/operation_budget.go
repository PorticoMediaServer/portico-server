package remotemedia

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
)

// OperationBudget is the single per-remote concurrent-operation ceiling. It
// owns no actor/operation permission and does not count upstream idle sockets.
// The source process lifetime and normalized profile supply its actual bounds.
type OperationBudget struct {
	life   context.Context
	slots  chan struct{}
	mu     sync.Mutex
	active int
	peak   int
}

type OperationCounts struct{ Capacity, Active, Peak int }
type operationBudgetKey struct{}
type operationPermit struct {
	budget *OperationBudget
	active atomic.Bool
}

// NewOperationBudget rejects unknown capacity/lifetime without a fallback.
func NewOperationBudget(life context.Context, capacity int) *OperationBudget {
	if life == nil || capacity < 1 || capacity > 256 {
		return nil
	}
	return &OperationBudget{life: life, slots: make(chan struct{}, capacity)}
}

func (b *OperationBudget) Counts() OperationCounts {
	if b == nil {
		return OperationCounts{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return OperationCounts{Capacity: cap(b.slots), Active: b.active, Peak: b.peak}
}

func (b *OperationBudget) enter(ctx context.Context) (context.Context, func(), error) {
	if b == nil || b.life == nil || cap(b.slots) < 1 || ctx == nil {
		return nil, nil, ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := b.life.Err(); err != nil {
		return nil, nil, err
	}
	// A reconnect's metadata fences run sequentially inside the operation that
	// owns this private context. They borrow its slot; no second local pool or
	// cap1 self-deadlock. A returned/escaped old context cannot revive a permit.
	if current, ok := ctx.Value(operationBudgetKey{}).(*operationPermit); ok && current.budget == b && current.active.Load() {
		return ctx, func() {}, nil
	}
	select {
	case b.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-b.life.Done():
		return nil, nil, b.life.Err()
	}
	if err := ctx.Err(); err != nil {
		<-b.slots
		return nil, nil, err
	}
	if err := b.life.Err(); err != nil {
		<-b.slots
		return nil, nil, err
	}
	permit := &operationPermit{budget: b}
	permit.active.Store(true)
	b.mu.Lock()
	b.active++
	b.peak = max(b.peak, b.active)
	b.mu.Unlock()
	life, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(b.life, cancel)
	var once sync.Once
	release := func() {
		once.Do(func() {
			permit.active.Store(false)
			stop()
			cancel()
			b.mu.Lock()
			b.active--
			b.mu.Unlock()
			<-b.slots
		})
	}
	return context.WithValue(life, operationBudgetKey{}, permit), release, nil
}

// admittedBody retains the same operation through progressive consumption.
// Child close/EOF/cancellation ends only that stream's capacity, not the source.
type admittedBody struct {
	body    io.ReadCloser
	release func()
	once    sync.Once
	stop    func() bool
}

func retainOperationBody(ctx context.Context, body io.ReadCloser, release func()) io.ReadCloser {
	b := &admittedBody{body: body, release: release}
	// Set the stop hook before the callback can use it.
	ready := make(chan struct{})
	b.stop = context.AfterFunc(ctx, func() { <-ready; b.Close() })
	close(ready)
	return b
}

func (b *admittedBody) Close() error {
	var err error
	b.once.Do(func() {
		b.stop()
		err = b.body.Close()
		b.release()
	})
	return err
}
func (b *admittedBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if err != nil {
		b.Close()
	}
	return n, err
}
