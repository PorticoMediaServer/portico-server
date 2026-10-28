package livechannels

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"unicode"
)

// ChannelMark is authoritative within a source's complete authorized lineup.
// Filtering and page boundaries never change the collision domain.
type ChannelMark struct {
	Text string `json:"text"`
	Seed string `json:"seed"`
}
type markName struct{ source, id, name, number string }

func channelMarkBase(name string) (string, string) {
	words := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if len(words) == 0 {
		return "?", "?"
	}
	initials := ""
	for _, w := range words {
		r := []rune(w)
		initials += strings.ToUpper(string(r[0]))
	}
	base := initials
	if len([]rune(base)) > 2 {
		base = string([]rune(base)[:2])
	}
	if len(words) == 1 {
		r := []rune(strings.ToUpper(words[0]))
		base = string(r[:min(2, len(r))])
	}
	alternate := initials
	for _, w := range words {
		if unicode.IsNumber([]rune(w)[0]) {
			alternate = string([]rune(initials)[:1]) + w
			break
		}
	}
	return base, alternate
}
func channelMarks(names []markName) map[string]ChannelMark {
	sort.Slice(names, func(i, j int) bool {
		a, b := names[i], names[j]
		if a.source != b.source {
			return a.source < b.source
		}
		if a.name != b.name {
			return a.name < b.name
		}
		return a.id < b.id
	})
	baseCounts := map[string]int{}
	for _, n := range names {
		base, _ := channelMarkBase(n.name)
		baseCounts[n.source+"\x00"+base]++
	}
	proposed := make([]string, len(names))
	counts := map[string]int{}
	for i, n := range names {
		base, alternate := channelMarkBase(n.name)
		if baseCounts[n.source+"\x00"+base] > 1 && alternate != "" {
			base = alternate
		}
		proposed[i] = base
		counts[n.source+"\x00"+base]++
	}
	out := map[string]ChannelMark{}
	used := map[string]bool{}
	for i, n := range names {
		mark := proposed[i]
		if counts[n.source+"\x00"+mark] > 1 {
			suffix := n.number
			if suffix == "" {
				suffix = n.id
			}
			mark += " " + suffix
		}
		key := n.source + "\x00" + mark
		if used[key] {
			mark += " " + n.id
		}
		used[n.source+"\x00"+mark] = true
		out[n.source+"\x00"+n.id] = ChannelMark{Text: mark, Seed: n.source + ":" + n.id}
	}
	return out
}
func directoryMarks(ctx context.Context, tx *sql.Tx, scope DirectoryScope, q DirectoryQuery, channels []Channel) error {
	q.Search = ""
	q.Group = ""
	q.FavoritesOnly = false
	q.SourceID = ""
	q.Kind = "all"
	q.IncludeHidden = false
	// Owners retain the same collision domain when revealing server-hidden rows.
	q.IncludeServerHidden = scope.Owner
	rows, err := tx.QueryContext(ctx, directoryEligibleSQL+`SELECT source_id,channel_id,name,number FROM eligible ORDER BY source_id,channel_id`, directoryArgs(scope, q)...)
	if err != nil {
		return err
	}
	names := []markName{}
	for rows.Next() {
		var n markName
		if err = rows.Scan(&n.source, &n.id, &n.name, &n.number); err != nil {
			break
		}
		names = append(names, n)
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return err
	}
	marks := channelMarks(names)
	for i := range channels {
		if mark, ok := marks[channels[i].SourceID+"\x00"+channels[i].ID]; ok {
			channels[i].Mark = &mark
		}
	}
	return nil
}
