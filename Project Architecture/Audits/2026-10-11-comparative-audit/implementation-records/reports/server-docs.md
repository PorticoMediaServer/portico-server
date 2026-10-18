# server-docs

## Outcome

Updated 34 Markdown documents, including two new public guides, against the checked-out source. No code, generated contracts, schemas, Git metadata, originals or native files were changed. Nothing was committed, pushed, installed or deployed.

- README now points owners and developers to the relevant documents without promised release availability or marketing claims.
- INSTALLING covers Windows service, macOS daemon/PKG, Linux package lifecycle, portable archives, Docker, state/cache/backup locations, upgrade/restart, backup/restore, reverse proxy, overlays, IPv6, HTTP and custom certificates.
- `docs/libraries.md` covers tolerant naming, filename/folder precedence, embedded tags, episodes, editions/versions/parts, extras, ignore-file syntax and bounds, provider choice, needs attention, missing-title cleanup and actual deletion.
- API guide chapters now describe current sign-in/session phases, refresh and TV code flow; keys/tokens/scopes; whole-millisecond catalogue/offline fields; 428 preconditions; current v1 playback/queues; event stream; bounded title query; JSON-safe Sonarr/Radarr scripts; signed webhook delivery; desktop MCP setup and all source-listed tool names.
- Ancillary identity, administration, downloads, metadata, preference, notification, social and web documents had their stale statements corrected. Duplicated API overview and artwork text now point to their single explanatory chapter.
- THIRD-PARTY-NOTICES covers Go, browser assets, libass WebAssembly contents/fonts, server FFmpeg pins, selected Apple MPVKit/Android libmpv dependencies, container/release tools and external services. `docs/dependency-licenses.md` inventories all 234 npm package/version/licence entries across both lockfiles.
- Milestones: snapshot 001, “Explain service installation, recovery and library matching”; snapshot 002, “Bring the API guide and dependency notices up to date”. Final snapshot 003 is taken after verification with the completed cumulative tree.

Verification is static source and executable-example checking, not service/device acceptance.

## Decisions I made

- Source wins over old lane reports and generated snapshots. In particular, direct-account session management is present in `foundation.go`, even though an earlier report describes it as removed; the guide documents the routes actually registered today.
- One current v1 playback API. Removed v2 recipes, old sign-in shortcut, wire accountToken and direct/devices spellings from the active guide; did not preserve alternative recipes.
- API revisions and paging remain per family. Document both body revisions and If-Match, mandatory 428 responses where enforced, and the current `revision_mismatch`/`after_refresh` vocabulary. Do not promise a uniform envelope or one idempotency store.
- Catalogue Go fields still named Seconds are converted by custom JSON methods. Documentation uses current durationMs/progressMs/startMs and offline positionMs, not the internal field names.
- Backup scrubbing ends sessions but deliberately preserves keys/tokens. Explicitly warn that restoring an older backup can revive a key revoked afterwards; account passwords, second-factor seeds and device approvals also return to backup-time state.
- The checked-in handwritten YAML remains because `apispec.go`, `openapi_coverage_test.go`, `device_contract_test.go` and `guide/check-examples.mjs` read it. Deletion was conditional in the brief and is not safe on this base. Active prose calls it historical, not a second contract.
- Generic remote-HTTP desktop MCP configuration plus initialize/tools-list curl examples; no invented dependency on a particular assistant's configuration syntax. No enable switch exists.
- Sonarr/Radarr use Custom Script, not their incompatible built-in webhook payloads. A script reports the series/movie directory, JSON-encodes names with jq and allows path mapping. Test exits successfully without claiming it scheduled an actual scan.
- Apple LGPL product and Android full flavour are documented separately from the server FFmpeg GPL build. Native artifacts are read-only; no native licence-screen or source-distribution completion is claimed.
- Kept legal/project policy documents (LICENSE, TRADEMARKS, CONTRIBUTING, SUPPORT) intact except links elsewhere; those policies are not facts that code can prove.
- Context-sync was attempted on personal Ubuntu `ehler-homelab-mini-1`; its lock directory was read-only. Available canonical project/standards/verification guidance was read directly. No questions or helper agents were used.

