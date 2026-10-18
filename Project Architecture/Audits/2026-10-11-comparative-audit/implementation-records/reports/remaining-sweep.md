# Remaining sweep — everything not yet done (15 Oct 2026)

Read-only lane. Checked against `main-server` 6ae3ccf4 and `main-native` ea762a7 (`audit-implementation/main`; final-pass-3 had not committed anything on top when this was checked) and `main-internal`. No source was changed. No subagents or helpers were used.

**Sources swept:**
- all 101 lane reports, with every Needed-from-others, Not-finished, Leftovers and Follow-ups section extracted and read;
- `coverage-open.md` and `coverage-ledger.md`;
- decision-audit W/N/T/V/S;
- the owner's report: §3–4, Appendix 0 (gap lists and the remaining-limits column), Appendix D and Appendix E;
- `PROGRAM-BRIEF.md` §1, decision by decision;
- `STATE.md`, for what Justin has since answered.

**Method.**
- Items were deduplicated, and later lanes win: the cp-*, cp-followups and final-pass-2 reports supersede earlier gap lists.
- Every item filed as A was checked in code with grep or a read. The evidence is given in each line.
- A few items could not be re-checked cheaply. They are marked "(not re-verified)" rather than dropped.
- Bug IDs follow Appendix D, then the cp-lane closures. All 588 are accounted for there.

## Summary

| Class | Count | Meaning |
|---|---:|---|
| **A** — codeable now on Ubuntu | **69** | server 25, web 11, native 22, cross-cutting 2, trivial cleanups 9. 4 native lines (N12, N15, N16, N17) are Apple/Android code that can be written here but needs a device to prove. |
| **B** — platform / device / real hardware | 22 | Mac/Xcode, devices, real media/GPU/tuner, Windows/macOS/Docker installs, other browsers, capacity hosts |
| **C** — needs Justin | 16 | decisions not yet made, approvals, credentials and signing, pushing and release |
| **D** — deliberately excluded by Justin | 14 | listed with the decision each one rests on |
| **E** — in progress in a cp2 lane | 23 | cp2-viewer 5, cp2-native 6, cp2-server 4, cp2-reads 8, plus final-pass-3 |
| done (was listed somewhere as open, now verified closed) | 60+ | brief list at the end |

**Short answer to Justin's question:** no. Besides device and platform work, there are 69 codeable items not in flight (class A, 9 of them trivial cleanups) and 16 items waiting on you (class C). Most of the A items are small. The large ones are:
- rich (HEVC/HDR/multichannel) optimized downloads (A16);
- fMP4 and two-engine Live TV (A17);
- HDR mastering metadata capture (A18);
- the native credits-time Up Next (N1);
- automatic provider-identity joining for music and books (A11).

Each of these sits under one of your decisions and is not in any cp2 lane.

---

## A — codeable now (not in flight)

Size: XS is under an hour, S is under half a day, M is one to three days, L is more than that.

### Server

