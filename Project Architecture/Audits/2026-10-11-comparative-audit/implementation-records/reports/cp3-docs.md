# cp3-docs: docs, tests and cleanups

Worktrees: `server-cp3-docs` and `native-cp3-docs` on branch `audit-implementation/cp3-docs`, plus `main-internal` for the docs only. `audit-implementation/main` is merged into both code branches: server main is 2281643a, native main is 8d4e109. Nothing was pushed. No subagents or helpers were used. Execution host: mini-1 (Ubuntu).

## 1 Outcome

**Trivial cleanups (T1–T9)**
- **T1.** CORS no longer allows `X-Playback-Controller-Token` or `X-Portico-Device-Id`. The iOS listening owner no longer sends the token header, and its unused `proof:` parameter is gone. Three Go test fixtures no longer set the header.
- **T2.** `gofmt` is clean: `playbackv1/admin.go`, plus two other unformatted files I found (`catalog/playlist_types.go`, `metadata/service.go`).
- **T3.** There were two web hooks called `useQuietRefresh`. The refused-list retry in `app/quiet-refresh.ts` is now `useRefreshRequiredRetry`, and its 5 callers are updated. The freshness hub's `useQuietRefresh` keeps its name.
- **T4.** Deleted the stale native build notes `screens/NOTES-search-saved.md` and `screens/NOTES-live.md`. The live one named files that no longer exist.
- **T5.** Removed dead TV code from native `Dashboard.tsx`: the sparkline sizes, the TV stroke width and an unused `client` in `StatisticsPanel`.
- **T6.** `'vega'` added:
  - to client-core `DestinationPlatform`. Vega is a television, so Play on is not offered there. Test added.
  - to native `DevicePlatform`. Vega's override now types `devicePlatform` and `playOnPlatform` as `vega` with no casts.
- **T7.** The web page cover shown when a different server answers at the address now has its own wording (`web.connection.mismatchBody`, under the "A different server is answering" title). `web.connection.reconnectingBody` keeps the launch case, reworded to fit it.
- **T8.** Checked: `deleteMedia.titlesCovered`, `.fileChanged` and `.fileNotRemoved` are all used (web delete dialog, web DeleteVersion, native TitleTools). No change.
- **T9.** Half done.
  - `HandoffSnapshot` now publishes the committed `handoffId`, from both `send` and `reconnect`. Native `play-on.tsx` drops its transport wrapper and the `committedHandoffId` parser, and remembers the handoff from the controller's state instead.
  - `paused-load.ts` stays. See section 9.

**Tests (X1)**
- **Kotlin JVM unit tests** (Android local unit tests, `app/src/test`):
  - `PorticoMpvSupportTest`: value checks, rendition mapping, embedded subtitle matching, languages, caption style, decimal formatting.
  - `PorticoAudioPlanTest`: decode pairs, playable plans, gain and peak limits, duration.
  - `PorticoDownloadWorkTest`: the old standalone `test/download-work-tests.kt` converted to JUnit and that file removed.
  - Wiring: `testImplementation("junit:junit:4.13.2")` added, and CI's android job now runs `:app:testGoogleDebugUnitTest :app:testFiretvDebugUnitTest`.
- **MCP tool tests** (`server/internal/httpapi/mcp_tools_test.go`):
  - Every one of the 44 tools must be offered to an owner's admin key unless its feature is not set up. Only `tv_guide` is unset in the fixture.
  - Each tool must have unique names, describe every required argument and call `/v1/` routes only.
  - A call to each tool must reach its route. 41 tools are called with real catalogue data, and the webhook tools go through create → update → test → deliveries → delete → gone.
- **Download recovery tests** (appended to `apps/native/test/offline-ownership.test.ts`), driving the real `OfflineDownloads`:
  - `tend()` restarts stopped transfers but not held or unmendable ones.
  - After a 410 it backs off for a quarter of an hour.
  - It completes missing extras later without transferring the file again.
  - `recover()` adopts a fresh claim for the same file, flags a changed file without touching the copy, and leaves a still-preparing title for the next renewal.
  - Test-harness fix: the stub's `setMeta` now announces the change, as both platforms' native modules do.

**Documents (X2)**
- `THIRD-PARTY-NOTICES.md`: the MPVKit and media-kit sections are replaced by Portico's own media engine (component, version and licence table; LGPL-2.1-or-later throughout; what was dropped).
- `ENGINE-CONTRACT.md`:
  - §7: cue address corrected to `/v1/media/subtitles/<grant>/…`.
  - §10: split into an Apple list and an Android/Fire list. Android has conditional direct HDR, no DV5 and no deinterlacing.
