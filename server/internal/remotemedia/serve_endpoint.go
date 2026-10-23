package remotemedia

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ServeEndpoint is private supervisor authority, never an owner-supplied URL or
// an Approval for an upstream provider. It admits one numeric loopback port.
// The source supervisor must independently enforce the provider's egress policy.
type ServeEndpoint struct {
	root          url.URL
	authorization string
	client        *http.Client
}

type serveReconnectKey struct{}

func NewServeEndpoint(port int, username, password string, connections int) (*ServeEndpoint, error) {
	if port < 1 || port > 65535 || connections < 1 || connections > 256 || username == "" || password == "" || len(username) > 1024 || len(password) > 8192 || strings.ContainsAny(username, ":\r\n\x00") || strings.ContainsAny(password, "\r\n\x00") {
		return nil, ErrPolicy
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	transport := &http.Transport{
		Proxy: nil, DisableCompression: true,
		MaxConnsPerHost: connections, MaxIdleConns: connections, MaxIdleConnsPerHost: connections, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 32 << 10,
		DialContext: func(ctx context.Context, network, target string) (net.Conn, error) {
			if network != "tcp" || target != address {
				return nil, ErrPolicy
			}
			check, ok := ctx.Value(serveReconnectKey{}).(func() error)
			if !ok {
				return nil, ErrPolicy
			}
			if err := check(); err != nil {
				return nil, err
			}
			conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp4", address)
			if err != nil {
				return nil, ErrUnavailable
			}
			if err := check(); err != nil {
				conn.Close()
				return nil, err
			}
			return &timedConn{Conn: conn, timeout: 15 * time.Second}, nil
		},
	}
	return &ServeEndpoint{
		root:          url.URL{Scheme: "http", Host: address, Path: "/"},
		authorization: "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password)),
		client:        &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func (s *ServeEndpoint) Close() { s.client.CloseIdleConnections() }

// ReadRange never retries, follows redirects, or falls back to an unconditioned
// GET. A served ETag is only an additional precondition, not provider identity.
// A separate observation/authority fence must surround this operation.
func (s *ServeEndpoint) ReadRange(ctx context.Context, relative, etag string, size, offset, length int64, reconnect func(context.Context) error) ([]byte, error) {
	if reconnect == nil {
		return nil, ErrPolicy
	}
	requestContext := ctx
	ctx = context.WithValue(ctx, serveReconnectKey{}, func() error { return reconnect(requestContext) })
	if relative == "" || !cleanDAVRelative(relative) || strings.Contains(relative, ":") || !serveETag(etag) || size <= 0 || offset < 0 || length < 1 || length > 1<<20 || offset >= size || length > size-offset {
		return nil, ErrRepresentationRange
	}
	target := s.root
	target.Path += relative
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, ErrPolicy
	}
	req.Header.Set("Authorization", s.authorization)
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "Portico/1")
	req.Header.Set("If-Match", etag)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrObservationChanged) {
			return nil, ErrObservationChanged
		}
		return nil, boundRequestError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPreconditionFailed || resp.StatusCode == http.StatusNotFound {
		return nil, ErrObservationChanged
	}
	if resp.StatusCode != http.StatusPartialContent {
		return nil, safeResponseError(resp)
	}
	start, end, total, err := parseRepresentationRange(resp.Header.Get("Content-Range"))
	if err != nil || start != offset || end != offset+length-1 || total != size || resp.ContentLength != length || resp.Header.Get("Content-Encoding") != "" || resp.Header.Get("ETag") != etag {
		return nil, ErrRepresentationRange
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, length+1))
	if err != nil || int64(len(data)) != length {
		return nil, ErrRepresentationRange
	}
	return data, nil
}

func serveETag(tag string) bool {
	if len(tag) < 2 || len(tag) > 4096 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return false
	}
	for _, c := range []byte(tag[1 : len(tag)-1]) {
		if c < 0x21 || c == '"' || c == 0x7f {
			return false
		}
	}
	return true
}
