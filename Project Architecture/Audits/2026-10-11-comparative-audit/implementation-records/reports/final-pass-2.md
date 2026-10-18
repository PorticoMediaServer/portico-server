# final-pass-2

Host ehler-personal (mini-1), 15 Oct 2026 18:37–19:45 (ADT). Worked directly in `main-server` (from a4f86250) and `main-native` (from 01b78bd), branch `audit-implementation/main`; committed, never pushed. No subagents, no helper model. `GOFLAGS=-p=4`, `GOCACHE=_program/scratch/gocache-final-2` (deleted at the end), `nice -n 10`. Free disk never below 90 GB.

## 1. Outcome

- **Everything passes after five server fixes and one native mirror.** Server `go test ./...` (87 packages with tests, 16 without), apikit, client-core 1,977/1,977, web `npm run check` 581/581, cast 21/21, contracts 30/30, production web build, Windows/macOS cross-builds (amd64 and arm64), `go run ./cmd/apigen` no diff, both native type-checks, `lint:hooks`, both string lints, native 676/676, Vega type-checks and 87/87, licence lists current (web and native), shared packages identical. `go vet` shows only the 63 known vendored SQLite warnings; `scripts/vet-go.sh` passes.
- **End to end passes:** server driver 18/18 groups; browser driver 13/13 groups with 114/114 screens; an extra one-off walk of 13 completion-pass screens at both widths (26 observations) all rendered.
- **Android works end to end on the emulator:** `:portico-mpv:assembleRelease` and `:app:assembleGoogleDebug -PreactNativeArchitectures=x86_64` build; on the Android 16 x86_64 emulator the debug app signed in to a local server, Player = mpv was offered and chosen, and a generated film played through mpv (picture advancing, audio focus taken, PorticoMpv logging).
- **The flaky test was a test bug, not a race in queue capture.** Its "hold" never held (details in section 4). Fixed without sleeps; 20/20 under `-race` after the fix while the unfixed code failed under the same conditions.
- **Justin's decision (15 Oct) is implemented:** starting the next episode during the credits is a viewer setting again, **off by default**, on web and every native client.

## 2. Command table

Logs: `_program/scratch/final-pass-2/logs/` (no credentials).

