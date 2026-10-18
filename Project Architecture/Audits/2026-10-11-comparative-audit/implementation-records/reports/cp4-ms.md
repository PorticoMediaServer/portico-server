# cp4-ms — seconds to milliseconds on the wire (sweep item A4)

## 1 Outcome

Every duration, position, interval and wait that the server's API still sent or accepted in seconds now travels in whole milliseconds under an `…Ms` name. The generated schemas (`packages/contracts`) contain no `…Seconds` field any more; the one remaining name with "Seconds" in it is `convertedSecondsPerSecond`, a speed ratio, not a duration. Server, client-core, web, phone/tablet/TV and Vega were updated together; type-checks are clean on all four. Stored data keeps its units (database columns, saved prepared-media facts, engine internals); conversion happens at the API boundary, using the existing `apiwire` duration helpers on the server and `wireDurations`/`durationIntent` in client-core, so UI models keep the units they already used.

The work was interrupted at 23:50 on 15 Oct and saved as WIP commits; it was resumed, finished and restructured into proper commits (WIP commits replaced).

## 2 Decisions I made

- **Settings documents are renamed, not wrapped.** Owner/viewer settings whose JSON key is the stored key (console settings, live defaults, live-source overrides, DVR padding, guide refresh draft, viewer preferences) now store and publish milliseconds under the new key. A document saved before this change simply falls back to the default for that one field (readers already ignore unknown keys). Pre-release, no shim.
- **Viewer preference keys renamed:** `playback.skipBackSeconds` → `playback.skipBackMs` (5000/10000/15000/30000), `playback.skipForwardSeconds` → `playback.skipForwardMs` (10000/15000/30000/60000), `playback.upNextCountdownSeconds` → `playback.upNextCountdownMs` (0/5000/10000/15000), `music.crossfadeSeconds` → `music.crossfadeMs` (0–12000). Their i18n keys follow (`pref.label.playback.skipBackMs`, etc.). Saved values of the old keys are dropped (defaults apply).
- **Trickplay interval** must be whole seconds (thumbnails are cut on seconds): a `trickplayIntervalMs` not divisible by 1000 is refused with a field error.
- **HTTP `Retry-After` stays whole seconds** (HTTP's definition). The error body's hint becomes `retryAfterMs`.
- **Not converted, deliberately:**
  - The server↔Hosted heartbeat answer (`heartbeatAfterSeconds`) and Hosted's own copy of the toolkit (`main-internal/hosted-services/apikit/apierror` still emits `retryAfterSeconds` in Hosted error bodies). Hosted is a separately deployed service with no worktree in this lane; changing the server side alone would break check-ins against the running Hosted. Clients read Hosted's retry wait from the header, so nothing client-side depends on that body field. Follow-up for the Hosted repo: rename `retryAfterSeconds` → `retryAfterMs` in its apikit copy and `heartbeatAfterSeconds` → `heartbeatAfterMs`, then change `server/internal/hosted/current_schedule.go` in the same release.
  - Quick Connect (`/v1/quick-connect`: `expiresIn`, `interval`) keeps the RFC 8628 device-flow shape: the same client type (`DeviceAuthorization`) also reads Hosted's device flow, which has the same fields.
  - Microsecond decimal strings (subtitles `…Us`, social/Watch Together `positionUs`) are a different, deliberate precision clock, not seconds.
  - The Cast sender↔receiver message protocol (`client-core/src/cast/protocol.ts`, receiver in portico-internal) is not the server API.
  - `browse.field.durationSeconds` is an i18n label key, not a time value; `scalebench` JSON is a benchmark report; `strideSeconds` is a stored analysis-algorithm parameter.
- Settings rows: the DVR start-early / finish-late number fields now cap at the server's own bounds (30 and 120 minutes) instead of offering 120/240 minutes that the server silently clamped; the retry window's minimum is 5 s, matching the server.

## 3 Audit bugs closed / gaps closed

- **A4 / A-08.41** (time units not unified) → closed for the server API, see section 4. Test: regenerated contracts contain no `…Seconds` field; server golden/contract fixtures re-recorded; client-core, web, native and Vega tests updated to wire-format fixtures.
- Found and fixed in passing: the For-you **"Short picks" row's See all filter** used `durationSeconds`, a field the browse compiler no longer accepts (so See all failed); it now sends `durationMs` with a millisecond operand.
- Found and fixed: the **capabilities limit** `longPollSecondsMax` (unregistered map key) → `longPollMaxMs: 25000`.

## 4 API changes (every field changed)

| Route(s) | Old | New |
|---|---|---|
| every apikit error body | `retryAfterSeconds` | `retryAfterMs` (header unchanged) |
| `GET /v1/events` query | `waitSeconds` (0–25) | `waitMs` (0–25000) |
| `GET /v1/receivers/{id}/inbox/wait` query | `waitSeconds` (0–55) | `waitMs` (0–55000) |
| `GET /v1/capabilities` limits | `longPollSecondsMax: 25` | `longPollMaxMs: 25000` |
| `GET /v1/notifications/capabilities` | `heartbeatSeconds`, `maxWaitSeconds` | `heartbeatMs`, `maxWaitMs` |
| console settings (`GET/…/console/settings`), `GET /v1/admin/transcode/capacity` | `throttleBufferSeconds`, `playedRetentionSeconds` (registry ids too) | `throttleBufferMs` (10000–600000, default 60000), `playedRetentionMs` (0–3600000, default 180000) |
| admin playback session delivery diagnostics | `convertedThroughSeconds`, `playedRetentionSeconds`, `throttleBufferSeconds` | `convertedThroughMs`, `playedRetentionMs`, `throttleBufferMs` |
| library settings (`GET/PUT …/settings`) `navigation` | `trickplayIntervalSeconds`, `videoPreviewSeconds` | `trickplayIntervalMs`, `videoPreviewMs` |
| library sources (list, `POST/PATCH …/sources`, check), scan/analysis policy `trickplay` | `intervalSeconds` | `intervalMs` |
| live settings, live-source configuration (settings + effective) | `streamBufferSeconds`, `retryWindowSeconds` | `streamBufferMs`, `retryWindowMs` (field errors `settings.streamBufferMs` / `settings.retryWindowMs`) |
| remote live source preview/save draft | `refreshSeconds` | `refreshMs` (300000–86400000) |
| `POST /v1/admin/live-sources/discover` | `windowSeconds` (0–10) | `windowMs` (0–10000) |
| admin DVR settings, recording groups | `prePaddingSeconds`, `postPaddingSeconds` | `prePaddingMs`, `postPaddingMs` (field errors `options.prePaddingMs`/`postPaddingMs`) |
| viewer DVR recordings/rules options (all DVR routes) | `beforeSeconds`, `afterSeconds` | `beforeMs`, `afterMs` (0–21600000) |
| `GET /v1/preferences`, `GET /v1/bootstrap` | keys `playback.skipBackSeconds`, `playback.skipForwardSeconds`, `playback.upNextCountdownSeconds`, `music.crossfadeSeconds`; `audioEffects.crossfadeSeconds` | `playback.skipBackMs`, `playback.skipForwardMs`, `playback.upNextCountdownMs`, `music.crossfadeMs`; `audioEffects.crossfadeMs` |
| queue `postPlay` (all queue routes) | `countdownSeconds` | `countdownMs` |
| `GET /v1/playback-sessions/{id}/chapters` | `duration`, chapters `startSeconds`/`endSeconds`, action `positionSeconds` | `durationMs`, `startMs`/`endMs`, `positionMs` |
| `GET /v1/admin/playback/sessions` (activity rows) | `positionSeconds` (nullable) | `positionMs` (nullable) |
| `GET /v1/items/{id}/lyrics` sources | `startSeconds`, `duration`, `sourceDuration` | `startMs`, `durationMs`, `sourceDurationMs` |
| `GET /v1/items/{id}/trickplay` sets | `intervalSeconds`, `durationSeconds`, `sourceOffsetSeconds` | `intervalMs`, `durationMs`, `sourceOffsetMs` |
| `GET /v1/items/{id}/playback-offers` sources and `deliveryPlan`; prepared `facts` (offers, prepared versions, delete) | `duration` | `durationMs` |
| download requests/preparations (`title`) | `durationSeconds` | `durationMs` |
| download preparations `progress` | `etaSeconds` (nullable) | `etaMs` (nullable) |
| `POST …/grant` | `renewAfterSeconds` | `renewAfterMs` |
| download receipts issue/revalidate/revoke `viewing` | `positionSeconds` | `positionMs` |
| `GET …/bundle` | chapters/markers `startSeconds`/`endSeconds`; trickplay `intervalSeconds`/`durationSeconds`/`sourceOffsetSeconds` | `startMs`/`endMs`; `intervalMs`/`durationMs`/`sourceOffsetMs` |
| home and home row, item/show recommendations | `cacheTtlSeconds` | `cacheTtlMs` |
| browse capabilities `queryLimits` | `cursorTtlSeconds` | `cursorTtlMs` |
| personal-state writes, batch, saved resources, history actions, recommendation reset | `receiptLifetimeSeconds` | `receiptLifetimeMs` (2592000000) |
| Watch Together `POST /v1/groups/{id}/invites` | `expiresInSeconds` | `expiresInMs` (60000–max, default 900000) |

Contracts regenerated (`go run ./cmd/apigen`), OpenAPI document regenerated, API guide (`server/api/guide/01-overview.md`, `06-events.md`), `notifications.md`, `social.md`, `downloads.md` and `preferences.md` updated. MCP tools needed no change (they already used `positionMs`; their outputs are the routes' outputs).

## 5 Schema changes

None. DVR defaults stay in `before_seconds`/`after_seconds` (converted at read/write); prepared-media `facts_json` keeps seconds (a separate stored form, `Facts.EncodeStored/DecodeStored`, so existing prepared versions read unchanged).

## 6 Settings added

None new. Renamed (now milliseconds): console `throttleBufferMs`, `playedRetentionMs`; viewer `playback.skipBackMs`, `playback.skipForwardMs`, `playback.upNextCountdownMs`, `music.crossfadeMs`.

## 7 Tests run

- `go build ./...`, `go vet ./internal/...` (server) → clean.
- `go test` administration, catalog, downloads, hosted, identity, livechannels/..., lyrics, mediaanalysis, operations, playback, playbackv1, preparedmedia, recordingmedia, social, telemetry, apiwire, `cmd/server`, `apikit/apierror` → pass.
- `go test ./internal/httpapi` full run → pass except `TestListeningHTTPPreferenceEnvelopeAndCAS` ("explicit reviewed helper binary required", environment). Two failures seen once under parallel load (`TestEveryRouteSurvivesHostileInput` on the group events stream, `TestFiniteRealRestartReplaysPersistedQueueInOrder` in playback) passed when rerun alone. After the main merge: OpenAPI/registry, read-path golden, foundation contract replay, events long poll, receiver inbox wait, restriction matrix, operations, downloads → pass.
- Golden/fixture files regenerated: `PORTICO_GOLDEN_UPDATE=1` (read-path golden) and `PORTICO_CONTRACT_UPDATE=1` (contract fixtures); diffs checked to be only the renamed fields.
- `packages/contracts` tests → 30/30. client-core: `tsc --noEmit` clean, every test file passes, `lint:routes` passes.
- web: `tsc --noEmit` clean, `lint:hooks`, `lint:strings`, preference-consumer check (39 keys) pass; `npm test` 587 pass, 1 fail (`feedback-label.test.ts`, `feedbackCategoryLabel is not a function`, in a file this lane did not touch).
- native: `tsc --noEmit` and test tsconfig clean, every test file passes, `lint:hooks`, `lint:strings` pass. Vega: `tsc` (app and tests) clean, every test file passes, `lint:strings` passes.
- Shared packages mirrored with `--write`; `--check` identical to portico-server 04db236b.

## 8 Device test notes

- Skip back/forward buttons (phone, tablet, TV, web, Cast bar, Play On bar, Android media session, Vega remote) still jump the chosen seconds after changing the setting; the Settings choices read 5/10/15/30 s and 10/15/30/60 s.
- Up Next countdown: choose 0/5/10/15 s; the post-play card counts that long (online and offline).
- Crossfade on music: choose 2–12 s; the overlap is audible at that length (web, Android, Apple).
- Live TV owner settings: stream buffer and retry window show and save seconds; a live source override shows "server default N s"; guide refresh choices save; tuner search still finds HDHomeRuns.
- DVR: start-early/finish-late on a recording and on a series rule land at the right minutes in the schedule; owner DVR defaults save in minutes.
- Library settings: "a picture every N seconds" saves and trickplay previews keep their spacing; scan-interval choices on library folders save.
- Downloads: progress ETA, transfers that renew their address, offline chapters/markers/scrub previews in a downloaded file, resume position after revalidation.
- Player chapters list and lyrics timing line up with playback.
- Events/notifications long poll on devices without SSE (Vega, Apple fallback) keep receiving events and don't time out.
- A server whose settings were saved before this build: throttle/retention, live buffer/retry, skip/countdown/crossfade preferences show defaults once and save normally afterwards.

## 9 Not finished

- Hosted repo (no worktree here): rename `retryAfterSeconds` in `hosted-services/apikit/apierror` and `heartbeatAfterSeconds` in `hosted/internal/directory`, together with the server's `hosted/current_schedule.go`; Quick Connect/Hosted device flow `expiresIn`/`interval` left in RFC 8628 shape (see Decisions).
- Microsecond-string clocks (subtitles, Watch Together) and the Cast receiver protocol are outside "seconds" and were not changed.
- Not exercised on devices or in end-to-end drivers.

## Commits

- portico-server (`audit-implementation/cp4-ms`): `73b05156` Carry every duration on the wire in milliseconds; `04db236b` merge of `audit-implementation/main`.
- portico-react-native (`audit-implementation/cp4-ms`): `881ffb3` mirror from 73b05156; `b4da641` Read and send the millisecond wire fields on phones, tablets, TV and Vega; `a6ccc03` merge of `audit-implementation/main` and mirror from 04db236b.
