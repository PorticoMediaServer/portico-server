package catalog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"portico.local/server/internal/facettext"
	"portico.local/server/internal/requestinput"
	"strconv"
	"strings"
	"time"
)

var ErrPersonQuery = requestinput.New("person paging requires limit 1–100 and role cast, crew or all")

type Person struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	SortName    string         `json:"sortName"`
	Biography   string         `json:"biography"`
	BirthDate   string         `json:"birthDate"`
	DeathDate   string         `json:"deathDate"`
	PortraitURL string         `json:"portraitUrl"`
	Roles       []string       `json:"roles"`
	KnownFor    []ContentEntry `json:"knownFor"`
	ProviderIDs map[string]any `json:"providerIds"`
	Revision    int64          `json:"revision"`
}
type PersonCredit struct {
	Media      ContentEntry `json:"media"`
	Role       string       `json:"role"`
	Character  string       `json:"character"`
	Department string       `json:"department"`
	CreditKind string       `json:"creditKind"`
}
type PersonPageInfo struct {
	Total      int    `json:"total"`
	NextCursor string `json:"nextCursor"`
}
type PersonPage struct {
	ServerID    string          `json:"serverId"`
	ViewerFence string          `json:"viewerFence"`
	Revision    ContentRevision `json:"revision"`
	Person      Person          `json:"person"`
	Credits     []PersonCredit  `json:"credits"`
	PageInfo    PersonPageInfo  `json:"pageInfo"`
}
type PersonSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	SortName    string   `json:"sortName"`
	PortraitURL string   `json:"portraitUrl,omitempty"`
	Roles       []string `json:"roles"`
	CreditCount int      `json:"creditCount"`
}
type PeopleDirectory struct {
	ServerID    string          `json:"serverId"`
	ViewerFence string          `json:"viewerFence"`
	Revision    ContentRevision `json:"revision"`
	Query       string          `json:"q"`
	People      []PersonSummary `json:"people"`
}

// Acting is the only cast department a provider publishes; everything else is
// crew. Clients never infer this, so the classification is published per credit.
func creditKind(department string) string {
	if strings.EqualFold(department, "Acting") || strings.EqualFold(department, "Cast") {
		return "cast"
	}
	return "crew"
}
func personDepartmentFilter(role string) (string, error) {
	switch role {
	case "", "all":
		return "", nil
	case "cast":
		return ` AND lower(dept.label) IN('acting','cast')`, nil
	case "crew":
		return ` AND lower(dept.label) NOT IN('acting','cast')`, nil
	}
	return "", ErrPersonQuery
}
func (s *Service) peopleRevision(libraries []string, profile string) (ContentRevision, error) {
	return s.homeRevision(libraries, profile)
}

// portraitSubject names the item and artwork subject that serve this person's
// portrait. Only artwork attached to an item the viewer may read is offered.
func (s *Service) portraitSubject(person string, viewer Viewer) (string, string, string, error) {
	restriction, bound := ItemRestrictionSQL("i.id", viewer.EffectiveRestrictions())
	var item, subject, digest string
	args := append([]any{person, viewer.librariesJSON()}, bound...)
	e := s.read().QueryRow(`SELECT pid(i.public_id),a.subject,a.digest FROM artwork_selections a JOIN catalog_people p ON p.token=? AND a.subject=p.identity_key JOIN catalog_credits pc ON pc.person_id=p.id AND pc.entity_id=a.entity_id JOIN catalog_entities i ON i.id=a.entity_id JOIN catalog_libraries l ON l.id=i.library_id AND l.library_id IN(SELECT value FROM json_each(?)) WHERE a.kind='item' AND a.role='portrait' AND `+restriction+` ORDER BY a.entity_id LIMIT 1`, args...).Scan(&item, &subject, &digest)
	if errors.Is(e, sql.ErrNoRows) {
		return "", "", "", nil
	}
	return item, subject, digest, e
}
func personPortraitURL(person, digest string) string {
	if digest == "" {
		return ""
	}
	return "/v1/people/" + url.PathEscape(person) + "/portrait?v=" + url.QueryEscape(digest) + "&w=400"
}

