package catalog

import (
	"encoding/json"
	"portico.local/server/internal/personalstate"
)

// A profile contributes at most one episode per started show. The index is
// updated with personal activity, so a request begins with started shows and
// never discovers them by scanning the whole library. The candidate checks
// apply restrictions before selecting the one episode: a hidden episode must
// not consume the show's slot.
// showActivityKey is a position in a profile's started shows, newest first:
// the walk continues strictly after it.
type showActivityKey struct {
	lastActivity string
	showID       int64
}

func homeShowContinuation(r HomeRequest, cutoffs, premieres string, window int, showID string, after *showActivityKey) (string, []any) {
	partialVisibility, partialArgs := ItemRestrictionSQL("i2.id", r.Restrictions)
	nextVisibility, nextArgs := ItemRestrictionSQL("i2.id", r.Restrictions)
	libraries, _ := json.Marshal(r.Libraries)
	showFilter := ""
	if showID != "" {
		showFilter = ` AND show_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`
	}
	if after != nil {
		showFilter += ` AND (last_activity<? OR last_activity=? AND show_id>?)`
	}
	// profile_show_activity already names the newest activity item. When it is
	// still partial, it is necessarily the show's newest partial episode. Try
	// that primary-key lookup before scanning the profile's older progress for
	// a fallback. A profile with thousands of shows otherwise re-walks every
	// progress row once per show.
	partial := func(latest bool) string {
		item := ""
		if latest {
			item = ` AND pa.item_id=CAST(w.last_item_id AS INTEGER)`
		}
		return `(SELECT pid(i2.public_id) FROM progress_activity pa JOIN catalog_episodes e2 ON e2.entity_id=pa.item_id JOIN catalog_entities i2 ON i2.id=pa.item_id
   CROSS JOIN progress p ON p.profile_id=pa.profile_id AND p.item_id=pa.item_id
   WHERE pa.profile_id=w.profile_id AND e2.show_id=w.show_id AND EXISTS(SELECT 1 FROM catalog_libraries cl WHERE cl.id=i2.library_id AND cl.library_id=w.library_id)
    ` + item + `
	    AND pa.state!='ended' AND p.position>0 AND ` + homeVisible("i2.id") + `
    AND p.position<(SELECT max(a.duration)*1000-3000 FROM catalog_asset_links ia JOIN catalog_assets a ON a.id=ia.asset_id WHERE ia.entity_id=i2.id AND a.available=1 AND ia.available=1)
    AND ` + personalstate.CompactSQL("pa.profile_id", "i2.id") + `=0
    AND NOT EXISTS(SELECT 1 FROM continue_dismissals d WHERE d.profile_id=p.profile_id AND d.item_id=p.item_id AND d.playback_id=p.playback_id)
    AND ` + partialVisibility + ` AND pa.updated_at>=(SELECT value FROM json_each(?) WHERE key=w.library_id)
   ORDER BY pa.updated_at DESC,pa.item_id DESC LIMIT 1)`
	}
	// The next-episode and premiere checks join the compact episode and
	// season tables by integer id: SQLite cannot flatten a LEFT JOIN onto a
	// view that is itself a join, and would materialise every episode of
	// every library once per statement (NEW-47).
	base := `SELECT chosen.id,chosen.ord FROM (SELECT COALESCE(
  ` + partial(true) + `,
  ` + partial(false) + `,
  (SELECT pid(i2.public_id) FROM catalog_entities pe JOIN catalog_episodes previous ON previous.entity_id=pe.id
   LEFT JOIN catalog_seasons ps ON ps.entity_id=previous.season_id AND (SELECT retired FROM catalog_entities WHERE id=previous.season_id)=0
   JOIN catalog_entities sh ON sh.id=w.show_id JOIN catalog_episodes e2 ON e2.show_id=sh.id
   JOIN catalog_entities i2 ON i2.id=e2.entity_id LEFT JOIN catalog_seasons s2 ON s2.entity_id=e2.season_id AND (SELECT retired FROM catalog_entities WHERE id=e2.season_id)=0
	   WHERE pe.id=CAST(w.last_item_id AS INTEGER) AND EXISTS(SELECT 1 FROM catalog_libraries cl WHERE cl.id=i2.library_id AND cl.library_id=w.library_id) AND ` + homeVisible("i2.id") + `
    AND (e2.numbering='absolute' AND previous.numbering='absolute' AND e2.number>previous.number
      OR e2.numbering!='absolute' AND previous.numbering!='absolute' AND
       (COALESCE(s2.number,0)>COALESCE(ps.number,0) OR COALESCE(s2.number,0)=COALESCE(ps.number,0) AND e2.number>previous.number))
    AND (COALESCE(s2.number,0)>0 OR COALESCE(ps.number,0)=0)
    AND ` + personalstate.CompactSQL("w.profile_id", "i2.id") + `=0
    AND COALESCE((SELECT p.position FROM progress p WHERE p.profile_id=w.profile_id AND p.item_id=i2.id),0)=0
    AND NOT EXISTS(SELECT 1 FROM continue_dismissals d WHERE d.profile_id=w.profile_id AND d.item_id=i2.id AND d.playback_id=COALESCE((SELECT p.playback_id FROM progress p WHERE p.profile_id=d.profile_id AND p.item_id=d.item_id),''))
    AND ` + nextVisibility + `
   ORDER BY COALESCE(s2.number,0),e2.number,i2.id LIMIT 1)
	 ) AS id,w.last_activity AS ord,w.library_id AS library_id,w.last_item_id AS last_item_id FROM
	 (SELECT * FROM profile_show_activity WHERE profile_id=? AND library_id IN(SELECT value FROM json_each(?))` + showFilter + ` ORDER BY last_activity DESC,show_id LIMIT ?) w
	 WHERE w.profile_id=? AND w.library_id IN(SELECT value FROM json_each(?))
	 AND NOT EXISTS(SELECT 1 FROM continue_show_dismissals d WHERE d.profile_id=w.profile_id AND d.show_id=w.show_id
	  AND d.last_activity>=w.last_activity AND d.playback_id=COALESCE((SELECT p.playback_id FROM progress p WHERE p.profile_id=d.profile_id AND p.item_id=d.item_id),''))) chosen
	 WHERE chosen.id IS NOT NULL AND (chosen.ord>=(SELECT value FROM json_each(?) WHERE key=chosen.library_id)
	 OR (EXISTS(SELECT 1 FROM json_each(?) WHERE key=chosen.library_id AND value=1)
	 AND EXISTS(SELECT 1 FROM catalog_entities ce JOIN catalog_episodes e ON e.entity_id=ce.id JOIN catalog_seasons se ON se.entity_id=e.season_id AND (SELECT retired FROM catalog_entities WHERE id=e.season_id)=0
	  JOIN catalog_entities pe ON pe.id=CAST(chosen.last_item_id AS INTEGER) JOIN catalog_episodes prior ON prior.entity_id=pe.id
	  LEFT JOIN catalog_seasons sp ON sp.entity_id=prior.season_id AND (SELECT retired FROM catalog_entities WHERE id=prior.season_id)=0
	  WHERE ce.public_id=pid_blob(chosen.id) AND e.number=1 AND se.number>COALESCE(sp.number,0))))`
	args := append([]any{}, partialArgs...)
	args = append(args, cutoffs)
	args = append(args, partialArgs...)
	args = append(args, cutoffs)
	args = append(args, nextArgs...)
	args = append(args, r.Profile, string(libraries))
	if showID != "" {
		args = append(args, showID)
	}
	if after != nil {
		args = append(args, after.lastActivity, after.lastActivity, after.showID)
	}
	args = append(args, window, r.Profile, string(libraries), cutoffs, premieres)
	return base, args
}
