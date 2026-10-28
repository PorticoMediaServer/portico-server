package readprep

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var testKey = Key{Namespace: "facets-v1", Basis: "library-lifetime:42/content:7", Canonical: "exact-policy/query/window"}

type testWork struct {
	bytes int64
	step  func(context.Context, *Budget) (StepResult, error)
}

func (w *testWork) AccountedBytes() int64                                   { return w.bytes }
func (w *testWork) Step(ctx context.Context, b *Budget) (StepResult, error) { return w.step(ctx, b) }

func storeFor(t *testing.T, cfg Config) *Store {
	t.Helper()
	// Blocking test steps use explicit channels, not the production deadline.
	if cfg.StepTimeout == 0 {
		cfg.StepTimeout = time.Minute
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func submit(t *testing.T, s *Store, caller Caller, key Key, p Plan) Handle {
	t.Helper()
	h, err := s.Submit(context.Background(), caller, key, p)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func eventually(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !f() {
		if time.Now().After(deadline) {
			t.Fatal("condition did not become true")
		}
		time.Sleep(time.Millisecond)
	}
}

func ready(t *testing.T, s *Store, caller Caller, h Handle) Result {
	t.Helper()
	var r Result
	eventually(t, func() bool {
		var err error
		r, err = s.Poll(context.Background(), caller, h)
		if err != nil {
			t.Fatal(err)
		}
		return r.State == Ready
	})
	return r
}

func simplePlan(payload string) Plan {
	return Plan{InitialBytes: 64, New: func(context.Context, *Budget) (Computation, error) {
		return &testWork{bytes: 64, step: func(context.Context, *Budget) (StepResult, error) {
			return StepResult{Done: true, Payload: []byte(payload)}, nil
		}}, nil
	}}
}

func TestSharingPrivateHandlesAndPayloadOwnership(t *testing.T) {
	s := storeFor(t, Config{})
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var made atomic.Int32
	p := Plan{InitialBytes: 64, New: func(context.Context, *Budget) (Computation, error) {
		made.Add(1)
		return &testWork{bytes: 64, step: func(ctx context.Context, b *Budget) (StepResult, error) {
			close(started)
			select {
			case <-release:
				return StepResult{Done: true, Payload: []byte("ready")}, nil
			case <-ctx.Done():
				return StepResult{}, ctx.Err()
			}
		}}, nil
	}}
	request, cancelRequest := context.WithCancel(context.Background())
	h1, err := s.Submit(request, "session-a/profile-1", testKey, p)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancelRequest() // One HTTP request's lifetime is not shared demand lifetime.
	h2 := submit(t, s, "session-b/profile-2", testKey, Plan{New: func(context.Context, *Budget) (Computation, error) {
		panic("identical content must not construct twice")
	}})
	if h1 == h2 || len(h1) != 43 || len(h2) != 43 {
		t.Fatal("handles must be independent 256-bit tokens")
	}
	if _, err := s.Poll(context.Background(), "session-b/profile-2", h1); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if err := s.Cancel(context.Background(), "session-b/profile-2", h1); err != nil {
		t.Fatal(err)
	}
	if r, err := s.Poll(context.Background(), "session-a/profile-1", h1); err != nil || r.State != Preparing || r.Payload != nil {
		t.Fatalf("%+v %v", r, err)
	}
	if err := s.Cancel(context.Background(), "session-a/profile-1", h1); err != nil {
		t.Fatal(err)
	}
	// Release without a double-close from the cleanup above.
	release <- struct{}{}
	r := ready(t, s, "session-b/profile-2", h2)
	if string(r.Payload) != "ready" || r.Basis != testKey.Basis || made.Load() != 1 {
		t.Fatalf("%+v made=%d", r, made.Load())
	}
	r.Payload[0] = 'X'
	if string(ready(t, s, "session-b/profile-2", h2).Payload) != "ready" {
		t.Fatal("poll mutated shared payload")
	}
	h3, err := s.Open(context.Background(), "session-c", testKey)
	if err != nil || h3 == h2 {
		t.Fatalf("%s %v", h3, err)
	}
	if got := s.Stats(); got.Entries != 1 || got.Handles != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestFullKeyEqualityAndCallerContext(t *testing.T) {
	s := storeFor(t, Config{})
	keys := []Key{testKey,
		{Namespace: testKey.Namespace + "x", Basis: testKey.Basis, Canonical: testKey.Canonical},
		{Namespace: testKey.Namespace, Basis: testKey.Basis + "x", Canonical: testKey.Canonical},
		{Namespace: testKey.Namespace, Basis: testKey.Basis, Canonical: testKey.Canonical + "\x00"},
		{Namespace: "a", Basis: "bc", Canonical: "d"}, {Namespace: "ab", Basis: "c", Canonical: "d"}}
	for i, key := range keys {
		h := submit(t, s, "account/session/profile/policy", key, simplePlan(string(rune('a'+i))))
		if string(ready(t, s, "account/session/profile/policy", h).Payload) != string(rune('a'+i)) {
			t.Fatal("keys aliased")
		}
		for _, other := range []Caller{"", "account/session/profile/policy\x00", "account/session/other/policy", "account/new/profile/policy"} {
			if _, err := s.Poll(context.Background(), other, h); !errors.Is(err, ErrStale) {
				t.Fatalf("other=%q err=%v", other, err)
			}
		}
	}
	if s.Stats().Entries != len(keys) {
		t.Fatal("full key equality not preserved")
	}
	s.Invalidate(testKey)
	if _, err := s.Open(context.Background(), "account/session/profile/policy", testKey); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	// New process/store knows no handles or content from a previous instance.
	fresh := storeFor(t, Config{})
	if _, err := fresh.Open(context.Background(), "account/session/profile/policy", keys[1]); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}

func TestLastCancelKeepsUnwindingMemoryCharged(t *testing.T) {
	s := storeFor(t, Config{Workers: 1, MaxBytes: 2100})
	started, cancelled, unwind := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer close(unwind)
	h := submit(t, s, "a", testKey, Plan{InitialBytes: 600, New: func(context.Context, *Budget) (Computation, error) {
		return &testWork{bytes: 600, step: func(ctx context.Context, _ *Budget) (StepResult, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			<-unwind
			return StepResult{}, ctx.Err()
		}}, nil
	}})
	<-started
	h2, err := s.Open(context.Background(), "b", testKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(context.Background(), "a", h); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
		t.Fatal("other demand was cancelled")
	default:
	}
	if err := s.Cancel(context.Background(), "b", h2); err != nil {
		t.Fatal(err)
	}
	<-cancelled
	stats := s.Stats()
	if stats.Entries != 0 || stats.Handles != 0 || stats.Bytes < 600 || stats.Running != 1 {
		t.Fatalf("%+v", stats)
	}
	if _, err := s.Submit(context.Background(), "c", testKey, simplePlan("x")); !errors.Is(err, ErrCapacity) {
		t.Fatalf("retiring work bypassed cap: %v", err)
	}
	if _, err := s.Poll(context.Background(), "b", h2); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	unwind <- struct{}{}
	eventually(t, func() bool { return s.Stats().Bytes == 0 })
	if err := s.Cancel(context.Background(), "b", h2); err != nil {
		t.Fatal(err)
	}
}

type testClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *testClock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.at }
func (c *testClock) add(d time.Duration) { c.mu.Lock(); c.at = c.at.Add(d); c.mu.Unlock() }

func TestOriginalIdleAndCompleteExpiry(t *testing.T) {
	clock := &testClock{at: time.Unix(1234, 0)}
	s := storeFor(t, Config{Now: clock.now, OriginalTTL: 25 * time.Second, IdleTTL: 10 * time.Second, CompleteTTL: time.Minute})
	h := submit(t, s, "a", testKey, simplePlan("yes"))
	ready(t, s, "a", h)
	for i := 0; i < 2; i++ {
		clock.add(9 * time.Second)
		if _, err := s.Poll(context.Background(), "a", h); err != nil {
			t.Fatal(err)
		}
	}
	clock.add(7 * time.Second) // Renewing idle never renews original lifetime.
	if _, err := s.Poll(context.Background(), "a", h); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	h2, err := s.Open(context.Background(), "a", testKey)
	if err != nil {
		t.Fatal(err)
	}
	clock.add(10 * time.Second)
	if _, err := s.Poll(context.Background(), "a", h2); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	clock.add(24 * time.Second) // Content still reusable at t=59 despite no demand.
	h3, err := s.Open(context.Background(), "b", testKey)
	if err != nil {
		t.Fatal(err)
	}
	clock.add(time.Second)
	if _, err := s.Poll(context.Background(), "b", h3); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if got := s.Stats(); got.Bytes != 0 || got.Entries != 0 || got.Handles != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestExpiryStopsUnfinishedWork(t *testing.T) {
	clock := &testClock{at: time.Unix(1234, 0)}
	s := storeFor(t, Config{Now: clock.now, IdleTTL: time.Second})
	started, stopped := make(chan struct{}), make(chan struct{})
	submit(t, s, "a", testKey, Plan{New: func(context.Context, *Budget) (Computation, error) {
		return &testWork{step: func(ctx context.Context, _ *Budget) (StepResult, error) {
			close(started)
			<-ctx.Done()
			close(stopped)
			return StepResult{}, ctx.Err()
		}}, nil
	}})
	<-started
	clock.add(time.Second)
	if s.Stats().Handles != 0 {
		t.Fatal("expired handle retained")
	}
	<-stopped
	eventually(t, func() bool { return s.Stats().Bytes == 0 })
}

func TestAdmissionGrowthEvictionAndPayloadAccounting(t *testing.T) {
	s := storeFor(t, Config{Workers: 1, MaxBytes: 2600})
	old := submit(t, s, "a", testKey, simplePlan("old"))
	ready(t, s, "a", old)
	key2 := testKey
	key2.Canonical += "2"
	var denied atomic.Bool
	h := submit(t, s, "b", key2, Plan{InitialBytes: 64, New: func(context.Context, *Budget) (Computation, error) {
		w := &testWork{bytes: 64}
		w.step = func(_ context.Context, b *Budget) (StepResult, error) {
			if err := b.Resize(1400); err != nil {
				return StepResult{}, err
			}
			// This succeeds only by evicting completed old content and handles.
			w.bytes = 1400
			if err := b.Resize(math.MaxInt64); !errors.Is(err, ErrCapacity) {
				return StepResult{}, errors.New("overflow bypass")
			}
			denied.Store(true)
			// Ignoring failed growth must still become a terminal capacity error.
			return StepResult{Done: true, Payload: []byte("never ready")}, nil
		}
		return w, nil
	}})
	eventually(t, func() bool { _, err := s.Poll(context.Background(), "b", h); return errors.Is(err, ErrCapacity) })
	if !denied.Load() {
		t.Fatal("growth was not attempted")
	}
	if _, err := s.Poll(context.Background(), "a", old); !errors.Is(err, ErrStale) {
		t.Fatalf("old entry survived pressure: %v", err)
	}
	if stats := s.Stats(); stats.Bytes > 2600 || stats.Entries != 1 {
		t.Fatalf("%+v", stats)
	}

	// A huge descriptor/estimate is rejected before the factory runs.
	var made atomic.Bool
	key3 := testKey
	key3.Canonical = strings.Repeat("x", 3000)
	for _, p := range []Plan{{InitialBytes: math.MaxInt64, New: func(context.Context, *Budget) (Computation, error) { made.Store(true); return nil, nil }}} {
		if _, err := s.Submit(context.Background(), "c", key3, p); !errors.Is(err, ErrCapacity) {
			t.Fatal(err)
		}
	}
	if made.Load() {
		t.Fatal("factory ran before admission")
	}
}

func TestPayloadCapacityAndInFlightCopyCharged(t *testing.T) {
	s := storeFor(t, Config{MaxBytes: 2000})
	h := submit(t, s, "a", testKey, Plan{InitialBytes: 800, New: func(context.Context, *Budget) (Computation, error) {
		return &testWork{bytes: 800, step: func(context.Context, *Budget) (StepResult, error) {
			p := make([]byte, 700, 800)
			return StepResult{Done: true, Payload: p}, nil
		}}, nil
	}})
	eventually(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.entries[testKey].complete })
	before := s.Stats().Bytes
	if before < 800+entryOverhead+handleOverhead {
		t.Fatalf("payload backing capacity not charged: %d", before)
	}
	if _, err := s.Poll(context.Background(), "a", h); !errors.Is(err, ErrCapacity) {
		t.Fatalf("copy did not require admission: %v", err)
	}
	if s.Stats().Bytes != before {
		t.Fatal("failed copy changed reservation")
	}
}

func TestTerminalErrorsDoNotEndlesslyPrepare(t *testing.T) {
	for _, mode := range []string{"factory", "step", "panic", "nil", "underreported", "overreserved", "partial-payload", "deadline", "bad-resize"} {
		t.Run(mode, func(t *testing.T) {
			cfg := Config{}
			if mode == "deadline" {
				cfg.StepTimeout = time.Millisecond
			}
			s := storeFor(t, cfg)
			var calls atomic.Int32
			p := Plan{InitialBytes: 64, New: func(context.Context, *Budget) (Computation, error) {
				if mode == "factory" {
					return nil, errors.New("deterministic factory failure")
				}
				if mode == "nil" {
					return nil, nil
				}
				w := &testWork{bytes: 64}
				w.step = func(ctx context.Context, b *Budget) (StepResult, error) {
					calls.Add(1)
					switch mode {
					case "step":
						return StepResult{}, errors.New("deterministic failure")
					case "panic":
						panic("bad producer")
					case "underreported":
						w.bytes = 1
						return StepResult{Done: true, Payload: make([]byte, 2, 10)}, nil
					case "overreserved":
						w.bytes = 65
						return StepResult{}, nil
					case "partial-payload":
						return StepResult{Payload: []byte("partial")}, nil
					case "deadline":
						<-ctx.Done()
						return StepResult{}, ctx.Err()
					case "bad-resize":
						b.Resize(-1)
						return StepResult{}, nil
					}
					return StepResult{}, nil
				}
				return w, nil
			}}
			h := submit(t, s, "a", testKey, p)
			want := ErrFailed
			if mode == "underreported" || mode == "overreserved" || mode == "partial-payload" || mode == "bad-resize" || mode == "nil" {
				want = ErrInvalid
			}
			eventually(t, func() bool { _, err := s.Poll(context.Background(), "a", h); return errors.Is(err, want) })
			count := calls.Load()
			h2 := submit(t, s, "b", testKey, p)
			if _, err := s.Poll(context.Background(), "b", h2); !errors.Is(err, want) {
				t.Fatal(err)
			}
			if calls.Load() != count || count > 1 {
				t.Fatal("terminal failure was recomputed")
			}
		})
	}
}

func TestBoundedQueueWorkersHandlesAndYield(t *testing.T) {
	s := storeFor(t, Config{Workers: 1, MaxQueued: 1, MaxHandles: 3})
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var steps atomic.Int32
	first := submit(t, s, "a", testKey, Plan{InitialBytes: 64, New: func(context.Context, *Budget) (Computation, error) {
		return &testWork{bytes: 64, step: func(ctx context.Context, _ *Budget) (StepResult, error) {
			if steps.Add(1) == 1 {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return StepResult{}, ctx.Err()
				}
				return StepResult{}, nil
			}
			return StepResult{Done: true, Payload: []byte("first")}, nil
		}}, nil
	}})
	<-started
	key2 := testKey
	key2.Canonical += "2"
	var observed atomic.Int32
	second := submit(t, s, "b", key2, Plan{InitialBytes: 64, New: func(context.Context, *Budget) (Computation, error) {
		observed.Store(steps.Load())
		return simplePlan("second").New(context.Background(), nil)
	}})
	key3 := testKey
	key3.Canonical += "3"
	if _, err := s.Submit(context.Background(), "c", key3, simplePlan("third")); !errors.Is(err, ErrCapacity) {
		t.Fatalf("queue: %v", err)
	}
	third, err := s.Open(context.Background(), "c", testKey) // sharing needs no queue slot
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(context.Background(), "d", testKey); !errors.Is(err, ErrCapacity) {
		t.Fatalf("handles: %v", err)
	}
	if got := s.Stats(); got.Queued != 1 || got.Running != 1 || got.Handles != 3 {
		t.Fatalf("%+v", got)
	}
	release <- struct{}{}
	ready(t, s, "a", first)
	ready(t, s, "b", second)
	ready(t, s, "c", third)
	if steps.Load() != 2 || observed.Load() != 1 {
		t.Fatalf("computation did not yield fairly: %d %d", steps.Load(), observed.Load())
	}
}

func TestCancelPollInvalidateAndShutdownRaces(t *testing.T) {
	s := storeFor(t, Config{})
	started := make(chan struct{})
	h := submit(t, s, "a", testKey, Plan{New: func(context.Context, *Budget) (Computation, error) {
		return &testWork{step: func(ctx context.Context, _ *Budget) (StepResult, error) {
			close(started)
			<-ctx.Done()
			return StepResult{}, ctx.Err()
		}}, nil
	}})
	<-started
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				switch i % 4 {
				case 0:
					s.Poll(context.Background(), "a", h)
				case 1:
					s.Cancel(context.Background(), "a", h)
				case 2:
					s.Invalidate(testKey)
				case 3:
					s.Close()
				}
			}
		}(i)
	}
	wg.Wait()
	if got := s.Stats(); got != (Stats{}) {
		t.Fatalf("shutdown leaked memory or work: %+v", got)
	}
	if _, err := s.Submit(context.Background(), "a", testKey, simplePlan("x")); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := s.Open(context.Background(), "a", testKey); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := s.Poll(context.Background(), "a", h); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err := s.Cancel(context.Background(), "a", h); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestValidationAndCancelledRequests(t *testing.T) {
	for _, cfg := range []Config{{Workers: 3}, {MaxQueued: 17}, {MaxHandles: 513}, {MaxBytes: -1}, {IdleTTL: -1}, {OriginalTTL: -1}, {CompleteTTL: -1}, {StepTimeout: -1}} {
		if s, err := New(cfg); !errors.Is(err, ErrInvalid) {
			if s != nil {
				s.Close()
			}
			t.Fatalf("%+v: %v", cfg, err)
		}
	}
	s := storeFor(t, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Submit(ctx, "a", testKey, simplePlan("x")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.Poll(ctx, "a", "unknown"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := s.Cancel(ctx, "a", "unknown"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, key := range []Key{{}, {Namespace: "a", Basis: "b"}, {Namespace: "a", Canonical: "c"}} {
		if _, err := s.Submit(context.Background(), "a", key, simplePlan("x")); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, err := s.Submit(context.Background(), "", testKey, simplePlan("x")); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.Submit(context.Background(), "a", testKey, Plan{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.Open(context.Background(), "a", testKey); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if s.Stats().Bytes != 0 {
		t.Fatal("invalid request allocated store memory")
	}
}
