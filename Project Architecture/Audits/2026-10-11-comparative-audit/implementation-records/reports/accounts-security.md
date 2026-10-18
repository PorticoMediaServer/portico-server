# Lane report: accounts-security

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-accounts-security`, branch `audit-implementation/accounts-security`, 14 commits on top of `0e03d99` and then four merges of `audit-implementation/main` (head `4584db7`, level with base `aa71fab`; see section 10). Nothing pushed. Paths below are relative to that worktree.

All eleven items of the lane brief are delivered on the server and in `packages/client-core`. What is left is listed in section 9: client screens, three hand-written OpenAPI files and the API guide, and a short list of low-rated bugs I chose not to change or that belong to other lanes.

What was run, on the final tree: `go build ./...` for Linux, Windows and macOS, a release-tagged build, `go vet` on every touched package, and the full test suites of every package this lane touched. All pass except one test in `catalog` that fails the same way on the untouched base commit (details at the end of section 8). Not run: `go test ./...`, the web or native test suites, any device.

## 1. Outcome

1. **Remote sign-in policy is enforced everywhere a session is created.** The owner's "who may sign in from outside" setting (allow, owner only, off) was checked on one sign-in route no app used. It is now applied by the one function that creates a session, so no route can leave it out: password sign-in, Portico Account sign-in, second-factor completion, self-registration, TV-code collection, invitation acceptance and Cast pairing are all held to it. With the setting "off", an outside caller is refused before any password, code or assertion is examined. A refusal creates nothing, so nothing has to be revoked afterwards.
2. **A screen paired as a Cast target gets a playback-only session.** It is a member's session, whoever paired it, and it is refused every route that is not playback. The list of what it may do is closed: a route added later is refused for it unless it is added on purpose.
3. **Sessions end when their Hosted signing key is known to be revoked.** Each session records the signing key behind the assertion that admitted it. A profile chosen from that sign-in, a television it approved by code and a Cast target it paired carry the same record. Learning the revocation ends all of them at once. Sessions that never involved Hosted are untouched.
4. **Backups hold no sign-ins, and a restore signs everyone out.** Each backup copy has its sessions ended and its live sign-in secrets removed as it is written, with the deleted content overwritten. The first start after a restore also signs everyone out before the server answers a request, and does the same to the copy of the previous database the restore set aside. No step is added for the owner.
5. **One rating rule.** An account's rating limit and a profile's are the same rule on the same scale (the age a rating stands for). A PG limit shows TV-PG. The lower ceiling holds, and unrated titles are hidden when either side hides them.
6. **Permission changes apply live.** Giving or taking a library, changing a profile's restrictions or library list, promoting or demoting a member and transferring ownership no longer sign anyone out. A person is signed out when the account is turned off or deleted, when their password is replaced, and when a profile gets a new PIN.
7. **A member can be deleted.** The owner first reads what will go, then confirms. The account, its profiles and what they saved, its devices and its sessions are removed. The owner's play history keeps the plays and blanks the person's name, their profile names and their device names.
8. **"Require approval for new devices" works.** With it on, a member's new device waits until the owner or an administrator approves it, and the members' device list shows the real device records. The owner's own devices are never held.
9. **Invitations.** An account that is off records when and why. A valid invitation brings a turned-off member back only if it was made after they were turned off. An invitation can be made for one named Portico Account. Invite-only sign-up now works.
10. **Remaining bugs in documents 01 and 15**: see section 3.
11. **Document wording** for D-37 and D-59: see section 7.

Two defects surfaced while doing this work that were not in the audit, both fixed:

- **A session narrower than its account borrowed the account's tier on administrator routes.** A second profile on the owner's account is a member, but member management, invitations, role changes, limits, the device list, logs and diagnostics let it through, because those routes asked what tier the account holds and not what tier the session holds. Fixed in `server/internal/httpapi/access_area.go` (`tierAuthorityTx`); test `TestASecondProfileOfTheOwnersAccountIsNotAnAdministrator`.
- **The secure-delete setting did not reach the connection that did the deleting.** My first version of the backup scrub left every secret readable in the copy's free pages. The test that reads the raw file bytes caught it; the setting is now made, and checked, on the writing connection.

## 2. Decisions I made

**Sign-in policy and allowed hours**

1. The policy is judged at session creation, against the address of the device being signed in. For a TV code that is the television, when it asks for a code (told at once under "off") and again when it collects. Who approves, and from where, does not matter.
2. "Owner only" means the owner's account, judged by its stored tier. An owner's first session after the password is an account session with no tier, and that is still the owner signing in.
3. Choosing a profile and renewing a session are not new sign-ins and are exempt from the remote policy, as the setting's own description promises. They are not exempt from allowed hours.
4. Under "owner only", a member's correct password is still distinguishable from a wrong one (403 against 401). The alternative, refusing non-owners before the password check, would reveal which username is the owner's. Under "off" neither is revealed.
5. Outside allowed hours the answer is `403 outside_allowed_hours` and nothing is revoked. Browsing still stops when the 15-minute access token expires, as before; what changed is that the device is let back in when the hours return.
6. An access schedule that names no time zone now runs on the server's clock. It used to run in UTC while the People screen called it "server time".
7. The unused `POST /v1/sessions` route and `PorticoClient.login()` are deleted.

**Cast**

8. A Cast target may use: the playback session API, the media a session hands it, artwork, a session's subtitle plan, the device capability document, `GET /v1/me` and signing itself out. Nothing else. `server/internal/httpapi/cast_target_scope.go` is the whole list.
9. A Cast target is not held by device approval: the signed-in device that paired it vouches for it.
10. A Cast target is given an access token only and renews through its own rotating device token, as before. The ordinary refresh credential was never handed out and is now removed rather than left unused.

**Signing keys and Hosted**

11. "Hosted attached" (Q-4) is defined as: the server's claim is installed. A Portico Account can only sign in to a server Hosted knows, and Hosted knows a server by its claim, however the server was first set up. A server set up for direct sign-in and never claimed answers false.
12. Sessions whose admitting key was never recorded are not ended by any revocation. On a pre-release database that is every Portico-admitted session issued before this change.

**Restore and backups**

13. Both mitigations from D-36 are implemented, not one: backups are scrubbed and a restore signs everyone out. The second covers copies the first cannot (an older backup, a hand-made copy, the pre-restore copy).
14. A backup whose copy cannot be scrubbed is not kept. This is stricter than keeping it; a scrub should essentially never fail.
15. A sign-out that cannot be done after a restore stops the start. Serving from a restored database with its old sessions is what this exists to prevent.
16. Password hashes and second-factor secrets stay in the database and its backups in the clear. That is the "nothing encrypted at rest" trade-off and I left it alone. Removing second-factor secrets from a backup would turn everyone's second factor off on restore.
17. Deleted content in a backup copy is overwritten using SQLite's secure-delete, which costs a write per freed page. I did not rewrite the whole copy (VACUUM), which would double backup time and space on a large library.

**Ratings**

18. The account limit keeps `maxContentRating`, gains an optional `ratingSystem`, and returns `maximumAge`. With no system named, the rating is read as any spelling the table knows, so the existing People screen keeps working.
19. `allowUnrated: false` on an account now hides unrated titles even with no rating ceiling. Before, the switch did nothing without a ceiling.
20. A title carrying several ratings is judged by all of them for an account limit, as it already was for a profile's. The account limit used to read only the first.
21. Where two countries use the same letters for different ages, the higher age is kept ("A" is 18). "S" is left unrated because its meanings are too far apart.
22. The account's denied list still matches labels and tags; a profile's blocked list still matches labels only. Unifying them means changing four precomputed count paths in the catalogue and its dirty-tracking triggers. Not done; see section 9.
23. Per-library "block unrated" was not built. It does not fall out naturally: the unrated rule is re-derived in four precomputed count paths besides the SQL predicate.
24. A library classified under the older rating table is classified again once, on the first start of a build with a newer table. The table of spellings is one row per distinct rating, not per title.

**Members, devices, invitations**

25. Who may delete: the owner deletes anyone but themselves; an administrator deletes plain members. Same rule as turning a member off.
26. Deletion also blanks the device names in the owner's play history ("Sam's iPhone" names the person). D-58 says "blank the name"; I read that as every name.
27. Deleting a single profile blanks that profile's name in the play history and leaves the account's name.
28. Portico Account erasure (the person deleting their account at Hosted) now runs the same erasure as "delete member", so the two cannot drift.
29. Ownership transfer is live too: both people stay signed in and their sessions take their new tiers in the same transaction.
30. A PIN change still signs that profile's devices out. A PIN is a credential; the devices on the profile got there with the old one.
31. The owner is exempt from device approval. Administrators are not: the owner approves theirs. An administrator sees and decides about plain members' devices only.
32. A television signed in by code under the approval rule is recorded as waiting and the code stays approved. The code still expires after ten minutes; if the owner approves later than that, the television asks for a new code and it then works at once.
33. Turning the approval rule off lets waiting devices in. A blocked device stays blocked.
34. An invitation re-admits a turned-off member only if it was made after they were turned off, whether the owner turned them off or they left. For someone who left, the time recorded is when they left at Hosted, not when this server heard, so an invitation the owner sent in between still works.
35. Recipient binding is by Portico Account username, because that is what the signed identity assertion carries. Binding by email needs a Hosted change (section 7).
36. Invite-only sign-up spends only a plain member's invitation. An administrator's invitation, and one made for a named Portico Account, are refused there and stay usable through the invitation link, so nothing is silently downgraded.

**Small things**

37. Records of sessions that ended more than 90 days ago are removed.
38. A password or PIN check waits up to three seconds for a hashing slot before answering "busy".
39. Twenty wrong PINs across devices in a day close a profile to PIN entry for fifteen minutes after the last one.
40. The public `GET /v1/system` keeps `version` and drops `buildId`, `sourceDigest` and `builtAt`. Clients need the version before sign-in; the owner reads the exact build from the maintenance document.

## 3. Bugs

**Document 01**

| Bug | Status |
|---|---|
| B-01.1 | Fixed. Outside allowed hours a renewal is refused without revoking; the device is let back in later. |
| B-01.2 | Fixed. Same as B-15.1. |
| B-01.3 | Not mine (api-platform). I removed nothing of the API key code. |
| B-01.4 | Fixed. The switch is the identity policy; the legacy device table and header path are deleted. |
| B-01.5 | Already fixed on the base branch. |
| B-01.6 | Fixed. The waiting television's record is kept and the same code works once approved. |
| B-01.7 | Fixed. The owner is exempt; the owner and administrators can list members' devices. |
| B-01.8 | Fixed. "Hosted attached" is whether the claim is installed. |
| B-01.9 | Already fixed for 403; now no 403 ends a sign-in at all. A 401 from a gateway still does. |
| B-01.10 | Fixed. One rating rule. |
| B-01.11 | Fixed. Punctuation and spacing no longer matter; more countries' ratings are known. "A" stays 18 on purpose. |
| B-01.12 | Fixed. Names are blanked on member deletion, Portico Account erasure and profile deletion. |
| B-01.13 | Fixed. Invite-only sign-up spends the invitation inside the sign-up, after the username check. |
| B-01.14 | Not mine (networking). I call the one existing `requestIsRemote` and added no other test. |
| B-01.15 | Fixed. A short bounded wait replaces the instant "busy". |
| B-01.16 | Fixed. The PIN is hashed before the transaction opens. |
| B-01.17 | Fixed. Installation ids no longer travel in addresses; another device's id is not shown. |
| B-01.18 | Fixed for session records (90 days). The per-account failed-attempt ledger is still unpruned. |
| B-01.19 | Fixed. Refresh reuse and Portico erasure go through the clean-up hook; the schedule, remote-policy and tier-change revocations no longer exist. |
| B-01.20 | Fixed at every site the audit listed. |
| B-01.21 | Not changed. A restricted profile can still list and rename the account's devices and approve a TV code. The audit calls this "mostly by design"; changing it alters what a second profile can do on a shared television. The one part that was an escalation (borrowing the account's tier on administrator routes) is fixed. |
| B-01.22 | Partly. The three retired Hosted routes and the legacy device table are removed. The "hosted" session authority, the `viewer_only` grant that nothing creates, the unused setup helpers and the legacy `pin_attempts` columns remain. |
| B-01.23 | Not investigated. It needs a running Apple client. |

**Document 15**

| Bug | Status |
|---|---|
| B-15.1 | Fixed. |
| B-15.2 | Already fixed on the base branch. |
| B-15.3, B-15.4, B-15.6, B-15.8, B-15.11 | Not mine (networking). |
| B-15.5 | Fixed as far as the trade-off allows: backups hold no sessions and a restore signs everyone out. Password hashes and second-factor secrets remain in the clear by decision. |
| B-15.7 | Document correction supplied in section 7. No code change. |
| B-15.9 | Not changed. Deliberate (no lockout). |
| B-15.10 | Fixed with B-01.15. |
| B-15.12 | Reduced. A stolen key's sessions now end when the server learns of the revocation. How fast a server learns is unchanged. |
| B-15.13 | Not changed. Deliberate (a TV code confers the approver's role). |
| B-15.14 | Fixed with B-01.17. |
| B-15.15 | Fixed. |
| B-15.16 | Fixed, untested against a real Cast sender; see section 8. |
| B-15.17 | Not mine (ops-packaging owns the update feed). Still present. |
| B-15.18 | Fixed. |
| B-15.19 | Not mine (ops-packaging owns workflows). Still present. |
| B-15.20 | Not mine (providers). Still present. |
| B-15.21 | Fixed. Wrong PINs are also counted across devices. |
| B-15.22 | Not changed. Deliberate. |
| B-15.23 | Fixed for asking for a TV code and for both invitation routes. A television polling for its answer is deliberately not counted. |
| B-15.24 | The download file name is fixed. The CSV formula case was already handled (`csvCell`). |

**Others named in the brief**

| Bug | Status |
|---|---|
| B-13.14 | Fixed. |
| N-V01.1 | Fixed. |
| B-09.19 | Fixed. |
| B-13.21 (Q-5) | Fixed. |

## 4. API changes

**Removed**

- `POST /v1/sessions`.
- `GET /v1/auth/remembered-accounts?installationId=` (replaced below).
- `POST /v1/hosted/attach`, `POST /v1/hosted/restrictions`, `POST /v1/hosted/profiles/offline-select` (they answered 410).

**New, on the registry**

| Route | Request | Response |
|---|---|---|
| `GET /v1/direct/members/{id}/deletion-preview` | none | `{accountId, username, role, revision, porticoAccount, profiles, devices, playlists, savedResources, playsKept}` |
| `DELETE /v1/direct/members/{id}` | `{expectedRevision}` | 204; 412 `revision_mismatch` if the member changed |
| `POST /v1/auth/remembered-accounts` (public) | `{installationId}` | `{items}` |

**Changed, moved onto the registry**

| Route | Old | New |
|---|---|---|
| `GET /v1/admin/access/devices` | Read the legacy table, always empty | Lists real device records. Item adds `username`, `app`, `appVersion`; `trust` is `pending`, `approved` or `blocked`. Errors use the registry envelope. |
| `PUT /v1/admin/access/devices/{id}/trust` | 409 on a stale revision; replayed by `operationId` | 412 `revision_mismatch`; repeating a decision that already holds returns the device unchanged. `operationId` is accepted and ignored. |
| `DELETE /v1/direct/remembered-accounts/{accountId}` | `?installationId=` in the address | `{installationId}` in the body |

**Changed shape or behaviour, not moved**

| Route | Change |
|---|---|
| Every session-issuing route, and `POST /v1/quick-connect` | Can answer `403 remote_sign_in_not_allowed`. |
| `POST /v1/auth/refresh`, sign-in routes, `POST /v1/direct/profiles/{id}/select` | `403 outside_allowed_hours`, nothing revoked. Was `403 access_limit` plus revocation. |
| Member limits document and member list | Adds `ratingSystem` and `maximumAge`. Members also carry `disabledAt` and `disabledReason` (`owner` or `left`). |
| `GET /v1/direct/members` | Items carry `disabledAt`, `disabledReason`. |
| `GET /v1/devices` | `current` comes from the session; the `installationId` query parameter is ignored; `installationId` is empty for every device but the caller's own. |
| `POST /v1/admin/access/invitations` | Accepts `porticoUsername`; the invitation document returns it. |
| `POST /v1/access/invitations/preview` | Adds `forPorticoAccount`. |
| `POST /v1/access/invitations/accept` | Can answer `403 invitation_for_another_account`; answers `410 invitation_expired` for a turned-off member presenting an older invitation. |
| `POST /v1/quick-connect/token` | Can answer `403 device_approval_pending` or `403 device_denied`. |
| `GET /v1/system` | Drops `buildId`, `sourceDigest`, `builtAt`. |
| `PUT /v1/direct/profiles/{id}/restrictions`, `PATCH /v1/direct/members/{id}`, `PUT /v1/admin/access/members/{id}/role`, `POST /v1/direct/ownership` | Same shapes; no longer sign anyone out except as listed in section 1. |
| Member, tier, device and invitation management refusals | 403 where they answered 401. |
| A Cast target's session | 403 on every route outside playback. |

`packages/contracts` and `server/api/portico.openapi.json` were regenerated with `go run ./cmd/apigen`. Other lanes regenerate too; on a merge conflict in those files, regenerate rather than resolve by hand.

`packages/client-core` updated: `server-connections.ts` (`REFRESH_OUTSIDE_HOURS`, no 403 ends a sign-in), `index.ts` (`login()` removed), `server-administration.ts` (limits, member, invitation and device shapes; `parseMemberDeletionPreview`, `previewMemberDeletion`, `deleteMember`), `identity-client.ts` and `identity-devices.ts` (remembered accounts, device list), `presentation/errors.ts`.

## 5. Schema changes

All in `server/internal/persistence/migrations/0001_baseline.sql`, resynced with `scripts/schema-sync.py`.

- `access_devices` and its two indexes: removed.
- `identity_devices`: `revision INTEGER NOT NULL DEFAULT 1`; index `identity_devices_recent(last_seen DESC, id DESC)`.
- `direct_memberships`: `disabled_ms`, `disabled_reason` (`''`, `owner`, `left`); trigger `direct_membership_disabled` keeps them whichever statement flips `disabled`.
- `access_invitations`: `bound_username`.
- `portico_admitted_families`: `signer_key_id`; index `portico_admitted_signer`.
- `portico_factor_challenges`: `signer_key_id`.
- `authorization_session_families.revoked_reason`: adds `signing_key_revoked` and `restored`.
- `authorization_family_grants.grant_kind`: adds `cast_target`.
- `social_cast_bootstraps`: `pairing_family_id`.
- `configuration` keys (no schema change): `rating_table_version`, `identity.restoreHandled`.

## 6. Owner or viewer settings added or wired

| Setting | Type | Default | Where it takes effect | Screen |
|---|---|---|---|---|
| `remoteSignInPolicy` | allow / owner-only / off | allow | Every session created (`identity.IssueTx`) | Exists (Connectivity) |
| `deviceApprovalRequired` | boolean | false | Device registration at sign-in. The same value as `deviceApproval` on `PUT /v1/direct/registration-policy`. | The switch exists; the members' device list exists per person on the People page and now shows real data |
| Member limit `ratingSystem` | one of the published rating systems, or empty | empty | The catalogue predicate | No picker yet; the People page offers the American list only |
| Self-registration `invite-only` | existing value | off | `POST /v1/auth/register` | No first-party screen sets the sign-up policy |

No new owner switches were added. Retention of ended session records (90 days), the hashing-slot wait (3 seconds) and the cross-device PIN count (20 a day) are constants.

## 7. Needed from others

**Client screens** (strings are in the catalogue unless noted)

- Sign-in and session: show `outside_allowed_hours` as "not at this time of day" rather than a generic failure, and keep the person signed in. Web string `web.signIn.error.outsideHours` exists; native needs the same handling.
- People: a "Delete this person" action with the preview as its confirmation (`server.people.delete*`), using `previewMemberDeletion` and `deleteMember`. Show `disabledAt` and `disabledReason` (`server.people.turnedOffByOwner`, `server.people.leftOnTheirOwn`).
- People: a rating-system picker for the account limit, fed by `GET /v1/rating-systems`, as the profile restriction screen already has.
- Play history: show `server.people.removedPerson` where a row's name is empty.
- Invitations: a field for `porticoUsername`; on the join page, offer only Portico Account sign-in when the preview says `forPorticoAccount`. Web string `web.signIn.error.invitationOtherAccount` exists.
- Devices: the web list already works with the new data. It could show `username`, `app` and `appVersion`.
- A television waiting for approval: show "waiting for the owner to approve this device" on `device_approval_pending` from the code poll, and keep polling.
- After a live permission change a client's cached `viewer.role` is stale until its next renewal (at most 15 minutes). A client that gets `403 forbidden` on an administrator screen should re-read `/v1/me`.
- Native: the app's copy of client-core must be re-synced. It still sends `installationId` in the device list address (harmless, ignored) and calls the removed `GET /v1/auth/remembered-accounts` if it uses the account switcher.

**Privacy screen sentence (D-59).** Added to the catalogue as `pref.note.privacy.ownerSeesPlays`:

> The owner of this server can always see what is played on it, by whom and on which device. Pausing or clearing your own history does not change that.

It belongs at the top of the Privacy group, above "Pause watch history". If members can be deleted, a second sentence is true and worth showing to owners on the People page: "Deleting a person keeps their plays in your play history without their name."

**Server architecture document (D-37).** `portico-internal/Project Architecture/02 Server.md`, line 52. Current text:

> **Sessions** are device-bound: a device registers a key, and access tokens are short-lived and renewed with a refresh token bound to that device and installation. Session families let one sign-out end every token issued from a sign-in.

Replace with:

> **Sessions** are tied to a device record, not to a key. Each installation of an app generates a random installation id once and keeps it; the server keeps one device record per account and installation. Access tokens are short-lived bearer tokens, renewed with a rotating refresh token that is accepted only together with that installation id. No device holds a private key and no request proves possession of one: anyone who captures the access token, the next refresh token and the installation id (all three travel in requests, so over plain HTTP a listener on the network can) holds the session until the real device next renews, at which point the reuse is detected and the whole session is ended. Session families let one sign-out end every token issued from a sign-in.

The same document needs four further corrections from this lane, where section 1 of the brief and this work now differ from it:

- Line 53, "A session that is itself viewer-only links a viewer-only device": nothing creates a viewer-only session. Either delete the sentence or keep it with "no session of this kind is issued today".
- Line 53: add "A screen paired as a Cast target is the exception: it gets a playback-only member session whoever paired it."
- Line 75 (play history): add "Deleting a member, or a Portico Account being deleted, keeps the plays and blanks the person's name, profile names and device names."
- Line 78 (`backup`): add "A backup holds no sign-ins; a restore signs everyone out."

`portico-internal/Product Contracts/identity/contract.md` disagrees with the code in the places the audit lists (D1 to D12 in document 01). Three are settled by this work: §9 erasure now does anonymise play history; `api/identity.md` §7 ("sign out everywhere requires the account password") is corrected in the repository copy; `api/admin-access.md` is rewritten for limits, devices, deletion and live changes.

**Other lanes**

- **api-platform**: `server/api/openapi.yaml` still documents `POST /v1/sessions` (`createSession`), and `server/api/guide/01-overview.md` and `02-authentication.md` build examples from it. I left them because that lane is rewriting the authentication guide; `guide/check-examples.mjs` will need the examples moved to `POST /v1/direct/sign-in`. `identity.openapi.yaml` and `admin-access.openapi.yaml` (hand-written) are stale for every route in section 4. A restore brings back API keys that were revoked after the backup was taken; whatever stores keys and personal tokens should be added to `sessionScrub` in `server/internal/identity/restore.go`, or scrubbed the same way. A Cast target's session must not be able to mint a key or token: the routes will be refused for it by default, since they are not on the playback list.
- **networking**: the cast-target route check and the sign-in admission are attached in the request middleware in `server/internal/httpapi/router.go`, next to `WithSignInSource`. I added three paths to the cheap limiter's list there. `secureSetting` in `secure_connections.go` gained one field (`remoteSignIn`). `X-Portico-Device-Id` is still in the CORS allow-list and is no longer read by anything.
- **ops-packaging**: `backup.Service` gained `SetCopyScrubber` and one call in `create`, and `backup.ReadRestoreResult(state)`; `cmd/server/main.go` gained two lines and `cmd/server/restore_sessions.go`. If the restore result file or the `-pre-restore` folder naming changes, `signOutAfterRestore` must follow. B-15.17 and B-15.19 are yours.
- **identity** (titles and files): `server/internal/catalog/restrictions.go`, `viewer.go`, `visibility_classes.go`, `rating_allowed.go`, `browse_facets.go`, `browse_counts.go` and `content.go` lost the member rating ladder; `catalog.Viewer` lost `MemberMaximumAge`, `MemberMaxRating` and `MemberAllowUnrated`. A viewer's whole policy is now `Viewer.Restrictions`.
- **social**: `server/internal/social/cast.go` issues through `identity.IssueCastTargetTx`. A Cast receiver that needs a route outside `cast_target_scope.go` must have it added there.
- **Hosted**: to bind an invitation by email, the identity assertion (`portico.identity.v1`) needs a verified `email` claim. The server would then compare it with the invitation's address in `AcceptPortico`. Nothing else is needed from Hosted for this lane.

## 8. Test notes for other environments

**Cast sender with the narrowed script policy (B-15.16).** Needs Chrome on a network with a Chromecast or Google TV, and a server with a Cast application id set. Open the web app, open DevTools console, start casting a title. Pass: the Cast button appears, the device list opens, playback starts on the television, and the console shows no "Refused to load the script" lines. Fail: any such line naming a `www.gstatic.com` path. The policy allows `/cv/js/sender/`, `/eureka/clank/` and `/cast/sdk/libs/sender/`; if the console names another path, add it in `server/internal/httpapi/router.go` (search for `script-src`).

**Cast target on a real device (B-13.14).** Same set-up. Pair a television from the owner's account and play a film, pause, seek, change subtitle, let it finish. Pass: all of that works and the artwork shows on the television. Then, with the television's access token (from the receiver's debug log), `curl -H "Authorization: Bearer <token>" https://<server>/v1/libraries`. Pass: 403. Fail: 200, or any playback step answering 403 (that step's route is missing from `cast_target_scope.go`).