| # | What is missing | Size | Source | Evidence (current code) |
|---|---|---|---|---|
| A1 | Webhook edits are not revision-fenced: two owners editing one webhook is last-write-wins (server, then web and native callers) | S | native-owner §7; App. E | `webhooks.go` update takes no expected revision; the `revision` column exists but is never compared |
| A2 | Saved-resource writes other than reorder answer an unregistered `409 playlist_conflict` that no contract lists | S | cp-followups §9 | `httpapi/router.go:491`; absent from `packages/contracts` |
| A3 | Conditional GET (B-08.12 residual): no typed route adopts `apikit.Conditional`, so 304s still compose the response first | M | cp-polish §9 | grep `apikit.Conditional(` in `server/internal` = 0 |
| A4 | Time units not unified (A-08.41): about 88 wire fields are still `*Seconds` (positionSeconds, startSeconds, progressSeconds…) while general media fields are ms | M | search-sort §9; base-health §7 | `grep 'json:"…Seconds'` = 88 |
| A5 | The visibility-class builder has no cooperative pause/yield, so the owner's "pause background work" does not cover it | S | limits-settings; review-livetv | `catalog/visibility_classes.go` and `visibility_incremental.go` never call `dbwork.Yield`/`PaceBackground` |
| A6 | Deep analysis still has a fixed 6-hour outer timeout instead of a progress signal | S | limits-settings; review-livetv | `ingestion/deep_analysis.go:19` |
| A7 | A show merge runs as one write transaction over all children (a large show holds the write gate) | M | identity §9; review-identity | `catalog/title_merge.go` `mergeShowChildrenTx` |
| A8 | No redirect from a merged-away title id (old links, bookmarks, cached offline receipts) | S | identity §9; server-followups §9 | nothing in `title_merge.go` |
| A9 | Library removal can leave a hidden empty library row when derived per-library rows still reference it | S | identity §9 | `catalog/library_removal.go` savepoint fallback ("stays as a hidden, empty row") |
| A10 | Automatic provider-identity joining for albums, artists and books is absent (MusicBrainz identity not published for tracks). Manual whole-container merge is E | M | identity §9; review-identity §7; App. 0 | `AdoptProviderIdentityTx` called only from screen/film/show paths |
| A11 | Needs-attention lists and counts cover film/TV/anime only; music and audiobooks have no owner review queue | M | server-followups §9 | attention routes use the screen-metadata queue only |
| A12 | Webhook `title.updated` does not fire for container-only metadata changes (overview, artwork of a show or album) | S | api-platform §9; App. E | baseline trigger `webhook_title_updated AFTER UPDATE OF title,year` |
| A13 | Freshness hints still missing: the viewer Guide, DVR and Downloads screens use a 30-second clock plus `notification.changed`; `personal.changed`/`progress.changed` are library-scoped, not title-scoped; recommendation and trending rows have no hint | S–M | web-viewer §7.7; freshness-clients §7 | `Channels.tsx:21`, `DVR.tsx:17`, `Downloads.tsx:12`; baseline lines 6418–6426 |
| A14 | No route reads one access record by id (member, device or invitation); the web walks up to 32 list pages to re-send a change | S | web-owner §7.5 | none in the registry |
| A15 | A failed-attempt ledger per account (`identity_credential_attempts`) is only reset on success, never pruned | S | accounts-security §9.7; ledger B-01.18 | `cmd/server/maintenance.go:140` prunes only `identity_unknown_attempts` |
| A16 | **Rich optimized downloads:** prepared and download recipes are H.264 SDR / AAC stereo only. This is against "conversions keep HEVC, HDR, multichannel" | L | conversion-outputs; planner-wiring; App. 0 | no HEVC/HDR in `internal/preparedmedia` |
| A17 | **Live TV is TS-only:** no fMP4 live publication (FLAC/Opus/HEVC-in-fMP4 tuples), and no Automatic/mpv two-engine live selection. mpv refuses live channels on Apple and Android | L | planner-wiring; livetv; apple-mpv §9 | `linearbuffer/buffer.go:490` rejects `EXT-X-MAP` |
| A18 | HDR mastering-display and MaxCLL are never captured from sources, so conversions cannot carry them | M | planner-wiring; review-playback-pipeline | no mastering fields in assets/probefacts |
| A19 | Capability document depth: one limit per codec (no "4K30 but 1080p60"); 8-bit and 10-bit H.264 share one ceiling (server schema, client-core and native probes) | M | android-mpv §7; apple-mpv §7; App. E | `playback/capability_document.go` `ClientVideoCodec` has a single max size/rate |
| A20 | Live TV: guide extensions during an active capture are not adopted; very large playlist-only payloads are held as one bounded string | M | livetv §9; App. 0 | (not re-verified) |
| A21 | Matcher races: a new file's analysis still resets an in-flight episode fetch, and `dispatchScreenChildren` resets episodes without checking the lease | S | perf-followups §9 | (not re-verified) |
| A22 | Reads still growing that are **not** in cp2-reads' list: subtitle-language and title-substring filters; container card member counts (F12) and Home poster borrowing; Continue Listening (grows with profile history); arbitrary filtered/restricted group counts | M | perf-reads; scale-check-2; grouping-sorts; review-server-late | perf-reads "Not finished"; cp2-lanes.md item list |
| A23 | Cross-library facet counts are library-wide, not counts within the group | S–M | cp-browse-parity §9 | — |
| A24 | DVR owner notices are English server strings, not catalogue keys | S | cp-native-playback §9 | — |
| A25 | Sign-ins with no installation id under device approval each create a new waiting device record, with no cap | S | accounts-security §9.10 | `identity/issuing_device.go:57` (verify the HTTP paths) |

### Web

