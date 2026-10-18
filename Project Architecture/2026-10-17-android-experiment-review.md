# Android experiment review on Ubuntu

## Purpose and scope

Bring the existing Classic and Cinema native experiments to the Ubuntu Android phone and Android TV emulators so Justin can compare their current visuals and interactions. These are private fixture-driven design experiments and will not be released.

Keep each design, sample household, navigation structure and simulated player intact. Make only build, packaging and Android compatibility corrections needed for useful review. Do not expand into general polish, real backend integration, real playback, production signing or exhaustive device acceptance. Preserve the Apple projects and the installed main Portico application.

## UI direction

Classic keeps its familiar media-library layout, poster grids and rail. Cinema keeps its marquee, immersive artwork, typography and expanding rail. Phone touch and Android TV D-pad interactions must allow representative browsing, details, dialogs and simulated player review. Retain the bundled fonts, artwork and screen indexes.

## Architecture and data

Each experiment remains its own React Native application in `experiments/classic` or `experiments/cinema`, using the existing `react-native-tvos` dependency, typed service interfaces and in-memory fixture implementation. No server deployment or API changes are needed. Shared Ubuntu build/install/launch tooling belongs alongside the experiments and must select each application explicitly.

## Security and preservation

Use distinct experiment application IDs and the existing development signing approach. No production credentials, main-app authentication state or backend services are needed. Do not uninstall, reset or replace the main Portico app. Preserve unrelated changes and all existing emulator data. Build caches, logs, APKs and screenshots stay ignored. No publishing or Git commits are required by this task.

## Platform and deployment

Execution host: personal Ubuntu mini-1 (`100.120.50.24`). Existing phone and Android TV AVDs are the review targets. Build x86_64 standalone testing APKs with bundled JavaScript; keep local Metro development possible. Java 17 and Android SDK/NDK are installed. Bound build concurrency to avoid exhausting memory while the emulators remain available.

## Verification and handoff

Build both APKs, install both alongside the main app on both emulators, inspect actual screenshots and exercise representative phone and D-pad flows. Record any fixes and remaining review limitations. Leave repeatable commands, application names and screen-index access documented. Emulator review establishes suitability for comparing these experiments; it does not establish Apple rendering parity or production readiness.

## Status

Complete for the requested visual-review scope. Classic and Cinema were brought up separately, both standalone APKs installed on the phone and TV emulators, and representative screenshots and interactions reviewed. The existing main-client TV display adapter corrected confirmed 2× Android TV scaling while preserving phone and Apple behavior. Other changes are Android packaging, Classic fonts/deep links and official launcher artwork.

The shared helper is `scripts/test-experiments.sh`; the review and verification guide is `experiments/ANDROID-REVIEW.md`. Local screenshots and before/after installation evidence are ignored under `.scratch/experiment-review/`. Both devices retain the main Portico app and both experiments. There were no dependency upgrades, design-screen changes, commits or publishing operations.