## Bugs

The lane brief names no domain audit documents to implement and prohibits code edits. Reviewed documentation-related entries in domains 08/09/15/25, their BUG-REGISTER rows, and FIX-LOG; runtime defects belong to their owning lanes. Results below are documentation results, not newly claimed runtime fixes.

- B-08.5: already fixed in base; guide now documents 25-second unified-feed polling.
- B-08.11: fixed public documentation drift: no claim that unused apikit cursor/idempotency pieces are the single runtime mechanism; document per-route contracts and retained YAML consumers.
- B-08.14: already addressed by base title query; guide documents its revision-independent, 30-minute cursor and concurrent-walk limits.
- B-08.16 / B-15.15: already fixed in base; public system/build example no longer publishes build fingerprint.
- B-09.3 / B-25.20: already fixed in package scripts; installation documents start-on-first-install and restart-only-if-active upgrade behaviour.
- B-09.4 / B-09.5: base uses real Windows service/LocalService with service SID and firewall rule; documents updated. No Windows runtime claim.
- B-09.8 / B-09.21: already addressed in networking base; documents HTTP from any address, owner home-networks definition and dual-stack listeners.
- B-09.14: already fixed/owner configured; document additive systemd ReadWritePaths and host ACLs rather than promise settings grant filesystem access.
- B-09.18: already fixed; document PORTICO_BACKUP_DIR, distinct cache placement and local rollback copies.
- B-09.19: base session scrub verified; documentation now distinguishes session invalidation from intentionally restored keys/tokens/passwords. Key restoration remains a deliberate choice.
- B-09.27: already fixed; documented full archive launchers and persistent state paths.
- B-15.7: fixed public documentation; access is bearer authority, installation/device records bind refresh, not a proof of a device private key.
- B-15.13: not a defect under programme decision; explicitly describe the approver's identity and role, including owner TV authority.
- B-25.6 / B-25.7 / B-25.8 / B-25.19: source already enforces signer injection/pinned public key, final artifact sealing and requested-tag checkout; release docs describe it without claiming operational custody/installer verification is provisioned.
- B-25.11: font/icon notices already present in base; notices now link real source licence files and cover new subtitle assets. Full transitive release licence collection is not verified by this lane.
- B-25.12: not fixed; Apple distribution/store rights and LGPL distribution compliance need artifact/release review, outside documents-only scope.
- B-25.14: no exhaustive SQL-guard claim remains in the public docs reviewed; underlying guard implementation not changed.
- B-25.15: no exhaustive hostile-input-suite claim is made in the public docs reviewed; internal audit prose and suite implementation are outside this lane.

## API changes

None. Documentation changed to reflect existing routes and fields:

- Direct sign-in returns session.accessToken; account-scoped phase can require profile selection. One-profile/no-PIN can already view. The old POST /v1/sessions shortcut is removed.
- Current device paths are /v1/devices; direct-account session management is source-registered in foundation.go.
- Current playback/queues are /v1; no v2 recipe remains. Header preconditions and timeline seq/generation/positionMs are documented.
- Catalogue durations/progress and offline positions are whole milliseconds on the wire.
- No YAML removed because live test/tooling consumers still read it.

## Schema changes

None. Persistence README's unsupported measured upgrade-time claim was replaced by a bounded-work design statement. The existing direct-baseline prerelease policy remains.

## Owner or viewer settings added or wired

None. Documented existing settings and runtime consumers: home networks/trusted proxies/external origins/custom certificates/connection limit; state/cache/backup/recording paths; per-library online/local provider and language; missing-title rule (after_days, 30 days by default); API rate (600 default)/member-token permission; Original versus explicit Automatic quality. Actual source and current web pages were consulted, not assumed native parity.