- `web/README.md`: describes the shared `/v1/events` stream (Web Locks or a local-storage lease, long poll only as the last fallback) and the freshness hub.
- Native `bridge/README.md`: adds `event-stream.ts` and the `vega` platform name.
- Native `README.md`: the stale "Known gaps" list is removed (all three items are delivered).
- API guide `03-browsing.md`: capped totals (`totalIsLowerBound`, `totalAtLeast`, "10,000+"), `groupOrder`, and `POST /v1/browse`.
- main-internal:
  - `02 Server.md`: capped totals, `list.changed` and admin hints.
  - `03 Catalogue`: capped totals; sort/filter parity is now delivered; collections keep a manual order.
  - `04 Playback`: own engine; the player-choices rule; engine changes re-plan in place (the old "replacement session" claim was wrong); PiP without drawn subtitles.
  - `06 Clients.md`: Vega described; two engines; player-choices rule; open-ended client totals; live filters; owner panels on phones and tablets with a read-only TV Dashboard; About shows licences; Vega transport working with the accepted risk.
  - `01 Overview.md`: the same Vega transport correction.
  - Playback Protocol spec: the player-choices rule, in-place engine re-plan, own engine, subtitle drawing, PiP, and the MP3/ADTS/Ogg seek index.
  - NAV-6.7: filters apply live, and Back keeps them.
  - `Backlog.md`: engine mentions.

**Justin's additions (coordinator message, 15 Oct)**
- **Dropped proposals removed or marked**: spoiler controls, writing NFO and artwork beside the media, country rating tables, per-library unrated / kids allow-list, and unified profile and account label rules.
  - main-internal: `Backlog.md`, `architecture-amendments.md`, `metadata/contract.md` (the write-beside export is no longer promised) and `media-presentation/contract.md` (the spoiler-policy line is removed).
  - `_program/reports/decision-audit.md`: D-26, D-39, D-52 and S3 are marked "dropped by Justin 15 Oct".
  - `_program/reports/remaining-sweep.md`: C-8 is marked the same way.
  - The audit's `00-FINAL-REPORT.md` decision register is only in Justin's main portico-internal checkout (untracked there), so I did not touch it.
- **About page licences confirmed on every client.**
  - Web: About › Open-source licenses opens the licence list.
  - Phone, tablet, Apple TV, Android TV and Fire TV: the shared `SettingsPage` About shows the "Open-source licenses" row, which pushes `Licences`.
  - Vega: renders the same shared App and About with its own `vega.json` loader.
  - Each build's list includes the own engine (`engine:ffmpeg@8.1.2`, `engine:mpv@0.41.0`, `engine:libass@0.17.5`).
  - No gap was found, so no code change was needed.

## 2 Decisions I made

- **The Kotlin tests use JUnit 4 as a test-only dependency.** It never ships, so the licence lists don't change. Gradle offline can't run them on this machine because JUnit is not in `~/.gradle` and I can't download it. The first Gradle run needs a network once; CI has one.
  - I verified the tests here instead: I compiled them with the Kotlin 2.1.20 compiler from the Gradle cache, together with all app sources, against a scratch JUnit-compatible stub (`org.junit.Test` and `Assert`, kept outside the repo), then ran them with a reflective runner. 15 of 15 pass.
  - The alternative was another standalone `main()` file like the old download test. I rejected it because CI could not discover it.
- **T1 removes the iOS `proof:` parameter entirely** rather than just the header, because every caller passed `nil`.
- **T3 renames the refused-list hook**, not the hub hook, because cp3-api is changing the hub hook's callers (Guide, DVR, Downloads) tonight. That keeps merges small.
- **The MCP tests accept "temporarily unavailable" refusals** for tools whose service the administration fixture does not build (Metadata, backups, DVR, the health panel). They still prove that arguments pass the schema and that the call reaches an API route. `start_library_scan` and `report_changed_paths` are not called because the fixture has no ingestion service, and calling them panics inside the fixture. Their listing and scope filtering are covered by the existing test.

## 3 Audit items closed

| ID | What changed | Test |
|---|---|---|
| T1 | CORS headers trimmed; Obj-C header and parameter removed; fixtures updated | `go vet ./internal/httpapi`; MCP and playback fixture tests pass |
| T2 | gofmt on 3 files | `gofmt -l` clean on the touched packages |
| T3 | `useRefreshRequiredRetry` | web `tsc`, `lint:hooks` |
| T4 | 2 stale notes deleted | — |
| T5 | dead TV sizes and unused variable removed | native `tsc`, `lint:hooks` |
| T6 | `'vega'` in both platform types | `destinations.test.ts`; native and Vega `tsc` |
| T7 | `web.connection.mismatchBody` | web `tsc`, `lint:strings` |
| T8 | verified used | — |
| T9 (half) | `HandoffSnapshot.handoffId`; wrapper and parser removed | `remote-playback.test.ts` (asserts `handoffId`), `play-on-memory.test.ts` |
| X1 | Kotlin, MCP and download recovery tests | see section 7 |
| X2 | documents listed in section 1 | API guide checker |

## 4 API changes

- CORS `Access-Control-Allow-Headers` no longer lists `X-Playback-Controller-Token` or `X-Portico-Device-Id`. The server read neither.
- client-core `HandoffSnapshot` gains `handoffId?: string`, and `DestinationPlatform` gains `'vega'`.
- No route changes. `apigen` produced no diff.

