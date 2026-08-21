package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	protocolClassificationTimeout = 3 * time.Second
	protocolClassificationLimit   = 256
	protocolConnectionLimit       = 1024
)

type protocolMuxDiagnostics struct {
	Accepted               uint64
	Active                 uint64
	PeakActive             uint64
	ClassifiedHTTP         uint64
	ClassifiedTLS          uint64
	ClassificationFailed   uint64
	ClassificationTimedOut uint64
	AdmissionRejected      uint64
}

// protocolMux owns one TCP socket and dispatches accepted connections to the
// HTTP or TLS server. Classification happens concurrently so a client that
// connects without sending data cannot block new connections from being
// accepted.
type protocolMux struct {
	listener               net.Listener
	done                   chan struct{}
	close                  sync.Once
	http                   *connectionListener
	tls                    *connectionListener
	classifySlots          chan struct{}
	connectionSlots        chan struct{}
	classifierMu           sync.Mutex
	classifiers            map[net.Conn]struct{}
	classifierWG           sync.WaitGroup
	observer               func(protocolMuxDiagnostics)
	accepted               atomic.Uint64
	active                 atomic.Uint64
	peakActive             atomic.Uint64
	classifiedHTTP         atomic.Uint64
	classifiedTLS          atomic.Uint64
	classificationFailed   atomic.Uint64
	classificationTimedOut atomic.Uint64
	admissionRejected      atomic.Uint64
}

type connectionListener struct {
	addr   net.Addr
	conns  chan net.Conn
	done   chan struct{}
	parent <-chan struct{}
	close  sync.Once
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

type admittedConn struct {
	net.Conn
	close   sync.Once
	release func()
}

func (c *admittedConn) Close() error {
	err := c.Conn.Close()
	c.ReleaseAdmission()
	return err
}

func (c *admittedConn) ReleaseAdmission() { c.close.Do(c.release) }

func releaseProtocolAdmission(connection net.Conn) {
	for connection != nil {
		if admitted, ok := connection.(interface{ ReleaseAdmission() }); ok {
			admitted.ReleaseAdmission()
			return
		}
		switch wrapped := connection.(type) {
		case *tls.Conn:
			connection = wrapped.NetConn()
		case *bufferedConn:
			connection = wrapped.Conn
		default:
			return
		}
	}
}

func protocolAdmissionConnState(connection net.Conn, state http.ConnState) {
	switch state {
	case http.StateActive, http.StateHijacked, http.StateClosed:
		releaseProtocolAdmission(connection)
	}
}

func (c *bufferedConn) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}

func newProtocolMux(listener net.Listener) *protocolMux {
	return newProtocolMuxWithObserver(listener, nil)
}

func newProtocolMuxWithObserver(listener net.Listener, observer func(protocolMuxDiagnostics)) *protocolMux {
	return newProtocolMuxWithLimits(listener, observer, protocolClassificationLimit, protocolConnectionLimit)
}

func newProtocolMuxWithLimits(listener net.Listener, observer func(protocolMuxDiagnostics), classificationLimit, connectionLimit int) *protocolMux {
	classificationLimit = max(1, classificationLimit)
	connectionLimit = max(1, connectionLimit)
	mux := &protocolMux{
		listener: listener, done: make(chan struct{}),
		classifySlots:   make(chan struct{}, classificationLimit),
		connectionSlots: make(chan struct{}, connectionLimit),
		classifiers:     map[net.Conn]struct{}{},
		observer:        observer,
	}
	mux.http = newConnectionListener(listener.Addr(), mux.done)
	mux.tls = newConnectionListener(listener.Addr(), mux.done)
	go mux.accept()
	return mux
}

func newConnectionListener(addr net.Addr, parent <-chan struct{}) *connectionListener {
	return &connectionListener{
		addr:   addr,
		conns:  make(chan net.Conn, protocolConnectionLimit),
		done:   make(chan struct{}),
		parent: parent,
	}
}

func (m *protocolMux) accept() {
	defer close(m.done)
	for {
		connection, err := m.listener.Accept()
		if err != nil {
			return
		}
		m.accepted.Add(1)
		select {
		case m.connectionSlots <- struct{}{}:
			current := m.active.Add(1)
			for peak := m.peakActive.Load(); current > peak && !m.peakActive.CompareAndSwap(peak, current); peak = m.peakActive.Load() {
			}
			tracked := &admittedConn{Conn: connection, release: func() {
				<-m.connectionSlots
				m.active.Add(^uint64(0))
			}}
			select {
			case m.classifySlots <- struct{}{}:
				m.classifierMu.Lock()
				m.classifiers[tracked] = struct{}{}
				m.classifierWG.Add(1)
				m.classifierMu.Unlock()
				go func() {
					defer func() {
						<-m.classifySlots
						m.classifierMu.Lock()
						delete(m.classifiers, tracked)
						m.classifierMu.Unlock()
						m.classifierWG.Done()
					}()
					m.classify(tracked)
				}()
			default:
				m.recordAdmissionRejection()
				_ = tracked.Close()
			}
		default:
			m.recordAdmissionRejection()
			_ = connection.Close()
		}
	}
}

