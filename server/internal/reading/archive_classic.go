//go:build portico_rar7z

package reading

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"time"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
)

// openClassicArchive is the single place the pinned RAR and 7z readers are
// used. Both are in-process, bounded readers: no external program, no path is
// passed to a shell, and every member is filtered by the same safeMember rules
// as the ZIP path before anything is opened.
func openClassicArchive(ctx context.Context, format string, ra io.ReaderAt, size int64) (memberArchive, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if format == "cbr" {
		return openRAR(ctx, ra, size)
	}
	return openSevenZip(ctx, size, ra)
}

// readerAtFileSystem hands the leased reader to rardecode without exposing a
// filesystem path; a multi-volume archive is refused because no volume set is
// offered.
type readerAtFileSystem struct {
	name string
	ra   io.ReaderAt
	size int64
}

func (r readerAtFileSystem) Open(name string) (fs.File, error) {
	if name != r.name {
		return nil, fs.ErrNotExist
	}
	return &readerAtFile{name: name, ra: r.ra, size: r.size}, nil
}

type readerAtFile struct {
	name string
	ra   io.ReaderAt
	size int64
	at   int64
}

func (f *readerAtFile) Stat() (fs.FileInfo, error) { return readerAtInfo{f}, nil }
func (f *readerAtFile) Read(p []byte) (int, error) {
	if f.at >= f.size {
		return 0, io.EOF
	}
	read, err := f.ra.ReadAt(p, f.at)
	f.at += int64(read)
	return read, err
}
func (f *readerAtFile) Close() error { return nil }

type readerAtInfo struct{ f *readerAtFile }

func (i readerAtInfo) Name() string       { return i.f.name }
func (i readerAtInfo) Size() int64        { return i.f.size }
func (i readerAtInfo) Mode() fs.FileMode  { return 0 }
func (i readerAtInfo) ModTime() time.Time { return time.Time{} }
func (i readerAtInfo) IsDir() bool        { return false }
func (i readerAtInfo) Sys() any           { return nil }

// openRAR reads headers one at a time through rardecode's streaming reader. No
// header set is materialised before admission: each Next is bounded by the same
// dictionary/read limits, counted against MaxEntries and MaxExpanded, and solid,
// encrypted or multi-volume archives are refused whole.
// RAR3-era containers may select the separate PPM allocator inside the first
// Read (decode29_ppm), whose heap is not governed by MaxDictionarySize. Only the
// RAR5 signature is admitted; a legacy RAR4 container is refused before the
// parser runs rather than promising a bound the option does not provide.
func preflightRAR(ra io.ReaderAt) error {
	var head [8]byte
	if _, err := ra.ReadAt(head[:], 0); err != nil {
		return archiveFailure(err)
	}
	if head[0] != 'R' || head[1] != 'a' || head[2] != 'r' || head[3] != '!' || head[4] != 0x1a || head[5] != 0x07 {
		return ErrInvalid
	}
	if head[6] != 0x01 || head[7] != 0x00 {
		return archiveFormatError("cbr")
	}
	return nil
}

func openRAR(ctx context.Context, ra io.ReaderAt, size int64) (memberArchive, error) {
	if size < 22 || size > MaxSource {
		return nil, ErrLimit
	}
	if err := preflightRAR(ra); err != nil {
		return nil, err
	}
	reader, err := rardecode.NewReader(&contextReader{ctx: ctx, r: io.NewSectionReader(ra, 0, size)},
		rardecode.MaxDictionarySize(maxRARDictionary))
	if err != nil {
		return nil, archiveFailure(err)
	}
	out := make([]archiveMember, 0, 32)
	seen := map[string]bool{}
	var expanded int64
	headers := 0
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, rardecode.ErrMultiVolume) || errors.Is(err, rardecode.ErrArchiveEncrypted) || errors.Is(err, rardecode.ErrArchivedFileEncrypted) || errors.Is(err, rardecode.ErrSolidOpen) {
			return nil, archiveFormatError("cbr")
		}
		if err != nil {
			return nil, archiveFailure(err)
		}
		// Counted after a real header, so exactly MaxEntries is allowed and the
		// next one is refused.
		headers++
		if headers > MaxEntries {
			return nil, ErrLimit
		}
		if header.Solid || header.Encrypted || header.HeaderEncrypted || header.LinkType != rardecode.LinkTypeNone || header.UnKnownSize {
			return nil, archiveFormatError("cbr")
		}
		if header.IsDir {
			continue
		}
		if !memberName(header.Name, seen) {
			continue
		}
		if header.UnPackedSize < 0 || header.UnPackedSize > MaxMember || uint64(MaxExpanded)-uint64(max(expanded, 0)) < uint64(header.UnPackedSize) {
			return nil, ErrLimit
		}
		expanded += header.UnPackedSize
		if !safeMember(header.Name, false, uint64(header.UnPackedSize)) {
			continue
		}
		name, memberSize := header.Name, header.UnPackedSize
		out = append(out, archiveMember{Name: name, Size: memberSize, Open: func() (io.ReadCloser, error) {
			return openRARMember(ctx, ra, size, name, memberSize)
		}})
	}
	if len(out) == 0 {
		return nil, ErrInvalid
	}
	sortMembers(out)
	return &listMembers{files: out, close: func() error { return nil }}, nil
}

