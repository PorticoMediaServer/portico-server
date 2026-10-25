package catalog

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
)

// Author and series pages publish the same bounded book rows as the existing
// books section. This adds only stored, viewer-visible facts to those rows:
// author, narrator, series position, a complete book duration and the viewer's
// own book resume. Every read is scoped to the page ids and the requesting
// profile; no other profile or reading endpoint is consulted.

type bookProjectionFacts struct {
	author, narrator, series string
	seriesPosition           string
	seriesIndex              float64
}

type bookTimelineRow struct {
	file      string
	permitted bool
	seconds   sql.NullFloat64
}

func (s *Service) enrichAuthorSeriesBooks(r ContentRequest, sec *ContentSection) error {
	if len(sec.Entries) == 0 {
		return nil
	}
	ids := make([]string, 0, len(sec.Entries))
	for i := range sec.Entries {
		ids = append(ids, sec.Entries[i].ID)
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	facts := map[string]bookProjectionFacts{}
	rows, err := s.read().Query(`SELECT pid(e.public_id),ctx.author,ctx.narrator,COALESCE(NULLIF(ctx.series_name,''),b.series),COALESCE(NULLIF(ctx.series_position,''),b.series_position),ctx.series_index
		FROM listening_book_context ctx JOIN catalog_entities e ON e.id=ctx.id LEFT JOIN catalog_books b ON b.entity_id=e.id
		WHERE ctx.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND e.public_id IN (SELECT pid_blob(value) FROM json_each(?))`,
		r.Library, string(encoded))
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var f bookProjectionFacts
		if err = rows.Scan(&id, &f.author, &f.narrator, &f.series, &f.seriesPosition, &f.seriesIndex); err != nil {
			rows.Close()
			return err
		}
		facts[id] = f
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	summaries, err := s.bookTimelineSummaries(r.Viewer, ids)
	if err != nil {
		return err
	}
	progress, err := s.bookProgressBatch(r, ids, summaries)
	if err != nil {
		return err
	}
	for i := range sec.Entries {
		v := &sec.Entries[i]
		if f, ok := facts[v.ID]; ok {
			v.Author = f.author
			if f.narrator != "" {
				v.Narrators = []string{f.narrator}
			}
			if f.series != "" {
				position := f.seriesPosition
				if position == "" && f.seriesIndex > 0 {
					position = strconv.FormatFloat(f.seriesIndex, 'f', -1, 64)
				}
				v.Series = []AudioSeries{{Name: f.series, Position: position}}
			}
		}
		if summary, ok := summaries[v.ID]; ok && summary.duration != nil {
			seconds := float64(*summary.duration) / 1000
			v.Duration = &seconds
		}
		if p := progress[v.ID]; p != nil {
			v.BookProgress = p
			if p.PositionMs != nil {
				seconds := float64(*p.PositionMs) / 1000
				v.ProgressSeconds = &seconds
			}
		}
	}
	return nil
}

// One bounded query for the returned book ids. A duration is published only
// when the whole book is within the existing 512-part bound and every part is
// visible with a known duration; otherwise it stays absent.
func (s *Service) bookTimelineSummaries(viewer Viewer, books []string) (map[string]bookTimelineSummary, error) {
	out := map[string]bookTimelineSummary{}
	if len(books) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(books)
	if err != nil {
		return nil, err
	}
	visible, args := viewer.itemVisibilitySQL("file.id")
	query := `SELECT book_id,file_id,permitted,seconds FROM (
		SELECT pid(book.public_id) book_id,pid(file.public_id) file_id,
			CASE WHEN (` + visible + `) AND ` + bookPartAvailable + ` THEN 1 ELSE 0 END permitted,
			` + bookPartDuration + ` seconds,
			ROW_NUMBER() OVER (PARTITION BY book.id ORDER BY ` + bookPartOrder + `) rn
		FROM catalog_entities book CROSS JOIN catalog_book_files f INDEXED BY catalog_book_files_listening_order ON f.book_id=book.id
			CROSS JOIN catalog_entities file ON file.id=f.entity_id
		WHERE book.kind=8 AND book.retired=0 AND file.retired=0
			AND book.public_id IN (SELECT pid_blob(value) FROM json_each(?))
	) WHERE rn<=513`
	args = append(args, string(encoded))
	rows, err := s.read().Query(query, args...)
	if err != nil {
		return nil, err
	}
	grouped := map[string][]bookTimelineRow{}
	for rows.Next() {
		var book string
		var row bookTimelineRow
		if err = rows.Scan(&book, &row.file, &row.permitted, &row.seconds); err != nil {
			rows.Close()
			return nil, err
		}
		grouped[book] = append(grouped[book], row)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for book, parts := range grouped {
		summary := bookTimelineSummary{parts: map[string]BookTimelinePart{}}
		total, prefix := int64(0), true
		for index, row := range parts {
			part := BookTimelinePart{ItemID: row.file}
			if row.permitted {
				part.DurationMs = knownMillis(row.seconds)
				if prefix {
					offset := total
					part.OffsetMs = &offset
				}
				summary.parts[row.file] = part
			}
			duration := part.DurationMs
			if !row.permitted || duration == nil || total > maxInt64-*duration {
				prefix = false
			} else if prefix {
				total += *duration
			}
			if index >= 511 {
				prefix = false
			}
		}
		if prefix && len(parts) > 0 && len(parts) <= 512 {
			summary.duration = &total
		}
		out[book] = summary
	}
	return out, nil
}

const maxInt64 = int64(^uint64(0) >> 1)

// The requesting profile's own resume for the same returned book ids. Resume
// rows outside the visible book set never contribute progress.
func (s *Service) bookProgressBatch(r ContentRequest, books []string, summaries map[string]bookTimelineSummary) (map[string]*BookProgress, error) {
	out := map[string]*BookProgress{}
	if len(books) == 0 || r.Profile == "" {
		return out, nil
	}
	encoded, err := json.Marshal(books)
	if err != nil {
		return nil, err
	}
	visible, args := r.Viewer.itemVisibilitySQL("file.id")
	rows, err := s.read().Query(`SELECT pid(book.public_id),pid(file.public_id),r.position
		FROM book_resume r
		JOIN catalog_entities book ON book.id=r.book_id AND book.kind=8 AND book.retired=0
		JOIN catalog_entities file ON file.id=r.item_id AND file.retired=0
		JOIN catalog_book_files f ON f.entity_id=file.id AND f.book_id=book.id
		WHERE r.profile_id=? AND r.completed=0 AND book.public_id IN (SELECT pid_blob(value) FROM json_each(?)) AND `+visible,
		append([]any{r.Profile, string(encoded)}, args...)...)
	if err != nil {
		return nil, err
	}
	type resumeRow struct {
		book, file string
		position   int64
	}
	var resumes []resumeRow
	for rows.Next() {
		var row resumeRow
		if err = rows.Scan(&row.book, &row.file, &row.position); err != nil {
			rows.Close()
			return nil, err
		}
		resumes = append(resumes, row)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(resumes) == 0 {
		return out, nil
	}
	chapters := map[string]BookProgressChapter{}
	rows, err = s.read().Query(`SELECT pid(book.public_id),c.chapter_index,c.title,c.start_seconds,c.end_seconds
		FROM book_resume r
		JOIN catalog_entities book ON book.id=r.book_id AND book.kind=8 AND book.retired=0
		JOIN catalog_book_chapters c ON c.file_id=r.item_id
		WHERE r.profile_id=? AND r.completed=0 AND book.public_id IN (SELECT pid_blob(value) FROM json_each(?))
			AND c.start_seconds<=r.position/1000.0 AND c.end_seconds>r.position/1000.0
		ORDER BY pid(book.public_id),c.start_seconds DESC`,
		r.Profile, string(encoded))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var book string
		var chapter BookProgressChapter
		var start, end float64
		if err = rows.Scan(&book, &chapter.Index, &chapter.Title, &start, &end); err != nil {
			rows.Close()
			return nil, err
		}
		if _, exists := chapters[book]; !exists {
			chapter.StartMs = int64(start * 1000)
			chapter.EndMs = int64(end * 1000)
			chapters[book] = chapter
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, resume := range resumes {
		summary := summaries[resume.book]
		part, ok := summary.parts[resume.file]
		if !ok {
			continue
		}
		position := max(resume.position, 0)
		if part.DurationMs != nil {
			position = min(position, *part.DurationMs)
		}
		progress := &BookProgress{BookID: resume.book, ItemID: resume.file, PartPositionMs: position, DurationMs: summary.duration}
		if part.OffsetMs != nil {
			value := *part.OffsetMs + position
			progress.PositionMs = &value
		}
		if progress.PositionMs != nil && progress.DurationMs != nil {
			value := max(int64(0), *progress.DurationMs-*progress.PositionMs)
			progress.RemainingMs = &value
		}
		if chapter, ok := chapters[resume.book]; ok {
			progress.Chapter = &chapter
		}
		out[resume.book] = progress
	}
	return out, nil
}

// Entity totals are the authorized section count; duration and narrators are
// published only for a complete first window (start 0, exact count, no cursor,
// every returned book carrying the fact).
func applyAuthorSeriesEntityFacts(out *ContentEnvelope) {
	if out.Entity == nil {
		return
	}
	var books *ContentSection
	for i := range out.Sections {
		if out.Sections[i].ID == "books" {
			books = &out.Sections[i]
			break
		}
	}
	if books == nil {
		return
	}
	count := books.TotalCount
	out.Entity.Count = &count
	if books.Start != 0 || books.NextCursor != "" || count == 0 || len(books.Entries) != count {
		return
	}
	total, completeDuration, completeNarrators := 0.0, true, true
	narrators := []string{}
	seen := map[string]bool{}
	for _, entry := range books.Entries {
		if entry.Duration == nil {
			completeDuration = false
		} else {
			total += *entry.Duration
		}
		if len(entry.Narrators) == 0 {
			completeNarrators = false
		} else {
			for _, narrator := range entry.Narrators {
				if !seen[narrator] {
					seen[narrator] = true
					narrators = append(narrators, narrator)
				}
			}
		}
	}
	if completeDuration {
		out.Entity.Duration = &total
	}
	if completeNarrators {
		sort.Strings(narrators)
		out.Entity.Narrators = narrators
	}
}
