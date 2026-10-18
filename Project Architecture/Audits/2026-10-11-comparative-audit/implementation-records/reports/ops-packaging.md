## Outcome
The server now runs beneath a small restart parent, including portable launches and plain Docker. Restore staging is a detached, pollable job. Windows packaging installs an SCM service, macOS packaging installs a system launchd service through a package, and Linux packaging retains its corrected upgrade lifecycle. Optional cache and backup destinations are wired. New databases use incremental reclamation, pending migrations receive a retained online database copy, the default update feed is configured with opt-out and a periodic owner notice, and signed-in clients can upload bounded reports for owner read/download/delete. The focused checks and cross-compiles pass. Native runtime acceptance is not claimed.

## Decisions I made
- Windows runs as LocalService with an unrestricted service SID, not SYSTEM. State ACLs name administrators, SYSTEM and that SID. Authenticated UNC shares require an owner-configured service account; upgrades preserve the configured account. The installer uses the candidate control helper to stop SCM and retires any old scheduled launch plus its obsolete batch/XML files. No scheduled startup path is retained.
- macOS uses a system daemon as `_portico`, with state in `/Library/Application Support/Portico Media Server`; the app opens the browser. The package, including the package inside the DMG, is the installation path. Existing daemon configuration survives upgrades.
- One state folder remains the default. `PORTICO_CACHE_DIR` moves HLS, prepared conversions, subtitle rendering and linear media cache. `PORTICO_BACKUP_DIR` moves regular backups. Pre-restore rollback copies and the three pre-migration copies stay beside the live database for atomic local recovery.
- Manual backups remain until deleted; the owner keep count applies to scheduled backups. A skipped spring-forward start moves to the first real wall-clock minute that date.
- Windows linked/mounted-volume library roots are allowed: resolve the selected root and hold a target directory handle. Internal name-surrogate entries remain refused. Managed rclone mounts remain outside this platform follow-up.
- Windows child jobs no longer permit silent breakaway. Nested jobs remain supported; failure to adopt is explicitly logged rather than described as guarded.
- Incremental reclamation is established before WAL creates the first file header, without a full VACUUM or pre-release conversion path. Each idle maintenance step reclaims at most 64 pages.
- Client reports retain at most 100 total, five per authenticated device, with a 256 KiB body and 1 MiB daily account budget. Device identity comes from the signed-in session, not uploaded metadata. Existing credential redaction is applied before persistence.
- Official builds must use an injected signing key matching an independently pinned public key. Final published artifacts receive an authenticated inventory covering complete packages, including web payloads. No signing identity is invented.
- Context refresh was attempted but its lock is outside writable roots. Existing canonical project/standards/verification guidance was read. Execution host is personal Ubuntu `ehler-homelab-mini-1`; no releases, deployments, pushes or Git metadata writes.

