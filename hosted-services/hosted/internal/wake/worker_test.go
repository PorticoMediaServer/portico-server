package wake

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/hosted/internal/security"
	"portico.local/hosted/trust"
)

type recordingRepo struct {
	job      Work
	handed   atomic.Bool
	finished atomic.Int64
	status   atomic.Int64
	claims   atomic.Int64
	dueCalls atomic.Int64
	due      time.Time
	hasDue   bool
	repeat   bool
}

func (r *recordingRepo) Claim(context.Context, string, time.Time) (Work, bool, error) {
	r.claims.Add(1)
	if !r.repeat && r.handed.Swap(true) {
		return Work{}, false, nil
	}
	if r.job.ServerID == "" {
		return Work{}, false, nil
	}
	return r.job, true, nil
}
func (r *recordingRepo) NextDue(context.Context) (time.Time, bool, error) {
	r.dueCalls.Add(1)
	return r.due, r.hasDue, nil
}
func (r *recordingRepo) Finish(_ context.Context, _ Work, status int, _ time.Time) error {
	r.finished.Add(1)
	r.status.Store(int64(status))
	return nil
}

func testKeys(t *testing.T) *security.Keys {
	t.Helper()
	_, private, e := ed25519.GenerateKey(nil)
	if e != nil {
		t.Fatal(e)
	}
	public, root, e := ed25519.GenerateKey(nil)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := trust.Certify(root, private.Public().(ed25519.PublicKey), "k1", "documents", time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 1, nil)
	if e != nil {
		t.Fatal(e)
	}
	return &security.Keys{KeyID: "k1", PrivateKey: private, RootPublicKey: public, Certificate: cert}
}

// The worker's default egress client is never allowed a loopback destination. The flag that
// used to permit one could not change any outcome — a base URL only reaches this queue after
// directory.registerEndpoint accepted it with loopback refused — so it is gone, and this
// proves the remaining behaviour: a loopback target is simply not delivered to.
func TestDefaultClientRefusesALoopbackWakeTarget(t *testing.T) {
	served := atomic.Int64{}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer origin.Close()
	repo := &recordingRepo{job: Work{ServerID: "srv_1", Kind: "policy", BaseURL: origin.URL, Revision: 4}}
	worker := &Worker{Repo: repo, Keys: testKeys(t)}
	worked, e := worker.Once(context.Background())
	if !worked || e != nil {
		t.Fatalf("expected the job to be claimed and finished: worked=%v err=%v", worked, e)
	}
	if served.Load() != 0 {
		t.Fatalf("a loopback wake reached the listener %d times", served.Load())
	}
	if repo.finished.Load() != 1 || repo.status.Load() != 0 {
		t.Fatalf("expected one failed delivery recorded, got %d with status %d", repo.finished.Load(), repo.status.Load())
	}
}

// An injected client is still honoured; only the egress permission is gone.
func TestInjectedClientStillDelivers(t *testing.T) {
	served := atomic.Int64{}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer origin.Close()
	repo := &recordingRepo{job: Work{ServerID: "srv_1", Kind: "policy", BaseURL: origin.URL, Revision: 4}}
	worker := &Worker{Repo: repo, Keys: testKeys(t), Client: origin.Client()}
	if _, e := worker.Once(context.Background()); e != nil {
		t.Fatal(e)
	}
	if served.Load() != 1 || repo.status.Load() != http.StatusOK {
		t.Fatalf("served=%d status=%d", served.Load(), repo.status.Load())
	}
}

// The point of the change: an idle worker asks the database nothing on a clock. It used to
// claim once a second for ever, whether or not any server had anything waiting.
func TestAnIdleWorkerDoesNoPerSecondDatabaseWork(t *testing.T) {
	repo := &recordingRepo{}
	repo.handed.Store(true)
	worker := &Worker{Repo: repo, Keys: testKeys(t), SafetyInterval: time.Hour}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	worker.Run(ctx)
	// One pass on entry, and then silence: no notification, and the safety tick is an hour.
	if claims := repo.claims.Load(); claims != 1 {
		t.Fatal("an idle worker polled", claims)
	}
}

// A notification is what replaces the clock: the enqueue says so, and the worker looks at once.
func TestANotificationWakesTheWorkerAtOnce(t *testing.T) {
	repo := &recordingRepo{}
	repo.handed.Store(true)
	notify := make(chan struct{}, 1)
	worker := &Worker{Repo: repo, Keys: testKeys(t), SafetyInterval: time.Hour, Notify: notify}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	for range 3 {
		notify <- struct{}{}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if claims := repo.claims.Load(); claims < 4 {
		t.Fatal("notifications did not wake the worker", claims)
	}
}

// Losing every notification costs latency and no work: the safety tick still finds the job.
func TestTheSafetyTickStillFindsWorkWithoutAnyNotification(t *testing.T) {
	repo := &recordingRepo{}
	repo.handed.Store(true)
	worker := &Worker{Repo: repo, Keys: testKeys(t), SafetyInterval: 30 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	worker.Run(ctx)
	if claims := repo.claims.Load(); claims < 3 {
		t.Fatal("the safety tick did not run", claims)
	}
}

// A retry deadline is slept to exactly, not polled towards, and never past the safety tick.
func TestTheWorkerSleepsUntilTheEarliestDeadline(t *testing.T) {
	repo := &recordingRepo{due: time.Now().Add(40 * time.Millisecond), hasDue: true}
	repo.handed.Store(true)
	worker := &Worker{Repo: repo, Keys: testKeys(t), SafetyInterval: time.Hour}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	worker.Run(ctx)
	if claims := repo.claims.Load(); claims < 2 {
		t.Fatal("the worker slept through its own deadline", claims)
	}
	if repo.dueCalls.Load() < 1 {
		t.Fatal("the deadline was never read")
	}
}

// The backlog drain stays bounded: a full batch pauses before the next one, because the wake
// delivery rate is what downstream admission control is sized against.
func TestABacklogDrainsAtABoundedRate(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer origin.Close()
	repo := &recordingRepo{job: Work{ServerID: "srv_1", Kind: "policy", BaseURL: origin.URL, Revision: 4}, repeat: true}
	worker := &Worker{Repo: repo, Keys: testKeys(t), Client: origin.Client(), SafetyInterval: time.Hour, DrainInterval: 80 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 260*time.Millisecond)
	defer cancel()
	worker.Run(ctx)
	// Three or four batches of sixteen in a quarter of a second; without the pause an endless
	// queue would be delivered as fast as the client allows.
	if delivered := repo.finished.Load(); delivered > 5*wakeBatch {
		t.Fatal("the drain rate was not bounded", delivered)
	}
	if delivered := repo.finished.Load(); delivered < wakeBatch {
		t.Fatal("the backlog did not drain", delivered)
	}
}

func TestExpiredSignerDoesNotDeliverAndReleasesWakeLease(t *testing.T) {
	keys := testKeys(t)
	keys.Certificate.NotAfter = time.Now().Add(-time.Minute)
	repo := &recordingRepo{job: Work{ServerID: "srv_1", Kind: "policy", BaseURL: "https://outside.example", Revision: 4}}
	worked, e := (&Worker{Repo: repo, Keys: keys}).Once(context.Background())
	if !worked || e == nil || repo.finished.Load() != 1 || repo.status.Load() != 0 {
		t.Fatal(worked, e, repo.finished.Load())
	}
}
