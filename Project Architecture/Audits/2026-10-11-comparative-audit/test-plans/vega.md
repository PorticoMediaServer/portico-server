# Vega acceptance

## Purpose and evidence

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Vega SDK0.24/CLI on PATH and Node22+, Vega OS1.2 physical Fire TV in developer mode (virtual device only build/navigation evidence). Matching candidate native repository and private server/phone; remote, HDR/source fixtures and optional debug trace sink. Current source refuses authenticated SDK transport for safety: success of password/code sign-in is BLOCKED, not expected on this candidate. Conditional cases preserve the original lane’s acceptance scope for a later qualified transport.

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

On a qualification copy check synchronized packages from native root, then `cd apps/vega`, `npm ci --legacy-peer-deps` (use the committed lockfile), `npx tsc --noEmit`, and `node scripts/vega.mjs run --release`. If no lockfile in a specifically reviewed candidate, use its recorded npm install --legacy-peer-deps procedure and record resolved inputs. Packages appear under build/{aarch64,armv7,x86_64}-release/*.vpkg; device list must select the actual target architecture. Use `node scripts/vega.mjs device status`, `logs 200`, `key left select` and `shot /private/evidence.png`. Path-space problems require PORTICO_VEGA_STAGE pointing at a writable path without spaces; stage/mirror through the wrapper. Missing SDK/adaptor/package/ABI is a build prerequisite, not a transport fix. For physical renderer traces only set debugConsole=true/debugSink privately in app.json and `node scripts/debug-sink.mjs`; virtual screenshots may stop updating once video draws. Restore production flags before packaging.

The code at apps/vega/src/bridge/transport.ts and event-stream.ts must refuse credential-bearing/authenticated requests before constructing SDK XHR because redirect suppression is unqualified. Never remove that refusal merely to run the later cases. Mark dependent cases Blocked until a reviewed native transport with validated TLS, redirect suppression and bounded cancel/deadline support exists. TV offline downloads/mpv/speed/gapless/crossfade/leveling/HDR/passthrough controls are not supported and must be hidden.

## Smoke subset

Run cases 1 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. First build, signed-out navigation and honest refusal — smoke

1. Build/install Release with debugger/Metro absent, cold/warm launch and use remote through signed-out discovery/loading screens.
2. Attempt direct password, refresh/profile/bearer control and authenticated stream through the existing focused transport/event-stream test files. On device attempt sign-in and inspect unavailable-device message.
3. Use recording redirect capture fixture for a307 credential endpoint once future transport exists, then301/302/308 and TLS/abort/deadline variants.

**Expected:** Current source builds and stays responsive, refuses credentials before SDK request and shows unavailable-device message rather than promise of retry. No credential capture. Future transport cannot proceed until no-follow/TLS/cancel boundary qualified.

**Failure:** Functional sign-in claimed despite refusal, credential-bearing SDK dispatch, endless retry or renderer locks during spinner.

**Route to:** vega, leftovers-native.

### 2. Code sign-in and identity — conditional

1. After approved transport replacement, choose Portico Account and local Quick Connect separately; phone Link a TV approves. Record receiving friendly name/class/platform and inherited role.
2. Block Hosted503/Retry-After/date/delay, verify code retained/backoff and recovery. Point saved address at different server and test globalHTTP/IPv6/trusted/untrustedHTTPS.

**Expected:** Conditional: Fire TV actual name, not Apple TV/vega; local policy/approval applies, HTTP admitted with identity proof before bearer, invalidTLS refuses. Current refusal means Blocked.

**Failure:** Wrong class/role, lost code, unsafe redirect/authentication or reported Pass without actual transport.

**Route to:** vega, native-sync, hosted-followups, leftovers-native.

### 3. Browse, roots, focus and capability document — conditional

1. After sign-in browse Home/library/title/Search/Saved, deep rows/tab/sort/filter/group-wide options, rail/back and profile/relaunch memory. Menu card once on release, Menu Play/Sign out no action.
2. Inspect capability v2: client.family=vega, one native w3cmedia engine, text subtitles, embeddedAudioSwitching=false, SDR/stereo max2; record probed versus declared HEVC bounds/decoder trace.
3. Change content from phone while focused fifth card/long row; background/reopen,35min/1MiB feed rotation and retained-root navigation; send report.

**Expected:** Conditional: focus/scroll kept, quiet current screens, supported per-scope options, unsupported controls hidden and device reports truthful capability/probe evidence. Report platform vega/television. Current auth block prevents acceptance.

**Failure:** Stale/density/focus jumps, undeclared feature shown, stale shared React shape assumed renderer integration or broad HDR/directHTTP claims.

**Route to:** vega, native-health, freshness-clients, grouping-sorts, leftovers-native.

### 4. Playback, quality and lifecycle — conditional

1. After qualified transport play H264/HEVC MKV AAC remux, unsupported MPEG2/VC1 conversion and trustedHTTPS directMP4; sameMP4 overHTTP follows admitted remux policy. Resume/seek20min/back.
2. Play AC3/EAC3 surround and HDR10: honest stereo conversion/SDR tone-map. Start Original then720p/Automatic, throttle3Mbps on20Mbps, restore and inspect throughput facts/session/generation/queue.
3. Home/return video and music, network loss20s,20min playback/performance and bounded error/earlyEOF.
4. After safe transport exists, hold seek, inject clamped landed position/out-of-order binding/retired seek and rate≠1; clock/play stays held until supported half-second landed acknowledgement or bounded error. Cancel SSE before headers: pending open rejects once, not stuck.

**Expected:** Conditional: supported remux/direct/conversion matches plan, correct SDR/stereo, resume retained, native quality replans same queue; video pauses/background music continues if declared. Current transport refusal blocks these cases.

**Failure:** Black/grey/silent output, zero-position restart, fake throughput adaptation, false watched after disconnect or unqualified HDR/receiver behavior.

**Route to:** vega, playback-core, planner-wiring, review-native-js.

### 5. Subtitles, Live TV, Together and Play on — conditional

1. After transport qualification choose SRT/appearance/offset then ASS/PGS burn/off/next episode. Tune live, pause2min, seek/go-live, stay across programme boundary/10min and inspect hls carry trace.
2. Join phone group, play/pause/seek35min across feed rotation; host paused-title change/slowseek/end protection. Enable Receive from other devices, phone Play on/approve then remote controls/reconnect.
3. Repeat unreadable/bad media grants and stale callback/network route changes; no bearer redirect and no private viewer/title disclosure.

**Expected:** Conditional: captions/canvas/time and burn correct; live window/programme boundary healthy; group timing/focus and handoff authority correct. Until auth transport is supplied all dependent cases remain Blocked.

**Failure:** Burn missing, programme boundary freeze, polling drift, wrong-device handoff or safety refusal bypass.

**Route to:** vega, social, livetv, subtitles-text, leftovers-native.
