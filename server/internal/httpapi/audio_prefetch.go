package httpapi

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// A progressive request does not promise a bounded Range. Its response can
// retain its exact identity through commit, with backpressure at the aggregate
// private byte budget. Explicit finite/suffix ranges keep strict reservation.
func openAudioRange(header string) bool {
	if header == "" {
		return true
	}
	s, ok := strings.CutPrefix(strings.TrimSpace(header), "bytes=")
	if !ok || strings.Contains(s, ",") {
		return false
	}
	a, b, ok := strings.Cut(s, "-")
	n, err := strconv.ParseInt(a, 10, 64)
	return ok && b == "" && err == nil && n >= 0
}

func (d Dependencies) privateAudioBody(w http.ResponseWriter, r *http.Request, grant, session string, limit int64) *privateAudioWriter {
	check := func() error {
		_, p, item, err := d.Playback.ResolveDecodeGrantContext(r.Context(), grant)
		if err == nil {
			err = d.itemAccess(r.Context(), p, item)
		}
		return err
	}
	return &privateAudioWriter{ResponseWriter: w, ctx: r.Context(),
		close:     d.Playback.RetainPrefetch(session),
		authority: newStreamAuthority(check),
		private:   func() (bool, error) { return d.Playback.PrivatePresentation(r.Context(), session) },
		take:      func(n int64) int64 { return d.Playback.PrefetchTake(session, n, limit) },
		refund:    func(n int64) { d.Playback.PrefetchRefund(session, n) }}
}

// Waiting retains only the current encoder/copy chunk, not a media spool. Every
// wake checks current grant/item authority and preparation expiry before bytes
// can resume. This supports ordinary HTTP continuation; it makes no CAF preload,
// SDK cache reuse, timeout or gapless guarantee.
type privateAudioWriter struct {
	http.ResponseWriter
	ctx       context.Context
	authority *streamAuthority
	private   func() (bool, error)
	take      func(int64) int64
	refund    func(int64)
	released  bool
	status    int
	close     func()
}

func (w *privateAudioWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *privateAudioWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *privateAudioWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.status < 200 || w.status >= 300 {
		return w.ResponseWriter.Write(p)
	}
	written := 0
	for len(p) > 0 {
		if err := w.ctx.Err(); err != nil {
			return written, err
		}
		if err := w.authority.check(); err != nil {
			return written, err
		}
		if !w.released {
			private, err := w.private()
			if err != nil {
				return written, err
			}
			if !private {
				// A missing private row can mean cancellation as well as commit.
				// Never release using a cached verdict from before that change.
				if err = w.authority.full(); err != nil {
					return written, err
				}
				w.released = true
			}
		}
		if w.released {
			n, err := w.ResponseWriter.Write(p)
			return written + n, err
		}
		n := w.take(int64(len(p)))
		if n == 0 {
			if err := w.authority.full(); err != nil {
				return written, err
			}
			// Make accepted prefix bytes available before waiting for commit.
			if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil && err != http.ErrNotSupported {
				return written, err
			}
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-w.ctx.Done():
				timer.Stop()
				return written, w.ctx.Err()
			case <-timer.C:
			}
			continue
		}
		count, err := w.ResponseWriter.Write(p[:n])
		written += count
		if int64(count) < n {
			w.refund(n - int64(count))
		}
		if err != nil {
			return written, err
		}
		if int64(count) != n {
			return written, io.ErrShortWrite
		}
		p = p[count:]
	}
	return written, nil
}
