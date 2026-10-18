## Outcome

The server now marks current show, season, album and book members watched or unwatched atomically, including partially played members. Both actions clear resume positions, fence earlier playback and return a thirty-second Undo receipt. Undo restores the prior completion mix and progress, survives a service restart, is scoped to the profile and container, and refuses later member edits, playback, overlapping marks, membership changes or expiry. History, ratings and saved flags are preserved.

Home rows declare an explicit destination. Show-page Next Up ignores Home dismissals and age limits; an explicitly unmarked anchor is offered again. Unfinished films have no age expiry. Missing files do not enter continuation; episode duration uses the episode's range, and unknown-duration titles can resume. A season-premiere exception requires a recently added first available episode of a later season. The merged row uses the largest participating library maximum so a small library does not shrink everyone else's row.

Meaningful rewatch progress retains the existing contract: the title becomes unwatched until it completes again. Counts increment once per occurrence. Unchanged paused heartbeats and brief replays do not advance personal recency or personal revisions. A valid completion from another accepted session can complete the title, subject to explicit fences. Paused-history session starts preserve old resume positions. Shared popularity excludes paused/opted-out plays and requires three eligible profiles.

Viewer History now applies complete current library, rating, label and recording access before paging. Effective container completion feeds recommendation signals, including season marks. Historical evidence can use the shared writer with its occurrence time; owner and viewer completion facts are sticky and countable once independently of clearing or pausing the viewer's History.

The old Home projection and `GET /v1/content?view=home` are removed; `/v1/home` is the Home read. Home, Home-row paging, viewer History and global Saved reads have typed registry contracts and explicit credential scopes. The current Home HTTP tests and the old-endpoint rejection test pass. Client screens and the downloads transport remain programme dependencies below.

## Decisions I made

- A mark supersedes all older member watched/unwatched intent, including deliberate older unwatched marks. A newer item action still wins. This follows the wording of Mark Show/Season Watched rather than preserving a contrary old member default.
- Undo is a separate action, not Mark Unwatched. Its token is single-use, with revision checks; a repeated consumed token is rejected without reapplying anything. The full thirty-second window is set at the end of the mark's transaction.
- Container actions are bounded at 5,000 members; larger shows should be handled by season. This changes the old constant-time default into bounded atomic work so progress, fences, snapshots and cached recommendation state agree immediately. No 5,000-member latency qualification was run.
- Older item intents and their exact timestamps are captured in the Undo journal. Marks write explicit member intent as well as the container watermark, so a member with unknown/bad added-at metadata is marked correctly. Undo restores the original intent or its absence. Changed added-at or parent bindings decline Undo rather than silently restoring a different effective default. The destructive background reset worker and its table are deleted.
- Rewatch behavior is unchanged: meaningful rewatch progress clears watched until completion. Completion counts remain actual plays, not manual marks.
- Credits-only completion is preserved. Accepted terminal evidence remains trusted; premature EOF is an engine qualification risk, not a reproduced defect.
- Known durations use the configured started percentage without the old thirty-second ceiling. Unknown durations start at thirty seconds. Existing audiobook one-second resume and near-end completion rules are preserved.
- Video uses the library percentage unless the viewer explicitly sets their played percentage. The shared preference help now explains this distinction from music; no new switch was added.
- Recommendation days use the server's local date. All devices still share one day boundary.
- Popular on this server requires three opted-in, currently eligible viewers. Owner history still records opted-out/paused plays; those plays do not increment publicly shared popularity counts.
- Home destinations are a tagged union: library browse, the full row, or a saved view. This is more explicit than the earlier web report's browse-only suggestion and can describe every composed row without guessing ids.
- One normalized personal completion ledger prevents a resumed-history completion being lost because the owner's history already recorded it, and prevents clearing History from duplicating the same occurrence's count. Viewing-activity reset keeps that deduplication ledger; profile erasure deletes it and title merging rekeys it.
- Execution host: ehler-homelab-mini-1, personal Ubuntu. Context refresh failed because its lock directory is read-only; available canonical project standards, project note and verification guidance were read directly. No helper or subagent was used.

## Bugs

