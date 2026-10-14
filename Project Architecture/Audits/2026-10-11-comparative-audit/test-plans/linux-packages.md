# Linux packages acceptance

## Purpose and evidence

Updated for the evening implementation and owner decisions. New cases remain qualification work; known missing candidate implementation is Fail, while an unavailable test prerequisite is Blocked. Native-owner recorded 655 native/87 Vega before the mirror; the later running log confirms 655/87 again after the mirror with clean type checks and equal packages. Physical acceptance remains unrun.

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Debian 12/Ubuntu 24.04 and Fedora 41+/Rocky 9 disposable VMs with systemd PID 1, x64 and ARM64 when available; sudo, nfpm, matching sealed tools/web, two package versions sharing baseline. Real NAS/NFS/SMB/FUSE and a removable disk are needed for source identity acceptance.

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

Install the candidate `.deb` with `sudo apt install ./portico-media-server-linux-x64.deb` or `.rpm` with `sudo dnf install ./portico-media-server-linux-x64.rpm` (arm64 filename on ARM64). Run `systemctl status portico-media-server` and open port 32500. State is `/var/lib/portico-media-server`, runtime user `portico`. Before external writes use `sudo systemctl edit portico-media-server` with additive `[Service] ReadWritePaths=/srv/fixture-media /srv/fixture-recordings` and `Environment=PORTICO_RECORDINGS_DIR=/srv/fixture-recordings`; create/ACL those paths for portico, then daemon-reload/restart. Preserve strict hardening. Certificates need narrow read/traversal ACLs. Missing nfpm is a runner dependency; permission denial needs the selected DAC and sandbox path grant, not disabling ProtectSystem.

## Smoke subset

Run cases 1, 2, 3, 4, 5 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. First install and running/stopped upgrades — smoke

1. Install preceding package, record `systemctl is-active`, `is-enabled`, ExecMainStartTimestamp and catalogue. Upgrade to candidate while active; verify build/state and changed start time.
2. Stop, downgrade to the same-baseline preceding fixture if needed, then upgrade again; repeat with disabled service. Remove using apt/dnf and inspect process/status/data.
3. On separate Debian VM set policy-rc.d to deny service start, install/upgrade and observe policy. Test package install in an image/chroot without systemd.

**Expected:** First install enables/starts when policy permits. Active upgrade restarts; stopped/disabled stays so; removal stops/disables but retains state/media. Denied Debian policy is honored; no-systemd installation succeeds. If preceding package predates fixed scriptlets, record that separate legacy-fixture limitation and use an explicit enable/start only afterward.

**Failure:** Silent stopped active upgrade, disabled state reset, state/media deleted, policy-rc.d ignored or chroot service-start failure.

**Route to:** ops-packaging, leftovers-server-b, server-docs; fix-pass identity-ops-live.

### 2. Selected write paths and private state boundary — smoke

1. Check `systemctl cat/show portico-media-server` for ProtectSystem=strict, ProtectHome=read-only, NoNewPrivileges, PrivateTmp and only selected ReadWritePaths. Verify external deletion fails before adding the intended drop-in, then succeeds afterward.
2. Record a fixture tuner to the selected recording directory, load a readable owner certificate, and test a selected media directory under /home. Attempt an unrelated external write from a service-context qualification harness.
3. Bind-mount fixture media at state/mounts, place hard-linked media under state/recordings, save entry modes, loosen state root to 0755 and use owner permission-fix; compare all modes.

**Expected:** Only selected external writes work and unrelated paths stay read-only. State root becomes 0700; mounts, descendants and external hard-linked media modes remain identical; private root with loose artwork causes no exposure warning/tree walk.

**Failure:** Global hardening removed, outside write succeeds or media permissions changed by state repair.

**Route to:** ops-packaging, review-ops; fix-pass platform-packaging.

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

### 7. Failure page, log rotation and process drain

1. Stop only the fixture service, preserve state, deny its state directory or use a disposable corrupt DB; start and open / plus /v1/readiness. Correct the cause and restart.
2. Cause an early startup error, then generate ordinary runtime messages exceeding the 4 MiB rotation threshold. Restart twice and inspect current/previous logs and a downloaded client report.
3. While conversion/scan/recording runs, stop the service. Track parent/worker/helper/FFmpeg PIDs with creation times; kill just the worker on a fresh run and check descendant containment.

