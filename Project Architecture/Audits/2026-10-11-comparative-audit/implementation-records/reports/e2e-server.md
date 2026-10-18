# Outcome

A reusable production-process runner now lives at `scripts/e2e-server.py`. Its final run passed **18 scenario groups and 642 HTTP requests**, including deliberate permission, capacity and preparation refusals. All server/listener traffic used disposable localhost state, unclaimed Direct Sign-In, generated credentials and local metadata. The runner stops its supervised process tree on completion or failure.

Three server defects were reproduced and fixed: scanned ASS/SSA subtitles lost their styling; the typed playback API returned 500 at conversion capacity; selected-file lookup returned an empty asset ID before scanning its query result. Each fix has a focused regression test. No commits, pushes or deployments were performed.

The finished run verified:

- First-run browser bootstrap, owner creation, setup completion and subsequent owner/member Direct Sign-In.
- Film names with/without years, scene naming and a deliberately misspelled title; two versions of Arrival; an edition; two multipart files; extras; an ignored sample; a broken file isolated to a scan issue; and a hostile STRM descriptor inventoried without playing/contacting its destination.
- Two shows including a loose episode; season-folder E01; filename season 2 overriding its season 1 folder; absolute-numbered anime; a tagged two-disc album; a compilation; and a chaptered M4B. Expected top-level counts: 12 films, 2 shows, 1 anime show, 2 albums, 1 book. The twelfth film is a dedicated longer load fixture.
- Browse/search during scans, sorts in both directions, genre/year filters, genre/collection/show/album group scopes using the common bounded browse vocabulary, content/category/collection reads, title details and owner attention/skip lists.
- Direct MP4, copy-only MKV remux, HEVC 10-bit conversion, embedded ASS burn-in, original ASS delivery, embedded text discovery, UTF-8/UTF-16/Windows-1252 sidecar delivery with the accented text preserved, actual playlists/segments/init files, quality changes and timeline progress. The HEVC fixture has six audio channels.
- Automatic selecting mpv to avoid HEVC conversion, a declared native MKV/HEVC profile playing directly, and measured FLAC/M4B direct audio bytes. These are capability/API checks, not physical engine execution.
- Title watched/unwatched and show mark/undo; rating-restricted profile concealment across the browse/search/content requests, hidden-title detail/options/playback refusals, and continued visibility of the permitted title.
- Four API scopes and forbidden cross-scope/session-only calls; MCP initialize/list/call; local test and real scan webhooks with HMAC-SHA256 verification and delivery readback; and a real held SSE response delivering `personal.changed`.
- Physical deletion of one alternate version while retaining its title; paths-changed queued scans and outside-path reporting; missing-file marking and restoration under the same ID with personal state preserved; an unavailable library root without missing-title marking; and library removal retaining its media.
- A live FFmpeg conversion occupying the owner's one conversion slot, another conversion receiving the correct retryable refusal, and 16 owner settings reads across eight concurrent clients. Backup, name change, restore, automatic supervised restart, retirement of the old session, fresh sign-in and readback of the backed-up name/catalogue/API key all passed.

Final evidence: `_program/scratch/e2e-server/final-run.log` and `_program/scratch/e2e-server/run-20261013-131637-32590e/`. The latter includes private credentials, state, server logs, per-library scan/issues, recognition/details, queries, playback, file lifecycle, unreachable-root, event and restore evidence plus `results.json`. **Keep that directory private; do not add it to Git.**

| Final timing | Result |
|---|---:|
| Fixture generation | 18.003 s |
| Server ready | 2.369 s |
| Five initial library scans plus publication/read traffic | 14.790 s |
| Movies / Shows / Anime / Music / Books scan wall time | 7.643 / 2.514 / 0.971 / 1.974 / 0.979 s |
| Browse (202 successful calls, including concurrent scan traffic) | reported median 11.06 ms; max 98.09 ms |
| Search (123 successful calls, including concurrent scan traffic) | reported median 20.31 ms; max 71.47 ms |
| Owner route with occupied conversion capacity and a live FFmpeg job | max 83.82 ms |
| Backup/restore/restart/readback group | 4.560 s |
| All scenario groups combined | 55.277 s |

These are small synthetic-library localhost observations on shared mini-1, not a large-library or production concurrency claim.

# Decisions I made

