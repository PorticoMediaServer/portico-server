package playbackv1

import (
	"context"
	"database/sql"
)

// A span refers either to the fixed shuffle permutation or to appended source
// positions. Editing splits ranges, never expands millions of queue entries.
type orderSpan struct {
	source       bool
	start, count int64
}

func (l *layout) sourceTotal() int64 {
	var n int64
	for _, g := range l.segments {
		n += g.count
	}
	return n
}
func (l *layout) initOrder() {
	if len(l.spans) > 0 {
		return
	}
	if l.q.domain > 0 {
		l.spans = append(l.spans, orderSpan{start: 0, count: l.q.domain})
	}
	if n := l.sourceTotal() - l.q.domain; n > 0 {
		l.spans = append(l.spans, orderSpan{source: true, start: l.q.domain, count: n})
	}
}
func (l *layout) insertOrder(pos int64, v orderSpan) {
	l.initOrder()
	out := []orderSpan{}
	for _, s := range l.spans {
		if pos >= 0 && pos <= s.count {
			if pos > 0 {
				out = append(out, orderSpan{s.source, s.start, pos})
			}
			out = append(out, v)
			if pos < s.count {
				out = append(out, orderSpan{s.source, s.start + pos, s.count - pos})
			}
			pos = -1
		} else {
			out = append(out, s)
			if pos >= 0 {
				pos -= s.count
			}
		}
	}
	if pos == 0 {
		out = append(out, v)
	}
	l.spans = out
}
func (l *layout) removeOrder(pos int64) {
	l.initOrder()
	out := []orderSpan{}
	for _, s := range l.spans {
		if pos >= 0 && pos < s.count {
			if pos > 0 {
				out = append(out, orderSpan{s.source, s.start, pos})
			}
			if pos+1 < s.count {
				out = append(out, orderSpan{s.source, s.start + pos + 1, s.count - pos - 1})
			}
			pos = -1
		} else {
			out = append(out, s)
			if pos >= 0 {
				pos -= s.count
			}
		}
	}
	l.spans = out
}
func (l *layout) saveOrder(ctx context.Context, tx *sql.Tx) error {
	if len(l.spans) > MaxQueueSegments {
		return ErrQueueTooLarge
	}
	if _, e := tx.ExecContext(ctx, `DELETE FROM queue_v1_play_spans WHERE queue_id=?`, l.q.id); e != nil {
		return e
	}
	for i, s := range l.spans {
		if s.count <= 0 {
			continue
		}
		if _, e := tx.ExecContext(ctx, `INSERT INTO queue_v1_play_spans(queue_id,rank,source,start,count)VALUES(?,?,?,?,?)`, l.q.id, i, s.source, s.start, s.count); e != nil {
			return e
		}
	}
	return nil
}
func (l *layout) baseOrderToSource(p int64) int64 {
	sh := l.shuffle()
	if sh == nil || p >= l.q.domain {
		return p
	}
	if p == 0 {
		return l.q.first
	}
	c, _ := sh.IndexOf(uint64(l.q.first))
	k := uint64(p)
	if uint64(p-1) < c {
		k = uint64(p - 1)
	}
	at, _ := sh.At(k)
	return int64(at)
}
func (l *layout) baseSourceToOrder(src int64) int64 {
	sh := l.shuffle()
	if sh == nil || src >= l.q.domain {
		return src
	}
	if src == l.q.first {
		return 0
	}
	c, _ := sh.IndexOf(uint64(l.q.first))
	k, _ := sh.IndexOf(uint64(src))
	if k < c {
		return int64(k) + 1
	}
	return int64(k)
}