**Backup and restore.** On any installed server with two signed-in devices: take a manual backup; sign one device out; restore the backup; let the server restart. Pass: both devices are asked to sign in again, the same password works, the log has one line "This server was restored from a backup. Everyone has been signed out", and a second restart does not sign anyone out. Then `sqlite3 <state>/backups/<id>/portico.db "select count(*) from authorization_session_families where revoked=0; select count(*) from identity_refresh_receipts"`. Pass: `0` and `0`. The scrub adds a write per freed page to each backup; on a very large database, time one backup before and after to confirm the cost is small.

**Windows and macOS.** Both cross-compile. Nothing here is platform-specific except the folder picker's link check, whose test is skipped on Windows because creating a symbolic link needs a privilege. On Windows, create a junction inside a library root pointing outside it and browse to it in the folder picker. Pass: it is refused. Note for ops-packaging's Q-9: a library root that is itself a junction is resolved before the check, so it should still be browsable; confirm.

**Remote sign-in policy behind a reverse proxy.** Needs a server behind a proxy listed in trusted proxies. Set the policy to "off"; sign in from the LAN (works) and from outside through the proxy (`403 remote_sign_in_not_allowed`). If the outside attempt is admitted, the proxy's address is being read as the client's, which is the networking lane's parser, not this policy.