// openRARMember seeks the one member from a fresh streaming reader. Solid
// archives are refused above, so skipping earlier headers needs no decompressed
// page data.
func openRARMember(ctx context.Context, ra io.ReaderAt, size int64, name string, memberSize int64) (io.ReadCloser, error) {
	reader, err := rardecode.NewReader(&contextReader{ctx: ctx, r: io.NewSectionReader(ra, 0, size)},
		rardecode.MaxDictionarySize(maxRARDictionary))
	if err != nil {
		return nil, archiveFailure(err)
	}
	for headers := 0; ; headers++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if headers >= MaxEntries {
			return nil, ErrLimit
		}
		header, err := reader.Next()
		if err != nil {
			return nil, archiveFailure(err)
		}
		if header.Name != name {
			continue
		}
		return &boundedArchiveReader{ctx: ctx, reader: reader, remaining: memberSize}, nil
	}
}

// boundedArchiveReader stops at the declared member size and checks the request
// between reads; the underlying section reader is owned by this call.
type boundedArchiveReader struct {
	ctx       context.Context
	reader    io.Reader
	remaining int64
	closed    bool
}

func (b *boundedArchiveReader) Read(p []byte) (int, error) {
	if b.closed {
		return 0, io.EOF
	}
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	if b.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	read, err := b.reader.Read(p)
	b.remaining -= int64(read)
	return read, err
}

func (b *boundedArchiveReader) Close() error { b.closed = true; return nil }

// fsMembers walks a filesystem-backed archive once, bounded, keeping only plain
// image members.
func fsMembers(ctx context.Context, rfs fs.FS) ([]archiveMember, error) {
	members := []archiveMember{}
	seen := map[string]bool{}
	var expanded int64
	encountered := 0
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if depth > maxArchiveDepth {
			return ErrLimit
		}
		entries, err := fs.ReadDir(rfs, dir)
		if err != nil {
			return archiveFailure(err)
		}
		for _, entry := range entries {
			encountered++
			if encountered > MaxEntries {
				return ErrLimit
			}
			name := entry.Name()
			if dir != "." {
				name = dir + "/" + entry.Name()
			}
			if !memberName(name, seen) {
				continue
			}
			if entry.IsDir() {
				if err := walk(name, depth+1); err != nil {
					return err
				}
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return archiveFailure(err)
			}
			if info.Size() < 0 || info.Size() > MaxMember || uint64(MaxExpanded)-uint64(max(expanded, 0)) < uint64(info.Size()) {
				return ErrLimit
			}
			expanded += info.Size()
			if !safeMember(name, false, uint64(info.Size())) {
				continue
			}
			memberName := name
			members = append(members, archiveMember{Name: memberName, Size: info.Size(), Open: func() (io.ReadCloser, error) {
				file, err := rfs.Open(memberName)
				if err != nil {
					return nil, archiveFailure(err)
				}
				return file, nil
			}})
		}
		return nil
	}
	if err := walk(".", 0); err != nil {
		return nil, err
	}
	return members, nil
}

type listMembers struct {
	files []archiveMember
	close func() error
}

func (l *listMembers) Members() []archiveMember { return l.files }
func (l *listMembers) Close() error             { return l.close() }

