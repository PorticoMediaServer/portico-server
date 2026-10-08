package social

import (
	"context"
	"database/sql"
	"encoding/json"
)

// The profile-scoped document is the same source used by the typed preferences
// registry. Reading it in the group snapshot makes privacy changes immediate.
func watchTogetherVisible(alias string) string {
	return `COALESCE((SELECT json_extract(d.body,'$."privacy.includeInWatchTogether"') FROM console_documents d WHERE d.scope='profile:'||json_array(` + alias + `.authority,` + alias + `.account_id,` + alias + `.profile_id)),1)<>0`
}
func watchTogetherMember(ctx context.Context, tx *sql.Tx, group, member string) (bool, error) {
	var visible bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM social_group_members m WHERE m.group_id=? AND m.id=? AND `+watchTogetherVisible("m")+`)`, group, member).Scan(&visible)
	return visible, err
}
func privateEventMember(kind string, payload any) string {
	if kind != EventReadiness {
		return ""
	}
	raw, _ := json.Marshal(payload)
	var value struct {
		Member string `json:"memberId"`
	}
	_ = json.Unmarshal(raw, &value)
	return value.Member
}
func visibleEvent(ctx context.Context, tx *sql.Tx, group, kind string, payload any) (bool, error) {
	if kind == EventHost {
		var host string
		if err := tx.QueryRowContext(ctx, `SELECT host_member_id FROM social_groups WHERE id=?`, group).Scan(&host); err != nil {
			return false, err
		}
		return watchTogetherMember(ctx, tx, group, host)
	}
	if member := privateEventMember(kind, payload); member != "" {
		return watchTogetherMember(ctx, tx, group, member)
	}
	return true, nil
}