| # | What is missing | Size | Source | Evidence |
|---|---|---|---|---|
| W1 | Collection reorder: library collections arrange only the first 500; a personal collection re-reads from the first page after every move | S | cp-followups §9 | — |
| W2 | Saved lists that have been paged refresh only the current page (deep refresh exists only on Home row See all) | S | cp-polish §9; freshness-clients | — |
| W3 | `currentI18n()` helpers (about 37 calls) do not redraw on a language change | S | web-app §9 | — |
| W4 | Px font sizes remain in inline TSX styles (B-16.17 residual) | S | cp-polish §9 | `DeleteVersion.tsx:47`, `Logs.tsx:51`, `HostedChooser.tsx:151,159`, `LiveTV.tsx:145` |
| W5 | Drawn subtitles in the browser have no Korean fallback font | S | App. E | `player/subtitles/fallback-fonts.ts:18` ("Korean syllables … not in this font") |
| W6 | Bitmap subtitles: DVB character-coded and progressive objects are skipped; VobSub in-cue colour changes are ignored | M | web-subtitles §9; App. E | — |
| W7 | The join page ignores the invitation's `forPorticoAccount` (web and native) | S | accounts-security §7 | `access/invitations.go:64`; no client reads it |
| W8 | The 4K H.264 probe asks `avc1.640028` (level 4.0) at 3840×2160 and may under-declare 4K H.264 | S | vega.md §7.1 | `client-core/src/client-profile.ts:487,532` |
| W9 | About 200 unused catalogue strings, plus the named stale keys from console and web-owner, need a review pass | S | web-app §9; web-owner §7; console §7 | — |
| W10 | Skipped parts from tolerant parsing are recorded and never shown (an optional diagnostics row) | S | web-app §9 | `client-core/src/tolerant.ts:38` |
| W11 | Owner "Delete" dialog and other new React screens have no render tests (the web Reorder modes, the PiP removal, `usePlaybackChoice`) | M | cp-followups §9; cp-web-playback §9 | — |

### Native (phone/tablet/TV; Vega where it shares screens)

| # | What is missing | Size | Source | Evidence |
|---|---|---|---|---|
| N1 | **"Start the next episode during the credits" is offered in native Settings, but no native player honours it.** Native has no credits-time Up Next card or countdown (web has both) | M | final-pass-2 §3 | no `startDuringCredits` in `apps/native/src` |
| N2 | Audiobook speeds above 2× (2.5×, 3×) are not offered on native | S | web-subtitles §7 | `bridge/NativePlaybackSpeed.ts:3` stops at 2 |
| N3 | Native still shows "refresh required" notices where web now re-reads quietly | S–M | cp-web-playback (web only) | `Library.tsx:261`, `Show.tsx:351`, `Saved.tsx:276,356`, `Search.tsx`, `Entity.tsx`, `SavedPages.tsx` |
| N4 | The "Playing on your other devices" panel is web-only | S | App. 0 | `settings-structure.ts:341` `platforms: ['web']` |
| N5 | The owner's list of everyone's devices (People › Devices) is web-only; native lists devices per account | S | native-owner §9 | `settings-structure.ts:425`; `PeoplePanels.tsx:329` |
| N6 | Native invitations have no Portico-username field | S | accounts-security §7 | grep `porticoUsername` in native = 0 |
| N7 | Native version and subtitle menus do not use `versionNames`/`subtitleTrackNames`, so duplicate labels such as "English / File" twice remain | S | e2e-web §7 | used only in web `PlaybackChoices.tsx`/`Player.tsx` |
| N8 | Native still uses English tables (B-18.22 residual) | S | web-owner §4 | `tierLabels`, `scopeLabels`, `scopeHelp` (`panel-logic.ts:126`) |
| N9 | Native owner wording borrows the nearest key for: share/copy token, webhook Off, viewing-hours time help, dashboard free space | S | native-owner §7 | — |
| N10 | The in-player engine override can lose a 4-second republish race on a slow link | S | cp-followups §9 | — |
| N11 | Card fade-in skips large windowed grids and changes of more than 60 cards | M | native-screens §9; App. E | — |
| N12 | iOS: a background download whose transfer address changes restarts from zero. This is Apple ObjC, written here and proven on a device | M | downloads §9; App. 0 | — |
| N13 | Chapter thumbnails and trickplay images do not travel with downloads (server bundle plus native offline player) | M | downloads §9 | nothing in `internal/downloads` |
| N14 | Retire the old offline players `PorticoOfflinePlayerActivity` and the AVPlayerViewController path in `PorticoDownloads.m`. Signed-out downloads now use the common player; retire after device acceptance | S | native-screens §7 | still in `AndroidManifest.xml`, `PorticoDownloads.kt`, `PorticoDownloads.m` |
| N15 | Android mpv: software deinterlacing is not offered although the engine has bwdif/yadif; Dolby Vision profile 5 would need `vo=gpu-next`; Fire OS receiver/passthrough reporting reads only `AudioDeviceInfo`. Kotlin, written here, proven on a device | M | own-mpv §11; android-mpv §9 | `PorticoMpvPlaybackModule.kt:181` sets `softwareDeinterlace` false |
| N16 | Android mpv fallback fonts are chosen by language only: mixed-script or untagged lines can show boxes. libass is built without a system font provider | M | native-playback §9; App. E | `media-engine/lib/common.sh:347` |
| N17 | The Android download job does not survive a reboot, and does not use API 34 user-initiated transfer jobs | S | cp-native-playback §2,§9 | — |
| N18 | The tuner countdown uses the device clock with no skew correction | S | cp-native-playback §9 | — |
| N19 | `CastTransport` has no stable receiver id, so two TVs with the same name share one stored proof | S | native-sync §7; social §7; App. E | (not re-verified) |
| N20 | "Outside allowed hours" is shown only at launch and sign-in; a session already running sees ordinary failures | S | native-sync §9 | (not re-verified) |
| N21 | `PlaybackApiError` drops `reason`, so a too-slow quality never names the quality that fits (native always says "Try Automatic"). Shared client-core | S | native-playback §7.2 | `client-core/src/playback-v1/http.ts:17`; `PlayerOptions.tsx:265` reads `.reason` |
| N22 | `viewer-selection.ts`: unify retirement by session family in client-core (native guards it locally today) | S | review-native-js §7 | (not re-verified) |

