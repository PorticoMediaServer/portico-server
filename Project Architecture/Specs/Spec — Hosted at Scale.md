# Spec — Hosted at Scale (identity + discovery; the server owns membership)

October status: the architecture below supplies the server-owned membership/discovery design. The current Hosted baseline is revision 21, fresh-only; older migration/schema numbers are historical implementation notes, not supported adoption. Public-client Sign in with Portico now uses reviewed S256 consent/code exchange returning only a server-bound assertion. Runtime signing and bounded revocation wakes/check-in have source fixes; real browser/provider/OS/fleet acceptance and apikit synchronisation remain open. Sources: [Hosted](../../../_program/reports/hosted.md), [Hosted review fixes](../../../_program/reports/review-hosted.md), [Hosted follow-ups](../../../_program/reports/hosted-followups.md). Capacity arithmetic below remains design evidence, not a measured release guarantee.

Justin's rule (23 Sep): clients reach Hosted only to sign in and for a handful of rare cases; the user's
server is the authority for who may use it; Hosted must stay economical at hundreds of thousands to a
few million users on **one** VPS; everything already set up keeps working while Hosted is down.

## 1. What changes, in one paragraph

Today Hosted *owns* membership: invitations are accepted on Hosted, Hosted keeps members, profiles,
PINs and library grants, signs a 90-day membership policy per server, wakes the server to pull it, and
every first connection of a device redeems an opaque ticket that the server must take back to Hosted
online. After this change Hosted only **proves who a person is** (a short-lived signed identity
assertion the server verifies offline) and keeps a **discovery index** ("which servers is account A a
member of") that the servers themselves keep current by pushing sequence-numbered deltas. A Portico
Account member of a server is an ordinary server account (same tables, profiles, PINs, restrictions,
limits, sessions and devices as a direct member) that is *linked* to a Hosted account instead of
having a password. Devices learn about new servers with a conditional version check answered from
Hosted's memory.

## 2. Authority table

| Fact | Authority | Replica / cache | Notes |
|---|---|---|---|
| Identity (account id, username, display name, email, password, MFA, provider links, Hosted sign-in sessions) | **Hosted** | none | Unchanged. |
| Account deletion | **Hosted** | erasure jobs to affected servers | Existing signed `portico.account-erasure.v1` jobs. |
| Membership (is A a member of S, role owner/admin/member, disabled) | **Server** (`account_portico_links` + `direct_memberships`) | Hosted discovery index `server_members` | Server pushes deltas; Hosted never decides access. |
| Profiles, PINs, profile trust, library allow-lists, parental restrictions, access limits | **Server** | none | Exactly the direct-member tables. Hosted account profiles stop being used for server access. |
| Server sessions, devices, sign-out, session review | **Server** | none | Direct-member machinery (`authority='local'` families). |
| Invitations | **Server** (`access_invitations`) | none | No email delivery; the owner shares the link. |
| Server list per account | **Hosted discovery index** (a replica of server facts) | device keeps the list + version | Version kept in Hosted memory. |
| Server identity, claim, routes, DNS, certificates, members-only DNS label | **Hosted** | server caches | Unchanged. |

"Portico Account member" on the server = an `accounts` row linked to a Hosted account, empty password,
no local second factor (Hosted enforces MFA at Hosted sign-in). The link lives in its own table
(`account_portico_links`), sessions are `authority:"local"`, and the sign-in snapshot's account carries
`hostedAccountId`, so every server feature treats it as a direct member.

## 3. Endpoints, triggers and frequency

### Client → Hosted

| Endpoint | Auth | Trigger | Expected frequency |
|---|---|---|---|
| `POST /v1/sessions`, OIDC, registration, MFA | none/flow | person signs in | once per device, rarely again |
| `POST /v1/sessions/refresh` | refresh token | access token expired *and* a Hosted call is needed; keep-alive once ⅓ of the 90-day window is used | ~12 / client / year |
| **`GET /v1/server-list/{watch}/{version}`** (new) | watch token (HMAC, no DB) | app foreground, only if ≥ 6 h (jittered 5–7 h) since the last check; "Refresh Servers"; after accepting an invitation | ≤ 4 / device / day; 304 almost always |
| `GET /v1/servers` | account bearer | only when the version check says "changed", at sign-in, "Refresh Servers" when changed | on change only |
| **`POST /v1/servers/{id}/identity`** (new) | account bearer | device has no valid session on that server (first connect, server session lost/refused, invitation acceptance) | ~ once per device per server, then only after a lost server session |
| `GET /v1/servers/{id}/routes` | account bearer | every remembered route refused, or 3 s silence (existing) | rare |
| Account screens (security, sessions, notifications, export, deletion) | account bearer | person opens them | user-paced |
| `GET /v1/keys` | none | a signature fails to verify (key rotation) | rare; cacheable |

Removed client calls: `POST /v1/servers/{id}/attachments`, `GET /v1/servers/{id}/profiles`, Hosted
invitations/memberships (`/v1/invitations*`, `/v1/servers/{id}/invitations|memberships|membership|members/*|leave|ownership`),
Hosted profiles (`/v1/profiles*`, `/v1/account/profiles*`) for server access. Owner transfer and leave move to the server
(existing `TransferDirectOwnership`; leave = delete own membership, phase 2 route).

### Client → user server (new or changed)

| Endpoint | Body | Result |
|---|---|---|
| `POST /v1/direct/sign-in` | `{"porticoIdentity": <assertion envelope>}` (alternative to `{username,password}`) | identical to a direct sign-in (`DirectSignIn`: account session, profile list, device-approval state). Not a member → **403 `access_refused`**. |
| `POST /v1/auth/refresh` | unchanged | a session migration 0120 ended → **401 `session_migrated`** (re-admit silently; not a sign-out). |
| `POST /v1/access/invitations/accept` | `{"code", "porticoIdentity"}` (alternative to `{code, username, password, name}`) | creates the linked account, then the same sign-in answer as above. |
| `POST /v1/hosted/attach`, `POST /v1/hosted/profiles/offline-select`, `POST /v1/hosted/restrictions` | — | **removed** (410 `moved`). |

### Server → Hosted

| Endpoint | Auth | Trigger | Frequency |
|---|---|---|---|
| **`POST /v1/servers/{id}/members`** (new: delta or full) | installed server credential | a linked member was added, removed, disabled or changed role; Hosted answered "gap"; first run after upgrade (full) | on change only (≈ membership edits) |
| `POST /v1/servers/{id}/heartbeat` (check-in) | server credential | weekly if wakeable, daily if not, or on wake | ≤ 1 / server / day |
| `POST /v1/server-claims/cleanup` | server-signed proof | check-in says `cleanupPending`, or claim retired | on account deletion only |
| `POST /v1/servers/{id}/events` | server credential | rename, storage nearly full, shutdown | rare |
| routes / direct-route / certificates / endpoint | server credential | address change, weekly re-proof, certificate renewal | unchanged (doc 47) |

Removed server calls: `GET /v1/servers/{id}/policy`, `POST …/policy/ack`, `POST …/attachments/redeem`,
`POST …/restrictions`. The 30-day policy renewal is gone; the check-in carries no policy revision.
Check-in body `{wakeRevision, memberSequence, memberDigest}` → `{heartbeatAfterSeconds, presenceExpiresAt,
routeLabel, memberSequence, memberDigestMatches, cleanupPending}`.

### Hosted → server

`POST {server}/v1/hosted/wake` (signed, content-free apart from `kind`). Sent only for: account deletion
affecting a member (erasure job queued), members-only label rotated after a removal. Never for membership
additions (the server is the source of those).

## 4. Wire shapes

**Identity assertion** (Hosted `POST /v1/servers/{id}/identity`, body `{}`; response `trust.Envelope`,
purpose "documents", signed by the current Hosted signing key; the server verifies with its pinned root):

```json
{"kind":"portico.identity.v1","accountId":"acc_…","username":"justin","displayName":"Justin",
 "serverId":"srv_…","nonce":"<22+ b64url>","issuedAt":"…Z","expiresAt":"…Z (≤ issuedAt+5m)"}
```
Server checks: signature chain, kind, `serverId` = self, `issuedAt` ≤ now+1 min, expiry in the future,
lifetime ≤ 10 min, nonce not seen (table `hosted_identity_nonces`, pruned at expiry). Hosted does **not**
check membership — it asserts identity to one audience; the server decides.

**Membership push** (server → Hosted, `POST /v1/servers/{id}/members`, installed server credential):
```json
{"after":41,"through":44,"full":false,
 "members":[{"accountId":"acc_…","role":"member","active":true},{"accountId":"acc_…","role":"","active":false}]}
```
The journal only names accounts; a push carries each named account's *current* standing, so repeated
changes coalesce. `full:true` (`after` 0) lists every active member. Response `{"sequence","applied"}`.
- Hosted applies a delta only when `after` = its stored sequence; an exact replay (`through` = stored) is
  answered applied; anything else is answered **200 `applied:false`** with the stored sequence, and the
  server's next push is the full list. A full push replaces the server's index rows in one transaction.
- The server journals in the writer's own transaction (SQLite triggers on `account_portico_links` and on
  `direct_memberships` role/disabled for linked accounts; deleting an account cascades to its link).