| Command | First run | Cause | Fix / final |
|---|---|---|---|
| helper build `go build -o …/playback-test-helper ./cmd/server` | exit 0 | | rebuilt after the schema fix (see `cmd/server` row) |
| `go vet ./...` (server) | exit 1, 63 diagnostics, all `possible misuse of unsafe.Pointer` in `internal/thirdparty/sqlite` | known vendored SQLite | none; rerun at the end identical. `sh ../scripts/vet-go.sh` exit 0 |
| `go test ./...` (server), 8 min, load 6–17 | 5 packages' tests failed | below | |
| — `compactcatalog.TestPerItemDerivationStatementsSeek` | `INSERT INTO catalog_entities` planned `SCAN saved_entry_undo` | the playlist/collection undo table (cp-queues-playlists) has an `item_id` foreign key to catalogue entities with no index | `saved_entry_undo_item_id_fk` index in the baseline's child-side FK index block, `schema-sync.py` rerun; 93d1dcad |
| — `httpapi.TestReadPathGoldenEquivalence` | 136 of 422 cases differ | classified all 136: 134 browse answers only gain `sortLetter` (B-07.10, cp-browse-parity); the two capabilities answers gain the `cast`, `crew`, `savedCollection`, `homeRow` fields and surfaces, the `groupOrder` sort and `requiresGroup` (cp-browse-parity). No entry, order, total or status changed | regenerated with `PORTICO_GOLDEN_UPDATE=1`; 4eaa5fdd |
| — `httpapi.TestPlaybackV1QueueQueryCapturesBeyond500AndKeepsItsSnapshot` | `first batch: … Total:520 … State:ready` | test bug, section 4 | `HoldQueueBuilds`; 00767ddb |
| — `operations.TestStoredSettingsHaveRuntimeConsumers` | 7 stored settings with no reviewed consumer: `DVRDefaults.liveTunerConflict`, `LiveSourceSettings.importLimits.{bytes,channels,programmes}`, `MetadataSettingsUpdate.openSubtitles{ApiKey,Username,Password}` | settings added by cp-native-playback/cp-owner-parity without guard entries | verified each has a real runtime consumer and listed it: tuner allocator reads `live_tuner_conflict` (`livechannels/allocator.go`), per-source limits go through `SourceImportLimits` (`livechannels/configuration.go`), credentials reach the provider through `SetCredentials` (`cmd/server/main.go`). Guard unchanged; 33c0a432 |
| — `operations.TestStoredPreferencesHaveRuntimeConsumers` | stale consumer `playback.startNextDuringCredits` | my preference edit was mid-way when this package compiled | gone once the preference commit was complete (246f2543) |
| `go test ./...` rerun 2 (after fixes, during two `-race` runs) | 1 failure: `cmd/server.TestRecoverOwnerCLINeedsNoKey` "schema migration 1 digest mismatch" | stale helper binary: built before the baseline index was added | helper rebuilt; `cmd/server` ok |
| `go test ./...` rerun 3 (fresh helper) | exit 0: 87 packages ok, 16 without tests, 8 min 29 s (the flaky test passed here and in rerun 2, both under full-suite load) | | |
| apikit `go test ./...` / `go vet ./...` | exit 0 / exit 0 | | |
| `go run ./cmd/apigen` | no diff at the end (my contract change regenerated in 246f2543) | | |
| Windows/macOS × amd64/arm64 `CGO_ENABLED=0 go build ./...` | all exit 0 | | |
| `npm run test:core` | 1,977/1,977 | | |
| `npm run check --workspace portico-web` | exit 0, 581/581 (tokens, strict design lint, oxlint hooks, 39 preference consumers, string lints, tsc, tests) | | `npm ci` not needed: oxlint is installed |
| `npm run test:cast` | 21/21 | | |
| `packages/contracts` `npm test` / `npm run build` | 30/30 / exit 0 | | |
| production web build `--configLoader runner` | exit 0 (chunk-size and one ineffective-dynamic-import warning only) | | |
| licences: `web/scripts/licences.ts --check`; `apps/native/scripts/licences.mts --check` (engine notices from `scratch/own-mpv/out`) | up to date / exit 0 | | no lockfile changed |
| native `shared-packages.mjs --check` | 8 files differ, all from my preference commit (identical to a4f86250 before it) | | `--write` at 00767ddb (main-server HEAD), `--check` identical; native 5b6231f |
| native `typecheck`, `typecheck:test`, `lint:hooks`, `lint:strings` | all exit 0 | | |
| native `npm test` | 676/676 | | |
| Vega `typecheck`, `typecheck:test`, `lint:strings`, `npm test` | exit 0, 87/87 | | |
| flaky test `-race -count=20` (fixed code, under load: second race run, full suite, Gradle, emulator) | **20/20 pass**, no data race, 42 min (96–220 s per run under the race detector and load) | | |
| same, unfixed code (a4f86250 in a scratch worktree), same conditions | **2/20 fail** with the reported "first batch: Total:520 … ready", no data race reported, 41 min (a first attempt without `-timeout` hit the 10-minute package limit) | | |
| server driver `scripts/e2e-server.py` | 18/18 groups, 89 s. browse n=323 median 11.8 ms / max 116 ms; search n=243 median 22.2 ms / max 350 ms (host loaded by two `-race` runs) | | |
| browser driver `web/test/integration/browser-smoke.mjs` | 13/13 groups, 114/114 screens, 4 min 23 s | | |
| one-off new-screen walk (scratch tool, not committed) | 26/26 rendered | | |
| `:portico-mpv:assembleRelease` | BUILD SUCCESSFUL, 47 s | | |
| `:app:assembleGoogleDebug -PreactNativeArchitectures=x86_64` | BUILD SUCCESSFUL, 2 min 46 s | | |
| emulator playback with Player = mpv | played | | |

