package livechannels

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// SearchProgramme is one real guide title on one channel. MoreAirings is a
// lower bound when the bounded sibling window did not reach that title's end.
type SearchProgramme struct {
	Airing           Airing
	MoreAirings      int
	MoreAiringsExact bool
}
type ProgrammeSearchPage struct {
	Items                            []SearchProgramme
	NextCursor, Revision, Start, End string
}
type programmeSearchCursor struct {
	Binding, Revision, Fence, Key, ChannelKey, Title, WithinStart, WithinID string
	Start, End                                                              string
	Expires                                                                 int64
}

// SearchProgrammes walks guide title groups rather than loading a guide into
// client memory. An empty result window can still carry a continuation. Current
// directory authority is resolved in the same transaction as programme facts.
func (s *Store) SearchProgrammes(ctx context.Context, a DirectoryAuthority, project AiringProjection, q string, limit int, cursor string, allowed func(string) bool) (ProgrammeSearchPage, error) {
	out := ProgrammeSearchPage{Items: []SearchProgramme{}}
	if len(q) > 512 || limit < 1 || limit > 40 {
		return out, ErrInvalid
	}
	binding := mustJSON([]any{q, limit})
	start := time.Now().UTC().Truncate(time.Minute)
	c := programmeSearchCursor{Binding: binding, Start: start.Format(time.RFC3339), End: start.Add(14 * 24 * time.Hour).Format(time.RFC3339), Expires: time.Now().Add(30 * time.Minute).Unix()}
	if cursor != "" {
		if err := s.readGuideValue(cursor, "search-programmes", &c); err != nil {
			return out, err
		}
		if c.Binding != binding || c.Expires <= time.Now().Unix() {
			return out, ErrCursor
		}
	}
	err := s.directorySnapshot(ctx, a, func(tx *sql.Tx, scope DirectoryScope) error {
		dq := DirectoryQuery{Kind: "all", Sort: "server", Limit: 50}
		rev, err := s.directoryRevision(ctx, tx, scope, dq)
		if err != nil {
			return err
		}
		if cursor != "" && (c.Revision != rev || c.Fence != scope.Fence) {
			return ErrCursor
		}
		c.Revision, c.Fence = rev, scope.Fence
		out.Revision = rev
		out.Start, out.End = c.Start, c.End
		// Select only a bounded group-key window before reading descriptions/facts.
		// The source window and title indexes serve the two branches independently.
		st, _ := time.Parse(time.RFC3339, c.Start)
		en, _ := time.Parse(time.RFC3339, c.End)
		groupsSQL := directoryEligibleSQL + `, programme_groups AS (
   SELECT lower(e.name)||':'||e.provenance||':'||e.source_id||':'||e.channel_id||':'||j.value k,e.generation,e.channel_id,j.value title,e.provenance
   FROM eligible e CROSS JOIN json_each((SELECT json_group_array(title) FROM (
    SELECT lower(p.title) title FROM live_programmes p INDEXED BY live_programmes_search_group
    WHERE p.generation_id=e.generation AND p.channel_id=e.channel_id AND p.start_utc<? AND p.end_utc>?
    AND (lower(e.name)||':'||e.provenance||':'||e.source_id||':'||e.channel_id>? OR(lower(p.title)>? OR(lower(p.title)=? AND ?<>'')))
    GROUP BY lower(p.title) ORDER BY lower(p.title) LIMIT ?))) j
   WHERE e.provenance='live-source' AND lower(e.name)||':'||e.provenance||':'||e.source_id||':'||e.channel_id>=?
   UNION ALL
   SELECT lower(e.name)||':'||e.provenance||':'||e.source_id||':'||e.channel_id||':'||j.value,e.generation,e.channel_id,j.value,e.provenance
   FROM eligible e CROSS JOIN json_each((SELECT json_group_array(title) FROM (
    SELECT lower(p.title) title FROM lc_entries p INDEXED BY lc_entries_search_group
    WHERE p.generation_id=e.generation AND p.channel_id=e.channel_id AND p.start_ms<? AND p.end_ms>?
    AND (lower(e.name)||':'||e.provenance||':'||e.source_id||':'||e.channel_id>? OR(lower(p.title)>? OR(lower(p.title)=? AND ?<>'')))
    GROUP BY lower(p.title) ORDER BY lower(p.title) LIMIT ?))) j
   WHERE e.provenance='library-channel' AND lower(e.name)||':'||e.provenance||':'||e.source_id||':'||e.channel_id>=?
  ) SELECT k,generation,channel_id,title,provenance FROM programme_groups ORDER BY k LIMIT ?`
		args := append(directoryArgs(scope, dq), c.End, c.Start, c.ChannelKey, c.Title, c.Title, c.WithinStart, limit+1, c.ChannelKey, en.UnixMilli(), st.UnixMilli(), c.ChannelKey, c.Title, c.Title, c.WithinStart, limit+1, c.ChannelKey, limit+1)
		rows, err := tx.QueryContext(ctx, groupsSQL, args...)
		if err != nil {
			return err
		}
		type group struct{ key, generation, channel, title, kind string }
		groups := []group{}
		for rows.Next() {
			var g group
			if err = rows.Scan(&g.key, &g.generation, &g.channel, &g.title, &g.kind); err != nil {
				break
			}
			if g.key == c.Key && c.WithinStart == "" {
				continue
			}
			groups = append(groups, g)
		}
		if e := rows.Err(); err == nil {
			err = e
		}
		rows.Close()
		if err != nil {
			return err
		}
		// +1 is lookahead only. Every selected group contributes at most 129
		// airings to this one batch query. No per-card SQL or HTTP fanout.
		moreGroups := len(groups) > limit
		if moreGroups {
			groups = groups[:limit]
		}
		request := []any{}
		for _, g := range groups {
			after, afterID := "", ""
			if g.key == c.Key {
				after, afterID = c.WithinStart, c.WithinID
			}
			request = append(request, []string{g.key, g.generation, g.channel, g.title, g.kind, after, afterID})
		}
		if len(request) == 0 {
			return nil
		}
		// Correlated indexed bounded key selection stays inside a single statement.
		// Joining full airing facts happens only for those selected keys.
		pageSQL := directoryEligibleSQL + airingCandidatesSQL + `, requested AS (SELECT json_extract(value,'$[0]') k,json_extract(value,'$[1]') gen,json_extract(value,'$[2]') channel,json_extract(value,'$[3]') title,json_extract(value,'$[4]') kind,json_extract(value,'$[5]') after_start,json_extract(value,'$[6]') after_id FROM json_each(?)), selected_keys AS MATERIALIZED (
   SELECT r.k,r.gen,r.channel,r.kind,j.value id FROM requested r CROSS JOIN json_each((SELECT json_group_array(id) FROM (
    SELECT p.id FROM live_programmes p INDEXED BY live_programmes_search_group WHERE p.generation_id=r.gen AND p.channel_id=r.channel AND lower(p.title)=r.title AND p.start_utc<? AND p.end_utc>? AND (p.start_utc>r.after_start OR(p.start_utc=r.after_start AND p.id>r.after_id)) ORDER BY p.start_utc,p.id LIMIT 129))) j WHERE r.kind='live-source'
   UNION ALL
   SELECT r.k,r.gen,r.channel,r.kind,j.value FROM requested r CROSS JOIN json_each((SELECT json_group_array(id) FROM (
    SELECT p.occurrence_id id FROM lc_entries p INDEXED BY lc_entries_search_group WHERE p.generation_id=r.gen AND p.channel_id=r.channel AND lower(p.title)=r.title AND p.start_ms<? AND p.end_ms>? AND (strftime('%Y-%m-%dT%H:%M:%SZ',p.start_ms/1000,'unixepoch')>r.after_start OR(strftime('%Y-%m-%dT%H:%M:%SZ',p.start_ms/1000,'unixepoch')=r.after_start AND p.occurrence_id>r.after_id)) ORDER BY p.start_ms,p.occurrence_id LIMIT 129))) j WHERE r.kind='library-channel'
  ) SELECT r.k,` + "a." + strings.ReplaceAll(airingColumns, ",", ",a.") + ` FROM selected_keys r CROSS JOIN airings a ON a.generation=r.gen AND a.channel_id=r.channel AND a.provenance=r.kind AND a.id=r.id ORDER BY r.k,start_utc,a.id`
		args = append(directoryArgs(scope, dq), mustJSON(request), c.End, c.Start, en.UnixMilli(), st.UnixMilli())
		rows, err = tx.QueryContext(ctx, pageSQL, args...)
		if err != nil {
			return err
		}
		batches := map[string][]Airing{}
		for rows.Next() {
			var key, facts string
			var v Airing
			p, ch := &v.Programme, &v.Channel
			if err = rows.Scan(&key, &ch.ID, &ch.SourceID, &ch.Provenance, &ch.Name, &ch.Number, &ch.Group, &ch.Generation, &p.ID, &p.Title, &p.Start, &p.End, &p.Lineage, &p.SeriesID, &p.EpisodeID, &p.NewEvidence, &p.Description, &facts, &p.Image); err != nil {
				break
			}
			p.ChannelID = ch.ID
			p.ApplyFacts(facts)
			ch.Programmes = []Programme{}
			batches[key] = append(batches[key], v)
		}
		if e := rows.Err(); err == nil {
			err = e
		}
		rows.Close()
		if err != nil {
			return err
		}
		for _, g := range groups {
			batch := batches[g.key]
			truncated := len(batch) > 128
			if truncated {
				batch = batch[:128]
			}
			if g.kind == string(LibraryChannel) {
				if project == nil {
					batch = nil
				} else {
					batch, err = project(ctx, tx, batch)
					if err != nil {
						return err
					}
				}
			}
			matches := []Airing{}
			for _, v := range batch {
				rating := ""
				if v.Programme.Rating != nil {
					rating = v.Programme.Rating.Value
				}
				if allowed != nil && !allowed(rating) {
					continue
				}
				if strings.Contains(strings.ToLower(v.Programme.Title+" "+v.Programme.Subtitle), strings.ToLower(q)) {
					matches = append(matches, v)
				}
			}
			c.Key = g.key
			c.ChannelKey = strings.TrimSuffix(g.key, ":"+g.title)
			c.Title = g.title
			c.WithinStart, c.WithinID = "", ""
			if len(matches) > 0 {
				out.Items = append(out.Items, SearchProgramme{Airing: matches[0], MoreAirings: len(matches) - 1, MoreAiringsExact: !truncated})
			} else if truncated {
				// Continue an incomplete group even if every scanned airing was hidden or
				// did not match. Never infer exhaustion from a short/empty visible page.
				raw := batches[g.key][127]
				c.WithinStart, c.WithinID = raw.Programme.Start, raw.Programme.ID
				out.NextCursor = s.signGuideValue(c, "search-programmes")
				break
			}
		}
		if out.NextCursor == "" && moreGroups {
			out.NextCursor = s.signGuideValue(c, "search-programmes")
		}
		return nil
	})
	if err != nil {
		return ProgrammeSearchPage{}, err
	}
	return out, nil
}

