# Lane report: console — the web screens for what the server lanes built

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/server-console`, branch `audit-implementation/console`, four commits on top of `3b2b345` (`2c2f565`, `12ddd75`, `991be4c`, `fce3b3e`). Nothing pushed, nothing deployed. No server code was changed.

How it was checked: `npx tsc --noEmit` is clean in `web/` and in `packages/client-core`; `lint:strings` (0 hard-coded strings, baseline 0), the glossary lint and `lint:design --strict` are clean; the preference-consumer check passes. Test files run and passing: four new client-core files (36 tests), the extended `web/test/card-name.test.ts`, and every existing test file that touches a screen or module I changed (16 web files, 13 client-core files). **Nothing was seen in a browser, on a phone-width viewport or against a running server.** Every screen below is unverified at runtime; section 8 is the click-through list.

## 1. Outcome

**Server › Integrations** (new page, web only; tabs API keys · Members' tokens · Webhooks · Assistants)
- **API keys.** Create with a name, a checklist of what the key may do (each choice has one sentence under it, and "Everything" ticks the rest), and when it stops working (never, 30 days, 90 days, a year). The secret is shown once with Copy and "I've copied it"; the dialog cannot be dismissed by accident while it is showing. Each key's row says what it may do, the last characters of the key, when it was last used and when it expires. Keys that no longer work are behind "Show N that no longer work", each with why it stopped. Revoke asks first.
- **Limits.** "Requests each key or token may make", a plain number a minute; empty is the usual 600.
- **Members' tokens.** Every member's personal access tokens with whose each is; the owner can end any of them. "Members may make their own tokens: Yes / No".
- **Webhooks.** List with an on/off switch per webhook and what needs attention (not arriving, events not kept, waiting). Add and edit in one dialog: name, address, and the events as a grouped checklist (Titles, Library scans, Playback, Recordings, People, This server) with All/None per group. Optional signing secret on add; "Replace the secret" afterwards; either way the secret is shown once. "Send a test" opens the delivery log. The log shows each delivery's event, state, time, attempts, when the next try is, and why it has not arrived, and keeps itself current while anything is waiting. Remove asks first.
- **Assistants.** What it is, the address to give an assistant with Copy, a note that an assistant elsewhere needs an address that reaches the server from there, and a button to where keys are made. There is no enable setting on the server, so none is shown.

**A member's own tokens.** Settings › Profile & account › "Personal access tokens" (collapsed until opened; shown to everyone except the owner). Same create / copy-once / revoke flow, with the per-token rate and what ends a token. When the owner has turned tokens off, existing ones are still listed and can be revoked; creating is disabled with the reason.

**Needs attention.** A library (film, TV, anime) has a "Needs attention" tab with a count. Each title says why it is there in a sentence, shows up to three closest matches with why each was suggested ("Same title · Same year") and "Use this", and offers what the server allows for it: Search for it, Enter a provider ID (a number with its provider, an IMDb ID, or the title's address at TMDB / TheTVDB / AniList / IMDb), Open the show (episodes), Dismiss. The list follows searches and matches while they settle. The Libraries list shows "N to match" beside a library that has any, and it opens that tab.

**Server › Metadata** (new page, web only). The owner's own TMDB read token and TheTVDB project key (never shown back: the row says whether your key or Portico's is in use, and a key can be replaced or removed), "Include adult titles when matching", and the local artwork storage limit in GB with what artwork takes on disk now. A row points to where each library's provider is chosen. **Background work speed** (Gentle, Normal, Fast) is on Server › Schedule beside Background task priority.

**Titles and files**
- A title whose files are gone says "Missing" and since when, on its page and in an episode's panel. A card that cannot play carries an "Unavailable" mark on its art and in its accessible name.
- The Files list names parts and versions (versions are numbered within a part) and shows a version's edition.
- **Split** (owner; films, tracks, book files): "Not this title…" on a version makes it a title of its own and opens it.
- **Merge** (owner): select exactly two titles of one kind and choose Merge in the selection bar; pick which one remains.
- **Remove a library**: Libraries › a library › General, with the library's name typed to confirm and two sentences saying what goes and what is kept.
- **Delete dialog**: says how many titles a show, season, album or book takes with it, and after deleting names each title that was kept and why.
- Refresh metadata is now offered on a show's card menu.
- The per-library "Your API key" field is gone: the server refuses a non-empty one, so it could only fail a save. A row there points to Server › Metadata.

**Server › Remote access**
- **Home network**: a line saying where the owner is connected from and whether that counts as home ("You are connected from 192.168.2.14, which counts as home", with a Home / Away from home badge and the plain-HTTP note when it applies); **Home networks** as a list with Remove on each entry and a field to add one.
- **Access from outside**: who may sign in, **External address**, Extra addresses (list), **Trusted reverse proxies** (list, moved out of Advanced).
- **Your own certificate**: its own section with a status line (in use and until when; what is wrong with the files; and, when replacement files are bad, that the previous certificate is still in use), then domain, certificate file, key file.
- **Connection limit** under Advanced: a number, empty for the server's own choice, with a note that it takes effect after a restart.
- Sub-path hosting is not offered anywhere. The two retired choices (required HTTPS, public address as home) were already absent from the structure.

## 2. Decisions I made

1. **One page, "Integrations", for keys, members' tokens, webhooks and assistants**, instead of leaving keys under People and scattering the rest. People is now Accounts · Invitations on the web. The iPhone app's People page keeps its keys tab, because the native app has a panel for it and none for the new page (the new pages are `platforms: ['web']`).
2. **The owner is not offered personal access tokens.** The owner makes API keys; everyone else makes tokens. Two near-identical lists for one person seemed worse than one. A token the owner made through the API still appears in "Members' tokens".
3. **Expiry is four named choices**, not a date picker. The server accepts any instant up to ten years ahead.
4. **"Everything" stands alone.** Ticking it shows the other choices ticked and locked, and sends only that one.
5. **The word "scope" appears nowhere on screen** (the glossary lint forbids it): "What it may do".
6. **Background work speed sits on Schedule beside Background task priority**, not on the Metadata page, so the two settings that govern background work explain each other. The brief listed it under metadata settings.
7. **Metadata is a page of its own** rather than a tab of Libraries: a library opened inside the Libraries page already uses the `tab` address for its own tabs.
8. **Home networks, extra addresses and trusted proxies are add/remove lists.** I changed how the web draws the existing `lines` control rather than adding a control type, so the shared settings structure is unchanged for the native app.
9. **Trusted reverse proxies moved out of Advanced** into "Access from outside": a reverse proxy on its own hostname is a supported way to run the server at release.
10. **The needs-attention badge counts only titles that wait for the owner.** Work still in progress (pending, searching) is listed in the tab but not counted. The count comes from the first page of each library's list (see section 7), so it reads "25+" when there are more.
11. **Merge is offered from a selection of two**, as in Plex, not from a search dialog on the title page. Split is on the version in the Files list.
12. **Cards say "Unavailable", not "Missing".** A card's entry carries only `available: false`, which is also what an unreachable disk produces; only the title page knows it is missing (`missingSince`).
13. **Per-version delete is not built** (section 9). I considered chaining split → delete from the browser and rejected it: it cannot work for TV at all (the server does not split episodes), it is three writes that can stop half way, and it would fill in the server's typed confirmation on the owner's behalf for an irreversible delete.
14. **Split finds the file's record by reading the library's file list**, because the server cannot yet be asked for one file (section 7). It narrows to the file's folder when it can, shows how many files it has looked at, and can be cancelled. In a very large music library this can take a minute.
15. **Removed the dead per-library provider key field** (pre-release rule: remove what will never be used). The strings are left in the catalogue for the string clean-up.
16. **I did not merge the base again.** It has moved on (Watch Together, casting, playback scopes); the brief says a second pass brings those lanes.

## 3. Bugs

My brief names no audit documents. Bugs other lanes left for a web screen:
- B-18.5 (a library cannot be removed): the web control now exists, on the identity lane's route.
- B-18.6 (refresh on containers): Refresh metadata is now offered on a show, on the route the providers lane added. Seasons, albums, artists and books are still not offered (the server cannot).
- B-18.27 (dead settings): the per-library provider API-key field is removed. Pinned libraries and "Forget remembered devices" untouched.
- B-01.3 (API keys): the redesigned panel (checklist, expiry, state, last used) the api-platform lane asked for is built.

## 4. API changes

None on the server. Client-core additions (all in `packages/client-core/src`):
- `server-admin/api-words.ts`: wording and small rules for keys, tokens and webhooks (`chosenScopes`, `scopesText`, `expiryInstant`, `credentialFacts`, `credentialState`, `webhookEventGroups`, `webhookEventLabel`, `webhookTrouble`, `deliveryProblem`, `deliveryProgress`, `webhookAddressOk`, `assistantAddress`).
- `server-admin/attention-words.ts`: `attentionReason`, `candidateFacts`, `candidateLine`, `attentionCount`, `attentionCountText`, `attentionOffers`, `typedProviderId`, `hasAttentionList`.
- `server-admin/status-words.ts`: `connectionLine`, `ownCertificateLine`.
- `server-admin/server-forms.ts`: `withListEntry`.
- `title-identity.ts`: `canMergeTitles`, `MERGEABLE_KINDS`, `SPLITTABLE_KINDS`, `folderOfPath`, `libraryFolders`, `findLibraryFile`, `separateLibraryFile`.
- `metadata-settings.ts`: `artworkLimitText`, `artworkLimitBytes`.
- `presentation/entry-actions.ts`: `refreshMetadataPath(kind, id)`. `canRefreshMetadata` is unchanged, so the native apps behave as before until they adopt the new function.
- `presentation/errors.ts`: own sentences for `credential_limit`, `access_tokens_off`, `webhook_limit`, `confirmation_mismatch`.
- `presentation/settings-structure.ts`: pages `integrations` and `metadata`; capability `accessTokens`; custom ids `accessTokens`, `server.memberTokens`, `server.webhooks`, `server.assistants`, `server.metadataSettings`, `server.connection`, `server.ownCertificateStatus`.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired (screens)

| Setting | Control | Where |
|---|---|---|
| `apiRequestsPerMinute` | number a minute, empty = 600 | Server › Integrations › API keys › Limits |
| `blockMemberAccessTokens` | "Members may make their own tokens": Yes / No | Server › Integrations › Members' tokens |
| `tmdbReadToken`, `tvdbProjectKey` | password field, replace or remove; shows only whether one is set | Server › Metadata |
| `includeAdult` | switch | Server › Metadata |
| `localArtworkLimitBytes` | number in GB | Server › Metadata |
| `backgroundSpeed` | Gentle / Normal / Fast | Server › Schedule |
| `lanNetworks` | add/remove list | Server › Remote access › Home network |
| `advertiseUrl` | text | Server › Remote access › Access from outside |
| `accessUrls`, `trustedProxies` | add/remove lists | Server › Remote access › Access from outside |
| `customCertificateDomain` / `Path` / `KeyPath` | text, with a status line | Server › Remote access › Your own certificate |
| `connectionLimit` | number, empty = automatic | Server › Remote access › Advanced |
| `library.missingTitles`, `library.missingTitlesDays` | already had a screen (identity lane) | Server › Storage & backups › Missing titles |
| Per-library provider and "local metadata only" | already had a screen | Libraries › a library › Metadata |

## 7. Needed from others

**Server (each is the exact thing a screen is waiting for)**
1. **Delete one version of a title.** No route exists. Wanted: the delete preview and delete routes accept the title's file ids (`files[].id` from the detail) for one title, with the same preview, typed confirmation and receipts; or `DELETE /v1/admin/items/{id}/files/{fileId}` with that contract. It must work for episodes. The web then adds "Delete this version…" beside "Not this title…".
2. **Ask for one file's library record.** Either `GET /v1/admin/libraries/{id}/inventory?assetId=<files[].id>`, or `objectId` and `revision` on each of an owner's detail `files[]`. Today the web reads the list 40 files at a time to find it (`findLibraryFile`); with either, that function becomes one request and the progress line goes.
3. **Split for TV and anime.** `separate` refuses libraries that are not film, music or audiobook, so a wrongly grouped episode file cannot be split from the web.
4. **A count for needs attention.** `total` on `GET /v1/libraries/{id}/metadata/attention` that leaves out work in progress, or a count on each library in `GET /v1/admin/libraries`. Today the web reads one page per film/TV/anime library when the Libraries page opens and shows "25+". A way to leave in-progress rows out of the list would also keep a first import from filling it.
5. **"Missing" on entries.** `missing: true` (or `missingSince`) on content entries, so a card can say Missing rather than Unavailable.
6. **Artwork in use against the limit.** `localArtworkUsedBytes` on `GET /v1/admin/metadata/settings`. The page shows the Storage figure for all artwork and says that it includes fetched artwork, which the limit does not count.
7. **Editions in the version picker.** The playback offers' `sources[]` carry no edition, so two versions that differ only by edition read the same in the Version control beside Play. With that (and the version id a start accepts), the Files list can also get a "Play this version" button.
8. **An MCP on/off setting**, if one is wanted. None exists; the panel says nothing about turning it on.

**Native lanes**
- The structure has new ids (section 4). The new pages are web-only; nothing new is drawn on phone or TV. The phone's People page is unchanged.
- Use `refreshMetadataPath` to offer Refresh on shows.
- The key panel there still offers four presets; the web is now a checklist with expiry (`api-words.ts` has the shared rules and words).

**String clean-up** (no longer referenced by the web after this lane): `web.connectivity.homeNetworksHelp`, `.extraAddressesHelp`, `.trustedProxiesHelp`, `.customCertHelp`, `.connectionLimitHelp`, `.advertiseUrlHelp`, `.encrypted`, `.encryptedHelp`, `.wanAsLan`, `.wanAsLanHelp`, `settings.server.customCertDomain`, `web.librarySettings.apiKey`, `.apiKeySet`, `.apiKeyOptional`. Check the native apps before deleting.

**Second pass (reports present, code not in this worktree)**: accounts-security (People: rating-system picker and the other items under its "Client screens"), ops-packaging (Restart control, restore progress, owner client-report list), conversion-outputs (`ffmpegPath`, `sessionStorageMiB`), subtitles-text (OpenSubtitles credentials), livetv (global versus personal hide), personal-state (Undo for containers), search-sort (half-star ratings, paged Genres/Series).

## 8. Test notes: click-through per screen

Environment: the web app against a server built from `audit-implementation/main` with this branch merged, signed in as the owner unless a step says otherwise. Run each at desktop width and again at 375 px. Pass is the described result; anything else is a failure.

**Integrations › API keys**
1. Settings › Server › Integrations opens on API keys. People shows only Accounts and Invitations.
2. Create a key: Create is disabled until there is a name and at least one tick. Tick Everything: the other three show ticked and locked.
3. Create with Read, "In 30 days": the dialog shows the key starting `pk.`, Copy says "Copied", Escape and clicking outside do not close it. After "I've copied it" the row reads "Read · ends in xxxx · Never used · Expires <date>".
4. `curl -H "Authorization: Bearer <key>" <server>/v1/libraries` then Refresh: the row says "Last used just now".
5. Revoke: confirm dialog, then the key leaves the list and "Show 1 that no longer works" reveals it marked Revoked.
6. Limits: type 30, Save. A 31st quick request with a key answers 429. Clear the field, Save: back to 600.

**Members' tokens and a member's own**
7. As a member (not the owner): Settings › Profile & account has "Personal access tokens", closed. Open it, create a token: secret starts `pat.`, shown once.
8. As the owner: Integrations › Members' tokens lists it with the member's name. Revoke it; the member's token stops working.
9. Set "Members may make their own tokens" to No, Save. As the member: Create is disabled with the sentence saying the owner turned it off; existing tokens can still be revoked. As the owner, the account pages show no token section.

**Webhooks**
10. Add: Add is disabled until there is a name, a valid address and one event. `ftp://x` or an address with `user:pass@` shows the address sentence. "All" on Playback ticks its four events.
11. Add with an empty secret: a secret is shown once. Add with a typed secret: the webhook is added; the secret dialog appears only if the server returns the secret (either is a pass; record which).
12. Send a test: the log opens, the test row goes from "Waiting to be sent" to Delivered without pressing Refresh. Point the webhook at a closed port and test again: the row says it could not connect, shows attempts and "trying again at …", and the list row shows the not-arriving line.
13. Switch off: Send a test is disabled in the menu. Replace the secret: new secret shown once. Remove: gone, with its log.
14. Add webhooks up to the server's maximum: Add disables with the sentence naming the maximum.

