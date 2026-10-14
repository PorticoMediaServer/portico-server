# Docker acceptance

## Purpose and evidence

Updated for the evening implementation and owner decisions. New cases remain qualification work; known missing candidate implementation is Fail, while an unavailable test prerequisite is Blocked. Native-owner recorded 655 native/87 Vega before the mirror; the later running log confirms 655/87 again after the mirror with clean type checks and equal packages. Physical acceptance remains unrun.

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Private x64 and ARM64 Linux Docker hosts, approved candidate image or dedicated release builder with buildx/QEMU and both qualified Linux media bundles. Persistent named state volume, copied media and optional separate backup/cache volumes; host GPU and render/video groups for device pass-through; user namespaces/bubblewrap for required confinement.

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

Prefer the verified image digest supplied with candidate evidence. Full Docker builds belong to the qualification runner, never this lane. On that runner: `docker buildx build --platform linux/amd64,linux/arm64 --build-arg PORTICO_VERSION=0.1.0 --build-arg PORTICO_BUILT_AT=2026-10-13T00:00:00Z -f Dockerfile .`; supply both locked bundles as described in Dockerfile/RELEASING.md. Export/load a named candidate for each host; record its digest. Start without restart policy: `docker volume create portico-accept-state`, then `docker run -d --name portico-accept -p 32500:32500 -e TZ=America/Halifax -v portico-accept-state:/var/lib/portico -v /absolute/fixture-media:/media:ro IMAGE_DIGEST`. Replace paths/image, grant container user narrow host permissions and keep service private. `docker inspect` shows health and `docker logs` shows startup. Missing bundle inventory, wrong architecture/exec-format, readonly state or cache ownership are failures to fix at their respective input boundary; never bypass verification or run privileged as a cure.

## Smoke subset

Run cases 1, 2, 3, 4, 5 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. Both architectures and persistent state — smoke

1. Start the actual amd64/arm64 candidates independently; verify qualified FFmpeg version and health/readiness then complete setup and scan read-only media.
2. Attempt Delete in read-only media; stop/recreate same image and then upgrade to a same-baseline candidate using the same state volume.
3. Inspect built image provenance, manifests and all required web/subtitle/font/tool assets. Repeat single-platform build with a flat native bundle on the qualification runner.

**Expected:** Both images start on their architecture, health becomes ready and state persists; read-only deletion refuses without losing title. All bundles match target/hash; no system FFmpeg substitution.

**Failure:** Wrong architecture, lost volume, missing web/font/tool files, runtime writes media despite ro or invalid seal accepted.

**Route to:** ops-packaging, server-docs; fix-pass identity-ops-live.

### 2. Restart, restore, stop and init reaping — smoke

1. With no restart policy record container PID and worker, trigger owner Restart then perform a valid restore.
2. Run media/scan helpers and `docker stop --time 45 portico-accept`; examine PIDs/zombies before/after. On a separate fixture kill just worker while a helper/grandchild exists.
3. If overriding ENTRYPOINT, explicitly use Docker --init and repeat.

**Expected:** Container parent survives owner restart/restore, worker changes/readiness returns. Tini forwards stop/drains/reaps children; no orphan/zombie buildup or reliance on restart policy.

**Failure:** Container exits on owner restart/restore, orphan worker or stop exceeds allowed drain with surviving processes.

**Route to:** ops-packaging, review-ops.

### 3. Restore, rollback and sign-out — smoke

1. Sign in on two devices. In Server > Storage & backups create a manual backup; copy the complete backup directory, including identity keys, to a separate test disk. Change a title and saved state, then Restore that backup and watch copying/checking/restart progress.
2. Verify restored state, then sign in again and restart normally. Stage the automatic pre-restore copy by its listed ID and by its absolute folder. Repeat five restores and inspect retained local recovery copies.
3. On a stopped disposable installation corrupt a copied manifest/hash, add a reserved/escaping path, and try restore. Separately stage a bare database from another disposable server whose identity keys are absent.

**Expected:** Valid restore succeeds beyond ten seconds, restores the selected rows/identity and signs people out once; ordinary restart preserves the new sessions. Both recovery-copy forms stage; only three automatic local recovery copies remain and manual backups survive scheduled pruning. Bad manifests refuse before live state moves; a foreign bare DB rolls back at restart with a useful reason and old state starts.

**Failure:** Revived sessions, partial state, repeated sign-out, a hash regenerated to bless corruption, lost manual copy, unbounded memory while hashing, or a boot loop.

