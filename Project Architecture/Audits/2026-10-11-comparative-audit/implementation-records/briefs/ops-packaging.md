# Lane: ops-packaging — restart, installers on four platforms, backup and restore, updates, release

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-ops-packaging` (branch `audit-implementation/ops-packaging`). This is your working directory and, with `/home/justin/projects/Portico/.worktrees/audit-implementation/_program`, the only place you can write.
**You cannot commit** (the sandbox keeps git metadata read-only). At each milestone, when the tree compiles, run from the worktree root:
`bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh ops-packaging "<one-line commit subject in plain language>"`
Snapshots are cumulative; each becomes one commit. Take one at least every hour of work and always one at the very end, after which you must not change the tree. Go environment: `GOCACHE=/home/justin/projects/Portico/.worktrees/audit-implementation/_program/gocache` and `GOFLAGS=-p=2` are already set.
Audit documents: 09 (operations entries), 25 (all), 05 (B-05.28 follow-up); verification V06. Decision register D-34, D-33, D-56, D-57, D-47, D-09 (database space), D-10. Also `fix-pass/lane-reports/report-platform-packaging.md` (the Windows acceptance procedure and open question Q-9).

Release targets for the server: Windows, macOS, Linux and Docker, all first-class.

## What to deliver
1. **The server restarts itself** (D-34). After a restore, an update or a setting that needs it, the server comes back on every platform without a service manager's help, including Windows and plain Docker (B-09.2). A small supervising parent that re-launches the worker is acceptable; so is re-exec. Offer Restart as an owner action.
2. **Windows as a real service** rather than a scheduled task with awkward defaults (B-09.4, B-09.5): install, start at boot, stop, upgrade and uninstall cleanly; state folder location and its wording; the installer script reviewed line by line since it has never been compiled. Settle Q-9: library roots that are junctions, symbolic links or mounted folders; the silent job-object breakaway in `childprocess_windows.go`.
3. **macOS**: a proper launchd service and package, state location, upgrade path. **Linux**: `.deb` and `.rpm` upgrades leave the service running (check the fix log), systemd unit hardening that does not break hardware conversion. **Docker**: an image that works with no service manager, sensible volumes, health check, and the qualified FFmpeg input (B-25.4, check the fix log).
4. **Backup and restore**: the 10-second integrity check on restore becomes progress-based (B-09.1); optional separate destinations for cache and for backups (D-33) while one folder stays the default; a copy of the database before a start that has migrations to apply (D-56), bounded and pruned.
5. **Database space reclamation** (D-09): choose incremental reclamation now, set where the database is first created, with a paced background step, so space comes back after large deletions without a stop-the-world vacuum. Pre-release databases need no upgrade path.
6. **Update notices** (D-57): a default update feed, opt-out, and the feed's shape matching what the server reads (B-09.11 / B-25.1).
7. **Crash and log reports to the owner's own server** (D-47): a route that accepts a bounded report from a signed-in client (app version, device class, a short log), stores a bounded number, and lets the owner list, read, download and delete them. Client lanes send them later.
8. **Release engineering**: the Windows ZIP built from the wrong path (B-25.3, check the fix log), manual release CI testing a different revision (B-25.19, check the fix log), and every other confirmed item in document 25 that is still present. Scripts and workflows only; do not run a release.
9. Every other confirmed operations bug in document 09 that is still present.

## Boundaries
- You own `operations`, `backup`, `supervise`, `childprocess`, `diagnostics`, `servicelog`, `packaging/`, `scripts/`, CI workflows and `cmd/server` process lifecycle. The **networking** lane owns listeners, certificates and proxies. The **identity** lane is removing the state-folder trash and owns deletion. The **accounts-security** lane owns what a restore does to sessions.
- Cross-compile for `GOOS=windows` and `GOOS=darwin` after every platform change. Nothing can be run on Windows or macOS here: write the acceptance steps for each platform, install through upgrade through uninstall.
