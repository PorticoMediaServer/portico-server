# Lane report: native-owner

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/native-native-owner`, branch `audit-implementation/native-owner`, 6 commits on top of `a732395` (23 files, about +2,500 −170). Work ran on personal mini-1 (`ehler-homelab-mini-1`). Nothing pushed, deployed, installed or built. No Swift, Objective-C or Kotlin was changed, and `packages/*` is untouched. **Nothing was run on a device, a simulator or Vega, and nothing was looked at on a screen: everything below is source, type-check and unit-test evidence.**

# 1. Outcome

**Where it all lives.** On iPhone, iPad and Android phones and tablets the owner's Server heading now has the same pages as the web, in the same order. Two pages are new there (Integrations, Metadata) and six existing pages gain panels or rows (Libraries, Live TV & Channels, People, Storage & backups, Troubleshooting, General). A member gets one new group under Profile (their own access tokens). Television gets none of it.

**N1 — Integrations (phone and tablet).**
- *API keys* (owner): list, create with a name, what it may do (the server's scopes as switches; "Everything" stands alone) and when it stops working (never, 30 days, 90 days, a year), revoke. The key is shown once, readable in full, with the system share sheet to copy it. The requests-per-minute setting sits under the list and saves with the page.
- *Members' tokens* (owner): everyone's personal tokens, whose each is, revoke; and the "may members make tokens" setting.
- *Personal access tokens* (any member): under Settings › Profile, behind one row that opens it. Create, list, revoke, the same copy-once display. When the owner has turned tokens off, existing ones are still listed and can still be revoked.
- *Webhooks*: list with what is failing, waiting or dropped; add; edit (whether it sends, name, address, events by group with choose-all); send a test; delivery log that keeps itself current while anything waits; replace the secret; remove. The signing secret is shown once.
- *Assistants*: the address to give an assistant (with share) and what key it needs.
- The keys list has left People › API keys: it is in one place, as on the web.

**N2 — People.**
- *Delete this person*: the server's preview is the confirmation (account, number of profiles and devices, how many plays stay in the owner's history without the name, and that a Portico Account itself is not deleted). If the account changed while the sheet was open, the preview is read again and the owner confirms again.
- *Viewing hours*: add, edit (days, from, until, "ends the next day") and remove, saved with the account. This replaces the old line that only said the saved schedule was kept.
- *Rating system*, then a rating in it, where the server publishes systems. A limit saved before systems existed stays as it is until changed.
- Role, limits, library and sign-in saves, device approval and invitation revoke now retry silently when the account moved underneath.

**N3 — Library and metadata tools.**
- *Needs attention*: each film, TV and anime library row shows the server's exact count ("7 to match"); the library's Metadata tab opens the list. Each title says why it is there, offers the suggested matches ("Use this"), and a menu of only what the server named for it: search by hand, enter a provider ID or address, open the show, dismiss. The list follows a search or match until it settles.
- *Metadata* page: the owner's TMDB and TheTVDB keys (never shown back; set, replace, go back to Portico's), adult titles, and the local-artwork limit with what is used. One Save with the page.
- *Remove library*: typed confirmation of the library's name, with what goes and what is kept.
- *A title's files* (owner, on the title page, episode panel and show page): a file opens "Not this title…" (split, only for films, tracks and book files that are on disk and have another version) and "Delete this version…" (the server's preview and typed confirmation). "Merge" finds another title of the same kind in the library, asks which to keep, and merges.

**N4 — Runtime and diagnostics.**
- Storage & backups: *In use now* (streams, connections, analysis files, local artwork, each against its limit), *Where things are kept* (database, temporary files, backups, reclaimable space, copies made before an upgrade with delete), *Downloads* person by person with "Remove their downloads".
- Schedule: *Pause background work for an hour* / Resume (the row existed in the structure and drew nothing).
- Streaming: *Your FFmpeg build* status (also drew nothing before). It says "checked, used after a restart" when that is the case, never that an override is in use when it is not.
- General: *Restart server*, asked first, then waits for the server and says when it is back.
- Troubleshooting: *Titles set aside* (only while there are any) and *Client reports* (list, read, share, delete).

**N5 — Live TV sources.**
- The add form offers the guide (XMLTV) address for an HDHomeRun too, with the tuner-specific help.
- A source's settings gain *Only these groups* and *Only these channels*: searchable pick lists from what the source really has, a page at a time.
- New *Channels and numbers*: per channel its number, the guide listing it takes programs from, hidden or not, beside what the source sent. Saved in batches.
- What an import left out, and why, is shown above the sources list and on the add form's check.
- Guide days and the three import limits were already phone rows; a test now holds them there.
- Source rows use the shared state words ("Up to date", "Needs its sign-in again") instead of the raw state.

**Television Dashboard (Apple TV, Android TV, Fire TV, Vega).** Settings › Server › Dashboard is now one read-only screen: server name, version and uptime, health; who is watching what, on which device, playing directly or being converted; background work under way; free space, Portico's own files and the database; open alerts and work that failed in the last day. No row does anything when pressed. It re-reads on the event feed and at a calm pace, keeps what is shown while it reads, and keeps row identity so focus does not move.

# 2. Decisions I made

1. **Owner pages are taken from the web's own structure, not re-authored.** The shared settings structure still marks them web-only, and `packages/*` are copies. A native adapter (`withOwnerPages`) asks the shared code for the web structure and lifts exactly the named pages and sections into the phone structure, in the web's order. No label or row is written twice, and when the shared structure names phones the adapter adds nothing.
2. **The television Dashboard removes actions that were there before.** Until now a TV owner could stop someone's playback, acknowledge an alert and pause or cancel a task from the Dashboard. The 14 October decision says read-only, so those are gone from TV. The load history sparklines are also gone from TV: they were not in the list of what the Dashboard shows. `ServerPageBody` draws nothing else on TV whatever the structure lists.
3. **Owner file tools sit on the title page.** The brief says not to change viewer screens, but split, delete-a-version and merge have no other home (the web has them there too). The change is additive and owner-only: for a member, and on TV, the Files list renders exactly as before.
4. **Merge on native starts from one title and finds the other by name**, because native has no multi-select. It is offered for films, shows and episodes. Songs have no Details section on native, and audiobook files and extras have no search group, so merge is not offered for those.
5. **A webhook's on/off switch is in its edit sheet**, saved with Save, rather than a switch in the list (a native list row is one press target). The list says "Off".
6. **Secrets are shown with the existing pattern**: readable row plus the system share sheet. The app has no clipboard dependency and I did not add one.
7. **Viewing hours times are typed** ("8", "8:30", "20.15", "0830"); something that is not a time changes nothing. There is no native time picker in the design system and I did not add a dependency.
8. **A member's tokens stay collapsed** behind one row and are not read until opened, as the shared structure says ("kept out of the way").
9. **A conflict keeps the owner's edits on screen.** When a save is refused because the same setting changed elsewhere, the draft is rebased onto the new document and shown; pressing Save again saves it. Same rule as the web.
10. **Deleting a member is never retried silently**: the preview is the confirmation, so a changed account means reading and confirming again.
11. **Client reports are shared, not downloaded.** A phone has no downloads folder to drop a file in; the share sheet covers copy, save and send.
12. **The file id is not shown on the Files list.** There is no catalogue string for it and a bare id is a code. Edition and "missing since" were already shown.
13. No new strings were invented. Where the catalogue had no exact sentence, the nearest existing key is used and the wanted key is listed in section 7.
14. The cheap helper script was not used.

# 3. Bugs

No bug register was assigned to this lane. From the decision audit's list:
- **B-11.8** (HDHomeRun add form hides the guide address): fixed on native.
- **B-18.5** (native removes a folder, not a library): fixed; whole-library removal with typed confirmation.
- **B-18.27** (native owner resource, metadata, integration and diagnostic panels missing): fixed for N1–N5.

Found and fixed while working:
- The Live TV add form kept the checked preview after the guide address was edited, so a typed guide address was silently left out of what was added. Editing it now asks for the check again.
- `server.backgroundPause` and `server.ffmpegStatus` were listed for phones by the shared structure and drew nothing.
- One malformed row in the Live TV sources answer would have failed the whole list; a bad row is now left out alone.

# 4. API changes

None. New native consumers of existing routes: `/v1/me/access-tokens`, `/v1/admin/access-tokens`, API access options, `/v1/admin/webhooks` (+ test, deliveries), `/v1/mcp` (address only), member deletion preview and delete, `/v1/rating-systems`, metadata attention (list, act, counts), metadata settings, library removal, title merge, inventory file lookup and separate, version delete preview and delete, resource usage, storage status and upgrade-copy delete, download usage and revoke, background pause, restart, catalogue failures, client logs (list, read, delete), live source group and channel choices, channel map.

# 5. Schema changes

None.

# 6. Owner or viewer settings added or wired

Nothing new was invented; these existing settings now have a phone and tablet screen.

| Setting | Type, default | Screen |
|---|---|---|
| `runtime.apiRequestsPerMinute` | number, empty = the server's usual | Server › Integrations › API keys |
| `runtime.blockMemberAccessTokens` | choice (members may / may not) | Server › Integrations › Members' tokens |
| Metadata: artwork limit, adult titles, TMDB and TheTVDB keys | GB number, switch, secret text | Server › Metadata |
| Member limits: rating system, viewing hours | choice, list of windows | Server › People › an account |
| Live source: enabled groups, enabled channels, per-channel number / guide listing / hidden | pick lists, text, switch | Server › Live TV & Channels › a source |

# 7. Needed from others

**Shared packages (canonical copy in portico-server, then re-sync; each lets a native stand-in go)**
- `presentation/settings-structure.ts`: add `phone` to the `metadata` and `integrations` pages; to Storage `usage`, `places`, `downloadUsage`; to Troubleshooting `setAside`, `clientReports`; to General `restart`; to Profile `tokens`. Remove the phone-only `apiKeys` section under People. Then delete `withOwnerPages` and its three tables in `apps/native/src/screens/settings/native-structure.ts`.
- Consider one television entry for the read-only Dashboard, so "TV shows only this" is said in the structure and not in `ServerPageBody`.
- `server-admin/console-words.ts`: job names, job states and alert names are English tables, not catalogue keys. The TV Dashboard and the phone Dashboard both show them.

**Catalogue keys that do not exist (nothing is staged in code; the key in brackets is what is shown today)**
- `api.tokens.share` "Share or copy the token", `api.webhooks.shareSecret` "Share or copy the secret", `api.mcp.shareAddress` "Share or copy the address", `server.reports.share` "Share this report" [all `invite.share`, "Share"]
- `api.webhooks.off` "Off" [`server.channels.state.disabled`]
- `title.merge.action` "Merge with another title…" [`web.selection.merge`, "Merge"]; `title.merge.find` "Title to merge with" [`server.attention.searchFor`]; `title.merge.none` "No other title matches." [`server.live.noMatches`]
- `server.metadata.keyUnsaved` "Your new key will be used when you save" [`settings.save.unsaved`]
- `server.dashboard.freeSpace` "Free space" [`server.live.freeOnVolume`, "Free on the drive"]; `server.dashboard.porticoFiles` "Portico's own files" [`web.maintenance.totalLabel`, "Total"]
- `server.people.hoursTimeHelp` "Type a time, such as 8:30 or 20:15." [no hint is shown today]
- `mediaInfo.fileId`, if the file id should be shown to owners.

**Server**
- Webhooks carry no revision, so two owners editing one webhook is last-write-wins. Nothing to retry on the client.
- The download-usage answer says more people exist (`profilesNextCursor`) but the panel, like the web's, lists the first page only.

**Native, outside this lane**
- `screens/server/Dashboard.tsx` still has television sizes for the sparklines that TV no longer draws, and an unused `client` in `StatisticsPanel` (an existing lint error).

# 8. Test notes for other environments

Run here, all passing: native type-check (app and tests), Vega type-check (app and tests), both string lints (0 hard-coded strings), native suite 655 of 655 (605 + 50 new), Vega suite 87 of 87. ESLint on the touched folders reports one error, which was there before (`Dashboard.tsx`, unused `client`).

New tests: `test/owner-structure.test.ts` (10: which pages and panels a phone owner, a member and a TV get), `test/owner-rules.test.ts` (29: key requests, webhook drafts, delivery log, rating choices, typed times, metadata save and conflict, attention choices, channel map parsing and batching, file tools, merge order, TV work-in-progress and recent problems), `test/fresh-send.test.ts` (11: silent retry, rebase, the one real conflict, paging).

On devices. Use a server with an owner, one member, Live TV with an M3U and an HDHomeRun, and a library with unmatched titles. Check each on a phone and a tablet.

1. **Who sees what.** Owner: Settings › Server lists Dashboard, Play history, Libraries, Metadata, Live TV & Channels, People, Integrations, Streaming, Remote access, Storage & backups, Schedule, Troubleshooting, General. Member: no Server heading; Profile has "Personal access tokens" above Sign out. Fail: a member sees any Server page, or People still has an API keys tab.
2. **Keys and tokens.** Create a key with Read and a 30-day expiry. Pass: the key is shown once in full, Share works, the row says "Expires …". Close and reopen: it cannot be seen again. Revoke: it moves under "Show 1 that no longer works". As a member, create a token; as owner, see it under Members' tokens and revoke it; turn tokens off and check the member sees the notice and no Create row.
3. **Webhooks.** Add one pointing at a request catcher; the secret appears once. Send a test: the log opens, shows "Waiting to be sent" then "Delivered" without touching anything. Edit: turn it off, Save; the row says Off and Send a test is disabled. Replace the secret; remove.
4. **Delete a person.** Open a member, scroll to the end, Delete. Pass: the sheet names profiles, devices and plays kept. From another device change that member's libraries, then confirm: "changed while you were looking", and the figures re-read. Confirm again: gone from the list.
5. **Viewing hours.** Add hours, set Weekdays 16:00 to 20:00, Save, reopen: kept. Set Until 02:00: "ends the next day". Type "banana" as a time: nothing changes. Sign in as that member outside the hours: refused.
6. **Silent retry.** Open a member on two devices. On A change max streams and save. On B (stale) add viewing hours and save. Pass: B saves with no error and both changes are present. Then change max streams on both to different numbers: B gets one error and keeps its number on screen; Save again writes it.
7. **Needs attention.** A library row shows "N to match"; N equals the web's badge. Use a match: the success line, the title leaves, the count drops. Search by hand: "Your search is in progress", then matches appear by themselves. Enter `tt0133093`. Dismiss one. Tap a title: both sheets close and its page opens.
8. **Metadata page.** Change the limit to 0.05: Save is blocked with the gigabytes sentence. Paste a TMDB key, Save: "Your own key is in use"; "Use Portico's key", Save: back.
9. **Remove library.** Typing the wrong name keeps Remove disabled. Start a scan, then remove: it goes with no "out of date" error. Files on disk untouched.
10. **File tools.** On a film with two versions: tap a file, "Not this title…": a new title opens. On the new title, Merge, find the original, keep the original: one title with two versions again. Delete one version on a library that allows deleting: typed confirmation, the file is gone from disk, the title stays. On a library that does not allow it: the "turned off" note and no field. On an episode: only Delete is offered.
11. **Runtime.** Storage shows the three new groups with plausible numbers. Pause background work: the row turns to Resume with a time. Restart: confirm, "Waiting for the server to come back", then "The server has restarted".
12. **Live TV.** Add an HDHomeRun with an XMLTV address: after Check, edit the address: the check is asked for again. In the source's settings choose two groups, Save, refresh: only those channels. In Channels and numbers set a number and a guide listing ID for one channel and hide another, Save 3 changes: reopen shows them. With a source of more than 200 channels, change 250 and save: all saved.
13. **Television.** As owner on Apple TV, Android TV, Fire TV and Vega: Settings › Server › Dashboard. Pass: five groups; pressing Select on any row does nothing; start and stop a stream elsewhere and the row appears and goes within a few seconds while focus stays where it was; no Stop, Acknowledge or task menu anywhere. As a member: no Server heading.

# 9. Not finished

- **Nothing was seen on a screen.** Every layout here needs eyes: the sheets inside sheets (source settings › pick list; account › typed rows), the long footers, the TV Dashboard at ten feet.
- **Three levels of sheet were avoided, two are used** in several places. Two is what the app already did; I did not verify the new ones on iOS.
- **Not built, by scope**: the web's "every device" list under People › Devices (native keeps devices per account), the playback-problems panel under Streaming, and the "playing on your other devices" panel under Device. None is in this brief's list.
- **Merge** is not offered for songs, audiobook files or extras on native (decision 4).
- **Paged lists do not refresh by themselves** (keys, tokens, client reports, the attention list outside a search): they re-read on pull-to-refresh and after the owner's own action. A quiet re-read would collapse "Show more".
- **Client report download as a file** is share only (decision 11).
- **Vega**: the Dashboard type-checks against Vega's SDK and shares the Apple TV code; it was not run.