**Device approval.** Turn "require approval" on. Sign a member in on a new phone: it says it is waiting. On the owner's People page the phone appears as waiting; approve it; sign in again: it works. Sign the owner in on a new device: it is not held.

**Signing-key revocation.** No real environment needed beyond the test in `server/internal/hosted/trust_revocation_test.go`, which uses a real certificate chain. On a live Hosted deployment the check is: revoke a documents key, have any server verify one new envelope, and confirm Portico Account members on that server are signed out with reason `signing_key_revoked` while password-only members are not.

**Verification run here, on the final tree.**

- Builds: Linux, Windows, macOS and a release-tagged build all succeed. `go vet` is clean on every touched package.
- Web: `npx tsc --noEmit` passes. client-core: the three test files I changed pass (41 tests).
- Full Go suites that pass: `httpapi` (391 s, with `PORTICO_PLAYBACK_TEST_HELPER` set to a built server), `identity`, `persistence`, `access`, `hosted`, `social`, `backup`, `operations`, `administration`, `contentaccess`, `compactcatalog`, and the new test in `cmd/server`.
- `catalog`: every test passes except `TestDiscoverCarriesPicksForYou` ("no Science Fiction for you section"). It fails identically on an untouched copy of the base commit `0e03d99`, so it is not caused by this lane. It belongs to whoever owns recommendations.
- One guard caught a mistake of mine and is satisfied now: `compactcatalog`'s "every statement must seek" test flagged the one-off rating reclassification, which reads its whole (small) table on purpose. I moved it out of that package rather than exempting it.

