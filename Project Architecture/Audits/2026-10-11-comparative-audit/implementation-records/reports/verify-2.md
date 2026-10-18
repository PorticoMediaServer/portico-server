# verify-2

Host ehler-personal (mini-1), 16 Oct 2026 11:33–13:40 (ADT). Worked only in `server-verify-2` (from main-server fd7814ed) and `native-verify-2` (from main-native 6aeb48c), branch `audit-implementation/verify-2`; committed with Justin's identity, never pushed. After the coordinator's message, merged `audit-implementation/main` (server 78637f57 with cp5-reads, native 0075018) into both and re-ran the affected parts. No subagents or helpers. `GOFLAGS=-p=4`, `GOCACHE=_program/scratch/gocache-verify-2` (deleted), `nice -n 10`. Free disk never below 69 GB.

## 1. Outcome

- **The merged state passes after five real fixes and three stale tests/drivers**, including the cp5-reads merge. Final state (server 16dd585b + ea57a234, native 5eb5ff9):
  - Server: `go test ./...` — every package passes (84 ok in the post-merge full run plus the three packages that failed there under load, all ok on rerun: httpapi, playback, playbackv1; 16 without tests); `go vet` only the 63 known vendored SQLite warnings, `scripts/vet-go.sh` clean; `gofmt -l` empty; apikit tests and vet; `go run ./cmd/apigen` no diff; Windows/macOS cross-builds; production web build; client-core 2,024/2,024; web `npm run check` 589/589; cast 21/21; contracts 30/30 and build; web and native licence lists current; schema in sync.
  - Native: shared packages identical (mirrored at 5eb5ff9); both type-checks, `lint:hooks`, `lint:strings`; `npm test` 701/701; Vega type-checks, string lint, 88/88.
  - Kotlin: `:portico-mpv:assembleRelease` and `:app:assembleGoogleDebug -PreactNativeArchitectures=x86_64` both built from the **new all-ABI engine**; JVM unit tests 17/17 Google and 17/17 Fire TV.
  - Media engine: `build-android.sh` for **all four ABIs with the libass patch** (26 min), `verify.py android` **5,026 checks, 0 failed**; the patch string is in `libass.so` for arm64-v8a, armeabi-v7a, x86 and x86_64. Output kept at `_program/scratch/verify-2/engine-out/android` (242 MB) for the final run.
  - End to end (post-merge binary and bundle): server driver **18/18 groups**; browser driver **13/13 groups, 114/114 screens**; the 15–16 Oct screen walk **33 observations, 32 rendered/pass, 1 "review"** (a transient 404, section 6).
- **Android emulator:** phone setup of a fresh server works end to end; a film played through mpv; an **Arabic + Latin ASS line draws correctly with mpv** (both scripts, no boxes); the MP3 audiobook resumed after a cold restart; credits Up Next with the setting off shows the card and starts nothing early; the renamed `…Ms` settings save under the new keys and take effect.
- **One new real bug I did not fix (section 9):** with **Player = mpv chosen explicitly** and an external subtitle track picked on the details page before Play, playback stops at 0:00 ("Playback could not continue"). The same title and track play under Automatic (which also uses mpv). It needs a native-player investigation; exact repro below.

## 2. Command table

Logs: `_program/scratch/verify-2/logs/` (no credentials).

