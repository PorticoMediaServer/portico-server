package reading

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"io"
	"unicode"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

const readingTextRuleVersion = 1
const maxCountDocument = 8 << 20
const maxCountPublication = 64 << 20

// Count only complete admitted XHTML spine text. A nil count is unknown, not
// zero. This is descriptive metadata; no source or viewer grant comes from it.
func countEPUBText(ctx context.Context, e *EPUB) (*int64, error) {
	var total, words int64
	for _, name := range e.Spine {
		file := e.archive.files[name]
		if file == nil || e.allowed[name] != "application/xhtml+xml" || file.UncompressedSize64 > maxCountDocument || file.UncompressedSize64 > uint64(maxCountPublication-total) {
			return nil, nil
		}
		data, err := e.archive.read(ctx, name, maxCountDocument)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Only complete acquired bytes can establish malformed/unsupported text.
			// Even a length/format refusal here may wrap a truncated provider read.
			return nil, err
		}
		total += int64(len(data))
		count, ok := countXHTMLText(ctx, data)
		if !ok {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, nil
		}
		words += count
	}
	return &words, nil
}

// Rule 1: count maximal Unicode letter/number/mark runs containing a letter or
// number, in XHTML body character data. Inline markup preserves runs; element
// boundaries for block/line containers separate them. Script/style/template and
// head content are not publication text. Scripts requiring dictionary word
// segmentation are unsupported, rather than receiving a whitespace guess.
func countXHTMLText(ctx context.Context, data []byte) (int64, bool) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	const xhtml = "http://www.w3.org/1999/xhtml"
	depth, body, skip := 0, 0, 0
	root, closed, bodySeen := false, false, false
	var words int64
	inWord := false
	boundary := func() { inWord = false }
	for {
		if ctx.Err() != nil {
			return 0, false
		}
		token, err := d.Token()
		if err == io.EOF {
			return words, root && closed && bodySeen && depth == 0
		}
		if err != nil {
			return 0, false
		}
		switch v := token.(type) {
		case xml.Directive:
			return 0, false
		case xml.StartElement:
			depth++
			if depth > 256 || closed || v.Name.Space != xhtml {
				return 0, false
			}
			if !root {
				if depth != 1 || v.Name.Local != "html" {
					return 0, false
				}
				root = true
			}
			if v.Name.Local == "body" {
				if depth != 2 || bodySeen {
					return 0, false
				}
				body = depth
				bodySeen = true
			}
			if skip == 0 && (v.Name.Local == "script" || v.Name.Local == "style" || v.Name.Local == "template") {
				skip = depth
				boundary()
			}
			if textBlock(v.Name.Local) {
				boundary()
			}
		case xml.EndElement:
			if textBlock(v.Name.Local) {
				boundary()
			}
			if depth == skip {
				skip = 0
				boundary()
			}
			if depth == body {
				body = 0
				boundary()
			}
			depth--
			if depth == 0 {
				closed = true
			}
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(v)) != 0 {
				return 0, false
			}
			if body == 0 || skip != 0 {
				continue
			}
			for _, r := range string(v) {
				if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar) {
					return 0, false
				}
				if unicode.IsLetter(r) || unicode.IsNumber(r) {
					if !inWord {
						words++
						inWord = true
					}
				} else if !unicode.IsMark(r) {
					boundary()
				}
			}
		}
	}
}
func textBlock(name string) bool {
	switch name {
	case "address", "article", "aside", "blockquote", "br", "dd", "div", "dl", "dt", "figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "ol", "p", "pre", "section", "table", "td", "th", "tr", "ul":
		return true
	}
	return false
}

// Reuse one exact admitted source; never acquire a second version or retain an
// actor credential for background work. Cache only this row/revision/rule.
func (s *Service) adoptTextCount(ctx context.Context, p identity.Principal, f *Source) error {
	selected := f.Selection
	if selected.Format != "epub" {
		return nil
	}
	g, err := dbwork.BeginSnapshot(ctx, s.DB)
	if err != nil {
		return err
	}
	var cached int
	err = g.Tx().QueryRowContext(ctx, `SELECT 1 FROM reading_text_counts WHERE item_id=? AND asset_id=? AND inventory_object_id=? AND source_revision=? AND rule_version=?`, selected.Entity, selected.Asset, selected.inventoryRow, selected.inventory, readingTextRuleVersion).Scan(&cached)
	g.Rollback()
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	e, err := f.epub()
	if err != nil {
		return err
	}
	count, err := countEPUBText(ctx, e)
	if err != nil {
		return err
	}
	if err = f.Validate(); err != nil {
		return err
	}
	gate, err := dbwork.Begin(ctx, s.DB, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer gate.Rollback()
	current, err := s.selectTx(ctx, gate.Tx(), p, selected.ItemID, selected.SourceID)
	if err != nil {
		return err
	}
	if current.Revision != selected.Revision || current.inventoryRow != selected.inventoryRow || current.object != selected.object || current.inventory != selected.inventory {
		return ErrChanged
	}
	_, err = gate.Tx().ExecContext(ctx, `INSERT INTO reading_text_counts(item_id,asset_id,word_count,inventory_object_id,source_revision,rule_version) VALUES(?,?,?,?,?,?) ON CONFLICT(item_id,asset_id) DO UPDATE SET word_count=excluded.word_count,inventory_object_id=excluded.inventory_object_id,source_revision=excluded.source_revision,rule_version=excluded.rule_version WHERE reading_text_counts.word_count IS NOT excluded.word_count OR reading_text_counts.inventory_object_id<>excluded.inventory_object_id OR reading_text_counts.source_revision<>excluded.source_revision OR reading_text_counts.rule_version<>excluded.rule_version`, current.Entity, current.Asset, count, current.inventoryRow, current.inventory, readingTextRuleVersion)
	if err != nil {
		return err
	}
	return gate.Commit()
}
