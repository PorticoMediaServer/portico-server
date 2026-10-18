# Lane report: web-owner — the owner console's remaining screen work

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/server-web-owner`, branch `audit-implementation/web-owner`, 13 commits on top of `6f1530a` (`e4309a4` … `00e6a37`). Nothing pushed, nothing deployed. **No server code changed** (no file under `server/` or `apikit/`). 64 files, +2140 −429.

**How it was checked.** `npx tsc --noEmit` clean in `web/` and `packages/client-core`. `lint:strings` (0 hard-coded strings, baseline 0), the raw-error and glossary lints, `lint:design --strict` and the preference-consumer check all pass. Test files run and passing: 16 client-core files (162 tests; the new `quiet-read` and `now-playing-transport` files, and extended `freshness-actions`, `console`, `api-words`, `settings-console-pages`, `storage-management`, `remote-sources`, `metadata-attention`) and 25 web files that touch a changed screen. One web test fails and failed before I started: `web/test/now-playing.test.ts` "the server's session.updated payload reaches core as an owner's stop" (the fake playback server builds a route with an unencoded id; confirmed with my changes stashed). Whole suites were not run.

**The browser driver ran for me.** I built the web app and the server and ran `web/test/integration/browser-smoke.mjs` once in full (loopback only, the credentials it generates for itself, fresh state under my scratch directory): every group passed and it exited 0, on the tree as of `ffc4160`. For the later commits I rebuilt the web app and looked at specific owner pages and dialogs on that same disposable server with a small script of my own (kept in scratch, not in the repository), at 1440 and 390 px: Integrations, People (accounts, an account with viewing hours, devices), Live TV sources and recording, Storage & backups top to bottom, Play history, Troubleshooting, the delete-backup and restore dialogs, the add-source dialog on the HDHomeRun tab. Three things that look showed were fixed (stacked footers under the keys list, "last seen Just now" capitalised mid-sentence, a phone icon on devices of unknown kind). **Not seen in a browser:** Watching now with a real session on it (nothing was playing during the look), the playback-problems panel with rows, the episode-order controls in Edit metadata with a real provider candidate, and the two commits after the last look (`a3870ec` alignment of the viewing-hours row, `00e6a37` a test).

## 1. Outcome

**Watching now (Dashboard).** Each row keeps Stop and gains a menu: Pause or Resume (whichever applies), Back 30 seconds, Forward 30 seconds and Start over where there is a timeline (films, episodes, music, books; a channel is only paused and resumed), and Details. The row says where playback is ("At 12:34"). The command is sent against the session as it is at that moment and sent again by itself if the session moved in between; playback that ended meanwhile says "That playback has already ended."

**Playing on your other devices (Settings › This device).** Back 30 seconds, Forward 30 seconds and Start over in a menu beside Pause/Resume and Stop, offered only where the server allows seeking.

**Open pages stay current.** Console panels now read through one quiet mechanism: what is shown stays while it is read again, a read nobody asked for never spins, blanks or shows its own failure (it is retried with back-off, and the shell's one "can't reach the server" state covers an outage), a first load gets a couple of quiet attempts before its error shows, and Try again / Refresh / the read after the owner's own action answer at once. Wired: API keys, members' tokens, a member's own tokens, webhooks, accounts and their roles, invitations, devices, client reports, viewer feedback, play history (new plays arrive at the top; rows are no longer emptied by a filter or Refresh), backups, storage folders, what is in use, downloads usage, remote sources (a mount or share changing state by itself), Live TV sources, tuners, recording space, recording rules, Library Channels, and the playback-problems list. The pace is 30 seconds while the page is in front where the server sends no hint (see section 7 for the hints that would replace it). Disk usage is the exception: measuring it walks the server's folders, so it is read when the page opens, on Refresh and after a clean-up.

**Out-of-date saves are sent again, silently.** A change the server refuses as out of date (`428 revision_required`, `412`, or a conflict code) is read again and sent again by itself, through the shared `retryWhenStale` path:
- every settings document on the Server pages (the nine draft forms): the draft is carried onto the newer document field by field. Only a setting changed both here and elsewhere, to something different, is reported; the form then holds the newer document with the owner's own values still in it, so saving again is a knowing choice. A page being edited also now shows other people's changes to the fields it did not touch, instead of freezing until saved.
- an account's limits, role, and sign-in/libraries; revoking an invitation; approving or blocking a device; a library's settings, metadata source and scan depth; the server's metadata settings; removing a library; deleting from disk (re-sent only if what would be deleted is exactly what was confirmed); pausing, refreshing and removing a Live TV source; a source's own settings; recording space; pausing or removing a recording rule; turning a Library Channel on or off, reordering and deleting channels; answering a title that needs attention; acknowledging an alert; updating a feedback report; saving a schedule.
- "already done" is not an error: approving a device someone else approved, pausing a source that is already paused, and the like, simply succeed.

**New for owners.**
- **Streaming › Playback problems Portico remembers**: the ways of playing a file that failed on a device and are being avoided, with when it was reported, until when it is avoided, the server's own detail, and Forget (lets the first way be tried again). Absent while there are none.
- **Edit metadata › Match**: a show's match can choose which of the provider's episode orders to use, and "Add to each file's episode number" for files numbered straight through (anime); the current match shows its order in words instead of the stored code, and "Use this order" changes it.
- **An account's viewing hours** (People › an account): which days, from when, until when, a row at a time, saved with the page. Before, hours set elsewhere were only counted ("kept as they are") and could not be seen, changed or removed on the web.
- **Add a live TV source › HDHomeRun** can be given a guide (XMLTV) address; it was hidden for tuners although the server accepts it.

**One pattern (the coherence pass).**
- **Lists.** Every list is a group of rows; the create action is `secondary sm` in the group heading (API keys, tokens, webhooks, invitations, accounts, sources, remote sources, backups, recording rules, viewing hours), per the component spec. Only a page that is its list keeps a primary create action (Libraries, Library Channels). Viewer feedback and Remote sources were redrawn as groups like their neighbours (they had larger headings, a centred empty state and different edges).
- **Empty, loading, failed.** One empty line (`SettingsEmpty`), one "Loading {thing}" (`SettingsLoading`), one inset notice (`SettingsNote`), one footer (`SettingsFooter`); three different empty styles and several panels that showed nothing while loading are gone. An invisible padded footer that used to sit under Disk usage and Updates is gone.
- **Confirmations.** Blocking a device and revoking an invitation now ask first (both took effect on the click). Restore a backup uses the same confirmation as everything else that takes something away. What is typed to confirm is never an identifier: delete a backup asks for the word "delete" (it asked for the backup's id), clean up a folder asks for the folder's name (it asked for its id). The control that opens a confirmation is no longer red; the dialog's confirm button is. Cancel is no longer a hard-coded English default, and every confirm button carries its verb.
- **One Save.** "Who can record" lost its own Save button and joins the page's Save.
- **One Refresh.** Play history's own Refresh button is gone; it answers to the page's.
- **Phone width.** A group's heading action now goes under the heading text instead of squeezing the description into a narrow column beside it (seen in the earlier run's phone screenshots of People and Recording).
- **Words.** Task kinds and states, alert names and severities, scan states/depths/steps, feedback categories and statuses, roles, device platforms, Live TV source states and kinds, remote-storage states, Library Channel states and the channel-map table headings come from the catalogue instead of English-only tables or the server's raw codes. Removed hard-coded sentences: "Invitation revoked.", "Device approved.", "Saved.", "Cleaned up.", "Message sent.", "Scan started.", "Logo saved.", "Sent from … while …", and others.

**Bugs found and fixed on the way.**
- Add a live TV source showed "The request did not finish. Try again." for every failure, whatever the cause (the dialog passed an already-worded message to a function that expected the error). It now says what is wrong (bad address, sign-in refused, tuners busy, and so on).
- A device with no platform read "owner · · last seen …" (a doubled separator); "web" was shown as the raw code.
- Polling the accounts list would have reset the edit-account dialog while typing; it is filled once when it opens.
- The console client kept a refused request as "unconfirmed" after a 412 or 428, so the next attempt replayed the old revision; a refusal for a moved or missing revision is now treated as the definite answer it is.
- A manual backup asked for while a scheduled one is running (the new `409 request_in_progress`) no longer reads as "this changed": the running backup's progress is shown.

## 2. Decisions I made

1. **Owner transport is a menu, with relative moves.** The owner's control document carries a position but no duration, so there is no scrubber: Back 30 seconds, Forward 30 seconds, Start over. Stop stays the one visible button (it is the common action); the rest is in the row's menu, like every other console row. Details replaced the separate Open button.
2. **Thirty seconds where the server says nothing.** The event feed has no hint for keys, tokens, webhooks, members, devices, invitations, reports, storage, sources, tuners, recordings or channels. Rather than leave those pages stale, they re-read every 30 seconds while in front (the promise is "within about 30 seconds"), quietly. The hints that would replace this are listed in section 7.
3. **Disk usage is not polled** (it walks folders; up to 120 s on the server).
4. **A read that named nothing to follow is left alone.** A document opened in a dialog to be edited is read once; only panels that opted in are re-read, so nothing is moved under an owner's typing by a reconnect.
5. **Field-by-field, lists as one value.** When a draft meets a newer document, objects are compared field by field and a list (home networks, windows) is one value. A conflict is declared only when the same field was changed on both sides to different values.
6. **A quiet reload while editing now moves the untouched fields.** The earlier rule (freeze the whole form while it is dirty) was tested; I replaced it and its test, because per-field carry-over is both safer for the next save and what "stays current" means.
7. **Delete from disk is re-sent only for an identical preview** (same confirmation text, same files and size). Otherwise the new preview is shown and nothing is deleted.
8. **Blocking a device asks first; approving does not.**
9. **Restore is a destructive-toned confirmation without a typed word**: a copy of the current data is kept first, so it is bounded, but it does replace what the server has.
10. **Typed words:** "delete" for a backup; the folder's name for a clean-up; the count stays for missing titles and the server's own token for media.
11. **Create actions follow the component spec** (`secondary sm` in the group heading). This demoted "Add account" and "Add source" from primary.
12. **Remote sources became one group** with the kind in each row's line, instead of a large heading over two sub-groups.
13. **Viewing hours: one control for days** (Every day, Weekdays, Weekends, or one day). A set of days saved elsewhere that is none of those is kept and shown as "Several days: …". The time zone is shown, not edited: times are the server's own unless a zone was saved through the API.
14. **Episode order controls live in Edit metadata** (the web's identification screen), per match, and the offset is offered only where the server accepts one (TMDB's own order and its alternate orders).
15. **Playback problems are grouped by file**, because Forget works per file. The row cannot name the title (section 7).
16. **Per-grouping sorts: nothing to do in the console.** No owner list route accepts a sort, and no owner list has a sort control. The only sort choice on an owner screen is a Library Channel rule's order, which is the server's fixed vocabulary for rules, not a grouping.
17. **The ambiguous labels are left to web-viewer.** Where an owner sees versions (the Files list, Delete this version, Not this title) they are already numbered "Version 1/2" with their paths. The two places the browser run photographed (the version picker beside Play and the Download dialog) and the subtitle menu are viewer screens in files my brief tells me to leave. See section 7.
18. **Unknown codes read as a general word**, except a task kind or scan step (shown as its own name in words, which tells two apart) and a feedback category (kept as it is, as an existing test requires).
19. **New strings are in their own catalogue part** (`packages/i18n/src/catalog/en-US/owner-console.ts`), merged into the catalogue, so the sibling lane's string edits and mine do not collide. Where a message's meaning changed I added a new id rather than editing the old one.
20. **Shared files I touched outside the owner pages**, minimally: `web/src/ui/Settings.tsx` and its CSS (four small pieces, the confirm dialog's labels, the phone-width heading), `web/src/app/delete-media-dialog.tsx` (re-send), `web/src/screens/shared/MetadataEditor.tsx` (the Match tab only), and client-core `freshness-actions.ts`, `freshness-hub.ts` (one export), `console.ts` (one line), `presentation/errors.ts`, `presentation/settings-structure.ts` (one web-only panel).

## 3. Bugs

My brief names no audit documents. From the reports I was pointed at:
- `428 revision_required` not recognised by the shared predicate (base-health, native-health): **fixed** in `isStaleAnswer`, and every owner write goes through the shared retry.
- "HDHomeRun guide URL hidden on add" (review-livetv, N5): **fixed** on the web.
- Failure-memory screen missing (leftovers-server-a 1): **built**.
- Anime order offset controls missing (leftovers-server-a 2): **built** on the web.
- Secondary-profile gating of account devices (leftovers-server-a 4): already fixed by the browser lane; not changed.
- `409 request_in_progress` on manual backup (review-ops): **handled**.
- Console pass 3 "not done" 2 and 3 (owner pause/resume/seek; remote seek): **done**. Item 4 (freshness for keys, webhooks, devices, reports, storage, Live TV, DVR): **done**, by pace where there is no hint.

## 4. API changes

None on the server. Client-core additions (`packages/client-core/src`):
- `freshness-actions.ts`: `revision_required` and `revision_mismatch` count as out of date; `sendWithFreshRecord`, `rebaseDraft`.
- `freshness-hub.ts`: `finalAnswer` exported.
- `server-admin/quiet-read.ts` (new): `QuietRead`, `QUIET_READ_EVERY_MS`.
- `server-admin/draft-form.ts`: save rebases and retries; a reload while editing carries the draft.
- `server-admin/now-playing.ts`: `transportOffers`, `transportAction`, `seekTarget`, `transportDone`, `sendTransport`, `positionClock`, `REMOTE_SKIP_MS`; `sessionFacts` includes the position.
- `server-admin/people-words.ts`, `storage-words.ts`, `console-catalogue.ts` (new); `live-words.ts`: `liveSourceState`.
- `storage-management.ts`, `remote-sources.ts`: `revalidate()`.
- `playback-failure-memory.ts`: `groupPlaybackFailures`.
- `screen-metadata.ts`: `readTmdbOrder`, `orderTakesOffset`.
- `presentation/errors.ts`: own sentences for `session_ended`, the Live TV source codes and `image_too_large`.
- `presentation/settings-structure.ts`: custom id `server.playbackProblems` (web only).
- `console.ts`: 412 and 428 clear a held request like other refusals.
The older English tables (`jobLabels`, `alertLabels`, `tierLabels`, `linearError`, …) are still exported: the native apps read them. The web no longer does.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired (screens)

No setting was added. Given a screen or a fuller one in this lane:

| Setting / action | Control | Where |
|---|---|---|
| Member limit `schedule.windows` | rows of days + from + until, Add hours | People › an account › Viewing hours |
| Show match `order` (alternate order, `tmdb-absolute` offset) | drop-down + number | Edit metadata › Match |
| Remote source `guideUrl` for an HDHomeRun | optional address | Add a live TV source › HDHomeRun |
| Playback failure memory (read, clear per file) | list with Forget | Server › Streaming |
| Admin session controls (pause, resume, seek) | row menu | Dashboard › Watching now |
| Own remote sessions (seek) | row menu | Settings › This device |

## 7. Needed from others

**Server (each is exactly what a screen is waiting for)**
1. **Hints on the event feed for owner lists**, so the 30-second reads can go: an admin-audience event when any of these change — API keys and access tokens; webhooks and their delivery state; members and roles (`/v1/direct/members`, `/v1/admin/access/members`); invitations; devices and their trust; client reports; feedback reports; backups; storage status and upgrade copies; download usage; remote mounts and WebDAV sources; Live TV sources and their refresh state; tuner assignments; DVR storage and recording groups; Library Channels; playback route failures. One type per family with the resource id is enough; the console already turns any feed event type into a dependency (`dependsOn.event('<type>')`).
2. **Title on a playback failure.** `GET /v1/admin/playback/route-failures` rows carry `sourceId` only; `itemId` and `title` would let the row name the title and link to it. Today the row says what failed and on which player, not for what.
3. **Duration on the admin controls document** (`durationMs` on `GET /v1/admin/sessions/{id}/controls`, or on the admin session list) if a position slider is wanted, and so Forward cannot run past the end.
4. Still open from earlier passes: per-source import limits on a saved source's document; account and profile names on tuner assignments; subtitle-provider credentials as a setting with a route.
5. **A way to read one access record**: member, device and invitation by id. To re-send a role or trust change the web walks the list pages (bounded at 32) to find the record again.

**web-viewer lane**
- **Version labels** (`web/src/screens/detail/PlaybackChoices.tsx` and the Download dialog): two same-resolution, same-codec copies read the same. The owner's Files list numbers them "Version N" (`mediaInfo.version`); the picker could append the same number when labels repeat.
- **Subtitle labels** ("English / File" twice) in the player's menu: the format name is available from `codecLabel` in `presentation/title-files.ts`.
- **Obeying remote transport.** The owner's and the remote's pause/resume/seek arrive at the playing device as `session.updated`. The server lane said nothing had verified a player obeying it; I did not either.
- **Tuner reservation countdown** (`channel.reservation-required`, review-livetv): a viewer-side prompt in the player, not built here.

**Native lanes**
- Sync `packages/client-core` and `packages/i18n` (new catalogue part `en-US/owner-console.ts`). `sessionFacts` now includes the position. `DraftForm` behaves differently while editing (section 2, item 6).
- The same panels are not on phone or TV: viewing hours, playback problems, the episode-order controls, owner pause/resume/seek. The shared pieces are there (`people-words.ts`, `groupPlaybackFailures`, `readTmdbOrder`, `sendTransport`, `QuietRead`).
- `console-catalogue.ts` replaces the English tables; the tables can be deleted once native reads the catalogue.

**Documents**
- `web/docs/administration.md` is updated (the page is called Streaming on screen; People has three tabs; the list, confirmation and staying-current rules are written down).
- `packages/design/COMPONENTS.md` says a `form` dialog has no ✕; the web's confirmation dialogs do have one. I left both as they are.

**String clean-up** (no longer referenced by the web): `web.dvrSettings.savePermission`, `web.maintenance.deleteBackupBody`, `web.maintenance.cleanUpBody`, `web.missingTitles.removeBody`, `server.people.scheduleKept`, `web.storage.loading`, `web.storage.webdavShares`, `server.reports.actions`, `web.nowPlaying.open`. Check native before deleting.

## 8. Test notes for other environments

Environment: the web app against a server built from the integrated branch, signed in as the owner unless a step says otherwise. Each at desktop width and at 390 px. Pass is the described result.

**Watching now**
1. Play a film on another device as a member. Dashboard › Watching now shows the row with "At m:ss". Open its menu: Pause, Back 30 seconds, Forward 30 seconds, Start over, Details. Pause: the other device pauses within a moment and the row says Paused; the menu now offers Resume.
2. Forward 30 seconds twice: the row's position moves by about a minute and the other device jumps. Start over returns it to the beginning.
3. Play a Live TV channel: the menu offers only Pause/Resume and no moves.
4. Stop playback on the device, then choose Pause from a menu left open: "That playback has already ended."
5. As a member (not owner or administrator): no Watching now panel, and the controls route refuses.

**Playing on your other devices**
6. Play on a TV from the same profile; in a browser open Settings › This device: the row has Pause, Stop and a menu with Back, Forward, Start over; each acts on the TV.

**Staying current** (two browsers, A and B, both owners unless said)
7. A on Integrations › API keys. In B create a key: it appears in A within about 30 seconds, without a spinner or the list emptying. Revoke it in B: A moves it under "Show 1 that no longer works".
8. A on People › Devices. Sign in on a new device with approval turned on: it appears as Waiting in A by itself.
9. A on Storage & backups. Start a backup in B: A shows the progress bar, then the new backup.
10. A on Live TV › Sources. Refresh a source's guide in B: A's badge goes to Refreshing and back.
11. Stop the server with A on any Server page for 20 seconds, then start it: no red notices appear on the panels while it is down (the shell's own "can't reach" state may); the panels are current again within a minute of it returning.
12. A on Server › Remote access with a home network half typed into the field and another row changed but not saved; in B change "Who may sign in from outside" and save. Within 30 seconds A shows B's value in that row, and A's own unsaved change is still there with the Save bar.

**Out-of-date saves**
13. A and B both on Server › General. B renames the server and saves. A (not reloaded) changes "Check for new versions" and saves: saved, no message, and B's name is kept.
14. A and B both change the server name to different names; B saves, then A saves: A sees "This changed while you were looking at it…", the name field still holds A's text, and pressing Save again saves A's name.
15. A opens a member's account; B changes that member's role; A changes the stream limit and saves: saved with no message.
16. A and B on People › Devices with a waiting device. B approves it; A presses Approve: "Device approved." and no error.
17. Through the API, send any revision-bearing change without `expectedRevision`: 428. (The web never sends one without; this only confirms the server's answer the client now recognises.)

**New screens**
18. People › a member › Viewing hours: "Any time". Add hours: a row "Every day, from 08:00 until 20:00"; Save. Sign in as that member at 21:00 server time: refused with the time-of-day message. Change the row to Weekends, Save, reload: kept. An end before the start shows "ends the next day". Remove the row, Save: any time again.
19. Make a device fail a file one way (for example an HEVC file on a browser without HEVC, played at Original): Server › Streaming gains "Playback problems Portico remembers" with a row. Forget: "Forgotten…", the row leaves, and the next play tries the first way again. With none, the group is absent.
20. An anime show whose files are numbered straight through (1–24) but which the provider splits into two seasons: Edit metadata › Match shows "Episode order" (when there is more than one) and "Add to each file's episode number". Leave the offset at 0 and use the match: episodes 13–24 are unmatched or wrong. Set what the files need (for example 0 with the provider's absolute order, or 12 for a second-season folder numbered from 1), "Use this order": the episodes line up. The current match line names the order in words.
21. Add a live TV source › HDHomeRun: the Guide (XMLTV) URL field is there with its help; with an address, Check source reports programs as well as channels.

**Patterns**
22. People › Devices › Block: a dialog names the device and says it is signed out at once; Cancel leaves it. People › Invitations › Revoke: asks first.
23. Storage & backups › a backup › Delete: the dialog asks to type "delete" (any capitals) and shows the backup's date; the id appears nowhere. Clean up on Logs: asks for "Logs".
24. Restore this backup: the same dialog style, a red "Restore backup", the backup's date above the consequence.
25. Live TV › Recording › Who can record: choose a profile, flip the switch: the page's Save bar appears; no button in the group. Save: kept after reload.
26. At 390 px: every group with an action (API keys, Webhooks, Accounts, Recording rules, Backups, Viewing hours) has the action under its description, not beside it.
27. Add a live TV source with a wrong address: the dialog says the playlist or guide couldn't be used (not "The request did not finish").

## 9. Not finished

- **Not seen in a browser:** Watching now with a live session and its menu; the playback-problems panel with rows; the episode-order controls with a real provider candidate (the test server used local metadata only); the last two commits. Everything else listed at the top was looked at.
- **No pause/resume/seek was exercised against a real player.** The shared rules are unit-tested; whether the playing device obeys is the player's side (section 7).
- **Not silently re-sent:** saving a Library Channel from the builder (a whole-form editor; a conflict is still shown), and remote-storage mount commands (the mount service reports its own conflict state). Both are now much less likely to conflict because their lists are read every 30 seconds.
- **Component-level tests** exist only where the harness already had them (metadata source, updated). The new panels (playback problems, viewing hours, the Now Playing menu) are covered by tests of the rules they are built from, not of their rendering.
- **The 30-second reads are a stand-in** for feed hints the server does not send (section 7, item 1).
- **Native apps:** nothing in this lane reaches phone or TV.
- **Not touched:** the log tail, records and capabilities panels on Troubleshooting, Remote access status rows and General (they keep their own small inline wrappers, visually the same as the new shared ones); the Library Channel builder's own screens; W1–W6 (the viewer lane's).
- Full test suites and release builds were not run, as instructed. One pre-existing failing web test is named at the top.