| Command | First result | Cause | Fix | Rerun |
|---|---|---|---|---|
| `npm ci` (server worktree, own `node_modules`) | 0 vulnerabilities | | | |
| helper build | exit 0 | | rebuilt after each server change | exit 0 |
| `gofmt -l` | 4 files (`catalog/library_pins.go`, `catalog/playlist_content.go`, `mediaanalysis/produce.go`, `social/privacy.go`) | merged lanes | formatted; import groups fixed by hand in `library_pins.go` (4ee553d5) | empty, also after the cp5-reads merge |
| `go vet ./...` / `scripts/vet-go.sh` | 63 × vendored `unsafe.Pointer` / exit 0 | known | none | same after merge |
| apikit `go test` / `go vet` | exit 0 / 0 | | | 0 / 0 after the apikit change |
| `go run ./cmd/apigen` | no diff | | | no diff after apikit change and after merge |
| cross-builds windows/darwin × amd64/arm64 | all 0 | | | windows/amd64 and darwin/arm64 re-run after merge: 0 |
| `go test -timeout 30m ./...` run 1 (9 min 50 s, load 4–12) | 3 packages failed | below | | |
| — `httpapi.TestChannelMutationWaitCancelsWithoutReadingBody/library` | 400 `invalid_library_channel`, "avatar body read before permission check" | **real regression** from 950196a6 (merged-title id replacement): apikit's `MountHTTP` read and rewrote the whole body **before** the handler ran, so a request waiting for a Library Channels mutation slot, or one about to be refused, had its body read anyway | body replacement is now lazy: read and rewritten on the handler's first read (071f5225); new apikit test `TestRefsLeaveAnUnreadHTTPBodyUnread` (fails on the old code) | httpapi ok; apikit ok |
| — `mediaexec.TestEveryProcessSpawnIsMediaexecOrListed` | `storage/inventory_batch.go:inventoryBatchRequest` unlisted | cp3-bulkbuild's batched storage helper | listed with its reason (server binary, lists folders, opens no media) — same as `inventoryRequest` (4a7e7d76) | ok |
| — `httpapi.TestReadPathGoldenEquivalence` | 52 cases differ | classified: **only** `revision.catalog` (632→712 on library cases, 919→1031 on Home rows); cp2-reads' own branch already failed this; counting genre sources on entities bumps the library revision | re-recorded (9cbe8e9a) | ok |
| `go test ./...` run 2 | **exit 0** (87 ok, 16 without tests) | | | |
| client-core / cast / contracts | 2,024/2,024 · 21/21 · 30/30 + build | | | same after merge |
| web `npm run check` | 588/589: `browse-collection-dispose.test.ts` expected a dependency array | d8cbc881 made a library list's freshness dependencies a function of the titles it holds now; the test stubbed the old helper | test uses the real client-core rules and checks both cases (titles held now; a personal-state filter follows all personal changes) (3d65152a) | 589/589, and after merge |
| production web build | exit 0 (chunk-size warning) | | | exit 0 after merge |
| licences web / native (`MEDIA_ENGINE_OUT_DIR` own-mpv and the new engine) | up to date / exit 0 / exit 0 | | | |
| native shared `--check`, typechecks, lints, `npm test`; Vega | identical; all 0; 701/701; 88/88 | | | after merge: mirror needed only for my client-core fix (5eb5ff9), then identical; all 0; 701/701; 88/88 |
| `build-android.sh` (all ABIs) + `verify.py android` | exit 0; 5,026/0 | | | |
| `:portico-mpv:assembleRelease -PporticoLibmpvDir=…/engine-out/android` | BUILD SUCCESSFUL 34 s | | | |
| `:app:assembleGoogleDebug -PreactNativeArchitectures=x86_64` (same engine) | BUILD SUCCESSFUL 2 min | | | |
| `:app:testGoogleDebugUnitTest :app:testFiretvDebugUnitTest` | 17/17, 17/17 | | | |
| server driver | 10/18, then "GET /v1/events returned 400: waitSeconds unknown" | stale driver: cp4-ms renamed the query to `waitMs` | `?waitMs=0` (9d7abbd5) | 18/18 (browse median 14.1 ms, search 18.9 ms); after merge 18/18 (14.3 / 20.0 ms) |
| browser driver | 13/13, 114/114 | | | after merge 13/13, 114/114 |
| screen walk (`tools/newscreens-v2.mjs`) | my tool raced the scan (album ids, person search) | tool, not product (but see section 9: provisional album ids) | waits for the scan to settle | 33 observations, 32 ok, 1 transient 404 (twice, also after merge) |
| Android emulator checks | section 5 | | analysis 500 fixed (below) | |
| marker POST with an unknown field (found on the emulator set-up) | **500 internal** ("Invalid lyrics …") | **real bug**: analysis routes decode with the lyrics routes' strict decoder; `analysisFailure` did not know `lyrics.ErrInput` | mapped to 400 `invalid_analysis`; case added to `TestWrappedDomainErrorsNeverExposeCauses` (fails on the old code) (0b8ae6c2) | ok |
| Version menu label (seen on the emulator) | "720p · MKV · 720p · MKV", "SD · MP4 · 360p · MP4" | **real bug** from aed6a4ff: client-core appended height and container to a server label that already holds them | a fact the label states is not repeated; height before container ("720p · MKV", "SD · 360p · MP4"); test case added (2b2ec9c3, mirrored 5eb5ff9) | client-core, native ok |
| **after merging cp5-reads:** `go test -timeout 30m ./...` (8 min 36 s) | 3 packages failed: httpapi (golden), playback (`TestHLSThrottleStopsConversionAheadOfTheViewer`), playbackv1 (`TestGrowingQueryShuffleKeepsTheSameSeededOrder`, then `TestQueryCaptureConnectionAdmissionKeepsTheRequestDeadline`) | golden: classified, **only** `revision.catalog` again (712→872, Home rows 1031→1255; cp5-reads' counters). playback/playbackv1: load timing (see section 9) | golden re-recorded (ea57a234) | httpapi, playback, playbackv1 together: all ok; playback throttle test 3/3 alone; the two playbackv1 tests 8/8 under a 12-core CPU load |
| catalog / compactcatalog / persistence / ingestion / scalebench after merge | ok (348 s / 79 s / 68 s / 49 s / 55 s) | | | |

## 3. Fixes made

Server (`server-verify-2`):
- 4ee553d5 — gofmt of four merged files.
- 3d65152a — web `browse-collection-dispose.test.ts` follows the title-scoped freshness dependencies (stale test; reason above).
- 071f5225 — **apikit:** id replacement in HTTP bodies (`rewriteHTTPBody`) wraps the body and does its read on the handler's first `Read`. Restores the guarantee that a refused or queued request's body is not read (the Library Channels mutation gate and the avatar admission principle). Content-Length stays the client's. New test.
- 4a7e7d76 — spawn inventory lists the batched inventory helper (guard unchanged; same category as existing entries).
- 9cbe8e9a, ea57a234 — read-path golden re-recorded after classifying every differing case (only the catalogue revision counter).
- 9d7abbd5 — server driver uses `waitMs`.
- 0b8ae6c2 — malformed analysis requests answer 400 `invalid_analysis`, not 500.
- 2b2ec9c3 — Version menus don't repeat the resolution and container (phone, tablet, TV, Vega use `mediaVersionNames`).
- 16dd585b — merge of `audit-implementation/main` (cp5-reads).

Native (`native-verify-2`): fast-forward to 0075018, then 5eb5ff9 (mirror of the client-core fix).

No guard, bound, restriction matrix or security test was weakened.

## 4. Decisions I made

- Made the apikit body rewrite lazy rather than moving it into each handler: one place, all HTTP routes keep "check, wait, then read".
- Treated the playback/playbackv1 failures in the post-merge full run as load timing (all pass on rerun, including under CPU load), not as code faults; analysis in section 9.
- Kept the all-ABI engine output for the final run (242 MB); deleted its 1.8 GB work folder.
- Did not chase the explicit-mpv + preselected-subtitle failure into the native player (see 9); it needs focused native work, and the brief's other checks were still open.

## 5. Android (emulator)

Own emulator (`Portico_Phone_API36`, `-port 5600 -read-only -no-snapshot`, headless), own Metro on 19434 (app pointed at it with `debug_http_host`, as verify-1 described), APK from this worktree built on the new all-ABI engine. No leftover emulator or Metro from earlier runs was running.

- **Phone server setup (fresh server, web parity):** "Sign in directly to a server" → typing the fresh server's address offered "Nobody has set up this server yet… Set up this server" → setup screen (server name, owner; no Portico Account choice because this server has no Hosted configured — same as the web) → "Create owner" signed straight in as owner, Home showed the server name; the server reported `setupRequired:false`. Leaving setup with Back discards the draft (expected).
- **Film with Player = mpv:** Browser Film played through mpv (frame counter 10.2 s → end; `PorticoMpv` log, audio focus).
- **Arabic + Latin ASS with mpv:** the line "مرحبا Sarah، أهلا بك في Portico" drew both scripts correctly (screens `arabic-2.png`, `fast-sub-exo.png`). Under Player = Automatic with the track picked, it played to the end with the subtitle. The `fontselect: Using a loaded font` line is at info level, which the app doesn't log; the patch is confirmed in every ABI's `libass.so`. **But see section 9 for Player = mpv chosen explicitly.**
- **MP3 audiobook resume:** played, sought to 5:35, paused, force-stopped, relaunched: Home offered "Resume · 4m left"; resume played on from 5:35 (5:41, then 5:47 six seconds later).
- **Credits Up Next, setting off (default):** episode with an owner-approved credits marker 30–45 s: card at ~32.5 s "Up next · Second Episode · Starts in 0:28" (rest of the title + my 15 s countdown), counting down 0:25, 0:21; nothing started early; the episode ended at 0:45 and the end card counted "Playing in 11s".
- **Renamed `…Ms` settings:** Skip back 30 s, Skip forward 15 s, Up Next countdown 15 s, Crossfade 4 s saved as `playback.skipBackMs 30000`, `playback.skipForwardMs 15000`, `playback.upNextCountdownMs 15000`, `music.crossfadeMs 4000` (read back from `GET /v1/preferences?deviceClass=mobile`). The player's buttons showed 30/15 and the end card used the 15 s countdown. Crossfade's effect on audio was not listened to.
- **Settings › Servers:** renders ("Connected to", "Signed in directly"). "Certificates you trusted" never appeared: an HTTPS sign-in to the loopback server answered "Portico couldn't check this server's certificate", so there was no certificate to trust. Needs a device check against a self-signed server on a real LAN address.
- App uninstalled; emulator, Metro and servers stopped; Gradle outputs deleted; `node_modules` symlink restored.

## 6. Web screens (desktop 1440 × 1000 and phone 390 × 844)

Rendered with no exception or unexpected failed request: Dashboard, Integrations, People › Devices, Open-source licenses, Secure connection, Troubleshooting (skipped parts), Saved › Playlists (Name · A–Z, Alpha before Beta), a playlist, Continue Watching row, **music and audiobook Needs attention** tabs, **local person page** (NFO-only actor: All/Cast/Crew, "Cast Film as The Lead"), **Not this artist…** (menu present on both widths; performed on desktop: a new same-named artist page opens with the album, and the library now has two "Browser Artist"), collection Reorder (server order matched), rail pin editor.
- The library page asks `GET /v1/libraries/{id}/metadata/screen` for music/audiobooks and gets the documented 404 ("no lookup policy"); the client treats it as none. It is console noise only; my walk ignores it.
- **Transient 404:** right after "Not this artist", the new artist page's first `GET /v1/containers/artist/{id}/personal-state` answers 404 (`VisibleEntity` doesn't see the new artist until derivation catches up); a moment later it answers 200. The page itself renders. Seen twice, also after the merge.
- Certificate list and phone setup are native-only screens (section 5).

