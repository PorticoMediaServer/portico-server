# Lane: android-mpv — mpv as a second player engine on Android, Android TV and Fire TV

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/native-android-mpv` (branch `audit-implementation/android-mpv`, repository portico-react-native). You can commit. Nothing can be built here: no Gradle. Write complete, careful Kotlin and leave exact build and device test notes. Your report is `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/android-mpv.md`.

Read first, in this order:
1. `apps/native/Native/ENGINE-CONTRACT.md` in your worktree: the exact contract between JavaScript and a player engine (every command, event and field). The Apple mpv engine was just written against it (`apps/native/Native/PorticoMpv*.swift`); read it as a worked example of the same job on another platform.
2. `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/apple-mpv.md`: what that lane built, the decisions it made, the JavaScript engine selection (`apps/native/src/player/`, `apps/native/src/bridge/player-engines`), and what it left unfinished.
3. The existing Android engine: `apps/native/android/app/src/main/java/tv/getportico/portico/native/PorticoPlayerView.kt` and its neighbours (ExoPlayer/Media3).
Audit documents: 20 (all), 19; verification V02. References for how others embed mpv on Android (do not copy code): `_reference/plezy`, `_reference/jellyfin-android`, `_reference/wholphin`.

## What to deliver
1. **An mpv engine for Android** (phones, tablets, Android TV, Fire TV) as a second native module and view that implements the engine contract exactly, so the JavaScript player screen drives it with no Android-specific branches. Use libmpv through its client and render APIs on a SurfaceView (or TextureView where needed), hardware decoding through MediaCodec with a software fall back, and libass for styled subtitles. Dependency: a prebuilt LGPL libmpv for Android (the same kind of AAR other open-source Android players publish); wire it into Gradle, pin the version, and say in your notes exactly what is fetched and how to build. Keep the APK's ABI split sensible.
2. It must cover everything the Apple engine covers where Android allows: direct play of MKV and other containers, HEVC and AV1 where the device decodes them, HDR output where the display supports it, multichannel and lossless audio decoding, audio passthrough where Android allows it (declare honestly), embedded and external subtitles (text, ASS, PGS, VobSub) with track switching, audio track switching, speed, seeking, chapters, resume, progress reporting through the same path, audio focus and interruption handling, media session and remote keys (D-pad and media keys on TV), background behaviour that matches the existing engine, and refresh-rate matching where the existing engine has it.
3. **What mpv cannot do on Android is declared, not hidden** (for example Picture in Picture or casting from this engine), through the same capability flags the Apple engine uses, so the player screen hides those controls.
4. **The mpv capability profile for Android**: containers, codecs with limits by device class (use MediaCodecList to find real hardware decoders; software limits by device class as estimates clearly marked), dynamic range, audio, `subtitleRendering` = `text`, `ass`, `pgs`, `vobsub`, `dvb`, published as the second engine profile exactly as the Apple side does.
5. **Engine selection works on Android** through the JavaScript the Apple lane wrote: make `mpvAvailable` and the rest true on Android, with the Player setting (Automatic, Native, mpv) appearing there.
6. Fix the confirmed Android player-engine bugs in document 20 that you meet in this code, including the one where Android declares ASS and PGS rendering the ExoPlayer engine does not have (B-20.11): the native profile must declare only what it renders.
7. Fire OS differences that matter (codec reporting, refresh-rate APIs, passthrough): handle and note them.

## Boundaries
- `packages/*` here are copies and must not be edited. Record needed shared changes under "Needed from others".
- A **downloads** lane is working in the native download manager and screens; an **audio-engines** lane in the music engines. Stay in the video player engine code.
- Do not change the contract. If you find it ambiguous or wrong, implement the most sensible reading, and list the exact correction in your report.
- Allowed checks: `npx tsc --noEmit` in `apps/native`, single test files, and standalone Kotlin type-checking if the tools for it exist here (a previous lane managed a "standalone Kotlin" check; look in `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/native-app.md` for how).
