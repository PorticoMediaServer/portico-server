# Outcome

Independent review completed against clean starting commit `ce34473`, with registry history compared from `0d7e365^` through `3f05da7` and the current combined base checked directly. **Findings: 0 blocker, 0 high, 3 medium, 0 low; all 3 fixed.**

Generated paths cannot normalize into another route or inject path/query delimiters. Generated explicit unions retain their types, reject values outside their alternatives and preserve meaningful nullable subtitles. The analysis, lyrics and readiness contracts now describe the existing handler parameters, statuses and responses. Exact generated operations are used for analysis calls; subtitle regex consumers use route templates.

No server authorization, admission, handler decoding, title visibility, stream behavior, screen layout, schema or settings changed. Production still has 622 operations. Server compilation, targeted Go tests/vet, TypeScript and affected shared-client/decoder tests passed. Cumulative snapshots 001 and 002 capture the finished tree; 002 is the final snapshot. No tree changes follow it.

# Decisions I made

- Reviewed code and pre-migration tables rather than accepting author reports. All **466 exact old credential-scope entries** represented by current operations and **520 exact old admission entries** matched without differences. Prefix placeholders were replaced by concrete declarations; non-API infrastructure remains outside the registry.
- Accepted the HTTP adapter as the smallest way to retain existing authorization, decoding, cache headers, status/error envelopes, uploads and streaming. A second serializer or authorizer was not introduced.
- Invalid builder parameters fail locally. Valid raw values are encoded once; pre-encoded values retain their existing spelling. Dot segments are rejected because browsers normalize even percent-encoded dots. Regex fragments use `routePattern`, not the URL builder.
- Union decoding tries the more specific job shape first, preserving every job field before considering the smaller control acknowledgement. Object decoding remains tolerant of additive fields. Optional null is preserved only when the schema explicitly accepts it.
- Corrected declarations to match handlers, including owner access on analysis mutations. Retained all scopes, routes and runtime behavior. No legacy paths, compatibility adapters or visual changes were added.
- Context sync failed because its lock directory is read-only. Existing canonical project, verification and attribution guidance was read directly. Execution host: `ehler-homelab-mini-1` (personal Ubuntu). Updated user AGENTS guidance was received during work and respected.
- No questions, waiting for user answers, helpers/subagents, commits, pushes or deployments. Only named writable trees and scratch paths were used.

# Bugs

The numbered review findings F1–F3 are fixed below. Historical audit entries describe a much older tree. Statuses distinguish verified source changes from wider acceptance work; this is not a new whole-web audit.