- Use the production build and system `/usr/bin/ffmpeg` plus `/usr/bin/ffprobe`. State, generated media, keys/passwords and logs are created only in unique private run directories under the authorized scratch folder.
- The host refuses bubblewrap user namespaces. Kept Portico's existing default fallback (open inputs, resource/output bounds, protocol restrictions, sanitized environment and process supervision); did not disable or weaken a security requirement or add a test-only bypass.
- Scanned ASS/SSA must preserve its authored style, so discovery marks it as styled and first delivery caches it as styled. Devices that render ASS receive the original; other devices can burn it. Explicit text-rendering paths still exist.
- Reuse the existing `conversion_capacity` error meaning. The typed API now says 429, `retry: same_request`, `Retry-After: 2`, distinguishing temporary server conversion occupancy from the account's 409 stream policy refusal.
- Audio measurement remains background work. Corrected the selected token and let the runner retry boundedly after measurement, rather than making playback synchronously decode a complete audiobook.
- Wait for asynchronous catalogue projections when validating scan publication/missing/restored rows, and concurrently prove that browse/search still answer while scanning.
- Add a 180-second synthetic 720p load fixture so the owner reserve probe runs beside a live conversion. Other fixtures are about 12 seconds and generated from a synthetic test pattern/sine wave.
- Keep name-only local NFO credits unlinked to a canonical provider person; a name alone is not evidence of a canonical identity. Do not inject fake database people to make that test appear exercised.
- Context sync failed because the sandbox cannot write its lock. Read the existing canonical project, standards and verification guidance directly; no shared guidance was edited. Host: `ehler-homelab-mini-1`.

# Bugs

This lane brief assigns no domain/audit document or B-number list. The following are new runtime findings against this base; they are not a claim to resolve the historical 588-entry register.

| Finding | Status and regression |
|---|---|
| Scanned ASS/SSA advertised as external text; lazy cache hard-coded `Render: text`, rejecting burn-in and stripping styles | **Fixed.** `subtitles/persist.go` classifies styled formats, `lazy.go` preserves them. `TestDiscoveredASSCachePreservesStyleAndOriginal` verifies discovery, cache revision/rendering and authored original delivery. Real burn/original paths pass. |
| Full conversion capacity reaches typed session wrapper as an unmapped domain error, producing 500 | **Fixed.** Error definition/mapping and route error lists now publish retryable 429. `TestPlaybackV1ConversionCapacityUsesRetryableEnvelope` covers the actual registry wrapper and a wrapped domain error. Shared-message regression and real saturated-capacity run pass. |
| `SelectedAsset` evaluates `id` before its accompanying `Scan(&id)`, returning an empty token with nil error | **Fixed.** Scan completes before return. `TestSelectedAssetReturnsTheScannedVersion` verifies default, explicit second version and absent version. Real background measurement/direct FLAC/M4B delivery passes. |

Runner/expectation corrections were not server defects: settings are a complete document under `data`; detailed files are on `/detail`; editions belong to files; search uses `releaseYear`/`dateAdded`; categories/collections are film-only; profile restrictions belong to the account manager; admin scope includes all grantable scopes; grouped show episodes are read through seasons; the title tag names a playable audiobook file while album/book-folder evidence groups the book. Local-only scanning preserves a misspelling instead of invoking an online matcher.

# API changes

No route or request/success-response shape was added, removed or changed. No scope or restriction classification changed.

| Existing route | Previously at full conversion capacity | Now |
|---|---|---|
| `POST /v1/playback/sessions` | 500 `internal` | 429 `conversion_capacity`, same-request retry, two-second retry header |
| `PATCH /v1/playback/sessions/{id}` | Same unmapped domain failure could arise during conversion replanning | Same declared retryable error |
| `POST /v1/queues` and `POST /v1/queues/{id}:advance` | Shared start path lacked the published capacity contract | Same error declared on these shared-start consumers |

`go run ./cmd/apigen` regenerated OpenAPI and shared route/error contracts. Shared playback and queue messages now use the i18n catalogue's `error.playbackConversionCapacity`. Parsers/request types did not need a shape change.

# Schema changes

None. No migration or schema-generated artifact was changed. No compatibility path or old database was introduced.

# Owner or viewer settings added or wired

No setting added. Existing runtime settings were exercised:

| Setting | Type/default | Effect proved; screen |
|---|---|---|
| `maxConcurrentSessions`, `maxSoftwareSessions`, `maxHardwareSessions` | Integers; 0 means unlimited | Set to 1; second conversion refused while remux/direct scenarios work. Existing Playback settings screen; typed API error repaired. |
| `library.missingTitles` | Enumeration; `after_days` default | Test selects `never`; missing title remains and restores. Existing server settings. |
| Library `allowMediaDeletion` | Boolean; false | Enabled for Movies before deleting one version; physical file removed, title retained. Existing library settings. |
| `name` | String | Hot name change read back; backup restore reinstates old name. Existing console settings. |
| Library metadata agent | Enumeration; online default | All fixtures created with `local` before first scan. Existing library creation/settings. |
| `updates.feedUrl`, `lanDiscoveryEnabled` | Release-feed URL / true | Test clears feed and disables discovery; process also starts with discovery disabled. Existing console settings. |
| Session quality / capability `engineSetting` | Original / Automatic | Mid-session lower limit changes generation; Automatic chooses mpv for direct HEVC, Native honors declared native support. Existing viewer settings/player controls; runner drives the API. |