## Bugs
- B-05.28: prior Windows identity/probe/helper fixes preserved; root-link decision and parent-job breakaway follow-up implemented. Native acceptance outstanding.
- B-09.1: fixed further: detached restore receipt, copy byte progress, integrity phase, retained result; prior long integrity lane preserved.
- B-09.2: fixed: supervising parent restarts exit 75 on every target; owner Restart route added.
- B-09.3: already fixed: fresh install start and running-state-preserving upgrades verified in maintainer scripts.
- B-09.4: fixed through SCM replacement; NSIS compilation passed on Linux with a fixture payload. Windows runtime acceptance outstanding.
- B-09.5: fixed service account, scoped state ACL, firewall provisioning and explicit UNC/account guidance; Windows runtime acceptance outstanding.
- B-09.6: already fixed bounded wrapper behavior; new service paths use the early rotated recorder.
- B-09.7: not fixed here; networking owns listener limits. Current composition already uses boundedDirectListener; networking must validate its implementation and load behavior.
- B-09.8: not fixed here; networking owns HTTP/TLS and address classification under the programme's accepted-HTTP decision.
- B-09.9: fixed: saved conversion folder used at startup; requested/effective state and pending Restart projection wired.
- B-09.10: not fixed here; networking/client settings own the secure-connections choices and programme policy replacement.
- B-09.11: manifest/parser mismatch already fixed; default feed, periodic owner notice and clear-to-opt-out now wired.
- B-09.12: fixed: owner storage/certificate notices run on startup and every five minutes at maintenance priority.
- B-09.13: not fixed here; identity lane owns file deletion and removal of state-folder trash, per explicit boundary.
- B-09.14: prior additive writable-path guidance preserved; available render/video groups now granted to the service, with hardware-compatible unit hardening. Native GPU acceptance outstanding.
- B-09.15: already fixed: boundary-only permission checks and explicit unsupported Windows assessment preserved.
- B-09.16: already fixed: manifest hashing streams; no full database allocation.
- B-09.17: fixed scheduled failure owner alert, shared running job admission, Windows free-space check, manifest durability, manual retention and DST scheduling. Directory sync on Windows uses flushed files plus NTFS rename journaling; no unsupported directory flush call.
- B-09.18: fixed: separate backup path and explicit same-disk caveat in installation guidance/catalogue.
- B-09.19: not fixed here; accounts-security owns restored credential/session policy.
- B-09.20: fixed: restore admission refuses staging when the single-instance lock is unavailable, instead of falsely restarting successfully, with `restore_lock_unavailable` and catalogue copy.
- B-09.21: package/image service binds now dual-stack; networking owns the server default and topology/listener acceptance.
- B-09.22: fixed: HTTP shutdown and Hosted shutdown notice are joined before returning and closing dependencies.
- B-09.23: fixed for the owned operational log path: early rotated logging, redaction and bounded previous-run log tail now work. File-level retention is now enforced by bounded scans outside the recording lock, with a changed file deferred to the next tick. Relevant lifecycle and maintenance warnings and contained panics have explicit levels; backup/restore phases produce debug records. Startup failure is logged before the recorder closes. Most ordinary source-wide messages still use info; broader instrumentation belongs to their domain owners.
- B-09.24: not fixed here; networking owns proxy/locality consistency.
- B-09.25: not fixed here; networking owns listener/proxy parsing and its error wording.
- B-09.26: fixed image driver/timezone/discovery configuration and build-stage-only Python; Docker/GPU runtime acceptance outstanding.
- B-09.27: already fixed; portable archive fixture tests pass.
- B-09.28: not fixed here; scheduler/client lanes own non-backup window semantics and screen wording.
- B-09.29: not fixed: rotated logs now include early failures, but a fatal startup still exits rather than retaining an authenticated recovery console. This requires a separate recovery surface, beyond minimal process wiring.
- B-09.30: not a confirmed defect; existing pinned online-copy snapshot is retained for consistency. Large active-import WAL qualification remains a runtime test.
- B-09.31: fixed through package/system-daemon installation and explicit uninstall; app opening does not restart or relocate the daemon. Console URL can be supplied when opening from a shell.
- B-09.32: fixed: recover-owner requires the instance lock before opening/migrating the database.
- B-09.33: not fixed here; networking owns readiness treatment under required HTTPS.
- B-09.34: startup responses no longer disclose phases or raw failure paths. Public system build metadata remains for the API/client lanes to replace together.
- B-09.35: already fixed: restored-state readiness/rollback verification preserved.
- B-25.1: already fixed writer/reader shape; default feed now configured.
- B-25.2: fixed shared vet driver exception used by CI, Makefile and release builder.
- B-25.3: already fixed portable ZIP output path; fixture packaging tests pass.
- B-25.4: already fixed architecture-qualified FFmpeg input; build-stage verification retained.
- B-25.5: already fixed in Hosted per fix log; that separate repository is outside this lane.
- B-25.6: fixed: missing or unpinned signing keys fail before builds; no generated release key.
- B-25.7: fixed final-artifact authenticated inventory in release publishing, covering packages and embedded web/assets.
- B-25.8: prior codesigned derivative sealing preserved and independently pinned signer enforced; eight fixture tests pass.
- B-25.9: fixed release workflow gate on exact source SHA, full media tier, release performance tier and canonical qualified Linux FFmpeg bundle. Definition only: workflow was not run.
- B-25.10: not fixed here; native repository CI and physical Apple/Vega/device release evidence belong to client lanes/coordinator.
- B-25.11: fixed this tree's Manrope/Lucide notices and explicit package/container notices for scaling, SQLite, decoder fonts and hls.js. Native font/notice distribution and exhaustive dependency-license inventory require the native/release integrators.
- B-25.12: not fixed: component-specific Apple store distribution rights cannot be established by source changes or a runtime test. Owner/release rights review required.
- B-25.13: fixed AST goroutine guard; found and corrected one previously missed inline storage expiry goroutine.
- B-25.14: not fixed here: dynamic SQL constructor coverage belongs to database/query owners. Existing Go guard comment already distinguishes complete literals from assembled statements.
- B-25.15: not fixed here: hostile-input route batteries belong to API/security testing owners; no all-route expansion run.
- B-25.16: fixed 60-minute full runner/default release race timeout; workflow media/performance qualification has an explicit 120-minute test limit. Runtime duration not measured here.
- B-25.17: fixed by retiring the dormant Python harness referencing removed files; current Go schema tests remain.
- B-25.18: not fixed here: squashing historical pre-release migration backfills touches other lanes' domain tables and requires the database integrator. Online pre-migration safety copy implemented separately.
- B-25.19: prior tag-ref fix strengthened: resolve immutable commit SHA and use it for tests/build/package/publish checkouts; tag publication still names the original tag.
- B-25.20: already fixed Linux lifecycle (duplicate B-09.3).
- B-25.21: not fixed here: pinned cross-repository native/Roku dependency tuples require changes outside the named worktree.