- B-08.1: already fixed in current `request_locality.go`; shared origin handling uses proxy/locality policy. Real reverse-proxy acceptance was not repeated.
- B-08.2: already fixed on this combined base: `router.go:failure` defaults to 500/internal and logs server failures. Earlier registry/API-platform reports describing a 400 default are stale.
- B-08.3: already fixed: shared event client retries 409 and only permanently falls back for 404/405; opening takeover remains on the server.
- B-08.4: already fixed: `events_routes.go` sends `stream.opened` with the initial cursor.
- B-08.5: already fixed: capability and client long-poll maximum are 25 seconds.
- B-08.6: not fixed here: general event wake fan-out remains inherited event-feed/performance work; this migration did not introduce it, and load impact was not measured.
- B-08.7: already fixed: canonical trusted-proxy address resolution is shared by sign-in, fairness and credential resolution.
- B-08.8: already fixed in current grid source: resource catalogue, semantic viewer and policy revisions replace global writes/token hash. Time buckets still bound signed continuation reuse.
- B-08.9: not a demonstrated defect: intentional response/null/page guards retained; adapter retains previous serialization. Real contract fixtures decode successfully.
- B-08.10: already fixed: shared API errors retain server retry directives. Targeted API-error tests passed; migration kept those branches.
- B-08.11: registry/document drift addressed by prior migration and corrected further by F2/F3. Separate toolkit cursor/idempotency/precondition differences remain inherited documentation/domain work.
- B-08.12: partly already fixed: compression negotiation and hashed asset caching remain; post-composition 304 cost is inherited, not changed here.
- B-08.13: already guarded: item/library access labels require matching placeholders, and handler checks remain on HTTP contracts. F2 corrects analysis owner metadata. Restriction matrices pass; no newly reachable credential/role or hidden-title leak found.
- B-08.14: inherited trade-off: third-party title query has revision-independent paging; composed first-party walks retain their fences. No new paging design in this lane.
- B-08.15: already fixed: original media MIME derives from container in `media_original.go`.
- B-08.16: already fixed: public system omits build/source/time fingerprint.
- B-08.17: not fixed here: realtime pool capacity/sharing remains inherited admission work. No capacity target is claimed.
- B-08.18: partly already fixed: large-upload lane uses rolling upload progress deadlines; native slow-uplink acceptance and other upload families remain networking work.
- B-16.1: already fixed structurally on base: browse collections are pooled for Back restoration. Browser navigation acceptance was not repeated.
- B-16.2: already fixed for erasing another tab's credential: `ViewerSelectionCommit.clear` uses family-conditional clearing. Multi-tab runtime acceptance remains for identity/web.
- B-16.3: already fixed: recognized content-hashed assets are immutable, confirmed in installation source/fix log.
- B-16.4: not fixed here: virtual-scroll geometry is screen work, explicitly outside this review's editing boundary.
- B-16.5: not fixed here: grid observer/layout/memoization performance requires the frontend lane and browser measurements.
- B-16.6: partly already fixed on combined base: active regional I18n is published during provider render and sampled screens use it. Whole-catalogue/locale presentation acceptance remains outside this lane.
- B-16.7: partly already fixed: visibility-aware feed integration exists. Multi-tab HTTP/1.1 connection capacity remains browser/freshness acceptance.
- B-16.8: already fixed in general shared HTTP path: `withinDeadline` wraps requests with bounded deadlines; this migration retains them.
- B-16.9: already fixed by recorded lazy-import reload change. Browser upgrade acceptance was not repeated.
- B-16.10: already fixed for the cited Home enum: tolerant helpers/fallback remain; generated unions now retain explicit alternatives without changing screen parsers.
- B-16.11: already fixed in connection overlay source: current content remains mounted underneath the status cover.
- B-16.12: partly already fixed: card focus has a `:has` fallback. Whole browser-floor/CSS acceptance remains frontend work.
- B-16.13: already fixed: main landmark and skip navigation exist; recorded fix retained.
- B-16.14: partly already fixed: page-title/announcement implementation exists. Timing/same-name gaps from fix log remain frontend acceptance work.
- B-16.15: already fixed: accessible card labels include viewing state; recorded fix retained.
- B-16.16: partly already fixed: windowed lists expose position/size. Real focus and screen-reader acceptance remains frontend work.
- B-16.17: partly already fixed: TextArea/Select use `aria-describedby`. Contrast, font sizing and primitive presentation are outside this lane.
- B-16.18: not fixed here: Radix/CSP scroll locking requires server-served browser acceptance and frontend changes.
- B-16.19: already fixed on base: browse vocabulary uses bounded revalidation scoped to sign-in.
- B-16.20: already fixed: windowed reader honors server retry wait and `waitUntil`; bounded default remains fallback.
- B-16.21: already fixed: cursor stack resets when its dependency tuple changes.
- B-16.22: not fixed/re-reviewed here: credits paging concurrency/failure presentation is screen work; no claim of current browser reproduction.
- B-16.23: not fixed here: React hook tooling policy is frontend/release work.
- B-16.24: not fixed/rebuilt here: production development-chunk contents require frontend build acceptance, forbidden in this lane.
- B-16.25: not changed here: CI/container Node alignment belongs to release/base-health; no container build or rollout performed.

# API changes

**Runtime paths, scopes, request and response behavior remain unchanged; no routes added/removed.** These are declaration/generated-consumer corrections:

