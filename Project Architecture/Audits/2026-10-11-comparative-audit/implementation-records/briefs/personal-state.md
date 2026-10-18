# Lane: personal-state — watched, progress, history, Home and discovery

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-personal-state` (branch `audit-implementation/personal-state`). This is your working directory and, with `/home/justin/projects/Portico/.worktrees/audit-implementation/_program`, the only place you can write.
**You cannot commit** (the sandbox keeps git metadata read-only). At each milestone, when the tree compiles, run from the worktree root:
`bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh personal-state "<one-line commit subject in plain language>"`
Snapshots are cumulative; each becomes one commit. Take one at least every hour of work and always one at the very end, after which you must not change the tree. Go environment: `GOCACHE` and `GOFLAGS=-p=2` are already set. Your report is `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/personal-state.md`.
A rule added since the first lanes: every route on the registry must declare the key scope it needs (`Requires: apikit.ScopeRead | ScopePlayback | ScopeLibrary | ScopeAdmin | ScopeSession | ScopeOpen`; see `apikit/registry.go`), or the server will not start. After changing routes run `go run ./cmd/apigen` from `server/` to regenerate the contract files; after changing the schema run `python3 scripts/schema-sync.py` from the root.

Audit documents: 12 (all); verification V05, G2. Decision register D-60, D-61. Read `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/identity.md` first: personal state now attaches to the title, not the file, survives a title's removal by provider identity, and that work is in your worktree. Also `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/web-app.md` (it needs a field on Home rows) and `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/music-audiobooks.md` (listening rules already done).

## What to deliver
1. **"Mark show watched" does what it says** (D-60, B-12.1): it also finishes episodes that are in progress, and there is a real Undo that restores exactly the previous state (progress included) for a short while. The same for a season, and for "mark unwatched".
2. **Rewatching** (D-61): unanswered by Justin, so the conservative reading applies: keep the current contract's behaviour, but make sure play counts and last-watched are right and that a rewatch in progress appears in Continue Watching.
3. **Home rows say where "See all" goes**: each composed Home row carries an explicit destination (`seeAll`: the library, tab, sort and filter, or the list) so no client guesses from a row id (B-18.28). Add it to the document and the `client-core` parser and types.
4. **Continue Watching and Up Next that behave like Plex**: the next episode appears when one finishes; a started film is offered until finished or removed from the row; removing from the row sticks; a title with a missing file is not offered; specials and multi-version titles do not confuse "next".
5. **History**: offline plays and plays from every engine reach history with the right time (a downloads lane is sending them through the normal path); the two histories stay consistent; the owner's view and the viewer's own controls behave as the contract says.
6. **Hidden and restricted titles never leak** through Home, search suggestions, recommendations, history or "because you watched" (the fix log notes hidden-title redaction needs coordinated work: do the server side fully and say exactly what each client must do).
7. Every other confirmed bug in document 12 that is still present on the server or in non-screen `client-core` code.

## Boundaries
- You own `personalstate` behaviour (not its storage shape, which the identity lane just set), the Home composition, recommendations and history code, and their routes.
- Running at the same time: **freshness-server** (revision counters and event publication — it will publish personal-state changes; keep your writes going through the existing write path so it can hook them), **scan-loop**, **search-sort**. Do not change counters, events, scanning, or sort and search text.
- A **downloads** lane owns offline progress upload; a **social** lane owns Watch Together.
