# fix-final

Host ehler-personal (mini-1), 16 Oct 2026. Worked only in `server-fix-final` (from main-server 0a416ed8) and `native-fix-final` (from main-native 7e72d0b), branch `audit-implementation/fix-final`. Committed with Justin's identity, never pushed. No subagents or helpers. `GOFLAGS=-p=4`, own `GOCACHE` (deleted at the end), `nice -n 10`. Free disk never below 75 GB. `audit-implementation/main` had not moved in either repo, so there was nothing to merge. No shared package changed, so no mirror was needed (`shared-packages.mjs --check`: identical).

## 1 Outcome

All four bugs are fixed at their cause. Each has a test that fails on the old code. Bugs 1–3 were also checked against a live server, and bug 1 on the Android emulator.

1. **mpv + preselected subtitle stopped at 0:00.** This was not an mpv problem. Any re-plan in place broke this way, on every engine.
   - Picking a subtitle before Play makes the server plan the session again (generation 1 → 2). The service deliberately keeps the same intent for that (26997872, "Keep the queue when the viewer changes quality, version or audio").
   - Every engine's command check refused "same intent, different binding" as `Stale playback binding.`: Android mpv, Android Media3, Apple mpv, Apple AVPlayer and Vega. Only Media3 and AVPlayer had an exception, and only for a live channel's next window.
   - So the engine kept loading generation 1. The server had already withdrawn that media grant: the 404 `Relay` and `moov atom not found` in verify-2's logcat were the old grant failing. The start then timed out.
   - Under Automatic it worked only because the title first started on the native engine and then switched engines, and a switch takes a new lease.
   - Fix: every engine now accepts a newer generation with the same lease, intent, session and title as a re-plan in place, and opens the new source. Anything else is still refused. ENGINE-CONTRACT.md says so.
   - On the emulator: before the fix, I reproduced verify-2's exact symptom and logged the refusals (`rejected 3 Stale playback binding.`, then 4 and 5). After it, the same title played to the end with the Arabic ASS line drawn by mpv (`android/5.png`).
   - This also affects quality, version and audio changes during playback on the native engines, which the old rule would refuse in the same way. I could not drive that through the UI on the emulator in time (device note below).
2. **Brief 404 after "Not this artist".**
   - The new artist had no browse row until the derivation worker reached it, and `VisibleEntity` (personal-state and the other container reads) needs that row.
   - The row is now derived in the same transaction (`compactcatalog.DeriveBrowseRowTx`). The fix is on the server, so web, phone, tablet and TV all get it; native's "Not this artist" uses the same route.
   - Live check: `POST /v1/admin/titles/separate-artist`, then immediately `GET /v1/containers/artist/{new}/personal-state` → 200.
3. **Album id during a first scan.**
   - A tagless first pass files the album under "Unknown artist" (and a book without its author). The tag pass moves its tracks to a new album and deletes the first one.
   - Now, when an emptied album or book is deleted because its last member moved, its public id is written to `catalog_merged_ids`, pointing at the container the member went to. Ids it had absorbed are chained along.
   - The existing merged-title resolver (paths, bodies and queries) then sends old links there.
   - A container kept for its state (retired) keeps its own id. Files going missing redirect nothing.
   - Live check: three new albums scanned. Each first pass left a redirect, and reading the album page (`/content?view=album&entityId=<old>`) and `/v1/containers/album/<old>/personal-state` by the provisional id both answered 200 with the real album.
4. **Load-sensitive queue tests.**
   - The real cause was not the 2 s scan budget. When I set `scanBudgetTime` to 1 ns, the test still passed, because the first query batch is always one full chunk.
   - The cause: the test's device `shuffle_device` was never registered in `identity_devices`. The sweep that every queue create starts in the background removes "device queues whose device is gone". Whenever it ran between the create's commit and its reply read (under load), the reply showed the queue empty (`Total:0 Segments:[]`).
   - **Not a product bug.** A real caller's device always exists: `deviceOf` resolves it through `identity_device_families`, which references `identity_devices`.
   - Fix (tests only, no product limit changed):
     - Register the device.
     - Run the sweep synchronously in the test. Removing the registration now fails deterministically.
     - Release every capture a test's builds still hold in `t.Cleanup`. In the forced-failure run, the next test (`…AdmissionKeepsTheRequestDeadline`) passed, so the cascade is gone.

