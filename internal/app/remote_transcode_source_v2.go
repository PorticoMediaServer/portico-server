package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/ffmpegsupervisor"
)

const (
	remoteTranscodeStartupTimeout = 25 * time.Second
	remoteTranscodeReadTimeout    = 30 * time.Second
)

// remoteTranscodeSourceRequest contains the only HTTP metadata permitted to
// cross into a planned transcode. Authentication and cookie headers are
// intentionally not representable here. SourceUserAgent is an administrator-
// owned Live TV compatibility hint, not a client-supplied header.
type remoteTranscodeSourceRequest struct {
	URL            string
	UserAgent      string
	AllowHDHomeRun bool
	ReadTimeout    time.Duration
}

func remoteTranscodeRequestForItem(item MediaItem, sourceURL string) (remoteTranscodeSourceRequest, bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return remoteTranscodeSourceRequest{}, false, nil
	}
	request := remoteTranscodeSourceRequest{
		URL:            parsed.String(),
		UserAgent:      item.SourceUserAgent,
		AllowHDHomeRun: hlsTranscodeAllowsHDHomeRunLAN(item),
	}
	if request.AllowHDHomeRun {
		_, err = validateHDHomeRunURL(request.URL)
	} else {
		_, err = validateExternalURL(request.URL)
	}
	if err != nil {
		return remoteTranscodeSourceRequest{}, true, classifyRemoteTranscodeError(err)
	}
	if parsed.User != nil {
		return remoteTranscodeSourceRequest{}, true, &playbackStorageError{Kind: playbackStorageErrorTransient, Consumer: playbackStorageTranscode, Operation: "open remote source", Cause: errors.New("credentials in source URLs are not supported")}
	}
	return request, true, nil
}

// remoteTranscodeSource is consumed directly by os/exec's stdin copier. There
// is no whole-file buffer and no independently-lived feeder goroutine. Closing
// it cancels both the HTTP exchange and any blocked response-body read.
type remoteTranscodeSource struct {
	body    io.ReadCloser
	cancel  context.CancelFunc
	timeout time.Duration

	mu     sync.Mutex
	err    error
	closed bool
}

func openRemoteTranscodeSource(ctx context.Context, request remoteTranscodeSourceRequest) (*remoteTranscodeSource, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var (
		validated *url.URL
		err       error
	)
	if request.AllowHDHomeRun {
		validated, err = validateHDHomeRunURL(request.URL)
	} else {
		validated, err = validateExternalURL(request.URL)
	}
	if err != nil {
		return nil, classifyRemoteTranscodeError(err)
	}
	if validated.User != nil {
		return nil, &playbackStorageError{Kind: playbackStorageErrorTransient, Consumer: playbackStorageTranscode, Operation: "open remote source", Cause: errors.New("credentials in source URLs are not supported")}
	}

	requestCtx, cancel := context.WithCancel(ctx)
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodGet, validated.String(), nil)
	if err != nil {
		cancel()
		return nil, classifyRemoteTranscodeError(err)
	}
	httpRequest.Header.Set("Accept", "*/*")
	httpRequest.Header.Set("Accept-Encoding", "identity")
	if userAgent := safeRemoteTranscodeUserAgent(request.UserAgent); userAgent != "" {
		httpRequest.Header.Set("User-Agent", userAgent)
	}

	base := liveTVHTTPClientForContext(ctx)
	if request.AllowHDHomeRun && ctx.Value(liveTVHTTPClientContextKey{}) == nil {
		base = hdhomerunHTTPClient()
	}
	client := *base
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many source redirects")
		}
		var redirectErr error
		if request.AllowHDHomeRun {
			_, redirectErr = validateHDHomeRunURL(next.URL.String())
		} else {
			_, redirectErr = validateExternalURL(next.URL.String())
		}
		if redirectErr != nil || next.URL.User != nil {
			return errors.New("source redirect is not allowed")
		}
		// Go copies ordinary headers across redirects. Strip credentials even
		// if a future caller adds them to the initial request by mistake.
		next.Header.Del("Authorization")
		next.Header.Del("Proxy-Authorization")
		next.Header.Del("Cookie")
		next.Header.Del("Referer")
		return nil
	}
	startupDone := make(chan struct{})
	startupTimer := time.AfterFunc(remoteTranscodeStartupTimeout, func() {
		defer close(startupDone)
		cancel()
	})
	response, err := client.Do(httpRequest)
	if !startupTimer.Stop() {
		<-startupDone
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, &playbackStorageError{Kind: playbackStorageErrorStalled, Consumer: playbackStorageTranscode, Operation: "open remote source", Cause: errors.New("remote source startup timed out")}
	}
	if err != nil {
		cancel()
		return nil, classifyRemoteTranscodeError(err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		cancel()
		kind := playbackStorageErrorTransient
		if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
			kind = playbackStorageErrorOffline
		}
		return nil, &playbackStorageError{Kind: kind, Consumer: playbackStorageTranscode, Operation: "open remote source", Cause: fmt.Errorf("remote source returned HTTP %d", response.StatusCode)}
	}
	timeout := request.ReadTimeout
	if timeout <= 0 {
		timeout = remoteTranscodeReadTimeout
	}
	return &remoteTranscodeSource{body: response.Body, cancel: cancel, timeout: timeout}, nil
}