**Expected:** Recovery page stays reachable with useful redacted guidance and readiness 503; corrected state starts. Startup evidence persists once per attempt, runtime logs rotate (default four files) without duplication. Stop returns after worker/helpers/grandchildren exit; no credentials/private URLs in reports.

**Failure:** Blank recovery, secrets/path leakage, duplicated unbounded logs, orphan helper or service management returns before drain.

**Route to:** ops-packaging, limits-settings, review-ops; fix-pass platform-packaging.

### 8. Native watchers, permissions and source identity

1. Scan ordinary files, a root link and inside/outside links. Deny one child subtree while deleting/adding healthy siblings; rescan. Add/delete/overwrite video, NFO, artwork and subtitles with monitoring on, then off.
2. Overflow a watcher burst, rename/delete the whole watched folder and pause/resume/cancel a scan. Remount network/FUSE storage in a different device order and rescan unchanged files.
3. Retag a version with contradictory provider/release/narration IDs; test two roots with coincident filename/size/time/inode facts.

**Expected:** Healthy rows complete with named skip diagnostics; unreadable subtrees remain available; explicit named removals in proven roots are immediate. Watcher paths reach limited scans, fallback covers lost events, monitoring off prevents automatic scans. No phantom replacement or cross-root asset reuse; contradictory identity never steals old personal state.

**Failure:** False missing rows, all files reprobed solely due to device number, leaked handles, scan helper survives cancellation or different roots share asset tokens.

**Route to:** scan-loop, review-scan, naming, identity, review-identity; fix-pass scanning-metadata.

### 9. Platform media, tags, subtitles and previews

1. Scan film/TV/audio samples with embedded title/IDs, WEBP cover, chapters/lyrics, NFO/OPF and legacy text. Use temp/subtitle paths containing spaces, commas, apostrophes and Windows drive colons where relevant.
2. Play direct, remux and fixed quality; select SRT/ASS/PGS/VobSub/DVB; seek 0→12→6. Scan >32 tracks and raise/lower the owner subtitle limits; cancel/retry a cold track from a large slow NAS file.
3. Choose Complete/Custom analysis and compare square/anamorphic/HDR10/HLG/DV5 thumbnails and a partly damaged file with a trusted reference.

**Expected:** Useful metadata/cues survive a bad row; extracted tracks/fonts preserve timing and confinement. Slow progressing extraction can exceed 90 seconds but remains within its two-hour outer bound; two-minute idle stops it. Source replacement/revocation invalidates grants. Previews have correct SDR colour/aspect/SAR and optional bad stages leave usable catalogue/playback.

**Failure:** Platform extraction unavailable, wrong MIME/drive escaping, stretched previews, fonts lost, one bad track blocks analysis, source replacement publishes stale output or unsupported DV stage poisons everything.

**Route to:** subtitles-text, music-audiobooks, audio-engines, leftovers-server-a, conversion-outputs; fix-pass catalogue-playback.

### 10. Portable tar and selected hardware access

1. Unpack complete tar to a path with spaces and launch start-portico.sh from another directory; repeat explicit state override and restart.
2. On a real GPU host add only numeric render/video groups needed by the runtime, retain service hardening and play a forced hardware conversion; capture backend/probe/stages.

**Expected:** Web/tools/state resolve correctly, umask protects state; qualified device access works inside the normal executor. Unsupported device reports fallback honestly.

**Failure:** Missing tools/web, state placed in calling directory or claimed hardware output without an actual device stage.

**Route to:** ops-packaging, conversion-outputs, planner-wiring.

### 11. Naming, versions, releases, ignores and tolerant tags

1. Use fresh fixture roots and filenames from server/internal/naming/testdata/corpus.json. Make generic movie filenames with folder year/IDs, meaningful conflicting filenames, S02E01 in Season1, E01/01 in Season2, seasonless/flat/absolute anime, episode0, editions/parts/extras, BDMV/VIDEO_TS and sample/system files. Scan and inspect exact title/coordinates/extra owner.
2. Add nested ignore file/change it/rescan; put Film(2020).mkv plus same-stem SRT directly in root and >4096 unrelated flat-folder entries. Use resolution tag1920x1080 and title1917/2001, NFC/NFD names and malformed optional NFO/tag/chapter values.
3. Scan two copies of same MusicBrainz release across artist/year/folders, partial IDs and differing release/edition/narration IDs; retag one version to contradictory release and re-add watched music/book under new path. NFO directors/writers/tags/ratings/runtime/trailing URL then remove plot/NFO and reanalyse.