## 2 Decisions I made

- **Bug 1 fixed in the engines, not in client-core.** Keeping the intent across an in-place re-plan is how the service is designed: queue operations, timers and audio plans are all keyed by intent id. The Media3/AVPlayer linear `newWindow` exception already treated "newer generation of the same session and title under the same intent" as current. I generalised that one rule, through one helper per platform (`PorticoMpvSupport.replannedInPlace` on Kotlin and Swift, inline in ObjC, `replannedInPlace` in Vega), and changed nothing else.
- **Bug 2: derive only the artist's browse row in the request, not the moved songs' memberships.** That is one row instead of up to 5,000 per album in an interactive write. Until the edges are derived, the artist is visible as an empty container, which is the existing catalogue contract for a fresh container.
- **Bug 3: redirect, not re-key.** Re-keying the provisional album in place would have been the more invasive scanner change (multi-track albums move one track at a time).
  - The redirect reuses cp4-leftovers' resolver and also covers tag edits that change an album's identity later.
  - Not redirected: an emptied "Unknown artist" (or any artist). Pointing a shared bucket at whichever artist took the last song would be arbitrary.
  - Cost: `SurvivingTitleIDs`'s "no merged titles" short cut will now usually be false on servers with music. A request body that names a title id is then parsed and looked up (one primary-key seek per id), not passed through byte for byte. It's still flat per request, and the rows are 16-byte keys (about one per album, ever).
- **Bug 4: no product change** (see above).

## 3 Audit bugs closed

| Bug | Change | Test (fails on the old code) |
|---|---|---|
| mpv + preselected subtitle stops playback (verify-2 §9) | Engines accept a re-plan in place: `PorticoMpvController.kt`, `PorticoPlaybackController.kt`, `PorticoMpvSupport.kt`, `PorticoMpvController.swift`, `PorticoMpvSupport.swift`, `PorticoPlaybackController.m`, Vega `bridge/playback.ts`; ENGINE-CONTRACT.md | JVM `PorticoMpvSupportTest.aNewerGenerationOfTheSamePlaybackIsAReplanNotAStaleCommand`; Vega `a newer generation of the same session under the same intent is a re-plan in place: its source opens` (checked failing with the rule removed); emulator before/after |
| Brief 404 after "Not this artist" (verify-2 §6) | `compactcatalog.DeriveBrowseRowTx`; `SeparateArtist` calls it for the new artist | `catalog.TestOwnerSeparatesAlbumsFromAnArtist` (new `VisibleEntity` check; fails without the call) |
| Album/book id during a first scan (verify-2 §9) | `settleEmptiedInto`: a deleted, emptied album or book leads to where its last member went; `retireEmptiedAlbumTx`/`settleEmptiedBook` take that target | `catalog.TestTaglessFirstImportThenTagsLeavesOneAlbum`, `…LeavesOneBook` (old id resolves to the new one; `SurvivingTitleID`) — both fail with the redirect disabled |
| Load-sensitive queue tests (verify-2 §9) | Test device registered, sweep run in the test, captures released in cleanup | `TestGrowingQueryShuffleKeepsTheSameSeededOrder` fails deterministically without the device; passes 15/15 repeated, and 40/40 earlier under parallel catalog load |

## 4 API changes

None. Behaviour changes:
- An old (provisional) album or book id now answers for the album or book that replaced it, instead of 404.
- Engines accept a newer generation's command for the same playback.

## 5 Schema changes

None. `catalog_merged_ids` is used as it is.

## 6 Settings added

None.

## 7 Tests run