**Assistants**
15. The address is `<the address in the browser>/v1/mcp`; Copy works; "Create a key" lands on the API keys tab. Point an MCP client at it with a key: tools are listed.

**Needs attention**
16. Import a library with a misspelled film and a file with no usable name. Libraries shows "2 to match" on that library; clicking it opens the Needs attention tab with a count in the tab.
17. "Use this" on a suggestion: success line, and the title leaves the list within a few seconds.
18. Search for it: type a title, Search; the dialog closes, the row says a search is in progress, then shows new suggestions without a manual refresh.
19. Enter a provider ID: paste `https://www.themoviedb.org/movie/603-the-matrix` (no provider picker shown), then on another title type `603` (picker appears). Both match.
20. Dismiss: the row leaves and the count drops. Rename the file and scan: it returns.
21. An episode row offers only Open the show and Dismiss. A library set to local metadata only offers only Dismiss.
22. A music or audiobook library has no Needs attention tab and no badge.

**Metadata**
23. Server › Metadata: both keys read "Portico's key is in use". Paste a TMDB token, Save: "Your own key is in use"; the field is empty again. "Use Portico's key", Save: back.
24. Artwork limit: `abc` shows the gigabytes sentence and blocks Save; `1,5` and `1.5` both save and re-show as 1.5.
25. Server › Schedule: Background work speed shows Normal; change to Fast, Save, reload: Fast.
26. Libraries › a library › Metadata: no key field; the "Your own provider keys" row opens Server › Metadata.

