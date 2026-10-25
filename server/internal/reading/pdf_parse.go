package reading

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"
	"unicode"

	"rsc.io/pdf"
)

// parsePDFText runs only in the isolated helper process. It reads the page
// content streams of the pinned parser; it does not read the Info dictionary,
// render pages, run OCR, execute actions, load external resources or resolve
// host files. The helper's RLIMIT_AS/RLIMIT_CPU bound every allocation and CPU
// path the parser can reach, so a decoded-work explosion kills only the child.
func parsePDFText(ctx context.Context, r io.ReaderAt, size int64) (count *int64, opening string, err error) {
	if size <= 0 || size > maxCountPDFSource {
		return nil, "", nil
	}
	reader := &pdfReader{ctx: ctx, source: r, size: size}
	defer func() {
		if recover() != nil {
			count, opening = nil, ""
			err = reader.err
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, "", err
	}
	document, e := pdf.NewReader(reader, size)
	if reader.err != nil {
		return nil, "", reader.err
	}
	if e != nil {
		return nil, "", nil
	}
	// Encrypted documents are not counted even when an empty user password would
	// let the parser decrypt them: no DRM/password path is part of this rule.
	if !document.Trailer().Key("Encrypt").IsNull() {
		return nil, "", nil
	}
	pages := document.NumPage()
	if pages <= 0 || pages > maxCountPDFPages {
		return nil, "", nil
	}
	var words int64
	var excerpt openingText
	inWord := false
	var previous pdf.Text
	havePrevious := false
	textBytes := 0
	for pageNumber := 1; pageNumber <= pages; pageNumber++ {
		if err = ctx.Err(); err != nil {
			return nil, "", err
		}
		page := document.Page(pageNumber)
		if page.V.IsNull() || !pdfPageSupported(page) {
			return nil, "", nil
		}
		content := page.Content()
		if reader.err != nil {
			return nil, "", reader.err
		}
		havePrevious = false
		inWord = false
		for _, fragment := range content.Text {
			if err = ctx.Err(); err != nil {
				return nil, "", err
			}
			text := fragment.S
			if text == "" {
				continue
			}
			if len(text) > maxCountPDFRun {
				return nil, "", nil
			}
			textBytes += len(text)
			if textBytes > maxCountPDFText {
				return nil, "", nil
			}
			if havePrevious && !pdfLineBreak(previous, fragment) {
				if pdfWordGap(previous, fragment) {
					inWord = false
					excerpt.add(" ")
				}
			} else {
				inWord = false
				excerpt.add(" ")
			}
			var supported = true
			words, inWord, supported = pdfCountRun(text, words, inWord)
			if !supported {
				return nil, "", nil
			}
			excerpt.add(text)
			previous, havePrevious = fragment, true
		}
	}
	if words <= 0 {
		return nil, "", nil
	}
	return &words, excerpt.text.String(), nil
}

// pdfReader returns the file bytes the parser asks for, coalescing small random
// reads into one bounded window. It retains the first real file/context failure;
// parser panics on malformed structure are not file failures.
type pdfReader struct {
	ctx    context.Context
	source io.ReaderAt
	size   int64

	mu     sync.Mutex
	buffer []byte
	base   int64
	length int
	err    error
}

func (p *pdfReader) ReadAt(dst []byte, off int64) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	if off < 0 || off >= p.size {
		return 0, io.EOF
	}
	if err := p.ctx.Err(); err != nil {
		p.record(err)
		return 0, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(dst) <= pdfReadWindow {
		if off >= p.base && off+int64(len(dst)) <= p.base+int64(p.length) {
			n := copy(dst, p.buffer[int(off-p.base):p.length])
			if n == len(dst) {
				return n, nil
			}
			return n, io.EOF
		}
		want := int64(pdfReadWindow)
		if off+want > p.size {
			want = p.size - off
		}
		if cap(p.buffer) < int(want) {
			p.buffer = make([]byte, want)
		} else {
			p.buffer = p.buffer[:want]
		}
		n, err := p.source.ReadAt(p.buffer, off)
		if err != nil && !errors.Is(err, io.EOF) {
			p.record(err)
			return 0, err
		}
		p.base, p.length = off, n
		if n == 0 {
			return 0, io.EOF
		}
		copied := copy(dst, p.buffer[:n])
		if copied < len(dst) {
			return copied, io.EOF
		}
		return copied, nil
	}
	n, err := p.source.ReadAt(dst, off)
	if err != nil && !errors.Is(err, io.EOF) {
		p.record(err)
	}
	return n, err
}

func (p *pdfReader) record(err error) {
	if p.err == nil && !errors.Is(err, io.EOF) {
		p.err = err
	}
}

// pdfPageSupported rejects encodings this extractor cannot decode completely.
// The pinned parser implements WinAnsi/MacRoman, Differences and ToUnicode/CMap
// simple fonts; Identity-H composite fonts (and unknown encoding kinds) would
// otherwise be counted from raw glyph-code bytes. A missing font dictionary is
// likewise ambiguous. The publication stays unknown rather than guessing.
func pdfPageSupported(page pdf.Page) bool {
	for _, name := range page.Fonts() {
		font := page.Font(name)
		if font.V.IsNull() {
			return false
		}
		encoding := font.V.Key("Encoding")
		switch encoding.Kind() {
		case pdf.Name:
			switch encoding.Name() {
			case "WinAnsiEncoding", "MacRomanEncoding":
			default:
				return false
			}
		case pdf.Dict, pdf.Null:
		default:
			return false
		}
	}
	return true
}

// pdfCountRun applies the same rule 1 tokenization as EPUB text: maximal
// Unicode letter/number/combining-mark runs containing a letter or number.
// Scripts that require dictionary segmentation are unsupported, so the whole
// publication stays unknown rather than receiving a whitespace guess.
func pdfCountRun(text string, words int64, inWord bool) (int64, bool, bool) {
	for _, r := range text {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar) {
			return words, inWord, false
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if !inWord {
				words++
				inWord = true
			}
		} else if !unicode.IsMark(r) {
			inWord = false
		}
	}
	return words, inWord, true
}

// pdfLineBreak reports a new visual line: a different baseline beyond half the
// smaller font size. Text fragments on one baseline may continue the same word.
func pdfLineBreak(previous, next pdf.Text) bool {
	tolerance := math.Max(0.5, 0.5*math.Min(math.Abs(previous.FontSize), math.Abs(next.FontSize)))
	return math.Abs(next.Y-previous.Y) > tolerance
}

// pdfWordGap reports a horizontal separation large enough to be a word space.
// Small gaps are kept inside the word so a word split across font runs is not
// counted twice; the heuristic uses only the page's real text geometry.
func pdfWordGap(previous, next pdf.Text) bool {
	advance := math.Max(previous.W, 0)
	tolerance := math.Max(0.5, 0.15*math.Max(math.Abs(previous.FontSize), math.Abs(next.FontSize)))
	return next.X > previous.X+advance+tolerance
}
