# Lane brief: final-pass-2 — test everything after the completion pass

Work directly in `main-server` (`/home/justin/projects/Portico/.worktrees/audit-implementation/main-server`, head a4f86250) and `main-native` (`.../main-native`, head 01b78bd), both on branch `audit-implementation/main`. Report: `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/final-pass-2.md`.

## Rules
- **Do not spawn subagents or helpers; do the work yourself.** Commit with Justin's configured identity, no AI attribution or tool names in messages. Never push. Don't touch Justin's main checkouts.
- Never weaken a guard, bound, restriction matrix or security test to make something pass. Fix the cause; if a test is genuinely stale after the merges, update it and say why.
- `GOFLAGS=-p=4`, `GOCACHE=$PR/scratch/gocache-final-2` (delete at the end), `nice -n 10`. Stop if free disk < 40 GB.
- Read `_program/reports/final-pass.md` first: it records exactly how the previous final pass ran everything (playback helper build and `PORTICO_PLAYBACK_TEST_HELPER`, cross-builds, drivers). Then read the `cp-*.md` and `licences.md` reports' "Not finished" and test sections so you know what changed.

## Steps
1. Server: `go vet` (known vendored SQLite warnings only), full `go test ./...` with the helper set, apikit tests, `go run ./cmd/apigen` gives no diff, Windows/macOS cross-builds, production web build, client-core, web `npm run check` (after `npm ci` if needed), cast tests, contracts tests.
2. Flaky test: `TestPlaybackV1QueueQueryCapturesBeyond500AndKeepsItsSnapshot` (relies on a 10 ms sleep) failed under load in two lanes. Find out whether it is a test timing assumption or a real race in queue capture; fix properly (no longer sleeps, or the race fixed). Run it 20× under `-race` and under full-suite load.
3. Native: shared packages identical to main-server (`--check`; lock names main-server HEAD after any server commit — rerun `--write` and commit if you commit on the server), type-checks, `lint:hooks`, `lint:strings`, native `npm test`, Vega type-check and tests, licence-list checks (web and native) current.
4. End to end: the real-server driver and the headless-browser driver (as in final-pass.md). Extend the browser driver only if a new screen from the completion pass crashes or is unreachable; report new-screen coverage gaps rather than building a big new suite.
5. Android: build the media engine output exists (`_program/scratch/own-mpv/out` or rebuild arm64 only if missing) and run `./gradlew :portico-mpv:assembleRelease` and the app's `assembleGoogleDebug -PreactNativeArchitectures=x86_64` if it works in reasonable time; if an emulator image is installed, launch the app and play one generated file with Player = mpv. Report what you could and couldn't do.
6. Fix every failure you find (commit with clear messages). Rerun what you touched.

## Report
Outcome in plain words first; table of every command → result (first run, cause, fix); fixes made; the flaky test's root cause; anything not run and why.