**Titles and files**
27. Delete a film's file outside Portico and scan: its page shows "Missing" with the date; its card shows "Unavailable" bottom-left on dimmed art and is read as "<title>, Unavailable". Put the file back and scan: both go.
28. A film with two files: Files shows Version 1 and Version 2. A film in two parts with two versions of part 1: "Part 1 · Version 1", "Part 1 · Version 2", "Part 2".
29. As the owner on a film with two versions: "Not this title…" → Split it off. Pass: the new title's page opens; the original has one version. Cancel during "Finding this file": nothing changes. Try in a library of several thousand files and note how long it takes.
30. In a film library, select two films: Merge appears (not with one, three, or a film and a collection). Merge, keeping the first: one title with both files as versions, watched state combined, selection closed.
31. Libraries › a library › General › Remove library: the button is disabled until the name is typed exactly; after confirming, the list no longer has it, the rail loses it, files are still on disk. Add the folder again and scan: watched marks return.
32. Delete a show from its card: the dialog says how many titles will be removed. Make one episode's file read-only and delete the season: the result names that episode with why it was kept.
33. A show's card ⋯ menu has Refresh metadata and it queues.

**Remote access**
34. Home network shows "You are connected from <address>, which counts as home". Remove the range that covers that address, Save: it now says away from home with the other badge. Add it back with the field and Add, Save.
35. Empty the home networks list: "Nothing listed." Typing an entry without pressing Add shows no Save bar.
36. Set External address to `https://media.example.com`, Save, reload: kept.
37. Own certificate with valid files: "In use, valid until <date>". Replace the certificate file with garbage: within 30 seconds and a Refresh, the line says the files can't be used and that the previous certificate is still in use.
38. Connection limit: empty shows "Automatic"; type 500, Save; the row keeps 500 and the help says it applies after a restart.
39. Settings search finds "webhook", "home networks", "metadata".
40. At 375 px: every dialog is a bottom sheet with reachable buttons; the webhook event checklist is one column; the candidate rows wrap with "Use this" below the title.

