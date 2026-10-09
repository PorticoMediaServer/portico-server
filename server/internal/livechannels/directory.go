package livechannels

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/thirdparty/sqlite"
)

// Directory ordering is identical on every client: English Unicode collation,
// numeric channel prefixes (including decimal/exponent prefixes), then stable ids.
// Unlike SQLite CAST, a nonnumeric number sorts after numeric channel numbers.
var channelNumericPrefix = regexp.MustCompile(`^[+-]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`)
var directoryCollators = sync.Pool{New: func() any { return collate.New(language.English) }}

func init() {
	sqlite.MustRegisterCollationUtf8("portico_guide_name", func(a, b string) int {
		c := directoryCollators.Get().(*collate.Collator)
		n := c.CompareString(a, b)
		directoryCollators.Put(c)
		return n
	})
	sqlite.MustRegisterDeterministicScalarFunction("portico_channel_number", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		s, _ := args[0].(string)
		prefix := channelNumericPrefix.FindString(strings.TrimSpace(s))
		if prefix == "" {
			return nil, nil
		}
		n, err := strconv.ParseFloat(prefix, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, nil
		}
		return n, nil
	})
}

var ErrDirectoryAnchorNotFound = errors.New("The channel is not available in this view.")

var ErrChannelIdentifierConflict = errors.New("Two authorized channel sources use the same channel identifier. The server owner must change the conflicting Library Channel identifier.")

type DirectoryRow struct {
	Channel    Channel     `json:"channel"`
	Source     GuideSource `json:"source"`
	Revision   int64       `json:"revision"`
	Position   int         `json:"position"`
	LogoItemID string      `json:"-"`
}

// DirectoryAuthority is a source-level policy projection inside this snapshot.
// It is separate from Authority, whose callback can additionally restrict
// individual channels. HTTP's source membership policy is source-level.
type DirectoryScope struct {
	Fence              string
	AllowedLiveSources []string
	LibraryRows        []DirectoryRow
	Viewer             Owner
}
type DirectoryAuthority func(context.Context, *sql.Tx) (DirectoryScope, error)
type DirectoryQuery struct {
	Kind              string `json:"kind"`
	SourceID          string `json:"sourceId"`
	Search            string `json:"search"`
	Group             string `json:"group"`
	FavoritesOnly     bool   `json:"favorites"`
	IncludeHidden     bool   `json:"includeHidden"`
	Sort              string `json:"sort"`
	Offset            int    `json:"offset"`
	Limit             int    `json:"limit"`
	Revision          string `json:"-"`
	AnchorChannelID   string `json:"-"`
	AnchorSourceID    string `json:"-"`
	AnchorProvenance  string `json:"-"`
	Direction         string `json:"-"`
	DeliveryAvailable bool   `json:"-"`
}
type Directory struct {
	ViewerFence string        `json:"viewerFence"`
	Revision    string        `json:"revision"`
	Total       int           `json:"total"`
	Offset      int           `json:"offset"`
	Limit       int           `json:"limit"`
	Channels    []Channel     `json:"channels"`
	Sources     []GuideSource `json:"sources"`
}
type GuideGroupCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type SourceSummary struct {
	GuideSource
	Position           int               `json:"position"`
	RecordAvailable    bool              `json:"recordAvailable"`
	GuideDays          int               `json:"guideDays"`
	ChannelCount       int               `json:"channelCount"`
	Groups             []string          `json:"groups"`
	GroupCounts        []GuideGroupCount `json:"groupCounts"`
	FavoriteCount      int               `json:"favoriteCount"`
	UnavailableReasons map[string]int    `json:"unavailableReasons"`
}
type DirectorySummary struct {
	ViewerFence string          `json:"viewerFence"`
	Revision    string          `json:"revision"`
	Sources     []SourceSummary `json:"sources"`
}

