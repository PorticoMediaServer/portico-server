package catalog

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/operations"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Home rows are a server-owned composition. Clients receive membership,
// ordering, policy and paging shape; they never rank or hide rows themselves.
// Every row is expressed as one candidate projection (id, ord) so counting,
// keyset paging and anchored ranges share a single bounded implementation.

const (
	HomeRowDefaultLimit = 12
	HomeRowMaxLimit     = 60
	homeCursorLifetime  = 30 * time.Minute
)

var ErrHomeRowUnknown = errors.New("that home row does not exist for this viewer")
var ErrHomeLayoutRow = errors.New("home layout refers to a row that does not exist")

type HomeRow struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	TitleText          ServerText `json:"titleText"`
	Kind               string     `json:"kind"`
	ArtworkShape       string     `json:"artworkShape"`
	Explanation        string     `json:"explanation,omitempty"`
	Endpoint           string     `json:"endpoint"`
	LibraryID          string     `json:"libraryId,omitempty"`
	PrivacySensitivity string     `json:"privacySensitivity"`
	PolicyState        string     `json:"policyState"`
	Relation           string     `json:"relation,omitempty"`
	// Family groups rows the layout arranges as one ("for_you": the personal
	// rows, which vary with the profile and the day).
	Family          string          `json:"family,omitempty"`
	Provider        string          `json:"provider,omitempty"`
	EvidenceID      string          `json:"evidenceId,omitempty"`
	Priority        int             `json:"priority"`
	CacheTTLSeconds int             `json:"cacheTtlSeconds"`
	Required        bool            `json:"required"`
	Hideable        bool            `json:"hideable"`
	Reorderable     bool            `json:"reorderable"`
	Critical        bool            `json:"critical"`
	CursorCapable   bool            `json:"cursorCapable"`
	Entries         []ContentEntry  `json:"entries"`
	Total           int             `json:"total"`
	Start           int             `json:"start"`
	Limit           int             `json:"limit"`
	HasMore         bool            `json:"hasMore"`
	NextCursor      string          `json:"nextCursor"`
	AnchorID        string          `json:"anchorId,omitempty"`
	Revision        ContentRevision `json:"revision"`
	// lead is the first entry's own order value: for the Continue rows, when it
	// was last played. Home compares the two to choose its hero.
	lead string
}

// HomeHero names the entry Home opens with: the first entry of one of its rows.
type HomeHero struct {
	RowID   string `json:"rowId"`
	EntryID string `json:"entryId"`
}

type HomeLayout struct {
	Revision     int64    `json:"revision"`
	RowOrder     []string `json:"rowOrder"`
	HiddenRowIDs []string `json:"hiddenRowIds"`
}

type HomeDocument struct {
	ServerID    string    `json:"serverId"`
	ViewerFence string    `json:"viewerFence"`
	Rows        []HomeRow `json:"rows"`
	// Hero is what the viewer was last in the middle of: the newer of the first
	// Continue Watching and the first Continue Listening entry. Absent when
	// neither row is on this Home.
	Hero        *HomeHero       `json:"hero,omitempty"`
	Layout      HomeLayout      `json:"layout"`
	Revision    ContentRevision `json:"revision"`
	GeneratedAt string          `json:"generatedAt"`
}

// HomeRowPage is the paging intent for one row. Cursor and start are exclusive;
// anchorId re-resolves start against the live projection after a publication.
type HomeRowPage struct {
	Cursor   string
	Revision string
	AnchorID string
	Limit    int
	Start    int
}

type homeRowSpec struct {
	ID           string
	Title        string
	TitleCode    string // the catalogue id, when not the fixed rows' own
	Family       string
	TitleParams  map[string]string
	Kind         string
	ArtworkShape string
	Explanation  string
	LibraryID    string
	Privacy      string
	PolicyState  string
	Direction    string
	Priority     int
	// Rank orders rows of one priority (the libraries' recent shelves, in library order).
	Rank        int
	CacheTTL    int
	Required    bool
	Hideable    bool
	Reorderable bool
	Critical    bool
	Cursors     bool
	// DefaultHidden rows stay off Home until the viewer places them: a
	// layout shows one only when it orders the row and does not hide it.
	DefaultHidden bool
}

// hiddenIn says whether a layout hides the row.
func (spec homeRowSpec) hiddenIn(order []string, hidden map[string]bool) bool {
	if spec.Required || !spec.Hideable {
		return false
	}
	if hidden[spec.ID] {
		return true
	}
	if !spec.DefaultHidden {
		return false
	}
	for _, id := range order {
		if id == spec.ID {
			return false
		}
	}
	return true
}

// The recent shelves: one per library ("Recently added in Movies"). Libraries
// are never mixed in one shelf: a row is posters or squares, not both.
const homeRecentPrefix = "recent_"

type homeSource struct {
	base string
	args []any
	cap  int
	// total is the row's exact count when a maintained counter can answer it, so
	// the composition does not run a `count(*)` over the whole projection for a
	// number the client renders. Nil means "count the projection".
	total *int
	// selfRestricted marks a projection whose SQL already applies the viewer's
	// restriction itself, so the generic item wrap must not run again.
	selfRestricted bool
	fingerprint    string
}

func (spec homeRowSpec) descriptor() HomeRow {
	return HomeRow{ID: spec.ID, Title: spec.Title, TitleText: spec.titleText(), Kind: spec.Kind, ArtworkShape: spec.ArtworkShape, Explanation: spec.Explanation,
		Endpoint: "/v1/home/rows/" + spec.ID, LibraryID: spec.LibraryID, PrivacySensitivity: spec.Privacy, PolicyState: spec.PolicyState,
		Priority: spec.Priority, CacheTTLSeconds: spec.CacheTTL, Required: spec.Required, Hideable: spec.Hideable,
		Reorderable: spec.Reorderable, Critical: spec.Critical, CursorCapable: spec.Cursors, Family: spec.Family, Entries: []ContentEntry{}}
}

