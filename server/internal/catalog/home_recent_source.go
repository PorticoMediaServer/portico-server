package catalog

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
)

// homeRecentMaximum bounds a Recently Added shelf, as the owner maximum bounds
// Continue Watching: the shelf, its total and its pages cover the newest works
// only. The whole library, newest first, is the library's own browse view.
const homeRecentMaximum = 100

// recentClass names a restricted viewer's published visibility class, whose
// rows carry each visible work's newest addition (compact_visibility_recent).
type recentClass struct{ id, generation int64 }

// homeRecentWorkKeySQL folds editions and members into works: a show, an
// album, a book, or one film across its editions. It is the compact
// equivalent of the recommendation engine's work key, inlined here so this
// walk does not depend on another lane's rewrite.
func homeRecentWorkKeySQL(i string) string {
	return `CASE ` + i + `.kind
	 WHEN 4 THEN 'show:'||(SELECT pid(parent.public_id) FROM catalog_episodes e JOIN catalog_entities parent ON parent.id=e.show_id WHERE e.entity_id=` + i + `.id)
	 WHEN 7 THEN 'album:'||(SELECT pid(parent.public_id) FROM catalog_songs sg JOIN catalog_entities parent ON parent.id=sg.album_id WHERE sg.entity_id=` + i + `.id)
	 WHEN 9 THEN 'book:'||(SELECT pid(parent.public_id) FROM catalog_book_files f JOIN catalog_entities parent ON parent.id=f.book_id WHERE f.entity_id=` + i + `.id)
	 WHEN 1 THEN 'movie:'||COALESCE('tmdb:'||NULLIF((SELECT d.provider_id FROM metadata_details d WHERE d.item_id=` + i + `.id AND d.provider='tmdb' AND d.provider_id<>''),''),
	  'tvdb:'||NULLIF((SELECT d.provider_id FROM metadata_details d WHERE d.item_id=` + i + `.id AND d.provider='tvdb' AND d.provider_id<>''),''),
	  'anilist:'||NULLIF((SELECT d.provider_id FROM metadata_details d WHERE d.item_id=` + i + `.id AND d.provider='anilist' AND d.provider_id<>''),''),'i:'||pid(` + i + `.public_id))
	 ELSE 'item:'||pid(` + i + `.public_id) END`
}

// homeRecentPhases are the two walks of homeRecentSource. The first walks
// works newest first on catalog_browse_recent_works: a show, album or book by
// its newest available member (maintained by the catalogue worker), a
// standalone item by its own added time, so each row is a different work and
// the walk costs the works shown, never the episodes or songs under them.
// The second walks items with no added date, which sort last as they always
// did. Their arguments are (library, last added, last entity id, restriction
// arguments…, limit); each phase has its own restriction arguments.
//
// A restricted viewer with a published class walks the class's own rows by
// the same key, so it reads only works it may see (each rechecked against the
// current restrictions, as a class may lag a change by one refresh) and the
// shelf is complete however little of the library the viewer can see.
func homeRecentPhases(r HomeRequest, class *recentClass) ([]string, [][]any) {
	// Permission and availability must hold for the same member: a work's
	// member metrics carry an added time only while that member is available.
	memberRestriction, memberArgs := ItemRestrictionSQL("member.item_id", r.Restrictions)
	ownRestriction, ownArgs := ItemRestrictionSQL("br.entity_id", r.Restrictions)
	// A work's newest 64 available members are probed, so a denied show with
	// thousands of episodes costs 64 checks, not all of them.
	workRestriction := `(CASE WHEN i.kind IN(2,6,8) THEN EXISTS(SELECT 1 FROM (SELECT item_id FROM catalog_browse_member_metrics INDEXED BY catalog_browse_metric_added
	  WHERE entity_id=br.entity_id AND added IS NOT NULL ORDER BY added DESC LIMIT 64) member WHERE ` + memberRestriction + `) ELSE ` + ownRestriction + ` END)`
	workArgs := append(append([]any{}, memberArgs...), ownArgs...)
	itemRestriction, itemArgs := ItemRestrictionSQL("i.id", r.Restrictions)
	if class != nil {
		workRestriction, workArgs = EntityRestrictionSQL("br.entity_id", r.Restrictions)
	}
	// Each phase reads a raw window of its index and says per row whether the
	// viewer may see it; the walk advances over every row, so what a viewer
	// cannot see is inspected once and counted against the budget.
	window := `SELECT entity_id,recent_text FROM catalog_browse_rows INDEXED BY catalog_browse_recent_works
	  WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND recent_text IS NOT NULL AND (recent_text,entity_id)<(?,?)
	  ORDER BY recent_text DESC,entity_id DESC LIMIT ?`
	if class != nil {
		window = `SELECT entity_id,recent AS recent_text FROM compact_visibility_rows INDEXED BY compact_visibility_recent
	  WHERE class_id=` + strconv.FormatInt(class.id, 10) + ` AND generation=` + strconv.FormatInt(class.generation, 10) + `
	  AND library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND recent IS NOT NULL AND (recent,entity_id)<(?,?)
	  ORDER BY recent DESC,entity_id DESC LIMIT ?`
	}
	works := `SELECT br.recent_text,` + homeRecentWorkKeySQL("i") + `,pid(i.public_id),br.entity_id,i.retired=0 AND ` + workRestriction + `
	 FROM (` + window + `) br CROSS JOIN catalog_entities i ON i.id=br.entity_id
	 ORDER BY br.recent_text DESC,br.entity_id DESC`
	columns := `SELECT COALESCE(br.added_text,''),` + homeRecentWorkKeySQL("i") + `,
	 CASE i.kind WHEN 1 THEN pid(i.public_id)
	  WHEN 4 THEN (SELECT pid(parent.public_id) FROM catalog_episodes e JOIN catalog_entities parent ON parent.id=e.show_id WHERE e.entity_id=i.id)
	  WHEN 7 THEN (SELECT pid(parent.public_id) FROM catalog_songs sg JOIN catalog_entities parent ON parent.id=sg.album_id WHERE sg.entity_id=i.id)
	  WHEN 9 THEN (SELECT pid(parent.public_id) FROM catalog_book_files f JOIN catalog_entities parent ON parent.id=f.book_id WHERE f.entity_id=i.id)
	  ELSE pid(i.public_id) END,
	 br.entity_id,k.playable=1 AND i.kind<>11 AND i.retired=0
	  AND EXISTS(SELECT 1 FROM catalog_item_availability v WHERE v.entity_id=i.id AND v.available=1 AND v.retired=0)
	  AND ` + itemRestriction
	undated := columns + ` FROM (SELECT entity_id,added_text FROM catalog_browse_rows INDEXED BY catalog_browse_recent_undated
	  WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND item_id IS NOT NULL AND added_text IS NULL AND ''<=? AND entity_id<?
	  ORDER BY entity_id DESC LIMIT ?) br CROSS JOIN catalog_entities i ON i.id=br.entity_id JOIN catalog_kinds k ON k.id=i.kind ORDER BY br.entity_id DESC`
	return []string{works, undated}, [][]any{workArgs, itemArgs}
}

