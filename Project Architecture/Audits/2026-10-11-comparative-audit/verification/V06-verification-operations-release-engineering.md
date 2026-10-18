# V06 — Verification of operations and release engineering (documents 09 and 25)

## Summary

Completed all **24 Tier 1** and **20 Tier 2** entries. Tier 1: 4 Confirmed, 17 Confirmed, narrower, and 3 Unsettled; Tier 2: 10 Confirmed and 10 Confirmed, narrower. No whole entry was refuted, but several absolute statements and impact assumptions were disproved or constrained. Twelve Low entries are carried forward without re-checking. Source evidence is strong for the restore deadline, Linux update outage, Windows ZIP path and Docker build-input defects. B-09.3/B-25.20 and B-09.11/B-25.1 are duplicate implementation tasks.

The most important upgrade is **B-09.35 (Medium → High)**: a schema-valid bare foreign database can be recorded as restored before the missing external identity key prevents startup, outside the automatic rollback boundary. Linux upgrade severity is reconciled to High in both duplicate entries. Several original High assessments become Medium because they describe conditional deployment behavior or release qualification risk rather than a demonstrated general outage. The fixed 256-socket cap is real, but its viewer/load threshold remains unmeasured.

The simple plain-backup model is an explicit product decision. Restoring historical credentials is therefore classified as a **deliberate trade-off**, with residual risk and conflicting older contracts requiring documentation reconciliation. DB-only relocation is not promised: the normative moving procedure requires the whole state folder. Windows task defaults and current vet failure remain pending exact platform/toolchain evidence; Apple distribution rights require a rights review, which runtime tests cannot provide. No new finding outside the assigned entries was added.

## Verdict table

First-pass confidence/severity comes from the supplied worklist; wording is shortened for readability. High Windows/Medium elsewhere is retained as a platform-specific first-pass rating. “No” means no matching Backlog entry found in the domain/topic scan, not absence from every historical tracker. Sizes: S hours, M days, L at least a week. Tier 1 rows come first.

| ID | Claim (short) | First pass (confidence / severity) | Verdict | Class | Your severity | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-09.1 | Restore and integrity checks inherit a ten-second budget | Reading / High | Confirmed, narrower | Defect | High | M | No |
| B-09.2 | Restore exits without a Windows restart supervisor | Reading / High on Windows, Medium elsewhere | Confirmed, narrower | Defect | High | M | No |
| B-09.3 | Linux package lifecycle stops upgrades | Reading; lifecycle not exercised / High for anyone updating a packaged install | Confirmed, narrower | Defect | High | S | No |
| B-09.4 | Scheduled Task execution and battery defaults | Probable; needs a check on a Windows machine / High on Windows | Unsettled — needs runtime | Defect | High | S | No |
| B-09.5 | SYSTEM account and missing Windows firewall provisioning | Probable / High for first-run success on Windows | Confirmed, narrower | Defect | Medium | M | No |
| B-09.7 | The direct listener retains a hidden 256-socket ceiling | Reading for the cap and the close; the user-visible impact needs a load test / High against the "couple of hundred concurrent browsers" goal | Confirmed, narrower | Defect | Medium | S | No |
| B-09.8 | Socket HTTP policy excludes tailnet and global IPv6 peers | Reading / Medium to High for the self-hosting audience | Confirmed, narrower | Defect | Medium | M | No |
| B-09.12 | Storage/certificate notification producer has no default schedule | Probable / Medium; a silently filling disk is the classic way these servers break | Confirmed | Unfinished feature | Medium | S | No |
| B-09.13 | Retained deleted files remain on the state volume | Reading / High as a design risk | Confirmed, narrower | Defect | High | M | No |
| B-09.14 | systemd write sandbox conflicts with external media writes | Reading the unit; each consequence follows from documented systemd behaviour and was not tested / Medium to High | Confirmed, narrower | Defect | Medium | M | No |
| B-09.15 | Permission walk includes mounted media and is not Windows ACL-aware | Reading + probable / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-09.26 | Container hardware/discovery/timezone assumptions | Probable + reading / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-09.35 | Bare foreign database restore can pass validation then fail startup | Reading + speculative / Medium if the second part is real | Confirmed, narrower | Defect | High | M | No |
| B-25.2 | Release vet invocation differs from the accepted driver policy | Probable / High | Unsettled — needs runtime | Defect | High | S | No |
| B-25.3 | Windows ZIP resolves dist inside its payload directory | Reading / High | Confirmed | Defect | High | S | No |
| B-25.4 | Docker release has no qualified bundle input | Reading / High | Confirmed | Defect | High | M | No |
| B-25.6 | Builder creates a local signing identity when absent | Reading / High | Confirmed, narrower | Unfinished feature | Medium | S | No; explicitly known in release-signing.md |
| B-25.8 | Optional macOS signing invalidates sealed byte digests | Reading / High | Confirmed, narrower | Defect | Medium | M | No |
| B-25.9 | Media/performance qualification is not enforced by app release CI | Reading / High | Confirmed, narrower | Design risk | Medium | M | Partly: test-runtime/full-timeout work |
| B-25.10 | Native CI omits Apple/Vega builds and physical acceptance gates | Reading / High | Confirmed, narrower | Design risk | Medium | L | Partly: ports/device/UI acceptance |
| B-25.12 | GPL Apple distribution requires a component-specific rights decision | Speculative / High | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-25.16 | Full-tier timeout is shorter than a historical package duration | Probable / Medium | Confirmed, narrower | Design risk | Medium | S | Yes: runner-test.sh --full cannot finish httpapi |
| B-25.19 | Manual release CI can check a different ref from packaged tag | Reading / High | Confirmed | Defect | High | S | No |
| B-25.20 | Linux upgrade has no scripted service restart (duplicate) | Probable / Medium | Confirmed, narrower | Defect | High | S | No |
| B-09.6 | Unbounded wrapper logs | Reading / Medium | Confirmed | Defect | Medium | S | No |
| B-09.9 | Converted-video folder setting is not wired | Reading / Medium | Confirmed | Defect | Medium | S | No |
| B-09.10 | Secure-connections choice cannot save | Reading / Low to Medium | Confirmed | Defect | Low | S | No |
| B-09.11 | Published update document does not match parser | Reading / Medium | Confirmed | Defect | Medium | S | No; signing integration broadly known |
| B-09.16 | Pre-restore DB hashing allocates the entire file | Reading / Medium | Confirmed | Defect | Medium | S | No |
| B-09.17 | Scheduled backup robustness gaps | Reading / Low individually; Medium together because backups are the safety net | Confirmed, narrower | Defect | Medium | M | No |
| B-09.18 | Backup destination is fixed in code, but not necessarily the same volume | Reading / Medium | Confirmed, narrower | Design risk | Medium | S | No |
| B-09.19 | Restore revives historical authentication state by current simple-backup design | Reading / Medium as a risk an owner should be told about | Confirmed, narrower | Deliberate trade-off | Medium | S | No; Backups spec supersedes older complex design |
| B-09.21 | IPv4-only default listener | Reading / Medium | Confirmed | Unfinished feature | Medium | S | No |
| B-09.23 | Console log omits startup and previous-process evidence | Reading / Medium for supportability | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-09.24 | Proxy client identity differs across boundaries | Reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-09.27 | Archives need unprovided environment configuration | Reading / Medium for those artifacts | Confirmed, narrower | Defect | Medium | S | No |
| B-09.28 | Maintenance-window wording promises unsupported job timing | Reading / Low to Medium | Confirmed, narrower | Documentation error | Low | S | No |
| B-09.29 | Failed-start reason lives in process/service logs | Reading / Low to Medium; see F-09.10 | Confirmed, narrower | Missing feature mislabelled as a bug | Low | M | No |
| B-25.1 | Public manifest/parser mismatch (duplicate) | Reading / Medium | Confirmed | Defect | Medium | S | No |
| B-25.7 | Component seal does not cover final package payload | Reading / Medium | Confirmed, narrower | Design risk | Medium | M | No; broad signing enforcement is known |
| B-25.11 | Promised font/icon notices are missing | Reading / Medium | Confirmed | Defect | Medium | S | No |
| B-25.14 | Static SQL gate excludes dynamically assembled statements | Reading / Medium | Confirmed, narrower | Documentation error | Low | S | No; specific scale costs are known |
| B-25.18 | Startup migrations perform data-dependent work | Reading / Medium | Confirmed | Design risk | Medium | M | No explicit migration-policy item |
| B-25.21 | Sibling CI inputs follow moving default branches | Reading / Medium | Confirmed | Design risk | Medium | M | No |