// homeRowTitleCodes are the client catalogue ids for the fixed Home rows.
var homeRowTitleCodes = map[string]string{
	"continue":           "home.row.continueWatching",
	"continue_listening": "home.row.continueListening",
	"recommended":        "home.row.recommended",
	"trending_now":       "home.row.trending",
	"community_watching": "home.row.popularOnServer",
	recFamily:            "home.row.moreRecommendations",
}

// titleText is the row title as {code, params, fallback} (CON-19).
func (spec homeRowSpec) titleText() ServerText {
	code := homeRowTitleCodes[spec.ID]
	if spec.TitleCode != "" {
		code = spec.TitleCode
	}
	if code == "" && strings.HasPrefix(spec.ID, homeRecentPrefix) {
		code = "home.row.recentlyAdded"
		if spec.TitleParams["library"] != "" {
			code = "home.row.recentlyAddedIn"
		}
	}
	return ServerText{Code: code, Params: spec.TitleParams, Fallback: spec.Title}
}

func homeLimit(limit int) int {
	if limit <= 0 {
		return HomeRowDefaultLimit
	}
	if limit > HomeRowMaxLimit {
		return HomeRowMaxLimit
	}
	return limit
}

func homeUnique(ids []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// A personal row is a list of what one person has done, and it is bounded by
// that person's history rather than by the library. Every one of them is written
// as a CROSS JOIN from the personal table outwards, which in SQLite is not a
// different join — it is the same join with the order fixed.
//
// The order has to be fixed because the planner gets it wrong here, and gets it
// wrong only under a restriction. `i.library_id IN (SELECT value FROM
// json_each(?))` looks selective and countable to it while `pi.profile_id=?` has
// no statistics, so when the viewer's restriction wraps the projection the
// planner flips the loops and reads every item in the library, probing the
// personal table for each. Measured on the release catalogue, the watchlist row
// for a restricted viewer: 2.02 seconds returning no rows, and 135 microseconds
// with the order fixed.
//
// --- candidate projections -------------------------------------------------

// Visibility is read from the maintained projection rather than recomputed.
//
// `inventory_item_availability` is a view with four correlated `EXISTS` inside a
// three-table join — about seven B-tree probes for every candidate row it is
// asked about — and every surface in this package asked it about every row it
// considered, including the `count(*)` next to the page. `item_visibility`
// (internal/persistence/item_visibility.go) holds the same value, maintained by
// trigger from the same five inputs, so the substitution is a change of where
// the answer comes from and not of what the answer is. A reconciler diffs the
// two continuously and a test asserts they agree.
const homeAvailable = `EXISTS(SELECT 1 FROM catalog_item_availability v WHERE v.entity_id=%s AND v.available=1)`
const homeRetired = `NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=%s)`

// homeVisible requires a live entity and its available, non-DVR-retired item
// projection.
func homeVisible(alias string) string {
	return fmt.Sprintf(`EXISTS(SELECT 1 FROM catalog_entities e JOIN catalog_item_availability v ON v.entity_id=e.id WHERE e.id=%s AND e.retired=0 AND v.available=1 AND v.retired=0)`, alias)
}

// continueListeningMaximum bounds Continue Listening the way the owner maximum
// bounds Continue Watching: the row, its count and its pages never read past
// this many entries, however long the profile's listening history is.
const continueListeningMaximum = 40

func homeContinueBase(mode string) string {
	kind := `k.playable=1 AND k.listening=0 AND i.kind<>11`
	libraries := `a.library_id IN(SELECT value FROM json_each(?))`
	activity := `progress_activity a`
	if mode == "continue_movies" {
		kind = `i.kind=1`
		libraries = `a.library_id IN(SELECT l.id FROM libraries l WHERE l.kind='movie' AND l.id IN(SELECT value FROM json_each(?)))`
	}
	if mode == "continue_listening" {
		kind = `(k.playable=1 AND k.listening=1 AND (i.kind<>9 OR EXISTS(SELECT 1 FROM catalog_book_files f JOIN book_resume b ON b.book_id=f.book_id AND b.profile_id=a.profile_id WHERE f.entity_id=i.id AND b.item_id=i.id)))`
		// One ordered walk of the profile's unfinished activity across its music
		// and audiobook libraries (0079). The per-library index would make SQLite
		// sort the whole history before the row's LIMIT could stop it.
		activity = `progress_activity a INDEXED BY progress_home_profile_recent`
	}
	// id names the public id for hydration, entity the integer id for
	// restriction predicates, ord the activity row's own column so the planner
	// can take the row order straight from the index and stop at the row's limit.
	// Progress positions are milliseconds (unit 0); asset durations are seconds.
	return `SELECT pid(i.public_id) AS id,a.item_id AS entity,a.updated_at AS ord FROM ` + activity + ` CROSS JOIN progress p ON p.profile_id=a.profile_id AND p.item_id=a.item_id JOIN catalog_entities i ON i.id=a.item_id JOIN catalog_kinds k ON k.id=i.kind` +
		` WHERE a.profile_id=? AND ` + libraries + ` AND i.retired=0 AND EXISTS(SELECT 1 FROM catalog_libraries cl WHERE cl.id=i.library_id AND cl.library_id=a.library_id) AND ` + kind +
		` AND ` + fmt.Sprintf(homeRetired, "i.id") + ` AND a.state!='ended' AND NOT EXISTS(SELECT 1 FROM continue_dismissals d WHERE d.profile_id=p.profile_id AND d.item_id=p.item_id AND d.playback_id=p.playback_id) AND p.position>0` +
		` AND p.position<(SELECT max(s.duration)*1000-CASE WHEN i.kind=9 THEN 0 ELSE 3000 END FROM catalog_asset_links l JOIN catalog_assets s ON s.id=l.asset_id WHERE l.entity_id=i.id AND s.available=1 AND l.available=1)`
}

// homeSource wraps every row's candidate projection in the viewer's content
// restriction. Wrapping the finished projection rather than editing each of the
// nine bases means the row count, the anchor rank and the keyset page all see the
// same rows, and a new row type cannot forget the predicate.
func (s *Service) homeSource(r HomeRequest, spec homeRowSpec) (homeSource, error) {
	src, err := s.homeSourceBase(r, spec)
	if err != nil {
		return src, err
	}
	if !src.selfRestricted {
		// Candidate ids are public ids; the predicate takes integer entity ids.
		restriction, restrictionArgs := ItemRestrictionSQL("(SELECT id FROM catalog_entities WHERE public_id=pid_blob(candidate.id))", r.Restrictions)
		if restriction != "1" {
			src.base = `SELECT candidate.id,candidate.ord FROM (` + src.base + `) candidate WHERE ` + restriction
			src.args = append(src.args, restrictionArgs...)
		}
	}
	if src.cap > 0 {
		src.base = `SELECT id,ord FROM (` + src.base + `) ORDER BY ord DESC,id DESC LIMIT ?`
		src.args = append(src.args, src.cap)
	}
	return src, nil
}

func (s *Service) homeSourceBase(r HomeRequest, spec homeRowSpec) (homeSource, error) {
	raw, _ := json.Marshal(r.Libraries)
	libraries := string(raw)
	switch {
	case spec.ID == "continue" || spec.ID == "continue_listening":
		mode := "continue_movies"
		if spec.ID == "continue_listening" {
			mode = "continue_listening"
			return homeSource{base: homeContinueBase(mode), args: []any{r.Profile, libraries}, cap: continueListeningMaximum}, nil
		}
		cutoffs, premieres, maximum, err := s.continueWatchingPolicy(r.Libraries, r.now())
		if err != nil {
			return homeSource{}, err
		}
		return s.homeContinueSource(r, libraries, cutoffs, premieres, maximum)
	case spec.ID == "recommended" || spec.ID == "trending_now" || spec.ID == "community_watching" || strings.HasPrefix(spec.ID, recFamily+":"):
		return s.homeEngineSource(r, spec)
	case strings.HasPrefix(spec.ID, homeViewPrefix):
		return s.homeViewSource(r, spec)
	case strings.HasPrefix(spec.ID, "recent_"):
		return s.homeEngineSource(r, spec)
	}
	return homeSource{}, ErrHomeRowUnknown
}

// --- row catalogue ---------------------------------------------------------

func homeArtwork(kind string) string {
	if kind == "music" || kind == "audiobook" {
		return "square"
	}
	return "poster"
}

func (s *Service) homeSpecs(r HomeRequest) ([]homeRowSpec, error) {
	specs := []homeRowSpec{
		{ID: "continue", Title: "Continue Watching", Kind: "continue", ArtworkShape: "poster", Priority: 10, CacheTTL: 45, Required: true, Reorderable: true, Critical: true, Cursors: true, Privacy: "personal", PolicyState: "available"},
		{ID: "continue_listening", Title: "Continue Listening", Kind: "continue", ArtworkShape: "square", Priority: 20, CacheTTL: 45, Required: true, Reorderable: true, Cursors: true, Privacy: "personal", PolicyState: "available"},
		{Direction: "asc", ID: "recommended", Title: "Recommended for you", Kind: "recommendation", ArtworkShape: "poster", Priority: 60, CacheTTL: 300, Hideable: true, Reorderable: true, Cursors: true, Privacy: "personal", PolicyState: "available"},
		// The personal rows, arranged as one: Home expands it into the
		// strongest few of the profile's generated rows (rec_generators.go).
		{Direction: "asc", ID: recFamily, Title: "More recommendations", Kind: "family", Family: recFamily, ArtworkShape: "poster", Priority: 65, CacheTTL: 300, Hideable: true, Reorderable: true, Privacy: "personal", PolicyState: "available"},
		{Direction: "asc", ID: "trending_now", Title: "Trending now", Kind: "recommendation", ArtworkShape: "poster", Priority: 70, CacheTTL: 300, Hideable: true, Reorderable: true, Cursors: true, Privacy: "catalog", PolicyState: "available"},
		{Direction: "asc", ID: "community_watching", Title: "Popular on this server", Kind: "community", ArtworkShape: "poster", Priority: 80, CacheTTL: 300, Hideable: true, Reorderable: true, Cursors: true, Privacy: "aggregated", PolicyState: "available"},
	}
	if !r.CommunityActivity {
		for index := range specs {
			if specs[index].ID == "community_watching" {
				specs[index].PolicyState = "disabled_by_owner"
			}
		}
	}
	specs = append(specs, s.homeViewSpecs(r)...)
	libraries, e := s.homeLibraries(r.Libraries)
	if e != nil {
		return nil, e
	}
	for index, library := range libraries {
		specs = append(specs, homeRowSpec{Direction: "desc", ID: "recent_" + library.ID, Title: "Recently added in " + library.Name, TitleParams: map[string]string{"library": library.Name}, Kind: "recent",
			// Straight after what the viewer is in the middle of, in the owner's library order.
			ArtworkShape: homeArtwork(library.Kind), LibraryID: library.ID, Priority: 30, Rank: index, CacheTTL: 120,
			Hideable: true, Reorderable: true, Cursors: true, Privacy: "catalog", PolicyState: "available"})
	}
	return specs, nil
}

func (s *Service) homeLibraries(ids []string) ([]Library, error) {
	out := []Library{}
	if len(ids) == 0 {
		return out, nil
	}
	raw, _ := json.Marshal(ids)
	rows, e := s.read().Query(`SELECT id,name,kind FROM libraries WHERE id IN(SELECT value FROM json_each(?)) ORDER BY position,name,id`, string(raw))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var library Library
		if e = rows.Scan(&library.ID, &library.Name, &library.Kind); e != nil {
			return nil, e
		}
		library.DefaultView = DefaultLibraryView(library.Kind)
		out = append(out, library)
	}
	return out, rows.Err()
}

