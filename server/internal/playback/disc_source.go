package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/disc"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

func discPlanInputTx(ctx context.Context, tx *sql.Tx, input *DeliveryInput) error {
	var known bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_disc_facts WHERE asset_id=?)`, input.SourceID).Scan(&known); err != nil {
		return err
	}
	if !known {
		return nil
	}
	d, err := catalog.ReadDiscSelection(ctx, tx, input.SourceID)
	if err != nil {
		return err
	}
	if d == nil {
		return ErrIncompatible
	}
	if err = d.Ready(ctx, tx); err != nil {
		return err
	}
	input.Disc = true
	// An ISO contains several titles; dividing its size by one title's duration
	// is not that title's bitrate. Keep only actual selected-stream evidence.
	if input.Source != nil {
		rate := int64(0)
		known := true
		if input.Source.Video != nil {
			rate = input.Source.Video.BitRate
			known = rate > 0
		}
		for _, a := range input.Source.Audio {
			rate += a.BitRate
			known = known && a.BitRate > 0
		}
		input.Source.BitRate = 0
		if known {
			input.Source.BitRate = rate
		}
	}
	return nil
}
func pinDiscSessionTx(ctx context.Context, tx *sql.Tx, id, asset string) error {
	d, err := catalog.ReadDiscSelection(ctx, tx, asset)
	if err != nil || d == nil {
		return err
	}
	if err = d.Ready(ctx, tx); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO playback_disc_pins(session_id,object_id,title_key,source_revision,fingerprint,stream_revision,chapter_revision) VALUES(?,?,?,?,?,?,?)`, id, d.Object, d.MediaKey, d.Revision, d.Fingerprint, d.StreamRevision, d.ChapterRevision)
	return err
}

