# Live TV, tuner and IPTV acceptance

## Purpose and evidence

Updated for the evening implementation and owner decisions. New cases remain qualification work; known missing candidate implementation is Fail, while an unavailable test prerequisite is Blocked. Native-owner recorded 655 native/87 Vega before the mirror; the later running log confirms 655/87 again after the mirror with clean type checks and equal packages. Physical acceptance remains unrun.

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Private conversions-capable installed server on Linux then Windows/macOS candidates, actual HDHomeRun with one available tuner and multicast network, lawful private IPTV account plus controlled M3U/XMLTV/HLS test origin. Two separate profiles/clients and large/slow disk; disposable small recording volume for full-disk case. Real TV receivers/native/browser engines for codec acceptance. Never overload a real provider: large malformed/scale lists come from the controlled fixture origin.

## Media and accounts

For ordinary playback, make synthetic samples with the candidate's qualified FFmpeg/ffprobe pair. Run in a new writable fixture folder (never over existing media):

```sh
ffmpeg -f lavfi -i testsrc2=size=1280x720:rate=24 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 19 -c:v libx264 -g 96 -keyint_min 96 -sc_threshold 0 -c:a aac -movflags +faststart 'Fixture (2026).mp4'
ffmpeg -i 'Fixture (2026).mp4' -c copy 'Fixture (2026).mkv'
ffmpeg -f lavfi -i sine=frequency=440:sample_rate=44100 -t 120 -c:a flac -metadata artist='Fixture Artist' -metadata album='Fixture Album' -metadata title='Track 01' -metadata track=1 'Track 01.flac'
```

Make three TV copies named `Fixture Show/Season 1/Fixture Show - S01E01.mkv` through `S01E03.mkv`; make a second version/edition copy and a two-part film. Write a UTF-8 sidecar `Fixture (2026).en.srt` containing `1`, `00:00:02,000 --> 00:00:05,000`, and `Fixture caption`, separated by newlines and followed by a blank line. For long buffering/background/download cases generate a 20-minute/2+GB synthetic file by raising duration/bitrate and verify its hash before testing; a 19-second clip cannot establish those cases.

For HDR10/HLG/Dolby Vision 5/7/8, TrueHD/DTS-HD/Atmos, PGS/VobSub/DVB, HE-AAC/SBR and attached-font ASS use legally held test clips/disc or broadcast extracts. Log provenance, ffprobe facts and a correctly rendered reference frame/audio/channel map. SDR bars with HDR tags are not HDR-fidelity evidence. The server checkout has `fixtures/audio-gapless/` and `fixtures/audio-gapless/generate-portable.py`; preserve its original corpus and generate extras with `python3 fixtures/audio-gapless/generate-portable.py --out /tmp/portico-audio-extra`. Obtain measured HE-AAC/PS fixtures separately. Missing real formats block their cases. Make bad-input copies separately: truncate a copy, corrupt a metadata row, or insert a malformed subtitle cue between good cues. Record which data changed.

## Build or install

Install verified candidate package with writable recordings path and qualified FFmpeg/ffprobe; on systemd add only selected ReadWritePaths plus portico permissions. Owner opens Server > Live TV/Sources, Discover/Add tuner or add M3U URL and XMLTV URL, Preview then Save; select groups/channels and Refresh. Record device/model/count/firmware and source settings. Make controlled M3U with #EXTM3U, #EXTINF:-1 tvg-id="fixture" tvg-name="Fixture" group-title="News",Fixture followed by a reachable stream URL; XMLTV has <tv><channel id="fixture"><display-name>Fixture</display-name></channel><programme channel="fixture" start="YYYYMMDDhhmmss +0000" stop="YYYYMMDDhhmmss +0000"><title>Fixture Programme</title></programme></tv> using actual UTC times spanning the run. Use candidate qualified ffmpeg to loop synthetic media into the controlled stream service; inspect source request count independently. HLS variants need TS/fMP4 masters, AES128/range/optional LL tags and nonzero origin. Provider credentials remain private. If preview fails because source/tool/permissions unavailable, fix that named prerequisite; do not remove parser/entity/expanded-size bounds.

## Smoke subset

Run cases 1, 2, 4 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. Shared tuner/input and private DVR ownership — smoke