## 3. Fixes made

Server (`main-server`):
- 93d1dcad — index `saved_entry_undo(item_id)`, so deleting or inserting a catalogue entity never scans the undo list.
- 4eaa5fdd — read-path golden re-recorded after classifying every differing case (above).
- 33c0a432 — stored-settings guard names the real runtime consumers of the tuner-conflict, per-source import-limit and OpenSubtitles settings.
- 246f2543 — **Justin's decision (15 Oct): starting the next episode during the credits is a configurable choice, off by default.**
  - Registry: `playback.startNextDuringCredits`, boolean, default `false`, profile scope, consumer `client.playback.continuation`, next to autoplay and the countdown. Consumer guard and runtime-source guard list it; bootstrap contract golden updated (the new key only).
  - Server publishes it in each queue's `postPlay` as `startDuringCredits` (always present). OpenAPI and contracts regenerated with `apigen`.
  - client-core: `QueuePostPlay.startDuringCredits` (absent = off) in the queue header parse, the v1 queue player's view and `parseQueuePostPlay`.
  - Web player: `upNextCountdown` counts down to an early start only when the preference is on **and** autoplay is on **and** the credits marker is `automaticSafe`; otherwise the credits card still shows the next title's artwork and "Starts in m:ss" (rest of the title plus the end card's countdown) and nothing advances before the title ends. On: the earlier behaviour, still a `completion` advance that counts toward "Still watching?".
  - Settings: one row in the shared settings structure (Playback › Next episode, shown while autoplay is on), so web, phone, tablet, TV and Vega all draw it; label "Start the next episode during the credits" with help text.
  - Native players: none starts the next title during the credits (no credits countdown exists in `apps/native`); nothing to change. The separate "Credits: Skip automatically" setting (default "Ask each time") still skips a sure credits marker on web and native when the viewer chose it; that is its own choice and was left alone.
  - Tests: client-core (default false, carried through from the server, malformed refused), web `up-next-credits.test.ts` (off by default and when false: not automatic; on: automatic; the card feeds the preference and leaves early only on an automatic countdown). Preference-consumer check passes with 39 keys.
  - Seen running: web Settings › Playback shows the row, off (screenshot); native app on the emulator shows the same row, off.
- 00767ddb — flaky test fix (section 4).

Native (`main-native`):
- 5b6231f — shared packages mirrored from 00767ddb (the 8 preference files) and the lock names 00767ddb.

No guard, bound, restriction matrix or security test was weakened. Stale tests updated: the read-path golden (intended API additions) and the stored-settings consumer list (new settings with real consumers).

## 4. The flaky test's root cause

`TestPlaybackV1QueueQueryCapturesBeyond500AndKeepsItsSnapshot` meant to hold background queue builds, change the catalogue while the capture is retained, then resume and check the queue kept its captured membership. The hold never held:

1. The lazy "resume after a restart" is a `sync.Once` spent only by the first queue request (`resumeOnce`). The test called `ResumeQueueBuilds()` directly, believing that spent it; it does not.
2. So the test's `POST /v1/queues` spent the Once and started `ResumeQueueBuilds` in a goroutine, which cleared `PauseQueueBuilds` and kicked the builder while the request was still answering.
3. The builder then built all 520 keys from the retained capture. When that finished before the create read its own reply, the reply said `Total:520, State:ready` and the test failed ("first batch"). Under load the request is slower relative to the build, hence "fails under load". The 10 ms sleep only gave the first kick's builder time to go idle; that builder also ignored the pause between steps.
4. Proof, scratch probe (not committed): with the old code, 500 ms after the create the pause was false and the queue was fully built (3/3 runs) — before the test's catalogue changes, so the test never exercised them. With the fix the queue was still 256 keys, building, 500 ms later.

It is not a production race: production never pauses builds, and a create whose reply already shows more than its first batch is correct behaviour. Capture itself was sound (the capture is registered before the snapshot is committed as building).