## 9. Not finished

- **Delete one version** is not built: the server has no route for it (section 7, item 1).
- **Choose a version to play** is the existing Version control beside Play; nothing was added to it, and it does not show editions (section 7, item 7).
- **Split** works only where the server allows it (films, tracks, book files) and finds the file by reading the library's file list, which is slow in a very large library (section 7, items 2 and 3).
- **The needs-attention count** is the first page's ("25+"), costs one request per film/TV/anime library when Libraries opens, and shows nothing when the first page is entirely work in progress (section 7, item 4).
- **Cards say Unavailable**, not Missing (section 7, item 5).
- **Artwork usage** shown is all artwork, not what counts against the limit (section 7, item 6).
- **No component-level tests** for the new screens: the tests cover the shared rules and wording they are built from, the settings structure, and the card. The dialogs' flows (create, copy-once, revoke, delivery log polling, split progress) are untested code until the click-through is done.
- **Nothing was run in a browser.** Spacing of the candidate rows, the list editor, the Unavailable mark over artwork, and the key rows on Metadata are the most likely to need a visual adjustment.
- **The native apps** have none of this; the new pages are web-only.
- **Other lanes' screens** (accounts, operations and restart, conversion, subtitles credentials, Live TV, downloads, Watch Together) are not built: their code is not in this worktree.
- Full test suites and a production build were not run, as instructed.

