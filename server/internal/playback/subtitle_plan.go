package playback

import (
	"database/sql"
	"errors"
	"fmt"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

type AudioManifestIdentity struct {
	GroupID      string `json:"groupId"`
	Name         string `json:"name"`
	PlaylistFile string `json:"playlistFile"`
}

type SubtitleTrack struct {
	ID                      string                `json:"id"`
	Origin                  string                `json:"origin"`
	Label                   string                `json:"label"`
	ManifestLanguage        string                `json:"manifestLanguage"`
	Default                 bool                  `json:"default"`
	Forced                  bool                  `json:"forced"`
	SourceRevision          int64                 `json:"sourceRevision"`
	ManifestIdentity        AudioManifestIdentity `json:"manifestIdentity"`
	ordinal, index          int
	locator, format, digest string
	size, modified          int64
}
type SubtitlePlan struct {
	PolicyVersion       int             `json:"policyVersion"`
	Revision            string          `json:"revision"`
	SessionID           string          `json:"sessionId"`
	Generation          int             `json:"generation"`
	SourceID            string          `json:"sourceId"`
	FactsRevision       int64           `json:"factsRevision"`
	AssociationRevision int64           `json:"associationRevision"`
	InitialIntent       string          `json:"initialIntent"`
	OffAvailable        bool            `json:"offAvailable"`
	Tracks              []SubtitleTrack `json:"tracks"`
	originUS            int64
	status, reason      string
	clock               sql.NullInt64
	duration            float64
}

func persistSubtitlePlan(tx *sql.Tx, id, asset, mode string) error {
	if mode != "hls" {
		return nil
	}
	var revision, factsRevision, origin int64
	var known bool
	var status string
	e := tx.QueryRow(`SELECT f.revision,s.revision,f.origin_us,f.timing_known,f.status FROM asset_subtitle_facts f JOIN catalog_assets a ON a.token=f.asset_id JOIN asset_stream_facts s ON s.asset_id=a.token WHERE a.token=? AND a.video_codec!='' AND a.container!='strm' AND f.size=a.size AND f.modified_ns=a.modified_ns AND s.size=a.size AND s.modified_ns=a.modified_ns`, asset).Scan(&revision, &factsRevision, &origin, &known, &status)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	reason := ""
	if !known {
		reason = "unsupported_timing"
	} else if status != "known" {
		reason = "inventory_unavailable"
	}
	rows, e := tx.Query(`SELECT t.id,t.origin,t.locator,t.stream_index,COALESCE(t.sidecar_id,''),t.format,t.language,t.title,t.is_default,t.is_forced,COALESCE(s.size,0),COALESCE(s.modified_ns,0),COALESCE(s.digest,'') FROM asset_subtitles t LEFT JOIN subtitle_sidecars s ON s.id=t.sidecar_id WHERE t.asset_id=? AND t.reason='' ORDER BY t.origin,t.stream_index,t.id LIMIT 9`, asset)
	if e != nil {
		return e
	}
	type record struct {
		t       SubtitleTrack
		sidecar string
	}
	var records []record
	for rows.Next() {
		var r record
		var lang, title string
		if e = rows.Scan(&r.t.ID, &r.t.Origin, &r.t.locator, &r.t.index, &r.sidecar, &r.t.format, &lang, &title, &r.t.Default, &r.t.Forced, &r.t.size, &r.t.modified, &r.t.digest); e != nil {
			rows.Close()
			return e
		}
		r.t.ordinal = len(records)
		r.t.Label = publicAudioText(title, 96)
		if r.t.Label == "" {
			r.t.Label = "Subtitles"
		}
		r.t.Label += fmt.Sprintf(" — Track %d", len(records)+1)
		r.t.ManifestLanguage = manifestAudioLanguage(lang)
		records = append(records, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if len(records) == 0 {
		reason = "no_supported_tracks"
	}
	if len(records) > subtitles.MaxPreparedTracks {
		reason = "capacity"
		records = nil
	}
	status = "preparing"
	if reason != "" {
		status = "unavailable"
		records = nil
	}
	_, e = tx.Exec(`INSERT INTO playback_subtitle_plans VALUES(?,?,1,?,?,?,?,?,NULL,240000)`, id, identity.Token(), factsRevision, revision, origin, status, reason)
	if e != nil {
		return e
	}
	for _, r := range records {
		var side any
		if r.sidecar != "" {
			side = r.sidecar
		}
		_, e = tx.Exec(`INSERT INTO playback_subtitle_tracks VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, r.t.ordinal, r.t.ID, r.t.Origin, r.t.locator, r.t.index, side, r.t.size, r.t.modified, r.t.digest, r.t.format, r.t.Label, r.t.ManifestLanguage, r.t.Default, r.t.Forced, "portico_subtitles", fmt.Sprintf("subtitle_%d", r.t.ordinal), fmt.Sprintf("subtitle-%d.m3u8", r.t.ordinal))
		if e != nil {
			return e
		}
	}
	return nil
}
func loadSubtitlePlan(q audioQuery, id string) (*SubtitlePlan, error) {
	p := &SubtitlePlan{SessionID: id, InitialIntent: "off", OffAvailable: true, Tracks: []SubtitleTrack{}}
	e := q.QueryRow(`SELECT p.revision,p.policy_version,p.facts_revision,p.association_revision,p.origin_us,p.status,p.reason,p.clock,s.generation,s.asset_id,s.duration FROM playback_subtitle_plans p JOIN playback_sessions s ON s.id=p.session_id WHERE p.session_id=?`, id).Scan(&p.Revision, &p.PolicyVersion, &p.FactsRevision, &p.AssociationRevision, &p.originUS, &p.status, &p.reason, &p.clock, &p.Generation, &p.SourceID, &p.duration)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if p.PolicyVersion != 1 {
		return nil, subtitles.ErrTiming
	}
	rows, e := q.Query(`SELECT ordinal,id,origin,locator,stream_index,source_size,source_modified_ns,source_digest,format,label,language,is_default,is_forced,group_id,manifest_name,playlist_file FROM playback_subtitle_tracks WHERE session_id=? ORDER BY ordinal LIMIT 9`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var t SubtitleTrack
		if e = rows.Scan(&t.ordinal, &t.ID, &t.Origin, &t.locator, &t.index, &t.size, &t.modified, &t.digest, &t.format, &t.Label, &t.ManifestLanguage, &t.Default, &t.Forced, &t.ManifestIdentity.GroupID, &t.ManifestIdentity.Name, &t.ManifestIdentity.PlaylistFile); e != nil {
			return nil, e
		}
		if t.ordinal != len(p.Tracks) || t.ordinal >= 8 || t.ManifestIdentity.GroupID != "portico_subtitles" || t.ManifestIdentity.Name != fmt.Sprintf("subtitle_%d", t.ordinal) || t.ManifestIdentity.PlaylistFile != fmt.Sprintf("subtitle-%d.m3u8", t.ordinal) {
			return nil, subtitles.ErrText
		}
		t.SourceRevision = p.AssociationRevision
		p.Tracks = append(p.Tracks, t)
	}
	return p, rows.Err()
}
func subtitlePinValid(q audioQuery, p *SubtitlePlan) error {
	var valid bool
	e := q.QueryRow(`SELECT EXISTS(SELECT 1 FROM playback_source_pins pin JOIN catalog_assets a ON a.token=pin.asset_id JOIN asset_subtitle_facts f ON f.asset_id=a.token JOIN asset_stream_facts st ON st.asset_id=a.token WHERE pin.session_id=? AND a.available=1 AND a.size=pin.size AND a.modified_ns=pin.modified_ns AND f.size=pin.size AND f.modified_ns=pin.modified_ns AND f.revision=? AND st.revision=? AND NOT EXISTS(SELECT 1 FROM playback_subtitle_tracks t LEFT JOIN subtitle_sidecars sc ON sc.id=t.sidecar_id WHERE t.session_id=pin.session_id AND t.origin='sidecar' AND (sc.id IS NULL OR sc.size!=t.source_size OR sc.modified_ns!=t.source_modified_ns OR sc.digest!=t.source_digest)))`, p.SessionID, p.AssociationRevision, p.FactsRevision).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return ErrStaleOffer
	}
	return nil
}
