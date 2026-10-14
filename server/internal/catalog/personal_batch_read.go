package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"portico.local/server/internal/personalstate"
)

// Container actions read the same personal state as an individual action, but
// resolve their captured, bounded membership together rather than four reads
// for every member while holding the write gate.
func readPersonalBatch(ctx context.Context, tx *sql.Tx, profile string, ids []int64) (map[int64]PersonalState, error) {
	raw, _ := json.Marshal(ids)
	query := `SELECT e.id,COALESCE(pi.watchlisted,0),COALESCE(pi.favorite,0),COALESCE(pi.not_interested,0),pi.rating,COALESCE(pi.revision,0),COALESCE(` + personalstate.SQL("scope.profile", "e.id") + `,0),COALESCE(pi.last_played_at,''),COALESCE(p.position,0)/1000.0,
 EXISTS(SELECT 1 FROM continue_dismissals d WHERE d.profile_id=scope.profile AND d.item_id=e.id AND d.playback_id=COALESCE(p.playback_id,'')),
 (SELECT COALESCE(json_group_array(json_object('id',c.id,'field',c.field,'baseRevision',c.base_revision,'baseValue',json(c.base_value),'choices',json((SELECT COALESCE(json_group_array(json_object('operationId',v.operation_id,'deviceId',v.device_id,'authoredAt',v.authored_at,'value',json(v.value))),'[]') FROM personal_explicit_events v WHERE v.profile_id=c.profile_id AND v.item_id=c.item_id AND v.field=c.field AND v.base_revision=c.base_revision)))),'[]') FROM personal_conflicts c WHERE c.profile_id=scope.profile AND c.item_id=e.id AND c.resolved=0)
 FROM json_each(?) requested CROSS JOIN catalog_entities e ON e.id=CAST(requested.value AS INTEGER) CROSS JOIN (SELECT ? profile) scope
 LEFT JOIN personal_items pi ON pi.profile_id=scope.profile AND pi.item_id=e.id LEFT JOIN progress p ON p.profile_id=scope.profile AND p.item_id=e.id`
	rows, err := tx.QueryContext(ctx, query, string(raw), profile)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]PersonalState{}
	for rows.Next() {
		var id int64
		state := PersonalState{Status: "current", Conflicts: []PersonalConflict{}}
		var conflicts string
		if err := rows.Scan(&id, &state.Watchlisted, &state.Favorite, &state.NotInterested, &state.Rating, &state.Revision, &state.Watched, &state.LastPlayedAt, &state.ProgressSeconds, &state.ContinueDismissed, &conflicts); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(conflicts), &state.Conflicts); err != nil {
			return nil, err
		}
		if len(state.Conflicts) > 0 {
			state.Status = "needs-resolution"
		}
		out[id] = state
	}
	return out, rows.Err()
}

func snapshotPersonalBatch(ctx context.Context, tx *sql.Tx, profile string, ids []int64, states map[int64]PersonalState) error {
	values := make([][3]any, 0, len(ids))
	for _, id := range ids {
		state, ok := states[id]
		if !ok {
			return sql.ErrNoRows
		}
		raw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		values = append(values, [3]any{id, state.Revision, string(raw)})
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO personal_snapshots(profile_id,item_id,revision,state,created_at) SELECT ?,CAST(json_extract(value,'$[0]') AS INTEGER),CAST(json_extract(value,'$[1]') AS INTEGER),json_extract(value,'$[2]'),? FROM json_each(?)`, profile, time.Now().UTC().Format(time.RFC3339), string(raw))
	return err
}