// --- paging ----------------------------------------------------------------

type homeCursor struct {
	Fingerprint string `json:"fingerprint,omitempty"`
	Row         string `json:"row"`
	Profile     string `json:"profile"`
	Viewer      string `json:"viewer"`
	Direction   string `json:"direction"`
	Value       string `json:"value"`
	ID          string `json:"id"`
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
	Catalog     int64  `json:"catalog"`
	Revision    int64  `json:"revision"`
	Expires     int64  `json:"expires"`
}

func (s *Service) encodeHomeCursor(value homeCursor) (string, error) {
	key, e := s.cursorKey()
	if e != nil {
		return "", e
	}
	raw, e := json.Marshal(value)
	if e != nil {
		return "", e
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Service) decodeHomeCursor(raw string, expected homeCursor, revision ContentRevision) (homeCursor, error) {
	var out homeCursor
	if len(raw) > 4096 {
		return out, ErrCursor
	}
	payload, signature, ok := strings.Cut(raw, ".")
	if !ok {
		return out, ErrCursor
	}
	data, e := base64.RawURLEncoding.DecodeString(payload)
	if e != nil {
		return out, ErrCursor
	}
	sig, e := base64.RawURLEncoding.DecodeString(signature)
	if e != nil {
		return out, ErrCursor
	}
	key, e := s.cursorKey()
	if e != nil {
		return out, e
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	if !hmac.Equal(sig, mac.Sum(nil)) || json.Unmarshal(data, &out) != nil {
		return out, ErrCursor
	}
	if out.Row != expected.Row || out.Profile != expected.Profile || out.Viewer != expected.Viewer || out.Direction != expected.Direction || out.Limit != expected.Limit || out.Expires < time.Now().Unix() {
		return out, ErrCursor
	}
	if out.Catalog != revision.Catalog || out.Revision != revision.Viewer || out.Fingerprint != expected.Fingerprint {
		return out, ErrStaleContinuation
	}
	return out, nil
}

func homeOrder(direction string) (string, string) {
	if direction == "asc" {
		return "ASC", ">"
	}
	return "DESC", "<"
}

func (s *Service) homeRowIDs(spec homeRowSpec, src homeSource, page HomeRowPage, revision ContentRevision, viewer string, profile string) (ids []string, total, start int, next, lead string, err error) {
	order, operator := homeOrder(spec.Direction)
	limit := homeLimit(page.Limit)
	start = page.Start
	if start < 0 {
		return nil, 0, 0, "", "", ErrCursor
	}
	// The count is asked for only when the page cannot answer it.
	//
	// Every row's total is exact and appears in the JSON, and it used to be a
	// second execution of the row's whole projection — which for the personal
	// rows is the expensive half: the up-next row's projection alone measured
	// 11.5 ms, and it ran twice. A first page that comes back short has counted
	// the row by reading it, and a short first page is what these rows almost
	// always produce.
	countTotal := func() (int, error) {
		if src.total != nil {
			return *src.total, nil
		}
		var total int
		e := s.read().QueryRow(`SELECT count(*) FROM (`+src.base+`)`, src.args...).Scan(&total)
		return total, e
	}
	var e error
	if page.Cursor != "" || page.AnchorID != "" || start != 0 || src.total != nil {
		if total, e = countTotal(); e != nil {
			return nil, 0, 0, "", "", e
		}
	}
	base := homeCursor{Fingerprint: src.fingerprint, Row: spec.ID, Profile: profile, Viewer: viewer, Direction: spec.Direction, Limit: limit, Catalog: revision.Catalog, Revision: revision.Viewer}
	if page.Cursor != "" {
		if page.Start != 0 || page.AnchorID != "" {
			return nil, 0, 0, "", "", ErrCursor
		}
		cursor, e := s.decodeHomeCursor(page.Cursor, base, revision)
		if e != nil {
			return nil, 0, 0, "", "", e
		}
		args := append(append([]any{}, src.args...), cursor.Value, cursor.Value, cursor.ID, limit+1)
		query := `SELECT id,ord FROM (` + src.base + `) WHERE (ord` + operator + `? OR (ord=? AND id` + operator + `?)) ORDER BY ord ` + order + `,id ` + order + ` LIMIT ?`
		ids, next, _, lead, e := s.homeScanPage(query, args, base, cursor.Offset, limit)
		return ids, total, cursor.Offset, next, lead, e
	}
	if page.AnchorID != "" {
		args := append(append([]any{}, src.args...), page.AnchorID)
		var rank int
		rankErr := s.read().QueryRow(`SELECT position FROM (SELECT id,ROW_NUMBER() OVER (ORDER BY ord `+order+`,id `+order+`)-1 AS position FROM (`+src.base+`)) WHERE id=?`, args...).Scan(&rank)
		if rankErr == nil {
			start = rank
		} else if !errors.Is(rankErr, sql.ErrNoRows) {
			return nil, 0, 0, "", "", rankErr
		}
	}
	if src.total != nil || page.AnchorID != "" || start != 0 {
		if total == 0 {
			start = 0
		} else if start > total-1 {
			start = total - 1
		}
	}
	args := append(append([]any{}, src.args...), limit+1, start)
	query := `SELECT id,ord FROM (` + src.base + `) ORDER BY ord ` + order + `,id ` + order + ` LIMIT ? OFFSET ?`
	ids, next, scanned, lead, e := s.homeScanPage(query, args, base, start, limit)
	if e != nil {
		return nil, 0, 0, "", "", e
	}
	if src.total == nil && page.AnchorID == "" && start == 0 {
		if scanned <= limit {
			// The page is the whole row: it asked for one more than it wanted and
			// did not get it, so the count is what it read.
			total = scanned
		} else if total, e = countTotal(); e != nil {
			return nil, 0, 0, "", "", e
		}
	}
	return ids, total, start, next, lead, nil
}

// homeScanPage reads one page. lead is the first entry's order value.
func (s *Service) homeScanPage(query string, args []any, base homeCursor, offset, limit int) (ids []string, next string, scanned int, lead string, e error) {
	rows, e := s.read().Query(query, args...)
	if e != nil {
		return nil, "", 0, "", e
	}
	ids = []string{}
	orders := []string{}
	for rows.Next() {
		var id, ord string
		if e = rows.Scan(&id, &ord); e != nil {
			rows.Close()
			return nil, "", 0, "", e
		}
		ids = append(ids, id)
		orders = append(orders, ord)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, "", 0, "", e
	}
	scanned = len(ids)
	if len(orders) > 0 {
		lead = orders[0]
	}
	if len(ids) > limit {
		ids = ids[:limit]
		orders = orders[:limit]
		cursor := base
		cursor.Value = orders[len(orders)-1]
		cursor.ID = ids[len(ids)-1]
		cursor.Offset = offset + limit
		cursor.Expires = time.Now().Add(homeCursorLifetime).Unix()
		if next, e = s.encodeHomeCursor(cursor); e != nil {
			return nil, "", 0, "", e
		}
	}
	return ids, next, scanned, lead, nil
}

func (s *Service) homeEntries(profile string, ids []string) ([]ContentEntry, error) {
	entries := []ContentEntry{}
	if len(ids) == 0 {
		return entries, nil
	}
	items, e := s.mediaPage(profile, ids, false)
	if e != nil {
		return nil, e
	}
	loaded := map[string]ContentEntry{}
	for _, item := range items {
		row := contentItem(item)
		if !item.Available {
			row.Playback = nil
		}
		loaded[item.ID] = row
	}
	for _, id := range ids {
		row, ok := loaded[id]
		if !ok {
			return nil, ErrStaleContinuation
		}
		entries = append(entries, row)
	}
	if e = s.nameEntryMakers(entries); e != nil {
		return nil, e
	}
	return entries, nil
}

// --- public composition ----------------------------------------------------

func (r HomeRequest) now() time.Time {
	if r.Now.IsZero() {
		return time.Now()
	}
	return r.Now
}

func (s *Service) homeRowFor(r HomeRequest, spec homeRowSpec, revision ContentRevision, page HomeRowPage) (HomeRow, error) {
	row := spec.descriptor()
	row.Revision = revision
	row.Limit = homeLimit(page.Limit)
	src, e := s.homeSource(r, spec)
	if e != nil {
		return row, e
	}
	ids, total, start, next, lead, e := s.homeRowIDs(spec, src, page, revision, r.ViewerFence, r.Profile)
	if e != nil {
		return row, e
	}
	entries, e := s.homeRowEntries(r.Profile, spec, ids)
	if e != nil {
		return row, e
	}
	if e = s.homeEnrich(r.Profile, entries); e != nil {
		return row, e
	}
	row.Entries = entries
	row.lead = lead
	row.Total = total
	row.Start = start
	row.NextCursor = next
	row.HasMore = next != "" || start+len(entries) < total
	row.AnchorID = page.AnchorID
	return row, nil
}

// homeRowEntries hydrates one row's page: engine rows may list container works
// (shows, albums, books), every other row lists items only.
func (s *Service) homeRowEntries(profile string, spec homeRowSpec, ids []string) ([]ContentEntry, error) {
	if spec.ID == "recommended" || spec.ID == "trending_now" || spec.ID == "community_watching" || strings.HasPrefix(spec.ID, "recent_") || strings.HasPrefix(spec.ID, recFamily+":") || strings.HasPrefix(spec.ID, homeViewPrefix) {
		return s.homeEngineEntries(profile, ids)
	}
	return s.homeEntries(profile, ids)
}

func homeOrdered(specs []homeRowSpec, order []string) []homeRowSpec {
	rank := map[string]int{}
	for index, id := range order {
		if _, seen := rank[id]; !seen {
			rank[id] = index
		}
	}
	sort.SliceStable(specs, func(i, j int) bool {
		a, aOK := rank[specs[i].ID]
		b, bOK := rank[specs[j].ID]
		if aOK != bOK {
			return aOK
		}
		if aOK && a != b {
			return a < b
		}
		if specs[i].Priority != specs[j].Priority {
			return specs[i].Priority < specs[j].Priority
		}
		if specs[i].Rank != specs[j].Rank {
			return specs[i].Rank < specs[j].Rank
		}
		return specs[i].ID < specs[j].ID
	})
	return specs
}

// HomeRowCatalogue publishes the row identities and their policy without
// composing entries. Layout validation uses it so a viewer can never order or
// hide a row that this server does not publish.
func (s *Service) HomeRowCatalogue(r HomeRequest) ([]HomeRow, error) {
	r = r.scoped()
	if err := s.prepareViewer(r.Viewer); err != nil {
		return nil, err
	}
	r.RowOrder = homeCanonicalLayout(r.RowOrder)
	r.HiddenRowIDs = homeCanonicalLayout(r.HiddenRowIDs)
	specs, e := s.homeSpecs(r)
	if e != nil {
		return nil, e
	}
	out := []HomeRow{}
	for _, spec := range homeOrdered(specs, r.RowOrder) {
		out = append(out, spec.descriptor())
	}
	return out, nil
}

// HomeLayoutRow is one row a viewer can arrange (CON-23): its identity and policy, and whether
// the saved layout hides it. No entries: Customise Home lists every row, empty or not, without
// composing any of them.
type HomeLayoutRow struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	TitleText    ServerText `json:"titleText"`
	Kind         string     `json:"kind"`
	ArtworkShape string     `json:"artworkShape"`
	LibraryID    string     `json:"libraryId,omitempty"`
	Required     bool       `json:"required"`
	Hideable     bool       `json:"hideable"`
	Reorderable  bool       `json:"reorderable"`
	Hidden       bool       `json:"hidden"`
}

// HomeLayoutView is the saved layout with every row it can arrange, in the viewer's order.
type HomeLayoutView struct {
	Revision     int64           `json:"revision"`
	RowOrder     []string        `json:"rowOrder"`
	HiddenRowIDs []string        `json:"hiddenRowIds"`
	Rows         []HomeLayoutRow `json:"rows"`
}

// HomeLayoutRows lists every row the viewer could have on Home (CON-23), in their order, whether
// it has entries today or not; `GET /v1/home` omits empty rows, so Customise Home can't list from
// it. Rows the owner turned off are left out. It reads only the row definitions and the visible
// libraries: no row is composed.
func (s *Service) HomeLayoutRows(r HomeRequest) (HomeLayoutView, error) {
	r = r.scoped()
	if err := s.prepareViewer(r.Viewer); err != nil {
		return HomeLayoutView{}, err
	}
	r.RowOrder = homeCanonicalLayout(r.RowOrder)
	r.HiddenRowIDs = homeCanonicalLayout(r.HiddenRowIDs)
	r.Libraries = homeUnique(r.Libraries)
	specs, e := s.homeSpecs(r)
	if e != nil {
		return HomeLayoutView{}, e
	}
	hidden := map[string]bool{}
	for _, id := range r.HiddenRowIDs {
		hidden[id] = true
	}
	out := HomeLayoutView{Revision: r.LayoutRevision, RowOrder: append([]string{}, r.RowOrder...), HiddenRowIDs: append([]string{}, r.HiddenRowIDs...), Rows: []HomeLayoutRow{}}
	for _, spec := range homeOrdered(specs, r.RowOrder) {
		if spec.PolicyState != "available" {
			continue
		}
		out.Rows = append(out.Rows, HomeLayoutRow{ID: spec.ID, Title: spec.Title, TitleText: spec.titleText(), Kind: spec.Kind, ArtworkShape: spec.ArtworkShape, LibraryID: spec.LibraryID,
			Required: spec.Required, Hideable: spec.Hideable, Reorderable: spec.Reorderable, Hidden: spec.hiddenIn(r.RowOrder, hidden)})
	}
	return out, nil
}

// Home composes every visible row with an inline preview. Rows with no entries
// are omitted unless they are critical; hidden rows are dropped unless the row
// is required, which no layout may hide.
func (s *Service) HomeRows(r HomeRequest) (HomeDocument, error) {
	if dbwork.Snapshot(s.Context()) == nil {
		var out HomeDocument
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var readErr error
			out, readErr = s.WithContext(ctx).HomeRows(r)
			return readErr
		})
		return out, err
	}
	if err := s.compactProjectionReady(18, 19, 20, 21, 25, 26); err != nil {
		return HomeDocument{}, err
	}
	r = r.scoped()
	if err := s.prepareViewer(r.Viewer); err != nil {
		return HomeDocument{}, err
	}
	r.RowOrder = homeCanonicalLayout(r.RowOrder)
	r.HiddenRowIDs = homeCanonicalLayout(r.HiddenRowIDs)
	r.Now = s.recommendationNow(r.Now)
	s = s.WithRecommendationRestrictions(r.Restrictions)
	s.recSources = map[string]homeSource{}
	r.Libraries = homeUnique(r.Libraries)
	out := HomeDocument{ServerID: r.ServerID, ViewerFence: r.ViewerFence, Rows: []HomeRow{},
		Layout: HomeLayout{Revision: r.LayoutRevision, RowOrder: append([]string{}, r.RowOrder...), HiddenRowIDs: append([]string{}, r.HiddenRowIDs...)}}
	if out.Layout.RowOrder == nil {
		out.Layout.RowOrder = []string{}
	}
	before, e := s.homeRevision(r.Libraries, r.Profile)
	if e != nil {
		return out, e
	}
	out.Revision = before
	specs, e := s.homeSpecs(r)
	if e != nil {
		return out, e
	}
	hidden := map[string]bool{}
	for _, id := range r.HiddenRowIDs {
		hidden[id] = true
	}
	for _, spec := range homeOrdered(specs, r.RowOrder) {
		if spec.PolicyState != "available" {
			continue
		}
		if spec.hiddenIn(r.RowOrder, hidden) {
			continue
		}
		if len(out.Rows) == operations.HomeLayoutMaxRows && !spec.Required {
			continue
		}
		if spec.ID == recFamily {
			rows, e := s.recFamilyRows(r, before)
			if e != nil {
				return out, e
			}
			for _, row := range rows {
				if len(out.Rows) == operations.HomeLayoutMaxRows {
					break
				}
				out.Rows = append(out.Rows, row)
			}
			continue
		}
		row, e := s.homeRowFor(r, spec, before, HomeRowPage{Limit: r.Limit})
		if e != nil {
			return out, e
		}
		if (len(row.Entries) == 0 && !spec.Critical) || (spec.ID == "recommended" && row.Total < recMinimumShelf) || (spec.ID == "trending_now" && row.Total < recMinimumTrend) {
			continue
		}
		if len(out.Rows) == operations.HomeLayoutMaxRows {
			// A custom order may put required rows last. Preserve them by dropping
			// the last optional preview; all rows remain in the customization catalogue.
			for i := len(out.Rows) - 1; i >= 0; i-- {
				if !out.Rows[i].Required {
					out.Rows = append(out.Rows[:i], out.Rows[i+1:]...)
					break
				}
			}
		}
		out.Rows = append(out.Rows, row)
	}
	after, e := s.homeRevision(r.Libraries, r.Profile)
	if e != nil {
		return out, e
	}
	if before != after {
		return out, ErrStaleContinuation
	}
	out.Hero = homeHero(out.Rows)
	out.GeneratedAt = r.now().UTC().Format("2006-01-02T15:04:05.000Z")
	return out, nil
}

