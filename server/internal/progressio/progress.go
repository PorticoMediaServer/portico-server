// Package progressio bounds inactivity while allowing a slow producer to finish.
package progressio

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

type Watch struct {
	mu     sync.Mutex
	timer  *time.Timer
	idle   time.Duration
	closed bool
	cancel context.CancelFunc
	outer  context.CancelFunc
}

// New keeps the parent's cancellation. overall is a safety bound for hostile
// trickle input; zero uses no extra wall-clock cap. Ping records actual bytes or
// completed work, never the passage of time.
func New(parent context.Context, idle, overall time.Duration) (context.Context, *Watch) {
	outer := func() {}
	if overall > 0 {
		parent, outer = context.WithTimeout(parent, overall)
	}
	ctx, cancel := context.WithCancel(parent)
	w := &Watch{idle: idle, cancel: cancel, outer: outer}
	w.timer = time.AfterFunc(idle, func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if !w.closed {
			w.closed = true
			w.cancel()
		}
	})
	return ctx, w
}
func (w *Watch) Ping() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.timer.Reset(w.idle)
	}
}
func (w *Watch) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	w.timer.Stop()
	w.cancel()
	w.outer()
}
func (w *Watch) Reader(r io.Reader) io.Reader { return &reader{r, w} }

type reader struct {
	io.Reader
	watch *Watch
}

func (r *reader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.watch.Ping()
	}
	return n, err
}

// Transport wraps the caller's existing policy transport. Redirect, proxy, DNS,
// TLS and address authorization decisions remain with that transport/client.
type Transport struct {
	Base          http.RoundTripper
	Idle, Overall time.Duration
}

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	idle := t.Idle
	if idle <= 0 {
		idle = 30 * time.Second
	}
	ctx, watch := New(req.Context(), idle, t.Overall)
	copy := req.Clone(ctx)
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(copy)
	if err != nil {
		watch.Close()
		return response, err
	}
	watch.Ping()
	response.Body = &body{ReadCloser: response.Body, watch: watch}
	return response, nil
}
func (t Transport) CloseIdleConnections() {
	if c, ok := t.Base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

type body struct {
	io.ReadCloser
	watch *Watch
}

func (b *body) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.watch.Ping()
	}
	if err == io.EOF {
		b.watch.Close()
	}
	return n, err
}
func (b *body) Close() error { b.watch.Close(); return b.ReadCloser.Close() }
