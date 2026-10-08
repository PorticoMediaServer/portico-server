package httpapi

import (
	"net/http"
	"time"
)

// The server sets no WriteTimeout on purpose: a stream and a byte range
// legitimately outlive any fixed budget, and a global one would cut off a
// healthy download. The consequence, until now, was that nothing bounded a
// single write. Once a client stops reading and the socket buffer fills,
// fmt.Fprintf on an event stream blocks indefinitely — no RST arrives from a
// phone that went into a tunnel, sometimes for many minutes. The goroutine, its
// lane slot, its open file and, for a mounted source, its helper process are all
// held for that whole time. The bounded stream-lifetime timer never fires,
// because the goroutine is stuck inside the write rather than in its select.
//
// Two shapes, for the two kinds of write:
//
//   - A frame deadline for server-sent events. Each frame is small and a
//     legitimate client accepts one in milliseconds, so twenty seconds — the
//     heartbeat interval — is invisible when healthy and reclaims a dead socket
//     within one beat.
//   - A rolling deadline for media bodies, refreshed on every successful write.
//     This is the one that has to be right: a fixed deadline would cut off a
//     slow-but-alive client part way through a long file, which is exactly the
//     client least able to recover. What is bounded is the gap between two
//     successful writes, not the length of the response.

// These are variables rather than constants so a test can shrink them; nothing
// in the server writes to them.
var (
	// streamFrameDeadline bounds one server-sent event frame.
	streamFrameDeadline = 20 * time.Second
	// mediaWriteDeadline bounds the gap between two successful body writes. Long
	// enough for any real buffering pause on a phone, short enough to reclaim a
	// socket whose peer has gone without saying so.
	mediaWriteDeadline = 60 * time.Second
	// deadlineRefreshInterval keeps the rolling refresh off the per-write path
	// without letting the effective window drift far from mediaWriteDeadline.
	deadlineRefreshInterval = 5 * time.Second
)

// frameDeadline bounds the next write on a stream. A response writer that cannot
// take a deadline (a test recorder, a wrapper that does not unwrap) is left
// alone: the deadline is a reclamation mechanism, not a correctness one.
func frameDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(streamFrameDeadline))
}

// clearWriteDeadline removes a deadline this package set, for a handler that
// hands the connection on.
func clearWriteDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
}

// rollingDeadlineWriter refreshes a write deadline as the body makes progress,
// so what is bounded is a stall rather than the transfer.
type rollingDeadlineWriter struct {
	http.ResponseWriter
	controller *http.ResponseController
	refreshed  time.Time
}

// withRollingDeadline wraps w so every write is covered by a deadline that moves
// with the transfer.
func withRollingDeadline(w http.ResponseWriter) *rollingDeadlineWriter {
	wrapped := &rollingDeadlineWriter{ResponseWriter: w, controller: http.NewResponseController(w)}
	wrapped.refresh()
	return wrapped
}

func (r *rollingDeadlineWriter) refresh() {
	r.refreshed = time.Now()
	_ = r.controller.SetWriteDeadline(r.refreshed.Add(mediaWriteDeadline))
}

func (r *rollingDeadlineWriter) Write(p []byte) (int, error) {
	if time.Since(r.refreshed) >= deadlineRefreshInterval {
		r.refresh()
	}
	return r.ResponseWriter.Write(p)
}

// Unwrap keeps http.NewResponseController and net/http's own feature detection
// working through the wrapper.
func (r *rollingDeadlineWriter) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *rollingDeadlineWriter) Flush() { _ = r.controller.Flush() }

// release drops the deadline once the body is done, so a keep-alive connection
// carries nothing from this response into the next.
func (r *rollingDeadlineWriter) release() { _ = r.controller.SetWriteDeadline(time.Time{}) }
