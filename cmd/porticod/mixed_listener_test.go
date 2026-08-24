package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type temporaryAcceptError struct{}

func (temporaryAcceptError) Error() string   { return "temporary accept failure" }
func (temporaryAcceptError) Timeout() bool   { return false }
func (temporaryAcceptError) Temporary() bool { return true }

type scriptedAcceptResult struct {
	connection net.Conn
	err        error
}

type scriptedListener struct {
	results chan scriptedAcceptResult
	closed  chan struct{}
}

func newScriptedListener() *scriptedListener {
	return &scriptedListener{results: make(chan scriptedAcceptResult, 8), closed: make(chan struct{})}
}

func (l *scriptedListener) Accept() (net.Conn, error) {
	select {
	case result := <-l.results:
		return result.connection, result.err
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *scriptedListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}
func (l *scriptedListener) Addr() net.Addr { return dummyAddr("127.0.0.1:32500") }

func TestProtocolMuxRetriesTemporaryAcceptFailure(t *testing.T) {
	listener := newScriptedListener()
	mux := newProtocolMux(listener)
	t.Cleanup(func() { _ = mux.Close() })
	listener.results <- scriptedAcceptResult{err: temporaryAcceptError{}}
	server, client := net.Pipe()
	listener.results <- scriptedAcceptResult{connection: server}
	defer client.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, _ := mux.http.Accept()
		accepted <- connection
	}()
	if _, err := client.Write([]byte("G")); err != nil {
		t.Fatalf("write classification byte: %v", err)
	}
	select {
	case connection := <-accepted:
		if connection == nil {
			t.Fatal("temporary accept failure closed the protocol mux")
		}
		_ = connection.Close()
	case <-time.After(time.Second):
		t.Fatal("protocol mux did not resume after temporary accept failure")
	}
}

func TestProtocolMuxPropagatesPermanentAcceptFailure(t *testing.T) {
	listener := newScriptedListener()
	mux := newProtocolMux(listener)
	permanent := errors.New("permanent accept failure")
	listener.results <- scriptedAcceptResult{err: permanent}
	if _, err := mux.http.Accept(); !errors.Is(err, permanent) {
		t.Fatalf("HTTP listener error = %v, want %v", err, permanent)
	}
	if _, err := mux.tls.Accept(); !errors.Is(err, permanent) {
		t.Fatalf("TLS listener error = %v, want %v", err, permanent)
	}
}

func TestProtocolMuxClassifiesPlaintextAndTLSConnections(t *testing.T) {
	mux := &protocolMux{done: make(chan struct{})}
	mux.http = newConnectionListener(dummyAddr("127.0.0.1:32500"), mux.done)
	mux.tls = newConnectionListener(dummyAddr("127.0.0.1:32500"), mux.done)

	tests := []struct {
		name   string
		prefix byte
		target *connectionListener
	}{
		{name: "http", prefix: 'G', target: mux.http},
		{name: "tls", prefix: 0x16, target: mux.tls},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, client := net.Pipe()
			defer client.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				connection, _ := test.target.Accept()
				accepted <- connection
			}()
			go mux.classify(server)
			if _, err := client.Write([]byte{test.prefix}); err != nil {
				t.Fatalf("write classification byte: %v", err)
			}
			select {
			case connection := <-accepted:
				defer connection.Close()
				buffer := make([]byte, 1)
				if _, err := io.ReadFull(connection, buffer); err != nil {
					t.Fatalf("read buffered classification byte: %v", err)
				}
				if buffer[0] != test.prefix {
					t.Fatalf("read prefix %x, want %x", buffer[0], test.prefix)
				}
			case <-time.After(time.Second):
				t.Fatal("classified connection was not offered to target listener")
			}
		})
	}
	diagnostics := mux.diagnostics()
	if diagnostics.ClassifiedHTTP != 1 || diagnostics.ClassifiedTLS != 1 {
		t.Fatalf("classification diagnostics = %#v", diagnostics)
	}
}