func safeRemoteTranscodeUserAgent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 512 || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}

func classifyRemoteTranscodeError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	classified := classifyPlaybackStorageError(playbackStorageTranscode, "read remote source", err)
	if typed, ok := classified.(*playbackStorageError); ok {
		return &playbackStorageError{Kind: typed.Kind, Consumer: typed.Consumer, Operation: typed.Operation, Cause: errors.New("remote transport failure")}
	}
	return &playbackStorageError{Kind: playbackStorageErrorTransient, Consumer: playbackStorageTranscode, Operation: "read remote source", Cause: errors.New("remote transport failure")}
}

func (s *remoteTranscodeSource) Read(buffer []byte) (int, error) {
	if s == nil || s.body == nil {
		return 0, io.ErrClosedPipe
	}
	// Arm the timer only while the transport is inside Read. Backpressure from
	// FFmpeg between calls therefore cannot be mistaken for a network stall.
	timerDone := make(chan struct{})
	timer := time.AfterFunc(s.timeout, func() {
		defer close(timerDone)
		s.fail(errStorageIOStalled)
	})
	n, err := s.body.Read(buffer)
	if !timer.Stop() {
		<-timerDone
		s.mu.Lock()
		stored := s.err
		s.mu.Unlock()
		if stored != nil {
			return n, stored
		}
	}
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		s.fail(err)
		s.mu.Lock()
		err = s.err
		s.mu.Unlock()
	}
	return n, err
}

func (s *remoteTranscodeSource) fail(err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	if s.err == nil && !s.closed {
		s.err = classifyRemoteTranscodeError(err)
	}
	cancel := s.cancel
	body := s.body
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if body != nil {
		_ = body.Close()
	}
}

func (s *remoteTranscodeSource) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cancel := s.cancel
	body := s.body
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if body != nil {
		return body.Close()
	}
	return nil
}

func (s *remoteTranscodeSource) Err() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// supervisedRemoteExecFactoryV2 binds the HTTP body to pipe:0 and makes the
// supervisor generation the sole owner of both process and feeder lifetime.
func supervisedRemoteExecFactoryV2(sourceRequest remoteTranscodeSourceRequest, build func(context.Context) (*exec.Cmd, error)) transcodeProcessFactoryV2 {
	return supervisedReaderExecFactoryV2(func(ctx context.Context) (*remoteTranscodeSource, error) {
		return openRemoteTranscodeSource(ctx, sourceRequest)
	}, build)
}

func supervisedReaderExecFactoryV2(open func(context.Context) (*remoteTranscodeSource, error), build func(context.Context) (*exec.Cmd, error)) transcodeProcessFactoryV2 {
	return func(ctx context.Context) (ffmpegsupervisor.Process, error) {
		if open == nil {
			return nil, errors.New("remote source opener is required")
		}
		source, err := open(ctx)
		if err != nil {
			return nil, err
		}
		cmd, err := build(ctx)
		if err != nil {
			_ = source.Close()
			return nil, err
		}
		if cmd == nil {
			_ = source.Close()
			return nil, errors.New("ffmpeg command builder returned nil")
		}
		cmd.Stdin = source
		if err := cmd.Start(); err != nil {
			_ = source.Close()
			return nil, err
		}
		return &supervisedRemoteExecProcessV2{cmd: cmd, source: source}, nil
	}
}

type supervisedRemoteExecProcessV2 struct {
	cmd    *exec.Cmd
	source *remoteTranscodeSource
}

func (p *supervisedRemoteExecProcessV2) Wait() error {
	err := p.cmd.Wait()
	feedErr := p.source.Err()
	_ = p.source.Close()
	if feedErr != nil {
		return feedErr
	}
	return err
}

func (p *supervisedRemoteExecProcessV2) Interrupt() error {
	_ = p.source.Close()
	if p.cmd == nil || p.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return p.cmd.Process.Signal(os.Interrupt)
}

func (p *supervisedRemoteExecProcessV2) Kill() error {
	_ = p.source.Close()
	if p.cmd == nil || p.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return p.cmd.Process.Kill()
}
