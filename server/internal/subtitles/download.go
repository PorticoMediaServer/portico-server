package subtitles

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediaartifact"
)

// DownloadTrack is one subtitle track a download carries with it, so the title
// has its subtitles with no server in reach.
//
// Formats lists what can be fetched for the track. Every track has "vtt", the
// text rendering every device can show. A track kept in a styled or picture
// form the device can draw for itself is listed with that form as well.
type DownloadTrack struct {
	ID       string   `json:"id"`
	Revision int64    `json:"revision"`
	Language string   `json:"language"`
	Title    string   `json:"title"`
	Format   string   `json:"format"`
	Default  bool     `json:"default"`
	Forced   bool     `json:"forced"`
	Formats  []string `json:"formats"`
}

// DownloadTracks lists the text tracks of one file of a title that this viewer
// may take offline: the shared ones and the viewer's own, for the file as it
// is now. A track read from an earlier version of the file is left out; its
// timings are not the file's.
func (s *Service) DownloadTracks(ctx context.Context, p identity.Principal, item, source string) ([]DownloadTrack, error) {
	out := []DownloadTrack{}
	if !validID(item) || !validID(source) {
		return out, ErrInput
	}
	snapshot, p, e := s.readTx(ctx, p, item)
	if e != nil {
		return out, e
	}
	defer snapshot.Rollback()
	catalog, e := s.ListTx(ctx, snapshot.Tx(), p, item, source)
	if e != nil {
		return out, e
	}
	for _, r := range catalog.Resources {
		if !r.Enabled || r.Retired || r.Renderer != "external_text" || !visible(r, p) {
			continue
		}
		out = append(out, DownloadTrack{ID: r.ID, Revision: r.Revision, Language: r.Language, Title: r.Title, Format: r.Format, Default: r.Default, Forced: r.Forced, Formats: []string{"vtt"}})
	}
	return out, snapshot.Commit()
}

// DownloadTrackVTT renders one track as a whole WebVTT document on the title's
// own clock, for a device to keep beside the downloaded file. The caller has
// already established that the viewer may transfer the title; this checks the
// track itself the way a listing does.
func (s *Service) DownloadTrackVTT(ctx context.Context, p identity.Principal, item, id string, revision int64) ([]byte, error) {
	if !validID(item) || !validID(id) || revision < 1 {
		return nil, ErrInput
	}
	snapshot, p, e := s.readTx(ctx, p, item)
	if e != nil {
		return nil, e
	}
	defer snapshot.Rollback()
	r, e := scanResource(snapshot.Tx().QueryRowContext(ctx, `SELECT `+resourceColumns+` FROM subtitle_resources r JOIN subtitle_revisions v ON v.resource_id=r.id AND v.revision=r.current_revision WHERE r.id=? AND v.revision=? AND r.deleted=0 AND r.item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, id, revision, item))
	if errors.Is(e, sql.ErrNoRows) {
		return nil, identity.ErrNotVisible
	}
	if e != nil {
		return nil, e
	}
	if r.Renderer != "external_text" || !visible(r, p) {
		return nil, identity.ErrNotVisible
	}
	var offset int64
	if e = snapshot.Tx().QueryRowContext(ctx, `SELECT offset_us FROM subtitle_revisions WHERE resource_id=? AND revision=?`, id, revision).Scan(&offset); e != nil {
		return nil, e
	}
	// Opened under the snapshot, so collection cannot retire the bytes between
	// the check and the read.
	reader, e := s.objects.Open(ctx, mediaartifact.Object{Digest: r.digest, Size: r.size})
	if e != nil {
		return nil, e
	}
	defer reader.Close()
	if e = snapshot.Commit(); e != nil {
		return nil, e
	}
	document, e := ManifestWebVTT(reader, offset, -1)
	if e != nil {
		return nil, e
	}
	// The timestamp map is for a subtitle cut into streaming segments. A file
	// kept whole beside its video has no use for it.
	return bytes.Replace(document, []byte("X-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:0\n"), nil, 1), nil
}