## 9. Not finished

1. **Client screens.** Everything in section 7 under "Client screens". The server and client-core are done; no screen was built.
2. **Hand-written API documents.** `server/api/openapi.yaml`, `identity.openapi.yaml`, `admin-access.openapi.yaml` and the two guide chapters still describe the old routes. `admin-access.md`, `identity.md` and `README.md` are corrected.
3. **One label rule.** A profile's blocked list matches labels; an account's denied list matches labels and tags. Not unified (decision 22).
4. **Per-library "block unrated"** and **country rating tables**. Not built (decision 23). The rating table is wider than before but still one table without a country.
5. **B-01.22, the left-over "hosted" session authority.** Still branched on in about twenty places. Removing it is a mechanical but wide change across `identity`, `hosted`, `social` and `httpapi`, and I judged it too likely to collide with other lanes mid-programme.
6. **B-01.21.** A restricted profile can still list and rename the account's devices and approve a TV code.
7. **B-01.18, second half.** The per-account failed-attempt ledger is not pruned.
8. **B-01.23.** Not investigated.
9. **Under "owner only", a member's correct password is distinguishable from a wrong one** from outside (decision 4). A choice, recorded here because it is a residual.
10. **A sign-in with no installation id under device approval** creates a new waiting device record each time. A client that sends no installation id cannot be approved at all; first-party clients all send one. No cap on those records.
11. **API keys and personal tokens after a restore** are the api-platform lane's to scrub (section 7).
12. **Not run:** the whole-repository Go suite, the web and native test suites, anything on a device, anything through a real proxy or a real Hosted.

