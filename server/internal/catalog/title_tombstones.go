package catalog

import (
	"context"
	"database/sql"
	"strings"

	"portico.local/server/internal/dbwork"
)

// RemovedTitle is personal, temporary evidence of a real removal. Outages,
// missing files and retired inventory do not create tombstones.
type RemovedTitle struct {
	ID                    string `json:"id"`
	Title                 string `json:"title"`
	Kind                  string `json:"kind"`
	PosterURL             string `json:"posterUrl,omitempty"`
	BackdropURL           string `json:"backdropUrl,omitempty"`
	RemovedAt             string `json:"removedAt"`
	CanRemoveFromContinue bool   `json:"canRemoveFromContinue"`
}

func removedTitlePredicate(v Viewer) (string, []any) {
	where := `t.public_id=? AND t.expires_at>strftime('%Y-%m-%dT%H:%M:%fZ','now') AND EXISTS(SELECT 1 FROM catalog_title_tombstone_viewers pv WHERE pv.public_id=t.public_id AND pv.profile_id=?) AND (t.library_id IN(SELECT value FROM json_each(?)) OR NOT EXISTS(SELECT 1 FROM libraries WHERE id=t.library_id))`
	// A confirmed removal builds its viewer/attribute facts in bounded turns.
	// The exact entity's deletion publishes those facts; incomplete/cancelled
	// builds and a recreated title never appear as an actual removed title.
	where += ` AND NOT EXISTS(SELECT 1 FROM catalog_entities live WHERE live.public_id=pid_blob(t.public_id))`
	args := []any{v.Profile, v.librariesJSON()}
	r := v.EffectiveRestrictions()
	if r.MaximumAge != nil {
		where += ` AND NOT EXISTS(SELECT 1 FROM catalog_title_tombstone_attributes a JOIN content_rating_ages age ON age.value_key=a.value_key WHERE a.public_id=t.public_id AND a.field_id=1 AND age.minimum_age>?)`
		args = append(args, *r.MaximumAge)
	}
	if r.BlockUnrated {
		where += ` AND EXISTS(SELECT 1 FROM catalog_title_tombstone_attributes a JOIN content_rating_ages age ON age.value_key=a.value_key WHERE a.public_id=t.public_id AND a.field_id=1 AND age.minimum_age>=0)`
	}
	for _, label := range r.BlockedLabels {
		where += ` AND NOT EXISTS(SELECT 1 FROM catalog_title_tombstone_attributes a WHERE a.public_id=t.public_id AND a.field_id=2 AND a.value_key=?)`
		args = append(args, strings.ToLower(label))
	}
	for _, label := range r.MemberDeniedLabels {
		where += ` AND NOT EXISTS(SELECT 1 FROM catalog_title_tombstone_attributes a WHERE a.public_id=t.public_id AND a.field_id IN(2,3) AND lower(a.source_value)=?)`
		args = append(args, strings.ToLower(label))
	}
	if r.BlockRecordings {
		where += ` AND t.recording=0`
	}
	return where, args
}

func (s *Service) RemovedTitle(v Viewer, id string) (RemovedTitle, error) {
	out := RemovedTitle{}
	where, args := removedTitlePredicate(v)
	err := s.read().QueryRow(`SELECT t.public_id,t.title,`+compactBrowseKindSQL("t.kind")+`,t.poster_url,t.backdrop_url,t.removed_at,(SELECT pv.continuing FROM catalog_title_tombstone_viewers pv WHERE pv.public_id=t.public_id AND pv.profile_id=?) FROM catalog_title_tombstones t WHERE `+where, append([]any{v.Profile, id}, args...)...).Scan(&out.ID, &out.Title, &out.Kind, &out.PosterURL, &out.BackdropURL, &out.RemovedAt, &out.CanRemoveFromContinue)
	return out, err
}