// homeHero is the entry Home opens with: whatever the viewer was last in the
// middle of. Continue Watching and Continue Listening are each ordered by when
// their entries were last played, so the hero is the newer of the two rows'
// first entries — an unfinished title, or the next episode of the show last
// watched, by the rows' own rules. A row the viewer has hidden offers nothing.
func homeHero(rows []HomeRow) *HomeHero {
	var hero *HomeHero
	newest := ""
	for i := range rows {
		row := &rows[i]
		if (row.ID != "continue" && row.ID != "continue_listening") || len(row.Entries) == 0 {
			continue
		}
		if hero == nil || row.lead > newest {
			hero, newest = &HomeHero{RowID: row.ID, EntryID: row.Entries[0].ID}, row.lead
		}
	}
	return hero
}

// HomeSingleRow pages one row. A supplied revision that no longer matches the
// live projection is a stale continuation, not a silently different page.
func (s *Service) HomeSingleRow(r HomeRequest, id string, page HomeRowPage) (HomeRow, error) {
	if dbwork.Snapshot(s.Context()) == nil {
		var out HomeRow
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var readErr error
			out, readErr = s.WithContext(ctx).HomeSingleRow(r, id, page)
			return readErr
		})
		return out, err
	}
	if err := s.compactProjectionReady(18, 19, 20, 21, 25, 26); err != nil {
		return HomeRow{}, err
	}
	r = r.scoped()
	if err := s.prepareViewer(r.Viewer); err != nil {
		return HomeRow{}, err
	}
	id = homeCanonicalRow(id)
	r.RowOrder = homeCanonicalLayout(r.RowOrder)
	r.HiddenRowIDs = homeCanonicalLayout(r.HiddenRowIDs)
	r.Now = s.recommendationNow(r.Now)
	s = s.WithRecommendationRestrictions(r.Restrictions)
	s.recSources = map[string]homeSource{}
	r.Libraries = homeUnique(r.Libraries)
	specs, e := s.homeSpecs(r)
	if e != nil {
		return HomeRow{}, e
	}
	var found *homeRowSpec
	for index := range specs {
		if specs[index].ID == id && id != recFamily {
			found = &specs[index]
		}
	}
	if strings.HasPrefix(id, recFamily+":") && !hidesFamily(r.HiddenRowIDs) {
		row, e := s.recPersonalRow(r, id)
		if e != nil {
			return HomeRow{}, e
		}
		s.recSources[id+":"+idsJSON(r.Libraries)] = candidateSource(row.items)
		found = &row.spec
	}
	if found == nil || found.PolicyState != "available" {
		return HomeRow{}, ErrHomeRowUnknown
	}
	before, e := s.homeRevision(r.Libraries, r.Profile)
	if e != nil {
		return HomeRow{}, e
	}
	if page.Revision != "" && page.Revision != fmt.Sprintf("%d:%d", before.Catalog, before.Viewer) {
		return HomeRow{}, ErrStaleContinuation
	}
	row, e := s.homeRowFor(r, *found, before, page)
	if e != nil {
		return row, e
	}
	after, e := s.homeRevision(r.Libraries, r.Profile)
	if e != nil {
		return row, e
	}
	if before != after {
		return row, ErrStaleContinuation
	}
	return row, nil
}

