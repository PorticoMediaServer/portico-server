# Portico server — static code audit

Date: 2026-09-25. Component: `apps/portico-server/server`, its `apikit` dependency, and server release/container tooling. Requested baseline: split repository `820dde1`.

## Method and evidence boundary

This is a **source-only review**. No application, test, build, service, decoder, database query, network request, deployment, or exploit was executed. Only source inventory/search/read operations and this report write were performed. Application files were not changed. Paths below are relative to the enclosing Portico workspace, so `apps/portico-server/` identifies the split repository. Line numbers identify the inspected source, not a generated build.

The server inventory contains **1,056 non-test Go files / 218,859 physical source lines and 766 Go test files**. This is a risk-directed audit with deep review of selected call paths and broader package inventory; it is not a claim that every line or every route has been exhaustively verified. The coverage matrix explicitly distinguishes deep-path, sampled, and inventory-only review. Tests were read selectively for assertions and omissions; their presence is not evidence that they currently pass. Existing architectural descriptions were context; current source determined the findings.

Confidence means confidence in the source-level mechanism. “High” does not mean reproduced on a running server. Severity: P1 is a release blocker or severe availability/data-integrity defect; P2 is a material feature, security-control, scalability, or reliability defect. The independent adversarial pass is recorded in `07-adversarial-hosted-reviewer.md`; it retained all twelve mechanisms, narrowed several consequences and downgraded SV-012 to P2. No P0 was established.

## Findings overview

| ID | Priority | Confidence | Finding |
|---|---|---|---|
| SV-001 | P1 | High | Subtitle refresh/search and lyric search leak the process-wide database writer gate |
| SV-002 | P1 | High | Restore journal is overwritten non-atomically and startup proceeds after unresolved recovery errors |
| SV-003 | P1 | High | Isolated release snapshots omit required Go embed inputs |
| SV-004 | P1 | High | Docker web build omits the required i18n source package |
| SV-005 | P2 | High | Generated pre-restore backups are rejected by the restore source validator |
| SV-006 | P2 | High | Concurrent restore staging shares destructive temporary paths and ignores operation identity |
| SV-007 | P2 | High | Failed Cast credential guesses roll back their own rate-limit counter |
| SV-008 | P2 | High | Hosted Cast pairing consumes its code and then always rejects session issuance |
| SV-009 | P2 | High | Lost Cast reconnect results can force manual re-pairing |
| SV-010 | P2 | High | Server source provenance excludes a compiled local dependency |
| SV-011 | P2 | High mechanism; medium operational impact | Recommendation requests synchronously replay the entire pending profile backlog |
| SV-012 | P2 | High | Shipped Cast receiver still requires removed Playback v2 endpoints |

**Counts: 12 source-supported defects: 4 P1, 8 P2.** Conditional concerns later in the report are not included in these counts.

## SV-001 — P1 — Subtitle and lyric publication leak the global writer gate

**Locations:** `apps/portico-server/server/internal/subtitles/refresh.go:16–55`; `server/internal/subtitles/provider.go:261–298`; `server/internal/lyrics/acquisition.go:190–264` (all subsequent `server/` paths in this finding have the same `apps/portico-server/` prefix). Gate ownership: `server/internal/dbwork/write.go:11–14,71–120,210–247,269–310`. HTTP entrypoints: `server/internal/httpapi/subtitle_routes.go:165–186`; lyric search is wired in `server/internal/httpapi/lyrics.go`.

**Trigger:** An authorized viewer refreshes subtitles successfully through the extraction phase, searches an enabled subtitle provider successfully, or searches local/provider lyrics successfully through acquisition. The refresh and local-lyrics cases do not require a configured external provider.

**Evidence chain:** Each method ends its first `gated` transaction before expensive acquisition. It then opens **`gated2`**, uses `gated2.Tx()`, but defers **`gated.Rollback()`** and commits **`gated.Commit()`**. The first handle is already complete. `Write.Commit`/`Rollback` explicitly return without work after `done` is set. The second handle's `finish` is never called. `dbwork.Begin` acquired a package-global gate; SQL transaction cancellation cannot call this wrapper's release function. Therefore the new publication rolls back when its context ends, while the application writer permit remains held. Refresh then enters `s.List`, which can itself wait behind the leaked gate; searches can return candidate IDs that were never committed.

**Impact:** A normal viewer action can stop subsequent gated writes across authentication, playback state, scanning, and administration until process restart. Ordinary snapshot reads are not all blocked, and a database-handle reset does not release the wrapper-owned gate. Failures after acquiring `gated2` leak too. This is both a functional publication defect and a server-wide availability defect, not merely a missing deferred SQL rollback.

**Fix:** Finish the second wrapper at every affected site. Prefer a scoped `WithWriteTx` publication closure so variable-renaming mistakes cannot leak ownership. Keep acquisition outside the transaction. Consider a static rule identifying a `dbwork.Write` acquisition without a matching ownership terminator; do not rely on garbage collection to release the gate.

**Verification to add/run after repair:** For all three paths, exercise successful acquisition, changed-source rejection, authorization change, and insertion failure. Assert persisted candidates/inventory, then immediately perform an unrelated gated write with a short deadline. Cancel the request after publication and assert the gate is idle. A test that only checks the returned candidate payload misses this bug.

## SV-002 — P1 — Restore recovery can leave missing/mixed live state and still start

