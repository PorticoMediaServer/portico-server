# Second adversarial review — server and ancillary findings

Date: 2026-09-25. Reviewer assignment: the first-pass Hosted reviewer independently challenges **SV-001–SV-012 and AX-001–AX-003**. All 15 assigned findings received a source-level challenge; no original report or application source was changed.

## Method, baseline, and verdicts

This pass examined callers, authorization and configuration gates, transaction/error semantics, alternate recovery paths, default reachability, and the risks of the proposed fixes. It was entirely static: no applications, tests, builds, database queries, network requests, deployments, or exploit attempts were run. Test source establishes intended behavior only. References are relative to `/Users/justin/Documents/Codex Projects/Portico`. For compactness, **S/** means `apps/portico-server/`; **R/** means `portico-recommendation-enhancement/`. All line ranges refer to current inspected source.

The original baselines remain authoritative: server split repository `820dde1`, internal repository `33ce964`, and per-file inventory hashes for the uncommitted recommendation repository. The primary reviewer reported an external internal-repository HEAD change to `8f412e4` while all 3,752 inventoried source hashes remained unchanged. This does not establish a new application baseline or represent a change made by this reviewer.

“Uphold” means the claimed mechanism and priority survive the challenge. “Narrow” means the mechanism survives but scope, trigger, consequence, or confidence needs qualification. No runtime occurrence is inferred from source alone. None of the assigned mechanisms was disproved; this is not an instruction to retain the original wording unchanged.

| ID | Verdict | Priority after review | Main correction |
|---|---|---|---|
| SV-001 | Uphold | P1 | SQL cancellation and DB-handle replacement do not release the wrapper-owned global writer gate. Ordinary reads are not all blocked. |
| SV-002 | Narrow | P1 | Fence unresolved recovery, while allowing continuation after a positively established clean rollback. Some authority errors already stop startup. |
| SV-003 | Uphold | P1 | Applies to the isolated release builder after prerequisites pass; working-tree builds do not falsify it. |
| SV-004 | Uphold | P1 | Clean Docker web-stage source closure is incomplete. |
| SV-005 | Uphold | P2 | DB-only manual selection exists but is not an equivalent state-file restore. |
| SV-006 | Narrow | P2 | Owner-authorized overlapping staging; not an unauthenticated race or proof every overlap corrupts bytes. |
| SV-007 | Narrow | P2 | Wrong/invalid-state credentials often roll back accounting; successful and some expired-known outcomes do persist it. No feasible brute force demonstrated. |
| SV-008 | Narrow | P2 | The relevant condition is a server principal with `hosted` authority, not merely visiting the Hosted web client. |
| SV-009 | Narrow | P2 | Lost rotation results can force manual re-pairing; do not call the receiver permanently unrecoverable. |
| SV-010 | Uphold | P2 | Source provenance is incomplete; artifact hashes/signatures remain independent integrity protections. |
| SV-011 | Narrow | P2; impact conditional | Work scales with distinct pending works. Read snapshots, deadlines and asynchronous projection exist but do not bound the synchronous overlay. |
| SV-012 | Downgrade | P2, from P1 | Configured Cast is broken, but the optional/default-disabled feature does not establish a universal release blocker. |
| AX-001 | Narrow | P2 | Conditional non-JSON transient response skips retry and pauses the batch; upstream frequency was not observed. |
| AX-002 | Narrow | P2; distribution conditional | Ordinary wheel execution is affected; documented editable installation can work. |
| AX-003 | Uphold | P2; concurrency conditional | Atomic output publication does not give separately read input collections one snapshot. |

After the severity correction, the server set contains **4 P1 and 8 P2** findings. The three ancillary findings retain P2 with the qualifications below. This pass changes wording and severity, not the number of surviving source mechanisms.

## SV-001 — Uphold P1, high confidence

**Challenge:** Could `database/sql` cancellation, a deferred rollback, a read-only follow-up, or the DB health watchdog release the writer permit? Are these merely unreachable maintenance helpers?

**Evidence:** `S/server/internal/subtitles/refresh.go:16–55`, `subtitles/provider.go:261–298`, and `lyrics/acquisition.go:190–264` acquire a second wrapper but defer/commit the first completed wrapper. `dbwork/write.go:74–116,210–247,269–301` acquires the global gate and makes completion of an already-finished wrapper a no-op. The release callback belongs to that wrapper, not the underlying SQL transaction. `dbwork/gate.go:16` supplies a stuck-wait diagnostic interval, not forced permit revocation. `dbwork/health.go:164–214` can replace DB handles but does not clear the package-global ownership. HTTP subtitle entrypoints are wired at `httpapi/subtitle_routes.go:165–186`; the lyric search entrypoint is present in `httpapi/lyrics.go`.

**Counterevidence and limits:** Extraction/provider success and the second authorized transaction must be reached. Authorization/source checks are real. General snapshot reads use the read path (`dbwork/read.go:11–20`) and are not all gated writes. However, subtitle `List` uses `s.tx` at `subtitles/service.go:237–248`, so refresh's follow-up can itself wait on the leaked permit. A context-cancelled SQL transaction does not call `Write.finish`.

**Corrected wording:** A successful authorized acquisition that enters publication can leave the global application writer permit held, preventing subsequent gated writes until restart. This is not a claim that every HTTP request or SQL reader stops.

**Fix invariants:** Keep slow extraction/network work outside the gate; retain current source and authorization checks inside publication. Scoped ownership of the second wrapper is appropriate. Force-releasing the gate on a watchdog timer could admit two writers while the original owner is still active and is not a safe substitute.

**Verification target:** Exercise every second-transaction exit and cancellation, then obtain an unrelated gated write within a short deadline. Inspect persisted state, not only returned candidates.

## SV-002 — Narrow P1, high confidence

**Challenge:** Could rollback, authority validation, the instance lock, or the startup error path prove that the current state is safe before serving it?

**Evidence:** `S/server/internal/backup/apply.go:187–235` writes `preparing`, moves live files, then changes the marker to `switching`. `writeMarker` at `484–489` overwrites with `os.WriteFile`; `moveIfPresent:448–457` does not provide durable directory ordering. Invalid marker/recovery errors return from `90–138`. Startup holds the instance lock when available but logs and continues after every `ApplyStaged` error (`cmd/server/main.go:253–287`), then opens authority and persistence (`298–327`).

**Counterevidence and limits:** `failPreparing:279–287` and `finishRollback:316–326` can successfully restore the original and still return the triggering error. Thus **not every non-nil error means unsafe live state**. `networking_initializer.go:31–88` refuses some combinations, notably missing authority beside an existing DB. It is not a complete restore disposition check: a remaining valid authority can be opened, and absence of the DB permits a new installation path. `persistence/schema.go:36–63` performs ordinary opening/migration rather than resolving the interrupted restore. The original backup is not necessarily lost.

**Corrected wording:** Non-atomic journal updates and unconditional startup continuation can expose an unresolved restore state. Startup must distinguish recovery that positively returned to a valid original/replacement from recovery whose state is unresolved; do not claim every restore error necessarily creates an empty installation.

**Fix invariants:** An atomic, durable marker and explicit recovery disposition are required. Making all restore errors fatal would unnecessarily reject clean rollback; deleting preserved state to force startup would destroy the recovery path. Failure injection must include marker-write/durability boundaries, not only named phase hooks (`backup/apply_crash_test.go:43–68,100–126`).

## SV-003 — Uphold P1, high confidence

**Challenge:** Are resources generated later, copied by another phase, or only used by non-built tooling?

**Evidence:** `S/scripts/build-server-release.py:44–52,118–142` copies the suffix/path-selected inventory into isolated server/apikit snapshots and builds from those snapshots. Required embed patterns remain in `server/internal/persistence/migrations.go:12`, `server/internal/mediaexec/fonts.go:24–28`, and `apikit/generate.go:17–21`; SQL, font/license and template files do not match that inventory. No intervening resource-generation/copy phase restores them.

**Counterevidence and limits:** The normal checkout contains the resources, and external release prerequisites can fail before compilation. Neither proves the isolated source snapshot complete. The embedded receiver is explicitly included, which shows the snapshot mechanism can include resources but does not cover these other resource classes.

**Corrected wording:** Once prerequisites pass, the supplied isolated release workflow cannot compile its current embedded-resource closure. This is a source-contract conclusion, not a recorded failed build.

**Fix invariants:** Hash and copy the same complete allowed module/resource closure. Replacing the filter with an unrestricted recursive workspace copy risks including private configuration, build products, or unrelated inputs. Verify all embed targets in the isolated tree before executing a later build.

## SV-004 — Uphold P1, high confidence

**Challenge:** Could npm, another Docker stage, type erasure, or a workspace mount provide i18n?

**Evidence:** `S/Dockerfile:31–47` supplies web, client-core, design and HLS source to the clean Node stage but omits i18n. `web/vite.config.ts:8` directly aliases that local folder. `packages/client-core/src/presentation/server-text.ts:7` and `web/src/screens/downloads/Downloads.tsx:3` import runtime values, so this is not an erased type-only reference. `npm ci --ignore-scripts` does not generate that sibling source. Later Go-stage copies cannot satisfy an earlier Vite build.

**Counterevidence and limits:** Local builds with the full checkout can resolve the import, and `.dockerignore` does not itself remove the package. A file being in the build context is insufficient when no COPY includes it. No live container build was executed.

**Corrected wording:** The checked-in Dockerfile's clean web build has an incomplete local source closure and cannot resolve required i18n runtime imports.

**Fix invariants:** Add the package to both copy and provenance inventories; do not stub translations or fetch a different package over the network. Verify in the actual isolated stage, not a host directory containing all packages.

## SV-005 — Uphold P2, high confidence

**Challenge:** Does manifest kind exclude the generated safety backup, or does another normal restore path recover all its state?

**Evidence:** `S/server/internal/backup/apply.go:195–218,550–581` includes the moved `portico.db` in `hashTree(pre)` and its manifest. `backup.go:427–461` lists these completed snapshots; `files.go:163–175` does not normalize away the database entry. Both ID and folder branches of `stage.go:152–197` reject an entry equal to `DatabaseName`.

**Counterevidence and limits:** The bare-file branch (`stage.go:199–202`) permits manually selecting the database. That leaves associated authority/state-file restoration out of the operation and is not an equivalent safety-snapshot restore. The finding does not assert destroyed original bytes. Existing list/prune fixtures do not model the full round trip.

**Corrected wording:** Generated pre-restore snapshots are listed but cannot be restored through their manifest-based ID/folder path because their own database entry is rejected.

**Fix invariants:** Retain explicit DB integrity coverage. Do not merely drop its hash; define consistent database/WAL/state-file roles without copying reserved database entries twice. Preserve compatibility with already-written manifests and verify the complete A→B→A→B recovery cycle.

## SV-006 — Narrow P2, high confidence

**Challenge:** Are requests unauthorized, serialized by admission, fenced by restart, or already deduplicated by operation ID?

**Evidence:** `S/server/internal/httpapi/backup_routes.go:138–154` validates an operation ID then calls `Stage` without it. `backup/stage.go:58–147` shares `.incoming`, `.staged-complete`, and staged paths, with removal/copy/inspection/rename operations but no per-operation exclusion. `httpapi/route_lanes.go:301` selects the admin-heavy lane, whose capacity is four (`admission.go:64–76`), not one. Admission is a concurrency semaphore (`350–385`). The stage callback waits two seconds before shutdown (`cmd/server/main.go:911–919`) and does not serialize requests already copying.

**Counterevidence and limits:** The route requires Owner and the registry enforces that before the handler (`apikit/registry.go:149–168`). This is an owner-initiated overlap/retry failure, not an unauthenticated filesystem attack. Backup creation's service mutex does not protect this separate Stage path. Sequential replacement of an older completed stage appears intentional. Overlap can cause deletion/rename failures or disagreement between inspected and later staged input; a particular corrupt byte outcome was not reproduced.

**Corrected wording:** Two authorized overlapping stage operations can interfere through shared temporary paths, and the accepted operation ID does not supply deduplication or result identity.

**Fix invariants:** Use operation-owned directories and serialize final stage publication, with a durable identity/digest receipt. Preserve explicit sequential supersession and unresolved recovery journals. Holding the global database write gate during large file copies would introduce a different availability defect.

## SV-007 — Narrow P2, high confidence

**Challenge:** Are all attempt counters actually discarded, or do the HTTP edge, strong credentials, or successful outcomes provide meaningful alternative controls?

**Evidence:** `S/server/internal/social/cast.go:220–261` increments the rate bucket inside the same transaction as credential lookup; unknown/invalid-state returns then execute deferred rollback. Reconnect does the same (`358–395`). `social/store.go:247–269` has no independent accounting commit. Cast is not in the dedicated authentication-rate dispatch (`httpapi/router.go:1266–1272`); its default lane (`route_lanes.go:61–62`) limits concurrent work, not attempts per hour.

**Counterevidence and limits:** Successful requests commit their counts, and an expired known reconnect credential at `cast.go:392` commits as well. Cryptographically generated code/token entropy, short TTLs, TLS/origin checks and request deadlines remain meaningful controls (`store.go:166–185`; `router.go:1224–1249`). CORS does not prevent a direct nonbrowser client from submitting allowed HTTPS requests. No practical guessing success or exhaustion threshold was demonstrated. The single-wrong-code assertion at `social_routes_test.go:929–933` does not exercise persistent failed-attempt accounting.

**Corrected wording:** The intended persistent failed-attempt budget is bypassed for wrong and several invalid-state credentials because its write rolls back with the refusal; this does not establish feasible token brute force.

**Fix invariants:** Blindly committing the entire failed credential transaction can persist partial credential mutations. Committing every failure into only the existing global bucket could let unauthenticated callers lock out all legitimate pairing. Keep independent bounded accounting, source/device isolation and global capacity protections; test each refusal outcome separately.

## SV-008 — Narrow P2, high confidence

**Challenge:** Could a caller guard exclude Hosted authority, or could current Hosted policy be supplied to issuance implicitly?

**Evidence:** `S/server/internal/httpapi/social_routes.go:135–165,779–793` obtains the normal principal and exposes bootstrap without a local-only check. `router.go:593–638` validates current identity/library/feature access, but `restriction_admission.go:16–54` adds no Cast authority restriction. `social/cast.go:139–187` stores `p.Authority`; redemption commits before `castSession` (`279–286`). `identity/service.go:207–223` rejects every non-local authority in `IssueWithDevice`; the Hosted policy-aware `IssueTx` is a different boundary.

**Counterevidence and limits:** A Hosted web origin does not alone imply the server principal is Hosted. Correctly authenticated local principals can pair at the identity layer. Application-ID configuration determines whether normal clients advertise Cast; `social_routes_test.go:34–47,914–998` exercises local identities only. Hosted policy validation at ordinary principal resolution does not pass a verified current policy horizon into this later issuance call. The identity refusal is fail-closed and correct.

**Corrected wording:** With configured Cast and a server principal whose authority is `hosted`, bootstrap accepts and redemption consumes a code before its session-issuance helper rejects that authority. Do not generalize to every user of the Hosted website.

**Fix invariants:** Preserve policy horizons, epochs, profile restrictions, device scope and revocation checks; do not relax `IssueWithDevice` to admit Hosted blindly. Either explicitly refuse before bootstrap or issue with verified policy in a transactionally consistent consumption operation. Recheck policy on reconnect as well.

## SV-009 — Narrow P2, high confidence

**Challenge:** Is recovery permanently impossible, and do all post-response receiver errors lose the replacement token?

**Evidence:** `S/server/internal/social/cast.go:294–321` replaces the only token digest. Reconnect looks up that digest, commits rotation, then issues the session (`358–408`). There is no operation-bound result replay for a response lost after commit. The receiver cannot store a token it never receives (`receiver/cast/receiver.js:173–207,240–251`).

**Counterevidence and limits:** New bootstrap/manual pairing can recover the receiver; “permanently stranded” overstates the impact. Existing access may remain useful until its own expiry. Once a successful response is received, `adopt` saves the token **before** `prepareController`, so the obsolete-playback failure in SV-012 does not itself imply replacement-token loss. `social_routes_test.go:972–980` intentionally rejects the old token after rotation but does not test a lost response.

**Corrected wording:** Loss of a committed reconnect result can prevent automatic recovery with the previously stored token and force manual re-pairing. Do not imply all playback stops immediately or that a receiver cannot ever be repaired.

**Fix invariants:** Atomic issuance plus rotation fixes issuance failure but cannot alone fix response loss. An operation-bound, bounded replay result must preserve device/account revocation, expiry and replay constraints. Indefinitely accepting the old token weakens theft recovery and is not an acceptable shortcut.

## SV-010 — Uphold P2, high confidence

**Challenge:** Is apikit included transitively in the digest, or do artifact hashes already cover the same claim?

**Evidence:** `S/scripts/build-server-release.py:17–26` adds sibling source for the Hosted module case, not server apikit. `118–142` copies and builds the sibling; `server/go.mod` uses the local replacement. Initial copy checks do not incorporate it into subsequent root digest verification. `scripts/container-release.mjs:25–30` similarly excludes apikit while `Dockerfile:54–60` compiles it.

**Counterevidence and limits:** Artifact SHA/signature verification (`build-server-release.py:71–93`) still authenticates the produced bytes. Native build IDs include fresh identity/time, so do not claim an apikit edit necessarily leaves the native BuildID identical; the source digest is the incomplete value. Container BuildID derives from that incomplete inventory. SV-003/SV-004 currently block normal release paths earlier; this is a latent provenance defect once those build failures are repaired, not evidence a bad artifact has already shipped.

**Corrected wording:** The source identity omits a compiled local dependency, so it cannot attest the complete source closure or detect later captured-sibling changes. This is distinct from forged binaries passing artifact-integrity checks.

**Fix invariants:** Use a canonical sorted input manifest for capture, hashing and final verification across all compiled modules/resources. Keep source identity, toolchain identity and byte-integrity claims explicit. A fix that only adds apikit while leaving i18n or embedded resources outside the closure is incomplete.

## SV-011 — Narrow P2 mechanism; operational impact conditional

**Challenge:** Does readiness cap pending jobs, does the read snapshot bound history, does the worker always finish first, or is growth proportional to every viewing event?

**Evidence:** `S/server/internal/catalog/rec_engine.go:175–232` selects all pending profile jobs and performs per-work signal/stored/facet reads before request memo construction (`377–405`). `catalog/compact_listening.go:6` delegates readiness to `compactcatalog/derive.go:124–138`, which checks catalog-derivation rebuilding for domains 18 and 32. It neither examines nor caps `rec_profile_jobs`. `compactcatalog/derive.go:384–406` and `rec_profile.go:192–224` asynchronously process bounded pages; they do not establish catch-up before request execution.

**Counterevidence and limits:** The schema primary key is `(profile_id, work_id)` (`persistence/migrations/0001_baseline.sql:769–773`), and producers upsert revisions (`5105–5143`). Repeated events for one work do not append arbitrary numbers of jobs. The relevant N is **distinct pending works**, bounded ultimately by the profile's touched catalog population. Existing taste/engaged sets are also materialized, but that does not justify describing every history event as a separate pending job. Home composition uses consistent read snapshots (`httpapi/home.go:152–171`); catalog reads propagate context (`catalog/reads.go:39–44,64–95`) and admission sets deadlines (`admission.go:64–76,382–385`). These prevent mixed read state and unlimited request lifetime, but a snapshot does not cap row count; it can keep seeing a backlog even after the worker drains it elsewhere. The normal background worker reduces exposure. No benchmark establishes latency or frequency.

**Corrected wording:** When many distinct works remain pending for a profile, recommendation requests synchronously materialize and replay that entire pending set. Work is not bounded by requested result count and can consume the request budget; actual scale and impact require representative measurement. This is not a data-race or proven ranking error.

**Fix invariants:** Immediate personal feedback is intentional: `catalog/rec_engine_test.go:153–178` checks hides/watched behavior before asynchronous materialization. Blindly adding LIMIT or serving a stale full projection can reintroduce hidden/disliked results. Separate immediate exclusion/authority fences from bounded ranking updates, retain revision/watermark semantics, and preserve responsive reads. Verify statement count and memory against distinct-work backlog sizes, not merely repeated events on one work.

## SV-012 — Downgrade to P2, high confidence in configured failure

**Challenge:** Is there a legacy adapter, a negotiated API version, a server fallback, or a default-wide failure warranting P1?

**Evidence:** `S/server/receiver/receiver.go:14–18`, `httpapi/social_routes.go:875–885` and `receiver/cast/index.html:119` ship the actual receiver script. `receiver.js:230–245` unconditionally initializes v2 controllers/lanes after the existing profile-publication call. Queue/occurrence/control paths remain v2 (`144,365–377,395–420`). The request helper simply constructs the path (`173–205`). Current handlers are the v1 contracts (`httpapi/playback_v1_routes.go:234–301`); no production v2 translation route was found. Installation fallback (`httpapi/installation.go:14,38–46`) does not adapt them. A POST can return method-not-allowed rather than 404, so “all calls 404” would be inaccurate.

**Counterevidence and limits:** Cast application ID is optional/default empty (`social/cast.go:49–52`), and normal web discovery requires configuration plus the secure-context path (`web/src/bridge/cast.ts:51–68`). No live configuration or product rule requiring Cast for every release was established. Local pairing can succeed independently, and receiver token persistence precedes controller preparation. Static asset tests (`social_routes_test.go:1000–1009`) demonstrate served bytes, not protocol compatibility.

**Corrected wording:** When the embedded receiver is used for configured Cast, it cannot initialize/play/control against the current server because it requires removed APIs. This affects local as well as Hosted accounts and is independent of HS-009/SV-008. Classify P2 as a complete optional-feature failure; a documented release-wide acceptance requirement could justify later promotion to P1.

**Fix invariants:** Port the complete capability, queue/session/timeline, intent and renewal contract. Renaming URL prefixes cannot fix changed semantics, and resurrecting an unreviewed legacy adapter may bypass current device/authority rules. Verify actual receiver payloads against route schemas, then device acceptance only when execution is authorized.

## AX-001 — Narrow P2, high confidence in conditional branch

**Challenge:** Does the HTTP adapter already retry, does the outer layer recover, and is non-JSON provider behavior established?

**Evidence:** `R/src/portico_enrichment/similar_lookup.py:698–730` reads status, then requires JSON before transient-status classification and Retry-After handling. With the default one allowed retry (`372–386`) and default session (`410–415`), a transient non-JSON body reaches the immediate JSON error branch instead of the intended retry. There is no configured session adapter retry that repairs the default path.

**Counterevidence and limits:** Network exceptions and valid JSON transient/maxlag responses do retry. With retry count zero, no retry is intended. The outer catch at `500–504` records `api_error`, marks the report paused and stops that search batch; `630–638` returns the report. This is not necessarily a process-wide uncaught exception or false identity assignment. The maxlag test at `tests/test_similar_lookup.py:238–257` uses valid JSON. No provider requests were made, so describing non-JSON failures as empirically “common” is unsupported.

**Corrected wording:** If a transient HTTP response has an empty/non-JSON body, the configured retry is skipped and lookup pauses with an API error, requiring later recovery/rerun. Frequency is unknown.

**Fix invariants:** Classify transient HTTP status before requiring JSON, while preserving JSON-only maxlag detection on successful HTTP status, bounded retries, Retry-After caps and request pacing. Do not retry malformed successful content indefinitely. Add fake-response cases; none were executed in this audit.

## AX-002 — Narrow P2, high confidence; distribution use conditional

**Challenge:** Is the file packaged implicitly or is checkout-relative execution the supported installation model?

**Evidence:** `R/pyproject.toml:1–23` discovers Python packages under `src`, with no package-data/data-files rule for root `schema/transport.sql`; no alternative MANIFEST/setup resource declaration was found. `src/portico_enrichment/transport.py:14,36–51` computes the repository-relative location from `__file__` and reads it during initialization. A conventional installed wheel does not retain that checkout layout/resource location.

**Counterevidence and limits:** `README.md:47–56` directs users to local setup, and `Makefile:8–9` installs editable source. That path can work. Existing-artifact validation does not need schema initialization, and not every CLI operation invokes transport creation. The relevant build entrypoint calls the builder (`src/portico_enrichment/pilot.py:535–541`); this is not proof that an ordinary deployed wheel workflow currently exists or has failed.

**Corrected wording:** The transport initializer/build path is not portable to an ordinary wheel-only installation because its SQL resource is neither packaged at, nor resolved from, an installed resource location. Documented editable checkout execution is unaffected by that specific mismatch.

**Fix invariants:** Package version-matched SQL inside the module and load it through an installed-resource API that also works editable. Keep explicit user-provided vocabulary/input files distinct from packaged runtime resources. Verify a clean wheel environment outside the checkout after changes; no wheel was built here.

## AX-003 — Uphold P2, high confidence; concurrent input mutation required

**Challenge:** Could SQLite connection contexts, immutable source revisions, command serialization, or atomic output replacement already make the input coherent?

**Evidence:** `R/src/portico_enrichment/transport.py:146–149` independently calls the three fetch functions. `storage.py:146–157` opens ordinary SQLite connections without a shared explicit read transaction. `fetch_candidates:467–482` materializes candidates then performs provider-ID lookups; decisions (`484–495`) and sources (`520–523`) use separate connections. Candidate/provider writes (`227–290`) and decision writes (`387–432`) can commit between those reads. The pilot builder (`pilot.py:535–541`) does not freeze the ledger or enforce exclusive command execution.

**Counterevidence and limits:** Source revisions and decision metadata retain historical evidence, but the export reads current candidate/provider facts and separately read decisions/sources; those records do not imply all collections came from one point in time. Even using one SQLite connection context without an explicit read transaction would not establish the desired snapshot. Output construction/validation and atomic replacement (`transport.py:241–283`) protect the destination artifact from partial publication; they do not reconcile input epochs. This requires concurrent mutation and is not proof any published dataset was affected or that SQLite itself corrupts data.

**Corrected wording:** Concurrent ledger updates can cause transport export to combine candidate/provider facts, decisions and provenance read at different commit boundaries, yielding a coherent-looking artifact that never existed as one input snapshot.

**Fix invariants:** Materialize all required input on one connection under one explicit read transaction (including provider IDs), then end the read snapshot before expensive output creation/VACUUM where possible. Merely sharing a connection is insufficient. Record snapshot identity, but distinguish point-in-time consistency from the separate policy question of whether every decision must bind exactly to the candidate revision it reviewed. A long exclusive write lock through export would unnecessarily block acquisition.

## Completion and remaining proof limits

All 15 assigned findings were checked. The main false-positive controls examined were owner/access enforcement, principal authority, policy horizon, request admission and deadlines, context cancellation, wrapper ownership, recovery rollback and authority opening, optional configuration, token persistence order, read snapshot isolation, unique job keys, asynchronous worker progress, clean build input boundaries, wheel versus editable installation, outer error handling, and atomic artifact publication.

This is a second targeted challenge, not a second complete audit of every server/ancillary package. It does not establish runtime exploitability, provider response frequency, production Cast configuration, real restore crash/power-loss behavior, release success/failure logs, representative performance, or actual concurrent-export corruption. The original reports' broader package coverage and omissions remain applicable. Proposed verification is deliberately unexecuted under the user's code-only instruction.
