package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
)

const (
	RestrictedMemberFacetBaseSource = "simple-restricted-members-live-v1"
	RestrictedShowFacetBaseSource   = "simple-restricted-show-live-v1"
)

type restrictedFacetWitness struct {
	Version int                               `json:"version"`
	Kind    string                            `json:"kind"`
	Fence   compactcatalog.FacetProducerFence `json:"fence"`
}

// One raw accepted cursor, one pending policy reducer and one show/value group;
// no persisted per-class member/witness graph and no all-value map.
type restrictedFacetCursor struct {
	HasAfter     bool                      `json:"hasAfter"`
	Entity       int64                     `json:"entity"`
	Show         int64                     `json:"show"`
	Field        string                    `json:"field"`
	Value        string                    `json:"value"`
	Season       *int64                    `json:"season"`
	Number       int64                     `json:"number"`
	Pending      bool                      `json:"pending"`
	Policy       FacetBaseItemPolicyCursor `json:"policy"`
	HasGroup     bool                      `json:"hasGroup"`
	GroupShow    int64                     `json:"groupShow"`
	GroupField   string                    `json:"groupField"`
	GroupValue   string                    `json:"groupValue"`
	GroupVisible bool                      `json:"groupVisible"`
}

// CaptureRestrictedFacetBaseWitness names an exact LIVE branch. Published
// restricted-decade counts have a different linked-episode oracle and must
// have their own adapter; existing active legacy classes therefore fail closed.
// Nonshow mixed show/episode layouts also fail closed: legacy maintained and
// member-live predicates differ there, so ordinary member facts are insufficient.
func CaptureRestrictedFacetBaseWitness(ctx context.Context, q compactcatalog.ReadQuery, scope FacetBaseScope) (string, error) {
	if !scope.valid() {
		return "", ErrFacetBaseInput
	}
	r, err := parseFacetBasePolicy(scope.Policy)
	if err != nil {
		return "", err
	}
	if !r.Active() {
		return "", ErrFacetBaseUnavailable
	}
	var kind string
	if err = q.QueryRowContext(ctx, `SELECT l.kind FROM catalog_libraries c JOIN libraries l ON l.id=c.library_id WHERE c.id=? AND c.library_id=? AND c.retired=0`, scope.LibraryID, scope.Library).Scan(&kind); err != nil {
		return "", err
	}
	shows := kind == "tv" || kind == "anime"
	if isReadingLibrary(kind) || (shows && scope.Source != RestrictedShowFacetBaseSource) || (!shows && scope.Source != RestrictedMemberFacetBaseSource) {
		return "", ErrFacetBaseUnavailable
	}
	if !shows {
		var mixed bool
		if err = q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_browse_rows WHERE library_id=? AND kind IN(?,?))`, scope.LibraryID, compactcatalog.Show, compactcatalog.Episode).Scan(&mixed); err != nil {
			return "", err
		}
		if mixed {
			return "", ErrFacetBaseUnavailable
		}
	}
	if err = rejectPublishedFacetDecade(ctx, q, scope, r); err != nil {
		return "", err
	}
	f, err := compactcatalog.CaptureFacetProducerFence(ctx, q)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(restrictedFacetWitness{Version: 1, Kind: kind, Fence: f})
	return string(raw), err
}
func rejectPublishedFacetDecade(ctx context.Context, q compactcatalog.ReadQuery, scope FacetBaseScope, r identity.ContentRestrictions) error {
	if scope.Field != "decade" {
		return nil
	}
	// This lookup identifies the OLD dispatch branch only. It is never used as a
	// policy equivalence key or as evidence that that class has exact membership.
	key, _ := visibilityClassKey(scope.Library, r)
	var published bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM compact_visibility_classes c WHERE c.class_key=? AND c.library_id=? AND c.retired=0 AND c.active_generation>0)`, key, scope.LibraryID).Scan(&published)
	if err != nil {
		return err
	}
	if published {
		return ErrFacetBaseUnavailable
	}
	return nil
}

type RestrictedFacetBaseProducer struct{}

