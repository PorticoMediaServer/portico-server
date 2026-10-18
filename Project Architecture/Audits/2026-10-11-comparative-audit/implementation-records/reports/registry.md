# Outcome

All 620 production API operations now have registry declarations: the 287 existing declarations plus 333 retained HTTP handlers with typed requests/responses. The production router rejects an undeclared or duplicate HTTP binding. Optional networking, guide and media families are included in the generated contract even when their services are not configured.

`server/api/portico.openapi.json` is the sole specification. It includes path/query parameters, scope/access/lane/cost/admission metadata, body bounds, reusable wire schemas, alternate statuses, uploads and media/stream content types. The old YAML specifications and six obsolete guide chapters have been removed; the surviving tool guides and examples use the generated document.

Over 400 shared-client path expressions use generated builders. Operation-indexed response types are used in over 100 requests and typed body helpers cover ordinary JSON literals while preserving existing parsers and public interfaces. Generated builders preserve the existing encoding when required and have a small path-only runtime lookup. A regression check rejects new hand-written API paths. Six dormant/development calls are explicitly recorded below rather than inventing server routes for them.

An overlapping event-stream reconnect now retries streaming instead of permanently switching to long-polling.

# Decisions I made

- Preserve existing decoding, authorization order, response statuses, headers, ETags, streaming and multipart behavior. The `RegisterHTTP` adapter is a typed declaration/binding path on apikit; it retains the handler’s access/HTTP behavior and enforces credential scope before entering it.
- Classify every concrete route in the restriction matrices as the generated admission table expands; probe title-bearing reads and mutations and distinguish grant credentials from sessions.
- Keep all admission decisions from the previous production table. The table is now generated from each operation’s declaration; a source/artifact guard checks the same registry. Non-API web/receiver infrastructure remains outside the API registry.
- Do not unify legacy error envelopes, body defaults or revision encodings during a registration migration. The generated document represents both retry-envelope families. Signed certificates retain their existing opaque public-client boundary and are verified by the server.
- Publish generated contracts from the existing Hosted registry for the shared account/connectivity clients as a separate `packages/contracts/src/hosted` module. No Hosted backend was edited or contacted. `packages/contracts/scripts/sync-hosted.mjs` makes the sync reproducible from its generated directory and applies the shared builder template. Hosted profile-management endpoints absent from that current registry remain an explicit gap.
- Keep the six existing unavailable/development client call literals in a bounded lint allowance; any additional occurrence or different path fails. The production contract does not advertise endpoints the server does not serve.
- Execution host is the personal Ubuntu host `ehler-homelab-mini-1`. Context sync could not write its lock in this sandbox; existing canonical project and verification guidance was read directly.
- No commits, pushes, deployment, full test suite, platform build or helper/subagent sessions were performed. Cumulative snapshots were used.

# Bugs

- B-08.1: already fixed on this base: request locality uses trusted proxy origin handling and accepts an HTTPS browser origin for the same backend host; `request_locality.go:sameRequestOrigin`.
- B-08.2: not fixed: the retained `failure` classifier still has the broad invalid-request fallback. Reclassifying hundreds of domain errors changes behavior and belongs to the server/error lane.
- B-08.3: fixed here: an SSE 409 retries streaming with its cursor; only 404/405 permanently disable streaming. The overlapping-reconnect test passes.
- B-08.4: already fixed: `events_routes.go` sends a `stream.opened` frame containing the current cursor before waiting.
- B-08.5: already fixed: server capabilities and the shared event client both publish/clamp the 25-second maximum.
- B-08.6: not fixed: event-ring wake fan-out remains; no load experiment or redesign was authorized in this registration lane. Needed from freshness/performance.
- B-08.7: already fixed on this base: sign-in/rate/fairness peers use the shared trusted-proxy address resolver rather than a single forwarded address.
- B-08.8: already fixed: grid ETags use catalogue, semantic viewer and policy revisions plus the time bucket, not the process-wide write count or token generation.
- B-08.9: not fixed as a universal serializer change: existing typed JSON routes retain their null and 512 KiB checks; the newly registered HTTP handlers retain their previous serializers. The body/response safety bounds were not weakened.
- B-08.10: already fixed: shared API errors recognize the registry retry directive. Generated OpenAPI now includes a default envelope for retained handler errors as well.
- B-08.11: fixed for this lane’s document/registry disagreement: the published route document comes from the live registry, the old specifications are deleted, and the scope/admission guards read the generated declarations. Cursor-key rotation and uniform domain idempotency remain separate server work.
- B-08.12: already partly fixed: hashed web assets are immutable and compression honors its request negotiation. Strong/compressed ETag and recomposition costs are retained; no response behavior was changed here.
- B-08.13: already partly fixed: invalid viewer-item/library parameter declarations are rejected at registration. HTTP-adapter contracts explicitly declare that their handlers retain access checks; no authorization policy was changed. Generic administrative scope enforcement still needs the owning lane’s review.
- B-08.14: not fixed: revision-fenced full-library walks remain a documented trade-off. No compatibility cursor or snapshot-export subsystem was added.
- B-08.15: already fixed: original media MIME comes from the actual container in `media_original.go`.
- B-08.16: already fixed: the public system document omits build ID, source digest and build time.
- B-08.17: not fixed: existing realtime pool limits and sharing remain; registration now derives their exact admission decisions without a second hand table. Capacity acceptance belongs to performance/freshness.
- B-08.18: not fixed: `cmd/server/main.go` still has the 15-second read timeout. Upload-specific deadline changes are outside this lane; body limits are documented and retained.

- B-16.1: already fixed on this base: library list lifetime uses deferred service disposal while the window is saved; no UI changes made here.
- B-16.2: already partly fixed: viewer clearing takes a retiring family and is conditional; multi-tab acceptance still belongs to web/identity.
- B-16.3: already fixed: all content-hashed web assets receive immutable caching.
- B-16.4: not fixed here: virtual-scroll mapping is UI work; no grid code was changed.
- B-16.5: not fixed here: observer/layout and renderer memoization costs are UI/performance work.
- B-16.6: not fixed as a whole: current-I18n/module formatter inconsistencies still exist. This lane adds no screen text; needed from web/i18n.
- B-16.7: already partly fixed: shared event streams and visibility-aware feed management exist in client-core. Multi-tab HTTP/1 acceptance remains for web/freshness.
- B-16.8: not fixed as a whole: some narrow transports have deadlines, but the general request/route layers still need the resilience lane’s policy. Existing deadlines were retained.
- B-16.9: already fixed in the recorded base fix pass: failed lazy imports can reload. Browser acceptance is not claimed here.
- B-16.10: already fixed for the cited Home enum: unknown artwork shapes use the safe fallback and unreadable entries are skipped. This migration deliberately retains tolerant parsers.
- B-16.11: already fixed: connection status covers an inert current screen rather than unmounting it.
- B-16.12: already partly fixed: card CSS has a :has fallback. Browser-floor and all CSS acceptance remain for web.
- B-16.13: already fixed: shell main/skip navigation exists.
- B-16.14: already partly fixed: `page-title.tsx` assigns and announces titles. The fix log records announcement timing/same-name gaps for web.
- B-16.15: already fixed in the base: accessible card labels include viewing state.
- B-16.16: already partly fixed: windowed grids publish list position/size. Physical keyboard/screen-reader focus acceptance remains for web.
- B-16.17: already partly fixed: TextArea and Select now reference help/error descriptions. Pixel font sizing and contrast support remain UI work.
- B-16.18: not fixed here: Radix/CSP scroll-lock behavior requires a server-served browser check and the web lane.
- B-16.19: already fixed: browse vocabulary uses a bounded, sign-in-scoped revalidating cache.
- B-16.20: already fixed for server-directed delay: the windowed reader computes retry waits and honors `waitUntil`; the 5-second default remains a fallback.
- B-16.21: already fixed: cursor history resets when its dependency tuple changes.
- B-16.22: not fixed here: credits read concurrency and failure presentation belong to the web detail lane; no screen code changed.
- B-16.23: not fixed here: React hook lint/tooling policy is outside the shared API registry work.
- B-16.24: not fixed here: production dev-screen bundle contents require the web build lane; no full web build run.
- B-16.25: not fixed here: CI/image Node alignment belongs to base-health/release. No Docker or CI rollout changes made.

The statuses above combine current static source checks with the recorded fix log; they are not browser, hardware or load reproductions. A-08.3 is resolved for the production server’s registry coverage. A-16.12 is substantially addressed by generated paths/types; the six exact call-site gaps below prevent claiming complete removal of path literals.

# API changes

No existing production route was removed or renamed and no request/response behavior was intentionally changed. Newly generated operation identifiers and HTTP/admission metadata are additive declarations. Health checks are also documented. Receiver assets and the web fallback remain infrastructure rather than API actions. Automatic OPTIONS handling remains middleware.