## 7. API and schema changes

- No route, field or schema change. Behaviour: HTTP bodies are read for id replacement only when the handler reads them; malformed analysis requests answer 400 instead of 500.

## 8. Device test notes

- Re-check **Player = mpv + a subtitle picked on the details page** on a real phone after it is fixed (section 9).
- Certificates you trusted (Settings › Servers) with a self-signed server on the LAN.
- Arabic/Hindi/Thai lines with mpv on a real arm64 phone (engine is ready in `engine-out`).
- Not run here: iOS, tvOS, Fire TV, Vega device or simulator, Apple engine build, release APK, TV emulator.

## 9. Not finished, and observations

- **Bug, not fixed — explicit mpv + preselected subtitle.** Repro: Settings › Quality & data › Player = mpv; open a film that has an external `.ass` (here a single-file MP4 + `.ar.ass`); on the details page set Subtitles to the track; Play. Result: "Playback stopped — Playback could not continue", at 0:00. Logcat: `ffmpeg/demuxer: moov atom not found`, `Seek failed (to 0, size 111)`, then repeated `http: HTTP error 404 Relay`, `cplayer: Failed to recognize file format`; JS warns `[playback] start timed out … "intent":"paused" … "nativeListening":false`. Same film with subtitles Off under explicit mpv after this failure also failed until the app was restarted; with Player = Automatic the same film and track play (mpv engine, subtitle drawn). The server answers the session's media and subtitle URLs correctly with ranges (probed). Likely area: the explicit-engine start path binding the relay (`PorticoMpvRelay.bind`) to a URL other than the one mpv opens, or a stale session after a re-plan. MP4 moov placement is not the cause (a moov-at-end file plays).
- **Load-timing tests (passed on rerun):** `playbackv1.TestGrowingQueryShuffleKeepsTheSameSeededOrder` got an empty first batch: the request's 2 s scan budget (`scanBudgetTime`) can expire before a shuffled first batch under heavy I/O; because it then calls `t.Fatal` while holding builds, its capture keeps one of the two process-wide `queryCaptureSlots`, and `TestQueryCaptureConnectionAdmissionKeepsTheRequestDeadline` (which asserts no slot is held) fails after it. A `t.Cleanup` that releases captures would stop the cascade; a test-only time budget would remove the flake. `playback.TestHLSThrottleStopsConversionAheadOfTheViewer` ("did not resume on demand: still being prepared") also passed 3/3 alone. Product note: under load a shuffled queue can start with an empty first batch and nothing current until the background build adds entries.
- **Provisional ids during a first scan (pre-existing, confirmed on a pre-cp3-bulkbuild build):** a new album first appears under "Unknown artist" with one album id, and ~1–2 s later under its real artist with a different id; the first id then answers 404 (not a merged-away alias). Same for a new audiobook. A page opened in that window breaks.
- **Catalogue revision growth:** two read-path changes (genre-source counts, cp5-reads counters) raised the fixture's catalogue revision from 632 to 872 (Home 919 → 1255). Revisions are counters; the cost is extra revision bumps while a library is built.
- Not done: release APK, Fire TV/TV emulator, Apple, Vega devices.

