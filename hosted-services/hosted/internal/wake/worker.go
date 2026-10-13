package wake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"portico.local/hosted/internal/egress"
	"portico.local/hosted/internal/security"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Work struct {
	ServerID, Kind, BaseURL, LeaseToken string
	Revision                            int64
	Attempts                            int
}
type Repository interface {
	Claim(context.Context, string, time.Time) (Work, bool, error)
	Finish(context.Context, Work, int, time.Time) error
	// NextDue is the earliest instant at which any deliverable wake may next be attempted.
	// It is what lets the worker sleep exactly as long as it should instead of asking once a
	// second whether anything has changed, and it is one index scan on wake_due.
	NextDue(context.Context) (time.Time, bool, error)
}

// A wake is delivered only to a server's registered base URL, and a base URL is written only
// after directory.registerEndpoint accepted it through egress.ServerOrigin(..., false) and
// proved it (postgres/endpoint.go). A loopback origin is refused there, so no loopback address
// can ever reach this queue: the worker's egress client is never given permission for one.
type Worker struct {
	Repo   Repository
	Keys   *security.Keys
	Client *http.Client
	// Notify is signalled when a wake becomes deliverable, by the database triggers on
	// wake_state through the process's notification listener. It is an optimisation, never a
	// guarantee: nil, or a notification lost while the listener was reconnecting, costs
	// latency up to SafetyInterval and no work.
	Notify <-chan struct{}
	// SafetyInterval is the longest this worker will sleep when it believes nothing is due.
	SafetyInterval time.Duration
	// DrainInterval is the pause between full batches while a backlog is draining. It is what
	// holds delivery to sixteen a second, which admission control downstream depends on.
	DrainInterval time.Duration
}

// wakeBatch is how many deliveries one pass makes before pausing, so a backlog drains at a
// bounded rate rather than as fast as the network allows.
const wakeBatch = 32

const (
	defaultWakeSafetyInterval = 45 * time.Second
	defaultWakeDrainInterval  = time.Second
)

func RetryDelay(status, attempt int, retryAfter string, now time.Time) time.Duration {
	delay := time.Duration(1<<min(attempt, 8)) * time.Second
	if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
		if seconds, e := strconv.Atoi(strings.TrimSpace(retryAfter)); e == nil && seconds >= 0 {
			delay = max(delay, time.Duration(min(seconds, 3600))*time.Second)
		} else if deadline, e := http.ParseTime(retryAfter); e == nil {
			delay = max(delay, min(deadline.Sub(now), time.Hour))
		}
	}
	return delay + time.Duration(rand.Int64N(int64(delay/5+1)))
}

// Run delivers wakes until the context ends. It used to tick once a second whether or not any
// job existed, which on an idle deployment was the one piece of per-second database work
// Hosted did, for ever, on every instance. Now a pass happens when the database says a wake
// became deliverable, when the earliest retry deadline arrives, or on a slow safety tick that
// exists only to cover a notification nobody was listening for.
func (w *Worker) Run(ctx context.Context) {
	safety := w.SafetyInterval
	if safety <= 0 {
		safety = defaultWakeSafetyInterval
	}
	drain := w.DrainInterval
	if drain <= 0 {
		drain = defaultWakeDrainInterval
	}
	timer := time.NewTimer(safety)
	defer timer.Stop()
	for {
		delivered := 0
		var jobs sync.WaitGroup
		// A process whose signer is expired or revoked takes no work: claiming a
		// job spends an attempt and pushes its next try back, which would delay
		// the process that can sign. It looks again at the safety interval.
		usable := w.Keys.ValidateSigner(time.Now()) == nil
		for range wakeBatch {
			if !usable {
				break
			}
			job, ok, e := w.Repo.Claim(ctx, security.RandomID("lease_"), time.Now().UTC())
			if e != nil || !ok {
				break
			}
			delivered++
			jobs.Add(1)
			go func() { defer jobs.Done(); _, _ = w.deliver(ctx, job) }()
		}
		jobs.Wait()
		wait := safety
		if !usable {
			// Nothing can be sent; do not wake for deadlines that cannot be met.
		} else if delivered == wakeBatch {
			// Still busy. Pause between batches so the backlog drains at a bounded rate
			// rather than as fast as the pool and the network allow.
			wait = drain
		} else if at, ok, e := w.Repo.NextDue(ctx); e == nil && ok {
			// Sleep exactly until the earliest retry deadline, never past the safety tick.
			if until := time.Until(at); until < wait {
				wait = max(until, 0)
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(wait)
		select {
		case <-ctx.Done():
			return
		case <-w.Notify:
		case <-timer.C:
		}
	}
}
func (w *Worker) Once(ctx context.Context) (bool, error) {
	if e := w.Keys.ValidateSigner(time.Now()); e != nil {
		return false, e
	}
	job, ok, e := w.Repo.Claim(ctx, security.RandomID("lease_"), time.Now().UTC())
	if e != nil || !ok {
		return ok, e
	}
	return w.deliver(ctx, job)
}
func (w *Worker) deliver(ctx context.Context, job Work) (bool, error) {
	now := time.Now().UTC()
	raw, e := json.Marshal(struct {
		ServerID  string    `json:"serverId"`
		Kind      string    `json:"kind"`
		Revision  int64     `json:"revision"`
		IssuedAt  time.Time `json:"issuedAt"`
		ExpiresAt time.Time `json:"expiresAt"`
	}{job.ServerID, job.Kind, job.Revision, now, now.Add(time.Minute)})
	if e != nil {
		return true, e
	}
	signed, e := w.Keys.Sign(raw)
	if e != nil {
		finish := w.Repo.Finish(ctx, job, 0, now.Add(RetryDelay(0, job.Attempts, "", now)))
		return true, errors.Join(e, finish)
	}
	body, _ := json.Marshal(signed)
	request, e := http.NewRequestWithContext(ctx, http.MethodPost, job.BaseURL+"/v1/hosted/wake", bytes.NewReader(body))
	if e != nil {
		return true, e
	}
	request.Header.Set("Content-Type", "application/json")
	client := w.Client
	if client == nil {
		client = egress.NewServerClient(false)
	}
	response, e := client.Do(request)
	status := 0
	retry := ""
	if e == nil {
		status = response.StatusCode
		retry = response.Header.Get("Retry-After")
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		response.Body.Close()
	}
	// Delivery is not policy acknowledgement. Successful callbacks still retry slowly until exact ack.
	delay := RetryDelay(status, job.Attempts, retry, now)
	if status >= 200 && status < 300 {
		delay = max(30*time.Second, RetryDelay(status, job.Attempts+5, retry, now))
	}
	finish := w.Repo.Finish(ctx, job, status, now.Add(delay))
	return true, finish
}
