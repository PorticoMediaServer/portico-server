package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	longPollDefaultWaitSeconds = 20
	longPollMaximumWaitSeconds = 25
	longPollMaximumPerOwner    = 4
	longPollMaximumTotal       = 512
	longPollCursorLifetime     = 5 * time.Minute
	// Mutations wake the exact long-poll subscription immediately. This slower
	// fallback exists only to catch out-of-band credential expiry/revocation;
	// polling every 500ms multiplied the full authentication query path by every
	// connected client even while the server was idle.
	longPollAuthorizationCheck = 15 * time.Second
	longPollMaximumAppEvents   = 32
	longPollMaximumBytes       = 128 * 1024
	longPollRetainedAppEvents  = 256
	longPollRetainedAppBytes   = 512 * 1024
)

var (
	errLongPollAuthorizationLost = errors.New("long-poll authorization changed")
	errLongPollShutdown          = errors.New("long-poll server shutdown")
)

type longPollRuntime struct {
	broker *longPollBroker
	bootID string
	key    []byte

	mu      sync.Mutex
	active  int
	byOwner map[string]int
	logical map[string]struct{}
}

type longPollBroker struct {
	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
	app     []longPollAppRecord
	appSize int
	floor   uint64
}

type longPollAppRecord struct {
	event AppEvent
	size  int
}

type longPollCursorClaims struct {
	Version  string `json:"v"`
	BootID   string `json:"b"`
	Kind     string `json:"k"`
	Scope    string `json:"s"`
	Position uint64 `json:"p"`
	Marker   string `json:"m,omitempty"`
	Expires  int64  `json:"e"`
}

type longPollRequest struct {
	cursor      string
	wait        time.Duration
	waitSeconds int
}

type longPollEnvelope struct {
	Version       string `json:"version"`
	Cursor        string `json:"cursor"`
	ServerTime    string `json:"serverTime"`
	ResetRequired bool   `json:"resetRequired"`
	HasMore       bool   `json:"hasMore"`
	Events        any    `json:"events"`
}

type notificationInvalidationEvent struct {
	Version    string `json:"version"`
	Kind       string `json:"kind"`
	OccurredAt string `json:"occurredAt"`
}

type longPollDiscardWriter struct{ header http.Header }

const streamWriteDeadline = 10 * time.Second

func prepareStreamWrite(w http.ResponseWriter) {
	if w == nil {
		return
	}
	// ResponseController keeps a slow socket from pinning an SSE handler past
	// the listener shutdown budget. Implementations that do not expose a
	// deadline simply keep the existing best-effort streaming behavior.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(streamWriteDeadline))
}

func streamResumeResetRequested(r *http.Request, w http.ResponseWriter, flusher http.Flusher) bool {
	if r == nil || strings.TrimSpace(r.Header.Get("Last-Event-ID")) == "" {
		return false
	}
	prepareStreamWrite(w)
	_, _ = fmt.Fprint(w, "event: stream-reset\ndata: {\"resetRequired\":true,\"reason\":\"resume_not_supported\"}\n\n")
	flusher.Flush()
	return true
}

func (w *longPollDiscardWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (*longPollDiscardWriter) Write([]byte) (int, error) { return 0, nil }
func (*longPollDiscardWriter) WriteHeader(int)           {}

func newLongPollRuntime() *longPollRuntime {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		fallback := sha256.Sum256([]byte(randomID("long-poll") + time.Now().UTC().Format(time.RFC3339Nano)))
		copy(key, fallback[:])
	}
	boot := make([]byte, 16)
	if _, err := rand.Read(boot); err != nil {
		copy(boot, key[:16])
	}
	return &longPollRuntime{
		broker:  &longPollBroker{waiters: map[string]map[chan struct{}]struct{}{}},
		bootID:  base64.RawURLEncoding.EncodeToString(boot),
		key:     key,
		byOwner: map[string]int{},
		logical: map[string]struct{}{},
	}
}

func (s *Server) ensureLongPollRuntime() *longPollRuntime {
	s.longPollMu.Lock()
	defer s.longPollMu.Unlock()
	if s.longPoll == nil {
		s.longPoll = newLongPollRuntime()
	}
	return s.longPoll
}