## 10. For cp5-reads-style changes (browse/search only), rerun

1. `go test -timeout 30m ./internal/catalog/... ./internal/compactcatalog/... ./internal/httpapi/... ./internal/persistence/... ./internal/ingestion/...` (helper built first; `PORTICO_PLAYBACK_TEST_HELPER` set).
2. `TestReadPathGoldenEquivalence`: classify every differing case before `PORTICO_GOLDEN_UPDATE=1` (a revision-only difference is expected when derived counters change).
3. `python3 scripts/schema-sync.py` (no diff expected), `go run ./cmd/apigen` (no diff), `gofmt -l`.
4. `npm run test:core`, `npm run check --workspace portico-web`, native `npm run typecheck`, `typecheck:test`, `npm test`, Vega `npm run typecheck`, `npm test`; `shared-packages.mjs --check`.
5. `make build-server` + production web build, then `scripts/e2e-server.py` and `web/test/integration/browser-smoke.mjs`.

## Commits

- portico-server, `audit-implementation/verify-2`: 4ee553d5, 3d65152a, 071f5225, 4a7e7d76, 9cbe8e9a, 9d7abbd5, 0b8ae6c2, 2b2ec9c3, 16dd585b (merge of main 78637f57), ea57a234.
- portico-react-native, `audit-implementation/verify-2`: fast-forward to 0075018, then 5eb5ff9.

## Clean-up

GOCACHE deleted; engine work folder, e2e evidence folders (generated credentials), fresh-server state, the fresh owner's credential file, server binaries and helper deleted. Kept in `_program/scratch/verify-2/`: logs, tools (`newscreens-v2.mjs`, probes, Android helpers), Android screenshots (no passwords; typed passwords are masked) and `engine-out/android`. The server worktree keeps its own `node_modules` (`npm ci`); the native worktree's `node_modules` are symlinks to main-native again. No server, Chrome, Metro, Gradle or emulator process of this lane is running.