- Hosted bumps the list version of every account whose row appeared, disappeared or changed role; a
  removal queues the members-only label rotation, which wakes the server when done.
- Every check-in carries the server's acked sequence and a digest of its full active list ("accountId:role"
  lines, sorted, SHA-256). If Hosted's index disagrees in either, the server pushes its full list. This is
  the "never behind" guarantee (including after a restore on either side) without heartbeats.

**Server list** (`GET /v1/servers`): existing `{items:[…]}` plus top-level `"version":"<n>"` and
`"watch":"<token>"`, and header `ETag: "<n>"` (`If-None-Match` → 304). The version is read before the list.

**Version check** (`GET /v1/server-list/{watch}/{version}`, no credential): `304` if `{version}` is
current, else `200 {"version":"<m>"}`; bad token → 404; map not loaded yet → 503 + Retry-After. The held
version is in the path so a browser sends a simple request (no CORS preflight). Never touches PostgreSQL.

## 5. Hosted: version map

- `accounts.server_list_version bigint NOT NULL DEFAULT 0`, set from one global sequence
  (`server_list_versions`) whenever an account's server set changes (membership push, retirement,
  deletion), taken under one advisory lock so values commit in order.
- In memory: `map[uint64]int64` keyed by a per-process seeded hash of the account id, value = max
  version of the accounts in that slot. Loaded at start by paging `WHERE server_list_version > last ORDER
  BY server_list_version`; afterwards the same read runs on NOTIFY `portico_server_list` (a statement
  trigger on the column), on a push this process handled, and on a 30 s safety tick (one indexed read
  that finds nothing). Measured: 1 M accounts ≈ 38 MB; a check ≈ 240 ns of CPU (M4 Pro).
