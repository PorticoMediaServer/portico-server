package reading

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ErrArchiveFormat reports an archive this server cannot read. It is returned
// before any member is opened, never as a reason to guess or relabel a format.
var ErrArchiveFormat = errors.New("reading: unsupported archive format")

// Bounded fixed-page archive reading. CBR (RAR) and CB7 (7z) reuse the ZIP
// publication's exact admission: source size, directory/raw-entry checks,
// member count and size, MaxExpanded work, canonical paths and no duplicates.
const maxArchiveDepth = 32

// The RAR decoder's dictionary ceiling is the publication's member budget; a
// larger dictionary means this file is not a fixed-page comic this server will
// decode.
const maxRARDictionary = MaxMember

type archiveMember struct {
	Name string
	Size int64
	Open func() (io.ReadCloser, error)
}

type memberArchive interface {
	Members() []archiveMember
	Close() error
}

// openMembers maps the publication's format onto the same bounded reader the
// ZIP publication already uses. A classic format is only read when its decoder
// is linked in at Release's pinned version (build tag portico_rar7z); otherwise
// the format is refused rather than recognised.
func openMembers(ctx context.Context, format string, ra io.ReaderAt, size int64) (memberArchive, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if size < 22 || size > MaxSource {
		return nil, ErrLimit
	}
	switch strings.ToLower(format) {
	case "cbz":
		// The established ZIP admission runs before zip.NewReader materialises
		// the central directory.
		if err := inspectDirectory(ra, size); err != nil {
			return nil, err
		}
		zr, err := zip.NewReader(ra, size)
		if err != nil {
			return nil, archiveFailure(err)
		}
		if len(zr.File) > MaxEntries {
			return nil, ErrLimit
		}
		seen := map[string]bool{}
		out := make([]archiveMember, 0, len(zr.File))
		var expanded int64
		for _, f := range zr.File {
			if !memberName(f.Name, seen) {
				continue
			}
			if !f.Mode().IsRegular() {
				continue
			}
			expanded += int64(f.UncompressedSize64)
			if expanded > MaxExpanded || int64(f.UncompressedSize64) > MaxMember {
				return nil, ErrLimit
			}
			if !safeMember(f.Name, f.FileInfo().IsDir(), f.UncompressedSize64) {
				continue
			}
			member := f
			out = append(out, archiveMember{Name: member.Name, Size: int64(member.UncompressedSize64), Open: member.Open})
		}
		if len(out) == 0 {
			return nil, ErrInvalid
		}
		return &zipMembers{files: out}, nil
	case "cbr", "cb7":
		return openClassicArchive(ctx, format, ra, size)
	default:
		return nil, ErrArchiveFormat
	}
}

type zipMembers struct{ files []archiveMember }

func (z *zipMembers) Members() []archiveMember { return z.files }
func (z *zipMembers) Close() error             { return nil }

// memberName counts every encountered header and refuses duplicate names, so a
// filtered-out member cannot hide extra work.
func memberName(name string, seen map[string]bool) bool {
	if len(name) > 1024 {
		return false
	}
	if seen[name] {
		return false
	}
	seen[name] = true
	return true
}

// safeMember keeps only a canonical relative image member inside the budget;
// the path policy is the archive package's own safePath, not a weaker copy.
func safeMember(name string, dir bool, size uint64) bool {
	if dir || size == 0 || size > MaxMember {
		return false
	}
	if !safePath(name) {
		return false
	}
	return isImageMember(name)
}

func isImageMember(name string) bool {
	lower := strings.ToLower(name)
	// Only formats the fixed-page renderer actually decodes; a member the image
	// inspector would reject is never listed as a page.
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// boundedMemberRead reads exactly the header's declared size under the request's
// cancellation and the aggregate work budget, refusing a short or oversized
// stream rather than trusting metadata alone.
func boundedMemberRead(ctx context.Context, member archiveMember, expanded *int64) ([]byte, error) {
	if member.Size <= 0 || member.Size > MaxMember || *expanded+member.Size > MaxExpanded {
		return nil, ErrLimit
	}
	body, err := member.Open()
	if err != nil {
		return nil, archiveFailure(err)
	}
	defer body.Close()
	out, err := io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, r: body}, member.Size+1))
	if err != nil {
		return nil, archiveFailure(err)
	}
	if int64(len(out)) != member.Size {
		return nil, ErrInvalid
	}
	*expanded += member.Size
	return out, nil
}

// sortMembers orders pages with the archive package's one natural comparator.
func sortMembers(members []archiveMember) {
	sort.SliceStable(members, func(i, j int) bool { return naturalLess(members[i].Name, members[j].Name) })
}

func archiveFormatError(format string) error {
	return fmt.Errorf("%w: %s", ErrArchiveFormat, format)
}
