# Lane brief: final-pass-3 — verify after the follow-ups merge

Work directly in `main-server` (head 6ae3ccf4 or later) and `main-native` on `audit-implementation/main`. Report: `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/final-pass-3.md`.

Rules exactly as in `briefs/final-pass-2.md` (no subagents or helpers; Justin's identity, no AI attribution; never push; never weaken a guard or security test; own GOCACHE deleted at the end; nice; stop if < 40 GB free). Follow `reports/final-pass-2.md` for how every command is run (playback helper build + `PORTICO_PLAYBACK_TEST_HELPER`, drivers, Android builds). Read `reports/cp-followups.md` to know what changed: PiP without subtitle hand-over, seek-index exact resume for MP3/ADTS/Ogg (FLAC deep start removed), session-only player choices (per-title memory and "Forget last choice" removed), manual order for personal and library collections.

Steps:
1. Server: vet, full `go test ./...` (with the helper rebuilt from the current tree), apikit, apigen no diff, Windows/macOS cross-builds, production web build, client-core, web `npm run check`, cast, contracts, licence lists current.
2. Native: shared packages `--check` (lock names main-server HEAD; if you commit on the server, rerun `--write` and commit), type-checks, lints, native tests, Vega.
3. End to end: server driver and browser driver. Visit once (desktop + phone width) the new/changed screens from cp-followups: a collection in Reorder mode (personal and library), the player's PiP control if the driver can reach it, and a web audiobook resume into an MP3 (exact landing).
4. Android: `:portico-mpv:assembleRelease` and `assembleGoogleDebug -PreactNativeArchitectures=x86_64`; on the emulator play one file with mpv and check a fresh playback does not reuse the previous playback's subtitle/speed choice (session-only rule).
5. Fix every failure (commit clearly), rerun what you touched.

Report: outcome first; command → result table; fixes; anything not run.