## Tier 1 — detailed verification

### B-09.1 Restore and integrity checks inherit a ten-second budget

- **Claim:** Restore validation and owner integrity operations use an unsuitable request deadline; healthy candidates can be reported corrupt.
- **What I traced:** Production registry calls registerBackups (apps/portico-server/server/internal/httpapi/foundation.go:609); lane mapping selects adminHeavy (apps/portico-server/server/internal/httpapi/route_lanes.go:125,308), with ten seconds at apps/portico-server/server/internal/httpapi/admission.go:80,486. Handler passes ctx to Stage (apps/portico-server/server/internal/httpapi/backup_routes.go:149); Stage hashes/copies synchronously before InspectCandidate (apps/portico-server/server/internal/backup/stage.go:70,110,121). InspectCandidate maps query/cancellation errors to integrity failure (apps/portico-server/server/internal/persistence/inspect.go:47). Diagnostics use r.Context at apps/portico-server/server/internal/httpapi/admin_concurrency.go:201,213. Driver checks cancellation during VM execution (apps/portico-server/server/internal/thirdparty/sqlite/PORTICO.md:13). The router actually wraps its mux with admission (apps/portico-server/server/internal/httpapi/router.go:1234), so this is not unused registry metadata.
- **Verdict:** **Confirmed, narrower.** The deadline and misleading restore error are confirmed. There is no demonstrated database-size threshold: the first pass estimates hundreds of MB/GiB without measurement. Diagnostics may return an incomplete report, rather than the restore error. Backup creation already detaches its context (apps/portico-server/server/internal/backup/backup.go:309); restore does not.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** Owners whose staging/validation exceeds ten seconds lose the normal restore path; large libraries or slow storage are most exposed.
- **Fix direction:** Make staging and diagnostics supervised operations with bounded admission, progress, cancellation and honest timeout errors; touch httpapi/backup_routes.go, admission/route_lanes, backup/stage.go and diagnostics handlers. Do not merely give arbitrary user work an infinite request. (**Size:** M.)
- **Related:** B-09.16 increases staging cost; B-09.35 covers later startup verification. They concern different restore phases.

### B-09.2 Restore exits without a Windows restart supervisor

- **Claim:** Restore requests a process restart, but several distributions provide only a one-shot process.
- **What I traced:** apps/portico-server/server/cmd/server/main.go:917 sets restartRequested and cancels after two seconds; apps/portico-server/server/cmd/server/main.go:124 exits 75. apps/portico-server/packaging/windows/installer.nsi:37-46 writes a one-shot cmd and an ONSTART task, without restart-on-failure or loop. Linux unit:17 uses Restart=on-failure; macOS launchd plist has KeepAlive. Dockerfile:94 uses tini (signal/reaping, not application restart).
- **Verdict:** **Confirmed, narrower.** Windows has no automatic second run after the restore exit. Docker depends on the owner runtime restart policy, and terminal runs require relaunch; these are conditional deployment limitations, not failures of systemd/launchd. The staged data and pre-restore safety mechanism remain available.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** Packaged Windows owners finish staging and then lose service until explicitly relaunching the task or rebooting; Linux/macOS supervision already handles exit 75.
- **Fix direction:** Provide a restart-capable Windows supervisor and document container restart policy/portable relaunch, with a truthful restart-required response for unsupervised installs; touches Windows packaging, INSTALLING.md and backup UI. (**Size:** M.)
- **Related:** B-09.4 shares the Windows task lifecycle; B-09.29 concerns failed startup visibility.

### B-09.3 Linux package lifecycle stops upgrades

- **Claim:** Linux maintainer scripts enable without starting and stop/disable on upgrades as well as removal.
- **What I traced:** apps/portico-server/packaging/linux/nfpm.yaml:31 wires postinstall.sh and preremove.sh. apps/portico-server/packaging/linux/postinstall.sh:7-8 reloads/enables only; apps/portico-server/packaging/linux/preremove.sh:4-5 unconditionally stops/disables. Neither examines lifecycle arguments. The unit restart policy cannot restart a service explicitly stopped by systemctl.
- **Verdict:** **Confirmed, narrower.** The service has no script path to restart after upgrade. Fresh install is enabled-only. The precise RPM disabled-at-end ordering requires qualification of the generated RPM/nfpm lifecycle, so I do not present that detail as reproduced or universal.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** An otherwise running packaged installation goes offline during update and has no scripted return to service.
- **Fix direction:** Use distribution-aware install/upgrade/removal branches, preserve prior enable/running state, and qualify generated DEB/RPM upgrade/remove behavior; touches packaging/linux scripts and nfpm.yaml. (**Size:** S.)
- **Related:** B-25.20 is the same finding; merge it into this item. Both agree on the missing restart; RPM end state remains conditional.

### B-09.4 Scheduled Task execution and battery defaults