| Operation | Old declaration | Corrected declaration |
|---|---|---|
| GET `/v1/items/{id}/analysis` and `/analysis/artifacts/{artifact}` | No query fields | `sourceId`, `sourceRevision`, `mappingRevision`, `sessionId`, `generation`, `positionUS` |
| POST `/v1/items/{id}/analysis/jobs` | Device, 200 job | Owner, 202 job |
| POST `/v1/items/{id}/analysis/jobs/{job}/control` | Device, required job at 200 | Owner, `{status}` at 200 or job at 202; generated response union |
| POST `/v1/items/{id}/analysis/markers`; PUT `/analysis/policy` | Device access label | Owner label matching existing explicit check |
| Five lyric mutation operations | Advertised three query parameters | No query parameters, matching retained rejection |
| PUT `/v1/lyrics/provider` | 64 KiB body bound | Existing 2 MiB decoder bound |
| POST `/v1/libraries/{id}/lyrics/fetch` | 200, 64 KiB | 202, existing 2 MiB decoder bound |
| GET `/v1/readiness` | 200 document, all other statuses assumed error envelope | Adds 503 readiness document |
| Generated raw/pre-encoded path helpers | Accepted dot segments/delimiters | Local rejection of destination-changing parameters |
| Generated explicit union schemas | `unknown`, no branch validation; optional null discarded | Typed unions, branch validation, meaningful null retained |

Regenerated production, optional and development contracts. Synced Hosted route builders from its existing read-only generated contract source (96 operations); no Hosted backend change. Server/client OpenAPI remains the sole production document. `media-analysis.ts` now uses exact operation builders and request/response types; subtitle validation uses route templates. No screen work or new user-facing strings.

# Schema changes

None. No migration or schema-sync change was needed.

# Owner or viewer settings added or wired

None.

# Needed from others

- Native/TV/Vega package synchronization must copy the corrected canonical shared contracts and client-core files. No native checkout was writable here; no platform implementation or visual change was attempted.
- The inherited audit gaps named above remain with their owning frontend, freshness/performance, networking and release lanes. They are not new registration findings. No new client screen is required for F1–F3.
- Earlier registry/API-platform reports need their B-08.2/default-error and B-16.8/deadline statements refreshed: current combined source already defaults unexpected errors to internal failures and bounds general client requests.
- Dormant remote-control/Hosted profile protocols remain separate optional declarations with no production handler, as on this base. This review did not revive those APIs.

# Test notes for other environments

All Go commands use `GOFLAGS=-p=2`; node_modules was reused without installation. Passed on personal Ubuntu:

- Server `go build ./...` (compilation only; no full test suite).
- Server targeted `go test ./internal/httpapi -run 'TestAnalysisContractsKeep|TestReadinessContractKeeps|TestEveryRouteDeclaresACredentialScope|TestEveryRegisteredRouteHasALane|TestProductionRouterConstructsWithEveryMediaRoute|TestAllProductionRoutePatternsAreCompatible|TestRestrictedProfileRouteMatrix|TestRestrictedProfileWriteRouteMatrix|TestOpenAPICoversRegisteredRoutes|TestEveryRouteAnswersErrorsInTheEnvelope' -count=1`; targeted key/scope checks also passed.
- `go vet ./internal/httpapi`; apikit `go vet .` and targeted HTTP adapter/generation/schema-provider/nullable/binary documentation tests.
- `npx tsc --noEmit -p` for contracts, client-core and web configurations.
- Affected Node test files: generated-routes, media-analysis, lyrics, subtitles, subtitle-originals, api-error-path, union-contracts. Existing recorded response fixtures (`packages/contracts/test/contracts.test.ts`, 26 cases) decode successfully.
- `go run ./cmd/apigen`, `go run ./cmd/clientcontractgen`, development `go run -tags devtrust ./cmd/apigen -development`, and Hosted sync script. Final generation drift check recorded below. `git diff --check` passes.

Cold acceptance instructions:

