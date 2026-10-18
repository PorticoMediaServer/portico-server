# Outcome

All 32 assigned findings have been read in the register, domain reports, fix log and second review, and checked against available source. The server now logs unknown legacy faults as 500 responses, preserves resume after an early known-duration end, falls back to the configured completion percentage when credits are unavailable, bounds backup snapshot pressure, fetches release feeds through the shared network policy, delivers large broadcasts with resumable checkpoints, and prevents future offline timestamps from fencing later activity. Development migrations have been folded into the baseline. CI actions and container inputs are pinned, vulnerability checks are added, and hostile-input coverage is stable per route. Native-only gaps and external document corrections are listed below, not claimed complete.

Executed on personal mini-1 (`ehler-homelab-mini-1`), in this lane's worktree. Context refresh was attempted and failed on its read-only lock directory; existing project, standards and verification guidance was loaded. No release, push or deployment was run. Snapshots 001 and 002 were taken after compiling; final cumulative snapshot 003 records the finished work.

# Decisions I made

- Preserve accepted plain HTTP, light password/no-lockout, unencrypted-state and TV-code role policies. No cryptographic device-key feature was inferred from an inaccurate document.
- Known-duration terminal reports must be within one second of the end (or 1% on a short title), or satisfy normal percentage/approved-credits policy. Unknown-duration natural ends remain accepted. This guards early EOF without making credits completion require 100% playback.
- Credits mode uses an approved current-source marker when available; otherwise it uses the configured percentage, following Plex. Unapproved guesses cannot complete early.
- Preserve registry response guards. Do not invent required values or send oversized pages after a committed mutation. Add conformance fixtures and reject null request documents before handlers instead.
- A consistent online backup may pin a snapshot for at most two minutes and add at most 128 MiB to the source WAL. It aborts rather than retaining checkpoint pressure indefinitely; retry at a quieter time. Existing backup publication remains atomic and does not list incomplete copies.
- The owner's configured release-feed origin is explicit permission to fetch it, including a private origin. Metadata addresses remain blocked. DNS answers are pinned, redirects stay within that origin, and TLS cannot downgrade. No new approval screen is required.
- Broadcasts run in pages of 50, rechecking authorization and releasing the writer between pages. Cursor/count checkpoints survive interruption for fourteen days. The initial last profile bounds the walk; new profiles are not promised a notice. The final response shape and 50-ID sample remain unchanged.
- Offline reports within the existing 24-hour skew tolerance are clamped to server receipt time before ordering/history writes. Larger future skew remains refused per entry. This preserves usable evidence without allowing a future watermark to beat tomorrow's activity.
- Pre-release databases with the previous baseline digest require reset. Historical migration/backfill compatibility is removed as the programme requires.
- Keep the bounded 30-minute/1-GiB timeshift default. Its actual rewind span is reported; 30 minutes is a ceiling, not a guaranteed span at every bitrate.
- Native files/workflows are outside both writable worktrees. Their necessary changes are specified below; no files in the main native checkout were edited.

# Bugs