- B-12.1 — fixed: container marks supersede earlier member intent, clear member/book progress and fence earlier playback.
- B-12.2 — fixed on server: show-page continuation ignores Home age/dismissal rules and keeps meaningful rewatch progress. Client pages must use the supplied nextUp rather than an early-page guess.
- B-12.3 — fixed in the shared historical writer's explicit/activity fences; not fixed on the old downloads upload path in this worktree. The downloads lane must use the shared path below.
- B-12.4 — fixed: an unwatched anchor itself is eligible for Next Up.
- B-12.5 — shared history/count writer implemented and tested with historical time; the downloads upload/journal connection is owned by downloads and not present here.
- B-12.6 — fixed: member writes queue existing recommendation work, and RecSignalFor reads effective watched state. Season mark/Undo signal regression passes.
- B-12.7 — shared writer classifies completion on server, uses fixed millisecond activity/history stamps and stamps watched intent with occurrence time. Old downloads direct writes still need removal by downloads.
- B-12.8 — not changed: unconfirmed early-EOF risk; accepted terminal evidence is contractual. Requires real engine qualification described below.
- B-12.9 — not a defect under V05: credits-only intentionally has no percentage fallback; preserved.
- B-12.10 — clarified in shared catalogue help: music percentage, library video policy until explicit profile override, separate audiobook completion. Existing policy preserved.
- B-12.11 — fixed: started percentage applies to long as well as short known-duration video.
- B-12.12 — fixed: unchanged paused reports and below-started replays do not move personal recency/anchor. History still receives accepted reports.
- B-12.13 — fixed: unknown-duration continuation and session resume no longer collapse to zero because of the missing duration; qualified terminal evidence can finish.
- B-12.14 — fixed: noncanonical completion may update watched/progress, while explicit fences and newer personal evidence remain authoritative.
- B-12.15 — fixed: premiere exemption needs a recent addition and recognizes the first available episode, not only episode number one.
- B-12.16 — fixed: local server date for recommendation rotation/fingerprints.
- B-12.17 — already fixed by web-app lane according to its report; server search-history preference enforcement already exists. Search text and screens were not changed here.
- B-12.18 — already fixed by identity lane, present in this worktree; title/file state continuity was not redesigned.
- B-12.19 — already fixed by identity lane: purged anchors move to remaining episodes. Undo additionally declines changed membership.
- B-12.20 — partly fixed: unchanged paused/brief/repeated completion observations no longer rewrite personal state or snapshots at new revisions. Meaningful active progress still updates visible resume and existing revisions. Counter/event splitting belongs to freshness-server; no counter/trigger rewrite was made.
- B-12.21 — fixed by choosing a three-profile minimum for public popularity.
- B-12.22 — fixed: shared counts exclude privacy opt-out and paused history; owner record remains complete, and private per-viewer counts remain distinct.
- B-12.23 — fixed: continuation bounds use linked episode start/end, not whole-file duration.
- B-12.24 — fixed: merged maximum uses the largest participating library cap rather than the smallest.
- B-12.25 — fixed: explicit resume upserts activity for a never-played title without fabricating History or last-played activity.
- B-12.26 — fixed: separate short-lived Undo restores the previous completion mix and positions; unwatched remains an explicit reset.
- B-18.28 — fixed on server and non-screen client-core: every Home row has a tagged destination; library Discover's recent section also has the existing ContentSeeAll browse destination.

## API changes

- `PUT /v1/containers/{kind}/{id}/personal-state`: same request. Watched/unwatched now supersede older member marks and clear progress; response adds optional `undoToken` and `undoUntil`. Saved-flag behavior is unchanged. A mark is bounded to 5,000 members.
- New `POST /v1/containers/{kind}/{id}/personal-state/undo`, `{expectedRevision,undoToken}` -> ContainerPersonal with restored default and a new revision. Requires playback scope and the same current content authorization as marking. Revision mismatch is HTTP 412; expired Undo is HTTP 409 `operation_expired`.
- `GET /v1/home` and `GET /v1/home/rows/{id}`: rows add optional `seeAll`. Every composed Home row sends it. Shapes:
  - `{type:"library",libraryId,pivot,query?,sort:[{field,direction}]}`;
  - `{type:"row",rowId}`;
  - `{type:"saved_view",resourceId}`.