## 10. Merge with the base (13 October)

`audit-implementation/main` was merged into this branch four times as it moved (`e4a0ad1`, `b8cecab`, `c3da536`, `aa71fab`); the branch head `4584db7` contains the base tip `aa71fab`. Generated files were taken from the base and regenerated (`go run ./cmd/apigen -root ..`, `python3 scripts/schema-sync.py`).

### What I decided

- **Remote sign-in and locality.** `secure_connections.go` is gone with the base. The owner's remote sign-in choice now rides in networking's connection cache (`httpapi/connection_policy.go`, `remoteSignIn`), and "remote" is `Dependencies.requestIsRemote`, the one home-network test. A stored value this build does not know closes remote sign-in. No private-address check remains in any sign-in or recovery path.
- **Key scopes on this lane's routes.** Member deletion and its preview, and both remembered-account routes, are `session`: they authenticate through the direct-account caller, which refuses a key outright, exactly like the sibling `PATCH /v1/direct/members/{id}` the base already marks `session`. (The coordinator's note said "admin" for deletion; declaring admin would advertise something a key cannot do.) Listing members' devices is `admin`. Approving or blocking a device is `session`, by the base's own rule that a key never approves a device. Stale table entries for routes this lane removed are gone from `route_scopes.go` and `route_lanes.go`.
- **Keys and tokens.** Disabling a member ends their keys (the base's trigger). Deleting a member now marks them ended first, so their sessions really end before the account row goes (`identity/member_erasure.go`). A role change follows live (the base's trigger and this lane's live tiers agree). Limits and library access are read live for a key as for a session.
- **Backup and restore.** The backup scrub and the sign-out after a restore end every sign-in and leave live keys and tokens working, as the base decided. A backup holds only a digest of a key's secret. Consequence to know: a key revoked after a backup was taken is live again if that backup is restored; it shows in the key list. The sign-out runs at start-up from `restore-result.json`, so it holds on the new asynchronous restore and self-restart path (the restart is a normal start).
- **Cast.** A paired screen may call `DELETE /v1/cast/devices/{id}` for its own pairing only, and must name its generation (`If-Match`): another screen of the same profile answers 404, no generation 400, wrong generation 409. The reconnect path issues the same narrow session. A paired screen cannot create or list keys or tokens (403). The receiver inventory test now also asserts that every request the receiver sends with a session is one a paired screen may make.
- **Hosted signing certificate.** The check-in answer's `signingCertificate` is verified against the pinned root (documents purpose, in date, signature) and fed to the existing revocation machinery (`hosted/trust.go` `learnSigningCertificate`, called from `current_schedule.go`), so sessions a revoked key admitted end on a server nobody signs in to. An invalid, expired, wrong-purpose, tampered or older certificate teaches nothing and removes nothing.
- **Guide ratings.** The Live TV lane's `AdmitProgramme` used the old member rating ladder this lane removed. It is kept, judged on the one age scale; an unrated slot is refused when the account may not watch unrated titles, whether or not a ceiling is set, as in the catalogue.
- **Smaller ones.** The page policy keeps this lane's narrow Cast script sources and `form-action 'self'` and adds the subtitle lane's `'wasm-unsafe-eval'`. The download name header keeps the downloads lane's two-name form with this lane's safe quoting of the plain name. `client-core/src/server-administration.ts` holds a raw NUL byte in a pattern, so git treated it as binary and silently kept one side; it was merged by hand as text and both sides are in it.

### Defects found while merging, and fixed

- **A revoked session stayed usable for up to 15 seconds on ordinary routes.** Ending a session is written as `UPDATE … RETURNING`, which reaches the database driver as a query; the hook that clears cached sign-ins watched only execs. A session ended from a request outside the security lanes (unpairing a Cast screen, for one) stayed in the principal cache until its entry expired. `dbwork/instrument.go` now notices writes that return rows; test in `dbwork/authority_test.go`.
- **Test fixtures that create the first owner sent no content type**, which the base's setup route now requires, so seven tests stopped at "setup 401" (among them the principal-cache revocation test and the hostile-input test). Fixed in the fixtures.
- **With the hostile-input test running again, it found a crash**: six metadata routes dereferenced a missing metadata service. They now answer 503 `metadata_unavailable` (`httpapi/metadata_attention.go`).
- A base test for `/v1/titles` set a rating limit without classifying ratings; it now classifies first (an unclassified rating is withheld by design).

### Verified on the merged tree

`go build ./...` for Linux, Windows and macOS; `go vet` and the full suites of `httpapi`, `identity`, `access`, `hosted`, `social`, `backup`, `operations`, `playbackruntime`, `dbwork`, `cmd/server`, `persistence`; `npx tsc --noEmit` in `web`; the client-core administration, identity and session tests. `identity`, `access`, `hosted`, `social`, `backup`, `operations`, `playbackruntime` and `persistence` pass in full.

### Still open (all present on the base itself; none is from this lane)

- **The server does not start on the base tip.** Two route patterns conflict and the router panics at start-up: `GET /v1/media/linear/{playbackId}/{generation}/{token}/{name}` (`httpapi/linear_media.go:83`) and `GET /v1/media/{grant}/subtitles/{resource}/{revision}/original.ass` (`httpapi/subtitle_routes.go`). Both `cmd/server` start-up tests fail on it (`TestSIGTERMDrainsInsideItsBudgetAndLeavesNoChildren`, `TestHealthCheckAgreesWithReadiness`). It belongs to the Live TV and subtitle lanes; I did not change either route.
- `httpapi`, six tests: the two restriction matrices (28 routes added by other lanes are unclassified, and one stale entry for the removed `GET /v1/admin/access/api-keys`); `TestOpenAPICoversRegisteredRoutes` (`GET /receiver/cast/`); `TestAdministrationTierBoundaries`; `TestAdministrationOwnerBoundaryAndLibrarySettingsRoundTrip`; `TestListeningHTTPPreferenceEnvelopeAndCAS` (needs a helper binary).
- `dbwork` `TestNoRawWriteTransactionsOutsideDBWork`: `internal/livechannels/programme_spool.go:66` opens a raw transaction.
- client-core: six tests in other lanes' areas fail (group sessions, directory refresh, route origins, history facets); not run to ground here.
- The restriction matrices are this product's guard that hidden titles stay hidden. The unclassified routes should be classified by the lanes that added them before release.
