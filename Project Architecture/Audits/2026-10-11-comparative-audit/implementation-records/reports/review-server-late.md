# Outcome

Completed the independent review on base ce34473, personal Ubuntu mini-1. Found four medium-severity defects and fixed all four; no blocker, high or low findings. Playlist download requests retain their own identity across unrelated title merges; retry preserves the requested download version; physically deleted files remain unavailable after a later conflict or authority refusal; restricted DVR channel pickers can reach every permitted channel past excluded prefixes. Required context-sync failed because its state/lock directory is read-only; existing canonical guidance was read directly.

# Decisions I made

- Preserve the programme's intentional HTTP, password, TV approval and owner-choice policies.
- Review the combined code rather than treating authors' reports as proof. Already-resolved identity review findings are controls, not new findings.
- Separate physical-file absence from the later authorized title mutation. A factual reconciliation may run for up to five seconds after cancellation/refusal, but grants no authority to remove a catalogue link or any additional file.
- Version-specific retry may reuse only a same-source successor claim; another version is not a substitute.
- DVR picker order is the source's indexed position. A page examines at most 256 candidates, returns at most 30 permitted channels and may be empty with continuation. Its cursor carries no excluded channel identity.
- Updated AGENTS guidance revision 903b1fbea30aec5db763f43747827250650b340b was received during finalization; relevant standards/verification guidance was refreshed. No scope or publishing authorization changed.

# Bugs

The audit register, fix log and the assigned leftovers' bug references were reconciled with source. “Already fixed” below describes inherited source behavior; only the named tests in Test notes were executed here. Native, external-document and historical incident limitations are explicit, rather than inferred fixed from another report.

