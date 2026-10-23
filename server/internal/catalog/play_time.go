package catalog

import "database/sql"

// Split only an accepted, short report interval, at fifteen-minute
// boundaries. Pauses, seeks and missing reports never become watched time.
func recordPlayedInterval(tx *sql.Tx, id, from, to, watched int64) error {
	if to <= from || watched <= 0 {
		return nil
	}
	remaining := watched
	for at := from; at < to; {
		bucket := at - at%900000
		end := min(to, bucket+900000)
		part := (end - at) * watched / (to - from)
		if end == to {
			part = remaining
		}
		if _, e := tx.Exec(`INSERT INTO play_history_time(history_id,bucket_ms,watched_ms) VALUES(?,?,?) ON CONFLICT(history_id,bucket_ms) DO UPDATE SET watched_ms=watched_ms+excluded.watched_ms`, id, bucket, part); e != nil {
			return e
		}
		remaining -= part
		at = end
	}
	return nil
}

// RecordLivePlaybackTx records an accepted linear playback report. It measures
// playing wall time between reports, independently of the channel's seek clock.
func RecordLivePlaybackTx(tx *sql.Tx, session string, now int64, state string) error {
	var id, previous int64
	var previousState string
	e := tx.QueryRow(`SELECT id,updated_ms,last_state FROM play_history WHERE playback_id=?`, session).Scan(&id, &previous, &previousState)
	if e == sql.ErrNoRows {
		if state != "playing" {
			return nil
		}
		_, e = tx.Exec(`INSERT INTO play_history(playback_id,authority,account_id,profile_id,user_name,profile_name,item_id,library_id,kind,title,device_id,device_name,platform,started_ms,updated_ms,last_state,location)
 SELECT v.id,v.authority,v.account_id,v.profile_id,COALESCE(a.username,''),CASE WHEN p.name<>a.username THEN COALESCE(p.name,'') ELSE '' END,0,'',0,COALESCE(json_extract(cs.selection_json,'$.name'),''),v.device_id,COALESCE(d.name,''),COALESCE(d.platform,''),?,?,?,v.location
 FROM playback_v1_sessions v LEFT JOIN accounts a ON a.id=v.account_id LEFT JOIN direct_profiles p ON p.id=v.profile_id AND p.account_id=v.account_id
 LEFT JOIN identity_devices d ON d.id=v.device_id AND d.account_id=v.account_id
 LEFT JOIN playback_channel_sessions cs ON cs.session_id=v.id WHERE v.id=?`, now, now, state, session)
		return e
	}
	if e != nil {
		return e
	}
	watched := int64(0)
	if previousState == "playing" && now > previous && now-previous <= 60000 {
		watched = now - previous
	}
	if watched > 0 {
		if e = recordPlayedInterval(tx, id, previous, now, watched); e != nil {
			return e
		}
	}
	_, e = tx.Exec(`UPDATE play_history SET updated_ms=max(updated_ms,?),last_state=?,watched_ms=watched_ms+? WHERE id=?`, now, state, watched, id)
	return e
}
