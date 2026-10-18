# Review — platform-packaging lane

Reviewer: independent read (Claude). Lane author: GPT 6.1 Sol via Codex CLI.
Worktree: `/home/justin/projects/Portico/.worktrees/audit-fixes/server-platform-packaging`, branch `audit-fixes/platform-packaging`, base 82aaa54, eight commits. Nothing in the worktree was modified (`git status --short` empty before and after).

## How this was checked

None of these fixes can run on this Linux host, so the evidence is: reading the code and its callers, cross-compiling, the lane's own tests, and reading Go's Windows runtime source (`$GOROOT/src/os`, go1.27.1) for the behaviour the fixes rely on.

**Cross-compile.** `GOOS=windows go build ./...` fails offline exactly as the lane reported: `github.com/mattn/go-isatty v0.0.24` and `github.com/ncruces/go-strftime v1.0.0` (Windows-only dependencies of `modernc.org/libc`) are not in `/home/justin/go/pkg/mod`. Nothing was downloaded (`GOPROXY=off`). To get past that without downloading, I built with an alternate module file kept in the scratchpad (`go build -modfile=<scratch>/go.mod`) that replaces only those two leaf modules with two-line stubs (`isatty.IsTerminal`, `strftime.Format`, the only symbols libc uses). All Portico code and every other dependency is the real source. Results:

| Command (with the two stubbed leaf modules) | Result |
| --- | --- |
| `GOOS=windows GOARCH=amd64 go build ./...` | PASS |
| `GOOS=windows GOARCH=arm64 go build ./...` | PASS |
| `GOOS=darwin GOARCH=arm64 go build ./...` | PASS |
| `GOOS=windows GOARCH=amd64 go vet ./internal/storage/... ./internal/subtitles/... ./internal/backup/... ./internal/decoder/... ./internal/subtitlevideo/... ./cmd/server/... ./internal/servicelog/... ./internal/httpapi/... ./internal/mediaexec/...` (this also compiles the Windows-only tests) | PASS |

This is a compile and vet check of the whole Windows server, which the lane did not have. It is not a run. A real `GOOS=windows` build needs those two modules in the cache (one `go mod download` on a networked machine).

**Native (Linux) tests run:** `go test ./internal/backup ./internal/servicelog ./internal/subtitles ./internal/storage` PASS; `go test ./cmd/server -run TestPackagedMessageLogDoesNotDuplicateRuntimeOutput -v` PASS (both sub-cases); `go vet` on the same packages clean; `python3 scripts/test-portable-packaging.py` 2 PASS; `python3 scripts/test-macos-seal.py` 6 PASS; `bash -n scripts/package-release.sh scripts/package-macos.sh`, `sh -n` on `packaging/macos/launcher.sh`, `packaging/macos/server-launcher.sh`, `packaging/portable/start-portico.sh`, `packaging/linux/postinstall.sh` all clean; the launchd plist parses (`plistlib`), the release workflow parses (PyYAML); `systemd-analyze verify` on a scratch copy of the unit plus a sample drop-in (including a quoted path with a space) reports no directive errors. No PowerShell, NSIS, `codesign` or Windows/macOS host was available, so `register-task.ps1`, `installer.nsi`, `start-portico.cmd` and the real signing path were read only.

---

### 2d0a27d — Identify local Windows files by volume and file index  (bug B-05.28)

- **Verdict: ACCEPT WITH CHANGES.** The code is a correct and necessary first step and changes nothing on Unix. It does **not** fix B-05.28: a Windows owner still cannot get a usable local library. Keep the bug open.
- **What it does:** Adds `observedObjectBinding` for Windows (`local_observation_windows.go`): opens the path with `CreateFile(FILE_READ_ATTRIBUTES, …, FILE_FLAG_BACKUP_SEMANTICS|FILE_FLAG_OPEN_REPARSE_POINT)`, checks the handle is the same object as the `os.FileInfo` the caller already has, and returns `volume:<serial>:file:<64-bit index>` plus NTFS ChangeTime as the change token. The Unix implementations gain an ignored `path` parameter; the "unsupported" stub now excludes Windows.

**What a Windows owner gets after this commit (traced, by reading):**

1. *Add library* — now succeeds. `catalog.CreateAuthorizedThen` (`catalog/catalog.go:147`) → `inspectSource` → `storage.Client.InspectRoot` → helper op `root` (`storage/client.go:199-215`) → `snapshot` → the new binding returns a non-empty root identity. Same for `SaveSource` (`catalog/library_sources.go:262-269`) and the scan's root check (`ingestion/inventory.go:37-51`).
2. *First scan, inventory phase* — should work. On Windows `InventoryPage` uses the retained-stream path shared with macOS (`storage/inventory_page.go:92-94`, `inventory_page_stream.go`, `inventory_page_retained_names.go`), built on `os.Root`, which Go implements on Windows. Files are discovered and committed.
3. *First scan, analysis phase* — **fails for every file.** `ingestion/analysis.go:340` calls `probe.InspectScan` → `assets.Probe.inspect` (`assets/assets.go:114`) → `storage.RunScanCommand` (`storage/scan_content.go:45-105`) → helper op `inventory-command` → `storage/scan_exec_other.go:7-12`, which returns "retained-descriptor scan execution is unsupported on this server platform". The object is marked `probe_or_sidecar_failed` (`ingestion/analysis.go:427-428`), `CommitInventoryAnalysis` (`catalog/inventory.go:385`) is never reached, and that is the only place catalogue items are published. **Net result: the library exists, the scan lists files, every file carries a warning, and there are no titles.**

