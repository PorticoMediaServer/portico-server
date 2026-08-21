package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type dvrInputTransport struct {
	URL       string
	listener  net.Listener
	server    *http.Server
	transport *http.Transport
	cancel    context.CancelFunc
	done      chan struct{}
	once      sync.Once
}

func startDVRInputTransport(ctx context.Context, recordingID, leaseToken, upstreamURL string, resolver liveTVResolver, providerUserAgent ...string) (*dvrInputTransport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(recordingID) == "" || strings.TrimSpace(leaseToken) == "" {
		return nil, errors.New("DVR input lease is invalid")
	}
	approval, parsed, err := approveLiveTVEndpoint(upstreamURL, "dvr-stream")
	if err != nil {
		return nil, errors.New("DVR provider endpoint is not approved")
	}
	transportCtx, cancelTransport := context.WithCancel(ctx)
	client, err := newApprovedLiveTVHTTPClient(transportCtx, approval, resolver)
	if err != nil {
		cancelTransport()
		return nil, errors.New("DVR provider address is not approved")
	}
	// The ordinary provider client has a whole-request timeout suitable for
	// manifests. DVR bodies are intentionally long lived; dial, TLS and response
	// header deadlines remain enforced by its pinned transport while output
	// progress is bounded by the DVR watchdog.
	client.Timeout = 0
	transport, _ := client.Transport.(*http.Transport)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		cancelTransport()
		if transport != nil {
			transport.CloseIdleConnections()
		}
		return nil, errors.New("DVR input transport could not start")
	}
	capability := randomID("dvrinput")
	path := "/input/" + capability
	result := &dvrInputTransport{listener: listener, transport: transport, cancel: cancelTransport, done: make(chan struct{})}
	configuredUserAgent := ""
	if len(providerUserAgent) > 0 {
		configuredUserAgent = providerUserAgent[0]
	}
	handler := newDVRInputTransportHandler(path, transportCtx, parsed.String(), client, configuredUserAgent)
	result.server = &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 15 * time.Second}
	result.URL = "http://" + listener.Addr().String() + path
	go func() {
		defer close(result.done)
		_ = result.server.Serve(listener)
	}()
	go func() {
		select {
		case <-ctx.Done():
			result.Close()
		case <-result.done:
		}
	}()
	return result, nil
}

func newDVRInputTransportHandler(path string, transportCtx context.Context, providerURL string, client *http.Client, providerUserAgent string) http.Handler {
	if transportCtx == nil {
		transportCtx = context.Background()
	}
	providerUserAgent = effectiveLiveTVUserAgent(providerUserAgent)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != path || !requestPeerIsLoopback(r.RemoteAddr) {
			http.NotFound(w, r)
			return
		}
		requestCtx, cancelRequest := context.WithCancel(transportCtx)
		stopRequestCancel := context.AfterFunc(r.Context(), cancelRequest)
		defer func() {
			stopRequestCancel()
			cancelRequest()
		}()
		req, reqErr := http.NewRequestWithContext(requestCtx, http.MethodGet, providerURL, nil)
		if reqErr != nil {
			http.Error(w, "provider request failed", http.StatusBadGateway)
			return
		}
		req.Header.Set("Accept", "*/*")
		req.Header.Set("User-Agent", providerUserAgent)
		for _, header := range []string{"Range", "If-Range"} {
			if value := r.Header.Get(header); value != "" {
				req.Header.Set(header, value)
			}
		}
		resp, fetchErr := client.Do(req)
		if fetchErr != nil {
			http.Error(w, "provider stream unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
			if value := resp.Header.Get(header); value != "" {
				w.Header().Set(header, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.CopyBuffer(w, resp.Body, make([]byte, 64*1024))
	})
}

// startLiveTVInputTransport uses the same loopback-only capability boundary as
// DVR recording, but its lifetime is owned by a Live TV transcode session.
// Provider credentials therefore stay in the Server's HTTP client and never
// enter FFmpeg argv or the process table.
func startLiveTVInputTransport(ctx context.Context, channelID, upstreamURL string, resolver liveTVResolver, providerUserAgent ...string) (*dvrInputTransport, error) {
	return startDVRInputTransport(ctx, "live-tv:"+strings.TrimSpace(channelID), randomID("livelease"), upstreamURL, resolver, providerUserAgent...)
}

func (t *dvrInputTransport) Close() {
	if t == nil {
		return
	}
	t.once.Do(func() {
		t.cancel()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = t.server.Shutdown(shutdownCtx)
		_ = t.server.Close()
		_ = t.listener.Close()
		if t.transport != nil {
			t.transport.CloseIdleConnections()
		}
	})
}

func requestPeerIsLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
