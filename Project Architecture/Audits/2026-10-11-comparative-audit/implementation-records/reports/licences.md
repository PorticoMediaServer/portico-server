# Lane report: licences (on-screen open-source licences, neutral build paths)

Worktrees: native `native-licences` (branch `audit-implementation/licences`, from `4f0689c`), server/web `server-licences` (branch `audit-implementation/licences`, from `f719c2a0`). Nothing pushed. Scratch: `_program/scratch/licences/` (`out/android` = the verified engine build, `logs/`).

## 1. Outcome

- **Every first-party client now has an "Open-source licenses" screen** (en-GB/en-CA say "licences"), reached from Settings › About. It opens with Portico's own GPLv3 notice (a row opening the full GPL text) and where the corresponding source is published, then lists every component, grouped, with version and licence; each opens a page with its copyright lines and full licence text.
  - **Web**: Settings › About › "Open-source licenses" (`/settings/about?tab=licenses`, one component `&id=…`). Lists the browser bundle's npm packages, the subtitle renderer, fonts and icons, and what the server binary carries (Go, the Go modules, vendored server code and fonts, the FFmpeg build). Source links: Portico server/web repository and the FFmpeg toolchain release (`ffmpeg-8.1.2-portico.1`). The list is its own chunk, fetched only when the page opens.
  - **iPhone/iPad, Android phone/tablet**: Settings › About › "Open-source licenses" pushes a list screen; a row pushes the component screen. Source links open in the browser.
  - **Apple TV, Android TV, Fire TV (Android)**: same route from the About page in the two-column Settings. Everything is focusable with the remote: rows, the source rows (shown as readable rows with the address, since a TV cannot open links), the version/licence/copyright rows, and the licence text, split into paragraphs that each take focus so the remote scrolls through long texts. Both screens are virtualised lists.
  - **Vega**: the same shared screens (it runs the Apple app's screens), with its own list via a Vega override of the loader.
- **The lists are generated, not hand-written**, from the real inventories (section 3) and **checked**: tests fail when any inventory changes without regenerating, and an Android release build stops when its libraries differ from the pinned lists.
- **Neutral build paths**: the media engine's libraries no longer contain any build-machine folder. **All four Android ABIs were rebuilt** (arm64-v8a, armeabi-v7a, x86, x86_64) and `verify.py android` passes **5,025 checks, 0 failed** (it was 108 failures on the previous build with the new check). The server's release builder now uses `-trimpath`; the Android app's own C/C++ gets prefix maps; web and JavaScript bundles were already clean.

## 2. Component counts

| Client | File | Components | Distinct texts | Size | Breakdown |
|---|---|---|---|---|---|
| Web | `web/src/generated/licences.json` | 153 | 39 | 218 KB | Portico 1, npm 120, subtitle renderer 1, fonts/icons 10, server 21 (Go std + 13 modules, SQLite driver copy, image scaler, 4 server fonts, FFmpeg) |
| Android / Android TV / Fire TV | `apps/native/src/licences/android.json` | 272 | 57 | 783 KB | Portico 1, npm 49, engine 14 + 28 libdovi crates, Maven 140, Google Play services bundled notices 32, React Native's native libraries 6, fonts/icons 2. 50 are Google-build only (Cast, Play services) and hidden on Fire TV |
| iPhone / iPad / Apple TV | `apps/native/src/licences/apple.json` | 105 | 39 | 443 KB | Portico 1, npm 49, engine 15 + 28 crates, pods 10, fonts/icons 2. google-cast-sdk shown on iPhone/iPad only |
| Vega | `apps/vega/src/licences/vega.json` | 80 | 22 | 696 KB | Portico 1, npm 77, fonts/icons 2 (one Amazon SDK package ships a 605 KB notice) |

## 3. How the lists are generated and checked

Shared pieces (server repo, mirrored to native with `scripts/shared-packages.mjs`):
- `packages/client-core/src/presentation/licences.ts`: the file's types and what the screens show (sections in fixed order, row text, detail, paragraphs for TV, source rows, variant filtering, parser).
- `packages/client-core/src/licences/inventory.ts`: pure generator rules: lockfile production packages (no dev/devOptional, no workspace links, no `@types/*`), copyright lines split from the head of a licence text so identical wordings are stored once (GNU licences kept whole), Maven POM and Podfile.lock readers, stable output.
- `packages/client-core/src/licences/standard-texts.ts`: standard MIT/ISC/BSD-3-Clause/0BSD/Apache-2.0 wording, copied by script from real offline licence files, used only when a component ships no licence file.

Web — `node --experimental-strip-types web/scripts/licences.ts [--check]` (`npm run licences` in web/):
- npm: production packages of the **root `package-lock.json`** (what the Dockerfile installs); texts from node_modules (installed version must equal the locked one); patched hls.js from `third_party/hls.js/LICENSE`. This is a lockfile closure, so a few packages pulled in only by qrcode's command-line tool (yargs and friends) are listed though tree-shaken out.
- `third_party/*/provenance.json`, `web/src/ui/fonts/OFL.txt`, `packages/design/LICENSE-LUCIDE`.
- Server: `go list -deps ./cmd/server` for linux/darwin/windows × amd64/arm64, offline (`GOPROXY=off`), licence files from the module cache; Go's own LICENSE+PATENTS; vendored code/font licences; FFmpeg from `third_party/ffmpeg/sources.lock.json`.
- Check: `web/test/licences.test.ts` compares the SHA-256 of every repository file the generator read (lockfile, go.mod/go.sum, provenance and licence files), checks the npm rows equal the lockfile's production packages, and, where node_modules and Go are present, regenerates and requires a byte-identical file.

Apps — `node --experimental-strip-types apps/native/scripts/licences.mts [--record] [--check]` (from the native repo root; needs the engine's notices: `MEDIA_ENGINE_OUT_DIR` or `third_party/media-engine/out`):
- npm: **what Metro actually bundles**, recorded by `--record` from Metro's own module graph (source map) for android, ios and Vega (`kepler`) into `apps/native/licences/js-{android,ios,vega}.txt` (49/49/77 packages, against 407/1,149 production packages in the lockfiles: most of react-native's dependencies are tooling, and `@babel/runtime`, a devDependency, is bundled). Versions are checked against the lockfiles.
- Engine: `third_party/media-engine/sources.lock.json` and the build's `notices/` (collect-notices.sh output), including libdovi's vendored Rust crates. Either platform's notices serve (same pinned archives).
- Android: `apps/native/licences/android-{google,firetv}.txt` pin each flavour's release runtime classpath (seeded from AGP's own dependency report of the last release build, which matches today's build.gradle). Licence names from each artifact's POM in Gradle's cache, licence files from inside the AAR/JAR; Google Play services' `third_party_licenses.json/.txt` are split into one row per bundled component (32, deduplicated); Google's SDK terms are a web page, so those rows say so and link it.
- Apple: `ios/Podfile.lock` and `tvos/Podfile.lock`. Pods that are an npm package's native code are covered by that package (or listed under its name); React Native's generated code is skipped; React Native's third-party pods, Hermes, SocketRocket and google-cast-sdk read their texts from `ios/Pods/<name>` after `pod install`.
- Fonts/assets: `apps/native/licences/assets.json` (Manrope with its OFL text, now `apps/native/assets/OFL-Manrope.txt`; Lucide icons with `packages/design/LICENSE-LUCIDE`, now mirrored).
- Checks: `apps/native/test/licences.test.ts` (12 tests): input digests per list, recorded bundles match locked versions and contain no tooling, every pinned Android library present with Cast only in the Google build, engine rows equal sources.lock.json per platform with crates, every non-generated pod listed, every font in `apps/native/assets` inventoried, and the lists loaded only by the licence screen. Gradle: `checkGoogleReleaseOpenSourceLicences` / `checkFiretvReleaseOpenSourceLicences` run before each release build and stop it when the resolved runtime classpath differs from the pinned list; `./gradlew :app:recordReleaseDependencies` rewrites the lists.
- Lazy loading: per-platform loader files (`src/licences/load.ts` Apple, `load.android.ts`, Vega override) `require` the JSON inside the function; Metro bundles only the platform's file and evaluates it only when the screen calls it, one frame after the screen appears. Web: dynamic `import()` of the JSON from a lazily loaded component.

## 4. Not resolvable offline (Mac step and notes)

- **Apple pods' licence texts** (marked `textPending`, the screen says the text is not shown): boost, DoubleConversion, fast_float, fmt, glog, RCT-Folly, Hermes, SocketRocket, google-cast-sdk (licence names recorded from the podspecs; trunk pods from a small table in the script). **glslang** (Apple-only engine part) until build-apple.sh has collected its notices. MoltenVK shows the standard Apache-2.0 text until then.
- The **same six React Native native libraries are compiled into react-android** on Android; their Android rows are also pending and are filled by the same Mac run (the script reads `apps/native/ios/Pods/<name>` for both lists).
- **Mac step**: after `third_party/media-engine/build-apple.sh` and `pod install` in `apps/native/ios` and `apps/native/tvos`, run `node --experimental-strip-types apps/native/scripts/licences.mts` from the repository root, check that it no longer prints a "text pending" list, run `npm test` in apps/native, commit the three JSON files.

## 5. Build paths: fixes and scans

| Build | Before | Fix | After |
|---|---|---|---|
| Media engine, Android (all 4 ABIs) | 9 libraries per ABI contained `/home/justin/…/work` (27 strings per ABI): FFmpeg's configuration string in all 6 FFmpeg libraries, `__FILE__` from FFmpeg headers in libavcodec, libplacebo header in libmpv, mpv's configure line and config folder, OpenSSL's module/engine folders and compiler line, libdovi's Rust crate paths. `verify.py` with the new check: 108 failures | `lib/common.sh` "Neutral paths": `-ffile-prefix-map/-fdebug-prefix-map/-fmacro-prefix-map` (work dir → `/portico-media-engine`, NDK/Xcode → `/toolchain`) on every C/C++/ObjC compile (meson cross file, autotools, cmake incl. uchardet, FFmpeg `--extra-cflags`, OpenSSL Configure); Rust `--remap-path-prefix` via `CARGO_ENCODED_RUSTFLAGS`; FFmpeg `config.h`, mpv `config.h` and OpenSSL `buildinf.h` (its char-array "compiler:" line) rewritten before compiling; OpenSSL `ENGINESDIR`/`MODULESDIR` set to `/dev/null/…`; the path functions are part of every component's rebuild key | **0** path strings in all 80 libraries; `verify.py android`: **5,025 passed, 0 failed** (`logs/verify-before.txt`, `logs/verify-after.txt`). No rpaths (checked) |
| Media engine, Apple | not runnable here | same flags and header rewrites in `build-apple.sh`; `strip -x -S` (removes the N_OSO debug map); verify gets `--work`; MoltenVK (Khronos's prebuilt) is checked for this machine's folders only | Mac playbook |
| `verify.py` | — | new check: no library may contain the work directory (as given and resolved), the recipe folder, `$HOME/`, `/home/` or `/Users/`; `--work` option | — |
| Server Go, release builder `scripts/build-server-release.py` (used by release.yml for every platform, so also the Windows/macOS/Linux packages) | no `-trimpath`: a test build had 2,358 strings with `/home/justin` | `-trimpath` added; `verify_manifest` refuses a build whose `go version -m` lacks `-trimpath=true` | 0 (`go version -m`: `-trimpath=true`) |
| Server Go, Dockerfile / Makefile | already `-trimpath` | — | clean |
| `package-release.sh`, `package-macos.sh`, Windows NSIS | package sealed builds only (no compiling) | — | covered by the builder fix |
| Server FFmpeg bundles (`scripts/build-ffmpeg.sh`) | Linux/Windows: BtbN container builds (paths under the container's `/opt/ffbuild`). macOS: Homebridge recipe under `runtime/ffmpeg-build/…/recipe/workspace`, so `ffmpeg -buildconf` very likely names the CI runner's workspace (`/Users/runner/…`). No bundle on this host to scan | **not changed**: the FFmpeg toolchain is "manual approval only" and any recipe change means a new qualified toolchain | for Justin (section 9) |
| Web production build (`vite build`, scanned in scratch) | source maps are shipped (`build.sourcemap: true`) but their `sources` are relative; the only `/home/` strings are routes (`/v1/home/…`) and Emscripten's virtual `/home/web_user` | — | clean |
| Native JS bundles (Metro android and Vega, release mode) | 0 absolute paths | — | clean |
| Android app's own C/C++ (scanned the last release APK on disk, Oct 9) | `libappmodules.so`, `libreact_codegen_rnscreens.so`, `…rnsvg.so`, `…safeareacontext.so` contained `/home/justin/.gradle/…/react-android…/propsConversions.h` and `…/node_modules/react-native-screens/…cpp` (asserts) | `app/build.gradle`: `-ffile-prefix-map` for the checkout, node_modules (also its link target) and Gradle's user home in `defaultConfig.externalNativeBuild.cmake` c/cppFlags (React Native's plugin adds to, not replaces, these); `portico-mpv`: the checkout and the engine folder. Folders with spaces are skipped (CMake splits flags at spaces) | not rebuilt here (Gradle not allowed); test note |
| libporticompv.so (mpv JNI bridge) | the bridge source has no `__FILE__`/assert; AGP strips debug info | prefix maps added anyway | — |

## 6. Decisions I made

1. **"Licenses" in the US source catalogue**, "licences" in en-GB/en-CA by a new Commonwealth rule (`license$`/`licenses$` → noun forms; "licensed" untouched).
2. **Web shows the server's components too** (Go, modules, FFmpeg, server fonts): the web app is the server's own screen and the server has no other on-screen notice.
3. **Bundled, not declared, for the apps**: Metro's graph decides the JS rows (49 instead of 407). The web uses the lockfile's production closure (over-inclusive by a few CLI-only packages) because it needs no build to generate.
4. **Android pinned lists + Gradle check** instead of running Gradle in the generator. The lists were seeded from AGP's dependency report of the Oct 9 release build (it matches the current build.gradle; nothing Maven-related changed since). The Gradle check walks the resolved graph and ignores BOM/platform components; if its view differs slightly from AGP's report, the first release build will say exactly what differs and `recordReleaseDependencies` fixes it.
5. **Google Play services**: their own terms are a web page (POM link) and the screen says so; the third-party code inside them is listed row by row from Google's own index.
6. **Copyright lines** are split off licence heads so the same MIT/BSD wording is stored once and shown with the right owner. For a package with no licence file the standard text of its declared licence is shown with "Copyright (c) <author named in the package>".
7. **No search box**: the lists are grouped and virtualised; search can be added if wanted.
8. **Vega**: no separate engine/native section (it ships no native libraries of ours).
9. **FFmpeg toolchain recipe not touched** (manual-approval policy).
10. Script file is `licences.mts` in apps/native (ES module without changing the React Native package's type).
11. The `verify.py` path check treats Khronos's prebuilt MoltenVK leniently (this machine's folders only): we cannot rebuild it.
12. The neutral-path guard that skips folders containing spaces was added after the verified build; it does not change output for paths without spaces but changes the rebuild key, so the next engine build rebuilds everything once.

## 7. Bugs

None assigned to this lane.

## 8. API, schema, settings

API changes: none. Schema changes: none. Settings: none. New strings (`licenses.*`, 28) in the en-US catalogue; regional rule added.

## 9. Needed from others / for Justin

- **Mac step** (section 4) for Apple pod texts, glslang, and the six React Native native libraries on Android.
- **Server FFmpeg macOS bundle**: next time the toolchain is rebuilt, build the Homebridge recipe path-neutral (add `-ffile-prefix-map=$WORKSPACE=/ffbuild` to its CFLAGS through the existing patch set and run it from a neutral folder), then scan `ffmpeg -buildconf` and the dylibs for `/Users/`. Linux/Windows bundles: scan once with `strings | grep -E '/home/|/Users/'` when convenient.
- **iOS/tvOS app's own compiled code** (Pods built from node_modules) likely embeds `/Users/…` paths in asserts like Android's did; a Podfile `post_install` adding `-ffile-prefix-map=<checkout>=/portico` to `OTHER_CFLAGS`/`OTHER_CPLUSPLUSFLAGS` would fix it (needs a Mac to verify; mind a checkout path with spaces).
- **Release/ops**: publish the media-engine source bundle with each app release at the apps repository's Releases page (the screen points there).
- **Docs**: `THIRD-PARTY-NOTICES.md` (server repo) still describes MPVKit and media-kit for the native apps; the own engine replaced them. `docs/dependency-licenses.md` could now point at the generated list.

## 10. Test notes for other environments

- **Android (Gradle, any Linux host)**: `./gradlew :app:assembleGoogleRelease -PporticoLibmpvDir=<engine out/android>`. Pass: `checkGoogleReleaseOpenSourceLicences` runs before `preGoogleReleaseBuild` and succeeds; if it fails listing "New/Gone" libraries, run `./gradlew :app:recordReleaseDependencies`, then `node --experimental-strip-types apps/native/scripts/licences.mts`, re-run, commit. Then unzip the APK and `strings -a lib/*/*.so | grep -E '/home/|/Users/'` must print nothing except false positives such as `/v1/home/rows` (expect libappmodules and the codegen libraries clean now).
- **Android device/TV**: Settings › About › Open-source licenses: list loads after the screen appears; Fire TV build hides play-services/Cast rows; open "FFmpeg" and move down with the remote through the text; Back returns to the list, then About.
- **iPhone/Apple TV**: same path; on Apple TV every row and text paragraph takes focus; google-cast-sdk appears on iPhone only.
- **Vega virtual device**: `node scripts/vega.mjs run --release`, Settings › About › Open-source licenses; 80 components, no engine section.
- **Web**: Settings › About › Open-source licenses; the list chunk (`licences-*.js`, ~209 KB) loads only then; back link returns to About; a component page shows a scrollable, keyboard-focusable text.
- **Apple engine build**: `build-apple.sh` ends with `verify.py apple --work …`; any path failure names the library and the string.

## 11. Tests run (all on this host)

- client-core `licences.test.ts` 11/11; whole client-core `npm test` **1,958/1,958** (1,947 + 11); `npx tsc --noEmit -p packages/client-core` clean.
- web `licences.test.ts` 6/6 (includes a full regeneration compare and screen-level tests of the list and detail); whole web `npm test` **552/552** (546 + 6); web `tsc --noEmit`, `lint:strings`, `lint-design --strict` clean.
- native `licences.test.ts` 12/12; native `npm test` **677/677** (665 + 12); Vega `npm test` **87/87**; `tsc --noEmit` clean for native, native tests (now including the script) and Vega; `lint:strings` clean for native and Vega; `shared-packages.mjs --check` identical.
- Media engine Android, 4 ABIs rebuilt from scratch with the new flags (~25 min, `nice -n 10`, `-j8`), `verify.py android`: 5,025 passed, 0 failed. Not run on an emulator this time (the dependency graph and contents checks match the previous, emulator-tested build).
- Server release-style Go build with and without `-trimpath` compared (2,358 → 0 `/home/justin` strings). Web `vite build` into scratch scanned. Metro release bundles (android, Vega) scanned. Groovy syntax check of both Gradle files with Gradle 9's Groovy and API jars (compiles).
- Not run: Gradle, Xcode, any device; `build-apple.sh`.

## 12. Commits

Server (`audit-implementation/licences`):
```
b20a1d5b Build release server binaries without the build machine's folders
70a7261a Say when a licence's terms are published on the web, and read pods named in capitals
1e6bbe28 Show open-source licences in the web app's Settings › About
6175a711 Describe open-source licence lists for every client and the rules that build them
```
Native (`audit-implementation/licences`):
```
e0707ea Keep build-machine folders out of the Android app's own native libraries
450aef9 Build the media engine without the build machine's folders in its libraries
e705e9c Show open-source licences in the apps' Settings › About
7d768e7 Mirror the shared licence list model and its strings
```

## 13. Not finished

- Apple pod texts, glslang's text and the six React Native native libraries' texts: pending until the Mac step (section 4).
- The Android Gradle check and the app's C/C++ prefix maps are written and syntax-checked but not run (Gradle not allowed here).
- Apple engine path-neutral build not run (no Mac).
- macOS server FFmpeg bundle and iOS Pods path neutrality: proposed, not changed (section 9).
- No visual browser check of the web screen (it needs a signed-in server session); covered by screen-level tests only.
