# Lane report: downloads

Worktrees: `server-downloads` (portico-server) and `native-downloads` (portico-react-native), both on branch `audit-implementation/downloads`. Nothing pushed.

Commits (Justin's identity, none pushed):
- portico-server `audit-implementation/downloads`, 11 commits over `87e3fbd`: `347e26f` `7adcb49` `0de7e7e` `0abec18` `e6df8dd` `d952772` `9ce0fea` `13600d2` `080c570` `cbfd5ba` `446122c`.
- portico-react-native `audit-implementation/downloads`, 5 commits over `0509de2`: `eb29a22` `7269336` `91bc168` `96c9516` `112a425`. `packages/*` there is identical to the server worktree's (`shared-packages.mjs --check`).

What was run here, all passing: `go build ./...` (Linux, `GOOS=windows`, `GOOS=darwin` for `internal/...`), `go vet` on the touched packages, `go test` on `internal/downloads`, `internal/preparedmedia`, `internal/decoder`, `internal/apispec`, `internal/persistence` (through `scripts/schema-sync.py`), targeted `internal/httpapi` (`-run 'Download|ProfileFeature|RestrictionMatrix|Lane|Admission|WrappedDomain'`), targeted `internal/catalog` (`-run 'Personal|Activity|Progress|PlayHistory|History'`) and `internal/subtitles` (`-run 'List|Manifest|WebVTT|Download'`); the four client-core download test files (40 tests); the four native download test files (`offline-ownership` 32, `container-enrollment` 4, `local-playback` 9, `offline-groups` 2); `npx tsc --noEmit` in `web/`, in `apps/native` and for `apps/native/test`; the Kotlin sources through the Kotlin compiler (no Gradle).

The cheap helper model was used for four bounded jobs, each reviewed and re-run by me: re-pointing the guide chapter at the generated contract, bringing the client-core download tests and the native download tests in line with the new source, and writing the bundle and subtitle-reader Go tests. It changed no non-test source. **Nothing was run on a device, in a simulator or in a browser. The Objective-C was not compiled** (no Xcode here); it was written and read, and checked only for balanced brackets. Section 8 says how to test it.

## 1. Outcome

- **A copy on a device outlives server tidying.** A receipt now answers only to what its viewer may do. The server's own copy running out its retention, the file being replaced, a converted copy being deleted, the title being removed, or the viewer removing the claim from the server's list all end the *claim* and leave the copy playable. The copy ends when the owner takes access away: downloads switched off for the profile, the account disabled, the title or its library closed to the viewer, or the receipt withdrawn. A title that has left the server is reported (`titleOnServer: false`, `title.onServer: false`) and shown as "No longer on your server. It stays on this device until you remove it."
- **Everything a download needs travels with it.** Every claim carries its title facts (name, year, length, show/album/book and season/disc and episode/track/part numbers). A bundle route describes the rest: description, artwork (poster 800 wide, backdrop 1920, episode still, album cover), chapters, skip markers, the audio tracks inside the file, and every text subtitle track as WebVTT. The device stores the bundle and the files beside the media file and builds its Downloads list from them alone: real titles, episodes grouped under their show in order, songs under their album, artwork from local files, no request to the server.
- **Transfers resume instead of restarting.** Android continues `partial.part` with `Range` and `If-Range: "sha256-…"` under any address for the same file, retries a dropped connection by itself (about a quarter of an hour of growing waits), waits for Wi-Fi and carries on when it returns, and a transfer left by a killed process is started again from its bytes the next time the app runs. iOS continues through the system's own resume data, which works because the app now keeps one address per download alive. Integrity is still verified by SHA-256 at the end on both.
- **Addresses outlive an OS pause.** A transfer address stays good while it is used or renewed, 24 hours after the last of either, and `renew` extends the same address (even one that had run out, for up to 7 days). Every request made with it still rechecks permission.
- **Originals are served, not copied.** The library file is read once in place (checkpointed windows) to learn its SHA-256; the digest is remembered per file and shared by every viewer, so the second person waits for nothing. The file is served directly through the same confined opener playback uses, which refuses a file whose size or modified time changed. No state-volume space is used. The device verifies the digest of what it received.
- **Quality options are real.** The menu lists the original plus each smaller quality the server already holds a copy of or the conversion code reports it can build (today 1080p and 720p, and the audio recipe). The same check guards claims and the owner's Optimize list. When the conversion code learns a recipe its rung appears with no change here.
- **Offline plays count.** The device keeps one journal entry per play. The server passes each through the same personal-state writer online playback uses, dated to when it happened: it reaches the owner's play history, the viewer's history and the play counts, completion follows the viewer's own thresholds and credits markers, and it moves the resume point and watched state only when it is the newest thing known (not older than online playback, an explicit mark on the title or its show/season/album/book, or another offline play).
- Other fixes are listed in section 3.

## 2. Decisions I made

1. **Removing a claim from the server's list no longer removes copies from devices.** A device removes its own copy (and then withdraws that copy's receipt on the server). The explicit way to pull a copy off another device is still `receipts/revoke`. Reason: the brief says a copy stays until the person removes it or the owner revokes access; the server list is the server's side.
2. **Sign-out keeps the files** (as the code did, against the old contract text). A different profile cannot see them, but now sees how much space they take and can remove them ("Downloads from other profiles use … on this device. Remove them"). The contract document should be corrected (§3 rule 6, §11).
3. **Transfer address lifetime: 24 hours idle, extended by use and by renewal, revived by renewal for up to 7 days, ended with the claim.** This is longer than the old ten minutes on purpose (the brief asks for it); every request still rechecks the viewer's permission and that the bytes are unchanged.
4. **Originals: read once in place, trust size + modified time afterwards.** A file altered in place without either changing would be served with a stale digest; the device's own SHA-256 check rejects it ("The file arrived damaged"). This is the trade-off the brief asked for in place of the copy.
5. **The owner's storage ceiling (`maxPreparedBytes`) now counts converted copies only.** Originals take no server space and are never refused for it.
6. **Retention still expires an unfetched claim** (default 30 days); it now frees only the claim.
7. **Receipts are renewed once a day**, not on every connection, in one write per 100. Renewal is also how a device learns access was withdrawn, so withdrawal reaches a device within a day of it being online (immediately when the server answers "downloads are off for this profile").
8. **`/v1/downloads/receipts/revalidate` and `/v1/downloads/progress` are no longer shut by the per-profile downloads switch.** Before, switching downloads off for a profile made renewal answer 403, the app treated that as "offline", and the copies kept playing until their receipts ran out. Both routes hand nothing out.
9. **A title stored as several files is refused with its own reason** (`several_files`); a title with several *versions* downloads the first readable one. When the identity lane's best-version rule lands it belongs in `resolveSource` (`server/internal/downloads/targets.go`), the one place the choice is made.
10. **A whole-show request no longer fails when the library changes while it is read.** It restarts the reading (up to 3 times), then carries on regardless; every member is still rechecked at admission. "Everything in this show" ignores the viewer's own watch state.
11. **Offline plays older than what the server knows are still recorded in history**, and change nothing else. A viewer with paused history: the owner's record is kept, nothing of the viewer's is written (same as online).
12. **The two hand-written OpenAPI files for downloads were deleted** (`downloads.openapi.yaml`, `download-requests.openapi.yaml`); the generated `portico.openapi.json` is the contract and the guide chapter points at it. `server/api/downloads.md` was rewritten as the human guide.
13. **Android: no WorkManager** (own code over new dependencies). A transfer does not restart while the app's process is dead; it restarts the next time the app runs. The foreground service now stops cleanly when Android says its daily allowance is spent.
14. **Device journal and state document are version 3; older documents keep only settings and server keys** (pre-release, no compatibility code).
15. **Clock protection (B-14.21) not added.** Still wall-clock; low severity and "not DRM" by decision.
16. **`renewed`/`retention` texts** and all download reason sentences moved into the string catalogue; wording rewritten as plain sentences.