// recFamilyRows composes the personal rows Home shows today, each a first page
// of its own ranking (already deduplicated across the family).
func (s *Service) recFamilyRows(r HomeRequest, revision ContentRevision) ([]HomeRow, error) {
	generated, err := s.recPersonalRows(r)
	if err != nil {
		return nil, err
	}
	out := make([]HomeRow, 0, len(generated))
	for _, g := range generated {
		s.recSources[g.spec.ID+":"+idsJSON(r.Libraries)] = candidateSource(g.items)
		row, err := s.homeRowFor(r, g.spec, revision, HomeRowPage{Limit: r.Limit})
		if err != nil {
			return nil, err
		}
		if len(row.Entries) > 0 {
			out = append(out, row)
		}
	}
	return out, nil
}

func hidesFamily(hidden []string) bool {
	for _, id := range hidden {
		if id == recFamily {
			return true
		}
	}
	return false
}

// ValidateHomeLayout refuses orders and hidden sets that name rows this server
// does not publish, and refuses to hide a required row by name.
func (s *Service) ValidateHomeLayout(r HomeRequest, order, hidden []string) error {
	order = homeCanonicalLayout(order)
	hidden = homeCanonicalLayout(hidden)
	r.RowOrder = order
	specs, e := s.homeSpecs(r)
	if e != nil {
		return e
	}
	known := map[string]homeRowSpec{}
	for _, spec := range specs {
		known[spec.ID] = spec
	}
	if len(order) > operations.HomeLayoutMaxRows || len(hidden) > operations.HomeLayoutMaxRows {
		return fmt.Errorf("%w: too many rows", ErrHomeLayoutRow)
	}
	seen := map[string]bool{}
	for _, id := range order {
		spec, ok := known[id]
		if !ok && strings.HasPrefix(id, homeViewPrefix) && utf8.RuneCountInString(id) <= operations.HomeLayoutRowIDMaxLength {
			// A saved view the viewer can't read (deleted, unshared) is kept
			// and ignored: refusing it would block every later layout save.
			seen[id] = true
			continue
		}
		if !ok || utf8.RuneCountInString(id) > operations.HomeLayoutRowIDMaxLength {
			return fmt.Errorf("%w: %s", ErrHomeLayoutRow, id)
		}
		if seen[id] {
			return fmt.Errorf("%w: %s is listed twice", ErrHomeLayoutRow, id)
		}
		seen[id] = true
		if !spec.Reorderable {
			return fmt.Errorf("%w: %s cannot be reordered", ErrHomeLayoutRow, id)
		}
	}
	seen = map[string]bool{}
	for _, id := range hidden {
		spec, ok := known[id]
		if !ok || utf8.RuneCountInString(id) > operations.HomeLayoutRowIDMaxLength {
			return fmt.Errorf("%w: %s", ErrHomeLayoutRow, id)
		}
		if seen[id] {
			return fmt.Errorf("%w: %s is listed twice", ErrHomeLayoutRow, id)
		}
		seen[id] = true
		if spec.Required || !spec.Hideable {
			return fmt.Errorf("%w: %s is required and cannot be hidden", ErrHomeLayoutRow, id)
		}
	}
	return nil
}

