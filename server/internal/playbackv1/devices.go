package playbackv1

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playback"
)

// ParseRevision reads an If-Match value: the quoted strong ETag ("12") or the
// bare revision (12), per spec §17.3. Weak validators never authorize a write.
func ParseRevision(v string) (int64, bool) {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "W/") {
		return 0, false
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	n, err := strconv.ParseInt(v, 10, 64)
	return n, err == nil && n >= 0
}

// ETag is the quoted strong ETag of a revision.
func ETag(revision int64) string { return `"` + strconv.FormatInt(revision, 10) + `"` }

// CapabilitiesDocument is a device's stored profile and its revision.
type CapabilitiesDocument struct {
	Revision     int64        `json:"revision"`
	Capabilities Capabilities `json:"capabilities"`
}

// Capabilities reads the device's current document.
func (s *Service) Capabilities(ctx context.Context, device string) (CapabilitiesDocument, error) {
	var out CapabilitiesDocument
	var raw string
	err := dbwork.QueryRow(ctx, s.DB, `SELECT revision,document FROM playback_device_capabilities WHERE device_id=?`, device).Scan(&out.Revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal([]byte(raw), &out.Capabilities)
}

// PutCapabilities replaces the device's whole document (spec §17.17). The
// precondition is checked first, and the document and the planner's translation
// of it are written in one gated transaction, so a refused write changes nothing
// (review P3). The planner reads the translation by device, so the profile
// survives token rotation and pruning (review P2).
func (s *Service) PutCapabilities(ctx context.Context, p identity.Principal, device, ifMatch string, c Capabilities) (int64, error) {
	if err := c.Validate(); err != nil {
		return 0, err
	}
	want := int64(-1)
	if ifMatch != "" {
		n, ok := ParseRevision(ifMatch)
		if !ok {
			return 0, &FieldError{Path: "If-Match"}
		}
		want = n
	}
	var platform, app, appVersion string
	if err := dbwork.QueryRow(ctx, s.DB, `SELECT platform,app,app_version FROM identity_devices WHERE id=?`, device).Scan(&platform, &app, &appVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, identity.ErrUnauthorized
		}
		return 0, err
	}
	raw, err := json.Marshal(c.ClientProfile(platform, app, appVersion))
	if err != nil {
		return 0, err
	}
	profile, err := playback.ParseClientProfile(raw)
	if err != nil {
		return 0, &FieldError{Path: ""}
	}
	planner, err := json.Marshal(profile)
	if err != nil {
		return 0, err
	}
	document, err := json.Marshal(c)
	if err != nil {
		return 0, err
	}
	var revision int64
	err = dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassPlaybackStart, func(ctx context.Context, tx *sql.Tx) error {
		var current int64
		var stored, storedPlanner string
		err := tx.QueryRowContext(ctx, `SELECT revision,document,planner_profile FROM playback_device_capabilities WHERE device_id=?`, device).Scan(&current, &stored, &storedPlanner)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if want >= 0 && want != current {
			var doc CapabilitiesDocument
			doc.Revision = current
			_ = json.Unmarshal([]byte(stored), &doc.Capabilities)
			return &RevisionError{Current: doc, Revision: current}
		}
		if err == nil && stored == string(document) && storedPlanner == string(planner) {
			revision = current
			return nil
		}
		revision = current + 1
		_, err = tx.ExecContext(ctx, `INSERT INTO playback_device_capabilities(device_id,account_id,profile_id,revision,document,planner_profile,updated_at_ms) VALUES(?,?,?,?,?,?,?) ON CONFLICT(device_id) DO UPDATE SET account_id=excluded.account_id,profile_id=excluded.profile_id,revision=excluded.revision,document=excluded.document,planner_profile=excluded.planner_profile,updated_at_ms=excluded.updated_at_ms`, device, p.AccountID, p.ProfileID, revision, string(document), string(planner), s.now().UnixMilli())
		return err
	})
	return revision, err
}

// RevisionError is a stale If-Match: the caller gets 412 with the current
// resource (spec §17.3).
type RevisionError struct {
	Current  any
	Revision int64
}

func (e *RevisionError) Error() string { return "revision mismatch" }
func (e *RevisionError) Unwrap() error { return ErrRevision }
