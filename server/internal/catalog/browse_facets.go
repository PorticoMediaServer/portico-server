package catalog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// Facets count the values a viewer could actually choose. Every count is taken
// through browse_entities, so retired DVR rows and rows outside the library are
// excluded by construction, and the result is cached per catalog revision.

type FacetValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int    `json:"count"`
}
type FacetPage struct {
	Field  string       `json:"field"`
	Values []FacetValue `json:"values"`
}
type FacetRequest struct {
	Library string
	Profile string
	Field   string
	Q       string
	Limit   int
	// Restrictions must match the browse request whose facets these are, or the
	// counts would advertise titles the page itself refuses to show.
	Restrictions identity.ContentRestrictions
}

// facetSource describes how one facet is counted. Join is appended after
// `FROM catalog_browse_rows e` and may reference e.item_id (the INTEGER entity
// id of the member item).
type facetSource struct {
	Join  string
	Value string
	Label string
}

var facetSources = map[string]facetSource{
	"genre":         {Join: `JOIN catalog_term_sources ts ON ts.entity_id IN(e.item_id,(SELECT ep.show_id FROM catalog_episodes ep WHERE ep.entity_id=e.item_id)) JOIN catalog_terms mg ON mg.id=ts.term_id AND mg.vocab=1`, Value: "COALESCE(ts.label_override,mg.label)", Label: "COALESCE(ts.label_override,mg.label)"},
	"decade":        {Value: "CAST(((e.year/10)*10) AS TEXT)", Label: "CAST(((e.year/10)*10) AS TEXT)||'s'", Join: ``},
	"year":          {Value: "CAST(e.year AS TEXT)", Label: "CAST(e.year AS TEXT)"},
	"contentRating": {Join: attributeFacetJoin("contentRating"), Value: "ca.source_value", Label: "ca.source_value"},
	"resolution":    {Join: `JOIN catalog_asset_links ia ON ia.entity_id=e.item_id JOIN catalog_assets ast ON ast.id=ia.asset_id`, Value: `CASE WHEN ast.height>=2000 THEN '4k' WHEN ast.height>=1000 THEN '1080p' WHEN ast.height>=700 THEN '720p' ELSE 'sd' END`, Label: `CASE WHEN ast.height>=2000 THEN '4K' WHEN ast.height>=1000 THEN '1080p' WHEN ast.height>=700 THEN '720p' ELSE 'SD' END`},
	"audioLanguage": {Join: attributeFacetJoin("audioLanguage"), Value: "ca.source_value", Label: "ca.source_value"},
	"studio":        {Join: attributeFacetJoin("studio"), Value: "ca.source_value", Label: "ca.source_value"},
	"network":       {Join: attributeFacetJoin("network"), Value: "ca.source_value", Label: "ca.source_value"},
	"tag":           {Join: attributeFacetJoin("tag"), Value: "ca.source_value", Label: "ca.source_value"},
	"label":         {Join: attributeFacetJoin("label"), Value: "ca.source_value", Label: "ca.source_value"},
	"series":        {Join: attributeFacetJoin("series"), Value: "ca.source_value", Label: "ca.source_value"},
	"collection":    {Join: `JOIN catalog_collection_members cim ON cim.item_id=e.item_id JOIN catalog_entities col ON col.id=cim.collection_id`, Value: "pid(col.public_id)", Label: "col.title"},
}

func attributeFacetJoin(field string) string {
	return `JOIN catalog_item_attribute_edges ca ON ca.item_id=e.item_id JOIN catalog_attribute_terms ca_term ON ca_term.id=ca.term_id AND ca_term.field_id=(SELECT id FROM catalog_attribute_fields WHERE field='` + field + `')`
}

// Facets returns the counted values of one field under the viewer's visibility.
func (s *Service) Facets(viewer Viewer, request FacetRequest) (FacetPage, error) {
	out := FacetPage{Field: request.Field, Values: []FacetValue{}}
	if !viewer.AllowsLibrary(request.Library) {
		return out, sql.ErrNoRows
	}
	if err := s.prepareViewer(viewer); err != nil {
		return out, err
	}
	request.Profile, request.Restrictions = viewer.Profile, viewer.EffectiveRestrictions()
	source, ok := facetSources[request.Field]
	if !ok {
		return out, browseIssue("field", "field is not a counted facet")
	}
	if request.Limit < 1 || request.Limit > browseFacetLimit {
		request.Limit = browseFacetLimit
	}
	if len(request.Q) > 100 {
		return out, browseIssue("q", "facet filter exceeds 100 characters")
	}
	if _, err := s.library(request.Library); err != nil {
		return out, err
	}
	if err := s.browseReady(request.Library); err != nil {
		return out, err
	}
	// The key already carries the restriction fence, so one viewer's counts can
	// never be served to a viewer under a different restriction. It leaves out
	// q and the limit: a field's whole value list is counted once per catalogue
	// revision, and a typeahead's filter and limit are applied to it here, so
	// each keystroke is not another pass over the library.
	key := request.Library + "\x00" + request.Field + "\x00" + RestrictionFence(request.Restrictions)
	revision, err := s.ContentRevision(request.Library, request.Profile)
	if err != nil {
		return out, err
	}
	all, hit := s.state.facetCache.get(key, revision.Catalog)
	if !hit {
		// Single-flight: two hundred viewers arriving at a cold facet together
		// produce one computation, not two hundred.
		all, err = s.state.facetCache.do(key, func() (FacetPage, error) {
			if cached, hit := s.state.facetCache.get(key, revision.Catalog); hit {
				return cached, nil
			}
			return s.computeFacets(request, source, revision, key)
		})
		if err != nil {
			return out, err
		}
	}
	needle := asciiLower(request.Q)
	for _, value := range all.Values {
		if len(out.Values) == request.Limit {
			break
		}
		if needle == "" || strings.Contains(asciiLower(value.Label), needle) {
			out.Values = append(out.Values, value)
		}
	}
	return out, nil
}