**Route to:** ops-packaging, review-ops, accounts-security, review-accounts-security; fix-pass identity-ops-live.

### 4. Delete only the approved files — smoke

1. Enable deletion in the fixture library. Watch a film with two versions; preview Delete this version, replace that file without scanning, then confirm using the typed title. Repeat with the unchanged version.
2. Preview a show/season deletion; add an episode/version after preview, make one original file non-removable using service-account ACLs, then confirm. Retry/cancel after partial failure.
3. Replace a parent with an outside-root symlink/junction between preview and confirmation. Also attempt deletion with deletion disabled.
4. After physical unlink use the source review-server-late regression’s second authorization callback to rename the title/demote the actor before final catalogue mutation; runtime timing requires a qualification hook. Check deleted bytes are unavailable even if later mutation is refused.

**Expected:** Changed/outside-root/newly attached files remain; unchanged approved version alone disappears on disk, retained title ID/progress stays. Partial results identify retained files and reasons; no trash copy/folder is created; disabled deletion cannot proceed. Physically deleted files reconcile as unavailable despite a later conflict/refusal, without gaining authority to unlink anything else.

**Failure:** An outside/new/replaced file removed, whole title erased after one failed unlink, disk bytes copied into trash, or retry deleting a newly added member.

**Route to:** identity, review-identity, server-followups, console, review-server-late.

### 5. Missing titles, unmounted drives and library removal — smoke

1. Watch/save a title. Delete its file outside Portico in a readable root, scan and inspect its Missing date/card and Server > Storage. Re-add a renamed/better copy and scan.
2. Unmount the fixture volume so its mount point is empty; repeat with unreadable root, replacement volume and hidden-only content. Scan and report a removed path while offline.
3. Set missing-title cleanup to Immediately, scheduled and Never in turn on separate fixtures. Remove missing titles, re-add the provider-identifiable watched file, then remove the library using its exact typed name while another library plays.

**Expected:** Real disappearance becomes missing and a recognized replacement attaches to the same title. Offline/unproven absence preserves inventory/state. Cleanup follows the chosen rule; provider identity restores watched state after title removal. Library vanishes from clients within about 30 seconds, removal job finishes, files remain and other playback/browse answers.

**Failure:** Empty mount causes missing cleanup, state lost on recognized re-add, library removal deletes disk files or blocks other requests.

**Route to:** identity, review-identity, scan-loop, review-scan, freshness-server, native-sync.

### 6. Large backup, interrupted apply and maintenance

1. Use a multi-GB disposable catalogue/WAL larger than available RAM, ongoing scanner writes and a slow separate backup disk. Create a backup while browsing; record RSS/WAL/free space and duration.
2. Interrupt the worker during restore copy, verification and completion in separate restored VM snapshots. Also deny result-file publication, then repair permissions and restart.
3. Create manual/scheduled backups and reports with a short retention; restart and let maintenance run. Delete catalogue rows to create free pages and inspect storage reclamation. Test migration-copy publication failure only on a matching supported candidate fixture.

**Expected:** Backup is consistent/listed or fails within configured growth/time/space bounds and releases the pinned snapshot; no partial copy is listed. Restore retains old or committed new state, resumes markers and records completion once. Pruning preserves manual copies/current data/today’s report budget; reclamation is paced, no foreground vacuum. A failed recovery-copy publication prevents migration.

**Failure:** WAL grows unchecked, snapshot remains pinned after failure, silent state loss, repeated completion, large whole-file allocation or foreground stalls.

**Route to:** ops-packaging, review-ops, leftovers-server-a, leftovers-server-b, server-followups.

### 7. Timezone, proxy/gateway locality and discovery

1. Set TZ=America/Halifax and an overnight local maintenance window including a DST transition on a disposable-clock fixture; inspect next-run times.
2. Observe status.connection through bridge/rootless/tunnel mode. Set trusted proxy only for real forwarding peers; remove hidden gateway ranges or set home networks [] when no original client address exists.
3. Repeat with host networking on a private test host, no -p mapping, and actual multicast discovery across two interfaces/IPv4/IPv6.

**Expected:** Window follows host timezone and overnight/DST policy. Home networks alone governs locality; hidden gateway never makes all remote users home. Discovery is qualified separately, unique interfaces grouped by identity.

**Failure:** Wrong window, inferred overlay locality, fabricated client IP or claimed multicast from a bridge-only test.