- B-08.2 — Fixed. `httpapi.failure` defaults to logged `500 internal`; caller-validation errors use `requestinput`, including wrapped errors, legacy JSON decoders and query/service validators. Internal stored-document, dependency and database faults remain server errors. The full per-route hostile-input battery and dedicated logging/redaction-to-response tests pass.
- B-08.9 — Not present as an identified production response defect. Guards retained per V01; required-list, nullable-pointer, omitted-list and oversized-response conformance fixtures added and pass. Root-null requests are now refused before registered and legacy handlers. Missing optional fixture dependencies were corrected rather than weakening guards.
- B-08.11 — Fixed in writable documentation. `server/api/README.md` now distinguishes optional toolkit packages from actual catalogue cursor, family-specific receipt and revision behavior. Current catalogue cursors already retain their traversal position across expiry/revisions and refresh totals (`catalog/discovery.go`); the original expired-cursor rejection is not present. External architecture wording still needs correction.
- B-09.30 — Fixed. `backup/copy.go` bounds pinned snapshot lifetime and WAL growth; pressure and timeout tests pass, including immediate WAL truncation after abort.
- B-11.12 — Already fixed in `recordingmedia.Driver.Recover`: retained captures resume while the original live window remains open, increment gap evidence, and publish explicitly partial recovery. Sealed objects are validated/published. No real-tuner restart acceptance was run.
- B-11.21 — Already fixed in `recordingmedia/storage.go`: traversal is under `measureMu`, not the capture `writeMu`; counters are reconciled briefly and concurrent writes counted conservatively. Hard-linked recording objects count once. Disk latency/drop acceptance remains a device test.
- B-11.35 — Not changed: a deliberate bounded-storage tuning choice, not a false client window. `playbackruntime/linear.go` retains 30 minutes/1 GiB, with actual available span supplied by the buffer. High-bitrate streams legitimately retain less time.
- B-12.8 — Fixed. Early known-duration EOF retains resume below completion policy; natural end, approved credits and percentage completion remain usable. `TestEarlyEndKeepsResumeForKnownDuration` passes.
- B-12.9 — Fixed. Credits mode falls back to percentage without an approved current marker; marker/no-marker/unapproved fixtures pass. `server/api/home.md` updated.
- B-12.10 — Already clarified in `packages/i18n/src/catalog/en-US/preferences.ts`: music uses the displayed profile percentage; video inherits the library percentage until an explicit override; audiobooks finish near the end. Existing per-library/profile precedence retained and focused preference tests pass. No screen rendering claim made.
- B-12.22 — Already fixed in `catalog/personal_activity.go` and `play_history.go`: shared counts require unpaused history and the activity opt-in; owner history and personal counts remain separate. Opt-out/history-pause focused tests pass.
- B-13.3 — Already fixed in shared `GroupFollower`, independently verified and extended with native-style simulations at 1.2, 2, 5, 12 and 19 seconds of seek latency and a no-op rate setter. All converge within the 750-ms band, issue at most three seeks over 120 seconds and never supersede a pending seek. Existing failed-seek, unknown-seek-state, host-clock-change and end-room tests also pass. Players lacking completion/duration evidence retain bounded waiting rather than guessed compensation. Physical acceptance remains required.
- B-13.18 — Not finished in screens/engine adapters. Shared `GroupPlayer.load` already passes desired initial playing state, but `web/src/app/together.tsx` ignores it; native initial preparation also needs a guaranteed paused engine start. Exact integration below; no server route can prevent an adapter's audible first frame.
- B-13.22 — Fixed for large fan-out in `operations.BroadcastNotice`: paged, fenced and resumable, with a 522-profile interruption/replay test. The Hosted-audience subclaim is not present in current admission: `identity.PorticoSignInTx` resolves a local membership and calls `directSignInTx`, using local profile inboxes. Dormant Hosted-authority device helpers do not establish a current Hosted-issued viewer path.
- B-14.14 — Server/non-screen work already present; native cleanup remains incomplete in this writable scope. Downloads survive server tidying and shared service/usage enumerate durable claims. The prior downloads lane reports privacy-safe other-profile space cleanup; its sign-out retention decision and device-wide settings remain explicit. Actual device cleanup/sign-out behavior must be completed/verified in the native worktree.
- B-14.20 — Not finished in the native platform. Durable server requests and `DownloadsService.followDevice` already recover claims on the next app run; the downloads lane reports Wi-Fi/retry/service-timeout improvements. Automatic recovery while the Android process is dead still needs an installation-owned platform job. No native files are writable here.
- B-14.21 — Partly fixed. Server future timestamps are clamped before comparison/history/watermark writes; a regression proves they cannot replace later online resume. Device receipt expiry still uses its wall clock through `receiptUsable`; monotonic persisted receipt-clock integration is not finished outside this server worktree.
- B-15.7 — Documentation error; writable identity guide now explicitly says bearer credentials have installation/family association, not private-key proof. The external architecture's “registers a key” promise needs correction. Accepted HTTP policy retained.
- B-15.9 — Not changed: the explicitly accepted no-lockout trade-off. Current factor challenges still have attempt bounds. No stricter lockout or forced factor policy added.
- B-15.10 — Already fixed in `identity.takeHashSlot`: three-second bounded, cancellable wait, no writer held during hashing/admission. Existing focused admission/password/PIN tests are included in final verification.
- B-15.13 — Not changed: programme expressly says a TV code confers the approver's role. Existing device proof and primary/subordinate profile admission bounds remain. Role-identifying approval copy belongs to client screens.
- B-15.14 — Already fixed transport: `POST /v1/auth/remembered-accounts` and `IdentityClient.rememberedAccounts` use a body rather than query string. Corrected the inaccurate 256-bit entropy claim in source comments/identity OpenAPI prose; grammar/length validation does not prove randomness. Descriptor lookup remains intentionally unauthenticated and reveals no credentials.
- B-15.17 — Fixed. Release feed uses `remotemedia.Policy`/client with bounded time/body, pinned addresses, metadata-service blocking and origin-confined redirects. `safeUpdateURL` already restricts rendered release links to credential-free HTTPS. Private configured feed, metadata and redirect tests pass.
- B-15.19 — Partly fixed. Remaining tagged checkout/setup-go uses are pinned; Docker base manifests are digest-pinned after hash verification against Docker Registry responses; pinned govulncheck and npm vulnerability jobs added. Release key generation was already forbidden by `build-server-release.py`. OS installer signing is still a separate credential/qualification step. Live npm scan found four high advisory paths caused by `source-map-js` with no published fix; no blind dependency substitution made.
- B-15.20 — Already fixed server/API: `metadata/runtime_settings.go` stores owner TMDB/TheTVDB overrides, returns presence only, and hot runtime settings select those credentials. A key can be replaced without a release. Console integration/visibility needs client verification; public application credentials remain the intentional default.
- B-25.10 — Not finished: Apple/Vega release-build and physical acceptance gates are in the native repository, outside writable scope. Native CI's source omission was confirmed read-only. Existing bridge tests do not prove device delivery; exact workflow/acceptance work below.
- B-25.12 — Skipped as instructed: component-specific Apple distribution rights/licence review is not code and cannot be settled by a runtime test.
- B-25.14 — Fixed in writable API guide and extended the SQL guard to prepare the private Live TV spool against its own actual literal DDL: literal-only SQL preparation scope and the separate responsibility for runtime query constructors are explicit. `statements_prepare_test.go` already documents that boundary. External `02 Server.md` claim needs narrowing.
- B-25.15 — Fixed. Every route receives every hostile body, every hostile query and every hostile wildcard separately, under both auth modes. Each request has a bounded deadline and a cancellation backstop. 503 is explicitly permitted for optional fixture subsystems/admission. No route-index rotation or exhaustive cross-product claim remains. The final authenticated-canary battery passes (11.5 seconds), verifies current/everywhere sign-out renewal and uses an injected empty tuner discovery result.
- B-25.18 — Fixed. Migrations 2–11 folded into baseline; obsolete migration SQL and data backfills removed; one-step schema guard added, schema version 1 and digest/known objects synced. Fresh/reopen/index checks pass.
- B-25.20 — Already fixed in Linux maintainer scripts, matching FIX-LOG: preremove distinguishes upgrade/removal; postinstall starts fresh install, restarts running upgrades, preserves stopped/disabled state and honors Debian policy through deb-systemd-invoke. Portable packaging fixtures pass. Actual DEB/RPM upgrade acceptance not run.
- B-25.21 — Not finished: native and Roku sibling checkouts still omit dependency refs, confirmed in read-only source. Those workflows and dependency locks are outside writable scope; no server-side dispatch pretending to validate a moving client branch was added. Roku is excluded from this programme.

