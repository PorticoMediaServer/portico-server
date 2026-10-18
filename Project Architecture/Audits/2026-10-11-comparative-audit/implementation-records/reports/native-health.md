# Outcome

Native and Vega are clean against the refreshed shared packages. Native's full suite passes **514/514** (baseline 494/497); Vega passes **64/64**. Both application and test TypeScript checks and both string lints pass.

An explicit **428 `revision_required`** on an existing revision-bearing operation now goes through `client-core`'s `retryWhenStale` path: read its resource again, put the current revision into the body or `If-Match`, and retry silently. Both native transport factories use this adapter; Vega uses these same native factories through its platform shims. The adapter preserves authority, cancellation, the caller's deadline, the original intent and its operation ID. Retries stop after two rereads. Ordinary conflicts, authorization refusals, interrupted writes and other 428 codes keep their existing handling.

Existing library browse controls now intersect the server's selected pivot and surface advertisements. Unsupported filters, quick filters and remembered sorts are not offered; supported sorts retain their direction or adopt the advertised default. Missing/empty advertisements do not borrow the library-wide options. Switching libraries cannot reuse another library's vocabulary. A toolbar with no advertised sorts hides Sort. Existing legacy content/Saved controls already consume their projection's advertised option lists.

Ratings and positions already use the new units and were retained: Detail converts five displayed stars into integer **0–10** ratings; native playback reports/starts/seeks and deferred offline progress send **milliseconds**. Existing native playback and offline tests assert these boundaries, including `120_000` milliseconds with no `positionSeconds` wire field.

# Decisions I made

- Work ran on `ehler-homelab-mini-1`, personal Ubuntu, inside the named worktree. Context sync failed because its lock path is read-only. Canonical project, project standards and verification guidance were read directly. No install, commit, push, deployment, helper session or platform build was performed.
- Shared packages remain untouched. Two explicitly marked local adapters cover gaps in current shared consumers: `src/bridge/revision-required-fetch.ts` and `src/app/browse-options.ts`.
- Do not convert a missing revision into an invented zero or replay an interrupted write. Only the server's explicit 428/revision_required refusal triggers the adapter. A failed, malformed, unauthorized or cancelled reread never replays the mutation.
- Resource reads use generated registry paths and preserve escaped IDs. Settings read their own revisions; console/administration envelopes use `data`/`result`; queue-entry commands read their parent queue; session commands read their session; deletion reads the configuration/deletion preview that owns its revision. Library deletion still requires the original typed confirmation.
- A device trust action must find that exact device. Its reread follows bounded pages of 100, at most 32 pages, under the original request deadline. Repeated, missing or invalid continuations cannot substitute another device's or a list-wide revision.
- No new screen was designed. Genre/title-group navigation selects `group-titles`; ordinary tabs select `library`. Both intersect their selected pivot. Aggregate tabs use their own advertised vocabulary; screens without existing controls remain the frontend lane's scope.
- Keep tests aligned with the programme's tolerance rule without dropping validation: one malformed Home entry is skipped, while a malformed document envelope is still rejected. Android source guards continue to require the non-replaying write transport, one-shot bodies and exact uncertainty classifications.

# Bugs

The lane brief assigns no audit domain documents or bug IDs. No broader native audit completion is claimed.

| Error group / cause | Finding and repair |
| --- | --- |
| `android-request-replay.test.ts`, earlier transport changes | Test expected the old inline read/write predicate. Current Kotlin uses a `critical` flag to choose `writeClient`. Updated guard asserts that choice; retained non-replaying and one-shot-body assertions. No Kotlin edit. |
| `playback-failure-policy.test.ts`, earlier playback classification changes | Test required a two-member, single-line uncertainty set. Current Kotlin also correctly classifies decoder resource reclamation and audio sink initialization/write failures as non-media faults. Test now asserts the exact five-member set and retains both platform reporting guards. No platform edit. |
| `content-forward.test.ts`, refreshed tolerant shared readers | Test expected a malformed individual Home card to invalidate the document. It now asserts that only the bad card is omitted and separately asserts malformed-envelope rejection. This was a tolerance mismatch, not a timestamp issue. |
| 428 handling, `base-health` typed revision enforcement | Shared freshness predicates and several specialized consumers do not recognize `revision_required`. Added the bounded local transport adapter using the existing shared retry helper. |
| Browse options, `grouping-sorts` per-pivot/per-surface lists | Shared toolbar consumes library-wide definitions. Native now supplies a scoped vocabulary, including supported defaults and compound quick-filter checks. |
| Ratings/positions, `search-sort`/integrator wire units | Already adapted in the starting tree. Confirmed `Detail.tsx`, native playback and offline progress; passing suite retains their wire-unit assertions. |

# API changes

No route, request/response shape or server handler changed; no route was added or removed. Consumers use existing generated route builders/patterns for capability, facet and revision-resource reads. The local adapter adds bounded GET rereads before retrying an explicitly refused mutation.

The generated contract over-declares revision errors on some GET/create/timeline routes through shared error lists. Those routes have no revision-bearing mutation to rebase; their responses retain their own policy. Actual revision-bearing typed operations are covered by the adapter's explicit operation map.

# Schema changes

