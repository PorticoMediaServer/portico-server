# Lane report: android-mpv

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/native-android-mpv` (branch `audit-implementation/android-mpv`, repository `portico-react-native`, 15 commits over `cb0bf85`).
Status: COMPLETE as written code. **Nothing here has been built with Gradle or run on a device.** What was checked is listed in section 8; treat the first build as the start of testing.

## 1. Outcome

- **mpv is a second player engine on Android phones and tablets, Android TV and Fire TV.** It is its own native module and view (`PorticoMpvPlaybackModule`, `PorticoMpvPlayer`) beside the Media3 ones and implements `Native/ENGINE-CONTRACT.md`: the same lease, commands, facts, rules and error texts, ported rule for rule from the Apple engine. The JavaScript the Apple lane wrote drives it with no Android branch.
  - Kotlin, in `apps/native/android/app/src/main/java/tv/getportico/portico/native/`: `PorticoMpvController.kt` (the engine), `PorticoMpvCore.kt` (the libmpv wrapper), `PorticoMpvRelay.kt` (network), `PorticoMpvSupport.kt` (validation, track matching, font fetch), `PorticoMpvDevice.kt` (device facts, app visibility, default font), `PorticoMpvPlayerView.kt`, `PorticoMpvPlaybackModule.kt` (module and media-session player).
  - A new Gradle module, `apps/native/android/portico-mpv`: fetches the pinned libmpv, and builds our small JNI bridge (`src/main/cpp/portico_mpv_jni.c`, `PorticoMpvNative.java`).
- **What it plays:** MKV, WebM, AVI, TS, MP4 and the other containers directly; H.264, HEVC, VP9, AV1, MPEG-2, MPEG-4 and VP8 through the chip's decoder (MediaCodec) where the device has one, everything else (also VC-1, 10-bit H.264) through mpv's software decoders; DTS, DTS-HD, TrueHD, FLAC, Opus and the rest decoded on the device to PCM in up to 7.1 channels; embedded and external subtitles as text, ASS (with attached fonts), PGS, VobSub and DVB, drawn on the device; audio and subtitle track switching inside the engine; speed, exact seeking, resume, chapters, progress through the same fact path. It also plays the server's HLS streams.
- **HDR:** an HDR10 or HLG title on a screen that shows it is shown in HDR: the decoder then draws straight onto the surface, as it does for Media3. While subtitles that mpv itself must draw (ASS, PGS, VobSub, DVB) are on, the title is tone-mapped to standard range on the device instead, because nothing of mpv's own is drawn in the HDR mode. The plain-text subtitle line stays in both.
- **System behaviour:** one system media session for whichever engine plays (headset buttons, remote transport keys, Bluetooth); audio focus (calls, other apps, ducking); headphones unplugged; the app leaving the screen pauses and tells JavaScript; the display is asked to match the title's frame rate (Android 11 and later) and a television is waited for; the screen stays on while playing.
- **What mpv does not do is declared:** no Picture in Picture, no live channels, no music sessions, no bitstream passthrough, no Dolby Vision profile 5, no interlaced video. The module's `features` and the capability profile say so, and the player hides or avoids them.
- **The Player setting** (Automatic, this device's player, mpv) appears on Android wherever the libraries load: Settings, Quality, and the player's menu.
- **The mpv capability profile for Android** is published as `engines.mpv` of the version 2 capability document: hardware codecs at each MediaCodec decoder's own limits, software codecs within estimates by device class, `subtitleRendering` text, ass, pgs, vobsub, dvb.
- **The Android platform player's profile now says only what is true** (text subtitles only; software-only codecs capped; a decoder kept beside a bitstream), and four Android player bugs from document 20 are fixed (section 3).

## 2. Decisions I made

1. **Which libmpv.** media-kit's `libmpv-android-video-build` release `v1.1.11`, flavour `full`, one `libmpv.so` per architecture, pinned by SHA-256, fetched at build time from GitHub. It is the LGPL flavour (FFmpeg 6.0 `--disable-gpl --enable-version3`, mpv `-Dgpl=false`, libass 0.17.1, dav1d, mbedTLS; FFmpeg linked into libmpv). I rejected `dev.jdtech.mpv:libmpv` on Maven Central, the obvious one-line dependency: its build scripts use `--enable-gpl --enable-version3` and mpv's GPL default, so it is GPL. I read both projects' build scripts to confirm.
   - **The owner should know:** the FFmpeg in it is LGPL **version 3** (because it links mbedTLS). Our engine never uses FFmpeg's TLS (see 4), so a build of our own without mbedTLS could be LGPL 2.1 if version 3 is unwelcome.
   - **It is an older mpv** (a revision of October 2023, between 0.36 and 0.37) without libplacebo, without FFmpeg's S/PDIF muxer and without deinterlacing filters. That is why three things are declared "no" (passthrough, Dolby Vision profile 5, interlaced). A build of our own from the same scripts with those three added would lift all three; I did not build one here (hours of compile, and outside the allowed checks).
2. **Our own JNI bridge, not a published Java wrapper.** The chosen binaries carry no Java API, and the usual wrappers drop what the contract needs (a command's reply, why a file ended, which playlist entry an event is for). The bridge is about 400 lines of C with one event thread per player; text crosses as UTF-8 bytes so no file name or track title can abort the JNI layer. mpv's `client.h` (ISC licence) is checked in beside it.
3. **Picture: mpv's OpenGL ES output on a `SurfaceView`, embedded through mpv's window option**, not the render API. It is how mpv is embedded on Android everywhere, and needs no EGL code of ours. The surface is laid out at the picture's own shape, so it is never stretched in either drawing mode.
4. **All of mpv's network reads go through a relay inside the app** (as on Apple). FFmpeg in this build has no access to the system's trusted certificates and does not verify TLS. mpv is only ever given `http://127.0.0.1:<port>/...`; the relay fetches the real resource with OkHttp (system TLS validation, the app's network security rules), serves only paths under the playback's own media grant, GET and HEAD only, refuses redirects, and tells the engine why a fetch failed so failures are classified as precisely as Media3's. HLS playlists are rewritten so every address in them comes back through the relay; mpv's own network code is limited to plain HTTP (to the relay) and files, and a file may not send mpv to other sources.
5. **HDR by direct output** (`vo=mediacodec_embed`), on by default, chosen per title after the first frame: HDR title, screen shows that kind, hardware decoder for a 10-bit codec, no mpv-drawn subtitles. If the picture is lost in that mode the engine goes back to mpv's own drawing for that file. **This is the least certain part of the lane** and the first thing to test on a television. One constant turns it off (`PorticoMpvController.HDR_DIRECT_OUTPUT`); then every HDR title is tone-mapped, as on Apple TV.
6. **Hardware decoding order.** Televisions use the decoder's frames in place (zero-copy), then copying; phones and tablets copy first (exact colours, and their processors keep up), then zero-copy. A codec with only a software decoder in the system is never sent to MediaCodec: mpv's own software decoders are better.
7. **4K on a television whose interface is 1080p.** In mpv's own drawing mode the picture is drawn at the panel's size on boxes with more than 2.5 GB of memory, and at interface size (usually 1080p) on smaller ones, whose graphics chips do not keep up. In the direct HDR mode it is always full size.
8. **Plain-text subtitles the server also publishes as cues are drawn by the app**, not by mpv, even though mpv declares `text`. Android's text drawing has a font for every script; mpv here has one default font. The look is the same by contract. mpv draws text itself only when there is no cue document (a download).
9. **mpv's default subtitle font** is a link to a system font (`Roboto`), or the system's Chinese, Japanese and Korean font when the chosen track is in one of those languages. Styled files' attached fonts work as usual. Other scripts (Arabic, Hebrew, Thai, Indic) drawn by mpv without their own fonts will show empty boxes; see section 9.
10. **Audio focus is the engine's own**, since mpv opens its own audio output. It is asked for when playback is about to start, a passing loss holds playback, a lasting loss is not asked for again until the viewer presses play, and the app tells JavaScript through the same system-media commands Media3's listener produces.
11. **One system media session for both engines.** `PorticoSystemMedia` now wraps whichever engine has the video; for mpv that is a small Media3 `SimpleBasePlayer` that mirrors mpv's position, speed and state. Presses still go to JavaScript only.
12. **Frame-rate matching uses `Surface.setFrameRate` (Android 11 and later)**, so the system's "match content frame rate" setting decides. Below Android 11 (Fire OS 6 and 7) there is none, the same as the Media3 engine. The engine waits for the television only when the switch may break the picture (setting "always", Android 11, or an HDR switch); never on phones.
13. **Leaving the app pauses a video in either engine** and sends `systemPaused` (B-20.4). An activity in a Picture in Picture window counts as on screen. Music and audiobooks play on.
14. **Software limits are the Apple lane's estimates by device class** (memory and processor count), reused unchanged for Android and also applied to the Android platform player where the system has only a software decoder (B-20.14). They are estimates, marked `declared`.
15. **Dolby Vision on Android in mpv:** profiles 7 and 8 are offered (their HDR10 layer plays); profile 5 is not (its colours need libplacebo).
16. **APK size.** libmpv adds about 8 MB of download per architecture. The single APK is unchanged by default; `-PporticoAbiSplits=true` also writes one APK per architecture. I did not change the default artefact names, which the README and CI use.
17. **mpv's configuration folder** (`noBackupFilesDir/mpv`) exists only to hold the default font link, which is where mpv looks for it. mpv reads no settings from it.
18. **mpv starts on its own thread, and an option it refuses stops the start with the option's name**, except a listed set (looks, performance, options whose name changed between mpv versions). This follows the corrected Apple engine. It means a wrong option name shows up at the first test as a named error, not as odd behaviour later.
19. **mpv is kept for 30 seconds after a lease ends** (the Apple engine now destroys it at once, because its audio output deactivates the shared audio session; Android's does not). Releasing the lease still finishes only after mpv has closed the title and its sound.
20. I did not add Kotlin unit tests: the project has no Android test set-up or test dependency, and adding JUnit is a new dependency. The pure logic was exercised on the JVM from a throwaway harness instead (section 8).

## 3. Bugs

Document 20:

| Bug | Result |
| --- | --- |
| B-20.1 | Not mine (planner). The document sends passthrough as its own list. |
| B-20.2 | Apple; fixed by the Apple lane. |
| B-20.3 | Already fixed. The mpv engine uses the same rule. |
| B-20.4 | **Fixed** for both Android engines: a video is paused when the app leaves the screen without Picture in Picture, and JavaScript is told (`systemPaused`). Needs a device to confirm. |
| B-20.5 | Apple. On Android the mpv engine waits for the display before `ready`; Media3 still votes at first frame (not changed). |
| B-20.6 | Already fixed for Media3. mpv classifies failures the same way. |
| B-20.7 | Client side fixed by the Apple lane; Android now sends the fact too. |
| B-20.8 | Already fixed. mpv uses the source codec names. |
| B-20.9 | Apple. |
| B-20.10 | **Fixed**: an output that lists no channel counts is eight channels on a digital output and two elsewhere, not always two. |
| B-20.11 | **Fixed** where this repository can: the published native profile declares text only (now stated and tested for Android), and the Android module's `features.subtitleRendering` is `['text']`. The false table in `packages/client-core` (`androidDeclaredProfile`) is a copy here; see section 7. |
| B-20.12 | **Fixed on Android** for both engines: the app-drawn line and mpv's own subtitles scale with the system caption size. Colour and background are not read. |
| B-20.13 | Already fixed. |
| B-20.14 | **Fixed** in the published document: a codec with only a software decoder is declared at the software estimate for the device's class, or left out. The shared package still ignores the flag. |
| B-20.15 | Fixed by the Apple lane in the document; mpv on Android supplies chapters. |
| B-20.16, .17, .18 | Already fixed. |
| B-20.19 | Not fixed: speculative, needs a cue-less MKV on a device, and it is inside Media3's extractor. Such a file can now be played in mpv, which seeks it. |
| B-20.20 | **Fixed**: passthrough is reported only when sound is going to the digital output, and the published document keeps the device's own decoder beside a bitstream. |
| B-20.21 | Not fixed: it is in the music crossfade code, which belongs to the audio-engines lane. |
| B-20.22, .23 | Apple. The mpv engine gives audio focus back half a second after a lease ends. |

Document 19 (B-19.1 to B-19.19): none is in the video player engine; they belong to the native-app lane. Not touched.

### The read-through of the Apple engine (`apple-mpv-review.md`), checked against the Kotlin

| Finding there | Here |
| --- | --- |
| The drawing surface could be freed before mpv ended | Did not apply: the bridge holds its own reference to the surface, and the surface is taken back (waiting) before mpv is ended. |
| mpv created on the main thread; refused options ignored | **Applied.** mpv now starts on the core's thread; a refused option is named (decision 18). |
| Swift pointer conversion; `CFString` keys; wake-up coalescing | Did not apply (Swift only; the bridge has its own event thread). |
| Release finished before audio teardown | **Applied in Android's form:** `release` resolves after mpv has closed the title (decision 19). No shared audio session is deactivated on Android, so mpv itself is kept. |
| No size bound on property documents | **Applied:** 2 MiB. |
| A restart arriving before the seek's reply left the seek pending | **Applied** (the same mistake was in the port). |
| An end reached before the first restart was never announced | **Applied.** |
| Audio layout and mixing with other apps | Did not apply: Android's output is asked for explicit layouts by output, and audio focus is the engine's own. |
| Absolute HLS addresses bypassed the relay's TLS validation | **Applied:** playlists are rewritten (bounded to 2 MiB, refused whole if anything is out of scope), references are blocked for non-HLS files, FFmpeg's protocols are limited. |
| A source outside a media grant granted its whole folder; the built address was not rechecked | **Applied.** |
| Relay connections could pile up; head size | **Applied:** 16 at once, 10 seconds to send a request. The head limit was already enforced before routing. |
| Subtitle and font files over HTTP refused except on private hosts | **Applied:** any host, same origin and grant, plain path. (Android's network security settings still decide whether plain HTTP to a public host is allowed at all; that is the networking lane's.) |
| Fonts buffered in memory, 16 × 24 MiB, stale callbacks | **Applied:** one 24 MiB budget, written to disk as they arrive, cancelled fetches store nothing. |
| A late font fetch changed a newer choice's fonts | **Applied.** The fetch was already fenced; the font folder is now also cleared when a subtitle file is removed. |
| Subtitle choice key lacked the embedded title and fonts | **Applied.** |
| A subtitle file added late selected itself | **Applied:** late tracks are removed and the current choice is re-selected. |
| A local VobSub index's `.sub` could lie outside Downloads | **Applied.** |
| Ready announced before the television was matched when the view came late | **Applied** for televisions: readiness waits (10 s at most) for the surface, and playback is paused the moment a wait starts. |
| Engine handoff, Up Next and pause state, capability publishing (JavaScript) | Not Kotlin. They are on the base branch and need no Android change. |
| Software-decoding claims (Apple TV HD, failed probe, 10-bit H.264) | **Applied** to the Android part of the profile too: I merged the base branch's `mpv-profile.ts` into this lane's by hand and gave an 8-bit-only Android HEVC, VP9 or AV1 chip the same rule (both depths share the limit both fit). |

One thing I found while applying these, which the Apple engine probably shares: with `access-references=no`, mpv refuses to open a VobSub index's `.sub` companion when the index is added as an external file during a direct play. The Android engine allows references for exactly as long as that one `sub-add` takes. The corrected Swift does not; see section 7.

## 4. API changes

No server routes. No change to what the client sends, except that on Android the capability document's `engines.mpv` now exists, and `engines.native` is corrected as in B-20.11, B-20.14 and B-20.20.

Native modules (Android): new `PorticoMpvPlaybackModule` (`acquire`, `command`, `release`, `facts`; events `PorticoMpvPlaybackFact`, `PorticoMpvPictureInPicture`; constants `engine`, `available`, `features`) and view `PorticoMpvPlayer`. `PorticoPlaybackModule` gains the contract's constants (`engine: 'native'`, `available`, `features`), and its `acquire` answers `engine: 'native'`.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

| Setting | Type | Default | Where it takes effect | Screen |
| --- | --- | --- | --- | --- |
| Player (`preferences.playerEngine`, this device) | Automatic, This device's player, mpv | Automatic | Now also on Android, Android TV and Fire TV | Yes (the Apple lane's row and player menu), shown where mpv loads |
| System caption size (Android Accessibility) | system | 1 | Scales subtitles in both Android engines | The system's |
| System "match content frame rate" (Android TV 12+) | system | system | Decides whether the display switches for mpv | The system's |