**Locations:** `apps/portico-server/server/internal/backup/apply.go:90–138,187–235,427–457,484–489`; `apps/portico-server/server/cmd/server/main.go:267–287,323`; `apps/portico-server/server/internal/persistence/schema.go:36–64`.

**Trigger:** A process/power interruption during a restore marker overwrite after files have moved, or an I/O error that prevents recovery/rollback from finishing.

**Evidence chain:** Restore first records `preparing`, moves `server.sqlite` and replaced state files into the pre-restore directory, then overwrites the journal with `switching`. `writeMarker` uses `os.WriteFile` on the existing marker, without temporary-file replacement or a durability sync. A crash after truncate/before complete write leaves invalid JSON; a power loss also lacks ordering guarantees between journal and renames. `ApplyStaged` returns immediately for an unreadable/invalid marker and for failed `undoPreparing`/rollback. Main logs **“continuing on the current state” for every error**, then proceeds through authority opening and `persistence.Open(server.sqlite)`. There is no proven-current-state outcome that gates this continuation. If the live DB was already moved out, normal persistence opening can create a new empty database; if only some files moved, identity/database state can be mixed. The preserved original may remain recoverable in backups, but that does not make the newly served state correct.

**Impact:** An unresolved restore failure can become a fresh or inconsistent live installation, with misleading recovery status, instead of refusing to serve that unresolved state. Not every error is unsafe: clean rollback paths can return the original error after restoring valid state, and authority opening rejects some inconsistent combinations. The missing invariant is a typed, positively established recovery disposition before ordinary startup. This is not a claim that the original backup is necessarily destroyed.

**Fix:** Use an atomically replaced, fsynced journal and durable directory ordering for rename phases. Recovery must return a typed disposition distinguishing “safe original restored,” “valid replacement installed,” and “recovery unresolved.” Main must refuse to open ordinary live state for the unresolved case and report the recovery location. A helper such as the administration subsystem's durable file journal is a useful existing design reference, but its semantics must be adapted to restore phases.

**Verification:** Extend interruption tests to the marker's truncate/write/rename/sync boundaries and failed file moves/rollback, including disk-full and permission errors. Test the real startup decision after a failed restore, not only `ApplyStaged`. Assert that no empty live DB is created while recovery is unresolved. `backup/apply_crash_test.go:43–68,100–126` exercises named step hooks and preserves the original bytes; it does not interrupt the marker write itself, and its restart helper does not reproduce main's continue-on-error behavior.

## SV-003 — P1 — Release snapshots exclude required embedded resources

**Locations:** `apps/portico-server/scripts/build-server-release.py:44–52,118–142`; `apps/portico-server/server/internal/persistence/migrations.go:12`; `apps/portico-server/server/internal/mediaexec/fonts.go:24–28`; `apps/portico-server/apikit/generate.go:17–21`.

**Trigger:** Building a server release through the supplied isolated-snapshot builder, after its external prerequisites pass.

**Evidence chain:** `source_files` includes `.go`, `go.mod`, `go.sum`, selected protocol fixtures, `testdata`, and `receiver/cast`. The builder copies exactly this inventory for the server and apikit. Required embed targets are SQL migrations, `fonts/DejaVuSans.ttf`, `fonts/LICENSE`, and apikit `templates/*.txt`; none match the selection. The copied Go files still contain `//go:embed` directives naming those missing files. The builder then invokes the helper/server build from that snapshot. Go cannot compile a package when its embed pattern has no matching file.

**Impact:** The release workflow cannot produce the intended artifact from the current source. This is established by the source inventory contract, not by claiming a build was run.

**Fix:** Capture the complete allowed source/resource tree, or derive required embed inputs and local-module closure explicitly. Hash the same complete input set that is copied and compiled. Avoid a suffix allowlist that silently stops including assets as packages evolve.

**Verification:** Add a static snapshot completeness check for every embed target, including nested package resources. Then build from the isolated snapshot in the release environment and exercise embedded migration/font/template access. Do not validate only builds against the original working tree.

## SV-004 — P1 — Docker's web stage cannot resolve i18n

**Locations:** `apps/portico-server/Dockerfile:31–47`; `apps/portico-server/web/vite.config.ts:8`; `apps/portico-server/packages/client-core/src/presentation/server-text.ts:7`; representative runtime import `apps/portico-server/web/src/screens/downloads/Downloads.tsx:3`.

**Trigger:** Building the supplied Dockerfile from a clean context.

**Evidence chain:** The web stage copies `web`, `packages/client-core`, `packages/design`, and vendored HLS but never `packages/i18n`. Vite resolves `@i18n` directly to `../packages/i18n/src/index.ts`; screens import actual runtime values from it, and client-core also imports the same source through a relative path. `npm ci` cannot supply this omitted local source folder. The subsequent Vite production build therefore has unresolved source imports.

**Impact:** The server container image build is blocked before the Go/runtime stages can complete.

**Fix:** Include i18n in the build context and copied input closure, and include it in source provenance. Prefer one declared local-package inventory used by both build-context assembly and source hashing.

**Verification:** Build the Dockerfile from a clean checkout/context with no host bind mounts or prior artifacts. A local web build with the full monorepo present does not verify this path. Static verification should ensure all local aliases and relative cross-package imports are represented in COPY inputs.

## SV-005 — P2 — Safety backups created during restore cannot be restored by ID/folder

**Locations:** `apps/portico-server/server/internal/backup/apply.go:195–218,550–579`; `apps/portico-server/server/internal/backup/stage.go:152–197`; listing `apps/portico-server/server/internal/backup/backup.go:427–461`.

