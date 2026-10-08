package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/social"
)

// Notifications and feedback.
//
// Three shapes of request live here and they are deliberately handled
// differently:
//
//   - Ordinary reads and writes go through noticeRegister, which is the console
//     envelope with a short timeout and an explicit Content-Length, so a native
//     bounded reader can consume them.
//   - GET /v1/notifications/events holds a connection open, so it cannot use
//     that wrapper: it streams, heartbeats every 20 seconds, and re-checks
//     authorization on every frame rather than trusting the handshake.
//   - GET /v1/notifications/wait parks the same way without server-sent events,
//     for clients that cannot keep an event-stream reader open.
//
// Both long-lived shapes are admitted by the same limiter, because an
// unbounded number of parked connections is a denial of service on a server
// whose database allows one writer.

const (
	// Every app instance holds a notification stream open, so this number is the
	// number of viewers the server supports, not a safety margin. It was 64
	// against a realtime lane of 512 and a target of two hundred users: the
	// sixty-fifth viewer simply never got notifications, and saw a capacity error
	// on a route that should always work. It now sits at the lane's own
	// admission, so the lane is the single place capacity is decided.
	//
	// Raising it is only safe alongside the per-frame write deadline in
	// write_deadline.go — 512 parked streams with no bound on a single write is a
	// worse problem than 64 with one.
	noticeStreamTotal = 512
	// The per-viewer cap stays where it is. It is an abuse fence, not a capacity
	// limit: no legitimate client opens a fifth concurrent notification stream,
	// and one client's reconnect loop must not be able to take the lane.
	noticeStreamPerView = 4
	noticeStreamMaxLife = 30 * time.Minute
	// noticeStreamSafetyInterval is the ticker behind the wake channel: it exists
	// only so a producer that never calls WakeNotifications still reaches a
	// parked reader, and at this cadence two hundred connected viewers cost about
	// four queries a second between them rather than two hundred.
	noticeStreamSafetyInterval = 45 * time.Second
	noticeDefaultWaitSec       = 25
)

// noticeStreamLimiter bounds parked connections overall and per viewer. The
// previous build limited its long polls but left its event streams unbounded;
// one limiter covers both here.
type noticeStreamLimiter struct {
	mu    sync.Mutex
	total int
	held  map[string]int
}

func (l *noticeStreamLimiter) admit(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= noticeStreamTotal || l.held[key] >= noticeStreamPerView {
		return false
	}
	l.total++
	l.held[key]++
	return true
}

func (l *noticeStreamLimiter) release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.total--
	if l.held[key]--; l.held[key] <= 0 {
		delete(l.held, key)
	}
}

func noticeScopeEnvelope(d Dependencies, p identity.Principal) map[string]string {
	return map[string]string{
		"serverId":    d.Identity.ID(),
		"viewerFence": fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", p.Hash, p.Epoch)))),
	}
}

// noticeBody decodes a request body strictly: unknown fields are a 400 rather
// than a silently ignored instruction from a client that expects them to apply.
func noticeBody(r *http.Request, v any) error {
	var raw json.RawMessage
	if e := decode(nil, r, &raw); e != nil {
		return &operations.ValidationError{Fields: []string{"request"}}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if e := dec.Decode(v); e != nil {
		return &operations.ValidationError{Fields: []string{"request"}}
	}
	return nil
}

func noticeLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}
	n, e := strconv.Atoi(raw)
	if e != nil || n < 1 || n > 100 {
		return 0, operations.ErrInvalid
	}
	return n, nil
}

func noticeRevisionParam(r *http.Request, key string) (int64, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, nil
	}
	n, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || n < 0 {
		return 0, operations.ErrInvalid
	}
	return n, nil
}

