# Outcome

Implemented the pure Go package `server/internal/naming`. It reads library-relative paths with either separator and returns title candidates, release-year hints, seasonal and absolute episode numbers, air dates, parts, editions, version labels, provider IDs, extras, disc layouts, audio track/disc numbers, album candidates and artist candidates. Every retained value carries its source, component path and heuristic confidence. Competing evidence is retained; usable file evidence wins folder conflicts. There are no database, filesystem, network or provider calls in this package, and no new dependencies.

The existing catalogue now uses small adapters for `FilenameTitle`, `ParseEpisode` and extras classification. Flat shows, translated season folders, folder-derived years, filename season precedence, episode ranges and dated episodes inside numbered season folders work through those adapters. Display names lose release junk and excess whitespace. Parser version 3 lets the existing bounded issue-reparse mechanism retry previously rejected automatic episodic sources.

Built-in hidden/system/sample exclusions are wired into the local inventory helper and catalogue inventory publication. The catalogue gate covers managed listings too. Excluded pages retain their normal cursor and revision checks. The bounded `.plexignore`/`.ignore` pattern parser and matcher are ready for the later scanner integration; those files are not yet loaded by today's scanner.

Checked in 657 table-driven naming cases grouped by Plex layouts, Sonarr/Radarr formats, scene releases, anime fansubs, numeric titles and badly organised libraries, plus 127 extras cases, ignore cases, source/precedence assertions and adversarial cases. All selected checks pass. Two cumulative snapshots are the handoff; no commits, pushes or deployments were made.

# Decisions I made

