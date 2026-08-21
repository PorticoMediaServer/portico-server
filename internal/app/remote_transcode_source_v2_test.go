package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

type remoteTranscodeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f remoteTranscodeRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func remoteTranscodeTestContext(client *http.Client) context.Context {
	return context.WithValue(context.Background(), liveTVHTTPClientContextKey{}, client)
}

func TestRemoteTranscodeSourceStreamsWithoutForwardingSecrets(t *testing.T) {
	var captured http.Header
	client := &http.Client{Transport: remoteTranscodeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		captured = request.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("media-payload")), Request: request}, nil
	})}
	source, err := openRemoteTranscodeSource(remoteTranscodeTestContext(client), remoteTranscodeSourceRequest{
		URL: "https://media.example.test/video?id=opaque", UserAgent: "Portico source adapter/1",
	})
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	payload, err := io.ReadAll(source)
	if err != nil || string(payload) != "media-payload" {
		t.Fatalf("read = %q, %v", payload, err)
	}
	if captured.Get("User-Agent") != "Portico source adapter/1" || captured.Get("Accept") != "*/*" {
		t.Fatalf("safe source headers = %#v", captured)
	}
	for _, forbidden := range []string{"Authorization", "Proxy-Authorization", "Cookie"} {
		if captured.Get(forbidden) != "" {
			t.Fatalf("secret header %s was forwarded", forbidden)
		}
	}
}

func TestRemoteTranscodeSourceRejectsCredentialURLAndUnsafeRedirect(t *testing.T) {
	client := &http.Client{Transport: remoteTranscodeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"http://127.0.0.1/private"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})}
	for _, rawURL := range []string{"https://user:password@media.example.test/video", "https://media.example.test/redirect"} {
		_, err := openRemoteTranscodeSource(remoteTranscodeTestContext(client), remoteTranscodeSourceRequest{URL: rawURL})
		if err == nil {
			t.Fatalf("unsafe source %q was accepted", rawURL)
		}
		if strings.Contains(err.Error(), rawURL) || strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "127.0.0.1") {
			t.Fatalf("public error leaked source details: %v", err)
		}
		var storageErr *playbackStorageError
		if !errors.As(err, &storageErr) {
			t.Fatalf("error is not typed storage failure: %T", err)
		}
	}
}

type remoteTranscodeBlockingBody struct {
	once   sync.Once
	closed chan struct{}
}

func (b *remoteTranscodeBlockingBody) Read([]byte) (int, error) {
	<-b.closed
	return 0, context.Canceled
}

func (b *remoteTranscodeBlockingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

func TestRemoteTranscodeSourceClassifiesStalledBodyAndCloses(t *testing.T) {
	body := &remoteTranscodeBlockingBody{closed: make(chan struct{})}
	client := &http.Client{Transport: remoteTranscodeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: request}, nil
	})}
	source, err := openRemoteTranscodeSource(remoteTranscodeTestContext(client), remoteTranscodeSourceRequest{
		URL: "https://media.example.test/stalled", ReadTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	started := time.Now()
	_, err = source.Read(make([]byte, 1))
	if !errors.Is(err, errPlaybackStorageStalled) {
		t.Fatalf("read error = %v, want stalled storage error", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("stalled read did not cancel promptly")
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("stalled response body was not closed")
	}
}

func TestSupervisedRemoteExecFactoryJoinsSourceLifecycle(t *testing.T) {
	body := &remoteTranscodeBlockingBody{closed: make(chan struct{})}
	client := &http.Client{Transport: remoteTranscodeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: request}, nil
	})}
	ctx := remoteTranscodeTestContext(client)
	factory := supervisedRemoteExecFactoryV2(remoteTranscodeSourceRequest{
		URL: "https://media.example.test/endless", ReadTimeout: time.Minute,
	}, func(processCtx context.Context) (*exec.Cmd, error) {
		return exec.CommandContext(processCtx, "/bin/sh", "-c", "cat >/dev/null"), nil
	})
	process, err := factory(ctx)
	if err != nil {
		t.Fatalf("start process: %v", err)
	}
	if err := process.Kill(); err != nil {
		t.Fatalf("kill process: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("process wait did not join stdin feeder")
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("process shutdown did not close remote source")
	}
}