**Expected:** File/embedded evidence wins conflict; correct stacks/coordinates/extras, no disc stream mistaken for poster, samples/hidden entries excluded without stalled pages. Useful fields retained with named skips, root sidecar no false warning. True releases share, contradictions separate and former title personal state remains; owner locks/undo preserved, unlocked removed NFO fields return to provider.

**Failure:** Wrong season from resolution, empty numeric title search, duplicate/split true release or contradictory version steals title, one bad optional field blocks all import, inherited ignores mark offline roots missing.

**Route to:** naming, scan-loop, review-scan, providers-matching, music-audiobooks, base-health; fix-pass scanning-metadata.

### 12. Real metadata providers, locale, artwork and outage recovery

1. On private libraries choose default TMDB then TheTVDB with authorized test keys; import yearless/misspelled/numeric titles and absolute anime spanning seasons. Fix match/history/undo, alternate group/offset order and explicit refresh; record actual request counts.
2. Change language/country/provider with crosswalk evidence; inspect seasonNN-poster/show/episode thumbs, source/shared actor image download dedup and TheTVDB companyType/companyTypeName Network/Studio filters. Compare NFC/NFD transferred macOS folders.
3. Set local-only and repeat with no provider traffic. Under controlled provider proxy inject429 Retry-After/prolonged outage then restore; speed gentle/normal/fast and immediate manual search. Lower/raise local and provider artwork quotas; duplicate image bytes count once.

**Expected:** Decent-confidence matches rather than arbitrary unmatched; correct provider/order/locale/roles and manual selections stay pinned. Local-only zero remote traffic, retained source/images reused; capacity warning then recovery, useful live company fields. Retry floor/concurrency bounds and ≤five-minute idle safety wake honored; UI stays reachable.

**Failure:** Fake provider-only test claimed live schema proof, wrong episode still/order, silent overwrite of locked art, quota charges wrong category, outage never recovers or expired token/login loop.

**Route to:** providers-matching, scan-loop, leftovers-server-a, server-followups; fix-pass scanning-metadata.

### 13. Rebuild, poison title, retained generations and bounded scale

1. On a private qualification copy run `GOFLAGS=-p=2 go -C server run ./internal/catalog/scalebench -dir .scratch/accept-scale -items 10000 -samples 5 -profile -explain title`. Preserve emitted query/latency/hold/RSS evidence; scale 200000/2000000 only on a dedicated capacity host. No benchmark was executed by this lane.
2. Use existing compactcatalog generation/poison/retry fixtures for deterministic version change/restart; physical copy needs explicit test hook. While actual populated rebuild/import runs browse/search owner/restricted clients, remove library and run second generation/FTS integrity plus orphan dictionary/event/history pruning.
3. On weak hardware run watcher storms/blocking proof helper/slow NAS and 200 real subscriptions/100 direct consumers; record p95/p99 waits, DB statement/write-gate max holds, dispatch delay, WAL and foreground response compared with scan paused.

**Expected:** Previous content always answers (no preparing), poison isolated/reason durable/fix retry, current generation/reference integrity. Work/checkpoints/candidate/absence proof shared two-second budget bounded; indexed page cost stable, console reachable and private progress audience isolated. Report measured capacity rather than assuming target met.

**Failure:** Copied startup whole catalogue, blank old generation, poison stalls all workers, full-inventory joins/temp sorts, false claim that two-million-row index proof equals mixed catalogue/device capacity.

**Route to:** freshness-server, review-freshness, review-scan, search-sort, grouping-sorts, scale-check, leftovers-server-a; fix-pass catalogue-playback.

### 14. Owner broadcasts, watchdog and recovery at scale

1. Create >500 disposable profiles using supported setup fixture, send same owner broadcast operation/body, cancel between pages then restart/replay; send changed body same key and revoke owner between pages.
2. In dedicated debug qualification harness hold actual write gate>20s with strict panic OFF; watch degraded health/one holder-class log then release. Run actual long backup/import and correlate class-specific holds rather than multiplying drain time.
3. Save short report retention/restart/maintenance and inspect global cap/dated cursors/redaction/1MiB daily budget.

**Expected:** One notice per traversed inbox and cumulative receipt, changed key body refused/revocation stops later pages. Watchdog reports/recovery truthful, no production panic. Report pruning bounded/current budget intact with exact millisecond dates and private diagnostic contents.