## API changes
- `POST /v1/admin/restart`: new owner-only 202 action.
- `POST /v1/admin/backups/restore`: 200 staged result becomes 202 `{jobId}`; work detached from request timeouts. `GET /v1/admin/backups` gains optional restore progress/result/error. Failed staging can retry the same logical operation after its source is repaired; successful completion replays after restart. Completed restore records retain job identity so clients can recover after missing the staged response during restart. Shared client transport/parsers updated; existing screen helper still returns a validated staging result. The existing refusal-to-copy map has the new lock-unavailable code; no screen layout was changed.
- Existing client report upload/list/read routes moved from raw mux handlers onto typed apikit contracts. Owner-only read/list, new owner download (a JSON attachment carrying metadata and log) and delete routes. Upload includes `deviceClass`; page cursor now includes time plus ID to retain ties.
- Generated OpenAPI and TypeScript contracts refreshed. Removed the corresponding raw client-log route handlers; no duplicate path remains.
- Startup readiness still answers 503 while starting, with operational phase/raw-error disclosure removed.

## Schema changes
`client_log_uploads.device_class` added directly to baseline; schema-sync refreshed recorded schema metadata. No new migration. New database creation initializes incremental auto-vacuum once, before WAL, without repeating a file-mutating pragma on ordinary connections. Pending validated migrations receive a synced online copy (including WAL), with three retained copies under `migration-backups/`.

## Owner or viewer settings added or wired
- `temporaryDirectory`: existing absolute path, default empty; now used by HLS at startup and compared with the applied value for pending Restart. Existing Playback screen exists; the settings registry revision is now p09.3.
- `PORTICO_CACHE_DIR`: absolute startup directory, default state; transient media consumers wired. No console field added.
- `PORTICO_BACKUP_DIR`: absolute startup directory, default state/backups; regular backup create/list/delete/restore/prune wired. No console field added.
- `updates.feedUrl`: existing hot string; default official latest published manifest. Empty opts out and clears its owner notice. Startup/five-minute maintenance drives the cached check (successful fetch refreshes after six hours); failures leave the previous notice alone. Existing Updates screen exists.
- Scheduled backup keep count: existing setting, default seven; now protects manual copies from scheduled pruning.
- Existing storage/certificate notice thresholds now have the five-minute runtime consumer.

## Needed from others
- Client lanes: render restore phases/byte progress, add Restart control and owner client-report list/read/download/delete screen; automatic/manual report sending. Shared services and catalogue copy are supplied.
- Networking: B-09.7/.8/.10/.21 (server default)/.24/.25/.33 and remaining public `/v1/system` build disclosure. Package binds are `[::]:32500`; ensure release listener/topology acceptance on all OSes.
- Identity and accounts-security: B-09.13 and B-09.19 as explicitly assigned.
- API/security/database integrators: B-25.14/.15/.18; native/coordinator: B-25.10/.21 and remaining B-25.11 native notices; rights review B-25.12.
- Configure repository variable `PORTICO_RELEASE_PUBLIC_KEY` to the independently approved base64 Ed25519 public key and retain the existing private signing secret. Signed macOS packages additionally need the installer signing identity. No key was inspected or inferred here.
- Update architecture operations/server and settings/backup contracts for system-daemon macOS state path, SCM account, retained manual copies, restart parent, async restore, separate paths and incremental space reclamation. Source notices came from official Google Fonts Manrope OFL and Lucide upstream license URLs, recorded in THIRD-PARTY-NOTICES.md.

