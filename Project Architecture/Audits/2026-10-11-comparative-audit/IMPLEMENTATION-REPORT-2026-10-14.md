# Portico implementation report — 14 October 2026

This is the owner's summary of the implementation programme that followed the comparative audit. Sections 1–6 are the short version. The appendices hold the per-decision detail, every lane's record and all 588 audit bug dispositions.

## 1. Where things stand

- **Every decision you made is built in source.** Some client gaps remain; they are listed in section 4. Of the audit's 588 bugs, 359 are fixed, 173 were already fixed, 7 were not present and 11 are intentional under your decisions. 38 remain open: none High, 20 Medium and 18 Low.
- **Independent reviews of the new work found 137 problems, and all of them are fixed.** That includes 1 blocker and 42 High.
- **Everything passes on Ubuntu after the final pass:**
  - **Server:** 87 Go packages.
  - **Shared and web:** 1,947 shared-client tests, 546 web tests and 20 Cast tests.
  - **Native:** 660 native tests and 87 Vega tests, with type checks and string lints.
  - **Builds:** the production web build, plus Windows and macOS cross-builds.
  - **End to end:** the real-server driver passes 18 of 18 groups, and the headless-Chrome driver passes 13 of 13 groups with 114 screens rendered.
- **It is not ready to release.** Nothing has been compiled for Apple; the Swift and Objective-C code is unbuilt. Nothing has been built as a full Android application or run on any phone, TV, Fire TV or Vega device. Nothing has been installed on Windows, macOS or Docker. No real GPU, receiver or tuner has been used.
- **Nothing was pushed, merged into your own branches or deployed.** All work sits on `audit-implementation/main` in each repository (section 6). Hosted was not redeployed, because it needs an empty database (section 3).

## 2. Performance

You asked for per-title cost to stay roughly constant as a library grows, so that 50,000 and 5 million titles both behave well. That now holds for adding, scanning, everyday actions and the rebuild. A few reads still grow, as listed at the end of this section. The figures come from single runs of the synthetic-library benchmark on this Ubuntu host, which was busy with other work at the time.

| Area | Before | After |
|---|---|---|
| Adding one new file to a 12,000-title library | 74.8 s; re-derived 75,041 rows | 13 s; re-derives 34 rows |
| Adding a film, 50,000 → 200,000 titles | — | 11.28 → 11.22 ms (flat) |
| Progress updates, marking 500 watched, removing a library | grew with size | flat from 5,000 to 200,000 |
| Background queue claim | grew with queue | constant 0.4 ms |
| Full rebuild of 200,000 titles from empty | 44 min | about 30 min |
| Rebuild speed, 200,000 → 1 million titles | fell 17% (198 → 165 work units/s) | flat (693 → 714 units/s) |
| One show's metadata update | 257 ms | 11 ms |
| Worst artist metadata update | 8.1 s | 7.8 ms |
| Longest database write lock | about 2 s | 0.3–0.7 s |
| Requests over 1 s at 200,000 titles | 69 | 4 |
| Requests over 500 ms at 200,000 titles | 327 | 57 |
| Broad search at 200,000 titles (median) | 858 ms | 14 ms |
| Cold Home at 10,000 titles | 14.5 s | 0.1 s |

- **Large totals are exact up to 10,000 and then read "10,000+".** Grids keep loading past that point and settle on the exact count at the end. This now works on web, phone, tablet, TV and Vega.
- **Scanning never waits on metadata providers.** New titles are matched first, metadata fills in afterwards within each provider's rate limit, and new episodes no longer re-match their show.
- **The million-title runs were stopped early,** as you asked. The rebuild ran for 12–15 minutes (about 14% of the way) to read its rate. Reads and adds were not measured at a million, because both need a finished rebuild.
- **Still growing with size; reported, not fixed:**
  - Broad multi-word search prefixes (up to 258 ms at 200,000).
  - Per-person credit counts in the People group.
  - Songs or episodes sorted ascending by duration or rating (70 ms at 200,000).
  - Deep pages in some sort orders. You accepted this, since real views rarely hold 200,000 titles.
  - Home composed in one call. The apps no longer use that path; they load each row separately.
- **The next large rebuild gain would need a separate bulk path for building from empty.** That is a redesign and was not attempted.

## 3. Things you should know, and choices to confirm

These were decided, or arose, while you were away. Each one has an alternative in Appendix 0 or E if you would rather it went the other way.