// Every live row stays in SQLite. Only the <=64 library configurations cross
// the Go boundary to apply their selected-library policy. Count, source summary
// and global sort/paging use this same authorized, filtered relation.
const directoryEligibleSQL = `WITH candidates AS (
 SELECT c.channel_id,s.id source_id,'live-source' provenance,c.name,c.number,c.group_name,s.active_generation generation,'' logo_path,
 COALESCE(p.favorite,0) favorite,COALESCE(p.hidden,0) hidden,COALESCE(p.revision,0) preference_revision,
 s.name source_name,s.published_at,c.available_start,c.available_end,COALESCE(r.state,'manual') refresh_state,0 source_position,c.position channel_position,s.revision config_revision
 FROM live_sources s JOIN live_channel_versions c ON c.generation_id=s.active_generation
 LEFT JOIN live_channel_preferences p ON p.authority=? AND p.account_id=? AND p.profile_id=? AND p.source_id=s.id AND p.channel_id=c.channel_id
 LEFT JOIN live_remote_configs r ON r.source_id=s.id
 WHERE s.state='active' AND s.id IN (SELECT value FROM json_each(?))
 UNION ALL
 SELECT json_extract(value,'$.channel.id'),json_extract(value,'$.channel.sourceId'),'library-channel',json_extract(value,'$.channel.name'),json_extract(value,'$.channel.number'),json_extract(value,'$.channel.group'),json_extract(value,'$.channel.generation'),COALESCE(json_extract(value,'$.channel.logoPath'),''),
 json_extract(value,'$.channel.favorite'),json_extract(value,'$.channel.hidden'),json_extract(value,'$.channel.preferenceRevision'),json_extract(value,'$.source.name'),json_extract(value,'$.source.publishedAt'),json_extract(value,'$.source.availableStart'),json_extract(value,'$.source.availableEnd'),json_extract(value,'$.source.refreshState'),json_extract(value,'$.position'),json_extract(value,'$.position'),json_extract(value,'$.revision') FROM json_each(?)
), eligible AS (SELECT * FROM candidates WHERE (?='all' OR provenance=?)
 AND (?='' OR source_id=? OR (?='library' AND provenance='library-channel'))
 AND (?='' OR instr(lower(name||' '||group_name),lower(?))>0)
 AND (?='' OR group_name=?) AND (?=0 OR favorite=1) AND (?=1 OR hidden=0)) `

func directoryArgs(scope DirectoryScope, q DirectoryQuery) []any {
	live, _ := json.Marshal(scope.AllowedLiveSources)
	library, _ := json.Marshal(scope.LibraryRows)
	return []any{scope.Viewer.Authority, scope.Viewer.AccountID, scope.Viewer.ProfileID, string(live), string(library), q.Kind, q.Kind, q.SourceID, q.SourceID, q.SourceID, q.Search, q.Search, q.Group, q.Group, q.FavoritesOnly, q.IncludeHidden}
}
func validDirectoryQuery(q DirectoryQuery) bool {
	if q.Direction != "" {
		if (q.Direction != "next" && q.Direction != "previous") || q.AnchorChannelID == "" || len(q.AnchorChannelID) > 256 || q.AnchorSourceID == "" || len(q.AnchorSourceID) > 256 || (q.AnchorProvenance != string(LiveSource) && q.AnchorProvenance != string(LibraryChannel)) || q.Offset != 0 || q.Limit != 1 {
			return false
		}
	} else if q.AnchorChannelID != "" || q.AnchorSourceID != "" || q.AnchorProvenance != "" {
		return false
	}
	return (q.Kind == "all" || q.Kind == string(LiveSource) || q.Kind == string(LibraryChannel)) && (q.Sort == "server" || q.Sort == "number" || q.Sort == "name") && q.Offset >= 0 && q.Offset <= MaxSources*MaxChannels+64 && q.Limit >= 1 && q.Limit <= 50 && len(q.SourceID) <= 256 && len(q.Search) <= 120 && len(q.Group) <= 120 && len(q.Revision) <= 64
}