## Test notes for other environments

Ubuntu checks completed during implementation: focused restore crash/rollback tests, durable restore replay, request cancellation/progress, external backup path, manual/scheduled retention, incremental reclamation and migration copy/pruning; owner report boundaries/global cap/tied cursors/redaction; settings applied-versus-requested conversion folder; DST window cases; AST goroutine guard; parent restart/drain; log history/redaction/file retention; real server SIGTERM and HTTP-capacity shutdown. TypeScript no-emit and the eight client backup tests pass. Portable packaging (2), macOS sealing (8) and final-artifact custody/sealing (3) fixture tests pass. Go vet on touched packages passes. Final cross-compiles for Windows amd64/arm64 and Darwin arm64 pass with the actual module graph, without compile stubs. The Windows linked-root test binary also compiles, without executing it. The emitted public feed was exercised through the Go reader and produced the expected update notice. Failed-start log persistence, raw JSON attachment shape, and restore refusal catalogue checks pass.

NSIS compile evidence: Ubuntu NSIS 3.10-2ubuntu2 packages downloaded/extracted under `/tmp/portico-nsis` only. `makensis -V2 -DOUTPUT_FILE=/tmp/portico-nsis/setup-fixture.exe -DSTAGE_DIR=/tmp/portico-nsis/stage -DPRODUCT_VERSION=0.1.0 packaging/windows/installer.nsi` succeeds. The input was a compilation fixture, not executable qualification; this verifies syntax, plugin invocation and control-flow labels only. Full script reviewed: Unicode path handling, quoted direct process commands, popped exit statuses, 60-second command timeout, stop/process join before file replacement, SID/ACL/firewall setup, silent failure exits and state-preserving uninstall.

Windows cold acceptance (disposable Windows 10/11/Server VM, first amd64/NTFS then arm64; provision NSIS 3, real sealed candidate/preceding SCM package with the same current pre-release baseline, matching web/media bundle and cached Go dependencies):
1. Compile the actual payload with `makensis /V4 /DOUTPUT_FILE=C:\Evidence\Portico-Setup.exe /DSTAGE_DIR=C:\Inputs\payload /DPRODUCT_VERSION=<candidate> packaging\windows\installer.nsi`; retain log and executable hash. Install interactively under spaces, ampersand and accented path, then repeat `/S` from a clean snapshot. No startup task or batch launcher should be installed. SCM has no console handles; the service parent must supply valid null output handles and still persist worker startup failures in the rotated log.
2. `sc.exe qc PorticoMediaServer`, `sc.exe queryex PorticoMediaServer`, `sc.exe qsidtype PorticoMediaServer`, `icacls.exe "%ProgramData%\Portico Media Server"` and the named firewall rule must show automatic boot, LocalService by default, unrestricted service SID, explicit private state access and TCP 32500. Reboot without signing in and require readiness plus one parent/worker pair. Complete setup with disposable credentials and require web assets/catalogue persistence.
3. Grant the service SID access to synthetic local media. Scan ordinary roots, a junction, a directory symbolic link and a mounted-volume directory. Repeat NTFS, ReFS and an authenticated UNC share with a configured service account. Denied/missing/file roots must fail promptly; retargeting/replacing a link during acquisition must not publish facts for an inconsistent identity. Internal unsafe name-surrogate entries remain refused. Repeat traversal-only parent ACL and readable leaf cases from the earlier platform acceptance handoff.
4. Pre-release databases with a different baseline digest require a fresh fixture; no compatibility upgrade is claimed. Run native `go test ./internal/storage -run 'TestWindowsLinkedRoot|TestRootRegistration|TestLockedScanInput|TestWindowsInventoryCommand|TestWindowsEmbeddedLyric|TestWindowsProbeDeadlineEndsHelperAndDescendant' -count=1`; no compile stubs. Scan/movie/episode/music titles and embedded lyrics/artwork must actually appear. Exercise original playback, conversion/seek, SRT/ASS and bitmap subtitle paths using the earlier full B-05.28 procedure.
5. Trigger the owner Restart API and restore a backup; SCM service PID/container parent remains, worker PID changes, readiness returns and state matches the selected backup. Stop via Services/`--service-stop` during playback; require worker/helpers/FFmpeg exit before management command returns. Kill only the worker to qualify job containment, including grandchildren; no silent breakaway survivor. Inventory helpers also obey their independent five-minute deadline.
6. Upgrade between two SCM packages while running; require new build, retained catalogue and the same configured service account. Upgrade changes only the executable path, preserving account credentials, startup policy, recovery actions and existing service SID mode; a new service defaults to unrestricted SID. Repeat silent upgrade and a deliberately failed stop: files must be left intact on stop failure. Uninstall must remove service/firewall/install files, retain state/media, and leave no process. Capture SCM config/status, PIDs plus creation times, package/compiler hashes, rotating log tail and redacted HTTP/helper failures.