- `Read(relative, kind)` expects a library-relative path. The source root is not title evidence. Absolute paths remain accepted for existing display callers, which currently supply them; identification consumers must strip their admitted root before reading.
- File and folder candidates are preserved rather than requiring equal names. Generic filenames such as `movie`, `video`, an opaque hash or a disc-stream number fall back to a useful folder. A meaningful filename wins even when it names a different show. Equal names may borrow a missing folder year; unrelated folder years do not become file years.
- Confidence measures syntactic evidence strength, not provider-match probability. The matcher must still validate candidates and IDs. Provider IDs remain ordered file first, then title folders; conflicting hints are not silently discarded.
- An unadorned final year remains title text, with a weaker shortened candidate and year hint. Explicit bracket years, adjacent years and scene release tags strengthen release-year evidence. Scene names also retain an uncut title alternative, since a name alone cannot prove whether a bare number belongs to a title. This preserves `1917`, `Space 1999`, `Cyberpunk 2077 Edgerunners` and `Blade Runner 2049` while retaining useful search alternatives.
- Episode ranges expand to at most eight links. Numbers remain bounded at four digits; explicit episode zero is returned as evidence. Picture dimensions, channel-layout codec tags, title-embedded marker text and unsupported bare numbering do not silently become episodes. Explicit lists such as `02x03 - x04 - x15` preserve every listed number within the bound.
- Dates carry the air date independently of folder season evidence. The existing catalogue adapter keeps its current date representation (year/day-of-year), without rejecting a numbered season folder. Air dates retain the previous minimum year of 1900.
- A descriptive episode title ending in `Part 1` is not a stacked part. A standalone stack suffix or a separate trailing dash suffix is a part hint. Film sequel titles such as `Harry Potter and the Deathly Hallows Part 1 (2010)` keep their title text.
- The existing catalogue still rejects attachment of episode zero and stacked episodic parts; its schema/association model is outside this lane. The reader supplies their evidence now. Disc stream grouping is likewise an identity-lane operation, not a name-reader database operation.
- Extras need an owning media folder below the library root. Top-level `Extras` shows and `Shorts` categories remain titles. Explicit filename suffixes outrank a conflicting extras-folder type. A film named `Trailer` remains a film when its own folder corroborates that name.
- Exact sample files/folders and scene sample markers are excluded; well-named films such as `Sample (2020)` remain. Root directories are not filtered as entries.
- Ignore patterns use Plex's documented line syntax, rather than Git ignore semantics. Root basename patterns recurse; nested basename patterns apply in their containing directory; slash patterns are relative to their containing directory. `*`, `?` and character classes are supported; `!` is literal, not negation. Invalid/oversized lines are skipped individually and reported with line numbers. Bounds: 64 KiB contents, 512 patterns, 1 KiB per pattern; name-reader bounds are 16 KiB per path and 128 components.
- Loading ignore-file contents would change the inventory contract, which explicitly avoids content reads, and requires lifecycle/invalidation work beyond a small local walk edit. Supplied pure functions and the built-in gates are the local integration performed here.
- Reference sources: [Plex naming](https://support.plex.tv/articles/naming-and-organizing-your-movie-media-files/), [Plex exclusions](https://support.plex.tv/articles/201381883-special-keyword-file-folder-exclusion/), [Jellyfin identifier forms](https://jellyfin.org/docs/general/server/metadata/identifiers/). Jellyfin naming tests were read as a convention checklist; no comparison-project implementation was copied.
- Execution host: `ehler-homelab-mini-1`. Context-sync failed because its lock directory is read-only. Existing project, project-standards, verification and attribution guidance was read; no successful refresh is claimed.

# Bugs

- B-05.1 — already fixed on the base according to the fix log; scan cadence is outside this lane.
- B-05.2 — not fixed here; changing-directory recovery belongs to scanning.
- B-05.3 — not fixed here; changed-file analysis/invalidation belongs to scanning.
- B-05.4 — not fixed here; unreadable-subfolder isolation belongs to scanning. Ignored system entries now avoid unnecessary local stat/probe.
- B-05.5 — not fixed here; dangling-link recovery belongs to scanning. Existing root/symlink safeguards are retained.
- B-05.6 — fixed for newly enumerated local and managed publication paths through built-in exclusions. Existing cached/catalogued exclusions need the scanner's reconciliation/invalidation work.
- B-05.7 — fixed in the shared reader and catalogue display adapter, extending the base's already-fixed unambiguous cases. Ambiguous bare scene years retain title alternatives. The matcher must adopt this reader for the end-to-end fix.
- B-05.8 — not fixed here; title/version identity and history preservation belong to identity. Version/edition hints are supplied.
- B-05.9 — not fixed here; accepted associations still need identity-lane rename reconciliation. The parser version bump retries issues only.
- B-05.10 — fixed in the episode adapter: flat folders, folder years, translated/abbreviated seasons and intervening disc folders are read.
- B-05.11 — fixed in the episode adapter: folder equality is no longer a gate; filename identity/season wins, with folder alternatives retained.
- B-05.12 — already partly fixed on the base; retained and extended resolution/codec filtering and marker boundaries, including smaller picture dimensions. Genuine `101x102` and `1x264` remain valid markers.
- B-05.13 — fixed in the reader for dates, title text ending in Part N, x-ranges/lists and marker text in episode titles. Eight-link/numeric bounds remain. Episode zero and real stacks are read but still require identity/schema support for attachment.
- B-05.14 — fixed in the extras adapter: top-level show/category names are not swallowed and a season hierarchy interrupts accidental extras inheritance.
- B-05.15 — not fixed end-to-end; disc roots and their owner paths are returned, but current catalogue stream grouping is outside this lane.
- B-05.16 — fixed for explicit samples and bare/suffixed trailers in media-owner context. Well-named films called Sample or Trailer are retained. Arbitrary `backdrops` layouts are not newly classified.
- B-05.17 — fixed for newly read display names: release tags, provider hints, editions, hashes, parts and whitespace are separated from titles. Old accepted catalogue rows are not rewritten here.
- B-05.18 — not fixed here; persistent file identity belongs to scanning/identity and needs mount testing.
- B-05.19 — not fixed here; directory timestamp reliability belongs to scanning and needs filesystem testing.
- B-05.20 — not fixed here; bookkeeping pruning belongs to scanning.
- B-05.21 — not fixed here; selective analysis scheduling belongs to scanning.
- B-05.22 — already fixed for local sidecar lookups on the base according to the fix log; unchanged-sidecar invalidation belongs to scanning.
- B-05.23 — not fixed here; scan defaults/watchers belong to scanning/settings.
- B-05.24 — not fixed here; missing-title/trash policy belongs to identity.
- B-05.25 — not fixed here; disk deletion/trash behaviour belongs to identity/administration.
- B-05.26 — not fixed here; source-scoped warning attribution belongs to scanning. Fewer valid names now become naming issues.
- B-05.27 — not fixed here; canonical show identity belongs to identity/providers. Candidate names, years and IDs are supplied separately.
- B-05.28 — already partly fixed on the base according to the fix log; Windows inventory/platform acceptance belongs to platform/scanning.
- B-05.29 — not fixed here; tolerant bounded sidecar parsing belongs to local metadata.
- B-05.30 — already fixed for administrator folder notices on the base according to the fix log; Jobs projection is outside this lane.
- B-05.31 — not fixed end-to-end; `Disc 1 - Studio`/`Disc 2 - Live` and separator variants are read by the pure package, but existing audio album association must adopt those hints in music-audiobooks.
- B-05.32 — fixed for new reader evidence by NFC normalization. The base already compared Unicode-equivalent folder/file spellings; stored identity reconciliation is still outside this lane.
- B-05.33 — not fixed here; symlink alias identity/cache handling belongs to scanning.
- B-06.1 — fixed in shared name reading/catalogue display, with numeric-title and explicit-year corpus coverage. Not fixed end-to-end until providers-matching replaces its separate `screenName`/year parser with this evidence.

# API changes

No HTTP routes, route shapes, removed routes or client-core changes.

New internal Go API in `portico.local/server/internal/naming`:

- `Read(relative, kind) Reading`: ordered typed `Evidence[T]` slices with `Value`, `Source`, `Path`, `Confidence`, plus skipped-evidence diagnostics. Fields include `Titles`, `Years`, `Seasons`, `Episodes`, `AbsoluteNumbers`, `AirDates`, `Parts`, `Editions`, `Versions`, `ProviderIDs`, `Extras`, `Discs`, `TrackNumbers`, `DiscNumbers`, `Artists`, `Albums`.
- `Reading.PreferredTitle() (string, int)`: minimal display selection for current callers. Matchers should inspect alternatives instead.
- `SeasonFolder`, `DiscFolder`: pure layout helpers.
- `ReadExtras(relative, kind)`, `ExtraTitle(filename)`: contextual classification/display helpers.
- `DiscLayout(relative)`: disc-root hints, including an input that is the disc directory itself. That terminal-directory source is `folder_name`; ordinary file-relative sources remain `file_name`, `parent_folder`, `grandparent_folder`, `ancestor_folder`.
- `IgnoreName`, `IgnorePath`: built-in entry/path exclusions.
- `ParseIgnore(scope, contents, root) (IgnoreRules, []IgnoreDiagnostic)` and `IgnoreRules.Match(relative, directory)`: bounded pattern parsing/matching with line-specific skip reasons.

Existing `catalog.FilenameTitle`, `catalog.ParseEpisode`, `catalog.ClassifyExtra` and `catalog.ClassifyExtraFor` signatures are unchanged. `EpisodeParserVersion` changes from 2 to 3. No compatibility routes or alternate persistent documents were introduced.

# Schema changes

None. No schema sync was required.

# Owner or viewer settings added or wired

None. No screens were added. Built-in exclusions are default scanner behaviour; ignore files are owner-authored files whose loading remains a scanner integration task.

# Needed from others

- **providers-matching:** import `naming`; consume all usable title/year alternatives and file-first provider hints, resolve IMDb IDs in the appropriate media type, and replace the independent filename/year cleaners. Preserve owner/provider metadata precedence. No matching thresholds or network calls changed here.
- **identity:** attach versions/editions, parts and disc roots using the returned evidence; reconcile accepted automatic mappings on rename; support episode zero if the release model permits it; normalize canonical keys. Name-reading does not implement any of these persistent operations.
- **later scan integration:** pass source-relative paths and library kind; load `.plexignore` and `.ignore` through bounded admitted storage reads, including managed sources; carry root/nested scope and diagnostics; invalidate cached traversal when rules/policy change; prune previously cached excluded directories and reconcile existing objects through the normal missing path. Keep root reachability, page cursor/revision and symlink guarantees. The local helper and publication gate already apply built-in rules, but do not force old unchanged directory caches to be listed again.
- **music-audiobooks:** consume `DiscFolder`, `DiscNumbers`, `TrackNumbers`, `Albums` and `Artists` only as path hints, subordinate to usable tags, and update album/disc association. No audio repository or tag parsing was changed.
- **strings/owner diagnostics:** translate new internal path/ignore-budget skip reasons when scanner diagnostics become owner-facing. No new visible strings/screens are part of this patch.
- **documents:** catalogue architecture and naming contracts that require folder/file agreement or filename-only titles need to describe shared evidence and file precedence. `INVENTORY_CONTRACT.md` must be updated by the scan integration for ignore-file content reads and by identity for disc-layout grouping; today's explicitly ordinary VOB/M2TS treatment remains unchanged here.

# Test notes for other environments

Verified on Ubuntu with `GOFLAGS=-p=2` and the shared programme Go cache:

- `go test ./internal/naming`: all 657 naming rows, separate extras/ignore tables, provenance, ambiguity, bounds and fuzz seeds pass.
- `go test ./internal/catalog -run 'Test(FilenameTitle|ShowFolder|EpisodeNaming|EpisodeStack|ExtraClassification|DetailExtrasGrouped|ReparseEpisodic|EpisodeHierarchy|AbsoluteAnime)'`: selected parser, SQLite association, manual correction, extras/detail and issue-reparse checks pass.
- `go test ./internal/storage -run 'TestInventory(Ignore|PagesReplay|RootReplacement)'`: exclusions preserve page progress; existing cursor/revision replay and root/symlink rejection checks pass.
- `go vet ./internal/naming ./internal/catalog ./internal/storage` and permitted `go build ./...` pass.
- Five-second, two-worker fuzz runs pass: `FuzzRead` executed 14,136 inputs; `FuzzIgnore` executed 9,777 inputs. These are short robustness checks, not exhaustive proof.
- Touched packages cross-compile for Windows amd64 and macOS arm64 with `GOOS=windows` / `GOOS=darwin`, `GOARCH=amd64` / `GOARCH=arm64`, `go build ./internal/naming ./internal/catalog ./internal/storage`.
- `git diff --check` passes. No full test suite or production scan ran.

Cold follow-up procedure: from the worktree's `server` directory, rerun the commands above on Windows/macOS; for native tests run the storage ignore/cursor checks against a temporary root. Pass means the root contains only the expected visible film/film-folder entries, every page progresses, ignored entries never become observations, and an escaping enabled symlink remains rejected. For a source-level test after integration, use a disposable baseline database and files named in `testdata/corpus.json`; inspect title/episode assignments, retained competing hints, extras owner/type and absence of sample/system objects. Change a nested ignore file and rescan; pass means its scope and invalidation work without marking an unreachable root missing. That final source-level test depends on the other lanes' integration and has not been run here.

Snapshots: `snapshots/naming/001` — Read media names from files and folders with source evidence. Final cumulative snapshot `snapshots/naming/002` — Recognize media layouts and exclude hidden files and samples.

# Not finished

No remaining pure name-reader implementation task is known from the lane brief. End-to-end matching, persistent version/part/disc grouping, episode-zero attachment, audio album grouping, accepted rename repair, ignore-file loading/invalidation and old-cache reconciliation remain with the owning lanes described above. Existing catalogue rows were not repaired or migrated. Cross-compilation does not establish native filesystem/runtime acceptance. Snapshot 002 is the final tree; no worktree changes follow it.
