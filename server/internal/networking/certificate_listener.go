package networking

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"portico.local/server/internal/supervise"
	"sync"
	"time"
)

// DirectListener serves ordinary publicly trusted TLS on the server's existing
// port, while preserving the private-LAN HTTP bootstrap/recovery transport.
// Peer admission is not authentication: downstream pairing, route proofs and
// session authorization still apply. Public peers must use TLS.
// Classification and total accepted sockets are bounded separately.
type DirectListener struct {
	net.Listener
	config      *tls.Config
	ready       chan net.Conn
	done        chan struct{}
	classify    chan struct{}
	connections chan struct{}
	once        sync.Once
	mu          sync.Mutex
	pending     map[*countedTLSConn]struct{}
	err         error
}

func NewDirectListener(inner net.Listener, manager *CertificateManager) (*DirectListener, error) {
	return newDirectListener(inner, manager, nil, nil)
}

// NewDirectListenerWithCustom serves an owner-supplied certificate for its
// domain in front of the Hosted manager. A nil manager with a configured
// custom certificate still serves (unclaimed / Direct Sign-In); other names
// behave exactly as before.
func NewDirectListenerWithCustom(inner net.Listener, manager *CertificateManager, custom *CustomCertificate) (*DirectListener, error) {
	return newDirectListener(inner, manager, nil, custom)
}

// NewDirectListenerWithTLSConfig lets an embedding server supply its own
// certificate source while keeping the same socket classifier and ALPN path.
// The caller retains responsibility for the certificate's trust policy.
func NewDirectListenerWithTLSConfig(inner net.Listener, config *tls.Config) (*DirectListener, error) {
	if config == nil {
		return nil, ErrInvalid
	}
	return newDirectListener(inner, nil, config, nil)
}

func newDirectListener(inner net.Listener, manager *CertificateManager, override *tls.Config, custom *CustomCertificate) (*DirectListener, error) {
	if inner == nil {
		return nil, ErrInvalid
	}
	l := &DirectListener{Listener: inner, ready: make(chan net.Conn, 64), done: make(chan struct{}), classify: make(chan struct{}, 64), connections: make(chan struct{}, 256), pending: map[*countedTLSConn]struct{}{}}
	getCertificate := func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return nil, ErrUnavailable }
	if manager != nil {
		getCertificate = manager.GetCertificate
	}
	if custom != nil {
		managerGet := getCertificate
		getCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if cert, e := custom.Get(hello); e == nil && cert != nil {
				return cert, nil
			} else if hello != nil && custom.Serves(hello.ServerName) {
				// The name is the custom certificate's, even though it failed
				// to load: fail here rather than falling through to a manager
				// that cannot serve it either.
				return nil, errCustomCertificate
			}
			return managerGet(hello)
		}
	}
	l.config = &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: getCertificate, NextProtos: []string{"h2", "http/1.1"}}
	if override != nil {
		l.config = override.Clone()
		if l.config.MinVersion < tls.VersionTLS12 {
			l.config.MinVersion = tls.VersionTLS12
		}
		l.config.NextProtos = []string{"h2", "http/1.1"}
	}
	// Resumed sessions must still be fenced by live claim authority. Go rotates
	// in-memory ticket keys automatically; no ticket key is persisted. A custom
	// certificate is not claim-fenced.
	if manager != nil {
		l.config.VerifyConnection = func(state tls.ConnectionState) error {
			// A58: a full handshake already ran GetCertificate (and its
			// authority fence) moments ago; only a resumed session skipped it.
			if !state.DidResume {
				return nil
			}
			if custom != nil && custom.Serves(state.ServerName) {
				return nil
			}
			_, e := manager.GetCertificate(&tls.ClientHelloInfo{ServerName: state.ServerName})
			return e
		}
	}
	supervise.Go("networking.direct-listener.accept", func() { l.acceptLoop() })
	return l, nil
}
func (l *DirectListener) acceptLoop() {
	for {
		raw, e := l.Listener.Accept()
		if e != nil {
			l.mu.Lock()
			if l.err == nil {
				l.err = e
			}
			l.mu.Unlock()
			l.Close()
			return
		}
		select {
		case l.connections <- struct{}{}:
		default:
			raw.Close()
			continue
		}
		c := &countedTLSConn{Conn: raw, closed: func() { <-l.connections }}
		l.mu.Lock()
		l.pending[c] = struct{}{}
		l.mu.Unlock()
		select {
		case <-l.done:
			l.forget(c)
			c.Close()
			return
		default:
		}
		select {
		case l.classify <- struct{}{}:
			supervise.Go("networking.direct-listener.classify", func() { l.classifyConnection(c) })
		default:
			l.forget(c)
			c.Close()
		}
	}
}
func (l *DirectListener) forget(c *countedTLSConn) { l.mu.Lock(); delete(l.pending, c); l.mu.Unlock() }
func (l *DirectListener) classifyConnection(c *countedTLSConn) {
	defer func() { <-l.classify }()
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	reader := bufio.NewReaderSize(c, 1024)
	first, e := reader.Peek(1)
	if e != nil {
		l.forget(c)
		c.Close()
		return
	}
	buffered := &bufferedTLSConn{Conn: c, reader: reader, owner: c}
	var conn net.Conn
	if first[0] == 0x16 {
		conn = tls.Server(buffered, l.config)
	} else if localHTTPConnection(c.RemoteAddr()) && first[0] >= 'A' && first[0] <= 'Z' {
		conn = buffered
	} else {
		l.forget(c)
		c.Close()
		return
	}
	c.SetReadDeadline(time.Time{})
	select {
	case l.ready <- conn:
	case <-l.done:
		l.forget(c)
		c.Close()
	}
}