- `GET /v1/personal-history`: wire shape unchanged; complete viewer restrictions now apply before paging. It has a typed registry response, read scope and the existing quiet scoped-response behavior.
- `GET /v1/content`: global Saved/playlist directory only. The old `view=home` branch, old server projection and client-core Home branch are deleted. Its Saved response is registered with read scope.
- Home and History registry migration retains the conditional/scoped HTTP handlers, not a duplicate route. `apigen` emits the new contracts; no version/compatibility shim is introduced.
- Client-core: HomeSeeAll/validateHomeSeeAll, receipt fields on ContainerState, undoContainerState; library content no longer accepts a Home route. Parser tests cover exact destinations and receipt transmission.
- Internal integration: `AcceptPlaybackProgressAt(tx, viewer, playbackId, itemPublicId, sequence, positionSeconds, durationSeconds, state, canonical, occurrenceTime)` uses the normal shared writer. `personalstate.WriteAt` preserves the occurrence time of watched evidence. Callers must authorize and sequence/replay-check evidence first.

## Schema changes

Edited `0001_baseline.sql` and ran `scripts/schema-sync.py`; no compatibility migration.

- Removed `container_personal_resets`.
- Added `container_personal_undo`: profile/container-scoped token, previous default/watermark/clear fence, mark revision/time, expiry, original member count. Unique live token per profile/container, expiry and container indexes.
- Added `container_personal_undo_items`: member's effective watched state, expected post-mark personal revision, progress/unit/session, activity time/state, book pointer position, original intent/time, the mark intent time and original added/parent bindings. Indexed by token/member and member; foreign-key cleanup handles removed titles/receipts.
- Added `personal_completed_plays(profile_id,playback_id,item_id)` and its item index. Independent deduplication for the viewer's completion count. It follows title merges and profile erasure.
- Generated known schema/columns/digest synchronized. Identity's retained title state/storage shape was not changed.

## Owner or viewer settings added or wired

No new setting.

- Existing `playback.startedThresholdPercent`, integer 1–25, default 5: now governs known-duration resume/Continue eligibility without the thirty-second ceiling. Existing screen/catalogue label remains.
- Existing `playback.playedThresholdPercent`, integer 75–100, default 95: music percentage and explicit viewer override of video percentage. Shared help now explains the library default distinction; audiobook completion remains its own listening rule.
- Existing `privacy.pauseWatchHistory`: still suppresses personal progress, watched, History and private counts; session start now preserves old resume, and resuming history can count completion once.
- Existing `privacy.showActivityToMembers`: suppresses publicly shared counts as well as recommendation activity eligibility. Owner history is unaffected.
- Existing library Continue weeks/premiere choices: apply to Home show offers, not unfinished films or the show detail page; premieres need a recent addition.
- Existing per-library maximum: the largest participating cap governs the merged row, bounded at 200.

## Needed from others

- **Web/native/Vega client lanes:** show Undo after a container mark using `undoToken`, `undoUntil`, `revision` and `undoContainerState`. Restore/re-read counts quietly; do not offer Mark Unwatched as an Undo. Use `action.undo`, `personal.undoExpired` and `personal.listChanged` catalogue strings. A used/changed token returns 412; expiry returns 409. No screen changes were made here.
- **Home clients:** switch on `seeAll.type`. A library target opens its exact library/pivot/query/sort; row target opens rowId; saved-view target opens resourceId. The web report's earlier ContentSeeAll-only parser/navigation suggestion must adapt to this tagged union. Remove all id-based guessing. Discover's recent section now has ContentSeeAll, so its old recent-id heuristic can also go.
- **Show clients:** use server nextUp/season intent; remove older first-page episode fallbacks when nextUp is absent for a finished show. Meaningful rewatch continuation remains server-owned.
- **Downloads lane:** route verified occurrence journal events through the shared timestamped writer and remove the old direct applyProgress writer/client-supplied watched decision. Keep upload idempotency, event/session identity, sequence, authorisation and title/receipt ownership checks. This branch does not alter `internal/downloads/progress.go`. Its old route still misses histories and explicit fences until your work merges. Your planned AcceptOfflinePlay wrapper can delegate to AcceptPlaybackProgressAt; the shared private body now takes a clock. The completion ledger must not be deleted by Clear History or reset of viewing activity.
- **Freshness-server:** writes still use the existing transaction/write path and personal/progress rows so publication hooks can coalesce them. Preserve WriteAt alongside Write if adding hooks. Active meaningful progress still updates personal revisions; this lane deliberately did not change revision counters or events.
- **Identity:** retain the added personal_completed_plays rekey in title merging and deletion in profile erasure. Removal of container reset cleanup now targets the Undo journal. There is no destructive reset adapter to register.
- **Engine lanes:** qualify early EOF versus valid terminal evidence across Native/mpv/web and normal/offline occurrences. Do not treat this server unit coverage as player acceptance.
- **Documents:** saved-state contract's protected-member defaults/constant-time container wording, container Undo, thresholds, old generic Home path and UTC recommendation day are stale. Update the related architecture/Home/product-contract documents outside this lane. V05's rewatch/credits decisions remain respected. No import/export/tracker integration was added.
- **Programme integration:** pre-existing metadata route-lane omissions and credential/network/MCP restriction-matrix omissions below are outside this lane; their owners must repair the classifications without weakening the tests.