**Trigger:** Complete one restore, then choose the generated `pre-restore` backup as a restore source by backup ID or backup folder.

**Evidence chain:** The pre-restore folder contains the moved database named `portico.db`. `hashTree(pre)` includes that regular file and stores it in `Manifest.Files`. Both manifest-based branches of `resolveSource` explicitly reject any entry whose name equals `DatabaseName` (`portico.db`). These same generated manifests are accepted by listing. Ordinary backup creation uses a different file-manifest shape, so ordinary restore tests do not exercise the mismatch.

**Impact:** The advertised/listed safety snapshot cannot be selected through the normal restore API. An operator may manually select the DB file, but that bypasses the folder's associated state-file restoration and is not an equivalent recovery workflow.

**Fix:** Define one manifest schema for database, WAL, and auxiliary state, and use it consistently in creation and restore resolution. Validate/hash the database explicitly; do not simply strip its integrity coverage to satisfy the existing rejection.

**Verification:** Backup state A, change to B, restore A, select the generated pre-restore backup and restore B. Assert DB plus associated authority/state files. Include WAL-present and WAL-absent variants. Existing `backup_test.go:146–214` constructs pre-restore manifests without realistic files for list/prune assertions, so it does not prove this round trip.

## SV-006 — P2 — Restore staging races and operation IDs do not provide idempotency

**Locations:** `apps/portico-server/server/internal/backup/stage.go:58–147`; `apps/portico-server/server/internal/httpapi/backup_routes.go:138–154`; request lane capacities `apps/portico-server/server/internal/httpapi/admission.go:64–76`.

**Trigger:** Two authorized restore requests overlap, including retries after a slow response or separate owner tabs. The route requires owner authorization. This is an owner overlap/retry condition, not an unauthenticated filesystem attack.

**Evidence chain:** The API validates `operationId` and discards it, calling `Stage(ctx, body.Source)`. Stage has no mutex/receipt reservation and every call uses the same `restore-staged/.incoming` and `.staged-complete`. Every call removes both staging paths before copying, and later removes/renames those shared paths again. Admission permits multiple concurrent requests; it is not an exclusive restore lock. Therefore one call can delete another call's copied database or stage, and separate file copy/validation steps can operate on another request's content. The reported result is not fenced to a particular source/operation.

**Impact:** Concurrent owner restores/retries can fail nondeterministically, replace a successfully staged restore, or report validation for a stage that another request has replaced. This does not establish that every overlap corrupts bytes; sequential stage supersession appears intentional. A restore is sufficiently consequential that source selection and successful response must be tied to one durable operation.

**Fix:** Serialize restore staging, use operation-specific temporary directories, and persist an operation receipt bound to the exact source identity/digest. Publish the complete stage with one atomic switch only if the operation still owns the staging generation. Refuse to supersede an unresolved recovery journal. Couple restart signaling to successful publication of that same operation.

**Verification:** Barrier-controlled concurrent Stage calls for same and different IDs/sources, duplicate operation retries, cancellation during copy, and deletion of source backups while staging. Assert exactly one complete, self-consistent stage and responses matching its operation/source; no shared temporary deletion.

## SV-007 — P2 — Invalid Cast attempts do not consume the claimed rate limit

**Locations:** `apps/portico-server/server/internal/social/cast.go:235–261,366–380`; `apps/portico-server/server/internal/social/store.go:247–271`; `apps/portico-server/server/internal/httpapi/social_routes.go:148–153,807–834`; `apps/portico-server/server/internal/httpapi/router.go:1266–1274`.

**Trigger:** Send well-formed unauthenticated `/v1/cast/redeem` requests with unknown codes (or reconnect requests with unknown tokens).

**Evidence chain:** `s.limit` inserts/increments `social_rate` inside the same transaction used to look up the credential. An unknown code/token returns before commit; the deferred rollback discards the counter increment. The wrapper only sets response cache control. Router `authRate` applies to direct/auth/sessions/setup, not either Cast route. General admission bounds simultaneous work but does not preserve the advertised 30-attempts-per-hour budget. Six-character codes are created at `cast.go:172`; failed guesses can continue without consuming that durable allowance.

**Impact:** The intended persistent failed-attempt budget is absent for unknown and several invalid-state credentials, and an unauthenticated caller can repeatedly acquire the global writer gate for lookup/counter work. This does not prove a practical code compromise within its five-minute lifetime; no brute-force timing was measured. Successful requests and some known-expired reconnect outcomes do commit accounting; entropy, short expiry, HTTPS and admission remain meaningful controls.

**Fix:** Commit failed-attempt accounting independently of the credential mutation (or explicitly commit the refusal outcome) and add the intended edge-level limit. Scope limits carefully so an attacker cannot consume one global bucket and lock out all legitimate TVs; combine global capacity with source/device buckets and bounded storage.

**Verification:** At least 31 well-formed wrong codes in a controlled clock window must hit the limit; repeat across restart. Verify success, expired/redeemed codes, reconnect failures, and bucket isolation. Confirm admission fairness is not mistaken for a rate budget.

## SV-008 — P2 — Hosted Cast bootstrap can never finish

**Locations:** `apps/portico-server/server/internal/social/cast.go:138–187,279–286,328–343`; `apps/portico-server/server/internal/identity/service.go:203–223`; Cast viewer route `apps/portico-server/server/internal/httpapi/social_routes.go:154–164,776–803`.

