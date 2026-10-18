# Outcome

The scan loop now reads source-relative folder, filename and bounded embedded-video evidence through the shared naming package. Matching receives typed provenance and provider hints; film/show attachment receives parsed identities; versions retain editions and split-part numbers. Explicit filename seasons win folder/tag coordinates. Episode zero and stacked episode parts attach. Extras have a specific title owner, including loose-film trailers; disc folders are one unavailable logical title rather than a title per stream.

Local scans enumerate bounded pages instead of trusting folder timestamps, so in-place media changes are observed. Bounded `.plexignore` and `.ignore` configuration is read inside the retained root. Managed adapters read the exact configuration names from completed staged listings, inherit ancestor rules and preserve their unfiltered atomic commit pages. Changing/unreadable folders and unsafe/dangling links get owner diagnostics and protected absence coverage while readable files continue through analysis. Failed probes leave name-based titles in place. Changed paths schedule another limited pass. Source-scoped warnings no longer blame other folders.

A supervised native watcher feeds the same `ReportPaths` limited-scan path: Linux inotify, Windows ReadDirectoryChangesW, macOS FSEvents; non-cgo macOS builds use kqueue. Events debounce and coalesce. Linux sources sharing a watched inode each receive its events; removed sources leave the monitor’s fallback map. Periodic fallback covers network storage, watcher failures and watch limits. Per-library monitoring defaults on. Reported removal of the last file in a readable unchanged root is immediate.

NFO/artwork/subtitle additions, edits and deletions invalidate only affected files, including parent metadata for child seasons/discs. Artwork positive caches are invalidated; complete observations release only automatic images backed by removed/replaced sidecars. Owner selections and retained image bytes survive. Normal scans check evidence in fixed candidate pages and create analysis rows only for missing revision/policy/operation/head evidence, including policy-only changes. Unchanged sources cannot make one enqueue quantum an unbounded writer read. Shared gentle/normal/fast profiles set page sizes and source concurrency; playback adds a bounded delay rather than canceling analysis. Existing helper CPU/I/O priority and foreground writer pacing remain.

Terminal checkpoints and current directory caches are pruned in bounded chunks. Managed listing entries/cursors/generations also retire, leaving one newest ready snapshot per current directory for inherited ignore configuration; obsolete generations do not accumulate. Twenty scan summaries per source remain; change history retains up to 10,000 entries or 30 days, with explicit expired-cursor signaling.

Snapshots 001 and 002 recorded the compiled wiring/tolerance and watcher/retention milestones. Final cumulative snapshot 003 records the complete lane: “Keep scans tolerant, watch changed folders and queue only missing analysis” (351,291 tracked diff bytes and 39 new files). The worktree has not been changed after snapshot 003. No commit, push or deployment.

# Decisions I made

- Ordinary local scans always list file facts; an unchanged directory timestamp cannot prove unchanged media. Managed adapter listings keep their existing supervised protocol; configuration is found in complete staging snapshots, without issuing speculative missing-file requests. Per-job ignore caches hold at most 128 scopes/512 KiB and retire on completion.
- File List Only still avoids media/NFO/art/subtitle reads, but reads bounded ignore configuration. Ignore rules are scan configuration, not media analysis. Ignore content is capped per file and cumulatively; excessive diagnostics are summarized without dropping good rules.
- A changing folder protects only its subtree and schedules a later limited pass. An unreadable folder keeps old evidence and waits for a future notification/periodic scan; it is not retried in a tight loop.
- Built-in exclusions and user rules intentionally reconcile prior entries as missing. Symlink escape protection remains intact; a skipped unsafe link does not prove absence.
- Meaningful file evidence excludes contradictory folder titles/IDs from matching and attachment. Embedded movie/show tags are file evidence; an episode display title is not a show name. Provider details still validate match candidates.
- Named editions are version labels on one title. Disc folders are unsupported playable formats represented once; no guessed main stream or disc playback was added.
- Extras attach only when a folder or filename identifies one owner. Ambiguous extras remain extras with no owner rather than appearing on every film in the folder.
- Automatic monitoring uses two seconds of quiet, at most ten seconds of continuous events; one helper, at most 64 roots/4,096 directory watches and the existing 64 KiB helper-request bound. Network sources/unavailable watchers fall back every five minutes; monitored local sources have a fifteen-minute safety inventory. Existing explicit source intervals continue to work with monitoring off.
- Empty-root protection remains for ordinary scans and identity rebound. An explicit paths-changed/watcher run may mark the last file missing when the admitted root remained the same. Unreachable/changed roots still stop absence publication.
- Keep the newest twenty terminal scans per source, prune heavy terminal work after fifteen minutes, and keep 10,000 changes or thirty days. Active and paused work is never pruned. An expired feed cursor tells consumers to reread inventory.
- Linux native watcher tests passed through the real supervised helper. Windows/macOS cross-compilation is code evidence, not native filesystem acceptance. The macOS framework-backed C bridge still needs its platform build.
- Host: ehler-homelab-mini-1. Context-sync could not write its lock in this sandbox; readable canonical project/verification guidance was used directly.

