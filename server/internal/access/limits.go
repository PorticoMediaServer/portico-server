package access

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// ContentRatings is the published ladder the maximum-content-rating limit
// compares against, from least to most restricted. A rating a library carries
// that is not on this ladder is treated as unrated: it is allowed unless the
// limit also sets allowUnrated to false, because silently hiding a title whose
// rating the server does not recognise is worse than showing it.
var ContentRatings = []string{"TV-Y", "TV-Y7", "G", "TV-G", "PG", "TV-PG", "PG-13", "TV-14", "R", "TV-MA", "NC-17"}

func ratingRank(v string) int {
	v = strings.ToUpper(strings.TrimSpace(v))
	for i, r := range ContentRatings {
		if r == v {
			return i + 1
		}
	}
	return 0
}

// isCanonicalChannelID reports whether id is a canonical v1 channel id
// ("live:<source>:<channel>" or "library:<channel>", with no empty parts and
// no extra colons). The allow/deny lists store these only; anything else is
// rejected at write time.
func isCanonicalChannelID(id string) bool {
	if strings.HasPrefix(id, "live:") {
		rest := strings.TrimPrefix(id, "live:")
		source, channel, ok := strings.Cut(rest, ":")
		return ok && source != "" && channel != "" && !strings.Contains(channel, ":")
	}
	if strings.HasPrefix(id, "library:") {
		channel := strings.TrimPrefix(id, "library:")
		return channel != "" && !strings.Contains(channel, ":")
	}
	return false
}

// ScheduleWindow is one weekday/time band in the member's own timezone. Days are
// 0 (Sunday) through 6. StartMinute and EndMinute are minutes past local
// midnight; a window whose end is not after its start wraps past midnight.
type ScheduleWindow struct {
	Days        []int `json:"days"`
	StartMinute int   `json:"startMinute"`
	EndMinute   int   `json:"endMinute"`
}

// AccessSchedule denies sign-in and playback outside its windows. An empty
// window list means no schedule restriction at all, which is the default.
type AccessSchedule struct {
	Timezone string           `json:"timezone"`
	Windows  []ScheduleWindow `json:"windows"`
}

// ChannelPolicy restricts live channels by their canonical v1 channel id
// only ("live:<source>:<channel>" or "library:<channel>"). Mode "all" ignores
// the list; "allow" permits only the listed ids; "deny" permits everything else.
type ChannelPolicy struct {
	Mode     string   `json:"mode"`
	Channels []string `json:"channels"`
}

// TagPolicy denies items carrying any of these catalog label or tag values.
type TagPolicy struct {
	DeniedLabels []string `json:"deniedLabels"`
}

// Limits is one member's access envelope.
//
// Where each limit is enforced:
//
//	MaxStreams           — Enforcer.AdmitPlayback, called before a playback lease
//	                       is created; counts live playback_sessions rows plus
//	                       live playback_v1_sessions rows (a v1 session's legacy
//	                       presentation row is excluded, so one stream counts once).
//	RemoteBitrateKbps    — Enforcer.AdmitPlayback returns it as a clamp, and the
//	                       delivery policy applies it as MaxVideoBitrateBPS for a
//	                       remote transport class.
//	MaxContentRating     — Enforcer.AdmitPlayback (item rating vs the ladder), and
//	                       Enforcer.VisibleItem / VisibilityClause for catalog
//	                       visibility.
//	TagPolicy            — the same three.
//	Schedule             — Enforcer.AdmitSession, Enforcer.AdmitPlayback and
//	                       Enforcer.AdmitChannel.
//	ChannelPolicy        — Enforcer.AdmitChannel, called by the v1 channel
//	                       start path with canonical v1 ids only.
type Limits struct {
	MaxStreams        int            `json:"maxStreams"`
	RemoteBitrateKbps int            `json:"remoteBitrateKbps"`
	MaxContentRating  string         `json:"maxContentRating"`
	AllowUnrated      bool           `json:"allowUnrated"`
	Schedule          AccessSchedule `json:"schedule"`
	Channels          ChannelPolicy  `json:"channelPolicy"`
	Tags              TagPolicy      `json:"tagPolicy"`
}

// DefaultLimits is an unrestricted member: zero means unlimited everywhere, an
// empty rating means no ceiling, and unrated content is allowed.
func DefaultLimits() Limits {
	return Limits{AllowUnrated: true, Schedule: AccessSchedule{Windows: []ScheduleWindow{}}, Channels: ChannelPolicy{Mode: "all", Channels: []string{}}, Tags: TagPolicy{DeniedLabels: []string{}}}
}