// Classify the socket peer, never Host, Forwarded or X-Forwarded-For. In
// particular a public peer cannot claim a LAN route with an HTTP header. A
// reverse proxy remains a trust boundary and must not expose this LAN route.
func localHTTPConnection(address net.Addr) bool {
	peer, ok := address.(*net.TCPAddr)
	if !ok || peer == nil {
		return false
	}
	ip := peer.IP
	return ip != nil && !ip.IsUnspecified() && !ip.IsMulticast() &&
		(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}
func (l *DirectListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, l.closedError()
	case conn := <-l.ready:
		var raw *countedTLSConn
		switch c := conn.(type) {
		case *bufferedTLSConn:
			raw = c.owner
		case *tls.Conn:
			if buffered, ok := c.NetConn().(*bufferedTLSConn); ok {
				raw = buffered.owner
			}
		}
		if raw != nil {
			l.forget(raw)
		}
		select {
		case <-l.done:
			conn.Close()
			return nil, l.closedError()
		default:
			return conn, nil
		}
	}
}
func (l *DirectListener) closedError() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return l.err
	}
	return net.ErrClosed
}
func (l *DirectListener) Close() error {
	var result error
	l.once.Do(func() {
		close(l.done)
		result = l.Listener.Close()
		l.mu.Lock()
		pending := make([]*countedTLSConn, 0, len(l.pending))
		for c := range l.pending {
			pending = append(pending, c)
		}
		l.mu.Unlock()
		for _, c := range pending {
			c.Close()
			l.forget(c)
		}
	})
	if errors.Is(result, net.ErrClosed) {
		return nil
	}
	return result
}

type countedTLSConn struct {
	net.Conn
	once   sync.Once
	closed func()
}

func (c *countedTLSConn) Close() error { e := c.Conn.Close(); c.once.Do(c.closed); return e }

type bufferedTLSConn struct {
	net.Conn
	reader *bufio.Reader
	owner  *countedTLSConn
}

func (c *bufferedTLSConn) Read(b []byte) (int, error) { return c.reader.Read(b) }
