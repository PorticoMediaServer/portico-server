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
	"portico.local/server/internal/worker"
)

const PageLimit = 100

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
}

type Hub struct {
	DB      *sql.DB
	mu      sync.Mutex
	readers map[chan struct{}]struct{}
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
	return &Hub{DB: db, readers: map[chan struct{}]struct{}{}, streams: map[string]stream{}}
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
	unregister := dbwork.WakeOnTables(signal, "api_events", "notification_inbox", "console_operations")
	defer unregister()
	unregisterDirect := dbwork.WakeOnDirectWrites(signal, "api_events", "notification_inbox", "console_operations")
	defer unregisterDirect()
	worker.Run(ctx, "api.events", signal, func(context.Context) time.Duration {
		h.Wake()
		return 0
	})
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
	h.readers[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.readers, ch)
		h.mu.Unlock()
	}
}

// AdmitStream permits one live stream per installation. A duplicate retires
// the old stream and receives a retryable conflict, so its next attempt starts
// from the same durable cursor instead of silently keeping two subscriptions.
func (h *Hub) AdmitStream(key string, cancel context.CancelFunc) (func(), bool) {
	if key == "" || cancel == nil {
		return func() {}, false
	}
	h.mu.Lock()
	if prior, exists := h.streams[key]; exists {
		delete(h.streams, key)
		h.mu.Unlock()
		prior.cancel()
		return func() {}, false
	}
	h.next++
	id := h.next
	h.streams[key] = stream{id: id, cancel: cancel}
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		if current, exists := h.streams[key]; exists && current.id == id {
			delete(h.streams, key)
		}
		h.mu.Unlock()
	}, true
}

func (h *Hub) Read(ctx context.Context, p identity.Principal, owner bool, after int64, supplied bool) (Page, error) {
	out := Page{Events: []Event{}, NextAfter: "0"}
	gate, err := dbwork.BeginSnapshot(ctx, h.DB)
	if err != nil {
		return out, err
	}
	defer gate.Rollback()
	tx := gate.Tx()
	var minimum, maximum int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(min(id),0),COALESCE(max(id),0) FROM api_events`).Scan(&minimum, &maximum); err != nil {
		return out, err
	}
	out.NextAfter = strconv.FormatInt(maximum, 10)
	if !supplied {
		return out, gate.Commit()
	}
	if after > maximum || minimum > 0 && after < minimum-1 {
		out.Events = append(out.Events, Event{ID: out.NextAfter, Type: "stream.resync", At: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), Resource: Resource{Kind: "stream", ID: "all"}, Revision: out.NextAfter})
		return out, gate.Commit()
	}
	adminInbox := owner && p.Authority == "local"
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
	if owner {
		operationAudience = apievents.AdminAudience
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,audience,type,resource_kind,resource_id,revision,data,at_ms FROM api_events WHERE id>? AND (audience IN (?,?,?,?) OR audience >= 'library:' AND audience < 'library;') ORDER BY id LIMIT ?`, after, audience, deviceAudience, adminAudience, operationAudience, PageLimit+1)
	if err != nil {
		return out, err
	}
	examined := 0
	var lastExamined int64
	for rows.Next() {
		var id, at int64
		var rowAudience, kind, resourceKind, resource, revision, data string
		if err = rows.Scan(&id, &rowAudience, &kind, &resourceKind, &resource, &revision, &data, &at); err != nil {
			break
		}
		examined++
		lastExamined = id
		if strings.HasPrefix(rowAudience, "library:") {
			if h.LibraryVisible == nil {
				continue
			}
			if !h.LibraryVisible(tx, p, strings.TrimPrefix(rowAudience, "library:")) {
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
	} else if examined == PageLimit+1 {
		out.NextAfter = strconv.FormatInt(lastExamined, 10)
	}
	return out, gate.Commit()
}
