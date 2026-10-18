# Lane brief: verify-1 — full verification of the merged state (16 Oct)

Twelve lanes merged into `audit-implementation/main` on 15 Oct without full test runs (Justin chose delivery first). Four more lanes are finishing now and will merge later. Your job: run the complete verification on the current merged state **in your own worktrees** and fix every failure, so the final run after the last merges is short.

Worktrees: `$G/server-verify-1`, `$G/native-verify-1` (branch `audit-implementation/verify-1`, from main-server 617dc62d / main-native a8c51ea). Commit fixes there (Justin's identity, no AI attribution; never push). Rules as in `cp-common.md`; never weaken a guard, bound, restriction matrix or security test; fix causes; a stale test may be updated with a stated reason.

Follow `reports/final-pass-2.md` exactly for how to run each step (playback helper + `PORTICO_PLAYBACK_TEST_HELPER`, drivers, Android). Steps:
1. Server: vet (known vendored SQLite warnings only), full `go test ./...`, apikit, apigen no diff, Windows/macOS cross-builds, production web build (`npm ci` in web first: new dev dependency oxlint), client-core, web `npm run check`, cast, contracts, licence-list checks (regenerate if stale).
2. Native: shared packages `--check`, type-checks, `lint:hooks`, `lint:strings`, `npm test`, Vega type-check and tests, native licence list check. Kotlin: `./gradlew :portico-mpv:assembleRelease` and `:app:assembleGoogleDebug -PreactNativeArchitectures=x86_64` plus the new JVM unit tests (`./gradlew :app:testGoogleDebugUnitTest` or as cp3-docs set them up; JUnit needs network once).
3. End to end: server driver and browser driver; visit the screens added on 15 Oct once (owner tools, pins, reorder, licences, secure connection, playlist list sorting, skipped-data panel) at desktop and phone width.
4. Android emulator if available: sign in, play a film with Player = mpv, play an MP3 audiobook resume, check credits Up Next with the setting off (default).
5. Fix every failure, rerun what you touched. Keep a table: command → first result → cause → fix → rerun.
Report `$PR/reports/verify-1.md`, outcome first. Scratch under `$PR/scratch/verify-1`, own GOCACHE, delete at the end.
