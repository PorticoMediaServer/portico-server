package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"portico.local/server/internal/contentaccess"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/reading"
	"portico.local/server/internal/storage"
)

func (d Dependencies) readingService() *reading.Service {
	return &reading.Service{DB: d.DB, Storage: d.Storage, Mounts: d.Mounts, Authorize: func(ctx context.Context, tx *sql.Tx, p identity.Principal, library, item string) error {
		live, err := d.Identity.PrincipalByDigestTx(ctx, tx, p.Hash)
		if err != nil {
			return err
		}
		if live.Viewer != p.Viewer {
			return identity.ErrUnauthorized
		}
		if err = live.PermitsRequest(ctx); err != nil {
			return err
		}
		if err = d.allowedLibraryTx(ctx, live, library, tx); err != nil {
			return err
		}
		return contentaccess.VisibleItemTx(ctx, tx, live, item)
	}}
}

func readingFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, reading.ErrChanged), errors.Is(err, storage.ErrObservedSourceLost):
		policyError(w, "revision_mismatch")
	case errors.Is(err, reading.ErrUnavailable):
		policyError(w, "not_found")
	case errors.Is(err, reading.ErrInvalid):
		policyError(w, "invalid_request")
	case errors.Is(err, reading.ErrLimit):
		policyError(w, "body_too_large")
	case errors.Is(err, storage.ErrBusy):
		policyError(w, "request_in_progress")
	default:
		failure(w, err)
	}
}

func readingHeaders(w http.ResponseWriter, revision string) {
	w.Header().Set("ETag", `"`+revision+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; base-uri 'none'; form-action 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func (d Dependencies) openReadingBytes(w http.ResponseWriter, r *http.Request) (*reading.Source, bool) {
	p, err := d.strictPrincipalFor(r)
	if err != nil {
		readingFailure(w, err)
		return nil, false
	}
	// A concrete publication revision is mandatory, including for HEAD. A
	// wildcard cannot establish which source the engine actually opened.
	match := r.Header.Get("If-Match")
	if match == "" {
		policyError(w, "revision_required")
		return nil, false
	}
	if len(match) != 66 || match[0] != '"' || match[len(match)-1] != '"' {
		policyError(w, "revision_mismatch")
		return nil, false
	}
	f, err := d.readingService().Open(r.Context(), p, r.PathValue("id"), r.URL.Query().Get("sourceId"), match[1:len(match)-1])
	if err != nil {
		readingFailure(w, err)
		return nil, false
	}
	readingHeaders(w, f.Revision)
	return f, true
}

// readingRange accepts only one explicit finite byte interval. It never
// converts an unsupported range to a full download or shortens the declared
// interval. The client PDF adapter currently asks for 64 KiB chunks.
func readingRange(raw string, size int64) (start, end int64, ok bool) {
	if len(raw) > 96 || !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
		return
	}
	a, b, found := strings.Cut(strings.TrimPrefix(raw, "bytes="), "-")
	if !found || a == "" || b == "" {
		return
	}
	for _, s := range []string{a, b} {
		for _, c := range s {
			if c < '0' || c > '9' {
				return
			}
		}
	}
	var err error
	start, err = strconv.ParseInt(a, 10, 64)
	if err != nil {
		return
	}
	end, err = strconv.ParseInt(b, 10, 64)
	ok = err == nil && start >= 0 && end >= start && end < size && end-start < 2<<20
	return
}

func (d Dependencies) serveReadingSource(w http.ResponseWriter, r *http.Request) {
	f, ok := d.openReadingBytes(w, r)
	if !ok {
		return
	}
	defer f.Close()
	if f.Format != "pdf" {
		policyError(w, "not_found")
		return
	}
	var header [5]byte
	if _, err := f.ReadAt(header[:], 0); err != nil {
		readingFailure(w, err)
		return
	}
	if string(header[:]) != "%PDF-" {
		readingFailure(w, reading.ErrInvalid)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="publication.pdf"`)
	w.Header().Set("Accept-Ranges", "bytes")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
		return
	}
	start, end, ok := readingRange(r.Header.Get("Range"), f.Size)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", f.Size))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	stream, err := f.PDFRange(start, end+1)
	if err != nil {
		readingFailure(w, err)
		return
	}
	// Read the first bounded block before committing headers, so revocation or
	// source replacement during setup is an error envelope, never a fake 206.
	first := make([]byte, min(int(end-start+1), 64<<10))
	if _, err = io.ReadFull(stream, first); err != nil {
		readingFailure(w, err)
		return
	}
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, f.Size))
	body := withRollingDeadline(w)
	defer body.release()
	body.WriteHeader(http.StatusPartialContent)
	if _, err = body.Write(first); err != nil {
		return
	}
	if _, err = io.Copy(body, stream); err != nil {
		panic(http.ErrAbortHandler)
	}
}

func (d Dependencies) serveReadingAsset(w http.ResponseWriter, r *http.Request) {
	f, ok := d.openReadingBytes(w, r)
	if !ok {
		return
	}
	defer f.Close()
	if f.Format != "epub" {
		policyError(w, "not_found")
		return
	}
	e, err := reading.OpenEPUB(r.Context(), f, f.Size)
	if err != nil {
		readingFailure(w, err)
		return
	}
	b, mime, err := e.Asset(r.Context(), r.PathValue("asset"))
	if err != nil {
		readingFailure(w, err)
		return
	}
	serveReadingBlob(w, r, f, mime, b)
}

func (d Dependencies) serveReadingPage(w http.ResponseWriter, r *http.Request) {
	f, ok := d.openReadingBytes(w, r)
	if !ok {
		return
	}
	defer f.Close()
	if f.Format != "cbz" {
		policyError(w, "not_found")
		return
	}
	index, err := strconv.Atoi(r.PathValue("page"))
	if err != nil {
		readingFailure(w, reading.ErrInvalid)
		return
	}
	width := 0
	if raw := r.URL.Query().Get("width"); raw != "" {
		width, err = strconv.Atoi(raw)
		if err != nil || width == 0 {
			readingFailure(w, reading.ErrInvalid)
			return
		}
	}
	c, err := reading.OpenComic(r.Context(), f, f.Size)
	if err != nil {
		readingFailure(w, err)
		return
	}
	p, err := c.Page(r.Context(), index, width)
	if err != nil {
		readingFailure(w, err)
		return
	}
	serveReadingBlob(w, r, f, p.Image.MIME, p.Bytes)
}

func serveReadingBlob(w http.ResponseWriter, r *http.Request, f *reading.Source, mime string, b []byte) {
	if err := f.Validate(); err != nil {
		readingFailure(w, err)
		return
	}
	revision := f.Revision
	readingHeaders(w, revision)
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	if r.Header.Get("If-None-Match") == `"`+revision+`"` {
		w.Header().Del("Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	body := withRollingDeadline(w)
	defer body.release()
	reader := &guardedReader{Reader: bytes.NewReader(b), check: f.Validate}
	if _, err := io.Copy(body, reader); err != nil {
		panic(http.ErrAbortHandler)
	}
}