func (b *longPollBroker) subscribe(key string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	waiters := b.waiters[key]
	if waiters == nil {
		waiters = map[chan struct{}]struct{}{}
		b.waiters[key] = waiters
	}
	waiters[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if waiters := b.waiters[key]; waiters != nil {
			delete(waiters, ch)
			if len(waiters) == 0 {
				delete(b.waiters, key)
			}
		}
		b.mu.Unlock()
		// Deliberately do not close ch. Publishers copy no channel references,
		// and waiters always leave through cancellation, timeout, or shutdown.
	}
}

func (b *longPollBroker) publish(key string) {
	b.mu.Lock()
	for waiter := range b.waiters[key] {
		select {
		case waiter <- struct{}{}:
		default:
		}
	}
	b.mu.Unlock()
}

func (b *longPollBroker) publishApp(event AppEvent) {
	raw, _ := json.Marshal(event)
	record := longPollAppRecord{event: event, size: len(raw)}
	b.mu.Lock()
	b.app = append(b.app, record)
	b.appSize += record.size
	for len(b.app) > longPollRetainedAppEvents || b.appSize > longPollRetainedAppBytes {
		removed := b.app[0]
		b.app = b.app[1:]
		b.appSize -= removed.size
		b.floor = removed.event.ID
	}
	for waiter := range b.waiters[longPollAppBrokerKey] {
		select {
		case waiter <- struct{}{}:
		default:
		}
	}
	b.mu.Unlock()
}

func (b *longPollBroker) appPosition() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.app) == 0 {
		return b.floor
	}
	return b.app[len(b.app)-1].event.ID
}

func (b *longPollBroker) appEventsAfter(position uint64) (events []AppEvent, next uint64, hasMore, overflow bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	current := b.floor
	if len(b.app) > 0 {
		current = b.app[len(b.app)-1].event.ID
	}
	if position > current || position < b.floor {
		return nil, current, false, true
	}
	bytes := 0
	for _, record := range b.app {
		if record.event.ID <= position {
			continue
		}
		if len(events) >= longPollMaximumAppEvents || (len(events) > 0 && bytes+record.size > longPollMaximumBytes) {
			hasMore = true
			break
		}
		events = append(events, record.event)
		bytes += record.size
		next = record.event.ID
	}
	if len(events) == 0 {
		next = position
	}
	return events, next, hasMore, false
}