**Trigger:** With Cast configured, a server principal whose authority is `hosted` starts a bootstrap and the receiver redeems its valid code. Merely visiting the Hosted website does not establish that principal authority.

**Evidence chain:** The viewer helper accepts the normal authenticated principal; bootstrap stores `p.Authority`, including `hosted`. Redemption marks the code redeemed and commits the device token. `castSession` then calls `Identity.IssueWithDevice` with that stored authority. `IssueWithDevice` unconditionally rejects every non-`local` authority because Hosted issuance requires a verified policy horizon via `IssueTx`. No alternate Hosted issuance path or earlier Hosted refusal exists in this chain.

**Impact:** Hosted viewers get a valid-looking pairing code that is consumed without returning a playable session. Retrying the code yields the consumed-code error. The refusal is security-correct at the identity boundary; bypassing it would be the wrong fix.

**Fix:** Implement Hosted Cast issuance through the verified current Hosted policy/epoch and `IssueTx` boundary, ideally in the same atomic operation as code consumption. If Hosted Cast is intentionally unsupported, reject it explicitly before generating a code and make the client capability truthful.

**Verification:** Test local and Hosted owners/members, expired policy horizon, revocation between bootstrap/redeem, and policy change before reconnect. A Hosted code must either be refused before publication or mint a session subject to current Hosted policy without consuming the code on an issuance failure.

## SV-009 — P2 — Lost reconnect results can force manual Cast re-pairing

**Locations:** `apps/portico-server/server/internal/social/cast.go:294–321,358–408`; receiver persistence `apps/portico-server/server/receiver/cast/receiver.js:239–251`.

**Trigger:** A reconnect commits token rotation, then session issuance fails, the request is cancelled, or its response is lost before the receiver stores the replacement token.

**Evidence chain:** `issueCastDevice` replaces the sole `token_digest`; reconnect commits that change at lines 405–407 before `castSession` runs. The server retains no previous-token retry window or operation-bound replay receipt. On retry, lookup by the client's old digest returns not found. The receiver only saves the new token in `adopt` after a successful response, so it cannot recover the committed replacement if the result never arrived. The initial redeem has the analogous consume-before-issue gap, but ordinary reconnect can encounter this without Hosted involvement.

**Impact:** A lost committed connection/issuance result can force manual pairing again and interrupt automatic receiver recovery. The receiver is recoverable by new pairing, and an existing access token need not stop working immediately. Once a successful response reaches `adopt`, its replacement token is stored before controller preparation, so the separate SV-012 error does not itself lose that token. Immediate one-use rotation alone does not distinguish response loss from theft.

**Fix:** Atomically issue the session with rotation where feasible, then provide a narrowly bounded replay contract tied to the old token, installation and operation ID. Return the same already-issued result on the permitted retry; do not indefinitely accept both tokens. Apply an equivalent atomic/replay policy to initial redemption.

**Verification:** Drop the response after commit and retry using the old client state; it must recover the same rotation safely. Inject identity issuance errors before response. Exercise concurrent reconnects and a replay outside the allowed recovery window to retain theft detection.

## SV-010 — P2 — Release source identity excludes compiled apikit

**Locations:** `apps/portico-server/scripts/build-server-release.py:17–26,124–133,154–156`; `apps/portico-server/scripts/container-release.mjs:25–30`; `apps/portico-server/Dockerfile:54–60`; `apps/portico-server/server/go.mod` local apikit replacement.

**Trigger:** Change only apikit and create a server release/container, or verify a copied release source directory whose apikit sibling was changed.

**Evidence chain:** The native builder copies apikit as a local dependency, but `snapshot_digest` includes sibling source only for the Hosted module and does not include apikit in either branch. Its server source digest and repeated snapshot verification therefore omit a compiled local input. Container hashing similarly walks server/web/client-core/design/HLS only, while the Go build copies and compiles apikit. Container BuildID is derived from this incomplete digest. The Docker digest also needs i18n once SV-004 is fixed.

**Impact:** The reported source identity cannot identify all source used by the binary; changing apikit can leave the source digest unchanged, and copied-dependency changes evade the source-snapshot consistency check. This provenance defect is latent while SV-003/SV-004 block their normal build paths. Native BuildID includes fresh identity/time and need not stay unchanged after a dependency edit; the incomplete source digest is the relevant value. The artifact byte hash/signature remains a separate integrity control; this finding does **not** claim that arbitrary replacement binaries pass checksum/signature verification.

**Fix:** Compute provenance over the complete local-module/resource closure actually compiled, including apikit, embedded resources, generated source inputs, and explicit trust/toolchain identity as appropriate. Use a stable sorted manifest and feed the same inventory to capture, hash and verification. Clarify whether the container identity is merely descriptive (`proof: unsealed`) or an attestation; do not imply the latter.

**Verification:** Changing each compiled local dependency independently must change the digest; modifying captured sibling source must fail post-build verification. The same canonical inventory should drive native and container source accounting. Validate this after fixing the missing-resource build failure rather than treating that failure as provenance protection.

## SV-011 — P2 — Recommendation reads replay an unbounded profile backlog

**Locations:** `apps/portico-server/server/internal/catalog/rec_engine.go:97–164,175–239,377–405`; callers `apps/portico-server/server/internal/catalog/rec_browse.go:25`, `rec_generators.go:349,411`, `rec_title.go:255`; job producers `apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:5105–5143`; worker `apps/portico-server/server/internal/compactcatalog/rec_profile.go:194–220`.