- B-01.14 — Already fixed: onboarding and playback use the shared owner-defined network classifier.
- B-01.21 — Already fixed: secondary profiles cannot manage account device/remembered-account records; selected regression passes.
- B-01.22 — Already fixed in release admission: local families/membership checks replace retired Hosted-family/viewer-only admission.
- B-01.23 — Not fixed here: historical Apple explicit-signout incident has no correlated runtime evidence; native adoption/incident reproduction remains external.
- B-02.8 — Not a current defect: sustained-rewatch completion clearing remains the deliberate saved-state behavior.
- B-02.16 — Already fixed server diagnostics: bounded owner read/clear route-failure memory; owner/native screen acceptance remains separate.
- B-02.22 — Already fixed: production startup does not read PORTICO_FINITE_VOD.
- B-04.10 — Already fixed: canonical ASS text salvage isolates bad optional evidence rather than rejecting usable dialogue.
- B-04.18 — Already fixed: explicit off wins; single untagged forced/default fallback exists in subtitle preference selection.
- B-04.23 — Already fixed: CleanChapters isolates invalid bounded entries.
- B-04.28 — Already fixed for optional visual analysis: bounded 5% decode-error tolerance; exact audio paths remain strict.
- B-04.35 — Already fixed default Complete policy: checksum and diagnostic fingerprint passes are explicit Custom operations; shared-decode optimization is not claimed.
- B-04.38 — Already fixed command/fact wiring: colour/tone-map and SAR correction reach analysis recipes; image/GPU runtime quality unverified.
- B-04.39 — Already fixed in shared web player: sub-20-ms offset changes are ignored; browser flicker acceptance unverified.
- B-05.5 — Already fixed: inventory preserves bounded warnings/protection for unreadable or outside-root links.
- B-05.29 — Already fixed: local XML charset salvage retains byte/node/depth/DOCTYPE protections.
- B-05.31 — Already fixed: disc-folder interpretation and semantic album grouping are wired.
- B-06.20 — Already fixed: bounded TMDB portraits and retained artwork acquisition reuse; no fresh-provider runtime claim.
- B-06.21 — Already fixed: explicit TMDB absolute/group/offset mapping; selected end-to-end publication test passes, preserves local numbers and validates provider coordinates.
- B-06.22 — Already fixed: includeAdult defaults false and owner override remains wired.
- B-06.23 — Already mitigated in source: slower provider/artwork watchers and streamed history GC. Poster/auth query cost at target scale remains unmeasured.
- B-06.24 — Already fixed: locale/region/freshness-bound episode reuse and fresh-sibling fanout skip; three selected regression controls pass.
- B-07.13 — Already bounded: online backup retains a consistent snapshot but limits time/WAL-growth/disk pressure; multi-gigabyte load acceptance unverified.
- B-07.14 — Already fixed: only final 0001_baseline.sql is shipped, with no development backfills.
- B-07.16 — Already fixed: heldSince-based production watchdog diagnoses stuck holds.
- B-07.18 — No reproduced contention defect: bounded/checkpointed derived work exists; weak-host measurement remains needed.
- B-07.19 — Not a defect under programme decisions: keep SQLite safeguards and quiet backup-copy advice; no owner-program detection/blocking.
- B-08.2 — Already fixed: unclassified server failures map to logged internal errors rather than invalid_request.
- B-08.9 — No concrete failing response established: registry validation remains protective; invalid root request null is refused, row salvage is domain-specific.
- B-08.11 — Documentation narrowed in writable API guide; external architecture assertions still need correction.
- B-09.30 — Already bounded by the backup pressure/time controls; still pins one consistent snapshot by design.
- B-11.12 — Already fixed in source: Recover resumes a retained capture until the accepted end and honestly labels the recovered result partial. Tuner acceptance remains outstanding.
- B-11.21 — Already fixed: directory traversal holds measureMu, not capture's writeMu; only short counter/health sections take writeMu. Real slow-disk acceptance unverified.
- B-11.35 — Deliberate tuning choice: retain bounded timeshift defaults; not a new defect.
- B-12.8 — Already fixed: terminal completion has early-EOF protection, rather than marking any ended report watched.
- B-12.9 — Already fixed: credits mode falls back to the configured threshold when no approved credits marker exists.
- B-12.10 — Already fixed in source: effective viewer/library completion policy reaches the writer; native display acceptance remains external.
- B-12.22 — Already fixed: paused/private history policy fences counting/history publication.
- B-13.3 — Already fixed shared seek logic: pending-seek wait and measured lead compensation exist; physical native seek/drift acceptance not run here.
- B-13.18 — Not fixed in web adapter: together.tsx load still ignores desired playing argument. Exact later screen/adapter work is listed below; no visual file changed.
- B-13.22 — Already fixed source: notices fan out in bounded, fenced, resumable profile pages.
- B-14.14 — Not finished in native scope: privacy-safe cleanup of signed-out/inactive profile files and device-wide space controls need platform acceptance.
- B-14.20 — Not finished in native scope: Android process-dead transfer recovery requires a durable platform job; server claims are durable.
- B-14.21 — Already fixed server timestamp fencing; shared receiptUsable still accepts a supplied wall-clock Date. Persisted trusted/monotonic device clock remains native work.
- B-15.7 — Writable identity guide no longer claims bearer tokens prove a registered device key; external architecture text remains to correct.
- B-15.9 — Deliberate no-lockout policy; not a finding.
- B-15.10 — Already bounded: hashing admission has cancellable queueing outside the database writer.
- B-15.13 — Deliberate TV-approval role policy; not a finding.
- B-15.14 — Already fixed transport: remembered-account lookup uses POST/body. Descriptor lookup remains intentionally unauthenticated; entropy is not proved by shape.
- B-15.17 — Already fixed: bounded release-feed client uses pinned owner-approved origin and metadata/redirect protections.
- B-15.19 — Partly complete source: pinned workflow/container inputs and vulnerability jobs exist. Prior npm advisory report and installer signing remain acceptance work; no new advisory scan or credentials generated here.
- B-15.20 — Already wired: owner provider credentials hot-apply and read responses expose presence only.
- B-18.4 — Already fixed server grouping continuation beyond 200 values; strict filtered/restricted aggregation performance and client migration remain disclosed gaps.
- B-25.10 — Not finished in writable scope: physical native acceptance and Apple/Vega release CI belong to native repositories.
- B-25.12 — Not fixed here, as explicitly instructed: distribution-rights/licence decision is outside code review.
- B-25.14 — Already narrowed: SQL guard prepares literal production SQL, including private spool DDL; dynamic queries require their own controls. Changed literal SQL passes preparation here.
- B-25.15 — Already fixed source: hostile-input cases are individually applied with deadlines and an authenticated canary. That broad battery was not rerun in this lane.
- B-25.18 — Already fixed: single immutable development baseline; no migration backfill chain.
- B-25.20 — Already fixed in packaging source: running upgrades restart, stopped/disabled remain so, Debian policy is honored; actual package lifecycle unverified here.
- B-25.21 — Not finished in writable scope: native dependency revision locks/workflows belong to the sibling checkout. Roku is excluded by programme decision.