// notificationRoutes registers the whole workstream. Nothing here replaces the
// pre-existing /v1/notifications and /v1/feedback routes; these are the
// documented surface and they read and write the same records.
func (d Dependencies) notificationRoutes(mux *http.ServeMux) {
	if d.Console == nil || d.DB == nil {
		return
	}
	limiter := &noticeStreamLimiter{held: map[string]int{}}
	slots := make(chan struct{}, 8)
	type action func(*http.Request, identity.Principal, operations.Authorize) (any, error)
	register := func(pattern string, owner bool, query []string, fn action) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			var p identity.Principal
			var e error
			if owner {
				p, e = d.owner(r)
			} else {
				p, e = d.principal(r)
			}
			if e != nil {
				consoleError(w, e)
				return
			}
			if e = consoleQuery(r, query...); e != nil {
				consoleError(w, e)
				return
			}
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				consoleError(w, operations.ErrCapacity)
				return
			}
			auth := d.consoleAuthority(p, owner)
			out, e := fn(r, p, auth)
			if e != nil {
				consoleError(w, e)
				return
			}
			// Re-check authorization after the read: a session revoked mid-request
			// must not receive the document it was building.
			if e = d.ConsoleCheck(ctx, auth); e != nil {
				consoleError(w, e)
				return
			}
			consoleWrite(w, 200, map[string]any{"scope": noticeScopeEnvelope(d, p), "data": out})
		})
	}

	register("GET /v1/notifications/capabilities", false, nil, func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
		return operations.NoticeVocabularyDocument(), nil
	})
	register("GET /v1/notifications/inbox", false, []string{"audience", "state", "cursor", "limit"}, func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
		limit, e := noticeLimit(r)
		if e != nil {
			return nil, e
		}
		q := r.URL.Query()
		return d.Console.Notices(r.Context(), p, a, operations.NoticeQuery{
			Audience: q.Get("audience"), State: q.Get("state"), Cursor: q.Get("cursor"), Limit: limit,
		})
	})
	register("GET /v1/notifications/unread-count", false, []string{"audience"}, func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
		return d.Console.NoticeSummary(r.Context(), p, a, r.URL.Query().Get("audience"))
	})
	register("POST /v1/notifications/inbox/actions", false, nil, func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
		var c operations.NoticeBatch
		if e := noticeBody(r, &c); e != nil {
			return nil, e
		}
		return d.Console.ApplyNotices(r.Context(), p, a, c)
	})
	register("POST /v1/admin/notifications/broadcast", true, nil, func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
		var c operations.NoticeBroadcast
		if e := noticeBody(r, &c); e != nil {
			return nil, e
		}
		return d.Console.BroadcastNotice(r.Context(), p, a, c)
	})
	register("GET /v1/admin/notifications/settings", true, nil, func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
		return d.Console.NotificationSettings(r.Context(), a)
	})
	register("PATCH /v1/admin/notifications/settings", true, nil, func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
		var c operations.NotificationSettingsChange
		if e := noticeBody(r, &c); e != nil {
			return nil, e
		}
		return d.Console.ApplyNotificationSettings(r.Context(), p, a, c)
	})

	// Feedback.
	register("GET /v1/feedback/capabilities", false, nil, func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
		return d.Console.FeedbackCapabilitiesDocument(r.Context(), p, a)
	})
	register("POST /v1/feedback/reports", false, nil, func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
		var c operations.FeedbackSubmission
		if e := noticeBody(r, &c); e != nil {
			return nil, e
		}
		return d.Console.SubmitFeedback(r.Context(), p, a, c)
	})
	// These four are registered as whole literals rather than composed from a
	// prefix, so a source scan of the routing table finds them and the published
	// document has to carry them.
	listReports := func(owner bool) action {
		return func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
			limit, e := noticeLimit(r)
			if e != nil {
				return nil, e
			}
			q := r.URL.Query()
			return d.Console.FeedbackReports(r.Context(), p, a, owner, operations.FeedbackFilter{
				Status: q.Get("status"), Kind: q.Get("kind"), Category: q.Get("category"),
				Reporter: q.Get("reporter"), Cursor: q.Get("cursor"), Limit: limit,
			})
		}
	}
	readReport := func(owner bool) action {
		return func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
			return d.Console.FeedbackReportDocument(r.Context(), p, a, owner, r.PathValue("id"))
		}
	}
	listQuery := []string{"status", "kind", "category", "reporter", "cursor", "limit"}
	register("GET /v1/feedback/reports", false, listQuery, listReports(false))
	register("GET /v1/feedback/reports/{id}", false, nil, readReport(false))
	register("GET /v1/admin/feedback/reports", true, listQuery, listReports(true))
	register("GET /v1/admin/feedback/reports/{id}", true, nil, readReport(true))
	register("POST /v1/admin/feedback/reports/{id}/status", true, nil, func(r *http.Request, p identity.Principal, a operations.Authorize) (any, error) {
		var c operations.FeedbackTransition
		if e := noticeBody(r, &c); e != nil {
			return nil, e
		}
		return d.Console.TransitionFeedback(r.Context(), p, a, r.PathValue("id"), c)
	})

	mux.HandleFunc("GET /v1/notifications/wait", func(w http.ResponseWriter, r *http.Request) {
		d.noticeWait(w, r, limiter)
	})
	mux.HandleFunc("GET /v1/notifications/events", func(w http.ResponseWriter, r *http.Request) {
		d.noticeEvents(w, r, limiter)
	})
}