func (r *longPollRuntime) digest(parts ...string) string {
	mac := hmac.New(sha256.New, r.key)
	for _, part := range parts {
		_, _ = mac.Write([]byte(part))
		_, _ = mac.Write([]byte{0})
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (r *longPollRuntime) signCursor(claims longPollCursorClaims) (string, error) {
	claims.Version = "v1"
	claims.BootID = r.bootID
	claims.Expires = time.Now().UTC().Add(longPollCursorLifetime).Unix()
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, r.key)
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (r *longPollRuntime) parseCursor(raw, kind, scope string) (longPollCursorClaims, bool, error) {
	if raw == "" {
		return longPollCursorClaims{}, false, nil
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || len(raw) > 4096 {
		return longPollCursorClaims{}, false, errors.New("cursor encoding is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return longPollCursorClaims{}, false, errors.New("cursor signature is invalid")
	}
	mac := hmac.New(sha256.New, r.key)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return longPollCursorClaims{}, false, errors.New("cursor signature is invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return longPollCursorClaims{}, false, errors.New("cursor encoding is invalid")
	}
	var claims longPollCursorClaims
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Version != "v1" || claims.Kind == "" || claims.Scope == "" {
		return longPollCursorClaims{}, false, errors.New("cursor payload is invalid")
	}
	if claims.Kind != kind || !hmac.Equal([]byte(claims.Scope), []byte(scope)) {
		return longPollCursorClaims{}, false, errors.New("cursor scope is invalid")
	}
	reset := claims.BootID != r.bootID || time.Now().UTC().Unix() > claims.Expires
	return claims, reset, nil
}

func (r *longPollRuntime) acquire(owner, logical string) (func(), int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.logical[logical]; exists {
		return nil, http.StatusConflict
	}
	if r.active >= longPollMaximumTotal || r.byOwner[owner] >= longPollMaximumPerOwner {
		return nil, http.StatusTooManyRequests
	}
	r.active++
	r.byOwner[owner]++
	r.logical[logical] = struct{}{}
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.active--
		r.byOwner[owner]--
		if r.byOwner[owner] == 0 {
			delete(r.byOwner, owner)
		}
		delete(r.logical, logical)
	}, 0
}

const longPollAppBrokerKey = "app-events"

func parseLongPollRequest(w http.ResponseWriter, r *http.Request, extraQuery ...string) (longPollRequest, bool) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return longPollRequest{}, false
	}
	if r.ContentLength > 0 || len(r.TransferEncoding) > 0 {
		writeError(w, http.StatusBadRequest, "long_poll_body_not_allowed", "Long-poll requests do not accept a request body.")
		return longPollRequest{}, false
	}
	allowed := map[string]bool{"cursor": true, "waitSeconds": true}
	for _, key := range extraQuery {
		allowed[key] = true
	}
	for key, values := range r.URL.Query() {
		if !allowed[key] || len(values) != 1 {
			writeError(w, http.StatusBadRequest, "invalid_long_poll_query", "Long-poll query parameters are invalid.")
			return longPollRequest{}, false
		}
	}
	waitSeconds := longPollDefaultWaitSeconds
	if raw, exists := r.URL.Query()["waitSeconds"]; exists {
		parsed, err := strconv.Atoi(raw[0])
		if err != nil || parsed < 0 || parsed > longPollMaximumWaitSeconds {
			writeError(w, http.StatusBadRequest, "invalid_wait_seconds", "waitSeconds must be an integer from 0 through 25.")
			return longPollRequest{}, false
		}
		waitSeconds = parsed
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if len(cursor) > 4096 {
		writeError(w, http.StatusBadRequest, "invalid_poll_cursor", "The long-poll cursor is invalid.")
		return longPollRequest{}, false
	}
	return longPollRequest{cursor: cursor, waitSeconds: waitSeconds, wait: time.Duration(waitSeconds) * time.Second}, true
}

func (s *Server) longPollPrincipalScope(r *http.Request, user User, kind, resource, audience string) (owner, scope string, err error) {
	ctx := r.Context()
	settings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return "", "", err
	}
	serverID, err := s.localServerIDContext(ctx, settings)
	if err != nil {
		return "", "", err
	}
	authority := viewerAuthorityForAuthProvider(user.AuthProvider)
	// Cursors survive ordinary access-token rotation and authorization-revision
	// refreshes. Authorization is revalidated independently before and during
	// every poll; binding a cursor to the rotating bearer made a successful
	// refresh terminal because the retried cursor could only produce HTTP 400.
	// Account, profile, server and stable device/API-key identity still prevent
	// replay across viewers or installations.
	owner = s.longPoll.digest("owner", authority, accountIDForUser(user), viewerProfileID(user), user.DeviceID, user.APIKeyID)
	scope = s.longPoll.digest("scope", "v1", kind, resource, audience, authority, accountIDForUser(user), serverID,
		viewerProfileID(user), user.DeviceID, user.APIKeyID)
	return owner, scope, nil
}

func (s *Server) longPollUserAuthorizationCurrent(r *http.Request, expected User) bool {
	w := &longPollDiscardWriter{}
	current, ok, err := s.currentUserWithError(w, r.Clone(r.Context()))
	if err != nil || !ok {
		return false
	}
	currentScopes := append([]string(nil), current.APIKeyScopes...)
	expectedScopes := append([]string(nil), expected.APIKeyScopes...)
	sort.Strings(currentScopes)
	sort.Strings(expectedScopes)
	return accountIDForUser(current) == accountIDForUser(expected) &&
		viewerProfileID(current) == viewerProfileID(expected) &&
		current.DeviceID == expected.DeviceID && current.APIKeyID == expected.APIKeyID &&
		strings.Join(currentScopes, "\x00") == strings.Join(expectedScopes, "\x00") &&
		s.authorizationRevisionForUserContext(r.Context(), current) == s.authorizationRevisionForUserContext(r.Context(), expected)
}

func (s *Server) beginLongPoll(w http.ResponseWriter, owner, logical string) (func(), bool) {
	release, status := s.longPoll.acquire(owner, logical)
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "1")
		writeError(w, status, "long_poll_busy", "Too many long-poll requests are active. Try again shortly.")
		return nil, false
	}
	if status == http.StatusConflict {
		writeError(w, status, "long_poll_already_active", "A long-poll request is already active for this stream.")
		return nil, false
	}
	return release, true
}