# API changes

No route added, removed or request/response field changed. Generated registry remains authoritative; apigen is run after changes.

- Download request policy refusals now use the registered `not_permitted` mapper, malformed file-preview title IDs return `invalid_request`, and administration cancellation/deadlines return retryable `administration_unavailable`.
- Legacy failures previously defaulted to `400 invalid_request` with no log; unknown causes now become logged `500 internal`. Explicit input errors remain 400 and known typed refusals retain their codes.
- Registered JSON requests refuse root `null` as `400 invalid_request` before calling handlers. Nullable nested fields and response pointers retain their semantics.
- Broadcast responses retain the existing audience/count/revision/dedupeKey/IDs fields; retries with the same operation/payload resume committed pages.
- Playback/offline evidence completion and skew semantics changed as described above, with unchanged wire shapes.
- Identity remembered-account lookup was already POST/body on this base; no GET compatibility route added.

# Schema changes

- Folded final schema from development migrations 2–11 into baseline 1. Preserved Cast recovery fields/index, library order, show settings, play history/count tables, show facet triggers, TMDB default trigger and positive recommendation-seed index. Removed reaction and unused featured index, plus all development data backfills.
- Added `notice_broadcast_work`, indexed expiry and `direct_profiles_broadcast(deleted,account_id,id)` for bounded resumable fan-out. Its IDs field is only the existing bounded receipt sample (at most 50); recipient traversal and counts are typed columns, not a serialized job graph. Maintenance removes up to 100 expired jobs per checkpoint.
- No migration or compatibility shim added. Schema-sync refreshes digest and known-object records. A changed development baseline returns the reset-required error before comparing its old high schema version; dedicated old-baseline/newer-schema/drift tests pass.

# Owner or viewer settings added or wired

No new settings. Existing completion policy now takes effect consistently without an approved marker; existing library/profile percentage precedence retained. Existing metadata credential overrides were verified wired server-side. Timeshift's original bounded defaults retained.

