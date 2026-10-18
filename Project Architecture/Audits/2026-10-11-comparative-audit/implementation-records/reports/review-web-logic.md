# review-web-logic

## 1. Outcome
Seven confirmed findings fixed: 3 high, 4 medium, 0 blocker, 0 low. Playback cannot be taken over by retired responses; image subtitle decoding has combined allocation/work limits; settings saves preserve concurrent edits; returning follower tabs catch up; remembered choices stay ordered and readable; music reads are bounded; malformed event polls back off. Starting worktree was clean at ce34473 on personal mini-1. No commits, deployments, pushes or full suites. Cumulative programme snapshots carry the changes.

## 2. Decisions I made
- Combined decoder limits are shared internal constants; tests may lower them but cannot raise them. Oversized parts are skipped without removing valid earlier cues.
- Runtime settings retry only after rereading and proving the edited fields still match their original values or requested result. A revision alone is insufficient.
- Library memory keeps the newest pending asynchronous snapshot and evicts oldest memories to stay within its own read limit.
- A shared-stream catch-up resync is local and does not manufacture a server event cursor.
- Follow the programme's review-first requirement: record confirmed findings before changing production code.
- Context sync could not acquire its lock because the context state directory is read-only. Read existing project, standards, and verification guidance; freshness of that guidance was not refreshed.

## 3. Bugs
Historical IDs from the reviewed authors’ source documents/register are listed for traceability. “Not fixed here” on an out-of-scope row means no new independent verification or change, not a claim that the old bug still exists. These are not additional confirmed findings and are excluded from severity totals.

