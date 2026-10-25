package httpapi

import "portico.local/server/internal/reading"

type ReadingWordCount int64

func (ReadingWordCount) JSONSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 0, "maximum": 67108864}
}

// Only actual admitted comic-page names establish this server-side page count.
// PDF and EPUB screen-page counts remain the real client renderer's facts.
type ReadingPageCount int64

func (ReadingPageCount) JSONSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": reading.MaxEntries}
}

type ReadingOpeningLines string

func (ReadingOpeningLines) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}
}

// Called only after Prepare's original source/principal admission. Versions and
// catalogue lists do not receive this file-body excerpt through generic metadata.
func readingManifestFactsProjection(out *ReadingMetadata, m reading.Metadata) {
	if m.WordCount != nil {
		value := ReadingWordCount(*m.WordCount)
		out.WordCount = &value
	}
	if m.PageCount != nil {
		value := ReadingPageCount(*m.PageCount)
		out.PageCount = &value
	}
	out.OpeningLines = ReadingOpeningLines(m.OpeningLines)
	if m.Series != nil {
		out.Series = &ReadingSeries{ID: m.Series.ID, Name: m.Series.Name, Number: m.Series.Number, Source: ReadingAttributionSource(m.Series.Source)}
	}
}