// sevenZipHeader admits a CB7 header without the parser allocating from it.
// The plain header is walked as the grammar the pinned decoder reads: top-level
// id+size properties, the declared file count, and folder coder records whose
// method, property size and LZMA/LZMA2 dictionary are bounded here. Anything the
// walk cannot parse exactly is refused, never guessed.
const (
	sevenZipHeaderEnd      = 0x00
	sevenZipHeaderMain     = 0x04
	sevenZipHeaderFiles    = 0x05
	sevenZipHeaderUnpack   = 0x07
	sevenZipHeaderFolders  = 0x0B
	sevenZipMaxHeaderBytes = 16 << 20
	sevenZipMaxCoders      = 64
	sevenZipMaxCoderProps  = 64 << 10
)

type sevenZipCursor struct {
	body []byte
	at   int
}

func (c *sevenZipCursor) byte() (byte, bool) {
	if c.at >= len(c.body) {
		return 0, false
	}
	value := c.body[c.at]
	c.at++
	return value, true
}

func (c *sevenZipCursor) number() (uint64, bool) {
	first, ok := c.byte()
	if !ok {
		return 0, false
	}
	var value uint64
	mask := byte(0x80)
	for i := 0; i < 8; i++ {
		if first&mask == 0 {
			value += uint64(first&(mask-1)) << (8 * i)
			return value, true
		}
		next, ok := c.byte()
		if !ok {
			return 0, false
		}
		value += uint64(next) << (8 * i)
		mask >>= 1
	}
	return 0, false
}

func (c *sevenZipCursor) skip(n uint64) bool {
	if n > uint64(len(c.body)-c.at) {
		return false
	}
	c.at += int(n)
	return true
}

func (c *sevenZipCursor) property() (byte, *sevenZipCursor, bool) {
	id, ok := c.byte()
	if !ok || id == sevenZipHeaderEnd {
		return id, nil, ok
	}
	size, ok := c.number()
	if !ok || !c.skip(size) {
		return 0, nil, false
	}
	return id, &sevenZipCursor{body: c.body[c.at-int(size) : c.at]}, true
}

// lzmaDictionary reads a bounded dictionary size from LZMA or LZMA2 coder
// properties. Unknown layouts are refused rather than assumed safe.
func lzmaDictionary(method uint64, props []byte) (uint64, bool) {
	switch method {
	case 0x030101: // LZMA
		if len(props) != 5 {
			return 0, false
		}
		return uint64(props[1]) | uint64(props[2])<<8 | uint64(props[3])<<16 | uint64(props[4])<<24, true
	case 0x21: // LZMA2
		if len(props) != 1 {
			return 0, false
		}
		if props[0] > 40 {
			return 0, false
		}
		dict := uint64(2) | uint64(props[0]&1)
		return dict << (props[0]/2 + 11), true
	}
	// Methods without a dictionary property still must have small properties.
	return 0, true
}

// boundedSevenZipHeader returns declared file and folder counts after walking
// the plain header grammar, including each folder's coders. A count above
// MaxEntries, an unknown method, oversized coder properties or a dictionary
// beyond the member budget is refused.
func boundedSevenZipHeader(body []byte) (files, folders uint64, ok bool) {
	c := &sevenZipCursor{body: body}
	marker, ok := c.byte()
	if !ok || marker != 0x01 {
		return 0, 0, false
	}
	for {
		id, region, valid := c.property()
		if !valid {
			return 0, 0, false
		}
		if id == sevenZipHeaderEnd {
			return files, folders, true
		}
		switch id {
		case sevenZipHeaderFiles:
			count, ok := region.number()
			if !ok || count == 0 || count > MaxEntries {
				return 0, 0, false
			}
			files = count
			for {
				inner, _, valid := region.property()
				if !valid {
					return 0, 0, false
				}
				if inner == sevenZipHeaderEnd {
					break
				}
			}
		case sevenZipHeaderMain:
			stream := &sevenZipCursor{body: region.body}
			for {
				sub, subRegion, valid := stream.property()
				if !valid {
					return 0, 0, false
				}
				if sub == sevenZipHeaderEnd {
					break
				}
				if sub != sevenZipHeaderUnpack {
					continue
				}
				count, ok := sevenZipFolders(subRegion)
				if !ok || count == 0 || count > MaxEntries {
					return 0, 0, false
				}
				folders = count
			}
		}
	}
}

