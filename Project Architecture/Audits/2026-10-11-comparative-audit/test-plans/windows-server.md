# Windows server acceptance

## Purpose and evidence

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Disposable Windows 10/11 and Windows Server VM, first x64 NTFS then ARM64 hardware/VM; administrator, NSIS 3, real candidate/preceding packages. ReFS, a mounted-volume directory and an authenticated UNC test share are needed for storage variants. The service runs as LocalService by default; grant NT SERVICE\PorticoMediaServer only the fixture media permissions. Set a dedicated service account for the UNC case; retain that account through upgrade.

## Media and accounts

For ordinary playback, make synthetic samples with the candidate's qualified FFmpeg/ffprobe pair. Run in a new writable fixture folder (never over existing media):

```sh
ffmpeg -f lavfi -i testsrc2=size=1280x720:rate=24 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 19 -c:v libx264 -g 96 -keyint_min 96 -sc_threshold 0 -c:a aac -movflags +faststart 'Fixture (2026).mp4'
ffmpeg -i 'Fixture (2026).mp4' -c copy 'Fixture (2026).mkv'
ffmpeg -f lavfi -i sine=frequency=440:sample_rate=44100 -t 120 -c:a flac -metadata artist='Fixture Artist' -metadata album='Fixture Album' -metadata title='Track 01' -metadata track=1 'Track 01.flac'
```

Make three TV copies named `Fixture Show/Season 1/Fixture Show - S01E01.mkv` through `S01E03.mkv`; make a second version/edition copy and a two-part film. Write a UTF-8 sidecar `Fixture (2026).en.srt` containing `1`, `00:00:02,000 --> 00:00:05,000`, and `Fixture caption`, separated by newlines and followed by a blank line. For long buffering/background/download cases generate a 20-minute/2+GB synthetic file by raising duration/bitrate and verify its hash before testing; a 19-second clip cannot establish those cases.

For HDR10/HLG/Dolby Vision 5/7/8, TrueHD/DTS-HD/Atmos, PGS/VobSub/DVB, HE-AAC/SBR and attached-font ASS use legally held test clips/disc or broadcast extracts. Log provenance, ffprobe facts and a correctly rendered reference frame/audio/channel map. SDR bars with HDR tags are not HDR-fidelity evidence. The server checkout has `fixtures/audio-gapless/` and `fixtures/audio-gapless/generate-portable.py`; preserve its original corpus and generate extras with `python3 fixtures/audio-gapless/generate-portable.py --out /tmp/portico-audio-extra`. Obtain measured HE-AAC/PS fixtures separately. Missing real formats block their cases. Make bad-input copies separately: truncate a copy, corrupt a metadata row, or insert a malformed subtitle cue between good cues. Record which data changed.

## Build or install

Use a sealed candidate stage plus the matching web and qualified media bundle from the release owner, and verify the independently pinned public key, manifest signature, artifact and toolchain hashes before installation. Record preceding/candidate versions with matching baseline digests. Packaging inputs are `STAGE` (server, manifest/signature and third_party), `WEB` (built web with index.html), `VERSION`, and an absolute `OUT` folder. On a dedicated qualification runner, from the server repository:

```sh
bash scripts/package-release.sh OS ARCH VERSION STAGE WEB OUT BUILD_NUMBER
```

Replace each uppercase argument with its actual value; OS is linux/windows/darwin, ARCH is amd64/arm64. `nfpm`, NSIS `makensis`, `zip`/`tar` and matching platform tools are needed. Linux emits tar/deb/rpm, Windows ZIP/Setup.exe, Darwin a payload used by `package-macos.sh`. A missing manifest, wrong media target or failed seal is a build failure; get the correct approved input. Do not manufacture a signature or change a digest to make it pass. `scripts/build-server-release.py --version VERSION --os OS --arch ARCH --ffmpeg-bundle BUNDLE` creates a sealed stage and prints its directory, but runs full race tests: it belongs to the later release runner and must not be invoked by a lane under the programme's limited-testing rule. Keys stay in protected files named by PORTICO_RELEASE_SIGNING_KEY_FILE, PORTICO_RELEASE_PUBLIC_KEY_FILE and PORTICO_ROOT_PUBLIC_KEY_FILE.