**Trigger:** A profile accumulates pending recommendation updates for many distinct works, such as a large history/feedback update or a worker backlog, then requests recommendations/Discover/title-related recommendations.

**Evidence chain:** `recSession` calls `recLoadTaste` before candidate generation/memo key construction. That method selects every pending `rec_profile_jobs` row for the profile without a limit, retains all IDs, and loops over all of them. Each iteration computes `RecSignalFor`, reads `RecStored`, and potentially reads work facets, updating taste and seed arrays. Candidate/seed limits elsewhere do not bound this overlay. Jobs are deliberately persisted by multiple personal-state triggers, while the background worker drains a bounded page. The compound key makes lookup selective by profile, but does not bound the number of that profile's rows. Existing taste/engaged sets are also loaded in full. Jobs are unique by `(profile_id, work_id)`, so repeated events on one work coalesce; the relevant growth is distinct pending works. Read snapshots and request deadlines bound consistency/lifetime, and the background worker reduces backlog. Readiness checks catalog derivation domains 18/32, not a bound on pending profile jobs.

**Impact:** Request SQL count, memory and CPU grow with profile history/backlog rather than the requested result count. A backlog can make recommendation surfaces hit their request budget repeatedly while competing for DB readers; exact latency was not measured. It is not evidence of incorrect recommendation ranking.

**Fix:** Preserve immediate hide/dislike exclusion semantics independently of ranking catch-up. Keep request-time overlays bounded to a small, explicit recent-change set and defer older work to the projection worker; maintain revision/watermark semantics so incomplete materialization is honest. Alternatively serve the last complete taste projection until catch-up and invalidate once complete. Avoid merely adding a LIMIT that silently treats old negative feedback as absent.

**Verification:** Static query-cost assertions plus runtime tests with 0, 100, 10,000 and larger pending profile jobs. Assert bounded request statement count/memory, correct immediate handling of recent hides/dislikes, eventual projection consistency and responsive unrelated reads. Read schema indexes and planner tests are useful, but no `EXPLAIN`, benchmark or latency test was run here.

## SV-012 — P2 — Embedded Cast receiver uses removed Playback v2

**Second-pass classification:** P2 complete configured-feature failure. Cast is optional/default-unconfigured; no requirement that it block every release was established. This is independent of the Hosted CSP and authority-issuance failures. A POST to an absent route can receive 405 rather than 404; the finding is protocol incompatibility, not a particular status code.

**Locations:** `apps/portico-server/server/receiver/cast/receiver.js:144,173–205,230–245,357–380,395–420`; shipping path `apps/portico-server/server/receiver/receiver.go:14–18`, `apps/portico-server/server/receiver/cast/index.html:119`, `apps/portico-server/server/internal/httpapi/social_routes.go:876–885`; current route family `apps/portico-server/server/internal/httpapi/playback_v1_routes.go:234–298`.

**Trigger:** Use the Cast receiver embedded in this server, even with successful local pairing and functioning Google Cast runtime.

**Evidence chain:** The server embeds the `cast` directory and serves its bytes directly; its HTML loads `receiver.js`. After redeem/reconnect, `adopt` unconditionally calls `prepareController`. That routine publishes the legacy client profile (a route which does still exist), then POSTs `/v2/playback/controllers` and `/v2/playback/lanes` using protocol `2.0`. The generic request helper concatenates these paths to the server base URL without version negotiation or adaptation. Production Go route registrations have no corresponding v2 handlers; current session/control APIs live under `/v1/playback/...`. Even bypassing controller initialization would leave queue creation/playback, occurrence reads, observations, renewals, stop and intents on hardcoded `/v2/queues` or `/v2/playback/occurrences` paths. The static-asset fallback refuses API namespaces rather than translating them.

**Impact:** The shipped Cast receiver cannot initialize or control playback against its own current server. This is independent of the Hosted issuance defect and affects local accounts too. Pairing can persist a token before initialization fails, making the flow appear partly successful while playback remains unusable.

**Fix:** Port the complete embedded receiver to the current Playback v1 device capabilities, queue, session, timeline and change contracts; updating only the first URL is insufficient because request/response shapes and authority semantics differ. Reuse a maintained client contract or generate an explicit receiver adapter so the receiver cannot drift separately from server/client-core.

**Verification:** Add a contract test inventorying the receiver's actual request paths and payloads against production route schemas, then execute local and Hosted pairing, load, play/pause/seek, next/queue behavior, lease/timeline renewal, subtitles and recovery on a real supported Cast target. Static HTTP asset-serving tests prove that bytes are shipped, not that those bytes speak the implemented protocol. No Cast runtime or request was executed during this audit.

## Additional concerns and questions — not confirmed defects