### Cross-cutting

| # | What is missing | Size | Source |
|---|---|---|---|
| X1 | Test coverage gaps: no Kotlin unit tests; MCP adapter tests do not exercise all ~44 tools; downloads `tend()`/`recover()` are untested; no render tests for new native screens (separate from B-19.15) | M | android-mpv; review-api-platform §7; downloads §9 |
| X2 | Documents with known errors: Architecture `02 Server.md` (device keys), `ENGINE-CONTRACT.md` §7/§10, `06 Clients.md` (Vega, two engines), NAV-6.7 (live filters), Playback Protocol §5.5/§12, capped totals and matching rules, `web/README.md` and native bridge README (feed is no longer a long poll), **`THIRD-PARTY-NOTICES.md` still describes MPVKit and media-kit** (lines 118, 151) | M | many lanes' "Documents" sections |

### Trivial cleanups (XS each)

These come under the pre-release rule: delete legacy.
- **T1.** CORS still allows `X-Playback-Controller-Token` and `X-Portico-Device-Id`, which nothing reads, and `PorticoListeningOwner.m:63` still sends the token. Evidence: `router.go:1202`.
- **T2.** `gofmt` deviation in `playbackv1/admin.go`.
- **T3.** Two different hooks are both named `useQuietRefresh` (`web/src/app/freshness.tsx:137`, `web/src/app/quiet-refresh.ts:24`).
- **T4.** Stale `apps/native/src/screens/NOTES-search-saved.md` ("both are unwired").
- **T5.** Dead TV sparkline sizes in native `Dashboard.tsx:187`.
- **T6.** `DestinationPlatform`/`DevicePlatform` lack `'vega'` (Vega casts to `'firetv'`).
- **T7.** `web.connection.reconnectingBody` wording no longer fits its only case.
- **T8.** Delete-dialog strings (`deleteMedia.titlesCovered`, `.fileChanged`, `.fileNotRemoved`): confirm they are used, or remove them.
- **T9.** `paused-load.ts` and `HandoffSnapshot` wrapper: `PlaybackService.play` has no starting state. (not re-verified)

---

## B — platform, device or hardware dependent

