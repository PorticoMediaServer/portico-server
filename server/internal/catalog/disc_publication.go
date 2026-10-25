package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/disc"
	"portico.local/server/internal/identity"
)

// CommitInventoryDiscProbe publishes structure observed by the bounded disc
// helper. It is not a timed-media probe, a playback-availability writer, or a
// grant to open a source. The scan caller must use the admitted local path and
// recheck its exact observations before/after I/O. This transaction additionally
// fences the real job, policy, source generation/incarnation and object revision.
// A remote caller stays closed until its combined root/read-session producer is
// composed; an ETag or transport ChangeToken cannot replace catalog revision.
// The normalized schema is steward-owned; this method creates no runtime DDL.
func (s *Service) CommitInventoryDiscProbe(ctx context.Context, job, object, revision string, policyRevision int64, probe disc.Probe) error {
	raw, err := json.Marshal(probe)
	if err != nil {
		return err
	}
	validated, err := disc.DecodeProbe(raw)
	if err != nil && !errors.Is(err, disc.ErrEncrypted) {
		return err
	}
	digest := sha256.Sum256(append([]byte("disc_structure_v1\x00"), raw...))
	fingerprint := hex.EncodeToString(digest[:])
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	source, err := InventoryFenceTx(ctx, tx, job)
	if err != nil {
		return err
	}
	if source.Kind != "local" {
		return disc.ErrRemoteUnqualified
	}
	policy, err := ScanPolicyTx(ctx, tx, source.LibraryID)
	if err != nil {
		return err
	}
	if policy.Revision != policyRevision || !policy.Allows("probe") {
		return context.Canceled
	}
	var asset string
	err = tx.QueryRowContext(ctx, `SELECT asset_id FROM inventory_objects
 WHERE id=? AND source_id=? AND root_incarnation=? AND revision=? AND state='available' AND retired=0`, object, source.ID, source.Incarnation, revision).Scan(&asset)
	if err != nil {
		return err
	}
	var oldAsset, oldIncarnation, oldFingerprint string
	err = tx.QueryRowContext(ctx, `SELECT asset_id,root_incarnation,fingerprint FROM inventory_disc_facts WHERE object_id=?`, object).Scan(&oldAsset, &oldIncarnation, &oldFingerprint)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if oldAsset != "" && (oldAsset != asset || oldIncarnation != source.Incarnation) {
		// A title token never silently starts referring to another physical
		// source incarnation. Its children retire with the old parent.
		if _, err = tx.ExecContext(ctx, `DELETE FROM inventory_disc_facts WHERE object_id=?`, object); err != nil {
			return err
		}
		oldFingerprint = ""
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO inventory_disc_facts(object_id,asset_id,source_revision,source_generation,root_incarnation,kind,packaging,encryption,fingerprint)
 VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(object_id) DO UPDATE SET
 media_title_key=CASE WHEN inventory_disc_facts.source_revision=excluded.source_revision AND inventory_disc_facts.source_generation=excluded.source_generation AND inventory_disc_facts.root_incarnation=excluded.root_incarnation AND inventory_disc_facts.fingerprint=excluded.fingerprint THEN inventory_disc_facts.media_title_key ELSE '' END,
 media_stream_revision=CASE WHEN inventory_disc_facts.source_revision=excluded.source_revision AND inventory_disc_facts.source_generation=excluded.source_generation AND inventory_disc_facts.root_incarnation=excluded.root_incarnation AND inventory_disc_facts.fingerprint=excluded.fingerprint THEN inventory_disc_facts.media_stream_revision ELSE 0 END,
 media_chapter_revision=CASE WHEN inventory_disc_facts.source_revision=excluded.source_revision AND inventory_disc_facts.source_generation=excluded.source_generation AND inventory_disc_facts.root_incarnation=excluded.root_incarnation AND inventory_disc_facts.fingerprint=excluded.fingerprint THEN inventory_disc_facts.media_chapter_revision ELSE 0 END,
 owner_title_key=CASE WHEN inventory_disc_facts.source_revision=excluded.source_revision AND inventory_disc_facts.source_generation=excluded.source_generation AND inventory_disc_facts.root_incarnation=excluded.root_incarnation AND inventory_disc_facts.fingerprint=excluded.fingerprint THEN inventory_disc_facts.owner_title_key ELSE '' END,
 choice_revision=CASE WHEN inventory_disc_facts.source_revision=excluded.source_revision AND inventory_disc_facts.fingerprint=excluded.fingerprint THEN inventory_disc_facts.choice_revision ELSE inventory_disc_facts.choice_revision+1 END,
 asset_id=excluded.asset_id,source_revision=excluded.source_revision,source_generation=excluded.source_generation,root_incarnation=excluded.root_incarnation,kind=excluded.kind,packaging=excluded.packaging,encryption=excluded.encryption,fingerprint=excluded.fingerprint`,
		object, asset, revision, source.Generation, source.Incarnation, validated.Kind, validated.Packaging, validated.Encryption, fingerprint)
	if err != nil {
		return err
	}
	if oldFingerprint == fingerprint {
		// A new observation can reaffirm unchanged structure without rewriting
		// thousands of stream/chapter rows or changing stable title identities.
		return gated.Commit()
	}
	if err = writeDiscTitlesTx(ctx, tx, object, validated); err != nil {
		return err
	}
	return gated.Commit()
}

// persistDiscMediaSelectionTx runs only in the accepting timed-media writer,
// after its stream/chapter facts. Structure alone never says which title those
// facts describe, and a later metadata write must not silently inherit the link.
func persistDiscMediaSelectionTx(ctx context.Context, tx *sql.Tx, source LibrarySource, object, revision, asset string, size, modified int64, facts assets.Facts) error {
	if facts.DiscSelection == nil {
		_, err := tx.ExecContext(ctx, `UPDATE inventory_disc_facts SET media_title_key='',media_stream_revision=0,media_chapter_revision=0 WHERE object_id=?`, object)
		return err
	}
	if source.Kind != "local" {
		return disc.ErrRemoteUnqualified
	}
	var streams, chapters int64
	err := tx.QueryRowContext(ctx, `SELECT st.revision,ch.revision FROM asset_stream_facts st JOIN asset_chapter_facts ch ON ch.asset_id=st.asset_id
 WHERE st.asset_id=? AND st.size=? AND st.modified_ns=? AND ch.size=st.size AND ch.modified_ns=st.modified_ns AND ch.status='known'`, asset, size, modified).Scan(&streams, &chapters)
	if err != nil {
		return err
	}
	selection := facts.DiscSelection
	result, err := tx.ExecContext(ctx, `UPDATE inventory_disc_facts SET media_title_key=?,media_stream_revision=?,media_chapter_revision=?
 WHERE object_id=? AND asset_id=? AND source_revision=? AND source_generation=? AND root_incarnation=? AND fingerprint=? AND encryption='none'
 AND (owner_title_key='' OR owner_title_key=?)
 AND EXISTS(SELECT 1 FROM inventory_disc_titles t WHERE t.object_id=inventory_disc_facts.object_id AND t.title_key=?
 AND t.duration_ms=? AND (SELECT COUNT(*) FROM inventory_disc_chapters c WHERE c.object_id=t.object_id AND c.title_key=t.title_key)=?)`,
		selection.TitleKey, streams, chapters, object, asset, revision, source.Generation, source.Incarnation, selection.StructureFingerprint, selection.TitleKey, selection.TitleKey, int64(math.Round(facts.Duration*1000)), len(facts.Chapters))
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("disc media selection changed during analysis")
	}
	if len(selection.Members) > disc.MaxConsumedMembers {
		return disc.ErrProtocol
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM inventory_disc_media_members WHERE object_id=?`, object); err != nil {
		return err
	}
	for _, member := range selection.Members {
		if member.Path == "" || member.ObjectIdentity == "" {
			return disc.ErrProtocol
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_disc_media_members(object_id,path,object_identity,size,modified_ns,required,directory) VALUES(?,?,?,?,?,?,?)`, object, member.Path, member.ObjectIdentity, member.Size, member.ModifiedNS, member.Required, member.Directory); err != nil {
			return err
		}
	}

	policy, err := ScanPolicyTx(ctx, tx, source.LibraryID)
	if err != nil {
		return err
	}
	return persistDiscTitleMediaTx(ctx, tx, object, revision, selection.StructureFingerprint, facts, policy)
}

func writeDiscTitlesTx(ctx context.Context, tx *sql.Tx, object string, probe disc.Probe) error {
	rows, err := tx.QueryContext(ctx, `SELECT title_key,selection_token,title_number FROM inventory_disc_titles WHERE object_id=?`, object)
	if err != nil {
		return err
	}
	existing := map[string]string{}
	for rows.Next() {
		var key, token string
		var number int
		if err = rows.Scan(&key, &token, &number); err != nil {
			rows.Close()
			return err
		}
		if token == "" || number < 1 || number > disc.MaxTitles {
			rows.Close()
			return disc.ErrProtocol
		}
		existing[key] = token
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Avoid transient unique(title_number) collisions when real durations
	// change the sorted order. Only the final 1..N ordinal is ever committed.
	if _, err = tx.ExecContext(ctx, `UPDATE inventory_disc_titles SET title_number=title_number+? WHERE object_id=?`, disc.MaxTitles, object); err != nil {
		return err
	}
	titleStatement, err := tx.PrepareContext(ctx, `INSERT INTO inventory_disc_titles(object_id,title_key,selection_token,title_number,disc_order,duration_ms,angle_count,playlist,dvd_title,vts,pgc)
 VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(object_id,title_key) DO UPDATE SET title_number=excluded.title_number,disc_order=excluded.disc_order,duration_ms=excluded.duration_ms,angle_count=excluded.angle_count,playlist=excluded.playlist,dvd_title=excluded.dvd_title,vts=excluded.vts,pgc=excluded.pgc`)
	if err != nil {
		return err
	}
	defer titleStatement.Close()
	clipStatement, err := tx.PrepareContext(ctx, `INSERT INTO inventory_disc_clips(object_id,title_key,sequence,file,in_time_90k,out_time_90k) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer clipStatement.Close()
	streamStatement, err := tx.PrepareContext(ctx, `INSERT INTO inventory_disc_streams(object_id,title_key,stream_order,clip_sequence,pid,kind,coding,language,dv_enhancement,dv_enhancement_known,dv_base_pid,video_role) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer streamStatement.Close()
	chapterStatement, err := tx.PrepareContext(ctx, `INSERT INTO inventory_disc_chapters(object_id,title_key,chapter_order,start_ms) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer chapterStatement.Close()
	for number, title := range probe.OrderedTitles() {
		key := title.Key(probe.Kind)
		token := existing[key]
		if token == "" {
			token = identity.Token()
		}
		delete(existing, key)
		if _, err = titleStatement.ExecContext(ctx, object, key, token, number+1, title.DiscOrder, title.DurationMs, title.AngleCount, title.Playlist, title.Title, title.VTS, title.PGC); err != nil {
			return err
		}
		// Static table names only; no client/provider value enters SQL syntax.
		for _, table := range []string{"inventory_disc_clips", "inventory_disc_streams", "inventory_disc_chapters"} {
			if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE object_id=? AND title_key=?`, object, key); err != nil {
				return err
			}
		}
		for _, clip := range title.Clips {
			if _, err = clipStatement.ExecContext(ctx, object, key, clip.Sequence, clip.File, clip.InTime90k, clip.OutTime90k); err != nil {
				return err
			}
		}
		for order, stream := range title.Streams {
			if _, err = streamStatement.ExecContext(ctx, object, key, order, stream.ClipSequence, stream.PID, stream.Kind, stream.Coding, stream.Language, stream.DolbyVisionEnhancementLayer, stream.DolbyVisionEnhancementKnown, stream.DolbyVisionBasePID, stream.VideoRole); err != nil {
				return err
			}
		}
		for order, chapter := range title.Chapters {
			if _, err = chapterStatement.ExecContext(ctx, object, key, order, chapter.StartMs); err != nil {
				return err
			}
		}
	}
	for key := range existing {
		if _, err = tx.ExecContext(ctx, `DELETE FROM inventory_disc_titles WHERE object_id=? AND title_key=?`, object, key); err != nil {
			return err
		}
	}
	return nil
}