---

# Second pass (13 October)

Brought level with `audit-implementation/main` by fast-forward (`f35864d`), then six commits: `26568ba`, `4c5ed36`, `19af418`, `7206d72`, `17a179c`, `96174f2`. No server code changed. Checked the same way as the first pass: `tsc --noEmit` clean in `web/` and `packages/client-core`; string, glossary and design lints clean; my test files and every existing test file that touches a changed screen pass (11 web files, plus `live-words`, `settings-console-pages`, `settings-scope`, `i18n`, `catalog`, `server-administration` and the first-pass files in client-core). **Still nothing seen in a browser or against a running server.**

## S1. Outcome

**People (accounts-security)**
- **Delete this person**, at the bottom of an account's page (never for the owner). The dialog first reads the server's preview and uses it as the confirmation: how many profiles and devices go, how many plays stay in the owner's history without the name, and that a Portico Account itself is not deleted. If the account changed meanwhile, it says so and reads the preview again.
- An account that is off says "Turned off <date>" or "Left <date>" in the list and on its page.
- **One rating scale**: an account's limit is now a rating system and a rating in it, the same list a profile's limits use. A limit saved before systems existed stays shown as it is until changed.
- **Invitations** take an optional Portico Account username; the invitation then says it is only for that account.
- **Devices tab** (People › Devices): the switch "New devices wait for your approval", and every device that has signed in, newest first, with whose it is and Approve / Block.
- Play history shows "Someone who was removed" where a name is gone.
- **Privacy** (a member's Settings › Privacy) opens with the accounts lane's sentence about what the owner always sees.
- Already in place and checked, not changed: the remote sign-in policy (Remote access › Who may sign in from outside), the "not at this time of day" and "waiting for the owner to approve" sign-in messages.

**Operations**
- **Restart server** on Server › General, asked first; the row waits for the server to answer again and says when it is back.
- **Restore** shows what it is doing (copying, checking) and a progress bar while it is made ready.
- **Check for new versions** switch in Version and updates: off clears the address the server checks, on puts back the usual one (read from the settings registry). Part of the page's Save.
- **Client reports** on Troubleshooting: list, Read (in a dialog), Download report, Delete report.

**Conversion and playback**
- Server › Playback gains "Working space for each stream" (GB) and "FFmpeg location". Temp folder and hardware were already there.
- Settings › Quality: "If something looks or sounds wrong" now works on the web. Always play in stereo and Turn off HDR are kept by the browser and stamped on the capability document it publishes, republished on change. The passthrough switch and the Player row never appear on the web. The saved qualities for home and away were already drawn from the playback lane's rows.

**Live TV**
- Sources › "Guide and line-up limits": most channels, most programs, largest file (MB).
- A source's settings: "Only these groups" and "Only these channels" as add/remove lists; empty means all.
- What an import left out and why, as a notice above the sources list (per source) and under the preview when adding a source.
- Tuners: each line says what is using the tuner (Being watched, Recording now, or when a recording starts) and "finishing" for one being retired; the status words are now from the catalogue.

**Watch Together**: the waiting state reads "Waiting for everyone" (it fell through to "Ended"); a current title this profile may not see reads as that instead of "Nothing chosen"; a private viewer is named only "Private viewer".

**First-pass gaps**: the version picker beside Play now names a version's edition where the picker's ids are the title's file ids.

## S2. Decisions

1. **Device approval got its own People tab** rather than rows on Accounts: an opened account takes the Accounts tab, and a global list beneath one person's page would be wrong.
2. **Deleting a person has no typed confirmation.** The server's preview is the confirmation, as the accounts lane designed it; the button is danger-toned and disabled until the preview has been read.
3. **Update notices are a switch**, not an address field. An address the owner set by hand is left alone; if the usual address cannot be read from the registry, the switch cannot be turned on (it is disabled) rather than guessing one.
4. **A client report is downloaded from what Read fetched**, as one JSON file, instead of the server's download route: the same content without a second authenticated request path in the browser.
5. **Working space is shown in GB** (stored in MiB). A value under 1 GB set through the API shows as 0 until edited.
6. **FFmpeg has no separate status line.** The server reports a rejected build only as "still waiting for a restart", so the row's help says what that means after a restart (S4, item 3).
7. **Groups and channels are typed lists.** No route lists a saved source's groups or channel ids to pick from (S4, item 5).
8. **Playback fixes are republished without re-probing the browser**: a switch changes only what is stamped on the document.
9. **One catalogue fix outside my screens**: `downloads.reason.cancelled` read "Cancelled."; the US-spelling test failed on the base because of it. Now "Canceled."
10. I made the client-reports list tolerant of one unreadable row (it refused the whole page).

## S3. Settings given a screen in this pass

| Setting | Control | Where |
|---|---|---|
| `deviceApprovalRequired` | switch | Server › People › Devices |
| Member limit `ratingSystem` | drop-down, then the rating | an account's Limits |
| Invitation `porticoUsername` | optional text | Invite someone |
| `updates.feedUrl` | "Check for new versions" switch | Server › General |
| `sessionStorageMiB` | number, GB | Server › Playback |
| `ffmpegPath` | text | Server › Playback |
| Live `importLimits` (channels, programmes, bytes) | three numbers | Server › Live TV › Sources |
| Source `enabledGroups`, `enabledChannels` | add/remove lists | a source's settings |
| Device overrides `forceStereo`, `disableHdr` | switches | Settings › Quality |

## S4. Still needed from others

1. **Live permission changes.** Not done. A changed role reaches this browser at its next renewal (up to 15 minutes); nothing re-reads `/v1/me` on a 403. The session object exposes no "read the viewer again"; it belongs in `web/src/app/session.tsx` with the code that owns the sign-in.
2. **Pre-upgrade copy, separate cache and backup folders, space reclamation**: nothing to show. The folders are start-up variables with no route; the pre-upgrade copies under `migration-backups/` and reclamation status are not exposed by any route. (Each backup's row already shows its full path, so a separate backup folder is visible there.)
3. **FFmpeg validation status**: a field on the settings document saying whether the named build is in use, was rejected, and why.
4. **Per-source import limits**: the source input accepts them; no saved-source document returns them, so only the server-wide limits have rows.
5. **Pick lists for groups and channels**: a route listing a saved source's group names and channel ids (with names).
6. **Who is watching** on a tuner: assignments carry account and profile ids, not names.
7. **Server-wide downloads usage and revoking a profile's downloads**: no client-core service or screen; `GET /v1/downloads/usage?scope=server` exists.
8. **Watch Together and Cast items not done** (social report, items 3, 5, 6): loading paused without an audible first frame, remote controls for a handed-off session, routing session-change events to the player. They are in the player and its bridge.
9. **Subtitle provider credentials**: start-up variables, no route, no screen.
10. **TV activation and the Hosted outage gate**: native and client-core connectivity work; the web's Link a TV page approves a code on the local server and is not affected.
11. **Shared recordings**: stored once on the server; no client-visible field changed, so nothing to draw differently.
12. **First-pass gaps still open on the server**: needs-attention total (still the first page, "25+"), `missing` on entries (cards still say Unavailable), artwork bytes counted against the limit, deleting one version, asking for one file's library record.
13. **Native**: none of this pass is on phone or TV except the approval switch row and the import-limit and conversion rows, which are plain shared rows.

## S5. Click-through (second pass)

As the owner unless a step says otherwise; desktop width, then 375 px.

**People**
1. Open a member's account: "Delete this person" is the last group. Open it: the dialog shows a loading line, then the sentence with profile and device counts, the plays-kept line when there are any, and the Portico Account line for a Portico member. Delete: back on the list without them; their rows in Play history read "Someone who was removed".
2. Open the dialog, change the member in another tab (rename or limits), press Delete: the "changed while you were looking" sentence appears and the preview is read again.
3. The owner's own account page has no delete group.
4. Turn an account off: the list row and the account page read "Turned off <today>".
5. Limits: Rating system lists the published systems; choosing one empties the rating; the rating list shows that system's ratings with ages. Save, reload: kept. An account with an old plain rating shows it with no system chosen.
6. Invite someone with a Portico Account username: the pending row reads "only for the Portico Account <name>"; another Portico Account opening the link is refused with the other-account message.
7. People › Devices: switch on, Save. Sign in on a new browser: it waits; the list shows it at the top as Waiting with whose it is; Approve lets it in. Block signs it out.
8. As a member, Settings › Privacy starts with the sentence about the owner seeing plays.

**Operations**
9. General › Restart server: confirm; the row shows a spinner and "Waiting for the server to come back", then the success line. Something playing stops.
10. Storage & backups › Restore this backup: while it is made ready, a bar with "Copying the backup." then "Checking the backup.", then the existing restart wait and result.
11. General › Check for new versions: off, Save: the status row says no feed is set. On, Save: the check returns.
12. Troubleshooting › Client reports: send a report from an app; it is listed with what sent it, when and its size. Read shows the log; Download saves a `.json`; Delete asks, then removes it. With none: "No reports."

**Conversion and playback**
13. Server › Playback: Working space shows 4 (GB); set 8, Save, reload: 8. FFmpeg location: enter a path, Save: the row says it takes effect after a restart.
14. Settings › Quality on the web: "If something looks or sounds wrong" shows Always play in stereo, and Turn off HDR only on an HDR screen; no passthrough row, no Player row. Turn stereo on, then play a 5.1 title: the server plans stereo (check Now Playing or the session's audio).

**Live TV**
15. Sources: the three limit rows show 50,000 / 5,000,000 / 512 MB; lower Most channels below a source's size, Save, refresh that source: it lists fewer channels and a notice names how many were left out and why.
16. A source's settings: add a group name under "Only these groups", Save, refresh: only that group's channels remain. Remove it: all return.
17. Add a source whose playlist has a channel with no address: the preview shows the left-out notice under the counts.
18. Recording › Tuners while someone watches live and a recording runs: two lines, "Being watched · <channel>" and "Recording now · <title>"; the status reads "2 of 2 in use" in the warning tone.

**Watch Together**
19. Start a title while one member is still loading: the badge reads "Waiting for everyone". With a title a restricted profile cannot see: that profile's screen reads "Something you can't see on this profile". A member who hides their viewing reads "Private viewer".

**Titles**
20. A film with a Theatrical and a Director's Cut file: the Version control beside Play names each edition (if it does not, the picker's ids are not the file ids: S4, item 12 context).

---

# Third pass (13 October)

Fast-forwarded to `audit-implementation/main` (`e939fbb`), then five commits on `audit-implementation/console`. No server code changed. Checks: `tsc --noEmit` clean in `web/` and `packages/client-core`; string, glossary and design lints clean; my client-core test files pass (`settings-console-pages`, `catalogue-failures`, `title-split`, `attention-words`, `live-words`, `api-words`, plus `i18n` and `metadata-attention`); 24 of the 25 existing web test files that touch a changed screen pass. The one that fails, `web/test/maintenance-windows.test.ts`, fails on the base (it calls `backgroundPriorityOrDefault`, which the limits lane removed); I left it for the base-health lane. **Nothing was seen in a browser or against a running server.**

## T1. Outcome

**From the server follow-ups**
- **Delete one version**: "Delete this version…" on each file in a title's Files list and in an episode's panel (owners). The dialog shows the server's preview for that one file, asks for what the server says to type, and says the title and its other versions stay. A file the server kept says why.
- **Needs attention**: the badge and the tab count are the server's exact totals, read in one request for every library; "25+" is gone.
- **Missing**: a card whose title is missing says "Missing" (in its name too); other unplayable cards still say "Unavailable". The title page already said Missing.
- **Artwork**: Server › Metadata shows "<used> of <limit> used" from the settings document; the borrowed Storage figure is gone.
- **Storage & backups** gains three groups: **In use now** (streams, connections, analysis files, artwork, each against its limit, "Not measured yet" where the server has not measured); **Where things are kept** (database, temporary files and backups folders; space the database can give back and when it last did; each copy made before an upgrade, with Delete); **Downloads** (the total and each person's, with "Remove their downloads").
- **FFmpeg**: a status row under FFmpeg location: checked and in use, checked and waiting for a restart, or refused with the reason and what is missing.
- **Live TV source settings**: "Only these groups" and "Only these channels" are now pick lists read from the source (search, tick, Show more, "Bring in all"); the typed lists are gone.
- **Remote controls**: Settings › This device › "Playing on your other devices" lists what this profile is playing elsewhere with Pause, Resume and Stop, as the server allows for each; hidden while nothing is. A title the profile may not see is not named.
- **Role changes** apply at once through the shared client (the follow-ups lane); the web needed nothing, and S4 item 1 of the second pass no longer applies. No screen carried a note saying otherwise.

**Limits and settings**: "Pause background work for an hour / Resume" on Server › Schedule directly under Background work speed, with when the pause ends. The limits lane had already removed the separate priority row and added the four storage rows; I removed nothing further.

**Scans**: a library's Scanning tab has "Watch for library changes", and "What the last scan skipped" opens a paged list of each file or folder with the reason in a sentence. The Libraries list and an open library now follow a scan by themselves (count, state) instead of on reload.

**Rebuilds**: Troubleshooting › "Titles set aside" lists titles a rebuild could not finish, with the server's reason, how often they were tried and when; it is absent while there are none. I found no "being prepared" wording left for browsing or search (the two remaining uses are about a download and an account export, which are being prepared).

**Personal state**: after marking a show, season, album or book watched or unwatched on its page, a line says so with **Undo** for as long as the server offers it.

**Other**: a download asked for from a title page names the version chosen beside Play. The split dialog no longer searches the file list (the server answers for one file).

**Quiet updates (freshness)** wired on owner pages: the needs-attention list and badge, Missing titles, the Libraries list and an open library, Titles set aside, and every server settings form (a form being edited keeps what was typed).

## T2. Decisions

1. **Remote controls live in Settings › This device**, not in the player or the shell: the web's own "Play on" is the browser's AirPlay/Cast button and has no device list to attach them to.
2. **A person's downloads are removed without a typed confirmation**: nothing of theirs is lost that cannot be downloaded again.
3. **Titles set aside show the server's reason as it is**, in small monospace under a plain sentence: it is a diagnostic for the owner, like the logs beside it.
4. **Storage places are shown, not edited**: the folders are start-up choices.
5. **Undo is on the title's own page**, not after a mark made from a card's menu (the menu has closed by then and nothing on screen owns the offer).

## T3. Not done, and why

1. **Genres and Series paging, and limiting the sort control per grouping** (item 6). The web reads sorts from the server's own vocabulary per library and tab, and the integrator traced that it does not send a refused sort; I did not verify that further or change the library screen's windowing. Star ratings on the 0–10 scale were already fixed on the base by the integrator.
2. **Pause, resume and seek on the owner's Now Playing rows** (the admin controls route): not added; Stop is still the only owner control there. The service is `playbackControlsService.readAdmin/controlAdmin`.
3. **Seek** in the remote controls: only Pause, Resume and Stop.
4. **Freshness for Live TV, DVR, Downloads, keys, webhooks, devices, client reports and storage panels**: not wired; I did not find named feed dependencies for them and did not guess. Tuners and "In use now" keep their timers.
5. **Planner and review reports**: nothing in them named a web screen beyond what is above; the "approximate Dolby Vision" helper the planner lane wants removed is in the web player, which I did not touch.
6. **Undo from a card's menu**, and **Home "See all"**: the second was already wired by the web-app lane and the server now sends the destination; I changed nothing there.
7. Still open from earlier passes: per-source import limits, who is watching on a tuner (names), subtitle provider credentials.

## T4. Click-through (third pass)

As the owner unless a step says otherwise; desktop, then 375 px.

1. A film with two versions: Files shows "Delete this version…" on each. Open one: the preview names the size and path; Delete is disabled until the title is typed; after deleting, the page reloads with one version and the file is gone from disk. Delete the last version of another film: the title stays, marked Missing.
2. An episode with two versions: the same from the episode's panel.
3. A library with deletion turned off: the dialog says deleting is off and offers no field.
4. Libraries: the "N to match" badge equals the count in that library's Needs attention tab, with no "+". Match a title in another tab: the badge drops within about 30 seconds without a reload.
5. A missing film's card reads "Missing"; a film on an unplugged disk reads "Unavailable".
6. Server › Metadata: the artwork row reads "<used> of <limit> used".
7. Storage & backups: "In use now" shows streams and connections while something plays; "Where things are kept" shows three folders, the space line, and after an upgrade a copy row whose Delete asks first; "Downloads" lists each person with a size, and "Remove their downloads" empties that person's and their devices lose them when next online.
8. Server › Playback: enter a path that is not FFmpeg, Save: the status row says why it was refused and that Portico's own is in use. Enter a good build: "Checked. It will be used after the server restarts"; restart: "Checked and in use".
9. Server › Schedule: "Pause for an hour": the row says until when; a running scan stops taking new work; "Resume" clears it.
10. A Live TV source's settings: Choose under "Only these groups": the list shows the source's groups with channel counts; search narrows it; tick two, Save, refresh the source: only those remain. "Bring in all" clears the choice.
11. As a member, play something on a TV, then open Settings › This device in a browser on the same profile: the TV is listed with the title; Pause pauses the TV within a moment, Resume resumes, Stop ends it and the row leaves. With nothing playing elsewhere the group is absent.
12. A library › Scanning: turn "Watch for library changes" off, Save; adding a file is then picked up only by the next scan. "What the last scan skipped": add a `sample.mkv` and a broken symlink, scan: both are listed with a sentence each.
13. Start a scan and leave the library's page open: the Last scan row counts up and ends without a reload.
14. Troubleshooting: with a title the server could not rebuild (see the freshness lane's poison-title test), "Titles set aside" lists it with attempts and the reason; once fixed it leaves the list.
15. On a show's page, mark the show watched: "Marked as watched." with Undo; Undo within 30 seconds puts each episode back as it was, including half-watched ones. After 30 seconds the line is gone. The same on a season, an album and a book.
16. Choose the second version beside Play, then Download from the ⋯ menu: the prepared download is of that version.
17. Change a server setting from another browser while a Server page is open here: the row shows the new value within about 30 seconds; a row being edited here keeps what was typed.