// PersonPortrait resolves the artwork the portrait route serves. It returns the
// owning item and artwork subject so the existing item artwork path serves it.
func (s *Service) PersonPortrait(viewer Viewer, person string) (string, string, error) {
	if e := s.prepareViewer(viewer); e != nil {
		return "", "", e
	}
	item, subject, _, e := s.portraitSubject(person, viewer)
	if e != nil {
		return "", "", e
	}
	if item == "" {
		return "", "", sql.ErrNoRows
	}
	return item, subject, nil
}
func (s *Service) personRoles(person string, viewer Viewer) ([]string, error) {
	restriction, bound := ItemRestrictionSQL("i.id", viewer.EffectiveRestrictions())
	args := append([]any{viewer.librariesJSON(), person}, bound...)
	rows, e := s.read().Query(`SELECT dept.label,count(*) FROM catalog_credits pc JOIN catalog_entities i ON i.id=pc.entity_id JOIN catalog_libraries l ON l.id=i.library_id AND l.library_id IN(SELECT value FROM json_each(?)) JOIN catalog_credit_labels dept ON dept.id=pc.department_id WHERE pc.person_id=(SELECT id FROM catalog_people WHERE token=?) AND dept.label<>'' AND `+restriction+` GROUP BY dept.label ORDER BY count(*) DESC,dept.label LIMIT 16`, args...)
	if e != nil {
		return nil, e
	}
	out := []string{}
	for rows.Next() {
		var department string
		var n int
		if e = rows.Scan(&department, &n); e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, department)
	}
	e = rows.Err()
	rows.Close()
	return out, e
}

// knownFor is the person's strongest billing, not a popularity guess: lowest
// credit ordinal first, then the most recently catalogued work.
func (s *Service) personKnownFor(person string, viewer Viewer) ([]ContentEntry, error) {
	restriction, bound := ItemRestrictionSQL("i.id", viewer.EffectiveRestrictions())
	args := append([]any{viewer.librariesJSON(), person}, bound...)
	rows, e := s.read().Query(`SELECT pid(i.public_id),min(pc.ord) FROM catalog_credits pc JOIN catalog_entities i ON i.id=pc.entity_id JOIN catalog_libraries l ON l.id=i.library_id AND l.library_id IN(SELECT value FROM json_each(?)) JOIN catalog_item_details d ON d.entity_id=i.id WHERE pc.person_id=(SELECT id FROM catalog_people WHERE token=?) AND `+restriction+` GROUP BY pc.entity_id ORDER BY min(pc.ord),COALESCE(d.added_text,'') DESC,pc.entity_id LIMIT 8`, args...)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		var ordinal int
		if e = rows.Scan(&id, &ordinal); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	items, e := s.mediaPage(viewer.Profile, ids, false)
	if e != nil {
		return nil, e
	}
	byID := map[string]Item{}
	for _, item := range items {
		byID[item.ID] = item
	}
	out := []ContentEntry{}
	for _, id := range ids {
		item, ok := byID[id]
		if !ok {
			continue
		}
		entry := contentItem(item)
		if !item.Available {
			entry.Playback = nil
		}
		out = append(out, entry)
	}
	return out, nil
}
func (s *Service) person(id string, viewer Viewer) (Person, error) {
	out := Person{ID: id, Roles: []string{}, KnownFor: []ContentEntry{}, ProviderIDs: map[string]any{}}
	var providers string
	e := s.read().QueryRow(`SELECT token,name,sort_name,biography,birth_date,death_date,portrait_url,provider_ids,revision FROM catalog_people WHERE token=?`, id).Scan(&out.ID, &out.Name, &out.SortName, &out.Biography, &out.BirthDate, &out.DeathDate, &out.PortraitURL, &providers, &out.Revision)
	if e != nil {
		return out, e
	}
	if e = json.Unmarshal([]byte(providers), &out.ProviderIDs); e != nil {
		out.ProviderIDs = map[string]any{}
	}
	_, _, digest, e := s.portraitSubject(id, viewer)
	if e != nil {
		return out, e
	}
	// A provider portrait URL is evidence, never a client-reachable address.
	out.PortraitURL = personPortraitURL(id, digest)
	return out, nil
}

