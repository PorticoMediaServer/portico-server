# Lane: social — Watch Together, casting and "Play on"

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-social` (branch `audit-implementation/social`). This is your working directory and, with `/home/justin/projects/Portico/.worktrees/audit-implementation/_program`, the only place you can write.
**You cannot commit** (the sandbox keeps git metadata read-only). At each milestone, when the tree compiles, run from the worktree root:
`bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh social "<one-line commit subject in plain language>"`
Snapshots are cumulative; each becomes one commit. Take one at least every hour of work and always one at the very end, after which you must not change the tree. Go environment: `GOCACHE` and `GOFLAGS=-p=2` are already set.

Audit documents: 13 (all); verification V08, G2. Decision register D-45. `fix-pass/lane-reports/report-social-downloads.md` and `FIX-LOG-2026-10-12.md` say what the fix pass already did here.

Justin's position: Watch Together and casting are not where he wants design effort. Fix the defects so they work reliably; do not redesign and do not add features.

## What to deliver
1. **A short bounded wait after a seek or a title change** (D-45, B-13.2): today nobody waits for anybody, so members drift. After a seek or title change the group pauses until every active member reports ready or a few seconds pass, whichever is first; a slow or absent member never holds the group beyond the bound, and catches up when they arrive.
2. **No conflicts from unrelated activity**: a member's heartbeat must never make the host's command fail (B-13.1; check the fix log, then confirm no other counter in the group has the same fault). A person must never see a conflict unless the thing they acted on really changed; split counters where one catch-all revision is doing several jobs.
3. **Casting**: casting stops when the phone disconnects (B-13.7); it must continue, with the phone able to reconnect to the same session and control it. Every other confirmed casting defect in document 13.
4. **"Play on"** moves playback to another device but cannot then control it: make the controlling device able to pause, resume, seek and stop what it started, through the existing session-control path.
5. Every other confirmed bug in document 13 that is still present, on the server, in `packages/client-core` (non-screen code) and in the Cast receiver code in this repository if it lives here.

## Boundaries
- You own the `social` package and its routes, the casting and remote-control server code, and the matching non-screen modules of `client-core`.
- The **accounts-security** lane is changing what authority a paired Cast target holds (a narrow playback-only authority instead of a full session). Do not change pairing authority; keep to behaviour.
- The **playback-core** lane owns playback sessions, the planner and the capability document and is reworking them now; use the session-control operations as they exist in your worktree and keep your calls to them in one place.
- The **identity** lane owns personal state (progress, watched). Do not change how progress is stored.
- Screens are adapted by client lanes afterwards: list exactly what each client must do.
