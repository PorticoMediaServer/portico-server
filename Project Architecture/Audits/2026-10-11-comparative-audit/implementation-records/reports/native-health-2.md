# Outcome

Native’s complete suite passes **605/605**, with zero failures, skipped or cancelled tests. Vega’s complete suite passes **87/87**, also with zero failures, skipped or cancelled tests. All four application/test TypeScript configurations and both string lints pass. All requested commands are clean.

The native library screen consumes the refreshed shared `defaultSort`, `opens` and `categoryDestination` definitions directly. Recently aired remains a changeable starting order; series category links now send `contains-any`, consistent with the shared filter controls. The obsolete `fixedSort` fallback and duplicate category/group destination implementations are deleted.

Guide tests now exercise bounded directory pages and named-channel programme reads. They no longer negotiate a removed endpoint, and store cleanup runs even after an assertion fails. Playback, resume, capability and queue tests run the real consumers with a narrowly marked local adapter for an incorrect shared fixture’s encoded media parameter.

Existing behavioural adaptations are retained and verified: 428 `revision_required` rereads/retries through shared `retryWhenStale`; ratings convert five displayed stars to integer 0–10; playback/deferred progress send milliseconds; existing library/group controls intersect server-advertised pivot and surface options.

# Decisions I made

- Work ran on personal Ubuntu `ehler-homelab-mini-1`, in the assigned worktree. Context sync was attempted but its lock directory was read-only; canonical project, standards and verification guidance were read directly.
- No copied `packages/*` edits, dependency installs, platform builds, commits, pushes, deployment or helper/model sessions.
- Use the current shared definitions and remove superseded native fallback code. No new screen or setting was designed.
- The shared fake server still passes an unencoded slash to `encodedRoutePath`, both here and in the read-only server base, despite the queue-and-retention report recording a previous repair. Encode only that exact fixture parameter through `apps/native/test/resolve.mjs` until canonical correction; production route validation stays unchanged. A regression checks both strict raw-slash rejection and the fixture’s complete encoded nested file path.
- The guide directory is now one server-authored response. Tests check healthy source rows returned by that directory and an error for a failed selected source; they do not reinstate client-side old-server negotiation or whole-lineup reads.

# Bugs

The lane brief assigns no audit domain documents or bug IDs. No wider audit closure is claimed.

| Error group and cause | Confirmed repair/result |
|---|---|
| `channels-model.test.ts`: shared bounded directory API replaced the retired 404 negotiation and client directory walk | Updated fixture to `/v1/guide/channels`, offset/limit/revision envelopes, server ordering and source/group filters. Programme requests must name channels. Five failing assertions repaired; all 16 cases pass. |
| Hanging `channels-model` file | The store’s retry timers survived a failed assertion because dispose came after it. Register cleanup with `t.after` before starting the store. Full suite now settles. |
| 19 failures in capability-profile, native-playback, queue-step, queue-step-group and v1-resume | Shared fake session response threw `Invalid encoded route parameter` before consumer assertions. Narrow test adapter encodes the generation/file parameter; affected five files pass 25/25 without loosening their behaviour assertions. Added one registry/fixture regression. |
| Shared library-tabs changes (`web-viewer`) | Removed legacy sort/group/category adapters; consume shared definitions directly. Book-series links now use the shared `contains-any` operator. All 14 library-view tests pass. |
| Vega HLS buffered seek | Final run exposed a scheduler race: moving forward inside the buffer may fetch the next segment to keep the 30-second look-ahead. Wait for the new look-ahead buffer endpoint rather than 50 ms; require exactly the one new segment and the full retained buffer span. Duplicate/init/playlist reads still fail. No production change or dropped invariant. Final Vega suite passes 87/87. |
| 428 revision enforcement (`base-health`) | Already adapted. Both native transport factories use the existing bounded adapter; 11 revision-required cases pass, including cancellation, failed reads, exact paged device identity, same operation ID and retained genuine failures. Shared freshness now also recognizes `revision_required`/`revision_mismatch`; specialized transport coverage remains useful. |
| Wire units (`search-sort`/integrator) | Already adapted. Detail’s star input sends `Math.round(stars * 2)`; playback timeline/stop and deferred progress assertions retain milliseconds, including 120000 and no `positionSeconds` wire field. |
| Pivot/surface vocabulary (`grouping-sorts`, native-screens) | Already adapted. Existing browse hook applies `scopeBrowseCapabilities`; six option cases and grouping tests pass. Unsupported remembered sorts fall back to advertised options; missing surface advertisements cannot borrow another surface. |

# API changes

No server route, registry, request/response shape or legacy route changed. Native consumes current shared category/tab definitions. Test fixtures use the current bounded guide directory and generated media route.

# Schema changes

None.

# Owner or viewer settings added or wired

No new settings. Existing library/tab Sort and Filter retain profile-scoped memory and server-advertised choices. Recently aired’s shared `defaultSort` applies only until the viewer chooses another offered sort. Rating remains five displayed stars with 0–10 on the wire. Original quality and Automatic engine defaults are unchanged.

# Needed from others