func (p RestrictedFacetBaseProducer) validate(ctx context.Context, tx *sql.Tx, s FacetBaseSnapshot) (identity.ContentRestrictions, error) {
	r, err := parseFacetBasePolicy(s.Scope.Policy)
	if err != nil {
		return r, err
	}
	var w restrictedFacetWitness
	if !r.Active() || json.Unmarshal([]byte(s.Witness), &w) != nil || w.Version != 1 {
		return r, ErrFacetBaseUnavailable
	}
	shows := w.Kind == "tv" || w.Kind == "anime"
	if (shows && s.Scope.Source != RestrictedShowFacetBaseSource) || (!shows && s.Scope.Source != RestrictedMemberFacetBaseSource) {
		return r, ErrFacetBaseUnavailable
	}
	if err = compactcatalog.ValidateFacetProducerFence(ctx, tx, w.Fence); err != nil {
		return r, err
	}
	return r, rejectPublishedFacetDecade(ctx, tx, s.Scope, r)
}
func (p RestrictedFacetBaseProducer) ValidatePublication(ctx context.Context, tx *sql.Tx, s FacetBaseSnapshot) error {
	_, err := p.validate(ctx, tx, s)
	return err
}
func (p RestrictedFacetBaseProducer) Next(ctx context.Context, tx *sql.Tx, s FacetBaseSnapshot, cursor string) (FacetBaseBatch, error) {
	r, err := p.validate(ctx, tx, s)
	if err != nil {
		return FacetBaseBatch{}, err
	}
	var c restrictedFacetCursor
	if cursor != "" && json.Unmarshal([]byte(cursor), &c) != nil {
		return FacetBaseBatch{}, ErrFacetBaseInput
	}
	var out FacetBaseBatch
	if c.Pending {
		done, visible, err := StepFacetBaseItemPolicy(ctx, tx, r, &c.Policy)
		if err != nil {
			return out, err
		}
		if done {
			c.Pending = false
			if s.Scope.Source == RestrictedShowFacetBaseSource {
				c.GroupVisible = c.GroupVisible || visible
			} else if visible {
				f, err := incrementFacetBaseFact(ctx, tx, s, c.Value, 1)
				if err != nil {
					return out, err
				}
				out.Facts = []FacetBaseFact{f}
			}
		}
	} else if s.Scope.Source == RestrictedShowFacetBaseSource {
		if err = p.nextShow(ctx, tx, s, &c, &out); err != nil {
			return out, err
		}
	} else {
		query := `SELECT entity_id,field,value,library_id FROM catalog_facet_members`
		args := []any{}
		if c.HasAfter {
			query += ` WHERE (entity_id,field,value)>(?,?,?)`
			args = append(args, c.Entity, c.Field, c.Value)
		}
		query += ` ORDER BY entity_id,field,value LIMIT 1`
		var entity, library int64
		var field, value string
		err = tx.QueryRowContext(ctx, query, args...).Scan(&entity, &field, &value, &library)
		if errors.Is(err, sql.ErrNoRows) {
			out.Done = true
		} else if err != nil {
			return out, err
		} else {
			c.HasAfter = true
			c.Entity, c.Field, c.Value = entity, field, value
			if library == s.Scope.LibraryID && field == s.Scope.Field && value != "" {
				var item sql.NullInt64
				err = tx.QueryRowContext(ctx, `SELECT item_id FROM catalog_browse_rows WHERE entity_id=?`, entity).Scan(&item)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return out, err
				}
				if err == nil && item.Valid {
					c.Pending = true
					c.Policy = FacetBaseItemPolicyCursor{Item: item.Int64}
				}
			}
		}
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		return out, err
	}
	out.Cursor = string(encoded)
	return out, nil
}

