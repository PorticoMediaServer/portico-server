package reading

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"portico.local/server/internal/storage"
)

const mobiDependency = "libmobi:85dcfe803fc2a21020ddcf15c3eb66b93d388add"
const mobiOptions = "portico-epub-v1;unencrypted-only;no-replica;default-kf8"
const maxMobiInput = 64 << 20
const maxMobiOutput = 128 << 20

// MobiProvenance identifies actual original bytes and derived output separately.
// Nothing in it is a substitute for the captured Source's live authority.
type MobiProvenance struct {
	OriginalRevision string
	OriginalSourceID string
	OriginalFormat   string
	InputSHA256      string
	Converter        string
	WorkerSHA256     string
	Options          string
	OutputSHA256     string
	Revision         string
}

type MobiBundle struct {
	// Exact Release-qualified absolute files, never PATH discovery or downloads.
	Sandbox, SandboxSHA256 string
	Worker, WorkerSHA256   string
	Scratch                string
}

type MobiConverter struct {
	bundle     MobiBundle
	supervisor *storage.Supervisor
	slot       chan struct{}
}

func NewMobiConverter(bundle MobiBundle, supervisor *storage.Supervisor) (*MobiConverter, error) {
	if supervisor == nil || !filepath.IsAbs(bundle.Scratch) {
		return nil, ErrUnavailable
	}
	// The already provisioned scratch root must be private and not a symlink.
	info, err := os.Lstat(bundle.Scratch)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrUnavailable
	}
	for _, entry := range []struct{ path, digest string }{{bundle.Sandbox, bundle.SandboxSHA256}, {bundle.Worker, bundle.WorkerSHA256}} {
		if !filepath.IsAbs(entry.path) || len(entry.digest) != 64 {
			return nil, ErrUnavailable
		}
		digest, err := hex.DecodeString(entry.digest)
		if err != nil || len(digest) != 32 {
			return nil, ErrUnavailable
		}
	}
	return &MobiConverter{bundle: bundle, supervisor: supervisor, slot: make(chan struct{}, 1)}, nil
}

// DerivedMobi is request/session-owned, not an independently authorized file.
// Its reader keeps every parser extent behind the original physical-source fence.
type DerivedMobi struct {
	source     *Source
	raw        []byte
	provenance MobiProvenance
}

func (d *DerivedMobi) Provenance() MobiProvenance { return d.provenance }
func (d *DerivedMobi) ReadAt(p []byte, offset int64) (int, error) {
	if err := d.source.Validate(); err != nil {
		return 0, err
	}
	n, err := bytes.NewReader(d.raw).ReadAt(p, offset)
	if check := d.source.Validate(); check != nil {
		clear(p[:n])
		return 0, check
	}
	return n, err
}
func (d *DerivedMobi) EPUB() (*EPUB, error) {
	if err := d.source.Validate(); err != nil {
		return nil, err
	}
	publication, err := OpenEPUB(d.source.Context(), d, int64(len(d.raw)))
	if err != nil {
		return nil, err
	}
	if err = d.source.Validate(); err != nil {
		return nil, err
	}
	return publication, nil
}

// Convert refuses sequential-only transport rather than silently staging a
// source whose whole-file/range policy does not permit this conversion.
func (c *MobiConverter) Convert(source *Source) (*DerivedMobi, error) {
	if c == nil || source == nil || (source.Format != "mobi" && source.Format != "azw3") {
		return nil, ErrUnavailable
	}
	if source.Sequential() {
		return nil, ErrUnavailable
	}
	if source.Size <= 0 || source.Size > maxMobiInput {
		return nil, ErrLimit
	}
	select {
	case c.slot <- struct{}{}:
		defer func() { <-c.slot }()
	default:
		return nil, storage.ErrBusy
	}
	ctx, cancel := context.WithTimeout(source.Context(), 45*time.Second)
	defer cancel()
	if err := source.Validate(); err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp(c.bundle.Scratch, "reading-mobi-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	input, err := os.OpenFile(filepath.Join(directory, "input"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	inputHash := sha256.New()
	buffer := make([]byte, 128<<10)
	for offset := int64(0); offset < source.Size; {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		count := int(min(int64(len(buffer)), source.Size-offset))
		n, readErr := source.ReadAt(buffer[:count], offset)
		if n != count || (readErr != nil && readErr != io.EOF) {
			if readErr == nil {
				readErr = io.ErrUnexpectedEOF
			}
			return nil, readErr
		}
		if _, err = io.MultiWriter(input, inputHash).Write(buffer[:n]); err != nil {
			return nil, err
		}
		offset += int64(n)
	}
	if err = source.Validate(); err != nil {
		return nil, err
	}
	if _, err = input.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	raw, err := c.run(ctx, input)
	if err != nil {
		return nil, err
	}
	if err = source.Validate(); err != nil {
		return nil, err
	}
	outputHash := sha256.Sum256(raw)
	provenance := MobiProvenance{OriginalRevision: source.Revision, OriginalSourceID: source.SourceID, OriginalFormat: source.Format, InputSHA256: hex.EncodeToString(inputHash.Sum(nil)), Converter: mobiDependency, WorkerSHA256: c.bundle.WorkerSHA256, Options: mobiOptions, OutputSHA256: hex.EncodeToString(outputHash[:])}
	encoded, _ := json.Marshal(provenance)
	revision := sha256.Sum256(append([]byte("portico.reading.derived-epub.v1\x00"), encoded...))
	provenance.Revision = hex.EncodeToString(revision[:])
	derived := &DerivedMobi{source: source, raw: raw, provenance: provenance}
	// The genuine output must pass the ordinary bounded EPUB package parser.
	// Assets are subsequently served through EPUB.Asset's existing sanitizer.
	if _, err = OpenEPUB(ctx, derived, int64(len(raw))); err != nil {
		clear(raw)
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return derived, nil
}