None.

# Owner or viewer settings added or wired

No new settings. Existing browse Sort and Filter query controls are wired to the server's pivot/surface advertisements. Existing library-memory preferences are still profile scoped; an unsupported remembered sort falls back to an advertised default. Rating remains five displayed stars with the server's 0–10 wire value.

# Needed from others

- Shared client owner: teach the shared freshness predicate and specialized revision consumers to handle 428 `revision_required`; reuse bounded reread/retry policies and preserve meaningful conflicts. The native transport adapter covers present first-party consumers until this is unified upstream; do not copy it into `packages/` without that review.
- Shared presentation owner: make `toolbarModel`/sort selection intersect `resolvedPivot.sorts` and `resolvedPivot.filters` plus the requested surface's lists, including quick-filter expression fields. Explicit empty/missing advertised lists must not fall back to the entire library vocabulary. The native adapter supplies this now.
- Frontend lane: new full-control title-group screens (person, container, collection, Saved/search scopes) and controls on surfaces that currently have none remain that lane's work. Existing legacy envelopes retain the server's narrower advertised vocabulary; this lane did not design replacements.
- A device-specific revision GET would avoid walking an administration device list. The current adapter remains bounded and cannot silently recover a device beyond 3,200 entries or the existing caller deadline; it preserves the refusal instead.
- No new catalogue strings or server/schema work are required for these fixes.

# Test notes for other environments

All checks ran on Ubuntu against the copied packages and existing node_modules symlinks. Logs are in `_program/scratch/native-health/`.

| Required command | Baseline | Final result |
| --- | --- | --- |
| `apps/native: npx tsc --noEmit` | Exit 0 | Exit 0; `native-final-app-typecheck.log` |
| `apps/native: npx tsc -p test/tsconfig.json --noEmit` | Exit 0 | Exit 0; `native-final-test-typecheck.log` |
| `apps/native: npm run lint:strings` | Exit 0 | Exit 0; `native-final-strings.log` |
| `apps/native: npm test` | Exit 1; 494/497, three stale assertions | Exit 0; **514/514**, zero skipped/cancelled; `native-verified-suite.log` |
| `apps/vega: npx tsc --noEmit` | Exit 0 | Exit 0; `vega-final-app-typecheck.log` |
| `apps/vega: npx tsc -p test/tsconfig.json --noEmit` | Exit 0 | Exit 0; `vega-final-test-typecheck.log` |
| `apps/vega: npm run lint:strings` | Exit 0 | Exit 0; `vega-final-strings.log` |
| `apps/vega: npm test` | Exit 0; 64/64 | Exit 0; **64/64**, zero skipped/cancelled; `vega-verified-suite.log` |
| `git diff --check` | Clean | Clean |
| `git diff HEAD -- packages` | No changes | No changes |

Additional focused checks: original assertion repairs **9/9**; initial adapter regressions **13/13**; completed adapter/edge regressions **17/17**. The first post-change whole native run passed 510/510 before four edge regressions were added; the final full run includes all 514. Existing module-type/experimental Node warnings and pending baseline diagnostic string-lint notices are unchanged and do not fail the commands. Initial two log-path mistakes were corrected before executing the checks; no dependency installation was needed.

Kotlin standalone checking is not applicable: no Kotlin was touched. Swift and Objective-C were also unchanged.

Cold device acceptance on iPhone/iPad/Apple TV, Android/Android TV/Fire TV and Vega:

1. Build each normal platform app against this tree and a server with the current generated contract. Use an authenticated viewer and an owner profile for administration. No production deployment is required.
2. At a test server/proxy boundary, return one 428/revision_required for a settings/container/queue mutation, then allow the real GET and retry. Pass: the appropriate resource is reread, the retry carries its exact current body/header revision and the original operation ID, and the screen reports success without flashing an error. Verify the ordinary 409/412 handling still behaves as before.
3. Repeat with the reread returning 403, malformed revision data, a repeated device cursor or an aborted request. Pass: no second mutation is sent. Repeated 428s allow at most three mutation attempts and two rereads; genuine inability to recover can be shown by the existing error presenter.
4. Open video/music/book pivots and an existing genre/title-group view. Advertise different sorts/filters for each, then remove one remembered option. Pass: only that pivot/surface's options appear; unsupported remembered choices fall back or are dropped; no sort control appears when no sort is advertised. Switch libraries while capability reads are delayed; another library's options must not flash or be requested.
5. Rate a title five stars, play/seek to 120 seconds and upload deferred offline progress. Pass: rating is 10, position is 120000 milliseconds, no seconds field appears on the progress wire, and reopening the title resumes at 120 seconds. These values already had unit-test coverage here; this step verifies the device/transport boundary.

# Not finished

No required command is failing, and the requested lane work is complete. Hardware rendering/playback and native platform builds were not run; the acceptance steps above remain for their normal environments. Shared-package unification and new frontend screens are intentionally left to their owning lanes. Recovery beyond the bounded device-list read is not claimed.

Cumulative snapshots: 001 (test repairs), 002 (revision and browse adapters), 003 (bounded pagination and missing-advertisement edges). Final snapshot 004 captures the verified tree; no worktree changes were made after it.
