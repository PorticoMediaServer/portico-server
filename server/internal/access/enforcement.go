package access

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"portico.local/server/internal/identity"
)

// Admission failures. Each one is a policy answer, not a fault: the message is
// written for the person who hit it, because a client shows it verbatim.
var (
	ErrStreamLimit   = errors.New("This account already has as many streams playing as its limit allows.")
	ErrSchedule      = errors.New("This account is outside the hours it is allowed to watch.")
	ErrContentRating = errors.New("This title's rating is above the limit set for this account.")
	ErrLabelDenied   = errors.New("This title carries a label that is not allowed for this account.")
	ErrChannelDenied = errors.New("This channel is not allowed for this account.")
	ErrDeviceTrust   = errors.New("This device is waiting for approval on this server.")
)

// Enforcer applies per-member limits at admission. It reads the limits envelope
// inside the caller's transaction so a limit saved a moment ago is already in
// force on the next operation, with no cache to invalidate.
//
// An administrator is not exempt by accident: limits are stored per account and
// an owner or admin simply has none by default. A stored envelope on an
// administrative account is applied like any other.
type Enforcer struct {
	Store *Store
	// ApprovalRequired is the server's device approval policy. The caller reads
	// it from the settings registry and passes the answer in, rather than handing
	// over a closure: SQLite runs on a single writer connection here, so a nested
	// read inside the admission transaction would deadlock against it.
	ApprovalRequired bool
}

// Admission is what the caller asks about.
type Admission struct {
	ItemID string
	// Remote is true when the request arrived over a non-LAN transport; the
	// remote bitrate cap applies only then.
	Remote bool
	// DeviceID is the client's device identifier, when it sent one.
	DeviceID string
	// ReplanSessionID is the v1 session this admission continues or replaces
	// (a quality/version/audio/part change, a queue transition, a replacing
	// start). It already holds its stream, so it is not counted again against
	// maxStreams. ReplanMediaID is a prepared next entry's media session that
	// exists before its v1 session does; it is not a second stream either.
	ReplanSessionID string
	ReplanMediaID   string
}

// Decision carries the clamps a permitted request still has to respect.
type Decision struct {
	// MaxVideoBitrateBPS is 0 when the account has no remote bitrate cap, or the
	// request is not remote.
	MaxVideoBitrateBPS int
}

func (e *Enforcer) limits(ctx context.Context, tx *sql.Tx, account string) (Limits, error) {
	if e == nil || e.Store == nil {
		return DefaultLimits(), nil
	}
	doc, err := readLimits(tx, account)
	return doc.Limits, err
}

// AdmitSession is the sign-in side: the access schedule. There is no session
// count: how many devices a member has signed in never blocks anyone.
func (e *Enforcer) AdmitSession(ctx context.Context, tx *sql.Tx, p identity.Principal) error {
	limits, err := e.limits(ctx, tx, p.AccountID)
	if err != nil {
		return err
	}
	return withinSchedule(limits.Schedule, e.now())
}

// AdmitPlayback is the playback side: concurrent stream count, the schedule, the
// maximum content rating, denied labels, device trust and the remote bitrate
// cap. It runs before a playback lease exists, so the count it reads excludes
// the request being admitted.
func (e *Enforcer) AdmitPlayback(ctx context.Context, tx *sql.Tx, p identity.Principal, q Admission) (Decision, error) {
	var out Decision
	limits, err := e.limits(ctx, tx, p.AccountID)
	if err != nil {
		return out, err
	}
	now := e.now()
	if err = withinSchedule(limits.Schedule, now); err != nil {
		return out, err
	}
	if q.DeviceID != "" && e.ApprovalRequired {
		trust, err := DeviceTrust(ctx, tx, q.DeviceID)
		if err != nil {
			return out, err
		}
		if trust != "approved" {
			return out, ErrDeviceTrust
		}
	}
	if limits.MaxStreams > 0 {
		// One stream is one count, whatever protocol holds it. Legacy
		// playback_sessions rows back v2 occurrences and the presentations of
		// v1 VOD/audio sessions, so those are excluded here and counted once
		// through their v1 row instead; v1 channel sessions never create a
		// legacy row, so without the second count they would play for free.
		var n int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM playback_sessions WHERE account_id=? AND state NOT IN('stopped','ended','failed') AND expires_at>? AND id NOT IN(SELECT media_session_id FROM playback_v1_sessions WHERE account_id=? AND ended_ms=0 AND COALESCE(media_session_id,'')<>'') AND id<>?`, p.AccountID, now.UTC().Format(time.RFC3339), p.AccountID, q.ReplanMediaID).Scan(&n); err != nil {
			return out, err
		}
		var v1 int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM playback_v1_sessions WHERE account_id=? AND ended_ms=0 AND id<>?`, p.AccountID, q.ReplanSessionID).Scan(&v1); err != nil {
			return out, err
		}
		if n+v1 >= limits.MaxStreams {
			return out, ErrStreamLimit
		}
	}
	if q.ItemID != "" {
		if err = itemAllowed(ctx, tx, limits, q.ItemID); err != nil {
			return out, err
		}
	}
	if q.Remote && limits.RemoteBitrateKbps > 0 {
		out.MaxVideoBitrateBPS = limits.RemoteBitrateKbps * 1000
	}
	return out, nil
}