func (m *protocolMux) recordAdmissionRejection() {
	count := m.admissionRejected.Add(1)
	if m.observer != nil && (count == 1 || count&(count-1) == 0) {
		m.observer(m.diagnostics())
	}
}

func (m *protocolMux) classify(connection net.Conn) {
	_ = connection.SetReadDeadline(time.Now().Add(protocolClassificationTimeout))
	reader := bufio.NewReaderSize(connection, 4096)
	prefix, err := reader.Peek(1)
	_ = connection.SetReadDeadline(time.Time{})
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			m.classificationTimedOut.Add(1)
		} else {
			m.classificationFailed.Add(1)
		}
		_ = connection.Close()
		return
	}
	target := m.http
	if prefix[0] == 0x16 { // TLS handshake record.
		target = m.tls
		m.classifiedTLS.Add(1)
	} else {
		m.classifiedHTTP.Add(1)
	}
	if !target.offer(&bufferedConn{Conn: connection, reader: reader}) {
		m.classificationFailed.Add(1)
	}
}

func (m *protocolMux) diagnostics() protocolMuxDiagnostics {
	return protocolMuxDiagnostics{
		Accepted: m.accepted.Load(), Active: m.active.Load(), PeakActive: m.peakActive.Load(),
		ClassifiedHTTP: m.classifiedHTTP.Load(), ClassifiedTLS: m.classifiedTLS.Load(),
		ClassificationFailed: m.classificationFailed.Load(), ClassificationTimedOut: m.classificationTimedOut.Load(),
		AdmissionRejected: m.admissionRejected.Load(),
	}
}

func (m *protocolMux) Close() error {
	var err error
	m.close.Do(func() {
		err = m.listener.Close()
		m.classifierMu.Lock()
		pending := make([]net.Conn, 0, len(m.classifiers))
		for connection := range m.classifiers {
			pending = append(pending, connection)
		}
		m.classifierMu.Unlock()
		for _, connection := range pending {
			_ = connection.Close()
		}
		m.classifierWG.Wait()
		_ = m.http.Close()
		_ = m.tls.Close()
	})
	return err
}

func (l *connectionListener) offer(connection net.Conn) bool {
	select {
	case l.conns <- connection:
		return true
	case <-l.done:
		_ = connection.Close()
		return false
	case <-l.parent:
		_ = connection.Close()
		return false
	}
}

func (l *connectionListener) Accept() (net.Conn, error) {
	select {
	case connection := <-l.conns:
		return connection, nil
	case <-l.done:
		return nil, net.ErrClosed
	case <-l.parent:
		return nil, net.ErrClosed
	}
}

func (l *connectionListener) Close() error {
	l.close.Do(func() {
		close(l.done)
		for {
			select {
			case connection := <-l.conns:
				_ = connection.Close()
			default:
				return
			}
		}
	})
	return nil
}

func (l *connectionListener) Addr() net.Addr { return l.addr }

type certificateLoader struct {
	mu     sync.RWMutex
	loader func(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

func (l *certificateLoader) Set(loader func(*tls.ClientHelloInfo) (*tls.Certificate, error)) {
	l.mu.Lock()
	l.loader = loader
	l.mu.Unlock()
}

func (l *certificateLoader) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	l.mu.RLock()
	loader := l.loader
	l.mu.RUnlock()
	if loader == nil {
		return nil, errors.New("Portico TLS certificate is not ready")
	}
	return loader(hello)
}

func localPlaintextOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localPeerAddress(r.RemoteAddr) {
			w.Header().Set("Connection", "close")
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusUpgradeRequired)
			_, _ = w.Write([]byte(`{"type":"https://portico.media/problems/tls-required","title":"TLS required","status":426,"code":"tls_required","detail":"Public Portico connections must use HTTPS."}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tlsAuthorityOnly prevents a public client from reaching the application
// through an unnamed TLS session or with an HTTP authority that differs from
// the SNI name used for the handshake. Local IP-based diagnostics may omit SNI.
func tlsAuthorityOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverName := ""
		if r.TLS != nil {
			serverName = normalizedAuthorityHost(r.TLS.ServerName)
		}
		requestHost := normalizedAuthorityHost(r.Host)
		if (!localPeerAddress(r.RemoteAddr) && serverName == "") || (serverName != "" && requestHost != serverName) {
			w.Header().Set("Connection", "close")
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusMisdirectedRequest)
			_, _ = w.Write([]byte(`{"type":"https://portico.media/problems/tls-authority-mismatch","title":"TLS authority mismatch","status":421,"code":"tls_authority_mismatch","detail":"The HTTPS hostname must match the TLS server name."}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func normalizedAuthorityHost(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || strings.ContainsAny(value, "\r\n\t /\\") {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	return strings.Trim(strings.Trim(value, "[]"), ".")
}

func localPeerAddress(remoteAddress string) bool {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		host = remoteAddress
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast()
}

func shutdownServers(ctx context.Context, mux *protocolMux, servers ...*http.Server) error {
	_ = mux.Close()
	var result error
	for _, server := range servers {
		if server == nil {
			continue
		}
		if err := server.Shutdown(ctx); err != nil && result == nil {
			result = err
		}
	}
	return result
}