The old `server/api/*.yaml` files, retired playback examples and duplicate endpoint reference were removed. `TestOpenAPICoversRegisteredRoutes` no longer inserts the live registry into its parsed document (which had hidden stale generation); it verifies the actual JSON artifact. Named schema and guide-example guards also use that artifact.

Known shape variations intentionally retained: administration returns `{protocolVersion,serverId,result}`; console/notifications use `{scope,data}`; other reads often return a direct document. Revision fields differ between numeric and string encodings. The same inventory/scan action route can return a JSON result or 204; these alternate statuses are documented. Health readiness can return 503 with its readiness document. Existing media handlers choose their actual MIME and range status at runtime.

Inventory: every declared production API operation is below. A conditional HTTP family mounts when its corresponding service is configured; existing typed families may instead return their normal unavailable response. `HTTP handler` means the typed adapter retains the original authorization/serializer. `typed/raw` means an existing typed or raw registration. Body bounds are bytes. Lane is the broad API work class; admission is the precise server pool.

| Production operation | Registry | Key scope | Access | Lane | Cost | Admission | Body limit | Registration |
|---|---|---|---|---|---|---|---:|---|
| `GET /health/live` | yes | `open` | `public` | `default` | `constant` | `default` | 0 | HTTP handler |
| `GET /health/ready` | yes | `open` | `public` | `default` | `constant` | `default` | 0 | HTTP handler |
| `POST /v1/access/invitations/accept` | yes | `session` | `public` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/access/invitations/preview` | yes | `session` | `public` | `security` | `constant` | `default` | 4096 | typed/raw |
| `GET /v1/admin/access-tokens` | yes | `session` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `DELETE /v1/admin/access-tokens/{id}` | yes | `session` | `owner` | `security` | `constant` | `security-fence` | 0 | typed/raw |
| `GET /v1/admin/access/devices` | yes | `admin` | `admin_scope` | `security` | `page_sized` | `admin` | 0 | typed/raw |
| `PUT /v1/admin/access/devices/{id}/trust` | yes | `session` | `admin_scope` | `security` | `constant` | `security-fence` | 4096 | typed/raw |
| `GET /v1/admin/access/invitations` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/access/invitations` | yes | `admin` | `admin_scope` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `POST /v1/admin/access/invitations/{id}/revoke` | yes | `admin` | `admin_scope` | `security` | `constant` | `security-fence` | 65536 | HTTP handler |
| `GET /v1/admin/access/members` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/access/members/{id}/limits` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `PUT /v1/admin/access/members/{id}/limits` | yes | `admin` | `admin_scope` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `PUT /v1/admin/access/members/{id}/role` | yes | `admin` | `admin_scope` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `GET /v1/admin/analysis-operations` | yes | `library` | `owner` | `default` | `constant` | `admin-heavy` | 0 | HTTP handler |
| `GET /v1/admin/api-keys` | yes | `session` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `POST /v1/admin/api-keys` | yes | `session` | `owner` | `security` | `constant` | `security-fence` | 4096 | typed/raw |
| `DELETE /v1/admin/api-keys/{id}` | yes | `session` | `owner` | `security` | `constant` | `security-fence` | 0 | typed/raw |
| `GET /v1/admin/attention` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/backups` | yes | `admin` | `owner` | `default` | `constant` | `admin-heavy` | 0 | typed/raw |
| `POST /v1/admin/backups` | yes | `admin` | `owner` | `default` | `constant` | `admin-heavy` | 4096 | typed/raw |
| `POST /v1/admin/backups/restore` | yes | `admin` | `owner` | `default` | `constant` | `admin-long` | 4096 | typed/raw |
| `DELETE /v1/admin/backups/{id}` | yes | `admin` | `owner` | `default` | `constant` | `admin-heavy` | 0 | typed/raw |
| `GET /v1/admin/catalogue/failures` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `GET /v1/admin/connectivity/policy` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | typed/raw |
| `PATCH /v1/admin/connectivity/policy` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 32768 | typed/raw |
| `GET /v1/admin/connectivity/status` | yes | `admin` | `admin_scope` | `default` | `constant` | `admin` | 0 | typed/raw |
| `GET /v1/admin/console/accounts` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/console/alerts` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `POST /v1/admin/console/alerts/{id}/acknowledge` | yes | `admin` | `owner` | `default` | `constant` | `admin-heavy` | 65536 | HTTP handler |
| `POST /v1/admin/console/diagnostics/capture` | yes | `admin` | `device` | `default` | `page_sized` | `admin-heavy` | 1048576 | typed/raw |
| `GET /v1/admin/console/diagnostics/{lane}` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `POST /v1/admin/console/exports` | yes | `admin` | `device` | `default` | `page_sized` | `admin-heavy` | 1048576 | typed/raw |
| `GET /v1/admin/console/exports/{id}` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `GET /v1/admin/console/job-kinds` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/console/jobs` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `POST /v1/admin/console/jobs` | yes | `admin` | `device` | `default` | `page_sized` | `admin-heavy` | 1048576 | typed/raw |
| `POST /v1/admin/console/jobs/{id}/{action}` | yes | `admin` | `device` | `default` | `page_sized` | `admin-heavy` | 1048576 | typed/raw |
| `GET /v1/admin/console/panels/{panel}` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `GET /v1/admin/console/schedules` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `PUT /v1/admin/console/schedules` | yes | `admin` | `device` | `default` | `page_sized` | `admin-heavy` | 1048576 | typed/raw |
| `GET /v1/admin/console/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | typed/raw |
| `PATCH /v1/admin/console/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | typed/raw |
| `GET /v1/admin/console/stream-options` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/diagnostics/bundle` | yes | `admin` | `owner` | `default` | `constant` | `admin-heavy` | 0 | HTTP handler |
| `GET /v1/admin/diagnostics/capabilities` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | typed/raw |
| `GET /v1/admin/diagnostics/client-logs` | yes | `admin` | `owner` | `default` | `page_sized` | `admin-heavy` | 0 | typed/raw |
| `DELETE /v1/admin/diagnostics/client-logs/{id}` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | typed/raw |
| `GET /v1/admin/diagnostics/client-logs/{id}` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | typed/raw |
| `GET /v1/admin/diagnostics/client-logs/{id}/download` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | typed/raw |
| `GET /v1/admin/diagnostics/concurrency` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/diagnostics/integrity` | yes | `admin` | `owner` | `default` | `constant` | `admin-long` | 0 | HTTP handler |
| `POST /v1/admin/diagnostics/integrity/repair` | yes | `admin` | `owner` | `default` | `constant` | `admin-long` | 0 | HTTP handler |
| `GET /v1/admin/diagnostics/pprof/{profile}` | yes | `admin` | `owner` | `default` | `constant` | `admin-heavy` | 0 | HTTP handler |
| `POST /v1/admin/downloads/revoke` | yes | `playback` | `owner` | `security` | `page_sized` | `security-fence` | 4096 | typed/raw |
| `GET /v1/admin/dvr/recording-groups` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/dvr/recording-groups` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `DELETE /v1/admin/dvr/recording-groups/{id}` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `PUT /v1/admin/dvr/recording-groups/{id}` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `GET /v1/admin/dvr/recording-owners` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/dvr/recording-permissions` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `PUT /v1/admin/dvr/recording-permissions` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `GET /v1/admin/dvr/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `PUT /v1/admin/dvr/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `GET /v1/admin/dvr/tuners` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `GET /v1/admin/feedback` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `GET /v1/admin/feedback/reports` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `GET /v1/admin/feedback/reports/{id}` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `POST /v1/admin/feedback/reports/{id}/status` | yes | `admin` | `device` | `default` | `page_sized` | `admin-heavy` | 1048576 | typed/raw |
| `GET /v1/admin/feedback/{id}` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `PATCH /v1/admin/feedback/{id}` | yes | `admin` | `device` | `default` | `page_sized` | `admin-heavy` | 1048576 | typed/raw |
| `GET /v1/admin/filesystem` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `DELETE /v1/admin/ingestion/jobs/{id}` | yes | `library` | `owner` | `default` | `constant` | `default` | 0 | HTTP handler |
| `GET /v1/admin/ingestion/jobs/{id}` | yes | `library` | `owner` | `default` | `constant` | `default` | 0 | HTTP handler |
| `POST /v1/admin/ingestion/jobs/{id}/{action}` | yes | `library` | `owner` | `default` | `constant` | `default` | 0 | HTTP handler |
| `DELETE /v1/admin/items/{id}/files/{fileId}` | yes | `library` | `owner` | `default` | `page_sized` | `admin-heavy` | 4096 | typed/raw |
| `POST /v1/admin/items/{id}/files/{fileId}/delete/preview` | yes | `library` | `owner` | `default` | `page_sized` | `admin-heavy` | 4096 | typed/raw |
| `GET /v1/admin/jobs` | yes | `library` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/libraries` | yes | `library` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `PUT /v1/admin/libraries/order` | yes | `library` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `DELETE /v1/admin/libraries/{id}` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 4096 | typed/raw |
| `GET /v1/admin/libraries/{id}` | yes | `library` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `PATCH /v1/admin/libraries/{id}` | yes | `library` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `GET /v1/admin/libraries/{id}/inventory` | yes | `library` | `owner` | `default` | `page_sized` | `admin-heavy` | 0 | typed/raw |
| `GET /v1/admin/libraries/{id}/inventory-config` | yes | `library` | `owner` | `default` | `constant` | `admin-heavy` | 0 | HTTP handler |
| `POST /v1/admin/libraries/{id}/inventory/{object}/{action}` | yes | `library` | `owner` | `default` | `constant` | `admin-heavy` | 65536 | HTTP handler |
| `PUT /v1/admin/libraries/{id}/scan-policy` | yes | `library` | `owner` | `default` | `constant` | `admin-heavy` | 65536 | HTTP handler |
| `GET /v1/admin/libraries/{id}/scans/{job}/issues` | yes | `library` | `owner` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/admin/libraries/{id}/settings` | yes | `library` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `PUT /v1/admin/libraries/{id}/settings` | yes | `library` | `owner` | `default` | `constant` | `admin` | 65536 | typed/raw |
| `PUT /v1/admin/libraries/{id}/shows/{show}/settings` | yes | `library` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `POST /v1/admin/libraries/{id}/sources` | yes | `library` | `owner` | `default` | `constant` | `admin-heavy` | 65536 | HTTP handler |
| `DELETE /v1/admin/libraries/{id}/sources/{source}` | yes | `library` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `PATCH /v1/admin/libraries/{id}/sources/{source}` | yes | `library` | `owner` | `default` | `constant` | `admin-heavy` | 65536 | HTTP handler |
| `GET /v1/admin/libraries/{id}/sources/{source}/changes` | yes | `library` | `owner` | `browsing` | `page_sized` | `admin` | 0 | typed/raw |
| `POST /v1/admin/libraries/{id}/sources/{source}/check` | yes | `library` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/libraries/{id}/sources/{source}/scans` | yes | `library` | `owner` | `default` | `constant` | `admin-heavy` | 0 | HTTP handler |
| `GET /v1/admin/library-channels` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/library-channels` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 1048576 | HTTP handler |
| `GET /v1/admin/library-channels/block-presets` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/library-channels/block-presets` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `DELETE /v1/admin/library-channels/block-presets/{id}` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `PUT /v1/admin/library-channels/block-presets/{id}` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `GET /v1/admin/library-channels/criteria` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/library-channels/defaults` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 4096 | HTTP handler |
| `POST /v1/admin/library-channels/preview` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 1048576 | HTTP handler |
| `POST /v1/admin/library-channels/reorder` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 32768 | HTTP handler |
| `GET /v1/admin/library-channels/templates` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/library-channels/templates/{id}/install` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 4096 | HTTP handler |
| `POST /v1/admin/library-channels/{id}/delete` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 4096 | HTTP handler |
| `GET /v1/admin/library-channels/{id}/health` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/library-channels/{id}/logo` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 11534336 | HTTP handler |
| `POST /v1/admin/library-channels/{id}/regenerate` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 4096 | HTTP handler |
| `GET /v1/admin/library-settings` | yes | `library` | `owner` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/live-sources` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `POST /v1/admin/live-sources` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `upload` | 50331648 | typed/raw |
| `POST /v1/admin/live-sources/discover` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `POST /v1/admin/live-sources/preview` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `upload` | 50331648 | typed/raw |
| `POST /v1/admin/live-sources/remote/preview` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 8388608 | typed/raw |
| `POST /v1/admin/live-sources/remote/save` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 4096 | typed/raw |
| `GET /v1/admin/live-sources/{id}/channel-map` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `PUT /v1/admin/live-sources/{id}/channel-map` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `GET /v1/admin/live-sources/{id}/configuration` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `PUT /v1/admin/live-sources/{id}/configuration` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 8388608 | typed/raw |
| `POST /v1/admin/live-sources/{id}/enabled` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `POST /v1/admin/live-sources/{id}/logos` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `POST /v1/admin/live-sources/{id}/refresh` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `POST /v1/admin/live-sources/{id}/remove` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `GET /v1/admin/live/settings` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `PUT /v1/admin/live/settings` | yes | `admin` | `admin_scope` | `default` | `page_sized` | `admin` | 65536 | typed/raw |
| `GET /v1/admin/live/sources/{id}/channels` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `GET /v1/admin/live/sources/{id}/groups` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `GET /v1/admin/logs` | yes | `admin` | `owner` | `default` | `page_sized` | `admin-heavy` | 0 | HTTP handler |
| `POST /v1/admin/logs/debug-window` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `GET /v1/admin/logs/events` | yes | `admin` | `owner` | `realtime` | `constant` | `realtime` | 0 | HTTP handler |
| `GET /v1/admin/logs/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `PATCH /v1/admin/logs/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `POST /v1/admin/maintenance/background-pause` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 4096 | typed/raw |
| `GET /v1/admin/maintenance/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | typed/raw |
| `PUT /v1/admin/maintenance/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | typed/raw |
| `POST /v1/admin/media/delete` | yes | `library` | `owner` | `default` | `constant` | `admin-heavy` | 65536 | HTTP handler |
| `POST /v1/admin/media/delete/preview` | yes | `library` | `owner` | `default` | `constant` | `admin-heavy` | 65536 | HTTP handler |
| `GET /v1/admin/metadata-providers` | yes | `library` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/metadata/attention` | yes | `library` | `owner` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/admin/metadata/attention/counts` | yes | `library` | `owner` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/admin/metadata/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | typed/raw |
| `PUT /v1/admin/metadata/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 32768 | typed/raw |
| `GET /v1/admin/missing-titles` | yes | `library` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `POST /v1/admin/missing-titles/empty` | yes | `library` | `owner` | `default` | `page_sized` | `admin-heavy` | 4096 | typed/raw |
| `POST /v1/admin/notifications/broadcast` | yes | `admin` | `owner` | `default` | `constant` | `admin-heavy` | 65536 | HTTP handler |
| `GET /v1/admin/notifications/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `PATCH /v1/admin/notifications/settings` | yes | `admin` | `owner` | `default` | `constant` | `admin-heavy` | 65536 | HTTP handler |
| `GET /v1/admin/operations/{panel}` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/play-history` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `GET /v1/admin/play-history/summary` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `GET /v1/admin/playback-operations/{operationId}` | yes | `playback` | `owner` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/playback-sessions` | yes | `playback` | `owner` | `default` | `page_sized` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/playback-sessions/{id}/stop` | yes | `playback` | `owner` | `default` | `constant` | `admin-heavy` | 65536 | HTTP handler |
| `GET /v1/admin/playback/diagnostics` | yes | `admin` | `owner` | `media` | `constant` | `playback` | 0 | HTTP handler |
| `GET /v1/admin/playback/history` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 1048576 | typed/raw |
| `GET /v1/admin/playback/history.csv` | yes | `admin` | `owner` | `media` | `page_sized` | `playback` | 0 | HTTP handler |
| `GET /v1/admin/resource-usage` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `POST /v1/admin/restart` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 4096 | typed/raw |
| `GET /v1/admin/sessions` | yes | `playback` | `admin_scope` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `GET /v1/admin/sessions/{id}/controls` | yes | `playback` | `admin_scope` | `media` | `constant` | `media` | 0 | typed/raw |
| `POST /v1/admin/sessions/{id}/controls` | yes | `playback` | `admin_scope` | `media` | `constant` | `media` | 4096 | typed/raw |
| `POST /v1/admin/sessions/{id}:terminate` | yes | `playback` | `admin_scope` | `default` | `constant` | `admin` | 4096 | typed/raw |
| `POST /v1/admin/state-permissions:fix` | yes | `admin` | `owner` | `default` | `constant` | `admin-heavy` | 4096 | typed/raw |
| `GET /v1/admin/storage-usage` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/storage-usage/{category}/cleanup` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 65536 | HTTP handler |
| `GET /v1/admin/storage/status` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `DELETE /v1/admin/storage/upgrade-copies/{id}` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | typed/raw |
| `GET /v1/admin/support-report` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/telemetry` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/telemetry/now` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `POST /v1/admin/titles/merge` | yes | `library` | `owner` | `default` | `page_sized` | `admin-heavy` | 4096 | typed/raw |
| `GET /v1/admin/transcode/capacity` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | HTTP handler |
| `GET /v1/admin/updates` | yes | `admin` | `owner` | `default` | `constant` | `admin` | 0 | typed/raw |
| `GET /v1/admin/webhooks` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `POST /v1/admin/webhooks` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 8192 | typed/raw |
| `DELETE /v1/admin/webhooks/{id}` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `PATCH /v1/admin/webhooks/{id}` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 8192 | typed/raw |
| `GET /v1/admin/webhooks/{id}/deliveries` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `POST /v1/admin/webhooks/{id}/test` | yes | `admin` | `owner` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `GET /v1/albums/{id}/metadata/musicbrainz` | yes | `library` | `owner` | `default` | `constant` | `default` | 0 | HTTP handler |
| `PUT /v1/albums/{id}/metadata/musicbrainz` | yes | `library` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/albums/{id}/metadata/musicbrainz/policy` | yes | `library` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/albums/{id}/metadata/musicbrainz/retry` | yes | `library` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/albums/{id}/metadata/musicbrainz/search` | yes | `library` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/albums/{id}/songs` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | HTTP handler |
| `GET /v1/artists/{id}/albums` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | HTTP handler |
| `GET /v1/attribution` | yes | `open` | `public` | `default` | `page_sized` | `default` | 0 | HTTP handler |
| `GET /v1/auth/capabilities` | yes | `open` | `public` | `default` | `constant` | `auth` | 0 | typed/raw |
| `POST /v1/auth/refresh` | yes | `session` | `public` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `POST /v1/auth/register` | yes | `session` | `public` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `POST /v1/auth/remembered-accounts` | yes | `session` | `public` | `security` | `constant` | `auth` | 4096 | typed/raw |
| `POST /v1/auth/two-factor/challenge` | yes | `session` | `public` | `security` | `constant` | `security-fence` | 65536 | HTTP handler |
| `GET /v1/books/{id}/chapters` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | HTTP handler |
| `GET /v1/books/{id}/files` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | HTTP handler |
| `GET /v1/bootstrap` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/capabilities` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | typed/raw |
| `POST /v1/cast/bootstrap` | yes | `session` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `GET /v1/cast/bootstrap/{id}` | yes | `session` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/cast/configuration` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `PUT /v1/cast/configuration` | yes | `admin` | `owner` | `default` | `constant` | `default` | 262144 | typed/raw |
| `DELETE /v1/cast/devices/{id}` | yes | `session` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `POST /v1/cast/reconnect` | yes | `session` | `public` | `security` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/cast/redeem` | yes | `session` | `public` | `security` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/channels/preferences` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `DELETE /v1/collections/{id}` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/collections/{id}` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PATCH /v1/collections/{id}` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/collections/{id}/items` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `DELETE /v1/collections/{id}/items/{itemId}` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/collections/{id}/items/{itemId}` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `POST /v1/collections/{id}/items:batch` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/console/registry` | yes | `admin` | `device` | `default` | `page_sized` | `admin` | 0 | typed/raw |
| `GET /v1/containers/{kind}/{id}/personal-state` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `PUT /v1/containers/{kind}/{id}/personal-state` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 2048 | typed/raw |
| `POST /v1/containers/{kind}/{id}/personal-state/undo` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 2048 | typed/raw |
| `GET /v1/content` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/devices` | yes | `session` | `device` | `security` | `page_sized` | `auth` | 0 | typed/raw |
| `POST /v1/devices` | yes | `session` | `device` | `security` | `constant` | `auth` | 4096 | typed/raw |
| `DELETE /v1/devices/{id}` | yes | `session` | `device` | `security` | `constant` | `auth` | 0 | typed/raw |
| `PATCH /v1/devices/{id}` | yes | `session` | `device` | `security` | `constant` | `auth` | 4096 | typed/raw |
| `POST /v1/devices/{id}/approval` | yes | `session` | `device` | `security` | `constant` | `auth` | 4096 | typed/raw |
| `DELETE /v1/devices/{id}/sessions` | yes | `session` | `device` | `security` | `constant` | `auth` | 0 | typed/raw |
| `POST /v1/devices/{id}/sessions/bind` | yes | `session` | `device` | `security` | `constant` | `auth` | 4096 | typed/raw |
| `POST /v1/diagnostics/client-logs` | yes | `session` | `device` | `default` | `constant` | `upload` | 1576960 | typed/raw |
| `GET /v1/direct` | yes | `session` | `device` | `security` | `constant` | `default` | 0 | typed/raw |
| `GET /v1/direct/members` | yes | `session` | `device` | `security` | `constant` | `auth` | 0 | HTTP handler |
| `POST /v1/direct/members` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `DELETE /v1/direct/members/{id}` | yes | `session` | `device` | `security` | `constant` | `security-fence` | 4096 | typed/raw |
| `PATCH /v1/direct/members/{id}` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `GET /v1/direct/members/{id}/deletion-preview` | yes | `session` | `device` | `security` | `constant` | `auth` | 0 | typed/raw |
| `POST /v1/direct/ownership` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `POST /v1/direct/ownership/custody` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `POST /v1/direct/password` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `GET /v1/direct/pin-recovery` | yes | `session` | `admin_scope` | `security` | `constant` | `security-fence` | 0 | typed/raw |
| `POST /v1/direct/portico-challenge` | yes | `session` | `public` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `POST /v1/direct/profile-switch` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `POST /v1/direct/profiles` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `GET /v1/direct/profiles/avatars` | yes | `read` | `device` | `default` | `constant` | `auth` | 0 | typed/raw |
| `PUT /v1/direct/profiles/order` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `DELETE /v1/direct/profiles/{id}` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `PATCH /v1/direct/profiles/{id}` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `DELETE /v1/direct/profiles/{id}/avatar` | yes | `session` | `device` | `security` | `constant` | `auth` | 0 | HTTP handler |
| `POST /v1/direct/profiles/{id}/avatar` | yes | `session` | `device` | `security` | `constant` | `auth` | 5242880 | HTTP handler |
| `GET /v1/direct/profiles/{id}/deletion-preview` | yes | `session` | `device` | `security` | `constant` | `auth` | 0 | HTTP handler |
| `POST /v1/direct/profiles/{id}/pin-reset` | yes | `session` | `admin_scope` | `security` | `constant` | `security-fence` | 4096 | typed/raw |
| `GET /v1/direct/profiles/{id}/restrictions` | yes | `read` | `device` | `security` | `constant` | `security-fence` | 0 | typed/raw |
| `PUT /v1/direct/profiles/{id}/restrictions` | yes | `admin` | `admin_scope` | `security` | `constant` | `security-fence` | 32768 | typed/raw |
| `POST /v1/direct/profiles/{id}/select` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `GET /v1/direct/registration-policy` | yes | `admin` | `owner` | `security` | `constant` | `auth` | 0 | typed/raw |
| `PUT /v1/direct/registration-policy` | yes | `admin` | `owner` | `security` | `constant` | `auth` | 4096 | typed/raw |
| `POST /v1/direct/remembered-accounts` | yes | `session` | `device` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `DELETE /v1/direct/remembered-accounts/{accountId}` | yes | `session` | `device` | `security` | `constant` | `auth` | 4096 | typed/raw |
| `GET /v1/direct/sessions` | yes | `session` | `admin_scope` | `security` | `page_sized` | `auth` | 0 | typed/raw |
| `POST /v1/direct/sessions/sign-out-everywhere` | yes | `session` | `admin_scope` | `security` | `constant` | `auth` | 0 | typed/raw |
| `DELETE /v1/direct/sessions/{id}` | yes | `session` | `admin_scope` | `security` | `constant` | `security-fence` | 0 | typed/raw |
| `POST /v1/direct/sign-in` | yes | `session` | `public` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `DELETE /v1/direct/trust` | yes | `session` | `device` | `security` | `constant` | `security-fence` | 65536 | HTTP handler |
| `DELETE /v1/direct/two-factor` | yes | `session` | `device` | `security` | `constant` | `security-fence` | 65536 | HTTP handler |
| `GET /v1/direct/two-factor` | yes | `session` | `device` | `security` | `constant` | `security-fence` | 0 | HTTP handler |
| `POST /v1/direct/two-factor/enrol` | yes | `session` | `device` | `security` | `constant` | `security-fence` | 65536 | HTTP handler |
| `POST /v1/direct/two-factor/verify` | yes | `session` | `device` | `security` | `constant` | `security-fence` | 65536 | HTTP handler |
| `GET /v1/downloads/artifacts/{grant}` | yes | `open` | `public` | `media` | `constant` | `bulk-transfer` | 0 | typed/raw |
| `HEAD /v1/downloads/artifacts/{grant}` | yes | `open` | `public` | `media` | `constant` | `bulk-transfer` | 0 | typed/raw |
| `GET /v1/downloads/artifacts/{grant}/artwork/{role}` | yes | `open` | `public` | `media` | `constant` | `media` | 0 | typed/raw |
| `GET /v1/downloads/artifacts/{grant}/subtitles/{resource}/{revision}.vtt` | yes | `open` | `public` | `media` | `constant` | `media` | 0 | typed/raw |
| `GET /v1/downloads/artifacts/{grant}/subtitles/{resource}/{revision}/{file}` | yes | `open` | `public` | `media` | `constant` | `media` | 0 | typed/raw |
| `GET /v1/downloads/preparations` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `POST /v1/downloads/preparations` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 32768 | typed/raw |
| `GET /v1/downloads/preparations/{id}` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | typed/raw |
| `POST /v1/downloads/preparations/{id}/actions` | yes | `playback` | `device` | `default` | `constant` | `default` | 4096 | typed/raw |
| `GET /v1/downloads/preparations/{id}/bundle` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `POST /v1/downloads/preparations/{id}/grant` | yes | `playback` | `device` | `default` | `constant` | `default` | 4096 | typed/raw |
| `POST /v1/downloads/progress` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 65536 | typed/raw |
| `GET /v1/downloads/receipt-keys` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | typed/raw |
| `POST /v1/downloads/receipts` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 32768 | typed/raw |
| `POST /v1/downloads/receipts/revalidate` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 32768 | typed/raw |
| `GET /v1/downloads/receipts/revocations` | yes | `read` | `device` | `security` | `page_sized` | `security-fence` | 0 | typed/raw |
| `POST /v1/downloads/receipts/revoke` | yes | `playback` | `device` | `security` | `page_sized` | `security-fence` | 32768 | typed/raw |
| `GET /v1/downloads/requests` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `POST /v1/downloads/requests` | yes | `playback` | `device` | `default` | `selection_sized_async` | `default` | 8192 | typed/raw |
| `GET /v1/downloads/requests/{id}` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/downloads/settings` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | typed/raw |
| `PUT /v1/downloads/settings` | yes | `admin` | `owner` | `default` | `constant` | `default` | 4096 | typed/raw |
| `GET /v1/downloads/usage` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `GET /v1/dvr` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/dvr/channels` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `POST /v1/dvr/recordings` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `DELETE /v1/dvr/recordings/{id}` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/dvr/recordings/{id}` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PATCH /v1/dvr/recordings/{id}` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/dvr/recordings/{id}/cancel` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/dvr/recordings/{id}/delete-preview` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/dvr/recordings/{id}/keep` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/dvr/rules/preview` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `DELETE /v1/dvr/rules/{id}` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `PUT /v1/dvr/rules/{id}` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/dvr/storage` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/dvr/storage` | yes | `admin` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/events` | yes | `read` | `device` | `realtime` | `constant` | `realtime` | 0 | typed/raw |
| `GET /v1/feedback` | yes | `read` | `device` | `default` | `page_sized` | `default` | 1048576 | typed/raw |
| `POST /v1/feedback` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 1048576 | typed/raw |
| `GET /v1/feedback/capabilities` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | HTTP handler |
| `GET /v1/feedback/reports` | yes | `read` | `device` | `default` | `page_sized` | `default` | 1048576 | typed/raw |
| `POST /v1/feedback/reports` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 1048576 | typed/raw |
| `GET /v1/feedback/reports/{id}` | yes | `read` | `device` | `default` | `page_sized` | `default` | 1048576 | typed/raw |
| `GET /v1/feedback/{id}` | yes | `read` | `device` | `default` | `page_sized` | `default` | 1048576 | typed/raw |
| `GET /v1/groups` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `POST /v1/groups` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/groups/join` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `GET /v1/groups/{id}` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `POST /v1/groups/{id}/end` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `GET /v1/groups/{id}/events` | yes | `read` | `device` | `realtime` | `page_sized` | `realtime` | 0 | typed/raw |
| `POST /v1/groups/{id}/heartbeat` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `POST /v1/groups/{id}/host-transfer` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/groups/{id}/invites` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `DELETE /v1/groups/{id}/invites/{inviteId}` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `POST /v1/groups/{id}/leave` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `GET /v1/groups/{id}/queue` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `POST /v1/groups/{id}/queue` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/groups/{id}/readiness` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/groups/{id}/settings` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/groups/{id}/transport` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `GET /v1/guide` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | HTTP handler |
| `GET /v1/guide/channels` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/guide/images/{digest}` | yes | `read` | `device` | `media` | `constant` | `media` | 0 | HTTP handler |
| `GET /v1/guide/sources` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `POST /v1/handoffs` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `GET /v1/handoffs/{id}` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `POST /v1/handoffs/{id}/commit` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/handoffs/{id}/readiness` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/handoffs/{id}/rollback` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `GET /v1/home` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/home/layout` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/home/layout` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/home/layout/reset` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/home/rows/{id}` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `POST /v1/hosted/wake` | yes | `session` | `public` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `GET /v1/items` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `PUT /v1/items/personal-state:batch` | yes | `playback` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `GET /v1/items/{id}` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `GET /v1/items/{id}/analysis` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/items/{id}/analysis/artifacts/{artifact}` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `POST /v1/items/{id}/analysis/jobs` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 2097152 | HTTP handler |
| `POST /v1/items/{id}/analysis/jobs/{job}/control` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 2097152 | HTTP handler |
| `POST /v1/items/{id}/analysis/markers` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 2097152 | HTTP handler |
| `PUT /v1/items/{id}/analysis/policy` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 2097152 | HTTP handler |
| `GET /v1/items/{id}/art/{kind}` | yes | `read` | `device` | `media` | `constant` | `media` | 0 | HTTP handler |
| `GET /v1/items/{id}/chapters/{chapter}/image` | yes | `read` | `device` | `default` | `constant` | `media` | 0 | typed/raw |
| `GET /v1/items/{id}/credits` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `POST /v1/items/{id}/delete` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/items/{id}/delete/preview` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/items/{id}/detail` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `GET /v1/items/{id}/listening` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/items/{id}/lyrics` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `POST /v1/items/{id}/lyrics` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 2097152 | HTTP handler |
| `POST /v1/items/{id}/lyrics/search` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 2097152 | HTTP handler |
| `PUT /v1/items/{id}/lyrics/selection` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 2097152 | HTTP handler |
| `DELETE /v1/items/{id}/lyrics/{resource}` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 2097152 | HTTP handler |
| `PATCH /v1/items/{id}/lyrics/{resource}/offset` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 2097152 | HTTP handler |
| `GET /v1/items/{id}/lyrics/{resource}/revisions/{revision}` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `POST /v1/items/{id}/metadata/local-audio/policy` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/items/{id}/metadata/manual` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PATCH /v1/items/{id}/metadata/manual` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 262144 | HTTP handler |
| `GET /v1/items/{id}/metadata/musicbrainz` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/items/{id}/metadata/musicbrainz` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/items/{id}/metadata/musicbrainz/policy` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/items/{id}/metadata/musicbrainz/retry` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/items/{id}/metadata/musicbrainz/search` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/items/{id}/metadata/refresh` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 4096 | typed/raw |
| `GET /v1/items/{id}/metadata/screen` | yes | `library` | `owner` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `PUT /v1/items/{id}/metadata/screen` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/items/{id}/metadata/screen/retry` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/items/{id}/metadata/screen/search` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/items/{id}/optimizations` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `PUT /v1/items/{id}/personal-state` | yes | `playback` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `GET /v1/items/{id}/playback-offers` | yes | `read` | `device` | `media` | `constant` | `playback` | 0 | HTTP handler |
| `GET /v1/items/{id}/prepared-versions` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/items/{id}/recommendations` | yes | `read` | `device` | `default` | `page_sized` | `expensive` | 0 | HTTP handler |
| `GET /v1/items/{id}/source-availability` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/items/{id}/trickplay` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/items/{id}/trickplay/{set}/thumbnails.vtt` | yes | `read` | `device` | `media` | `page_sized` | `media` | 0 | HTTP handler |
| `GET /v1/items/{id}/trickplay/{set}/tiles/{tile}.jpg` | yes | `read` | `device` | `default` | `constant` | `media` | 0 | typed/raw |
| `GET /v1/items/{itemId}/download-options` | yes | `read` | `viewer_item_restrictions` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/items/{itemId}/playback-options` | yes | `read` | `viewer_item_restrictions` | `media` | `constant` | `playback` | 0 | typed/raw |
| `GET /v1/items/{itemId}/playback/{session}/subtitles` | yes | `read` | `viewer_item_restrictions` | `default` | `constant` | `playback` | 0 | typed/raw |
| `PUT /v1/items/{itemId}/playback/{session}/subtitles` | yes | `playback` | `viewer_item_restrictions` | `default` | `constant` | `playback` | 805306368 | typed/raw |
| `GET /v1/items/{itemId}/subtitles` | yes | `read` | `viewer_item_restrictions` | `default` | `constant` | `browsing` | 0 | typed/raw |
| `POST /v1/items/{itemId}/subtitles` | yes | `library` | `viewer_item_restrictions` | `default` | `constant` | `default` | 805306368 | typed/raw |
| `POST /v1/items/{itemId}/subtitles/apply` | yes | `library` | `viewer_item_restrictions` | `default` | `constant` | `default` | 805306368 | typed/raw |
| `POST /v1/items/{itemId}/subtitles/import` | yes | `library` | `viewer_item_restrictions` | `default` | `constant` | `default` | 805306368 | typed/raw |
| `POST /v1/items/{itemId}/subtitles/refresh` | yes | `library` | `viewer_item_restrictions` | `default` | `constant` | `default` | 805306368 | typed/raw |
| `POST /v1/items/{itemId}/subtitles/remote` | yes | `library` | `viewer_item_restrictions` | `default` | `constant` | `default` | 805306368 | typed/raw |
| `POST /v1/items/{itemId}/subtitles/search` | yes | `library` | `viewer_item_restrictions` | `default` | `constant` | `default` | 805306368 | typed/raw |
| `DELETE /v1/items/{itemId}/subtitles/{resource}` | yes | `library` | `viewer_item_restrictions` | `default` | `constant` | `default` | 805306368 | typed/raw |
| `PUT /v1/items/{itemId}/subtitles/{resource}` | yes | `library` | `viewer_item_restrictions` | `default` | `constant` | `default` | 805306368 | typed/raw |
| `POST /v1/jobs` | yes | `playback` | `device` | `default` | `selection_sized_async` | `default` | 65536 | typed/raw |
| `GET /v1/jobs/{id}` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | typed/raw |
| `GET /v1/jobs/{id}/failures` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `GET /v1/libraries` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `POST /v1/libraries` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/libraries/paths-changed` | yes | `library` | `owner` | `default` | `page_sized` | `default` | 1048576 | typed/raw |
| `GET /v1/libraries/{id}/artists` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/libraries/{id}/books` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/libraries/{id}/browse` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `POST /v1/libraries/{id}/browse` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 65536 | typed/raw |
| `GET /v1/libraries/{id}/browse-capabilities` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/libraries/{id}/categories` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/libraries/{id}/collections` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `POST /v1/libraries/{id}/collections` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/libraries/{id}/content` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `GET /v1/libraries/{id}/discover` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `POST /v1/libraries/{id}/episode-assignments` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/libraries/{id}/episode-issues` | yes | `library` | `owner` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/libraries/{id}/facets` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/libraries/{id}/inventory-status` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/libraries/{id}/listening/selection` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `POST /v1/libraries/{id}/lyrics/fetch` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/libraries/{id}/metadata/agent` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/libraries/{id}/metadata/agent` | yes | `library` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/libraries/{id}/metadata/attention` | yes | `library` | `owner` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `POST /v1/libraries/{id}/metadata/attention` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 4096 | typed/raw |
| `GET /v1/libraries/{id}/metadata/screen` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/libraries/{id}/metadata/screen` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/libraries/{id}/network-roots` | yes | `library` | `owner` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `POST /v1/libraries/{id}/network-roots` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/libraries/{id}/scans` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/libraries/{id}/show-workspace` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/libraries/{id}/shows` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/libraries/{id}/strm-analysis-policy` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/libraries/{id}/strm-analysis-policy` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/library-channels/preferences` | yes | `playback` | `device` | `default` | `constant` | `default` | 4096 | HTTP handler |
| `GET /v1/library-channels/{id}/logo` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | HTTP handler |
| `GET /v1/library-kinds/{kind}/metadata-agents` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/listening/preferences` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/listening/preferences` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `PUT /v1/lyrics/provider` | yes | `admin` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/mcp` | yes | `open` | `public` | `default` | `constant` | `default` | 0 | typed/raw |
| `POST /v1/mcp` | yes | `open` | `public` | `default` | `page_sized` | `tools` | 1048576 | typed/raw |
| `GET /v1/me` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | HTTP handler |
| `GET /v1/me/access-tokens` | yes | `session` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `POST /v1/me/access-tokens` | yes | `session` | `device` | `security` | `constant` | `security-fence` | 4096 | typed/raw |
| `DELETE /v1/me/access-tokens/{id}` | yes | `session` | `device` | `security` | `constant` | `security-fence` | 0 | typed/raw |
| `GET /v1/me/api-access` | yes | `session` | `device` | `default` | `constant` | `default` | 0 | typed/raw |
| `GET /v1/me/devices/current/capabilities` | yes | `read` | `device` | `default` | `constant` | `browsing` | 0 | typed/raw |
| `PUT /v1/me/devices/current/capabilities` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | typed/raw |
| `GET /v1/me/library-navigation` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | HTTP handler |
| `PUT /v1/me/library-navigation` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/me/recommendations:reset` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/media/linear/{playbackId}/{generation}/{token}/{name}` | yes | `playback` | `device` | `media` | `constant` | `media-body` | 0 | HTTP handler |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}.vtt` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/fonts` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/fonts/{font}` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/fonts/{font}` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.ass` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.ass` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.idx` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.idx` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.mks` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.mks` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.ssa` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.ssa` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.sub` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.sub` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.sup` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.sup` | yes | `open` | `public` | `media` | `constant` | `media-body` | 0 | typed/raw |
| `GET /v1/media/{grant}` | yes | `playback` | `device` | `media` | `constant` | `media-body` | 0 | HTTP handler |
| `GET /v1/media/{grant}/audio` | yes | `playback` | `device` | `media` | `constant` | `media-body` | 0 | HTTP handler |
| `HEAD /v1/media/{grant}/audio` | yes | `playback` | `device` | `media` | `constant` | `media-body` | 0 | HTTP handler |
| `GET /v1/media/{grant}/audio-converted` | yes | `playback` | `device` | `media` | `constant` | `media-body` | 0 | HTTP handler |
| `GET /v1/media/{grant}/{file}` | yes | `playback` | `device` | `media` | `constant` | `media-body` | 0 | HTTP handler |
| `POST /v1/metadata/bulk` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/metadata/{kind}/{id}` | yes | `library` | `owner` | `default` | `constant` | `default` | 0 | HTTP handler |
| `POST /v1/metadata/{kind}/{id}` | yes | `library` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/metadata/{kind}/{id}/art/{role}` | yes | `read` | `owner` | `media` | `page_sized` | `media` | 0 | HTTP handler |
| `POST /v1/metadata/{kind}/{id}/art/{role}/upload` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 11534336 | HTTP handler |
| `DELETE /v1/metadata/{kind}/{id}/art/{role}/upload/{candidateId}` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/metadata/{kind}/{id}/preview` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/networking/certificate` | yes | `admin` | `owner` | `security` | `constant` | `auth` | 0 | HTTP handler |
| `POST /v1/networking/certificate/config` | yes | `admin` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/networking/certificate/retry` | yes | `admin` | `owner` | `default` | `constant` | `default` | 0 | HTTP handler |
| `GET /v1/networking/claim` | yes | `session` | `owner` | `security` | `constant` | `auth` | 0 | HTTP handler |
| `POST /v1/networking/claim/approve` | yes | `session` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/networking/claim/await-approval` | yes | `session` | `owner` | `realtime` | `constant` | `realtime` | 65536 | HTTP handler |
| `POST /v1/networking/claim/cancel` | yes | `session` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/networking/claim/continue` | yes | `session` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/networking/claim/endpoint` | yes | `session` | `owner` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `POST /v1/networking/claim/prepare` | yes | `session` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/networking/claim/web-approval` | yes | `session` | `owner` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `GET /v1/networking/connection` | yes | `open` | `public` | `default` | `constant` | `default` | 0 | typed/raw |
| `POST /v1/networking/endpoint-proof` | yes | `session` | `public` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `POST /v1/networking/identity-proof` | yes | `session` | `public` | `security` | `constant` | `auth` | 4096 | typed/raw |
| `GET /v1/networking/remote` | yes | `admin` | `owner` | `security` | `constant` | `auth` | 0 | HTTP handler |
| `POST /v1/networking/remote/check` | yes | `admin` | `owner` | `default` | `constant` | `default` | 0 | HTTP handler |
| `POST /v1/networking/remote/config` | yes | `admin` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/notifications` | yes | `read` | `device` | `default` | `page_sized` | `default` | 1048576 | typed/raw |
| `GET /v1/notifications/capabilities` | yes | `read` | `device` | `realtime` | `constant` | `realtime` | 0 | HTTP handler |
| `GET /v1/notifications/inbox` | yes | `read` | `device` | `default` | `page_sized` | `default` | 1048576 | typed/raw |
| `POST /v1/notifications/inbox/actions` | yes | `playback` | `device` | `realtime` | `constant` | `realtime` | 65536 | HTTP handler |
| `GET /v1/notifications/unread-count` | yes | `read` | `device` | `default` | `page_sized` | `default` | 1048576 | typed/raw |
| `POST /v1/notifications/{id}/read` | yes | `playback` | `device` | `realtime` | `constant` | `realtime` | 0 | HTTP handler |
| `POST /v1/optimization-jobs/{id}/cancel` | yes | `library` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/optimization-jobs/{id}/retry` | yes | `library` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/people` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/people/{id}` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `GET /v1/people/{id}/portrait` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `GET /v1/personal-history` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `POST /v1/personal-history/actions` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/playback/controls` | yes | `playback` | `device` | `media` | `page_sized` | `media` | 0 | typed/raw |
| `GET /v1/playback/delivery-policy` | yes | `read` | `device` | `media` | `constant` | `playback` | 0 | HTTP handler |
| `POST /v1/playback/sessions` | yes | `playback` | `device` | `media` | `constant` | `playback` | 8192 | typed/raw |
| `DELETE /v1/playback/sessions/{id}` | yes | `playback` | `device` | `media` | `constant` | `security-fence` | 1024 | typed/raw |
| `GET /v1/playback/sessions/{id}` | yes | `read` | `device` | `media` | `constant` | `security-fence` | 0 | typed/raw |
| `PATCH /v1/playback/sessions/{id}` | yes | `playback` | `device` | `media` | `constant` | `playback` | 4096 | typed/raw |
| `GET /v1/playback/sessions/{id}/chapters` | yes | `read` | `device` | `security` | `page_sized` | `security-fence` | 0 | HTTP handler |
| `POST /v1/playback/sessions/{id}/timeline` | yes | `playback` | `device` | `media` | `constant` | `playback` | 4096 | typed/raw |
| `POST /v1/playlists` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `DELETE /v1/playlists/{id}` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/playlists/{id}` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PATCH /v1/playlists/{id}` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/playlists/{id}/content` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `POST /v1/playlists/{id}/entries` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `DELETE /v1/playlists/{id}/entries/{entryId}` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/playlists/{id}/order` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `PUT /v1/playlists/{id}/order` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 65536 | typed/raw |
| `GET /v1/playlists/{id}/share-candidates` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `DELETE /v1/playlists/{id}/shares` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `PUT /v1/playlists/{id}/shares` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/preferences` | yes | `read` | `device` | `default` | `page_sized` | `default` | 0 | HTTP handler |
| `PATCH /v1/preferences` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/prepared-versions/{id}/delete` | yes | `library` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/profiles/{id}/avatar` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | HTTP handler |
| `POST /v1/queues` | yes | `playback` | `device` | `media` | `selection_sized_async` | `playback` | 65536 | typed/raw |
| `DELETE /v1/queues/{id}` | yes | `playback` | `device` | `default` | `constant` | `default` | 0 | typed/raw |
| `GET /v1/queues/{id}` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | typed/raw |
| `PATCH /v1/queues/{id}` | yes | `playback` | `device` | `default` | `constant` | `default` | 1024 | typed/raw |
| `GET /v1/queues/{id}/entries` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `DELETE /v1/queues/{id}/entries/{entryId}` | yes | `playback` | `device` | `default` | `constant` | `default` | 0 | typed/raw |
| `POST /v1/queues/{id}/entries/{entryId}:move` | yes | `playback` | `device` | `default` | `constant` | `default` | 1024 | typed/raw |
| `POST /v1/queues/{id}/segments` | yes | `playback` | `device` | `media` | `selection_sized_async` | `playback` | 32768 | typed/raw |
| `GET /v1/queues/{id}/window` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `POST /v1/queues/{id}:advance` | yes | `playback` | `device` | `media` | `constant` | `playback` | 1024 | typed/raw |
| `POST /v1/queues/{id}:commit-next` | yes | `playback` | `device` | `media` | `constant` | `playback` | 1024 | typed/raw |
| `POST /v1/queues/{id}:prepare-next` | yes | `playback` | `device` | `media` | `constant` | `playback` | 1024 | typed/raw |
| `POST /v1/queues/{id}:save-as-playlist` | yes | `playback` | `device` | `default` | `constant` | `default` | 8192 | typed/raw |
| `POST /v1/quick-connect` | yes | `session` | `public` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/quick-connect/cancel` | yes | `session` | `public` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/quick-connect/decision` | yes | `session` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/quick-connect/review` | yes | `session` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/quick-connect/token` | yes | `session` | `public` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/rating-systems` | yes | `read` | `device` | `default` | `constant` | `default` | 0 | typed/raw |
| `GET /v1/readiness` | yes | `open` | `public` | `default` | `constant` | `default` | 0 | HTTP handler |
| `GET /v1/receivers` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `POST /v1/receivers` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `DELETE /v1/receivers/{id}` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 0 | typed/raw |
| `GET /v1/receivers/{id}/grants` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `POST /v1/receivers/{id}/grants` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/receivers/{id}/grants/{grantId}/decision` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `POST /v1/receivers/{id}/heartbeat` | yes | `playback` | `device` | `default` | `page_sized` | `default` | 262144 | typed/raw |
| `GET /v1/receivers/{id}/inbox` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/receivers/{id}/inbox/wait` | yes | `read` | `device` | `realtime` | `page_sized` | `realtime` | 0 | typed/raw |
| `GET /v1/saved-pins/order` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/saved-pins/order` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `PUT /v1/saved-pins/{kind}/{id}` | yes | `playback` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/saved-resources` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `POST /v1/saved-resources` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `DELETE /v1/saved-resources/{id}` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/saved-resources/{id}` | yes | `read` | `device` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PATCH /v1/saved-resources/{id}` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/saved-resources/{id}/content` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 1048576 | typed/raw |
| `PUT /v1/saved-resources/{id}/entries` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/saved-resources/{id}/share-candidates` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `DELETE /v1/saved-resources/{id}/shares` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `PUT /v1/saved-resources/{id}/shares` | yes | `playback` | `device` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/search` | yes | `read` | `device` | `browsing` | `page_sized` | `expensive` | 0 | typed/raw |
| `DELETE /v1/search/history` | yes | `playback` | `device` | `default` | `constant` | `expensive` | 0 | HTTP handler |
| `GET /v1/search/history` | yes | `read` | `device` | `default` | `constant` | `expensive` | 0 | HTTP handler |
| `GET /v1/seasons/{id}/episodes` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/server` | yes | `open` | `public` | `default` | `constant` | `default` | 0 | typed/raw |
| `DELETE /v1/sessions/current` | yes | `session` | `device` | `security` | `constant` | `security-fence` | 0 | HTTP handler |
| `POST /v1/setup` | yes | `session` | `public` | `security` | `constant` | `auth` | 65536 | typed/raw |
| `POST /v1/setup/browser` | yes | `session` | `public` | `security` | `constant` | `default` | 4096 | typed/raw |
| `POST /v1/setup/finish` | yes | `session` | `owner` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `POST /v1/setup/remote-recovery` | yes | `session` | `owner` | `security` | `constant` | `auth` | 65536 | HTTP handler |
| `POST /v1/setup/resume` | yes | `session` | `public` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/setup/state` | yes | `session` | `owner` | `default` | `constant` | `default` | 0 | HTTP handler |
| `GET /v1/shows/{id}/episodes` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `POST /v1/shows/{id}/metadata/refresh` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 4096 | typed/raw |
| `GET /v1/shows/{id}/metadata/screen` | yes | `library` | `owner` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `PUT /v1/shows/{id}/metadata/screen` | yes | `library` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/shows/{id}/metadata/screen/retry` | yes | `library` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/shows/{id}/metadata/screen/search` | yes | `library` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `PUT /v1/shows/{id}/metadata/screen/season` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/shows/{id}/metadata/tvdb` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 0 | HTTP handler |
| `PUT /v1/shows/{id}/metadata/tvdb` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `POST /v1/shows/{id}/metadata/tvdb/retry` | yes | `library` | `owner` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |
| `GET /v1/shows/{id}/recommendations` | yes | `read` | `device` | `default` | `page_sized` | `expensive` | 0 | HTTP handler |
| `GET /v1/shows/{id}/seasons` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/storage/mounts` | yes | `admin` | `owner` | `default` | `page_sized` | `default` | 0 | HTTP handler |
| `POST /v1/storage/mounts` | yes | `admin` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `DELETE /v1/storage/mounts/{id}` | yes | `admin` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `POST /v1/storage/mounts/{id}/actions` | yes | `admin` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/storage/operations/{id}` | yes | `admin` | `owner` | `default` | `page_sized` | `default` | 0 | HTTP handler |
| `GET /v1/storage/source-operations/{id}` | yes | `admin` | `owner` | `default` | `page_sized` | `default` | 0 | HTTP handler |
| `GET /v1/storage/sources` | yes | `admin` | `owner` | `default` | `constant` | `default` | 0 | HTTP handler |
| `POST /v1/storage/sources/webdav` | yes | `admin` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `DELETE /v1/storage/sources/webdav/{id}` | yes | `admin` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `PUT /v1/storage/sources/webdav/{id}` | yes | `admin` | `owner` | `default` | `constant` | `default` | 65536 | HTTP handler |
| `GET /v1/suggestions` | yes | `read` | `device` | `default` | `page_sized` | `expensive` | 0 | HTTP handler |
| `GET /v1/system` | yes | `open` | `public` | `default` | `constant` | `default` | 0 | HTTP handler |
| `GET /v1/titles` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | typed/raw |
| `GET /v1/topshelf` | yes | `read` | `device` | `browsing` | `page_sized` | `browsing` | 0 | HTTP handler |
| `GET /v1/topshelf/art/{id}` | yes | `read` | `device` | `media` | `constant` | `media` | 0 | HTTP handler |
| `POST /v1/topshelf/token` | yes | `session` | `device` | `browsing` | `constant` | `browsing` | 65536 | HTTP handler |