| ID | Disposition |
|---|---|
| B-10.1 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.2 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.3 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.4 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.5 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.6 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.7 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.8 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.9 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.10 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.11 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.12 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.13 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.14 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.15 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.16 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.17 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.18 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.19 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.20 | Already fixed — resampler continuity test passes across multiple nonaligned joins. |
| B-10.21 | Already fixed in code — runtime decoder qualification remains; actual Firefox Vorbis acceptance unverified. |
| B-10.22 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.23 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.24 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.25 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.26 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.27 | Already fixed in code — unity when gain absent and peak-limited gain remain; no new normalization policy. |
| B-10.28 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.29 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.30 | Already fixed — repeated ID3/padding bounded MP3 synchronization regression passes. |
| B-10.31 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.32 | Already fixed in source — listening system metadata includes author/narrator/chapter; actual system UI unverified. |
| B-10.33 | Not fixed here — server/native/library-metadata or remaining conformance item outside this web-logic repair; see audio-engines/music-audiobooks reports. |
| B-10.34 | Already fixed — own-render failure returns to ordinary playback; wired route failure test passes. |
| B-16.1 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.2 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.3 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.4 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.5 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.6 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.7 | Fixed further here — follower catch-up and malformed-poll backoff; insecure visible tabs retain existing separate long polls. |
| B-16.8 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.9 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.10 | Already fixed on reviewed Home/content/console parsing paths — bounded envelopes/scopes stay exact, malformed rows are isolated. Event-poll envelope fixed here. |
| B-16.11 | Already fixed — Connection keeps the page mounted; freshness quiet-read tests pass. |
| B-16.12 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.13 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.14 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.15 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.16 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.17 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.18 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.19 | Already fixed — browse path has no permanently stale capability map; hub dependencies tested. |
| B-16.20 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.21 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.22 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.23 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.24 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-16.25 | Not fixed here — screen/layout/tooling/runtime-acceptance item outside this targeted logic review; see web-app report. |
| B-17.1 | Already fixed — quality/audio/version use session changes; core and wired web tests pass. |
| B-17.2 | Already fixed — wired quality, replay and route failure retain the queue (web tests). |
| B-17.3 | Already fixed — external pause becomes intent (wired web/media tests). |
| B-17.4 | Already fixed — receiving bytes/preparing hints keep loads alive (wired media tests); Range chunks now report progress too. |
| B-17.5 | Already fixed — media test verifies resume is the initial HLS start position. |
| B-17.6 | Already fixed by gating/policy in source; AirPlay and native Safari acceptance remain unverified. |
| B-17.7 | Already fixed for ordinary playback — wired test covers page-close DELETE/hidden flush; live channel expiry is still its existing owner policy. |
| B-17.8 | Already fixed — lease recovery vs owner stop covered by controller/web tests. |
| B-17.9 | Already fixed — modifier checks remain in shortcut helper (fix log); no change. |
| B-17.10 | Not fixed here — visual/player-comfort runtime acceptance outside this logic repair; see web-player/web-subtitles reports. |
| B-17.11 | Not fixed here — visual/player-comfort runtime acceptance outside this logic repair; see web-player/web-subtitles reports. |
| B-17.12 | Not fixed here — visual/player-comfort runtime acceptance outside this logic repair; see web-player/web-subtitles reports. |
| B-17.13 | Not fixed here — visual/player-comfort runtime acceptance outside this logic repair; see web-player/web-subtitles reports. |
| B-17.14 | Already fixed in source — buffer sizing test passes; actual high-bitrate pressure still needs browser acceptance. |
| B-17.15 | Fixed further here — deferred start/change/read/stop lifetime regressions; existing double-Next web test passes. |
| B-17.16 | Already fixed — real rate included in timeline, core reporter tests. |
| B-17.17 | Already fixed — indexed/bounded FLAC and unindexed lossy fallback pass targeted audio tests; Range handling improved here. |
| B-17.18 | Already fixed in code — audioSession/interruption handling remains; iPhone/iPad acceptance not performed. |
| B-17.19 | Not fixed here — speculative hardware-media-key activation needs actual browser/device evidence. |
| B-17.20 | Already fixed — subtitle hook retains content during retry; real outage playback not exercised. |
| B-17.21 | Not fixed here — visual/player-comfort runtime acceptance outside this logic repair; see web-player/web-subtitles reports. |
| B-17.22 | Not fixed here — visual/player-comfort runtime acceptance outside this logic repair; see web-player/web-subtitles reports. |
| B-17.23 | Already fixed — fixed level/quality behaviour covered by media test. |
| B-17.24 | Already fixed in source — offset tolerance media test passes; real demux offset behaviour unverified. |
| B-17.25 | Already fixed — tests drive the wired WebPlayback/controller/port, not a retired adapter. |
| B-17.26 | Not fixed here — visual/player-comfort runtime acceptance outside this logic repair; see web-player/web-subtitles reports. |
| B-18.1 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.2 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.3 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.4 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.5 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.6 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.7 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.8 | Not present as a proven defect — V09 refutes the historical Hosted-authority premise; no authorization change made. |
| B-18.9 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.10 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.11 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.12 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.13 | Not present in current delete flow — selection opens preview/typed confirmation; no new confirmation added. |
| B-18.14 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.15 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.16 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.17 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.18 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.19 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.20 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.21 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.22 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.23 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.24 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.25 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.26 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.27 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.28 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.29 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-18.30 | Not fixed here — screen feature, presentation or owner-server contract outside this targeted repair; see web-app/console reports. |
| B-22.1 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.2 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.3 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.4 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.5 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.6 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.7 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.8 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.9 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.10 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.11 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.12 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.13 | Already fixed — bounded SSE reader is used by eventStreamOpener; targeted feed tests pass. |
| B-22.14 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.15 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.16 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.17 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.18 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.19 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.20 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.21 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.22 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.23 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.24 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |
| B-22.25 | Already fixed — positive spread beyond directed reconnect floors remains; event retry tests run below. |
| B-22.26 | Not fixed here — identity/routing/native/Hosted boundary outside the reviewed event/parsing logic; see networking/web-app/native-sync reports. |

## 4. API changes
None. Routes and schema are unchanged.

## 5. Schema changes
None.

## 6. Owner or viewer settings added or wired
None.

## 7. Needed from others
- Refresh the native and Vega copies of `packages/client-core` from this canonical worktree during integration; this lane cannot write those worktrees.
- Browser/device acceptance below is still required. No visual redesign or new strings are needed.
- Architecture documents should describe bounded image decoding and comparison-based settings retries. They are outside this lane’s write scope.

## 8. Test notes for other environments
No browser or device acceptance has been performed. Ubuntu checks use the existing Node modules; no installation/build/deployment occurred. Type checks and targeted test commands/results are listed in the final verification notes below.

