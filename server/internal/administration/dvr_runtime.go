package administration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/livechannels/dvr"
)

func runtimeDefaults(ctx context.Context, tx *sql.Tx) (DVRDocument, error) {
	v := DefaultDVRDefaults()
	var rev int64
	var days, count int
	err := tx.QueryRowContext(ctx, `SELECT revision,before_seconds,after_seconds,retention_days,episode_limit FROM dvr_defaults WHERE singleton=1`).Scan(&rev, &v.PrePaddingSeconds, &v.PostPaddingSeconds, &days, &count)
	if count > 0 {
		v.Keep = KeepPolicy{Mode: "keep-count", KeepCount: count}
	} else if days > 0 {
		v.Keep = KeepPolicy{Mode: "keep-days", KeepDays: days}
	}
	return DVRDocument{rev, digestOf(v), v, []FolderToken{}, dvrEnumerations()}, err
}
func (s *Service) runtimeDVRSettings(ctx context.Context, auth Authorize) (DVRDocument, error) {
	var out DVRDocument
	err := s.snapshot(ctx, auth, func(tx *sql.Tx) error { var e error; out, e = runtimeDefaults(ctx, tx); return e })
	return out, err
}
func (s *Service) saveRuntimeDVRSettings(ctx context.Context, auth Authorize, c Change[DVRDefaults]) (DVRDocument, error) {
	var out DVRDocument
	if !validOperationID(c.OperationID) {
		return out, ErrInput
	}
	if err := validateDVRDefaults(&c.Settings); err != nil {
		return out, err
	}
	err := s.transaction(ctx, auth, func(tx *sql.Tx) error {
		saved, replay, err := receipt[DVRDocument](ctx, tx, dvrScope, c.OperationID, digestOf(c))
		if err != nil {
			return err
		}
		if replay {
			out = saved
			return nil
		}
		days, count := 0, 0
		if c.Settings.Keep.Mode == "keep-days" {
			days = c.Settings.Keep.KeepDays
		}
		if c.Settings.Keep.Mode == "keep-count" {
			count = c.Settings.Keep.KeepCount
		}
		result, err := tx.ExecContext(ctx, `UPDATE dvr_defaults SET revision=revision+1,before_seconds=?,after_seconds=?,retention_days=?,episode_limit=? WHERE singleton=1 AND revision=?`, c.Settings.PrePaddingSeconds, c.Settings.PostPaddingSeconds, days, count, c.ExpectedRevision)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return ErrConflict
		}
		out, err = runtimeDefaults(ctx, tx)
		if err != nil {
			return err
		}
		return saveReceipt(ctx, tx, dvrScope, c.OperationID, digestOf(c), out, s.milliseconds())
	})
	return out, err
}
func groupFromRule(r dvr.Rule, o livechannels.Owner) RecordingGroup {
	before, after := r.Config.Options.BeforeSeconds, r.Config.Options.AfterSeconds
	keep := KeepPolicy{Mode: "keep-all"}
	if r.Config.Options.EpisodeLimit > 0 {
		keep = KeepPolicy{Mode: "keep-count", KeepCount: r.Config.Options.EpisodeLimit}
	} else if r.Config.Options.RetentionDays > 0 {
		keep = KeepPolicy{Mode: "keep-days", KeepDays: r.Config.Options.RetentionDays}
	}
	return RecordingGroup{Owner: o, Status: r.ReconcileState, ID: r.ID, Kind: "series", Name: r.Config.Name, Match: r.Config.SeriesID, SourceID: r.Config.SourceID, Enabled: r.Config.Enabled, Revision: r.Revision, Options: GroupOptions{PrePaddingSeconds: &before, PostPaddingSeconds: &after, Keep: &keep, NewEpisodesOnly: r.Config.Episodes == "new", Priority: r.Config.Options.Priority}, UpdatedAt: r.UpdatedAt}
}
func (s *Service) runtimeRecordingGroups(ctx context.Context, auth Authorize, token string, limit int) (RecordingGroupPage, error) {
	out := RecordingGroupPage{Items: []RecordingGroup{}}
	size, err := pageSize(limit)
	if err != nil {
		return out, err
	}
	c, err := decodeCursor("recording-groups", token)
	if err != nil {
		return out, err
	}
	err = s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT id,authority,account_id,profile_id,revision,config_json,updated_ms,COALESCE((SELECT phase FROM dvr_rule_work w WHERE w.rule_id=dvr_rules.id),'idle') FROM dvr_rules WHERE deleted=0 AND (?=0 OR updated_ms<? OR (updated_ms=? AND id<?)) UNION ALL SELECT id,'','','',revision,json_object('name',name,'sourceId',source_id,'seriesId',match_text),created_ms,'legacy' FROM admin_dvr_groups WHERE (?=0 OR created_ms<? OR (created_ms=? AND id<?)) ORDER BY 7 DESC,1 DESC LIMIT ?`, c.Order, c.Order, c.Order, c.ID, c.Order, c.Order, c.Order, c.ID, size+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		orders := []int64{}
		for rows.Next() {
			var r dvr.Rule
			var o livechannels.Owner
			var raw, kind string
			var at int64
			if e = rows.Scan(&r.ID, &o.Authority, &o.AccountID, &o.ProfileID, &r.Revision, &raw, &at, &kind); e != nil {
				return e
			}
			if e = json.Unmarshal([]byte(raw), &r.Config); e != nil {
				return e
			}
			r.UpdatedAt = timeFromMilliseconds(at)
			r.ReconcileState = kind
			if !r.Config.Enabled {
				r.ReconcileState = "disabled"
			}
			if kind == "legacy" {
				r.Config.Enabled = false
				r.ReconcileState = "needs-owner-and-guide-selection"
			}
			g := groupFromRule(r, o)
			g.CreatedAt = g.UpdatedAt
			out.Items = append(out.Items, g)
			orders = append(orders, at)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > size {
			out.Items = out.Items[:size]
			out.NextCursor = encodeCursor("recording-groups", orders[size-1], out.Items[size-1].ID)
		}
		return nil
	})
	return out, err
}

// Authority is the already rechecked server owner; Store still independently
// checks the selected recording owner's durable membership and recording grant.
func groupAuthority(auth Authorize) livechannels.Authority {
	return func(ctx context.Context, tx *sql.Tx, _ bool) (string, func(string, string) bool, error) {
		if err := auth(ctx, tx); err != nil {
			return "", nil, err
		}
		return "server-owner", func(string, string) bool { return true }, nil
	}
}
func mapDVRError(err error) error {
	switch {
	case errors.Is(err, dvr.ErrDenied), errors.Is(err, livechannels.ErrDenied):
		return ErrDenied
	case errors.Is(err, dvr.ErrConflict):
		return ErrConflict
	case errors.Is(err, dvr.ErrInvalid), errors.Is(err, dvr.ErrUnsupportedPredicate):
		return invalid("anchor", "match", "options")
	case errors.Is(err, dvr.ErrCaptureUnavailable):
		return ErrUnavailable
	}
	return err
}
func (s *Service) saveRuntimeRecordingGroup(ctx context.Context, auth Authorize, id string, c RecordingGroupChange) (RecordingGroup, error) {
	var out RecordingGroup
	if s.DVR == nil {
		return out, ErrUnavailable
	}
	if !validOperationID(c.OperationID) || !c.Owner.Valid() {
		return out, invalid("owner", "operationId")
	}
	if err := validateGroup(&c); err != nil {
		return out, err
	}
	if c.Kind != "series" || c.Options.FolderTemplate != "" {
		return out, invalid("kind", "options.folderTemplate")
	}
	if c.Options.Keep != nil && c.Options.Keep.KeepUntilWatched {
		return out, invalid("options.keepPolicy")
	}
	err := s.transaction(ctx, auth, func(tx *sql.Tx) error {
		saved, replay, err := receipt[RecordingGroup](ctx, tx, "recording-group", c.OperationID, digestOf([]any{id, c}))
		if err != nil {
			return err
		}
		if replay {
			out = saved
			return nil
		}
		opts, err := dvr.DefaultOptionsTx(ctx, tx)
		if err != nil {
			return err
		}
		opts.Priority = c.Options.Priority
		if c.Options.PrePaddingSeconds != nil {
			opts.BeforeSeconds = *c.Options.PrePaddingSeconds
		}
		if c.Options.PostPaddingSeconds != nil {
			opts.AfterSeconds = *c.Options.PostPaddingSeconds
		}
		if c.Options.Keep != nil {
			opts.RetentionDays, opts.EpisodeLimit = 0, 0
			if c.Options.Keep.Mode == "keep-days" {
				opts.RetentionDays = c.Options.Keep.KeepDays
			}
			if c.Options.Keep.Mode == "keep-count" {
				opts.EpisodeLimit = c.Options.Keep.KeepCount
			}
		}
		target := id
		if target == "" {
			target = digestOf([]string{"admin-rule", c.OperationID, c.Owner.Key()})
		}
		episodes := "all"
		if c.Options.NewEpisodesOnly {
			episodes = "new"
		}
		in := dvr.RuleInput{ID: target, Mutation: dvr.Mutation{ExpectedRevision: c.ExpectedRevision, RequestID: digestOf([]string{"admin-rule-request", c.OperationID})[:48]}, Anchor: c.Anchor, Config: dvr.RuleConfig{Name: c.Name, SourceID: c.SourceID, SeriesID: c.Match, Enabled: c.Enabled, Episodes: episodes, AllowedChannels: []string{}, BlockedChannels: []string{}, BlockedKeywords: []string{}, Keywords: []string{}, Options: opts}}
		r, err := s.DVR.SaveRule(dbwork.WithWrite(ctx, tx, dbwork.ClassInteractive), groupAuthority(auth), c.Owner, in)
		if err != nil {
			return mapDVRError(err)
		}
		out = groupFromRule(r, c.Owner)
		return saveReceipt(ctx, tx, "recording-group", c.OperationID, digestOf([]any{id, c}), out, s.milliseconds())
	})
	return out, err
}
func (s *Service) deleteRuntimeRecordingGroup(ctx context.Context, auth Authorize, id string, revision int64, operation string) error {
	if !validOperationID(operation) || id == "" {
		return ErrInput
	}
	return s.transaction(ctx, auth, func(tx *sql.Tx) error {
		var o livechannels.Owner
		err := tx.QueryRowContext(ctx, `SELECT authority,account_id,profile_id FROM dvr_rules WHERE id=?`, id).Scan(&o.Authority, &o.AccountID, &o.ProfileID)
		if errors.Is(err, sql.ErrNoRows) {
			r, e := tx.ExecContext(ctx, `DELETE FROM admin_dvr_groups WHERE id=? AND revision=?`, id, revision)
			if e != nil {
				return e
			}
			n, _ := r.RowsAffected()
			if n != 1 {
				return ErrConflict
			}
			return nil
		}
		if err != nil {
			return err
		}
		if s.DVR == nil {
			return ErrUnavailable
		}
		return mapDVRError(s.DVR.DeleteRule(dbwork.WithWrite(ctx, tx, dbwork.ClassInteractive), groupAuthority(auth), o, id, dvr.DeleteRuleInput{Mutation: dvr.Mutation{ExpectedRevision: revision, RequestID: digestOf([]string{"admin-delete-rule", operation})[:48]}, Future: "keep"}))
	})
}