macOS cold acceptance (Apple Silicon, macOS 13+; real sealed payload (upgrade fixtures share the current pre-release baseline) and optional Developer ID Application/Installer credentials):
1. Build `.pkg` and DMG with `scripts/package-macos.sh`; check `pkgutil --check-signature`, `codesign --verify --deep --strict`, and public-key derivative verification. Inspect the package payload/scripts before installation. No signer/private key belongs in evidence.
2. `sudo installer -pkg <candidate.pkg> -target /`; `sudo launchctl print system/tv.getportico.server.service` must identify `_portico`, fixed application path and system state path. Reboot without interactive login and require readiness. Opening the app only opens the console and must not change daemon PID.
3. Grant `_portico` access to synthetic local/external media; scan and play direct/converted/subtitled samples. Repeat owner Restart and restore; worker changes and parent daemon remains. Stop/bootout during playback and require all children gone.
4. Upgrade with the candidate package over a preceding package, preserving state and a custom daemon configuration. Require the new build running afterward. Run the bundled uninstall script with sudo; daemon/app/package receipt go, state/media remain. Verify no launchd retry after proper uninstall. Record OS/architecture, installer logs, launchd state, package signature/provenance, PIDs and redacted logs.

Linux/Docker/GPU cold acceptance:
- On real Debian/Ubuntu and Fedora/Rocky, install, start, reboot, upgrade a running service, upgrade a deliberately stopped/disabled service, then uninstall. Respect Debian policy-rc.d. Retain systemd enable/running status and data; external writable paths require an additive drop-in. Check render/video group membership and actual VAAPI/QSV conversion; the unit must retain hardening while device access works.
- Build amd64 and arm64 images only from their qualified bundles on a release runner. Run one without restart policy, with persistent state/media volumes and optional separate cache/backup volumes. Require health/readiness, owner Restart and restore without container exit. `docker stop --time 45` must drain, and init must reap orphan descendants. Set TZ and verify a local maintenance window; qualify discovery using host networking. Pass `/dev/dri` plus numeric host render/video groups and verify hardware mode, HDR/tone mapping and audio on an actual GPU. Intel driver packages are selected only for amd64; Mesa/VA libraries remain architecture-neutral.
- On a realistically large database under import, measure online-backup pinned-snapshot WAL growth (B-09.30), restore copy/integrity progress beyond ten seconds, and paced free-page reclamation with browse traffic. No stop-the-world vacuum should occur.
- On GitHub Actions, exercise manual release for a tag different from dispatch branch. All checkouts/tests/artifacts must name the resolved SHA, and publication refuses a tag moved afterward. Require approved independently pinned public key, qualified media/performance gate, package completeness and final-artifact signature verification; modifying a package/web payload/feed must fail verification. No workflow/release was run here.

## Not finished
Native OS service/package lifecycle, Docker/GPU behavior and release workflow execution are unverified here. No release/build pipeline or deployment was run. B-09.29 recovery console, broader source-wide log instrumentation beyond lifecycle/maintenance, and outside-lane issues explicitly listed above remain. Snapshot 001 was taken before its pending build result was inspected; it contained a missing telemetry import, corrected in snapshot 002. Snapshot 003 records the corrected implementation after passing crash/rollback checks. The final snapshot is recorded after the confirmed Linux build and platform compile checks, superseding earlier checkpoints.


Verification checkpoint after recovery: NSIS 3.10 Linux compilation with a fixture payload PASS; macOS sealing fixtures (8) PASS; final release sealing/custody fixtures (3) PASS; client backup transport tests (8) PASS; owner report download/delete and 100-report/timestamp-tie paging tests PASS; log file retention/redaction/reopen tests PASS. Real server SIGTERM drain and HTTP capacity shutdown tests PASS on Ubuntu using a rebuilt disposable helper. The broader restore crash tests caught repeated auto-vacuum assignments changing rollback bytes; initialization was moved to a one-time file-creation step and the exact crash/rollback and incremental-reclamation tests now PASS.


