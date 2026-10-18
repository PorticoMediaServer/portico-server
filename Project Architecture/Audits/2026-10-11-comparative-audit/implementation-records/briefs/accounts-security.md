# Lane: accounts-security — genuine security defects, members, restrictions

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-accounts-security` (branch `audit-implementation/accounts-security`). You can commit.
Audit documents: 01, 15; verification V01. Decision register D-36 to D-40, D-58, D-59. Also the open questions Q-4, Q-5, Q-7 in `FIX-LOG-2026-10-12.md` and `fix-pass/lane-reports/report-identity-ops-live.md`.

The deliberate trade-offs stand and must not be "fixed": light password rules, no lockout, nothing encrypted at rest, a TV code giving the TV the approver's role. Plain HTTP is now allowed from anywhere (the networking lane does that).

## What to deliver
1. The owner's remote sign-in policy ("owner only" / "off") is enforced on the sign-in route clients really use, and on every other way a session can be obtained (B-01.2 / B-15.1).
2. A television or other device paired as a **Cast target** gets a narrow playback-only authority, not a full session with the pairing person's role (B-13.14).
3. When a server learns a Hosted signing key was revoked, sessions admitted under that key end (N-V01.1). Record which key admitted each session; the schema is free to change.
4. **Restore** does not bring revoked sessions back to life: everyone is signed out on restore and live secrets are not usable from a backup copy (B-09.19, B-15.5), without adding any step for the owner.
5. **One rating engine** for accounts and profiles, with one scale, so "PG" does not hide "TV-PG" (A-01.5, B-01.10). Per-library "block unrated" if it falls out naturally.
6. **Permission changes apply live** instead of signing the person out everywhere; sign out only when access is removed (D-40).
7. **Delete a member** (D-58): today a member can only be disabled. On deletion, their play history rows keep the plays with the name blanked.
8. Device approval: the setting exists, saves and does nothing (B-01.4). Wire it.
9. Invitations: decide and implement the safe behaviour for re-admitting a disabled member, record when and why a member was disabled, and bind an invitation to its recipient where that is cheap (Q-5).
10. The remaining confirmed bugs in documents 01 and 15 that are not network-layer and not API keys.
11. In your report: the exact wording corrections the Server architecture document needs where it claims device keys that do not exist (D-37), and the plain sentence the privacy screen should show about the owner seeing every play (D-59).

## Boundaries
- **api-platform** owns API keys, personal tokens, webhooks and MCP (B-01.3 is theirs).
- **networking** owns the local-or-remote test, proxy headers and client address parsing (B-15.3, B-15.6, B-15.11) and first-run protection (B-15.4). Use one function for "is this request remote" and do not add another.
- **ops-packaging** owns backup and restore mechanics; you own what a restore does to sessions and secrets. Keep your edits there narrow.
- Update `packages/client-core` for changed shapes; list the screens needed.
