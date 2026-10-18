# Lane report: api-platform

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-api-platform`, branch `audit-implementation/api-platform` (nothing pushed). Status: complete for the lane brief, merged with `audit-implementation/main` (networking, naming, music-audiobooks), with the gaps listed in section 9.

What was run: `go build ./...` (also `GOOS=windows` and `GOOS=darwin` for the touched packages), `go vet` on touched packages, and the targeted tests named in section 8. No full suite was run, as instructed.

## 1. Outcome

- **Scopes.** Every route on the typed registry now has to declare the scope an API key or token needs (`apikit.Metadata.Requires`); a route without one cannot be registered. The ~500 hand-registered routes are classified in one table (`server/internal/httpapi/route_scopes.go`), checked by a test for completeness. A route nobody has classified is closed to keys and tokens at run time.
- **Owner API keys work.** Create (secret shown once), name, scopes, optional expiry, last used, revoke. Only a digest of the secret is stored. Used as `Authorization: Bearer pk.…` on any route.
- **Personal access tokens** for any member (`pat.…`), limited to that member's own role, libraries and restrictions and to the scopes chosen. The owner can list and end any member's tokens.
- **A key or token can never be used as an unscoped session.** It is resolved and scope-checked at the edge before anything else runs, and the identity service refuses a key-shaped bearer that did not come through that check.
- **Third-party sign-in.** Both ways already worked for any app without registration; they are now documented for outside developers (address + user name + password; and the "enter this code" flow). Both issue ordinary member sessions.
- **Protection from bad clients.** Each key or token has its own request rate (600 a minute by default, owner-adjustable), its own cap on requests in flight (4, plus 2 long-lived), and all keys together are capped below the lanes people use. Answers carry `RateLimit` headers and `429` with `Retry-After`.
- **A bounded title query**, `GET /v1/titles`: one library and one kind, sorted by title, date added or year, range only on the sort key, or a lookup by TMDB/TVDB/IMDb/AniList id. The caller's libraries and restrictions are in the SQL. Its cursor is not tied to the library revision, so a tool can walk a large library to the end during a scan.
- **"These paths changed"**, `POST /v1/libraries/paths-changed`: maps each path to its library folder and runs a scan limited to those folders. No targeted scan existed, so the narrowest one was added. Reports during a running scan wait and run afterwards; a deleted folder is handled.
- **Outbound webhooks**, sent by the local server only: URL, events, secret; HMAC-SHA256 signature over timestamp and body; event id for de-duplication; retries with backoff from a persisted queue; delivery log; "send test". All 17 events in the brief are produced.
- **A built-in MCP server** at `POST /v1/mcp` (Streamable HTTP, JSON-RPC 2.0, `initialize`, `tools/list`, `tools/call`), no new dependency, 40 tools, authenticated by key or token, tools filtered by scope and by the member's role.
- **Documentation** for outsiders: four new guide chapters and corrections to two existing ones (section 7 lists them).
- **Clients:** `packages/client-core/src/api-access.ts` (services and parsers for keys, tokens, webhooks, title query, paths-changed), words in the string catalogue, and the existing web "API keys" panel moved onto the new routes so it works.

## 2. Decisions I made

1. **A key or token acts as the member who made it and has a lifetime of its own.** It is stored in the session-family tables, so every live check a session gets (account, profile, role, device approval, libraries, restrictions) applies with no second code path, but nothing that ends a member's sessions ends it. Four schema triggers (`api_credential_*`) enforce that, so it holds for every present and future sign-out path without that code knowing keys exist.
2. **What ends a key or token (changed on the owner's instruction):** revocation, expiry, its member being disabled or deleted, or its profile being deleted. **A password change, "sign out everywhere", signing out a device, and an owner's edit of the member's access do not end it**, so changing the owner password no longer breaks Sonarr-style integrations. Disabling a member ends their keys for good (re-enabling does not revive them).
2a. **A key holds the role its member holds now.** Demotion narrows it at once (admin calls answer 403; it keeps working as a member's); promotion widens it, never beyond its scopes. I first made it "never raised", but the server's tier checks already take the live role as the authority, so anything else produced a wrong 401.
2b. **Restore from backup: keys stay valid.** They are rows in the database, so a restore brings back the keys that existed at backup time and they work. The cost, stated in the guide: a key revoked after that backup is valid again after restoring the older database, so the owner should review the lists after a restore. I judged this acceptable because the same is true of everything else a restore brings back (a removed member returns too) and the alternative breaks every integration on every restore. If the accounts lane's restore rule should also end keys, it only has to set `api_credentials.revoked_ms`.
2c. **Keys and tokens are no longer listed among the member's devices** once approved, so "sign out" and "forget" on the device list cannot hit them by accident. They are listed and revoked only in their own lists.
2d. **Keys do not use the console's reserved connections** (networking lane's 32-connection reserve): a request carrying a key takes an ordinary connection whatever path it asks for.
3. **Four scopes, not nested:** `read`, `playback`, `library`, `admin`; `admin` is everything. A download manager can hold `library` alone and cannot read the catalogue. `playback` also covers the member's own watch state, lists, preferences and recordings.
4. **Session-only operations.** Signing in/out, passwords, two-factor, profile switch, device approval and linking, TV-code approval, server claim, and creating/listing/revoking keys and tokens cannot be done with a key or token whatever its scopes. A key can never mint or widen a credential. All `/v1/direct/*` routes are session-only; member listing for keys goes through `/v1/admin/access/members`.
5. **"What is playing" and "stop a session" need `playback`, not `admin`** (plus the administrator role), so a home-automation key does not need full administration.
6. **Device approval applies to tokens.** When the owner approves new devices by hand, a member's new token waits for approval like a new device (it appears in the device list). The owner's own keys are not held.
7. (Superseded by 2c.)
8. **Limits:** 100 keys per server, 25 tokens per member, expiry at most 10 years ahead, 20 webhooks, 1,000 queued deliveries per webhook, 200 log entries per webhook.
9. **Paths-changed is owner-only** (like the existing scan routes) and needs the `library` scope. Paths must be as the server sees them; no path-mapping option was added.
10. **Removal detection in a limited scan** follows the existing rule (a file must be seen missing twice and pass the grace period). I did not change that; the identity lane owns the title/file model.
11. **Webhook events come from database triggers** for everything that is a table change (titles, scans, playback, recordings, sign-ins, low storage), so other lanes' Go refactors cannot silently drop an event. Backup, update and server-start events are emitted from Go.
12. **`title.added` is sent about 45 seconds after the title appears**, with the title it has by then, and an update before that is folded into it. Seasons, songs, artists and extras are not "titles" for webhooks.
13. **A member's private recordings library is never reported** in title events. Recording and playback events do name the programme and the member (the owner already sees both in the console).
14. **Webhook URLs may be private addresses; redirects are never followed; URLs with a user name or password are refused.** TLS certificates are verified.
15. **A webhook that keeps failing is not switched off automatically**; it shows a failing count and eventually drops new events once 1,000 are waiting.
16. **MCP accepts only a key or token**, not an app session, so what an assistant can do is always something chosen. Each tool is one or two calls to the server's own API, made in-process with the caller's credential, so a tool cannot do what the API would refuse.
17. **MCP `change_settings` reads the current settings, changes only the named ones and writes the whole document back**, because the settings route replaces the document.
18. **Public `GET /v1/system` no longer returns the build id, source digest or build time** (B-08.16/B-15.15). It still returns the version, which apps show before sign-in.
19. **Long-poll maximum is advertised as 25 seconds**, which is what the server accepts (B-08.5).
20. **A reconnecting event stream takes over from the old one** instead of being refused (B-08.3). `event_stream_exists` is no longer produced.

## 3. Bugs

| Bug | Result |
|---|---|
| B-01.3 API keys never authenticate | **Fixed** (whole feature). |
| B-08.1 / B-15.8 web app refused behind TLS proxy | **Fixed by the networking lane** (now merged). |
| B-08.2 unknown errors become `400`, unlogged | Not fixed: ~590 legacy call sites; belongs with the registry migration (wave 3). New routes here are on the registry, which defaults to a logged 500. |
| B-08.3 reconnect demoted to long-poll | **Fixed** server side: the newcomer takes over. Client-core still treats a 409 as permanent, but the server no longer sends one. |
| B-08.4 new stream has no cursor | **Fixed**: a stream that opens with nothing waiting sends `stream.opened` carrying its position. |
| B-08.5 advertised 55 s, accepted 25 s | **Fixed**: both say 25 (server capability and client-core clamp). |
| B-08.6 every event wakes every client | Not fixed: freshness-server lane. |
| B-08.7 / B-15.3 limits shared behind a proxy | **Fixed by the networking lane** (now merged). The per-credential limits are keyed by credential, not address. |
| B-08.8 grid ETags | Not fixed: freshness-server lane. |
| B-08.9 registry refuses a response containing null | Not changed, deliberately (V01: intentional). Every new route initialises its lists and is exercised by a test. |
| B-08.10 | Already fixed. |
| B-08.11 unused toolkit pieces described in docs | Partly: the `AuthenticateKey` part is gone; the cursor/idempotency/precondition drift is for the documents lane. |
| B-08.12 ETag/encoding nits | Partly fixed by the networking lane (gzip `q=0`, weak validators on compressed bodies). Still open: the 304 is produced after composing the answer; only `assets/index-*` is cached as immutable. |
| B-08.13 registry access label not enforced | **Fixed**: a `viewer_item` route must name `{itemId}` and a `viewer_library` route `{libraryId}` or it cannot be registered, and the authorizer now checks the library. |
| B-08.14 full-library walk may never finish | **Addressed for integrators**: `GET /v1/titles` pages without a revision fence. First-party cursors unchanged. |
| B-08.15 | Already fixed. |
| B-08.16 / B-15.15 public build fingerprint | **Fixed** (see decision 18). |
| B-08.17 one realtime pool | Not fixed: limits lane. Keys are capped at 2 long-lived requests each, 64 in total. |
| B-08.18 slow uploads cut at 15 s | **Fixed by the networking lane** for the large JSON upload routes. |
| B-15.23 TV-code and invitation routes outside the cheap limiter | **Fixed** for starting, looking up and deciding a code and for invitation routes. The TV's own poll is left out on purpose (several TVs behind one address poll legitimately). |
| B-15.7 tokens not device-key bound though docs say so | Not fixed: accounts-security lane / documents. |

## 4. API changes

**Removed**
- `GET /v1/admin/access/api-keys`, `POST /v1/admin/access/api-keys`, `POST /v1/admin/access/api-keys/{id}/revoke` (and their entries in `admin-access.openapi.yaml`).
- Error `api_keys_disabled`; `event_stream_exists` is no longer returned.

**Added (all on the typed registry; contracts regenerated in `server/api/portico.openapi.json` and `packages/contracts`)**

| Route | Who | Scope | Notes |
|---|---|---|---|
| `GET /v1/me/api-access` | member | session | scopes, maxima, whether tokens are allowed, rate |
| `GET /v1/admin/api-keys`, `POST /v1/admin/api-keys`, `DELETE /v1/admin/api-keys/{id}` | owner | session | body `{operationId, name, scopes[], expiresAt?}`; answer `{credential, secret?}` |
| `GET /v1/me/access-tokens`, `POST /v1/me/access-tokens`, `DELETE /v1/me/access-tokens/{id}` | member | session | same shapes |
| `GET /v1/admin/access-tokens`, `DELETE /v1/admin/access-tokens/{id}` | owner | session | every member's tokens; `?accountId=` |
| `GET /v1/titles` | member | read | bounded title query |
| `POST /v1/libraries/paths-changed` | owner | library | `{paths:[{path, change?}]}` → `202 {paths[], jobs[]}` |
| `GET/POST /v1/admin/webhooks`, `PATCH/DELETE /v1/admin/webhooks/{id}`, `GET /v1/admin/webhooks/{id}/deliveries`, `POST /v1/admin/webhooks/{id}/test` | owner | admin | |
| `POST /v1/mcp` (and `GET /v1/mcp` → 405) | key or token | open | JSON-RPC |

**Changed**
- Every operation in `portico.openapi.json` has `x-scope`.
- `apikit.Metadata`: new mandatory `Requires`; the old `Scope` field (admin area) is renamed `AdminScope`.
- `GET /v1/system`: `buildId`, `sourceDigest`, `builtAt` removed.
- `GET /v1/capabilities`: `longPollSecondsMax` 55 → 25.
- `GET /v1/events` (stream): sends `stream.opened` first when nothing is waiting; a second stream for the same installation replaces the first.
- New error codes: `insufficient_scope`, `session_required`, `credential_busy`, `access_tokens_off`, `credential_limit`, `credential_unavailable`, `webhook_limit`.
- Requests with a key or token carry `RateLimit` / `RateLimit-Policy` response headers.
- New admission lane `tools` (16 slots, 30 s) for MCP.

## 5. Schema changes (`0001_baseline.sql`, synced with `scripts/schema-sync.py`)

- Removed table `access_api_keys` and its index.
- Added `api_credentials` (with `ended_reason`), `api_credential_use`.
- Added four triggers that give credentials their own lifetime: `api_credential_family_guard` (a live credential's family does not stay revoked), `api_credential_epoch` (it follows the account's epoch), `api_credential_role` (it holds the member's current role), `api_credential_member_disabled` (disabling the member ends it).
- Added `inventory_path_notices`, `inventory_run_targets`.
- Added `webhooks`, `webhook_deliveries`, `webhook_state`, `webhook_events` (a pass-through table that is always empty at rest).
- Added 15 triggers named `webhook_*` on `catalog_entities`, `metadata_publication_heads`, `jobs`, `playback_v1_sessions`, `dvr_recordings`, `identity_device_families`, `notification_records` and `webhook_events`. Each does nothing unless an enabled webhook subscribes to its event. Every one is run by `internal/webhooks/triggers_test.go`.
- `dbwork` authority table list: `api_keys`/`access_api_keys` replaced by `api_credentials`.

All of this is one block at the end of the baseline file.

## 6. Owner or viewer settings added or wired

| Name | Type | Default | Takes effect | Screen |
|---|---|---|---|---|
| `apiRequestsPerMinute` | integer 0–100000 | 0 = 600 | Rate for one key or token, at once | None yet (it is in the settings registry, so it shows wherever the registry is rendered) |
| `blockMemberAccessTokens` | boolean | false | Stops members (not owner or administrators) creating tokens; existing ones keep working | None yet |

Both are phrased so that a settings document saved without them reads as the default.

## 7. Needed from others

**Console (web) screens**
- API keys: the existing panel works but offers four preset choices and no expiry. Wanted: scope checkboxes, expiry, state and "ended because…" text, last used.
- Personal access tokens (member settings), and the owner's list of members' tokens with revoke.
- Webhooks: list, add/edit, secret shown once, rotate, send test, delivery log, failing/dropped counts.
- MCP: a page showing the address (`<server>/v1/mcp`) and pointing to key creation.
- The two settings above.
- Services and parsers are in `packages/client-core/src/api-access.ts`; words are in `packages/i18n/src/catalog/en-US/web.ts` under `api.*` (scopes, states, webhook events, delivery failure reasons).

**Native apps:** personal access tokens screen if wanted there; nothing else.

**Other server lanes**
- providers-matching / scan-loop: there is still no library-wide list of unmatched or low-confidence titles. The MCP tools offered are `list_unmatched_files` (TV files that could not be placed) and `get_match` per title. When a needs-attention route exists, add one tool for it in `internal/httpapi/mcp.go`.
- scan-loop: the limited scan lives in `internal/ingestion/targeted.go` with three small hooks (`incremental.go`, `inventory.go`, `service.go`). If the scan pipeline is reworked, keep `ReportPaths` and the rule that a limited run judges only objects under its targets.
- identity lane: if the catalogue's title table changes, the `webhook_title_*` triggers reference `catalog_entities(public_id, kind, title, year, library_id, parent_id, retired)`.
- playback lanes: `webhook_playback_*` triggers reference `playback_v1_sessions(state, ended_ms, end_reason, device_id, item_id, channel_id, position_ms, account_id, profile_id, kind)`. Legacy (non-v1) playback sessions produce no webhook events.
- networking lane: browser-based third-party tools on another origin are still refused by the origin check, including the preflight. If such tools should work with a key, the origin rule needs to allow them.
- accounts-security lane: `identity/credential_security.go` used to revoke rows in the old `access_api_keys` table on every password change; that statement is removed (the table is gone and keys now survive a password change). If that lane rewrites session revocation, the `api_credential_*` triggers keep keys alive without any change on its side; to end a member's keys deliberately, set `api_credentials.revoked_ms`. Decide whether a restore should end keys (decision 2b). Dead references to an `api-key` authority remain in `identity/restrictions.go` (harmless).
- registry lane (wave 3): each entry in `route_scopes.go` moves to `Requires` with its route; the test `TestEveryRouteDeclaresACredentialScope` checks the two agree.
- hosted lane: "Sign in with Portico" for third-party apps is not here.

**Documents to correct**
- `Spec — API v1 Style Guide` amendment 11 (API keys deliberately disabled) is superseded.
- `server/api/openapi.yaml` still lists the removed `/v1/system` fields and the `event_stream_exists` answer.
- `server/api/guide/check-examples.mjs` reports 16 failures, all older `/v2/playback` mentions in chapter 4; none are from this lane.

## 8. Test notes for other environments

Tests written and run here (all pass):
- `apikit`: `go test .`
- `server/internal/httpapi`: `-run 'TestPasswordChange|TestDisablingOrDeleting|TestACredentialHolds|TestAPIKey|TestCredential|TestEveryRouteDeclares|TestScopesDoNot|TestTitleQuery|TestPathsChanged|TestWebhooksAre|TestMCP|TestEvents|TestOpenAPICovers|TestEveryRegisteredRouteHasALane'`
- `server/internal/access`, `server/internal/webhooks` (whole packages), `server/internal/eventfeed`
- `server/internal/ingestion`: `-run 'TestReportedPaths|TestIncrementalScanSkips'`
- `server/internal/identity`: `-run 'Famil|Authent|Logout|Renew|Refresh|Device'`
- `server/internal/persistence`: `-run 'Schema|KnownSchema'`; `server/cmd/server`: `-run TestMainWiresEveryHTTPDependency`
- `packages/client-core`: `node --experimental-strip-types --test test/api-access.test.ts test/server-administration.test.ts test/events-retry.test.ts`; `web`: `npx tsc --noEmit`

One unrelated test fails in this environment before and after: `TestListeningHTTPPreferenceEnvelopeAndCAS` ("explicit reviewed helper binary required").

Not exercised here; to check on a running server:
1. **Key end to end.** Sign in as owner, `POST /v1/admin/api-keys` with `{"operationId":"t-00000001","name":"t","scopes":["read"]}`. Pass: 201 with `secret` starting `pk.`. `curl -H "Authorization: Bearer <secret>" <server>/v1/libraries` → 200; `POST /v1/libraries/<id>/scans` → 403 `insufficient_scope`; `GET /v1/admin/api-keys` → 403 `session_required`. Revoke, then the first call → 401. Also: change the owner password and sign out everywhere, then confirm the key still answers 200; disable a member and confirm their token answers 401 and is listed as ended.
2. **Rate limit.** Set `apiRequestsPerMinute` to 30, make 31 quick requests with a key. Pass: the 31st is 429 with `Retry-After`; the owner's browser session is unaffected.
3. **Paths changed on a real library (Linux, then Windows and macOS).** Add a file to an existing film folder, `POST /v1/libraries/paths-changed` with that file's absolute path using a `library` key. Pass: `status:"queued"`, `folder` is the film folder, one job; the file appears; `inventory_directories` for that job lists only that folder. Repeat with a deleted season folder: the job completes (does not fail) and the episodes are counted missing once. On Windows use backslash paths as the server shows them. On a network share (remote source) confirm the same.
4. **Webhook delivery.** Add a webhook to a local listener (for example `nc -l` or a small HTTP server), subscribe to `playback.started`, `scan.finished`, `member.signed_in`; play something, run a scan, sign in on another device. Pass: one POST each with `Portico-Signature`; verify with the Python snippet in `server/api/guide/09-webhooks.md`. Stop the listener: the log shows `could_not_connect` and retries; restart it: delivered with the same `Portico-Event-Id`.
5. **Webhook events that need real conditions:** `recording.finished`/`recording.failed` (needs a tuner), `storage.low` (fill the state volume past the owner's warning level), `update.available` (set a release feed with a newer version; the check runs 2 minutes after start and then every 6 hours), `backup.succeeded` (run a backup), `server.started` (restart). The triggers themselves are tested against the real schema; these check the surrounding wiring.
6. **Playback triggers under real playback.** Start, pause, resume, change quality mid-play, stop. Pass: exactly started, paused, resumed, stopped; the quality change produces nothing.
7. **MCP with a real client.** Point Claude Code or another MCP client at `<server>/v1/mcp` with a key header. Pass: the tool list matches the key's scopes; `list_libraries` and `search` answer; with a `read` key `start_library_scan` is absent.
8. **Scan cost on a large library.** With a webhook subscribed to `title.added`, run a first import of a large library. Pass: scan time is not noticeably longer; the webhook shows `dropped` > 0 rather than an unbounded queue.

## 9. Not finished

- **No console screens** for tokens, webhooks, MCP or the two settings (by brief; listed in section 7). The existing API-keys panel was only adapted, not redesigned.
- **Unmatched / low-confidence titles tool:** only partial (per-title match state and unplaced TV files), because no library-wide route exists.
- **"Control a session" over MCP:** an administrator can stop any session; pause/resume/seek works only for the caller's own sessions, because the API has no route to control another member's playback.
- **Removal via paths-changed** relies on the existing missing-file rule (two observations and the grace period), so a removed file is not marked missing on the first report.
- **No path mapping** for tools that see different mount paths than the server.
- **Browser-origin third-party tools** remain blocked by the origin check (networking lane).
- **Legacy playback sessions** (non-v1) emit no webhook events.
- **Webhook `title.updated`** fires on title/year changes and on republished metadata of playable titles; a change to only a show's or album's other details (overview, artwork) does not fire it.
- **Still open after the merge:** B-08.2, B-08.6, B-08.8, B-08.17, and the remainder of B-08.12. (B-08.1, B-08.7 and B-08.18 were fixed by the networking lane.)
- **Client-core** still treats a 409 on the event stream as permanent; harmless now that the server does not send it, but worth removing with the freshness-clients work.
- **Not run:** full Go suite, full client-core suite, native type-checks, and anything on Windows/macOS beyond cross-compiling.
- `TestEveryRouteSurvivesHostileInput` fails on `audit-implementation/main` itself (its fixture's `POST /v1/setup` answers 401 since the networking lane's setup changes); it fails the same way after the merge and is not caused by this lane.
- A key revoked after a backup is valid again when that backup is restored (decision 2b).