For focused platform tests on a qualification checkout, use the Go version in server/go.mod, cached/downloaded locked dependencies and `GOFLAGS=-p=2`; no compile stubs. Run only named tests. A missing reviewed helper is resolved by `GOFLAGS=-p=2 go -C server build -o /tmp/portico-test-helper ./cmd/server` on Unix and setting PORTICO_PLAYBACK_TEST_HELPER to that absolute binary; use an appropriate absolute .exe path on Windows. Do not interpret an environment SKIP as acceptance.

From an elevated PowerShell install the matching Setup.exe. For independent compilation use `makensis /V4 /DOUTPUT_FILE=C:\Evidence\Portico-Setup.exe /DSTAGE_DIR=C:\Inputs\payload /DPRODUCT_VERSION=0.1.0 packaging\windows\installer.nsi`. Use the actual candidate version. Test both interactive and `/S` silent installation in clean snapshots. `sc.exe qc PorticoMediaServer`, `sc.exe queryex PorticoMediaServer`, `sc.exe qsidtype PorticoMediaServer` and `icacls.exe "$env:ProgramData\Portico Media Server"` show its configuration. Console port is 32500. Unresolved Go modules require the declared module cache; missing media tools require the matching sealed bundle. Service login/access errors require the intended SID/account ACL, not an administrator runtime bypass.

## Smoke subset

Run cases 1, 2, 3, 4 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. Service install, reboot, upgrade and uninstall — smoke

1. Install under a path containing spaces, an ampersand and accented text, complete setup, and inspect the named service, service SID, ProgramData ACL and Portico firewall rule.
2. Reboot without signing in. Require readiness and one service parent/worker pair. Trigger owner Restart and then restore; compare service parent and worker PIDs.
3. Set an explicit share account and recovery/start policy; upgrade interactively and silently while running. Force stop failure in a VM snapshot before an upgrade. Uninstall and check disk/state/processes.

**Expected:** Automatic service start before login, unrestricted service SID, private state ACL and TCP 32500 rule; no scheduled task or batch startup launcher. Restart/restore changes worker only. Upgrade preserves account credentials/configuration and catalogue; failed stop leaves payload intact. Uninstall removes service/firewall/install files, retains state/media, leaves no process.

**Failure:** Lost UNC credentials/state, duplicate process, worker startup errors missing from logs, payload overwritten after failed stop or scheduled-task regression.

**Route to:** ops-packaging, server-docs; fix-pass platform-packaging.

### 2. Restore, rollback and sign-out — smoke

1. Sign in on two devices. In Server > Storage & backups create a manual backup; copy the complete backup directory, including identity keys, to a separate test disk. Change a title and saved state, then Restore that backup and watch copying/checking/restart progress.
2. Verify restored state, then sign in again and restart normally. Stage the automatic pre-restore copy by its listed ID and by its absolute folder. Repeat five restores and inspect retained local recovery copies.
3. On a stopped disposable installation corrupt a copied manifest/hash, add a reserved/escaping path, and try restore. Separately stage a bare database from another disposable server whose identity keys are absent.

**Expected:** Valid restore succeeds beyond ten seconds, restores the selected rows/identity and signs people out once; ordinary restart preserves the new sessions. Both recovery-copy forms stage; only three automatic local recovery copies remain and manual backups survive scheduled pruning. Bad manifests refuse before live state moves; a foreign bare DB rolls back at restart with a useful reason and old state starts.

**Failure:** Revived sessions, partial state, repeated sign-out, a hash regenerated to bless corruption, lost manual copy, unbounded memory while hashing, or a boot loop.

**Route to:** ops-packaging, review-ops, accounts-security, review-accounts-security; fix-pass identity-ops-live.

### 3. Delete only the approved files — smoke

1. Enable deletion in the fixture library. Watch a film with two versions; preview Delete this version, replace that file without scanning, then confirm using the typed title. Repeat with the unchanged version.
2. Preview a show/season deletion; add an episode/version after preview, make one original file non-removable using service-account ACLs, then confirm. Retry/cancel after partial failure.
3. Replace a parent with an outside-root symlink/junction between preview and confirmation. Also attempt deletion with deletion disabled.

