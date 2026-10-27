package eventfeed

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"portico.local/server/internal/apievents"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/supervise"
	"portico.local/server/internal/worker"
)

const PageLimit = 100

// The indexed minimum detects retention gaps. The durable sequence keeps the
// resume position when scheduled pruning empties the ring.
const eventBoundsSQL = `SELECT COALESCE((SELECT min(id) FROM api_events),0),COALESCE((SELECT seq FROM sqlite_sequence WHERE name='api_events'),0)`

type Resource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type Event struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	At       string          `json:"at"`
	Resource Resource        `json:"resource"`
	Revision string          `json:"revision"`
	Data     json.RawMessage `json:"data,omitempty"`
}
type Page struct {
	Events    []Event `json:"events"`
	NextAfter string  `json:"nextAfter"`
	More      bool    `json:"more"`
}

type Hub struct {
	DB      *sql.DB
	mu      sync.Mutex
	readers map[chan struct{}]map[string]bool
	streams map[string]stream
	next    uint64
	// LibraryVisible gates library: audiences. Nil delivers none.
	LibraryVisible func(tx *sql.Tx, p identity.Principal, library string) bool
}
type stream struct {
	id     uint64
	cancel context.CancelFunc
}

func New(db *sql.DB) *Hub {
	return &Hub{DB: db, readers: map[chan struct{}]map[string]bool{}, streams: map[string]stream{}}
}

// InstallationKey uses the live token-to-device binding, never a client-sent
// installation header. Two accounts on one installation have separate streams.
func (h *Hub) InstallationKey(ctx context.Context, p identity.Principal) (string, error) {
	var installation string
	err := h.DB.QueryRowContext(ctx, `SELECT d.installation_id FROM authorization_family_tokens t
 JOIN identity_device_families b ON b.family_id=t.family_id
 JOIN identity_devices d ON d.id=b.device_id WHERE t.token_hash=?`, p.Hash).Scan(&installation)
	if err != nil {
		return "", err
	}
	return operations.AccountKey(p) + ":" + installation, nil
}

// Run wakes subscribers from committed changes to the source tables. The
// outbox trigger is in that same commit; no idle client needs a polling loop.
func (h *Hub) Run(ctx context.Context) {
	signal := worker.NewSignal()
	unregister := dbwork.WakeOnTables(signal, "api_events", "notification_inbox", "console_operations", "api_change_pending")
	defer unregister()
	unregisterDirect := dbwork.WakeOnDirectWrites(signal, "api_events", "notification_inbox", "console_operations", "api_change_pending")
	defer unregisterDirect()
	// Security changes must re-authorize existing streams without idle polling.
	security := worker.NewSignal()
	stop := dbwork.WakeOnTableWrites(security, "authorization_session_families", "authorization_family_tokens", "accounts", "direct_memberships", "direct_profiles", "identity_devices", "libraries", "library_sources", "direct_profile_trust", "restrictions", "profile_restrictions", "restriction_outbox", "policy", "library_network_policy", "api_credentials", "hosted_profile_revisions")
	defer stop()
	supervise.Go("api.events.authority", func() {
		for security.Wait(ctx) {
			h.Wake()
		}
	})
	var after int64
	runChanges(ctx, signal, func(ctx context.Context) time.Duration {
		before := dbwork.ChangingCommits()
		count, err := flushPendingHints(ctx, h.DB)
		if err != nil {
			return 2 * time.Second
		}
		rows, err := h.DB.QueryContext(ctx, `SELECT id,audience FROM api_events WHERE id>? ORDER BY id LIMIT 1000`, after)
		if err != nil {
			return 2 * time.Second
		}
		audiences := map[string]bool{}
		read := 0
		for rows.Next() {
			var id int64
			var audience string
			if err = rows.Scan(&id, &audience); err != nil {
				break
			}
			after = id
			audiences[audience] = true
			read++
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return 2 * time.Second
		}
		h.wakeAudiences(audiences)
		if count == 64 || read == 1000 {
			return 2 * time.Second
		}
		retireOwnFlushWake(signal, before, count > 0)
		return 0
	})

}

// flushPendingHints publishes one bounded batch of coalesced hints. It probes
// the outbox on a read handle first, so a pass with nothing pending takes no
// writer gate at all: the gate is the one resource every scan write shares.
//
// The probe is a hint, never the authority. Flush re-reads and re-limits the
// batch inside the transaction, so a hint that lands after an empty probe is
// published by the pass its own commit wakes, and a second drainer that wins
// the race leaves an empty transaction rather than a lost or duplicated hint.
func flushPendingHints(ctx context.Context, db *sql.DB) (int, error) {
	var pending bool
	if err := dbwork.QueryRow(ctx, db, `SELECT EXISTS(SELECT 1 FROM api_change_pending)`).Scan(&pending); err != nil {
		return 0, err
	}
	if !pending {
		return 0, nil
	}
	count := 0
	err := dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		var flushErr error
		count, flushErr = apievents.Flush(ctx, tx, 64)
		return flushErr
	})
	return count, err
}

