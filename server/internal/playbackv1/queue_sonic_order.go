package playbackv1

import "portico.local/server/internal/sonic"

// Spans keep their existing meaning: only the seeded base permutation maps
// through a captured row. Explicit source spans and appended tails stay intact.
func (l *layout) baseOrderToSource(position int64) int64 {
	if position >= 0 && position < l.q.domain && l.musicQueue() {
		if window, ok := l.sonicWindows[position/sonic.SmoothWindowMax]; ok {
			index := position % sonic.SmoothWindowMax
			if index < int64(len(window.ordinals)) {
				return window.ordinals[index]
			}
		}
	}
	return l.ordinaryOrderToSource(position)
}

func (l *layout) baseSourceToOrder(source int64) int64 {
	position := l.ordinarySourceToOrder(source)
	if position >= 0 && position < l.q.domain && l.musicQueue() {
		if window, ok := l.sonicWindows[position/sonic.SmoothWindowMax]; ok {
			for index, ordinal := range window.ordinals {
				if ordinal == source {
					return position/sonic.SmoothWindowMax*sonic.SmoothWindowMax + int64(index)
				}
			}
		}
	}
	return position
}
