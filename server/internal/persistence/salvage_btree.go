package persistence

import (
	"context"
	"encoding/binary"
	"math"

	"portico.local/server/internal/dbwork"
)

// A damaged table cannot be read past its damage in key order, but SQLite can
// still seek to any leaf whose own page and whose path from the root are
// intact. leafRanges finds those leaves by reading the table's b-tree pages
// directly (sqlite_dbpage): an interior page lists its children and the keys
// that separate them, which bound each leaf exactly. Only child page numbers
// and separator keys are decoded; rows are always read back through SQL. A
// page that cannot be parsed contributes its whole key range as one range,
// which SQL then reads or fails to read as a unit.

// keyRange bounds one leaf (lo exclusive; hi exclusive, or inclusive when it
// ends in posInf), or one separator row (both ends inclusive). A separator of
// a WITHOUT ROWID table is a whole row, decoded from its interior page: row
// holds it, since reading it back by key may first descend into its left
// child.
type keyRange struct {
	lo, hi []any
	row    []any
}

const (
	pageIndexInterior = 0x02
	pageTableInterior = 0x05
	pageIndexLeaf     = 0x0a
	pageTableLeaf     = 0x0d
	btreeMaxDepth     = 32
)

type btreeReader struct {
	ctx      context.Context
	db       *dbwork.Candidate
	usable   int
	keys     int
	rowid    bool
	visited  map[int64]bool
	ranges   []keyRange
	maxPages int64
}

func leafRanges(ctx context.Context, db *dbwork.Candidate, table string, keys int, rowid bool) ([]keyRange, error) {
	var root, pageSize, pages int64
	if err := db.QueryRowContext(ctx, `SELECT rootpage FROM damaged.sqlite_schema WHERE type='table' AND name=?`, table).Scan(&root); err != nil {
		return nil, nil // nothing more can be found without the schema entry
	}
	if err := db.QueryRowContext(ctx, `PRAGMA damaged.page_size`).Scan(&pageSize); err != nil {
		return nil, err
	}
	if err := db.QueryRowContext(ctx, `PRAGMA damaged.page_count`).Scan(&pages); err != nil {
		return nil, err
	}
	first, err := dbPage(ctx, db, 1)
	if err != nil || len(first) < 100 {
		return nil, nil
	}
	r := &btreeReader{ctx: ctx, db: db, usable: int(pageSize) - int(first[20]), keys: keys, rowid: rowid, visited: map[int64]bool{}, maxPages: pages}
	if err = r.walk(root, nil, nil, 0); err != nil {
		return nil, err
	}
	return r.ranges, nil
}

func dbPage(ctx context.Context, db *dbwork.Candidate, page int64) ([]byte, error) {
	var data []byte
	err := db.QueryRowContext(ctx, `SELECT data FROM sqlite_dbpage('damaged') WHERE pgno=?`, page).Scan(&data)
	return data, err
}

// walk records the leaves under page, whose keys lie between lo and hi.
func (r *btreeReader) walk(page int64, lo, hi []any, depth int) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	whole := func() error { r.ranges = append(r.ranges, keyRange{lo: lo, hi: hi}); return nil }
	if page < 1 || page > r.maxPages || r.visited[page] || depth > btreeMaxDepth {
		return whole()
	}
	r.visited[page] = true
	data, err := dbPage(r.ctx, r.db, page)
	if err != nil {
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}
		return whole()
	}
	head := 0
	if page == 1 {
		head = 100
	}
	if len(data) < head+12 {
		return whole()
	}
	kind := data[head]
	switch {
	case kind == pageTableLeaf && r.rowid, kind == pageIndexLeaf && !r.rowid:
		return whole()
	case kind == pageTableInterior && r.rowid, kind == pageIndexInterior && !r.rowid:
	default:
		return whole() // not this tree's page shape: let SQL try the range
	}
	cells := int(binary.BigEndian.Uint16(data[head+3:]))
	right := int64(binary.BigEndian.Uint32(data[head+8:]))
	if head+12+2*cells > len(data) {
		return whole()
	}
	children := make([]int64, 0, cells+1)
	separators := make([][]any, 0, cells)
	var rows [][]any
	for i := 0; i < cells; i++ {
		at := int(binary.BigEndian.Uint16(data[head+12+2*i:]))
		if at+4 > len(data) {
			return whole()
		}
		child := int64(binary.BigEndian.Uint32(data[at:]))
		var key []any
		if r.rowid {
			rowid, n := varint(data[at+4:])
			if n == 0 {
				return whole()
			}
			key = []any{int64(rowid)}
		} else {
			record, ok := r.payload(data, at+4)
			if !ok {
				return whole()
			}
			values, ok := decodeRecord(record)
			if !ok || len(values) < r.keys {
				return whole()
			}
			key = values[:r.keys]
			rows = append(rows, values)
		}
		children = append(children, child)
		separators = append(separators, key)
	}
	low := lo
	for i, child := range children {
		key := separators[i]
		if r.rowid {
			// A table interior key is the largest rowid in its left child.
			if err = r.walk(child, low, append(append([]any{}, key...), posInf), depth+1); err != nil {
				return err
			}
		} else {
			// An index interior key is itself a row, between its two children.
			if err = r.walk(child, low, key, depth+1); err != nil {
				return err
			}
			r.ranges = append(r.ranges, keyRange{lo: append(append([]any{}, key...), negInf), hi: append(append([]any{}, key...), posInf), row: rows[i]})
		}
		low = key
	}
	return r.walk(right, low, hi, depth+1)
}

