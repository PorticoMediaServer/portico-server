# Lane brief: remaining-sweep — one complete, verified list of everything not yet done

Justin asked, and must get a true answer: **is anything left other than work that needs other devices/platforms to test?** Previous answers were built from the owner's report summary and missed items recorded deeper in the record. Your job is to produce the one complete list, verified against the code. **Read-only on code: do not change any source; do not commit to any repo.** Write only your output files.

Paths: `G=/home/justin/projects/Portico/.worktrees/audit-implementation`, `PR=$G/_program`. Current code: `$G/main-server` and `$G/main-native` (branch `audit-implementation/main`), and `$G/main-internal`. Rules: do not spawn subagents or helpers; do the work yourself.

## Sources to sweep (all of them)
1. Every lane report in `$PR/reports/` (≈100 files): every "Not finished", "Needed from others", "Leftovers", "Left", "Follow-ups", "Remaining", "Untested"/"Not done" section, and any sentence that says something remains, is absent, is partial, is out of scope "for now", or was deferred.
2. `$PR/coverage-open.md` and `$PR/coverage-ledger.md` (items not marked done).
3. `$PR/reports/decision-audit.md` "Needed from others" (W*, N*, T*, V*, S* items).
4. The owner's report `$G/main-internal/Project Architecture/Audits/2026-10-11-comparative-audit/IMPLEMENTATION-REPORT-2026-10-14.md`: Appendix 0 (all gap lists and the decision table's "remaining limits" column), Appendix D (open bugs), Appendix E (choices).
5. `$PR/PROGRAM-BRIEF.md` §1 — every decision Justin made; check each is fully delivered on every client it applies to.
6. The original audit's decision register and report (`$G/main-internal/Project Architecture/Audits/2026-10-11-comparative-audit/00-FINAL-REPORT.md` and the domain docs) only to confirm scope of each decision.

## Already in flight — don't re-list as new, but mark them "in progress (cp2-<lane>)"
`$PR/briefs/cp2-lanes.md` lists what four running lanes are doing now (viewer gaps, native gaps, server gaps incl. B-13.10/B-04.35, growing reads). Also the final-pass-3 verification.

## For every item
- Dedupe across sources (many items appear in several reports; later lanes often fixed earlier lanes' leftovers).
- **Verify against the current code** whether it is still true (grep/read the code; check tests). Many old leftovers were fixed later — mark those `done (evidence: file/commit/test)`.
- Classify what remains:
  - **A — codeable now on Ubuntu** (server, web, Android, Vega JS, shared code; includes Apple Swift/ObjC code that can be written here even though it is compiled on the Mac — say so).
  - **B — platform/device-dependent**: can only be done or proven with a Mac/Xcode, a physical device, real hardware (GPU, tuner, receiver), real media we don't have, Windows/macOS/Docker installs.
  - **C — needs Justin**: a decision not yet made, approval (e.g. FFmpeg toolchain rebuild), credentials, signing, pushing/release.
  - **D — deliberately excluded** by Justin's decisions (cite which).
  - **E — in progress** in a cp2 lane.
- Be concrete: one line each, what is missing, where (client/server), source(s), evidence.

## Output
1. `$PR/reports/remaining-sweep.md`: summary counts per class first, then the A list grouped sensibly (server / web / native / cross-cutting) with a size estimate (S/M/L) each, then B, C, D, E, then the `done` list (brief) so it is auditable.
2. Reply with a short plain summary: counts per class, and the A list in one line per item.
Be thorough: missing an item is the failure mode Justin is worried about. When in doubt, list it with your evidence rather than dropping it.