// DismissRemovedTitle removes only this profile's reference. The title and
// other profiles' activity are unchanged; a repeated delete is harmless.
func (s *Service) DismissRemovedTitle(v Viewer, id string, authorize func(*sql.Tx) error) error {
	ctx := s.Context()
	return dbwork.WithWriteTx(ctx, s.db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		if authorize != nil {
			if err := authorize(tx); err != nil {
				return err
			}
		}
		where, args := removedTitlePredicate(v)
		var found string
		err := tx.QueryRowContext(ctx, `SELECT t.public_id FROM catalog_title_tombstones t WHERE `+where, append([]any{id}, args...)...).Scan(&found)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE catalog_title_tombstone_viewers SET continuing=0 WHERE public_id=? AND profile_id=? AND continuing<>0`, id, v.Profile)
		return err
	})
}

// PruneRemovedTitles expires private references without retaining title names.
func (s *Service) PruneRemovedTitles(ctx context.Context) error {
	return dbwork.WithWriteTx(ctx, s.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		// A confirmed removal may still be copying its private facts. Terminal
		// mutations have entity_id=NULL, so this point entity/FK-prefix probe
		// protects only real live custody, never old command history.
		_, err := tx.ExecContext(ctx, `DELETE FROM catalog_title_tombstones AS t WHERE expires_at<=strftime('%Y-%m-%dT%H:%M:%fZ','now')
 AND NOT EXISTS(SELECT 1 FROM catalog_entities e CROSS JOIN catalog_entity_mutations m INDEXED BY catalog_entity_mutations_entity_fk ON m.entity_id=e.id WHERE e.public_id=pid_blob(t.public_id))`)
		return err
	})
}

// homeRemovedSource has exactly the same personal/current-policy fence as a
// direct tombstone read. Only unfinished progress participates in Continue.
func (s *Service) homeRemovedSource(v Viewer) (homeSource, error) {
	where, args := removedTitlePredicate(v)
	where = strings.Replace(where, "t.public_id=? AND ", "", 1)
	return homeSource{base: `SELECT t.public_id AS id,pv.activity_at AS ord FROM catalog_title_tombstones t JOIN catalog_title_tombstone_viewers pv ON pv.public_id=t.public_id WHERE pv.profile_id=? AND pv.continuing=1 AND t.kind IN(1,4) AND ` + where, args: append([]any{v.Profile}, args...), selfRestricted: true}, nil
}

func (s *Service) homeContinueEntries(v Viewer, ids []string) ([]ContentEntry, error) {
	where, args := removedTitlePredicate(v)
	where = strings.Replace(where, "t.public_id=?", "t.public_id IN(SELECT value FROM json_each(?))", 1)
	rows, err := s.read().Query(`SELECT t.public_id,t.title,`+compactBrowseKindSQL("t.kind")+`,t.poster_url,t.backdrop_url FROM catalog_title_tombstones t WHERE `+where, append([]any{idsJSON(ids)}, args...)...)
	if err != nil {
		return nil, err
	}
	removed := map[string]ContentEntry{}
	for rows.Next() {
		entry := ContentEntry{Removed: true}
		if err = rows.Scan(&entry.ID, &entry.Title, &entry.Kind, &entry.PosterURL, &entry.BackdropURL); err != nil {
			break
		}
		available := false
		entry.Available = &available
		entry.Navigation = &ContentNavigation{View: "item", EntityID: entry.ID}
		entry.HomeActions = &HomeActions{Details: true, RemoveFromContinue: true}
		removed[entry.ID] = entry
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	live := []string{}
	for _, id := range ids {
		if _, ok := removed[id]; !ok {
			live = append(live, id)
		}
	}
	entries, err := s.homeEntries(v.Profile, live)
	if err != nil {
		return nil, err
	}
	byID := removed
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	out := make([]ContentEntry, 0, len(ids))
	for _, id := range ids {
		entry, ok := byID[id]
		if !ok {
			return nil, ErrStaleContinuation
		}
		out = append(out, entry)
	}
	return out, nil
}