// noticeWait is the long-poll variant. It parks until the inbox revision moves
// past the client's, the wait elapses, or the client goes away. A client with no
// revision yet gets an immediate answer rather than a pointless park.
func (d Dependencies) noticeWait(w http.ResponseWriter, r *http.Request, limiter *noticeStreamLimiter) {
	w.Header().Set("Cache-Control", "no-store")
	p, e := d.principal(r)
	if e != nil {
		consoleError(w, e)
		return
	}
	if e = consoleQuery(r, "audience", "revision", "waitSeconds"); e != nil {
		consoleError(w, e)
		return
	}
	since, e := noticeRevisionParam(r, "revision")
	if e != nil {
		consoleError(w, e)
		return
	}
	wait := time.Duration(noticeDefaultWaitSec) * time.Second
	if raw := r.URL.Query().Get("waitSeconds"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 60 {
			consoleError(w, operations.ErrInvalid)
			return
		}
		wait = time.Duration(n) * time.Second
	}
	key := p.Hash
	if !limiter.admit(key) {
		consoleError(w, operations.ErrCapacity)
		return
	}
	defer limiter.release(key)
	ctx, cancel := context.WithTimeout(r.Context(), wait+10*time.Second)
	defer cancel()
	auth := d.consoleAuthority(p, false)
	out, e := d.Console.NoticeWait(ctx, p, auth, r.URL.Query().Get("audience"), since, wait)
	if e != nil {
		consoleError(w, e)
		return
	}
	if e = d.ConsoleCheck(ctx, auth); e != nil {
		consoleError(w, e)
		return
	}
	consoleWrite(w, 200, map[string]any{"scope": noticeScopeEnvelope(d, p), "data": out})
}