1. Apple: compile, link and archive iPhone, iPad and Apple TV. Run `build-apple.sh` (own media engine) first. Add an XCTest target for `PorticoMpvTests.swift`. B-19.18 scene launch acceptance. Verify the iOS Pods `-ffile-prefix-map`. Generate the Apple pod, glslang and RN native-library licence texts (Mac step).
2. All Swift and ObjC written this programme is unbuilt: mpv controllers, fonts, `sub-forced-events-only`, ADTS AAC priming in `PorticoAudioRender.m`, Downloads.
3. Android: release build, other ABIs, Fire TV and Android TV flavours, real phones and boxes. Includes the JobScheduler download job, the Layout restart, offline fonts, MediaCodec, 16 KB pages, HDR direct output with mpv, passthrough and frame-rate matching (B-19.8 density).
4. Vega: SDK build, device sign-in, and redirect captures (301/302/303/307/308). HDR and surround probing is not possible through the SDK (see C12). The capability probe has never run on a device. Live channels are untested on a device.
5. Windows, macOS, Linux and Docker installs: install, upgrade, reboot, remove; SCM/launchd identity; ACL, UNC and NAS watchers; macOS FSEvents cgo bridge compile; Docker multicast.
6. GPUs, HDR displays, AV receivers and tuners: every conversion backend; tone mapping; passthrough; one tuner input with several viewers and private DVR; Windows custody of the tuner.
7. B-10.28: real HE-AAC/PS and physical gapless fixtures. The Core Audio decoder's 1024-frame priming is unproven against the server's ADTS AAC.
8. Browsers: Firefox, Safari and Edge. B-17.19 media keys and lock screen with the `<audio>` carrier. PiP. iPhone fullscreen. Screen readers, forced colours, font scaling, an old browser (Safari 15). Bluetooth output-latency figures.
9. Real-device obedience to owner and remote pause/resume/seek (`session.updated` on a player), and Watch Together first-frame silence (B-13.18) on mpv and native.
10. B-01.23: the historical Apple `explicit_signout` incident needs a correlated device trace.
11. B-14.14: real cleanup of a signed-out account's downloads on a device, and confirmation that its scope is device-wide.
12. B-25.10: Apple and Vega release CI with physical first-party acceptance (the workflow drafted by cp-polish has never run).
13. Hosted on an empty PostgreSQL staging database: public TLS endpoint, wake, providers, browser and OS callbacks, multiple replicas and cutover.
14. Real provider credentials: TMDB, OpenSubtitles, MusicBrainz match quality.
15. PGS end to end: needs a PGS sample or encoder. Canonical person grouping: needs provider-backed people data.
16. Capacity: 200 browsers and 100 streams, a full 1M read/add matrix, 2M mixed, SSE fan-out, years of retention churn.
17. Software-decode ceilings (estimates) need measuring on real devices.
18. Apple TV mpv is SDR only, with no PiP, AirPlay video, live channels or passthrough. Android libmpv has no S/PDIF passthrough. These are engine and platform output limits, to be proven or lifted on devices.
19. E-AC-3 encoding above 5.1 is unavailable in upstream FFmpeg (a library limit).
20. Frame-rate matching is impossible below Android 11 (Fire OS 6/7) and on tvOS 15–16 with mpv.
21. Unknown-duration early end of stream can look like a normal end (needs runtime evidence on engines).
22. Accessibility on devices: TalkBack, VoiceOver, TV remote focus, IME, density and font scaling.

## C — needs Justin

1. Landing: merge and push `audit-implementation/main` in all three repositories. **Deploy Hosted on an empty format-21 database** and keep the old binary and database for rollback.
2. Signing and release credentials:
   - `PORTICO_RELEASE_PUBLIC_KEY` and the private signer;
   - Windows and macOS installer signing and notarization;
   - Apple signing and stores;
   - the Android release key (release builds still use the debug key);
   - the Vega package (manual).
3. Licensing:
   - B-25.12, the Apple distribution-rights review;
   - publish the media-engine source bundle (about 133 MB) with each app release;
   - attach the libass-wasm corresponding source;
   - accept FriBidi (LGPL) and the `wasm-unsafe-eval` CSP allowance.
4. Approve a server FFmpeg toolchain rebuild for a path-neutral macOS bundle. This is a manual-approval policy.
5. Own media-engine choices:
   - size: all decoders at `-O3` (29 MB) versus `--enable-small` or a curated decoder list;
   - a neutral build work directory;
   - whether self-signed owner certificates are trusted app-wide.