- ETag = slot value. Because every bump takes a fresh value from one global sequence, an account that
  changed after a device received ETag *e* always has slot > *e* — a collision can cause an extra
  refetch but never a missed change (also across restarts and reseeding).
- Watch token = `<b64url(accountId)>.<b64url(HMAC-SHA256(k, "portico.server-list.v1\0"+accountId)[:16])>`,
  `k` derived from Hosted's seal key (rotating it = one extra full fetch per device).

## 6. Capacity budget — 1 M users, ~2 M devices, ~300 k servers, one VPS

| Traffic | Rate (avg / peak) | Cost per request | DB work |
|---|---|---|---|
| Version checks | ≤ 8 M/day worst case (2 M devices × 4); realistic 3–5 M → 60–95 /s avg, ~300 /s peak | TLS + HMAC + map read (µs) | none |
| Full list fetch | on change + sign-in; ≤ 0.3 M/day → ~4 /s | auth + 1 list query + route signing | ~3 indexed reads |
| Identity assertions | ~0.1 / device / day → 0.2 M/day → ~3 /s | auth + Ed25519 sign (~50 µs) | 1 read |
| Hosted refresh / keep-alive | ~70 k/day → < 1 /s | | 1 write |
| Membership pushes | ~0.05 / server / day → 15 k/day | | 1 txn, O(changes) |
| Check-ins | ≤ 300 k/day (daily worst) → 3.5 /s | | 1 txn |
| Erasure / wake | account deletions only | | |
| DNS feed pull | 2–3 nodes / 5 s | | 1 read |
| Certificates | 300 k servers / ~60 days → ~5 k/day ≈ 3.5 /min (quota 25/min, budget 20/min) | ACME | |
| **Total** | **~100 /s avg, ~350 /s peak** | | **< 20 DB txn/s** |

Memory: version map ≈ 38 MB per million accounts (measured, `BenchmarkServerListCheck`); the rest of
Hosted is stateless per request. Rows: `accounts` 1 M; `server_members` ≈ 1.2 M (300 k servers × 4) at
~120 B + 2 indexes ≈ 300 MB; `servers` 300 k. PostgreSQL shared buffers of 2–4 GB hold the hot set.
A 4-core/16 GB VPS has an order of magnitude of headroom; TLS handshakes dominate CPU.

## 7. Failure and backoff

- Every client call to Hosted goes through the existing per-origin `HostedGate` (exponential from 5 s,
  ×2 per failure, cap 2^8, plus 0–50 % jitter, `Retry-After` as a floor); automatic calls (version
  check, keep-alive, routes) wait while the gate is open; a person's tap always goes through.
- The version check never runs in the background; it is skipped while the gate is open and rescheduled
  for the next foreground after the gate's deadline.