1. **Decoder sandbox strength depends on platform and configured fallback.** Docker explicitly permits baseline playback when user namespaces/bubblewrap are unavailable, while `PORTICO_DECODER_SANDBOX=required` refuses it. The newer decoder confinement and storage-supervisor path includes endpoint ownership and Linux proxy confinement; older generic `mediaexec` paths merit a complete caller inventory before claiming every decoder has identical network/filesystem isolation. FFmpeg protocol restrictions constrain parsing behavior, not all consequences of a compromised native process. No parser exploit or confinement escape was attempted or established.
2. **Global HLS byte budget is a reclamation target, not an absolute active-output reservation.** `playback/hls_sweep.go` deliberately prioritizes inactive output and has per-session limits and a disk-free floor. Many simultaneously active conversions may exceed the nominal global cache target. Confirm the product capacity contract and test maximum admitted activity; the code comments make this distinction deliberate, so it is not reported as an unconditional quota bug.
3. **Original-download snapshots copy an entire file in one serialized worker step.** `downloads/source_snapshot.go:22–86` is memory bounded, checks cancellation and available disk, and checks request revision every 32MiB. It nonetheless holds snapshot ownership for a whole file and runs under the download advance mutex. Measure fairness with very large/slow sources and queued small downloads before deciding whether chunk scheduling is required. No timing claim is made.
4. **Restore from a caller-supplied bare DB assumes a usable source snapshot.** Stage copies a DB and then a WAL separately, and ignores the WAL-copy error (`backup/stage.go:103–114`). Integrity inspection checks structure, not “all committed source transactions included.” Clarify whether selecting a live SQLite file is supported; if supported, use a SQLite backup/snapshot operation instead of unrelated file copies. This concern is additional to the confirmed staging/journal defects.
5. **Instance locking can fail open.** `cmd/server/main.go:259–275` continues if locking is unavailable, with a warning and restore/orphan-kill suppression. This avoids refusing unusual filesystems but relies on the owner running one server; validate the supported filesystem/deployment contract. It is not proof that two instances are normally started.
6. **Proxy topology consistency needs acceptance coverage.** Request locality parses a proxy chain, while some sign-in/rate-source helpers use more constrained forwarded-address handling. Check multi-hop trusted-proxy deployments for bucket sharing and secure-origin behavior. No bypass was demonstrated.
7. **Default LAN HTTP web entrypoint requires cross-component verification.** `cmd/server/main.go:142,192–197` advertises LAN `http://...:32500` setup. Parent audit is checking whether browser crypto/storage dependencies work outside secure contexts. This report establishes the intended server entrypoint only; it does not duplicate the client-side finding.

## Security and correctness paths reviewed without a confirmed defect

These are observed safeguards in selected paths, not blanket certifications:

- **HTTP/auth:** Router principal resolution, owner checks, source-origin/CORS response handling, authentication rate dispatch, request lane admission and principal-cache invalidation were read. The apikit registry performs access enforcement before typed handlers and validates bounded JSON; handlers' later owner rechecks are not their sole authorization. Legacy and registry routes coexist, which deserves continuing parity tests.
- **Identity:** Password hashing slots, sign-in cooldown dimensions, session/device issuance boundaries, and selected two-factor handling were read. Hosted issuance deliberately requires a policy horizon; SV-008 is a caller mismatch with that boundary. Full cryptographic protocol proof, all revocation races, and all Hosted policy routes were not completed in this lane.
- **Persistence:** WAL reader/writer separation, process-wide priority write gate, FULL-durability security-fence writes and migration digest checks were read. Fresh schema installation is transactional; old pre-release schemas are explicitly rejected, not silently migrated. The principal defect found here is lost wrapper ownership (SV-001), not a generic claim that SQLite concurrency is unsafe.
- **Content authorization:** `contentaccess/item.go` applies profile restrictions, record visibility and member ceilings through common item and set predicates. Downloads recheck profile permission/content visibility when minting/redeeming grants; HTTP additionally checks current library access before serving bytes. This sample did not reveal an item-ID bypass. A route-complete authorization matrix remains unverified.
- **Remote media:** `remotemedia/policy.go` and client transport pin validated DNS at connection time, reject protected address classes by default, and handle redirects/header stripping. Descriptor/provider approvals and source evidence are separate concepts; generic remote URLs are not handed directly to ffprobe in the reviewed assets path.
- **Artwork:** `metadata/artwork_worker.go:33–151` uses provider-specific HTTPS host/path allowlists, disables environment proxies, rejects non-public resolved addresses at dial time and revalidates redirects. It bounds bodies to 8MiB; image normalization checks dimensions and 24 million pixels before full decode (`:209–227`). No proven SSRF chain emerged from this fetch path. Local-artwork confinement depends on its storage adapter, not the remote allowlist.
- **Media source and process lifetimes:** Version/evidence-bearing storage inputs, retained source readers, endpoint reservations, child process supervision, private output roots, and decoder input-format/protocol constraints were traced in selected playback/subtitle/probe paths. Output limits and command arguments were sampled; every codec/filter combination was not reviewed.
- **HLS:** Start/seek replacement, window cancellation, restart throttling, per-session reclamation, disk-free checks and producer joining were reviewed. The mutex is deliberately released before waiting for the previous window; cancellation/process joining matters at that wait. No running-process result proves it works on all platforms.
- **Subtitles/lyrics:** Parsing/acquisition/provider publication and retained range input were sampled. Provider results carry short-lived actor-bound candidate state; source and authority are rechecked before publication. SV-001 currently defeats the publication phase even though those checks exist.
- **Live TV/DVR:** Worker claims, source capacity, recording storage ownership, retention selection and retired-reader grace were reviewed in selected files. File deletion uses the storage boundary and durable metadata state. Full tuner/playlist/EPG interoperability, cancellation ordering and recording recovery were not executed.
- **Downloads/prepared media:** Revision fencing, bounded job selection, immutable copied original snapshots, short-lived grants, content rechecks, quota/storage-floor checks, prepared-worker custody locks and reader pins were sampled. A copied snapshot identifies the bytes actually copied rather than claiming a mutable library path is immutable.
- **Ingestion/metadata:** Inventory pages, source generation fencing, worker bounds, retry policy and publication-after-external-work were read. Metadata provider HTTP paths use timeouts/body limits/pacing; artwork has its own stricter transport. Provider correctness/language matching and all media-type import rules were not exhaustively checked.
- **Remote access/networking:** Selected UPnP gateway binding, mapping verification, route-publication proof windows and locality logic were inspected. These controls are meaningful but do not constitute a live NAT, DNS, certificate-renewal or Internet-reachability acceptance test.
- **Packaging:** Docker runs as an unprivileged user with tini and supplies a verified FFmpeg bundle. The supplied manifest verifier checks complete inventory, hashes, required provenance and symlink escape. The shell verifier explicitly checks codec/filter/protocol features. None of these scripts was executed. Native installer availability and signing/notarization acceptance are not established by a Go release builder alone.

