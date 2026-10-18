# Security sweep

## Outcome
Completed the combined-server attacker pass on personal Ubuntu host `ehler-homelab-mini-1`, starting from a clean tree and reviewing changes from `82aaa54` through HEAD. Five new findings were confirmed and fixed: **0 critical, 0 high, 4 medium, 1 low**. Expired cached credentials stop at their deadlines; personal credential scopes no longer grant administrative playback or events; programme pictures enforce current source/rating restrictions; diagnostic records and forwarded standard logs scrub named secrets. All fixes have regression coverage.

The report contains all 622 registry operations, historical bug dispositions, eight review areas, and reproducible test notes. Linux build, Windows amd64/Darwin arm64 cross-builds, touched-package vet, targeted boundary tests, 17 shared guide-parser tests and seven short fuzz runs passed. No full suites, installs, commits, pushes, live-server probes or deployments were performed. Snapshots are cumulative in `_program/snapshots/security-sweep/`.

### Review coverage and evidence

1. **Ways in:** generated every permission row from the registry contract; read the contextual authorizer, handler-owned HTTP registration, scope gate, owner/member authority, Cast target allow-list and special byte-grant handlers. Checked public setup/sign-in/proof/health/connection/MCP rows, all four admin playback operations, credential-management routes, owner settings/secrets, images, subtitles/fonts and download/media grants. Both GET and write restriction matrices and the mandatory-scope guard passed after route changes. The digest image exemption was replaced by a programme-specific rule and concrete serving tests.
2. **Outbound requests:** inspected the complete HTTP-client/call inventory across remote media and DAV, Live TV guide/M3U/HDHomeRun/Xtream, guide/artwork fetches, providers/subtitles, update feed, enrichment, Hosted/claim/certificates and webhooks. Remote media pins/revalidates DNS and approvals, blocks metadata destinations, refuses TLS downgrade/redirect escape and strips credentials on redirects. Artwork uses provider host/path allow-lists, rejects non-public DNS at dial time, revalidates redirect destinations and enforces byte/pixel/concurrency budgets. Fixed-origin provider/Hosted calls disable redirects and have time/response bounds. Update feed explicitly approves the owner's configured origin, bounds bytes/time and cannot redirect outside it. Webhooks deliberately allow owner-chosen LAN/loopback targets, do not follow redirects, disable proxies and bound timeout/response drain; only administrative operations configure them. Enrichment uses owner/runtime configuration plus signed manifests/hashes and decompression limits; this is trusted configuration, not a member-supplied URL. Targeted DNS, redirect, metadata-address, feed and webhook tests passed. No real provider/Hosted network requests were made.
3. **Files and paths:** reviewed static assets' `os.Root`, original-media descriptor/root leases, inventory root identity, subtitle sidecar root confinement and companion-file identity/digest checks, fonts/private objects, download artifacts, guide images, reports, backups/restore and deletion. Media reads retain descriptor custody and reauthorize flowing bytes. Delete pins root and parent handles and refuses changed/non-regular/unpreviewed objects. Backup manifest names are allow-listed; linked database/WAL and traversal entries are refused. State/artifact directories are private and not exposed as arbitrary member paths. Root/link replacement, delete preview, restore link/name and subtitle custody tests passed. Owner-selected roots remain owner-controlled. Local/rootless Docker and OS-specific filesystem behavior still require the environment checks below.
4. **Processes:** enumerated every spawn and inspected central mediaexec/decoder wrappers plus storage/subtitle helpers, rclone/native/guardian, topology and telemetry exceptions. Media command arguments are argv values rather than shell interpolation; media inputs use descriptors/private bridges and protocol/format allow-lists. Central mediaexec builds the sandbox/minimal environment, limits outputs, handles process groups/cancellation and records baseline posture when the sandbox is unavailable. Rclone is an explicit owner-chosen, digest-checked executable with private configuration and constrained argument construction; it is not a member execution facility. Spawn-inventory, minimal-environment and job-validation tests passed. No claim of FFmpeg sandbox enforcement on physical Windows/macOS/GPU hardware is made here.
5. **Untrusted parsing:** read NFO tree/field/depth/entity bounds and legacy charset conversion; XMLTV token, per-record, depth/attribute/node and byte guards; tolerant M3U row handling/channel limits; audio tag/atom/frame bounds; subtitle cue/text/timing bounds and richer asset rejection; image byte/dimension/decode concurrency gates; MCP body/batch/result/argument bounds; report UTF-8, per-account/day/device/global retention bounds. Added fuzz targets for NFO, XMLTV, playlists, tolerant subtitle formats and MCP; used existing tag and naming targets. Seven six-second runs, one worker each, executed **185,762** cases without a crash or invariant failure. These small inputs/runs are smoke coverage, not exhaustive fuzzing.
6. **Sessions/keys/events/MCP:** traced live family/device/member/recovery checks, refresh reuse and receipt replay, signer provenance/inheritance/revocation, cache generation fencing, API credential expiry/revocation and scope management, profile/device/admin outbox audiences, and nested MCP requests through the same router. Fixed deadline caching and two scope crossings. Regression tests prove key scopes cannot raise member roles or mint credentials, another profile's events are withheld, revoked streams discard bytes, stolen refresh predecessors revoke families, and learned signer revocations end issued/derived families while preserving unrelated direct sessions. MCP tool arguments cannot select an arbitrary route or network origin; each call re-enters normal admission and authorization with the caller's credential.
7. **Browser surface:** read CSP (self plus wasm-unsafe-eval and the specific Cast SDK paths; no text eval/inline script permission), base/object/frame/form directives, nosniff/referrer/cache policy, setup JSON/origin/fetch-site gates, trusted proxy origin canonicalization and static serving. Searched server/web/shared code for HTML injection sinks; current static pages do not template member/file text into executable HTML. Same-origin setup and real proxy-header policy tests passed. DNS-rebinding behavior in browsers and physical reverse-proxy acceptance were not exercised; remote first-run ownership and plain HTTP remain explicit choices. Corrected stale local-only/HTTPS-required issuance comments.
8. **Secrets:** inspected client/server record scrubbing, diagnostic bundle settings redaction and aggregate reports, webhook payload construction/log reasons, signed Hosted errors, credential one-time replies and backup/restore scrubbing. The new short-secret regression also verifies the stderr copy. API errors expose registered messages, not raw provider URLs/credentials. Backups intentionally remain plaintext, preserve API keys by policy and scrub/revoke interactive sessions; private directories and bounded receipt retention remain required. Unlabeled arbitrary prose cannot reliably be classified as a secret. Runtime panic output and external service-manager/provider behavior are outside the recorder's guarantees.

### Registry permission inventory

622 operations from the generated registry contract `server/api/portico.openapi.json` (regenerated in this lane). Access totals: public 51, owner 226, admin_scope 31, device 301, viewer_item_restrictions 13.