func TestProtocolMuxBoundsAndDrainsSilentPreAuthConnections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mux := newProtocolMuxWithLimits(listener, nil, 1, 1)
	first, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial first: %v", err)
	}
	defer first.Close()
	deadline := time.Now().Add(time.Second)
	for mux.diagnostics().Active != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	second, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial second: %v", err)
	}
	defer second.Close()
	_ = second.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := second.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection beyond pre-auth capacity remained open")
	}
	if mux.diagnostics().AdmissionRejected == 0 {
		t.Fatal("pre-auth saturation was not recorded")
	}
	started := time.Now()
	if err := mux.Close(); err != nil {
		t.Fatalf("close mux: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("mux close waited for the silent classification deadline")
	}
}

func TestProtocolAdmissionReleasesAfterTLSHandshakeBecomesActive(t *testing.T) {
	released := 0
	connection := &admittedConn{Conn: &stubConn{}, release: func() { released++ }}
	wrapped := tls.Client(&bufferedConn{Conn: connection, reader: bufio.NewReader(connection)}, &tls.Config{InsecureSkipVerify: true})
	protocolAdmissionConnState(wrapped, http.StateActive)
	protocolAdmissionConnState(wrapped, http.StateClosed)
	if released != 1 {
		t.Fatalf("admission release count = %d, want 1", released)
	}
}

func TestLocalPlaintextOnlyAllowsLocalAndPrivatePeers(t *testing.T) {
	handler := localPlaintextOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, remoteAddress := range []string{
		"127.0.0.1:50000",
		"[::1]:50000",
		"192.168.1.10:50000",
		"10.0.0.10:50000",
		"169.254.10.2:50000",
	} {
		t.Run(remoteAddress, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://portico.test/", nil)
			request.RemoteAddr = remoteAddress
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
			}
		})
	}
}

func TestLocalPlaintextOnlyRejectsPublicPeers(t *testing.T) {
	handler := localPlaintextOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("public plaintext request reached application handler")
	}))
	request := httptest.NewRequest(http.MethodGet, "http://portico.test/", nil)
	request.RemoteAddr = "203.0.113.10:50000"
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUpgradeRequired {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUpgradeRequired)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Fatalf("Content-Type = %q", contentType)
	}
}

func TestTLSAuthorityOnlyRequiresPublicSNIAndMatchingHost(t *testing.T) {
	handler := tlsAuthorityOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	tests := []struct {
		name       string
		remoteAddr string
		host       string
		serverName string
		wantStatus int
	}{
		{name: "public match", remoteAddr: "203.0.113.10:50000", host: "server.direct.getportico.tv:32500", serverName: "server.direct.getportico.tv", wantStatus: http.StatusNoContent},
		{name: "public missing sni", remoteAddr: "203.0.113.10:50000", host: "server.direct.getportico.tv:32500", wantStatus: http.StatusMisdirectedRequest},
		{name: "public mismatch", remoteAddr: "203.0.113.10:50000", host: "other.example:32500", serverName: "server.direct.getportico.tv", wantStatus: http.StatusMisdirectedRequest},
		{name: "local ip diagnostic", remoteAddr: "127.0.0.1:50000", host: "127.0.0.1:32500", wantStatus: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://"+test.host+"/", nil)
			request.Host = test.host
			request.RemoteAddr = test.remoteAddr
			request.TLS.ServerName = test.serverName
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

type dummyAddr string

func (a dummyAddr) Network() string { return "tcp" }
func (a dummyAddr) String() string  { return string(a) }

type stubConn struct{ net.Conn }

func (stubConn) Close() error                     { return nil }
func (stubConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (stubConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (stubConn) SetDeadline(time.Time) error      { return nil }
func (stubConn) SetReadDeadline(time.Time) error  { return nil }
func (stubConn) SetWriteDeadline(time.Time) error { return nil }