## SQL and resource-cost review

Static reads considered predicate shape, result limits and supporting indexes rather than assuming `LIMIT` makes every query cheap. Observed examples: recommendation signal recent/work indexes; download work/viewer/running indexes; prepared-media work/item/version indexes; DVR due/owner/rule indexes; artwork due index. The download worker caps a selected work page at eight and retirement pages at 64; prepared worker inspects at most 64 pending jobs per pass. Several predicates still contain correlated checks or ordering expressions, so only a live planner/representative-data test can establish their actual cost. The unbounded profile overlay in SV-011 remains unbounded even with its primary key/indexes.

`persistence/planner_stats_test.go` includes explicit gate-fairness and statement-plan assertions; it was read as intended coverage only. No EXPLAIN output, fresh/large database timing, lock contention, memory profile or disk-pressure measurement is asserted. All tests remain unexecuted under the requested code-only constraint.

## Package coverage matrix

Legend: **D** = one or more important call paths were followed across callers/callees; **S** = selected source sections/types/queries read; **I** = inventory/search only, no sufficient substantive review to clear the module. D is not full package coverage. Counts include production `.go` files and test `.go` files separately; physical lines include comments. The exact selected paths are described above and in findings. Packages such as catalog/httpapi/metadata are much larger than their reviewed subset.

| Server package (relative to `server/`) | Production files | Lines | Test files | Review |
|---|---:|---:|---:|:---:|
| `cmd/apigen` | 1 | 45 | 0 | I |
| `cmd/devhash` | 1 | 17 | 0 | I |
| `cmd/server` | 16 | 2,348 | 13 | D |
| `internal/access` | 7 | 1,985 | 3 | S |
| `internal/administration` | 27 | 6,658 | 14 | S |
| `internal/apievents` | 1 | 71 | 2 | I |
| `internal/apispec` | 2 | 348 | 2 | I |
| `internal/artifactlease` | 5 | 64 | 0 | I |
| `internal/assets` | 11 | 1,195 | 5 | S |
| `internal/atomicfile` | 3 | 82 | 1 | I |
| `internal/audiofacts` | 2 | 532 | 4 | I |
| `internal/authoritygate` | 1 | 157 | 0 | I |
| `internal/backup` | 9 | 1,971 | 3 | D |
| `internal/buildinfo` | 1 | 12 | 0 | I |
| `internal/capabilityreport` | 1 | 100 | 1 | I |
| `internal/catalog` | 115 | 29,140 | 108 | S |
| `internal/catalog/scalebench` | 2 | 551 | 1 | I |
| `internal/catalogtest` | 2 | 856 | 2 | I |
| `internal/childprocess` | 5 | 169 | 1 | I |
| `internal/claimweb` | 1 | 70 | 0 | I |
| `internal/compactcatalog` | 24 | 6,622 | 16 | S |
| `internal/connectivity` | 2 | 383 | 1 | I |
| `internal/contentaccess` | 1 | 182 | 0 | D |
| `internal/dbwork` | 22 | 3,997 | 13 | D |
| `internal/decoder` | 26 | 3,241 | 23 | S |
| `internal/decodertest` | 1 | 57 | 0 | I |
| `internal/diagnostics` | 1 | 164 | 1 | I |
| `internal/diskspace` | 3 | 82 | 0 | I |
| `internal/downloads` | 14 | 4,166 | 8 | S |
| `internal/enrichment` | 1 | 831 | 1 | I |
| `internal/entityid` | 1 | 89 | 0 | I |
| `internal/eventfeed` | 1 | 213 | 2 | I |
| `internal/facettext` | 1 | 11 | 0 | I |
| `internal/hosted` | 15 | 2,359 | 12 | I |
| `internal/hostedtrust` | 6 | 171 | 3 | I |
| `internal/hostlimits` | 7 | 246 | 1 | I |
| `internal/httpapi` | 115 | 22,259 | 149 | D |
| `internal/httpapi/fixture` | 1 | 819 | 0 | I |
| `internal/identity` | 32 | 7,604 | 26 | D |
| `internal/ingestion` | 10 | 2,176 | 13 | S |
| `internal/linearbuffer` | 4 | 874 | 3 | I |
| `internal/linearinput` | 4 | 859 | 2 | I |
| `internal/livechannels` | 15 | 3,382 | 11 | I |
| `internal/livechannels/dvr` | 15 | 3,280 | 8 | S |
| `internal/livechannels/library` | 13 | 3,523 | 11 | I |
| `internal/localmetadata` | 7 | 1,068 | 4 | I |
| `internal/localonly` | 1 | 31 | 0 | I |
| `internal/lyrics` | 5 | 1,684 | 4 | D |
| `internal/mediaanalysis` | 15 | 3,172 | 8 | I |
| `internal/mediaartifact` | 3 | 803 | 2 | I |
| `internal/mediaexec` | 14 | 1,646 | 4 | S |
| `internal/mediaprobe` | 1 | 66 | 1 | I |
| `internal/mediasource` | 3 | 394 | 3 | I |
| `internal/mediatimeline` | 1 | 83 | 1 | I |
| `internal/mediatools` | 1 | 219 | 1 | I |
| `internal/metadata` | 70 | 16,402 | 39 | S |
| `internal/metadataprovider` | 17 | 3,270 | 10 | S |
| `internal/mounts` | 15 | 2,230 | 7 | I |
| `internal/networking` | 55 | 10,955 | 47 | S |
| `internal/notify` | 2 | 675 | 0 | I |
| `internal/operations` | 23 | 6,866 | 19 | I |
| `internal/persistence` | 13 | 3,666 | 23 | D |
| `internal/personalstate` | 1 | 57 | 0 | I |
| `internal/playback` | 54 | 13,259 | 41 | S |
| `internal/playback/vod` | 1 | 656 | 2 | I |
| `internal/playbackruntime` | 6 | 1,758 | 4 | I |
| `internal/playbackv1` | 26 | 8,191 | 7 | S |
| `internal/preparedmedia` | 9 | 2,248 | 5 | S |
| `internal/privateoutput` | 1 | 44 | 1 | I |
| `internal/probefacts` | 2 | 271 | 1 | I |
| `internal/producerinput` | 1 | 566 | 1 | I |
| `internal/recordingaccess` | 6 | 413 | 3 | I |
| `internal/recordingmedia` | 4 | 759 | 0 | I |
| `internal/remotemedia` | 6 | 1,567 | 3 | D |
| `internal/remotesources` | 9 | 2,074 | 3 | I |
| `internal/scanevents` | 1 | 237 | 3 | I |
| `internal/segmentmarkers` | 1 | 71 | 1 | I |
| `internal/servicelog` | 3 | 888 | 2 | I |
| `internal/social` | 12 | 4,322 | 4 | D |
| `internal/sorttext` | 1 | 58 | 1 | I |
| `internal/sourceaccess` | 1 | 193 | 2 | I |
| `internal/storage` | 54 | 5,266 | 19 | S |
| `internal/subtitles` | 22 | 4,540 | 14 | D |
| `internal/subtitlevideo` | 13 | 1,481 | 3 | I |
| `internal/supervise` | 3 | 340 | 2 | I |
| `internal/telemetry` | 19 | 1,793 | 3 | I |
| `internal/testauth` | 1 | 31 | 0 | I |
| `internal/testtier` | 1 | 53 | 1 | I |
| `internal/worker` | 2 | 282 | 2 | S |
| `internal/workpolicy` | 1 | 232 | 1 | I |
| `receiver` | 1 | 18 | 0 | I |