// Person publishes one canonical person with a cursor page of their credits.
// Cost: one indexed count, one indexed page of at most limit rows, plus the
// bounded known-for and role rollups.
func (s *Service) Person(viewer Viewer, server, id, role, cursor string, limit int) (PersonPage, error) {
	out := PersonPage{ServerID: server, ViewerFence: viewer.Fence, Credits: []PersonCredit{}}
	if e := s.prepareViewer(viewer); e != nil {
		return out, e
	}
	department, e := personDepartmentFilter(role)
	if e != nil {
		return out, e
	}
	if limit < 1 || limit > 100 {
		return out, ErrPersonQuery
	}
	rev, e := s.peopleRevision(viewer.Libraries, viewer.Profile)
	if e != nil {
		return out, e
	}
	out.Revision = rev
	visible, visibleArgs := viewer.itemVisibilitySQL("pc.entity_id")
	visibleArgs = append([]any{id}, visibleArgs...)
	var entitled int
	if e = s.read().QueryRow(`SELECT 1 FROM catalog_credits pc WHERE pc.person_id=(SELECT id FROM catalog_people WHERE token=?) AND `+visible+` LIMIT 1`, visibleArgs...).Scan(&entitled); e != nil {
		return out, e
	}
	if out.Person, e = s.person(id, viewer); e != nil {
		return out, e
	}
	if out.Person.Roles, e = s.personRoles(id, viewer); e != nil {
		return out, e
	}
	if out.Person.KnownFor, e = s.personKnownFor(id, viewer); e != nil {
		return out, e
	}
	restriction, bound := ItemRestrictionSQL("i.id", viewer.EffectiveRestrictions())
	where := ` FROM catalog_credits pc JOIN catalog_entities i ON i.id=pc.entity_id JOIN catalog_libraries l ON l.id=i.library_id AND l.library_id IN(SELECT value FROM json_each(?)) JOIN catalog_credit_labels role ON role.id=pc.role_id JOIN catalog_credit_labels dept ON dept.id=pc.department_id WHERE pc.person_id=(SELECT id FROM catalog_people WHERE token=?)` + department + ` AND ` + restriction
	args := append([]any{viewer.librariesJSON(), id}, bound...)
	if e = s.read().QueryRow(`SELECT count(*)`+where, args...).Scan(&out.PageInfo.Total); e != nil {
		return out, e
	}
	scope := cursorScope{Profile: viewer.Profile, View: "person", Entity: id, Category: role, Viewer: viewer.Fence, Sort: "title", Direction: "asc", Limit: limit}
	if cursor != "" {
		c, err := s.decodeRevisionCursor(cursor, scope, rev)
		if err != nil {
			return out, err
		}
		entity, ord, ok := strings.Cut(c.ID, ":")
		entityID, entityErr := strconv.ParseInt(entity, 10, 64)
		ordNum, ordErr := strconv.ParseInt(ord, 10, 64)
		if !ok || entityErr != nil || ordErr != nil {
			return out, ErrCursor
		}
		where += ` AND (i.title COLLATE NOCASE>? COLLATE NOCASE OR (i.title COLLATE NOCASE=? COLLATE NOCASE AND (pc.entity_id>? OR (pc.entity_id=? AND pc.ord>?))))`
		args = append(args, c.Value, c.Value, entityID, entityID, ordNum)
	}
	args = append(args, limit+1)
	rows, e := s.read().Query(`SELECT pc.entity_id,pc.ord,pid(i.public_id),i.title,role.label,dept.label`+where+` ORDER BY i.title COLLATE NOCASE,pc.entity_id,pc.ord LIMIT ?`, args...)
	if e != nil {
		return out, e
	}
	type row struct {
		entity, ord                   int64
		item, title, role, department string
	}
	selected := []row{}
	for rows.Next() {
		var v row
		if e = rows.Scan(&v.entity, &v.ord, &v.item, &v.title, &v.role, &v.department); e != nil {
			rows.Close()
			return out, e
		}
		selected = append(selected, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(selected) > limit {
		selected = selected[:limit]
		last := selected[limit-1]
		out.PageInfo.NextCursor, e = s.encodeRevisionCursor(cursorValue{Scope: scope, Value: last.title, ID: strconv.FormatInt(last.entity, 10) + ":" + strconv.FormatInt(last.ord, 10), Expires: time.Now().Add(30 * time.Minute).Unix()}, rev)
		if e != nil {
			return out, e
		}
	}
	ids := []string{}
	for _, v := range selected {
		ids = append(ids, v.item)
	}
	items, e := s.mediaPage(viewer.Profile, ids, false)
	if e != nil {
		return out, e
	}
	byID := map[string]Item{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, v := range selected {
		item, ok := byID[v.item]
		if !ok {
			return out, ErrStaleContinuation
		}
		entry := contentItem(item)
		if !item.Available {
			entry.Playback = nil
		}
		out.Credits = append(out.Credits, PersonCredit{Media: entry, Role: v.role, Character: v.role, Department: v.department, CreditKind: creditKind(v.department)})
	}
	after, e := s.peopleRevision(viewer.Libraries, viewer.Profile)
	if e != nil {
		return out, e
	}
	if after != rev {
		return out, ErrStaleContinuation
	}
	return out, nil
}

// People answers a bounded name search. The index is a prefix-token projection
// of canonical person names; matching and ordering stay on the server.
func (s *Service) People(viewer Viewer, server, q string, limit int) (PeopleDirectory, error) {
	out := PeopleDirectory{ServerID: server, ViewerFence: viewer.Fence, People: []PersonSummary{}}
	if e := s.prepareViewer(viewer); e != nil {
		return out, e
	}
	if limit < 1 || limit > 100 {
		return out, ErrPersonQuery
	}
	normalized, match, e := NormalizeSearch(q)
	if e != nil {
		return out, e
	}
	out.Query = normalized
	if out.Revision, e = s.peopleRevision(viewer.Libraries, viewer.Profile); e != nil {
		return out, e
	}
	summaries, _, _, e := s.peopleMatches(normalized, match, viewer, limit, 0, "relevance", "desc")
	if e != nil {
		return out, e
	}
	out.People = summaries
	return out, nil
}

// peopleRankedMaximum is the most matching people a name search ranks by
// relevance (bm25 with the exact-name and name-prefix boosts). A broader
// search (a letter or two typed) reads people in name order instead, those
// whose name starts with the typed text first, so its cost follows the page.
const peopleRankedMaximum = 1000

// peopleWalkWindow is how many people a broad name search reads in name order
// for one page before it ranks the matches after all.
const peopleWalkWindow = 20000

// peopleWalkChunk is how many people of the window are tested at a time.
const peopleWalkChunk = 64

// personCreditedSQL is "this person has a credit the viewer can see" over the
// person id expression. An unrestricted viewer reads the maintained counts; a
// restricted one tests the restriction on the person's credits.
func personCreditedSQL(viewer Viewer, person string) (string, []any) {
	restrictions := viewer.EffectiveRestrictions()
	if !restrictions.Active() {
		return `EXISTS(SELECT 1 FROM catalog_person_credit_counts pcc JOIN catalog_libraries l ON l.id=pcc.library_id AND l.library_id IN(SELECT value FROM json_each(?)) WHERE pcc.person_id=` + person + `)`, []any{viewer.librariesJSON()}
	}
	restriction, bound := ItemRestrictionSQL("i.id", restrictions)
	return `EXISTS(SELECT 1 FROM catalog_credits pc JOIN catalog_entities i ON i.id=pc.entity_id JOIN catalog_libraries l ON l.id=i.library_id AND l.library_id IN(SELECT value FROM json_each(?)) WHERE pc.person_id=` + person + ` AND ` + restriction + `)`, append([]any{viewer.librariesJSON()}, bound...)
}

// personCreditCountSQL counts the person's credits the viewer can see.
func personCreditCountSQL(viewer Viewer, person string) (string, []any) {
	restrictions := viewer.EffectiveRestrictions()
	if !restrictions.Active() {
		return `(SELECT COALESCE(sum(pcc.credits),0) FROM catalog_person_credit_counts pcc JOIN catalog_libraries l ON l.id=pcc.library_id AND l.library_id IN(SELECT value FROM json_each(?)) WHERE pcc.person_id=` + person + `)`, []any{viewer.librariesJSON()}
	}
	restriction, bound := ItemRestrictionSQL("i.id", restrictions)
	return `(SELECT count(*) FROM catalog_credits pc JOIN catalog_entities i ON i.id=pc.entity_id JOIN catalog_libraries l ON l.id=i.library_id AND l.library_id IN(SELECT value FROM json_each(?)) WHERE pc.person_id=` + person + ` AND ` + restriction + `)`, append([]any{viewer.librariesJSON()}, bound...)
}

// peopleMatches is shared by the directory and the search people group. offset
// and the sort are applied by the caller's paging contract. total counts the
// matches up to peopleRankedMaximum; exact is false beyond it.
func (s *Service) peopleMatches(q, match string, viewer Viewer, limit, offset int, order, direction string) (out []PersonSummary, total int, exact bool, e error) {
	credited, creditedArgs := personCreditedSQL(viewer, "people.id")
	extra, extraArgs := searchSubwordPredicate("portico_search_fold(people.name)", q)
	matched := `catalog_people_names JOIN catalog_people people ON people.id=catalog_people_names.rowid WHERE catalog_people_names MATCH ? AND ` + credited + extra
	matchedArgs := append(append([]any{match}, creditedArgs...), extraArgs...)
	if e = s.read().QueryRow(`SELECT count(*) FROM (SELECT 1 FROM `+matched+` LIMIT `+strconv.Itoa(peopleRankedMaximum+1)+`)`, matchedArgs...).Scan(&total); e != nil {
		return nil, 0, false, e
	}
	exact = total <= peopleRankedMaximum
	if !exact {
		total = peopleRankedMaximum
	}
	ids := []int64(nil)
	if !exact {
		var walked bool
		ids, walked, e = s.peopleNameWalk(q, match, viewer, limit, offset, order, direction)
		if e != nil {
			return nil, 0, false, e
		}
		if !walked {
			ids = nil
		}
	}
	count, countArgs := personCreditCountSQL(viewer, "page.id")
	var rows *sql.Rows
	if ids != nil {
		raw, _ := json.Marshal(ids)
		rows, e = s.read().Query(`SELECT page.token,page.name,page.sort_name,`+count+` FROM json_each(?) j CROSS JOIN catalog_people page ON page.id=j.value ORDER BY j.key`, append(countArgs, string(raw))...)
	} else {
		// Relevance is bm25 over the name index with an exact-name and a name-prefix
		// boost, so "ada" surfaces Ada before Adaline without client-side ranking.
		// The page is chosen first; credits are counted only for its people.
		relevance := `(-bm25(catalog_people_names))+(CASE WHEN portico_search_fold(people.name)=? THEN 1000 ELSE 0 END)+(CASE WHEN portico_search_fold(people.name) LIKE ? ESCAPE '\' THEN 100 ELSE 0 END)`
		relevanceArgs := []any{facettext.SearchFold(q), searchPrefix(facettext.SearchFold(q))}
		inner, outer := `ORDER BY rel DESC,people.sort_name,people.id`, `ORDER BY page.rel DESC,page.sort_name,page.id`
		if order == "title" {
			relevance, relevanceArgs = `0`, nil
			dir := strings.ToUpper(direction)
			inner, outer = `ORDER BY people.sort_name `+dir+`,people.id `+dir, `ORDER BY page.sort_name `+dir+`,page.id `+dir
		}
		args := append([]any{}, countArgs...)
		args = append(args, relevanceArgs...)
		args = append(args, matchedArgs...)
		args = append(args, limit, offset)
		rows, e = s.read().Query(`SELECT page.token,page.name,page.sort_name,`+count+` credits
	 FROM (SELECT people.id,people.token,people.name,people.sort_name,`+relevance+` AS rel FROM `+matched+` `+inner+` LIMIT ? OFFSET ?) page `+outer, args...)
	}
	if e != nil {
		return nil, 0, false, e
	}
	out = []PersonSummary{}
	for rows.Next() {
		var v PersonSummary
		v.Roles = []string{}
		if e = rows.Scan(&v.ID, &v.Name, &v.SortName, &v.CreditCount); e != nil {
			rows.Close()
			return nil, 0, false, e
		}
		out = append(out, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, 0, false, e
	}
	// The portrait and the roles used to be a query each, per person: a hundred
	// results were two hundred and one statements for one directory page. They
	// are the same two questions asked of a list of ids, so they are asked once.
	tokens := make([]string, 0, len(out))
	for i := range out {
		tokens = append(tokens, out[i].ID)
	}
	digests, e := s.portraitDigests(tokens, viewer)
	if e != nil {
		return nil, 0, false, e
	}
	roles, e := s.peopleRoles(tokens, viewer)
	if e != nil {
		return nil, 0, false, e
	}
	for i := range out {
		out[i].PortraitURL = personPortraitURL(out[i].ID, digests[out[i].ID])
		if named := roles[out[i].ID]; named != nil {
			out[i].Roles = named
		}
	}
	return out, total, exact, nil
}

// peopleNameWalk pages a broad name search in name order without ranking its
// matches: for relevance, the people whose name starts with the typed text
// first, then the rest; for the name sort, everyone in name order. Each
// person read is tested against the name index by its id. ok is false when
// peopleWalkWindow people held neither the page nor the end of the list.
func (s *Service) peopleNameWalk(q, match string, viewer Viewer, limit, offset int, order, direction string) (ids []int64, ok bool, e error) {
	credited, creditedArgs := personCreditedSQL(viewer, "people.id")
	extra, extraArgs := searchSubwordPredicate("portico_search_fold(people.name)", q)
	want := offset + limit
	dir := "ASC"
	if order == "title" && direction == "desc" {
		dir = "DESC"
	}
	// A person's name is tested here, as the name index tokens it: every typed
	// word starts one of the name's words. The index itself, asked about one
	// person at a time, reads every name a broad prefix matches each time.
	typed := facettext.SearchTerms(q)
	named := func(name string) bool {
		words := strings.Fields(facettext.SearchDocument(name))
		for _, term := range typed {
			found := false
			for _, word := range words {
				if strings.HasPrefix(word, term) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	walk := func(bound string, boundArgs []any, n int) ([]int64, bool, error) {
		// The window is a bounded run of the name index, read in order; the
		// names that match are tested for a visible credit a chunk at a time
		// and the walk stops at n.
		rows, err := s.read().Query(`SELECT people.id,people.name FROM catalog_people people INDEXED BY catalog_people_name WHERE 1`+bound+` ORDER BY people.sort_name `+dir+`,people.id `+dir+` LIMIT `+strconv.Itoa(peopleWalkWindow), boundArgs...)
		if err != nil {
			return nil, false, err
		}
		read, candidates := 0, []int64{}
		for rows.Next() {
			var id int64
			var name string
			if err = rows.Scan(&id, &name); err != nil {
				rows.Close()
				return nil, false, err
			}
			read++
			if named(name) {
				candidates = append(candidates, id)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, false, err
		}
		found := []int64{}
		for at := 0; at < len(candidates) && len(found) < n; at += peopleWalkChunk {
			chunk, _ := json.Marshal(candidates[at:min(at+peopleWalkChunk, len(candidates))])
			passed, err := s.entityIDs(`SELECT people.id FROM json_each(?) j CROSS JOIN catalog_people people ON people.id=j.value WHERE `+credited+extra+` ORDER BY j.key`, append(append([]any{string(chunk)}, creditedArgs...), extraArgs...)...)
			if err != nil {
				return nil, false, err
			}
			found = append(found, passed...)
		}
		if len(found) >= n {
			return found[:n], true, nil
		}
		return found, read < peopleWalkWindow, nil
	}
	if order == "title" {
		found, done, err := walk("", nil, want)
		if err != nil || !done {
			return nil, false, err
		}
		if offset >= len(found) {
			return []int64{}, true, nil
		}
		return found[offset:], true, nil
	}
	lower := strings.ToLower(q)
	prefix := ` AND people.sort_name>=? AND people.sort_name<?`
	prefixArgs := []any{lower, lower + "\U0010ffff"}
	first, done, err := walk(prefix, prefixArgs, want)
	if err != nil || !done {
		return nil, false, err
	}
	if len(first) < want {
		rest, done, err := walk(` AND NOT (people.sort_name>=? AND people.sort_name<?)`, prefixArgs, want-len(first))
		if err != nil || !done {
			return nil, false, err
		}
		first = append(first, rest...)
	}
	if offset >= len(first) {
		return []int64{}, true, nil
	}
	return first[offset:], true, nil
}

// portraitDigests resolves the portrait thumbnail digest for a page of people in
// one statement. It is the batched form of portraitSubject: the same selection
// — the lowest item id carrying a portrait for that person in a visible library
// — resolved for every id at once.
func (s *Service) portraitDigests(ids []string, viewer Viewer) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	restriction, bound := ItemRestrictionSQL("i.id", viewer.EffectiveRestrictions())
	args := make([]any, 0, len(ids)+1+len(bound))
	args = append(args, viewer.librariesJSON())
	args = append(args, bound...)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, e := s.read().Query(`SELECT p.token,COALESCE((SELECT a.digest FROM artwork_selections a
 JOIN catalog_credits pc ON pc.person_id=p.id AND pc.entity_id=a.entity_id
	 JOIN catalog_entities i ON i.id=a.entity_id JOIN catalog_libraries l ON l.id=i.library_id AND l.library_id IN(SELECT value FROM json_each(?))
 WHERE a.subject=p.identity_key AND a.kind='item' AND a.role='portrait' AND `+restriction+` ORDER BY a.entity_id LIMIT 1),'')
 FROM catalog_people p WHERE p.token IN (`+placeholders(len(ids))+`)`, args...)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var id, digest string
		if e = rows.Scan(&id, &digest); e != nil {
			rows.Close()
			return nil, e
		}
		out[id] = digest
	}
	e = rows.Err()
	rows.Close()
	return out, e
}

// peopleRoles is personRoles for a page of people: the same grouping, the same
// ordering, the same limit of sixteen departments each, in one statement.
func (s *Service) peopleRoles(ids []string, viewer Viewer) (map[string][]string, error) {
	out := map[string][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	query := `SELECT p.token,dept.label,sum(pcc.credits) AS n FROM catalog_person_credit_counts pcc
	 JOIN catalog_people p ON p.id=pcc.person_id
	 JOIN catalog_libraries l ON l.id=pcc.library_id AND l.library_id IN(SELECT value FROM json_each(?))
	 JOIN catalog_credit_labels dept ON dept.id=pcc.department_id
 WHERE p.token IN (` + placeholders(len(ids)) + `) AND dept.label<>''
 GROUP BY p.token,dept.label ORDER BY p.token,n DESC,dept.label`
	args := []any{viewer.librariesJSON()}
	for _, id := range ids {
		args = append(args, id)
	}
	if restrictions := viewer.EffectiveRestrictions(); restrictions.Active() {
		restriction, bound := ItemRestrictionSQL("i.id", restrictions)
		query = `SELECT p.token,dept.label,count(*) AS n FROM catalog_credits pc
	 JOIN catalog_people p ON p.id=pc.person_id
	 JOIN catalog_entities i ON i.id=pc.entity_id JOIN catalog_libraries l ON l.id=i.library_id AND l.library_id IN(SELECT value FROM json_each(?))
	 JOIN catalog_credit_labels dept ON dept.id=pc.department_id
 WHERE p.token IN (` + placeholders(len(ids)) + `) AND dept.label<>'' AND ` + restriction + `
 GROUP BY p.token,dept.label ORDER BY p.token,n DESC,dept.label`
		args = append(args, bound...)
	}
	rows, e := s.read().Query(query, args...)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var person, department string
		var n int
		if e = rows.Scan(&person, &department, &n); e != nil {
			rows.Close()
			return nil, e
		}
		// The per-person limit of sixteen is applied here rather than in SQL,
		// because one statement cannot carry a per-group LIMIT.
		if len(out[person]) < 16 {
			out[person] = append(out[person], department)
		}
	}
	e = rows.Err()
	rows.Close()
	return out, e
}