6. Cast with styled or image subtitles is burn-in only; there is no libass on the receiver. This is not answered in the record.
7. "No email" versus Hosted: Hosted still has an SMTP outbox (`hosted/internal/mailoutbox`: registration, OIDC contact, password reset, email change, invitation). Confirm that transactional account mail is allowed.
8. **Dropped by Justin 15 Oct** (no longer open): optional expansions (S3): spoiler controls, writing NFO and artwork beside the media, country rating tables, per-library unrated and kids allow-list mode. Also whether to unify profile label rules with account label/tag rules (accounts decisions 22/23).
9. Local-metadata people (NFO credits without a provider identity) have no person page. This is a product call.
10. There is no online audiobook metadata provider, and disc folders (VOB, BDMV) are shown but cannot be played. Product calls.
11. Third-party browser tools on other origins work only through the `PORTICO_ALLOWED_ORIGINS` environment variable. Decide whether owners get a setting.
12. Vega cannot detect HDR or surround. Keep SDR and stereo, or add a manual viewer setting.
13. A bulk empty-build rebuild path (the next large rebuild gain, a redesign). You said further performance work comes later.
14. Device state kept as it is; please confirm (cp-followups §6): volume and mute, PiP window position, web grid/list and poster size, the Android Layout setting.
15. Recorded defaults never confirmed (owner's report §3, Appendix 0 and E; items already superseded are left out):
    - missing titles are kept for 30 days;
    - an unproducible chosen quality is refused;
    - conservative matching thresholds: accept at 0.78 with a 0.07 margin; below 0.90 goes to attention;
    - Apple allows plain HTTP app-wide, not per address;
    - member deletion and remembered-account management refuse API keys;
    - DVR recordings are stored once;
    - the Home row toolbar is capped at 500;
    - under Keep watching, a recording starts late as a partial recording when a tuner frees up;
    - retention ceilings have no owner control;
    - multipart titles cannot be downloaded as one;
    - TV/anime split goes through episode placement, not the generic Separate.
16. Website: a page behind `SECURE_CONNECTION_HELP_URL` that explains how to turn on a secure connection.

The other open questions in the cp reports are answered in the record (credits, PiP, deep resume, player choices, collections, administrator scope).

## D — deliberately excluded by Justin's decisions

1. B-19.15: replacing the React-internals hook test driver. Justin, 15 Oct: skip it.
2. Plugins, themes, custom CSS (§1 product positions).
3. Push notifications and email from the server (§1). Hosted mail is C7.
4. Light theme, CarPlay, Android Auto, Roku, sub-path hosting, a relay (§1 Scope and Running the server).
5. Watch-state import and export (§1 Library).
6. Watch Together and casting redesign beyond defect fixes, for example libass on the Cast receiver (§1 Scope).
7. TV owner administration beyond the read-only Dashboard, including the removed TV Stop, Acknowledge and task actions (Justin, 14 Oct).
8. Vega pre-follow redirect refusal; residual risk accepted (B-22.19; Justin, 14 Oct).
9. Restore ending API keys; it does not (Justin, 14 Oct).
10. Per-title memory of in-player choices and "Forget last choice" (removed, 15 Oct). The FLAC deep start was replaced by the seek index.
11. Subtitles in PiP (15 Oct). The "Progress" sort on My List and Favorites was dropped (15 Oct). No Reorder on TV, because TV does not edit collections.
12. Security trade-offs: light password rules, no lockout, nothing encrypted at rest, a TV code confers the approver's role, plain HTTP from anywhere, and under "owner only" a correct member password is distinguishable (§1).
13. An MCP enable switch (none; one API and no wall of switches).
14. Seek indexes for files measured before the change are not re-measured. This is moot under the pre-release fresh baseline.

## E — in progress (cp2 lanes, see `briefs/cp2-lanes.md`)

**cp2-viewer**
- E1. B-18.27 remainder: library and saved pins on web, native, TV and Vega.
- E2. Whole-group playback for arcs, ranges and non-season groups, and for Home shelf rows (server snapshot); sorted Home row play.
- E3. Cross-library letter rail.
- E4. History across libraries beyond 10,000.
- E5. Music Playlists tab: full sort and filter.

**cp2-native**
- E6. Remove an entry from a personal collection, with Undo (B-18.29 native).
- E7. Sorted My List, Favorites and History draw cards by kind.
- E8. Forced only only for mixed image tracks, and verify our mpv honours it.
- E9. Drag handles for collection reorder.
- E10. Seek-index exact resume in Android and Apple audio engines.
- E11. Forgot PIN on native.

**cp2-server**
- E12. Merge whole albums, books and artists, with split and undo, on web and phone/tablet.
- E13. Silent retry for WebDAV commands.
- E14. B-13.10 Watch Together room-ceiling benchmark and fix.
- E15. B-04.35 Complete-scan shared decode.

**cp2-reads**
- E16. Broad multi-word search.
- E17. Per-person credit counts.
- E18. Duration and rating ascending first page.
- E19. Deep pages in fact sorts.
- E20. `HomeRows`.
- E21. Indexes for versions, duration, resolution, credit names and genre presence.
- E22. Search by year and date added.
- E23. Restricted Home.

**final-pass-3:** verification after the cp-followups merge.

---

## Done — previously listed as open, verified closed (brief, for audit)

**Identity and accounts**
- Owner privacy sentence on web (`AccountPanels.tsx:43`) and native (`SettingsPage.tsx:247`).
- `outside_allowed_hours` handled on native (`connection-status.ts`, `sign-in-ended.ts`).
- Member deletion preview and `disabledReason` on web and native (`People.tsx`, `PeoplePanels.tsx`).
- Rating-system picker on web and native.
- `removedPerson` in play history on web and native.
- `device_approval_pending` handled (`device-approval.ts`, `sign-in-errors.ts`).
- B-01.22: no `Authority=="hosted"` branches remain. B-01.21 per App. D.

**API and contracts**
- Hand-written OpenAPI YAMLs deleted (`server/api` holds only `portico.openapi.json`).
- Optional consumer declarations retired.
- Legacy paths gone: `legacy-bridge.ts`, `playback-memory`, `deepStart`, commands and transfers literals.
- MCP needs-attention tools; MCP `control_playback` falls back to administrator transport.
- `durationMs` on controls, failure title identity, tuner names, per-source import limits, OpenSubtitles credentials UI on web and native.
- `list.changed` hints for owner lists; Watch Together on the hub.
- B-18.9 administrator role. B-08.12 named-ETag check (adoption is A3).

**Playback**
- B-21.8, B-21.9, B-21.11; subtitle PUT retired.
- B-11.27 tuner pre-emption setting and countdown.
- B-10.11 Apple ADTS AAC; B-14.20 JobScheduler; B-14.21 receipt clock.
- B-19.10 Layout setting; B-19.19.
- Offline fonts; offline next episode; chapter pictures; Cast subtitle choice from native.
- Native Play next and Add to queue on the title page.
- Credits preference (web), PiP rule, seek index (web), player-choices rule, collections manual order (web and phone/tablet).
- Dolby Vision refusal wording gone. A live tab leaves on `pagehide`.

**Browse**
- B-07.10 `sortLetter`; B-18.23 `discNumber`.
- Cross-library `/v1/browse`; `groupOrder` kept under filters.
- Toolbars on Home-row, personal-collection, book-part, disc and episode-group pages; Cast/Crew on person pages; group memory.
- Native My List, Favorites and History toolbars. 10,000+ totals on every client.

**Web and native polish**
- B-16.4, B-16.5, B-16.7, B-16.10 (salvage on reviewed paths), B-16.12, B-16.14, B-16.17 (except W4), B-16.23, B-16.25.
- B-25.21 lock; B-15.19 advisory; B-18.22 (except N8); B-18.30; B-17.19 code path.
- Native `new-words.ts`/`player-words.ts` deleted; Vega transport strings are in the canonical catalogue.

**Native owner tools**
- Playback problems, Watching-now transport with Move to…, episode order and offset, merge of songs, files and extras.
- Paged lists refresh quietly. Shared phone page declarations adopted (adapters deleted).
- Forget remembered devices on web and native. Counts-as-started and Include-me-in-Watch-Together rows.

**Platform**
- Android Cast module exists (google flavour, `PorticoCast.kt`).
- Windows Live TV physical-lock and artifact-lease adapters exist (`livechannels/physical_windows.go`, `artifactlease/*_windows.go`).
- Own media engine built and verified for Android.
- On-screen licence notices on web, phone/tablet, TV and Vega.
- final-pass-2 had every Ubuntu suite and both end-to-end drivers passing at a4f86250 / 01b78bd plus fixes.

**Owner's report §4**
- Every item is closed except those in A, B, C or E above: native fade (N11), iOS partial transfer (N12), Apple TV and Vega output limits (B18, C12), accessibility (B22), the 38 bugs (cp-closed or listed above).