## Test notes for other environments

Verified so far:

- Linux server `GOFLAGS=-p=2 go build ./...` and targeted vet for catalog, compactcatalog, personalstate, playback, httpapi and identity.
- Windows amd64 and Darwin arm64 cross-builds of catalog/playback/httpapi, rerun after the old-Home cleanup.
- Focused Go regressions for Undo, completion/progress restoration, later edits/profile scope/expiry, show continuation, paused recency, explicit resume, episode range, unknown duration/missing availability, rewatch/private/shared counts, historical timestamps and older offline evidence, container recommendation signals, privacy pause/clear deduplication, thresholds, native writer ordering and scoped HTTP receipt use. Latest targeted catalog/playback/httpapi run passed. Ingestion tests compile with updated Home consumers; no media ingestion test ran. Unknown added-at members are explicitly marked, later arrivals stay unwatched, original intent timestamps restore, and season reassignment declines Undo. Timeline numeric bounds remain enforced with unknown duration.
- Existing HTTP Home composition/paging/customization/hero tests, the old Home endpoint rejection, scoped Undo receipt use and every-route credential-scope check passed after old-Home cleanup. New Home/History read registrations preserve their existing handlers.
- Client-core type-check passed after contract generation and removal of the old Home branch. The personal-lane parser test file passes (2 tests), and the library-content test file passes (24 tests).
- Schema-sync's persistence checks passed. No full repository suite, native build, browser session or physical playback was run.

Broader selected checks exposed pre-existing failures:

- `TestDiscoverCarriesPicksForYou`: expects a Science Fiction row but receives recommended/person/short/recent rows. Identity's report already reproduced this on the untouched base. The targeted test still fails the same assertion here; not claimed fixed.
- `TestEveryRegisteredRouteHasALane`: missing metadata attention/settings/show refresh/screen patterns. Static `git show HEAD` confirms the missing metadata patterns are absent from the base lane map.
- GET restriction matrix: missing MCP, API/access-token, console-settings, networking/connection and title API classifications, plus an obsolete access/api-keys pattern. Write matrix: missing access-token, paths-changed and MCP classifications. Those omissions are in HEAD; this lane adds its Undo classification and the specific HTTP profile/visibility probe passes. The broad matrices are not claimed passing.

Cold acceptance:

1. Start a disposable fresh-baseline server on each supported OS. Create a show with completed, untouched and partial episodes, a special, a multi-episode file and two versions. Mark show/season both ways, Undo within thirty seconds, and read back every member's completion/progress. History, flags and ratings must remain. Repeat after restart inside the window; expiry, newer playback/edit, changed membership or overlapping container mark must decline without partial restoration.
2. Finish an episode and check Home's next episode. Dismiss the show, wait/force an old activity time, and open its page: correct Next Up must remain. Mark the anchor unwatched: offer that episode. Old season premieres must expire; newly added first available episodes of a later season can return. Missing copies must not appear; a replacement version must preserve title state.
3. Replay a finished film: meaningful progress appears in Continue, one completion count per occurrence, correct last activity. Pause for hours: no Home reordering. Test unknown duration, a boundary episode in a shared file and manual resume on an untouched title.
4. Upload verified offline occurrence evidence through the downloads integration, with original timestamp. Owner history, viewer history and counts must agree except intentional pause/clear controls. An older observation cannot undo a newer explicit item/container action. Pause at threshold, unpause at end, clear History and retry: completion count stays one. Opted-out plays remain owner-visible and absent from public popularity.
5. Change ratings/blocked labels/library access after plays. Home, suggestions, recommendations and viewer History must omit restricted titles before counting/paging. History may not show their ids, artwork, titles, parent captions or navigation. Owner-only historical snapshots are intentionally retained.
6. Across every engine/platform, play an intentionally truncated fixture with a longer catalogued duration. Valid end should finish; an error/early EOF must retain resume through the engine fact adapter. This boundary was not exercised here.
7. In every first-party client, verify exact See all destinations and container Undo; clients must not reconstruct a destination from row ids or a continuation from the first season page. Verify quiet refresh and the current screen staying mounted with freshness/client-lane work merged.

