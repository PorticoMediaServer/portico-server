# P04A inventory and scan lifecycle contract

This describes the implemented local/OS-mounted source boundary. Provider
registration, managed-rclone, native WebDAV and STRM completion belong to P04B.
The existing storage supervisor, source-access registry, catalog, jobs and
contract-32 playback ownership remain authoritative.

## Identity and revisions

`library_sources.id` is an opaque durable source ID, not a path. A migrated
primary source uses its existing library ID to preserve current root consumers.
A library can retain up to 64 source records. Each source has a configuration
`generation`, an opaque root `incarnation`, an observed root object identity and
an explicit confirmation state. A root locator edit retaining the same physical
identity preserves its incarnation. A replacement requires explicit owner
acceptance, changes incarnation and cannot silently adopt the old locations.
An unmounted backing directory is not the previously admitted mount.

`inventory_objects.id` is a durable location record; `asset_id` is the existing
physical asset identity; `item_assets` associates it with existing logical item
IDs. A pathname is a locator, never a logical identity. Exact observed physical
identity plus size and modification time may reuse a unique existing asset on
rename/move/hardlink. Ambiguous candidates do not auto-merge. A same-source
single-location move retains its location ID. Additional locations share an
asset; distinct copies are separate assets until explicitly associated with a
logical entry. Cross-volume copies without continuity evidence are not matched
by filename. Replacement retires the collided location without deleting the old
asset, item history or surviving aliases.

Owner `associate` adds a physical version to an existing same-library/type item
and records the target in `inventory_version_links`. It preserves old logical
history and does not infer title equivalence. Only unambiguous whole-item,
part-zero associations are admitted; episodic links must be whole-source.
Existing audio hierarchy identity is preserved across folder-only moves when
its admitted metadata grouping is unchanged.

`mediasource.InventoryEvidence.Revision()` hashes a versioned encoding of source
incarnation scope, provider kind, observed object identity, size, modification
nanoseconds, change token and optional provider version. Local evidence uses
the file's inode (without the device number, which a restart may change) and
change-time evidence. This is invalidation evidence,
NOT a cryptographic digest, `NoInPlaceMutation` claim or playback byte-version
grant. Contract-32 admission still performs its own validation. Analysis is
fenced by location revision, source generation/incarnation, policy revision and
current job state before publication.

## Durable inventory and completion

The existing startup constructs `ingestion.Service`, injects the guarded storage
client, and runs its worker. One durable active claim exists per source,
including paused jobs. Library scans fan out over enabled sources and coalesce
existing claims; the response retains the first job ID and adds all `jobs` IDs.
Up to four sources run concurrently, one bounded quantum per source. The scan
phases are `inventory`, `legacy_inventory`, `verifying`, `reconciling`,
`analysis_enqueue`, `basic_pending`, `basic_running`, then terminal completion.

Inventory pages contain at most 128 entries, including directory work. Each
page atomically commits observations, immediate provisional catalog entries,
change records, child-directory work and its continuation. A crash can replay an
uncommitted page; it cannot advance a durable cursor past uncommitted entries.
The adapter cursor is opaque and valid only for the admitted root and unchanged
directory revision. Linux uses per-record getdents cookies; the portable fallback
uses bounded-memory lexical pages and re-enumerates the directory. Very large
single directories on the fallback have a performance cost, not an unbounded
materialized listing. Changed revisions fail the run conservatively; explicit
retry starts a new inventory while retaining prior commits. Pause/resume and
restart retain the same committed continuation.

Completed directory identity/revision is rechecked before the inventory becomes
authoritative. Migrated path-only roots can publish observed files but require
owner confirmation before authoritative absence reconciliation. Legacy catalog
assets are adopted in bounded pages, retaining existing IDs; they are not swept
or discarded at database startup. `discovered` counts committed recognized
media observations, not a percentage of an unknown total. Unsupported images
count as warnings. `analyzed` counts committed analysis results, not all files.

`inventory_completed` means verified enumeration for that source run. It is
published before bounded missing reconciliation and optional analysis finish;
it is NOT a promise that the whole job or every analysis stage completed.
`inventory_runs.authoritative`, generation/incarnation and
`inventory_completed_at` record this distinction. Job states include `queued`,
`running`, `paused`, `cancelled`, `failed`, `complete` and
`complete_with_warnings`. A failed analysis does not erase committed inventory.

Only completed, current-root inventories contribute absence evidence. A file
that a complete listing of its folder does not contain is `missing` from that
scan on: there is no second scan and no waiting period. The nearest fully
enumerated ancestor is rechecked outside the database writer immediately before
each missing publication. Permission failure, interruption, changed directories,
source outage or root replacement is never an authoritative empty listing, and
neither is a root that lists nothing at all where the source held available
files before: that is what an unmounted disk looks like, so the scan fails as
source unavailable and nothing is marked. Reappearance clears the missing state
through a fresh admitted object observation.