// The old "trending" ID represented server activity. Preserve saved layouts
// while giving the provider feed its own identity.
func homeCanonicalRow(id string) string {
	if id == "trending" {
		return "community_watching"
	}
	return id
}
func homeCanonicalLayout(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		// Rows Home no longer has: "featured" (the hero became what the viewer was
		// last in the middle of), "recent_all" (Recently added is per library),
		// "watchlist" and "favorites" (My List and Favorites are Saved's tabs, not
		// shelves; Justin, 2–3 Oct 2026). A layout saved before then still names
		// them; they are dropped here so it keeps saving.
		if id == "featured" || id == "recent_all" || id == "watchlist" || id == "favorites" {
			continue
		}
		out = append(out, homeCanonicalRow(id))
	}
	return out
}

// homeEnrich adds what a Home card or the hero shows beyond the shared entry (M5): the year, the
// content rating, the first two genres and the viewer's watchlist state. Four bounded reads for
// the whole page (at most one page of ids), never one per entry. A container work (a show, an
// album) takes its year from the browse projection; attributes and watchlist state are items'.
func (s *Service) homeEnrich(profile string, entries []ContentEntry) error {
	if len(entries) == 0 {
		return nil
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	list := idsJSON(ids)
	years := map[string]int{}
	rows, e := s.read().Query(`SELECT pid(entity.public_id),entity.year FROM json_each(?) requested CROSS JOIN catalog_entities entity ON entity.public_id=pid_blob(requested.value) WHERE entity.year>0`, list)
	if e != nil {
		return e
	}
	for rows.Next() {
		var id string
		var year int
		if e = rows.Scan(&id, &year); e != nil {
			rows.Close()
			return e
		}
		years[id] = year
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	ratings, genres := map[string]string{}, map[string][]string{}
	rows, e = s.read().Query(`SELECT pid(item.public_id),field.field,edge.source_value
	 FROM json_each(?) requested CROSS JOIN catalog_entities item ON item.public_id=pid_blob(requested.value)
	 CROSS JOIN catalog_item_attribute_edges edge ON edge.item_id=item.id
	 CROSS JOIN catalog_attribute_terms term ON term.id=edge.term_id
	 CROSS JOIN catalog_attribute_fields field ON field.id=term.field_id
	 WHERE field.field='contentRating'
	 ORDER BY item.id,edge.source_rowid`, list)
	if e != nil {
		return e
	}
	for rows.Next() {
		var id, field, value string
		if e = rows.Scan(&id, &field, &value); e != nil {
			rows.Close()
			return e
		}
		if field == "contentRating" && ratings[id] == "" {
			ratings[id] = value
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	// Every read starts from the requested ids (CROSS JOIN fixes the order):
	// left to the planner, the rating and genre reads started from the
	// field and the vocabulary and walked every title of the server.
	// Genres are the entity's genre terms (catalog_terms vocabulary 1), each
	// once however many providers name it, in the providers' order: the first
	// a provider names is the title's primary genre.
	rows, e = s.read().Query(`SELECT pid(item.public_id),COALESCE(min(src.label_override),term.label)
	 FROM json_each(?) requested CROSS JOIN catalog_entities item ON item.public_id=pid_blob(requested.value)
	 CROSS JOIN catalog_term_sources src ON src.entity_id=item.id
	 CROSS JOIN catalog_terms term ON term.id=src.term_id AND term.vocab=1
	 GROUP BY item.id,term.id ORDER BY item.id,min(src.ordinal),term.label`, list)
	if e != nil {
		return e
	}
	for rows.Next() {
		var id, label string
		if e = rows.Scan(&id, &label); e != nil {
			rows.Close()
			return e
		}
		if len(genres[id]) < 2 {
			genres[id] = append(genres[id], label)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	watchlisted := map[string]bool{}
	rows, e = s.read().Query(`SELECT pid(entity.public_id),mine.watchlisted FROM json_each(?) requested CROSS JOIN catalog_entities entity ON entity.public_id=pid_blob(requested.value)
	 CROSS JOIN personal_items mine ON mine.profile_id=? AND mine.item_id=entity.id`, list, profile)
	if e != nil {
		return e
	}
	for rows.Next() {
		var id string
		var on bool
		if e = rows.Scan(&id, &on); e != nil {
			rows.Close()
			return e
		}
		watchlisted[id] = on
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for index := range entries {
		entry := &entries[index]
		if year, ok := years[entry.ID]; ok {
			entry.Year = &year
		}
		entry.ContentRating = ratings[entry.ID]
		entry.Genres = genres[entry.ID]
		if isItemEntity(entry.Kind) || entry.Playback != nil {
			on := watchlisted[entry.ID]
			entry.Watchlisted = &on
		}
	}
	return nil
}