1. On any browser/native client, construct an item path with raw `..` or encoded `%2e%2e`, a raw pre-encoded slash/question/hash/backslash/control, or malformed percent escape. Builder must reject before transport. Valid raw Unicode, slash, percent and punctuation must remain one correctly encoded segment. Test normal subtitle catalog and original ASS/image subtitle/font paths after package sync; existing anchored validators must still accept only their grant-bound paths.
2. Decode device-command subtitles with string/null and profile-library policy with list/null. Keep valid null as a meaningful value; reject numbers, objects and wrong list element types. Ordinary omitted optional values and additive response fields must keep their tolerant behavior.
3. On a configured server with an owner session or permitted library key, read analysis with all advertised source/session/revision/position fields; queue -> 202 job; pause/resume/cancel -> 200 `{status}`; retry -> 202 job. Decode both branches without dropping job fields. A member must still be refused on owner analysis mutations, and restricted/absent titles must remain concealed identically. Fetch library lyrics -> 202 scheduler job; lyrical writes must reject query parameters. Compare the served handler replies with generated schemas.
4. During startup with no database, readiness must be 503 `{ready:false,status:"starting"}` with Retry-After 2; configured readiness retains 200. Real-network slow upload, tuner/GPU, Windows/macOS runtime, physical clients and capacity acceptance were not run. No handler behavior changed for those platforms.

No full suite, Gradle/Xcode/Docker/web build, external service, live load or browser visual acceptance was performed. The background-job reply contracts are source-backed with declaration/decoder regressions; this lane did not add a successful running-scheduler integration fixture.

# Not finished

No numbered finding remains unfinished. All three findings have a failing-before/passing-after regression. Platform package sync and runtime acceptance, plus inherited audit work explicitly listed above, remain outside this lane's writable scope. No deployment is claimed.

# Findings (recorded before source changes)

## F1 — Route parameters can change the destination URL — medium

- Location: `apikit/templates/route-builders.ts.txt:16` and `:28` (all generated server, Hosted, optional and development builders).
- `routePath` leaves `.` and `..` unchanged because `encodeURIComponent` does; WHATWG URL resolution removes these segments. `encodedRoutePath` inserts raw `/`, `?`, `#` and backslash without checking, so a purported parameter can add path segments, inject a query or truncate the request. Percent-encoded dot segments normalize too. This is a client destination-integrity defect, not a demonstrated server authorization bypass.
- Evidence: the shared template is used by every generated module; existing tests exercise spaces and missing parameters only. A new regression will check hostile inputs and final URL resolution.
- Smallest sound fix: reject dot segments in both builders and reject URL delimiters, controls and malformed escaping in the pre-encoded builder; keep valid percent encoding once and normal opaque IDs unchanged.

## F2 — Analysis and lyric contracts differ from the retained handlers — medium

- Location: `server/internal/httpapi/http_contracts_playback.go:95–128,218` and `http_contracts_administration.go:88`.
- Analysis GET declarations omit all six accepted query fields (`sourceId`, `sourceRevision`, `mappingRevision`, `sessionId`, `generation`, `positionUS`). Queueing analysis and fetching library lyrics return 202 but declare 200. Analysis control returns `{status}` at 200 for pause/resume/cancel and a job at 202 for retry, while declaring only a required job at 200. Lyric writes advertise query fields that the handler rejects; the provider and bulk lyric routes use a 2 MiB decoder but declare 64 KiB. Readiness can return a 503 readiness document rather than the default error envelope.
- Evidence: source trace through `analysisQuery`, `analysisRoutes`, `lyricRoutes`, `lyricDecode` and `concurrencyRoutes`; static handler status inventory independently found the missing lyrics 202. Runtime behavior is retained; generated consumers receive an inaccurate specification/type/decoder.
- Smallest sound fix: correct the localized declarations, model both analysis-control response branches as a typed union, and add handler/declaration/decoder regression checks. Use the exact generated operations for analysis calls rather than adding operation suffixes to a base path.

## Verified non-findings so far

- All exact pre-migration hand-table scope entries match current registry scopes. Deleted prefix placeholders were replaced with concrete declarations; the six remaining infrastructure entries are unchanged.
- Targeted route scope, lane, production-router, OpenAPI coverage and both restriction matrices passed on the unchanged tree (`11.568s`).
- HTTP adapter calls the same original handlers without introducing body decoding, query rejection, response serialization or cache/stream changes; repeated scope enforcement does not mint authority.


## F3 — Explicit schema unions lose both typing and validation — medium