func (s *Service) computeFacets(request FacetRequest, source facetSource, revision ContentRevision, key string) (FacetPage, error) {
	out := FacetPage{Field: request.Field, Values: []FacetValue{}}
	// The maintained counts answer every viewer whose restriction is decided
	// per item (age, unrated, labels). A member-level restriction, or counts
	// still being built, count live below.
	r := request.Restrictions
	// A show library counts shows (derive_facet_counts.go). The maintained
	// counts are kept under the show's own keys, which cannot say whether a
	// restricted viewer may see any of its episodes, so a restricted viewer's
	// counts in a show library are taken live: a show counts when one of its
	// episodes is visible to them.
	lib, err := s.library(request.Library)
	if err != nil {
		return out, err
	}
	shows := lib.Kind == "tv" || lib.Kind == "anime"
	if r.MemberMaxRating == "" && len(r.MemberDeniedLabels) == 0 && !(shows && r.Active()) && s.compactProjectionReady(33) == nil {
		if out.Values, err = s.facetsFromCounts(request); err != nil {
			return out, err
		}
		return s.storeFacets(request, revision, key, out)
	}
	query := ""
	args := []any{}
	if request.Field == "decade" && request.Restrictions.Active() {
		classKey, generation, ready, classErr := s.publishedVisibilityClass(request.Library, request.Restrictions)
		if classErr != nil {
			return out, classErr
		}
		if ready {
			// The background class maintains decade totals. A facet-cache miss
			// must not recheck every item's rating and labels in a large library.
			query = `SELECT CAST(decade AS TEXT),CAST(decade AS TEXT)||'s',sum(total) AS facet_count
				FROM compact_visibility_counts WHERE class_id=(SELECT id FROM compact_visibility_classes WHERE class_key=?) AND generation=?
				AND library_id=(SELECT id FROM catalog_libraries WHERE library_id=?)
				AND kind IN (1,4,7,9,11)
				AND decade BETWEEN 1800 AND 2190`
			if shows {
				// The class counts its visible shows (kind 2) as well as their episodes.
				query = strings.Replace(query, "kind IN (1,4,7,9,11)", "kind=2", 1)
			}
			args = []any{classKey, generation, request.Library}
			query += ` GROUP BY decade ORDER BY facet_count DESC,decade COLLATE NOCASE`
		}
	}
	if query == "" && shows {
		// Each visible episode stands for its show: the row a facet reads is the
		// episode (its file and title facts) under the show's id and year.
		restriction, restrictionArgs := ItemRestrictionSQL("er.item_id", request.Restrictions)
		from := `(SELECT er.item_id AS item_id,ep.show_id AS entity_id,sh.year AS year FROM catalog_browse_rows er
		 JOIN catalog_episodes ep ON ep.entity_id=er.entity_id JOIN catalog_browse_rows sh ON sh.entity_id=ep.show_id
		 WHERE er.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND er.item_id IS NOT NULL AND ` + restriction + `) e`
		args = append([]any{request.Library}, restrictionArgs...)
		where := "1"
		if request.Field == "decade" || request.Field == "year" {
			where = `e.year BETWEEN 1800 AND 2199`
		}
		query = `SELECT ` + source.Value + ` AS facet_value,` + source.Label + ` AS facet_label,count(DISTINCT e.entity_id) AS facet_count FROM ` + from + ` ` + source.Join + ` WHERE ` + where + ` GROUP BY facet_value ORDER BY facet_count DESC,facet_label COLLATE NOCASE`
	}
	if query == "" {
		where := `e.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND e.item_id IS NOT NULL`
		args = []any{request.Library}
		if request.Field == "decade" || request.Field == "year" {
			where += ` AND e.year BETWEEN 1800 AND 2199`
		}
		if restriction, restrictionArgs := ItemRestrictionSQL("e.item_id", request.Restrictions); restriction != "1" {
			where += " AND " + restriction
			args = append(args, restrictionArgs...)
		}
		query = `SELECT ` + source.Value + ` AS facet_value,` + source.Label + ` AS facet_label,count(DISTINCT e.entity_id) AS facet_count FROM catalog_browse_rows e ` + source.Join + ` WHERE ` + where + ` GROUP BY facet_value ORDER BY facet_count DESC,facet_label COLLATE NOCASE`
	}
	rows, err := s.read().Query(query, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var value FacetValue
		if err = rows.Scan(&value.Value, &value.Label, &value.Count); err != nil {
			rows.Close()
			return out, err
		}
		if value.Value == "" {
			continue
		}
		out.Values = append(out.Values, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return s.storeFacets(request, revision, key, out)
}

// storeFacets keeps a field's counted values for this catalogue revision.
func (s *Service) storeFacets(request FacetRequest, revision ContentRevision, key string, out FacetPage) (FacetPage, error) {
	after, err := s.ContentRevision(request.Library, request.Profile)
	if err != nil {
		return out, err
	}
	if after.Catalog != revision.Catalog {
		return out, ErrStaleContinuation
	}
	s.state.facetCache.put(key, facetCacheEntry{revision: revision.Catalog, publications: dbwork.Publications(), expires: time.Now().Add(time.Minute), page: out})
	return out, nil
}

// facetsFromCounts reads one field's maintained counts (catalog_facet_counts)
// and decides each count against the viewer's restriction by its keys, as the
// browse buckets are decided: the cost is the field's distinct values, never
// the library's items.
func (s *Service) facetsFromCounts(request FacetRequest) ([]FacetValue, error) {
	r := request.Restrictions
	var ages ratingAges
	blocked := map[string]bool{}
	if r.Active() {
		var err error
		if ages, err = s.ratingAges(); err != nil {
			return nil, err
		}
		for _, label := range r.BlockedLabels {
			blocked[strings.ToLower(label)] = true
		}
	}
	rows, err := s.read().Query(`SELECT value,rating_key,label_key,total FROM catalog_facet_counts
	 WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND field=?`, request.Library, request.Field)
	if err != nil {
		return nil, err
	}
	totals := map[string]int{}
	for rows.Next() {
		var value string
		var bucket browseBucket
		if err = rows.Scan(&value, &bucket.ratingKey, &bucket.labelKey, &bucket.total); err != nil {
			rows.Close()
			return nil, err
		}
		if !r.Active() || bucketVisible(bucket, ages, r, blocked) {
			totals[value] += bucket.total
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	labels := map[string]string{}
	if request.Field == "collection" && len(totals) > 0 {
		// Collection counts are kept by entity id; the value a filter takes is
		// the collection's public id and its label is its title.
		ids := make([]string, 0, len(totals))
		for id := range totals {
			ids = append(ids, id)
		}
		raw, _ := json.Marshal(ids)
		titles, err := s.read().Query(`SELECT CAST(e.id AS TEXT),pid(e.public_id),e.title FROM json_each(?) j CROSS JOIN catalog_entities e ON e.id=CAST(j.value AS INTEGER)`, string(raw))
		if err != nil {
			return nil, err
		}
		public := map[string]int{}
		for titles.Next() {
			var id, pid, title string
			if err = titles.Scan(&id, &pid, &title); err != nil {
				titles.Close()
				return nil, err
			}
			public[pid] += totals[id]
			labels[pid] = title
		}
		err = titles.Err()
		titles.Close()
		if err != nil {
			return nil, err
		}
		totals = public
	}
	out := make([]FacetValue, 0, len(totals))
	for value, count := range totals {
		if count == 0 {
			continue
		}
		label := value
		switch request.Field {
		case "decade":
			label = value + "s"
		case "resolution":
			label = map[string]string{"4k": "4K", "sd": "SD"}[value]
			if label == "" {
				label = value
			}
		case "collection":
			label = labels[value]
		}
		out = append(out, FacetValue{Value: value, Label: label, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if a, b := asciiLower(out[i].Label), asciiLower(out[j].Label); a != b {
			return a < b
		}
		return out[i].Value < out[j].Value
	})
	return out, nil
}

// browseCategoryPredicate translates the legacy `category=<facet>:<value>` chip
// into an expression predicate so both surfaces execute the same engine.
func browseCategoryPredicate(category string) (*BrowseNode, error) {
	field, value, ok := strings.Cut(category, ":")
	if !ok || value == "" {
		return nil, errors.New("unknown category")
	}
	switch field {
	case "decade":
		year, err := strconv.Atoi(value)
		if err != nil || year < 1800 || year > 2190 || year%10 != 0 {
			return nil, errors.New("invalid decade category")
		}
		return &BrowseNode{Field: "decade", Operator: "equals", Value: float64(year)}, nil
	case "genre", "series", "tag", "label", "studio", "network", "contentRating", "audioLanguage", "collection", "resolution":
		definition, ok := browseFieldByID(field)
		if !ok {
			return nil, errors.New("unknown category")
		}
		operator := "equals"
		if definition.Type == valueSet {
			operator = "contains"
		}
		return &BrowseNode{Field: field, Operator: operator, Value: value}, nil
	case "year":
		year, err := strconv.Atoi(value)
		if err != nil || year < 1800 || year > 2199 {
			return nil, errors.New("invalid year category")
		}
		return &BrowseNode{Field: "year", Operator: "equals", Value: float64(year)}, nil
	}
	return nil, errors.New("unknown category")
}