1. With one tuner start two viewers on same channel and two profiles recording same airing. Inspect physical tuner and upstream requests/one capture inode. Let one viewer use converted output, another copy.
2. Stop/seek one viewer, cancel/delete recording leader mid-programme, leave follower. Try different channel while capacity occupied; rapid same-channel rejoin and later close all.
3. Read recording from wrong profile, remove one owner and play remaining copy, then final owner and wait readers retire. Refresh only guide while playing.

**Expected:** One input per channel/capture, independent viewer conversions, other consumers uninterrupted, earlier follower bytes retained, private authorization; capacity refusal names occupants honestly. Grace reuse/retire correct, one shared byte count, final physical bytes removed only after readers retire; guide refresh no retune.

**Failure:** Tuner multiplied by viewers, canceled owner publishes recording, shared file deleted early, other-profile access or guide refresh closes stream.

**Route to:** livetv, review-livetv, console.

### 2. DVR restart, interrupted publication and real disk pressure — smoke

1. Record across worker/server restart inside airing window, withdraw leader then restart; inspect gap/incomplete labels and follower prefix. Repeat restart outside window.
2. Use a disposable low-space recording filesystem; exhaust then free space while recording/scheduled retry. Observe finite retries/start tolerance and no fake recovered interval.
3. Use the focused recovered-object regression for crash between sealing bytes/publication and temporary link/probe/DB failures; actual deterministic crash injection needs a qualification hook and is Blocked without it.

**Expected:** Restart locates durable object/prefix, canceled state retained, capture resumes within window with honest gaps; outside remains incomplete. Transient failures retry and terminate honestly after configured attempts (review tested three), low space can start late partial without source forever retained.

**Failure:** Old prefix replaces sealed bytes, canceled recording returns, missed interval claimed recovered, full disk loses unrelated files or retry spins.

**Route to:** livetv, review-livetv, leftovers-server-b; fix-pass identity-ops-live.

### 3. Ratings and authority change during recording

1. Schedule an allowed programme, then lower account/profile ratings or withdraw source/library recording authority while running. Inspect all list/detail/overlap/preview/receipt replays as that member.
2. Cancel guide programme through TV/phone UI, Back once then confirm; repeat while running and inject denied cancel on private proxy.

**Expected:** No forbidden title/description/episode/item metadata in reads/replays, restricted new/running work refused/stopped; allowed cleanup still works. Safe confirmation Cancel focus, recorded-so-far consequence and failure inside sheet.

**Failure:** Restricted capture continues/publishes, hidden metadata leaks or cancel bypass/ambiguous partial consequence.

**Route to:** review-livetv, accounts-security, console; fix-pass native.

### 4. Tolerant large playlists/guides and hostile bounds — smoke

1. Serve30000+ M3U channels/200+MiB expanded guide through controlled gzip/chunked slow origin; include one bad EXTINF/missingURL/duplicateID/rotating query token/BOM/legacy text/harmlessDOCTYPE/missing stop/unknown element.
2. Preview/save/refresh while browsing old guide; select News-only then picker-search Sports/choose two/Bring all; lower Most channels/guide rows/expanded MiB, refresh then raise.
3. Use separate copies with entity/internal subset, deep nesting, giant field, gzip expansion overflow and repeated Unicode aliases; test explicit mapping authoritative and stale generation cursor refusal.
4. Make allowed DVR channels occur after >256 excluded entries. Follow all picker continuations, including empty pages; change guide generation and send malformed cursor on separate requests.

**Expected:** Good rows retained with skip/limit reasons, old guide available through checkpoint publication, selection correct/choice pages not limited to currently enabled groups. Harmless data tolerated; hostile whole document fails bounded/no external entity access; alias work bounded. Picker examines at most256 candidates/page and returns≤30 visible choices; all permitted channels exactly once and no excluded-ID cursor disclosure.

**Failure:** One odd row rejects all, expanded-size unbounded, partial guide visible, alias cross product, hidden omitted groups or old cursor accepted.

**Route to:** livetv, review-livetv, limits-settings, server-followups, console, review-server-late.

### 5. Live codec/HDR/transport and slow reader isolation