- Servers: every Hosted call uses `networking.RetryAt` (exponential, capped, +½ jitter, `Retry-After`
  floor), deadlines persisted; the membership outbox is a journal, so a push that fails is retried with the
  same backoff and coalesces everything queued meanwhile.
- **Hosted down:** existing server sessions, profile switching, playback, invitations (code/link) and
  member management keep working; new first-time connections of a Portico Account device to a server need
  an identity assertion and wait; version checks fail quietly; nothing is signed out.
- **Server down:** clients keep their list; Hosted is not consulted to learn "is it up".

## 8. Invitation flow

1. Owner/admin creates an invitation on the server (`POST /v1/admin/access/invitations`, existing; role +
   libraries; email optional). The server returns a one-time code and a link
   `https://web.getportico.tv/join#server=<id>&code=<code>` (app link on Apple). The owner shares it.
2. No email is sent. The owner shares the server-created link.
3. Invitee opens the link. If not signed in to a Portico Account, the app signs in (Hosted). The app asks
   Hosted for an identity assertion for that server and calls the server's accept with
   `{code, porticoIdentity}`. The server checks the code, creates the linked account + primary profile,
   applies role/libraries, journals the change, and answers with a sign-in result — the invitee is in.
4. The server's control loop pushes the delta; Hosted upserts `server_members`, bumps the invitee's
   list version. The invitee's other devices learn about the server at their next version check.
   The accepting device adds the server immediately (it already has the answer).
5. A direct (non-Portico) invitee still accepts with username/password; nothing reaches Hosted.

## 9. Removal, revocation, deletion

- **Owner removes / disables a member** (server): existing direct paths revoke the member's families and
  stop playback at once → the member's clients get 401 on refresh / 403 `access_refused` on sign-in and
  go to sign-in / drop the server. Journal → push → Hosted drops the index row, bumps the version, queues
  a members-only label rotation (A20/A86), which wakes the server when done.
- **Member leaves** (phase 2 route on the server, same effect).
- **Portico Account deleted** (Hosted): `account_erasure_jobs` for every server in the index where the
  account is a member (and owned servers are retired as today); Hosted wakes those servers only. The
  server's check-in answer says `cleanupPending`; the server runs the existing signed cleanup exchange and
  `applyCleanupJob` now also deletes the linked local account and erases its profiles' saved data.
- **Ban/suspend:** no such feature exists; if added it uses the same job channel (kind
  `account_suspended`), never a heartbeat.
- **Hosted sign-out everywhere / password change:** affects Hosted sessions only. Server sessions are the
  server's (Justin: "users are virtually never signed out"); a person revokes them in the server's session
  list.

## 10. Migration from the signed-policy model (pre-release): a clean cut

Decision, 23 Sep (no production data, no users): no conversion and no
compatibility window. Hosted, servers and clients deploy together (DEPLOY-NEXT §8).

Server migration **0120** (`0120_server_membership.sql`), plain SQL, written once:
- Creates `account_portico_links(account_id → accounts, hosted_account_id unique, consent, unindexed)`,
  `hosted_membership_journal`, `hosted_membership_sync` (… `custodian`), `hosted_identity_nonces`,
  `hosted_identity_challenges`, `portico_admitted_families`, `portico_factor_challenges`,
  `hosted_migrated_families(family_id, refresh_hash)`, and the journal triggers.
- Ends every `authority='hosted'` session, remembering each refresh credential hash, so a refresh answers
  **401 `session_migrated`** and the client re-admits silently: the claimant as the owner, anyone else
  `access_refused` until invited again.
- Deletes the cached `policy`, `restrictions`, `restriction_outbox` and `hosted_profile_revisions` (the
  tables stay empty until a drop migration).
- Converts nothing. Portico Account members sign in and join again by invitation; their Hosted-era
  personal data is not carried over. The claimant is linked to the local owner at the first control pass
  from the installed claim (`linkClaimOwner`).

Hosted migration **0017** (`migration_0017.go`):
- Creates `server_members`, `server_member_sync`, `server_member_full_prior`, `server_member_departures`,
  `accounts.server_list_version` + index, sequence `server_list_versions`, `servers_owner_account`,
  `servers.custody_released`, the label-rotation trigger on index deletes and the version NOTIFY trigger.
- The index starts **empty** (no seed from `memberships`); accounts that had memberships get a new list
  version. Cancels pending Hosted invitations. `p10_account_epoch_policy` no longer wakes servers.
- Leaves the legacy tables in place, unused, for a later drop migration.

## 11. What is removed

- Server: policy fetch/apply/ack, 30-day renewal, `desired_policy_revision` wake semantics, ticket
  redemption (`/v1/hosted/attach`), offline profile proofs, restriction outbox sync, the policy-based
  `allowed` path for `authority='hosted'` (now always unauthorized), policy-driven family reconciliation.
