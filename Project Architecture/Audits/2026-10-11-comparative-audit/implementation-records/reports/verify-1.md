# verify-1

Host ehler-personal (mini-1), 16 Oct 2026 09:15–11:20 (ADT). Worked only in `server-verify-1` (from main-server 617dc62d) and `native-verify-1` (from main-native a8c51ea), branch `audit-implementation/verify-1`; committed with Justin's identity, never pushed. No subagents or helpers. `GOFLAGS=-p=4`, `GOCACHE=_program/scratch/gocache-verify-1` (deleted at the end), `nice -n 10`. Free disk never below 73 GB. The host was shared with other lanes the whole time (cp3-bulkbuild's two 200k scalebench runs, other lanes' Go tests, an emulator and Metro left from 15 Oct); load average 8–25.

## 1. Outcome

- **The merged state passes after three real fixes and two stale tests.** Nothing in the product was broken by the twelve 15 Oct merges except one goroutine started outside `supervise.Go` (a guard failure). The other failures were a web test importing a removed export, a native test that predated two merged features, and a backup timing test that measured a fresh database's first write instead of the backup.
- Server: `go test ./...` **exit 0: 87 packages ok, 16 without tests**; `go vet` only the 63 known vendored SQLite warnings and `scripts/vet-go.sh` clean; apikit tests and vet clean; `go run ./cmd/apigen` no diff; Windows and macOS (amd64, arm64) cross-builds; production web build; client-core 2,018/2,018; web `npm run check` 588/588; cast 21/21; contracts 30/30 and build; web and native licence lists current.
- Native: shared packages identical; both type-checks, `lint:hooks`, `lint:strings` clean; `npm test` 697/697; Vega type-checks, string lint and 88/88.
- Kotlin: `:portico-mpv:assembleRelease` and `:app:assembleGoogleDebug -PreactNativeArchitectures=x86_64` build; **the new JVM unit tests run through Gradle for the first time: 15/15 Google and 15/15 Fire TV** (`:app:testGoogleDebugUnitTest :app:testFiretvDebugUnitTest`; JUnit resolved online).
- End to end: server driver 18/18 groups; browser driver 13/13 groups and 114/114 screens; the 15 Oct screens walked once at desktop and phone width, 23/23 observations rendered or passed (owner tools, devices, licences, Secure connection, skipped parts, playlist list sorting, a playlist, a Home row, collection Reorder with one move checked through the API, the rail's pin editor).
- Android emulator (own instance, Android 16 x86_64): signed in to a local server, **a film played through mpv**, **an MP3 audiobook resumed mid-file** (paused at 5:44, cold app restart, Resume started at 5:46 and advanced), and **with "Start the next episode during the credits" off (the default) the credits Up Next card showed at 33 s of a 45 s episode with "Starts in 0:22", counted down, and nothing started before the title ended**; the end card then ran its own countdown.

## 2. Command table

Logs: `_program/scratch/verify-1/logs/` (no credentials).

| Command | First result | Cause | Fix | Rerun |
|---|---|---|---|---|
| `npm ci` (server worktree root, own `node_modules`) | 197 packages, 4 s | | | |
| helper build `go build -o …/playback-test-helper ./cmd/server` | exit 0 | | rebuilt after the server fix | exit 0 |
| `go vet ./...` (server) | exit 1, 63 × `possible misuse of unsafe.Pointer`, all `internal/thirdparty/sqlite` | known vendored SQLite | none | `scripts/vet-go.sh` exit 0 |
| `go test ./...` (server) run 1, 14 min 56 s, load 15–25 | 3 packages failed | below | | |
| — `supervise.TestEveryGoroutineIsStartedThroughThisPackage` | bare `go func()` at `internal/mediaanalysis/shared_audio.go:247` | the shared audio pass (31d3809d merge, "shared-decode Complete scans") starts its waveform/fingerprint readers with `go` | `supervise.Go`, and the stage closes its pipe on any exit so the decode loop cannot block; a panic becomes the stage's error instead of a missing summary (c18a45eb) | supervise and mediaanalysis ok |
| — `catalog` | `panic: test timed out after 10m0s` (package total, the running test was 1 s in) | host load: other lanes' scalebench and Go tests; the package took 286–356 s in earlier passes | none (not a code fault) | 460 s, 500 s and 344 s in later runs, all ok |
| — `backup.TestBackupDuringConcurrentWrites` | `writer waited 281ms during backup` (bound 250 ms); again in run 2 (270 ms) | test measured the first gated write on a fresh database: the probe row was seeded with plain `db.Exec`, so the writer loop's first `dbwork.ExecWrite` paid the cold write path (50–150 ms quiet, ~300 ms loaded) **with or without a backup**. Probe (scratch, removed): cold first write 49–79 ms without a backup, 81–213 ms with; warm, every write during a backup ≤ 3 ms | seed the row through the write gate first; bound unchanged (01ec7770) | under the same load: unfixed 1/20 failed (299 ms), fixed 60/60 pass |
| `go test ./...` run 2 (after the supervise fix), 8 min 55 s | only the backup timing failure above | | | |
| `go test ./...` run 3 (all fixes) | exit 0, 87 ok, 16 without tests, 1 m 26 s (82 packages reused run 2's cached results with identical inputs; backup, dbwork, mediaexec, persistence and supervise re-ran) | | | |
| apikit `go test ./...` / `go vet ./...` | exit 0 / exit 0 | | | |
| `go run ./cmd/apigen` | no diff | | | |
| `CGO_ENABLED=0 go build ./...` windows/darwin × amd64/arm64 | all exit 0 | | | |
| `npm run test:core` | 2,018/2,018 | | | |
| `npm run test:cast` | 21/21 | | | |
| `packages/contracts` `npm test` / `npm run build` | 30/30 / exit 0 | | | |
| `npm run check --workspace portico-web` | 587/588: `feedback-label.test.ts` "feedbackCategoryLabel is not a function" | 9656affd moved owner words to the catalogue and removed the English-only `feedbackCategoryLabel` re-export from `Feedback.tsx`; the test still imported it | test now checks the shared `feedbackCategoryName` the screen uses (known categories, an unknown leaf shown as-is, empty → Other) (8574d256) | 588/588 |
| production web build `--configLoader runner` | exit 0 (chunk-size warning only) | | | |
| licences: `web/scripts/licences.ts --check`; `apps/native/scripts/licences.mts --check` (`MEDIA_ENGINE_OUT_DIR=scratch/own-mpv/out`) | up to date / exit 0 | | | |
| native `shared-packages.mjs --check` | identical | | | |
| native `typecheck`, `typecheck:test`, `lint:hooks`, `lint:strings` | all exit 0 | | | |
| native `npm test` | 695/697: `owner-structure.test.ts` | Troubleshooting gained the skipped-parts section before Export, and phones (and administrators) now draw everyone's devices (`server.devices`, 7b154e7/9656affd), which the test still excluded | test follows the shared structure; the drawn-panels check no longer exempts `server.devices` (it is drawn: `AccountDevices` without an account) (0fc93ad) | 697/697 |
| Vega `typecheck`, `typecheck:test`, `lint:strings`, `npm test` | exit 0, 88/88 | | | |
| `:portico-mpv:assembleRelease -PporticoLibmpvDir=scratch/own-mpv/out/android` | BUILD SUCCESSFUL 1 m 32 s | | | |
| `:app:assembleGoogleDebug -PreactNativeArchitectures=x86_64` | failed: "Could not find project.android.packageName" | my private `node_modules` copy had excluded the React Native CLI's own `build/` (JavaScript) folders | copied them in (my set-up, not the repo) | BUILD SUCCESSFUL 4 m 17 s |
| `:app:testGoogleDebugUnitTest :app:testFiretvDebugUnitTest` | BUILD SUCCESSFUL; 15/15 and 15/15 | | | |
| server driver `scripts/e2e-server.py` | 18/18 groups, 1 m 41 s; browse n=324 median 15.7 ms / max 70 ms; search n=238 median 23.2 ms / max 175 ms (loaded host) | | | |
| browser driver `browser-smoke.mjs` | 13/13 groups, 114/114 screens, 4 m 46 s | | | |
| 15 Oct screen walk (scratch `tools/newscreens-v1.mjs`) | 23/23 after fixing my own script's playlist set-up (wrong create route) | | | |
| Android emulator checks | pass (section 5) | | | |

## 3. Fixes made

Server (`server-verify-1`):
- 8574d256 — web `feedback-label.test.ts` tests the shared `feedbackCategoryName` (stale import; reason above).
- c18a45eb — `mediaanalysis/shared_audio.go`: the waveform and fingerprint readers start through `supervise.Go("mediaanalysis.sharedAudio.<stage>")`; the pipe reader is closed in a `defer`, and a panic leaves `errSharedStagePanicked` as the stage's error. Goroutine guard unchanged.
- 01ec7770 — `backup_test.go`: the probe row is written through `dbwork.ExecWrite` before the writer loop, so the 250 ms bound measures waits during the backup. Bound unchanged.

Native (`native-verify-1`):
- 0fc93ad — `owner-structure.test.ts` follows the merged settings structure (skipped parts; administrators' and phones' device list).

No guard, bound, restriction matrix or security test was weakened. Stale tests updated: the two above, each for a merged, intended change.

## 4. Decisions I made

- Ran `npm ci` into the server worktree's own `node_modules` (as the brief asks) rather than linking main-server's, so the check used exactly the lockfile's packages including oxlint.
- The catalog package timeout was treated as host load, not a fault: it passed in three later runs (344–500 s) and CI runs with `-timeout=120m`. It is close to the 10-minute default on a busy host; noted under section 7.
- Fixed the backup test rather than calling it a flake: measured that the failing wait was always the first write and that a warm writer waits ≤ 3 ms during a backup, so the test now measures what it claims with the same bound.
- Android: started my own emulator instance (`-port 5600 -read-only`) because the one on 5590 (started 15 Oct 22:35) had the app in the foreground and might belong to another lane. Left it and the old Metro running.

## 5. Android (emulator)

- APK from this worktree (`assembleGoogleDebug`, x86_64, engine from `scratch/own-mpv/out/android`), local server (`make build-server` binary) on the browser run's disposable state plus a generated 10-minute VBR MP3 book. The AVD again carried a stale `demo.getportico.tv` session; cleared the app's data, then "Sign in directly to a server" with the run's generated owner.
- **Important for the next runner:** the debug app on an emulator reaches Metro at the host's port 19424 directly (10.0.2.2), so `adb reverse` does not redirect it. My first pass of checks was therefore served by the **leftover Metro from `main-native` (pid 662952, running since 15 Oct 22:35, mid-merge)**, and on that bundle the credits card did not appear. I pointed the app at this worktree's Metro (dev setting `debug_http_host=localhost:19434` in `shared_prefs/tv.getportico.portico_preferences.xml`, `adb reverse tcp:19434 tcp:19434`) and repeated all three checks on the current code. Whoever owns that Metro should stop it.
- Settings › Playback: "Play the next episode automatically" on, "Start the next episode during the credits" **off** (read from the switches' `checked` state). Quality & data › Player offered and accepted **mpv**.
- Film: Browser Film played through mpv (frame counter 9.1 s → 21.7 s on the first bundle; on the current bundle resumed at 0:35 and played to the end card; `PorticoMpv` log lines, audio focus taken). mpv tries `h264_mediacodec` without a surface first and plays anyway (as in final-pass-2).
- MP3 audiobook: played, skipped to 5:44, paused, app force-stopped and relaunched; Home offered "Resume · 4m left"; Resume started at 5:46 and advanced (5:52 after six seconds). Repeated on the current bundle from 6:35 → 6:41.
- Credits Up Next (setting off): Browser Show S1E1 (45 s) given an owner-approved credits marker 30–45 s through `POST /v1/items/{id}/analysis/markers` (the same marker reached the web player: card shown at 34.8 s). On the phone: no card at 29.7 s; at 33.8 s "Up next · Second Episode · Starts in 0:22" with Play now / Watch credits; 0:18 at 37.8 s, 0:14 at 41.6 s; the episode ran to 0:44, then the end card "Playing in 9s". Nothing started early.
- Observed, not fixed: every cold start logs a development warning `[preferences] load failed: Network request failed` (the provider retries after 5 s and the settings then load); probably the emulator's network not being ready at launch. The playback "start timed out" warning appeared once on the old bundle only.
- App uninstalled, my emulator, Metro and server stopped; Gradle outputs deleted and the `node_modules` symlink restored.

## 6. Coverage of the 15 Oct screens (web)

Rendered at 1440 × 1000 and 390 × 844 with no exception or failed request: Settings › Dashboard, Integrations, People › Devices (owner device list), About › Open-source licenses, Remote access (Secure connection), Troubleshooting (the "Parts this app could not read" panel), Saved › Playlists (sort control "Name · A–Z", Alpha before Beta), a playlist page (Playlist order, filters), Continue Watching row page (Row order). Reorder on a personal collection moved the last title to the top at both widths and the server's order matched. The rail's pin editor opened and pinning/unpinning the Films library worked (desktop; phones have no rail).

Not walked: Administrator-role screens (needs a second account with that role), Fire TV/TV focus, Vega device.

## 7. Not finished, and notes for the final run

- Not run (no hardware here): iOS, tvOS, Fire TV, Vega hardware or virtual device, Apple engine builds, release APK on the emulator, TV emulator.
- `internal/catalog` now takes 344–500 s on this host under other lanes' load (286–356 s in earlier passes); with four more lanes merging it may cross Go's default 10-minute package timeout during a busy full run. CI uses `-timeout=120m`; locally run with `-timeout 30m` if the host is shared.
- gofmt deviations from merged lanes (not touched, no test checks them): `internal/catalog/library_pins.go`, `internal/catalog/playlist_content.go`, `internal/mediaanalysis/produce.go`, `internal/social/privacy.go`.
- Leftover processes from before this lane (not mine, left running): emulator `emulator-5590` with the app in the foreground, Metro pid 662952 in `main-native`.

## Commits

- portico-server, `audit-implementation/verify-1`: 8574d256, c18a45eb, 01ec7770 (on 617dc62d).
- portico-react-native, `audit-implementation/verify-1`: 0fc93ad (on a8c51ea).

## Clean-up

GOCACHE `_program/scratch/gocache-verify-1` deleted. E2E evidence folders (they hold generated credentials), server binaries and the helper deleted after the results were recorded; logs, the scratch tools, the screen-walk screenshots and results (`screens/`) and Android screenshots kept in `_program/scratch/verify-1/` (checked: no password in them). The server worktree keeps its own `node_modules` (from `npm ci`); the native worktree's `node_modules` are symlinks again. No server, Chrome, Metro, Gradle or emulator process of this lane left running.
