# Web and client-core lane report (portico-server, branch `audit-fixes/web-client-core`, base 82aaa54)

(Saved by the coordinator from the lane's final message.) 19 IDs: 18 fixed, 1 skipped. Paths relative to the worktree `.worktrees/audit-fixes/server-web-client-core`; "core" is `packages/client-core`. Nothing was checked in a browser, with assistive technology, or on a device.

Passing at head: `npm run test:core` 1477 (baseline 1454); `npm run check --workspace portico-web` exit 0 (tokens, lint-design --strict, preference consumers PASS 56 keys, lint-strings, tsc, 417 tests; baseline 406); `npm run test:cast` 16 (baseline 14); `go test ./internal/httpapi -run 'Cast|Receiver'` ok; `vite build` ok.

Files to mirror into the React Native copy: `packages/client-core/src/{group-session.ts,index.ts,playback-v1/http.ts,portico-servers.ts,server-connections.ts,setup-code.ts,sse.ts}`; `packages/i18n/src/catalog/{en-US.ts,en-US/web.ts}`.

REQUIRED NATIVE FOLLOW-UP for B-13.2: in `apps/portico-react-native/apps/native/src/app/together.tsx` (~line 32) add `seeking: !!s.pendingSeek` to the `state()` return; without it native keeps the old behaviour.

### B-01.9 — A 403 on token refresh deletes the stored sign-in — FIXED (403 only) — 060737e
- Found: `refreshStoredSession` (core `src/server-connections.ts`) deleted the stored record on any 401 or 403. Server: refresh answers 401 for a failed credential; its only family-ending 403 is `access_limit` (`access_admission.go:61`); the origin check (`router.go:1256`) is an unrelated 403.
- Change: a 403 ends the sign-in only with code `access_limit`; any other 403 becomes non-terminal `refresh_unavailable_now` (credential and pending request id kept). 401 unchanged.
- Tests: core `test/session-refresh.test.ts` (two new). Not verified: a real proxy/WAF 403.
- Left open: a 401 without Portico's envelope (e.g. from a gateway) still ends the sign-in; an existing test encodes that. Doc: comment atop `web/src/bridge/restore-policy.ts` still says "401/403".

### B-22.4 — Unreadable protected store classified as "sign-in replaced" — FIXED — dc5d0e6
- Change: a failed `storage.read()` now throws `SessionRefreshError('store_unavailable', …, signOut=false)`; the refresher retries on its existing backoff. A record that reads but is absent, another family, or unparseable still ends as before.
- Tests: core `test/session-refresh.test.ts` (two new). Not verified: Keychain, Keystore or IndexedDB failures on real platforms.
- RISK: `BrowserProtectedStore.read()` throws identically for busy, lock timeout, incomplete record and failed decrypt, so a permanently corrupt browser vault now retries (up to every 120 s) instead of signing out. Telling corrupt from busy needs adapter changes on both platforms.

### B-22.9 — Server-list watch wedges on a hung fetch — FIXED — 82d8403
- Change: version check and list fetch run under the watcher's own 30 s deadline (`SERVER_LIST_TIMEOUT_MS`); a race releases the flight even if the transport ignores its signal; timeout is `ApiError(504,'server_list_unavailable')` so the Hosted gate counts it.
- Tests: core `test/portico-servers.test.ts` (two new; both hung before).

### B-22.11 — HTTP-date Retry-After ignored — FIXED — af9f250
- Change: watcher uses shared `retryAfterSecondsFrom`; playback parser trims and parses the date form locally (avoids an import cycle).
- Tests: core `test/retry-after.test.ts`. Not changed: `src/playback-v1/sessions.ts:71` (202 "preparing" wait) still does `Number(header)`; an HTTP-date makes it NaN and the timer fires immediately.

### B-22.12 — Browser Hosted errors discard Retry-After — FIXED — 067b35a
- Change: `BrowserAccountApi.perform` (`web/src/bridge/account.ts`) passes the header through. Tests: `web/test/account-retry-after.test.ts`. Not changed: `account-api.ts` and `account-sessions-api.ts` do not report to the gate.

### B-22.13 — SSE frame cap does not bound accumulated data — FIXED — e6f9c63
- Change: the cap counts what is held for the current frame (data lines plus unfinished line); each line capped individually; complete frames parsed first; comments never count. Same 262,144 limit and error text. Tests: core `test/sse.test.ts` (four new).

### B-08.10 — Registry errors read as non-retryable — FIXED — 88abfbf
- Change: `error.retryable ?? (error.retry === 'same_request' || status >= 500)`. Tests: core `test/error-envelope.test.ts`.

### B-16.9 — Stale chunk after an upgrade — FIXED, UNTESTED ON a browser — a9ae9c8
- Change: `screenFileFailed(error)` recognises Chromium, Firefox and Safari failed-import messages and Vite's CSS preload error; for those, Try again reloads the page. No automatic reload (could interrupt playback).
- Tests: `web/test/route-error-reload.test.ts`.
- Test note: build and serve the web app, open Home in Chrome, Firefox and Safari. Rebuild so hashed names change and replace the served assets. Without reloading, open Settings. Pass: error screen appears and Try again reloads into Settings. Fail: Try again shows the same error. Confirm the console message matches one of the four patterns.

### B-16.13 — No main landmark or skip link — FIXED, UNTESTED ON a browser / screen reader — 40748d2
- Change: content wrapper is `<main id="main-content" tabIndex={-1}>`; a "Skip to content" link is the first element of the frame, off-screen until focused; focuses the main region directly so the address does not change. New string `web.shell.skipToContent`.
- Tests: `web/test/shell-landmarks.test.ts`.
- Test note: signed in, desktop and phone widths, press Tab once from the address bar. Pass: visible "Skip to content" top-left; Enter moves focus into the page; next Tab lands on the first page control, not the rail; URL unchanged. With VoiceOver or NVDA the landmarks list shows one "main". No focus ring around the whole page.

### B-16.14 — Same document title everywhere — FIXED, UNTESTED ON a browser / screen reader — d6f07cd
- Change: new `web/src/app/page-title.tsx`, mounted once in the root route. Page name is the main region's `h1`, or a `data-page-title` attribute (Home sets it). Title becomes "{page} · Portico". A polite live region announces the name once per pathname change (not on first load, not for query-only changes). Read from the DOM by a body-wide `MutationObserver` throttled to 250 ms. New string `web.shell.pageTitle`.
- Tests: `web/test/page-title.test.ts`.
- Test note: visit Home, a library, a film, a show, Settings, a settings section, Search and sign-in. Pass: each tab title names the page; history entries distinguishable. With a screen reader each navigation is announced once; a refresh or tab change is not; a profile switch never speaks the previous profile's page. Fail: stale title, repeated announcements, or jank while scrolling a large grid.

### B-16.15 — Card name is the title only — FIXED, UNTESTED ON a screen reader — 78243e2
- Change: `cardName(title,{watched,progress})` gives "Dune, Watched", "Severance, 40% played", or both; the tick is `aria-hidden`. New strings `card.nameWithState`, `card.progress`.
- Tests: `web/test/card-name.test.ts`.
- Test note: with VoiceOver or NVDA, Tab onto an unwatched, a watched and a half-watched card. Pass: "Title, link", "Title, Watched, link", "Title, 50% played, link". Fail: state missing or read twice.

### B-17.9 — Player shortcuts fire with modifiers — FIXED, UNTESTED ON a browser — cdc964a
- Change: new `web/src/player/shortcut-key.ts`: nothing when Ctrl, Cmd or Alt is held (AltGr excepted) or during composition; lower-cases single letters so Caps Lock works.
- Tests: `web/test/player-shortcut-key.test.ts`.
- Test note: with the player open on Windows and macOS, Ctrl/Cmd+L focuses the address bar without seeking, Ctrl/Cmd+F opens find without fullscreen, Alt+Left goes back. Plain k, j, l, f, m, space, arrows, ? and Escape still work, including with Caps Lock on.

### B-18.1 — Group tabs ignore which group was clicked — FIXED (browser not exercised) — 92ecf29
- Found: `web/src/app/detail.ts` hard-coded `service.selectGroup('unassigned_absolute')`. Change: the hook takes the id. Tests: `web/test/show-group-selection.test.ts`.
- Check later: a real show with "Hide seasons" or "Split into ranges of 100": each tab and the dropdown loads that group's episodes.

### B-18.2 — Tags or Labels edit dropped when already locked — FIXED (browser not exercised) — e30294b
- Change: `edit.values !== undefined` counts as a change (`web/src/screens/shared/MetadataEditor.tsx`); `GeneralTab` exported for the test. Tests: `web/test/metadata-editor-validation.test.ts`.

### B-18.11 — Preference-consumer check silently skipped — FIXED — e5b18fb
- Found: core `scripts/check-preference-consumers.mjs` looked for `apps/apple/src` and exited 0 at import when missing (which also made its unit tests exit before running).
- Change: path is `apps/native/src`; nothing exits at import; with no native checkout the check still runs over this repository and says so; an explicitly set `PORTICO_REACT_NATIVE_REPO` that does not exist fails; exceptions list is empty.
- Result: `PASS: 56 keys, 0 documented exception(s)`; no preferences without consumers surfaced. Tests: core `test/preference-consumers.test.ts` now runs 7 tests (was 0).
- Caveat (lane note): the ratchet is weak — `server/internal/operations/preference_consumers.go` and core `settings-structure.ts` name nearly every key and count as readers, so a setting with no real effect would still pass.

### B-18.4 — Genres and Series tabs stop at 200 — SKIPPED
- Reason: the server offers no paging for these lists (`browseAggregate`, `browse_engine.go:968-988`, `browseFacetLimit = 200`, no cursor). Needs server work first.

### B-13.7 — Casting stops when the last sender disconnects — FIXED, UNTESTED ON a real Chromecast — bfcd24d
- Found: `server/receiver/cast/receiver.js` `SENDER_DISCONNECTED` queued `stopPlayback` when no senders were left; contract §7.1.7 and §7.3 say sender departure does not end playback.
- Change: the last-sender branch clears the controller but no longer stops. Tests: `server/receiver/cast/test/receiver.test.cjs` (two new).
- Test note: on a registered Chromecast or Google TV, cast a film from a phone, then lock the phone, turn its Wi-Fi off, or force-quit the app. Pass: the film keeps playing past ten minutes, progress still appears on the server, receiver returns to idle after the film ends. Fail: playback stops shortly after the phone drops.
- Related gap (not fixed): a returning phone gets `pair-required`, and pairing again stops the film (`takeControl` → `switchTo` always calls `stopPlayback()`); contract §7.3 says a reconnect resumes control without reloading.

### B-13.2 — Slow follower seek restarted every three seconds — FIXED for web, UNTESTED ON a real slow seek — 92d46e0
- Change: `GroupPlayer.state()` gains optional `seeking`; while true the follower issues no seek and no rate change (bounded by the playback service's 15 s pending-seek timeout). Web adapter sets `seeking: !!s.pendingSeek`. No protocol, readiness or revision change.
- Tests: core `test/group-session.test.ts`.
- Test note: two members in a room; one plays a converted stream on a throttled link so a seek takes over three seconds. Host seeks; log `seekRevision` on the slow member. Pass: one revision per host seek, then playback resumes. Fail: revisions climbing every three seconds.
- Native needs the one-line adapter change noted at the top.

### B-22.26 — Activation polling retries at a fixed 5 s — FIXED (cadence only) — d2723d7
- Change (core `src/setup-code.ts`): one `backoff(n)`: 5 s doubling to a 30 s cap with up to 20% jitter below the cap, for create failures and unanswered polls; any protocol answer or new code resets the counter; larger `Retry-After` still honoured; `poll`'s envelope and key checks untouched.
- Tests: core `test/networking-review.test.ts` (three new). Left open for Justin: whether activation should report to or obey the shared Hosted outage gate.

## Commits (`git log --oneline 82aaa54..HEAD`)
d2723d7, 92d46e0, bfcd24d, e5b18fb, e30294b, 92ecf29, cdc964a, 78243e2, d6f07cd, 40748d2, a9ae9c8, 88abfbf, e6f9c63, 067b35a, af9f250, 82d8403, dc5d0e6, 060737e (newest first).

## Noticed outside the list (not fixed)
1. Cast reconnect stops the film on re-pair (see B-13.7).
2. `playback-v1/sessions.ts:71` NaN wait on an HTTP-date Retry-After.
3. Preference ratchet is weak (see B-18.11).
4. Corrupt browser vault: `BrowserProtectedStore.change()` decodes before writing, so a corrupt record cannot be overwritten by a new sign-in either.
5. Refresh 401 from a gateway still ends the sign-in.
6. Home's `h1` is the featured title, not "Home".
7. Card `unavailable` state is conveyed only by dimming.
8. Core `test/group-session.test.ts` hangs ~60 s when an assertion fails (service not disposed).

---

# After review (Codex review: reviews/review-web-client-core.md) — six follow-up commits

| Commit | Reviewed commit | Findings addressed |
|---|---|---|
| `c09c209` End a sign-in whose saved record is damaged for good, and let the next sign-in replace it | dc5d0e6 (B-22.4) | Blocker |
| `29af71d` Bring a Watch Together follower whose seeks are slow back to the group, and leave a buffering one alone | 92d46e0 (B-13.2) | Both blockers + should-fix (buffering) |
| `62d53ff` Count an event stream frame's name and id against its cap, and read a CRLF split between reads as one line ending | e6f9c63 (B-22.13) | Should-fix 1, 2 |
| `8ba1919` End unattended Cast playback when the title finishes without a final report or the TV's pairing is revoked | bfcd24d (B-13.7) | Should-fix 1, 2 |
| `9208a29` Keep activation retries spread out once they reach the 30 second cap | d2723d7 (B-22.26) | Should-fix 1 |
| `e07822b` Announce a destination only by its own name, and never across a change of viewer | d6f07cd (B-16.14) | Should-fix 1, 2 |

Checks: `npm run test:core` 1487; `npm run check --workspace portico-web` exit 0 (428 tests, lints, tsc, preference consumers PASS 56); `npm run test:cast` 18; `go test ./internal/httpapi -run 'Cast|Receiver'` ok; `vite build` ok. Nothing run in a browser, on a Chromecast or on a native device.

- B-22.4 (`c09c209`): a store can say a record is damaged for good by throwing an error with code `invalid_saved_connection`; core ends the sign-in at once as `saved_sign_in_damaged` (server not asked, record not touched); any other read failure still keeps the sign-in and retries. Browser vault reports bad JSON, a missing key, failed authentication and a wrong shape as damaged; its `change()` treats a damaged record as absent so the next sign-in saves over it. Web restore policy sends a damaged saved connection to the sign-in screen.
- B-13.2 (`29af71d`): the follower waits for its own seek to land, measures how far behind the moving group it landed and aims the next seek that far ahead (capped at 15 s). With 8-second seeks: two seeks, then within 0.75 s. An adapter that does not report `seeking` is handled (seek counts as landed once the reported position moves, 20 s bound). Corrections skipped while buffering; a member with a pending seek reports `buffering`. Also changes the "nudge four seconds then seek again" loop (B-13.3). No group-wide wait, no revision change.
- B-22.13 (`62d53ff`), B-13.7 (`8ba1919`: local teardown runs even if the final "ended" report fails; a reconnect refused with `cast_device_revoked` / `cast_device_not_found` / `cast_device_expired` or a changed scope stops playback locally), B-22.26 (`9208a29`: waits 5–6, 10–12, 20–24, then 24–30 s), B-16.14 (`e07822b`: `PageHeader` `provisional` flag; a viewer change announces nothing).
- Finding rejected: put Hosted activation behind the shared outage gate — not done (pending Justin; the gate's ~21-minute back-off conflicts with the contract's 30 s cap). B-22.26 = PARTIALLY FIXED: cadence done, gate participation open.

## Files changed under packages/ (final, for mirroring; native mirrors `src` only)
`packages/client-core/src/`: group-session.ts, index.ts, playback-v1/http.ts, portico-servers.ts, server-connections.ts, session-refresh.ts, setup-code.ts, sse.ts. `packages/i18n/src/catalog/`: en-US.ts, en-US/web.ts.

## Native adapter changes needed (React Native repo; not made by this lane)
- B-13.2 — `apps/native/src/app/together.tsx` line ~32: replace the `state()` return with:
  `return {itemId: s.itemId ?? '', positionSeconds: s.pendingSeek?.positionSeconds ?? s.positionSeconds, playing: s.phase === 'ready' && s.intent === 'playing', buffering: !!s.recovering || s.phase === 'starting', ready: s.phase === 'ready', seeking: !!s.pendingSeek};`
- B-22.4 — until done, native keeps a damaged record and retries (destroys nothing but never ends): (1) `apps/native/src/bridge/queue-storage.ts:11`: when `JSON.parse(raw)` throws for the connection record (`network.selected.v1`), reject with `Object.assign(new Error('Saved server credentials are incomplete.'), {code: 'invalid_saved_connection'})`; (2) `apps/native/src/bridge/server-connections.ts:29-36`: in `change`, treat a damaged record as absent (call `update(undefined)`), write when the update returns a record, leave untouched otherwise; (3) native modules must split "cannot read now" from "damaged" with its own rejection code mapped to `invalid_saved_connection` (Android `PorticoRuntime.kt:153` currently rejects both as `secure_storage`; Apple: `PorticoSecureStorageRead.h` not read by the lane).

## Revised test notes
- B-22.4 (browser): sign in, then in DevTools open IndexedDB `portico.server-connections.v1` → `vault` and (a) delete `key`, or (b) replace `selected` with `{version: 2, plain: '{broken'}`. Wait for a renewal or reload. Pass: the sign-in screen for that server appears within about two minutes (immediately on reload); signing in works and survives another reload. Fail: "Can't reach" for ever, or sign-in fails to save. Separately, block IndexedDB briefly (hold a version-change open in another tab): the session stays signed in and renews once unblocked.
- B-13.2: two members; one on a converted stream with a throttled link so seeks take over three seconds. Log seek target, landing position, `seekRevision`, drift and reported readiness for 60 s after a host seek. Pass: no new revision while one is pending; at most one or two further corrective seeks, each landing closer; drift under 0.75 s afterwards; readiness `buffering` during the seek, then `ready`. Fail: a corrective seek every seek-length for ever. Also seek within 15 s of the end of a title (the lead can aim past the end; check it does not end early in a way that matters). Run on web and on native video, with and without the adapter line.
- B-13.7 (Chromecast): as before, plus: cut the receiver's network just before the title ends → TV returns to idle; revoke the Cast device from the server while it plays unattended → playback stops within roughly a report interval; drop only the network mid-film → playback continues and reporting resumes.
- B-16.14: as before, plus: open a library on a slow connection → the stand-in title is never spoken and the real name once; go from one film to another → the first film's name is never spoken for the second; switch profile while on a page → nothing is announced. Known gap: two consecutive pages with the identical name in the same element produce no announcement.
- B-22.26: during a simulated Hosted outage on a TV, log retry times. Pass: about 5, 10, 20 s, then 24–30 s, differing between two devices, with the code staying on screen.

---

# Round three (after review file reviews/review-round2-clients.md)
- `4090364` "Never aim a Watch Together seek near the end of a title, and wait out a seek the player does not report on" — 29af71d blockers 1 and 2 and should-fix 3. `GroupPlayer.state()` gains optional `durationSeconds`; a seek is aimed ahead only if that leaves at least as much of the title as the lead itself and never less than five seconds, otherwise it goes to where the group is. A player that does not report `seeking` gets a fixed 20-second wait after each seek and its seeks are never aimed ahead (safe, but does not converge for slow seeks). Lead cap equals the 20-second wait. A failed seek teaches nothing. A host move, pause or resume during a pending seek re-judges the member at once. A member reports `buffering` only when its player says it is seeking or buffering.
- `e2bfc4c` "Replace a stored vault key that cannot decrypt along with the record it guards" — c09c209 should-fix 2 (browser vault key of the wrong kind is treated as damage).
- Not taken: e07822b page-announcement should-fixes (a heading that appears and is left within one 250 ms window can still be announced for the next page; two consecutive pages with the same name produce no announcement) — needs each data-bound screen to stamp its heading with its resource (~7 screens); cannot be checked without a browser.
- Checks: test:core 1492; web check exit 0 (431 tests); test:cast 18; vite build ok.
- Mirror list unchanged (only `group-session.ts` changed this round).
- Native adapter change needed (updated): `apps/native/src/app/together.tsx` `state()` return must report BOTH `seeking: !!s.pendingSeek` and `durationSeconds: s.duration > 0 ? s.duration : undefined` (field name `duration` unconfirmed on native engine state).
- Revised test notes: (a) slow seeks: pass = no new request while one is pending; at most two or three corrective seeks, each landing closer; drift under 0.75 s afterwards; `buffering` during the seek then `ready`. (b) End of title: with the group inside the last 30 s, the slow member never finishes before the group; near the end its seeks go to the group's position, not beyond. (c) Native before the adapter line: one seek about every 20 s, never one while another is pending (safe fallback, not expected to converge). (d) Host seek/pause/resume during a member's pending seek: the member follows the new position at once with no burst of seeks. (e) Browser vault with a bad key: replace the `key` entry in IndexedDB `portico.server-connections.v1` → `vault` with a key of another algorithm, reload: sign-in screen appears, signing in works and survives another reload.