func (s *Server) waitLongPoll(ctx context.Context, signal <-chan struct{}, wait time.Duration, authorized func() bool, changed func() (bool, error)) (bool, error) {
	if wait <= 0 {
		return false, nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	ticker := time.NewTicker(longPollAuthorizationCheck)
	defer ticker.Stop()
	shutdown := s.shutdownDone()
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-shutdown:
			return false, errLongPollShutdown
		case <-timer.C:
			if !authorized() {
				return false, errLongPollAuthorizationLost
			}
			return false, nil
		case <-signal:
			if !authorized() {
				return false, errLongPollAuthorizationLost
			}
			didChange, err := changed()
			if err != nil || didChange {
				return didChange, err
			}
		case <-ticker.C:
			if !authorized() {
				return false, errLongPollAuthorizationLost
			}
		}
	}
}

func writeLongPollShutdown(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, errLongPollShutdown) {
		return false
	}
	w.Header().Set("Retry-After", "1")
	writeError(w, http.StatusServiceUnavailable, "server_shutting_down", "The server is shutting down; reconnect shortly.")
	return true
}

func writeLongPollCursorError(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, "invalid_poll_cursor", "The long-poll cursor is invalid for this stream.")
}

func writeLongPollEnvelope(w http.ResponseWriter, cursor string, reset, hasMore bool, events any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, longPollEnvelope{
		Version: "v1", Cursor: cursor, ServerTime: time.Now().UTC().Format(time.RFC3339Nano),
		ResetRequired: reset, HasMore: hasMore, Events: events,
	})
}

func nonnegativeUint64(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func (s *Server) nextLongPollCursor(kind, scope string, position uint64, marker string) (string, error) {
	return s.longPoll.signCursor(longPollCursorClaims{Kind: kind, Scope: scope, Position: position, Marker: marker})
}

func (s *Server) longPollMarker(values ...string) string {
	if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
		return ""
	}
	return s.longPoll.digest(append([]string{"marker"}, values...)...)
}

func (s *Server) handleAppEventsPoll(w http.ResponseWriter, r *http.Request, user User) {
	req, ok := parseLongPollRequest(w, r)
	if !ok {
		return
	}
	owner, scope, err := s.longPollPrincipalScope(r, user, "app", "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "long_poll_failed", "Unable to initialize application updates.")
		return
	}
	claims, reset, err := s.longPoll.parseCursor(req.cursor, "app", scope)
	if err != nil {
		writeLongPollCursorError(w)
		return
	}
	release, ok := s.beginLongPoll(w, owner, s.longPoll.digest("logical", scope))
	if !ok {
		return
	}
	defer release()
	signal, unsubscribe := s.longPoll.broker.subscribe(longPollAppBrokerKey)
	defer unsubscribe()
	if req.cursor == "" || reset {
		position := s.longPoll.broker.appPosition()
		cursor, _ := s.nextLongPollCursor("app", scope, position, "")
		writeLongPollEnvelope(w, cursor, reset, false, []AppEvent{})
		return
	}
	read := func() ([]AppEvent, uint64, bool, bool) {
		events, next, hasMore, overflow := s.longPoll.broker.appEventsAfter(claims.Position)
		return s.projectAppEventsForUserContext(r.Context(), user, events), next, hasMore, overflow
	}
	events, next, hasMore, overflow := read()
	if overflow {
		cursor, _ := s.nextLongPollCursor("app", scope, s.longPoll.broker.appPosition(), "")
		writeLongPollEnvelope(w, cursor, true, false, []AppEvent{})
		return
	}
	if len(events) == 0 && next <= claims.Position {
		_, err = s.waitLongPoll(r.Context(), signal, req.wait,
			func() bool { return s.longPollUserAuthorizationCurrent(r, user) },
			func() (bool, error) {
				items, candidateNext, _, overflow := read()
				return len(items) > 0 || candidateNext > claims.Position || overflow, nil
			})
		if err != nil {
			if writeLongPollShutdown(w, err) {
				return
			}
			if errors.Is(err, errLongPollAuthorizationLost) {
				writeError(w, http.StatusUnauthorized, "authorization_changed", "Authorization changed while waiting for updates.")
			}
			return
		}
		events, next, hasMore, overflow = read()
	}
	if !s.longPollUserAuthorizationCurrent(r, user) {
		writeError(w, http.StatusUnauthorized, "authorization_changed", "Authorization changed while waiting for updates.")
		return
	}
	if overflow {
		next = s.longPoll.broker.appPosition()
		events = []AppEvent{}
		hasMore = false
		reset = true
	}
	cursor, _ := s.nextLongPollCursor("app", scope, next, "")
	writeLongPollEnvelope(w, cursor, reset, hasMore, events)
}

