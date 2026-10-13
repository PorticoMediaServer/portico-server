# Portico API developer guide

Use the server's own origin, such as `http://127.0.0.1:32500` or
`https://media.example.com`. Hosted is not needed for local API integrations.
The [generated registry contract](../portico.openapi.json) describes typed
routes. This guide also covers hand-registered routes by reading their handlers;
the old YAML files and their example checker have not caught up with the router.
Examples below are illustrative requests, with IDs and secrets to replace.

1. [Conventions](01-overview.md): time, paging, errors, revisions and limits.
2. [Authentication](02-authentication.md): sessions, direct sign-in, refresh and TV codes.
3. [Browsing](03-browsing.md): libraries, content, search and title details.
4. [Playback](04-playback-v1.md): capabilities, sessions, queues and subtitles.
5. [Personal state and jobs](05-personal-state-and-jobs.md).
6. [Events](06-events.md): changes while a screen is open.
7. [Keys, tokens and scopes](07-keys-tokens-and-scopes.md).
8. [Tools](08-for-tools.md): title query and Sonarr/Radarr changed-path reports.
9. [Webhooks](09-webhooks.md): events, signatures and delivery.
10. [MCP](10-mcp.md): connect a desktop assistant and discover its tools.

For owner setup and media organisation, see [installation](../../../INSTALLING.md)
and [libraries](../../../docs/libraries.md). Topic references in the API folder
cover metadata editing, downloads, administration and social playback.

The checked-in generated contract also needs regeneration as source-registered
routes move forward. For example, current account-session routes are registered
in `internal/httpapi/foundation.go` but are missing from this generated snapshot.
Use their documented current source until that snapshot is refreshed.
