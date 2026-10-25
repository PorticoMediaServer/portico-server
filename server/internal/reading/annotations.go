package reading

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

const annotationLimit = 256
const annotationPage = 50

type Annotation struct {
	ID, Kind             string
	Location             Location
	Text, Note, Colour   string
	CreatedAt, UpdatedAt int64
}
type AnnotationPage struct {
	SourceID, SourceRevision string
	Revision                 int64
	Entries                  []Annotation
	NextCursor               string
}
type AnnotationMutation struct {
	ExpectedRevision int64
	Operation        string
	Annotation       Annotation
}

func annotationID(id string) bool {
	if len(id) < 1 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func annotationText(s string, limit int) bool {
	return len(s) <= limit && strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' }) < 0
}
func (f *Source) annotationOwner() (string, error) {
	p := f.p
	if p.ProfileID == "" || p.Role == "guest" || p.Role == "account" || p.Grant != "" {
		return "", identity.ErrForbidden
	}
	return identity.PersonalKey(p.Viewer), nil
}

// Every read/write uses the original acquisition's principal and exact version.
func (f *Source) annotationSelection(ctx context.Context, tx *sql.Tx) (Selection, error) {
	v, err := f.service.selectTx(ctx, tx, f.p, f.ItemID, f.SourceID)
	if err != nil {
		return v, err
	}
	if v.Revision != f.Revision {
		return v, ErrChanged
	}
	if f.remoteSession != nil {
		if err = f.remoteSession.remote.CheckTx(ctx, tx); err != nil {
			return v, err
		}
	}
	return v, nil
}
func annotationRevision(ctx context.Context, tx *sql.Tx, owner string, v Selection) (int64, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, `SELECT revision FROM reading_annotation_sets WHERE owner_key=? AND item_id=? AND asset_id=? AND source_revision=?`, owner, v.Entity, v.Asset, v.Revision).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return revision, err
}
func (f *Source) Annotations(cursor string, expected *int64) (*AnnotationPage, error) {
	owner, err := f.annotationOwner()
	if err != nil {
		return nil, err
	}
	if cursor != "" && (!annotationID(cursor) || expected == nil) || expected != nil && (*expected < 0 || *expected > 9007199254740991) {
		return nil, ErrInvalid
	}
	if err = f.Validate(); err != nil {
		return nil, err
	}
	ctx := f.ctx
	g, err := dbwork.BeginSnapshot(ctx, f.service.DB)
	if err != nil {
		return nil, err
	}
	defer g.Rollback()
	v, err := f.annotationSelection(ctx, g.Tx())
	if err != nil {
		return nil, err
	}
	revision, err := annotationRevision(ctx, g.Tx(), owner, v)
	if err != nil {
		return nil, err
	}
	if expected != nil && revision != *expected {
		return nil, ErrConflict
	}
	out := &AnnotationPage{SourceID: v.SourceID, SourceRevision: v.Revision, Revision: revision, Entries: []Annotation{}}
	rows, err := g.Tx().QueryContext(ctx, `SELECT id,kind,format,cfi,section,page_index,progression,quote,note,colour,created_at,updated_at FROM reading_annotations WHERE owner_key=? AND item_id=? AND asset_id=? AND source_revision=? AND id>? ORDER BY id LIMIT ?`, owner, v.Entity, v.Asset, v.Revision, cursor, annotationPage+1)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var a Annotation
		var cfi, section sql.NullString
		var page sql.NullInt64
		if err = rows.Scan(&a.ID, &a.Kind, &a.Location.Format, &cfi, &section, &page, &a.Location.Progression, &a.Text, &a.Note, &a.Colour, &a.CreatedAt, &a.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		a.Location.CFI = cfi.String
		a.Location.Section = section.String
		if page.Valid {
			value := page.Int64
			a.Location.PageIndex = &value
		}
		if len(out.Entries) == annotationPage {
			out.NextCursor = out.Entries[len(out.Entries)-1].ID
			break
		}
		out.Entries = append(out.Entries, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	g.Rollback()
	if err = f.Validate(); err != nil {
		return nil, err
	}
	return out, nil
}
func (f *Source) annotationLocation(l Location) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if l.Format != f.Format {
		return ErrInvalid
	}
	switch f.Format {
	case "epub":
		e, err := f.epub()
		if err != nil {
			return f.StopAndRecheck(err)
		}
		if l.Section != "" {
			found := false
			for _, section := range e.Spine {
				if section == l.Section {
					found = true
					break
				}
			}
			if !found {
				return ErrInvalid
			}
		}
	case "cbz":
		c, err := f.comic()
		if err != nil {
			return f.StopAndRecheck(err)
		}
		if *l.PageIndex >= int64(len(c.Names)) {
			return ErrInvalid
		}
	case "pdf":
		if err := f.CheckPDF(); err != nil {
			return err
		}
	}
	return f.Validate()
}

// User-authored passages/notes are personal content, never metadata evidence.
// This explicit action is independent of automatic history recording policy.
func (f *Source) MutateAnnotation(m AnnotationMutation) (int64, error) {
	owner, err := f.annotationOwner()
	if err != nil {
		return 0, err
	}
	a := m.Annotation
	if m.ExpectedRevision < 0 || m.ExpectedRevision >= 9007199254740991 || !annotationID(a.ID) || (m.Operation != "upsert" && m.Operation != "delete") {
		return 0, ErrInvalid
	}
	if m.Operation == "upsert" {
		if a.Kind != "bookmark" && a.Kind != "highlight" || !annotationText(a.Text, 8192) || !annotationText(a.Note, 4096) {
			return 0, ErrInvalid
		}
		if a.Kind == "bookmark" && (a.Text != "" || a.Colour != "") || a.Kind == "highlight" && (strings.TrimSpace(a.Text) == "" || a.Colour != "yellow" && a.Colour != "green" && a.Colour != "blue" && a.Colour != "pink") {
			return 0, ErrInvalid
		}
		if err = f.annotationLocation(a.Location); err != nil {
			return 0, err
		}
	} else if err = f.Validate(); err != nil {
		return 0, err
	}
	ctx := f.ctx
	g, err := dbwork.Begin(ctx, f.service.DB, dbwork.ClassInteractive)
	if err != nil {
		return 0, err
	}
	defer g.Rollback()
	tx := g.Tx()
	v, err := f.annotationSelection(ctx, tx)
	if err != nil {
		return 0, err
	}
	revision, err := annotationRevision(ctx, tx, owner, v)
	if err != nil {
		return 0, err
	}
	if revision != m.ExpectedRevision {
		return 0, ErrConflict
	}
	revision++
	changed, err := tx.ExecContext(ctx, `INSERT INTO reading_annotation_sets(owner_key,item_id,asset_id,source_revision,revision) VALUES(?,?,?,?,?) ON CONFLICT(owner_key,item_id,asset_id,source_revision) DO UPDATE SET revision=excluded.revision WHERE reading_annotation_sets.revision=?`, owner, v.Entity, v.Asset, v.Revision, revision, m.ExpectedRevision)
	if err != nil {
		return 0, err
	}
	n, err := changed.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, ErrConflict
	}
	if m.Operation == "delete" {
		result, e := tx.ExecContext(ctx, `DELETE FROM reading_annotations WHERE owner_key=? AND item_id=? AND asset_id=? AND source_revision=? AND id=?`, owner, v.Entity, v.Asset, v.Revision, a.ID)
		if e != nil {
			return 0, e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return 0, e
		}
		if n != 1 {
			return 0, ErrConflict
		}
	} else {
		var count, all, existing int
		if err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(id=?),0) FROM reading_annotations WHERE owner_key=? AND item_id=? AND asset_id=? AND source_revision=?`, a.ID, owner, v.Entity, v.Asset, v.Revision).Scan(&count, &existing); err != nil {
			return 0, err
		}
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM reading_annotations WHERE owner_key=? AND item_id=? AND asset_id=?`, owner, v.Entity, v.Asset).Scan(&all); err != nil {
			return 0, err
		}
		if existing == 0 && (count >= annotationLimit || all >= 4096) {
			return 0, ErrInvalid
		}
		var cfi, section, page any
		if a.Location.Format == "epub" {
			cfi = a.Location.CFI
			if a.Location.Section != "" {
				section = a.Location.Section
			}
		} else {
			page = *a.Location.PageIndex
		}
		now := time.Now().UnixMilli()
		_, err = tx.ExecContext(ctx, `INSERT INTO reading_annotations(owner_key,item_id,asset_id,source_revision,id,kind,format,cfi,section,page_index,progression,quote,note,colour,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(owner_key,item_id,asset_id,source_revision,id) DO UPDATE SET kind=excluded.kind,format=excluded.format,cfi=excluded.cfi,section=excluded.section,page_index=excluded.page_index,progression=excluded.progression,quote=excluded.quote,note=excluded.note,colour=excluded.colour,updated_at=excluded.updated_at`, owner, v.Entity, v.Asset, v.Revision, a.ID, a.Kind, a.Location.Format, cfi, section, page, a.Location.Progression, a.Text, a.Note, a.Colour, now, now)
		if err != nil {
			return 0, err
		}
	}
	if _, err = f.annotationSelection(ctx, tx); err != nil {
		return 0, err
	}
	if err = ctx.Err(); err != nil {
		return 0, err
	}
	if err = g.Commit(); err != nil {
		return 0, err
	}
	return revision, nil
}