Fix (00767ddb): `HoldQueueBuilds()` spends the lazy resume and sets the pause under the builder's lock; `ResumeQueueBuilds()` also spends it; the builder checks the pause before each snapshot or save step. The test uses `HoldQueueBuilds()` (no sleep), and so do the five other tests that set the pause after requests had started (their unlocked writes raced a running builder). Results: fixed code 20/20 under `-race` while the host ran the unfixed copy, a full suite, Gradle and the emulator, and it passed in both later full-suite runs; the unfixed code failed 2/20 under the same conditions with the original symptom.

Observation: query captures share two process-wide slots (`queryCaptureSlots`) held up to 30 minutes. A test that holds a query build and never resumes it pins one slot for the rest of its process; the third such create answers 503 `queue_building` (seen with the probe). The real tests all resume.

## 5. End to end and new-screen coverage

- Server driver and browser driver as in final-pass.md (production server via `make build-server` into scratch, production web bundle). Not exercised, as before: PGS generation (no encoder), canonical people grouping (local NFO only).
- New screens checked once (scratch tool reusing the browser run's disposable state, desktop and phone): Settings › About › Open-source licenses (list renders); Settings › Playback (new row present, off); Saved › Favorites, Playlists, Collections, Saved views, History (sort/filter toolbars render); Home row pages Continue Watching, Continue Listening, Recommended, More recommendations, Trending now (toolbar with "Row order"); a playlist page with two films ("Playlist order", Play, Shuffle, filters). All rendered with no exception or failed request, so the committed browser driver was not extended.
- Coverage gaps (not walked, report only): a person page with All/Cast/Crew (the generated media has no person identities, so no person page exists to open); the Move to… and merge sheets; Administrator-role screens (needs a second account with that role); licence detail page; Picture in Picture, Forced only and the credits countdown itself (need real credits markers and a browser with PiP); collection Undo and playlist Remove/Undo; letter-rail refresh re-anchoring. The routine screen walk still visits only the routes it had before.

## 6. Android

- Engine output existed (`_program/scratch/own-mpv/out/android`), not rebuilt.
- Gradle used a private copy of `apps/native/node_modules` in place of the symlink to the main checkout (as own-mpv did); the symlink is restored and the Gradle build folders (`.gradle`, `build`, `.cxx`) deleted. Nothing written to Justin's main checkout.
- Emulator `Portico_Phone_API36` (Android 16 x86_64) started headless with `-read-only -no-snapshot`. The debug app (Metro on 19424, `adb reverse` for Metro and the server) first showed a stale session for `demo.getportico.tv` with a Sign in button stuck busy; after clearing the app's data it offered "Sign in directly to a server". Signed in to a local server holding the browser run's generated media and generated account, chose the main profile, set Quality & data › Player = mpv (the row was offered, so the mpv libraries loaded), resumed "Browser Film": it played through mpv (frame counter 14.25 s → 26.9 s in two screenshots; logcat `PorticoMpv` lines and audio focus taken; mpv first tried `h264_mediacodec` without a surface and played anyway). App uninstalled, emulator, Metro and server stopped.
- Not done: release build on the emulator, Fire TV flavour, TV emulator, and the other ABIs (brief asked for x86_64 only).

## 7. Not run, and why

- Device runs other than the Android emulator: iOS, tvOS, Fire TV, Vega hardware and the Vega virtual device (not available on Ubuntu).
- Swift/Objective-C compile and the Apple engine (needs a Mac).
- Scale benchmarks (not in this brief).
- The pre-existing gofmt deviations in `playbackv1/admin.go` were left alone (`queue_build.go`'s one was fixed because the file was edited).

## Clean-up

GOCACHE `_program/scratch/gocache-final-2` deleted. The scratch worktree of a4f86250 (used for the before-fix runs) removed with `git worktree remove`. E2E evidence folders (they hold generated credentials) and binaries deleted after the results were recorded; logs and Android screenshots kept in `_program/scratch/final-pass-2/` (no credentials). No test, server, Chrome, Metro, Gradle or emulator process left running.