Non-API production patterns: `/` serves the optional web app (open/static/default/constant, not registry); `GET /receiver/cast/` serves receiver assets (open/static/default/constant, not registry). `/dlna/` is an infrastructure reservation in the existing admission/scope support, not an API handler added here. Development-only E2E handlers are excluded from production inventory.

# Schema changes

No database/schema changes. No migration or schema sync was needed.

# Owner or viewer settings added or wired

None. Existing owner/viewer settings and their screens are unchanged.

# Needed from others

- Native/TV/Vega shared-package sync must include `packages/contracts` alongside `client-core`, including its Hosted module, because shared clients now import generated builders/types. No native worktree was writable in this lane.
- Identity/Hosted owners must reconcile `ManagedProfilesClient`’s Hosted profile family: its `/v1/account/profiles`/profile-deletion expectations are absent from the current Hosted source/registry. Its public interface was preserved; no fake Hosted operations were published.
- Social/playback owner must reconcile the unused `DeviceCommandsClient` protocol with the actual receiver/handoff API. Its three endpoint families below exist only in fake-server tests, not the production server.
- Development E2E paths need a separate development contract if the programme requires generated builders for development fixtures as well; they must not enter production OpenAPI.
- The personal-state owner must reconcile saved-resource entry mutations: the new hidden-versus-absent probes expose different `current`/conflict results in the retained handler. The registration diff does not change that handler logic.
- Base-health should update the stale expectations identified below. Web/freshness/performance owners retain the non-registry bugs identified in the Bugs section. The private architecture/client documents that describe partial registry coverage or the retired playback/Hosted profile paths need reconciliation after programme merge.