1. Tune native HEVC HDR/MPEG2/AC3/EAC3/AAC5.1 per actual device capability, Original then lower quality; inspect audio-only conversion retaining HDR copy.
2. Use controlled HLS variant selection/AES128/key/range/LL tags and share acquisition count; slow progressing startup>30s then idle stall, dropped network within/beyond retry window.
3. Pause2min/resume, seek outside live window/go-live/channel up/down, cross hour programme boundary, keep one lagging viewer while another watches. Try fMP4-only client without live publisher support.

**Expected:** Copy supported tracks, convert only needed ones; progressing input budget renews, abandoned opener closes after grace, each reader span bounded and lagging loss independent. Programme boundary keeps stream; unsupported live-fMP4 refuses honestly until implemented.

**Failure:** HDR downgraded on audio-only conversion, whole stream retuned by guide, one slow viewer breaks others, wrong variant or advertised unproducible fMP4.

**Route to:** livetv, review-livetv, native-playback, web-player, vega, planner-wiring.

### 6. Schedules, reruns, guide-only refresh and source changes

1. Schedule back-to-back channels with padding/overlap priorities; same programme two profiles, reliable rerun and unknown episode; inspect show/season/episode/description private library publication.
2. Record source revision/session during guide-only refresh, then change URL and finally disable source; inspect programme and upstream.
3. Run qualified source tests only: GOFLAGS=-p=2 go test ./internal/httpapi -run "TestChannelsALiveTunePlays|TestChannelsAnExhaustedTunerRefusesTheNextViewer" -count=1 -v with actual PORTICO_PLAYBACK_TEST_HELPER/media tier as documented in tests.

**Expected:** Honest capacity/priority/start tolerance/rerun decisions and correct hierarchy; guide-only update no retune/revision restart, URL revision retunes and disable yields source_disabled. Named media tests execute rather than SKIP.

**Failure:** Duplicate tuner, incorrect programme edition/episode, no retune after URL change or guide refresh consumes counted start failure indefinitely.

**Route to:** livetv, review-livetv; fix-pass identity-ops-live.

### 7. Import/capture load, services and retention

1. Generate five-million-row admitted guide on controlled origin while live capture, scan and conversions run; keep owner/browse/console requests and low connectionLimit active.
2. Measure temporary disk/RSS/mapping time/checkpoint lease, thousands of recordings on slow disk and daily traversal. Stop/restart Windows/macOS/Linux service during active capture.
3. Check physical helper/reader/file retirement and cleanup, then real driver/HDR/surround output on those OSes.

**Expected:** Finite import memory/spool/lease/no partial generation, capture/browse/owner pause/terminate/restart reachable; sweeps paced. OS service drains/recovery/permissions actual, not inferred from cross-compile or fake statfs.

**Failure:** Input dropped by maintenance, unbounded spool/heap, console inaccessible, temporary files/helpers left or purported native success from build only.

**Route to:** livetv, review-livetv, limits-settings, ops-packaging, leftovers-server-b.

### 8. Late source screens, paging, countdown and viewer updates

1. On web and native phone/tablet add HDHomeRun with XMLTV, edit guide address after Check, save and inspect actual programmes. Batch-map >200 channels with number/guide-ID/hidden and group/channel filters; skipped rows are explained.
2. Restricted DVR picker traverses an empty excluded prefix with nextCursor without disclosing excluded IDs; follow until true exhaustion. Native/web guide/DVR refresh stays visible; web clock refresh is about 30 seconds where events are missing.
3. Reserve/preempt a channel while multiple viewers watch; observe reservationRequiredAtMs and private channel.reservation-required feed at each player. A visible truthful countdown is required; backend event alone is not UI acceptance. Test owner playback controls against one viewer without affecting shared input.
4. TV owner Dashboard remains read-only; administer sources from phone/tablet/web. Two recording owners keep private records and one capture; cancel/recover one without deleting the other’s bytes.

**Expected:** Correct source/mapping/pages and genuine viewer countdown/transport privacy, one input/private capture ownership. Missing player countdown remains an explicit Fail; current clock update is not falsely called a server event.

**Failure:** Guide omitted, partial map save, premature short-page exhaustion, excluded identity in cursor, no promised countdown or TV admin writes.

**Route to:** native-owner, web-owner, web-viewer, review-server-late, livetv.
