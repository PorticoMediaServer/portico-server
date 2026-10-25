package reading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"portico.local/server/internal/childprocess"
)

// PDF text coverage runs in a bounded helper process, never in the server's own
// address space: a malformed or malicious document can consume only the child's
// hard memory/CPU budget, and an OOM kills the child instead of the server. The
// parent copies the admitted source into a private temporary file, runs the
// pinned parser in that child, waits for it to retire, and removes the file
// before releasing the source custody. The child receives exactly one fixed
// argument vector; there is no shell, no network, no PDF action and no external
// resource path.
const (
	maxCountPDFSource = 128 << 20
	maxCountPDFPages  = 3000
	maxCountPDFText   = 64 << 20
	maxCountPDFRun    = 4 << 20
	maxPDFResultBytes = 64 << 10
	PDFTextHelperArg  = "pdf-text-extract"
)

const (
	pdfHelperOK      = 0
	pdfHelperUnknown = 3
	pdfHelperRetry   = 4
)

// pdfTextHelperExecutable is the server binary; a package test may replace it
// with a real helper invocation instead of the test binary.
var pdfTextHelperExecutable = os.Executable

type pdfTextResult struct {
	Count   int64  `json:"count"`
	Opening string `json:"opening"`
}

// countPDFText counts complete supported page text. A nil count is unknown: an
// empty/image-only document, encryption, unsupported filters/fonts, malformed or
// over-budget structure never yields zero or a partial count. A blank page is
// not a failure; every page up to the budget is inspected, and the page count
// itself is never counted as text.
func countPDFText(ctx context.Context, r io.ReaderAt, size int64) (*int64, string, error) {
	if size <= 0 || size > maxCountPDFSource {
		return nil, "", nil
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	tempDir := filepath.Clean(os.TempDir())
	file, err := os.CreateTemp(tempDir, "portico-pdftext-*")
	if err != nil {
		return nil, "", err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = copyPDFSource(ctx, r, size, file); err != nil {
		file.Close()
		if errors.Is(err, ErrInvalid) {
			return nil, "", nil
		}
		return nil, "", err
	}
	if err = file.Close(); err != nil {
		return nil, "", err
	}
	executable, err := pdfTextHelperExecutable()
	if err != nil {
		return nil, "", err
	}
	// The helper's argument vector is ours alone: the executable, the fixed
	// subcommand and the private temporary path. No source-controlled text is
	// ever an argument.
	command := exec.CommandContext(ctx, executable, PDFTextHelperArg, name)
	command.Env = []string{"TMPDIR=" + tempDir}
	output := &boundedBuffer{max: maxPDFResultBytes}
	command.Stdout = output
	command.Stderr = io.Discard
	childprocess.Configure(command)
	runErr := command.Run()
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	if runErr != nil {
		var exit *exec.ExitError
		if errors.As(runErr, &exit) && exit.ExitCode() == pdfHelperUnknown {
			return nil, "", nil
		}
		// A signal, OOM kill, CPU limit or helper crash is a parser-process
		// failure, not a determination: retry it rather than caching a fact.
		return nil, "", fmt.Errorf("pdf text extractor: %w", runErr)
	}
	var result pdfTextResult
	if err = json.Unmarshal(output.Bytes(), &result); err != nil {
		return nil, "", fmt.Errorf("pdf text extractor result: %w", err)
	}
	if result.Count <= 0 || len(result.Opening) > 4096 {
		return nil, "", fmt.Errorf("pdf text extractor result")
	}
	count := result.Count
	return &count, result.Opening, nil
}

// copyPDFSource snapshots the admitted source through its own ReaderAt, so every
// extent keeps the source's context and currentness checks. A short read is an
// incomplete acquisition, never a shorter document.
func copyPDFSource(ctx context.Context, r io.ReaderAt, size int64, w io.Writer) error {
	buffer := make([]byte, 256<<10)
	for offset := int64(0); offset < size; {
		if err := ctx.Err(); err != nil {
			return err
		}
		want := int64(len(buffer))
		if offset+want > size {
			want = size - offset
		}
		n, err := r.ReadAt(buffer[:want], offset)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
			offset += int64(n)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if n == 0 {
			return io.ErrUnexpectedEOF
		}
	}
	return nil
}

// boundedBuffer refuses to retain more than max output bytes; a runaway helper
// cannot grow the server's memory through its stdout.
type boundedBuffer struct {
	max int
	buf []byte
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	if len(b.buf)+len(value) > b.max {
		return 0, fmt.Errorf("pdf text extractor output exceeds bound")
	}
	b.buf = append(b.buf, value...)
	return len(value), nil
}

func (b *boundedBuffer) Bytes() []byte { return b.buf }

// RunPDFTextHelper is the child entry point. It never starts the server. The
// path must be the parent's private temporary snapshot.
func RunPDFTextHelper(path string) int {
	// Only the process the parent actually started with the fixed helper argv
	// may apply hard limits and parse; an accidental in-process call must never
	// restrict the server.
	if len(os.Args) != 3 || os.Args[1] != PDFTextHelperArg {
		return pdfHelperUnknown
	}
	if !filepath.IsAbs(path) || filepath.Dir(path) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(path), "portico-pdftext-") {
		return pdfHelperUnknown
	}
	if err := limitPDFProcess(); err != nil {
		return pdfHelperUnknown
	}
	file, err := os.Open(path)
	if err != nil {
		return pdfHelperRetry
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxCountPDFSource {
		return pdfHelperUnknown
	}
	count, opening, err := parsePDFText(context.Background(), file, info.Size())
	if err != nil {
		return pdfHelperRetry
	}
	if count == nil {
		return pdfHelperUnknown
	}
	if err = json.NewEncoder(os.Stdout).Encode(pdfTextResult{Count: *count, Opening: opening}); err != nil {
		return pdfHelperRetry
	}
	return pdfHelperOK
}