Final verification record: Linux `GOFLAGS=-p=2 go build ./...` PASS; Windows amd64/arm64 and Darwin arm64 builds PASS without alternate module files or stubs; native Windows storage regression binary COMPILES only. Linux vet on touched packages PASS; Windows vet on cmd/server, storage and childprocess PASS. Real Ubuntu worker startup-failure logging and SIGTERM/capacity drain tests PASS. Selected Go persistence/backup crash, receipt replay, filesystem initialization, settings, notices/feed, report and log tests PASS; Node backup transport (8) and existing web refusal-copy (3) tests PASS; TypeScript no-emit PASS. NSIS fixture compilation PASS; portable recipe (2), macOS derivative sealing (8), final-artifact signer/tamper (3) fixtures PASS; shell, YAML/property-list parsing and `git diff --check` PASS. No full test suite, Docker/Gradle/Xcode build, release or deployment was run. Final cumulative snapshot subject: "Finish server operations and platform release packaging".

Final receipt retry regression PASS. Migration snapshot finalization errors are propagated before any migration is allowed. Snapshot 004 is the final cumulative snapshot; the worktree is not edited afterward.

Restore staging now propagates WAL copy/read failures and refuses a non-regular WAL rather than silently omitting it. Focused stage/progress/retry checks PASS.

A new Windows service gets two bounded recovery attempts (5/30 seconds) for worker failures as well as process crashes; upgrades retain owner recovery configuration. Native SCM acceptance must include that failure-action flag.

Installer retirement also removes an old Scheduled Task and batch/XML files instead of leaving a second boot launch behind. This is teardown, not a retained compatibility supervisor; different pre-release database baselines still require a fresh fixture.

## Merge with the base

All eight conflicts are resolved in the working files. No staging, commit or snapshot was attempted; the merge index remains for Justin to finish.

- `INSTALLING.md`: kept service, cache/backup and Docker guidance together with base networking/proxy/certificate guidance.
- `packages/client-core/src/administration.ts`: combined tolerant, deduplicated backup rows with restore progress and durable completion identity; retained the base's deletion shape.
- `packages/client-core/src/index.ts`: kept client-report exports and both metadata exports.
- `server/cmd/server/main.go`: combined Restart, restore/log maintenance and applied conversion-folder tracking with ListeningPort, webhooks and reserved-console connections. Kept one joined HTTP shutdown; removed the obsolete second connection cap. Installed backup webhook callbacks before starting its scheduler.
- `server/internal/backup/backup.go`: retained async restore state and the base backup-completion callback.
- `server/internal/httpapi/backup_routes.go`: retained owner Restart and 202 async restore; every route requires admin scope.
- `server/internal/httpapi/router.go`: retained Restart and ListeningPort; followed removal of the unused mandatory-HTTPS hook.
- `server/internal/operations/settings.go`: retained default update feed and applied temporary directory; adopted missing-title settings and shared network defaults, with trash/mandatory-HTTPS fields removed.

Client-report uploads require session scope; owner list/read/download/delete require admin scope. Added integration tests for these boundaries and Restart, plus a client test combining tolerant backup rows and restore progress. Verification found seven new metadata routes missing admission-lane entries and four deleted trash/item routes still in the scope table; corrected both.

Generated artifacts were regenerated with `GOFLAGS=-p=2 go run ./cmd/apigen` and `python3 scripts/schema-sync.py`, never hand-merged. Final Linux, Windows amd64 and macOS arm64 `GOFLAGS=-p=2 go build ./...` pass. Vet on cmd/server, backup, httpapi and operations passes. Targeted tests in all four packages pass, including restore crash/rollback/replay, settings, route scope/lane/OpenAPI coverage, reports, real startup failure logging and SIGTERM drain. The initial route-coverage failures above pass after correction. Client administration/backup tests: 28 pass. Web no-emit passes. No native runtime acceptance or full suite was run.

Final merge scan: `rg --hidden --no-ignore` over the whole worktree (excluding Git metadata only) finds no conflict-marker lines; `git diff --check` passes. Web `npx tsc --noEmit` was repeated after the final changes and passes. Working files are ready for Justin to stage and commit the merge.
