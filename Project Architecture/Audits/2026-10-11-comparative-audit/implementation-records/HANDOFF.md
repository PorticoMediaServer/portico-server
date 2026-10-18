# Handoff — Portico audit implementation programme (15 Oct 2026, 11:50 pm)

> **16 Oct, final:** all lanes are merged and verified; this handoff is historical. Current state: the owner's report front (main-internal `1658cd8`) and `_program/STATE.md`. Heads: server `fbcd4ba9`, native `2bdb850`, internal `1658cd8`.

For the next agent (Claude on Justin's other account, or Codex). Self-contained: read this first, then `_program/STATE.md` (running log) and `_program/PROGRAM-BRIEF.md` §1 (every decision Justin has made — authoritative).

## 1. Where everything is

- Programme folder: `/home/justin/projects/Portico/.worktrees/audit-implementation/` (`$G`). Orchestration files in `$G/_program/` (`$PR`): `STATE.md`, `PROGRAM-BRIEF.md`, `briefs/`, `reports/` (one per lane), `drafts/`, scripts (`mergeq.sh` merges a lane branch into both bases).
- Integration branches (all named `audit-implementation/main`), each in its own worktree:
  - portico-server → `$G/main-server` — head `617dc62d`
  - portico-react-native → `$G/main-native` — head `a8c51ea`
  - portico-internal → `$G/main-internal` — head `2644ff9`
- **Nothing is pushed, merged into Justin's own branches, or deployed.** Justin's main checkouts are untouched and must stay so: portico-server `codex/performance-resilience`, portico-react-native `codex/android-testing-performance`, portico-internal `codex/ubuntu-demo-testing`.
- Lane worktrees `$G/server-<lane>` / `$G/native-<lane>` hold each lane's branch `audit-implementation/<lane>`; merged ones can be removed with `git worktree remove` once you're sure.

## 2. State at handoff

**Merged into all `audit-implementation/main` branches (15 Oct):** own media engine, licence screens, the first completion pass (browse parity, queues/playlists, owner parity, native playback incl. tuner setting, web playback, polish), follow-ups (PiP, seek-index resume, player-choices rule, collection order), and tonight's cp2/cp3 lanes: cp2-native, cp2-server, cp2-viewer, cp3-nscreens, cp3-docs, cp3-web, cp3-network, cp3-securedefault (+ Hosted doc on main-internal), cp3-nplay, cp3-api, cp3-outputs, cp3-identity. After the last merge: `go build`, `apigen` (no diff) and web/native/Vega type-checks were clean. **No full test suite has run on this merged state.**

**Stopped at 11:50 pm, NOT merged — finish, then merge (each has its own worktree and branch):**

| Lane | Branch / worktrees | State | What's left |
|---|---|---|---|
| cp3-bulkbuild — fast library build from scratch | `audit-implementation/cp3-bulkbuild` (`server-cp3-bulkbuild`; native has no changes) | 10 commits, main already merged in, clean | Was adding a test when stopped. Report `reports/cp3-bulkbuild.md` is written except the before/after numbers (§1 says "filled in at the end"); scale DBs in `$PR/scratch/cp3-bulkbuild` (2.7 GB). Re-run its 50k/200k measurement, fill the numbers, run `go test ./internal/compactcatalog ./internal/catalog ./internal/ingestion`, merge. Note: it edits storage-limit checks in the baseline schema. |
| cp2-reads — reads that grow with library size | `audit-implementation/cp2-reads` (`server-cp2-reads`, `native-cp2-reads`) | server 22 commits + 1 WIP commit (`1eed43f0`, unfinished); native 3 commits; **98 commits behind main** | Review the WIP commit, merge main into the branch, finish the before/after comparison (was writing the 50k comparison script; scratch `$PR/scratch/cp2-reads`, 2.9 GB), update `reports/cp2-reads.md`, merge. Scope: `briefs/cp2-lanes.md` "cp2-reads" + sweep items A22/A23. |
| cp4-ms — seconds → milliseconds on the wire (sweep A4, ~88 fields) | `audit-implementation/cp4-ms` (`server-cp4-ms`, `native-cp4-ms`) | **one WIP commit each** (server `e3c93b34`, 142 files; native `b603566`, 13 files) — unfinished, not type-checked | Finish the conversion (brief: `briefs/cp4-lanes.md`), make server/web/native/Vega type-check, regenerate contracts, write `reports/cp4-ms.md`. Merge it **last** (touches nearly every contract). |
| cp4-leftovers — tonight's codeable leftovers | `audit-implementation/cp4-leftovers` (`server-cp4-leftovers`, `native-cp4-leftovers`) | server 10 commits; native 11 commits + 1 WIP (`409df6f`, offline Forced only in progress) | No report yet: read its commit messages to see what's done, finish the WIP, then continue the list in `briefs/cp4-lanes.md` (merged-away ids at the API boundary, "Not this artist", certificate screen, History beyond 10,000, etc.), write `reports/cp4-leftovers.md`, merge. |

Suggested merge order: cp3-bulkbuild → cp2-reads → cp4-leftovers → cp4-ms, then the combined verification (§3 step 2).

## 3. What to do next, in order

1. **Merge any unmerged lane branches** listed in §2 (from `$PR`: `./mergeq.sh <lane> "<message>"`). Conflict patterns seen tonight and how they were resolved:
   - generated contracts (`packages/contracts/src/types.ts`, `schemas.ts`): take either side, then `cd main-server/server && go run ./cmd/apigen`;
   - native `packages/portico-server.lock.json` and any mirrored `packages/**` file: take the server's copy, then from `main-native`: `PORTICO_SERVER_REPO=$G/main-server node scripts/shared-packages.mjs --write` and commit;
   - i18n catalogue (`packages/i18n/src/catalog/en-US*.ts`): keep both sides (union); a lane deleted ~396 unused strings, so if a type-check says a key is missing, restore it from `reports/cp3-web.md`;
   - code conflicts: keep both lanes' intent (examples in STATE.md).
   After each merge: `go build ./...`, `go run ./cmd/apigen` (no diff), web `npx tsc --noEmit -p web`, native `npx tsc --noEmit -p apps/native`, Vega `npx tsc --noEmit -p apps/vega`.
2. **Combined verification** (nothing tonight ran full suites — Justin chose delivery over testing; expect tail-end bugs). Follow `reports/final-pass-2.md` exactly for how to run: full `go test ./...` with the playback helper built and `PORTICO_PLAYBACK_TEST_HELPER` set; apikit tests; Windows/macOS cross-builds; production web build; client-core; web `npm run check` (run `npm ci` in `web` first — new dev dependency oxlint); cast; contracts; native `npm test`, `lint:hooks`, `lint:strings`, type-checks; Vega; licence-list checks; the real-server e2e driver and the headless-browser driver; Android `:portico-mpv:assembleRelease` and `assembleGoogleDebug -PreactNativeArchitectures=x86_64` (media engine output in `$PR/scratch/own-mpv/out/android`, or rebuild with `third_party/media-engine/build-android.sh`). Fix what fails; never weaken a guard, bound, restriction matrix or security test.
3. **Owner's report**: `main-internal/Project Architecture/Audits/2026-10-11-comparative-audit/IMPLEMENTATION-REPORT-2026-10-14.md`. A draft of the new front (sections 1, 3, 4) is in `$PR/drafts/owner-report-front-draft.md` — fill the `@@…@@` placeholders with the verification results, add tonight's cp2/cp3/cp4 lanes (reports in `$PR/reports/cp2-*.md`, `cp3-*.md`, `cp4-*.md`), update Appendix D (27 of 38 open audit bugs closed by the 15 Oct completion pass; see `reports/remaining-sweep.md` for what remains), update §2 performance with `reports/cp2-reads.md` and `reports/cp3-bulkbuild.md`, commit on main-internal.
4. **Stock-take for Justin** (plain language, short): what landed, what's open (device/Mac work, Justin's pending items), choices lanes made that he may want to overrule (each lane report has "Decisions I made").