// payload returns the record of an index interior cell whose payload size
// starts at at, following its overflow pages when it does not fit.
func (r *btreeReader) payload(data []byte, at int) ([]byte, bool) {
	size, n := varint(data[at:])
	if n == 0 || size > 1<<24 {
		return nil, false
	}
	at += n
	total := int(size)
	maxLocal := (r.usable-12)*64/255 - 23
	minLocal := (r.usable-12)*32/255 - 23
	local := total
	if total > maxLocal {
		local = minLocal + (total-minLocal)%(r.usable-4)
		if local > maxLocal {
			local = minLocal
		}
	}
	if at+local > len(data) {
		return nil, false
	}
	out := append(make([]byte, 0, total), data[at:at+local]...)
	if local == total {
		return out, true
	}
	if at+local+4 > len(data) {
		return nil, false
	}
	next := int64(binary.BigEndian.Uint32(data[at+local:]))
	for hops := 0; len(out) < total && next != 0 && hops < 1<<16; hops++ {
		page, err := dbPage(r.ctx, r.db, next)
		if err != nil || len(page) < r.usable {
			return nil, false
		}
		take := min(total-len(out), r.usable-4)
		out = append(out, page[4:4+take]...)
		next = int64(binary.BigEndian.Uint32(page))
	}
	return out, len(out) == total
}

// varint decodes SQLite's big-endian variable-length integer; n is 0 when
// the bytes run out.
func varint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < 9; i++ {
		if i >= len(b) {
			return 0, 0
		}
		if i == 8 {
			return v<<8 | uint64(b[i]), 9
		}
		v = v<<7 | uint64(b[i]&0x7f)
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return v, 9
}

// decodeRecord decodes a record in SQLite's record format.
func decodeRecord(record []byte) ([]any, bool) {
	size, n := varint(record)
	if n == 0 || size > uint64(len(record)) {
		return nil, false
	}
	var types []uint64
	for at := n; at < int(size); {
		t, m := varint(record[at:])
		if m == 0 {
			return nil, false
		}
		types = append(types, t)
		at += m
	}
	values := make([]any, 0, len(types))
	body := record[size:]
	for _, t := range types {
		width := 0
		switch {
		case t == 0, t == 8, t == 9:
		case t >= 1 && t <= 4:
			width = int(t)
		case t == 5:
			width = 6
		case t == 6, t == 7:
			width = 8
		case t >= 12:
			width = int(t-12) / 2
		default:
			return nil, false
		}
		if width > len(body) {
			return nil, false
		}
		field := body[:width]
		body = body[width:]
		switch {
		case t == 0:
			values = append(values, nil)
		case t == 8:
			values = append(values, int64(0))
		case t == 9:
			values = append(values, int64(1))
		case t <= 6:
			var v int64
			if field[0]&0x80 != 0 {
				v = -1
			}
			for _, c := range field {
				v = v<<8 | int64(c)
			}
			values = append(values, v)
		case t == 7:
			values = append(values, math.Float64frombits(binary.BigEndian.Uint64(field)))
		case t%2 == 0:
			values = append(values, append([]byte{}, field...))
		default:
			values = append(values, string(field))
		}
	}
	return values, true
}
