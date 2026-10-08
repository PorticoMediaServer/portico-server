# Administration pages

These are the owner administration surfaces: deleting media, configuring a
library, configuring a live source, DVR defaults, library channel presets and
server maintenance. The machine-readable contract is
`admin-libraries.openapi.yaml`; this document explains the decisions behind it.

Everything here is owner-only except `GET /v1/library-channels/{id}/logo`,
because a channel logo is part of what a viewer sees. A Hosted account carrying
the owner role is not this server's owner: administration always resolves through
the locally managed owner and the server's installed membership policy.

## The shape every page shares

- One response envelope: `{protocolVersion, serverId, result}`.
- One document per page, carrying a `revision` and a `digest`.
- One write envelope: `{expectedRevision, operationId, settings}`. A write whose
  `expectedRevision` does not match the stored revision is refused with
  `409 administration_conflict` and nothing is stored.
- `operationId` makes a write idempotent. Replaying the same identifier with the
  same body returns the first result rather than applying the change twice;
  replaying it with a different body is refused. Identifiers are 8–128 characters
  of `[A-Za-z0-9-_.]`.
- Every list pages by an opaque `cursor` with `limit` between 1 and 200
  (default 40). A cursor is fenced to the listing that minted it, so replaying
  one against a different listing is refused with `409 stale_continuation`.
- Owner authority is checked before the work starts, again inside the same
  transaction as the write, and again before the response is written. A session
  revoked mid-request cannot complete a mutation.
- A validation refusal names the exact fields: `{"error": {"code":
  "invalid_administration_input", "fields": ["settings.folderTemplate"]}}`.
- Unknown query parameters are refused. A typo is never a silently ignored
  filter.

Storage: one table of documents keyed by scope (`library:<id>`,
`live-source:<id>`, `live`, `dvr`, `maintenance`), one table of operation
receipts, and one small table per listable thing (trash entries, recording
groups, block presets, channel logos, channel number overrides, backups). No
domain state is duplicated: each page reads the domain's own tables and writes
only its configuration document.

## Deleting media

A delete is three steps, because it is the one page action that destroys data.

1. **Preview** — `POST /v1/items/{id}/delete/preview`, or
   `POST /v1/admin/media/delete/preview` with up to 200 `itemIds`. The preview
   lists every file with its size and whether it is still present, marks files
   another item also uses (those bytes stay), counts everything that refers to
   the item (collections, playlists, saved positions, library channel slots,
   published recordings), reports the library's `allowMediaDeletion` and
   `trashRetentionDays`, and returns the `confirmation` text and the `revision`.
2. **Confirm** — `confirmation` is the item's title for a single delete and the
   target count as a decimal string for a bulk one. `confirmationKind` says
   which. A mismatch is `409 confirmation_mismatch`.
3. **Delete** — `POST /v1/items/{id}/delete` or `POST /v1/admin/media/delete`
   with `{deleteFiles, confirmation, expectedRevision, operationId}`.

`expectedRevision` is the deletion fence: the sum of the `library_revisions`
revisions of every library the targets sit in. Anything that changes a targeted
library moves it, so a delete cannot be applied against a library that has
changed since the preview.

`allowMediaDeletion` is off on a fresh library. Until an owner turns it on, every
delete against that library is `403 administration_denied`, and a bulk delete
that touches even one such library is refused whole rather than half-applied.

`deleteFiles: false` removes the catalog entry and leaves the bytes where they
are; `filesRetained` says so, and the next scan readopts them. `deleteFiles:
true` moves each file into `<state>/trash/<entryId>/` when the library's
retention is above zero, and removes it outright when the retention is zero. A
move that crosses a filesystem falls back to a copy-then-remove, which is the
normal case for a library on its own volume. Files shared with another item are
counted in `filesKept` and left alone.

The trash is `GET /v1/admin/trash`, and `POST /v1/admin/trash/{id}/restore` puts
an entry's files back at their original paths. A path that is occupied again is
reported in `conflicts` rather than overwritten, and the entry stays held so the
owner can act on it. `reindexRequired` is always true: restoring returns bytes,
not catalog rows, and the next scan of the library readopts them.
`POST /v1/admin/trash/empty` purges permanently and takes the number of entries
it will purge as its confirmation; `expiredOnly` keeps entries whose retention
has not run out.