- Location: `apikit/generate.go:507` and `apikit/templates/runtime.ts.txt:7`, with concrete producers in `server/cmd/clientcontractgen/main.go:42–54`.
- Schema providers publish `anyOf` for nullable subtitles and allowed-library lists. `tsType` sees no `type` and returns `unknown`; generated `value` treats a schema with no type as unconstrained JSON before considering the union. Consequently the advertised string-or-null subtitle accepts arbitrary objects/numbers, and array-or-null libraries accept numbers. This is at the generated consumer boundary; the server still validates its inputs.
- Evidence: generated optional `Command.subtitles` and `ProfilePolicy.allowedLibraries` are `unknown`; the runtime has no union branch. Add a concrete decoder regression against these existing generated schemas before fixing it.
- Smallest sound fix: generate TypeScript union types and validate `anyOf`/`oneOf` branches before applying the empty-schema fallback; preserve tolerant object parsing and optional-null rules. Exercise both existing nullable providers and analysis-control responses.

# Fixes

Implementation beginning after the findings above were recorded. No findings withdrawn.

## Implemented fixes and evidence so far

- F1: the new hostile-parameter test failed before the change (`Missing expected exception: get_items_id_detail: raw .`), and direct URL reproduction resolved `..`/`%2e%2e` to `/v1/detail`. Both builders now reject dot segments; the encoded builder also rejects raw delimiters, controls and malformed escaping. Server, Hosted, optional and development builders were regenerated. Valid escaped slash/question/hash values remain one segment. Subtitle regular expressions now obtain their path template from `routePattern`, rather than passing regex fragments as encoded URL parameters. This fixes the two internal pattern consumers exposed by the stricter builder without changing their validators.
- F3: the Go type test failed with `union type erased: { "label": unknown }`; existing optional decoder tests failed by discarding subtitle null and accepting malformed library lists. The generator now emits real `anyOf`/`oneOf` TypeScript unions, checks alternatives before unconstrained-schema handling, and preserves explicit nullable optional fields. Existing optional subtitle/library tests pass; the analysis-control decoder retains both acknowledgements and all queued-job fields. Tolerant object parsing remains.
- F2: two new server tests failed before correction with 12 missing accepted-query fields, wrong 200 declarations for two 202 operations, omitted control response branch, false lyric write query fields, two incorrect body bounds and missing readiness 503. Both pass after localized declaration corrections. The 503 check calls the actual readiness handler and verifies Retry-After. Analysis query checks call `analysisQuery`; successful job queue/control behavior is established by source trace, not a new running-scheduler fixture. Analysis mutation access metadata now says owner, matching its existing explicit owner check. No authorization or request decoder behavior was changed.
- Baseline and post-change route coverage, scope, admission, real router construction, read/write restriction matrices and error-envelope checks passed (`10.236s` post-change). Server `go vet ./internal/httpapi` and apikit vet passed. Contracts and client-core TypeScript passed. Targeted media-analysis behavior tests passed, preserving source/revision fences, timeout/cancellation, read-only refusal and mutation retry receipts. Subtitle originals, lyrics and API-error tests passed after the pattern-consumer correction.

## Final verification and snapshots

- Final repeat of production, optional, development and Hosted generation changed **zero artifact hashes**. The production document still contains 622 operations; Hosted builder sync uses 96 existing operations.
- Final affected Node selection passed **63 tests**, with no failures (generated paths, media analysis, lyrics, subtitles, subtitle originals, API errors and explicit union decoding). The separate existing recorded-response fixture file passed 26 cases.
- `git diff --check` passed. Earlier passing server build, focused Go tests/vet and contracts/client-core/web TypeScript checks apply to the identical final source tree.
- Snapshot 001: `Keep generated routes and wire contracts faithful to their handlers`.
- Final snapshot 002: `Finish verifying generated paths and response contracts`.
- Snapshots are cumulative against `ce34473`, under `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/snapshots/review-registry/`. Source edits are limited to apikit generation/templates, localized analysis/lyrics/readiness declarations, generated artifacts, three shared client implementation files and focused regression tests. No source changes follow the final snapshot.
- F1, F2 and F3 are fixed. No numbered finding was withdrawn or left open. Original findings above remain the before-fix record; this Fixes section supersedes their defect status.