**Expected:** Changed/outside-root/newly attached files remain; unchanged approved version alone disappears on disk, retained title ID/progress stays. Partial results identify retained files and reasons; no trash copy/folder is created; disabled deletion cannot proceed.

**Failure:** An outside/new/replaced file removed, whole title erased after one failed unlink, disk bytes copied into trash, or retry deleting a newly added member.

**Route to:** identity, review-identity, server-followups, console.

### 4. Missing titles, unmounted drives and library removal — smoke

1. Watch/save a title. Delete its file outside Portico in a readable root, scan and inspect its Missing date/card and Server > Storage. Re-add a renamed/better copy and scan.
2. Unmount the fixture volume so its mount point is empty; repeat with unreadable root, replacement volume and hidden-only content. Scan and report a removed path while offline.
3. Set missing-title cleanup to Immediately, scheduled and Never in turn on separate fixtures. Remove missing titles, re-add the provider-identifiable watched file, then remove the library using its exact typed name while another library plays.

**Expected:** Real disappearance becomes missing and a recognized replacement attaches to the same title. Offline/unproven absence preserves inventory/state. Cleanup follows the chosen rule; provider identity restores watched state after title removal. Library vanishes from clients within about 30 seconds, removal job finishes, files remain and other playback/browse answers.

**Failure:** Empty mount causes missing cleanup, state lost on recognized re-add, library removal deletes disk files or blocks other requests.

**Route to:** identity, review-identity, scan-loop, review-scan, freshness-server, native-sync.

### 5. Large backup, interrupted apply and maintenance

1. Use a multi-GB disposable catalogue/WAL larger than available RAM, ongoing scanner writes and a slow separate backup disk. Create a backup while browsing; record RSS/WAL/free space and duration.
2. Interrupt the worker during restore copy, verification and completion in separate restored VM snapshots. Also deny result-file publication, then repair permissions and restart.
3. Create manual/scheduled backups and reports with a short retention; restart and let maintenance run. Delete catalogue rows to create free pages and inspect storage reclamation. Test migration-copy publication failure only on a matching supported candidate fixture.

**Expected:** Backup is consistent/listed or fails within configured growth/time/space bounds and releases the pinned snapshot; no partial copy is listed. Restore retains old or committed new state, resumes markers and records completion once. Pruning preserves manual copies/current data/today’s report budget; reclamation is paced, no foreground vacuum. A failed recovery-copy publication prevents migration.

**Failure:** WAL grows unchecked, snapshot remains pinned after failure, silent state loss, repeated completion, large whole-file allocation or foreground stalls.

**Route to:** ops-packaging, review-ops, leftovers-server-a, leftovers-server-b, server-followups.

### 6. Failure page, log rotation and process drain

1. Stop only the fixture service, preserve state, deny its state directory or use a disposable corrupt DB; start and open / plus /v1/readiness. Correct the cause and restart.
2. Cause an early startup error, then generate ordinary runtime messages exceeding the 4 MiB rotation threshold. Restart twice and inspect current/previous logs and a downloaded client report.
3. While conversion/scan/recording runs, stop the service. Track parent/worker/helper/FFmpeg PIDs with creation times; kill just the worker on a fresh run and check descendant containment.

**Expected:** Recovery page stays reachable with useful redacted guidance and readiness 503; corrected state starts. Startup evidence persists once per attempt, runtime logs rotate (default four files) without duplication. Stop returns after worker/helpers/grandchildren exit; no credentials/private URLs in reports.

**Failure:** Blank recovery, secrets/path leakage, duplicated unbounded logs, orphan helper or service management returns before drain.

**Route to:** ops-packaging, limits-settings, review-ops; fix-pass platform-packaging.

### 7. Native watchers, permissions and source identity

