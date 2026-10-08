package playbackv1

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"portico.local/server/internal/dbwork"
)

// AdminSession is one active playback on the server (spec §14, Now Playing):
// who, on what device, what, how it's delivered and from where.
type AdminSession struct {
	ID            string     `json:"id"`
	Protocol      string     `json:"protocol"` // "v1"
	User          AdminName  `json:"user"`
	Profile       AdminName  `json:"profile"`
	Device        AdminName  `json:"device"`
	Item          *AdminItem `json:"item,omitempty"`
	Kind          string     `json:"kind"` // "vod" | "audio" | "channel"
	State         string     `json:"state"`
	PositionMs    int64      `json:"positionMs"`
	Decision      Decisions  `json:"decision"`
	BitrateKbps   int        `json:"bitrateKbps,omitempty"`
	BandwidthKbps int64      `json:"bandwidthKbps,omitempty"`
	Location      string     `json:"location"`
	StartedAt     string     `json:"startedAt"`
}
type AdminName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type AdminItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type AdminSessionPage struct {
	Items []AdminSession `json:"items"`
	Page  Page           `json:"page"`
}

// AdminSessionsMax is the most sessions one page lists.
const AdminSessionsMax = 200

// adminActiveV1 says what "active" means: a v1 session not ended whose lease
// holds. It is served by a partial index of only its active rows
// (migration 0066).
const adminActiveV1 = `v.ended_ms=0 AND v.lease_expires_ms>?`

// The branch yields the same columns: protocol, id, created, user id and
// name, profile id and name, device id and name, item id and title, kind,
// state, position, presentation, bandwidth, location.
const adminV1Branch = `SELECT 'v1' AS protocol,v.id AS id,v.created_ms AS created,v.account_id,COALESCE(a.username,''),v.profile_id,COALESCE(p.name,''),v.device_id,COALESCE(d.name,''),
 CASE WHEN v.kind IN ('live','channel') THEN v.channel_id ELSE COALESCE(pid(i.public_id),'') END,CASE WHEN v.kind IN ('live','channel') THEN COALESCE(json_extract(cs.selection_json,'$.name'),'') ELSE COALESCE(i.title,'') END,
 CASE WHEN v.kind IN ('live','channel') THEN 'channel' ELSE v.kind END,v.state,CASE WHEN v.kind IN ('live','channel') THEN COALESCE(cs.confirmed_position_us/1000,0) ELSE v.position_ms END,v.presentation,v.bandwidth_kbps,v.location
 FROM playback_v1_sessions v INDEXED BY playback_v1_sessions_active
 LEFT JOIN playback_channel_sessions cs ON cs.session_id=v.id
 LEFT JOIN accounts a ON a.id=v.account_id AND v.authority='local'
 LEFT JOIN direct_profiles p ON p.id=v.profile_id AND v.authority='local'
 LEFT JOIN identity_devices d ON d.id=v.device_id
 LEFT JOIN catalog_entities i ON i.id=v.item_id
 WHERE ` + adminActiveV1 + ` AND (v.created_ms<? OR v.created_ms=? AND v.id>?)
 ORDER BY v.created_ms DESC,v.id LIMIT ?`

// AdminSessions lists active playbacks, newest first, a page at a time.
func (s *Service) AdminSessions(ctx context.Context, limit int, cursor string) (AdminSessionPage, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > AdminSessionsMax {
		return AdminSessionPage{}, &FieldError{Path: "limit"}
	}
	afterCreated, afterID := int64(1<<62), ""
	if cursor != "" {
		created, id, ok := strings.Cut(strings.TrimPrefix(cursor, "t:"), ":")
		n, err := strconv.ParseInt(created, 10, 64)
		if !strings.HasPrefix(cursor, "t:") || !ok || err != nil || id == "" {
			return AdminSessionPage{}, &FieldError{Path: "cursor"}
		}
		afterCreated, afterID = n, id
	}
	tx, done, err := dbwork.BeginRead(ctx, s.DB)
	if err != nil {
		return AdminSessionPage{}, err
	}
	defer done()
	// Sessions and catalogue facts are synchronous; no derived-domain wait.
	now := s.now().UnixMilli()
	out := AdminSessionPage{Items: []AdminSession{}, Page: Page{Limit: limit}}
	var v1 int64
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM playback_v1_sessions v INDEXED BY playback_v1_sessions_active WHERE `+adminActiveV1, now).Scan(&v1); err != nil {
		return out, err
	}
	out.Page.Total = v1
	rows, err := tx.QueryContext(ctx, `SELECT * FROM (`+adminV1Branch+`) ORDER BY created DESC,id LIMIT ?`,
		now, afterCreated, afterCreated, afterID, limit+1,
		limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	lastMs := int64(0)
	for rows.Next() {
		var a AdminSession
		var item, title, presentation string
		var created int64
		if err = rows.Scan(&a.Protocol, &a.ID, &created, &a.User.ID, &a.User.Name, &a.Profile.ID, &a.Profile.Name, &a.Device.ID, &a.Device.Name, &item, &title, &a.Kind, &a.State, &a.PositionMs, &presentation, &a.BandwidthKbps, &a.Location); err != nil {
			return out, err
		}
		if len(out.Items) == limit {
			out.Page.NextCursor = "t:" + strconv.FormatInt(lastMs, 10) + ":" + out.Items[len(out.Items)-1].ID
			break
		}
		lastMs = created
		for _, n := range []*AdminName{&a.User, &a.Profile, &a.Device} {
			if n.Name == "" {
				n.Name = n.ID
			}
		}
		if item != "" {
			a.Item = &AdminItem{ID: item, Title: title}
		}
		// Only the decision and bitrate: the presentation's URL is a capability.
		var p struct {
			Decision    Decisions `json:"decision"`
			BitrateKbps int       `json:"bitrateKbps"`
		}
		if presentation != "" && json.Unmarshal([]byte(presentation), &p) == nil {
			a.Decision, a.BitrateKbps = p.Decision, p.BitrateKbps
		}
		a.StartedAt = time.UnixMilli(created).UTC().Format(time.RFC3339Nano)
		out.Items = append(out.Items, a)
	}
	return out, rows.Err()
}

// AdminTerminateMessageMax bounds what an administrator can show the viewer.
const AdminTerminateMessageMax = 500

// Terminate ends a v1 session for an administrator (spec §14). The session's
// device gets session.updated with reason "terminated" and the message. Ending
// one that's already over is not an error; an unknown id is not found.
func (s *Service) Terminate(ctx context.Context, id, message string) error {
	message = strings.TrimSpace(message)
	if utf8.RuneCountInString(message) > AdminTerminateMessageMax || strings.ContainsAny(message, "\x00") {
		return &FieldError{Path: "message"}
	}
	if _, err := loadRow(ctx, s.DB, id); err == nil {
		return s.End(ctx, id, "terminated", message)
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	return ErrNotFound
}