Cold browser/device script (private test server, current Chrome/Firefox/Safari; native refreshed packages where applicable):
1. Start a film, delay a session GET/PATCH in a test proxy, change titles twice, then release the old responses. The current title/queue must stay current. Close/reload: old sessions should end; Now Playing must lose the old entry immediately or after the bounded request failure/lease fallback. Repeat while paused and during lease recovery.
2. Play real ASS with embedded fonts, a cropped/forced PGS sample, DVD idx/sub, and a DVB recording. Seek quickly, pause, change rate, enter fullscreen and select another track. Cues must follow the new time; workers must be released. Feed a tiny DVB file containing 256 large region definitions: memory must remain bounded and valid retained content must still render. Verify burn fallback on unsupported content. Test wasm CSP and worker asset loading in a release artifact; unit tests do not establish real libass output.
3. Two secure tabs with the same viewer/server: leave A visible, hide B for more than 30 seconds until its feed stops; change watched state/metadata on A; reopen B. B must quietly refetch once and update within the feed/hub window without a blank screen. Repeat over plain HTTP (the existing per-tab long-poll fallback remains). Disconnect the server: content must remain and retries back off.
4. Two owner tabs: edit Name in A, change Connection limit/Home networks in B and save B, then save A. Both edits must survive. Have both edit the same field: A must retain its draft and show a real conflict. Drop the response after a safely rebased save; Retry must use the exact operation key/body and reconcile one receipt.
5. Save many library tabs/sorts/filters, switch profile, restart, and Forget choices. Latest choices must survive only in their original viewer/server scope; Forget must persist even when initial storage is slow.
6. Stream a long music/audiobook file and seek deep with and without a FLAC index. A valid Range response must start without downloading the prefix. In a test proxy ignore Range or send the wrong Content-Range: the own engine must fail cleanly into ordinary playback, with bounded memory. Trickle a block for longer than 30 seconds while delivering chunks: it remains alive. Pause/seek during prepared-track loading: cancellation must settle promptly. Check gapless joins and normalization on real speakers; unit samples are not listening acceptance.

## 9. Not finished
No confirmed finding is left unfixed. Real browser/libass/device acceptance is not performed. This is a targeted independent review of the named logic paths, not a renewed whole-product audit; historical items outside those paths are explicitly left to their owner lanes below.

## Findings
All seven findings have fixes and targeted regression evidence below.

## Fixes
All seven confirmed findings fixed; milestone notes and before/after evidence follow.

### 1. High — image subtitle limits do not bound combined decoded memory or work
Files: `web/src/player/subtitles/dvb.ts:194–251`, `pgs.ts:65–82`, `vobsub.ts:157–182`, `image-worker.ts:16–32`.
DVB permits 256 individually valid regions of 9,437,184 pixels (2.4 GB active pixels) from tiny region definitions. Every page packs each visible region afresh, before checking the cue limit, and retained cues have no combined byte limit. PGS can paint 64 individually valid pictures in one cue (over 2 GB of RGBA); concurrent asynchronous worker paints multiply it. VobSub repeated offsets can copy the same multi-pack unit per index row; command sequences can repeatedly scan the remaining unit. Existing hostile tests exercise individual dimensions, never combined budgets. Evidence: reasoned allocation/loop trace; safe reduced-budget tests will reproduce the amplification without allocating gigabytes. Smallest fix: combined state, retained-data, frame and work budgets checked before allocation; avoid overlapping worker paints; reject only excessive parts.

### 2. High — delayed controller operations can replace or end the new session
File: `packages/client-core/src/playback-v1/sessions.ts:397–421, 487–529`.
`sessionUpdated` adopts its GET result without checking that the controller still owns that session. PATCH 412 adopts `current` before the replacement check. Queued changes read whichever session is current when they run. `stop` flushes asynchronously and subsequently pauses/publishes ended even if a new title was attached. `dispose` does not invalidate pending starts. Superseded starts leave a created server session alive. Evidence: reasoned asynchronous traces; deferred-response regressions planned. Smallest fix: fence operations by lifecycle and session identity across every await, bind queued changes to their original session, retire stale created sessions, and invalidate on disposal.

### 3. Medium — a returning follower tab misses changes made while its feed was stopped
File: `packages/client-core/src/event-feed.ts:208–240`.
A follower joins an existing broadcast stream with its previous cursor, sends `who`, receives only `open`, and never replays or requests a resync. The leader's stream does not reconnect, so earlier changes never reach the reopened tab's hub; documents marked current stay stale indefinitely until another event. Existing tests start both tabs before all events. Smallest fix: include the leader cursor in open acknowledgements and resync a joining follower when its cursor differs, without advancing its resume cursor beyond events it received.

