package social

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Activity privacy anonymizes a participant, rather than hiding room occupancy
// or removing someone from the readiness barrier.
func watchTogetherVisible(alias string) string {
	return `COALESCE((SELECT json_extract(d.body,'$."privacy.includeInWatchTogether"') FROM console_documents d WHERE d.scope='profile:'||json_array(` + alias + `.authority,` + alias + `.account_id,` + alias + `.profile_id)),1)<>0`
}
func (s *Store) projectActivity(ctx context.Context, tx *sql.Tx, group string, payload json.RawMessage) (json.RawMessage, error) {
	var body map[string]any
	if e := json.Unmarshal(payload, &body); e != nil {
		return nil, e
	}
	member, _ := body["memberId"].(string)
	if member == "" {
		return payload, nil
	}
	var public bool
	var name string
	if e := tx.QueryRowContext(ctx, `SELECT `+watchTogetherVisible("m")+`,display_name FROM social_group_members m WHERE group_id=? AND id=?`, group, member).Scan(&public, &name); e != nil {
		return nil, e
	}
	body["private"] = !public
	body["displayName"] = name
	if !public {
		body["displayName"] = "Viewer"
		body["positionUs"] = "0"
		body["reportedAt"] = nil
	}
	return json.Marshal(body)
}
