# Lane brief: licences — on-screen licence notices and neutral build paths

Worktrees:
- native: `/home/justin/projects/Portico/.worktrees/audit-implementation/native-licences` (portico-react-native, branch `audit-implementation/licences`, from main-native `4f0689c`, which includes the own media engine in `third_party/media-engine/`)
- server/web: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-licences` (portico-server, branch `audit-implementation/licences`, from main-server `f719c2a0`)

Report: `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/licences.md`.

## Rules

- **Do not spawn subagents or helpers; do the work yourself.** Commit only in your two worktrees, Justin's configured identity, no AI attribution or tool names in messages. Never push. Don't touch other worktrees or Justin's main checkouts.
- Shared packages (`packages/client-core`, `contracts`, `i18n`, `design`) are canonical in portico-server and mirrored into native by `scripts/shared-packages.mjs` (run from native with `PORTICO_SERVER_REPO=<server-licences path>`). New strings go in the server's `packages/i18n` catalogue first, then mirror; never edit the native copy by hand.
- Disk: build scratch under `_program/scratch/licences/`; stop if free space < 40 GB. `nice -n 10`, ≤ `-j8`.

## Task 1 — on-screen open-source licence notices (all first-party clients)

Licences require the notices to travel with the apps (LGPL 2.1 engine libraries, OpenSSL Apache-2.0, fonts under OFL/Apache, npm packages under MIT/BSD/ISC/Apache, Android libraries such as Media3 and Google Cast, etc.). Today nothing shows them on screen.

Build an **"Open-source licences"** screen, reached from the existing About / Settings area, on:
- **web** (server repo `web/`),
- **native phone/tablet**, **Android TV / Fire TV / Apple TV** (TV layouts must be fully focus-navigable with a remote, long texts scrollable by remote),
- **Vega**.

Requirements:
- A list of components (name, version, licence name) → a detail view with the full licence text and copyright lines. Portico's own GPLv3 notice and a link/statement on where the corresponding source is published (Portico source, the media-engine source bundle, FFmpeg source for the server bundle) at the top.
- **Generated, not hand-written**: a script per repo produces a data file (JSON) from the real inventories — npm lockfiles for what is actually bundled into each client (production dependencies only; exclude dev tooling), the media engine's `collect-notices.sh` output / `sources.lock.json` (native), fonts and vendored assets (`third_party/`, `THIRD-PARTY-NOTICES.md`), Android Gradle dependencies (use the Gradle dependency/licence information available offline, or a pinned list with a check), CocoaPods (from `Podfile.lock` + each pod's licence file where available offline; if a pod's licence text isn't available on Ubuntu, record name/version/licence identifier and mark it for the Mac step). Dedupe identical licence texts. Add a **check** (test or lint) that fails when the generated file is out of date with the lockfiles, so it can't silently drift.
- Load the licence data lazily (it is large); don't bloat the first paint or the TV app start.
- Strings through the i18n catalogue; follow existing screen patterns, components and design tokens (look at the existing About/Settings screens on each client first and match them).
- Tests for the generator and the check; screen-level tests where the existing test style allows. Type-checks clean. Native `npm test` (665) and Vega (87), web check (546), client-core (1,947) still pass.

## Task 2 — neutral build paths in release binaries

The media engine build leaves the build machine's folder path inside the binaries.
- `third_party/media-engine/` (native): make both `build-android.sh` and `build-apple.sh` produce path-neutral output: `-ffile-prefix-map`/`-fdebug-prefix-map`/`-fmacro-prefix-map` for C/C++ (all components, including FFmpeg's configure `--extra-cflags`, meson and cmake builds), Rust `--remap-path-prefix` (libdovi via cargo-c, `RUSTFLAGS`), and anything else the strings scan finds (e.g. pkg-config paths, rpaths, `__FILE__` in asserts). Add a `verify.py` check that fails if any built library contains the work directory, `$HOME`, or `/home/`/`/Users/` paths. Rebuild **Android** (at least arm64-v8a fully; all four ABIs if time allows — record which) and run `verify.py android` to prove it. The Apple script gets the same flags (not runnable here).
- Check the other release builds for the same issue and fix if needed: server Go builds (the Dockerfile already uses `-trimpath`; check `scripts/package-release.sh`, `scripts/package-macos.sh`, `scripts/build-ffmpeg.sh` and any Windows packaging script), the web production build (source maps or absolute paths in the bundle), the Android app's own JNI library (`portico-mpv` CMake), and the native JS bundles. Report what you checked even when it was already clean.

## Report

`_program/reports/licences.md`: outcome in plain words first; what each client shows and how to reach it; how the data is generated and checked; component counts per client; anything that could not be resolved offline (e.g. pod licence texts for the Mac step); build-path fixes and the scan results before/after; tests run with results; commits in each repo.
