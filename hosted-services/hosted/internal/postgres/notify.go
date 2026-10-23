package postgres

import (
	"context"
	"database/sql"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// Hosted's workers used to ask PostgreSQL whether there was anything to do on a fixed clock —
// the wake worker every second, for ever, on a database that is almost always empty of work.
// At the scale this service is meant to run at, that is the only per-second cost an idle
// deployment has, and it buys nothing: the moment work appears is known exactly, because a
// transaction created it.
//
// So the writer says so. Every enqueue publishes a NOTIFY inside its own transaction, which
// PostgreSQL delivers on commit and discards on rollback, and one connection per process
// listens. The workers then sleep until the work exists or their own retry deadline arrives.
//
// What LISTEN/NOTIFY does not give you is durability: a notification sent while nobody is
// listening is gone. That is why nothing here is load-bearing. Every worker keeps a slow
// safety tick, and a listener that reconnects signals every channel immediately, because the
// gap may have hidden anything. A missed notification costs latency, never work.

// Channel names are completed with the current schema, so the per-schema databases the tests
// build do not hear each other's notifications.
const (
	WakeChannel        = "portico_wake"
	CertificateChannel = "portico_certificates"
	MailChannel        = "portico_mail"
	// Announced by triggers (migration 0006) on every insert into the queue.
	ExportChannel = "portico_exports"
	// LabelRotationChannel announces a queued member-label rotation (A87).
	LabelRotationChannel = "portico_label_rotation"
	// ServerListChannel announces that some account's server-list version moved
	// (migration 0017); the in-memory version map reads what is new.
	ServerListChannel = "portico_server_list"
)

// notifyTx publishes on a channel inside the caller's transaction. Delivery is exactly at
// commit, so a worker never wakes for work a rollback took away.
func notifyTx(ctx context.Context, tx *sql.Tx, channel string) error {
	_, e := tx.ExecContext(ctx, `SELECT pg_notify($1 || '_' || current_schema(), '')`, channel)
	return e
}

// Listener holds one PostgreSQL connection of its own, outside the request pool, and turns
// notifications into signals on buffered channels. A signal means "look now"; it carries
// nothing, so a worker that coalesces two of them into one pass has lost nothing either.
type Listener struct {
	connected atomic.Bool
	url       string
	mu        sync.Mutex
	// subscribers is fixed before Run starts: every worker subscribes at wiring time, so the
	// connection can LISTEN on the whole set at once and re-LISTEN after a reconnect.
	subscribers map[string][]chan struct{}
	Log         func(string)
}

func NewListener(url string) *Listener {
	return &Listener{url: url, subscribers: map[string][]chan struct{}{}}
}

// Subscribe returns a channel signalled when that channel is notified. The buffer is one: a
// burst is one look, which is the whole point.
func (l *Listener) Subscribe(channel string) <-chan struct{} {
	out := make(chan struct{}, 1)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.subscribers[channel] = append(l.subscribers[channel], out)
	return out
}

func (l *Listener) signal(channel string) {
	l.mu.Lock()
	targets := l.subscribers[channel]
	l.mu.Unlock()
	for _, target := range targets {
		select {
		case target <- struct{}{}:
		default:
		}
	}
}

func (l *Listener) signalAll() {
	l.mu.Lock()
	channels := make([]string, 0, len(l.subscribers))
	for channel := range l.subscribers {
		channels = append(channels, channel)
	}
	l.mu.Unlock()
	for _, channel := range channels {
		l.signal(channel)
	}
}

func (l *Listener) channels() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.subscribers))
	for channel := range l.subscribers {
		out = append(out, channel)
	}
	return out
}

// Run keeps one listening connection alive for the life of the process. Nothing depends on it
// staying up: while it is down the workers fall back to their safety ticks, and when it comes
// back every subscriber is signalled once because the gap may have swallowed a notification.
func (l *Listener) Run(ctx context.Context) {
	failures := 0
	for ctx.Err() == nil {
		if e := l.session(ctx); e != nil && ctx.Err() == nil {
			failures = min(failures+1, 6)
			// The same rule as everywhere else: exponential with proportional jitter, so a
			// PostgreSQL that restarts is not met by every Hosted instance at once.
			delay := time.Duration(1<<failures) * time.Second
			delay += time.Duration(rand.Int64N(int64(delay/2) + 1))
			if l.Log != nil && failures == 1 {
				l.Log("database notification listener lost: " + e.Error())
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}
		failures = 0
	}
}

func (l *Listener) session(ctx context.Context) error {
	conn, e := pgx.Connect(ctx, l.url)
	if e != nil {
		return e
	}
	defer conn.Close(context.WithoutCancel(ctx))
	var schema string
	if e = conn.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); e != nil {
		return e
	}
	for _, channel := range l.channels() {
		if _, e = conn.Exec(ctx, `LISTEN "`+channel+`_`+schema+`"`); e != nil {
			return e
		}
	}
	// Anything that happened while there was no listener is invisible now, so assume the
	// worst once rather than sleep through it.
	l.connected.Store(true)
	defer l.connected.Store(false)
	l.signalAll()
	for {
		notification, e := conn.WaitForNotification(ctx)
		if e != nil {
			return e
		}
		name := notification.Channel
		if len(name) > len(schema)+1 && name[len(name)-len(schema)-1:] == "_"+schema {
			name = name[:len(name)-len(schema)-1]
		}
		l.signal(name)
	}
}

func (l *Listener) Connected() bool { return l.connected.Load() }