| Command | Result |
|---|---|
| `go test ./internal/catalog/... ./internal/compactcatalog/... ./internal/playbackv1/... ./internal/httpapi/... ./internal/entityid/... ./internal/ingestion/... ./internal/persistence/...` (helper = this branch's server binary) | all ok (catalog 295 s, scalebench, compactcatalog, playbackv1, httpapi 222 s incl. read-path golden, httpapi/fixture, ingestion, persistence) |
| `go test ./internal/playbackv1/ -run 'TestGrowingQueryShuffle\|TestQueryCapture' -count=15` | ok |
| `go vet` (catalog, compactcatalog, playbackv1); `gofmt -l` | clean; empty |
| `go run ./cmd/apigen` | no diff |
| native `npm run typecheck`, `typecheck:test`, `lint:hooks`, `lint:strings`, `npm test` | 0, 0, 0, clean, 701/701 |
| Vega `npm run typecheck`, `typecheck:test`, `lint:strings`, `npm test` | 0, 0, clean, 89/89 |
| `shared-packages.mjs --check` | identical |
| Gradle `:app:assembleGoogleDebug :app:testGoogleDebugUnitTest :app:testFiretvDebugUnitTest -PreactNativeArchitectures=x86_64 -PporticoLibmpvDir=…/verify-2/engine-out/android` | BUILD SUCCESSFUL; 18/18 and 18/18 |
| Emulator (Portico_Phone_API36, own Metro on 19434, local fixture server) | before: stopped at 0:00, three refused commands logged; after: plays to the end, Arabic+Latin ASS line drawn by mpv |
| Live server probes (`scratch/fix-final/tools/album-ids.mjs`, `old-ids.mjs`, `separate.mjs`) | provisional album ids → 200 with the real album; new artist personal-state → 200 straight after the split |

Not run: client-core/web suites (no client-core or web file changed), the full `go test ./...`, the e2e drivers (the orchestrator's final run).

## 8 Device test notes

- **Android phone, Player = mpv:** pick a styled (`.ass`) subtitle on the details page, then Play. It should start and draw the track. Also pick one while playing.
- **Every engine, during playback:** change Quality, Version and Audio track (iPhone/Apple TV AVPlayer, Android Media3, Fire TV, Vega). Each now reloads in place at the same position. Under the old rule they would have stalled ("start timed out"). **Apple Swift/ObjC was not compiled here:** please build iOS/tvOS first.
- **Apple mpv:** the same subtitle-before-Play check.
- Add a new album folder to a music library and open it from Recently Added during the scan. It should never 404, and should land on the album under its real artist.

## 9 Not finished / observations

- I could not exercise a native (Media3) quality change through the emulator UI. The 45 s fixture film ended before I could reach the menu. The rule change is the same one that is proven on mpv; it is covered by the JVM and Vega tests.
- Swift and ObjC edits are reviewed but not compiled (no Xcode here).
- An emptied artist (including "Unknown artist") is still deleted without a redirect (decision above).
- Observed, not changed: a client that cancels the `GET /v1/events` long-poll is logged as `500 internal: context canceled` (server log noise, seen once).
- **Emulator incident:** the AVD image still held an old app state signed out of `demo.getportico.tv` (username "demo"). While dismissing the Metro error screen, one tap landed on that sign-in form. The screenshot shows its Sign in button spinning with a one-character password field. I did not enter that character; it may have been left from the image's old state. So the form may have been submitted once to demo.getportico.tv. I cleared the app's data immediately and used only the local fixture server after that. No real credential was involved.

## Commits

- portico-server `audit-implementation/fix-final`: 015f701d (new artist visible at once), 2c59d3a9 (first album/book ids lead on), 1808094b (queue capture tests).
- portico-react-native `audit-implementation/fix-final`: a14841a (engines accept a re-planned presentation; contract, Vega and JVM tests).

## Clean-up

- Deleted: GOCACHE, server binaries and test binaries, and the fixture state (it held generated credentials). The app was uninstalled, the emulator and Metro stopped, and the Gradle outputs deleted.
- The native worktree's `node_modules` is a symlink to main-native again. I temporarily replaced it with a hardlinked copy while Metro ran, because Metro does not follow the symlink.
- The server worktree keeps its own `node_modules` (`npm ci`).
- Kept in `_program/scratch/fix-final/`: logs (JS logs hold session ids, no credentials), tools, and Android screenshots (no passwords).
- No process of this lane is running.