// RemoteBitrateCap is the member's remote bitrate cap in bits per second (zero
// when none), read without admitting anything: a plan preview must match what a
// start would get, and must not count as a stream or observe the device.
func (e *Enforcer) RemoteBitrateCap(ctx context.Context, tx *sql.Tx, p identity.Principal) (int, error) {
	limits, err := e.limits(ctx, tx, p.AccountID)
	if err != nil {
		return 0, err
	}
	return limits.RemoteBitrateKbps * 1000, nil
}

// AdmitChannel applies the access schedule and the live channel allow/deny
// policy. Both the stored lists and the channel under test are canonical v1
// channel ids ("live:<source>:<channel>" or "library:<channel>"); anything
// else never matches, so a non-canonical list entry denies nothing (deny) or
// everything (allow) until it is cleaned.
func (e *Enforcer) AdmitChannel(ctx context.Context, tx *sql.Tx, p identity.Principal, channel string) error {
	limits, err := e.limits(ctx, tx, p.AccountID)
	if err != nil {
		return err
	}
	if err = withinSchedule(limits.Schedule, e.now()); err != nil {
		return err
	}
	return channelAllowed(limits.Channels, channel)
}

// VisibleItem is the catalog-visibility side of the content rating and label
// limits: a title a member may not play is also a title the member may not see.
// Detail and playback call it per item; the browse projection applies the same
// rule in bulk through VisibilityClause.
func (e *Enforcer) VisibleItem(ctx context.Context, tx *sql.Tx, p identity.Principal, item string) error {
	limits, err := e.limits(ctx, tx, p.AccountID)
	if err != nil {
		return err
	}
	return itemAllowed(ctx, tx, limits, item)
}

// VisibilityClause returns a SQL predicate and its arguments that keep only rows
// a member may see, given an expression yielding the integer catalogue entity id.
// It returns an empty string when the member's limits hide nothing, so a caller
// can skip the join entirely in the common case.
func (e *Enforcer) VisibilityClause(ctx context.Context, tx *sql.Tx, p identity.Principal, itemColumn string) (string, []any, error) {
	limits, err := e.limits(ctx, tx, p.AccountID)
	if err != nil {
		return "", nil, err
	}
	clauses, args := []string{}, []any{}
	if limits.MaxContentRating != "" {
		// Ratings are compared through the published ladder, so an unknown value
		// is "unrated" and follows allowUnrated rather than sorting as text.
		ladder, values := ratingLadderSQL(itemColumn, limits.MaxContentRating, limits.AllowUnrated)
		clauses = append(clauses, ladder)
		args = append(args, values...)
	}
	for _, label := range limits.Tags.DeniedLabels {
		clauses = append(clauses, `NOT EXISTS(SELECT 1 FROM catalog_item_attribute_edges dca JOIN catalog_attribute_terms term ON term.id=dca.term_id WHERE dca.item_id=`+itemColumn+` AND term.field_id IN(2,3) AND lower(dca.source_value)=?)`)
		args = append(args, strings.ToLower(label))
	}
	if len(clauses) == 0 {
		return "", nil, nil
	}
	return publishedItemSQL(itemColumn) + ` AND (` + strings.Join(clauses, " AND ") + `)`, args, nil
}

// publishedItemSQL fences removed facts even for an unrestricted caller: the
// entity exists, is not retired, and its library is live. It replaces the
// deleted compactcatalog.PublishedItemSQL: catalogue facts are written
// synchronously now, so there is no projection lag to fence and no separate
// attribute fence. Inputs are trusted SQL expressions, never request text.
func publishedItemSQL(itemColumn string) string {
	return `EXISTS(SELECT 1 FROM catalog_entities pe JOIN catalog_libraries l ON l.id=pe.library_id WHERE pe.id=` + itemColumn + ` AND l.retired=0 AND pe.retired=0 AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements r WHERE r.item_id=pe.id))`
}

