package httpapi

import (
	"net/http"
	"strconv"

	"portico.local/server/internal/reading"
)

// This is a cover only, never a caller-chosen page/file. The original complete
// source revision is mandatory and the existing root/principal/page guards run
// before and after reading; no session, playback or download grant is minted.
func (d Dependencies) readingImageFolderCover(w http.ResponseWriter, r *http.Request) {
	p, err := d.strictPrincipalFor(r)
	if err != nil {
		readingFailure(w, err)
		return
	}
	source, revision := r.URL.Query().Get("sourceId"), r.URL.Query().Get("sourceRevision")
	if source == "" || len(source) > 256 || len(revision) != 64 {
		readingFailure(w, reading.ErrInvalid)
		return
	}
	for _, c := range revision {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			readingFailure(w, reading.ErrInvalid)
			return
		}
	}
	width := 512
	raw, alternate := r.URL.Query().Get("width"), r.URL.Query().Get("w")
	if raw != "" && alternate != "" {
		readingFailure(w, reading.ErrInvalid)
		return
	}
	if raw == "" {
		raw = alternate
	}
	if raw != "" {
		width, err = strconv.Atoi(raw)
		if err != nil || width < 32 || width > 2048 {
			readingFailure(w, reading.ErrInvalid)
			return
		}
	}
	f, err := d.readingService().Open(r.Context(), p, r.PathValue("itemId"), source, revision)
	if err != nil {
		readingFailure(w, err)
		return
	}
	defer f.Close()
	if f.Format != "image-folder" {
		readingFailure(w, reading.ErrUnavailable)
		return
	}
	comic, err := f.Comic()
	if err != nil {
		readingFailure(w, f.StopAndRecheck(err))
		return
	}
	page, err := comic.Page(f.Context(), 0, width)
	if err != nil {
		readingFailure(w, f.StopAndRecheck(err))
		return
	}
	serveReadingBlob(w, r, f, page.Image.MIME, page.Bytes)
}
