# Classic and Cinema: Android visual review

These are the original native frontend experiments, running on fixture data for design and interaction comparison. Playback is simulated. No Portico server or real sign-in is needed. They are private testing apps, not release candidates.

## Open either design

From `/home/justin/projects/Portico/portico-internal`:

```sh
scripts/test-experiments.sh start phone
scripts/test-experiments.sh start tv

scripts/test-experiments.sh open phone classic
scripts/test-experiments.sh open phone cinema
scripts/test-experiments.sh open tv classic
scripts/test-experiments.sh open tv cinema
```

Both apps also appear as **Portico Classic** and **Portico Cinema** in the Android launcher. Their experiment IDs are separate from the main Portico app. Switching apps preserves the main app's installation and data.

Cinema can start on its original welcome flow. To skip directly into the populated library, use `screen phone cinema home` (or `screen tv cinema home`). Allow the cold launch and fixture loading to settle before interacting; subsequent navigation is quicker. The installed builds work without Metro.

The phone emulator supports normal mouse/touch interaction. On TV, use the emulator remote or arrow keys and Select; Back returns through the app. Keyboard/remote behavior can also be driven explicitly:

```sh
scripts/test-experiments.sh key tv right down select back
```

## Jump to a screen

Both designs have an in-app screen index: Classic under **Settings > Screen index**, Cinema under **Settings > About > Screen index**. The existing review deep links also work on Android:

```sh
scripts/test-experiments.sh screen phone classic movie
scripts/test-experiments.sh screen phone cinema movie
scripts/test-experiments.sh screen tv classic home
scripts/test-experiments.sh screen tv cinema home
```

Useful shared screen IDs include `home`, `movie`, `show`, `search`, `player`, `now-playing`, `settings` and `library-movies`. The full inventories are in `classic/src/nav/screens.ts` and `cinema/src/nav/screenIndex.ts`; some IDs differ between the designs.

Screenshots save to ignored `.scratch/experiment-review/` by default:

```sh
scripts/test-experiments.sh shot tv classic
scripts/test-experiments.sh shot phone cinema
```

## Rebuild after changes

```sh
scripts/test-experiments.sh build classic
scripts/test-experiments.sh build cinema
scripts/test-experiments.sh install phone all
scripts/test-experiments.sh install tv all
```

The helper installs locked dependencies if absent and builds one experiment at a time with a bounded Gradle worker/heap. Default ABI is `x86_64` for these emulators. APKs are at `classic/android/app/build/outputs/apk/release/app-release.apk` and the equivalent Cinema path. JavaScript and artwork are embedded; no Metro server is needed for review.

For faster code iteration, each project retains its original Metro setup (Classic 8091, Cinema 8092). That uses a separate debug build; the installed standalone review APKs do not connect to Metro.

## Scope and verification

Android changes are limited to build/packaging and compatibility needed for reviewing existing designs. This pass does not finish the original Classic polish backlog, connect a backend, provide real video playback or establish Apple/physical-device parity.

Verified on 17 October 2026 on personal Ubuntu mini-1:

- Both locked dependency restores, TypeScript checks and standalone release builds pass. Each APK contains its JavaScript, artwork/fonts and x86_64 native libraries.
- Both apps installed on both existing AVDs: `Portico_Phone_API36` (`emulator-5554`, 1080×2400) and `Portico_TV_API36` (`emulator-5556`, 1920×1080).
- Phone: inspected Home, title details, action sheets, Search and simulated player screens in both designs. Exercised Classic's detail Play button and both action sheets; Cinema's player Back returns to the previous screen. Text input and the system keyboard were exercised in Classic Search.
- TV: inspected both designs at their intended 1920-point canvas. Classic rail navigation opened Movies; detail buttons, action-menu focus and Back worked. Cinema poster focus updated the marquee, Select opened the selected title, and Back returned. Both simulated players render with their TV controls.
- Neither device's retained Android crash buffer contains an experiment crash reference. The main Portico app's version and first-install/last-update records match the records taken before this work.
- Final review state: phone on Classic Home; TV on Cinema Home. Both experiments remain installed on both devices. Test-only in-memory changes were reset by restarting the experiment processes.

Compatibility changes: Classic's separate application ID/name, Android deep-link registration and four missing Manrope fonts; official launcher icons/banners in both experiments; and the existing main client's app-scoped TV display adapter, preserving these designs' 1920-point sizing without changing the emulator's global density. No screen-design JavaScript or Apple projects changed.

Review evidence is local and ignored under `../.scratch/experiment-review/`, including `classic-phone-ready.png`, `cinema-tv-ready.png`, detail/action/player screenshots and the installation/crash-buffer checks. One phone System UI ANR appeared during the resource-heavy cold boot while the first builds were finishing; choosing Wait allowed it to recover and the review continued. These software-rendered emulators are useful for comparing layout and interaction, not measuring device performance. This was a representative smoke pass, not an every-screen acceptance audit.