## 4. Still open after tonight (not codeable here, or needs Justin)

- **Devices / Mac:** nothing has run on a physical device; Swift/Objective-C is uncompiled. The Mac playbook (build the media engine first: `third_party/media-engine/build-apple.sh`, then pods, Xcode) is in the Apple test plans in `main-internal/.../test-plans/`. Mac-only items: Apple builds, iOS build-path fix, macOS FFmpeg toolchain rebuild (Justin approved, do it on the Mac), the licence-text fill script for 9 pods. Live TV through mpv is written but switched off behind `LIVE_CHANNELS` (Kotlin) / `liveChannels` (Swift) until a device check.
- **Justin later:** Hosted redeploy (needs an empty format-21 database), release signing keys, publishing source bundles before release.
- Full list with evidence: `reports/remaining-sweep.md` classes B (platform) and C (Justin), minus what he answered on 15 Oct (recorded in PROGRAM-BRIEF §1).

## 5. Standing rules (do not break)

- Git identity: Justin's human identity (configured). **No AI authors, co-authors, bylines or tool names in commit messages.** Never push unless Justin asks.
- Keep secrets out of Git; never copy values from `/home/justin/projects/Portico/.private`.
- Demo VPS `demo.getportico.tv`: no load tests; transcoding stays disabled there.
- Security trade-offs are deliberate (plain HTTP allowed except Apple remote, light passwords, no lockout, nothing encrypted at rest). Don't "fix" them.
- Product rules (details in PROGRAM-BRIEF §1): Settings decide every playback start, in-player changes last one playback; API keys are Plex-like (any tool, any origin, header or `?apiKey=` on GET); owners' own certificates are trusted; Apple plain HTTP only on the local network; secure connection by default with a Portico Account; tuner conflicts default to the recording failing; next episode during credits off by default; volume/mute is the system's except on web; no plugins/themes/custom CSS/push/email notifications/light theme/import-export/Roku/sub-path hosting.
- Pre-release: edit the baseline schema `0001_baseline.sql` (then `python3 scripts/schema-sync.py`), no migrations or compatibility shims. Every route declares an apikit scope. Per-item cost must stay flat as libraries grow.
- Disk: keep an eye on `df -h /`; delete per-lane Go caches (`$PR/scratch/gocache-*`) and scale databases when done.
