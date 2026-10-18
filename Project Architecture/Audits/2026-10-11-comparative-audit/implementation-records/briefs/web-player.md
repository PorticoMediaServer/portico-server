# Lane: web-player — finish the move to the new web playback controller

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-web-player` (branch `audit-implementation/web-player`). You can commit. `node_modules` is a symlink; do not run `npm install`.
Audit documents: 17 (all), 16 where it touches the player; verification V09 (including N-V09.1). Decision register D-12, D-22.

## What to deliver
1. The web app ships the older player adapter, without behavioural tests, while the stricter controller with the tests is not wired in (A-17.35, B-17.25). **Wire the new controller in as the only web playback path and delete the legacy adapter** and everything that exists only for it. Fix N-V09.1 in the same work.
2. The web player uses Playback Protocol v1 only. Remove any use of the legacy session-start route; the server lane is deleting it.
3. Fix every confirmed web player bug in document 17 that is still present, including: quality, version and audio changes made in the player reach the server and keep the queue (B-17.1); B-17.2, B-17.5, B-17.8; fragment and stall timeouts that turn "slow" into "failed" become progress-based (B-17.4): a fragment that is still receiving bytes is not failed.
4. The fuller keyboard map (A-17.30) and the player comforts that are cheap on web.
5. A quality menu that matches the product decision: Original (default), Automatic, then the fixed qualities; plus a saved default for home and for remote in settings. Automatic on web: measure throughput from fragment downloads, start at Original, ask the server to step down on sustained shortfall or repeated stalls and back up after sustained headroom, with hysteresis so it does not flap.
6. An honest, plain error presentation when media cannot be played at all.

## Boundaries
- The server's **playback-core** lane is replacing the capability document and adding a re-plan operation and the quality values at the same time as you work. Build against the protocol as it is in your worktree, keep all protocol calls behind the existing `client-core` playback services so they can be re-pointed, and list in your report every place that will need to change.
- A later lane adds on-device rendering of styled and image subtitles on web. Leave a clean seam: one place where a subtitle track is attached to the video element.
- The **web-app** lane owns every non-player screen and the non-player parts of `client-core`. You own the player screens and the playback modules of `client-core`.
- Type-check with `npx tsc --noEmit` in `web`; run only the player's own test files.