**Remaining Windows stubs and gaps, in the order the owner hits them:**

| # | Where | What is missing | Needed for |
| --- | --- | --- | --- |
| 1 | `storage/scan_exec_other.go:7-12` (`inventoryCommandHelper`) | No Windows way to run ffprobe/ffmpeg against the admitted file. The Unix version dups the file to descriptor 3 and `exec`s the tool. | Every scan probe — the blocker for "first scan produces titles" |
| 2 | `storage/scan_exec_other.go:16` (`descriptorInput = "/dev/fd/3"`), used at `storage/scan_content.go:80` and `storage/lyrics.go:47` | The tool is told to read `/dev/fd/3`, which does not exist on Windows. A Windows design must pass a path (as `playback_source_windows.go` does) or a pipe. | Same |
| 3 | `storage/scan_content.go:173-182` (`scanHelperCommand`) | Sets `cmd.ExtraFiles`; `childprocess/childprocess_windows.go:71` silently drops it, so the helper has no descriptor 3 even if #1 existed. | Same |
| 4 | `mediaexec/shim_other.go:8-9` (`execShim`, `execInPlace` return `ErrUnsupported`) | `mediaexec.ExecArgv` cannot replace the helper process on Windows; the Windows helper must spawn the tool as a child and wait (the job object already ends it with the server). | Same |
| 5 | Other callers of `RunScanCommand` that fail the same way: `assets/playback_analysis.go:36` (playback facts), `localmetadata/audio.go:81` (embedded cover art) | — | Scan completeness |
| 6 | `storage/lyrics_probe_other.go:10-12` | Embedded lyric probing unsupported. | Music libraries |
| 7 | `storage/root_registration_other.go:11-17` (`duplicateRoot`, `RegisterRoot`, `registerRootHelper`, `physicalMountIdentity`, `sameMountFilesystem` all return `ErrObservedSourceUnsupported`) | No Windows root registration (the Unix one passes a directory descriptor over a socket pair with `SCM_RIGHTS`). Reached through `sourceaccess.Registry.Borrow` (`sourceaccess/registry.go:107`). | Observed playback, playback analysis (`playback/descriptor_analysis.go:71`), embedded subtitle extraction from local files (`subtitlevideo/input.go:146`) |
| 8 | `storage/observed_playback.go:76` and `:376` | `OpenObservedPlayback` and its helper refuse any OS but Linux and macOS (they depend on #7 and on inherited descriptor 3). | Playback of local files through the observed-source path (`playback/observed_local_input.go:54`) |
| 9 | `storage/local_version_other.go:11-13` (`localVersionEvidence` → `ErrIdentityRequired`), used at `storage/versioned_playback.go:218,258` | No Windows object/revision evidence for versioned playback. Its comment says "Windows … remain unqualified". | Versioned local playback |
| 10 | `storage/playback_descriptor_windows.go:14-24` (`DescriptorPassing = false`; `OpenPlaybackDescriptor`, `ValidatePlaybackDescriptor` return `ErrPlaybackSource`) | Direct play falls back to the fully isolated reader (`assets/playback_reader.go:41-52`); that fallback and `playback_source_windows.go` (converted playback, marked UNVERIFIED in its own comment) exist but have never run. | Playback |
| 11 | `storage/mount_windows.go:7-13` (`isMount`, `DirectoryIdentity`), `mounts/service.go:263` | Managed (rclone) mounts unsupported; any request carrying a `MountedRoot` fails in `storage.Helper` (`client.go:137-153`). | Managed remote mounts only — not plain local folders |
| 12 | `storage/inventory_changes_other.go` | No change watcher (same as macOS); not a blocker. | — |

I did not trace playback end to end; rows 7–10 are what the stubs say, not a tested playback path. Rows 1–4 are one piece of work (a Windows `inventory-command`), and they are what stands between this commit and a first scan that produces titles.

- **Findings:**
  1. **Blocker (for closing the bug, not for merging the commit)** — B-05.28 must stay open and Windows must not be described as able to host local libraries. The lane's report says so; the commit subject is accurate ("Identify…"). Make sure the coordinator's status does not read "B-05.28 FIXED".
  2. **Should fix** — `local_observation_windows.go:13-15` (comment), `:21` (share mode), and the lane report: the claim that the handle "holds writers and replacers out" is wrong. Windows only applies share-mode checks to opens that request read-data, write-data or delete access; a `FILE_READ_ATTRIBUTES`-only open is neither checked against existing opens nor recorded, so `FILE_SHARE_READ` here blocks nobody. (Go's own `loadFileId` in `os/types_windows.go` opens with access 0 and share 0 for exactly this reason.) The identity is still sound — all facts come from one open handle, and the file record cannot be recycled while the handle is open — but nothing is locked, and size/mtime (from `file.Stat()`, line 27) and ChangeTime (line 47) come from separate calls, so a concurrent writer can produce a mixed revision (harmless: the next scan sees a new mtime). Change: correct the comment; pass `FILE_SHARE_READ|FILE_SHARE_WRITE|FILE_SHARE_DELETE` so the code says what happens (as `administration/file_identity_windows.go:15` already does); optionally compare `basic.LastWriteTime` from the same `FILE_BASIC_INFO` call against `info.ModTime()` instead of a separate `Stat`.
  3. **Should fix** — `:21` and `:38-40`: reparse points are refused wholesale, and because the open uses `FILE_FLAG_OPEN_REPARSE_POINT` while the caller's `info` was obtained by *following* (`os.Stat` in the `root` op, `client.go:208`), a library root that is a **junction or a volume mounted at a folder** fails at the `os.SameFile` check (line 31) and surfaces as "source unavailable or storage operation timed out". Both are ordinary Windows media-server layouts, and on Unix a symlinked root and a mount point are both admitted. Files on a **Data Deduplication** volume (Windows Server) also carry a reparse attribute and are refused, although Go treats them as regular files. Recommended: open without `FILE_FLAG_OPEN_REPARSE_POINT` (the `SameFile` check already ties the handle to whatever object `info` describes, link or target), and refuse only name-surrogate reparse tags if any are still seen. If refusing junction roots is intended, say so in the owner-facing error rather than "unavailable".
  4. **Should fix** — `:50`: identity is the 32-bit volume serial plus the 64-bit file index. (a) On **ReFS** the 64-bit index is not guaranteed unique; `GetFileInformationByHandleEx(FileIdInfo)` returns the 128-bit id and a 64-bit volume serial. Use it when available and fall back to the 64-bit pair. (b) Some **SMB servers** return file index 0; then every file on the share gets the same identity and the catalogue would treat them as one object. Return `ErrObservedSourceUnsupported` when the index is 0. (c) Cloned disks share a volume serial; low risk, note only.
  5. **Should fix** — `storage/inventory_page.go:196`: for an entry reached through a followed symlink, `info` is the target (line 183) but the path passed to `snapshot` is the link, so on Windows the binding opens the link, `SameFile` fails and the entry gets an empty identity. Pass the resolved path in that branch. (Only with "follow symlinks" enabled.)
  6. **Nit** — `client.go:342` still discards the binding error, so every Windows failure mode above is a silent empty identity. Not introduced here, but it is now the only diagnostic a Windows tester would have; a debug log line in the helper would save time.
  7. **Nit** — `administration/file_identity_windows.go` already has a Windows volume/index helper with different rules; two implementations will drift.
  - Checked and fine: build tags (`windows`; `!darwin && !linux && !windows` for the stub); handle closed through `os.NewFile` + deferred `Close`; `FILE_FLAG_BACKUP_SEMANTICS` present so directories and drive roots open; the local `FILE_BASIC_INFO` struct is 40 bytes with the right layout; `%d` formatting; Unix files only gained an ignored parameter.
- **Tests run:** Windows amd64/arm64 build and vet (see top) — the new file and its two Windows-only tests compile. `go test ./internal/storage` on Linux PASS (does not exercise the new code).
- **Residual risk (needs Windows):** (i) that `os.Stat` by path (`GetFileAttributesEx`) and a handle `Stat` report the same size for a *large* directory — line 31 compares directory sizes too, and `LocalInventoryPage` (`inventory_page.go:204-212`) compares a path-stat revision with a handle-stat revision; if NTFS reports them differently, every page of a big folder fails with `ErrInventoryChanged`. The lane's test only covers an empty temp directory. Test: a folder with ~1,000 files, compare `os.Stat(dir).Size()` with `f.Stat().Size()`, then scan it. (ii) FAT/exFAT external drives: the file index is the directory-entry position and ChangeTime is 0, so identity changes on rename and can be reused. (iii) Paths longer than 260 characters. (iv) A junction *inside* a library: by reading, `os.Root.Stat` refuses an absolute link target and `LocalInventoryPage` returns the error (`inventory_page.go:188-191`), failing the whole directory rather than skipping the entry.

---

### 9a68c81 — Enable embedded subtitle extraction when the configured tool is available  (bug B-04.31)

- **Verdict: ACCEPT WITH CHANGES.**
- **What it does:** Removes the `runtime.GOOS == "windows"` line that cleared the FFmpeg path in `subtitles.New` (`subtitles/service.go:63`). That field's only use is `service.go:327`, which marks embedded tracks `extraction_unavailable`.
- **Confinement (item 2 of the brief):** Nothing on Linux changes — the diff touches only that gate; `decoder/confinement_linux.go` and the sandbox decision are untouched. On Windows extraction goes through the same path every other Windows decoder job already uses: `decoder.RunSubtitleExtract` (`decoder/subtitle.go:43-76`) → `confinedCommand` → `decoder/confinement_other.go:9` → `mediaexec.Command`. What is kept: input only from the private loopback bridge with `-protocol_whitelist http,tcp` and a format whitelist; output capped (4 MiB text, `MaxSubtitleOutput` otherwise); `PORTICO_DECODER_SANDBOX=required` still refuses to run (`mediaexec.Argv` → `ErrSandboxRequired` → `ErrConfinementUnavailable`); the job object ends the child with the server. What Windows does not have, and did not have before: any filesystem or network sandbox around FFmpeg. No guarantee was weakened.
- **Findings:**
  1. **Should fix** — the gate was stale for *remote* sources but is still true for *local* files. `subtitlevideo.Runtime.local` (`subtitlevideo/input.go:146-151`) needs `sourceaccess.Registry.Borrow` → `RegisterRoot` and `OpenObservedPlayback`, both unsupported on Windows (table rows 7–8). So after this commit a Windows server lists embedded tracks of local files as enabled, and selecting one fails, where before it honestly said `extraction_unavailable`. Worse, once Windows scanning works, `ImportScanText` (`subtitles/scan_text.go:45-57`) will try to extract every embedded text track at scan time, fail, and mark the file `subtitle_text_import_failed` (`ingestion/analysis.go:483-484`). Today this is masked because Windows has no local titles at all. Either keep the track disabled for local sources on platforms where the observed-source path is unsupported (one condition at `service.go:327`), or land this together with rows 7–8. Record the dependency either way.
  2. **Nit** — `TestConfiguredExtractionToolIsRetained` passes with and without the fix on Linux (the lane says so); it only fails without the fix when run on Windows.
- **Tests run:** `go test ./internal/subtitles` PASS; Windows vet of `subtitles`, `decoder`, `subtitlevideo`, `mediaexec` PASS.
- **Residual risk:** the loopback bridge and FFmpeg extraction on Windows have never run; the lane's test note (remote source, text and bitmap tracks, `required` sandbox setting) is the right test.

---

### be23914 — Set explicit execution and battery settings for the Windows startup task  (bug B-09.4)

- **Verdict: ACCEPT WITH CHANGES.**
- **What it does:** The installer now ships `register-task.ps1` and registers the task with the ScheduledTasks cmdlets (`ExecutionTimeLimit = 0`, start on battery, do not stop on battery, SYSTEM, highest run level, at-startup trigger), aborting the install if registration fails.
- **Read for errors:** `New-ScheduledTaskAction -Execute cmd.exe -Argument '/d /s /c ""<launcher>""'` is right (with `/s`, cmd strips the outer quote pair and runs the quoted path, so spaces and `&` are safe). `-ExecutionTimeLimit ([TimeSpan]::Zero)` serialises as `PT0S` (no limit). Principal and trigger are right. `$ErrorActionPreference = 'Stop'` gives a non-zero exit on failure. In the NSIS script, `Pop $2` takes the PowerShell exit code (the most recent push) and `StrCmp $2 "0" +3` correctly skips `MessageBox` and `Abort`. `${__FILEDIR__}` is a real NSIS predefine; the backslash path follows the existing `File /r "${STAGE_DIR}\*"` pattern that already works with makensis on Linux. `-File "…" -Launcher "…"` quoting handles an install path with spaces.
- **Findings:**
  1. **Should fix** — `installer.nsi:47-51`: installation now depends on PowerShell being allowed to run a script. A machine policy that enforces script signing (Group Policy overrides `-ExecutionPolicy Bypass`) or application control makes the script fail, and the installer then aborts after it has already deleted the old task (line 46) — so an upgrade on such a machine leaves no startup task at all. Fall back to the previous `schtasks /Create …` line when PowerShell fails (the server starts, with default limits, and say so in the log), or register from an XML file with `schtasks /Create /XML`, which needs no PowerShell.
  2. **Should fix** — `installer.nsi:47`: add `-InputFormat None`. PowerShell started from `nsExec` with a redirected standard input is a known hang on some versions; the flag costs nothing.
  3. **Nit** — `installer.nsi:50`: `MessageBox` without `/SD IDOK` blocks a silent (`/S`) install. `register-task.ps1:8` hard-codes the task name that `installer.nsi:8` defines.
- **Tests run:** none possible here (no PowerShell, no NSIS). `scripts/test-portable-packaging.py` uses a fake `makensis`, so `installer.nsi` has never been compiled since this change.
- **Residual risk:** compile the installer with real makensis; then the lane's test note (install under a path with spaces, `schtasks /Query /TN "Portico Media Server" /XML`, expect `PT0S` and both battery flags false, SYSTEM, highest) is specific and sufficient. Also run it once on Windows on ARM (the installer is 32-bit and will start the 32-bit PowerShell).

---

### f878c6b — Keep packaged startup evidence without duplicating runtime logs  (bug B-09.6)

- **Verdict: ACCEPT WITH CHANGES.**
- **What it does:** A new environment switch `PORTICO_LOG_STDERR=off` makes the server stop copying log lines to stderr once the rotated recorder is installed (`cmd/server/administration.go:52-58`). The Windows launcher sets it and truncates `server.log` on each start (`>` instead of `>>`). On macOS, launchd now runs `/bin/sh server-launcher.sh`, which sets the switch, truncates both files and `exec`s the server.
- **Checked:** default behaviour (Docker, systemd, a terminal) is unchanged — the tee stays unless the variable is exactly `off`. The recorder writes each line to its file synchronously (`servicelog.go:285-302`), and there is no `log.Fatal` after the recorder is installed (`main.go:862` onward), so a fatal message cannot be lost. Everything logged before line 862 still reaches the wrapper file. `PlistBuddy Set :ProgramArguments:1 …` matches the new two-element array; `exec` keeps the server as the launchd-tracked process.
- **Findings:**
  1. **Should fix** — `packaging/macos/tv.getportico.server.service.plist:15-16` and `launcher.sh:21-22`: launchd still opens `StandardOutPath`/`StandardErrorPath` itself, in append mode, before `/bin/sh` runs. Whenever the wrapper cannot start the server — the app was dragged to the Trash or moved, the DMG it was run from was ejected, or the `logs` folder is gone — `sh` prints an error to that appended file and `KeepAlive` relaunches it every few seconds, forever. That is unbounded growth again, in a case where the old plist wrote nothing (launchd simply failed to spawn the binary). Since the wrapper does its own redirection, set both plist paths to `/dev/null` (or delete the two keys and the two `PlistBuddy` lines).
  2. **Nit** — with the switch on, anything logged after `closeAdministration()` during shutdown goes to a closed recorder, and lines below the owner's configured log level are no longer anywhere. Acceptable; say so in the troubleshooting text.
  3. **Nit** — about sixteen call sites still write to `os.Stderr` directly, and a Go panic trace does too; these are per-event, and each launch truncates the file, so growth is bounded in practice.
- **Tests run:** `go test ./cmd/server -run TestPackagedMessageLogDoesNotDuplicateRuntimeOutput -v` PASS; the test would fail without the fix (it asserts runtime lines are absent from the stand-in stderr when the switch is `off`). `sh -n` on both macOS scripts clean; plist parses.
- **Residual risk (needs macOS):** the launch agent's program is now `/bin/sh`. Check that macOS still attributes the background item and any Files-and-Folders privacy prompts to Portico rather than to "sh", and that media on external or network volumes is still readable after the change.

---

### ded48e2 — Document least-privilege systemd access for owner-configured paths  (bug B-09.14)

- **Verdict: ACCEPT.** A documentation mitigation, not a behavioural fix; record it that way.
- **What it does:** Adds an optional `EnvironmentFile=-/etc/portico-media-server/environment` to the unit, two comments, and an INSTALLING.md section telling owners to add `ReadWritePaths=` (and `Environment=`) in a `systemctl edit` drop-in.
- **Against systemd's directives (item 4):** No hardening was removed — `NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`, `ProtectHome=read-only` and the state `ReadWritePaths` are untouched. What the text claims is correct per `systemd.exec(5)` (systemd 259 manual on this host): `ReadWritePaths=` is a list setting, so a drop-in adds to it; it is the documented way to allow writes under `ProtectSystem=strict`; `ProtectHome=read-only` is "mostly equivalent to `ReadOnlyPaths=`" and the manual says to nest `ReadWritePaths=` inside read-only paths, so a path under `/home` works; quoted paths with spaces parse (`systemd-analyze verify` accepted my sample). `EnvironmentFile=` with the `-` prefix is optional and is read by systemd as root before privileges drop.
- **Findings:**
  1. **Nit** — INSTALLING.md: a `ReadWritePaths=` entry that does not exist at start makes the service fail with `226/NAMESPACE`. The text says "create those directories first"; also mention the `-` prefix (`ReadWritePaths=-/mnt/usb/recordings`) or `RequiresMountsFor=` for removable and network media, otherwise an unplugged disk stops the whole server from starting.
  2. **Nit** — the audit entry also lists VAAPI/QSV device access (`render`/`video` groups); the new section does not mention `SupplementaryGroups=render video`. One line in the same drop-in example would cover it.
  3. **Nit (scope)** — `EnvironmentFile=` is a small addition beyond the defect; the drop-in already shows `Environment=`. Harmless; a file there overrides the unit's `Environment=` lines, including `PORTICO_STATE_DIR`, which the text does cover.
  - The console still offers deletion and an external recordings folder that will fail until the owner edits the unit; nothing tells them at the point of failure. That is the "configuration diagnostics" half of the second review's fix direction and is not done here.
- **Tests run:** `systemd-analyze verify` on a scratch copy with a drop-in: no directive errors.
- **Residual risk:** the lane's test note (install the package in a VM, apply the drop-in, delete a file, record, check an unrelated path stays read-only) is specific and would catch a wrong claim.

---

### 751ca7e — Protect state access without walking or chmodding media  (bug B-09.15)

- **Verdict: ACCEPT WITH CHANGES.**
- **What it does:** `CheckPermissions` and `FixPermissions` (`backup/stateperms.go`) no longer walk the tree. They open the state directory itself, confirm it is the same non-symlink directory `Lstat` saw, and check or `fchmod` only that one directory (removing group/other bits, keeping owner bits). On Windows the check reports "not exposed" and the fix returns a new error mapped to `administration_unavailable`.
- **Item 3 of the brief — can it change permissions outside the state folder?** No, under any configuration I could construct. The only write is `file.Chmod` on a handle (`stateperms.go:48`) that `openStatePermissionBoundary` has verified with `os.SameFile` against an `Lstat` that was a directory, not a symlink (`:52-71`). No descendant is opened, listed or chmodded, so a media folder, rclone mount, recordings folder, backup, symlink or hard link inside the state folder is never touched; cache, recordings or backup paths configured outside the state folder were never visited before and still are not. A swap of the state path for a symlink between `Lstat` and `Open` fails the `SameFile` check.
- **Does it still protect the state folder?** Yes: a state directory with no group/other bits cannot be traversed by another account, whatever the modes of the files inside, and that is what is checked and repaired. What is given up is defence in depth: files inside stay `0644` if they were, so they are exposed again if the directory mode is later loosened. That matches the second review's reading ("a private parent with loose children need not actually expose those files") and removes the false warning.
- **Findings:**
  1. **Should fix** — `stateperms.go:52-58`: a state path that is itself a symlink (for example `/var/lib/portico` → a bigger disk) is now rejected. At start-up the check logs an error and never sets or clears the alert (`main.go:830-834`); the fix button returns `internal` (HTTP 500) through `backupRouteError`. Before, the symlinked root was silently reported as fine, which was also wrong, but the new behaviour is a dead end. Resolve the path once (`filepath.EvalSymlinks`) and apply the same boundary logic to the resolved directory — the symlink's target *is* the state folder.
  2. **Should fix / decide** — `stateperms.go:17-19`: on Windows the check answers "not exposed", which the console shows as a clean bill of health, while B-09.5 says the ProgramData folder inherits permissive access. The second review asked for "a truthful unsupported state". There is no such state in `StatePermissionsDocument`, so returning false is the smallest change, but it should not be read as verified. At minimum log once on Windows that access is not assessed.
  3. **Nit** — the Docker half of the audit entry is not addressed (deliberately): the image's home directory is still created with default mode, so a new container can still warn. The other lane edits the Dockerfile; a `chmod 700` there would close it.
  4. **Nit** — Backups spec §6 wording ("tightens only Portico's own files") needs the update the lane lists.
- **Tests run:** `go test ./internal/backup` PASS, including the three new tests (private parent with loose child does not warn; fix leaves a hard-linked external file and descendants untouched; symlinked root rejected) and the amended `TestStatePermissionsCheckAndFix`. Windows vet compiles `stateperms_windows_test.go`.
- **Residual risk:** state on a filesystem that ignores `chmod` (a CIFS or exFAT mount) keeps warning after the fix, as before.

---

### 1bb58bc — Configure web and state paths in portable archive launchers  (bug B-09.27)

- **Verdict: ACCEPT WITH CHANGES.**
- **What it does:** Adds `packaging/portable/start-portico.sh`, `start-portico.cmd` and `README.txt`, and copies them into the Linux tarball and the Windows ZIP. The launchers default `PORTICO_WEB_DIR` and `PORTICO_STATE_DIR` to folders beside the launcher and pass arguments through.
- **Read for errors:** the shell launcher is correct for paths with spaces (`cd -P -- "$(dirname -- "$0")"`, quoted expansions, `exec … "$@"`). The batch file is correct: `%~dp0` ends in a backslash, `set "NAME=value"` protects spaces and `&`, `if not defined` preserves an owner's override, `%*` forwards arguments. FFmpeg is found beside the executable by `mediatools.Resolve` (it uses `os.Executable`), so no variable is needed. The `.deb`/`.rpm` are unaffected (nfpm lists its contents explicitly).
- **Findings:**
  1. **Should fix** — `scripts/package-release.sh:40-42`: the portable files are copied into `$PAYLOAD`, and the NSIS installer then packs the same folder (`installer.nsi:23`, `File /r "${STAGE_DIR}\*"`). So the **installed** Windows server gets the portable `README.txt` (which tells the owner state lives "beside the launcher" — false for an install) and, for a moment, the portable `start-portico.cmd` under the same name the installer's own launcher uses (it is overwritten at `installer.nsi:38-45`). If an install is interrupted between those two steps, an existing startup task would run the portable launcher and start a fresh, empty server with its state under Program Files. Build the ZIP from a copy that has the portable files, or exclude them in NSIS (`File /r /x README.txt /x start-portico.cmd …`).
  2. **Should fix** — merge conflict with the other lane in this exact hunk; see Cross-commit issues.
  3. **Nit** — `start-portico.cmd` and `README.txt` are committed with LF line endings. Simple batch files run with LF, but CRLF is the safe form; add a `.gitattributes` entry (`*.cmd text eol=crlf`).
  4. **Nit** — `start-portico.sh:3`: if the launcher is run through a symlink, `$0` is the link and the payload folder is wrong. `start-portico.sh:4`: `umask 077` also applies to anything the server writes outside its state (recordings in a shared folder become owner-only); fine for a personal run, worth a line in the README.
- **Tests run:** `python3 scripts/test-portable-packaging.py` 2 PASS. The Linux test really runs the unpacked launcher from another directory with a space in the path; the Windows test only checks the two files are in the ZIP — the batch file has never been executed.
- **Residual risk:** the lane's test note (unpack into a path with spaces, launch from elsewhere, check state lands in `<unpacked>\state`) is adequate.

---

### b4202a3 — Authenticate final macOS binary digests after codesigning  (bug B-25.8)

- **Verdict: ACCEPT WITH CHANGES; one decision for Justin.** The default unsigned release is unaffected: every new step is conditional on `PORTICO_MACOS_SIGNING_IDENTITY`.
- **What it does:** In signed mode `package-macos.sh` now calls a new `seal-macos-payload.py`, which (1) verifies the builder's manifest signature, artifact digest, toolchain digests and embedded build identity, (2) copies the original `manifest.json`, `manifest.sig` and `toolchain-manifest.json` to `source-seal/`, (3) codesigns the server and each FFmpeg tool, (4) rewrites the toolchain manifest and the server manifest with the post-signing digests plus the digests of the three preserved originals, (5) signs the new manifest with the release key and re-verifies. The app bundle is then signed, verified, and the payload verified once more.
- **Order and coverage (item 5):** the order is right — nested binaries are signed before the manifest is re-signed, and the app is signed last without `--deep`, so it does not re-sign them; the final verify would catch it if it did. The final signature covers: the signed server binary; via `toolchainManifestSha256`, every file in the FFmpeg bundle including the signed tools; and the three original seal files. It still does not cover `web/` or the licence files (that is B-25.7, not this bug). `toolchain["files"]` keys (`bin/ffmpeg`) match the format `verify-ffmpeg-manifest.py` and `mediatools.VerifyBundle` expect; the added `sourceManifestSha256` and `sourceSeal` keys are ignored by both readers (neither rejects unknown fields); `source-seal/` sits outside the FFmpeg bundle so the bundle inventory check is unaffected.
- **Findings:**
  1. **Decision / Should fix** — `.github/workflows/release.yml:160-171`: the release private key is now written to disk on a second runner (the hosted `macos-15` job), where before it existed only in the Linux build job. `server/api/release-signing.md` says the key belongs on "a dedicated hardware signer or an isolated release runner". The step is scoped to signed builds and removed with `if: always()`, which is as careful as this design can be, but it is a custody change and Justin should choose it knowingly (see "For Justin").
  2. **Should fix** — `seal-macos-payload.py:100-109`: `--verify` requires the **private** key file, only to derive the public key. So nobody can check a signed DMG without holding the release private key, and the lane's own test note tells the tester to do exactly that. Accept a public key for verification (`--public-key FILE`; the builder already writes `runtime/release-signing/public-key`) and keep `--key` for sealing only.
  3. **Should fix (unverified)** — `release.yml:157-159`: `python3 -m pip install --user …` on the macOS runner. Homebrew's Python there is marked externally managed and has refused plain `pip install` on recent macOS images; if so this step fails the first time signing is switched on. Use a virtual environment (and put it on `PATH` for `package-macos.sh`) or `actions/setup-python` pinned by hash. I could not run a macOS runner to confirm.
  4. **Should fix (hygiene)** — `scripts/test-macos-seal.py:79`: the committed test sets `GOCACHE="/tmp/gocache-codex", GOPROXY="off", GOTOOLCHAIN="local"` — the lane's sandbox settings, including a tool name, in a repository file. The test uses a fake `go`; delete the three entries.
  5. **Should fix** — `seal-macos-payload.py:47-56`: when a `sourceSeal` is present the verifier checks the three files' digests and the original signature, but not that the original manifest describes the same build as the final one. Add: every field of `source-seal/manifest.json` except `artifact.sha256` and `toolchainManifestSha256` must equal the final manifest's. It is a few lines and makes the provenance claim checkable instead of manual.
  6. **Nit** — the workflow passes a signing identity but nothing imports a certificate into a keychain (true before this commit as well), so a real identity cannot work on a hosted runner yet; only ad-hoc (`-`) would.
- **Tests run:** `python3 scripts/test-macos-seal.py` 6 PASS on Linux with fake `codesign`, `hdiutil` and `go` and real Ed25519. The tests do fail for the right reasons without the fix (digest mismatch after the fake signer appends bytes; tampered input and wrong key reach the signer). `bash -n scripts/package-macos.sh` clean; workflow YAML parses.
- **Residual risk (needs macOS and a real identity):** that `go version -m` still reads the build identity from a codesigned Mach-O (the script checks this and fails closed if not); that signing the app leaves the nested binaries byte-identical (also checked); that `scripts/env.sh`, which the seal script goes through and which exports a `DEVELOPER_DIR` for a beta Xcode, is harmless on the runner.

---

## Skipped items

The lane skipped nothing; all eight are reported as FIXED — UNTESTED. Two of those labels overstate:

- **B-05.28** is not fixed (see the table above). The lane says this clearly in its "Not verified" paragraph; the status line should read "PARTIAL — identity only".
- **B-09.14** is documentation plus an optional environment file. That is a reasonable outcome for a pass that must not weaken the sandbox, but the console still offers actions that fail until the owner edits the unit.

Not skipping B-05.28's remaining work was the right call for this pass: a Windows `inventory-command`, root registration and observed playback are more than a day and touch the read-confinement design.

## Cross-commit issues

**Inside this lane**

- **1bb58bc × be23914:** portable files end up inside the Windows installer (finding 1 under 1bb58bc).
- **9a68c81 depends on the unfinished part of 2d0a27d:** enabling embedded extraction on Windows is only truthful for remote sources until root registration and observed playback exist there (finding 1 under 9a68c81).
- **f878c6b × be23914:** both edit the launcher lines of `installer.nsi`; they are consistent with each other (truncate, `PORTICO_LOG_STDERR=off`, then PowerShell registration).

**Against `audit-fixes/identity-ops-live`** (three files changed by both; tested with `git merge-file` on scratch copies, nothing written to either repository)

| File | Result | Resolution |
| --- | --- | --- |
| `scripts/package-release.sh` | **Textual conflict** in the `windows)` case: this lane inserts the `cp … start-portico.cmd` line where the other lane inserts the lines that make `OUT` and `PAYLOAD` absolute. | Keep both, absolute paths first, then the copy, then `zip` and `makensis`. Better, resolve it together with finding 1 under 1bb58bc: after making the paths absolute, build the ZIP from a copy of the payload that includes the portable files, and hand `makensis` the payload without them. |
| `.github/workflows/release.yml` | Merges cleanly (other lane: `tests` job `with: ref`, and a toolchain step in `docker`; this lane: the `macos` job). | None. No contradiction: the other lane's update manifest still says `signed: false`, which stays true for the release as a whole. |
| `server/internal/backup/backup_test.go` | Merges cleanly; both add the same `"runtime"` import on the same line, which git folds into one. | None; run `go vet ./internal/backup` after the merge to confirm a single import. |

Files only the other lane changed that interact with this lane:

- `packaging/linux/postinstall.sh` / `preremove.sh` (start on install, `try-restart` on upgrade, stop only on removal): compatible with this lane's unit change. Owner drop-ins live under `/etc/systemd/system/portico-media-server.service.d/` and survive upgrades; the new `EnvironmentFile=` is optional, so the first start on install is unaffected. INSTALLING.md's "restart after editing" instruction still holds.
- `Dockerfile`: the other lane does not change how the state directory is created, so the Docker warning described under 751ca7e finding 3 remains.

## Summary

| Commit | Bug | Verdict | Blockers |
| --- | --- | --- | --- |
| 2d0a27d | B-05.28 | ACCEPT WITH CHANGES | Bug stays open: the first scan fails every file at the probe (`scan_exec_other.go:7`); no titles on Windows |
| 9a68c81 | B-04.31 | ACCEPT WITH CHANGES | none |
| be23914 | B-09.4 | ACCEPT WITH CHANGES | none |
| f878c6b | B-09.6 | ACCEPT WITH CHANGES | none |
| ded48e2 | B-09.14 | ACCEPT | none |
| 751ca7e | B-09.15 | ACCEPT WITH CHANGES | none |
| 1bb58bc | B-09.27 | ACCEPT WITH CHANGES | none |
| b4202a3 | B-25.8 | ACCEPT WITH CHANGES | none (one decision) |

## For Justin

1. **Release key on the macOS runner (b4202a3).** Signed macOS builds now need the release private key on the hosted macOS job. Accept that (ideally behind a protected GitHub Environment), or leave signed macOS builds off until there is a signer that does not spread the key. Unsigned releases are unaffected either way.
2. **Junction and mounted-folder library roots on Windows (2d0a27d).** The new code refuses them. Decide whether they should be admitted, as symlinked roots and mount points are on Linux and macOS.
3. **What the Windows console should say about state-folder access (751ca7e).** It now says "not exposed" without having checked.
4. **Order of work for Windows.** The next piece is a Windows `inventory-command` (table rows 1–4); without it nothing else on Windows can be tested with a local library.