# Bugs

- B-05.1 — already fixed urgent continuation retained; shared speed now controls scanning. Production-loop cadence regressions pass.
- B-05.2 — fixed: changed coverage is protected, diagnosed and retried; analysis continues.
- B-05.3 — fixed in scanning: local file facts are reobserved, including in-place writes; changed-during-analysis paths are queued again. Playback’s strict source pin is retained.
- B-05.4 — fixed: unreadable subtree isolated, diagnosed, kept and tested under a non-root user.
- B-05.5 — fixed: dangling/outside links skipped and diagnosed; containment stays enforced.
- B-05.6 — naming exclusions retained and now reconcile old inventory safely; user ignore rules integrated.
- B-05.7 — naming fix completed into matching/attachment; no separate release-year grammar remains.
- B-05.8 — identity lane already fixed title/version model; new filename/tag provider/edition/part evidence now reaches it.
- B-05.9 — identity rename fix retained and fed shared evidence; manual version/episode placement remains pinned.
- B-05.10 — naming fix retained; flat and translated-season evidence reaches show attachment.
- B-05.11 — naming file precedence retained into matching and attachment; conflicting folder hints do not override it.
- B-05.12 — already fixed; naming resolution/codec bounds remain.
- B-05.13 — naming corrections retained; episode zero and genuine stacked episode parts now attach. Eight-link/numeric bounds remain.
- B-05.14 — naming extras classification retained; specific ownership replaces broad folder sharing.
- B-05.15 — fixed as one unsupported disc-folder entry; no disc playback claimed.
- B-05.16 — naming sample/trailer handling retained and integrated with ignores/ownership.
- B-05.17 — shared display cleanup retained; matching no longer reparses cleaned titles with another grammar.
- B-05.18 — identity lane’s device-renumber handling retained. Representative NFS/SMB/FUSE remount acceptance remains unrun.
- B-05.19 — directory timestamps no longer authorize local skipping; network mounts are enumerated too.
- B-05.20 — fixed: bounded terminal work/history/change-feed/cache pruning and durable integrity summaries.
- B-05.21 — fixed: selective queue rows; policy/operation/algorithm/head/STRM evidence invalidation retained.
- B-05.22 — base negative-listing optimization retained, extended to JPEG/WebP names, and sidecar edits invalidate it.
- B-05.23 — fixed: default monitoring, startup scan and periodic fallback; owner may disable monitoring.
- B-05.24 — identity lane’s immediate missing model retained; limited reports include last-file removal without two observations/grace.
- B-05.25 — already fixed by identity; deletion still removes disk files and no trash copy is restored.
- B-05.26 — fixed: persistent naming attention scoped through the affected source’s current inventory.
- B-05.27 — identity model retained; source-relative full show-folder keys and provider/name evidence wired.
- B-05.28 — existing Windows helper/identity work retained; watcher/helper additions cross-compile. Native Windows acceptance remains required.
- B-05.29 — predecessor local-metadata tolerance retained; scanner now notices sidecar changes/removal while preserving failed observations.
- B-05.30 — base stopped-folder notice retained; typed paged owner diagnostics now expose all skips and invalid ignore-line numbers.
- B-05.31 — music grouping fix retained; canonical DiscFolder now includes its volume/part/act/digital-media forms and is used by audio attachment.
- B-05.32 — NFC naming/identity behavior retained; source-relative folder identity avoids unrelated category evidence.
- B-05.33 — fixed: directory aliases are detected before visited-identity updates and protect their old subtree; two-scan regression passes.
- B-06.1 — shared reader wired into matching; numeric-title behavior verified.
- B-06.2 — already fixed by providers-matching; typed evidence now supplies stronger yearless queries.
- B-06.3 — predecessor provider/owner artwork guards retained; sidecar replacement releases only automatic selection.
- B-06.4 — already fixed by providers-matching/music; existing quota/per-image bounds retained.
- B-06.5 — predecessor season artwork routing retained; changed/removed sidecars are reobserved.
- B-06.6 — already fixed by providers-matching.
- B-06.7 — already implemented by providers-matching; no new provider adapter here.
- B-06.8 — already fixed by providers-matching.
- B-06.9 — already fixed by providers-matching; no locale-policy redesign here.
- B-06.10 — already fixed by providers-matching.
- B-06.11 — provider rates/concurrency already fixed; scanner pacing/selective work completed. No live first-import timing claim.
- B-06.12 — already fixed provider recovery retained.
- B-06.13 — already fixed owner-search priority retained.
- B-06.14 — already fixed provider region default retained.
- B-06.15 — predecessor NFO ownership retirement now receives changed/deleted observations from scanning.
- B-06.16 — predecessor tolerant NFO parser retained; no safety bounds removed.
- B-06.17 — already fixed optional provider-field isolation retained.
- B-06.18 — already fixed diversity ordering retained.
- B-06.19 — already fixed artwork language ordering retained.
- B-06.20 — predecessor retained-source reuse preserved.
- B-06.21 — predecessor absolute-coordinate provider support retained.
- B-06.22 — predecessor adult-search setting retained.
- B-06.23 — predecessor worker/history cost fixes retained.
- B-06.24 — predecessor episode freshness retained; sidecar reobservation does not change provider freshness rules.
- B-07.15 — scan bookkeeping portion fixed; catalogue dictionary/derived retention remains with freshness/identity lanes.

