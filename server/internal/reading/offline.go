package reading

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"portico.local/server/internal/identity"
)

// OfflineDocument is a bounded private descriptor for exactly one observed
// publication. Resource digests describe the original archive members, never URLs.
// It contains no annotation capability, credentials, filesystem paths or session.
type OfflineResource struct {
	Offset         int64  `json:"offset"`
	CompressedSize int64  `json:"compressedSize"`
	Compression    uint16 `json:"compression"`
	Path           string `json:"path"`
	MIME           string `json:"mime"`
	Size           int64  `json:"size"`
	SHA256         string `json:"sha256"`
}
type OfflineMetadata struct {
	Title     string `json:"title"`
	Author    string `json:"author,omitempty"`
	Language  string `json:"language,omitempty"`
	Direction string `json:"direction,omitempty"`
}
type OfflineContents struct {
	Title    string `json:"title"`
	Section  string `json:"section"`
	Fragment string `json:"fragment,omitempty"`
	Depth    int    `json:"depth"`
}
type OfflineLocation struct {
	Format      string  `json:"format"`
	CFI         string  `json:"cfi,omitempty"`
	Section     string  `json:"section,omitempty"`
	PageIndex   *int64  `json:"pageIndex,omitempty"`
	Progression float64 `json:"progression"`
}
type OfflineProgress struct {
	SourceID       string          `json:"sourceId"`
	SourceRevision string          `json:"sourceRevision"`
	Location       OfflineLocation `json:"location"`
	Revision       int64           `json:"revision"`
	ObservedAt     int64           `json:"observedAt"`
	Stale          bool            `json:"stale"`
}
type OfflineDocument struct {
	Progress  *OfflineProgress  `json:"progress"`
	ItemID    string            `json:"itemId"`
	SourceID  string            `json:"sourceId"`
	Revision  string            `json:"revision"`
	Format    string            `json:"format"`
	Size      int64             `json:"size"`
	Metadata  OfflineMetadata   `json:"metadata"`
	Resources []OfflineResource `json:"resources"`
	Spine     []string          `json:"spine"`
	PageNames []string          `json:"pageNames"`
	Contents  []OfflineContents `json:"contents"`
}

func (d OfflineDocument) Digest() string {
	raw, _ := json.Marshal(d)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// SelectOfflineTx joins download admission to the same current inventory and
// Reading authority as online open, inside the admitting transaction.
func (s *Service) SelectOfflineTx(ctx context.Context, tx *sql.Tx, p identity.Principal, item, source string) (Selection, error) {
	return s.selectTx(ctx, tx, p, item, source)
}

// OfflineDocument validates every resource using the existing reader validators.
// No accepted offline EPUB can later bypass XHTML/CSS checks by reading raw ZIP
// members. Work is cancellable and bounded by existing per-member/source limits
// plus a cumulative expansion ceiling; only one member is materialized at a time.
func (s *Service) OfflineDocument(ctx context.Context, p identity.Principal, item, source string) (OfflineDocument, error) {
	out := OfflineDocument{Resources: []OfflineResource{}, Spine: []string{}, PageNames: []string{}, Contents: []OfflineContents{}}
	f, err := s.Open(ctx, p, item, source, "")
	if err != nil {
		return out, err
	}
	defer f.Close()
	doc, err := Inspect(f)
	if err != nil {
		return out, err
	}
	out.ItemID = f.ItemID
	out.SourceID = f.SourceID
	out.Revision = f.Revision
	out.Format = f.Format
	out.Size = f.Size
	saved, err := f.Progress()
	if err != nil {
		return out, err
	}
	if saved != nil {
		l := saved.Location
		out.Progress = &OfflineProgress{saved.SourceID, saved.SourceRevision, OfflineLocation{l.Format, l.CFI, l.Section, l.PageIndex, l.Progression}, saved.Revision, saved.ObservedAt, saved.Stale}
	}
	out.Metadata = OfflineMetadata{Title: doc.Metadata.Title, Author: doc.Metadata.Author, Language: doc.Metadata.Language, Direction: doc.Metadata.Direction}
	out.Spine = append(out.Spine, doc.Spine...)
	out.PageNames = append(out.PageNames, doc.PageNames...)
	for _, entry := range doc.Contents {
		out.Contents = append(out.Contents, OfflineContents{Title: entry.Title, Section: entry.Section, Fragment: entry.Fragment, Depth: entry.Depth})
	}
	var total int64
	add := func(archive *archive, path, mime string, data []byte) error {
		entry := archive.files[path]
		if entry == nil || entry.zipped == nil {
			return ErrUnavailable
		}
		zipped := entry.zipped
		if zipped.Method != zip.Store && zipped.Method != zip.Deflate {
			return ErrUnavailable
		}
		offset, err := zipped.DataOffset()
		if err != nil {
			return err
		}
		if offset < 0 || zipped.CompressedSize64 > uint64(f.Size) || uint64(offset) > uint64(f.Size)-zipped.CompressedSize64 {
			return ErrInvalid
		}
		total += int64(len(data))
		if total > 512<<20 {
			return ErrLimit
		}
		sum := sha256.Sum256(data)
		out.Resources = append(out.Resources, OfflineResource{Offset: offset, CompressedSize: int64(zipped.CompressedSize64), Compression: zipped.Method, Path: path, MIME: mime, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])})
		return ctx.Err()
	}
	switch f.Format {
	case "epub":
		book, e := f.EPUB()
		if e != nil {
			return out, e
		}
		for _, r := range doc.Resources {
			data, mime, e := book.Asset(f.Context(), r.Path)
			if e != nil {
				return out, e
			}
			if e = add(book.archive, r.Path, mime, data); e != nil {
				return out, e
			}
		}
	case "cbz":
		book, e := f.Comic()
		if e != nil {
			return out, e
		}
		for index := range doc.PageNames {
			page, e := book.Page(f.Context(), index, 0)
			if e != nil {
				return out, e
			}
			if e = add(book.archive, page.Name, page.Image.MIME, page.Bytes); e != nil {
				return out, e
			}
		}
	case "pdf":
		if err = f.CheckPDF(); err != nil {
			return out, err
		}
	default:
		return out, ErrInvalid
	}
	if err = f.Validate(); err != nil {
		return out, err
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	return out, nil
}
