package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"portico.local/server/internal/eventfeed"
	"portico.local/server/internal/identity"
)

const eventHeartbeat = 25 * time.Second
const eventStreamLife = 30 * time.Minute

func eventFailure(w http.ResponseWriter, status int, code, message string) {
	consoleWrite(w, status, map[string]any{"error": map[string]any{
		"code": code, "message": message, "retryable": status == 409 || status == 503,
	}})
}

func eventAfter(raw string) (int64, bool, error) {
	if raw == "" {
		return 0, false, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, false, errors.New("invalid event cursor")
	}
	return n, true, nil
}

func (d Dependencies) eventAuthority(r *http.Request) (identity.Principal, bool, error) {
	p, err := d.strictPrincipalFor(r)
	if err != nil {
		return p, false, err
	}
	if p.Role != "owner" {
		return p, false, nil
	}
	_, err = d.owner(r)
	if err != nil {
		if errors.Is(err, identity.ErrUnauthorized) {
			return p, false, nil
		}
		return p, false, err
	}
	return p, true, nil
}

func (d Dependencies) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := consoleQuery(r, "after", "waitSeconds", "topics"); err != nil || r.URL.Query().Get("topics") != "" {
		eventFailure(w, 400, "invalid_request", "Only implicit notification and operation topics are available.")
		return
	}
	longPoll := r.URL.Query().Has("waitSeconds")
	if !longPoll && r.URL.Query().Has("after") {
		eventFailure(w, 400, "invalid_request", "Stream resumes with Last-Event-ID.")
		return
	}
	raw := r.Header.Get("Last-Event-ID")
	if longPoll {
		raw = r.URL.Query().Get("after")
	}
	after, supplied, err := eventAfter(raw)
	if err != nil {
		eventFailure(w, 400, "invalid_cursor", "Use a non-negative numeric event cursor.")
		return
	}
	wait := 25
	if longPoll {
		wait, err = strconv.Atoi(r.URL.Query().Get("waitSeconds"))
		if err != nil || wait < 0 || wait > 25 {
			eventFailure(w, 400, "invalid_request", "waitSeconds must be between 0 and 25.")
			return
		}
	}
	p, owner, err := d.eventAuthority(r)
	if err != nil {
		consoleError(w, err)
		return
	}
	wake, release := d.Events.Subscribe()
	defer release()
	page, err := d.Events.Read(r.Context(), p, owner, after, supplied)
	if err != nil {
		consoleError(w, err)
		return
	}
	if longPoll {
		d.eventLongPoll(w, r, p, wake, page, wait)
		return
	}
	d.eventStream(w, r, p, wake, page)
}

func (d Dependencies) eventLongPoll(w http.ResponseWriter, r *http.Request, p identity.Principal, wake <-chan struct{}, page eventfeed.Page, wait int) {
	if len(page.Events) == 0 && wait > 0 {
		timer := time.NewTimer(time.Duration(wait) * time.Second)
		defer timer.Stop()
		for len(page.Events) == 0 {
			select {
			case <-r.Context().Done():
				return
			case <-timer.C:
				goto answer
			case <-wake:
			}
			fresh, owner, err := d.eventAuthority(r)
			if err != nil {
				consoleError(w, err)
				return
			}
			if fresh.Hash != p.Hash {
				eventFailure(w, 401, "unauthorized", "Authentication is required.")
				return
			}
			page, err = d.Events.Read(r.Context(), fresh, owner, mustEventID(page.NextAfter), true)
			if err != nil {
				consoleError(w, err)
				return
			}
		}
	}
answer:
	if _, _, err := d.eventAuthority(r); err != nil {
		consoleError(w, err)
		return
	}
	consoleWrite(w, 200, page)
}

func mustEventID(raw string) int64 {
	id, _ := strconv.ParseInt(raw, 10, 64)
	return id
}

func (d Dependencies) eventStream(w http.ResponseWriter, r *http.Request, p identity.Principal, wake <-chan struct{}, page eventfeed.Page) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		eventFailure(w, 503, "stream_unavailable", "This connection cannot stream events.")
		return
	}
	key, err := d.Events.InstallationKey(r.Context(), p)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			eventFailure(w, 401, "unauthorized", "Authentication is required.")
		} else {
			consoleError(w, err)
		}
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	release, admitted := d.Events.AdmitStream(key, cancel)
	if !admitted {
		eventFailure(w, 409, "event_stream_exists", "An existing stream was retired; reconnect using your last event ID.")
		return
	}
	defer release()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	send := func(e eventfeed.Event) bool {
		b, err := json.Marshal(e)
		if err != nil {
			return false
		}
		frameDeadline(w)
		if _, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", e.ID, e.Type, b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	frameDeadline(w)
	if _, err = fmt.Fprint(w, "retry: 1000\n\n"); err != nil {
		return
	}
	flusher.Flush()
	flushPending := func() bool {
		for {
			fresh, owner, err := d.eventAuthority(r.WithContext(ctx))
			if err != nil || fresh.Hash != p.Hash {
				return false
			}
			for _, e := range page.Events {
				if !send(e) {
					return false
				}
			}
			if len(page.Events) < eventfeed.PageLimit {
				return true
			}
			page, err = d.Events.Read(ctx, fresh, owner, mustEventID(page.NextAfter), true)
			if err != nil {
				return false
			}
		}
	}
	if !flushPending() {
		return
	}
	heartbeat := time.NewTicker(eventHeartbeat)
	defer heartbeat.Stop()
	end := time.NewTimer(eventStreamLife)
	defer end.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-end.C:
			frameDeadline(w)
			fmt.Fprint(w, "event: stream.closed\ndata: {\"reconnectAfterMs\":1000}\n\n")
			flusher.Flush()
			return
		case <-heartbeat.C:
			if _, _, err := d.eventAuthority(r.WithContext(ctx)); err != nil {
				return
			}
			frameDeadline(w)
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
			continue
		case <-wake:
		}
		fresh, owner, err := d.eventAuthority(r.WithContext(ctx))
		if err != nil || fresh.Hash != p.Hash {
			return
		}
		page, err = d.Events.Read(ctx, fresh, owner, mustEventID(page.NextAfter), true)
		if err != nil {
			return
		}
		if !flushPending() {
			return
		}
	}
}