**Route to:** networking, review-networking, ops-packaging.

### 8. GPU and confinement inside the container

1. On supported host pass /dev/dri with `--device=/dev/dri` and host numeric render/video `--group-add` values. Force VAAPI/QSV HEVC10 conversion, subtitles and tone mapping, inspect stages/segments.
2. Repeat baseline environment and `PORTICO_DECODER_SANDBOX=required` on a host genuinely allowing namespaces. Test software-only graph and unreadable device without expanding privileges.

**Expected:** Actual device/driver output matches declared depth/range/audio and normal grants. Baseline accurately reports confinement status; required confinement either operates genuinely or refuses clearly.

**Failure:** A successful unconfined run claimed as sandbox proof, privileged bypass, wrong vendor packages on arm64 or black/green output.

**Route to:** ops-packaging, conversion-outputs, planner-wiring; fix-pass hardware-conversion.

### 9. Release source, bundle and final-artifact custody

1. In isolated scratch repository/CI run manual release tagA while dispatch branch pointsB; inspect resolved checkout/test/build/artifact SHA and move tag after resolution on a separate fixture.
2. Verify both qualified amd64/arm64 bundle hashes and architecture in buildx stage, package completeness, independently pinned public key and required media/performance gates.
3. Alter final server/web/package/update-feed bytes separately and run final-artifact signature/custody verification; preserve original signed copy. Do not publish/deploy to production.

**Expected:** All jobs/artifacts follow resolvedA, moved tag refuses publication, wrong architecture/bundle and tampered final artifact fail; derivative macOS seal retains approved source provenance, update reader can follow permitted real HTTPS asset chain. Scratch CI evidence only.

**Failure:** DispatchB tested for tagA, unpinned supplied key self-verifies artifact, package/web tamper accepted or publication performed without authorization.

**Route to:** ops-packaging, review-ops, server-docs; fix-pass identity-ops-live, hosted.

### 10. Final baseline startup and restore key inventory

1. Start a new empty candidate state. Identity-bound triggers and counters are accepted and setup completes. In separate copied states remove/alter an identity trigger/accounting row: recovery must refuse the damaged schema; do not weaken the verifier.
2. Create key A; take backup; create key B and revoke A; change server name. Restore the backup with its confirmation and wait for supervised restart. The backed-up name/catalogue return, old interactive sessions are refused and a fresh sign-in works.
3. Key A exists again as it did at backup time; B does not exist. Both INSERT VALUES and INSERT SELECT baseline seed tables restore correctly, with no lost bounds/accounting seeds. Validate request expiry and narrower permission grants survive transactional authentication; forged wider grants refuse.

**Expected:** Fresh exact baseline starts, altered baseline refuses, restore keeps backup-time keys while ending interactive sessions and reconstructing all required seeds. This is the owner’s chosen key policy.

**Failure:** Fresh install recovery loop, weakened schema comparison, keys all revoked despite backup policy, post-backup key survives or transaction drops expiry/grant restrictions.

**Route to:** base-health-3, accounts-security, queue-and-retention.

### 11. Bounded housekeeping and preserved owner history

1. On a copied disposable baseline create expired receipts/cache/reference rows plus active playback/job/artifact pins. Use Gentle then Fast; observe bounded quanta, shorter/larger budgets and owner pause checked between quanta.
2. In a throwaway database lower one compiled-table ceiling using the qualification fixture. A new key refuses at the bound, exact retry/update succeeds and deleting an eligible row releases capacity. Check child-before-parent cleanup and foreign keys; never modify production ceilings/state to run this.
3. Set play history forever and missing-title cleanup never. Run maintenance repeatedly; both choices and retained watch identity survive. Erase a profile then replay an old command/credential: permanent replay/deletion guards must still refuse resurrection.
4. Retire analysis data that remains referenced; files/summary rows survive until custody permits cleanup, then prune summaries in 32-row batches. Verify actual filesystem readers on this OS and record disk/cache use.

**Expected:** Bounded reclaim without active/reference loss, no orphan keys, exact replay at a ceiling, reusable capacity and preserved forever/never choices. These checks do not establish multiyear churn or universal throughput.

**Failure:** Unbounded writer hold, deleting active bytes/history, replay resurrects authority, ceiling prevents exact retry/update or rows/fence capacity never settles.

**Route to:** queue-and-retention, limits-settings, ops-packaging, base-health-3.
