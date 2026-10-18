# Completion pass — shared rules for every lane (15 Oct 2026)

Justin asked for the remaining audit scope to be finished before he starts hands-on testing: the gaps against his decisions (owner's report `IMPLEMENTATION-REPORT-2026-10-14.md` §4 and Appendix 0 "What is not finished") and the open audit bugs that can be fixed without devices (Appendix D). Report: `/home/justin/projects/Portico/.worktrees/audit-implementation/main-internal/Project Architecture/Audits/2026-10-11-comparative-audit/IMPLEMENTATION-REPORT-2026-10-14.md`. Programme decisions: `_program/PROGRAM-BRIEF.md` §1 (read it). Lane reports referenced by Appendix D are in `_program/reports/`; `decision-audit.md` has the per-client "Needed from others" list.

Paths: `G=/home/justin/projects/Portico/.worktrees/audit-implementation`, `PR=$G/_program`.

## Rules (non-negotiable)

- **Do not spawn subagents or helpers of any kind (no Agent tool, no Codex, no Opencode); do the work yourself.**
- Each lane has two worktrees: `$G/server-cp-<lane>` (portico-server: Go server, `web/`, canonical shared packages) and `$G/native-cp-<lane>` (portico-react-native: phone/tablet/TV apps, Vega), both on branch `audit-implementation/cp-<lane>`, from main-server `f719c2a0` and main-native `4f0689c`. Work and commit only there. Justin's configured Git identity; no AI authors, co-authors, bylines or tool names in commit messages. Never push. Never touch Justin's main checkouts or any other worktree.
- **Vertical slices:** you own your features end to end — server, generated contracts, client-core, web, native phone/tablet, TV (focus-navigable), Vega where it applies. A feature is done when every first-party client that should have it has it.
- Shared packages (`packages/client-core`, `contracts`, `i18n`, `design`) are canonical in portico-server. After changing them, mirror into your native worktree with `PORTICO_SERVER_REPO=$G/server-cp-<lane> node scripts/shared-packages.mjs` (from the native worktree) and commit the mirror there. Never hand-edit the native copies. New strings go in the i18n catalogue.
- API: typed apikit registry with key scopes on every route (`Requires: apikit.Scope…`); regenerate with `go run ./cmd/apigen`. Schema: edit the pre-release baseline `0001_baseline.sql` (no migrations) and run `python3 scripts/schema-sync.py`. No back-compat shims (pre-release). All writes through the single write gate (`dbwork`).
- **Performance discipline** (Justin cares): per-item cost must stay flat as libraries grow; no member scans where a maintained counter/index exists; bounded batches; no N+1 reads; totals exact to 10,000 then "10,000+" (`totalIsLowerBound`).
- Security trade-offs are deliberate (plain HTTP allowed, light passwords, no lockout, nothing encrypted at rest). Don't "fix" them. Never weaken a guard, bound, restriction matrix or security test to make something pass.
- Product exclusions stand: no plugins/themes/custom CSS, no push or email, no light theme, no import/export, no Roku, no sub-path hosting, no CarPlay/Android Auto.
- Match the surrounding code: naming, comment density, components, design tokens, existing screen patterns.
- Build and test economy: `GOFLAGS=-p=4`, `GOCACHE=$PR/scratch/gocache-<lane>`, `nice -n 10`. Run the tests of the packages and clients you touch (Go package tests, `npm run test:core`, web tests for touched areas, native `npm test`, Vega `npm test`, type-checks). The orchestrator runs the full suites and end-to-end drivers at the end. Check `df -h /` before heavy steps; stop and report if free space < 40 GB. Delete your GOCACHE at the end.
- Other lanes run in parallel on different features; if you must touch a shared hotspot (i18n catalogue, generated contracts, a shared screen), keep edits minimal and localized so merges are easy. Don't refactor unrelated code.
- If a decision is genuinely needed, take the option that best fits Justin's positioning (Jellyfin-style freedom + Plex-style polish, slight lean to ease of use), record it in your report under "Decisions I made", and continue. Don't stop to ask.

## Report

Write `$PR/reports/cp-<lane>.md`: 1 Outcome (plain words) · 2 Decisions I made · 3 Audit bugs closed (ID → what changed → test) and decision gaps closed · 4 API changes · 5 Schema changes · 6 Settings added · 7 Tests run (command → result) · 8 Device test notes (what to check on real hardware) · 9 Not finished (be honest) · commits in each repo. Reply with a short plain-language summary.
