# Portico API style guide

October programme reconciliation. The current registry and generated `server/api/portico.openapi.json` own exact route schemas. Existing `/v1` paths are pre-release; no compatibility is preserved until release establishes v1. Remove retired routes and duplicate hand-written shapes rather than keeping alternate APIs.

## Contracts and authority

Use apikit for method/path, typed request/response, access rule, scope, lane, cost, limits and errors. Regenerate OpenAPI and shared contracts after changes and update non-UI consumers together. Registry migration is still in progress; generated-type availability does not prove all handlers/parsers already use it. Preserve exact identifiers, credentials, scope, role and bound validation while tolerating bad presentation rows at the smallest unit.

General catalogue/owner media times are integer milliseconds and instants RFC 3339 UTC. Specialised telemetry/subtitle/social/prepared shapes still need reconciliation. Use the generated field names, not a parallel example schema.

Paging is bounded and authorised in the query. Signed continuations carry their relevant query/order/owner evidence; stale publication can recover quietly. Do not treat every catalogue or position tick as a command conflict. API title-query cursors deliberately permit long traversal during scans. ETags/304 are supported, but generic composition cost and legacy error mapping remain follow-ups.

## Integration credentials

Owner API keys and personal member tokens are usable scoped bearers with digest-only storage. Scopes are read, playback, library and admin, in addition to current role/library/content checks. Session-only operations include authentication, linking/approval and credential management. Keys cannot mint credentials or borrow console reserve. Password change/ordinary sign-out do not revoke them; disable/delete/expiry/revocation do. Restoring a backup can restore a revoked key; review lists after restore.

Third parties may use direct sign-in, device codes or Hosted public-client Sign in with Portico. Hosted PKCE consent yields a server/challenge-bound identity assertion only. It does not give media authority or require app registration. Do not describe a complete general OAuth token service that the code does not implement.

Local webhooks use persistent queues, signed deliveries and bounded retry/logs. MCP uses the same API authority and explicit keys/tokens. No Hosted service, plugin or theme is involved.

## Freshness and transport

One authenticated `/v1/events` feed supplies scoped hints with long-poll fallback capped at 25 seconds. Clients retain screens, coalesce changes, reread quietly and delay transient errors. Browse/search answer with readable generations during rebuild; media session preparation is a distinct operation.

Plain HTTP accepts any address. Home networks alone define locality. Trusted proxies validate forwarded evidence; owner certificates and own-hostname TLS proxies work. Hosted-advertised candidates are TLS-only. No mandatory HTTPS, sub-path hosting or device possession key is promised. CORS/setup/CSRF requirements remain route-specific; never infer that every authentication endpoint permits any origin. WebAssembly subtitle delivery needs its qualified CSP/assets.

Sources: [API platform](../../../_program/reports/api-platform.md), [review](../../../_program/reports/review-api-platform.md), [registry](../../../_program/reports/registry.md), [freshness](../../../_program/reports/freshness-server.md), [network fixes](../../../_program/reports/review-networking.md), [search/sort](../../../_program/reports/search-sort.md). Integration/security matrix and live-client acceptance remain open; no all-route completion is claimed.