# API changes

All modified/new routes are typed registry routes requiring library scope and owner role; generated contracts are regenerated with apigen.

- GET /v1/admin/libraries/{id}/scans/{job}/issues — new; after/limit (max 128), jobId, items (sequence, relative path, reason, line, messageKey, warning), nextSequence, hasMore. Paths are owner-only.
- GET/PUT /v1/admin/libraries/{id}/settings — migrated from legacy handlers; bare LibraryDocument response replaces legacy envelope. settings gains monitorChanges boolean, default true. Existing client-core envelope parser already supports bare typed documents; settings parser/type updated.
- GET /v1/admin/libraries/{id}/sources/{source}/changes — migrated; previous items/nextSequence/hasMore retained, oldestSequence/resetRequired added for pruned cursors.
- Existing POST /v1/libraries/paths-changed shape remains; watcher and removals reuse ReportPaths.
- Removed duplicate legacy handlers for the migrated routes and stale scope-map entries for the identity lane’s already removed raw delete/trash routes.
- New client-core scan-issues parsers/read services exported; i18n supplies plain reasons and monitoring labels.
- Internal helper protocol adds native watch requests/events and closed inventory error codes distinguishing absence from permission/change failures. No public storage route added.

# Schema changes

Baseline only, synchronized with schema-sync; development databases must be recreated as the programme requires.