# Test notes for other environments

Verified here: `GOFLAGS=-p=2 go build ./...`; identity, metadata, browse (apart from the two exceptions below), personal state, Live TV/guide, playback/media and administration selections; credential-scope, admission-lane and production-router overlap/construction guards; published OpenAPI coverage, exact regenerated-document comparison, multipart/header/empty-response metadata and named response-schema checks; read restriction matrix (286 classified routes); apikit adapter semantics/scope test; web TypeScript checks; generated-route source guard; contract fixture decoder checks; guide example validation. Contract and client-core TypeScript checks also passed; Hosted sync reproduced 96 contracts. The final contract fixture file (26 tests), profile-membership file (8 tests), event-feed file (7 tests), route-builder/source-guard file (2 tests) and guide examples passed. Selected client identity/metadata/saved/playback/event tests passed (74 and 30 tests in two selections), and administration/library/subtitle/Hosted-connection/download tests passed (144 tests in a selection). These were selected files/packages, not a whole-suite run.

The event fixture was updated with the current server’s `more` field rather than weakening the decoder.

Existing/base/environment failures, not silently treated as passing:

- `TestListeningHTTPPreferenceEnvelopeAndCAS`: requires an explicitly reviewed decoder helper binary.
- `TestBrowseReadinessFailsClosedDuringDerivedRebuild`: expects refusal; current code returns previous content as required by the programme.
- `TestAdministrationTierBoundaries`: an already typed diagnostics/client-log route returns 403 to admin where the fixture expects access.
- `TestAdministrationOwnerBoundaryAndLibrarySettingsRoundTrip`: already typed updates report `unavailable` where the fixture expects `no-feed`.
- `TestAdministrationMissingRevisionHTTP`: an already typed route answers 415 to a request with no Content-Type.
- `TestRestrictedProfileWriteRouteMatrix`: all concrete routes are now classified. Its 438 hidden-versus-absent comparisons expose a retained saved-resource entry difference: an allowed-library hidden title can return a `current` document/conflict whereas its absent twin follows the no-items fast path. Lyrics are now configured in the fixture so unrelated service absence does not mask this check. No authorization or mutation behavior was changed to make it pass.