## Needed from others

1. **Registry migration lane:** regenerate the API contract after source route registration catches up, and move all consumers off handwritten YAML before deleting it. The checked-in contract omits source-registered foundation operations including GET /v1/direct/sessions. Documentation now marks the snapshot coverage limitation. `guide/check-examples.mjs` still hardcodes YAML; it must validate current registry contracts and source-registered auth examples instead.
2. **API tests/tools:** source comments saying “account token”, secure-connections-required, two-observation missing and manual backup pruning are stale in places. Documentation follows implementation bodies, not these comments. No code comments edited here.
3. **Provider registry:** `/v1/admin/metadata-providers` still offers MusicBrainz for book-like kinds while current LibraryAgentOptions says online audiobook providers are unavailable. Owner guidance uses the per-library-kind metadata-agent options; reconcile the older descriptor endpoint.
4. **Release/licence packaging:** attach complete WebAssembly renderer corresponding source including pinned submodules/build instructions; do not treat an upstream top-level URL/archive as proof of source delivery. Verify Go/npm transitive notice collection in final packages, not just the small explicit copies in package-release.sh.
5. **Native release owners:** retain complete MPVKit/libmpv dependency copyright/licence texts, source/build materials and replaceability/relinking materials appropriate to the actual linked artifacts. Android's hashes pin binaries but do not identify a corresponding source commit. Apple/Android licence screens and store distribution rights were not verified.
6. **Owner/runtime acceptance:** service, package, network/proxy, restore and hardware/native acceptance remains as below; no deployment was authorized or performed here.

## Test notes for other environments

### Executed here

- `git diff --check`: passes.
- Documentation-only scope assertion: 34 changed/new files, all .md; no YAML deleted, no code/contract/schema edit.
- Local Markdown link check: 65 local targets exist; remote links were not tested for release availability.
- All pure JSON fences parse (mixed HTTP exchanges/ellipsis sketches use text fences).
- Shell fences pass bash -n; fixed literal unquoted angle-bracket placeholders in fixture instructions.
- Nine guide examples pass jsonschema against current generated request/response schemas: playback start, timeline, queue create, API-key create/response, title-query response, paths-changed request/response, webhook create.
- Python webhook verifier: accepts a current valid signature; rejects malformed/duplicate/stale headers, wrong secret and changed raw body. JavaScript example also checked with valid, malformed/stale and changed-body cases; duplicate handling was subsequently brought in line with Python and rechecked before final snapshot.
- `python3 -I third_party/libass-wasm/verify.py`: passes.
- npm table compared to both lockfiles; all 234 unique package/version/licence entries have recorded licence metadata.
- Source read: service/installers, Dockerfile, backup/staging/scrub, networking policy/certificates, naming/ignore/extras, provider options/attention, typed/body preconditions, actual custom wire JSON methods, sessions/refresh/code handlers, event feed, title query/paths-changed, MCP and webhooks, release pipeline/signers.
- Apple exact native project pin checked against fetched MPVKit 1.1.0-n9.0.2 Package.swift/LICENSE and cached upstream build.swift. Android artifact pins read from the native worktree; upstream recipe available at release date checked (candidate commit 1ecf5100ec0f75e1d0630f48851b85769c92eed2, not a verified binary-to-source correspondence).
- Sonarr/Radarr current official CustomScript.cs sources checked for series/movie path and Test variables; official wiki pages linked for installed-version reference. No integration was run.
- No Go/web full build or full test suite run; documents-only changes do not justify them. Temporary example check scripts are `/tmp/server-docs-check.py` and `/tmp/server-docs-licences/` evidence; not shipped as product code.

### Acceptance someone can follow cold

