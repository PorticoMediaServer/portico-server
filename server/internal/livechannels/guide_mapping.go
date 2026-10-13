package livechannels

import (
	"sort"
	"strings"
)

type guideChannelIndex struct{ ids, names map[string][]int }

func guideName(v string) string { return strings.ToLower(strings.Join(strings.Fields(v), " ")) }
func indexGuideChannels(channels []parsedChannel, explicit map[string]bool) guideChannelIndex {
	index := guideChannelIndex{ids: map[string][]int{}, names: map[string][]int{}}
	for i, ch := range channels {
		if explicit[ch.key] {
			continue
		}
		for _, key := range []string{ch.key, ch.guideKey, ch.number} {
			if key != "" {
				index.ids[key] = append(index.ids[key], i)
			}
		}
		key := guideName(ch.name)
		index.names[key] = append(index.names[key], i)
	}
	return index
}
func (index guideChannelIndex) match(id string, names []string) []int {
	candidates := append([]int(nil), index.ids[id]...)
	for _, name := range names {
		candidates = append(candidates, index.names[guideName(name)]...)
	}
	sort.Ints(candidates)
	unique := candidates[:0]
	for _, i := range candidates {
		if len(unique) == 0 || unique[len(unique)-1] != i {
			unique = append(unique, i)
		}
	}
	return unique
}