Cold follow-up: on the merged programme checkout, regenerate from `server/`, run the four scope/lane/router guards and `npx tsc --noEmit` in web, then `node packages/client-core/scripts/check-generated-routes.mjs` and `node server/api/guide/check-examples.mjs`. Verify native shared-package sync/type checks after copying contracts. On a configured server, compare representative legacy HTTP responses before/after for sign-in/profile changes, library scans, owner settings, DVR scheduling, downloads and range/HEAD media reads: statuses, ETags, cookies/credentials, MIME and body bytes should match. Exercise overlapping SSE reconnects: retry a 409 with the last cursor, keep streaming, and receive new events without polling. Use a real reviewed decoder binary for listening/media package acceptance. No device, tuner, GPU, public-network, browser visual or capacity acceptance is claimed here.

# Not finished

- Full removal of client-core path literals is not claimed. The bounded regression allowance contains exactly six existing calls: `GET /v1/me/devices?controllable=true`, `POST /v1/devices/{id}/commands`, `POST /v1/playback/sessions/{id}/transfers` (dormant fake-server protocol), `GET /v1/dev/e2e`, `POST /v1/dev/e2e/sign-in` (development only), and `GET /v1/account/profile-deletions/{id}` (absent Hosted operation). The related Hosted profile suffix family also needs its owner’s API decision. No production server routes were invented for these.
- Native/TV/Vega contract-package sync and platform acceptance remain for the client lanes.
- The non-registry bugs and base/environment test mismatches listed above remain with their owners.
- Final verification is complete. The final cumulative snapshot is `009`; snapshots `001`–`008` preserve the earlier area migrations. No worktree changes follow the final snapshot.

