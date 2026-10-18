# Lane: api-platform — API keys, personal tokens, third-party sign-in, webhooks, MCP

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-api-platform` (branch `audit-implementation/api-platform`). You can commit.
Audit documents: 08, 01 (B-01.3 and token entries), 15 (API surface), 23 and 24 for how others are used; verification V01, G2. Decision register D-06, D-28, D-50.

## What to deliver
1. **Scopes on the registry.** Every registered operation declares the scope it needs. Scopes are few and plain: read, control playback, manage library, administer (full). Member credentials are also bounded by that member's own permissions and restrictions. Add the field and the enforcement; a later lane moves the remaining hand-registered routes onto the registry.
2. **Owner API keys** that work: create (shown once), name, scopes, optional expiry, last used, revoke. Stored as digests. Authenticate with a bearer header. Finish what B-01.3 left half-built.
3. **Personal access tokens** for any member, limited to that member's permissions and to the scopes chosen.
4. **Third-party client sign-in** without registering an app: (a) the existing "enter this code" device flow, documented for outside apps; (b) direct sign-in to a server that does not use Hosted with server address, username and password. Both issue ordinary member sessions. ("Sign in with Portico" through Hosted is a later lane.)
5. **Protection from bad clients.** Per-credential admission and rate limits so one key or token cannot starve the server or the owner console. Bounded page sizes, request sizes and concurrency.
6. **A bounded item query** for third parties: filter, sort and page titles by indexed fields only, with the caller's restrictions applied in SQL as everywhere else. It is an addition; composed screens stay the default.
7. **"These paths changed"**: a call (manage-library scope) that tells Portico which paths were added, changed or removed, in the way Sonarr and Radarr notify Plex and Jellyfin, and triggers a scan limited to them. Use whatever targeted-scan entry exists (G1 says part of this exists); if none, add the narrowest one.
8. **Outbound webhooks**, owner-configured, sent by the local server only: URL, chosen events, a secret. Each delivery is signed (HMAC-SHA256 over timestamp and body), carries an event id for de-duplication, retries with backoff from a small persisted queue, and has a delivery log and a "send test" action. Events: title added, updated, removed; scan started and finished; playback started, paused, resumed, stopped; recording finished or failed; member signed in; backup succeeded or failed; storage low; update available; server started. The owner chooses the URL, so private addresses are allowed here; keep time, size and redirect bounds.
9. **An MCP server** built into Portico (no new dependency): the Streamable HTTP transport at one path, JSON-RPC 2.0, `initialize`, `tools/list`, `tools/call`; authenticated by API key or personal token, with tools filtered by scope. A curated tool set over the same services the API uses: list libraries, search, browse, title details, start a scan (library or paths), scan status, unmatched or low-confidence titles, fix a match, what is playing, stop or control a session, members, server health and storage, read and change settings, run a backup, activity and logs, guide and recordings, mark watched, manage webhooks. Tool descriptions in plain language.
10. **Documentation for outsiders**: update the existing API guide with authentication, scopes, limits, webhooks (payloads and signature check) and MCP setup.

## Boundaries
- The **accounts-security** lane owns sign-in policy, sessions, restrictions and backup contents. Do not rework those; add the new credential types beside them.
- The **networking** lane owns the local-or-remote test and client address parsing. Call whatever single function exists.
- Console screens for keys, tokens and webhooks are built by a client lane afterwards: add the routes, the `client-core` services and parsers, and list the screens needed.
- Secrets never appear in logs, events or webhook payloads.