### 4. Medium — asynchronous library-memory writes complete out of order
File: `packages/client-core/src/library-memory.ts:168–177, 210–221`.
Every edit starts storage.write/remove independently. Native asynchronous storage can finish an older write last and restore an older sort/filter, or undo Forget. Forget before the initial read also permits saved choices to be resurrected. Evidence: deferred storage traces; regression planned. Smallest fix: ordered persistence, and tombstones for forget operations while initial storage loads.

### 5. Medium — the music Range reader buffers arbitrary response bodies
File: `web/src/bridge/audio/bytes.ts:91–103`.
`arrayBuffer()` reads an unlimited response before slicing a 200 answer or caching a 206 answer. A proxy that ignores Range can make every block fetch download an entire multi-hour book; a malformed 206 can return the wrong range and produce corrupted audio. Cancellation while a prepared reader waits for unlock is ignored. Smallest fix: stream into a bounded block, verify range identity, refuse an ignored range that cannot serve the requested bounded block, and honour abort while waiting.

### 6. High — a stale runtime-settings form can overwrite another owner's changes
Files: `packages/client-core/src/server-admin/server-forms.ts:60`, `packages/client-core/src/console.ts:152–159`.
The runtime form sends its entire stale settings document as PATCH values. On a conflict the console swaps in currentRevision and resends those values without rereading or comparing them. Saving a changed server name can revert a newer connection limit, home networks or access policy. Even a true conflict on the edited key is retried automatically. The existing retry test asserts identical values are replayed, which proves the unsafe behaviour rather than safe merging. Smallest fix: send only edited fields, reread after a conflict, and silently retry only if those specific fields did not change (or already equal the requested values); preserve drafts on real conflicts.

### Fix milestone — controller lifetime and image subtitle budgets
- Controller: six deferred-response regressions failed before the fix (wrong session `s1` instead of `s2`, replacement ended, unexpected PATCH, and player load after disposal). All six now pass, along with the 23 existing core controller tests. Operations are fenced across awaits; queued changes stay with their title; stop pauses immediately and cleans up only its captured session; created sessions arriving after retirement are deleted.
- Image subtitles: reduced-budget hostile regressions failed before the fix (two regions/cues instead of one); they now pass. Combined active/indexed state and frame budgets are 16M pixels, retained data 96 MiB, total eager DVB/VobSub work 512M units. Limits can only be lowered in test parameters. Excessive parts increment `skipped`; remaining pictures survive. VobSub caches unique unit offsets and reads nonoverlapping control sequences; the image worker paints serially, and the page holds at most two outstanding paints (20 rapid seeks previously submitted 20).
- Targeted web checks: subtitle budgets/images/overlay, web playback/media player: 60 tests passed. Web `tsc --noEmit` passed. No Go files touched; Go vet is inapplicable.

### Fix milestone — safe settings saves
- Runtime saves derive a patch from the originally read values; a stale revision triggers a scoped reread and comparison of only those keys. Unrelated changes survive; changes to the edited key retain the draft and surface the conflict. The two new regressions failed before (connectionLimit reverted from 123 to 0; a true conflict sent two writes) and now pass.
- Quiet DraftForm reloads retain the original comparison basis while dirty, so remote changes do not accidentally become local edits in a stale whole-form draft.
- Console mutations no longer retry from a revision number alone. Job commands reread available actions before retrying; runtime settings compare their fields. Other mutations without a proof callback preserve their conflict. Lost-response reconciliation still uses the identical idempotency key and payload.
- Console test file: 30 tests passed, including owner/viewer fencing, receipt reconciliation and new race tests. Web type check passed. Shared package signatures changed internally only; no API route or wire shape changed.

Finding 4 follow-up, before fixing: `library-memory.ts:198–200` limits library count and individual filter size, but never total serialized size. Sixty-four libraries with sixteen filtered tabs each can exceed the 256 KiB read limit; the next launch discards the entire stored document. This is another route to lost remembered choices in the same persistence finding. Fix: enforce the serialized bound by evicting oldest library memories while retaining the newest.

### Fix milestone — follower freshness and remembered choices
- A broadcast leader acknowledges its current cursor. A joining follower whose original cursor differs requests a local resync exactly once; this never fabricates or advances a server cursor. New follower regression failed before (zero resyncs instead of one), passes after. Existing takeover, stream-error, backoff, hub dependency, quiet-update and stale-action tests pass: 27 in these three files.
- Remembered choices use one ordered asynchronous writer with the newest pending snapshot coalesced. Initial storage reads respect Forget tombstones, merge edits without dropping other tabs, and persist the merged result. Total serialized memory is kept under its own 256 KiB reader limit by dropping oldest library memories. Three before/after regressions reproduce reversed write completion, forgotten data resurrecting, and an unreadable overlarge saved document; all pass after. Memory file: 14 tests passed.
- Web type check passed. Snapshot follows; no API/schema/visual changes.