**Failure:** Duplicate broadcast on restart, continuing after authority lost, runaway watchdog logging/panic or unrelated process halt treated as measured write-gate problem.

**Route to:** leftovers-server-b, leftovers-server-a, ops-packaging, console.

### 15. Final baseline startup and restore key inventory

1. Start a new empty candidate state. Identity-bound triggers and counters are accepted and setup completes. In separate copied states remove/alter an identity trigger/accounting row: recovery must refuse the damaged schema; do not weaken the verifier.
2. Create key A; take backup; create key B and revoke A; change server name. Restore the backup with its confirmation and wait for supervised restart. The backed-up name/catalogue return, old interactive sessions are refused and a fresh sign-in works.
3. Key A exists again as it did at backup time; B does not exist. Both INSERT VALUES and INSERT SELECT baseline seed tables restore correctly, with no lost bounds/accounting seeds. Validate request expiry and narrower permission grants survive transactional authentication; forged wider grants refuse.

**Expected:** Fresh exact baseline starts, altered baseline refuses, restore keeps backup-time keys while ending interactive sessions and reconstructing all required seeds. This is the owner’s chosen key policy.

**Failure:** Fresh install recovery loop, weakened schema comparison, keys all revoked despite backup policy, post-backup key survives or transaction drops expiry/grant restrictions.

**Route to:** base-health-3, accounts-security, queue-and-retention.

### 16. Bounded housekeeping and preserved owner history

1. On a copied disposable baseline create expired receipts/cache/reference rows plus active playback/job/artifact pins. Use Gentle then Fast; observe bounded quanta, shorter/larger budgets and owner pause checked between quanta.
2. In a throwaway database lower one compiled-table ceiling using the qualification fixture. A new key refuses at the bound, exact retry/update succeeds and deleting an eligible row releases capacity. Check child-before-parent cleanup and foreign keys; never modify production ceilings/state to run this.
3. Set play history forever and missing-title cleanup never. Run maintenance repeatedly; both choices and retained watch identity survive. Erase a profile then replay an old command/credential: permanent replay/deletion guards must still refuse resurrection.
4. Retire analysis data that remains referenced; files/summary rows survive until custody permits cleanup, then prune summaries in 32-row batches. Verify actual filesystem readers on this OS and record disk/cache use.

**Expected:** Bounded reclaim without active/reference loss, no orphan keys, exact replay at a ceiling, reusable capacity and preserved forever/never choices. These checks do not establish multiyear churn or universal throughput.

**Failure:** Unbounded writer hold, deleting active bytes/history, replay resurrects authority, ceiling prevents exact retry/update or rows/fence capacity never settles.

**Route to:** queue-and-retention, limits-settings, ops-packaging, base-health-3.

### 17. Deferred Home and large mixed-library qualification

1. Use an integrated candidate that includes scale-check-2’s final snapshot; record that prerequisite. Test 10k, 200k and one-million mixed video/music/books with versions, genres, credits and three restricted profiles. A full two-million fixture is not required under the recorded 8 GB scratch budget; structural tests are separate evidence.
2. Hold one Home preview response. The row manifest arrives first; two row slots work independently, a stalled/failing row times out and other rows appear. Quiet refresh keeps cards. Switch viewer while responses are held: old hero/row/pagination cannot cross the fence.
3. Compare every advertised sort/direction/operator, exact totals and subgroup membership on first/deep pages during empty and version-bump rebuilds; browse/search keep responding with prior content and live restrictions. Exercise broad search prefixes, group missing/personal/play-state predicates and mark/Undo 500/5,000.
4. Record actual latency/refusals/RSS/WAL/disk, writer holds and first visibility after add/remove. Treat one observation as one sample, not p95. Monitor space and preserve logs before deleting each owned database. Test authenticated HTTP/concurrent players separately from catalogue-process benchmarks.

**Expected:** No unavailable/preparing browse reply, independent visible Home rows, exact restricted results and reproducible measured resource outcomes. Capacity claims require explicit targets/sample distribution; pending 200k/one-million runs or structural two-million proof are never called mixed-scale acceptance.

**Failure:** Stalled shelf blocks Home, old viewer cards publish, totals/order/privacy differ, large operation kills control reachability or disk budget exhausted without preserved evidence.

**Route to:** scale-check-2, scale-check, freshness-server, queue-and-retention.
