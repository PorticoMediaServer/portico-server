package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func parseRemoteStorageLocator(raw string) (sourceID, objectPath string, err error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "portico-storage" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", "", errors.New("invalid Portico storage locator")
	}
	p, e := normalizeRemoteObjectPath(strings.TrimPrefix(u.Path, "/"))
	if e != nil {
		return "", "", e
	}
	return u.Host, p, nil
}

func parseStorageHTTPRange(value string, size int64) (start, end int64, partial bool, err error) {
	start = 0
	end = size - 1
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		err = errors.New("unsupported byte range")
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(value, "bytes="), "-", 2)
	if len(parts) != 2 {
		err = errors.New("invalid byte range")
		return
	}
	if parts[0] == "" {
		suffix, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || suffix <= 0 {
			err = errors.New("invalid byte range")
			return
		}
		if suffix > size {
			suffix = size
		}
		start = size - suffix
	} else {
		var parseErr error
		start, parseErr = strconv.ParseInt(parts[0], 10, 64)
		if parseErr != nil || start < 0 {
			err = errors.New("invalid byte range")
			return
		}
		if parts[1] != "" {
			end, parseErr = strconv.ParseInt(parts[1], 10, 64)
			if parseErr != nil {
				err = errors.New("invalid byte range")
				return
			}
		}
	}
	if start >= size || end < start {
		err = errors.New("byte range is outside object")
		return
	}
	if end >= size {
		end = size - 1
	}
	partial = true
	return
}

func (s *Server) serveRemoteStorageObject(w http.ResponseWriter, r *http.Request, item MediaItem, locator string) error {
	sourceID, objectPath, err := parseRemoteStorageLocator(locator)
	if err != nil {
		return err
	}
	var size int64
	var contentType string
	err = s.queryUserRow(r.Context(), `SELECT o.size_bytes,o.content_type FROM storage_remote_objects o JOIN storage_sources s ON s.id=o.source_id WHERE o.source_id=? AND o.object_path=? AND o.missing_since='' AND s.library_id=?`, sourceID, objectPath, item.LibraryID).Scan(&size, &contentType)
	if err != nil {
		return err
	}
	start, end, partial, err := parseStorageHTTPRange(r.Header.Get("Range"), size)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil
	}
	length := end - start + 1
	var reader io.ReadCloser
	if r.Method != http.MethodHead {
		backend, backendErr := s.remoteBackendForSource(r.Context(), sourceID)
		if backendErr != nil {
			return backendErr
		}
		reader, err = backend.OpenRange(r.Context(), objectPath, start, length)
		if err != nil {
			return err
		}
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, no-store")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	if partial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if r.Method == http.MethodHead {
		return nil
	}
	limited := &io.LimitedReader{R: reader, N: length}
	exact := &exactRangeReadCloser{LimitedReader: limited, Closer: reader}
	copyErr := copyRemotePlaybackBody(r.Context(), w, exact)
	closeErr := exact.Close()
	if copyErr != nil {
		return copyErr
	}
	if limited.N != 0 {
		return io.ErrUnexpectedEOF
	}
	return closeErr
}

func (s *Server) openRemoteStorageTranscodeSource(ctx context.Context, item MediaItem, locator string) (*remoteTranscodeSource, error) {
	sourceID, objectPath, err := parseRemoteStorageLocator(locator)
	if err != nil {
		return nil, err
	}
	var size int64
	if err := s.queryBackgroundRow(ctx, `SELECT o.size_bytes FROM storage_remote_objects o JOIN storage_sources source ON source.id=o.source_id WHERE o.source_id=? AND o.object_path=? AND o.missing_since='' AND source.library_id=?`, sourceID, objectPath, item.LibraryID).Scan(&size); err != nil {
		return nil, err
	}
	if size <= 0 {
		return nil, errors.New("remote storage object is empty")
	}
	backend, err := s.remoteBackendForSource(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	readCtx, cancel := context.WithCancel(ctx)
	body, err := backend.OpenRange(readCtx, objectPath, 0, size)
	if err != nil {
		cancel()
		return nil, err
	}
	return &remoteTranscodeSource{body: body, cancel: cancel, timeout: remoteTranscodeReadTimeout}, nil
}

type exactRangeReadCloser struct {
	*io.LimitedReader
	io.Closer
}

var _ = context.Canceled