// noticeEvents is the server-sent event stream.
//
// Notification frames carry the inbox revision as id, so
// Last-Event-ID resume is exact: the server replays the notifications whose
// revision is above the one the client last saw. When more changed than a frame
// can carry, the frame says resync rather than silently dropping records —
// which is what the previous build did by refusing resume altogether.
func (d Dependencies) noticeEvents(w http.ResponseWriter, r *http.Request, limiter *noticeStreamLimiter) {
	w.Header().Set("Cache-Control", "no-store")
	p, e := d.principal(r)
	if e != nil {
		consoleError(w, e)
		return
	}
	if e = consoleQuery(r, "audience", "revision"); e != nil {
		consoleError(w, e)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		consoleError(w, operations.ErrCapacity)
		return
	}
	since, e := noticeRevisionParam(r, "revision")
	if e != nil {
		consoleError(w, e)
		return
	}
	// Last-Event-ID is the reconnect authority; the query parameter is the
	// first-connect hint. A malformed header resumes from nothing rather than
	// failing the connection.
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= 0 {
			since = n
		}
	}
	audience := r.URL.Query().Get("audience")
	auth := d.consoleAuthority(p, false)
	// Validate the audience and the session before committing to a long-lived
	// response, so a bad request still gets a JSON error rather than a stream.
	first, e := d.Console.NoticeChanges(r.Context(), p, auth, audience, since)
	if e != nil {
		consoleError(w, e)
		return
	}
	key := p.Hash
	if !limiter.admit(key) {
		consoleError(w, operations.ErrCapacity)
		return
	}
	defer limiter.release(key)
	receiverChanges, releaseReceivers := social.SubscribeReceiverEvents(d.DB, p)
	defer releaseReceivers()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Connection", "keep-alive")
	// Proxies that buffer would defeat the point of the stream.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	send := func(event string, revision int64, payload any) bool {
		body, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		// Without this the write blocks until the OS gives up on a client that has
		// stopped reading, which can be minutes, and the bounded stream-lifetime
		// timer below never fires because the goroutine is stuck here rather than
		// in its select. A healthy client accepts a frame this size in
		// milliseconds.
		frameDeadline(w)
		if _, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", revision, event, body); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send("hello", first.Revision, map[string]any{
		"revision": first.Revision, "counts": first.Counts, "audience": audience,
		"heartbeatSeconds": 20, "serverId": d.Identity.ID(),
	}) {
		return
	}
	if since > 0 && (len(first.Items) > 0 || first.Resync) {
		if !send("change", first.Revision, first) {
			return
		}
	}
	current := first.Revision
	ctx := r.Context()
	deadline := time.NewTimer(noticeStreamMaxLife)
	defer deadline.Stop()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	// The stream is driven by the wake channel the notification writers already
	// broadcast on. It used to be driven by a two-second ticker, which at two
	// hundred connected viewers is a hundred authorisation checks and a hundred
	// change queries every second, forever, whether or not anything happened —
	// load the `realtime` lane cannot see, because it has neither a budget nor a
	// queue. The ticker survives as a safety net for a writer in another package
	// that forgets to wake, at a cadence where it costs nothing.
	wake, releaseWake := operations.SubscribeNotifications()
	defer releaseWake()
	poll := time.NewTicker(noticeStreamSafetyInterval)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			// A bounded life keeps a forgotten client from holding a slot forever.
			// The client reconnects with Last-Event-ID and loses nothing.
			send("closed", current, map[string]any{"revision": current, "reason": "stream-lifetime"})
			return
		case <-heartbeat.C:
			// The heartbeat is also where a long-lived transport re-proves its
			// authority. It is a strict check against live state, never from the
			// principal cache: a durable revocation must not leave a parked
			// stream authorised until a cache entry expires.
			if e = d.ConsoleCheck(ctx, auth); e != nil {
				send("closed", current, map[string]any{"revision": current, "reason": "unauthorized"})
				return
			}
			if !send("heartbeat", current, map[string]any{"revision": current, "at": time.Now().UnixMilli()}) {
				return
			}
			continue
		case event := <-receiverChanges:
			if e = d.ConsoleCheck(ctx, auth); e != nil {
				return
			}
			// No id: receiver invalidations must not advance the notification cursor.
			// Reconnect always emits resync; the durable receiver inbox supplies data.
			raw, err := json.Marshal(event)
			if err != nil {
				return
			}
			frameDeadline(w)
			if _, err = fmt.Fprintf(w, "event: receiver.inbox_changed\ndata: %s\n\n", raw); err != nil {
				return
			}
			flusher.Flush()
			continue
		case <-wake:
		case <-poll.C:
		}
		// Authorization is re-derived before anything is sent, so a revoked
		// session stops receiving rather than living until it disconnects.
		if e = d.ConsoleCheck(ctx, auth); e != nil {
			send("closed", current, map[string]any{"revision": current, "reason": "unauthorized"})
			return
		}
		delta, err := d.Console.NoticeChanges(ctx, p, auth, audience, current)
		if err != nil {
			send("closed", current, map[string]any{"revision": current, "reason": "unavailable"})
			return
		}
		if delta.Revision == current {
			continue
		}
		current = delta.Revision
		if !send("change", current, delta) {
			return
		}
	}
}