## 7. Needed from others

**Shared packages** (copies here, not edited). Everything in the Apple report's section 7 applies to Android unchanged. In addition:

1. `client-core` `androidDeclaredProfile` / `androidProfile`: remove `styled: ['ass','ssa'], bitmap: ['pgs']` and its comment (B-20.11); read the decoder's `hardware` flag (B-20.14); stop a passthrough entry replacing a decode entry (B-20.20). Then delete the Android corrections in `apps/native/src/bridge/capability-document.ts` (`nativeEngineCapabilities`, the `android` argument).
2. `client-core` capability types: `MpvDeviceFacts` gained four optional fields (below); the shared type should carry them.

**Contract (`Native/ENGINE-CONTRACT.md`).** I did not edit it. Corrections to make:

- Section 8, `facts()`: add optional `hardwareLimits: {<codec>: {maxWidth, maxHeight, maxFrameRate, tenBit}}`, `hdrOutputRanges: string[]`, `dolbyVisionProfiles: number[]`, `softwareDeinterlace: boolean` (Android sends them; absent on Apple).
- Section 6: on Android the HLS rendition name is not visible to mpv (this libmpv does not expose track metadata), so mapping is by `streamIndex`, the track title, then order.
- Sections 7 and 9: external subtitle and font files are accepted from any host over HTTP or HTTPS under the same-origin and grant rules (the Android engine now does this, as the corrected Apple one does); the media source stays restricted; redirects stay refused.
- Section 7: "On Android, plain text that also has a cue document is drawn from the cue document."
- Section 9, display matching: "Android: Android 11 and later, through the system's frame-rate request; the engine waits on a television only." Leaving the app: the Media3 engine now pauses and sends `systemPaused` too.
- Section 10 is Apple's list. Android: HDR10 and HLG are shown in HDR unless mpv draws the subtitles; no Dolby Vision profile 5; no interlaced video; no passthrough.

