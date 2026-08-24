package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type webDAVBackend struct {
	BaseURL            *url.URL
	Username, Password string
	Client             *http.Client
	Scheduler          *remoteStorageScheduler
}

func (w *webDAVBackend) Kind() string { return "webdav" }
func (w *webDAVBackend) client() *http.Client {
	client := http.Client{Timeout: 2 * time.Minute}
	if w.Client != nil {
		client = *w.Client
	}
	prior := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if w.BaseURL == nil || !sameWebDAVOrigin(req.URL, w.BaseURL) {
			return http.ErrUseLastResponse
		}
		if prior != nil {
			return prior(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &client
}

func sameWebDAVOrigin(left, right *url.URL) bool {
	return left != nil && right != nil && strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}
func (w *webDAVBackend) objectURL(object string) (*url.URL, error) {
	if w.BaseURL == nil || (w.BaseURL.Scheme != "https" && w.BaseURL.Scheme != "http") || w.BaseURL.Host == "" {
		return nil, errors.New("invalid WebDAV base URL")
	}
	u := *w.BaseURL
	if object != "" {
		p, err := normalizeRemoteObjectPath(object)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, p)
	}
	u.RawQuery = ""
	u.Fragment = ""
	return &u, nil
}
func (w *webDAVBackend) request(ctx context.Context, method, object string, body io.Reader) (*http.Request, error) {
	u, err := w.objectURL(object)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if w.Username != "" {
		req.SetBasicAuth(w.Username, w.Password)
	}
	return req, nil
}

type davMultiStatus struct {
	Responses []davResponse `xml:"response"`
	SyncToken string        `xml:"sync-token"`
}
type davResponse struct {
	Href      string `xml:"href"`
	Status    string `xml:"status"`
	PropStats []struct {
		Status string `xml:"status"`
		Prop   struct {
			ETag         string `xml:"getetag"`
			Length       int64  `xml:"getcontentlength"`
			Type         string `xml:"getcontenttype"`
			Modified     string `xml:"getlastmodified"`
			ResourceType struct {
				Collection *struct{} `xml:"collection"`
			} `xml:"resourcetype"`
		} `xml:"prop"`
	} `xml:"propstat"`
}

const webDAVBFSCursorPrefix = "webdav-bfs:"

type webDAVBFSCursor struct {
	Queue     []string `json:"queue"`
	Offset    int      `json:"offset,omitempty"`
	SyncToken string   `json:"syncToken,omitempty"`
}

type webDAVInventoryEntry struct {
	path       string
	collection bool
	deleted    bool
	object     storageObject
}

func (w *webDAVBackend) Stat(ctx context.Context, object string) (storageObject, error) {
	objectPath, err := normalizeRemoteObjectPath(object)
	if err != nil {
		return storageObject{}, err
	}
	operationCtx, release, err := w.Scheduler.acquireOperation(ctx, false)
	if err != nil {
		return storageObject{}, err
	}
	defer release()
	body := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:getetag/><d:getcontentlength/><d:getlastmodified/><d:getcontenttype/><d:resourcetype/></d:prop></d:propfind>`
	multi, status, err := w.doInventoryRequest(operationCtx, "PROPFIND", objectPath, "0", body)
	if err != nil {
		return storageObject{}, err
	}
	if status != http.StatusMultiStatus {
		return storageObject{}, fmt.Errorf("WebDAV object stat returned HTTP %d", status)
	}
	for _, entry := range w.inventoryEntries(multi) {
		if entry.path == objectPath && !entry.collection && !entry.deleted {
			return entry.object, nil
		}
	}
	return storageObject{}, errors.New("WebDAV object stat did not return the requested object")
}

func (w *webDAVBackend) Inventory(ctx context.Context, cursorOrSyncToken string, limit int) (storageInventoryPage, error) {
	if limit <= 0 {
		limit = 10000
	}
	operationCtx, release, err := w.Scheduler.acquireOperation(ctx, false)
	if err != nil {
		return storageInventoryPage{}, err
	}
	defer release()
	if strings.HasPrefix(cursorOrSyncToken, webDAVBFSCursorPrefix) || cursorOrSyncToken == "" {
		return w.inventoryFullBFS(operationCtx, cursorOrSyncToken, limit)
	}
	return w.inventoryDelta(operationCtx, cursorOrSyncToken, limit)
}

func (w *webDAVBackend) inventoryDelta(ctx context.Context, syncToken string, limit int) (storageInventoryPage, error) {
	body := `<?xml version="1.0"?><d:sync-collection xmlns:d="DAV:"><d:sync-token>` + webDAVXMLEscape(syncToken) + `</d:sync-token><d:sync-level>infinite</d:sync-level><d:prop><d:getetag/><d:getcontentlength/><d:getlastmodified/><d:getcontenttype/><d:resourcetype/></d:prop></d:sync-collection>`
	multi, status, err := w.doInventoryRequest(ctx, "REPORT", "", "1", body)
	if err != nil {
		return storageInventoryPage{}, err
	}
	if status != http.StatusMultiStatus {
		if status == http.StatusForbidden || status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented || status == http.StatusInsufficientStorage {
			return w.inventoryFullBFS(ctx, "", limit)
		}
		return storageInventoryPage{}, fmt.Errorf("WebDAV inventory returned HTTP %d", status)
	}
	entries := w.inventoryEntries(multi)
	if len(entries) > limit {
		// Sync-collection has no portable offset cursor. A large delta therefore
		// falls back to the bounded directory BFS rather than failing forever or
		// silently truncating changes.
		return w.inventoryFullBFS(ctx, "", limit)
	}
	page := storageInventoryPage{SyncToken: strings.TrimSpace(multi.SyncToken)}
	for _, entry := range entries {
		if entry.collection {
			continue
		}
		if entry.deleted {
			page.DeletedPaths = append(page.DeletedPaths, entry.path)
		} else {
			page.Objects = append(page.Objects, entry.object)
		}
	}
	return page, nil
}

func (w *webDAVBackend) inventoryFullBFS(ctx context.Context, encodedCursor string, limit int) (storageInventoryPage, error) {
	state := webDAVBFSCursor{Queue: []string{""}}
	if encodedCursor != "" {
		if len(encodedCursor) > remoteStorageCursorMaxBytes {
			return storageInventoryPage{}, fmt.Errorf("%w: oversized WebDAV cursor", errRemoteInventoryCursorInvalid)
		}
		if !strings.HasPrefix(encodedCursor, webDAVBFSCursorPrefix) {
			return storageInventoryPage{}, fmt.Errorf("%w: invalid WebDAV cursor", errRemoteInventoryCursorInvalid)
		}
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encodedCursor, webDAVBFSCursorPrefix))
		if err != nil || json.Unmarshal(raw, &state) != nil || len(state.Queue) == 0 || state.Offset < 0 {
			return storageInventoryPage{}, fmt.Errorf("%w: invalid WebDAV cursor", errRemoteInventoryCursorInvalid)
		}
		if len(state.Queue) > remoteStorageDirectoryQueueMax {
			return storageInventoryPage{}, fmt.Errorf("%w: oversized WebDAV directory queue", errRemoteInventoryCursorInvalid)
		}
	}
	directory := state.Queue[0]
	body := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:getetag/><d:getcontentlength/><d:getlastmodified/><d:getcontenttype/><d:resourcetype/></d:prop></d:propfind>`
	multi, status, err := w.doInventoryRequest(ctx, "PROPFIND", directory, "1", body)
	if err != nil {
		return storageInventoryPage{}, err
	}
	if status != http.StatusMultiStatus {
		return storageInventoryPage{}, fmt.Errorf("WebDAV inventory returned HTTP %d", status)
	}
	if token := strings.TrimSpace(multi.SyncToken); token != "" {
		state.SyncToken = token
	}
	entries := w.inventoryEntries(multi)
	if state.Offset > len(entries) {
		return storageInventoryPage{}, fmt.Errorf("%w: WebDAV directory changed during resume", errRemoteInventoryCursorInvalid)
	}
	end := min(len(entries), state.Offset+limit)
	page := storageInventoryPage{}
	queued := make(map[string]bool, len(state.Queue))
	for _, item := range state.Queue {
		queued[item] = true
	}
	for _, entry := range entries[state.Offset:end] {
		if entry.collection {
			if entry.path != directory && !queued[entry.path] {
				if len(state.Queue) >= remoteStorageDirectoryQueueMax {
					return storageInventoryPage{}, errors.New("WebDAV directory queue exceeded the inventory limit")
				}
				state.Queue = append(state.Queue, entry.path)
				queued[entry.path] = true
			}
			continue
		}
		if entry.deleted {
			page.DeletedPaths = append(page.DeletedPaths, entry.path)
		} else {
			page.Objects = append(page.Objects, entry.object)
		}
	}
	if end < len(entries) {
		state.Offset = end
	} else {
		state.Queue = state.Queue[1:]
		state.Offset = 0
	}
	if len(state.Queue) == 0 {
		page.Authoritative = true
		page.SyncToken = state.SyncToken
		return page, nil
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return storageInventoryPage{}, err
	}
	if len(raw) > remoteStorageCursorMaxBytes {
		return storageInventoryPage{}, errors.New("WebDAV inventory cursor exceeded the storage limit")
	}
	page.NextCursor = webDAVBFSCursorPrefix + base64.RawURLEncoding.EncodeToString(raw)
	return page, nil
}

func (w *webDAVBackend) doInventoryRequest(ctx context.Context, method, object, depth, body string) (davMultiStatus, int, error) {
	var multi davMultiStatus
	req, err := w.request(ctx, method, object, strings.NewReader(body))
	if err != nil {
		return multi, 0, err
	}
	req.Header.Set("Depth", depth)
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	resp, err := w.client().Do(req)
	if err != nil {
		return multi, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return multi, resp.StatusCode, nil
	}
	const maxInventoryBytes = 64 << 20
	encoded, err := io.ReadAll(io.LimitReader(resp.Body, maxInventoryBytes+1))
	if err != nil {
		return multi, resp.StatusCode, err
	}
	if len(encoded) > maxInventoryBytes {
		return multi, resp.StatusCode, errors.New("WebDAV inventory response exceeded 64 MiB")
	}
	if err := xml.Unmarshal(encoded, &multi); err != nil {
		return multi, resp.StatusCode, err
	}
	return multi, resp.StatusCode, nil
}

func (w *webDAVBackend) inventoryEntries(multi davMultiStatus) []webDAVInventoryEntry {
	basePath := strings.TrimSuffix(w.BaseURL.EscapedPath(), "/") + "/"
	entries := make([]webDAVInventoryEntry, 0, len(multi.Responses))
	for _, item := range multi.Responses {
		href, err := url.Parse(item.Href)
		if err != nil || (href.IsAbs() && !sameWebDAVOrigin(href, w.BaseURL)) {
			continue
		}
		escaped := href.EscapedPath()
		if !strings.HasPrefix(escaped, basePath) {
			continue
		}
		rel, err := url.PathUnescape(strings.TrimPrefix(escaped, basePath))
		if err != nil || rel == "" {
			continue
		}
		p, err := normalizeRemoteObjectPath(rel)
		if err != nil {
			continue
		}
		entry := webDAVInventoryEntry{path: p, deleted: strings.Contains(item.Status, " 404 ")}
		if entry.deleted {
			entries = append(entries, entry)
			continue
		}
		for _, ps := range item.PropStats {
			if !strings.Contains(ps.Status, " 200 ") {
				continue
			}
			if ps.Prop.ResourceType.Collection != nil {
				entry.collection = true
				entries = append(entries, entry)
				break
			}
			mt, _ := http.ParseTime(ps.Prop.Modified)
			revision := ps.Prop.ETag + "\x00" + strconv.FormatInt(ps.Prop.Length, 10) + "\x00" + mt.UTC().Format(time.RFC3339Nano)
			entry.object = storageObject{Path: p, Revision: revision, ETag: ps.Prop.ETag, Size: ps.Prop.Length, ModTime: mt, ContentType: ps.Prop.Type}
			entries = append(entries, entry)
			break
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].path == entries[j].path {
			return entries[i].collection && !entries[j].collection
		}
		return entries[i].path < entries[j].path
	})
	return entries
}
func (w *webDAVBackend) OpenRange(ctx context.Context, object string, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 || length <= 0 {
		return nil, errors.New("invalid WebDAV read range")
	}
	operationCtx, release, err := w.Scheduler.acquireOperation(ctx, remoteStorageReadIsPlayback(ctx))
	if err != nil {
		return nil, err
	}
	req, err := w.request(operationCtx, http.MethodGet, object, nil)
	if err != nil {
		release()
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	client := w.client()
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		release()
		if cause := context.Cause(operationCtx); cause != nil {
			return nil, cause
		}
		return nil, err
	}
	if resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		release()
		return nil, fmt.Errorf("WebDAV server did not honor range request: HTTP %d", resp.StatusCode)
	}
	if !validWebDAVContentRange(resp.Header.Get("Content-Range"), offset, length) || (resp.ContentLength >= 0 && resp.ContentLength != length) {
		resp.Body.Close()
		release()
		return nil, errors.New("WebDAV server returned a mismatched byte range")
	}
	return &releaseReadCloser{Reader: io.LimitReader(resp.Body, length), body: resp.Body, release: release}, nil
}

type releaseReadCloser struct {
	io.Reader
	body      io.ReadCloser
	release   func()
	closeOnce sync.Once
	closeErr  error
}

func (r *releaseReadCloser) Close() error {
	r.closeOnce.Do(func() {
		r.closeErr = r.body.Close()
		r.release()
	})
	return r.closeErr
}

func validWebDAVContentRange(value string, offset, length int64) bool {
	unit, value, ok := strings.Cut(strings.TrimSpace(value), " ")
	if !ok || !strings.EqualFold(unit, "bytes") {
		return false
	}
	span, total, ok := strings.Cut(value, "/")
	if !ok {
		return false
	}
	startText, endText, ok := strings.Cut(span, "-")
	if !ok {
		return false
	}
	start, startErr := strconv.ParseInt(startText, 10, 64)
	end, endErr := strconv.ParseInt(endText, 10, 64)
	if startErr != nil || endErr != nil || start != offset || end != offset+length-1 {
		return false
	}
	if total == "*" {
		return true
	}
	totalSize, err := strconv.ParseInt(total, 10, 64)
	return err == nil && totalSize > end
}
func webDAVXMLEscape(v string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(v))
	return b.String()
}