## 3. Bugs

| ID | Result |
|---|---|
| B-14.1 | Fixed (server `receipts.go` `Revalidate`, `worker.go` `tidy`; device removes only on a real refusal). |
| B-14.2 | Fixed on Android (real byte-range resume under any address). Fixed on iOS for every case where the address is unchanged, which is now the ordinary case; when the address had to change the system's partial cannot be re-pointed and the transfer restarts (see section 9). |
| B-14.3 | Fixed (renewable 24 h address; the app renews hourly while a transfer runs and on every attempt). |
| B-14.4 | Fixed (menu, claims and Optimize list ask the encoder's own argument builder). |
| B-14.5 | Fixed (no copy, one read, shared digest, bounded windows; nothing holds the worker for a whole file). |
| B-14.6 | Fixed (receipt taken before the first byte; stored with the entry at start). |
| B-14.7 | Fixed (title on every claim; web and native lists use it; browser saves "Film (2000).mp4"). |
| B-14.8 | Fixed (server lists a device's requests; the shared service finds them again on start). |
| B-14.9 | Fixed (daily renewal, one manifest write per batch; the provider was already keyed per viewer by the fix pass). |
| B-14.10 | Fixed (shared writer; history, counts, thresholds, credits, explicit-mark ordering, position cleared on completion, songs keep no resume point, writer ownership). |
| B-14.11 | Already fixed; behaviour kept and re-expressed through the shared writer (book tests pass, one assertion updated: the pointer is now held by the offline play rather than by nobody). |
| B-14.12 | Not fixed. The bundle now tells the device the file's container and codecs, but choosing the mpv engine for a file the native player cannot open, or marking such an option in the menu, needs the player lanes (section 7). |
| B-14.13 | Fixed (progress no longer moves the revision; the shared client re-reads and re-sends once on a conflict). |
| B-14.14 | Partly fixed: other profiles' space is shown and can be removed; files are still kept at sign-out (decision 2). Settings stay device-wide. |
| B-14.15 | Fixed (a ready claim whose converted copy is gone becomes `unavailable`, at tidy and at grant/transfer time; copies on devices are untouched). |
| B-14.16 | Fixed (unknown receipt keeps the copy and asks for a new receipt; unknown claim re-claims and adopts it when the digest matches). |
| B-14.17 | Fixed (decision 10). |
| B-14.18 | Fixed in code (the arrived file is kept in the download's folder and verified at next launch); needs the device test in section 8. |
| B-14.19 | Fixed in code (launch reconciliation of entries against live tasks; Pause works with no task). |
| B-14.20 | Partly fixed: automatic retry, Wi-Fi return, restart after process death on next run, service time-out handling. No restart while the process is dead (decision 13). |
| B-14.21 | Not fixed (decision 15). |
| B-14.22 | Fixed (renewal carries the server's resume point; progress results set it). |
| B-14.23 | Fixed (asking again restarts a failed claim; "already in your downloads" when nothing new). |
| B-14.24 | Fixed (hourly bounded pruning of ended claims, finished requests, dead receipts, orphaned marks and digests). |
| B-14.25 | Fixed (housekeeping at most once a minute, reads first, no write when nothing is due). |
| B-14.26 | Fixed (versions download; parts refuse with `several_files`). |
| B-14.27 | (a) fixed; (b) the device withdraws its receipt on removal, the claim is left (decision 1); (c) fixed, reasons are a closed set; (d) fixed, a retried ended claim steps aside for a newer live one; (e) no longer applies (no first-use window); (f) Android still refuses redirects, deliberately. |
| N-V08.1 | Already fixed in the fix pass; its remaining gap (a request not found again after restart) is closed by B-14.8. |
| B-12.3 | Fixed (an explicit mark newer than the observation wins; the watched intent is dated to the play). |
| B-12.5 | Fixed. |
| Doc 22 | No bug entry in document 22 concerns downloads; its offline notes (A-22.41, A-22.47) describe behaviour that is kept (per-viewer journal). |
| New | `history_paused` was missing from the client parser, so a viewer with paused history could never drain the offline journal. Fixed. |
| New | `TestDownloadTransferSupportsRangeAndValidators` failed on the base branch (setup needs a JSON content type). Fixed and extended. |
| New | Offline progress wrote state for titles the viewer could not see (and told a hidden title from a missing one). Now answers `item_deleted` and writes nothing. |
| New | A device-chosen play id is bound to the viewer before it is used as a history key, so it cannot reach another viewer's row. |
| New | Shared downloads service polled a failing list every 15 s for ever, and left a timer behind on disposal. Fixed (backs off to 5 minutes; timers cleared). |

## 4. API changes

All download routes are now on the apikit registry (`server/internal/httpapi/downloads_routes.go`); errors use the registry envelope `{error:{code,message,retry,current?,requestId}}`. Generated contracts regenerated (`server/api/portico.openapi.json`, `packages/contracts`).

| Route | Change |
|---|---|
| `GET /v1/items/{id}/download-options` | Lists only producible or existing qualities. Unavailable original reports `several_files` where that is the cause. |
| `POST /v1/downloads/preparations` | Items carry `title`. A failed existing claim is restarted and counted in `accepted`. An unproducible quality is rejected per item. |
| `GET /v1/downloads/preparations[/{id}]` | `title: {kind,title,year,durationSeconds,parentId,parentTitle,groupId,groupTitle,groupNumber,number,artist,onServer}`. `itemId` is stored, so it survives the title's removal. `revision` moves only on state changes. |
| `POST …/{id}/actions` | Conflict is `409 download_conflict` with the row in `error.current` (was a top-level `preparation`). Remove/cancel no longer withdraw receipts. |
| `POST …/{id}/grant` | Body `{operationId, renew?}`. Response drops `replayWindowSeconds`, adds `renewAfterSeconds`, `renewed`; `200` when renewed, `201` when new. |
| `GET/HEAD /v1/downloads/artifacts/{grant}` | Readable `Content-Disposition`. New errors: `410 download_unavailable`, `503 download_source_offline`. No first-use window. |
| `GET /v1/downloads/preparations/{id}/bundle` | New. |
| `GET /v1/downloads/artifacts/{grant}/artwork/{role}` | New (poster, still, cover, backdrop). |
| `GET /v1/downloads/artifacts/{grant}/subtitles/{resource}/{revision}.vtt` | New. |
| `POST /v1/downloads/receipts` | Outcome adds `titleOnServer`. Unknown claim answers `unknown_preparation` (was `unknown_receipt`). |
| `POST /v1/downloads/receipts/revalidate` | No longer depends on the claim's state. Outcome adds `titleOnServer` and `viewing: {positionSeconds, watched}`. `retention_expired` is no longer a refusal. Answers even when downloads are off for the profile. |
| `POST /v1/downloads/receipts/revoke` | `reason` must be a documented reason code. |
| `POST /v1/downloads/progress` | Entry is `{playId, itemId, positionSeconds, ended?, observedAt}` (`watched` removed). Result adds `playId`. `history_paused` outcome documented. Answers even when downloads are off for the profile. |
| `GET /v1/downloads/settings` | Adds `qualities: [{quality,label,kind,targetDisplayHeight}]`. |
| `GET /v1/downloads/requests?deviceId=` | New. |
| `GET /v1/downloads/requests/{id}` | `errorCode` no longer includes `selection_changed`. |
| Removed | `server/api/downloads.openapi.yaml`, `server/api/download-requests.openapi.yaml`. |

client-core (`packages/client-core/src/downloads.ts`, `downloads-service.ts`, `download-requests.ts`) follows all of the above: `parseDownloadTitle`, `parseDownloadBundle`, `downloadTitleLines`, `DownloadsService.grant(prep, renew?)`, `.bundle()`, `.qualities()`, `.followDevice()`, `listDownloadRequests`. `downloadQualities` is gone (a quality is whatever the server lists).

## 5. Schema changes

`server/internal/persistence/migrations/0001_baseline.sql`, download tables only; `scripts/schema-sync.py` run.

- `download_preparations`: added `item_public`, `title_json`, `source_modified_ns`; removed `source_version_json`.
- `download_grants`: added `artifact_digest`.
- `download_receipts`: added `item_public`, `library_id`.
- `download_requests`: added `restarts`.
- New table `download_source_digests(asset_id, size, modified_ns, sha256, hashed_ms)`.
- The state folder's `downloads/source-snapshots` directory is no longer created or used.

## 6. Owner or viewer settings added or wired

None added. `downloads.maxPreparedBytes` now applies to converted copies only (decision 5). `downloads.retentionDays` now frees only the claim. `GET /v1/downloads/settings` additionally reports the producible qualities (computed, not stored).

## 7. Needed from others

- **Player lanes (native-playback, apple-mpv, android-mpv):** `offlineDownloads.localMedia(entry)` now returns `extras` (`apps/native/src/bridge/local-playback.ts` `LocalMedia.extras`: the bundle with chapters, markers, audio tracks and media facts, plus subtitle tracks as local `file://` WebVTT files and the backdrop). The local player path still shows no subtitles, chapters or skip buttons and has no engine choice: `apps/native/src/player/usePlayer.ts` excludes local sessions from `subtitleScope`. Wiring those in, and choosing mpv when `extras.bundle.media.container` is one the native player cannot open (B-14.12), is player work.
- **subtitles-text lane:** `server/internal/subtitles/download.go` is a new additive file built on `ListTx`, `scanResource`, `resourceColumns`, `visible`, `readTx`, `s.objects.Open` and `ManifestWebVTT` as they are on this base. If the lane renamed any of these, this file needs the same rename. To carry styled and image originals with a download: list the extra form in `DownloadTrack.Formats` and add a route beside `get_download_subtitle` for it; the bundle, the client parser (`files` keyed by form) and the device (`sub-<id>-<rev>.<form>`, prefers `ass`, `pgs`, `vobsub` over `vtt`) already carry any form.
- **api-platform lane:** the 21 download routes are new on the registry and carry no API-key scope; they need one when the scope gate merges. `server/api/guide/check-examples.mjs` no longer lists the two deleted files; chapter 05 points at `portico.openapi.json`.
- **identity lane:** `catalog/personal_activity.go` gained a `clock` parameter on the writer's body (`acceptPlaybackProgress`) and `catalog/offline_play.go` adds `AcceptPlaybackProgressAt`; `playback/offline_play.go` adds `AcceptOfflinePlay`. These are the only calls into personal state from downloads (`server/internal/downloads/progress.go`, one call). The read that decides "newest" (`newest()` in the same file) reads `personal_explicit_events`, `container_personal_state`, `progress_activity`, `playback_observations`, `book_resume`.
- **accounts-security lane:** `requestFeature` in `httpapi/restriction_admission.go` exempts the two routes in decision 8; `profile_feature_matrix_test.go` updated to match.
- **conversion-outputs lane:** when new recipes are added, `preparedmedia.acceptOutput` (H.264/AAC stereo/≤30 fps assumptions) and `packages/client-core/src/prepared-media.ts` (`parsePreparedVersion`, rejects wider than 1920) still assume today's two recipes; the download menu itself needs nothing.
- **Console (web):** no screen exists for `GET /v1/downloads/usage?scope=server` or for revoking a profile's downloads; unchanged by this lane.
- **Documents to correct:** `Product Contracts/saved-state-and-downloads/contract.md` §3 rule 6 and §11 (sign-out), §8–§10 (states, receipt shape, grant lifetime), `architecture-amendments.md` line 71 (ordering is by observation time, mapped onto the intent order), `Project Architecture/04 Playback and Media.md` where it describes the snapshot copy.
- **Vega:** downloads remain hidden there (module rejects); unchanged.

## 8. Test notes for other environments

Server for all of these: a build of `audit-implementation/downloads` with a library containing (a) one MP4 film with an external `.srt`, (b) one MKV film larger than 2 GB, (c) one season of a show, (d) one album.

**Server, any OS (Windows and macOS included).**
1. Claim an original of (b). Pass: the claim is `ready` after one read of the file (watch disk reads), no `downloads/source-snapshots` folder appears in the state directory, and a second profile claiming the same film is `ready` at once.
2. With a transfer half done, replace the file on disk and rescan. Pass: the transfer stops with 410, the claim shows "The file on the server was replaced…", asking again prepares the new file. Before the rescan the transfer answers 503 and nothing is ended.
3. Set retention to 1 day, download (a) to a phone, wait a day (or set the server clock forward), open the app online. Pass: the claim is `expired` on the server and the film still plays on the phone with the network off. Fail: it is removed.
4. Delete (a) from the library, open the app online, then go offline. Pass: it still plays and shows "No longer on your server…".
5. Switch downloads off for the profile, open the app online. Pass: its downloads are removed within that session.
6. On Windows specifically: step 1 and a ranged transfer (`curl -r 1000-2000` on the grant URL returns 206 with `ETag: "sha256-…"`).

**iPhone and iPad.**
1. Start (b) on Wi-Fi, turn Wi-Fi off for two minutes mid-transfer, turn it on. Pass: the percentage continues from where it was, not from 0.
2. Start (b), press Pause, wait 15 minutes, Continue. Pass: continues from where it was.
3. Start (b), swipe the app away, reopen after a minute. Pass: the entry continues by itself from where it was (resume data from the force-quit).
4. With "Wi-Fi only" on and the phone on mobile data, ask for (a); the row says "Waiting for Wi-Fi". Join Wi-Fi 30 minutes later with the app closed. Pass: it downloads. Then repeat leaving the phone off Wi-Fi for more than 24 hours without opening the app: expected result is that it restarts from 0 when the app is next opened online (the known gap in section 9); note what actually happens.
5. Queue the season, lock the phone, let it finish, turn on Airplane Mode *before* opening the app. Pass: every episode plays. Fail: "Connect to your server once to finish setting up this download".
6. Large file verification (B-14.18): download (b), and as soon as the bar reaches 100% force-quit. Reopen. Pass: the entry becomes ready after a moment without downloading again. Record the file size and how long the check took.
7. Downloads list offline (Airplane Mode, cold start, "Watch downloads"): episodes sit under the show name in order with the show's poster; the film has its poster; nothing shows an id.
8. Play (a) offline for a minute, stop, go online. Pass: Continue Watching on the web shows it at that minute, History shows the play at the time it was watched, not the time it synced. Mark it watched on the web, then sync an older offline play from a second device: it must stay watched.
9. Files: in the app container, `Application Support/Downloads/<id>/extras/` holds `bundle.json`, `poster`, `backdrop` and `sub-….vtt`.

**Android phone, and a Fire tablet.**
1–3 and 5, 7, 8 as above. Additionally:
- Start (b), force-stop the app from Settings, reopen. Pass: continues from the partial (`partial.part` in `no_backup/Downloads/<id>/` is not deleted).
- Start (b) on Wi-Fi with "Wi-Fi only" on, switch to mobile data: the row says "Waiting for Wi-Fi" and resumes by itself on Wi-Fi with the app in the background.
- Pull the network for 30 seconds: the row says "Waiting for a connection" and continues.
- Leave a very large download running in the background for several hours on Android 15: when the system ends the service the entry shows Paused and continues when the app is opened. Fail: the app is killed with an ANR or the entry is stuck at "downloading".
- Fire tablet: confirm the download notification appears and that downloads work with the app in the background at all (Fire OS background limits differ).

**Web.** Downloads page: rows show titles ("Show · S1 E02"), Save gives a file named after the title, a claim whose title was deleted says "This title is no longer on the server."; the whole-season dialog lists Original plus the qualities the server reports; a whole-season request is still listed after a page reload.

## 9. Not finished

- **Nothing was run on a device.** The Objective-C module (`apps/native/Native/PorticoDownloads.m`) was not compiled. The Kotlin was type-checked only.
- **iOS: when the transfer address has to change, the system's partial file cannot be continued and that file restarts from zero.** This happens if a transfer sits untouched for more than 24 hours with the app never opened, or the claim was re-made. An app-managed range download on iOS would close it; it was not built because it cannot be validated without a device and would fight the background session.
- **The offline player does not yet use what travels with the download.** Subtitles, chapters, skip markers and audio-track names are stored and handed to the player (`LocalMedia.extras`) but the player's local path ignores them, and there is no next-episode queue offline. Player lanes (section 7).
- **Styled and image subtitle originals are not carried** (text tracks as WebVTT only); embedded text tracks are carried only where the subtitle catalogue lists them as resources on this base. Needs the subtitles-text lane's originals (section 7).
- **Chapter thumbnails and trickplay images are not carried.**
- **B-14.12** (an original the device's native player cannot open is still offered without a note).
- **B-14.21** (clock rollback) and **Android restart while the process is dead** (decisions 13, 15).
- **B-14.14** only partly (sign-out keeps files; settings are device-wide).
- The native bridge's new paths have unit tests for ownership, renewal, receipts-first, plays and grouping, but none that drive `tend()` (automatic restart of a stopped transfer, hourly address renewal, completing a bundle later) or `recover()` (unknown receipt); those were written and type-checked only.
- The native ownership test file keeps its existing `APP-01`/`APP-02` test-name prefixes (an older convention in that file), including on the tests added there.
- Full test suites were not run (by instruction). The full `internal/httpapi` suite in particular exercises routes this lane moved onto the registry and should be run at integration.

## 10. After merging the base (13 Oct, later)

`audit-implementation/main` was merged into the server branch twice (it moved during the first merge); head is `95e4f1c`, worktree clean. The native worktree was not touched.

Decisions in the merge:
- **Shared parsers:** the web-app lane's tolerant reading is kept (an unreadable row is left out, counts are read off the rows, an unknown reason is said as its own words) together with this lane's contract (titles, play ids, `history_paused`, renewable grants). Two places where the sides disagreed: a quality the app does not know is **kept** in the menu and accepted in a receipt (the server's menu is the list; a receipt for a file already on the device must not fail over a quality name), and a claim whose title cannot be read is listed without a name rather than dropped. `parseDownloadSettings` falls back to the original alone when the quality list is missing.
- **Key scopes:** every download route declares one in its registration: reads `read`; claiming, grants, receipts, revoking and offline plays `playback`; owner settings `admin`; the routes fetched by transfer address alone (file, artwork, subtitles) `open`, because they carry no credential and taking the address is what needs `playback`. Their rows were removed from `route_scopes.go`. `get_download_options` uses device access with its own item check (the registry reserves viewer-item access for `{itemId}` paths).
- **Conversion sizes:** nothing to change; the menu already asks the encoder's builder, which now accepts all six sizes, so 2160p to 360p are offered where the source is tall enough. The output check takes the conversion lane's level and 60 fps rule.
- **Version choice:** takes the identity lane's best-present-version order.
- **Subtitle originals now travel:** a styled script (`ass`) with its attached fonts (`font0`…), a picture stream (`pgs`, `dvb`), a VobSub pair (`vobsub` + `sub`), each beside `vtt` where a text reading exists. New route `GET /v1/downloads/artifacts/{grant}/subtitles/{resource}/{revision}/{file}` (`original`, `companion`, `font-N`). Tracks are read out of their file on first need; the bundle answers `409 download_not_ready` while a styled track is still being read, and the device asks again.

Run after the merge, passing: `go build ./...` (Linux, Windows, macOS), `go vet` on the touched packages, `go test` for `internal/downloads`, `internal/preparedmedia`, `internal/apispec`, the subtitle download tests, `internal/httpapi` with `-run 'TestEveryRouteDeclaresACredentialScope|Scope|Lane|Admission|ProfileFeature|Download|RestrictionMatrix|WrappedDomain|Registry'`, `npx tsc --noEmit` in `web/`, and the four client-core download test files (47 tests). Contracts and schema lists regenerated.

Still open after the merge:
- `internal/subtitles` `TestSubtitleRequestsTakeTheWriteGateInTheirOwnClass` fails; it exercises `List`, and this branch's only difference from the base in that package is the two additive download files.
- The native copy of `packages/client-core` still has the older bundle parser (at most 8 files per track); a styled track with more than six fonts is refused by it until the packages are re-copied. The device stores the new forms generically (`sub-<id>-<rev>.<form>`) and already prefers `ass`, `pgs`, `vobsub` over `vtt`; nothing plays them offline yet (player lanes).
- A claim cannot name a particular version of a title; it takes the best one present.
- When two titles are merged, claims and receipts of the title that was folded in are not re-pointed: the claim ends as "no longer in the library" and its copies renew as `titleOnServer: false`, still playable.
- Converted downloads are still H.264 SDR stereo at every size; HEVC, HDR-preserving and multichannel *prepared* recipes do not exist yet (the conversion lane left them to prepared media).