func (s *Server) notificationLongPollKey(recipient notificationRecipient) string {
	return "notification:" + s.longPoll.digest(recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience)
}

func (s *Server) notifyLongPollNotification(recipient notificationRecipient) {
	runtime := s.ensureLongPollRuntime()
	runtime.broker.publish("notification:" + runtime.digest(recipient.Authority, recipient.AccountID, recipient.ServerID, recipient.ProfileID, recipient.Audience))
}

func (s *Server) notificationLongPollPrincipalScope(recipient notificationRecipient, authorization notificationStreamAuthorization) (owner, scope string) {
	owner = s.longPoll.digest("notification-owner", recipient.Authority, recipient.AccountID, recipient.ProfileID,
		authorization.InstallationID, authorization.DeviceID)
	scope = s.longPoll.digest("scope", "v1", "notification", recipient.Authority, recipient.AccountID, recipient.ServerID,
		recipient.ProfileID, recipient.Audience, authorization.InstallationID, authorization.DeviceID)
	return owner, scope
}

func (s *Server) handleViewerNotificationEventsPoll(w http.ResponseWriter, r *http.Request, user User) {
	req, ok := parseLongPollRequest(w, r, "audience")
	if !ok {
		return
	}
	recipient, ok := s.notificationRecipientForRequest(w, r, user)
	if !ok {
		return
	}
	authorization, err := s.notificationStreamAuthorizationContext(r.Context(), r, user, recipient)
	if err != nil {
		writeProductError(w, http.StatusForbidden, "interactive_session_required", "Notifications require a signed-in app session.")
		return
	}
	owner, scope := s.notificationLongPollPrincipalScope(recipient, authorization)
	claims, reset, err := s.longPoll.parseCursor(req.cursor, "notification", scope)
	if err != nil {
		writeLongPollCursorError(w)
		return
	}
	release, ok := s.beginLongPoll(w, owner, s.longPoll.digest("logical", scope))
	if !ok {
		return
	}
	defer release()
	signal, unsubscribe := s.longPoll.broker.subscribe(s.notificationLongPollKey(recipient))
	defer unsubscribe()
	revision, err := notificationRevisionContext(s, r.Context(), recipient)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_poll_failed", "Unable to load notification updates.")
		return
	}
	events := []notificationInvalidationEvent{}
	if req.cursor == "" || (!reset && nonnegativeUint64(revision) != claims.Position) {
		events = append(events, notificationInvalidationEvent{Version: "v1", Kind: "notifications.invalidated", OccurredAt: time.Now().UTC().Format(time.RFC3339Nano)})
	}
	if req.cursor != "" && !reset && claims.Position > nonnegativeUint64(revision) {
		reset = true
		events = nil
	}
	if len(events) == 0 && !reset {
		base := revision
		_, err = s.waitLongPoll(r.Context(), signal, req.wait,
			func() bool {
				return s.longPollUserAuthorizationCurrent(r, user) && s.notificationStreamAuthorizationCurrentContext(r.Context(), recipient, authorization)
			},
			func() (bool, error) {
				current, readErr := notificationRevisionContext(s, r.Context(), recipient)
				return current != base, readErr
			})
		if err != nil {
			if writeLongPollShutdown(w, err) {
				return
			}
			if errors.Is(err, errLongPollAuthorizationLost) {
				if !s.longPollUserAuthorizationCurrent(r, user) {
					writeProductError(w, http.StatusUnauthorized, "authorization_changed", "Authorization changed while waiting for notification updates.")
				} else {
					writeProductError(w, http.StatusForbidden, "authorization_changed", "Notification access changed while waiting for updates.")
				}
			}
			return
		}
		revision, err = notificationRevisionContext(s, r.Context(), recipient)
		if err != nil {
			return
		}
		if revision != base {
			events = append(events, notificationInvalidationEvent{Version: "v1", Kind: "notifications.invalidated", OccurredAt: time.Now().UTC().Format(time.RFC3339Nano)})
		}
	}
	if !s.longPollUserAuthorizationCurrent(r, user) {
		writeProductError(w, http.StatusUnauthorized, "authorization_changed", "Authorization changed while waiting for notification updates.")
		return
	}
	if !s.notificationStreamAuthorizationCurrentContext(r.Context(), recipient, authorization) {
		writeProductError(w, http.StatusForbidden, "authorization_changed", "Notification access changed while waiting for updates.")
		return
	}
	cursor, _ := s.nextLongPollCursor("notification", scope, nonnegativeUint64(revision), "")
	writeLongPollEnvelope(w, cursor, reset, false, events)
}