- library_sources: last_integrity_at, changes_pruned_sequence.
- inventory_runs: reported, root_rebound.
- inventory_objects: analysis_algorithm, analysis_generation.
- New inventory_scan_issues with path/reason/line, warning and absence protection, indexed by job and protected path.
- New inventory_sidecars and inventory_sidecar_changes with bounded replay cursors.
- New inventory_video_names and inventory_video_ids: normalized, revision-bound embedded-video title/coordinate/edition/ID evidence; local-evidence wake hooks use existing metadata work authority.
- item_extras: nullable owner_id and owner index.
- New inventory_artwork_sources tracks automatic sidecar carriers per asset/role. local_artwork admits episode stills already written by its caller.
- inventory_directory_cache: seen_job for safe removal/exclusion reconciliation. inventory_runs_retention and remote listing directory-history indexes. No derivation workers, viewer revision counters, search/sort or personal-state algorithms changed.

# Owner or viewer settings added or wired

- monitorChanges: per-library boolean, true by default, persisted in the existing administration document. Controls automatic watching/startup/fallback scanning; manual/paths-changed and configured schedules continue. No new screen in this server lane.
- backgroundSpeed: existing shared gentle/normal/fast maintenance setting, default normal. Page sizes 32/64/128; source concurrency 2/4/8; existing foreground pacing adapts; same-source playback adds a bounded 250 ms analysis delay. Screen remains with client lanes.
- Existing scan tiers, operations, providers/local-only policy, source intervals/symlink setting, missing-title rule and helper background-priority setting remain effective.

# Needed from others

- API/identity lane: fix the existing hostile-input test’s bootstrap fixture (setup returns 401 before route exercises); no setup/authentication code was changed here.
- Console/client lanes: add the monitoring switch and paged scan-diagnostics view using the new client-core service; render messageKey and line/path. Keep these paths out of viewer scan status and persistent client caches. Add feed-reset behavior for external/owner integrations using change history.
- Providers/client lanes: display embedded/file/folder evidence source strings where match evidence is shown. Existing metadata attention/services remain usable.
- Platform packaging: build the framework-backed Darwin FSEvents C bridge on macOS with cgo; qualify Windows/macOS native watches and network shares with the procedure below. Cross-compiled non-cgo Darwin uses kqueue plus fallback.
- Documents lane: update libraries-and-scanning contract/architecture for tolerant coverage, immediate reported absence, default monitoring/fallback, ignore configuration reads in File List Only, selective queueing, bounded history and unsupported grouped disc folders. Earlier identity/naming/provider reports’ scan-loop wiring gaps are now covered.
- Freshness lane owns viewer revision/event/rebuild behavior; no redesign performed here.

# Test notes for other environments

Verified: Linux go build ./..., schema-sync/persistence checks, focused new ingestion/naming/embedded/metadata regressions, production cadence tests, Linux watcher/helper tests, bounded retention (including managed listings and cross-generation retirement), managed root/nested ignores, expired-cursor tests, typed route/scope/OpenAPI checks, client-core type-check and two individual client test files (21 tests). Final Linux build, Windows amd64 and Darwin arm64 non-cgo cross-compiles pass. Focused vet passes for all touched Go packages. The selective-enqueue regression crosses an unchanged 32-object page before reaching the changed tail object. Client-core and web TypeScript checks pass. No full suite/build programme, deployment, tuner/GPU or native client build.

One additional existing HTTP test was accidentally included by a broad `TestEveryRoute` filter: `TestEveryRouteSurvivesHostileInput` fails its setup fixture with POST /v1/setup returning 401, before exercising scan routes. It also fails in isolation. No authentication/setup changes were made in this lane; repair that fixture in the API/identity lane. The exact typed scan/settings, credential-scope and OpenAPI coverage checks pass.