// A nonempty flush changes exactly one gated transaction; an empty flush
// changes none. Only changing commits can wake subscribers. Counting every
// successful transaction would mistake an unrelated no-change commit for a
// peer write and retain the flush's own already-covered wake. A real peer,
// including an unknown write scope, still retains its pass. A commit after
// this check wakes the signal normally.
func retireOwnFlushWake(signal *worker.Signal, before uint64, ownChange bool) {
	if !signal.Take() {
		return
	}
	covered := before
	if ownChange {
		covered++
	}
	if dbwork.ChangingCommits() != covered {
		signal.Wake()
	}
}

func (h *Hub) Wake() {
	h.mu.Lock()
	for ch := range h.readers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	h.mu.Unlock()
}
func (h *Hub) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.readers[ch] = nil
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.readers, ch)
		h.mu.Unlock()
	}
}

// AdmitStream permits one live stream per installation. A newcomer takes the
// place of the stream that was there: the old one is retired and the new one is
// admitted. A device that changed network reconnects before the server has
// noticed its old connection is dead, and refusing it then would turn every
// such reconnect into a failure; there is still only ever one subscription,
// and the newcomer resumes from the cursor it sends.
func (h *Hub) AdmitStream(key string, cancel context.CancelFunc) (func(), bool) {
	if key == "" || cancel == nil {
		return func() {}, false
	}
	h.mu.Lock()
	prior, replaced := h.streams[key]
	h.next++
	id := h.next
	h.streams[key] = stream{id: id, cancel: cancel}
	h.mu.Unlock()
	if replaced {
		prior.cancel()
	}
	return func() {
		h.mu.Lock()
		if current, exists := h.streams[key]; exists && current.id == id {
			delete(h.streams, key)
		}
		h.mu.Unlock()
	}, true
}