func (s *Server) handlePlaybackCommandEventsPoll(w http.ResponseWriter, r *http.Request, user User, sessionID string) {
	req, ok := parseLongPollRequest(w, r)
	if !ok {
		return
	}
	owner, scope, err := s.longPollPrincipalScope(r, user, "playback-command", sessionID, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "long_poll_failed", "Unable to initialize playback command updates.")
		return
	}
	claims, reset, err := s.longPoll.parseCursor(req.cursor, "playback-command", scope)
	if err != nil {
		writeLongPollCursorError(w)
		return
	}
	release, ok := s.beginLongPoll(w, owner, s.longPoll.digest("logical", scope))
	if !ok {
		return
	}
	defer release()
	signal, unsubscribe := s.longPoll.broker.subscribe("playback-command:" + sessionID)
	defer unsubscribe()
	command, err := s.playbackCommand(user, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "playback_session_not_found", "Playback session was not found.")
		} else {
			writeError(w, http.StatusInternalServerError, "playback_command_failed", "Unable to load playback commands.")
		}
		return
	}
	marker := s.longPollMarker(command.ID)
	events := []PlaybackCommand{}
	if req.cursor == "" && command.ID != "" {
		events = append(events, command)
	} else if !reset && command.ID != "" && marker != claims.Marker {
		events = append(events, command)
	}
	if len(events) == 0 && !reset {
		_, err = s.waitLongPoll(r.Context(), signal, req.wait,
			func() bool { return s.longPollUserAuthorizationCurrent(r, user) },
			func() (bool, error) {
				current, readErr := s.playbackCommand(user, sessionID)
				return readErr == nil && current.ID != "" && s.longPollMarker(current.ID) != claims.Marker, readErr
			})
		if err != nil {
			if writeLongPollShutdown(w, err) {
				return
			}
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "playback_session_not_found", "Playback session was not found.")
			} else if errors.Is(err, errLongPollAuthorizationLost) {
				writeError(w, http.StatusUnauthorized, "authorization_changed", "Authorization changed while waiting for playback commands.")
			}
			return
		}
		command, err = s.playbackCommand(user, sessionID)
		if err != nil {
			return
		}
		marker = s.longPollMarker(command.ID)
		if command.ID != "" && marker != claims.Marker {
			events = append(events, command)
		}
	}
	if !s.longPollUserAuthorizationCurrent(r, user) {
		writeError(w, http.StatusUnauthorized, "authorization_changed", "Authorization changed while waiting for playback commands.")
		return
	}
	cursor, _ := s.nextLongPollCursor("playback-command", scope, 0, marker)
	writeLongPollEnvelope(w, cursor, reset, false, events)
}

