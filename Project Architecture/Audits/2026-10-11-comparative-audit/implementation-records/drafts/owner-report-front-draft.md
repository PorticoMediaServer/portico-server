# Portico implementation report — 14–15 October 2026

This is the owner's summary of the implementation programme that followed the comparative audit, updated after the 15 October completion pass. Sections 1–6 are the short version. The appendices hold the per-decision detail, every lane's record and all 588 audit bug dispositions.

## 1. Where things stand

- **Every decision you made is built in source, including the client gaps that were still open on 14 October.** The completion pass on 15 October finished them (section 4 lists what is left).
- **Audit bugs:** of the 588, 386 are fixed, 173 were already fixed, 7 were not present and 11 are intentional under your decisions. **11 remain open (7 Medium, 4 Low, none High)**, and almost all of them need real devices, real media or a release to settle (section 4).
- **Independent reviews of the new work found 137 problems, and all of them are fixed.**
- **Everything passes on Ubuntu after the final pass (15 October):**
  - **Server:** @@GO@@.
  - **Shared and web:** @@JS@@.
  - **Native:** @@NATIVE@@.
  - **Builds:** the production web build, Windows and macOS cross-builds, and the Android app with Portico's own media engine.
  - **End to end:** @@E2E@@.
  - **Android emulator:** the app signs in to a local server and plays a film through Portico's own mpv engine.
- **It is not ready to release.** Nothing has been compiled for Apple; the Swift and Objective-C code is unbuilt. Nothing has run on a real phone, TV, Fire TV or Vega device. Nothing has been installed on Windows, macOS or Docker. No real GPU, receiver or tuner has been used.
- **Nothing was pushed, merged into your own branches or deployed.** All work sits on `audit-implementation/main` in each repository (section 6). Hosted was not redeployed, because it needs an empty database (section 3).
@@PERF@@
## 3. Your decisions on 15 October, and things you should know

**Settled on 15 October (built as you asked):**
- **Tuner conflicts:** an owner setting. By default live viewing is never interrupted: a recording that needs a tuner someone is watching waits, starts late (kept as a partial recording) if a tuner frees up, or fails with a clear reason and an owner notice. The other option lets the recording take the tuner, with a countdown on every player.
- **Next episode during the credits:** a viewer setting, off by default. When off, the credits card shows what's next but nothing starts before the title ends.
- **Player choices:** the Settings menu decides how every playback starts; changes made in the player last for that playback only (carried through autoplay and the queue). There is no per-title memory and no "Forget last choice".
- **Picture-in-picture:** no subtitle support. Burned-in subtitles stay; drawn ones disappear until the picture returns. Opening it never starts a conversion.
- **Exact resume in MP3, ADTS and Ogg:** a small per-file seek index built once at scan time; the player reads the original file from the right place. No conversion.
- **Collections:** personal and library collections have a manual order ("Collection order" is the default, playback follows it).
- **Administrator role:** exactly what the server allows that tier (Watching now, members below them, invitations, devices, logs, titles set aside). Confirmed.
- **My List and Favorites:** the full toolbar replaces their old chips; the "Progress" sort is dropped.

**Things you should know:**
- **The apps' mpv engine is Portico's own build** from pinned source: FFmpeg at the server's 8.1.2 configured LGPL 2.1, OpenSSL 3, mpv and the subtitle and HDR libraries, each a separate dynamic library. It replaces MPVKit (Apple) and media-kit (Android), whose FFmpeg was LGPL version 3 and statically linked. Android is built and verified; the Apple build script runs only on the Mac and is the first step of the Apple test plans. Each release must publish the engine's source bundle (about 133 MB).
- **Every client now shows its open-source licences** (Settings › About), generated from the real dependency lists, with a check that fails if they drift. On the Mac, one script fills the licence texts for nine iOS libraries that can't be read on Ubuntu.
- **Release builds no longer contain build-machine folder names** (server and Android). The macOS server's bundled FFmpeg still does; fixing it means rebuilding the FFmpeg toolchain, which its own rules reserve for your approval.
- **The TV owner Dashboard is read-only,** so a TV no longer offers Stop playback, Acknowledge or task actions.
- **Browser subtitle rendering** uses libass compiled to WebAssembly (FriBidi inside is LGPL) and needs `wasm-unsafe-eval` in the web content policy.
- **Apple apps allow plain HTTP to any address** (one app-wide setting).
- **Vega signs in with the redirect risk you accepted.**
- **Hosted must start on an empty database.**
- **Supported browsers:** Chrome and Edge 111, Firefox 113, Safari 16.4; older ones get a plain message.
- **Defaults still in place:** missing titles kept 30 days; a quality the server can't produce is refused rather than raised; uncertain matches go to the attention list; casting burns styled subtitles in (the Chromecast receiver shows text only); Android treats a box with no touchscreen as a TV, with a Layout setting to override.

## 4. What is not finished

**Open audit bugs (11), mostly for devices, real media or release:**
- Apple's old unexplained sign-out incident (B-01.23); the real cost of "Complete" scanning (B-04.35); gapless fixtures needing real HE-AAC media (B-10.28); a Watch Together benchmark at the room-size ceiling (B-13.10); device cleanup after sign-out on real devices (B-14.14); Android TV density on hardware (B-19.8); Apple and Vega release builds and first-party device acceptance (B-25.10); Apple distribution rights and the final binary notices review (B-25.12).
- Partly done: release installer signing (B-15.19, the advisory is fixed); library and saved-item pin controls (B-18.27, everything else in it is done); the native test helper still imitates React internals (B-19.15, Vega CI is added).

**Smaller gaps the lanes left:**
- Home shelf rows and story-arc episode groups play the rows loaded on screen, not the whole list.
- Lists that span libraries have no letter rail; History across libraries reads the 10,000 most recent plays.
- Native: sorted My List and Favorites draw every card as a poster; there's no way to remove an entry from a personal collection; collection reorder is by menu (no drag); "Forced only" shows for every image track.
- The music Playlists tab (the list of playlists) sorts by name only.
- Seek-index exact resume is used by the web player only; Apple and Android seek those files their own way.
- Merging whole albums, books or artists; Forgot PIN on phones; play history for administrators; the silent retry for WebDAV source commands.
- Apple TV mpv is SDR only; Vega is SDR and stereo.
- The new release workflow has never run.
- The reads that still grow with library size (section 2), which you deferred.