Identity review R1–R10 fixes are already present on this base. Root confinement/replacement-root tests pass here; the review's remaining best-playable/multipart/container/provider/client integration limits were not represented as resolved by this lane.


# API changes

No route added or removed and no request/response shape, key scope or client parser changed. GET /v1/dvr/channels now pages in source order; its opaque continuation encodes generation plus indexed source position rather than generation plus channel ID. Retired cursor contents are rejected under the pre-release rule. Apigen completed with no generated diff. Version deletion and download mutation behavior corrections use existing contracts.

# Schema changes

None. The final baseline remains unchanged.

# Owner or viewer settings added or wired

None.

# Needed from others

Confirmed inherited gaps, outside these four local corrections:

- **Grouping performance:** browse_groups.go calculates arbitrary filtered/restricted group count/recent keys over matching facet memberships and may sort that whole set in SQLite. Unfiltered keys are maintained/indexed. Query-specific or visibility-class maintained keys remain needed for the strict large-library promise.
- **Deep direct ranges:** browse_indexed_sorts.go walks bounded pages from the start for new keys without position blocks. Ordinary cursor pages seek directly. Add maintained position blocks before claiming bounded arbitrary jumps.
- **Client/envelope migration:** common browse scopes have the full vocabulary, but legacy global search/person/detail/list envelopes and first-party screens retain previous defaults. Clients must intersect surface/pivot capabilities and carry subgroup predicates, rather than offering unsupported options. Native/Vega package synchronization is integration work.
- **Paused Together load:** web/src/app/together.tsx:42 still ignores the desired playing argument and calls play(). Later client work must pass initial paused intent through engine preparation before the first audible frame; native adapters need the same acceptance test. No visual file changed here.
- **Native downloads:** finish/qualify account-specific cleanup, Android durable process-dead recovery and persisted trusted/monotonic receipt expiry. Shared receiptUsable still checks the supplied Date.
- **Identity integration:** the prior identity review's best-playable/multipart/container-provider and retained per-play/playlist-position limits are not resolved by these download-claim fixes. Full v1 session/queue continuity on title merges remains integration work.
- **Release/docs:** actual native/Apple/Vega CI and device acceptance, exact cross-repository revision locks, installer signing, prior reported npm advisory and external architecture wording remain with their owners. No vulnerability scanner, signing process, deployment or release was run here.
- **Picker consumers:** a fully excluded raw page can have zero channels and a nonempty nextCursor. Follow continuation until empty; do not infer exhaustion from a short/empty page. The server bound protects responsiveness without exposing excluded channel names or IDs.

# Test notes for other environments

Verification on Ubuntu mini-1, from this worktree's server/ with GOFLAGS=-p=2:

- 32 distinct top-level selected tests pass across administration, catalog, downloads, DVR, HTTP, metadata, ingestion, identity and persistence. Exact selections/results: `_program/scratch/review-server-late/final-fix-tests.log`, `review-controls.log`, `final-cursor-checks.log`. The final cursor test includes unrestricted/excluded-prefix/all-denied cases, malformed positions, generation mismatch, page bounds, duplicate detection and no excluded-ID cursor exposure.
- Original negative reproductions: `initial-regressions.log`, `initial-choices.log`; intermediate cursor-boundary failure: `choices-cursor-boundary.log`. All reproduce a concrete assertion before their fix. R3's second authorization callback deterministically injects a later title edit/refusal; this proves write-boundary ordering, not a timed concurrent-thread race.
- Existing controls pass for pinned versions and merged claims, missing flags, exact attention totals, role changes, cache revocation, MCP scope/role filtering through actual API calls, hidden remote-playback titles, preparing-session pause, restricted browse filters, seeded random paging, removed cursor anchors, personal sort index plans, secondary-profile device management, anime mapping and locale-bound episode reuse.
- Targeted go vet passes for administration, catalog, downloads and livechannels/dvr. Changed literal SQL passes TestProductionStatementsPrepareAgainstTheBaseline. Apigen finishes with no contract diff. git diff --check passes.
- Linux go build ./... passes. Windows amd64 and Darwin arm64 cross-builds pass for all four changed packages; DVR was cross-built again after its final cursor fix. These are compilation results, not OS filesystem/runtime acceptance.

Cold acceptance on disposable Windows/macOS/Linux servers:

1. Import a film/episode/track/book part with two real versions. Preview/delete one; pass means only the approved file goes, the other version and personal state remain. Delete the last version; pass means the title becomes missing and follows the owner rule. Replace parents/roots with links or a same-size/timestamp replacement root; pass means outside/replacement files remain untouched. Race a title rename or member demotion after physical unlink; pass means deleted bytes are unavailable even when later mutation is refused.
2. Submit original downloads for two versions. Make A unavailable while B stays live; retry A. Pass means the resumed claim remains A; a same-source successor may be reused. Merge a show whose entity ID equals a playlist ID while that playlist has a pending download request. Pass means playlist target/membership stay unchanged and show requests/claims follow the surviving title.
3. Import hundreds of channels. Restrict the profile so its allowed channels occur after more than 256 excluded entries. Follow every continuation, including an empty page; pass means every allowed channel appears once, no excluded title/ID appears, and pages contain at most 30 visible choices. A changed guide generation/malformed cursor must fail safely.
4. Qualify large filtered group queries/deep ranges, physical player/tuner recovery and installer lifecycle separately. No capacity, real-provider, codec, GPU, tuner or browser/native acceptance follows from these focused tests.

# Not finished

No newly reported finding remains open. Confirmed inherited completion gaps are listed under Needed from others below. Physical-device, OS-runtime, tuner, real media/provider and large-load acceptance were not run; this is source review with focused regressions, not a release acceptance claim.

# Findings (recorded before fixes)

## R1 — Playlist requests are retargeted by unrelated title merges — medium

`server/internal/catalog/download_identity.go:16`: `download_requests.target_id` is polymorphic, but the merge updates every numeric match without checking `target_kind`. A playlist's integer ID can equal the absorbed title ID; merging that title silently changes the playlist request's selection. Trace: `requestTargetID` resolves playlist IDs from `catalog_playlists`, all other IDs from `catalog_entities`. Smallest fix: exclude playlist targets from catalogue-entity retargeting. Reproduced before the fix; see the corresponding Fixes entry and initial log.

## R2 — Retrying one download version can return a different version — medium

`server/internal/downloads/preparations.go:385`: retry's sibling lookup matches profile/title/quality but omits `source_asset_id`, although admission now deduplicates by that field. An unavailable version A and active version B cause retry(A) to cancel A and return B. Smallest fix: include pinned source identity in sibling matching; verify same-source dedup still works. Reproduced before the fix; see the corresponding Fixes entry and initial log.

## R3 — A post-unlink title edit rolls back missing-file reconciliation — medium

`server/internal/administration/version_deletion.go:153`: after physical deletion, the write recomputes the entire preview key before marking deleted files. A concurrent title rename/link edit causes ErrConflict and rolls back the missing/unavailable update, even though disk deletion has happened. Smallest fix: reconcile the approved, unchanged physical asset independently before the later title/link mutation, preserving normal conflicts and authorization refusals. Reproduced before the fix; see the corresponding Fixes entry and initial log.