The default retention is the `library.trashRetentionDays` owner setting, which a
fresh library inherits and any library overrides on its own page. That setting is
nullable: a client that does not send it leaves the shipped 30 days alone rather
than switching the trash off.

## Libraries

**Folder picking** — `GET /v1/admin/filesystem`. With no `path` it returns the
platform roots: present drive letters and the user profile on Windows, plus a UNC
hint because network neighbours cannot be enumerated portably and a
`\\server\share` path is typed; `/`, `/Volumes` and the home folder on macOS; `/`
plus every real mount point from `/proc/self/mountinfo` (pseudo filesystems
filtered) on Linux. With a `path` it returns one page of that directory, sorted
case-insensitively. Dotfiles, the Windows system folders (`$Recycle.Bin`, `System
Volume Information`, `Recovery`) and the kernel trees (`/proc`, `/sys`, `/dev`,
`/run`) are never listed, and browsing into a kernel tree is refused. At most
20,000 names are read from one directory; `truncated` says when there were more.
`writable` is probed by creating and removing a file, because mode bits answer
the wrong question on a network share. Files appear only with
`includeFiles=true`, for the pickers that choose a file rather than a folder.

**Providers** — per media kind, a library chooses a provider, optionally an API
key override, and a language and region. `GET /v1/admin/metadata-providers`
publishes which providers serve which kinds, which accept a key and which accept
a locale; a selection outside that is refused. A key is write-only: reads return
`apiKey: ""` with `apiKeySet: true`, and an empty `apiKey` on a write keeps the
stored one, so a client that never receives a key can still save the form.

**The analysis matrix** — `GET /v1/admin/analysis-operations` publishes every
operation the server knows, grouped by what it costs:

| Cost class | Meaning | Operations |
| --- | --- | --- |
| `metadata-only` | One small read per file | `probe`, `local_metadata`, `artwork_extraction`, `subtitle_discovery`, `chapter_read` |
| `single-pass-read` | Reads the file once, no video decode | `deep_stream_analysis`, `subtitle_extraction`, `loudness`, `audio_fingerprint` |
| `decode` | Decodes frames; sustained CPU or a busy encoder | `trickplay`, `chapter_images`, `video_preview`, `segment_detection`, `credits_detection` |
| `network` | Contacts a provider off this machine | `subtitle_download`, `lyrics` |

Each operation publishes the media kinds it applies to, whether it is on by
default, whether it writes artifacts (which is why the storage page has a
category for it), and the operations it `requires`. A selection that enables an
operation without its dependency is refused by name, because the operation would
otherwise silently never run.

**Generated navigation** — trickplay interval, tile width and maximum tile count;
the chapter thumbnail mode (`none`, `embedded`, `generated`); and whether a video
preview is produced and how long it is. Numeric values clamp into their published
range rather than failing a save; enumerations reject.

## Live sources

Server-wide values live on `GET/PUT /v1/admin/live/settings`: stream buffer
seconds, retry window, user agent, guide days, whether logos are imported and
whether tuner discovery may send packets. Per-source values live on
`GET/PUT /v1/admin/live-sources/{id}/configuration`, and the response carries
`effective` — the defaults with this source's overrides applied — so no client
recomputes them. A null override inherits.

A source's `kind` is `playlist`, `hdhomerun`, or `xmltv-guide`: a **standalone
guide** that publishes programmes for channels that come from somewhere else and
has no stream of its own. A guide-only source therefore refuses stream tuning and
refuses to name a guide source, and a streaming source names its guide with
`guideSourceId`.

Filters are three include/exclude lists — categories, countries, keywords — each
folded for duplicates and case. An empty value list means the filter is not
applied whatever the mode says.

**Renumbering and guide mapping** — `GET/PUT
/v1/admin/live-sources/{id}/channel-map` pages the active generation's channels.
`sourceNumber` is what the source published; `number` is what the server serves —
the sequential numbering when that mode is chosen, or a per-channel override.
Overrides also carry `guideChannelId` and `hidden`. An override with an empty
number, no mapping and `hidden` false clears the row, and two overrides may not
claim the same number. The write is fenced on the source configuration's
revision, so a renumber cannot land on top of a numbering-mode change the caller
never saw.