// administrative is a verified owner or administrator: both hear the admin
// audience's list hints; only the owner's account inbox is the owner's.
func (h *Hub) Read(ctx context.Context, p identity.Principal, administrative bool, after int64, supplied bool) (Page, error) {
	out := Page{Events: []Event{}, NextAfter: "0"}
	gate, err := dbwork.BeginSnapshot(ctx, h.DB)
	if err != nil {
		return out, err
	}
	defer gate.Rollback()
	tx := gate.Tx()
	capacityAudience := ""
	if check := capacityRecipient(ctx); check != nil {
		if err = check(ctx, tx, p); err != nil {
			return out, err
		}
		capacityAudience = apievents.CapacityAudience(p.Authority, p.AccountID)
	}
	var minimum, maximum int64
	if err = tx.QueryRowContext(ctx, eventBoundsSQL).Scan(&minimum, &maximum); err != nil {
		return out, err
	}
	out.NextAfter = strconv.FormatInt(maximum, 10)
	if !supplied {
		return out, gate.Commit()
	}
	if after > maximum || minimum > 0 && after < minimum-1 || minimum == 0 && after < maximum {
		out.Events = append(out.Events, Event{ID: out.NextAfter, Type: "stream.resync", At: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), Resource: Resource{Kind: "stream", ID: "all"}, Revision: out.NextAfter})
		return out, gate.Commit()
	}
	adminInbox := administrative && p.Role == "owner" && p.Authority == "local"
	audience := apievents.ProfileAudience(p.Authority, p.AccountID, p.ProfileID)
	var deviceID string
	if err = tx.QueryRowContext(ctx, `SELECT device_id FROM identity_device_families b JOIN authorization_family_tokens t ON t.family_id=b.family_id WHERE t.token_hash=?`, p.Hash).Scan(&deviceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Page{}, err
	}
	deviceAudience := ""
	if deviceID != "" {
		deviceAudience = apievents.DeviceAudience(deviceID)
	}
	adminAudience := ""
	operationAudience := ""
	if adminInbox {
		adminAudience = apievents.AccountAudience(p.Authority, p.AccountID)
	}
	if administrative {
		operationAudience = apievents.AdminAudience
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,audience,type,resource_kind,resource_id,revision,data,at_ms FROM api_events WHERE id>? AND (audience IN (?,?,?,?,?,'server') OR audience >= 'library:' AND audience < 'library;') ORDER BY id LIMIT ?`, after, audience, deviceAudience, adminAudience, operationAudience, capacityAudience, PageLimit+1)
	if err != nil {
		return out, err
	}
	examined := 0
	var lastExamined int64
	// All events and access checks use this one snapshot: each library's
	// verdict is stable for the page, including denied libraries.
	visibility := map[string]bool{}
	for rows.Next() {
		var id, at int64
		var rowAudience, kind, resourceKind, resource, revision, data string
		if err = rows.Scan(&id, &rowAudience, &kind, &resourceKind, &resource, &revision, &data, &at); err != nil {
			break
		}
		examined++
		lastExamined = id
		// A capacity hint cannot arrive through server/admin/profile audiences,
		// or carry even a private producer's accidental metadata fields.
		if kind == "capacity.changed" {
			if capacityAudience == "" || rowAudience != capacityAudience || resourceKind != "" || resource != "" || revision != "" || data != "" {
				continue
			}
		} else if capacityAudience != "" && rowAudience == capacityAudience {
			continue
		}
		if strings.HasPrefix(rowAudience, "library:") {
			if h.LibraryVisible == nil {
				continue
			}
			library := strings.TrimPrefix(rowAudience, "library:")
			visible, checked := visibility[library]
			if !checked {
				visible = h.LibraryVisible(tx, p, library)
				visibility[library] = visible
			}
			if !visible {
				continue
			}
		}
		event := Event{ID: strconv.FormatInt(id, 10), Type: kind, At: time.UnixMilli(at).UTC().Format("2006-01-02T15:04:05.000Z"), Resource: Resource{Kind: resourceKind, ID: resource}, Revision: revision}
		if json.Valid([]byte(data)) {
			event.Data = json.RawMessage(data)
		}
		out.Events = append(out.Events, event)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return Page{}, err
	}
	if len(out.Events) > PageLimit {
		out.Events = out.Events[:PageLimit]
		out.NextAfter = out.Events[PageLimit-1].ID
		out.More = true
	} else if examined == PageLimit+1 {
		out.More = true
		out.NextAfter = strconv.FormatInt(lastExamined, 10)
	}
	return out, gate.Commit()
}

// SubscribeFor filters private profile/device changes in memory. Library hints
// still pass the snapshot authorization check in Read before delivery.
func (h *Hub) SubscribeFor(p identity.Principal, administrative bool) (<-chan struct{}, func()) {
	return h.subscribeFor(p, administrative, false)
}
func (h *Hub) subscribeFor(p identity.Principal, administrative, capacity bool) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	audiences := map[string]bool{apievents.ProfileAudience(p.Authority, p.AccountID, p.ProfileID): true, "server": true}
	if p.DeviceID != "" {
		audiences[apievents.DeviceAudience(p.DeviceID)] = true
	}
	if capacity {
		audiences[apievents.CapacityAudience(p.Authority, p.AccountID)] = true
	}
	if administrative {
		audiences[apievents.AdminAudience] = true
		if p.Role == "owner" {
			audiences[apievents.AccountAudience(p.Authority, p.AccountID)] = true
		}
	}
	h.mu.Lock()
	h.readers[ch] = audiences
	h.mu.Unlock()
	return ch, func() { h.mu.Lock(); delete(h.readers, ch); h.mu.Unlock() }
}
func (h *Hub) wakeAudiences(changed map[string]bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, scope := range h.readers {
		relevant := false
		for audience := range changed {
			if scope == nil || scope[audience] || strings.HasPrefix(audience, "library:") {
				relevant = true
				break
			}
		}
		if relevant {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
}

func (h *Hub) TokenDeadline(ctx context.Context, p identity.Principal) (time.Time, error) {
	var expiry string
	err := h.DB.QueryRowContext(ctx, `SELECT min(t.expires_at,f.authorization_horizon) FROM authorization_family_tokens t JOIN authorization_session_families f ON f.id=t.family_id WHERE t.token_hash=?`, p.Hash).Scan(&expiry)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, expiry)
}

// runChanges sleeps indefinitely when no committed changes need delivery.
func runChanges(ctx context.Context, signal *worker.Signal, step func(context.Context) time.Duration) {
	var last time.Time
	for ctx.Err() == nil {
		if !last.IsZero() {
			remaining := 2*time.Second - time.Since(last)
			if remaining > 0 {
				timer := time.NewTimer(remaining)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}
		delay := step(ctx)
		last = time.Now()
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		} else if !signal.Wait(ctx) {
			return
		}
	}
}
