package main

import (
	"log"
	"net"
	"portico.local/server/internal/networking"
	"sync"
	"sync/atomic"
	"time"
)

// The admission lanes protect handlers, not accepted connections. Every accepted
// connection is a goroutine and two buffers — call it thirty kilobytes — before
// any lane is consulted, and nothing bounded how many there could be except the
// descriptor limit. Ten times the target load is therefore ten times the memory,
// spent on connections that will be refused anyway.
//
// The cap is a listener, not a handler: a connection that is never accepted
// costs nothing at all. Past it the kernel's own listen backlog absorbs the
// burst, and past that the client sees a connection refusal — which is the
// honest answer when the server has no capacity left to explain itself with, and
// is what every HTTP client already retries.
//
// The number is a ceiling, not a working value. Two hundred viewers with a
// handful of connections each, plus browsers that open six per host, is a few
// hundred; 2,048 is several times that and about sixty megabytes of buffers.

// maximumConnections is how many accepted connections the server will hold.
const maximumConnections = 2048

// cappedListener bounds accepted connections.
type cappedListener struct {
	net.Listener
	slots chan struct{}
	// held is published so the diagnostics and the log can say how close to the
	// ceiling the server is running.
	held     atomic.Int64
	deferred atomic.Uint64
	warned   sync.Once
}

func capConnections(inner net.Listener, limit int) *cappedListener {
	if limit < 1 {
		limit = maximumConnections
	}
	l := &cappedListener{Listener: inner, slots: make(chan struct{}, limit)}
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
	case <-l.slots:
	default:
		// At the ceiling. Wait for a slot rather than accepting a connection there
		// is no room to serve: the backlog is a better queue than the heap.
		l.deferred.Add(1)
		l.warned.Do(func() {
			log.Printf("Connection ceiling of %d reached; further connections wait in the listen backlog", cap(l.slots))
		})
		<-l.slots
	}
	conn, err := l.Listener.Accept()
	if err != nil {
		l.slots <- struct{}{}
		return nil, err
	}
	l.held.Add(1)
	return &cappedConn{Conn: conn, listener: l}, nil
}

// Held reports how many connections are open, and how many accepts have had to
// wait for a slot.
func (l *cappedListener) Held() (int64, uint64) { return l.held.Load(), l.deferred.Load() }

// cappedConn returns its slot exactly once, whatever closes it.
type cappedConn struct {
	net.Conn
	listener *cappedListener
	once     sync.Once
}

func (c *cappedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() {
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