- **Windows x64 and ARM64, administrator:** install matching Setup.exe, inspect Get-Service, complete setup, grant service SID access to a test library, restart and reboot. Upgrade into same directory. Pass: service starts without user login, one worker, same state/library, firewall 32500 rule, account configured for a UNC share retained. Fail: empty new state, duplicate process, lost NAS account or stopped active service. Stop before copying state; uninstall must retain ProgramData state.
- **Apple Silicon macOS, administrator:** install PKG, inspect launchctl label/_portico, give a test media folder access, open app, bootout/bootstrap restart, upgrade and reboot. Pass: same state and service without login; app opens console. Fail: wrong owner/path, service not loaded, duplicate process or altered owner launchd settings. Uninstall script must keep state.
- **Debian/Ubuntu and RPM/systemd:** install, verify enabled/active and console, grant only selected deletion/recording paths with drop-in plus Unix permissions, restart, upgrade active then deliberately stopped service. Pass: active restarts, stopped stays stopped, writes only granted paths; fail: broad sandbox removal or service state lost. Check a policy-rc.d host separately.
- **Linux Docker amd64/arm64:** use documented named volume/mount example, restart, pull/recreate with same volume, run backup and restore. Pass: health becomes ready, state persists, read-only media cannot be deleted, restart parent returns after restore. Fail: volume loss, orphan workers or restore requiring an absent external supervisor. GPU pass-through and multicast/host networking require host-specific checks.
- **Networks:** own-hostname TLS proxy with preserved/overwritten forwarding headers, IPv4/IPv6 literals, overlay HTTP, custom PEM certificate and renewal. Pass: same-origin setup/sign-in/edit succeeds, forwarded client address matches trusted chain, home list alone changes classification, HTTP remains admitted, broken cert replacement retains previous valid cert. Fail: proxy origin refusal, untrusted forwarded-header acceptance, sub-path silently claimed supported or hidden overlay-locality rule.
- **Recovery:** create manual + scheduled backups, copy complete directory off-host, test same-build restore on disposable state, then failed/incompatible restore. Pass: async job, recorded lastRestore, same identity/catalogue, people signed out, keys restored and reviewable, manual retention preserved, interrupted apply retains rollback. Fail: copying only live DB, revived person sessions, partial startup or silent incompatibility.
- **Integrations:** create read-only and library-only keys with a test owner; page titles/provider lookup; actual Sonarr/Radarr import/delete with mapped paths; signed webhook test/retry; desktop assistant initialize/list-tools; SSE reconnect from opening cursor. Pass: correct scope refusals, targeted scan only expected folders, raw-body HMAC verified, role-filtered MCP tools, no lost initial feed position. Test credential revocation and post-backup restore explicitly.

## Not finished

No authorized document edits are left pending. Final cumulative snapshot is the deliverable; integration must reconcile documents with changes that arrive after this base.

Statements not established by source/example checks, and therefore not claimed as verified:

- Published package/image availability, successful installation/upgrades/reboots on Windows/macOS/systemd/Docker, filesystem/NAS ACL outcomes and physical GPU/native behavior.
- Actual proxy/overlay/IPv6 reachability, certificate renewal and Hosted advertisement; actual Sonarr/Radarr triggers on a specific installed version; desktop-assistant-specific config compatibility.
- Runtime backup/restore, failure rollback and original-media recovery. Written instructions are source-backed but not exercised on an owner's data.
- Complete platform FFmpeg bundle dependency bill/licence/source contents: this worktree has recipes, not the completed bundles. THIRD-PARTY-NOTICES explicitly identifies the artifact's inventory as the authority.
- Native binary-to-source correspondence, all embedded framework subdependency notices, relinking/replaceability materials, native app licence screens, and Apple store/distribution rights. Apple exact package dependency graph and Android declared recipe/pins are documented; these are not distribution-compliance acceptance.
- Production signer custody, OS package signing/notarization and installer trusted-key provisioning. Source enforcement exists; operational provisioning is unverified.
- Complete generated-contract coverage and current handwritten-YAML example checking. Retained consumers prevent YAML deletion within this lane. Needed from others lists the exact follow-up.