**Apple engine (apple-mpv lane):** check an external VobSub track (`render.url` to an `.idx`) during a direct play. With `access-references=no` set for the whole player and `yes` only per HLS load, mpv should refuse to open the `.sub` beside the index, and the track falls back to cues or fails. The Android engine opens references for the duration of that one `sub-add` (`PorticoMpvController.kt`, `referencesToken`).

**Merging this branch:** `apps/native/src/bridge/mpv-profile.ts` was changed here and on the base branch. This lane's file already contains the base branch's changes; where git reports a conflict in `mpvEngineCapabilities`, take this lane's side. `test/mpv-profile.test.ts` here is the base branch's file, unchanged.

**Server, playback-core:**

3. The document has one `display`. From mpv on Android the screen shows HDR only while no mpv-drawn subtitle is on; `mpvDisplayRanges()` gives the list. A planner that weighs HDR needs a per-engine display list (the Apple lane asked for the same).
4. One video entry per codec cannot say "4K at 30 but 1080p at 60". Android decoders report the rate at their largest picture, so a phone whose H.264 decoder does 4K30 is declared at 30 frames a second and a 1080p60 file is refused for it. Both Android profiles have this; it needs a second limit in the schema.
5. Interlaced titles are not offered to mpv on Android; the server must deinterlace or use the native engine.