Cold acceptance:
1. Fresh baseline database; local movie/TV/music libraries. Import generic movie filenames with folder year/IDs, contrasting meaningful filenames, editions/parts, translated/flat seasons, episode zero, extras and BDMV/VIDEO_TS. Pass: one title with versions/parts, filename coordinates win, extras attach to one owner, disc streams are not posters.
2. Watch a title; rename/replace its file and edit embedded title/IDs. Pass: shared evidence reaches matching/attachment, title state survives recognized replacements, manually placed versions/episodes and owner image choices stay pinned.
3. While a scan runs, add/remove a subfolder, edit a file in place, deny permissions to another subtree, add a dangling/outside symlink. Pass: readable files finish, owner issues identify skips, protected files stay available, valid removals in stable folders become missing, changed paths get another limited pass.
4. With a native watcher running, add a file, overwrite it in place, change/add/delete NFO, artwork and subtitle files, then delete the last media file in a readable root. Pass: affected folders scan within the debounce window, unrelated folders are not listed, sidecar state refreshes, automatic stale artwork is released, owner picks stay, removal is immediate. Disable monitoring: no automatic watch/fallback scans; explicit scans/reports still work.
5. Windows: local NTFS and SMB, rename/delete entire watched folders, overflow a watcher burst, pause/resume/cancel/restart scans. macOS: build with cgo/frameworks and exercise FSEvents on APFS plus SMB; also test a non-cgo/kqueue build. Pass: bounded handles/processes, cancellation shuts down helper, missed native coverage is recovered by fallback, unreadable/unmounted roots never authorize absence.
6. Linux network/FUSE/union mount: remount/device-renumber and in-place writes without directory timestamp changes. Pass: no phantom replacement from the preserved identity rules, local enumeration notices file facts, offline source keeps all titles.
7. Large source: start playback and browse/owner-console during import; switch gentle/normal/fast. Pass: progress continues while playing, helper/process/read bounds remain, browsing stays reachable, unchanged scans add no completed-object queue rows. Measure listing/matching time on actual storage/providers; no throughput benchmark claimed here.
8. Repeated scans: after retention age, run PruneSource repeatedly or let the worker run. Pass: each transaction removes a bounded chunk, active/paused runs survive, twenty summaries remain, directory/queue/remote-staging history shrinks in bounded chunks while current inherited-ignore snapshots remain, change feed caps and signals resetRequired for an old nonzero cursor.

# Not finished

The lane implementation is complete. Client controls/screens and physical Windows/macOS/network-mount/provider performance acceptance are programme handoffs, not delivered UI/runtime evidence from this lane. The macOS cgo/FSEvents bridge has source implementation but has not been compiled against Apple frameworks here. Disc playback is intentionally not implemented; grouped disc titles have a truthful unsupported status. No first-import time is claimed. The unrelated hostile-input HTTP setup-fixture failure is recorded above and left to its owner.

# Merge with the base

Reopened the worktree for Justin's requested base merge; no staging, commit or snapshot.

- `settings.ts`: kept both additive string sets: scan monitoring/diagnostics and the base's integrations, metadata, connection and library controls. Removed settings stay removed.
- `attach.go`: kept the shared source-relative naming reader and slash-normalized full folder identity, preserving the base's distinct nested show folders, minor-spelling tolerance and retained-state restoration after audio/provider attachment.
- `catalog.go`: kept `movieFileReadingTx`, which already supplies the base's provider hints while also supplying revision-bound embedded evidence, source-relative file precedence, editions and parts. The base's title/file attachment machinery stays in use.
- All four scan-loop registry routes explicitly require `ScopeLibrary` and owner access. Generated contracts/schema files are regenerated by the tools, not merged by hand.

Verified: no conflict markers in the worktree (including hidden/ignored project files; Git metadata and dependency directories excluded); apigen and schema-sync/persistence checks; Linux, Windows amd64 and macOS arm64 non-cgo builds; focused catalogue identity/naming/audio/provider tests and scan integration tests; catalogue/ingestion/httpapi vet; web TypeScript; 35 client-core administration, scan-issues and catalogue tests. All requested checks passed. No staging, commit or snapshot was performed.

Additional check: `TestOpenAPICoversRegisteredRoutes` fails because the incoming base adds `GET /receiver/cast/` to its route lanes without an OpenAPI path for that prefix. It is described in `server/api/social.md`, but not in the OpenAPI source read by the guard. The merge does not change cast code or documentation; this separate base documentation gap remains. Typed scan settings and mandatory credential-scope checks pass.