# Needed from others

- Native/TV/Vega lanes must synchronize changed canonical client-core/i18n/contracts packages; only this server worktree was writable. No new screen is required.
- Provide a legitimate PGS-bearing synthetic/sample file or a supported encoder for the deferred bitmap-subtitle runtime case.
- To exercise canonical person grouping entirely offline, provide provider-backed canonical person evidence through the supported metadata flow. This fixture's NFO name-only credits display correctly, but have no canonical `personId`; the people directory legitimately returns no linked person. Canonical person grouping was not falsely marked passed.
- Physical engines, subtitle appearance/fonts, HDR tone mapping, GPU/tuner behavior and native Windows/macOS execution remain the owning environment/device acceptance work. This lane does not establish them.

# Test notes for other environments

Re-run from the named repository root with an absolute private directory **outside the repository**:

```sh
python3 scripts/e2e-server.py --scratch /absolute/private/disposable-folder
```

The runner builds with `GOFLAGS=-p=2`, needs FFmpeg with libx264/libx265/FLAC/ASS and ffprobe, reserves loopback ports, makes new credentials/state each run, writes evidence there and stops the process tree. `--binary /absolute/path/to/portico-server` skips the build. The live-child reserve assertion uses Linux `/proc`; the process cleanup uses Unix process groups. The runner is currently a Linux/Unix tool, not a Windows harness.

Verification performed (limited scope; no full suite):

- Production server build after every server fix: pass.
- Focused `subtitles` discovery/cache/original/text regression tests: pass. Verbose rerun proves the new ASS regression executed rather than skipped.
- Focused `playback` selected-version, choice/audio tests: pass. A requested `playbackv1` regex matched no tests; no coverage is claimed for that empty selection.
- Focused HTTP capacity/session lifecycle, subtitle sidecar/burn/original and production-router tests: pass. Verbose subtitle rerun executed all three; none skipped.
- `GOFLAGS=-p=2 go vet ./internal/playback ./internal/subtitles ./internal/httpapi`: pass.
- `node --experimental-strip-types --test packages/client-core/test/playback-messages.test.ts`: 4/4 pass.
- `npx tsc --noEmit -p packages/client-core/tsconfig.json`: pass.
- Production apigen: pass. `git diff --check`: pass.
- Final production runner: exit 0, all 18 groups pass. Final log lists the two unexercised fixture cases explicitly.

**Linux with namespaces:** run the same command on a host allowing bubblewrap namespaces; confirm media qualification reports sandboxed, then all 18 groups pass and no child survives cleanup. This sandbox cannot establish that mode.

**Windows/macOS native acceptance:** use a packaged server and equivalent fresh local libraries. Publish native/text and ASS-capable device documents; fetch direct/remux/conversion, choose an ASS track, switch between burn and original, verify its appearance on the device. At conversion capacity verify 429 plus the retry header without leaking converter details. Play a previously unmeasured FLAC/M4B: its actual file must be queued, measurement must complete in the background and subsequent direct-audio plans must carry positive sample/frame facts. Verify backup restore restarts the worker and retires old sessions while preserving restored catalogue/API keys. Failure means any 500 at ordinary capacity, dropped style, permanently empty audio asset request, leaked restricted title, incorrect season, lost restored title ID/state, unresponsive owner route or partial restore.

# Not finished

- PGS runtime generation/delivery/burn-in was not exercised: installed FFmpeg has no PGS encoder and no source sample was provided.
- Canonical person-scoped sort/filter runtime case was not exercised because offline NFO provides only unlinked names. Genre, collection, show and album scoped sorting/filtering were exercised.
- No online provider match/confidence behavior, real device rendering, GPU/HDR/bitmap presentation, native Windows/macOS runtime or large-production-load acceptance is claimed. The requested local-only test intentionally avoids online providers.

All implemented fixes, the runnable Linux harness and their required local checks are finished. Secrets/state remain private under scratch, and no test process is intentionally left running.

Snapshots are cumulative root-level snapshots: 001 preserves subtitle styles, 002 publishes capacity refusals, 003 returns selected file IDs, and 004 is the final complete delivery snapshot including the finished runner. No worktree changes follow the final snapshot.
