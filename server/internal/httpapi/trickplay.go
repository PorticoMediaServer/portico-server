package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediaanalysis"
)

// Chapter thumbnails and trickplay tiles are item-scoped, immutable previews of
// catalogued media, exactly like item artwork. They are therefore published on
// the item path under the same library-visibility authorization as
// GET /v1/items/{id}/art/{kind}, not under a playback grant: a grant belongs to
// one prepared or HLS session, is reissued on renewal and dies with the
// session, so grant-scoped preview URLs could not be cached by a player, could
// not be shown before a session exists, and would break mid-stream on renewal.
func (d Dependencies) trickplayRoutes(mux *http.ServeMux) {
	image := func(w http.ResponseWriter, r *http.Request, read func(context.Context, mediaanalysis.Access) (mediaanalysis.Image, error)) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if d.Analysis == nil {
			analysisFailure(w, mediaanalysis.ErrUnsupported)
			return
		}
		if r.URL.RawQuery != "" {
			analysisFailure(w, mediaanalysis.ErrInput)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		a, e := d.analysisAccess(r)
		if e != nil {
			analysisFailure(w, e)
			return
		}
		out, e := read(ctx, a)
		if e != nil {
			analysisFailure(w, e)
			return
		}
		after, e := d.analysisAccess(r)
		if e != nil {
			analysisFailure(w, e)
			return
		}
		if a.ViewerFence != after.ViewerFence || a.LibraryID != after.LibraryID {
			failure(w, identity.ErrNotVisible)
			return
		}
		w.Header().Set("Content-Type", out.MIME)
		w.Header().Set("Content-Length", strconv.Itoa(len(out.Data)))
		// Preview bytes are immutable for their digest; the viewer fence is
		// rechecked on every request, so a private cache is safe.
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.Header().Set("ETag", `"`+out.Digest+`"`)
		_, _ = w.Write(out.Data)
	}
	mux.HandleFunc("GET /v1/items/{id}/chapters/{chapter}/image", func(w http.ResponseWriter, r *http.Request) {
		chapter := r.PathValue("chapter")
		image(w, r, func(ctx context.Context, a mediaanalysis.Access) (mediaanalysis.Image, error) {
			return d.Analysis.ChapterImage(ctx, a, chapter)
		})
	})
	mux.HandleFunc("GET /v1/items/{id}/trickplay/{set}/tiles/{tile}", func(w http.ResponseWriter, r *http.Request) {
		set, name := r.PathValue("set"), r.PathValue("tile")
		raw, ok := strings.CutSuffix(name, ".jpg")
		index, e := strconv.Atoi(raw)
		if !ok || e != nil || raw != strconv.Itoa(index) {
			analysisFailure(w, mediaanalysis.ErrInput)
			return
		}
		image(w, r, func(ctx context.Context, a mediaanalysis.Access) (mediaanalysis.Image, error) {
			return d.Analysis.Tile(ctx, a, set, index)
		})
	})
	mux.HandleFunc("GET /v1/items/{id}/trickplay/{set}/thumbnails.vtt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if d.Analysis == nil {
			analysisFailure(w, mediaanalysis.ErrUnsupported)
			return
		}
		if r.URL.RawQuery != "" {
			analysisFailure(w, mediaanalysis.ErrInput)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		a, e := d.analysisAccess(r)
		if e != nil {
			analysisFailure(w, e)
			return
		}
		out, e := d.Analysis.Thumbnails(ctx, a, r.PathValue("set"))
		if e != nil {
			analysisFailure(w, e)
			return
		}
		after, e := d.analysisAccess(r)
		if e != nil {
			analysisFailure(w, e)
			return
		}
		if a.ViewerFence != after.ViewerFence || a.LibraryID != after.LibraryID {
			failure(w, identity.ErrNotVisible)
			return
		}
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(out)))
		w.Header().Set("Cache-Control", "private, max-age=300")
		_, _ = w.Write([]byte(out))
	})
	mux.HandleFunc("GET /v1/items/{id}/trickplay", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if d.Analysis == nil {
			analysisFailure(w, mediaanalysis.ErrUnsupported)
			return
		}
		if r.URL.RawQuery != "" {
			analysisFailure(w, mediaanalysis.ErrInput)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		a, e := d.analysisAccess(r)
		if e != nil {
			analysisFailure(w, e)
			return
		}
		out, e := d.Analysis.Trickplay(ctx, a)
		if e != nil {
			analysisFailure(w, e)
			return
		}
		after, e := d.analysisAccess(r)
		if e != nil {
			analysisFailure(w, e)
			return
		}
		if a.ViewerFence != after.ViewerFence || a.LibraryID != after.LibraryID {
			failure(w, identity.ErrNotVisible)
			return
		}
		write(w, 200, out)
	})
}