- Hosted routes: `GET /v1/servers/{id}/policy`, `POST …/policy/ack`, `POST …/attachments`,
  `POST …/attachments/redeem`, `POST …/restrictions`, `POST|GET …/memberships`, `GET|POST …/invitations`,
  `GET …/profiles`, `GET /v1/servers/{server}/membership`, `PUT …/members/{account}`, `POST …/leave`,
  `POST …/ownership`, and all `/v1/invitations*`. Their store code and tables stay until the drop migration
  (phase 3). `DELETE /v1/servers/{server}` (retire a claim) stays.
- Clients: ticket attach, offline profile proof, Hosted profile chooser for servers, Hosted invitation and
  membership screens (move to the server's access screens, which already exist for direct members).

## 12. Phasing

1. (this branch) Server + Hosted: identity assertion, Portico sign-in, linked accounts, journal + push,
   discovery index, version map + watch, check-in without policy, deletion via index, migrations.
2. (this branch, after the API settles) client-core: version-checked list, Refresh Servers, identity
   sign-in, invitation acceptance by identity; with UI on web and Apple.
3. Reconcile residual mail requirements, leave flow and retired routes/tables; no email delivery is added.

## 13. As built (23 Sep, be/member)

Commits `0abd2282` (server), `dcf8ccc4` + `bc9325e5` (Hosted), `77dcf68a` (benchmark, load tool),
`4d8b397a` (client-core). Differences from the first draft of this spec, all folded in above:
sign-in is `POST /v1/direct/sign-in` (the DirectSignIn path) rather than `/v1/sessions`; the link is a
table, not an `accounts` column; a push gap is `200 applied:false`, not 409; there is no epoch (sequence +
check-in digest cover restores); the held version is a path segment; the push is authenticated by the
server's installed credential, not an identity-key signature.

Decisions taken while building:
- A recovery owner that is also a Portico Account may use its sessions from anywhere; the recovery
  *password* still signs in only from the private network (`recoveryPasswordAllowedTx`).
- No one can set a password on a Portico member (`UpdateDirectMember` refuses).
- A deleted Portico Account's member account is erased on the server (profiles' saved data, sessions,
  account); an owner is only unlinked (it is the recovery account).
- Hosted account profiles (`/v1/profiles`, `/v1/account/profiles*`) are left in place: nothing on the
  server uses them any more.

Refinements (23 Sep):
- **Clean cut** (above); the conversion, its migration hook and the PIN-reset state it produced
  (`profile_pin_reset_required`, `pinResetRequired`) are gone. **No compatibility window:** Hosted has no
  ticket routes or legacy check-in answer; the server answers `/v1/hosted/attach` with `410 moved`.
- **Custody (M6, M10).** Hosted issues a `portico.custody.v1` assertion on
  `POST /v1/servers/{id}/identity {challenge, purpose:"custody"}`. Custody of a claim moves to a new owner
  only when the push carries that owner's own custody assertion (≤ 24 h old). When the server's list no
  longer names the custodian as owner and custody cannot follow, `servers.custody_released` is set: the
  former custodian no longer sees the server as its own and may leave; retiring waits for an owner who
  accepts custody. Push answers carry `custodian` ("" while released); the server keeps it and the owner's
  snapshot says `custodyPending`; the owner accepts with `POST /v1/direct/ownership/custody
  {porticoIdentity}` (client-core `acceptPorticoCustody`). A consent lists an account for the first time
  only if ≤ 30 days old.
- **Signed challenge (M7).** `POST /v1/direct/portico-challenge {nonce}` answers
  `{challenge, expiresAt, proof:{payload, signature}}`, a `portico.challenge-proof` signed by the server's
  identity key over the challenge, the client nonce and the installation. client-core verifies it against
  the server id (the key's digest) before asking Hosted. A transparent relay on plain HTTP can still
  forward bytes; only TLS or the LAN protects the channel itself.
- **Push size (M14).** Deltas and full pages stop at 200 accounts or 640 KiB of members; after 4 failed
  pushes a delta falls back to the paged full list. The control transport's 16 KiB request cap no longer
  applies to the push (1 MiB, Hosted's limit) — before this, any push of more than about a dozen members
  with consents never left the server.
- **Locks (N5).** A server operation locks the custodian's and the custody candidate's account rows in id
  order, in one statement, first.
- **Roles and profiles (N3, N4).** A Portico account invited as admin is a local admin and is indexed as
  admin. Its primary profile is the one new profile named after its display name (Hosted keeps no server
  profiles).
