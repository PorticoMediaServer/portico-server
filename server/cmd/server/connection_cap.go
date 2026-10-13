package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"portico.local/server/internal/networking"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The admission lanes protect handlers, not accepted connections. Every accepted
// connection has buffers and goroutine state before any lane is consulted.
// HTTP/2 adds concurrent stream state and flow-control buffers, so transport
// memory cannot be budgeted as just two HTTP/1 buffers per connection.
//
// The cap is a listener, not a handler: a connection that is never accepted
// costs nothing at all. Past it the kernel's own listen backlog absorbs the
// burst, and past that the client sees a connection refusal — which is the
// honest answer when the server has no capacity left to explain itself with, and
// is what every HTTP client already retries.
//
// The absolute ceiling is 2,048; connectionBudget selects a smaller bound from
// the discoverable host/process memory using a conservative planning allowance.
// This is one part of overload protection, alongside stream and admission caps.

// maximumConnections is the upper bound even on large hosts.
const maximumConnections = 2048

// cappedListener bounds accepted connections.
type cappedListener struct {
	net.Listener
	slots        chan struct{}
	ordinary     chan struct{}
	ownerRequest func(*http.Request) bool
	// held is published so the diagnostics and the log can say how close to the
	// ceiling the server is running.
	held      atomic.Int64
	deferred  atomic.Uint64
	warned    sync.Once
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

func capConnections(inner net.Listener, limit int) *cappedListener {
	if limit < 1 {
		limit = maximumConnections
	}
	l := &cappedListener{Listener: inner, slots: make(chan struct{}, limit), done: make(chan struct{})}
	for i := 0; i < limit; i++ {
		l.slots <- struct{}{}
	}
	return l
}

// boundedDirectListener puts the socket cap under the TLS classifier. net/http
// must receive the classifier's *tls.Conn to recognize h2 and populate r.TLS.
func boundedDirectListener(raw net.Listener, limit int, build func(net.Listener) (*networking.DirectListener, error)) (*networking.DirectListener, error) {
	capped := capConnections(raw, limit)
	direct, err := build(capped)
	if err != nil {
		_ = capped.Close()
		return nil, err
	}
	return direct, nil
}

func (l *cappedListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	default:
	}
	select {
	case <-l.slots:
	default:
		// At the ceiling. Wait for a slot rather than accepting a connection there
		// is no room to serve: the backlog is a better queue than the heap.
		l.deferred.Add(1)
		l.warned.Do(func() {
			log.Printf("Connection ceiling of %d reached; further connections wait in the listen backlog", cap(l.slots))
		})
		select {
		case <-l.slots:
		case <-l.done:
			return nil, net.ErrClosed
		}
	}
	conn, err := l.Listener.Accept()
	if err != nil {
		l.slots <- struct{}{}
		return nil, err
	}
	l.held.Add(1)
	return &cappedConn{Conn: conn, listener: l}, nil
}

// Closing the transport must also wake an accept parked at the capacity ceiling.
// Otherwise http.Server.Shutdown waits forever for its serving listener to exit.
func (l *cappedListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.done)
		l.closeErr = l.Listener.Close()
	})
	return l.closeErr
}

// Held reports how many connections are open, and how many accepts have had to
// wait for a slot.
func (l *cappedListener) Held() (int64, uint64) { return l.held.Load(), l.deferred.Load() }

// cappedConn returns its slot exactly once, whatever closes it.
type cappedConn struct {
	net.Conn
	listener     *cappedListener
	once         sync.Once
	mu           sync.Mutex
	ordinaryHeld bool
	closed       bool
}

func (c *cappedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		if c.ordinaryHeld {
			<-c.listener.ordinary
		}
		c.mu.Unlock()
		c.listener.held.Add(-1)
		select {
		case c.listener.slots <- struct{}{}:
		default:
			// Unreachable: the channel holds exactly the slots handed out. Dropping
			// one here would shrink the ceiling permanently, so it is worth saying.
			log.Print("connection slot released twice; the connection ceiling has shrunk")
		}
	})
	return err
}

// SetDeadline and friends pass through; a wrapped connection that quietly lost
// its deadlines would defeat the write budgets the streaming routes set.
func (c *cappedConn) SetDeadline(t time.Time) error      { return c.Conn.SetDeadline(t) }
func (c *cappedConn) SetReadDeadline(t time.Time) error  { return c.Conn.SetReadDeadline(t) }
func (c *cappedConn) SetWriteDeadline(t time.Time) error { return c.Conn.SetWriteDeadline(t) }

// consoleConnectionReserve is separate from the owner's ordinary connection
// budget. Requests reveal their route only after TLS and HTTP headers arrive,
// so these bounded slots let the console reach authorization even at capacity.
const consoleConnectionReserve = 32

type connectionKey struct{}

func consoleConnections(inner net.Listener, limit int) *cappedListener {
	l := capConnections(inner, limit+consoleConnectionReserve)
	l.ordinary = make(chan struct{}, limit)
	return l
}

func (l *cappedListener) context(ctx context.Context, conn net.Conn) context.Context {
	if c, ok := networking.RawConnection(conn).(*cappedConn); ok {
		return context.WithValue(ctx, connectionKey{}, c)
	}
	return ctx
}

func (l *cappedListener) protectConsole(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Context().Value(connectionKey{}).(*cappedConn)
		// Route authorization still runs normally. The reserve also carries the
		// console shell and sign-in, so an owner without a warm session can enter.
		console := !strings.HasPrefix(r.URL.Path, "/v1/") || strings.HasPrefix(r.URL.Path, "/v1/admin/") || strings.HasPrefix(r.URL.Path, "/v1/auth/") || strings.HasPrefix(r.URL.Path, "/v1/direct/") || r.URL.Path == "/v1/server" || r.URL.Path == "/v1/system" || r.URL.Path == "/v1/capabilities" || r.URL.Path == "/v1/readiness" || strings.HasPrefix(r.URL.Path, "/v1/sessions") || r.URL.Path == "/v1/networking/connection" || strings.HasPrefix(r.URL.Path, "/v1/setup")
		if !console && l.ownerRequest != nil {
			console = l.ownerRequest(r)
		}
		if c != nil && console && r.ProtoMajor < 2 {
			c.mu.Lock()
			ordinary := c.ordinaryHeld
			c.mu.Unlock()
			if !ordinary {
				w.Header().Set("Connection", "close")
			}
		}
		if c != nil && !console && !c.admitOrdinary() {
			w.Header().Set("Connection", "close")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"code":"server_busy","message":"The server is busy. Try again shortly.","retryable":true}}`)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (c *cappedConn) admitOrdinary() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ordinaryHeld || c.listener.ordinary == nil {
		return true
	}
	if c.closed {
		return false
	}
	select {
	case c.listener.ordinary <- struct{}{}:
		c.ordinaryHeld = true
		return true
	default:
		return false
	}
}

// Idle reserve sockets are released promptly, so page loads and sign-in probes
// cannot occupy all console slots for the normal keep-alive interval.
func (l *cappedListener) state(conn net.Conn, state http.ConnState) {
	if state != http.StateIdle {
		return
	}
	c, ok := networking.RawConnection(conn).(*cappedConn)
	if !ok {
		return
	}
	c.mu.Lock()
	ordinary := c.ordinaryHeld
	c.mu.Unlock()
	if !ordinary {
		_ = conn.Close()
	}
}
