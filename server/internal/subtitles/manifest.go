package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediaartifact"
	"strconv"
	"strings"
)

// PinManifestTextTx snapshots a bounded set of already imported text tracks.
// It performs no extraction. Personal tracks remain confined to this viewer.
func PinManifestTextTx(ctx context.Context, tx *sql.Tx, session, item, source, viewer string) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO playback_manifest_subtitles(session_id,resource_id,resource_revision)
 SELECT ?,r.id,v.revision FROM subtitle_resources r JOIN subtitle_revisions v ON v.resource_id=r.id AND v.revision=r.current_revision
 JOIN catalog_assets a ON a.token=r.source_id JOIN subtitle_render_revisions rr ON rr.resource_id=r.id AND rr.revision=v.revision
 WHERE r.item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND r.source_id=? AND r.deleted=0 AND (r.scope='shared' OR r.owner=?) AND rr.renderer='external_text'
 AND v.source_size=a.size AND v.source_modified_ns=a.modified_ns ORDER BY r.id LIMIT 8`, session, item, source, viewer)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO playback_subtitle_pins SELECT session_id,resource_id,resource_revision FROM playback_manifest_subtitles WHERE session_id=?`, session)
	return err
}

func (s *Service) OpenManifestDocument(ctx context.Context, p identity.Principal, item, session, id string, revision int64) (*mediaartifact.Reader, int64, error) {
	gate, p, e := s.tx(ctx, p, item)
	if e != nil {
		return nil, 0, e
	}
	defer gate.Rollback()
	tx := gate.Tx()
	scope, e := sessionTx(ctx, tx, p, item, session)
	if e != nil {
		return nil, 0, e
	}
	if !scope.available {
		return nil, 0, ErrUnavailable
	}
	r, e := scanResource(tx.QueryRowContext(ctx, `SELECT `+resourceColumns+` FROM playback_manifest_subtitles pin JOIN subtitle_resources r ON r.id=pin.resource_id JOIN subtitle_revisions v ON v.resource_id=r.id AND v.revision=pin.resource_revision WHERE pin.session_id=? AND pin.resource_id=? AND pin.resource_revision=? AND r.item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, session, id, revision, item))
	if e != nil || !visible(r, p) || r.Renderer != "external_text" || r.SourceID != scope.source || r.sourceSize != scope.size || r.sourceModified != scope.modified {
		return nil, 0, identity.ErrUnauthorized
	}
	offset, e := strconv.ParseInt(r.OffsetUS, 10, 64)
	if e != nil {
		return nil, 0, ErrInput
	}
	// A grant generation fixes the selected track's personal timing.
	var selectedOffset int64
	err := tx.QueryRowContext(ctx, `SELECT offset_us FROM playback_subtitle_state WHERE session_id=? AND generation=? AND resource_id=? AND resource_revision=? AND renderer='external_text'`, session, scope.generation, id, revision).Scan(&selectedOffset)
	if err == nil {
		offset = selectedOffset
	} else if err != sql.ErrNoRows {
		return nil, 0, err
	}
	reader, e := s.objects.Open(ctx, mediaartifact.Object{Digest: r.digest, Size: r.size})
	if e != nil {
		return nil, 0, e
	}
	if e = gate.Commit(); e != nil {
		reader.Close()
		return nil, 0, e
	}
	return reader, offset, nil
}

// ManifestWebVTT uses the title clock. Cues crossing a segment edge are repeated
// with their original timestamps, as required by HLS WebVTT. Empty windows are
// valid and retain the timestamp map. Markup comes only from structured flags.
func ManifestWebVTT(reader io.Reader, offset int64, segment int) ([]byte, error) {
	if segment < -1 || segment > 1440 {
		return nil, ErrInput
	}
	raw, e := io.ReadAll(io.LimitReader(reader, MaxInputBytes+1))
	if e != nil || len(raw) > MaxInputBytes {
		return nil, ErrCapacity
	}
	var d Document
	if json.Unmarshal(raw, &d) != nil || len(d.Cues) > MaxCues {
		return nil, ErrInput
	}
	var b strings.Builder
	b.WriteString("WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:0\n\n")
	stamp := func(us int64) string {
		ms := us / 1000
		return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
	}
	for i, c := range d.Cues {
		start, e := strconv.ParseInt(c.StartUS, 10, 64)
		if e != nil {
			return nil, ErrInput
		}
		end, e := strconv.ParseInt(c.EndUS, 10, 64)
		if e != nil {
			return nil, ErrInput
		}
		start += offset
		end += offset
		if end <= 0 || segment >= 0 && (end <= int64(segment)*60000000 || start >= int64(segment+1)*60000000) {
			continue
		}
		start = max(int64(0), start)
		if end <= start {
			return nil, ErrInput
		}
		text := html.EscapeString(c.Text)
		if c.Italic {
			text = "<i>" + text + "</i>"
		}
		placement := ""
		if c.Top {
			placement = " line:10%"
		}
		fmt.Fprintf(&b, "%d\n%s --> %s%s\n%s\n\n", i+1, stamp(start), stamp(end), placement, text)
		if b.Len() > MaxInputBytes {
			return nil, ErrCapacity
		}
	}
	return []byte(b.String()), nil
}

// The ordinal suffix disambiguates duplicate human labels in AVFoundation/CAF.
func ManifestSubtitleName(title, language string, ordinal int) string {
	title = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, title)
	if title == "" {
		title = language + " · Subtitles"
	}
	return fmt.Sprintf("%s · %d", title, ordinal+1)
}

func (s *Service) CheckManifestDocument(ctx context.Context, p identity.Principal, item, session, id string, revision int64) error {
	reader, _, err := s.OpenManifestDocument(ctx, p, item, session, id, revision)
	if err != nil {
		return err
	}
	return reader.Close()
}