- **Claim:** The Windows task may inherit a three-day execution limit and battery restrictions that stop a long-running server.
- **What I traced:** apps/portico-server/packaging/windows/installer.nsi:45 invokes schtasks /Create without XML/settings or restart policy. Microsoft documents the 72-hour default and battery-start default: [ExecutionTimeLimit](https://learn.microsoft.com/en-us/windows/win32/taskschd/tasksettings-executiontimelimit), [DisallowStartIfOnBatteries](https://learn.microsoft.com/en-us/windows/win32/taskschd/tasksettings-disallowstartifonbatteries). Microsoft also documents the default stop-on-battery setting: [StopIfGoingOnBatteries](https://learn.microsoft.com/en-us/windows/win32/taskschd/tasksettings-stopifgoingonbatteries).
- **Verdict:** **Unsettled — needs runtime.** The source omission is real and the platform defaults are documented, but the exact XML generated by this schtasks command on supported Windows was not inspected. Cheapest experiment: create an equivalently configured harmless long-running task in a disposable Windows VM, export with schtasks /Query /TN <test-name> /XML, inspect ExecutionTimeLimit, DisallowStartIfOnBatteries and StopIfGoingOnBatteries. PT72H/true confirms the claimed conditions; PT0S/false refutes them. No three-day wait is needed.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** If those defaults are inherited, all persistent Windows installs stop after 72 hours; laptops can stop sooner. Severity is provisional pending XML evidence.
- **Fix direction:** Register explicit unlimited execution and battery settings, or move to a service supervisor; touches installer.nsi and a task XML template. (**Size:** S.)
- **Related:** B-09.2 compounds the outage because no restart trigger exists.

### B-09.5 SYSTEM account and missing Windows firewall provisioning

- **Claim:** Windows packaging uses SYSTEM, does not provision inbound firewall access, and relies on inherited state-directory ACLs.
- **What I traced:** apps/portico-server/packaging/windows/installer.nsi:35 creates the state folder without ACL changes; :42 runs server/children under the task account and :45 selects SYSTEM. No firewall or account-credential provisioning occurs in the installer. Microsoft confirms logon-specific drive mappings and recommends suitably authorized UNC access: [Services and redirected drives](https://learn.microsoft.com/en-us/windows/win32/services/services-and-redirected-drives).
- **Verdict:** **Confirmed, narrower.** User-mapped drives are unavailable to SYSTEM, but SYSTEM does not categorically break every network share: suitably authorized UNC/machine-account access can work. Missing firewall provisioning is confirmed, while actual LAN blocking depends on existing firewall policy. Default ProgramData ACL exposure was not inspected; it is a conditional risk. Media sandbox restrictions still exist, so privileged parsing is not wholly unrestricted.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **Medium.** Windows owners relying on Explorer mappings or a default blocking firewall need additional setup; local disks and allowed inbound networks can work.
- **Fix direction:** Provide a least-privilege service/account strategy, supported UNC credential path, explicit private-profile inbound rule and private state ACL; touches Windows installer/supervisor and setup documentation. (**Size:** M.)
- **Related:** B-09.15 covers the POSIX-mode warning on Windows; B-05.28 is a separate Windows library-path problem, not proved here.

### B-09.7 The direct listener retains a hidden 256-socket ceiling

- **Claim:** The memory-derived cap is undermined by a second fixed direct-listener cap that closes overflow sockets.
- **What I traced:** apps/portico-server/server/cmd/server/main.go:996-999 uses boundedDirectListener for nonloopback binds (and certificate-enabled loopback). apps/portico-server/server/cmd/server/connection_cap.go:57 puts capConnections beneath the classifier; :67-88 waits before raw Accept. apps/portico-server/server/internal/networking/certificate_listener.go:57 allocates 256 slots and :116-121 closes sockets when full. apps/portico-server/server/cmd/server/transport_policy.go:30 selects 16-2048.
- **Verdict:** **Confirmed, narrower.** The effective cap is min(memory budget,256), with closing rather than backlog waiting only when the inner budget permits more than 256 (normally above 4 GiB). Low-memory hosts already queue at their lower cap. Existing bounded TLS integration test uses limit 4 (apps/portico-server/server/cmd/server/connection_cap_tls_test.go:54), so does not contradict this. The claimed few-dozen-browser threshold depends on actual browser connection use and was not established.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **Medium.** Capable nonloopback deployments exceeding 256 simultaneously accepted sockets see resets; the number of viewers affected is unmeasured, so High capacity impact is not established.
- **Fix direction:** Pass the transport budget into DirectListener or eliminate its duplicate total-socket limit while retaining classification limits; touches networking/certificate_listener.go and cmd/server/connection_cap.go. (**Size:** S.)
- **Related:** B-16.7 concerns HTTP/1 client connection pools; B-08.17 concerns a distinct realtime lane. They are related capacity limits, not duplicates.

### B-09.8 Socket HTTP policy excludes tailnet and global IPv6 peers

- **Claim:** Cleartext socket admission disagrees with reported home-network classification and does not consume owner home networks.
- **What I traced:** apps/portico-server/server/internal/networking/certificate_listener.go:157,176 admits only loopback/private/link-local raw peers. apps/portico-server/server/internal/networking/certificate_listener_test.go:24 explicitly rejects 100.64.0.1. apps/portico-server/server/internal/connectivity/status.go:246 calls 100.64/10 LAN. apps/portico-server/server/cmd/server/main.go:996 wires classifier on nonloopback binds. portico-internal/Product Contracts/architecture-amendments.md:23 deliberately confines HTTP to trusted LAN and public routes to HTTPS.
- **Verdict:** **Confirmed, narrower.** Tailnet source peers and global IPv6 peers are rejected, even if an owner treats them as home networks; the report discrepancy is real. This does not mean every tailnet or IPv6 deployment fails: private IPv4/ULA, a properly named custom TLS certificate, or a suitably configured proxy can work. Public HTTPS is deliberate; the defect is the inconsistent/configuration-insensitive definition of trusted home.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **Medium.** Owners expecting HTTP over an encrypted tailnet or global-address LAN get a closed socket despite other locality settings.
- **Fix direction:** Agree on a validated owner-configurable socket trust policy and shared locality terminology without allowing headers to bypass public-peer TLS; touches networking listener, connectivity status, settings/locality contracts. (**Size:** M.)
- **Related:** B-09.24 and B-15.22 describe separate locality/policy inconsistencies; agree on the existence of multiple boundaries.

### B-09.12 Storage/certificate notification producer has no default schedule

- **Claim:** The visible warning thresholds have no default production trigger.
- **What I traced:** apps/portico-server/server/cmd/server/console.go:85 registers notification-maintenance; apps/portico-server/server/internal/operations/notifications.go:926 produces storage notices and :972 certificate notices. Production calls to NotificationMaintenance appear only in that adapter; other calls are tests. apps/portico-server/server/internal/operations/schedule_wake.go:21 schedules existing console_schedules; apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:1980 defines an empty table, with no INSERT seed. apps/portico-server/web/src/screens/server/MaintenanceWindows.tsx:80-81 renders only existing rows.
- **Verdict:** **Confirmed.** The functions work when explicitly invoked or an API-created schedule exists; notifications tests call them directly. No default schedule or client producer was found. On-demand dashboard attention is a mitigation, not an inbox trigger. The contract promises operational alerts (settings-and-operations/contract.md §11.2).
- **Class:** Unfinished feature. **In Backlog:** No.
- **Severity:** **Medium.** Default-install owners who do not inspect health miss low-storage and expiry inbox notices.
- **Fix direction:** Seed or wire a durable notification-maintenance cadence and validate existing-install upgrade seeding; touches console scheduling, migrations/initialization and notifications. (**Size:** S.)
- **Related:** B-09.13 and B-09.17 increase the value of those warnings but are not duplicates.

### B-09.13 Retained deleted files remain on the state volume

- **Claim:** Retained file deletion copies across volumes into state/trash and expiry is never automatically purged.
- **What I traced:** apps/portico-server/server/internal/administration/deletion.go:301 fixes the trash root under state; :432 invokes the file journal move. :439 stores held entries only when retention >0; :464-470 zero-retention commits then removes the stage. :717,750 provides owner-invoked EmptyTrash/ExpiredOnly. Only production caller is apps/portico-server/server/internal/httpapi/administration.go:238; apps/portico-server/server/cmd/server/console.go:41 catalog-trash-cleanup invokes a different catalog method. apps/portico-server/server/internal/operations/settings.go:95,191 ships 30 days.
- **Verdict:** **Confirmed, narrower.** Confirmed for default positive retention and deleteFiles=true. It is wrong to read the title as every deletion: catalog-only deletion leaves original media; zero-retention staging is purged after successful commit. Default expired held files remain indefinitely, contrary to the retention wording. Cross-volume copy can fail safely rather than always fill the disk, but retained accumulation remains unbounded.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** Owners deleting large files from separate media disks can exhaust the database volume, including long after the advertised retention.
- **Fix direction:** Keep recoverable bytes on the source volume and implement durable expiry cleanup/headroom checks integrated with the file journal; touches administration/deletion.go, file journal, scheduler and retention settings. (**Size:** M.)
- **Related:** B-18.13 addresses delete confirmation; B-14.5 is a separate state-volume copy risk. B-05.24 catalog missing-file retention is not this physical trash.

### B-09.14 systemd write sandbox conflicts with external media writes

- **Claim:** The shipped systemd service cannot write to ordinary media/recording paths outside its state folder.
- **What I traced:** apps/portico-server/packaging/linux/portico-media-server.service:10-23 uses portico, ProtectSystem=strict, ProtectHome=read-only and only the state ReadWritePaths. apps/portico-server/packaging/linux/postinstall.sh:3-5 creates only the portico group/user. apps/portico-server/server/internal/administration/deletion.go:432 moves original media; recordings can use PORTICO_RECORDINGS_DIR (cmd/server/main.go). The upstream [systemd execution manual](https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml) defines strict read-only protection and ReadWritePaths exceptions; local installed manual agrees.
- **Verdict:** **Confirmed, narrower.** External file deletion and recordings are blocked unless owner overrides the unit/mount policy. Reading a restore source outside state is not blocked merely by read-only protection; restore stages inside state. Let’s Encrypt readability and /dev/dri access depend on host ACLs/groups, not an absolute consequence of ProtectSystem. Existing media beneath state remains writable.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **Medium.** Packaged Linux owners using separately mounted media hit broken delete/recording actions; reading/playing ordinary readable files need not fail.
- **Fix direction:** Generate documented least-privilege writable-path overrides and supplemental hardware groups from actual configured resources, or adjust sandbox policy consistently with owner-authorized writes; touches unit, installer guidance and configuration diagnostics. (**Size:** M.)
- **Related:** B-09.26 shares host device/driver prerequisites, but Docker is a separate distribution.

### B-09.15 Permission walk includes mounted media and is not Windows ACL-aware

- **Claim:** The state permission warning/fix traverses all ordinary descendant directories, including mounts, without platform-specific ACL logic.
- **What I traced:** apps/portico-server/server/internal/backup/stateperms.go:17 WalkDir checks any group/other mode bits, stops at first exposure, skips symlinks and ignores many read errors; :47-64 fix walks and chmods. apps/portico-server/server/cmd/server/main.go:829 runs check in a background goroutine; apps/portico-server/server/internal/httpapi/backup_routes.go:167 exposes the explicit fix. Backups spec §6 requires tightening only Portico-owned files.
- **Verdict:** **Confirmed, narrower.** The complete healthy-tree walk and mounted-directory chmod reach are confirmed. Symlinked external media is not followed. A private parent with loose children need not actually expose those files, so warning semantics overstate effective access. Windows mode bits cannot establish NTFS ACL exposure and chmod cannot repair ACLs; exact platform warning persistence needs Windows evidence. Docker exposure is conditional on volume permissions, not a defect merely because it runs in a container.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **Medium.** Large healthy trees consume background startup I/O; owners fixing a real mount path may chmod external media; Windows owners receive unsuitable permission guidance.
- **Fix direction:** Inventory owned state boundaries, skip mount/recording media, account for parent traversal, and use a Windows ACL implementation or truthful unsupported state; touches backup/stateperms.go and state-permissions UI. (**Size:** M.)
- **Related:** B-09.5 concerns inherited Windows ACLs. Both distinguish physical access from a POSIX-mode heuristic.

### B-09.26 Container hardware/discovery/timezone assumptions

- **Claim:** The minimal image lacks a complete default hardware/discovery/timezone deployment configuration.
- **What I traced:** apps/portico-server/Dockerfile:63-87 installs only bubblewrap/CA/python/tini and a normalized FFmpeg bundle, creates an unprivileged user, exposes TCP only, and sets no TZ. apps/portico-server/scripts/normalize-ffmpeg-bundle.py:9-19 copies executable tools/licenses/source, no vendor driver libraries. apps/portico-server/server/internal/workpolicy/policy.go:135 resolves empty timezone to Local; apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:4401 seeds 03:00 with empty zone. Dockerfile:22-28 explicitly describes sandbox fallback.
- **Verdict:** **Confirmed, narrower.** Normalized bundle does not carry vendor runtime drivers; accelerated success still depends on GPU/backend, device mappings and group ACLs, so “no hardware transcoding” is not universal. Default bridge discovery limitations need actual topology evidence; EXPOSE does not itself block multicast/UDP. UTC is a default-local-time outcome and owner timezone overrides work. Baseline sandbox fallback is documented deliberate behavior, not a new defect.
- **Class:** Design risk. **In Backlog:** No.
- **Severity:** **Medium.** Owners deploying the bare image can miss hardware access and LAN discovery, and backup timing can surprise them.
- **Fix direction:** Document supported device/group/driver and networking/timezone recipes; build target-appropriate driver support if warranted. Touch Dockerfile, INSTALLING.md and container deployment examples; qualify hardware on real devices. (**Size:** M.)
- **Related:** B-09.14 shares device prerequisites; B-25.4 is the separate missing bundle build input.

### B-09.35 Bare foreign database restore can pass validation then fail startup

- **Claim:** A bare foreign database can be marked restored before external identity-key checks fail; copying only the database also lacks mandatory state files.
- **What I traced:** apps/portico-server/server/internal/backup/stage.go:218-221 accepts a regular bare path with no associated state files; apps/portico-server/server/internal/persistence/inspect.go:65-99 checks schema, not server/key identity. apps/portico-server/server/internal/backup/apply.go:28-61 verifies DB and retires playback; :332-338 records restored and removes marker. apps/portico-server/server/cmd/server/main.go:282 calls ApplyStaged before :304 authority and :362 OpenCurrentKeys. apps/portico-server/server/internal/networking/claim_keys.go:116-127 reads the incarnation named by restored DB and rejects missing/mismatched key. apps/portico-server/server/cmd/server/networking_initializer.go:39-42 refuses DB-only state without authority file.
- **Verdict:** **Confirmed, narrower.** The speculative second part is now a concrete source path: a valid same-schema DB from another server names a key absent in the receiving networking-keys directory. Apply succeeds, removes the rollback marker, and later OpenCurrentKeys returns an error on every restart. Safety copy remains recoverable manually; there is no demonstrated data destruction. Whole-folder migration is explicitly required by Backups spec §3, so DB-only copying is an intentional prerequisite, not itself a bug.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** Owners using the expressly supported bare-DB restore option for a foreign/missing-key candidate can leave the server repeatedly failing startup; upgrade Medium to High.
- **Fix direction:** Preflight candidate identity/key prerequisites and include all normal startup identity checks in the rollback-protected verification boundary; touches backup/stage.go, apply.go and cmd/server authority/key initialization. (**Size:** M.)
- **Related:** B-09.29 covers visibility of the subsequent fatal start. B-09.19 is the distinct restored-credentials decision.

### B-25.2 Release vet invocation differs from the accepted driver policy

- **Claim:** The release builder and Makefile run all vet analyzers on a maintained driver for which CI disables unsafeptr.
- **What I traced:** apps/portico-server/.github/workflows/ci.yml:60-66 excludes the driver then vets it with -unsafeptr=false. apps/portico-server/scripts/build-server-release.py:149 and apps/portico-server/Makefile:12 use go vet ./.... apps/portico-server/server/internal/thirdparty/sqlite/PORTICO.md:24 records inherited unsafe.Pointer diagnostics; current driver includes translated pointer arithmetic. The builder preserves a controlled Go environment (:29-33) and uses check=True (:149).
- **Verdict:** **Unsettled — needs runtime.** Invocation disagreement is certain and a nonzero vet result stops the builder. I did not run the analyzer, so present-day failure under the pinned Go version remains unestablished. Cheapest experiment: copy current server/apikit source to an isolated scratch checkout, using only already available offline dependencies and scratch caches, run the exact release go vet ./..., then the CI split invocation. Unsafe-pointer-only failure in the first and success in the second confirms; clean first invocation refutes the blocker. This is proposed follow-up, not an experiment performed in this audit.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** If inherited warnings persist, every official server release job and Makefile test-go stops; retain High provisionally.
- **Fix direction:** Centralize the documented analyzer policy while retaining all checks for application packages; touches build-server-release.py, Makefile, CI and a shared vet entry point. (**Size:** S.)
- **Related:** B-25.3 and B-25.4 are independent later release blockers; fixing them does not resolve this analyzer uncertainty.

### B-25.3 Windows ZIP resolves dist inside its payload directory

- **Claim:** Portable ZIP packaging writes to a nonexistent nested output directory in the official workflow.
- **What I traced:** apps/portico-server/.github/workflows/release.yml:132 passes relative dist. apps/portico-server/scripts/package-release.sh:10,14 makes PAYLOAD=dist/payload-windows-<arch>; :15-17 creates the outer directories; :38 changes into PAYLOAD then writes dist/Portico-...zip. No nested dist is created, and set -e (:8) propagates failure before NSIS packaging.
- **Verdict:** **Confirmed.** The shell path resolves to <repo>/dist/payload-windows-<arch>/dist/.... No preceding copy creates that directory in a clean payload. Absolute OUT avoids it, but the production call is relative. This is deterministic path reasoning, not a package build reproduction.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** Both Windows jobs fail before archives/installers are uploaded, preventing the all-platform publication dependency from completing.
- **Fix direction:** Resolve OUT/PAYLOAD to absolute paths before changing directories and qualify exactly the workflow argument form; touches package-release.sh. (**Size:** S.)
- **Related:** B-09.27 is the separate usability of archives after creation, not a duplicate.

### B-25.4 Docker release has no qualified bundle input

- **Claim:** The Docker job starts from a fresh checkout but needs an ignored generated bundle that earlier jobs do not transfer.
- **What I traced:** apps/portico-server/.github/workflows/release.yml:113-118 downloads bundles only in server jobs; :156-172 Docker checks out its own context, with no bundle download, and requests two architectures. Dockerfile:76 COPY needs third_party/ffmpeg/bundle, :78 verifies it. third_party/ffmpeg/.gitignore excludes bundle; tracked source directory has only recipe/provenance. publish needs docker (apps/portico-server/.github/workflows/release.yml:186).
- **Verdict:** **Confirmed.** No Docker stage manufactures that host-context input. Job dependencies do not share workspace files. One architecture’s bundle would also fail target validation for the other. Therefore the source graph cannot build the release container from a clean tag.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** Every clean official release is blocked at Docker before publication, independently of Windows ZIP and vet.
- **Fix direction:** Provide verified per-target bundle downloads/build contexts or target-aware Docker stages and validate both platform images; touches Dockerfile and release workflow. (**Size:** M.)
- **Related:** B-09.26 concerns how a successfully built container runs, not this missing build dependency.

### B-25.6 Builder creates a local signing identity when absent

- **Claim:** The generic release builder generates a new signer when no protected signing key is provisioned.
- **What I traced:** apps/portico-server/scripts/build-server-release.py:35-43 creates a raw private key if missing; :167 signs and verifies with that same generated public key. apps/portico-server/server/api/release-signing.md:3 prohibits production key generation on development hosts and :9 acknowledges enforcement unfinished. apps/portico-server/.github/workflows/release.yml:53-55 requires a nonempty secret and :119-125 restores/checks its 32-byte key before builder invocation.
- **Verdict:** **Confirmed, narrower.** Local/manual builder calls can silently make a self-consistent but unofficial signature. The official GitHub workflow already prevents the ordinary absent-secret fallback, so the title must not imply that normal release jobs silently invent a production identity. Signed custody is explicitly unfinished and previews advertise unsigned packages (apps/portico-server/INSTALLING.md:10,20).
- **Class:** Unfinished feature. **In Backlog:** No; explicitly known in release-signing.md.
- **Severity:** **Medium.** Local release users can mistake mathematical signature validity for approved release identity; no current automatic-install exploit or key leak is demonstrated.
- **Fix direction:** Require an injected approved signer in official mode and expose explicit development-signing mode; pin/identify the public key and document custody. Touch builder, signing docs and workflow identity checks. (**Size:** S.)
- **Related:** B-25.7 describes artifact coverage rather than signer identity; they need separate fixes.

### B-25.8 Optional macOS signing invalidates sealed byte digests

- **Claim:** The signed macOS variant mutates server/tool bytes after their application/toolchain hashes are sealed.
- **What I traced:** apps/portico-server/scripts/build-server-release.py:158 records server SHA256 and :165-167 copies/seals toolchain-manifest identity. apps/portico-server/scripts/package-release.sh:21-22 preserves those manifests. apps/portico-server/scripts/package-macos.sh:14 copies payload, :29-34 conditionally force-signs server and FFmpeg binaries. verify_manifest at builder:82 rejects a changed server digest; verify-ffmpeg-manifest.py verifies tool hashes. apps/portico-server/.github/workflows/release.yml:151 injects optional identity.
- **Verdict:** **Confirmed, narrower.** For a successful nonempty codesigning-identity path, embedded Mach-O signatures change bytes while manifests remain unchanged. Unsigned default previews avoid it. The current packaged-tool resolver (apps/portico-server/server/internal/mediatools/resolve.go:185) can use adjacent tools without this manifest verification, so this does not establish all signed playback fails today; it establishes inconsistent integrity metadata and verifier rejection.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **Medium.** Owners/reviewers of the optional signed macOS distribution cannot validate packaged bytes against the preserved seal; pre-release unsigned default is unaffected.
- **Fix direction:** OS-sign binaries before final digest/toolchain sealing, or explicitly seal signed derivatives and preserve source provenance separately; touches macOS packaging, release builder and manifest verification. (**Size:** M.)
- **Related:** B-25.7 is the broader final-package signature scope; B-25.8 is an actual byte mutation within presently listed components.

### B-25.9 Media/performance qualification is not enforced by app release CI

- **Claim:** App release success does not require full media/performance tiers or exact target-media integration tests.
- **What I traced:** apps/portico-server/server/internal/testtier/testtier.go:25-29 skips media when unset and :8 documents integrator all-tier runs per merge batch. apps/portico-server/.github/workflows/ci.yml:49 and builder:148 leave tiers unset. controlled_env (:30) preserves caller PORTICO_TEST_TIER, so manual override works. builder validates target FFmpeg (:110) but copies it into stage only at :165. apps/portico-server/server/internal/decodertest/tools.go:26,47 searches canonical bundle/PATH and can skip missing tools. apps/portico-server/.github/workflows/ffmpeg-toolchain.yml:32-70 separately runs two required real-media tests on target runners.
- **Verdict:** **Confirmed, narrower.** The automated app-release gate omits these tiers and exact changed-app/target-bundle qualification, but manual merge-batch qualification is deliberately documented. It is not evidence that media tests have never run or that releases fail playback. Inheriting an explicitly set tier also means the builder does not prohibit full coverage.
- **Class:** Design risk. **In Backlog:** Partly: test-runtime/full-timeout work.
- **Severity:** **Medium.** Release decisions lack automated linkage to full media and scale evidence; impact is a qualification risk, not a demonstrated product outage.
- **Fix direction:** Require source-SHA/toolchain-specific qualification evidence, with target-native tests and explicit performance lane separate from everyday CI; touches release/CI workflows, builder and test-tier/evidence documentation. (**Size:** M.)
- **Related:** B-25.10 covers native-client platform qualification; B-25.16 is the cost/timeout of the documented full preset.

### B-25.10 Native CI omits Apple/Vega builds and physical acceptance gates

- **Claim:** The inspected native CI cannot establish Apple/Vega native build health or real-device release readiness.
- **What I traced:** apps/portico-react-native/.github/workflows/ci.yml:16-35 runs shared/JS checks; :38-50 builds Android debug only. Available workflows are ci.yml/security.yml, with no store/release workflow. apps/native/package.json:9 invokes JS tests; apps/vega/package.json:7 offers a release build command but CI does not invoke it. Project Architecture/08 Operations.md Testing describes separate client testing; portico-internal/Product Contracts/governance.md:15 explicitly retains hardware evidence needs.
- **Verdict:** **Confirmed, narrower.** Automated lane omissions are real, but they do not prove missing manual Apple/Vega builds or a falsely certified current release. Existing context records Android/Fire/Vega release testing builds and states that physical acceptance is outstanding. Physical-device evidence can be a supervised manual gate; demanding it all be automated is broader than necessary.
- **Class:** Design risk. **In Backlog:** Partly: ports/device/UI acceptance.
- **Severity:** **Medium.** Native platform changes can escape CI; future public platform-readiness claims need build and physical evidence. Medium is proportionate for an explicitly prerelease product.
- **Fix direction:** Add Apple/Vega release-build lanes and require an auditable per-platform acceptance checklist/evidence before claiming support; touches native CI, platform build scripts and release qualification records. (**Size:** L.)
- **Related:** B-25.9 concerns server media gates. B-19.18 is a particular unverified Apple startup concern, not proof of a broken client here.

### B-25.12 GPL Apple distribution requires a component-specific rights decision

- **Claim:** GPL-labelled native/shared code has no visible Apple-specific grant, leaving a distribution-rights question.
- **What I traced:** apps/portico-react-native/LICENSE contains GPLv3; apps/portico-react-native/README.md:104 declares GPL-3.0-or-later and :72 says store-ready packages are not yet published. Shared package license fields and root server licensing agree; no Apple-specific exception was found in the inspected native notices/README. Product policy explicitly chooses GPL and no external contributions (original BRIEF.md product decisions).
- **Verdict:** **Unsettled — needs runtime.** This verdict label is retained from the brief taxonomy, but no runtime test can settle legal distribution rights. Absence of an exception in source does not prove incompatibility, infringement, rejection, or absence of a private rights review. Cheapest decisive follow-up is an inventory of the exact Apple-shipped copyleft components/ownership/grants plus a component-specific review against the intended Apple terms; valid rights for every component settle it positively. Successful App Store submission alone would not establish rights.
- **Class:** Design risk. **In Backlog:** No.
- **Severity:** **Medium.** Potential store-distribution blockage matters before store launch; no current store-ready release or concrete rights conflict is established, so High is premature.
- **Fix direction:** Record an explicit component-by-component distribution decision and required grants/notices, preserving GPL product policy; touches native/shared licensing inventory and distribution documentation. No legal conclusion or licensing change proposed without that review. (**Size:** M.)
- **Related:** B-25.11 confirms notice omissions; those do not by themselves prove Apple rights incompatibility.

### B-25.16 Full-tier timeout is shorter than a historical package duration

- **Claim:** The advertised full preset keeps 30 minutes while Backlog records httpapi taking 2082 seconds.
- **What I traced:** apps/portico-server/scripts/runner-test.sh:42 selects all for --full; :211-219 inserts -timeout 30m unless supplied. portico-internal/Project Architecture/Backlog.md:57 records 2082s on 2 October and the timeout mismatch. Current runner comments explicitly preserve caller timeout overrides. The October source was not timed.
- **Verdict:** **Confirmed, narrower.** The numeric comparison and unchanged default are confirmed (2082s >1800s). A current full run is not proved to time out: fixtures/hardware/source changed, and caller timeout can override the preset. Cheapest follow-up is one full httpapi run on the documented runner with a sufficiently long observation timeout and duration/fixture breakdown; above 1800s confirms current preset failure.
- **Class:** Design risk. **In Backlog:** Yes: runner-test.sh --full cannot finish httpapi.
- **Severity:** **Medium.** Integrators on workloads comparable to the historical measured run lose complete all-tier feedback; already acknowledged open work.
- **Fix direction:** Move scale-only costs into a required performance lane or choose a measured justified full-tier timeout; touches runner-test.sh and tier placement in httpapi tests. (**Size:** S.)
- **Related:** B-25.9 agrees that tier availability alone is not enforced qualification. Merge historical timeout concerns rather than reopening a new blocker without timing.

### B-25.19 Manual release CI can check a different ref from packaged tag

- **Claim:** workflow_dispatch can package a requested tag while reusable CI checks the dispatcher ref.
- **What I traced:** apps/portico-server/.github/workflows/release.yml:18 accepts tag; :42,66,100 explicitly check it out, while :57-59 calls ci.yml without a ref input. apps/portico-server/.github/workflows/ci.yml:7 defines workflow_call with no inputs and :32,73 checkout without ref. GitHub reusable workflow context follows the caller: [Reuse workflows](https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows).
- **Verdict:** **Confirmed.** The requested string changes artifact checkouts, not the caller github.ref/SHA. Tag pushes naturally align; a dispatch on main for another tag does not. Builder tests the tagged server separately, and web tagged build proves compilation, but neither substitutes for tagged client-core/generated-contract tests.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** Manual releases can publish a tag without its promised web/core/contract CI gate, so retain High release-integrity severity.
- **Fix direction:** Resolve tag to immutable commit once, pass it to all reusable checkouts and record/check that artifact and test SHAs agree; touches release.yml and ci.yml. (**Size:** S.)
- **Related:** B-25.21 is a separate foreign-repository moving-ref issue.

### B-25.20 Linux upgrade has no scripted service restart (duplicate)

- **Claim:** Package upgrade stops the service and never starts the new process.
- **What I traced:** apps/portico-server/packaging/linux/nfpm.yaml:31-33 wires the scripts; apps/portico-server/packaging/linux/preremove.sh:4-5 unconditionally stops/disables; apps/portico-server/packaging/linux/postinstall.sh:7-8 only reloads/enables. The script arguments are ignored. getportico-tv/src/content/docs/administration/update-portico.md describes package updating without an explicit manual service-start requirement.
- **Verdict:** **Confirmed, narrower.** Same source defect as B-09.3. Upgrade outage is confirmed by the missing lifecycle branch, while exact generated RPM ordering/state needs artifact qualification. Raise the first pass Medium to High to match its duplicate, rather than assign conflicting severity to the same package action.
- **Class:** Defect. **In Backlog:** No.
- **Severity:** **High.** All running Linux packaged installations using this lifecycle can be left offline after upgrade.
- **Fix direction:** Apply B-09.3’s package-aware lifecycle fix and generated DEB/RPM checks once; touches packaging/linux scripts. (**Size:** S.)
- **Related:** Exact duplicate of B-09.3; both agree. Count as one implementation task.

## Tier 2 — quick checks

B-09.18 and B-09.19 are promoted to detailed discussion because their scope/classification changes the action. The other entries retain quick source checks.

### B-09.6 Unbounded wrapper logs

**Confirmed — Defect; Medium; size S; Backlog: No.** apps/portico-server/server/cmd/server/administration.go:27 tees every recorded line to stderr; apps/portico-server/packaging/windows/installer.nsi:42 appends indefinitely and apps/portico-server/packaging/macos/launcher.sh:21-22 sets unmanaged stdout/stderr files. Rotated messages.log does not bound these copies. Fix wrapper output/rotation while preserving early-startup evidence; do not simply discard stderr before the recorder exists.

### B-09.9 Converted-video folder setting is not wired

**Confirmed — Defect; Medium; size S; Backlog: No.** apps/portico-server/server/internal/operations/settings.go:83 stores a restart field; apps/portico-server/server/internal/telemetry/capacity.go:191 reports its directory, but apps/portico-server/server/cmd/server/main.go:664 constructs HLS under state/hls. The visible consumer claim is false (settings-and-operations contract §2.2); consume the effective setting at HLS construction and align readiness.

### B-09.10 Secure-connections choice cannot save

**Confirmed — Defect; Low; size S; Backlog: No.** apps/portico-server/packages/client-core/src/presentation/settings-structure.ts:392 sends lan-plain-allowed alongside required/preferred; apps/portico-server/server/internal/operations/settings.go:52 accepts only the latter two. There is no transport mapping in this path. Remove or implement the third value consistently; lower severity because other supported choices remain usable.

### B-09.11 Published update document does not match parser

**Confirmed — Defect; Medium; size S; Backlog: No; signing integration broadly known.** apps/portico-server/scripts/write-public-update-manifest.mjs:21-29 emits channel/version/assets; apps/portico-server/server/internal/administration/maintenance.go:490,525 rejects an empty channels map. Align writer/parser and check their exact document; duplicate B-25.1, with matching verdict.

### B-09.16 Pre-restore DB hashing allocates the entire file

**Confirmed — Defect; Medium; size S; Backlog: No.** apps/portico-server/server/internal/backup/apply.go:209 hashes every pre-restore file; apps/portico-server/server/internal/backup/stage.go:187 verifies that manifest via apps/portico-server/server/internal/backup/files.go:248 os.ReadFile. The database-sized allocation is real, while OOM depends on size/memory; stream hashes. Ordinary backup manifests exclude the DB hash, so this is specifically the safety-copy restore path.

### B-09.17 Scheduled backup robustness gaps

**Confirmed, narrower — Defect; Medium; size M; Backlog: No.** apps/portico-server/server/internal/backup/schedule.go:62-75 logs failures, bypasses Start/running state and retries on later ticks; apps/portico-server/server/internal/backup/disk_other.go:6 skips Windows space checks; apps/portico-server/server/internal/backup/files.go:150-159 lacks manifest fsync and apps/portico-server/server/internal/backup/backup.go:420 lacks directory fsync. Prune includes manual copies (:487-509), as explicitly documented/tested retention rather than necessarily a bug. Add alerts, shared admission and durability/Windows checks; DST skip follows apps/portico-server/server/internal/workpolicy/policy.go:178 and has no measured incidence.

### B-09.18 Backup destination is fixed in code, but not necessarily the same volume

- **Claim:** Backup directory is not configurable and default backups share state storage; first pass says another volume is impossible.
- **What I traced:** apps/portico-server/server/internal/backup/backup.go:341-347 creates state/backups with MkdirAll; :473 joins backup IDs underneath it. Stage resolves that path (:182); no separate backup env var is read. portico-internal/Project Architecture/02 Server.md:19 advertises PORTICO_BACKUP_DIR; newer Backups spec §1 explicitly names state/backups. No same-disk-warning text was found in the relevant English settings UI/catalog.
- **Verdict:** **Confirmed, narrower.** The missing setting and misleading old env-var documentation are confirmed. However state/backups can be a separately mounted directory (or parent symlink): no volume-equality check prohibits it. Thus “only on the database’s own volume” is false as an absolute statement. Default topology still shares failure risk and no independent-copy warning is shown. Promoted to detailed verification to preserve this correction.
- **Class:** Design risk. **In Backlog:** No.
- **Severity:** **Medium.** Default owners can lose both DB and backup on one disk failure; advanced owners can put the same logical folder on another device.
- **Fix direction:** Correct the env-var documentation, explain same-disk limitations and supported independent-copy/mount workflow; optionally implement a real destination setting. Touch backup service, capacity reporting and INSTALLING/settings guidance. (**Size:** S.)
- **Related:** B-09.17 concerns backup reliability; neither implies an independently stored copy.

### B-09.19 Restore revives historical authentication state by current simple-backup design

- **Claim:** Restore rolls credentials/accounts back to the backup; older contracts instead require a fresh security incarnation.
- **What I traced:** apps/portico-server/server/internal/backup/backup.go:398 snapshots the whole database; apps/portico-server/server/internal/backup/files.go:21-34 includes authority/key state. apps/portico-server/server/internal/backup/apply.go:58 invokes playbackv1.RetireRestored; apps/portico-server/server/internal/playbackv1/restore.go:25-28 stops playback/grants, without invalidating auth session families or API keys. Backups (Plex model) spec §2.5 describes sign-ins after restore and §2 rejects a more complex restore engine; Product Contracts/settings-and-operations/contract.md §12.2 and portico-internal/Product Contracts/architecture-amendments.md:115 still demand fresh external incarnation.
- **Verdict:** **Confirmed, narrower.** Credential rollback is real; password restoration is ordinary backup semantics. I do not confirm it as an accidental security bug against Justin’s normative simple-backup decision. The older security-incarnation contract conflicts and must be reconciled explicitly. Actual validity of any particular refresh/access token still depends on its own expiry and current validation; “all old sessions work forever” would be false. Promoted because classification materially changes the action.
- **Class:** Deliberate trade-off. **In Backlog:** No; Backups spec supersedes older complex design.
- **Severity:** **Medium.** An owner restoring after compromise or post-backup revocation can restore access that they had removed; residual risk remains Medium.
- **Fix direction:** Explain credential/member rollback in restore confirmation and align the governing docs; offer an explicit post-restore revoke-all action if desired without changing ordinary restore simplicity. Touch restore UI, Backups spec/contract and existing identity revocation APIs. (**Size:** S.)
- **Related:** B-09.35 concerns missing key startup failure, not credential resurrection; B-14.16 describes offline receipt dead ends after restore.

### B-09.21 IPv4-only default listener

**Confirmed — Unfinished feature; Medium; size S; Backlog: No.** apps/portico-server/server/cmd/server/main.go:142 defaults to 0.0.0.0, as do distribution launchers; apps/portico-server/server/internal/networking/topology.go:253 deliberately excludes IPv6 for that bind. The implemented IPv6 publishing switch does not change the listener. Provide a supported dual-stack bind and truthful prerequisite guidance; remote IPv6 behavior still needs OS/network qualification.

### B-09.23 Console log omits startup and previous-process evidence

**Confirmed, narrower — Unfinished feature; Medium; size M; Backlog: No.** apps/portico-server/server/cmd/server/main.go:862 installs the recorder after early startup; apps/portico-server/server/internal/servicelog/servicelog.go:419 reads only its fresh ring and :376 prunes only that ring. The server recorder has no redaction stage (:215,275); apps/portico-server/server/internal/servicelog/clientlogs.go:125 does redact client uploads, so do not generalize this to every log lane. Add early recording, bounded historical read and server redaction; no actual credential leak or exhaustive call-site level count was proved.

### B-09.24 Proxy client identity differs across boundaries

**Confirmed, narrower — Defect; Medium; size M; Backlog: No.** apps/portico-server/server/internal/httpapi/request_locality.go:119 parses a rightmost-untrusted XFF chain; apps/portico-server/server/internal/httpapi/onboarding.go:18 and apps/portico-server/server/internal/httpapi/rate.go:194 accept one address from dependency/environment proxy trust. Socket policy uses raw peer (apps/portico-server/server/internal/networking/certificate_listener.go:176). Unify HTTP trust/locality and preserve distinct socket security; headerless tunnels inherently cannot reveal original peers. Duplicates B-08.7/B-15.3 agree on shared limiter buckets, not a universal proxy auth bypass. A configured trusted proxy without readable XFF is already treated remote by apps/portico-server/server/internal/httpapi/request_locality.go:85-87; the original headerless “everyone is local” example applies only when that peer is not configured trusted, or to the separate setup/socket rules.

### B-09.27 Archives need unprovided environment configuration

**Confirmed, narrower — Defect; Medium; size S; Backlog: No.** apps/portico-server/scripts/package-release.sh:21-26 copies payload without launcher/readme; apps/portico-server/server/cmd/server/main.go:214 defaults state relative to cwd and apps/portico-server/server/internal/httpapi/installation.go:33 serves no web if PORTICO_WEB_DIR is unset. The binary may run, so “do not run” overstates it; documented unpack-and-use instructions (apps/portico-server/INSTALLING.md:13,17) do not supply needed configuration. Add portable launchers/readme and explicit state location.

### B-09.28 Maintenance-window wording promises unsupported job timing

**Confirmed, narrower — Documentation error; Low; size S; Backlog: No.** apps/portico-server/packages/i18n/src/catalog/en-US/settings.ts:252-257 promises window execution, while apps/portico-server/server/internal/workpolicy/policy.go:145 deliberately always permits background work; apps/portico-server/server/internal/backup/schedule.go:51 alone consumes these generic windows. Separate console_schedules can start registered jobs, so “only backups can ever be scheduled” is too broad. Align this surface with continuous background progress and actual scheduling semantics.

### B-09.29 Failed-start reason lives in process/service logs

**Confirmed, narrower — Missing feature mislabelled as a bug; Low; size M; Backlog: No.** apps/portico-server/server/cmd/server/main.go:124-129 exits on run errors; the startup listener closes when run returns (:247) and systemd limits restarts (packaging/linux/portico-media-server.service:5-6). A persistent recovery/status UI is absent, but stderr/journal evidence exists; add a supervisor recovery surface if required by contract, rather than claim failed startup is wholly invisible.

### B-25.1 Public manifest/parser mismatch (duplicate)

**Confirmed — Defect; Medium; size S; Backlog: No.** apps/portico-server/scripts/write-public-update-manifest.mjs:22 emits singular channel; apps/portico-server/server/internal/administration/maintenance.go:491,526 requires plural channels with builds. Exact duplicate B-09.11; implement one shared schema/parser integration fix.

### B-25.7 Component seal does not cover final package payload

**Confirmed, narrower — Design risk; Medium; size M; Backlog: No; broad signing enforcement is known.** apps/portico-server/scripts/build-server-release.py:158 seals one server artifact; apps/portico-server/scripts/package-release.sh:23,26 adds/rewrites web later. Release workflow additionally emits whole-package checksums and GitHub provenance attestations (:215,220), so it is wrong to imply no final-package integrity evidence at all. Component manifest remains insufficient alone; define final-package authentication/pinned installer verification separately.

### B-25.11 Promised font/icon notices are missing

**Confirmed — Defect; Medium; size S; Backlog: No.** apps/portico-server/THIRD-PARTY-NOTICES.md:30-34 promises adjacent Manrope license/Lucide records; web/src/ui/fonts and packages/design lack such files, native font assets likewise. apps/portico-server/packages/design/src/icons.ts:5 says Lucide-derived. apps/portico-server/scripts/package-release.sh:24 copies only GPL/summary alongside FFmpeg notices, omitting SCALE_LICENSE and maintained SQLite BSD text; collect truthful notices/provenance and check final archives. This is source/recipe evidence, not a legal ruling about unseen packages.

### B-25.14 Static SQL gate excludes dynamically assembled statements

**Confirmed, narrower — Documentation error; Low; size S; Backlog: No; specific scale costs are known.** apps/portico-server/server/internal/persistence/statements_prepare_test.go:18-22 explicitly documents literal-only coverage; :72-78 skips dynamic concatenation, and apps/portico-server/server/internal/persistence/statements_scan_test.go:148 consumes that set. portico-internal/Project Architecture/02 Server.md:32 overstates “every SQL statement.” Correct docs and maintain constructor-specific semantic/plan coverage; this establishes no unsafe production SQL or new runtime scale defect.

### B-25.18 Startup migrations perform data-dependent work

**Confirmed — Design risk; Medium; size M; Backlog: No explicit migration-policy item.** apps/portico-server/server/internal/persistence/migrations/0008_show_facets.sql:17,21,26 backfills genres in startup apps/portico-server/server/internal/persistence/schema.go:135; 0003_drop_reaction.sql:6 drops a column and 0011_recommendation_positive_seeds.sql:5 builds an index. apps/portico-server/server/internal/persistence/README.md:33,41 says no data-growing startup backfill. Fresh empty installs avoid historic backfill cost; record pre-release exceptions and use post-ready resumable derivation for future populated upgrades. B-07.14 agrees on policy drift but rates current pre-release impact Low; timing remains unmeasured.

### B-25.21 Sibling CI inputs follow moving default branches

**Confirmed — Design risk; Medium; size M; Backlog: No.** apps/portico-react-native/.github/workflows/ci.yml:23-24 checks server without ref, and apps/portico-react-native/scripts/shared-packages.mjs:15-20 compares current sibling copies. Worklist-cited Roku workflow :23-26 likewise omits refs (read only that CI evidence; Roku client otherwise out of scope). Pin dependency commits and trigger intentional downstream checks; no current content drift or remote job failure was established.

## Not re-checked (Low)

Copied from the worklist, with no second-reviewer verdict or severity reassessment:

- **B-09.20 — On a filesystem without advisory locks, a restore restarts the server and then does nothing.** Not re-checked. conf: Confirmed by reading; sev: Low (narrow), but confusing when hit.
- **B-09.22 — Shutdown does not wait for in-flight HTTP requests.** Not re-checked. conf: Confirmed by reading; sev: Low (no data risk; clients retry), but it makes "graceful restart during playback" less graceful than intended and the s.
- **B-09.25 — Unfriendly failures on two environment variables.** Not re-checked. conf: Confirmed by reading; sev: Low.
- **B-09.30 — A long backup holds back the write-ahead log.** Not re-checked. conf: Speculative; needs a runtime check with a large database under a scan; sev: Low.
- **B-09.31 — macOS launcher leftovers.** Not re-checked. conf: Confirmed by reading; sev: Low.
- **B-09.32 — recover-owner opens the live database without the instance lock.** Not re-checked. conf: Confirmed by reading; the scenario needs mismatched binaries; sev: Low.
- **B-09.33 — Health check versus "require HTTPS" on a specific bind address.** Not re-checked. conf: Speculative; sev: Low.
- **B-09.34 — Public endpoints say a little more than the contract allows.** Not re-checked. conf: Confirmed by reading; sev: Low.
- **B-25.5 — Hosted apikit has actual drift and its checker is not in CI.** Not re-checked. conf: Confirmed by reading; sev: Low.
- **B-25.13 — Bare-goroutine guard is a line-regex gate, not a complete AST gate.** Not re-checked. conf: Confirmed by reading; sev: Low.
- **B-25.15 — Hostile-input test rotates cases despite exhaustive-sounding prose.** Not re-checked. conf: Confirmed by reading; sev: Low.
- **B-25.17 — Python schema integration test references removed files.** Not re-checked. conf: Confirmed by reading; sev: Low.

## New findings

None. The newly established foreign-DB identity-key failure is evidence completing B-09.35, not a separate finding.

## Disagreements worth Justin's attention

- **B-09.35:** raise Medium to High for supported bare-DB restore, because key verification happens after the restore has been finalized. Keep whole-state-folder migration as the documented deliberate requirement.
- **B-09.19:** historical credentials return, but classify the simple restore decision as a trade-off. Reconcile the older fresh-incarnation contract with the normative Plex-model spec before planning a more elaborate engine. Do not treat plain backups or ordinary password rollback as newly discovered negligence.
- **B-09.3 / B-25.20:** one Linux lifecycle fix; High for upgrade outage. Exact RPM final enable state still needs generated-package evidence. **B-09.11 / B-25.1:** one update-document schema fix.
- **B-09.5 / B-09.14 / B-09.26:** separate certain source omissions from conditional ACL, firewall, hardware and network outcomes. SYSTEM does not prohibit every authorized UNC share; read-only systemd protection does not prohibit reading external restore sources; Docker EXPOSE is not a multicast firewall.
- **B-09.7:** High → Medium; the hidden socket cap is confirmed only above the lower memory cap, and “a few dozen browsers” is not an established failure threshold.
- **B-25.6 / B-25.8 / B-25.9 / B-25.10 / B-25.12:** High → Medium. Respect the enforced GitHub key injection, unsigned-preview default, documented manual integrator/device evidence, and unknown legal outcome. These remain useful release tasks without alleging a current general failure.
- **B-09.18:** a fixed logical backup path need not be the same physical volume. Separate mounted backup storage remains possible; default same-disk risk and stale advertised environment variable remain real.
- **B-09.10 / B-09.28 / B-09.29:** resolve original Low–Medium ranges to Low. Respect continuous background work; fix scheduling claims and distinguish conventional logged startup failure from a missing persistent recovery UI. **B-25.14:** Medium → Low; narrow documentation about an intentionally literal-only SQL guard, without inventing a production SQL defect.

## Evidence boundary

This was a static review of the working trees as present on 11 October 2026, on **ehler-homelab-mini-1**. Read-only Git HEAD observations: server `82aaa54f89efdc2e029b635746627cd8c8b71960`; native `a3d8e7cd14c16229715a07c67f43cb8364bae743`. These identify checked-out history, not a guarantee that working files are all committed or immutable. Source documents 09/25, the original/verification briefs and worklist, relevant operations/backup/schema/signing contracts, Backlog, and canonical project/verification guidance were read. The context-sync refresh was attempted and failed on its read-only lock path; existing context was used.

No Portico server, build, installer, test suite, formatter, toolchain verification executable or comparison code was run. No Portico hosted endpoint, demo, `.private`, browser, computer-use or MCP tool was accessed. No subagents were spawned. Python scripts under `/tmp` only assembled and checked this report (plus reading an installed systemd manual); no copied-product behavior experiments were performed. Only this output file was written under Portico.

Public Microsoft/GitHub/systemd documentation was consulted for task defaults, redirected drives, reusable workflow context and service filesystem protection; links appear with the relevant evidence. Platform consequences stated from documentation remain inferences about this source configuration, not observed installation behavior.

The cheapest remaining decisive checks are the harmless exported Windows task XML for B-09.4, isolated exact-toolchain vet invocation for B-25.2, and component-specific Apple rights review for B-25.12. Additional acceptance evidence remains necessary for generated DEB/RPM lifecycle, actual ACL/firewall policy, container/GPU/multicast behavior, macOS signed bytes, database copy/check durations, full-tier runtime and capacity/load impact. Each conditional claim is narrowed where those boundaries matter. All 44 assigned Tier 1/2 IDs were reached; none remain unreviewed.