- **Canonical shared fixture owner:** in `packages/client-core/src/playback-v1/testing/fake-server.ts:243`, change `file:s.generation+'/master.m3u8'` to `file:encodeURIComponent(s.generation+'/master.m3u8')`, or use `routePath` with raw parameters. Then remove the marked `load` adapter in `apps/native/test/resolve.mjs`. The copied shared fixture remains unchanged here.
- **Canonical i18n owner:** add `vega.connection.redirectLimit` with exactly: `This device can check redirects only after a request has been sent.`
- **Canonical i18n owner:** add `vega.connection.credentialExposed` with exactly: `This connection sent your sign-in details to another address, so you have been signed out.`
  Both are still in `apps/vega/src/i18n/transport.ts`, using the shared formatter; adopt canonically and sync before deleting the local catalogue.
- **Following native frontend lane:** the new whole-view queue selector still needs screen/player forwarding described in queue-and-retention, section “Client selector wiring (S1)”. Existing saved-view Play still gathers up to 500 titles; this lane does not claim that scope is repaired. Use `{saved:{id}}` for saved resources and `{query:{libraryId,pivot,query,sort}}` for groups with their exact membership expression. No paging controls belong in selectors.
- Device acceptance and Vega’s documented post-send redirect limitation remain as described in the owning transport/platform reports.

# Test notes for other environments

Ubuntu, Node 24.21.0, existing symlinked dependencies. Logs: `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/native-health-2/`.

| Command | Baseline | Final recorded result |
|---|---|---|
| Native `npx tsc --noEmit` | Exit 0 | Exit 0; `native-app-typecheck-verified.log` |
| Native `npx tsc -p test/tsconfig.json --noEmit` | Exit 0 | Exit 0; `native-test-typecheck-verified.log` |
| Native `npm run lint:strings` | Exit 0 | Exit 0; `native-strings-final.log`, zero hard-coded strings, glossary passes |
| Native `npm test` | Hung at channels-model; separate remaining-file run 569/588, 19 failed | Exit 0, **605/605**; `native-suite-verified.log` |
| Vega `npx tsc --noEmit` | Exit 0 | Exit 0; `vega-app-typecheck-final.log` |
| Vega `npx tsc -p test/tsconfig.json --noEmit` | Exit 0 | Exit 0; `vega-test-typecheck-verified.log` |
| Vega `npm run lint:strings` | Exit 0 | Exit 0; `vega-strings-final.log`, zero hard-coded strings |
| Vega `npm test` | Exit 0, **87/87** | Exit 0, **87/87**; `vega-suite-verified.log` |
| `git diff --check` | Clean | Clean |
| `git diff HEAD -- packages` | No changes | No changes |

Focused runs: guide **16/16**; five playback/capability/queue/resume files **25/25**; combined guide/library/fixture **31/31**. No tests were skipped or marked todo. Initial root-relative invocation mistakes were corrected before successful runs. The initial unbounded native baseline was interrupted after confirming the guide hang; the isolated guide baseline timed out at 45 seconds. The bounded duplicate baseline was interrupted too. Final ordinary `npm test` settles normally. Vega’s first confirmation run passed 86/87 and exposed the timing-sensitive HLS seek assertion; the repaired complete suite passes 87/87. Existing Node module-type/experimental warnings and baselined diagnostic lint notices remain non-failing.

Kotlin standalone checking is not applicable: no Kotlin changed. Swift and Objective-C were unchanged. No Gradle, Xcode or Vega platform build ran.

Cold acceptance on iPhone/iPad/Apple TV, Android/Android TV/Fire TV and Vega, against the integrated test server:

1. Open Recently aired, choose a different advertised sort, leave and return. Pass: the new choice persists; the default does not override it. Open an audiobook series category. Pass: Books are filtered to that series with `contains-any`, and the ordinary controls reflect the filter. Clear it and switch libraries/pivots; only current advertised options appear.
2. Open Channels with live and library sources. Scroll through more than 50 channels and move through time blocks. Pass: bounded directory pages, programme reads naming visible/margin channels, correct global order and usable native tune handles. Fail a directory read, retry, and leave the screen; no continuing orphan reads/timers.
3. Inject one 428/revision_required for a settings/container/queue/session mutation. Pass: exact resource reread and silent retry with current body/header revision and original operation identity. Failed/unauthorized/malformed/cancelled rereads send no retry; repeated refusals stop after two rereads. Ordinary meaningful conflicts retain their existing policy.
4. Rate five stars, resume/seek to 120 seconds, upload offline progress. Pass: rating 10 and position 120000 milliseconds; reopen at 120 seconds. Exercise native/mpv quality and track changes, queue next/previous and resume; actual engine/media behaviour requires the device boundary beyond these logic tests.
5. Vega sign-in, profile/PIN and event acceptance follow vega-transport’s cold steps. Confirm its two messages appear in About/sign-in as intended. No stronger redirect or real device sign-in guarantee is added by this lane.

# Not finished

No required local implementation or command remains unfinished. Canonical fixture/message adoption, following frontend whole-view selector wiring and real platform/device acceptance remain with their owners as listed above. Copied shared packages are unchanged; passing native tests use the explicitly documented test-fixture adapter.

Cumulative snapshots: 001 (test/contracts), 002 (shared tab/category consumption), 003 (Vega buffered-seek assertion). Final 004 captures the verified tree with the deterministic look-ahead wait. No source-tree changes follow the final snapshot. Nothing committed, pushed or deployed.