- **The TV owner Dashboard is read-only, following your answer.** As a result, a TV no longer offers the owner Stop playback, Acknowledge or task actions it had before.
- **Licences to review before distribution:**
  - Browser subtitle rendering uses libass compiled to WebAssembly. It includes FriBidi, which is LGPL, and needs the `wasm-unsafe-eval` allowance in the web content policy.
  - Apple's mpv engine is the LGPL MPVKit build. Checked against MPVKit's build script at the pinned `1.1.0-n9.0.2`: its FFmpeg is configured with `--enable-version3` (so LGPL version 3, not 2.1) and linked statically, and it bundles GMP, Nettle and GnuTLS. LGPL version 3 plus static linking is the hardest combination for the App Store; building our own FFmpeg and mpv as LGPL 2.1 dynamic frameworks (no `--enable-version3`, no GMP, Apple's own TLS) would remove it.
  - Android's libmpv contains LGPL version 3 FFmpeg.
- **Apple apps allow plain HTTP to any address.** This follows "HTTP from anywhere", but it is one broad app-wide setting rather than a narrower per-address rule.
- **Vega's sign-in works, with the redirect risk you accepted.** If a server redirects, the SDK may send credentials before Portico can detect it. Portico then rejects the response, signs out and tries to revoke the session.
- **Hosted must start on an empty database.** Accounts, claims and the directory are recreated rather than migrated. Keep the old binary and database for rollback.
- **Defaults I chose:**
  - Missing titles are kept for 30 days before emptying.
  - A requested quality the server cannot produce is refused rather than silently raised.
  - Matching is conservative: uncertain matches go to the attention list rather than being joined.
  - A group's own order (playlist position, episode order) gives way to a normal sort once you filter.
  - Lists without a server change signal refresh quietly every 30 seconds.

## 4. What is not finished

- **Sort and filter everywhere:**
  - Home row "See all", personal collections and a book's parts still lack the full toolbar; each needs a small server field.
  - Native My List, Favorites, History and music Playlists have narrower controls.
  - Cross-library lists sort one library at a time.
- **Play all and Shuffle across a whole view:** the server supports it, but the screens still play only the entries already loaded, and saved views can stop at 500.
- **Native owner tools:**
  - Missing: playback problems, Watching-now pause and seek, and episode-order controls.
  - Some owner lists refresh only when pulled.
- **Web owner:**
  - Channel-builder saves and mount commands can still show a conflict.
  - Subtitle-provider credentials have no screen.
- **Playback edges:**
  - Offline styled-subtitle fonts are passed to mpv but ignored for local files.
  - There is no offline next-episode queue.
  - Apple TV mpv is SDR only; Vega is SDR and stereo.
  - Web drawn subtitles don't follow picture-in-picture or casting.
- **Accessibility and translation:** screen readers, remote focus and font scaling are untested. Some new native strings are still English.
- **Audit bugs:** the 38 open entries are listed in Appendix D.

## 5. Testing: done, and still to do

**Done on Ubuntu:** everything in section 1, plus the performance runs above.
- **Server driver:** runs a real server with generated media. It covers playback, conversion, subtitles, keys, webhooks, MCP, events, deletion, restore and restart.
- **Browser driver:** runs headless Chrome. It covers Original and fixed-quality playback, painted text, ASS and PGS subtitles, downloads, quiet updates and owner pages at desktop and phone widths.

**Still to do (the [test plans](test-plans/README.md) hold 250 step-by-step cases):**
1. Compile and archive the Apple apps on the Mac, then run them on iPhone, iPad and Apple TV.
2. Build the Android, Android TV and Fire TV apps on Ubuntu, and Vega with its SDK, then run them on devices.
3. Install the server on Windows, macOS and Docker, and run upgrade, restart and restore.
4. Test with real media hardware: HDR, surround receivers, GPUs and a tuner.
5. Try Firefox, Safari and Edge, and do a human walk-through of the web app.
6. Stage Hosted on an empty PostgreSQL database.

Keep conversion disabled on the demo server, and do capacity tests on separate hardware.

## 6. How to land it

| Repository | Programme branch | Head |
|---|---|---|
| portico-server | `audit-implementation/main` (worktree `main-server`) | `f719c2a0` |
| portico-react-native | `audit-implementation/main` (worktree `main-native`) | `8c8b973` |
| portico-internal | `audit-implementation/main` (worktree `main-internal`) | the commit containing this report |

The worktrees are under `/home/justin/projects/Portico/.worktrees/audit-implementation/`. Your own checkouts are untouched.

Recommended order:
1. Land the server first; it holds the shared packages and contracts.
2. Land native at the matching package revision. Its shared packages are byte-identical to the server's now.
3. Land internal (Hosted and docs) last.

The server uses one fresh baseline database, so start each test server from empty state and keep old binaries with their old databases. Lane reports and the running log are in `_program/` in the same folder; archive them with the internal handoff if the worktrees move.

---

## Appendix 0. Detailed record from the lanes

This section was written by the integration lanes before the final performance and test pass. Its test counts and scale figures are superseded by sections 1 and 2; its per-decision table, choices with alternatives and gap lists remain accurate.

### What was built against your decisions

The table follows your decisions. Later owning-lane receipts supersede early gap lists. “Built” means implemented source with the checks stated alongside it.

| Your decision | What now exists | Clients and remaining limits | Evidence |
|---|---|---|---|
| Titles separate from files; retained personal identity | Stable titles, versions, merge/split, physical version deletion, missing state, 30-day/immediate/never emptying and retained provider state | Web controls; phone/tablet remove-library, merge/split and version tools now built. Native merge excludes songs, audiobook files and extras. General audio-container/provider merge remains limited | Server suite and real-server lifecycle/restore checks; native rules/type checks; filesystem/platform acceptance remains |
| Tolerant names and metadata; file wins | Bounded filename/folder/tag reader, misspellings/yearless confidence, episode/season variants, ignores, extras, tolerant sidecars/subtrees and owner skipped-data diagnostics | Web and native attention/library tools; episode order/offset on web. No new online book provider or disc playback | Naming/provider/scan regressions; generated-media real-server scans; live provider and macOS watcher bridge unqualified |
| Owner chooses provider, TMDB/local-only | Per-library agents, provider credentials, pace, attention suggestions/search/ID/dismiss, artwork quota | Web and phone/tablet metadata panels. Native attention lists require pull-to-refresh except searches/actions | Fixtures and integrated suites; native 50 new owner tests; real provider credentials untested |
| Native primary plus Automatic/Native/mpv; one capability document | Shared controller, exact engine/output facts, native direct/remux preference, mpv avoiding costly conversion, same-session replan after engine setting change | Apple/Android/Fire source; Vega uses its own engine. Known Apple TV/Android mpv output limits remain | Server/shared/native tests and narrow Android compiler/linker checks; no Apple compile/full Android build/device run |
| Original default; Automatic opt-in | Saved home/remote defaults Original; Automatic starts Original even with throughput supplied, then requests steps; fixed quality is a ceiling | Web/native settings and session menus; direct-file adaptation mainly observes stalls | Final suite and Chrome Original/480p playback; real adaptive connection/device acceptance remains |
| HEVC/HDR/surround conversion and tone mapping | Qualified rich picture/audio targets, plain remux, independent fMP4, software/hardware fallback, confinement and bounds | Platform claims determine output. E-AC-3 encode at most 5.1; optimized downloads still H.264 SDR/stereo; live remains MPEG-TS | Generated FFmpeg/decode tests and real-server conversion; no GPU/display/receiver acceptance |
| Styled/image subtitles on device | Tolerant text, original ASS/PGS/VobSub/DVB/fonts, browser libass/bitmap workers, native mpv paths, high-quality burn-in | Chrome paints SRT/ASS/PGS. Web choices use session PATCH. Native standalone subtitle mutation remains. Offline fonts are passed but engines ignore local font paths; iPhone web fullscreen/PiP/casting have limits | Chrome pixels/cues and parser/worker tests; genuine DVD/DVB corpus, Safari/device composition still need acceptance |
| Local API, keys, tokens, webhooks and MCP | Four scopes with live permissions, signed bounded webhook delivery, title query, changed paths and MCP; generated typed routes | Web and phone/tablet integrations. No Hosted dependency for local integrations; session-only account operations still refuse keys | Registry/scope tests, real-server key/MCP/webhook/SSE checks; Hosted toolkit refreshed and generated contracts checked here |
| No pre-release compatibility | One server baseline; retired API documents/routes deleted; generated consumers mirrored | Twelve optional consumer declarations still do not imply backend handlers. Native shared structure adapters remain for several screens | Final server suites; native type checks after final mirror; later running log records 655/87 passing after the mirror and exact package equality |
| HTTP anywhere; owner networks define locality | HTTP/TLS, IPv4/6, hostname proxies, explicit certificate/route settings and console reserve | Apple arbitrary HTTP loads enabled. Vega HTTP sign-in restored; exact-origin binding and redirect containment use accepted fallback | Ubuntu listener/proxy tests and Windows/macOS cross-builds; actual topology/platform acceptance remains |
| Browse/search always answer; quiet freshness | Retained browse/FTS generations, bounded checkpoints and split counters. Quiet web reads preserve cards/forms. Deferred Home manifest and independent rows are written in the scale lane | Some web viewer/owner lists reread every 30 seconds because server hints do not exist. Native paged keys/tokens/reports/attention lists do not quietly refresh. Plain-HTTP tabs still have separate long polls | Final server/Chrome drivers; simulated hub checks; scale Home changes still await final integration |
| Same wide sort/filter in every grouping | Web/native library tabs, genres/categories, collections, authors, artist/album songs, seasons, people, playlists and saved views use scoped browse; Recently aired has a changeable default | Web Genres/Series page past 200. Cross-library groups select one library at a time. Home rows, personal collections, book parts/discs and several native saved lists lack fields/controls; own order is unavailable under a filter | Scoped SQL/toolbar tests, Chrome group checks and native rule/type checks; physical focus and full parity remain |
| Real Undo and useful controls | Receipt-based container Undo on pages/card menus; native Play next/Add to queue; card size/backdrops/captions; paused Together start and Play on controls | Native fade skips large windowed grids and >60-card changes. Play on uses reported position, not a local extrapolation. Saved-resource removal still lacks Undo | Focused rules/suites; web card Undo in Chrome; native/two-player device checks remain |
| Reachable owner console, phone/tablet tools, TV read-only | Background speed/pause, resources/recovery; web Watching now pause/resume/relative seek, playback-problems Forget, viewing hours, episode order and HDHomeRun guide field; native N1–N5 panels | TV Dashboard actions removed as requested. Native playback problems, owner Watching now pause/resume/seek and other-device owner panel not built; owner transport against an actual player not verified | Final web suite/driver, owner screenshots at two widths; native owner rules/type checks only |
| Full music/books/Live TV/DVR/download scope | Broad own tags, lyrics/chapters, own listening engines, shared tuner input/private viewers, private shared DVR bytes, durable offline context/history | Apple limited-AAC target, Android dead-process download recovery, trusted receipt clock, offline next-episode queue, rich live/output and native Cast gaps remain | Audio/guide/sharing/download tests; generated real-server audio; no physical gapless/tuner/NAS/Cast acceptance |
| Four server platforms | Supervisor/restart, SCM/macOS/Linux/Docker recipes, detached restore/backups and storage status | Signer/notarization/artifact rights remain operator/release work | Linux runtime fixtures, installer fixture compile and cross-builds; no Windows/macOS/Docker installation or Apple application compile |

Your exclusions stand: no plugins, themes, custom CSS, push notifications, email, light theme, watch-state import/export, CarPlay, Android Auto, sub-path hosting or relay. Roku is outside this programme.

### Choices made on your behalf

#### Your answers are now settled

- **Phones and tablets get owner administration; TVs do not.** TVs have a read-only owner Dashboard. The removed TV Stop/Acknowledge/task actions follow that decision.
- **Vega's redirect risk is accepted.** Credentials are tied to the exact origin; redirected responses are rejected, affected sessions/media stop, saved sign-in is retired and bounded issuer-only revocation is attempted. The SDK may disclose a password/token/grant before detection; a chain returning to the original origin may be invisible. Revocation is an attempt, not a guarantee. This is no longer an open owner question or blanket sign-in blocker.
- **Restore does not end API keys.** Interactive sessions end. Keys absent at the backup point do not exist afterwards; an older backup can restore a subsequently revoked key. Review the restored inventory. Password-change survival was also directly instructed earlier.
- **Every grouping must offer the same wide sorting/filtering.** Title-only exceptions are not approved. The missing fields/controls in section 4 remain work to do.

#### Remaining choices to confirm or overrule

- **Missing titles default to 30 days.** Immediate, another interval and never remain available. Unreachable roots are protected. Alternative: keep everything until manually emptied.
- **Unproducible selected quality is refused.** The server does not serve Original above a chosen ceiling. Alternative: explicitly report and offer a higher-quality fallback.
- **Library identity is conservative.** Unmatched show folders remain separate; confident provider identity or owner merge joins them. Matching accepts at 0.78 with 0.07 margin; below 0.90 stays in attention. Defaults are en-US/US. Alternative: more aggressive identity joining or adjusted corpus thresholds.
- **Browser ASS uses vendored libass WebAssembly 4.2.4.** It includes LGPL FriBidi and requires the specific WebAssembly CSP allowance. A fresh nonce permits the dialog scroll-lock stylesheet without broad inline-style permission. Alternative: qualify an own bundle/render path. Distribution materials remain to be reviewed.
- **Apple uses the LGPL MPVKit product; Android libmpv contains LGPL version-3 FFmpeg.** Apple TV mpv is SDR and has no PiP/AirPlay/live/music/passthrough. Android lacks passthrough/deinterlace/profile-5 reshaping. Alternative: qualify different native builds and their distribution materials.
- **Apple allows arbitrary HTTP loads.** This follows HTTP-from-anywhere but is a broad application policy. Alternative: a narrower per-address bridge; no such bridge was built.
- **Member deletion and remembered-account management refuse keys.** These require interactive session authority. Alternative: add explicit scoped account/destructive automation.
- **DVR is private per profile and stored once.** Owners retain separate records/hard links to shared capture bytes. Alternative: duplicate physical recordings.
- **Hosted deploys to empty format-21 state.** Old accounts/claims/directory state must be recreated. Alternative: authorize a deliberate migration project.
- **Group defaults preserve authored order only until a filter/sort is chosen.** Filtering falls back to a server sort; playlist position, collection order, episode order and album-type order have no server sort. Alternative: add those orders. A category link becomes an ordinary removable library filter. Person Cast/Crew cannot retain the toolbar because the person predicate has no role operand.
- **Cross-library controls operate per library.** Web offers library chips; native chooses a library, initially the one holding most members. Alternative: add a bounded cross-library surface. Web group choices live in the address; native choices live with the open screen; neither is remembered between group visits.
- **Quiet web lists use a 30-second clock when no hint exists.** Reads stop when hidden, preserve content and stay silent on failure. Expensive disk usage is not polled. Alternative: emit profile/admin hints for each list family. Native paged owner lists still require pull-to-refresh; that is a gap against freshness, not approval of an exception.
- **Watching now seeks by 30-second steps and Start over.** The admin control document has no duration, so no scrubber is shown and Forward is not capped to a known end. Native Play on displays the last reported position and obtains length from the title. Alternative: publish authoritative duration on control documents.
- **Playback problems group by file.** Forget is per source; the server provides no title/name link. Native has no equivalent panel. Alternative: add item identity/title to the rows and the native panel.
- **Native owner secrets/reports use the share sheet.** Time windows accept typed times; merge finds a second title by name and applies only to film/show/episode. Last-write-wins webhook edits have no revision. Alternative: stronger webhook revision fences and different native controls.
- **Queue capture runs in bounded background work.** Playback starts from the first captured window. If a changed source loses its old read snapshot, the queue keeps its accepted prefix and fails that segment rather than mixing memberships. Alternative: a fully materialized queue before playback, with slower start and higher cost. Screens still need selector forwarding.
- **Retention adds compiled resource ceilings and bounded cleanup.** Gentle/Fast uses smaller/larger quanta; history kept forever and titles kept never-empty retain those choices. New records may be refused at a ceiling; exact replay/update remains valid. Alternative: owner-adjustable ceilings after capacity qualification.
- **Scale Home begins with a row manifest.** Two previews load concurrently and independently; Recently Added examines at most 4,096 candidates and recommendation descriptors keep 64 traits. Complete browse remains. Alternative: serial full Home or larger candidate limits after measurement. This scale-lane change has not received the final integration/health pass yet.

Appendix E keeps the other implementation choices and alternatives. These choices do not waive the source/platform gaps below.

### What is not finished

#### Screens and consumers

- **Browse fields:** Home recommendation See all, personal collections, book parts/discs and non-season episode groups lack an appropriate field/pivot. Native My List/Favorites keep their narrower list controls; History and music Playlists lack the wide toolbar. Cross-library sorts need a cross-library surface. A group's own order cannot be combined with a filter. Person roles have no browse operand.
- **Whole-view queues:** the server/core query/saved selectors are implemented, but web/native/TV/Vega Play and Shuffle handlers still need forwarding. Native sorted lists play loaded entries and saved views can stop at 500. Duplicate playlist entries still need an occurrence/revision anchor.
- **Native owner freshness and parity:** paged keys, tokens, reports and attention lists reread on pull/action, not quietly. Shared phone page declarations still need canonical adoption so local adapters can go. Download usage shows only its first profile page. Native playback-problems, owner Watching now pause/resume/seek, episode-order controls and the other-device owner panel remain absent; native merge does not cover songs/book files/extras. TVs correctly have no broad administration.
- **Web owner gaps:** Library Channel builder saves and mount commands can still show a conflict. Per-source import-limit readback, tuner person names and subtitle-provider credential UI remain. Admin session duration and playback-failure title identity are absent. Real receiver obedience to owner/remote pause/seek is unverified.
- **Viewer gaps:** local-metadata people still have credits but no canonical person page; saved-resource entry removal Undo, direct-visit episode air dates/guest/director/writer links, authoritative letter anchors, compressed scrolling/memoized cells/deep-list refresh, web album disc headings still parsed from English. Exact native pre-play subtitle resource/discovery identity, remembered-choice clear controls and chapter/trickplay imagery remain.
- **Offline/platform consumers:** local ASS fonts are ignored by both mpv engines despite being passed; no offline next-episode queue. Signed-out downloads now use the common player but await actual acceptance. iOS changed-address partial transfer and Android process-dead recovery/trusted receipt clock remain. Native fade skips large windowed grids.
- **Web playback/navigation gaps:** drawn subtitles do not follow PiP/casting; no forced-only control; Up next lacks artwork/countdown; MP3/ADTS/Ogg deep resume and output-latency compensation remain. Saved lists/search/person pages are not retained after Back, Watch Together is not on the freshness hub and five refresh-required notices remain.
- **Accessibility and strings:** remote/VoiceOver/TalkBack/IME/density/font scaling, browser floor, hook enforcement and Node build-version alignment remain. Web fallback metadata labels, shared SubtitleService/queue-player messages and native owner job/alert tables retain English. Exact native share/merge/Undo/time-help/dashboard and Vega transport keys await canonical catalogue adoption.

#### Server and shared contracts

- Native listening still needs the Apple bitrate-limited AAC target. Rich optimized downloads, live Automatic/mpv/fMP4 publication and complete HDR mastering facts remain incomplete.
- Standalone subtitle PUT remains for native/service/reservation/social consumers. Web is off it; complete coordinated session-PATCH conversion before deleting the duplicate route. Management/original/font/grant reads remain.
- General audio/provider merge and Saved-resource collection search remain absent. Account label/tag restrictions still differ from profile labels; country-wide rating coverage and per-library unrated/kids/spoiler/export proposals were not all approved or built.
- Owner/viewer list events are missing, as are cross-library/group-order/book-part fields above. Optional generated consumer declarations still need implementation or retirement according to scope.
- Scale-check-2 continues: final 10k all-operator measurements complete, final 200k and one-million measurements pending. Its latest source has deferred Home, grouped read/search and batched state optimizations, but awaits final snapshot/integration. Arbitrary cold exact totals remain proportional to matching content. A full two-million mixed fixture exceeds the 8 GB budget; two-million structural tests are a different result.
- Original playback/history/multipart/merge continuity, artwork pruning, container webhook coverage, guide-window extension and long-lived capacity/retention remain qualified gaps. New table ceilings are source-tested, not years of production churn.

#### Platform and release acceptance

- No Apple application compile/link/archive, real Android APK build/install, Vega package/device acceptance, Windows/macOS runtime, Docker lifecycle, GPU/HDR/output receiver, real tuner/NAS or physical Cast synchronization has been performed for this programme.
- Vega is functional in source with accepted redirect risk; HDR/surround/mpv/downloads/discovery/Cast/home rows and unsupported audio effects remain hidden or absent. Apple TV and Android mpv limitations remain as described above.
- Native suites were rerun after the last package mirror; rerun them after any subsequent scale/contracts merge. Exact dependency locks, native/WASM source/notices/relinking materials, Apple distribution rights, signing/notarization and the reported source-map-js advisory remain release work.
- The historical Apple explicit_signout incident still has no established cause or correlated runtime trace. No speculative cause is claimed fixed.

### What to test, and where

The [test plans](test-plans/README.md) now include the afternoon work. They are instructions, not executed acceptance. Each platform must build/install before its functional results count.

| Environment | Completed evidence | Still required |
|---|---|---|
| Ubuntu server | Final 86-package suite and 18-group real-server driver; generated files, auth/restrictions, queue/planning, keys/MCP/webhooks/events, missing/deletion/restore/restart | Representative large libraries, full queue screen forwarding, years of churn and real filesystem/provider behavior |
| Chrome on Ubuntu | 13 groups, 57 screens at 1440×1000 and 390×844; Original/fixed playback, SRT/ASS/PGS pixels, downloads and quiet version refresh; owner screenshots | Firefox/Safari/Edge, accessibility/media keys/PiP/AirPlay, real owner-to-player controls, populated failure/provider panels and remaining browse scopes |
| Native Apple/Android/TV/Vega | 655 native and 87 Vega tests before the mirror in native-owner; the later running log confirms both suites again after the mirror, with type checks and package equality. Touched native owner ESLint retains an existing unused variable; no claim of all lint systems clean | Post-mirror suites, actual application builds, screens/focus/sheets, common signed-out player, local fonts, remote/owner controls and lifecycle |
| Vega SDK/device | Exact-origin/redirect containment and sign-in/stream bridge tests | Actual password/code/refresh/media paths and synthetic-secret 301/302/303/307/308 captures. Accept exposure-before-detection risk; require rejection/sign-out/revocation attempt, not zero captured secrets |
| Hosted isolated PostgreSQL | Earlier 16 loopback checks; this lane's toolkit/build/contracts and focused database tests recorded below | Immediate claim admission and 120-second shared-gate cases after claim-and-gate; public TLS endpoint/wake, providers, browser/OS callbacks, multiple replicas/cutover |
| Windows/macOS/Linux/Docker | Linux fixtures and Windows/macOS cross-builds | Install/upgrade/reboot/remove, service identity/ACL/UNC, watcher/NAS, certificate/proxy/IPv6/discovery and restore/power loss |
| GPUs/receivers/tuners | Generated software conversion and sharing regressions | Each backend's true HDR/colour/audio/subtitles, one channel input/multiple viewers/private DVR, cancellation/recovery and Windows custody |
| Scale on representative hosts | Final scale-lane 10k matrix: 6,945 measurements, no errors; cold full Home 79.784/102.894 ms open/restricted; manifest 0.282 ms; worst subgroup 125.374 ms | Final 200k/one-million mixed matrices and integration; authenticated full-server/HTTP/SSE/playback concurrency. No 200-browser/100-stream or two-million mixed capacity claim |

The viewer pass did not visually inspect person/collection/saved-view toolbars, a populated audiobook Series tab, music Favorites, grouped bulk selection, clock-refresh timing, paused Together start or live-tab closure. Tests/rules and the final browser walk do not substitute for those populated/manual cases. Owner Watching now with a live target, populated playback failures and real provider order choices also remain acceptance work.

Scale figures are single observations on a shared host. “Cold Home” resets the service cache; OS/SQLite pages may be warm. Old large rebuild failures and deleted databases are not final-code capacity results. The disk-full recovery preserved repository work; continue monitoring cache, scratch and free space.

### Bugs and independent reviews

The current Bugs sections contain evidence for every one of the audit's **588 unique IDs**. Counts below were recomputed from those sections, using owning-lane evidence and adjudicating compound entries. Out-of-area “not fixed here” does not override a delivered fix. “Already fixed” means the selected source reports an inherited base fix; different late reviewers may describe an earlier programme fix that way. This rerun also changes some fixed/already/not-present categories from the midday table; the per-ID evidence is recorded in handoff-update.md. These are reported source dispositions, not 588 runtime reproductions.

| Disposition | Count |
|---|---:|
| Reported fixed | 359 |
| Already fixed on the selected lane base | 173 |
| Reported not present | 7 |
| Intentional under current decisions | 11 |
| Open or partly delivered | 38 |
| **Total** | **588** |

The open set is **0 High, 20 Medium and 18 Low** under the register's second-review severity. There are no remaining reported High source findings in this reconciliation. That does not remove release gates or prove device safety.

Two previously open High entries changed: **B-11.8** now has HDHomeRun guide setup on web and native; **B-22.19** is the accepted Vega fallback, classified intentional rather than a guarantee that the SDK prevents credential disclosure. Also closed in source: web playback-problems screen B-02.16, paused Together start B-13.18, dialog CSP B-16.18, paged Genres/Series B-18.4 and reachable native appearance preferences B-21.6. B-18.27 remains partial: the native owner lane reports its new panels fixed, but the original entry also covers dormant/inert controls and catalogue cleanup. New panels alone do not close the whole original claim. B-21.11 remains open because query selectors are implemented but screens still do not use them.

The older ledger's first-pass High list remains explicit: B-06.21/B-06.24 source fixes; B-13.3 shared seek repair awaiting actual timing; B-19.18 scene source uncompiled; B-22.6 accepted HTTP trust; B-22.19 accepted redirect risk; B-25.10/B-25.12 release/device coverage and Apple component rights remain open (Medium after review).

#### Completed independent review findings

| Review | Blocker / High / Medium / Low | Total | Result |
|---|---:|---:|---|
| Identity/title/file | 0 / 3 / 7 / 0 | 10 | All corrected |
| API platform | 0 / 2 / 3 / 1 | 6 | All corrected |
| Networking | 0 / 2 / 5 / 3 | 10 | All corrected |
| Accounts/security | 0 / 2 / 3 / 0 | 5 | All corrected |
| Playback core | 0 / 6 / 1 / 0 | 7 | All corrected |
| Downloads | 0 / 3 / 3 / 0 | 6 | All corrected |
| Hosted | 1 / 0 / 3 / 6 | 10 | Nine defects corrected; one coverage addition |
| Freshness | 0 / 0 / 5 / 3 | 8 | All corrected; withdrawn suspicion excluded |
| Intake/scanning | 0 / 5 / 3 / 0 | 8 | All corrected; disproved suspicion excluded |
| Conversion pipeline | 0 / 2 / 4 / 3 | 9 | All corrected |
| Live TV/DVR | 0 / 5 / 6 / 1 | 12 | All corrected |
| Operations | 0 / 2 / 7 / 0 | 9 | All corrected |
| Late server | 0 / 0 / 4 / 0 | 4 | All corrected, including private picker cursor |
| Registry | 0 / 0 / 3 / 0 | 3 | All corrected |
| Web logic | 0 / 3 / 4 / 0 | 7 | All corrected |
| Security sweep | 0 / 0 / 4 / 1 | 5 | All corrected; seven bounded fuzz passes |
| Native JavaScript | 0 / 7 / 10 / 1 | 18 | All corrected |
| **Total** | **1 / 42 / 75 / 19** | **137** | Completed review inventory, including one coverage item |

Serious repairs include symlink deletion escape, cross-disk identity collisions, refused re-plans destroying working playback, revoked signer/device authority, one DVR owner's cancel damaging another's, stale starts replacing newer playback, lost offline acknowledgements, subtitle memory amplification and diagnostic credentials. Late security fixes enforce expiry without a database write, admin scope for cross-member playback/events and programme-bound guide images. Final health fixed new bounds triggers refusing fresh startup, dropped transaction expiry/grants and restore seed detection. No confirmed finding in these completed reviews is left unfixed; the separate inherited gaps in section 4 remain.

Apple/Android native reviews used different inventories and are not forced into this severity total. The browser run's nine finding rows and server driver's three repairs are additional runtime work, not added twice to independent-review counts. Claim-and-gate closes the two Hosted runtime handoffs; the original loopback driver included an owner-link wait and injected wake, so it is not proof of immediate admission or automatic public wake delivery.

Appendix D lists every open entry and all 588 dispositions. The original register and 12 October fix log stay historical.

## Appendix A. Lane records

All report filenames in this appendix refer to `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/`. Links resolve in the current worktree layout. Archive those reports with the internal handoff if the branches move elsewhere. Final appendices in an older report take precedence over its initial gap list; later owning-lane reports take precedence over unrelated “not mine” entries.

**accounts-security.** Unified new-sign-in admission, live permission changes, Cast-only authority, member erasure and restore scrubbing. Account/profile/TV policies were repaired; native management screens, residual restriction differences and historical Apple sign-out acceptance remain. [Report](../../../../_program/reports/accounts-security.md).

**android-mpv.** Added Android/Android TV/Fire TV mpv module, relay and JNI wiring with declared codec/output limits. JavaScript/profile checks passed; a full application build and physical HDR/audio/subtitle/lifecycle validation remain. [Report](../../../../_program/reports/android-mpv.md).

**android-native-review.** Repaired download ownership/integrity, callbacks, relay lifecycle, captions and engine teardown. Standalone Kotlin/Java and four-ABI JNI linkage/checksums passed. No Gradle packaging, APK or device run. [Report](../../../../_program/reports/android-native-review.md).

**api-platform.** Implemented keys, tokens, scopes/budgets, title query, changed-path scanning, signed webhooks and MCP. Integrations survive password/session changes and restore. Follow-ups and reviews supersede its early missing-screen and missing-service lists. [Report](../../../../_program/reports/api-platform.md).

**apple-mpv.** Added separate mpv engine and system-validated relay for Apple platforms, plus engine choices and declared limits. Apple TV output is SDR. Swift source was never compiled or exercised. [Report](../../../../_program/reports/apple-mpv.md).

**apple-mpv-review.** Repaired render-surface/audio lifetime, seek ordering, bounded fonts/HLS relay and capability declarations. JavaScript/project parsing checks passed; Swift tests and native linking have not run. [Report](../../../../_program/reports/apple-mpv-review.md).

**apple-native-review.** Repaired AVPlayer validation, callback queues, downloads, captions, audio facts and Top Shelf isolation/privacy. Project/plist/type checks passed. Its initial scene/reinstall gaps were later addressed in source by leftovers-native, with no Apple compile. [Report](../../../../_program/reports/apple-native-review.md).

**audio-engines.** Implemented continuous web resampling, bounded deep-seek fallback, runtime decoder probes, Android gain and Apple AAC trim/rewind/fallback source. Narrow tests passed; Apple limited AAC target and real HE-AAC/device output qualification remain. [Report](../../../../_program/reports/audio-engines.md).

**base-health.** Combined early lanes, repaired cross-lane identity/revision/restriction/units/intake failures and ran full server/shared/web/Cast suites. All requested checks passed except known vendored vet warnings. Later merges required the second pass. [Report](../../../../_program/reports/base-health.md).

**base-health-2.** Repaired later retention/auth/PIN/request/recording/playback collisions. Final report records 85 Go packages, 1,848 shared tests, 512 web, 20 Cast, four apikit packages and cross/dev builds passing, with vendored vet warnings retained. [Report](../../../../_program/reports/base-health-2.md).

**console.** Three initial web passes delivered integration/library/account/runtime tooling. Web-owner/viewer and native-owner now supersede its pause/seek, Undo, HDHomeRun guide, episode-order and phone panel gaps. Remaining exact controls/events are listed in section 4. [Report](../../../../_program/reports/console.md).

**conversion-outputs.** Added qualified rich picture/audio outputs, fMP4 publication, HDR/tone-map/burn-in/font paths, hardware fallback and bounded diagnostics/working space. Ordinary planner wiring came later. GPU/native output and richer prepared recipes remain unproved/incomplete. [Report](../../../../_program/reports/conversion-outputs.md).

**decision-audit.** Traced all 31 programme bullets and 61 decisions. Six cross-path corrections include Original-first Automatic, mpv remux, no PIN lockout, bounded guide reads, tolerant download extras and same-session replan; three real-server fixes were incorporated once. Its earlier screen/Vega gaps are superseded by later lanes where delivered; remaining exact gaps are in section 4. [Report](../../../../_program/reports/decision-audit.md).

**docs-internal.** Updated 41 architecture/contract/spec/backlog/decision documents. Separates requirements, reported source and unverified acceptance; preserves decision history and explicitly records Vega transport and Hosted fresh-database consequences. Later changes still need reconciliation. [Report](../../../../_program/reports/docs-internal.md).

**downloads.** Implemented durable device receipts, resumable originals/conversions, offline context, renewal and play history. Its appended merge carries styled/image originals and fonts; later follow-ups preserve chosen version and merged claims. Platform recovery/clock/signed-out player gaps remain. [Report](../../../../_program/reports/downloads.md).

**e2e-hosted.** Completed 16 backend/shared-gate loopback checks with real format-21 PostgreSQL and local server. Found immediate claim admission and activation-gate gaps; claim-and-gate now repairs both. The original driver waits for owner links and uses injected wake transport, so public wake/immediate admission/provider/device acceptance still need separate checks. [Report](../../../../_program/reports/e2e-hosted.md).

**e2e-server.** Completed 18 groups on a real loopback server with generated media. Reproduced and repaired ASS style loss, conversion-capacity error mapping and selected-asset lookup. Final base health reran all groups successfully; no PGS source/provider-backed canonical person fixture in this driver. [Report](../../../../_program/reports/e2e-server.md).

**e2e-web.** Loaded production web in headless Chrome, checked 57 routes at two widths and all interaction groups. Fixed nine finding rows including dialog CSP, subtitle rendering, AV1 conversion, version freshness, author IDs and profile permissions. Later viewer work resolves its toolbar/label handoffs. No physical browser/device acceptance implied. [Report](../../../../_program/reports/e2e-web.md).

**freshness-clients.** Added quiet viewer-scoped hub, retained reads, event sharing and delayed failures. Later web owner/viewer lanes add 30-second rereads for lists without hints; native shelves/grids gain limited fade. Native paged owner/deep-list/HTTP tab-sharing gaps remain. [Report](../../../../_program/reports/freshness-clients.md).

**freshness-server.** Added retained browse generations/two FTS slots, separate revision domains, private coalesced feed, failure isolation and bounded reclamation. Client/console consumers came later. No target-scale throughput or physical network acceptance claimed. [Report](../../../../_program/reports/freshness-server.md).

**grouping-sorts.** Added common scoped title vocabulary, indexed scalar/profile orders, exact paged aggregates and capability contracts. Filtered aggregate sorting/deep direct ranges retain disclosed cost gaps; existing legacy envelope screens need migration. [Report](../../../../_program/reports/grouping-sorts.md).

**handoff-draft.** Assembled the midday owner report, earlier bug reconciliation, settings/components and source limitations. This handoff-update replaces its stale integration, Vega, screen, test and owner-question statements; originals/fix log remain historical. [Report](../../../../_program/reports/handoff-draft.md).

**hosted.** Added no-registration S256-PKCE app consent returning the ordinary server-bound assertion, signer/revocation/outage fixes and fresh-only format. Review/follow-ups supersede format-20/toolkit/provenance gaps. No existing service was changed. [Report](../../../../_program/reports/hosted.md).

**hosted-followups.** Synchronized 99 JSON routes, durable certificate check-in and consent revocation; format 21 needs empty state. Claim-and-gate closes the local oversized-root bound. This update refreshes the toolkit again, regenerates and checks Hosted contracts. Fleet/provider/deployment acceptance remains. [Report](../../../../_program/reports/hosted-followups.md).

**identity.** Separated titles/assets/versions, retained state, missing-title rules, real file deletion and background library removal. Later review fixed confinement/asset/approved-set issues; later intake/wiring delivered editions/parts. General audio/container merge and large transaction behavior remain limited. [Report](../../../../_program/reports/identity.md).

**integrator.** Combined overlapping server/native work, restored compatible schema/settings/units and package copies, and exposed hidden integration defects. Its early grouping/title-order question was subsequently overruled by the wide subgroup sort decision. [Report](../../../../_program/reports/integrator.md).

**leftovers-native.** Disposed 116 entries and repaired scenes/reinstall/control reserve/runtime/settings in source. Its Vega blanket refusal was replaced by the accepted functional fallback. Later screens/owner lanes close broad grouping/Undo/appearance/administration gaps; occurrence/subtitle/lifecycle/qualification gaps remain. [Report](../../../../_program/reports/leftovers-native.md).

**leftovers-server-a.** Disposed 27 entries: anime orders/offsets, fresh episode reuse, account-management boundaries, legacy admission removal, failure-memory APIs, tolerant local XML, preview rules, backup bounds and watchdog. Historical Apple incident and owner screens remain. [Report](../../../../_program/reports/leftovers-server-a.md).

**leftovers-server-b.** Disposed 32 entries: internal error mapping, early EOF/credits fallback, bounded backup/fan-out, offline future stamps, baseline consolidation and release-input/vulnerability guards. Native clock/recovery/CI, advisory resolution and Apple rights remain open. [Report](../../../../_program/reports/leftovers-server-b.md).

**limits-settings.** Wired runtime controls and still-watching, added owner analysis/subtitle limits, consolidated background pace/pause, introduced corrupt-state recovery and source-consumer guards. Structural witnesses do not prove device effects; its early quality inventory predates final consolidation. [Report](../../../../_program/reports/limits-settings.md).

**livetv.** Implemented shared acquisition/private readers, tolerant streamed/spooled guides, import controls, shared private DVR bytes and recovery facts. Backend tests passed; owner guide/countdown consumers, Windows custody, real tuners and load acceptance remain. [Report](../../../../_program/reports/livetv.md).

**music-audiobooks.** Added bounded broad audio-tag reader, lyrics/art/chapter/grouping, natural part order, comfort rewind, queue shuffle semantics and durable audio facts. Own-renderer/native follow-ups came later. Online book provider and physical/sample-exact codec coverage remain absent. [Report](../../../../_program/reports/music-audiobooks.md).

**naming.** Added bounded pure naming/evidence/ignore reader and hundreds of corpus tests without a dependency. Scan-loop later wired configuration and identity/evidence into production. Disc playback is deliberately unsupported; hostile bounds remain. [Report](../../../../_program/reports/naming.md).

**native-app.** Repaired scoped lifetimes, paged Saved/History, links/setup/account UX and accessibility. Later screens add controls/Undo and common offline launch. Exact occurrence/subtitle selectors and screen forwarding of full query queues remain. [Report](../../../../_program/reports/native-app.md).

**native-health.** Made refreshed native/Vega packages and existing browse controls pass: 514 native/64 Vega tests, app/test type checks and lints. Added bounded silent 428 recovery. These are JavaScript checks, not native application builds or rendering evidence. [Report](../../../../_program/reports/native-health.md).

**native-playback.** Moved native video to shared controller with capability, quality and subtitle/display source wiring. Later screens preserve the session on engine changes and add remote controls. Offline fonts and exact discovered-track choices remain; platform SDK/device behavior is unproved. [Report](../../../../_program/reports/native-playback.md).

**native-sync.** Mirrored packages and joined settings, version selection, HTTP/ATS, account gates, handoff/report/privacy and sign-out removal. Later native-owner provides custom phone/tablet panels; native screens add preference consumers. Device cleanup and remaining shared adapters still need acceptance. [Report](../../../../_program/reports/native-sync.md).

**networking.** Unified locality/proxy handling, HTTP/TLS/IPv6 listener, external routes, own-certificate reload, discovery topology and console reserve. Independent review repaired h2/trust replay/policy refresh defects. Real proxy/multicast/container/device behavior remains unqualified. [Report](../../../../_program/reports/networking.md).

**ops-packaging.** Added restart parent, detached restore, SCM/macOS/Linux/Docker recipes, alternate storage, notices/reports, signing inventory and reclamation. Fixture/cross checks passed. No installer/reboot/signing/notarization/Docker/GPU acceptance was performed. [Report](../../../../_program/reports/ops-packaging.md).

**personal-state.** Implemented bounded atomic container marks/Undo, completion/history/privacy ledgers, Home destinations and corrected Continue behavior. Download/quiet-counter integration came later. Physical EOF/offline/large-container and complete UI acceptance remain. [Report](../../../../_program/reports/personal-state.md).

**planner-wiring.** Connected converter qualification/target persistence to ordinary/live/offline planning, signal-preserving choices and executable fMP4 tests. Live publisher remains TS; optimized downloads remain H.264 SDR stereo; mastering-display/MaxCLL facts are not captured. [Report](../../../../_program/reports/planner-wiring.md).

**playback-core.** Created one capability document, native/mpv choice, Original/fixed/Automatic policy and generation-fenced session changes, remembered tracks and failure memory. Producer/client/review lanes supersede its early missing-output/menu notes. Real hardware claims remain untested. [Report](../../../../_program/reports/playback-core.md).

**providers-matching.** Added verified fuzzy/yearless matching, attention actions, provider-specific pace, locale/credential/art/NFO policy and refresh. Scan-loop wired sidecars; leftovers completed anime/episode-cache cases. Fixtures are not live provider acceptance. [Report](../../../../_program/reports/providers-matching.md).

**registry.** Declared retained production handlers and generated path/response/body helpers; duplicate YAML removed. Production inventory remains 622 operations. Later registry review repairs path safety, declarations and union decoding; toolkit copy refreshed here. [Report](../../../../_program/reports/registry.md).

**review-accounts-security.** Found and fixed five findings: revoked-signer descendants/pending paths, transaction-time device authority, restore integrity/retry and waiting-device admission. Regressions passed. Original residual client/privacy/incidents are not certified closed. [Report](../../../../_program/reports/review-accounts-security.md).

**review-api-platform.** Found six findings, later fixed in appended phase: pre-admission SQL, cursor cost, webhook fairness/order, MCP gap and use accounting. Its initial findings-only “unfixed” text is historical; appended fixes and base integration supersede it. [Report](../../../../_program/reports/review-api-platform.md).

**review-downloads.** Found and fixed six access/replay/activity/address/metadata-publication defects while preserving standalone copies. Focused repository tests passed; native storage/restart/expiry/signed-out operation remains separate acceptance. [Report](../../../../_program/reports/review-downloads.md).

**review-freshness.** Found eight confirmed findings and fixed them, including event replay gaps, dismissal/history/search hints, counters, Unicode predicates and diagnostic wire time. One suspicion was withdrawn. Large-capacity and missing screen consumers remain separate. [Report](../../../../_program/reports/review-freshness.md).

**review-hosted.** Reported ten closed findings including real-server challenge mismatch, cookie lookup, check-in/signing availability and safe consent/callback behavior. One item added coverage. Isolated PostgreSQL checks passed; browser/device/fleet/cutover acceptance remains unperformed. [Report](../../../../_program/reports/review-hosted.md).

**review-identity.** Found ten findings and later repaired cross-disk asset identity, symlink-safe deletion, approved-set consistency, show/provider/tag/retention identity and offline missing protection. Regression tests passed; device/file-system and large-transaction acceptance remains. [Report](../../../../_program/reports/review-identity.md).

**review-livetv.** Found and fixed 12 parser/acquisition/capture/authorization/recovery/control defects. In particular shared owners survive cancellation/finalization and restricted metadata stays hidden. No physical tuner/NAS/GPU or complete saturation acceptance. [Report](../../../../_program/reports/review-livetv.md).

**review-native-js.** Completed 18 repairs: 7 High, 10 Medium, 1 Low. Native 547/Vega 72 at that checkpoint, clean type/string checks. Regressions cover lost offline acknowledgements, viewer enrollment, stale starts, stream seek/cancel and secret redaction. Vega transport and later health/owner lanes supersede earlier gaps/counts. [Report](../../../../_program/reports/review-native-js.md).

**review-networking.** Found and fixed ten h2/trust replay/reserve/origin/scoped-address/policy/publication/status defects. Initial findings-only text is preserved historically; the appended implementation receipt is current. Actual supported topologies/capacity remain unqualified. [Report](../../../../_program/reports/review-networking.md).

**review-ops.** Found and fixed nine restore/copy/scrub/report/retention/supervisor defects, with focused regressions. Native power-loss/service/release acceptance and rights/signing work remain. No original database or deployment was touched. [Report](../../../../_program/reports/review-ops.md).

**review-playback-core.** Found and fixed seven transactional/authorization/ceiling/locality/control-context defects. Refused changes preserve working playback; source visibility is rechecked. Physical throughput/player behavior and the distinct subtitle mutation boundary remain unproved. [Report](../../../../_program/reports/review-playback-core.md).

**review-playback-pipeline.** Found and fixed nine geometry/copy/colour/header/fMP4/audio-clock/signalling/orphan-walk defects. Real generated conversion checks complement source assertions; physical GPU/HDR and confinement behavior still require acceptance. [Report](../../../../_program/reports/review-playback-pipeline.md).

**review-registry.** Completed three Medium repairs: safe generated paths, accurate analysis/lyric/readiness declarations and actual union typing/decoding. Targeted checks pass; generated contracts are included in final health and refreshed Hosted copy. [Report](../../../../_program/reports/review-registry.md).

**review-scan.** Confirmed eight findings and fixed them, including unsafe weak audio identity, stale revival/removals, unreadable subtrees, bounded absence checks and extra/artwork evidence. One suspicion disproved. Native watchers/NAS/provider/large-load acceptance remains. [Report](../../../../_program/reports/review-scan.md).

**review-server-late.** Completed four Medium repairs: download request identity/exact-version retry, factual deletion reconciliation and hidden-ID-free restricted DVR continuation. Final base suites include these changes; actual filesystem/tuner acceptance remains. [Report](../../../../_program/reports/review-server-late.md).

**review-web-logic.** Completed seven repairs: 3 High, 4 Medium. Combined subtitle memory/work bounds, retired controller operations, safe settings rebasing, follower resync, bounded music ranges, memory ordering and malformed-feed backoff all have regressions. [Report](../../../../_program/reports/review-web-logic.md).

**scale-check.** Completed initial mixed 10k and structural 10k/200k/2M index checks and browse/count/hydration repairs. Cold Home and grouped deep outliers triggered scale-check-2. Old isolated index observations are not mixed large-catalogue capacity. [Report](../../../../_program/reports/scale-check.md).

**scan-loop.** Wired relative naming/tags/IDs/editions/parts/extras/discs, ignore/evidence lifecycle, tolerant pages, native watchers, change-driven limited scans and bounded retention. Linux watcher exercised; macOS cgo and real platform mounts remain uncompiled/unqualified. [Report](../../../../_program/reports/scan-loop.md).

**search-sort.** Added natural/Unicode sorting, richer indexed search/CJK groups, aggregate paging, integer 0–10 ratings and general millisecond/UTC boundaries. Common grouping controls came later; raw-title anchors/Saved search/specialized time shapes remain incomplete. [Report](../../../../_program/reports/search-sort.md).

**security-sweep.** Completed five repairs: 4 Medium, 1 Low; no High/Critical. Expiry, admin scope/events, programme-bound guide images and diagnostic secret scrubbing corrected; seven bounded fuzz passes. Preserves deliberate HTTP/plaintext/no-lockout/TV-role decisions; physical/deployment assurance remains. [Report](../../../../_program/reports/security-sweep.md).

**server-docs.** Updated 34 public documents and two guides, installation/API/media/dependency notices and executable examples. Later registry removal supersedes its kept-YAML note. Source-backed instructions are not proven deployment, artifact licence or signing acceptance. [Report](../../../../_program/reports/server-docs.md).

**server-followups.** Delivered exact attention/artwork/resource status, one-version preview/delete, download identity pinning, source pickers, owner transport controls and role freshness. Web third-pass consumers followed; later review caught request/deletion/picker edge cases. [Report](../../../../_program/reports/server-followups.md).

**social.** Repaired room readiness/revisions/privacy and Cast recovery. Later web/native screens carry paused-start intent. Actual synchronization, receiver controls, duplicate-TV identity and Android Cast module remain outstanding. [Report](../../../../_program/reports/social.md).

**subtitles-text.** Delivered lazy track discoveries, tolerant legacy text/cues, raw originals/fonts, provider login and preview/marker/conditional-response fixes. Later limits expand inventory/file defaults. Renderer/burn/offline integration and real corpus/provider acceptance remain separate. [Report](../../../../_program/reports/subtitles-text.md).

**test-plans.** Completed fifteen environment plans and index with 203 initial numbered cases. This update adds late owner/group/queue/transport/retry/retention cases and corrects obsolete Vega refusal expectations. Plans require execution; none is a passing result by itself. [Report](../../../../_program/reports/test-plans.md).

**vega.** Implemented shared engine/live/caption/capability shims. Later transport restores authenticated operation using exact-origin binding/redirect containment, accepted by the owner. SDR/stereo and unsupported features remain; real SDK/device acceptance outstanding. [Report](../../../../_program/reports/vega.md).

**web-app.** Implemented bounded tolerant presentation parsing, per-tab memory, search privacy/history, stable reconnect/page state, role confirmation and non-player UX fixes. Security identities/receipts remain strict. Later hub/console work supersedes its early missing-field list. [Report](../../../../_program/reports/web-app.md).

**web-player.** Replaced old adapter with shared controller and persistent video element; repaired session/timeline/lease/queue/quality/failure/keyboard/seek behavior. Later subtitle/audio lanes fill parts of its gap list. Actual hls.js/element/media-key behavior remains unqualified. [Report](../../../../_program/reports/web-player.md).

**web-subtitles.** Added vendored browser libass, own PGS/VobSub/DVB workers, fallback fonts/position, 3x audiobook/up-next and native-HLS policy. Merger wired raw plans; late web review repairs combined-memory budgets. Real corpus/browser/PiP/cast/font coverage remains incomplete. [Report](../../../../_program/reports/web-subtitles.md).

**base-health-3.** Final server integration pass repairs fresh startup bounds, transaction expiry/grants and restore seed extraction. Every requested suite passes: 86 Go packages, 1,937 core, 545 web, 20 Cast, 18 real-server and 13 browser groups; Linux/cross-builds pass. Vendored vet warnings retained; no native/hardware acceptance. [Report](../../../../_program/reports/base-health-3.md).

**claim-and-gate.** Closes immediate claim-owner admission in the install transaction with post-commit wake, activation origin-gate bypass and oversized certified revisions. Focused server/client regressions pass. Original Hosted loopback driver was not rerun for these final fixes. [Report](../../../../_program/reports/claim-and-gate.md).

**queue-and-retention.** Implements snapshot-captured query/saved-view queues, bounded checkpoints/restarts/shuffle and complete record inventory, compiled table ceilings and housekeeping. Server/core tests pass. First-party screen selector forwarding, subtitle retirement and lifetime capacity remain other work. [Report](../../../../_program/reports/queue-and-retention.md).

**web-viewer.** Adds group/tab toolbars, paged Genres/Series, card/page receipt Undo, catalogued labels, session-PATCH subtitle choices and quiet downloads/DVR/guide. Chrome driver and focused checks pass. Home/personal/book fields, cross-library/group-order/role sorts and full queue forwarding remain gaps. [Report](../../../../_program/reports/web-viewer.md).

**web-owner.** Adds Watching now transport/seek, playback problems, viewing hours, episode order/offset, HDHomeRun guide URL, field-safe silent 428/412 retry and quiet lists. Most owner pages inspected in Chrome at two widths. Live owner-to-player control/provider/failure panels and builder/mount conflicts remain. [Report](../../../../_program/reports/web-owner.md).

**native-screens.** Adds scoped group controls, Undo, Play next/queue, common signed-out player, stored subtitle companions, appearance/captions, in-place engine replan, Play on controls and bounded fade/paused Together. Rules/type checks only; local fonts, full queries, several scopes and actual layouts remain. [Report](../../../../_program/reports/native-screens.md).

**native-health-2.** Native 605 and Vega 87 tests pass after fixture/guide cleanup; app/test type checks and string lints pass. Uses shared defaultSort/category definitions without legacy adapters. These results precede owner panels and the final shared refresh. [Report](../../../../_program/reports/native-health-2.md).

**native-owner.** Adds phone/tablet N1–N5 owner panels and read-only TV Dashboard; removes old TV actions. Native 655 and Vega 87 tests, app/test type checks and string lints pass before final mirror. Existing touched-folder ESLint unused variable, missing catalogue keys, paged refresh and actual screen/device acceptance remain. [Report](../../../../_program/reports/native-owner.md).

**vega-transport.** Restores sign-in/control/streams with exact-origin credential binding, observed redirect rejection, durable sign-out and bounded revocation attempts. Vega 87 tests and type/string checks pass. SDK pre-follow disclosure/return-chain risk is accepted; actual device/capture forwarding not run. [Report](../../../../_program/reports/vega-transport.md).

**handoff-update.** Refreshes this report, reconciles current Bugs sections, extends/corrects 250 cases across fifteen environment plans and aligns Hosted toolkit/contracts. Focused build/contract/HTTP/toolkit/TypeScript/database checks are recorded in its report. No commit, push or deployment; final cumulative snapshot is the handoff. [Report](../../../../_program/reports/handoff-update.md).

**scale-check-2.** Still running after disk recovery. Final-code 10k completes 6,945 measurements without errors; 200k/one-million final matrices pending. Deferred Home, indexed grouped/search reads and batched mark/undo are written and focused-tested. No final snapshot/integration/large mixed capacity or full-server concurrency claim. [Report](../../../../_program/reports/scale-check-2.md).

## Appendix B. New and newly wired settings

The tables cover changed controls, rather than repeating every unchanged setting. “Screen” means reported/source-present; web has automated visual checks, native has no device/visual acceptance. Defaults/limits describe this implementation, not a tested capacity. The old 20 per-network quality fields, physical-trash/grace controls, WAN-as-LAN and forced-TLS choices were retired; do not copy them from an early report's settings inventory.

### Owner and library controls

| Control / key | Type and default | Effect and reachability |
|---|---|---|
| Missing titles `library.missingTitles`, `missingTitlesDays` | Immediately / after days / never; after 30 days, 1–3650 | Last available file gone; automatic cleanup at worker checkpoints. Web Storage & backups; phone/tablet settings rows; device acceptance pending |
| Monitor changes `monitorChanges` | Per-library boolean, true | Native watcher plus periodic fallback; manual/configured scans still work. Web Scanning tab and shared native library rows; actual watcher acceptance pending |
| Background speed `backgroundSpeed` | Gentle / Normal / Fast, Normal | Shared provider/database/source pace and process priority. Web and phone/tablet Schedule; housekeeping quanta now paced too. Separate priority choice removed |
| Background pause | Owner action, no pause; one hour | Cooperative batches/admissions stop until deadline or Resume. Web Schedule and phone/tablet Schedule; TV read-only |
| Analysis storage `analysisStorageGiB` | Integer, 32 GiB; 1–16384 | New analysis artifact admission/publication. Shared Storage & backups row |
| Per-stage output `analysisStageStorageMiB` | Integer, 256 MiB; 1–4096 and no larger than total | Stage budget. Shared Storage & backups row |
| Tracks per file `subtitleTrackLimit` | Integer, 128; 32–4096 | Useful bounded subtitle inventory on new scan. Shared storage row |
| Largest subtitle `subtitleFileLimitMiB` | Integer, 128 MiB; 32–512 | New sidecar/acquisition/embedded bitmap reads. Shared storage row |
| Local artwork `localArtworkLimitBytes` | Integer bytes, 2 GiB | New ingestion quota and deduplicated usage ledger; retained images survive lowering. Web and phone/tablet Metadata, GB display |
| Adult search `includeAdult` | Boolean, false | TMDB search. Web Metadata and phone/tablet Metadata |
| Provider credentials `tmdbReadToken`, `tvdbProjectKey` | Optional secrets, unset | Hot overrides, reads show presence only; blank restores application credentials. Web Metadata and phone/tablet Metadata |
| Library provider/language/region | Curated choices; TMDB video, en-US/US; local-only available | Adapter/field ownership and certification policy. Web and phone/tablet library metadata; live-provider acceptance pending |
| Anime provider order/offset | Owner selection plus signed integer offset | Absolute order mapping; affected files can need order. Web Edit metadata Match; native dedicated order/offset screen pending |
| FFmpeg `ffmpegPath` | Absolute path, empty uses default | Qualification/status before restart; no hot running-tool change. Web Playback, requested/effective/refusal status |
| Conversion working space `sessionStorageMiB` | Integer, 4096 MiB; 64–1048576 | Per-session pressure/reclamation. Web Playback uses GB; sub-GB values display poorly until edited |
| Temporary directory `temporaryDirectory` | Absolute path, empty | Restart; dedicated scratch child. Existing Playback row |
| Background conversions `maxBackgroundSessions` | Integer, 0 means no extra owner cap | Future background process admission. Existing Playback row |
| Tone mapping `hdrToneMapping` | Boolean, true | HDR planning; qualified algorithms/backends still apply. Existing Playback row |
| Remote sign-in `remoteSignInPolicy` | Allow / Owner only / Off, Allow | Every new issuance using receiving device locality. Existing connectivity screen |
| New-device approval `deviceApprovalRequired` | Boolean, false | Real device registration, waiting states and decisions; owner exempt. Web People › Devices; shared/native row |
| Member rating system `ratingSystem` | Published system or empty, empty | Unified age rule with unrated behavior. Web People and phone/tablet account rating picker |
| Self-registration policy | Existing choice, off | Invite-only now works. No first-party policy-setting screen reported |
| Home networks `lanNetworks` | List, standard local/attached ranges | Sole locality authority; empty means all remote. Web lists and shared rows |
| Trusted proxies `trustedProxies` | List, empty | Verified address/scheme/host chains. Existing screen; web add/remove list |
| External address `advertiseUrl`, extra `accessUrls` | Origin/string list, empty | Local candidate publication; only HTTPS reaches Hosted. Web Remote access and shared phone rows |
| Ordinary connections `connectionLimit` | Integer, 0 auto; maximum 65536 | Restart; fixed 32-console reserve additional. Web Advanced and shared phone row |
| Own certificate path/key/domain | Strings, empty together | Validated/reloaded PEM source with serving-state readback. Existing/web/shared rows |
| LAN discovery/interface | Boolean true / string empty | Hot discovery lifecycle; explicit interface preference, no name-based automatic exclusions. Existing screens |
| API request rate `apiRequestsPerMinute` | Integer, 0 means 600/minute; up to 100000 | Per-key/token rate at once. Web and phone/tablet Integrations |
| Block member token creation `blockMemberAccessTokens` | Boolean, false | Stops new plain-member tokens, preserves existing. Web and phone/tablet Integrations |
| Live import limits `importLimits` | 512 MiB / 50,000 channels / 5,000,000 programmes | Owner maxima 1 GiB / 100,000 / 10,000,000; bounded per-record parsers remain. Web server-wide fields; per-source readback pending |
| Source groups/channels and mapping/numbering | Existing lists/default all and source fields | Filter before staging, stable provider choice IDs; web and phone/tablet source pickers/mapping; HDHomeRun guide address delivered |
| Live buffer/retry/user agent/horizon/logo/discovery | Existing values/defaults | Runtime consumers wired; source stream buffer is startup delay, not retained timeshift length |
| DVR padding/keep policy | Existing numbers/count/days | Capture and retention now use them; private ownership/shared storage preserved |
| Download storage `maxPreparedBytes`, `retentionDays` | Existing ceiling / 30-day claim retention | Converted artifacts only count as server prepared bytes; originals are in-place. Device copies survive claim expiry |
| Updates `updates.feedUrl` | Official feed; empty opts out | Periodic cached owner notice; custom feeds retained. Web “Check for new versions” switch |
| Scheduled backup keep count | Integer, seven | Scheduled copies only; manual copies retained until deletion. Existing screen |
| Certificate/storage notices and log/history retention | Existing thresholds/days | Five-minute notices and bounded maintenance consumers. Existing settings; acceptance pending |
| `PORTICO_CACHE_DIR`, `PORTICO_BACKUP_DIR` | Startup absolute paths, default under state | Transient media and ordinary backups; rollback copies stay beside database. Read-only web storage display, no editing field |
| OpenSubtitles username/password | Startup optional secrets, unset | Login/renewal; anonymous API-key path remains. No credential UI; old static token input retired |

### Viewer and device controls

| Control | Type / default | Effect and reachability |
|---|---|---|
| `quality.home`, `quality.remote` | Original / Automatic / fixed; Original | Device-class saved start defaults; session menu overrides for that viewing. Web/native rows |
| Player engine `playerEngine` / `engineSetting` | Automatic / Native / mpv; Automatic | Installation-local capability publication and planning; shown only where mpv is built. Native settings/player menu |
| `forceStereo`, `disablePassthrough`, `disableHdr` | Device booleans, false | Narrow declared capabilities; passthrough/HDR controls hidden where unavailable. Web/native wiring reported |
| Caption size/background | Choices, medium/translucent | Native text and web captions; authored ASS preserves style. Native player panel and native Settings Audio & Subtitles |
| Drawn subtitle vertical position | Device value, 0; −10% to +30% | Web overlay positioning in 2% steps; Up/Down/Reset in subtitle menu |
| Audiobook default speed | Choice, 1×; now through 3× | Server/shared/web extended choices; native engine/screen/device limits still need qualification |
| Music normalization, gapless, crossfade | Off / true / zero seconds | Gain/peak-safe PCM, continuous compatible joins, bounded crossfade. Existing controls; unsupported Vega effects hidden |
| Started threshold | Integer 1–25%, 5% | Known-duration Continue eligibility without 30-second ceiling; existing screen reachability noted as a remaining handoff |
| Played threshold | Integer 75–100%, 95% | Music and explicit video override; otherwise video library policy. Books use time remaining |
| Still-watching protection/count | Boolean true / integer 3, 2–5 | Server queue auto-advance policy and Continue prompt; manual next resets run. Existing controls now consumed |
| Search history `search.rememberHistory` | Profile boolean, true | Commit-only history; off shows none. Web and native Privacy |
| Badges `notifications.badges` | Profile boolean, true | Native Appearance row now exposed as well as web consumer |
| Pause history / show activity to members | Existing booleans | Owner records unaffected; private progress/counts and shared popularity honor privacy |
| Together activity `privacy.includeInWatchTogether` | Profile-server boolean, true | False anonymizes participant/position but preserves visible room occupancy/readiness; exact screen reachability still needs reconciliation |
| Appearance card size/backdrops/reduce motion | Existing values | Web consumers exist; native shelf/grid/title/Home card/backdrop consumers now wired |
| Remove downloads at sign-out | Per-action boolean, false | Exact account's files only; keeps other profiles. Native option reported, physical cleanup untested |
| Library tab/sort/filter memory; fit/fill | Device/viewer memory, no new switch | Links preserve explicit intent; per-tab library memory; web fit/fill menu/key |

Remembered audio/subtitle choice, failure-memory clear, container Undo, version delete, reboot/restart, restore and report upload are actions, not extra saved switches. No MCP enable switch exists.

### Afternoon screen wiring and actions

No new stored preference was invented by the frontend lanes. Existing viewing-hours windows now have web/phone controls; web episode-order/offset and HDHomeRun guide URL have controls; native rating systems, metadata keys, artwork/adult policy, source groups/channels/map and API rate/member-token policy are reachable. Caption size/background and native card size/backdrops are wired. Web playback-problems Forget, Watching now pause/resume/relative seek, native Play on, receipt Undo, member-delete preview, library removal, file tools, share/report, pause/restart and TV Dashboard are actions. TV exposes none of the owner mutation actions. Native background pause and FFmpeg status rows now render. `backgroundSpeed` additionally paces table/analysis housekeeping. No new release dependency was added by these later lanes; exact native catalogue wording/adapters remain as listed in section 4.

## Appendix C. Added components and licences

This is the source inventory, not a legal conclusion or proof that a final package contains complete notices/source/relinking materials. B-25.12 remains open. Review the actual signed artifact and `THIRD-PARTY-NOTICES.md`, plus the native dependency notices, before distribution.

| Added/bundled component | Version / purpose | Recorded licence and distribution concern |
|---|---|---|
| Browser libass WebAssembly | Vendored `@jellyfin/libass-wasm` 4.2.4, hash/provenance pinned | Bundle includes FriBidi LGPL-2.1-or-later; other parts recorded as MIT/ISC/FreeType/Zlib/BSD/BSL/public-domain. Keep corresponding source/submodules and notices; CSP needs wasm-unsafe-eval |
| Apple MPVKit | SPM exact `1.1.0-n9.0.2`, LGPL product, binary module Libmpv | LGPL component inventory, relinking/replaceability/source and store distribution require artifact review. Do not substitute the GPL product |
| Android libmpv | media-kit `v1.1.11` full flavour, four SHA-256-pinned ABIs | mpv `gpl=false`, FFmpeg `disable-gpl` plus `enable-version3`: LGPL version-3 FFmpeg with mbedTLS. libass/dav1d/other linked components need full bundle notices/source. Older features deliberately absent |
| libmpv client header | Checked-in JNI API header | ISC, preserve header notice |
| Browser fallback fonts | Noto script fonts, Droid Sans Fallback, embedded Liberation Sans | Noto/Liberation SIL OFL 1.1; Droid Apache 2.0. Pins/package origins recorded. Korean fallback absent in browser bundle |
| Server burn-in fallback fonts | Noto CJK/Thai/Devanagari alongside existing DejaVu | Noto SIL OFL, embedded licences; DejaVu's existing font notice must travel. Font attachments retain bounds |

FFmpeg was already a server dependency. Existing React Native/Media3/Google Cast/graphics/browser dependencies were not replaced wholesale by this programme. Packaging/docs added or corrected existing Manrope OFL and Lucide ISC notices. Broad tags, bitmap decoders, queue/freshness logic, MCP, watcher and relay work were written without adding a new framework dependency. The public docs lane inventories 234 npm package/version/licence rows across the server lockfiles; that does not establish the complete native binary graph. Published release artifacts remain the authority for their own dependency bill.

## Appendix D. Reconciled original bug register

This is a reproducible reading view of reports, not a new audit. All 588 IDs are accounted for. It rereads every current report's Bugs section and uses the fix log as historical context. It uses the owning remaining-bug reports and later screen/transport receipts, excludes unrelated out-of-area statements as completion evidence, and explicitly adjudicates compound/partial wording. A source check overrides one contradiction: B-18.13 is fixed by the actual selection-bar preview/typed-confirmation dialog, despite a remaining-bug report calling it an intentional no-confirmation flow.

Open includes partial source fixes, unimplemented client consumers, unresolved incidents and explicit acceptance/design risks. Fixed and already-fixed entries still need their environment acceptance. B-02.16 now has the web owner screen; B-08.12 remains open for response-composition cost. Completed source repairs are not downgraded solely because a physical device has not been run, but findings explicitly about test/release coverage remain open.

| ID | Reviewed severity | Original claim | Current gap / owning report |
|---|---|---|---|
| B-01.23 | Medium | Related Backlog item: two Apple sign-ins ended with explicit_signout | Historical Apple explicit_signout has no correlated trace or established cause; source defenses do not reproduce the incident. [leftovers-server-a](../../../../_program/reports/leftovers-server-a.md) |
| B-04.35 | Medium | "Complete" scanning is far heavier than its label suggests (design risk) | Default Complete avoids checksum/fingerprint work; shared-decode optimization and representative scan cost remain unqualified. [leftovers-server-a](../../../../_program/reports/leftovers-server-a.md) |
| B-07.10 | Low | The client's letter-bucket match disagrees with the server's letters | Client letter anchors still lack authoritative article-stripped/explicit sort-key facts. [freshness-server](../../../../_program/reports/freshness-server.md) |
| B-08.12 | Low | If-None-Match, ETag and encoding nits | Conditional responses still compose the response before comparing its validator; encoding/header fixes do not remove that cost. [networking](../../../../_program/reports/networking.md) |
| B-10.11 | Medium | A bitrate limit on an Apple device turns the audio plan off | Apple limited-bitrate listening still lacks the own renderer’s supported converted AAC target/selection. [apple-native-review](../../../../_program/reports/apple-native-review.md) |
| B-10.28 | Medium | The gapless fixtures leave whole classes of file untested (design risk) | Real HE-AAC/PS and physical codec/output gapless fixtures are still needed. [apple-native-review](../../../../_program/reports/apple-native-review.md) |
| B-11.27 | Medium | The pre-emption countdown is not delivered | Server reservation deadline/event exists; viewer countdown consumption is not established on all players. [livetv](../../../../_program/reports/livetv.md) |
| B-13.10 | Medium | Cost grows with the square of the group size | Hot projection/event work improved, but no ceiling-scale room benchmark closes the group-size cost risk. [social](../../../../_program/reports/social.md) |
| B-14.14 | Medium | Files of a signed-out account or another profile stay on the device, invisible and unremovable | Optional account sign-out removal and privacy-safe reclaim exist; actual device cleanup and device-wide scope qualification remain. [native-sync](../../../../_program/reports/native-sync.md) |
| B-14.20 | Medium | Android transfer lifetime | Android automatic transfer recovery while the process is dead still needs an installation-owned platform job. [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-14.21 | Low | Clocks | Server future activity is fenced; persisted trusted/monotonic device receipt expiry remains wall-clock based. [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-15.19 | Low | Supply-chain loose ends | Workflow/container pins exist; installer signing and the reported source-map-js advisory still need release resolution. [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-16.4 | Low | WindowedGrid's mapped-scroll mode has discontinuities near the ends | Compressed scrolling still has end-range discontinuities. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.5 | Low | WindowedGrid re-creates its ResizeObserver and forces layout on every row change; inline renderers defeat per-page memoisation | Stable measurement subscriptions exist; inline library cell renderers still need memoisation. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.7 | Medium | Each open tab holds a long-poll; on plain HTTP several tabs can starve the connection pool | Visible plain-HTTP tabs can still own separate long polls; secure-only sharing does not close it. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.10 | Medium | Strict parsers reject whole documents for unknown enum values | Presentation row salvage is implemented on reviewed paths; wider descriptive parser coverage remains partial. Strict authority and no pre-release compatibility stand. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.12 | Low | No declared browser floor; older engines degrade silently | No declared build target/browser floor and complete older-engine acceptance. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.14 | Medium | Every page has the same document title; navigation is not announced | Titles/announcements exist; rapid navigation and identical-heading announcement cases remain unqualified. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.17 | Low | Smaller accessibility defects in primitives | Forced-colour and font-scale primitive styling/accessibility work remains. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.23 | Low | React hook dependency rules are not enforced by any tool | Web hook dependency rule enforcement is absent. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.25 | Low | CI and the shipped image build the web with different Node versions | CI and container web build use different Node versions; alignment/pinning remains. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-17.19 | Medium | Media keys and lock-screen controls for music depend on Web Audio counting as "media" | Actual background browser media-key/lock-screen activation is not established. [review-playback-core](../../../../_program/reports/review-playback-core.md) |
| B-18.9 | Medium | The "Administrator" role can be granted but gives nothing in the web app | Web management still gates on exact owner despite the selectable administrator role. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.22 | Low | English text written straight into components | Player/console strings moved to catalogue; web metadata fallback labels and some native tables still use English. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.23 | Low | Disc headings depend on an English phrase inside a subtitle | Web album disc grouping still reads discNumberFromSubtitle rather than entry.discNumber. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.27 | Low | Parts that exist but cannot be reached or do nothing | Native N1–N5 panels are delivered. The original compound dormant/inert-control and catalogue-cleanup entry is not fully reconciled. [native-owner](../../../../_program/reports/native-owner.md) [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.29 | Low | Removing an entry from a playlist or collection is immediate, with no Undo | Saved-resource entry removal still has no visible occurrence-safe Undo restore. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.30 | Low | The episode panel loses facts on a direct visit and does not link people | Direct episode visit air date and guest/director/writer navigation remain incomplete. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-19.8 | Low | The Android TV density override has gaps and wide side effects | Android TV rotation/font-scale density relies on internal/runtime metrics; hardware behavior remains unqualified. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-19.10 | Medium | A television that does not report "television" gets a touch layout with no visible focus | Fire/Leanback supplies TV facts; generic boxes without those facts still need an explicit layout choice. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-19.15 | Low | The test harness leans on a React internal, and native tests are not run | Hook driver/type checks improved; React-internal imitation and Vega SDK CI remain gaps. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-19.19 | Low | Small contract leftovers | Runtime PiP gating exists; native search keyboard and Android adjustResize follow-ups remain. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-21.8 | Medium | Playing a duplicate playlist occurrence starts the first matching item | Duplicate playlist starts still need an occurrence/revision-fenced selector, not a loaded index/item ID. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-21.9 | Medium | Pre-play subtitles omit unknown-language tracks and merge distinct same-language choices | Native pre-play subtitle choices still lose exact discovery/resource identity and unknown language alternatives. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-21.11 | Low | Saved-view playback stops at 500 | Complete query/saved selectors exist on server/core; first-party screen Play/Shuffle handlers still need forwarding. [queue-and-retention](../../../../_program/reports/queue-and-retention.md) |
| B-25.10 | Medium | Apple/Vega and physical native behavior are not automated release gates | Apple/Vega release-build coverage and physical first-party acceptance are not complete. [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-25.12 | Medium | GPL Apple distribution rights remain unresolved | Component-specific Apple distribution rights and final binary notices/source/relinking materials are unreviewed. [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-25.21 | Medium | Cross-repo CI checks moving default branches, not recorded dependency revisions | Exact cross-repository dependency locks and native release workflows remain incomplete; Roku is excluded. [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |

### All IDs by domain and disposition

F = reported fixed; A = already fixed on the base; N = reported not present; I = intentional under current decisions; O = open/partial. Numbers after each domain are that domain's bug suffixes, so `01: 1` means B-01.1. This compact ledger preserves every ID without 588 repetitive paragraphs.

**01.** F: 2, 4, 6, 7, 8, 10, 11, 12, 13, 15, 16, 17, 18, 19, 20, 21, 22; A: 1, 3, 5, 9, 14; N: —; I: —; O: 23.

**02.** F: 1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 13, 14, 15, 16, 18, 19, 20, 21, 22; A: 12, 17; N: —; I: 8; O: —.

**03.** F: 2, 8, 9, 10, 11, 12, 13, 14, 16, 18, 19; A: 1, 3, 4, 5, 6, 15; N: 7, 17; I: —; O: —.

**04.** F: 1, 2, 4, 5, 6, 7, 8, 9, 11, 12, 13, 14, 17, 18, 19, 20, 21, 22, 24, 25, 26, 27, 28, 29, 30, 33, 34, 36, 37, 38, 40; A: 3, 10, 15, 16, 23, 31, 32, 39; N: —; I: —; O: 35.

**05.** F: 2, 3, 4, 6, 7, 8, 9, 10, 11, 13, 14, 15, 16, 17, 18, 20, 21, 23, 24, 25, 26, 27, 29, 32, 33; A: 1, 5, 12, 19, 22, 28, 30, 31; N: —; I: —; O: —.

**06.** F: 2, 4, 7, 8, 9, 15, 21; A: 1, 3, 5, 6, 10, 11, 12, 13, 14, 16, 17, 18, 19, 20, 22, 23, 24; N: —; I: —; O: —.

**07.** F: 2, 3, 4, 6, 7, 9, 11, 13, 14, 15, 16, 17; A: 1, 5, 8, 12, 18; N: —; I: 19; O: 10.

**08.** F: 1, 2, 3, 4, 5, 6, 7, 8, 11, 13, 16, 17, 18; A: 10, 14, 15; N: 9; I: —; O: 12.

**09.** F: 1, 2, 4, 5, 7, 8, 9, 10, 12, 13, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 28, 29, 30, 31, 32, 33, 34; A: 3, 6, 11, 14, 15, 16, 27, 35; N: —; I: —; O: —.

**10.** F: 1, 2, 3, 4, 6, 9, 10, 12, 13, 14, 15, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 29, 30, 32, 33, 34; A: 5, 7, 8, 16, 17; N: 31; I: —; O: 11, 28.

**11.** F: 3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 14, 15, 16, 18, 19, 20, 23, 24, 25, 26, 28, 29, 30, 32, 33, 34; A: 1, 2, 12, 17, 21, 22; N: 31; I: 35; O: 27.

**12.** F: 1, 2, 3, 4, 5, 6, 7, 8, 9, 11, 12, 13, 14, 15, 16, 18, 19, 20, 21, 23, 24, 25, 26; A: 10, 17, 22; N: —; I: —; O: —.

**13.** F: 1, 4, 5, 6, 7, 8, 9, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23; A: 2, 3; N: —; I: —; O: 10.

**14.** F: 1, 3, 4, 5, 6, 7, 8, 9, 10, 12, 13, 15, 16, 17, 18, 19, 22, 23, 24, 25, 26, 27; A: 2, 11; N: —; I: —; O: 14, 20, 21.

**15.** F: 1, 3, 4, 5, 6, 7, 8, 11, 12, 15, 16, 17, 18, 21, 23, 24; A: 2, 10, 14, 20; N: —; I: 9, 13, 22; O: 19.

**16.** F: 18; A: 1, 2, 3, 6, 8, 9, 11, 13, 15, 16, 19, 20, 21, 22, 24; N: —; I: —; O: 4, 5, 7, 10, 12, 14, 17, 23, 25.

**17.** F: 1, 2, 3, 4, 5, 6, 7, 8, 10, 11, 12, 13, 14, 15, 16, 17, 18, 20, 21, 22, 23, 24, 25, 26; A: 9; N: —; I: —; O: 19.

**18.** F: 4, 13; A: 1, 2, 3, 5, 6, 7, 10, 11, 12, 14, 15, 16, 17, 18, 19, 20, 21, 24, 25, 26, 28; N: 8; I: —; O: 9, 22, 23, 27, 29, 30.

**19.** F: 9, 12, 14, 16, 18; A: 1, 2, 3, 4, 5, 6, 7, 13; N: 17; I: 11; O: 8, 10, 15, 19.

**20.** F: 1, 2, 4, 7, 9, 10, 11, 12, 14, 15, 17, 18, 19, 20, 21, 22, 23; A: 3, 5, 6, 8, 13, 16; N: —; I: —; O: —.

**21.** F: 6; A: 1, 2, 3, 4, 5, 7, 10, 12, 13, 14, 16; N: —; I: 15; O: 8, 9, 11.

**22.** F: 15, 22; A: 1, 2, 3, 4, 5, 7, 8, 9, 10, 11, 12, 13, 14, 16, 17, 18, 20, 23, 24, 25, 26; N: —; I: 6, 19, 21; O: —.

**25.** F: 2, 5, 6, 7, 9, 11, 13, 14, 15, 16, 17, 18; A: 1, 3, 4, 8, 19, 20; N: —; I: —; O: 10, 12, 21.


## Appendix E. Remaining choices and their alternatives

These complete section 3. They are implementation choices, not extra owner decisions.

### Credentials, privacy and recovery

- **Live permission edits keep sessions alive.** Disable/delete, password replacement and PIN changes still end affected sessions. Clients quietly refresh live roles. New-device approval exempts the owner but not administrators. The alternative is broad sign-out on each policy edit.
- **A secondary profile cannot manage account devices/remembered sign-ins, but may still approve a TV with its own role.** TV approval follows your explicit role decision. The alternative is primary-profile-only TV approval.
- **Remote policy is evaluated at new sign-in using the receiving device's locality.** Renewal/profile selection are not new sign-ins; allowed hours still apply. Under Owner only, valid and invalid member passwords remain distinguishable by response. The alternative leaks the owner's username by refusing before password verification, or uses a broader uniform response redesign.
- **Allowed schedules without a timezone use server time.** Outside-hours refusals preserve credentials. The alternative is UTC schedules or immediate session revocation.
- **Crash reports are captured locally and sent only on request.** Owner reports are bounded/redacted. The alternative is automatic server upload; none was added.
- **Root revocations are permanent learned facts.** Returning to a revoked signing key is not valid rollback. Missed fleet delivery follows existing daily/weekly check-ins, not the Hosted process's 30-second fence. The alternative would need new fleet delivery guarantees.
### Library and personal state

- **Retained personal state is bounded at 100,000 removed titles per profile, oldest first, without a time limit.** It includes watched/progress/rating/favourite/My List/count, not every per-play row or playlist occurrence. The alternative is a larger/unbounded archive or a timed expiry.
- **Delete means physical unlink, with approved-set checks.** Bulk file deletion has preview and typed confirmation; a changed/shared file can be kept. A container action covers at most 5,000 titles, selection requests 200. “Delete but keep files” still permits a later scan to return the title. The alternative is removing the keep-files action or a durable large-job design. No catalogue trash folder remains.
- **Anime alternate order uses explicit signed offsets.** Local number plus offset maps to an owner-selected provider order; seasonal files keep base order. Ambiguity becomes Needs order for that file. The alternative is guessed offsets, with wrong-match risk.
- **Tag lists retain genuine artists and punctuation.** Album release IDs win; fallback is album artist/title/year. Generic tags are too weak to merge audio. A matching TXT sidecar is lyrics unless recognized as a rip log. The alternative is heuristic artist splitting or narrower lyric suffixes.
- **Complete scan prepares useful navigation/audio features, without default checksum/fingerprint diagnostics.** Custom retains those operations. Disc folders are represented truthfully but are unplayable. The alternative is heavier Complete or actual disc-playback development.
- **Mark container is atomic and has a 30-second Undo.** It resets member resume and completion intent; Undo refuses later edits/playback/membership changes. Sustained unfinished rewatch clears watched until completion. The alternative is preserving old member exceptions or changing rewatch semantics.
- **Credits completion falls back to the chosen percentage without an approved current marker.** Early known-duration EOF preserves resume; unknown-duration natural end remains accepted. The alternative is requiring markers or rejecting unknown-duration terminal evidence.
- **Books complete within 30 seconds of end, capped at 5% for short files; resume is retained.** Resume rewinds 15 seconds after five minutes away, 30 after an hour. Explicit seeks stay explicit. The alternative is completion percentages or no comfort rewind.
- **Popular requires three eligible, opted-in viewers.** Recommendation day uses server local date. Continue limits use the largest participating library maximum; film resume does not age out; show detail ignores Home dismissal/age. The alternative is different popularity/privacy/day/continuation rules.
- **Filters apply live and Back keeps them.** Tab choices are remembered per viewer/server/device; explicit links do not overwrite memory. An old browser tab does not silently follow another profile. The alternative is draft filters or automatic cross-tab profile switching.
- **Only one sort is accepted per browse request.** Random is a stable seeded index rotation; file size/resolution and certain container metrics use member maxima. Valued personal rows precede unvalued title-order rows. Aggregate “Recently added” means title addition, not private viewing. The alternative is new compound/query-specific maintained orders.
### Playback, rendering and licensing

- **Fixed quality is a ceiling; Original can use a quality-mode conversion.** Current ordinary planner rates are the governing ladder. A member cap lifted later can widen the next plan; lower limits always apply. Versions are compared for least processing, then resolution/bitrate, with at most eight compared. The alternative is forced conversion, fixed historical session limits or wider version enumeration.
- **The later decision pass corrects two earlier choices.** Automatic must start at Original even with a throughput estimate, and profile-wide PIN lockout contradicts your no-lockout policy. Both are reported corrected in that lane, included in final server health. Existing per-attempt/input admission bounds remain; the alternative is retaining behavior that conflicts with your direction.
- **Automatic is client-driven.** It reacts to sustained shortfall/stalls and cautious recovery, with a 15-second change gap and increasing backoff after a failed upgrade. Direct files react mainly to stalls. A low-throughput refusal can be retried without the estimate. The alternative is server probing or more aggressive adaptation.
- **Unknown analyzed-file bitrate uses a conservative resolution estimate.** Unknown output routes never enable passthrough. Software decoding tiers are estimates. The alternative is refusing more media until measured facts exist; actual hardware limits still need calibration.
- **No approximate Dolby Vision conversion.** A compatible base layer may be used; no-base profile 5 requires qualified reshaping. Lossless audio output is preferred where accepted, even from a lossy source, to avoid another lossy encode. The alternative is broader but degraded fallback.
- **Remember failures per engine/file/device.** Definite delivery faults last seven days; generic faults six hours. Web Streaming has the playback-problems list/Forget; native needs the panel. The alternative is shorter/no memory or automatic broader retry.
- **Native platform players declare no embedded audio switching.** Track changes use session re-plan so choices are remembered; mpv server-audio changes can reload briefly. Player-setting changes now re-plan in the same session/queue entry, retaining position and choices. The alternative needs exact choice/settings APIs that retain all session options without reload.
- **Native mpv network reads use system-validated relays.** Grant-scoped reads, bounded HLS rewriting and no redirects keep credentials away from mpv's own TLS stack. Engine release waits for actual teardown rather than retaining an idle handle. The alternative is a materially different native transport/ownership design.
- **Android HDR direct output gives way to SDR when mpv must draw styled/image subtitles.** Phones/TVs use different decoder paths; frame-rate switching follows system policy. Mixed-script/untagged fallback fonts remain limited. The alternative is an upgraded rendering/font build; physical HDR validation comes first.
- **Web styled subtitles render at at most 1440 lines, on a canvas capped at 4K.** They keep authored position and use a device-local vertical adjustment. iPhone fullscreen cannot carry that overlay. PiP/casting do not yet switch subtitle mode. The alternative is a lower cap, full-resolution cost or complete mode-switch integration.
- **Web bitmap support is bounded and partial.** DVB character/progressive objects and VobSub in-cue colour changes are omitted; forced flags have no viewer-only filter control. Browser Korean fallback is absent. The alternative is expanded parsers/fonts and real corpus tests.
- **Web seeks are local and one video element survives title changes.** iPhone uses native HLS; Mac/iPad Safari use hls.js first. Frame step assumes 24 fps; End goes five seconds before end. Up next during credits is an offer, never automatic playback. The alternative is broader native-HLS/AirPlay support or new source-rate/control semantics.
- **Normalization never guesses missing gain or peak.** Unity when unavailable; album falls back to track; amplification requires peak and caps at 32×. Unindexed lossy deep web seeks fall back to ordinary playback, losing optional effects. The alternative is more analysis/indexing rather than inaccurate sample claims.
### Operations, integrations and shared media

- **Timeshift stays at 30 minutes/1 GiB and reports actual span.** High bitrate yields less rewind. New live readers start at the current input chunk. The alternative is owner-adjustable retention/history replay with extra resource work.
- **Guide overflow skips excess rows; oversized documents are refused.** Large guides spool to private SQLite; selected groups/channels filter before staging. The alternative is larger limits after load qualification, not removing hostile-input bounds.
- **Download receipts renew daily.** Online revocation ordinarily reaches a device within a day; disconnected copies rely on their receipt lifetime. Transfer addresses expire after 24 idle hours and can renew within seven days. Original files are hashed once then checked by size/mtime; device digest catches hidden mutation. The alternative is more frequent renewal or copying originals.
- **Multipart titles cannot become one download; one readable version can.** Unfetched claims expire after the existing 30-day default, freeing server claims rather than device files. No automatic Android restart while process-dead was delivered. Device receipt expiry remains wall-clock based. The alternative is bundle-of-parts, JobScheduler and a persisted trusted monotonic clock.
- **Watch Together waits up to five seconds with a 750-ms readiness band.** Play can bypass the wait; buffering stays local; private participation is anonymous but occupies a visible room slot. Non-hosts leave after ten minutes; connected ready members may claim an absent host after two. The alternative is a more elaborate room redesign, outside scope.
- **Cast recovery is installation-local and same-profile.** A sender gets a private token; duplicate TV names still need better proof-storage identity. The alternative is a stable receiver-ID store or new cross-profile control policy.
- **Four integration scopes remain flat.** Admin covers everything allowed to keys; playback includes personal media state/DVR, library can report changed paths without catalogue reads. Keys use ordinary connections rather than the console reserve. URLs are server-visible paths with no built-in mount mapping. The late security sweep narrows cross-member owner transport from playback to admin scope while leaving own-profile controls at playback; that fix is included in final server health. The alternative is more scopes or mapping configuration.
- **Webhooks allow private destinations, never redirects and never auto-disable a failing destination.** Each has bounded queue/logs; full queues drop new events. Title-added waits about 45 seconds for metadata; private recording libraries are excluded from title events. Some container-only metadata changes still lack title-updated events. The alternative is different event coverage/backpressure policy.
- **MCP calls the same API with chosen credentials.** It has no enable switch and accepts keys/tokens only. Some control/attention tools were added by follow-ups; no claim of arbitrary absent backend operations. The alternative is a separately authorized protocol surface, which would conflict with one-API simplicity.
- **Recovery serves a plain HTTP status page while database/TLS settings are unavailable.** It offers no unauthenticated repair. An invalid custom FFmpeg keeps console access and falls back to default tools, with requested/actual status. The alternative is no recovery listener or stopping all service on tool rejection.
- **Update notices use the official feed with opt-out; custom feeds may be private.** Network policy pins DNS and confines redirects. No signing identity is generated; release artifacts need an approved pinned key and final inventory. The alternative is manual-only update checks or a different signer ceremony.
- **Web Integrations groups owner controls and hides owner personal-token creation in favour of keys. Native Profile exposes personal tokens behind one collapsed row.** Expiry has named choices; merge uses a two-title selection; deletion uses server preview; storage locations are read-only; Receipt Undo now appears from pages and card menus on web/native. The alternative is different screen placement/control affordances during frontend completion.


- **Optional spoiler controls and writing metadata beside media were not introduced.** The owner had not approved those optional proposals; the later decision matrix records conservative deferral. The alternative is an explicit product choice followed by new projection/storage/control work. This is separate from the requested completed library/provider flows.

### Additional bounded defaults and small consumer choices

- **Integrations have fixed count/lifetime caps:** 100 keys/server, 25 tokens/member, ten-year expiry horizon, 20 webhooks, 1,000 pending deliveries and 200 log entries/destination. Credentials default to four ordinary plus two long-lived requests in flight. The alternative is owner-adjustable larger caps after resource qualification.
- **Provider pace is deliberately conservative.** Gentle/Normal/Fast use bounded request and title concurrency; MusicBrainz stays at one request/second and AniList stays below its documented allowance. Retryable outages eventually back off from 15 minutes to six hours, respecting Retry-After; authentication/validation failures wait for correction. The alternative is different measured provider budgets, not unrestricted retry.
- **Owner diagnostics are bounded rather than complete exports.** Client reports cap at 100 total/five per device, 256 KiB/report and 1 MiB/account/day. Conversion failures keep the newest 200 for 30 days; catalogue rebuild failures show the latest 100 with a More indication. The alternative is pagination/export and another retention policy.
- **Scan bookkeeping keeps twenty summaries/source and 10,000 changes or 30 days.** Watchers coalesce after two quiet seconds/ten continuous seconds, with 64 roots and 4,096 directory watches. Failed/network watches fall back every five minutes; local safety inventory every fifteen. Expired change cursors require rereading. The alternative is different bounds/cadence after platform/load tests.
- **File List Only may read ignore configuration.** It still avoids media/local metadata analysis. Owner ignore changes can make old entries missing; unreadable/changing evidence protects only its affected subtree. Extras need one unambiguous owner and retain no automatic owner beyond bounded evidence. The alternative is stricter configuration-only intake or an explicit owner selection.
- **Optional visual analysis tolerates up to 5% decode errors.** Exact audio/loudness/fingerprint paths remain strict. Source changes discard results; qualified Dolby Vision is required. The alternative is abandoning optional images sooner or relaxing exact audio accuracy, which was not done.
- **Subtitle cold fills are shared and bounded.** Two fills run concurrently, 64 can be in flight; first-reader cancellation does not cancel shared extraction. Original/font attachment names never become paths. Provider language falls back through the viewer's list then English; bad credentials stop retry. The alternative is eager extraction, different concurrency or explicit provider language controls.
- **The native reinstall reset is exact and protected-data-aware.** It deletes only six Portico Keychain services when an installation marker is absent; established locked installations keep ordinary background behavior. Unreadable protected state waits for retry rather than erasing itself. The alternative is broader first-launch deletion, which risks real background sessions.
- **Kept navigation history freezes rather than being discarded.** Retained-history memory is not thereby bounded. Runtime Fire/Leanback facts decide TV layout; generic large-screen guesses are not used. Shipped regional English keeps existing plural fallback; more languages need real Hermes/Vega qualification. The alternative is bounded history, explicit layout choice and a separately qualified language expansion.
- **Invitation links can wait thirty minutes while signed out.** Other held links keep ten minutes; duplicate-delivery windows are five seconds or ten for initial launch. Key-loss reset asks first and keeps downloads; after reset JavaScript reloads. The alternative is shorter links or automatic destructive recovery.
- **Native network admission reserves control within 76 requests and ten active reads.** Apple uses separate socket pools; Android uses separate dispatchers and one-shot writes. Ordinary reads/changes/console requests have distinct deadlines including waiting time; deliberate streams retain their own lifetimes. The alternative is another bounded admission model after real socket-load tests.
- **Presentation parsing skips only descriptive units.** Unknown authority/identity/cursor/revision/limits and uncertain write receipts remain strict. Download unknown quality names remain readable; an unnamed claim remains listed rather than hidden. The alternative is whole-document rejection or dangerously guessed authority.
- **New restricted generations may show protected empty content.** Old readable content stays during ordinary rebuild; unpublished restricted anchors are omitted. Expired signed cursors become scoped positioning hints. Kept documents are memory-only, failures wait fifteen seconds, hidden feeds stop after thirty seconds unless playing. The alternative is persisted snapshots or more polling, with privacy/resource costs.
- **Late stale settings fixes compare edited keys, not just revision numbers.** Unrelated changes may retry quietly; genuinely changed keys preserve the draft and conflict. Queued/late player operations are bound to their original session. The alternative is unsafe full-document replay, which is rejected by the review.
- **Optional/development protocol contracts remain separate.** Two development operations compile only in the development trust build; twelve retained consumer protocols do not add production handlers. Special subtitle operations retain explicitly named microseconds even though general media fields use milliseconds. The alternative is completing/retiring those protocols and a deliberate specialized-unit rewrite.
- **DVR picker continuation may return an empty page with a cursor.** It advances by examined source ordinal without disclosing denied IDs. Clients must continue until the cursor ends. The alternative is unbounded filtering work to fill every page, which would harm responsiveness.
- **Large owner notices walk fifty profiles/checkpoint and resume for fourteen days.** The starting last-profile bound excludes newly created profiles from that delivery promise. Profile download revocation walks at most 100 claims/receipts per call. The alternative is larger transactions or a new durable job API.
- **Live guide parsing retains narrow hard bounds even when document limits rise.** Manual preview/save retains a 48 MiB body; huge remote guides use streamed ingestion. Entity declarations remain refused; missing stops are inferred, overlaps trimmed and conflicting repeat flags prefer Repeat. Catch-up metadata is retained without catch-up playback. The alternative is new import/catch-up workflows, not lifting parser safety.
- **Software/output assumptions remain conservative.** Apple combines H.264 bit depths under one lower software ceiling when the document cannot express per-depth bounds. Android direct HDR is contingent on the actual surface/decoder path; television render resolution depends on the device tier. The alternative is richer per-depth/output facts and qualified native libraries.
- **Ordinary audio preserves known rates/depth and uses 48 kHz when unknown.** AC-3/E-AC-3 encoding stays at 5.1; compatible 7.1 copying or FLAC/AAC/Opus is preferred where admitted. Web decoder alignment is probed before admitting optional effects. The alternative is stronger codec qualification/indexes rather than guessed sample accuracy.

### Later presentation and workflow choices

- **Music Favorites within one library shows albums**, including albums containing a favourite song. A song is not separately listed there. The one-library toolbar replaces narrow saved-list controls and omits their old Progress choice. Alternative: add a true cross-library saved-list surface retaining every saved-list metric.
- **Local metadata credits do not create people pages.** They cannot link to a canonical person in the current local-only fixtures. Alternative: define stable local person identity and page/search publication; this is an unresolved product choice, not a hidden provider requirement.
- **Native TV fade starts at 35% opacity** so a new row can receive focus. It skips first display, scrolling, Reduce Motion and changes above 60 cards. Large windowed grids do not fade. Alternative: publish stable full-key changes for those grids and qualify focus before widening animation.
- **Native VobSub uses .idx with its .sub beside it.** A pair is offered only when both are present; existing download markers request the corrected companions again. Styled scripts can be offered before all fonts arrive, but local font consumption is still missing. Alternative: require every font before offering styled subtitles.
- **Restore asks for confirmation without a typed word.** Backup delete asks for delete; cleanup uses the folder name; media removal uses the server’s preview token. Background manual backup while a scheduled backup runs shows that job’s progress. Alternative: require a typed restore phrase or add a separate concurrent backup operation.
- **Webhooks on native switch Off inside Edit, then Save.** Secrets/reports use system Share. Viewing hours use typed time, saved unusual day sets are retained, and file IDs are not printed on the native Files list. Alternative: new pickers/row controls and catalogue labels. Webhook concurrent edit remains last-write-wins until the server adds a revision.
- **TV load-history sparklines were removed with Dashboard actions.** Phone/web status tooling remains. Alternative: separately approve extra read-only TV charts; broad TV administration stays excluded.