**Other lanes:**

6. audio-engines: B-20.21. Also, `PorticoSystemMedia.kt` changed (it now follows either engine); the listening owner's session is untouched.
7. downloads: an offline MKV now plays in mpv on Android. A downloaded subtitle file can be given as `render.url` with a `file:` address inside the Downloads folder. `PorticoOfflinePlayerActivity` (theirs) is not connected to mpv.
8. native-app / native-playback: `PorticoSubtitleSelection.privateHost` is now `internal` (one word) so the mpv engine shares the rule. Colour and background of the caption style are still not read on Android.

**Before release:** the app's licences screen needs the notices for libmpv (LGPL 2.1 or later), FFmpeg (LGPL 3), libass (ISC), dav1d (BSD), FreeType, HarfBuzz, FriBidi (LGPL 2.1), mbedTLS (Apache 2.0) and libxml2 (MIT), with where to get the sources.

**Documents to correct:** as the Apple report says (`06 Clients.md`, `04 Playback and Media.md`, the Picture in Picture contract). The Android port log's "a video in the background is either in Picture in Picture or paused" is now true.

## 8. Test notes for other environments

### What was checked here

- Kotlin: every Android source type-checks with the Kotlin 2.1.20 compiler from the Gradle cache against android-36 and the cached libraries, both flavours (a copy of `_program/native-ktcheck.sh` that also takes `portico-mpv`'s Java file). Type-checked, never linked or run.
- C: `portico_mpv_jni.c` compiles with the NDK 27.1 clang for all four architectures with `-Wall -Wextra -Werror`. Not linked (no libmpv here).
- Pure logic (rendition mapping, embedded subtitle matching, number rules, language codes) run on the JVM from a throwaway harness: 28 checks, all pass. The playlist rewriting and the relay could not be run this way (they use Android's address class).
- JavaScript: `npx tsc --noEmit` and `npx tsc -p test/tsconfig.json --noEmit` clean; `npm run lint:strings` clean; new `test/mpv-profile-android.test.ts` (8 tests) passes; the base branch's corrected `mpv-profile.test.ts` passes against the merged profile; `engine-choice`, `engine-switch`, `engine-media`, `capability-profile` still pass. The full suite was not run.
- mpv option, property and command names were checked against the manual of the exact mpv revision in the binary. FFmpeg 6.0's MediaCodec decoder list and the absence of the S/PDIF muxer were checked in its source and the build script.
- **Not checked:** the Gradle files (never evaluated), the download task, CMake, packaging, and all behaviour on a device.

### Building (JDK 17, Android SDK 36, NDK 27.1, network)

1. `cd apps/native && npm ci --legacy-peer-deps`.
2. `cd android && ./gradlew :portico-mpv:fetchLibmpv`. Pass: four files under `portico-mpv/build/libmpv/imported/<abi>/libmpv.so`. Fail: a checksum message (the release was re-published: stop and look) or a network error (use `-PporticoLibmpvDir`).
3. `./gradlew :app:assembleGoogleDebug` and `:app:assembleFiretvDebug`. **Expect problems on the first build**; the likeliest:
   - `portico-mpv/build.gradle` (Groovy, written blind): task wiring before CMake, `abiFilters`.
   - CMake: the imported `libmpv.so` must be packaged by the build. Check with `unzip -l app/build/outputs/apk/google/debug/app-google-debug.apk | grep -E "libmpv|libporticompv"`: both, for every architecture. If `libmpv.so` is missing, add `jniLibs.srcDir` for `build/libmpv/imported`; if it is there twice, remove the import from packaging.
   - If the app fails at start with `dlopen ... libc++_shared.so` for libmpv: add `packagingOptions { jniLibs { pickFirsts += ['**/libc++_shared.so'] } }` (React Native already ships that library).
4. `adb logcat -s PorticoMpv` while testing shows mpv's warnings and errors; the engine's `errorDetail` fact field carries the last one.

### Device tests. Pass and fail are stated for each.

Devices: an Android phone (HDR screen if possible), an Android TV on Android 12 or later with an HDR television and a receiver, a Fire TV Stick 4K (32-bit) and an older Fire TV (Fire OS 6 or 7). Server from the integrated programme branch.

1. **Setting appears.** Settings, Quality shows "Player". Fail: no row (libraries did not load: check logcat for `UnsatisfiedLinkError`).
2. **MKV direct, Automatic.** H.264/AAC MKV with DTS audio or PGS subtitles. Pass: starts in a few seconds, the server shows no conversion. Fail: a conversion session, an error, or a black picture with sound (surface: see 14).
3. **Forced mpv.** Player = mpv; play an MP4. Pass: plays; no Picture in Picture button on a phone.
4. **Seek, resume, speed.** Scrub (lands within half a second), leave and resume (starts at the saved position), 1.5x (sound stays in pitch).
5. **Audio tracks.** A file with two audio tracks: choose the second. Pass: changes within a second, no new session.
6. **Subtitles.** (a) SRT while streaming: white on a soft black box inside the picture (the app's line). (b) ASS with styling and attached fonts: positioned, coloured, right typeface. (c) PGS and VobSub: images, correctly timed. (d) Timing +0.5 s moves them later. (e) A downloaded MKV offline with an embedded SRT: text drawn by mpv, readable. (f) A Japanese ASS track without attached fonts: Japanese glyphs, not boxes. Fail for any: nothing drawn, boxes, or a burn-in conversion.
7. **Engine switch mid-title.** Start an MP4 natively, choose mpv in the player menu. Pass: same position within a second, Up Next unchanged. Switch back. Lock-screen or headset controls keep working across the switch (test 12).
8. **Multichannel.** Android TV into a 5.1 or 7.1 receiver: DTS-HD or TrueHD plays as multichannel PCM. Fail: stereo, or wrong channel order (check rear and centre).
9. **HDR (the uncertain one).** HDR television, HDR10 HEVC MKV with DTS audio, no subtitles, in mpv. Pass: the television shows its HDR badge, the picture is correct, pause and seek show the right frame. Then turn a PGS track on. Pass: the picture stays (now standard range, tone-mapped), subtitles appear; turning them off returns to HDR. Fail: black picture at either switch, a frozen frame, stretched picture. If it fails, set `HDR_DIRECT_OUTPUT` to false and note the device. Also an HLG title, and an HDR title on a standard-range television (must tone-map with sensible colours; on a television this goes through zero-copy decoding, where wrong colours are possible).
10. **Dolby Vision.** A profile 8 file plays with correct colour (as HDR10). A profile 5 file should not be sent to mpv on Automatic; forced to mpv it will be green and purple (known, declared).
11. **Frame-rate matching.** Android TV 12 or later, 23.976 fps title: with the system setting "Always" the television switches and playback starts after the switch, with the opening seconds intact; "Seamless only" starts at once; "Never" does nothing. Fire OS 6 and 7: no switch (expected). Fire OS 8: check what happens and whether the one-second wait is right.
12. **Media session and remote.** Bluetooth headset play/pause, a television remote's play/pause and seek keys, and "pause" by voice all work in mpv; the position shown by the system moves. D-pad controls work as in the native engine.
13. **Audio focus.** Phone: a call arrives (pauses, resumes after only if it was playing); start music in another app (pauses and stays paused; Play takes the sound back); a navigation prompt ducks without pausing. Unplug headphones: pauses.
14. **Leaving the app (B-20.4), both engines.** Television: press Home during a film in mpv, and again in the native engine. Pass: sound stops at once; on return the picture is there, the button says Play, one press continues. Phone: same with the screen switched off; with the native engine and Picture in Picture on, the film continues in the small window instead. Fail: sound behind the launcher, black picture on return, a crash in `surfaceDestroyed`, or music stopping when the screen goes off.
14a. **HLS and the relay.** A converted (HLS) title in mpv, with both relative and absolute addresses in its playlists, plays, and the server's log shows every request coming from the app's HTTP client. A playlist naming another host is refused (a plain error), never fetched. A direct MKV with an external VobSub track (`.idx`) shows its subtitles. From another app or `adb shell`, `curl http://127.0.0.1:<port>/anything` answers 404, and an idle connection is closed after 10 seconds.
14b. **Start-up option check.** If every mpv title fails with "mpv could not start: option <name>", that option is refused by this libmpv: either it is misspelt or it belongs in `OPTIONAL_OPTIONS` (`PorticoMpvController.kt`).
15. **Network loss.** Wi-Fi off for 15 seconds mid-title. Pass: "Reconnecting", then it continues from the same place. Fail: the title ends or is marked watched.
16. **TLS and plain HTTP.** A server with an untrusted certificate: mpv refuses it, like Media3. A plain-HTTP server on the home network plays. A plain-HTTP public address is refused.
17. **Errors.** A corrupt file: a plain error or the next route, not a hang. A file whose video mpv cannot decode: the server is asked for another route.
18. **Downloads.** Download an MKV, go offline, play: it plays in mpv.
19. **Weak devices.** 4K HEVC for 20 minutes on a Fire TV Stick 4K: no stutter, no termination. 1080p software VC-1 or 10-bit H.264 on the same: watch for dropped frames (the software limits are estimates). A 4K SDR title on a 4K box with over 2.5 GB: drawn at 4K without stutter; if it stutters, raise the memory threshold in `renderSize`.
20. **Capability document.** Read the stored document for an Android TV with a receiver: `engines.native.subtitleRendering` is `["text"]`, AC-3 appears under both `audio` and `audioPassthrough`, a software-only AV1 decoder is capped or absent, `engines.mpv.video` limits match the device's decoders.

## 9. Not finished

- **Not built, not run.** All native code in this lane is untested. The Gradle wiring is the likeliest first failure.
- **Bitstream passthrough in mpv**: not possible with this libmpv (no S/PDIF muxer). Sound is decoded to PCM. Declared.
- **Interlaced video in mpv**: no deinterlacing filter in this libmpv. Not offered; declared.
- **Dolby Vision profile 5 in mpv**: no libplacebo in this libmpv. Not offered; declared. Dolby Vision is never output as Dolby Vision from mpv, only as its HDR10 layer.
- **HDR with mpv-drawn subtitles**: tone-mapped to standard range (decision 5). HDR10+ dynamic metadata is not used.
- **HDR direct output is unproven** on any device (decision 5).
- **Fonts for scripts other than Latin, Greek, Cyrillic, Chinese, Japanese and Korean** when mpv draws text or an ASS file has no attached fonts: empty boxes. Streaming plain text is not affected (decision 8).
- **Frame-rate matching below Android 11** (Fire OS 6 and 7): none, in either engine.
- **Fire OS passthrough reporting** for the native engine: Fire OS reports receiver formats through an HDMI broadcast and a system setting rather than only `AudioDeviceInfo`; `PorticoCapabilities` still reads only the latter. Not changed.
- **Picture in Picture, live channels and music in mpv**: not built (declared and hidden), as on Apple.
- **No Kotlin unit tests** in the repository (decision 20).
- **Software decoding limits are estimates.** They need measuring on real devices.
- **The plan's engine does not reach the player** until the shared package carries `engine` on the session (Apple report, section 7, item 1). Until then Automatic on Android means: native, except a download Media3 cannot open or a subtitle that needs mpv. The setting "mpv" works today.
- **B-20.19 and B-20.21**: not fixed (section 3).
- **An HLS stream of unknown length that ends early** can still look like a real end (the early-end check needs a known length), as on Apple.
- **A playlist is recognised by `.m3u8` or its content type.** A stream whose address does not end in `.m3u8` is opened as a file and may not follow references, so it would not play in mpv. The client code's HLS addresses are `/v1/media/<grant>/<generation>/master.m3u8`; the server itself was not checked.
- **A newer libmpv of our own** (libplacebo, S/PDIF muxer, deinterlacing, no mbedTLS) would remove the first three gaps and the LGPL 3 question. Not built.