1. Scan ordinary files, a root link and inside/outside links. Deny one child subtree while deleting/adding healthy siblings; rescan. Add/delete/overwrite video, NFO, artwork and subtitles with monitoring on, then off.
2. Overflow a watcher burst, rename/delete the whole watched folder and pause/resume/cancel a scan. Remount network/FUSE storage in a different device order and rescan unchanged files.
3. Retag a version with contradictory provider/release/narration IDs; test two roots with coincident filename/size/time/inode facts.

**Expected:** Healthy rows complete with named skip diagnostics; unreadable subtrees remain available; explicit named removals in proven roots are immediate. Watcher paths reach limited scans, fallback covers lost events, monitoring off prevents automatic scans. No phantom replacement or cross-root asset reuse; contradictory identity never steals old personal state.

**Failure:** False missing rows, all files reprobed solely due to device number, leaked handles, scan helper survives cancellation or different roots share asset tokens.

**Route to:** scan-loop, review-scan, naming, identity, review-identity; fix-pass scanning-metadata.

### 8. Platform media, tags, subtitles and previews

1. Scan film/TV/audio samples with embedded title/IDs, WEBP cover, chapters/lyrics, NFO/OPF and legacy text. Use temp/subtitle paths containing spaces, commas, apostrophes and Windows drive colons where relevant.
2. Play direct, remux and fixed quality; select SRT/ASS/PGS/VobSub/DVB; seek 0→12→6. Scan >32 tracks and raise/lower the owner subtitle limits; cancel/retry a cold track from a large slow NAS file.
3. Choose Complete/Custom analysis and compare square/anamorphic/HDR10/HLG/DV5 thumbnails and a partly damaged file with a trusted reference.

**Expected:** Useful metadata/cues survive a bad row; extracted tracks/fonts preserve timing and confinement. Slow progressing extraction can exceed 90 seconds but remains within its two-hour outer bound; two-minute idle stops it. Source replacement/revocation invalidates grants. Previews have correct SDR colour/aspect/SAR and optional bad stages leave usable catalogue/playback.

**Failure:** Platform extraction unavailable, wrong MIME/drive escaping, stretched previews, fonts lost, one bad track blocks analysis, source replacement publishes stale output or unsupported DV stage poisons everything.

**Route to:** subtitles-text, music-audiobooks, audio-engines, leftovers-server-a, conversion-outputs; fix-pass catalogue-playback.

### 9. Windows retained handles and junctions

1. Run from server with GOFLAGS=-p=2: `go test ./internal/storage -run "TestWindowsLinkedRoot|TestRootRegistration|TestLockedScanInput|TestWindowsInventoryCommand|TestWindowsEmbeddedLyric|TestWindowsProbeDeadlineEndsHelperAndDescendant|TestLocalObjectBinding" -count=1 -v`.
2. Scan an ordinary root, root junction, directory symlink, volume mount, ReFS and UNC share. Rename a file, then replace/rescan it. Retarget a root link while acquisition is in flight; use traversal-only parent/readable leaf ACLs.
3. Browse a root junction in the folder picker; browse an internal junction escaping the admitted root.

**Expected:** Every selected Windows test executes, usable scans/probes/playback occur on real files, rename preserves proven identity and replacement changes it. Admitted linked roots work; unsafe internal/name-surrogate paths refuse; concurrent retargeting publishes no inconsistent identity. Inventory helper and descendants obey five-minute deadline.

**Failure:** A cross-compile offered as runtime proof, all local media unavailable, outside-root access, inconsistent token or hung inventory helper.

**Route to:** ops-packaging, scan-loop, accounts-security; fix-pass platform-packaging.

### 10. Portable launch and state diagnostics

1. Unpack full ZIP into a writable path with spaces, run start-portico.cmd from a different current directory and finish setup. Stop/restart; repeat with an absolute PORTICO_STATE_DIR.
2. Call the owner state-permission repair on Windows through Troubleshooting and inspect ACLs before/after.

**Expected:** Web/tools resolve adjacent to launcher and default state stays only beside it; override is honored. If NTFS ACL repair is unavailable, it refuses clearly without a POSIX exposure warning or ACL mutation.

**Failure:** State in calling directory, missing web/tools, changed ACLs after an unsupported repair.

**Route to:** ops-packaging; fix-pass platform-packaging.