// sevenZipFolders walks UnpackInfo: kFolder, folder count, external flag, then
// each folder's coder records with their bounded methods and properties.
func sevenZipFolders(region *sevenZipCursor) (uint64, bool) {
	id, ok := region.byte()
	if !ok || id != sevenZipHeaderFolders {
		return 0, false
	}
	count, ok := region.number()
	if !ok || count == 0 || count > MaxEntries {
		return 0, false
	}
	external, ok := region.byte()
	if !ok || external != 0 {
		return 0, false
	}
	for folder := uint64(0); folder < count; folder++ {
		coders, ok := region.number()
		if !ok || coders == 0 || coders > sevenZipMaxCoders {
			return 0, false
		}
		for coder := uint64(0); coder < coders; coder++ {
			flags, ok := region.byte()
			if !ok {
				return 0, false
			}
			methodSize := uint64(flags & 0x0F)
			if methodSize == 0 || methodSize > 15 {
				return 0, false
			}
			var method uint64
			for i := uint64(0); i < methodSize; i++ {
				part, ok := region.byte()
				if !ok {
					return 0, false
				}
				method = method<<8 | uint64(part)
			}
			var props []byte
			if flags&0x20 != 0 {
				size, ok := region.number()
				if !ok || size > sevenZipMaxCoderProps || !region.skip(size) {
					return 0, false
				}
				props = region.body[region.at-int(size) : region.at]
			}
			switch method {
			case 0x030101, 0x21: // LZMA, LZMA2: dictionary must fit the member budget.
				dict, ok := lzmaDictionary(method, props)
				if !ok || dict > uint64(MaxMember) {
					return 0, false
				}
			case 0x00, 0x03, 0x04, 0x03030103, 0x0303011B, 0x06, 0x08, 0x09: // Copy, Delta, BCJ x86/ARM/etc.
			default:
				return 0, false
			}
		}
	}
	return count, true
}

// openSevenZip admits CB7 only after the plain header's own declared file and
// folder counts are read and bounded; sevenzip.NewReader then materialises a
// header no larger than those admitted counts.
func openSevenZip(ctx context.Context, size int64, ra io.ReaderAt) (memberArchive, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if size < 32 || size > MaxSource {
		return nil, ErrLimit
	}
	var head [32]byte
	if _, err := ra.ReadAt(head[:], 0); err != nil {
		return nil, archiveFailure(err)
	}
	if string(head[:6]) != "7z\xbc\xaf'\x1c" {
		return nil, ErrInvalid
	}
	nextOffset := binary.LittleEndian.Uint64(head[12:20])
	nextSize := binary.LittleEndian.Uint64(head[20:28])
	if nextOffset > uint64(size)-32 {
		return nil, ErrLimit
	}
	if nextSize == 0 || nextSize > sevenZipMaxHeaderBytes {
		return nil, ErrLimit
	}
	if nextSize > uint64(size)-32-nextOffset {
		return nil, ErrLimit
	}
	body := make([]byte, nextSize)
	if _, err := ra.ReadAt(body, int64(32)+int64(nextOffset)); err != nil {
		return nil, archiveFailure(err)
	}
	files, folders, ok := boundedSevenZipHeader(body)
	if !ok || files == 0 || folders == 0 {
		return nil, archiveFormatError("cb7")
	}
	zr, err := sevenzip.NewReader(ra, size)
	if err != nil {
		return nil, archiveFailure(err)
	}
	if uint64(len(zr.File)) > files || uint64(len(zr.File)) > MaxEntries {
		return nil, ErrLimit
	}
	out := make([]archiveMember, 0, len(zr.File))
	seen := map[string]bool{}
	var expanded int64
	for _, f := range zr.File {
		if !memberName(f.Name, seen) {
			continue
		}
		if f.FileInfo().IsDir() || f.FileInfo().Mode()&fs.ModeSymlink != 0 {
			continue
		}
		if f.UncompressedSize64 > uint64(MaxMember) || uint64(MaxExpanded)-uint64(max(expanded, 0)) < f.UncompressedSize64 {
			return nil, ErrLimit
		}
		expanded += int64(f.UncompressedSize64)
		if !safeMember(f.Name, false, f.UncompressedSize64) {
			continue
		}
		member := f
		out = append(out, archiveMember{Name: member.Name, Size: int64(member.UncompressedSize64), Open: member.Open})
	}
	if len(out) == 0 {
		return nil, ErrInvalid
	}
	sortMembers(out)
	return &listMembers{files: out, close: func() error { return nil }}, nil
}