## Not finished

- Client screens: container Undo, native tagged See all navigation and removal of obsolete show-page fallbacks remain outside this lane. Web tagged See all navigation is now integrated during the base merge.
- Downloads transport/journal has landed from the base; its offline history, ordering, privacy and book-pointer integration tests now pass. Real offline uploads remain unverified.
- Premature EOF/engine terminal evidence, physical-device playback, real offline upload, a 5,000-member latency/load qualification, and multi-platform UI acceptance are unverified.
- The route-lane completeness and credential-scope checks now pass after the base merge. The broad restriction matrices and pre-existing Discover fixture were not rerun in this follow-up; their earlier failures remain recorded above. Counter/event splitting belongs to freshness-server.
- Implementation and focused regressions are complete in this lane. Snapshots 001 and 002 were taken at compiling milestones; final cumulative snapshot 003 follows final checks, with subject “Keep personal watch state and Home consistent”. The later base-merge follow-up below changes that tree at the owner's instruction without another snapshot. No commit, push or deployment was performed.

## Merge with the base

All seven source conflicts are resolved. No staging, commit or snapshot has been performed for this merge; Git will still list the paths as unmerged until the owner stages them.

- `container-state.ts`: kept the base's tolerant display flags/counts and unknown-field handling; retained the validated Undo token/expiry and Undo service.
- `home.ts`: kept tolerant row/entry parsing, optional text and cache metadata; retained the tagged library/row/saved-view destinations. Adapted the web See all helper and Home screen to follow those destinations rather than infer them from row ids.
- `library-content.ts`: kept base row isolation, neutral display fallbacks and deduplication, with exact library scope and paging bounds. Kept removal of the duplicate global Home endpoint.
- `library-content.test.ts`: kept current library tests and rejection of obsolete Home requests; dropped base tests of that removed endpoint. Home is tested through its composed document service.
- `personal_activity.go`: retained completion deduplication, privacy, chronology fences, evidence bounds and listening thresholds. Used the base's single package-level playback writer; simplified `offline_play.go` to validate the occurrence time and delegate to it, avoiding duplicate timestamp rewriting after `WriteAt` already authors the intent correctly.
- `policy_errors.go`: combined Undo/continuation errors with base Live-source input, confirmation, network and credential errors.
- `route_lanes.go`: followed the base's replacements/removals and new routes; retained Undo admission and classified saved content as Browsing to match its typed contract. Personal reads declare Read; writes and Undo declare Playback.

Regenerated contracts/OpenAPI with `GOFLAGS=-p=2 go run ./cmd/apigen` in `server/`; regenerated schema artifacts with `python3 scripts/schema-sync.py` (its persistence checks passed). Linux `GOFLAGS=-p=2 go build ./...` passed. Targeted vet of catalog/httpapi/personalstate/playback passed. Focused catalog, playback and httpapi tests passed, including all 20 lane regressions, Undo HTTP/restrictions, composed Home, API keys/MCP, route-lane completeness and declared credential scopes. The four client-core test files passed (48 tests); web See all tests passed (6 tests); web `npx tsc --noEmit` passed. Repository-wide grep found no conflict markers and `git diff --check` passed.

Full `GOFLAGS=-p=2 GOOS=windows GOARCH=amd64 go build ./...` and `GOFLAGS=-p=2 GOOS=darwin GOARCH=arm64 go build ./...` passed. Extra `npx tsc --noEmit -p packages/client-core/tsconfig.json` passed. Offline download integration found one obsolete fixture: 120 seconds of a one-hour film no longer reaches the corrected five-percent started threshold. Changed that older-play fixture to 240 seconds, preserving the history-versus-newer-resume ordering assertion. `GOFLAGS=-p=2 go test ./internal/downloads -run 'TestOffline|TestProgressBatchIsIdempotent' -count=1` now passes, including paused history, replay/order fencing and book-pointer tests. No merge-resolution work remains; staging and committing are left to the owner.