**Discovery** — `POST /v1/admin/live-sources/discover` sends two requests from
one socket, because a device may answer either and an owner should not have to
know which: the HDHomeRun discovery packet (type `0x0002`, the tuner-type and
wildcard-device TLVs, the protocol's little-endian CRC-32) to every interface
broadcast address and the global broadcast on UDP 65001, and an SSDP `M-SEARCH`
to `239.255.255.250:1900`. Broadcasting per interface rather than setting a
platform socket option is what makes this behave the same on Windows, macOS and
Linux. A binary reply is accepted when it carries the discovery reply type; an
SSDP reply is accepted only when it identifies itself as a tuner, since every
device on the network answers `ssdp:all`. Each responder is reported with the
address its `discover.json` and `lineup.json` are served from, and `configured`
says whether a live source already points at it. The sweep waits at most ten
seconds and does nothing while `discoveryEnabled` is off, which it says.

**Logos** — `POST /v1/admin/live-sources/{id}/logos` imports the logos a source
advertises, validating each image exactly as an upload is validated. Nothing is
fetched unless the composition root supplied a fetcher and the source's effective
`logoImport` is on; otherwise the result explains why it did nothing.

## DVR

`GET/PUT /v1/admin/dvr/settings` reads and writes normalized runtime padding and retention defaults. Supported retention is `keep-all`, `keep-count` or `keep-days`. New groups snapshot these defaults for null options; direct recordings omit `options` or use `useDefaults: true`; direct rules use `useDefaults: true`. Existing intent keeps its saved options. Conversion is `none`, profile is `original`, tuner preference is `first-available`. Folder templates must be empty, sidecars and keep-until-watched false; unsupported controls are refused. The folder-token vocabulary is empty.

`GET/PUT /v1/admin/dvr/recording-permissions` is server-owner-only. A grant identifies an explicit `{authority, accountId, profileId}`, `enabled`, `inheritProfiles`, and current `revision` (zero on create). Enabling validates current target membership. Server ownership grants no implicit recording permission. A child profile inherits only when explicitly enabled by the owner; explicit profile denial overrides inherited permission. Revocation affects admission and durable capture checks without deleting existing recordings.

`GET /v1/admin/dvr/tuners` shows, per source, the tuner count, what is recording
now, and what is scheduled in the next 48 hours. `conflicts` is how many upcoming
recordings cannot all fit, computed by sweeping the start and end points against
the tuner count — so the page reports a real over-subscription rather than a
guess. A server with no live channel store reports an empty view, not a failure.

`GET/POST/PUT/DELETE /v1/admin/dvr/recording-groups` writes real DVR series rules. Creation requires explicit `owner` and `anchor` (`sourceId`, `channelId`, `generation`, `programmeId`) from the current guide; `match` is that programme's actual series ID. The selected owner needs recording permission. Keyword-only groups and unsupported folder choices are rejected. `newEpisodesOnly` requires reliable guide evidence. Null options snapshot current defaults when saved. Removing a group stops further scheduling and preserves previously scheduled recordings. Historical ownerless rows are shown disabled with `status: needs-owner-and-guide-selection`; they never acquire an inferred owner.

## Library channels

`GET /v1/admin/library-channels/criteria` publishes the selection vocabulary from
the catalog's own attribute projection: with no `field`, the fields the catalog
actually holds and how many distinct values each has; with a `field`, that field's
values with the number of items carrying each. The page therefore never offers a
filter the library cannot answer.

`GET/POST/PUT/DELETE /v1/admin/library-channels/block-presets` are named,
reusable block definitions: criteria clauses (`includes` / `excludes` over a
field), an ordering (`shuffle`, `sequential`, `recent`, `least-played`), a
duration, an optional placement in the day and an optional day set. Names are
unique after case and whitespace folding. Removing a preset leaves channels
already built from it alone.

`POST /v1/admin/library-channels/{id}/logo` takes `multipart/form-data` with a
`file` part. The container is identified from the bytes, never from a declared
content type or filename: JPEG, PNG and WebP, up to 10 MiB, 16 to 4096 pixels on
each side. The image is proved decodable before anything is written — a hostile
image never holds a write lock — and is stored under its own digest, so an
identical upload costs nothing. `GET /v1/library-channels/{id}/logo` serves it to
any signed-in viewer with a strong `ETag`.

`GET /v1/admin/library-channels/{id}/health` reports whether a channel has a
published generation, how far its schedule reaches, how many slots and candidates
it has, whether a generation is queued, and a sentence saying what is wrong: off,
never generated, a reported failure, or a schedule that came out empty because
the criteria matched nothing.

## Maintenance

**Backups.** A backup is a plain folder `<state>/backups/<UTC timestamp>/`
holding `portico.db` — a consistent copy of the database made with SQLite's
online backup while the server runs — the state files needed to come back as
the same server, and `manifest.json` with integrity hashes. Nothing is
encrypted; the folder permissions protect it. `GET /v1/admin/backups` lists
them with the running backup's progress and the last restore's outcome.
`POST /v1/admin/backups {operationId}` starts one (idempotent per operation
id) and `DELETE /v1/admin/backups/{id}` removes one. The nightly window runs
one automatically; `backupKeepCount` keeps the newest scheduled and manual
backups, plus the newest three pre-restore copies.

`POST /v1/admin/backups/restore {operationId, source}` stages one of the
listed backups or a server path — a backup folder or a bare `.db` file that
is a Portico database — and answers `{staged, restartRequired, validation}`.
The server then restarts, and the next start moves the current state into a
pre-restore backup, moves the staged copy into place, and runs the ordinary
migrations; a failure moves the pre-restore copy back automatically and
records it. A bare `.db` replaces only the database.

**Storage care.** `GET /v1/admin/storage-usage` measures the state directory by
category: artwork, trickplay, prepared media, conversion working files,
subtitles, live buffer, downloads, logs, trash, backups and recordings. Each
category names its directories, whether the page may clean it, whether the bytes
are regenerable, and which retention setting governs it. Recordings and backups
are content and are never cleanable. `POST
/v1/admin/storage-usage/{category}/cleanup` removes files older than the
category's retention (or an explicit `olderThanDays`), takes the category
identifier as its confirmation, and refuses the trash, which has its own
endpoint because emptying it is a different decision.

**Updates.** `GET /v1/admin/updates` reports the running build and, when the
`updates.feedUrl` owner setting names a feed, the newest build that feed lists on
the `updates.releaseChannel`. Versions compare as dotted numbers with any
`-suffix` sorting below the same numbers without one, so a release candidate
never reads as newer than the release. Nothing is installed: `autoInstall` is
always false.

**Windows and retention.** `GET/PUT /v1/admin/maintenance/settings` holds the
maintenance windows — a day set, a start minute, a duration, an optional IANA
timezone and the tasks the window admits — with cadence presets (`nightly`,
`weekdays`, `weekends`, `weekly`, `custom`) published alongside the day set each
means, so a client never asks an owner to tick seven boxes. Retention is per
category with deliberately long maxima (ten years for artwork, trickplay,
prepared media, subtitles and trash), because an owner with a large disk
should be able to keep them effectively forever. Backups are kept by count
(`backupKeepCount`), not by age.

**Moving a server.** Stop Portico, copy the whole state folder to the new
machine, point Portico at it (`PORTICO_STATE_DIR`), and start: it just works.
Portico's own files are stored absolute and rebased to the new folder on first
start. Media library paths are absolute too and still point at the old
machine's mounts, so change them in the console when the new machine mounts
media elsewhere.

## Settings registry

Three server-wide values live in the owner settings registry rather than in these
documents, because they are server policy that other surfaces read:
`updates.releaseChannel`, `updates.feedUrl` and `library.trashRetentionDays`.
They are read and written through the console settings endpoints like every other
registry field. `updates.releaseChannel` falls back to `stable` when a client
omits it and `library.trashRetentionDays` is nullable, so a client written against
an earlier registry revision can still save the settings document.

`GET /v1/admin/dvr/recording-owners?limit=100&cursor=…` supplies current account/profile choices for the owner-only recording-permission editor. The result has `items` (`owner`, `accountName`, `profileName`) and `nextCursor`; limit is 1–200. Continue through empty filtered pages until the cursor is empty. All candidates pass current membership/claim checks; this endpoint neither grants permission nor contacts Hosted. Stale, revoked or deleted identities are not returned.

Scans accept `?mode=full` or `?mode=integrity` on the existing scan endpoints.
Without an explicit mode, a new source uses first-import mode; subsequent scans
reuse unchanged directory fingerprints. Known subdirectories are still checked
individually. At least once every seven days, an automatic scan uses integrity
mode to detect replacements that did not change their parent directory mtime.
Full and integrity scans enumerate every directory. All modes retain the existing
root-identity, cancellation, and two-observation missing-file safeguards.
