# Lane: web-app — shared client behaviour and the non-player web app

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-web-app` (branch `audit-implementation/web-app`). You can commit. `node_modules` is a symlink; do not run `npm install`.
Audit documents: 16, 18, 22 (web and shared parts); verification V09, V10. Decision register D-02 (client parsing), D-25, D-27.

## What to deliver
1. **Tolerant client parsing** (B-16.10): the strict response parsers in `client-core` reject a whole document on one unknown enum value, so a newer server breaks an older app. Unknown enum values, unknown fields and one bad row must not reject the document: drop or neutralise the smallest unit and keep the rest. Keep the bounds.
2. **Remembered choices** (D-25): a library reopens on the tab the person last used, with their last sort and filter. Implement the behaviour in `client-core` so the native apps get it too, stored per profile on the device, and wire it on web.
3. **Accessibility that is not cosmetic**: a main landmark and skip link (B-16.13), page titles that follow the screen (B-16.14), cards that announce watched and in-progress state (B-16.15), and the remaining items of that kind in documents 16 and 18.
4. **Language catalogue**: about fifteen web screens bypass the shared catalogue (B-16.6, B-18.12). Move them onto it. Remove strings that are truly unused (B-18.27), carefully.
5. **Phone width**: the web app loses Switch profile, Switch server and Feedback at phone width (B-18.14). Restore them.
6. "See all" destinations are guessed from a row id (B-18.28); use what the server sends, and if the server does not send it, record exactly what field is needed.
7. Settings labelled "This device" that really apply to every device of that class (B-18.19, B-21.7): relabel plainly ("All phones", "All TVs", and so on) in the shared settings structure.
8. The browse-capabilities cache with no revision key (B-16.19), and every other confirmed bug in documents 16 and 18 that is still present and not a player bug.
9. A quiet, one-line note on the sign-in screen when the page is loaded over plain HTTP from a non-local address, with a link to how to change it. No blocking, no dialogs.

## Boundaries
- The **web-player** lane owns the player screens and the playback modules of `client-core`.
- Server lanes are changing API shapes at the same time and will update the `client-core` parsers and services for their own routes. Avoid restructuring service files you do not need to touch.
- Later lanes build the new owner console screens (API keys, webhooks, providers per library, trash, limits) and the live-refresh consumer. Do not start those.
- Type-check with `npx tsc --noEmit`; run only the test files for what you change, plus `npm run lint:strings` if it is quick.