Additional source/build surfaces:

| Surface | Review | Specific scope / limit |
|---|:---:|---|
| `apikit` | D | Registry/access enforcement, bounded decoding, typed route/idempotency/cursor support sampled; embed inputs followed into release snapshot. Not every template/generated route audited. |
| `server/receiver/cast/receiver.js` | S | Credential persistence/reconnect and hardcoded controller/queue/playback calls followed for SV-009/SV-012; receiver rendering and real device playback not tested. |
| `Dockerfile` | D | Full stages and COPY/build input closure, runtime user/init/toolchain paths. No image built. |
| `scripts/build-server-release.py` | D | Snapshot selection, sibling capture, digest and artifact verification, signing/output paths; no script executed. |
| `scripts/container-release.mjs` | D | Input inventory, identity and manifest generation; no script executed. |
| `scripts/env.sh` | S | Toolchain/environment routing source read. |
| `scripts/verify-ffmpeg-manifest.py`, `verify-ffmpeg-bundle.sh` | S | Source read for inventory/hash/path checks and runtime qualification contract; no verifier executed. |
| `scripts/build-ffmpeg.sh`, `normalize-ffmpeg-bundle.py`, `requirements-release.txt` | I | Inventoried; full third-party source/build-chain audit not completed. |
| `server/internal/persistence/migrations/0001_baseline.sql` | S | Relevant constraints, indexes and recommendation triggers inspected; full 5,000+ line schema not exhaustively reviewed. |
| Fonts, receiver static assets, Go module manifests | S | Embedded-resource and local-dependency inclusion checked; fonts/vendor contents not audited. |
| Other scripts, trust material, vendored FFmpeg/HLS inputs | I | Presence/integration references only. No private material inspected; no dependency/CVE or license-compliance certification. |

## Remaining verification boundary

The report does not establish that the server currently builds, tests pass, a restore succeeds on any filesystem, playback works with real codecs/devices, a race is reproduced, the advertised capacity is met, a package has no other defects, or any production environment is secure. No dependency vulnerability lookup or current external documentation research was performed. Native installation/service registration artifacts, platform signing/notarization, network acceptance, load tests, database integrity against real user state, and media fixture fuzzing remain outside this static run.

Suggested repair order: SV-001 first (ordinary user actions disable writes); SV-002/SV-005/SV-006 together before relying on recovery; SV-003/SV-004/SV-010 before release promotion; SV-007/SV-008/SV-009/SV-012 together before Cast acceptance; SV-011 before large-profile capacity acceptance. Keep the report's regression suggestions as future work until execution is explicitly in scope.