# Merge with the base

Resolved file contents; Git's unmerged index is left for Justin to stage and commit. No snapshot, staging or commit was performed.

- `server/api/README.md`: retained the complete generated-contract instructions and Hosted sync; added the base's prerelease, cursor, revision and SQL-verification qualifications.
- `server/api/guide/01-overview.md`: restored the base's current conventions; replaced obsolete partial-registry/YAML warnings with generated-document guidance.
- `02-authentication.md`: restored current account/profile, refresh, two-factor and TV-code flows; normalized the profile parameter and removed a retired route mention from the active endpoint checker.
- `03-browsing.md`: restored server-composed browsing, grouped sorts/filters and current search guidance.
- `04-playback-v1.md`: restored current session/queue/subtitle guidance, retaining the base's delivery and authorization details; used the actual subtitle parameter name.
- `05-personal-state-and-jobs.md`: restored current personal state, jobs and download guidance, without the removed trash command.
- `06-events.md`: restored stream takeover, cursor/position, resync and polling guidance.
- `08-for-tools.md`: kept the registry operation IDs and validated examples plus the base's Sonarr/Radarr script, path encoding and test-event instructions.
- `09-webhooks.md`: retained signature/delivery guidance and the base's HTTPS example.
- `guide/README.md`: restored links to the updated chapters and base playback-failure/anime-order guidance; the generated JSON remains authoritative.
- `identity.openapi.yaml` and `openapi.yaml`: kept their removal; current identity/networking behavior is represented by regenerated contracts and refreshed guides.
- `backup_routes.go`: retained generated admission metadata and the base's retryable `request_in_progress` error.
- `playback_v1_routes.go`: kept existing admission metadata and registered the base's playback-failure memory. Its two declarations have admin scope/admission; the new client uses generated builders/types.
- `restriction_matrix_test.go`: combined complete route classification with the base's stronger title, report, inbox, device-download and grant probes; kept Lyrics and the base's console/credential fixture wiring; classified the new owner diagnostics read.
- `restriction_writes_test.go`: combined concrete-route coverage with the base's stronger subtitle, feedback and MCP probes. Preserved the hidden-versus-absent saved-list probe; it exposed a real retained handler discrepancy. `saved_resources.go` now rejects hidden/absent additions identically and rechecks live item authority in the write transaction. Both matrices pass.
- `route_scopes.go`: retained only non-API infrastructure. Every merged registry declaration has its required scope; comparison against the base's route scopes found no disagreements. Removed trash/settings and retired HTTPS modes were not reintroduced.

Regenerated `packages/contracts/src/*`, `server/api/portico.openapi.json` (622 operations) and persistence schema/column artifacts using the prescribed generators; none were hand-merged. Schema sync's persistence checks pass. Targeted HTTP, backup-busy, playback-failure, restriction, API-key/token/MCP, apikit revision/adapter and apispec tests pass; vet passes for httpapi, apispec and apikit. Guide validation covers 75 endpoint mentions and three examples; the generated-route guard and new client test pass. Web TypeScript and Linux builds pass. Final Windows amd64 and macOS arm64 cross-builds also pass (`CGO_ENABLED=0`, `GOFLAGS=-p=2`, `go build ./...`). Conflict-marker grep and `git diff --check` are clean. Context-sync remains unable to write its read-only lock; canonical guidance was read directly.

Two additional checks outside the conflict selections still use the retired Hosted-session authority: `TestLibraryDiscoveryAndCollectionPermissions` and `TestPlaylistHTTPShareNeverGrantsMedia`. Their old fixture helpers call `IssueTx(..., "hosted", ...)`, which the identity lane now deliberately refuses; both fail during fixture setup. No production compatibility path or authentication exception was restored. The requested merged-route/restriction/backup/diagnostics selections and the separate API-key/token/MCP selection pass.