// The selection is checked alongside the occurrence's existing physical-source
// pin. A later scan may not silently change a running title or its track indices.
func checkDiscSessionTx(ctx context.Context, tx *sql.Tx, id string) error {
	var bad bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM playback_disc_pins p WHERE p.session_id=? AND NOT EXISTS(
 SELECT 1 FROM inventory_disc_facts d JOIN inventory_objects o ON o.id=d.object_id AND o.asset_id=d.asset_id
 JOIN library_sources s ON s.id=o.source_id JOIN playback_sessions ps ON ps.asset_id=d.asset_id AND ps.id=p.session_id
 JOIN catalog_assets a ON a.token=d.asset_id
 JOIN asset_stream_facts st ON st.asset_id=d.asset_id AND st.size=a.size AND st.modified_ns=a.modified_ns
 JOIN asset_chapter_facts ch ON ch.asset_id=d.asset_id AND ch.size=a.size AND ch.modified_ns=a.modified_ns
 WHERE d.object_id=p.object_id AND d.media_title_key=p.title_key AND d.fingerprint=p.fingerprint AND d.source_revision=p.source_revision
 AND d.media_stream_revision=p.stream_revision AND d.media_chapter_revision=p.chapter_revision AND st.revision=p.stream_revision AND ch.revision=p.chapter_revision
 AND d.encryption='none' AND o.revision=d.source_revision AND o.retired=0 AND o.state='available' AND s.enabled=1 AND s.kind='local'
 AND d.source_generation=s.generation AND d.root_incarnation=s.incarnation AND o.root_incarnation=s.incarnation))`, id).Scan(&bad)
	if err != nil {
		return err
	}
	if bad {
		return identity.ErrUnauthorized
	}
	return nil
}
func (h *HLS) openDiscInput(ctx context.Context, id string) (*assets.PlaybackInput, bool, error) {
	tx, done, err := dbwork.BeginRead(ctx, h.db)
	if err != nil {
		return nil, true, err
	}
	var asset string
	err = tx.QueryRowContext(ctx, `SELECT ps.asset_id FROM playback_sessions ps JOIN playback_disc_pins p ON p.session_id=ps.id WHERE ps.id=?`, id).Scan(&asset)
	if errors.Is(err, sql.ErrNoRows) {
		done()
		return nil, false, nil
	}
	if err != nil {
		done()
		return nil, true, err
	}
	if err = checkDiscSessionTx(ctx, tx, id); err != nil {
		done()
		return nil, true, err
	}
	d, err := catalog.ReadDiscSelection(ctx, tx, asset)
	if err != nil || d == nil {
		done()
		return nil, true, ErrIncompatible
	}
	members := []storage.DiscMember{}
	rows, err := tx.QueryContext(ctx, `SELECT path,object_identity,size,modified_ns,required,directory FROM inventory_disc_media_members WHERE object_id=? ORDER BY path LIMIT ?`, d.Object, disc.MaxConsumedMembers+1)
	if err != nil {
		done()
		return nil, true, err
	}
	for rows.Next() {
		var m storage.DiscMember
		if err = rows.Scan(&m.Path, &m.ObjectIdentity, &m.Size, &m.ModifiedNS, &m.Required, &m.Directory); err != nil {
			break
		}
		members = append(members, m)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	done()
	if err != nil {
		return nil, true, err
	}
	directory := d.Structure.Packaging == "folder"
	if h.SourceStorage == nil || len(members) > disc.MaxConsumedMembers || directory && len(members) == 0 {
		return nil, true, ErrIncompatible
	}
	req := storage.InventoryRequest{Root: d.Root, RootIdentity: d.RootIdentity, RelativePath: d.Relative, FollowSymlinks: d.FollowSymlinks}
	path := filepath.Join(d.Root, d.Relative)
	var pinnedFile *os.File
	validate := func(check context.Context) error {
		current, release, e := dbwork.BeginRead(check, h.db)
		if e != nil {
			return e
		}
		e = checkSessionOccurrenceTx(check, current, id)
		release()
		if e != nil {
			return e
		}
		if h.SourceStorage.Guard != nil {
			if e = h.SourceStorage.Guard(path); e != nil {
				return e
			}
		}
		if pinnedFile != nil && !directory {
			if e = h.SourceStorage.ValidatePlaybackDescriptor(check, pinnedFile, d.Size, d.ModifiedNS); e != nil {
				return e
			}
		}
		if directory {
			required, optional, dirs := []string{}, []string{}, []string{}
			// Include absent optional members too. Their later appearance changes the
			// accepted read set and must not pass by omission from the stored witness.
			consumed, e := disc.ConsumedMembers(d.Structure)
			if e != nil {
				return e
			}
			for _, m := range consumed {
				switch {
				case m.Directory:
					dirs = append(dirs, m.Path)
				case m.Required:
					required = append(required, m.Path)
				default:
					optional = append(optional, m.Path)
				}
			}
			actual, e := h.SourceStorage.DiscMemberManifest(check, d.Source, req, required, optional, dirs)
			if e != nil {
				return e
			}
			if !reflect.DeepEqual(actual, members) {
				return ErrStaleOffer
			}
		}
		return nil
	}
	readCtx := ctx
	if err = validate(ctx); err != nil {
		return nil, true, err
	}
	file, err := h.SourceStorage.OpenDiscPlaybackDescriptor(readCtx, req, d.ObjectIdentity, d.Size, d.ModifiedNS, directory)
	if err != nil {
		return nil, true, err
	}
	pinnedFile = file
	input := &assets.PlaybackInput{File: file, Inherit: true, Validate: validate}
	failed := func(e error) (*assets.PlaybackInput, bool, error) { input.Close(); return nil, true, e }
	tool := disc.Tool{Binary: filepath.Join(filepath.Dir(h.binary), "portico-disc")}
	actual, err := tool.ProbeDescriptor(ctx, file, directory)
	if err != nil {
		return failed(err)
	}
	if !reflect.DeepEqual(actual, d.Structure) {
		return failed(ErrStaleOffer)
	}
	title, ok := d.MainTitle()
	if !ok || title.Key(d.Structure.Kind) != d.MediaKey {
		return failed(ErrStaleOffer)
	}
	input.InputAt = func(check context.Context, ms int64) ([]string, string, int64, error) {
		if e := validate(check); e != nil {
			return nil, "", 0, e
		}
		var seek *disc.Seek
		if ms > 0 && d.Structure.Kind == "bluray" {
			value, e := tool.SeekDescriptor(check, file, directory, *title.Playlist, ms)
			if e != nil {
				return nil, "", 0, e
			}
			seek = &value
		}
		in, e := d.Structure.FFmpegInput(disc.DescriptorPath(directory), title, ms, seek)
		if e != nil {
			return nil, "", 0, e
		}
		if e = validate(check); e != nil {
			return nil, "", 0, e
		}
		return in.Options, in.URL, in.TimestampOffsetMs, nil
	}
	input.Options, input.Argument, _, err = input.InputAt(ctx, 0)
	if err != nil {
		return failed(err)
	}
	if directory {
		input.MountFD = 3
		input.MountPath = "/disc"
	}
	// FFmetadata is an inherited pipe protocol input. It is an unlinked private
	// file descriptor, never a sidecar path in the source library.
	metadata, err := title.ChapterMetadata()
	if err != nil {
		return failed(err)
	}
	dir := filepath.Join(h.root, id)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return failed(err)
	}
	chapter, err := os.CreateTemp(dir, "disc-chapters-")
	if err != nil {
		return failed(err)
	}
	os.Remove(chapter.Name())
	if _, err = chapter.Write(metadata); err != nil {
		chapter.Close()
		return failed(err)
	}
	if _, err = chapter.Seek(0, 0); err != nil {
		chapter.Close()
		return failed(err)
	}
	input.Extra = []*os.File{chapter}
	if err = validate(ctx); err != nil {
		return failed(err)
	}
	return input, true, nil
}

// Disc cut lists belong to the exact selected presentation. The ordinary
// asset/size/mtime cache cannot distinguish two titles in one unchanged ISO.
func (h *HLS) discTimeline(ctx context.Context, id string) (*copyTimeline, bool) {
	var raw string
	if err := dbwork.QueryRow(ctx, h.db, `SELECT timeline FROM playback_disc_pins WHERE session_id=?`, id).Scan(&raw); err != nil {
		return nil, false
	}
	var t copyTimeline
	if raw == "" || len(raw) > 1<<20 || json.Unmarshal([]byte(raw), &t) != nil || t.count() == 0 {
		return nil, true
	}
	return &t, true
}
func (h *HLS) storeDiscTimeline(ctx context.Context, id string, t *copyTimeline) {
	raw, err := json.Marshal(t)
	if err != nil || len(raw) > 1<<20 {
		return
	}
	_, _ = dbwork.ExecWrite(ctx, h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_disc_pins SET timeline=? WHERE session_id=?`, string(raw), id)
}