## 5 Schema changes

None.

## 6 Settings added

None. One string was added: `web.connection.mismatchBody`.

## 7 Tests run

- `go test ./internal/httpapi -run 'TestMCP|TestPlaybackV1Current|TestCurrentPlayback'` → ok (after the merge).
- `go vet ./internal/httpapi ./internal/playbackv1 ./internal/catalog ./internal/metadata` → clean. `go run ./cmd/apigen` → no diff.
- client-core `destinations.test.ts` and `remote-playback.test.ts` → 13 of 13 pass.
- Web: `tsc --noEmit` clean, `lint:hooks` clean, `lint:strings` clean. `server/api/guide/check-examples.mjs` → all validate.
- Native:
  - `tsc --noEmit` and `tsc -p test/tsconfig.json` are clean, and so are `lint:hooks` and `lint:strings`.
  - `offline-ownership.test.ts` (45), `play-on-memory.test.ts` and `paused-load.test.ts` → 55 of 55 pass.
- Vega: `tsc`, test `tsc` and `lint:strings` all clean.
- Kotlin: all app sources plus the tests compile with K2JVMCompiler 2.1.20 against android-36, using a JUnit stub. The 3 test classes pass 15 of 15. Not run through Gradle.
- Shared packages re-mirrored (`--write`); the lock names server 8ca0bd56.

## 8 Device test notes

- **Android, with network access once:** `cd apps/native/android && ./gradlew :app:testGoogleDebugUnitTest :app:testFiretvDebugUnitTest`. Pass: 15 tests per flavour. A resolution failure for `junit:junit:4.13.2` means the run was offline: run it once online.
- **iOS listening (Mac build):** `PorticoListeningOwner.m` compiles without the `proof:` selector parts. Audiobook and music playback, progress and stop work as before.
- **Play on (phone → TV):** send a title to a TV, quit the phone app, then reopen it. Pass: the controls for the TV session come back, which means the committed handoff was remembered from the controller's state.
- **Web:** change the server at a saved address to another Portico server while a page is open. Pass: the cover reads "A different server is answering" with the new body text.

## 9 Not finished

- **T9, first half (`paused-load.ts`).** Not done. Removing it needs `PlaybackService.play` to take a starting state through the device-queue path (`queue.playItem`) and the native queue controller. That is a playback-core change in the area cp3-nplay is editing tonight (N10/N22), not a trivial cleanup. The helper works and has its own test.
- **X1, partly not done.**
  - No render tests for the new native screens. This is not in my brief's X1 list, and native has no React render harness.
  - `start_library_scan` and `report_changed_paths` are not called in the MCP test (fixture limitation, see section 2).
- **X2, partly not done.**
  - Not checked: other lanes' requested edits to the PiP/AirPlay product contract, A11Y-10, NAV-11 (invitation lifetime) and `Spec — Title Pages`.
  - Not updated: architecture text for items being fixed tonight by other lanes (pause covering the visibility builder, the deep-analysis timeout, the hidden empty library row, Apple local-only HTTP). It should be refreshed after those lanes merge.
- **Decision register:** the audit's own `00-FINAL-REPORT.md` register (D-26, D-39, D-52) lives only in Justin's main portico-internal checkout, so it is not marked.

## Commits

**Server (`audit-implementation/cp3-docs`):**
- 763e0478 Stop allowing two request headers nothing reads, and format the remaining Go sources
- 4a78a28e Give the refused-list retry hook its own name
- af5dedb6 Say that another server is answering when the page is covered for a changed identity
- 40e66cbe Name Vega as its own platform for Play on
- 71abc385 Publish the committed handoff on the Play on controller's state
- aca1085a Test that every MCP tool is offered, described and reaches its route
- 8af86fc2 Describe the apps' own media engine in the notices and the shared event stream in the web README
- 4de29020 Document capped totals, group order and lists across libraries in the browsing guide
- 8ca0bd56 Merge `audit-implementation/main`

**Native (`audit-implementation/cp3-docs`):**
- 56344eb Stop sending the old playback controller header from the iOS listening owner
- 7edb31c Keep a committed Play on handoff from the controller's own state, name Vega as its platform, and drop dead TV dashboard sizes and stale notes (includes the mirror)
- 239f3a6 Add JVM unit tests for the mpv engine's rules, the audio plan and download file work, and run them in CI
- 87e74f9 Test that downloads restart, back off, complete their extras and recover forgotten receipts by themselves
- e669fa8 Bring the engine contract and native READMEs up to date
- b827163 Merge `audit-implementation/main` (re-mirrored, lock → 8ca0bd56)

**main-internal (`audit-implementation/main`):**
- 7f58c9a Take spoiler controls, writing beside the media, country rating tables, kids allow-lists and unified label rules off the plans
- c674251 Bring the architecture documents up to date

**_program (not a repository):** `reports/decision-audit.md` and `reports/remaining-sweep.md` are annotated with the 15 Oct drops.