## R4 — Restricted DVR channel choices stop before later allowed channels — medium

`server/internal/livechannels/dvr/presentation.go:153`: SQL limits raw candidates to 31, then Go filters permissions; continuation is emitted only for more than 30 accepted channels. An excluded prefix can produce an empty terminal page while allowed channels remain. Smallest fix: bounded raw keyset pages with a cursor tracking examined candidates, never dropping or disclosing excluded channels. Reproduced before the fix; see the corresponding Fixes entry and initial log.

# Fixes

Four fixes completed below. Original failing logs were saved before production changes. Snapshots 001 and 002 capture the two small groups; a final cumulative snapshot follows all checks.

## R1 and R2 — fixed

Both failing tests reproduce on the original base (see `initial-regressions.log`): the playlist request target changed from 1 to 2; retry(A) returned B's claim ID. Catalogue retargeting now excludes playlist targets. Retry matches `source_asset_id` as admission does. Regression controls verify entity-target retargeting and same-source sibling reuse; existing requested-version/merge and failed-claim retry tests pass. Targeted catalog/downloads vet and Linux `go build ./...` pass. No schema or route changes.

## R3 — fixed

Original test fails with “physically deleted file is still available after conflict.” Deletion now commits a separate, five-second, factual reconciliation immediately after disk deletion and before the authorized link mutation. It only marks the captured token/path/size/timestamp unavailable; it grants no permission to unlink a changed title. Cancellation/role changes cannot cancel this factual update. Later conflicts and authorization refusals still return normally. The denied-authority control verifies the link remains while the deleted file is unavailable. Existing ordinary/last-version/idempotency tests pass. If the database itself cannot accept the reconciliation, the call still reports failure; durable storage failure is not recoverable by pretending deletion rolled back.

## R4 — fixed

Original test fails with “got 0 allowed choices, want 50” for a 350-channel denied prefix. The picker now reads at most 257 raw rows and returns at most 30 allowed choices. Its cursor advances by the last examined candidate, including denied candidates, with an extra row proving continuation. Tests cover unrestricted traversal, a shorter denied prefix, a prefix longer than one raw page, all denied channels, no duplicates and the visible-page bound. Names and IDs of excluded channels are never returned. The final continuation uses source position, as detailed in the boundary follow-up below. No client parser/route shape or schema change.

### R4 boundary follow-up before final snapshot

The first paging fix advanced with the last examined raw channel ID. Unlike the original implementation's allowed-only anchor, that can place an excluded channel ID in its readable cursor. This was caught in the independent boundary pass before delivery. Use the existing unique `(generation_id, position)` index and source ordinal as the keyset/cursor instead, retaining the generation fence and fixed candidate bound. This also gives the picker the source's channel order. No names/IDs of excluded channels need to travel in a cursor.

The cursor-boundary regression fails on the intermediate R4 implementation with “cursor reveals excluded channel 256” and passes after ordinal keyset paging. Negative/leading-zero/retired-format/overlong positions and changed generations are refused; the public cursor remains a bounded opaque string. `client-core/src/dvr.ts` treats it as opaque and needs no shape update. Apigen was run after this behavioral cursor change and generated no contract diff. The old channel-ID cursor is intentionally not preserved in this pre-release API.


## Final verification and snapshots

All four findings are fixed. No finding was dismissed. Severity counts: blocker 0, high 0, medium 4, low 0. The excluded-ID cursor follow-up belongs to R4 and was corrected before final delivery.

Cumulative snapshots: 001 download selection identity; 002 physical deletion reconciliation and bounded channel continuation; 003 hidden-identity-free channel cursors. **Final cumulative snapshot: `_program/snapshots/review-server-late/004`.** No source changes follow it. Snapshots 001/002 are intermediate TDD milestones; use the final tree for acceptance.

No route/response shape, schema, setting, UI appearance or client parser changed. No commit, push, deployment, full build outside the permitted Go build, or full test suite ran. New tests and local fixes are confined to the named worktree. Broader incomplete client/integration/performance and device/release acceptance items remain explicitly recorded above.