func (s *Server) materializeWatchWithFriendsEvent(ctx context.Context, user User, groupID string) (WatchWithFriendsGroup, bool, error) {
	group, err := s.watchWithFriendsGroupForUserContext(ctx, user, groupID, true)
	if err == nil {
		return group, false, nil
	}
	ended, endedErr := s.watchWithFriendsGroupSnapshotForUserContext(ctx, user, groupID, true)
	if endedErr == nil && ended.State == "stopped" {
		return ended, true, nil
	}
	return WatchWithFriendsGroup{}, false, err
}

func (s *Server) handleWatchWithFriendsGroupEventsPoll(w http.ResponseWriter, r *http.Request, user User, groupID string) {
	req, ok := parseLongPollRequest(w, r)
	if !ok {
		return
	}
	owner, scope, err := s.longPollPrincipalScope(r, user, "watch-with-friends", groupID, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "long_poll_failed", "Unable to initialize Watch With Friends updates.")
		return
	}
	claims, reset, err := s.longPoll.parseCursor(req.cursor, "watch-with-friends", scope)
	if err != nil {
		writeLongPollCursorError(w)
		return
	}
	release, ok := s.beginLongPoll(w, owner, s.longPoll.digest("logical", scope))
	if !ok {
		return
	}
	defer release()
	signal, unsubscribe := s.longPoll.broker.subscribe("watch-with-friends:" + groupID)
	defer unsubscribe()
	group, ended, err := s.materializeWatchWithFriendsEvent(r.Context(), user, groupID)
	if err != nil {
		writeError(w, http.StatusNotFound, "watch_with_friends_not_found", "Watch With Friends group was not found.")
		return
	}
	events := []WatchWithFriendsGroup{}
	if req.cursor == "" || (!reset && claims.Position != nonnegativeUint64(group.Revision)) {
		events = append(events, group)
	}
	if req.cursor != "" && !reset && claims.Position > nonnegativeUint64(group.Revision) {
		reset = true
		events = nil
	}
	if len(events) == 0 && !reset && !ended {
		base := group.Revision
		_, err = s.waitLongPoll(r.Context(), signal, req.wait,
			func() bool { return s.longPollUserAuthorizationCurrent(r, user) },
			func() (bool, error) {
				current, _, readErr := s.materializeWatchWithFriendsEvent(r.Context(), user, groupID)
				return readErr == nil && current.Revision != base, readErr
			})
		if err != nil {
			if writeLongPollShutdown(w, err) {
				return
			}
			if errors.Is(err, errLongPollAuthorizationLost) {
				writeError(w, http.StatusUnauthorized, "authorization_changed", "Authorization changed while waiting for Watch With Friends updates.")
			}
			return
		}
		group, ended, err = s.materializeWatchWithFriendsEvent(r.Context(), user, groupID)
		if err != nil {
			return
		}
		if group.Revision != base {
			events = append(events, group)
		}
	}
	// The ended snapshot is authorized by membership and media access, even
	// though the active-stream authorization intentionally becomes false.
	if !s.longPollUserAuthorizationCurrent(r, user) {
		writeError(w, http.StatusUnauthorized, "authorization_changed", "Authorization changed while waiting for Watch With Friends updates.")
		return
	}
	if !ended && !s.watchWithFriendsStreamAuthorizedContext(r.Context(), user, groupID) {
		writeError(w, http.StatusForbidden, "authorization_changed", "Watch With Friends access changed while waiting for updates.")
		return
	}
	cursor, _ := s.nextLongPollCursor("watch-with-friends", scope, nonnegativeUint64(group.Revision), s.longPollMarker(group.State))
	writeLongPollEnvelope(w, cursor, reset, false, events)
}

func longPollCursorFingerprint(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:8])
}

// Kept private so callers cannot accidentally log opaque cursors. This helper
// is intended only for low-cardinality diagnostics and tests.
func formatLongPollStream(kind, resource string) string {
	return fmt.Sprintf("%s:%s", strings.TrimSpace(kind), strings.TrimSpace(resource))
}
