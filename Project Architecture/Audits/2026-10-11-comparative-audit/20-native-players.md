# 20 — Native playback engines and player behaviour (Apple and Android)

Read-only comparative audit, 11 October 2026. Everything here is a **source finding**: nothing was built, run or reproduced on a device. Where a conclusion depends on how AVPlayer, ExoPlayer or a television behaves at run time, it says so.

Paths: Portico paths are relative to `/home/justin/projects/Portico`. The React Native app is `apps/portico-react-native/apps/native` (shortened to `native/` below); client-core is `apps/portico-react-native/packages/client-core/src` (shortened to `client-core/`; the copy in portico-server is identical, checked with `diff -rq`); the server is `apps/portico-server/server` (shortened to `server/`). Comparison paths are `plezy/…`, `swiftfin/…`, `jellyfin-androidtv/…` and so on.

Jargon used throughout:

- **Direct play**: the device is handed the original file and plays it untouched.
- **Remux / copy / direct stream**: the server repackages the same video and audio into another container (for Portico, HLS) without re-encoding. Cheap for the server.
- **Transcode / convert**: the server decodes and re-encodes. Expensive, and for video it loses quality.
- **Burn-in**: subtitles are drawn into the picture by the server, which forces a video transcode.
- **Passthrough / bitstream**: the device hands the undecoded Dolby or DTS audio to a receiver or soundbar over HDMI, which decodes it.
- **Platform player**: AVPlayer (Apple) or ExoPlayer/Media3 (Android), the players the operating system vendor supplies.
- **Universal engine**: a player the app brings with it (mpv, VLC, Infuse's own), built on FFmpeg, which decodes nearly every format on the device.
- **DV**: Dolby Vision. Profile 5 is the streaming-service flavour with no fallback layer; profile 7 is the UHD Blu-ray flavour with a second "enhancement" layer; profile 8.1 is single-layer with an HDR10 fallback.

## Scope and sources

**Portico, read in full** (not just listed):

- Docs: `Project Architecture/04 Playback and Media.md`, `06 Clients.md`, `Backlog.md`, `Specs/Spec — Playback Protocol v1.md`, `Product Contracts/playback/contract.md`, the player sections of `Audits/2026-10-03-android-port-log.md` and (skimmed) `…-vega-port-log.md`.
- iOS/tvOS native: `native/Native/PorticoPlaybackController.{h,m}`, `PorticoPlayer.m`, `PorticoPlaybackModule.m`, `PorticoCapabilities.m`, `PorticoSubtitleSelection.{h,m}`, `PorticoSubtitleFiles.m`, `PorticoAudioSelection.{h,m}`, `PorticoSystemMedia.m`, `PorticoSiriRemote.m`, `PorticoAudioRoutePicker.m`, `PorticoCast.m`, `PorticoActivity.m`; `Info.plist` for both targets (background modes, transport security).
- Android native (`native/android/app/src/main/java/tv/getportico/portico/native/`): `PorticoPlaybackController.kt`, `PorticoPlayerView.kt` (with Picture in Picture), `PorticoPlaybackModule.kt`, `PorticoCapabilities.kt`, `PorticoSubtitleSelection.kt`, `PorticoAudioSelection.kt`, `PorticoSystemMedia.kt`; `MainActivity.kt`; the manifest and `build.gradle` dependencies.
- JavaScript: `native/src/player/` (`usePlayer.ts`, `VideoPlayer.tsx`, `Timeline.tsx`, `SkipPrompt.tsx`, `PostPlay.tsx`, `dolby-vision.ts`, the subtitle/audio/quality/sleep parts of `PlayerOptions.tsx`), `native/src/bridge/` (`NativePlaybackCoordinator.ts`, `useNativePlayback.ts`, `device-profile.ts`, `client-profile.ts`, `capability-publisher.ts`, `audio-decode.ts`, `device-platform.ts`, `NativePlaybackSpeed.ts`, `NativeSystemMedia.ts`, `NativeSiriRemote.ts`, `useNativeAudioSelection.ts`).
- client-core: `client-profile.ts` (all of it), `playback-v1/capability-profile.ts`, `player-offers.ts`, `presentation/playback-info.ts`, the engine-failure, seek and fact paths of `index.ts` (`PlaybackService`), the player rows of `presentation/settings-structure.ts`.
- Server, only as far as needed to know what the declared capabilities do: `internal/playback/delivery_decision.go` (all), `client_profile.go` (profile lookup and baseline), `route_failures.go`, `subtitle_delivery.go`, `internal/playbackv1/capabilities.go` (the conversion of a device's capabilities into the planner's profile), `playbackv1/devices.go`, `playbackv1/subtitles.go`, `testdata/client-profiles.json`, `declared_profiles_test.go` (matrix rows).
- Vega: `apps/vega/src/bridge/capability-profile.ts`, the head of `playback.ts`. Lightly.

**Comparison projects, read for this domain:**

- **Plezy**: README feature list and footnotes, `lib/services/settings_service.dart` (every preference), `display_mode_service.dart`, `playback_open_outcome.dart`, `video_decode_capabilities.dart`, `jellyfin_client/parts/playback.dart` (its device profile), the header comment and policy of every file in `android/app/src/main/kotlin/com/edde746/plezy/exoplayer/` (33 files, 14,700 lines), `ios/Runner/MpvPlayer/MpvPlayerCore.swift` (display criteria and the mode-switch waiter), `mpv-build.lock.json`.
- **Swiftfin**: `Shared/Objects/VideoPlayerType/*` (the AVPlayer and VLC/mpv device profiles), `PlaybackCapabilities.swift`, `PlaybackCompatibility/*`, `MediaPlayerProxy+AVPlayer.swift` (status, display criteria, Picture in Picture), the `VideoPlayer` section of `SwiftfinDefaults.swift`.
- **jellyfin-androidtv**: `util/profile/deviceProfile.kt` (all), `KnownDefects.kt`, `codec/*` (codec query and passthrough check), `preference/UserPreferences.kt`, `playback/media3/exoplayer` (backend options, build file), the refresh-rate and retry paths of `ui/playback/PlaybackController.java`.
- **Wholphin**: README, `services/RefreshRateService.kt`, `ui/playback/SeekAcceleration.kt`, `services/PlaybackLifecycleObserver.kt`, file inventory.
- **Streamyfin**: `modules/mpv-player` and `modules/exoplayer-player` inventory, `ios/MPVLayerRenderer.swift` options, `PiPController.swift`, `package.json`, podspec and Gradle dependencies.
- **jellyfin-android**: player file inventory, gesture and decoder settings (grep level).
- **Infuse**: public forum answers from Firecore staff only (links in the text). Not code.

**Not reached:** jellyfin-desktop (mpv desktop shell; no Portico counterpart), Plex and Emby clients (closed), the full 4,400-line Plezy `ExoPlayerCore.kt` (only its policy files and headers), Plezy's Flutter player screens, the Vega HLS loader (`hls.ts`), Portico's listening (music) owner beyond where it shares the video player, Cast and AirPlay hand-off beyond what the player module does (domain 13), and the server's HLS production (domains 02 and 17).

## How Portico does it today

### One player per platform, driven from JavaScript

Each platform has exactly one video engine:

| Platform | Engine | Surface | Subtitles |
|---|---|---|---|
| iPhone, iPad, Apple TV | `AVPlayer` with a bare `AVPlayerLayer` (`native/Native/PorticoPlaybackController.m:55`, `PorticoPlayer.m:17`). Not `AVPlayerViewController`: the system's own player screen is never shown. | A `UIView` hosting the layer | A `UILabel` over the picture, or AVPlayer's own text track when one can be matched |
| Android phone, tablet, Android TV, Fire TV | Media3 ExoPlayer 1.10.1 with the default renderers (`PorticoPlaybackController.kt:375-399`; `android/app/build.gradle:97-101`: `media3-exoplayer`, `-exoplayer-hls`, `-datasource-okhttp`, `-session`). No FFmpeg decoder extension, no libass, no `media3-ui`. | A `SurfaceView` letterboxed by hand (`PorticoPlayerView.kt:63-89`) | A `TextView` over the picture; ExoPlayer's own text track type is switched off (`PorticoPlaybackController.kt:394`) |
| Vega OS | Amazon's W3C media element, fed by a small HLS loader the port wrote itself (`apps/vega/src/bridge/playback.ts`, `hls.ts`) | SDK surface | Drawn by the app |

All of the player's controls (the chrome) are React Native components shared by every platform (`native/src/player/VideoPlayer.tsx`). The native side is deliberately thin: "a surface and a line of text" (Android port log, Phase 2).

### The command and fact protocol between JavaScript and the engine

JavaScript owns the playback session; native owns the engine. They talk through a small, strict protocol (`PorticoPlaybackController.m:136-200`, mirrored line for line in Kotlin):

1. JavaScript **acquires a lease** on the one controller (`acquire`).
2. It sends **commands**. Each command is the whole desired state: a *binding* (lease, intent number, session id, generation, item id), the media address, paused or not, the rate, a pending seek, the chosen audio rendition, the chosen subtitle track, and a rising **command revision**. A command is validated in full before anything changes; stale or conflicting ones are refused with a reason.
3. Native answers with **facts** (`ready`, `firstFrame`, `progress` twice a second, `stalled`, `seekApplied`/`seekFailed`, `ended`, `error`, audio mapping facts). Each fact carries the binding it belongs to, a rising sequence number and the last command revision the engine had seen, so JavaScript can discard anything from a superseded item, route or seek (`native/src/bridge/NativePlaybackCoordinator.ts:265-298`).

This is the most distinctive thing about Portico's player and none of the comparison projects has an equivalent. It exists because the React Native bridge is asynchronous and Portico wants hard guarantees ("nothing from a superseded item can be read as current").

### How a play starts

1. For a video, JavaScript first asks `GET /v1/items/{id}/playback-options` to see whether a Dolby Vision colour warning is needed (`native/src/player/usePlayer.ts:388-410`).
2. It opens a Playback v1 session. The server chooses the delivery (original file, HLS remux, HLS conversion) from the device's stored capabilities.
3. The coordinator acquires the lease and sends the first command with the media URL (a capability URL, no headers needed).
4. Native loads it: `AVPlayerItem playerItemWithURL:` (`PorticoPlaybackController.m:294`) or `MediaItem.fromUri` (`PorticoPlaybackController.kt:408`).
5. Resume is a seek: JavaScript puts the resume position in `pendingSeek`, and native performs an exact seek once the item is ready.

### What the device tells the server it can play

There are **two** capability documents, both built from the same inputs:

- The **client profile** (`PUT /v1/playback/client-profile`, `client-core/client-profile.ts`): a declared table per platform family, narrowed or widened by what a native probe reports. It records codecs with profiles, levels, bit depths, HDR ranges, Dolby Vision profiles, the audio output route and channel count, which containers each transport takes, and subtitle formats. Published by `native/src/bridge/client-profile.ts` from `native/src/app/device.tsx:48`.
- The **v1 capability profile** (`PUT /v1/me/devices/current/capabilities`, spec §3), derived from the client profile by `client-core/playback-v1/capability-profile.ts`, with the form factor, network class, `audioDecode` (for music) and feature flags added. Published by `native/src/bridge/capability-publisher.ts`.

On the server the v1 document is converted back into a planner profile (`server/internal/playbackv1/capabilities.go:213-316`) and **that conversion wins** for any device that has published one (`server/internal/playback/client_profile.go:459-470`). The richer client-profile document is then never read for that device. Fields the v1 document has no place for are lost on the way; two of those losses are bugs (B-20.1, B-20.2).

The native probes:

- **Apple** (`PorticoCapabilities.m:29-64`): hardware decode of HEVC, AV1 and Dolby Vision (`VTIsHardwareDecodeSupported`), the display's HDR modes (`AVPlayer.availableHDRModes`, `eligibleForHDRPlayback`), screen size and refresh rate, and the audio route with its channel count and whether Spatial Audio is on. It republishes when the audio route or HDR eligibility changes.
- **Android** (`PorticoCapabilities.kt`): every decoder in `MediaCodecList` with its largest size, frame rate, bitrate, profiles and highest level, 10-bit, HDR formats and Dolby Vision profiles; audio decoders with channel counts; the display's HDR types and largest mode; what the HDMI sink accepts as a bitstream; the output route. It republishes when an audio device or the display changes.

Apple's declared table (`client-core/client-profile.ts:119-141`) says AVPlayer plays MP4, M4V and MOV directly and nothing else; MKV, AVI and TS are always repackaged into HLS. Android's (`:146-166`) says ExoPlayer plays MP4, MKV, WebM and TS directly.

### Subtitles

On both platforms the app draws subtitles itself from a **cue document**: the server converts every text subtitle track (embedded, sidecar, uploaded or downloaded) into one JSON document of plain-text cues with start and end times in microseconds and two style flags, *italic* and *top* (`PorticoSubtitleSelection.m:118-129`). The player fetches it from under the session's media grant, re-checks it every 25 seconds with an ETag, and blanks the text if 40 seconds pass without a good answer ("a revoked session's subtitles go dark with its video").

On Apple, when the chosen track is an embedded text track that can be matched by name or by language to one of AVPlayer's own "legible" options, AVPlayer draws it instead (`PorticoSubtitleSelection.m:29-55, 76-87`); that version survives Picture in Picture and AirPlay. Everything else is the app's label, which does not.

Image subtitles (PGS, VobSub) and styled ASS are, from the client's point of view, "burn-in" tracks: choosing one makes the server re-encode the video with the subtitles drawn in. The player warns first when the source is 4K or HDR (`native/src/player/PlayerOptions.tsx:237`).

The text is drawn at 3.5 % of the picture width, capped at 40 points, white on a 70 % black box, bottom-centred inside the picture (not in the letterbox bars) (`PorticoPlayer.m:54`, `PorticoPlayerView.kt:80-88`).

### Audio tracks

For HLS the server names each audio rendition; native maps the names onto the engine's tracks (every rendition to exactly one track or the mapping is refused) and selects the pending one (`PorticoAudioSelection.m:55-71`, `PorticoAudioSelection.kt:82-112`). For a direct-played file the app does not switch tracks in the engine: choosing another track asks the server for the title again from the same position with that track named (`native/src/bridge/useNativeAudioSelection.ts:27-33`).

### Seeking

Every seek is exact: zero tolerance on Apple (`PorticoPlaybackController.m:369`), ExoPlayer's default exact seek on Android. Native then checks where the engine landed and reports `seekApplied` only if it is within half a second of the target; otherwise `seekFailed`, and playback pauses with "That position is not available yet". On Apple TV, resting a finger on the Siri Remote's touch surface scrubs the timeline (`PorticoSiriRemote.m`, a GameController micro-gamepad reading absolute finger position). The timeline has no preview thumbnails, chapter ticks or marker ticks (`native/src/player/Timeline.tsx`).

### Display matching, Picture in Picture, system integration

- **tvOS frame-rate and dynamic-range matching:** when the first frame is ready the view reads `asset.preferredDisplayCriteria` and hands it to `AVDisplayManager` (`PorticoPlayer.m:55-60`), and clears it when the lease or window goes. Android has nothing equivalent.
- **Picture in Picture:** `AVPictureInPictureController` on the player layer with automatic start when the app is left (`PorticoPlayer.m:28-34`); on Android phones and tablets the whole activity shrinks and a second video stage is laid over it (`PorticoPlayerView.kt:195-290`). Not on Android TV or Fire TV.
- **Now Playing and remote commands:** `PorticoSystemMedia` publishes the title, artwork, position and rate, and registers play, pause, skip, seek, next and previous. Every command becomes an event for JavaScript, never a direct engine action, so Watch Together rules and progress reporting stay in one place. Skip-forward inside an intro marker jumps to its end.
- **Interruptions:** a phone call or Siri pauses through the same event path and resumes only what it paused; unplugging headphones pauses (`PorticoSystemMedia.m:333-356`; Android `PorticoSystemMedia.kt:112-146`).
- **AirPlay:** `allowsExternalPlayback` is switched on for iPhone and iPad; the player shows "Playing on …" while external playback is active.

### Errors and recovery

Native classifies a failure as **network** (retried on the same route with back-off, "Reconnecting…", give up after 90 seconds) or **engine** (`decode_error`, `source_not_supported`, `engine_error`). An engine failure is reported to the server (`POST /v1/playback/route-failures`); the server remembers that this delivery route failed for this file on this sign-in for seven days and the client immediately asks for the title again, which now comes back through the next route up (remux, then audio conversion, then full conversion). The viewer sees a recovery spinner, not an error, unless the last resort fails too (`client-core/index.ts:1453-1468`, `server/internal/playback/route_failures.go`).

### Options the player offers

Audio, Subtitles (with online search, add-a-file, and timing ±0.5 s), Quality (Automatic plus the server's ladder), Speed (0.5×–2× in seven steps), Sleep timer, and under More: Playback info, Up next, Watch Together, Report a problem. Post-play has Up Next with a countdown, "Still watching?", Replay. Skip Intro/Recap/Credits follow the profile's ask/auto/off preference.

## How the others do it

### Jellyfin's native clients

Jellyfin's server makes the same three-way decision (direct play, direct stream, transcode), but from a **DeviceProfile sent with every playback request** rather than a stored document. Each client builds its own.

**jellyfin-androidtv** (Android TV and Fire TV; Kotlin):

- Engine: ExoPlayer/Media3, **with the FFmpeg audio decoder extension** (`jellyfin-androidtv/playback/media3/exoplayer/build.gradle.kts:38`) so DTS, TrueHD, and anything the device lacks decode on the device to PCM, and **libass** through `ass-media3` (`:40`) for styled ASS. The renderer mode is "extension on", or "prefer extension" behind a setting (`ExoPlayerBackend.kt:101-105`). It used to carry libVLC as a second engine; that is gone.
- Device profile (`util/profile/deviceProfile.kt`): direct play of ASF, MKV, MOV, MP4, OGM, TS, VOB, WebM, WMV with AV1, H.264, HEVC, MPEG-1/2, VC-1, VP8, VP9; every audio codec including DTS and TrueHD; per-profile level limits read from `MediaCodecList`; H.264 reference-frame limits by width; HDR range types computed from decoder profiles; a list of **known-defect devices** (four Fire TV 4K models that mishandle Dolby Vision with HDR10+; the 2019 Shield that plays DV profile 7 without saying so, `KnownDefects.kt`).
- **User overrides for nearly every detection** (`preference/UserPreferences.kt`): bitstream AC3 / E-AC3 / DTS / TrueHD each Auto/On/Off; audio behaviour (direct or downmix to stereo); HDR10, HDR10+, DV profile 5, 7 and 8 each Auto/On/Off; maximum H.264 and HEVC level; software codecs on or off; ASS direct play; PGS direct play; prefer FFmpeg decoders; refresh-rate switching (off / scale on TV / scale on device); zoom mode; night mode; buffer length; resume pre-roll; video start delay; subtitle size, weight, colour, stroke, background and vertical offset; external player.
- Passthrough detection asks ExoPlayer's own `AudioCapabilities` the exact question the audio sink will ask at play time (`codec/audioPassthrough.kt`).
- Failure fallback: up to three retries per play; after the first error direct play is disabled, after the second direct stream too; the counter resets after 30 seconds of healthy playback (`PlaybackController.java:257-269, 523-524`). Nothing is remembered between plays.

**Swiftfin** (iOS and tvOS; Swift): **three engines the viewer chooses between** — AVPlayer ("native"), VLCKit, and mpv behind an experimental switch (`swiftfin/Shared/Objects/VideoPlayerType/VideoPlayerType.swift`). Each has its own device profile:

- AVPlayer: MP4/M4V/MOV only, AAC/AC3/E-AC3/ALAC/FLAC, H.264, HEVC (only with the `hvc1` or `dvh1` sample entry), AV1 where hardware exists; image subtitles are burned in; text goes through HLS WebVTT.
- VLC and mpv: essentially everything, direct, with all subtitle formats embedded.
- Overrides: a compatibility mode (Auto / Most compatible / Direct play / Custom), a custom device-profile editor, "force Dolby Vision transcode" and "force HDR transcode" switches.
- Player: fully configurable gestures (horizontal pan, swipe, long-press speed, multi-tap, pinch, left and right vertical pans), custom playback rates, chapter slider, resume offset, "pause on background", a subtitle configuration, a bitrate test for automatic quality.
- On tvOS its AVPlayer path sets the display criteria as soon as the item is ready, before it starts the clock, and shows buffering while the mode switch is in progress (`MediaPlayerProxy+AVPlayer.swift:466-562`).

**jellyfin-android** (phones; Kotlin): ExoPlayer with the FFmpeg decoder and libass, double-tap seek, brightness and volume swipes, horizontal swipe seeking, touch lock, trickplay thumbnails, chapter markings, a lifecycle observer, a playback service with a notification, external player hand-off.

### Plezy

Plezy plays Plex, Jellyfin and Emby from one Flutter app. Its engine story is the most elaborate of the group:

- **mpv on every platform** (pinned builds, `plezy/mpv-build.lock.json`), rendering through AVFoundation sample buffers on iOS and tvOS (so the system handles HDR and Dolby Vision display and Picture in Picture).
- **On Android, ExoPlayer is the default and mpv is the fallback**, switched mid-open when ExoPlayer cannot take the file ("backend-switched", `lib/services/playback_open_outcome.dart`). Its ExoPlayer backend is 14,700 lines and is a catalogue of real-device scars (each file names the issue number): Dolby Vision profile 7 converted to 8.1 on the fly with libdovi or stripped to HEVC (`DoviConvertingTrackOutput.kt`); HDR10+ metadata stripped from DV streams for chipsets that crash on it (`DvBitstreamSanitizer.kt`); TrueHD and DTS-HD MA packed into IEC 61937 bursts itself because Android will not bitstream them raw on the routes measured (`TrueHdMatPacker.kt`, `DtsHdIecPacker.kt`, `IecCarrierSink.kt`); its own PGS parser because Media3's drops all but one object (`PgsCompositionParser.kt`); repair of unseekable MKV files (`CuelessSeekExtractorWrapper.kt`); watchdogs for a decoder that stalls after resume, a buffer that never drains, and a stream that never ends on tunneled decoders; loudness normalisation; libass with a per-device latency calibrator.
- It asks the server for direct play of MP4, MKV, WebM, MOV and TS with **any audio codec** and every subtitle format embedded (`lib/services/jellyfin_client/parts/playback.dart:968-1021`), and narrows video codecs only by a hardware-decode probe ("can we decode it without dropping frames", `video_decode_capabilities.dart`).
- Settings: about sixty that touch playback (inventory table below), including raw mpv config text.
- tvOS display matching sets the criteria from the decoded stream and **holds playback paused until the HDMI mode switch has ended** plus a settle delay, with watchdogs for a switch that never reports its end (`ios/Runner/MpvPlayer/MpvPlayerCore.swift:440-560`).

### Wholphin (Android TV)

Media3 and mpv as two selectable back ends; "ExoPlayer with optional extra audio, SSA/ASS and AV1 software decoding; MPV for direct playing anything". Automatic refresh-rate **and resolution** switching that waits for the display to report the change and then the two seconds Android recommends for a non-seamless switch (`services/RefreshRateService.kt`); hold-to-seek acceleration that scales with the length of the film (`ui/playback/SeekAcceleration.kt`); a lifecycle observer that pauses on activity pause and resumes on return (`services/PlaybackLifecycleObserver.kt`); trickplay previews; subtitle style with separate HDR settings; skip-back on resume; known-defect list adapted from jellyfin-androidtv.

### Streamyfin (React Native, the closest technical match)

Streamyfin runs on the same `react-native-tvos` fork as Portico (`streamyfin/package.json:91`) and has **moved its player to mpv**: an Expo native module wrapping MPVKit on iOS and tvOS and `dev.jdtech.mpv:libmpv` on Android (`modules/mpv-player/ios/MpvPlayer.podspec`, `android/build.gradle:66`), plus an ExoPlayer module on Android. mpv is configured with `vo=avfoundation`, VideoToolbox hardware decoding with software fallback, and the AudioUnit output (`modules/mpv-player/ios/MPVLayerRenderer.swift:303-363`). Picture in Picture works through an `AVSampleBufferDisplayLayer` content source (`PiPController.swift`). It also sets `AVDisplayCriteria` on tvOS itself. Notably the **player screen is native** (Swift and Kotlin view controllers, about 31,000 lines in the module), not React Native components. It is licensed MPL-2.0 and ships in the App Store and Play Store.

### Infuse (the Apple quality bar; public statements only)

Infuse brings its own engine and plays MKV and friends without a server. From Firecore staff answers on their forum: Apple TV supports Dolby Vision profiles 5 and 8 only, so profile 7 discs play as HDR ([thread](https://community.firecore.com/t/dolby-vision-profile-7-8-support-ts-mkv-files/19713/1658)); Apple TV does not allow audio passthrough, so Infuse decodes lossless audio and sends it as PCM without Atmos metadata, and Atmos survives only inside Dolby Digital Plus ([thread](https://community.firecore.com/t/video-and-audio-formats-on-apple-tv-4k-2021/32482.md)); frame-rate and dynamic-range matching are the system's, switched on in tvOS Settings; HDR10+ is passed with a "Use HDR10+" switch for sets that misbehave ([thread](https://community.firecore.com/t/hdr10-issues-on-tvos-18/52107)). I did not find Firecore's official format table in the search results, so treat the detail as indicative.

The useful point for Portico: **even the best Apple engine cannot bitstream TrueHD or DTS on Apple TV**. What it can do, and Portico cannot, is decode them on the device and output lossless multichannel PCM, and render PGS and ASS on the device.

### Format truth tables

What happens to each format, per product, as the code reads today. "Direct" = original file; "Remux" = repackaged without re-encoding; "Convert" = re-encoded on the server; "Device" = decoded or drawn on the device.

**Portico on iPhone, iPad and Apple TV (AVPlayer).** Sources: `client-core/client-profile.ts:119-141, 437-474`; `server/internal/playback/delivery_decision.go`; the v1 conversion in `server/internal/playbackv1/capabilities.go:213-316`.

| Input | Outcome |
|---|---|
| MP4 / M4V / MOV container | Direct |
| MKV, MPEG-TS, AVI, WebM container | Remux to HLS when the codecs inside are acceptable, otherwise Convert |
| H.264 8-bit | Direct / Remux |
| H.264 10-bit (Hi10P, common in anime) | Convert |
| HEVC 8- and 10-bit, up to 4096×2304 at 60 fps (1080p on devices without hardware HEVC) | Direct / Remux. An MP4 whose HEVC sample entry is `hev1` is not caught (B-20.9) |
| AV1 | Direct / Remux only where the chip decodes AV1 in hardware (`VTIsHardwareDecodeSupported`); otherwise Convert |
| VP9, MPEG-2, VC-1, MPEG-4 ASP (XviD/DivX) | Convert |
| Anything interlaced | Convert (server deinterlaces; no client declares `interlaced`) |
| HDR10, HLG on an HDR display | Direct / Remux |
| HDR10, HLG on an SDR display or in Low Power Mode | **Convert with tone mapping today**, although AVPlayer can tone-map it itself (B-20.2) |
| Dolby Vision profile 5, 8.1, 8.4 where the device decodes DV | Direct / Remux with the `dvh1` entry |
| Dolby Vision profile 7 | Remux as its HDR10 base layer (no device declares 7) |
| Dolby Vision profile 5 without DV support | Convert, colours approximate, with a warning before play |
| AAC, AC-3, E-AC-3 (including Atmos in E-AC-3), ALAC, FLAC, MP3 | Direct / Remux |
| TrueHD (with or without Atmos), DTS, DTS-HD MA, Opus, Vorbis | **Convert to AAC**: 5.1 at 384 kbps when the source has six or more channels, else stereo (`delivery_decision.go:347-372`) |
| SRT, WebVTT, other text | Device: plain text drawn by the app, or AVPlayer's own track when one matches |
| ASS / SSA | Plain text (styling lost) or burn-in; see domain 04 for which |
| PGS, VobSub, DVB | Burn-in: a full video Convert |

**Portico on Android, Android TV and Fire TV (ExoPlayer).** Sources: `client-core/client-profile.ts:146-166, 490-511`; `PorticoCapabilities.kt`.

| Input | Outcome |
|---|---|
| MP4 / M4V / MOV / MKV / WebM / TS / M2TS | Direct |
| AVI | Remux |
| H.264, HEVC, AV1, VP9, VP8, MPEG-4 | Direct when `MediaCodecList` has a decoder whose profile, level, size, frame rate and bitrate cover the file. A software decoder counts when there is no hardware one (B-20.14) |
| H.264 10-bit | Convert unless a decoder lists High 10 (almost none do) |
| MPEG-2 | **Convert always**: the decoder is declared under the name `mpeg2`, the planner compares against FFmpeg's `mpeg2video` (B-20.8) |
| VC-1 | Convert always: `video/wvc1` is not in the probe's table |
| Anything interlaced | Convert |
| HDR10, HDR10+, HLG, Dolby Vision | Direct only when both a decoder profile and the display report that format; otherwise Convert (or the HDR10 fallback layer for DV 7/8.1) |
| AAC, MP3, Opus, Vorbis, FLAC, PCM | Direct |
| AC-3, E-AC-3, DTS, TrueHD | Direct only where the device has a **decoder** for it. Where a receiver reports bitstream support the codec is marked passthrough and then **refused, so it is converted to AAC** (B-20.1) |
| SRT, WebVTT, other text | Device: plain text drawn by the app |
| ASS / SSA, PGS, VobSub | Plain text or burn-in, as on Apple. (The declared profile says the device renders ASS and PGS; it does not: B-20.11) |

**The others, same inputs.**

| Input | Swiftfin, AVPlayer engine | Swiftfin, VLC or mpv engine | jellyfin-androidtv | Plezy (mpv; Android ExoPlayer with mpv fallback) | Infuse (public statements) |
|---|---|---|---|---|---|
| MKV container | Remux | Direct | Direct | Direct | Direct |
| AVI, WMV, VOB, OGM | Convert | Direct (most) | Direct where the codecs decode | Direct | Direct |
| H.264 10-bit | Convert | Device (software) | Direct only if the decoder reports High 10 | Device (software) | Device |
| HEVC | Direct in MP4 with `hvc1`/`dvh1` only | Direct | Direct, per-profile level checked | Direct where hardware exists, else Convert | Direct |
| AV1 | Direct with hardware | Direct | Direct with a decoder | Direct with hardware | Direct |
| VP9, MPEG-2, VC-1 | Convert (VP9 direct only with hardware) | Device | Direct with a decoder | Device | Device |
| Interlaced | Convert | Device deinterlace | Direct (decoder deinterlaces) | Device, with a deinterlace setting | Device |
| DV profile 7 | HDR10 base (Jellyfin strips) | Not traced | Direct on listed devices, else base layer | Converted to 8.1 on the device, or stripped (Android); base layer on Apple TV | Plays as HDR |
| DTS, DTS-HD | Convert | Device decode | Device decode (FFmpeg) or bitstream | Device decode or bitstream (TV, desktop) | Device decode to PCM |
| TrueHD | Convert | Convert (not in its profile) | Device decode or bitstream | Device decode or bitstream, own IEC packer on Android | Device decode to PCM, no Atmos |
| ASS styled | Convert to WebVTT | Device (libass) | Device (libass) when switched on, else burn-in | Device (libass) | Device |
| PGS | Burn-in | Device | Device (default on) | Device (own parser on Android) | Device |
| VobSub | Burn-in | Device | Burn-in | Device | Device |

The pattern is plain: **Portico's Apple column equals Swiftfin's AVPlayer column, and Swiftfin's users are offered a second engine precisely because of that column.** Portico's Android column is narrower than jellyfin-androidtv's because Portico uses stock Media3 without the FFmpeg audio extension or libass.

### Player settings inventory

"Yes" means a setting or control the viewer can reach. Portico's column is the native apps only.

| Setting or control | Portico native | Swiftfin | jellyfin-androidtv | Plezy | Wholphin |
|---|---|---|---|---|---|
| Choice of engine | No | AVPlayer / VLC / mpv | No (prefer FFmpeg decoders) | ExoPlayer or mpv on Android | ExoPlayer or mpv |
| Compatibility mode / custom device profile | No | Yes | No | Refused-codec list (desktop) | No |
| Force HDR or DV conversion, per format | No | Yes (HDR, DV) | Yes (HDR10, HDR10+, DV 5/7/8 each Auto/On/Off) | Disable DV, DV conversion mode, HDR tone-map mode | Not traced |
| Maximum codec level override | No | Via custom profile | Yes (H.264, HEVC) | No | Not traced |
| Software decoders on/off | No | n/a | Yes | Hardware decoding on/off | AV1 software decode option |
| Audio passthrough per codec | No | n/a | Yes (AC3, E-AC3, DTS, TrueHD: Auto/On/Off) | Yes | Yes |
| Downmix to stereo / channel limit / centre boost | No | No | Downmix yes | Yes (limit, centre boost, normalise downmix) | Not traced |
| Night mode / loudness normalisation / volume boost | Music only | No | Night mode | Normalisation, max volume to 300 % | Not traced |
| Audio delay | No | Not traced | No | Yes | Not traced |
| Subtitle delay | Yes (±0.5 s steps) | Yes | Yes | Yes | Yes |
| Subtitle size / colour / outline / background / position | No (fixed) | Yes | Yes | Yes (plus bold, italic, ASS override, render resolution) | Yes, with separate HDR style |
| ASS on device, PGS on device | No | VLC/mpv | Both, switchable | Yes | Yes |
| Refresh-rate matching | tvOS automatic, no switch | tvOS | Yes, three modes | Yes, plus resolution, plus switch delay | Yes, plus resolution |
| Zoom / fit modes | No | Pinch, aspect fill | Yes | Yes (50–200 %, presets, pinch) | Not traced |
| Playback speed | 0.5–2× | Custom list, long-press speed | Yes | 0.25–8×, scope per title | Yes |
| Skip intervals | Yes (profile setting) | Yes | Fixed | Yes (two sizes) | Yes |
| Hold-to-seek acceleration | No | n/a | Partly | Yes | Yes |
| Rewind a little on resume | No | Yes | Yes | Yes | Yes |
| Scrub thumbnails, chapter ticks | No | Chapter slider | Trickplay, chapters | BIF/trickplay, chapter markers | Trickplay |
| Touch gestures (double-tap, brightness, volume) | No | Fully configurable | n/a (TV) | Yes | n/a (TV) |
| Skip intro / credits ask-auto-off | Yes | Yes | Yes (per segment type) | Yes, with auto-skip delay | Yes |
| Auto-play next, countdown, still watching | Yes (server policy) | Yes | Yes | Yes | Yes |
| Sleep timer | Yes | No | No | Yes, including end of video | Not traced |
| Buffer size | No | No | Yes | Yes | Not traced |
| Quality / bitrate | Server ladder + Automatic | Max bitrate, bitrate test | Max bitrate | Presets, separate cellular preset | Yes |
| External player | No | No | Yes | Yes (Android, with progress sync back) | Yes |
| Picture in Picture | iOS, iPadOS, tvOS, Android phone/tablet | iOS | No | Android, iOS, macOS; auto-PiP switch | No |
| Pause when the app goes to the background | No switch (and see B-20.4, B-20.7) | Yes (switch) | Activity lifecycle | Yes | Yes |
| Playback info / stats | Delivery and why, codecs, bitrate | Yes | Yes, with a device-profile report | Yes, with performance overlay | Yes |
| Shaders / upscaling / ambient lighting | No | No | No | Yes (mpv back end) | No |

## Architectural differences

### A-20.1 Platform players fed by the server, versus an engine the app brings with it

- **Portico:** AVPlayer on Apple, stock ExoPlayer on Android, nothing else. The server makes each file fit the player. Deliberate: "The player on each client is a thin engine that follows that decision" (`04 Playback and Media.md`), and the spec's bar is "implementable … with plain HLS/progressive players (AVPlayer, ExoPlayer, hls.js, Chromecast, VLC, mpv)".
- **Others:** Plezy brings mpv everywhere. Streamyfin (same React Native fork) moved to mpv. Swiftfin offers AVPlayer, VLC and mpv. Wholphin offers ExoPlayer and mpv. jellyfin-androidtv stays on ExoPlayer but adds FFmpeg audio decoding and libass. Infuse is its own engine. **Portico is the only product in the comparison whose Apple client has no way to play an MKV without the server touching it.**
- **Why it matters:** This one choice decides most of the format tables above. Consequences, each of which is its own entry below: every MKV on Apple needs a server remux session; Hi10P, VC-1, MPEG-2, VP9 and interlaced video need a full transcode; DTS and TrueHD become 384 kbps AAC; PGS and VobSub subtitles need a full video transcode. Portico targets weak servers ("target hardware spans weak boxes"), which is exactly where those transcodes hurt. In return Portico gets things the embedded engines struggle with: correct system HDR and Dolby Vision on Apple with no effort, AirPlay, Picture in Picture, low battery use, a small binary, App Store review with no codec questions, and no FFmpeg build to maintain.
- **Assessment:** **Trade-off (subjective), and the single most consequential decision in this domain.** My view: the platform-player choice is right as the *default* and the reference path (it is what makes the protocol implementable by third parties), but the gap to the field is now large enough that a second, optional engine deserves a serious look before the architecture is frozen. The cheapest meaningful step is not mpv; it is two additions to the existing Android engine (A-20.9, A-20.19) and a decision on Apple (F-20.1). What would change my view: evidence that Portico's users' libraries are mostly MP4/H.264/AAC (then the gap is academic), or a measured weak-server test showing remux-to-HLS is effectively free and that burn-in and DTS conversion are rare.

### A-20.2 The player screen is React Native; competitors with heavy engines moved it native

- **Portico:** All chrome is shared React Native (`native/src/player/VideoPlayer.tsx`); native supplies a surface and one line of subtitle text. Deliberate (`06 Clients.md`: "The player layout is fixed"; Android port log: "No `media3-ui`").
- **Others:** Streamyfin's mpv module carries a complete native player screen per platform (Swift and Kotlin, with TV focus coordinators). Swiftfin is SwiftUI. Plezy is Flutter. jellyfin-androidtv uses Leanback overlays.
- **Why it matters:** One chrome for five platforms is a real saving and keeps the layout rule enforceable. The cost is that everything time-critical crosses the bridge: progress facts arrive twice a second, scrubbing previews are JavaScript state, and anything a native player screen gives for free (system scrubbing gestures on tvOS, Siri "what did they say", the system info panel) has to be rebuilt (Portico rebuilt Siri Remote touch scrubbing in `PorticoSiriRemote.m`).
- **Assessment:** **Portico ahead** on consistency and maintenance; **behind** on tvOS system conveniences. I would keep it. Adopting a native screen would be large and would break the layout rule.

### A-20.3 A leased, revisioned command-and-fact protocol between JavaScript and the engine

- **Portico:** Described above; `PorticoPlaybackController.m:136-200`, `NativePlaybackCoordinator.ts`. Commands are validated before any mutation; facts are fenced by binding, sequence and command revision; there are debug self-tests for stale and conflicting commands (`PorticoPlaybackController.m:81-105`).
- **Others:** Streamyfin and Swiftfin call imperative methods (`play`, `seek`, `setSubtitleTrack`) and listen for events. Plezy wraps mpv properties in Dart streams and uses a per-open "outcome" object to stop late events from a previous file being read as the current one (`playback_open_outcome.dart`), which is the same problem solved once, less generally.
- **Why it matters:** It removes a whole class of races (progress from the previous episode marking the next one watched; a seek acknowledged by the wrong item). The cost is weight: the protocol is hard to read (dense single-line Objective-C), every new engine feature needs a field on both sides, and a system-initiated change the protocol does not model (the operating system pausing the player) leaves the two sides disagreeing (B-20.7).
- **Assessment:** **Portico ahead.** Worth keeping and advertising to third-party client authors as the reference adapter pattern. If a second engine is ever added, this protocol is the natural seam: an mpv adapter would implement the same commands and facts.

### A-20.4 Capabilities are a stored device document, not part of each play request

- **Portico:** The device publishes its capabilities after sign-in and whenever the audio route or display changes; the server stores them per device and plans every session from the stored copy (`capability-publisher.ts`, `server/internal/playback/client_profile.go:459-487`). A device that never published is planned against a conservative baseline (H.264 8-bit, stereo AAC, MP4 or HLS-TS, `client_profile.go:369-392`).
- **Jellyfin clients:** Build a DeviceProfile and send it in the body of every `PlaybackInfo` request. Nothing is stored.
- **Why it matters:** Portico's way keeps play requests small, lets the server show an owner what each device claims, and lets the title page preview "Plays as original / Converts audio" before play. Its weakness is staleness: a play that starts within the 1.5-second settle window after a route change (AirPods connected, HDMI switched), or before the first publish has landed, is planned against the old document. Portico waits up to 3 seconds for the first publish at startup (`useNativePlayback.ts:127`) but not on later changes.
- **Assessment:** **Trade-off (subjective).** I prefer Portico's for the preview and diagnostics. A cheap hardening: have the client include the capability revision it believes is current in the session start, so the server can tell a stale plan from a fresh one. Small.

### A-20.5 Two capability documents, and the poorer one wins

- **Portico:** The native apps publish both the client profile and the v1 capability profile. For a device with a v1 document the planner uses only the v1 conversion (`client_profile.go:466-470`), which has no audio output route, no output channel count, no Spatial Audio flag, no `interlaced`, no per-transport codec lists, and never receives `toneMapsToSdr` or `embeddedAudioSwitching` from any client.
- **Others:** One document per client.
- **Why it matters:** Careful work in the client profile is silently discarded: the tvOS HDMI channel probe (COMPAT-03), device-side HDR tone mapping (COMPAT-02), the route check that keeps bitstreams off Bluetooth. See B-20.1 and B-20.2 for the two consequences that change what viewers get.
- **Assessment:** **Portico behind (accidental).** This looks like a migration left half-way: v1 was specified with a smaller schema than the planner had grown. Either extend the v1 schema (add `audioOutput {route, maxChannels, spatial}`, `interlaced`, and have clients send `toneMapsToSdr`), or let the planner prefer the client-profile document when both exist. Small to medium; touches the spec, `capability-profile.ts`, `capabilities.go` and the test matrix (which today exercises only the client-profile path, `declared_profiles_test.go`).

### A-20.6 A declared table plus a probe, with evidence recorded

- **Portico:** Each codec entry says whether it rests on a declared table, a probe or both (`evidence`), "so a wrong decision can be traced to its source" (`client-core/client-profile.ts:5-12`). The Apple table is declared and narrowed by probes; the Android table is replaced wholesale by the `MediaCodecList` probe.
- **Others:** jellyfin-androidtv probes `MediaCodecList` too but asks narrower questions per profile (level for Main separately from Main 10; reference frames) and layers known-defect lists and user overrides on top. Swiftfin's AVPlayer table is static plus four `VTIsHardwareDecodeSupported` answers. Plezy probes hardware only to *remove* codecs.
- **Why it matters:** Evidence tags are a good diagnostic idea nobody else has. But under the v1 conversion every entry is stamped `declared` again (`capabilities.go:240, 269`), so the trace the owner sees no longer says what was probed.
- **Assessment:** **Portico ahead in design, behind in what survives to the server.** Fix with A-20.5.

### A-20.7 A wrong capability claim is corrected by a remembered failure, not by the viewer

- **Portico:** An engine failure closes that delivery route for that file on that sign-in for seven days and replays through the next route automatically, up to three escalations per play (`client-core/index.ts:1453-1468`; `route_failures.go:23`).
- **jellyfin-androidtv:** In-play retry counter: second attempt without direct play, third without direct stream; forgotten when the play ends. **Plezy:** per-media decoder recovery caps, then a switch to mpv. **Swiftfin:** shows the error; the viewer changes the engine or compatibility mode in Settings.
- **Why it matters:** Portico's is the only self-healing design and needs no settings. Its risks: it is only as good as the client's classification of "engine" versus "network" (B-20.6: several ordinary network and server errors are classified as engine failures, which downgrades a file for a week); it keys on the file, so a device that cannot decode, say, any HEVC level 6.1 file pays one failed start per file rather than learning the pattern; and there is no way for a viewer or owner to see or clear the memory.
- **Assessment:** **Portico ahead**, with caveats. Worth adding: an owner-visible list of remembered route failures with a clear button, and a per-device "this codec failed N different files" signal. Small.

### A-20.8 No user override when detection is wrong

- **Portico:** None. No setting changes what the device declares or forces a route. Consistent with "owner choice, low ceremony" and with thin clients.
- **Others:** jellyfin-androidtv has about fifteen (per-codec bitstream, per-HDR-format, level caps, software codecs, ASS/PGS direct play). Swiftfin has compatibility modes, a profile editor, and force-transcode switches for HDR and DV. Plezy has disable-DV, DV conversion mode, passthrough, refused codecs, hardware decoding off.
- **Why it matters:** Televisions and receivers lie. A set that advertises Dolby Vision but shows it purple, a soundbar that claims E-AC-3 and plays silence, a Fire TV that reports a level it chokes on. A-20.7 catches failures the engine *reports*; it cannot catch a picture that plays but looks wrong, or audio that plays silent. Every competitor's override exists because of a support thread.
- **Assessment:** **Portico behind.** I would add a very small set, on the device, under an "If something looks or sounds wrong" heading rather than as tuning knobs: "Dolby Vision: Automatic / Off", "HDR: Automatic / Off", "Surround sound passthrough: Automatic / Off", and "Prefer converting on the server for this device". They simply edit the published capabilities. Small. This does not conflict with "quality changes only when a person asks": a person is asking.

### A-20.9 Lossless and DTS audio become 384 kbps AAC

- **Portico:** Any audio codec the device cannot decode is converted to AAC, 5.1 at 384 kbps for a six-or-more-channel source if the device's AAC decoder and output take six channels, otherwise stereo; E-AC-3 or AC-3 at 640 kbps only for a passthrough-only device on an HDMI route (`delivery_decision.go:347-372`). Seven-point-one is always reduced to 5.1 and Atmos objects are lost. The spec says something different (§5.4: "DTS/TrueHD to Apple TV → E-AC-3 5.1 (7.1 when the route supports it); PCM multichannel when local and the profile allows").
- **Others:** jellyfin-androidtv decodes DTS and TrueHD on the device with the FFmpeg extension, to full-resolution multichannel PCM, or bitstreams them. Plezy and mpv decode on the device. Infuse decodes to lossless PCM. Swiftfin's AVPlayer engine has Jellyfin transcode to one of AAC/AC-3/E-AC-3/ALAC/FLAC, with up to eight channels.
- **Why it matters:** This is the most common real-world conversion for a Blu-ray library (most UHD remuxes carry TrueHD or DTS-HD MA as the main track), so it is the server load viewers will meet most often, and it is audibly lossy on a good system. Audio conversion alone is cheap compared with video, so the weak-server concern is mild; the concern is fidelity (contract P04: "Preserve compatible source streams").
- **Assessment:** **Portico behind.** Three independent improvements, in rising cost: (1) convert to **FLAC** (lossless, AVPlayer and ExoPlayer both play multichannel FLAC in fragmented MP4, and the server's copy list already includes it) or to E-AC-3 at a high bitrate instead of 384 kbps AAC, and keep 7.1 where the device takes eight channels: small, server only; (2) add the Media3 FFmpeg audio decoder to the Android app so DTS and TrueHD play directly there: small in code, but adds a native library, about 2–4 MB per ABI, and an LGPL component; (3) nothing equivalent exists for AVPlayer short of a second engine.

### A-20.10 Passthrough is detected from the HDMI device's encodings, not from the audio sink's own answer

- **Portico (Android):** Reads `AudioDeviceInfo.getEncodings()` of HDMI, ARC, eARC and digital-line outputs (`PorticoCapabilities.kt:240-258`). Built-in television speakers are excluded. The server then refuses a passthrough codec unless the route is HDMI, ARC or optical (`delivery_decision.go:215, 374-376`).
- **jellyfin-androidtv:** Asks Media3's `AudioCapabilities.isPassthroughPlaybackSupported` with the same audio attributes the player will use (`codec/audioPassthrough.kt`), which also covers Fire OS's and other vendors' surround settings and televisions whose internal speakers accept Dolby bitstreams. **Plezy:** the same, plus its own carriers for TrueHD and DTS-HD because the platform's answers are unreliable on real devices.
- **Why it matters:** The question that matters is "will the AudioTrack ExoPlayer is about to open accept this encoding", and only the sink's own capability query answers it. Portico's probe can say yes where the sink says no (Bluetooth headphones connected while the HDMI receiver is still listed: Portico builds the passthrough list from every HDMI device regardless of where audio is routed, `:243`) and no where the sink says yes (an integrated television with no AC-3 decoder that passes AC-3 to its own audio processor).
- **Assessment:** **Portico behind.** Replace the hand-rolled probe with `AudioCapabilities` (Media3 is already a dependency). Small. Moot until B-20.1 is fixed, because today the result is discarded.

### A-20.11 On Apple the system owns the audio output; Portico only declares

- **Portico:** Sets the audio session to playback / movie mode with the long-form video route policy and multichannel content support (`PorticoPlaybackController.m:428-438`), declares AAC, AC-3, E-AC-3 with Atmos, ALAC, FLAC, and lets tvOS decide what reaches the receiver. No in-app choice between Dolby Atmos pass-through and multichannel PCM.
- **Others:** The same constraint binds everyone on Apple TV (Infuse: "Apple TV does not allow passthrough"). mpv-based apps choose between an AudioUnit output (decode everything) and an AVFoundation audio renderer (which can hand Dolby Digital Plus with Atmos to the system); Plezy advertises passthrough on Apple TV.
- **Why it matters:** Portico's Apple audio path is as good as an AVPlayer app can be: E-AC-3 with Atmos copied into HLS reaches the receiver as Atmos. The only gap versus Infuse is lossless audio (A-20.9).
- **Assessment:** **Trade-off / parity.** Nothing to change here beyond A-20.9.

### A-20.12 Downmixing is the server's job

- **Portico:** When the device cannot take the channel count, the server downmixes during conversion; the client has no downmix controls (no centre boost, no night mode, no dialogue lift) for video. Under the v1 conversion the device's real output channel count is not known to the planner at all (A-20.5).
- **Others:** Plezy: output channel limit, centre-channel boost in dB, "normalise downmix", loudness normalisation, volume to 300 %. jellyfin-androidtv: downmix-to-stereo switch and night mode.
- **Why it matters:** "I can't hear the dialogue" is one of the most common complaints in home video. A client-side dynamic-range or centre-boost control costs the server nothing.
- **Assessment:** **Portico behind**, mildly. On Android a `DynamicsProcessing` or `LoudnessEnhancer` effect bound to the audio session is small (Plezy's `AudioNormalizationEffect.kt` is 200 lines). On AVPlayer it needs an audio tap (`MTAudioProcessingTap`), which does not work on HLS streams, so it is effectively server-side there. Consider a server "dialogue boost" conversion option instead; medium.

### A-20.13 HDR on an SDR screen: device tone mapping is intended but not delivered

- **Portico:** The client profile says an Apple device with hardware HEVC decodes HDR10 and HLG and tone-maps them itself for an SDR screen, "so the server must not convert those titles" (`client-core/client-profile.ts:452-456`, COMPAT-02). The v1 document has a `toneMapsToSdr` field for this; no client sends it, and the conversion drops any HDR range the display does not show (`capabilities.go:254`).
- **Others:** Swiftfin's AVPlayer profile always accepts HDR10/HLG when HEVC hardware exists, with a "force HDR transcode" switch. Plezy/mpv tone-map on the device with selectable algorithms. jellyfin-androidtv trusts decoder profiles and lets the user override.
- **Why it matters:** Tone-mapping a 4K HDR film on the server is the heaviest thing a media server does. On a weak box it will not run in real time. See B-20.2.
- **Assessment:** **Portico behind (accidental).** Fix is small: send `toneMapsToSdr: true` for Apple HEVC and AV1 entries whose client-profile ranges exceed the display's. For Android, device tone mapping is unreliable across vendors, so the current "display must show it" rule is defensible there; an override (A-20.8) covers the sets that do it well.

### A-20.14 Dolby Vision profile 7 falls back to its base layer by copy; others convert it on the device

- **Portico:** Declares profiles 5 and 8 on Apple and whatever the Dolby Vision decoder lists on Android. For a profile the device lacks, the planner uses the fallback layer named by the stream's compatibility id (HDR10, HLG or SDR), by copy; profile 5 has none and is converted with an "approximate colours" warning shown before play (`delivery_decision.go:126-155`; `native/src/player/dolby-vision.ts`). A Dolby Vision sample entry is never sent to a device that does not know it (`:200-204`).
- **Others:** Plezy converts profile 7 to 8.1 **on the device** as the bytes pass through ExoPlayer (libdovi), or strips the Dolby Vision data, selectable; strips HDR10+ data from Dolby Vision streams on chipsets that crash on the combination; has a "disable Dolby Vision" switch. jellyfin-androidtv keeps device lists for both defects. Jellyfin's server strips Dolby Vision data with an FFmpeg bitstream filter when the client lacks it.
- **Why it matters:** Portico's decision table is more careful than most (the sample-entry rule and the pre-play warning are not in any other open client). What it lacks is the profile 7 → 8.1 conversion, which is what turns a UHD Blu-ray remux into real Dolby Vision on an Apple TV or a Fire TV instead of plain HDR10; on Portico that would be a server-side bitstream rewrite during the HLS remux (no re-encode). I did not find such a filter in the server's HLS arguments (grep for `dovi` in `internal/playback`, `decoder`, `mediaexec` finds none), and I did not find one that strips the enhancement layer either; that belongs to domain 02.
- **Assessment:** **Portico ahead** on honesty and safety; **behind** on profile 7 picture quality. Medium, server side.

### A-20.15 No known-defect device list

- **Portico:** One special case: Apple TV HD (`AppleTV5,x`) is capped at 1080p H.264 (`client-core/client-profile.ts:461-464`).
- **Others:** jellyfin-androidtv and Wholphin list Fire TV models with the DV + HDR10+ crash and the Shield's unreported profile 7. Plezy handles the same chipsets in the bitstream.
- **Why it matters:** Fire TV is a named Portico target ("Fire TV" flavour, Vega). The four Fire TV 4K models in `jellyfin-androidtv/…/KnownDefects.kt` are among the best-selling streaming devices.
- **Assessment:** **Portico behind.** A small table in client-core keyed by `Build.MODEL` (the probe already reports `model`), applied in `androidProfile`. Small. A-20.7 partly covers crashes that surface as decoder errors, but a black screen with sound does not raise one.

### A-20.16 Frame-rate and dynamic-range matching: tvOS only, automatic, and late

- **Portico:** tvOS: criteria taken from the asset and applied once the first frame is ready for display; playback is not held while the television switches (`PorticoPlayer.m:55-60`). No switch in Portico's settings (tvOS's own "Match Content" settings decide). Android TV and Fire TV: nothing; ExoPlayer's default asks only for *seamless* rate changes, which most televisions cannot do, so 24 fps films play at 60 Hz with judder.
- **Others:** Plezy sets criteria from the decoded stream and keeps playback paused until the mode switch ends, with a configurable extra delay. Swiftfin sets them before starting the clock. jellyfin-androidtv, Wholphin and Plezy all set `preferredDisplayModeId` on Android TV with a setting (off / rate only / rate and resolution), and Wholphin waits for the display-changed callback plus two seconds.
- **Why it matters:** Judder on 24 fps content is the most visible picture-quality difference between a "real" TV client and a casual one, and Android TV is where most users will meet it. On tvOS the late switch risks a black screen over the first seconds of every film whose rate or range differs from the home screen's (B-20.5).
- **Assessment:** **Portico behind.** tvOS: apply criteria at `readyToPlay`, hold `paused` until `AVDisplayManager.isDisplayModeSwitchInProgress` clears (the command protocol already has a place for "not yet": do not announce `ready`). Small. Android: a `Display.Mode` chooser and a window attribute, with the wait; small to medium, plus a setting.

### A-20.17 One server-normalised subtitle document drawn by the app

- **Portico:** Every text track becomes plain cues with two flags; the app draws one label. "One renderer, one look, and the same rule as Apple" (Android port log). Deliberate.
- **Others:** ExoPlayer clients let the engine render SubRip, WebVTT, SSA and PGS; libass clients render ASS faithfully with embedded fonts; mpv does everything. AVPlayer clients use HLS WebVTT renditions, which the system styles and carries to AirPlay and Picture in Picture.
- **Why it matters:** Portico's way guarantees identical behaviour on five platforms, lets subtitles be switched or re-timed without touching the stream, and works for sidecar and downloaded subtitles on direct play (where AVPlayer has no mechanism at all). The costs: styling is reduced to italic and top; positioned signs, karaoke, colours and fonts in ASS are lost unless burned in; overlapping cues are joined into one centred block; the system's caption style (size, colour, background, set in Accessibility) is ignored for app-drawn text; and they do not travel to AirPlay or Picture in Picture on Apple.
- **Assessment:** **Trade-off (subjective).** I think the normalised document is a good base and should stay, but the renderer should grow: read the system caption style on Apple (`MACaptionAppearance`) and Android (`CaptioningManager`), honour the two existing profile settings (B-20.12), and carry position and basic colour in the cue document. Small to medium. Faithful ASS needs libass on the device (F-20.4).

### A-20.18 Subtitles are re-authorised every 25 seconds

- **Portico:** The cue document is fetched again every 25 seconds with `If-None-Match`; the text is blanked 40 seconds after the last good answer, and immediately when a re-check fails (`PorticoSubtitleSelection.m:89, 117`; Kotlin `:124-135, 184-190`).
- **Others:** Fetch once.
- **Why it matters:** It is a revocation mechanism (a session the owner ended loses its subtitles as well as its video). It also means a chatty request for the whole length of every subtitled play, and that a single dropped request removes subtitles the device already holds (B-20.13).
- **Assessment:** **Portico's choice looks over-engineered for the threat.** Video segments already stop when a session is revoked; plain-text subtitles outliving the video by a few minutes on a device that already downloaded them is not a meaningful leak. I would fetch once per selection and drop the timer. Small, and it removes a failure mode. What would change my view: a requirement that subtitle *edits* by the owner appear live mid-play (then keep the ETag check but never blank on failure).

### A-20.19 Image and styled subtitles force a video transcode

- **Portico:** PGS, VobSub, DVB and (when chosen as styled) ASS are burned in by the server, with a confirmation first on 4K/HDR sources (`PlayerOptions.tsx:237`; `playbackv1/subtitles.go:27-34`). The owner's transcoding switch disables them entirely.
- **Others:** jellyfin-androidtv renders PGS on the device by default and ASS with libass when switched on; Plezy and Wholphin render both; VLC and mpv engines render everything; only AVPlayer-engine clients burn in.
- **Why it matters:** PGS is the subtitle format of every Blu-ray rip. On Portico, turning on English subtitles for a 4K HDR remux turns a direct play into a 4K HEVC decode, tone-map-preserving re-encode with overlay: on a weak server, unwatchable; on any server, a quality loss. This is the single largest gap for the weak-server target after A-20.13.
- **Assessment:** **Portico behind.** Options: (a) Android: let ExoPlayer render PGS itself (it has a PGS decoder; Plezy replaced it for multi-object streams) on direct play, and deliver PGS as a sidecar `.sup` for HLS: medium. (b) All platforms: have the **server** rasterise image subtitles once into a timed image track (PNG tiles with positions) that the app draws over the video, in the same spirit as the cue document: medium to large, but it keeps "one renderer" and works on AVPlayer too; the spec already foresees client rendering ("PGS/VobSub when `client`"). (c) OCR to text: lossy, not recommended. I favour (b).

### A-20.20 Audio track choice on a direct-played file restarts the session

- **Portico:** Engine-side track switching exists only for HLS renditions. For an original file, choosing another track asks the server again, and the planner sends an HLS route because `embeddedAudioSwitching` is false for v1 devices (`delivery_plan.go:366`; `capabilities.go:220`; the native feature map never sets it, `native/src/bridge/audio-decode.ts:31, 70`).
- **Others:** ExoPlayer, VLC and mpv clients switch tracks in the engine instantly; AVPlayer clients use the media selection group for MP4.
- **Why it matters:** A dual-audio anime MKV on Android is a direct play until the viewer picks Japanese, then becomes a server remux session. Cheap, but a reload and a server session where competitors have neither.
- **Assessment:** **Portico behind**, mildly. Implementing real embedded switching on Android (map the server's stream index to ExoPlayer's track by position within the audio group) and on Apple MP4 is small to medium, and the client profile already claims it (`client-profile.ts:139, 164`: a doc/code disagreement).

### A-20.21 Subtitle look is fixed

- **Portico:** White text, 70 % black box, 3.5 % of picture width capped at 40 points, bottom-centre inside the picture, system font. The profile's subtitle size and background settings are offered on the web only (`client-core/presentation/settings-structure.ts:205-210`, with the comment "Apple's system player styles them from the device's Accessibility settings", which is not true of the app-drawn label; see B-20.12).
- **Others:** Every comparison client has size, colour, outline or shadow, background opacity and vertical position; Wholphin keeps a separate style for HDR content (white subtitles at full brightness are painful in HDR); Plezy can anchor subtitles to the screen rather than the picture, so they sit in the black bar of a 2.39:1 film.
- **Why it matters:** Accessibility (large text), comfort in HDR, and taste. On Apple TV the 40-point cap makes text about 3.7 % of screen height, which is on the small side for a television across a room.
- **Assessment:** **Portico behind.** Small: pass size, background and position through the existing subtitle command and apply them in the two label classes; default them from the system caption style.

### A-20.22 Every seek is exact and verified

- **Portico:** Zero-tolerance seeks; the result must land within 0.5 s or the seek is reported failed and playback pauses.
- **Others:** mpv and ExoPlayer clients typically seek to the nearest keyframe while scrubbing (fast) and exactly on release; AVPlayer clients use a tolerance while dragging. jellyfin-androidtv turns on constant-bitrate seeking and widens the TS timestamp search so files without a seek index still seek (`ExoPlayerBackend.kt:82-89`); Plezy repairs MKV files with no cue index (`CuelessSeekExtractorWrapper.kt`).
- **Why it matters:** Exact seeks are the right default for resume accuracy and for Watch Together. They are slower on high-bitrate direct plays (the decoder must decode from the previous keyframe), and the strict verification turns "the file has no index" into "that position is not available" rather than "close enough" (B-20.19).
- **Assessment:** **Trade-off (subjective).** Keep exact seeks for committed seeks; consider `SeekParameters.CLOSEST_SYNC` (Android) and a tolerance (Apple) for repeated skip presses and scrubbing, and accept the landed position instead of failing when the engine lands elsewhere on a file with no index. Small.

### A-20.23 The timeline has no pictures and no marks

- **Portico:** A bar and two clocks (`native/src/player/Timeline.tsx`). No scrub thumbnails, no chapter ticks or chapter list in the player, no intro/credits marks, although the device declares `trickplay: true, chapters: true` (B-20.15) and the server has chapter thumbnails and markers.
- **Others:** All of them have thumbnails while scrubbing (Jellyfin trickplay, Plex BIF) and most show chapter marks; Swiftfin has a chapter slider; jellyfin-android draws chapter markings on the bar.
- **Why it matters:** Scrubbing blind on a television remote is the most-felt gap in day-to-day use.
- **Assessment:** **Portico behind.** Medium (a thumbnail strip component, tile-sheet fetch and crop; ticks are small). Check domain 04 for what the server already publishes.

### A-20.24 Television seeking: fixed steps, no acceleration; Siri Remote surface scrubbing is custom

- **Portico:** Left/right on the focused timeline skip by the profile's back/forward seconds per press (`VideoPlayer.tsx:163-167`). The Android port log notes "two D-pad presses moved the target about 20 seconds here. I did not compare the hold-to-accelerate behaviour". Apple TV adds finger-position scrubbing across a tenth of the duration per swipe (`NativeSiriRemote.ts`, `scrubTarget`).
- **Others:** Wholphin scales the step with hold duration and film length (up to 10× on long films); Plezy has two step sizes and hold-to-seek; AVKit on tvOS gives the system scrubber for free.
- **Assessment:** **Portico behind** on Android TV and Fire TV, roughly at parity on Apple TV. Small: key-repeat counting in the shared player.

### A-20.25 Handheld player has no gestures

- **Portico:** A tap shows or hides the controls. No double-tap to skip, no brightness or volume swipe, no pinch to fill, no long-press for speed, no touch lock.
- **Others:** Swiftfin (every gesture configurable), jellyfin-android, Plezy and Streamyfin all have them; they are the convention set by YouTube, Netflix and VLC.
- **Assessment:** **Portico behind.** Small to medium in the shared React Native player; pinch-to-fill needs a `videoGravity` / scaling property on the native views.

### A-20.26 No zoom or aspect modes

- **Portico:** Always aspect-fit (`AVLayerVideoGravityResizeAspect`, `PorticoPlayer.m:17`; manual letterbox, `PorticoPlayerView.kt:63-79`).
- **Others:** Fit / fill / stretch / zoom percentage on every Android TV client and Plezy; pinch on phones.
- **Why it matters:** 4:3 content with baked-in bars, ultrawide phones, and 21:9 films on phones with notches.
- **Assessment:** **Portico behind.** Small.

### A-20.27 Quality is one rendition; "Automatic" is the server's choice

- **Portico:** The server sends a single rendition; the player caps the engine at the screen size only as a safety (`PorticoPlaybackController.m:25-37`, `.kt:391-395`). "Automatic" means the server plans from the profile's per-network limits. The spec describes a client-side Auto that picks a limit from its own bandwidth estimate (§5.2); the native apps do not measure bandwidth or send one.
- **Others:** Swiftfin has a bitrate test that measures the link and sets a ceiling; jellyfin clients have a max-bitrate setting; Plezy has presets with a separate cellular preset. None does true adaptive switching for a personal server either.
- **Why it matters:** "Quality never changes unless asked" is a Portico principle and the single rendition honours it. The flip side is behaviour on a poor link: nothing lowers the bitrate, so a remote direct play of an 80 Mbit/s remux stalls until the viewer picks a lower rung by hand. The stall is at least explained ("Reconnecting…").
- **Assessment:** **Trade-off (deliberate, cited in the brief and spec).** Consider a non-intrusive nudge instead of adaptation: after repeated stalls, offer "This connection can't keep up. Switch to 1080p 8 Mbit/s?" Small, and still "a person asked".

### A-20.28 Buffering is the engine default with a low-memory cap

- **Portico:** AVPlayer automatic; 30-second forward buffer on devices with 2.5 GB or less (`PorticoPlaybackController.m:33`). ExoPlayer default load control; 15–30 s on low-memory devices (`.kt:378-380`).
- **Others:** jellyfin-androidtv and Plezy expose a buffer-size setting; Plezy sizes the byte budget against Media3's own default after finding that a flat 64 MiB is only five seconds of a 100 Mbit/s remux (`LoadControlPolicy.kt`).
- **Why it matters:** ExoPlayer's default targets 50 seconds *or* a byte budget (about 144 MB for video plus audio) with size taking priority, which for UHD remuxes is roughly ten seconds. Fine on a LAN, thin on Wi-Fi.
- **Assessment:** **Parity for now.** No change suggested until device testing shows stalls; note Plezy's finding for when it does.

### A-20.29 The engine never plays "by itself": every state change is a JavaScript intent

- **Portico:** Native never resumes, seeks or changes track on its own; remote commands, interruptions and route changes are turned into events that JavaScript converts into its usual intent (`PorticoSystemMedia.m:9-23`).
- **Others:** Native players act directly on the engine and tell the UI afterwards.
- **Why it matters:** One place for Watch Together rules and progress reporting. But the engine *can* be changed from outside without a command: the operating system pauses video when the app is backgrounded, and audio focus loss pauses ExoPlayer. Portico handles the cases it listens for (interruption, route change, focus loss) and not the rest (B-20.4, B-20.7).
- **Assessment:** **Portico ahead in principle; incomplete in coverage.** Add a reconciliation rule: when a `progress` fact says paused while the intent is playing and no seek or route change is pending, either re-apply the intent or adopt the pause. Small.

### A-20.30 Leaving the app: automatic Picture in Picture on phones, nothing on Android TV

- **Portico:** iPhone/iPad: Picture in Picture starts automatically when the app is left (if the system setting allows). Android phones/tablets: the same from Android 12, and via `onUserLeaveHint` before. Android TV and Fire TV: no Picture in Picture and no lifecycle handling at all (`MainActivity.kt` has no `onStop`/`onPause` player hook; no lifecycle listener in the native folder).
- **Others:** Wholphin and jellyfin-android pause on activity pause and resume on return; Swiftfin has a "pause on background" switch; Plezy has an auto-PiP setting.
- **Assessment:** **Portico behind on Android (B-20.4).** Small.

### A-20.31 System media integration is hand-built rather than inherited

- **Portico:** `MPNowPlayingInfoCenter` and `MPRemoteCommandCenter` by hand on Apple; a Media3 `MediaSession` whose player is a forwarding proxy on Android, without a `MediaSessionService`, so there is no notification or lock-screen control for video in the background (Android port log, "Differences").
- **Others:** AVKit-based players get Now Playing, Siri commands, the info panel and AirPlay metadata for free; Android clients run a playback service.
- **Why it matters:** Portico's approach works with its custom chrome and intent model and adds nice touches (skip-forward inside an intro jumps to its end; next/previous step through chapters when there is no queue, though chapters are never actually supplied for video: B-20.15). What it lacks on tvOS are the AVKit-only features: Siri "what did they say?", "skip ahead two minutes" accuracy, the swipe-down info panel.
- **Assessment:** **Trade-off (subjective).** Keep. Document the tvOS Siri gaps as known.

### A-20.32 Picture in Picture uses the player layer

- **Portico:** `AVPictureInPictureController(playerLayer:)`, so the system window shows AVPlayer's video and only AVPlayer-drawn subtitles; the app reports `subtitlesCarried: false` so the player can say so (`PorticoPlayer.m:28-52`). tvOS Picture in Picture is offered. Android draws the app's subtitle label in the small window.
- **Others:** mpv-based apps on Apple use a sample-buffer display layer as the content source (Streamyfin `PiPController.swift`; Plezy `MpvPipController.swift`), which lets them composite their own subtitles into the frames.
- **Assessment:** **Parity**, and the honesty flag is a good touch. Converting embedded text tracks to HLS WebVTT renditions (so AVPlayer draws them) would carry more subtitles into Picture in Picture and AirPlay; that is a server change (domain 04).

### A-20.33 Playback speed: seven fixed steps to 2×

- **Portico:** 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2; live is 1× only; Apple drops the slow steps when the item cannot play slow-forward (`PorticoPlaybackController.m:376-380`). The profile's default video speed is not applied on native (B-20.17).
- **Others:** Plezy 0.25×–8× with a scope (this title / this show / everywhere); Swiftfin user-defined rate list and a long-press temporary speed-up.
- **Assessment:** **Portico slightly behind.** Small.

### A-20.34 Post-play and skip are server policy

- **Portico:** The server says whether to autoplay, the countdown, and whether a "still watching?" check is due; auto-skip of a marker happens only when the server marks it safe for unattended skipping, and every skip is reported back for marker-quality analytics (`PostPlay.tsx`, `SkipPrompt.tsx`, spec §4.2).
- **Others:** Client-side settings and timers; Plezy has an "auto-skip delay" and regex patterns for chapter-name fallback; jellyfin-androidtv has per-segment-type actions.
- **Assessment:** **Portico ahead** (consistent across devices, and the safe-to-auto-skip flag is unique). A chapter-name fallback for files with "Intro"/"Credits" chapters is a server feature to consider (domain 04).

### A-20.35 Start-up makes an extra request to decide whether to warn about Dolby Vision

- **Portico:** Before every video play the app fetches playback options to look for the approximate-colours reason, then starts (`usePlayer.ts:388-410`).
- **Others:** Start immediately; no equivalent warning.
- **Why it matters:** One extra round trip on every start (B-20.18 for the slow-server case). The warning itself is a quality-honesty feature nobody else has.
- **Assessment:** **Trade-off.** Carry the flag on the title page's data or on the session's first response and warn from there (the session already carries `decision.video.reasons`), so the common path has no extra request. Small.

### A-20.36 Progress is observed twice a second and checkpointed on change

- **Portico:** Native emits a `progress` fact every 0.5 s; client-core checkpoints to the server when the position moves 3 s or the state changes (`client-core/index.ts:1438-1443`), and the server's lease expects a report every 10 s.
- **Others:** Jellyfin clients report every 10 s and on state changes.
- **Why it matters:** Portico's resume point is fresher after a crash or power cut. The cost is bridge traffic and, if each checkpoint is a request, more server chatter per stream than Jellyfin; whether checkpoints are coalesced before sending was not traced here (domain 02).
- **Assessment:** **Unclear** (depends on coalescing). Worth a measurement with a couple of hundred concurrent players, since that is a stated target.

### A-20.37 Live TV positions are wall-clock dates

- **Portico:** For a channel the command carries the timeline origin; Apple seeks by date (`seekToDate`, with one exact refinement when the date seek lands on a nearby keyframe, `PorticoPlaybackController.m:349-368`), Android converts through the HLS window start (`PorticoPlaybackController.kt:885-897`). Speed is locked to 1×.
- **Others:** Treat live as an ordinary sliding HLS window with engine positions.
- **Why it matters:** Wall-clock positions let pause, rewind and "behind live" labels survive a buffer restart. Android has no handling for falling out of the live window (`ERROR_CODE_BEHIND_LIVE_WINDOW` is classified as an engine error; B-20.6).
- **Assessment:** **Portico ahead in model**, with one Android gap.

### A-20.38 Downloads play in the same controller

- **Portico:** A downloaded file is accepted only from the app's own Downloads folder after resolving symlinks (`PorticoPlaybackController.m:16-24`, `.kt:1022-1031`) and plays through the same lease, commands and facts, with progress journaled on the device. (An older second path through `AVPlayerViewController` still exists in `PorticoDownloads.m:451-470`.)
- **Others:** Similar; Plezy hands downloads to mpv with sidecar subtitles.
- **Assessment:** **Parity.** Note for domain 14: an offline file must be something AVPlayer plays, so downloads to Apple devices are always MP4-family conversions or remuxes, where an mpv-based client downloads the original.

### A-20.39 Vega plays HLS through a loader the port wrote

- **Portico:** Amazon's media element will not open plain-HTTP addresses or HLS from an address, so the Vega app fetches segments itself and feeds media source extensions (`apps/vega/src/bridge/hls.ts`, 337 lines); speed is fixed at 1×; from an HTTP server only HLS is requested (`capability-profile.ts`).
- **Others:** Amazon's documented route is a patched Shaka Player.
- **Why it matters:** A private HLS client is a maintenance commitment (discontinuities, live windows, retries) on a platform nobody could test on hardware.
- **Assessment:** **Risk to track**, not a difference to adopt from anyone. Also: the declared Vega profile marks AC-3 and E-AC-3 as passthrough and has no audio route, so the planner refuses both (B-20.1).

### A-20.40 Licences and distribution

- **Portico:** GPL-3.0-or-later, no external contributions, App Store and Play Store distribution intended. Today the native apps link only platform frameworks, Media3 (Apache-2.0), OkHttp (Apache-2.0) and the Google Cast SDK (proprietary, `google` flavour only).
- **Others:** Plezy is GPL-3.0 and ships mpv on the App Store. Streamyfin is MPL-2.0 with MPVKit. Swiftfin is MPL-2.0 with VLCKit (LGPL-2.1) and mpv. jellyfin-androidtv is GPL-2.0 with an LGPL FFmpeg build.
- **Why it matters:** mpv and FFmpeg are LGPL-2.1-or-later when built without GPL-only parts (mpv's `-Dgpl=false`; FFmpeg without `--enable-gpl`), which is compatible with a GPL-3.0 app. The App Store question is the usual one for copyleft: Apple's terms add restrictions the GPL does not allow a distributor to add, which matters when someone *else* holds copyright in the GPL code. Portico owns all of its own code (no outside contributions), so it can grant Apple whatever is needed for its own code; the LGPL libraries are the part to check (LGPL allows distribution under other terms provided relinking is possible, which statically linked iOS apps satisfy only by offering object files). VLC, Plezy, Streamyfin and Infuse-like apps ship this way today. This is a question for a lawyer, not a blocker in practice. Patent licences are separate: Dolby and DTS decoders in FFmpeg are unlicensed implementations; Infuse pays, the open-source apps do not.
- **Assessment:** **Unclear (legal), not technical.** If a second engine is considered, get the licence position written down first.

### A-20.41 Binary size, battery and maintenance

- **Portico:** No bundled codecs. Hardware decoding always (AVPlayer and MediaCodec), which is the best case for battery and heat.
- **Others:** libmpv with FFmpeg adds very roughly 15–30 MB per architecture to an app, a build pipeline to own (Plezy pins five per-platform builds by checksum), and software decoding paths that drain a phone when hardware decoding does not apply (Hi10P, VC-1).
- **Assessment:** **Portico ahead.** Any second engine should be opt-in per play ("this file needs the alternative player") so the default keeps these properties.

## Missing features and capabilities to consider

Sizes are rough: **small** = days, **medium** = a week or two, **large** = a month or more.

### F-20.1 An optional second engine for what the platform player cannot do

- **What:** A universal engine (mpv) available as an alternative back end, chosen automatically per play when the planner sees that the platform player would need a video transcode or a burn-in, and the device has the power to decode locally.
- **Who has it:** Swiftfin (viewer picks AVPlayer / VLC / mpv), Wholphin (ExoPlayer / mpv), Plezy (mpv everywhere; on Android ExoPlayer first with automatic hand-over to mpv), Streamyfin (mpv on the same React Native fork Portico uses).
- **Value:** Removes the server from most plays: MKV direct on Apple; Hi10P, VC-1, MPEG-2, interlaced and VP9 decoded on the device; DTS and TrueHD decoded to lossless PCM; PGS and ASS drawn on the device with full styling. For a weak server this is the difference between "plays" and "cannot play" for a Blu-ray-remux library.
- **What it would involve in React Native:** (1) a native module per platform wrapping libmpv (MPVKit on Apple; a libmpv AAR on Android), rendering through `vo=avfoundation` / `AVSampleBufferDisplayLayer` on Apple so HDR, Dolby Vision, frame-rate matching and Picture in Picture stay system-handled; (2) an adapter that speaks Portico's existing command-and-fact protocol (A-20.3), so nothing in JavaScript changes except the name of the engine; (3) a new delivery mode in the capability profile ("this device also has engine X with these capabilities"), so the planner can offer "original, to be played by the alternative engine" alongside today's offers; (4) subtitle and audio selection mapped to mpv track ids from the server's stream indexes; (5) a pinned, reproducible mpv/FFmpeg build per platform and architecture; (6) licence review (A-20.40). Streamyfin's module is the closest prior art; its Apple renderer alone is 1,760 lines and its module about 31,000 with the native screens Portico would not need.
- **Costs and conflicts:** 15–30 MB per architecture; a build pipeline; software decoding heats phones; AirPlay video does not work from mpv (only screen mirroring); engine bugs become Portico's. It does not conflict with thin clients (the server still decides) but it does end "the player is a thin engine".
- **Size:** Large.
- **Opinion:** Do not start here. Do F-20.2, F-20.3 and F-20.5 first; they close most of the Android gap inside the engine Portico already has. Then decide on Apple with data: how often do real libraries hit a video transcode or burn-in on Apple devices? If often, mpv on tvOS is the highest-value single addition, because Apple TV is the reference client and has the power to decode anything.

### F-20.2 FFmpeg audio decoding on Android

- **What:** Add Media3's FFmpeg audio decoder extension so DTS, DTS-HD, TrueHD, AC-3 and E-AC-3 decode on the device when there is no platform decoder and no receiver.
- **Who:** jellyfin-androidtv, jellyfin-android, Wholphin, Plezy (all ship a prebuilt `media3-ffmpeg-decoder`).
- **Value:** Ends audio conversion for Android clients; lossless multichannel PCM out.
- **Size:** Small in code (renderer mode "on", declare the codecs with evidence `declared`); adds a native library and an LGPL component. No Portico principle is touched.
- **Opinion:** Yes.

### F-20.3 Render ASS and PGS on the device (Android first)

- **What:** libass for styled subtitles and on-device PGS/VobSub rendering.
- **Who:** jellyfin-androidtv (`ass-media3`, switchable), jellyfin-android, Wholphin, Plezy (libass-android, own PGS parser, a calibrator for subtitle/video plane latency).
- **Value:** Removes the most expensive conversion Portico performs (A-20.19) and restores anime typesetting.
- **Conflict:** Breaks "one renderer, one look" on purpose for these formats. On direct play the engine reads the track from the file; for HLS the track must be delivered as a sidecar.
- **Size:** Medium for Android. On Apple it needs either a second engine or the server-rasterised image track described in A-20.19 (b).
- **Opinion:** Yes for PGS (the common case); ASS second.

### F-20.4 A better audio conversion target

- **What:** Convert undecodable audio to FLAC (lossless) or high-bitrate E-AC-3, keep 7.1, instead of 384 kbps AAC 5.1. Implement what the spec already says (§5.4).
- **Size:** Small, server only. **Opinion:** Yes; it is the cheapest fidelity win in this audit.

### F-20.5 Refresh-rate (and optionally resolution) matching on Android TV and Fire TV

- **Who:** jellyfin-androidtv (three modes), Wholphin (rate and resolution, with the wait), Plezy (plus a configurable switch delay).
- **Value:** Removes 24p judder on the platform most affected.
- **Size:** Small to medium; needs a setting because some televisions take seconds to switch. **Opinion:** Yes, default on for rate, off for resolution.

### F-20.6 "If something looks or sounds wrong" overrides

- **What:** Device-local switches that edit the published capabilities: Dolby Vision off, HDR off, surround passthrough off, prefer conversion. See A-20.8.
- **Who:** jellyfin-androidtv (about fifteen), Swiftfin, Plezy.
- **Size:** Small. **Opinion:** Yes, a deliberately short list.

### F-20.7 Known-defect device table

- **What:** Per-model corrections (Fire TV 4K family with Dolby Vision + HDR10+; Shield profile 7). See A-20.15.
- **Size:** Small. **Opinion:** Yes before Fire TV ships.

### F-20.8 Dolby Vision profile 7 as real Dolby Vision

- **What:** Rewrite profile 7 to 8.1 during the HLS remux (bitstream filter, no re-encode), and strip HDR10+ or Dolby Vision data where a device needs one or the other.
- **Who:** Plezy does it on the device; Jellyfin's server strips.
- **Size:** Medium, server side (domain 02). **Opinion:** Worth it for Apple TV owners with UHD remuxes; after the basics.

### F-20.9 Scrub thumbnails, chapter marks and a chapter list in the native player

- See A-20.23. **Size:** Medium. **Opinion:** Yes; it is declared already.

### F-20.10 Handheld gestures and touch lock

- Double-tap to skip, vertical swipes for brightness and volume, pinch to fill, long-press for temporary 2×, a lock button against accidental touches. See A-20.25. **Size:** Small to medium. **Opinion:** Yes for double-tap and pinch; the rest optional.

### F-20.11 Zoom and aspect modes

- Fit / fill / stretch. See A-20.26. **Size:** Small. **Opinion:** Yes.

### F-20.12 Subtitle appearance and system caption style

- Size, colour, outline, background, vertical position, HDR-aware brightness; default from the system. See A-20.21. **Size:** Small. **Opinion:** Yes (accessibility).

### F-20.13 Audio delay

- **What:** A per-device audio offset (Bluetooth speakers and some soundbars lag).
- **Who:** Plezy, mpv and VLC clients. ExoPlayer has no public audio-offset control; AVPlayer has none. With platform players this is realistically a server-side offset on converted streams or not available.
- **Size:** Medium, and partial. **Opinion:** Skip unless a second engine arrives.

### F-20.14 Dialogue boost / night mode / loudness normalisation for video

- See A-20.12. **Size:** Small on Android (session effect); server-side on Apple. **Opinion:** Nice to have.

### F-20.15 Hold-to-seek acceleration on television remotes

- See A-20.24. **Size:** Small. **Opinion:** Yes.

### F-20.16 Rewind a few seconds on resume

- **Who:** Swiftfin (`resumeOffset`), jellyfin-androidtv (`resumeSubtractDuration`), Plezy (`rewindOnResume`), Wholphin.
- **Value:** Picking a film up after a day benefits from a few seconds of context.
- **Size:** Small; could be a server preference applied to the resume position so every client gets it. **Opinion:** Yes, as a profile preference.

### F-20.17 Pause when the app leaves the foreground, and video controls in the Android notification

- See A-20.30, B-20.4. A lifecycle hook is small; a `MediaSessionService` for video is medium. **Opinion:** The hook is required; the service is optional.

### F-20.18 External player hand-off

- **What:** "Open in VLC / Infuse / mpv / Just Player" with the direct-play URL and subtitles, and progress synced back where the player reports it.
- **Who:** jellyfin-androidtv, Wholphin, Plezy (Android intents with progress sync back), jellyfin-android.
- **Value:** The cheapest possible answer to "my file needs a universal engine": let the viewer use one they already have. Portico's capability URLs need no headers, which suits hand-off.
- **Conflict:** The session's progress and lease are held by another app; needs a "detached" session kind or a long-lived download-style grant.
- **Size:** Medium. **Opinion:** A pragmatic alternative to F-20.1 for power users, especially on Apple (Infuse and VLC accept URL schemes).

### F-20.19 Real engine-side audio track switching on direct play

- See A-20.20. **Size:** Small to medium. **Opinion:** Yes on Android.

### F-20.20 Engine watchdogs

- **What:** Detect "playing but no frames after resume", "buffering with a full buffer", "past the end but never ended" and recover (small seek, reload, or declare ended).
- **Who:** Plezy (`ResumeStallPolicy.kt`, `BufferingStallPolicy.kt`, `EndOfStreamPolicy.kt`), each tied to a named chipset (Amlogic Mi Box, MediaTek Fire TV 4K Max).
- **Value:** These are failures the engine never reports as errors, so Portico's route-failure mechanism cannot see them. The "never ends" case matters most to Portico: no `ended` fact means no post-play, no next episode and no watched mark.
- **Size:** Small each. **Opinion:** Add the end-of-stream one before Fire TV ships; the others when a device shows the symptom.

### F-20.21 Stall-triggered quality suggestion

- See A-20.27. **Size:** Small. **Opinion:** Yes.

### F-20.22 Playback statistics on native

- Dropped frames, decoder name (hardware or software), buffer length, measured bitrate. `Backlog.md` already lists dropped frames and buffer for Apple. On Android `DecoderCounters` and `AnalyticsListener` give them directly; on Apple the item's access log does. **Size:** Small. **Opinion:** Yes; it is the first thing support will ask for.

### F-20.23 Sleep timer "until the end of this episode"

- **Who:** Plezy. **Size:** Small. **Opinion:** Nice to have.

### F-20.24 Tunneled playback and software-decoder choices on Android TV

- **Who:** Plezy (tunneling switch, default off, with a watchdog for the bug it triggers), jellyfin-androidtv (software codecs switch).
- **Opinion:** Not now; tunneling causes as many problems as it solves.

### F-20.25 Wider speed range and per-title speed memory

- See A-20.33. **Size:** Small. **Opinion:** Low priority.

### F-20.26 An owner-visible view of route failures and device capabilities

- **What:** In the console: what each device declared (with evidence), and which files it reported as failing which route, with a clear button.
- **Who:** jellyfin-androidtv has an in-app "device profile report" (`deviceProfileReport.kt`).
- **Size:** Small to medium. **Opinion:** Yes; it makes A-20.7 debuggable.

## Possible bugs and risks in Portico

None of these is in `Backlog.md` unless stated. "Confirmed by reading" means the code path was traced to a definite outcome; whether a device then misbehaves in the way described is a runtime matter and is said so.

### B-20.1 Audio a receiver could take as a bitstream is refused and converted, on every device that publishes v1 capabilities

- **What goes wrong:** On an Android TV or Fire TV connected to a receiver or soundbar, the probe marks AC-3, E-AC-3, DTS and TrueHD as `passthrough: true`. The planner refuses a passthrough codec unless the audio route is `hdmi`, `arc` or `optical`. The v1 capability document has no route field, so after conversion the route is empty and **every passthrough codec is refused**; the audio is converted to AAC. The same applies to the Vega app, whose declared table marks AC-3 and E-AC-3 as passthrough.
- **Evidence:** `PorticoCapabilities.kt:240-258` builds the list; `client-core/client-profile.ts:499-503` sets `passthrough: true` and overwrites the decode entry with `Object.assign`; `client-core/playback-v1/capability-profile.ts:49-55` carries `passthrough` but no output route; `server/internal/playbackv1/capabilities.go:267-277` builds `AudioOutput` with only `MaxChannels`; `server/internal/playback/client_profile.go:466-470` makes that profile the planner's; `server/internal/playback/delivery_decision.go:215-217` rejects when `!passthroughRoute(client.AudioOutput.Route)`; `:374-376` defines the accepted routes. The test matrix covers this rule only with client-profile documents that carry `route: "hdmi"` (`declared_profiles_test.go`, "shield_avr"), never through the v1 conversion.
- **Effect:** Dolby Digital 5.1 arrives as AAC 5.1 and is decoded to PCM on the device. A receiver takes multichannel PCM; a soundbar on ordinary ARC takes only stereo PCM, so surround collapses to stereo there. Atmos is lost everywhere. Perversely, a television with no receiver (no passthrough entries) keeps its decoder entries and may direct-play the same track.
- **How the comparison surfaced it:** jellyfin-androidtv's per-codec bitstream settings prompted the question "what does Portico's planner do with a passthrough flag".
- **Confidence:** Confirmed by reading (planner logic). Not run on a device with a receiver.
- **Severity:** High for Android TV / Fire TV / Vega with external audio.

### B-20.2 HDR titles are tone-mapped on the server for Apple devices whose screen is SDR at that moment

- **What goes wrong:** The client profile deliberately lists HDR10 and HLG on the HEVC decoder even when the screen is SDR, because AVPlayer tone-maps (COMPAT-02). The v1 document has a `toneMapsToSdr` field for exactly this; no client sets it. The server's conversion keeps an HDR range only if the display shows it or `toneMapsToSdr` is true, so the range is dropped and the planner rejects direct and copy routes with `dynamic_range_unsupported`, leaving a tone-mapping transcode.
- **When:** iPhone SE, base iPad and iPad Air (SDR panels); Apple TV HD; Apple TV 4K on an SDR television or with HDR switched off; and any iPhone in Low Power Mode, because `AVPlayer.eligibleForHDRPlayback` turns false there and the app republishes on that notification (`PorticoCapabilities.m:19-24, 59`; `client-profile.ts:446-451`).
- **Evidence:** `client-core/client-profile.ts:452-456`; `client-core/playback-v1/capability-profile.ts:38-48` (no `toneMapsToSdr`); grep finds `toneMapsToSdr` in no client code; `server/internal/playbackv1/capabilities.go:250-256`.
- **Effect:** A 4K HDR film that could be direct-played or remuxed becomes the most expensive conversion the server has. On a weak server it will not keep up.
- **Confidence:** Confirmed by reading. **Severity:** High for the weak-server target.

### B-20.3 App-drawn subtitles are refused when the server is reached over plain HTTP

- **What goes wrong:** Both native subtitle fetchers accept the cue document's address only if it is `https`, or `http` on the device's own loopback. A server on the home network at `http://192.168.x.x:port` (the configuration `network_security_config.xml` and `NSAllowsLocalNetworking` exist to allow) fails the check; the selection is invalidated with "The subtitle address does not belong to this playback session." JavaScript discards that status (`usePlayer.ts:180`: `if (f.type === 'subtitleStatus') return;`), so the viewer simply sees no subtitles. On Apple the early return also skips native selection of embedded tracks.
- **Evidence:** `native/Native/PorticoSubtitleSelection.m:68-71`; `PorticoSubtitleSelection.kt:104-111`. The line after it requires the subtitle scheme to equal the media scheme, which suggests plain HTTP was meant to work when the media is plain HTTP.
- **Why testing missed it:** The Android port reached its dev server through `adb reverse` as `127.0.0.1` (port log, "Differences from the Apple app"), which is loopback; the demo server is HTTPS. The Vega fetcher has no such rule.
- **How the comparison surfaced it:** Asking how each client fetches sidecar subtitles and with what credentials.
- **Confidence:** Confirmed by reading. **Severity:** High where a server is used without a certificate, which Portico's principles call first-class; none for claimed servers on HTTPS.

### B-20.4 On Android a video keeps playing after the app is left when Picture in Picture does not take over

- **What goes wrong:** Nothing pauses ExoPlayer when the activity stops. Picture in Picture is not offered on televisions (`PorticoPlayerView.kt:202-204`), so pressing Home on an Android TV or Fire TV during a film leaves the sound playing behind the launcher, with a wake lock held (`PorticoPlaybackController.kt:387`) and the session reporting progress. The same happens on a phone when the screen is switched off, or when the user has disabled Picture in Picture for the app. `setHandleAudioBecomingNoisy(false)` and the absence of a media notification mean there is no visible way to stop it except reopening the app.
- **Evidence:** `MainActivity.kt` overrides only `onPictureInPictureModeChanged`, `onUserLeaveHint`, key dispatch and `onNewIntent`; no `LifecycleEventListener` or `onHostPause` exists in the player modules (grep). The port log states "a video in the background is either in Picture in Picture … or paused", but no code pauses it.
- **How the comparison surfaced it:** Wholphin's `PlaybackLifecycleObserver` and jellyfin-android's `PlayerLifecycleObserver`.
- **Confidence:** Probable (the absence of a pause is confirmed; that ExoPlayer continues audio after `onStop` is its documented behaviour but was not run). **Severity:** High on Android TV and Fire TV.

### B-20.5 On Apple TV the display switches mode after playback has started

- **What goes wrong:** The display criteria are applied when the layer reports its first frame ready (`PorticoPlayer.m:55-60`), by which time the controller has already announced `ready` and applied the play intent. The television then renegotiates HDMI (typically one to several seconds of black) while the film runs. There is no wait on `isDisplayModeSwitchInProgress`, and the criteria are loaded asynchronously after that, adding delay.
- **Evidence:** as cited; `PorticoPlaybackController.m:403` (status handler plays as soon as the item is ready). Compare `plezy/ios/Runner/MpvPlayer/MpvPlayerCore.swift:440-560` (holds playback until the switch ends, with measured notes on when the progress flag can be trusted) and `swiftfin/…/MediaPlayerProxy+AVPlayer.swift:476-478, 546-562` (criteria set before the clock starts).
- **Effect:** With tvOS "Match Content" on, the opening seconds of anything whose frame rate or dynamic range differs from the home screen's are lost to a black screen; with a resume, the viewer misses the moment they resumed at.
- **Confidence:** Probable (ordering confirmed by reading; the visible effect depends on the television). **Severity:** Medium.

### B-20.6 Ordinary network and server errors are reported as engine failures, which downgrades the file for a week

- **What goes wrong:** Only a short list counts as "network": on Android two error codes (connection failed, connection timeout, `PorticoPlaybackController.kt:1038-1039`); on Apple six `NSURLError` codes (`PorticoPlaybackController.m:412`). Everything else is an engine failure, default class `engine_error`. That includes a bad HTTP status from the server (a busy 503, a 404 for a segment not yet produced, a 403/410 after a grant lapsed), an unspecified I/O error, a TLS failure, "data not allowed" on cellular, and on Android falling behind the live window. JavaScript reports any engine failure to the server (`usePlayer.ts:205-210`), and the server closes the route that was in use for that file on that sign-in for seven days (`route_failures.go:23, 79-88`), so the next plays of that file go through a remux or a conversion although nothing was wrong with the file or the decoder.
- **Also:** For Live TV on Android, `ERROR_CODE_BEHIND_LIVE_WINDOW` (the viewer paused long enough to fall out of the buffer) needs a re-prepare at the live edge; here it becomes an error. Channels are excluded from escalation (`client-core/index.ts:1456`), so the viewer gets "This media could not be played".
- **How the comparison surfaced it:** jellyfin-androidtv forgets its retry state after 30 seconds of healthy playback; Plezy separates "a format that already rendered a frame is supported, so a later decoder failure is transient" (`VideoDecoderRecoveryPolicy.kt`).
- **Confidence:** Confirmed by reading (classification and consequence). How often real errors fall in the gap needs runtime observation.
- **Severity:** Medium. Suggested rule: treat every I/O-class error as network; count a failure as an engine failure only before the first frame or when the code is a decoder or parser code; never escalate after healthy playback of the same item.

### B-20.7 When the system pauses the player, the app still believes it is playing

- **What goes wrong:** Facts never change the intent (`client-core/index.ts:1401-1445`), and the coordinator sends a command only when the desired state changes (`NativePlaybackCoordinator.ts:195-196`). If the operating system pauses the engine outside the paths Portico listens to, nothing re-applies "play" and nothing flips the intent to "paused". The clearest case: iOS and tvOS pause an `AVPlayer` that is showing video when the app goes to the background without Picture in Picture or AirPlay (Home on Apple TV; a phone with automatic Picture in Picture turned off). On return the picture is frozen, the button shows Pause, the controls auto-hide because the app thinks it is playing, and it takes two presses (pause, then play) to continue.
- **Evidence:** as cited; `PorticoPlaybackController.m:387-397` (`applyIntent` runs only on a command, a status change or a seek); `VideoPlayer.tsx:119, 453`.
- **Confidence:** Probable (the logic is confirmed; the system pause is standard AVFoundation behaviour, not reproduced here). **Severity:** Medium.

### B-20.8 Android declares its MPEG-2 decoder under a name the planner never matches; VC-1 is not declared at all

- **What goes wrong:** The probe maps `video/mpeg2` to `"mpeg2"` (`PorticoCapabilities.kt:302`). The planner looks a source codec up by FFmpeg's name, which is `mpeg2video` (`delivery_decision.go:162, 299`; the declared webOS table uses `mpeg2video`). Nothing translates between them (`capabilities.go:239` only lower-cases). `video/wvc1` is absent from the probe's table.
- **Effect:** DVD rips, over-the-air recordings and Live TV in MPEG-2 are always converted for Android clients, including set-top boxes whose chips decode MPEG-2 in hardware. (They would also be converted today for being interlaced; both need fixing for Live TV to be cheap.)
- **Note:** The v1 spec's own vocabulary is `mpeg2|vc1` (§3), so this is a mismatch between the spec's names and the planner's, not only an Android typo.
- **Confidence:** Confirmed by reading. **Severity:** Medium (Live TV and DVR on Android TV).

### B-20.9 An MP4 whose HEVC track is tagged `hev1` (or Dolby Vision `dvhe`) is offered to AVPlayer as a direct play

- **What goes wrong:** AVFoundation plays HEVC in MP4/MOV only with the `hvc1` sample entry (Dolby Vision: `dvh1`). The planner checks the sample entry only to keep Dolby Vision entries away from non-DV devices (`delivery_decision.go:200-204`); a `hev1` file passes as an ordinary direct play for Apple.
- **Effect:** Typically audio with no picture, or a failed item. If AVPlayer reports a failure, route-failure escalation recovers by remuxing (which retags). If it plays sound over black without an error, nothing recovers.
- **How the comparison surfaced it:** Swiftfin's AVPlayer profile makes the tag a *required* condition (`VideoPlayerType+AVPlayer.swift`, `videoCodecTag` in `hvc1`, `dvh1`).
- **Confidence:** Probable (the missing check is confirmed; AVPlayer's behaviour with `hev1` is well known but was not reproduced). **Severity:** Medium; `hev1` is what FFmpeg writes by default, so such files are common.

### B-20.10 Android reads "any channel count" as stereo

- **What goes wrong:** `AudioDeviceInfo.getChannelCounts()` returns an empty array to mean the device accepts arbitrary channel counts. `output()` turns empty into 0 and then clamps to 2 (`PorticoCapabilities.kt:264-266`). The passthrough function handles the same empty case correctly (`:244-248`).
- **Effect today:** None, because the v1 conversion discards the output channel count (A-20.5). It becomes live the moment that is fixed: surround conversions would come out stereo on such devices.
- **Confidence:** Confirmed by reading. **Severity:** Low now; latent.

### B-20.11 Android declares that it renders ASS, SSA and PGS; it renders none of them

- **What goes wrong:** The Android table says `styled: ['ass','ssa'], bitmap: ['pgs']` with the comment "ExoPlayer draws SubRip, WebVTT and SSA/ASS text itself, and PGS through its bitmap decoder" (`client-core/client-profile.ts:162-163`), but the app disables ExoPlayer's text tracks and draws only the plain-text cue document (`PorticoPlaybackController.kt:394`; port log: "Subtitles are always drawn by the app"). In addition the v1 builder emits bitmap formats with `render: 'native'` while the server files a bitmap format only when `render` is `client` (`capability-profile.ts:59`; `capabilities.go:302-313`).
- **Effect today:** None: the planner never reads the subtitle capability lists (grep: assigned, validated, never consulted); burn-in is decided by the track's own type. It is a trap for whoever implements the spec's "render on the client" precedence: Android would stop getting burn-in and show nothing.
- **Confidence:** Confirmed by reading. **Severity:** Low now; latent High.

### B-20.12 App-drawn subtitles ignore both Portico's appearance settings and the system's caption style

- **What goes wrong:** The settings structure offers subtitle size and background on the web only, with the comment "Apple's system player styles them from the device's Accessibility settings" (`settings-structure.ts:205`). On Apple most subtitles are not drawn by the system player; they are a `UILabel` with a fixed style (`PorticoPlayer.m:17, 54`), which does not read `MACaptionAppearance`. Android's `TextView` likewise ignores `CaptioningManager`.
- **Effect:** A viewer who has set large captions in Accessibility gets Portico's fixed size; on Apple TV that is at most 40 points.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (accessibility).

### B-20.13 One failed re-check blanks subtitles the device already has, silently

- **What goes wrong:** Every 25 seconds the cue document is re-requested. On any failure of that request (a timeout on a busy server, a Wi-Fi blip) the cues are discarded at once, and they return only when a later request succeeds (`PorticoSubtitleSelection.m:117`; `.kt:184-190`). Video keeps playing from its buffer meanwhile. The status message is dropped in JavaScript (B-20.3).
- **Confidence:** Confirmed by reading. **Severity:** Medium-Low. See A-20.18 for the simpler design.

### B-20.14 A device with only a software decoder for a codec declares that codec at the software decoder's limits

- **What goes wrong:** The probe prefers a hardware decoder but falls back to a software one and reports `hardware: false` (`PorticoCapabilities.kt:130-138, 158`). `androidProfile` never reads that flag (`client-core/client-profile.ts:495`). Android ships a software AV1 decoder on every recent device and software HEVC on most; their advertised maximum sizes are far beyond what a phone or stick decodes in real time.
- **Effect:** A 4K AV1 or HEVC file is direct-played to a device that will drop most frames. Dropped frames are not an error, so nothing escalates.
- **How the comparison surfaced it:** Plezy: "the probe answers 'can we decode it without dropping frames', not 'can we decode it'" (`video_decode_capabilities.dart`); jellyfin-androidtv has a software-codecs switch; Portico's own Apple merge caps software HEVC at 1080p (`client-profile.ts:442-445`).
- **Confidence:** Confirmed by reading (flag unused); effect Probable. **Severity:** Medium on low-end Android and older Fire TV.

### B-20.15 The device claims trick-play and chapters; the native player has neither

- **What goes wrong:** `features: {trickplay: true, chapters: true}` is published (`native/src/bridge/audio-decode.ts:31, 70`), but `Timeline.tsx` draws no thumbnails or chapter marks, and `VideoPlayer.tsx:245-263` passes markers but never `chapters` to the system media module, so the lock-screen "next/previous steps through chapters" code (`PorticoSystemMedia.m:285-288, 318-329`) can never run for video.
- **Confidence:** Confirmed by reading. **Severity:** Low (a claim the server may act on later; a dead native feature).

### B-20.16 The Next and Previous buttons in the video player call the music owner and do nothing for video

- **What goes wrong:** The transport's Next and Previous, the mini player's, and the lock-screen/remote "next track" command all call `service.systemListeningAction(...)` (`VideoPlayer.tsx:274-275, 451, 455`), which is `this.nativeV1?.advance(action) ?? Promise.resolve()` (`client-core/index.ts:827`). `nativeV1` exists only while the native listening owner plays a song or audiobook (`native-listening-kinds.ts:9`). For an episode it is undefined, so the call resolves without doing anything, while the buttons are enabled whenever the queue has a next or previous entry.
- **Workaround in the UI:** Up next → Play now, and the post-play card, use other paths that work.
- **Confidence:** Probable (traced in client-core and the player; I did not find another handler, but did not run it). **Severity:** Medium.

### B-20.17 The profile's default video speed is offered in Settings but never applied by the native player

- **Evidence:** `settings-structure.ts:185-186` shows `playback.defaultSpeed` on every client that has speed control; the web player reads it (`web/src/player/Player.tsx:147, 180`); no file under `native/src` reads it (grep), and `NativePlaybackSpeed.prefer` has no caller.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-20.18 A slow answer to the pre-play Dolby Vision check delays every video start

- **What goes wrong:** `play` waits for `GET …/playback-options` before starting (`usePlayer.ts:399-409`). It fails open on an error, but there is no short timeout of its own, so on a busy or distant server the start waits up to the transport's 15 seconds before the session is even requested.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (start-up time on weak servers).

### B-20.19 A file the engine cannot seek exactly turns every seek, including resume, into a failure

- **What goes wrong:** A seek counts only if the engine lands within 0.5 s of the target; otherwise the seek fails and the intent becomes paused (`PorticoPlaybackController.kt:536-555`, `.m:333-347`; `client-core/index.ts:1373-1379`). ExoPlayer reports Matroska files without a cue index, and some transport streams, as unseekable and lands at the start. Portico uses the default extractors with no constant-bitrate seeking or cue repair.
- **How the comparison surfaced it:** `plezy/…/CuelessSeekExtractorWrapper.kt`; `jellyfin-androidtv/…/ExoPlayerBackend.kt:82-89`.
- **Confidence:** Speculative (needs such a file on Android). **Severity:** Low to Medium; affects direct play only, and only on Android.

### B-20.20 Passthrough is declared from every HDMI device even when sound is going elsewhere, and replaces the decode capability

- **What goes wrong:** `passthrough()` scans all digital outputs regardless of the active route (`PorticoCapabilities.kt:243`), and the merge overwrites a decodable codec's entry with the passthrough one (`client-profile.ts:499-503`). With Bluetooth headphones on a television that also has a receiver attached, AC-3 the device could decode is treated as passthrough-only and (correctly) refused for Bluetooth, so it is converted although it could have been decoded locally.
- **Confidence:** Confirmed by reading. **Severity:** Low (and masked by B-20.1 today).

### B-20.21 After a music crossfade, the surviving ExoPlayer is the app's video player too

- **What goes wrong:** A crossfade builds a second player without the low-memory load control, the screen-size cap or the movie audio attributes, then makes it *the* player (`PorticoPlaybackController.kt:671-689, 713-725`). The next film plays on it.
- **Confidence:** Confirmed by reading. **Severity:** Low (wrong buffer limits on low-memory devices).

### B-20.22 The video audio session is never released on Apple

- **What goes wrong:** Each video load activates the session (`PorticoPlaybackController.m:428-438`); releasing the lease does not deactivate it (only the listening owner does, `PorticoListeningOwner.m:959`). Music from another app that Portico interrupted is not told it may resume when the film is closed.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-20.23 Observer removal relies on the player's current item rather than the item observed (latent)

- **What:** `removeObservation` calls `removeObserver` on `_player.currentItem` (`PorticoPlaybackController.m:441`). It is correct only because every path removes observers before replacing the item. If the item ever changes without passing through that path, the result is an Objective-C exception, which terminates the app. Holding the observed item in a variable would make it safe.
- **Confidence:** Speculative (no failing path found). **Severity:** Low.

## Where Portico is stronger

- **The lease, command and fact protocol** (A-20.3). No comparison client has an equivalent guarantee against stale events; Plezy solved one instance of the problem with a purpose-built object. It is also the right seam for any future engine.
- **Self-healing delivery** (A-20.7). An engine failure silently becomes the next route, remembered per file. Everyone else either shows an error or forgets at the end of the play. (It needs the classification fix in B-20.6 to be safe.)
- **Capability evidence and a stored, inspectable device profile** (A-20.4, A-20.6). Good for support and for the "plays as original / converts audio" preview before play.
- **Dolby Vision honesty** (A-20.14): the pre-play approximate-colours warning, the fallback-layer logic by compatibility id, and never sending a Dolby Vision sample entry to a device that would refuse it. None of the open clients warns before a lossy Dolby Vision conversion.
- **Burn-in confirmation** for 4K/HDR sources, and preferring a text track over an image track of the same language (`PlayerOptions.tsx:228-237`).
- **Server-authorised auto-skip and server post-play policy** (A-20.34): identical behaviour on every device, with skip analytics.
- **Interruptions resume only what they paused** (`VideoPlayer.tsx:269-270`), and skip-forward on a headset inside an intro jumps to its end (`PorticoSystemMedia.m:308-316`).
- **Siri Remote surface scrubbing** rebuilt for a custom chrome (`PorticoSiriRemote.m`); most React Native tvOS apps do not have it.
- **Honest Picture in Picture and AirPlay**: the app knows when its subtitles will not travel and can say so (`subtitlesCarried`).
- **Audio rendition mapping by name with a one-to-one check** (`PorticoAudioSelection`), rather than by index; an index mismatch between server and player is a classic wrong-language bug in other clients. Swiftfin needs an index map for the same reason.
- **Security posture of the native player**: media and subtitle addresses are validated against the session's own grant; redirects are refused everywhere a credential or grant travels; local files are accepted only from the app's Downloads folder after resolving symlinks; artwork fetches are bounded and never follow redirects.
- **System and battery behaviour**: hardware decoding only, system HDR and Dolby Vision on Apple, AirPlay, a small binary, no codec build to maintain (A-20.41).
- **Network-versus-engine recovery with a visible "Reconnecting…" state** and a 90-second give-up, rather than an immediate error (`usePlayer.ts:134-194`).
- **One player chrome on every platform** with an enforced layout rule.

## Doc/code disagreements

1. **Spec §5.4 versus the planner (audio conversion target).** The spec says DTS/TrueHD to Apple TV become E-AC-3 5.1 (7.1 when the route supports it) and multichannel PCM when local. The code converts to AAC, 5.1 at 384 kbps or stereo, and uses E-AC-3/AC-3 only for passthrough-only devices (`delivery_decision.go:347-372`).
2. **Spec §3 says the capability document is sent when the "audio route/receiver" changes and that the server "uses it for every decision".** The document has no field for the route or the output channel count, so the planner cannot use them (A-20.5, B-20.1).
3. **Spec §5.6 `toneMapsToSdr`** exists in the schema and the server; no client sends it (B-20.2).
4. **Spec §3 codec names `mpeg2`, `vc1`** versus the planner's FFmpeg names (`mpeg2video`) (B-20.8).
5. **Spec §5.5 subtitle precedence** (render on the client when the profile says so; PGS when `client`) is not what the server does: the subtitle capability lists are stored and never read; burn-in follows the track's type (`playbackv1/subtitles.go:27-34`). And the client emits bitmap formats as `render: "native"` where the server would only accept `client` (B-20.11).
6. **`client-core/client-profile.ts:162`** says ExoPlayer draws SubRip, WebVTT, SSA/ASS and PGS itself; the Android app disables ExoPlayer's text renderer (B-20.11).
7. **`client-profile.ts:139, 164` `embeddedAudioSwitching: true`** for Apple and Android; neither app switches an embedded track in the engine, and the v1 path reports the feature as absent (A-20.20).
8. **`settings-structure.ts:205`** says Apple's system player styles subtitles from Accessibility settings; most subtitles on Apple are drawn by the app's own label (B-20.12).
9. **Capability features `trickplay: true, chapters: true`** versus a native player with neither (B-20.15).
10. **Android port log, Phase 2: "a video in the background is either in Picture in Picture … or paused".** Nothing pauses it (B-20.4).
11. **`06 Clients.md` lists the Android app's Media3 player and says "subtitles drawn by the app"**, which matches the code; `04 Playback and Media.md` says "text subtitles are delivered as WebVTT renditions where the client supports them", but the native clients take a JSON cue document, not WebVTT (the v1 response labels it `format: "webvtt"`, `playbackv1/subtitles.go:76`).
12. **Two capability publishers** (`native/src/app/device.tsx:48` and `useNativePlayback.ts:97`) send two documents; `06 Clients.md` and the spec describe one.
13. **`06 Clients.md`: "More holds Playback info …; on the web dropped frames and buffer".** Consistent with the code and with `Backlog.md`; noted only because the native player has the data available and does not forward it (F-20.22).
14. **Spec §5.2 client-side "Auto" quality from the client's bandwidth estimate.** The native apps do not estimate bandwidth; "Automatic" is the server's plan (A-20.27).

## Open questions for Justin

1. **The central fork.** Is "platform players only" a principle, or a starting point? Would you accept an optional second engine on Apple TV if it removed most server conversions, at the cost of 15–30 MB, a codec build to maintain and a licence review? (A-20.1, F-20.1)
2. **Short of that:** do you want the two Android additions that every Jellyfin Android client ships, FFmpeg audio decoding (F-20.2) and on-device PGS/ASS (F-20.3)? Each adds a native library and makes Android differ from Apple.
3. **Image subtitles on Apple.** Would a server-rasterised image-subtitle track drawn by the app (no video transcode) be acceptable as "one renderer", or must image subtitles stay burned in until there is a second engine? (A-20.19)
4. **Audio conversion target.** FLAC (lossless, larger) or high-bitrate E-AC-3 (keeps Atmos-less 7.1 compactly, passes to receivers on Apple TV), or keep AAC? (A-20.9, F-20.4)
5. **One capability document or two?** Extend v1 with the audio output and tone-mapping facts, or let the planner use the client profile? (A-20.5)
6. **User overrides.** Is a short "if something looks or sounds wrong" list on the device acceptable, given the no-knobs preference? (A-20.8, F-20.6)
7. **Subtitle re-authorisation.** Is live revocation of on-screen subtitles a requirement, or can the 25-second re-check go? (A-20.18)
8. **Plain HTTP and subtitles.** Was "HTTPS or loopback only" for the subtitle document intended (then plain-HTTP servers have no subtitles on native), or a carry-over from the stricter profile transport? (B-20.3)
9. **Android TV when Home is pressed:** pause, or stop the session? And should a phone with the screen off keep a film's sound going? (B-20.4)
10. **Refresh-rate matching on Android TV:** default on? Include resolution switching? (F-20.5)
11. **Route-failure memory:** seven days per file per sign-in. Should it be visible and clearable by the owner, and should failures after healthy playback count at all? (A-20.7, B-20.6)
12. **Interlaced video.** Should Android TV (whose decoders deinterlace) and Apple TV declare `interlaced`, so Live TV and DVR can be remuxed instead of converted? This needs a device check per platform.
13. **External player hand-off** as the pragmatic answer for power users on Apple (Infuse, VLC) and Android: wanted, or contrary to the product? (F-20.18)
14. **Seeking:** keep every seek exact, or allow keyframe seeks while skipping repeatedly and scrubbing? (A-20.22)
15. **The Vega HLS loader:** keep the hand-written one, or move to Amazon's Shaka route before release? (A-20.39)

## Evidence boundary

- **Nothing was run.** No build, simulator, emulator, device, television, receiver or server. Every statement about what AVPlayer, ExoPlayer, tvOS or Android *does* in response to Portico's code rests on documented platform behaviour and on how other projects work around it, not on observation.
- **Device-dependent claims that most need a hardware check:** B-20.1 (what a real receiver's `AudioDeviceInfo` reports and what the planner then chooses), B-20.4 (sound after Home on Android TV), B-20.5 (how long a given television blanks), B-20.7 (pause on background on iOS and tvOS), B-20.9 (`hev1` in AVPlayer), B-20.14 (software AV1/HEVC frame drops), B-20.19 (unseekable files).
- **Server behaviour was read only where it consumes client declarations.** How HLS is produced, how the timeline of a converted stream relates to player time, whether Dolby Vision data is rewritten or stripped during a remux, and the full subtitle pipeline belong to domains 02, 04 and 17. In particular I did not establish whether an embedded ASS track is offered as plain text or as burn-in by default.
- **Subtitle timing against player time** (the cue document is "source-relative"; AVPlayer and ExoPlayer report time in the stream's own clock) was not verified for remuxed or converted streams that do not start at zero. If the server's HLS timestamps are not zero-based, app-drawn subtitles would be offset on one platform and not the other; this needs a runtime check with a file whose first video timestamp is not zero.
- **Music and audiobooks** share the Android player object and the Apple controller; only the parts that touch video were reviewed (B-20.21).
- **Plezy's Dart player screens, Swiftfin's VLC proxy, and Plezy's 4,400-line `ExoPlayerCore.kt`** were not read in full; claims about those products come from their settings definitions, README, device profiles and policy files.
- **Infuse** claims come from forum posts by Firecore staff and users between 2021 and 2026, not from an official format table.
- **Licence remarks** (A-20.40) are an engineer's reading, not legal advice.
- **Performance** (bridge traffic from half-second facts, checkpoint rate at a couple of hundred concurrent players, start-up time) was not measured.

## Outside my domain

- **Plain HTTP to a remote hostname is blocked on Apple.** Both `Info.plist` files set `NSAllowsArbitraryLoads` false with `NSAllowsLocalNetworking` true (`native/ios/Portico/Info.plist:42-48`, same in `tvos`). That permits plain HTTP to IP literals, unqualified names and `.local`, but not to `http://my-home.example.com:8096`. If "remote access is port forwarding" and "plain HTTP stays first-class" are both meant to hold for a dynamic-DNS name without a certificate, AVPlayer and the app's transport will refuse it on iPhone, iPad and Apple TV. (Networking domain.)
- **The profile transport refuses plain HTTP except to loopback** on both platforms (`PorticoPlayer.m:249`; Android port log). Same theme as B-20.3. (Identity domain.)
- **No Dolby Vision bitstream filter was found** in the server's HLS arguments for the profile 7 fallback (the enhancement layer and Dolby Vision data appear to be copied through and merely not signalled). Whether AVPlayer and ExoPlayer tolerate that needs checking. (Domain 02.)
- **A second, older download playback path** using `AVPlayerViewController` remains in `PorticoDownloads.m:451-470` beside the shared-controller path. (Domain 14.)
- **Live TV in the United States is MPEG-2, interlaced.** With B-20.8 and the interlaced rule, every native client forces a full conversion for every over-the-air channel. (Live TV domain.)
- **Timeline checkpoints every three seconds of progress** per player (`client-core/index.ts:1438-1443`): worth a look for the "couple of hundred concurrent browsers" target. (Domain 02.)