// ratingLadderSQL builds an "allowed values" test from the ladder for the item
// id expression named by column. A rating that is not on the ladder is unrated,
// and unrated titles pass only when the limit allows them.
func ratingLadderSQL(column, max string, allowUnrated bool) (string, []any) {
	// Same source as itemAllowed: the catalog attribute, else the item's own rating.
	rating := `upper(trim(COALESCE((SELECT rca.source_value FROM catalog_item_attribute_edges rca JOIN catalog_attribute_terms term ON term.id=rca.term_id WHERE rca.item_id=` + column + ` AND term.field_id=1 ORDER BY rca.source_rowid LIMIT 1),(SELECT d.content_rating FROM catalog_item_details d WHERE d.entity_id=` + column + `),'')))`
	ceiling := ratingRank(max)
	allowed, marks := []any{}, []string{}
	ladder, ladderMarks := []any{}, []string{}
	for _, r := range ContentRatings {
		ladder, ladderMarks = append(ladder, r), append(ladderMarks, "?")
		if ratingRank(r) <= ceiling {
			allowed, marks = append(allowed, r), append(marks, "?")
		}
	}
	clause := rating + ` IN(` + strings.Join(marks, ",") + `)`
	if !allowUnrated {
		return clause, allowed
	}
	return `(` + rating + ` NOT IN(` + strings.Join(ladderMarks, ",") + `) OR ` + clause + `)`, append(ladder, allowed...)
}

func itemAllowed(ctx context.Context, tx *sql.Tx, limits Limits, item string) error {
	if limits.MaxContentRating == "" && len(limits.Tags.DeniedLabels) == 0 {
		return nil
	}
	var itemID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&itemID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrContentRating
		}
		return err
	}
	var published bool
	if err := tx.QueryRowContext(ctx, `SELECT `+publishedItemSQL("?"), itemID).Scan(&published); err != nil {
		return err
	}
	if !published {
		return ErrContentRating
	}
	if limits.MaxContentRating != "" {
		var rating string
		e := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT a.source_value FROM catalog_item_attribute_edges a JOIN catalog_attribute_terms term ON term.id=a.term_id WHERE a.item_id=? AND term.field_id=1 ORDER BY a.source_rowid LIMIT 1),(SELECT d.content_rating FROM catalog_item_details d WHERE d.entity_id=?),'')`, itemID, itemID).Scan(&rating)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		rank := ratingRank(rating)
		if rank == 0 && !limits.AllowUnrated || rank > ratingRank(limits.MaxContentRating) {
			return ErrContentRating
		}
	}
	for _, label := range limits.Tags.DeniedLabels {
		var carries bool
		if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_item_attribute_edges a JOIN catalog_attribute_terms term ON term.id=a.term_id WHERE a.item_id=? AND term.field_id IN(2,3) AND lower(a.source_value)=?)`, itemID, strings.ToLower(label)).Scan(&carries); e != nil {
			return e
		}
		if carries {
			return ErrLabelDenied
		}
	}
	return nil
}

func channelAllowed(policy ChannelPolicy, channel string) error {
	if policy.Mode == "" || policy.Mode == "all" || channel == "" {
		return nil
	}
	listed := false
	for _, id := range policy.Channels {
		if id == channel {
			listed = true
			break
		}
	}
	if policy.Mode == "allow" && !listed || policy.Mode == "deny" && listed {
		return ErrChannelDenied
	}
	return nil
}

func (e *Enforcer) now() time.Time {
	if e != nil && e.Store != nil && e.Store.Now != nil {
		return e.Store.Now()
	}
	return time.Now()
}

// withinSchedule evaluates the access schedule in the member's own timezone. An
// empty window list means no restriction. A window whose end minute is not after
// its start wraps past local midnight and is evaluated as two bands.
func withinSchedule(schedule AccessSchedule, now time.Time) error {
	if len(schedule.Windows) == 0 {
		return nil
	}
	location := time.UTC
	if schedule.Timezone != "" {
		if loaded, err := time.LoadLocation(schedule.Timezone); err == nil {
			location = loaded
		}
	}
	local := now.In(location)
	day, minute := int(local.Weekday()), local.Hour()*60+local.Minute()
	for _, w := range schedule.Windows {
		if w.EndMinute > w.StartMinute {
			if containsDay(w.Days, day) && minute >= w.StartMinute && minute < w.EndMinute {
				return nil
			}
			continue
		}
		// Wrapping window: the tail of the listed day, or the head of the next.
		if containsDay(w.Days, day) && minute >= w.StartMinute {
			return nil
		}
		if containsDay(w.Days, (day+6)%7) && minute < w.EndMinute {
			return nil
		}
	}
	return ErrSchedule
}

func containsDay(days []int, day int) bool {
	for _, d := range days {
		if d == day {
			return true
		}
	}
	return false
}