# Needed from others

1. **Web/native Together adapters:** honor `GroupPlayer.load(itemId,position,playing)` before media starts. Guarantee initial pause in the engine's prepare/start contract, including mpv/native players; a later pause after `play()` is insufficient. Supply real seeking/buffering and duration to the follower. Test paused/preparing room load with headphones and no audible first frame.
2. **Native downloads:** complete/verify privacy-safe cleanup of files owned by the exact signed-out account, retaining other inactive profiles unless the viewer explicitly reclaims their space. Show device-wide setting scope. The previous lane's intentional sign-out-retention decision needs programme integration resolution rather than an unreviewed device deletion here.
3. **Android downloads:** use built-in JobScheduler (own platform code, no WorkManager dependency) for durable installation-owned recovery after process death, fenced by current authorization, Wi-Fi policy and available storage. Ensure dataSync `onTimeout` pauses retained bytes, stops the service and schedules safe retry. The server/shared service requires no new route.
4. **Receipt clock:** native offline registry needs a persisted trusted server-time anchor from verified receipts/revalidation plus platform monotonic elapsed time, with restart/boot handling and revalidation able to repair an ahead wall clock. Route local playback eligibility through that clock; do not turn deliberate expiry policy into a DRM feature. Server future-stamp fencing is complete here.
5. **Native CI:** add Apple iPhone/iPad/tvOS release compilation on the Mac runner and Vega release-package build/bridge tests on Ubuntu. Keep public readiness behind auditable physical auth/load/playback/seek/subtitles/audio/focus/update evidence for each supported platform. No Xcode, Gradle, Vega or release build ran here.
6. **Cross-repository CI:** record exact server package and native revisions in native's dependency lock; checkout that SHA, compare copied packages against it, and update the lock deliberately. Any server-to-native dispatch must name exact server/native SHAs. The read-only native `ci.yml` currently follows server default; Roku's counterpart is outside programme scope.
7. **External documents:** `Project Architecture/02 Server.md` must stop promising a registered device private key, uniform toolkit receipt/cursor implementation or preparation of every dynamically assembled SQL statement. Saved-state completion text needs credits fallback and early-EOF policy; native download contracts need the eventual sign-out and monotonic-clock policy. Those source directories were read-only.
8. **Schema integration:** use this folded baseline as the final schema, merge other lanes’ localized baseline changes onto it, and run schema-sync after integration. Do not restore migrations 2–11 or their development-data backfills.
9. **Security/release:** resolve `source-map-js` advisory GHSA-68fv-2mgg-jv7q before accepting a green JavaScript vulnerability gate; OS signing credentials and exact signed package acceptance are still required wherever signed installers are promised. This lane neither generated keys nor released packages.

# Test notes for other environments

Verified so far:

- Linux `GOFLAGS=-p=2 go build ./...` passes after baseline folding and implementation.
- Focused catalogue completion/preferences/privacy tests pass.
- Shared `group-session.test.ts`: 32 tests pass, including the five new 120-second simulated native seek cases, without a device or rate nudge.
- Apikit registry/request/conformance focused tests pass.
- HTTP failure/logging and complete per-route hostile-input battery pass (11.5 seconds with the final authenticated canary).
- Broadcast: ordinary delivery plus 522-profile interruption, checkpoint, changed-intent refusal, continuation, no duplication and receipt replay pass.
- Backup: WAL-growth abort releases the snapshot and permits immediate TRUNCATE; snapshot timeout passes.
- Release-feed origin/metadata protection and bounded private configured feed tests pass.
- Offline future-clock regression passes.
- Schema-sync checks pass; portable packaging test file passes (2 fixtures).
- `npm audit --audit-level=high` ran: four high dependency paths, one root `source-map-js` advisory, no fix reported. Scanner fails rather than hiding it.
- `go vet` on all changed server packages passes; apigen completed with no generated shape changes.
- Windows amd64 and macOS arm64 `go build ./...` cross-compilation passes. This is server compilation, not Apple client/device acceptance.
- Production router construction, overlapping-pattern guard and error-envelope tests pass.
- Existing bounded password/PIN hashing admission tests pass.
- Positive recommendation index/known-schema checks pass. Direct schema comparison proves every pre-existing final schema object matches baseline plus former migrations; only the three broadcast objects are new.
- SQL preparation guard now prepares Live TV spool queries against DDL extracted from that producer's separate SQLite schema. This closes a pre-existing false-positive without globally allowlisting a table or excluding its column validation; the focused guard passes.
- Workflow YAML parses, and `git diff --check` passes. govulncheck's new workflow was not executed here; npm advisory scan was.
- Final hostile-input verification now also renews the owner credential (including its current account epoch) across intentionally valid empty-body current/everywhere sign-out cases, and has an authenticated canary so later coverage cannot become silent 401-only tests. The final battery passes with its authenticated canary (11.5 seconds). The retained authorization exposed and fixed download-request policy mapping and numeric file-preview ID validation; administration cancellation/deadlines now return retryable unavailable rather than a logged 500.