func (p RestrictedFacetBaseProducer) nextShow(ctx context.Context, tx *sql.Tx, s FacetBaseSnapshot, c *restrictedFacetCursor, out *FacetBaseBatch) error {
	year := s.Scope.Field == "year" || s.Scope.Field == "decade"
	var show, episode int64
	var field, value string
	var err error
	var season sql.NullInt64
	var number int64
	if year {
		// The existing show/season/number/entity index is the full raw key.
		// NULL season needs a separate bounded range; never claim that this
		// index has (show,entity) order or add another per-episode index.
		selectRow := func(where string, args ...any) error {
			return tx.QueryRowContext(ctx, `SELECT show_id,season_id,number,entity_id FROM catalog_episodes INDEXED BY catalog_episodes_show `+where+` ORDER BY show_id,season_id,number,entity_id LIMIT 1`, args...).Scan(&show, &season, &number, &episode)
		}
		if !c.HasAfter {
			err = selectRow("")
		} else if c.Season != nil {
			err = selectRow(`WHERE (show_id,season_id,number,entity_id)>(?,?,?,?)`, c.Show, *c.Season, c.Number, c.Entity)
		} else {
			err = selectRow(`WHERE show_id=? AND season_id IS NULL AND (number,entity_id)>(?,?)`, c.Show, c.Number, c.Entity)
			if errors.Is(err, sql.ErrNoRows) {
				err = selectRow(`WHERE show_id=? AND season_id IS NOT NULL`, c.Show)
			}
			if errors.Is(err, sql.ErrNoRows) {
				err = selectRow(`WHERE show_id>?`, c.Show)
			}
		}
		field = s.Scope.Field
	} else {
		q := `SELECT show_id,field,value,episode_id FROM catalog_show_facet_episodes INDEXED BY catalog_show_facet_episodes_show_value`
		args := []any{}
		if c.HasAfter {
			q += ` WHERE (show_id,field,value,episode_id)>(?,?,?,?)`
			args = append(args, c.Show, c.Field, c.Value, c.Entity)
		}
		q += ` ORDER BY show_id,field,value,episode_id LIMIT 1`
		err = tx.QueryRowContext(ctx, q, args...).Scan(&show, &field, &value, &episode)
	}
	end := errors.Is(err, sql.ErrNoRows)
	if err != nil && !end {
		return err
	}
	changed := !end && c.HasGroup && (show != c.GroupShow || field != c.GroupField || !year && value != c.GroupValue)
	if c.HasGroup && (end || changed) {
		if c.GroupVisible && c.GroupField == s.Scope.Field && c.GroupValue != "" {
			f, err := incrementFacetBaseFact(ctx, tx, s, c.GroupValue, 1)
			if err != nil {
				return err
			}
			out.Facts = []FacetBaseFact{f}
		}
		c.HasGroup = false
		c.GroupVisible = false
		// Leave the prefetched next group's raw key unconsumed. The next quantum
		// reads it again; accepted group counts and this checkpoint commit together.
		if !end {
			return nil
		}
	}
	if end {
		out.Done = true
		return nil
	}
	if !c.HasGroup {
		c.HasGroup = true
		c.GroupShow, c.GroupField, c.GroupValue = show, field, value
	}
	c.HasAfter = true
	if year {
		c.Season = nil
		if season.Valid {
			v := season.Int64
			c.Season = &v
		}
		c.Number = number
	}
	c.Show, c.Field, c.Value, c.Entity = show, field, value, episode
	if field != s.Scope.Field || c.GroupVisible {
		return nil
	}
	var item sql.NullInt64
	var library int64
	var showYear sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT ep.item_id,ep.library_id,sh.year FROM catalog_browse_rows ep JOIN catalog_browse_rows sh ON sh.entity_id=? WHERE ep.entity_id=?`, show, episode).Scan(&item, &library, &showYear)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if library != s.Scope.LibraryID || !item.Valid {
		return nil
	}
	if year {
		if !showYear.Valid || showYear.Int64 < 1800 || showYear.Int64 > 2199 {
			return nil
		}
		v := showYear.Int64
		if field == "decade" {
			v = v / 10 * 10
		}
		c.GroupValue = strconv.FormatInt(v, 10)
	}
	c.Pending = true
	c.Policy = FacetBaseItemPolicyCursor{Item: item.Int64}
	return nil
}
func incrementFacetBaseFact(ctx context.Context, tx *sql.Tx, s FacetBaseSnapshot, value string, delta int64) (FacetBaseFact, error) {
	f, err := unrestrictedFacetPublicFact(ctx, tx, s.Scope.Field, value, delta)
	if err != nil {
		return f, err
	}
	var previous int64
	err = tx.QueryRowContext(ctx, `SELECT -r.negative_count FROM catalog_facet_base_rows r JOIN catalog_facet_base_values v ON v.id=r.value_id WHERE r.base_id=? AND v.library_id=? AND v.field=? AND v.value=?`, s.ID, s.Scope.LibraryID, s.Scope.Field, f.Value).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return f, err
	}
	if delta < 0 || previous > math.MaxInt64-delta {
		return f, ErrFacetBaseInput
	}
	f.Count = previous + delta
	return f, nil
}
