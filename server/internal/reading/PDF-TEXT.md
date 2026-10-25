# PDF text coverage

PDF word counts use the same complete-publication rule, `reading_text_counts`
row/inventory/revision/rule/opening custody and background preparation lifecycle
as EPUB. The extractor is `rsc.io/pdf v0.1.1` (BSD-3-Clause, Go Authors), pinned
in `server/go.mod`/`go.sum` and recorded in `THIRD-PARTY-NOTICES.md`; Release
owns its packaging and delivery review.

Only genuine text-bearing PDFs are counted. The worker snapshots the current
admitted local source through its own `ReaderAt` and currentness checks, then
runs the pinned parser in a separate bounded helper process (the server binary's
fixed `pdf-text-extract` entry point). The helper applies its own hard
`RLIMIT_AS` (1 GiB address space), `RLIMIT_CPU` (30 s), a 192 MiB runtime soft
limit and no core file before opening the private 0600 snapshot. The hard address-space/CPU limits are applied only on Linux; on any other
platform the helper refuses and PDF counts stay unknown. The parser can
therefore exhaust only the child; an OOM, CPU kill or crash retires the child,
is waited on by the parent, and is never adopted or cached as a fact. No
detached goroutine or timeout releases the source before the child has physically
retired. The parent caps helper stdout at 64 KiB and removes the snapshot after
the wait. It never runs OCR or image analysis, never counts page count, metadata,
title or filename as text, never renders pages, never executes PDF actions, and
never loads external resources or resolves host files.

Supported coverage is computed across every page of the document up to the
bounded page/file budget. A blank page is not a failure: it contributes no text
and the following pages are still inspected; no page is skipped or invented. A
document with no extracted words, an encrypted/unsupported/malformed structure,
an unsupported filter or font encoding, a late inspection budget, or a source
that changes during any extent remains unknown (NULL), never zero or a partial
estimate. Only a complete determination is adopted; acquisition, busy,
cancellation, timeout and source-change failures write no row and are retried.
Encrypted documents are refused even when an empty user password is present;
no DRM/password/key path is admitted. An extractor change bumps the shared
`readingTextRuleVersion`, which invalidates cached counts for every format, so
no second extractor-version column or schema change is introduced.

Bounds: source 128 MiB, 3000 pages, 64 MiB extracted text, 4 MiB per text run,
in addition to the child's hard address-space/CPU limits and the existing
worker's background class, admission/pause, per-publication deadline and
cancellation. A malformed-structure panic inside the bounded child is an unknown
determination; a real read, child-limit, crash, cancellation or source-change
failure is retryable and is never cached as an unsupported document.

The EPUB viewer-open path is unchanged and remains EPUB-only; PDFs are prepared
only by the registered background worker, so opening a fixed-layout file does
not synchronously parse it. Short reads/About estimates consume a PDF only when
the accepted count exists (`0 < W < 45000`, 250 words per minute).
