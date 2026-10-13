package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediaartifact"
)

type originalFile struct {
	part, name, format, encoding string
	object                       mediaartifact.Object
}

func (s *Service) sealOriginalFiles(ctx context.Context, raw []byte, durationUS int64) ([]originalFile, error) {
	var a RenderAsset
	if json.Unmarshal(raw, &a) != nil || a.Version != 2 {
		return nil, ErrInput
	}
	data := a.Data
	if len(a.Authored) > 0 {
		data = a.Authored
	}
	files := []originalFile{}
	add := func(part, name, format, encoding string, data []byte) error {
		if len(data) == 0 {
			return nil
		}
		if len(data) > MaxBinaryBytes {
			return ErrCapacity
		}
		w, e := s.objects.Begin(MaxBinaryBytes)
		if e != nil {
			return e
		}
		defer w.Abort()
		if _, e = w.Write(data); e != nil {
			return e
		}
		object, e := w.Seal(ctx)
		if e != nil {
			return e
		}
		files = append(files, originalFile{part, name, format, encoding, object})
		return nil
	}
	if e := add("data", "", a.Format, a.Encoding, data); e != nil {
		return nil, e
	}
	if e := add("companion", "", "vobsub", "", a.Companion); e != nil {
		return nil, e
	}
	if a.Format == "ass" || a.Format == "ssa" {
		if cues, e := ParseASSAsText(a.Data, durationUS); e == nil {
			if document, e := textDocument(cues); e == nil {
				if e = add("text", "", "text", "utf-8", document); e != nil {
					return nil, e
				}
			}
		}
	}

	for i, f := range a.Fonts {
		if e := add("font:"+strconv.Itoa(i), f.Name, "font", "", f.Data); e != nil {
			return nil, e
		}
	}
	return files, nil
}
func publishOriginalFiles(ctx context.Context, tx *sql.Tx, id string, revision int64, files []originalFile) error {
	for _, f := range files {
		if _, e := tx.ExecContext(ctx, `INSERT OR IGNORE INTO subtitle_original_files(resource_id,revision,part,name,format,encoding,digest,size) VALUES(?,?,?,?,?,?,?,?)`, id, revision, f.part, f.name, f.format, f.encoding, f.object.Digest, f.object.Size); e != nil {
			return e
		}
	}
	return nil
}

// OpenOriginalFile lends one immutable raw file. Range delivery reads directly
// from the object store, without decoding a whole bitmap/font archive per viewer.
func (s *Service) OpenOriginalFile(ctx context.Context, p identity.Principal, item, session, id string, revision int64, part string) (*mediaartifact.Reader, int64, string, string, error) {
	if e := s.CheckOriginal(ctx, p, item, session, id, revision); e != nil {
		return nil, 0, "", "", e
	}
	if e := s.ensureCached(ctx, p, item, id, revision); e != nil {
		return nil, 0, "", "", e
	}
	snapshot, p, e := s.readTx(ctx, p, item)
	if e != nil {
		return nil, 0, "", "", e
	}
	defer snapshot.Rollback()
	var object mediaartifact.Object
	var format, encoding string
	e = snapshot.Tx().QueryRowContext(ctx, `SELECT digest,size,format,encoding FROM subtitle_original_files WHERE resource_id=? AND revision=? AND part=?`, id, revision, part).Scan(&object.Digest, &object.Size, &format, &encoding)
	if e != nil {
		return nil, 0, "", "", e
	}
	reader, e := s.objects.Open(ctx, object)
	return reader, object.Size, format, encoding, e
}
func (s *Service) OriginalFonts(ctx context.Context, p identity.Principal, item, session, id string, revision int64, base string) (FontFiles, error) {
	out := FontFiles{Fonts: []FontFile{}}
	if e := s.CheckOriginal(ctx, p, item, session, id, revision); e != nil {
		return out, e
	}
	if e := s.ensureCached(ctx, p, item, id, revision); e != nil {
		return out, e
	}
	snapshot, p, e := s.readTx(ctx, p, item)
	if e != nil {
		return out, e
	}
	defer snapshot.Rollback()
	_ = snapshot.Tx().QueryRowContext(ctx, `SELECT skipped_fonts FROM subtitle_original_revisions WHERE resource_id=? AND revision=?`, id, revision).Scan(&out.SkippedCount)
	rows, e := snapshot.Tx().QueryContext(ctx, `SELECT part,name FROM subtitle_original_files WHERE resource_id=? AND revision=? AND part LIKE 'font:%' ORDER BY length(part),part LIMIT 16`, id, revision)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var part, name string
		if e = rows.Scan(&part, &name); e != nil {
			return out, e
		}
		out.Fonts = append(out.Fonts, FontFile{Name: name, URL: base + "/fonts/" + part[len("font:"):]})
	}
	return out, rows.Err()
}
