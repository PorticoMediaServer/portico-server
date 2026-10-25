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
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
)

// DiscTitleRef is a checked title selection within an existing physical
// version. Revision binds the offered title's measured facts as well as source.
type DiscTitleRef struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}

func discTitleRevision(source, token, facts string) string {
	sum := sha256.Sum256([]byte("disc_title_offer_v1\x00" + source + "\x00" + token + "\x00" + facts))
	return hex.EncodeToString(sum[:])
}
func persistDiscTitleMediaTx(ctx context.Context, tx *sql.Tx, object, revision, fingerprint string, f assets.Facts, policy ScanPolicy) error {
	if f.DiscSelection == nil || len(f.Streams) == 0 || len(f.Streams) > 128 || len(f.DiscSelection.Members) > disc.MaxConsumedMembers {
		return disc.ErrProtocol
	}
	key := f.DiscSelection.TitleKey
	raw, _ := json.Marshal([]any{revision, fingerprint, f.Container, f.VideoCodec, f.AudioCodec, f.Width, f.Height, f.Duration, f.OriginUS, f.TimingKnown, f.DiscSelection.Members})
	seen := map[int]bool{}
	for _, st := range f.Streams {
		if st.Index < 0 || seen[st.Index] || (st.Type != "video" && st.Type != "audio" && st.Type != "subtitle") {
			return disc.ErrProtocol
		}
		seen[st.Index] = true
		encoded, _ := json.Marshal(st)
		breadth, _ := json.Marshal(st.Detail.StreamBreadth)
		raw = append(raw, encoded...)
		raw = append(raw, st.EncodeDetail()...)
		raw = append(raw, breadth...)
	}
	if len(raw) > disc.MaxOutputBytes {
		return disc.ErrProtocol
	}
	sum := sha256.Sum256(raw)
	mediaRevision := hex.EncodeToString(sum[:])
	var duration int64
	if err := tx.QueryRowContext(ctx, `SELECT duration_ms FROM inventory_disc_titles WHERE object_id=? AND title_key=?`, object, key).Scan(&duration); err != nil {
		return err
	}
	if int64(math.Round(f.Duration*1000)) != duration {
		return disc.ErrProtocol
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO inventory_disc_title_media(object_id,title_key,source_revision,fingerprint,revision,container,video_codec,audio_codec,width,height,duration_ms,origin_us,timing_known) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(object_id,title_key) DO UPDATE SET source_revision=excluded.source_revision,fingerprint=excluded.fingerprint,revision=excluded.revision,container=excluded.container,video_codec=excluded.video_codec,audio_codec=excluded.audio_codec,width=excluded.width,height=excluded.height,duration_ms=excluded.duration_ms,origin_us=excluded.origin_us,timing_known=excluded.timing_known`, object, key, revision, fingerprint, mediaRevision, f.Container, f.VideoCodec, f.AudioCodec, f.Width, f.Height, duration, f.OriginUS, f.TimingKnown)
	if err != nil {
		return err
	}
	for _, table := range []string{"inventory_disc_title_media_streams", "inventory_disc_title_media_members"} {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE object_id=? AND title_key=?`, object, key); err != nil {
			return err
		}
	}
	for _, st := range f.Streams {
		breadth, _ := json.Marshal(st.Detail.StreamBreadth)
		_, err = tx.ExecContext(ctx, `INSERT INTO inventory_disc_title_media_streams(object_id,title_key,stream_index,type,codec,language,title,channels,channel_layout,is_default,is_forced,color_transfer,color_primaries,detail_json,breadth_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, object, key, st.Index, st.Type, st.Codec, st.Language, st.Title, st.Channels, st.ChannelLayout, st.Default, st.Forced, st.ColorTransfer, st.ColorPrimaries, st.EncodeDetail(), string(breadth))
		if err != nil {
			return err
		}
	}
	for _, m := range f.DiscSelection.Members {
		if m.Path == "" || m.ObjectIdentity == "" {
			return disc.ErrProtocol
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO inventory_disc_title_media_members(object_id,title_key,path,object_identity,size,modified_ns,required,directory) VALUES(?,?,?,?,?,?,?,?)`, object, key, m.Path, m.ObjectIdentity, m.Size, m.ModifiedNS, m.Required, m.Directory)
		if err != nil {
			return err
		}
	}
	if !policy.Allows("subtitles") {
		return nil
	}
	tracks := []subtitles.Track{}
	for _, stream := range f.Streams {
		if stream.Type == "subtitle" {
			format := subtitles.CodecFormat(stream.Codec)
			reason := ""
			if format == "" {
				format = stream.Codec
				reason = "unsupported_format"
			}
			tracks = append(tracks, subtitles.Track{Origin: "embedded", StreamIndex: stream.Index, Format: format, Language: stream.Language, Title: stream.Title, Default: stream.Default, Forced: stream.Forced, HearingImpaired: stream.HearingImpaired, Reason: reason})
		}
	}
	if err = subtitles.PersistDiscResourcesTx(ctx, tx, object, key, revision, fingerprint, mediaRevision, policy.Revision, f.OriginUS, f.TimingKnown, tracks); err != nil {
		return err
	}

	return nil
}
func ReadDiscTitleMedia(ctx context.Context, q discQueryer, d *DiscSelection, key string) (assets.Facts, string, error) {
	var f assets.Facts
	var revision string
	var ms int64
	err := q.QueryRowContext(ctx, `SELECT revision,container,video_codec,audio_codec,width,height,duration_ms,origin_us,timing_known FROM inventory_disc_title_media WHERE object_id=? AND title_key=? AND source_revision=? AND fingerprint=?`, d.Object, key, d.Revision, d.Fingerprint).Scan(&revision, &f.Container, &f.VideoCodec, &f.AudioCodec, &f.Width, &f.Height, &ms, &f.OriginUS, &f.TimingKnown)
	if err != nil {
		return f, "", err
	}
	f.Duration = float64(ms) / 1000
	rows, err := q.QueryContext(ctx, `SELECT stream_index,type,codec,language,title,channels,channel_layout,is_default,is_forced,color_transfer,color_primaries,detail_json,breadth_json FROM inventory_disc_title_media_streams WHERE object_id=? AND title_key=? ORDER BY stream_index LIMIT 129`, d.Object, key)
	if err != nil {
		return f, "", err
	}
	for rows.Next() {
		var st assets.Stream
		var detail, breadth string
		if err = rows.Scan(&st.Index, &st.Type, &st.Codec, &st.Language, &st.Title, &st.Channels, &st.ChannelLayout, &st.Default, &st.Forced, &st.ColorTransfer, &st.ColorPrimaries, &detail, &breadth); err != nil {
			break
		}
		st.DecodeDetail(detail)
		if len(breadth) > 8192 || json.Unmarshal([]byte(breadth), &st.Detail.StreamBreadth) != nil {
			err = disc.ErrProtocol
			break
		}
		f.Streams = append(f.Streams, st)
		if len(f.Streams) > 128 {
			err = disc.ErrProtocol
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || len(f.Streams) == 0 {
		return f, "", disc.ErrProtocol
	}
	f.DiscSelection = &assets.DiscMediaSelection{TitleKey: key, StructureFingerprint: d.Fingerprint, Members: []storage.DiscMember{}}
	rows, err = q.QueryContext(ctx, `SELECT path,object_identity,size,modified_ns,required,directory FROM inventory_disc_title_media_members WHERE object_id=? AND title_key=? ORDER BY path LIMIT ?`, d.Object, key, disc.MaxConsumedMembers+1)
	if err != nil {
		return f, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var m storage.DiscMember
		if err = rows.Scan(&m.Path, &m.ObjectIdentity, &m.Size, &m.ModifiedNS, &m.Required, &m.Directory); err != nil {
			return f, "", err
		}
		f.DiscSelection.Members = append(f.DiscSelection.Members, m)
		if len(f.DiscSelection.Members) > disc.MaxConsumedMembers {
			return f, "", disc.ErrProtocol
		}
	}
	return f, revision, rows.Err()
}

// ResolveDiscTitleTx binds a public title handle to current cached timed facts.
// The caller has already authorized this item and selected its physical asset.
func ResolveDiscTitleTx(ctx context.Context, tx *sql.Tx, d *DiscSelection, ref DiscTitleRef) (disc.Title, assets.Facts, string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT selection_token,title_key FROM inventory_disc_titles WHERE object_id=? LIMIT ?`, d.Object, disc.MaxTitles+1)
	if err != nil {
		return disc.Title{}, assets.Facts{}, "", err
	}
	var key, token string
	for rows.Next() {
		var t, k string
		if err = rows.Scan(&t, &k); err != nil {
			break
		}
		if discPublicID(t) == ref.ID {
			key, token = k, t
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return disc.Title{}, assets.Facts{}, "", err
	}
	if key == "" {
		return disc.Title{}, assets.Facts{}, "", sql.ErrNoRows
	}
	f, revision, err := ReadDiscTitleMedia(ctx, tx, d, key)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrDiscPreparing
	}
	if err != nil {
		return disc.Title{}, f, "", err
	}
	if ref.Revision != discTitleRevision(d.PublicRevision(), token, revision) {
		return disc.Title{}, f, "", ErrDiscSelectionChanged
	}
	for _, t := range d.Structure.Titles {
		if t.Key(d.Structure.Kind) == key {
			return t, f, revision, nil
		}
	}
	return disc.Title{}, f, "", disc.ErrProtocol
}
func (s *Service) CommitDiscAlternate(ctx context.Context, job, object, revision string, policyRevision int64, f assets.Facts) error {
	if f.DiscSelection == nil {
		return disc.ErrProtocol
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer gated.Rollback()
	tx := gated.Tx()
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
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_disc_title_requests r JOIN inventory_disc_facts d ON d.object_id=r.object_id JOIN inventory_objects o ON o.id=d.object_id JOIN library_sources s ON s.id=o.source_id WHERE r.job_id=? AND r.object_id=? AND r.title_key=? AND r.source_revision=? AND r.fingerprint=? AND d.source_revision=r.source_revision AND d.fingerprint=r.fingerprint AND d.encryption='none' AND o.source_id=? AND o.revision=d.source_revision AND o.retired=0 AND o.state='available' AND d.source_generation=s.generation AND d.root_incarnation=s.incarnation AND o.root_incarnation=s.incarnation)`, job, object, f.DiscSelection.TitleKey, revision, f.DiscSelection.StructureFingerprint, source.ID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrDiscSelectionChanged
	}
	if err = persistDiscTitleMediaTx(ctx, tx, object, revision, f.DiscSelection.StructureFingerprint, f, policy); err != nil {
		return err
	}
	return gated.Commit()
}
