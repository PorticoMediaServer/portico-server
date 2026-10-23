package remotemedia

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const serveListingProperties = `<d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/><d:getetag/><d:getcontentlength/><d:getlastmodified/></d:prop></d:propfind>`

// ListSnapshot reads one complete Depth1 response, capped at 4 MiB by the DAV
// parser. It neither requests allprop nor pretends rclone supports sync tokens.
// Nothing is returned after a truncated, over-bound or invalid response. The
// source owner stages this result privately before keyset pages become visible.
func (s *ServeEndpoint) ListSnapshot(ctx context.Context, relative string, reconnect func(context.Context) error) (DAVPage, error) {
	if reconnect == nil {
		return DAVPage{}, ErrPolicy
	}
	requestContext := ctx
	ctx = context.WithValue(ctx, serveReconnectKey{}, func() error { return reconnect(requestContext) })
	if !cleanDAVRelative(relative) || strings.Contains(relative, ":") {
		return DAVPage{}, ErrPolicy
	}
	target := s.root
	target.Path += relative
	target.Path = strings.TrimSuffix(target.Path, "/") + "/"
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", target.String(), strings.NewReader(serveListingProperties))
	if err != nil {
		return DAVPage{}, ErrPolicy
	}
	req.Header.Set("Authorization", s.authorization)
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrObservationChanged) {
			return DAVPage{}, ErrObservationChanged
		}
		return DAVPage{}, boundRequestError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return DAVPage{}, safeResponseError(resp)
	}
	if resp.Header.Get("Content-Encoding") != "" {
		return DAVPage{}, ErrDAVResponse
	}
	d := DAV{config: DAVConfig{Root: s.root.String()}}
	return d.parseResponse(resp.Body, relative, false, false)
}
