package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"portico.local/server/internal/requestinput"
	"strconv"
	"time"

	"portico.local/apikit"
	"portico.local/server/internal/apievents"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/eventfeed"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/social"
	"portico.local/server/internal/supervise"
)

var eventHeartbeat = 25 * time.Second

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
		return 0, false, requestinput.New("invalid event cursor")
	}
	return n, true, nil
}

func (d Dependencies) eventAuthority(r *http.Request) (identity.Principal, bool, error) {
	p, err := d.strictPrincipalFor(r)
	if err != nil {
		return p, false, err
	}
	if err = d.checkCapacityReader(r, p); err != nil {
		return p, false, err
	}
	if credential, ok := identity.APICredentialFrom(r.Context()); ok {
		granted := make([]apikit.Scope, 0, len(credential.Scopes))
		for _, scope := range credential.Scopes {
			granted = append(granted, apikit.Scope(scope))
		}
		if !apikit.ScopeAllows(granted, apikit.ScopeAdmin) {
			return p, false, nil
		}
	}
	// The owner and administrators hear the administration audience (lists,
	// sessions, tasks); each is re-checked against the live membership.
	switch p.Role {
	case "owner":
		_, err = d.owner(r)
	case "admin":
		_, err = d.administrator(r)
	default:
		return p, false, nil
	}
	if err != nil {
		if errors.Is(err, identity.ErrUnauthorized) || errors.Is(err, identity.ErrForbidden) {
			return p, false, nil
		}
		return p, false, err
	}
	return p, true, nil
}

func (d Dependencies) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := consoleQuery(r, "after", "waitMs"); err != nil {
		eventFailure(w, 400, "invalid_request", "This event query cannot be used.")
		return
	}
	longPoll := r.URL.Query().Has("waitMs")
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
	wait := 25000
	if longPoll {
		wait, err = strconv.Atoi(r.URL.Query().Get("waitMs"))
		if err != nil || wait < 0 || wait > 25000 {
			eventFailure(w, 400, "invalid_request", "waitMs must be between 0 and 25000.")
			return
		}
	}
	p, owner, err := d.eventAuthority(r)
	if err != nil {
		consoleError(w, err)
		return
	}
	r, err = d.withCapacityAdmission(r, p)
	if err != nil {
		consoleError(w, err)
		return
	}
	bridgeCtx, stopBridge := context.WithCancel(r.Context())
	defer stopBridge()
	d.bridgeReceiverEvents(bridgeCtx, p)
	wake, release := d.Events.SubscribeForContext(r.Context(), p, owner)
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
		timer := time.NewTimer(time.Duration(wait) * time.Millisecond)
		defer timer.Stop()
		for len(page.Events) == 0 {
			if !page.More {
				select {
				case <-r.Context().Done():
					return
				case <-timer.C:
					goto answer
				case <-wake:
				}
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
	lastSent := ""
	send := func(e eventfeed.Event) bool {
		if e.Type == "capacity.changed" {
			if err := d.checkCapacityReader(r.WithContext(ctx), p); err != nil {
				return false
			}
		}
		b, err := json.Marshal(e)
		if err != nil {
			return false
		}
		frameDeadline(w)
		if _, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", e.ID, e.Type, b); err != nil {
			return false
		}
		flusher.Flush()
		lastSent = e.ID
		return true
	}
	frameDeadline(w)
	if _, err = fmt.Fprint(w, "retry: 1000\n\n"); err != nil {
		return
	}
	flusher.Flush()
	// A stream that opens with nothing waiting says where it stands. Without
	// this a client learns its position only from the first real event, and a
	// connection that drops before one arrives would resume from "now" and miss
	// whatever happened in between.
	if len(page.Events) == 0 && page.NextAfter != "" {
		if !send(eventfeed.Event{ID: page.NextAfter, Type: "stream.opened", At: time.Now().UTC().Format(time.RFC3339)}) {
			return
		}
	}
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
			if !page.More {
				if page.NextAfter != "" && lastSent != page.NextAfter {
					if !send(eventfeed.Event{ID: page.NextAfter, Type: "stream.position"}) {
						return false
					}
				}
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
	deadline, err := d.Events.TokenDeadline(ctx, p)
	if err != nil {
		return
	}
	life := time.Until(deadline)
	if life < 0 {
		life = 0
	}
	if life > eventStreamLife {
		life = eventStreamLife
	}
	end := time.NewTimer(life)
	defer end.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-end.C:
			frameDeadline(w)
			send(eventfeed.Event{ID: page.NextAfter, Type: "stream.closed", Data: json.RawMessage(`{"reconnectAfterMs":1000}`)})
			flusher.Flush()
			return
		case <-heartbeat.C:
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

func (d Dependencies) bridgeReceiverEvents(ctx context.Context, p identity.Principal) {
	changes, release := social.SubscribeReceiverEvents(d.DB, p)
	supervise.Go("api.receiver-events", func() {
		defer release()
		first := true
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-changes:
				if first && event.Kind == "resync" {
					first = false
					continue
				}
				first = false
				data, err := json.Marshal(event)
				if err != nil {
					continue
				}
				_ = dbwork.WithWriteTx(ctx, d.DB, dbwork.ClassInteractive, func(tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx, `INSERT INTO api_change_pending(audience,type,resource_kind,resource_id,revision,data) VALUES(?,'receiver.inbox_changed','receiver',?,'',?) ON CONFLICT(audience,type,resource_kind,resource_id) DO UPDATE SET data=excluded.data`, apievents.ProfileAudience(p.Authority, p.AccountID, p.ProfileID), event.ReceiverID, string(data))
					return err
				})
			}
		}
	})
}