This is the **effective required policy after fixes**, combining registry role/scope declarations and handler-owned credential gates. The role/scope gates and surprising cases were read in source; targeted route-matrix tests provide runtime coverage, not an exploit proof for every row. `Y` means eligible subject to live authorization and the resource checks; `own` means caller/account/profile ownership; `filtered` means content restrictions and feature limits; `admin` means administrator or owner; `manager` means a managing profile of that same direct account. A single-scope key column remains bounded by its maker's role, grant and profile restrictions. `special` means a download/media grant, pairing code, refresh secret, setup state, signature or challenge proof is required; the public label does not make protected bytes anonymous. `open` means that route does not require a session. Anonymous sign-in operations require valid issuance proofs and applicable admission policy. Paired screens have no key scopes and remain member-tier playback grants.

Actual-versus-intended mismatches found: all four administrator playback routes formerly declared playback scope; the read event feed formerly enabled administrator audiences for an owner-backed read key. The digest-only guide image endpoint also failed current programme/source authorization; it has been replaced by the programme-bound route. Corrected rows are reflected below. No additional mismatch was confirmed by this inventory.

| Route | Access / key scope | Anonymous | Paired screen | Member | Restricted profile | Read key | Playback key | Library key | Admin key | Owner session |
|---|---|---|---|---|---|---|---|---|---|---|
| `GET /health/live` | public / open | open | open | open | open | open | open | open | open | open |
| `GET /health/ready` | public / open | open | open | open | open | open | open | open | open | open |
| `POST /v1/access/invitations/accept` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/access/invitations/preview` | public / session | special | special | special | special | N | N | N | N | special |
| `GET /v1/admin/access-tokens` | owner / session | N | N | N | N | N | N | N | N | Y |
| `DELETE /v1/admin/access-tokens/{id}` | owner / session | N | N | N | N | N | N | N | N | Y |
| `GET /v1/admin/access/devices` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `PUT /v1/admin/access/devices/{id}/trust` | admin_scope / session | N | N | N | N | N | N | N | N | Y |
| `GET /v1/admin/access/invitations` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `POST /v1/admin/access/invitations` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `POST /v1/admin/access/invitations/{id}/revoke` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `GET /v1/admin/access/members` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `GET /v1/admin/access/members/{id}/limits` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `PUT /v1/admin/access/members/{id}/limits` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `PUT /v1/admin/access/members/{id}/role` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `GET /v1/admin/analysis-operations` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/api-keys` | owner / session | N | N | N | N | N | N | N | N | Y |
| `POST /v1/admin/api-keys` | owner / session | N | N | N | N | N | N | N | N | Y |
| `DELETE /v1/admin/api-keys/{id}` | owner / session | N | N | N | N | N | N | N | N | Y |
| `GET /v1/admin/attention` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/backups` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/backups` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/backups/restore` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/admin/backups/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/catalogue/failures` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `GET /v1/admin/connectivity/policy` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PATCH /v1/admin/connectivity/policy` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/connectivity/status` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `GET /v1/admin/console/accounts` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/console/alerts` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `POST /v1/admin/console/alerts/{id}/acknowledge` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/console/diagnostics/capture` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/console/diagnostics/{lane}` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `POST /v1/admin/console/exports` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/console/exports/{id}` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/console/job-kinds` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/console/jobs` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `POST /v1/admin/console/jobs` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `POST /v1/admin/console/jobs/{id}/{action}` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/console/panels/{panel}` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/console/schedules` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `PUT /v1/admin/console/schedules` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/console/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PATCH /v1/admin/console/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/console/stream-options` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/diagnostics/bundle` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/diagnostics/capabilities` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/diagnostics/client-logs` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/admin/diagnostics/client-logs/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/diagnostics/client-logs/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/diagnostics/client-logs/{id}/download` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/diagnostics/concurrency` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/diagnostics/integrity` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/diagnostics/integrity/repair` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/diagnostics/pprof/{profile}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/downloads/revoke` | owner / playback | N | N | N | N | N | owner | N | owner | Y |
| `GET /v1/admin/dvr/recording-groups` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/dvr/recording-groups` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/admin/dvr/recording-groups/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PUT /v1/admin/dvr/recording-groups/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/dvr/recording-owners` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/dvr/recording-permissions` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PUT /v1/admin/dvr/recording-permissions` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/dvr/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PUT /v1/admin/dvr/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/dvr/tuners` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `GET /v1/admin/feedback` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/feedback/reports` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/feedback/reports/{id}` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `POST /v1/admin/feedback/reports/{id}/status` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/feedback/{id}` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `PATCH /v1/admin/feedback/{id}` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/filesystem` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/admin/ingestion/jobs/{id}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/ingestion/jobs/{id}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/admin/ingestion/jobs/{id}/{action}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `DELETE /v1/admin/items/{id}/files/{fileId}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/admin/items/{id}/files/{fileId}/delete/preview` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/jobs` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/libraries` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/admin/libraries/order` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `DELETE /v1/admin/libraries/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/libraries/{id}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PATCH /v1/admin/libraries/{id}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/libraries/{id}/inventory` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/libraries/{id}/inventory-config` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/admin/libraries/{id}/inventory/{object}/{action}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/admin/libraries/{id}/scan-policy` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/libraries/{id}/scans/{job}/issues` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/libraries/{id}/settings` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/admin/libraries/{id}/settings` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/admin/libraries/{id}/shows/{show}/settings` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/admin/libraries/{id}/sources` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `DELETE /v1/admin/libraries/{id}/sources/{source}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PATCH /v1/admin/libraries/{id}/sources/{source}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/libraries/{id}/sources/{source}/changes` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/admin/libraries/{id}/sources/{source}/check` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/admin/libraries/{id}/sources/{source}/scans` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/library-channels` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/library-channels` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/library-channels/block-presets` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/library-channels/block-presets` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/admin/library-channels/block-presets/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PUT /v1/admin/library-channels/block-presets/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/library-channels/criteria` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/library-channels/defaults` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/library-channels/preview` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/library-channels/reorder` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/library-channels/templates` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/library-channels/templates/{id}/install` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/library-channels/{id}/delete` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/library-channels/{id}/health` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/library-channels/{id}/logo` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/library-channels/{id}/regenerate` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/library-settings` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/live-sources` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `POST /v1/admin/live-sources` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `POST /v1/admin/live-sources/discover` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/live-sources/preview` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `POST /v1/admin/live-sources/remote/preview` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `POST /v1/admin/live-sources/remote/save` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `GET /v1/admin/live-sources/{id}/channel-map` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PUT /v1/admin/live-sources/{id}/channel-map` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/live-sources/{id}/configuration` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `PUT /v1/admin/live-sources/{id}/configuration` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `POST /v1/admin/live-sources/{id}/enabled` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/live-sources/{id}/logos` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/live-sources/{id}/refresh` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/live-sources/{id}/remove` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/live/settings` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `PUT /v1/admin/live/settings` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `GET /v1/admin/live/sources/{id}/channels` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/live/sources/{id}/groups` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/logs` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/logs/debug-window` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/logs/events` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/logs/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PATCH /v1/admin/logs/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/maintenance/background-pause` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/maintenance/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PUT /v1/admin/maintenance/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/media/delete` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/admin/media/delete/preview` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/metadata-providers` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/metadata/attention` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/metadata/attention/counts` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/metadata/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PUT /v1/admin/metadata/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/missing-titles` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/admin/missing-titles/empty` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/admin/notifications/broadcast` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/notifications/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PATCH /v1/admin/notifications/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/operations/{panel}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/play-history` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/play-history/summary` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/playback-operations/{operationId}` | owner / playback | N | N | N | N | N | owner | N | owner | Y |
| `GET /v1/admin/playback-sessions` | owner / playback | N | N | N | N | N | owner | N | owner | Y |
| `POST /v1/admin/playback-sessions/{id}/stop` | owner / playback | N | N | N | N | N | owner | N | owner | Y |
| `GET /v1/admin/playback/diagnostics` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/playback/history` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/admin/playback/history.csv` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/playback/route-failures` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/admin/playback/route-failures/{sourceId}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/resource-usage` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/restart` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/sessions` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `GET /v1/admin/sessions/{id}/controls` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `POST /v1/admin/sessions/{id}/controls` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `POST /v1/admin/sessions/{id}:terminate` | admin_scope / admin | N | N | N | N | N | N | N | admin | Y |
| `POST /v1/admin/state-permissions:fix` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/storage-usage` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/storage-usage/{category}/cleanup` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/storage/status` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/admin/storage/upgrade-copies/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/support-report` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/telemetry` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/telemetry/now` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/titles/merge` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/admin/transcode/capacity` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/updates` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/webhooks` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/webhooks` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/admin/webhooks/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PATCH /v1/admin/webhooks/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/admin/webhooks/{id}/deliveries` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/admin/webhooks/{id}/test` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/albums/{id}/metadata/musicbrainz` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/albums/{id}/metadata/musicbrainz` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/albums/{id}/metadata/musicbrainz/policy` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/albums/{id}/metadata/musicbrainz/retry` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/albums/{id}/metadata/musicbrainz/search` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/albums/{id}/songs` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/artists/{id}/albums` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/attribution` | public / open | open | open | open | open | open | open | open | open | open |
| `GET /v1/auth/capabilities` | public / open | open | open | open | open | open | open | open | open | open |
| `POST /v1/auth/refresh` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/auth/register` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/auth/remembered-accounts` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/auth/two-factor/challenge` | public / session | special | special | special | special | N | N | N | N | special |
| `GET /v1/books/{id}/chapters` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/books/{id}/files` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/bootstrap` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/capabilities` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/cast/bootstrap` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/cast/bootstrap/{id}` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/cast/configuration` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/cast/configuration` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/cast/devices/{id}` | device / session | N | own | own | filtered | N | N | N | N | Y |
| `POST /v1/cast/reconnect` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/cast/redeem` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/channels/preferences` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/collections/{id}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/collections/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PATCH /v1/collections/{id}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/collections/{id}/items` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `DELETE /v1/collections/{id}/items/{itemId}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/collections/{id}/items/{itemId}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/collections/{id}/items:batch` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/console/registry` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/containers/{kind}/{id}/personal-state` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/containers/{kind}/{id}/personal-state` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/containers/{kind}/{id}/personal-state/undo` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/content` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/devices` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/devices` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `DELETE /v1/devices/{id}` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `PATCH /v1/devices/{id}` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/devices/{id}/approval` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `DELETE /v1/devices/{id}/sessions` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/devices/{id}/sessions/bind` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/diagnostics/client-logs` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/direct` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/direct/members` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/direct/members` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `DELETE /v1/direct/members/{id}` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `PATCH /v1/direct/members/{id}` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/direct/members/{id}/deletion-preview` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/direct/ownership` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/direct/ownership/custody` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/direct/password` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/direct/pin-recovery` | admin_scope / session | N | N | manager | manager | N | N | N | N | Y |
| `POST /v1/direct/portico-challenge` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/direct/profile-switch` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/direct/profiles` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/direct/profiles/avatars` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/direct/profiles/order` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `DELETE /v1/direct/profiles/{id}` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `PATCH /v1/direct/profiles/{id}` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `DELETE /v1/direct/profiles/{id}/avatar` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/direct/profiles/{id}/avatar` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/direct/profiles/{id}/deletion-preview` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/direct/profiles/{id}/pin-reset` | admin_scope / session | N | N | manager | manager | N | N | N | N | Y |
| `GET /v1/direct/profiles/{id}/restrictions` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/direct/profiles/{id}/restrictions` | admin_scope / admin | N | N | manager | manager | N | N | N | manager | Y |
| `POST /v1/direct/profiles/{id}/select` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/direct/registration-policy` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PUT /v1/direct/registration-policy` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/direct/remembered-accounts` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `DELETE /v1/direct/remembered-accounts/{accountId}` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/direct/sessions` | admin_scope / session | N | N | manager | manager | N | N | N | N | Y |
| `POST /v1/direct/sessions/sign-out-everywhere` | admin_scope / session | N | N | manager | manager | N | N | N | N | Y |
| `DELETE /v1/direct/sessions/{id}` | admin_scope / session | N | N | manager | manager | N | N | N | N | Y |
| `POST /v1/direct/sign-in` | public / session | special | special | special | special | N | N | N | N | special |
| `DELETE /v1/direct/trust` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `DELETE /v1/direct/two-factor` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/direct/two-factor` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/direct/two-factor/enrol` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/direct/two-factor/verify` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/downloads/artifacts/{grant}` | public / open | special | special | special | special | special | special | special | special | special |
| `HEAD /v1/downloads/artifacts/{grant}` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/downloads/artifacts/{grant}/artwork/{role}` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/downloads/artifacts/{grant}/subtitles/{resource}/{revision}.vtt` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/downloads/artifacts/{grant}/subtitles/{resource}/{revision}/{file}` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/downloads/preparations` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/downloads/preparations` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/downloads/preparations/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/downloads/preparations/{id}/actions` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/downloads/preparations/{id}/bundle` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/downloads/preparations/{id}/grant` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/downloads/progress` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/downloads/receipt-keys` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/downloads/receipts` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/downloads/receipts/revalidate` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/downloads/receipts/revocations` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/downloads/receipts/revoke` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/downloads/requests` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/downloads/requests` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/downloads/requests/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/downloads/settings` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/downloads/settings` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/downloads/usage` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/dvr` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/dvr/channels` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/dvr/recordings` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/dvr/recordings/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/dvr/recordings/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PATCH /v1/dvr/recordings/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/dvr/recordings/{id}/cancel` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/dvr/recordings/{id}/delete-preview` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/dvr/recordings/{id}/keep` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/dvr/rules/preview` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/dvr/rules/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `PUT /v1/dvr/rules/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/dvr/storage` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/dvr/storage` | device / admin | N | N | own | filtered | N | N | N | filtered | Y |
| `GET /v1/events` | device / read | N | N | own | filtered | viewer | N | N | owner + viewer | Y |
| `GET /v1/feedback` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/feedback` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/feedback/capabilities` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/feedback/reports` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/feedback/reports` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/feedback/reports/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/feedback/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/groups` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/groups` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/groups/join` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/groups/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/groups/{id}/end` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/groups/{id}/events` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/groups/{id}/heartbeat` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/groups/{id}/host-transfer` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/groups/{id}/invites` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/groups/{id}/invites/{inviteId}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/groups/{id}/leave` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/groups/{id}/queue` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/groups/{id}/queue` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/groups/{id}/readiness` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/groups/{id}/settings` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/groups/{id}/transport` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/guide` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/guide/channels` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/guide/programme-images/{generation}/{programme}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/guide/sources` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/handoffs` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/handoffs/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/handoffs/{id}/commit` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/handoffs/{id}/readiness` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/handoffs/{id}/rollback` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/home` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/home/layout` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/home/layout` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/home/layout/reset` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/home/rows/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/hosted/wake` | public / session | special | special | special | special | N | N | N | N | special |
| `GET /v1/items` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/items/personal-state:batch` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/items/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/analysis` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/analysis/artifacts/{artifact}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/items/{id}/analysis/jobs` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `POST /v1/items/{id}/analysis/jobs/{job}/control` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `POST /v1/items/{id}/analysis/markers` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `PUT /v1/items/{id}/analysis/policy` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `GET /v1/items/{id}/art/{kind}` | device / read | N | own | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/chapters/{chapter}/image` | device / read | N | own | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/credits` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/items/{id}/delete` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `POST /v1/items/{id}/delete/preview` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `GET /v1/items/{id}/detail` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/listening` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/lyrics` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/items/{id}/lyrics` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `POST /v1/items/{id}/lyrics/search` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `PUT /v1/items/{id}/lyrics/selection` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/items/{id}/lyrics/{resource}` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `PATCH /v1/items/{id}/lyrics/{resource}/offset` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `GET /v1/items/{id}/lyrics/{resource}/revisions/{revision}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/items/{id}/metadata/local-audio/policy` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/items/{id}/metadata/manual` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PATCH /v1/items/{id}/metadata/manual` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/items/{id}/metadata/musicbrainz` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/items/{id}/metadata/musicbrainz` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/items/{id}/metadata/musicbrainz/policy` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/items/{id}/metadata/musicbrainz/retry` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/items/{id}/metadata/musicbrainz/search` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/items/{id}/metadata/refresh` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/items/{id}/metadata/screen` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/items/{id}/metadata/screen` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/items/{id}/metadata/screen/retry` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/items/{id}/metadata/screen/search` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/items/{id}/optimizations` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `PUT /v1/items/{id}/personal-state` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/items/{id}/playback-offers` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/prepared-versions` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/recommendations` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/source-availability` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/trickplay` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/trickplay/{set}/thumbnails.vtt` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{id}/trickplay/{set}/tiles/{tile}.jpg` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{itemId}/download-options` | viewer_item_restrictions / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{itemId}/playback-options` | viewer_item_restrictions / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/items/{itemId}/playback/{session}/subtitles` | viewer_item_restrictions / read | N | own | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/items/{itemId}/playback/{session}/subtitles` | viewer_item_restrictions / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/items/{itemId}/subtitles` | viewer_item_restrictions / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/items/{itemId}/subtitles` | viewer_item_restrictions / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `POST /v1/items/{itemId}/subtitles/apply` | viewer_item_restrictions / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `POST /v1/items/{itemId}/subtitles/import` | viewer_item_restrictions / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `POST /v1/items/{itemId}/subtitles/refresh` | viewer_item_restrictions / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `POST /v1/items/{itemId}/subtitles/remote` | viewer_item_restrictions / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `POST /v1/items/{itemId}/subtitles/search` | viewer_item_restrictions / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `DELETE /v1/items/{itemId}/subtitles/{resource}` | viewer_item_restrictions / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `PUT /v1/items/{itemId}/subtitles/{resource}` | viewer_item_restrictions / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `POST /v1/jobs` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/jobs/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/jobs/{id}/failures` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/libraries` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/libraries/paths-changed` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/libraries/{id}/artists` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries/{id}/books` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries/{id}/browse` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/libraries/{id}/browse` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries/{id}/browse-capabilities` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries/{id}/categories` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries/{id}/collections` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/libraries/{id}/collections` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/libraries/{id}/content` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries/{id}/discover` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/libraries/{id}/episode-assignments` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/libraries/{id}/episode-issues` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/libraries/{id}/facets` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries/{id}/inventory-status` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries/{id}/listening/selection` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/libraries/{id}/lyrics/fetch` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/libraries/{id}/metadata/agent` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `PUT /v1/libraries/{id}/metadata/agent` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `GET /v1/libraries/{id}/metadata/attention` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/libraries/{id}/metadata/attention` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/libraries/{id}/metadata/screen` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/libraries/{id}/metadata/screen` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/libraries/{id}/network-roots` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/libraries/{id}/network-roots` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/libraries/{id}/scans` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/libraries/{id}/show-workspace` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries/{id}/shows` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/libraries/{id}/strm-analysis-policy` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/libraries/{id}/strm-analysis-policy` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/library-channels/preferences` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/library-channels/{id}/logo` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/library-kinds/{kind}/metadata-agents` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/listening/preferences` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/listening/preferences` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `PUT /v1/lyrics/provider` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/mcp` | public / open | open | open | open | open | open | open | open | open | open |
| `POST /v1/mcp` | public / open | N | N | key only | key only | scoped tools | scoped tools | scoped tools | scoped tools | key only |
| `GET /v1/me` | device / read | N | own | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/me/access-tokens` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/me/access-tokens` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `DELETE /v1/me/access-tokens/{id}` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/me/api-access` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `GET /v1/me/devices/current/capabilities` | device / read | N | own | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/me/devices/current/capabilities` | device / playback | N | own | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/me/library-navigation` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/me/library-navigation` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/me/recommendations:reset` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/media/linear/{playbackId}/{generation}/{token}/{name}` | device / playback | special | special | special | special | N | special | N | special | special |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}.vtt` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/fonts` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/fonts/{font}` | public / open | special | special | special | special | special | special | special | special | special |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/fonts/{font}` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.ass` | public / open | special | special | special | special | special | special | special | special | special |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.ass` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.idx` | public / open | special | special | special | special | special | special | special | special | special |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.idx` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.mks` | public / open | special | special | special | special | special | special | special | special | special |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.mks` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.ssa` | public / open | special | special | special | special | special | special | special | special | special |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.ssa` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.sub` | public / open | special | special | special | special | special | special | special | special | special |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.sub` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/media/subtitles/{grant}/{resource}/{revision}/original.sup` | public / open | special | special | special | special | special | special | special | special | special |
| `HEAD /v1/media/subtitles/{grant}/{resource}/{revision}/original.sup` | public / open | special | special | special | special | special | special | special | special | special |
| `GET /v1/media/{grant}` | device / playback | special | special | special | special | N | special | N | special | special |
| `GET /v1/media/{grant}/audio` | device / playback | special | special | special | special | N | special | N | special | special |
| `HEAD /v1/media/{grant}/audio` | device / playback | special | special | special | special | N | special | N | special | special |
| `GET /v1/media/{grant}/audio-converted` | device / playback | special | special | special | special | N | special | N | special | special |
| `GET /v1/media/{grant}/{file}` | device / playback | special | special | special | special | N | special | N | special | special |
| `POST /v1/metadata/bulk` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/metadata/{kind}/{id}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/metadata/{kind}/{id}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/metadata/{kind}/{id}/art/{role}` | owner / read | N | N | N | N | owner | N | N | owner | Y |
| `POST /v1/metadata/{kind}/{id}/art/{role}/upload` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `DELETE /v1/metadata/{kind}/{id}/art/{role}/upload/{candidateId}` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/metadata/{kind}/{id}/preview` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/networking/certificate` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/networking/certificate/config` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/networking/certificate/retry` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/networking/claim` | owner / session | N | N | N | N | N | N | N | N | Y |
| `POST /v1/networking/claim/approve` | owner / session | N | N | N | N | N | N | N | N | Y |
| `POST /v1/networking/claim/await-approval` | owner / session | N | N | N | N | N | N | N | N | Y |
| `POST /v1/networking/claim/cancel` | owner / session | N | N | N | N | N | N | N | N | Y |
| `POST /v1/networking/claim/continue` | owner / session | N | N | N | N | N | N | N | N | Y |
| `POST /v1/networking/claim/endpoint` | owner / session | N | N | N | N | N | N | N | N | Y |
| `POST /v1/networking/claim/prepare` | owner / session | N | N | N | N | N | N | N | N | Y |
| `POST /v1/networking/claim/web-approval` | owner / session | N | N | N | N | N | N | N | N | Y |
| `GET /v1/networking/connection` | public / open | open | open | open | open | open | open | open | open | open |
| `POST /v1/networking/endpoint-proof` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/networking/identity-proof` | public / session | special | special | special | special | N | N | N | N | special |
| `GET /v1/networking/remote` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/networking/remote/check` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/networking/remote/config` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/notifications` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/notifications/capabilities` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/notifications/inbox` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/notifications/inbox/actions` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/notifications/unread-count` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/notifications/{id}/read` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/optimization-jobs/{id}/cancel` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/optimization-jobs/{id}/retry` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/people` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/people/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/people/{id}/portrait` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/personal-history` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/personal-history/actions` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/playback/controls` | device / playback | N | own | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/playback/delivery-policy` | device / read | N | own | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/playback/sessions` | device / playback | N | own | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/playback/sessions/{id}` | device / playback | N | own | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/playback/sessions/{id}` | device / read | N | own | own | filtered | filtered | N | N | filtered | Y |
| `PATCH /v1/playback/sessions/{id}` | device / playback | N | own | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/playback/sessions/{id}/chapters` | device / read | N | own | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/playback/sessions/{id}/timeline` | device / playback | N | own | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/playlists` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/playlists/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/playlists/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PATCH /v1/playlists/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/playlists/{id}/content` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/playlists/{id}/entries` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/playlists/{id}/entries/{entryId}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/playlists/{id}/order` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/playlists/{id}/order` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/playlists/{id}/share-candidates` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `DELETE /v1/playlists/{id}/shares` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `PUT /v1/playlists/{id}/shares` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/preferences` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PATCH /v1/preferences` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/prepared-versions/{id}/delete` | device / library | N | N | own | filtered | N | N | filtered | filtered | Y |
| `GET /v1/profiles/{id}/avatar` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/queues` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/queues/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/queues/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PATCH /v1/queues/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/queues/{id}/entries` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `DELETE /v1/queues/{id}/entries/{entryId}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/queues/{id}/entries/{entryId}:move` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/queues/{id}/segments` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/queues/{id}/window` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/queues/{id}:advance` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/queues/{id}:commit-next` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/queues/{id}:prepare-next` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/queues/{id}:save-as-playlist` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/quick-connect` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/quick-connect/cancel` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/quick-connect/decision` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/quick-connect/review` | device / session | N | N | own | filtered | N | N | N | N | Y |
| `POST /v1/quick-connect/token` | public / session | special | special | special | special | N | N | N | N | special |
| `GET /v1/rating-systems` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/readiness` | public / open | open | open | open | open | open | open | open | open | open |
| `GET /v1/receivers` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/receivers` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/receivers/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/receivers/{id}/grants` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/receivers/{id}/grants` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/receivers/{id}/grants/{grantId}/decision` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `POST /v1/receivers/{id}/heartbeat` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/receivers/{id}/inbox` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/receivers/{id}/inbox/wait` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/saved-pins/order` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/saved-pins/order` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `PUT /v1/saved-pins/{kind}/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/saved-resources` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/saved-resources` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `DELETE /v1/saved-resources/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/saved-resources/{id}` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PATCH /v1/saved-resources/{id}` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/saved-resources/{id}/content` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `PUT /v1/saved-resources/{id}/entries` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/saved-resources/{id}/share-candidates` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `DELETE /v1/saved-resources/{id}/shares` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `PUT /v1/saved-resources/{id}/shares` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/search` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `DELETE /v1/search/history` | device / playback | N | N | own | filtered | N | filtered | N | filtered | Y |
| `GET /v1/search/history` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/seasons/{id}/episodes` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/server` | public / open | open | open | open | open | open | open | open | open | open |
| `DELETE /v1/sessions/current` | device / session | N | own | own | filtered | N | N | N | N | Y |
| `POST /v1/setup` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/setup/browser` | public / session | special | special | special | special | N | N | N | N | special |
| `POST /v1/setup/finish` | owner / session | N | N | N | N | N | N | N | N | Y |
| `POST /v1/setup/remote-recovery` | owner / session | N | N | N | N | N | N | N | N | Y |
| `POST /v1/setup/resume` | public / session | special | special | special | special | N | N | N | N | special |
| `GET /v1/setup/state` | owner / session | N | N | N | N | N | N | N | N | Y |
| `GET /v1/shows/{id}/episodes` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `POST /v1/shows/{id}/metadata/refresh` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/shows/{id}/metadata/screen` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/shows/{id}/metadata/screen` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/shows/{id}/metadata/screen/retry` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/shows/{id}/metadata/screen/search` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/shows/{id}/metadata/screen/season` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/shows/{id}/metadata/tvdb` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `PUT /v1/shows/{id}/metadata/tvdb` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `POST /v1/shows/{id}/metadata/tvdb/retry` | owner / library | N | N | N | N | N | N | owner | owner | Y |
| `GET /v1/shows/{id}/recommendations` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/shows/{id}/seasons` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/storage/mounts` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/storage/mounts` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/storage/mounts/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/storage/mounts/{id}/actions` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/storage/operations/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/storage/source-operations/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/storage/sources` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `POST /v1/storage/sources/webdav` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `DELETE /v1/storage/sources/webdav/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `PUT /v1/storage/sources/webdav/{id}` | owner / admin | N | N | N | N | N | N | N | owner | Y |
| `GET /v1/suggestions` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/system` | public / open | open | open | open | open | open | open | open | open | open |
| `GET /v1/titles` | device / read | N | N | own | filtered | filtered | N | N | filtered | Y |
| `GET /v1/topshelf` | device / read | special | special | special | special | special | N | N | special | special |
| `GET /v1/topshelf/art/{id}` | device / read | special | special | special | special | special | N | N | special | special |
| `POST /v1/topshelf/token` | device / session | N | N | own | filtered | N | N | N | N | Y |

## Decisions I made
- Preserve the programme's deliberate security trade-offs, including plain HTTP, owner-configured network destinations, no account lockout, plaintext persistence and TV role inheritance.
- Context refresh was attempted but its lock file is read-only in this sandbox. Existing canonical project, standards, verification and attribution guidance was read.
- Source inspection establishes wiring and attack paths; regression execution establishes the tested boundary. Neither is a production penetration test or device acceptance.
- An owner's **playback** credential controls its own playback. Admin scope is required for other members' sessions and the administrative outbox, even when the underlying member is owner.
- A content digest identifies stored bytes, not permission to retrieve them. Live guide pictures therefore carry programme identity and use current source/rating checks. A superseded generation's image URL is withdrawn.
- Guide images use no-store so a one-year immutable browser cache cannot continue serving a withdrawn image. Internal storage still deduplicates by content digest.
- MCP personal playback and member administration are separate tools so narrow credentials retain useful personal controls.
- Do not add new dependencies, transport enforcement, account lockouts or restrictions on explicitly owner-configured webhook targets.

## Bugs
### Credential caches outlive token expiry — Medium — fixed
- Files: `server/internal/httpapi/principal_cache.go:93`, `server/internal/httpapi/api_gate.go:185`, `server/internal/identity/families.go:202`, `server/internal/access/credentials.go:371`.
- Attack: warm a browse/authentication cache immediately before a session or API credential expires, then keep using it during the remainder of the 15-second TTL. Expiry passage does not write a database table and therefore does not advance the authority generation.
- Evidence: both cache insertion paths formerly used only `now + TTL`; live family authentication checks token and family deadlines. Deterministic tests prove admission immediately before the deadline and rejection at it, including a principal obtained through real family authentication.
- Fix: carry the earlier verified session/family deadline and credential expiry into cache entries and cap their TTLs. Revocation generation fences remain intact.
- Validation: targeted HTTP cache tests passed; `GOFLAGS=-p=2 go build ./...` passed.

### Playback scope reaches other members — Medium — fixed
- Files: `server/internal/httpapi/playback_v1_routes.go:161`, `server/internal/httpapi/playback_controls.go:15`, `server/internal/httpapi/mcp.go:406`.
- Attack: obtain the owner's key limited to playback, then enumerate all viewers' sessions, pause or seek another viewer, or terminate them. Role authorization succeeds because the underlying member is owner; four administrative operations incorrectly required only playback scope.
- Evidence: the registry and handler call chain admit that key to administrator-only services. Regression requests against all four operations now return `403 insufficient_scope`; MCP lists and calls are also fenced.
- Fix: require admin scope for session administration. Keep `control_playback` for personal playback and add `control_member_playback` for administration, so narrowing scope does not remove personal control.

### Read scope receives administrative events — Medium — fixed
- File: `server/internal/httpapi/events_routes.go:44`.
- Attack: an owner's read-only key calls the general viewer event feed and receives the admin outbox, despite being refused the administrative resource routes.
- Evidence: event authority formerly depended only on the underlying owner role. A regression seeds private admin and public server events; the read key now receives only the public event while an admin key and an owner session receive both.
- Fix: require admin scope before enabling owner-only event audiences. The route remains read scoped for viewer events.
- Validation for both scope fixes: targeted scope, MCP, administrator-control and expiry tests passed; server build passed; API generator run.

### Diagnostic secrets survive scrubbing and stderr forwarding — Low — fixed
- File: `server/internal/servicelog/clientlogs.go:29`, `server/internal/servicelog/servicelog.go:227`.
- Attack/exposure: a client or provider error includes a short password, TOTP seed, API key, URL userinfo, cookie, or OAuth-style token parameter. The previous scrubber removed bearer tokens, four query names and 40-character strings, leaving these values in persisted reports and server message records. The recorder also forwarded raw bytes to stderr before scrubbing its own copy.
- Evidence: adversarial examples in `secret_values_test.go` include a 15-character password, 15-character factor seed, short provider key, eight-digit device code and escaped JSON values. They are now redacted; normal title/status/error-code diagnostics remain unchanged.
- Fix: scrub named JSON/key-value secrets, credential headers, URL userinfo and additional credential query names before the existing length heuristic. Scrub the separately forwarded stderr copy as well. This does not claim arbitrary unlabeled text can be recognized as a secret.
- Validation: focused scrubbing, stderr forwarding, standard log and file-retention tests passed; server build passed.

### Known image hash bypasses programme restrictions — Medium — fixed
- Files: `server/internal/httpapi/guide_images.go:16`, `server/internal/livechannels/store.go:773`, `packages/client-core/src/channel-guide.ts:99`.
- Attack: a restricted member knows a programme image digest (from an earlier unrestricted session or the public provider bytes) and requests its image directly. The old handler verified only the session and digest, without source policy or programme restrictions. Removing the image URL from the guide did not authorize image delivery.
- Evidence: the previous digest route called `GuideImageBytes` after a generic principal check. Regression cases now cover anonymous access, unknown records, a member denied an owner-only source, an unrestricted member allowed a shared source, that same URL denied after the member's rating ceiling is narrowed (including If-None-Match), and a superseded generation denied even to the owner.
- Fix: replace the digest endpoint with `GET /v1/guide/programme-images/{generation}/{programme}`. Authorize the live session, current active source and programme rating before reading bytes, and recheck before delivery. Disable immutable browser caching. Keep digest identity only in private storage. Publish the new path from the guide and admit it in the shared client parser while continuing to reject external/traversal/retired paths.
- Validation: guide-serving, route scope, GET/write restriction matrices and programme icon projection tests passed; server build passed. Shared guide parser: 17 tests passed; generated-contract TypeScript check passed.

### Historical audit dispositions

These are prior findings, not additional new-severity counts. Source evidence above and the referenced current seams were used; runtime acceptance limitations are retained rather than inferred from a green build.

- **B-01.1** — already fixed — schedules refuse temporarily through admission without retiring the family.
- **B-01.2** — already fixed — duplicate of B-15.1.
- **B-01.3** — already fixed — scoped keys authenticate; concrete scope/role/non-escalation tests passed.
- **B-01.4** — already fixed — playback/session checks use the verified identity device and its live approval state; no client-selected ID bypass.
- **B-01.5** — already fixed — duplicate of B-15.2.
- **B-01.6** — already fixed — TV approval/pending issuance follows current device policy; targeted signer/TV tests passed.
- **B-01.7** — already fixed in server — member device queue/trust routes use current authority and keep the owner reachable. Physical screen/recovery acceptance belongs to clients.
- **B-01.8** — already fixed — `Identity.Hosted()` calls the installed current-claim readiness seam, wired by `networking_initializer.go`.
- **B-01.9** — already fixed for nonterminal 403 — shared refresh retains credentials. A bare 401 remains terminal; a gateway falsely returning 401 needs deployment acceptance.
- **B-01.10** — already fixed — account limits narrow profile content restrictions through the common rating-age semantics.
- **B-01.11** — already fixed in classification — rating-age aliases and unknown-as-unrated rules are shared. Provider spelling coverage is not exhaustive.
- **B-01.12** — not fixed as an owner audit-retention change — V01 classifies retained owner play history as a deliberate policy trade-off. Private profile data and runtime grants use exact erasure boundaries.
- **B-01.13** — already fixed — invitation redeemer is wired into self-registration in `cmd/server/administration.go`.
- **B-01.14** — already fixed — duplicate locality rule consolidated with B-15.6/11.
- **B-01.15** — already fixed — duplicate of bounded hash admission B-15.10.
- **B-01.16** — already fixed — PIN hashing/checking occurs outside the database writer with bounded hash admission.
- **B-01.17** — not present as sole authentication — installation ids are metadata/binding, not a credential; refresh also requires its secret. Pre-sign-in remembered-account convenience remains intentional.
- **B-01.18** — already fixed — ended session/history pruning is bounded and scheduled by setup maintenance.
- **B-01.19** — already fixed — family revocation is centralized with runtime hooks and authority wake/generation fencing; stream revocation test passed.
- **B-01.20** — already fixed — valid role/restriction refusals use 403 or hidden 404; unauthenticated/revoked remains 401. Route matrices passed.
- **B-01.21** — already fixed — selected child/viewer profiles cannot administer accounts or raise role/scopes; Cast/secondary-owner-profile tests passed.
- **B-01.22** — already fixed for production authority — server-native membership and typed retained protocols replace retired production authority paths; development-only trust fixtures remain separate.
- **B-01.23** — not fixed — historical Apple explicit_signout incident has no runtime reproduction/source-backed cause here; no speculative session change made.
- **B-08.1** — already fixed — duplicate trusted-proxy origin B-15.8.
- **B-08.2** — already fixed — unknown server failures map to logged 500; request-input errors are named.
- **B-08.3** — already fixed — a replacement SSE stream retires its predecessor and preserves continuation.
- **B-08.4** — already fixed — SSE sends an initial cursor and stream position.
- **B-08.5** — already fixed — capabilities publish the same 25-second long-poll ceiling the handler accepts.
- **B-08.6** — already fixed — subscriptions wake for relevant profile/device/admin/server audiences and library hints; reads enforce visibility.
- **B-08.7** — already fixed — duplicate canonical proxy limits B-15.3/6.
- **B-08.8** — already fixed — separate revisions/viewer fences and scoped validators replaced catch-all invalidation. Performance behavior is outside this security pass.
- **B-08.9** — not a confirmed security defect — mandatory response/registry validation remains fail-closed; malformed rows are handled by domain parsers and matrix coverage.
- **B-08.10** — already fixed — shared error normalization understands registry retry semantics; no new retry policy introduced.
- **B-08.11** — already fixed — all 622 operations are described by the live typed registry and generated contract.
- **B-08.12** — already fixed for authorization — registry access precedes conditional responses; output/header safeguards remain. Guide images now reauthorize even conditional requests and do not use immutable caching.
- **B-08.13** — fixed in this lane for the concrete scope mismatches above — handler-owned byte grants are explicitly distinguished from labels in the inventory; access labels alone are not an enforcement proof.
- **B-08.14** — not a security defect — revision-fenced enumeration continues to reject a changed view. Stable export under continuous change is separate work, not permission to weaken the fence.
- **B-08.15** — already fixed — originals publish their actual container MIME type; retained by this lane.
- **B-08.16** — already fixed — duplicate build-fingerprint B-15.15.
- **B-08.17** — already improved — separate admission/reserves and credential stream concurrency prevent one script using the whole realtime pool; no capacity benchmark claim.
- **B-08.18** — already fixed — uploads use their bounded upload lane and progress deadlines rather than a universal 15-second wall-clock cutoff.
- **B-15.1** — already fixed — issuance/renewal admission is centralized in identity and `withSessionAdmission`; remote-policy regression passed.
- **B-15.2** — already fixed — nullable uncollected approvals expire and cleanup continues; `PruneSetup` has bounded batches and the worker safety tick.
- **B-15.3** — already fixed — setup subjects and rate limits use canonical trusted-proxy client addresses.
- **B-15.4** — not fixed as an ownership policy change — remote first-run setup is intentional. JSON/origin/fetch-site gates are present; arbitrary-host DNS rebinding remains an accepted fresh-install risk, not runtime-reproduced here. Removed the stale local-only comment.
- **B-15.5** — not fixed by encryption — plaintext at rest is required. Current backups scrub interactive sign-ins; restore ends them; API keys intentionally survive. Seeds/configuration remain sensitive private state.
- **B-15.6** — already fixed — networking policy parses proxy chains consistently for recovery, limits, locality and origin.
- **B-15.7** — not fixed by device cryptography — credentials remain bearer/family/device bound, not proof-of-possession keys. Private architecture/native comments need correction outside this worktree.
- **B-15.8** — already fixed — canonical proxy origin and preserved-host HTTPS support; proxy/origin tests passed.
- **B-15.9** — not fixed by lockout — preserve the explicit no-lockout decision and bounded challenge/source pacing.
- **B-15.10** — already fixed — hashing slots now wait cancellably for up to three seconds, without holding the writer (`identity/service.go:122`). Capacity under distributed load is not established.
- **B-15.11** — already fixed in policy — the owner home networks are the single locality rule, with trusted proxy handling and observed client addresses. An address-hiding tunnel must be represented by owner policy; no detection is added.
- **B-15.12** — already fixed for learned revocations — check-ins learn signed current certificates and atomically revoke signer-derived families. Before a disconnected server learns a revocation, trust risk persists; certificate cadence/Hosted incident acceptance is external work.
- **B-15.13** — not fixed — TV-code approval deliberately confers the approver’s role; device proof and family provenance are retained.
- **B-15.14** — already fixed for URL exposure — remembered-account lookup is POST with the installation id in the body. Anonymous convenience lookup remains the intended pre-sign-in behavior.
- **B-15.15** — already fixed for build fingerprint — public system data no longer includes build id/source digest/time. Semantic version remains intentional pre-sign-in capability information.
- **B-15.16** — already fixed — CSP allows the specific Cast sender paths and self form action; wasm-unsafe-eval does not permit text eval.
- **B-15.17** — already fixed — update feed uses approved-origin remote-media policy and byte/time bounds; escape/metadata regression passed.
- **B-15.18** — already fixed — resolved folder paths are rechecked; symlink picker regression passed.
- **B-15.19** — already fixed for action pins, Docker digests and dependency checks. OS installer signing remains unfinished outside server/shared-client scope; release text explicitly says packages are unsigned and automatic install is disabled.
- **B-15.20** — already fixed for overrides — metadata settings have owner TMDB/TVDB credential overrides and configured flags. Embedded defaults remain the deliberate fleet dependency.
- **B-15.21** — already fixed — PIN failure debt also counts across devices; targeted cross-device regression passed.
- **B-15.22** — not present as the old enforcement contract — plain HTTP is accepted by programme decision; no required-TLS exception rule is introduced. Removed stale issuance commentary.
- **B-15.23** — already fixed — code creation/review/decision and invitation prefixes enter the cheap limiter before SQL.
- **B-15.24** — already fixed — download disposition uses the escaping helper and history CSV neutralizes spreadsheet formulas.
- **N-V01.1** — already fixed: verified signer ids are recorded and inherited by Portico-admitted families; learned revocations end existing and derived families atomically. Hosted check-in, signer/TV, pending-MFA and pending-approval regressions passed.

## API changes
- `GET /v1/admin/sessions`, `GET/POST /v1/admin/sessions/{id}/controls`, `POST /v1/admin/sessions/{id}:terminate`: credential scope changes from playback to admin. Session role rules and response shapes unchanged. Generated OpenAPI regenerated; shared clients use these same routes without a shape change.
- `GET /v1/events`: same wire shape and read scope; credentials without admin scope no longer receive owner-only events.
- MCP `control_playback` controls the caller's own playback. New `control_member_playback` handles administrative controls and requires admin scope plus administrator role. No MCP route removed.
- Removed `GET /v1/guide/images/{digest}`. Added `GET /v1/guide/programme-images/{generation}/{programme}` with the same binary response and read scope. `Programme.image` now uses the programme-bound URL, and shared client parsing accepts it. Image responses now use `private, no-store` instead of a one-year immutable cache. Generated contracts regenerated.

## Schema changes
None.

## Owner or viewer settings added or wired
None. Existing credential scopes and source/profile restriction settings now enforce their stated boundaries on the affected paths.

## Needed from others
- Client lanes: synchronize the updated `packages/client-core` and generated `packages/contracts` into native/Vega consumers. Existing guide screens consume `Programme.image`; no screen redesign/new setting is required. The digest route is removed, so do not preserve old generated builders.
- Private architecture/native documentation: correct device-key/proof-of-possession and mandatory-encryption/TLS promises that contradict actual bearer binding and programme decisions. Those repositories are outside the writable lane.
- Packaging/operations: native installer signing/notarization and real release verification remain separate; the current workflow explicitly publishes unsigned installers. No signer/deployment changes were attempted.
- Hosted/security operations: exercise actual signer rotation/revocation propagation during a disconnected-server incident. The local learned-revocation code is verified by fixtures, not production propagation.
- Identity/client owners: the historical Apple explicit_signout report still needs real diagnostic evidence; no cause was invented.

## Test notes for other environments

Run from the server worktree, with `GOFLAGS=-p=2`. The following were actually executed on Ubuntu, using isolated temporary fixture databases and no live accounts:

- `go build ./...`, `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=darwin GOARCH=arm64 go build ./...`: passed.
- `go vet ./internal/httpapi ./internal/identity ./internal/access ./internal/livechannels ./internal/servicelog ./internal/localmetadata ./internal/subtitles`: passed.
- `go test ./internal/httpapi -run 'TestAuthenticationCache|TestPlaybackCredentialCannot|TestReadCredentialDoes|TestGuideImageServing|TestMCP' -count=1`: selected tests were executed in groups as changes landed and passed. Also passed both named restriction matrices (`TestRestrictedProfileRouteMatrix`, `TestRestrictedProfileWriteRouteMatrix`), the route-scope guard, Cast/secondary-owner-profile, credential/role/expiry, proxy/setup-origin, profile event, stream revocation and playback restriction tests.
- Selected tests passed in administration (delete replacement/symlinks/preview, resolved picker, update feed), remote media (DNS/redirect/address policy), mediaexec (spawn inventory/environment/job validation), identity/Hosted (admission, PIN across devices, refresh reuse, signer inheritance/revocation), backup (manifest names/linked WAL/scrub), subtitles (unsafe assets/rooted sidecars), livechannels (programme icon projection), webhooks (validation/signature/delivery) and servicelog (short secrets/stderr/retention).
- `node --experimental-strip-types --test packages/client-core/test/channel-guide.test.ts`: **17/17 passed**. `npx tsc -p packages/contracts/tsconfig.json --noEmit`: passed, without installing dependencies.
- Fuzz from `server/`: `go test <package> -run '^$' -fuzz '^<target>$' -fuzztime=6s -parallel=1`. Results: localmetadata/FuzzVideoNFOStructure 25,075 executions; livechannels/FuzzGuideXMLStructure 19,487; livechannels/FuzzPlaylistRows 16,041; subtitles/FuzzTolerantSubtitleFormats 20,388; audiotags/FuzzRead 88,564; naming/FuzzRead 4,475; httpapi/FuzzMCPMessages 11,732. All passed. No failure corpus was produced.

Cold follow-up acceptance:

1. **Windows/macOS files and sandbox:** use a disposable server, two media roots and a fixture film/subtitle. Replace a source root/ancestor/final file with links (Windows junction/reparse point included), and attempt playback, subtitle original/fonts and previewed deletion. Pass: reads/deletes cannot escape the admitted tree, changed media fails closed, unchanged media still plays, owner can browse configured roots. Fail: bytes outside the root or deletion of a replacement. Repeat with native FFmpeg/GPU conversion; inspect reported sandbox/baseline posture and ensure children have no server/provider credentials and die on cancellation.
2. **Proxy, IPv6, HTTP:** use an isolated hostname reverse proxy and overlay access. Configure home networks and trusted proxies; send one/two-hop and malformed forwarding chains over both HTTP and TLS. Pass: observed client/locality follow owner policy, malformed trusted chains fail remote, sign-in limits do not merge real clients, cross-origin setup writes are rejected, ordinary HTTP remains usable. Fail: untrusted headers change authority or a proxy prevents legitimate bundled writes.
3. **Keys/MCP/events:** create owner keys separately scoped read, playback and admin plus a restricted member token. On read/playback keys, attempt admin session listing/controls/termination and administrative MCP tools. Pass: 403/hidden tool, personal controls remain usable, read event feed has no admin outbox, restricted token cannot see hidden titles/programme images. Warm a cache then let a short credential expire naturally without a database write: subsequent calls must refuse at expiry. Revoke while streaming: no further protected bytes/events.
4. **Guide images on each client:** refresh a guide with a shared source, one allowed programme and one over the profile ceiling. Pass: the allowed picture loads from the programme-bound path, hidden picture does not; changing source access/rating withdraws a previously known URL; switching active generations withdraws the old URL. No external provider URL or retired digest route is used. Verify after synchronizing shared packages into Apple/Android/TV/Vega clients.
5. **Hosted incidents/backups:** on an isolated claim, sign in with signer A and derive profile/TV families; teach root-signed revocation through a legitimate check-in using signer B. Pass: A's families/authentication/refresh end, unrelated direct password families and deliberately durable API keys remain as policy says. Restore a disposable backup: interactive sessions stay ended and restored key policy matches the documented behavior; private paths/seeds are never copied into test notes.
6. **Diagnostics:** provoke a fake error/report with short named passwords, factor seeds, cookie/auth headers and signed URL parameters. Inspect downloaded report, rotating records and standard service stderr. Pass: marked secrets are redacted and normal status/title diagnostics remain readable. Do not use real production credentials as probes.

Full suites, browser rendering, actual provider calls, native OS execution, hardware/tuner/media decoding, real proxy/DNS-rebinding behavior and deployed signer propagation were intentionally not run here.

## Not finished
No confirmed in-scope code finding is left unfixed. The pass is a source review plus bounded regression/fuzz verification, not exhaustive assurance for every decoder, dependency, deployment or interleaving. Native package synchronization, private documentation corrections, installer signing, physical client/platform/proxy/GPU/tuner acceptance, production Hosted revocation propagation and the historical Apple incident remain with the owners listed above. Deliberate HTTP/plaintext/no-lockout/TV-role choices and offline trust risk are preserved.

Snapshots: `001` (credential expiry), `002` (administrative scopes/events), `003` (programme images, shared parser, secret scrubbing and fuzz targets). Final snapshot `004` saved, including the two corrected issuance comments. The worktree is unchanged after that snapshot.