// The revision binds the entire filtered directory, not its current page. A
// changed source, preference or policy cannot mix channel positions across pages.
func (s *Store) directoryRevision(ctx context.Context, tx *sql.Tx, scope DirectoryScope, q DirectoryQuery) (string, error) {
	h := sha256.New()
	q.Offset = 0
	q.Limit = 0
	q.Revision = ""
	raw, _ := json.Marshal(q)
	fmt.Fprintf(h, "%d:%s%d:%s", len(scope.Fence), scope.Fence, len(raw), raw)
	library, _ := json.Marshal(scope.LibraryRows)
	h.Write(library)
	rows, err := tx.QueryContext(ctx, `SELECT id,revision,state,active_generation,name FROM live_sources WHERE id IN (SELECT value FROM json_each(?)) ORDER BY id`, mustJSON(scope.AllowedLiveSources))
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var id, state, generation, name string
		var revision int64
		if err = rows.Scan(&id, &revision, &state, &generation, &name); err != nil {
			rows.Close()
			return "", err
		}
		fmt.Fprintf(h, "%d:%s:%d:%d:%s:%d:%s:%d:%s", len(id), id, revision, len(state), state, len(generation), generation, len(name), name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	rows, err = tx.QueryContext(ctx, `SELECT source_id,channel_id,favorite,hidden,revision FROM live_channel_preferences WHERE authority=? AND account_id=? AND profile_id=? ORDER BY source_id,channel_id`, scope.Viewer.Authority, scope.Viewer.AccountID, scope.Viewer.ProfileID)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var source, channel string
		var favorite, hidden, revision int64
		if err = rows.Scan(&source, &channel, &favorite, &hidden, &revision); err != nil {
			rows.Close()
			return "", err
		}
		fmt.Fprintf(h, "%d:%s:%d:%s:%d:%d:%d", len(source), source, len(channel), channel, favorite, hidden, revision)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func (s *Store) directorySnapshot(ctx context.Context, a DirectoryAuthority, work func(*sql.Tx, DirectoryScope) error) error {
	if a == nil {
		return ErrDenied
	}
	gated, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return ErrUnavailable
	}
	defer gated.Rollback()
	tx := gated.Tx()
	scope, err := a(ctx, tx)
	if err != nil {
		return err
	}
	if scope.Fence == "" || len(scope.AllowedLiveSources) > MaxSources || len(scope.LibraryRows) > 64 {
		return ErrDenied
	}
	if err = work(tx, scope); err != nil {
		return err
	}
	current, err := a(ctx, tx)
	if err != nil {
		return err
	}
	if current.Fence != scope.Fence {
		return ErrDenied
	}
	if err = gated.Commit(); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Store) ChannelDirectory(ctx context.Context, a DirectoryAuthority, q DirectoryQuery) (Directory, error) {
	out := Directory{Offset: q.Offset, Limit: q.Limit, Channels: []Channel{}, Sources: []GuideSource{}}
	if !validDirectoryQuery(q) {
		return out, ErrInvalid
	}
	err := s.directorySnapshot(ctx, a, func(tx *sql.Tx, scope DirectoryScope) error {
		out.ViewerFence = scope.Fence
		var err error
		out.Revision, err = s.directoryRevision(ctx, tx, scope, q)
		if err != nil {
			return err
		}
		if q.Revision != "" && q.Revision != out.Revision {
			return ErrCursor
		}
		args := directoryArgs(scope, q)
		if len(scope.LibraryRows) > 0 {
			var distinct int
			if err = tx.QueryRowContext(ctx, directoryEligibleSQL+`SELECT count(*),count(DISTINCT channel_id) FROM eligible`, args...).Scan(&out.Total, &distinct); err != nil {
				return err
			}
			if distinct != out.Total {
				return ErrChannelIdentifierConflict
			}
		} else if err = tx.QueryRowContext(ctx, directoryEligibleSQL+`SELECT count(*) FROM eligible`, args...).Scan(&out.Total); err != nil {
			return err
		}
		order := directoryOrder(q.Sort)
		if q.Direction != "" {
			anchor, neighbor, err := directoryNeighborOffset(ctx, tx, scope, q, order)
			if err != nil {
				return err
			}
			out.Offset = anchor
			if neighbor < 0 {
				return nil
			}
			out.Offset = neighbor
		}

		rows, err := tx.QueryContext(ctx, directoryEligibleSQL+`SELECT channel_id,source_id,provenance,name,number,group_name,generation,logo_path,favorite,hidden,preference_revision,source_name,published_at,available_start,available_end,refresh_state FROM eligible ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, q.Limit, out.Offset)...)
		if err != nil {
			return err
		}
		sources := map[string]bool{}
		for rows.Next() {
			var c Channel
			var src GuideSource
			if err = rows.Scan(&c.ID, &c.SourceID, &c.Provenance, &c.Name, &c.Number, &c.Group, &c.Generation, &c.LogoPath, &c.Favorite, &c.Hidden, &c.PreferenceRevision, &src.Name, &src.PublishedAt, &src.AvailableStart, &src.AvailableEnd, &src.RefreshState); err != nil {
				rows.Close()
				return err
			}
			c.Programmes = []Programme{}
			c.TuneUnavailableReason = "delivery-unavailable"
			c.RecordUnavailableReason = "recording-unavailable"
			if c.Provenance == LibraryChannel {
				c.RecordUnavailableReason = "not-recordable"
			}
			out.Channels = append(out.Channels, c)
			sourceKey := string(c.Provenance) + "\x00" + c.SourceID
			if !sources[sourceKey] {
				src.ID = c.SourceID
				src.Generation = c.Generation
				src.Provenance = c.Provenance
				out.Sources = append(out.Sources, src)
				sources[sourceKey] = true
			}
		}
		err = rows.Err()
		rows.Close()
		return err
	})
	return out, err
}

func (s *Store) ChannelSourceSummary(ctx context.Context, a DirectoryAuthority, includeHidden bool, now time.Time) (DirectorySummary, error) {
	out := DirectorySummary{Sources: []SourceSummary{}}
	q := DirectoryQuery{Kind: "all", Sort: "server", Limit: 50, IncludeHidden: includeHidden}
	err := s.directorySnapshot(ctx, a, func(tx *sql.Tx, scope DirectoryScope) error {
		out.ViewerFence = scope.Fence
		var err error
		out.Revision, err = s.directoryRevision(ctx, tx, scope, q)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, directoryEligibleSQL+`SELECT provenance,CASE WHEN provenance='library-channel' THEN 'library' ELSE source_id END,CASE WHEN provenance='library-channel' THEN 'Library Channels' ELSE source_name END,min(generation),min(published_at),min(NULLIF(available_start,'')),max(NULLIF(available_end,'')),CASE WHEN sum(refresh_state IN ('degraded','credentials-required','stale','source-failure'))>0 THEN 'degraded' WHEN sum(refresh_state='refreshing')>0 THEN 'refreshing' ELSE min(refresh_state) END,count(*),sum(favorite),json_group_array(group_name)
  FROM eligible GROUP BY provenance,CASE WHEN provenance='library-channel' THEN 'library' ELSE source_id END ORDER BY CASE provenance WHEN 'live-source' THEN 0 ELSE 1 END,source_name,source_id`, directoryArgs(scope, q)...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var v SourceSummary
			var groups string
			var start, end sql.NullString
			if err = rows.Scan(&v.Provenance, &v.ID, &v.Name, &v.Generation, &v.PublishedAt, &start, &end, &v.RefreshState, &v.ChannelCount, &v.FavoriteCount, &groups); err != nil {
				rows.Close()
				return err
			}
			v.AvailableStart = start.String
			v.AvailableEnd = end.String
			v.Position = len(out.Sources)
			v.Groups = []string{}
			v.GroupCounts = []GuideGroupCount{}
			v.UnavailableReasons = map[string]int{}
			counts := map[string]int{}
			var names []string
			if err = json.Unmarshal([]byte(groups), &names); err != nil {
				rows.Close()
				return err
			}
			for _, name := range names {
				if name != "" {
					counts[name]++
				}
			}
			for name, count := range counts {
				v.GroupCounts = append(v.GroupCounts, GuideGroupCount{name, count})
			}
			sortSummaryGroups(v.GroupCounts)
			for _, g := range v.GroupCounts {
				v.Groups = append(v.Groups, g.Name)
			}
			v.GuideDays = GuideDays([]GuideSource{v.GuideSource}, now)
			out.Sources = append(out.Sources, v)
		}
		err = rows.Err()
		rows.Close()
		return err
	})
	return out, err
}
func sortSummaryGroups(groups []GuideGroupCount) {
	c := directoryCollators.Get().(*collate.Collator)
	defer directoryCollators.Put(c)
	sort.Slice(groups, func(i, j int) bool { return c.CompareString(groups[i].Name, groups[j].Name) < 0 })
}

func directoryOrder(sort string) string {
	order := `CASE provenance WHEN 'live-source' THEN 0 ELSE 1 END,source_name,source_id,channel_position,channel_id`
	numeric := `(portico_channel_number(number) IS NULL),portico_channel_number(number),name COLLATE portico_guide_name,channel_id`
	if sort == "number" {
		return `CASE provenance WHEN 'live-source' THEN 0 ELSE 1 END,CASE WHEN provenance='live-source' THEN source_name ELSE '' END,CASE WHEN provenance='live-source' THEN source_id ELSE '' END,` + numeric
	}
	if sort == "name" {
		return `name COLLATE portico_guide_name,` + numeric
	}
	return order
}

// The anchor is found before excluding unavailable rows. A channel that lost
// tune availability still steps to its actual neighbor in the current view.
// Only metadata is ranked; no full lineup is allocated or sent to the client.
func directoryNeighborOffset(ctx context.Context, tx *sql.Tx, scope DirectoryScope, q DirectoryQuery, order string) (int, int, error) {
	ranked := directoryEligibleSQL + `, ranked AS (SELECT channel_id,source_id,provenance,generation,row_number() OVER (ORDER BY ` + order + `)-1 ordinal FROM eligible) `
	args := directoryArgs(scope, q)
	anchor := -1
	err := tx.QueryRowContext(ctx, ranked+`SELECT ordinal FROM ranked WHERE channel_id=? AND source_id=? AND provenance=?`, append(args, q.AnchorChannelID, q.AnchorSourceID, q.AnchorProvenance)...).Scan(&anchor)
	if err == sql.ErrNoRows {
		return -1, -1, ErrDirectoryAnchorNotFound
	}
	if err != nil {
		return -1, -1, err
	}
	if !q.DeliveryAvailable {
		return anchor, -1, nil
	}
	comparison, sortDirection := ">", "ASC"
	if q.Direction == "previous" {
		comparison, sortDirection = "<", "DESC"
	}
	neighbor := -1
	err = tx.QueryRowContext(ctx, ranked+`SELECT ordinal FROM ranked WHERE generation<>'' AND NOT(channel_id=? AND source_id=? AND provenance=?) ORDER BY CASE WHEN ordinal`+comparison+`? THEN 0 ELSE 1 END,ordinal `+sortDirection+` LIMIT 1`, append(args, q.AnchorChannelID, q.AnchorSourceID, q.AnchorProvenance, anchor)...).Scan(&neighbor)
	if err == sql.ErrNoRows {
		return anchor, -1, nil
	}
	return anchor, neighbor, err
}