// LimitsDocument is the wire shape every limits read and write returns.
type LimitsDocument struct {
	AccountID string `json:"accountId"`
	Revision  int64  `json:"revision"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Limits    Limits `json:"limits"`
}

func normalizeLimits(v *Limits) error {
	fields := []string{}
	if v.MaxStreams < 0 || v.MaxStreams > 1000 {
		fields = append(fields, "limits.maxStreams")
	}
	if v.RemoteBitrateKbps < 0 || v.RemoteBitrateKbps > 200000 {
		fields = append(fields, "limits.remoteBitrateKbps")
	}
	if v.MaxContentRating != "" && ratingRank(v.MaxContentRating) == 0 {
		fields = append(fields, "limits.maxContentRating")
	}
	if v.Schedule.Timezone != "" {
		if _, e := time.LoadLocation(v.Schedule.Timezone); e != nil {
			fields = append(fields, "limits.schedule.timezone")
		}
	}
	if len(v.Schedule.Windows) > 64 {
		fields = append(fields, "limits.schedule.windows")
	}
	for i := range v.Schedule.Windows {
		w := &v.Schedule.Windows[i]
		if len(w.Days) == 0 || len(w.Days) > 7 || w.StartMinute < 0 || w.StartMinute > 1439 || w.EndMinute < 0 || w.EndMinute > 1440 || w.StartMinute == w.EndMinute {
			fields = append(fields, "limits.schedule.windows")
			break
		}
		seen := map[int]bool{}
		for _, d := range w.Days {
			if d < 0 || d > 6 || seen[d] {
				fields = append(fields, "limits.schedule.windows")
				break
			}
			seen[d] = true
		}
		sort.Ints(w.Days)
	}
	switch v.Channels.Mode {
	case "", "all":
		v.Channels.Mode = "all"
	case "allow", "deny":
	default:
		fields = append(fields, "limits.channelPolicy.mode")
	}
	if v.Channels.Channels == nil {
		v.Channels.Channels = []string{}
	}
	if len(v.Channels.Channels) > 512 {
		fields = append(fields, "limits.channelPolicy.channels")
	}
	for _, id := range v.Channels.Channels {
		if !safeText(id, 200) || strings.TrimSpace(id) == "" || !isCanonicalChannelID(id) {
			fields = append(fields, "limits.channelPolicy.channels")
			break
		}
	}
	if v.Tags.DeniedLabels == nil {
		v.Tags.DeniedLabels = []string{}
	}
	if len(v.Tags.DeniedLabels) > 256 {
		fields = append(fields, "limits.tagPolicy.deniedLabels")
	}
	for i, label := range v.Tags.DeniedLabels {
		if !safeText(label, 120) || strings.TrimSpace(label) == "" {
			fields = append(fields, "limits.tagPolicy.deniedLabels")
			break
		}
		v.Tags.DeniedLabels[i] = strings.TrimSpace(label)
	}
	if v.Schedule.Windows == nil {
		v.Schedule.Windows = []ScheduleWindow{}
	}
	v.MaxContentRating = strings.ToUpper(strings.TrimSpace(v.MaxContentRating))
	if len(fields) > 0 {
		return &ValidationError{fields}
	}
	return nil
}

func readLimits(tx *sql.Tx, account string) (LimitsDocument, error) {
	out := LimitsDocument{AccountID: account, Revision: 1, Limits: DefaultLimits()}
	var body string
	var updated int64
	err := tx.QueryRow(`SELECT revision,updated_ms,body FROM access_limits WHERE account_id=?`, account).Scan(&out.Revision, &updated, &body)
	if errors.Is(err, sql.ErrNoRows) {
		// A member with no stored row is unrestricted at revision 1, so a client
		// can write against that revision without a create step.
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Limits = DefaultLimits()
	if err = json.Unmarshal([]byte(body), &out.Limits); err != nil {
		return out, err
	}
	out.UpdatedAt = time.UnixMilli(updated).UTC().Format(time.RFC3339)
	return out, nil
}

// Limits returns one member's limits document.
func (s *Store) Limits(ctx context.Context, auth Authorize, account string) (out LimitsDocument, err error) {
	if !validID(account) {
		return out, invalid("accountId")
	}
	err = s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		var e error
		if e = accountExists(tx, account); e != nil {
			return e
		}
		out, e = readLimits(tx, account)
		return e
	})
	return
}

type LimitsChange struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
	Limits           Limits `json:"limits"`
}

// SetLimits replaces a member's limits. The whole envelope is written at once so
// a client never observes half a policy.
func (s *Store) SetLimits(ctx context.Context, auth Authorize, account string, c LimitsChange) (out LimitsDocument, err error) {
	if !validID(account) {
		return out, invalid("accountId")
	}
	if !validOperationID(c.OperationID) {
		return out, invalid("operationId")
	}
	if err = normalizeLimits(&c.Limits); err != nil {
		return out, err
	}
	err = s.transaction(ctx, auth, func(tx *sql.Tx) error {
		scope := "limits:" + account
		body, want, e := receipt(tx, scope, c.OperationID, c)
		if e != nil {
			return e
		}
		if body != "" {
			return json.Unmarshal([]byte(body), &out)
		}
		if e = accountExists(tx, account); e != nil {
			return e
		}
		current, e := readLimits(tx, account)
		if e != nil {
			return e
		}
		if current.Revision != c.ExpectedRevision {
			return &ConflictError{current.Revision}
		}
		revision, now := current.Revision+1, s.now()
		raw, _ := json.Marshal(c.Limits)
		if _, e = tx.Exec(`INSERT INTO access_limits VALUES(?,?,?,?) ON CONFLICT(account_id) DO UPDATE SET revision=excluded.revision,updated_ms=excluded.updated_ms,body=excluded.body`, account, revision, now, string(raw)); e != nil {
			return e
		}
		out = LimitsDocument{AccountID: account, Revision: revision, UpdatedAt: time.UnixMilli(now).UTC().Format(time.RFC3339), Limits: c.Limits}
		return saveReceipt(tx, scope, c.OperationID, want, out, now)
	})
	return
}

func accountExists(tx *sql.Tx, account string) error {
	var ok bool
	if e := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM accounts WHERE id=?)`, account).Scan(&ok); e != nil {
		return e
	}
	if !ok {
		return sql.ErrNoRows
	}
	return nil
}