// homeRecentSource walks the library's items newest first on items_home_recent,
// folds them into works (a show, an album, a book, or one film across its
// editions) and stops once homeRecentMaximum works are found. It never
// materialises the library (ARCH-SRV-04): the cost is the items above the
// last shown work.
func (s *Service) homeRecentSource(r HomeRequest, library string) (homeSource, error) {
	values, err := s.homeRecentWorks(r, library)
	if err != nil {
		return homeSource{}, err
	}
	return homeRecentList(values), nil
}

// homeRecentList is a recent shelf as a row source: (id, order key) pairs the
// row pages by keyset.
func homeRecentList(values [][2]string) homeSource {
	raw, _ := json.Marshal(values)
	n := len(values)
	return homeSource{
		base:           `SELECT json_extract(value,'$[0]') AS id,json_extract(value,'$[1]') AS ord FROM json_each(?)`,
		args:           []any{string(raw)},
		total:          &n,
		selfRestricted: true,
		fingerprint:    fmt.Sprintf("%x", sha256.Sum256(raw)),
	}
}

// homeRecentWorks is one library's shelf as (entity, order key) pairs, newest
// first.
func (s *Service) homeRecentWorks(r HomeRequest, library string) ([][2]string, error) {
	// Dated items newest first; then, only while the shelf is still short,
	// items with no added date, which sort last as they always did.
	var class *recentClass
	if r.Restrictions.Active() {
		key, generation, ready, err := s.publishedVisibilityClass(library, r.Restrictions)
		if err != nil {
			return nil, err
		}
		if ready {
			class = &recentClass{generation: generation}
			if err = s.read().QueryRow(`SELECT id FROM compact_visibility_classes WHERE class_key=?`, key).Scan(&class.id); err != nil {
				return nil, err
			}
		}
	}
	phases, phaseArgs := homeRecentPhases(r, class)
	type work struct{ key, entity, added string }
	found := []work{}
	seen := map[string]int{}
	for phase, query := range phases {
		restrictionArgs := phaseArgs[phase]
		lastAdded, lastEntity := "\uffff", int64(1<<62)
		for len(found) < homeRecentMaximum {
			args := append(append([]any{}, restrictionArgs...), library, lastAdded, lastEntity, 256)
			rows, err := s.read().Query(query, args...)
			if err != nil {
				return nil, err
			}
			n := 0
			for rows.Next() {
				var added, key string
				var entity *string
				var ent int64
				var visible bool
				if err = rows.Scan(&added, &key, &entity, &ent, &visible); err != nil {
					rows.Close()
					return nil, err
				}
				n++
				lastAdded, lastEntity = added, ent
				if !visible || key == "" || entity == nil || *entity == "" {
					continue
				}
				if at, ok := seen[key]; ok {
					// A later-walked member of a work already shown: the
					// work's entity is its smallest member entity, as the
					// recommendation engine chooses it.
					if *entity < found[at].entity {
						found[at].entity = *entity
					}
					continue
				}
				if len(found) < homeRecentMaximum {
					seen[key] = len(found)
					found = append(found, work{key, *entity, added})
				}
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
			if n < 256 {
				break
			}
		}
	}
	values := make([][2]string, 0, len(found))
	ids := map[string]bool{}
	for _, w := range found {
		if ids[w.entity] {
			continue
		}
		ids[w.entity] = true
		values = append(values, [2]string{w.entity, w.added + "\x1f" + w.entity})
	}
	return values, nil
}