### 7. Medium — malformed event polling responses are accepted and retried in a tight loop
File: `packages/client-core/src/playback-v1/events.ts:253–264`.
A 200 response with `{}`, an HTML-proxy JSON document, a non-array `events`, or no cursor is treated as a successful empty poll and immediately repeated. The polling response also has no event-count or cursor-length bound in this client. Evidence: source trace; a regression will have the transport return `{}` once, then hold the next call, and assert it entered backoff instead of immediately issuing a second request. Smallest fix: validate the exact outer polling envelope and its bounds while retaining row-level tolerance, reject malformed envelopes so normal backoff applies.

### Final fix and verification notes

Finding 5 is fixed: Range bodies stream into at most one 256 KiB block, declared and actual sizes are checked, a 206 must name the requested byte range and file size, and ignored Range responses can only be used for a bounded whole small file at offset zero. Reads are capped at the existing largest MP4 index size (64 MiB), with only two block requests per read batch. Aborting a prepared wait or a stalled body settles immediately; closed readers cannot serve old cache hits. Every arriving chunk reports progress before the block completes. Four new hostile/cancellation regressions failed before the fix (accepted huge body/wrong range; wait hung) and pass after; valid cross-block and small-file cases pass too.

Finding 7 is fixed: malformed event polling envelopes reject into the existing backoff; rows remain tolerant. The client matches the server’s 100-event page bound and bounds cursors to 128 characters. Before the fix, a malformed first poll immediately sent a second request instead of scheduling backoff; the regression now passes.

A final settings regression additionally verifies loss of the response after a safe rebase: Retry replays the *exact* rebased payload and idempotency key, rather than conflicting with the original revision’s payload. Ended sessions never load media. Initial memory sort resets work even without another library edit.

Final checks (targeted files only):
- `npx tsc --noEmit` in `web` and `packages/client-core`: pass after all changes.
- From root, `node --experimental-strip-types --test` with these explicit client-core files: `session-lifetime`, `console`, `library-memory`, `event-feed`, `playback-v1-core`, `events-retry`, `freshness-hub`, `freshness-actions`, `tolerant`, `library-content-exact`, `home` (each under `packages/client-core/test`, `.test.ts`): **135 passed, zero failed**.
- From `web`, `node --experimental-strip-types --import ./test/register.mjs --test` with explicit files: `subtitle-budgets`, `subtitle-images`, `subtitle-overlay`, `audio-range`, `audio-track`, `audio-demux`, `audio-seek-fixtures`, `web-playback`, `web-media-player`, `backups`, `delete-media-idempotency` (each under `test`, `.test.ts`): **88 passed, zero failed**.
- **223 targeted tests passed in the final verification.** These are not full suites. No Go files/routes/schema were changed, so Go vet/apigen/schema-sync are inapplicable.
- Review also checked console viewer fences and mutation receipts, Home/content envelope scoping, libass worker wiring, and delete/revoke/restore confirmation handlers. No `innerHTML`, `dangerouslySetInnerHTML`, or `document.write` rendering of server/file text was found under `web/src`. This is source evidence, not a runtime security attestation.
- `git diff --check`: clean. No UI layout or appearance changes. No added dependency, owner/viewer setting, route, schema or user-facing catalogue string.

Snapshot history (cumulative against ce34473):
1. `snapshots/review-web-logic/001` — Keep playback responses with their session and bound subtitle decoding.
2. `snapshots/review-web-logic/002` — Preserve concurrent settings changes when saving owner drafts.
3. `snapshots/review-web-logic/003` — Refresh returning tabs and keep remembered choices ordered and readable.
4. `snapshots/review-web-logic/004` — Bound music delivery and reject malformed event polling documents. Final cumulative snapshot: 77,124 bytes tracked patch, three new test files; `git diff --check` clean. No worktree changes followed this snapshot.

Final disposition: **0 blocker, 3 high, 4 medium, 0 low; 7 fixed; 0 confirmed findings left.** Remaining work is integration of the shared package copies and browser/device acceptance, plus the historical out-of-scope items explicitly listed in section 3.