Missing is not removal. A missing file is no longer a version of its title; a
title left with no file stays in the catalogue, marked missing with the time
its last file went (`catalog_missing_titles`). The owner's rule (immediately,
after a number of days, never; thirty days by default) decides when missing
files and missing titles are removed, and "empty now" removes them at once.
Removing a title keeps each profile's state under the title's identities so
the same title coming back gets it back. A file that resolves to a missing
title (the same provider identity, or read as the same title) re-attaches to
it.

A file at the same place with the same size and modified time is the same
file whatever identity its filesystem reports: device numbers are left out of
kept identities (`storage.StableObjectIdentity`), a root whose device number
alone changed is the same root (`storage.SameRoot`), and a changed inode with
unchanged size and time keeps the file's catalogue record and analysis.

Scheduling is optional, persisted per source, has a 15-minute minimum interval
and applies jitter. It coalesces paused/in-flight jobs. Source edits clear
completion evidence and require fresh inventory for cleanup.

Owner actions on a file are `associate` (this file is another version of that
title) and `separate` (this file is not a version of the title it was grouped
with). Both are remembered across scans. Source removal disables its head,
cancels work and marks the files under it missing in the background; their
titles then follow the rule for missing titles. A surviving available version
keeps the logical item available.

## Analysis and resource ownership

File List Only performs directory/stat work and provisional catalog publication;
it does not read media bytes, metadata sidecars, artwork, subtitles or STRM
contents. Basic selects bounded probe, local metadata and subtitle stages.
Complete preserves those stages and requests declared deep-operation names.
Custom validates explicit operations and dependencies; it never silently grants
additional media-read permission. Policy edits have a durable pending handoff,
preempt old work and queue missing analysis on current revisions without
requiring another full inventory.

No new checksum, trickplay, waveform, loudness, fingerprint or segment-detection
algorithms are implemented here. Uninstalled requested stages remain explicit
`analysis_stages_not_installed` warnings, never fabricated successful analysis.
Existing completed basic operations can be reused under a stronger policy.

Analysis checks policy before each admitted read and again before publishing,
defers/preempts for source playback activity, bounds probe/output/sidecar work,
and uses the existing storage supervisor. Production scan probes and embedded
artwork use a retained root-contained file descriptor in the supervised helper;
on Linux/macOS it execs the media tool in the same process so cancellation and
physical Wait ownership do not create an unmanaged child. Root-contained small
reads use the same context. File/pipe protocol and format restrictions reject
indirect playlist/descriptor expansion. Other operating systems report the
actual unsupported retained-descriptor command capability. These guards do not
change existing provider transport or relax playback version checks.

Ordinary extracted media files such as M2TS/MTS/VOB/MPG/MPEG use the normal file
path. ISO/IMG remain visible unsupported inventory outcomes. There is no disc
decryption, menu navigation or archive expansion.

## Changes and consumer projection

`inventory_changes.sequence` is a monotonically increasing database cursor, not
a source-local count. Read `GET /v1/admin/libraries/{id}/sources/{source}/changes`
with `after`; pages contain at most 128 records and return `nextSequence` and
`hasMore`. Sequences can have gaps for another source. Consumers persist their
own cursor, tolerate replay, and process committed records idempotently by
sequence. Per-job/object/revision/kind uniqueness also prevents page replay from
duplicating events. There is no automatic event-log retention/purge in P04A.

Kinds include `created`, `revision_changed`, `reappeared`, `relocated`,
`replaced`, `missing`, `analysis_completed`, `inventory_completed`,
`root_identity_confirmation_required`, `version_associated` and
`version_separated`. A changed revision takes precedence over a separate
relocation/reappearance label; consumers must read the current location state,
not infer a complete snapshot from one label. Owner-generated changes use an
`owner:` operation token rather than a scan job ID. Completion records have
empty object/asset/revision fields. Events carry stable IDs, not secret locators.
There is no invented global transaction snapshot across different sources.

Owner inventory is a live 40-row keyset view, not a frozen running-scan snapshot.
Continuation tokens bind library, source, filter, viewer, limit and configuration
revision. Commands use configuration/revision preconditions. Lost command
responses require explicit read reconciliation, never automatic mutation replay.
Owner scope changes clear cached locators and abort pending reads.

Viewer `GET /v1/libraries/{id}/inventory-status` rechecks library authorization and
exposes only source IDs, health, job/phase, counts, pause reason and completion
status. Web/iOS/tvOS consume this path-free projection. Source configuration is
owner web UI only. Existing catalog readers use exact-library/item/asset
availability views; playback resolves an available admitted location and its
exact source root. Shows, albums and books remain navigable when an unavailable
child has no play action. No parallel player or credential system is introduced.