Cold acceptance steps:

- **Tuner:** on a disposable server, start a recording spanning restart; restart mid-programme while source remains available. Confirm post-restart bytes continue, gap/partial labels remain honest and no missed interval is described as recovered. Measure thousands of recordings on a slow disk while capturing a real-time source; pass means writes/capture continue during traversal.
- **Together:** one native member with a stable 1–2-second converted HLS seek and no rate correction, one web member, 120 seconds; record request/landed targets and clock drift. Pass: convergence inside 750 ms and no periodic correction loop. Also test 8/19-second seeks, failed seek, host seek during pending local seek, paused initial load and end-room protection.
- **Backup:** large disposable state under continuous scanner writes, slow destination. Pass: consistent listed backup or explicit bounded failure; writes/owner UI stay reachable, extra WAL bounded and snapshot released after failure. No partly copied backup appears in the list.
- **Broadcast:** >500 profiles, cancel/drop request between pages, restart and resend identical operation/body. Pass: one notice per traversed inbox and accurate cumulative receipt; changed body under same key refused; revoking owner between pages stops further delivery.
- **Downloads:** two profiles, sign-out/switch, process kill, Wi-Fi loss/return, Android service timeout, app relaunch and receipt expiry with forward/backward device clock changes. Confirm private files never display to another profile, space can be reclaimed explicitly, recovery is authorized and new online resume beats a skewed offline journal.
- **Packages:** build/install actual DEB/RPM on disposable Ubuntu/Fedora, upgrade while running/stopped/disabled, then remove. Pass: running upgrade restarts, stopped/disabled stays so, removal stops/disables, Debian policy-rc.d is respected. Portable fixture success is not that lifecycle proof.
- **Apple/Vega:** exact signed release package on each promised physical device; startup, auth, paused Together load, focus, seek, supported playback/subtitles/audio and upgrade acceptance. Linux source checks cannot establish it.

Security scanner reference: Go's maintained [vulnerability tooling](https://go.dev/doc/security/vuln/); govulncheck `v1.9.0` was resolved from the Go module proxy. Container digests were fetched and their manifest bytes SHA-256 checked against Docker Registry response digests on 2026-10-13.

# Not finished

- Native first-frame pause, signed-out-file cleanup policy/integration, Android process-dead recovery and persisted monotonic receipt expiry (B-13.18, B-14.14, B-14.20 and device portion of B-14.21).
- Native Apple/Vega CI/physical acceptance and cross-repository revision locks (B-25.10, B-25.21) cannot be edited in either writable worktree.
- External architecture/product-contract wording and component-specific Apple licence decision (B-25.12 explicitly skipped).
- OS signed-installer acceptance and the newly observed unfixed npm advisory (B-15.19).
- Physical media/tuner/disk/installer acceptance listed above. None is implied by passing source/unit checks.


# Merge with the base

- `server/api/README.md`: followed the base's current `/v1` guide, replacing obsolete route/version documentation. Retained our toolkit-guarantee and SQL-check limitations; removed the temporary stale-contract warning after regeneration.
- `catalog/personal_activity.go`: kept both imports and both behaviors: typed input errors, early-EOF protection and credits fallback, plus the base's private history-change publication. Audiobook completion/resume rules remain intact.
- `persistence/statements_prepare_test.go`: kept AST extraction, adopted the base's empty-schema failure and coverage of every literal table/index declaration, and prepare spool queries directly against their separate schema. Removed duplicate fallback logic.
- Regenerated contracts with apigen and schema records with schema-sync; no generated file was hand-merged. Existing route scopes retained; completeness and production-router tests pass.
- Verified Linux, Windows amd64 and macOS arm64 builds; catalog/persistence/httpapi vet and targeted completion, freshness, schema, SQL, router and scope tests; web `npx tsc --noEmit`; whitespace and conflict scans. No conflict blocks remain. The whole-tree grep's sole `=======` match is a pre-existing Markdown heading underline in `node_modules/js-tokens/README.md`, not a conflict.
- No staging, commit or snapshot performed. The Git index remains for Justin to stage and commit.