type ChannelSearchPage struct {
	Items                []Channel
	NextCursor, Revision string
	Total                int
}
type channelSearchCursor struct {
	Binding, Revision, Fence, Value, ID string
	Expires                             int64
}

func (s *Store) SearchChannels(ctx context.Context, a DirectoryAuthority, q, sortID, direction string, limit int, cursor string) (ChannelSearchPage, error) {
	out := ChannelSearchPage{Items: []Channel{}}
	if len(q) > 512 || limit < 1 || limit > 40 || (direction != "asc" && direction != "desc") {
		return out, ErrInvalid
	}
	binding := mustJSON([]any{q, sortID, direction, limit})
	c := channelSearchCursor{Binding: binding, Expires: time.Now().Add(30 * time.Minute).Unix()}
	if cursor != "" {
		if e := s.readGuideValue(cursor, "search-channels", &c); e != nil {
			return out, e
		}
		if c.Binding != binding || c.Expires <= time.Now().Unix() {
			return out, ErrCursor
		}
	}
	err := s.directorySnapshot(ctx, a, func(tx *sql.Tx, scope DirectoryScope) error {
		dq := DirectoryQuery{Kind: "all", Sort: "server", Limit: 50}
		rev, err := s.directoryRevision(ctx, tx, scope, dq)
		if err != nil {
			return err
		}
		if cursor != "" && (c.Revision != rev || c.Fence != scope.Fence) {
			return ErrCursor
		}
		c.Revision, c.Fence = rev, scope.Fence
		out.Revision = rev
		where := ` WHERE instr(lower(name),lower(?))>0`
		args := append(directoryArgs(scope, dq), q)
		if err = tx.QueryRowContext(ctx, directoryEligibleSQL+`SELECT count(*) FROM eligible`+where, args...).Scan(&out.Total); err != nil {
			return err
		}
		order := `lower(name)`
		if sortID == "relevance" {
			order = `printf('%04d',CASE WHEN lower(name)=lower(?) THEN 1000 WHEN substr(lower(name),1,length(?))=lower(?) THEN 100 ELSE 0 END)||lower(name)`
			args = append(directoryArgs(scope, dq), q, q, q, q)
		} else {
			args = append(directoryArgs(scope, dq), q)
		}
		op := ">"
		if direction == "desc" {
			op = "<"
		}
		sql := directoryEligibleSQL + `, ranked AS (SELECT *,` + order + ` ord,provenance||':'||source_id||':'||channel_id tie FROM eligible` + where + `) SELECT channel_id,source_id,provenance,name,number,group_name,generation,ord,tie FROM ranked`
		if cursor != "" {
			sql += ` WHERE (ord` + op + `? OR(ord=? AND tie` + op + `?))`
			args = append(args, c.Value, c.Value, c.ID)
		}
		sql += ` ORDER BY ord ` + direction + `,tie ` + direction + ` LIMIT ?`
		args = append(args, limit+1)
		rows, err := tx.QueryContext(ctx, sql, args...)
		if err != nil {
			return err
		}
		values, ids := []string{}, []string{}
		for rows.Next() {
			var ch Channel
			var value, id string
			if err = rows.Scan(&ch.ID, &ch.SourceID, &ch.Provenance, &ch.Name, &ch.Number, &ch.Group, &ch.Generation, &value, &id); err != nil {
				break
			}
			ch.Programmes = []Programme{}
			out.Items = append(out.Items, ch)
			values = append(values, value)
			ids = append(ids, id)
		}
		if e := rows.Err(); err == nil {
			err = e
		}
		rows.Close()
		if err != nil {
			return err
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			c.Value, c.ID = values[limit-1], ids[limit-1]
			out.NextCursor = s.signGuideValue(c, "search-channels")
		}
		return nil
	})
	return out, err
}
